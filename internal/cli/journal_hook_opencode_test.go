package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func runOpenCodeSessionStartFixture(t *testing.T, workingDir, stateHome, input string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := (Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(input)}).Run([]string{"journal", "context", "--from-hook", "--opencode-hook"})
	return out.String(), err
}

// openCodeSessionStartPayload is the exact document the generated OpenCode
// plugin writes to the hook's stdin (see serializeOpenCodeLifecyclePayload).
const openCodeSessionStartPayload = `{"target":"opencode","session_id":"ses_1","lifecycle_event":"system.transform"}`

func TestOpenCodeSessionStartEmitsThePlainTextDigest(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(opencode): continuity marker"}); err != nil {
		t.Fatal(err)
	}

	output, err := runOpenCodeSessionStartFixture(t, workingDir, stateHome, openCodeSessionStartPayload)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(output, "wrap(opencode): continuity marker") {
		t.Fatalf("output = %q, want the complete digest", output)
	}
	// OpenCode pushes stdout onto the system-prompt array verbatim; a JSON
	// envelope would arrive at the model as literal JSON.
	if strings.HasPrefix(strings.TrimSpace(output), "{") {
		t.Fatalf("output = %q, want plain text rather than a native envelope", output)
	}
}

func TestOpenCodeSessionStartSuppressesSubagentPayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	if err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: workingDir, StateHome: stateHome}).Run([]string{"journal", "log", "wrap(opencode): continuity marker"}); err != nil {
		t.Fatal(err)
	}

	for name, input := range map[string]string{
		"agent_id":   `{"target":"opencode","session_id":"ses_1","lifecycle_event":"system.transform","agent_id":"child-1"}`,
		"background": `{"target":"opencode","session_id":"ses_1","lifecycle_event":"system.transform","is_background":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runOpenCodeSessionStartFixture(t, workingDir, stateHome, input)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if output != "" {
				t.Fatalf("output = %q, want silent suppression", output)
			}
		})
	}
}

func TestOpenCodeSessionStartRejectsForeignPayloads(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for name, input := range map[string]string{
		"malformed":        `{not-json`,
		"wrong-target":     `{"target":"cursor","session_id":"ses_1","lifecycle_event":"system.transform"}`,
		"missing-target":   `{"session_id":"ses_1","lifecycle_event":"system.transform"}`,
		"wrong-lifecycle":  `{"target":"opencode","session_id":"ses_1","lifecycle_event":"session.compacting"}`,
		"missing-session":  `{"target":"opencode","lifecycle_event":"system.transform"}`,
		"session-not-text": `{"target":"opencode","session_id":7,"lifecycle_event":"system.transform"}`,
	} {
		t.Run(name, func(t *testing.T) {
			output, err := runOpenCodeSessionStartFixture(t, workingDir, stateHome, input)
			if err == nil {
				t.Fatalf("error = nil, output = %q", output)
			}
			if output != "" {
				t.Fatalf("output = %q, want no digest for a payload this adapter does not own", output)
			}
		})
	}
}

// TestOpenCodeSessionStartRejectsSelectorsAndOutputOverrides keeps the variant a
// single native surface: it always emits the complete digest.
func TestOpenCodeSessionStartRejectsSelectorsAndOutputOverrides(t *testing.T) {
	workingDir, stateHome := setupJournalHookRunner(t)
	for _, args := range [][]string{
		{"journal", "context", "--opencode-hook"},
		{"journal", "context", "--from-hook", "--opencode-hook", "--branch", "main"},
		{"journal", "context", "--from-hook", "--opencode-hook", "--layer", "active-changes"},
		{"journal", "context", "--from-hook", "--opencode-hook", "--json"},
		{"journal", "context", "--from-hook", "--opencode-hook", "--codex-hook"},
	} {
		var out bytes.Buffer
		err := (Runner{Stdout: &out, WorkingDir: workingDir, StateHome: stateHome, Stdin: strings.NewReader(openCodeSessionStartPayload)}).Run(args)
		if err == nil {
			t.Fatalf("args=%v error = nil, output = %q", args, out.String())
		}
	}
}

func TestOpenCodeSessionStartMissingStateWarnsWithoutMutation(t *testing.T) {
	workingDir, stateHome := freshHookRunnerDir(t)
	before, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}

	output, err := runOpenCodeSessionStartFixture(t, workingDir, stateHome, openCodeSessionStartPayload)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if strings.TrimSpace(output) != openCodeSessionStartWarning {
		t.Fatalf("output = %q, want the model-visible warning", output)
	}
	after, err := os.ReadDir(stateHome)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("state home changed: before=%v after=%v", before, after)
	}
}

// TestNativeOpenCodeSessionStartRendersItsOwnAdapter proves the wiring the
// digest depends on: the generated plugin must invoke OpenCode's variant, not
// the neutral command, or the nudge is unreachable no matter how it is built.
func TestNativeOpenCodeSessionStartRendersItsOwnAdapter(t *testing.T) {
	hooks, err := readNativeBuildHooks("../../config/hooks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	plugin := renderNativeOpenCodePlugin(hooks, "0.0.0-test")
	if !strings.Contains(plugin, "loaf journal context --from-hook --opencode-hook") {
		t.Fatalf("generated plugin does not invoke the OpenCode adapter:\n%s", plugin)
	}
	if strings.Contains(plugin, `"command": "loaf journal context --from-hook"`) {
		t.Fatalf("generated plugin still invokes the neutral command:\n%s", plugin)
	}
}
