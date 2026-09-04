package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/levifig/loaf/vnext/continuity"
)

const maxRehearsalFactsV1 = 100_000

// RehearsalFact is one validated immutable fact prepared for an isolated
// migration rehearsal. Values can be created only by the typed constructors
// below; the type is not a general raw-fact insertion surface.
type RehearsalFact struct {
	intent appendIntentV1
}

// RehearsalSnapshotVerifier independently checks the folded rehearsal result
// while the import transaction is still open. It must not call store methods.
type RehearsalSnapshotVerifier func(continuity.Snapshot) error

// NewProjectRehearsalFact prepares a project registration for a rehearsal.
func NewProjectRehearsalFact(
	projectID continuity.ProjectID,
	factID continuity.FactID,
	payload continuity.ProjectRegistrationPayload,
) (RehearsalFact, error) {
	content, err := encodeProjectRegistrationV1(payload)
	if err != nil {
		return RehearsalFact{}, err
	}
	return newRehearsalFactV1(appendIntentV1{
		projectID: projectID,
		factID:    factID,
		subject:   continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)},
		kind:      continuity.FactProjectRegistered,
		content:   content,
	})
}

// NewJournalRehearsalFact prepares a journal entry for a rehearsal.
func NewJournalRehearsalFact(
	projectID continuity.ProjectID,
	factID continuity.FactID,
	journalID continuity.SubjectID,
	payload continuity.JournalRecordedPayload,
) (RehearsalFact, error) {
	content, err := encodeJournalRecordedV1(payload)
	if err != nil {
		return RehearsalFact{}, err
	}
	return newRehearsalFactV1(appendIntentV1{
		projectID: projectID,
		factID:    factID,
		subject:   continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: journalID},
		kind:      continuity.FactJournalRecorded,
		content:   content,
	})
}

// NewWrapRehearsalFact prepares a wrap for a rehearsal.
func NewWrapRehearsalFact(
	projectID continuity.ProjectID,
	factID continuity.FactID,
	wrapID continuity.SubjectID,
	payload continuity.WrapRecordedPayload,
) (RehearsalFact, error) {
	content, err := encodeWrapRecordedV1(payload)
	if err != nil {
		return RehearsalFact{}, err
	}
	return newRehearsalFactV1(appendIntentV1{
		projectID: projectID,
		factID:    factID,
		subject:   continuity.SubjectRef{Kind: continuity.RecordWrap, ID: wrapID},
		kind:      continuity.FactWrapRecorded,
		content:   content,
	})
}

// NewHandoffRehearsalFact prepares an unfocused legacy handoff for a rehearsal.
func NewHandoffRehearsalFact(
	projectID continuity.ProjectID,
	factID continuity.FactID,
	handoffID continuity.SubjectID,
	payload continuity.HandoffRecordedPayload,
) (RehearsalFact, error) {
	if payload.Focus != nil {
		return RehearsalFact{}, &continuity.Problem{
			Code: continuity.ProblemInvalid, Field: "focus", Detail: "legacy rehearsal handoffs must be unfocused",
		}
	}
	content, err := encodeHandoffRecordedV1(payload)
	if err != nil {
		return RehearsalFact{}, err
	}
	return newRehearsalFactV1(appendIntentV1{
		projectID: projectID,
		factID:    factID,
		subject:   continuity.SubjectRef{Kind: continuity.RecordHandoff, ID: handoffID},
		kind:      continuity.FactHandoffRecorded,
		content:   content,
	})
}

func newRehearsalFactV1(intent appendIntentV1) (RehearsalFact, error) {
	if err := validateAppendIntentV1(intent); err != nil {
		return RehearsalFact{}, err
	}
	canonical, err := canonicalizeStoredContentV1(intent.kind, payloadVersionV1, string(intent.content))
	if err != nil || canonical != intent.content {
		return RehearsalFact{}, &continuity.Problem{
			Code: continuity.ProblemInvalid, Field: "content", Detail: "content is not sealed canonical v1 data",
		}
	}
	return RehearsalFact{intent: intent}, nil
}

