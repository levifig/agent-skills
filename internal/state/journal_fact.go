package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// JournalFactPayload is the journal-kind fact body. Display timestamps live here.
type JournalFactPayload struct {
	EntryType        string `json:"entry_type"`
	Scope            string `json:"scope,omitempty"`
	Message          string `json:"message"`
	ObservedBranch   string `json:"observed_branch,omitempty"`
	ObservedWorktree string `json:"observed_worktree,omitempty"`
	HarnessSessionID string `json:"harness_session_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// JournalFactParity describes parity between journal facts and the projection.
type JournalFactParity struct {
	FactRows       int  `json:"fact_rows"`
	ProjectionRows int  `json:"projection_rows"`
	Missing        int  `json:"missing"`
	Extra          int  `json:"extra"`
	Changed        int  `json:"changed"`
	Ready          bool `json:"ready"`
}

const JournalFactParityDivergenceCode = "journal-fact-parity-diverged"

const JournalFactParityRepairCommand = "loaf state repair journal-facts --dry-run --json"

func encodeJournalFactPayload(payload JournalFactPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode journal fact payload: %w", err)
	}
	return string(raw), nil
}

func decodeJournalFactPayload(raw string) (JournalFactPayload, error) {
	var payload JournalFactPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return JournalFactPayload{}, fmt.Errorf("decode journal fact payload: %w", err)
	}
	return payload, nil
}

func appendJournalFactTx(ctx context.Context, tx *sql.Tx, projectID string, id string, payload JournalFactPayload, now time.Time) (FactEnvelope, error) {
	encoded, err := encodeJournalFactPayload(payload)
	if err != nil {
		return FactEnvelope{}, err
	}
	return appendFactTx(ctx, tx, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindJournal,
		Payload:   encoded,
		ID:        id,
		Now:       now,
	})
}

func insertJournalProjectionTx(ctx context.Context, execer journalWriteExecer, projectID string, id string, payload JournalFactPayload) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message,
  observed_branch, observed_worktree, harness_session_id,
  spec_id, task_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)
`, id, projectID, payload.EntryType, emptyToNil(payload.Scope), payload.Message,
		emptyToNil(payload.ObservedBranch), emptyToNil(payload.ObservedWorktree), emptyToNil(payload.HarnessSessionID),
		payload.CreatedAt, payload.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert journal projection row %q: %w", id, err)
	}
	return nil
}

func journalFactExistsTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE id = ?`, id).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect journal fact id %q: %w", id, err)
	}
	return count > 0, nil
}

func appendJournalDecisionFactAndProjectionTx(ctx context.Context, tx *sql.Tx, projectID, decisionID, scope, decisionMessage string, nowTime time.Time) error {
	now := nowTime.Format(time.RFC3339Nano)
	payload := JournalFactPayload{
		EntryType: "decision",
		Scope:     scope,
		Message:   decisionMessage,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := appendJournalFactTx(ctx, tx, projectID, decisionID, payload, nowTime); err != nil {
		return err
	}
	return insertJournalProjectionTx(ctx, tx, projectID, decisionID, payload)
}

func upsertJournalProjectionTx(ctx context.Context, execer journalWriteExecer, projectID, id string, payload JournalFactPayload) error {
	_, err := execer.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message,
  observed_branch, observed_worktree, harness_session_id,
  spec_id, task_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  entry_type = excluded.entry_type,
  scope = excluded.scope,
  message = excluded.message,
  observed_branch = excluded.observed_branch,
  observed_worktree = excluded.observed_worktree,
  harness_session_id = excluded.harness_session_id,
  updated_at = excluded.updated_at
`, id, projectID, payload.EntryType, emptyToNil(payload.Scope), payload.Message,
		emptyToNil(payload.ObservedBranch), emptyToNil(payload.ObservedWorktree), emptyToNil(payload.HarnessSessionID),
		payload.CreatedAt, payload.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert journal projection row %q: %w", id, err)
	}
	return nil
}

