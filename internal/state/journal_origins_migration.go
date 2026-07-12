package state

import (
	"context"
	"database/sql"
	"fmt"
)

const journalOriginsMigrationVersion = 11

func pruneOptionalProvenanceTx(ctx context.Context, tx *sql.Tx) error {
	originsExists, err := optionalTableExistsTx(ctx, tx, "journal_origins")
	if err != nil {
		return err
	}
	if originsExists {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM journal_origins
WHERE NOT EXISTS (
  SELECT 1 FROM journal_entries AS j
  WHERE j.id = journal_origins.journal_entry_id
    AND j.project_id = journal_origins.project_id
		)`); err != nil {
			return fmt.Errorf("delete origins for removed journal rows: %w", err)
		}
	}
	deferralsExists, err := optionalTableExistsTx(ctx, tx, "journal_deferrals")
	if err != nil {
		return err
	}
	if deferralsExists {
		if _, err := tx.ExecContext(ctx, `
DELETE FROM journal_deferrals
WHERE NOT EXISTS (
  SELECT 1 FROM journal_entries AS j
  WHERE j.id = journal_deferrals.journal_entry_id
    AND j.project_id = journal_deferrals.project_id
)
OR NOT EXISTS (
  SELECT 1 FROM sparks AS s
  WHERE s.id = journal_deferrals.spark_id
    AND s.project_id = journal_deferrals.project_id
		)`); err != nil {
			return fmt.Errorf("delete deferrals with missing endpoints: %w", err)
		}
	}
	var orphanOrigins, orphanDeferrals int
	if originsExists {
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM journal_origins AS o
WHERE NOT EXISTS (
  SELECT 1 FROM journal_entries AS j
  WHERE j.id = o.journal_entry_id AND j.project_id = o.project_id
)`).Scan(&orphanOrigins); err != nil {
			return fmt.Errorf("audit orphaned journal origins: %w", err)
		}
	}
	if deferralsExists {
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM journal_deferrals AS d
WHERE NOT EXISTS (
  SELECT 1 FROM journal_entries AS j
  WHERE j.id = d.journal_entry_id AND j.project_id = d.project_id
)
   OR NOT EXISTS (
  SELECT 1 FROM sparks AS s
  WHERE s.id = d.spark_id AND s.project_id = d.project_id
)`).Scan(&orphanDeferrals); err != nil {
			return fmt.Errorf("audit orphaned journal deferrals: %w", err)
		}
	}
	if orphanOrigins != 0 || orphanDeferrals != 0 {
		return fmt.Errorf("optional provenance orphan audit failed: origins=%d deferrals=%d", orphanOrigins, orphanDeferrals)
	}
	return nil
}

func optionalTableExistsTx(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&exists); err != nil {
		return false, fmt.Errorf("inspect optional table %s: %w", table, err)
	}
	return exists != 0, nil
}
