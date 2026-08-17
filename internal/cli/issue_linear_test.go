package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func linearIssueCLIFixture(t *testing.T) (workingDir, stateHome string, fake *state.LinearFake) {
	t.Helper()
	fake = state.NewLinearFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	workingDir, stateHome = issueCLIFixture(t)
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_API_URL", server.URL)
	t.Setenv("LINEAR_TEAM_KEY", "ENG")
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.SetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueIdentityOptions{
		Authority: state.IssueAuthorityLinear,
		Prefix:    "ENG",
	}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	return workingDir, stateHome, fake
}

func TestRunnerIssueNewMintsLinearKeyAndLeavesCounter(t *testing.T) {
	workingDir, stateHome, _ := linearIssueCLIFixture(t)
	out, err := runIssue(t, workingDir, stateHome, "new", "Minted from loaf", "--json")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, out)
	}
	created := decodeIssueResult(t, out)
	if created.Issue.Alias != "ENG-1" {
		t.Fatalf("alias = %q, want ENG-1", created.Issue.Alias)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	identity, err := state.GetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if identity.NextNumber != 1 {
		t.Fatalf("next_number = %d, want 1", identity.NextNumber)
	}
	if strings.Contains(out, "LOAF-") {
		t.Fatalf("minted a local alias:\n%s", out)
	}
}

func TestRunnerIssueNewLinearUnreachableDoesNotMintLocal(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	fake.Unreachable = true
	out, err := runIssue(t, workingDir, stateHome, "new", "Offline idea")
	if err == nil {
		t.Fatalf("issue new error = nil, want unreachable\n%s", out)
	}
	if !strings.Contains(err.Error(), "loaf spark") || !strings.Contains(err.Error(), "loaf idea") {
		t.Fatalf("error = %v, want spark/idea offline path", err)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	listed, listErr := state.ListIssues(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueListOptions{})
	if listErr != nil {
		t.Fatalf("ListIssues() error = %v", listErr)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none after failed mint", listed.Issues)
	}
	identity, idErr := state.GetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome})
	if idErr != nil {
		t.Fatalf("GetIssueIdentity() error = %v", idErr)
	}
	if identity.NextNumber != 1 {
		t.Fatalf("next_number = %d, want 1", identity.NextNumber)
	}
}

func TestRunnerIssueNewReportsOrphanedLinearMintOnLocalFailure(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	out, err := runIssue(t, workingDir, stateHome, "new", "Orphan me", "--kind", "not-a-kind")
	if err == nil {
		t.Fatalf("issue new error = nil, want local bind failure\n%s", out)
	}
	if !strings.Contains(err.Error(), "ENG-1") || !strings.Contains(err.Error(), "loaf issue pull ENG-1") {
		t.Fatalf("error = %v, want Linear key and pull recovery", err)
	}
	if _, ok := fake.Issue("ENG-1"); !ok {
		t.Fatal("Linear issue ENG-1 was not created")
	}
	root, rootErr := project.ResolveRoot(workingDir)
	if rootErr != nil {
		t.Fatalf("ResolveRoot() error = %v", rootErr)
	}
	listed, listErr := state.ListIssues(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueListOptions{})
	if listErr != nil {
		t.Fatalf("ListIssues() error = %v", listErr)
	}
	if len(listed.Issues) != 0 {
		t.Fatalf("issues = %#v, want none after failed local bind", listed.Issues)
	}
}

func TestRunnerIssuePullTreeReattachesExistingChild(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	fake.SeedIssue("ENG-10", "Root", "", "triage", "")
	fake.SeedIssue("ENG-11", "Child", "", "unstarted", "ENG-10")

	if _, err := runIssue(t, workingDir, stateHome, "pull", "ENG-11"); err != nil {
		t.Fatalf("issue pull child error = %v", err)
	}
	shownOut, err := runIssue(t, workingDir, stateHome, "show", "ENG-11", "--json")
	if err != nil {
		t.Fatalf("show child error = %v", err)
	}
	child := decodeIssueResult(t, shownOut)
	if child.Issue.ParentID != "" {
		t.Fatalf("solo child parent = %q, want empty", child.Issue.ParentID)
	}

	if _, err := runIssue(t, workingDir, stateHome, "pull", "ENG-10", "--tree"); err != nil {
		t.Fatalf("issue pull --tree error = %v", err)
	}
	rootOut, err := runIssue(t, workingDir, stateHome, "show", "ENG-10", "--json")
	if err != nil {
		t.Fatalf("show root error = %v", err)
	}
	rootIssue := decodeIssueResult(t, rootOut)
	if len(rootIssue.Children) != 1 || rootIssue.Children[0].Alias != "ENG-11" {
		t.Fatalf("root children = %#v, want depth-2 edge to ENG-11", rootIssue.Children)
	}
	childOut, err := runIssue(t, workingDir, stateHome, "show", "ENG-11", "--json")
	if err != nil {
		t.Fatalf("show reattached child error = %v", err)
	}
	reattached := decodeIssueResult(t, childOut)
	if reattached.Issue.ParentID != rootIssue.Issue.ID {
		t.Fatalf("reattached child parent = %q, want %q", reattached.Issue.ParentID, rootIssue.Issue.ID)
	}
}

