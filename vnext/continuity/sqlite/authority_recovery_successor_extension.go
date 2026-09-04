package sqlite

import (
	"context"
	"database/sql"
)

// validateReadySyncAuthorityRecoverySuccessorExtensionV1 proves that a fully
// streamed READY successor is a monotonic inventory and membership-event
// extension of its fully streamed READY predecessor. Each query is an EXISTS
// probe, so validation retains fixed memory regardless of inventory size.
func validateReadySyncAuthorityRecoverySuccessorExtensionV1(
	ctx context.Context,
	tx *sql.Tx,
	state persistedSyncAuthorityRecoveryStateV1,
) error {
	if state.predecessor == nil || !state.predecessor.candidate.Ready || !state.successor.candidate.Ready {
		return corruptSyncAuthorityRecoveryTransitionV1("successor extension proof requires ready predecessor and successor candidates")
	}
	projectID := string(state.value.Transition.ProjectID)
	predecessorID := state.predecessor.candidate.CandidateID[:]
	successorID := state.successor.candidate.CandidateID[:]
	predecessorGeneration := state.predecessor.candidate.Snapshot.MembershipGeneration

	var invalid int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_environments AS predecessor
  LEFT JOIN continuity_sync_authority_candidate_environments AS successor
    ON successor.project_id = predecessor.project_id
   AND successor.candidate_id = ?
   AND successor.environment_id = predecessor.environment_id
  WHERE predecessor.project_id = ? AND predecessor.candidate_id = ?
    AND successor.environment_id IS NULL
)`, successorID, projectID, predecessorID).Scan(&invalid); err != nil {
		return syncTransactionProblem(ctx)
	}
	if invalid != 0 {
		return corruptSyncAuthorityRecoveryTransitionV1("ready successor omits a predecessor environment")
	}

	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_environments AS predecessor
  JOIN continuity_sync_authority_candidate_environments AS successor
    ON successor.project_id = predecessor.project_id
   AND successor.candidate_id = ?
   AND successor.environment_id = predecessor.environment_id
  WHERE predecessor.project_id = ? AND predecessor.candidate_id = ?
    AND NOT (
      predecessor.certificate_id IS successor.certificate_id
      AND predecessor.certificate_bytes IS successor.certificate_bytes
      AND predecessor.mode = successor.mode
      AND predecessor.expires_at_millis = successor.expires_at_millis
      AND predecessor.join_membership_generation = successor.join_membership_generation
    )
)`, successorID, projectID, predecessorID).Scan(&invalid); err != nil {
		return syncTransactionProblem(ctx)
	}
	if invalid != 0 {
		return corruptSyncAuthorityRecoveryTransitionV1("ready successor changes a predecessor environment certificate")
	}

	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_environments AS predecessor
  JOIN continuity_sync_authority_candidate_environments AS successor
    ON successor.project_id = predecessor.project_id
   AND successor.candidate_id = ?
   AND successor.environment_id = predecessor.environment_id
  WHERE predecessor.project_id = ? AND predecessor.candidate_id = ?
    AND NOT (
      (predecessor.retirement_id IS NULL AND successor.retirement_id IS NULL)
      OR (
        predecessor.retirement_id IS NULL
        AND successor.retirement_id IS NOT NULL
        AND successor.retirement_membership_generation > ?
      )
      OR (
        predecessor.retirement_id IS NOT NULL
        AND successor.retirement_id IS NOT NULL
        AND predecessor.retirement_relay_generation IS successor.retirement_relay_generation
        AND predecessor.retirement_membership_generation = successor.retirement_membership_generation
        AND predecessor.retirement_final_environment_sequence = successor.retirement_final_environment_sequence
        AND predecessor.retirement_final_envelope_digest IS successor.retirement_final_envelope_digest
        AND predecessor.retirement_id IS successor.retirement_id
        AND predecessor.retirement_bytes IS successor.retirement_bytes
      )
    )
)`, successorID, projectID, predecessorID, predecessorGeneration).Scan(&invalid); err != nil {
		return syncTransactionProblem(ctx)
	}
	if invalid != 0 {
		return corruptSyncAuthorityRecoveryTransitionV1("ready successor changes a predecessor terminal retirement")
	}

	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_environments AS successor
  LEFT JOIN continuity_sync_authority_candidate_environments AS predecessor
    ON predecessor.project_id = successor.project_id
   AND predecessor.candidate_id = ?
   AND predecessor.environment_id = successor.environment_id
  WHERE successor.project_id = ? AND successor.candidate_id = ?
    AND predecessor.environment_id IS NULL
    AND successor.join_membership_generation <= ?
)`, predecessorID, projectID, successorID, predecessorGeneration).Scan(&invalid); err != nil {
		return syncTransactionProblem(ctx)
	}
	if invalid != 0 {
		return corruptSyncAuthorityRecoveryTransitionV1("ready successor inserts an environment before the recovery target")
	}

	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_membership_events AS predecessor
  LEFT JOIN continuity_sync_authority_candidate_membership_events AS successor
    ON successor.project_id = predecessor.project_id
   AND successor.candidate_id = ?
   AND successor.membership_generation = predecessor.membership_generation
  WHERE predecessor.project_id = ? AND predecessor.candidate_id = ?
    AND predecessor.membership_generation <= ?
    AND (
      successor.membership_generation IS NULL
      OR successor.event_kind <> predecessor.event_kind
      OR successor.environment_id <> predecessor.environment_id
    )
)`, successorID, projectID, predecessorID, predecessorGeneration).Scan(&invalid); err != nil {
		return syncTransactionProblem(ctx)
	}
	if invalid != 0 {
		return corruptSyncAuthorityRecoveryTransitionV1("ready successor changes the predecessor membership-event prefix")
	}
	return nil
}
