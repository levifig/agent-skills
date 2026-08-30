package sqlite

import (
	"context"
	"crypto/sha256"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestTerminalCandidateLifecycleCreatesReadsReplaysAndDiscards(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-candidate-lifecycle")
	projectID := continuity.ProjectID("project-terminal-candidate-lifecycle")
	fact := syncProjectFact(t, projectID, "fact-terminal-candidate-root", "environment-a", 1, 100)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", 1, verified[0].EnvelopeDigest)
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	inbox, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 1)
	if err != nil {
		t.Fatalf("PendingSyncFramesAfter() error = %v", err)
	}
	frame := VerifiedTerminalCandidateFrame{Inbox: inbox[0], Sealed: &verified[0]}

	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{frame}, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(create) error = %v", err)
	}
	if candidate.ProjectID != projectID || candidate.StartArrivalSequence != 1 || candidate.ThroughArrivalSequence != 1 || candidate.FrameCount != 1 {
		t.Fatalf("candidate = %#v", candidate)
	}

	current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate() = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, candidate)
	}
	replayed, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{frame}, 1_000, 100)
	if err != nil || replayed != candidate {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(replay) = (%#v, %v), want (%#v, nil)", replayed, err, candidate)
	}

	checkpoint := TerminalCandidateCheckpoint{
		CandidateID:            candidate.CandidateID,
		ThroughArrivalSequence: candidate.ThroughArrivalSequence,
		FrameCount:             candidate.FrameCount,
		RollingCandidateDigest: candidate.RollingCandidateDigest,
	}
	wrongCheckpoint := checkpoint
	wrongCheckpoint.RollingCandidateDigest[0] ^= 0xff
	if err := store.DiscardTerminalCandidate(context.Background(), projectID, wrongCheckpoint); err == nil {
		t.Fatal("DiscardTerminalCandidate(wrong checkpoint) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	if err := store.DiscardTerminalCandidate(context.Background(), projectID, checkpoint); err != nil {
		t.Fatalf("DiscardTerminalCandidate() error = %v", err)
	}
	if _, found, err := store.CurrentTerminalCandidate(context.Background(), projectID); err != nil || found {
		t.Fatalf("CurrentTerminalCandidate(after discard) = (_, %v, %v), want (_, false, nil)", found, err)
	}
	if err := store.DiscardTerminalCandidate(context.Background(), projectID, checkpoint); err != nil {
		t.Fatalf("DiscardTerminalCandidate(retry) error = %v", err)
	}
	retained, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 1)
	if err != nil || len(retained) != 1 || !opaqueSyncFrameEqual(retained[0], inbox[0]) {
		t.Fatalf("PendingSyncFramesAfter(after discard) = (%#v, %v), want exact inbox", retained, err)
	}
}

func TestTerminalCandidateReplayOverlapAndAppendRollback(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-append-validation", 3)
	first, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames[:1], 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(create) error = %v", err)
	}

	alteredReplay := frames[0]
	sealed := *alteredReplay.Sealed
	sealed.Fact.HLCLogical++
	alteredReplay.Sealed = &sealed
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{alteredReplay}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames[:2], 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	alteredPrunedBytes := cloneTerminalCandidateInputV1(frames[1])
	alteredPrunedBytes.Inbox.PrunedArrival = append(alteredPrunedBytes.Inbox.PrunedArrival, 'x')
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{alteredPrunedBytes}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)

	badChain := cloneTerminalCandidateInputV1(frames[1])
	badChain.Pruned.Reference.PreviousEnvelopeDigest = sha256.Sum256([]byte("wrong-previous"))
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{badChain}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorEnvelopeChain)

	badHLC := cloneTerminalCandidateInputV1(frames[1])
	badHLC.Pruned.HLC = continuity.HybridTime{
		WallMillis: frames[0].Sealed.Fact.HLCWallMillis,
		Logical:    frames[0].Sealed.Fact.HLCLogical,
	}
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{badHLC}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorHLC)

	badFact := cloneTerminalCandidateInputV1(frames[1])
	badFact.Pruned.Reference.FactID = frames[0].Sealed.Fact.FactID
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{badFact}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)

	badNonce := cloneTerminalCandidateInputV1(frames[1])
	badNonce.Pruned.Reference.Nonce = frames[0].Sealed.Nonce
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{badNonce}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorNonceReuse)

	rollbackChunk := []VerifiedTerminalCandidateFrame{
		cloneTerminalCandidateInputV1(frames[1]),
		cloneTerminalCandidateInputV1(frames[2]),
	}
	rollbackChunk[1].Pruned.HLC = rollbackChunk[0].Pruned.HLC
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, rollbackChunk, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorHLC)
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != first {
		t.Fatalf("candidate after failed append = (%#v, %v, %v), want unchanged %#v", current, found, currentErr, first)
	}
	var leaked int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames WHERE project_id = ? AND arrival_sequence > 1`, string(projectID)).Scan(&leaked); err != nil {
		t.Fatalf("count failed append rows: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("failed append retained %d child rows", leaked)
	}
	completed, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames[1:], 1_000, 100)
	if err != nil || completed.FrameCount != 3 {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(valid append) = (%#v, %v)", completed, err)
	}
}

func TestTerminalCandidatePrunedInboxBindingPersistsExactBytesV1(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-pruned-inbox-binding", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	var candidateBytes []byte
	if err := store.db.QueryRow(`
