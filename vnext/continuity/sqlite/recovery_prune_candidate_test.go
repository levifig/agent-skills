package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncRecoveryPruneProjectionFormattingIsRedacted(t *testing.T) {
	page := testSyncRecoveryPruneCandidatePageV1(0, 1, 1, 2, false, 0x31)
	record := page.Records[0]
	target := record.Targets[0]
	for _, formatted := range []string{
		fmt.Sprintf("%v", page),
		fmt.Sprintf("%#v", page),
		fmt.Sprintf("%v", record),
		fmt.Sprintf("%#v", record),
		fmt.Sprintf("%v", target),
		fmt.Sprintf("%#v", target),
	} {
		for _, privateValue := range []string{
			string(target.Reference.FactID),
			string(target.Reference.EnvironmentID),
			fmt.Sprintf("%x", record.PruneID),
			fmt.Sprintf("%x", target.Reference.EnvelopeDigest),
		} {
			if strings.Contains(formatted, privateValue) {
				t.Fatalf("formatted recovery prune projection exposed private value %q: %s", privateValue, formatted)
			}
		}
	}
}

func TestVerifySyncRecoveryPruneCandidatePageRecordsRejectsSemanticSubstitution(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-prune-candidate-semantic-substitution")
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(t, stateRoot, "semantic-substitution", 4, 1)
	page := testSyncRecoveryPruneCandidatePageV1(0, 1, 1, 2, false, 0x31)
	ready, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, page)
	if err != nil {
		t.Fatalf("stage ready recovery prune candidate: %v", err)
	}
	if err := store.VerifySyncRecoveryPruneCandidatePageRecords(
		context.Background(), projectID, snapshot, ready.Checkpoint(), page,
	); err != nil {
		t.Fatalf("verify exact recovery prune projection: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_recovery_prune_targets
SET fact_kind = ?
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = ?`,
		string(continuity.FactScratchpadClaimRecorded), string(projectID), ready.CandidateID[:],
		page.Records[0].Targets[0].Reference.ArrivalSequence,
	); err != nil {
		t.Fatalf("substitute structurally valid recovery prune target: %v", err)
	}
	if current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID); err != nil || !found || current != ready {
		t.Fatalf("structurally valid substituted candidate = (%#v, %t, %v), want retained checkpoint", current, found, err)
	}
	if err := store.VerifySyncRecoveryPruneCandidatePageRecords(
		context.Background(), projectID, snapshot, ready.Checkpoint(), page,
	); err == nil {
		t.Fatal("semantic recovery prune substitution verification error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestSyncRecoveryPruneFinalPageAuditsEntireIndexBeforeReady(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-prune-candidate-final-audit")
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(t, stateRoot, "final-audit", 8, 5)
	firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x31)
	partial, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, firstPage)
	if err != nil {
		t.Fatalf("stage first recovery prune page: %v", err)
	}
	if _, err := store.db.Exec(`
DELETE FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ? AND prune_sequence = 1`,
		string(projectID), partial.CandidateID[:],
	); err != nil {
		t.Fatalf("corrupt earlier recovery prune page: %v", err)
	}
	finalPage := testSyncRecoveryPruneCandidatePageV1(4, 1, 1, 3, false, 0x41)
	expected := partial.Checkpoint()
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected, finalPage,
	); err == nil {
		t.Fatal("stage final page over corrupt prefix error = nil")
	}
	var pageCount, pruneCount, targetCount int64
	if err := store.db.QueryRow(`
SELECT page_count, prune_count, target_count
FROM continuity_sync_recovery_prune_candidates
WHERE project_id = ?`, string(projectID)).Scan(&pageCount, &pruneCount, &targetCount); err != nil {
		t.Fatalf("read candidate after rejected final page: %v", err)
	}
	if pageCount != partial.PageCount || pruneCount != partial.PruneCount || targetCount != partial.TargetCount {
		t.Fatalf("candidate after rejected final page = %d/%d/%d, want %d/%d/%d", pageCount, pruneCount, targetCount, partial.PageCount, partial.PruneCount, partial.TargetCount)
	}
	var finalRecords int64
	if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_recovery_prune_records
WHERE project_id = ? AND candidate_id = ? AND prune_sequence > ?`,
		string(projectID), partial.CandidateID[:], partial.ThroughPruneSequence,
	).Scan(&finalRecords); err != nil {
		t.Fatalf("count rolled-back final recovery prune records: %v", err)
	}
	if finalRecords != 0 {
		t.Fatalf("rolled-back final recovery prune records = %d, want 0", finalRecords)
	}
}

func TestSyncRecoveryPruneReadyReplayAuditsEntireIndex(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-prune-candidate-ready-replay-audit")
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(t, stateRoot, "ready-replay-audit", 8, 5)
	firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x31)
	partial, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, firstPage)
	if err != nil {
		t.Fatalf("stage first recovery prune page: %v", err)
	}
	finalPage := testSyncRecoveryPruneCandidatePageV1(4, 1, 1, 3, false, 0x41)
	expected := partial.Checkpoint()
	ready, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected, finalPage,
	)
	if err != nil {
		t.Fatalf("stage ready recovery prune candidate: %v", err)
	}
	if _, err := store.db.Exec(`
DELETE FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ? AND prune_sequence = 1`,
		string(projectID), ready.CandidateID[:],
	); err != nil {
		t.Fatalf("corrupt ready recovery prune prefix: %v", err)
	}
	readyCheckpoint := ready.Checkpoint()
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &readyCheckpoint,
		SyncRecoveryPruneCandidatePage{
			AfterPruneSequence:       ready.ThroughPruneSequence,
			LastMembershipGeneration: ready.LastMembershipGeneration,
			ResultingRollingDigest:   ready.RollingInventoryDigest,
			InventoryDigest:          ready.InventoryDigest,
		},
	); err == nil {
		t.Fatal("ready replay over corrupt prefix error = nil")
	}
}

