package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	sqlite3 "github.com/ncruces/go-sqlite3"

	"github.com/levifig/loaf/internal/project"
)

const (
	StorageHomeActionCopy            = "copy-legacy-to-data"
	StorageHomeActionMerge           = "merge-legacy-to-data"
	StorageHomeActionAlreadyMigrated = "already-migrated"
	StorageHomeActionNoLegacyState   = "no-legacy-state"
)

var projectScopedMergeTables = []string{
	"sources",
	"specs",
	"tasks",
	"ideas",
	"sparks",
	"brainstorms",
	"shaping_drafts",
	"sessions",
	"reports",
	"journal_entries",
	"events",
	"relationships",
	"tags",
	"entity_tags",
	"bundles",
	"bundle_members",
	"aliases",
	"backend_mappings",
	"hook_events",
	"exports",
	"session_state_snapshots",
}

// StorageHomeMigrationPlan describes the XDG_STATE_HOME to XDG_DATA_HOME move.
type StorageHomeMigrationPlan struct {
	ContractVersion      int      `json:"contract_version"`
	Version              int      `json:"version"`
	DatabaseScope        string   `json:"database_scope"`
	MigrationScope       string   `json:"migration_scope"`
	ProjectRoot          string   `json:"project_root"`
	ProjectID            string   `json:"project_id,omitempty"`
	ProjectName          string   `json:"project_name,omitempty"`
	ProjectCurrentPath   string   `json:"project_current_path,omitempty"`
	DatabasePath         string   `json:"database_path"`
	LegacyDatabasePath   string   `json:"legacy_database_path"`
	DatabaseExists       bool     `json:"database_exists"`
	LegacyDatabaseExists bool     `json:"legacy_database_exists"`
	Action               string   `json:"action"`
	Applied              bool     `json:"applied"`
	Warnings             []string `json:"warnings,omitempty"`
}

// PreviewStorageHomeMigration reports whether an old XDG_STATE_HOME database
// can be copied into the durable XDG_DATA_HOME location.
func PreviewStorageHomeMigration(root project.Root, resolver PathResolver) (StorageHomeMigrationPlan, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	legacyPath, err := migrationSourceDatabasePath(root, resolver)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}

	plan := StorageHomeMigrationPlan{
		ContractVersion:    StateJSONContractVersion,
		Version:            1,
		DatabaseScope:      "global",
		MigrationScope:     "project",
		ProjectRoot:        root.Path(),
		DatabasePath:       databasePath,
		LegacyDatabasePath: legacyPath,
	}
	if databasePath == legacyPath {
		plan.Action = StorageHomeActionAlreadyMigrated
		return plan, nil
	}

	plan.DatabaseExists = regularFileExists(databasePath)
	plan.LegacyDatabaseExists = regularFileExists(legacyPath)
	if plan.DatabaseExists {
		if status, err := Inspect(root, resolver); err == nil && status.Mode == ModeSQLiteReady {
			plan.recordVerifiedProject(status)
		}
	}
	switch {
	case plan.DatabaseExists && plan.LegacyDatabaseExists:
		if databaseContainsRootProject(databasePath, root) {
			plan.Action = StorageHomeActionAlreadyMigrated
			plan.Warnings = append(plan.Warnings, "legacy project database remains after migration; leaving it untouched")
		} else {
			plan.Action = StorageHomeActionMerge
		}
	case plan.DatabaseExists:
		plan.Action = StorageHomeActionAlreadyMigrated
	case plan.LegacyDatabaseExists:
		plan.Action = StorageHomeActionCopy
	default:
		plan.Action = StorageHomeActionNoLegacyState
	}
	return plan, nil
}

// ApplyStorageHomeMigration copies a legacy XDG_STATE_HOME database to the
// XDG_DATA_HOME location without deleting or rewriting the legacy file.
func ApplyStorageHomeMigration(ctx context.Context, root project.Root, resolver PathResolver) (StorageHomeMigrationPlan, error) {
	return applyStorageHomeMigrationWithOps(ctx, root, resolver, nil)
}

type storageHomeMigrationCopyOps struct {
	afterCopy     func(string) error
	beforePublish func(string) error
	publish       func(string, string) (bool, error)
}

