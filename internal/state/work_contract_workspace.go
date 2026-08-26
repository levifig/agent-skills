package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func upsertWorkContractWorkspaceTx(ctx context.Context, tx *sql.Tx, projectID, contractID, branch, worktree, now string) error {
	if branch == "" && worktree == "" {
		_, err := tx.ExecContext(ctx, `DELETE FROM work_contract_workspace WHERE project_id = ? AND contract_id = ?`, projectID, contractID)
		return err
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO work_contract_workspace (contract_id, project_id, started_branch, started_worktree, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(contract_id) DO UPDATE SET
  started_branch = excluded.started_branch,
  started_worktree = excluded.started_worktree,
  updated_at = excluded.updated_at
`, contractID, projectID, branch, worktree, now, now)
	return err
}

func loadWorkContractWorkspaceTx(ctx context.Context, tx *sql.Tx, projectID, contractID string) (branch, worktree string, err error) {
	var startedBranch, startedWorktree sql.NullString
	err = tx.QueryRowContext(ctx, `
SELECT started_branch, started_worktree FROM work_contract_workspace WHERE project_id = ? AND contract_id = ?
`, projectID, contractID).Scan(&startedBranch, &startedWorktree)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("load work contract workspace: %w", err)
	}
	return startedBranch.String, startedWorktree.String, nil
}
