package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestImportAliasFirstRekeyReimportStable(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAliasFirstImportFixture(t, root.Path())

	first, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("first ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, first.DatabasePath)
	defer store.Close()

	beforeEntities := countProjectEntities(t, store, first.ProjectID)
	beforeSources := countTableWhere(t, store, `SELECT COUNT(*) FROM sources WHERE project_id = ?`, first.ProjectID)
	beforeAliasMap := aliasEntityMap(t, store, first.ProjectID)
	beforeEntityIDs := entityIDSet(t, store, first.ProjectID)
	beforeSourceIDs := sourceIDSet(t, store, first.ProjectID)

	if beforeEntities["specs"] < 1 || beforeEntities["tasks"] < 1 || beforeEntities["ideas"] < 1 {
		t.Fatalf("fixture import too thin: %#v", beforeEntities)
	}
	if beforeSources < 1 {
		t.Fatal("expected imported sources")
	}
	if len(beforeAliasMap) < 1 {
		t.Fatal("expected imported aliases")
	}

	newProjectID := "proj_aliasfirst_rekey_00000001"
	rekeyProjectLikeLegacy(t, store, first.ProjectID, newProjectID, root.Path())

	second, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if second.ProjectID != newProjectID {
		t.Fatalf("ProjectID after rekey reimport = %q, want %q", second.ProjectID, newProjectID)
	}

	afterEntities := countProjectEntities(t, store, newProjectID)
	afterSources := countTableWhere(t, store, `SELECT COUNT(*) FROM sources WHERE project_id = ?`, newProjectID)
	afterAliasMap := aliasEntityMap(t, store, newProjectID)
	afterEntityIDs := entityIDSet(t, store, newProjectID)
	afterSourceIDs := sourceIDSet(t, store, newProjectID)
	orphans := countAliasOrphans(t, store, newProjectID)

	if !mapsEqual(beforeEntities, afterEntities) {
		t.Fatalf("entity counts drifted after rekey re-import\nbefore=%#v\nafter=%#v", beforeEntities, afterEntities)
	}
	if afterSources != beforeSources {
		t.Fatalf("sources = %d, want %d", afterSources, beforeSources)
	}
	if orphans != 0 {
		t.Fatalf("alias orphans = %d, want 0", orphans)
	}
	if !mapsEqual(beforeAliasMap, afterAliasMap) {
		t.Fatalf("alias→entity map drifted\nbefore=%v\nafter=%v", beforeAliasMap, afterAliasMap)
	}
	if !stringSetsEqual(beforeEntityIDs, afterEntityIDs) {
		t.Fatalf("entity IDs changed\nbefore=%v\nafter=%v", sortedKeys(beforeEntityIDs), sortedKeys(afterEntityIDs))
	}
	if !stringSetsEqual(beforeSourceIDs, afterSourceIDs) {
		t.Fatalf("source IDs changed\nbefore=%v\nafter=%v", sortedKeys(beforeSourceIDs), sortedKeys(afterSourceIDs))
	}

	// Derived IDs under the new project ID must not have been minted as twins.
	for namespace, entityID := range afterAliasMap {
		parts := strings.SplitN(namespace, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		kind, alias := parts[0], parts[1]
		twin := stableMigrationID(kind, newProjectID, alias)
		if twin == entityID {
			continue
		}
		var count int
		if err := store.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE project_id = ? AND id = ?`, entityTableForKind(kind)), newProjectID, twin).Scan(&count); err != nil {
			t.Fatalf("count twin %s: %v", twin, err)
		}
		if count != 0 {
			t.Fatalf("twin row minted for %s/%s: %s", kind, alias, twin)
		}
	}
}

func TestImportAliasFirstIdempotentNoChurn(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAliasFirstImportFixture(t, root.Path())

	first, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("first ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, first.DatabasePath)
	defer store.Close()

	beforeDump := logicalBusinessDump(t, store)
	beforeEntities := countProjectEntities(t, store, first.ProjectID)
	beforeSources := countTableWhere(t, store, `SELECT COUNT(*) FROM sources WHERE project_id = ?`, first.ProjectID)
	beforeAliasMap := aliasEntityMap(t, store, first.ProjectID)

	second, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if second.ProjectID != first.ProjectID {
		t.Fatalf("ProjectID changed: %q -> %q", first.ProjectID, second.ProjectID)
	}

	afterDump := logicalBusinessDump(t, store)
	if beforeDump != afterDump {
		t.Fatalf("canonical business dump drifted on no-op re-import\nbefore:\n%s\nafter:\n%s", beforeDump, afterDump)
	}
	if !mapsEqual(beforeEntities, countProjectEntities(t, store, first.ProjectID)) {
		t.Fatalf("entity counts drifted on no-op re-import")
	}
	if got := countTableWhere(t, store, `SELECT COUNT(*) FROM sources WHERE project_id = ?`, first.ProjectID); got != beforeSources {
		t.Fatalf("sources = %d, want %d", got, beforeSources)
	}
	if !mapsEqual(beforeAliasMap, aliasEntityMap(t, store, first.ProjectID)) {
		t.Fatalf("alias→entity map drifted on no-op re-import")
	}
	if orphans := countAliasOrphans(t, store, first.ProjectID); orphans != 0 {
		t.Fatalf("alias orphans after idempotent re-import = %d, want 0", orphans)
	}
}

func TestImportAliasFirstDoesNotRepointLiveAlias(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAliasFirstImportFixture(t, root.Path())

	first, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, first.DatabasePath)
	defer store.Close()

	var liveEntityID string
	if err := store.db.QueryRowContext(ctx, `
SELECT entity_id FROM aliases
WHERE project_id = ? AND namespace = 'spec' AND alias = 'SPEC-001'
`, first.ProjectID).Scan(&liveEntityID); err != nil {
		t.Fatalf("read SPEC-001 alias: %v", err)
	}

	// Force project_id rewrite without changing entity IDs, then re-import.
	newProjectID := "proj_aliasfirst_repoint_000001"
	rekeyProjectLikeLegacy(t, store, first.ProjectID, newProjectID, root.Path())

	if _, err := ApplyMarkdownMigration(ctx, root, resolver); err != nil {
		t.Fatalf("re-import error = %v", err)
	}

	var afterEntityID string
	if err := store.db.QueryRowContext(ctx, `
SELECT entity_id FROM aliases
WHERE project_id = ? AND namespace = 'spec' AND alias = 'SPEC-001'
`, newProjectID).Scan(&afterEntityID); err != nil {
		t.Fatalf("read SPEC-001 alias after reimport: %v", err)
	}
	if afterEntityID != liveEntityID {
		t.Fatalf("alias re-pointed: %q -> %q", liveEntityID, afterEntityID)
	}
	var rowCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM specs WHERE project_id = ?`, newProjectID).Scan(&rowCount); err != nil {
		t.Fatalf("count specs: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("specs rows = %d, want 1 (no twin)", rowCount)
	}
}

func writeAliasFirstImportFixture(t *testing.T, root string) {
	t.Helper()
	writeAgentsFile(t, root, "specs/SPEC-001-example.md", `---
id: SPEC-001
title: Alias First Spec
status: implementing
---
# Alias First Spec
`)
	writeAgentsFile(t, root, "tasks/TASK-001-example.md", `---
id: TASK-001
title: Alias First Task
status: todo
---
# Alias First Task
`)
	writeAgentsFile(t, root, "ideas/20260528-alias-idea.md", `---
id: idea-alias-first
title: Alias First Idea
---
# Alias First Idea
`)
	writeAgentsFile(t, root, "drafts/20260528-brainstorm-alias.md", `---
id: brainstorm-alias-first
title: Alias First Brainstorm
---
# Alias First Brainstorm
`)
	writeAgentsFile(t, root, "reports/report-alias.md", `---
id: report-alias-first
title: Alias First Report
status: final
---
# Alias First Report
`)
	writeAgentsFile(t, root, "sessions/20260528-session.md", `---
branch: feature/alias-first
---
[2026-05-28 10:00] spark(scope): aliasfirst-spark capture this
`)
	writeAgentsFile(t, root, "TASKS.json", `{
  "tasks": {
    "TASK-001": {
      "title": "Alias First Task",
      "spec": "SPEC-001",
      "status": "todo",
      "priority": "P1"
    }
  },
  "specs": {
    "SPEC-001": {
      "title": "Alias First Spec",
      "status": "implementing"
    }
  }
}
`)
}

// rekeyProjectLikeLegacy mirrors rekeyLegacyProjectTx: insert opaque project,
// rewrite project_id on projectScopedRekeyTables, swap projects row.
// Also moves artifact_bodies and journal_origins so FK delete of the old
// project row succeeds (those tables are populated by import but absent from
// projectScopedRekeyTables).
func rekeyProjectLikeLegacy(t *testing.T, store *Store, legacyID string, nextID string, currentPath string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin rekey: %v", err)
	}
	defer tx.Rollback()

	var createdAt string
	var friendly sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT created_at, friendly_name FROM projects WHERE id = ?`, legacyID).Scan(&createdAt, &friendly); err != nil {
		t.Fatalf("read legacy project: %v", err)
	}
	friendlyName := "project"
	if friendly.Valid && strings.TrimSpace(friendly.String) != "" {
		friendlyName = friendly.String
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, nextID, nextID, friendlyName, currentPath, now, createdAt, now); err != nil {
		t.Fatalf("insert rekeyed project: %v", err)
	}
	for _, table := range projectScopedRekeyTables() {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET project_id = ? WHERE project_id = ?`, table), nextID, legacyID); err != nil {
			t.Fatalf("rekey %s: %v", table, err)
		}
	}
	for _, table := range []string{"artifact_bodies", "journal_origins"} {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET project_id = ? WHERE project_id = ?`, table), nextID, legacyID); err != nil {
			t.Fatalf("rekey %s: %v", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, legacyID); err != nil {
		t.Fatalf("delete legacy project: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit rekey: %v", err)
	}
}

func countProjectEntities(t *testing.T, store *Store, projectID string) map[string]int {
	t.Helper()
	tables := []string{"specs", "tasks", "ideas", "brainstorms", "reports", "sparks", "shaping_drafts"}
	out := make(map[string]int, len(tables))
	for _, table := range tables {
		out[table] = countTableWhere(t, store, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE project_id = ?`, table), projectID)
	}
	return out
}

