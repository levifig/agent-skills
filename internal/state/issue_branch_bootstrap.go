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

const BootstrapReceiptKind = "bootstrap"

type BootstrapIssueBranchContractOptions struct {
	IssueID         string
	Branch          string
	StartedWorktree string
}

type BootstrapIssueBranchContractResult struct {
	ContractVersion    int          `json:"contract_version"`
	DatabaseScope      string       `json:"database_scope"`
	DatabasePath       string       `json:"database_path,omitempty"`
	ProjectID          string       `json:"project_id,omitempty"`
	ProjectName        string       `json:"project_name,omitempty"`
	ProjectCurrentPath string       `json:"project_current_path,omitempty"`
	Issue              Issue        `json:"issue"`
	AuthorityRef       AuthorityRef `json:"authority_ref"`
	Contract           WorkContract `json:"contract"`
	Created            bool         `json:"created"`
}

func BootstrapIssueBranchContract(ctx context.Context, root project.Root, resolver PathResolver, options BootstrapIssueBranchContractOptions) (BootstrapIssueBranchContractResult, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}
	defer store.Close()
	return store.BootstrapIssueBranchContract(ctx, root, options)
}

func (s *Store) BootstrapIssueBranchContract(ctx context.Context, root project.Root, options BootstrapIssueBranchContractOptions) (BootstrapIssueBranchContractResult, error) {
	issueID := strings.TrimSpace(options.IssueID)
	branch := strings.TrimSpace(options.Branch)
	if issueID == "" || branch == "" {
		return BootstrapIssueBranchContractResult{}, fmt.Errorf("bootstrap branch contract requires issue id and branch")
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return BootstrapIssueBranchContractResult{}, fmt.Errorf("begin branch bootstrap: %w", err)
	}
	defer tx.Rollback()

	issue, err := loadIssueTx(ctx, tx, projectID, issueID)
	if err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}
	issueAlias := ""
	if err := tx.QueryRowContext(ctx, `
SELECT alias FROM aliases
WHERE project_id = ? AND namespace = ? AND entity_id = ?
`, projectID, issueNamespace, issueID).Scan(&issueAlias); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return BootstrapIssueBranchContractResult{}, fmt.Errorf("lookup issue alias: %w", err)
	}

	authorityRef := AuthorityRef{Provider: AuthorityProviderBranch, Key: branch}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	criteria := issueCriteriaToInputs(issue.Criteria)
	created := false

	contract, err := loadWorkContractByRefTx(ctx, tx, projectID, authorityRef)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `
UPDATE work_contracts
SET title = ?, body = ?, fog = ?, status = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, issue.Title, issue.Body, emptyToNil(strings.TrimSpace(issue.Fog)), issue.Status, now, projectID, contract.ID); err != nil {
			return BootstrapIssueBranchContractResult{}, fmt.Errorf("refresh bootstrapped contract: %w", err)
		}
		if err := replaceWorkContractCriteriaTx(ctx, tx, projectID, contract.ID, criteria, now); err != nil {
			return BootstrapIssueBranchContractResult{}, err
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
			return BootstrapIssueBranchContractResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE work_contracts SET status = ?, updated_at = ? WHERE project_id = ? AND id = ?`, issue.Status, now, projectID, contract.ID); err != nil {
			return BootstrapIssueBranchContractResult{}, fmt.Errorf("set bootstrapped contract status: %w", err)
		}
		created = true
	default:
		return BootstrapIssueBranchContractResult{}, err
	}

	if strings.TrimSpace(options.StartedWorktree) != "" {
		if err := upsertWorkContractWorkspaceTx(ctx, tx, projectID, contract.ID, branch, strings.TrimSpace(options.StartedWorktree), now); err != nil {
			return BootstrapIssueBranchContractResult{}, fmt.Errorf("record bootstrapped workspace: %w", err)
		}
	}

	receiptID, err := newOpaqueStateID("wcr")
	if err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO work_contract_receipts (id, project_id, provider, provider_ref, receipt_kind, receipt_value, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, provider, provider_ref, receipt_kind) DO UPDATE SET
  receipt_value = excluded.receipt_value,
  updated_at = excluded.updated_at
`, receiptID, projectID, authorityRef.Provider, authorityRef.Key, BootstrapReceiptKind, issueID, now, now); err != nil {
		return BootstrapIssueBranchContractResult{}, fmt.Errorf("record bootstrap receipt: %w", err)
	}
	if err := upsertWorkContractMappingTx(ctx, tx, projectID, authorityRef, RenderOutMappingIssueID, issueID, now); err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}
	if issueAlias != "" {
		if err := upsertWorkContractMappingTx(ctx, tx, projectID, authorityRef, RenderOutMappingIssueAlias, issueAlias, now); err != nil {
			return BootstrapIssueBranchContractResult{}, err
		}
	}

	contract, err = loadWorkContractTx(ctx, tx, projectID, contract.ID)
	if err != nil {
		return BootstrapIssueBranchContractResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return BootstrapIssueBranchContractResult{}, fmt.Errorf("commit branch bootstrap: %w", err)
	}

	return BootstrapIssueBranchContractResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Issue:              issue,
		AuthorityRef:       authorityRef,
		Contract:           contract,
		Created:            created,
	}, nil
}
