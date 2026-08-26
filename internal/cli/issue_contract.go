package cli

// Authority-ref routing for issue machinery in LOAF-82.
// Covered here: check (including --human; GitHub-tracker publication for branch:/pr:), verify, render, and branch: start/stop against the contract store.
// Deferred to later slices: issue show/create/dod/promote for ref-keyed contracts (LOAF-83 migration + LOAF-85 bootstrap); linear:/pr: start/stop semantics (LOAF-85).
// Linear readiness publication for all work-contract refs (branch:/pr:/linear:) is deferred to LOAF-83/85 — PublishLinearReadiness requires legacy issue rows/mappings, not wct_* ids. --human is still accepted.

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func authorityRefCommand(ref string) bool {
	return state.IsAuthorityRef(ref)
}

func providerQualifiedRefCommand(ref string) bool {
	return state.IsAuthorityRef(ref) || strings.Contains(strings.TrimSpace(ref), ":")
}

var updateWorkContractFn = func(ctx context.Context, root project.Root, resolver state.PathResolver, options state.WorkContractUpdateOptions) (state.WorkContract, error) {
	return state.UpdateWorkContract(ctx, root, resolver, options)
}

func (r Runner) runIssueVerifyContract(ctx context.Context, projectRoot project.Root, ref string, jsonOutput bool, out io.Writer) error {
	shown, err := state.ShowWorkContract(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
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
		Results:            []issueVerifyCriterion{},
		OK:                 true,
	}
	result.Issue = contractAsIssue(shown.Contract)

	for _, criterion := range shown.Contract.Criteria {
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
		fmt.Fprintf(out, "no executable V-tier criteria on %s\n", shown.Contract.AuthorityRef.String())
	}
	if !result.OK {
		return ExitError{Code: 1}
	}
	return nil
}

