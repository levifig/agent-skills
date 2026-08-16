package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

const cliShapedBody = "The problem is derived from nine required headings.\n\nOut of scope: polish and tracker adapters.\n"

type recordingReadinessPublisher struct {
	publications []ReadinessPublication
}

func (r *recordingReadinessPublisher) Publish(_ context.Context, publication ReadinessPublication) error {
	r.publications = append(r.publications, publication)
	return nil
}

func TestRunnerIssueCheckUncoveredCriterionFailsAndNamesIt(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Covered slice"); err != nil {
		t.Fatalf("dod add 1 error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Left behind"); err != nil {
		t.Fatalf("dod add 2 error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "1"); err != nil {
		t.Fatalf("promote error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1")
	if err == nil {
		t.Fatalf("issue check error = nil, want uncovered failure\n%s", out)
	}
	if !errors.As(err, &ExitError{}) {
		t.Fatalf("error = %v, want ExitError", err)
	}
	if !strings.Contains(out, "uncovered criterion 2: Left behind") {
		t.Fatalf("check output missing named uncovered criterion:\n%s", out)
	}
}

func TestRunnerIssueCheckReportsOrphanWithReadyToPasteRemedy(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Promoted slice"); err != nil {
		t.Fatalf("dod add error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "1"); err != nil {
		t.Fatalf("promote error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-2", "Stray extra work"); err != nil {
		t.Fatalf("dod add orphan error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1")
	if err != nil {
		t.Fatalf("issue check error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "LOAF-2 criterion 2: Stray extra work") {
		t.Fatalf("missing orphan report:\n%s", out)
	}
	wantRemedy := "loaf issue new --parent 'LOAF-1' --status backlog -- 'Stray extra work'"
	if !strings.Contains(out, wantRemedy) {
		t.Fatalf("missing ready-to-paste remedy %q:\n%s", wantRemedy, out)
	}
}

func TestRunnerIssueOrphanRemedyCreatesSiblingInBacklog(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Promoted slice"); err != nil {
		t.Fatalf("dod add error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "1"); err != nil {
		t.Fatalf("promote error = %v", err)
	}
	dangerous := "don't $(touch /tmp/pwned) `reboot`"
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-2", dangerous); err != nil {
		t.Fatalf("dod add orphan error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue check error = %v\n%s", err, out)
	}
	var result issueCheckResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out)
	}
	if len(result.Orphans) != 1 {
		t.Fatalf("orphans = %#v, want one", result.Orphans)
	}
	remedy := result.Orphans[0].Remedy
	if strings.Contains(remedy, `"`) || strings.Contains(remedy, "&&") || strings.Contains(remedy, "<") {
		t.Fatalf("remedy is not a single POSIX-quoted command: %q", remedy)
	}
	argv, err := parsePOSIXArgv(remedy)
	if err != nil {
		t.Fatalf("parsePOSIXArgv(%q) error = %v", remedy, err)
	}
	if len(argv) < 3 || argv[0] != "loaf" || argv[1] != "issue" {
		t.Fatalf("argv = %#v, want loaf issue ...", argv)
	}

	// --json is a flag; inject it before `--` so a hyphen-leading title stays positional.
	createdOut, err := runIssue(t, workingDir, stateHome, issueNewArgsWithJSON(argv[2:])...)
	if err != nil {
		t.Fatalf("emitted command error = %v\n%s", err, createdOut)
	}
	created := decodeIssueResult(t, createdOut)
	if created.Issue.Title != dangerous {
		t.Fatalf("sibling title = %q, want %q", created.Issue.Title, dangerous)
	}
	if created.Issue.Status != state.IssueStatusBacklog {
		t.Fatalf("sibling status = %q, want backlog", created.Issue.Status)
	}
	if created.Issue.ParentID != result.Issue.ID {
		t.Fatalf("sibling parent = %q, want %q", created.Issue.ParentID, result.Issue.ID)
	}
}

func TestParseIssueNewArgsEndOfOptionsAcceptsHyphenLeadingTitle(t *testing.T) {
	options, err := parseIssueNewArgs([]string{"--parent", "LOAF-1", "--status", "backlog", "--", "--help"})
	if err != nil {
		t.Fatalf("parseIssueNewArgs() error = %v", err)
	}
	if options.create.Title != "--help" || options.create.Parent != "LOAF-1" || options.status != state.IssueStatusBacklog {
		t.Fatalf("options = %#v, want title --help under parent LOAF-1 in backlog", options)
	}

	if _, err := parseIssueNewArgs([]string{"--help"}); err == nil || !strings.Contains(err.Error(), `unknown option "--help"`) {
		t.Fatalf("parseIssueNewArgs(--help) error = %v, want unknown option", err)
	}

	bare, err := parseIssueNewArgs([]string{"--", "--help"})
	if err != nil {
		t.Fatalf("parseIssueNewArgs(-- --help) error = %v", err)
	}
	if bare.create.Title != "--help" {
		t.Fatalf("bare title = %q, want --help", bare.create.Title)
	}
}

func TestRunnerIssueOrphanRemedyCreatesSiblingFromHyphenLeadingTitle(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Promoted slice"); err != nil {
		t.Fatalf("dod add error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "1"); err != nil {
		t.Fatalf("promote error = %v", err)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.AddIssueCriterion(context.Background(), root, state.PathResolver{StateHome: stateHome}, "LOAF-2", state.IssueCriterionInput{Text: "--help"}); err != nil {
		t.Fatalf("AddIssueCriterion(--help) error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue check error = %v\n%s", err, out)
	}
	var result issueCheckResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out)
	}
	if len(result.Orphans) != 1 || result.Orphans[0].Text != "--help" {
		t.Fatalf("orphans = %#v, want one titled --help", result.Orphans)
	}
	remedy := result.Orphans[0].Remedy
	if remedy != "loaf issue new --parent 'LOAF-1' --status backlog -- '--help'" {
		t.Fatalf("remedy = %q", remedy)
	}
	argv, err := parsePOSIXArgv(remedy)
	if err != nil {
		t.Fatalf("parsePOSIXArgv(%q) error = %v", remedy, err)
	}

	createdOut, err := runIssue(t, workingDir, stateHome, issueNewArgsWithJSON(argv[2:])...)
	if err != nil {
		t.Fatalf("emitted command error = %v\n%s", err, createdOut)
	}
	created := decodeIssueResult(t, createdOut)
	if created.Issue.Title != "--help" {
		t.Fatalf("sibling title = %q, want --help", created.Issue.Title)
	}
	if created.Issue.Status != state.IssueStatusBacklog {
		t.Fatalf("sibling status = %q, want backlog", created.Issue.Status)
	}
	if created.Issue.ParentID != result.Issue.ID {
		t.Fatalf("sibling parent = %q, want %q", created.Issue.ParentID, result.Issue.ID)
	}
}

func issueNewArgsWithJSON(args []string) []string {
	if len(args) == 0 {
		return []string{"--json"}
	}
	out := make([]string, 0, len(args)+1)
	out = append(out, args[0], "--json")
	out = append(out, args[1:]...)
	return out
}

func parsePOSIXArgv(command string) ([]string, error) {
	var args []string
	i := 0
	for i < len(command) {
		for i < len(command) && command[i] == ' ' {
			i++
		}
		if i >= len(command) {
			break
		}
		if command[i] != '\'' {
			start := i
			for i < len(command) && command[i] != ' ' {
				i++
			}
			args = append(args, command[start:i])
			continue
		}
		var b strings.Builder
		i++
		for i < len(command) {
			if command[i] == '\'' {
				if i+3 < len(command) && command[i:i+4] == `'\''` {
					b.WriteByte('\'')
					i += 4
					continue
				}
				i++
				break
			}
			b.WriteByte(command[i])
			i++
		}
		args = append(args, b.String())
	}
	return args, nil
}

func TestRunnerIssueCheckDecisionReadyOnQuestion(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Pick a store", "--kind", "decision", "--body", "Need a direction."); err != nil {
		t.Fatalf("issue new blank decision error = %v", err)
	}
	blankOut, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1")
	if err == nil {
		t.Fatalf("blank decision check error = nil, want not ready\n%s", blankOut)
	}
	if !strings.Contains(blankOut, "sharp question") {
		t.Fatalf("blank decision output = %q", blankOut)
	}

	if _, err := runIssue(t, workingDir, stateHome, "new", "Should we keep the local store?", "--kind", "decision"); err != nil {
		t.Fatalf("issue new question error = %v", err)
	}
	readyOut, err := runIssue(t, workingDir, stateHome, "check", "LOAF-2")
	if err != nil {
		t.Fatalf("question decision check error = %v\n%s", err, readyOut)
	}
	if !strings.Contains(readyOut, "issue LOAF-2 is ready") {
		t.Fatalf("question decision output = %q", readyOut)
	}
}

