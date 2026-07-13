package cli

import (
	"fmt"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

// journalHookEnvelopeVersion is the compact adapter contract between target
// hook payloads and journal capture. Raw harness payloads are never persisted.
const journalHookEnvelopeVersion = 1

const (
	journalHookDiagnosticUnknownSuccess  = "journal-hook-success-fidelity-unknown"
	journalHookDiagnosticUnsupported     = "journal-hook-completion-unsupported"
	journalHookDiagnosticMissingState    = "journal-hook-state-missing"
	journalHookDiagnosticInvalidEnvelope = "journal-hook-envelope-invalid"
	journalHookDiagnosticUnknownEvent    = "journal-hook-event-unknown"
	journalHookDiagnosticNoDurableResult = "journal-hook-durable-result-unknown"
	journalHookDiagnosticOutcomeUnknown  = "journal-hook-outcome-unknown"
	journalHookDiagnosticOutcomeMismatch = "journal-hook-outcome-incompatible"
)

// normalizedJournalHookEnvelope contains only fields that the adapter can
// prove from a target payload. Pointer success is intentional: false is known
// and different from an absent/unknown result signal.
type normalizedJournalHookEnvelope struct {
	ContractVersion   int
	Target            string
	Harness           string
	HarnessVersion    string
	Event             string
	SessionID         string
	AgentID           string
	Background        bool
	Worktree          string
	Success           *bool
	Outcome           string
	CommandFamily     string
	DurableResultKind string
	DurableResultID   string
	Message           string
	Title             string
	TaskDescription   string
	DiagnosticCode    string
	Diagnostic        string
}

func (e normalizedJournalHookEnvelope) isSubagent() bool {
	return strings.TrimSpace(e.AgentID) != ""
}

func (e normalizedJournalHookEnvelope) suppressesContext() bool {
	return e.isSubagent() || e.Background
}

func (e normalizedJournalHookEnvelope) hasProvenResult() bool {
	return e.Success != nil && *e.Success && e.outcomeCompatible() && e.durableResultCompatible()
}

func (e normalizedJournalHookEnvelope) outcomeCompatible() bool {
	if e.Success == nil || !*e.Success {
		return false
	}
	switch e.CommandFamily {
	case "git_commit":
		return e.Outcome == "created" || e.Outcome == "amended"
	case "pr_create":
		return e.Outcome == "created"
	case "pr_merge":
		return e.Outcome == "merged"
	case "task_completed":
		return e.Outcome == "completed"
	default:
		return false
	}
}

func (e normalizedJournalHookEnvelope) durableResultCompatible() bool {
	if strings.TrimSpace(e.DurableResultID) == "" {
		return false
	}
	switch e.CommandFamily {
	case "git_commit":
		return e.DurableResultKind == "git-commit"
	case "pr_create":
		return e.DurableResultKind == "pull-request:create"
	case "pr_merge":
		return e.DurableResultKind == "pull-request:merge"
	case "task_completed":
		return e.DurableResultKind == "task-completion"
	default:
		return false
	}
}

func (e normalizedJournalHookEnvelope) warning(code, message string) journalHookWarning {
	return journalHookWarning{
		ContractVersion: state.StateJSONContractVersion,
		EnvelopeVersion: e.ContractVersion,
		Kind:            "journal-diagnostic",
		Severity:        "warning",
		Code:            code,
		Message:         message,
		Target:          e.Target,
		Harness:         e.Harness,
		HarnessVersion:  e.HarnessVersion,
		Event:           e.Event,
		Outcome:         e.Outcome,
		NonBlocking:     true,
	}
}

func (e normalizedJournalHookEnvelope) origin(branch string) state.JournalOriginInput {
	return state.JournalOriginInput{
		EnvelopeVersion:        e.ContractVersion,
		CaptureMechanism:       state.JournalOriginMechanismHook,
		ObservedHarness:        e.Harness,
		ObservedHarnessVersion: e.HarnessVersion,
		HarnessSessionID:       e.SessionID,
		AgentID:                e.AgentID,
		SourceEvent:            e.Event,
		Branch:                 branch,
		Worktree:               e.Worktree,
		DurableResultKind:      e.DurableResultKind,
		DurableResultID:        e.DurableResultID,
	}
}

func normalizeJournalHookEnvelope(input journalHookInput, defaultWorktree string) normalizedJournalHookEnvelope {
	raw := input.Raw
	envelope := normalizedJournalHookEnvelope{ContractVersion: journalHookEnvelopeVersion}
	if raw == nil {
		return envelope
	}
	target, namespace, namespaced := journalHookTargetNamespace(raw)
	source := raw
	if namespaced {
		source = namespace
	}
	envelope.Target = target
	envelope.Harness = firstMapString(source, source, "harness", "harness_name", "runtime", "provider")
	envelope.HarnessVersion = firstMapString(source, source, "harness_version", "version", "cli_version", "runtime_version")
	if envelope.Harness == "" {
		envelope.Harness = target
	}
	envelope.Event = firstMapString(source, source, "hook_event_name", "event", "event_name", "lifecycle_event")
	envelope.SessionID = firstMapString(source, source, "session_id", "sessionID", "sessionId", "conversation_id", "thread_id")
	envelope.AgentID = firstMapString(source, source, "agent_id", "agentID", "subagent_id")
	if envelope.AgentID == "" {
		if agent := firstMapNestedString(source, source, []string{"agent", "id"}, []string{"agent", "agent_id"}); agent != "" {
			envelope.AgentID = agent
		}
	}
	if envelope.AgentID == "" && boolValue(firstMapBool(source, source, "is_subagent", "subagent")) {
		envelope.AgentID = "subagent"
	}
	background := firstMapBool(source, source, "is_background", "background", "run_in_background")
	if background != nil {
		envelope.Background = *background
	}
	if !envelope.Background {
		mode := strings.ToLower(firstMapString(source, source, "agent_type", "agent_mode", "mode"))
		envelope.Background = strings.Contains(mode, "background")
	}
	envelope.Worktree = firstMapString(source, source, "worktree", "cwd", "working_directory", "project_root")
	if envelope.Worktree == "" {
		envelope.Worktree = strings.TrimSpace(defaultWorktree)
	}
	envelope.Success = firstMapBool(source, source, "success", "ok", "succeeded")
	if envelope.Success == nil {
		envelope.Success = firstNestedBool(source, source, []string{"result", "success"}, []string{"result", "ok"}, []string{"output", "success"})
	}
	envelope.Outcome = normalizeJournalHookOutcome(firstMapString(source, source, "outcome", "result_outcome", "status", "result_status"))
	if envelope.Outcome == "unknown" {
		if result := firstMapMap(source, source, "result", "output"); result != nil {
			envelope.Outcome = normalizeJournalHookOutcome(firstMapString(result, result, "outcome", "status", "result_status"))
		}
	}
	envelope.CommandFamily = normalizeJournalHookCommandFamily(firstMapString(source, source, "command_family", "commandFamily", "family"))
	if envelope.CommandFamily == "" {
		envelope.CommandFamily = journalHookCommandFamilyFromPayload(source, source)
	}
	envelope.DurableResultKind, envelope.DurableResultID = journalHookDurableResult(source, source, envelope.CommandFamily)
	envelope.Message = firstMapString(source, source, "message", "subject")
	envelope.Title = firstMapString(source, source, "title", "subject")
	envelope.TaskDescription = firstMapString(source, source, "task_description", "task_subject", "description")
	if envelope.Event == "" && envelope.CommandFamily != "" {
		envelope.Event = envelope.CommandFamily
	}
	return envelope
}

func normalizeJournalHookOutcome(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	switch value {
	case "created", "create", "new":
		return "created"
	case "amended", "amend":
		return "amended"
	case "merged", "merge":
		return "merged"
	case "completed", "complete", "done":
		return "completed"
	case "failed", "failure", "error", "errored":
		return "failed"
	case "no-op", "noop", "nochange", "no-change", "unchanged", "already-exists":
		return "no-op"
	default:
		return "unknown"
	}
}

func journalHookTargetNamespace(raw map[string]any) (string, map[string]any, bool) {
	aliases := []struct {
		raw       string
		canonical string
	}{
		{"claude", "claude-code"}, {"claude_code", "claude-code"}, {"claude-code", "claude-code"},
		{"cursor", "cursor"}, {"codex", "codex"}, {"opencode", "opencode"}, {"open-code", "opencode"}, {"open_code", "opencode"},
		{"amp", "amp"}, {"synthetic", "synthetic"}, {"test", "test"},
	}
	target := normalizeJournalHookTarget(firstNonEmpty(stringMapValue(raw, "target"), stringMapValue(raw, "adapter"), stringMapValue(raw, "harness"), stringMapValue(raw, "harness_name")))
	for _, alias := range aliases {
		if nested, ok := raw[alias.raw].(map[string]any); ok {
			if target == "" || target == alias.canonical {
				return alias.canonical, nested, true
			}
		}
	}
	return target, raw, false
}

func normalizeJournalHookTarget(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "claude", "claude-code":
		return "claude-code"
	case "open-code", "opencode":
		return "opencode"
	default:
		return value
	}
}