func (r Runner) runIssueCheckContract(ctx context.Context, projectRoot project.Root, ref string, human string, jsonOutput bool, out io.Writer) error {
	resolver := state.PathResolver{StateHome: r.StateHome}
	readiness, err := state.CheckWorkContractReadiness(ctx, projectRoot, resolver, ref)
	if err != nil {
		return err
	}
	shown, err := state.ShowWorkContract(ctx, projectRoot, resolver, ref)
	if err != nil {
		return err
	}
	issue := contractAsIssue(readiness.Contract)
	result := issueCheckResult{
		ContractVersion:    shown.ContractVersion,
		DatabaseScope:      shown.DatabaseScope,
		DatabasePath:       shown.DatabasePath,
		ProjectID:          shown.ProjectID,
		ProjectName:        shown.ProjectName,
		ProjectCurrentPath: shown.ProjectCurrentPath,
		Issue:              issue,
		Kind:               readiness.Kind,
		Shaped:             readiness.Shaped,
		Covered:            readiness.Covered,
		Ready:              readiness.Ready,
		Failures:           readiness.Failures,
		Orphans:            readiness.Orphans,
	}
	// Work-contract Linear publication needs legacy issue mapping (LOAF-83/85).
	// Never pass wct_* ids into PublishLinearReadiness — that GetIssue path fails and would
	// turn a ready check into a non-zero exit on Linear-tracker projects.
	if readiness.Ready {
		identity, err := state.GetIssueIdentity(ctx, projectRoot, resolver)
		if err != nil {
			return err
		}
		if trackerAuthority(identity.Authority) && identity.Authority != state.IssueAuthorityLinear {
			publication := ReadinessPublication{
				IssueID:     issue.ID,
				IssueRef:    issueDisplayRef(issue),
				Label:       readinessLabelAgent,
				Authority:   identity.Authority,
				ProjectPath: projectRoot.Path(),
				StateHome:   r.StateHome,
			}
			if human != "" {
				publication.Label = readinessLabelHuman
				publication.Reason = human
			}
			if err := defaultReadinessPublisher.Publish(ctx, publication); err != nil {
				return err
			}
			result.Publication = &publication
		}
	}
	if jsonOutput {
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

func (r Runner) runIssueRenderContract(ctx context.Context, projectRoot project.Root, ref string, jsonOutput bool, out io.Writer) error {
	result, err := state.ShowWorkContract(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	markdown := state.RenderWorkContractMarkdown(result)
	if jsonOutput {
		return writeJSON(out, map[string]any{
			"contract_version":     result.ContractVersion,
			"database_scope":       result.DatabaseScope,
			"database_path":        result.DatabasePath,
			"project_id":           result.ProjectID,
			"project_name":         result.ProjectName,
			"project_current_path": result.ProjectCurrentPath,
			"authority_ref":        result.Contract.AuthorityRef,
			"contract":             result.Contract,
			"markdown":             markdown,
		})
	}
	fmt.Fprint(out, markdown)
	return nil
}

func contractAsIssue(contract state.WorkContract) state.Issue {
	return state.Issue{
		ID:              contract.ID,
		Alias:           contract.AuthorityRef.String(),
		ParentID:        contract.ParentContractID,
		Kind:            contract.Kind,
		Title:           contract.Title,
		Body:            contract.Body,
		Fog:             contract.Fog,
		Status:          contract.Status,
		StartedBranch:   contract.StartedBranch,
		StartedWorktree: contract.StartedWorktree,
		WorktreeMissing: contract.WorktreeMissing,
		Criteria:        contractCriteriaAsIssue(contract.Criteria),
		CreatedAt:       contract.CreatedAt,
		UpdatedAt:       contract.UpdatedAt,
	}
}

func contractCriteriaAsIssue(criteria []state.WorkContractCriterion) []state.IssueCriterion {
	out := make([]state.IssueCriterion, 0, len(criteria))
	for _, criterion := range criteria {
		out = append(out, state.IssueCriterion{
			ID:       criterion.ID,
			Position: criterion.Position,
			Text:     criterion.Text,
			Command:  criterion.Command,
			Expect:   criterion.Expect,
			Tier:     criterion.Tier,
		})
	}
	return out
}

func (r Runner) runIssueStartContract(ctx context.Context, projectRoot project.Root, ref string, jsonOutput bool, out io.Writer) error {
	authorityRef, err := state.ParseAuthorityRef(ref)
	if err != nil {
		return err
	}
	if authorityRef.Provider != state.AuthorityProviderBranch {
		return fmt.Errorf("%s worktree start is not supported in v1; use branch: refs for trackerless workspaces", authorityRef.Provider)
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	shown, err := state.ShowWorkContract(ctx, projectRoot, resolver, ref)
	if err != nil {
		return err
	}
	if err := refuseIssueStartLifecycle(contractAsIssue(shown.Contract)); err != nil {
		return err
	}
	if strings.TrimSpace(shown.Contract.StartedWorktree) != "" {
		return fmt.Errorf("contract %s is already started at %s", ref, shown.Contract.StartedWorktree)
	}
	repoRoot := projectRoot.Path()
	if !gitRepoAt(repoRoot) {
		return fmt.Errorf("issue start requires a git repository")
	}
	branch := authorityRef.Key
	worktree := issueWorktreePath(repoRoot, branch)
	if _, err := os.Stat(worktree); err == nil {
		return fmt.Errorf("worktree path %s already exists", worktree)
	}
	base := resolveReleaseDefaultBranch(repoRoot)
	if base == "" {
		return fmt.Errorf("could not resolve repository default branch")
	}
	base = qualifyIssueStartBase(repoRoot, base)
	createdBranch, err := addIssueWorktree(repoRoot, worktree, branch, base)
	if err != nil {
		return err
	}
	updated, err := updateWorkContractFn(ctx, projectRoot, resolver, state.WorkContractUpdateOptions{
		AuthorityRef:    authorityRef,
		Status:          state.IssueStatusActive,
		SetStatus:       true,
		StartedBranch:   branch,
		StartedWorktree: worktree,
		SetStarted:      true,
	})
	if err != nil {
		return wrapIssueStartUpdateError(err, rollbackIssueWorktree(repoRoot, worktree, branch, createdBranch), worktree, branch)
	}
	if jsonOutput {
		return writeJSON(out, map[string]any{
			"authority_ref": authorityRef,
			"contract":      updated,
			"branch":        branch,
			"worktree":      worktree,
			"base":          base,
		})
	}
	fmt.Fprintf(out, "started contract %s\nbranch: %s\nworktree: %s\nbase: %s\n", ref, branch, worktree, base)
	return nil
}

func (r Runner) runIssueStopContract(ctx context.Context, projectRoot project.Root, ref string, force, jsonOutput bool, out io.Writer) error {
	authorityRef, err := state.ParseAuthorityRef(ref)
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	shown, err := state.ShowWorkContract(ctx, projectRoot, resolver, ref)
	if err != nil {
		return err
	}
	if strings.TrimSpace(shown.Contract.StartedWorktree) == "" {
		return fmt.Errorf("contract %s is not started", ref)
	}
	savedBranch := shown.Contract.StartedBranch
	savedWorktree := shown.Contract.StartedWorktree
	if _, err := updateWorkContractFn(ctx, projectRoot, resolver, state.WorkContractUpdateOptions{
		AuthorityRef: authorityRef,
		SetStarted:   true,
	}); err != nil {
		return err
	}
	alreadyGone, err := removeIssueWorktreeFn(projectRoot.Path(), savedWorktree, force)
	if err != nil {
		if _, restoreErr := updateWorkContractFn(ctx, projectRoot, resolver, state.WorkContractUpdateOptions{
			AuthorityRef:    authorityRef,
			StartedBranch:   savedBranch,
			StartedWorktree: savedWorktree,
			SetStarted:      true,
		}); restoreErr != nil {
			return fmt.Errorf("remove worktree: %w (also failed to restore started fields: %v)", err, restoreErr)
		}
		return err
	}
	if jsonOutput {
		return writeJSON(out, map[string]any{
			"authority_ref": authorityRef,
			"branch":        savedBranch,
			"worktree":      savedWorktree,
			"already_gone":  alreadyGone,
		})
	}
	fmt.Fprintf(out, "stopped contract %s\nkept branch: %s\n", ref, savedBranch)
	if alreadyGone {
		fmt.Fprintf(out, "worktree already gone: %s\n", savedWorktree)
	} else {
		fmt.Fprintf(out, "removed worktree: %s\n", savedWorktree)
	}
	return nil
}
