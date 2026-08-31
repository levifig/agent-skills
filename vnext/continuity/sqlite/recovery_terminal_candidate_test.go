package sqlite

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestSyncRecoveryPruneTargetByArrivalRequiresExactReadyCandidate(t *testing.T) {
	fixture := recoveryTerminalCandidateFixtureV1(t, "point-lookup")
	match, found, err := fixture.store.SyncRecoveryPruneTargetByArrival(
		context.Background(), fixture.projectID, fixture.recoveryPrunes, fixture.pruned.Reference.ArrivalSequence,
	)
	if err != nil || !found {
		t.Fatalf("exact recovery prune target lookup = (%#v, %t, %v)", match, found, err)
	}
	if match.PruneID != fixture.pruned.PruneID || match.PruneCertificateID != fixture.pruned.PruneCertificateID ||
		match.MembershipGeneration != fixture.binding.MembershipGeneration || match.Reference != fixture.pruned.Reference ||
		match.FactKind != fixture.pruned.FactKind || match.HLC != fixture.pruned.HLC {
		t.Fatalf("exact recovery prune target match = %#v", match)
	}
	for _, formatted := range []string{fmt.Sprintf("%v", match), fmt.Sprintf("%#v", match)} {
		for _, privateValue := range []string{
			string(match.Reference.FactID), string(match.Reference.EnvironmentID), fmt.Sprintf("%x", match.PruneID),
		} {
			if strings.Contains(formatted, privateValue) {
				t.Fatalf("formatted recovery prune target exposed %q: %s", privateValue, formatted)
			}
		}
	}
	if missing, found, err := fixture.store.SyncRecoveryPruneTargetByArrival(
		context.Background(), fixture.projectID, fixture.recoveryPrunes, 1,
	); err != nil || found || missing != (SyncRecoveryPruneTargetMatch{}) {
		t.Fatalf("exact absent recovery prune target = (%#v, %t, %v), want zero false nil", missing, found, err)
	}
	stale := fixture.recoveryPrunes
	stale.InventoryDigest[0] ^= 0xff
	if _, _, err := fixture.store.SyncRecoveryPruneTargetByArrival(
		context.Background(), fixture.projectID, stale, fixture.pruned.Reference.ArrivalSequence,
	); err == nil {
		t.Fatal("stale recovery prune target lookup error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestRecoveryTerminalCandidateClassifiesMissingReadyTargetAsStoreCorruption(t *testing.T) {
	fixture := recoveryTerminalCandidateFixtureV1(t, "missing-ready-target")
	if _, err := fixture.store.db.Exec(`
DELETE FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = ?`,
		string(fixture.projectID), fixture.recoveryPrunes.CandidateID[:], fixture.pruned.Reference.ArrivalSequence,
	); err != nil {
		t.Fatalf("delete ready recovery prune target: %v", err)
	}
	if _, _, err := fixture.store.SyncRecoveryPruneTargetByArrival(
		context.Background(), fixture.projectID, fixture.recoveryPrunes, fixture.pruned.Reference.ArrivalSequence,
	); err == nil {
		t.Fatal("lookup over missing ready target error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	if _, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), fixture.projectID, fixture.binding, fixture.recoveryPrunes,
		[]VerifiedTerminalCandidateFrame{{Inbox: fixture.inbox, Pruned: &fixture.pruned}}, 1_000, 100,
	); err == nil {
		t.Fatal("terminal staging over missing ready target error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	var active int64
	if err := fixture.store.db.QueryRow(`
SELECT COUNT(*) FROM continuity_sync_terminal_candidates
WHERE project_id = ? AND state = 'staging'`, string(fixture.projectID)).Scan(&active); err != nil || active != 0 {
		t.Fatalf("terminal candidates after corrupt target rejection = %d, %v", active, err)
	}
}

func TestRecoveryTerminalCandidateStartsAllSealedSnapshotUnderReadyInventory(t *testing.T) {
	store := openSyncStore(t, "recovery-terminal-all-sealed")
	projectID := continuity.ProjectID("project-recovery-terminal-all-sealed")
	authority := testActiveSyncAuthority()
	authority.InventoryArrivalHead = 1
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
	); err != nil {
		t.Fatalf("seed all-sealed recovery watermark: %v", err)
	}

	fact := syncProjectFact(t, projectID, "fact-recovery-terminal-all-sealed", "environment-a", 1, 100)
	encoded, err := continuitywire.Encode(fact)
	if err != nil {
		t.Fatalf("encode all-sealed recovery root: %v", err)
	}
	sealedBytes := append([]byte("sealed:"), encoded...)
	envelopeDigest := sha256.Sum256(sealedBytes)
	inbox := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: envelopeDigest, SealedEnvelope: sealedBytes}
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, 1, []OpaqueSyncFrame{inbox},
	); err != nil {
		t.Fatalf("stage all-sealed recovery root: %v", err)
	}
	sealed := VerifiedSyncFrame{
		ArrivalSequence: 1, EnvelopeDigest: envelopeDigest,
		CertificateID: testSyncCertificateID("environment-a"), KeyGeneration: 1,
		Nonce: testNonce("recovery-terminal-all-sealed"), Fact: fact,
	}
	frame := VerifiedTerminalCandidateFrame{Inbox: inbox, Sealed: &sealed}
	if _, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, []VerifiedTerminalCandidateFrame{frame}, 1_000, 100,
	); err == nil {
		t.Fatal("ordinary all-sealed terminal candidate without trigger error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorCandidate)
	}

	recoveryPrunes, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, SyncRecoveryPruneSnapshot{Authority: binding}, nil,
		SyncRecoveryPruneCandidatePage{
			ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(0x91),
			InventoryDigest:        testSyncRecoveryPruneInventoryDigestV1(0x92),
		},
	)
	if err != nil {
		t.Fatalf("stage ready empty recovery prune inventory: %v", err)
	}
	candidate, err := store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), projectID, binding, recoveryPrunes,
		[]VerifiedTerminalCandidateFrame{frame}, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("start all-sealed recovery terminal candidate: %v", err)
	}
	wantCheckpoint := TerminalCandidateCheckpoint{
		CandidateID:            candidate.CandidateID,
		ThroughArrivalSequence: candidate.ThroughArrivalSequence,
		FrameCount:             candidate.FrameCount,
		RollingCandidateDigest: candidate.RollingCandidateDigest,
	}
	if candidate.Checkpoint() != wantCheckpoint {
		t.Fatalf("terminal candidate checkpoint = %#v, want %#v", candidate.Checkpoint(), wantCheckpoint)
	}
}

