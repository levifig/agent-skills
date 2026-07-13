package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// Claude Code consumes SessionStart hook output as a target-native JSON
// envelope. Keep this renderer deliberately separate from the neutral journal
// context result so other harnesses cannot accidentally inherit Claude's
// payload contract.
type claudeSessionStartOutput struct {
	HookSpecificOutput *claudeHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string                    `json:"systemMessage,omitempty"`
}

type claudeHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

const (
	claudeSessionStartEvent         = "SessionStart"
	claudeSessionStartSystemMessage = "Loaf journal state is not initialized; continuity context is unavailable."
)

var claudeSessionStartSources = map[string]bool{
	"startup": true,
	"resume":  true,
	"clear":   true,
	"compact": true,
}

func (r Runner) runClaudeSessionStartContext(out io.Writer, runtime state.Runtime, options journalContextCLIOptions) error {
	if !options.fromHook {
		return errors.New("Claude Code SessionStart context requires --from-hook")
	}
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	source, err := validateClaudeSessionStartInput(hookInput)
	if err != nil {
		return err
	}
	if claudeSessionStartSuppressed(hookInput) {
		return nil
	}
	root, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	result, err := r.evaluateJournalHookContext(context.Background(), runtime, root, options, &hookInput, true)
	if err != nil {
		return err
	}
	switch result.disposition {
	case journalHookContextSuppressed:
		return nil
	case journalHookContextWarning:
		return writeJSON(out, claudeSessionStartOutput{SystemMessage: claudeSessionStartSystemMessage})
	case journalHookContextModelAvailable:
		if result.context == nil {
			return errors.New("Claude Code SessionStart context result is missing its neutral context")
		}
		additionalContext := renderClaudeSessionStartContext(*result.context)
		if additionalContext == "" {
			return errors.New("Claude Code SessionStart context renderer produced an empty digest")
		}
		return writeJSON(out, claudeSessionStartOutput{
			HookSpecificOutput: &claudeHookSpecificOutput{
				HookEventName:     claudeSessionStartEvent,
				AdditionalContext: additionalContext,
			},
		})
	default:
		return fmt.Errorf("Claude Code SessionStart source %q returned invalid context disposition %q", source, result.disposition)
	}
}

func validateClaudeSessionStartInput(input journalHookInput) (string, error) {
	if len(input.Raw) == 0 {
		return "", errors.New("Claude Code SessionStart hook input is missing or malformed")
	}
	eventValue, eventOK := input.Raw["hook_event_name"].(string)
	if !eventOK || eventValue == "" {
		return "", errors.New("Claude Code SessionStart hook input is missing hook_event_name")
	}
	if eventValue != claudeSessionStartEvent {
		return "", fmt.Errorf("Claude Code hook event %q is not a supported SessionStart event", eventValue)
	}
	source, sourceOK := input.Raw["source"].(string)
	if !sourceOK {
		source = ""
	}
	if !claudeSessionStartSources[source] {
		if source == "" {
			return "", errors.New("Claude Code SessionStart hook input is missing a supported source")
		}
		return "", fmt.Errorf("Claude Code SessionStart source %q is unsupported", source)
	}
	return source, nil
}

func claudeSessionStartSuppressed(input journalHookInput) bool {
	envelope := normalizeJournalHookEnvelope(input, "")
	if envelope.suppressesContext() {
		return true
	}
	agentType := strings.ToLower(strings.TrimSpace(firstMapString(input.Raw, input.Raw, "agent_type", "agent_mode", "mode")))
	return strings.Contains(agentType, "child") || strings.Contains(agentType, "background")
}

func renderClaudeSessionStartContext(result journalContextCLIResult) string {
	var out strings.Builder
	writeJournalContextHuman(&out, result)
	return strings.TrimSpace(out.String())
}
