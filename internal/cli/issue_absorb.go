package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type issueAbsorbOptions struct {
	jsonOutput bool
	dismiss    bool
	all        bool
	history    bool
	dryRun     bool
	ref        string
}

func writeIssueAbsorbHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue absorb <task-or-intent-ref> [--dismiss] [--json]\n       loaf issue absorb --all [--history] [--dry-run] [--dismiss] [--json]", "Mint a fresh issue from leftover SQLite work, archive the source as absorbed, and keep TASK-*/INTENT-* from becoming a live issue alias. A single ref stays leftover-open only. --all projects open tasks and non-terminal intents; --history also mints done tasks as done issues and archived tasks as cancelled issues, and refuses when the project already has independently created issues. --dismiss archives the source as superseded without minting.",
		"--all        Project every leftover row in scope for the current project",
		"--history    Include done and archived tasks, and ordinarily resolved intents",
		"--dry-run    Rehearse --all without writing",
		"--dismiss    Archive the source as superseded without minting an issue",
		"--json       Output the absorb result, global database scope, and project identity as JSON")
}

func parseIssueAbsorbArgs(args []string) (issueAbsorbOptions, error) {
	var options issueAbsorbOptions
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--dismiss":
			options.dismiss = true
		case "--all":
			options.all = true
		case "--history":
			options.history = true
		case "--dry-run":
			options.dryRun = true
		default:
			if strings.HasPrefix(arg, "-") {
				return issueAbsorbOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			if options.ref != "" {
				return issueAbsorbOptions{}, fmt.Errorf("issue absorb accepts exactly one argument")
			}
			options.ref = arg
		}
	}
	if options.history && !options.all {
		return issueAbsorbOptions{}, fmt.Errorf("issue absorb --history requires --all")
	}
	if options.dryRun && !options.all {
		return issueAbsorbOptions{}, fmt.Errorf("issue absorb --dry-run requires --all")
	}
	if options.dismiss && options.history {
		return issueAbsorbOptions{}, fmt.Errorf("issue absorb --dismiss cannot be combined with --history")
	}
	if options.all && strings.TrimSpace(options.ref) != "" {
		return issueAbsorbOptions{}, fmt.Errorf("issue absorb --all does not accept a task or intent ref")
	}
	if !options.all && strings.TrimSpace(options.ref) == "" {
		return issueAbsorbOptions{}, fmt.Errorf("issue absorb requires a task or intent ref, or --all")
	}
	return options, nil
}

