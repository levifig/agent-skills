package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	RenderOutReceiptKind       = "render-out"
	RenderOutMappingIssueID    = "internal_issue_id"
	RenderOutMappingIssueAlias = "internal_issue_alias"
	RenderOutMappingExportPath = "manual_export_path"
)

// IssueRenderOutOptions controls render-out of one internal issue row.
type IssueRenderOutOptions struct {
	Ref          string
	Branch       string
	ManualExport string
	Retire       bool
	DryRun       bool
}

// IssueRenderOutResult is the durable authority surface created from an issue row.
type IssueRenderOutResult struct {
	ContractVersion    int          `json:"contract_version"`
	DatabaseScope      string       `json:"database_scope"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Issue              Issue        `json:"issue"`
	AuthorityRef       AuthorityRef `json:"authority_ref"`
	Contract           WorkContract `json:"contract"`
	ReceiptKind        string       `json:"receipt_kind"`
	Retired            bool         `json:"retired"`
	Action             string       `json:"action"`
}

// RenderOutIssue publishes an internal issue row to a durable authority ref.
func RenderOutIssue(ctx context.Context, root project.Root, resolver PathResolver, options IssueRenderOutOptions) (IssueRenderOutResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return IssueRenderOutResult{}, err
	}
	defer store.Close()
	return store.RenderOutIssue(ctx, root, options)
}

func (s *Store) RenderOutIssue(ctx context.Context, root project.Root, options IssueRenderOutOptions) (IssueRenderOutResult, error) {
	ref := strings.TrimSpace(options.Ref)
	if ref == "" {
		return IssueRenderOutResult{}, fmt.Errorf("render-out requires an issue ref")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return IssueRenderOutResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return IssueRenderOutResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return IssueRenderOutResult{}, fmt.Errorf("begin issue render-out: %w", err)
	}
	defer tx.Rollback()

	issueID, issueAlias, err := resolveIssueRefTx(ctx, tx, projectID, ref)
	if err != nil {
		return IssueRenderOutResult{}, err
	}
	issue, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return IssueRenderOutResult{}, err
	}
	if issue.ArchivedAt != "" {
		return IssueRenderOutResult{}, fmt.Errorf("issue %s is already archived", firstNonEmpty(issueAlias, issue.ID))
	}
	if issue.Kind == IssueKindDecision {
		return IssueRenderOutResult{}, fmt.Errorf("decision-kind issues re-home via ledger facts (LOAF-84), not render-out")
	}

	issueIdentity, err := loadIssueIdentityTx(ctx, tx, projectID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return IssueRenderOutResult{}, err
	}
	if err == nil && issueIdentity.Authority == IssueAuthorityLinear {
		bound, bindErr := lookupLinearIssueIdentifierTx(ctx, tx, projectID, issueID)
		if bindErr != nil {
			return IssueRenderOutResult{}, bindErr
		}
		if bound == "" {
			return IssueRenderOutResult{}, fmt.Errorf("linear-configured project: publish issue to Linear first (loaf issue push %s), then render-out", firstNonEmpty(issueAlias, ref))
		}
		authorityRef := AuthorityRef{Provider: AuthorityProviderLinear, Key: bound}
		return s.renderOutToExistingAuthority(ctx, tx, identity, issue, issueAlias, authorityRef, options)
	}

	if strings.TrimSpace(options.ManualExport) != "" {
		exportPath := strings.TrimSpace(options.ManualExport)
		authorityRef := AuthorityRef{Provider: AuthorityProviderBranch, Key: exportPath}
		return s.renderOutToExistingAuthority(ctx, tx, identity, issue, issueAlias, authorityRef, options)
	}

	branch := strings.TrimSpace(options.Branch)
	if branch == "" {
		branch = strings.TrimSpace(issue.StartedBranch)
	}
	if branch == "" && issueAlias != "" {
		suffix := strings.TrimPrefix(strings.ToUpper(issueAlias), "LOAF-")
		branch = "issue/loaf-" + strings.ToLower(suffix)
	}
	if branch == "" {
		return IssueRenderOutResult{}, fmt.Errorf("render-out requires --branch, a started branch on the issue, or a LOAF alias")
	}
	authorityRef := AuthorityRef{Provider: AuthorityProviderBranch, Key: branch}
	return s.renderOutToExistingAuthority(ctx, tx, identity, issue, issueAlias, authorityRef, options)
}

func (s *Store) renderOutToExistingAuthority(ctx context.Context, tx *sql.Tx, identity ProjectIdentity, issue Issue, issueAlias string, authorityRef AuthorityRef, options IssueRenderOutOptions) (IssueRenderOutResult, error) {
	projectID := identity.ID
	now := time.Now().UTC().Format(time.RFC3339Nano)
	criteria := issueCriteriaToInputs(issue.Criteria)

	result := IssueRenderOutResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Issue:              issue,
		AuthorityRef:       authorityRef,
		ReceiptKind:        RenderOutReceiptKind,
		Action:             "applied",
	}
	if options.DryRun {
		result.Action = "dry-run"
		result.Contract = WorkContract{
			AuthorityRef: authorityRef,
			Kind:         issue.Kind,
			Title:        issue.Title,
			Body:         issue.Body,
			Fog:          issue.Fog,
			Status:       issue.Status,
			Criteria:     criteriaToWorkContract(criteria),
		}
		return result, nil
	}

	var contract WorkContract
	existing, err := loadWorkContractByRefTx(ctx, tx, projectID, authorityRef)
	switch {
	case err == nil:
		contract = existing
		if _, err := tx.ExecContext(ctx, `
UPDATE work_contracts
SET title = ?, body = ?, fog = ?, status = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, issue.Title, issue.Body, emptyToNil(strings.TrimSpace(issue.Fog)), issue.Status, now, projectID, contract.ID); err != nil {
			return IssueRenderOutResult{}, fmt.Errorf("refresh rendered work contract: %w", err)
		}
		if err := replaceWorkContractCriteriaTx(ctx, tx, projectID, contract.ID, criteria, now); err != nil {
			return IssueRenderOutResult{}, err
		}
		contract, err = loadWorkContractTx(ctx, tx, projectID, contract.ID)
		if err != nil {
			return IssueRenderOutResult{}, err
		}
	case strings.Contains(err.Error(), "not found"):
		contract, err = createWorkContractTx(ctx, tx, projectID, WorkContractCreateOptions{
			AuthorityRef: authorityRef,
			Title:        issue.Title,
			Body:         issue.Body,
			Fog:          issue.Fog,
			Kind:         issue.Kind,
			Criteria:     criteria,
		}, now)
		if err != nil {
			return IssueRenderOutResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE work_contracts SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, issue.Status, now, projectID, contract.ID); err != nil {
			return IssueRenderOutResult{}, fmt.Errorf("set rendered contract status: %w", err)
		}
		contract, err = loadWorkContractTx(ctx, tx, projectID, contract.ID)
		if err != nil {
			return IssueRenderOutResult{}, err
		}
	default:
		return IssueRenderOutResult{}, err
	}

	if err := upsertWorkContractReceiptTx(ctx, tx, projectID, authorityRef, RenderOutReceiptKind, issue.ID, now); err != nil {
		return IssueRenderOutResult{}, fmt.Errorf("record render-out receipt: %w", err)
	}
	if err := upsertWorkContractMappingTx(ctx, tx, projectID, authorityRef, RenderOutMappingIssueID, issue.ID, now); err != nil {
		return IssueRenderOutResult{}, err
	}
	if issueAlias != "" {
		if err := upsertWorkContractMappingTx(ctx, tx, projectID, authorityRef, RenderOutMappingIssueAlias, issueAlias, now); err != nil {
			return IssueRenderOutResult{}, err
		}
	}
	if strings.TrimSpace(options.ManualExport) != "" {
		if err := upsertWorkContractMappingTx(ctx, tx, projectID, authorityRef, RenderOutMappingExportPath, strings.TrimSpace(options.ManualExport), now); err != nil {
			return IssueRenderOutResult{}, err
		}
	}

	if options.Retire {
		archivedAt := issue.ArchivedAt
		if archivedAt == "" {
			archivedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE issues SET status = ?, archived_at = ?, updated_at = ? WHERE project_id = ? AND id = ?
`, IssueStatusCancelled, archivedAt, now, projectID, issue.ID); err != nil {
			return IssueRenderOutResult{}, fmt.Errorf("retire rendered issue: %w", err)
		}
		if issue.Status != IssueStatusCancelled {
			if _, err := insertIssueStatusEventTx(ctx, tx, projectID, issue.ID, issue.Status, IssueStatusCancelled, "rendered to "+authorityRef.String(), now); err != nil {
				return IssueRenderOutResult{}, err
			}
		}
		result.Retired = true
		issue.Status = IssueStatusCancelled
		issue.ArchivedAt = archivedAt
		result.Issue = issue
	}

	if err := tx.Commit(); err != nil {
		return IssueRenderOutResult{}, fmt.Errorf("commit issue render-out: %w", err)
	}
	result.Contract = contract
	return result, nil
}

func lookupLinearIssueIdentifierTx(ctx context.Context, tx *sql.Tx, projectID, issueID string) (string, error) {
	var identifier string
	err := tx.QueryRowContext(ctx, `
SELECT external_id FROM backend_mappings
WHERE project_id = ? AND backend = 'linear' AND entity_kind = 'issue' AND entity_id = ?
`, projectID, issueID).Scan(&identifier)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return identifier, err
}

func issueCriteriaToInputs(criteria []IssueCriterion) []IssueCriterionInput {
	inputs := make([]IssueCriterionInput, 0, len(criteria))
	for _, criterion := range criteria {
		inputs = append(inputs, IssueCriterionInput{
			Text:     criterion.Text,
			Command:  criterion.Command,
			Expect:   criterion.Expect,
			Tier:     criterion.Tier,
			Position: criterion.Position,
		})
	}
	return inputs
}

func criteriaToWorkContract(inputs []IssueCriterionInput) []WorkContractCriterion {
	out := make([]WorkContractCriterion, 0, len(inputs))
	for i, input := range inputs {
		out = append(out, WorkContractCriterion{
			Position: i + 1,
			Text:     input.Text,
			Command:  input.Command,
			Expect:   input.Expect,
			Tier:     input.Tier,
		})
	}
	return out
}
