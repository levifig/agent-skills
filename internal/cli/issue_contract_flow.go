package cli

// Flow-skill contract routing for LOAF-86: new --ref, show, edit, retitle,
// status, and dod add/list/remove/claim/unclaim against the work-contract store.

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func issueResultFromContract(shown state.WorkContractResult) state.IssueResult {
	result := state.IssueResult{
		ContractVersion:    shown.ContractVersion,
		DatabaseScope:      shown.DatabaseScope,
		DatabasePath:       shown.DatabasePath,
		ProjectID:          shown.ProjectID,
		ProjectName:        shown.ProjectName,
		ProjectCurrentPath: shown.ProjectCurrentPath,
		Issue:              contractAsIssue(shown.Contract),
		Children:           contractSummariesAsIssueSummaries(shown.Children),
	}
	if shown.Parent != nil {
		parent := contractSummaryAsIssueSummary(*shown.Parent)
		result.Parent = &parent
	}
	return result
}

func contractSummaryAsIssueSummary(summary state.WorkContractSummary) state.IssueSummary {
	return state.IssueSummary{
		ID:     summary.ID,
		Alias:  summary.AuthorityRef.String(),
		Kind:   summary.Kind,
		Title:  summary.Title,
		Status: summary.Status,
	}
}

func contractSummariesAsIssueSummaries(summaries []state.WorkContractSummary) []state.IssueSummary {
	out := make([]state.IssueSummary, 0, len(summaries))
	for _, summary := range summaries {
		out = append(out, contractSummaryAsIssueSummary(summary))
	}
	return out
}

func (r Runner) showContractResult(ctx context.Context, projectRoot project.Root, ref string) (state.IssueResult, error) {
	shown, err := state.ShowWorkContract(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return state.IssueResult{}, err
	}
	return issueResultFromContract(shown), nil
}

func (r Runner) writeContractIssueResult(out io.Writer, result state.IssueResult, jsonOutput bool, writeHuman func()) error {
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeHuman()
	return nil
}

func (r Runner) runIssueNewContract(ctx context.Context, projectRoot project.Root, options issueNewOptions, out io.Writer) error {
	authorityRef, err := state.ParseAuthorityRef(options.ref)
	if err != nil {
		return err
	}
	create := state.WorkContractCreateOptions{
		AuthorityRef: authorityRef,
		Title:        options.create.Title,
		Body:         options.create.Body,
		Fog:          options.create.Fog,
		Kind:         options.create.Kind,
	}
	if parent := strings.TrimSpace(options.create.Parent); parent != "" {
		parentRef, err := state.ParseAuthorityRef(parent)
		if err != nil {
			return fmt.Errorf("work-contract --parent must be a provider-qualified ref (linear:, branch:, or pr:): %w", err)
		}
		create.Parent = parentRef
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	created, err := state.CreateWorkContract(ctx, projectRoot, resolver, create)
	if err != nil {
		return err
	}
	if options.status != "" && options.status != state.IssueStatusTriage {
		if _, err := state.UpdateWorkContract(ctx, projectRoot, resolver, state.WorkContractUpdateOptions{
			AuthorityRef: created.AuthorityRef,
			Status:       options.status,
			SetStatus:    true,
		}); err != nil {
			return err
		}
	}
	result, err := r.showContractResult(ctx, projectRoot, created.AuthorityRef.String())
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, options.jsonOutput, func() {
		writeIssueCreated(out, result)
	})
}

func (r Runner) runIssueShowContract(ctx context.Context, projectRoot project.Root, ref string, jsonOutput bool, out io.Writer) error {
	result, err := r.showContractResult(ctx, projectRoot, ref)
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, jsonOutput, func() {
		writeIssueShow(out, result)
	})
}

func (r Runner) runIssueEditContract(ctx context.Context, projectRoot project.Root, options issueEditOptions, body string, out io.Writer) error {
	authorityRef, err := state.ParseAuthorityRef(options.ref)
	if err != nil {
		return err
	}
	if _, err := state.UpdateWorkContract(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, state.WorkContractUpdateOptions{
		AuthorityRef: authorityRef,
		Body:         body,
		SetBody:      true,
	}); err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, options.ref)
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, options.jsonOutput, func() {
		fmt.Fprintf(out, "edited issue %s\n", issueDisplayRef(result.Issue))
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	})
}