func TestRunnerIssuePullTreeKeepsParentEdges(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	fake.SeedIssue("ENG-10", "Root", "", "triage", "")
	fake.SeedIssue("ENG-11", "Child", "", "unstarted", "ENG-10")
	fake.SeedIssue("ENG-12", "Grandchild", "", "started", "ENG-11")

	out, err := runIssue(t, workingDir, stateHome, "pull", "ENG-10", "--tree", "--json")
	if err != nil {
		t.Fatalf("issue pull --tree error = %v\n%s", err, out)
	}
	var result state.LinearPullResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("unmarshal pull: %v\n%s", err, out)
	}
	if result.Issue.Alias != "ENG-10" || len(result.Tree) != 3 {
		t.Fatalf("pull = %#v", result)
	}
	byAlias := map[string]state.Issue{}
	for _, issue := range result.Tree {
		byAlias[issue.Alias] = issue
	}
	if byAlias["ENG-11"].ParentID != byAlias["ENG-10"].ID {
		t.Fatalf("child parent = %q, want %q", byAlias["ENG-11"].ParentID, byAlias["ENG-10"].ID)
	}
	if byAlias["ENG-12"].ParentID != byAlias["ENG-11"].ID {
		t.Fatalf("grandchild parent = %q, want %q", byAlias["ENG-12"].ParentID, byAlias["ENG-11"].ID)
	}
	if byAlias["ENG-12"].Status != state.IssueStatusActive {
		t.Fatalf("grandchild status = %q, want active", byAlias["ENG-12"].Status)
	}
}

