package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issueAbsorbOptions struct {
	jsonOutput bool
	dismiss    bool
	ref        string
}

func writeIssueAbsorbHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue absorb <task-or-intent-ref> [--dismiss] [--json]", "Mint a fresh issue from a leftover task or intent, archive the source as absorbed, and keep TASK-*/INTENT-* from becoming a live issue alias. --dismiss archives the source as superseded without minting.",
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
	if strings.TrimSpace(options.ref) == "" {
		return issueAbsorbOptions{}, fmt.Errorf("issue absorb requires a task or intent ref")
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
