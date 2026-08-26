package state

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
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

func TestMigration0018DeletesGoneKindRelationshipsAndAliases(t *testing.T) {
	ctx := context.Background()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	migrations := SchemaMigrations()
	dropIdx := -1
	for i, migration := range migrations {
		if migration.Name == "drop_findings_verdicts_runs" {
			dropIdx = i
			break
		}
	}
	if dropIdx < 0 {
		t.Fatal("drop_findings_verdicts_runs migration missing")
	}
	if err := ApplyMigrations(ctx, store.db, migrations[:dropIdx]); err != nil {
		t.Fatalf("ApplyMigrations(pre-0018) error = %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	projectID := projectIDForTest(t, store, root)
	now := "2026-08-26T00:00:00Z"

	mustExec(t, store, `
INSERT INTO reports (id, project_id, report_kind, title, status, created_at, updated_at)
VALUES ('report-alive', ?, 'research', 'Alive Report', 'draft', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES
  ('alias-report', ?, 'report', 'report-alive', 'report', 'REPORT-ALIVE', ?, ?),
  ('alias-finding', ?, 'finding', 'finding-fossil', 'finding', 'FINDING-FOSSIL', ?, ?)
`, projectID, now, now, projectID, now, now)
	mustExec(t, store, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES
  ('rel-report-finding', ?, 'report', 'report-alive', 'finding', 'finding-fossil', 'produced', 'fossil', 'command', ?, ?),
  ('rel-run-report', ?, 'run', 'run-fossil', 'report', 'report-alive', 'generated', 'fossil', 'command', ?, ?),
  ('rel-keep', ?, 'report', 'report-alive', 'report', 'report-alive', 'self', 'keep', 'manual', ?, ?)
`, projectID, now, now, projectID, now, now, projectID, now, now)

	if err := ApplyMigrations(ctx, store.db, migrations[dropIdx:]); err != nil {
		t.Fatalf("ApplyMigrations(0018) error = %v", err)
	}

	var fossilRelationships, fossilAliases, keptRelationships int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE from_entity_kind IN ('finding', 'verdict', 'run')
   OR to_entity_kind IN ('finding', 'verdict', 'run')
`).Scan(&fossilRelationships); err != nil {
		t.Fatalf("count fossil relationships: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM aliases WHERE entity_kind IN ('finding', 'verdict', 'run')
`).Scan(&fossilAliases); err != nil {
		t.Fatalf("count fossil aliases: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relationships WHERE id = 'rel-keep'`).Scan(&keptRelationships); err != nil {
		t.Fatalf("count kept relationships: %v", err)
	}
	if fossilRelationships != 0 || fossilAliases != 0 {
		t.Fatalf("fossil relationships=%d aliases=%d, want 0 after migration 0018", fossilRelationships, fossilAliases)
	}
	if keptRelationships != 1 {
		t.Fatalf("kept relationships = %d, want 1", keptRelationships)
	}
}

func TestTraceAndLinkListSkipGoneKindNeighborEdges(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	projectID := projectIDForTest(t, store, root)
	now := "2026-08-26T00:00:00Z"
	mustExec(t, store, `
INSERT INTO reports (id, project_id, report_kind, title, status, created_at, updated_at)
VALUES ('report-alive', ?, 'research', 'Alive Report', 'draft', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-report', ?, 'report', 'report-alive', 'report', 'REPORT-ALIVE', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES
  ('rel-fossil', ?, 'report', 'report-alive', 'finding', 'finding-fossil', 'produced', 'fossil neighbor', 'command', ?, ?),
  ('rel-alive', ?, 'report', 'report-alive', 'report', 'report-alive', 'self', 'alive neighbor', 'manual', ?, ?)
`, projectID, now, now, projectID, now, now)

	trace, err := store.Trace(ctx, root, "REPORT-ALIVE")
	if err != nil {
		t.Fatalf("Trace() error = %v, want success despite fossil finding neighbor", err)
	}
	for _, rel := range trace.Relationships {
		if rel.Entity.Kind == "finding" || rel.Entity.Kind == "verdict" || rel.Entity.Kind == "run" {
			t.Fatalf("trace relationships = %#v, want fossil gone-kind edges skipped", trace.Relationships)
		}
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "self", "report", "REPORT-ALIVE") {
		t.Fatalf("trace relationships = %#v, want alive report self edge retained", trace.Relationships)
	}

	list, err := store.ListLinks(ctx, root, "REPORT-ALIVE")
	if err != nil {
		t.Fatalf("ListLinks() error = %v, want success despite fossil finding neighbor", err)
	}
	for _, rel := range list.Relationships {
		if rel.Entity.Kind == "finding" || rel.Entity.Kind == "verdict" || rel.Entity.Kind == "run" {
			t.Fatalf("list relationships = %#v, want fossil gone-kind edges skipped", list.Relationships)
		}
	}
	if !hasStateTraceRelationship(list.Relationships, "outbound", "self", "report", "REPORT-ALIVE") {
		t.Fatalf("list relationships = %#v, want alive report self edge retained", list.Relationships)
	}
}
