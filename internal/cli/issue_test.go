package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func issueCLIFixture(t *testing.T) (string, string) {
	t.Helper()
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	return workingDir, stateHome
}

func runIssue(t *testing.T, workingDir, stateHome string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run(append([]string{"issue"}, args...))
	return stdout.String(), err
}

func decodeIssueResult(t *testing.T, data string) state.IssueResult {
	t.Helper()
	var result state.IssueResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", data, err)
	}
	return result
}

func TestRunnerIssueNewEditShowRoundTripsBody(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	body := "Line one.\n\nLine two with trailing space. \n\tindented\n"

	createdOut, err := runIssue(t, workingDir, stateHome, "new", "Round trip", "--body", body, "--json")
	if err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	created := decodeIssueResult(t, createdOut)
	if created.Issue.Alias != "LOAF-1" {
		t.Fatalf("created alias = %q, want LOAF-1", created.Issue.Alias)
	}
	if created.Issue.Body != body {
		t.Fatalf("created body = %q, want %q", created.Issue.Body, body)
	}
	assertCLIProjectContext(t, workingDir, created.ContractVersion, created.DatabaseScope, created.DatabasePath, created.ProjectID, created.ProjectName, created.ProjectCurrentPath)

	bodyFile := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(body) error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "edit", "LOAF-1", "--body-file", bodyFile); err != nil {
		t.Fatalf("issue edit --body-file error = %v", err)
	}

	shownOut, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue show error = %v", err)
	}
	shown := decodeIssueResult(t, shownOut)
	if shown.Issue.Body != body {
		t.Fatalf("show body = %q, want byte-identical %q", shown.Issue.Body, body)
	}
}

