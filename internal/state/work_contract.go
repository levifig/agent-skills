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

// WorkContract is the ref-keyed contract read model.
type WorkContract struct {
	ID               string                  `json:"id"`
	AuthorityRef     AuthorityRef            `json:"authority_ref"`
	ParentContractID string                  `json:"parent_contract_id,omitempty"`
	Kind             string                  `json:"kind"`
	Title            string                  `json:"title"`
	Body             string                  `json:"body"`
	Fog              string                  `json:"fog,omitempty"`
	Status           string                  `json:"status"`
	StartedBranch    string                  `json:"started_branch,omitempty"`
	StartedWorktree  string                  `json:"started_worktree,omitempty"`
	WorktreeMissing  bool                    `json:"worktree_missing,omitempty"`
	Criteria         []WorkContractCriterion `json:"criteria,omitempty"`
	Mappings         []WorkContractMapping   `json:"mappings,omitempty"`
	Receipts         []WorkContractReceipt   `json:"receipts,omitempty"`
	CreatedAt        string                  `json:"created_at"`
	UpdatedAt        string                  `json:"updated_at"`
}

// WorkContractCreateOptions describes a new ref-keyed contract.
type WorkContractCreateOptions struct {
	AuthorityRef AuthorityRef
	Title        string
	Body         string
	Fog          string
	Kind         string
	Parent       AuthorityRef
	Criteria     []IssueCriterionInput
}

// WorkContractUpdateOptions describes a partial contract mutation.
type WorkContractUpdateOptions struct {
	AuthorityRef    AuthorityRef
	Title           string
	SetTitle        bool
	Body            string
	SetBody         bool
	Fog             string
	SetFog          bool
	Kind            string
	SetKind         bool
	Status          string
	SetStatus       bool
	StartedBranch   string
	StartedWorktree string
	SetStarted      bool
}

// WorkContractResult is one contract plus project identity for CLI JSON.
type WorkContractResult struct {
	ContractVersion    int                   `json:"contract_version,omitempty"`
	DatabaseScope      string                `json:"database_scope,omitempty"`
	DatabasePath       string                `json:"database_path,omitempty"`
	ProjectID          string                `json:"project_id,omitempty"`
	ProjectName        string                `json:"project_name,omitempty"`
	ProjectCurrentPath string                `json:"project_current_path,omitempty"`
	Contract           WorkContract          `json:"contract"`
	Parent             *WorkContractSummary  `json:"parent,omitempty"`
	Children           []WorkContractSummary `json:"children,omitempty"`
}

// WorkContractSummary is a compact contract row.
type WorkContractSummary struct {
	ID           string       `json:"id"`
	AuthorityRef AuthorityRef `json:"authority_ref"`
	Kind         string       `json:"kind"`
	Title        string       `json:"title"`
	Status       string       `json:"status"`
}

// CreateWorkContract writes a contract keyed by authority ref.
func CreateWorkContract(ctx context.Context, root project.Root, resolver PathResolver, options WorkContractCreateOptions) (WorkContract, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return WorkContract{}, err
	}
	defer store.Close()
	return store.CreateWorkContract(ctx, root, options)
}

