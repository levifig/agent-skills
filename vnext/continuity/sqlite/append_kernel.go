package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	payloadVersionV1  = 1
	envelopeVersionV1 = 1
)

type appendIntentV1 struct {
	projectID continuity.ProjectID
	factID    continuity.FactID
	subject   continuity.SubjectRef
	kind      continuity.FactKind
	content   canonicalContentV1
}

type storedFactV1 struct {
	factID              continuity.FactID
	projectID           continuity.ProjectID
	subject             continuity.SubjectRef
	kind                continuity.FactKind
	payloadVersion      int
	content             canonicalContentV1
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	clock               continuity.HybridTime
	envelopeVersion     int
}

func (store *Store) appendFactV1(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, subject continuity.SubjectRef, kind continuity.FactKind, content canonicalContentV1) (continuity.AppendReceipt, error) {
	intent := appendIntentV1{projectID: projectID, factID: factID, subject: subject, kind: kind, content: content}
	if err := validateAppendIntentV1(intent); err != nil {
		return continuity.AppendReceipt{}, err
	}
	canonical, err := canonicalizeStoredContentV1(kind, payloadVersionV1, string(content))
	if err != nil || canonical != content {
		return continuity.AppendReceipt{}, &continuity.Problem{Code: continuity.ProblemInvalid, Field: "content", Detail: "content is not sealed canonical v1 data"}
	}
	if store == nil {
		return continuity.AppendReceipt{}, storeClosedProblemV1()
	}
	if ctx == nil {
		return continuity.AppendReceipt{}, &continuity.Problem{Code: continuity.ProblemInvalid, Field: "context", Detail: "must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		return continuity.AppendReceipt{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil || store.wallMillis == nil {
		return continuity.AppendReceipt{}, storeClosedProblemV1()
	}

	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return continuity.AppendReceipt{}, ctxErr
		}
		if errors.Is(err, sql.ErrConnDone) {
			return continuity.AppendReceipt{}, storeClosedProblemV1()
		}
		return continuity.AppendReceipt{}, storeUnavailableProblemV1()
	}
	defer tx.Rollback()

	receipt, err := store.appendIntentInTxV1(ctx, tx, intent)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return continuity.AppendReceipt{}, &continuity.Problem{Code: continuity.ProblemCommitUnknown, Detail: "the append commit outcome is unknown; retry with the retained fact id"}
	}
	return receipt, nil
}

func (store *Store) appendIntentInTxV1(ctx context.Context, tx *sql.Tx, intent appendIntentV1) (continuity.AppendReceipt, error) {
	if err := rejectPrunedFactIDV1(ctx, tx, intent.factID); err != nil {
		return continuity.AppendReceipt{}, err
	}
	existing, found, err := readFactByIDV1(ctx, tx, intent.factID)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	if found {
		if !existing.matchesIntent(intent) {
			return continuity.AppendReceipt{}, &continuity.Problem{Code: continuity.ProblemFactConflict, Field: "fact_id", Detail: "is already bound to a different immutable fact"}
		}
		receipt := existing.receipt()
		receipt.Replayed = true
		return receipt, nil
	}

	if err := admitFactV1(ctx, tx, intent); err != nil {
		return continuity.AppendReceipt{}, err
	}
	sequence, clock, err := allocateEnvelopeV1(ctx, tx, intent.projectID, store.environmentID, store.wallMillis())
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	if err := insertFactV1(ctx, tx, intent, store.environmentID, sequence, clock); err != nil {
		return continuity.AppendReceipt{}, err
	}
	if err := advanceEnvironmentHeadV1(ctx, tx, storedFactV1{
		factID:              intent.factID,
		projectID:           intent.projectID,
		subject:             intent.subject,
		kind:                intent.kind,
		payloadVersion:      payloadVersionV1,
		content:             intent.content,
		environmentID:       store.environmentID,
		environmentSequence: sequence,
		clock:               clock,
		envelopeVersion:     envelopeVersionV1,
	}); err != nil {
		return continuity.AppendReceipt{}, err
	}
	return continuity.AppendReceipt{
		FactID:              intent.factID,
		ProjectID:           intent.projectID,
		Subject:             intent.subject,
		Kind:                intent.kind,
		EnvironmentID:       store.environmentID,
		EnvironmentSequence: sequence,
		Clock:               clock,
	}, nil
}

func rejectPrunedFactIDV1(ctx context.Context, tx *sql.Tx, factID continuity.FactID) error {
	var tombstoneProjectID continuity.ProjectID
	err := tx.QueryRowContext(ctx, `
SELECT project_id
FROM continuity_sync_tombstones
WHERE fact_id = ?`, string(factID)).Scan(&tombstoneProjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return transactionOperationProblemV1(ctx)
	}
	return &continuity.Problem{Code: continuity.ProblemFactConflict, Field: "fact_id", Detail: "is retained as a prune tombstone"}
}

