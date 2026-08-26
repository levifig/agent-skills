package state

import (
	"context"
	"fmt"

	"github.com/levifig/loaf/internal/project"
)

// ListWorkContractFrontier returns open (triage/backlog/todo) work contracts
// as provider-qualified refs. The view is derived at read time.
func ListWorkContractFrontier(ctx context.Context, root project.Root, resolver PathResolver) ([]WorkContractSummary, error) {
	store, err := openProjectStoreReadExisting(ctx, root, resolver)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListWorkContractFrontier(ctx, root)
}

// ListWorkContractFrontier returns open work-contract refs from an open store.
func (s *Store) ListWorkContractFrontier(ctx context.Context, root project.Root) ([]WorkContractSummary, error) {
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return nil, err
	}
	return s.listWorkContractFrontier(ctx, projectID)
}

func (s *Store) listWorkContractFrontier(ctx context.Context, projectID string) ([]WorkContractSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, provider, provider_ref, kind, title, status
FROM work_contracts
WHERE project_id = ?
  AND status IN (?, ?, ?)
ORDER BY created_at, id
`, projectID, IssueStatusTriage, IssueStatusBacklog, IssueStatusTodo)
	if err != nil {
		return nil, fmt.Errorf("query work contract frontier: %w", err)
	}
	defer rows.Close()

	refs := []WorkContractSummary{}
	for rows.Next() {
		var summary WorkContractSummary
		if err := rows.Scan(&summary.ID, &summary.AuthorityRef.Provider, &summary.AuthorityRef.Key, &summary.Kind, &summary.Title, &summary.Status); err != nil {
			return nil, fmt.Errorf("scan work contract frontier: %w", err)
		}
		refs = append(refs, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate work contract frontier: %w", err)
	}
	return refs, nil
}
