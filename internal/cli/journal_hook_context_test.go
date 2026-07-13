package cli

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func TestEvaluateJournalHookContextMissingStateReturnsStructuredWarningWithoutMutation(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := project.ResolveWorkingDirectory(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	runtime := state.NewRuntime(workingDirectory)
	input := &journalHookInput{Raw: map[string]any{"session_id": "primary"}}
	before, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Runner{WorkingDir: workingDir, StateHome: stateHome}).evaluateJournalHookContext(context.Background(), runtime, root, journalContextCLIOptions{fromHook: true}, input, true)
	if err != nil {
		t.Fatalf("evaluateJournalHookContext() error = %v", err)
	}
	if result.disposition != journalHookContextWarning {
		t.Fatalf("disposition = %q, want %q", result.disposition, journalHookContextWarning)
	}
	if result.context != nil || result.warning == nil {
		t.Fatalf("result = %#v, want warning without context", result)
	}
	if result.warning.Code != journalHookDiagnosticMissingState || !result.warning.NonBlocking {
		t.Fatalf("warning = %#v, want structured nonblocking missing-state warning", result.warning)
	}
	after, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("state home changed during missing-state query: before=%v after=%v", before, after)
	}
}

func TestEvaluateJournalHookContextSuppressesSubagentAndBackgroundPayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := project.ResolveWorkingDirectory(workingDir)
	if err != nil {
		t.Fatal(err)
	}
	runtime := state.NewRuntime(workingDirectory)
	for name, raw := range map[string]map[string]any{
		"subagent":   {"session_id": "child", "agent_id": "child-1"},
		"background": {"session_id": "background", "is_background": true},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := (Runner{WorkingDir: workingDir, StateHome: stateHome}).evaluateJournalHookContext(context.Background(), runtime, root, journalContextCLIOptions{fromHook: true}, &journalHookInput{Raw: raw}, true)
			if err != nil {
				t.Fatalf("evaluateJournalHookContext() error = %v", err)
			}
			if result.disposition != journalHookContextSuppressed || result.context != nil || result.warning != nil {
				t.Fatalf("result = %#v, want structurally suppressed/no output", result)
			}
			if err := result.validate(); err != nil {
				t.Fatalf("suppressed result validation error = %v", err)
			}
		})
	}
}

func TestEvaluateJournalHookContextAvailableResultIncludesActiveChanges(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "hook-context-active", "20260713")
	if err := os.WriteFile(changeFile, []byte("---\nchange: hook-context-active\ncreated: 2026-07-13\nbranch: main\nlineage: hook-context-line\n---\nworking bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	runner := Runner{WorkingDir: repo, StateHome: stateHome, Stdout: &bytes.Buffer{}}
	if err := runner.Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	root, err := project.ResolveRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	workingDirectory, err := project.ResolveWorkingDirectory(repo)
	if err != nil {
		t.Fatal(err)
	}
	runtime := state.NewRuntime(workingDirectory)
	result, err := runner.evaluateJournalHookContext(context.Background(), runtime, root, journalContextCLIOptions{}, nil, false)
	if err != nil {
		t.Fatalf("evaluateJournalHookContext() error = %v", err)
	}
	if result.disposition != journalHookContextModelAvailable || result.context == nil || result.warning != nil {
		t.Fatalf("result = %#v, want complete available context", result)
	}
	active := result.context.Layers.ActiveChanges
	if active == nil || active.AvailableCount != 1 || len(active.Items) != 1 || active.Items[0].Slug != "hook-context-active" {
		t.Fatalf("active changes = %#v, want composed active Change result", active)
	}
	if err := result.validate(); err != nil {
		t.Fatalf("available result validation error = %v", err)
	}
}

func TestJournalHookContextResultInvariantRejectsInvalidCombinations(t *testing.T) {
	invalid := []journalHookContextResult{
		{disposition: journalHookContextModelAvailable},
		{disposition: journalHookContextWarning},
		{disposition: journalHookContextWarning, warning: &journalHookContextWarningResult{Code: "code", Message: "message"}},
		{disposition: journalHookContextSuppressed, warning: &journalHookContextWarningResult{Code: "code", Message: "message", NonBlocking: true}},
	}
	for index, result := range invalid {
		if err := result.validate(); err == nil {
			t.Errorf("invalid result %d validated successfully: %#v", index, result)
		}
	}
	available, err := newJournalHookContextAvailable(journalContextCLIResult{})
	if err != nil {
		t.Fatalf("new available result error = %v", err)
	}
	if err := available.validate(); err != nil {
		t.Fatalf("new available result validation error = %v", err)
	}
}

func TestJournalContextHookOutputRegressionRemainsHumanAndJSONCompatible(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	logRunner := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRunner.Run([]string{"journal", "log", "wrap(hooks): checkpoint one"}); err != nil {
		t.Fatal(err)
	}
	var human bytes.Buffer
	if err := (Runner{Stdout: &human, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"session_id":"primary"}`)}).Run([]string{"journal", "context", "--from-hook"}); err != nil {
		t.Fatalf("human hook context error = %v", err)
	}
	for _, want := range []string{"loaf journal context", "project-synthesis: none", "scoped-checkpoint: showing 1 of 1", "latest checkpoint (not project synthesis)", "wrap(hooks): checkpoint one"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("human hook context = %q, want %q", human.String(), want)
		}
	}
	var machine bytes.Buffer
	if err := (Runner{Stdout: &machine, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"session_id":"primary"}`)}).Run([]string{"journal", "context", "--from-hook", "--json"}); err != nil {
		t.Fatalf("JSON hook context error = %v", err)
	}
	if !strings.Contains(machine.String(), `"contract_version": 2`) || !strings.Contains(machine.String(), `"project_synthesis"`) || !strings.Contains(machine.String(), `"scoped_checkpoint"`) {
		t.Fatalf("JSON hook context = %q, want existing contract-v2 layers", machine.String())
	}
}

