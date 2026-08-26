package cli

import (
	"context"
	"encoding/json"
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

func seedReadyBranchDecisionContract(t *testing.T, workingDir, stateHome, branch string) string {
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
		Kind:         state.IssueKindDecision,
		Title:        "Should we ship readiness publication?",
		Body:         "Sharp decision body?",
	}); err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
	}
	return authorityRef.String()
}

func seedReadyLinearDecisionContract(t *testing.T, workingDir, stateHome, key string) string {
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
	authorityRef := state.AuthorityRef{Provider: state.AuthorityProviderLinear, Key: key}
	if _, err := store.CreateWorkContract(ctx, root, state.WorkContractCreateOptions{
		AuthorityRef: authorityRef,
		Kind:         state.IssueKindDecision,
		Title:        "Should linear publish now?",
		Body:         "Deferred publication decision?",
	}); err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
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

func TestRunnerIssueContractCheckPublishesOnTrackerBackedProject(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.SetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueIdentityOptions{Authority: state.IssueAuthorityGitHub}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	ref := seedReadyBranchDecisionContract(t, workingDir, stateHome, "issue/ready-pub")

	fake := &recordingReadinessPublisher{}
	previous := defaultReadinessPublisher
	defaultReadinessPublisher = fake
	t.Cleanup(func() { defaultReadinessPublisher = previous })

	out, err := runIssue(t, workingDir, stateHome, "check", ref, "--json")
	if err != nil {
		t.Fatalf("issue check error = %v\n%s", err, out)
	}
	if len(fake.publications) != 1 || fake.publications[0].Label != readinessLabelAgent {
		t.Fatalf("publications = %#v, want one ready-for-agent", fake.publications)
	}
	if fake.publications[0].IssueRef != ref {
		t.Fatalf("IssueRef = %q, want %q", fake.publications[0].IssueRef, ref)
	}

	fake.publications = nil
	humanOut, err := runIssue(t, workingDir, stateHome, "check", ref, "--human", "needs a human call", "--json")
	if err != nil {
		t.Fatalf("issue check --human error = %v\n%s", err, humanOut)
	}
	if len(fake.publications) != 1 || fake.publications[0].Label != readinessLabelHuman || fake.publications[0].Reason != "needs a human call" {
		t.Fatalf("human publications = %#v", fake.publications)
	}

	var result issueCheckResult
	if err := json.Unmarshal([]byte(humanOut), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if result.Publication == nil || result.Publication.Label != readinessLabelHuman || result.Publication.Reason != "needs a human call" {
		t.Fatalf("json publication = %#v", result.Publication)
	}
}

func TestRunnerIssueContractCheckDefersLinearPublicationButAcceptsHuman(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.SetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueIdentityOptions{Authority: state.IssueAuthorityLinear}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	ref := seedReadyLinearDecisionContract(t, workingDir, stateHome, "ENG-82")

	fake := &recordingReadinessPublisher{}
	previous := defaultReadinessPublisher
	defaultReadinessPublisher = fake
	t.Cleanup(func() { defaultReadinessPublisher = previous })

	out, err := runIssue(t, workingDir, stateHome, "check", ref, "--human", "still accepted", "--json")
	if err != nil {
		t.Fatalf("issue check linear: --human error = %v\n%s", err, out)
	}
	if len(fake.publications) != 0 {
		t.Fatalf("publications = %#v, want none for deferred linear: publication", fake.publications)
	}
	var result issueCheckResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !result.Ready {
		t.Fatalf("result = %#v, want ready", result)
	}
	if result.Publication != nil {
		t.Fatalf("publication = %#v, want omitted for deferred linear: path", result.Publication)
	}
}

func TestRunnerIssueContractVerifyEmitsAdvisoryWarnings(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
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
	authorityRef := state.AuthorityRef{Provider: state.AuthorityProviderBranch, Key: "issue/verify-advisory"}
	if _, err := store.CreateWorkContract(ctx, root, state.WorkContractCreateOptions{
		AuthorityRef: authorityRef,
		Title:        "Verify advisory contract",
		Body:         "Body.\n\nOut of scope: n/a.",
		Criteria: []state.IssueCriterionInput{{
			Text:    "Exit ok with prose expect",
			Command: "true",
			Expect:  "exit 0 and the output reads well",
			Tier:    state.IssueCriterionTierV,
		}},
	}); err != nil {
		t.Fatalf("CreateWorkContract() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "verify", authorityRef.String())
	if err != nil {
		t.Fatalf("issue verify error = %v\n%s", err, out)
	}
	if !strings.Contains(out, `unenforceable Expect clause "the output reads well"`) {
		t.Fatalf("verify output = %q, want advisory warning for unenforceable Expect clause", out)
	}
	if !strings.Contains(out, "warn") {
		t.Fatalf("verify output = %q, want yellow warn label", out)
	}
}
