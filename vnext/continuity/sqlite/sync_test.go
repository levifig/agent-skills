package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

func TestContinuityStoreExportsExactPersistedFact(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	projectID := continuity.ProjectID("project-export")
	receipt := mustAppendV1(t)(store.RegisterProject(
		context.Background(),
		projectID,
		"fact-project",
		continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf <private>"},
	))

	fact, err := store.ExportFact(context.Background(), receipt.FactID)
	if err != nil {
		t.Fatalf("ExportFact() error = %v", err)
	}
	if fact.WireVersion != continuitywire.Version1 ||
		fact.FactID != receipt.FactID ||
		fact.ProjectID != projectID ||
		fact.SubjectKind != continuity.RecordProjectIdentity ||
		fact.SubjectID != continuity.SubjectID(projectID) ||
		fact.FactKind != continuity.FactProjectRegistered ||
		fact.PayloadVersion != 1 ||
		fact.EnvironmentID != receipt.EnvironmentID ||
		fact.EnvironmentSequence != receipt.EnvironmentSequence ||
		fact.HLCWallMillis != receipt.Clock.WallMillis ||
		fact.HLCLogical != receipt.Clock.Logical ||
		fact.EnvelopeVersion != 1 {
		t.Fatalf("ExportFact() = %#v, want exact persisted fields", fact)
	}
	wantPayload, err := encodeProjectRegistrationV1(continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf <private>"})
	if err != nil {
		t.Fatalf("encode expected payload: %v", err)
	}
	if string(fact.CanonicalPayload) != string(wantPayload) {
		t.Fatalf("payload = %s, want exact %s", fact.CanonicalPayload, wantPayload)
	}
}

func TestContinuityStoreStagesContiguousOpaquePageAtomically(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-stage")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	frames := []OpaqueSyncFrame{
		testOpaqueFrame(1, "one"),
		testOpaqueFrame(2, "two"),
	}
	progress, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 4, frames)
	if err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	if progress.ChannelID != testSyncChannelID("channel-a") || progress.ActivationState != SyncActivationStaging || progress.DownloadedCursor != 2 || progress.AppliedCursor != 0 || progress.RelayHead != 4 {
		t.Fatalf("progress = %#v", progress)
	}
	pending, err := store.PendingSyncFrames(context.Background(), projectID, 16)
	if err != nil {
		t.Fatalf("PendingSyncFrames() error = %v", err)
	}
	if len(pending) != 2 || !opaqueSyncFrameEqual(pending[0], frames[0]) || !opaqueSyncFrameEqual(pending[1], frames[1]) {
		t.Fatalf("pending = %#v, want exact staged frames %#v", pending, frames)
	}

	_, err = store.StageSyncPage(
		context.Background(),
		projectID,
		testSyncChannelID("channel-a"),
		2,
		4,
		[]OpaqueSyncFrame{testOpaqueFrame(4, "gap")},
	)
	assertSyncErrorCode(t, err, SyncErrorArrivalGap)
	pendingAfterGap, err := store.PendingSyncFrames(context.Background(), projectID, 16)
	if err != nil {
		t.Fatalf("PendingSyncFrames(after gap) error = %v", err)
	}
	if len(pendingAfterGap) != 2 {
		t.Fatalf("pending after refused gap = %d, want unchanged 2", len(pendingAfterGap))
	}
	mismatched := testOpaqueFrame(3, "three")
	mismatched.EnvelopeDigest = sha256.Sum256([]byte("different sealed bytes"))
	_, err = store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 2, 4, []OpaqueSyncFrame{mismatched})
	assertSyncErrorCode(t, err, SyncErrorInvalid)

	_, err = store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-b"), 2, 4, nil)
	assertSyncErrorCode(t, err, SyncErrorConflict)
}

func TestContinuityStoreStagesAndRepagesTaggedOpaqueFrames(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "tagged-opaque-inbox")
	projectID := continuity.ProjectID("project-tagged-opaque-inbox")
	channelID := testSyncChannelID("channel-a")
	installTestSyncAuthority(t, store, projectID, channelID)
	frames := []OpaqueSyncFrame{
		testOpaqueFrame(1, "sealed-before-prune"),
		testOpaquePrunedFrame(2, "pruned-arrival"),
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, channelID, 0, 2, frames); err != nil {
		t.Fatalf("StageSyncPage(tagged) error = %v", err)
	}

	for _, read := range []struct {
		name string
		load func() ([]OpaqueSyncFrame, error)
	}{
		{name: "pending", load: func() ([]OpaqueSyncFrame, error) {
			return store.PendingSyncFrames(context.Background(), projectID, 2)
		}},
		{name: "after", load: func() ([]OpaqueSyncFrame, error) {
			return store.PendingSyncFramesAfter(context.Background(), projectID, 0, 2)
		}},
	} {
		got, err := read.load()
		if err != nil {
			t.Fatalf("%s tagged read error = %v", read.name, err)
		}
		if len(got) != len(frames) || !opaqueSyncFrameEqual(got[0], frames[0]) || !opaqueSyncFrameEqual(got[1], frames[1]) {
			t.Fatalf("%s tagged read = %#v, want %#v", read.name, got, frames)
		}
		got[1].PrunedArrival[0] ^= 0xff
	}
	reloaded, err := store.PendingSyncFramesAfter(context.Background(), projectID, 1, 1)
	if err != nil {
		t.Fatalf("reload pruned frame: %v", err)
	}
	if len(reloaded) != 1 || !opaqueSyncFrameEqual(reloaded[0], frames[1]) {
		t.Fatal("tagged read returned an alias into retained pruned bytes")
	}

	if _, err := store.StageSyncPage(context.Background(), projectID, channelID, 0, 2, frames); err != nil {
		t.Fatalf("StageSyncPage(exact tagged retry) error = %v", err)
	}
	conflicting := append([]OpaqueSyncFrame(nil), frames...)
	conflicting[1].PrunedArrival = []byte("different-pruned-arrival")
	_, err = store.StageSyncPage(context.Background(), projectID, channelID, 0, 2, conflicting)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	kindConflict := append([]OpaqueSyncFrame(nil), frames...)
	kindConflict[1].PrunedArrival = nil
	kindConflict[1].SealedEnvelope = []byte("original-envelope-pruned-arrival")
	_, err = store.StageSyncPage(context.Background(), projectID, channelID, 0, 2, kindConflict)
	assertSyncErrorCode(t, err, SyncErrorConflict)
}

