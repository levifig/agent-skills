package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// WorkContractMapping is a Loaf mapping fact for an authority ref.
type WorkContractMapping struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// UpsertWorkContractMapping stores a mapping fact for an authority ref.
func UpsertWorkContractMapping(ctx context.Context, root project.Root, resolver PathResolver, rawRef, kind, value string) error {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.UpsertWorkContractMapping(ctx, root, rawRef, kind, value)
}

func (s *Store) UpsertWorkContractMapping(ctx context.Context, root project.Root, rawRef, kind, value string) error {
	authorityRef, err := ParseAuthorityRef(rawRef)
	if err != nil {
		return err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertWorkContractMappingTx(ctx, tx, projectID, authorityRef, kind, value, now); err != nil {
		return err
	}
	return tx.Commit()
}

func loadWorkContractMappingsTx(ctx context.Context, tx *sql.Tx, projectID string, ref AuthorityRef) ([]WorkContractMapping, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT mapping_kind, mapping_value FROM work_contract_mappings
WHERE project_id = ? AND provider = ? AND provider_ref = ?
ORDER BY mapping_kind
`, projectID, ref.Provider, ref.Key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mappings []WorkContractMapping
	for rows.Next() {
		var item WorkContractMapping
		if err := rows.Scan(&item.Kind, &item.Value); err != nil {
			return nil, err
		}
		mappings = append(mappings, item)
	}
	return mappings, rows.Err()
}
