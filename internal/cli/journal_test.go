package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/internal/state"
)

func TestJournalCommandsRefuseUnknownMovedCheckoutAndPreserveIdentity(t *testing.T) {
	parent := t.TempDir()
	oldPath := filepath.Join(parent, "old checkout's path")
	newPath := filepath.Join(parent, "new checkout with spaces'")
	if err := os.MkdirAll(oldPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(old) error = %v", err)
	}
	if err := os.MkdirAll(newPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(new) error = %v", err)
	}
	oldPath = realpath(t, oldPath)
	newPath = realpath(t, newPath)
	databasePath := filepath.Join(t.TempDir(), "isolated journal.sqlite")
	t.Setenv("LOAF_DB", databasePath)

	var stdout bytes.Buffer
	oldRunner := Runner{Stdout: &stdout, WorkingDir: oldPath}
	if err := oldRunner.Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatalf("state init --json error = %v\n%s", err, stdout.String())
	}
	stdout.Reset()
	if err := oldRunner.Run([]string{"journal", "log", "decision(move): preserve continuity", "--json"}); err != nil {
		t.Fatalf("journal log --json error = %v\n%s", err, stdout.String())
	}
	var logged state.JournalLogResult
	if err := json.Unmarshal(stdout.Bytes(), &logged); err != nil {
		t.Fatalf("decode journal log = %v\n%s", err, stdout.String())
	}
	if logged.ProjectID == "" || logged.ID == "" {
		t.Fatalf("logged = %#v, want project and entry IDs", logged)
	}

	countRows := func(query string) int {
		db := openCLITestDB(t, databasePath)
		defer closeCLITestDB(t, db)
		return sqliteCount(t, db, query)
	}
	projectsBefore := countRows(`SELECT COUNT(*) FROM projects`)
	pathsBefore := countRows(`SELECT COUNT(*) FROM project_paths`)
	entriesBefore := countRows(`SELECT COUNT(*) FROM journal_entries`)

	if err := os.RemoveAll(newPath); err != nil {
		t.Fatalf("RemoveAll(new) error = %v", err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename(old,new) error = %v", err)
	}
	newPath = realpath(t, newPath)
	moveCommand := fmt.Sprintf("loaf project move --from '%s' --to '%s' --dry-run", strings.ReplaceAll(oldPath, "'", "'\\''"), strings.ReplaceAll(newPath, "'", "'\\''"))

	commands := [][]string{
		{"journal", "recent"},
		{"journal", "search", "continuity"},
		{"journal", "context"},
		{"journal", "log", "decision(move): must refuse"},
	}
	for _, args := range commands {
		var human bytes.Buffer
		err := (Runner{Stdout: &human, WorkingDir: newPath}).Run(args)
		if err == nil {
			t.Fatalf("Run(%v) error = nil, want unregistered project identity", args)
		}
		for _, want := range []string{newPath, moveCommand, "loaf state init"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Run(%v) error = %q, want %q", args, err, want)
			}
		}

		jsonArgs := append(append([]string{}, args...), "--json")
		var machine bytes.Buffer
		err = (Runner{Stdout: &machine, WorkingDir: newPath}).Run(jsonArgs)
		if err == nil {
			t.Fatalf("Run(%v) error = nil, want JSON command error", jsonArgs)
		}
		var output struct {
			ContractVersion   int      `json:"contract_version"`
			Command           string   `json:"command"`
			Error             string   `json:"error"`
			Code              string   `json:"code"`
			CurrentPath       string   `json:"current_path"`
			KnownCurrentPaths []string `json:"known_current_paths"`
			Suggestions       []struct {
				Action  string `json:"action"`
				Command string `json:"command"`
			} `json:"suggestions"`
		}
		if err := json.Unmarshal(machine.Bytes(), &output); err != nil {
			t.Fatalf("decode Run(%v) JSON error = %v\n%s", jsonArgs, err, machine.String())
		}
		if output.ContractVersion != state.StateJSONContractVersion || output.Code != state.ProjectIdentityUnregisteredCode || output.CurrentPath != newPath {
			t.Fatalf("Run(%v) JSON error = %#v, want contract/code/path", jsonArgs, output)
		}
		if len(output.KnownCurrentPaths) != 1 || output.KnownCurrentPaths[0] != oldPath {
			t.Fatalf("Run(%v) known paths = %#v, want [%q]", jsonArgs, output.KnownCurrentPaths, oldPath)
		}
		if len(output.Suggestions) != 2 || output.Suggestions[0].Action != "move-project" || output.Suggestions[0].Command != moveCommand || output.Suggestions[1].Action != "initialize-project" || output.Suggestions[1].Command != "loaf state init" {
			t.Fatalf("Run(%v) suggestions = %#v, want move then initialize", jsonArgs, output.Suggestions)
		}
		if output.Error == "" || !strings.Contains(output.Error, moveCommand) {
			t.Fatalf("Run(%v) JSON error = %#v, want deterministic human diagnostic", jsonArgs, output)
		}
		if got := countRows(`SELECT COUNT(*) FROM projects`); got != projectsBefore {
			t.Fatalf("projects after Run(%v) = %d, want %d", jsonArgs, got, projectsBefore)
		}
		if got := countRows(`SELECT COUNT(*) FROM project_paths`); got != pathsBefore {
			t.Fatalf("project_paths after Run(%v) = %d, want %d", jsonArgs, got, pathsBefore)
		}
		if got := countRows(`SELECT COUNT(*) FROM journal_entries`); got != entriesBefore {
			t.Fatalf("journal_entries after Run(%v) = %d, want %d", jsonArgs, got, entriesBefore)
		}
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: newPath}).Run([]string{"project", "move", "--from", oldPath, "--to", newPath, "--json"}); err != nil {
		t.Fatalf("project move --json error = %v\n%s", err, stdout.String())
	}
	var moved state.ProjectMoveResult
	if err := json.Unmarshal(stdout.Bytes(), &moved); err != nil {
		t.Fatalf("decode project move = %v\n%s", err, stdout.String())
	}
	if moved.Project.ID != logged.ProjectID || moved.Project.CurrentPath != newPath {
		t.Fatalf("moved project = %#v, want ID %q at %q", moved.Project, logged.ProjectID, newPath)
	}

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: newPath}).Run([]string{"journal", "recent", "--json"}); err != nil {
		t.Fatalf("journal recent after move --json error = %v\n%s", err, stdout.String())
	}
	var recent state.JournalRecent
	if err := json.Unmarshal(stdout.Bytes(), &recent); err != nil {
		t.Fatalf("decode recent after move = %v\n%s", err, stdout.String())
	}
	if recent.ProjectID != logged.ProjectID || len(recent.Entries) != 1 || recent.Entries[0].ID != logged.ID {
		t.Fatalf("recent after move = %#v, want original project and entry", recent)
	}

	db := openCLITestDB(t, databasePath)
	defer closeCLITestDB(t, db)
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM projects`); got != 1 {
		t.Fatalf("projects after move = %d, want 1", got)
	}
	if got := sqliteCount(t, db, `SELECT COUNT(*) FROM project_paths WHERE is_current = 1`); got != 1 {
		t.Fatalf("current project mappings after move = %d, want 1", got)
	}
}

func TestParseJournalDeferArgsStrictContract(t *testing.T) {
	base := []string{"intent", "--why", "why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "operation"}
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "valid flags anywhere", args: append([]string{"--json"}, base...), want: true},
		{name: "missing why", args: []string{"intent", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "operation"}},
		{name: "blank boundary", args: []string{"intent", "--why", "why", "--boundary", " ", "--trigger", "trigger", "--operation-id", "operation"}},
		{name: "duplicate trigger", args: append(append([]string{}, base...), "--trigger", "again")},
		{name: "unknown option", args: append(append([]string{}, base...), "--unknown")},
		{name: "extra positional", args: append(append([]string{}, base...), "extra")},
		{name: "missing value", args: []string{"intent", "--why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "operation"}},
		{name: "duplicate json", args: append(append([]string{"--json"}, base...), "--json")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseJournalDeferArgs(test.args)
			if (err == nil) != test.want {
				t.Fatalf("parseJournalDeferArgs(%v) error = %v, want success=%t", test.args, err, test.want)
			}
		})
	}
}

func TestJournalDeferJSONParseErrorsAreOneObjectAndSilent(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var output bytes.Buffer
	err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "defer", "intent", "--why", "why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "op", "--unknown", "--json"})
	if err == nil {
		t.Fatal("journal defer parse error = nil")
	}
	var exitErr interface {
		ExitCode() int
		Silent() bool
	}
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !exitErr.Silent() {
		t.Fatalf("journal defer parse error = %v, want silent exit 1", err)
	}
	var envelope struct {
		ContractVersion int    `json:"contract_version"`
		Command         string `json:"command"`
		Code            string `json:"code"`
		Error           string `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("decode parse error = %v\n%s", err, output.String())
	}
	if envelope.ContractVersion != state.StateJSONContractVersion || envelope.Command != "journal defer" || envelope.Code != "journal-defer-validation" || envelope.Error == "" {
		t.Fatalf("parse error envelope = %#v, raw=%q", envelope, output.String())
	}
}

