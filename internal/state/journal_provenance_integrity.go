package state

import (
	"context"
	"database/sql"
	"fmt"
)

// JournalProvenanceIntegrity reports project-scoped endpoint integrity for
// optional journal origins and deferred-intent rows.
type JournalProvenanceIntegrity struct {
	OriginRows                int  `json:"origin_rows"`
	OriginMissingJournal      int  `json:"origin_missing_journal"`
	OriginProjectMismatches   int  `json:"origin_project_mismatches"`
	DeferralRows              int  `json:"deferral_rows"`
	DeferralMissingDecision   int  `json:"deferral_missing_decision"`
	DeferralMissingSpark      int  `json:"deferral_missing_spark"`
	DeferralProjectMismatches int  `json:"deferral_project_mismatches"`
	Ready                     bool `json:"ready"`
}

// InspectJournalProvenanceIntegrity performs a read-only project-endpoint
// audit for optional journal provenance tables.
func InspectJournalProvenanceIntegrity(ctx context.Context, store *Store) (JournalProvenanceIntegrity, error) {
	if store == nil || store.db == nil {
		return JournalProvenanceIntegrity{}, fmt.Errorf("inspect journal provenance: store is nil")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return JournalProvenanceIntegrity{}, fmt.Errorf("inspect journal provenance: begin read transaction: %w", err)
	}
	defer tx.Rollback()
	return inspectJournalProvenanceIntegrity(ctx, tx)
}

func inspectJournalProvenanceIntegrity(ctx context.Context, queryer queryContext) (JournalProvenanceIntegrity, error) {
	if queryer == nil {
		return JournalProvenanceIntegrity{}, fmt.Errorf("inspect journal provenance: queryer is nil")
	}
	result := JournalProvenanceIntegrity{Ready: true}
	origins, err := provenanceTableExists(ctx, queryer, "journal_origins")
	if err != nil {
		return JournalProvenanceIntegrity{}, err
	}
	if origins {
		if err := queryProvenanceCount(ctx, queryer, `SELECT COUNT(*) FROM journal_origins`, &result.OriginRows); err != nil {
			return JournalProvenanceIntegrity{}, err
		}
		if err := queryProvenanceCount(ctx, queryer, `
SELECT COUNT(*) FROM journal_origins AS o
WHERE NOT EXISTS (
  SELECT 1 FROM journal_entries AS j
  WHERE j.id = o.journal_entry_id AND j.project_id = o.project_id
)`, &result.OriginMissingJournal); err != nil {
			return JournalProvenanceIntegrity{}, fmt.Errorf("inspect origin journal endpoints: %w", err)
		}
		if err := queryProvenanceCount(ctx, queryer, `
SELECT COUNT(*) FROM journal_origins AS o
JOIN journal_entries AS j ON j.id = o.journal_entry_id
WHERE j.project_id <> o.project_id`, &result.OriginProjectMismatches); err != nil {
			return JournalProvenanceIntegrity{}, fmt.Errorf("inspect origin project scope: %w", err)
		}
	}
	deferrals, err := provenanceTableExists(ctx, queryer, "journal_deferrals")
	if err != nil {
		return JournalProvenanceIntegrity{}, err
	}
	if deferrals {
		if err := queryProvenanceCount(ctx, queryer, `SELECT COUNT(*) FROM journal_deferrals`, &result.DeferralRows); err != nil {
			return JournalProvenanceIntegrity{}, err
		}
		if err := queryProvenanceCount(ctx, queryer, `
SELECT COUNT(*) FROM journal_deferrals AS d
WHERE NOT EXISTS (
  SELECT 1 FROM journal_entries AS j
  WHERE j.id = d.journal_entry_id AND j.project_id = d.project_id
)`, &result.DeferralMissingDecision); err != nil {
			return JournalProvenanceIntegrity{}, fmt.Errorf("inspect deferral decision endpoints: %w", err)
		}
		if err := queryProvenanceCount(ctx, queryer, `
SELECT COUNT(*) FROM journal_deferrals AS d
WHERE NOT EXISTS (
  SELECT 1 FROM sparks AS s
  WHERE s.id = d.spark_id AND s.project_id = d.project_id
)`, &result.DeferralMissingSpark); err != nil {
			return JournalProvenanceIntegrity{}, fmt.Errorf("inspect deferral spark endpoints: %w", err)
		}
		if err := queryProvenanceCount(ctx, queryer, `
SELECT COUNT(*) FROM journal_deferrals AS d
WHERE EXISTS (SELECT 1 FROM journal_entries AS j WHERE j.id = d.journal_entry_id AND j.project_id <> d.project_id)
   OR EXISTS (SELECT 1 FROM sparks AS s WHERE s.id = d.spark_id AND s.project_id <> d.project_id)`, &result.DeferralProjectMismatches); err != nil {
			return JournalProvenanceIntegrity{}, fmt.Errorf("inspect deferral project scope: %w", err)
		}
	}
	result.Ready = result.OriginMissingJournal == 0 && result.OriginProjectMismatches == 0 && result.DeferralMissingDecision == 0 && result.DeferralMissingSpark == 0 && result.DeferralProjectMismatches == 0
	return result, nil
}

func provenanceTableExists(ctx context.Context, queryer queryContext, table string) (bool, error) {
	var count int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect provenance table %s: %w", table, err)
	}
	return count != 0, nil
}

func queryProvenanceCount(ctx context.Context, queryer queryContext, query string, destination *int) error {
	if err := queryer.QueryRowContext(ctx, query).Scan(destination); err != nil {
		return fmt.Errorf("count provenance rows: %w", err)
	}
	return nil
}
