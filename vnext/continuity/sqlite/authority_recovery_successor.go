package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

// SyncAuthorityRecoveryTransitionStart is the sensitive-value-free exact intent used
// to begin or resume one post-registration recovery authority transition.
// PredecessorCheckpoint is zero only for target membership generation one.
type SyncAuthorityRecoveryTransitionStart struct {
	WriterEnvironmentID        continuity.EnvironmentID
	WriterCertificateID        [32]byte
	TargetMembershipGeneration uint32
	PredecessorCheckpoint      SyncAuthorityCandidateCheckpoint
	SuccessorSnapshot          SyncAuthoritySnapshot
}

// SyncAuthorityRecoveryState is the fixed-size current transition and its
// authenticated recovery-successor checkpoint. Complete candidate-stream
// audits are reserved for public reads, READY completion and replay,
// replacement, abort, and promotion; the retained predecessor is identified by
// Transition and is never copied into the result.
type SyncAuthorityRecoveryState struct {
	Transition SyncAuthorityRecoveryTransition
	Successor  SyncAuthorityCandidate
}

// BeginSyncAuthorityRecoveryTransition atomically retains the exact READY
// predecessor, stages the first bounded successor page, binds the registered
// local writer, and advances the durable relay watermark. An exact retry
// returns the current successor checkpoint even after later pages were added.
func (store *Store) BeginSyncAuthorityRecoveryTransition(
	ctx context.Context,
	projectID continuity.ProjectID,
	start SyncAuthorityRecoveryTransitionStart,
	firstPage SyncAuthorityPage,
) (SyncAuthorityRecoveryState, error) {
	if err := validateSyncAuthorityRecoveryTransitionStartV1(projectID, start); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	prepared, err := prepareSyncAuthorityCandidatePageV2(projectID, start.SuccessorSnapshot, firstPage)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if prepared.AfterEnvironmentID != "" {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "after_environment_id", "must be empty for the first recovery-successor page")
	}
	successorID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, start.SuccessorSnapshot)
	if err != nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "successor_snapshot", "cannot be encoded by the authority candidate codec")
	}
	if store == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if start.WriterEnvironmentID != store.environmentID {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "writer_environment_id", "does not identify the local store writer")
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncAuthorityRecoveryState{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	current, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if found {
		if !syncAuthorityRecoveryStateMatchesStartV1(current, start, successorID) {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "recovery_transition", "does not match the active recovery transition")
		}
		replayed, exact, err := exactSyncAuthorityCandidatePageReplayV2(ctx, tx, current.successor, prepared)
		if err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		if !replayed || !exact {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "first_page", "does not exactly match the retained recovery-successor prefix")
		}
		if err := tx.Commit(); err != nil {
			return SyncAuthorityRecoveryState{}, syncTransactionProblem(ctx)
		}
		return current.value, nil
	}

	var predecessor *persistedSyncAuthorityCandidateV2
	canonicalBase, err := readCanonicalSyncAuthorityBaseV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if start.TargetMembershipGeneration == 1 {
		if canonicalBase.found {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "predecessor_checkpoint", "generation-one recovery requires no canonical authority")
		}
		candidateExists, err := anySyncAuthorityCandidateExistsV2(ctx, tx, projectID)
		if err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		if candidateExists {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "predecessor_checkpoint", "generation-one recovery requires no authority candidate")
		}
		if err := validateSyncAuthorityPromotionBootstrapAbsenceV2(ctx, tx, projectID, successorID); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
	} else {
		persistedPredecessor, predecessorFound, err := readAndValidateActiveSyncAuthorityCandidateV2(ctx, tx, projectID)
		if err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		if !predecessorFound || !persistedPredecessor.candidate.Ready ||
			persistedPredecessor.candidate.Checkpoint() != start.PredecessorCheckpoint {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "predecessor_checkpoint", "does not match the active ready authority candidate")
		}
		if err := validateSyncAuthorityCandidateBaseV2(
			persistedPredecessor.candidate.Snapshot, canonicalBase.digestVersion, canonicalBase.digest, canonicalBase.found,
		); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		if err := validateSyncAuthorityCandidateHeaderAgainstCanonicalV2(persistedPredecessor.candidate.Snapshot, canonicalBase); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		if err := validateReadySyncAuthorityCandidateAgainstCanonicalV2(ctx, tx, persistedPredecessor, canonicalBase); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		predecessorCopy := persistedPredecessor
		predecessor = &predecessorCopy
	}
	if err := validateSyncAuthorityRecoverySuccessorBaseV1(start, predecessor); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := advanceSyncAuthorityRecoveryWatermarkV1(ctx, tx, projectID, start.SuccessorSnapshot); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if predecessor != nil {
		if err := changeSyncAuthorityRecoveryPredecessorRoleV1(
			ctx, tx, projectID, start.PredecessorCheckpoint,
			syncAuthorityCandidateRoleOrdinaryV1, syncAuthorityCandidateRoleRecoveryPredecessorV1,
		); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
	}
	if err := insertFirstSyncAuthorityCandidatePageV2(
		ctx, tx, projectID, successorID, start.SuccessorSnapshot, prepared, headerDigest,
	); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := changeInsertedSyncAuthorityRecoverySuccessorRoleV1(ctx, tx, projectID, successorID); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	attemptID, err := generateSyncAuthorityRecoveryAttemptIDV1()
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	transition := syncAuthorityRecoveryTransitionFromStartV1(projectID, start, successorID, attemptID)
	if err := insertSyncAuthorityRecoveryTransitionV1(ctx, tx, transition); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !prepared.More {
		provisional := persistedSyncAuthorityRecoveryStateV1{value: SyncAuthorityRecoveryState{Transition: transition}}
		if err := validateReadySyncAuthorityRecoveryWriterV1(ctx, tx, provisional, false); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
	}
	next, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !found || next.value.Transition != transition || next.value.Successor.CandidateID != successorID {
		return SyncAuthorityRecoveryState{}, corruptSyncAuthorityRecoveryTransitionV1("recovery transition disappeared during begin")
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "recovery transition begin outcome is unknown; retry the exact start and first page")
	}
	return next.value, nil
}