func countTableWhere(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := store.db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}

func aliasEntityMap(t *testing.T, store *Store, projectID string) map[string]string {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `
SELECT namespace, alias, entity_id
FROM aliases
WHERE project_id = ?
ORDER BY namespace, alias
`, projectID)
	if err != nil {
		t.Fatalf("query aliases: %v", err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var namespace, alias, entityID string
		if err := rows.Scan(&namespace, &alias, &entityID); err != nil {
			t.Fatalf("scan alias: %v", err)
		}
		out[namespace+"\x00"+alias] = entityID
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("aliases rows: %v", err)
	}
	return out
}

func entityIDSet(t *testing.T, store *Store, projectID string) map[string]struct{} {
	t.Helper()
	out := map[string]struct{}{}
	for _, table := range []string{"specs", "tasks", "ideas", "brainstorms", "reports", "sparks", "shaping_drafts"} {
		rows, err := store.db.QueryContext(context.Background(), fmt.Sprintf(`SELECT id FROM %s WHERE project_id = ?`, table), projectID)
		if err != nil {
			t.Fatalf("query %s ids: %v", table, err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatalf("scan %s id: %v", table, err)
			}
			out[table+":"+id] = struct{}{}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatalf("%s rows: %v", table, err)
		}
	}
	return out
}

