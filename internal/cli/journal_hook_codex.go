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

// Codex consumes SessionStart command output as a target-native JSON
// envelope. Keep this renderer separate from the neutral journal context
// result and from the other harness adapters.
type codexSessionStartOutput struct {
	HookSpecificOutput *codexHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string                   `json:"systemMessage,omitempty"`
}

type codexHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

const (
	codexSessionStartEvent         = "SessionStart"
	codexSessionStartSystemMessage = "Loaf journal state is not initialized; continuity context is unavailable."
)

var (
	codexSessionStartSources = map[string]bool{
		"startup": true,
		"resume":  true,
		"clear":   true,
		"compact": true,
	}
	codexSessionStartPermissionModes = map[string]bool{
		"default":           true,
		"acceptEdits":       true,
		"plan":              true,
		"dontAsk":           true,
		"bypassPermissions": true,
	}
	codexSessionStartInputFields = map[string]bool{
		"cwd":             true,
		"hook_event_name": true,
		"model":           true,
		"permission_mode": true,
		"session_id":      true,
		"source":          true,
		"transcript_path": true,
	}
)

func (r Runner) runCodexSessionStartContext(out io.Writer, runtime state.Runtime, options journalContextCLIOptions) error {
	if !options.fromHook {
		return errors.New("Codex SessionStart context requires --from-hook")
	}
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	source, err := validateCodexSessionStartInput(hookInput)
	if err != nil {
		return err
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
		return writeJSON(out, codexSessionStartOutput{SystemMessage: codexSessionStartSystemMessage})
	case journalHookContextModelAvailable:
		if result.context == nil {
			return errors.New("Codex SessionStart context result is missing its neutral context")
		}
		additionalContext := renderCodexSessionStartContext(*result.context)
		if additionalContext == "" {
			return errors.New("Codex SessionStart context renderer produced an empty digest")
		}
		return writeJSON(out, codexSessionStartOutput{
			HookSpecificOutput: &codexHookSpecificOutput{
				HookEventName:     codexSessionStartEvent,
				AdditionalContext: additionalContext,
			},
		})
	default:
		return fmt.Errorf("Codex SessionStart source %q returned invalid context disposition %q", source, result.disposition)
	}
}

// validateCodexSessionStartInput mirrors the exact codex-cli 0.144.1
// session-start.command.input schema. The adapter deliberately rejects
// aliases, unknown fields, wrong types, and trimmed or mixed-case values.
func validateCodexSessionStartInput(input journalHookInput) (string, error) {
	if len(input.Raw) == 0 {
		return "", errors.New("Codex SessionStart hook input is missing or malformed")
	}
	for field := range input.Raw {
		if !codexSessionStartInputFields[field] {
			return "", fmt.Errorf("Codex SessionStart hook input contains unsupported field %q", field)
		}
	}
	if value, ok := input.Raw["cwd"]; !ok {
		return "", errors.New("Codex SessionStart hook input is missing cwd")
	} else if _, ok := value.(string); !ok {
		return "", errors.New("Codex SessionStart cwd must be a string")
	}
	if value, ok := input.Raw["hook_event_name"]; !ok {
		return "", errors.New("Codex SessionStart hook input is missing hook_event_name")
	} else if event, ok := value.(string); !ok || event != codexSessionStartEvent {
		return "", fmt.Errorf("Codex hook event %v is not the exact SessionStart event", value)
	}
	if value, ok := input.Raw["model"]; !ok {
		return "", errors.New("Codex SessionStart hook input is missing model")
	} else if _, ok := value.(string); !ok {
		return "", errors.New("Codex SessionStart model must be a string")
	}
	if value, ok := input.Raw["permission_mode"]; !ok {
		return "", errors.New("Codex SessionStart hook input is missing permission_mode")
	} else if mode, ok := value.(string); !ok || !codexSessionStartPermissionModes[mode] {
		return "", fmt.Errorf("Codex SessionStart permission_mode %v is unsupported", value)
	}
	if value, ok := input.Raw["session_id"]; !ok {
		return "", errors.New("Codex SessionStart hook input is missing session_id")
	} else if _, ok := value.(string); !ok {
		return "", errors.New("Codex SessionStart session_id must be a string")
	}
	value, ok := input.Raw["source"]
	if !ok {
		return "", errors.New("Codex SessionStart hook input is missing source")
	}
	source, ok := value.(string)
	if !ok || !codexSessionStartSources[source] {
		return "", fmt.Errorf("Codex SessionStart source %v is unsupported", value)
	}
	if value, ok := input.Raw["transcript_path"]; !ok {
		return "", errors.New("Codex SessionStart hook input is missing transcript_path")
	} else if value != nil {
		if _, ok := value.(string); !ok {
			return "", errors.New("Codex SessionStart transcript_path must be a string or null")
		}
	}
	return source, nil
}

func renderCodexSessionStartContext(result journalContextCLIResult) string {
	var out strings.Builder
	writeJournalContextHuman(&out, result)
	return strings.TrimSpace(out.String())
}