func applyStorageHomeMigrationWithOps(ctx context.Context, root project.Root, resolver PathResolver, ops *storageHomeMigrationCopyOps) (StorageHomeMigrationPlan, error) {
	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if plan.Action != StorageHomeActionCopy {
		if plan.Action == StorageHomeActionMerge {
			return ApplyProjectDatabaseMerge(ctx, root, resolver, plan)
		}
		return plan, nil
	}

	legacyStore, err := OpenStoreReadOnly(plan.LegacyDatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open legacy state database: %w", err)
	}
	if err := verifyStorageHomeSource(ctx, legacyStore); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("verify legacy state database: %w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("verify legacy state database: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(plan.DatabasePath), 0o700); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("create data state directory: %w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("create data state directory: %w", err)
	}
	stagingPath, stagingInfo, err := reserveStorageHomeStaging(plan.DatabasePath)
	if err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("%w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, err
	}
	stagingOwned := true
	defer func() {
		if stagingOwned {
			cleanupOwnedStorageHomeStaging(stagingPath, stagingInfo)
		}
	}()
	if err := copySQLiteDatabaseToReserved(ctx, legacyStore, stagingPath); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("%w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, err
	}
	if err := legacyStore.Close(); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("close legacy state database: %w", err)
	}
	if ops != nil && ops.afterCopy != nil {
		if err := ops.afterCopy(stagingPath); err != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("after staging state database copy: %w", err)
		}
	}

	copiedStore, err := OpenStore(stagingPath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open staging state database: %w", err)
	}
	if err := copiedStore.ApplyMigrations(ctx); err != nil {
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("upgrade copied state database: %w; close copied state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("upgrade copied state database: %w", err)
	}
	if err := copiedStore.UpsertProject(ctx, root); err != nil {
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("record copied state project: %w; close copied state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("record copied state project: %w", err)
	}
	rebuildTx, err := copiedStore.db.BeginTx(ctx, nil)
	if err != nil {
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("begin copied journal search rebuild: %w; close copied state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("begin copied journal search rebuild: %w", err)
	}
	if _, err := rebuildAndVerifyJournalSearch(ctx, rebuildTx); err != nil {
		_ = rebuildTx.Rollback()
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("rebuild copied journal search: %w; close copied state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("rebuild copied journal search: %w", err)
	}
	if err := rebuildTx.Commit(); err != nil {
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("commit copied journal search rebuild: %w; close copied state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("commit copied journal search rebuild: %w", err)
	}
	identity, err := verifyStorageHomeDestination(ctx, root, stagingPath)
	if err != nil {
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("verify staging state database: %w; close staging state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("verify staging state database: %w", err)
	}
	if err := checkpointStorageHomeStaging(ctx, copiedStore); err != nil {
		if closeErr := copiedStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("checkpoint staging state database: %w; close staging state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("checkpoint staging state database: %w", err)
	}
	if err := copiedStore.Close(); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("close staging state database: %w", err)
	}
	if err := ensureStorageHomeStagingSidecarsClean(stagingPath); err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if err := syncStorageHomeFile(stagingPath); err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if ops != nil && ops.beforePublish != nil {
		if err := ops.beforePublish(stagingPath); err != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("before staging state database publish: %w", err)
		}
	}
	publish := publishStorageHomeStaging
	if ops != nil && ops.publish != nil {
		publish = ops.publish
	}
	published, err := publish(stagingPath, plan.DatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if published {
		stagingOwned = false
	} else {
		identity, err = verifyStorageHomeDestination(ctx, root, plan.DatabasePath)
		if err != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("validate concurrently published state database: %w", err)
		}
	}
	plan.ProjectID = identity.ID
	plan.ProjectName = identity.FriendlyName
	plan.ProjectCurrentPath = identity.CurrentPath
	plan.Applied = true
	plan.DatabaseExists = true
	plan.LegacyDatabaseExists = true
	plan.Action = StorageHomeActionAlreadyMigrated
	plan.Warnings = append(plan.Warnings, "legacy project database left untouched; remove it manually after verifying the global database")
	return plan, nil
}

// ApplyProjectDatabaseMerge copies the current project's rows from a legacy
// project-sharded database into the global state database.
func ApplyProjectDatabaseMerge(ctx context.Context, root project.Root, resolver PathResolver, plan StorageHomeMigrationPlan) (StorageHomeMigrationPlan, error) {
	return applyProjectDatabaseMergeWithOps(ctx, root, resolver, plan, nil)
}

type projectDatabaseMergeOps struct {
	beforeCommit func(context.Context, *sql.Tx) error
}

func applyProjectDatabaseMergeWithOps(ctx context.Context, root project.Root, resolver PathResolver, plan StorageHomeMigrationPlan, ops *projectDatabaseMergeOps) (StorageHomeMigrationPlan, error) {
	store, err := OpenStore(plan.DatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open global state database: %w", err)
	}
	defer store.Close()
	if err := requireCurrentSchemaForDerivedRepair(ctx, store); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("global state database is not current: %w", err)
	}
	if err := store.mergeProjectDatabaseWithOps(ctx, plan.LegacyDatabasePath, root, ops); err != nil {
		return StorageHomeMigrationPlan{}, err
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if status.Mode != ModeSQLiteReady {
		return StorageHomeMigrationPlan{}, fmt.Errorf("global state database is not ready: %s", status.Mode)
	}
	plan.recordVerifiedProject(status)
	plan.Applied = true
	plan.DatabaseExists = true
	plan.LegacyDatabaseExists = true
	plan.Action = StorageHomeActionAlreadyMigrated
	plan.Warnings = append(plan.Warnings, "legacy project database left untouched; remove it manually after verifying the global database")
	return plan, nil
}

func (p *StorageHomeMigrationPlan) recordVerifiedProject(status Status) {
	p.ProjectID = status.ProjectID
	p.ProjectName = status.ProjectName
	p.ProjectCurrentPath = status.ProjectCurrentPath
}

func (s *Store) mergeProjectDatabase(ctx context.Context, sourcePath string, root project.Root) error {
	return s.mergeProjectDatabaseWithOps(ctx, sourcePath, root, nil)
}

func (s *Store) mergeProjectDatabaseWithOps(ctx context.Context, sourcePath string, root project.Root, ops *projectDatabaseMergeOps) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project database merge: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `ATTACH DATABASE ? AS legacy`, sourcePath); err != nil {
		return fmt.Errorf("attach legacy project database: %w", err)
	}
	projectID, err := resolveAttachedProjectID(ctx, tx, root)
	if err != nil {
		return err
	}

	if err := copyProjectRow(ctx, tx, projectID); err != nil {
		return err
	}
	if err := copyProjectPathRows(ctx, tx, projectID); err != nil {
		return err
	}
	for _, table := range projectScopedMergeTables {
		if err := copyProjectScopedRows(ctx, tx, table, projectID); err != nil {
			return err
		}
	}
	if err := copyProjectProvenanceRows(ctx, tx, projectID); err != nil {
		return err
	}
	if err := registerMergedProjectTx(ctx, tx, root, projectID); err != nil {
		return err
	}
	provenance, err := inspectJournalProvenanceIntegrity(ctx, tx)
	if err != nil {
		return fmt.Errorf("inspect journal provenance after project database merge: %w", err)
	}
	if !provenance.Ready {
		return fmt.Errorf("journal provenance after project database merge is not ready: %#v", provenance)
	}
	if _, err := rebuildAndVerifyJournalSearch(ctx, tx); err != nil {
		return fmt.Errorf("rebuild journal search after project database merge: %w", err)
	}
	if ops != nil && ops.beforeCommit != nil {
		if err := ops.beforeCommit(ctx, tx); err != nil {
			return fmt.Errorf("project database merge before commit: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project database merge: %w", err)
	}
	return nil
}

func copyProjectProvenanceRows(ctx context.Context, tx mergeExecer, projectID string) error {
	if !legacyTableExists(ctx, tx, "journal_origins") {
		if err := backfillMergedJournalOrigins(ctx, tx, projectID); err != nil {
			return err
		}
	}
	for _, table := range []string{"journal_origins", "journal_deferrals"} {
		if !legacyTableExists(ctx, tx, table) {
			continue
		}
		columns := tableColumns(ctx, tx, "legacy", table)
		expected := provenanceTableColumns(table)
		if !sameColumnSet(columns, expected) {
			return fmt.Errorf("legacy %s table has incompatible columns", table)
		}
		columnList := quotedColumnList(expected)
		conflictKeys := `m.journal_entry_id = l.journal_entry_id`
		if table == "journal_deferrals" {
			conflictKeys = `(m.project_id = l.project_id AND m.operation_key = l.operation_key) OR m.journal_entry_id = l.journal_entry_id OR m.spark_id = l.spark_id`
		}
		comparisons := make([]string, 0, len(expected))
		for _, column := range expected {
			quoted := quoteSQLiteIdentifier(column)
			comparisons = append(comparisons, fmt.Sprintf("m.%s IS NOT l.%s", quoted, quoted))
		}
		var conflicts int
		query := fmt.Sprintf(`
SELECT COUNT(*)
FROM main.%s AS m
JOIN legacy.%s AS l ON %s
WHERE l.project_id = ? AND (%s)`, quoteSQLiteIdentifier(table), quoteSQLiteIdentifier(table), conflictKeys, strings.Join(comparisons, " OR "))
		if err := tx.QueryRowContext(ctx, query, projectID).Scan(&conflicts); err != nil {
			return fmt.Errorf("inspect %s merge conflicts: %w", table, err)
		}
		if conflicts != 0 {
			return fmt.Errorf("refusing to overwrite conflicting %s provenance rows: %d", table, conflicts)
		}
		query = fmt.Sprintf(`INSERT OR IGNORE INTO %s (%s) SELECT %s FROM legacy.%s WHERE project_id = ?`, quoteSQLiteIdentifier(table), columnList, columnList, quoteSQLiteIdentifier(table))
		if _, err := tx.ExecContext(ctx, query, projectID); err != nil {
			return fmt.Errorf("merge %s rows: %w", table, err)
		}
	}
	return nil
}

func backfillMergedJournalOrigins(ctx context.Context, tx mergeExecer, projectID string) error {
	if !legacyTableExists(ctx, tx, "journal_entries") {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO journal_origins (
  journal_entry_id, project_id, envelope_version, capture_mechanism,
  observed_harness, observed_harness_version, harness_session_id, agent_id,
  source_event, branch, worktree, head, change_path, change_sha256,
  dirty, reconstructable, durable_result_kind, durable_result_id, created_at
)
SELECT
  j.id, j.project_id, 1, 'unknown',
  NULL, NULL, j.harness_session_id, NULL,
  NULL, j.observed_branch, j.observed_worktree, NULL, NULL, NULL,
  NULL, NULL, NULL, NULL, j.created_at
FROM journal_entries AS j
WHERE j.project_id = ?
  AND EXISTS (
    SELECT 1 FROM legacy.journal_entries AS l
    WHERE l.id = j.id AND l.project_id = ?
  )
  AND NOT EXISTS (
    SELECT 1 FROM journal_origins AS o
    WHERE o.journal_entry_id = j.id
  )`, projectID, projectID)
	if err != nil {
		return fmt.Errorf("backfill unknown journal origins after project database merge: %w", err)
	}
	return nil
}

func provenanceTableColumns(table string) []string {
	if table == "journal_origins" {
		return []string{"journal_entry_id", "project_id", "envelope_version", "capture_mechanism", "observed_harness", "observed_harness_version", "harness_session_id", "agent_id", "source_event", "branch", "worktree", "head", "change_path", "change_sha256", "dirty", "reconstructable", "durable_result_kind", "durable_result_id", "created_at"}
	}
	return []string{"project_id", "operation_key", "journal_entry_id", "spark_id", "stored_digest", "created_at"}
}

func sameColumnSet(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	seen := make(map[string]bool, len(actual))
	for _, column := range actual {
		seen[column] = true
	}
	for _, column := range expected {
		if !seen[column] {
			return false
		}
	}
	return true
}

type mergeExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func copyProjectRow(ctx context.Context, tx mergeExecer, projectID string) error {
	if !legacyTableExists(ctx, tx, "projects") {
		return fmt.Errorf("merge project row: legacy projects table is missing")
	}
	columns := sharedColumns(ctx, tx, "projects")
	if len(columns) == 0 {
		return fmt.Errorf("merge project row: no shared project columns")
	}
	primaryKey := tablePrimaryKeyColumns(ctx, tx, "main", "projects")
	if len(primaryKey) == 0 || !columnsContain(columns, primaryKey) {
		return fmt.Errorf("merge project row: projects primary key is unavailable")
	}
	columnList := quotedColumnList(columns)
	comparisons := columnDifferencePredicates(projectIdentityComparableColumns(columns), "m", "l")
	join := primaryKeyJoinPredicate(primaryKey, "m", "l")
	var conflicts int
	query := fmt.Sprintf(`
SELECT COUNT(*)
FROM main.projects AS m
JOIN legacy.projects AS l ON %s
WHERE l.id = ? AND (%s)`, join, strings.Join(comparisons, " OR "))
	if err := tx.QueryRowContext(ctx, query, projectID).Scan(&conflicts); err != nil {
		return fmt.Errorf("inspect project merge conflicts: %w", err)
	}
	if conflicts != 0 {
		return fmt.Errorf("refusing to overwrite conflicting project row %q", projectID)
	}
	query = fmt.Sprintf(`
INSERT INTO projects (%s)
SELECT %s FROM legacy.projects AS l
WHERE l.id = ?
  AND NOT EXISTS (SELECT 1 FROM main.projects AS m WHERE %s)
`, columnList, columnList, primaryKeyJoinPredicate(primaryKey, "m", "l"))
	_, err := tx.ExecContext(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("merge project row: %w", err)
	}
	return nil
}

func copyProjectScopedRows(ctx context.Context, tx mergeExecer, table string, projectID string) error {
	if !legacyTableExists(ctx, tx, table) {
		return nil
	}
	columns := sharedColumns(ctx, tx, table)
	if len(columns) == 0 {
		return nil
	}
	if !columnsContain(columns, []string{"project_id"}) {
		return fmt.Errorf("merge %s rows: project_id column is unavailable", table)
	}
	primaryKey := tablePrimaryKeyColumns(ctx, tx, "main", table)
	if len(primaryKey) == 0 || !columnsContain(tableColumns(ctx, tx, "legacy", table), primaryKey) {
		return fmt.Errorf("merge %s rows: primary key is unavailable", table)
	}
	columnList := quotedColumnList(columns)
	quotedTable := quoteSQLiteIdentifier(table)
	join := primaryKeyJoinPredicate(primaryKey, "m", "l")
	comparisons := columnDifferencePredicates(columns, "m", "l")
	var conflicts int
	query := fmt.Sprintf(`
SELECT COUNT(*)
FROM main.%s AS m
JOIN legacy.%s AS l ON %s
WHERE l.project_id = ? AND (m.project_id IS NOT l.project_id OR %s)`, quotedTable, quotedTable, join, strings.Join(comparisons, " OR "))
	if err := tx.QueryRowContext(ctx, query, projectID).Scan(&conflicts); err != nil {
		return fmt.Errorf("inspect %s merge conflicts: %w", table, err)
	}
	if conflicts != 0 {
		return fmt.Errorf("refusing to overwrite conflicting %s rows: %d", table, conflicts)
	}
	query = fmt.Sprintf(`
INSERT INTO %s (%s)
SELECT %s FROM legacy.%s AS l
WHERE l.project_id = ?
  AND NOT EXISTS (SELECT 1 FROM main.%s AS m WHERE %s)
`, quotedTable, columnList, columnList, quotedTable, quotedTable, join)
	_, err := tx.ExecContext(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("merge %s rows: %w", table, err)
	}
	return nil
}

func copyProjectPathRows(ctx context.Context, tx mergeExecer, projectID string) error {
	if !legacyTableExists(ctx, tx, "project_paths") {
		return nil
	}
	columns := sharedColumns(ctx, tx, "project_paths")
	if !columnsContain(columns, []string{"id", "project_id", "path"}) {
		return fmt.Errorf("merge project_paths rows: required identity columns are unavailable")
	}
	quotedTable := quoteSQLiteIdentifier("project_paths")
	var conflicts int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM main.project_paths AS m
JOIN legacy.project_paths AS l ON m.path = l.path
WHERE l.project_id = ? AND m.project_id IS NOT l.project_id`, projectID).Scan(&conflicts); err != nil {
		return fmt.Errorf("inspect project path ownership conflicts: %w", err)
	}
	if conflicts != 0 {
		return fmt.Errorf("refusing to overwrite project path owned by another project: %d", conflicts)
	}
	pathColumns := tableColumns(ctx, tx, "main", "project_paths")
	pathDifferences := columnDifferencePredicates(pathColumns, "m", "l")
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM main.project_paths AS m
JOIN legacy.project_paths AS l ON m.id = l.id
WHERE l.project_id = ? AND (%s)`, strings.Join(pathDifferences, " OR ")), projectID).Scan(&conflicts); err != nil {
		return fmt.Errorf("inspect project path identity conflicts: %w", err)
	}
	if conflicts != 0 {
		return fmt.Errorf("refusing to overwrite conflicting project path rows: %d", conflicts)
	}
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM main.project_paths AS m
JOIN legacy.project_paths AS l ON m.path = l.path AND m.project_id = l.project_id
WHERE l.project_id = ? AND (%s)`, strings.Join(pathDifferences, " OR ")), projectID).Scan(&conflicts); err != nil {
		return fmt.Errorf("inspect project path ownership identity conflicts: %w", err)
	}
	if conflicts != 0 {
		return fmt.Errorf("refusing to overwrite project path rows with differing identities: %d", conflicts)
	}
	primaryKey := tablePrimaryKeyColumns(ctx, tx, "main", "project_paths")
	if len(primaryKey) == 0 {
		return fmt.Errorf("merge project_paths rows: primary key is unavailable")
	}
	columnList := quotedColumnList(columns)
	join := primaryKeyJoinPredicate(primaryKey, "m", "l")
	query := fmt.Sprintf(`
INSERT INTO %s (%s)
SELECT %s FROM legacy.project_paths AS l
WHERE l.project_id = ?
  AND NOT EXISTS (SELECT 1 FROM main.project_paths AS m WHERE m.path = l.path)
  AND NOT EXISTS (SELECT 1 FROM main.project_paths AS m WHERE %s)
`, quotedTable, columnList, columnList, join)
	if _, err := tx.ExecContext(ctx, query, projectID); err != nil {
		return fmt.Errorf("merge project_paths rows: %w", err)
	}
	return nil
}

func columnsContain(columns []string, required []string) bool {
	seen := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		seen[column] = struct{}{}
	}
	for _, column := range required {
		if _, ok := seen[column]; !ok {
			return false
		}
	}
	return true
}