func sourceIDSet(t *testing.T, store *Store, projectID string) map[string]struct{} {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `SELECT id FROM sources WHERE project_id = ?`, projectID)
	if err != nil {
		t.Fatalf("query sources: %v", err)
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan source id: %v", err)
		}
		out[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("sources rows: %v", err)
	}
	return out
}

func countAliasOrphans(t *testing.T, store *Store, projectID string) int {
	t.Helper()
	// Entity row with no matching alias for its kind/namespace.
	total := 0
	for kind, table := range map[string]string{
		"spec":       "specs",
		"task":       "tasks",
		"idea":       "ideas",
		"brainstorm": "brainstorms",
		"report":     "reports",
		"spark":      "sparks",
	} {
		var n int
		query := fmt.Sprintf(`
SELECT COUNT(*)
FROM %s AS e
WHERE e.project_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM aliases AS a
    WHERE a.project_id = e.project_id
      AND a.entity_kind = ?
      AND a.entity_id = e.id
      AND a.namespace = ?
  )
`, table)
		if err := store.db.QueryRowContext(context.Background(), query, projectID, kind, kind).Scan(&n); err != nil {
			t.Fatalf("count orphans %s: %v", table, err)
		}
		total += n
	}
	return total
}

func entityTableForKind(kind string) string {
	switch kind {
	case "spec":
		return "specs"
	case "task":
		return "tasks"
	case "idea":
		return "ideas"
	case "brainstorm":
		return "brainstorms"
	case "report":
		return "reports"
	case "spark":
		return "sparks"
	case "shaping_draft":
		return "shaping_drafts"
	default:
		return kind + "s"
	}
}

