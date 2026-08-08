package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	AliasOrphanMigrationActionDryRun   = "dry-run"
	AliasOrphanMigrationActionApply    = "apply"
	AliasOrphanMigrationActionRollback = "rollback"

	aliasOrphanMigrationName = "alias-orphans"

	aliasOrphanProofDerivation      = "derivation"
	aliasOrphanProofContentIdentity = "content-identity"
	aliasOrphanProofUnproven        = "unproven"

	aliasOrphanDispositionRetire       = "retire"
	aliasOrphanDispositionRealias      = "realias"
	aliasOrphanDispositionArchiveMoot  = "archive-as-moot"
	aliasOrphanDispositionDeleteDangle = "delete-dangling-alias"

	// brokenEvidenceReportID is the named per-row disposition for the bodyless
	// report whose evidence is unrecoverable (SPEC-047 already shipped the
	// simplification it guarded).
	brokenEvidenceReportID = "report:7644bb23d2664de93b6cb6a5"

	aliasOrphanArchiveMootEventType = "status_normalized"
	aliasOrphanArchiveMootNote      = "evidence unrecoverable; archived as moot — SPEC-047 shipped the simplification this report guarded against deepening"

	// june24EventClusterPrefix matches the 2026-06-24 re-import event cluster
	// used as a content-identity title-match gate.
	june24EventClusterPrefix = "2026-06-24"
)

// AliasOrphanMigrationResult is the preview/apply/rollback outcome for the
// alias-orphan repair migration. Classification spans every project in the
// global database.
type AliasOrphanMigrationResult struct {
	ContractVersion      int                         `json:"contract_version"`
	DatabaseScope        string                      `json:"database_scope"`
	DatabasePath         string                      `json:"database_path"`
	ProjectID            string                      `json:"project_id,omitempty"`
	ProjectName          string                      `json:"project_name,omitempty"`
	ProjectCurrentPath   string                      `json:"project_current_path,omitempty"`
	Action               string                      `json:"action"`
	Applied              bool                        `json:"applied"`
	CopyRun              bool                        `json:"copy_run"`
	BackupPath           string                      `json:"backup_path,omitempty"`
	RollbackManifestPath string                      `json:"rollback_manifest_path,omitempty"`
	Projects             []AliasOrphanProjectSummary `json:"projects"`
	Totals               AliasOrphanCounts           `json:"totals"`
	Dispositions         []AliasOrphanDisposition    `json:"dispositions,omitempty"`
	OperatorFlags        []string                    `json:"operator_flags,omitempty"`
	Warnings             []string                    `json:"warnings,omitempty"`
	RowsRestored         int                         `json:"rows_restored,omitempty"`
}

// AliasOrphanProjectSummary reports classification for one project.
type AliasOrphanProjectSummary struct {
	ProjectID          string                    `json:"project_id"`
	ProjectName        string                    `json:"project_name,omitempty"`
	ProjectCurrentPath string                    `json:"project_current_path,omitempty"`
	LegacyProjectID    string                    `json:"legacy_project_id,omitempty"`
	Tables             []AliasOrphanTableSummary `json:"tables"`
	Counts             AliasOrphanCounts         `json:"counts"`
	Dispositions       []AliasOrphanDisposition  `json:"dispositions,omitempty"`
}

// AliasOrphanTableSummary reports per-entity-table classification counts.
type AliasOrphanTableSummary struct {
	Kind             string                   `json:"kind"`
	Table            string                   `json:"table"`
	Orphans          int                      `json:"orphans"`
	Retire           int                      `json:"retire"`
	Unproven         int                      `json:"unproven"`
	DanglingAliases  int                      `json:"dangling_aliases"`
	OrphanedSources  int                      `json:"orphaned_sources,omitempty"`
	Classifications  []AliasOrphanRowClassify `json:"classifications,omitempty"`
	DanglingAliasIDs []string                 `json:"dangling_alias_ids,omitempty"`
}

// AliasOrphanCounts aggregates migration action counts.
type AliasOrphanCounts struct {
	Orphans           int `json:"orphans"`
	Retire            int `json:"retire"`
	Unproven          int `json:"unproven"`
	DanglingAliases   int `json:"dangling_aliases"`
	OrphanedSources   int `json:"orphaned_sources"`
	NamedDispositions int `json:"named_dispositions,omitempty"`
	OperatorRetire    int `json:"operator_retire,omitempty"`
	OperatorRealias   int `json:"operator_realias,omitempty"`
	EntitiesRetired   int `json:"entities_retired,omitempty"`
	AliasesDeleted    int `json:"aliases_deleted,omitempty"`
	SourcesDeleted    int `json:"sources_deleted,omitempty"`
	StatusesChanged   int `json:"statuses_changed,omitempty"`
	AliasesInserted   int `json:"aliases_inserted,omitempty"`
}

// AliasOrphanRowClassify is one orphan entity's classification.
type AliasOrphanRowClassify struct {
	ProjectID   string `json:"project_id"`
	Kind        string `json:"kind"`
	Table       string `json:"table"`
	EntityID    string `json:"entity_id"`
	Title       string `json:"title,omitempty"`
	Proof       string `json:"proof"`
	TwinID      string `json:"twin_id,omitempty"`
	TwinAlias   string `json:"twin_alias,omitempty"`
	Disposition string `json:"disposition,omitempty"`
}

// AliasOrphanDisposition is a planned action against a specific row.
type AliasOrphanDisposition struct {
	ProjectID string `json:"project_id"`
	Kind      string `json:"kind,omitempty"`
	EntityID  string `json:"entity_id"`
	Action    string `json:"action"`
	Alias     string `json:"alias,omitempty"`
	Proof     string `json:"proof,omitempty"`
	Note      string `json:"note,omitempty"`
	Flag      string `json:"flag,omitempty"`
}

// AliasOrphanApplyOptions carries explicit per-row operator dispositions for apply.
type AliasOrphanApplyOptions struct {
	Retire  []string          // entity IDs
	Realias map[string]string // entity ID → alias
	Flags   []string          // verbatim flag strings for the manifest
}

// AliasOrphanRollbackManifest preserves every deleted/changed row for rollback.
type AliasOrphanRollbackManifest struct {
	ContractVersion      int                       `json:"contract_version"`
	Migration            string                    `json:"migration"`
	CreatedAt            string                    `json:"created_at"`
	DatabaseScope        string                    `json:"database_scope"`
	DatabasePath         string                    `json:"database_path"`
	OperatorFlags        []string                  `json:"operator_flags,omitempty"`
	OperatorDispositions []AliasOrphanDisposition  `json:"operator_dispositions,omitempty"`
	DeletedRows          []AliasOrphanDeletedRow   `json:"deleted_rows"`
	StatusChanges        []AliasOrphanStatusChange `json:"status_changes,omitempty"`
	AliasInserts         []AliasOrphanAliasInsert  `json:"alias_inserts,omitempty"`
	Unlinks              []AliasOrphanUnlink       `json:"unlinks,omitempty"`
	Counts               AliasOrphanCounts         `json:"counts"`
	Metadata             map[string]string         `json:"metadata,omitempty"`
}

