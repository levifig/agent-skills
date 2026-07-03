package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSearchJournalHitsWithoutDocsIndexing proves the journal-only search path
// (SPEC-056 M1) returns journal hits while never scanning or indexing docs. A
// broken (invalid UTF-8) docs fixture that makes the global Search fail must not
// affect SearchJournal, and no docs_index rows may be written by the journal path.
func TestSearchJournalHitsWithoutDocsIndexing(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)

	if _, err := LogJournal(ctx, root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "decision(search): zephyr journal entry"}); err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}

	// A docs file whose bytes are not valid UTF-8 makes scanDocsIndexCandidates
	// (invoked by the global Search freshness check) return an error.
	writeBrokenDocsFile(t, root.Path(), "docs/broken.md")

	// Sanity: the global Search path really does fail on this fixture, so the
	// journal path avoiding it is meaningful.
	if _, err := store.Search(ctx, root, SearchOptions{Query: "zephyr"}); err == nil {
		t.Fatal("Search() error = nil, want docs-scan failure on broken UTF-8 fixture")
	}

	// The journal-only search must succeed and return the journal hit despite the
	// broken docs fixture, because it never touches the docs index.
	result, err := store.SearchJournal(ctx, root, SearchOptions{Query: "zephyr"})
	if err != nil {
		t.Fatalf("SearchJournal() error = %v, want journal hit without docs scan", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("SearchJournal results = %#v, want exactly one journal hit", result.Results)
	}
	if result.Results[0].Source != "journal_entry" || result.Results[0].ProjectID != projectID {
		t.Fatalf("SearchJournal hit = %#v, want current-project journal hit", result.Results[0])
	}

	// No docs_index rows may exist: the journal path performed no indexing.
	var docsRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs_index`).Scan(&docsRows); err != nil {
		t.Fatalf("count docs_index rows error = %v", err)
	}
	if docsRows != 0 {
		t.Fatalf("docs_index rows = %d, want 0 (journal search must not index docs)", docsRows)
	}
}

// TestSearchJournalScopesToCurrentProjectAndAllProjects verifies the project
// scoping contract of the journal-only search matches the global path: default
// is current-project only; --all crosses projects.
func TestSearchJournalScopesToCurrentProjectAndAllProjects(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(root) error = %v", err)
	}
	if _, err := Initialize(ctx, otherRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(otherRoot) error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	otherProjectID := projectIDForTest(t, store, otherRoot)

	if _, err := LogJournal(ctx, root, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "decision(search): quartz current entry"}); err != nil {
		t.Fatalf("LogJournal(current) error = %v", err)
	}
	if _, err := LogJournal(ctx, otherRoot, PathResolver{StateHome: stateHome}, JournalLogOptions{Entry: "decision(search): quartz other entry"}); err != nil {
		t.Fatalf("LogJournal(other) error = %v", err)
	}

	current, err := SearchJournal(ctx, root, PathResolver{StateHome: stateHome}, SearchOptions{Query: "quartz"})
	if err != nil {
		t.Fatalf("SearchJournal(current) error = %v", err)
	}
	if len(current.Results) != 1 || current.Results[0].ProjectID != projectID {
		t.Fatalf("SearchJournal(current) = %#v, want one current-project hit", current.Results)
	}

	all, err := SearchJournal(ctx, root, PathResolver{StateHome: stateHome}, SearchOptions{Query: "quartz", AllProjects: true})
	if err != nil {
		t.Fatalf("SearchJournal(all) error = %v", err)
	}
	if !all.AllProjects {
		t.Fatal("SearchJournal(all).AllProjects = false, want true")
	}
	if !searchHitsContain(all.Results, "journal_entry", otherProjectID, "") {
		t.Fatalf("SearchJournal(all) = %#v, want cross-project journal hit", all.Results)
	}
}

// TestSearchJournalMissingStateErrors confirms the non-hook journal search path
// still surfaces the uninitialized-state error (M2 leaves interactive behavior
// unchanged; only hook paths degrade gracefully).
func TestSearchJournalMissingStateErrors(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	_, err := SearchJournal(context.Background(), root, PathResolver{StateHome: stateHome}, SearchOptions{Query: "anything"})
	if err == nil {
		t.Fatal("SearchJournal() error = nil, want uninitialized-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("SearchJournal() error = %v, want uninitialized-state sentinel", err)
	}
}

// writeBrokenDocsFile writes a docs/*.md file whose bytes are not valid UTF-8,
// so the docs index scan rejects it. Used to prove the journal search path never
// scans docs.
func writeBrokenDocsFile(t *testing.T, rootPath string, relativePath string) {
	t.Helper()
	path := filepath.Join(rootPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
