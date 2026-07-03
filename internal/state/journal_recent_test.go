package state

import (
	"context"
	"fmt"
	"testing"
)

// seedJournalEntry inserts a project-scoped journal entry directly, giving tests
// deterministic control over entry_type, branch, and created_at ordering. It
// mirrors the project-scoped insert path in journal.go (session_id NULL) and
// keeps the FTS mirror in lockstep.
func seedJournalEntry(t *testing.T, store *Store, projectID string, entryType string, scope string, message string, branch string, createdAt string) string {
	t.Helper()
	id := stableMigrationID("journal-test", projectID, createdAt, entryType, scope, message)
	_, err := store.db.ExecContext(context.Background(), `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message,
  observed_branch, observed_worktree, harness_session_id,
  session_id, spec_id, task_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, ?, ?)
`, id, projectID, entryType, emptyToNil(scope), message, emptyToNil(branch), createdAt, createdAt)
	if err != nil {
		t.Fatalf("seed journal entry: %v", err)
	}
	if err := insertJournalSearchStore(context.Background(), store, projectID, id, entryType, scope, message); err != nil {
		t.Fatalf("seed journal search row: %v", err)
	}
	return id
}

func insertJournalSearchStore(ctx context.Context, store *Store, projectID string, journalEntryID string, entryType string, scope string, message string) error {
	var rowID int64
	if err := store.db.QueryRowContext(ctx, `SELECT rowid FROM journal_entries WHERE project_id = ? AND id = ?`, projectID, journalEntryID).Scan(&rowID); err != nil {
		return err
	}
	_, err := store.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
VALUES (?, ?, ?, '', ?, ?, ?)
`, rowID, projectID, journalEntryID, entryType, scope, message)
	return err
}

func seedRecentFixture(t *testing.T, store *Store, projectID string) {
	t.Helper()
	// Interleave two branches with a project wrap between the older and newer
	// feature-branch entries so since-last-wrap has a meaningful cutoff.
	seedJournalEntry(t, store, projectID, "decision", "core", "old main decision", "main", "2026-07-01T09:00:00Z")
	seedJournalEntry(t, store, projectID, "discover", "feat", "pre-wrap feature note", "feat/x", "2026-07-01T10:00:00Z")
	seedJournalEntry(t, store, projectID, "wrap", "", "wrapped the feature checkpoint", "feat/x", "2026-07-01T11:00:00Z")
	seedJournalEntry(t, store, projectID, "task", "feat", "post-wrap feature note", "feat/x", "2026-07-01T12:00:00Z")
	seedJournalEntry(t, store, projectID, "note", "main", "latest main note", "main", "2026-07-01T13:00:00Z")
}

func TestRecentJournalReturnsProjectTimelineNewestFirst(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedRecentFixture(t, store, projectID)
	_ = status

	result, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{})
	if err != nil {
		t.Fatalf("RecentJournal() error = %v", err)
	}
	assertSessionProjectContext(t, root, result.ContractVersion, result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Entries) != 5 {
		t.Fatalf("timeline length = %d, want 5 (%#v)", len(result.Entries), result.Entries)
	}
	if result.Entries[0].Message != "latest main note" {
		t.Fatalf("newest entry = %q, want latest main note", result.Entries[0].Message)
	}
	if result.Entries[len(result.Entries)-1].Message != "old main decision" {
		t.Fatalf("oldest entry = %q, want old main decision", result.Entries[len(result.Entries)-1].Message)
	}
}

func TestRecentJournalBranchFilterScopesToObservedBranch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedRecentFixture(t, store, projectID)

	result, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{Branch: "feat/x"})
	if err != nil {
		t.Fatalf("RecentJournal(branch) error = %v", err)
	}
	if result.Branch != "feat/x" {
		t.Fatalf("result.Branch = %q, want feat/x", result.Branch)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("branch timeline length = %d, want 3 (%#v)", len(result.Entries), result.Entries)
	}
	for _, entry := range result.Entries {
		if entry.ObservedBranch != "feat/x" {
			t.Fatalf("entry branch = %q, want feat/x", entry.ObservedBranch)
		}
	}
}

func TestRecentJournalSinceLastWrapTrimsToPostWrapEntries(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedRecentFixture(t, store, projectID)

	// Project-scoped since-last-wrap: only entries after the 11:00 wrap survive.
	result, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{SinceLastWrap: true})
	if err != nil {
		t.Fatalf("RecentJournal(since-last-wrap) error = %v", err)
	}
	if !result.SinceLastWrap {
		t.Fatal("result.SinceLastWrap = false, want true")
	}
	if len(result.Entries) != 2 {
		t.Fatalf("since-last-wrap length = %d, want 2 (%#v)", len(result.Entries), result.Entries)
	}
	for _, entry := range result.Entries {
		if entry.Message == "wrapped the feature checkpoint" {
			t.Fatalf("wrap entry itself leaked into since-last-wrap window: %#v", entry)
		}
		if entry.CreatedAt <= "2026-07-01T11:00:00Z" {
			t.Fatalf("entry %q at %s is not strictly after the wrap", entry.Message, entry.CreatedAt)
		}
	}
}

func TestRecentJournalSinceLastWrapBranchScopedCutoff(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedRecentFixture(t, store, projectID)

	// Branch feat/x wrap is at 11:00; only the 12:00 post-wrap feature note is on
	// that branch after it.
	result, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{Branch: "feat/x", SinceLastWrap: true})
	if err != nil {
		t.Fatalf("RecentJournal(branch, since-last-wrap) error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("branch since-last-wrap length = %d, want 1 (%#v)", len(result.Entries), result.Entries)
	}
	if result.Entries[0].Message != "post-wrap feature note" {
		t.Fatalf("entry = %q, want post-wrap feature note", result.Entries[0].Message)
	}
}

func TestRecentJournalSinceLastWrapWithoutWrapReturnsFullWindow(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	seedJournalEntry(t, store, projectID, "decision", "core", "only entry", "main", "2026-07-01T09:00:00Z")

	result, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{SinceLastWrap: true})
	if err != nil {
		t.Fatalf("RecentJournal(since-last-wrap, no wrap) error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("no-wrap timeline length = %d, want 1 (%#v)", len(result.Entries), result.Entries)
	}
}

func TestRecentJournalRespectsLimit(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	for i := 0; i < 6; i++ {
		seedJournalEntry(t, store, projectID, "note", "", fmt.Sprintf("entry %d", i), "main", fmt.Sprintf("2026-07-01T09:0%d:00Z", i))
	}

	result, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{Limit: 3})
	if err != nil {
		t.Fatalf("RecentJournal(limit) error = %v", err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("limited timeline length = %d, want 3", len(result.Entries))
	}
	if result.Entries[0].Message != "entry 5" {
		t.Fatalf("newest limited entry = %q, want entry 5", result.Entries[0].Message)
	}
}

func TestShowJournalReturnsEntryByID(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	id := seedJournalEntry(t, store, projectID, "decision", "sqlite", "durable body", "main", "2026-07-01T09:00:00Z")

	show, err := store.ShowJournal(context.Background(), root, id)
	if err != nil {
		t.Fatalf("ShowJournal() error = %v", err)
	}
	if show.Entry.ID != id || show.Entry.EntryType != "decision" || show.Entry.Scope != "sqlite" || show.Entry.Message != "durable body" {
		t.Fatalf("entry = %#v, want seeded entry", show.Entry)
	}
	assertSessionProjectContext(t, root, show.ContractVersion, show.DatabaseScope, show.DatabasePath, show.ProjectID, show.ProjectName, show.ProjectCurrentPath)
}

func TestShowJournalUnknownEntryErrors(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	if _, err := store.ShowJournal(context.Background(), root, "does-not-exist"); err == nil {
		t.Fatal("ShowJournal(unknown) error = nil, want not-found error")
	}
}

// TestUntaggedManualEntryLandsProjectScopedAndIsDiscoverable proves the
// journal-first invariant: a manual entry with no harness-session tag and no
// branch is written project-scoped and surfaces in both the flat timeline and
// FTS search. This is the log path `loaf journal log` drives (LinkSession off).
func TestUntaggedManualEntryLandsProjectScopedAndIsDiscoverable(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	logged, err := LogJournal(context.Background(), root, resolver, JournalLogOptions{
		Entry: "note(reminder): revisit the FTS rebuild ordering",
	})
	if err != nil {
		t.Fatalf("LogJournal(untagged) error = %v", err)
	}
	if logged.ID == "" {
		t.Fatalf("logged entry has no id: %#v", logged)
	}
	if logged.HarnessSessionID != "" || logged.ObservedBranch != "" {
		t.Fatalf("untagged entry carried context: %#v", logged)
	}

	store := openTestStore(t, root, stateHome)
	defer store.Close()

	// Appears in the flat project timeline.
	recent, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{})
	if err != nil {
		t.Fatalf("RecentJournal() error = %v", err)
	}
	found := false
	for _, entry := range recent.Entries {
		if entry.ID == logged.ID {
			found = true
			if entry.ObservedBranch != "" || entry.HarnessSessionID != "" {
				t.Fatalf("timeline entry carried context: %#v", entry)
			}
		}
	}
	if !found {
		t.Fatalf("untagged entry %s missing from timeline: %#v", logged.ID, recent.Entries)
	}

	// Appears in FTS search.
	search, err := store.Search(context.Background(), root, SearchOptions{Query: "rebuild ordering"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	hit := false
	for _, result := range search.Results {
		if result.JournalEntryID == logged.ID {
			hit = true
		}
	}
	if !hit {
		t.Fatalf("untagged entry %s missing from search results: %#v", logged.ID, search.Results)
	}
}
