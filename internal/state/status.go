package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

const (
	ModeMarkdownOnly = "markdown-only"
	ModeSQLiteReady  = "sqlite-ready"
	ModeInvalid      = "invalid"
)

const (
	RepairCategoryLocalDatabase          = "local-database"
	RepairCategoryStorageMigration       = "storage-migration"
	RepairCategoryProjectIdentity        = "project-identity"
	RepairCategoryRelationshipProvenance = "relationship-provenance"
	RepairCategoryBackendMapping         = "backend-mapping"
	RepairCategoryExternalSync           = "external-sync"
	RepairCategoryMarkdownImport         = "markdown-import"
	RepairCategoryCompatibilityExport    = "compatibility-export"
	RepairCategoryJournalSearch          = "journal-search"
)

const (
	DiagnosticPolicyInvalidLocalData     = "invalid-local-data"
	DiagnosticPolicyWarningDrift         = "warning-drift"
	DiagnosticPolicyExternalSyncGap      = "external-sync-gap"
	DiagnosticPolicyImportPending        = "import-pending"
	DiagnosticPolicyStaleExport          = "stale-export"
	DiagnosticPolicyDerivedIndexDiverged = "derived-index-diverged"
)

// Diagnostic describes a state-runtime observation without mutating state.
type Diagnostic struct {
	Severity             string         `json:"severity"`
	Code                 string         `json:"code"`
	Category             string         `json:"category,omitempty"`
	Policy               string         `json:"policy,omitempty"`
	Message              string         `json:"message"`
	Details              map[string]any `json:"details,omitempty"`
	RequiresExternalSync bool           `json:"requires_external_sync,omitempty"`
}

// RepairAction describes an explicit repair recommendation from diagnostics.
type RepairAction struct {
	Code                 string `json:"code"`
	DiagnosticCode       string `json:"diagnostic_code"`
	Category             string `json:"category"`
	Description          string `json:"description"`
	Command              string `json:"command,omitempty"`
	Path                 string `json:"path,omitempty"`
	Safe                 bool   `json:"safe"`
	Applied              bool   `json:"applied"`
	RequiresExternalSync bool   `json:"requires_external_sync"`
}

// Status is the pre-init state view exposed by `loaf state status`.
type Status struct {
	ContractVersion      int            `json:"contract_version"`
	DatabaseScope        string         `json:"database_scope"`
	ProjectRoot          string         `json:"project_root"`
	ProjectID            string         `json:"project_id,omitempty"`
	LegacyProjectKey     string         `json:"legacy_project_key,omitempty"`
	ProjectName          string         `json:"project_name,omitempty"`
	ProjectCurrentPath   string         `json:"project_current_path,omitempty"`
	DatabasePath         string         `json:"database_path"`
	LegacyDatabasePath   string         `json:"legacy_database_path,omitempty"`
	DatabaseExists       bool           `json:"database_exists"`
	LegacyDatabaseExists bool           `json:"legacy_database_exists"`
	DatabaseParentExists bool           `json:"database_parent_exists"`
	SchemaVersion        int            `json:"schema_version"`
	Mode                 string         `json:"mode"`
	Diagnostics          []Diagnostic   `json:"diagnostics"`
	RepairPlan           []RepairAction `json:"repair_plan"`
}

// Inspect returns the current state-runtime status without creating files.
func Inspect(root project.Root, resolver PathResolver) (Status, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return Status{}, err
	}

	status := Status{
		ContractVersion:  StateJSONContractVersion,
		DatabaseScope:    "global",
		ProjectRoot:      root.Path(),
		LegacyProjectKey: ProjectID(root),
		DatabasePath:     databasePath,
		Diagnostics:      []Diagnostic{},
		RepairPlan:       []RepairAction{},
	}
	if legacyPath, err := migrationSourceDatabasePath(root, resolver); err == nil && legacyPath != databasePath {
		status.LegacyDatabasePath = legacyPath
		if info, err := os.Stat(legacyPath); err == nil && !info.IsDir() {
			status.LegacyDatabaseExists = true
		}
	}

	parent := filepath.Dir(databasePath)
	if info, err := os.Stat(parent); err == nil && info.IsDir() {
		status.DatabaseParentExists = true
	}

	info, err := os.Stat(databasePath)
	switch {
	case err == nil && info.IsDir():
		status.DatabaseExists = true
		status.Mode = ModeInvalid
		status.Diagnostics = append(status.Diagnostics, Diagnostic{
			Severity: "error",
			Code:     "database-path-is-directory",
			Message:  fmt.Sprintf("database path is a directory: %s", databasePath),
		})
	case err == nil && info.Size() == 0:
		status.DatabaseExists = true
		status.Mode = ModeInvalid
		status.Diagnostics = append(status.Diagnostics, Diagnostic{
			Severity: "error",
			Code:     "database-file-empty",
			Message:  fmt.Sprintf("database file is empty: %s", databasePath),
		})
	case err == nil:
		status.DatabaseExists = true
		store, err := OpenStoreReadOnly(databasePath)
		if err != nil {
			status.Mode = ModeInvalid
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "error",
				Code:     "database-open-failed",
				Message:  err.Error(),
			})
			return status, nil
		}
		defer store.Close()
		version, err := store.SchemaVersion(context.Background())
		if err != nil {
			status.Mode = ModeInvalid
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "error",
				Code:     "schema-version-unreadable",
				Message:  err.Error(),
			})
			return status, nil
		}
		status.SchemaVersion = version
		schemaDiagnostics, schemaValid := inspectSchemaMigrations(context.Background(), store, version)
		status.Diagnostics = append(status.Diagnostics, schemaDiagnostics...)
		if !schemaValid {
			status.Mode = ModeInvalid
			return status, nil
		}
		invariantDiagnostics, invariantValid, err := inspectOperationalInvariants(context.Background(), store)
		if err != nil {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "error",
				Code:     "state-invariants-unreadable",
				Message:  err.Error(),
			})
		} else {
			status.Diagnostics = append(status.Diagnostics, invariantDiagnostics...)
		}
		if !invariantValid {
			status.Mode = ModeInvalid
			return status, nil
		}
		status.Mode = ModeSQLiteReady
		if identity, err := store.LookupProjectIdentityForRoot(context.Background(), root); err == nil {
			status.ProjectID = identity.ID
			status.ProjectName = identity.FriendlyName
			status.ProjectCurrentPath = identity.CurrentPath
		} else {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "project-identity-unreadable",
				Message:  err.Error(),
			})
		}
		status.Diagnostics = append(status.Diagnostics, Diagnostic{
			Severity: "info",
			Code:     "sqlite-ready",
			Message:  fmt.Sprintf("SQLite state database is ready at schema version %d", version),
		})
		if status.LegacyDatabaseExists {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "legacy-project-database-leftover",
				Message:  fmt.Sprintf("legacy project database remains at %s after global DB initialization", status.LegacyDatabasePath),
			})
		}
		markdownDiagnostics, err := inspectUnimportedLocalMarkdown(context.Background(), root, store, status.ProjectID)
		if err != nil {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "local-markdown-import-check-unreadable",
				Message:  err.Error(),
			})
		} else {
			status.Diagnostics = append(status.Diagnostics, markdownDiagnostics...)
		}
		ephemeralDiagnostics, err := inspectEphemeralMarkdownCutover(root)
		if err != nil {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "ephemeral-markdown-cutover-check-unreadable",
				Message:  err.Error(),
			})
		} else {
			status.Diagnostics = append(status.Diagnostics, ephemeralDiagnostics...)
		}
		linearDiagnostics, err := inspectLinearModeTaskMappings(context.Background(), root, store, status.ProjectID)
		if err != nil {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "linear-mode-task-mappings-unreadable",
				Message:  err.Error(),
			})
		} else {
			status.Diagnostics = append(status.Diagnostics, linearDiagnostics...)
		}
		exportDiagnostics, err := inspectStaleExports(context.Background(), store)
		if err != nil {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "export-staleness-unreadable",
				Message:  err.Error(),
			})
		} else {
			status.Diagnostics = append(status.Diagnostics, exportDiagnostics...)
		}
	case os.IsNotExist(err):
		status.Mode = ModeMarkdownOnly
		status.Diagnostics = append(status.Diagnostics,
			Diagnostic{
				Severity: "warn",
				Code:     "database-missing",
				Message:  "SQLite state database does not exist yet",
			},
			Diagnostic{
				Severity: "info",
				Code:     "markdown-fallback-active",
				Message:  "Markdown compatibility remains active for projects without initialized SQLite state",
			},
		)
		if status.LegacyDatabaseExists {
			status.Diagnostics = append(status.Diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "legacy-state-database-detected",
				Message:  fmt.Sprintf("legacy SQLite state database exists at %s; run `loaf state migrate storage-home --apply` to copy it to %s", status.LegacyDatabasePath, status.DatabasePath),
			})
		}
	default:
		return Status{}, fmt.Errorf("inspect state database: %w", err)
	}

	return status, nil
}