func TestSyncRecoveryPrunePreflightFencesExactPruneHead(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-prune-candidate-preflight-prune-head")
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(t, stateRoot, "preflight-prune-head", 8, 6)
	firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x31)
	partial, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, firstPage)
	if err != nil {
		t.Fatalf("stage first recovery prune page: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_recovery_prune_candidates
SET prune_head = prune_head + 1
WHERE project_id = ? AND candidate_id = ?`, string(projectID), partial.CandidateID[:]); err != nil {
		t.Fatalf("change recovery prune candidate head: %v", err)
	}
	if err := store.VerifySyncRecoveryPrunePreflight(
		context.Background(), projectID, snapshot.Authority, &partial,
	); err == nil {
		t.Fatal("preflight over changed prune head error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestSyncRecoveryPruneCandidateStagesReplaysReadiesAndDiscards(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-prune-candidate-lifecycle")
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(t, stateRoot, "lifecycle", 8, 6)

	firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x31)
	first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil, firstPage,
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncRecoveryPruneCandidatePage(first) error = %v", err)
	}
	if first.ProjectID != projectID || first.CandidateID == ([32]byte{}) || first.Snapshot != snapshot ||
		first.PageCount != 1 || first.PruneCount != 4 || first.TargetCount != 4 ||
		first.ThroughPruneSequence != 4 || first.LastMembershipGeneration != 2 ||
		first.RollingInventoryDigest != firstPage.ResultingRollingDigest || first.Ready ||
		first.InventoryDigest != (SyncRecoveryPruneInventoryDigest{}) {
		t.Fatalf("first candidate = %#v", first)
	}
	current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
	if err != nil || !found || current != first {
		t.Fatalf("CurrentSyncRecoveryPruneCandidate() = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
	}
	replayed, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil, firstPage,
	)
	if err != nil || replayed != first {
		t.Fatalf("exact first replay = (%#v, %v), want (%#v, nil)", replayed, err, first)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	current, found, err = store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
	if err != nil || !found || current != first {
		t.Fatalf("CurrentSyncRecoveryPruneCandidate(reopen) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
	}

	finalPage := testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0x41)
	expected := first.Checkpoint()
	ready, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected, finalPage,
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncRecoveryPruneCandidatePage(final) error = %v", err)
	}
	if !ready.Ready || ready.PageCount != 2 || ready.PruneCount != 6 || ready.TargetCount != 6 ||
		ready.ThroughPruneSequence != snapshot.PruneHead || ready.LastMembershipGeneration != 3 ||
		ready.RollingInventoryDigest != finalPage.ResultingRollingDigest || ready.InventoryDigest != finalPage.InventoryDigest {
		t.Fatalf("ready candidate = %#v", ready)
	}
	replayed, err = store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected, finalPage,
	)
	if err != nil || replayed != ready {
		t.Fatalf("exact successor replay = (%#v, %v), want (%#v, nil)", replayed, err, ready)
	}
	alteredProjection := cloneSyncRecoveryPrunePageV1(finalPage)
	alteredProjection.Records[0].PruneID[0] ^= 0xff
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected, alteredProjection,
	); err == nil {
		t.Fatal("altered persisted projection replay error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	readyCheckpoint := ready.Checkpoint()
	readyNoop := SyncRecoveryPruneCandidatePage{
		AfterPruneSequence:       ready.ThroughPruneSequence,
		LastMembershipGeneration: ready.LastMembershipGeneration,
		ResultingRollingDigest:   ready.RollingInventoryDigest,
		InventoryDigest:          ready.InventoryDigest,
	}
	replayed, err = store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &readyCheckpoint, readyNoop,
	)
	if err != nil || replayed != ready {
		t.Fatalf("ready no-op replay = (%#v, %v), want (%#v, nil)", replayed, err, ready)
	}
	altered := finalPage
	altered.ResultingRollingDigest[0] ^= 0xff
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected, altered,
	); err == nil {
		t.Fatal("altered successor replay error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}

	stale := expected
	if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, stale); err == nil {
		t.Fatal("DiscardSyncRecoveryPruneCandidate(stale) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("DiscardSyncRecoveryPruneCandidate() error = %v", err)
	}
	assertSyncRecoveryPruneIndexCountsV1(t, store, projectID, 0, 0)
	if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("DiscardSyncRecoveryPruneCandidate(idempotent) error = %v", err)
	}
	if current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID); err != nil || found {
		t.Fatalf("CurrentSyncRecoveryPruneCandidate(after discard) = (%#v, %v, %v), want absent", current, found, err)
	}
}

func TestSyncRecoveryPruneCandidateRejectsCrossPageIdentityReuse(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-cross-page-reuse"), "cross-page-reuse", 8, 6,
	)
	firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x32)
	first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil, firstPage,
	)
	if err != nil {
		t.Fatalf("stage first page: %v", err)
	}
	assertSyncRecoveryPruneIndexCountsV1(t, store, projectID, 4, 4)

	tests := []struct {
		name   string
		mutate func(*SyncRecoveryPruneCandidatePage)
	}{
		{name: "prune id", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].PruneID = firstPage.Records[0].PruneID
		}},
		{name: "prune certificate id", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].PruneCertificateID = firstPage.Records[0].PruneCertificateID
		}},
		{name: "arrival sequence", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].Targets[0].Reference.ArrivalSequence = firstPage.Records[1].Targets[0].Reference.ArrivalSequence
		}},
		{name: "fact id", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].Targets[0].Reference.FactID = firstPage.Records[0].Targets[0].Reference.FactID
		}},
		{name: "source sequence", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].Targets[0].Reference.EnvironmentID = firstPage.Records[1].Targets[0].Reference.EnvironmentID
			page.Records[0].Targets[0].Reference.EnvironmentSequence = firstPage.Records[1].Targets[0].Reference.EnvironmentSequence
		}},
		{name: "envelope digest", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].Targets[0].Reference.EnvelopeDigest = firstPage.Records[0].Targets[0].Reference.EnvelopeDigest
		}},
		{name: "generation nonce", mutate: func(page *SyncRecoveryPruneCandidatePage) {
			page.Records[0].Targets[0].Reference.KeyGeneration = firstPage.Records[0].Targets[0].Reference.KeyGeneration
			page.Records[0].Targets[0].Reference.Nonce = firstPage.Records[0].Targets[0].Reference.Nonce
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			page := cloneSyncRecoveryPrunePageV1(testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0x42))
			test.mutate(&page)
			expected := first.Checkpoint()
			if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
				context.Background(), projectID, snapshot, &expected, page,
			); err == nil {
				t.Fatal("cross-page identity reuse error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorConflict)
			}
			current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
			if err != nil || !found || current != first {
				t.Fatalf("candidate after refused page = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
			}
			assertSyncRecoveryPruneIndexCountsV1(t, store, projectID, 4, 4)
		})
	}
}

func TestSyncRecoveryPruneCandidatePersistsEmptyInventory(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-empty"), "empty", 5, 0,
	)
	page := SyncRecoveryPruneCandidatePage{
		ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(0x51),
		InventoryDigest:        testSyncRecoveryPruneInventoryDigestV1(0x52),
	}
	ready, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil, page,
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncRecoveryPruneCandidatePage(empty) error = %v", err)
	}
	if !ready.Ready || ready.PageCount != 1 || ready.PruneCount != 0 || ready.TargetCount != 0 ||
		ready.ThroughPruneSequence != 0 || ready.LastMembershipGeneration != 0 ||
		ready.RollingInventoryDigest != page.ResultingRollingDigest || ready.InventoryDigest != page.InventoryDigest {
		t.Fatalf("empty candidate = %#v", ready)
	}
}

func TestSyncRecoveryPruneCandidateRejectsInvalidPagesAndCheckpoints(t *testing.T) {
	snapshot := SyncRecoveryPruneSnapshot{
		Authority: SyncAuthorityBinding{
			ChannelID:              testSyncChannelID("recovery-prune-validation"),
			RelayGeneration:        testAuthorityDigest(0x61),
			AdminPublicKey:         testAuthorityDigest(0x62),
			MembershipGeneration:   3,
			InventoryArrivalHead:   8,
			AuthorityDigestVersion: 2,
			AuthorityDigest:        testAuthorityDigest(0x63),
		},
		PruneHead: 6,
	}
	projectID := continuity.ProjectID("project-recovery-prune-validation")
	valid := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x64)

	tests := []struct {
		name     string
		snapshot SyncRecoveryPruneSnapshot
		expected *SyncRecoveryPruneCandidateCheckpoint
		page     SyncRecoveryPruneCandidatePage
	}{
		{name: "negative prune head", snapshot: mutateSyncRecoveryPruneSnapshotV1(snapshot, func(value *SyncRecoveryPruneSnapshot) { value.PruneHead = -1 }), page: valid},
		{name: "prune head exceeds arrival", snapshot: mutateSyncRecoveryPruneSnapshotV1(snapshot, func(value *SyncRecoveryPruneSnapshot) { value.PruneHead = 9 }), page: valid},
		{name: "authority v1", snapshot: mutateSyncRecoveryPruneSnapshotV1(snapshot, func(value *SyncRecoveryPruneSnapshot) {
			value.Authority.AuthorityDigestVersion = 1
			value.Authority.InventoryArrivalHead = 0
			value.PruneHead = 0
		}), page: SyncRecoveryPruneCandidatePage{ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(1), InventoryDigest: testSyncRecoveryPruneInventoryDigestV1(2)}},
		{name: "wrong first cursor", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) { value.AfterPruneSequence = 1 })},
		{name: "oversized page", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) { value.PagePruneCount = 5 })},
		{name: "short nonfinal page", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) { value.PagePruneCount = 3 })},
		{name: "empty nonempty inventory", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) {
			value.PagePruneCount = 0
			value.PageTargetCount = 0
			value.LastMembershipGeneration = 0
			value.More = false
			value.InventoryDigest = testSyncRecoveryPruneInventoryDigestV1(3)
		})},
		{name: "too few targets", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) { value.PageTargetCount = 3 })},
		{name: "too many targets", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) {
			value.PageTargetCount = 4*maximumSyncRecoveryPruneTargetsV1 + 1
		})},
		{name: "generation zero", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) { value.LastMembershipGeneration = 0 })},
		{name: "generation above snapshot", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) { value.LastMembershipGeneration = 4 })},
		{name: "zero rolling digest", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) {
			value.ResultingRollingDigest = SyncRecoveryPruneRollingDigest{}
		})},
		{name: "nonfinal final digest", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) {
			value.InventoryDigest = testSyncRecoveryPruneInventoryDigestV1(4)
		})},
		{name: "nonfinal reaches head", snapshot: mutateSyncRecoveryPruneSnapshotV1(snapshot, func(value *SyncRecoveryPruneSnapshot) { value.PruneHead = 4 }), page: valid},
		{name: "final misses head", snapshot: snapshot, page: mutateSyncRecoveryPrunePageV1(valid, func(value *SyncRecoveryPruneCandidatePage) {
			value.More = false
			value.InventoryDigest = testSyncRecoveryPruneInventoryDigestV1(5)
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := prepareSyncRecoveryPruneCandidateSuccessorV1(projectID, test.snapshot, test.expected, test.page)
			if err == nil {
				t.Fatal("prepareSyncRecoveryPruneCandidateSuccessorV1() error = nil")
			}
			assertSyncErrorCode(t, err, SyncErrorInvalid)
		})
	}

	first, err := prepareSyncRecoveryPruneCandidateSuccessorV1(projectID, snapshot, nil, valid)
	if err != nil {
		t.Fatalf("prepare first checkpoint: %v", err)
	}
	final := testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0x70)
	ready := first.Checkpoint()
	ready.CandidateID = testAuthorityDigest(0x70)
	ready.Ready = true
	ready.InventoryDigest = testSyncRecoveryPruneInventoryDigestV1(0x71)
	if _, err := prepareSyncRecoveryPruneCandidateSuccessorV1(projectID, snapshot, &ready, final); err == nil {
		t.Fatal("ready predecessor error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestSyncRecoveryPruneCandidateDiscardCannotABARecreatedAttempt(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-discard-aba"), "discard-aba", 8, 6,
	)
	page := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x75)
	first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, page)
	if err != nil {
		t.Fatalf("stage first attempt: %v", err)
	}
	stale := first.Checkpoint()
	if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, stale); err != nil {
		t.Fatalf("discard first attempt: %v", err)
	}
	recreated, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, page)
	if err != nil {
		t.Fatalf("stage recreated attempt: %v", err)
	}
	if recreated.CandidateID == first.CandidateID {
		t.Fatal("recreated candidate reused the discarded attempt identity")
	}
	if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, stale); err == nil {
		t.Fatal("discard recreated candidate with stale checkpoint error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
	if err != nil || !found || current != recreated {
		t.Fatalf("CurrentSyncRecoveryPruneCandidate(after stale discard) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, recreated)
	}
}

func TestSyncRecoveryPruneCandidateFencesAuthorityWatermarkAndDownload(t *testing.T) {
	t.Run("download incomplete", func(t *testing.T) {
		store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
			t, filepath.Join(testTempDir(t), "state-recovery-prune-download-fence"), "download-fence", 8, 6,
		)
		if _, err := store.db.Exec(`
UPDATE continuity_sync_projects SET downloaded_cursor = 7 WHERE project_id = ?`, string(projectID)); err != nil {
			t.Fatalf("lower downloaded cursor: %v", err)
		}
		_, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
			context.Background(), projectID, snapshot, nil,
			testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x81),
		)
		assertSyncErrorCode(t, err, SyncErrorCursor)
	})

	t.Run("authority drift", func(t *testing.T) {
		store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
			t, filepath.Join(testTempDir(t), "state-recovery-prune-authority-fence"), "authority-fence", 8, 6,
		)
		firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x82)
		first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, firstPage)
		if err != nil {
			t.Fatalf("stage first page: %v", err)
		}
		driftedDigest := testAuthorityDigest(0x83)
		if _, err := store.db.Exec(`
UPDATE continuity_sync_authorities SET authority_digest = ? WHERE project_id = ?`, driftedDigest[:], string(projectID)); err != nil {
			t.Fatalf("drift canonical authority: %v", err)
		}
		current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
		if err != nil || !found || current != first {
			t.Fatalf("CurrentSyncRecoveryPruneCandidate(stale authority) = (%#v, %v, %v)", current, found, err)
		}
		expected := first.Checkpoint()
		_, err = store.StageVerifiedSyncRecoveryPruneCandidatePage(
			context.Background(), projectID, snapshot, &expected,
			testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0x84),
		)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, expected); err != nil {
			t.Fatalf("discard stale-authority candidate: %v", err)
		}
	})
}

func TestSyncRecoveryPruneCandidateFreezesMutableStagingPrefix(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-prefix-freeze"), "prefix-freeze", 8, 6,
	)
	first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil,
		testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x86),
	)
	if err != nil {
		t.Fatalf("stage recovery prune candidate: %v", err)
	}

	if progress, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, snapshot.Authority,
		snapshot.Authority.InventoryArrivalHead, snapshot.Authority.InventoryArrivalHead, nil,
	); err != nil || progress.DownloadedCursor != snapshot.Authority.InventoryArrivalHead {
		t.Fatalf("StageSyncPageUnderAuthority(exact no-op) = (%#v, %v)", progress, err)
	}
	assertActiveRecoveryPruneGateV1(t, func() error {
		_, err := store.StageSyncPage(
			context.Background(), projectID, snapshot.Authority.ChannelID,
			snapshot.Authority.InventoryArrivalHead, snapshot.Authority.InventoryArrivalHead, nil,
		)
		return err
	})
	assertActiveRecoveryPruneGateV1(t, func() error {
		_, err := store.ApplySyncBatch(
			context.Background(), projectID, snapshot.Authority, nil, 1_000, 100,
		)
		return err
	})
	assertActiveRecoveryPruneGateV1(t, func() error {
		_, err := store.ActivateStagedSync(context.Background(), projectID, snapshot.Authority)
		return err
	})
	assertActiveRecoveryPruneGateV1(t, func() error {
		return store.DiscardStagedSync(context.Background(), projectID, snapshot.Authority.ChannelID)
	})

	authoritySnapshot := SyncAuthoritySnapshot{
		ChannelID:                  snapshot.Authority.ChannelID,
		RelayGeneration:            snapshot.Authority.RelayGeneration,
		AdminPublicKey:             snapshot.Authority.AdminPublicKey,
		MembershipGeneration:       snapshot.Authority.MembershipGeneration,
		InventoryArrivalHead:       snapshot.Authority.InventoryArrivalHead,
		BaseAuthorityDigestVersion: snapshot.Authority.AuthorityDigestVersion,
		BaseAuthorityDigest:        snapshot.Authority.AuthorityDigest,
	}
	assertActiveRecoveryPruneGateV1(t, func() error {
		_, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, authoritySnapshot,
			syncAuthorityCandidatePageV2("", testSyncAuthority().Environments, false),
		)
		return err
	})
	recoverySnapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	assertActiveRecoveryPruneGateV1(t, func() error {
		_, err := store.BeginSyncAuthorityRecoveryTransition(
			context.Background(), projectID,
			SyncAuthorityRecoveryTransitionStart{
				WriterEnvironmentID:        store.WriterEnvironmentID(),
				WriterCertificateID:        sha256.Sum256([]byte("blocked recovery writer")),
				TargetMembershipGeneration: 1,
				SuccessorSnapshot:          recoverySnapshot,
			},
			syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), false),
		)
		return err
	})
	_, terminalFrames, _ := terminalHotPathFramesV2(t, projectID, testSyncAuthority().Environments[0], 1)
	assertActiveRecoveryPruneGateV1(t, func() error {
		_, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, snapshot.Authority, terminalFrames, 1_000, 100,
		)
		return err
	})
	current, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
	if err != nil || !found || current != first {
		t.Fatalf("frozen candidate changed = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, first)
	}
}

func TestSyncRecoveryPruneCandidateMutualExclusionIsBidirectional(t *testing.T) {
	t.Run("authority candidate blocks prune candidate", func(t *testing.T) {
		store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
			t, filepath.Join(testTempDir(t), "state-recovery-prune-authority-exclusion"), "authority-exclusion", 8, 6,
		)
		authoritySnapshot := SyncAuthoritySnapshot{
			ChannelID:                  snapshot.Authority.ChannelID,
			RelayGeneration:            snapshot.Authority.RelayGeneration,
			AdminPublicKey:             snapshot.Authority.AdminPublicKey,
			MembershipGeneration:       snapshot.Authority.MembershipGeneration,
			InventoryArrivalHead:       snapshot.Authority.InventoryArrivalHead,
			BaseAuthorityDigestVersion: snapshot.Authority.AuthorityDigestVersion,
			BaseAuthorityDigest:        snapshot.Authority.AuthorityDigest,
		}
		candidate, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, authoritySnapshot,
			syncAuthorityCandidatePageV2("", cloneSyncAuthorityCandidateEnvironmentsV2(testSyncAuthority().Environments), false),
		)
		if err != nil {
			t.Fatalf("stage authority candidate: %v", err)
		}
		_, err = store.StageVerifiedSyncRecoveryPruneCandidatePage(
			context.Background(), projectID, snapshot, nil,
			testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x8b),
		)
		assertSyncRecoveryPruneConflictFieldV1(t, err, "sync_authority_candidate")
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("authority candidate after refused prune = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, candidate)
		}
		assertNoSyncRecoveryPruneCandidateV1(t, store, projectID)
	})

	t.Run("recovery transition blocks prune candidate", func(t *testing.T) {
		store := openSyncStore(t, "recovery-prune-transition-exclusion")
		projectID := continuity.ProjectID("project-recovery-prune-transition-exclusion")
		authority := testSyncAuthority()
		authority.InventoryArrivalHead = 8
		digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
		binding := syncAuthorityBindingForTest(authority, 2, digest)
		if _, err := store.AdvanceSyncRelayWatermark(
			context.Background(), syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
		); err != nil {
			t.Fatalf("advance recovery-transition watermark: %v", err)
		}
		predecessorSnapshot := syncAuthoritySnapshotFromAuthorityV2(authority, 2, digest)
		predecessor, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, predecessorSnapshot,
			syncAuthorityCandidatePageV2("", cloneSyncAuthorityCandidateEnvironmentsV2(authority.Environments), false),
		)
		if err != nil {
			t.Fatalf("stage recovery predecessor: %v", err)
		}
		writer := SyncEnvironmentCertificate{
			EnvironmentID:            string(store.WriterEnvironmentID()),
			CertificateID:            sha256.Sum256([]byte("recovery prune exclusion writer")),
			CertificateBytes:         []byte("recovery prune exclusion writer certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: authority.MembershipGeneration + 1,
		}
		environments := append(cloneSyncAuthorityCandidateEnvironmentsV2(authority.Environments), writer)
		start := syncAuthorityRecoveryStartV1(
			predecessor, writer, authority.InventoryArrivalHead, authority.MembershipGeneration+1,
		)
		state, err := store.BeginSyncAuthorityRecoveryTransition(
			context.Background(), projectID, start,
			syncAuthorityCandidatePageV2("", environments, false),
		)
		if err != nil {
			t.Fatalf("begin recovery transition: %v", err)
		}
		_, err = store.StageVerifiedSyncRecoveryPruneCandidatePage(
			context.Background(), projectID,
			SyncRecoveryPruneSnapshot{Authority: binding, PruneHead: 0}, nil,
			SyncRecoveryPruneCandidatePage{
				ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(0x8c),
				InventoryDigest:        testSyncRecoveryPruneInventoryDigestV1(0x8d),
			},
		)
		assertSyncRecoveryPruneConflictFieldV1(t, err, "sync_authority_recovery_transition")
		current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
		if err != nil || !found || current != state {
			t.Fatalf("recovery transition after refused prune = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, state)
		}
		assertNoSyncRecoveryPruneCandidateV1(t, store, projectID)
	})

	t.Run("terminal candidate blocks prune candidate", func(t *testing.T) {
		store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(
			t, "recovery-prune-terminal-exclusion", 2,
		)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("stage terminal candidate: %v", err)
		}
		_, err = store.StageVerifiedSyncRecoveryPruneCandidatePage(
			context.Background(), projectID,
			SyncRecoveryPruneSnapshot{Authority: binding, PruneHead: 0}, nil,
			SyncRecoveryPruneCandidatePage{
				ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(0x8e),
				InventoryDigest:        testSyncRecoveryPruneInventoryDigestV1(0x8f),
			},
		)
		assertSyncRecoveryPruneConflictFieldV1(t, err, "terminal_candidate")
		current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("terminal candidate after refused prune = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, candidate)
		}
		assertNoSyncRecoveryPruneCandidateV1(t, store, projectID)
	})
}

func TestSyncRecoveryPruneCandidateAllowsNewerWatermarkButRejectsResume(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-watermark-advance"), "watermark-advance", 8, 6,
	)
	firstPage := testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x87)
	first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(context.Background(), projectID, snapshot, nil, firstPage)
	if err != nil {
		t.Fatalf("stage first page: %v", err)
	}
	newer := syncRelayWatermarkFromAuthorityBindingV1(projectID, snapshot.Authority)
	newer.RelayHead++
	if retained, err := store.AdvanceSyncRelayWatermark(context.Background(), newer); err != nil || retained != newer {
		t.Fatalf("AdvanceSyncRelayWatermark(newer) = (%#v, %v), want (%#v, nil)", retained, err, newer)
	}
	expected := first.Checkpoint()
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, &expected,
		testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0x88),
	); err == nil {
		t.Fatal("resume under advanced watermark error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorCursor)
	}
	if err := store.DiscardSyncRecoveryPruneCandidate(context.Background(), projectID, expected); err != nil {
		t.Fatalf("DiscardSyncRecoveryPruneCandidate(stale watermark) error = %v", err)
	}
}

func TestSyncRecoveryPruneCandidateRequiresStagingActivation(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-staging-only"), "staging-only", 8, 0,
	)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET activation_state = 'attached', applied_cursor = downloaded_cursor
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("seed attached progress: %v", err)
	}
	_, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil,
		SyncRecoveryPruneCandidatePage{
			ResultingRollingDigest: testSyncRecoveryPruneRollingDigestV1(0x89),
			InventoryDigest:        testSyncRecoveryPruneInventoryDigestV1(0x8a),
		},
	)
	if err == nil {
		t.Fatal("StageVerifiedSyncRecoveryPruneCandidatePage(attached) error = nil")
	}
	assertSyncErrorCode(t, err, SyncErrorConflict)
}

func TestCurrentSyncRecoveryPruneCandidateFailsClosedOnCorruption(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-corruption"), "corruption", 8, 6,
	)
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil,
		testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x91),
	); err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatalf("enable corruption fixture: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_recovery_prune_candidates
SET target_count = 5000
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("corrupt candidate: %v", err)
	}
	if _, _, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncRecoveryPruneCandidate(corrupt) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestCurrentSyncRecoveryPruneCandidateRejectsNonNullZeroInventoryDigest(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-zero-inventory-digest"), "zero-inventory-digest", 8, 6,
	)
	if _, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil,
		testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x92),
	); err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatalf("enable corruption fixture: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_recovery_prune_candidates
SET inventory_digest = zeroblob(32)
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("corrupt inventory digest: %v", err)
	}
	if _, _, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncRecoveryPruneCandidate(non-NULL zero inventory digest) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestCurrentSyncRecoveryPruneCandidateFailsClosedOnIndexCardinalityCorruption(t *testing.T) {
	store, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(
		t, filepath.Join(testTempDir(t), "state-recovery-prune-index-corruption"), "index-corruption", 8, 6,
	)
	first, err := store.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil,
		testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0x93),
	)
	if err != nil {
		t.Fatalf("stage candidate: %v", err)
	}
	if _, err := store.db.Exec(`
DELETE FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence = 1`,
		string(projectID), first.CandidateID[:],
	); err != nil {
		t.Fatalf("corrupt recovery prune target index: %v", err)
	}
	if _, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID); err == nil || found {
		t.Fatalf("CurrentSyncRecoveryPruneCandidate(corrupt index) = (found=%v, err=%v), want store error", found, err)
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestSyncRecoveryPruneCandidateCommitCancellationAfterAttemptIsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	commitCalls := 0
	err := commitSyncRecoveryPruneCandidatePageV1(ctx, func() error {
		commitCalls++
		cancel()
		return context.Canceled
	})
	if commitCalls != 1 {
		t.Fatalf("commit calls = %d, want 1", commitCalls)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("attempted commit error = %v, must retain unknown-outcome classification", err)
	}
	assertSyncErrorCode(t, err, SyncErrorStore)

	preCanceled, cancelBefore := context.WithCancel(context.Background())
	cancelBefore()
	commitCalls = 0
	err = commitSyncRecoveryPruneCandidatePageV1(preCanceled, func() error {
		commitCalls++
		return nil
	})
	if !errors.Is(err, context.Canceled) || commitCalls != 0 {
		t.Fatalf("pre-canceled commit = (%v, %d calls), want context cancellation before commit", err, commitCalls)
	}
}

func TestSyncRecoveryPruneCandidateConcurrentSuccessorUsesExactCAS(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state-recovery-prune-concurrent-cas")
	firstStore, projectID, snapshot := openSyncRecoveryPruneCandidateFixtureV1(t, stateRoot, "concurrent-cas", 8, 6)
	first, err := firstStore.StageVerifiedSyncRecoveryPruneCandidatePage(
		context.Background(), projectID, snapshot, nil,
		testSyncRecoveryPruneCandidatePageV1(0, 4, 4, 2, true, 0xa1),
	)
	if err != nil {
		t.Fatalf("stage first page: %v", err)
	}
	secondStore, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(second store) error = %v", err)
	}
	t.Cleanup(func() { secondStore.Close() })
	stores := [2]*Store{firstStore, secondStore}
	pages := [2]SyncRecoveryPruneCandidatePage{
		testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0xa2),
		testSyncRecoveryPruneCandidatePageV1(4, 2, 2, 3, false, 0xa3),
	}
	expected := first.Checkpoint()
	type result struct {
		candidate SyncRecoveryPruneCandidate
		err       error
	}
	results := make([]result, len(stores))
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index].candidate, results[index].err = stores[index].StageVerifiedSyncRecoveryPruneCandidatePage(
				context.Background(), projectID, snapshot, &expected, pages[index],
			)
		}(index)
	}
	close(start)
	group.Wait()

	successes := 0
	conflicts := 0
	for _, result := range results {
		switch {
		case result.err == nil:
			successes++
		case syncErrorCodeV1(result.err) == SyncErrorConflict:
			conflicts++
		default:
			t.Fatalf("concurrent result error = %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent outcomes = %d successes, %d conflicts", successes, conflicts)
	}
}

func openSyncRecoveryPruneCandidateFixtureV1(
	t *testing.T,
	stateRoot,
	suffix string,
	arrivalHead,
	pruneHead int64,
) (*Store, continuity.ProjectID, SyncRecoveryPruneSnapshot) {
	t.Helper()
	store, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(%s) error = %v", suffix, err)
	}
	t.Cleanup(func() { store.Close() })
	projectID := continuity.ProjectID("project-recovery-prune-" + syncSlug(suffix))
	authority := testSyncAuthority()
	authority.InventoryArrivalHead = arrivalHead
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(%s) error = %v", suffix, err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = ?, relay_head = ?
WHERE project_id = ?`, arrivalHead, arrivalHead, string(projectID)); err != nil {
		t.Fatalf("seed recovery prune progress: %v", err)
	}
	return store, projectID, SyncRecoveryPruneSnapshot{Authority: binding, PruneHead: pruneHead}
}