func projectIdentityComparableColumns(columns []string) []string {
	comparable := make([]string, 0, len(columns))
	for _, column := range columns {
		if column == "current_path" || column == "friendly_name" {
			continue
		}
		comparable = append(comparable, column)
	}
	return comparable
}

func columnDifferencePredicates(columns []string, left string, right string) []string {
	predicates := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted := quoteSQLiteIdentifier(column)
		predicates = append(predicates, fmt.Sprintf("%s.%s IS NOT %s.%s", left, quoted, right, quoted))
	}
	return predicates
}

func primaryKeyJoinPredicate(columns []string, left string, right string) string {
	predicates := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted := quoteSQLiteIdentifier(column)
		predicates = append(predicates, fmt.Sprintf("%s.%s IS %s.%s", left, quoted, right, quoted))
	}
	return strings.Join(predicates, " AND ")
}

func legacyTableExists(ctx context.Context, tx mergeExecer, table string) bool {
	rows, err := tx.QueryContext(ctx, `SELECT 1 FROM legacy.sqlite_schema WHERE type = 'table' AND name = ? LIMIT 1`, table)
	if err != nil {
		return false
	}
	defer rows.Close()
	return rows.Next()
}

func sharedColumns(ctx context.Context, tx mergeExecer, table string) []string {
	mainColumns := tableColumns(ctx, tx, "main", table)
	if len(mainColumns) == 0 {
		return nil
	}
	legacyColumns := tableColumns(ctx, tx, "legacy", table)
	legacySet := map[string]bool{}
	for _, column := range legacyColumns {
		legacySet[column] = true
	}
	var shared []string
	for _, column := range mainColumns {
		if legacySet[column] {
			shared = append(shared, column)
		}
	}
	return shared
}

