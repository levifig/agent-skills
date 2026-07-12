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
// and git/gh PostToolUse). Under the journal-first model (SPEC-056) there is no
// session entity: the only lifecycle-relevant field is agent_id, which flags a
// subagent invocation the hooks must silently ignore.
type journalHookInput struct {
	SessionID string
	AgentID   string
	Raw       map[string]any
}

type journalHookWarning struct {
	ContractVersion  int    `json:"contract_version"`
	Kind             string `json:"kind"`
	Severity         string `json:"severity"`
	Code             string `json:"code"`
	Message          string `json:"message"`
	Key              string `json:"key"`
	LatestSourceID   string `json:"latest_source_id"`
	LatestSourceType string `json:"latest_source_type"`
	NonBlocking      bool   `json:"non_blocking"`
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
	raw := input.Raw
	if raw == nil {
		return "", false
	}
	hookEventName := stringMapValue(raw, "hook_event_name")
	toolName := firstNonEmpty(stringMapValue(raw, "tool_name"), nestedStringMapValue(raw, "tool", "name"))
	if hookEventName == "TaskCompleted" || toolName == "TaskCompleted" {
		description := firstNonEmpty(stringMapValue(raw, "task_description"), stringMapValue(raw, "task_subject"), "task")
		return "task(completed): " + description, true
	}
	command := firstNonEmpty(
		nestedStringMapValue(raw, "tool_input", "command"),
		nestedStringMapValue(raw, "input", "command"),
		nestedStringMapValue(raw, "tool", "input", "command"),
	)
	if command == "" {
		if commit := stringMapValue(raw, "commit"); commit != "" {
			return fmt.Sprintf("commit(%s): %s", commit, firstNonEmpty(stringMapValue(raw, "message"), "commit")), true
		}
		if pr := stringMapValue(raw, "pr"); pr != "" {
			return fmt.Sprintf("pr(create): %s (#%s)", firstNonEmpty(stringMapValue(raw, "title"), "PR created"), pr), true
		}
		if merge := stringMapValue(raw, "merge"); merge != "" {
			return fmt.Sprintf("pr(merge): #%s", merge), true
		}
		return "", false
	}
	switch {
	case strings.Contains(command, "git commit"):
		message := commandFlagValue(command, "-m")
		if message == "" {
			message = "commit"
		}
		return fmt.Sprintf("commit(unknown): %s", message), true
	case strings.Contains(command, "gh pr create"):
		title := commandFlagValue(command, "--title")
		if title == "" {
			title = "PR created"
		}
		return "pr(create): " + title, true
	case strings.Contains(command, "gh pr merge"):
		return "pr(merge): #unknown", true
	default:
		return "", false
	}
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
		"  No threshold - if it mutates, track it. TaskCompleted events auto-log to the journal.",
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
	if hookInput.AgentID != "" {
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
	if hookInput.AgentID != "" {
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
	if hookInput.AgentID != "" {
		return false, nil
	}
	if options.harnessSessionID == "" {
		options.harnessSessionID = hookInput.SessionID
	}
	if options.entry == "" {
		entry, ok := deriveJournalHookLogEntry(hookInput)
		if !ok {
			return false, nil
		}
		options.entry = entry
	}
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
	if hookInput.AgentID != "" {
		return nil
	}
	// PostCompact resumption is a hook path: a missing state database must not
	// fail the harness. The subagent guard already ran above, so the inner digest
	// call reads no further stdin.
	return r.runJournalContextDigest(nil, out, runtime, true)
}
