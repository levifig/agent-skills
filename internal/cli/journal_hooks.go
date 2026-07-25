package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// journalHookInput is the harness hook payload read from stdin for journal
// hooks (SessionStart, PreCompact, PostCompact, UserPromptSubmit, TaskCompleted,
// and git/gh PostToolUse). Under the journal-first model there is no
// session entity: the only lifecycle-relevant field is agent_id, which flags a
// subagent invocation the hooks must silently ignore.
type journalHookInput struct {
	SessionID string
	AgentID   string
	Raw       map[string]any
}

type journalHookWarning struct {
	ContractVersion  int    `json:"contract_version"`
	EnvelopeVersion  int    `json:"envelope_version,omitempty"`
	Kind             string `json:"kind"`
	Severity         string `json:"severity"`
	Code             string `json:"code"`
	Message          string `json:"message"`
	Target           string `json:"target,omitempty"`
	Harness          string `json:"harness,omitempty"`
	HarnessVersion   string `json:"harness_version,omitempty"`
	Event            string `json:"event,omitempty"`
	Outcome          string `json:"outcome,omitempty"`
	Key              string `json:"key"`
	LatestSourceID   string `json:"latest_source_id"`
	LatestSourceType string `json:"latest_source_type"`
	NonBlocking      bool   `json:"non_blocking"`
}

func writeJournalHookWarning(out io.Writer, warning *journalHookWarning) error {
	if warning == nil {
		return nil
	}
	return writeJSON(out, *warning)
}

func writeJournalHookUnmatchedUnblockWarning(out io.Writer, unmatched *state.JournalUnmatchedUnblockError) error {
	return writeJSON(out, journalHookWarning{
		ContractVersion:  state.StateJSONContractVersion,
		Kind:             "journal-diagnostic",
		Severity:         "warning",
		Code:             unmatched.Code,
		Message:          unmatched.Error(),
		Key:              unmatched.Key,
		LatestSourceID:   unmatched.LatestSourceID,
		LatestSourceType: unmatched.LatestSourceType,
		NonBlocking:      true,
	})
}

// readJournalHookInput parses the hook JSON on stdin. A missing or empty stdin
// yields a zero-value input (no error) so hooks degrade gracefully when invoked
// outside a harness.
func (r Runner) readJournalHookInput() (journalHookInput, error) {
	reader := r.Stdin
	if reader == nil {
		info, err := os.Stdin.Stat()
		if err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
			reader = os.Stdin
		}
	}
	if reader == nil {
		return journalHookInput{}, nil
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return journalHookInput{}, fmt.Errorf("read journal hook input: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return journalHookInput{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return journalHookInput{}, fmt.Errorf("parse journal hook input: %w", err)
	}
	return journalHookInput{
		SessionID: stringMapValue(raw, "session_id"),
		AgentID:   stringMapValue(raw, "agent_id"),
		Raw:       raw,
	}, nil
}

// deriveJournalHookLogEntry maps a git/gh/task hook payload to a journal entry
// string. Returns ok=false when the payload carries no recognizable event, so
// the hook writes nothing rather than logging noise.
func deriveJournalHookLogEntry(input journalHookInput) (string, bool) {
	envelope := normalizeJournalHookEnvelope(input, "")
	entry, ok, _ := journalHookEntryForEnvelope(envelope, input.Raw, false)
	return entry, ok
}

// isStateMissingError reports whether err is the canonical "state database is
// missing or uninitialized" signal emitted across the state package (the same
// sentinel withStateMissingContext keys off). Hook-invoked journal paths use it
// to degrade gracefully on a fresh install rather than failing the harness.
func isStateMissingError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLite state database is not initialized")
}

// journalContextPromptInstructions is injected on every UserPromptSubmit. It
// carries the durable implementation principles without any session-lifecycle
// language: the journal is the only session-related structure.
func journalContextPromptInstructions() string {
	return strings.Join([]string{
		"[Implementation Principles]",
		"- When the user's message is a QUESTION, answer it and STOP. Do not implement anything.",
		"  Wait for explicit instructions before taking action.",
		"- Create a Task BEFORE any tool use that changes something (Edit, Write, Bash, etc.).",
		"  No threshold - if it mutates, track it. Automatic task completion capture is disabled until a proven adapter is installed; write completion entries explicitly.",
		"  Create tasks before starting work, update status as you go, mark complete when done.",
		"- Delegate code changes to agents - orchestrator coordinates, doesn't implement.",
		"- Log decisions: loaf journal log \"decision(scope): description\".",
		"- One concern per agent, parallel when independent.",
	}, "\n")
}