func TestRunnerIssuePushAndReconcileHonorStatusAuthority(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	fake.SeedIssue("ENG-20", "Drift", "tracker body", "unstarted", "")
	if _, err := runIssue(t, workingDir, stateHome, "pull", "ENG-20"); err != nil {
		t.Fatalf("issue pull error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "status", "ENG-20", "active"); err != nil {
		t.Fatalf("issue status error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "edit", "ENG-20", "--message", "local shaping body"); err != nil {
		t.Fatalf("issue edit error = %v", err)
	}
	fake.SetIssueState("ENG-20", "completed", time.Now().UTC().Add(time.Hour))
	fake.SetIssueTitle("ENG-20", "Tracker title", time.Now().UTC().Add(time.Hour))

	pushOut, err := runIssue(t, workingDir, stateHome, "push", "ENG-20")
	if err != nil {
		t.Fatalf("issue push error = %v\n%s", err, pushOut)
	}
	if !strings.Contains(pushOut, "status: skipped") {
		t.Fatalf("push should honor newer tracker status:\n%s", pushOut)
	}
	if remote, ok := fake.Issue("ENG-20"); !ok || remote.State.Type != "completed" {
		t.Fatalf("tracker status overwritten: %#v", remote)
	}

	reconOut, err := runIssue(t, workingDir, stateHome, "reconcile", "ENG-20")
	if err != nil {
		t.Fatalf("issue reconcile error = %v\n%s", err, reconOut)
	}
	if !strings.Contains(reconOut, "status: both") && !strings.Contains(reconOut, "status: tracker") {
		t.Fatalf("reconcile missing status drift:\n%s", reconOut)
	}
	if !strings.Contains(reconOut, "--take-local") && !strings.Contains(reconOut, "unresolved") {
		t.Fatalf("reconcile resolved silently:\n%s", reconOut)
	}
	if !strings.Contains(reconOut, "title: tracker wins") {
		t.Fatalf("reconcile missing title drift:\n%s", reconOut)
	}
	if strings.Contains(reconOut, "description: drifted") {
		t.Fatalf("reconcile reported false description drift after push:\n%s", reconOut)
	}

	shownOut, err := runIssue(t, workingDir, stateHome, "show", "ENG-20", "--json")
	if err != nil {
		t.Fatalf("issue show error = %v", err)
	}
	shown := decodeIssueResult(t, shownOut)
	if shown.Issue.Title != "Tracker title" {
		t.Fatalf("local title = %q, want tracker title", shown.Issue.Title)
	}
	if shown.Issue.Status != state.IssueStatusActive {
		t.Fatalf("local status = %q, want still active until --take-*", shown.Issue.Status)
	}
	if shown.Issue.Body != "local shaping body" {
		t.Fatalf("local body rewritten from tracker: %q", shown.Issue.Body)
	}

	takeOut, err := runIssue(t, workingDir, stateHome, "reconcile", "ENG-20", "--take-tracker")
	if err != nil {
		t.Fatalf("issue reconcile --take-tracker error = %v\n%s", err, takeOut)
	}
	shownOut, err = runIssue(t, workingDir, stateHome, "show", "ENG-20", "--json")
	if err != nil {
		t.Fatalf("issue show after take-tracker error = %v", err)
	}
	shown = decodeIssueResult(t, shownOut)
	if shown.Issue.Status != state.IssueStatusDone {
		t.Fatalf("local status after --take-tracker = %q, want done", shown.Issue.Status)
	}

	if _, err := runIssue(t, workingDir, stateHome, "status", "ENG-20", "todo"); err != nil {
		t.Fatalf("issue status todo error = %v", err)
	}
	takeLocalOut, err := runIssue(t, workingDir, stateHome, "reconcile", "ENG-20", "--take-local")
	if err != nil {
		t.Fatalf("issue reconcile --take-local error = %v\n%s", err, takeLocalOut)
	}
	if remote, ok := fake.Issue("ENG-20"); !ok || remote.State.Type != "unstarted" {
		t.Fatalf("tracker after --take-local = %#v, want unstarted", remote)
	}
}

func TestRunnerIssuePushThenReconcileHasNoFalseDescriptionDrift(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	fake.SeedIssue("ENG-30", "Shaped", "raw tracker body", "unstarted", "")
	if _, err := runIssue(t, workingDir, stateHome, "pull", "ENG-30"); err != nil {
		t.Fatalf("issue pull error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "edit", "ENG-30", "--message", "local shaping body"); err != nil {
		t.Fatalf("issue edit error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "push", "ENG-30"); err != nil {
		t.Fatalf("issue push error = %v", err)
	}
	reconOut, err := runIssue(t, workingDir, stateHome, "reconcile", "ENG-30")
	if err != nil {
		t.Fatalf("issue reconcile error = %v\n%s", err, reconOut)
	}
	if strings.Contains(reconOut, "description: drifted") {
		t.Fatalf("false description drift after push:\n%s", reconOut)
	}
	fake.SetIssueDescription("ENG-30", "tracker edited the render", time.Now().UTC())
	driftOut, err := runIssue(t, workingDir, stateHome, "reconcile", "ENG-30")
	if err != nil {
		t.Fatalf("issue reconcile after remote edit error = %v\n%s", err, driftOut)
	}
	if !strings.Contains(driftOut, "description: drifted") {
		t.Fatalf("missing description drift after remote edit:\n%s", driftOut)
	}
}

func TestRunnerIssueCheckPublishesReadyForAgentThroughLinear(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	out, err := runIssue(t, workingDir, stateHome, "new", "Should we ship the adapter?", "--kind", "decision")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, out)
	}
	checkOut, err := runIssue(t, workingDir, stateHome, "check", "ENG-1", "--json")
	if err != nil {
		t.Fatalf("issue check error = %v\n%s", err, checkOut)
	}
	names := fake.IssueLabelNames("ENG-1")
	if !containsString(names, readinessLabelAgent) {
		t.Fatalf("labels = %#v, want %s", names, readinessLabelAgent)
	}

	humanOut, err := runIssue(t, workingDir, stateHome, "check", "ENG-1", "--human", "needs a human call")
	if err != nil {
		t.Fatalf("issue check --human error = %v\n%s", err, humanOut)
	}
	if !containsString(fake.IssueLabelNames("ENG-1"), readinessLabelHuman) {
		t.Fatalf("human labels = %#v", fake.IssueLabelNames("ENG-1"))
	}
	comments := fake.IssueComments("ENG-1")
	if len(comments) != 1 || comments[0].Body != "needs a human call" {
		t.Fatalf("comments = %#v", comments)
	}
}

func TestRunnerIssueCheckAttachesConfiguredTeamLabel(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	other := fake.SeedLabel("team_other", readinessLabelAgent)
	want := fake.SeedLabel(fake.Team.ID, readinessLabelAgent)
	out, err := runIssue(t, workingDir, stateHome, "new", "Should we ship the adapter?", "--kind", "decision")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, out)
	}
	if _, err := runIssue(t, workingDir, stateHome, "check", "ENG-1"); err != nil {
		t.Fatalf("issue check error = %v", err)
	}
	remote, ok := fake.Issue("ENG-1")
	if !ok {
		t.Fatal("ENG-1 missing from fake")
	}
	if !containsString(remote.LabelIDs, want.ID) {
		t.Fatalf("labels = %#v, want configured team label %q", remote.LabelIDs, want.ID)
	}
	if containsString(remote.LabelIDs, other.ID) {
		t.Fatalf("labels = %#v, attached other team's label %q", remote.LabelIDs, other.ID)
	}
}

