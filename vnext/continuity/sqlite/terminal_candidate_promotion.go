package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"math"
	"sort"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

const terminalCandidateCorpusDigestDomainV1 = "loaf.continuity.sync.post-promotion-corpus.v1"

// TerminalCandidateReceipt is the permanent fixed-size receipt for one
// promoted terminal candidate. It binds the immutable staging header to the
// resulting canonical corpus and applied cursor.
type TerminalCandidateReceipt struct {
	ProjectID                 continuity.ProjectID
	CandidateID               [32]byte
	ChannelID                 SyncChannelID
	RelayGeneration           [32]byte
	MembershipGeneration      uint32
	AuthorityDigest           [32]byte
	StartArrivalSequence      int64
	ThroughArrivalSequence    int64
	FrameCount                int64
	RollingCandidateDigest    [32]byte
	PostPromotionCorpusDigest [32]byte
	ResultingAppliedCursor    int64
}

type preparedTerminalCandidatePromotionFrameV1 struct {
	normalized              terminalCandidateFrameV1
	inbox                   OpaqueSyncFrame
	inboxBytes              []byte
	sealedFact              *storedFactV1
	prunedReference         *VerifiedPruneReference
	prunedKind              continuity.FactKind
	prunedArrivalDigest     [32]byte
	newSource               bool
	newFact                 bool
	insertReceipt           bool
	insertTombstone         bool
	fillPrunedArrivalDigest bool
	deleteOutbox            bool
}

// PromoteTerminalCandidate strictly revalidates and atomically promotes the
// exact staged candidate named by expected. An exact retry returns the retained
// immutable receipt without consulting mutable authority state.
func (store *Store) PromoteTerminalCandidate(
	ctx context.Context,
	projectID continuity.ProjectID,
	expected TerminalCandidateCheckpoint,
) (TerminalCandidateReceipt, error) {
	if err := validateTerminalCandidateCheckpointV1(expected); err != nil {
		return TerminalCandidateReceipt{}, err
	}
	if err := projectID.Validate(); err != nil {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if store == nil {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return TerminalCandidateReceipt{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return TerminalCandidateReceipt{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	if receipt, found, err := readPromotedTerminalCandidateReceiptV1(ctx, tx, projectID, expected.CandidateID); err != nil {
		return TerminalCandidateReceipt{}, err
	} else if found {
		if !terminalCandidateReceiptMatchesCheckpointV1(receipt, expected) {
			return TerminalCandidateReceipt{}, syncProblem(SyncErrorConflict, "checkpoint", "does not match the promoted candidate")
		}
		if err := tx.Commit(); err != nil {
			return TerminalCandidateReceipt{}, syncTransactionProblem(ctx)
		}
		return receipt, nil
	}
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return TerminalCandidateReceipt{}, err
	}

	candidate, found, err := readActiveTerminalCandidateV1(ctx, tx, projectID)
	if err != nil {
		return TerminalCandidateReceipt{}, err
	}
	if !found || !terminalCandidateMatchesCheckpointV1(candidate, expected) {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorConflict, "checkpoint", "does not match the active candidate")
	}
	binding, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	}
	if err != nil {
		return TerminalCandidateReceipt{}, err
	}
	candidateID, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, binding, candidate.StartArrivalSequence)
	if err != nil {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorStore, "", "terminal candidate identity could not be rederived")
	}
	if !terminalCandidateHeaderMatchesAuthorityBindingV2(candidate, binding, candidateID) {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorConflict, "sync_authority", "changed after terminal staging")
	}
	if binding.AuthorityDigestVersion == 2 {
		if err := requireKnownExactSyncRelayWatermarkV1(
			ctx, tx, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
		); err != nil {
			return TerminalCandidateReceipt{}, err
		}
	}
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return TerminalCandidateReceipt{}, err
	}
	if !found {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	if candidate.StartArrivalSequence < 1 || progress.AppliedCursor != candidate.StartArrivalSequence-1 ||
		progress.DownloadedCursor < candidate.ThroughArrivalSequence || progress.ChannelID != candidate.ChannelID {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorConflict, "sync_progress", "does not match the staged candidate prefix")
	}
	if binding.AuthorityDigestVersion == 2 {
		if candidate.ThroughArrivalSequence != binding.InventoryArrivalHead {
			return TerminalCandidateReceipt{}, syncProblem(SyncErrorCursor, "inventory_arrival_head", "candidate does not cover the exact authority cutoff")
		}
		if progress.DownloadedCursor != binding.InventoryArrivalHead {
			return TerminalCandidateReceipt{}, syncProblem(SyncErrorCursor, "downloaded_cursor", "does not match the exact authority cutoff")
		}
		if progress.RelayHead != binding.InventoryArrivalHead {
			return TerminalCandidateReceipt{}, syncProblem(SyncErrorCursor, "relay_head", "does not match the exact authority cutoff")
		}
	}

	frames, err := readTerminalCandidatePromotionFramesV1(ctx, tx, candidate)
	if err != nil {
		return TerminalCandidateReceipt{}, err
	}
	environmentIDs := make([]continuity.EnvironmentID, len(frames))
	for index := range frames {
		environmentIDs[index] = frames[index].normalized.environmentID
	}
	authorityEnvironments, err := readCanonicalSyncEnvironmentCertificatesV2(ctx, tx, projectID, binding, environmentIDs)
	if err != nil {
		return TerminalCandidateReceipt{}, err
	}
	if err := validateTerminalCandidatePromotionFramesV1(ctx, tx, candidate, authorityEnvironments, frames); err != nil {
		return TerminalCandidateReceipt{}, err
	}
	resultingFacts, err := planTerminalCandidatePromotionV1(ctx, tx, projectID, authorityEnvironments, frames)
	if err != nil {
		return TerminalCandidateReceipt{}, err
	}
	if len(resultingFacts) == 0 {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorCandidate, "", "candidate corpus has no project identity")
	}
	if _, err := foldProjectSnapshotV1(ctx, projectID, 0, resultingFacts); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return TerminalCandidateReceipt{}, ctxErr
		}
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorCandidate, "", "complete candidate corpus is not valid")
	}
	corpusDigest, err := terminalCandidateCorpusDigestV1(projectID, resultingFacts)
	if err != nil || corpusDigest == ([32]byte{}) {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorStore, "", "post-promotion corpus digest could not be derived")
	}

	if err := applyTerminalCandidatePromotionV1(ctx, tx, candidate, frames); err != nil {
		return TerminalCandidateReceipt{}, err
	}
	receipt := terminalCandidateReceiptV1(candidate, corpusDigest)
	if err := finishTerminalCandidatePromotionV1(ctx, tx, receipt, frames, binding, progress); err != nil {
		return TerminalCandidateReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return TerminalCandidateReceipt{}, syncProblem(SyncErrorStore, "", "terminal candidate promotion outcome is unknown; retry the exact checkpoint")
	}
	return receipt, nil
}

