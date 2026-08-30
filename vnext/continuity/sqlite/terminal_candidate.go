package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

// VerifiedTerminalPrunedFrame is one authenticated prune reference and its
// authenticated projection inputs. The caller is responsible for constructing
// it only from the exact PrunedArrival bytes carried by Inbox.
type VerifiedTerminalPrunedFrame struct {
	Reference          VerifiedPruneReference
	PruneCertificateID [32]byte
	FactKind           continuity.FactKind
	HLC                continuity.HybridTime
}

// VerifiedTerminalCandidateFrame binds exactly one staged opaque arrival to
// exactly one verified sealed fact or authenticated prune reference.
type VerifiedTerminalCandidateFrame struct {
	Inbox  OpaqueSyncFrame
	Sealed *VerifiedSyncFrame
	Pruned *VerifiedTerminalPrunedFrame
}

// TerminalCandidate is the fixed-size durable checkpoint for the one active
// terminal-history candidate of a project. It intentionally exposes no
// verified plaintext frames.
type TerminalCandidate struct {
	ProjectID              continuity.ProjectID
	CandidateID            [32]byte
	ChannelID              SyncChannelID
	RelayGeneration        [32]byte
	MembershipGeneration   uint32
	AuthorityDigest        [32]byte
	StartArrivalSequence   int64
	ThroughArrivalSequence int64
	FrameCount             int64
	RollingCandidateDigest [32]byte
}

// TerminalCandidateCheckpoint is the compare-and-swap token required to
// discard an active candidate without deleting a newer resume attempt.
type TerminalCandidateCheckpoint struct {
	CandidateID            [32]byte
	ThroughArrivalSequence int64
	FrameCount             int64
	RollingCandidateDigest [32]byte
}

type preparedTerminalCandidateFrameV1 struct {
	inbox      OpaqueSyncFrame
	normalized terminalCandidateFrameV1
	sealedFact *storedFactV1
}

type terminalCandidateSourceV1 struct {
	projectID           continuity.ProjectID
	factID              continuity.FactID
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	metadata            sealedEnvelopeMetadataV1
}

type terminalCandidateFrontierV1 struct {
	sequence int64
	clock    continuity.HybridTime
	metadata sealedEnvelopeMetadataV1
}

