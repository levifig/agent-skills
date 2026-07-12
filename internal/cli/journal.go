package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// runJournal dispatches the project-scoped journal command namespace (SPEC-056).
// The journal is the only session-related structure: entries are project events
// tagged with an opaque harness_session_id correlation column.
func (r Runner) runJournal(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeJournalHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"log":     writeJournalLogHelp,
		"recent":  writeJournalRecentHelp,
		"search":  writeJournalSearchHelp,
		"show":    writeJournalShowHelp,
		"defer":   writeJournalDeferHelp,
		"context": writeJournalContextHelp,
		"export":  writeJournalExportHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "log":
		return r.runJournalLog(args[1:], out, runtime)
	case "recent":
		return r.runJournalRecent(args[1:], out, runtime)
	case "search":
		return r.runJournalSearch(args[1:], out, runtime)
	case "show":
		return r.runJournalShow(args[1:], out, runtime)
	case "defer":
		return r.runJournalDefer(args[1:], out, runtime)
	case "context":
		return r.runJournalContext(args[1:], out, runtime)
	case "export":
		return r.runJournalExport(args[1:], out, runtime)
	default:
		return unknownSubcommandError("journal", args[0])
	}
}

func writeJournalHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal <command>", "Project-scoped journal: the durable, searchable record of what happened across all conversations.",
		"log       Append a typed journal entry",
		"recent    Show the recent project timeline",
		"search    Full-text search journal entries",
		"show      Show one journal entry by id",
		"defer     Capture a self-sufficient deferred intent and spark",
		"context   Emit the layered continuity digest",
		"export    Export the journal to markdown or JSONL")
}

func writeJournalLogHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal log [options] \"type(scope): message\"", "Append a project-scoped journal entry.",
		"--execpolicy-safe     Codex Auto mode: place immediately after `journal log`; require the registered project and derive database/provenance from the current runtime or hook payload",
		"--harness-session-id  Opaque conversation correlation tag",
		"--branch              Observed branch (defaults to current git branch)",
		"--worktree            Observed worktree path",
		"--from-hook           Derive the entry from a harness hook payload on stdin (git/gh/task events)",
		"--detect-linear       Scan recent commits for Linear magic words and log a discovery entry",
		"--json                Output the written entry and project context as JSON")
}

func writeJournalRecentHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal recent [options]", "Show the recent project journal timeline, newest first.",
		"--branch           Restrict to entries observed on one branch",
		"--since-last-wrap  Trim to entries logged after the most recent wrap",
		"--limit            Maximum entries to return",
		"--json             Output the timeline and project context as JSON")
}

func writeJournalSearchHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal search [options] <query>", "Full-text search across project journal entries.",
		"--all     Search across all projects",
		"--limit   Maximum hits to return",
		"--json    Output hits and project context as JSON")
}

func writeJournalShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal show <entry-id> [--json]", "Show one journal entry by id.",
		"--json  Output the entry and project context as JSON")
}

func writeJournalDeferHelp(out io.Writer) {
	writeUsageHelp(out, `loaf journal defer "<intent>" --why "..." --boundary "..." --trigger "..." --operation-id "..." [--change <slug|path>] [--json]`, "Capture one self-sufficient deferred intent as a reciprocal decision and open spark pair; stable operation IDs make first writes idempotent and reworded retries visible.",
		"<intent>              One-line intent to revisit",
		"--why                 Why this intent was deferred",
		"--boundary            What remains outside this packet",
		"--trigger             What should cause revisit",
		"--operation-id        Stable retry/idempotency key",
		"--change              Optional retained Change slug or canonical path for local evidence",
		"--json                Output the state result as JSON")
}

func writeJournalContextHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal context [options]", "Emit the contract-v2 active-truth continuity digest.",
		"--branch          Branch scope (defaults to the current git branch)",
		"--layer           Select one layer: project-synthesis, scoped-checkpoint, active-lineage, unresolved-blockers, deferred-intent, active-changes, branch-recency, transitional-tasks",
		"--limit           Maximum 1..100 items for the selected layer (requires --layer)",
		"--cursor          Continue the selected layer (requires --layer)",
		"--from-hook       Read the harness hook payload on stdin and exit silently for subagent invocations",
		"--json            Output contract-v2 project metadata, layers, and diagnostics as JSON",
		"",
		"Hook subcommands (read stdin, exit silently for subagents):",
		"  for-prompt      Inject implementation principles on UserPromptSubmit",
		"  for-compact     Inject journal-flush guidance before compaction",
		"  for-resumption  Emit the continuity digest after compaction")
}

func writeJournalExportHelp(out io.Writer) {
	writeUsageHelp(out, "loaf journal export [--format markdown|jsonl]", "Export the project journal. SQLite stays canonical; export is a transport view.",
		"--format  markdown (default) or jsonl")
}

type journalLogOptions struct {
	entry               string
	harnessSessionID    string
	harnessSessionIDSet bool
	branch              string
	branchSet           bool
	worktree            string
	worktreeSet         bool
	jsonOutput          bool
	fromHook            bool
	detectLinear        bool
	execpolicySafe      bool
}

const journalExecpolicySafeCode = "journal-execpolicy-safe"

type journalExecpolicySafeError struct {
	reason string
}

func (e *journalExecpolicySafeError) Error() string {
	if e == nil {
		return ""
	}
	return "--execpolicy-safe " + e.reason
}

