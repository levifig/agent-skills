package sqlite

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	syncAuthorityRecoveryTerminalReceiptDigestVersionV1 = 1
	syncAuthorityRecoveryTerminalReceiptDigestDomainV1  = "loaf/vnext/sync-authority-recovery-terminal-receipt/v1"
)

// SyncAuthorityRecoveryTerminalOutcome identifies the one terminal result of a
// recovery attempt.
type SyncAuthorityRecoveryTerminalOutcome string

const (
	SyncAuthorityRecoveryAborted  SyncAuthorityRecoveryTerminalOutcome = "aborted"
	SyncAuthorityRecoveryPromoted SyncAuthorityRecoveryTerminalOutcome = "promoted"
)

// SyncAuthorityRecoveryTerminalReceipt is the sensitive-value-free, bounded receipt
// retained after a recovery transition and its candidates can disappear.
type SyncAuthorityRecoveryTerminalReceipt struct {
	Outcome             SyncAuthorityRecoveryTerminalOutcome
	Transition          SyncAuthorityRecoveryTransition
	SuccessorCheckpoint SyncAuthorityCandidateCheckpoint
}

func newSyncAuthorityRecoveryTerminalReceiptV1(
	outcome SyncAuthorityRecoveryTerminalOutcome,
	transition SyncAuthorityRecoveryTransition,
	checkpoint SyncAuthorityCandidateCheckpoint,
) (SyncAuthorityRecoveryTerminalReceipt, error) {
	receipt := SyncAuthorityRecoveryTerminalReceipt{
		Outcome:             outcome,
		Transition:          transition,
		SuccessorCheckpoint: checkpoint,
	}
	if err := validateSyncAuthorityRecoveryTerminalReceiptV1(receipt); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, err
	}
	return receipt, nil
}

func readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	attemptID [32]byte,
) (SyncAuthorityRecoveryTerminalReceipt, bool, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, err
	}
	if attemptID == ([32]byte{}) {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, syncProblem(SyncErrorInvalid, "sync_authority_recovery_attempt", "must not be zero")
	}

	receipt := SyncAuthorityRecoveryTerminalReceipt{
		Transition: SyncAuthorityRecoveryTransition{ProjectID: projectID},
	}
	var persistedAttempt, predecessor, successor, writerCertificate []byte
	var rollingDigest, authorityDigest, receiptDigest []byte
	var targetGeneration, pageCount, environmentCount, ready, digestVersion int64
	err := tx.QueryRowContext(ctx, `
SELECT
  attempt_id, outcome, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation,
  successor_page_count, successor_environment_count,
  successor_through_environment_id, successor_rolling_environment_digest,
  successor_ready, successor_authority_digest,
  receipt_digest_version, receipt_digest
FROM continuity_sync_authority_recovery_terminal_receipts
WHERE project_id = ? AND attempt_id = ?`, string(projectID), attemptID[:]).Scan(
		&persistedAttempt, &receipt.Outcome, &predecessor, &successor,
		&receipt.Transition.WriterEnvironmentID, &writerCertificate, &targetGeneration,
		&pageCount, &environmentCount, &receipt.SuccessorCheckpoint.ThroughEnvironmentID,
		&rollingDigest, &ready, &authorityDigest, &digestVersion, &receiptDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, nil
	}
	if err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, syncTransactionProblem(ctx)
	}
	if len(persistedAttempt) != sha256.Size || isZeroDigestBytesV2(persistedAttempt) ||
		(predecessor != nil && (len(predecessor) != sha256.Size || isZeroDigestBytesV2(predecessor))) ||
		len(successor) != sha256.Size || isZeroDigestBytesV2(successor) ||
		len(writerCertificate) != sha256.Size || isZeroDigestBytesV2(writerCertificate) ||
		targetGeneration < 1 || targetGeneration > math.MaxUint32 ||
		pageCount < 1 || pageCount > math.MaxInt64/maximumSyncAuthorityCandidatePageEnvironments ||
		environmentCount < pageCount || environmentCount > pageCount*maximumSyncAuthorityCandidatePageEnvironments ||
		len(rollingDigest) != sha256.Size || isZeroDigestBytesV2(rollingDigest) ||
		(ready != 0 && ready != 1) ||
		(authorityDigest != nil && (len(authorityDigest) != sha256.Size || isZeroDigestBytesV2(authorityDigest))) ||
		digestVersion != syncAuthorityRecoveryTerminalReceiptDigestVersionV1 ||
		len(receiptDigest) != sha256.Size || isZeroDigestBytesV2(receiptDigest) {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, corruptSyncAuthorityRecoveryTerminalReceiptV1("receipt row is malformed")
	}
	copy(receipt.Transition.AttemptID[:], persistedAttempt)
	copy(receipt.Transition.PredecessorCandidateID[:], predecessor)
	copy(receipt.Transition.SuccessorCandidateID[:], successor)
	copy(receipt.Transition.WriterCertificateID[:], writerCertificate)
	receipt.Transition.TargetMembershipGeneration = uint32(targetGeneration)
	receipt.SuccessorCheckpoint.CandidateID = receipt.Transition.SuccessorCandidateID
	receipt.SuccessorCheckpoint.PageCount = pageCount
	receipt.SuccessorCheckpoint.EnvironmentCount = environmentCount
	copy(receipt.SuccessorCheckpoint.RollingEnvironmentDigest[:], rollingDigest)
	receipt.SuccessorCheckpoint.Ready = ready == 1
	copy(receipt.SuccessorCheckpoint.AuthorityDigest[:], authorityDigest)
	if receipt.Transition.AttemptID != attemptID {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, corruptSyncAuthorityRecoveryTerminalReceiptV1("receipt attempt key is stale")
	}
	if err := validateSyncAuthorityRecoveryTerminalReceiptV1(receipt); err != nil {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, corruptSyncAuthorityRecoveryTerminalReceiptV1("receipt fields are inconsistent")
	}
	wantDigest := syncAuthorityRecoveryTerminalReceiptDigestV1(receipt)
	if subtle.ConstantTimeCompare(receiptDigest, wantDigest[:]) != 1 {
		return SyncAuthorityRecoveryTerminalReceipt{}, false, corruptSyncAuthorityRecoveryTerminalReceiptV1("receipt digest is stale")
	}
	return receipt, true, nil
}