func TestContinuityStoreRejectsInvalidOpaqueFrameUnionWithoutMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		frame OpaqueSyncFrame
	}{
		{name: "neither representation", frame: OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: sha256.Sum256([]byte("original"))}},
		{name: "both representations", frame: OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: sha256.Sum256([]byte("sealed")), SealedEnvelope: []byte("sealed"), PrunedArrival: []byte("pruned")}},
		{name: "empty sealed representation", frame: OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: sha256.Sum256(nil), SealedEnvelope: []byte{}}},
		{name: "empty pruned representation", frame: OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: sha256.Sum256([]byte("original")), PrunedArrival: []byte{}}},
		{name: "oversized pruned representation", frame: OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: sha256.Sum256([]byte("original")), PrunedArrival: make([]byte, maximumPrunedArrivalBytes+1)}},
		{name: "zero original envelope digest", frame: OpaqueSyncFrame{ArrivalSequence: 1, PrunedArrival: []byte("pruned")}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := openSyncStore(t, "invalid-opaque-union-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-invalid-opaque-union-" + syncSlug(test.name))
			channelID := testSyncChannelID("channel-a")
			installTestSyncAuthority(t, store, projectID, channelID)

			_, err := store.StageSyncPage(context.Background(), projectID, channelID, 0, 1, []OpaqueSyncFrame{test.frame})
			assertSyncErrorCode(t, err, SyncErrorInvalid)
			progress, progressErr := store.CurrentSyncProgress(context.Background(), projectID)
			if progressErr != nil {
				t.Fatalf("CurrentSyncProgress() error = %v", progressErr)
			}
			if progress.DownloadedCursor != 0 || progress.AppliedCursor != 0 || progress.RelayHead != 0 {
				t.Fatalf("progress after invalid union = %#v, want zero cursors", progress)
			}
			var inboxRows int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?`, string(projectID)).Scan(&inboxRows); err != nil {
				t.Fatalf("count inbox rows: %v", err)
			}
			if inboxRows != 0 {
				t.Fatalf("inbox rows after invalid union = %d, want 0", inboxRows)
			}
		})
	}
}

func TestContinuityStoreAcceptsMaximumPrunedArrivalBytes(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "maximum-pruned-arrival")
	projectID := continuity.ProjectID("project-maximum-pruned-arrival")
	channelID := testSyncChannelID("channel-a")
	installTestSyncAuthority(t, store, projectID, channelID)
	frame := testOpaquePrunedFrame(1, "maximum")
	frame.PrunedArrival = make([]byte, maximumPrunedArrivalBytes)
	for index := range frame.PrunedArrival {
		frame.PrunedArrival[index] = byte(index)
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, channelID, 0, 1, []OpaqueSyncFrame{frame}); err != nil {
		t.Fatalf("StageSyncPage(maximum pruned arrival) error = %v", err)
	}
	got, err := store.PendingSyncFrames(context.Background(), projectID, 1)
	if err != nil {
		t.Fatalf("PendingSyncFrames(maximum pruned arrival) error = %v", err)
	}
	if len(got) != 1 || !opaqueSyncFrameEqual(got[0], frame) {
		t.Fatalf("maximum pruned arrival round trip = %#v, want exact frame", got)
	}
}

func TestContinuityStoreOrdinaryApplyStillGatesPrunedArrival(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "ordinary-apply-pruned-gate")
	projectID := continuity.ProjectID("project-ordinary-apply-pruned-gate")
	channelID := testSyncChannelID("channel-a")
	installTestSyncAuthority(t, store, projectID, channelID)
	opaque := testOpaquePrunedFrame(1, "ordinary-gate")
	if _, err := store.StageSyncPage(context.Background(), projectID, channelID, 0, 1, []OpaqueSyncFrame{opaque}); err != nil {
		t.Fatalf("StageSyncPage(pruned) error = %v", err)
	}
	fact := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
	_, err := store.ApplySyncBatch(context.Background(), projectID, []VerifiedSyncFrame{{
		ArrivalSequence: 1,
		EnvelopeDigest:  opaque.EnvelopeDigest,
		CertificateID:   testSyncCertificateID("environment-a"),
		KeyGeneration:   1,
		Nonce:           testNonce("ordinary-apply-pruned-gate"),
		Fact:            fact,
	}}, 1_000, 100)
	assertContentFreeSyncCodeV1(t, err, SyncErrorTerminalHistoryRequired)
	assertAppliedCursor(t, store, projectID, 0)
}

func TestContinuityStoreRejectsDuplicateEnvelopeDigestAcrossStagedPages(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "duplicate-staged-digest")
	projectID := continuity.ProjectID("project-duplicate-staged-digest")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	first := testOpaqueFrame(1, "same-envelope")
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, []OpaqueSyncFrame{first}); err != nil {
		t.Fatalf("StageSyncPage(first) error = %v", err)
	}
	duplicate := first
	duplicate.ArrivalSequence = 2
	_, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 1, 2, []OpaqueSyncFrame{duplicate})
	assertSyncErrorCode(t, err, SyncErrorConflict)
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	if progress.DownloadedCursor != 1 || progress.AppliedCursor != 0 {
		t.Fatalf("progress after duplicate staged digest = %#v", progress)
	}
}

func TestContinuityStoreConvergenceAppliesCompleteUnionAcrossPermutations(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-concurrent")
	root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
	created := syncIdeaCreatedFact(t, projectID, "fact-idea", "idea-1", "environment-a", 2, 101, "Original")
	revisionA := syncIdeaRevisionFact(t, projectID, "fact-revision-a", "idea-1", "environment-a", 3, 102, "fact-idea", "Branch A")
	revisionB := syncIdeaRevisionFact(t, projectID, "fact-revision-b", "idea-1", "environment-b", 1, 102, "fact-idea", "Branch B")
	permutations := map[string][]continuitywire.Fact{
		"source-a-first": {root, created, revisionA, revisionB},
		"source-b-first": {revisionB, root, created, revisionA},
	}
	for name, facts := range permutations {
		name, facts := name, facts
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store := openSyncStore(t, "concurrent-"+name)
			verified := stageSyncFacts(t, store, projectID, 1, facts)

			progress, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
			if err != nil {
				t.Fatalf("ApplySyncBatch() error = %v", err)
			}
			if progress.DownloadedCursor != 4 || progress.AppliedCursor != 4 {
				t.Fatalf("progress = %#v, want both cursors at 4", progress)
			}
			snapshot, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{})
			if err != nil {
				t.Fatalf("Snapshot() error = %v", err)
			}
			if len(snapshot.CurrentIdeas.Ideas) != 1 || snapshot.CurrentIdeas.Ideas[0].Content.Label != "Branch B" {
				t.Fatalf("concurrent projection = %#v, want deterministic Branch B winner", snapshot.CurrentIdeas.Ideas)
			}
			var factCount, receiptCount, headCount int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&factCount); err != nil {
				t.Fatalf("count facts: %v", err)
			}
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, string(projectID)).Scan(&receiptCount); err != nil {
				t.Fatalf("count receipts: %v", err)
			}
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_environment_heads WHERE project_id = ?`, string(projectID)).Scan(&headCount); err != nil {
				t.Fatalf("count heads: %v", err)
			}
			if factCount != 4 || receiptCount != 4 || headCount != 2 {
				t.Fatalf("atomic rows = facts %d receipts %d heads %d, want 4,4,2", factCount, receiptCount, headCount)
			}
		})
	}
}

