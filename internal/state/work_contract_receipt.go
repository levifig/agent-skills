package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// WorkContractReceipt is provenance keyed to an authority ref.
type WorkContractReceipt struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// UpsertWorkContractReceipt stores a receipt fact for an authority ref.
func UpsertWorkContractReceipt(ctx context.Context, root project.Root, resolver PathResolver, rawRef, kind, value string) error {
	store, err := openProjectStoreMutateExisting(ctx, root, resolver)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.UpsertWorkContractReceipt(ctx, root, rawRef, kind, value)
}

func (s *Store) UpsertWorkContractReceipt(ctx context.Context, root project.Root, rawRef, kind, value string) error {
	authorityRef, err := ParseAuthorityRef(rawRef)
	if err != nil {
		return err
	}
	projectID, err := s.projectID(ctx, root)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	receiptID, err := newOpaqueStateID("wcr")
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
INSERT INTO work_contract_receipts (id, project_id, provider, provider_ref, receipt_kind, receipt_value, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, provider, provider_ref, receipt_kind) DO UPDATE SET
  receipt_value = excluded.receipt_value,
  updated_at = excluded.updated_at
`, receiptID, projectID, authorityRef.Provider, authorityRef.Key, kind, value, now, now)
	if err != nil {
		return err
	}
	if _, err := appendCoreEventFactTx(ctx, tx, projectID, FactKindVerificationRecorded, "", CoreEventPayload{
		SubjectKind:  "verification",
		SubjectID:    receiptID,
		Provider:     authorityRef.Provider,
		ProviderRef:  authorityRef.Key,
		ReceiptKind:  kind,
		ReceiptValue: value,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, parseCoreEventTime(now), ""); err != nil {
		return err
	}
	return tx.Commit()
}

func loadWorkContractReceiptsTx(ctx context.Context, tx *sql.Tx, projectID string, ref AuthorityRef) ([]WorkContractReceipt, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT receipt_kind, receipt_value FROM work_contract_receipts
WHERE project_id = ? AND provider = ? AND provider_ref = ?
ORDER BY receipt_kind
`, projectID, ref.Provider, ref.Key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var receipts []WorkContractReceipt
	for rows.Next() {
		var item WorkContractReceipt
		if err := rows.Scan(&item.Kind, &item.Value); err != nil {
			return nil, err
		}
		receipts = append(receipts, item)
	}
	return receipts, rows.Err()
}