// AppendVerifiedSyncAuthorityRecoverySuccessorPage appends or exactly replays
// one bounded page under an exact transition, prior successor checkpoint, and
// immutable successor snapshot. Exact replay is recognized before a stale
// checkpoint or a newer relay watermark is rejected.
func (store *Store) AppendVerifiedSyncAuthorityRecoverySuccessorPage(
	ctx context.Context,
	projectID continuity.ProjectID,
	expectedTransition SyncAuthorityRecoveryTransition,
	expectedSuccessor SyncAuthorityCandidateCheckpoint,
	snapshot SyncAuthoritySnapshot,
	page SyncAuthorityPage,
) (SyncAuthorityRecoveryState, error) {
	if err := validateSyncAuthorityRecoveryTransitionValueV1(projectID, expectedTransition); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := validateSyncAuthorityRecoverySuccessorCheckpointV1(expectedTransition, expectedSuccessor); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	prepared, err := prepareSyncAuthorityCandidatePageV2(projectID, snapshot, page)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if store == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncAuthorityRecoveryState{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	current, found, err := readAndValidateSyncAuthorityRecoveryAppendStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !found || current.value.Transition != expectedTransition || current.value.Successor.Snapshot != snapshot {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "recovery_transition", "does not match the active recovery successor")
	}
	replayed, exact, err := exactSyncAuthorityRecoverySuccessorPageReplayV1(
		ctx, tx, current.successor, expectedSuccessor, prepared,
	)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if replayed {
		if !exact {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "page", "changes an already staged recovery-successor page")
		}
		if current.value.Successor.Ready {
			current, found, err = readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
			if err != nil {
				return SyncAuthorityRecoveryState{}, err
			}
			if !found || current.value.Transition != expectedTransition {
				return SyncAuthorityRecoveryState{}, corruptSyncAuthorityRecoveryTransitionV1("ready recovery successor disappeared during replay")
			}
		}
		if err := tx.Commit(); err != nil {
			return SyncAuthorityRecoveryState{}, syncTransactionProblem(ctx)
		}
		return current.value, nil
	}
	if current.value.Successor.Checkpoint() != expectedSuccessor {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "successor_checkpoint", "does not match the active recovery successor")
	}
	if current.value.Successor.Ready {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "page", "ready recovery successor is immutable")
	}
	if prepared.AfterEnvironmentID != current.value.Successor.ThroughEnvironmentID {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "after_environment_id", "is not the recovery-successor cursor")
	}
	if prepared.More {
		err = requireRetainedSyncAuthorityRecoveryWatermarkFloorV1(ctx, tx, projectID, snapshot)
	} else {
		err = requireSyncAuthorityRecoveryWatermarkFloorV1(ctx, tx, projectID, snapshot)
	}
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	expectedPageCount, err := checkedSyncAuthorityCandidateAdvanceV2(current.value.Successor.PageCount, 1)
	if err != nil {
		return SyncAuthorityRecoveryState{}, corruptSyncAuthorityRecoveryTransitionV1("recovery successor page count cannot advance")
	}
	expectedEnvironmentCount, err := checkedSyncAuthorityCandidateAdvanceV2(
		current.value.Successor.EnvironmentCount, int64(len(prepared.Environments)),
	)
	if err != nil {
		return SyncAuthorityRecoveryState{}, corruptSyncAuthorityRecoveryTransitionV1("recovery successor environment count cannot advance")
	}
	if err := appendSyncAuthorityCandidatePageV2(ctx, tx, current.successor, prepared, current.successor.headerDigest); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !prepared.More {
		if err := validateReadySyncAuthorityRecoveryWriterV1(ctx, tx, current, false); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
	}
	var next persistedSyncAuthorityRecoveryStateV1
	if prepared.More {
		next, found, err = readAndValidateSyncAuthorityRecoveryAppendStateV1(ctx, tx, projectID, store.environmentID)
	} else {
		next, found, err = readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	}
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !found || next.value.Transition != expectedTransition ||
		next.value.Successor.CandidateID != current.value.Successor.CandidateID ||
		next.value.Successor.Snapshot != current.value.Successor.Snapshot ||
		next.value.Successor.PageCount != expectedPageCount ||
		next.value.Successor.EnvironmentCount != expectedEnvironmentCount ||
		next.value.Successor.ThroughEnvironmentID != prepared.ThroughEnvironmentID ||
		next.value.Successor.Ready == prepared.More {
		return SyncAuthorityRecoveryState{}, corruptSyncAuthorityRecoveryTransitionV1("recovery successor checkpoint changed unexpectedly during append")
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "recovery-successor page outcome is unknown; retry the exact transition, checkpoint, snapshot, and page")
	}
	return next.value, nil
}