func terminalCandidateMatchesCheckpointV1(candidate TerminalCandidate, checkpoint TerminalCandidateCheckpoint) bool {
	return candidate.CandidateID == checkpoint.CandidateID &&
		candidate.ThroughArrivalSequence == checkpoint.ThroughArrivalSequence &&
		candidate.FrameCount == checkpoint.FrameCount &&
		candidate.RollingCandidateDigest == checkpoint.RollingCandidateDigest
}

func terminalCandidateReceiptMatchesCheckpointV1(receipt TerminalCandidateReceipt, checkpoint TerminalCandidateCheckpoint) bool {
	return receipt.CandidateID == checkpoint.CandidateID &&
		receipt.ThroughArrivalSequence == checkpoint.ThroughArrivalSequence &&
		receipt.FrameCount == checkpoint.FrameCount &&
		receipt.RollingCandidateDigest == checkpoint.RollingCandidateDigest
}

func terminalCandidateReceiptV1(candidate TerminalCandidate, corpusDigest [32]byte) TerminalCandidateReceipt {
	return TerminalCandidateReceipt{
		ProjectID:                 candidate.ProjectID,
		CandidateID:               candidate.CandidateID,
		ChannelID:                 candidate.ChannelID,
		RelayGeneration:           candidate.RelayGeneration,
		MembershipGeneration:      candidate.MembershipGeneration,
		AuthorityDigest:           candidate.AuthorityDigest,
		StartArrivalSequence:      candidate.StartArrivalSequence,
		ThroughArrivalSequence:    candidate.ThroughArrivalSequence,
		FrameCount:                candidate.FrameCount,
		RollingCandidateDigest:    candidate.RollingCandidateDigest,
		PostPromotionCorpusDigest: corpusDigest,
		ResultingAppliedCursor:    candidate.ThroughArrivalSequence,
	}
}

func readPromotedTerminalCandidateReceiptV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte) (TerminalCandidateReceipt, bool, error) {
	var retainedCandidateID, channelID, relayGeneration, authorityDigest, rollingDigest, corpusDigest []byte
	var membershipGeneration int64
	receipt := TerminalCandidateReceipt{ProjectID: projectID}
	err := tx.QueryRowContext(ctx, `
SELECT candidate_id, channel_id, relay_generation, membership_generation,
       authority_digest, start_arrival_sequence, through_arrival_sequence,
       frame_count, rolling_candidate_digest, post_promotion_corpus_digest,
       resulting_applied_cursor
FROM continuity_sync_terminal_candidates
WHERE project_id = ? AND candidate_id = ? AND state = 'promoted'`, string(projectID), candidateID[:]).Scan(
		&retainedCandidateID, &channelID, &relayGeneration, &membershipGeneration,
		&authorityDigest, &receipt.StartArrivalSequence, &receipt.ThroughArrivalSequence,
		&receipt.FrameCount, &rollingDigest, &corpusDigest, &receipt.ResultingAppliedCursor,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TerminalCandidateReceipt{}, false, nil
	}
	if err != nil {
		return TerminalCandidateReceipt{}, false, syncTransactionProblem(ctx)
	}
	if len(retainedCandidateID) != 32 || len(channelID) != 32 || len(relayGeneration) != 32 || len(authorityDigest) != 32 ||
		len(rollingDigest) != 32 || len(corpusDigest) != 32 || membershipGeneration < 1 || membershipGeneration > math.MaxUint32 ||
		receipt.StartArrivalSequence < 1 || receipt.ThroughArrivalSequence < receipt.StartArrivalSequence || receipt.FrameCount < 1 ||
		receipt.ThroughArrivalSequence-receipt.StartArrivalSequence == math.MaxInt64 ||
		receipt.FrameCount != receipt.ThroughArrivalSequence-receipt.StartArrivalSequence+1 ||
		receipt.ResultingAppliedCursor != receipt.ThroughArrivalSequence {
		return TerminalCandidateReceipt{}, false, syncProblem(SyncErrorStore, "", "promoted terminal candidate receipt is corrupt")
	}
	copy(receipt.CandidateID[:], retainedCandidateID)
	copy(receipt.ChannelID[:], channelID)
	copy(receipt.RelayGeneration[:], relayGeneration)
	copy(receipt.AuthorityDigest[:], authorityDigest)
	copy(receipt.RollingCandidateDigest[:], rollingDigest)
	copy(receipt.PostPromotionCorpusDigest[:], corpusDigest)
	receipt.MembershipGeneration = uint32(membershipGeneration)
	if receipt.CandidateID == ([32]byte{}) || receipt.ChannelID == (SyncChannelID{}) || receipt.RelayGeneration == ([32]byte{}) ||
		receipt.AuthorityDigest == ([32]byte{}) || receipt.RollingCandidateDigest == ([32]byte{}) || receipt.PostPromotionCorpusDigest == ([32]byte{}) {
		return TerminalCandidateReceipt{}, false, syncProblem(SyncErrorStore, "", "promoted terminal candidate receipt is corrupt")
	}
	rederivedCandidateID, err := deriveTerminalCandidateIDFromReceiptV1(receipt)
	if err != nil || rederivedCandidateID != receipt.CandidateID {
		return TerminalCandidateReceipt{}, false, syncProblem(SyncErrorStore, "", "promoted terminal candidate receipt is corrupt")
	}
	var appliedCursor, childCount int64
	if err := tx.QueryRowContext(ctx, `
SELECT p.applied_cursor,
       (SELECT COUNT(*)
        FROM continuity_sync_terminal_candidate_frames AS f
        WHERE f.project_id = p.project_id AND f.candidate_id = ?)
FROM continuity_sync_projects AS p
WHERE p.project_id = ?`, receipt.CandidateID[:], string(projectID)).Scan(&appliedCursor, &childCount); err != nil {
		return TerminalCandidateReceipt{}, false, syncProblem(SyncErrorStore, "", "promoted terminal candidate receipt is corrupt")
	}
	if appliedCursor < receipt.ResultingAppliedCursor || childCount != 0 {
		return TerminalCandidateReceipt{}, false, syncProblem(SyncErrorStore, "", "promoted terminal candidate receipt is corrupt")
	}
	return receipt, true, nil
}

