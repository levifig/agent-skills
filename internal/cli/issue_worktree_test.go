package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func issueGitFixture(t *testing.T) (string, string) {
	t.Helper()
	parent := realpath(t, t.TempDir())
	// Digit-leading dirname cannot form a valid issue prefix, so mint stays LOAF.
	repo := filepath.Join(parent, "001")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("Mkdir(repo) error = %v", err)
	}
	gitCLI(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(README) error = %v", err)
	}
	gitCLI(t, repo, "add", "README.md")
	gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "initial")
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	return repo, stateHome
}

func decodeIssueStart(t *testing.T, data string) issueStartResult {
	t.Helper()
	var result issueStartResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("json.Unmarshal(start %q) error = %v", data, err)
	}
	return result
}

func decodeIssueStop(t *testing.T, data string) issueStopResult {
	t.Helper()
	var result issueStopResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("json.Unmarshal(stop %q) error = %v", data, err)
	}
	return result
}

func gitOutputCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRunnerIssueVerifyRunsInInvokingWorktree(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Verify worktree"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	marker := "worktree-only-marker.txt"
	command := "test -f " + marker
	if _, err := runIssue(t, repo, stateHome, "dod", "add", "LOAF-1", "marker exists in invoking worktree",
		"--command", command, "--expect", "exit 0"); err != nil {
		t.Fatalf("dod add error = %v", err)
	}

	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if err := os.WriteFile(filepath.Join(started.Worktree, marker), []byte("present\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(marker) error = %v", err)
	}

	out, err := runIssue(t, started.Worktree, stateHome, "verify", "LOAF-1")
	if err != nil {
		t.Fatalf("verify from worktree error = %v\n%s", err, out)
	}
	if !strings.Contains(out, command) {
		t.Fatalf("verify output = %q, want command %q", out, command)
	}

	failOut, err := runIssue(t, repo, stateHome, "verify", "LOAF-1")
	if err == nil {
		t.Fatalf("verify from main checkout should fail when marker exists only in worktree\n%s", failOut)
	}
}

func TestRunnerIssueStartChildJoinsRootWorkspace(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child A", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child A error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child B", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child B error = %v", err)
	}

	parentOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start parent error = %v", err)
	}
	parent := decodeIssueStart(t, parentOut)
	if parent.Branch != "issue/loaf-1" || parent.Base != "main" || parent.Joined || parent.Requested != "LOAF-1" {
		t.Fatalf("parent start = %#v, want issue/loaf-1 from main", parent)
	}

	childAOut, err := runIssue(t, repo, stateHome, "start", "LOAF-2", "--json")
	if err != nil {
		t.Fatalf("issue start child A error = %v", err)
	}
	childA := decodeIssueStart(t, childAOut)
	if !childA.Joined || childA.Requested != "LOAF-2" || childA.Issue.Alias != "LOAF-1" {
		t.Fatalf("child A start = %#v, want join of LOAF-1", childA)
	}
	if childA.Branch != parent.Branch || childA.Worktree != parent.Worktree {
		t.Fatalf("child A workspace = %s %s, want parent %s %s", childA.Branch, childA.Worktree, parent.Branch, parent.Worktree)
	}
	if gitRefExists(repo, "refs/heads/issue/loaf-2") {
		t.Fatal("child A minted issue/loaf-2; descendants must join the root")
	}

	childBOut, err := runIssue(t, repo, stateHome, "start", "LOAF-3", "--json")
	if err != nil {
		t.Fatalf("issue start child B error = %v", err)
	}
	childB := decodeIssueStart(t, childBOut)
	if !childB.Joined || childB.Worktree != parent.Worktree || childB.Branch != "issue/loaf-1" {
		t.Fatalf("child B start = %#v, want same root workspace", childB)
	}
	if gitRefExists(repo, "refs/heads/issue/loaf-3") {
		t.Fatal("child B minted issue/loaf-3; descendants must join the root")
	}

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	for _, alias := range []string{"LOAF-2", "LOAF-3"} {
		issue, err := state.GetIssue(context.Background(), root, resolver, alias)
		if err != nil {
			t.Fatalf("GetIssue(%s) error = %v", alias, err)
		}
		if issue.Status != state.IssueStatusActive {
			t.Fatalf("%s status = %q, want active", alias, issue.Status)
		}
		if issue.StartedBranch != "" || issue.StartedWorktree != "" {
			t.Fatalf("%s started fields = %#v, want empty on descendant", alias, issue)
		}
	}
	listed, err := runIssue(t, repo, stateHome, "list", "--started")
	if err != nil {
		t.Fatalf("issue list --started error = %v", err)
	}
	if !strings.Contains(listed, "LOAF-1") || strings.Contains(listed, "LOAF-2") || strings.Contains(listed, "LOAF-3") {
		t.Fatalf("started list = %s, want only the root", listed)
	}
}