func TestJournalContextOrdinaryOutputRegressionRemainsHumanAndJSONCompatible(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	logRunner := Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}
	if err := logRunner.Run([]string{"journal", "log", "wrap(project): ordinary checkpoint"}); err != nil {
		t.Fatal(err)
	}
	var human bytes.Buffer
	if err := (Runner{Stdout: &human, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "context"}); err != nil {
		t.Fatalf("ordinary human context error = %v", err)
	}
	for _, want := range []string{"loaf journal context", "project-synthesis: showing 1 of 1", "wrap(project): ordinary checkpoint"} {
		if !strings.Contains(human.String(), want) {
			t.Fatalf("ordinary human context = %q, want %q", human.String(), want)
		}
	}
	var machine bytes.Buffer
	if err := (Runner{Stdout: &machine, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "context", "--json"}); err != nil {
		t.Fatalf("ordinary JSON context error = %v", err)
	}
	if !strings.Contains(machine.String(), `"contract_version": 2`) || !strings.Contains(machine.String(), `"project_synthesis"`) {
		t.Fatalf("ordinary JSON context = %q, want existing contract-v2 synthesis", machine.String())
	}
}

func TestJournalContextHookMissingStateOutputRemainsNonBlocking(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	var human bytes.Buffer
	if err := (Runner{Stdout: &human, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"session_id":"primary"}`)}).Run([]string{"journal", "context", "--from-hook"}); err != nil {
		t.Fatalf("missing-state hook context error = %v", err)
	}
	if !strings.Contains(human.String(), "no journal yet") || strings.Contains(human.String(), "SQLite state database is not initialized") {
		t.Fatalf("missing-state human output = %q, want nonblocking diagnostic", human.String())
	}
	var machine bytes.Buffer
	if err := (Runner{Stdout: &machine, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(`{"session_id":"primary"}`)}).Run([]string{"journal", "context", "--from-hook", "--json"}); err != nil {
		t.Fatalf("missing-state JSON hook context error = %v", err)
	}
	if machine.Len() != 0 {
		t.Fatalf("missing-state JSON output = %q, want unchanged empty output", machine.String())
	}
	if entries, err := os.ReadDir(stateHome); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("missing-state hook created state entries: %v", entries)
	}
}

func TestJournalHookContextResultWarningConstructorRequiresStructure(t *testing.T) {
	result, err := newJournalHookContextWarning(journalHookDiagnosticMissingState, "not initialized")
	if err != nil {
		t.Fatalf("warning constructor error = %v", err)
	}
	if result.disposition != journalHookContextWarning || result.warning == nil || result.warning.NonBlocking != true {
		t.Fatalf("warning result = %#v, want nonblocking warning", result)
	}
}
