package sqlite

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func currentSyncAuthorityBindingForTest(t testing.TB, store *Store, projectID continuity.ProjectID) SyncAuthorityBinding {
	t.Helper()
	binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
	}
	return binding
}

func promoteSyncAuthorityArrivalHeadForTest(t *testing.T, store *Store, projectID continuity.ProjectID, head int64) SyncAuthorityBinding {
	t.Helper()
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	if binding.InventoryArrivalHead == head {
		return binding
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, binding.AuthorityDigestVersion, binding.AuthorityDigest)
	snapshot.InventoryArrivalHead = head
	ready := stageSyncAuthorityCandidateInventoryV2(t, store, projectID, snapshot, authority.Environments)
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate(arrival head %d) error = %v", head, err)
	}
	return currentSyncAuthorityBindingForTest(t, store, projectID)
}

func syncAuthorityBindingV1ForTest(t testing.TB, projectID continuity.ProjectID, authority SyncAuthority) SyncAuthorityBinding {
	t.Helper()
	digest, err := frozenSyncAuthorityDigestV1(projectID, authority)
	if err != nil {
		t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
	}
	return syncAuthorityBindingForTest(authority, 1, digest)
}

func cloneTerminalCandidateAuthorityV1(authority SyncAuthority) SyncAuthority {
	clone := authority
	clone.Environments = make([]SyncEnvironmentCertificate, len(authority.Environments))
	for index, environment := range authority.Environments {
		clone.Environments[index] = environment
		clone.Environments[index].CertificateBytes = append([]byte(nil), environment.CertificateBytes...)
		if environment.Retirement != nil {
			retirement := *environment.Retirement
			retirement.RetirementBytes = append([]byte(nil), environment.Retirement.RetirementBytes...)
			clone.Environments[index].Retirement = &retirement
		}
	}
	return clone
}

func TestApplySyncBatchRequiresExactAuthorityBindingOnEmptyBatch(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "apply-binding-empty")
	projectID := continuity.ProjectID("project-apply-binding-empty")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
	}

	if _, err := store.ApplySyncBatch(context.Background(), projectID, binding, nil, 1_000, 100); err != nil {
		t.Fatalf("ApplySyncBatch(exact empty binding) error = %v", err)
	}
	drifted := binding
	drifted.AuthorityDigest[0] ^= 0xff
	_, err = store.ApplySyncBatch(context.Background(), projectID, drifted, nil, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorConflict)
}

func TestCanonicalSyncEnvironmentPointQueryUsesCompositePrimaryKey(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-point-query-plan")
	rows, err := store.db.Query(
		"EXPLAIN QUERY PLAN "+canonicalSyncEnvironmentCertificatePointQueryV2,
		"project-query-plan",
		"environment-query-plan",
	)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN point authority lookup: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan authority point query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read authority point query plan: %v", err)
	}
	joined := strings.Join(details, "\n")
	if !strings.Contains(joined, "SEARCH continuity_sync_environment_certificates") ||
		!strings.Contains(joined, "project_id=? AND environment_id=?") {
		t.Fatalf("authority point query plan = %q, want composite equality search", joined)
	}
}