// RepairPlanForStatus turns diagnostics into explicit, non-surprising repair actions.
func RepairPlanForStatus(status Status) []RepairAction {
	actions := []RepairAction{}
	for _, diagnostic := range status.Diagnostics {
		switch diagnostic.Code {
		case "database-missing":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "initialize-database",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryLocalDatabase,
				Description:    "Initialize the global SQLite database for this project.",
				Command:        "loaf state doctor --fix",
				Path:           status.DatabasePath,
				Safe:           true,
			})
		case "legacy-state-database-detected":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "migrate-storage-home",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryStorageMigration,
				Description:    "Preview and then apply storage-home migration to copy legacy state into the global database.",
				Command:        "loaf state migrate storage-home --dry-run",
				Path:           status.LegacyDatabasePath,
				Safe:           false,
			})
		case "legacy-project-database-leftover":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "review-legacy-project-database",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryLocalDatabase,
				Description:    "Preview archiving the leftover legacy project database after verifying the global database.",
				Command:        "loaf state repair legacy-project-database --dry-run --json",
				Path:           status.LegacyDatabasePath,
				Safe:           false,
			})
		case "schema-version-mismatch", "schema-checksum-mismatch", "schema-migration-missing":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "inspect-schema-migrations",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryLocalDatabase,
				Description:    "Inspect schema migration drift before applying any repair.",
				Command:        "loaf state doctor --json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "state-invariants-unreadable", "sqlite-integrity-check-failed", "sqlite-foreign-key-violation":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "inspect-state-invariants",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryLocalDatabase,
				Description:    "Inspect SQLite table integrity before applying any state repair.",
				Command:        "loaf state doctor --json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "project-current-path-missing", "project-current-path-mismatch", "orphaned-project-path":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "repair-project-path-invariants",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryProjectIdentity,
				Description:    "Inspect project identity and path history before repairing project path invariants.",
				Command:        "loaf project list --json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "relationship-origin-missing", "relationship-origin-unknown":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "audit-relationship-origin",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryRelationshipProvenance,
				Description:    "Audit relationship provenance before backfilling or pruning relationship rows.",
				Command:        "loaf state repair relationship-origin --origin imported --dry-run --json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "backend-mapping-field-empty", "backend-mapping-sensitive-value", "backend-mapping-entity-kind-unknown", "backend-mapping-entity-missing":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "inspect-backend-mappings",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryBackendMapping,
				Description:    "Inspect invalid local backend mapping rows before pruning or reconnecting integration metadata.",
				Command:        "loaf state doctor --json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "backend-mapping-entity-ambiguous", "backend-mapping-sync-status-unknown":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "audit-backend-mappings",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryBackendMapping,
				Description:    "Audit local backend mapping drift before pruning or reconnecting integration metadata.",
				Command:        "loaf state export all --format json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "linear-mode-local-task-unmapped":
			actions = appendRepairAction(actions, RepairAction{
				Code:                 "reconcile-linear-task-mappings",
				DiagnosticCode:       diagnostic.Code,
				Category:             RepairCategoryExternalSync,
				Description:          "Export local task state, then reconcile active local tasks with Linear or future backend sync tooling.",
				Command:              "loaf state export all --format json",
				Path:                 status.DatabasePath,
				Safe:                 false,
				RequiresExternalSync: true,
			})
		case "stale-compatibility-export":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "regenerate-export",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryCompatibilityExport,
				Description:    "Regenerate the stale compatibility export from SQLite state.",
				Safe:           false,
			})
		case JournalSearchDivergenceCode:
			actions = appendRepairAction(actions, RepairAction{
				Code:           "repair-journal-search",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryJournalSearch,
				Description:    "Rebuild the derived journal search index from canonical journal entries after creating a verified backup.",
				Command:        "loaf state repair journal-search --dry-run --json",
				Path:           status.DatabasePath,
				Safe:           false,
			})
		case "local-markdown-not-imported":
			actions = appendRepairAction(actions, RepairAction{
				Code:           "migrate-current-project-markdown",
				DiagnosticCode: diagnostic.Code,
				Category:       RepairCategoryMarkdownImport,
				Description:    "Preview importing this project's local .agents Markdown artifacts into the global SQLite database.",
				Command:        "loaf state migrate markdown --dry-run",
				Path:           status.ProjectRoot,
				Safe:           true,
			})
		}
	}
	return actions
}

