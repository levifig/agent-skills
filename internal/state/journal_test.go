package state

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestLogJournalWritesEntryWithNullableUnresolvedContext(t *testing.T) {
	requireGit(t)
	repo := initGitRepo(t)
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	result, err := store.LogJournal(context.Background(), root, JournalLogOptions{
		Entry:            "decision(sqlite): write to state first",
		ObservedBranch:   ObservedGitBranch(repo),
		ObservedWorktree: repo,
		HarnessSessionID: "harness-123",
	})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	if result.EntryType != "decision" || result.Scope != "sqlite" || result.Message != "write to state first" {
		t.Fatalf("result = %#v, want parsed journal entry", result)
	}
	assertSessionProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.ObservedBranch != "main" || result.ObservedWorktree != repo || result.HarnessSessionID != "harness-123" {
		t.Fatalf("result context = %#v, want observed context", result)
	}

	var entryType, scope, message, branch, worktree, harness sql.NullString
	var sessionID, specID, taskID sql.NullString
	err = store.db.QueryRowContext(context.Background(), `
SELECT entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, session_id, spec_id, task_id
FROM journal_entries
WHERE id = ?
`, result.ID).Scan(&entryType, &scope, &message, &branch, &worktree, &harness, &sessionID, &specID, &taskID)
	if err != nil {
		t.Fatalf("read journal entry error = %v", err)
	}
	if entryType.String != "decision" || scope.String != "sqlite" || message.String != "write to state first" {
		t.Fatalf("journal entry = %q %q %q, want parsed fields", entryType.String, scope.String, message.String)
	}
	if branch.String != "main" || worktree.String != repo || harness.String != "harness-123" {
		t.Fatalf("journal context = %#v %#v %#v, want observed values", branch, worktree, harness)
	}
	if sessionID.Valid || specID.Valid || taskID.Valid {
		t.Fatalf("resolved context = session:%#v spec:%#v task:%#v, want null unresolved context", sessionID, specID, taskID)
	}
}

func TestLogJournalRejectsMalformedEntry(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := LogJournal(context.Background(), root, PathResolver{StateHome: stateHome}, JournalLogOptions{
		Entry: "not a typed entry",
	})
	if err == nil {
		t.Fatal("LogJournal() error = nil, want malformed entry error")
	}
}

func TestLogJournalWaitsForConcurrentWriteTransaction(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	blocker, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(blocker) error = %v", err)
	}
	defer blocker.Close()

	tx, err := blocker.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx(blocker) error = %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(context.Background(), `UPDATE projects SET updated_at = updated_at`); err != nil {
		t.Fatalf("hold write transaction error = %v", err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := LogJournal(context.Background(), root, PathResolver{StateHome: stateHome}, JournalLogOptions{
			Entry: "discover(concurrency): queued while writer holds lock",
		})
		done <- err
	}()
	<-started

	select {
	case err := <-done:
		t.Fatalf("LogJournal returned before writer released lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit(blocker) error = %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("LogJournal() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("LogJournal did not complete after writer released lock")
	}

	reader, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(reader) error = %v", err)
	}
	defer reader.Close()
	var count int
	if err := reader.db.QueryRowContext(context.Background(), `
SELECT COUNT(*) FROM journal_entries
WHERE entry_type = 'discover'
  AND scope = 'concurrency'
  AND message = 'queued while writer holds lock'
`).Scan(&count); err != nil {
		t.Fatalf("count journal entries error = %v", err)
	}
	if count != 1 {
		t.Fatalf("journal entry count = %d, want 1", count)
	}
}

func TestLogJournalLinksHookEntryToHarnessSession(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-linked",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	result, err := LogJournal(context.Background(), root, PathResolver{StateHome: stateHome}, JournalLogOptions{
		Entry:            "task(completed): wire hook logging",
		ObservedBranch:   "main",
		ObservedWorktree: root.Path(),
		HarnessSessionID: "harness-linked",
		LinkSession:      true,
		IfSessionActive:  true,
	})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	if result.Session == nil || result.Session.ID != start.Session.ID {
		t.Fatalf("result session = %#v, want linked session %s", result.Session, start.Session.ID)
	}
	assertSessionProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)

	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if !hasJournalEntry(show.Session.JournalEntries, "task", "completed", "wire hook logging") {
		t.Fatalf("journal entries = %#v, want linked hook entry", show.Session.JournalEntries)
	}
}

func TestLogJournalHookNoopsWhenNoActiveSessionExists(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	result, err := LogJournal(context.Background(), root, PathResolver{StateHome: stateHome}, JournalLogOptions{
		Entry:           "task(completed): nothing to route",
		ObservedBranch:  "main",
		LinkSession:     true,
		IfSessionActive: true,
	})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	if result.ID != "" || result.NoopReason == "" {
		t.Fatalf("result = %#v, want noop without inserted journal", result)
	}
	assertSessionProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
}
