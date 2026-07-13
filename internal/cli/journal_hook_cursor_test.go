package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func runCursorSessionStartFixture(t *testing.T, workingDir, stateHome, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := (Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(input)}).Run([]string{"journal", "context", "--from-hook", "--cursor-hook"})
	return out.String(), err
}

func TestCursorSessionStartReturnsNativeAdditionalContext(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(cursor): continuity marker"}); err != nil {
		t.Fatal(err)
	}
	output, err := runCursorSessionStartFixture(t, workingDir, stateHome, `{"hook_event_name":"sessionStart","session_id":"s1","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc","workspace_roots":[]}`)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload cursorSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if payload.AdditionalContext == "" || !strings.Contains(payload.AdditionalContext, "wrap(cursor): continuity marker") {
		t.Fatalf("additional_context = %q, want complete digest", payload.AdditionalContext)
	}
}

func TestCursorSessionStartAllowsOmittedComposerModeForKnownIdentities(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(cursor): omitted composer mode"}); err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]string{
		"cursor-agent": "2026.05.09-0afadcc",
		"cursor-ide":   "3.11.19",
	} {
		t.Run(name, func(t *testing.T) {
			input := `{"hook_event_name":"sessionStart","is_background_agent":false,"cursor_version":"` + version + `"}`
			output, err := runCursorSessionStartFixture(t, workingDir, stateHome, input)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			var payload cursorSessionStartOutput
			if err := json.Unmarshal([]byte(output), &payload); err != nil {
				t.Fatalf("output = %q: %v", output, err)
			}
			if !strings.Contains(payload.AdditionalContext, "wrap(cursor): omitted composer mode") {
				t.Fatalf("additional_context = %q, want complete digest", payload.AdditionalContext)
			}
		})
	}
}

func TestCursorSessionStartIncludesActiveChangesAndCannotSelectThemAway(t *testing.T) {
	repo, changeFile, _ := committedOriginFixture(t, "cursor-active-change", "20260713")
	if err := os.WriteFile(changeFile, []byte("---\nslug: cursor-active-change\nlineage: cursor-line\n---\nworking bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateHome := t.TempDir()
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: repo, StateHome: stateHome}).Run([]string{"state", "init", "--json"}); err != nil {
		t.Fatal(err)
	}
	output, err := runCursorSessionStartFixture(t, repo, stateHome, `{"hook_event_name":"sessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload cursorSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if !strings.Contains(payload.AdditionalContext, "active-changes: showing 1 of 1") || !strings.Contains(payload.AdditionalContext, "cursor-active-change") {
		t.Fatalf("additional_context = %q, want complete active Change layer", payload.AdditionalContext)
	}
	for _, args := range [][]string{
		{"journal", "context", "--from-hook", "--cursor-hook", "--branch", "main"},
		{"journal", "context", "--from-hook", "--cursor-hook", "--layer", "active-changes"},
		{"journal", "context", "--from-hook", "--cursor-hook", "--limit", "1"},
		{"journal", "context", "--from-hook", "--cursor-hook", "--cursor", "next"},
		{"journal", "context", "--from-hook", "--cursor-hook", "--json"},
	} {
		var selected bytes.Buffer
		if err := (Runner{Stdout: &selected, WorkingDir: repo, StateHome: stateHome}).Run(args); err == nil || strings.Contains(selected.String(), `"additional_context"`) {
			t.Fatalf("args=%v error=%v output=%q, want rejection before native context", args, err, selected.String())
		}
	}
}

func TestCursorSessionStartSuppressesNativeBackgroundPayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, input := range map[string]string{
		"background": `{"hook_event_name":"sessionStart","composer_mode":"agent","cursor_version":"2026.05.09-0afadcc","is_background_agent":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runCursorSessionStartFixture(t, workingDir, stateHome, input)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if output != "" {
				t.Fatalf("output = %q, want silent suppression", output)
			}
		})
	}
}