func deriveTerminalCandidateIDFromReceiptV1(receipt TerminalCandidateReceipt) ([32]byte, error) {
	if receipt.ProjectID.Validate() != nil || receipt.ChannelID == (SyncChannelID{}) || receipt.RelayGeneration == ([32]byte{}) ||
		receipt.MembershipGeneration == 0 || receipt.AuthorityDigest == ([32]byte{}) || receipt.StartArrivalSequence < 1 {
		return [32]byte{}, invalidTerminalCandidateCodecV1()
	}
	identityTranscript, err := encodeTerminalCandidateTranscriptV1(
		terminalCandidateIdentityDomainV1,
		maximumTerminalCandidateIdentityBytesV1,
		terminalCandidateUint16BytesV1(terminalCandidateCodecVersionV1),
		[]byte(receipt.ProjectID),
		receipt.ChannelID[:],
		receipt.RelayGeneration[:],
		terminalCandidateUint32BytesV1(receipt.MembershipGeneration),
		receipt.AuthorityDigest[:],
		terminalCandidateInt64BytesV1(receipt.StartArrivalSequence),
	)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(identityTranscript), nil
}

func readTerminalCandidatePromotionFramesV1(ctx context.Context, tx *sql.Tx, candidate TerminalCandidate) ([]preparedTerminalCandidatePromotionFrameV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT f.arrival_sequence, f.frame_kind, f.fact_id, f.environment_id,
       f.environment_sequence, f.hlc_wall_millis, f.hlc_logical,
       f.previous_envelope_digest, f.envelope_digest, f.certificate_id,
       f.key_generation, f.nonce, f.prune_certificate_id, f.candidate_bytes,
       i.envelope_digest, i.frame_kind, i.frame_bytes, i.state
FROM continuity_sync_terminal_candidate_frames AS f
JOIN continuity_sync_inbox AS i
  ON i.project_id = f.project_id AND i.arrival_sequence = f.arrival_sequence
WHERE f.project_id = ? AND f.candidate_id = ?
ORDER BY f.arrival_sequence ASC`, string(candidate.ProjectID), candidate.CandidateID[:])
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	frames := make([]preparedTerminalCandidatePromotionFrameV1, 0)
	for rows.Next() {
		prepared, err := scanTerminalCandidatePromotionFrameV1(rows, candidate.ProjectID, candidate.CandidateID)
		if err != nil {
			return nil, err
		}
		frames = append(frames, prepared)
	}
	if err := rows.Err(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return frames, nil
}

func scanTerminalCandidatePromotionFrameV1(scanner interface{ Scan(dest ...any) error }, projectID continuity.ProjectID, candidateID [32]byte) (preparedTerminalCandidatePromotionFrameV1, error) {
	var prepared preparedTerminalCandidatePromotionFrameV1
	frame := terminalCandidateFrameV1{projectID: projectID, candidateID: candidateID}
	var previousDigest, envelopeDigest, certificateID, nonce, pruneCertificateID, candidateBytes []byte
	var inboxDigest, inboxBytes []byte
	var environmentSequence, wall, logical, keyGeneration int64
	var inboxKind, inboxState string
	if err := scanner.Scan(
		&frame.arrivalSequence, &frame.frameKind, &frame.factID, &frame.environmentID,
		&environmentSequence, &wall, &logical, &previousDigest, &envelopeDigest,
		&certificateID, &keyGeneration, &nonce, &pruneCertificateID, &candidateBytes,
		&inboxDigest, &inboxKind, &inboxBytes, &inboxState,
	); err != nil {
		return preparedTerminalCandidatePromotionFrameV1{}, syncProblem(SyncErrorStore, "", "terminal candidate frame could not be read")
	}
	if environmentSequence < 1 || logical < 0 || logical > math.MaxInt32 || keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
		len(previousDigest) != 32 || len(envelopeDigest) != 32 || len(certificateID) != 32 || len(nonce) != 24 {
		return preparedTerminalCandidatePromotionFrameV1{}, syncProblem(SyncErrorStore, "", "terminal candidate frame is corrupt")
	}
	frame.environmentSequence = environmentSequence
	frame.clock = continuity.HybridTime{WallMillis: wall, Logical: int32(logical)}
	copy(frame.previousEnvelopeDigest[:], previousDigest)
	copy(frame.envelopeDigest[:], envelopeDigest)
	copy(frame.certificateID[:], certificateID)
	frame.keyGeneration = uint32(keyGeneration)
	copy(frame.nonce[:], nonce)
	if pruneCertificateID != nil {
		if len(pruneCertificateID) != 32 {
			return preparedTerminalCandidatePromotionFrameV1{}, syncProblem(SyncErrorStore, "", "terminal candidate frame is corrupt")
		}
		pruneID := [32]byte{}
		copy(pruneID[:], pruneCertificateID)
		frame.pruneCertificateID = &pruneID
	}
	frame.candidateBytes = append([]byte(nil), candidateBytes...)
	if _, err := terminalCandidateFrameDigestV1(frame); err != nil {
		return preparedTerminalCandidatePromotionFrameV1{}, syncProblem(SyncErrorStore, "", "terminal candidate frame is corrupt")
	}
	inbox, err := opaqueSyncFrameFromColumnsV1(frame.arrivalSequence, inboxDigest, inboxKind, inboxBytes, inboxState)
	if err != nil || inbox.Quarantined {
		return preparedTerminalCandidatePromotionFrameV1{}, syncProblem(SyncErrorStore, "", "terminal candidate inbox row is corrupt")
	}
	if err := validateTerminalCandidateInboxBindingV1(frame, inbox); err != nil {
		return preparedTerminalCandidatePromotionFrameV1{}, err
	}
	prepared.normalized = frame
	prepared.inbox = inbox
	prepared.inboxBytes = append([]byte(nil), inboxBytes...)
	return prepared, nil
}

func validateTerminalCandidatePromotionFramesV1(ctx context.Context, tx *sql.Tx, candidate TerminalCandidate, authorityEnvironments map[continuity.EnvironmentID]SyncEnvironmentCertificate, frames []preparedTerminalCandidatePromotionFrameV1) error {
	if int64(len(frames)) != candidate.FrameCount || len(frames) == 0 {
		return syncProblem(SyncErrorStore, "", "terminal candidate frame count is corrupt")
	}
	rolling, err := terminalCandidateRollingSeedV1(candidate.CandidateID)
	if err != nil {
		return syncProblem(SyncErrorStore, "", "terminal candidate rolling seed is invalid")
	}
	expectedArrival := candidate.StartArrivalSequence
	for index := range frames {
		frame := &frames[index]
		if frame.normalized.arrivalSequence != expectedArrival {
			return syncProblem(SyncErrorStore, "", "terminal candidate arrivals are not contiguous")
		}
		environment, found := authorityEnvironments[frame.normalized.environmentID]
		if !found || frame.normalized.certificateID != environment.CertificateID {
			return syncProblem(SyncErrorCertificate, "certificate_id", "does not match pinned authority")
		}
		if err := validateTerminalCandidateRetirementFenceV1(environment, frame.normalized); err != nil {
			return err
		}
		frameDigest, err := terminalCandidateFrameDigestV1(frame.normalized)
		if err != nil {
			return syncProblem(SyncErrorStore, "", "terminal candidate frame digest is invalid")
		}
		count := int64(index + 1)
		rolling, err = terminalCandidateRollingStepV1(candidate.CandidateID, count, rolling, frameDigest)
		if err != nil {
			return syncProblem(SyncErrorStore, "", "terminal candidate rolling digest is invalid")
		}
		if frame.normalized.frameKind == terminalCandidateFrameKindSealedV1 {
			fact, err := terminalCandidatePromotionStoredFactV1(frame.normalized)
			if err != nil {
				return err
			}
			frame.sealedFact = &fact
		} else {
			body, err := decodeTerminalCandidatePrunedBodyV1(frame.normalized.candidateBytes)
			if err != nil {
				return syncProblem(SyncErrorStore, "", "pruned terminal candidate body is corrupt")
			}
			reference := terminalCandidatePruneReferenceV1(frame.normalized)
			frame.prunedReference = &reference
			frame.prunedKind = body.FactKind
			frame.prunedArrivalDigest = body.InboxFrameDigest
		}
		if index != len(frames)-1 {
			if expectedArrival == math.MaxInt64 {
				return syncProblem(SyncErrorStore, "", "terminal candidate arrival range overflows")
			}
			expectedArrival++
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if frames[len(frames)-1].normalized.arrivalSequence != candidate.ThroughArrivalSequence || rolling != candidate.RollingCandidateDigest {
		return syncProblem(SyncErrorStore, "", "terminal candidate header digest is corrupt")
	}
	return nil
}

func terminalCandidatePromotionStoredFactV1(frame terminalCandidateFrameV1) (storedFactV1, error) {
	fact, err := decodeTerminalCandidateSealedBodyV1(frame.projectID, frame.candidateBytes)
	if err != nil {
		return storedFactV1{}, syncProblem(SyncErrorStore, "", "sealed terminal candidate body is corrupt")
	}
	prepared, err := prepareVerifiedSyncFrames(frame.projectID, []VerifiedSyncFrame{{
		ArrivalSequence:        frame.arrivalSequence,
		PreviousEnvelopeDigest: frame.previousEnvelopeDigest,
		EnvelopeDigest:         frame.envelopeDigest,
		CertificateID:          frame.certificateID,
		KeyGeneration:          frame.keyGeneration,
		Nonce:                  frame.nonce,
		Fact:                   fact,
	}}, 0, 0)
	if err != nil || len(prepared) != 1 {
		return storedFactV1{}, syncProblem(SyncErrorStore, "", "sealed terminal candidate body is corrupt")
	}
	return prepared[0].fact, nil
}

func terminalCandidatePruneReferenceV1(frame terminalCandidateFrameV1) VerifiedPruneReference {
	return VerifiedPruneReference{
		FactID:                 frame.factID,
		EnvironmentID:          frame.environmentID,
		EnvironmentSequence:    frame.environmentSequence,
		ArrivalSequence:        frame.arrivalSequence,
		EnvelopeDigest:         frame.envelopeDigest,
		CertificateID:          frame.certificateID,
		PreviousEnvelopeDigest: frame.previousEnvelopeDigest,
		KeyGeneration:          frame.keyGeneration,
		Nonce:                  frame.nonce,
	}
}

func planTerminalCandidatePromotionV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, authorityEnvironments map[continuity.EnvironmentID]SyncEnvironmentCertificate, frames []preparedTerminalCandidatePromotionFrameV1) ([]storedFactV1, error) {
	existing, err := loadProjectFactsV1(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	byFact := make(map[continuity.FactID]storedFactV1, len(existing)+len(frames))
	bySource := make(map[string]storedFactV1, len(existing)+len(frames))
	for _, fact := range existing {
		byFact[fact.factID] = fact
		bySource[environmentSequenceKeyV1(fact.environmentID, fact.environmentSequence)] = fact
	}
	inventory, err := loadEnvelopeInventoryV1(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	frontiers := make(map[continuity.EnvironmentID]terminalCandidateFrontierV1)
	frontierLoaded := make(map[continuity.EnvironmentID]bool)
	touched := make(map[continuity.EnvironmentID]bool)
	for index := range frames {
		frame := &frames[index]
		normalized := frame.normalized
		metadata := terminalCandidateMetadataV1(normalized)
		if otherProject, found, err := terminalCandidateGlobalTombstoneProjectV1(ctx, tx, normalized.factID); err != nil {
			return nil, err
		} else if found && otherProject != projectID {
			return nil, syncProblem(SyncErrorConflict, "fact_id", "is tombstoned for another project")
		}
		canonical, sourceFound, err := readCanonicalTerminalSourceV1(ctx, tx, projectID, normalized.environmentID, normalized.environmentSequence)
		if err != nil {
			return nil, err
		}
		if sourceFound && (canonical.factID != normalized.factID || !sealedMetadataEqualV1(canonical.metadata, metadata)) {
			return nil, syncProblem(SyncErrorConflict, "environment_sequence", "conflicts with canonical envelope identity")
		}
		if err := inventory.admit(normalized.environmentID, normalized.environmentSequence, metadata); err != nil {
			return nil, err
		}
		if err := validateTerminalCandidatePromotionReceiptCollisionV1(ctx, tx, projectID, normalized); err != nil {
			return nil, err
		}
		receiptExists, err := terminalCandidatePromotionReceiptExistsV1(ctx, tx, projectID, normalized)
		if err != nil {
			return nil, err
		}
		tombstone, tombstoneFound, err := readTerminalCandidatePromotionTombstoneV1(ctx, tx, projectID, normalized)
		if err != nil {
			return nil, err
		}
		outboxFound, err := readTerminalCandidatePromotionOutboxV1(ctx, tx, projectID, normalized, frame.inboxBytes)
		if err != nil {
			return nil, err
		}

		liveByFact, factFound := byFact[normalized.factID]
		liveBySource, sourceFactFound := bySource[environmentSequenceKeyV1(normalized.environmentID, normalized.environmentSequence)]
		if !factFound {
			global, globalFound, err := readFactByIDV1(ctx, tx, normalized.factID)
			if err != nil {
				return nil, err
			}
			if globalFound {
				if global.projectID != projectID {
					return nil, syncProblem(SyncErrorConflict, "fact_id", "is bound to another project")
				}
				return nil, syncProblem(SyncErrorStore, "", "project fact index is inconsistent")
			}
		}
		if normalized.frameKind == terminalCandidateFrameKindSealedV1 {
			fact := *frame.sealedFact
			if factFound && !storedFactsEqualV1(liveByFact, fact) {
				return nil, syncProblem(SyncErrorConflict, "fact_id", "is bound to different immutable fields")
			}
			if sourceFactFound && !storedFactsEqualV1(liveBySource, fact) {
				return nil, syncProblem(SyncErrorConflict, "environment_sequence", "is bound to a different fact")
			}
			if tombstoneFound {
				if !terminalCandidateTombstoneMatchesFrameV1(tombstone, normalized) {
					return nil, syncProblem(SyncErrorConflict, "tombstone", "conflicts with the sealed candidate")
				}
				if factFound || sourceFactFound {
					return nil, syncProblem(SyncErrorStore, "", "fact is both live and tombstoned")
				}
			}
			if sourceFound && !factFound && !tombstoneFound && !outboxFound {
				return nil, syncProblem(SyncErrorStore, "", "canonical envelope has no retained fact or tombstone")
			}
			frame.newFact = !factFound && !tombstoneFound
			frame.insertReceipt = !receiptExists
			frame.deleteOutbox = outboxFound
			if frame.newFact {
				byFact[fact.factID] = fact
				bySource[environmentSequenceKeyV1(fact.environmentID, fact.environmentSequence)] = fact
				existing = append(existing, fact)
			}
		} else {
			if factFound || sourceFactFound {
				return nil, syncProblem(SyncErrorConflict, "fact_id", "pruned candidate identifies a live fact")
			}
			if outboxFound {
				return nil, syncProblem(SyncErrorConflict, "outbox", "pruned candidate identifies a sealed outbox fact")
			}
			if receiptExists && !tombstoneFound {
				return nil, syncProblem(SyncErrorStore, "", "pruned envelope receipt has no tombstone")
			}
			if tombstoneFound {
				if !terminalCandidateTombstoneMatchesFrameV1(tombstone, normalized) ||
					tombstone.reference.ArrivalSequence != normalized.arrivalSequence ||
					tombstone.pruneCertificateID != *normalized.pruneCertificateID {
					return nil, syncProblem(SyncErrorConflict, "tombstone", "conflicts with the pruned candidate")
				}
				if tombstone.prunedArrivalDigestKnown && tombstone.prunedArrivalDigest != frame.prunedArrivalDigest {
					return nil, syncProblem(SyncErrorConflict, "tombstone", "conflicts with the exact pruned arrival")
				}
				frame.fillPrunedArrivalDigest = !tombstone.prunedArrivalDigestKnown
			} else {
				frame.insertTombstone = true
			}
		}

		if !sourceFound {
			if !frontierLoaded[normalized.environmentID] {
				frontier, found, err := readTerminalCandidateCanonicalFrontierV1(ctx, tx, projectID, normalized.environmentID)
				if err != nil {
					return nil, err
				}
				if found {
					frontiers[normalized.environmentID] = frontier
				}
				frontierLoaded[normalized.environmentID] = true
			}
			frontier := frontiers[normalized.environmentID]
			next, err := advanceTerminalCandidateFrontierV1(frontier, normalized)
			if err != nil {
				return nil, err
			}
			frontiers[normalized.environmentID] = next
			frame.newSource = true
			touched[normalized.environmentID] = true
		}
	}
	for environmentID := range touched {
		environment, found := authorityEnvironments[environmentID]
		if !found {
			return nil, syncProblem(SyncErrorStore, "sync_authority", "touched environment certificate was not retained")
		}
		if environment.Retirement == nil {
			continue
		}
		frontier := frontiers[environmentID]
		if frontier.sequence != environment.Retirement.FinalEnvironmentSequence || frontier.metadata.digest != environment.Retirement.FinalEnvelopeDigest {
			return nil, syncProblem(SyncErrorRecoveryRequired, "", "")
		}
	}
	sort.Slice(existing, func(left, right int) bool { return storedFactLessV1(existing[left], existing[right]) })
	return existing, nil
}

func readTerminalCandidateCanonicalFrontierV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, environmentID continuity.EnvironmentID) (terminalCandidateFrontierV1, bool, error) {
	var sequence, wall, logical int64
	err := tx.QueryRowContext(ctx, `
SELECT highest_sequence, hlc_wall_millis, hlc_logical
FROM continuity_sync_environment_heads
WHERE project_id = ? AND environment_id = ?`, string(projectID), string(environmentID)).Scan(&sequence, &wall, &logical)
	if errors.Is(err, sql.ErrNoRows) {
		return terminalCandidateFrontierV1{}, false, nil
	}
	if err != nil {
		return terminalCandidateFrontierV1{}, false, syncTransactionProblem(ctx)
	}
	if sequence < 1 || logical < 0 || logical > math.MaxInt32 {
		return terminalCandidateFrontierV1{}, false, syncProblem(SyncErrorStore, "", "environment head is corrupt")
	}
	source, found, err := readCanonicalTerminalSourceV1(ctx, tx, projectID, environmentID, sequence)
	if err != nil {
		return terminalCandidateFrontierV1{}, false, err
	}
	if !found {
		return terminalCandidateFrontierV1{}, false, syncProblem(SyncErrorStore, "", "environment head has no canonical envelope identity")
	}
	return terminalCandidateFrontierV1{
		sequence: sequence,
		clock:    continuity.HybridTime{WallMillis: wall, Logical: int32(logical)},
		metadata: source.metadata,
	}, true, nil
}

func validateTerminalCandidatePromotionReceiptCollisionV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frame terminalCandidateFrameV1) error {
	rows, err := tx.QueryContext(ctx, `
SELECT arrival_sequence, fact_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_receipts
WHERE project_id = ? AND (
  arrival_sequence = ? OR fact_id = ? OR
  (environment_id = ? AND environment_sequence = ?) OR
  (key_generation = ? AND nonce = ?)
)`, string(projectID), frame.arrivalSequence, string(frame.factID), string(frame.environmentID), frame.environmentSequence, frame.keyGeneration, frame.nonce[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()
	for rows.Next() {
		var factID continuity.FactID
		var environmentID continuity.EnvironmentID
		var arrival, sequence, keyGeneration int64
		var previousDigest, digest, certificateID, nonce []byte
		if err := rows.Scan(&arrival, &factID, &environmentID, &sequence, &previousDigest, &digest, &certificateID, &keyGeneration, &nonce); err != nil {
			return syncTransactionProblem(ctx)
		}
		metadata := terminalCandidateMetadataV1(frame)
		if arrival != frame.arrivalSequence || factID != frame.factID || environmentID != frame.environmentID || sequence != frame.environmentSequence ||
			len(previousDigest) != 32 || len(digest) != 32 || len(certificateID) != 32 || len(nonce) != 24 ||
			keyGeneration != int64(frame.keyGeneration) || !bytes.Equal(previousDigest, metadata.previousDigest[:]) ||
			!bytes.Equal(digest, metadata.digest[:]) || !bytes.Equal(certificateID, metadata.certificateID[:]) || !bytes.Equal(nonce, metadata.nonce[:]) {
			return syncProblem(SyncErrorConflict, "receipt", "conflicts with the terminal candidate")
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func terminalCandidatePromotionReceiptExistsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frame terminalCandidateFrameV1) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM continuity_sync_receipts
WHERE project_id = ? AND arrival_sequence = ? AND fact_id = ?
  AND environment_id = ? AND environment_sequence = ?`,
		string(projectID), frame.arrivalSequence, string(frame.factID), string(frame.environmentID), frame.environmentSequence).Scan(&count); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return count != 0, nil
}

