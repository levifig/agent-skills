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

	projectID := projectIDForTest(t, store, root)
	now := "2026-05-28T23:25:55Z"
	insertHousekeepingEntity(t, store, "specs", projectID, "spec-complete", "Complete Spec", "complete", now)
	insertHousekeepingEntity(t, store, "tasks", projectID, "task-done", "Done Task", "done", now)
	insertHousekeepingEntity(t, store, "ideas", projectID, "idea-resolved", "Resolved Idea", "resolved", now)
	insertHousekeepingSpark(t, store, projectID, "spark-resolved", "resolved spark", "resolved", now)
	insertHousekeepingEntity(t, store, "brainstorms", projectID, "brainstorm-archived", "Archived Brainstorm", "archived", now)
	insertHousekeepingEntity(t, store, "shaping_drafts", projectID, "draft-absorbed", "Absorbed Draft", "absorbed", now)
	insertHousekeepingReport(t, store, projectID, "report-final", "Final Report", "final", now)

	summary, err := Housekeeping(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Housekeeping() error = %v", err)
	}
	if summary.DatabasePath != result.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", summary.DatabasePath, result.DatabasePath)
	}
	assertTaskProjectContext(t, root.Path(), summary.ContractVersion, summary.DatabaseScope, summary.DatabasePath, summary.ProjectID, summary.ProjectName, summary.ProjectCurrentPath)
	for name, status := range map[string]string{
		"specs":          "complete",
		"tasks":          "done",
		"ideas":          "resolved",
		"sparks":         "resolved",
		"brainstorms":    "archived",
		"shaping_drafts": "absorbed",
		"reports":        "final",
	} {
		section := summary.Sections[name]
		if section.Total != 1 || section.ByStatus[status] != 1 || section.CleanupCandidate != 1 {
			t.Fatalf("section %s = %#v, want one %s cleanup candidate", name, section, status)
		}
	}
	if len(summary.Signals) != 7 {
		t.Fatalf("Signals = %#v, want one signal per populated cleanup section", summary.Signals)
	}
}

func TestHousekeepingCountsCanonicalDoneAsCleanupCandidate(t *testing.T) {
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

	projectID := projectIDForTest(t, store, root)
	now := "2026-07-23T00:00:00Z"
	insertHousekeepingEntity(t, store, "specs", projectID, "spec-complete", "Complete Spec", "complete", now)
	insertHousekeepingEntity(t, store, "specs", projectID, "spec-done", "Done Spec", "done", now)
	insertHousekeepingEntity(t, store, "ideas", projectID, "idea-resolved", "Resolved Idea", "resolved", now)
	insertHousekeepingEntity(t, store, "ideas", projectID, "idea-done", "Done Idea", "done", now)
	insertHousekeepingSpark(t, store, projectID, "spark-resolved", "resolved spark", "resolved", now)
	insertHousekeepingSpark(t, store, projectID, "spark-done", "done spark", "done", now)
	insertHousekeepingEntity(t, store, "brainstorms", projectID, "brainstorm-resolved", "Resolved Brainstorm", "resolved", now)
	insertHousekeepingEntity(t, store, "brainstorms", projectID, "brainstorm-done", "Done Brainstorm", "done", now)
	insertHousekeepingReport(t, store, projectID, "report-final", "Final Report", "final", now)
	insertHousekeepingReport(t, store, projectID, "report-done", "Done Report", "done", now)

	summary, err := Housekeeping(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Housekeeping() error = %v", err)
	}
	for _, name := range []string{"specs", "ideas", "sparks", "brainstorms", "reports"} {
		section := summary.Sections[name]
		if section.Total != 2 || section.ByStatus["done"] != 1 || section.CleanupCandidate != 2 {
			t.Fatalf("section %s = %#v, want legacy and canonical done cleanup candidates", name, section)
		}
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

func insertHousekeepingReport(t *testing.T, store *Store, projectID string, id string, title string, status string, now string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `INSERT INTO reports (id, project_id, report_kind, title, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, id, projectID, "audit", title, status, now, now); err != nil {
		t.Fatalf("insert report %s error = %v", id, err)
	}
}
