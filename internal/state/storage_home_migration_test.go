package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestPreviewStorageHomeMigrationPlansLegacyCopy(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})

	plan, err := PreviewStorageHomeMigration(root, PathResolver{})
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}

	if plan.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", plan.ContractVersion, StateJSONContractVersion)
	}
	if plan.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", plan.DatabaseScope)
	}
	if plan.MigrationScope != "project" {
		t.Fatalf("MigrationScope = %q, want project", plan.MigrationScope)
	}
	if plan.ProjectID != "" {
		t.Fatalf("ProjectID = %q, want empty before apply", plan.ProjectID)
	}
	if plan.Action != StorageHomeActionCopy {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionCopy)
	}
	if plan.Applied {
		t.Fatal("Applied = true, want false for dry-run")
	}
	if plan.DatabaseExists {
		t.Fatal("DatabaseExists = true, want false before apply")
	}
	if !plan.LegacyDatabaseExists {
		t.Fatal("LegacyDatabaseExists = false, want true")
	}
	if plan.LegacyDatabasePath != legacyPath {
		t.Fatalf("LegacyDatabasePath = %q, want %q", plan.LegacyDatabasePath, legacyPath)
	}
	if !strings.HasPrefix(plan.DatabasePath, dataHome) {
		t.Fatalf("DatabasePath = %q, want under data home %q", plan.DatabasePath, dataHome)
	}
}

func TestApplyStorageHomeMigrationCopiesLegacyDatabaseWithoutDeletingIt(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})

	plan, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	if plan.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", plan.ContractVersion, StateJSONContractVersion)
	}
	if plan.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", plan.DatabaseScope)
	}
	if plan.MigrationScope != "project" {
		t.Fatalf("MigrationScope = %q, want project", plan.MigrationScope)
	}
	if plan.ProjectID == "" {
		t.Fatal("ProjectID is empty after apply")
	}
	if plan.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", plan.ProjectName, filepath.Base(root.Path()))
	}
	if plan.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", plan.ProjectCurrentPath, root.Path())
	}
	if !plan.Applied {
		t.Fatal("Applied = false, want true")
	}
	if plan.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionAlreadyMigrated)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy database stat error = %v, want legacy file preserved", err)
	}
	if _, err := os.Stat(plan.DatabasePath); err != nil {
		t.Fatalf("data-home database stat error = %v", err)
	}

	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q after migration", status.Mode, ModeSQLiteReady)
	}
	if status.DatabasePath != plan.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", status.DatabasePath, plan.DatabasePath)
	}

	second, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("second ApplyStorageHomeMigration() error = %v", err)
	}
	if second.Applied {
		t.Fatal("second Applied = true, want idempotent no-op")
	}
	if second.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("second Action = %q, want %q", second.Action, StorageHomeActionAlreadyMigrated)
	}
	if second.DatabaseScope != "global" || second.MigrationScope != "project" {
		t.Fatalf("second scopes = %q/%q, want global/project", second.DatabaseScope, second.MigrationScope)
	}
}

func TestStorageHomeMigrationUsesProjectDataDatabaseBeforeStateHome(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	projectPath := initializeProjectDataDatabase(t, root, PathResolver{})
	initializeLegacyStateDatabase(t, root, PathResolver{})

	plan, err := PreviewStorageHomeMigration(root, PathResolver{})
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}
	if plan.Action != StorageHomeActionCopy {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionCopy)
	}
	if plan.DatabaseScope != "global" || plan.MigrationScope != "project" {
		t.Fatalf("scopes = %q/%q, want global/project", plan.DatabaseScope, plan.MigrationScope)
	}
	if plan.LegacyDatabasePath != projectPath {
		t.Fatalf("LegacyDatabasePath = %q, want project data path %q", plan.LegacyDatabasePath, projectPath)
	}

	applied, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	if !applied.Applied {
		t.Fatal("Applied = false, want true")
	}
	if applied.ProjectID == "" {
		t.Fatal("ProjectID is empty after project database migration")
	}
	if applied.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", applied.ProjectCurrentPath, root.Path())
	}
	if applied.DatabasePath == projectPath {
		t.Fatalf("DatabasePath = %q, want global path distinct from project path", applied.DatabasePath)
	}
	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
}