// AliasOrphanDeletedRow is one full row snapshot for rollback restore.
type AliasOrphanDeletedRow struct {
	Table   string            `json:"table"`
	Columns []string          `json:"columns"`
	Values  []any             `json:"values"`
	Order   int               `json:"order"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// AliasOrphanStatusChange records a status rewrite for rollback.
type AliasOrphanStatusChange struct {
	ProjectID      string `json:"project_id"`
	Table          string `json:"table"`
	Kind           string `json:"kind"`
	EntityID       string `json:"entity_id"`
	PreviousStatus string `json:"previous_status"`
	NewStatus      string `json:"new_status"`
	EventID        string `json:"event_id"`
	EventNote      string `json:"event_note"`
}

// AliasOrphanAliasInsert records an alias created by --realias for rollback.
type AliasOrphanAliasInsert struct {
	ProjectID  string `json:"project_id"`
	AliasID    string `json:"alias_id"`
	EntityKind string `json:"entity_kind"`
	EntityID   string `json:"entity_id"`
	Namespace  string `json:"namespace"`
	Alias      string `json:"alias"`
}

// AliasOrphanUnlink records a FK null for rollback restore.
type AliasOrphanUnlink struct {
	Table      string `json:"table"`
	ProjectID  string `json:"project_id"`
	Column     string `json:"column"`
	RowID      string `json:"row_id"`
	PreviousID string `json:"previous_id"`
}

type aliasOrphanEntityTable struct {
	kind         string
	table        string
	titleColumn  string
	sourceColumn string
	namespace    string
}

var aliasOrphanEntityTables = []aliasOrphanEntityTable{
	{kind: "task", table: "tasks", titleColumn: "title", sourceColumn: "body_source_id", namespace: "task"},
	{kind: "spec", table: "specs", titleColumn: "title", sourceColumn: "body_source_id", namespace: "spec"},
	{kind: "report", table: "reports", titleColumn: "title", sourceColumn: "body_source_id", namespace: "report"},
	{kind: "idea", table: "ideas", titleColumn: "title", sourceColumn: "body_source_id", namespace: "idea"},
	{kind: "spark", table: "sparks", titleColumn: "text", sourceColumn: "source_id", namespace: "spark"},
	{kind: "brainstorm", table: "brainstorms", titleColumn: "title", sourceColumn: "body_source_id", namespace: "brainstorm"},
}

// PreviewAliasOrphanMigration classifies alias-orphans against a temporary copy.
func PreviewAliasOrphanMigration(ctx context.Context, root project.Root, resolver PathResolver) (AliasOrphanMigrationResult, error) {
	status, err := requireAliasOrphanMigrationStatus(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	source, err := OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer source.Close()

	tempDir, err := os.MkdirTemp("", "loaf-alias-orphan-migration-*")
	if err != nil {
		return AliasOrphanMigrationResult{}, fmt.Errorf("create alias-orphan migration temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	copyPath := filepath.Join(tempDir, "state.sqlite")
	if err := copySQLiteDatabase(ctx, source, copyPath, 0o600); err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	copyStore, err := OpenStore(copyPath)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer copyStore.Close()

	result, _, err := planAliasOrphanMigration(ctx, copyStore, aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionDryRun), AliasOrphanApplyOptions{})
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	result.CopyRun = true
	return result, nil
}

// ApplyAliasOrphanMigration backs up, writes a rollback manifest, and repairs alias-orphans.
func ApplyAliasOrphanMigration(ctx context.Context, root project.Root, resolver PathResolver, options AliasOrphanApplyOptions) (AliasOrphanMigrationResult, error) {
	status, err := requireAliasOrphanMigrationStatus(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer store.Close()

	result, manifest, err := planAliasOrphanMigration(ctx, store, aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionApply), options)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	result.BackupPath = backup.BackupPath
	result.Applied = true
	result.OperatorFlags = append([]string{}, options.Flags...)

	// Apply first so the rollback manifest captures every deleted row snapshot.
	if err := applyAliasOrphanMigrationManifest(ctx, store, &manifest); err != nil {
		return AliasOrphanMigrationResult{}, err
	}

	if aliasOrphanManifestHasWork(manifest) {
		manifestPath, err := writeAliasOrphanRollbackManifest(manifest, filepath.Dir(backup.BackupPath), time.Now().UTC())
		if err != nil {
			return AliasOrphanMigrationResult{}, err
		}
		result.RollbackManifestPath = manifestPath
	}

	verify, _, err := planAliasOrphanMigration(ctx, store, aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionApply), AliasOrphanApplyOptions{})
	if err != nil {
		return AliasOrphanMigrationResult{}, fmt.Errorf("post-apply verification: %w", err)
	}
	if verify.Totals.Retire > 0 || verify.Totals.DanglingAliases > 0 {
		return AliasOrphanMigrationResult{}, fmt.Errorf("post-apply verification failed: %d retire-class orphans and %d dangling aliases remain", verify.Totals.Retire, verify.Totals.DanglingAliases)
	}
	result.Totals.EntitiesRetired = manifest.Counts.EntitiesRetired
	result.Totals.AliasesDeleted = manifest.Counts.AliasesDeleted
	result.Totals.SourcesDeleted = manifest.Counts.SourcesDeleted
	result.Totals.StatusesChanged = manifest.Counts.StatusesChanged
	result.Totals.AliasesInserted = manifest.Counts.AliasesInserted
	return result, nil
}

// RollbackAliasOrphanMigration restores rows recorded in an alias-orphan rollback manifest.
func RollbackAliasOrphanMigration(ctx context.Context, root project.Root, resolver PathResolver, manifestPath string) (AliasOrphanMigrationResult, error) {
	if manifestPath == "" {
		return AliasOrphanMigrationResult{}, fmt.Errorf("alias-orphan rollback requires a manifest path")
	}
	status, err := requireAliasOrphanMigrationStatus(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	manifest, err := readAliasOrphanRollbackManifest(manifestPath)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	store, err := openInitializedStore(root, resolver)
	if err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	defer store.Close()

	result := aliasOrphanMigrationBaseResult(status, AliasOrphanMigrationActionRollback)
	result.Applied = true
	result.BackupPath = backup.BackupPath
	result.RollbackManifestPath = manifestPath
	result.OperatorFlags = append([]string{}, manifest.OperatorFlags...)
	if err := rollbackAliasOrphanMigrationManifest(ctx, store, manifest, &result); err != nil {
		return AliasOrphanMigrationResult{}, err
	}
	return result, nil
}

func aliasOrphanMigrationBaseResult(status Status, action string) AliasOrphanMigrationResult {
	return AliasOrphanMigrationResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		Action:             action,
		Projects:           []AliasOrphanProjectSummary{},
	}
}

func requireAliasOrphanMigrationStatus(root project.Root, resolver PathResolver) (Status, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return Status{}, err
	}
	switch status.Mode {
	case ModeSQLiteReady:
		return status, nil
	case ModeMarkdownOnly:
		return Status{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return Status{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	default:
		return Status{}, fmt.Errorf("state database is not ready; run `loaf state status`")
	}
}

func planAliasOrphanMigration(ctx context.Context, store *Store, result AliasOrphanMigrationResult, options AliasOrphanApplyOptions) (AliasOrphanMigrationResult, AliasOrphanRollbackManifest, error) {
	manifest := AliasOrphanRollbackManifest{
		ContractVersion: StateJSONContractVersion,
		Migration:       aliasOrphanMigrationName,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		DatabaseScope:   result.DatabaseScope,
		DatabasePath:    result.DatabasePath,
		OperatorFlags:   append([]string{}, options.Flags...),
		DeletedRows:     []AliasOrphanDeletedRow{},
		Metadata: map[string]string{
			"broken_evidence_report_id": brokenEvidenceReportID,
		},
	}

	retireSet := map[string]string{}
	for _, id := range options.Retire {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		flag := "--retire " + id
		retireSet[id] = flag
		manifest.OperatorDispositions = append(manifest.OperatorDispositions, AliasOrphanDisposition{
			EntityID: id,
			Action:   aliasOrphanDispositionRetire,
			Flag:     flag,
		})
	}
	realiasSet := map[string]string{}
	for id, alias := range options.Realias {
		id = strings.TrimSpace(id)
		alias = strings.TrimSpace(alias)
		if id == "" || alias == "" {
			continue
		}
		flag := "--realias " + id + "=" + alias
		realiasSet[id] = alias
		manifest.OperatorDispositions = append(manifest.OperatorDispositions, AliasOrphanDisposition{
			EntityID: id,
			Action:   aliasOrphanDispositionRealias,
			Alias:    alias,
			Flag:     flag,
		})
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		return result, manifest, err
	}

	for _, project := range projects.Projects {
		summary, err := classifyAliasOrphansForProject(ctx, store, project, retireSet, realiasSet)
		if err != nil {
			return result, manifest, err
		}
		result.Projects = append(result.Projects, summary)
		result.Totals.Orphans += summary.Counts.Orphans
		result.Totals.Retire += summary.Counts.Retire
		result.Totals.Unproven += summary.Counts.Unproven
		result.Totals.DanglingAliases += summary.Counts.DanglingAliases
		result.Totals.NamedDispositions += summary.Counts.NamedDispositions
		result.Totals.OperatorRetire += summary.Counts.OperatorRetire
		result.Totals.OperatorRealias += summary.Counts.OperatorRealias
		for _, d := range summary.Dispositions {
			result.Dispositions = append(result.Dispositions, d)
		}
	}

	if err := populateAliasOrphanManifestFromPlan(ctx, store, &manifest, result, retireSet, realiasSet); err != nil {
		return result, manifest, err
	}
	return result, manifest, nil
}

func classifyAliasOrphansForProject(ctx context.Context, store *Store, project ProjectIdentity, retireSet map[string]string, realiasSet map[string]string) (AliasOrphanProjectSummary, error) {
	summary := AliasOrphanProjectSummary{
		ProjectID:          project.ID,
		ProjectName:        project.FriendlyName,
		ProjectCurrentPath: project.CurrentPath,
		Tables:             []AliasOrphanTableSummary{},
		Dispositions:       []AliasOrphanDisposition{},
	}
	if project.CurrentPath != "" {
		summary.LegacyProjectID = legacyProjectIDFromPath(project.CurrentPath)
	}

	for _, table := range aliasOrphanEntityTables {
		exists, err := sqliteTableExists(ctx, store.db, table.table)
		if err != nil {
			return summary, err
		}
		if !exists {
			continue
		}
		tableSummary, err := classifyAliasOrphansForTable(ctx, store, project.ID, summary.LegacyProjectID, table, retireSet, realiasSet)
		if err != nil {
			return summary, err
		}
		summary.Tables = append(summary.Tables, tableSummary)
		summary.Counts.Orphans += tableSummary.Orphans
		summary.Counts.Retire += tableSummary.Retire
		summary.Counts.Unproven += tableSummary.Unproven
		summary.Counts.DanglingAliases += tableSummary.DanglingAliases
		for _, c := range tableSummary.Classifications {
			if c.Disposition == aliasOrphanDispositionRetire && (c.Proof == aliasOrphanProofDerivation || c.Proof == aliasOrphanProofContentIdentity) {
				summary.Dispositions = append(summary.Dispositions, AliasOrphanDisposition{
					ProjectID: project.ID,
					Kind:      c.Kind,
					EntityID:  c.EntityID,
					Action:    aliasOrphanDispositionRetire,
					Proof:     c.Proof,
					Note:      c.TwinAlias,
				})
			} else if c.Disposition == aliasOrphanDispositionRetire && c.Proof == aliasOrphanProofUnproven {
				summary.Counts.OperatorRetire++
				summary.Dispositions = append(summary.Dispositions, AliasOrphanDisposition{
					ProjectID: project.ID,
					Kind:      c.Kind,
					EntityID:  c.EntityID,
					Action:    aliasOrphanDispositionRetire,
					Proof:     c.Proof,
					Flag:      retireSet[c.EntityID],
				})
			} else if c.Disposition == aliasOrphanDispositionRealias {
				summary.Counts.OperatorRealias++
				summary.Dispositions = append(summary.Dispositions, AliasOrphanDisposition{
					ProjectID: project.ID,
					Kind:      c.Kind,
					EntityID:  c.EntityID,
					Action:    aliasOrphanDispositionRealias,
					Alias:     realiasSet[c.EntityID],
					Proof:     c.Proof,
					Flag:      "--realias " + c.EntityID + "=" + realiasSet[c.EntityID],
				})
			}
		}
		for _, aliasID := range tableSummary.DanglingAliasIDs {
			summary.Dispositions = append(summary.Dispositions, AliasOrphanDisposition{
				ProjectID: project.ID,
				Kind:      table.kind,
				EntityID:  aliasID,
				Action:    aliasOrphanDispositionDeleteDangle,
			})
		}
	}

	named, err := classifyBrokenEvidenceReport(ctx, store, project.ID)
	if err != nil {
		return summary, err
	}
	if named != nil {
		summary.Dispositions = append(summary.Dispositions, *named)
		summary.Counts.NamedDispositions++
	}
	return summary, nil
}

func classifyAliasOrphansForTable(ctx context.Context, store *Store, projectID string, legacyProjectID string, table aliasOrphanEntityTable, retireSet map[string]string, realiasSet map[string]string) (AliasOrphanTableSummary, error) {
	summary := AliasOrphanTableSummary{
		Kind:            table.kind,
		Table:           table.table,
		Classifications: []AliasOrphanRowClassify{},
	}

	type entityRow struct {
		id        string
		title     string
		createdAt string
	}
	orphanQuery := fmt.Sprintf(`
SELECT e.id, e.%s, e.created_at
FROM %s AS e
WHERE e.project_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM aliases AS a
    WHERE a.project_id = e.project_id
      AND a.entity_kind = ?
      AND a.entity_id = e.id
      AND a.namespace = ?
  )