func TestRunnerIssuePromoteMintsLinearChildAndLeavesCounter(t *testing.T) {
	workingDir, stateHome, fake := linearIssueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "ENG-1", "First slice"); err != nil {
		t.Fatalf("dod add error = %v", err)
	}
	out, err := runIssue(t, workingDir, stateHome, "promote", "ENG-1", "1", "--json")
	if err != nil {
		t.Fatalf("issue promote error = %v\n%s", err, out)
	}
	child := decodeIssueResult(t, out)
	if child.Issue.Alias != "ENG-2" {
		t.Fatalf("promoted alias = %q, want ENG-2", child.Issue.Alias)
	}
	if _, ok := fake.Issue("ENG-2"); !ok {
		t.Fatal("Linear issue ENG-2 was not created")
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	identity, err := state.GetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if identity.NextNumber != 1 {
		t.Fatalf("next_number = %d, want 1", identity.NextNumber)
	}
	shown, err := state.ShowIssue(context.Background(), root, state.PathResolver{StateHome: stateHome}, child.Issue.ID)
	if err != nil {
		t.Fatalf("ShowIssue() error = %v", err)
	}
	if shown.Issue.ParentID == "" {
		t.Fatal("promoted child has empty parent")
	}
}

func TestRunnerIssueLocalAuthorityDoesNotCallLinear(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_API_URL", server.URL)
	t.Setenv("LINEAR_TEAM_KEY", "ENG")

	out, err := runIssue(t, workingDir, stateHome, "new", "Local only", "--json")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, out)
	}
	created := decodeIssueResult(t, out)
	if created.Issue.Alias != "LOAF-1" {
		t.Fatalf("alias = %q, want LOAF-1", created.Issue.Alias)
	}
	if hits != 0 {
		t.Fatalf("linear hits = %d, want 0 for local authority", hits)
	}
}