// CurrentSyncAuthorityRecoverySuccessor returns the fixed-size current state
// after fully auditing both candidate streams, their direct-base relation, the
// transition links, and any READY writer proof.
func (store *Store) CurrentSyncAuthorityRecoverySuccessor(
	ctx context.Context,
	projectID continuity.ProjectID,
) (SyncAuthorityRecoveryState, bool, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthorityRecoveryState{}, false, err
	}
	if store == nil {
		return SyncAuthorityRecoveryState{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityRecoveryState{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityRecoveryState{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityRecoveryState{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncAuthorityRecoveryState{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	state, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil || !found {
		return SyncAuthorityRecoveryState{}, found, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityRecoveryState{}, false, syncTransactionProblem(ctx)
	}
	return state.value, true, nil
}

// AbortSyncAuthorityRecoveryTransition removes exactly one STAGING successor
// and restores its retained predecessor to the ordinary candidate slot. READY
// successor evidence is a point of no return and cannot be aborted. The relay
// watermark is never changed.
func (store *Store) AbortSyncAuthorityRecoveryTransition(
	ctx context.Context,
	projectID continuity.ProjectID,
	expectedTransition SyncAuthorityRecoveryTransition,
	expectedSuccessor SyncAuthorityCandidateCheckpoint,
) error {
	if err := validateSyncAuthorityRecoveryTransitionValueV1(projectID, expectedTransition); err != nil {
		return err
	}
	if err := validateSyncAuthorityRecoverySuccessorCheckpointV1(expectedTransition, expectedSuccessor); err != nil {
		return err
	}
	if expectedSuccessor.Ready {
		return syncProblem(SyncErrorConflict, "successor_checkpoint", "ready recovery successor evidence cannot be aborted")
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
	receipt, receiptFound, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
		ctx, tx, projectID, expectedTransition.AttemptID,
	)
	if err != nil {
		return err
	}
	if receiptFound {
		active, err := syncAuthorityRecoveryAttemptIsActiveV1(ctx, tx, projectID, expectedTransition.AttemptID)
		if err != nil {
			return err
		}
		if active {
			return corruptSyncAuthorityRecoveryTransitionV1("attempt is both active and terminal")
		}
		if !syncAuthorityRecoveryTerminalReceiptMatchesInputV1(
			receipt, SyncAuthorityRecoveryAborted, expectedTransition, expectedSuccessor,
		) {
			return syncProblem(SyncErrorConflict, "recovery_transition", "does not match the terminal recovery receipt")
		}
		if err := tx.Commit(); err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	current, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return err
	}
	if !found {
		return syncProblem(SyncErrorConflict, "recovery_transition", "does not match an active or terminal recovery attempt")
	}
	if current.value.Transition != expectedTransition {
		return syncProblem(SyncErrorConflict, "recovery_transition", "does not match the active recovery transition")
	}
	if current.value.Successor.Checkpoint() != expectedSuccessor {
		return syncProblem(SyncErrorConflict, "successor_checkpoint", "does not match the active recovery successor")
	}
	if current.value.Successor.Ready {
		return syncProblem(SyncErrorConflict, "successor_checkpoint", "ready recovery successor evidence cannot be aborted")
	}
	receipt, err = newSyncAuthorityRecoveryTerminalReceiptV1(
		SyncAuthorityRecoveryAborted, expectedTransition, expectedSuccessor,
	)
	if err != nil {
		return err
	}
	if err := insertSyncAuthorityRecoveryTerminalReceiptV1(ctx, tx, receipt); err != nil {
		return err
	}
	if err := deleteSyncAuthorityRecoveryTransitionV1(ctx, tx, expectedTransition); err != nil {
		return err
	}
	if err := deleteSyncAuthorityRecoverySuccessorV1(ctx, tx, projectID, expectedSuccessor); err != nil {
		return err
	}
	if current.predecessor != nil {
		if err := changeSyncAuthorityRecoveryPredecessorRoleV1(
			ctx, tx, projectID, current.predecessor.candidate.Checkpoint(),
			syncAuthorityCandidateRoleRecoveryPredecessorV1, syncAuthorityCandidateRoleOrdinaryV1,
		); err != nil {
			return err
		}
	}
	retainedReceipt, found, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
		ctx, tx, projectID, expectedTransition.AttemptID,
	)
	if err != nil {
		return err
	}
	if !found || !syncAuthorityRecoveryTerminalReceiptMatchesV1(retainedReceipt, receipt) {
		return corruptSyncAuthorityRecoveryTerminalReceiptV1("aborted receipt disappeared during mutation")
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "recovery transition abort outcome is unknown; retry the exact transition and successor checkpoint")
	}
	return nil
}

// ReplaceSyncAuthorityRecoverySuccessor atomically replaces an exact stale
// successor while retaining the READY predecessor. Staleness requires a relay
// watermark strictly above the old snapshot; the replacement snapshot must be
// at or above that floor. Exact desired-state replay is recognized before the
// old checkpoint is considered stale.
func (store *Store) ReplaceSyncAuthorityRecoverySuccessor(
	ctx context.Context,
	projectID continuity.ProjectID,
	expectedTransition SyncAuthorityRecoveryTransition,
	expectedSuccessor SyncAuthorityCandidateCheckpoint,
	snapshot SyncAuthoritySnapshot,
	firstPage SyncAuthorityPage,
) (SyncAuthorityRecoveryState, error) {
	if err := validateSyncAuthorityRecoveryTransitionValueV1(projectID, expectedTransition); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := validateSyncAuthorityRecoverySuccessorCheckpointV1(expectedTransition, expectedSuccessor); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	prepared, err := prepareSyncAuthorityCandidatePageV2(projectID, snapshot, firstPage)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if prepared.AfterEnvironmentID != "" {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "after_environment_id", "must be empty for the first replacement page")
	}
	replacementID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
	if err != nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "successor_snapshot", "cannot be encoded by the authority candidate codec")
	}
	if replacementID == expectedTransition.SuccessorCandidateID {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "successor_snapshot", "does not identify a replacement successor")
	}
	if store == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncAuthorityRecoveryState{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	current, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if found && syncAuthorityRecoveryTransitionIsReplacementV1(current.value.Transition, expectedTransition, replacementID) &&
		current.value.Successor.Snapshot == snapshot {
		replayed, exact, err := exactSyncAuthorityCandidatePageReplayV2(ctx, tx, current.successor, prepared)
		if err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
		if !replayed || !exact {
			return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "first_page", "does not exactly match the replacement successor prefix")
		}
		if err := tx.Commit(); err != nil {
			return SyncAuthorityRecoveryState{}, syncTransactionProblem(ctx)
		}
		return current.value, nil
	}
	if !found || current.value.Transition != expectedTransition {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "recovery_transition", "does not match the active recovery transition")
	}
	if current.value.Successor.Checkpoint() != expectedSuccessor {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "successor_checkpoint", "does not match the stale recovery successor")
	}
	start := SyncAuthorityRecoveryTransitionStart{
		WriterEnvironmentID:        expectedTransition.WriterEnvironmentID,
		WriterCertificateID:        expectedTransition.WriterCertificateID,
		TargetMembershipGeneration: expectedTransition.TargetMembershipGeneration,
		SuccessorSnapshot:          snapshot,
	}
	if current.predecessor != nil {
		start.PredecessorCheckpoint = current.predecessor.candidate.Checkpoint()
	}
	if err := validateSyncAuthorityRecoveryTransitionStartV1(projectID, start); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := validateSyncAuthorityRecoverySuccessorBaseV1(start, current.predecessor); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	floor, err := syncAuthorityRecoveryWatermarkFloorV1(ctx, tx, projectID, snapshot)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if floor.membershipGeneration <= current.value.Successor.Snapshot.MembershipGeneration &&
		floor.relayHead <= current.value.Successor.Snapshot.InventoryArrivalHead {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorConflict, "inventory_arrival_head", "active recovery successor is not stale")
	}
	if snapshot.MembershipGeneration < floor.membershipGeneration {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorCursor, "membership_generation", "regressed below the retained authority watermark")
	}
	if snapshot.InventoryArrivalHead < floor.relayHead {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorCursor, "inventory_arrival_head", "regressed below the retained relay watermark")
	}
	if err := advanceSyncAuthorityRecoveryWatermarkV1(ctx, tx, projectID, snapshot); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := deleteSyncAuthorityRecoveryTransitionV1(ctx, tx, expectedTransition); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := deleteSyncAuthorityRecoverySuccessorV1(ctx, tx, projectID, expectedSuccessor); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := insertFirstSyncAuthorityCandidatePageV2(ctx, tx, projectID, replacementID, snapshot, prepared, headerDigest); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if err := changeInsertedSyncAuthorityRecoverySuccessorRoleV1(ctx, tx, projectID, replacementID); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	replacementTransition := expectedTransition
	replacementTransition.SuccessorCandidateID = replacementID
	if err := insertSyncAuthorityRecoveryTransitionV1(ctx, tx, replacementTransition); err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !prepared.More {
		provisional := persistedSyncAuthorityRecoveryStateV1{value: SyncAuthorityRecoveryState{Transition: replacementTransition}}
		if err := validateReadySyncAuthorityRecoveryWriterV1(ctx, tx, provisional, false); err != nil {
			return SyncAuthorityRecoveryState{}, err
		}
	}
	next, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return SyncAuthorityRecoveryState{}, err
	}
	if !found || next.value.Transition != replacementTransition || next.value.Successor.CandidateID != replacementID {
		return SyncAuthorityRecoveryState{}, corruptSyncAuthorityRecoveryTransitionV1("replacement recovery successor disappeared")
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityRecoveryState{}, syncProblem(SyncErrorStore, "", "recovery-successor replacement outcome is unknown; retry the exact old transition, checkpoint, snapshot, and first page")
	}
	return next.value, nil
}