func TestJournalDeferCreatesReciprocalPairAndReusesOnRetry(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	args := []string{"journal", "defer", "capture boundary", "--why", "not enough evidence", "--boundary", "outside packet", "--trigger", "new evidence", "--operation-id", "defer-cli-1"}
	var firstOut bytes.Buffer
	if err := (Runner{Stdout: &firstOut, WorkingDir: workingDir, StateHome: stateHome}).Run(args); err != nil {
		t.Fatalf("journal defer error = %v\n%s", err, firstOut.String())
	}
	for _, want := range []string{"created decision + spark", "operation: defer-cli-1", "decision:", "spark:", "alias:", "input digest:", "stored digest:", "digest match: true"} {
		if !strings.Contains(firstOut.String(), want) {
			t.Fatalf("journal defer output = %q, want %q", firstOut.String(), want)
		}
	}

	var retryOut bytes.Buffer
	retryArgs := append([]string{}, args...)
	retryArgs = append(retryArgs, "--json")
	if err := (Runner{Stdout: &retryOut, WorkingDir: workingDir, StateHome: stateHome}).Run(retryArgs); err != nil {
		t.Fatalf("identical retry error = %v\n%s", err, retryOut.String())
	}
	var retry state.JournalDeferResult
	if err := json.Unmarshal(retryOut.Bytes(), &retry); err != nil {
		t.Fatalf("decode retry = %v\n%s", err, retryOut.String())
	}
	if retry.Created || !retry.InputDigestMatches || retry.Decision.ID == "" || retry.Spark.ID == "" || retry.Spark.Alias == "" {
		t.Fatalf("retry = %#v, want original reciprocal pair and matching digest", retry)
	}

	var rewordedOut bytes.Buffer
	reworded := []string{"journal", "defer", "reworded intent", "--why", "different why", "--boundary", "different boundary", "--trigger", "different trigger", "--operation-id", "defer-cli-1"}
	if err := (Runner{Stdout: &rewordedOut, WorkingDir: workingDir, StateHome: stateHome}).Run(reworded); err != nil {
		t.Fatalf("reworded retry error = %v\n%s", err, rewordedOut.String())
	}
	if !strings.Contains(rewordedOut.String(), "reused existing decision + spark") || !strings.Contains(rewordedOut.String(), "digest mismatch") || !strings.Contains(rewordedOut.String(), retry.Decision.ID) || !strings.Contains(rewordedOut.String(), retry.Spark.ID) {
		t.Fatalf("reworded retry output = %q, want reuse warning and original pair", rewordedOut.String())
	}
}

