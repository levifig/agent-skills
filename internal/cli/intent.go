package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func (r Runner) runIntent(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeIntentHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"create":  writeIntentCreateHelp,
		"defer":   writeIntentDeferHelp,
		"resume":  writeIntentResumeHelp,
		"resolve": writeIntentResolveHelp,
		"show":    writeIntentShowHelp,
		"list":    writeIntentListHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "create":
		return r.runIntentCreate(args[1:], out, runtime)
	case "defer":
		return r.runIntentDefer(args[1:], out, runtime)
	case "resume":
		return r.runIntentResume(args[1:], out, runtime)
	case "resolve":
		return r.runIntentResolve(args[1:], out, runtime)
	case "show":
		return r.runIntentShow(args[1:], out, runtime)
	case "list":
		return r.runIntentList(args[1:], out, runtime)
	default:
		return unknownSubcommandError("intent", args[0])
	}
}

func writeIntentHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf intent <subcommand> [options]", "Manage tracked Intent in native SQLite state. Disposition is derived from append-only facts; there is no mutable lifecycle status.", []subcommandHelpItem{
		{Name: "create", Summary: "Create a tracked or deferred Intent"},
		{Name: "defer", Summary: "Defer an existing Intent with an immutable payload"},
		{Name: "resume", Summary: "Append a tracked disposition superseding the current deferral"},
		{Name: "resolve", Summary: "Append a reasoned terminal disposition"},
		{Name: "show", Summary: "Show one Intent with derived disposition"},
		{Name: "list", Summary: "List Intents with derived dispositions"},
	})
}

func writeIntentCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intent create --title <title> --body <body> [--disposition deferred --why <why> --boundary <boundary> --trigger <trigger> --operation-id <key>] [--from <source>]... [--reason <reason>] [--operation-id <key>] [--json]", "Create one Intent snapshot plus its initial disposition in one transaction.",
		"--title          Bounded single-line title",
		"--body           Self-sufficient body",
		"--disposition    tracked (default) or deferred",
		"--why            Why the deferred direction matters",
		"--boundary       What excluded it now",
		"--trigger        When to revisit",
		"--operation-id   Retry-safe operation key (required when deferred)",
		"--from           Source entity reference (spark, idea, brainstorm, or journal entry); repeatable",
		"--reason         Optional reason recorded with the initial disposition",
		"--json           Output the created or reused Intent, digests, and project identity as JSON")
}

func writeIntentDeferHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intent defer <intent> --why <why> --boundary <boundary> --trigger <trigger> --operation-id <key> [--json]", "Append an immutable deferral to an existing Intent.",
		"--why            Why the direction matters",
		"--boundary       What excluded it now",
		"--trigger        When to revisit",
		"--operation-id   Retry-safe operation key",
		"--json           Output the deferred Intent, digests, and project identity as JSON")
}

func writeIntentResumeHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intent resume <intent> --reason <why now> [--json]", "Append a tracked disposition linked to the deferral it supersedes.",
		"--reason     Why the Intent is tracked again",
		"--json       Output the resumed Intent and project identity as JSON")
}

func writeIntentResolveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intent resolve <intent> --reason <outcome> [--json]", "Append a reasoned terminal disposition; history is never overwritten.",
		"--reason     Resolution outcome",
		"--json       Output the resolved Intent and project identity as JSON")
}

func writeIntentShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intent show <intent> [--json]", "Show one Intent: latest snapshot, derived disposition, deferral payload, and sources.",
		"--json       Output Intent detail, sources, and project identity as JSON")
}

func writeIntentListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intent list [--disposition tracked|deferred|resolved] [--json]", "List Intents with derived dispositions in deterministic order.",
		"--disposition  Filter by derived disposition",
		"--json         Output Intents and project identity as JSON")
}

type intentCreateCLIOptions struct {
	create     state.IntentCreateOptions
	jsonOutput bool
}