SELECT candidate_bytes
FROM continuity_sync_terminal_candidate_frames
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = 2`, string(projectID), candidate.CandidateID[:]).Scan(&candidateBytes); err != nil {
		t.Fatalf("read retained pruned candidate body: %v", err)
	}
	body, err := decodeTerminalCandidatePrunedBodyV1(candidateBytes)
	if err != nil {
		t.Fatalf("decode retained pruned candidate body: %v", err)
	}
	if body.InboxFrameDigest != sha256.Sum256(frames[1].Inbox.PrunedArrival) {
		t.Fatal("retained pruned candidate body does not bind exact inbox bytes")
	}

	mutated := cloneTerminalCandidateInputV1(frames[1])
	mutated.Inbox.PrunedArrival = append(mutated.Inbox.PrunedArrival, 0)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_inbox
SET frame_bytes = ?
WHERE project_id = ? AND arrival_sequence = 2`, mutated.Inbox.PrunedArrival, string(projectID)); err != nil {
		t.Fatalf("mutate retained pruned inbox fixture: %v", err)
	}
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{mutated}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("candidate after altered exact inbox replay = (%#v, %v, %v), want unchanged %#v", current, found, currentErr, candidate)
	}

	legacyBody, err := encodeTerminalCandidateTranscriptV1(
		terminalCandidatePrunedBodyDomainV1,
		maximumTerminalCandidatePrunedBodyBytesV1,
		terminalCandidateUint16BytesV1(1),
		body.ReferenceDigest[:],
		[]byte(body.FactKind),
		terminalCandidateInt64BytesV1(body.Clock.WallMillis),
		terminalCandidateInt32BytesV1(body.Clock.Logical),
	)
	if err != nil {
		t.Fatalf("encode prior pruned candidate body: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_terminal_candidate_frames
SET candidate_bytes = ?
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = 2`, legacyBody, string(projectID), candidate.CandidateID[:]); err != nil {
		t.Fatalf("install prior pruned candidate body fixture: %v", err)
	}
	current, found, currentErr = store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(prior body) = (%#v, %v, %v), want readable %#v", current, found, currentErr, candidate)
	}
	if err := store.DiscardTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate)); err != nil {
		t.Fatalf("DiscardTerminalCandidate(prior body) error = %v", err)
	}
}

func TestTerminalCandidateBoundsAndFutureSkewAreAtomic(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-bounds", 2)
	_, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, nil, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	tooMany := make([]VerifiedTerminalCandidateFrame, 17)
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, tooMany, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	oversized := cloneTerminalCandidateInputV1(frames[1])
	oversized.Inbox.PrunedArrival = make([]byte, maximumPrunedArrivalBytes+1)
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{oversized}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	duplicateFact := []VerifiedTerminalCandidateFrame{cloneTerminalCandidateInputV1(frames[0]), cloneTerminalCandidateInputV1(frames[1])}
	duplicateFact[1].Pruned.Reference.FactID = duplicateFact[0].Sealed.Fact.FactID
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, duplicateFact, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	duplicateSource := []VerifiedTerminalCandidateFrame{cloneTerminalCandidateInputV1(frames[0]), cloneTerminalCandidateInputV1(frames[1])}
	duplicateSource[1].Pruned.Reference.EnvironmentSequence = 1
	duplicateSource[1].Pruned.Reference.PreviousEnvelopeDigest = [32]byte{}
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, duplicateSource, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	duplicateNonce := []VerifiedTerminalCandidateFrame{cloneTerminalCandidateInputV1(frames[0]), cloneTerminalCandidateInputV1(frames[1])}
	duplicateNonce[1].Pruned.Reference.Nonce = duplicateNonce[0].Sealed.Nonce
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, duplicateNonce, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorNonceReuse)
	future := []VerifiedTerminalCandidateFrame{cloneTerminalCandidateInputV1(frames[0]), cloneTerminalCandidateInputV1(frames[1])}
	future[1].Pruned.HLC.WallMillis = 1_101
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, future, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorHLC)
	assertNoActiveTerminalCandidateV1(t, store, projectID)
}

func TestDiscardTerminalCandidateRefusesPromotedReceipt(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-promoted-discard", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	checkpoint := terminalCandidateCheckpointV1(candidate)
	if err := store.DiscardTerminalCandidate(context.Background(), projectID, checkpoint); err != nil {
		t.Fatalf("DiscardTerminalCandidate() error = %v", err)
	}
	postDigest := sha256.Sum256([]byte("post-promotion-corpus"))
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES(?, ?, 'promoted', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), candidate.CandidateID[:], candidate.ChannelID[:], candidate.RelayGeneration[:],
		candidate.MembershipGeneration, candidate.AuthorityDigest[:], candidate.StartArrivalSequence,
		candidate.ThroughArrivalSequence, candidate.FrameCount, candidate.RollingCandidateDigest[:],
		postDigest[:], candidate.ThroughArrivalSequence); err != nil {
		t.Fatalf("insert promoted receipt: %v", err)
	}
	err = store.DiscardTerminalCandidate(context.Background(), projectID, checkpoint)
	assertSyncErrorCode(t, err, SyncErrorConflict)
}

func TestTerminalCandidateConcurrentCreateAndAppendAreExactRetries(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-concurrent", 3)
	run := func(chunk []VerifiedTerminalCandidateFrame) [2]struct {
		candidate TerminalCandidate
		err       error
	} {
		var results [2]struct {
			candidate TerminalCandidate
			err       error
		}
		var wait sync.WaitGroup
		ready := make(chan struct{})
		wait.Add(2)
		for index := range results {
			go func(index int) {
				defer wait.Done()
				<-ready
				results[index].candidate, results[index].err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, chunk, 1_000, 100)
			}(index)
		}
		close(ready)
		wait.Wait()
		return results
	}
	created := run(frames[:1])
	if created[0].err != nil || created[1].err != nil || created[0].candidate != created[1].candidate {
		t.Fatalf("concurrent create = %#v", created)
	}
	appended := run(frames[1:])
	if appended[0].err != nil || appended[1].err != nil || appended[0].candidate != appended[1].candidate || appended[0].candidate.FrameCount != 3 {
		t.Fatalf("concurrent append = %#v", appended)
	}
}

func TestTerminalCandidateHasNoLifetimeFrameCapAndResumesAcrossReopen(t *testing.T) {
	const frameCount = 4_100

	stateRoot := filepath.Join(testTempDir(t), "state-terminal-candidate-no-lifetime-cap")
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	projectID := continuity.ProjectID("project-terminal-candidate-no-lifetime-cap")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	certificateID := testSyncCertificateID("environment-a")
	pruneCertificateID := sha256.Sum256([]byte("terminal-candidate-long-prune-certificate"))
	frames := make([]VerifiedTerminalCandidateFrame, 0, frameCount)
	opaque := make([]OpaqueSyncFrame, 0, frameCount)
	previousDigest := [32]byte{}
	for sequence := 1; sequence <= frameCount; sequence++ {
		label := strconv.Itoa(sequence)
		digest := sha256.Sum256([]byte("terminal-candidate-long-envelope:" + label))
		arrival := int64(sequence)
		reference := VerifiedPruneReference{
			FactID:                 continuity.FactID("fact-terminal-candidate-long-" + label),
			EnvironmentID:          "environment-a",
			EnvironmentSequence:    arrival,
			ArrivalSequence:        arrival,
			EnvelopeDigest:         digest,
			CertificateID:          certificateID,
			PreviousEnvelopeDigest: previousDigest,
			KeyGeneration:          1,
			Nonce:                  testNonce("terminal-candidate-long:" + label),
		}
		inbox := OpaqueSyncFrame{ArrivalSequence: arrival, EnvelopeDigest: digest, PrunedArrival: []byte("pruned:" + label)}
		pruned := VerifiedTerminalPrunedFrame{
			Reference:          reference,
			PruneCertificateID: pruneCertificateID,
			FactKind:           continuity.FactScratchpadMessageRecorded,
			HLC:                continuity.HybridTime{WallMillis: arrival},
		}
		opaque = append(opaque, inbox)
		frames = append(frames, VerifiedTerminalCandidateFrame{Inbox: inbox, Pruned: &pruned})
		previousDigest = digest
	}
	for offset := 0; offset < len(opaque); offset += maximumSyncPageFrames {
		end := offset + maximumSyncPageFrames
		if end > len(opaque) {
			end = len(opaque)
		}
		if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), int64(offset), frameCount, opaque[offset:end]); err != nil {
			t.Fatalf("StageSyncPage(%d:%d) error = %v", offset, end, err)
		}
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", frameCount, previousDigest)
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	var candidate TerminalCandidate
	for offset := 0; offset < len(frames); offset += maximumTerminalCandidateChunkFramesV1 {
		end := offset + maximumTerminalCandidateChunkFramesV1
		if end > len(frames) {
			end = len(frames)
		}
		candidate, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames[offset:end], frameCount+1_000, 100)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk(%d:%d) error = %v", offset, end, err)
		}
		if end < len(frames) && end%1_024 == 0 {
			if err := store.Close(); err != nil {
				t.Fatalf("Close(at %d) error = %v", end, err)
			}
			store, err = Open(stateRoot, "environment-local")
			if err != nil {
				t.Fatalf("Open(at %d) error = %v", end, err)
			}
			authority, err = store.CurrentSyncAuthority(context.Background(), projectID)
			if err != nil {
				t.Fatalf("CurrentSyncAuthority(at %d) error = %v", end, err)
			}
		}
	}
	if candidate.FrameCount != frameCount || candidate.StartArrivalSequence != 1 || candidate.ThroughArrivalSequence != frameCount {
		t.Fatalf("long candidate = %#v", candidate)
	}
	current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate() = (%#v, %v, %v), want %#v", current, found, err, candidate)
	}
	var retainedChildren int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames WHERE project_id = ?`, string(projectID)).Scan(&retainedChildren); err != nil {
		t.Fatalf("count retained candidate frames: %v", err)
	}
	if retainedChildren != frameCount {
		t.Fatalf("retained candidate frame count = %d, want %d", retainedChildren, frameCount)
	}
}

