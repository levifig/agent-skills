package state

import (
	"context"
	"database/sql"
	"testing"
)

func TestStartSessionCreatesSQLiteSessionWithLinkedStartJournal(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	result, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "feature/session-start",
		HarnessSessionID: "harness-123456789",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	if result.Action != SessionStartActionCreated || result.Session.Alias == "" || result.HarnessSessionID != "harness-123456789" {
		t.Fatalf("result = %#v, want created session with harness id", result)
	}
	if len(result.JournalEntryIDs) != 1 {
		t.Fatalf("JournalEntryIDs = %#v, want one start entry", result.JournalEntryIDs)
	}

	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, result.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	session := show.Session
	if session.Branch != "feature/session-start" || session.Status != "active" || session.HarnessSessionID != "harness-123456789" {
		t.Fatalf("session = %#v, want active native session metadata", session)
	}
	if !hasJournalEntry(session.JournalEntries, "session", "start", "=== SESSION STARTED === (session harness-)") {
		t.Fatalf("journal entries = %#v, want linked session(start)", session.JournalEntries)
	}
}

func TestStartSessionResumesSameHarnessSessionWithoutCreatingDuplicate(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-same",
	})
	if err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	second, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "feature/new-branch",
		HarnessSessionID: "harness-same",
	})
	if err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}
	if second.Action != SessionStartActionAlreadyActive || second.Session.ID != first.Session.ID {
		t.Fatalf("second = %#v, want same already-active session", second)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()
	if got := countRows(t, store, `SELECT COUNT(*) FROM sessions WHERE project_id = ?`, ProjectID(root)); got != 1 {
		t.Fatalf("session rows = %d, want 1", got)
	}
	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, first.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if show.Session.Branch != "feature/new-branch" {
		t.Fatalf("branch = %q, want updated branch", show.Session.Branch)
	}
}

func TestStartSessionRotatesDifferentHarnessSessionOnSameBranch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-old",
	})
	if err != nil {
		t.Fatalf("first StartSession() error = %v", err)
	}
	second, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-new",
	})
	if err != nil {
		t.Fatalf("second StartSession() error = %v", err)
	}
	if second.Action != SessionStartActionRotated || second.Session.ID == first.Session.ID || second.PreviousSession == nil {
		t.Fatalf("second = %#v, want rotated new session with previous session", second)
	}

	oldShow, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, first.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession(old) error = %v", err)
	}
	if oldShow.Session.Status != "stopped" {
		t.Fatalf("old status = %q, want stopped", oldShow.Session.Status)
	}
	if !hasJournalEntry(oldShow.Session.JournalEntries, "session", "end", "closed by new conversation") ||
		!hasJournalEntry(oldShow.Session.JournalEntries, "session", "stop", "=== SESSION STOPPED ===") {
		t.Fatalf("old journal entries = %#v, want end and stop entries", oldShow.Session.JournalEntries)
	}
}

func TestEndSessionStopsTargetHarnessSessionOnly(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	target, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "feature/target",
		HarnessSessionID: "harness-target",
	})
	if err != nil {
		t.Fatalf("target StartSession() error = %v", err)
	}
	other, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-other",
	})
	if err != nil {
		t.Fatalf("other StartSession() error = %v", err)
	}

	result, err := EndSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionEndOptions{
		Branch:           "main",
		HarnessSessionID: "harness-target",
	})
	if err != nil {
		t.Fatalf("EndSession() error = %v", err)
	}
	if result.Action != SessionEndActionStopped || result.Session.ID != target.Session.ID || len(result.JournalEntryIDs) != 2 {
		t.Fatalf("result = %#v, want stopped target session", result)
	}

	targetShow, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, target.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession(target) error = %v", err)
	}
	if targetShow.Session.Status != "stopped" {
		t.Fatalf("target status = %q, want stopped", targetShow.Session.Status)
	}
	if !hasJournalEntry(targetShow.Session.JournalEntries, "session", "end", "session ended") ||
		!hasJournalEntry(targetShow.Session.JournalEntries, "session", "stop", "=== SESSION STOPPED ===") {
		t.Fatalf("target journal entries = %#v, want end and stop entries", targetShow.Session.JournalEntries)
	}
	otherShow, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, other.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession(other) error = %v", err)
	}
	if otherShow.Session.Status != "active" {
		t.Fatalf("other status = %q, want active", otherShow.Session.Status)
	}
}

func TestEndSessionIfActiveNoopsWhenNoSessionExists(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	result, err := EndSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionEndOptions{
		Branch:   "main",
		IfActive: true,
	})
	if err != nil {
		t.Fatalf("EndSession(if-active) error = %v", err)
	}
	if result.Action != SessionEndActionNoop || result.NoopReason == "" {
		t.Fatalf("result = %#v, want noop with reason", result)
	}
}

func TestEndSessionWrapMarksDoneWithoutStopEntry(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-wrap",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	result, err := EndSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionEndOptions{
		HarnessSessionID: "harness-wrap",
		Wrap:             true,
	})
	if err != nil {
		t.Fatalf("EndSession(wrap) error = %v", err)
	}
	if result.Action != SessionEndActionDone {
		t.Fatalf("Action = %q, want %q", result.Action, SessionEndActionDone)
	}
	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if show.Session.Status != "done" {
		t.Fatalf("status = %q, want done", show.Session.Status)
	}
	if !hasJournalEntry(show.Session.JournalEntries, "session", "wrap", "session ended") {
		t.Fatalf("journal entries = %#v, want wrap entry", show.Session.JournalEntries)
	}
	if hasJournalEntry(show.Session.JournalEntries, "session", "stop", "=== SESSION STOPPED ===") {
		t.Fatalf("journal entries = %#v, did not expect stop entry for wrap", show.Session.JournalEntries)
	}
}

func TestEndSessionClearKeepsSessionActive(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	start, err := StartSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionStartOptions{
		Branch:           "main",
		HarnessSessionID: "harness-clear",
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	result, err := EndSession(context.Background(), root, PathResolver{StateHome: stateHome}, SessionEndOptions{
		HarnessSessionID: "harness-clear",
		Clear:            true,
	})
	if err != nil {
		t.Fatalf("EndSession(clear) error = %v", err)
	}
	if result.Action != SessionEndActionCleared {
		t.Fatalf("Action = %q, want %q", result.Action, SessionEndActionCleared)
	}
	show, err := ShowSession(context.Background(), root, PathResolver{StateHome: stateHome}, start.Session.Alias)
	if err != nil {
		t.Fatalf("ShowSession() error = %v", err)
	}
	if show.Session.Status != "active" {
		t.Fatalf("status = %q, want active", show.Session.Status)
	}
	if !hasJournalEntry(show.Session.JournalEntries, "session", "clear", "=== CONTEXT CLEARED ===") {
		t.Fatalf("journal entries = %#v, want clear entry", show.Session.JournalEntries)
	}
}

func countRows(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil && err != sql.ErrNoRows {
		t.Fatalf("count query error = %v", err)
	}
	return count
}