func readTerminalCandidatePromotionTombstoneV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frame terminalCandidateFrameV1) (retainedPruneTombstoneV1, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, arrival_sequence,
       previous_envelope_digest, envelope_digest, certificate_id, key_generation,
       nonce, prune_certificate_id, pruned_arrival_digest
FROM continuity_sync_tombstones
WHERE project_id = ? AND (
  fact_id = ? OR arrival_sequence = ? OR
  (environment_id = ? AND environment_sequence = ?) OR
  (key_generation = ? AND nonce = ?)
)`, string(projectID), string(frame.factID), frame.arrivalSequence, string(frame.environmentID), frame.environmentSequence, frame.keyGeneration, frame.nonce[:])
	if err != nil {
		return retainedPruneTombstoneV1{}, false, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	var retained retainedPruneTombstoneV1
	found := false
	for rows.Next() {
		if found {
			return retainedPruneTombstoneV1{}, false, syncProblem(SyncErrorStore, "", "terminal candidate tombstone identities disagree")
		}
		var previousDigest, digest, certificateID, nonce, pruneCertificateID, prunedArrivalDigest []byte
		var keyGeneration int64
		if err := rows.Scan(&retained.reference.FactID, &retained.reference.EnvironmentID, &retained.reference.EnvironmentSequence,
			&retained.reference.ArrivalSequence, &previousDigest, &digest, &certificateID, &keyGeneration, &nonce, &pruneCertificateID,
			&prunedArrivalDigest); err != nil {
			return retainedPruneTombstoneV1{}, false, syncTransactionProblem(ctx)
		}
		if len(previousDigest) != 32 || len(digest) != 32 || len(certificateID) != 32 || len(nonce) != 24 || len(pruneCertificateID) != 32 ||
			keyGeneration < 1 || keyGeneration > math.MaxUint32 {
			return retainedPruneTombstoneV1{}, false, syncProblem(SyncErrorStore, "", "terminal candidate tombstone is corrupt")
		}
		copy(retained.reference.PreviousEnvelopeDigest[:], previousDigest)
		copy(retained.reference.EnvelopeDigest[:], digest)
		copy(retained.reference.CertificateID[:], certificateID)
		retained.reference.KeyGeneration = uint32(keyGeneration)
		copy(retained.reference.Nonce[:], nonce)
		copy(retained.pruneCertificateID[:], pruneCertificateID)
		if prunedArrivalDigest != nil {
			if len(prunedArrivalDigest) != 32 {
				return retainedPruneTombstoneV1{}, false, syncProblem(SyncErrorStore, "", "terminal candidate tombstone is corrupt")
			}
			copy(retained.prunedArrivalDigest[:], prunedArrivalDigest)
			retained.prunedArrivalDigestKnown = true
		}
		found = true
	}
	if err := rows.Err(); err != nil {
		return retainedPruneTombstoneV1{}, false, syncTransactionProblem(ctx)
	}
	return retained, found, nil
}

func terminalCandidateTombstoneMatchesFrameV1(tombstone retainedPruneTombstoneV1, frame terminalCandidateFrameV1) bool {
	return tombstone.reference.FactID == frame.factID && tombstone.reference.EnvironmentID == frame.environmentID &&
		tombstone.reference.EnvironmentSequence == frame.environmentSequence &&
		tombstone.reference.PreviousEnvelopeDigest == frame.previousEnvelopeDigest && tombstone.reference.EnvelopeDigest == frame.envelopeDigest &&
		tombstone.reference.CertificateID == frame.certificateID && tombstone.reference.KeyGeneration == frame.keyGeneration && tombstone.reference.Nonce == frame.nonce
}

func readTerminalCandidatePromotionOutboxV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frame terminalCandidateFrameV1, inboxBytes []byte) (bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce, sealed_envelope
FROM continuity_sync_outbox
WHERE project_id = ? AND (
  fact_id = ? OR (environment_id = ? AND environment_sequence = ?) OR
  (key_generation = ? AND nonce = ?)
)`, string(projectID), string(frame.factID), string(frame.environmentID), frame.environmentSequence, frame.keyGeneration, frame.nonce[:])
	if err != nil {
		return false, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		if found {
			return false, syncProblem(SyncErrorStore, "", "terminal candidate outbox identities disagree")
		}
		var factID continuity.FactID
		var environmentID continuity.EnvironmentID
		var sequence, keyGeneration int64
		var previousDigest, digest, certificateID, nonce, sealedEnvelope []byte
		if err := rows.Scan(&factID, &environmentID, &sequence, &previousDigest, &digest, &certificateID, &keyGeneration, &nonce, &sealedEnvelope); err != nil {
			return false, syncTransactionProblem(ctx)
		}
		metadata := terminalCandidateMetadataV1(frame)
		if factID != frame.factID || environmentID != frame.environmentID || sequence != frame.environmentSequence ||
			len(previousDigest) != 32 || len(digest) != 32 || len(certificateID) != 32 || len(nonce) != 24 ||
			keyGeneration != int64(frame.keyGeneration) || !bytes.Equal(previousDigest, metadata.previousDigest[:]) ||
			!bytes.Equal(digest, metadata.digest[:]) || !bytes.Equal(certificateID, metadata.certificateID[:]) || !bytes.Equal(nonce, metadata.nonce[:]) {
			return false, syncProblem(SyncErrorConflict, "outbox", "conflicts with the terminal candidate")
		}
		if frame.frameKind == terminalCandidateFrameKindSealedV1 && !bytes.Equal(sealedEnvelope, inboxBytes) {
			return false, syncProblem(SyncErrorConflict, "outbox", "sealed bytes differ from the terminal candidate")
		}
		found = true
	}
	if err := rows.Err(); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return found, nil
}