func normalizeJournalHookCommandFamily(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	switch value {
	case "git_commit", "commit":
		return "git_commit"
	case "gh_pr_create", "pr_create", "github_pr_create":
		return "pr_create"
	case "gh_pr_merge", "pr_merge", "github_pr_merge":
		return "pr_merge"
	case "task_completed", "taskcompletion":
		return "task_completed"
	default:
		return ""
	}
}

func journalHookCommandFamilyFromPayload(namespace, raw map[string]any) string {
	command := firstMapString(namespace, raw, "command")
	if command == "" {
		command = firstNonEmpty(nestedStringMapValue(namespace, "tool_input", "command"), nestedStringMapValue(raw, "tool_input", "command"), nestedStringMapValue(raw, "input", "command"), nestedStringMapValue(raw, "tool", "input", "command"))
	}
	switch {
	case strings.Contains(command, "git commit"):
		return "git_commit"
	case strings.Contains(command, "gh pr create"):
		return "pr_create"
	case strings.Contains(command, "gh pr merge"):
		return "pr_merge"
	default:
		event := strings.ToLower(firstMapString(namespace, raw, "hook_event_name", "event", "event_name"))
		if event == "taskcompleted" || event == "task_completed" {
			return "task_completed"
		}
		return ""
	}
}

func journalHookDurableResult(namespace, raw map[string]any, family string) (string, string) {
	resultKind := normalizeJournalHookResultKind(firstMapString(namespace, raw, "durable_result_kind", "result_kind", "result_type"))
	resultID := firstMapString(namespace, raw, "durable_result_id", "result_id")
	if resultID == "" {
		switch family {
		case "git_commit":
			resultID = firstMapString(namespace, raw, "commit_sha", "commit_id", "sha", "sha1")
		case "pr_create", "pr_merge":
			resultID = firstMapString(namespace, raw, "pr_number", "pull_request_number", "number")
		case "task_completed":
			resultID = firstMapString(namespace, raw, "task_id", "task_completion_id", "completion_id")
		}
	}
	result := firstMapMap(namespace, raw, "result", "output", "commit", "pr", "merge")
	if resultID == "" && result != nil {
		resultID = firstMapString(result, result, "durable_result_id", "result_id", "id", "number", "sha", "sha1", "commit_sha", "commit_id", "url")
	}
	if resultKind == "" {
		switch family {
		case "git_commit":
			resultKind = "git-commit"
		case "pr_create":
			resultKind = "pull-request:create"
		case "pr_merge":
			resultKind = "pull-request:merge"
		case "task_completed":
			resultKind = "task-completion"
		}
	}
	if resultKind == "pull-request" {
		switch family {
		case "pr_create":
			resultKind = "pull-request:create"
		case "pr_merge":
			resultKind = "pull-request:merge"
		}
	}
	if resultKind == "task" || resultKind == "task-result" {
		resultKind = "task-completion"
	}
	return resultKind, strings.TrimSpace(resultID)
}