ORDER BY e.id
`, quoteSQLiteIdentifier(table.titleColumn), quoteSQLiteIdentifier(table.table))

	rows, err := store.db.QueryContext(ctx, orphanQuery, projectID, table.kind, table.namespace)
	if err != nil {
		return summary, fmt.Errorf("scan %s orphans: %w", table.table, err)
	}
	var orphans []entityRow
	for rows.Next() {
		var row entityRow
		if err := rows.Scan(&row.id, &row.title, &row.createdAt); err != nil {
			rows.Close()
			return summary, fmt.Errorf("scan %s orphan row: %w", table.table, err)
		}
		orphans = append(orphans, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return summary, fmt.Errorf("scan %s orphans: %w", table.table, err)
	}
	rows.Close()

	type aliasHolder struct {
		entityID  string
		alias     string
		title     string
		createdAt string
	}
	aliasRows, err := store.db.QueryContext(ctx, fmt.Sprintf(`
SELECT a.entity_id, a.alias, e.%s, e.created_at
FROM aliases AS a
JOIN %s AS e ON e.project_id = a.project_id AND e.id = a.entity_id
WHERE a.project_id = ? AND a.entity_kind = ? AND a.namespace = ?
ORDER BY a.alias
`, quoteSQLiteIdentifier(table.titleColumn), quoteSQLiteIdentifier(table.table)), projectID, table.kind, table.namespace)
	if err != nil {
		return summary, fmt.Errorf("scan %s alias holders: %w", table.table, err)
	}
	holdersByID := map[string]aliasHolder{}
	holdersByTitle := map[string][]aliasHolder{}
	derivedOrphanIDs := map[string]aliasHolder{}
	for aliasRows.Next() {
		var h aliasHolder
		if err := aliasRows.Scan(&h.entityID, &h.alias, &h.title, &h.createdAt); err != nil {
			aliasRows.Close()
			return summary, fmt.Errorf("scan %s alias holder: %w", table.table, err)
		}
		holdersByID[h.entityID] = h
		holdersByTitle[h.title] = append(holdersByTitle[h.title], h)
		if legacyProjectID != "" {
			derived := stableMigrationID(table.kind, legacyProjectID, h.alias)
			if derived != h.entityID {
				derivedOrphanIDs[derived] = h
			}
		}
	}
	if err := aliasRows.Err(); err != nil {
		aliasRows.Close()
		return summary, fmt.Errorf("scan %s alias holders: %w", table.table, err)
	}
	aliasRows.Close()

	summary.Orphans = len(orphans)
	for _, orphan := range orphans {
		classify := AliasOrphanRowClassify{
			ProjectID: projectID,
			Kind:      table.kind,
			Table:     table.table,
			EntityID:  orphan.id,
			Title:     orphan.title,
			Proof:     aliasOrphanProofUnproven,
		}
		if twin, ok := derivedOrphanIDs[orphan.id]; ok {
			classify.Proof = aliasOrphanProofDerivation
			classify.TwinID = twin.entityID
			classify.TwinAlias = twin.alias
			classify.Disposition = aliasOrphanDispositionRetire
			summary.Retire++
		} else if holders := holdersByTitle[orphan.title]; len(holders) == 1 && inJune24EventCluster(orphan.createdAt, holders[0].createdAt) {
			twin := holders[0]
			classify.Proof = aliasOrphanProofContentIdentity
			classify.TwinID = twin.entityID
			classify.TwinAlias = twin.alias
			classify.Disposition = aliasOrphanDispositionRetire
			summary.Retire++
		} else if _, ok := realiasSet[orphan.id]; ok {
			classify.Disposition = aliasOrphanDispositionRealias
			summary.Unproven++
		} else if _, ok := retireSet[orphan.id]; ok {
			classify.Disposition = aliasOrphanDispositionRetire
			summary.Unproven++
		} else {
			summary.Unproven++
		}
		summary.Classifications = append(summary.Classifications, classify)
	}

	danglingRows, err := store.db.QueryContext(ctx, fmt.Sprintf(`