func mapsEqual[K comparable, V comparable](a, b map[K]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func stringSetsEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// A spark's alias is the message's first word, so unrelated sparks collide on
// it routinely. Alias-first resolution must never let the second one overwrite
// the first one's text.
func TestImportAliasFirstKeepsCollidingSparksDistinct(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAgentsFile(t, root.Path(), "sessions/20260528-sparks.md", `---
branch: feature/sparks
---
[2026-05-28 10:00] spark(scope): dedupe the state tables one day
[2026-05-28 10:05] spark(scope): dedupe something entirely different
`)

	result, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, result.DatabasePath)
	defer store.Close()

	texts := sparkTexts(t, store, result.ProjectID)
	if len(texts) != 2 {
		t.Fatalf("spark rows = %v, want both journal lines preserved", texts)
	}
	for _, want := range []string{"dedupe the state tables one day", "dedupe something entirely different"} {
		if _, ok := texts[want]; !ok {
			t.Fatalf("spark %q missing from %v", want, texts)
		}
	}

	// Both rows keep an alias of their own: the second spark takes a numbered
	// alias instead of evicting the first, so neither is orphaned.
	if orphans := countAliasOrphans(t, store, result.ProjectID); orphans != 0 {
		t.Fatalf("alias orphans after a first import = %d, want 0", orphans)
	}
	aliases := aliasEntityMap(t, store, result.ProjectID)
	for _, want := range []string{"spark\x00SPARK-dedupe", "spark\x00SPARK-dedupe-2"} {
		if _, ok := aliases[want]; !ok {
			t.Fatalf("alias %q missing from %v", want, aliases)
		}
	}

	// Re-import is still idempotent: no third row, no rewritten text, no churn
	// in which spark holds which alias.
	if _, err := ApplyMarkdownMigration(ctx, root, resolver); err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if again := sparkTexts(t, store, result.ProjectID); len(again) != 2 {
		t.Fatalf("spark rows after re-import = %v, want 2", again)
	}
	if orphans := countAliasOrphans(t, store, result.ProjectID); orphans != 0 {
		t.Fatalf("alias orphans after re-import = %d, want 0", orphans)
	}
	if !mapsEqual(aliases, aliasEntityMap(t, store, result.ProjectID)) {
		t.Fatalf("alias→entity map drifted on re-import\nbefore=%v\nafter=%v", aliases, aliasEntityMap(t, store, result.ProjectID))
	}
}

// A journal file may carry the same spark line twice. Those are two intake
// items: identity resolution must not match the second onto the row the first
// line just minted.
func TestImportAliasFirstKeepsRepeatedSparkLinesDistinct(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAgentsFile(t, root.Path(), "sessions/20260613-repeats.md", `---
branch: feature/sparks
---
[2026-06-13 10:00] spark(x): widget idea worth keeping
[2026-06-13 10:10] spark(x): widget idea worth keeping
`)

	result, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, result.DatabasePath)
	defer store.Close()

	if got := sparkRowCount(t, store, result.ProjectID); got != 2 {
		t.Fatalf("spark rows = %d, want one per journal line", got)
	}
	if orphans := countAliasOrphans(t, store, result.ProjectID); orphans != 0 {
		t.Fatalf("alias orphans after a first import = %d, want 0", orphans)
	}
	aliases := aliasEntityMap(t, store, result.ProjectID)
	for _, want := range []string{"spark\x00SPARK-widget", "spark\x00SPARK-widget-2"} {
		if _, ok := aliases[want]; !ok {
			t.Fatalf("alias %q missing from %v", want, aliases)
		}
	}

	// Re-import adds nothing and reshuffles nothing.
	if _, err := ApplyMarkdownMigration(ctx, root, resolver); err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if got := sparkRowCount(t, store, result.ProjectID); got != 2 {
		t.Fatalf("spark rows after re-import = %d, want 2", got)
	}
	if !mapsEqual(aliases, aliasEntityMap(t, store, result.ProjectID)) {
		t.Fatalf("alias→entity map drifted on re-import\nbefore=%v\nafter=%v", aliases, aliasEntityMap(t, store, result.ProjectID))
	}
}