func TestRunnerIssueStartChildCreatesRootWorkspace(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}

	out, err := runIssue(t, repo, stateHome, "start", "LOAF-2", "--json")
	if err != nil {
		t.Fatalf("issue start child error = %v", err)
	}
	started := decodeIssueStart(t, out)
	if started.Joined || started.Requested != "LOAF-2" || started.Issue.Alias != "LOAF-1" {
		t.Fatalf("start child = %#v, want created root workspace", started)
	}
	if started.Branch != "issue/loaf-1" || started.Base != "main" {
		t.Fatalf("workspace = branch %q base %q, want issue/loaf-1 from main", started.Branch, started.Base)
	}
	if _, err := os.Stat(started.Worktree); err != nil {
		t.Fatalf("root worktree %s: %v", started.Worktree, err)
	}
	if gitRefExists(repo, "refs/heads/issue/loaf-2") {
		t.Fatal("start child minted issue/loaf-2")
	}

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	parent, err := state.GetIssue(context.Background(), root, resolver, "LOAF-1")
	if err != nil {
		t.Fatalf("GetIssue(parent) error = %v", err)
	}
	if parent.Status != state.IssueStatusActive || parent.StartedBranch != "issue/loaf-1" || parent.StartedWorktree != started.Worktree {
		t.Fatalf("parent after child start = %#v", parent)
	}
	child, err := state.GetIssue(context.Background(), root, resolver, "LOAF-2")
	if err != nil {
		t.Fatalf("GetIssue(child) error = %v", err)
	}
	if child.Status != state.IssueStatusActive || child.StartedBranch != "" || child.StartedWorktree != "" {
		t.Fatalf("child after start = %#v, want active without workspace", child)
	}

	human, err := runIssue(t, repo, stateHome, "start", "LOAF-2")
	if err != nil {
		t.Fatalf("idempotent join error = %v", err)
	}
	if !strings.Contains(human, "joined issue LOAF-1") || !strings.Contains(human, "activated: LOAF-2") {
		t.Fatalf("idempotent join output = %s", human)
	}
}

func TestRunnerIssueStopChildWithoutWorkspace(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-1"); err != nil {
		t.Fatalf("issue start parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-2"); err != nil {
		t.Fatalf("issue start child error = %v", err)
	}

	_, err := runIssue(t, repo, stateHome, "stop", "LOAF-2")
	if err == nil || !strings.Contains(err.Error(), "does not own a worktree") || !strings.Contains(err.Error(), "stop LOAF-1") {
		t.Fatalf("stop child error = %v, want root-owned refusal", err)
	}

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	parent, err := state.GetIssue(context.Background(), root, state.PathResolver{StateHome: stateHome}, "LOAF-1")
	if err != nil {
		t.Fatalf("GetIssue(parent) error = %v", err)
	}
	if parent.StartedBranch == "" || parent.StartedWorktree == "" {
		t.Fatalf("stop child cleared the root workspace: %#v", parent)
	}
	if _, err := os.Stat(parent.StartedWorktree); err != nil {
		t.Fatalf("root worktree missing after refused child stop: %v", err)
	}
}

func TestRunnerIssueStartChildRefusesMissingRootWorktree(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}
	parentOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start parent error = %v", err)
	}
	parent := decodeIssueStart(t, parentOut)
	if err := os.RemoveAll(parent.Worktree); err != nil {
		t.Fatalf("RemoveAll(%s) error = %v", parent.Worktree, err)
	}

	_, err = runIssue(t, repo, stateHome, "start", "LOAF-2")
	if err == nil || !strings.Contains(err.Error(), "missing") || !strings.Contains(err.Error(), "stop LOAF-1") {
		t.Fatalf("start child error = %v, want missing-root-worktree refusal", err)
	}

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	child, err := state.GetIssue(context.Background(), root, state.PathResolver{StateHome: stateHome}, "LOAF-2")
	if err != nil {
		t.Fatalf("GetIssue(child) error = %v", err)
	}
	if child.Status == state.IssueStatusActive {
		t.Fatalf("child status = %q, want unchanged after refused join", child.Status)
	}
}

