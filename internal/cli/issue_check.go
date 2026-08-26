package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issueCheckOptions struct {
	jsonOutput bool
	ref        string
	human      string
}

type issueCheckResult struct {
	ContractVersion    int                           `json:"contract_version"`
	DatabaseScope      string                        `json:"database_scope"`
	DatabasePath       string                        `json:"database_path"`
	ProjectID          string                        `json:"project_id"`
	ProjectName        string                        `json:"project_name"`
	ProjectCurrentPath string                        `json:"project_current_path"`
	Issue              state.Issue                   `json:"issue"`
	Kind               string                        `json:"kind"`
	Shaped             bool                          `json:"shaped"`
	Covered            bool                          `json:"covered"`
	Ready              bool                          `json:"ready"`
	Failures           []state.IssueReadinessFailure `json:"failures"`
	Orphans            []state.IssueReadinessOrphan  `json:"orphans"`
	Publication        *ReadinessPublication         `json:"publication,omitempty"`
}

func (r Runner) runIssueCheck(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueCheckArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue check", runtime)
	if err != nil {
		return err
	}
	if providerQualifiedRefCommand(options.ref) {
		return r.runIssueCheckContract(context.Background(), projectRoot, options.ref, options.jsonOutput, out)
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	readiness, err := state.CheckIssueReadiness(context.Background(), projectRoot, resolver, options.ref)
	if err != nil {
		return err
	}
	shown, err := state.ShowIssue(context.Background(), projectRoot, resolver, readiness.Issue.ID)
	if err != nil {
		return err
	}

	result := issueCheckResult{
		ContractVersion:    shown.ContractVersion,
		DatabaseScope:      shown.DatabaseScope,
		DatabasePath:       shown.DatabasePath,
		ProjectID:          shown.ProjectID,
		ProjectName:        shown.ProjectName,
		ProjectCurrentPath: shown.ProjectCurrentPath,
		Issue:              readiness.Issue,
		Kind:               readiness.Kind,
		Shaped:             readiness.Shaped,
		Covered:            readiness.Covered,
		Ready:              readiness.Ready,
		Failures:           readiness.Failures,
		Orphans:            readiness.Orphans,
	}

	if readiness.Ready {
		identity, err := state.GetIssueIdentity(context.Background(), projectRoot, resolver)
		if err != nil {
			return err
		}
		if trackerAuthority(identity.Authority) {
			publication := ReadinessPublication{
				IssueID:     readiness.Issue.ID,
				IssueRef:    issueDisplayRef(readiness.Issue),
				Label:       readinessLabelAgent,
				Authority:   identity.Authority,
				ProjectPath: projectRoot.Path(),
				StateHome:   r.StateHome,
			}
			if options.human != "" {
				publication.Label = readinessLabelHuman
				publication.Reason = options.human
			}
			if err := defaultReadinessPublisher.Publish(context.Background(), publication); err != nil {
				return err
			}
			result.Publication = &publication
		}
	}

	if options.jsonOutput {
		if err := writeJSON(out, result); err != nil {
			return err
		}
		if !readiness.Ready {
			return ExitError{Code: 1}
		}
		return nil
	}

	writeIssueCheck(out, result)
	if !readiness.Ready {
		return ExitError{Code: 1}
	}
	return nil
}

func writeIssueCheck(out io.Writer, result issueCheckResult) {
	ref := issueDisplayRef(result.Issue)
	if result.Ready {
		if result.Issue.Kind == state.IssueKindDecision {
			fmt.Fprintf(out, "issue %s is ready\n", ref)
		} else {
			fmt.Fprintf(out, "issue %s is shaped\n", ref)
		}
	} else {
		fmt.Fprintf(out, "issue %s is not ready\n", ref)
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	if len(result.Failures) > 0 {
		fmt.Fprintln(out, "failures:")
		for _, failure := range result.Failures {
			fmt.Fprintf(out, "  %s\n", failure.Message)
		}
	}
	if len(result.Orphans) > 0 {
		fmt.Fprintln(out, "orphans:")
		for _, orphan := range result.Orphans {
			fmt.Fprintf(out, "  %s criterion %d: %s\n", orphan.ChildRef, orphan.Position, orphan.Text)
			fmt.Fprintf(out, "  remedy: %s\n", orphan.Remedy)
		}
	}
}

func parseIssueCheckArgs(args []string) (issueCheckOptions, error) {
	var options issueCheckOptions
	var positional []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			options.jsonOutput = true
		case "--human":
			value, err := consumeFlagValue(args, &i, "--human")
			if err != nil {
				return issueCheckOptions{}, err
			}
			options.human = strings.TrimSpace(value)
			if options.human == "" {
				return issueCheckOptions{}, fmt.Errorf("issue check --human requires a reason")
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return issueCheckOptions{}, fmt.Errorf("unknown option %q", args[i])
			}
			positional = append(positional, args[i])
		}
	}
	if len(positional) != 1 {
		return issueCheckOptions{}, fmt.Errorf("issue check requires an issue ref")
	}
	options.ref = positional[0]
	return options, nil
}

func writeIssueCheckHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue check <ref> [--json] [--human <reason>]",
		"Derive readiness from the issue row: a delivery issue is shaped with a nonempty body, at least one criterion, and an out-of-scope statement; a decision issue is ready on a sharp question. Children add coverage (fail) and containment (report).",
		"--json        Output structured readiness results as JSON",
		"--human       Publish ready-for-human instead of ready-for-agent, with this reason")
}
