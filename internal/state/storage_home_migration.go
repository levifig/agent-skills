package state

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	legacyStore, err := OpenStore(plan.LegacyDatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open legacy state database: %w", err)
	}
	if _, err := legacyStore.SchemaVersion(ctx); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("read legacy state database schema: %w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("read legacy state database schema: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(plan.DatabasePath), 0o700); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("create data state directory: %w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("create data state directory: %w", err)
	}
	if err := copySQLiteDatabase(ctx, legacyStore, plan.DatabasePath, 0o600); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("%w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, err
	}
	if err := legacyStore.Close(); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("close legacy state database: %w", err)
	}

	copiedReady := false
	defer func() {
		if !copiedReady {
			_ = os.Remove(plan.DatabasePath)
		}
	}()
	copiedStore, err := OpenStore(plan.DatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open copied state database: %w", err)
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
	if err := copiedStore.Close(); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("close copied state database: %w", err)
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if status.Mode != ModeSQLiteReady {
		return StorageHomeMigrationPlan{}, fmt.Errorf("copied state database is not ready: %s", status.Mode)
	}
	copiedReady = true
	plan.recordVerifiedProject(status)
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
	store, err := OpenStore(plan.DatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open global state database: %w", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("upgrade global state database: %w", err)
	}
	if err := store.mergeProjectDatabase(ctx, plan.LegacyDatabasePath, ProjectID(root)); err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("record global state project: %w", err)
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

func (s *Store) mergeProjectDatabase(ctx context.Context, sourcePath string, projectID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project database merge: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `ATTACH DATABASE ? AS legacy`, sourcePath); err != nil {
		return fmt.Errorf("attach legacy project database: %w", err)
	}

	if err := copyProjectRow(ctx, tx, projectID); err != nil {
		return err
	}
	for _, table := range projectScopedMergeTables {
		if err := copyProjectScopedRows(ctx, tx, table, projectID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project database merge: %w", err)
	}
	return nil
}

type mergeExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func copyProjectRow(ctx context.Context, tx mergeExecer, projectID string) error {
	if !legacyTableExists(ctx, tx, "projects") {
		return nil
	}
	columns := sharedColumns(ctx, tx, "projects")
	if len(columns) == 0 {
		return nil
	}
	columnList := quotedColumnList(columns)
	_, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT OR REPLACE INTO projects (%s)
SELECT %s FROM legacy.projects WHERE id = ?
`, columnList, columnList), projectID)
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
	columnList := quotedColumnList(columns)
	quotedTable := quoteSQLiteIdentifier(table)
	_, err := tx.ExecContext(ctx, fmt.Sprintf(`
INSERT OR REPLACE INTO %s (%s)
SELECT %s FROM legacy.%s WHERE project_id = ?
`, quotedTable, columnList, columnList, quotedTable), projectID)
	if err != nil {
		return fmt.Errorf("merge %s rows: %w", table, err)
	}
	return nil
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
