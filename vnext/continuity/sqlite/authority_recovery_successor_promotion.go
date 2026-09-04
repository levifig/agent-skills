package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

// PromoteSyncAuthorityRecoverySuccessor atomically installs the exact READY
// recovery successor, removes its private staging graph, and retains one
// attempt-bound terminal receipt. Exact retries return that receipt without
// consulting canonical or other mutable state.
func (store *Store) PromoteSyncAuthorityRecoverySuccessor(
	ctx context.Context,
	projectID continuity.ProjectID,
	expectedTransition SyncAuthorityRecoveryTransition,
	expectedSuccessor SyncAuthorityCandidateCheckpoint,
) (SyncAuthorityRecoveryTerminalReceipt, error) {
	if err := validateSyncAuthorityRecoveryTransitionValueV1(projectID, expectedTransition); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := validateSyncAuthorityRecoverySuccessorCheckpointV1(expectedTransition, expectedSuccessor); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if !expectedSuccessor.Ready {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorInvalid, "successor_checkpoint", "must identify a ready recovery successor")
	}
	if store == nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	receipt, receiptFound, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
		ctx, tx, projectID, expectedTransition.AttemptID,
	)
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if receiptFound {
		active, err := syncAuthorityRecoveryAttemptIsActiveV1(ctx, tx, projectID, expectedTransition.AttemptID)
		if err != nil {
			return SyncAuthorityRecoveryTerminalReceipt{}, err
		}
		if active {
			return SyncAuthorityRecoveryTerminalReceipt{}, corruptSyncAuthorityRecoveryTransitionV1("attempt is both active and terminal")
		}
		if !syncAuthorityRecoveryTerminalReceiptMatchesInputV1(
			receipt, SyncAuthorityRecoveryPromoted, expectedTransition, expectedSuccessor,
		) {
			return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorConflict, "recovery_transition", "does not match the terminal recovery receipt")
		}
		if err := tx.Commit(); err != nil {
			return SyncAuthorityRecoveryTerminalReceipt{}, syncTransactionProblem(ctx)
		}
		return receipt, nil
	}

	current, found, err := readAndValidateSyncAuthorityRecoveryStateV1(ctx, tx, projectID, store.environmentID)
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if !found {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorConflict, "recovery_transition", "does not match an active or terminal recovery attempt")
	}
	if current.value.Transition != expectedTransition {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorConflict, "recovery_transition", "does not match the active recovery transition")
	}
	if current.value.Successor.Checkpoint() != expectedSuccessor || !current.value.Successor.Ready {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorConflict, "successor_checkpoint", "does not match the active ready recovery successor")
	}
	base, err := readCanonicalSyncAuthorityBaseV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	floor, err := syncAuthorityRecoveryWatermarkFloorV1(ctx, tx, projectID, current.successor.candidate.Snapshot)
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := requireSyncAuthoritySnapshotAtExactWatermarkV1(current.successor.candidate.Snapshot, floor); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	watermarkKey := syncRelayWatermarkKeyFromValueV1(
		syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, current.successor.candidate.Snapshot),
	)
	retainedWatermark, watermarkFound, err := readSyncRelayWatermarkV1(ctx, tx, watermarkKey)
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if !watermarkFound {
		return SyncAuthorityRecoveryTerminalReceipt{}, corruptSyncRelayWatermarkProblemV1()
	}
	if !base.found {
		if err := validateSyncAuthorityRecoveryPromotionBootstrapAbsenceV1(ctx, tx, current); err != nil {
			return SyncAuthorityRecoveryTerminalReceipt{}, err
		}
	}
	if err := validateSyncAuthorityPromotionProtectedStateV2(ctx, tx, current.successor); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}

	receipt, err = newSyncAuthorityRecoveryTerminalReceiptV1(
		SyncAuthorityRecoveryPromoted, expectedTransition, expectedSuccessor,
	)
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := insertSyncAuthorityRecoveryTerminalReceiptV1(ctx, tx, receipt); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := changeReadySyncAuthorityRecoverySuccessorRoleForPromotionV1(ctx, tx, projectID, expectedSuccessor); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	ordinaryReceipt := syncAuthorityCandidateReceiptV2(current.successor.candidate)
	if err := applySyncAuthorityPromotionV2(ctx, tx, current.successor, base, ordinaryReceipt); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := deleteSyncAuthorityRecoveryTransitionV1(ctx, tx, expectedTransition); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if current.predecessor != nil {
		if err := deleteReadySyncAuthorityRecoveryPredecessorV1(
			ctx, tx, projectID, current.predecessor.candidate.Checkpoint(),
		); err != nil {
			return SyncAuthorityRecoveryTerminalReceipt{}, err
		}
	}
	if err := deletePromotedSyncAuthorityRecoverySuccessorV1(ctx, tx, projectID, expectedSuccessor); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := validateTerminatedSyncAuthorityRecoveryPromotionV1(
		ctx, tx, current, receipt, retainedWatermark,
	); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, syncProblem(SyncErrorStore, "", "recovery-successor promotion outcome is unknown; retry the exact transition and successor checkpoint")
	}
	return receipt, nil
}