func parseIntentCreateArgs(args []string) (intentCreateCLIOptions, error) {
	options := intentCreateCLIOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Title = value
		case "--body":
			value, err := consumeFlagValue(args, &i, "--body")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Body = value
		case "--disposition":
			value, err := consumeFlagValue(args, &i, "--disposition")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Disposition = value
		case "--why":
			value, err := consumeFlagValue(args, &i, "--why")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Why = value
		case "--boundary":
			value, err := consumeFlagValue(args, &i, "--boundary")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Boundary = value
		case "--trigger":
			value, err := consumeFlagValue(args, &i, "--trigger")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Trigger = value
		case "--operation-id":
			value, err := consumeFlagValue(args, &i, "--operation-id")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.OperationID = value
		case "--reason":
			value, err := consumeFlagValue(args, &i, "--reason")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Reason = value
		case "--from":
			value, err := consumeFlagValue(args, &i, "--from")
			if err != nil {
				return intentCreateCLIOptions{}, err
			}
			options.create.Sources = append(options.create.Sources, value)
		default:
			return intentCreateCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.create.Title) == "" {
		return intentCreateCLIOptions{}, fmt.Errorf("intent create requires --title")
	}
	if strings.TrimSpace(options.create.Body) == "" {
		return intentCreateCLIOptions{}, fmt.Errorf("intent create requires --body")
	}
	return options, nil
}

type intentDeferCLIOptions struct {
	defer_     state.IntentDeferOptions
	jsonOutput bool
}

func parseIntentDeferArgs(args []string) (intentDeferCLIOptions, error) {
	options := intentDeferCLIOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--why":
			value, err := consumeFlagValue(args, &i, "--why")
			if err != nil {
				return intentDeferCLIOptions{}, err
			}
			options.defer_.Why = value
		case "--boundary":
			value, err := consumeFlagValue(args, &i, "--boundary")
			if err != nil {
				return intentDeferCLIOptions{}, err
			}
			options.defer_.Boundary = value
		case "--trigger":
			value, err := consumeFlagValue(args, &i, "--trigger")
			if err != nil {
				return intentDeferCLIOptions{}, err
			}
			options.defer_.Trigger = value
		case "--operation-id":
			value, err := consumeFlagValue(args, &i, "--operation-id")
			if err != nil {
				return intentDeferCLIOptions{}, err
			}
			options.defer_.OperationID = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return intentDeferCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.defer_.IntentRef != "" {
				return intentDeferCLIOptions{}, fmt.Errorf("intent defer accepts one intent reference")
			}
			options.defer_.IntentRef = args[i]
		}
	}
	if options.defer_.IntentRef == "" {
		return intentDeferCLIOptions{}, fmt.Errorf("intent defer requires an intent reference")
	}
	return options, nil
}

type intentDispositionCLIOptions struct {
	disposition state.IntentDispositionOptions
	jsonOutput  bool
}

func parseIntentDispositionArgs(command string, args []string) (intentDispositionCLIOptions, error) {
	options := intentDispositionCLIOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--reason":
			value, err := consumeFlagValue(args, &i, "--reason")
			if err != nil {
				return intentDispositionCLIOptions{}, err
			}
			options.disposition.Reason = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return intentDispositionCLIOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			if options.disposition.IntentRef != "" {
				return intentDispositionCLIOptions{}, fmt.Errorf("%s accepts one intent reference", command)
			}
			options.disposition.IntentRef = args[i]
		}
	}
	if options.disposition.IntentRef == "" {
		return intentDispositionCLIOptions{}, fmt.Errorf("%s requires an intent reference", command)
	}
	if strings.TrimSpace(options.disposition.Reason) == "" {
		return intentDispositionCLIOptions{}, fmt.Errorf("%s requires --reason", command)
	}
	return options, nil
}

func (r Runner) requireIntentSQLiteState(command string, runtime state.Runtime) (project.Root, error) {
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return project.Root{}, err
	}
	status, err := state.Inspect(projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return project.Root{}, err
	}
	switch status.Mode {
	case state.ModeMarkdownOnly:
		return project.Root{}, sqliteStateRequiredError(command)
	case state.ModeInvalid:
		return project.Root{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}
	return projectRoot, nil
}