func validateSyncAuthorityRecoveryTransitionStartV1(projectID continuity.ProjectID, start SyncAuthorityRecoveryTransitionStart) error {
	if err := validateSyncAuthoritySnapshotV2(projectID, start.SuccessorSnapshot); err != nil {
		return err
	}
	if start.WriterEnvironmentID.Validate() != nil {
		return syncProblem(SyncErrorInvalid, "writer_environment_id", "is invalid")
	}
	if start.WriterCertificateID == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, "writer_certificate_id", "must be nonzero")
	}
	if start.TargetMembershipGeneration == 0 || start.SuccessorSnapshot.MembershipGeneration < start.TargetMembershipGeneration {
		return syncProblem(SyncErrorInvalid, "target_membership_generation", "must be nonzero and not exceed successor membership")
	}
	if start.TargetMembershipGeneration == 1 {
		if start.PredecessorCheckpoint != (SyncAuthorityCandidateCheckpoint{}) ||
			start.SuccessorSnapshot.BaseAuthorityDigestVersion != 0 || start.SuccessorSnapshot.BaseAuthorityDigest != ([32]byte{}) {
			return syncProblem(SyncErrorInvalid, "predecessor_checkpoint", "must be absent for generation-one recovery")
		}
		return nil
	}
	if err := validateSyncAuthorityCandidateCheckpointV2(start.PredecessorCheckpoint); err != nil {
		return syncProblem(SyncErrorInvalid, "predecessor_checkpoint", "must identify a ready predecessor")
	}
	if !start.PredecessorCheckpoint.Ready {
		return syncProblem(SyncErrorInvalid, "predecessor_checkpoint", "must identify a ready predecessor")
	}
	if start.SuccessorSnapshot.BaseAuthorityDigestVersion != 2 ||
		start.SuccessorSnapshot.BaseAuthorityDigest != start.PredecessorCheckpoint.AuthorityDigest {
		return syncProblem(SyncErrorConflict, "base_authority_digest", "must identify the exact ready predecessor")
	}
	return nil
}

