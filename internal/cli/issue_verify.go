package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issueVerifyResult struct {
	ContractVersion    int                    `json:"contract_version"`
	DatabaseScope      string                 `json:"database_scope"`
	DatabasePath       string                 `json:"database_path"`
	ProjectID          string                 `json:"project_id"`
	ProjectName        string                 `json:"project_name"`
	ProjectCurrentPath string                 `json:"project_current_path"`
	Issue              state.Issue            `json:"issue"`
	Results            []issueVerifyCriterion `json:"results"`
	OK                 bool                   `json:"ok"`
}

type issueVerifyCriterion struct {
	Position     int                       `json:"position"`
	Text         string                    `json:"text"`
	Command      string                    `json:"command"`
	Expect       string                    `json:"expect,omitempty"`
	ExitCode     int                       `json:"exit_code"`
	OK           bool                      `json:"ok"`
	ExpectChecks []changeVerifyExpectCheck `json:"expect_checks,omitempty"`
	Advisory     []string                  `json:"advisory,omitempty"`
}

func (r Runner) runIssueVerify(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("issue verify", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue verify", runtime)
	if err != nil {
		return err
	}
	shown, err := state.ShowIssue(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}

	rootPath := projectRoot.Path()
	result := issueVerifyResult{
		ContractVersion:    shown.ContractVersion,
		DatabaseScope:      shown.DatabaseScope,
		DatabasePath:       shown.DatabasePath,
		ProjectID:          shown.ProjectID,
		ProjectName:        shown.ProjectName,
		ProjectCurrentPath: shown.ProjectCurrentPath,
		Issue:              shown.Issue,
		Results:            []issueVerifyCriterion{},
		OK:                 true,
	}

	for _, criterion := range shown.Issue.Criteria {
		if criterion.Tier != state.IssueCriterionTierV || strings.TrimSpace(criterion.Command) == "" {
			continue
		}
		exitCode, output, runErr := runChangeCriterionCommand(rootPath, criterion.Command)
		expectation := parseChangeExpectation(criterion.Expect)
		checks := evaluateChangeExpectation(expectation, exitCode, output)
		ok := runErr == nil && changeExpectChecksPass(checks)
		if !ok {
			result.OK = false
		}
		item := issueVerifyCriterion{
			Position:     criterion.Position,
			Text:         criterion.Text,
			Command:      criterion.Command,
			Expect:       criterion.Expect,
			ExitCode:     exitCode,
			OK:           ok,
			ExpectChecks: checks,
			Advisory:     expectation.Advisory,
		}
		result.Results = append(result.Results, item)
		if !jsonOutput {
			status := ansiGreen("ok")
			if !ok {
				status = ansiRed("fail")
			}
			fmt.Fprintf(out, "%s %d  %s%s\n", status, criterion.Position, criterion.Command, changeExpectFailureNote(runErr, exitCode, checks))
			for _, clause := range expectation.Advisory {
				fmt.Fprintf(out, "%s %d  unenforceable Expect clause %q — recorded as advisory, never checked\n",
					ansiYellow("warn"), criterion.Position, clause)
			}
		}
	}

	if jsonOutput {
		if err := writeJSON(out, result); err != nil {
			return err
		}
	} else if len(result.Results) == 0 {
		fmt.Fprintf(out, "no executable V-tier criteria on %s\n", issueDisplayRef(shown.Issue))
	}

	if !result.OK {
		return ExitError{Code: 1}
	}
	return nil
}

func writeIssueVerifyHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue verify <ref> [--json]",
		"Run the issue's V-tier criteria (command + expect) from the repository root. Honors exit N and contains `text`. Writes nothing; exits non-zero on any failure.",
		"--json       Output per-criterion results as JSON")
}