func testSyncRecoveryPruneCandidatePageV1(
	after,
	prunes,
	targets int64,
	membership uint32,
	more bool,
	seed byte,
) SyncRecoveryPruneCandidatePage {
	page := SyncRecoveryPruneCandidatePage{
		AfterPruneSequence:       after,
		PagePruneCount:           prunes,
		PageTargetCount:          targets,
		LastMembershipGeneration: membership,
		ResultingRollingDigest:   testSyncRecoveryPruneRollingDigestV1(seed),
		More:                     more,
		Records:                  testVerifiedSyncRecoveryPruneRecordsV1(after, prunes, targets, membership, seed),
	}
	if !more {
		page.InventoryDigest = testSyncRecoveryPruneInventoryDigestV1(seed + 1)
	}
	return page
}

func testVerifiedSyncRecoveryPruneRecordsV1(
	after,
	prunes,
	targets int64,
	membership uint32,
	seed byte,
) []VerifiedSyncRecoveryPruneRecord {
	if prunes <= 0 || targets < prunes {
		return nil
	}
	records := make([]VerifiedSyncRecoveryPruneRecord, prunes)
	remainingTargets := targets
	arrivalSequence := after
	for recordIndex := int64(0); recordIndex < prunes; recordIndex++ {
		remainingPrunes := prunes - recordIndex
		targetCount := int64(1)
		if extra := remainingTargets - remainingPrunes; extra > 0 {
			targetCount += extra
		}
		if targetCount > maximumSyncRecoveryPruneTargetsV1 {
			targetCount = maximumSyncRecoveryPruneTargetsV1
		}
		remainingTargets -= targetCount
		pruneSequence := after + recordIndex + 1
		record := VerifiedSyncRecoveryPruneRecord{
			PruneSequence:        pruneSequence,
			PruneID:              sha256.Sum256([]byte{seed, byte(pruneSequence), 0x11}),
			PruneCertificateID:   sha256.Sum256([]byte{seed, byte(pruneSequence), 0x12}),
			MembershipGeneration: membership,
			Targets:              make([]VerifiedSyncRecoveryPruneTarget, targetCount),
		}
		for targetIndex := int64(0); targetIndex < targetCount; targetIndex++ {
			arrivalSequence++
			envelopeDigest := sha256.Sum256([]byte{seed, byte(arrivalSequence), 0x21})
			certificateID := sha256.Sum256([]byte{seed, byte(arrivalSequence), 0x22})
			previousDigest := [32]byte{}
			if arrivalSequence > 1 {
				previousDigest = sha256.Sum256([]byte{seed, byte(arrivalSequence - 1), 0x21})
			}
			nonceDigest := sha256.Sum256([]byte{seed, byte(arrivalSequence), 0x23})
			var nonce [24]byte
			copy(nonce[:], nonceDigest[:len(nonce)])
			record.Targets[targetIndex] = VerifiedSyncRecoveryPruneTarget{
				Reference: VerifiedPruneReference{
					FactID:                 continuity.FactID(fmt.Sprintf("fact-recovery-%02x-%d", seed, arrivalSequence)),
					EnvironmentID:          continuity.EnvironmentID(fmt.Sprintf("environment-recovery-%02x", seed)),
					EnvironmentSequence:    arrivalSequence,
					ArrivalSequence:        arrivalSequence,
					EnvelopeDigest:         envelopeDigest,
					CertificateID:          certificateID,
					PreviousEnvelopeDigest: previousDigest,
					KeyGeneration:          1,
					Nonce:                  nonce,
				},
				FactKind: continuity.FactScratchpadMessageRecorded,
				HLC:      continuity.HybridTime{WallMillis: arrivalSequence},
			}
		}
		records[recordIndex] = record
	}
	return records
}