func validateSyncAuthorityRecoveryPromotionBootstrapAbsenceV1(
	ctx context.Context,
	tx *sql.Tx,
	state persistedSyncAuthorityRecoveryStateV1,
) error {
	projectID := string(state.value.Transition.ProjectID)
	predecessorID := state.value.Transition.PredecessorCandidateID[:]
	successorID := state.value.Transition.SuccessorCandidateID[:]
	var orphaned int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_inbox WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_receipts WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_outbox WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_tombstones WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_terminal_candidates WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_terminal_candidate_frames WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidates
  WHERE project_id = ? AND candidate_id IS NOT ? AND candidate_id IS NOT ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidate_pages
  WHERE project_id = ? AND candidate_id IS NOT ? AND candidate_id IS NOT ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidate_environments
  WHERE project_id = ? AND candidate_id IS NOT ? AND candidate_id IS NOT ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidate_membership_events
  WHERE project_id = ? AND candidate_id IS NOT ? AND candidate_id IS NOT ?
)`,
		projectID, projectID, projectID, projectID, projectID, projectID,
		projectID, predecessorID, successorID,
		projectID, predecessorID, successorID,
		projectID, predecessorID, successorID,
		projectID, predecessorID, successorID,
	).Scan(&orphaned); err != nil {
		return syncTransactionProblem(ctx)
	}
	if orphaned != 0 {
		return syncProblem(SyncErrorStore, "sync_authority", "sync state exists without a canonical project")
	}
	return nil
}

func changeReadySyncAuthorityRecoverySuccessorRoleForPromotionV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	checkpoint SyncAuthorityCandidateCheckpoint,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_authority_candidates
SET role = 'ordinary'
WHERE project_id = ? AND candidate_id = ?
  AND role = 'recovery-successor' AND state = 'ready'
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ? AND authority_digest_version = 2
  AND authority_digest = ?`,
		string(projectID), checkpoint.CandidateID[:], checkpoint.PageCount,
		checkpoint.EnvironmentCount, checkpoint.ThroughEnvironmentID,
		checkpoint.RollingEnvironmentDigest[:], checkpoint.AuthorityDigest[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "successor_checkpoint", "ready recovery successor changed during promotion")
	}
	return nil
}