func terminalCandidateGlobalTombstoneProjectV1(ctx context.Context, tx *sql.Tx, factID continuity.FactID) (continuity.ProjectID, bool, error) {
	var projectID continuity.ProjectID
	err := tx.QueryRowContext(ctx, `SELECT project_id FROM continuity_sync_tombstones WHERE fact_id = ?`, string(factID)).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, syncTransactionProblem(ctx)
	}
	return projectID, true, nil
}

func terminalCandidateCorpusDigestV1(projectID continuity.ProjectID, facts []storedFactV1) ([32]byte, error) {
	if projectID.Validate() != nil || len(facts) == 0 {
		return [32]byte{}, invalidTerminalCandidateCodecV1()
	}
	hasher := sha256.New()
	write := func(value []byte) { _, _ = hasher.Write(value) }
	write(binary.BigEndian.AppendUint32(nil, uint32(len(terminalCandidateCorpusDigestDomainV1))))
	write([]byte(terminalCandidateCorpusDigestDomainV1))
	write(binary.BigEndian.AppendUint16(nil, terminalCandidateCodecVersionV1))
	write(binary.BigEndian.AppendUint32(nil, uint32(len(projectID))))
	write([]byte(projectID))
	write(binary.BigEndian.AppendUint64(nil, uint64(len(facts))))
	for index, fact := range facts {
		if fact.projectID != projectID || (index > 0 && !storedFactLessV1(facts[index-1], fact)) {
			return [32]byte{}, invalidTerminalCandidateCodecV1()
		}
		encoded, err := continuitywire.Encode(storedFactWireV1(fact))
		if err != nil || len(encoded) < 1 || len(encoded) > continuitywire.MaxFactBytes {
			return [32]byte{}, invalidTerminalCandidateCodecV1()
		}
		write(binary.BigEndian.AppendUint32(nil, uint32(len(encoded))))
		write(encoded)
	}
	var digest [32]byte
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func applyTerminalCandidatePromotionV1(ctx context.Context, tx *sql.Tx, candidate TerminalCandidate, frames []preparedTerminalCandidatePromotionFrameV1) error {
	newFacts := make([]storedFactV1, 0)
	for _, frame := range frames {
		if frame.newFact {
			newFacts = append(newFacts, *frame.sealedFact)
		}
	}
	sort.Slice(newFacts, func(left, right int) bool { return storedFactLessV1(newFacts[left], newFacts[right]) })
	insert := func(fact storedFactV1) error {
		return insertFactV1(ctx, tx, appendIntentV1{projectID: fact.projectID, factID: fact.factID, subject: fact.subject, kind: fact.kind, content: fact.content}, fact.environmentID, fact.environmentSequence, fact.clock)
	}
	root := -1
	for index, fact := range newFacts {
		if fact.kind == continuity.FactProjectRegistered {
			root = index
			break
		}
	}
	if root >= 0 {
		if err := insert(newFacts[root]); err != nil {
			return err
		}
	}
	for index, fact := range newFacts {
		if index != root {
			if err := insert(fact); err != nil {
				return err
			}
		}
	}
	for _, frame := range frames {
		fact := storedFactV1{projectID: candidate.ProjectID, environmentID: frame.normalized.environmentID, environmentSequence: frame.normalized.environmentSequence, clock: frame.normalized.clock}
		if frame.newSource {
			if err := advanceEnvironmentHeadV1(ctx, tx, fact); err != nil {
				return err
			}
		}
		if err := recordSealedEnvironmentHeadV1(ctx, tx, fact, terminalCandidateMetadataV1(frame.normalized)); err != nil {
			return err
		}
		if frame.insertReceipt {
			if err := insertTerminalCandidatePromotionReceiptV1(ctx, tx, candidate.ProjectID, frame.normalized); err != nil {
				return err
			}
		}
		if frame.deleteOutbox {
			result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_outbox
WHERE project_id = ? AND fact_id = ? AND environment_id = ? AND environment_sequence = ?
  AND previous_envelope_digest = ? AND envelope_digest = ? AND certificate_id = ?
  AND key_generation = ? AND nonce = ? AND sealed_envelope = ?`,
				string(candidate.ProjectID), string(frame.normalized.factID), string(frame.normalized.environmentID), frame.normalized.environmentSequence,
				frame.normalized.previousEnvelopeDigest[:], frame.normalized.envelopeDigest[:], frame.normalized.certificateID[:],
				frame.normalized.keyGeneration, frame.normalized.nonce[:], frame.inboxBytes)
			if err != nil {
				return syncTransactionProblem(ctx)
			}
			if err := requireOneAffectedV1(result, ctx); err != nil {
				return syncProblem(SyncErrorConflict, "outbox", "changed during terminal promotion")
			}
		}
		if frame.insertTombstone {
			if err := insertPruneTombstoneV1(
				ctx, tx, candidate.ProjectID, *frame.prunedReference,
				*frame.normalized.pruneCertificateID, &frame.prunedArrivalDigest,
			); err != nil {
				return err
			}
		}
		if frame.fillPrunedArrivalDigest {
			if err := fillTerminalCandidatePrunedArrivalDigestV1(ctx, tx, candidate.ProjectID, frame); err != nil {
				return err
			}
		}
	}
	return nil
}

func fillTerminalCandidatePrunedArrivalDigestV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	frame preparedTerminalCandidatePromotionFrameV1,
) error {
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_tombstones
SET pruned_arrival_digest = ?
WHERE project_id = ? AND fact_id = ? AND arrival_sequence = ?
  AND prune_certificate_id = ? AND pruned_arrival_digest IS NULL`,
		frame.prunedArrivalDigest[:], string(projectID), string(frame.normalized.factID),
		frame.normalized.arrivalSequence, frame.normalized.pruneCertificateID[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "tombstone", "changed during terminal promotion")
	}
	return nil
}

func insertTerminalCandidatePromotionReceiptV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frame terminalCandidateFrameV1) error {
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_receipts(
  project_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), frame.arrivalSequence, string(frame.factID), string(frame.environmentID), frame.environmentSequence,
		frame.previousEnvelopeDigest[:], frame.envelopeDigest[:], frame.certificateID[:], frame.keyGeneration, frame.nonce[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}

func finishTerminalCandidatePromotionV1(
	ctx context.Context,
	tx *sql.Tx,
	receipt TerminalCandidateReceipt,
	frames []preparedTerminalCandidatePromotionFrameV1,
	binding SyncAuthorityBinding,
	progress SyncProgress,
) error {
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_terminal_candidate_frames
WHERE project_id = ? AND candidate_id = ?`, string(receipt.ProjectID), receipt.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != receipt.FrameCount {
		return syncProblem(SyncErrorConflict, "candidate_frames", "changed during terminal promotion")
	}
	for _, frame := range frames {
		result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence = ? AND envelope_digest = ?
  AND frame_kind = ? AND frame_bytes = ? AND state = 'staged'`,
			string(receipt.ProjectID), frame.normalized.arrivalSequence, frame.normalized.envelopeDigest[:], frame.normalized.frameKind, frame.inboxBytes)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return syncProblem(SyncErrorConflict, "inbox", "changed during terminal promotion")
		}
	}
	result, err = tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET applied_cursor = ?
