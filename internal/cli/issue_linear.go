package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issuePullOptions struct {
	jsonOutput bool
	tree       bool
	key        string
}

type issuePushOptions struct {
	jsonOutput bool
	ref        string
}

type issueReconcileOptions struct {
	jsonOutput  bool
	takeLocal   bool
	takeTracker bool
	ref         string
}

func (r Runner) runIssuePull(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssuePullArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue pull", runtime)
	if err != nil {
		return err
	}
	client, err := state.LinearClientFromEnv()
	if err != nil {
		return err
	}
	result, err := state.PullLinearIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, client, options.key, options.tree)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "pulled %s\n", issueDisplayRef(result.Issue))
	for _, issue := range result.Tree {
		fmt.Fprintf(out, "  %s  %s  %s\n", issueDisplayRef(issue), issue.Status, issue.Title)
	}
	return nil
}

func (r Runner) runIssuePush(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssuePushArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue push", runtime)
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	shown, err := state.ShowIssue(context.Background(), projectRoot, resolver, options.ref)
	if err != nil {
		return err
	}
	client, err := state.LinearClientFromEnv()
	if err != nil {
		return err
	}
	result, err := state.PushLinearIssue(context.Background(), projectRoot, resolver, client, shown.Issue.ID, renderIssueMarkdown(shown))
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "pushed %s\n", issueDisplayRef(result.Issue))
	fmt.Fprintln(out, "description: updated")
	if result.StatusWrote {
		fmt.Fprintf(out, "status: updated (%s)\n", result.Issue.Status)
	} else if result.StatusSkipped != "" {
		fmt.Fprintf(out, "status: skipped (%s)\n", result.StatusSkipped)
	} else {
		fmt.Fprintln(out, "status: unchanged")
	}
	return nil
}

func (r Runner) runIssueReconcile(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueReconcileArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue reconcile", runtime)
	if err != nil {
		return err
	}
	client, err := state.LinearClientFromEnv()
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	var results []state.LinearReconcileResult
	if options.ref == "" {
		results, err = state.ReconcileLinearIssues(context.Background(), projectRoot, resolver, client, options.takeLocal, options.takeTracker)
	} else {
		var result state.LinearReconcileResult
		result, err = state.ReconcileLinearIssue(context.Background(), projectRoot, resolver, client, options.ref, options.takeLocal, options.takeTracker)
		if err == nil {
			results = []state.LinearReconcileResult{result}
		}
	}
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, results)
	}
	if len(results) == 0 {
		fmt.Fprintln(out, "no linear-mapped issues to reconcile")
		return nil
	}
	for _, result := range results {
		writeIssueReconcile(out, result)
	}
	return nil
}

func writeIssueReconcile(out io.Writer, result state.LinearReconcileResult) {
	ref := issueDisplayRef(result.Issue)
	if result.InSync && len(result.Conflicts) == 0 {
		fmt.Fprintf(out, "issue %s is in sync\n", ref)
		return
	}
	fmt.Fprintf(out, "issue %s\n", ref)
	for _, conflict := range result.Conflicts {
		switch conflict.Field {
		case "title":
			fmt.Fprintln(out, "title: tracker wins")
			fmt.Fprintf(out, "  local:  %q\n", conflict.Local)
			fmt.Fprintf(out, "  linear: %q\n", conflict.Tracker)
			fmt.Fprintf(out, "  action: %s\n", conflict.Resolution)
		case "status":
			fmt.Fprintf(out, "status: %s\n", conflict.Mover)
			fmt.Fprintf(out, "  local:  %s (%s)\n", conflict.Local, conflict.LocalAt)
			fmt.Fprintf(out, "  linear: %s (%s)\n", conflict.Tracker, conflict.TrackerAt)
			fmt.Fprintf(out, "  action: %s\n", conflict.Resolution)
		case "description":
			fmt.Fprintln(out, "description: drifted (report only)")
			fmt.Fprintf(out, "  action: %s\n", conflict.Resolution)
		default:
			fmt.Fprintf(out, "%s: %s\n", conflict.Field, conflict.Resolution)
		}
	}
}

func parseIssuePullArgs(args []string) (issuePullOptions, error) {
	var options issuePullOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--tree":
			options.tree = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return issuePullOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return issuePullOptions{}, fmt.Errorf("issue pull requires a Linear key")
	}
	options.key = positional[0]
	return options, nil
}

func parseIssuePushArgs(args []string) (issuePushOptions, error) {
	var options issuePushOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		default:
			if strings.HasPrefix(arg, "-") {
				return issuePushOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 1 {
		return issuePushOptions{}, fmt.Errorf("issue push requires an issue ref")
	}
	options.ref = positional[0]
	return options, nil
}

func parseIssueReconcileArgs(args []string) (issueReconcileOptions, error) {
	var options issueReconcileOptions
	var positional []string
	for _, arg := range args {
		switch arg {
		case "--json":
			options.jsonOutput = true
		case "--take-local":
			options.takeLocal = true
		case "--take-tracker":
			options.takeTracker = true
		default:
			if strings.HasPrefix(arg, "-") {
				return issueReconcileOptions{}, fmt.Errorf("unknown option %q", arg)
			}
			positional = append(positional, arg)
		}
	}
	if options.takeLocal && options.takeTracker {
		return issueReconcileOptions{}, fmt.Errorf("issue reconcile accepts at most one of --take-local and --take-tracker")
	}
	if len(positional) > 1 {
		return issueReconcileOptions{}, fmt.Errorf("issue reconcile accepts at most one ref")
	}
	if len(positional) == 1 {
		options.ref = positional[0]
	}
	return options, nil
}

func writeIssuePullHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue pull <linear-key> [--tree] [--json]",
		"Adopt an existing Linear issue as a local row. The Linear key becomes the alias; the local counter is not advanced.",
		"--tree       Also adopt the sub-issue tree with parent edges intact",
		"--json       Output the adopted issue and tree as JSON")
}

func writeIssuePushHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue push <ref> [--json]",
		"Write loaf issue render as the Linear description. Status is written only when the local status event is newer than the tracker. Never renames the Linear issue.",
		"--json       Output the push result as JSON")
}

func writeIssueReconcileHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue reconcile [<ref>] [--take-local|--take-tracker] [--json]",
		"Compare local and Linear. Title drift updates the local title (tracker wins). Status drift is reported; use --take-local or --take-tracker to resolve. Description drift is reported only.",
		"--take-local    Write the local status to Linear",
		"--take-tracker  Write the Linear status to local through the events path",
		"--json          Output the reconcile result as JSON")
}