func TestRunnerIssueStartChildRefusesTerminalStartedRoot(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-1"); err != nil {
		t.Fatalf("issue start parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "status", "LOAF-1", "done"); err != nil {
		t.Fatalf("issue status done error = %v", err)
	}

	_, err := runIssue(t, repo, stateHome, "start", "LOAF-2")
	if err == nil || !strings.Contains(err.Error(), "done") {
		t.Fatalf("start child error = %v, want terminal root refusal", err)
	}

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	child, err := state.GetIssue(context.Background(), root, state.PathResolver{StateHome: stateHome}, "LOAF-2")
	if err != nil {
		t.Fatalf("GetIssue(child) error = %v", err)
	}
	if child.Status == state.IssueStatusActive {
		t.Fatalf("child status = %q, want unchanged after refused join", child.Status)
	}
}

func TestRunnerIssueStopChildNamesRootWhenRootNotStarted(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}

	_, err := runIssue(t, repo, stateHome, "stop", "LOAF-2")
	if err == nil || !strings.Contains(err.Error(), "does not own a worktree") || !strings.Contains(err.Error(), "stop LOAF-1") {
		t.Fatalf("stop child error = %v, want root-directed refusal", err)
	}
}

func TestRunnerIssueListStartedShowsLiveAndStale(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Live"); err != nil {
		t.Fatalf("issue new live error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Stale"); err != nil {
		t.Fatalf("issue new stale error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Idle"); err != nil {
		t.Fatalf("issue new idle error = %v", err)
	}

	liveOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start live error = %v", err)
	}
	live := decodeIssueStart(t, liveOut)
	staleOut, err := runIssue(t, repo, stateHome, "start", "LOAF-2", "--json")
	if err != nil {
		t.Fatalf("issue start stale error = %v", err)
	}
	stale := decodeIssueStart(t, staleOut)
	if err := os.RemoveAll(stale.Worktree); err != nil {
		t.Fatalf("RemoveAll(stale worktree) error = %v", err)
	}

	out, err := runIssue(t, repo, stateHome, "list", "--started")
	if err != nil {
		t.Fatalf("issue list --started error = %v", err)
	}
	if !strings.Contains(out, "LOAF-1") || !strings.Contains(out, live.Branch) || !strings.Contains(out, live.Worktree) {
		t.Fatalf("started list missing live row:\n%s", out)
	}
	if !strings.Contains(out, "LOAF-2") || !strings.Contains(out, stale.Branch) || !strings.Contains(out, stale.Worktree) || !strings.Contains(out, "(missing)") {
		t.Fatalf("started list missing stale marker:\n%s", out)
	}
	if strings.Contains(out, "LOAF-3") {
		t.Fatalf("started list included idle issue:\n%s", out)
	}

	jsonOut, err := runIssue(t, repo, stateHome, "list", "--started", "--json")
	if err != nil {
		t.Fatalf("issue list --started --json error = %v", err)
	}
	var listed state.IssueListResult
	if err := json.Unmarshal([]byte(jsonOut), &listed); err != nil {
		t.Fatalf("json.Unmarshal(list) error = %v", err)
	}
	if len(listed.Issues) != 2 {
		t.Fatalf("started json issues = %#v, want 2", listed.Issues)
	}
	byAlias := map[string]state.Issue{}
	for _, issue := range listed.Issues {
		byAlias[issue.Alias] = issue
	}
	if byAlias["LOAF-1"].WorktreeMissing || byAlias["LOAF-1"].StartedBranch != "issue/loaf-1" {
		t.Fatalf("live json row = %#v", byAlias["LOAF-1"])
	}
	if !byAlias["LOAF-2"].WorktreeMissing {
		t.Fatalf("stale json row = %#v, want worktree_missing", byAlias["LOAF-2"])
	}
}

func TestRunnerIssueStartRefusesAlreadyStarted(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Once"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-1"); err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	_, err := runIssue(t, repo, stateHome, "start", "LOAF-1")
	if err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("second start error = %v, want already started", err)
	}
}

