package state_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestRenderOutIssueCreatesBranchContractWithReceipt(t *testing.T) {
	ctx := context.Background()
	root, resolver := renderOutFixture(t)

	created, err := state.CreateIssue(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Trackerless export",
		Body:  "Problem body.\n\nOut of scope: nothing.",
		Kind:  state.IssueKindDelivery,
		Criteria: []state.IssueCriterionInput{{
			Text: "First criterion holds",
			Tier: state.IssueCriterionTierH,
		}},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	result, err := state.RenderOutIssue(ctx, root, resolver, state.IssueRenderOutOptions{
		Ref:    created.Alias,
		Branch: "issue/loaf-test-export",
	})
	if err != nil {
		t.Fatalf("RenderOutIssue() error = %v", err)
	}
	if result.AuthorityRef.String() != "branch:issue/loaf-test-export" {
		t.Fatalf("authority ref = %q", result.AuthorityRef.String())
	}
	if result.Contract.Title != created.Title {
		t.Fatalf("contract title = %q, want %q", result.Contract.Title, created.Title)
	}

	shown, err := state.ShowWorkContract(ctx, root, resolver, result.AuthorityRef.String())
	if err != nil {
		t.Fatalf("ShowWorkContract() error = %v", err)
	}
	if len(shown.Contract.Receipts) == 0 {
		t.Fatal("expected render-out receipt on contract")
	}
	found := false
	for _, receipt := range shown.Contract.Receipts {
		if receipt.Kind == state.RenderOutReceiptKind && receipt.Value == created.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("receipts = %#v, want render-out receipt for %s", shown.Contract.Receipts, created.ID)
	}
}

func TestRenderOutIssueRefusesDecisionKind(t *testing.T) {
	ctx := context.Background()
	root, resolver := renderOutFixture(t)
	created, err := state.CreateIssue(ctx, root, resolver, state.IssueCreateOptions{
		Title: "Decision row",
		Body:  "Should this happen?",
		Kind:  state.IssueKindDecision,
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if _, err := state.RenderOutIssue(ctx, root, resolver, state.IssueRenderOutOptions{Ref: created.Alias}); err == nil {
		t.Fatal("RenderOutIssue() error = nil, want refusal for decision kind")
	}
}

func renderOutFixture(t *testing.T) (project.Root, state.PathResolver) {
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
	return root, resolver
}