type journalUnmatchedUnblockJSON struct {
	ContractVersion  int    `json:"contract_version"`
	Command          string `json:"command"`
	Error            string `json:"error"`
	Code             string `json:"code"`
	Key              string `json:"key"`
	LatestSourceID   string `json:"latest_source_id"`
	LatestSourceType string `json:"latest_source_type"`
}

func parseJournalLogArgs(args []string) (journalLogOptions, error) {
	options := journalLogOptions{}
	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--execpolicy-safe":
			if options.execpolicySafe {
				return journalLogOptions{}, &journalExecpolicySafeError{reason: "may be specified only once"}
			}
			options.execpolicySafe = true
		case "--json":
			options.jsonOutput = true
		case "--from-hook":
			options.fromHook = true
		case "--detect-linear":
			options.detectLinear = true
		case "--harness-session-id":
			value, err := consumeFlagValue(args, &i, "--harness-session-id")
			if err != nil {
				return journalLogOptions{}, err
			}
			options.harnessSessionID = value
			options.harnessSessionIDSet = true
		case "--branch":
			value, err := consumeFlagValue(args, &i, "--branch")
			if err != nil {
				return journalLogOptions{}, err
			}
			options.branch = value
			options.branchSet = true
		case "--worktree":
			value, err := consumeFlagValue(args, &i, "--worktree")
			if err != nil {
				return journalLogOptions{}, err
			}
			options.worktree = value
			options.worktreeSet = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return journalLogOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if options.execpolicySafe {
		if options.harnessSessionIDSet {
			return journalLogOptions{}, &journalExecpolicySafeError{reason: "rejects caller-supplied --harness-session-id"}
		}
		if options.branchSet {
			return journalLogOptions{}, &journalExecpolicySafeError{reason: "rejects caller-supplied --branch"}
		}
		if options.worktreeSet {
			return journalLogOptions{}, &journalExecpolicySafeError{reason: "rejects caller-supplied --worktree"}
		}
		if options.fromHook && options.detectLinear {
			return journalLogOptions{}, &journalExecpolicySafeError{reason: "cannot combine --from-hook and --detect-linear"}
		}
	}
	// --from-hook and --detect-linear derive the entry from stdin/git, so the
	// positional entry is optional in that mode.
	if options.fromHook || options.detectLinear {
		if len(positional) > 1 {
			return journalLogOptions{}, fmt.Errorf("journal log accepts at most one \"type(scope): message\" entry")
		}
		if len(positional) == 1 {
			options.entry = positional[0]
		}
		return options, nil
	}
	if len(positional) != 1 {
		return journalLogOptions{}, fmt.Errorf("journal log requires exactly one \"type(scope): message\" entry")
	}
	options.entry = positional[0]
	return options, nil
}

func (r Runner) runJournalLog(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalLogArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "journal log", err)
		}
		return err
	}
	if options.execpolicySafe && os.Getenv("LOAF_DB") != "" {
		err := &journalExecpolicySafeError{reason: "refuses non-empty LOAF_DB; use the registered project database"}
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal log", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal log", err)
		}
		return err
	}
	worktree := options.worktree
	if worktree == "" {
		worktree = runtime.RootPath()
	}
	if options.fromHook || options.detectLinear {
		proceed, herr := r.journalLogFromHook(&options, projectRoot, worktree)
		if herr != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "journal log", herr)
			}
			return herr
		}
		if !proceed {
			return nil
		}
	}
	branch := options.branch
	if !options.branchSet {
		branch = state.ObservedGitBranch(worktree)
	}
	mechanism := "journal.log"
	if options.execpolicySafe {
		mechanism = "journal.log.execpolicy-safe"
	}
	origin := ResolveManualJournalOrigin(worktree, mechanism)
	result, err := state.LogJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalLogOptions{
		Entry:            options.entry,
		ObservedBranch:   branch,
		ObservedWorktree: worktree,
		HarnessSessionID: options.harnessSessionID,
		Origin:           &origin,
	})
	if err != nil {
		var unmatched *state.JournalUnmatchedUnblockError
		if errors.As(err, &unmatched) {
			if options.fromHook {
				return writeJournalHookUnmatchedUnblockWarning(out, unmatched)
			}
			if options.jsonOutput {
				if writeErr := writeJSON(out, journalUnmatchedUnblockJSON{
					ContractVersion:  state.StateJSONContractVersion,
					Command:          "journal log",
					Error:            unmatched.Error(),
					Code:             unmatched.Code,
					Key:              unmatched.Key,
					LatestSourceID:   unmatched.LatestSourceID,
					LatestSourceType: unmatched.LatestSourceType,
				}); writeErr != nil {
					return writeErr
				}
				return ExitError{Code: 1}
			}
		}
		// Ordinary hook-invoked logging (git/gh/task PostToolUse via --from-hook, or
		// Linear detection) degrades on a fresh install so a harness is not blocked.
		// The hardened execpolicy-safe mode is intentionally different: it requires
		// the registered project identity and keeps missing-state failures visible.
		if !options.execpolicySafe && (options.fromHook || options.detectLinear) && isStateMissingError(err) {
			return nil
		}
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal log", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprint(out, "\n  loaf journal log\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	label := result.EntryType
	if result.Scope != "" {
		label = fmt.Sprintf("%s(%s)", result.EntryType, result.Scope)
	}
	fmt.Fprintf(out, "  logged %s: %s\n", label, result.Message)
	if result.ObservedBranch != "" {
		fmt.Fprintf(out, "  branch: %s\n", result.ObservedBranch)
	}
	if result.HarnessSessionID != "" {
		fmt.Fprintf(out, "  harness session: %s\n", result.HarnessSessionID)
	}
	return nil
}