func TestTerminalCandidateMixedAppendReopenAndChunkInvariant(t *testing.T) {
	t.Parallel()

	stateRootA, storeA, projectID, authorityA, framesA := terminalCandidateMixedFixtureV1(t, "terminal-candidate-chunk-a", 3)
	first, err := storeA.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authorityA, framesA[:2], 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(first chunk) error = %v", err)
	}
	if first.FrameCount != 2 || first.ThroughArrivalSequence != 2 {
		t.Fatalf("first candidate = %#v", first)
	}
	if err := storeA.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	storeA, err = Open(stateRootA, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { _ = storeA.Close() })
	authorityA, err = storeA.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(reopen) error = %v", err)
	}
	resumed, err := storeA.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authorityA, framesA[2:], 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(append) error = %v", err)
	}
	if resumed.FrameCount != 3 || resumed.StartArrivalSequence != 1 || resumed.ThroughArrivalSequence != 3 {
		t.Fatalf("resumed candidate = %#v", resumed)
	}

	_, storeB, projectB, authorityB, framesB := terminalCandidateMixedFixtureV1(t, "terminal-candidate-chunk-b", 3)
	unchunked, err := storeB.StageVerifiedTerminalCandidateChunk(context.Background(), projectB, authorityB, framesB, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(unchunked) error = %v", err)
	}
	if resumed != unchunked {
		t.Fatalf("chunked candidate = %#v, unchunked = %#v", resumed, unchunked)
	}
}