func appendRepairAction(actions []RepairAction, action RepairAction) []RepairAction {
	for _, existing := range actions {
		if existing.Code == action.Code &&
			existing.DiagnosticCode == action.DiagnosticCode &&
			existing.Command == action.Command &&
			existing.Path == action.Path {
			return actions
		}
	}
	return append(actions, action)
}

func inspectSchemaMigrations(ctx context.Context, store *Store, version int) ([]Diagnostic, bool) {
	diagnostics := []Diagnostic{}
	valid := true
	if !acceptableSchemaVersion(version) {
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "schema-version-mismatch",
			Message:  fmt.Sprintf("schema version %d does not match expected version %d", version, CurrentSchemaVersion()),
		})
	}

	for _, migration := range SchemaMigrations() {
		var checksum string
		err := store.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).Scan(&checksum)
		switch {
		case err == nil && checksum != migration.Checksum():
			valid = false
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "error",
				Code:     "schema-checksum-mismatch",
				Message:  fmt.Sprintf("schema migration %d checksum does not match Go-owned migration", migration.Version),
			})
		case errorsIsNoRows(err):
			valid = false
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "error",
				Code:     "schema-migration-missing",
				Message:  fmt.Sprintf("schema migration %d is missing", migration.Version),
			})
		case err != nil:
			valid = false
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "error",
				Code:     "schema-version-unreadable",
				Message:  err.Error(),
			})
		}
	}

	// The journal-first migration is applied out-of-band and is absent on a
	// pre-migration database. When present, its checksum must still match the
	// Go-owned migration; a drifted checksum is drift like any other.
	journalFirst := JournalFirstMigration()
	var journalFirstChecksum string
	switch err := store.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, journalFirst.Version).Scan(&journalFirstChecksum); {
	case errorsIsNoRows(err):
		// Not applied; valid.
	case err != nil:
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "schema-version-unreadable",
			Message:  err.Error(),
		})
	case journalFirstChecksum != journalFirst.Checksum():
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "schema-checksum-mismatch",
			Message:  fmt.Sprintf("schema migration %d checksum does not match Go-owned migration", journalFirst.Version),
		})
	}
	return diagnostics, valid
}

func inspectOperationalInvariants(ctx context.Context, store *Store) ([]Diagnostic, bool, error) {
	diagnostics := []Diagnostic{}
	valid := true

	sqliteDiagnostics, sqliteValid, err := inspectSQLiteIntegrity(ctx, store)
	if err != nil {
		return nil, false, err
	}
	diagnostics = append(diagnostics, sqliteDiagnostics...)
	if !sqliteValid {
		valid = false
	}

	projectPathDiagnostics, projectPathsValid, err := inspectProjectPathInvariants(ctx, store)
	if err != nil {
		return nil, false, err
	}
	diagnostics = append(diagnostics, projectPathDiagnostics...)
	if !projectPathsValid {
		valid = false
	}

	relationshipDiagnostics, err := inspectRelationshipOriginInvariants(ctx, store)
	if err != nil {
		return nil, false, err
	}
	diagnostics = append(diagnostics, relationshipDiagnostics...)

	backendDiagnostics, backendMappingsValid, err := inspectBackendMappingInvariants(ctx, store)
	if err != nil {
		return nil, false, err
	}
	diagnostics = append(diagnostics, backendDiagnostics...)
	if !backendMappingsValid {
		valid = false
	}

	journalSearchParity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		return nil, false, err
	}
	if !journalSearchParity.Ready {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     JournalSearchDivergenceCode,
			Category: RepairCategoryJournalSearch,
			Policy:   DiagnosticPolicyDerivedIndexDiverged,
			Message: fmt.Sprintf(
				"journal search index diverged from canonical journal (canonical_rows=%d, index_rows=%d, missing=%d, extra=%d, changed=%d); run: loaf state repair journal-search --dry-run",
				journalSearchParity.CanonicalRows,
				journalSearchParity.IndexRows,
				journalSearchParity.Missing,
				journalSearchParity.Extra,
				journalSearchParity.Changed,
			),
			Details: map[string]any{
				"canonical_rows": journalSearchParity.CanonicalRows,
				"index_rows":     journalSearchParity.IndexRows,
				"missing":        journalSearchParity.Missing,
				"extra":          journalSearchParity.Extra,
				"changed":        journalSearchParity.Changed,
			},
		})
	}

	journalProvenance, err := InspectJournalProvenanceIntegrity(ctx, store)
	if err != nil {
		return nil, false, err
	}
	if !journalProvenance.Ready {
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "journal-provenance-invalid",
			Category: RepairCategoryRelationshipProvenance,
			Policy:   DiagnosticPolicyInvalidLocalData,
			Message:  fmt.Sprintf("journal provenance endpoints are invalid (origin_missing_journal=%d, origin_project_mismatches=%d, deferral_missing_decision=%d, deferral_missing_spark=%d, deferral_project_mismatches=%d)", journalProvenance.OriginMissingJournal, journalProvenance.OriginProjectMismatches, journalProvenance.DeferralMissingDecision, journalProvenance.DeferralMissingSpark, journalProvenance.DeferralProjectMismatches),
			Details: map[string]any{
				"origin_rows":                 journalProvenance.OriginRows,
				"origin_missing_journal":      journalProvenance.OriginMissingJournal,
				"origin_project_mismatches":   journalProvenance.OriginProjectMismatches,
				"deferral_rows":               journalProvenance.DeferralRows,
				"deferral_missing_decision":   journalProvenance.DeferralMissingDecision,
				"deferral_missing_spark":      journalProvenance.DeferralMissingSpark,
				"deferral_project_mismatches": journalProvenance.DeferralProjectMismatches,
			},
		})
	}

	return diagnostics, valid, nil
}