func TestApplyStorageHomeMigrationUpgradesCopiedLegacySchema(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	resolver := PathResolver{}
	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		t.Fatalf("LegacyDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("create legacy database dir error = %v", err)
	}
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if err := ApplyMigrations(ctx, legacyStore.db, SchemaMigrations()[:1]); err != nil {
		t.Fatalf("apply legacy schema migration error = %v", err)
	}
	if err := legacyStore.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("close legacy store error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	if !plan.Applied {
		t.Fatal("Applied = false, want true")
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy database stat error = %v, want legacy file preserved", err)
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	if status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentSchemaVersion())
	}
}

func TestApplyStorageHomeMigrationDoesNotOverwriteExistingDataHomeDatabase(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	initializeLegacyStateDatabase(t, root, PathResolver{})
	dataStatus, err := Initialize(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("Initialize(data) error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	if plan.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionAlreadyMigrated)
	}
	if plan.DatabasePath != dataStatus.DatabasePath {
		t.Fatalf("DatabasePath = %q, want existing data path %q", plan.DatabasePath, dataStatus.DatabasePath)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("Warnings empty, want legacy-leftover warning")
	}
}

func TestApplyStorageHomeMigrationIncludesPendingWALFrames(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	defer legacyStore.Close()

	if _, err := legacyStore.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("wal checkpoint error = %v", err)
	}
	wantProjectID := "pending-wal-project"
	wantProjectPath := filepath.Join(root.Path(), "pending-wal-project")
	if _, err := legacyStore.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, wantProjectID, wantProjectID, "Pending WAL Project", wantProjectPath, "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z"); err != nil {
		t.Fatalf("insert WAL-backed project error = %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, "pending-wal-project-path", wantProjectID, wantProjectPath, "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z"); err != nil {
		t.Fatalf("insert WAL-backed project path error = %v", err)
	}
	if info, err := os.Stat(legacyPath + "-wal"); err != nil {
		t.Fatalf("legacy WAL stat error = %v", err)
	} else if info.Size() == 0 {
		t.Fatal("legacy WAL is empty, want pending frames before migration")
	}

	plan, err := ApplyStorageHomeMigration(ctx, root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	dataStore, err := OpenStore(plan.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(data) error = %v", err)
	}
	defer dataStore.Close()
	var count int
	if err := dataStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE id = ?`, wantProjectID).Scan(&count); err != nil {
		t.Fatalf("query migrated project error = %v", err)
	}
	if count != 1 {
		t.Fatalf("migrated project count = %d, want 1", count)
	}
}

func initializeLegacyStateDatabase(t *testing.T, root project.Root, resolver PathResolver) string {
	t.Helper()
	ctx := context.Background()
	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		t.Fatalf("LegacyDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("create legacy database dir error = %v", err)
	}
	store, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations(legacy) error = %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject(legacy) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	return legacyPath
}

func initializeProjectDataDatabase(t *testing.T, root project.Root, resolver PathResolver) string {
	t.Helper()
	projectPath, err := resolver.ProjectDatabasePath(root)
	if err != nil {
		t.Fatalf("ProjectDatabasePath() error = %v", err)
	}
	initializeDatabaseAtPath(t, root, projectPath)
	return projectPath
}

func initializeDatabaseAtPath(t *testing.T, root project.Root, path string) {
	t.Helper()
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create database dir error = %v", err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(%s) error = %v", path, err)
	}
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations(%s) error = %v", path, err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject(%s) error = %v", path, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(%s) error = %v", path, err)
	}
}
