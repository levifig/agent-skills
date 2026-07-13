package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func runClaudeSessionStartFixture(t *testing.T, workingDir, stateHome, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := (Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(input)}).Run([]string{"journal", "context", "--from-hook", "--claude-code"})
	return out.String(), err
}

func TestClaudeSessionStartSourcesRenderIdenticalNativePayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(claude): continuity marker"}); err != nil {
		t.Fatal(err)
	}
	var first string
	for _, source := range []string{"startup", "resume", "clear", "compact"} {
		output, err := runClaudeSessionStartFixture(t, workingDir, stateHome, `{"hook_event_name":"SessionStart","source":"`+source+`","session_id":"s1"}`)
		if err != nil {
			t.Fatalf("source %s error = %v", source, err)
		}
		var payload claudeSessionStartOutput
		if err := json.Unmarshal([]byte(output), &payload); err != nil {
			t.Fatalf("source %s output = %q: %v", source, output, err)
		}
		if payload.HookSpecificOutput == nil || payload.HookSpecificOutput.HookEventName != claudeSessionStartEvent {
			t.Fatalf("source %s payload = %#v, want SessionStart hookSpecificOutput", source, payload)
		}
		if !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "wrap(claude): continuity marker") {
			t.Fatalf("source %s additionalContext = %q, want complete digest", source, payload.HookSpecificOutput.AdditionalContext)
		}
		if first == "" {
			first = payload.HookSpecificOutput.AdditionalContext
		} else if payload.HookSpecificOutput.AdditionalContext != first {
			t.Fatalf("source %s additionalContext differs from startup", source)
		}
	}
}

func TestClaudeSessionStartSuppressesChildAndBackgroundPayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, input := range map[string]string{
		"agent_id":   `{"hook_event_name":"SessionStart","source":"startup","session_id":"s1","agent_id":"child-1"}`,
		"agent_type": `{"hook_event_name":"SessionStart","source":"resume","session_id":"s1","agent_type":"child"}`,
		"background": `{"hook_event_name":"SessionStart","source":"compact","session_id":"s1","agent_type":"background"}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runClaudeSessionStartFixture(t, workingDir, stateHome, input)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if output != "" {
				t.Fatalf("output = %q, want silent suppression", output)
			}
		})
	}
}

func TestClaudeSessionStartMissingStateUsesSystemMessageWithoutMutation(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	before, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	output, err := runClaudeSessionStartFixture(t, workingDir, stateHome, `{"hook_event_name":"SessionStart","source":"startup","session_id":"s1"}`)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload claudeSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if payload.SystemMessage != claudeSessionStartSystemMessage || payload.HookSpecificOutput != nil {
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

func TestClaudeSessionStartAdditionalContextIncludesActiveChanges(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "claude-active-change", "20260713")
	if err := os.WriteFile(changeFile, []byte("---\nslug: claude-active-change\nlineage: claude-line\n---\nworking bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatal(err)
	}
	output, err := runClaudeSessionStartFixture(t, repo, stateHome, `{"hook_event_name":"SessionStart","source":"startup","session_id":"s1"}`)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload claudeSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if payload.HookSpecificOutput == nil || !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "active-changes: showing 1 of 1") || !strings.Contains(payload.HookSpecificOutput.AdditionalContext, "claude-active-change") {
		t.Fatalf("additionalContext = %q, want full active Change layer", payload.HookSpecificOutput.AdditionalContext)
	}
}

func TestClaudeSessionStartRejectsMalformedAndWrongEvents(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, input := range map[string]string{
		"malformed":         `{not-json`,
		"missing-event":     `{"source":"startup","session_id":"s1"}`,
		"wrong-event":       `{"hook_event_name":"UserPromptSubmit","source":"startup","session_id":"s1"}`,
		"wrong-source":      `{"hook_event_name":"SessionStart","source":"unknown","session_id":"s1"}`,
		"event-alias":       `{"event":"SessionStart","source":"startup","session_id":"s1"}`,
		"event-whitespace":  `{"hook_event_name":" SessionStart","source":"startup","session_id":"s1"}`,
		"source-alias":      `{"hook_event_name":"SessionStart","session_start_source":"startup","session_id":"s1"}`,
		"source-whitespace": `{"hook_event_name":"SessionStart","source":" startup","session_id":"s1"}`,
		"source-mixed-case": `{"hook_event_name":"SessionStart","source":"Startup","session_id":"s1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runClaudeSessionStartFixture(t, workingDir, stateHome, input)
			if err == nil {
				t.Fatalf("error = nil, output = %q", output)
			}
			if output != "" {
				t.Fatalf("output = %q, want no trusted SessionStart payload", output)
			}
		})
	}
}