func TestContinuityStoreAcceptsExactLocalEchoAndRejectsImmutableConflicts(t *testing.T) {
	t.Parallel()

	t.Run("exact local echo", func(t *testing.T) {
		store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
		projectID := continuity.ProjectID("project-echo")
		mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
		fact, err := store.ExportFact(context.Background(), "fact-project")
		if err != nil {
			t.Fatalf("ExportFact() error = %v", err)
		}
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{fact})
		progress, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
		if err != nil {
			t.Fatalf("ApplySyncBatch(exact echo) error = %v", err)
		}
		if progress.AppliedCursor != 1 {
			t.Fatalf("applied cursor = %d, want 1", progress.AppliedCursor)
		}
		var facts int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&facts); err != nil {
			t.Fatalf("count exact echo facts: %v", err)
		}
		if facts != 1 {
			t.Fatalf("fact count after exact echo = %d, want 1", facts)
		}
	})

	t.Run("fact id conflict", func(t *testing.T) {
		store, projectID := storeWithAppliedRoot(t, "fact-id-conflict")
		conflict := syncProjectFact(t, projectID, "fact-project", "environment-b", 1, 101)
		verified := stageSyncFacts(t, store, projectID, 2, []continuitywire.Fact{conflict})
		_, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertAppliedCursor(t, store, projectID, 1)
	})

	t.Run("environment sequence conflict", func(t *testing.T) {
		store, projectID := storeWithAppliedRoot(t, "sequence-conflict")
		conflict := syncProjectFact(t, projectID, "fact-other", "environment-a", 1, 100)
		verified := stageSyncFacts(t, store, projectID, 2, []continuitywire.Fact{conflict})
		_, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertAppliedCursor(t, store, projectID, 1)
	})

	t.Run("fact id conflict across projects", func(t *testing.T) {
		store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-cross-project-fact"), "environment-local", 100)
		mustAppendV1(t)(store.RegisterProject(context.Background(), "project-a", "fact-shared", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "A"}))
		projectID := continuity.ProjectID("project-b")
		conflict := syncProjectFact(t, projectID, "fact-shared", "environment-b", 1, 101)
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{conflict})
		_, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorConflict)
		assertAppliedCursor(t, store, projectID, 0)
	})
}

func TestContinuityStoreRejectsSourceGapsAndNonIncreasingHLC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		facts func(*testing.T, continuity.ProjectID) []continuitywire.Fact
		code  SyncErrorCode
	}{
		{
			name: "leading environment gap",
			facts: func(t *testing.T, projectID continuity.ProjectID) []continuitywire.Fact {
				return []continuitywire.Fact{syncIdeaCreatedFact(t, projectID, "fact-idea", "idea-1", "environment-b", 2, 100, "Gap")}
			},
			code: SyncErrorEnvironmentGap,
		},
		{
			name: "interior environment gap",
			facts: func(t *testing.T, projectID continuity.ProjectID) []continuitywire.Fact {
				return []continuitywire.Fact{
					syncIdeaCreatedFact(t, projectID, "fact-idea", "idea-1", "environment-b", 1, 100, "One"),
					syncIdeaCreatedFact(t, projectID, "fact-idea-2", "idea-2", "environment-b", 3, 102, "Three"),
				}
			},
			code: SyncErrorEnvironmentGap,
		},
		{
			name: "non increasing hybrid clock",
			facts: func(t *testing.T, projectID continuity.ProjectID) []continuitywire.Fact {
				return []continuitywire.Fact{
					syncIdeaCreatedFact(t, projectID, "fact-idea", "idea-1", "environment-b", 1, 100, "One"),
					syncIdeaCreatedFact(t, projectID, "fact-idea-2", "idea-2", "environment-b", 2, 100, "Two"),
				}
			},
			code: SyncErrorHLC,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store, projectID := storeWithLocalRoot(t, test.name)
			facts := test.facts(t, projectID)
			verified := stageSyncFacts(t, store, projectID, 1, facts)
			_, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
			assertSyncErrorCode(t, err, test.code)
			assertAppliedCursor(t, store, projectID, 0)
			for _, fact := range facts {
				if _, err := store.ExportFact(context.Background(), fact.FactID); err == nil {
					t.Fatalf("invalid fact %q committed despite atomic refusal", fact.FactID)
				}
			}
		})
	}
}

