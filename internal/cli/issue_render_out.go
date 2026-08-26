package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issueRenderOutOptions struct {
	ref          string
	branch       string
	manualExport string
	retire       bool
	dryRun       bool
	jsonOutput   bool
}

func writeIssueRenderOutHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue render-out <ref> [--branch <name>] [--export <path>] [--retire] [--dry-run] [--json]",
		"Render an internal issue row onto a durable authority ref without retiring unless --retire and a receipt was written.",
		"--branch   Trackerless branch ref key (default: issue started branch or issue/loaf-<n>)",
		"--export   Manual-export artifact path for trackerless projects",
		"--retire   Archive the internal row after recording the export receipt",
		"--dry-run  Preview the target authority ref and contract body",
		"--json     Output JSON")
}

func (r Runner) runIssueRenderOut(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueRenderOutArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue render-out", runtime)
	if err != nil {
		return err
	}
	result, err := state.RenderOutIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.IssueRenderOutOptions{
		Ref:          options.ref,
		Branch:       options.branch,
		ManualExport: options.manualExport,
		Retire:       options.retire,
		DryRun:       options.dryRun,
	})
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "%s %s\n", result.Action, result.AuthorityRef.String())
	if result.Retired {
		fmt.Fprintf(out, "retired %s\n", firstNonEmpty(result.Issue.Alias, result.Issue.ID))
	}
	return nil
}

func parseIssueRenderOutArgs(args []string) (issueRenderOutOptions, error) {
	options := issueRenderOutOptions{}
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--branch":
			value, err := consumeFlagValue(args, &i, "--branch")
			if err != nil {
				return issueRenderOutOptions{}, err
			}
			options.branch = value
		case "--export":
			value, err := consumeFlagValue(args, &i, "--export")
			if err != nil {
				return issueRenderOutOptions{}, err
			}
			options.manualExport = value
		case "--retire":
			options.retire = true
		case "--dry-run":
			options.dryRun = true
		default:
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return issueRenderOutOptions{}, fmt.Errorf("issue render-out requires exactly one issue ref")
	}
	options.ref = strings.TrimSpace(positional[0])
	return options, nil
}