func inspectUnimportedLocalMarkdown(ctx context.Context, root project.Root, store *Store, projectID string) ([]Diagnostic, error) {
	plan, err := PreviewMarkdownMigration(root)
	if err != nil {
		return nil, err
	}
	importableCount := markdownMigrationImportableCount(plan)
	if importableCount == 0 {
		return nil, nil
	}
	if projectID != "" {
		var importedSources int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sources WHERE project_id = ? AND path LIKE '.agents/%'`, projectID).Scan(&importedSources); err != nil {
			return nil, fmt.Errorf("inspect imported Markdown sources: %w", err)
		}
		if importedSources > 0 {
			return nil, nil
		}
	}
	return []Diagnostic{{
		Severity: "warn",
		Code:     "local-markdown-not-imported",
		Category: RepairCategoryMarkdownImport,
		Policy:   DiagnosticPolicyImportPending,
		Message:  fmt.Sprintf("local .agents Markdown has %d importable artifact(s), but this project has no imported Markdown sources in the global SQLite database; run `loaf state migrate markdown --dry-run` before trusting empty SQLite output", importableCount),
		Details: map[string]any{
			"agents_path":       plan.AgentsPath,
			"importable_count":  importableCount,
			"specs":             plan.Specs,
			"tasks":             plan.Tasks,
			"ideas":             plan.Ideas,
			"sparks":            plan.Sparks,
			"brainstorms":       plan.Brainstorms,
			"shaping_drafts":    plan.ShapingDrafts,
			"sessions":          plan.Sessions,
			"reports":           plan.Reports,
			"relationships":     plan.Relationships,
			"imported_sources":  0,
			"preview_command":   "loaf state migrate markdown --dry-run",
			"migration_command": "loaf state migrate markdown --apply",
		},
	}}, nil
}

func markdownMigrationImportableCount(plan MarkdownMigrationPlan) int {
	return plan.Specs +
		plan.Tasks +
		plan.Ideas +
		plan.Sparks +
		plan.Brainstorms +
		plan.ShapingDrafts +
		plan.Sessions +
		plan.Reports +
		plan.Relationships
}

func inspectEphemeralMarkdownCutover(root project.Root) ([]Diagnostic, error) {
	markdownFiles, err := countEphemeralMarkdownFiles(filepath.Join(root.Path(), ".agents"))
	if err != nil {
		return nil, err
	}
	tasksJSONPresent := false
	tasksJSONPath := filepath.Join(root.Path(), ".agents", "TASKS.json")
	if info, err := os.Stat(tasksJSONPath); err == nil && !info.IsDir() {
		tasksJSONPresent = true
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect .agents/TASKS.json: %w", err)
	}
	details := map[string]any{
		"ephemeral_markdown_files": markdownFiles,
		"tasks_json_present":       tasksJSONPresent,
	}
	if markdownFiles == 0 && !tasksJSONPresent {
		return []Diagnostic{{
			Severity: "info",
			Code:     "ephemeral-markdown-cutover-clear",
			Category: RepairCategoryMarkdownImport,
			Message:  "Ephemeral Markdown cutover surface is clear: 0 ephemeral Markdown file(s); .agents/TASKS.json absent",
			Details:  details,
		}}, nil
	}
	return []Diagnostic{{
		Severity: "warn",
		Code:     "ephemeral-markdown-cutover-drift",
		Category: RepairCategoryMarkdownImport,
		Policy:   DiagnosticPolicyWarningDrift,
		Message:  fmt.Sprintf("ephemeral Markdown cutover surface has %d legacy Markdown file(s); .agents/TASKS.json present: %t", markdownFiles, tasksJSONPresent),
		Details:  details,
	}}, nil
}

func countEphemeralMarkdownFiles(agentsPath string) (int, error) {
	total := 0
	for _, root := range []string{"tasks", "ideas", "sparks", "sessions", "brainstorms", "drafts"} {
		dir := filepath.Join(agentsPath, root)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return 0, fmt.Errorf("inspect .agents/%s: %w", root, err)
		}
		if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(entry.Name(), ".md") {
				total++
			}
			return nil
		}); err != nil {
			return 0, fmt.Errorf("inspect .agents/%s: %w", root, err)
		}
	}
	return total, nil
}

func inspectSQLiteIntegrity(ctx context.Context, store *Store) ([]Diagnostic, bool, error) {
	diagnostics := []Diagnostic{}
	valid := true

	checkRows, err := store.db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return nil, false, fmt.Errorf("run SQLite quick_check: %w", err)
	}
	defer checkRows.Close()
	for checkRows.Next() {
		var result string
		if err := checkRows.Scan(&result); err != nil {
			return nil, false, fmt.Errorf("scan SQLite quick_check: %w", err)
		}
		if result != "ok" {
			valid = false
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "error",
				Code:     "sqlite-integrity-check-failed",
				Message:  fmt.Sprintf("SQLite quick_check reported: %s", result),
			})
		}
	}
	if err := checkRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate SQLite quick_check: %w", err)
	}

	foreignKeyRows, err := store.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return nil, false, fmt.Errorf("run SQLite foreign_key_check: %w", err)
	}
	defer foreignKeyRows.Close()
	for foreignKeyRows.Next() {
		var tableName, parentTable string
		var rowID sql.NullInt64
		var foreignKeyID int
		if err := foreignKeyRows.Scan(&tableName, &rowID, &parentTable, &foreignKeyID); err != nil {
			return nil, false, fmt.Errorf("scan SQLite foreign_key_check: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "sqlite-foreign-key-violation",
			Message:  formatSQLiteForeignKeyViolation(tableName, rowID, parentTable, foreignKeyID),
		})
	}
	if err := foreignKeyRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate SQLite foreign_key_check: %w", err)
	}

	return diagnostics, valid, nil
}

func inspectProjectPathInvariants(ctx context.Context, store *Store) ([]Diagnostic, bool, error) {
	diagnostics := []Diagnostic{}
	valid := true

	missingRows, err := store.db.QueryContext(ctx, `
SELECT projects.id
FROM projects
LEFT JOIN project_paths ON project_paths.project_id = projects.id AND project_paths.is_current = 1
WHERE project_paths.id IS NULL
ORDER BY projects.id
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect missing current project paths: %w", err)
	}
	defer missingRows.Close()
	for missingRows.Next() {
		var projectID string
		if err := missingRows.Scan(&projectID); err != nil {
			return nil, false, fmt.Errorf("scan missing current project path: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "project-current-path-missing",
			Message:  fmt.Sprintf("project %s has no current project_paths row", projectID),
		})
	}
	if err := missingRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate missing current project paths: %w", err)
	}

	mismatchRows, err := store.db.QueryContext(ctx, `
SELECT projects.id, COALESCE(projects.current_path, ''), project_paths.path
FROM projects
JOIN project_paths ON project_paths.project_id = projects.id AND project_paths.is_current = 1
WHERE COALESCE(projects.current_path, '') <> project_paths.path
ORDER BY projects.id
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect current project path mismatches: %w", err)
	}
	defer mismatchRows.Close()
	for mismatchRows.Next() {
		var projectID, projectCurrentPath, currentPathRow string
		if err := mismatchRows.Scan(&projectID, &projectCurrentPath, &currentPathRow); err != nil {
			return nil, false, fmt.Errorf("scan current project path mismatch: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "project-current-path-mismatch",
			Message:  fmt.Sprintf("project %s current_path %q does not match current project_paths row %q", projectID, projectCurrentPath, currentPathRow),
		})
	}
	if err := mismatchRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate current project path mismatches: %w", err)
	}

	orphanRows, err := store.db.QueryContext(ctx, `
SELECT project_paths.id, project_paths.path
FROM project_paths
LEFT JOIN projects ON projects.id = project_paths.project_id
WHERE projects.id IS NULL
ORDER BY project_paths.id
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect orphaned project paths: %w", err)
	}
	defer orphanRows.Close()
	for orphanRows.Next() {
		var pathID, path string
		if err := orphanRows.Scan(&pathID, &path); err != nil {
			return nil, false, fmt.Errorf("scan orphaned project path: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "orphaned-project-path",
			Message:  fmt.Sprintf("project path %s at %s references a missing project", pathID, path),
		})
	}
	if err := orphanRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate orphaned project paths: %w", err)
	}

	return diagnostics, valid, nil
}

