package state

import (
	"context"
	"os"
	"testing"
)

func TestPreviewLifecycleStatusMigrationUsesCopyRun(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedLegacyLifecycleStatusRows(t, status.DatabasePath, status.ProjectID)

	result, err := PreviewLifecycleStatusMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewLifecycleStatusMigration() error = %v", err)
	}
	if result.Action != LifecycleStatusMigrationActionDryRun || result.Applied || !result.CopyRun {
		t.Fatalf("preview action/applied/copy_run = %q/%t/%t, want dry-run/false/true", result.Action, result.Applied, result.CopyRun)
	}
	if result.EntitiesRewritten != 9 || result.EventsRewritten != 3 || result.NormalizationEvents != 9 || result.LegacyStatusesRemaining != 0 {
		t.Fatalf("preview counts = %#v, want 9 entities, 3 events, 9 normalization events, 0 remaining in copy", result)
	}
	if got := rawLifecycleStatus(t, status.DatabasePath, "reports", "report-legacy"); got != "final" {
		t.Fatalf("live report status after preview = %q, want final", got)
	}
	if got := rawLifecycleEventToStatus(t, status.DatabasePath, "event-report-final"); got != "final" {
		t.Fatalf("live event to_status after preview = %q, want final", got)
	}
	if got := rawLifecycleNormalizationEventCount(t, status.DatabasePath, status.ProjectID); got != 0 {
		t.Fatalf("live normalization event count after preview = %d, want 0", got)
	}
}

func TestApplyAndRollbackLifecycleStatusMigration(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedLegacyLifecycleStatusRows(t, status.DatabasePath, status.ProjectID)

	applied, err := ApplyLifecycleStatusMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyLifecycleStatusMigration() error = %v", err)
	}
	if !applied.Applied || applied.CopyRun || applied.Action != LifecycleStatusMigrationActionApply {
		t.Fatalf("apply action/applied/copy_run = %q/%t/%t, want apply/true/false", applied.Action, applied.Applied, applied.CopyRun)
	}
	if applied.BackupPath == "" || applied.RollbackManifestPath == "" {
		t.Fatalf("apply backup/manifest paths = %q/%q, want both populated", applied.BackupPath, applied.RollbackManifestPath)
	}
	if _, err := os.Stat(applied.BackupPath); err != nil {
		t.Fatalf("stat backup path %q: %v", applied.BackupPath, err)
	}
	if _, err := os.Stat(applied.RollbackManifestPath); err != nil {
		t.Fatalf("stat rollback manifest path %q: %v", applied.RollbackManifestPath, err)
	}
	if applied.EntitiesRewritten != 9 || applied.EventsRewritten != 3 || applied.NormalizationEvents != 9 || applied.LegacyStatusesRemaining != 0 {
		t.Fatalf("apply counts = %#v, want 9 entities, 3 events, 9 normalization events, 0 remaining", applied)
	}
	if got := rawLifecycleStatus(t, status.DatabasePath, "reports", "report-legacy"); got != LifecycleStatusDone {
		t.Fatalf("report status after apply = %q, want done", got)
	}
	if got := rawLifecycleStatus(t, status.DatabasePath, "sessions", "session-legacy"); got != LifecycleStatusInProgress {
		t.Fatalf("session status after apply = %q, want in_progress", got)
	}
	if got := rawLifecycleEventToStatus(t, status.DatabasePath, "event-report-final"); got != LifecycleStatusDone {
		t.Fatalf("report event to_status after apply = %q, want done", got)
	}
	if got := rawLifecycleEventFromStatus(t, status.DatabasePath, "event-report-final"); got != "draft" {
		t.Fatalf("report event from_status after apply = %q, want preserved draft", got)
	}
	if got := rawLifecycleEventToStatus(t, status.DatabasePath, "event-session-stopped"); got != LifecycleStatusPaused {
		t.Fatalf("session event to_status after apply = %q, want paused", got)
	}
	if got := rawLifecycleEventToStatus(t, status.DatabasePath, "event-finding-ignored"); got != "triaged" {
		t.Fatalf("non-lifecycle event to_status after apply = %q, want triaged", got)
	}
	if got := rawLifecycleNormalizationEventCount(t, status.DatabasePath, status.ProjectID); got != 9 {
		t.Fatalf("normalization event count after apply = %d, want 9", got)
	}

	rolledBack, err := RollbackLifecycleStatusMigration(ctx, root, PathResolver{StateHome: stateHome}, applied.RollbackManifestPath)
	if err != nil {
		t.Fatalf("RollbackLifecycleStatusMigration() error = %v", err)
	}
	if !rolledBack.Applied || rolledBack.Action != LifecycleStatusMigrationActionRollback {
		t.Fatalf("rollback action/applied = %q/%t, want rollback/true", rolledBack.Action, rolledBack.Applied)
	}
	if rolledBack.RollbackEntitiesRestored != 9 || rolledBack.RollbackEventsRestored != 3 || rolledBack.LegacyStatusesRemaining != 12 {
		t.Fatalf("rollback counts = %#v, want 9 entities restored, 3 events restored, 12 legacy values remaining", rolledBack)
	}
	if got := rawLifecycleStatus(t, status.DatabasePath, "reports", "report-legacy"); got != "final" {
		t.Fatalf("report status after rollback = %q, want final", got)
	}
	if got := rawLifecycleStatus(t, status.DatabasePath, "sessions", "session-legacy"); got != "active" {
		t.Fatalf("session status after rollback = %q, want active", got)
	}
	if got := rawLifecycleEventToStatus(t, status.DatabasePath, "event-report-final"); got != "final" {
		t.Fatalf("report event to_status after rollback = %q, want final", got)
	}
	if got := rawLifecycleEventToStatus(t, status.DatabasePath, "event-session-stopped"); got != "stopped" {
		t.Fatalf("session event to_status after rollback = %q, want stopped", got)
	}
	if got := rawLifecycleNormalizationEventCount(t, status.DatabasePath, status.ProjectID); got != 0 {
		t.Fatalf("normalization event count after rollback = %d, want 0", got)
	}
}