func TestTerminalCandidateRequiresTheFirstFrameToTriggerTerminalHistory(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-candidate-first-trigger")
	projectID := continuity.ProjectID("project-terminal-candidate-first-trigger")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	firstFact := syncProjectFact(t, projectID, "fact-first-active", "environment-b", 1, 100)
	encoded, err := continuitywire.Encode(firstFact)
	if err != nil {
		t.Fatalf("encode first fact: %v", err)
	}
	sealed := append([]byte("sealed:"), encoded...)
	firstDigest := sha256.Sum256(sealed)
	firstVerified := VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  firstDigest,
		CertificateID:   testSyncCertificateID("environment-b"),
		KeyGeneration:   1,
		Nonce:           testNonce("first-trigger:1"),
		Fact:            firstFact,
	}
	secondDigest := sha256.Sum256([]byte("first-trigger:pruned"))
	secondOpaque := OpaqueSyncFrame{ArrivalSequence: 2, EnvelopeDigest: secondDigest, PrunedArrival: []byte("pruned-second")}
	secondPruned := VerifiedTerminalPrunedFrame{
		Reference: VerifiedPruneReference{
			FactID:              "fact-second-pruned",
			EnvironmentID:       "environment-a",
			EnvironmentSequence: 1,
			ArrivalSequence:     2,
			EnvelopeDigest:      secondDigest,
			CertificateID:       testSyncCertificateID("environment-a"),
			KeyGeneration:       1,
			Nonce:               testNonce("first-trigger:2"),
		},
		PruneCertificateID: sha256.Sum256([]byte("first-trigger:prune-certificate")),
		FactKind:           continuity.FactScratchpadMessageRecorded,
		HLC:                continuity.HybridTime{WallMillis: 101},
	}
	firstOpaque := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: firstDigest, SealedEnvelope: sealed}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, []OpaqueSyncFrame{firstOpaque, secondOpaque}); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", 1, secondDigest)
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{
		{Inbox: firstOpaque, Sealed: &firstVerified},
		{Inbox: secondOpaque, Pruned: &secondPruned},
	}, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorCandidate)
	assertNoActiveTerminalCandidateV1(t, store, projectID)
}