// journalContextCompactInstructions is injected before compaction. It retargets
// the emergency-flush guidance to the journal: log unrecorded entries now, and
// write a wrap-type checkpoint only when there is synthesis worth saving.
func journalContextCompactInstructions() string {
	return strings.Join([]string{
		"CONTEXT COMPACTION IMMINENT: Your conversation context will be compacted soon.",
		"",
		"REQUIRED before the model responds:",
		"",
		"1. **Flush journal entries.** Log all unrecorded decisions, discoveries, and progress:",
		"   - `decision(scope): key decisions made this conversation`",
		"   - `discover(scope): important findings`",
		"   - `finding(scope): analysis result`",
		"   Run `loaf journal log \"type(scope): description\"` for each.",
		"",
		"2. **Write a wrap checkpoint IF there is synthesis worth saving.** A wrap captures the",
		"   connective narrative that evaporates with the context window - \"tried X, abandoned",
		"   because Y, next is Z\" - not just raw events. Only write one when that synthesis exists:",
		"   `loaf journal log \"wrap(scope): what happened, what's blocked, what's next\"`.",
		"",
		"The journal IS your external memory. Entries not flushed now are lost forever.",
	}, "\n")
}

// runJournalContextForPrompt emits the UserPromptSubmit implementation
// principles, silently exiting for subagent invocations.
func (r Runner) runJournalContextForPrompt(out io.Writer) error {
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	if normalizeJournalHookEnvelope(hookInput, "").isSubagent() {
		return nil
	}
	fmt.Fprintln(out, journalContextPromptInstructions())
	return nil
}

// runJournalContextForCompact emits the PreCompact flush guidance, silently
// exiting for subagent invocations. It writes nothing to the journal: only the
// deliberate entries the model logs in response are persisted.
func (r Runner) runJournalContextForCompact(out io.Writer) error {
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	if normalizeJournalHookEnvelope(hookInput, "").isSubagent() {
		return nil
	}
	fmt.Fprintln(out, journalContextCompactInstructions())
	return nil
}

// journalLogFromHook applies git/gh/task hook derivation and Linear magic-word
// detection to a journal log invocation. It returns proceed=false when the hook
// should write nothing (subagent, empty payload, no derivable entry, or Linear
// integration disabled).
func (r Runner) journalLogFromHook(options *journalLogOptions, projectRoot project.Root, worktree string) (proceed bool, err error) {
	if options.detectLinear {
		entry, ok, disabled, derr := detectLinearJournalEntry(projectRoot.Path(), worktree)
		if derr != nil {
			return false, derr
		}
		if disabled || !ok {
			return false, nil
		}
		options.entry = entry
		return true, nil
	}

	hookInput, herr := r.readJournalHookInput()
	if herr != nil {
		return false, herr
	}
	envelope := normalizeJournalHookEnvelope(hookInput, worktree)
	if envelope.isSubagent() {
		options.hookSuppressed = true
		return false, nil
	}
	if options.harnessSessionID == "" {
		options.harnessSessionID = envelope.SessionID
	}
	if options.entry != "" && envelope.Event == "" && envelope.CommandFamily == "" {
		branch := state.ObservedGitBranch(worktree)
		origin := envelope.origin(branch)
		options.origin = &origin
		return true, nil
	}
	entry, ok, warning := journalHookEntryForEnvelope(envelope, hookInput.Raw, false)
	if warning != nil {
		options.hookWarning = warning
	}
	if !ok {
		return false, nil
	}
	if options.entry == "" {
		options.entry = entry
	}
	branch := state.ObservedGitBranch(worktree)
	origin := envelope.origin(branch)
	options.origin = &origin
	return true, nil
}

// runJournalContextResumption prints the layered continuity digest for
// post-compaction resumption. It is the same deterministic query as
// SessionStart: latest project wrap + recent branch entries + open tasks.
func (r Runner) runJournalContextResumption(out io.Writer, runtime state.Runtime) error {
	hookInput, err := r.readJournalHookInput()
	if err != nil {
		return err
	}
	return r.runJournalContextDigestWithHookInput(nil, out, runtime, true, &hookInput)
}