func journalFactPayloadContentEqual(a, b JournalFactPayload) bool {
	return a.EntryType == b.EntryType &&
		a.Scope == b.Scope &&
		a.Message == b.Message &&
		a.ObservedBranch == b.ObservedBranch &&
		a.ObservedWorktree == b.ObservedWorktree &&
		a.HarnessSessionID == b.HarnessSessionID &&
		a.CreatedAt == b.CreatedAt
}

func replaceJournalFactForImportTx(ctx context.Context, tx *sql.Tx, projectID, id string, payload JournalFactPayload, nowTime time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM facts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("replace journal fact %q: %w", id, err)
	}
	if _, err := appendJournalFactTx(ctx, tx, projectID, id, payload, nowTime); err != nil {
		return err
	}
	return upsertJournalProjectionTx(ctx, tx, projectID, id, payload)
}

func journalProjectionDiffersFromPayloadTx(ctx context.Context, tx *sql.Tx, id string, payload JournalFactPayload) (bool, error) {
	var entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt string
	err := tx.QueryRowContext(ctx, `
SELECT entry_type, COALESCE(scope, ''), message,
       COALESCE(observed_branch, ''), COALESCE(observed_worktree, ''),
       COALESCE(harness_session_id, ''), created_at, updated_at
FROM journal_entries
WHERE id = ?
`, id).Scan(&entryType, &scope, &message, &branch, &worktree, &sessionID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read journal projection %q: %w", id, err)
	}
	return !journalProjectionMatchesPayload(entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt, payload), nil
}

func upsertJournalViaFactsTx(ctx context.Context, tx *sql.Tx, projectID, id string, payload JournalFactPayload, nowTime time.Time) error {
	exists, err := journalFactExistsTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if !exists {
		if _, err := appendJournalFactTx(ctx, tx, projectID, id, payload, nowTime); err != nil {
			return err
		}
		return upsertJournalProjectionTx(ctx, tx, projectID, id, payload)
	}
	var projectionExists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = ?`, id).Scan(&projectionExists); err != nil {
		return fmt.Errorf("inspect journal projection id %q: %w", id, err)
	}
	if projectionExists == 0 {
		return insertJournalProjectionTx(ctx, tx, projectID, id, payload)
	}
	var payloadRaw string
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM facts WHERE id = ?`, id).Scan(&payloadRaw); err != nil {
		return fmt.Errorf("read journal fact payload %q: %w", id, err)
	}
	existing, err := decodeJournalFactPayload(payloadRaw)
	if err != nil {
		return err
	}
	// Grow-only facts own display timestamps; import callers pass fresh clocks.
	payload.CreatedAt = existing.CreatedAt
	payload.UpdatedAt = existing.UpdatedAt
	if journalFactPayloadContentEqual(existing, payload) {
		differs, err := journalProjectionDiffersFromPayloadTx(ctx, tx, id, payload)
		if err != nil {
			return err
		}
		if !differs {
			return nil
		}
		return upsertJournalProjectionTx(ctx, tx, projectID, id, payload)
	}
	payload.UpdatedAt = nowTime.UTC().Format(time.RFC3339Nano)
	return replaceJournalFactForImportTx(ctx, tx, projectID, id, payload, nowTime)
}

// InspectJournalFactParity compares journal-kind facts to journal_entries.
func InspectJournalFactParity(ctx context.Context, store *Store) (JournalFactParity, error) {
	if store == nil || store.db == nil {
		return JournalFactParity{}, fmt.Errorf("inspect journal fact parity: store is nil")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return JournalFactParity{}, fmt.Errorf("begin journal fact parity snapshot: %w", err)
	}
	defer tx.Rollback()
	return inspectJournalFactParity(ctx, tx)
}

