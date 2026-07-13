package cli

import (
	"bytes"
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

func TestParseJournalLogArgsExecpolicySafeAcceptsAnyOrder(t *testing.T) {
	options, err := parseJournalLogArgs([]string{
		"decision(safe): parser order",
		"--json",
		"--execpolicy-safe",
	})
	if err != nil {
		t.Fatalf("parseJournalLogArgs() error = %v", err)
	}
	if !options.execpolicySafe || !options.jsonOutput || options.entry == "" {
		t.Fatalf("options = %#v, want safe/json/manual entry", options)
	}
}

func TestParseJournalLogArgsExecpolicySafeRejectsProvenanceOverrides(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "worktree", args: []string{"--execpolicy-safe", "--worktree", "/tmp/other", "decision(safe): no"}, want: "--worktree"},
		{name: "branch", args: []string{"--execpolicy-safe", "--branch", "other", "decision(safe): no"}, want: "--branch"},
		{name: "harness session", args: []string{"--execpolicy-safe", "--harness-session-id", "other", "decision(safe): no"}, want: "--harness-session-id"},
		{name: "conflicting sources", args: []string{"--execpolicy-safe", "--from-hook", "--detect-linear"}, want: "cannot combine"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseJournalLogArgs(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseJournalLogArgs(%v) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestJournalLogExecpolicySafeManualUsesRegisteredRuntime(t *testing.T) {
	workingDir, stateHome, databasePath := initExecpolicySafeProject(t)
	var output bytes.Buffer
	err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{
		"journal", "log", "decision(safe): manual runtime", "--json", "--execpolicy-safe",
	})
	if err != nil {
		t.Fatalf("safe manual journal log error = %v\n%s", err, output.String())
	}
	var result state.JournalLogResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode safe manual result = %v\n%s", err, output.String())
	}
	if result.ID == "" || result.ProjectID == "" || result.Message != "manual runtime" {
		t.Fatalf("safe manual result = %#v, want persisted project entry", result)
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 1 {
		t.Fatalf("journal entries = %d, want 1", got)
	}
}

func TestJournalLogExecpolicySafeFromHookWarnsForUnprovenPayload(t *testing.T) {
	workingDir, stateHome, databasePath := initExecpolicySafeProject(t)
	var output bytes.Buffer
	err := (Runner{
		Stdout:     &output,
		WorkingDir: workingDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"safe-hook-session","hook_event_name":"TaskCompleted","task_description":"safe hook event"}`),
	}).Run([]string{"journal", "log", "--json", "--execpolicy-safe", "--from-hook"})
	if err != nil {
		t.Fatalf("safe hook journal log error = %v\n%s", err, output.String())
	}
	var warning journalHookWarning
	if err := json.Unmarshal(output.Bytes(), &warning); err != nil {
		t.Fatalf("decode safe hook result = %v\n%s", err, output.String())
	}
	if warning.Code != journalHookDiagnosticUnsupported || !warning.NonBlocking {
		t.Fatalf("safe hook warning = %#v, want unsupported nonblocking diagnostic", warning)
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
		t.Fatalf("journal entries = %d, want zero for unproven payload", got)
	}
}

func TestJournalLogExecpolicySafeDetectLinearUsesRuntimeGit(t *testing.T) {
	workingDir, stateHome, databasePath := initExecpolicySafeProject(t)
	gitExecpolicySafe(t, workingDir, "init")
	gitExecpolicySafe(t, workingDir, "config", "user.email", "safe@example.test")
	gitExecpolicySafe(t, workingDir, "config", "user.name", "Loaf Safe Test")
	writeFile(t, filepath.Join(workingDir, "marker.txt"), "safe\n")
	gitExecpolicySafe(t, workingDir, "add", "marker.txt")
	gitExecpolicySafe(t, workingDir, "commit", "-m", "Fixes ENG-424")

	var output bytes.Buffer
	err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{
		"journal", "log", "--detect-linear", "--execpolicy-safe", "--json",
	})
	if err != nil {
		t.Fatalf("safe detect-linear journal log error = %v\n%s", err, output.String())
	}
	var result state.JournalLogResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode safe detect-linear result = %v\n%s", err, output.String())
	}
	if !strings.Contains(result.Message, "ENG-424") {
		t.Fatalf("safe detect-linear result = %#v, want ENG-424 discovery", result)
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 1 {
		t.Fatalf("journal entries = %d, want 1", got)
	}
}

func TestJournalLogExecpolicySafeRejectsDatabaseOverrideWithoutMutation(t *testing.T) {
	workingDir, stateHome, databasePath := initExecpolicySafeProject(t)
	overridePath := filepath.Join(t.TempDir(), "redirected.sqlite")
	t.Setenv("LOAF_DB", overridePath)
	var output bytes.Buffer
	err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{
		"journal", "log", "--execpolicy-safe", "decision(safe): redirected", "--json",
	})
	if err == nil {
		t.Fatal("safe redirected journal log error = nil, want visible LOAF_DB refusal")
	}
	if !strings.Contains(output.String(), "refuses non-empty LOAF_DB") {
		t.Fatalf("safe redirected JSON error = %q, want visible diagnostic", output.String())
	}
	if _, statErr := os.Stat(overridePath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("override database stat error = %v, want no database mutation", statErr)
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
		t.Fatalf("registered journal entries = %d, want 0 after rejected redirect", got)
	}
}

func TestJournalLogExecpolicySafeRejectsOverridesWithoutMutation(t *testing.T) {
	workingDir, stateHome, databasePath := initExecpolicySafeProject(t)
	tests := [][]string{
		{"journal", "log", "--execpolicy-safe", "--worktree", "/tmp/other", "decision(safe): no"},
		{"journal", "log", "--execpolicy-safe", "--branch", "other", "decision(safe): no"},
		{"journal", "log", "--execpolicy-safe", "--harness-session-id", "other", "decision(safe): no"},
		{"journal", "log", "--execpolicy-safe", "--from-hook", "--detect-linear"},
	}
	for _, args := range tests {
		var output bytes.Buffer
		err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run(args)
		if err == nil {
			t.Fatalf("safe rejection args = %v, error = nil", args)
		}
		if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
			t.Fatalf("journal entries after rejected args %v = %d, want 0", args, got)
		}
	}
}

func TestJournalLogExecpolicySafeRejectsUnknownProjectWithoutMutation(t *testing.T) {
	_, stateHome, databasePath := initExecpolicySafeProject(t)
	unknownDir := realpath(t, t.TempDir())
	beforeProjects := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM projects`)
	var output bytes.Buffer
	err := (Runner{Stdout: &output, WorkingDir: unknownDir, StateHome: stateHome}).Run([]string{
		"journal", "log", "--execpolicy-safe", "decision(safe): unknown", "--json",
	})
	if err == nil {
		t.Fatalf("safe unknown-project error = nil, want %s", state.ProjectIdentityUnregisteredCode)
	}
	if !strings.Contains(output.String(), state.ProjectIdentityUnregisteredCode) {
		t.Fatalf("safe unknown-project JSON = %q, want identity code", output.String())
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM projects`); got != beforeProjects {
		t.Fatalf("projects after rejected unknown path = %d, want %d", got, beforeProjects)
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
		t.Fatalf("journal entries after rejected unknown path = %d, want 0", got)
	}
}

func TestJournalLogExecpolicySafeHookDoesNotDegradeOnMissingIdentity(t *testing.T) {
	_, stateHome, databasePath := initExecpolicySafeProject(t)
	unknownDir := realpath(t, t.TempDir())
	var output bytes.Buffer
	err := (Runner{
		Stdout:     &output,
		WorkingDir: unknownDir,
		StateHome:  stateHome,
		Stdin:      strings.NewReader(`{"session_id":"safe-hook","hook_event_name":"TaskCompleted","task_description":"unknown project"}`),
	}).Run([]string{"journal", "log", "--execpolicy-safe", "--from-hook", "--json"})
	if err == nil {
		t.Fatal("safe hook unknown-project error = nil, want visible identity failure")
	}
	if !strings.Contains(output.String(), state.ProjectIdentityUnregisteredCode) {
		t.Fatalf("safe hook unknown-project JSON = %q, want identity code", output.String())
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
		t.Fatalf("journal entries after rejected safe hook = %d, want 0", got)
	}
}

func TestJournalLogExecpolicySafeHookValidatesRegisteredEmptyAndUnknownPayloads(t *testing.T) {
	workingDir, stateHome, databasePath := initExecpolicySafeProject(t)
	for _, payload := range []string{"", `{"session_id":"safe-empty"}`, `{"unknown":"field"}`} {
		var output bytes.Buffer
		err := (Runner{
			Stdout:     &output,
			WorkingDir: workingDir,
			StateHome:  stateHome,
			Stdin:      strings.NewReader(payload),
		}).Run([]string{"journal", "log", "--execpolicy-safe", "--from-hook", "--json"})
		if err != nil {
			t.Fatalf("registered safe hook payload %q error = %v\n%s", payload, err, output.String())
		}
		if output.Len() != 0 {
			t.Fatalf("registered safe hook payload %q output = %q, want silent no-op", payload, output.String())
		}
	}
	unknownDir := realpath(t, t.TempDir())
	for _, payload := range []string{"", `{"unknown":"field"}`} {
		var output bytes.Buffer
		err := (Runner{
			Stdout:     &output,
			WorkingDir: unknownDir,
			StateHome:  stateHome,
			Stdin:      strings.NewReader(payload),
		}).Run([]string{"journal", "log", "--execpolicy-safe", "--from-hook", "--json"})
		if err == nil || !strings.Contains(output.String(), state.ProjectIdentityUnregisteredCode) {
			t.Fatalf("unknown safe hook payload %q error=%v output=%q, want registered-project failure", payload, err, output.String())
		}
	}
	if got := execpolicySafeCount(t, databasePath, `SELECT COUNT(*) FROM journal_entries`); got != 0 {
		t.Fatalf("journal entries after safe empty/unknown payloads = %d, want 0", got)
	}
}

func TestJournalLogExecpolicySafeHookPreservesSubagentSilentSuppression(t *testing.T) {
	unknownDir := realpath(t, t.TempDir())
	var output bytes.Buffer
	err := (Runner{
		Stdout:     &output,
		WorkingDir: unknownDir,
		StateHome:  t.TempDir(),
		Stdin:      strings.NewReader(`{"cursor":{"agent_id":"nested-subagent","event":"TaskCompleted"}}`),
	}).Run([]string{"journal", "log", "--execpolicy-safe", "--from-hook", "--json"})
	if err != nil || output.Len() != 0 {
		t.Fatalf("safe subagent hook error=%v output=%q, want silent suppression", err, output.String())
	}
}

func initExecpolicySafeProject(t *testing.T) (workingDir, stateHome, databasePath string) {
	t.Helper()
	t.Setenv("LOAF_DB", "")
	workingDir = realpath(t, t.TempDir())
	stateHome = t.TempDir()
	var output bytes.Buffer
	if err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatalf("state init error = %v\n%s", err, output.String())
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("resolve project root = %v", err)
	}
	databasePath, err = (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("resolve database path = %v", err)
	}
	return workingDir, stateHome, databasePath
}

func gitExecpolicySafe(t *testing.T, workingDir string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = workingDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, output)
	}
}

func execpolicySafeCount(t *testing.T, databasePath, query string) int {
	t.Helper()
	db := openCLITestDB(t, databasePath)
	defer closeCLITestDB(t, db)
	return sqliteCount(t, db, query)
}