func TestJournalDeferWithoutChangeSucceedsOutsideGit(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var output bytes.Buffer
	if err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "defer", "no git intent", "--why", "why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "no-git", "--json"}); err != nil {
		t.Fatalf("journal defer outside git error = %v\n%s", err, output.String())
	}
	var result state.JournalDeferResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode no-git result = %v\n%s", err, output.String())
	}
	if result.Origin == nil || result.Origin.CaptureMechanism != state.JournalOriginMechanismManual || result.Origin.SourceEvent != "journal.defer" || result.Origin.Branch != "" || result.Origin.Worktree != "" || result.Origin.Head != "" || result.Origin.ChangePath != "" {
		t.Fatalf("no-git origin = %#v, want self-sufficient manual envelope without fabricated git metadata", result.Origin)
	}
}

func TestJournalDeferWithoutChangeCapturesAvailableGitOrigin(t *testing.T) {
	repo, _, _ := committedOriginFixture(t, "manual-defer", "20260711")
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatalf("state init: %v", err)
	}
	var output bytes.Buffer
	if err := (Runner{Stdout: &output, WorkingDir: repo, StateHome: stateHome}).Run([]string{"journal", "defer", "git intent", "--why", "why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "git-origin", "--json"}); err != nil {
		t.Fatalf("journal defer in git: %v\n%s", err, output.String())
	}
	var result state.JournalDeferResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode git defer result: %v\n%s", err, output.String())
	}
	wantHead := strings.TrimSpace(mustOriginGitOutput(t, repo, "rev-parse", "HEAD"))
	if result.Origin == nil || result.Origin.EnvelopeVersion != state.JournalOriginEnvelopeVersion || result.Origin.CaptureMechanism != state.JournalOriginMechanismManual || result.Origin.SourceEvent != "journal.defer" || result.Origin.Branch != "main" || result.Origin.Worktree != realpath(t, repo) || result.Origin.Head != wantHead || result.Origin.ChangePath != "" || result.Origin.ChangeSHA256 != "" {
		t.Fatalf("git defer origin = %#v, want manual git context without Change evidence", result.Origin)
	}
	var showOut bytes.Buffer
	if err := (Runner{Stdout: &showOut, WorkingDir: repo, StateHome: stateHome}).Run([]string{"journal", "show", result.Decision.ID, "--json"}); err != nil {
		t.Fatalf("journal show: %v\n%s", err, showOut.String())
	}
	var shown state.JournalShow
	if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
		t.Fatalf("decode shown defer: %v", err)
	}
	if shown.Origin == nil || shown.Origin.Head != wantHead || shown.Origin.SourceEvent != "journal.defer" {
		t.Fatalf("shown defer origin = %#v", shown.Origin)
	}
}

