package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// WorkContractCriterion is one stored definition-of-done line on a contract.
type WorkContractCriterion struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Text     string `json:"text"`
	Command  string `json:"command,omitempty"`
	Expect   string `json:"expect,omitempty"`
	Tier     string `json:"tier"`
}

// ReplaceWorkContractCriteria replaces every criterion on a ref-keyed contract.
func ReplaceWorkContractCriteria(ctx context.Context, root project.Root, resolver PathResolver, rawRef string, inputs []IssueCriterionInput) (WorkContract, error) {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return WorkContract{}, err
	}
	defer store.Close()
	return store.ReplaceWorkContractCriteria(ctx, root, rawRef, inputs)
}

func (s *Store) ReplaceWorkContractCriteria(ctx context.Context, root project.Root, rawRef string, inputs []IssueCriterionInput) (WorkContract, error) {
	authorityRef, err := ParseAuthorityRef(rawRef)
	if err != nil {
		return WorkContract{}, err
	}
	criteria, err := normalizeIssueCriteria(inputs)
	if err != nil {
		return WorkContract{}, err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return WorkContract{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return WorkContract{}, fmt.Errorf("begin replace work contract criteria: %w", err)
	}
	defer tx.Rollback()

	contract, err := loadWorkContractByRefTx(ctx, tx, projectID, authorityRef)
	if err != nil {
		return WorkContract{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := replaceWorkContractCriteriaTx(ctx, tx, projectID, contract.ID, criteria, now); err != nil {
		return WorkContract{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE work_contracts SET updated_at = ? WHERE project_id = ? AND id = ?`, now, projectID, contract.ID); err != nil {
		return WorkContract{}, err
	}
	updated, err := loadWorkContractTx(ctx, tx, projectID, contract.ID)
	if err != nil {
		return WorkContract{}, err
	}
	if err := tx.Commit(); err != nil {
		return WorkContract{}, err
	}
	return updated, nil
}

func replaceWorkContractCriteriaTx(ctx context.Context, tx *sql.Tx, projectID, contractID string, inputs []IssueCriterionInput, now string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM work_contract_criteria WHERE project_id = ? AND contract_id = ?`, projectID, contractID); err != nil {
		return fmt.Errorf("clear work contract criteria: %w", err)
	}
	for _, input := range inputs {
		criterionID, err := newOpaqueStateID("wcc")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO work_contract_criteria (id, project_id, contract_id, position, text, command, expect, tier, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, criterionID, projectID, contractID, input.Position, input.Text, emptyToNil(strings.TrimSpace(input.Command)), emptyToNil(strings.TrimSpace(input.Expect)), input.Tier, now, now); err != nil {
			return fmt.Errorf("insert work contract criterion: %w", err)
		}
	}
	return nil
}

func loadWorkContractCriteriaTx(ctx context.Context, tx *sql.Tx, projectID, contractID string) ([]WorkContractCriterion, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT id, position, text, COALESCE(command, ''), COALESCE(expect, ''), tier
FROM work_contract_criteria
WHERE project_id = ? AND contract_id = ?
ORDER BY position
`, projectID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var criteria []WorkContractCriterion
	for rows.Next() {
		var item WorkContractCriterion
		if err := rows.Scan(&item.ID, &item.Position, &item.Text, &item.Command, &item.Expect, &item.Tier); err != nil {
			return nil, err
		}
		criteria = append(criteria, item)
	}
	return criteria, rows.Err()
}