func insertSyncAuthorityRecoveryTerminalReceiptV1(
	ctx context.Context,
	tx *sql.Tx,
	receipt SyncAuthorityRecoveryTerminalReceipt,
) error {
	if err := validateSyncAuthorityRecoveryTerminalReceiptV1(receipt); err != nil {
		return err
	}
	current, found, err := readAndAuditSyncAuthorityRecoveryTerminalReceiptV1(
		ctx, tx, receipt.Transition.ProjectID, receipt.Transition.AttemptID,
	)
	if err != nil {
		return err
	}
	if found {
		if syncAuthorityRecoveryTerminalReceiptMatchesV1(current, receipt) {
			return nil
		}
		return syncProblem(SyncErrorConflict, "sync_authority_recovery_terminal_receipt", "attempt already has a different terminal receipt")
	}

	var predecessor, authorityDigest any
	if receipt.Transition.PredecessorCandidateID != ([32]byte{}) {
		predecessor = receipt.Transition.PredecessorCandidateID[:]
	}
	if receipt.SuccessorCheckpoint.Ready {
		authorityDigest = receipt.SuccessorCheckpoint.AuthorityDigest[:]
	}
	digest := syncAuthorityRecoveryTerminalReceiptDigestV1(receipt)
	ready := 0
	if receipt.SuccessorCheckpoint.Ready {
		ready = 1
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authority_recovery_terminal_receipts(
  project_id, attempt_id, outcome, predecessor_candidate_id,
  successor_candidate_id, writer_environment_id, writer_certificate_id,
  target_membership_generation, successor_page_count,
  successor_environment_count, successor_through_environment_id,
  successor_rolling_environment_digest, successor_ready,
  successor_authority_digest, receipt_digest_version, receipt_digest
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
		string(receipt.Transition.ProjectID), receipt.Transition.AttemptID[:], string(receipt.Outcome), predecessor,
		receipt.Transition.SuccessorCandidateID[:], string(receipt.Transition.WriterEnvironmentID), receipt.Transition.WriterCertificateID[:],
		receipt.Transition.TargetMembershipGeneration, receipt.SuccessorCheckpoint.PageCount,
		receipt.SuccessorCheckpoint.EnvironmentCount, receipt.SuccessorCheckpoint.ThroughEnvironmentID,
		receipt.SuccessorCheckpoint.RollingEnvironmentDigest[:], ready, authorityDigest, digest[:],
	); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func syncAuthorityRecoveryTerminalReceiptMatchesInputV1(
	receipt SyncAuthorityRecoveryTerminalReceipt,
	outcome SyncAuthorityRecoveryTerminalOutcome,
	transition SyncAuthorityRecoveryTransition,
	checkpoint SyncAuthorityCandidateCheckpoint,
) bool {
	return receipt.Outcome == outcome &&
		syncAuthorityRecoveryTransitionMatchesV1(receipt.Transition, transition) &&
		receipt.SuccessorCheckpoint == checkpoint
}

func syncAuthorityRecoveryTerminalReceiptMatchesV1(left, right SyncAuthorityRecoveryTerminalReceipt) bool {
	return syncAuthorityRecoveryTerminalReceiptMatchesInputV1(
		left, right.Outcome, right.Transition, right.SuccessorCheckpoint,
	)
}

func validateSyncAuthorityRecoveryTerminalReceiptV1(receipt SyncAuthorityRecoveryTerminalReceipt) error {
	if err := validateSyncAuthorityRecoveryTransitionV1(receipt.Transition); err != nil {
		return err
	}
	checkpoint := receipt.SuccessorCheckpoint
	if err := validateSyncAuthorityCandidateCheckpointV2(checkpoint); err != nil ||
		checkpoint.CandidateID != receipt.Transition.SuccessorCandidateID ||
		checkpoint.PageCount > math.MaxInt64/maximumSyncAuthorityCandidatePageEnvironments ||
		checkpoint.EnvironmentCount < checkpoint.PageCount ||
		checkpoint.EnvironmentCount > checkpoint.PageCount*maximumSyncAuthorityCandidatePageEnvironments {
		return syncProblem(SyncErrorInvalid, "sync_authority_recovery_terminal_receipt", "has an invalid successor checkpoint")
	}
	if (receipt.Outcome == SyncAuthorityRecoveryAborted && checkpoint.Ready) ||
		(receipt.Outcome == SyncAuthorityRecoveryPromoted && !checkpoint.Ready) ||
		(receipt.Outcome != SyncAuthorityRecoveryAborted && receipt.Outcome != SyncAuthorityRecoveryPromoted) {
		return syncProblem(SyncErrorInvalid, "sync_authority_recovery_terminal_receipt", "has an invalid outcome")
	}
	return nil
}

func syncAuthorityRecoveryTerminalReceiptDigestV1(receipt SyncAuthorityRecoveryTerminalReceipt) [32]byte {
	fields := [][]byte{
		[]byte(receipt.Outcome),
		[]byte(receipt.Transition.ProjectID),
		receipt.Transition.AttemptID[:],
		receipt.Transition.PredecessorCandidateID[:],
		receipt.Transition.SuccessorCandidateID[:],
		[]byte(receipt.Transition.WriterEnvironmentID),
		receipt.Transition.WriterCertificateID[:],
		binary.BigEndian.AppendUint32(nil, receipt.Transition.TargetMembershipGeneration),
		receipt.SuccessorCheckpoint.CandidateID[:],
		binary.BigEndian.AppendUint64(nil, uint64(receipt.SuccessorCheckpoint.PageCount)),
		binary.BigEndian.AppendUint64(nil, uint64(receipt.SuccessorCheckpoint.EnvironmentCount)),
		[]byte(receipt.SuccessorCheckpoint.ThroughEnvironmentID),
		receipt.SuccessorCheckpoint.RollingEnvironmentDigest[:],
		[]byte{0},
		receipt.SuccessorCheckpoint.AuthorityDigest[:],
	}
	if receipt.SuccessorCheckpoint.Ready {
		fields[13][0] = 1
	}
	var encoded []byte
	encoded = appendRecoveryReceiptDigestFieldV1(encoded, []byte(syncAuthorityRecoveryTerminalReceiptDigestDomainV1))
	encoded = appendRecoveryReceiptDigestFieldV1(encoded, binary.BigEndian.AppendUint16(nil, syncAuthorityRecoveryTerminalReceiptDigestVersionV1))
	for _, field := range fields {
		encoded = appendRecoveryReceiptDigestFieldV1(encoded, field)
	}
	return sha256.Sum256(encoded)
}

func appendRecoveryReceiptDigestFieldV1(encoded, field []byte) []byte {
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field)))
	return append(encoded, field...)
}

func corruptSyncAuthorityRecoveryTerminalReceiptV1(detail string) error {
	return syncProblem(SyncErrorStore, "sync_authority_recovery_terminal_receipt", detail)
}