func TestRunnerIssueStopRemovesWorktreeKeepsBranch(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Workspace"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if started.Issue.Status != state.IssueStatusActive {
		t.Fatalf("status after start = %q, want active", started.Issue.Status)
	}

	stopOut, err := runIssue(t, repo, stateHome, "stop", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue stop error = %v", err)
	}
	stopped := decodeIssueStop(t, stopOut)
	if stopped.AlreadyGone {
		t.Fatal("stop already_gone = true, want removed")
	}
	if stopped.Issue.StartedBranch != "" || stopped.Issue.StartedWorktree != "" {
		t.Fatalf("stopped issue still started: %#v", stopped.Issue)
	}
	if stopped.Issue.Status != state.IssueStatusActive {
		t.Fatalf("status after stop = %q, want active (stop does not change status)", stopped.Issue.Status)
	}
	if _, err := os.Stat(started.Worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree %s still exists after stop: %v", started.Worktree, err)
	}
	if gitOutputCLI(t, repo, "rev-parse", "--verify", "issue/loaf-1") == "" {
		t.Fatal("branch issue/loaf-1 was deleted; stop must keep it")
	}
}

func TestRunnerIssueStopRefusesDirtyWithoutForce(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Dirty"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if err := os.WriteFile(filepath.Join(started.Worktree, "dirty.txt"), []byte("unstaged\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(dirty) error = %v", err)
	}

	_, err = runIssue(t, repo, stateHome, "stop", "LOAF-1")
	if err == nil || !strings.Contains(err.Error(), "dirty") || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("stop dirty error = %v, want dirty refusal", err)
	}
	if _, statErr := os.Stat(started.Worktree); statErr != nil {
		t.Fatalf("dirty worktree was removed without --force: %v", statErr)
	}

	if _, err := runIssue(t, repo, stateHome, "stop", "LOAF-1", "--force"); err != nil {
		t.Fatalf("stop --force error = %v", err)
	}
	if _, err := os.Stat(started.Worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree %s still exists after --force: %v", started.Worktree, err)
	}
}

func TestRunnerIssueStopRestoresStartedFieldsIfWorktreeRemovalFails(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Restore"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)

	orig := removeIssueWorktreeFn
	removeIssueWorktreeFn = func(repoRoot, worktree string, force bool) (bool, error) {
		return false, errors.New("injected worktree remove failure")
	}
	t.Cleanup(func() { removeIssueWorktreeFn = orig })

	_, err = runIssue(t, repo, stateHome, "stop", "LOAF-1")
	if err == nil || !strings.Contains(err.Error(), "injected worktree remove failure") {
		t.Fatalf("stop error = %v, want injected failure", err)
	}
	if _, statErr := os.Stat(started.Worktree); statErr != nil {
		t.Fatalf("worktree was removed after failed stop: %v", statErr)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	issue, err := state.GetIssue(context.Background(), root, state.PathResolver{StateHome: stateHome}, "LOAF-1")
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if issue.StartedBranch != started.Branch || issue.StartedWorktree != started.Worktree {
		t.Fatalf("started fields after failed stop = %q / %q, want %q / %q", issue.StartedBranch, issue.StartedWorktree, started.Branch, started.Worktree)
	}
}

func TestRunnerIssueStopCleansMissingWorktree(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Gone"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if err := os.RemoveAll(started.Worktree); err != nil {
		t.Fatalf("RemoveAll(worktree) error = %v", err)
	}

	stopOut, err := runIssue(t, repo, stateHome, "stop", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue stop missing error = %v", err)
	}
	stopped := decodeIssueStop(t, stopOut)
	if !stopped.AlreadyGone {
		t.Fatalf("already_gone = false, want true for missing worktree")
	}
	if stopped.Issue.StartedBranch != "" || stopped.Issue.StartedWorktree != "" {
		t.Fatalf("missing stop left started fields: %#v", stopped.Issue)
	}
}

func TestRunnerIssueStartRecordsThroughEventsPath(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Evented"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-1"); err != nil {
		t.Fatalf("issue start error = %v", err)
	}

	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	issue, err := state.GetIssue(context.Background(), root, resolver, "LOAF-1")
	if err != nil {
		t.Fatalf("GetIssue() error = %v", err)
	}
	if issue.Status != state.IssueStatusActive {
		t.Fatalf("status = %q, want active", issue.Status)
	}
	if issue.StartedBranch == "" || issue.StartedWorktree == "" {
		t.Fatalf("started fields empty after start: %#v", issue)
	}

	parity, err := state.CheckIssueStatusParity(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("CheckIssueStatusParity() error = %v", err)
	}
	if !parity.Consistent {
		t.Fatalf("parity = %#v, want column == latest event after start", parity)
	}
}

