package state

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestInitializeAppliesMigrationsAndRecordsProject(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()

	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	if status.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", status.SchemaVersion)
	}
	if !status.DatabaseExists {
		t.Fatal("DatabaseExists = false, want true")
	}
	if !filepath.IsAbs(status.DatabasePath) {
		t.Fatalf("DatabasePath = %q, want absolute path", status.DatabasePath)
	}
	if _, err := os.Stat(status.DatabasePath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	count, err := store.AppliedMigrationCount(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrationCount() error = %v", err)
	}
	if count != len(SchemaMigrations()) {
		t.Fatalf("AppliedMigrationCount() = %d, want %d", count, len(SchemaMigrations()))
	}

	var projectID string
	err = store.db.QueryRowContext(context.Background(), `SELECT id FROM projects WHERE id = ?`, ProjectID(root)).Scan(&projectID)
	if err != nil {
		t.Fatalf("project row missing: %v", err)
	}
}

func TestInitializeIsIdempotent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()

	first, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	second, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}

	if first.DatabasePath != second.DatabasePath {
		t.Fatalf("DatabasePath changed: %q -> %q", first.DatabasePath, second.DatabasePath)
	}
	if second.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", second.SchemaVersion)
	}
}

func TestOpenStoreAppliesConnectionPragmas(t *testing.T) {
	root := projectRoot(t)
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	var foreignKeys int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys error = %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout error = %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout = %d, want at least 5000", busyTimeout)
	}

	var journalMode string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode error = %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestApplyMigrationsDetectsChecksumDrift(t *testing.T) {
	db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	migration := SchemaMigration{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"}
	if err := ApplyMigrations(context.Background(), db, []SchemaMigration{migration}); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	drifted := SchemaMigration{Version: 1, Name: "one", SQL: "CREATE TABLE two (id TEXT PRIMARY KEY NOT NULL);\n"}
	if err := ApplyMigrations(context.Background(), db, []SchemaMigration{drifted}); err == nil {
		t.Fatal("ApplyMigrations() error = nil, want checksum mismatch")
	}
}