func tableColumns(ctx context.Context, tx mergeExecer, schema string, table string) []string {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`PRAGMA %s.table_info(%s)`, quoteSQLiteIdentifier(schema), quoteSQLiteIdentifier(table)))
	if err != nil {
		return nil
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil
		}
		columns = append(columns, name)
	}
	return columns
}

func tablePrimaryKeyColumns(ctx context.Context, tx mergeExecer, schema string, table string) []string {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`PRAGMA %s.table_info(%s)`, quoteSQLiteIdentifier(schema), quoteSQLiteIdentifier(table)))
	if err != nil {
		return nil
	}
	defer rows.Close()
	type primaryKeyColumn struct {
		order int
		name  string
	}
	var columns []primaryKeyColumn
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil
		}
		if pk > 0 {
			columns = append(columns, primaryKeyColumn{order: pk, name: name})
		}
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	for i := range columns {
		for j := i + 1; j < len(columns); j++ {
			if columns[j].order < columns[i].order {
				columns[i], columns[j] = columns[j], columns[i]
			}
		}
	}
	result := make([]string, 0, len(columns))
	for _, column := range columns {
		result = append(result, column.name)
	}
	return result
}

func registerMergedProjectTx(ctx context.Context, tx *sql.Tx, root project.Root, projectID string) error {
	var currentPath sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT current_path FROM main.projects WHERE id = ?`, projectID).Scan(&currentPath); err != nil {
		return fmt.Errorf("register merged project: read project %q: %w", projectID, err)
	}
	if currentPath.Valid && strings.TrimSpace(currentPath.String) != "" && currentPath.String != root.Path() {
		return fmt.Errorf("refusing to remap project %q from %q to %q", projectID, currentPath.String, root.Path())
	}
	rows, err := tx.QueryContext(ctx, `SELECT path FROM main.project_paths WHERE project_id = ? AND is_current = 1`, projectID)
	if err != nil {
		return fmt.Errorf("register merged project: inspect current paths for %q: %w", projectID, err)
	}
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return fmt.Errorf("register merged project: read current path for %q: %w", projectID, err)
		}
		if path != root.Path() {
			rows.Close()
			return fmt.Errorf("refusing to remap project %q from %q to %q", projectID, path, root.Path())
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("register merged project: iterate current paths for %q: %w", projectID, err)
	}
	rows.Close()
	var pathOwner string
	err = tx.QueryRowContext(ctx, `SELECT project_id FROM main.project_paths WHERE path = ?`, root.Path()).Scan(&pathOwner)
	switch {
	case err == nil && pathOwner != projectID:
		return fmt.Errorf("refusing to remap path %q from project %q to project %q", root.Path(), pathOwner, projectID)
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("register merged project: inspect path %q: %w", root.Path(), err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE main.projects
SET current_path = ?, friendly_name = COALESCE(NULLIF(friendly_name, ''), ?)
WHERE id = ?`, root.Path(), defaultProjectFriendlyName(root.Path()), projectID); err != nil {
		return fmt.Errorf("register merged project path: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE main.project_paths
SET is_current = 0
WHERE project_id = ? AND path <> ?`, projectID, root.Path()); err != nil {
		return fmt.Errorf("register merged project: clear stale paths: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
INSERT INTO main.project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
ON CONFLICT(path) DO UPDATE SET is_current = 1`, stableMigrationID("project-path", projectID, root.Path()), projectID, root.Path(), now, now, now, now); err != nil {
		return fmt.Errorf("register merged project: upsert current path: %w", err)
	}
	return nil
}

