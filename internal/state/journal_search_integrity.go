package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// JournalSearchParity describes the bidirectional identity and content
// relationship between canonical journal_entries and the derived FTS table.
type JournalSearchParity struct {
	CanonicalRows int  `json:"canonical_rows"`
	IndexRows     int  `json:"index_rows"`
	Missing       int  `json:"missing"`
	Extra         int  `json:"extra"`
	Changed       int  `json:"changed"`
	Ready         bool `json:"ready"`
}

// JournalSearchDivergenceCode is the stable diagnostic code for an invalid
// canonical/derived journal search relationship.
const JournalSearchDivergenceCode = "journal-search-diverged"

// JournalSearchDivergenceError refuses trusted search output when the
// canonical journal and derived FTS index are not exactly equal.
type JournalSearchDivergenceError struct {
	Code   string              `json:"code"`
	Parity JournalSearchParity `json:"parity"`
}

func (e *JournalSearchDivergenceError) Error() string {
	if e == nil {
		return JournalSearchDivergenceCode
	}
	return fmt.Sprintf(
		"journal search index diverged from canonical journal (canonical_rows=%d, index_rows=%d, missing=%d, extra=%d, changed=%d); run: loaf state repair journal-search --dry-run",
		e.Parity.CanonicalRows,
		e.Parity.IndexRows,
		e.Parity.Missing,
		e.Parity.Extra,
		e.Parity.Changed,
	)
}

// InspectJournalSearchParity compares every canonical journal row with every
// derived FTS row without modifying the database. It supports both the
// pre-journal-first session_id and post-journal-first harness_session_id
// correlation columns by following the live journal_search schema.
func InspectJournalSearchParity(ctx context.Context, store *Store) (JournalSearchParity, error) {
	if store == nil || store.db == nil {
		return JournalSearchParity{}, fmt.Errorf("inspect journal search parity: store is nil")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return JournalSearchParity{}, fmt.Errorf("begin journal search parity snapshot: %w", err)
	}
	defer tx.Rollback()

	parity, err := inspectJournalSearchParity(ctx, tx)
	if err != nil {
		return JournalSearchParity{}, err
	}
	return parity, nil
}

func inspectJournalSearchParity(ctx context.Context, queryer queryContext) (JournalSearchParity, error) {
	if queryer == nil {
		return JournalSearchParity{}, fmt.Errorf("inspect journal search parity: queryer is nil")
	}
	correlationColumn, err := journalSearchCorrelationColumn(ctx, queryer)
	if err != nil {
		return JournalSearchParity{}, err
	}
	correlationExpression, err := journalSearchCanonicalCorrelationExpression(correlationColumn)
	if err != nil {
		return JournalSearchParity{}, err
	}
	var canonicalRows, indexRows int64
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&canonicalRows); err != nil {
		return JournalSearchParity{}, fmt.Errorf("count canonical journal rows: %w", err)
	}
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search`).Scan(&indexRows); err != nil {
		return JournalSearchParity{}, fmt.Errorf("count journal search rows: %w", err)
	}
	var duplicateProjectID, duplicateJournalEntryID string
	duplicateErr := queryer.QueryRowContext(ctx, `
SELECT project_id, id
FROM journal_entries
GROUP BY project_id, id
HAVING COUNT(*) > 1
ORDER BY project_id, id
LIMIT 1`).Scan(&duplicateProjectID, &duplicateJournalEntryID)
	if duplicateErr == nil {
		return JournalSearchParity{}, fmt.Errorf("inspect journal search parity: duplicate canonical key project_id=%s journal_entry_id=%s", duplicateProjectID, duplicateJournalEntryID)
	}
	if !errors.Is(duplicateErr, sql.ErrNoRows) {
		return JournalSearchParity{}, fmt.Errorf("inspect canonical journal keys: %w", duplicateErr)
	}

	var missing int64
	if err := queryer.QueryRowContext(ctx, `
	SELECT COUNT(*)
	FROM (
	  SELECT derived.project_id, derived.journal_entry_id
	  FROM journal_search AS derived
	  JOIN journal_entries AS canonical
	    ON canonical.project_id = derived.project_id
	   AND canonical.id = derived.journal_entry_id
	  GROUP BY derived.project_id, derived.journal_entry_id
	) AS matched_keys`).Scan(&missing); err != nil {
		return JournalSearchParity{}, fmt.Errorf("count matched journal search rows: %w", err)
	}
	missing = canonicalRows - missing
	var unmatchedExtra, duplicateExtra int64
	if err := queryer.QueryRowContext(ctx, `
	SELECT COALESCE(SUM(
	  CASE WHEN canonical.id IS NULL THEN derived_count
	       WHEN derived_count > 1 THEN derived_count - 1
	       ELSE 0 END
	), 0)
	FROM (
	  SELECT project_id, journal_entry_id, COUNT(*) AS derived_count
	  FROM journal_search
	  GROUP BY project_id, journal_entry_id
	) AS derived_groups
	LEFT JOIN journal_entries AS canonical
	  ON canonical.project_id = derived_groups.project_id
	 AND canonical.id = derived_groups.journal_entry_id`).Scan(&unmatchedExtra); err != nil {
		return JournalSearchParity{}, fmt.Errorf("count extra journal search rows: %w", err)
	}
	var changed int64
	query := fmt.Sprintf(`
