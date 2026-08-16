package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// IssueCriterionClaim is a child criterion claiming a parent criterion.
type IssueCriterionClaim struct {
	ID                string `json:"id"`
	ChildCriterionID  string `json:"child_criterion_id"`
	ParentCriterionID string `json:"parent_criterion_id"`
}

// ClaimIssueCriterion records that the child's criterion at childPosition
// serves the parent's criterion at parentPosition. Positions resolve to IDs
// at write time.
func ClaimIssueCriterion(ctx context.Context, root project.Root, resolver PathResolver, childRef string, childPosition, parentPosition int) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.ClaimIssueCriterion(ctx, root, childRef, childPosition, parentPosition)
}

// ClaimIssueCriterion records a claim on an open store.
func (s *Store) ClaimIssueCriterion(ctx context.Context, root project.Root, childRef string, childPosition, parentPosition int) (Issue, error) {
	if childPosition < 1 {
		return Issue{}, &IssueValidationError{Field: "child_position", Err: fmt.Errorf("must be >= 1")}
	}
	if parentPosition < 1 {
		return Issue{}, &IssueValidationError{Field: "parent_position", Err: fmt.Errorf("must be >= 1")}
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	childID, _, err := resolveIssueRefTx(ctx, tx, projectID, childRef)
	if err != nil {
		return Issue{}, err
	}
	child, err := loadIssueTx(ctx, tx, projectID, childID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read child", Err: err}
	}
	if child.ParentID == "" {
		return Issue{}, &IssueValidationError{Field: "parent", Err: fmt.Errorf("issue %s has no parent", firstNonEmpty(child.Alias, child.ID))}
	}
	childCriterion, err := criterionAtPosition(child.Criteria, childPosition, "child_position")
	if err != nil {
		return Issue{}, err
	}
	parent, err := loadIssueTx(ctx, tx, projectID, child.ParentID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read parent", Err: err}
	}
	parentCriterion, err := criterionAtPosition(parent.Criteria, parentPosition, "parent_position")
	if err != nil {
		return Issue{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := insertIssueCriterionClaimTx(ctx, tx, projectID, childCriterion.ID, parentCriterion.ID, now); err != nil {
		return Issue{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, childID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "touch issue", Err: err}
	}
	detail, err := loadIssueTx(ctx, tx, projectID, childID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return detail, nil
}

// UnclaimIssueCriterion removes the claim from the child's criterion at
// childPosition to the parent's criterion at parentPosition.
func UnclaimIssueCriterion(ctx context.Context, root project.Root, resolver PathResolver, childRef string, childPosition, parentPosition int) (Issue, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return Issue{}, err
	}
	defer store.Close()
	return store.UnclaimIssueCriterion(ctx, root, childRef, childPosition, parentPosition)
}

// UnclaimIssueCriterion removes a claim on an open store.
func (s *Store) UnclaimIssueCriterion(ctx context.Context, root project.Root, childRef string, childPosition, parentPosition int) (Issue, error) {
	if childPosition < 1 {
		return Issue{}, &IssueValidationError{Field: "child_position", Err: fmt.Errorf("must be >= 1")}
	}
	if parentPosition < 1 {
		return Issue{}, &IssueValidationError{Field: "parent_position", Err: fmt.Errorf("must be >= 1")}
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return Issue{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "begin", Err: err}
	}
	defer tx.Rollback()

	childID, _, err := resolveIssueRefTx(ctx, tx, projectID, childRef)
	if err != nil {
		return Issue{}, err
	}
	child, err := loadIssueTx(ctx, tx, projectID, childID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read child", Err: err}
	}
	if child.ParentID == "" {
		return Issue{}, &IssueValidationError{Field: "parent", Err: fmt.Errorf("issue %s has no parent", firstNonEmpty(child.Alias, child.ID))}
	}
	childCriterion, err := criterionAtPosition(child.Criteria, childPosition, "child_position")
	if err != nil {
		return Issue{}, err
	}
	parent, err := loadIssueTx(ctx, tx, projectID, child.ParentID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read parent", Err: err}
	}
	parentCriterion, err := criterionAtPosition(parent.Criteria, parentPosition, "parent_position")
	if err != nil {
		return Issue{}, err
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM issue_criterion_claims
WHERE project_id = ? AND child_criterion_id = ? AND parent_criterion_id = ?
`, projectID, childCriterion.ID, parentCriterion.ID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "unclaim", Err: err}
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "unclaim rows", Err: err}
	}
	if affected == 0 {
		return Issue{}, &IssueValidationError{Field: "claim", Err: fmt.Errorf("no claim from child criterion %d to parent criterion %d", childPosition, parentPosition)}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE issues SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, childID); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "touch issue", Err: err}
	}
	detail, err := loadIssueTx(ctx, tx, projectID, childID)
	if err != nil {
		return Issue{}, &IssueTransactionError{Stage: "read result", Err: err}
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, &IssueTransactionError{Stage: "commit", Err: err}
	}
	return detail, nil
}

func insertIssueCriterionClaimTx(ctx context.Context, tx *sql.Tx, projectID, childCriterionID, parentCriterionID, now string) error {
	if childCriterionID == parentCriterionID {
		return &IssueValidationError{Field: "claim", Err: fmt.Errorf("a criterion cannot claim itself")}
	}
	id, err := newOpaqueStateID("icc")
	if err != nil {
		return &IssueTransactionError{Stage: "claim id", Err: err}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO issue_criterion_claims (id, project_id, child_criterion_id, parent_criterion_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (child_criterion_id, parent_criterion_id) DO NOTHING
`, id, projectID, childCriterionID, parentCriterionID, now, now); err != nil {
		return &IssueTransactionError{Stage: "claim", Err: err}
	}
	return nil
}

func listIssueCriterionClaimsForChildrenTx(ctx context.Context, tx *sql.Tx, projectID string, childIDs []string) ([]IssueCriterionClaim, error) {
	if len(childIDs) == 0 {
		return nil, nil
	}
	query := `
SELECT cl.id, cl.child_criterion_id, cl.parent_criterion_id
FROM issue_criterion_claims AS cl
JOIN issue_criteria AS child ON child.id = cl.child_criterion_id
WHERE cl.project_id = ? AND child.issue_id IN (` + sqlPlaceholders(len(childIDs)) + `)
ORDER BY cl.created_at, cl.id
`
	args := make([]any, 0, 1+len(childIDs))
	args = append(args, projectID)
	for _, id := range childIDs {
		args = append(args, id)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list criterion claims: %w", err)
	}
	defer rows.Close()
	claims := []IssueCriterionClaim{}
	for rows.Next() {
		var claim IssueCriterionClaim
		if err := rows.Scan(&claim.ID, &claim.ChildCriterionID, &claim.ParentCriterionID); err != nil {
			return nil, fmt.Errorf("scan criterion claim: %w", err)
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate criterion claims: %w", err)
	}
	return claims, nil
}

func criterionAtPosition(criteria []IssueCriterion, position int, field string) (IssueCriterion, error) {
	for _, criterion := range criteria {
		if criterion.Position == position {
			return criterion, nil
		}
	}
	return IssueCriterion{}, &IssueValidationError{Field: field, Err: fmt.Errorf("criterion position %d not found", position)}
}

func sqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := "?"
	for i := 1; i < n; i++ {
		out += ",?"
	}
	return out
}
