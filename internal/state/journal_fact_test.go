package state

import (
	"context"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func mustBackfillJournalFactForTest(t *testing.T, store *Store, projectID, journalEntryID string) {
	t.Helper()
	if err := backfillJournalFactForTest(context.Background(), store, projectID, journalEntryID); err != nil {
		t.Fatalf("backfill journal fact %s: %v", journalEntryID, err)
	}
}

func TestLogJournalWritesAndReadsAsFacts(t *testing.T) {
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
		Entry:            "decision(facts): journal on facts",
		ObservedBranch:   ObservedGitBranch(repo),
		ObservedWorktree: repo,
		HarnessSessionID: "harness-facts",
	})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	var factCount int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM facts WHERE id = ? AND kind = ?`, result.ID, FactKindJournal).Scan(&factCount); err != nil {
		t.Fatalf("count fact row: %v", err)
	}
	if factCount != 1 {
		t.Fatalf("fact count = %d, want 1", factCount)
	}
	parity, err := InspectJournalFactParity(context.Background(), store)
	if err != nil {
		t.Fatalf("InspectJournalFactParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("parity = %#v, want ready", parity)
	}
	recent, err := store.RecentJournal(context.Background(), root, JournalRecentOptions{Limit: 5})
	if err != nil {
		t.Fatalf("RecentJournal() error = %v", err)
	}
	if len(recent.Entries) == 0 || recent.Entries[0].ID != result.ID {
		t.Fatalf("recent entries = %#v, want logged entry", recent.Entries)
	}
}

func TestInspectJournalFactParityDetectsProjectionDivergence(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	projectID, err := store.projectID(ctx, root)
	if err != nil {
		t.Fatalf("projectID() error = %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	entryID := "journal-test-projection-only"
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at)
VALUES (?, ?, 'discover', 'facts', 'projection only', ?, ?)
`, entryID, projectID, now, now); err != nil {
		t.Fatalf("insert projection-only journal row: %v", err)
	}
	parity, err := InspectJournalFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalFactParity() error = %v", err)
	}
	if parity.Ready || parity.Extra == 0 {
		t.Fatalf("parity = %#v, want extra divergence", parity)
	}
	if err := backfillJournalFactForTest(ctx, store, projectID, entryID); err != nil {
		t.Fatalf("backfillJournalFactForTest() error = %v", err)
	}
	parity, err = InspectJournalFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalFactParity() after backfill error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("parity after backfill = %#v, want ready", parity)
	}
}

func TestMigrationWrapsExistingJournalCorpus(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	logged, err := store.LogJournal(ctx, root, JournalLogOptions{Entry: "discover(wrap): wrapped corpus"})
	if err != nil {
		t.Fatalf("LogJournal() error = %v", err)
	}
	var envID string
	if err := store.db.QueryRowContext(ctx, `SELECT env_id FROM facts WHERE id = ?`, logged.ID).Scan(&envID); err != nil {
		t.Fatalf("read wrapped env_id: %v", err)
	}
	if envID != localFactEnvID() && envID != legacyFactEnvID {
		t.Fatalf("env_id = %q, want local or legacy host", envID)
	}
	parity, err := InspectJournalFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalFactParity() error = %v", err)
	}
	if !parity.Ready || parity.FactRows == 0 {
		t.Fatalf("parity = %#v, want ready wrapped corpus", parity)
	}
}

func TestDeferJournalMaintainsJournalFactParity(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.DeferJournal(ctx, root, JournalDeferOptions{
		Intent: "parity intent", Why: "parity why", Boundary: "parity boundary", Trigger: "parity trigger",
		OperationID: "parity-op",
	}); err != nil {
		t.Fatalf("DeferJournal() error = %v", err)
	}
	parity, err := InspectJournalFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalFactParity() error = %v", err)
	}
	if !parity.Ready || parity.FactRows == 0 {
		t.Fatalf("parity = %#v, want ready defer decision fact", parity)
	}
}

func TestImportMarkdownMaintainsJournalFactParity(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	writeAgentsFile(t, root.Path(), "sessions/20260710-journal.md", `---
harness_session_id: hsid-fact-parity
---
[2026-07-10 10:00] decision(import): markdown fact parity
`)
	result, err := ApplyMarkdownMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyMarkdownMigration() error = %v", err)
	}
	store, err := OpenStore(result.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	parity, err := InspectJournalFactParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalFactParity() error = %v", err)
	}
	if !parity.Ready || parity.FactRows != 1 || parity.ProjectionRows != 1 {
		t.Fatalf("parity = %#v, want one ready imported journal fact", parity)
	}
}