func TestContinuityStoreValidatesCompleteCandidateUnionBeforeCommit(t *testing.T) {
	t.Parallel()

	store, projectID := storeWithLocalRoot(t, "candidate")
	valid := syncIdeaCreatedFact(t, projectID, "fact-valid", "idea-valid", "environment-b", 1, 100, "Valid")
	invalid := syncIdeaRevisionFact(t, projectID, "fact-invalid", "idea-invalid", "environment-b", 2, 101, "missing", "Invalid")
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{valid, invalid})

	_, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorCandidate)
	assertAppliedCursor(t, store, projectID, 0)
	if _, err := store.ExportFact(context.Background(), valid.FactID); err == nil {
		t.Fatal("valid prefix committed despite invalid complete candidate union")
	}
}

func TestContinuityStoreSkewQuarantinesFutureOnlyAndAcceptsOldOfflineFacts(t *testing.T) {
	t.Parallel()

	store, projectID := storeWithLocalRoot(t, "future")
	future := syncIdeaCreatedFact(t, projectID, "fact-future", "idea-future", "environment-b", 1, 100, "Future")
	later := syncIdeaCreatedFact(t, projectID, "fact-later", "idea-later", "environment-b", 2, 101, "Later")
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{future, later})

	progress, err := store.ApplySyncBatch(context.Background(), projectID, verified, 50, 10)
	if err != nil {
		t.Fatalf("ApplySyncBatch(future) error = %v", err)
	}
	if progress.DownloadedCursor != 2 || progress.AppliedCursor != 0 {
		t.Fatalf("future progress = %#v, want downloaded 2 applied 0", progress)
	}
	pending, err := store.PendingSyncFrames(context.Background(), projectID, 16)
	if err != nil {
		t.Fatalf("PendingSyncFrames() error = %v", err)
	}
	if len(pending) != 2 || !pending[0].Quarantined || pending[1].Quarantined {
		t.Fatalf("pending quarantine states = %#v, want first only quarantined", pending)
	}
	if _, err := store.ExportFact(context.Background(), future.FactID); err == nil {
		t.Fatal("future fact entered continuity corpus")
	}

	progress, err = store.ApplySyncBatch(context.Background(), projectID, verified, 200, 10)
	if err != nil {
		t.Fatalf("ApplySyncBatch(after clock catches up) error = %v", err)
	}
	if progress.AppliedCursor != 2 {
		t.Fatalf("applied cursor after retry = %d, want 2", progress.AppliedCursor)
	}
	if _, err := store.ExportFact(context.Background(), later.FactID); err != nil {
		t.Fatalf("old valid fact after quarantine retry was not applied: %v", err)
	}
}

func TestContinuityStoreRejectsEnvelopeChainGapCertificateChangeAndNonceReuse(t *testing.T) {
	t.Parallel()

	t.Run("previous digest chain", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "chain")
		first := syncIdeaCreatedFact(t, projectID, "fact-one", "idea-one", "environment-b", 1, 100, "One")
		firstVerified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{first})
		if _, err := store.ApplySyncBatch(context.Background(), projectID, firstVerified, 1_000, 100); err != nil {
			t.Fatalf("apply first envelope: %v", err)
		}
		second := syncIdeaCreatedFact(t, projectID, "fact-two", "idea-two", "environment-b", 2, 101, "Two")
		secondVerified := stageSyncFacts(t, store, projectID, 2, []continuitywire.Fact{second})
		secondVerified[0].PreviousEnvelopeDigest = sha256.Sum256([]byte("wrong previous envelope"))
		_, err := store.ApplySyncBatch(context.Background(), projectID, secondVerified, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorEnvelopeChain)
		assertAppliedCursor(t, store, projectID, 1)
	})

	t.Run("mint-once certificate", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "certificate")
		first := syncIdeaCreatedFact(t, projectID, "fact-one", "idea-one", "environment-b", 1, 100, "One")
		firstVerified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{first})
		if _, err := store.ApplySyncBatch(context.Background(), projectID, firstVerified, 1_000, 100); err != nil {
			t.Fatalf("apply first certificate: %v", err)
		}
		second := syncIdeaCreatedFact(t, projectID, "fact-two", "idea-two", "environment-b", 2, 101, "Two")
		secondVerified := stageSyncFacts(t, store, projectID, 2, []continuitywire.Fact{second})
		secondVerified[0].PreviousEnvelopeDigest = firstVerified[0].EnvelopeDigest
		secondVerified[0].CertificateID = sha256.Sum256([]byte("different certificate"))
		_, err := store.ApplySyncBatch(context.Background(), projectID, secondVerified, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorCertificate)
		assertAppliedCursor(t, store, projectID, 1)
	})

	t.Run("generation nonce reuse across environments", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "nonce")
		first := syncIdeaCreatedFact(t, projectID, "fact-one", "idea-one", "environment-b", 1, 100, "One")
		firstVerified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{first})
		if _, err := store.ApplySyncBatch(context.Background(), projectID, firstVerified, 1_000, 100); err != nil {
			t.Fatalf("apply first nonce: %v", err)
		}
		second := syncIdeaCreatedFact(t, projectID, "fact-two", "idea-two", "environment-a", 1, 101, "Two")
		secondVerified := stageSyncFacts(t, store, projectID, 2, []continuitywire.Fact{second})
		secondVerified[0].Nonce = firstVerified[0].Nonce
		_, err := store.ApplySyncBatch(context.Background(), projectID, secondVerified, 1_000, 100)
		assertSyncErrorCode(t, err, SyncErrorNonceReuse)
		assertAppliedCursor(t, store, projectID, 1)
	})
}