type journalRecentOptions struct {
	branch        string
	sinceLastWrap bool
	limit         int
	jsonOutput    bool
}

func parseJournalRecentArgs(args []string) (journalRecentOptions, error) {
	options := journalRecentOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--since-last-wrap":
			options.sinceLastWrap = true
		case "--branch":
			value, err := consumeFlagValue(args, &i, "--branch")
			if err != nil {
				return journalRecentOptions{}, err
			}
			options.branch = value
		case "--limit":
			value, err := consumeFlagValue(args, &i, "--limit")
			if err != nil {
				return journalRecentOptions{}, err
			}
			limit, err := parsePositiveLimit(value)
			if err != nil {
				return journalRecentOptions{}, err
			}
			options.limit = limit
		default:
			return journalRecentOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return options, nil
}

func (r Runner) runJournalRecent(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalRecentArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "journal recent", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal recent", err)
		}
		return err
	}
	result, err := state.RecentJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalRecentOptions{
		Branch:        options.branch,
		SinceLastWrap: options.sinceLastWrap,
		Limit:         options.limit,
	})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal recent", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprint(out, "\n  loaf journal recent\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.Branch != "" {
		fmt.Fprintf(out, "  branch: %s\n", result.Branch)
	}
	if result.SinceLastWrap {
		fmt.Fprintln(out, "  scope: since last wrap")
	}
	if len(result.Entries) == 0 {
		fmt.Fprintln(out, "  no journal entries")
		return nil
	}
	for _, entry := range result.Entries {
		fmt.Fprintf(out, "  %s\n", formatJournalEntryLine(entry))
	}
	return nil
}

func (r Runner) runJournalSearch(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalSearchArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "journal search", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal search", err)
		}
		return err
	}
	// Journal search queries only the journal_search FTS table joined to
	// journal_entries. It never refreshes or scans the docs index, so a
	// journal-only read cannot mutate docs state or fail on unrelated docs
	// scanning (SPEC-056 M1).
	result, err := state.SearchJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.SearchOptions{
		Query:       options.query,
		AllProjects: options.allProjects,
		Limit:       options.limit,
	})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal search", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "\n  loaf journal search %q\n\n", result.Query)
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.AllProjects {
		fmt.Fprintln(out, "  scope: all projects")
	} else {
		fmt.Fprintln(out, "  scope: current project")
	}
	fmt.Fprintf(out, "  results: %d\n", len(result.Results))
	for _, hit := range result.Results {
		fmt.Fprintf(out, "  - %s", searchHitAddress(hit))
		if hit.Snippet != "" {
			fmt.Fprintf(out, " — %s", hit.Snippet)
		}
		fmt.Fprintln(out)
	}
	return nil
}

type journalSearchOptions struct {
	query       string
	allProjects bool
	limit       int
	jsonOutput  bool
}

func parseJournalSearchArgs(args []string) (journalSearchOptions, error) {
	options := journalSearchOptions{}
	positional := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--all":
			options.allProjects = true
		case "--limit":
			value, err := consumeFlagValue(args, &i, "--limit")
			if err != nil {
				return journalSearchOptions{}, err
			}
			limit, err := parsePositiveLimit(value)
			if err != nil {
				return journalSearchOptions{}, err
			}
			options.limit = limit
		default:
			if strings.HasPrefix(args[i], "-") {
				return journalSearchOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) == 0 {
		return journalSearchOptions{}, fmt.Errorf("journal search requires a query")
	}
	options.query = strings.Join(positional, " ")
	return options, nil
}