// ImportRehearsalFacts atomically resumes an exact archive prefix, appends the
// missing suffix, and returns the snapshot folded at the commit boundary. Any
// unrelated fact or sync state rejects the rehearsal without changing facts.
func (store *Store) ImportRehearsalFacts(
	ctx context.Context,
	projectID continuity.ProjectID,
	facts []RehearsalFact,
	verify RehearsalSnapshotVerifier,
) (continuity.Snapshot, error) {
	if err := projectID.Validate(); err != nil {
		return continuity.Snapshot{}, refieldProblemV1(err, "project_id")
	}
	if len(facts) == 0 {
		return continuity.Snapshot{}, &continuity.Problem{
			Code: continuity.ProblemInvalid, Field: "facts", Detail: "must contain a project-first archive sequence",
		}
	}
	if len(facts) > maxRehearsalFactsV1 {
		return continuity.Snapshot{}, &continuity.Problem{
			Code: continuity.ProblemInvalid, Field: "facts", Detail: fmt.Sprintf("must contain at most %d records", maxRehearsalFactsV1),
		}
	}
	if verify == nil {
		return continuity.Snapshot{}, &continuity.Problem{
			Code: continuity.ProblemInvalid, Field: "verify", Detail: "must independently verify the folded rehearsal snapshot",
		}
	}
	for index := range facts {
		if facts[index].intent.projectID != projectID {
			return continuity.Snapshot{}, &continuity.Problem{
				Code: continuity.ProblemInvalid, Field: "facts", Detail: fmt.Sprintf("record %d belongs to another project", index),
			}
		}
		if err := validateAppendIntentV1(facts[index].intent); err != nil {
			return continuity.Snapshot{}, err
		}
	}
	if store == nil {
		return continuity.Snapshot{}, storeClosedProblemV1()
	}
	if ctx == nil {
		return continuity.Snapshot{}, &continuity.Problem{Code: continuity.ProblemInvalid, Field: "context", Detail: "must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		return continuity.Snapshot{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.db == nil || store.wallMillis == nil {
		return continuity.Snapshot{}, storeClosedProblemV1()
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return continuity.Snapshot{}, ctxErr
		}
		if errors.Is(err, sql.ErrConnDone) {
			return continuity.Snapshot{}, storeClosedProblemV1()
		}
		return continuity.Snapshot{}, storeUnavailableProblemV1()
	}
	defer tx.Rollback()

	if err := requireRehearsalSyncAbsenceV1(ctx, tx, projectID); err != nil {
		return continuity.Snapshot{}, err
	}
	existing, err := loadRehearsalFactsV1(ctx, tx, projectID, len(facts)+1)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := requireExactRehearsalPrefixV1(existing, facts); err != nil {
		return continuity.Snapshot{}, err
	}
	for index := len(existing); index < len(facts); index++ {
		if _, err := store.appendIntentInTxV1(ctx, tx, facts[index].intent); err != nil {
			return continuity.Snapshot{}, fmt.Errorf("append rehearsal fact %d: %w", index, err)
		}
	}
	actual, err := loadRehearsalFactsV1(ctx, tx, projectID, len(facts)+1)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := requireExactRehearsalInventoryV1(actual, facts); err != nil {
		return continuity.Snapshot{}, err
	}
	snapshot, err := foldProjectSnapshotV1(ctx, projectID, 0, actual)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := requireRehearsalSyncAbsenceV1(ctx, tx, projectID); err != nil {
		return continuity.Snapshot{}, err
	}
	if err := verify(snapshot); err != nil {
		return continuity.Snapshot{}, fmt.Errorf("verify rehearsal snapshot: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return continuity.Snapshot{}, &continuity.Problem{
			Code:   continuity.ProblemCommitUnknown,
			Detail: "the rehearsal import commit outcome is unknown; retry the exact archive",
		}
	}
	return snapshot, nil
}

func loadRehearsalFactsV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	limit int,
) ([]storedFactV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
FROM continuity_facts
WHERE project_id = ?
ORDER BY
  hlc_wall_millis ASC,
  hlc_logical ASC,
  environment_id COLLATE BINARY ASC,
  fact_id COLLATE BINARY ASC
LIMIT ?`, string(projectID), limit)
	if err != nil {
		return nil, transactionOperationProblemV1(ctx)
	}
	facts := make([]storedFactV1, 0, min(limit, 64))
	for rows.Next() {
		fact, err := scanStoredFactRowsV1(ctx, rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, transactionOperationProblemV1(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, transactionOperationProblemV1(ctx)
	}
	return facts, nil
}

func requireExactRehearsalPrefixV1(existing []storedFactV1, expected []RehearsalFact) error {
	if len(existing) > len(expected) {
		return rehearsalContaminationProblemV1()
	}
	for index := range existing {
		if existing[index].factID != expected[index].intent.factID || !existing[index].matchesIntent(expected[index].intent) {
			return rehearsalContaminationProblemV1()
		}
	}
	return nil
}

func requireExactRehearsalInventoryV1(actual []storedFactV1, expected []RehearsalFact) error {
	if len(actual) != len(expected) {
		return rehearsalContaminationProblemV1()
	}
	return requireExactRehearsalPrefixV1(actual, expected)
}

func rehearsalContaminationProblemV1() error {
	return &continuity.Problem{
		Code:   continuity.ProblemFactConflict,
		Field:  "facts",
		Detail: "destination facts are not an exact prefix of the rehearsal archive",
	}
}

func requireRehearsalSyncAbsenceV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	var present int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_projects WHERE project_id = ?
)`, string(projectID)).Scan(&present); err != nil {
		return transactionOperationProblemV1(ctx)
	}
	if present != 0 {
		return &continuity.Problem{
			Code:   continuity.ProblemFactConflict,
			Field:  "sync_state",
			Detail: "destination project already has sync state",
		}
	}
	return nil
}