func inspectRelationshipOriginInvariants(ctx context.Context, store *Store) ([]Diagnostic, error) {
	diagnostics := []Diagnostic{}
	var missingCount int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM relationships
WHERE origin IS NULL OR TRIM(origin) = ''
`).Scan(&missingCount); err != nil {
		return nil, fmt.Errorf("inspect missing relationship origins: %w", err)
	}
	if missingCount > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     "relationship-origin-missing",
			Message:  fmt.Sprintf("%d relationship row(s) are missing provenance origin", missingCount),
		})
	}

	unknownRows, err := store.db.QueryContext(ctx, `
SELECT origin, COUNT(*)
FROM relationships
WHERE origin IS NOT NULL AND TRIM(origin) != '' AND origin NOT IN ('imported', 'manual', 'command')
GROUP BY origin
ORDER BY origin
`)
	if err != nil {
		return nil, fmt.Errorf("inspect unknown relationship origins: %w", err)
	}
	defer unknownRows.Close()
	for unknownRows.Next() {
		var origin string
		var count int
		if err := unknownRows.Scan(&origin, &count); err != nil {
			return nil, fmt.Errorf("scan unknown relationship origin: %w", err)
		}
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     "relationship-origin-unknown",
			Message:  fmt.Sprintf("%d relationship row(s) have unknown provenance origin %q", count, origin),
		})
	}
	if err := unknownRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unknown relationship origins: %w", err)
	}
	return diagnostics, nil
}

func inspectBackendMappingInvariants(ctx context.Context, store *Store) ([]Diagnostic, bool, error) {
	diagnostics := []Diagnostic{}
	valid := true

	// Migration 0012 tables may be absent on behind-schema databases that this
	// scan classifies before an upgrade; their clauses join conditionally.
	var intentTableCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name IN ('intents', 'explorations', 'exploration_checkpoints', 'logical_conversations')`).Scan(&intentTableCount); err != nil {
		return nil, false, fmt.Errorf("inspect intent table presence: %w", err)
	}
	intentTablesPresent := intentTableCount == 4
	intentKindList := ""
	intentOrphanKindList := ""
	intentEntityCTE := ""
	if intentTablesPresent {
		// One kind→table source builds every conditional clause so the three
		// scan sites cannot drift apart.
		intentKinds := [][2]string{
			{"intent", "intents"},
			{"exploration", "explorations"},
			{"exploration_checkpoint", "exploration_checkpoints"},
			{"logical_conversation", "logical_conversations"},
		}
		for _, kind := range intentKinds {
			intentKindList += ",\n  '" + kind[0] + "'"
			intentOrphanKindList += ",\n    '" + kind[0] + "'"
			intentEntityCTE += "\n  UNION ALL SELECT '" + kind[0] + "', project_id, id FROM " + kind[1]
		}
	}

	blankRows, err := store.db.QueryContext(ctx, `
SELECT field, COUNT(*)
FROM (
  SELECT 'backend' AS field FROM backend_mappings WHERE TRIM(backend) = ''
  UNION ALL SELECT 'entity_kind' FROM backend_mappings WHERE TRIM(entity_kind) = ''
  UNION ALL SELECT 'entity_id' FROM backend_mappings WHERE TRIM(entity_id) = ''
  UNION ALL SELECT 'external_kind' FROM backend_mappings WHERE TRIM(external_kind) = ''
  UNION ALL SELECT 'external_id' FROM backend_mappings WHERE TRIM(external_id) = ''
  UNION ALL SELECT 'sync_status' FROM backend_mappings WHERE TRIM(sync_status) = ''
)
GROUP BY field
ORDER BY field
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect blank backend mapping fields: %w", err)
	}
	defer blankRows.Close()
	for blankRows.Next() {
		var field string
		var count int
		if err := blankRows.Scan(&field, &count); err != nil {
			return nil, false, fmt.Errorf("scan blank backend mapping field: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "backend-mapping-field-empty",
			Category: RepairCategoryBackendMapping,
			Policy:   DiagnosticPolicyInvalidLocalData,
			Message:  fmt.Sprintf("%d backend mapping row(s) have an empty %s field; fix or remove the local backend mapping row before trusting integration state", count, field),
			Details: map[string]any{
				"field":     field,
				"row_count": count,
			},
		})
	}
	if err := blankRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate blank backend mapping fields: %w", err)
	}

	sensitiveRows, err := store.db.QueryContext(ctx, `
SELECT field, value
FROM (
  SELECT 'external_id' AS field, external_id AS value FROM backend_mappings
  UNION ALL SELECT 'external_url', COALESCE(external_url, '') FROM backend_mappings
)
ORDER BY field
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect sensitive backend mapping values: %w", err)
	}
	defer sensitiveRows.Close()
	sensitiveCounts := map[string]int{}
	for sensitiveRows.Next() {
		var field, value string
		if err := sensitiveRows.Scan(&field, &value); err != nil {
			return nil, false, fmt.Errorf("scan sensitive backend mapping value: %w", err)
		}
		if backendMappingHasSensitiveValue(value) {
			sensitiveCounts[field]++
		}
	}
	if err := sensitiveRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate sensitive backend mapping values: %w", err)
	}
	for _, field := range []string{"external_id", "external_url"} {
		count := sensitiveCounts[field]
		if count == 0 {
			continue
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "backend-mapping-sensitive-value",
			Category: RepairCategoryBackendMapping,
			Policy:   DiagnosticPolicyInvalidLocalData,
			Message:  fmt.Sprintf("%d backend mapping row(s) contain sensitive-looking %s values; replace them with external record identifiers or URLs before trusting integration state", count, field),
			Details: map[string]any{
				"field":     field,
				"row_count": count,
			},
		})
	}

	unknownRows, err := store.db.QueryContext(ctx, `
SELECT entity_kind, COUNT(*)
FROM backend_mappings
WHERE entity_kind NOT IN (
  'project',
  'alias',
  'artifact_body',
  'spec',
  'task',
  'idea',
  'spark',
  'brainstorm',
  'shaping_draft',
  'report',
  'plan',
  'handoff',
  'council',
  'journal_entry',
  'event',
  'relationship',
  'tag',
  'entity_tag',
  'bundle',
  'bundle_member',
  'source',
  'hook_event',
  'export'`+intentKindList+`
)
GROUP BY entity_kind
ORDER BY entity_kind
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect unknown backend mapping entity kinds: %w", err)
	}
	defer unknownRows.Close()
	for unknownRows.Next() {
		var entityKind string
		var count int
		if err := unknownRows.Scan(&entityKind, &count); err != nil {
			return nil, false, fmt.Errorf("scan unknown backend mapping entity kind: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "backend-mapping-entity-kind-unknown",
			Category: RepairCategoryBackendMapping,
			Policy:   DiagnosticPolicyInvalidLocalData,
			Message:  fmt.Sprintf("%d backend mapping row(s) reference unknown local entity kind %q; fix or remove the local backend mapping row before trusting integration state", count, entityKind),
			Details: map[string]any{
				"entity_kind": entityKind,
				"row_count":   count,
			},
		})
	}
	if err := unknownRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate unknown backend mapping entity kinds: %w", err)
	}

	unknownStatusRows, err := store.db.QueryContext(ctx, `
SELECT sync_status, COUNT(*)
FROM backend_mappings
WHERE TRIM(sync_status) <> ''
  AND sync_status NOT IN ('linked', 'pending', 'stale', 'conflict', 'error')
GROUP BY sync_status
ORDER BY sync_status
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect unknown backend mapping sync statuses: %w", err)
	}
	defer unknownStatusRows.Close()
	for unknownStatusRows.Next() {
		var syncStatus string
		var count int
		if err := unknownStatusRows.Scan(&syncStatus, &count); err != nil {
			return nil, false, fmt.Errorf("scan unknown backend mapping sync status: %w", err)
		}
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     "backend-mapping-sync-status-unknown",
			Category: RepairCategoryBackendMapping,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  fmt.Sprintf("%d backend mapping row(s) have unknown sync_status %q; audit local integration metadata before pruning or reconnecting external records", count, syncStatus),
			Details: map[string]any{
				"row_count":   count,
				"sync_status": syncStatus,
			},
		})
	}
	if err := unknownStatusRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate unknown backend mapping sync statuses: %w", err)
	}

	missingRows, err := store.db.QueryContext(ctx, `
WITH local_entities(entity_kind, project_id, entity_id) AS (
  SELECT 'project', id, id FROM projects
  UNION ALL SELECT 'alias', project_id, id FROM aliases
  UNION ALL SELECT 'artifact_body', project_id, id FROM artifact_bodies
  UNION ALL SELECT 'spec', project_id, id FROM specs
  UNION ALL SELECT 'task', project_id, id FROM tasks
  UNION ALL SELECT 'idea', project_id, id FROM ideas
  UNION ALL SELECT 'spark', project_id, id FROM sparks
  UNION ALL SELECT 'brainstorm', project_id, id FROM brainstorms
  UNION ALL SELECT 'shaping_draft', project_id, id FROM shaping_drafts
  UNION ALL SELECT 'report', project_id, id FROM reports
  UNION ALL SELECT 'plan', project_id, id FROM plans
  UNION ALL SELECT 'handoff', project_id, id FROM handoffs
  UNION ALL SELECT 'council', project_id, id FROM councils
  UNION ALL SELECT 'journal_entry', project_id, id FROM journal_entries
  UNION ALL SELECT 'event', project_id, id FROM events
  UNION ALL SELECT 'relationship', project_id, id FROM relationships
  UNION ALL SELECT 'tag', project_id, id FROM tags
  UNION ALL SELECT 'entity_tag', project_id, id FROM entity_tags
  UNION ALL SELECT 'bundle', project_id, id FROM bundles
  UNION ALL SELECT 'bundle_member', project_id, id FROM bundle_members
  UNION ALL SELECT 'source', project_id, id FROM sources
  UNION ALL SELECT 'hook_event', project_id, id FROM hook_events
  UNION ALL SELECT 'export', project_id, id FROM exports`+intentEntityCTE+`
)
SELECT backend_mappings.id, backend_mappings.backend, backend_mappings.entity_kind, backend_mappings.entity_id, backend_mappings.external_kind, backend_mappings.external_id
FROM backend_mappings
LEFT JOIN local_entities
  ON local_entities.project_id = backend_mappings.project_id
 AND local_entities.entity_kind = backend_mappings.entity_kind
 AND local_entities.entity_id = backend_mappings.entity_id
WHERE local_entities.entity_id IS NULL
  AND backend_mappings.entity_kind IN (
    'project',
    'alias',
    'artifact_body',
    'spec',
    'task',
    'idea',
    'spark',
    'brainstorm',
    'shaping_draft',
    'report',
    'plan',
    'handoff',
    'council',
    'journal_entry',
    'event',
    'relationship',
    'tag',
    'entity_tag',
    'bundle',
    'bundle_member',
    'source',
    'hook_event',
    'export'`+intentOrphanKindList+`
  )
ORDER BY backend_mappings.id
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect orphaned backend mappings: %w", err)
	}
	defer missingRows.Close()
	for missingRows.Next() {
		var mappingID, backend, entityKind, entityID, externalKind, externalID string
		if err := missingRows.Scan(&mappingID, &backend, &entityKind, &entityID, &externalKind, &externalID); err != nil {
			return nil, false, fmt.Errorf("scan orphaned backend mapping: %w", err)
		}
		valid = false
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "error",
			Code:     "backend-mapping-entity-missing",
			Category: RepairCategoryBackendMapping,
			Policy:   DiagnosticPolicyInvalidLocalData,
			Message:  fmt.Sprintf("backend mapping %s links local %s %s to %s %s:%s, but the local entity is missing; fix or remove the local backend mapping row before trusting integration state", mappingID, entityKind, entityID, backend, externalKind, externalID),
			Details: map[string]any{
				"backend":       backend,
				"entity_id":     entityID,
				"entity_kind":   entityKind,
				"external_id":   externalID,
				"external_kind": externalKind,
				"mapping_id":    mappingID,
			},
		})
	}
	if err := missingRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate orphaned backend mappings: %w", err)
	}

	ambiguousRows, err := store.db.QueryContext(ctx, `
SELECT project_id, backend, entity_kind, entity_id, external_kind, COUNT(DISTINCT external_id)
FROM backend_mappings
GROUP BY project_id, backend, entity_kind, entity_id, external_kind
HAVING COUNT(DISTINCT external_id) > 1
ORDER BY project_id, backend, entity_kind, entity_id, external_kind
`)
	if err != nil {
		return nil, false, fmt.Errorf("inspect ambiguous backend mappings: %w", err)
	}
	defer ambiguousRows.Close()
	for ambiguousRows.Next() {
		var projectID, backend, entityKind, entityID, externalKind string
		var count int
		if err := ambiguousRows.Scan(&projectID, &backend, &entityKind, &entityID, &externalKind, &count); err != nil {
			return nil, false, fmt.Errorf("scan ambiguous backend mapping: %w", err)
		}
		diagnostics = append(diagnostics, Diagnostic{
			Severity: "warn",
			Code:     "backend-mapping-entity-ambiguous",
			Category: RepairCategoryBackendMapping,
			Policy:   DiagnosticPolicyWarningDrift,
			Message:  fmt.Sprintf("local %s %s in project %s maps to %d %s %s records; audit local integration metadata before pruning or reconnecting external records", entityKind, entityID, projectID, count, backend, externalKind),
			Details: map[string]any{
				"backend":                    backend,
				"distinct_external_id_count": count,
				"entity_id":                  entityID,
				"entity_kind":                entityKind,
				"external_kind":              externalKind,
				"project_id":                 projectID,
			},
		})
	}
	if err := ambiguousRows.Err(); err != nil {
		return nil, false, fmt.Errorf("iterate ambiguous backend mappings: %w", err)
	}

	return diagnostics, valid, nil
}

func backendMappingHasSensitiveValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	markers := []string{
		"authorization:",
		"bearer ",
		strings.Join([]string{"api", "_", "key", "="}, ""),
		strings.Join([]string{"x-api", "-", "key"}, ""),
		strings.Join([]string{"access_", "tok", "en", "="}, ""),
		strings.Join([]string{"refresh_", "tok", "en", "="}, ""),
		strings.Join([]string{"tok", "en", "="}, ""),
		strings.Join([]string{"pass", "word", "="}, ""),
		strings.Join([]string{"sec", "ret", "="}, ""),
		"ghp_",
		"xoxb-",
		"sk_live_",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func inspectLinearModeTaskMappings(ctx context.Context, root project.Root, store *Store, projectID string) ([]Diagnostic, error) {
	enabled, err := linearIntegrationEnabled(root.Path())
	if err != nil || !enabled {
		return nil, err
	}

	var unmappedCount int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM tasks
LEFT JOIN backend_mappings
  ON backend_mappings.project_id = tasks.project_id
 AND backend_mappings.backend = 'linear'
 AND backend_mappings.entity_kind = 'task'
 AND backend_mappings.entity_id = tasks.id
WHERE tasks.project_id = ?
  AND tasks.status <> 'archived'
  AND backend_mappings.id IS NULL
`, projectID).Scan(&unmappedCount); err != nil {
		return nil, fmt.Errorf("inspect Linear-mode task backend mappings: %w", err)
	}
	if unmappedCount == 0 {
		return nil, nil
	}
	return []Diagnostic{{
		Severity: "warn",
		Code:     "linear-mode-local-task-unmapped",
		Category: RepairCategoryExternalSync,
		Policy:   DiagnosticPolicyExternalSyncGap,
		Message:  fmt.Sprintf("Linear integration is enabled, but %d active local task row(s) have no Linear backend mapping; export local task state and reconcile it through Linear or future backend sync tooling", unmappedCount),
		Details: map[string]any{
			"backend":             "linear",
			"entity_kind":         "task",
			"unmapped_task_count": unmappedCount,
		},
		RequiresExternalSync: true,
	}}, nil
}