func (r Runner) runIssueRetitleContract(ctx context.Context, projectRoot project.Root, options issueRetitleOptions, out io.Writer) error {
	authorityRef, err := state.ParseAuthorityRef(options.ref)
	if err != nil {
		return err
	}
	if _, err := state.UpdateWorkContract(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, state.WorkContractUpdateOptions{
		AuthorityRef: authorityRef,
		Title:        options.title,
		SetTitle:     true,
	}); err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, options.ref)
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, options.jsonOutput, func() {
		fmt.Fprintf(out, "retitled issue %s\n", issueDisplayRef(result.Issue))
		fmt.Fprintf(out, "title: %s\n", result.Issue.Title)
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	})
}

func (r Runner) runIssueStatusContract(ctx context.Context, projectRoot project.Root, options issueStatusOptions, out io.Writer) error {
	if options.duplicateOf != "" {
		return fmt.Errorf("issue status --duplicate-of is not supported on work-contract refs")
	}
	authorityRef, err := state.ParseAuthorityRef(options.ref)
	if err != nil {
		return err
	}
	if _, err := state.UpdateWorkContract(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, state.WorkContractUpdateOptions{
		AuthorityRef: authorityRef,
		Status:       options.status,
		SetStatus:    true,
	}); err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, options.ref)
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, options.jsonOutput, func() {
		writeIssueStatus(out, result)
	})
}

func (r Runner) runIssueDodAddContract(ctx context.Context, projectRoot project.Root, options issueDodAddOptions, out io.Writer) error {
	updated, err := state.AddWorkContractCriterion(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, options.ref, options.input)
	if err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, updated.AuthorityRef.String())
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, options.jsonOutput, func() {
		fmt.Fprintf(out, "added criterion %d on %s\n", result.Issue.Criteria[len(result.Issue.Criteria)-1].Position, issueDisplayRef(result.Issue))
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	})
}

func (r Runner) runIssueDodListContract(ctx context.Context, projectRoot project.Root, ref string, jsonOutput bool, out io.Writer) error {
	result, err := r.showContractResult(ctx, projectRoot, ref)
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, jsonOutput, func() {
		writeIssueDodList(out, result)
	})
}

func (r Runner) runIssueDodRemoveContract(ctx context.Context, projectRoot project.Root, ref string, position int, jsonOutput bool, out io.Writer) error {
	if _, err := state.RemoveWorkContractCriterion(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, position); err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, ref)
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, jsonOutput, func() {
		fmt.Fprintf(out, "removed criterion %d from %s\n", position, issueDisplayRef(result.Issue))
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	})
}

func (r Runner) runIssueDodClaimContract(ctx context.Context, projectRoot project.Root, ref string, childPosition, parentPosition int, jsonOutput bool, out io.Writer) error {
	updated, err := state.ClaimWorkContractCriterion(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, childPosition, parentPosition)
	if err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, updated.AuthorityRef.String())
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, jsonOutput, func() {
		fmt.Fprintf(out, "claimed criterion %d on %s as serving parent criterion %d\n", childPosition, issueDisplayRef(result.Issue), parentPosition)
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	})
}

func (r Runner) runIssueDodUnclaimContract(ctx context.Context, projectRoot project.Root, ref string, childPosition, parentPosition int, jsonOutput bool, out io.Writer) error {
	updated, err := state.UnclaimWorkContractCriterion(ctx, projectRoot, state.PathResolver{StateHome: r.StateHome}, ref, childPosition, parentPosition)
	if err != nil {
		return err
	}
	result, err := r.showContractResult(ctx, projectRoot, updated.AuthorityRef.String())
	if err != nil {
		return err
	}
	return r.writeContractIssueResult(out, result, jsonOutput, func() {
		fmt.Fprintf(out, "unclaimed criterion %d on %s from parent criterion %d\n", childPosition, issueDisplayRef(result.Issue), parentPosition)
		writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	})
}
