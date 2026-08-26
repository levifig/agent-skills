package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/state"
)

func TestRunnerIssueFlowCommandsOperateOnAuthorityRefs(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	ref := "branch:issue/flow-86"

	createdOut, err := runIssue(t, workingDir, stateHome, "new", "--ref", ref, "Flow skills operate on refs",
		"--body", "Skills must address work as authority refs.\n\nOut of scope: tracker UI.", "--json")
	if err != nil {
		t.Fatalf("issue new --ref error = %v\n%s", err, createdOut)
	}
	created := decodeIssueResult(t, createdOut)
	if created.Issue.Alias != ref {
		t.Fatalf("created alias = %q, want %q", created.Issue.Alias, ref)
	}

	shownOut, err := runIssue(t, workingDir, stateHome, "show", ref, "--json")
	if err != nil {
		t.Fatalf("issue show ref error = %v\n%s", err, shownOut)
	}
	shown := decodeIssueResult(t, shownOut)
	if shown.Issue.Title != "Flow skills operate on refs" {
		t.Fatalf("show title = %q", shown.Issue.Title)
	}

	if _, err := runIssue(t, workingDir, stateHome, "dod", "add", ref, "V-tier package test",
		"--command", "go test ./internal/cli/ -run TestRunnerIssueFlowCommandsOperateOnAuthorityRefs",
		"--expect", "exit 0"); err != nil {
		t.Fatalf("issue dod add ref error = %v", err)
	}

	checkOut, err := runIssue(t, workingDir, stateHome, "check", ref, "--json")
	if err != nil {
		t.Fatalf("issue check ref error = %v\n%s", err, checkOut)
	}
	var check issueCheckResult
	if err := json.Unmarshal([]byte(checkOut), &check); err != nil {
		t.Fatalf("unmarshal check: %v\n%s", err, checkOut)
	}
	if !check.Ready || !check.Shaped {
		t.Fatalf("check ready=%v shaped=%v failures=%#v", check.Ready, check.Shaped, check.Failures)
	}

	frontierOut, err := runIssue(t, workingDir, stateHome, "frontier")
	if err != nil {
		t.Fatalf("issue frontier error = %v\n%s", err, frontierOut)
	}
	if !strings.Contains(frontierOut, ref) {
		t.Fatalf("frontier missing %s:\n%s", ref, frontierOut)
	}

	frontierJSON, err := runIssue(t, workingDir, stateHome, "frontier", "--json")
	if err != nil {
		t.Fatalf("issue frontier --json error = %v\n%s", err, frontierJSON)
	}
	var frontier state.IssueFrontierResult
	if err := json.Unmarshal([]byte(frontierJSON), &frontier); err != nil {
		t.Fatalf("unmarshal frontier: %v\n%s", err, frontierJSON)
	}
	if len(frontier.Refs) != 1 || frontier.Refs[0].AuthorityRef.String() != ref {
		t.Fatalf("frontier refs = %#v, want [%s]", frontier.Refs, ref)
	}

	if _, err := runIssue(t, workingDir, stateHome, "status", ref, "done"); err != nil {
		t.Fatalf("issue status ref done error = %v", err)
	}
	after, err := runIssue(t, workingDir, stateHome, "frontier")
	if err != nil {
		t.Fatalf("issue frontier after done error = %v\n%s", err, after)
	}
	if strings.Contains(after, ref) {
		t.Fatalf("frontier still lists done ref:\n%s", after)
	}
}

func TestRunnerIssueNewRefRefusesUnsupportedProvider(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	out, err := runIssue(t, workingDir, stateHome, "new", "--ref", "github:owner-1", "Unsupported")
	if err == nil {
		t.Fatalf("issue new --ref github error = nil, want refusal\n%s", out)
	}
	if !strings.Contains(err.Error(), "not a v1 authority provider") {
		t.Fatalf("error = %v, want unsupported provider message", err)
	}
}
