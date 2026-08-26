package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func seedBranchWorkContract(t *testing.T, workingDir, stateHome, branch, status string) string {
	t.Helper()
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	ctx := context.Background()
	resolver := state.PathResolver{StateHome: stateHome}
	initStatus, err := state.Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := state.OpenStore(initStatus.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	authorityRef := state.AuthorityRef{Provider: state.AuthorityProviderBranch, Key: branch}
	if _, err := store.CreateWorkContract(ctx, root, state.WorkContractCreateOptions{
		AuthorityRef: authorityRef,
		Title:        "Branch contract",
		Body:         "Body.\n\nOut of scope: n/a.",
		Criteria:     []state.IssueCriterionInput{{Text: "ship", Tier: state.IssueCriterionTierH}},
	}); err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
	}
	if status != "" && status != state.IssueStatusTriage {
		if _, err := store.UpdateWorkContract(ctx, root, state.WorkContractUpdateOptions{
			AuthorityRef: authorityRef,
			Status:       status,
			SetStatus:    true,
		}); err != nil {
			t.Fatalf("UpdateWorkContract() error = %v", err)
		}
	}
	return authorityRef.String()
}

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

func TestRunnerIssueStartContractRefusesTerminalStatus(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	branch := "issue/done"
	ref := seedBranchWorkContract(t, repo, stateHome, branch, state.IssueStatusDone)

	_, err := runIssue(t, repo, stateHome, "start", ref)
	if err == nil || !strings.Contains(err.Error(), "done") {
		t.Fatalf("start done contract error = %v, want terminal refusal", err)
	}
}

func TestRunnerIssueStartContractRollbackPreservesPreExistingBranch(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	branch := "issue/existing"
	gitCLI(t, repo, "branch", branch)
	ref := seedBranchWorkContract(t, repo, stateHome, branch, "")

	prev := updateWorkContractForStartFn
	updateWorkContractForStartFn = func(ctx context.Context, root project.Root, resolver state.PathResolver, options state.WorkContractUpdateOptions) (state.WorkContract, error) {
		return state.WorkContract{}, errors.New("simulated persistence failure")
	}
	t.Cleanup(func() { updateWorkContractForStartFn = prev })

	_, err := runIssue(t, repo, stateHome, "start", ref)
	if err == nil {
		t.Fatal("start error = nil, want update failure")
	}
	if !gitRefExists(repo, "refs/heads/"+branch) {
		t.Fatalf("branch %s was deleted after failed start rollback", branch)
	}
	worktree := issueWorktreePath(repo, branch)
	if _, statErr := os.Stat(worktree); !os.IsNotExist(statErr) {
		t.Fatalf("worktree %s still exists after rollback: %v", worktree, statErr)
	}
}