func validateSyncAuthorityRecoveryTransitionValueV1(projectID continuity.ProjectID, transition SyncAuthorityRecoveryTransition) error {
	if transition.ProjectID != projectID {
		return syncProblem(SyncErrorInvalid, "recovery_transition", "is invalid")
	}
	if err := validateSyncAuthorityRecoveryTransitionV1(transition); err != nil {
		return syncProblem(SyncErrorInvalid, "recovery_transition", "is invalid")
	}
	return nil
}

func validateSyncAuthorityRecoverySuccessorCheckpointV1(
	transition SyncAuthorityRecoveryTransition,
	checkpoint SyncAuthorityCandidateCheckpoint,
) error {
	if err := validateSyncAuthorityCandidateCheckpointV2(checkpoint); err != nil {
		return err
	}
	if checkpoint.CandidateID != transition.SuccessorCandidateID {
		return syncProblem(SyncErrorInvalid, "successor_checkpoint", "does not identify the expected transition successor")
	}
	return nil
}

func validateSyncAuthorityRecoverySuccessorBaseV1(
	start SyncAuthorityRecoveryTransitionStart,
	predecessor *persistedSyncAuthorityCandidateV2,
) error {
	if predecessor == nil {
		if start.TargetMembershipGeneration != 1 || start.PredecessorCheckpoint != (SyncAuthorityCandidateCheckpoint{}) ||
			start.SuccessorSnapshot.BaseAuthorityDigestVersion != 0 || start.SuccessorSnapshot.BaseAuthorityDigest != ([32]byte{}) {
			return syncProblem(SyncErrorConflict, "base_authority_digest", "generation-one recovery must have no predecessor base")
		}
		return nil
	}
	predecessorCandidate := predecessor.candidate
	if predecessorCandidate.Checkpoint() != start.PredecessorCheckpoint || !predecessorCandidate.Ready ||
		predecessorCandidate.Snapshot.MembershipGeneration+1 != start.TargetMembershipGeneration {
		return syncProblem(SyncErrorConflict, "predecessor_checkpoint", "does not identify membership immediately before the recovery target")
	}
	if start.SuccessorSnapshot.ChannelID != predecessorCandidate.Snapshot.ChannelID ||
		start.SuccessorSnapshot.RelayGeneration != predecessorCandidate.Snapshot.RelayGeneration ||
		start.SuccessorSnapshot.AdminPublicKey != predecessorCandidate.Snapshot.AdminPublicKey {
		return syncProblem(SyncErrorConflict, "successor_snapshot", "does not match the predecessor relay identity")
	}
	if start.SuccessorSnapshot.InventoryArrivalHead < predecessorCandidate.Snapshot.InventoryArrivalHead {
		return syncProblem(SyncErrorCursor, "inventory_arrival_head", "regressed below the predecessor observation")
	}
	if start.SuccessorSnapshot.BaseAuthorityDigestVersion != 2 ||
		start.SuccessorSnapshot.BaseAuthorityDigest != predecessorCandidate.AuthorityDigest {
		return syncProblem(SyncErrorConflict, "base_authority_digest", "does not identify the exact ready predecessor")
	}
	return nil
}

