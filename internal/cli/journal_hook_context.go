package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// journalHookContextDisposition is the typed result of one adapter-neutral
// bounded journal context evaluation. It is a routing seam, not a target
// payload or renderer contract.
type journalHookContextDisposition string

const (
	journalHookContextModelAvailable journalHookContextDisposition = "model_context_available"
	journalHookContextWarning        journalHookContextDisposition = "nonblocking_warning"
	journalHookContextSuppressed     journalHookContextDisposition = "suppressed"
)

// journalHookContextWarningResult carries a structured, nonblocking diagnostic
// without requiring callers to infer its kind from rendered human output.
type journalHookContextWarningResult struct {
	Code        string
	Message     string
	NonBlocking bool
}

// journalHookContextResult is the final neutral continuity result consumed by
// target adapters in later slices. Its invariant is exactly one of: a full
// composed model context, a nonblocking warning, or suppression.
type journalHookContextResult struct {
	disposition journalHookContextDisposition
	context     *journalContextCLIResult
	warning     *journalHookContextWarningResult
}

func (result journalHookContextResult) validate() error {
	switch result.disposition {
	case journalHookContextModelAvailable:
		if result.context == nil || result.warning != nil {
			return errors.New("model context disposition requires context and forbids warning")
		}
	case journalHookContextWarning:
		if result.context != nil || result.warning == nil || !result.warning.NonBlocking {
			return errors.New("warning disposition requires a nonblocking warning and forbids context")
		}
		if result.warning.Code == "" || result.warning.Message == "" {
			return errors.New("warning disposition requires code and message")
		}
	case journalHookContextSuppressed:
		if result.context != nil || result.warning != nil {
			return errors.New("suppressed disposition forbids context and warning")
		}
	default:
		return fmt.Errorf("unknown hook context disposition %q", result.disposition)
	}
	return nil
}

func newJournalHookContextSuppressed() (journalHookContextResult, error) {
	result := journalHookContextResult{disposition: journalHookContextSuppressed}
	return result, result.validate()
}

func newJournalHookContextWarning(code, message string) (journalHookContextResult, error) {
	result := journalHookContextResult{
		disposition: journalHookContextWarning,
		warning:     &journalHookContextWarningResult{Code: code, Message: message, NonBlocking: true},
	}
	return result, result.validate()
}

func newJournalHookContextAvailable(contextResult journalContextCLIResult) (journalHookContextResult, error) {
	result := journalHookContextResult{disposition: journalHookContextModelAvailable, context: &contextResult}
	return result, result.validate()
}

// evaluateJournalHookContext owns normalization, root-only suppression, the
// bounded state query, active-Change composition, and the final neutral
// context shape. It never initializes state or renders target-specific JSON.
func (r Runner) evaluateJournalHookContext(ctx context.Context, runtime state.Runtime, root project.Root, options journalContextCLIOptions, hookInput *journalHookInput, hookInvocation bool) (journalHookContextResult, error) {
	if hookInvocation {
		if hookInput == nil {
			return journalHookContextResult{}, errors.New("hook context evaluation requires hook input")
		}
		envelope := normalizeJournalHookEnvelope(*hookInput, runtime.RootPath())
		if envelope.suppressesContext() {
			return newJournalHookContextSuppressed()
		}
	} else if hookInput != nil {
		return journalHookContextResult{}, errors.New("hook input supplied for non-hook context evaluation")
	}

	changeSource, changeSourceErr := discoverActiveChanges(root.Path())
	if !options.branchSet {
		if changeSourceErr == nil {
			options.branch = changeSource.Branch
		} else {
			options.branch = state.ObservedGitBranch(runtime.RootPath())
		}
	}
	stateOptions := state.JournalContextOptions{Branch: options.branch}
	if changeSourceErr == nil {
		stateOptions.LineageKeys = changeSource.LineageKeys
	}
	if options.layer != "" && options.limitSet {
		setJournalContextStateLimit(&stateOptions, options.layer, options.limit)
	}
	if options.cursor != "" && options.layer != journalContextLayerActiveChanges {
		stateOptions.Cursor = options.cursor
		stateOptions.CursorLayer = journalContextStateLayer(options.layer)
	}
	stateResult, err := state.JournalContextForRoot(ctx, root, state.PathResolver{StateHome: r.StateHome}, stateOptions)
	if err != nil {
		if hookInvocation && isStateMissingError(err) {
			return newJournalHookContextWarning(journalHookDiagnosticMissingState, "journal state is not initialized; context delivery was skipped")
		}
		return journalHookContextResult{}, err
	}

	activeLayer := unavailableActiveChanges()
	if changeSourceErr == nil {
		activeLimit := defaultCLIJournalContextLimit
		if options.layer == journalContextLayerActiveChanges && options.limitSet {
			activeLimit = options.limit
		}
		activeLayer, err = activeChangesPage(changeSource, stateResult.ProjectID, options.branch, activeLimit, func() string {
			if options.layer == journalContextLayerActiveChanges {
				return options.cursor
			}
			return ""
		}())
		if err != nil {
			return journalHookContextResult{}, err
		}
	} else {
		stateResult.ActiveLineage.Available = false
		stateResult.ActiveLineage.AvailableCount = 0
		stateResult.ActiveLineage.ShownCount = 0
		stateResult.ActiveLineage.Truncated = false
		stateResult.ActiveLineage.Cursor = ""
		stateResult.ActiveLineage.Items = []state.JournalEntryRecord{}
		stateResult.Diagnostics = append(stateResult.Diagnostics, state.JournalContextDiagnostic{Code: changeSourceUnavailableCode, Message: "Change source unavailable: " + changeSourceErr.Error() + "; active-changes and active-lineage could not be derived"})
	}
	rewriteJournalContextExpandCommands(&stateResult, &activeLayer, options)
	contextResult := composeJournalContextCLIResult(stateResult, activeLayer, options.layer)
	return newJournalHookContextAvailable(contextResult)
}