func validateAppendIntentV1(intent appendIntentV1) error {
	if err := intent.projectID.Validate(); err != nil {
		return refieldProblemV1(err, "project_id")
	}
	if err := intent.factID.Validate(); err != nil {
		return refieldProblemV1(err, "fact_id")
	}
	if err := intent.subject.ID.Validate(); err != nil {
		return refieldProblemV1(err, "subject_id")
	}
	definition, ok := continuity.DefinitionFor(intent.kind)
	if !ok || definition.Record != intent.subject.Kind {
		return &continuity.Problem{Code: continuity.ProblemInvalid, Field: "kind", Detail: "does not match the closed subject family"}
	}
	if intent.subject.Kind == continuity.RecordProjectIdentity && intent.subject.ID != continuity.SubjectID(intent.projectID) {
		return &continuity.Problem{Code: continuity.ProblemInvalid, Field: "subject_id", Detail: "project identity must use the project id"}
	}
	return nil
}

func readFactByIDV1(ctx context.Context, tx *sql.Tx, factID continuity.FactID) (storedFactV1, bool, error) {
	row := tx.QueryRowContext(ctx, `
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
WHERE fact_id = ?`, string(factID))
	fact, err := scanStoredFactRowV1(ctx, row)
	if errors.Is(err, sql.ErrNoRows) {
		return storedFactV1{}, false, nil
	}
	if err != nil {
		return storedFactV1{}, false, err
	}
	return fact, true, nil
}

type storedFactColumnsV1 struct {
	fact        storedFactV1
	subjectKind string
	subjectID   string
	factKind    string
	content     string
	logical     int64
}

func scanStoredFactRowV1(ctx context.Context, row *sql.Row) (storedFactV1, error) {
	var columns storedFactColumnsV1
	err := row.Scan(
		&columns.fact.factID,
		&columns.fact.projectID,
		&columns.subjectKind,
		&columns.subjectID,
		&columns.factKind,
		&columns.fact.payloadVersion,
		&columns.content,
		&columns.fact.environmentID,
		&columns.fact.environmentSequence,
		&columns.fact.clock.WallMillis,
		&columns.logical,
		&columns.fact.envelopeVersion,
	)
	return finishStoredFactScanV1(ctx, columns, err)
}

func scanStoredFactRowsV1(ctx context.Context, rows *sql.Rows) (storedFactV1, error) {
	var columns storedFactColumnsV1
	err := rows.Scan(
		&columns.fact.factID,
		&columns.fact.projectID,
		&columns.subjectKind,
		&columns.subjectID,
		&columns.factKind,
		&columns.fact.payloadVersion,
		&columns.content,
		&columns.fact.environmentID,
		&columns.fact.environmentSequence,
		&columns.fact.clock.WallMillis,
		&columns.logical,
		&columns.fact.envelopeVersion,
	)
	return finishStoredFactScanV1(ctx, columns, err)
}

func finishStoredFactScanV1(ctx context.Context, columns storedFactColumnsV1, err error) (storedFactV1, error) {
	var fact storedFactV1
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storedFactV1{}, err
		}
		return storedFactV1{}, transactionOperationProblemV1(ctx)
	}
	fact = columns.fact
	fact.subject = continuity.SubjectRef{Kind: continuity.RecordKind(columns.subjectKind), ID: continuity.SubjectID(columns.subjectID)}
	fact.kind = continuity.FactKind(columns.factKind)
	fact.content = canonicalContentV1(columns.content)
	if columns.logical < 0 || columns.logical > math.MaxInt32 {
		return storedFactV1{}, corruptFactProblemV1()
	}
	fact.clock.Logical = int32(columns.logical)
	if err := validateStoredFactV1(fact); err != nil {
		return storedFactV1{}, err
	}
	return fact, nil
}

func validateStoredFactV1(fact storedFactV1) error {
	if err := fact.factID.Validate(); err != nil {
		return corruptFactProblemV1()
	}
	if err := fact.projectID.Validate(); err != nil {
		return corruptFactProblemV1()
	}
	if err := fact.subject.ID.Validate(); err != nil {
		return corruptFactProblemV1()
	}
	if err := fact.environmentID.Validate(); err != nil {
		return corruptFactProblemV1()
	}
	if fact.payloadVersion != payloadVersionV1 || fact.envelopeVersion != envelopeVersionV1 || fact.environmentSequence < 1 || fact.clock.WallMillis < 0 {
		return corruptFactProblemV1()
	}
	if legacyScratchpadFactV1(fact.subject, fact.kind) {
		return nil
	}
	definition, ok := continuity.DefinitionFor(fact.kind)
	if !ok || definition.Record != fact.subject.Kind {
		return corruptFactProblemV1()
	}
	if fact.subject.Kind == continuity.RecordProjectIdentity && fact.subject.ID != continuity.SubjectID(fact.projectID) {
		return corruptFactProblemV1()
	}
	canonical, err := canonicalizeStoredContentV1(fact.kind, fact.payloadVersion, string(fact.content))
	if err != nil || canonical != fact.content {
		return corruptFactProblemV1()
	}
	return nil
}