SELECT a.id
FROM aliases AS a
WHERE a.project_id = ?
  AND a.entity_kind = ?
  AND a.namespace = ?
  AND NOT EXISTS (
    SELECT 1 FROM %s AS e
    WHERE e.project_id = a.project_id AND e.id = a.entity_id
  )
ORDER BY a.id
`, quoteSQLiteIdentifier(table.table)), projectID, table.kind, table.namespace)
	if err != nil {
		return summary, fmt.Errorf("scan %s dangling aliases: %w", table.table, err)
	}
	for danglingRows.Next() {
		var aliasID string
		if err := danglingRows.Scan(&aliasID); err != nil {
			danglingRows.Close()
			return summary, fmt.Errorf("scan %s dangling alias: %w", table.table, err)
		}
		summary.DanglingAliasIDs = append(summary.DanglingAliasIDs, aliasID)
	}
	if err := danglingRows.Err(); err != nil {
		danglingRows.Close()
		return summary, fmt.Errorf("scan %s dangling aliases: %w", table.table, err)
	}
	danglingRows.Close()
	summary.DanglingAliases = len(summary.DanglingAliasIDs)

	return summary, nil
}

func classifyBrokenEvidenceReport(ctx context.Context, store *Store, projectID string) (*AliasOrphanDisposition, error) {
	var status string
	err := store.db.QueryRowContext(ctx, `
