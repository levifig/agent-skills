package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestInspectReportsMarkdownOnlyWithoutCreatingFiles(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeMarkdownOnly {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeMarkdownOnly)
	}
	if status.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", status.DatabaseScope)
	}
	if status.DatabaseExists {
		t.Fatal("DatabaseExists = true, want false")
	}
	if status.DatabaseParentExists {
		t.Fatal("DatabaseParentExists = true, want false before init")
	}
	if status.ProjectID != "" {
		t.Fatalf("ProjectID = %q, want empty before SQLite records durable identity", status.ProjectID)
	}
	if status.LegacyProjectKey != ProjectID(root) {
		t.Fatalf("LegacyProjectKey = %q, want %q", status.LegacyProjectKey, ProjectID(root))
	}
	if _, err := os.Stat(filepath.Dir(status.DatabasePath)); !os.IsNotExist(err) {
		t.Fatalf("database parent exists after Inspect(); err = %v", err)
	}
	assertDiagnostic(t, status.Diagnostics, "database-missing")
	assertDiagnostic(t, status.Diagnostics, "markdown-fallback-active")
}

func TestInspectReportsLegacyStateDatabaseWhenDataHomeIsMissing(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})

	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeMarkdownOnly {
		t.Fatalf("Mode = %q, want %q before storage-home migration", status.Mode, ModeMarkdownOnly)
	}
	if status.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", status.DatabaseScope)
	}
	if status.DatabaseExists {
		t.Fatal("DatabaseExists = true, want false for new data home before migration")
	}
	if !status.LegacyDatabaseExists {
		t.Fatal("LegacyDatabaseExists = false, want true")
	}
	if status.LegacyDatabasePath != legacyPath {
		t.Fatalf("LegacyDatabasePath = %q, want %q", status.LegacyDatabasePath, legacyPath)
	}
	if !strings.HasPrefix(status.DatabasePath, dataHome+string(filepath.Separator)) {
		t.Fatalf("DatabasePath = %q, want under XDG_DATA_HOME %q", status.DatabasePath, dataHome)
	}
	assertDiagnostic(t, status.Diagnostics, "legacy-state-database-detected")
}

func TestRepairPlanRecommendsSafeInitializationForMissingDatabase(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	plan := RepairPlanForStatus(status)
	action := findRepairAction(t, plan, "initialize-database")
	if !action.Safe {
		t.Fatalf("initialize action Safe = false, want true")
	}
	if action.Applied {
		t.Fatalf("initialize action Applied = true, want false for dry-run plan")
	}
	if action.Command != "loaf state doctor --fix" {
		t.Fatalf("initialize action Command = %q, want doctor --fix", action.Command)
	}
	if action.Category != RepairCategoryLocalDatabase {
		t.Fatalf("initialize action Category = %q, want %q", action.Category, RepairCategoryLocalDatabase)
	}
	if action.Path != status.DatabasePath {
		t.Fatalf("initialize action Path = %q, want %q", action.Path, status.DatabasePath)
	}
}

func TestRepairPlanTreatsLegacyLeftoverAsManualReview(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	if _, err := Initialize(context.Background(), root, PathResolver{}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertDiagnostic(t, status.Diagnostics, "legacy-project-database-leftover")

	action := findRepairAction(t, RepairPlanForStatus(status), "review-legacy-project-database")
	if action.Safe {
		t.Fatal("legacy leftover action Safe = true, want manual review")
	}
	if action.Applied {
		t.Fatal("legacy leftover action Applied = true, want false")
	}
	if action.Command != "loaf state repair legacy-project-database --dry-run --json" {
		t.Fatalf("legacy leftover action Command = %q, want legacy archive dry-run", action.Command)
	}
	if action.Category != RepairCategoryLocalDatabase {
		t.Fatalf("legacy leftover action Category = %q, want %q", action.Category, RepairCategoryLocalDatabase)
	}
	if action.Path != legacyPath {
		t.Fatalf("legacy leftover action Path = %q, want %q", action.Path, legacyPath)
	}
}

func TestRepairPlanDeduplicatesRepeatedActions(t *testing.T) {
	status := Status{
		DatabasePath: "/tmp/loaf.sqlite",
		Diagnostics: []Diagnostic{
			{Severity: "error", Code: "backend-mapping-entity-missing", Message: "first missing backend mapping"},
			{Severity: "error", Code: "backend-mapping-entity-missing", Message: "second missing backend mapping"},
		},
	}

	actions := RepairPlanForStatus(status)
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1: %#v", len(actions), actions)
	}
	if actions[0].Code != "inspect-backend-mappings" {
		t.Fatalf("action Code = %q, want inspect-backend-mappings", actions[0].Code)
	}
	if actions[0].Category != RepairCategoryBackendMapping {
		t.Fatalf("action Category = %q, want %q", actions[0].Category, RepairCategoryBackendMapping)
	}
}