func TestRunnerIssueNewUsesTeamMappingWithoutEnvTeamKey(t *testing.T) {
	workingDir, stateHome, _ := linearIssueCLIFixture(t)
	t.Setenv("LINEAR_TEAM_KEY", "")
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if err := state.WriteLinearTeamConfig(context.Background(), root, state.PathResolver{StateHome: stateHome}, "ENG"); err != nil {
		t.Fatalf("WriteLinearTeamConfig() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "new", "From mapping")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "ENG-1") {
		t.Fatalf("output missing Linear key:\n%s", out)
	}
}

func TestRunnerReleaseCutPushesLinearMembership(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	fake := state.NewLinearFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_API_URL", server.URL)
	t.Setenv("LINEAR_TEAM_KEY", "ENG")
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.SetIssueIdentity(context.Background(), root, resolver, state.IssueIdentityOptions{
		Authority: state.IssueAuthorityLinear,
		Prefix:    "ENG",
	}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	out, err := runIssue(t, repo, stateHome, "new", "Ship auth")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth ENG-1")
	gitCLI(t, repo, "tag", "v1.1.0")

	cutOut, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("release cut error = %v\n%s", err, cutOut)
	}
	keys := fake.ReleaseIssueKeys("1.1.0")
	if !containsString(keys, "ENG-1") {
		t.Fatalf("linear release members = %#v, want ENG-1; cut output:\n%s", keys, cutOut)
	}
	remote, ok := fake.Release("1.1.0")
	if !ok {
		t.Fatalf("linear release missing; cut output:\n%s", cutOut)
	}
	client := state.NewLinearClient(server.URL, "test-key")
	readBack, err := client.Release(context.Background(), remote.ID)
	if err != nil {
		t.Fatalf("Release() read-back error = %v", err)
	}
	if !containsString(readBack.IssueKeys, "ENG-1") {
		t.Fatalf("read-back members = %#v, want ENG-1", readBack.IssueKeys)
	}
}

func TestRunnerReleaseCutWarnsOnLinearPublicationFailure(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	fake := state.NewLinearFake()
	fake.ReleaseMutationError = "release create exploded"
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_API_URL", server.URL)
	t.Setenv("LINEAR_TEAM_KEY", "ENG")
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.SetIssueIdentity(context.Background(), root, resolver, state.IssueIdentityOptions{
		Authority: state.IssueAuthorityLinear,
		Prefix:    "ENG",
	}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth ENG-1")
	gitCLI(t, repo, "tag", "v1.1.0")

	stdout, stderr, err := runReleaseTrackIO(t, repo, stateHome, "cut", "--no-tag", "--no-gh", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("release cut error = %v\n%s", err, stdout)
	}
	if !strings.Contains(stderr, "Linear publication failed") || !strings.Contains(stderr, "release create exploded") {
		t.Fatalf("stderr = %q, want Linear failure specifics", stderr)
	}
	if strings.Contains(stdout, "Recorded Linear release") {
		t.Fatalf("stdout claimed Linear success:\n%s", stdout)
	}
}

func TestRunnerReleaseCutUnsupportedWorkspaceIsSilent(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	fake := state.NewLinearFake()
	fake.SupportsReleases = false
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_API_URL", server.URL)
	t.Setenv("LINEAR_TEAM_KEY", "ENG")
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.SetIssueIdentity(context.Background(), root, resolver, state.IssueIdentityOptions{
		Authority: state.IssueAuthorityLinear,
		Prefix:    "ENG",
	}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth ENG-1")
	gitCLI(t, repo, "tag", "v1.1.0")

	stdout, stderr, err := runReleaseTrackIO(t, repo, stateHome, "cut", "--no-tag", "--no-gh", "--base", "v1.0.0")
	if err != nil {
		t.Fatalf("release cut error = %v\n%s", err, stdout)
	}
	if strings.Contains(stderr, "Linear") || strings.Contains(stderr, "warning:") {
		t.Fatalf("unsupported workspace should be silent, stderr = %q", stderr)
	}
}

func TestWarnLinearReleasePublicationNamesUnmappedMembers(t *testing.T) {
	var stderr bytes.Buffer
	runner := Runner{Stderr: &stderr}
	runner.warnLinearReleasePublication(&bytes.Buffer{}, state.Release{Tag: "v1.1.0", TaggedCommit: "abc123"}, "", []string{"ENG-9", "ENG-10"})
	if !strings.Contains(stderr.String(), "unmapped members: ENG-9, ENG-10") {
		t.Fatalf("stderr = %q, want unmapped member keys", stderr.String())
	}
}

func TestRunnerReleaseCutLocalAuthorityDoesNotCallLinear(t *testing.T) {
	repo, stateHome := releaseTrackFixture(t)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	t.Setenv("LINEAR_API_KEY", "test-key")
	t.Setenv("LINEAR_API_URL", server.URL)
	t.Setenv("LINEAR_TEAM_KEY", "ENG")
	if _, err := runIssue(t, repo, stateHome, "new", "Ship auth"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	writeFile(t, filepath.Join(repo, "auth.txt"), "auth\n")
	gitCLI(t, repo, "add", "auth.txt")
	gitCLI(t, repo, "commit", "-m", "feat: add auth LOAF-1")
	gitCLI(t, repo, "tag", "v1.1.0")
	if _, err := runReleaseTrack(t, repo, stateHome, "cut", "--no-tag", "--no-gh", "--base", "v1.0.0"); err != nil {
		t.Fatalf("release cut error = %v", err)
	}
	if hits != 0 {
		t.Fatalf("linear hits = %d, want 0 for local authority", hits)
	}
}

func TestIssueAbsorbMintsLinearIdentityAndLeavesCounter(t *testing.T) {
	workingDir, stateHome, _ := linearIssueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	created, err := state.CreateTask(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.TaskCreateOptions{Title: "Tracker leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "absorb", created.Task.Alias, "--json")
	if err != nil {
		t.Fatalf("issue absorb error = %v\n%s", err, out)
	}
	result := decodeAbsorbResult(t, out)
	if result.Issue == nil || result.Issue.Alias != "ENG-1" {
		t.Fatalf("alias = %#v, want ENG-1", result.Issue)
	}
	identity, err := state.GetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if identity.NextNumber != 1 {
		t.Fatalf("next_number = %d, want 1", identity.NextNumber)
	}
	if strings.Contains(out, "LOAF-") {
		t.Fatalf("minted a local alias:\n%s", out)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", created.Task.Alias); err == nil {
		t.Fatal("issue show TASK alias error = nil, want missing issue")
	}
}
