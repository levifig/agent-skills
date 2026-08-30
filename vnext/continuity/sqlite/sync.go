package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

const (
	maximumSyncPageFrames      = 256
	maximumSealedEnvelopeBytes = 1_102_000
	maximumPrunedArrivalBytes  = 1_024
)

// SyncChannelID is the protocol's fixed-width opaque channel identity. It is
// intentionally defined at the persistence boundary so continuity storage does
// not depend on relay or cryptographic packages.
type SyncChannelID [32]byte

// SyncErrorCode is a machine-stable private-sync persistence failure class.
type SyncErrorCode string

const (
	SyncErrorInvalid                 SyncErrorCode = "invalid"
	SyncErrorNotFound                SyncErrorCode = "not_found"
	SyncErrorCursor                  SyncErrorCode = "cursor_conflict"
	SyncErrorArrivalGap              SyncErrorCode = "arrival_gap"
	SyncErrorConflict                SyncErrorCode = "immutable_conflict"
	SyncErrorEnvironmentGap          SyncErrorCode = "environment_gap"
	SyncErrorEnvelopeChain           SyncErrorCode = "envelope_chain"
	SyncErrorCertificate             SyncErrorCode = "environment_certificate"
	SyncErrorNonceReuse              SyncErrorCode = "nonce_reuse"
	SyncErrorHLC                     SyncErrorCode = "non_increasing_hlc"
	SyncErrorCandidate               SyncErrorCode = "invalid_candidate_union"
	SyncErrorNotAttached             SyncErrorCode = "not_attached"
	SyncErrorActivation              SyncErrorCode = "activation_incomplete"
	SyncErrorTerminalHistoryRequired SyncErrorCode = "terminal_history_required"
	SyncErrorRecoveryRequired        SyncErrorCode = "recovery_required"
	SyncErrorStore                   SyncErrorCode = "store_unavailable"
)

// SyncError reports a content-free, machine-stable sync persistence failure.
type SyncError struct {
	Code   SyncErrorCode
	Field  string
	Detail string
}

func (problem *SyncError) Error() string {
	if problem == nil {
		return "continuity sync error"
	}
	message := "continuity sync " + string(problem.Code)
	if problem.Field != "" {
		message += " at " + problem.Field
	}
	if problem.Detail != "" {
		message += ": " + problem.Detail
	}
	return message
}

// OpaqueSyncFrame is one untrusted relay arrival staged without interpreting
// its exact bytes. Exactly one of SealedEnvelope and PrunedArrival must be
// populated. EnvelopeDigest always identifies the original immutable sealed
// envelope; it does not identify the pruned-arrival wrapper. Quarantined is
// populated only by inbox reads.
type OpaqueSyncFrame struct {
	ArrivalSequence int64
	EnvelopeDigest  [32]byte
	SealedEnvelope  []byte
	PrunedArrival   []byte
	Quarantined     bool
}

// VerifiedSyncFrame binds a successfully opened persisted fact to its staged
// arrival and immutable full-envelope digest.
type VerifiedSyncFrame struct {
	ArrivalSequence        int64
	PreviousEnvelopeDigest [32]byte
	EnvelopeDigest         [32]byte
	CertificateID          [32]byte
	KeyGeneration          uint32
	Nonce                  [24]byte
	Fact                   continuitywire.Fact
}

// SealedOutboxFrame is one locally authored fact sealed exactly once for retry.
type SealedOutboxFrame struct {
	FactID                 continuity.FactID
	PreviousEnvelopeDigest [32]byte
	EnvelopeDigest         [32]byte
	CertificateID          [32]byte
	KeyGeneration          uint32
	Nonce                  [24]byte
	SealedEnvelope         []byte
}

// SyncProgress separates relay download progress from canonically applied
// progress. RelayHead is an observed pagination watermark, not authority.
type SyncProgress struct {
	ProjectID        continuity.ProjectID
	ChannelID        SyncChannelID
	ActivationState  SyncActivationState
	DownloadedCursor int64
	AppliedCursor    int64
	RelayHead        int64
}

// SyncActivationState distinguishes untrusted attach staging from a channel
// whose complete verified inventory has been explicitly activated.
type SyncActivationState string

const (
	// SyncActivationStaging contains opaque or verified attach work that is not
	// yet authorized for attached operation.
	SyncActivationStaging SyncActivationState = "staging"
	// SyncActivationAttached is the terminal channel-binding state.
	SyncActivationAttached SyncActivationState = "attached"
)