func (r Runner) runJournalShow(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	jsonOutput := jsonRequested
	positional := []string{}
	for _, arg := range args {
		if arg == "--json" {
			continue
		}
		if strings.HasPrefix(arg, "-") {
			err := fmt.Errorf("unknown option %q", arg)
			if jsonRequested {
				return writeJSONCommandError(out, "journal show", err)
			}
			return err
		}
		positional = append(positional, arg)
	}
	if len(positional) != 1 {
		err := fmt.Errorf("journal show requires exactly one entry id")
		if jsonOutput {
			return writeJSONCommandError(out, "journal show", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "journal show", err)
		}
		return err
	}
	result, err := state.ShowJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, positional[0])
	if err != nil {
		if jsonOutput {
			return writeJSONCommandError(out, "journal show", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprint(out, "\n  loaf journal show\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "  id: %s\n", result.Entry.ID)
	fmt.Fprintf(out, "  %s\n", formatJournalEntryLine(result.Entry))
	writeJournalOriginHuman(out, result.Origin, "  ")
	return nil
}

type journalDeferOptions struct {
	intent      string
	why         string
	boundary    string
	trigger     string
	operationID string
	change      string
	jsonOutput  bool
}

func parseJournalDeferArgs(args []string) (journalDeferOptions, error) {
	var options journalDeferOptions
	seen := map[string]bool{}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if seen[arg] && arg == "--json" {
			return journalDeferOptions{}, &state.JournalDeferValidationError{Field: "json", Err: fmt.Errorf("flag may be specified only once")}
		}
		switch arg {
		case "--json":
			seen[arg] = true
			options.jsonOutput = true
		case "--why", "--boundary", "--trigger", "--operation-id", "--change":
			if seen[arg] {
				return journalDeferOptions{}, &state.JournalDeferValidationError{Field: strings.TrimPrefix(arg, "--"), Err: fmt.Errorf("flag may be specified only once")}
			}
			seen[arg] = true
			value, err := consumeFlagValue(args, &i, arg)
			if err != nil {
				return journalDeferOptions{}, &state.JournalDeferValidationError{Field: strings.TrimPrefix(arg, "--"), Err: err}
			}
			if arg == "--change" && strings.TrimSpace(value) == "" {
				return journalDeferOptions{}, &state.JournalDeferValidationError{Field: "change", Err: fmt.Errorf("must be nonblank")}
			}
			switch arg {
			case "--why":
				options.why = value
			case "--boundary":
				options.boundary = value
			case "--trigger":
				options.trigger = value
			case "--operation-id":
				options.operationID = value
			case "--change":
				options.change = value
			}
		default:
			if strings.HasPrefix(arg, "-") {
				return journalDeferOptions{}, &state.JournalDeferValidationError{Field: "flag", Err: fmt.Errorf("unknown option %q", arg)}
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 1 {
		return journalDeferOptions{}, &state.JournalDeferValidationError{Field: "intent", Err: fmt.Errorf("requires exactly one positional intent")}
	}
	options.intent = positional[0]
	for field, value := range map[string]string{"why": options.why, "boundary": options.boundary, "trigger": options.trigger, "operation_id": options.operationID} {
		if strings.TrimSpace(value) == "" {
			return journalDeferOptions{}, &state.JournalDeferValidationError{Field: field, Err: fmt.Errorf("must be nonblank")}
		}
	}
	if strings.TrimSpace(options.intent) == "" {
		return journalDeferOptions{}, &state.JournalDeferValidationError{Field: "intent", Err: fmt.Errorf("must be nonblank")}
	}
	return options, nil
}

func (r Runner) runJournalDefer(args []string, out io.Writer, runtime state.Runtime) error {
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalDeferArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "journal defer", err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal defer", err)
		}
		return err
	}
	origin := ResolveManualJournalOrigin(runtime.RootPath(), "journal.defer")
	if options.change != "" {
		origin, err = ResolveChangeOrigin(runtime.RootPath(), options.change)
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "journal defer", err)
			}
			return err
		}
	}
	result, err := state.DeferJournal(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalDeferOptions{
		Intent:      options.intent,
		Why:         options.why,
		Boundary:    options.boundary,
		Trigger:     options.trigger,
		OperationID: options.operationID,
		Origin:      &origin,
	})
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal defer", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeJournalDeferHuman(out, result)
	return nil
}

func writeJournalDeferHuman(out io.Writer, result state.JournalDeferResult) {
	fmt.Fprint(out, "\n  loaf journal defer\n\n")
	if result.Created {
		fmt.Fprintln(out, "  created decision + spark")
	} else {
		fmt.Fprintln(out, "  reused existing decision + spark")
	}
	fmt.Fprintf(out, "  operation: %s\n", result.OperationID)
	fmt.Fprintf(out, "  decision: %s\n", result.Decision.ID)
	fmt.Fprintf(out, "  spark: %s\n", result.Spark.ID)
	fmt.Fprintf(out, "  alias: %s\n", result.Spark.Alias)
	fmt.Fprintf(out, "  input digest: %s\n", result.InputDigest)
	fmt.Fprintf(out, "  stored digest: %s\n", result.StoredDigest)
	fmt.Fprintf(out, "  digest match: %t\n", result.InputDigestMatches)
	if !result.InputDigestMatches {
		fmt.Fprintln(out, "  warning: operation already exists; original decision + spark pair reused despite digest mismatch")
	}
	writeJournalOriginHuman(out, result.Origin, "  ")
}

func writeJournalOriginHuman(out io.Writer, origin *state.JournalOrigin, indent string) {
	if origin == nil {
		return
	}
	fmt.Fprintf(out, "%sprovenance:\n", indent)
	fmt.Fprintf(out, "%s  mechanism: %s\n", indent, origin.CaptureMechanism)
	fmt.Fprintf(out, "%s  envelope version: %d\n", indent, origin.EnvelopeVersion)
	fmt.Fprintf(out, "%s  supported: %t\n", indent, origin.Supported)
	if !origin.Supported {
		fmt.Fprintf(out, "%s  warning: unsupported origin envelope version\n", indent)
	}
	if origin.SourceEvent != "" {
		fmt.Fprintf(out, "%s  source event: %s\n", indent, origin.SourceEvent)
	}
	if origin.Branch != "" {
		fmt.Fprintf(out, "%s  branch: %s\n", indent, origin.Branch)
	}
	if origin.Worktree != "" {
		fmt.Fprintf(out, "%s  worktree: %s\n", indent, origin.Worktree)
	}
	if origin.Head != "" {
		fmt.Fprintf(out, "%s  head: %s\n", indent, origin.Head)
	}
	if origin.ChangePath != "" {
		fmt.Fprintf(out, "%s  change path: %s\n", indent, origin.ChangePath)
	}
	if origin.ChangeSHA256 != "" {
		fmt.Fprintf(out, "%s  change sha256: %s\n", indent, origin.ChangeSHA256)
	}
	if origin.Dirty != nil {
		fmt.Fprintf(out, "%s  dirty: %t\n", indent, *origin.Dirty)
	}
	if origin.Reconstructable != nil {
		fmt.Fprintf(out, "%s  reconstructable: %t\n", indent, *origin.Reconstructable)
	}
}

func (r Runner) runJournalContext(args []string, out io.Writer, runtime state.Runtime) error {
	return r.runJournalContextDigest(args, out, runtime, false)
}

