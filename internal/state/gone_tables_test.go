package state

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoneTablesAreAbsentAfterMigrations(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := ApplyMigrations(ctx, db, SchemaMigrations()); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if err := validateGoneTablesAbsent(ctx, db); err != nil {
		t.Fatalf("validateGoneTablesAbsent() error = %v", err)
	}
}

func TestGoneTablesRefusedIfTheyReappear(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := ApplyMigrations(ctx, db, SchemaMigrations()); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE findings (id TEXT PRIMARY KEY NOT NULL)`); err != nil {
		t.Fatalf("create findings table error = %v", err)
	}
	err = validateGoneTablesAbsent(ctx, db)
	if err == nil {
		t.Fatal("validateGoneTablesAbsent() error = nil, want refusal when findings reappears")
	}
	if !strings.Contains(err.Error(), "findings") {
		t.Fatalf("validateGoneTablesAbsent() error = %v, want findings named", err)
	}
}

func TestStoreValidateCurrentSchemaRefusesGoneTables(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	db, err := sql.Open(sqliteDriverName, path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if err := ApplyMigrations(ctx, db, SchemaMigrations()); err != nil {
		db.Close()
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE runs (id TEXT PRIMARY KEY NOT NULL)`); err != nil {
		db.Close()
		t.Fatalf("create runs table error = %v", err)
	}
	db.Close()

	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	_, err = store.ValidateCurrentSchema(ctx)
	if err == nil {
		t.Fatal("ValidateCurrentSchema() error = nil, want gone-table refusal")
	}
	if !strings.Contains(err.Error(), "runs") {
		t.Fatalf("ValidateCurrentSchema() error = %v, want runs named", err)
	}
}