func TestRepairPlanPreservesDistinctDiagnosticActions(t *testing.T) {
	status := Status{
		DatabasePath: "/tmp/loaf.sqlite",
		Diagnostics: []Diagnostic{
			{Severity: "error", Code: "backend-mapping-entity-missing", Message: "missing backend mapping"},
			{Severity: "warn", Code: "backend-mapping-entity-ambiguous", Message: "ambiguous backend mapping"},
		},
	}

	actions := RepairPlanForStatus(status)
	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want 2: %#v", len(actions), actions)
	}
	if actions[0].DiagnosticCode == actions[1].DiagnosticCode {
		t.Fatalf("diagnostic codes should remain distinct: %#v", actions)
	}
}

func TestRepairPlanClassifiesBackendAndExternalSyncActions(t *testing.T) {
	status := Status{
		DatabasePath: "/tmp/loaf.sqlite",
		Diagnostics: []Diagnostic{
			{Severity: "error", Code: "backend-mapping-entity-missing", Message: "missing backend mapping"},
			{Severity: "warn", Code: "backend-mapping-sync-status-unknown", Message: "unknown sync status"},
			{Severity: "warn", Code: "linear-mode-local-task-unmapped", Message: "unmapped local task"},
		},
	}

	actions := RepairPlanForStatus(status)
	invalidMapping := findRepairAction(t, actions, "inspect-backend-mappings")
	if invalidMapping.Category != RepairCategoryBackendMapping || invalidMapping.RequiresExternalSync {
		t.Fatalf("invalid mapping action = %#v, want local backend-mapping audit", invalidMapping)
	}
	driftMapping := findRepairAction(t, actions, "audit-backend-mappings")
	if driftMapping.Category != RepairCategoryBackendMapping || driftMapping.RequiresExternalSync {
		t.Fatalf("drift mapping action = %#v, want local backend-mapping audit", driftMapping)
	}
	linearSync := findRepairAction(t, actions, "reconcile-linear-task-mappings")
	if linearSync.Category != RepairCategoryExternalSync || !linearSync.RequiresExternalSync {
		t.Fatalf("linear sync action = %#v, want external sync requirement", linearSync)
	}
}

func TestInspectReportsSQLiteReadyWhenDatabaseIsInitialized(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	initialized, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	if status.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", status.ContractVersion, StateJSONContractVersion)
	}
	if status.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", status.DatabaseScope)
	}
	if status.DatabasePath != initialized.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", status.DatabasePath, initialized.DatabasePath)
	}
	if !status.DatabaseExists {
		t.Fatal("DatabaseExists = false, want true")
	}
	if !status.DatabaseParentExists {
		t.Fatal("DatabaseParentExists = false, want true")
	}
	if status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentSchemaVersion())
	}
	if status.ProjectID == "" {
		t.Fatal("ProjectID is empty after SQLite records durable identity")
	}
	if status.ProjectID == ProjectID(root) {
		t.Fatalf("ProjectID = legacy path key %q, want generated durable identity", status.ProjectID)
	}
	if status.LegacyProjectKey != ProjectID(root) {
		t.Fatalf("LegacyProjectKey = %q, want %q", status.LegacyProjectKey, ProjectID(root))
	}
	if status.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want folder name", status.ProjectName)
	}
	if status.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", status.ProjectCurrentPath, root.Path())
	}
	assertDiagnostic(t, status.Diagnostics, "sqlite-ready")
}