func TestRunnerIssuePromoteMakesCoveragePassWithZeroExtraCommands(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "First slice"); err != nil {
		t.Fatalf("dod add 1 error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Second slice"); err != nil {
		t.Fatalf("dod add 2 error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "1"); err != nil {
		t.Fatalf("promote 1 error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "promote", "LOAF-1", "2"); err != nil {
		t.Fatalf("promote 2 error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue check after promote error = %v\n%s", err, out)
	}
	var result issueCheckResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out)
	}
	if !result.Ready || !result.Shaped || !result.Covered {
		t.Fatalf("result = %#v, want ready after full promote", result)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("failures = %#v", result.Failures)
	}
}

func TestRunnerIssueVerifyHonorsExitAndContains(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Verify me", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Exit ok", "--command", "true", "--expect", "exit 0"); err != nil {
		t.Fatalf("dod add exit error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Contains hello", "--command", "echo hello world", "--expect", "contains `hello`"); err != nil {
		t.Fatalf("dod add contains error = %v", err)
	}

	out, err := runIssue(t, workingDir, stateHome, "verify", "LOAF-1")
	if err != nil {
		t.Fatalf("issue verify error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "true") || !strings.Contains(out, "echo hello world") {
		t.Fatalf("verify output = %q", out)
	}

	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Must fail", "--command", "false", "--expect", "exit 0"); err != nil {
		t.Fatalf("dod add fail error = %v", err)
	}
	failOut, err := runIssue(t, workingDir, stateHome, "verify", "LOAF-1")
	if err == nil {
		t.Fatalf("issue verify error = nil, want failure\n%s", failOut)
	}
	if !strings.Contains(failOut, "fail") {
		t.Fatalf("failing verify output = %q", failOut)
	}
}