func (s *Store) CreateWorkContract(ctx context.Context, root project.Root, options WorkContractCreateOptions) (WorkContract, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContract{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkContract{}, fmt.Errorf("begin work contract: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	contract, err := createWorkContractTx(ctx, tx, projectID, options, now)
	if err != nil {
		return WorkContract{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkContract{}, fmt.Errorf("commit work contract: %w", err)
	}
	return contract, nil
}

func createWorkContractTx(ctx context.Context, tx *sql.Tx, projectID string, options WorkContractCreateOptions, now string) (WorkContract, error) {
	if options.AuthorityRef.Provider == "" || options.AuthorityRef.Key == "" {
		return WorkContract{}, fmt.Errorf("work contract requires a provider-qualified authority ref")
	}
	title, err := normalizeIssueTitle(options.Title)
	if err != nil {
		return WorkContract{}, err
	}
	kind, err := normalizeIssueKind(options.Kind)
	if err != nil {
		return WorkContract{}, err
	}
	criteria, err := normalizeIssueCriteria(options.Criteria)
	if err != nil {
		return WorkContract{}, err
	}

	contractID, err := newOpaqueStateID("wct")
	if err != nil {
		return WorkContract{}, err
	}

	parentID := ""
	if options.Parent.Provider != "" && options.Parent.Key != "" {
		parent, err := loadWorkContractByRefTx(ctx, tx, projectID, options.Parent)
		if err != nil {
			return WorkContract{}, err
		}
		parentID = parent.ID
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO work_contracts (id, project_id, provider, provider_ref, kind, title, body, fog, status, parent_contract_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, contractID, projectID, options.AuthorityRef.Provider, options.AuthorityRef.Key, kind, title, options.Body, emptyToNil(strings.TrimSpace(options.Fog)), IssueStatusTriage, emptyToNil(parentID), now, now); err != nil {
		return WorkContract{}, fmt.Errorf("insert work contract: %w", err)
	}
	if err := replaceWorkContractCriteriaTx(ctx, tx, projectID, contractID, criteria, now); err != nil {
		return WorkContract{}, err
	}
	return loadWorkContractTx(ctx, tx, projectID, contractID)
}

// ShowWorkContract returns one contract by authority ref.
func ShowWorkContract(ctx context.Context, root project.Root, resolver PathResolver, rawRef string) (WorkContractResult, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return WorkContractResult{}, err
	}
	defer store.Close()
	return store.ShowWorkContract(ctx, root, rawRef)
}

func (s *Store) ShowWorkContract(ctx context.Context, root project.Root, rawRef string) (WorkContractResult, error) {
	authorityRef, err := ParseAuthorityRef(rawRef)
	if err != nil {
		return WorkContractResult{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContractResult{}, err
	}
	identity, err := s.projectIdentity(ctx, projectID)
	if err != nil {
		return WorkContractResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return WorkContractResult{}, fmt.Errorf("begin show work contract: %w", err)
	}
	defer tx.Rollback()

	contract, err := loadWorkContractByRefTx(ctx, tx, projectID, authorityRef)
	if err != nil {
		return WorkContractResult{}, err
	}
	children, err := listWorkContractChildrenTx(ctx, tx, projectID, contract.ID)
	if err != nil {
		return WorkContractResult{}, err
	}
	var parent *WorkContractSummary
	if contract.ParentContractID != "" {
		parentContract, err := loadWorkContractTx(ctx, tx, projectID, contract.ParentContractID)
		if err != nil {
			return WorkContractResult{}, err
		}
		parent = &WorkContractSummary{
			ID:           parentContract.ID,
			AuthorityRef: parentContract.AuthorityRef,
			Kind:         parentContract.Kind,
			Title:        parentContract.Title,
			Status:       parentContract.Status,
		}
	}
	return WorkContractResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      identity.DatabaseScope,
		DatabasePath:       identity.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Contract:           contract,
		Parent:             parent,
		Children:           children,
	}, nil
}

// UpdateWorkContract mutates a ref-keyed contract.
func UpdateWorkContract(ctx context.Context, root project.Root, resolver PathResolver, options WorkContractUpdateOptions) (WorkContract, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return WorkContract{}, err
	}
	defer store.Close()
	return store.UpdateWorkContract(ctx, root, options)
}

func (s *Store) UpdateWorkContract(ctx context.Context, root project.Root, options WorkContractUpdateOptions) (WorkContract, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContract{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkContract{}, fmt.Errorf("begin update work contract: %w", err)
	}
	defer tx.Rollback()

	contract, err := loadWorkContractByRefTx(ctx, tx, projectID, options.AuthorityRef)
	if err != nil {
		return WorkContract{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if options.SetTitle {
		title, err := normalizeIssueTitle(options.Title)
		if err != nil {
			return WorkContract{}, err
		}
		contract.Title = title
	}
	if options.SetBody {
		contract.Body = options.Body
	}
	if options.SetFog {
		contract.Fog = strings.TrimSpace(options.Fog)
	}
	if options.SetKind {
		kind, err := normalizeIssueKind(options.Kind)
		if err != nil {
			return WorkContract{}, err
		}
		contract.Kind = kind
	}
	if options.SetStatus {
		status := strings.TrimSpace(options.Status)
		if !validIssueStatus(status) {
			return WorkContract{}, &IssueValidationError{Field: "status", Err: fmt.Errorf("invalid status %q", status)}
		}
		contract.Status = status
	}
	if options.SetStarted {
		branch := strings.TrimSpace(options.StartedBranch)
		worktree := strings.TrimSpace(options.StartedWorktree)
		if (branch == "") != (worktree == "") {
			return WorkContract{}, &IssueValidationError{Field: "started", Err: fmt.Errorf("started_branch and started_worktree must be set or cleared together")}
		}
		contract.StartedBranch = branch
		contract.StartedWorktree = worktree
		if err := upsertWorkContractWorkspaceTx(ctx, tx, projectID, contract.ID, branch, worktree, now); err != nil {
			return WorkContract{}, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE work_contracts
SET kind = ?, title = ?, body = ?, fog = ?, status = ?, updated_at = ?
WHERE project_id = ? AND id = ?
`, contract.Kind, contract.Title, contract.Body, emptyToNil(contract.Fog), contract.Status, now, projectID, contract.ID); err != nil {
		return WorkContract{}, fmt.Errorf("update work contract: %w", err)
	}

	updated, err := loadWorkContractTx(ctx, tx, projectID, contract.ID)
	if err != nil {
		return WorkContract{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkContract{}, fmt.Errorf("commit update work contract: %w", err)
	}
	return updated, nil
}

func loadWorkContractByRefTx(ctx context.Context, tx *sql.Tx, projectID string, ref AuthorityRef) (WorkContract, error) {
	var contractID string
	err := tx.QueryRowContext(ctx, `
SELECT id FROM work_contracts WHERE project_id = ? AND provider = ? AND provider_ref = ?
`, projectID, ref.Provider, ref.Key).Scan(&contractID)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkContract{}, fmt.Errorf("work contract %s not found", ref.String())
	}
	if err != nil {
		return WorkContract{}, fmt.Errorf("resolve work contract %s: %w", ref.String(), err)
	}
	return loadWorkContractTx(ctx, tx, projectID, contractID)
}

func loadWorkContractTx(ctx context.Context, tx *sql.Tx, projectID, contractID string) (WorkContract, error) {
	var contract WorkContract
	var parentID, fog sql.NullString
	err := tx.QueryRowContext(ctx, `
SELECT id, provider, provider_ref, parent_contract_id, kind, title, body, fog, status, created_at, updated_at
FROM work_contracts WHERE project_id = ? AND id = ?
`, projectID, contractID).Scan(
		&contract.ID, &contract.AuthorityRef.Provider, &contract.AuthorityRef.Key,
		&parentID, &contract.Kind, &contract.Title, &contract.Body, &fog,
		&contract.Status, &contract.CreatedAt, &contract.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkContract{}, fmt.Errorf("work contract %s not found", contractID)
	}
	if err != nil {
		return WorkContract{}, err
	}
	contract.ParentContractID = parentID.String
	contract.Fog = fog.String

	branch, worktree, err := loadWorkContractWorkspaceTx(ctx, tx, projectID, contractID)
	if err != nil {
		return WorkContract{}, err
	}
	contract.StartedBranch = branch
	contract.StartedWorktree = worktree

	criteria, err := loadWorkContractCriteriaTx(ctx, tx, projectID, contractID)
	if err != nil {
		return WorkContract{}, err
	}
	contract.Criteria = criteria

	mappings, err := loadWorkContractMappingsTx(ctx, tx, projectID, contract.AuthorityRef)
	if err != nil {
		return WorkContract{}, err
	}
	contract.Mappings = mappings

	receipts, err := loadWorkContractReceiptsTx(ctx, tx, projectID, contract.AuthorityRef)
	if err != nil {
		return WorkContract{}, err
	}
	contract.Receipts = receipts

	return contract, nil
}

func listWorkContractChildrenTx(ctx context.Context, tx *sql.Tx, projectID, contractID string) ([]WorkContractSummary, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, provider, provider_ref, kind, title, status
FROM work_contracts
WHERE project_id = ? AND parent_contract_id = ?
ORDER BY created_at, id
`, projectID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var children []WorkContractSummary
	for rows.Next() {
		var child WorkContractSummary
		if err := rows.Scan(&child.ID, &child.AuthorityRef.Provider, &child.AuthorityRef.Key, &child.Kind, &child.Title, &child.Status); err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return children, rows.Err()
}

// RenderWorkContractMarkdown renders shaping output for a ref-keyed contract.
func RenderWorkContractMarkdown(result WorkContractResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", result.Contract.Title)
	if strings.TrimSpace(result.Contract.Body) != "" {
		fmt.Fprintln(&b)
		b.WriteString(result.Contract.Body)
		if !strings.HasSuffix(result.Contract.Body, "\n") {
			fmt.Fprintln(&b)
		}
	}
	if len(result.Contract.Criteria) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Definition of Done")
		fmt.Fprintln(&b)
		checked := result.Contract.Status == IssueStatusDone
		for _, criterion := range result.Contract.Criteria {
			mark := " "
			if checked {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s\n", mark, criterion.Text)
		}
	}
	if len(result.Children) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Children")
		fmt.Fprintln(&b)
		for _, child := range result.Children {
			fmt.Fprintf(&b, "- %s: %s\n", child.AuthorityRef.String(), child.Title)
		}
	}
	return b.String()
}