func TestJournalLogCapturesManualOriginAcrossGitContexts(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T) (workingDir, branch, worktree, head string)
	}{
		{
			name: "committed checkout",
			prepare: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				repo, _, _ := committedOriginFixture(t, "manual-log", "20260711")
				return repo, "main", realpath(t, repo), strings.TrimSpace(mustOriginGitOutput(t, repo, "rev-parse", "HEAD"))
			},
		},
		{
			name: "detached checkout",
			prepare: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				repo, _, _ := committedOriginFixture(t, "manual-detached", "20260711")
				head := strings.TrimSpace(mustOriginGitOutput(t, repo, "rev-parse", "HEAD"))
				if err := originGitCLI(repo, "checkout", "--detach", head); err != nil {
					t.Fatalf("detach checkout: %v", err)
				}
				return repo, "", realpath(t, repo), head
			},
		},
		{
			name: "outside git",
			prepare: func(t *testing.T) (string, string, string, string) {
				t.Helper()
				return realpath(t, t.TempDir()), "", "", ""
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workingDir, wantBranch, wantWorktree, wantHead := tt.prepare(t)
			stateHome := t.TempDir()
			runner := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
			if err := runner.Run([]string{"state", "init", "--json"}); err != nil {
				t.Fatalf("state init: %v", err)
			}
			var logOut bytes.Buffer
			if err := (Runner{Stdout: &logOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "decision(manual): capture origin", "--json"}); err != nil {
				t.Fatalf("journal log: %v\n%s", err, logOut.String())
			}
			var logged state.JournalLogResult
			if err := json.Unmarshal(logOut.Bytes(), &logged); err != nil {
				t.Fatalf("decode log: %v\n%s", err, logOut.String())
			}
			var showOut bytes.Buffer
			if err := (Runner{Stdout: &showOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "show", logged.ID, "--json"}); err != nil {
				t.Fatalf("journal show: %v\n%s", err, showOut.String())
			}
			var shown state.JournalShow
			if err := json.Unmarshal(showOut.Bytes(), &shown); err != nil {
				t.Fatalf("decode show: %v\n%s", err, showOut.String())
			}
			if shown.Origin == nil {
				t.Fatal("shown manual journal origin = nil")
			}
			if shown.Origin.EnvelopeVersion != state.JournalOriginEnvelopeVersion || shown.Origin.CaptureMechanism != state.JournalOriginMechanismManual || shown.Origin.SourceEvent != "journal.log" {
				t.Fatalf("shown origin = %#v, want manual journal.log v1 envelope", shown.Origin)
			}
			if shown.Origin.Branch != wantBranch || shown.Origin.Worktree != wantWorktree || shown.Origin.Head != wantHead {
				t.Fatalf("shown origin = %#v, want branch=%q worktree=%q head=%q", shown.Origin, wantBranch, wantWorktree, wantHead)
			}
			if shown.Origin.ObservedHarness != "" || shown.Origin.ObservedHarnessVersion != "" || shown.Origin.HarnessSessionID != "" || shown.Origin.AgentID != "" {
				t.Fatalf("shown origin fabricated optional harness metadata: %#v", shown.Origin)
			}
		})
	}
}