func TestTerminalCandidateRejectsAuthorityDriftWithoutMutation(t *testing.T) {
	t.Parallel()

	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-authority-drift", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, frames, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(create) error = %v", err)
	}
	drifts := map[string]func(*SyncAuthority){
		"channel": func(value *SyncAuthority) { value.ChannelID = testSyncChannelID("drifted-channel") },
		"relay": func(value *SyncAuthority) {
			value.RelayGeneration = sha256.Sum256([]byte("drifted-relay"))
			for index := range value.Environments {
				if value.Environments[index].Retirement != nil {
					value.Environments[index].Retirement.RelayGeneration = value.RelayGeneration
				}
			}
		},
		"admin": func(value *SyncAuthority) { value.AdminPublicKey = sha256.Sum256([]byte("drifted-admin")) },
		"membership": func(value *SyncAuthority) {
			value.MembershipGeneration++
			value.Environments = append(value.Environments, SyncEnvironmentCertificate{
				EnvironmentID:            "environment-z",
				CertificateID:            testSyncCertificateID("environment-z"),
				CertificateBytes:         []byte("environment-z-certificate"),
				Mode:                     SyncEnvironmentTrusted,
				JoinMembershipGeneration: value.MembershipGeneration,
			})
		},
		"certificate": func(value *SyncAuthority) {
			value.Environments[0].CertificateBytes = []byte("drifted-certificate-bytes")
		},
		"environment id": func(value *SyncAuthority) {
			value.Environments[1].EnvironmentID = "environment-c"
		},
		"certificate id": func(value *SyncAuthority) {
			value.Environments[1].CertificateID = sha256.Sum256([]byte("drifted-certificate-id"))
		},
		"mode": func(value *SyncAuthority) {
			value.Environments[1].Mode = SyncEnvironmentTrusted
			value.Environments[1].ExpiresAtMillis = 0
		},
		"expiry": func(value *SyncAuthority) {
			value.Environments[1].ExpiresAtMillis++
		},
		"join membership": func(value *SyncAuthority) {
			value.Environments[1].JoinMembershipGeneration, value.Environments[2].JoinMembershipGeneration =
				value.Environments[2].JoinMembershipGeneration, value.Environments[1].JoinMembershipGeneration
		},
		"retirement membership": func(value *SyncAuthority) {
			value.Environments[0].Retirement.MembershipGeneration = 3
			value.Environments[2].JoinMembershipGeneration = 4
		},
		"retirement final sequence": func(value *SyncAuthority) {
			value.Environments[0].Retirement.FinalEnvironmentSequence = 1
			value.Environments[0].Retirement.FinalEnvelopeDigest = frames[0].Sealed.EnvelopeDigest
		},
		"retirement final digest": func(value *SyncAuthority) {
			value.Environments[0].Retirement.FinalEnvelopeDigest = sha256.Sum256([]byte("drifted-final-digest"))
		},
		"retirement id": func(value *SyncAuthority) {
			value.Environments[0].Retirement.RetirementID = sha256.Sum256([]byte("drifted-retirement-id"))
		},
		"retirement bytes": func(value *SyncAuthority) {
			value.Environments[0].Retirement.RetirementBytes = []byte("drifted-retirement-bytes")
		},
	}
	for name, mutate := range drifts {
		t.Run(name, func(t *testing.T) {
			drifted := cloneTerminalCandidateAuthorityV1(authority)
			mutate(&drifted)
			_, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, drifted, frames, 1_000, 100)
			assertSyncErrorCode(t, err, SyncErrorConflict)
			current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
			if currentErr != nil || !found || current != candidate {
				t.Fatalf("CurrentTerminalCandidate() = (%#v, %v, %v), want unchanged %#v", current, found, currentErr, candidate)
			}
		})
	}
	advanced := cloneTerminalCandidateAuthorityV1(authority)
	advanced.MembershipGeneration++
	advanced.Environments = append(advanced.Environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-z",
		CertificateID:            testSyncCertificateID("environment-z"),
		CertificateBytes:         []byte("environment-z-certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: advanced.MembershipGeneration,
	})
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, advanced); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(advance) error = %v", err)
	}
	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, advanced, frames, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after authority advance) = (%#v, %v, %v), want unchanged %#v", current, found, err, candidate)
	}
	if err := store.DiscardTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate)); err != nil {
		t.Fatalf("DiscardTerminalCandidate(after authority advance) error = %v", err)
	}
	assertNoActiveTerminalCandidateV1(t, store, projectID)
}

func TestTerminalCandidateRejectsInboxMismatchAndQuarantine(t *testing.T) {
	t.Parallel()

	t.Run("bytes and digest", func(t *testing.T) {
		_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-inbox-mismatch", 2)
		altered := frames[0]
		altered.Inbox = cloneOpaqueSyncFrameV1(frames[0].Inbox)
		altered.Inbox.SealedEnvelope = append(altered.Inbox.SealedEnvelope, 'x')
		altered.Inbox.EnvelopeDigest = sha256.Sum256(altered.Inbox.SealedEnvelope)
		sealed := *altered.Sealed
		sealed.EnvelopeDigest = altered.Inbox.EnvelopeDigest
		altered.Sealed = &sealed
		_, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{altered}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})

	t.Run("tag", func(t *testing.T) {
		_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-inbox-tag", 2)
		altered := frames[0]
		altered.Inbox = cloneOpaqueSyncFrameV1(frames[0].Inbox)
		altered.Inbox.SealedEnvelope = nil
		altered.Inbox.PrunedArrival = []byte("wrong-tag")
		_, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{altered}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})

	t.Run("quarantine", func(t *testing.T) {
		_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "terminal-candidate-inbox-quarantine", 2)
		if _, err := store.db.Exec(`UPDATE continuity_sync_inbox SET state = 'quarantined' WHERE project_id = ? AND arrival_sequence = 1`, string(projectID)); err != nil {
			t.Fatalf("quarantine inbox: %v", err)
		}
		_, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{frames[0]}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})
}