func TestClaudeSessionStartRejectsSelectorsAndOutputOverrides(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for _, args := range [][]string{
		{"journal", "context", "for-prompt", "--claude-code"},
		{"journal", "context", "for-compact", "--claude-code"},
		{"journal", "context", "for-resumption", "--claude-code"},
		{"journal", "context", "--from-hook", "--claude-code", "--branch", "main"},
		{"journal", "context", "--from-hook", "--claude-code", "--layer", "active-changes"},
		{"journal", "context", "--from-hook", "--claude-code", "--limit", "1"},
		{"journal", "context", "--from-hook", "--claude-code", "--cursor", "next"},
		{"journal", "context", "--from-hook", "--claude-code", "--json"},
	} {
		var output bytes.Buffer
		err := (Runner{Stdout: &output, WorkingDir: workingDir, StateHome: stateHome}).Run(args)
		if err == nil || bytes.Contains(output.Bytes(), []byte(`"hookSpecificOutput"`)) {
			t.Fatalf("args=%v error=%v output=%q, want rejection before emitting native context", args, err, output.String())
		}
	}
}

func TestNativeClaudeLifecyclePayloadPinsBinaryAndConsolidatesCompaction(t *testing.T) {
	hooks, err := readNativeBuildHooks("../../config/hooks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	payload := nativeClaudeHooksPayload(hooks)
	if len(payload.Hooks.SessionStart) != 1 || payload.Hooks.SessionStart[0].Matcher != "startup|resume|clear|compact" {
		t.Fatalf("SessionStart groups = %#v, want one all-source matcher", payload.Hooks.SessionStart)
	}
	start, ok := payload.Hooks.SessionStart[0].Hooks[0].(nativeClaudeSessionCommandHookJSON)
	if !ok {
		t.Fatalf("SessionStart hook = %#v, want command hook", payload.Hooks.SessionStart[0].Hooks[0])
	}
	if !strings.Contains(start.Command, `${CLAUDE_PLUGIN_ROOT}/bin/loaf" journal context --from-hook --claude-code`) {
		t.Fatalf("SessionStart command = %q, want plugin-binary pinning and Claude adapter", start.Command)
	}
	if len(payload.Hooks.PreCompact) != 1 {
		t.Fatalf("PreCompact groups = %#v, want one guidance hook", payload.Hooks.PreCompact)
	}
	preCompact, ok := payload.Hooks.PreCompact[0].Hooks[0].(nativeClaudeSessionCommandHookJSON)
	if !ok || !strings.Contains(preCompact.Command, `${CLAUDE_PLUGIN_ROOT}/bin/loaf" journal context for-compact`) {
		t.Fatalf("PreCompact hook = %#v, want plugin-binary pinning", payload.Hooks.PreCompact[0].Hooks[0])
	}
	if len(payload.Hooks.PostCompact) != 0 {
		t.Fatalf("PostCompact groups = %#v, want no Claude continuity handler", payload.Hooks.PostCompact)
	}
	if len(payload.Hooks.TaskCompleted) != 0 || len(payload.Hooks.Stop) != 0 || len(payload.Hooks.SessionEnd) != 0 {
		t.Fatalf("automatic completion/lifecycle hooks unexpectedly rendered: task=%#v stop=%#v end=%#v", payload.Hooks.TaskCompleted, payload.Hooks.Stop, payload.Hooks.SessionEnd)
	}
}
