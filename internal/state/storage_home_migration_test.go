package state

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPreviewStorageHomeMigrationPlansLegacyCopy(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyStatus, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}

	plan, err := PreviewStorageHomeMigration(root, PathResolver{})
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
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
	if plan.LegacyDatabasePath != legacyStatus.DatabasePath {
		t.Fatalf("LegacyDatabasePath = %q, want %q", plan.LegacyDatabasePath, legacyStatus.DatabasePath)
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

	legacyStatus, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	if !plan.Applied {
		t.Fatal("Applied = false, want true")
	}
	if plan.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionAlreadyMigrated)
	}
	if _, err := os.Stat(legacyStatus.DatabasePath); err != nil {
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
}

func TestApplyStorageHomeMigrationDoesNotOverwriteExistingDataHomeDatabase(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}
	dataStatus, err := Initialize(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("Initialize(data) error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	if plan.Applied {
		t.Fatal("Applied = true, want no overwrite")
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

	legacyStatus, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}
	legacyStore, err := OpenStore(legacyStatus.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	defer legacyStore.Close()

	if _, err := legacyStore.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("wal checkpoint error = %v", err)
	}
	wantProjectID := "pending-wal-project"
	if _, err := legacyStore.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
`, wantProjectID, wantProjectID, "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z"); err != nil {
		t.Fatalf("insert WAL-backed project error = %v", err)
	}
	if info, err := os.Stat(legacyStatus.DatabasePath + "-wal"); err != nil {
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