SELECT COUNT(*)
FROM (
  SELECT canonical.project_id, canonical.id
  FROM journal_search AS derived
  JOIN journal_entries AS canonical
    ON canonical.project_id = derived.project_id
   AND canonical.id = derived.journal_entry_id
  GROUP BY canonical.project_id, canonical.id
  HAVING SUM(CASE WHEN derived.rowid = canonical.rowid
                   AND COALESCE(derived.%s, '') = %s
                   AND derived.entry_type = canonical.entry_type
                   AND COALESCE(derived.scope, '') = COALESCE(canonical.scope, '')
                   AND derived.message = canonical.message
                  THEN 1 ELSE 0 END) = 0
)`, correlationColumn, correlationExpression)
	if err := queryer.QueryRowContext(ctx, query).Scan(&changed); err != nil {
		return JournalSearchParity{}, fmt.Errorf("count changed journal search rows: %w", err)
	}

	parity := JournalSearchParity{
		CanonicalRows: int(canonicalRows),
		IndexRows:     int(indexRows),
		Missing:       int(missing),
		Extra:         int(unmatchedExtra + duplicateExtra),
		Changed:       int(changed),
	}
	parity.Ready = parity.CanonicalRows == parity.IndexRows && parity.Missing == 0 && parity.Extra == 0 && parity.Changed == 0
	return parity, nil
}

// rebuildJournalSearchTx replaces the derived FTS rows from canonical journal
// rows inside the caller's transaction and returns the rebuilt row count.
func rebuildJournalSearchTx(ctx context.Context, tx *sql.Tx) (int, error) {
	if tx == nil {
		return 0, fmt.Errorf("rebuild journal search: transaction is nil")
	}
	return rebuildJournalSearch(ctx, tx)
}

// rebuildAndVerifyJournalSearch rebuilds the derived FTS table from canonical
// journal rows and verifies exact parity on the same transaction/queryer. A
// caller that is already inside a transaction can therefore roll back both
// canonical and derived writes if either operation fails.
func rebuildAndVerifyJournalSearch(ctx context.Context, queryer journalSearchQueryExecer) (int, error) {
	rebuilt, err := rebuildJournalSearch(ctx, queryer)
	if err != nil {
		return 0, err
	}
	parity, err := inspectJournalSearchParity(ctx, queryer)
	if err != nil {
		return 0, fmt.Errorf("verify rebuilt journal search parity: %w", err)
	}
	if !parity.Ready {
		return 0, fmt.Errorf("rebuilt journal search parity is not ready: canonical_rows=%d, index_rows=%d, missing=%d, extra=%d, changed=%d", parity.CanonicalRows, parity.IndexRows, parity.Missing, parity.Extra, parity.Changed)
	}
	return rebuilt, nil
}

type journalSearchQueryExecer interface {
	queryContext
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func rebuildJournalSearch(ctx context.Context, queryer journalSearchQueryExecer) (int, error) {
	if queryer == nil {
		return 0, fmt.Errorf("rebuild journal search: queryer is nil")
	}
	correlationColumn, err := journalSearchCorrelationColumn(ctx, queryer)
	if err != nil {
		return 0, err
	}
	correlationExpression, err := journalSearchCanonicalCorrelationExpression(correlationColumn)
	if err != nil {
		return 0, err
	}
	var rebuilt int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries`).Scan(&rebuilt); err != nil {
		return 0, fmt.Errorf("count canonical journal rows: %w", err)
	}
	if _, err := queryer.ExecContext(ctx, `DELETE FROM journal_search`); err != nil {
		return 0, fmt.Errorf("delete journal search rows: %w", err)
	}
	query := fmt.Sprintf(`
INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
SELECT canonical.rowid, canonical.project_id, canonical.id, %s, canonical.entry_type, COALESCE(canonical.scope, ''), canonical.message
FROM journal_entries AS canonical
ORDER BY canonical.rowid`, correlationColumn, correlationExpression)
	if _, err := queryer.ExecContext(ctx, query); err != nil {
		return 0, fmt.Errorf("rebuild journal search rows: %w", err)
	}
	return rebuilt, nil
}

func journalSearchCanonicalCorrelationExpression(correlationColumn string) (string, error) {
	switch correlationColumn {
	case "harness_session_id":
		return "COALESCE(canonical.harness_session_id, '')", nil
	case "session_id":
		return "COALESCE(NULLIF(canonical.harness_session_id, ''), canonical.session_id, '')", nil
	default:
		return "", fmt.Errorf("unsupported journal search correlation column %q", correlationColumn)
	}
}