const (
	journalContextLayerProjectSynthesis = "project-synthesis"
	journalContextLayerScopedCheckpoint = "scoped-checkpoint"
	journalContextLayerActiveLineage    = "active-lineage"
	journalContextLayerBlockers         = "unresolved-blockers"
	journalContextLayerDeferredIntent   = "deferred-intent"
	journalContextLayerBranchRecency    = "branch-recency"
	journalContextLayerTasks            = "transitional-tasks"
	defaultCLIJournalContextLimit       = 10
)

var canonicalJournalContextLayers = []string{
	journalContextLayerProjectSynthesis,
	journalContextLayerScopedCheckpoint,
	journalContextLayerActiveLineage,
	journalContextLayerBlockers,
	journalContextLayerDeferredIntent,
	journalContextLayerActiveChanges,
	journalContextLayerBranchRecency,
	journalContextLayerTasks,
}

type journalContextCLIOptions struct {
	branch     string
	branchSet  bool
	layer      string
	limit      int
	limitSet   bool
	cursor     string
	jsonOutput bool
	fromHook   bool
}

type journalContextLayersJSON struct {
	ProjectSynthesis   *state.JournalContextJournalLayer    `json:"project_synthesis,omitempty"`
	ScopedCheckpoint   *state.JournalContextCheckpointLayer `json:"scoped_checkpoint,omitempty"`
	ActiveLineage      *state.JournalContextJournalLayer    `json:"active_lineage,omitempty"`
	UnresolvedBlockers *state.JournalContextBlockerLayer    `json:"unresolved_blockers,omitempty"`
	DeferredIntent     *state.JournalContextDeferralLayer   `json:"deferred_intent,omitempty"`
	ActiveChanges      *activeChangeLayer                   `json:"active_changes,omitempty"`
	BranchRecency      *state.JournalContextJournalLayer    `json:"branch_recency,omitempty"`
	TransitionalTasks  *state.JournalContextTaskLayer       `json:"transitional_tasks,omitempty"`
}

type journalContextCLIResult struct {
	ContractVersion    int                              `json:"contract_version"`
	Command            string                           `json:"command"`
	DatabaseScope      string                           `json:"database_scope"`
	DatabasePath       string                           `json:"database_path"`
	ProjectID          string                           `json:"project_id"`
	ProjectName        string                           `json:"project_name"`
	ProjectCurrentPath string                           `json:"project_current_path"`
	JournalWatermark   int64                            `json:"journal_watermark"`
	Branch             string                           `json:"branch"`
	Layers             journalContextLayersJSON         `json:"layers"`
	Diagnostics        []state.JournalContextDiagnostic `json:"diagnostics"`
}

func parseJournalContextArgs(args []string) (journalContextCLIOptions, error) {
	options := journalContextCLIOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--from-hook":
			options.fromHook = true
		case "--branch", "--layer", "--limit", "--cursor":
			flag := args[i]
			value, err := consumeFlagValue(args, &i, flag)
			if err != nil {
				return journalContextCLIOptions{}, err
			}
			switch flag {
			case "--branch":
				options.branch, options.branchSet = value, true
			case "--layer":
				options.layer = value
			case "--limit":
				limit, err := strconv.Atoi(value)
				if err != nil || limit < 1 || limit > 100 {
					return journalContextCLIOptions{}, fmt.Errorf("--limit must be an integer from 1 to 100")
				}
				options.limit, options.limitSet = limit, true
			case "--cursor":
				options.cursor = value
			}
		default:
			return journalContextCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if options.layer != "" && !journalContextContains(canonicalJournalContextLayers, options.layer) {
		return journalContextCLIOptions{}, fmt.Errorf("unknown journal context layer %q (use %s)", options.layer, strings.Join(canonicalJournalContextLayers, ", "))
	}
	if options.layer == "" && options.limitSet {
		return journalContextCLIOptions{}, fmt.Errorf("--limit requires --layer")
	}
	if options.layer == "" && options.cursor != "" {
		return journalContextCLIOptions{}, fmt.Errorf("--cursor requires --layer")
	}
	if (options.layer == journalContextLayerProjectSynthesis || options.layer == journalContextLayerScopedCheckpoint) && options.cursor != "" {
		return journalContextCLIOptions{}, fmt.Errorf("--cursor is not supported for intrinsic one-item layer %s", options.layer)
	}
	return options, nil
}

func journalContextContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// runJournalContextDigest emits the layered continuity digest. hookInvocation is
// true when reached from a harness hook (SessionStart --from-hook or PostCompact
// for-resumption): on those paths a missing or uninitialized state database must
// never fail the harness, so it emits a single non-blocking diagnostic line and
// exits 0. Non-hook (interactive) invocations keep the current error behavior.
func (r Runner) runJournalContextDigest(args []string, out io.Writer, runtime state.Runtime, hookInvocation bool) error {
	// Hook subcommands: SessionStart/PostCompact emit the layered digest, while
	// UserPromptSubmit and PreCompact inject guidance text. All preserve the
	// subagent silent-exit guard.
	if len(args) > 0 {
		switch args[0] {
		case "for-prompt":
			return r.runJournalContextForPrompt(out)
		case "for-compact":
			return r.runJournalContextForCompact(out)
		case "for-resumption":
			return r.runJournalContextResumption(out, runtime)
		}
	}
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalContextArgs(args)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, "journal context", err)
		}
		return err
	}
	// SessionStart/PostCompact pass --from-hook so the digest can guard against
	// subagent invocations: read the hook payload and exit silently (writing
	// nothing) when an agent_id is present.
	if options.fromHook {
		hookInvocation = true
		hookInput, err := r.readJournalHookInput()
		if err != nil {
			if jsonRequested {
				return writeJSONCommandError(out, "journal context", err)
			}
			return err
		}
		if hookInput.AgentID != "" {
			return nil
		}
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal context", err)
		}
		return err
	}
	changeSource, changeSourceErr := discoverActiveChanges(projectRoot.Path())
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
	result, err := state.JournalContextForRoot(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.JournalContextOptions{
		Branch: stateOptions.Branch, LineageKeys: stateOptions.LineageKeys, LineageLimit: stateOptions.LineageLimit,
		BlockerLimit: stateOptions.BlockerLimit, DeferralLimit: stateOptions.DeferralLimit, BranchLimit: stateOptions.BranchLimit,
		TaskLimit: stateOptions.TaskLimit, Cursor: stateOptions.Cursor, CursorLayer: stateOptions.CursorLayer,
	})
	if err != nil {
		// On hook paths (SessionStart/PostCompact) a fresh install with no state
		// database yet must never fail the harness: emit one non-blocking
		// diagnostic line and exit 0. Interactive invocations keep erroring.
		if hookInvocation && isStateMissingError(err) {
			if !options.jsonOutput {
				fmt.Fprintln(out, "  loaf journal context: no journal yet (run `loaf state init` to start recording)")
			}
			return nil
		}
		if options.jsonOutput {
			return writeJSONCommandError(out, "journal context", err)
		}
		return r.withStateMissingContext(err, projectRoot)
	}
	activeLayer := unavailableActiveChanges()
	if changeSourceErr == nil {
		activeLimit := defaultCLIJournalContextLimit
		if options.layer == journalContextLayerActiveChanges && options.limitSet {
			activeLimit = options.limit
		}
		activeLayer, err = activeChangesPage(changeSource, result.ProjectID, options.branch, activeLimit, func() string {
			if options.layer == journalContextLayerActiveChanges {
				return options.cursor
			}
			return ""
		}())
		if err != nil {
			if options.jsonOutput {
				return writeJSONCommandError(out, "journal context", err)
			}
			return err
		}
	} else {
		result.ActiveLineage.Available = false
		result.ActiveLineage.AvailableCount = 0
		result.ActiveLineage.ShownCount = 0
		result.ActiveLineage.Truncated = false
		result.ActiveLineage.Cursor = ""
		result.ActiveLineage.Items = []state.JournalEntryRecord{}
		result.Diagnostics = append(result.Diagnostics, state.JournalContextDiagnostic{Code: changeSourceUnavailableCode, Message: "Change source unavailable: " + changeSourceErr.Error() + "; active-changes and active-lineage could not be derived"})
	}
	rewriteJournalContextExpandCommands(&result, &activeLayer, options)
	output := composeJournalContextCLIResult(result, activeLayer, options.layer)
	if options.jsonOutput {
		return writeJSON(out, output)
	}
	writeJournalContextHuman(out, output)
	return nil
}

func journalContextStateLayer(layer string) string {
	switch layer {
	case journalContextLayerProjectSynthesis:
		return state.JournalContextLayerSynthesis
	case journalContextLayerScopedCheckpoint:
		return state.JournalContextLayerCheckpoint
	case journalContextLayerActiveLineage:
		return state.JournalContextLayerLineage
	case journalContextLayerBlockers:
		return state.JournalContextLayerBlockers
	case journalContextLayerDeferredIntent:
		return state.JournalContextLayerDeferrals
	case journalContextLayerBranchRecency:
		return state.JournalContextLayerBranch
	case journalContextLayerTasks:
		return state.JournalContextLayerTasks
	default:
		return ""
	}
}

func setJournalContextStateLimit(options *state.JournalContextOptions, layer string, limit int) {
	switch layer {
	case journalContextLayerActiveLineage:
		options.LineageLimit = limit
	case journalContextLayerBlockers:
		options.BlockerLimit = limit
	case journalContextLayerDeferredIntent:
		options.DeferralLimit = limit
	case journalContextLayerBranchRecency:
		options.BranchLimit = limit
	case journalContextLayerTasks:
		options.TaskLimit = limit
	}
}

func journalContextExpandCommand(layer, cursor, branch string, limit int) string {
	command := "loaf journal context --layer " + layer
	if limit > 0 {
		command += " --limit " + strconv.Itoa(limit)
	}
	if branch != "" {
		command += " --branch " + journalContextShellQuote(branch)
	}
	if cursor != "" {
		command += " --cursor " + journalContextShellQuote(cursor)
	}
	return command
}

func journalContextShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func rewriteJournalContextExpandCommands(result *state.JournalContext, active *activeChangeLayer, options journalContextCLIOptions) {
	limit := defaultCLIJournalContextLimit
	if options.limitSet {
		limit = options.limit
	}
	result.ProjectSynthesis.ExpandCommand = journalContextExpandCommand(journalContextLayerProjectSynthesis, "", options.branch, 1)
	result.LatestCheckpoint.ExpandCommand = journalContextExpandCommand(journalContextLayerScopedCheckpoint, "", options.branch, 1)
	result.ActiveLineage.ExpandCommand = journalContextExpandCommand(journalContextLayerActiveLineage, result.ActiveLineage.Cursor, options.branch, limit)
	result.UnresolvedBlockers.ExpandCommand = journalContextExpandCommand(journalContextLayerBlockers, result.UnresolvedBlockers.Cursor, options.branch, limit)
	result.DeferredIntent.ExpandCommand = journalContextExpandCommand(journalContextLayerDeferredIntent, result.DeferredIntent.Cursor, options.branch, limit)
	active.ExpandCommand = journalContextExpandCommand(journalContextLayerActiveChanges, active.Cursor, options.branch, limit)
	result.BranchRecency.ExpandCommand = journalContextExpandCommand(journalContextLayerBranchRecency, result.BranchRecency.Cursor, options.branch, limit)
	result.TransitionalTasks.ExpandCommand = journalContextExpandCommand(journalContextLayerTasks, result.TransitionalTasks.Cursor, options.branch, limit)
}