func normalizeJournalHookResultKind(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "git-commit", "commit":
		return "git-commit"
	case "pull-request", "pr", "pullrequest":
		return "pull-request"
	case "task", "task-result", "task-completion", "taskcompleted":
		return "task-completion"
	default:
		return value
	}
}

func firstMapMap(primary, fallback map[string]any, keys ...string) map[string]any {
	for _, values := range []map[string]any{primary, fallback} {
		for _, key := range keys {
			if nested, ok := values[key].(map[string]any); ok {
				return nested
			}
		}
	}
	return nil
}

func firstMapString(primary, fallback map[string]any, keys ...string) string {
	for _, values := range []map[string]any{primary, fallback} {
		for _, key := range keys {
			if value := stringMapValue(values, key); value != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func firstMapBool(primary, fallback map[string]any, keys ...string) *bool {
	for _, values := range []map[string]any{primary, fallback} {
		for _, key := range keys {
			if value, ok := values[key].(bool); ok {
				return &value
			}
		}
	}
	return nil
}

func firstMapNestedString(primary, fallback map[string]any, paths ...[]string) string {
	for _, values := range []map[string]any{primary, fallback} {
		for _, path := range paths {
			if value := nestedStringMapValue(values, path...); value != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func firstNestedBool(primary, fallback map[string]any, paths ...[]string) *bool {
	for _, values := range []map[string]any{primary, fallback} {
		for _, path := range paths {
			current := values
			for i, key := range path {
				value, ok := current[key]
				if !ok {
					current = nil
					break
				}
				if i == len(path)-1 {
					if result, ok := value.(bool); ok {
						return &result
					}
					current = nil
					break
				}
				nested, ok := value.(map[string]any)
				if !ok {
					current = nil
					break
				}
				current = nested
			}
		}
	}
	return nil
}

func boolPtr(value bool) *bool { return &value }

func boolValue(value *bool) bool { return value != nil && *value }

func journalHookEntryForEnvelope(envelope normalizedJournalHookEnvelope, _ map[string]any, trusted bool) (string, bool, *journalHookWarning) {
	if envelope.Event == "" && envelope.CommandFamily == "" {
		return "", false, nil
	}
	if envelope.isSubagent() {
		return "", false, nil
	}
	if !trusted {
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticUnsupported, fmt.Sprintf("automatic completion capture is not proven for target %q", firstNonEmpty(envelope.Target, "unknown"))))
	}
	if envelope.Success == nil {
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticUnknownSuccess, "hook payload did not provide an explicit success result; no completion was recorded"))
	}
	if !*envelope.Success {
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticOutcomeMismatch, "hook reported failure; no completion was recorded"))
	}
	if envelope.Outcome == "unknown" {
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticOutcomeUnknown, "hook payload did not provide a recognized proven outcome; no completion was recorded"))
	}
	if !envelope.durableResultCompatible() {
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticNoDurableResult, "successful hook payload did not provide a compatible durable result identity; no completion was recorded"))
	}
	if !envelope.outcomeCompatible() {
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticOutcomeMismatch, "hook outcome is not compatible with its completion family; no completion was recorded"))
	}
	switch envelope.CommandFamily {
	case "task_completed":
		description := envelope.TaskDescription
		if description == "" {
			description = "task"
		}
		return "task(completed): " + description, true, nil
	case "git_commit":
		message := envelope.Message
		if message == "" {
			message = "commit"
		}
		return fmt.Sprintf("commit(%s): %s", envelope.DurableResultID, message), true, nil
	case "pr_create":
		title := envelope.Title
		if title == "" {
			title = "PR"
		}
		return fmt.Sprintf("pr(create): %s (#%s)", title, envelope.DurableResultID), true, nil
	case "pr_merge":
		return fmt.Sprintf("pr(merge): #%s", envelope.DurableResultID), true, nil
	default:
		return "", false, journalHookWarningPtr(envelope.warning(journalHookDiagnosticUnknownEvent, "hook event is not an approved automatic journal completion"))
	}
}

func journalHookWarningPtr(value journalHookWarning) *journalHookWarning { return &value }