func linearIntegrationEnabled(rootPath string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(rootPath, ".agents", "loaf.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read .agents/loaf.json: %w", err)
	}
	var config struct {
		Integrations struct {
			Linear struct {
				Enabled *bool `json:"enabled"`
			} `json:"linear"`
		} `json:"integrations"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("parse .agents/loaf.json: %w", err)
	}
	return config.Integrations.Linear.Enabled != nil && *config.Integrations.Linear.Enabled, nil
}

func inspectStaleExports(ctx context.Context, store *Store) ([]Diagnostic, error) {
	rows, err := store.db.QueryContext(ctx, `
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, specs.updated_at
FROM exports JOIN specs ON exports.source_entity_kind = 'spec' AND specs.project_id = exports.project_id AND specs.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, tasks.updated_at
FROM exports JOIN tasks ON exports.source_entity_kind = 'task' AND tasks.project_id = exports.project_id AND tasks.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, ideas.updated_at
FROM exports JOIN ideas ON exports.source_entity_kind = 'idea' AND ideas.project_id = exports.project_id AND ideas.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, sparks.updated_at
FROM exports JOIN sparks ON exports.source_entity_kind = 'spark' AND sparks.project_id = exports.project_id AND sparks.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, brainstorms.updated_at
FROM exports JOIN brainstorms ON exports.source_entity_kind = 'brainstorm' AND brainstorms.project_id = exports.project_id AND brainstorms.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, shaping_drafts.updated_at
FROM exports JOIN shaping_drafts ON exports.source_entity_kind = 'shaping_draft' AND shaping_drafts.project_id = exports.project_id AND shaping_drafts.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, reports.updated_at
FROM exports JOIN reports ON exports.source_entity_kind = 'report' AND reports.project_id = exports.project_id AND reports.id = exports.source_entity_id
UNION ALL
SELECT exports.id, exports.path, exports.source_entity_kind, exports.source_entity_id, exports.generated_at, journal_entries.updated_at
FROM exports JOIN journal_entries ON exports.source_entity_kind = 'journal_entry' AND journal_entries.project_id = exports.project_id AND journal_entries.id = exports.source_entity_id
`)
	if err != nil {
		return nil, fmt.Errorf("inspect stale exports: %w", err)
	}
	defer rows.Close()

	diagnostics := []Diagnostic{}
	for rows.Next() {
		var id, path, kind, entityID, generatedAt, updatedAt string
		if err := rows.Scan(&id, &path, &kind, &entityID, &generatedAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan stale export: %w", err)
		}
		if updatedAt > generatedAt {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: "warn",
				Code:     "stale-compatibility-export",
				Category: RepairCategoryCompatibilityExport,
				Policy:   DiagnosticPolicyStaleExport,
				Message:  fmt.Sprintf("export %s at %s is stale for %s %s", id, path, kind, entityID),
				Details: map[string]any{
					"export_id":          id,
					"path":               path,
					"source_entity_kind": kind,
					"source_entity_id":   entityID,
					"generated_at":       generatedAt,
					"source_updated_at":  updatedAt,
				},
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale exports: %w", err)
	}
	return diagnostics, nil
}

// classifyBehindSchemaTarget reports whether a database that Inspect marked
// ModeInvalid is invalid *only* because it is behind schema — i.e. it has fewer
// than the current auto-applied migrations, every recorded migration matches its
// Go-owned checksum, no unexpected migration versions are recorded, and SQLite
// integrity and foreign keys are clean. Such a database can be safely brought
// current by applying the pending non-destructive migrations. Any other invalid
// condition (checksum drift, corruption, foreign-key violations, or a version at
// or beyond the current baseline that is still somehow invalid) returns false so
// the caller refuses with a clear error.
func classifyBehindSchemaTarget(databasePath string) (bool, error) {
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return false, nil
	}
	defer store.Close()
	ctx := context.Background()

	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return false, nil
	}
	// Only strictly-behind databases qualify. A version at or beyond the current
	// baseline that is still invalid is a genuine problem, not a pending upgrade.
	if version >= CurrentSchemaVersion() || version < 1 {
		return false, nil
	}

	// Every recorded migration must be a known Go-owned migration with a matching
	// checksum. An unknown version or a drifted checksum is real drift, not a
	// pending upgrade.
	known := map[int]SchemaMigration{}
	for _, migration := range SchemaMigrations() {
		known[migration.Version] = migration
	}
	rows, err := store.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return false, nil
	}
	defer rows.Close()
	recorded := 0
	for rows.Next() {
		var recordedVersion int
		var recordedChecksum string
		if err := rows.Scan(&recordedVersion, &recordedChecksum); err != nil {
			return false, fmt.Errorf("scan schema migration row: %w", err)
		}
		migration, ok := known[recordedVersion]
		if !ok || migration.Checksum() != recordedChecksum {
			return false, nil
		}
		recorded++
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate schema migration rows: %w", err)
	}
	// The recorded rows must be exactly versions 1..version with no gaps, so the
	// pending set is precisely the known migrations above the current version.
	if recorded != version {
		return false, nil
	}

	// SQLite integrity and foreign keys must be clean; behind-schema is about
	// missing migrations, not corruption.
	_, integrityValid, err := inspectSQLiteIntegrity(ctx, store)
	if err != nil {
		return false, fmt.Errorf("classify behind-schema integrity: %w", err)
	}
	if !integrityValid {
		return false, nil
	}
	return true, nil
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