func TestJournalShowRendersOriginOnlyWhenPresent(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	var deferOut bytes.Buffer
	if err := (Runner{Stdout: &deferOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "defer", "show origin", "--why", "why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "show-origin", "--json"}); err != nil {
		t.Fatalf("journal defer --json error = %v\n%s", err, deferOut.String())
	}
	var deferred state.JournalDeferResult
	if err := json.Unmarshal(deferOut.Bytes(), &deferred); err != nil {
		t.Fatalf("decode deferred = %v", err)
	}
	var showOut bytes.Buffer
	if err := (Runner{Stdout: &showOut, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "show", deferred.Decision.ID}); err != nil {
		t.Fatalf("journal show error = %v\n%s", err, showOut.String())
	}
	if !strings.Contains(showOut.String(), "provenance:") || !strings.Contains(showOut.String(), "mechanism: manual") || !strings.Contains(showOut.String(), "source event: journal.defer") {
		t.Fatalf("journal show output = %q, want provenance block", showOut.String())
	}
	var showJSON bytes.Buffer
	if err := (Runner{Stdout: &showJSON, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "show", deferred.Decision.ID, "--json"}); err != nil {
		t.Fatalf("journal show --json error = %v\n%s", err, showJSON.String())
	}
	var shown state.JournalShow
	if err := json.Unmarshal(showJSON.Bytes(), &shown); err != nil {
		t.Fatalf("decode journal show = %v", err)
	}
	if shown.Origin == nil || shown.Origin.SourceEvent != "journal.defer" {
		t.Fatalf("shown origin = %#v", shown.Origin)
	}
}

func TestJournalDeferPublicCLIConvergesAcrossIndependentProcesses(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	t.Setenv("LOAF_DB", databasePath)
	workingDir, _ := setupJournalHookRunner(t)
	binary := buildCLIBinaryForTest(t)
	args := []string{"journal", "defer", "process convergence", "--why", "response may be lost", "--boundary", "do not execute", "--trigger", "when resumed", "--operation-id", "process-convergence", "--json"}
	start := make(chan struct{})
	results := make([]struct {
		result state.JournalDeferResult
		output string
		err    error
	}, 2)
	var wg sync.WaitGroup
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			cmd := exec.Command(binary, args...)
			cmd.Dir = workingDir
			cmd.Env = append(os.Environ(), "LOAF_DB="+databasePath)
			output, err := cmd.CombinedOutput()
			results[index].output = string(output)
			results[index].err = err
			if err == nil {
				results[index].err = json.Unmarshal(output, &results[index].result)
			}
		}(index)
	}
	close(start)
	wg.Wait()
	for index, result := range results {
		if result.err != nil {
			t.Fatalf("process %d error = %v\n%s", index, result.err, result.output)
		}
	}
	if results[0].result.Created == results[1].result.Created {
		t.Fatalf("created flags = %v/%v, want exactly one creator", results[0].result.Created, results[1].result.Created)
	}
	if results[0].result.Decision.ID != results[1].result.Decision.ID || results[0].result.Spark.ID != results[1].result.Spark.ID || results[0].result.Spark.Alias != results[1].result.Spark.Alias || results[0].result.OperationID != results[1].result.OperationID {
		t.Fatalf("process results diverged = %#v / %#v", results[0].result, results[1].result)
	}
	if !results[0].result.InputDigestMatches || !results[1].result.InputDigestMatches || results[0].result.InputDigest != results[1].result.InputDigest || results[0].result.StoredDigest != results[1].result.StoredDigest {
		t.Fatalf("process digest telemetry diverged = %#v / %#v", results[0].result, results[1].result)
	}
	db := openCLITestDB(t, databasePath)
	defer closeCLITestDB(t, db)
	for table, want := range map[string]int{"journal_entries": 1, "sparks": 1, "journal_deferrals": 1, "journal_origins": 1} {
		if got := sqliteCount(t, db, "SELECT COUNT(*) FROM "+table); got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
}

