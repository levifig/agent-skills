package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func upsertWorkContractMappingTx(ctx context.Context, tx *sql.Tx, projectID string, ref AuthorityRef, kind, value, now string) error {
	var mappingID string
	err := tx.QueryRowContext(ctx, `
SELECT id FROM work_contract_mappings
WHERE project_id = ? AND provider = ? AND provider_ref = ? AND mapping_kind = ?
`, projectID, ref.Provider, ref.Key, kind).Scan(&mappingID)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `
UPDATE work_contract_mappings
SET mapping_value = ?, updated_at = ?
WHERE id = ?
`, value, now, mappingID); err != nil {
			return fmt.Errorf("update work-contract mapping: %w", err)
		}
	case errors.Is(err, sql.ErrNoRows):
		mappingID, err = newOpaqueStateID("wcm")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO work_contract_mappings (id, project_id, provider, provider_ref, mapping_kind, mapping_value, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, mappingID, projectID, ref.Provider, ref.Key, kind, value, now, now); err != nil {
			return fmt.Errorf("insert work-contract mapping: %w", err)
		}
	default:
		return fmt.Errorf("lookup work-contract mapping: %w", err)
	}
	_, err = appendCoreEventFactTx(ctx, tx, projectID, FactKindRefRegistered, "", CoreEventPayload{
		SubjectKind:  "ref",
		SubjectID:    mappingID,
		Provider:     ref.Provider,
		ProviderRef:  ref.Key,
		MappingKind:  kind,
		MappingValue: value,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, parseCoreEventTime(now), "")
	return err
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