func TestRecoveryTerminalCandidateAuthenticatesStagesPromotesAndConsumesReadyIndex(t *testing.T) {
	fixture := recoveryTerminalCandidateFixtureV1(t, "lifecycle")
	frame := VerifiedTerminalCandidateFrame{Inbox: fixture.inbox, Pruned: &fixture.pruned}
	if _, err := fixture.store.StageVerifiedTerminalCandidateChunk(
		context.Background(), fixture.projectID, fixture.binding, []VerifiedTerminalCandidateFrame{frame}, 1_000, 100,
	); err == nil {
		t.Fatal("ordinary terminal staging with ready prune index error = nil")
	} else {
		assertSyncRecoveryPruneConflictFieldV1(t, err, "sync_recovery_prune_candidate")
	}

	for _, test := range []struct {
		name   string
		mutate func(*VerifiedTerminalCandidateFrame)
	}{
		{name: "prune id", mutate: func(frame *VerifiedTerminalCandidateFrame) { frame.Pruned.PruneID[0] ^= 0xff }},
		{name: "certificate id", mutate: func(frame *VerifiedTerminalCandidateFrame) { frame.Pruned.PruneCertificateID[0] ^= 0xff }},
		{name: "fact kind", mutate: func(frame *VerifiedTerminalCandidateFrame) {
			frame.Pruned.FactKind = continuity.FactScratchpadClaimRecorded
		}},
		{name: "clock", mutate: func(frame *VerifiedTerminalCandidateFrame) { frame.Pruned.HLC.Logical++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			alteredPruned := fixture.pruned
			altered := VerifiedTerminalCandidateFrame{Inbox: fixture.inbox, Pruned: &alteredPruned}
			test.mutate(&altered)
			if _, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
				context.Background(), fixture.projectID, fixture.binding, fixture.recoveryPrunes,
				[]VerifiedTerminalCandidateFrame{altered}, 1_000, 100,
			); err == nil {
				t.Fatal("altered recovery terminal staging error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorConflict)
			}
		})
	}

	candidate, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), fixture.projectID, fixture.binding, fixture.recoveryPrunes,
		[]VerifiedTerminalCandidateFrame{frame}, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("stage exact recovery terminal candidate: %v", err)
	}
	if err := fixture.store.DiscardSyncRecoveryPruneCandidate(
		context.Background(), fixture.projectID, fixture.recoveryPrunes.Checkpoint(),
	); err == nil {
		t.Fatal("discard ready prune index during terminal staging error = nil")
	} else {
		assertSyncRecoveryPruneConflictFieldV1(t, err, "terminal_candidate")
	}
	checkpoint := terminalCandidateCheckpointV1(candidate)
	receipt, err := fixture.store.PromoteTerminalCandidate(context.Background(), fixture.projectID, checkpoint)
	if err != nil {
		t.Fatalf("promote recovery terminal candidate: %v", err)
	}
	if receipt.ResultingAppliedCursor != fixture.binding.InventoryArrivalHead {
		t.Fatalf("recovery terminal applied cursor = %d", receipt.ResultingAppliedCursor)
	}
	assertNoSyncRecoveryPruneCandidateV1(t, fixture.store, fixture.projectID)
	if replay, err := fixture.store.PromoteTerminalCandidate(context.Background(), fixture.projectID, checkpoint); err != nil || replay != receipt {
		t.Fatalf("exact recovery terminal promotion replay = (%#v, %v), want (%#v, nil)", replay, err, receipt)
	}
	if _, err := fixture.store.ActivateStagedSync(context.Background(), fixture.projectID, fixture.binding); err != nil {
		t.Fatalf("activate promoted recovery terminal state: %v", err)
	}
}