WHERE project_id = ?
  AND channel_id = ? AND relay_generation = ? AND admin_public_key = ?
  AND membership_generation = ? AND activation_state = ?
  AND applied_cursor = ? AND downloaded_cursor = ? AND relay_head = ?
  AND EXISTS (
    SELECT 1
    FROM continuity_sync_authorities
    WHERE project_id = continuity_sync_projects.project_id
      AND digest_version = ? AND authority_digest = ? AND inventory_arrival_head = ?
  )`,
		receipt.ResultingAppliedCursor, string(receipt.ProjectID),
		binding.ChannelID[:], binding.RelayGeneration[:], binding.AdminPublicKey[:],
		binding.MembershipGeneration, progress.ActivationState,
		progress.AppliedCursor, progress.DownloadedCursor, progress.RelayHead,
		binding.AuthorityDigestVersion, binding.AuthorityDigest[:], binding.InventoryArrivalHead)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "sync_progress", "changed during terminal promotion")
	}
	result, err = tx.ExecContext(ctx, `
UPDATE continuity_sync_terminal_candidates
SET state = 'promoted', post_promotion_corpus_digest = ?, resulting_applied_cursor = ?
WHERE project_id = ? AND candidate_id = ? AND state = 'staging'
  AND channel_id = ? AND relay_generation = ? AND membership_generation = ?
  AND authority_digest = ? AND start_arrival_sequence = ? AND through_arrival_sequence = ?
  AND frame_count = ? AND rolling_candidate_digest = ?
  AND post_promotion_corpus_digest IS NULL AND resulting_applied_cursor IS NULL`,
		receipt.PostPromotionCorpusDigest[:], receipt.ResultingAppliedCursor,
		string(receipt.ProjectID), receipt.CandidateID[:], receipt.ChannelID[:], receipt.RelayGeneration[:],
		receipt.MembershipGeneration, receipt.AuthorityDigest[:], receipt.StartArrivalSequence,
		receipt.ThroughArrivalSequence, receipt.FrameCount, receipt.RollingCandidateDigest[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "candidate", "changed during terminal promotion")
	}
	return nil
}