func syncAuthorityRecoveryTransitionFromStartV1(
	projectID continuity.ProjectID,
	start SyncAuthorityRecoveryTransitionStart,
	successorID [32]byte,
	attemptID [32]byte,
) SyncAuthorityRecoveryTransition {
	transition := SyncAuthorityRecoveryTransition{
		ProjectID:                  projectID,
		AttemptID:                  attemptID,
		SuccessorCandidateID:       successorID,
		WriterEnvironmentID:        start.WriterEnvironmentID,
		WriterCertificateID:        start.WriterCertificateID,
		TargetMembershipGeneration: start.TargetMembershipGeneration,
	}
	if start.TargetMembershipGeneration > 1 {
		transition.PredecessorCandidateID = start.PredecessorCheckpoint.CandidateID
	}
	return transition
}

func syncAuthorityRecoveryStateMatchesStartV1(
	state persistedSyncAuthorityRecoveryStateV1,
	start SyncAuthorityRecoveryTransitionStart,
	successorID [32]byte,
) bool {
	want := syncAuthorityRecoveryTransitionFromStartV1(
		state.value.Transition.ProjectID, start, successorID, state.value.Transition.AttemptID,
	)
	if state.value.Transition != want || state.value.Successor.Snapshot != start.SuccessorSnapshot {
		return false
	}
	if state.predecessor == nil {
		return start.PredecessorCheckpoint == (SyncAuthorityCandidateCheckpoint{})
	}
	return state.predecessor.candidate.Checkpoint() == start.PredecessorCheckpoint
}

