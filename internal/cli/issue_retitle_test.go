package cli

import (
	"strings"
	"testing"
)

func TestRunnerIssueRetitleReplacesTitle(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	if _, err := runIssue(t, workingDir, stateHome, "new", "Old title"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	out, err := runIssue(t, workingDir, stateHome, "retitle", "LOAF-1", "New title")
	if err != nil {
		t.Fatalf("issue retitle error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "retitled issue LOAF-1") || !strings.Contains(out, "title: New title") {
		t.Fatalf("retitle = %q, want new title", out)
	}
	shown, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1", "--json")
	if err != nil {
		t.Fatalf("issue show error = %v\n%s", err, shown)
	}
	result := decodeIssueResult(t, shown)
	if result.Issue.Title != "New title" {
		t.Fatalf("shown title = %q, want New title", result.Issue.Title)
	}
}

func TestRunnerIssueRetitleRequiresRefAndTitle(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	_, err := runIssue(t, workingDir, stateHome, "retitle", "LOAF-1")
	if err == nil || !strings.Contains(err.Error(), "issue retitle requires an issue ref and a title") {
		t.Fatalf("error = %v, want ref and title", err)
	}
}

func TestRunnerIssueHelpListsRetitle(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	helpOut, err := runIssue(t, workingDir, stateHome, "--help")
	if err != nil {
		t.Fatalf("issue --help error = %v", err)
	}
	if !strings.Contains(helpOut, "retitle") {
		t.Fatalf("issue help missing retitle:\n%s", helpOut)
	}
}