func cloneSyncRecoveryPrunePageV1(page SyncRecoveryPruneCandidatePage) SyncRecoveryPruneCandidatePage {
	clone := page
	clone.Records = make([]VerifiedSyncRecoveryPruneRecord, len(page.Records))
	for index := range page.Records {
		clone.Records[index] = page.Records[index]
		clone.Records[index].Targets = append([]VerifiedSyncRecoveryPruneTarget(nil), page.Records[index].Targets...)
	}
	return clone
}

func assertSyncRecoveryPruneIndexCountsV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	wantRecords,
	wantTargets int64,
) {
	t.Helper()
	var records, targets int64
	if err := store.db.QueryRow(`
SELECT COUNT(*) FROM continuity_sync_recovery_prune_records WHERE project_id = ?`,
		string(projectID),
	).Scan(&records); err != nil {
		t.Fatalf("count recovery prune records: %v", err)
	}
	if err := store.db.QueryRow(`
SELECT COUNT(*) FROM continuity_sync_recovery_prune_targets WHERE project_id = ?`,
		string(projectID),
	).Scan(&targets); err != nil {
		t.Fatalf("count recovery prune targets: %v", err)
	}
	if records != wantRecords || targets != wantTargets {
		t.Fatalf("recovery prune index counts = records %d targets %d, want %d/%d", records, targets, wantRecords, wantTargets)
	}
}

