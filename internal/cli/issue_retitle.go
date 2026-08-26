package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issueRetitleOptions struct {
	jsonOutput bool
	ref        string
	title      string
}

func writeIssueRetitleHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue retitle <ref> <title> [--json]", "Replace an issue title. The body stays on loaf issue edit.",
		"--json       Output the retitled issue, global database scope, and project identity as JSON")
}

func parseIssueRetitleArgs(args []string) (issueRetitleOptions, error) {
	var options issueRetitleOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return issueRetitleOptions{}, fmt.Errorf("unknown issue retitle option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return issueRetitleOptions{}, fmt.Errorf("issue retitle requires an issue ref and a title")
	}
	options.ref = positional[0]
	options.title = positional[1]
	return options, nil
}

func (r Runner) runIssueRetitle(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueRetitleArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue retitle", runtime)
	if err != nil {
		return err
	}
	if providerQualifiedRefCommand(options.ref) {
		return r.runIssueRetitleContract(context.Background(), projectRoot, options, out)
	}
	updated, err := state.UpdateIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, state.IssueUpdateOptions{
		Ref:      options.ref,
		Title:    options.title,
		SetTitle: true,
	})
	if err != nil {
		return err
	}
	result, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, updated.ID)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "retitled issue %s\n", issueDisplayRef(result.Issue))
	fmt.Fprintf(out, "title: %s\n", result.Issue.Title)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}