func TestInspectWarnsWhenGlobalDatabaseHasNotImportedCurrentMarkdown(t *testing.T) {
	registeredRoot := projectRoot(t)
	unimportedRoot := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), registeredRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	writeAgentsFile(t, unimportedRoot.Path(), "reports/local.md", `---
title: Local Markdown Report
status: final
---
# Local Markdown Report
`)

	status, err := Inspect(unimportedRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect(unimported) error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "local-markdown-not-imported")
	if !strings.Contains(diagnostic.Message, "1 importable artifact") || !strings.Contains(diagnostic.Message, "loaf state migrate markdown --dry-run") {
		t.Fatalf("diagnostic Message = %q, want import guidance", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "local-markdown-not-imported", RepairCategoryMarkdownImport, DiagnosticPolicyImportPending, false)
	assertDiagnosticDetail(t, status.Diagnostics, "local-markdown-not-imported", "importable_count", 1)
	assertDiagnosticDetail(t, status.Diagnostics, "local-markdown-not-imported", "reports", 1)
	assertDiagnosticDetail(t, status.Diagnostics, "local-markdown-not-imported", "preview_command", "loaf state migrate markdown --dry-run")
	action := findRepairAction(t, RepairPlanForStatus(status), "migrate-current-project-markdown")
	if action.Command != "loaf state migrate markdown --dry-run" || !action.Safe {
		t.Fatalf("repair action = %#v, want safe markdown migration preview", action)
	}

	if _, err := ApplyMarkdownMigration(context.Background(), unimportedRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	migrated, err := Inspect(unimportedRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect(migrated) error = %v", err)
	}
	assertNoDiagnostic(t, migrated.Diagnostics, "local-markdown-not-imported")
}

func TestInspectWarnsWhenInitializedProjectHasUnimportedMarkdown(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-local.md", `---
title: Local Markdown Task
status: todo
---
# Local Markdown Task
`)
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "local-markdown-not-imported")
	if !strings.Contains(diagnostic.Message, "1 importable artifact") {
		t.Fatalf("diagnostic Message = %q, want local artifact count", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "local-markdown-not-imported", RepairCategoryMarkdownImport, DiagnosticPolicyImportPending, false)
	assertDiagnosticDetail(t, status.Diagnostics, "local-markdown-not-imported", "tasks", 1)
}

func TestInspectReportsEphemeralMarkdownCutoverSurface(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertDiagnostic(t, status.Diagnostics, "ephemeral-markdown-cutover-clear")
	assertDiagnosticDetail(t, status.Diagnostics, "ephemeral-markdown-cutover-clear", "ephemeral_markdown_files", 0)
	assertDiagnosticDetail(t, status.Diagnostics, "ephemeral-markdown-cutover-clear", "tasks_json_present", false)

	writeAgentsFile(t, root.Path(), "sessions/20260528-session.md", "# Session\n")
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	drifted, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect(drifted) error = %v", err)
	}
	assertDiagnosticPolicy(t, drifted.Diagnostics, "ephemeral-markdown-cutover-drift", RepairCategoryMarkdownImport, DiagnosticPolicyWarningDrift, false)
	assertDiagnosticDetail(t, drifted.Diagnostics, "ephemeral-markdown-cutover-drift", "ephemeral_markdown_files", 1)
	assertDiagnosticDetail(t, drifted.Diagnostics, "ephemeral-markdown-cutover-drift", "tasks_json_present", true)
}

func TestInspectReportsInvalidWhenDatabaseFileIsNotSQLite(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "database-open-failed")
}

func TestInspectReportsInvalidSchemaVersionMismatch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	_, err := store.db.ExecContext(context.Background(), `
INSERT INTO schema_migrations (version, name, checksum, applied_at)
VALUES (99, 'future_schema', 'future', '2026-05-28T10:00:00Z')
`)
	if err != nil {
		t.Fatalf("insert future schema migration error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "schema-version-mismatch")
}

func TestInspectReportsInvalidSchemaChecksumMismatch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	if _, err := store.db.ExecContext(context.Background(), `UPDATE schema_migrations SET checksum = 'drifted' WHERE version = 1`); err != nil {
		t.Fatalf("drift schema migration checksum error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "schema-checksum-mismatch")
}

func TestInspectReportsStaleCompatibilityExportsAsWarnings(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	_, err := store.db.ExecContext(context.Background(), `
INSERT INTO ideas (id, project_id, title, status, created_at, updated_at)
VALUES ('idea-stale-export', ?, 'Stale Export Idea', 'open', '2026-05-28T10:00:00Z', '2026-05-28T12:00:00Z');
`, projectID)
	if err != nil {
		t.Fatalf("insert stale export idea fixture error = %v", err)
	}
	_, err = store.db.ExecContext(context.Background(), `
INSERT INTO exports (id, project_id, export_kind, format, path, state_version, source_entity_kind, source_entity_id, generated_at, created_at, updated_at)
VALUES ('export-stale', ?, 'triage', 'markdown', '.agents/exports/triage.md', 1, 'idea', 'idea-stale-export', '2026-05-28T11:00:00Z', '2026-05-28T11:00:00Z', '2026-05-28T11:00:00Z');
`, projectID)
	if err != nil {
		t.Fatalf("insert stale export fixture error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q despite stale export warning", status.Mode, ModeSQLiteReady)
	}
	assertDiagnostic(t, status.Diagnostics, "sqlite-ready")
	assertDiagnosticPolicy(t, status.Diagnostics, "stale-compatibility-export", RepairCategoryCompatibilityExport, DiagnosticPolicyStaleExport, false)
	assertDiagnosticDetail(t, status.Diagnostics, "stale-compatibility-export", "export_id", "export-stale")
	assertDiagnosticDetail(t, status.Diagnostics, "stale-compatibility-export", "source_entity_kind", "idea")
	assertDiagnosticDetail(t, status.Diagnostics, "stale-compatibility-export", "source_entity_id", "idea-stale-export")
}

func TestInspectReportsInvalidProjectPathInvariants(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
UPDATE projects
SET current_path = ?
WHERE id = ?
`, filepath.Join(root.Path(), "stale"), projectID); err != nil {
		t.Fatalf("drift project current_path error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "project-current-path-mismatch")

	action := findRepairAction(t, RepairPlanForStatus(status), "repair-project-path-invariants")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual project path repair")
	}
	if action.Command != "loaf project list --json" {
		t.Fatalf("repair action Command = %q, want project list", action.Command)
	}
}

func TestInspectReportsSQLiteForeignKeyViolations(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-orphaned-project', 'project-missing', 'task', 'task-missing', 'task', 'TASK-MISSING', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`); err != nil {
		t.Fatalf("insert orphaned alias fixture error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "sqlite-foreign-key-violation")

	action := findRepairAction(t, RepairPlanForStatus(status), "inspect-state-invariants")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual integrity inspection")
	}
	if action.Command != "loaf state doctor --json" {
		t.Fatalf("repair action Command = %q, want state doctor JSON inspection", action.Command)
	}
}

func TestInspectUsesReadOnlyConnection(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	var journalMode string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA journal_mode = DELETE`).Scan(&journalMode); err != nil {
		t.Fatalf("set rollback journal mode error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	removeSQLiteSidecars(t, status.DatabasePath)
	if err := os.Chmod(status.DatabasePath, 0o400); err != nil {
		t.Fatalf("chmod database read-only error = %v", err)
	}
	defer os.Chmod(status.DatabasePath, 0o600)
	databaseDir := filepath.Dir(status.DatabasePath)
	if err := os.Chmod(databaseDir, 0o500); err != nil {
		t.Fatalf("chmod database directory read-only error = %v", err)
	}
	defer os.Chmod(databaseDir, 0o700)

	inspected, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspected.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", inspected.Mode, ModeSQLiteReady)
	}
}

func TestInspectReportsMissingRelationshipOriginAsWarning(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES ('relationship-without-origin', ?, 'task', 'task-one', 'spec', 'spec-one', 'implements', 'legacy row', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert relationship without origin error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q for relationship provenance warning", status.Mode, ModeSQLiteReady)
	}
	assertDiagnostic(t, status.Diagnostics, "relationship-origin-missing")

	action := findRepairAction(t, RepairPlanForStatus(status), "audit-relationship-origin")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual relationship audit")
	}
	if action.Command != "loaf state repair relationship-origin --origin imported --dry-run --json" {
		t.Fatalf("repair action Command = %q, want relationship origin repair dry-run", action.Command)
	}
}

func TestInspectReportsInvalidBackendMappingMissingEntity(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-orphaned', ?, 'linear', 'task', 'task-missing', 'issue', 'ENG-123', 'https://linear.app/workspace/issue/ENG-123', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert orphaned backend mapping error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "backend-mapping-entity-missing")
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-entity-missing", RepairCategoryBackendMapping, DiagnosticPolicyInvalidLocalData, false)
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-missing", "mapping_id", "backend-mapping-orphaned")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-missing", "entity_kind", "task")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-missing", "entity_id", "task-missing")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-missing", "external_id", "ENG-123")

	action := findRepairAction(t, RepairPlanForStatus(status), "inspect-backend-mappings")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual backend mapping audit")
	}
	if action.Command != "loaf state doctor --json" {
		t.Fatalf("repair action Command = %q, want state doctor JSON", action.Command)
	}
	if action.Category != RepairCategoryBackendMapping || action.RequiresExternalSync {
		t.Fatalf("repair action = %#v, want local backend mapping inspection", action)
	}
}

func TestInspectReportsInvalidBackendMappingEmptyFields(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES ('task-linear-empty-field', ?, NULL, 'Linear task with empty mapping field', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert task fixture error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-empty-field', ?, 'linear', 'task', 'task-linear-empty-field', 'issue', '   ', NULL, 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert empty-field backend mapping error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "backend-mapping-field-empty")
	if !strings.Contains(diagnostic.Message, "external_id") {
		t.Fatalf("diagnostic Message = %q, want field name", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-field-empty", RepairCategoryBackendMapping, DiagnosticPolicyInvalidLocalData, false)
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-field-empty", "field", "external_id")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-field-empty", "row_count", 1)

	action := findRepairAction(t, RepairPlanForStatus(status), "inspect-backend-mappings")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual backend mapping audit")
	}
	if action.Command != "loaf state doctor --json" {
		t.Fatalf("repair action Command = %q, want state doctor JSON", action.Command)
	}
	if action.Category != RepairCategoryBackendMapping || action.RequiresExternalSync {
		t.Fatalf("repair action = %#v, want local backend mapping inspection", action)
	}
}

func TestInspectReportsSensitiveBackendMappingValues(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES ('task-linear-sensitive', ?, NULL, 'Linear task with sensitive mapping value', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert task fixture error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-sensitive-value', ?, 'linear', 'task', 'task-linear-sensitive', 'issue', 'ENG-130', 'https://linear.app/workspace/issue/ENG-130?access_token=REDACTED', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert sensitive backend mapping error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "backend-mapping-sensitive-value")
	if !strings.Contains(diagnostic.Message, "external_url") {
		t.Fatalf("diagnostic Message = %q, want field name", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-sensitive-value", RepairCategoryBackendMapping, DiagnosticPolicyInvalidLocalData, false)
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-sensitive-value", "field", "external_url")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-sensitive-value", "row_count", 1)

	action := findRepairAction(t, RepairPlanForStatus(status), "inspect-backend-mappings")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual backend mapping audit")
	}
	if action.Category != RepairCategoryBackendMapping || action.RequiresExternalSync {
		t.Fatalf("repair action = %#v, want local backend mapping inspection", action)
	}
}

func TestInspectReportsUnknownBackendMappingEntityKind(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-unknown-kind', ?, 'linear', 'milestone', 'milestone-one', 'project_milestone', 'milestone-123', NULL, 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert unknown-kind backend mapping error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "backend-mapping-entity-kind-unknown")
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-entity-kind-unknown", RepairCategoryBackendMapping, DiagnosticPolicyInvalidLocalData, false)
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-kind-unknown", "entity_kind", "milestone")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-kind-unknown", "row_count", 1)
	assertNoDiagnostic(t, status.Diagnostics, "backend-mapping-entity-missing")
}

func TestInspectAcceptsProjectBackendMapping(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-linear-project', ?, 'linear', 'project', ?, 'project', 'LIN-PROJ-123', 'https://linear.app/workspace/project/LIN-PROJ-123', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID, projectID); err != nil {
		t.Fatalf("insert project backend mapping error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q for valid project backend mapping", status.Mode, ModeSQLiteReady)
	}
	assertNoDiagnostic(t, status.Diagnostics, "backend-mapping-entity-kind-unknown")
	assertNoDiagnostic(t, status.Diagnostics, "backend-mapping-entity-missing")
}

func TestInspectAcceptsNewArtifactEntityBackendMappings(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	now := "2026-06-24T13:00:00Z"
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO specs (id, project_id, title, status, body_source_id, created_at, updated_at)
VALUES ('spec-artifact-entities', ?, 'Artifact entities', 'drafting', NULL, ?, ?)
`, projectID, now, now); err != nil {
		t.Fatalf("insert spec fixture error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES ('task-artifact-entities', ?, 'spec-artifact-entities', 'todo', 'todo', 'P2', NULL, ?, ?)
`, projectID, now, now); err != nil {
		t.Fatalf("insert task fixture error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at)
VALUES ('session-artifact-entities', ?, NULL, NULL, 'active', NULL, ?, ?)
`, projectID, now, now); err != nil {
		t.Fatalf("insert session fixture error = %v", err)
	}
	fixtures := []struct {
		table string
		query string
		args  []any
	}{
		{
			table: "artifact_bodies",
			query: `INSERT INTO artifact_bodies (id, project_id, entity_kind, entity_id, body_kind, content, content_hash, source_id, created_at, updated_at)
VALUES ('artifact-body-one', ?, 'spec', 'spec-artifact-entities', 'markdown', 'body text', 'hash-body', NULL, ?, ?)`,
			args: []any{projectID, now, now},
		},
		{
			table: "plans",
			query: `INSERT INTO plans (id, project_id, spec_id, title, status, body_source_id, created_at, updated_at)
VALUES ('plan-one', ?, 'spec-artifact-entities', 'Plan one', 'draft', NULL, ?, ?)`,
			args: []any{projectID, now, now},
		},
		{
			table: "handoffs",
			query: `INSERT INTO handoffs (id, project_id, session_id, task_id, title, status, body_source_id, created_at, updated_at)
VALUES ('handoff-one', ?, 'session-artifact-entities', 'task-artifact-entities', 'Handoff one', 'final', NULL, ?, ?)`,
			args: []any{projectID, now, now},
		},
		{
			table: "councils",
			query: `INSERT INTO councils (id, project_id, spec_id, title, status, body_source_id, created_at, updated_at)
VALUES ('council-one', ?, 'spec-artifact-entities', 'Council one', 'archived', NULL, ?, ?)`,
			args: []any{projectID, now, now},
		},
	}
	for _, fixture := range fixtures {
		if _, err := store.db.ExecContext(context.Background(), fixture.query, fixture.args...); err != nil {
			t.Fatalf("insert %s fixture error = %v", fixture.table, err)
		}
	}
	for _, mapping := range []struct {
		id         string
		entityKind string
		entityID   string
	}{
		{"backend-mapping-artifact-body", "artifact_body", "artifact-body-one"},
		{"backend-mapping-plan", "plan", "plan-one"},
		{"backend-mapping-handoff", "handoff", "handoff-one"},
		{"backend-mapping-council", "council", "council-one"},
	} {
		if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES (?, ?, 'linear', ?, ?, 'artifact', ?, NULL, 'linked', ?, ?)
`, mapping.id, projectID, mapping.entityKind, mapping.entityID, "EXT-"+mapping.entityID, now, now); err != nil {
			t.Fatalf("insert backend mapping %s error = %v", mapping.id, err)
		}
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q for new artifact entity backend mappings", status.Mode, ModeSQLiteReady)
	}
	assertNoDiagnostic(t, status.Diagnostics, "backend-mapping-entity-kind-unknown")
	assertNoDiagnostic(t, status.Diagnostics, "backend-mapping-entity-missing")
}

func TestInspectRejectsProjectBackendMappingToDifferentProjectID(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-wrong-project', ?, 'linear', 'project', 'project-missing', 'project', 'LIN-PROJ-124', 'https://linear.app/workspace/project/LIN-PROJ-124', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert wrong project backend mapping error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q for mismatched project backend mapping", status.Mode, ModeInvalid)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "backend-mapping-entity-missing")
	if !strings.Contains(diagnostic.Message, "project project-missing") {
		t.Fatalf("diagnostic Message = %q, want project entity reference", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-entity-missing", RepairCategoryBackendMapping, DiagnosticPolicyInvalidLocalData, false)
}

func TestInspectReportsAmbiguousBackendMappingAsWarning(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES ('task-linear', ?, NULL, 'Linear-backed task', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert task fixture error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES
  ('backend-mapping-linear-one', ?, 'linear', 'task', 'task-linear', 'issue', 'ENG-123', 'https://linear.app/workspace/issue/ENG-123', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z'),
  ('backend-mapping-linear-two', ?, 'linear', 'task', 'task-linear', 'issue', 'ENG-124', 'https://linear.app/workspace/issue/ENG-124', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID, projectID); err != nil {
		t.Fatalf("insert ambiguous backend mapping fixtures error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q for ambiguous backend mapping warning", status.Mode, ModeSQLiteReady)
	}
	assertDiagnostic(t, status.Diagnostics, "backend-mapping-entity-ambiguous")
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-entity-ambiguous", RepairCategoryBackendMapping, DiagnosticPolicyWarningDrift, false)
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-ambiguous", "entity_kind", "task")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-ambiguous", "entity_id", "task-linear")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-entity-ambiguous", "distinct_external_id_count", 2)

	action := findRepairAction(t, RepairPlanForStatus(status), "audit-backend-mappings")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual backend mapping audit")
	}
	if action.Category != RepairCategoryBackendMapping || action.RequiresExternalSync {
		t.Fatalf("repair action = %#v, want local backend mapping audit", action)
	}
}

func TestInspectWarnsOnUnknownBackendMappingSyncStatus(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES
  ('task-linear-linked', ?, NULL, 'Linear linked task', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z'),
  ('task-linear-typo', ?, NULL, 'Linear typo task', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID, projectID); err != nil {
		t.Fatalf("insert task fixtures error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES
  ('backend-mapping-linear-linked', ?, 'linear', 'task', 'task-linear-linked', 'issue', 'ENG-125', 'https://linear.app/workspace/issue/ENG-125', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z'),
  ('backend-mapping-linear-typo', ?, 'linear', 'task', 'task-linear-typo', 'issue', 'ENG-126', 'https://linear.app/workspace/issue/ENG-126', 'lnked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID, projectID); err != nil {
		t.Fatalf("insert backend mapping fixtures error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q for unknown sync status warning", status.Mode, ModeSQLiteReady)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "backend-mapping-sync-status-unknown")
	if !strings.Contains(diagnostic.Message, "lnked") {
		t.Fatalf("diagnostic Message = %q, want unknown status value", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "backend-mapping-sync-status-unknown", RepairCategoryBackendMapping, DiagnosticPolicyWarningDrift, false)
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-sync-status-unknown", "sync_status", "lnked")
	assertDiagnosticDetail(t, status.Diagnostics, "backend-mapping-sync-status-unknown", "row_count", 1)

	action := findRepairAction(t, RepairPlanForStatus(status), "audit-backend-mappings")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual backend mapping audit")
	}
	if action.Category != RepairCategoryBackendMapping || action.RequiresExternalSync {
		t.Fatalf("repair action = %#v, want local backend mapping audit", action)
	}
}

func TestInspectWarnsOnUnmappedLocalTasksWhenLinearEnabled(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root.Path(), ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root.Path(), ".agents", "loaf.json"), []byte(`{"integrations":{"linear":{"enabled":true}}}`+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(loaf.json) error = %v", err)
	}
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at)
VALUES
  ('task-active-unmapped', ?, NULL, 'Active unmapped task', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z'),
  ('task-archived-unmapped', ?, NULL, 'Archived unmapped task', 'archived', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z'),
  ('task-active-mapped', ?, NULL, 'Active mapped task', 'todo', 'P2', NULL, '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID, projectID, projectID); err != nil {
		t.Fatalf("insert task fixtures error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, external_url, sync_status, created_at, updated_at)
VALUES ('backend-mapping-linear-task', ?, 'linear', 'task', 'task-active-mapped', 'issue', 'ENG-125', 'https://linear.app/workspace/issue/ENG-125', 'linked', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert mapped task backend fixture error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q for Linear-mode local task warning", status.Mode, ModeSQLiteReady)
	}
	diagnostic := findDiagnostic(t, status.Diagnostics, "linear-mode-local-task-unmapped")
	if !strings.Contains(diagnostic.Message, "1 active local task row") {
		t.Fatalf("diagnostic Message = %q, want count of only active unmapped tasks", diagnostic.Message)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, "linear-mode-local-task-unmapped", RepairCategoryExternalSync, DiagnosticPolicyExternalSyncGap, true)
	assertDiagnosticDetail(t, status.Diagnostics, "linear-mode-local-task-unmapped", "backend", "linear")
	assertDiagnosticDetail(t, status.Diagnostics, "linear-mode-local-task-unmapped", "entity_kind", "task")
	assertDiagnosticDetail(t, status.Diagnostics, "linear-mode-local-task-unmapped", "unmapped_task_count", 1)

	action := findRepairAction(t, RepairPlanForStatus(status), "reconcile-linear-task-mappings")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual Linear mapping reconciliation")
	}
	if action.Command != "loaf state export all --format json" {
		t.Fatalf("repair action Command = %q, want export all JSON", action.Command)
	}
	if action.Category != RepairCategoryExternalSync || !action.RequiresExternalSync {
		t.Fatalf("repair action = %#v, want external Linear sync requirement", action)
	}
}

func TestInspectReportsInvalidWhenOperationalInvariantsAreUnreadable(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	if _, err := store.db.ExecContext(context.Background(), `DROP TABLE relationships`); err != nil {
		t.Fatalf("drop relationships error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "state-invariants-unreadable")

	action := findRepairAction(t, RepairPlanForStatus(status), "inspect-state-invariants")
	if action.Safe {
		t.Fatalf("repair action Safe = true, want manual invariant inspection")
	}
	if action.Command != "loaf state doctor --json" {
		t.Fatalf("repair action Command = %q, want state doctor JSON inspection", action.Command)
	}
}

func TestInspectReportsInvalidEmptyFileWithoutOpeningSQLite(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "database-file-empty")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("empty file size = %d, want 0 after Inspect", info.Size())
	}
}

func TestInspectReportsInvalidWhenDatabasePathIsDirectory(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "database-path-is-directory")
}

func projectRoot(t *testing.T) project.Root {
	t.Helper()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	return root
}

func openTestStore(t *testing.T, root project.Root, stateHome string) *Store {
	t.Helper()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func projectIDForTest(t *testing.T, store *Store, root project.Root) string {
	t.Helper()
	identity, err := store.ProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ProjectIdentityForRoot() error = %v", err)
	}
	return identity.ID
}

// assertSessionProjectContext validates the shared project-context envelope
// carried by journal read/write results.
func assertSessionProjectContext(t *testing.T, root project.Root, contractVersion int, databaseScope string, databasePath string, projectID string, projectName string, projectCurrentPath string) {
	t.Helper()
	if contractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", contractVersion, StateJSONContractVersion)
	}
	if databaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", databaseScope)
	}
	if databasePath == "" {
		t.Fatal("DatabasePath is empty")
	}
	if projectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if projectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", projectName, filepath.Base(root.Path()))
	}
	if projectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", projectCurrentPath, root.Path())
	}
}

func assertDiagnostic(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	_ = findDiagnostic(t, diagnostics, code)
}

func assertDiagnosticPolicy(t *testing.T, diagnostics []Diagnostic, code string, category string, policy string, requiresExternalSync bool) {
	t.Helper()
	diagnostic := findDiagnostic(t, diagnostics, code)
	if diagnostic.Category != category || diagnostic.Policy != policy || diagnostic.RequiresExternalSync != requiresExternalSync {
		t.Fatalf("diagnostic %q = %#v, want category %q policy %q requiresExternalSync %v", code, diagnostic, category, policy, requiresExternalSync)
	}
}

func assertDiagnosticDetail(t *testing.T, diagnostics []Diagnostic, code string, key string, want any) {
	t.Helper()
	diagnostic := findDiagnostic(t, diagnostics, code)
	got, ok := diagnostic.Details[key]
	if !ok {
		t.Fatalf("diagnostic %q details = %#v, want key %q", code, diagnostic.Details, key)
	}
	if got != want {
		t.Fatalf("diagnostic %q details[%q] = %#v, want %#v", code, key, got, want)
	}
}

func findDiagnostic(t *testing.T, diagnostics []Diagnostic, code string) Diagnostic {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return diagnostic
		}
	}
	t.Fatalf("diagnostic %q not found in %#v", code, diagnostics)
	return Diagnostic{}
}

func assertNoDiagnostic(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			t.Fatalf("diagnostic %q found in %#v", code, diagnostics)
		}
	}
}

func removeSQLiteSidecars(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove SQLite sidecar %s error = %v", sidecar, err)
		}
	}
}

func findRepairAction(t *testing.T, actions []RepairAction, code string) RepairAction {
	t.Helper()
	for _, action := range actions {
		if action.Code == code {
			return action
		}
	}
	t.Fatalf("repair action %q not found in %#v", code, actions)
	return RepairAction{}
}