func TestJournalDeferMissingChangeFailsBeforeWritingPair(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "missing-change", "20260711")
	writeCLIAgentsFile(t, repo, "specs/SPEC-001-active.md", "---\nid: SPEC-001\ntitle: Active Spec\nstatus: implementing\n---\n# Active Spec\n")
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	stateHome := t.TempDir()
	t.Setenv("LOAF_DB", databasePath)
	var output bytes.Buffer
	if err := (Runner{Stdout: &output, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate error = %v\n%s", err, output.String())
	}
	if err := os.Remove(changeFile); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	err := (Runner{Stdout: &output, WorkingDir: repo, StateHome: stateHome}).Run([]string{"journal", "defer", "missing source", "--why", "why", "--boundary", "boundary", "--trigger", "trigger", "--operation-id", "missing-change", "--change", "missing-change", "--json"})
	if err == nil || !strings.Contains(output.String(), "change-not-found") {
		t.Fatalf("missing Change defer error = %v output=%q, want typed change-not-found", err, output.String())
	}
	db := openCLITestDB(t, databasePath)
	defer closeCLITestDB(t, db)
	for table, want := range map[string]int{"journal_deferrals": 0, "sparks": 0, "journal_origins": 0} {
		if got := sqliteCount(t, db, "SELECT COUNT(*) FROM "+table); got != want {
			t.Fatalf("%s rows after missing Change = %d, want %d", table, got, want)
		}
	}
}

func TestJournalDeferDirtyChangePersistsSelfSufficientPacketAfterWorktreeRemoval(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "durable-dirty-change", "20260711")
	writeCLIAgentsFile(t, repo, "specs/SPEC-001-active.md", "---\nid: SPEC-001\ntitle: Active Spec\nstatus: implementing\n---\n# Active Spec\n")
	databasePath := filepath.Join(t.TempDir(), "loaf.sqlite")
	stateHome := t.TempDir()
	t.Setenv("LOAF_DB", databasePath)
	if err := os.WriteFile(changeFile, []byte("---\nslug: durable-dirty-change\n---\ndirty working Change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := (Runner{Stdout: &output, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "migrate", "markdown", "--apply"}); err != nil {
		t.Fatalf("state migrate error = %v\n%s", err, output.String())
	}
	output.Reset()
	if err := (Runner{Stdout: &output, WorkingDir: repo, StateHome: stateHome}).Run([]string{"journal", "defer", "durable intent", "--why", "durable reason", "--boundary", "durable boundary", "--trigger", "durable trigger", "--operation-id", "durable-dirty", "--change", "durable-dirty-change", "--json"}); err != nil {
		t.Fatalf("journal defer dirty Change error = %v\n%s", err, output.String())
	}
	var deferred state.JournalDeferResult
	if err := json.Unmarshal(output.Bytes(), &deferred); err != nil {
		t.Fatalf("decode deferred dirty Change = %v\n%s", err, output.String())
	}
	if deferred.Origin == nil || deferred.Origin.ChangePath == "" || deferred.Origin.Dirty == nil || !*deferred.Origin.Dirty || deferred.Origin.Reconstructable == nil || *deferred.Origin.Reconstructable {
		t.Fatalf("dirty Change origin = %#v, want dirty=true/reconstructable=false", deferred.Origin)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	db := openCLITestDB(t, databasePath)
	defer closeCLITestDB(t, db)
	var decisionMessage, sparkText, changePath, changeSHA string
	var dirty, reconstructable int
	if err := db.QueryRow(`
SELECT j.message, s.text, o.change_path, o.change_sha256, o.dirty, o.reconstructable
FROM journal_deferrals AS d
JOIN journal_entries AS j ON j.id = d.journal_entry_id
JOIN sparks AS s ON s.id = d.spark_id
JOIN journal_origins AS o ON o.journal_entry_id = d.journal_entry_id
WHERE d.operation_key = ?`, "durable-dirty").Scan(&decisionMessage, &sparkText, &changePath, &changeSHA, &dirty, &reconstructable); err != nil {
		t.Fatalf("read persisted dirty Change packet after worktree removal: %v", err)
	}
	if !strings.Contains(decisionMessage, "Intent: durable intent") || !strings.Contains(decisionMessage, "Why: durable reason") || !strings.Contains(decisionMessage, "Boundary: durable boundary") || !strings.Contains(decisionMessage, "Trigger: durable trigger") || !strings.Contains(sparkText, "Intent: durable intent") {
		t.Fatalf("persisted packet lost self-sufficient fields: decision=%q spark=%q", decisionMessage, sparkText)
	}
	if changePath != deferred.Origin.ChangePath || changeSHA != deferred.Origin.ChangeSHA256 || dirty != 1 || reconstructable != 0 {
		t.Fatalf("persisted origin = path %q sha %q dirty %d reconstructable %d, want resolver pointer", changePath, changeSHA, dirty, reconstructable)
	}
}

func buildCLIBinaryForTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Dir(filepath.Dir(mustWorkingDirectory(t)))
	binary := filepath.Join(root, "loaf")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/loaf")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build CLI binary: %v\n%s", err, output)
	}
	return binary
}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return workingDir
}