func TestRunnerIssueStartRefusesTerminalAndArchived(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Done"); err != nil {
		t.Fatalf("issue new done error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Cancelled"); err != nil {
		t.Fatalf("issue new cancelled error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "status", "LOAF-1", "done"); err != nil {
		t.Fatalf("issue status done error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "status", "LOAF-2", "cancelled"); err != nil {
		t.Fatalf("issue status cancelled error = %v", err)
	}

	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-1"); err == nil || !strings.Contains(err.Error(), "done") {
		t.Fatalf("start done error = %v, want terminal refusal", err)
	}
	if _, err := runIssue(t, repo, stateHome, "start", "LOAF-2"); err == nil || !strings.Contains(err.Error(), "archived") && !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("start cancelled error = %v, want archived/terminal refusal", err)
	}
}

func TestQualifyIssueStartBaseUsesOriginRefWhenLocalMissing(t *testing.T) {
	repo, _ := issueGitFixture(t)
	head := gitOutputCLI(t, repo, "rev-parse", "HEAD")
	gitCLI(t, repo, "update-ref", "refs/remotes/origin/trunk", head)
	gitCLI(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	if gitRefExists(repo, "refs/heads/trunk") {
		t.Fatal("fixture must not have a local trunk")
	}
	if got := resolveReleaseDefaultBranch(repo); got != "trunk" {
		t.Fatalf("resolveReleaseDefaultBranch = %q, want trunk", got)
	}
	if got := qualifyIssueStartBase(repo, "trunk"); got != "origin/trunk" {
		t.Fatalf("qualifyIssueStartBase = %q, want origin/trunk", got)
	}
}

func TestRunnerIssueStartUsesRemoteOnlyDefaultBranch(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	head := gitOutputCLI(t, repo, "rev-parse", "HEAD")
	gitCLI(t, repo, "update-ref", "refs/remotes/origin/trunk", head)
	gitCLI(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	if gitRefExists(repo, "refs/heads/trunk") {
		t.Fatal("fixture must not have a local trunk")
	}

	if _, err := runIssue(t, repo, stateHome, "new", "Remote base"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v, want worktree from origin/trunk", err)
	}
	started := decodeIssueStart(t, startedOut)
	if started.Base != "origin/trunk" {
		t.Fatalf("base = %q, want origin/trunk", started.Base)
	}
	if started.Branch != "issue/loaf-1" {
		t.Fatalf("branch = %q, want issue/loaf-1", started.Branch)
	}
	if _, err := os.Stat(started.Worktree); err != nil {
		t.Fatalf("worktree %s: %v", started.Worktree, err)
	}
	if gitOutputCLI(t, repo, "rev-parse", "issue/loaf-1") != head {
		t.Fatalf("new branch HEAD = %s, want origin/trunk %s", gitOutputCLI(t, repo, "rev-parse", "issue/loaf-1"), head)
	}
}

func TestRunnerIssueStopPrunesMissingWorktreeThenRestartSucceeds(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Restart"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if err := os.RemoveAll(started.Worktree); err != nil {
		t.Fatalf("RemoveAll(worktree) error = %v", err)
	}

	if _, err := runIssue(t, repo, stateHome, "stop", "LOAF-1"); err != nil {
		t.Fatalf("issue stop missing error = %v", err)
	}

	restartOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start after pruned stop error = %v", err)
	}
	restarted := decodeIssueStart(t, restartOut)
	if restarted.Branch != started.Branch {
		t.Fatalf("restart branch = %q, want %q", restarted.Branch, started.Branch)
	}
	if _, err := os.Stat(restarted.Worktree); err != nil {
		t.Fatalf("restarted worktree %s: %v", restarted.Worktree, err)
	}
}

