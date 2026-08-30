package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	syncAuthorityCandidateRoleOrdinaryV1            = "ordinary"
	syncAuthorityCandidateRoleRecoveryPredecessorV1 = "recovery-predecessor"
	syncAuthorityCandidateRoleRecoverySuccessorV1   = "recovery-successor"
)

// SyncAuthorityRecoveryTransition is the sensitive-value-free, fixed-size recovery
// authority binding retained while a predecessor and successor candidate
// coexist. A zero predecessor candidate identifies the generation-one case.
type SyncAuthorityRecoveryTransition struct {
	ProjectID                  continuity.ProjectID
	PredecessorCandidateID     [32]byte
	SuccessorCandidateID       [32]byte
	WriterEnvironmentID        continuity.EnvironmentID
	WriterCertificateID        [32]byte
	TargetMembershipGeneration uint32
}

type syncAuthorityRecoveryParticipantV1 struct {
	role                 string
	state                string
	membershipGeneration uint32
}

// CurrentSyncAuthorityRecoveryTransition returns the exact transition binding
// after auditing every non-ordinary candidate role for the project.
func (store *Store) CurrentSyncAuthorityRecoveryTransition(
	ctx context.Context,
	projectID continuity.ProjectID,
) (SyncAuthorityRecoveryTransition, bool, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthorityRecoveryTransition{}, false, err
	}
	if store == nil {
		return SyncAuthorityRecoveryTransition{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityRecoveryTransition{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityRecoveryTransition{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityRecoveryTransition{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncAuthorityRecoveryTransition{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	transition, found, err := readAndAuditSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID)
	if err != nil || !found {
		return SyncAuthorityRecoveryTransition{}, found, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityRecoveryTransition{}, false, syncTransactionProblem(ctx)
	}
	return transition, true, nil
}

func requireNoSyncAuthorityRecoveryTransitionV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	_, found, err := readAndAuditSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if found {
		return syncProblem(SyncErrorConflict, "sync_authority_recovery_transition", "requires the dedicated recovery transition API")
	}
	return nil
}

func readAndAuditSyncAuthorityRecoveryTransitionV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
) (SyncAuthorityRecoveryTransition, bool, error) {
	participants, err := readSyncAuthorityRecoveryParticipantsV1(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityRecoveryTransition{}, false, err
	}

	transition := SyncAuthorityRecoveryTransition{ProjectID: projectID}
	var predecessor, successor, writerCertificate []byte
	var targetGeneration int64
	err = tx.QueryRowContext(ctx, `
SELECT predecessor_candidate_id, successor_candidate_id,
       writer_environment_id, writer_certificate_id,
       target_membership_generation
FROM continuity_sync_authority_recovery_transitions
WHERE project_id = ?`, string(projectID)).Scan(
		&predecessor,
		&successor,
		&transition.WriterEnvironmentID,
		&writerCertificate,
		&targetGeneration,
	)
	if errors.Is(err, sql.ErrNoRows) {
		if len(participants) != 0 {
			return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("candidate role exists without a transition link")
		}
		return SyncAuthorityRecoveryTransition{}, false, nil
	}
	if err != nil {
		return SyncAuthorityRecoveryTransition{}, false, syncTransactionProblem(ctx)
	}
	if (predecessor != nil && (len(predecessor) != sha256.Size || isZeroDigestBytesV2(predecessor))) ||
		len(successor) != sha256.Size || isZeroDigestBytesV2(successor) ||
		len(writerCertificate) != sha256.Size || isZeroDigestBytesV2(writerCertificate) ||
		transition.WriterEnvironmentID.Validate() != nil || targetGeneration < 1 || targetGeneration > math.MaxUint32 {
		return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("transition link is malformed")
	}
	copy(transition.PredecessorCandidateID[:], predecessor)
	copy(transition.SuccessorCandidateID[:], successor)
	copy(transition.WriterCertificateID[:], writerCertificate)
	transition.TargetMembershipGeneration = uint32(targetGeneration)
	if transition.PredecessorCandidateID == transition.SuccessorCandidateID {
		return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("transition candidate links are not distinct")
	}

	wantParticipants := 1
	if transition.PredecessorCandidateID != ([32]byte{}) {
		wantParticipants = 2
	}
	if len(participants) != wantParticipants {
		return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("transition participant count is inconsistent")
	}
	successorParticipant, ok := participants[transition.SuccessorCandidateID]
	if !ok || successorParticipant.role != syncAuthorityCandidateRoleRecoverySuccessorV1 ||
		(successorParticipant.state != "staging" && successorParticipant.state != "ready") ||
		successorParticipant.membershipGeneration < transition.TargetMembershipGeneration {
		return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("successor link does not match its candidate role")
	}
	if transition.PredecessorCandidateID == ([32]byte{}) {
		if transition.TargetMembershipGeneration != 1 {
			return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("generation-one transition has an invalid target")
		}
	} else {
		predecessorParticipant, ok := participants[transition.PredecessorCandidateID]
		if !ok || predecessorParticipant.role != syncAuthorityCandidateRoleRecoveryPredecessorV1 || predecessorParticipant.state != "ready" ||
			transition.TargetMembershipGeneration < 2 || predecessorParticipant.membershipGeneration != transition.TargetMembershipGeneration-1 {
			return SyncAuthorityRecoveryTransition{}, false, corruptSyncAuthorityRecoveryTransitionV1("predecessor link does not match its candidate role")
		}
	}
	return transition, true, nil
}

func readSyncAuthorityRecoveryParticipantsV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
) (map[[32]byte]syncAuthorityRecoveryParticipantV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT candidate_id, role, state, membership_generation
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND role <> 'ordinary'
ORDER BY candidate_id`, string(projectID))
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	participants := make(map[[32]byte]syncAuthorityRecoveryParticipantV1, 2)
	for rows.Next() {
		var candidateIDBytes []byte
		var participant syncAuthorityRecoveryParticipantV1
		var membershipGeneration int64
		if err := rows.Scan(&candidateIDBytes, &participant.role, &participant.state, &membershipGeneration); err != nil {
			return nil, syncTransactionProblem(ctx)
		}
		if len(candidateIDBytes) != sha256.Size || isZeroDigestBytesV2(candidateIDBytes) ||
			(participant.role != syncAuthorityCandidateRoleRecoveryPredecessorV1 && participant.role != syncAuthorityCandidateRoleRecoverySuccessorV1) ||
			membershipGeneration < 1 || membershipGeneration > math.MaxUint32 {
			return nil, corruptSyncAuthorityRecoveryTransitionV1("transition participant is malformed")
		}
		participant.membershipGeneration = uint32(membershipGeneration)
		var candidateID [32]byte
		copy(candidateID[:], candidateIDBytes)
		if _, duplicate := participants[candidateID]; duplicate {
			return nil, corruptSyncAuthorityRecoveryTransitionV1("transition participant identity is duplicated")
		}
		participants[candidateID] = participant
	}
	if err := rows.Err(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return participants, nil
}

func corruptSyncAuthorityRecoveryTransitionV1(detail string) error {
	return syncProblem(SyncErrorStore, "sync_authority_recovery_transition", detail)
}