SELECT status FROM reports WHERE project_id = ? AND id = ?
`, projectID, brokenEvidenceReportID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read broken-evidence report: %w", err)
	}
	if status == LifecycleStatusArchived {
		return nil, nil
	}
	return &AliasOrphanDisposition{
		ProjectID: projectID,
		Kind:      "report",
		EntityID:  brokenEvidenceReportID,
		Action:    aliasOrphanDispositionArchiveMoot,
		Note:      aliasOrphanArchiveMootNote,
	}, nil
}

func inJune24EventCluster(timestamps ...string) bool {
	for _, ts := range timestamps {
		if strings.HasPrefix(ts, june24EventClusterPrefix) {
			return true
		}
	}
	return false
}

func legacyProjectIDFromPath(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}

func populateAliasOrphanManifestFromPlan(ctx context.Context, store *Store, manifest *AliasOrphanRollbackManifest, plan AliasOrphanMigrationResult, retireSet map[string]string, realiasSet map[string]string) error {
	// Manifest is filled at apply time with full row snapshots. Planning only
	// records operator dispositions and high-level counts; apply re-reads rows
	// under the transaction so the snapshot matches the rows actually deleted.
	_ = ctx
	_ = store
	_ = retireSet
	_ = realiasSet
	manifest.Counts = AliasOrphanCounts{
		Orphans:           plan.Totals.Orphans,
		Retire:            plan.Totals.Retire,
		Unproven:          plan.Totals.Unproven,
		DanglingAliases:   plan.Totals.DanglingAliases,
		NamedDispositions: plan.Totals.NamedDispositions,
		OperatorRetire:    plan.Totals.OperatorRetire,
		OperatorRealias:   plan.Totals.OperatorRealias,
	}
	return nil
}

func aliasOrphanManifestHasWork(manifest AliasOrphanRollbackManifest) bool {
	return len(manifest.DeletedRows) > 0 ||
		len(manifest.StatusChanges) > 0 ||
		len(manifest.AliasInserts) > 0 ||
		len(manifest.Unlinks) > 0 ||
		manifest.Counts.EntitiesRetired > 0 ||
		manifest.Counts.AliasesDeleted > 0 ||
		manifest.Counts.SourcesDeleted > 0 ||
		manifest.Counts.StatusesChanged > 0 ||
		manifest.Counts.AliasesInserted > 0
}

func applyAliasOrphanMigrationManifest(ctx context.Context, store *Store, manifest *AliasOrphanRollbackManifest) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alias-orphan migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	order := 0

	// Re-classify inside the transaction against live state and apply.
	projects, err := listProjectsTx(ctx, tx, store.path)
	if err != nil {
		return err
	}

	retireSet := map[string]struct{}{}
	realiasSet := map[string]string{}
	for _, d := range manifest.OperatorDispositions {
		switch d.Action {
		case aliasOrphanDispositionRetire:
			retireSet[d.EntityID] = struct{}{}
		case aliasOrphanDispositionRealias:
			realiasSet[d.EntityID] = d.Alias
		}
	}

	for _, project := range projects {
		legacyID := ""
		if project.CurrentPath != "" {
			legacyID = legacyProjectIDFromPath(project.CurrentPath)
		}

		// Named disposition: archive broken-evidence report as moot.
		if err := applyBrokenEvidenceArchiveTx(ctx, tx, project.ID, now, manifest); err != nil {
			return err
		}

		for _, table := range aliasOrphanEntityTables {
			exists, err := sqliteTableExistsTx(ctx, tx, table.table)
			if err != nil {
				return err
			}
			if !exists {
				continue
			}

			summary, err := classifyAliasOrphansForTableTx(ctx, tx, project.ID, legacyID, table, retireSet, realiasSet)
			if err != nil {
				return err
			}

			for _, c := range summary.Classifications {
				switch c.Disposition {
				case aliasOrphanDispositionRetire:
					if err := retireEntityWithResidueTx(ctx, tx, project.ID, table, c.EntityID, now, manifest, &order); err != nil {
						return err
					}
					manifest.Counts.EntitiesRetired++
				case aliasOrphanDispositionRealias:
					alias := realiasSet[c.EntityID]
					if err := realiasEntityTx(ctx, tx, project.ID, table, c.EntityID, alias, now, manifest); err != nil {
						return err
					}
					manifest.Counts.AliasesInserted++
				}
			}

			for _, aliasID := range summary.DanglingAliasIDs {
				if err := deleteDanglingAliasTx(ctx, tx, project.ID, aliasID, manifest, &order); err != nil {
					return err
				}
				manifest.Counts.AliasesDeleted++
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alias-orphan migration: %w", err)
	}
	return nil
}

func applyBrokenEvidenceArchiveTx(ctx context.Context, tx *sql.Tx, projectID string, now string, manifest *AliasOrphanRollbackManifest) error {
	var previous string
	err := tx.QueryRowContext(ctx, `SELECT status FROM reports WHERE project_id = ? AND id = ?`, projectID, brokenEvidenceReportID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read broken-evidence report status: %w", err)
	}
	if previous == LifecycleStatusArchived {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE reports SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, LifecycleStatusArchived, now, projectID, brokenEvidenceReportID); err != nil {
		return fmt.Errorf("archive broken-evidence report: %w", err)
	}
	eventID := stableMigrationID("event", projectID, "report", brokenEvidenceReportID, aliasOrphanArchiveMootEventType, previous, LifecycleStatusArchived, "moot")
	if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at)
VALUES (?, ?, 'report', ?, ?, ?, ?, ?, ?, ?)
`, eventID, projectID, brokenEvidenceReportID, aliasOrphanArchiveMootEventType, previous, LifecycleStatusArchived, aliasOrphanArchiveMootNote, now, now); err != nil {
		return fmt.Errorf("record broken-evidence archive event: %w", err)
	}
	manifest.StatusChanges = append(manifest.StatusChanges, AliasOrphanStatusChange{
		ProjectID:      projectID,
		Table:          "reports",
		Kind:           "report",
		EntityID:       brokenEvidenceReportID,
		PreviousStatus: previous,
		NewStatus:      LifecycleStatusArchived,
		EventID:        eventID,
		EventNote:      aliasOrphanArchiveMootNote,
	})
	manifest.Counts.StatusesChanged++
	return nil
}

func realiasEntityTx(ctx context.Context, tx *sql.Tx, projectID string, table aliasOrphanEntityTable, entityID string, alias string, now string, manifest *AliasOrphanRollbackManifest) error {
	aliasID := stableMigrationID("alias", projectID, table.namespace, alias)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, namespace, alias) DO UPDATE SET
  entity_kind = excluded.entity_kind,
  entity_id = excluded.entity_id,
  updated_at = excluded.updated_at
`, aliasID, projectID, table.kind, entityID, table.namespace, alias, now, now); err != nil {
		return fmt.Errorf("realias %s %s as %s: %w", table.kind, entityID, alias, err)
	}
	manifest.AliasInserts = append(manifest.AliasInserts, AliasOrphanAliasInsert{
		ProjectID:  projectID,
		AliasID:    aliasID,
		EntityKind: table.kind,
		EntityID:   entityID,
		Namespace:  table.namespace,
		Alias:      alias,
	})
	return nil
}

func deleteDanglingAliasTx(ctx context.Context, tx *sql.Tx, projectID string, aliasID string, manifest *AliasOrphanRollbackManifest, order *int) error {
	if err := captureRowsTx(ctx, tx, "aliases", `SELECT * FROM aliases WHERE project_id = ? AND id = ?`, []any{projectID, aliasID}, manifest, order, nil); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM aliases WHERE project_id = ? AND id = ?`, projectID, aliasID); err != nil {
		return fmt.Errorf("delete dangling alias %s: %w", aliasID, err)
	}
	return nil
}