// ExportFact returns the exact immutable persisted-fact wire for factID.
func (store *Store) ExportFact(ctx context.Context, factID continuity.FactID) (continuitywire.Fact, error) {
	if err := factID.Validate(); err != nil {
		return continuitywire.Fact{}, syncProblem(SyncErrorInvalid, "fact_id", "is invalid")
	}
	if store == nil {
		return continuitywire.Fact{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return continuitywire.Fact{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return continuitywire.Fact{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return continuitywire.Fact{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return continuitywire.Fact{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	fact, found, err := readFactByIDV1(ctx, tx, factID)
	if err != nil {
		return continuitywire.Fact{}, err
	}
	if !found {
		return continuitywire.Fact{}, syncProblem(SyncErrorNotFound, "fact_id", "does not identify a retained fact")
	}
	wire := storedFactWireV1(fact)
	if err := continuitywire.Validate(wire); err != nil {
		return continuitywire.Fact{}, corruptFactProblemV1()
	}
	if err := tx.Commit(); err != nil {
		return continuitywire.Fact{}, syncTransactionProblem(ctx)
	}
	return wire, nil
}

// CurrentSyncProgress returns the retained staging or attached channel and its
// split cursors. A staging row is never evidence of successful attachment.
func (store *Store) CurrentSyncProgress(ctx context.Context, projectID continuity.ProjectID) (SyncProgress, error) {
	if err := projectID.Validate(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if store == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncProgress{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if !found {
		return SyncProgress{}, syncProblem(SyncErrorNotFound, "project_id", "has no sync state")
	}
	if err := tx.Commit(); err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	return progress, nil
}

// ActivateStagedSync marks one fully applied authority snapshot as attached.
// The exact canonical authority binding and its inventory-arrival cutoff are
// checked in the same transaction as the local durable activation conditions.
// An exact retry after an uncertain commit is idempotent.
func (store *Store) ActivateStagedSync(ctx context.Context, projectID continuity.ProjectID, expectedAuthority SyncAuthorityBinding) (SyncProgress, error) {
	if err := projectID.Validate(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if err := validateSyncAuthorityBindingV2(expectedAuthority); err != nil {
		return SyncProgress{}, err
	}
	if store == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncProgress{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	binding, err := requireExactCanonicalSyncAuthorityBindingV2(ctx, tx, projectID, expectedAuthority)
	if err != nil {
		return SyncProgress{}, err
	}
	if err := requireKnownExactSyncRelayWatermarkV1(
		ctx, tx, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
	); err != nil {
		return SyncProgress{}, err
	}
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if !found {
		return SyncProgress{}, syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	if progress.ChannelID != binding.ChannelID {
		return SyncProgress{}, syncProblem(SyncErrorStore, "sync_authority", "canonical authority and sync progress channels disagree")
	}
	if progress.ActivationState == SyncActivationAttached {
		if err := tx.Commit(); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		return progress, nil
	}
	activeAuthorityCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if activeAuthorityCandidate {
		return SyncProgress{}, syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before activation")
	}
	if progress.DownloadedCursor != progress.AppliedCursor {
		return SyncProgress{}, syncProblem(SyncErrorActivation, "cursor", "downloaded and applied cursors must agree")
	}
	if progress.AppliedCursor != binding.InventoryArrivalHead {
		return SyncProgress{}, syncProblem(SyncErrorConflict, "inventory_arrival_head", "does not match the fully applied staging cutoff")
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM continuity_sync_inbox
WHERE project_id = ?`, string(projectID)).Scan(&pending); err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	if pending != 0 {
		return SyncProgress{}, syncProblem(SyncErrorActivation, "inbox", "staged or quarantined arrivals remain")
	}
	facts, err := loadProjectFactsV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if len(facts) == 0 {
		return SyncProgress{}, syncProblem(SyncErrorActivation, "project_id", "canonical project root is missing")
	}
	if _, err := foldProjectSnapshotV1(ctx, projectID, 0, facts); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return SyncProgress{}, ctxErr
		}
		return SyncProgress{}, syncProblem(SyncErrorCandidate, "", "retained project corpus is not valid")
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET activation_state = 'attached'
WHERE project_id = ?
  AND activation_state = 'staging'
  AND channel_id = ?
  AND downloaded_cursor = ?
  AND applied_cursor = ?`, string(projectID), binding.ChannelID[:], binding.InventoryArrivalHead, binding.InventoryArrivalHead)
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return SyncProgress{}, err
	}
	progress.ActivationState = SyncActivationAttached
	if err := tx.Commit(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "activation commit outcome is unknown")
	}
	return progress, nil
}

// DiscardStagedSync removes an unactivated opaque staging channel so attach
// can be retried against another channel. It refuses after any canonical
// arrival was applied or after terminal activation.
func (store *Store) DiscardStagedSync(ctx context.Context, projectID continuity.ProjectID, channelID SyncChannelID) error {
	if err := projectID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if channelID == (SyncChannelID{}) {
		return syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if store == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	if progress.ChannelID != channelID {
		return syncProblem(SyncErrorConflict, "channel_id", "does not match the retained channel")
	}
	if progress.ActivationState != SyncActivationStaging {
		return syncProblem(SyncErrorConflict, "activation_state", "attached channel binding is terminal")
	}
	authorityCandidate, err := anySyncAuthorityCandidateExistsV2(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if authorityCandidate {
		return syncProblem(SyncErrorConflict, "sync_authority_candidate", "permanent authority candidate state prevents staged sync discard")
	}
	activeCandidate, err := activeTerminalCandidateExistsV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if activeCandidate {
		return syncProblem(SyncErrorConflict, "terminal_candidate", "must be discarded first")
	}
	if progress.AppliedCursor != 0 {
		return syncProblem(SyncErrorConflict, "applied_cursor", "canonical arrivals were already applied")
	}
	var durable int
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?)
  + (SELECT COUNT(*) FROM continuity_sync_outbox WHERE project_id = ?)
  + (SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?)`,
		string(projectID), string(projectID), string(projectID),
	).Scan(&durable); err != nil {
		return syncTransactionProblem(ctx)
	}
	if durable != 0 {
		return syncProblem(SyncErrorConflict, "sync_state", "durable sync identities already exist")
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_inbox
WHERE project_id = ?`, string(projectID)); err != nil {
		return syncTransactionProblem(ctx)
	}
	if _, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_environment_certificates
WHERE project_id = ?`, string(projectID)); err != nil {
		return syncTransactionProblem(ctx)
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_projects
WHERE project_id = ? AND activation_state = 'staging'`, string(projectID))
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "discard commit outcome is unknown")
	}
	return nil
}

// UnsealedLocalFact is the next locally authored fact and the only valid
// previous digest with which it may be sealed.
type UnsealedLocalFact struct {
	Fact                   continuitywire.Fact
	PreviousEnvelopeDigest [32]byte
}

// NextUnsealedLocalFact returns at most one fact because sealing it determines
// the previous digest for the following environment sequence.
func (store *Store) NextUnsealedLocalFact(ctx context.Context, projectID continuity.ProjectID) (UnsealedLocalFact, bool, error) {
	if err := projectID.Validate(); err != nil {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if store == nil {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return UnsealedLocalFact{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return UnsealedLocalFact{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return UnsealedLocalFact{}, false, err
	}
	if !found {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorNotFound, "project_id", "has no sync state")
	}
	if progress.ActivationState != SyncActivationAttached {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorNotAttached, "activation_state", "sealed outbox work requires an attached channel")
	}
	var sealedSequence int64
	var previousDigest []byte
	err = tx.QueryRowContext(ctx, `
SELECT sealed_sequence, envelope_digest
FROM continuity_sync_environment_heads
WHERE project_id = ? AND environment_id = ?`,
		string(projectID),
		string(store.environmentID),
	).Scan(&sealedSequence, &previousDigest)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return UnsealedLocalFact{}, false, syncTransactionProblem(ctx)
		}
		return UnsealedLocalFact{}, false, nil
	}
	if err != nil {
		return UnsealedLocalFact{}, false, syncTransactionProblem(ctx)
	}
	if sealedSequence == 0 {
		if previousDigest != nil {
			return UnsealedLocalFact{}, false, syncProblem(SyncErrorStore, "", "unsealed environment head carries a digest")
		}
	} else if len(previousDigest) != 32 {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorStore, "", "sealed environment head digest is corrupt")
	}
	if sealedSequence == math.MaxInt64 {
		return UnsealedLocalFact{}, false, syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "local environment sequence is exhausted")
	}
	nextSequence := sealedSequence + 1
	row := tx.QueryRowContext(ctx, `
SELECT
  fact.fact_id,
  fact.project_id,
  fact.subject_kind,
  fact.subject_id,
  fact.fact_kind,
  fact.payload_version,
  fact.content_json,
  fact.environment_id,
  fact.environment_sequence,
  fact.hlc_wall_millis,
  fact.hlc_logical,
  fact.envelope_version
FROM continuity_facts AS fact
LEFT JOIN continuity_sync_outbox AS outbox
  ON outbox.fact_id = fact.fact_id
LEFT JOIN continuity_sync_receipts AS receipt
  ON receipt.project_id = fact.project_id AND receipt.fact_id = fact.fact_id
LEFT JOIN continuity_sync_tombstones AS tombstone
  ON tombstone.fact_id = fact.fact_id
WHERE fact.project_id = ?
  AND fact.environment_id = ?
  AND fact.environment_sequence = ?
  AND outbox.fact_id IS NULL
  AND receipt.fact_id IS NULL
  AND tombstone.fact_id IS NULL`,
		string(projectID),
		string(store.environmentID),
		nextSequence,
	)
	fact, err := scanStoredFactRowV1(ctx, row)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return UnsealedLocalFact{}, false, syncTransactionProblem(ctx)
		}
		return UnsealedLocalFact{}, false, nil
	}
	if err != nil {
		return UnsealedLocalFact{}, false, err
	}
	result := UnsealedLocalFact{Fact: storedFactWireV1(fact)}
	copy(result.PreviousEnvelopeDigest[:], previousDigest)
	if err := tx.Commit(); err != nil {
		return UnsealedLocalFact{}, false, syncTransactionProblem(ctx)
	}
	return result, true, nil
}

// PersistSealedOutbox records exact local envelope bytes before first upload.
// An exact retry is idempotent; any changed immutable byte is a conflict.
func (store *Store) PersistSealedOutbox(ctx context.Context, projectID continuity.ProjectID, channelID SyncChannelID, frame SealedOutboxFrame) error {
	if err := projectID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if channelID == (SyncChannelID{}) {
		return syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if err := frame.FactID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, "fact_id", "is invalid")
	}
	if len(frame.SealedEnvelope) < 1 || len(frame.SealedEnvelope) > maximumSealedEnvelopeBytes {
		return syncProblem(SyncErrorInvalid, "sealed_envelope", "size is outside the protocol limit")
	}
	if sha256.Sum256(frame.SealedEnvelope) != frame.EnvelopeDigest {
		return syncProblem(SyncErrorInvalid, "envelope_digest", "does not identify the sealed envelope bytes")
	}
	if store == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return syncProblem(SyncErrorNotFound, "project_id", "has no sync state")
	}
	if progress.ChannelID != channelID {
		return syncProblem(SyncErrorConflict, "channel_id", "does not match the retained channel")
	}
	if progress.ActivationState != SyncActivationAttached {
		return syncProblem(SyncErrorNotAttached, "activation_state", "sealed outbox work requires an attached channel")
	}
	fact, found, err := readFactByIDV1(ctx, tx, frame.FactID)
	if err != nil {
		return err
	}
	if !found || fact.projectID != projectID || fact.environmentID != store.environmentID {
		return syncProblem(SyncErrorNotFound, "fact_id", "does not identify a local project fact")
	}
	metadata := sealedEnvelopeMetadataV1{
		previousDigest: frame.PreviousEnvelopeDigest,
		digest:         frame.EnvelopeDigest,
		certificateID:  frame.CertificateID,
		keyGeneration:  frame.KeyGeneration,
		nonce:          frame.Nonce,
	}
	if err := validateSealedMetadataV1(fact.environmentSequence, metadata); err != nil {
		return err
	}
	consumed, err := consumedEnvelopeRetryV1(ctx, tx, projectID, fact, metadata)
	if err != nil {
		return err
	}
	if consumed {
		if err := tx.Commit(); err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	existing, found, err := readSealedOutboxV1(ctx, tx, frame.FactID)
	if err != nil {
		return err
	}
	if found {
		if existing.FactID != frame.FactID ||
			existing.PreviousEnvelopeDigest != frame.PreviousEnvelopeDigest ||
			existing.EnvelopeDigest != frame.EnvelopeDigest ||
			existing.CertificateID != frame.CertificateID ||
			existing.KeyGeneration != frame.KeyGeneration ||
			existing.Nonce != frame.Nonce ||
			!bytes.Equal(existing.SealedEnvelope, frame.SealedEnvelope) {
			return syncProblem(SyncErrorConflict, "fact_id", "already has different sealed-once bytes")
		}
		if _, err := loadEnvelopeInventoryV1(ctx, tx, projectID); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	inventory, err := loadEnvelopeInventoryV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if err := inventory.admit(fact.environmentID, fact.environmentSequence, metadata); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_outbox(
  fact_id,
  project_id,
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce,
  sealed_envelope
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(frame.FactID),
		string(projectID),
		string(fact.environmentID),
		fact.environmentSequence,
		frame.PreviousEnvelopeDigest[:],
		frame.EnvelopeDigest[:],
		frame.CertificateID[:],
		frame.KeyGeneration,
		frame.Nonce[:],
		frame.SealedEnvelope,
	); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := recordSealedEnvironmentHeadV1(ctx, tx, fact, metadata); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "sealed outbox commit outcome is unknown")
	}
	return nil
}