func inspectJournalFactParity(ctx context.Context, queryer queryContext) (JournalFactParity, error) {
	if queryer == nil {
		return JournalFactParity{}, fmt.Errorf("inspect journal fact parity: queryer is nil")
	}
	if !tableExists(ctx, queryer, "facts") {
		return JournalFactParity{Ready: true}, nil
	}
	rows, err := queryer.QueryContext(ctx, `
SELECT f.id, f.project_id, f.payload
FROM facts AS f
WHERE f.kind = ?
ORDER BY f.project_id, f.hlc, f.env_id, f.id
`, FactKindJournal)
	if err != nil {
		return JournalFactParity{}, fmt.Errorf("list journal facts: %w", err)
	}
	defer rows.Close()

	type foldedRow struct {
		projectID string
		payload   JournalFactPayload
	}
	folded := map[string]foldedRow{}
	for rows.Next() {
		var id, projectID, payloadRaw string
		if err := rows.Scan(&id, &projectID, &payloadRaw); err != nil {
			return JournalFactParity{}, fmt.Errorf("scan journal fact: %w", err)
		}
		payload, err := decodeJournalFactPayload(payloadRaw)
		if err != nil {
			return JournalFactParity{}, err
		}
		folded[id] = foldedRow{projectID: projectID, payload: payload}
	}
	if err := rows.Err(); err != nil {
		return JournalFactParity{}, fmt.Errorf("iterate journal facts: %w", err)
	}

	projectionRows, err := queryer.QueryContext(ctx, `
SELECT id, project_id, entry_type, COALESCE(scope, ''), message,
       COALESCE(observed_branch, ''), COALESCE(observed_worktree, ''),
       COALESCE(harness_session_id, ''), created_at, updated_at
FROM journal_entries
ORDER BY project_id, created_at, rowid
`)
	if err != nil {
		return JournalFactParity{}, fmt.Errorf("list journal projection rows: %w", err)
	}
	defer projectionRows.Close()

	parity := JournalFactParity{FactRows: len(folded)}
	seen := map[string]struct{}{}
	for projectionRows.Next() {
		var id, projectID, entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt string
		if err := projectionRows.Scan(&id, &projectID, &entryType, &scope, &message, &branch, &worktree, &sessionID, &createdAt, &updatedAt); err != nil {
			return JournalFactParity{}, fmt.Errorf("scan journal projection row: %w", err)
		}
		parity.ProjectionRows++
		seen[id] = struct{}{}
		fact, ok := folded[id]
		if !ok {
			parity.Extra++
			continue
		}
		if fact.projectID != projectID || !journalProjectionMatchesPayload(entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt, fact.payload) {
			parity.Changed++
		}
	}
	if err := projectionRows.Err(); err != nil {
		return JournalFactParity{}, fmt.Errorf("iterate journal projection rows: %w", err)
	}
	for id := range folded {
		if _, ok := seen[id]; !ok {
			parity.Missing++
		}
	}
	parity.Ready = parity.Missing == 0 && parity.Extra == 0 && parity.Changed == 0
	return parity, nil
}

func journalProjectionMatchesPayload(entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt string, payload JournalFactPayload) bool {
	return payload.EntryType == entryType &&
		payload.Scope == scope &&
		payload.Message == message &&
		payload.ObservedBranch == branch &&
		payload.ObservedWorktree == worktree &&
		payload.HarnessSessionID == sessionID &&
		payload.CreatedAt == createdAt &&
		payload.UpdatedAt == updatedAt
}

func rebuildJournalProjectionFromFactsTx(ctx context.Context, execer journalWriteExecer, projectID string) (int, error) {
	rows, err := execer.QueryContext(ctx, `
SELECT id, payload
FROM facts
WHERE project_id = ? AND kind = ?
ORDER BY hlc ASC, env_id ASC, id ASC
`, projectID, FactKindJournal)
	if err != nil {
		return 0, fmt.Errorf("list journal facts for rebuild: %w", err)
	}
	defer rows.Close()

	type projection struct {
		id      string
		payload JournalFactPayload
	}
	projections := []projection{}
	for rows.Next() {
		var id, payloadRaw string
		if err := rows.Scan(&id, &payloadRaw); err != nil {
			return 0, fmt.Errorf("scan journal fact for rebuild: %w", err)
		}
		payload, err := decodeJournalFactPayload(payloadRaw)
		if err != nil {
			return 0, err
		}
		projections = append(projections, projection{id: id, payload: payload})
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate journal facts for rebuild: %w", err)
	}

	if _, err := execer.ExecContext(ctx, `DELETE FROM journal_search WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear journal search projection: %w", err)
	}
	if _, err := execer.ExecContext(ctx, `DELETE FROM journal_entries WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear journal projection: %w", err)
	}
	for _, row := range projections {
		if err := insertJournalProjectionTx(ctx, execer, projectID, row.id, row.payload); err != nil {
			return 0, err
		}
		if err := insertJournalSearchTx(ctx, execer, projectID, row.id, row.payload.HarnessSessionID, row.payload.EntryType, row.payload.Scope, row.payload.Message); err != nil {
			return 0, err
		}
	}
	return len(projections), nil
}

