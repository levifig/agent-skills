package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestRunnerIssueCheckRefusesUnsupportedAuthorityProvider(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	out, err := runIssue(t, workingDir, stateHome, "check", "github:owner-1")
	if err == nil {
		t.Fatalf("issue check error = nil, want unsupported provider failure\n%s", out)
	}
	if !strings.Contains(err.Error(), "not a v1 authority provider") {
		t.Fatalf("error = %v, want unsupported provider message", err)
	}
}

func TestRunnerIssueRenderAuthorityRefContract(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	ctx := context.Background()
	resolver := state.PathResolver{StateHome: stateHome}
	status, err := state.Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := state.OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.CreateWorkContract(ctx, root, state.WorkContractCreateOptions{
		AuthorityRef: state.AuthorityRef{Provider: state.AuthorityProviderPR, Key: "99"},
		Title:        "PR contract",
		Body:         "Body.\n\nOut of scope: n/a.",
		Criteria:     []state.IssueCriterionInput{{Text: "land the PR", Tier: state.IssueCriterionTierH}},
	}); err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "render", "pr:99")
	if err != nil {
		t.Fatalf("issue render error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "PR contract") || !strings.Contains(out, "Definition of Done") {
		t.Fatalf("render = %q, want title and DoD", out)
	}
}
