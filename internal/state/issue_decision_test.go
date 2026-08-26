package state_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestRecordDecisionQuestionWritesLedgerWithoutIssueRow(t *testing.T) {
	ctx := context.Background()
	root, resolver, stateHome := decisionFixture(t)

	record, err := state.RecordDecisionQuestion(ctx, root, resolver, state.DecisionRecordOptions{
		Title: "Should tokens live in httpOnly cookies?",
	})
	if err != nil {
		t.Fatalf("RecordDecisionQuestion() error = %v", err)
	}
	if record.QuestionID == "" || record.Scope != "project" {
		t.Fatalf("record = %#v", record)
	}

	store := openDecisionStore(t, root, resolver, stateHome)
	defer store.Close()
	if got := countQuery(t, store, `SELECT COUNT(*) FROM issues WHERE kind = ?`, state.IssueKindDecision); got != 0 {
		t.Fatalf("issues decision rows = %d, want 0", got)
	}
	if got := countQuery(t, store, `SELECT COUNT(*) FROM journal_entries WHERE id = ? AND entry_type = ? AND scope = ?`, record.QuestionID, state.JournalEntryTypeQuestion, "project"); got != 1 {
		t.Fatalf("question projection rows = %d, want 1", got)
	}
	if got := countQuery(t, store, `SELECT COUNT(*) FROM facts WHERE id = ? AND kind = ?`, record.QuestionID, state.FactKindJournal); got != 1 {
		t.Fatalf("question fact rows = %d, want 1", got)
	}
}

func TestRecordDecisionQuestionRequiresSharpQuestion(t *testing.T) {
	ctx := context.Background()
	root, resolver, _ := decisionFixture(t)

	if _, err := state.RecordDecisionQuestion(ctx, root, resolver, state.DecisionRecordOptions{
		Title: "Pick a store",
		Body:  "Need a direction.",
	}); err == nil {
		t.Fatal("RecordDecisionQuestion() error = nil, want sharp question validation")
	}
}

func TestReHomeDecisionIssueCreatesQuestionAndRetiresRow(t *testing.T) {
	ctx := context.Background()
	root, resolver, stateHome := decisionFixture(t)

	created, err := state.CreateIssueLegacyDecisionRow(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Should we keep SQLite?",
		Kind:  state.IssueKindDecision,
	})
	if err != nil {
		t.Fatalf("CreateIssueLegacyDecisionRow() error = %v", err)
	}

	result, err := state.ReHomeDecisionIssue(ctx, root, resolver, state.DecisionReHomeOptions{Ref: created.Alias})
	if err != nil {
		t.Fatalf("ReHomeDecisionIssue() error = %v", err)
	}
	if !result.Retired || result.QuestionID == "" || result.ResolutionID != "" {
		t.Fatalf("result = %#v", result)
	}

	store := openDecisionStore(t, root, resolver, stateHome)
	defer store.Close()
	if got := countQuery(t, store, `SELECT COUNT(*) FROM issues WHERE id = ? AND archived_at IS NOT NULL`, created.ID); got != 1 {
		t.Fatalf("archived issue rows = %d, want 1", got)
	}
	if got := countQuery(t, store, `SELECT COUNT(*) FROM journal_entries WHERE id = ? AND entry_type = ?`, result.QuestionID, state.JournalEntryTypeQuestion); got != 1 {
		t.Fatalf("question rows = %d, want 1", got)
	}
}

func TestReHomeDecisionIssueCreatesResolutionWhenDone(t *testing.T) {
	ctx := context.Background()
	root, resolver, _ := decisionFixture(t)

	created, err := state.CreateIssueLegacyDecisionRow(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Should we keep SQLite?",
		Body:  "Yes — single-writer local store stays canonical.",
		Kind:  state.IssueKindDecision,
	})
	if err != nil {
		t.Fatalf("CreateIssueLegacyDecisionRow() error = %v", err)
	}
	if _, err := state.UpdateIssue(ctx, root, resolver, state.IssueUpdateOptions{
		Ref: created.Alias, Status: state.IssueStatusDone, SetStatus: true,
	}); err != nil {
		t.Fatalf("UpdateIssue() error = %v", err)
	}

	result, err := state.ReHomeDecisionIssue(ctx, root, resolver, state.DecisionReHomeOptions{Ref: created.Alias})
	if err != nil {
		t.Fatalf("ReHomeDecisionIssue() error = %v", err)
	}
	if result.ResolutionID == "" {
		t.Fatalf("result = %#v, want resolution fact", result)
	}
}

func TestReHomeDecisionIssueRefusesDeliveryKind(t *testing.T) {
	ctx := context.Background()
	root, resolver, _ := decisionFixture(t)

	created, err := state.CreateIssue(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Delivery row",
		Kind:  state.IssueKindDelivery,
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := state.ReHomeDecisionIssue(ctx, root, resolver, state.DecisionReHomeOptions{Ref: created.Alias}); err == nil {
		t.Fatal("ReHomeDecisionIssue() error = nil, want refusal")
	}
}

func TestCreateIssueDecisionKindDoesNotMintTrackerRow(t *testing.T) {
	ctx := context.Background()
	root, resolver, stateHome := decisionFixture(t)

	created, err := state.CreateIssue(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Should we ship v1 now?",
		Kind:  state.IssueKindDecision,
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.ID == "" || created.Alias != "" {
		t.Fatalf("created = %#v, want journal id without alias", created)
	}

	store := openDecisionStore(t, root, resolver, stateHome)
	defer store.Close()
	if got := countQuery(t, store, `SELECT COUNT(*) FROM issues WHERE id = ?`, created.ID); got != 0 {
		t.Fatalf("issue rows for ledger decision = %d, want 0", got)
	}
	if got := countQuery(t, store, `SELECT COUNT(*) FROM journal_entries WHERE id = ? AND entry_type = ?`, created.ID, state.JournalEntryTypeQuestion); got != 1 {
		t.Fatalf("question rows = %d, want 1", got)
	}
}

func decisionFixture(t *testing.T) (project.Root, state.PathResolver, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	stateHome := t.TempDir()
	t.Setenv("LOAF_DB", filepath.Join(stateHome, "loaf.sqlite"))
	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	ctx := context.Background()
	if _, err := state.Initialize(ctx, root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return root, resolver, stateHome
}

func openDecisionStore(t *testing.T, root project.Root, resolver state.PathResolver, stateHome string) *state.Store {
	t.Helper()
	status, err := state.Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	store, err := state.OpenStoreReadOnly(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly() error = %v", err)
	}
	return store
}

func countQuery(t *testing.T, store *state.Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := store.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	return count
}