func TestRecoveryTerminalPromotionPreventsReadyIndexRecreationBeforeActivation(t *testing.T) {
	fixture := recoveryTerminalCandidateFixtureV1(t, "receipt-fence")
	candidate, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), fixture.projectID, fixture.binding, fixture.recoveryPrunes,
		[]VerifiedTerminalCandidateFrame{{Inbox: fixture.inbox, Pruned: &fixture.pruned}}, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("stage recovery terminal candidate: %v", err)
	}
	checkpoint := terminalCandidateCheckpointV1(candidate)
	receipt, err := fixture.store.PromoteTerminalCandidate(context.Background(), fixture.projectID, checkpoint)
	if err != nil {
		t.Fatalf("promote recovery terminal candidate: %v", err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close promoted recovery terminal store: %v", err)
	}

	reopened, err := Open(fixture.stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("reopen promoted recovery terminal store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err := reopened.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), fixture.projectID, fixture.pruneSnapshot, nil, fixture.prunePage,
	); err == nil {
		t.Fatal("recreate ready prune index after terminal promotion error = nil")
	} else {
		assertSyncRecoveryPruneConflictFieldV1(t, err, "terminal_candidate")
	}
	if replay, err := reopened.PromoteTerminalCandidate(context.Background(), fixture.projectID, checkpoint); err != nil || replay != receipt {
		t.Fatalf("promotion replay after rejected prune recreation = (%#v, %v), want (%#v, nil)", replay, err, receipt)
	}
	assertNoSyncRecoveryPruneCandidateV1(t, reopened, fixture.projectID)
	if _, err := reopened.ActivateStagedSync(context.Background(), fixture.projectID, fixture.binding); err != nil {
		t.Fatalf("activate after rejected prune recreation: %v", err)
	}
}

func TestRecoveryTerminalCandidateRejectsSealedRepresentationOfAuthenticatedTarget(t *testing.T) {
	fixture := recoveryTerminalCandidateFixtureV1(t, "sealed-resurrection")
	fact := syncScratchpadFactV1(
		t, fixture.projectID, fixture.pruned.Reference.FactID, "scratchpad-recovery-terminal",
		continuity.FactScratchpadMessageRecorded, fixture.pruned.Reference.EnvironmentID,
		fixture.pruned.Reference.EnvironmentSequence, fixture.pruned.HLC.WallMillis,
	)
	sealed := VerifiedSyncFrame{
		ArrivalSequence:        fixture.pruned.Reference.ArrivalSequence,
		PreviousEnvelopeDigest: fixture.pruned.Reference.PreviousEnvelopeDigest,
		EnvelopeDigest:         fixture.pruned.Reference.EnvelopeDigest,
		CertificateID:          fixture.pruned.Reference.CertificateID,
		KeyGeneration:          fixture.pruned.Reference.KeyGeneration,
		Nonce:                  fixture.pruned.Reference.Nonce,
		Fact:                   fact,
	}
	sealedInbox := fixture.inbox
	sealedInbox.PrunedArrival = nil
	sealedInbox.SealedEnvelope = []byte("recovery-terminal-pruned-envelope")
	if _, err := fixture.store.db.Exec(`
UPDATE continuity_sync_inbox
SET frame_kind = 'sealed', frame_bytes = ?
WHERE project_id = ? AND arrival_sequence = ?`,
		sealedInbox.SealedEnvelope, string(fixture.projectID), sealedInbox.ArrivalSequence,
	); err != nil {
		t.Fatalf("replace retained target representation: %v", err)
	}
	if _, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), fixture.projectID, fixture.binding, fixture.recoveryPrunes,
		[]VerifiedTerminalCandidateFrame{{Inbox: sealedInbox, Sealed: &sealed}}, 1_000, 100,
	); err == nil {
		t.Fatal("sealed representation of authenticated prune target error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

type recoveryTerminalCandidateFixture struct {
	store          *Store
	stateRoot      string
	projectID      continuity.ProjectID
	binding        SyncAuthorityBinding
	pruneSnapshot  SyncRecoveryPruneSnapshot
	prunePage      SyncRecoveryPruneCandidatePage
	recoveryPrunes SyncRecoveryPruneCandidate
	inbox          OpaqueSyncFrame
	pruned         VerifiedTerminalPrunedFrame
}

func recoveryTerminalCandidateFixtureV1(t *testing.T, suffix string) recoveryTerminalCandidateFixture {
	t.Helper()
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-terminal-"+suffix)
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("open recovery terminal fixture: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	projectID := continuity.ProjectID("project-recovery-terminal-" + syncSlug(suffix))
	authority := testActiveSyncAuthority()
	authority.InventoryArrivalHead = 2
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
	); err != nil {
		t.Fatalf("seed recovery terminal watermark: %v", err)
	}

	root := syncProjectFact(t, projectID, "fact-recovery-terminal-root", "environment-a", 1, 100)
	encoded, err := continuitywire.Encode(root)
	if err != nil {
		t.Fatalf("encode recovery terminal root: %v", err)
	}
	sealedBytes := append([]byte("sealed:"), encoded...)
	rootDigest := sha256.Sum256(sealedBytes)
	rootFrame := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: rootDigest, SealedEnvelope: sealedBytes}
	if _, err := store.StageSyncPageUnderAuthority(context.Background(), projectID, binding, 0, 2, []OpaqueSyncFrame{rootFrame}); err != nil {
		t.Fatalf("stage recovery terminal root: %v", err)
	}
	if _, err := store.ApplySyncBatch(context.Background(), projectID, binding, []VerifiedSyncFrame{{
		ArrivalSequence: 1, EnvelopeDigest: rootDigest,
		CertificateID: testSyncCertificateID("environment-a"), KeyGeneration: 1,
		Nonce: testNonce("recovery-terminal-root"), Fact: root,
	}}, 1_000, 100); err != nil {
		t.Fatalf("apply recovery terminal root: %v", err)
	}

	reference := VerifiedPruneReference{
		FactID: "fact-recovery-terminal-pruned", EnvironmentID: "environment-a",
		EnvironmentSequence: 2, ArrivalSequence: 2,
		EnvelopeDigest: sha256.Sum256([]byte("recovery-terminal-pruned-envelope")),
		CertificateID:  testSyncCertificateID("environment-a"), PreviousEnvelopeDigest: rootDigest,
		KeyGeneration: 1, Nonce: testNonce("recovery-terminal-pruned"),
	}
	pruned := VerifiedTerminalPrunedFrame{
		PruneID: sha256.Sum256([]byte("recovery-terminal-prune-id")), Reference: reference,
		PruneCertificateID: sha256.Sum256([]byte("recovery-terminal-prune-certificate-id")),
		FactKind:           continuity.FactScratchpadMessageRecorded, HLC: continuity.HybridTime{WallMillis: 101, Logical: 1},
	}
	inbox := OpaqueSyncFrame{
		ArrivalSequence: reference.ArrivalSequence, EnvelopeDigest: reference.EnvelopeDigest,
		PrunedArrival: []byte("authenticated-pruned-arrival"),
	}
	if _, err := store.StageSyncPageUnderAuthority(context.Background(), projectID, binding, 1, 2, []OpaqueSyncFrame{inbox}); err != nil {
		t.Fatalf("stage recovery terminal pruned arrival: %v", err)
	}
	page := SyncRecoveryPruneCandidatePage{
		PagePruneCount: 1, PageTargetCount: 1, LastMembershipGeneration: binding.MembershipGeneration,
		ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(0xa1),
		InventoryDigest:        testSyncRecoveryPruneInventoryDigestV1(0xa2),
		Records: []VerifiedSyncRecoveryPruneRecord{{
			PruneSequence: 1, PruneID: pruned.PruneID, PruneCertificateID: pruned.PruneCertificateID,
			MembershipGeneration: binding.MembershipGeneration,
			Targets:              []VerifiedSyncRecoveryPruneTarget{{Reference: reference, FactKind: pruned.FactKind, HLC: pruned.HLC}},
		}},
	}
	pruneSnapshot := SyncRecoveryPruneSnapshot{Authority: binding, PruneHead: 1}
	recoveryPrunes, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, pruneSnapshot, nil, page,
	)
	if err != nil {
		t.Fatalf("stage ready recovery prune index: %v", err)
	}
	return recoveryTerminalCandidateFixture{
		store: store, stateRoot: stateRoot, projectID: projectID, binding: binding,
		pruneSnapshot: pruneSnapshot, prunePage: page, recoveryPrunes: recoveryPrunes, inbox: inbox, pruned: pruned,
	}
}