func syncAuthorityRecoveryTransitionIsReplacementV1(
	current SyncAuthorityRecoveryTransition,
	previous SyncAuthorityRecoveryTransition,
	replacementID [32]byte,
) bool {
	want := previous
	want.SuccessorCandidateID = replacementID
	return current == want
}

func syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot) SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            snapshot.ChannelID,
		RelayGeneration:      snapshot.RelayGeneration,
		AdminPublicKey:       snapshot.AdminPublicKey,
		MembershipGeneration: snapshot.MembershipGeneration,
		RelayHead:            snapshot.InventoryArrivalHead,
	}
}

func syncAuthorityRecoveryWatermarkFloorV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) (syncRelayWatermarkRecordV1, error) {
	return auditSyncRelayWatermarkSourcesV1(ctx, tx, syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, snapshot))
}

func requireSyncAuthorityRecoveryWatermarkFloorV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) error {
	floor, err := syncAuthorityRecoveryWatermarkFloorV1(ctx, tx, projectID, snapshot)
	if err != nil {
		return err
	}
	return requireSyncAuthoritySnapshotAtExactWatermarkV1(snapshot, floor)
}

func requireRetainedSyncAuthorityRecoveryWatermarkV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) error {
	if _, err := syncAuthorityRecoveryWatermarkFloorV1(ctx, tx, projectID, snapshot); err != nil {
		return err
	}
	_, err := readAndValidateRetainedSyncAuthorityRecoveryWatermarkV1(ctx, tx, projectID, snapshot)
	return err
}

func requireRetainedSyncAuthorityRecoveryWatermarkFloorV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) error {
	if _, err := readAndValidateRetainedSyncAuthorityRecoveryWatermarkV1(ctx, tx, projectID, snapshot); err != nil {
		return err
	}
	floor, err := auditBoundedSyncRelayWatermarkSourcesV1(
		ctx, tx, syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, snapshot),
	)
	if err != nil {
		return err
	}
	return requireSyncAuthoritySnapshotAtExactWatermarkV1(snapshot, floor)
}

func readAndValidateRetainedSyncAuthorityRecoveryWatermarkV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) (syncRelayWatermarkRecordV1, error) {
	want := syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, snapshot)
	retained, found, err := readSyncRelayWatermarkV1(ctx, tx, syncRelayWatermarkKeyFromValueV1(want))
	if err != nil {
		return syncRelayWatermarkRecordV1{}, err
	}
	if !found {
		return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
	}
	if retained.adminPublicKey != snapshot.AdminPublicKey {
		return syncRelayWatermarkRecordV1{}, syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
	}
	if retained.membershipGeneration < snapshot.MembershipGeneration ||
		retained.relayHead < snapshot.InventoryArrivalHead {
		return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
	}
	return retained, nil
}

func advanceSyncAuthorityRecoveryWatermarkV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) error {
	return advanceVerifiedSyncRelayWatermarkObservationV1(
		ctx, tx, syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, snapshot),
	)
}

func requireSyncAuthoritySnapshotAtExactWatermarkV1(
	snapshot SyncAuthoritySnapshot,
	floor syncRelayWatermarkRecordV1,
) error {
	return requireSyncRelayObservationAtExactWatermarkV1(
		SyncRelayWatermark{
			MembershipGeneration: snapshot.MembershipGeneration,
			RelayHead:            snapshot.InventoryArrivalHead,
		},
		floor,
	)
}