func TestTerminalCandidateEnforcesRetirementAndExpiry(t *testing.T) {
	t.Parallel()

	t.Run("expired ephemeral needs recovery", func(t *testing.T) {
		store := openSyncStore(t, "terminal-candidate-expired")
		projectID := continuity.ProjectID("project-terminal-candidate-expired")
		fact := syncProjectFact(t, projectID, "fact-expired-root", "environment-b", 1, 100)
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
		inbox, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 1)
		if err != nil {
			t.Fatalf("PendingSyncFramesAfter() error = %v", err)
		}
		authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthority() error = %v", err)
		}
		_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{{Inbox: inbox[0], Sealed: &verified[0]}}, 10_000, 100)
		assertSyncErrorCode(t, err, SyncErrorRecoveryRequired)
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})

	t.Run("active trusted is not a trigger", func(t *testing.T) {
		store := openSyncStore(t, "terminal-candidate-active")
		projectID := continuity.ProjectID("project-terminal-candidate-active")
		fact := syncProjectFact(t, projectID, "fact-active-root", "environment-a", 1, 100)
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
		inbox, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 1)
		if err != nil {
			t.Fatalf("PendingSyncFramesAfter() error = %v", err)
		}
		authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthority() error = %v", err)
		}
		_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{{Inbox: inbox[0], Sealed: &verified[0]}}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorCandidate)
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})

	t.Run("final digest", func(t *testing.T) {
		store := openSyncStore(t, "terminal-candidate-retirement-digest")
		projectID := continuity.ProjectID("project-terminal-candidate-retirement-digest")
		fact := syncProjectFact(t, projectID, "fact-retired-root", "environment-a", 1, 100)
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
		inbox, err := store.PendingSyncFramesAfter(context.Background(), projectID, 0, 1)
		if err != nil {
			t.Fatalf("PendingSyncFramesAfter() error = %v", err)
		}
		retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", 1, sha256.Sum256([]byte("wrong-final")))
		authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthority() error = %v", err)
		}
		_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{{Inbox: inbox[0], Sealed: &verified[0]}}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorCertificate)
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})
}

func TestTerminalCandidateChecksCanonicalFactSourceNonceChainAndHLC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		environmentID continuity.EnvironmentID
		sequence      int64
		factID        continuity.FactID
		wall          int64
		previous      func([32]byte) [32]byte
		nonce         func([24]byte) [24]byte
		want          SyncErrorCode
	}{
		{
			name:          "source",
			environmentID: "environment-a",
			sequence:      1,
			factID:        "fact-conflicting-source",
			wall:          101,
			previous:      func([32]byte) [32]byte { return [32]byte{} },
			want:          SyncErrorConflict,
		},
		{
			name:          "fact",
			environmentID: "environment-b",
			sequence:      1,
			factID:        "fact-project",
			wall:          101,
			previous:      func([32]byte) [32]byte { return [32]byte{} },
			want:          SyncErrorConflict,
		},
		{
			name:          "nonce",
			environmentID: "environment-b",
			sequence:      1,
			factID:        "fact-conflicting-nonce",
			wall:          101,
			previous:      func([32]byte) [32]byte { return [32]byte{} },
			nonce:         func(value [24]byte) [24]byte { return value },
			want:          SyncErrorNonceReuse,
		},
		{
			name:          "chain",
			environmentID: "environment-a",
			sequence:      2,
			factID:        "fact-conflicting-chain",
			wall:          101,
			previous:      func([32]byte) [32]byte { return sha256.Sum256([]byte("wrong-chain")) },
			want:          SyncErrorEnvelopeChain,
		},
		{
			name:          "hlc",
			environmentID: "environment-a",
			sequence:      2,
			factID:        "fact-conflicting-hlc",
			wall:          100,
			previous:      func(value [32]byte) [32]byte { return value },
			want:          SyncErrorHLC,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID := storeWithAppliedRoot(t, "terminal-candidate-canonical-"+test.name)
			var rootDigestBytes, rootNonceBytes []byte
			if err := store.db.QueryRow(`
SELECT envelope_digest, nonce
FROM continuity_sync_receipts
WHERE project_id = ? AND arrival_sequence = 1`, string(projectID)).Scan(&rootDigestBytes, &rootNonceBytes); err != nil {
				t.Fatalf("read canonical root receipt: %v", err)
			}
			var rootDigest [32]byte
			var rootNonce [24]byte
			copy(rootDigest[:], rootDigestBytes)
			copy(rootNonce[:], rootNonceBytes)
			previous := test.previous(rootDigest)
			nonce := testNonce("terminal-candidate-canonical:" + test.name)
			if test.nonce != nil {
				nonce = test.nonce(rootNonce)
			}
			fact := syncProjectFact(t, projectID, test.factID, test.environmentID, test.sequence, test.wall)
			encoded, err := continuitywire.Encode(fact)
			if err != nil {
				t.Fatalf("encode candidate fact: %v", err)
			}
			sealed := append([]byte("sealed:"), encoded...)
			digest := sha256.Sum256(sealed)
			verified := VerifiedSyncFrame{
				ArrivalSequence:        2,
				PreviousEnvelopeDigest: previous,
				EnvelopeDigest:         digest,
				CertificateID:          testSyncCertificateID(string(test.environmentID)),
				KeyGeneration:          1,
				Nonce:                  nonce,
				Fact:                   fact,
			}
			inbox := OpaqueSyncFrame{ArrivalSequence: 2, EnvelopeDigest: digest, SealedEnvelope: sealed}
			if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 1, 2, []OpaqueSyncFrame{inbox}); err != nil {
				t.Fatalf("StageSyncPage() error = %v", err)
			}
			retireSyncEnvironmentForGateV1(t, store, projectID, test.environmentID, test.sequence, digest)
			authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
			if err != nil {
				t.Fatalf("CurrentSyncAuthority() error = %v", err)
			}
			_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{{Inbox: inbox, Sealed: &verified}}, 1_000, 100)
			assertSyncErrorCode(t, err, test.want)
			assertNoActiveTerminalCandidateV1(t, store, projectID)
		})
	}
}