func (fact storedFactV1) matchesIntent(intent appendIntentV1) bool {
	return fact.projectID == intent.projectID &&
		fact.subject == intent.subject &&
		fact.kind == intent.kind &&
		fact.payloadVersion == payloadVersionV1 &&
		fact.content == intent.content &&
		fact.envelopeVersion == envelopeVersionV1
}

func (fact storedFactV1) receipt() continuity.AppendReceipt {
	return continuity.AppendReceipt{
		FactID:              fact.factID,
		ProjectID:           fact.projectID,
		Subject:             fact.subject,
		Kind:                fact.kind,
		EnvironmentID:       fact.environmentID,
		EnvironmentSequence: fact.environmentSequence,
		Clock:               fact.clock,
	}
}

func allocateEnvelopeV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, environmentID continuity.EnvironmentID, wallMillis int64) (int64, continuity.HybridTime, error) {
	var maximumSequence int64
	if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(environment_sequence), 0)
FROM continuity_facts
	WHERE project_id = ? AND environment_id = ?`, string(projectID), string(environmentID)).Scan(&maximumSequence); err != nil {
		return 0, continuity.HybridTime{}, transactionOperationProblemV1(ctx)
	}
	if maximumSequence == math.MaxInt64 {
		return 0, continuity.HybridTime{}, &continuity.Problem{Code: continuity.ProblemClockExhausted, Field: "environment_sequence", Detail: "the local sequence is exhausted"}
	}
	sequence := maximumSequence + 1

	var maximumWall int64
	var maximumLogical int64
	err := tx.QueryRowContext(ctx, `
SELECT hlc_wall_millis, hlc_logical
FROM continuity_facts
WHERE project_id = ?
ORDER BY hlc_wall_millis DESC, hlc_logical DESC, environment_id DESC, fact_id DESC
	LIMIT 1`, string(projectID)).Scan(&maximumWall, &maximumLogical)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, continuity.HybridTime{}, transactionOperationProblemV1(ctx)
	}
	if wallMillis < 0 {
		wallMillis = 0
	}
	if errors.Is(err, sql.ErrNoRows) || wallMillis > maximumWall {
		return sequence, continuity.HybridTime{WallMillis: wallMillis}, nil
	}
	if maximumLogical >= math.MaxInt32 {
		return 0, continuity.HybridTime{}, &continuity.Problem{Code: continuity.ProblemClockExhausted, Field: "hlc_logical", Detail: "the project logical clock is exhausted at the current wall time"}
	}
	return sequence, continuity.HybridTime{WallMillis: maximumWall, Logical: int32(maximumLogical + 1)}, nil
}

func insertFactV1(ctx context.Context, tx *sql.Tx, intent appendIntentV1, environmentID continuity.EnvironmentID, sequence int64, clock continuity.HybridTime) error {
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_facts(
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
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(intent.factID),
		string(intent.projectID),
		string(intent.subject.Kind),
		string(intent.subject.ID),
		string(intent.kind),
		payloadVersionV1,
		string(intent.content),
		string(environmentID),
		sequence,
		clock.WallMillis,
		clock.Logical,
		envelopeVersionV1,
	)
	if err != nil {
		return transactionOperationProblemV1(ctx)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return transactionOperationProblemV1(ctx)
	}
	return nil
}

func refieldProblemV1(err error, field string) error {
	problem, ok := err.(*continuity.Problem)
	if !ok {
		return err
	}
	return &continuity.Problem{Code: problem.Code, Field: field, Detail: problem.Detail}
}

func storeClosedProblemV1() error {
	return &continuity.Problem{Code: continuity.ProblemStoreClosed, Detail: "the continuity store is closed"}
}

func storeUnavailableProblemV1() error {
	return &continuity.Problem{Code: continuity.ProblemStoreUnavailable, Detail: "the continuity store is unavailable"}
}

func transactionOperationProblemV1(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return storeUnavailableProblemV1()
}

func corruptFactProblemV1() error {
	return &continuity.Problem{Code: continuity.ProblemCorruptFact, Detail: "stored fact data does not match the v1 continuity contract"}
}