func (r Runner) runIntentCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIntentCreateArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIntentSQLiteState("intent create", runtime)
	if err != nil {
		return err
	}
	result, err := state.CreateIntent(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.create)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIntentMutation(out, result)
	return nil
}

func (r Runner) runIntentDefer(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIntentDeferArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIntentSQLiteState("intent defer", runtime)
	if err != nil {
		return err
	}
	result, err := state.DeferIntent(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.defer_)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIntentMutation(out, result)
	return nil
}

func (r Runner) runIntentResume(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIntentDispositionArgs("intent resume", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIntentSQLiteState("intent resume", runtime)
	if err != nil {
		return err
	}
	result, err := state.ResumeIntent(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.disposition)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIntentMutation(out, result)
	return nil
}

func (r Runner) runIntentResolve(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIntentDispositionArgs("intent resolve", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIntentSQLiteState("intent resolve", runtime)
	if err != nil {
		return err
	}
	result, err := state.ResolveIntent(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options.disposition)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIntentMutation(out, result)
	return nil
}

func (r Runner) runIntentShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("intent show", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIntentSQLiteState("intent show", runtime)
	if err != nil {
		return err
	}
	result, err := state.ShowIntent(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeIntentDetail(out, result.Intent)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runIntentList(args []string, out io.Writer, runtime state.Runtime) error {
	dispositionFilter := ""
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--disposition":
			value, err := consumeFlagValue(args, &i, "--disposition")
			if err != nil {
				return err
			}
			dispositionFilter = value
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	projectRoot, err := r.requireIntentSQLiteState("intent list", runtime)
	if err != nil {
		return err
	}
	result, err := state.ListIntents(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, dispositionFilter)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	if len(result.Intents) == 0 {
		fmt.Fprintln(out, "no intents found")
	}
	for _, item := range result.Intents {
		ref := item.Alias
		if ref == "" {
			ref = item.ID
		}
		fmt.Fprintf(out, "%s  %-9s %s\n", ref, item.Disposition, item.Title)
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func writeIntentMutation(out io.Writer, result state.IntentMutationResult) {
	if result.Created {
		fmt.Fprintln(out, "created intent")
	} else {
		fmt.Fprintln(out, "reused intent (operation key already established)")
	}
	writeIntentDetail(out, result.Intent)
	if result.OperationID != "" {
		fmt.Fprintf(out, "operation: %s\n", result.OperationID)
		fmt.Fprintf(out, "input digest: %s\n", result.InputDigest)
		fmt.Fprintf(out, "stored digest: %s\n", result.StoredDigest)
		fmt.Fprintf(out, "digest match: %t\n", result.InputDigestMatches)
		if !result.InputDigestMatches {
			fmt.Fprintln(out, "warning: retry input differs from the stored first write; the stored content remains canonical")
		}
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
}

func writeIntentDetail(out io.Writer, intent state.IntentDetail) {
	ref := intent.Alias
	if ref == "" {
		ref = intent.ID
	}
	fmt.Fprintf(out, "intent: %s\n", ref)
	fmt.Fprintf(out, "id: %s\n", intent.ID)
	fmt.Fprintf(out, "title: %s\n", intent.Title)
	fmt.Fprintf(out, "disposition: %s (seq %d)\n", intent.Disposition, intent.DispositionSeq)
	if intent.DispositionReason != "" {
		fmt.Fprintf(out, "reason: %s\n", intent.DispositionReason)
	}
	if intent.Deferral != nil {
		fmt.Fprintf(out, "deferred why: %s\n", intent.Deferral.Why)
		fmt.Fprintf(out, "deferred boundary: %s\n", intent.Deferral.Boundary)
		fmt.Fprintf(out, "deferred trigger: %s\n", intent.Deferral.RevisitTrigger)
	}
	for _, source := range intent.Sources {
		sourceRef := source.Entity.Alias
		if sourceRef == "" {
			sourceRef = source.Entity.ID
		}
		fmt.Fprintf(out, "source: %s %s\n", source.Entity.Kind, sourceRef)
	}
	fmt.Fprintf(out, "read: loaf intent show %s\n", ref)
}

func (r Runner) runIntake(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeIntakeHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"list": writeIntakeListHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "list":
		jsonOutput := false
		for _, arg := range args[1:] {
			if arg == "--json" {
				jsonOutput = true
				continue
			}
			return fmt.Errorf("unknown option %q", arg)
		}
		projectRoot, err := r.requireIntentSQLiteState("intake list", runtime)
		if err != nil {
			return err
		}
		result, err := state.ListIntake(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
		if err != nil {
			return err
		}
		if jsonOutput {
			return writeJSON(out, result)
		}
		if len(result.Items) == 0 {
			fmt.Fprintln(out, "intake is empty")
		}
		for _, item := range result.Items {
			descriptor := item.Status
			if item.Disposition != "" {
				descriptor = item.Disposition
			}
			ref := item.Alias
			if ref == "" {
				ref = item.ID
			}
			fmt.Fprintf(out, "%-15s %-9s %s\n", item.Kind, descriptor, item.Title)
			fmt.Fprintf(out, "  read: %s\n", item.ReadCommand)
			_ = ref
		}
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
		return nil
	default:
		return unknownSubcommandError("intake", args[0])
	}
}

func writeIntakeHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf intake <subcommand> [options]", "Read the deterministic local intake projection. The CLI reports facts; triage judgment stays with humans and Skills.", []subcommandHelpItem{
		{Name: "list", Summary: "List unresolved sparks, ideas, brainstorms, intents, and legacy deferrals"},
	})
}

func writeIntakeListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf intake list [--json]", "Project each unresolved logical item exactly once with provenance and exact read commands; no ranking, promotion, or disposition is chosen.",
		"--json       Output intake items and project identity as JSON")
}

func writeStateMigrateDeferralsHelp(out io.Writer) {
	writeUsageHelp(out, "loaf state migrate deferrals [--dry-run|--apply] [--json]", "Convert historical journal deferrals into canonical deferred Intents. Dry-run writes nothing; apply verifies a whole-database backup first, preserves every legacy row, links historical decision/spark provenance, and is rerunnable.",
		"--dry-run    Report the project-specific conversion manifest without writing",
		"--apply      Convert after creating and verifying a whole-database backup",
		"--json       Output the conversion manifest, counts, backup, and project identity as JSON")
}

func (r Runner) runStateMigrateDeferrals(args []string, out io.Writer, runtime state.Runtime) error {
	dryRun := false
	apply := false
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--apply":
			apply = true
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}
	if dryRun == apply {
		return fmt.Errorf("state migrate deferrals requires exactly one of --dry-run or --apply")
	}
	projectRoot, err := r.requireIntentSQLiteState("state migrate deferrals", runtime)
	if err != nil {
		return err
	}
	result, err := state.ConvertLegacyDeferrals(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, apply)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "action: %s\n", result.Action)
	if result.BackupPath != "" {
		fmt.Fprintf(out, "backup: %s (verified: %t)\n", result.BackupPath, result.BackupVerified)
	}
	fmt.Fprintf(out, "convertible: %d\nalready converted: %d\nunparseable: %d\n", result.Convertible, result.AlreadyConverted, result.Unparseable)
	for _, row := range result.Rows {
		line := fmt.Sprintf("%s  %s", row.Action, row.OperationKey)
		if row.IntentAlias != "" {
			line += "  -> " + row.IntentAlias
		} else if row.IntentID != "" {
			line += "  -> " + row.IntentID
		}
		if row.Reason != "" {
			line += "  (" + row.Reason + ")"
		}
		fmt.Fprintln(out, line)
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}