func composeJournalContextCLIResult(result state.JournalContext, active activeChangeLayer, selected string) journalContextCLIResult {
	output := journalContextCLIResult{
		ContractVersion:    state.JournalContextContractVersion,
		Command:            "journal context",
		DatabaseScope:      result.DatabaseScope,
		DatabasePath:       result.DatabasePath,
		ProjectID:          result.ProjectID,
		ProjectName:        result.ProjectName,
		ProjectCurrentPath: result.ProjectCurrentPath,
		JournalWatermark:   result.JournalWatermark,
		Branch:             result.Branch,
		Diagnostics:        append([]state.JournalContextDiagnostic(nil), result.Diagnostics...),
	}
	if output.Diagnostics == nil {
		output.Diagnostics = []state.JournalContextDiagnostic{}
	}
	include := func(layer string) bool { return selected == "" || selected == layer }
	if include(journalContextLayerProjectSynthesis) {
		output.Layers.ProjectSynthesis = &result.ProjectSynthesis
	}
	if include(journalContextLayerScopedCheckpoint) {
		output.Layers.ScopedCheckpoint = &result.LatestCheckpoint
	}
	if include(journalContextLayerActiveLineage) {
		output.Layers.ActiveLineage = &result.ActiveLineage
	}
	if include(journalContextLayerBlockers) {
		output.Layers.UnresolvedBlockers = &result.UnresolvedBlockers
	}
	if include(journalContextLayerDeferredIntent) {
		output.Layers.DeferredIntent = &result.DeferredIntent
	}
	if include(journalContextLayerActiveChanges) {
		output.Layers.ActiveChanges = &active
	}
	if include(journalContextLayerBranchRecency) {
		output.Layers.BranchRecency = &result.BranchRecency
	}
	if include(journalContextLayerTasks) {
		output.Layers.TransitionalTasks = &result.TransitionalTasks
	}
	return output
}

func writeJournalContextHuman(out io.Writer, result journalContextCLIResult) {
	fmt.Fprint(out, "\n  loaf journal context\n\n")
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.Branch != "" {
		fmt.Fprintf(out, "  branch: %s\n", result.Branch)
	} else {
		fmt.Fprintln(out, "  branch: detached or unavailable")
	}
	if layer := result.Layers.ProjectSynthesis; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerProjectSynthesis, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, entry := range layer.Items {
			fmt.Fprintf(out, "    %s\n", formatJournalEntryLine(entry))
		}
	}
	if layer := result.Layers.ScopedCheckpoint; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerScopedCheckpoint, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, item := range layer.Items {
			fmt.Fprintf(out, "    %s: %s\n", item.Label, formatJournalEntryLine(item.Entry))
		}
	}
	if layer := result.Layers.ActiveLineage; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerActiveLineage, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, entry := range layer.Items {
			fmt.Fprintf(out, "    %s\n", formatJournalEntryLine(entry))
		}
	}
	if layer := result.Layers.UnresolvedBlockers; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerBlockers, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, item := range layer.Items {
			fmt.Fprintf(out, "    %s: %s\n", item.Key, formatJournalEntryLine(item.Block))
		}
	}
	if layer := result.Layers.DeferredIntent; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerDeferredIntent, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, item := range layer.Items {
			fmt.Fprintf(out, "    %s: %s\n", item.OperationKey, item.Spark.Text)
		}
	}
	if layer := result.Layers.ActiveChanges; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerActiveChanges, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, item := range layer.Items {
			writeActiveChangeItemHuman(out, item)
		}
	}
	if layer := result.Layers.BranchRecency; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerBranchRecency, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, entry := range layer.Items {
			fmt.Fprintf(out, "    %s\n", formatJournalEntryLine(entry))
		}
	}
	if layer := result.Layers.TransitionalTasks; layer != nil {
		writeJournalLayerHuman(out, journalContextLayerTasks, layer.Available, layer.AvailableCount, layer.ShownCount, layer.ExpandCommand)
		for _, task := range layer.Items {
			fmt.Fprintf(out, "    %s (%s): %s\n", task.Ref, task.Status, task.Title)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(out, "  diagnostic %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
}

func writeJournalLayerHuman(out io.Writer, name string, available bool, availableCount, shownCount int, expandCommand string) {
	if !available {
		fmt.Fprintf(out, "  %s: unavailable\n", name)
		fmt.Fprintf(out, "    more: %s\n", expandCommand)
		return
	}
	if availableCount == 0 {
		fmt.Fprintf(out, "  %s: none\n", name)
		fmt.Fprintf(out, "    more: %s\n", expandCommand)
		return
	}
	fmt.Fprintf(out, "  %s: showing %d of %d\n", name, shownCount, availableCount)
	fmt.Fprintf(out, "    more: %s\n", expandCommand)
}

func (r Runner) runJournalExport(args []string, out io.Writer, runtime state.Runtime) error {
	format := state.ExportFormatMarkdown
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format":
			value, err := consumeFlagValue(args, &i, "--format")
			if err != nil {
				return err
			}
			format = value
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	var result state.MarkdownExport
	switch format {
	case state.ExportFormatMarkdown:
		result, err = state.ExportJournalMarkdown(context.Background(), projectRoot, resolver)
	case state.ExportFormatJSONL:
		result, err = state.ExportJournalJSONL(context.Background(), projectRoot, resolver)
	default:
		return fmt.Errorf("journal export format %q is not supported (use markdown or jsonl)", format)
	}
	if err != nil {
		return r.withStateMissingContext(err, projectRoot)
	}
	fmt.Fprint(out, result.Content)
	return nil
}

func formatJournalEntryLine(entry state.JournalEntryRecord) string {
	label := entry.EntryType
	if entry.Scope != "" {
		label = fmt.Sprintf("%s(%s)", entry.EntryType, entry.Scope)
	}
	line := fmt.Sprintf("[%s] %s: %s", formatJournalTimestamp(entry.CreatedAt), label, entry.Message)
	if entry.ObservedBranch != "" {
		line += fmt.Sprintf(" (branch: %s)", entry.ObservedBranch)
	}
	return line
}

func formatJournalTimestamp(value string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format("2006-01-02 15:04")
	}
	if len(value) >= 16 {
		return strings.ReplaceAll(value[:16], "T", " ")
	}
	return value
}

func parsePositiveLimit(value string) (int, error) {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 0, fmt.Errorf("--limit must be a positive integer")
	}
	return limit, nil
}

func writeStateMigrateJournalFirstHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate journal-first [--dry-run|--apply] [--json]", "Transform the global database to the journal-first model: purge lifecycle noise, drop the session entity, rekey journal search. Destructive by consent.",
		"--dry-run  Preview counts against a temporary database copy (no mutation, no backup)",
		"--apply    Take a mandatory backup, then apply the migration to the live database",
		"--json     Output migration contract, counts, backup path, and schema version as JSON")
}

func writeMigrateJournalFirstHelp(out io.Writer) {
	writeUsageHelp(out, "loaf migrate journal-first [--dry-run|--apply] [--json]", "Transform the global database to the journal-first model: purge lifecycle noise, drop the session entity, rekey journal search. Destructive by consent.",
		"--dry-run  Preview counts against a temporary database copy (no mutation, no backup)",
		"--apply    Take a mandatory backup, then apply the migration to the live database",
		"--json     Output migration contract, counts, backup path, and schema version as JSON")
}

type journalFirstMigrationOptions struct {
	dryRun     bool
	apply      bool
	jsonOutput bool
}

func parseJournalFirstMigrationArgs(args []string, command string) (journalFirstMigrationOptions, error) {
	var options journalFirstMigrationOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			options.dryRun = true
		case "--apply":
			options.apply = true
		case "--json":
			options.jsonOutput = true
		default:
			return journalFirstMigrationOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if options.apply && options.dryRun {
		return journalFirstMigrationOptions{}, fmt.Errorf("%s cannot combine --apply and --dry-run", command)
	}
	if !options.apply && !options.dryRun {
		return journalFirstMigrationOptions{}, fmt.Errorf("%s requires --dry-run or --apply", command)
	}
	return options, nil
}

func (r Runner) runJournalFirstMigration(args []string, out io.Writer, runtime state.Runtime, displayCommand string) error {
	command := strings.TrimPrefix(displayCommand, "loaf ")
	jsonRequested := hasFlag(args, "--json")
	options, err := parseJournalFirstMigrationArgs(args, command)
	if err != nil {
		if jsonRequested {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	var result state.JournalFirstMigrationResult
	if options.apply {
		result, err = state.ApplyJournalFirstMigration(context.Background(), projectRoot, resolver)
	} else {
		result, err = state.PreviewJournalFirstMigration(context.Background(), projectRoot, resolver)
	}
	if err != nil {
		if options.jsonOutput {
			return writeJSONCommandError(out, command, err)
		}
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "\n  %s\n\n", displayCommand)
	writeProjectMutationContext(out, "  ", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if result.CopyRun {
		fmt.Fprintln(out, "  mode: dry-run (temporary database copy, live database untouched)")
	} else {
		fmt.Fprintln(out, "  mode: applied to live database")
		if result.BackupPath != "" {
			fmt.Fprintf(out, "  backup: %s\n", result.BackupPath)
		}
	}
	fmt.Fprintf(out, "  journal entries: %d -> %d\n", result.JournalEntriesBefore, result.JournalEntriesAfter)
	fmt.Fprintf(out, "  noise purged: %d\n", result.NoiseEntriesPurged)
	for _, family := range result.PurgeBreakdown {
		if family.Count > 0 {
			fmt.Fprintf(out, "    %s: %d\n", family.Family, family.Count)
		}
	}
	fmt.Fprintf(out, "  session rows preserved as legacy_session: %d\n", result.SessionRowsPreservedAsLegacy)
	fmt.Fprintf(out, "  harness ids backfilled: %d\n", result.HarnessSessionBackfill)
	fmt.Fprintf(out, "  sessions dropped: %d\n", result.SessionsDropped)
	fmt.Fprintf(out, "  session events deleted: %d\n", result.SessionEventsDeleted)
	fmt.Fprintf(out, "  session aliases deleted: %d\n", result.SessionAliasesDeleted)
	fmt.Fprintf(out, "  journal search rows: %d\n", result.JournalSearchRows)
	fmt.Fprintf(out, "  schema version: %d\n", result.SchemaVersion)
	return nil
}
