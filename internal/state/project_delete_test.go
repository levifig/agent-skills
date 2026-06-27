package state

import (
	"context"
	"testing"
)

func TestDeleteProjectCascadesAcrossEntityTables(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "specs/SPEC-001-spec.md", `---
id: SPEC-001
title: A Spec
status: complete
---
# A Spec

Body content.
`)
	writeAgentsFile(t, root.Path(), "tasks/TASK-001-task.md", `---
id: TASK-001
title: A Task
status: todo
priority: P1
spec: SPEC-001
---
# A Task
`)
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{"TASK-001":{"title":"A Task","status":"todo","priority":"P1","spec":"SPEC-001"}}}`)
	if _, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	now := "2026-01-01T00:00:00Z"

	mustExec(t, store, `INSERT INTO journal_entries (id, project_id, entry_type, message, created_at, updated_at) VALUES (?, ?, 'decision', 'noted', ?, ?)`, "je-1", projectID, now, now)
	mustExec(t, store, `INSERT INTO journal_search (rowid, project_id, journal_entry_id, session_id, entry_type, scope, message) VALUES (1, ?, 'je-1', '', 'decision', '', 'noted')`, projectID)
	mustExec(t, store, `INSERT INTO sessions (id, project_id, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)`, "sess-1", projectID, now, now)
	mustExec(t, store, `INSERT INTO docs_index (id, project_id, path, content, content_hash, indexed_worktree, indexed_at, created_at, updated_at) VALUES (?, ?, 'README.md', 'doc body', 'hash', 'wt', ?, ?, ?)`, "doc-1", projectID, now, now, now)
	mustExec(t, store, `INSERT INTO docs_search (rowid, project_id, id, path, content) SELECT rowid, project_id, id, path, content FROM docs_index WHERE id = 'doc-1'`)

	tablesWithRows := []string{"specs", "tasks", "sources", "aliases", "artifact_bodies", "journal_entries", "sessions", "docs_index", "relationships"}
	for _, table := range tablesWithRows {
		if got := countRows(t, store, `SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, projectID); got == 0 {
			t.Fatalf("precondition: %s has 0 rows for project, want >0", table)
		}
	}

	result, err := store.DeleteProject(ctx, projectID)
	if err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if result.ProjectID != projectID || result.DatabaseScope != "global" {
		t.Fatalf("result identity incomplete: %#v", result)
	}

	allTables := append([]string{"artifact_bodies", "docs_index"}, projectScopedDeleteTables...)
	for _, table := range allTables {
		if got := countRows(t, store, `SELECT COUNT(*) FROM `+table+` WHERE project_id = ?`, projectID); got != 0 {
			t.Fatalf("after delete, %s has %d rows for project, want 0", table, got)
		}
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM projects WHERE id = ?`, projectID); got != 0 {
		t.Fatalf("projects row still present (%d), want 0", got)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM journal_search WHERE project_id = ?`, projectID); got != 0 {
		t.Fatalf("journal_search rows = %d after delete, want 0", got)
	}

	projects, err := store.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	for _, p := range projects.Projects {
		if p.ID == projectID {
			t.Fatalf("deleted project %s still listed", projectID)
		}
	}

	assertNoIntegrityViolations(t, store)
}

func TestDeleteProjectUnknownRef(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "TASKS.json", `{"tasks":{}}`)
	if _, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	if _, err := store.DeleteProject(ctx, "proj_does_not_exist"); err == nil {
		t.Fatal("DeleteProject() with unknown ref = nil error, want failure")
	}
}