func deleteReadySyncAuthorityRecoveryPredecessorV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	checkpoint SyncAuthorityCandidateCheckpoint,
) error {
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ?
  AND role = 'recovery-predecessor' AND state = 'ready'
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ? AND authority_digest_version = 2
  AND authority_digest = ?`,
		string(projectID), checkpoint.CandidateID[:], checkpoint.PageCount,
		checkpoint.EnvironmentCount, checkpoint.ThroughEnvironmentID,
		checkpoint.RollingEnvironmentDigest[:], checkpoint.AuthorityDigest[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "predecessor_checkpoint", "ready recovery predecessor changed during promotion")
	}
	return nil
}

func deletePromotedSyncAuthorityRecoverySuccessorV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	checkpoint SyncAuthorityCandidateCheckpoint,
) error {
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ?
  AND role = 'ordinary' AND state = 'promoted'
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ? AND authority_digest_version = 2
  AND authority_digest = ?`,
		string(projectID), checkpoint.CandidateID[:], checkpoint.PageCount,
		checkpoint.EnvironmentCount, checkpoint.ThroughEnvironmentID,
		checkpoint.RollingEnvironmentDigest[:], checkpoint.AuthorityDigest[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "successor_checkpoint", "promoted recovery successor changed during cleanup")
	}
	return nil
}

func validateTerminatedSyncAuthorityRecoveryPromotionV1(
	ctx context.Context,
	tx *sql.Tx,
	state persistedSyncAuthorityRecoveryStateV1,
	wantReceipt SyncAuthorityRecoveryTerminalReceipt,
	wantWatermark syncRelayWatermarkRecordV1,
) error {
	transition := state.value.Transition
	retainedReceipt, found, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
		ctx, tx, transition.ProjectID, transition.AttemptID,
	)
	if err != nil {
		return err
	}
	if !found || !syncAuthorityRecoveryTerminalReceiptMatchesV1(retainedReceipt, wantReceipt) {
		return corruptSyncAuthorityRecoveryTerminalReceiptV1("promoted receipt disappeared during mutation")
	}
	var transitionCount, candidateCount, childCount, participantCount int64
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidates
   WHERE project_id = ? AND candidate_id IN (?, ?)),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages
   WHERE project_id = ? AND candidate_id IN (?, ?))
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments
     WHERE project_id = ? AND candidate_id IN (?, ?))
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events
     WHERE project_id = ? AND candidate_id IN (?, ?)),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidates
   WHERE project_id = ? AND role <> 'ordinary')`,
		string(transition.ProjectID),
		string(transition.ProjectID), transition.PredecessorCandidateID[:], transition.SuccessorCandidateID[:],
		string(transition.ProjectID), transition.PredecessorCandidateID[:], transition.SuccessorCandidateID[:],
		string(transition.ProjectID), transition.PredecessorCandidateID[:], transition.SuccessorCandidateID[:],
		string(transition.ProjectID), transition.PredecessorCandidateID[:], transition.SuccessorCandidateID[:],
		string(transition.ProjectID),
	).Scan(&transitionCount, &candidateCount, &childCount, &participantCount); err != nil {
		return syncTransactionProblem(ctx)
	}
	if transitionCount != 0 || candidateCount != 0 || childCount != 0 || participantCount != 0 {
		return corruptSyncAuthorityRecoveryTransitionV1("terminal recovery staging rows survived promotion")
	}
	wantBinding := SyncAuthorityBinding{
		ChannelID:              state.successor.candidate.Snapshot.ChannelID,
		RelayGeneration:        state.successor.candidate.Snapshot.RelayGeneration,
		AdminPublicKey:         state.successor.candidate.Snapshot.AdminPublicKey,
		MembershipGeneration:   state.successor.candidate.Snapshot.MembershipGeneration,
		InventoryArrivalHead:   state.successor.candidate.Snapshot.InventoryArrivalHead,
		AuthorityDigestVersion: state.successor.candidate.AuthorityDigestVersion,
		AuthorityDigest:        state.successor.candidate.AuthorityDigest,
	}
	gotBinding, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, transition.ProjectID)
	if err != nil {
		return err
	}
	if gotBinding != wantBinding {
		return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority binding changed during recovery promotion")
	}
	gotWatermark, found, err := readSyncRelayWatermarkV1(ctx, tx, wantWatermark.key)
	if err != nil {
		return err
	}
	if !found || gotWatermark != wantWatermark {
		return corruptSyncRelayWatermarkProblemV1()
	}
	return nil
}