func retireEntityWithResidueTx(ctx context.Context, tx *sql.Tx, projectID string, table aliasOrphanEntityTable, entityID string, now string, manifest *AliasOrphanRollbackManifest, order *int) error {
	_ = now
	// Capture and delete artifact bodies (FTS included via delete helper after capture).
	if err := captureRowsTx(ctx, tx, "artifact_bodies", `
SELECT * FROM artifact_bodies WHERE project_id = ? AND entity_kind = ? AND entity_id = ?
`, []any{projectID, table.kind, entityID}, manifest, order, nil); err != nil {
		return err
	}
	if _, _, err := deleteArtifactBodiesForEntityTx(ctx, tx, projectID, table.kind, entityID); err != nil {
		return err
	}

	polymorphic := []struct {
		table string
		query string
		args  []any
	}{
		{"events", `SELECT * FROM events WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"entity_tags", `SELECT * FROM entity_tags WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"bundle_members", `SELECT * FROM bundle_members WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"backend_mappings", `SELECT * FROM backend_mappings WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"exports", `SELECT * FROM exports WHERE project_id = ? AND source_entity_kind = ? AND source_entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"relationships", `SELECT * FROM relationships WHERE project_id = ? AND ((from_entity_kind = ? AND from_entity_id = ?) OR (to_entity_kind = ? AND to_entity_id = ?))`, []any{projectID, table.kind, entityID, table.kind, entityID}},
	}
	for _, op := range polymorphic {
		if err := captureRowsTx(ctx, tx, op.table, op.query, op.args, manifest, order, nil); err != nil {
			return err
		}
	}
	deleteOps := []struct {
		table string
		query string
		args  []any
	}{
		{"events", `DELETE FROM events WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"entity_tags", `DELETE FROM entity_tags WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"bundle_members", `DELETE FROM bundle_members WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"backend_mappings", `DELETE FROM backend_mappings WHERE project_id = ? AND entity_kind = ? AND entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"exports", `DELETE FROM exports WHERE project_id = ? AND source_entity_kind = ? AND source_entity_id = ?`, []any{projectID, table.kind, entityID}},
		{"relationships", `DELETE FROM relationships WHERE project_id = ? AND ((from_entity_kind = ? AND from_entity_id = ?) OR (to_entity_kind = ? AND to_entity_id = ?))`, []any{projectID, table.kind, entityID, table.kind, entityID}},
	}
	for _, op := range deleteOps {
		if _, err := execCountTx(ctx, tx, op.query, op.args...); err != nil {
			return fmt.Errorf("delete %s rows for %s %s: %w", op.table, table.kind, entityID, err)
		}
	}

	if err := unlinkReferencesToEntityTx(ctx, tx, projectID, table.kind, entityID, manifest); err != nil {
		return err
	}

	// Capture entity source id before deleting the entity row.
	var bodySourceID sql.NullString
	sourceQuery := fmt.Sprintf(`SELECT %s FROM %s WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(table.sourceColumn), quoteSQLiteIdentifier(table.table))
	if err := tx.QueryRowContext(ctx, sourceQuery, projectID, entityID).Scan(&bodySourceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read %s source: %w", table.kind, err)
	}

	if err := captureRowsTx(ctx, tx, table.table, fmt.Sprintf(`SELECT * FROM %s WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(table.table)), []any{projectID, entityID}, manifest, order, nil); err != nil {
		return err
	}
	if _, err := execCountTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(table.table)), projectID, entityID); err != nil {
		return fmt.Errorf("delete %s row %s: %w", table.kind, entityID, err)
	}

	candidateSources := map[string]struct{}{}
	if bodySourceID.Valid && bodySourceID.String != "" {
		candidateSources[bodySourceID.String] = struct{}{}
	}
	// Also consider sources captured from artifact_bodies for this entity.
	for _, row := range manifest.DeletedRows {
		if row.Table != "artifact_bodies" {
			continue
		}
		if sid := rowValueString(row, "source_id"); sid != "" {
			if rowValueString(row, "entity_id") == entityID && rowValueString(row, "entity_kind") == table.kind {
				candidateSources[sid] = struct{}{}
			}
		}
	}
	for sid := range candidateSources {
		referenced, err := sourceStillReferencedTx(ctx, tx, projectID, sid)
		if err != nil {
			return err
		}
		if referenced {
			continue
		}
		if err := captureRowsTx(ctx, tx, "sources", `SELECT * FROM sources WHERE project_id = ? AND id = ?`, []any{projectID, sid}, manifest, order, nil); err != nil {
			return err
		}
		count, err := execCountTx(ctx, tx, `DELETE FROM sources WHERE project_id = ? AND id = ?`, projectID, sid)
		if err != nil {
			return fmt.Errorf("delete source %s: %w", sid, err)
		}
		manifest.Counts.SourcesDeleted += count
		manifest.Counts.OrphanedSources += count
	}
	return nil
}

func unlinkReferencesToEntityTx(ctx context.Context, tx *sql.Tx, projectID string, kind string, entityID string, manifest *AliasOrphanRollbackManifest) error {
	type unlinkSpec struct {
		table  string
		column string
	}
	var specs []unlinkSpec
	switch kind {
	case "spec":
		specs = []unlinkSpec{
			{"tasks", "spec_id"},
			{"journal_entries", "spec_id"},
			{"plans", "spec_id"},
			{"councils", "spec_id"},
		}
	case "task":
		specs = []unlinkSpec{
			{"journal_entries", "task_id"},
			{"handoffs", "task_id"},
		}
	default:
		return nil
	}
	for _, spec := range specs {
		exists, err := sqliteTableExistsTx(ctx, tx, spec.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT id FROM %s WHERE project_id = ? AND %s = ?`, quoteSQLiteIdentifier(spec.table), quoteSQLiteIdentifier(spec.column)), projectID, entityID)
		if err != nil {
			return fmt.Errorf("list %s.%s unlinks: %w", spec.table, spec.column, err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			manifest.Unlinks = append(manifest.Unlinks, AliasOrphanUnlink{
				Table:      spec.table,
				ProjectID:  projectID,
				Column:     spec.column,
				RowID:      id,
				PreviousID: entityID,
			})
		}
		if len(ids) > 0 {
			if _, err := execCountTx(ctx, tx, fmt.Sprintf(`UPDATE %s SET %s = NULL WHERE project_id = ? AND %s = ?`, quoteSQLiteIdentifier(spec.table), quoteSQLiteIdentifier(spec.column), quoteSQLiteIdentifier(spec.column)), projectID, entityID); err != nil {
				return fmt.Errorf("unlink %s.%s: %w", spec.table, spec.column, err)
			}
		}
	}
	return nil
}

func rollbackAliasOrphanMigrationManifest(ctx context.Context, store *Store, manifest AliasOrphanRollbackManifest, result *AliasOrphanMigrationResult) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin alias-orphan rollback: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return fmt.Errorf("defer foreign keys: %w", err)
	}

	// Undo alias inserts from --realias first.
	for _, insert := range manifest.AliasInserts {
		if _, err := tx.ExecContext(ctx, `DELETE FROM aliases WHERE project_id = ? AND id = ?`, insert.ProjectID, insert.AliasID); err != nil {
			return fmt.Errorf("rollback alias insert %s: %w", insert.AliasID, err)
		}
	}

	// Undo status changes: delete archive events and restore previous status.
	for _, change := range manifest.StatusChanges {
		if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE project_id = ? AND id = ?`, change.ProjectID, change.EventID); err != nil {
			return fmt.Errorf("rollback status event %s: %w", change.EventID, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET status = ? WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(change.Table)), change.PreviousStatus, change.ProjectID, change.EntityID); err != nil {
			return fmt.Errorf("rollback status for %s %s: %w", change.Kind, change.EntityID, err)
		}
	}

	// Restore deleted rows in reverse capture order so parents come before children when needed.
	rows := append([]AliasOrphanDeletedRow{}, manifest.DeletedRows...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Order > rows[j].Order })
	// Prefer restoring entity tables and sources before reference tables? Actually
	// reverse order of deletion: we deleted residue first then entity then sources.
	// Capture order: bodies, polymorphic, entity, sources. Reverse: sources, entity, polymorphic, bodies.
	// Restoring entity before polymorphic is required for FK if not deferred; with defer, any order works at commit.
	// Restore non-archive-event rows; archive events were deleted above via StatusChanges.
	archiveEventIDs := map[string]struct{}{}
	for _, change := range manifest.StatusChanges {
		archiveEventIDs[change.EventID] = struct{}{}
	}
	for _, row := range rows {
		if row.Table == "events" {
			if id := rowValueString(row, "id"); id != "" {
				if _, skip := archiveEventIDs[id]; skip {
					continue
				}
			}
		}
		if err := insertDeletedRowTx(ctx, tx, row); err != nil {
			return err
		}
		if row.Table == "artifact_bodies" {
			if err := restoreArtifactSearchForRowTx(ctx, tx, row); err != nil {
				return err
			}
		}
		result.RowsRestored++
	}

	// Restore unlinked FKs.
	for _, unlink := range manifest.Unlinks {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET %s = ? WHERE project_id = ? AND id = ?`, quoteSQLiteIdentifier(unlink.Table), quoteSQLiteIdentifier(unlink.Column)), unlink.PreviousID, unlink.ProjectID, unlink.RowID); err != nil {
			return fmt.Errorf("restore unlink %s.%s: %w", unlink.Table, unlink.Column, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit alias-orphan rollback: %w", err)
	}
	return nil
}