func TestApplySyncBatchUsesTouchedV2AuthorityPointReads(t *testing.T) {
	t.Parallel()

	t.Run("4097 environments ignore corrupt untouched tail", func(t *testing.T) {
		store := openSyncStore(t, "apply-v2-authority-untouched-tail")
		projectID := continuity.ProjectID("project-apply-v2-authority-untouched-tail")
		authority := syncAuthorityFromSnapshotForBindingTest(
			syncAuthorityCandidateBootstrapSnapshotV2(4_097),
			syncAuthorityCandidateManyEnvironmentsV2(4_097),
		)
		digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
		binding := syncAuthorityBindingForTest(authority, 2, digest)
		verified := stageHotPathRootFrameV2(t, store, projectID, authority.Environments[0])
		execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), authority.Environments[len(authority.Environments)-1].EnvironmentID)

		if _, err := store.ApplySyncBatch(context.Background(), projectID, binding, []VerifiedSyncFrame{verified}, 1_000, 100); err != nil {
			t.Fatalf("ApplySyncBatch(4097 authority environments) error = %v", err)
		}
		assertAppliedCursor(t, store, projectID, 1)
		if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
			t.Fatal("CurrentSyncAuthority(corrupt untouched tail) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorStore)
		}
	})

	t.Run("binding drift and corrupt touched row roll back", func(t *testing.T) {
		store := openSyncStore(t, "apply-v2-authority-touched-corrupt")
		projectID := continuity.ProjectID("project-apply-v2-authority-touched-corrupt")
		authority := syncAuthorityFromSnapshotForBindingTest(
			syncAuthorityCandidateBootstrapSnapshotV2(300),
			syncAuthorityCandidateManyEnvironmentsV2(300),
		)
		digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
		binding := syncAuthorityBindingForTest(authority, 2, digest)
		verified := stageHotPathRootFrameV2(t, store, projectID, authority.Environments[0])

		drifted := binding
		drifted.AdminPublicKey = sha256.Sum256([]byte("drifted apply admin"))
		_, err := store.ApplySyncBatch(context.Background(), projectID, drifted, []VerifiedSyncFrame{verified}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertAppliedCursor(t, store, projectID, 0)

		execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), authority.Environments[0].EnvironmentID)
		_, err = store.ApplySyncBatch(context.Background(), projectID, binding, []VerifiedSyncFrame{verified}, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorStore)
		assertAppliedCursor(t, store, projectID, 0)
		var facts, receipts int
		if err := store.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?)`, string(projectID), string(projectID)).Scan(&facts, &receipts); err != nil {
			t.Fatalf("count rolled-back hot apply rows: %v", err)
		}
		if facts != 0 || receipts != 0 {
			t.Fatalf("rolled-back hot apply rows: facts=%d receipts=%d", facts, receipts)
		}
	})
}

func TestTerminalCandidateUsesTouchedV2AuthorityPointReadsAcrossChunks(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-v2-authority-hot-path")
	projectID := continuity.ProjectID("project-terminal-v2-authority-hot-path")
	environments := syncAuthorityCandidateManyEnvironmentsV2(4_097)
	opaque, frames, finalDigest := terminalHotPathFramesV2(t, projectID, environments[0], 33)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(4_098)
	snapshot.InventoryArrivalHead = int64(len(opaque))
	environments[0].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          snapshot.RelayGeneration,
		MembershipGeneration:     4_098,
		FinalEnvironmentSequence: 33,
		FinalEnvelopeDigest:      finalDigest,
		RetirementID:             sha256.Sum256([]byte("terminal-v2-hot-retirement")),
		RetirementBytes:          []byte("terminal v2 hot retirement"),
	}
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, environments)
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	watermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
		t.Fatalf("AdvanceSyncRelayWatermark(v2 terminal frames) = (%#v, %v), want (%#v, nil)", got, err, watermark)
	}
	if _, err := store.StageSyncPageUnderAuthority(context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(v2 terminal frames) error = %v", err)
	}
	execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), environments[len(environments)-1].EnvironmentID)

	var candidate TerminalCandidate
	for offset := 0; offset < len(frames); offset += maximumTerminalCandidateChunkFramesV1 {
		end := offset + maximumTerminalCandidateChunkFramesV1
		if end > len(frames) {
			end = len(frames)
		}
		var err error
		candidate, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, binding, frames[offset:end], 10_000, 100)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk(v2 %d:%d) error = %v", offset, end, err)
		}
	}
	receipt, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	if err != nil {
		t.Fatalf("PromoteTerminalCandidate(v2 4097 environments) error = %v", err)
	}
	if receipt.FrameCount != int64(len(frames)) || receipt.ResultingAppliedCursor != int64(len(frames)) {
		t.Fatalf("v2 hot promotion receipt = %#v", receipt)
	}
	if replayed, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate)); err != nil || replayed != receipt {
		t.Fatalf("PromoteTerminalCandidate(v2 retry) = (%#v, %v), want (%#v, nil)", replayed, err, receipt)
	}
	if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthority(corrupt v2 terminal tail) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestTerminalCandidateV2PositiveHeadAdvanceRejectsResumeAndPromotion(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "terminal-v2-positive-head-drift")
	projectID := continuity.ProjectID("project-terminal-v2-positive-head-drift")
	environments := syncAuthorityCandidateManyEnvironmentsV2(300)
	opaque, frames, finalDigest := terminalHotPathFramesV2(t, projectID, environments[0], 17)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(301)
	snapshot.InventoryArrivalHead = int64(len(opaque))
	environments[0].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          snapshot.RelayGeneration,
		MembershipGeneration:     301,
		FinalEnvironmentSequence: 17,
		FinalEnvelopeDigest:      finalDigest,
		RetirementID:             sha256.Sum256([]byte("terminal-v2-drift-retirement")),
		RetirementBytes:          []byte("terminal v2 drift retirement"),
	}
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, environments)
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	watermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
		t.Fatalf("AdvanceSyncRelayWatermark(v2 drift frames) = (%#v, %v), want (%#v, nil)", got, err, watermark)
	}
	if _, err := store.StageSyncPageUnderAuthority(context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(v2 drift frames) error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, binding, frames[:maximumTerminalCandidateChunkFramesV1], 10_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(v2 first chunk) error = %v", err)
	}

	advanced := cloneTerminalCandidateAuthorityV1(authority)
	advanced.InventoryArrivalHead++
	advancedDigest := setCanonicalSyncAuthorityMetadataV2ForBindingTest(t, store, projectID, advanced)
	advancedBinding := syncAuthorityBindingForTest(advanced, 2, advancedDigest)
	advancedCandidateID, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, advancedBinding, candidate.StartArrivalSequence)
	if err != nil {
		t.Fatalf("derive advanced positive-head candidate id: %v", err)
	}
	if advancedCandidateID == candidate.CandidateID {
		t.Fatal("positive-head v2 authority advance retained terminal candidate identity")
	}

	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, advancedBinding, frames[maximumTerminalCandidateChunkFramesV1:], 10_000, 100)
	assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after v2 resume drift) = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority")
	current, found, currentErr = store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after v2 promotion drift) = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
}

func TestTerminalCandidateReplayAndPromotionFailClosedOnTouchedAuthorityCorruption(t *testing.T) {
	t.Parallel()

	_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-hot-touched-corruption", 2)
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, binding, frames, 1_000, 100)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(create) error = %v", err)
	}
	execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), string(frames[0].Sealed.Fact.EnvironmentID))

	_, err = store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, binding, frames, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorStore)
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after corrupt replay) = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertSyncErrorCode(t, err, SyncErrorStore)
	current, found, currentErr = store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after corrupt promotion) = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
}

func TestTerminalCandidateExactReplayRevalidatesTouchedAuthorityPolicy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, *Store, continuity.ProjectID, []VerifiedTerminalCandidateFrame)
	}{
		{
			name: "different valid certificate",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ []VerifiedTerminalCandidateFrame) {
				differentCertificateID := sha256.Sum256([]byte("different valid replay certificate"))
				execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = ?
WHERE project_id = ? AND environment_id = 'environment-a'`, differentCertificateID[:], string(projectID))
			},
		},
		{
			name: "different valid retirement fence",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, frames []VerifiedTerminalCandidateFrame) {
				execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET retirement_final_environment_sequence = 1,
    retirement_final_envelope_digest = ?
WHERE project_id = ? AND environment_id = 'environment-a'`, frames[0].Sealed.EnvelopeDigest[:], string(projectID))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "terminal-hot-replay-policy-"+syncSlug(test.name), 2)
			binding := currentSyncAuthorityBindingForTest(t, store, projectID)
			candidate, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, binding, frames, 1_000, 100)
			if err != nil {
				t.Fatalf("StageVerifiedTerminalCandidateChunk(create) error = %v", err)
			}
			test.mutate(t, store, projectID, frames)
			before := captureTerminalMutationStateV1(t, store, projectID)

			replayed, err := store.StageVerifiedTerminalCandidateChunk(context.Background(), projectID, binding, frames, 1_000, 100)
			assertSyncErrorCode(t, err, SyncErrorCertificate)
			if replayed != (TerminalCandidate{}) {
				t.Fatalf("StageVerifiedTerminalCandidateChunk(invalid replay) = %#v, want zero candidate", replayed)
			}
			assertTerminalMutationStateV1(t, store, projectID, before)
			current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
			if currentErr != nil || !found || current != candidate {
				t.Fatalf("CurrentTerminalCandidate(after rejected replay) = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
			}
		})
	}
}

func stageHotPathRootFrameV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	environment SyncEnvironmentCertificate,
) VerifiedSyncFrame {
	t.Helper()
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	watermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
		t.Fatalf("AdvanceSyncRelayWatermark(hot-path root) = (%#v, %v), want (%#v, nil)", got, err, watermark)
	}
	fact := syncProjectFact(t, projectID, "fact-hot-path-root", continuity.EnvironmentID(environment.EnvironmentID), 1, 100)
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode hot-path root: %v", err)
	}
	sealed := append([]byte("sealed:"), encoded...)
	digest := sha256.Sum256(sealed)
	if _, err := store.StageSyncPageUnderAuthority(context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, []OpaqueSyncFrame{{
		ArrivalSequence: 1,
		EnvelopeDigest:  digest,
		SealedEnvelope:  sealed,
	}}); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(hot-path root) error = %v", err)
	}
	return VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  digest,
		CertificateID:   environment.CertificateID,
		KeyGeneration:   1,
		Nonce:           testNonce("hot-path:" + string(projectID)),
		Fact:            fact,
	}
}

func terminalHotPathFramesV2(
	t *testing.T,
	projectID continuity.ProjectID,
	environment SyncEnvironmentCertificate,
	count int,
) ([]OpaqueSyncFrame, []VerifiedTerminalCandidateFrame, [32]byte) {
	t.Helper()
	if count < 1 {
		t.Fatal("terminal hot-path frame count must be positive")
	}
	environmentID := continuity.EnvironmentID(environment.EnvironmentID)
	fact := syncProjectFact(t, projectID, "fact-terminal-hot-root", environmentID, 1, 100)
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode terminal hot-path root: %v", err)
	}
	sealed := append([]byte("sealed:"), encoded...)
	firstDigest := sha256.Sum256(sealed)
	firstVerified := VerifiedSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  firstDigest,
		CertificateID:   environment.CertificateID,
		KeyGeneration:   1,
		Nonce:           testNonce("terminal-hot:1:" + string(projectID)),
		Fact:            fact,
	}
	opaque := []OpaqueSyncFrame{{ArrivalSequence: 1, EnvelopeDigest: firstDigest, SealedEnvelope: sealed}}
	frames := []VerifiedTerminalCandidateFrame{{Inbox: opaque[0], Sealed: &firstVerified}}
	previousDigest := firstDigest
	for sequence := 2; sequence <= count; sequence++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("terminal-hot-envelope:%s:%d", projectID, sequence)))
		reference := VerifiedPruneReference{
			FactID:                 continuity.FactID(fmt.Sprintf("fact-terminal-hot-pruned-%04d", sequence)),
			EnvironmentID:          environmentID,
			EnvironmentSequence:    int64(sequence),
			ArrivalSequence:        int64(sequence),
			EnvelopeDigest:         digest,
			CertificateID:          environment.CertificateID,
			PreviousEnvelopeDigest: previousDigest,
			KeyGeneration:          1,
			Nonce:                  testNonce(fmt.Sprintf("terminal-hot:%d:%s", sequence, projectID)),
		}
		prunedArrival := []byte(fmt.Sprintf("terminal-hot-pruned-arrival:%d", sequence))
		opaque = append(opaque, OpaqueSyncFrame{
			ArrivalSequence: int64(sequence),
			EnvelopeDigest:  digest,
			PrunedArrival:   prunedArrival,
		})
		pruned := VerifiedTerminalPrunedFrame{
			Reference:          reference,
			PruneCertificateID: sha256.Sum256([]byte("terminal-hot-prune-certificate")),
			FactKind:           continuity.FactScratchpadMessageRecorded,
			HLC:                continuity.HybridTime{WallMillis: int64(99 + sequence)},
		}
		frames = append(frames, VerifiedTerminalCandidateFrame{Inbox: opaque[len(opaque)-1], Pruned: &pruned})
		previousDigest = digest
	}
	return opaque, frames, previousDigest
}
