package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

type workContractCriterionClaim struct {
	ChildCriterionID  string
	ParentCriterionID string
}

// ClaimWorkContractCriterion records that a child criterion serves a parent criterion.
func ClaimWorkContractCriterion(ctx context.Context, root project.Root, resolver PathResolver, childRef string, childPosition, parentPosition int) (WorkContract, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return WorkContract{}, err
	}
	defer store.Close()
	return store.ClaimWorkContractCriterion(ctx, root, childRef, childPosition, parentPosition)
}

func (s *Store) ClaimWorkContractCriterion(ctx context.Context, root project.Root, childRef string, childPosition, parentPosition int) (WorkContract, error) {
	childAuthority, err := ParseAuthorityRef(childRef)
	if err != nil {
		return WorkContract{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContract{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkContract{}, err
	}
	defer tx.Rollback()

	childContract, err := loadWorkContractByRefTx(ctx, tx, projectID, childAuthority)
	if err != nil {
		return WorkContract{}, err
	}
	if childContract.ParentContractID == "" {
		return WorkContract{}, fmt.Errorf("work contract %s has no parent to claim against", childAuthority.String())
	}
	parentContract, err := loadWorkContractTx(ctx, tx, projectID, childContract.ParentContractID)
	if err != nil {
		return WorkContract{}, err
	}
	childCriterionID, err := workContractCriterionIDByPosition(childContract.Criteria, childPosition)
	if err != nil {
		return WorkContract{}, err
	}
	parentCriterionID, err := workContractCriterionIDByPosition(parentContract.Criteria, parentPosition)
	if err != nil {
		return WorkContract{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	claimID, err := newOpaqueStateID("wcl")
	if err != nil {
		return WorkContract{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO work_contract_criterion_claims (id, project_id, child_criterion_id, parent_criterion_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(child_criterion_id, parent_criterion_id) DO UPDATE SET updated_at = excluded.updated_at
`, claimID, projectID, childCriterionID, parentCriterionID, now, now); err != nil {
		return WorkContract{}, fmt.Errorf("claim work contract criterion: %w", err)
	}
	updated, err := loadWorkContractTx(ctx, tx, projectID, childContract.ID)
	if err != nil {
		return WorkContract{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkContract{}, err
	}
	return updated, nil
}

// UnclaimWorkContractCriterion removes a child-to-parent criterion claim.
func UnclaimWorkContractCriterion(ctx context.Context, root project.Root, resolver PathResolver, childRef string, childPosition, parentPosition int) (WorkContract, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return WorkContract{}, err
	}
	defer store.Close()
	return store.UnclaimWorkContractCriterion(ctx, root, childRef, childPosition, parentPosition)
}

func (s *Store) UnclaimWorkContractCriterion(ctx context.Context, root project.Root, childRef string, childPosition, parentPosition int) (WorkContract, error) {
	childAuthority, err := ParseAuthorityRef(childRef)
	if err != nil {
		return WorkContract{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContract{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkContract{}, err
	}
	defer tx.Rollback()

	childContract, err := loadWorkContractByRefTx(ctx, tx, projectID, childAuthority)
	if err != nil {
		return WorkContract{}, err
	}
	if childContract.ParentContractID == "" {
		return WorkContract{}, fmt.Errorf("work contract %s has no parent to unclaim against", childAuthority.String())
	}
	parentContract, err := loadWorkContractTx(ctx, tx, projectID, childContract.ParentContractID)
	if err != nil {
		return WorkContract{}, err
	}
	childCriterionID, err := workContractCriterionIDByPosition(childContract.Criteria, childPosition)
	if err != nil {
		return WorkContract{}, err
	}
	parentCriterionID, err := workContractCriterionIDByPosition(parentContract.Criteria, parentPosition)
	if err != nil {
		return WorkContract{}, err
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM work_contract_criterion_claims
WHERE project_id = ? AND child_criterion_id = ? AND parent_criterion_id = ?
`, projectID, childCriterionID, parentCriterionID); err != nil {
		return WorkContract{}, fmt.Errorf("unclaim work contract criterion: %w", err)
	}
	updated, err := loadWorkContractTx(ctx, tx, projectID, childContract.ID)
	if err != nil {
		return WorkContract{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkContract{}, err
	}
	return updated, nil
}

func workContractCriterionIDByPosition(criteria []WorkContractCriterion, position int) (string, error) {
	for _, criterion := range criteria {
		if criterion.Position == position {
			return criterion.ID, nil
		}
	}
	return "", fmt.Errorf("no criterion at position %d", position)
}

func listWorkContractCriterionClaimsForChildrenTx(ctx context.Context, tx *sql.Tx, projectID string, childContractIDs []string) ([]workContractCriterionClaim, error) {
	if len(childContractIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(childContractIDs))
	args := make([]any, 0, len(childContractIDs)+1)
	args = append(args, projectID)
	for i, id := range childContractIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`
SELECT cl.child_criterion_id, cl.parent_criterion_id
FROM work_contract_criterion_claims AS cl
JOIN work_contract_criteria AS child ON child.project_id = cl.project_id AND child.id = cl.child_criterion_id
WHERE cl.project_id = ? AND child.contract_id IN (%s)
`, joinSQLPlaceholders(placeholders))
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var claims []workContractCriterionClaim
	for rows.Next() {
		var claim workContractCriterionClaim
		if err := rows.Scan(&claim.ChildCriterionID, &claim.ParentCriterionID); err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	return claims, rows.Err()
}

func joinSQLPlaceholders(items []string) string {
	return strings.Join(items, ", ")
}