// TestJournalSearchReturnsHitsWithoutScanningDocs proves `loaf journal search`
// (SPEC-056 M1) returns journal hits without touching the docs index: a broken
// (invalid UTF-8) docs fixture that would fail the docs scan must not affect the
// journal search command.
func TestJournalSearchReturnsHitsWithoutScanningDocs(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)

	logRun := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRun.Run([]string{"journal", "log", "decision(search): sphinx searchable entry"}); err != nil {
		t.Fatalf("journal log error = %v", err)
	}

	// A docs file whose bytes are not valid UTF-8 makes any docs-index scan fail.
	// If journal search routed through the global docs-indexing path, it would
	// error on this fixture.
	writeBrokenCLIDocsFile(t, workingDir, "docs/broken.md")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"journal", "search", "sphinx"})
	if err != nil {
		t.Fatalf("journal search error = %v (must not scan docs)", err)
	}
	out := stdout.String()
	if strings.Contains(out, "docs") && strings.Contains(out, "UTF-8") {
		t.Fatalf("journal search stdout = %q, must not report a docs scan error", out)
	}
	for _, want := range []string{"loaf journal search", "results: 1", "sphinx searchable entry"} {
		if !strings.Contains(out, want) {
			t.Fatalf("journal search stdout = %q, want %q", out, want)
		}
	}
}

// TestJournalSearchJSONReturnsOnlyJournalHits confirms the JSON output shape
// carries the journal hits the command emits today.
func TestJournalSearchJSONReturnsOnlyJournalHits(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	logRun := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRun.Run([]string{"journal", "log", "finding(search): obsidian json hit"}); err != nil {
		t.Fatalf("journal log error = %v", err)
	}
	writeBrokenCLIDocsFile(t, workingDir, "docs/broken.md")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "search", "--json", "obsidian"}); err != nil {
		t.Fatalf("journal search --json error = %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"source": "journal_entry"`) {
		t.Fatalf("journal search --json stdout = %q, want journal_entry hit", out)
	}
	if strings.Contains(out, `"source": "docs_index"`) || strings.Contains(out, `"source": "artifact_body"`) {
		t.Fatalf("journal search --json stdout = %q, want only journal hits", out)
	}
}

// TestJournalSearchErrorsOnMissingStateNonHook confirms the interactive search
// path still errors when state is missing (M2 leaves non-hook behavior intact).
func TestJournalSearchErrorsOnMissingStateNonHook(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir() // no state init: database absent

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: workingDir, StateHome: stateHome}.Run([]string{"journal", "search", "anything"})
	if err == nil {
		t.Fatal("journal search error = nil, want uninitialized-state error on missing DB")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("journal search error = %v, want uninitialized-state sentinel", err)
	}
}

func writeBrokenCLIDocsFile(t *testing.T, rootPath string, relativePath string) {
	t.Helper()
	path := filepath.Join(rootPath, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relativePath, err)
	}
}
