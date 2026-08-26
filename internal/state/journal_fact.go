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
// EntryID, when set, is the stable journal_entries projection id. Empty means the
// fact id itself is the projection id (wrapped corpus and first write).
type JournalFactPayload struct {
	EntryType        string `json:"entry_type"`
	Scope            string `json:"scope,omitempty"`
	Message          string `json:"message"`
	ObservedBranch   string `json:"observed_branch,omitempty"`
	ObservedWorktree string `json:"observed_worktree,omitempty"`
	HarnessSessionID string `json:"harness_session_id,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	EntryID          string `json:"entry_id,omitempty"`
}

// JournalFactParity describes parity between folded journal facts and the projection.
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

type foldedJournalFact struct {
	projectID string
	payload   JournalFactPayload
}

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

func journalEntryIDFromFact(factID string, payload JournalFactPayload) string {
	if id := strings.TrimSpace(payload.EntryID); id != "" {
		return id
	}
	return factID
}

func appendJournalFactTx(ctx context.Context, tx *sql.Tx, projectID string, id string, payload JournalFactPayload, now time.Time) (FactEnvelope, error) {
	return appendJournalFactWithEnvTx(ctx, tx, projectID, id, payload, now, "")
}

func appendJournalFactWithEnvTx(ctx context.Context, tx *sql.Tx, projectID string, id string, payload JournalFactPayload, now time.Time, envID string) (FactEnvelope, error) {
	encoded, err := encodeJournalFactPayload(payload)
	if err != nil {
		return FactEnvelope{}, err
	}
	return appendFactTx(ctx, tx, AppendFactInput{
		ProjectID: projectID,
		Kind:      FactKindJournal,
		Payload:   encoded,
		EnvID:     envID,
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

func foldJournalFacts(ctx context.Context, queryer queryContext, projectID string) (map[string]foldedJournalFact, error) {
	query := `
SELECT f.id, f.project_id, f.payload
FROM facts AS f
WHERE f.kind = ?
`
	args := []any{FactKindJournal}
	if strings.TrimSpace(projectID) != "" {
		query += ` AND f.project_id = ?`
		args = append(args, projectID)
	}
	query += ` ORDER BY f.project_id, f.hlc, f.env_id, f.id`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list journal facts for fold: %w", err)
	}
	defer rows.Close()
	folded := map[string]foldedJournalFact{}
	for rows.Next() {
		var id, projID, payloadRaw string
		if err := rows.Scan(&id, &projID, &payloadRaw); err != nil {
			return nil, fmt.Errorf("scan journal fact for fold: %w", err)
		}
		payload, err := decodeJournalFactPayload(payloadRaw)
		if err != nil {
			return nil, err
		}
		entryID := journalEntryIDFromFact(id, payload)
		folded[entryID] = foldedJournalFact{projectID: projID, payload: payload}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate journal facts for fold: %w", err)
	}
	return folded, nil
}

func journalFactExistsForEntryTx(ctx context.Context, tx *sql.Tx, projectID, entryID string) (bool, error) {
	folded, err := foldJournalFacts(ctx, tx, projectID)
	if err != nil {
		return false, err
	}
	_, ok := folded[entryID]
	return ok, nil
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

func upsertJournalProjectionTx(ctx context.Context, execer journalWriteExecer, projectID string, id string, payload JournalFactPayload) error {
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

// appendJournalFactRevisionForImportTx appends a grow-only revision fact for an
// existing journal entry and updates the stable projection pointer. It never
// deletes facts rows.
func appendJournalFactRevisionForImportTx(ctx context.Context, tx *sql.Tx, projectID, entryID string, payload JournalFactPayload, nowTime time.Time) error {
	payload.EntryID = entryID
	newID, err := mintFactID(nowTime)
	if err != nil {
		return err
	}
	if _, err := appendJournalFactTx(ctx, tx, projectID, newID, payload, nowTime); err != nil {
		return err
	}
	return upsertJournalProjectionTx(ctx, tx, projectID, entryID, payload)
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
	folded, err := foldJournalFacts(ctx, tx, projectID)
	if err != nil {
		return err
	}
	existingFold, exists := folded[id]
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
	existing := existingFold.payload
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
	return appendJournalFactRevisionForImportTx(ctx, tx, projectID, id, payload, nowTime)
}

// InspectJournalFactParity compares folded journal facts to journal_entries.
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
	folded, err := foldJournalFacts(ctx, queryer, "")
	if err != nil {
		return JournalFactParity{}, err
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
	folded, err := foldJournalFacts(ctx, execer, projectID)
	if err != nil {
		return 0, err
	}

	if _, err := execer.ExecContext(ctx, `DELETE FROM journal_search WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear journal search projection: %w", err)
	}
	if _, err := execer.ExecContext(ctx, `DELETE FROM journal_entries WHERE project_id = ?`, projectID); err != nil {
		return 0, fmt.Errorf("clear journal projection: %w", err)
	}
	for entryID, row := range folded {
		if err := insertJournalProjectionTx(ctx, execer, projectID, entryID, row.payload); err != nil {
			return 0, err
		}
		if err := insertJournalSearchTx(ctx, execer, projectID, entryID, row.payload.HarnessSessionID, row.payload.EntryType, row.payload.Scope, row.payload.Message); err != nil {
			return 0, err
		}
	}
	return len(folded), nil
}

func backfillMissingJournalFactsForProjectTx(ctx context.Context, tx *sql.Tx, projectID string) error {
	if tx == nil {
		return fmt.Errorf("backfill missing journal facts: transaction is nil")
	}
	if !tableExists(ctx, tx, "facts") {
		return nil
	}
	folded, err := foldJournalFacts(ctx, tx, projectID)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
SELECT j.id, j.entry_type, COALESCE(j.scope, ''), j.message,
       COALESCE(j.observed_branch, ''), COALESCE(j.observed_worktree, ''),
       COALESCE(j.harness_session_id, ''), j.created_at, j.updated_at
FROM journal_entries AS j
WHERE j.project_id = ?
ORDER BY j.created_at, j.rowid
`, projectID)
	if err != nil {
		return fmt.Errorf("list journal rows for fact backfill: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, entryType, scope, message, branch, worktree, sessionID, createdAt, updatedAt string
		if err := rows.Scan(&id, &entryType, &scope, &message, &branch, &worktree, &sessionID, &createdAt, &updatedAt); err != nil {
			return fmt.Errorf("scan journal row missing fact: %w", err)
		}
		if _, ok := folded[id]; ok {
			continue
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
		if _, err := appendJournalFactWithEnvTx(ctx, tx, projectID, id, payload, created, legacyFactEnvID); err != nil {
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
// insert journal_entries directly. Historical wrap uses legacy-host env_id.
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
	exists, err := journalFactExistsForEntryTx(ctx, tx, projectID, journalEntryID)
	if err != nil {
		return err
	}
	if !exists {
		if _, err := appendJournalFactWithEnvTx(ctx, tx, projectID, journalEntryID, payload, created, legacyFactEnvID); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				return nil
			}
			return err
		}
	}
	return tx.Commit()
}