// PendingSealedOutbox returns byte-exact retry envelopes in source order.
func (store *Store) PendingSealedOutbox(ctx context.Context, projectID continuity.ProjectID, limit int) ([]SealedOutboxFrame, error) {
	if err := projectID.Validate(); err != nil {
		return nil, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if limit < 1 || limit > maximumSyncPageFrames {
		return nil, syncProblem(SyncErrorInvalid, "limit", "must be between 1 and 256")
	}
	if store == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return nil, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, syncProblem(SyncErrorNotFound, "project_id", "has no sync state")
	}
	if progress.ActivationState != SyncActivationAttached {
		return nil, syncProblem(SyncErrorNotAttached, "activation_state", "sealed outbox work requires an attached channel")
	}
	rows, err := tx.QueryContext(ctx, `
SELECT
  fact_id,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce,
  sealed_envelope
FROM continuity_sync_outbox
WHERE project_id = ?
ORDER BY environment_id, environment_sequence
LIMIT ?`, string(projectID), limit)
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	result := make([]SealedOutboxFrame, 0, limit)
	for rows.Next() {
		frame, err := scanSealedOutboxV1(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, frame)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	if err := tx.Commit(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return result, nil
}

// StageSyncPage atomically stages one contiguous opaque relay page and advances
// only the downloaded cursor.
func (store *Store) StageSyncPage(ctx context.Context, projectID continuity.ProjectID, channelID SyncChannelID, expectedAfter, relayHead int64, frames []OpaqueSyncFrame) (SyncProgress, error) {
	if err := validateStageSyncPage(projectID, channelID, expectedAfter, relayHead, frames); err != nil {
		return SyncProgress{}, err
	}
	if store == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncProgress{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if !found {
		return SyncProgress{}, syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	} else {
		if progress.ChannelID != channelID {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "channel_id", "does not match the retained channel")
		}
		if relayHead < progress.RelayHead {
			return SyncProgress{}, syncProblem(SyncErrorCursor, "relay_head", "regressed below the retained watermark")
		}
		if progress.DownloadedCursor != expectedAfter {
			if expectedAfter < progress.DownloadedCursor && len(frames) != 0 &&
				frames[len(frames)-1].ArrivalSequence <= progress.DownloadedCursor {
				if err := validateStagedPageReplayV1(ctx, tx, projectID, progress.AppliedCursor, frames); err != nil {
					return SyncProgress{}, err
				}
				if _, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET relay_head = ?
WHERE project_id = ?`, relayHead, string(projectID)); err != nil {
					return SyncProgress{}, syncTransactionProblem(ctx)
				}
				progress.RelayHead = relayHead
				if err := tx.Commit(); err != nil {
					return SyncProgress{}, syncProblem(SyncErrorStore, "", "stage retry commit outcome is unknown")
				}
				return progress, nil
			}
			return SyncProgress{}, syncProblem(SyncErrorCursor, "expected_after", "does not match downloaded progress")
		}
	}

	seenDigests := make(map[[32]byte]struct{}, len(frames))
	for _, frame := range frames {
		frameKind, frameBytes, err := opaqueSyncFrameStorageV1(frame)
		if err != nil {
			return SyncProgress{}, err
		}
		if _, duplicate := seenDigests[frame.EnvelopeDigest]; duplicate {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "envelope_digest", "appears more than once in the page")
		}
		seenDigests[frame.EnvelopeDigest] = struct{}{}
		var known int
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_receipts
  WHERE project_id = ? AND envelope_digest = ?
  UNION ALL
  SELECT 1
  FROM continuity_sync_tombstones
  WHERE project_id = ? AND envelope_digest = ?
  UNION ALL
  SELECT 1
  FROM continuity_sync_inbox
  WHERE project_id = ? AND envelope_digest = ?
)`,
			string(projectID),
			frame.EnvelopeDigest[:],
			string(projectID),
			frame.EnvelopeDigest[:],
			string(projectID),
			frame.EnvelopeDigest[:],
		).Scan(&known); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		if known != 0 {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "envelope_digest", "was already consumed at another arrival")
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES(?, ?, ?, ?, ?, 'staged')`,
			string(projectID),
			frame.ArrivalSequence,
			frame.EnvelopeDigest[:],
			frameKind,
			frameBytes,
		); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
	}
	downloaded := expectedAfter
	if len(frames) != 0 {
		downloaded = frames[len(frames)-1].ArrivalSequence
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET downloaded_cursor = ?, relay_head = ?
WHERE project_id = ?`, downloaded, relayHead, string(projectID))
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return SyncProgress{}, err
	}
	progress.DownloadedCursor = downloaded
	progress.RelayHead = relayHead
	if err := tx.Commit(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "stage commit outcome is unknown")
	}
	return progress, nil
}

// PendingSyncFrames returns the next bounded opaque applied-prefix candidates.
func (store *Store) PendingSyncFrames(ctx context.Context, projectID continuity.ProjectID, limit int) ([]OpaqueSyncFrame, error) {
	if err := projectID.Validate(); err != nil {
		return nil, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if limit < 1 || limit > maximumSyncPageFrames {
		return nil, syncProblem(SyncErrorInvalid, "limit", "must be between 1 and 256")
	}
	if store == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return nil, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	rows, err := tx.QueryContext(ctx, `
SELECT arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence > ?
ORDER BY arrival_sequence
LIMIT ?`, string(projectID), progress.AppliedCursor, limit)
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	frames := make([]OpaqueSyncFrame, 0, limit)
	expected := progress.AppliedCursor + 1
	for rows.Next() {
		var arrivalSequence int64
		var digest []byte
		var frameBytes []byte
		var frameKind, state string
		if err := rows.Scan(&arrivalSequence, &digest, &frameKind, &frameBytes, &state); err != nil {
			rows.Close()
			return nil, syncTransactionProblem(ctx)
		}
		frame, err := opaqueSyncFrameFromColumnsV1(arrivalSequence, digest, frameKind, frameBytes, state)
		if err != nil || frame.ArrivalSequence != expected {
			rows.Close()
			return nil, syncProblem(SyncErrorStore, "", "staged inbox is inconsistent")
		}
		frames = append(frames, frame)
		expected++
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	if progress.DownloadedCursor > progress.AppliedCursor && len(frames) == 0 {
		return nil, syncProblem(SyncErrorStore, "", "downloaded cursor outruns the staged inbox")
	}
	if err := tx.Commit(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return frames, nil
}

type preparedVerifiedSyncFrame struct {
	arrival int64
	sealedEnvelopeMetadataV1
	fact storedFactV1
}

type environmentFrontierV1 struct {
	sequence int64
	clock    continuity.HybridTime
}

type tombstoneV1 struct {
	factID              continuity.FactID
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	sealedEnvelopeMetadataV1
}

type sealedEnvelopeMetadataV1 struct {
	previousDigest [32]byte
	digest         [32]byte
	certificateID  [32]byte
	keyGeneration  uint32
	nonce          [24]byte
}

type envelopeInventoryEntryV1 struct {
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	metadata            sealedEnvelopeMetadataV1
}

type envelopeInventoryV1 struct {
	bySequence        map[string]envelopeInventoryEntryV1
	certificateByEnv  map[continuity.EnvironmentID][32]byte
	byGenerationNonce map[string]envelopeInventoryEntryV1
}

// ApplySyncBatch validates the caller-verified authority binding and complete
// supplied candidate union, then atomically advances only its contiguous
// non-future prefix.
func (store *Store) ApplySyncBatch(
	ctx context.Context,
	projectID continuity.ProjectID,
	verifiedAuthority SyncAuthorityBinding,
	frames []VerifiedSyncFrame,
	trustedNowMillis,
	maxFutureSkewMillis int64,
) (SyncProgress, error) {
	if store == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncProgress{}, err
	}
	if err := projectID.Validate(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	store.mu.RLock()
	if store.closed || store.db == nil {
		store.mu.RUnlock()
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	var activeCandidate int
	err := store.db.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_terminal_candidates
  WHERE project_id = ? AND state = 'staging'
)`, string(projectID)).Scan(&activeCandidate)
	store.mu.RUnlock()
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	if activeCandidate != 0 {
		return SyncProgress{}, syncProblem(SyncErrorTerminalHistoryRequired, "", "")
	}
	if err := validateSyncAuthorityBindingV2(verifiedAuthority); err != nil {
		return SyncProgress{}, err
	}
	prepared, err := prepareVerifiedSyncFrames(projectID, frames, trustedNowMillis, maxFutureSkewMillis)
	if err != nil {
		return SyncProgress{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if !found {
		return SyncProgress{}, syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	active, err := activeTerminalCandidateExistsV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if active {
		return SyncProgress{}, syncProblem(SyncErrorTerminalHistoryRequired, "", "")
	}
	binding, err := requireExactCanonicalSyncAuthorityBindingV2(ctx, tx, projectID, verifiedAuthority)
	if err != nil {
		return SyncProgress{}, err
	}
	if len(prepared) == 0 {
		if err := tx.Commit(); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		return progress, nil
	}
	if err := validateStagedBindingsV1(ctx, tx, projectID, progress, prepared); err != nil {
		return SyncProgress{}, err
	}
	environmentIDs := make([]continuity.EnvironmentID, len(prepared))
	for index := range prepared {
		environmentIDs[index] = prepared[index].fact.environmentID
	}
	authorityEnvironments, err := readCanonicalSyncEnvironmentCertificatesV2(ctx, tx, projectID, binding, environmentIDs)
	if err != nil {
		return SyncProgress{}, err
	}

	existing, err := loadProjectFactsV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	retainedFacts := append([]storedFactV1(nil), existing...)
	byFactID := make(map[continuity.FactID]storedFactV1, len(existing)+len(prepared))
	byEnvironmentSequence := make(map[string]storedFactV1, len(existing)+len(prepared))
	for _, fact := range existing {
		byFactID[fact.factID] = fact
		byEnvironmentSequence[environmentSequenceKeyV1(fact.environmentID, fact.environmentSequence)] = fact
	}
	frontiers, err := loadEnvironmentFrontiersV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	tombstonesByFact, tombstonesBySequence, err := loadTombstonesV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	envelopeInventory, err := loadEnvelopeInventoryV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	isFirstSeenEnvelope := make([]bool, len(prepared))
	for index, frame := range prepared {
		sequenceKey := environmentSequenceKeyV1(frame.fact.environmentID, frame.fact.environmentSequence)
		retainedEnvelope, hasRetainedEnvelope := envelopeInventory.bySequence[sequenceKey]
		isFirstSeenEnvelope[index] = !hasRetainedEnvelope ||
			!sealedMetadataEqualV1(retainedEnvelope.metadata, frame.sealedEnvelopeMetadataV1)
	}
	if err := validateOrdinarySyncFrameAuthorityV2(authorityEnvironments, prepared, isFirstSeenEnvelope, trustedNowMillis); err != nil {
		return SyncProgress{}, err
	}
	if err := rejectConsumedReceiptsV1(ctx, tx, projectID, prepared); err != nil {
		return SyncProgress{}, err
	}

	isNew := make([]bool, len(prepared))
	seenFacts := make(map[continuity.FactID]struct{}, len(prepared))
	seenSequences := make(map[string]struct{}, len(prepared))
	for index, frame := range prepared {
		fact := frame.fact
		if _, duplicate := seenFacts[fact.factID]; duplicate {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "fact_id", "appears more than once in the batch")
		}
		seenFacts[fact.factID] = struct{}{}
		sequenceKey := environmentSequenceKeyV1(fact.environmentID, fact.environmentSequence)
		if _, duplicate := seenSequences[sequenceKey]; duplicate {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "environment_sequence", "appears more than once in the batch")
		}
		seenSequences[sequenceKey] = struct{}{}
		if _, retainedInProject := byFactID[fact.factID]; !retainedInProject {
			retained, found, err := readFactByIDV1(ctx, tx, fact.factID)
			if err != nil {
				return SyncProgress{}, err
			}
			if found {
				if retained.projectID != projectID {
					return SyncProgress{}, syncProblem(SyncErrorConflict, "fact_id", "is already bound to another project")
				}
				return SyncProgress{}, syncProblem(SyncErrorStore, "", "project fact index is inconsistent")
			}
		}
		if _, tombstonedInProject := tombstonesByFact[fact.factID]; !tombstonedInProject {
			var tombstoneProjectID continuity.ProjectID
			err := tx.QueryRowContext(ctx, `
SELECT project_id
FROM continuity_sync_tombstones
WHERE fact_id = ?`, string(fact.factID)).Scan(&tombstoneProjectID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return SyncProgress{}, syncTransactionProblem(ctx)
			}
			if err == nil {
				if tombstoneProjectID != projectID {
					return SyncProgress{}, syncProblem(SyncErrorConflict, "fact_id", "is tombstoned for another project")
				}
				return SyncProgress{}, syncProblem(SyncErrorStore, "", "project tombstone index is inconsistent")
			}
		}

		if tombstone, ok := tombstonesByFact[fact.factID]; ok {
			if tombstone.environmentID != fact.environmentID ||
				tombstone.environmentSequence != fact.environmentSequence ||
				tombstone.digest != frame.digest {
				return SyncProgress{}, syncProblem(SyncErrorConflict, "fact_id", "conflicts with a prune tombstone")
			}
			if frontier, ok := frontiers[fact.environmentID]; !ok || frontier.sequence < fact.environmentSequence {
				return SyncProgress{}, syncProblem(SyncErrorStore, "", "tombstone outruns its environment head")
			}
			if err := envelopeInventory.admit(fact.environmentID, fact.environmentSequence, frame.sealedEnvelopeMetadataV1); err != nil {
				return SyncProgress{}, err
			}
			continue
		}
		if tombstone, ok := tombstonesBySequence[sequenceKey]; ok {
			if tombstone.factID != fact.factID || tombstone.digest != frame.digest {
				return SyncProgress{}, syncProblem(SyncErrorConflict, "environment_sequence", "conflicts with a prune tombstone")
			}
			if err := envelopeInventory.admit(fact.environmentID, fact.environmentSequence, frame.sealedEnvelopeMetadataV1); err != nil {
				return SyncProgress{}, err
			}
			continue
		}
		if current, ok := byFactID[fact.factID]; ok {
			if !storedFactsEqualV1(current, fact) {
				return SyncProgress{}, syncProblem(SyncErrorConflict, "fact_id", "is bound to different immutable fields")
			}
			if bySequence, ok := byEnvironmentSequence[sequenceKey]; !ok || !storedFactsEqualV1(bySequence, fact) {
				return SyncProgress{}, syncProblem(SyncErrorConflict, "environment_sequence", "does not match the retained fact")
			}
			if frontier, ok := frontiers[fact.environmentID]; !ok || frontier.sequence < fact.environmentSequence {
				return SyncProgress{}, syncProblem(SyncErrorStore, "", "retained fact outruns its environment head")
			}
			if err := envelopeInventory.admit(fact.environmentID, fact.environmentSequence, frame.sealedEnvelopeMetadataV1); err != nil {
				return SyncProgress{}, err
			}
			continue
		}
		if current, ok := byEnvironmentSequence[sequenceKey]; ok {
			if !storedFactsEqualV1(current, fact) {
				return SyncProgress{}, syncProblem(SyncErrorConflict, "environment_sequence", "is bound to a different fact")
			}
			if err := envelopeInventory.admit(fact.environmentID, fact.environmentSequence, frame.sealedEnvelopeMetadataV1); err != nil {
				return SyncProgress{}, err
			}
			continue
		}
		frontier, hasFrontier := frontiers[fact.environmentID]
		expectedSequence := int64(1)
		if hasFrontier {
			if frontier.sequence == math.MaxInt64 {
				return SyncProgress{}, syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "environment sequence is exhausted")
			}
			expectedSequence = frontier.sequence + 1
		}
		if fact.environmentSequence != expectedSequence {
			return SyncProgress{}, syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "does not extend the contiguous environment prefix")
		}
		if hasFrontier && !hybridTimeLessV1(frontier.clock, fact.clock) {
			return SyncProgress{}, syncProblem(SyncErrorHLC, "hlc", "does not increase with environment sequence")
		}
		if err := envelopeInventory.admit(fact.environmentID, fact.environmentSequence, frame.sealedEnvelopeMetadataV1); err != nil {
			return SyncProgress{}, err
		}
		frontiers[fact.environmentID] = environmentFrontierV1{sequence: fact.environmentSequence, clock: fact.clock}
		byFactID[fact.factID] = fact
		byEnvironmentSequence[sequenceKey] = fact
		existing = append(existing, fact)
		isNew[index] = true
	}
	sort.Slice(existing, func(left, right int) bool {
		return storedFactLessV1(existing[left], existing[right])
	})
	if len(existing) == 0 {
		return SyncProgress{}, syncProblem(SyncErrorCandidate, "", "candidate corpus has no project identity")
	}
	if _, err := foldProjectSnapshotV1(ctx, projectID, 0, existing); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return SyncProgress{}, ctxErr
		}
		return SyncProgress{}, syncProblem(SyncErrorCandidate, "", "complete candidate corpus is not valid")
	}

	futureIndex := len(prepared)
	for index, frame := range prepared {
		if futureSkewedV1(frame.fact.clock.WallMillis, trustedNowMillis, maxFutureSkewMillis) {
			futureIndex = index
			break
		}
	}
	applyCount := futureIndex
	if applyCount != len(prepared) && applyCount != 0 {
		prefixCandidate := append([]storedFactV1(nil), retainedFacts...)
		for index := 0; index < applyCount; index++ {
			if isNew[index] {
				prefixCandidate = append(prefixCandidate, prepared[index].fact)
			}
		}
		sort.Slice(prefixCandidate, func(left, right int) bool {
			return storedFactLessV1(prefixCandidate[left], prefixCandidate[right])
		})
		if len(prefixCandidate) == 0 {
			applyCount = 0
		} else if _, err := foldProjectSnapshotV1(ctx, projectID, 0, prefixCandidate); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return SyncProgress{}, ctxErr
			}
			applyCount = 0
		}
	}

	newFacts := make([]storedFactV1, 0, applyCount)
	for index := 0; index < applyCount; index++ {
		if isNew[index] {
			newFacts = append(newFacts, prepared[index].fact)
		}
	}
	sort.Slice(newFacts, func(left, right int) bool {
		return storedFactLessV1(newFacts[left], newFacts[right])
	})
	insertFact := func(fact storedFactV1) error {
		intent := appendIntentV1{
			projectID: fact.projectID,
			factID:    fact.factID,
			subject:   fact.subject,
			kind:      fact.kind,
			content:   fact.content,
		}
		if err := insertFactV1(ctx, tx, intent, fact.environmentID, fact.environmentSequence, fact.clock); err != nil {
			return err
		}
		return nil
	}
	rootIndex := -1
	for index, fact := range newFacts {
		if fact.kind == continuity.FactProjectRegistered {
			rootIndex = index
			break
		}
	}
	if rootIndex >= 0 {
		if err := insertFact(newFacts[rootIndex]); err != nil {
			return SyncProgress{}, err
		}
	}
	for index, fact := range newFacts {
		if index == rootIndex {
			continue
		}
		if err := insertFact(fact); err != nil {
			return SyncProgress{}, err
		}
	}
	// Environment heads follow source sequence, independently of the one SQL
	// insertion reordered above to satisfy the project-root trigger.
	for _, fact := range newFacts {
		if err := advanceEnvironmentHeadV1(ctx, tx, fact); err != nil {
			return SyncProgress{}, err
		}
	}

	for index := 0; index < applyCount; index++ {
		frame := prepared[index]
		if err := recordSealedEnvironmentHeadV1(ctx, tx, frame.fact, frame.sealedEnvelopeMetadataV1); err != nil {
			return SyncProgress{}, err
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_receipts(
  project_id,
  arrival_sequence,
  fact_id,
  environment_id,
  environment_sequence,
	previous_envelope_digest,
	envelope_digest,
	certificate_id,
	key_generation,
	nonce
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(projectID),
			frame.arrival,
			string(frame.fact.factID),
			string(frame.fact.environmentID),
			frame.fact.environmentSequence,
			frame.previousDigest[:],
			frame.digest[:],
			frame.certificateID[:],
			frame.keyGeneration,
			frame.nonce[:],
		); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		if _, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_outbox
WHERE project_id = ? AND fact_id = ? AND envelope_digest = ?`,
			string(projectID),
			string(frame.fact.factID),
			frame.digest[:],
		); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence = ? AND frame_kind = 'sealed'`, string(projectID), frame.arrival)
		if err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return SyncProgress{}, err
		}
	}
	if futureIndex < len(prepared) {
		result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_inbox
SET state = 'quarantined'
WHERE project_id = ? AND arrival_sequence = ? AND frame_kind = 'sealed'`,
			string(projectID),
			prepared[futureIndex].arrival,
		)
		if err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return SyncProgress{}, err
		}
	}
	if applyCount != 0 {
		progress.AppliedCursor = prepared[applyCount-1].arrival
		result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET applied_cursor = ?
WHERE project_id = ?`, progress.AppliedCursor, string(projectID))
		if err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return SyncProgress{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "apply commit outcome is unknown")
	}
	return progress, nil
}

func validateStageSyncPage(projectID continuity.ProjectID, channelID SyncChannelID, expectedAfter, relayHead int64, frames []OpaqueSyncFrame) error {
	if err := projectID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if channelID == (SyncChannelID{}) {
		return syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if expectedAfter < 0 || relayHead < expectedAfter {
		return syncProblem(SyncErrorInvalid, "cursor", "is negative or exceeds relay head")
	}
	if len(frames) > maximumSyncPageFrames {
		return syncProblem(SyncErrorInvalid, "frames", "exceeds 256 arrivals")
	}
	if len(frames) != 0 && expectedAfter == math.MaxInt64 {
		return syncProblem(SyncErrorInvalid, "expected_after", "cursor is exhausted")
	}
	expected := expectedAfter + 1
	for _, frame := range frames {
		if frame.ArrivalSequence != expected {
			return syncProblem(SyncErrorArrivalGap, "arrival_sequence", "page is not contiguous after expected cursor")
		}
		if frame.Quarantined {
			return syncProblem(SyncErrorInvalid, "quarantined", "is output-only state")
		}
		if _, _, err := opaqueSyncFrameStorageV1(frame); err != nil {
			return err
		}
		if expected == math.MaxInt64 && frame.ArrivalSequence != frames[len(frames)-1].ArrivalSequence {
			return syncProblem(SyncErrorInvalid, "arrival_sequence", "cursor is exhausted")
		}
		expected++
	}
	if len(frames) != 0 && frames[len(frames)-1].ArrivalSequence > relayHead {
		return syncProblem(SyncErrorCursor, "relay_head", "is below the staged page")
	}
	return nil
}

func prepareVerifiedSyncFrames(projectID continuity.ProjectID, frames []VerifiedSyncFrame, trustedNowMillis, maxFutureSkewMillis int64) ([]preparedVerifiedSyncFrame, error) {
	if err := projectID.Validate(); err != nil {
		return nil, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if len(frames) > maximumSyncPageFrames {
		return nil, syncProblem(SyncErrorInvalid, "frames", "exceeds 256 arrivals")
	}
	if trustedNowMillis < 0 || maxFutureSkewMillis < 0 {
		return nil, syncProblem(SyncErrorInvalid, "clock", "trusted time and skew must be nonnegative")
	}
	prepared := make([]preparedVerifiedSyncFrame, 0, len(frames))
	for _, frame := range frames {
		if frame.ArrivalSequence < 1 {
			return nil, syncProblem(SyncErrorInvalid, "arrival_sequence", "must begin at one")
		}
		if frame.Fact.ProjectID != projectID {
			return nil, syncProblem(SyncErrorConflict, "project_id", "inner fact does not match the receive project")
		}
		if err := continuitywire.Validate(frame.Fact); err != nil {
			return nil, syncProblem(SyncErrorInvalid, "fact", "does not match the persisted-fact wire")
		}
		canonical, err := canonicalizeStoredContentV1(frame.Fact.FactKind, int(frame.Fact.PayloadVersion), string(frame.Fact.CanonicalPayload))
		if err != nil || !bytes.Equal([]byte(canonical), frame.Fact.CanonicalPayload) {
			return nil, syncProblem(SyncErrorInvalid, "payload_json", "does not match the closed canonical payload")
		}
		fact := storedFactV1{
			factID:              frame.Fact.FactID,
			projectID:           frame.Fact.ProjectID,
			subject:             continuity.SubjectRef{Kind: frame.Fact.SubjectKind, ID: frame.Fact.SubjectID},
			kind:                frame.Fact.FactKind,
			payloadVersion:      int(frame.Fact.PayloadVersion),
			content:             canonical,
			environmentID:       frame.Fact.EnvironmentID,
			environmentSequence: frame.Fact.EnvironmentSequence,
			clock:               continuity.HybridTime{WallMillis: frame.Fact.HLCWallMillis, Logical: frame.Fact.HLCLogical},
			envelopeVersion:     int(frame.Fact.EnvelopeVersion),
		}
		if err := validateStoredFactV1(fact); err != nil {
			return nil, syncProblem(SyncErrorInvalid, "fact", "does not match persisted continuity constraints")
		}
		metadata := sealedEnvelopeMetadataV1{
			previousDigest: frame.PreviousEnvelopeDigest,
			digest:         frame.EnvelopeDigest,
			certificateID:  frame.CertificateID,
			keyGeneration:  frame.KeyGeneration,
			nonce:          frame.Nonce,
		}
		if err := validateSealedMetadataV1(fact.environmentSequence, metadata); err != nil {
			return nil, err
		}
		prepared = append(prepared, preparedVerifiedSyncFrame{
			arrival:                  frame.ArrivalSequence,
			sealedEnvelopeMetadataV1: metadata,
			fact:                     fact,
		})
	}
	return prepared, nil
}

func validateStagedBindingsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, progress SyncProgress, frames []preparedVerifiedSyncFrame) error {
	expected := progress.AppliedCursor + 1
	for _, frame := range frames {
		if frame.arrival != expected || frame.arrival > progress.DownloadedCursor {
			return syncProblem(SyncErrorArrivalGap, "arrival_sequence", "verified batch is not the staged applied prefix")
		}
		var digest []byte
		var frameKind, state string
		if err := tx.QueryRowContext(ctx, `
SELECT envelope_digest, frame_kind, state
FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence = ?`, string(projectID), frame.arrival).Scan(&digest, &frameKind, &state); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return syncProblem(SyncErrorArrivalGap, "arrival_sequence", "has no staged envelope")
			}
			return syncTransactionProblem(ctx)
		}
		if frameKind != "sealed" {
			return syncProblem(SyncErrorTerminalHistoryRequired, "", "")
		}
		if len(digest) != len(frame.digest) || !bytes.Equal(digest, frame.digest[:]) ||
			(state != "staged" && state != "quarantined") {
			return syncProblem(SyncErrorConflict, "envelope_digest", "does not match the staged envelope")
		}
		expected++
	}
	return nil
}

func validateStagedPageReplayV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, appliedCursor int64, frames []OpaqueSyncFrame) error {
	for _, frame := range frames {
		if frame.ArrivalSequence <= appliedCursor {
			candidateKind, _, err := opaqueSyncFrameStorageV1(frame)
			if err != nil {
				return err
			}
			if candidateKind == terminalCandidateFrameKindPrunedV1 {
				return syncProblem(SyncErrorConflict, "frame_bytes", "applied pruned arrival bytes are no longer retained")
			}
			rows, err := tx.QueryContext(ctx, `
SELECT envelope_digest, 'sealed'
FROM continuity_sync_receipts
WHERE project_id = ? AND arrival_sequence = ?
UNION ALL
SELECT envelope_digest, 'pruned'
FROM continuity_sync_tombstones
WHERE project_id = ? AND arrival_sequence = ?`,
				string(projectID),
				frame.ArrivalSequence,
				string(projectID),
				frame.ArrivalSequence,
			)
			if err != nil {
				return syncTransactionProblem(ctx)
			}
			matched := false
			for rows.Next() {
				var digest []byte
				var retainedKind string
				if err := rows.Scan(&digest, &retainedKind); err != nil {
					rows.Close()
					return syncTransactionProblem(ctx)
				}
				if retainedKind != candidateKind || len(digest) != len(frame.EnvelopeDigest) ||
					!bytes.Equal(digest, frame.EnvelopeDigest[:]) {
					rows.Close()
					return syncProblem(SyncErrorConflict, "frame_bytes", "stage retry conflicts with an applied arrival")
				}
				matched = true
			}
			if err := rows.Close(); err != nil {
				return syncTransactionProblem(ctx)
			}
			if !matched {
				return syncProblem(SyncErrorStore, "", "applied arrival has no immutable receipt")
			}
			continue
		}
		var digest, retainedBytes []byte
		var frameKind string
		if err := tx.QueryRowContext(ctx, `
SELECT envelope_digest, frame_kind, frame_bytes
FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence = ?`,
			string(projectID),
			frame.ArrivalSequence,
		).Scan(&digest, &frameKind, &retainedBytes); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return syncProblem(SyncErrorStore, "", "downloaded arrival has no staged envelope")
			}
			return syncTransactionProblem(ctx)
		}
		candidateKind, candidateBytes, err := opaqueSyncFrameStorageV1(frame)
		if err != nil {
			return err
		}
		if len(digest) != len(frame.EnvelopeDigest) ||
			!bytes.Equal(digest, frame.EnvelopeDigest[:]) ||
			frameKind != candidateKind ||
			!bytes.Equal(retainedBytes, candidateBytes) {
			return syncProblem(SyncErrorConflict, "frame_bytes", "stage retry differs from retained bytes")
		}
	}
	return nil
}

func opaqueSyncFrameStorageV1(frame OpaqueSyncFrame) (string, []byte, error) {
	sealedPresent := frame.SealedEnvelope != nil
	prunedPresent := frame.PrunedArrival != nil
	if sealedPresent == prunedPresent {
		return "", nil, syncProblem(SyncErrorInvalid, "frame_bytes", "must contain exactly one opaque frame representation")
	}
	if frame.EnvelopeDigest == ([32]byte{}) {
		return "", nil, syncProblem(SyncErrorInvalid, "envelope_digest", "is invalid")
	}
	if sealedPresent {
		if len(frame.SealedEnvelope) < 1 || len(frame.SealedEnvelope) > maximumSealedEnvelopeBytes {
			return "", nil, syncProblem(SyncErrorInvalid, "sealed_envelope", "size is outside the protocol limit")
		}
		if sha256.Sum256(frame.SealedEnvelope) != frame.EnvelopeDigest {
			return "", nil, syncProblem(SyncErrorInvalid, "envelope_digest", "does not identify the sealed envelope bytes")
		}
		return terminalCandidateFrameKindSealedV1, frame.SealedEnvelope, nil
	}
	if len(frame.PrunedArrival) < 1 || len(frame.PrunedArrival) > maximumPrunedArrivalBytes {
		return "", nil, syncProblem(SyncErrorInvalid, "pruned_arrival", "size is outside the protocol limit")
	}
	return terminalCandidateFrameKindPrunedV1, frame.PrunedArrival, nil
}

func opaqueSyncFrameFromColumnsV1(arrivalSequence int64, digest []byte, frameKind string, frameBytes []byte, state string) (OpaqueSyncFrame, error) {
	frame := OpaqueSyncFrame{ArrivalSequence: arrivalSequence, Quarantined: state == "quarantined"}
	if arrivalSequence < 1 || len(digest) != len(frame.EnvelopeDigest) ||
		(state != "staged" && state != "quarantined") {
		return OpaqueSyncFrame{}, syncProblem(SyncErrorStore, "", "staged inbox is inconsistent")
	}
	copy(frame.EnvelopeDigest[:], digest)
	switch frameKind {
	case terminalCandidateFrameKindSealedV1:
		frame.SealedEnvelope = append([]byte(nil), frameBytes...)
	case terminalCandidateFrameKindPrunedV1:
		frame.PrunedArrival = append([]byte(nil), frameBytes...)
	default:
		return OpaqueSyncFrame{}, syncProblem(SyncErrorStore, "", "staged inbox is inconsistent")
	}
	if _, _, err := opaqueSyncFrameStorageV1(frame); err != nil {
		return OpaqueSyncFrame{}, syncProblem(SyncErrorStore, "", "staged inbox is inconsistent")
	}
	return frame, nil
}

func readSyncProgressV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (SyncProgress, bool, error) {
	progress := SyncProgress{ProjectID: projectID}
	var channelID []byte
	err := tx.QueryRowContext(ctx, `
SELECT channel_id, activation_state, downloaded_cursor, applied_cursor, relay_head
FROM continuity_sync_projects
WHERE project_id = ?`, string(projectID)).Scan(
		&channelID,
		&progress.ActivationState,
		&progress.DownloadedCursor,
		&progress.AppliedCursor,
		&progress.RelayHead,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncProgress{}, false, nil
	}
	if err != nil {
		return SyncProgress{}, false, syncTransactionProblem(ctx)
	}
	if len(channelID) != len(progress.ChannelID) {
		return SyncProgress{}, false, syncProblem(SyncErrorStore, "channel_id", "retained channel identity is corrupt")
	}
	copy(progress.ChannelID[:], channelID)
	if progress.ChannelID == (SyncChannelID{}) {
		return SyncProgress{}, false, syncProblem(SyncErrorStore, "channel_id", "retained channel identity is corrupt")
	}
	if progress.ActivationState != SyncActivationStaging && progress.ActivationState != SyncActivationAttached {
		return SyncProgress{}, false, syncProblem(SyncErrorStore, "", "sync activation state is corrupt")
	}
	return progress, true, nil
}

func loadEnvironmentFrontiersV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (map[continuity.EnvironmentID]environmentFrontierV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT environment_id, highest_sequence, hlc_wall_millis, hlc_logical
FROM continuity_sync_environment_heads
WHERE project_id = ?`, string(projectID))
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	frontiers := make(map[continuity.EnvironmentID]environmentFrontierV1)
	for rows.Next() {
		var environmentID continuity.EnvironmentID
		var frontier environmentFrontierV1
		var logical int64
		if err := rows.Scan(&environmentID, &frontier.sequence, &frontier.clock.WallMillis, &logical); err != nil {
			rows.Close()
			return nil, syncTransactionProblem(ctx)
		}
		if logical < 0 || logical > math.MaxInt32 {
			rows.Close()
			return nil, syncProblem(SyncErrorStore, "", "environment head is corrupt")
		}
		frontier.clock.Logical = int32(logical)
		frontiers[environmentID] = frontier
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return frontiers, nil
}

func loadEnvelopeInventoryV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (*envelopeInventoryV1, error) {
	inventory := &envelopeInventoryV1{
		bySequence:        make(map[string]envelopeInventoryEntryV1),
		certificateByEnv:  make(map[continuity.EnvironmentID][32]byte),
		byGenerationNonce: make(map[string]envelopeInventoryEntryV1),
	}
	rows, err := tx.QueryContext(ctx, `
SELECT
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_receipts
WHERE project_id = ?
UNION ALL
SELECT
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_outbox
WHERE project_id = ?
UNION ALL
SELECT
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_tombstones
WHERE project_id = ?`, string(projectID), string(projectID), string(projectID))
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	for rows.Next() {
		entry, err := scanEnvelopeInventoryEntryV1(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if err := inventory.addPersisted(entry); err != nil {
			rows.Close()
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}

	headRows, err := tx.QueryContext(ctx, `
SELECT
  environment_id,
  sealed_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_environment_heads
WHERE project_id = ? AND sealed_sequence > 0`, string(projectID))
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	for headRows.Next() {
		entry, err := scanEnvelopeInventoryEntryV1(headRows)
		if err != nil {
			headRows.Close()
			return nil, err
		}
		stored, ok := inventory.bySequence[environmentSequenceKeyV1(entry.environmentID, entry.environmentSequence)]
		if !ok || !sealedMetadataEqualV1(stored.metadata, entry.metadata) {
			headRows.Close()
			return nil, syncProblem(SyncErrorStore, "", "sealed environment head has no exact retained metadata")
		}
	}
	if err := headRows.Err(); err != nil {
		headRows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := headRows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return inventory, nil
}

func scanEnvelopeInventoryEntryV1(scanner interface {
	Scan(dest ...any) error
}) (envelopeInventoryEntryV1, error) {
	var entry envelopeInventoryEntryV1
	var previousDigest, digest, certificateID, nonce []byte
	var keyGeneration int64
	if err := scanner.Scan(
		&entry.environmentID,
		&entry.environmentSequence,
		&previousDigest,
		&digest,
		&certificateID,
		&keyGeneration,
		&nonce,
	); err != nil {
		return envelopeInventoryEntryV1{}, syncProblem(SyncErrorStore, "", "cannot read sealed envelope metadata")
	}
	if len(previousDigest) != len(entry.metadata.previousDigest) ||
		len(digest) != len(entry.metadata.digest) ||
		len(certificateID) != len(entry.metadata.certificateID) ||
		len(nonce) != len(entry.metadata.nonce) ||
		keyGeneration < 1 || keyGeneration > math.MaxUint32 {
		return envelopeInventoryEntryV1{}, syncProblem(SyncErrorStore, "", "sealed envelope metadata is corrupt")
	}
	copy(entry.metadata.previousDigest[:], previousDigest)
	copy(entry.metadata.digest[:], digest)
	copy(entry.metadata.certificateID[:], certificateID)
	entry.metadata.keyGeneration = uint32(keyGeneration)
	copy(entry.metadata.nonce[:], nonce)
	if err := validateSealedMetadataV1(entry.environmentSequence, entry.metadata); err != nil {
		return envelopeInventoryEntryV1{}, syncProblem(SyncErrorStore, "", "sealed envelope metadata is corrupt")
	}
	return entry, nil
}

func (inventory *envelopeInventoryV1) addPersisted(entry envelopeInventoryEntryV1) error {
	sequenceKey := environmentSequenceKeyV1(entry.environmentID, entry.environmentSequence)
	if current, ok := inventory.bySequence[sequenceKey]; ok {
		if !sealedMetadataEqualV1(current.metadata, entry.metadata) {
			return syncProblem(SyncErrorStore, "", "persisted envelope sequence metadata conflicts")
		}
		return nil
	}
	if certificateID, ok := inventory.certificateByEnv[entry.environmentID]; ok && certificateID != entry.metadata.certificateID {
		return syncProblem(SyncErrorStore, "", "persisted environment certificate changed")
	}
	nonceKey := generationNonceKeyV1(entry.metadata.keyGeneration, entry.metadata.nonce)
	if owner, ok := inventory.byGenerationNonce[nonceKey]; ok &&
		(owner.environmentID != entry.environmentID || owner.environmentSequence != entry.environmentSequence) {
		return syncProblem(SyncErrorStore, "", "persisted generation nonce is reused")
	}
	inventory.bySequence[sequenceKey] = entry
	inventory.certificateByEnv[entry.environmentID] = entry.metadata.certificateID
	inventory.byGenerationNonce[nonceKey] = entry
	return nil
}

func (inventory *envelopeInventoryV1) admit(environmentID continuity.EnvironmentID, sequence int64, metadata sealedEnvelopeMetadataV1) error {
	entry := envelopeInventoryEntryV1{
		environmentID:       environmentID,
		environmentSequence: sequence,
		metadata:            metadata,
	}
	sequenceKey := environmentSequenceKeyV1(environmentID, sequence)
	if current, ok := inventory.bySequence[sequenceKey]; ok {
		if !sealedMetadataEqualV1(current.metadata, metadata) {
			return syncProblem(SyncErrorConflict, "envelope_metadata", "conflicts with the retained source sequence")
		}
		return nil
	}
	if certificateID, ok := inventory.certificateByEnv[environmentID]; ok && certificateID != metadata.certificateID {
		return syncProblem(SyncErrorCertificate, "certificate_id", "changed for a mint-once environment")
	}
	if sequence > 1 {
		previous, ok := inventory.bySequence[environmentSequenceKeyV1(environmentID, sequence-1)]
		if !ok || previous.metadata.digest != metadata.previousDigest {
			return syncProblem(SyncErrorEnvelopeChain, "previous_envelope_digest", "does not extend the retained envelope chain")
		}
	}
	nonceKey := generationNonceKeyV1(metadata.keyGeneration, metadata.nonce)
	if owner, ok := inventory.byGenerationNonce[nonceKey]; ok &&
		(owner.environmentID != environmentID || owner.environmentSequence != sequence) {
		return syncProblem(SyncErrorNonceReuse, "nonce", "was already used by this project and key generation")
	}
	inventory.bySequence[sequenceKey] = entry
	inventory.certificateByEnv[environmentID] = metadata.certificateID
	inventory.byGenerationNonce[nonceKey] = entry
	return nil
}

func loadTombstonesV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (map[continuity.FactID]tombstoneV1, map[string]tombstoneV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  fact_id,
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_tombstones
WHERE project_id = ?`, string(projectID))
	if err != nil {
		return nil, nil, syncTransactionProblem(ctx)
	}
	byFact := make(map[continuity.FactID]tombstoneV1)
	bySequence := make(map[string]tombstoneV1)
	for rows.Next() {
		var tombstone tombstoneV1
		var previousDigest, digest, certificateID, nonce []byte
		var keyGeneration int64
		if err := rows.Scan(
			&tombstone.factID,
			&tombstone.environmentID,
			&tombstone.environmentSequence,
			&previousDigest,
			&digest,
			&certificateID,
			&keyGeneration,
			&nonce,
		); err != nil {
			rows.Close()
			return nil, nil, syncTransactionProblem(ctx)
		}
		if len(previousDigest) != len(tombstone.previousDigest) ||
			len(digest) != len(tombstone.digest) ||
			len(certificateID) != len(tombstone.certificateID) ||
			len(nonce) != len(tombstone.nonce) ||
			keyGeneration < 1 || keyGeneration > math.MaxUint32 {
			rows.Close()
			return nil, nil, syncProblem(SyncErrorStore, "", "tombstone envelope metadata is corrupt")
		}
		copy(tombstone.previousDigest[:], previousDigest)
		copy(tombstone.digest[:], digest)
		copy(tombstone.certificateID[:], certificateID)
		tombstone.keyGeneration = uint32(keyGeneration)
		copy(tombstone.nonce[:], nonce)
		byFact[tombstone.factID] = tombstone
		bySequence[environmentSequenceKeyV1(tombstone.environmentID, tombstone.environmentSequence)] = tombstone
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, syncTransactionProblem(ctx)
	}
	return byFact, bySequence, nil
}

func rejectConsumedReceiptsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frames []preparedVerifiedSyncFrame) error {
	for _, frame := range frames {
		var count int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM continuity_sync_receipts
WHERE project_id = ?
  AND (
    arrival_sequence = ?
    OR fact_id = ?
    OR (environment_id = ? AND environment_sequence = ?)
  )`,
			string(projectID),
			frame.arrival,
			string(frame.fact.factID),
			string(frame.fact.environmentID),
			frame.fact.environmentSequence,
		).Scan(&count); err != nil {
			return syncTransactionProblem(ctx)
		}
		if count != 0 {
			return syncProblem(SyncErrorConflict, "receipt", "arrival or immutable identity was already consumed")
		}
	}
	return nil
}

func consumedEnvelopeRetryV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, fact storedFactV1, metadata sealedEnvelopeMetadataV1) (bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  fact_id,
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_receipts
WHERE project_id = ?
  AND (
    fact_id = ?
    OR (environment_id = ? AND environment_sequence = ?)
    OR envelope_digest = ?
  )
UNION ALL
SELECT
  fact_id,
  environment_id,
  environment_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_tombstones
WHERE project_id = ?
  AND (
    fact_id = ?
    OR (environment_id = ? AND environment_sequence = ?)
    OR envelope_digest = ?
  )`,
		string(projectID),
		string(fact.factID),
		string(fact.environmentID),
		fact.environmentSequence,
		metadata.digest[:],
		string(projectID),
		string(fact.factID),
		string(fact.environmentID),
		fact.environmentSequence,
		metadata.digest[:],
	)
	if err != nil {
		return false, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	matched := false
	for rows.Next() {
		var factID continuity.FactID
		var environmentID continuity.EnvironmentID
		var environmentSequence int64
		var previousDigest, digest, certificateID, nonce []byte
		var keyGeneration int64
		if err := rows.Scan(
			&factID,
			&environmentID,
			&environmentSequence,
			&previousDigest,
			&digest,
			&certificateID,
			&keyGeneration,
			&nonce,
		); err != nil {
			return false, syncTransactionProblem(ctx)
		}
		retained, err := sealedMetadataFromColumnsV1(
			previousDigest,
			digest,
			certificateID,
			sql.NullInt64{Int64: keyGeneration, Valid: true},
			nonce,
		)
		if err != nil {
			return false, syncProblem(SyncErrorStore, "", "consumed envelope metadata is corrupt")
		}
		if factID != fact.factID ||
			environmentID != fact.environmentID ||
			environmentSequence != fact.environmentSequence ||
			!sealedMetadataEqualV1(retained, metadata) {
			return false, syncProblem(SyncErrorConflict, "envelope_metadata", "conflicts with an already consumed envelope")
		}
		matched = true
	}
	if err := rows.Err(); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return matched, nil
}

func readSealedOutboxV1(ctx context.Context, tx *sql.Tx, factID continuity.FactID) (SealedOutboxFrame, bool, error) {
	row := tx.QueryRowContext(ctx, `
SELECT
  fact_id,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce,
  sealed_envelope
FROM continuity_sync_outbox
WHERE fact_id = ?`, string(factID))
	frame, err := scanSealedOutboxV1(row)
	if errors.Is(err, sql.ErrNoRows) {
		return SealedOutboxFrame{}, false, nil
	}
	if err != nil {
		return SealedOutboxFrame{}, false, err
	}
	return frame, true, nil
}

func scanSealedOutboxV1(scanner interface {
	Scan(dest ...any) error
}) (SealedOutboxFrame, error) {
	var frame SealedOutboxFrame
	var previousDigest, digest, certificateID, nonce []byte
	var keyGeneration int64
	if err := scanner.Scan(
		&frame.FactID,
		&previousDigest,
		&digest,
		&certificateID,
		&keyGeneration,
		&nonce,
		&frame.SealedEnvelope,
	); err != nil {
		return SealedOutboxFrame{}, err
	}
	if len(previousDigest) != len(frame.PreviousEnvelopeDigest) ||
		len(digest) != len(frame.EnvelopeDigest) ||
		len(certificateID) != len(frame.CertificateID) ||
		len(nonce) != len(frame.Nonce) ||
		keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
		len(frame.SealedEnvelope) < 1 || len(frame.SealedEnvelope) > maximumSealedEnvelopeBytes {
		return SealedOutboxFrame{}, syncProblem(SyncErrorStore, "", "sealed outbox row is corrupt")
	}
	copy(frame.PreviousEnvelopeDigest[:], previousDigest)
	copy(frame.EnvelopeDigest[:], digest)
	copy(frame.CertificateID[:], certificateID)
	frame.KeyGeneration = uint32(keyGeneration)
	copy(frame.Nonce[:], nonce)
	frame.SealedEnvelope = append([]byte(nil), frame.SealedEnvelope...)
	return frame, nil
}

func recordSealedEnvironmentHeadV1(ctx context.Context, tx *sql.Tx, fact storedFactV1, metadata sealedEnvelopeMetadataV1) error {
	var highestSequence, sealedSequence int64
	var previousDigest, digest, certificateID, nonce []byte
	var keyGeneration sql.NullInt64
	err := tx.QueryRowContext(ctx, `
SELECT
  highest_sequence,
  sealed_sequence,
  previous_envelope_digest,
  envelope_digest,
  certificate_id,
  key_generation,
  nonce
FROM continuity_sync_environment_heads
WHERE project_id = ? AND environment_id = ?`,
		string(fact.projectID),
		string(fact.environmentID),
	).Scan(
		&highestSequence,
		&sealedSequence,
		&previousDigest,
		&digest,
		&certificateID,
		&keyGeneration,
		&nonce,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if fact.environmentSequence > highestSequence {
		return syncProblem(SyncErrorStore, "", "sealed envelope outruns the fact environment head")
	}
	if fact.environmentSequence <= sealedSequence {
		if fact.environmentSequence == sealedSequence {
			current, err := sealedMetadataFromColumnsV1(previousDigest, digest, certificateID, keyGeneration, nonce)
			if err != nil {
				return err
			}
			if !sealedMetadataEqualV1(current, metadata) {
				return syncProblem(SyncErrorConflict, "envelope_metadata", "does not match the sealed environment head")
			}
		}
		return nil
	}
	if sealedSequence == math.MaxInt64 || fact.environmentSequence != sealedSequence+1 {
		return syncProblem(SyncErrorEnvelopeChain, "environment_sequence", "does not extend the sealed environment prefix")
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_environment_heads
SET
  sealed_sequence = ?,
  previous_envelope_digest = ?,
  envelope_digest = ?,
  certificate_id = ?,
  key_generation = ?,
  nonce = ?
WHERE project_id = ? AND environment_id = ?`,
		fact.environmentSequence,
		metadata.previousDigest[:],
		metadata.digest[:],
		metadata.certificateID[:],
		metadata.keyGeneration,
		metadata.nonce[:],
		string(fact.projectID),
		string(fact.environmentID),
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}

func sealedMetadataFromColumnsV1(previousDigest, digest, certificateID []byte, keyGeneration sql.NullInt64, nonce []byte) (sealedEnvelopeMetadataV1, error) {
	var metadata sealedEnvelopeMetadataV1
	if len(previousDigest) != len(metadata.previousDigest) ||
		len(digest) != len(metadata.digest) ||
		len(certificateID) != len(metadata.certificateID) ||
		len(nonce) != len(metadata.nonce) ||
		!keyGeneration.Valid || keyGeneration.Int64 < 1 || keyGeneration.Int64 > math.MaxUint32 {
		return sealedEnvelopeMetadataV1{}, syncProblem(SyncErrorStore, "", "sealed environment head metadata is corrupt")
	}
	copy(metadata.previousDigest[:], previousDigest)
	copy(metadata.digest[:], digest)
	copy(metadata.certificateID[:], certificateID)
	metadata.keyGeneration = uint32(keyGeneration.Int64)
	copy(metadata.nonce[:], nonce)
	return metadata, nil
}

func advanceEnvironmentHeadV1(ctx context.Context, tx *sql.Tx, fact storedFactV1) error {
	var sequence, wall, logical int64
	err := tx.QueryRowContext(ctx, `
SELECT highest_sequence, hlc_wall_millis, hlc_logical
FROM continuity_sync_environment_heads
WHERE project_id = ? AND environment_id = ?`,
		string(fact.projectID),
		string(fact.environmentID),
	).Scan(&sequence, &wall, &logical)
	if errors.Is(err, sql.ErrNoRows) {
		if fact.environmentSequence != 1 {
			return syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "does not begin at one")
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_environment_heads(
  project_id, environment_id, highest_sequence, hlc_wall_millis, hlc_logical,
  sealed_sequence
) VALUES(?, ?, ?, ?, ?, 0)`,
			string(fact.projectID),
			string(fact.environmentID),
			fact.environmentSequence,
			fact.clock.WallMillis,
			fact.clock.Logical,
		)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if sequence == math.MaxInt64 || fact.environmentSequence != sequence+1 {
		return syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "does not extend the persisted environment head")
	}
	previous := continuity.HybridTime{WallMillis: wall, Logical: int32(logical)}
	if logical < 0 || logical > math.MaxInt32 || !hybridTimeLessV1(previous, fact.clock) {
		return syncProblem(SyncErrorHLC, "hlc", "does not increase with environment sequence")
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_environment_heads
SET highest_sequence = ?, hlc_wall_millis = ?, hlc_logical = ?
WHERE project_id = ? AND environment_id = ?`,
		fact.environmentSequence,
		fact.clock.WallMillis,
		fact.clock.Logical,
		string(fact.projectID),
		string(fact.environmentID),
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}

func storedFactWireV1(fact storedFactV1) continuitywire.Fact {
	return continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              fact.factID,
		ProjectID:           fact.projectID,
		SubjectKind:         fact.subject.Kind,
		SubjectID:           fact.subject.ID,
		FactKind:            fact.kind,
		PayloadVersion:      uint16(fact.payloadVersion),
		CanonicalPayload:    append([]byte(nil), []byte(fact.content)...),
		EnvironmentID:       fact.environmentID,
		EnvironmentSequence: fact.environmentSequence,
		HLCWallMillis:       fact.clock.WallMillis,
		HLCLogical:          fact.clock.Logical,
		EnvelopeVersion:     uint16(fact.envelopeVersion),
	}
}

func storedFactsEqualV1(left, right storedFactV1) bool {
	return continuitywire.Equal(storedFactWireV1(left), storedFactWireV1(right))
}

func hybridTimeLessV1(left, right continuity.HybridTime) bool {
	return left.WallMillis < right.WallMillis ||
		(left.WallMillis == right.WallMillis && left.Logical < right.Logical)
}

func futureSkewedV1(wall, trustedNow, maximumSkew int64) bool {
	return wall > trustedNow && wall-trustedNow > maximumSkew
}

func validateSealedMetadataV1(environmentSequence int64, metadata sealedEnvelopeMetadataV1) error {
	if metadata.keyGeneration == 0 {
		return syncProblem(SyncErrorInvalid, "key_generation", "must begin at one")
	}
	if metadata.digest == [32]byte{} {
		return syncProblem(SyncErrorInvalid, "envelope_digest", "must not be zero")
	}
	if metadata.certificateID == [32]byte{} {
		return syncProblem(SyncErrorInvalid, "certificate_id", "must not be zero")
	}
	previousIsZero := metadata.previousDigest == [32]byte{}
	if environmentSequence == 1 && !previousIsZero {
		return syncProblem(SyncErrorEnvelopeChain, "previous_envelope_digest", "must be zero for the first environment sequence")
	}
	if environmentSequence > 1 && previousIsZero {
		return syncProblem(SyncErrorEnvelopeChain, "previous_envelope_digest", "must be nonzero after the first environment sequence")
	}
	return nil
}

func sealedMetadataEqualV1(left, right sealedEnvelopeMetadataV1) bool {
	return left.previousDigest == right.previousDigest &&
		left.digest == right.digest &&
		left.certificateID == right.certificateID &&
		left.keyGeneration == right.keyGeneration &&
		left.nonce == right.nonce
}

func generationNonceKeyV1(generation uint32, nonce [24]byte) string {
	return fmt.Sprintf("%d\x00%s", generation, string(nonce[:]))
}

func environmentSequenceKeyV1(environmentID continuity.EnvironmentID, sequence int64) string {
	return fmt.Sprintf("%s\x00%d", environmentID, sequence)
}

func requireOneAffectedV1(result sql.Result, ctx context.Context) error {
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func syncProblem(code SyncErrorCode, field, detail string) error {
	return &SyncError{Code: code, Field: field, Detail: detail}
}

func syncTransactionProblem(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return syncProblem(SyncErrorStore, "", "database operation failed")
}
