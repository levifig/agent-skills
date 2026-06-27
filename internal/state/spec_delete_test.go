package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteSpecCascadesEveryDependentRow(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-target.md", `---
id: SPEC-001
title: Target Spec
status: complete
---
# Target Spec

Searchable spec body content.
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-task.md", `---
id: TASK-001
title: Linked Task
status: todo
priority: P1
spec: SPEC-001
---
# Linked Task
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"Linked Task","status":"todo","priority":"P1","spec":"SPEC-001"}}}`)
	if _, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	specID := stableMigrationID("spec", projectID, "SPEC-001")
	now := "2026-01-01T00:00:00Z"

	// Insert additional dependent rows the markdown migration does not produce.
	mustExec(t, store, `INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, created_at, updated_at) VALUES (?, ?, 'spec', ?, 'created', ?, ?)`, "evt-spec", projectID, specID, now, now)
	mustExec(t, store, `INSERT INTO tags (id, project_id, name, created_at, updated_at) VALUES (?, ?, 'urgent', ?, ?)`, "tag-1", projectID, now, now)
	mustExec(t, store, `INSERT INTO entity_tags (id, project_id, tag_id, entity_kind, entity_id, created_at, updated_at) VALUES (?, ?, 'tag-1', 'spec', ?, ?, ?)`, "etag-1", projectID, specID, now, now)
	mustExec(t, store, `INSERT INTO bundles (id, project_id, slug, title, created_at, updated_at) VALUES (?, ?, 'b1', 'Bundle', ?, ?)`, "bundle-1", projectID, now, now)
	mustExec(t, store, `INSERT INTO bundle_members (id, project_id, bundle_id, entity_kind, entity_id, created_at, updated_at) VALUES (?, ?, 'bundle-1', 'spec', ?, ?, ?)`, "bm-1", projectID, specID, now, now)
	mustExec(t, store, `INSERT INTO backend_mappings (id, project_id, backend, entity_kind, entity_id, external_kind, external_id, sync_status, created_at, updated_at) VALUES (?, ?, 'linear', 'spec', ?, 'issue', 'EXT-1', 'synced', ?, ?)`, "bmap-1", projectID, specID, now, now)
	mustExec(t, store, `INSERT INTO exports (id, project_id, export_kind, format, path, source_entity_kind, source_entity_id, generated_at, created_at, updated_at) VALUES (?, ?, 'render', 'markdown', 'out.md', 'spec', ?, ?, ?, ?)`, "exp-1", projectID, specID, now, now, now)
	mustExec(t, store, `INSERT INTO plans (id, project_id, spec_id, title, status, created_at, updated_at) VALUES (?, ?, ?, 'Plan', 'open', ?, ?)`, "plan-1", projectID, specID, now, now)
	mustExec(t, store, `INSERT INTO councils (id, project_id, spec_id, title, status, created_at, updated_at) VALUES (?, ?, ?, 'Council', 'open', ?, ?)`, "council-1", projectID, specID, now, now)
	mustExec(t, store, `INSERT INTO journal_entries (id, project_id, entry_type, message, spec_id, created_at, updated_at) VALUES (?, ?, 'decision', 'noted', ?, ?, ?)`, "je-1", projectID, specID, now, now)

	result, err := store.DeleteSpec(ctx, root, "SPEC-001")
	if err != nil {
		t.Fatalf("DeleteSpec() error = %v", err)
	}
	if result.Spec == nil || result.Spec.ID != specID {
		t.Fatalf("result.Spec = %#v, want spec %s", result.Spec, specID)
	}
	if result.ContractVersion != StateJSONContractVersion || result.DatabaseScope != "global" || result.ProjectID == "" {
		t.Fatalf("result identity incomplete: %#v", result)
	}

	// Every dependent row referencing the spec must be gone.
	cascade := []struct {
		name  string
		query string
		args  []any
	}{
		{"specs", `SELECT COUNT(*) FROM specs WHERE project_id = ? AND id = ?`, []any{projectID, specID}},
		{"aliases", `SELECT COUNT(*) FROM aliases WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
		{"artifact_bodies", `SELECT COUNT(*) FROM artifact_bodies WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
		{"events", `SELECT COUNT(*) FROM events WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
		{"entity_tags", `SELECT COUNT(*) FROM entity_tags WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
		{"bundle_members", `SELECT COUNT(*) FROM bundle_members WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
		{"backend_mappings", `SELECT COUNT(*) FROM backend_mappings WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
		{"exports", `SELECT COUNT(*) FROM exports WHERE project_id = ? AND source_entity_kind = 'spec' AND source_entity_id = ?`, []any{projectID, specID}},
		{"relationships", `SELECT COUNT(*) FROM relationships WHERE project_id = ? AND ((from_entity_kind = 'spec' AND from_entity_id = ?) OR (to_entity_kind = 'spec' AND to_entity_id = ?))`, []any{projectID, specID, specID}},
		{"artifact_search", `SELECT COUNT(*) FROM artifact_search WHERE project_id = ? AND entity_kind = 'spec' AND entity_id = ?`, []any{projectID, specID}},
	}
	for _, c := range cascade {
		if got := countRows(t, store, c.query, c.args...); got != 0 {
			t.Fatalf("after delete, %s has %d rows for spec, want 0", c.name, got)
		}
	}

	// The spec's source row must be gone, but the task's source must remain.
	if got := countRows(t, store, `SELECT COUNT(*) FROM sources WHERE project_id = ? AND path LIKE '%SPEC-001%'`, projectID); got != 0 {
		t.Fatalf("spec source still present (%d rows), want 0", got)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM sources WHERE project_id = ? AND path LIKE '%TASK-001%'`, projectID); got != 1 {
		t.Fatalf("task source rows = %d, want 1 (must not be deleted)", got)
	}

	// First-class entities survive with the spec link cleared, not deleted.
	survivors := []struct {
		name  string
		table string
	}{
		{"tasks", "tasks"},
		{"plans", "plans"},
		{"councils", "councils"},
		{"journal_entries", "journal_entries"},
	}
	for _, s := range survivors {
		if got := countRows(t, store, `SELECT COUNT(*) FROM `+s.table+` WHERE project_id = ?`, projectID); got == 0 {
			t.Fatalf("%s rows = 0 after delete, want survivor preserved", s.name)
		}
		if got := countRows(t, store, `SELECT COUNT(*) FROM `+s.table+` WHERE project_id = ? AND spec_id = ?`, projectID, specID); got != 0 {
			t.Fatalf("%s still references deleted spec (%d rows), want spec_id cleared", s.name, got)
		}
	}

	assertNoIntegrityViolations(t, store)
}

func TestDeleteSpecRetainsOnDiskRender(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	specPath := "specs/SPEC-007-render.md"
	writeAgentsFile(t, root.Path(), specPath, `---
id: SPEC-007
title: Rendered Spec
status: complete
---
# Rendered Spec
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()

	result, err := store.DeleteSpec(ctx, root, "SPEC-007")
	if err != nil {
		t.Fatalf("DeleteSpec() error = %v", err)
	}
	onDisk := filepath.Join(root.Path(), ".agents", specPath)
	if !result.RenderRetained || result.RenderPath != onDisk {
		t.Fatalf("RenderPath = %q retained=%v, want %q retained", result.RenderPath, result.RenderRetained, onDisk)
	}
	if _, err := os.Stat(onDisk); err != nil {
		t.Fatalf("on-disk render must remain after DB delete: %v", err)
	}
}

func TestDeleteSpecRejectsNonSpecRef(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-task.md", `---
id: TASK-001
title: A Task
status: todo
priority: P1
---
# A Task
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"A Task","status":"todo","priority":"P1"}}}`)
	if _, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	if _, err := store.DeleteSpec(ctx, root, "TASK-001"); err == nil {
		t.Fatal("DeleteSpec() on a task ref = nil error, want failure")
	}
}

func mustExec(t *testing.T, store *Store, query string, args ...any) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func assertNoIntegrityViolations(t *testing.T, store *Store) {
	t.Helper()
	var integrity string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatalf("integrity_check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}
	rows, err := store.db.QueryContext(context.Background(), `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported violations, want none")
	}
}