func TestContinuityStoreRejectsZeroEnvelopeDigestAndCertificateID(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "zero-envelope-metadata")
	projectID := continuity.ProjectID("project-zero-envelope-metadata")
	root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})

	zeroDigest := append([]VerifiedSyncFrame(nil), verified...)
	zeroDigest[0].EnvelopeDigest = [32]byte{}
	_, err := store.ApplySyncBatch(context.Background(), projectID, zeroDigest, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	assertAppliedCursor(t, store, projectID, 0)

	zeroCertificate := append([]VerifiedSyncFrame(nil), verified...)
	zeroCertificate[0].CertificateID = [32]byte{}
	_, err = store.ApplySyncBatch(context.Background(), projectID, zeroCertificate, 1_000, 100)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	assertAppliedCursor(t, store, projectID, 0)
}

func TestContinuityStoreStageRetryIsIdempotentAfterLostResponse(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "stage-retry")
	projectID := continuity.ProjectID("project-stage-retry")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	frames := []OpaqueSyncFrame{testOpaqueFrame(1, "one"), testOpaqueFrame(2, "two")}
	first, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, frames)
	if err != nil {
		t.Fatalf("first StageSyncPage() error = %v", err)
	}
	replayed, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, frames)
	if err != nil {
		t.Fatalf("replayed StageSyncPage() error = %v", err)
	}
	if replayed != first {
		t.Fatalf("replayed progress = %#v, want %#v", replayed, first)
	}
	pending, err := store.PendingSyncFrames(context.Background(), projectID, 16)
	if err != nil {
		t.Fatalf("PendingSyncFrames() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending after exact retry = %d, want 2", len(pending))
	}

	conflicting := append([]OpaqueSyncFrame(nil), frames...)
	conflicting[0].SealedEnvelope = []byte("different sealed bytes")
	_, err = store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 2, conflicting)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
}

func TestContinuityStoreStageRetryRecoversAfterAppliedResponseLoss(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "stage-retry-applied")
	projectID := continuity.ProjectID("project-stage-retry-applied")
	root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})
	staged, err := store.PendingSyncFrames(context.Background(), projectID, 16)
	if err != nil {
		t.Fatalf("PendingSyncFrames() error = %v", err)
	}
	if _, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100); err != nil {
		t.Fatalf("ApplySyncBatch() error = %v", err)
	}
	replayed, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, staged)
	if err != nil {
		t.Fatalf("StageSyncPage(applied retry) error = %v", err)
	}
	if replayed.DownloadedCursor != 1 || replayed.AppliedCursor != 1 {
		t.Fatalf("applied retry progress = %#v, want both cursors at 1", replayed)
	}
	relabeled := append([]OpaqueSyncFrame(nil), staged...)
	relabeled[0].SealedEnvelope = nil
	relabeled[0].PrunedArrival = []byte("unbound-pruned-wrapper")
	_, err = store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, relabeled)
	assertSyncErrorCode(t, err, SyncErrorConflict)
}

func TestContinuityStoreAttachActivationIsExplicitAtomicAndAbortable(t *testing.T) {
	t.Parallel()

	t.Run("staged channel can be discarded and rebound before apply", func(t *testing.T) {
		store := openSyncStore(t, "activation-rebind")
		projectID := continuity.ProjectID("project-activation-rebind")
		bogus := []OpaqueSyncFrame{testOpaqueFrame(1, "bogus")}
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-bogus"))
		progress, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-bogus"), 0, 1, bogus)
		if err != nil {
			t.Fatalf("StageSyncPage(bogus) error = %v", err)
		}
		if progress.ActivationState != SyncActivationStaging {
			t.Fatalf("staged activation state = %q", progress.ActivationState)
		}
		if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-bogus")); err == nil {
			t.Fatal("ActivateStagedSync(incomplete) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorActivation)
		}
		if err := store.DiscardStagedSync(context.Background(), projectID, testSyncChannelID("channel-bogus")); err != nil {
			t.Fatalf("DiscardStagedSync() error = %v", err)
		}
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-good"))
		progress, err = store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-good"), 0, 0, nil)
		if err != nil {
			t.Fatalf("StageSyncPage(rebind) error = %v", err)
		}
		if progress.ChannelID != testSyncChannelID("channel-good") || progress.ActivationState != SyncActivationStaging {
			t.Fatalf("rebound progress = %#v", progress)
		}
		if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-good")); err == nil {
			t.Fatal("ActivateStagedSync(without root) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorActivation)
		}
	})

	t.Run("complete verified inventory activates terminal channel", func(t *testing.T) {
		store := openSyncStore(t, "activation-complete")
		projectID := continuity.ProjectID("project-activation-complete")
		root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})
		if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err == nil {
			t.Fatal("ActivateStagedSync(unapplied) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorActivation)
		}
		if _, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100); err != nil {
			t.Fatalf("ApplySyncBatch() error = %v", err)
		}
		progress, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a"))
		if err != nil {
			t.Fatalf("ActivateStagedSync() error = %v", err)
		}
		if progress.ActivationState != SyncActivationAttached || progress.AppliedCursor != 1 || progress.RelayHead != 1 {
			t.Fatalf("activated progress = %#v", progress)
		}
		retry, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a"))
		if err != nil || retry != progress {
			t.Fatalf("activation retry = %#v, %v; want %#v", retry, err, progress)
		}
		if err := store.DiscardStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err == nil {
			t.Fatal("DiscardStagedSync(attached) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
	})

	t.Run("relay head is not activation authority", func(t *testing.T) {
		store := openSyncStore(t, "activation-hostile-head")
		projectID := continuity.ProjectID("project-activation-hostile-head")
		root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
		verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})
		if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 1, math.MaxInt64, nil); err != nil {
			t.Fatalf("StageSyncPage(hostile head) error = %v", err)
		}
		if _, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100); err != nil {
			t.Fatalf("ApplySyncBatch() error = %v", err)
		}
		progress, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a"))
		if err != nil {
			t.Fatalf("ActivateStagedSync(hostile relay head) error = %v", err)
		}
		if progress.ActivationState != SyncActivationAttached || progress.DownloadedCursor != 1 || progress.AppliedCursor != 1 || progress.RelayHead != math.MaxInt64 {
			t.Fatalf("hostile-head activation progress = %#v", progress)
		}
	})

	t.Run("empty verified relay activates existing local root", func(t *testing.T) {
		store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-activation-empty"), "environment-local", 100)
		projectID := continuity.ProjectID("project-activation-empty")
		mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-empty"))
		if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-empty"), 0, 0, nil); err != nil {
			t.Fatalf("StageSyncPage(empty) error = %v", err)
		}
		progress, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-empty"))
		if err != nil {
			t.Fatalf("ActivateStagedSync(empty) error = %v", err)
		}
		if progress.ActivationState != SyncActivationAttached {
			t.Fatalf("empty activation state = %q", progress.ActivationState)
		}
	})
}