func TestRunnerIssueStartRestartAttachesToKeptBranch(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	if _, err := runIssue(t, repo, stateHome, "new", "Keep"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	startedOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue start error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if err := os.WriteFile(filepath.Join(started.Worktree, "kept.txt"), []byte("kept\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(kept) error = %v", err)
	}
	gitCLI(t, started.Worktree, "add", "kept.txt")
	gitCLI(t, started.Worktree, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "kept work")
	head := gitOutputCLI(t, repo, "rev-parse", started.Branch)

	if _, err := runIssue(t, repo, stateHome, "stop", "LOAF-1"); err != nil {
		t.Fatalf("issue stop error = %v", err)
	}
	restartOut, err := runIssue(t, repo, stateHome, "start", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue restart error = %v", err)
	}
	restarted := decodeIssueStart(t, restartOut)
	if restarted.Branch != started.Branch {
		t.Fatalf("restart branch = %q, want %q", restarted.Branch, started.Branch)
	}
	if gitOutputCLI(t, repo, "rev-parse", restarted.Branch) != head {
		t.Fatalf("restart HEAD = %s, want kept %s", gitOutputCLI(t, repo, "rev-parse", restarted.Branch), head)
	}
}

func TestRunnerIssueStartDisambiguatesCaseCollidingAliases(t *testing.T) {
	repo, stateHome := issueGitFixture(t)
	firstOut, err := runIssue(t, repo, stateHome, "new", "Upper", "--json")
	if err != nil {
		t.Fatalf("issue new first error = %v", err)
	}
	secondOut, err := runIssue(t, repo, stateHome, "new", "Lower", "--json")
	if err != nil {
		t.Fatalf("issue new second error = %v", err)
	}
	first := decodeIssueResult(t, firstOut).Issue
	second := decodeIssueResult(t, secondOut).Issue
	rewriteIssueAlias(t, repo, stateHome, first.ID, "FOO")
	rewriteIssueAlias(t, repo, stateHome, second.ID, "foo")

	startedOut, err := runIssue(t, repo, stateHome, "start", first.ID, "--json")
	if err != nil {
		t.Fatalf("issue start first error = %v", err)
	}
	started := decodeIssueStart(t, startedOut)
	if started.Branch != "issue/foo" {
		t.Fatalf("first branch = %q, want issue/foo", started.Branch)
	}
	if _, err := runIssue(t, repo, stateHome, "stop", first.ID); err != nil {
		t.Fatalf("issue stop first error = %v", err)
	}

	collidedOut, err := runIssue(t, repo, stateHome, "start", second.ID, "--json")
	if err != nil {
		t.Fatalf("issue start second error = %v, want disambiguated branch", err)
	}
	collided := decodeIssueStart(t, collidedOut)
	want := "issue/foo-" + issueStartBranchSuffix(second.ID)
	if collided.Branch != want {
		t.Fatalf("second branch = %q, want %q (must not attach to first issue's branch)", collided.Branch, want)
	}
	if collided.Worktree == started.Worktree {
		t.Fatalf("second attached to first worktree %s", started.Worktree)
	}
	if got := gitOutputCLI(t, collided.Worktree, "symbolic-ref", "--short", "HEAD"); got != want {
		t.Fatalf("second worktree HEAD = %s, want %s", got, want)
	}
}

func TestResolveIssueStartBranchOwnership(t *testing.T) {
	repo, _ := issueGitFixture(t)
	first := state.Issue{ID: "issue_aaaabbbbccccdddd1111222233334444", Alias: "FOO"}
	second := state.Issue{ID: "issue_eeeeffff000011112222333344445555", Alias: "foo"}

	got, err := resolveIssueStartBranch(first, []state.Issue{first, second}, repo)
	if err != nil || got != "issue/foo" {
		t.Fatalf("first start = %q err %v, want issue/foo", got, err)
	}

	gitCLI(t, repo, "branch", "issue/foo")
	got, err = resolveIssueStartBranch(first, []state.Issue{first}, repo)
	if err != nil || got != "issue/foo" {
		t.Fatalf("unique restart = %q err %v, want issue/foo", got, err)
	}

	got, err = resolveIssueStartBranch(second, []state.Issue{first, second}, repo)
	if err != nil {
		t.Fatalf("case collision error = %v", err)
	}
	if got != "issue/foo-eeeeffff" {
		t.Fatalf("case collision branch = %q, want issue/foo-eeeeffff", got)
	}

	live := first
	live.StartedBranch = "issue/foo"
	got, err = resolveIssueStartBranch(second, []state.Issue{live, second}, repo)
	if err != nil {
		t.Fatalf("live claim error = %v", err)
	}
	if got != "issue/foo-eeeeffff" {
		t.Fatalf("live claim branch = %q, want issue/foo-eeeeffff", got)
	}

	claimed := live
	claimed.StartedBranch = "issue/foo-eeeeffff"
	_, err = resolveIssueStartBranch(second, []state.Issue{claimed, second}, repo)
	if err == nil || !strings.Contains(err.Error(), "collides") || !strings.Contains(err.Error(), "FOO") || !strings.Contains(err.Error(), "foo") {
		t.Fatalf("double claim error = %v, want both issues named", err)
	}
}

func TestRollbackIssueWorktreeRemovesAddedWorktree(t *testing.T) {
	repo, _ := issueGitFixture(t)
	worktree := filepath.Join(filepath.Dir(repo), "repo-wt", "issue-rollback")
	if _, err := addIssueWorktree(repo, worktree, "issue/rollback", "main"); err != nil {
		t.Fatalf("addIssueWorktree() error = %v", err)
	}
	if err := rollbackIssueWorktree(repo, worktree, "issue/rollback", true); err != nil {
		t.Fatalf("rollbackIssueWorktree() error = %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree %s still exists after rollback: %v", worktree, err)
	}
	if gitRefExists(repo, "refs/heads/issue/rollback") {
		t.Fatal("branch issue/rollback still exists after rollback")
	}
}

func TestRollbackIssueWorktreePreservesPreExistingBranch(t *testing.T) {
	repo, _ := issueGitFixture(t)
	branch := "issue/existing"
	gitCLI(t, repo, "branch", branch)
	worktree := filepath.Join(filepath.Dir(repo), "repo-wt", "issue-existing")
	createdBranch, err := addIssueWorktree(repo, worktree, branch, "main")
	if err != nil {
		t.Fatalf("addIssueWorktree() error = %v", err)
	}
	if createdBranch {
		t.Fatal("addIssueWorktree() created branch, want attach to pre-existing branch")
	}
	if err := rollbackIssueWorktree(repo, worktree, branch, createdBranch); err != nil {
		t.Fatalf("rollbackIssueWorktree() error = %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree %s still exists after rollback: %v", worktree, err)
	}
	if !gitRefExists(repo, "refs/heads/"+branch) {
		t.Fatalf("branch %s was deleted after rollback with createdBranch=false", branch)
	}
}

func TestRollbackIssueWorktreeNamesLeftoversWhenCleanupFails(t *testing.T) {
	dir := t.TempDir()
	worktree := filepath.Join(dir, "missing-wt")
	err := rollbackIssueWorktree(dir, worktree, "issue/foo", true)
	if err == nil {
		t.Fatal("rollbackIssueWorktree() error = nil, want leftover cleanup failure")
	}
	if !strings.Contains(err.Error(), worktree) || !strings.Contains(err.Error(), "issue/foo") {
		t.Fatalf("error = %v, want leftover path and branch", err)
	}
}

func TestWrapIssueStartUpdateErrorIncludesCleanupFailure(t *testing.T) {
	dbErr := errors.New("db write failed")
	if got := wrapIssueStartUpdateError(dbErr, nil, "/tmp/wt", "issue/x"); got != dbErr {
		t.Fatalf("nil cleanup wrap = %v, want db error", got)
	}
	cleanErr := errors.New("git worktree remove failed")
	err := wrapIssueStartUpdateError(dbErr, cleanErr, "/tmp/repo-wt/issue-x", "issue/x")
	if !errors.Is(err, dbErr) {
		t.Fatalf("wrapped error = %v, want to unwrap to db error", err)
	}
	if !strings.Contains(err.Error(), "db write failed") || !strings.Contains(err.Error(), "git worktree remove failed") || !strings.Contains(err.Error(), "/tmp/repo-wt/issue-x") || !strings.Contains(err.Error(), "issue/x") {
		t.Fatalf("error = %v, want both failures and leftover path/branch", err)
	}
}

func rewriteIssueAlias(t *testing.T, repo, stateHome, issueID, alias string) {
	t.Helper()
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	path, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE aliases SET alias = ? WHERE entity_kind = 'issue' AND entity_id = ?`, alias, issueID); err != nil {
		t.Fatalf("UPDATE aliases error = %v", err)
	}
}