func quotedColumnList(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteSQLiteIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}

func quoteSQLiteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func migrationSourceDatabasePath(root project.Root, resolver PathResolver) (string, error) {
	projectPath, err := resolver.ProjectDatabasePath(root)
	if err != nil {
		return "", err
	}
	if regularFileExists(projectPath) {
		return projectPath, nil
	}
	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		return "", err
	}
	return legacyPath, nil
}

func verifyStorageHomeSource(ctx context.Context, store *Store) error {
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return err
	}
	if version == CurrentSchemaVersion() {
		if _, err := store.ValidateCurrentSchema(ctx); err != nil {
			return err
		}
	} else {
		behind, err := classifySchemaUpgradeTarget(store.path, version)
		if err != nil {
			return err
		}
		if !behind {
			return fmt.Errorf("legacy state schema %d is invalid or unsupported", version)
		}
	}
	if _, valid, err := inspectSQLiteIntegrity(ctx, store); err != nil {
		return err
	} else if !valid {
		return fmt.Errorf("legacy state SQLite integrity checks failed")
	}
	if version >= 9 {
		if _, valid, err := inspectOperationalInvariants(ctx, store); err != nil {
			return err
		} else if !valid {
			return fmt.Errorf("legacy state operational invariants failed")
		}
	}
	return nil
}