func testSyncRecoveryPruneRollingDigestV1(seed byte) SyncRecoveryPruneRollingDigest {
	return SyncRecoveryPruneRollingDigest(sha256.Sum256([]byte{seed, 0x01}))
}

func testSyncRecoveryPruneInventoryDigestV1(seed byte) SyncRecoveryPruneInventoryDigest {
	return SyncRecoveryPruneInventoryDigest(sha256.Sum256([]byte{seed, 0x02}))
}

func mutateSyncRecoveryPruneSnapshotV1(
	value SyncRecoveryPruneSnapshot,
	mutate func(*SyncRecoveryPruneSnapshot),
) SyncRecoveryPruneSnapshot {
	mutate(&value)
	return value
}

func mutateSyncRecoveryPrunePageV1(
	value SyncRecoveryPruneCandidatePage,
	mutate func(*SyncRecoveryPruneCandidatePage),
) SyncRecoveryPruneCandidatePage {
	mutate(&value)
	return value
}

func syncErrorCodeV1(err error) SyncErrorCode {
	var problem *SyncError
	if errors.As(err, &problem) {
		return problem.Code
	}
	return ""
}

func assertActiveRecoveryPruneGateV1(t *testing.T, call func() error) {
	t.Helper()
	err := call()
	if err == nil {
		t.Fatal("active recovery prune gate error = nil")
	}
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != "sync_recovery_prune_candidate" {
		t.Fatalf("active recovery prune gate error = %v, want conflict at sync_recovery_prune_candidate", err)
	}
}

func assertSyncRecoveryPruneConflictFieldV1(t *testing.T, err error, field string) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorConflict || problem.Field != field {
		t.Fatalf("recovery prune exclusion error = %v, want conflict at %s", err, field)
	}
}

func assertNoSyncRecoveryPruneCandidateV1(t *testing.T, store *Store, projectID continuity.ProjectID) {
	t.Helper()
	candidate, found, err := store.CurrentSyncRecoveryPruneCandidate(context.Background(), projectID)
	if err != nil || found || candidate != (SyncRecoveryPruneCandidate{}) {
		t.Fatalf("CurrentSyncRecoveryPruneCandidate() = (%#v, %v, %v), want absent", candidate, found, err)
	}
}
