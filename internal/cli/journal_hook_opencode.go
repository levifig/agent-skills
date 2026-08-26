package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// OpenCode consumes session-hook stdout as plain text: the generated plugin
// pushes the trimmed output straight onto the system-prompt array, so this
// adapter emits the digest itself rather than a JSON envelope. It exists as its
// own dispatch variant for the same reason the others do — the variant is what
// names the config dir whose `.loaf-version` marker describes the content this
// conversation is running on.
const (
	openCodeSessionStartTarget  = "opencode"
	openCodeSessionStartEvent   = "system.transform"
	openCodeSessionStartWarning = "Loaf journal state is not initialized; continuity context is unavailable."
)

func (r Runner) runOpenCodeSessionStartContext(out io.Writer, runtime state.Runtime, options journalContextCLIOptions) error {
	if err := r.enforceSessionStartAttach(r.Stderr, options.jsonOutput); err != nil {
		return err
	}
	if !options.fromHook {
		return errors.New("OpenCode session start context requires --from-hook")
	}
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	if err := validateOpenCodeSessionStartInput(hookInput); err != nil {
		return err
	}
	if normalizeJournalHookEnvelope(hookInput, "").suppressesContext() {
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
		fmt.Fprintln(out, openCodeSessionStartWarning)
		return nil
	case journalHookContextModelAvailable:
		if result.context == nil {
			return errors.New("OpenCode session start context result is missing its neutral context")
		}
		digest := r.renderSessionStartDigest(*result.context, harnessDriftOpenCode)
		if digest == "" {
			return errors.New("OpenCode session start context renderer produced an empty digest")
		}
		fmt.Fprintln(out, digest)
		return nil
	default:
		return fmt.Errorf("OpenCode session start returned invalid context disposition %q", result.disposition)
	}
}

// validateOpenCodeSessionStartInput checks the payload the generated OpenCode
// plugin writes. Unlike the Cursor and Codex adapters it tolerates unknown
// fields: this payload is Loaf-authored on both ends, so an unrecognized field
// means a plugin newer than the binary, and the right answer to that skew is
// still a digest rather than a hard failure.
func validateOpenCodeSessionStartInput(input journalHookInput) error {
	if len(input.Raw) == 0 {
		return errors.New("OpenCode session start hook input is missing or malformed")
	}
	target, ok := input.Raw["target"].(string)
	if !ok || target != openCodeSessionStartTarget {
		return fmt.Errorf("OpenCode session start target %v is not the OpenCode lifecycle payload", input.Raw["target"])
	}
	event, ok := input.Raw["lifecycle_event"].(string)
	if !ok || event != openCodeSessionStartEvent {
		return fmt.Errorf("OpenCode lifecycle event %v is not the session start event", input.Raw["lifecycle_event"])
	}
	sessionID, ok := input.Raw["session_id"].(string)
	if !ok || sessionID == "" {
		return errors.New("OpenCode session start hook input is missing session_id")
	}
	return nil
}