func resolveAttachedProjectID(ctx context.Context, tx mergeExecer, root project.Root) (string, error) {
	if !legacyTableExists(ctx, tx, "projects") {
		return "", fmt.Errorf("resolve legacy project identity: attached legacy database has no projects table")
	}
	candidates := make([]string, 0, 2)
	if legacyTableExists(ctx, tx, "project_paths") {
		pathColumns := tableColumns(ctx, tx, "legacy", "project_paths")
		if !columnsContain(pathColumns, []string{"project_id", "path", "is_current"}) {
			return "", fmt.Errorf("resolve legacy project identity: attached project_paths table has no current mapping columns")
		}
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT project_id FROM legacy.project_paths WHERE path = ? AND is_current = 1`, root.Path())
		if err != nil {
			return "", fmt.Errorf("resolve legacy project identity from project_paths: %w", err)
		}
		for rows.Next() {
			var projectID string
			if err := rows.Scan(&projectID); err != nil {
				rows.Close()
				return "", fmt.Errorf("resolve legacy project identity from project_paths: %w", err)
			}
			if strings.TrimSpace(projectID) != "" {
				candidates = append(candidates, projectID)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", fmt.Errorf("resolve legacy project identity from project_paths: %w", err)
		}
		rows.Close()
	}
	projectColumns := tableColumns(ctx, tx, "legacy", "projects")
	if columnsContain(projectColumns, []string{"current_path"}) {
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT id FROM legacy.projects WHERE current_path = ?`, root.Path())
		if err != nil {
			return "", fmt.Errorf("resolve legacy project identity from current_path: %w", err)
		}
		for rows.Next() {
			var projectID string
			if err := rows.Scan(&projectID); err != nil {
				rows.Close()
				return "", fmt.Errorf("resolve legacy project identity from current_path: %w", err)
			}
			if strings.TrimSpace(projectID) != "" {
				candidates = append(candidates, projectID)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", fmt.Errorf("resolve legacy project identity from current_path: %w", err)
		}
		rows.Close()
	}
	candidates = uniqueProjectIDs(candidates)
	if len(candidates) > 1 {
		return "", fmt.Errorf("resolve legacy project identity: ambiguous mapping for %q (%s)", root.Path(), strings.Join(candidates, ", "))
	}
	projectID := ""
	if len(candidates) == 1 {
		projectID = candidates[0]
	} else {
		projectID = ProjectID(root)
	}
	var projectRows int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM legacy.projects WHERE id = ?`, projectID).Scan(&projectRows); err != nil {
		return "", fmt.Errorf("resolve legacy project identity: inspect project %q: %w", projectID, err)
	}
	if projectRows != 1 {
		if len(candidates) == 1 {
			return "", fmt.Errorf("resolve legacy project identity: mapping for %q points to %q but exactly one legacy.projects row is required (found %d)", root.Path(), projectID, projectRows)
		}
		return "", fmt.Errorf("resolve legacy project identity: no project row maps to %q and legacy identity %q is absent", root.Path(), projectID)
	}
	return projectID, nil
}

func uniqueProjectIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func databaseContainsProject(path string, projectID string) bool {
	store, err := OpenStore(path)
	if err != nil {
		return false
	}
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, projectID).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func databaseContainsRootProject(path string, root project.Root) bool {
	store, err := OpenStore(path)
	if err != nil {
		return false
	}
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM project_paths WHERE path = ?`, root.Path()).Scan(&count); err == nil && count > 0 {
		return true
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, ProjectID(root)).Scan(&count); err != nil {
		return false
	}
	return count > 0
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func reserveStorageHomeStaging(databasePath string) (string, os.FileInfo, error) {
	file, err := os.CreateTemp(filepath.Dir(databasePath), ".loaf-storage-migration-*.sqlite")
	if err != nil {
		return "", nil, fmt.Errorf("reserve staging state database: %w", err)
	}
	path := file.Name()
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("stat reserved staging state database: %w", statErr)
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("close reserved staging state database: %w", closeErr)
	}
	return path, info, nil
}