func TestRunnerIssueTreePrintsDepthThree(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent"); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Grandchild", "--parent", "LOAF-2"); err != nil {
		t.Fatalf("issue new grandchild error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "tree", "LOAF-1")
	if err != nil {
		t.Fatalf("issue tree error = %v", err)
	}
	if !strings.Contains(out, "LOAF-1  triage  Parent") {
		t.Fatalf("tree missing parent line:\n%s", out)
	}
	if !strings.Contains(out, "  LOAF-2  triage  Child") {
		t.Fatalf("tree missing indented child:\n%s", out)
	}
	if !strings.Contains(out, "    LOAF-3  triage  Grandchild") {
		t.Fatalf("tree missing indented grandchild:\n%s", out)
	}
}

func TestRunnerIssueRenderIsCompletePRBody(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Ship the CLI", "--body", "Implement the issue spine.\n"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Tests pass"); err != nil {
		t.Fatalf("issue dod add error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Child work", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "render", "LOAF-1")
	if err != nil {
		t.Fatalf("issue render error = %v", err)
	}
	for _, want := range []string{
		"# Ship the CLI\n",
		"Implement the issue spine.\n",
		"## Definition of Done\n",
		"- [ ] Tests pass\n",
		"## Children\n",
		"- LOAF-2: Child work\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "scope:") || strings.Contains(out, "database:") {
		t.Fatalf("render included project headers that would require editing:\n%s", out)
	}

	if _, err := runIssue(t, workingDir, stateHome, "status", "LOAF-1", "done"); err != nil {
		t.Fatalf("issue status done error = %v", err)
	}
	doneOut, err := runIssue(t, workingDir, stateHome, "render", "LOAF-1")
	if err != nil {
		t.Fatalf("issue render(done) error = %v", err)
	}
	if !strings.Contains(doneOut, "- [x] Tests pass\n") {
		t.Fatalf("done render missing checked criterion:\n%s", doneOut)
	}
}

func TestRunnerIssueFrontierExcludesBlockedAndArchived(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Open"); err != nil {
		t.Fatalf("issue new open error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Blocker"); err != nil {
		t.Fatalf("issue new blocker error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Blocked"); err != nil {
		t.Fatalf("issue new blocked error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Cancelled"); err != nil {
		t.Fatalf("issue new cancelled error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "link", "LOAF-2", "blocks", "LOAF-3"); err != nil {
		t.Fatalf("issue link blocks error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "status", "LOAF-4", "cancelled"); err != nil {
		t.Fatalf("issue status cancelled error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "frontier")
	if err != nil {
		t.Fatalf("issue frontier error = %v", err)
	}
	if !strings.Contains(out, "LOAF-1") || !strings.Contains(out, "LOAF-2") {
		t.Fatalf("frontier missing open issues:\n%s", out)
	}
	if strings.Contains(out, "LOAF-3") {
		t.Fatalf("frontier included blocked issue:\n%s", out)
	}
	if strings.Contains(out, "LOAF-4") {
		t.Fatalf("frontier included archived issue:\n%s", out)
	}

	if _, err := runIssue(t, workingDir, stateHome, "status", "LOAF-2", "done"); err != nil {
		t.Fatalf("issue status blocker done error = %v", err)
	}
	unblocked, err := runIssue(t, workingDir, stateHome, "frontier")
	if err != nil {
		t.Fatalf("issue frontier after unblock error = %v", err)
	}
	if !strings.Contains(unblocked, "LOAF-3") {
		t.Fatalf("frontier after blocker done missing LOAF-3:\n%s", unblocked)
	}
}

func TestRunnerIssueStatusHonorsRemovalSemantics(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Keep"); err != nil {
		t.Fatalf("issue new keep error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Cancel me"); err != nil {
		t.Fatalf("issue new cancel error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Dup me"); err != nil {
		t.Fatalf("issue new dup error = %v", err)
	}

	cancelledOut, err := runIssue(t, workingDir, stateHome, "status", "LOAF-2", "cancelled", "--json")
	if err != nil {
		t.Fatalf("issue status cancelled error = %v", err)
	}
	cancelled := decodeIssueResult(t, cancelledOut)
	if cancelled.Issue.Status != state.IssueStatusCancelled || cancelled.Issue.ArchivedAt == "" {
		t.Fatalf("cancelled = %#v, want cancelled and archived", cancelled.Issue)
	}

	if _, err := runIssue(t, workingDir, stateHome, "status", "LOAF-3", "duplicate"); err == nil {
		t.Fatal("duplicate without survivor must fail")
	}

	dupOut, err := runIssue(t, workingDir, stateHome, "status", "LOAF-3", "duplicate", "--duplicate-of", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue status duplicate error = %v", err)
	}
	dup := decodeIssueResult(t, dupOut)
	if dup.Issue.Status != state.IssueStatusDuplicate || dup.Issue.ArchivedAt == "" {
		t.Fatalf("duplicate = %#v, want duplicate and archived", dup.Issue)
	}

	if _, err := runIssue(t, workingDir, stateHome, "status", "LOAF-1", "todo", "--duplicate-of", "LOAF-2"); err == nil {
		t.Fatal("duplicate-of on a write status must fail")
	}
}

func TestRunnerIssueListDodPromoteBucketExportAndHelp(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Listed", "--kind", "decision", "--fog", "still fuzzy"); err != nil {
		t.Fatalf("issue new listed error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Other"); err != nil {
		t.Fatalf("issue new other error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "status", "LOAF-2", "todo"); err != nil {
		t.Fatalf("issue status todo error = %v", err)
	}

	listOut, err := runIssue(t, workingDir, stateHome, "list", "--kind", "decision")
	if err != nil {
		t.Fatalf("issue list --kind error = %v", err)
	}
	if !strings.Contains(listOut, "LOAF-1") || strings.Contains(listOut, "LOAF-2") {
		t.Fatalf("kind filter = %q", listOut)
	}

	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "A human check"); err != nil {
		t.Fatalf("dod add H error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "A verify check", "--command", "true", "--expect", "exit 0"); err != nil {
		t.Fatalf("dod add V error = %v", err)
	}
	dodOut, err := runIssue(t, workingDir, stateHome, "dod", "list", "LOAF-1")
	if err != nil {
		t.Fatalf("dod list error = %v", err)
	}
	if !strings.Contains(dodOut, "[H] A human check") || !strings.Contains(dodOut, "[V] A verify check") {
		t.Fatalf("dod list = %q", dodOut)
	}

	promoteOut, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "1")
	if err != nil {
		t.Fatalf("issue promote error = %v", err)
	}
	if !strings.Contains(promoteOut, "promoted criterion 1 to LOAF-3") {
		t.Fatalf("promote output = %q", promoteOut)
	}

	if _, err := runIssue(t, workingDir, stateHome, "bucket", "LOAF-1", "now"); err != nil {
		t.Fatalf("issue bucket error = %v", err)
	}
	showOut, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1")
	if err != nil {
		t.Fatalf("issue show error = %v", err)
	}
	if !strings.Contains(showOut, "bucket: now") || !strings.Contains(showOut, "fog: still fuzzy") {
		t.Fatalf("show = %q", showOut)
	}

	exportOut, err := runIssue(t, workingDir, stateHome, "export")
	if err != nil {
		t.Fatalf("issue export error = %v", err)
	}
	var snapshot state.IssueExportSnapshot
	if err := json.Unmarshal([]byte(exportOut), &snapshot); err != nil {
		t.Fatalf("export JSON error = %v", err)
	}
	if snapshot.ExportKind != state.ExportKindIssue || len(snapshot.Issues) < 2 || len(snapshot.Criteria) < 2 {
		t.Fatalf("export snapshot = %#v", snapshot)
	}

	helpOut, err := runIssue(t, workingDir, stateHome, "--help")
	if err != nil {
		t.Fatalf("issue --help error = %v", err)
	}
	if !strings.Contains(helpOut, "loaf issue <subcommand>") {
		t.Fatalf("issue help = %q", helpOut)
	}
}

func TestRunnerIssueNewTrackerAuthorityPrintsOpaqueID(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.SetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueIdentityOptions{Authority: state.IssueAuthorityGitHub}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "new", "Tracker backed")
	if err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if strings.Contains(out, "LOAF-") {
		t.Fatalf("tracker create minted a local alias:\n%s", out)
	}
	if !strings.Contains(out, "note: no local alias is minted under a tracker authority") {
		t.Fatalf("tracker create missing note:\n%s", out)
	}
}

func TestRunnerIssueRequiresSQLiteAndUnknownSubcommand(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"issue", "list"})
	want := "loaf issue list requires initialized SQLite state; run `loaf state init` or `loaf state migrate markdown --apply` first"
	if err == nil || err.Error() != want {
		t.Fatalf("issue list without state error = %v, want %q", err, want)
	}

	workingDir, stateHome = issueCLIFixture(t)
	err = Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"issue", "explode"})
	if err == nil || !strings.Contains(err.Error(), `unknown loaf issue subcommand "explode"`) {
		t.Fatalf("unknown subcommand error = %v", err)
	}
}

func TestRunnerLegacyHelpRedirectsToIssue(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	for _, args := range [][]string{
		{"task", "--help"},
		{"spec", "--help"},
		{"intent", "--help"},
	} {
		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: workingDir}).Run(args); err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		if !strings.Contains(stdout.String(), "Superseded by loaf issue for new work.") {
			t.Fatalf("%v help missing redirect:\n%s", args, stdout.String())
		}
	}
}

func TestRunnerIssueEditRequiresBodyFlag(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Needs body"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	t.Setenv("EDITOR", "false")
	err := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"issue", "edit", "LOAF-1"})
	want := "issue edit requires body content via --body-file, --body -, or --message"
	if err == nil || err.Error() != want {
		t.Fatalf("issue edit without body error = %v, want %q", err, want)
	}
}

func TestRunnerRootHelpListsIssue(t *testing.T) {
	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: t.TempDir()}).Run([]string{"--help"}); err != nil {
		t.Fatalf("loaf --help error = %v", err)
	}
	if !strings.Contains(stdout.String(), "issue         Manage issues") {
		t.Fatalf("root help missing issue:\n%s", stdout.String())
	}
}
