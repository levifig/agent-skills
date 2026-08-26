package state

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestProjectScopedTableInventoryIsComplete(t *testing.T) {
	if err := validateProjectScopedTableInventory(); err != nil {
		t.Fatalf("validateProjectScopedTableInventory() error = %v", err)
	}

	columnsByTable := schemaColumnNames(t, effectiveSchemaSQL())
	var projectScoped []string
	for table, columns := range columnsByTable {
		if userScopedTables[table] {
			continue
		}
		hasProjectID := false
		for _, column := range columns {
			if column == "project_id" {
				hasProjectID = true
				break
			}
		}
		if hasProjectID {
			projectScoped = append(projectScoped, table)
		}
	}
	sort.Strings(projectScoped)

	var missing, extra []string
	for _, table := range projectScoped {
		if _, ok := projectScopedTableClasses[table]; !ok {
			missing = append(missing, table)
		}
	}
	for table := range projectScopedTableClasses {
		if class := projectScopedTableClasses[table]; class == TableClassGone {
			continue
		}
		found := false
		for _, candidate := range projectScoped {
			if candidate == table {
				found = true
				break
			}
		}
		if !found {
			extra = append(extra, table)
		}
	}
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("inventory mismatch:\n missing: %v\n extra: %v", missing, extra)
	}
}

func TestProjectScopedMergeTablesMatchSyncInventory(t *testing.T) {
	want := ProjectScopedSyncTables()
	got := append([]string(nil), projectScopedMergeTables...)
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("projectScopedMergeTables:\n got: %v\nwant: %v", got, want)
	}
}

func TestNonSyncTablesNeverAppearInMergeRegistry(t *testing.T) {
	mergeSet := make(map[string]struct{}, len(projectScopedMergeTables))
	for _, table := range projectScopedMergeTables {
		mergeSet[table] = struct{}{}
	}
	for table, class := range projectScopedTableClasses {
		if class == TableClassSync {
			continue
		}
		if _, ok := mergeSet[table]; ok {
			t.Fatalf("table %s is class %q but appears in projectScopedMergeTables", table, class)
		}
	}
}

func TestMigration0022DeletesFossilKindRelationships(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	migrations := SchemaMigrations()
	pruneIdx := -1
	for i, migration := range migrations {
		if migration.Name == "prune_fossil_relationship_edges" {
			pruneIdx = i
			break
		}
	}
	if pruneIdx < 0 {
		t.Fatal("prune_fossil_relationship_edges migration missing")
	}
	if err := ApplyMigrations(ctx, store.db, migrations[:pruneIdx]); err != nil {
		t.Fatalf("ApplyMigrations(pre-0022) error = %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	projectID := projectIDForTest(t, store, root)
	now := "2026-08-26T00:00:00Z"

	mustExec(t, store, `
INSERT INTO sparks (id, project_id, status, text, created_at, updated_at)
VALUES ('spark-fossil-trace', ?, 'captured', 'Alive spark', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO specs (id, project_id, title, status, created_at, updated_at)
VALUES ('spec-fossil', ?, 'Fossil spec', 'draft', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES
  ('alias-spark-fossil-trace', ?, 'spark', 'spark-fossil-trace', 'spark', 'SPARK-FOSSIL-TRACE', ?, ?),
  ('alias-spec', ?, 'spec', 'spec-fossil', 'spec', 'SPEC-FOSSIL', ?, ?)
`, projectID, now, now, projectID, now, now)
	mustExec(t, store, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES
  ('rel-spark-spec', ?, 'spark', 'spark-fossil-trace', 'spec', 'spec-fossil', 'produced', 'fossil', 'command', ?, ?),
  ('rel-keep', ?, 'spark', 'spark-fossil-trace', 'spark', 'spark-fossil-trace', 'self', 'keep', 'manual', ?, ?)
`, projectID, now, now, projectID, now, now)

	if err := ApplyMigrations(ctx, store.db, migrations[pruneIdx:pruneIdx+1]); err != nil {
		t.Fatalf("ApplyMigrations(0022) error = %v", err)
	}

 	var fossilRelationships, fossilAliases, keptRelationships int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM relationships
WHERE from_entity_kind IN ('plan', 'spec', 'task', 'intent', 'brainstorm')
   OR to_entity_kind IN ('plan', 'spec', 'task', 'intent', 'brainstorm')
`).Scan(&fossilRelationships); err != nil {
		t.Fatalf("count fossil relationships: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM aliases WHERE entity_kind IN ('plan', 'spec', 'task', 'intent', 'brainstorm')
`).Scan(&fossilAliases); err != nil {
		t.Fatalf("count fossil aliases: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relationships WHERE id = 'rel-keep'`).Scan(&keptRelationships); err != nil {
		t.Fatalf("count kept relationships: %v", err)
	}
	if fossilRelationships != 0 {
		t.Fatalf("fossil relationships=%d, want 0 after migration 0022", fossilRelationships)
	}
	if fossilAliases != 1 {
		t.Fatalf("fossil aliases=%d, want 1 preserved (relationship prune only)", fossilAliases)
	}
	if keptRelationships != 1 {
		t.Fatalf("kept relationships = %d, want 1", keptRelationships)
	}
}

func TestTraceAndLinkListHydrateLocalArchiveNeighbors(t *testing.T) {
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
INSERT INTO sparks (id, project_id, status, text, created_at, updated_at)
VALUES ('spark-fossil-trace', ?, 'captured', 'Alive spark', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO specs (id, project_id, title, status, created_at, updated_at)
VALUES ('spec-fossil', ?, 'Fossil spec', 'draft', ?, ?)
`, projectID, now, now)
	mustExec(t, store, `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES
  ('alias-spark-fossil-trace', ?, 'spark', 'spark-fossil-trace', 'spark', 'SPARK-FOSSIL-TRACE', ?, ?),
  ('alias-spec', ?, 'spec', 'spec-fossil', 'spec', 'SPEC-FOSSIL', ?, ?)
`, projectID, now, now, projectID, now, now)
	mustExec(t, store, `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, origin, created_at, updated_at)
VALUES
  ('rel-fossil', ?, 'spark', 'spark-fossil-trace', 'spec', 'spec-fossil', 'produced', 'fossil neighbor', 'command', ?, ?),
  ('rel-alive', ?, 'spark', 'spark-fossil-trace', 'spark', 'spark-fossil-trace', 'self', 'alive neighbor', 'manual', ?, ?)
`, projectID, now, now, projectID, now, now)

	trace, err := store.Trace(ctx, root, "SPARK-FOSSIL-TRACE")
	if err != nil {
		t.Fatalf("Trace() error = %v", err)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "produced", "spec", "SPEC-FOSSIL") {
		t.Fatalf("trace relationships = %#v, want local-archive spec neighbor hydrated", trace.Relationships)
	}
	if !hasStateTraceRelationship(trace.Relationships, "outbound", "self", "spark", "SPARK-FOSSIL-TRACE") {
		t.Fatalf("trace relationships = %#v, want alive spark self edge retained", trace.Relationships)
	}

	list, err := store.ListLinks(ctx, root, "SPARK-FOSSIL-TRACE")
	if err != nil {
		t.Fatalf("ListLinks() error = %v", err)
	}
	if !hasStateTraceRelationship(list.Relationships, "outbound", "produced", "spec", "SPEC-FOSSIL") {
		t.Fatalf("list relationships = %#v, want local-archive spec neighbor hydrated", list.Relationships)
	}
}