func copySQLiteDatabaseToReserved(ctx context.Context, source *Store, destination string) error {
	connection, err := source.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire legacy state database connection: %w", err)
	}
	defer connection.Close()
	if err := connection.Raw(func(driverConnection any) error {
		rawConnection, ok := driverConnection.(interface{ Raw() *sqlite3.Conn })
		if !ok {
			return fmt.Errorf("SQLite driver connection %T does not expose raw connection", driverConnection)
		}
		return rawConnection.Raw().Backup("main", destination)
	}); err != nil {
		return fmt.Errorf("copy state database to staging: %w", err)
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return fmt.Errorf("set staging state database permissions: %w", err)
	}
	return nil
}

func checkpointStorageHomeStaging(ctx context.Context, store *Store) error {
	var busy, logFrames, checkpointedFrames int
	if err := store.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logFrames, &checkpointedFrames); err != nil {
		return err
	}
	if busy != 0 || logFrames != 0 || checkpointedFrames != 0 {
		return fmt.Errorf("WAL checkpoint incomplete: busy=%d log=%d checkpointed=%d", busy, logFrames, checkpointedFrames)
	}
	return nil
}

func ensureStorageHomeStagingSidecarsClean(databasePath string) error {
	for _, sidecar := range []string{databasePath + "-wal", databasePath + "-shm"} {
		info, err := os.Lstat(sidecar)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect staging SQLite sidecar %s: %w", sidecar, err)
		}
		if !info.Mode().IsRegular() || info.Size() != 0 {
			return fmt.Errorf("staging SQLite sidecar remains live: %s (%d bytes)", sidecar, info.Size())
		}
		if err := os.Remove(sidecar); err != nil {
			return fmt.Errorf("remove empty staging SQLite sidecar %s: %w", sidecar, err)
		}
	}
	return nil
}

func syncStorageHomeFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open staging state database for sync: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync staging state database: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close synced staging state database: %w", err)
	}
	return nil
}

func verifyStorageHomeDestination(ctx context.Context, root project.Root, databasePath string) (ProjectIdentity, error) {
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return ProjectIdentity{}, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = store.Close()
		}
	}()
	if _, err := store.ValidateCurrentSchema(ctx); err != nil {
		return ProjectIdentity{}, err
	}
	if _, valid, err := inspectSQLiteIntegrity(ctx, store); err != nil {
		return ProjectIdentity{}, err
	} else if !valid {
		return ProjectIdentity{}, fmt.Errorf("SQLite integrity checks failed")
	}
	if _, valid, err := inspectOperationalInvariants(ctx, store); err != nil {
		return ProjectIdentity{}, err
	} else if !valid {
		return ProjectIdentity{}, fmt.Errorf("operational invariants failed")
	}
	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return ProjectIdentity{}, err
	}
	if err := store.Close(); err != nil {
		return ProjectIdentity{}, fmt.Errorf("close verified state database: %w", err)
	}
	closed = true
	return identity, nil
}

func publishStorageHomeStaging(stagingPath, databasePath string) (bool, error) {
	if err := os.Link(stagingPath, databasePath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("publish staging state database: %w", err)
	}
	if err := syncStorageHomeDirectory(filepath.Dir(databasePath)); err != nil {
		return true, err
	}
	if err := os.Remove(stagingPath); err != nil {
		return true, fmt.Errorf("remove published staging state database name: %w", err)
	}
	if err := syncStorageHomeDirectory(filepath.Dir(databasePath)); err != nil {
		return true, err
	}
	return true, nil
}

func syncStorageHomeDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open state database directory for sync: %w", err)
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return fmt.Errorf("sync state database directory: %w", err)
	}
	if err := directory.Close(); err != nil {
		return fmt.Errorf("close synced state database directory: %w", err)
	}
	return nil
}

func cleanupOwnedStorageHomeStaging(path string, reserved os.FileInfo) {
	info, err := os.Lstat(path)
	if err == nil && info.Mode().IsRegular() && os.SameFile(info, reserved) {
		_ = os.Remove(path + "-wal")
		_ = os.Remove(path + "-shm")
		_ = os.Remove(path)
	}
}

func copySQLiteDatabase(ctx context.Context, source *Store, destination string, mode os.FileMode) error {
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("create destination state database: %s already exists", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat destination state database: %w", err)
	}

	copied := false
	defer func() {
		if !copied {
			_ = os.Remove(destination)
		}
	}()
	if _, err := source.db.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("copy state database: %w", err)
	}
	if err := os.Chmod(destination, mode); err != nil {
		return fmt.Errorf("set destination state database permissions: %w", err)
	}
	copied = true
	return nil
}