func TestContinuityStoreConvergenceInsertsCanonicalRootBeforeLaterArrival(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "root-later")
	projectID := continuity.ProjectID("project-root-later")
	idea := syncIdeaCreatedFact(t, projectID, "fact-idea", "idea-one", "environment-b", 1, 101, "Idea")
	root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 102)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{idea, root})
	if _, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100); err != nil {
		t.Fatalf("ApplySyncBatch(root arrived later) error = %v", err)
	}
	if _, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{}); err != nil {
		t.Fatalf("Snapshot() after canonical insertion error = %v", err)
	}
}

func TestContinuityStorePersistsLocalEnvelopeOnceAndConsumesEcho(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-local", 100)
	projectID := continuity.ProjectID("project-outbox")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 0, nil); err != nil {
		t.Fatalf("stage sync project: %v", err)
	}
	if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err != nil {
		t.Fatalf("activate sync project: %v", err)
	}
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	if progress.ChannelID != testSyncChannelID("channel-a") || progress.DownloadedCursor != 0 || progress.AppliedCursor != 0 {
		t.Fatalf("CurrentSyncProgress() = %#v", progress)
	}
	unsealed, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil {
		t.Fatalf("NextUnsealedLocalFact() error = %v", err)
	}
	if !found || unsealed.Fact.FactID != "fact-project" || unsealed.PreviousEnvelopeDigest != [32]byte{} {
		t.Fatalf("initial unsealed fact = %#v, found %v", unsealed, found)
	}
	sealed := []byte("sealed local project fact")
	outbox := SealedOutboxFrame{
		FactID:         "fact-project",
		EnvelopeDigest: sha256.Sum256(sealed),
		CertificateID:  sha256.Sum256([]byte("local certificate")),
		KeyGeneration:  1,
		Nonce:          testNonce("local nonce"),
		SealedEnvelope: sealed,
	}
	mismatchedOutbox := outbox
	mismatchedOutbox.EnvelopeDigest = sha256.Sum256([]byte("different sealed local fact"))
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), mismatchedOutbox); err == nil {
		t.Fatal("PersistSealedOutbox(digest mismatch) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorInvalid)
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), outbox); err != nil {
		t.Fatalf("PersistSealedOutbox() error = %v", err)
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), outbox); err != nil {
		t.Fatalf("PersistSealedOutbox(exact retry) error = %v", err)
	}
	pendingOutbox, err := store.PendingSealedOutbox(context.Background(), projectID, 16)
	if err != nil {
		t.Fatalf("PendingSealedOutbox() error = %v", err)
	}
	if len(pendingOutbox) != 1 || !sealedOutboxFrameEqual(pendingOutbox[0], outbox) {
		t.Fatalf("pending outbox = %#v, want exact %#v", pendingOutbox, outbox)
	}
	mustAppendV1(t)(store.CreateIdea(context.Background(), projectID, "fact-idea", "idea-one", continuity.IdeaCreatedPayload{Observation: appendObservationV1(), Content: continuity.IdeaContent{Label: "Idea"}}))
	next, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil {
		t.Fatalf("NextUnsealedLocalFact(after local append) error = %v", err)
	}
	if !found || next.Fact.FactID != "fact-idea" || next.PreviousEnvelopeDigest != outbox.EnvelopeDigest {
		t.Fatalf("next unsealed fact = %#v, found %v", next, found)
	}
	conflict := outbox
	conflict.Nonce = testNonce("different nonce")
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), conflict); err == nil {
		t.Fatal("PersistSealedOutbox(conflict) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}

	fact, err := store.ExportFact(context.Background(), "fact-project")
	if err != nil {
		t.Fatalf("ExportFact() error = %v", err)
	}
	opaque := OpaqueSyncFrame{ArrivalSequence: 1, EnvelopeDigest: outbox.EnvelopeDigest, SealedEnvelope: outbox.SealedEnvelope}
	// The authority was installed before the initial empty stage and remains pinned.
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 1, []OpaqueSyncFrame{opaque}); err != nil {
		t.Fatalf("stage local echo: %v", err)
	}
	verified := VerifiedSyncFrame{
		ArrivalSequence:        1,
		PreviousEnvelopeDigest: outbox.PreviousEnvelopeDigest,
		EnvelopeDigest:         outbox.EnvelopeDigest,
		CertificateID:          outbox.CertificateID,
		KeyGeneration:          outbox.KeyGeneration,
		Nonce:                  outbox.Nonce,
		Fact:                   fact,
	}
	if _, err := store.ApplySyncBatch(context.Background(), projectID, []VerifiedSyncFrame{verified}, 1_000, 100); err != nil {
		t.Fatalf("apply local echo: %v", err)
	}
	var outboxRows, inboxRows, receiptRows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_outbox WHERE fact_id = 'fact-project'`).Scan(&outboxRows); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?`, string(projectID)).Scan(&inboxRows); err != nil {
		t.Fatalf("count inbox: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, string(projectID)).Scan(&receiptRows); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	if outboxRows != 0 || inboxRows != 0 || receiptRows != 1 {
		t.Fatalf("consumed echo rows = outbox %d inbox %d receipts %d, want 0,0,1", outboxRows, inboxRows, receiptRows)
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), outbox); err != nil {
		t.Fatalf("PersistSealedOutbox(consumed exact retry) error = %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_outbox WHERE fact_id = 'fact-project'`).Scan(&outboxRows); err != nil {
		t.Fatalf("count outbox after consumed retry: %v", err)
	}
	if outboxRows != 0 {
		t.Fatalf("consumed exact retry recreated %d outbox rows", outboxRows)
	}
	consumedConflict := outbox
	consumedConflict.Nonce = testNonce("consumed conflicting nonce")
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), consumedConflict); err == nil {
		t.Fatal("PersistSealedOutbox(consumed conflict) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestContinuityStoreGapRefusesExhaustedLocalSealedSequence(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-exhausted-sequence"), "environment-local", 100)
	projectID := continuity.ProjectID("project-exhausted-sequence")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 0, nil); err != nil {
		t.Fatalf("stage sync project: %v", err)
	}
	if _, err := store.ActivateStagedSync(context.Background(), projectID, testSyncChannelID("channel-a")); err != nil {
		t.Fatalf("activate sync project: %v", err)
	}
	previous := sha256.Sum256([]byte("previous"))
	digest := sha256.Sum256([]byte("digest"))
	certificateID := sha256.Sum256([]byte("certificate"))
	nonce := testNonce("nonce")
	if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET highest_sequence = ?, sealed_sequence = ?, previous_envelope_digest = ?,
    envelope_digest = ?, certificate_id = ?, key_generation = 1, nonce = ?