// A colliding spark that had to take a numbered alias is still found by the
// rekey re-import: identity is looked up by content, not by the base alias.
func TestImportAliasFirstCollidingSparksSurviveRekeyReimport(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAgentsFile(t, root.Path(), "sessions/20260528-sparks.md", `---
branch: feature/sparks
---
[2026-05-28 10:00] spark(scope): dedupe the state tables one day
[2026-05-28 10:05] spark(scope): dedupe something entirely different
`)

	first, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("first ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, first.DatabasePath)
	defer store.Close()
	beforeIDs := entityIDSet(t, store, first.ProjectID)

	newProjectID := "proj_sparkcollision_00000001"
	rekeyProjectLikeLegacy(t, store, first.ProjectID, newProjectID, root.Path())

	if _, err := ApplyMarkdownMigration(ctx, root, resolver); err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if orphans := countAliasOrphans(t, store, newProjectID); orphans != 0 {
		t.Fatalf("alias orphans after rekey re-import = %d, want 0", orphans)
	}
	if afterIDs := entityIDSet(t, store, newProjectID); !stringSetsEqual(beforeIDs, afterIDs) {
		t.Fatalf("entity IDs changed across rekey re-import\nbefore=%v\nafter=%v", sortedKeys(beforeIDs), sortedKeys(afterIDs))
	}
}

// The rekey that caused the damage must not fork spark identity either.
func TestImportAliasFirstSparkSurvivesRekeyReimport(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	writeAgentsFile(t, root.Path(), "sessions/20260528-one-spark.md", `---
branch: feature/sparks
---
[2026-05-28 10:00] spark(scope): dedupe the state tables one day
`)

	first, err := ApplyMarkdownMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("first ApplyMarkdownMigration() error = %v", err)
	}
	store := openStoreAt(t, first.DatabasePath)
	defer store.Close()
	beforeIDs := entityIDSet(t, store, first.ProjectID)

	newProjectID := "proj_sparkrekey_000000000001"
	rekeyProjectLikeLegacy(t, store, first.ProjectID, newProjectID, root.Path())

	if _, err := ApplyMarkdownMigration(ctx, root, resolver); err != nil {
		t.Fatalf("second ApplyMarkdownMigration() error = %v", err)
	}
	if orphans := countAliasOrphans(t, store, newProjectID); orphans != 0 {
		t.Fatalf("alias orphans after rekey re-import = %d, want 0", orphans)
	}
	if afterIDs := entityIDSet(t, store, newProjectID); !stringSetsEqual(beforeIDs, afterIDs) {
		t.Fatalf("entity IDs changed across rekey re-import\nbefore=%v\nafter=%v", sortedKeys(beforeIDs), sortedKeys(afterIDs))
	}
}

func sparkRowCount(t *testing.T, store *Store, projectID string) int {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM sparks WHERE project_id = ?`, projectID).Scan(&count); err != nil {
		t.Fatalf("count sparks: %v", err)
	}
	return count
}

func sparkTexts(t *testing.T, store *Store, projectID string) map[string]struct{} {
	t.Helper()
	rows, err := store.db.QueryContext(context.Background(), `SELECT text FROM sparks WHERE project_id = ?`, projectID)
	if err != nil {
		t.Fatalf("list sparks: %v", err)
	}
	defer rows.Close()
	texts := map[string]struct{}{}
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			t.Fatalf("scan spark: %v", err)
		}
		texts[text] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list sparks: %v", err)
	}
	return texts
}