func TestRunnerIssueCheckPublishesThroughReadinessSeam(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	if _, err := state.SetIssueIdentity(context.Background(), root, state.PathResolver{StateHome: stateHome}, state.IssueIdentityOptions{Authority: state.IssueAuthorityGitHub}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Should we publish readiness?", "--kind", "decision"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	listOut, err := runIssue(t, workingDir, stateHome, "list", "--json")
	if err != nil {
		t.Fatalf("issue list error = %v", err)
	}
	var listed state.IssueListResult
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list json error = %v", err)
	}
	if len(listed.Issues) != 1 {
		t.Fatalf("listed = %#v, want one issue", listed)
	}
	ref := listed.Issues[0].ID

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
	if result.Publication == nil || result.Publication.Label != readinessLabelHuman {
		t.Fatalf("json publication = %#v", result.Publication)
	}
}

func TestRunnerIssueDodServesAndClaimUnclaim(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Parent", "--body", cliShapedBody); err != nil {
		t.Fatalf("issue new parent error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-1", "Parent criterion"); err != nil {
		t.Fatalf("dod add parent error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Child", "--parent", "LOAF-1"); err != nil {
		t.Fatalf("issue new child error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", "LOAF-2", "Serves parent", "--serves", "1"); err != nil {
		t.Fatalf("dod add --serves error = %v", err)
	}
	out, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1")
	if err != nil {
		t.Fatalf("check after --serves error = %v\n%s", err, out)
	}

	if _, err := runIssue(t, workingDir, stateHome, "dod", "unclaim", "LOAF-2", "1", "1"); err != nil {
		t.Fatalf("dod unclaim error = %v", err)
	}
	uncovered, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1")
	if err == nil {
		t.Fatalf("check after unclaim error = nil, want uncovered\n%s", uncovered)
	}
	if _, err := runIssue(t, workingDir, stateHome, "dod", "claim", "LOAF-2", "1", "1"); err != nil {
		t.Fatalf("dod claim error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "check", "LOAF-1"); err != nil {
		t.Fatalf("check after claim error = %v", err)
	}
}