// StageVerifiedTerminalCandidateChunk creates, resumes, or exactly replays one
// bounded verified terminal-history chunk under the caller-verified authority
// binding. It does not fold or promote facts.
func (store *Store) StageVerifiedTerminalCandidateChunk(
	ctx context.Context,
	projectID continuity.ProjectID,
	verifiedAuthority SyncAuthorityBinding,
	frames []VerifiedTerminalCandidateFrame,
	trustedNowMillis,
	maxFutureSkewMillis int64,
) (TerminalCandidate, error) {
	prepared, err := prepareTerminalCandidateChunkV1(projectID, frames, trustedNowMillis, maxFutureSkewMillis)
	if err != nil {
		return TerminalCandidate{}, err
	}
	if err := validateSyncAuthorityBindingV2(verifiedAuthority); err != nil {
		return TerminalCandidate{}, err
	}
	if store == nil {
		return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return TerminalCandidate{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return TerminalCandidate{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return TerminalCandidate{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return TerminalCandidate{}, err
	}
	if !found {
		return TerminalCandidate{}, syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	binding, err := requireExactCanonicalSyncAuthorityBindingV2(ctx, tx, projectID, verifiedAuthority)
	if err != nil {
		return TerminalCandidate{}, err
	}

	current, active, err := readActiveTerminalCandidateV1(ctx, tx, projectID)
	if err != nil {
		return TerminalCandidate{}, err
	}
	start := prepared[0].normalized.arrivalSequence
	if active {
		start = current.StartArrivalSequence
	}
	candidateID, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, binding, start)
	if err != nil {
		return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "terminal candidate identity could not be derived")
	}
	if active {
		if !terminalCandidateHeaderMatchesAuthorityBindingV2(current, binding, candidateID) {
			return TerminalCandidate{}, syncProblem(SyncErrorConflict, "sync_authority", "active candidate is bound to another authority snapshot")
		}
		if current.StartArrivalSequence < 1 || current.StartArrivalSequence-1 != progress.AppliedCursor {
			return TerminalCandidate{}, syncProblem(SyncErrorConflict, "applied_cursor", "changed while a terminal candidate is active")
		}
	} else {
		if progress.AppliedCursor == math.MaxInt64 || prepared[0].normalized.arrivalSequence != progress.AppliedCursor+1 {
			return TerminalCandidate{}, syncProblem(SyncErrorArrivalGap, "arrival_sequence", "first chunk is not the staged applied prefix")
		}
	}

	for index := range prepared {
		prepared[index].normalized.candidateID = candidateID
		if prepared[index].normalized.arrivalSequence > progress.DownloadedCursor {
			return TerminalCandidate{}, syncProblem(SyncErrorCursor, "arrival_sequence", "exceeds downloaded progress")
		}
		if err := requireExactTerminalCandidateInboxV1(ctx, tx, projectID, prepared[index]); err != nil {
			return TerminalCandidate{}, err
		}
	}
	environmentIDs := make([]continuity.EnvironmentID, len(prepared))
	for index := range prepared {
		environmentIDs[index] = prepared[index].normalized.environmentID
	}
	authorityEnvironments, err := readCanonicalSyncEnvironmentCertificatesV2(ctx, tx, projectID, binding, environmentIDs)
	if err != nil {
		return TerminalCandidate{}, err
	}
	for index := range prepared {
		frame := &prepared[index]
		environment, found := authorityEnvironments[frame.normalized.environmentID]
		if !found || frame.normalized.certificateID != environment.CertificateID {
			return TerminalCandidate{}, syncProblem(SyncErrorCertificate, "certificate_id", "does not match pinned authority")
		}
		if err := validateTerminalCandidateRetirementFenceV1(environment, frame.normalized); err != nil {
			return TerminalCandidate{}, err
		}
	}

	firstArrival := prepared[0].normalized.arrivalSequence
	lastArrival := prepared[len(prepared)-1].normalized.arrivalSequence
	if active && firstArrival <= current.ThroughArrivalSequence {
		if lastArrival > current.ThroughArrivalSequence {
			return TerminalCandidate{}, syncProblem(SyncErrorConflict, "arrival_sequence", "chunk partially overlaps the active candidate")
		}
		if err := requireExactTerminalCandidateReplayV1(ctx, tx, projectID, candidateID, prepared); err != nil {
			return TerminalCandidate{}, err
		}
		if err := tx.Commit(); err != nil {
			return TerminalCandidate{}, syncTransactionProblem(ctx)
		}
		return current, nil
	}
	if active {
		if current.ThroughArrivalSequence == math.MaxInt64 || firstArrival != current.ThroughArrivalSequence+1 {
			return TerminalCandidate{}, syncProblem(SyncErrorArrivalGap, "arrival_sequence", "chunk is not the next candidate suffix")
		}
	}

	firstFrameTerminalTrigger := false
	frontiers := make(map[continuity.EnvironmentID]terminalCandidateFrontierV1)
	for index := range prepared {
		frame := &prepared[index]
		environment := authorityEnvironments[frame.normalized.environmentID]
		firstSeen, err := terminalCandidateFirstSeenSourceV1(ctx, tx, projectID, frame.normalized)
		if err != nil {
			return TerminalCandidate{}, err
		}
		if environment.Mode == SyncEnvironmentEphemeral && trustedNowMillis >= environment.ExpiresAtMillis && environment.Retirement == nil {
			return TerminalCandidate{}, syncProblem(SyncErrorRecoveryRequired, "", "")
		}
		isTerminalTrigger := frame.normalized.frameKind == terminalCandidateFrameKindPrunedV1 ||
			(firstSeen && ordinarySyncEnvironmentRequiresTerminalHistoryV1(environment, trustedNowMillis))
		if index == 0 {
			firstFrameTerminalTrigger = isTerminalTrigger
		}
		exactCanonicalDuplicate, err := validateTerminalCandidateCollisionsV1(ctx, tx, projectID, candidateID, *frame)
		if err != nil {
			return TerminalCandidate{}, err
		}
		if exactCanonicalDuplicate {
			continue
		}
		frontier, loaded := frontiers[frame.normalized.environmentID]
		if !loaded {
			frontier, found, err = readTerminalCandidateFrontierV1(ctx, tx, projectID, candidateID, frame.normalized.environmentID)
			if err != nil {
				return TerminalCandidate{}, err
			}
			if !found {
				frontier = terminalCandidateFrontierV1{}
			}
		}
		next, err := advanceTerminalCandidateFrontierV1(frontier, frame.normalized)
		if err != nil {
			return TerminalCandidate{}, err
		}
		frontiers[frame.normalized.environmentID] = next
	}
	if !active && !firstFrameTerminalTrigger {
		return TerminalCandidate{}, syncProblem(SyncErrorCandidate, "", "terminal trigger is missing")
	}

	rolling := current.RollingCandidateDigest
	count := current.FrameCount
	if !active {
		rolling, err = terminalCandidateRollingSeedV1(candidateID)
		if err != nil {
			return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "terminal candidate seed could not be derived")
		}
		count = 0
	}
	for index := range prepared {
		if count == math.MaxInt64 {
			return TerminalCandidate{}, syncProblem(SyncErrorCursor, "frame_count", "candidate frame count is exhausted")
		}
		frameDigest, digestErr := terminalCandidateFrameDigestV1(prepared[index].normalized)
		if digestErr != nil {
			return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "normalized terminal frame is invalid")
		}
		count++
		rolling, err = terminalCandidateRollingStepV1(candidateID, count, rolling, frameDigest)
		if err != nil {
			return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "terminal candidate digest could not be advanced")
		}
	}
	next := TerminalCandidate{
		ProjectID:              projectID,
		CandidateID:            candidateID,
		ChannelID:              binding.ChannelID,
		RelayGeneration:        binding.RelayGeneration,
		MembershipGeneration:   binding.MembershipGeneration,
		AuthorityDigest:        binding.AuthorityDigest,
		StartArrivalSequence:   start,
		ThroughArrivalSequence: lastArrival,
		FrameCount:             count,
		RollingCandidateDigest: rolling,
	}
	if !active {
		if err := insertTerminalCandidateHeaderV1(ctx, tx, next); err != nil {
			return TerminalCandidate{}, err
		}
	}
	for _, frame := range prepared {
		if err := insertTerminalCandidateFrameV1(ctx, tx, projectID, candidateID, frame.normalized); err != nil {
			return TerminalCandidate{}, err
		}
	}
	if active {
		if err := compareAndSwapTerminalCandidateHeaderV1(ctx, tx, current, next); err != nil {
			return TerminalCandidate{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return TerminalCandidate{}, syncProblem(SyncErrorStore, "", "terminal candidate commit outcome is unknown")
	}
	return next, nil
}

// CurrentTerminalCandidate returns only the fixed-size active staging header.
func (store *Store) CurrentTerminalCandidate(ctx context.Context, projectID continuity.ProjectID) (TerminalCandidate, bool, error) {
	if err := projectID.Validate(); err != nil {
		return TerminalCandidate{}, false, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if store == nil {
		return TerminalCandidate{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return TerminalCandidate{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return TerminalCandidate{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return TerminalCandidate{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return TerminalCandidate{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	candidate, found, err := readActiveTerminalCandidateV1(ctx, tx, projectID)
	if err != nil {
		return TerminalCandidate{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return TerminalCandidate{}, false, syncTransactionProblem(ctx)
	}
	return candidate, found, nil
}

// DiscardTerminalCandidate removes exactly the active staging candidate named
// by checkpoint. Child rows cascade; inbox and canonical state are preserved.
func (store *Store) DiscardTerminalCandidate(ctx context.Context, projectID continuity.ProjectID, checkpoint TerminalCandidateCheckpoint) error {
	if err := validateTerminalCandidateCheckpointV1(checkpoint); err != nil {
		return err
	}
	if err := projectID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, "project_id", "is invalid")
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
	var promoted int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_terminal_candidates
  WHERE project_id = ? AND candidate_id = ? AND state = 'promoted'
)`, string(projectID), checkpoint.CandidateID[:]).Scan(&promoted); err != nil {
		return syncTransactionProblem(ctx)
	}
	if promoted != 0 {
		return syncProblem(SyncErrorConflict, "candidate_id", "identifies a promoted candidate")
	}
	current, found, err := readActiveTerminalCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		if err := tx.Commit(); err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	if current.CandidateID != checkpoint.CandidateID ||
		current.ThroughArrivalSequence != checkpoint.ThroughArrivalSequence ||
		current.FrameCount != checkpoint.FrameCount ||
		current.RollingCandidateDigest != checkpoint.RollingCandidateDigest {
		return syncProblem(SyncErrorConflict, "checkpoint", "does not match the active candidate")
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_terminal_candidates
WHERE project_id = ? AND candidate_id = ? AND state = 'staging'
  AND through_arrival_sequence = ? AND frame_count = ?
  AND rolling_candidate_digest = ?`,
		string(projectID), checkpoint.CandidateID[:], checkpoint.ThroughArrivalSequence,
		checkpoint.FrameCount, checkpoint.RollingCandidateDigest[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "checkpoint", "active candidate changed")
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "terminal candidate discard outcome is unknown")
	}
	return nil
}

func prepareTerminalCandidateChunkV1(projectID continuity.ProjectID, frames []VerifiedTerminalCandidateFrame, trustedNowMillis, maxFutureSkewMillis int64) ([]preparedTerminalCandidateFrameV1, error) {
	if err := projectID.Validate(); err != nil {
		return nil, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if len(frames) < 1 || len(frames) > maximumTerminalCandidateChunkFramesV1 {
		return nil, syncProblem(SyncErrorInvalid, "frames", "must contain between 1 and 16 arrivals")
	}
	if trustedNowMillis < 0 || maxFutureSkewMillis < 0 {
		return nil, syncProblem(SyncErrorInvalid, "clock", "trusted time and skew must be nonnegative")
	}
	prepared := make([]preparedTerminalCandidateFrameV1, 0, len(frames))
	seenFacts := make(map[continuity.FactID]struct{}, len(frames))
	seenSources := make(map[string]struct{}, len(frames))
	seenNonces := make(map[string]struct{}, len(frames))
	var budget terminalCandidateChunkBudgetV1
	for index, input := range frames {
		frameKind, inboxBytes, err := opaqueSyncFrameStorageV1(input.Inbox)
		if err != nil {
			return nil, err
		}
		if input.Inbox.Quarantined {
			return nil, syncProblem(SyncErrorInvalid, "inbox.quarantined", "is output-only state")
		}
		if (input.Sealed == nil) == (input.Pruned == nil) {
			return nil, syncProblem(SyncErrorCandidate, "frame", "must contain exactly one verified representation")
		}
		entry := preparedTerminalCandidateFrameV1{inbox: cloneOpaqueSyncFrameV1(input.Inbox)}
		if input.Sealed != nil {
			if frameKind != terminalCandidateFrameKindSealedV1 || input.Inbox.ArrivalSequence != input.Sealed.ArrivalSequence || input.Inbox.EnvelopeDigest != input.Sealed.EnvelopeDigest {
				return nil, syncProblem(SyncErrorConflict, "inbox", "does not match the verified sealed frame")
			}
			sealed, err := prepareVerifiedSyncFrames(projectID, []VerifiedSyncFrame{*input.Sealed}, trustedNowMillis, maxFutureSkewMillis)
			if err != nil {
				return nil, err
			}
			body, err := encodeTerminalCandidateSealedBodyV1(projectID, input.Sealed.Fact)
			if err != nil {
				return nil, syncProblem(SyncErrorInvalid, "fact", "does not have canonical persisted bytes")
			}
			fact := sealed[0].fact
			entry.sealedFact = &fact
			entry.normalized = terminalCandidateFrameV1{
				projectID:              projectID,
				arrivalSequence:        sealed[0].arrival,
				frameKind:              terminalCandidateFrameKindSealedV1,
				factID:                 fact.factID,
				environmentID:          fact.environmentID,
				environmentSequence:    fact.environmentSequence,
				clock:                  fact.clock,
				previousEnvelopeDigest: sealed[0].previousDigest,
				envelopeDigest:         sealed[0].digest,
				certificateID:          sealed[0].certificateID,
				keyGeneration:          sealed[0].keyGeneration,
				nonce:                  sealed[0].nonce,
				candidateBytes:         append([]byte(nil), body...),
			}
		} else {
			if frameKind != terminalCandidateFrameKindPrunedV1 || input.Inbox.ArrivalSequence != input.Pruned.Reference.ArrivalSequence || input.Inbox.EnvelopeDigest != input.Pruned.Reference.EnvelopeDigest {
				return nil, syncProblem(SyncErrorConflict, "inbox", "does not match the verified pruned frame")
			}
			if err := validateVerifiedPruneReferenceV1(input.Pruned.Reference, "pruned.reference"); err != nil {
				return nil, err
			}
			if input.Pruned.PruneCertificateID == ([32]byte{}) {
				return nil, syncProblem(SyncErrorInvalid, "prune_certificate_id", "must be nonzero")
			}
			referenceDigest, err := continuitywire.PruneReferenceDigest(continuitywire.PruneReference{
				FactID:                 input.Pruned.Reference.FactID,
				EnvironmentID:          input.Pruned.Reference.EnvironmentID,
				EnvironmentSequence:    input.Pruned.Reference.EnvironmentSequence,
				ArrivalSequence:        input.Pruned.Reference.ArrivalSequence,
				EnvelopeDigest:         input.Pruned.Reference.EnvelopeDigest,
				CertificateID:          input.Pruned.Reference.CertificateID,
				PreviousEnvelopeDigest: input.Pruned.Reference.PreviousEnvelopeDigest,
				KeyGeneration:          input.Pruned.Reference.KeyGeneration,
				Nonce:                  input.Pruned.Reference.Nonce,
			})
			if err != nil {
				return nil, syncProblem(SyncErrorInvalid, "pruned.reference", "is invalid")
			}
			body, err := encodeTerminalCandidatePrunedBodyV1(terminalCandidatePrunedBodyV1{
				ReferenceDigest:  referenceDigest,
				InboxFrameDigest: sha256.Sum256(inboxBytes),
				FactKind:         input.Pruned.FactKind,
				Clock:            input.Pruned.HLC,
			})
			if err != nil {
				return nil, syncProblem(SyncErrorInvalid, "pruned", "does not have canonical anchor fields")
			}
			pruneCertificateID := input.Pruned.PruneCertificateID
			entry.normalized = terminalCandidateFrameV1{
				projectID:              projectID,
				arrivalSequence:        input.Pruned.Reference.ArrivalSequence,
				frameKind:              terminalCandidateFrameKindPrunedV1,
				factID:                 input.Pruned.Reference.FactID,
				environmentID:          input.Pruned.Reference.EnvironmentID,
				environmentSequence:    input.Pruned.Reference.EnvironmentSequence,
				clock:                  input.Pruned.HLC,
				previousEnvelopeDigest: input.Pruned.Reference.PreviousEnvelopeDigest,
				envelopeDigest:         input.Pruned.Reference.EnvelopeDigest,
				certificateID:          input.Pruned.Reference.CertificateID,
				keyGeneration:          input.Pruned.Reference.KeyGeneration,
				nonce:                  input.Pruned.Reference.Nonce,
				pruneCertificateID:     &pruneCertificateID,
				candidateBytes:         append([]byte(nil), body...),
			}
		}
		if index > 0 {
			previous := prepared[index-1].normalized.arrivalSequence
			if previous == math.MaxInt64 || entry.normalized.arrivalSequence != previous+1 {
				return nil, syncProblem(SyncErrorArrivalGap, "arrival_sequence", "chunk is not contiguous")
			}
		}
		if futureSkewedV1(entry.normalized.clock.WallMillis, trustedNowMillis, maxFutureSkewMillis) {
			return nil, syncProblem(SyncErrorHLC, "", "")
		}
		if _, duplicate := seenFacts[entry.normalized.factID]; duplicate {
			return nil, syncProblem(SyncErrorConflict, "fact_id", "appears more than once in the chunk")
		}
		seenFacts[entry.normalized.factID] = struct{}{}
		sourceKey := environmentSequenceKeyV1(entry.normalized.environmentID, entry.normalized.environmentSequence)
		if _, duplicate := seenSources[sourceKey]; duplicate {
			return nil, syncProblem(SyncErrorConflict, "environment_sequence", "appears more than once in the chunk")
		}
		seenSources[sourceKey] = struct{}{}
		nonceKey := generationNonceKeyV1(entry.normalized.keyGeneration, entry.normalized.nonce)
		if _, duplicate := seenNonces[nonceKey]; duplicate {
			return nil, syncProblem(SyncErrorNonceReuse, "nonce", "appears more than once in the chunk")
		}
		seenNonces[nonceKey] = struct{}{}
		if err := budget.admit(len(entry.normalized.candidateBytes), len(inboxBytes)); err != nil {
			return nil, syncProblem(SyncErrorInvalid, "frames", "exceeds a terminal chunk bound")
		}
		prepared = append(prepared, entry)
	}
	return prepared, nil
}

func cloneOpaqueSyncFrameV1(frame OpaqueSyncFrame) OpaqueSyncFrame {
	frame.SealedEnvelope = append([]byte(nil), frame.SealedEnvelope...)
	frame.PrunedArrival = append([]byte(nil), frame.PrunedArrival...)
	return frame
}

func terminalCandidateHeaderMatchesAuthorityBindingV2(candidate TerminalCandidate, binding SyncAuthorityBinding, candidateID [32]byte) bool {
	// The frozen terminal identity treats AuthorityDigest as an opaque commitment.
	// Every supported future authority digest must remain domain-separated and
	// commit the complete epoch before it can be accepted here.
	return candidate.CandidateID == candidateID && candidate.ChannelID == binding.ChannelID &&
		candidate.RelayGeneration == binding.RelayGeneration &&
		candidate.MembershipGeneration == binding.MembershipGeneration && candidate.AuthorityDigest == binding.AuthorityDigest
}

func readActiveTerminalCandidateV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (TerminalCandidate, bool, error) {
	var candidateID, channelID, relayGeneration, authorityDigest, rollingDigest []byte
	var membershipGeneration int64
	candidate := TerminalCandidate{ProjectID: projectID}
	err := tx.QueryRowContext(ctx, `
SELECT candidate_id, channel_id, relay_generation, membership_generation,
       authority_digest, start_arrival_sequence, through_arrival_sequence,
       frame_count, rolling_candidate_digest
FROM continuity_sync_terminal_candidates
WHERE project_id = ? AND state = 'staging'`, string(projectID)).Scan(
		&candidateID, &channelID, &relayGeneration, &membershipGeneration,
		&authorityDigest, &candidate.StartArrivalSequence, &candidate.ThroughArrivalSequence,
		&candidate.FrameCount, &rollingDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TerminalCandidate{}, false, nil
	}
	if err != nil {
		return TerminalCandidate{}, false, syncTransactionProblem(ctx)
	}
	if len(candidateID) != 32 || len(channelID) != 32 || len(relayGeneration) != 32 || len(authorityDigest) != 32 || len(rollingDigest) != 32 ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 || candidate.StartArrivalSequence < 1 ||
		candidate.ThroughArrivalSequence < candidate.StartArrivalSequence || candidate.FrameCount < 1 ||
		candidate.ThroughArrivalSequence-candidate.StartArrivalSequence == math.MaxInt64 ||
		candidate.FrameCount != candidate.ThroughArrivalSequence-candidate.StartArrivalSequence+1 {
		return TerminalCandidate{}, false, syncProblem(SyncErrorStore, "", "active terminal candidate header is corrupt")
	}
	copy(candidate.CandidateID[:], candidateID)
	copy(candidate.ChannelID[:], channelID)
	copy(candidate.RelayGeneration[:], relayGeneration)
	copy(candidate.AuthorityDigest[:], authorityDigest)
	copy(candidate.RollingCandidateDigest[:], rollingDigest)
	candidate.MembershipGeneration = uint32(membershipGeneration)
	if candidate.CandidateID == ([32]byte{}) || candidate.ChannelID == (SyncChannelID{}) ||
		candidate.RelayGeneration == ([32]byte{}) || candidate.AuthorityDigest == ([32]byte{}) ||
		candidate.RollingCandidateDigest == ([32]byte{}) {
		return TerminalCandidate{}, false, syncProblem(SyncErrorStore, "", "active terminal candidate header is corrupt")
	}
	return candidate, true, nil
}

func activeTerminalCandidateExistsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (bool, error) {
	var active int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_terminal_candidates
  WHERE project_id = ? AND state = 'staging'
)`, string(projectID)).Scan(&active); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return active != 0, nil
}

func requireExactTerminalCandidateInboxV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, prepared preparedTerminalCandidateFrameV1) error {
	var digest, frameBytes []byte
	var frameKind, state string
	err := tx.QueryRowContext(ctx, `
SELECT envelope_digest, frame_kind, frame_bytes, state
FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence = ?`, string(projectID), prepared.normalized.arrivalSequence).Scan(&digest, &frameKind, &frameBytes, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return syncProblem(SyncErrorArrivalGap, "arrival_sequence", "has no staged arrival")
	}
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	retained, err := opaqueSyncFrameFromColumnsV1(prepared.normalized.arrivalSequence, digest, frameKind, frameBytes, state)
	if err != nil {
		return err
	}
	if retained.Quarantined {
		return syncProblem(SyncErrorConflict, "inbox", "arrival is quarantined")
	}
	if !terminalCandidateOpaqueFrameEqualV1(retained, prepared.inbox) {
		return syncProblem(SyncErrorConflict, "inbox", "does not match the retained staged arrival")
	}
	if err := validateTerminalCandidateInboxBindingV1(prepared.normalized, retained); err != nil {
		return err
	}
	return nil
}

func validateTerminalCandidateInboxBindingV1(frame terminalCandidateFrameV1, inbox OpaqueSyncFrame) error {
	frameKind, frameBytes, err := opaqueSyncFrameStorageV1(inbox)
	if err != nil {
		return err
	}
	if frameKind != frame.frameKind || inbox.ArrivalSequence != frame.arrivalSequence || inbox.EnvelopeDigest != frame.envelopeDigest {
		return syncProblem(SyncErrorConflict, "inbox", "does not match the normalized terminal frame")
	}
	if frameKind != terminalCandidateFrameKindPrunedV1 {
		return nil
	}
	body, err := decodeTerminalCandidatePrunedBodyV1(frame.candidateBytes)
	if err != nil || body.InboxFrameDigest != sha256.Sum256(frameBytes) {
		return syncProblem(SyncErrorStore, "", "terminal candidate inbox binding is corrupt")
	}
	return nil
}

func terminalCandidateOpaqueFrameEqualV1(left, right OpaqueSyncFrame) bool {
	return left.ArrivalSequence == right.ArrivalSequence && left.EnvelopeDigest == right.EnvelopeDigest &&
		left.Quarantined == right.Quarantined && bytes.Equal(left.SealedEnvelope, right.SealedEnvelope) &&
		bytes.Equal(left.PrunedArrival, right.PrunedArrival)
}

func requireExactTerminalCandidateReplayV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, prepared []preparedTerminalCandidateFrameV1) error {
	for _, expected := range prepared {
		retained, found, err := readTerminalCandidateFrameV1(ctx, tx, projectID, candidateID, expected.normalized.arrivalSequence)
		if err != nil {
			return err
		}
		if !found || !terminalCandidateFramesEqualV1(retained, expected.normalized) {
			return syncProblem(SyncErrorConflict, "frame", "replay differs from the retained candidate")
		}
	}
	return nil
}

func readTerminalCandidateFrameV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, arrival int64) (terminalCandidateFrameV1, bool, error) {
	frame := terminalCandidateFrameV1{projectID: projectID, candidateID: candidateID, arrivalSequence: arrival}
	var previousDigest, envelopeDigest, certificateID, nonce, pruneCertificateID, candidateBytes []byte
	var environmentSequence, wall, logical, keyGeneration int64
	err := tx.QueryRowContext(ctx, `
SELECT frame_kind, fact_id, environment_id, environment_sequence,
       hlc_wall_millis, hlc_logical, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce,
       prune_certificate_id, candidate_bytes
FROM continuity_sync_terminal_candidate_frames
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = ?`,
		string(projectID), candidateID[:], arrival).Scan(
		&frame.frameKind, &frame.factID, &frame.environmentID, &environmentSequence,
		&wall, &logical, &previousDigest, &envelopeDigest, &certificateID,
		&keyGeneration, &nonce, &pruneCertificateID, &candidateBytes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return terminalCandidateFrameV1{}, false, nil
	}
	if err != nil {
		return terminalCandidateFrameV1{}, false, syncTransactionProblem(ctx)
	}
	if environmentSequence < 1 || logical < 0 || logical > math.MaxInt32 || keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
		len(previousDigest) != 32 || len(envelopeDigest) != 32 || len(certificateID) != 32 || len(nonce) != 24 {
		return terminalCandidateFrameV1{}, false, syncProblem(SyncErrorStore, "", "terminal candidate frame is corrupt")
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
			return terminalCandidateFrameV1{}, false, syncProblem(SyncErrorStore, "", "terminal candidate frame is corrupt")
		}
		pruneID := [32]byte{}
		copy(pruneID[:], pruneCertificateID)
		frame.pruneCertificateID = &pruneID
	}
	frame.candidateBytes = append([]byte(nil), candidateBytes...)
	if _, err := terminalCandidateFrameDigestV1(frame); err != nil {
		return terminalCandidateFrameV1{}, false, syncProblem(SyncErrorStore, "", "terminal candidate frame is corrupt")
	}
	return frame, true, nil
}

func terminalCandidateFramesEqualV1(left, right terminalCandidateFrameV1) bool {
	if left.projectID != right.projectID || left.candidateID != right.candidateID || left.arrivalSequence != right.arrivalSequence ||
		left.frameKind != right.frameKind || left.factID != right.factID || left.environmentID != right.environmentID ||
		left.environmentSequence != right.environmentSequence || left.clock != right.clock ||
		left.previousEnvelopeDigest != right.previousEnvelopeDigest || left.envelopeDigest != right.envelopeDigest ||
		left.certificateID != right.certificateID || left.keyGeneration != right.keyGeneration || left.nonce != right.nonce ||
		(left.pruneCertificateID == nil) != (right.pruneCertificateID == nil) || !bytes.Equal(left.candidateBytes, right.candidateBytes) {
		return false
	}
	return left.pruneCertificateID == nil || *left.pruneCertificateID == *right.pruneCertificateID
}

func validateTerminalCandidateRetirementFenceV1(environment SyncEnvironmentCertificate, frame terminalCandidateFrameV1) error {
	if environment.Retirement == nil {
		return nil
	}
	fence := environment.Retirement
	if frame.environmentSequence > fence.FinalEnvironmentSequence {
		return syncProblem(SyncErrorCertificate, "environment_sequence", "exceeds the authenticated retirement fence")
	}
	if frame.environmentSequence == fence.FinalEnvironmentSequence && frame.envelopeDigest != fence.FinalEnvelopeDigest {
		return syncProblem(SyncErrorCertificate, "envelope_digest", "does not match the authenticated retirement fence")
	}
	return nil
}

func terminalCandidateFirstSeenSourceV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, frame terminalCandidateFrameV1) (bool, error) {
	_, found, err := readCanonicalTerminalSourceV1(ctx, tx, projectID, frame.environmentID, frame.environmentSequence)
	return !found, err
}

func readCanonicalTerminalSourceV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, environmentID continuity.EnvironmentID, sequence int64) (terminalCandidateSourceV1, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_receipts
WHERE project_id = ? AND environment_id = ? AND environment_sequence = ?
UNION ALL
SELECT fact_id, previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_outbox
WHERE project_id = ? AND environment_id = ? AND environment_sequence = ?
UNION ALL
SELECT fact_id, previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_tombstones
WHERE project_id = ? AND environment_id = ? AND environment_sequence = ?`,
		string(projectID), string(environmentID), sequence,
		string(projectID), string(environmentID), sequence,
		string(projectID), string(environmentID), sequence)
	if err != nil {
		return terminalCandidateSourceV1{}, false, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	var retained terminalCandidateSourceV1
	found := false
	for rows.Next() {
		var previousDigest, envelopeDigest, certificateID, nonce []byte
		var keyGeneration int64
		current := terminalCandidateSourceV1{projectID: projectID, environmentID: environmentID, environmentSequence: sequence}
		if err := rows.Scan(&current.factID, &previousDigest, &envelopeDigest, &certificateID, &keyGeneration, &nonce); err != nil {
			return terminalCandidateSourceV1{}, false, syncTransactionProblem(ctx)
		}
		if len(previousDigest) != 32 || len(envelopeDigest) != 32 || len(certificateID) != 32 || len(nonce) != 24 || keyGeneration < 1 || keyGeneration > math.MaxUint32 {
			return terminalCandidateSourceV1{}, false, syncProblem(SyncErrorStore, "", "canonical envelope identity is corrupt")
		}
		copy(current.metadata.previousDigest[:], previousDigest)
		copy(current.metadata.digest[:], envelopeDigest)
		copy(current.metadata.certificateID[:], certificateID)
		current.metadata.keyGeneration = uint32(keyGeneration)
		copy(current.metadata.nonce[:], nonce)
		if found && !terminalCandidateSourcesEqualV1(retained, current) {
			return terminalCandidateSourceV1{}, false, syncProblem(SyncErrorStore, "", "canonical envelope identities disagree")
		}
		retained, found = current, true
	}
	if err := rows.Err(); err != nil {
		return terminalCandidateSourceV1{}, false, syncTransactionProblem(ctx)
	}
	return retained, found, nil
}

func terminalCandidateSourcesEqualV1(left, right terminalCandidateSourceV1) bool {
	return left.projectID == right.projectID && left.factID == right.factID && left.environmentID == right.environmentID &&
		left.environmentSequence == right.environmentSequence && sealedMetadataEqualV1(left.metadata, right.metadata)
}

func validateTerminalCandidateCollisionsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, prepared preparedTerminalCandidateFrameV1) (bool, error) {
	frame := prepared.normalized
	canonical, found, err := readCanonicalTerminalSourceV1(ctx, tx, projectID, frame.environmentID, frame.environmentSequence)
	if err != nil {
		return false, err
	}
	if found && (canonical.factID != frame.factID || !sealedMetadataEqualV1(canonical.metadata, terminalCandidateMetadataV1(frame))) {
		return false, syncProblem(SyncErrorConflict, "environment_sequence", "conflicts with a canonical envelope identity")
	}
	if err := rejectTerminalCandidateFactCollisionV1(ctx, tx, projectID, candidateID, prepared); err != nil {
		return false, err
	}
	if err := rejectTerminalCandidateNonceCollisionV1(ctx, tx, projectID, candidateID, frame); err != nil {
		return false, err
	}
	return found && prepared.sealedFact != nil, nil
}

func rejectTerminalCandidateFactCollisionV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, prepared preparedTerminalCandidateFrameV1) error {
	frame := prepared.normalized
	fact, found, err := readFactByIDV1(ctx, tx, frame.factID)
	if err != nil {
		return err
	}
	if found {
		if prepared.sealedFact == nil || fact.projectID != projectID || !storedFactsEqualV1(fact, *prepared.sealedFact) {
			return syncProblem(SyncErrorConflict, "fact_id", "is bound to another canonical fact")
		}
	}
	var candidateCollision int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_terminal_candidate_frames
  WHERE project_id = ? AND candidate_id = ? AND fact_id = ?
)`, string(projectID), candidateID[:], string(frame.factID)).Scan(&candidateCollision); err != nil {
		return syncTransactionProblem(ctx)
	}
	if candidateCollision != 0 {
		return syncProblem(SyncErrorConflict, "fact_id", "is already in the active candidate")
	}
	rows, err := tx.QueryContext(ctx, `
SELECT project_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_receipts WHERE project_id = ? AND fact_id = ?
UNION ALL
SELECT project_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_outbox WHERE project_id = ? AND fact_id = ?
UNION ALL
SELECT project_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_tombstones WHERE fact_id = ?`,
		string(projectID), string(frame.factID),
		string(projectID), string(frame.factID),
		string(frame.factID))
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()
	for rows.Next() {
		var source terminalCandidateSourceV1
		var previousDigest, envelopeDigest, certificateID, nonce []byte
		var keyGeneration int64
		if err := rows.Scan(&source.projectID, &source.environmentID, &source.environmentSequence, &previousDigest, &envelopeDigest, &certificateID, &keyGeneration, &nonce); err != nil {
			return syncTransactionProblem(ctx)
		}
		if len(previousDigest) != 32 || len(envelopeDigest) != 32 || len(certificateID) != 32 || len(nonce) != 24 || keyGeneration < 1 || keyGeneration > math.MaxUint32 {
			return syncProblem(SyncErrorStore, "", "canonical fact identity is corrupt")
		}
		source.factID = frame.factID
		copy(source.metadata.previousDigest[:], previousDigest)
		copy(source.metadata.digest[:], envelopeDigest)
		copy(source.metadata.certificateID[:], certificateID)
		source.metadata.keyGeneration = uint32(keyGeneration)
		copy(source.metadata.nonce[:], nonce)
		if source.projectID != projectID || source.environmentID != frame.environmentID || source.environmentSequence != frame.environmentSequence ||
			!sealedMetadataEqualV1(source.metadata, terminalCandidateMetadataV1(frame)) {
			return syncProblem(SyncErrorConflict, "fact_id", "is bound to another envelope identity")
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func rejectTerminalCandidateNonceCollisionV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, frame terminalCandidateFrameV1) error {
	var candidateCollision int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_terminal_candidate_frames
  WHERE project_id = ? AND candidate_id = ? AND key_generation = ? AND nonce = ?
)`, string(projectID), candidateID[:], frame.keyGeneration, frame.nonce[:]).Scan(&candidateCollision); err != nil {
		return syncTransactionProblem(ctx)
	}
	if candidateCollision != 0 {
		return syncProblem(SyncErrorNonceReuse, "nonce", "is already in the active candidate")
	}
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id
FROM continuity_sync_receipts WHERE project_id = ? AND key_generation = ? AND nonce = ?
UNION ALL
SELECT fact_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id
FROM continuity_sync_outbox WHERE project_id = ? AND key_generation = ? AND nonce = ?
UNION ALL
SELECT fact_id, environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id
FROM continuity_sync_tombstones WHERE project_id = ? AND key_generation = ? AND nonce = ?`,
		string(projectID), frame.keyGeneration, frame.nonce[:],
		string(projectID), frame.keyGeneration, frame.nonce[:],
		string(projectID), frame.keyGeneration, frame.nonce[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()
	for rows.Next() {
		var factID continuity.FactID
		var environmentID continuity.EnvironmentID
		var sequence int64
		var previousDigest, envelopeDigest, certificateID []byte
		if err := rows.Scan(&factID, &environmentID, &sequence, &previousDigest, &envelopeDigest, &certificateID); err != nil {
			return syncTransactionProblem(ctx)
		}
		metadata := terminalCandidateMetadataV1(frame)
		if len(previousDigest) != 32 || len(envelopeDigest) != 32 || len(certificateID) != 32 || factID != frame.factID || environmentID != frame.environmentID || sequence != frame.environmentSequence ||
			!bytes.Equal(previousDigest, metadata.previousDigest[:]) || !bytes.Equal(envelopeDigest, metadata.digest[:]) || !bytes.Equal(certificateID, metadata.certificateID[:]) {
			return syncProblem(SyncErrorNonceReuse, "nonce", "is bound to another envelope identity")
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func readTerminalCandidateFrontierV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, environmentID continuity.EnvironmentID) (terminalCandidateFrontierV1, bool, error) {
	var frontier terminalCandidateFrontierV1
	var highestSequence, wall, logical int64
	err := tx.QueryRowContext(ctx, `
SELECT highest_sequence, hlc_wall_millis, hlc_logical
FROM continuity_sync_environment_heads
WHERE project_id = ? AND environment_id = ?`, string(projectID), string(environmentID)).Scan(&highestSequence, &wall, &logical)
	found := false
	if err == nil {
		if highestSequence < 1 || logical < 0 || logical > math.MaxInt32 {
			return terminalCandidateFrontierV1{}, false, syncProblem(SyncErrorStore, "", "environment head is corrupt")
		}
		source, sourceFound, err := readCanonicalTerminalSourceV1(ctx, tx, projectID, environmentID, highestSequence)
		if err != nil {
			return terminalCandidateFrontierV1{}, false, err
		}
		if !sourceFound {
			return terminalCandidateFrontierV1{}, false, syncProblem(SyncErrorStore, "", "environment head has no canonical envelope identity")
		}
		frontier = terminalCandidateFrontierV1{sequence: highestSequence, clock: continuity.HybridTime{WallMillis: wall, Logical: int32(logical)}, metadata: source.metadata}
		found = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return terminalCandidateFrontierV1{}, false, syncTransactionProblem(ctx)
	}
	var candidateSequence, candidateWall, candidateLogical int64
	var previousDigest, envelopeDigest, certificateID, nonce []byte
	var keyGeneration int64
	err = tx.QueryRowContext(ctx, `
SELECT environment_sequence, hlc_wall_millis, hlc_logical,
       previous_envelope_digest, envelope_digest, certificate_id,
       key_generation, nonce
FROM continuity_sync_terminal_candidate_frames
WHERE project_id = ? AND candidate_id = ? AND environment_id = ?
ORDER BY environment_sequence DESC
LIMIT 1`, string(projectID), candidateID[:], string(environmentID)).Scan(
		&candidateSequence, &candidateWall, &candidateLogical, &previousDigest,
		&envelopeDigest, &certificateID, &keyGeneration, &nonce)
	if err == nil {
		if candidateSequence < 1 || candidateLogical < 0 || candidateLogical > math.MaxInt32 || keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
			len(previousDigest) != 32 || len(envelopeDigest) != 32 || len(certificateID) != 32 || len(nonce) != 24 {
			return terminalCandidateFrontierV1{}, false, syncProblem(SyncErrorStore, "", "candidate environment frontier is corrupt")
		}
		candidate := terminalCandidateFrontierV1{sequence: candidateSequence, clock: continuity.HybridTime{WallMillis: candidateWall, Logical: int32(candidateLogical)}}
		copy(candidate.metadata.previousDigest[:], previousDigest)
		copy(candidate.metadata.digest[:], envelopeDigest)
		copy(candidate.metadata.certificateID[:], certificateID)
		candidate.metadata.keyGeneration = uint32(keyGeneration)
		copy(candidate.metadata.nonce[:], nonce)
		if !found || candidate.sequence > frontier.sequence {
			frontier, found = candidate, true
		} else if candidate.sequence == frontier.sequence && (candidate.clock != frontier.clock || !sealedMetadataEqualV1(candidate.metadata, frontier.metadata)) {
			return terminalCandidateFrontierV1{}, false, syncProblem(SyncErrorStore, "", "canonical and candidate frontiers disagree")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return terminalCandidateFrontierV1{}, false, syncTransactionProblem(ctx)
	}
	return frontier, found, nil
}

func advanceTerminalCandidateFrontierV1(frontier terminalCandidateFrontierV1, frame terminalCandidateFrameV1) (terminalCandidateFrontierV1, error) {
	metadata := terminalCandidateMetadataV1(frame)
	if frontier.sequence == 0 {
		if frame.environmentSequence != 1 || frame.previousEnvelopeDigest != ([32]byte{}) {
			return terminalCandidateFrontierV1{}, syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "does not begin the environment chain")
		}
		return terminalCandidateFrontierV1{sequence: 1, clock: frame.clock, metadata: metadata}, nil
	}
	if frame.environmentSequence <= frontier.sequence {
		if frame.environmentSequence != frontier.sequence || frame.clock != frontier.clock || !sealedMetadataEqualV1(metadata, frontier.metadata) {
			return terminalCandidateFrontierV1{}, syncProblem(SyncErrorConflict, "environment_sequence", "does not match the retained environment frontier")
		}
		return frontier, nil
	}
	if frontier.sequence == math.MaxInt64 || frame.environmentSequence != frontier.sequence+1 {
		return terminalCandidateFrontierV1{}, syncProblem(SyncErrorEnvironmentGap, "environment_sequence", "does not extend the environment frontier")
	}
	if frame.previousEnvelopeDigest != frontier.metadata.digest {
		return terminalCandidateFrontierV1{}, syncProblem(SyncErrorEnvelopeChain, "previous_envelope_digest", "does not match the environment frontier")
	}
	if !hybridTimeLessV1(frontier.clock, frame.clock) {
		return terminalCandidateFrontierV1{}, syncProblem(SyncErrorHLC, "hlc", "does not increase with environment sequence")
	}
	return terminalCandidateFrontierV1{sequence: frame.environmentSequence, clock: frame.clock, metadata: metadata}, nil
}

func terminalCandidateMetadataV1(frame terminalCandidateFrameV1) sealedEnvelopeMetadataV1 {
	return sealedEnvelopeMetadataV1{
		previousDigest: frame.previousEnvelopeDigest,
		digest:         frame.envelopeDigest,
		certificateID:  frame.certificateID,
		keyGeneration:  frame.keyGeneration,
		nonce:          frame.nonce,
	}
}

func insertTerminalCandidateHeaderV1(ctx context.Context, tx *sql.Tx, candidate TerminalCandidate) error {
	_, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES(?, ?, 'staging', ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
		string(candidate.ProjectID), candidate.CandidateID[:], candidate.ChannelID[:], candidate.RelayGeneration[:],
		candidate.MembershipGeneration, candidate.AuthorityDigest[:], candidate.StartArrivalSequence,
		candidate.ThroughArrivalSequence, candidate.FrameCount, candidate.RollingCandidateDigest[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func insertTerminalCandidateFrameV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, frame terminalCandidateFrameV1) error {
	var pruneCertificateID any
	if frame.pruneCertificateID != nil {
		pruneCertificateID = frame.pruneCertificateID[:]
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, prune_certificate_id, candidate_bytes
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), candidateID[:], frame.arrivalSequence, frame.frameKind, string(frame.factID),
		string(frame.environmentID), frame.environmentSequence, frame.clock.WallMillis, frame.clock.Logical,
		frame.previousEnvelopeDigest[:], frame.envelopeDigest[:], frame.certificateID[:], frame.keyGeneration,
		frame.nonce[:], pruneCertificateID, frame.candidateBytes)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func compareAndSwapTerminalCandidateHeaderV1(ctx context.Context, tx *sql.Tx, current, next TerminalCandidate) error {
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_terminal_candidates
SET through_arrival_sequence = ?, frame_count = ?, rolling_candidate_digest = ?
WHERE project_id = ? AND candidate_id = ? AND state = 'staging'
  AND through_arrival_sequence = ? AND frame_count = ?
  AND rolling_candidate_digest = ?`,
		next.ThroughArrivalSequence, next.FrameCount, next.RollingCandidateDigest[:],
		string(current.ProjectID), current.CandidateID[:], current.ThroughArrivalSequence,
		current.FrameCount, current.RollingCandidateDigest[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "checkpoint", "active candidate changed")
	}
	return nil
}

func validateTerminalCandidateCheckpointV1(checkpoint TerminalCandidateCheckpoint) error {
	if checkpoint.CandidateID == ([32]byte{}) || checkpoint.ThroughArrivalSequence < 1 || checkpoint.FrameCount < 1 || checkpoint.RollingCandidateDigest == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, "checkpoint", "is invalid")
	}
	return nil
}