func TestTerminalCandidateAcceptsExactSealedDuplicateBelowCanonicalHead(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-terminal-candidate-canonical-duplicate"), "environment-b", 100)
	projectID := continuity.ProjectID("project-terminal-candidate-canonical-duplicate")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-local-root", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 0, nil); err != nil {
		t.Fatalf("StageSyncPage(empty) error = %v", err)
	}
	if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err != nil {
		t.Fatalf("ActivateStagedSync() error = %v", err)
	}

	root, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("NextUnsealedLocalFact(root) = (_, %v, %v), want fact", found, err)
	}
	rootSealed := []byte("sealed-local-root")
	rootOutbox := SealedOutboxFrame{
		FactID:         root.Fact.FactID,
		EnvelopeDigest: sha256.Sum256(rootSealed),
		CertificateID:  testSyncCertificateID("environment-b"),
		KeyGeneration:  1,
		Nonce:          testNonce("terminal-candidate-canonical-duplicate:1"),
		SealedEnvelope: rootSealed,
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), rootOutbox); err != nil {
		t.Fatalf("PersistSealedOutbox(root) error = %v", err)
	}
	mustAppendV1(t)(store.CreateIdea(context.Background(), projectID, "fact-local-second", "idea-local", continuity.IdeaCreatedPayload{Observation: appendObservationV1(), Content: continuity.IdeaContent{Label: "Local"}}))
	second, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("NextUnsealedLocalFact(second) = (_, %v, %v), want fact", found, err)
	}
	secondSealed := []byte("sealed-local-second")
	secondOutbox := SealedOutboxFrame{
		FactID:                 second.Fact.FactID,
		PreviousEnvelopeDigest: rootOutbox.EnvelopeDigest,
		EnvelopeDigest:         sha256.Sum256(secondSealed),
		CertificateID:          testSyncCertificateID("environment-b"),
		KeyGeneration:          1,
		Nonce:                  testNonce("terminal-candidate-canonical-duplicate:2"),
		SealedEnvelope:         secondSealed,
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), secondOutbox); err != nil {
		t.Fatalf("PersistSealedOutbox(second) error = %v", err)
	}

	remoteFact := syncIdeaCreatedFact(t, projectID, "fact-remote-terminal", "idea-remote", "environment-a", 1, 200, "Remote")
	remoteBody, err := continuitywire.Encode(remoteFact)
	if err != nil {
		t.Fatalf("encode remote terminal fact: %v", err)
	}
	remoteSealed := append([]byte("sealed:"), remoteBody...)
	remoteDigest := sha256.Sum256(remoteSealed)
	remoteVerified := VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  remoteDigest,
		CertificateID:   testSyncCertificateID("environment-a"),
		KeyGeneration:   1,
		Nonce:           testNonce("terminal-candidate-canonical-duplicate:remote"),
		Fact:            remoteFact,
	}
	rootEcho := VerifiedSyncFrame{
		ArrivalSequence:        2,
		PreviousEnvelopeDigest: rootOutbox.PreviousEnvelopeDigest,
		EnvelopeDigest:         rootOutbox.EnvelopeDigest,
		CertificateID:          rootOutbox.CertificateID,
		KeyGeneration:          rootOutbox.KeyGeneration,
		Nonce:                  rootOutbox.Nonce,
		Fact:                   root.Fact,
	}
	remoteInbox := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: remoteDigest, SealedEnvelope: remoteSealed}
	rootEchoInbox := OpaqueSyncFrame{ArrivalSequence: 2, EnvelopeDigest: rootOutbox.EnvelopeDigest, SealedEnvelope: rootOutbox.SealedEnvelope}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, []OpaqueSyncFrame{remoteInbox, rootEchoInbox}); err != nil {
		t.Fatalf("StageSyncPage(candidate arrivals) error = %v", err)
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", 1, remoteDigest)
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	if _, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{{Inbox: remoteInbox, Sealed: &remoteVerified}}, 1_000, 100); err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(trigger) error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, authority, []VerifiedTerminalCandidateFrame{{Inbox: rootEchoInbox, Sealed: &rootEcho}}, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(exact duplicate below head) error = %v", err)
	}
	if candidate.FrameCount != 2 || candidate.ThroughArrivalSequence != 2 {
		t.Fatalf("candidate = %#v, want two staged arrivals", candidate)
	}
}