func (r Runner) runIssueAbsorb(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueAbsorbArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue absorb", runtime)
	if err != nil {
		return err
	}
	if options.all {
		return r.runIssueAbsorbAll(out, projectRoot, options)
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	source, err := state.LookupAbsorbSource(context.Background(), projectRoot, resolver, options.ref)
	if err != nil {
		return err
	}

	absorb := state.AbsorbOptions{Ref: options.ref, Dismiss: options.dismiss}
	var minted *mintedLinearIssue
	if !options.dismiss {
		alias, mintedIssue, err := r.mintIssueIdentity(projectRoot, resolver, state.IssueCreateOptions{
			Title: source.Title,
			Body:  state.FormatAbsorbProvenance(source),
		})
		if err != nil {
			return err
		}
		absorb.Alias = alias
		minted = mintedIssue
	}

	result, err := state.Absorb(context.Background(), projectRoot, resolver, absorb)
	if err != nil {
		if minted != nil {
			return &state.LinearOrphanError{Identifier: minted.Identifier, URL: minted.URL, Err: err}
		}
		return err
	}
	if result.Issue != nil {
		if err := r.bindMintedLinearIssue(projectRoot, resolver, result.Issue.ID, minted); err != nil {
			return err
		}
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	writeIssueAbsorb(out, result)
	return nil
}

func (r Runner) runIssueAbsorbAll(out io.Writer, projectRoot project.Root, options issueAbsorbOptions) error {
	resolver := state.PathResolver{StateHome: r.StateHome}
	planOptions := state.AbsorbAllOptions{History: options.history, Dismiss: options.dismiss, DryRun: options.dryRun}
	plan, err := state.PlanAbsorbAll(context.Background(), projectRoot, resolver, planOptions)
	if err != nil {
		return err
	}
	if options.dryRun {
		plan.DryRun = true
		if options.jsonOutput {
			return writeJSON(out, plan)
		}
		writeIssueAbsorbAll(out, plan)
		return nil
	}
	for i, item := range plan.Items {
		if item.Action != state.AbsorbActionAbsorb && item.Action != state.AbsorbActionDismiss {
			continue
		}
		absorb := state.AbsorbOptions{
			Ref:         firstNonEmpty(item.Source.Alias, item.Source.ID),
			Dismiss:     item.Action == state.AbsorbActionDismiss,
			History:     options.history,
			IssueStatus: item.IssueStatus,
		}
		var minted *mintedLinearIssue
		if item.Action == state.AbsorbActionAbsorb {
			alias, mintedIssue, err := r.mintIssueIdentity(projectRoot, resolver, state.IssueCreateOptions{
				Title: item.Source.Title,
				Body:  state.FormatAbsorbProvenance(item.Source),
			})
			if err != nil {
				return err
			}
			absorb.Alias = alias
			minted = mintedIssue
		}
		result, err := state.Absorb(context.Background(), projectRoot, resolver, absorb)
		if err != nil {
			if minted != nil {
				return &state.LinearOrphanError{Identifier: minted.Identifier, URL: minted.URL, Err: err}
			}
			return err
		}
		if result.Issue != nil {
			if err := r.bindMintedLinearIssue(projectRoot, resolver, result.Issue.ID, minted); err != nil {
				return err
			}
		}
		plan.Items[i].Source = result.Source
		plan.Items[i].Issue = result.Issue
	}
	if options.jsonOutput {
		return writeJSON(out, plan)
	}
	writeIssueAbsorbAll(out, plan)
	return nil
}

func writeIssueAbsorb(out io.Writer, result state.AbsorbResult) {
	sourceRef := firstNonEmpty(result.Source.Alias, result.Source.ID, result.Source.DisplayRef)
	if result.Dismiss {
		fmt.Fprintf(out, "dismissed %s as superseded\n", sourceRef)
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
		fmt.Fprintf(out, "source: %s:%s\n", result.Source.Kind, result.Source.ID)
		return
	}
	if result.Issue == nil {
		fmt.Fprintf(out, "absorbed %s\n", sourceRef)
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
		return
	}
	fmt.Fprintf(out, "absorbed %s into %s\n", sourceRef, issueDisplayRef(*result.Issue))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	fmt.Fprintf(out, "title: %s\n", result.Issue.Title)
	fmt.Fprintf(out, "kind: %s\n", result.Issue.Kind)
	fmt.Fprintf(out, "status: %s\n", result.Issue.Status)
	fmt.Fprintf(out, "source: %s:%s\n", result.Source.Kind, result.Source.ID)
}

func writeIssueAbsorbAll(out io.Writer, result state.AbsorbProjectionResult) {
	parts := []string{}
	if result.DryRun {
		parts = append(parts, fmt.Sprintf("would absorb %d", result.Absorbed))
		if result.Dismissed > 0 {
			parts = append(parts, fmt.Sprintf("would dismiss %d", result.Dismissed))
		}
	} else {
		parts = append(parts, fmt.Sprintf("absorbed %d", result.Absorbed))
		if result.Dismissed > 0 {
			parts = append(parts, fmt.Sprintf("dismissed %d", result.Dismissed))
		}
	}
	if result.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("skipped %d", result.Skipped))
	}
	if result.Refused > 0 {
		parts = append(parts, fmt.Sprintf("refused %d", result.Refused))
	}
	fmt.Fprintf(out, "%s leftover source(s)\n", strings.Join(parts, ", "))
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	for _, item := range result.Items {
		ref := firstNonEmpty(item.Source.Alias, item.Source.ID, item.Source.DisplayRef)
		switch item.Action {
		case state.AbsorbActionAbsorb:
			if item.Issue != nil {
				fmt.Fprintf(out, "  absorbed %s into %s (%s)\n", ref, issueDisplayRef(*item.Issue), item.Issue.Status)
				continue
			}
			fmt.Fprintf(out, "  absorb %s -> %s\n", ref, item.IssueStatus)
		case state.AbsorbActionDismiss:
			fmt.Fprintf(out, "  dismiss %s\n", ref)
		case state.AbsorbActionSkip:
			fmt.Fprintf(out, "  skip %s %s\n", ref, item.Reason)
		case state.AbsorbActionRefuse:
			fmt.Fprintf(out, "  refuse %s %s\n", ref, item.Reason)
		}
	}
}