func restoreArtifactSearchForRowTx(ctx context.Context, tx *sql.Tx, row AliasOrphanDeletedRow) error {
	projectID := rowValueString(row, "project_id")
	entityKind := rowValueString(row, "entity_kind")
	entityID := rowValueString(row, "entity_id")
	bodyKind := rowValueString(row, "body_kind")
	content := rowValueString(row, "content")
	if projectID == "" || entityKind == "" || entityID == "" {
		return nil
	}
	rowID, err := artifactBodyRowID(ctx, tx, projectID, entityKind, entityID, firstNonEmpty(bodyKind, ArtifactBodyKindMarkdown))
	if err != nil {
		return err
	}
	return upsertArtifactSearchTx(ctx, tx, artifactSearchRow{}, false, rowID, projectID, entityKind, entityID, firstNonEmpty(bodyKind, ArtifactBodyKindMarkdown), content)
}

func insertDeletedRowTx(ctx context.Context, tx *sql.Tx, row AliasOrphanDeletedRow) error {
	if len(row.Columns) == 0 || len(row.Columns) != len(row.Values) {
		return fmt.Errorf("deleted row for %s has mismatched columns/values", row.Table)
	}
	quoted := make([]string, len(row.Columns))
	placeholders := make([]string, len(row.Columns))
	args := make([]any, len(row.Values))
	for i, col := range row.Columns {
		quoted[i] = quoteSQLiteIdentifier(col)
		placeholders[i] = "?"
		args[i] = normalizeManifestValue(row.Values[i])
	}
	query := fmt.Sprintf(`INSERT OR REPLACE INTO %s (%s) VALUES (%s)`, quoteSQLiteIdentifier(row.Table), strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("restore row into %s: %w", row.Table, err)
	}
	return nil
}

func normalizeManifestValue(value any) any {
	switch v := value.(type) {
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return v
	}
}

func captureRowsTx(ctx context.Context, tx *sql.Tx, table string, query string, args []any, manifest *AliasOrphanRollbackManifest, order *int, meta map[string]string) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("capture %s rows: %w", table, err)
	}
	defer rows.Close()
	scanned, err := scanRows(rows)
	if err != nil {
		return fmt.Errorf("scan %s rows for capture: %w", table, err)
	}
	for _, row := range scanned {
		columns := make([]string, 0, len(row))
		// Stable column order for deterministic manifests.
		for col := range row {
			columns = append(columns, col)
		}
		sort.Strings(columns)
		values := make([]any, len(columns))
		for i, col := range columns {
			values[i] = row[col]
		}
		*order++
		entry := AliasOrphanDeletedRow{
			Table:   table,
			Columns: columns,
			Values:  values,
			Order:   *order,
		}
		if len(meta) > 0 {
			entry.Meta = meta
		}
		manifest.DeletedRows = append(manifest.DeletedRows, entry)
	}
	return nil
}