func backfillMissingJournalFactsForProjectTx(ctx context.Context, tx *sql.Tx, projectID string) error {
	if tx == nil {
		return fmt.Errorf("backfill missing journal facts: transaction is nil")
	}
	if !tableExists(ctx, tx, "facts") {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
SELECT j.id, j.entry_type, COALESCE(j.scope, ''), j.message,
       COALESCE(j.observed_branch, ''), COALESCE(j.observed_worktree, ''),
       COALESCE(j.harness_session_id, ''), j.created_at, j.updated_at
FROM journal_entries AS j
LEFT JOIN facts AS f ON f.id = j.id
WHERE j.project_id = ? AND f.id IS NULL
ORDER BY j.created_at, j.rowid
`, projectID)
	if err != nil {
		return fmt.Errorf("list journal rows missing facts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt string
		if err := rows.Scan(&id, &entryType, &scope, &message, &branch, &worktree, &sessionID, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan journal row missing fact: %w", err)
		}
		created, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return fmt.Errorf("parse journal created_at: %w", err)
		}
		payload := JournalFactPayload{
			EntryType: entryType, Scope: scope, Message: message,
			ObservedBranch: branch, ObservedWorktree: worktree, HarnessSessionID: sessionID,
			CreatedAt: createdAt, UpdatedAt: updatedAt,
		}
		if _, err := appendJournalFactTx(ctx, tx, projectID, id, payload, created); err != nil {
			return err
		}
	}
	return rows.Err()
}

func tableExists(ctx context.Context, queryer queryContext, name string) bool {
	var count int
	err := queryer.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM sqlite_master
WHERE type = 'table' AND name = ?
`, name).Scan(&count)
	return err == nil && count > 0
}

// backfillJournalFactForTest mirrors one projection row into facts for tests that
// insert journal_entries directly.
func backfillJournalFactForTest(ctx context.Context, store *Store, projectID, journalEntryID string) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("backfill journal fact: store is nil")
	}
	var entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt sql.NullString
	err := store.db.QueryRowContext(ctx, `
SELECT entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, created_at, updated_at
FROM journal_entries
WHERE project_id = ? AND id = ?
`, projectID, journalEntryID).Scan(&entryType, &scope, &message, &branch, &worktree, &sessionID, &createdAt, &updatedAt)
	if err != nil {
		return fmt.Errorf("read journal projection row %q: %w", journalEntryID, err)
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt.String)
	if err != nil {
		return fmt.Errorf("parse journal created_at: %w", err)
	}
	payload := JournalFactPayload{
		EntryType:        entryType.String,
		Scope:            scope.String,
		Message:          message.String,
		ObservedBranch:   branch.String,
		ObservedWorktree: worktree.String,
		HarnessSessionID: sessionID.String,
		CreatedAt:        createdAt.String,
		UpdatedAt:        updatedAt.String,
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin journal fact backfill: %w", err)
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE id = ?`, journalEntryID).Scan(&existing); err != nil {
		return fmt.Errorf("inspect fact id: %w", err)
	}
	if existing == 0 {
		if _, err := appendJournalFactTx(ctx, tx, projectID, journalEntryID, payload, created); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				return nil
			}
			return err
		}
	}
	return tx.Commit()
}
