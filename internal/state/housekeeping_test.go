package state

import (
	"context"
	"testing"
)

func TestHousekeepingSummarizesSQLiteLifecycleState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	result, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	projectID := ProjectID(root)
	now := "2026-05-28T23:25:55Z"
	insertHousekeepingEntity(t, store, "specs", projectID, "spec-complete", "Complete Spec", "complete", now)
	insertHousekeepingEntity(t, store, "tasks", projectID, "task-done", "Done Task", "done", now)
	insertHousekeepingEntity(t, store, "ideas", projectID, "idea-resolved", "Resolved Idea", "resolved", now)
	insertHousekeepingSpark(t, store, projectID, "spark-resolved", "resolved spark", "resolved", now)
	insertHousekeepingEntity(t, store, "brainstorms", projectID, "brainstorm-archived", "Archived Brainstorm", "archived", now)
	insertHousekeepingEntity(t, store, "shaping_drafts", projectID, "draft-absorbed", "Absorbed Draft", "absorbed", now)
	insertHousekeepingSession(t, store, projectID, "session-done", "done", now)
	insertHousekeepingReport(t, store, projectID, "report-final", "Final Report", "final", now)

	summary, err := Housekeeping(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Housekeeping() error = %v", err)
	}
	if summary.DatabasePath != result.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", summary.DatabasePath, result.DatabasePath)
	}
	for name, status := range map[string]string{
		"specs":          "complete",
		"tasks":          "done",
		"ideas":          "resolved",
		"sparks":         "resolved",
		"brainstorms":    "archived",
		"shaping_drafts": "absorbed",
		"sessions":       "done",
		"reports":        "final",
	} {
		section := summary.Sections[name]
		if section.Total != 1 || section.ByStatus[status] != 1 || section.CleanupCandidate != 1 {
			t.Fatalf("section %s = %#v, want one %s cleanup candidate", name, section, status)
		}
	}
	if len(summary.Signals) != 8 {
		t.Fatalf("Signals = %#v, want one signal per populated cleanup section", summary.Signals)
	}
}

func insertHousekeepingEntity(t *testing.T, store *Store, table string, projectID string, id string, title string, status string, now string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `INSERT INTO `+table+` (id, project_id, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, projectID, title, status, now, now); err != nil {
		t.Fatalf("insert %s %s error = %v", table, id, err)
	}
}

func insertHousekeepingSpark(t *testing.T, store *Store, projectID string, id string, text string, status string, now string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `INSERT INTO sparks (id, project_id, text, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, id, projectID, text, status, now, now); err != nil {
		t.Fatalf("insert spark %s error = %v", id, err)
	}
}

func insertHousekeepingSession(t *testing.T, store *Store, projectID string, id string, status string, now string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `INSERT INTO sessions (id, project_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, projectID, status, now, now); err != nil {
		t.Fatalf("insert session %s error = %v", id, err)
	}
}

func insertHousekeepingReport(t *testing.T, store *Store, projectID string, id string, title string, status string, now string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `INSERT INTO reports (id, project_id, report_kind, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, projectID, "audit", title, status, now, now); err != nil {
		t.Fatalf("insert report %s error = %v", id, err)
	}
}