func rowValueString(row AliasOrphanDeletedRow, column string) string {
	for i, col := range row.Columns {
		if col == column {
			if i >= len(row.Values) || row.Values[i] == nil {
				return ""
			}
			switch v := row.Values[i].(type) {
			case string:
				return v
			case []byte:
				return string(v)
			default:
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

// Transaction-scoped classification helpers (mirror the non-tx classifiers).

func classifyAliasOrphansForTableTx(ctx context.Context, tx *sql.Tx, projectID string, legacyProjectID string, table aliasOrphanEntityTable, retireSet map[string]struct{}, realiasSet map[string]string) (AliasOrphanTableSummary, error) {
	// Reuse the DB-level classifier by wrapping is awkward; duplicate the SQL
	// against *sql.Tx for transactional consistency at apply time.
	summary := AliasOrphanTableSummary{
		Kind:            table.kind,
		Table:           table.table,
		Classifications: []AliasOrphanRowClassify{},
	}
	type entityRow struct {
		id        string
		title     string
		createdAt string
	}
	orphanQuery := fmt.Sprintf(`
SELECT e.id, e.%s, e.created_at
FROM %s AS e
WHERE e.project_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM aliases AS a
    WHERE a.project_id = e.project_id
      AND a.entity_kind = ?
      AND a.entity_id = e.id
      AND a.namespace = ?
  )
ORDER BY e.id
`, quoteSQLiteIdentifier(table.titleColumn), quoteSQLiteIdentifier(table.table))
	rows, err := tx.QueryContext(ctx, orphanQuery, projectID, table.kind, table.namespace)
	if err != nil {
		return summary, fmt.Errorf("scan %s orphans: %w", table.table, err)
	}
	var orphans []entityRow
	for rows.Next() {
		var row entityRow
		if err := rows.Scan(&row.id, &row.title, &row.createdAt); err != nil {
			rows.Close()
			return summary, err
		}
		orphans = append(orphans, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return summary, err
	}
	rows.Close()

	type aliasHolder struct {
		entityID  string
		alias     string
		title     string
		createdAt string
	}
	aliasRows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT a.entity_id, a.alias, e.%s, e.created_at
FROM aliases AS a
JOIN %s AS e ON e.project_id = a.project_id AND e.id = a.entity_id
WHERE a.project_id = ? AND a.entity_kind = ? AND a.namespace = ?
ORDER BY a.alias
`, quoteSQLiteIdentifier(table.titleColumn), quoteSQLiteIdentifier(table.table)), projectID, table.kind, table.namespace)
	if err != nil {
		return summary, err
	}
	holdersByTitle := map[string][]aliasHolder{}
	derivedOrphanIDs := map[string]aliasHolder{}
	for aliasRows.Next() {
		var h aliasHolder
		if err := aliasRows.Scan(&h.entityID, &h.alias, &h.title, &h.createdAt); err != nil {
			aliasRows.Close()
			return summary, err
		}
		holdersByTitle[h.title] = append(holdersByTitle[h.title], h)
		if legacyProjectID != "" {
			derived := stableMigrationID(table.kind, legacyProjectID, h.alias)
			if derived != h.entityID {
				derivedOrphanIDs[derived] = h
			}
		}
	}
	if err := aliasRows.Err(); err != nil {
		aliasRows.Close()
		return summary, err
	}
	aliasRows.Close()

	summary.Orphans = len(orphans)
	for _, orphan := range orphans {
		classify := AliasOrphanRowClassify{
			ProjectID: projectID,
			Kind:      table.kind,
			Table:     table.table,
			EntityID:  orphan.id,
			Title:     orphan.title,
			Proof:     aliasOrphanProofUnproven,
		}
		if twin, ok := derivedOrphanIDs[orphan.id]; ok {
			classify.Proof = aliasOrphanProofDerivation
			classify.TwinID = twin.entityID
			classify.TwinAlias = twin.alias
			classify.Disposition = aliasOrphanDispositionRetire
			summary.Retire++
		} else if holders := holdersByTitle[orphan.title]; len(holders) == 1 && inJune24EventCluster(orphan.createdAt, holders[0].createdAt) {
			twin := holders[0]
			classify.Proof = aliasOrphanProofContentIdentity
			classify.TwinID = twin.entityID
			classify.TwinAlias = twin.alias
			classify.Disposition = aliasOrphanDispositionRetire
			summary.Retire++
		} else if _, ok := realiasSet[orphan.id]; ok {
			classify.Disposition = aliasOrphanDispositionRealias
			summary.Unproven++
		} else if _, ok := retireSet[orphan.id]; ok {
			classify.Disposition = aliasOrphanDispositionRetire
			summary.Unproven++
		} else {
			summary.Unproven++
		}
		summary.Classifications = append(summary.Classifications, classify)
	}

	danglingRows, err := tx.QueryContext(ctx, fmt.Sprintf(`
SELECT a.id
FROM aliases AS a
WHERE a.project_id = ?
  AND a.entity_kind = ?
  AND a.namespace = ?
  AND NOT EXISTS (
    SELECT 1 FROM %s AS e
    WHERE e.project_id = a.project_id AND e.id = a.entity_id
  )
ORDER BY a.id
`, quoteSQLiteIdentifier(table.table)), projectID, table.kind, table.namespace)
	if err != nil {
		return summary, err
	}
	for danglingRows.Next() {
		var aliasID string
		if err := danglingRows.Scan(&aliasID); err != nil {
			danglingRows.Close()
			return summary, err
		}
		summary.DanglingAliasIDs = append(summary.DanglingAliasIDs, aliasID)
	}
	if err := danglingRows.Err(); err != nil {
		danglingRows.Close()
		return summary, err
	}
	danglingRows.Close()
	summary.DanglingAliases = len(summary.DanglingAliasIDs)
	return summary, nil
}

func listProjectsTx(ctx context.Context, tx *sql.Tx, databasePath string) ([]ProjectIdentity, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  projects.id,
  COALESCE(NULLIF(projects.friendly_name, ''), projects.id),
  COALESCE(current_path.path, projects.current_path, ''),
  COALESCE(projects.last_seen_at, '')
FROM projects
LEFT JOIN project_paths AS current_path
  ON current_path.project_id = projects.id
 AND current_path.is_current = 1
ORDER BY lower(COALESCE(NULLIF(projects.friendly_name, ''), projects.id)), projects.id
`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var projects []ProjectIdentity
	for rows.Next() {
		identity := ProjectIdentity{ContractVersion: StateJSONContractVersion, DatabaseScope: "global", DatabasePath: databasePath}
		if err := rows.Scan(&identity.ID, &identity.FriendlyName, &identity.CurrentPath, &identity.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan project identity: %w", err)
		}
		projects = append(projects, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

func sqliteTableExistsTx(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var name string
	err := tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("inspect table %s: %w", table, err)
}

func writeAliasOrphanRollbackManifest(manifest AliasOrphanRollbackManifest, dir string, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create alias-orphan rollback manifest directory: %w", err)
	}
	for i := 0; i < 100; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%02d", i)
		}
		path := filepath.Join(dir, fmt.Sprintf("alias-orphan-rollback-%s%s.json", now.Format("20060102T150405Z"), suffix))
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("stat alias-orphan rollback manifest: %w", err)
		}
		payload, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return "", fmt.Errorf("encode alias-orphan rollback manifest: %w", err)
		}
		payload = append(payload, '\n')
		if err := os.WriteFile(path, payload, 0o600); err != nil {
			return "", fmt.Errorf("write alias-orphan rollback manifest: %w", err)
		}
		return path, nil
	}
	return "", fmt.Errorf("create alias-orphan rollback manifest: exhausted timestamp suffixes")
}

func readAliasOrphanRollbackManifest(path string) (AliasOrphanRollbackManifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return AliasOrphanRollbackManifest{}, fmt.Errorf("read alias-orphan rollback manifest: %w", err)
	}
	var manifest AliasOrphanRollbackManifest
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.UseNumber()
	if err := dec.Decode(&manifest); err != nil {
		return AliasOrphanRollbackManifest{}, fmt.Errorf("decode alias-orphan rollback manifest: %w", err)
	}
	if manifest.Migration != aliasOrphanMigrationName {
		return AliasOrphanRollbackManifest{}, fmt.Errorf("rollback manifest migration %q is not %s", manifest.Migration, aliasOrphanMigrationName)
	}
	return manifest, nil
}
