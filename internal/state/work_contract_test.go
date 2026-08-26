package state

import (
	"context"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func workContractFixture(t *testing.T) (context.Context, project.Root, *Store, AuthorityRef) {
	t.Helper()
	root, store := issueTestFixture(t)
	ctx := context.Background()
	contract, err := store.CreateWorkContract(ctx, root, WorkContractCreateOptions{
		AuthorityRef: AuthorityRef{Provider: AuthorityProviderLinear, Key: "ENG-82"},
		Title:        "Contract machinery keys to refs",
		Body:         "Problem body.\n\nOut of scope: migration.",
		Criteria: []IssueCriterionInput{{
			Text: "DoD keys to provider-qualified authority refs",
			Tier: IssueCriterionTierH,
		}},
	})
	if err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
	}
	if contract.AuthorityRef.String() != "linear:ENG-82" {
		t.Fatalf("contract ref = %q, want linear:ENG-82", contract.AuthorityRef.String())
	}
	return ctx, root, store, contract.AuthorityRef
}

func TestWorkContractCreateShowVerifyReadiness(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)

	shown, err := store.ShowWorkContract(ctx, root, ref.String())
	if err != nil {
		t.Fatalf("ShowWorkContract() error = %v", err)
	}
	if len(shown.Contract.Criteria) != 1 {
		t.Fatalf("criteria len = %d, want 1", len(shown.Contract.Criteria))
	}

	readiness, err := store.CheckWorkContractReadiness(ctx, root, ref.String())
	if err != nil {
		t.Fatalf("CheckWorkContractReadiness() error = %v", err)
	}
	if !readiness.Shaped || !readiness.Ready {
		t.Fatalf("readiness = shaped:%v ready:%v failures:%#v", readiness.Shaped, readiness.Ready, readiness.Failures)
	}

	markdown := RenderWorkContractMarkdown(shown)
	if markdown == "" || !containsAll(markdown, "Contract machinery", "Definition of Done") {
		t.Fatalf("RenderWorkContractMarkdown() = %q, want title and DoD", markdown)
	}
}

func TestWorkContractWorkspaceBinding(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)

	updated, err := store.UpdateWorkContract(ctx, root, WorkContractUpdateOptions{
		AuthorityRef:    ref,
		StartedBranch:   "issue/loaf-68",
		StartedWorktree: "/tmp/issue-loaf-68",
		SetStarted:      true,
	})
	if err != nil {
		t.Fatalf("UpdateWorkContract() error = %v", err)
	}
	if updated.StartedBranch != "issue/loaf-68" || updated.StartedWorktree != "/tmp/issue-loaf-68" {
		t.Fatalf("workspace = %#v", updated)
	}
}

func TestWorkContractReceiptAndMapping(t *testing.T) {
	ctx, root, store, ref := workContractFixture(t)
	resolver := PathResolver{StateHome: t.TempDir()}

	if err := store.UpsertWorkContractMapping(ctx, root, ref.String(), "linear_url", "https://linear.app/issue/ENG-82"); err != nil {
		t.Fatalf("UpsertWorkContractMapping() error = %v", err)
	}
	if err := store.UpsertWorkContractReceipt(ctx, root, ref.String(), "export", "rendered-to-linear"); err != nil {
		t.Fatalf("UpsertWorkContractReceipt() error = %v", err)
	}

	shown, err := store.ShowWorkContract(ctx, root, ref.String())
	if err != nil {
		t.Fatalf("ShowWorkContract() error = %v", err)
	}
	if len(shown.Contract.Mappings) != 1 || len(shown.Contract.Receipts) != 1 {
		t.Fatalf("mappings=%#v receipts=%#v", shown.Contract.Mappings, shown.Contract.Receipts)
	}
	_ = resolver
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
