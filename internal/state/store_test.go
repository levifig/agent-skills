package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	if status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentSchemaVersion())
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
	if second.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", second.SchemaVersion, CurrentSchemaVersion())
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

func TestApplyMigrationsRejectsInvalidFutureMigrationSequence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		migrations []SchemaMigration
		want       string
	}{
		{
			name: "duplicate version",
			migrations: []SchemaMigration{
				{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
				{Version: 1, Name: "also_one", SQL: "CREATE TABLE also_one (id TEXT PRIMARY KEY NOT NULL);\n"},
			},
			want: "version 1 must be greater than previous version 1",
		},
		{
			name: "out of order",
			migrations: []SchemaMigration{
				{Version: 2, Name: "two", SQL: "CREATE TABLE two (id TEXT PRIMARY KEY NOT NULL);\n"},
				{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
			},
			want: "version 1 must be greater than previous version 2",
		},
		{
			name: "missing name",
			migrations: []SchemaMigration{
				{Version: 1, SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
			},
			want: "schema migration 1 must have a name",
		},
		{
			name: "empty sql",
			migrations: []SchemaMigration{
				{Version: 1, Name: "one"},
			},
			want: "schema migration 1 SQL cannot be empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
			if err != nil {
				t.Fatalf("sql.Open() error = %v", err)
			}
			defer db.Close()

			err = ApplyMigrations(context.Background(), db, tc.migrations)
			if err == nil {
				t.Fatal("ApplyMigrations() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ApplyMigrations() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestApplyMigrationsRollsBackFailedMigrationBatch(t *testing.T) {
	db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	migrations := []SchemaMigration{
		{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
		{Version: 2, Name: "bad", SQL: "CREATE TABLE broken (\n"},
	}
	if err := ApplyMigrations(context.Background(), db, migrations); err == nil {
		t.Fatal("ApplyMigrations() error = nil, want failure")
	}

	var tableName string
	err = db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'one'`).Scan(&tableName)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("table one lookup error = %v, want no table after rollback", err)
	}
	var migrationsTable string
	err = db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&migrationsTable)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("schema_migrations lookup error = %v, want no table after rollback", err)
	}
}