func TestCursorSessionStartMissingStateReturnsModelVisibleWarningWithoutMutation(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	before, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	output, err := runCursorSessionStartFixture(t, workingDir, stateHome, `{"hook_event_name":"sessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	var payload cursorSessionStartOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("output = %q: %v", output, err)
	}
	if payload.AdditionalContext != cursorSessionStartWarning {
		t.Fatalf("additional_context = %q, want model-visible warning", payload.AdditionalContext)
	}
	after, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("state home changed: before=%v after=%v", before, after)
	}
}

func TestCursorSessionStartRejectsNonNativeInputs(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, input := range map[string]string{
		"malformed":              `{not-json`,
		"missing-event":          `{"composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"wrong-event":            `{"hook_event_name":"SessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"event-alias":            `{"event":"sessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"event-whitespace":       `{"hook_event_name":" sessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"event-mixed-case":       `{"hook_event_name":"SessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"background-alias":       `{"hook_event_name":"sessionStart","composer_mode":"agent","is_background":true,"is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"background-string":      `{"hook_event_name":"sessionStart","composer_mode":"agent","is_background_agent":"true","cursor_version":"2026.05.09-0afadcc"}`,
		"mode-string":            `{"hook_event_name":"sessionStart","composer_mode":true,"is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"mode-alias":             `{"hook_event_name":"sessionStart","composerMode":"agent","is_background_agent":false,"cursor_version":"2026.05.09-0afadcc"}`,
		"cursor-version-alias":   `{"hook_event_name":"sessionStart","cursorVersion":"2026.05.09-0afadcc","is_background_agent":false}`,
		"event-camel-alias":      `{"hookEventName":"sessionStart","is_background_agent":false}`,
		"background-camel-alias": `{"hook_event_name":"sessionStart","isBackgroundAgent":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runCursorSessionStartFixture(t, workingDir, stateHome, input)
			if err == nil {
				t.Fatalf("error = nil, output = %q", output)
			}
			if output != "" {
				t.Fatalf("output = %q, want no trusted native payload", output)
			}
		})
	}
}

func TestCursorSessionStartRejectsMalformedPresentVersions(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, version := range map[string]string{
		"number":     `123`,
		"null":       `null`,
		"blank":      `""`,
		"whitespace": `"  "`,
		"padded":     `" 2026.05.09-0afadcc "`,
	} {
		t.Run(name, func(t *testing.T) {
			input := `{"hook_event_name":"sessionStart","is_background_agent":false,"cursor_version":` + version + `}`
			output, err := runCursorSessionStartFixture(t, workingDir, stateHome, input)
			if err == nil {
				t.Fatalf("error = nil, output = %q", output)
			}
			if output != "" {
				t.Fatalf("output = %q, want no trusted native payload", output)
			}
		})
	}
}

func TestCursorSessionStartUnknownOrMissingVersionReturnsExplicitFallback(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, input := range map[string]string{
		"missing": `{"hook_event_name":"sessionStart","composer_mode":"agent","is_background_agent":false}`,
		"unknown": `{"hook_event_name":"sessionStart","composer_mode":"agent","is_background_agent":false,"cursor_version":"9.9.9"}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runCursorSessionStartFixture(t, workingDir, stateHome, input)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			var payload cursorSessionStartOutput
			if err := json.Unmarshal([]byte(output), &payload); err != nil {
				t.Fatalf("output = %q: %v", output, err)
			}
			if payload.AdditionalContext != cursorSessionStartVersionWarning {
				t.Fatalf("additional_context = %q, want explicit version fallback", payload.AdditionalContext)
			}
		})
	}
}

func TestNativeCursorLifecycleContainsOnlyProvenJournalContextWiring(t *testing.T) {
	hooks, err := readNativeBuildHooks("../../config/hooks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	payload := nativeCursorHooksJSON{Version: 1}
	for _, hook := range hooks {
		if nativeCursorJournalContextHookOmitted(hook.id) {
			continue
		}
		if hook.section == "session" && nativeCursorSessionEvent(hook.event) == "sessionStart" {
			entry := nativeCursorHookEntry(hook, 60000, false)
			if hook.id == "session-start-loaf" {
				entry.Command = "loaf journal context --from-hook --cursor-hook"
			}
			payload.Hooks.SessionStart = append(payload.Hooks.SessionStart, entry)
		}
	}
	if len(payload.Hooks.SessionStart) != 1 || payload.Hooks.SessionStart[0].Command != "loaf journal context --from-hook --cursor-hook" {
		t.Fatalf("sessionStart = %#v, want one Cursor adapter", payload.Hooks.SessionStart)
	}
	if payload.Hooks.BeforeSubmitPrompt != nil || payload.Hooks.PreCompact != nil || payload.Hooks.Stop != nil || payload.Hooks.SessionEnd != nil {
		t.Fatalf("unsupported journal lifecycle hooks rendered: %#v", payload.Hooks)
	}
}