func changeSyncAuthorityRecoveryPredecessorRoleV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	checkpoint SyncAuthorityCandidateCheckpoint,
	fromRole string,
	toRole string,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_authority_candidates
SET role = ?
WHERE project_id = ? AND candidate_id = ? AND role = ? AND state = 'ready'
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ? AND authority_digest = ?`,
		toRole, string(projectID), checkpoint.CandidateID[:], fromRole,
		checkpoint.PageCount, checkpoint.EnvironmentCount, checkpoint.ThroughEnvironmentID,
		checkpoint.RollingEnvironmentDigest[:], checkpoint.AuthorityDigest[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "predecessor_checkpoint", "ready predecessor changed")
	}
	return nil
}

func changeInsertedSyncAuthorityRecoverySuccessorRoleV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_authority_candidates
SET role = 'recovery-successor'
WHERE project_id = ? AND candidate_id = ? AND role = 'ordinary'
  AND state IN ('staging', 'ready')`, string(projectID), candidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "successor_snapshot", "inserted recovery successor changed")
	}
	return nil
}

func insertSyncAuthorityRecoveryTransitionV1(
	ctx context.Context,
	tx *sql.Tx,
	transition SyncAuthorityRecoveryTransition,
) error {
	if err := validateSyncAuthorityRecoveryTransitionV1(transition); err != nil {
		return err
	}
	if _, found, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
		ctx, tx, transition.ProjectID, transition.AttemptID,
	); err != nil {
		return err
	} else if found {
		return syncProblem(SyncErrorConflict, "recovery_transition", "attempt identity is already terminal")
	}
	var predecessor any
	if transition.PredecessorCandidateID != ([32]byte{}) {
		predecessor = transition.PredecessorCandidateID[:]
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authority_recovery_transitions(
  project_id, attempt_id, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation
) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		string(transition.ProjectID), transition.AttemptID[:], predecessor, transition.SuccessorCandidateID[:],
		string(transition.WriterEnvironmentID), transition.WriterCertificateID[:], transition.TargetMembershipGeneration,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}

func deleteSyncAuthorityRecoveryTransitionV1(
	ctx context.Context,
	tx *sql.Tx,
	transition SyncAuthorityRecoveryTransition,
) error {
	var predecessor any
	if transition.PredecessorCandidateID != ([32]byte{}) {
		predecessor = transition.PredecessorCandidateID[:]
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_recovery_transitions
WHERE project_id = ? AND attempt_id = ? AND successor_candidate_id = ?
  AND writer_environment_id = ? AND writer_certificate_id = ?
  AND target_membership_generation = ?
  AND ((? IS NULL AND predecessor_candidate_id IS NULL) OR predecessor_candidate_id = ?)`,
		string(transition.ProjectID), transition.AttemptID[:], transition.SuccessorCandidateID[:],
		string(transition.WriterEnvironmentID), transition.WriterCertificateID[:], transition.TargetMembershipGeneration,
		predecessor, predecessor,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "recovery_transition", "changed during mutation")
	}
	return nil
}

func deleteSyncAuthorityRecoverySuccessorV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	checkpoint SyncAuthorityCandidateCheckpoint,
) error {
	state := "staging"
	var authorityDigest any
	if checkpoint.Ready {
		state = "ready"
		authorityDigest = checkpoint.AuthorityDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ? AND role = 'recovery-successor'
  AND state = ? AND page_count = ? AND environment_count = ?
  AND through_environment_id = ? AND rolling_environment_digest = ?
  AND ((? IS NULL AND authority_digest IS NULL) OR authority_digest = ?)`,
		string(projectID), checkpoint.CandidateID[:], state, checkpoint.PageCount,
		checkpoint.EnvironmentCount, checkpoint.ThroughEnvironmentID,
		checkpoint.RollingEnvironmentDigest[:], authorityDigest, authorityDigest,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "successor_checkpoint", "recovery successor changed")
	}
	return nil
}

func syncAuthorityRecoveryAttemptIsActiveV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	attemptID [32]byte,
) (bool, error) {
	var active int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
	  SELECT 1 FROM continuity_sync_authority_recovery_transitions
	  WHERE project_id = ? AND attempt_id = ?
)`, string(projectID), attemptID[:]).Scan(&active); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return active != 0, nil
}
