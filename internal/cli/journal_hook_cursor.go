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

// Cursor consumes sessionStart command-hook output as a native
// additional_context envelope. Keep this renderer separate from the neutral
// journal context result and from the Claude adapter.
type cursorSessionStartOutput struct {
	AdditionalContext string `json:"additional_context,omitempty"`
}

const (
	cursorSessionStartEvent          = "sessionStart"
	cursorSessionStartWarning        = "Loaf journal state is not initialized; continuity context is unavailable."
	cursorSessionStartVersionWarning = "Loaf continuity is unavailable for this Cursor version; run `loaf journal context` explicitly."
	cursorBackgroundAgentField       = "is_background_agent"
	cursorComposerModeField          = "composer_mode"
)

var cursorSessionStartVersions = map[string]bool{
	"3.11.19":            true,
	"2026.05.09-0afadcc": true,
}

type cursorSessionStartValidation struct {
	versionSupported bool
}

// runCursorSessionStartContext implements only the installed Cursor
// sessionStart command-hook contract. It always renders the complete neutral
// digest; selectors and output overrides are rejected by the generic parser.
func (r Runner) runCursorSessionStartContext(out io.Writer, runtime state.Runtime, options journalContextCLIOptions) error {
	if !options.fromHook {
		return errors.New("Cursor sessionStart context requires --from-hook")
	}
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	validation, err := validateCursorSessionStartInput(hookInput)
	if err != nil {
		return err
	}
	if cursorSessionStartSuppressed(hookInput) {
		return nil
	}
	if !validation.versionSupported {
		return writeJSON(out, cursorSessionStartOutput{AdditionalContext: cursorSessionStartVersionWarning})
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
		return writeJSON(out, cursorSessionStartOutput{AdditionalContext: cursorSessionStartWarning})
	case journalHookContextModelAvailable:
		if result.context == nil {
			return errors.New("Cursor sessionStart context result is missing its neutral context")
		}
		additionalContext := renderCursorSessionStartContext(*result.context)
		if additionalContext == "" {
			return errors.New("Cursor sessionStart context renderer produced an empty digest")
		}
		return writeJSON(out, cursorSessionStartOutput{AdditionalContext: additionalContext})
	default:
		return fmt.Errorf("Cursor sessionStart returned invalid context disposition %q", result.disposition)
	}
}

// validateCursorSessionStartInput checks exact native field names and values
// that affect routing. Other native fields are intentionally ignored because
// Cursor adds runtime metadata (model, generation, workspace roots, and
// version) that must not become Loaf's persisted surface.
func validateCursorSessionStartInput(input journalHookInput) (cursorSessionStartValidation, error) {
	if len(input.Raw) == 0 {
		return cursorSessionStartValidation{}, errors.New("Cursor sessionStart hook input is missing or malformed")
	}
	event, ok := input.Raw["hook_event_name"].(string)
	if !ok || event == "" {
		return cursorSessionStartValidation{}, errors.New("Cursor sessionStart hook input is missing hook_event_name")
	}
	if event != cursorSessionStartEvent {
		return cursorSessionStartValidation{}, fmt.Errorf("Cursor hook event %q is not a supported sessionStart event", event)
	}
	for _, alias := range []string{"event", "is_background", "composerMode"} {
		if _, exists := input.Raw[alias]; exists {
			return cursorSessionStartValidation{}, fmt.Errorf("Cursor sessionStart hook input uses unsupported field alias %q", alias)
		}
	}
	for _, alias := range []string{"cursorVersion", "hookEventName", "isBackgroundAgent"} {
		if _, exists := input.Raw[alias]; exists {
			return cursorSessionStartValidation{}, fmt.Errorf("Cursor sessionStart hook input uses unsupported field alias %q", alias)
		}
	}
	background, exists := input.Raw[cursorBackgroundAgentField]
	if !exists {
		return cursorSessionStartValidation{}, fmt.Errorf("Cursor sessionStart hook input is missing %s", cursorBackgroundAgentField)
	}
	if _, ok := background.(bool); !ok {
		return cursorSessionStartValidation{}, fmt.Errorf("Cursor sessionStart %s must be a boolean", cursorBackgroundAgentField)
	}
	if value, exists := input.Raw[cursorComposerModeField]; exists {
		mode, ok := value.(string)
		if !ok {
			return cursorSessionStartValidation{}, fmt.Errorf("Cursor sessionStart %s must be a string", cursorComposerModeField)
		}
		if strings.TrimSpace(mode) == "" {
			return cursorSessionStartValidation{}, fmt.Errorf("Cursor sessionStart %s must be nonblank", cursorComposerModeField)
		}
	}
	versionValue, exists := input.Raw["cursor_version"]
	if !exists {
		return cursorSessionStartValidation{}, nil
	}
	version, ok := versionValue.(string)
	if !ok {
		return cursorSessionStartValidation{}, errors.New("Cursor sessionStart cursor_version must be a string")
	}
	if strings.TrimSpace(version) == "" || strings.TrimSpace(version) != version {
		return cursorSessionStartValidation{}, errors.New("Cursor sessionStart cursor_version must be a nonblank exact version")
	}
	return cursorSessionStartValidation{versionSupported: cursorSessionStartVersions[version]}, nil
}

func cursorSessionStartSuppressed(input journalHookInput) bool {
	if background, ok := input.Raw[cursorBackgroundAgentField].(bool); ok && background {
		return true
	}
	return normalizeJournalHookEnvelope(input, "").suppressesContext()
}

func renderCursorSessionStartContext(result journalContextCLIResult) string {
	var out strings.Builder
	writeJournalContextHuman(&out, result)
	return strings.TrimSpace(out.String())
}