func assertNoActiveTerminalCandidateV1(t *testing.T, store *Store, projectID continuity.ProjectID) {
	t.Helper()
	_, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || found {
		t.Fatalf("CurrentTerminalCandidate() = (_, %v, %v), want (_, false, nil)", found, err)
	}
}

func cloneTerminalCandidateInputV1(frame VerifiedTerminalCandidateFrame) VerifiedTerminalCandidateFrame {
	clone := frame
	clone.Inbox = cloneOpaqueSyncFrameV1(frame.Inbox)
	if frame.Sealed != nil {
		sealed := *frame.Sealed
		sealed.Fact.CanonicalPayload = append([]byte(nil), frame.Sealed.Fact.CanonicalPayload...)
		clone.Sealed = &sealed
	}
	if frame.Pruned != nil {
		pruned := *frame.Pruned
		clone.Pruned = &pruned
	}
	return clone
}

func terminalCandidateCheckpointV1(candidate TerminalCandidate) TerminalCandidateCheckpoint {
	return TerminalCandidateCheckpoint{
		CandidateID:            candidate.CandidateID,
		ThroughArrivalSequence: candidate.ThroughArrivalSequence,
		FrameCount:             candidate.FrameCount,
		RollingCandidateDigest: candidate.RollingCandidateDigest,
	}
}

func terminalCandidateMixedFixtureV1(t *testing.T, suffix string, count int) (string, *Store, continuity.ProjectID, SyncAuthority, []VerifiedTerminalCandidateFrame) {
	t.Helper()
	stateRoot := filepath.Join(testTempDir(t), "state-"+suffix)
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	projectID := continuity.ProjectID("project-terminal-candidate-mixed")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))

	fact := syncProjectFact(t, projectID, "fact-terminal-candidate-root", "environment-a", 1, 100)
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode root: %v", err)
	}
	sealed := append([]byte("sealed:"), encoded...)
	firstDigest := sha256.Sum256(sealed)
	certificateID := testSyncCertificateID("environment-a")
	firstVerified := VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  firstDigest,
		CertificateID:   certificateID,
		KeyGeneration:   1,
		Nonce:           testNonce("terminal-candidate-mixed:1"),
		Fact:            fact,
	}
	opaque := []OpaqueSyncFrame{{ArrivalSequence: 1, EnvelopeDigest: firstDigest, SealedEnvelope: sealed}}
	frames := []VerifiedTerminalCandidateFrame{{Inbox: opaque[0], Sealed: &firstVerified}}
	previousDigest := firstDigest
	for sequence := 2; sequence <= count; sequence++ {
		digest := sha256.Sum256([]byte("terminal-candidate-envelope:" + strconv.Itoa(sequence)))
		reference := VerifiedPruneReference{
			FactID:                 continuity.FactID("fact-terminal-candidate-pruned-" + strconv.Itoa(sequence)),
			EnvironmentID:          "environment-a",
			EnvironmentSequence:    int64(sequence),
			ArrivalSequence:        int64(sequence),
			EnvelopeDigest:         digest,
			CertificateID:          certificateID,
			PreviousEnvelopeDigest: previousDigest,
			KeyGeneration:          1,
			Nonce:                  testNonce("terminal-candidate-mixed:" + strconv.Itoa(sequence)),
		}
		prunedArrival := []byte("pruned-arrival:" + strconv.Itoa(sequence))
		opaque = append(opaque, OpaqueSyncFrame{ArrivalSequence: int64(sequence), EnvelopeDigest: digest, PrunedArrival: prunedArrival})
		pruned := VerifiedTerminalPrunedFrame{
			Reference:          reference,
			PruneCertificateID: sha256.Sum256([]byte("prune-certificate")),
			FactKind:           continuity.FactScratchpadMessageRecorded,
			HLC:                continuity.HybridTime{WallMillis: int64(99 + sequence)},
		}
		frames = append(frames, VerifiedTerminalCandidateFrame{Inbox: opaque[len(opaque)-1], Pruned: &pruned})
		previousDigest = digest
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, int64(count), opaque); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	retireSyncEnvironmentForGateV1(t, store, projectID, "environment-a", int64(count), previousDigest)
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	return stateRoot, store, projectID, authority, frames
}