WHERE project_id = ? AND environment_id = ?`,
		math.MaxInt64,
		math.MaxInt64,
		previous[:],
		digest[:],
		certificateID[:],
		nonce[:],
		string(projectID),
		"environment-local",
	); err != nil {
		t.Fatalf("seed exhausted environment head: %v", err)
	}
	_, _, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	assertSyncErrorCode(t, err, SyncErrorEnvironmentGap)
}

func openSyncStore(t *testing.T, suffix string) *Store {
	t.Helper()
	store, err := Open(filepath.Join(testTempDir(t), "state-"+suffix), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func storeWithLocalRoot(t *testing.T, suffix string) (*Store, continuity.ProjectID) {
	t.Helper()
	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-"+suffix), "environment-local", 10)
	projectID := continuity.ProjectID("project-" + syncSlug(suffix))
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, continuity.FactID("fact-local-"+syncSlug(suffix)), continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Local"}))
	return store, projectID
}

func storeWithAppliedRoot(t *testing.T, suffix string) (*Store, continuity.ProjectID) {
	t.Helper()
	store := openSyncStore(t, suffix)
	projectID := continuity.ProjectID("project-" + syncSlug(suffix))
	root := syncProjectFact(t, projectID, "fact-project", "environment-a", 1, 100)
	verified := stageSyncFacts(t, store, projectID, 1, []continuitywire.Fact{root})
	if _, err := store.ApplySyncBatch(context.Background(), projectID, verified, 1_000, 100); err != nil {
		t.Fatalf("apply root: %v", err)
	}
	return store, projectID
}

func stageSyncFacts(t *testing.T, store *Store, projectID continuity.ProjectID, firstArrival int64, facts []continuitywire.Fact) []VerifiedSyncFrame {
	t.Helper()
	opaque := make([]OpaqueSyncFrame, 0, len(facts))
	verified := make([]VerifiedSyncFrame, 0, len(facts))
	previousByEnvironment := make(map[continuity.EnvironmentID][32]byte)
	for index, fact := range facts {
		arrival := firstArrival + int64(index)
		encoded, err := continuitywire.Encode(fact)
		if err != nil {
			t.Fatalf("encode staged fact %q: %v", fact.FactID, err)
		}
		sealed := append([]byte("sealed:"), encoded...)
		digest := sha256.Sum256(sealed)
		previous, found := previousByEnvironment[fact.EnvironmentID]
		if !found && fact.EnvironmentSequence > 1 {
			previous = sha256.Sum256([]byte("missing previous envelope"))
		}
		certificateID := sha256.Sum256([]byte("certificate:" + string(fact.EnvironmentID)))
		nonce := testNonce(string(projectID) + ":" + string(fact.EnvironmentID) + ":" + strconv.FormatInt(fact.EnvironmentSequence, 10))
		opaque = append(opaque, OpaqueSyncFrame{ArrivalSequence: arrival, EnvelopeDigest: digest, SealedEnvelope: sealed})
		verified = append(verified, VerifiedSyncFrame{
			ArrivalSequence:        arrival,
			PreviousEnvelopeDigest: previous,
			EnvelopeDigest:         digest,
			CertificateID:          certificateID,
			KeyGeneration:          1,
			Nonce:                  nonce,
			Fact:                   fact,
		})
		previousByEnvironment[fact.EnvironmentID] = digest
	}
	relayHead := firstArrival + int64(len(facts)) - 1
	if len(facts) == 0 {
		relayHead = firstArrival - 1
	}
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), firstArrival-1, relayHead, opaque); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	return verified
}

func testNonce(value string) [24]byte {
	digest := sha256.Sum256([]byte(value))
	var nonce [24]byte
	copy(nonce[:], digest[:len(nonce)])
	return nonce
}

func testOpaqueFrame(arrival int64, value string) OpaqueSyncFrame {
	sealed := []byte("sealed-" + value)
	return OpaqueSyncFrame{ArrivalSequence: arrival, EnvelopeDigest: sha256.Sum256(sealed), SealedEnvelope: sealed}
}

func testOpaquePrunedFrame(arrival int64, value string) OpaqueSyncFrame {
	return OpaqueSyncFrame{
		ArrivalSequence: arrival,
		EnvelopeDigest:  sha256.Sum256([]byte("original-envelope-" + value)),
		PrunedArrival:   []byte("pruned-" + value),
	}
}

func opaqueSyncFrameEqual(left, right OpaqueSyncFrame) bool {
	return left.ArrivalSequence == right.ArrivalSequence &&
		left.EnvelopeDigest == right.EnvelopeDigest &&
		string(left.SealedEnvelope) == string(right.SealedEnvelope) &&
		string(left.PrunedArrival) == string(right.PrunedArrival) &&
		left.Quarantined == right.Quarantined
}

func sealedOutboxFrameEqual(left, right SealedOutboxFrame) bool {
	return left.FactID == right.FactID &&
		left.PreviousEnvelopeDigest == right.PreviousEnvelopeDigest &&
		left.EnvelopeDigest == right.EnvelopeDigest &&
		left.CertificateID == right.CertificateID &&
		left.KeyGeneration == right.KeyGeneration &&
		left.Nonce == right.Nonce &&
		string(left.SealedEnvelope) == string(right.SealedEnvelope)
}

func syncProjectFact(t *testing.T, projectID continuity.ProjectID, factID continuity.FactID, environmentID continuity.EnvironmentID, sequence, wall int64) continuitywire.Fact {
	t.Helper()
	content, err := encodeProjectRegistrationV1(continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"})
	if err != nil {
		t.Fatalf("encode project fact: %v", err)
	}
	return syncFact(projectID, factID, continuity.RecordProjectIdentity, continuity.SubjectID(projectID), continuity.FactProjectRegistered, content, environmentID, sequence, wall)
}

func syncIdeaCreatedFact(t *testing.T, projectID continuity.ProjectID, factID continuity.FactID, subjectID continuity.SubjectID, environmentID continuity.EnvironmentID, sequence, wall int64, label string) continuitywire.Fact {
	t.Helper()
	content, err := encodeIdeaCreatedV1(continuity.IdeaCreatedPayload{Observation: appendObservationV1(), Content: continuity.IdeaContent{Label: label}})
	if err != nil {
		t.Fatalf("encode idea fact: %v", err)
	}
	return syncFact(projectID, factID, continuity.RecordIdea, subjectID, continuity.FactIdeaCreated, content, environmentID, sequence, wall)
}

func syncIdeaRevisionFact(t *testing.T, projectID continuity.ProjectID, factID continuity.FactID, subjectID continuity.SubjectID, environmentID continuity.EnvironmentID, sequence, wall int64, revises continuity.FactID, label string) continuitywire.Fact {
	t.Helper()
	content, err := encodeIdeaRevisionV1(continuity.IdeaRevisionPayload{Observation: appendObservationV1(), Revises: revises, Content: continuity.IdeaContent{Label: label}})
	if err != nil {
		t.Fatalf("encode idea revision: %v", err)
	}
	return syncFact(projectID, factID, continuity.RecordIdea, subjectID, continuity.FactIdeaRevised, content, environmentID, sequence, wall)
}

func syncFact(projectID continuity.ProjectID, factID continuity.FactID, subjectKind continuity.RecordKind, subjectID continuity.SubjectID, factKind continuity.FactKind, content canonicalContentV1, environmentID continuity.EnvironmentID, sequence, wall int64) continuitywire.Fact {
	return continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              factID,
		ProjectID:           projectID,
		SubjectKind:         subjectKind,
		SubjectID:           subjectID,
		FactKind:            factKind,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(content),
		EnvironmentID:       environmentID,
		EnvironmentSequence: sequence,
		HLCWallMillis:       wall,
		EnvelopeVersion:     1,
	}
}

func assertSyncErrorCode(t *testing.T, err error, code SyncErrorCode) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *SyncError code %q", err, code)
	}
	if problem.Code != code {
		t.Fatalf("error code = %q, want %q (error %v)", problem.Code, code, err)
	}
}

func assertAppliedCursor(t *testing.T, store *Store, projectID continuity.ProjectID, want int64) {
	t.Helper()
	var got int64
	if err := store.db.QueryRow(`SELECT applied_cursor FROM continuity_sync_projects WHERE project_id = ?`, string(projectID)).Scan(&got); err != nil {
		t.Fatalf("read applied cursor: %v", err)
	}
	if got != want {
		t.Fatalf("applied cursor = %d, want %d", got, want)
	}
}

func syncSlug(value string) string {
	result := make([]byte, 0, len(value))
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			result = append(result, character)
		}
	}
	return string(result)
}
