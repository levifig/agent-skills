package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func codexSessionStartInput(source string) string {
	return fmt.Sprintf(`{"cwd":"/workspace/project","hook_event_name":"SessionStart","model":"gpt-5.6","permission_mode":"default","session_id":"codex-session","source":%q,"transcript_path":null}`, source)
}

func runCodexSessionStartFixture(t *testing.T, workingDir, stateHome, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := (Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(input)}).Run([]string{"journal", "context", "--from-hook", "--codex-hook"})
	return out.String(), err
}

func TestCodexSessionStartSourcesRenderIdenticalNativePayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(codex): continuity marker"}); err != nil {
		t.Fatal(err)
	}
	var first string
	for _, source := range []string{"startup", "resume", "clear", "compact"} {
		output, err := runCodexSessionStartFixture(t, workingDir, stateHome, codexSessionStartInput(source))
		if err != nil {
			t.Fatalf("source %s error = %v", source, err)
		}
		var payload codexSessionStartOutput
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			t.Fatalf("source %s output = %q: %v", source, output, err)
		}
		if payload.HookSpecificOutput == nil || payload.HookSpecificOutput.HookEventName != codexSessionStartEvent {
			t.Fatalf("source %s payload = %#v, want SessionStart hookSpecificOutput", source, payload)
		}
		if !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "wrap(codex): continuity marker") {
			t.Fatalf("source %s additionalContext = %q, want complete digest", source, payload.HookSpecificOutput.AdditionalContext)
		}
		if first == "" {
			first = payload.HookSpecificOutput.AdditionalContext
		} else if payload.HookSpecificOutput.AdditionalContext != first {
			t.Fatalf("source %s additionalContext differs from startup", source)
		}
	}
}

func TestCodexSessionStartIncludesCompleteActiveChangeTruth(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "codex-active-change", "20260713")
	if err := os.WriteFile(changeFile, []byte("---\nslug: codex-active-change\nlineage: codex-line\n---\nworking bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatal(err)
	}
	output, err := runCodexSessionStartFixture(t, repo, stateHome, codexSessionStartInput("startup"))
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload codexSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if payload.HookSpecificOutput == nil || !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "active-changes: showing 1 of 1") || !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "codex-active-change") {
		t.Fatalf("additionalContext = %q, want complete active Change layer", payload.HookSpecificOutput.AdditionalContext)
	}
}

func TestCodexSessionStartMissingStateUsesSystemMessageWithoutMutation(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	before, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	output, err := runCodexSessionStartFixture(t, workingDir, stateHome, codexSessionStartInput("startup"))
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload codexSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if payload.SystemMessage != codexSessionStartSystemMessage || payload.HookSpecificOutput != nil {
		t.Fatalf("payload = %#v, want nonblocking systemMessage only", payload)
	}
	after, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("state home changed: before=%v after=%v", before, after)
	}
}

func TestCodexSessionStartRejectsSchemaDriftAndAliases(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	valid := codexSessionStartInput("startup")
	for name, input := range map[string]string{
		"missing-required":  `{"hook_event_name":"SessionStart","source":"startup"}`,
		"unknown-field":     strings.TrimSuffix(valid, "}") + `,"agent_id":"child"}`,
		"event-alias":       strings.Replace(valid, `"hook_event_name":"SessionStart"`, `"event":"SessionStart"`, 1),
		"event-whitespace":  strings.Replace(valid, `"hook_event_name":"SessionStart"`, `"hook_event_name":" SessionStart"`, 1),
		"source-whitespace": strings.Replace(valid, `"source":"startup"`, `"source":" startup"`, 1),
		"source-mixed-case": strings.Replace(valid, `"source":"startup"`, `"source":"Startup"`, 1),
		"bad-permission":    strings.Replace(valid, `"permission_mode":"default"`, `"permission_mode":"DEFAULT"`, 1),
		"bad-transcript":    strings.Replace(valid, `"transcript_path":null`, `"transcript_path":false`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runCodexSessionStartFixture(t, workingDir, stateHome, input)
			if err == nil {
				t.Fatalf("error = nil, output = %q", output)
			}
			if output != "" {
				t.Fatalf("output = %q, want no trusted SessionStart payload", output)
			}
		})
	}
}

func TestCodexSessionStartRejectsSelectorsAndOutputOverrides(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for _, args := range [][]string{
		{"journal", "context", "for-prompt", "--codex-hook"},
		{"journal", "context", "for-compact", "--codex-hook"},
		{"journal", "context", "for-resumption", "--codex-hook"},
		{"journal", "context", "--from-hook", "--codex-hook", "--branch", "main"},
		{"journal", "context", "--from-hook", "--codex-hook", "--layer", "active-changes"},
		{"journal", "context", "--from-hook", "--codex-hook", "--limit", "1"},
		{"journal", "context", "--from-hook", "--codex-hook", "--cursor", "next"},
		{"journal", "context", "--from-hook", "--codex-hook", "--json"},
	} {
		var output bytes.Buffer
		err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run(args)
		if err == nil || bytes.Contains(output.Bytes(), []byte(`"hookSpecificOutput"`)) {
			t.Fatalf("args=%v error=%v output=%q, want rejection before emitting native context", args, err, output.String())
		}
	}
}