func seedLegacyLifecycleStatusRows(t *testing.T, databasePath string, projectID string) {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	now := "2026-06-25T00:00:00Z"
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO specs (id, project_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, ?, ?, NULL, ?, ?)`, []any{"spec-legacy", projectID, "Legacy Spec", "implementing", now, now}},
		{`INSERT INTO tasks (id, project_id, spec_id, title, status, priority, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, NULL, NULL, ?, ?)`, []any{"task-canonical", projectID, "Canonical Task", LifecycleStatusTodo, now, now}},
		{`INSERT INTO ideas (id, project_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, ?, ?, NULL, ?, ?)`, []any{"idea-legacy", projectID, "Legacy Idea", "resolved", now, now}},
		{`INSERT INTO sparks (id, project_id, scope, status, text, source_id, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, NULL, ?, ?)`, []any{"spark-legacy", projectID, "resolved", "Legacy spark", now, now}},
		{`INSERT INTO brainstorms (id, project_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, ?, ?, NULL, ?, ?)`, []any{"brainstorm-legacy", projectID, "Legacy Brainstorm", "resolved", now, now}},
		{`INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, NULL, ?, ?)`, []any{"session-legacy", projectID, "main", "active", now, now}},
		{`INSERT INTO reports (id, project_id, report_kind, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, NULL, ?, ?)`, []any{"report-legacy", projectID, "audit", "Legacy Report", "final", now, now}},
		{`INSERT INTO plans (id, project_id, spec_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, NULL, ?, ?)`, []any{"plan-legacy", projectID, "Legacy Plan", "final", now, now}},
		{`INSERT INTO handoffs (id, project_id, session_id, task_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, NULL, ?, ?, NULL, ?, ?)`, []any{"handoff-legacy", projectID, "Legacy Handoff", "final", now, now}},
		{`INSERT INTO councils (id, project_id, spec_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, NULL, ?, ?)`, []any{"council-legacy", projectID, "Legacy Council", "final", now, now}},
		{`INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"event-spec-complete", projectID, "spec", "spec-legacy", "status_changed", "implementing", "complete", "legacy event", now, now}},
		{`INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"event-report-final", projectID, "report", "report-legacy", "status_changed", "draft", "final", "legacy event", now, now}},
		{`INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"event-session-stopped", projectID, "session", "session-legacy", "status_changed", "active", "stopped", "legacy event", now, now}},
		{`INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, []any{"event-finding-ignored", projectID, "finding", "finding-legacy", "status_changed", "new", "triaged", "non-lifecycle event", now, now}},
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(context.Background(), statement.sql, statement.args...); err != nil {
			t.Fatalf("seed statement %q error = %v", statement.sql, err)
		}
	}
}

func rawLifecycleStatus(t *testing.T, databasePath string, table string, id string) string {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var status string
	if err := store.db.QueryRow(`SELECT status FROM `+quoteSQLiteIdentifier(table)+` WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("read %s %s status: %v", table, id, err)
	}
	return status
}

func rawLifecycleEventToStatus(t *testing.T, databasePath string, eventID string) string {
	t.Helper()
	return rawLifecycleEventStatusColumn(t, databasePath, eventID, "to_status")
}

func rawLifecycleEventFromStatus(t *testing.T, databasePath string, eventID string) string {
	t.Helper()
	return rawLifecycleEventStatusColumn(t, databasePath, eventID, "from_status")
}

func rawLifecycleEventStatusColumn(t *testing.T, databasePath string, eventID string, column string) string {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var status string
	if err := store.db.QueryRow(`SELECT `+quoteSQLiteIdentifier(column)+` FROM events WHERE id = ?`, eventID).Scan(&status); err != nil {
		t.Fatalf("read event %s %s: %v", eventID, column, err)
	}
	return status
}

func rawLifecycleNormalizationEventCount(t *testing.T, databasePath string, projectID string) int {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM events WHERE project_id = ? AND event_type = ?`, projectID, lifecycleStatusMigrationEventType).Scan(&count); err != nil {
		t.Fatalf("count normalization events: %v", err)
	}
	return count
}
