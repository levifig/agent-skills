package coordinator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestDownloadRecoverySnapshotStagesExactSealedAndPrunedPages(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 2)
	first := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
	second := testRecoveryDownloadArrival(t, fixture.prepared, 2, 2, protocol.Digest(first.EnvelopeDigest))
	pruneID := relay.Digest(testArray32(0xb1))
	prunedAt := time.UnixMilli(2_000).UTC()
	second.Ciphertext = nil
	second.PruneID = &pruneID
	second.PrunedAt = &prunedAt
	arrivals := []relay.Arrival{first, second}
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		page := relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.binding.MembershipGeneration,
			Head:                 fixture.binding.InventoryArrivalHead,
		}
		switch request.After {
		case 0:
			page.Arrivals = append([]relay.Arrival(nil), arrivals[:1]...)
		case 1:
			page.Arrivals = append([]relay.Arrival(nil), arrivals[1:]...)
		default:
			t.Fatalf("unexpected page cursor %d", request.After)
		}
		return page, nil
	}

	progress, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("download recovery snapshot: %v", err)
	}
	if progress.ActivationState != continuitysqlite.SyncActivationStaging ||
		progress.DownloadedCursor != 2 || progress.AppliedCursor != 0 || progress.RelayHead != 2 {
		t.Fatalf("download progress = %#v, want staged 2/0 at head 2", progress)
	}
	if len(fixture.remote.pageRequests) != 2 {
		t.Fatalf("page requests = %d, want 2", len(fixture.remote.pageRequests))
	}
	wantAuthorization := recoveryDownloadAuthorization(fixture.prepared)
	for index, request := range fixture.remote.pageRequests {
		wantLimit := 2 - index
		if request.Authorization != wantAuthorization || request.After != int64(index) || request.Limit != wantLimit {
			t.Fatalf("page request %d did not use the exact authorization, cursor, and bounded remaining limit", index)
		}
	}

	pending, err := fixture.store.PendingSyncFramesAfter(context.Background(), fixture.recovery.ProjectID, 0, 2)
	if err != nil {
		t.Fatalf("PendingSyncFramesAfter() error = %v", err)
	}
	wantSealed, err := sealedRecoveryArrivalBytes(first)
	if err != nil {
		t.Fatalf("sealed recovery arrival bytes: %v", err)
	}
	wantPruned, err := prunedRecoveryArrivalBytes(second, fixture.prepared.RelayGeneration)
	if err != nil {
		t.Fatalf("pruned recovery arrival bytes: %v", err)
	}
	if len(pending) != 2 || pending[0].ArrivalSequence != 1 || pending[0].EnvelopeDigest != [32]byte(first.EnvelopeDigest) ||
		!bytes.Equal(pending[0].SealedEnvelope, wantSealed) || pending[0].PrunedArrival != nil ||
		pending[1].ArrivalSequence != 2 || pending[1].EnvelopeDigest != [32]byte(second.EnvelopeDigest) ||
		pending[1].SealedEnvelope != nil || !bytes.Equal(pending[1].PrunedArrival, wantPruned) {
		t.Fatalf("pending recovery frames = %#v, want exact sealed/pruned bytes", pending)
	}

	replayed, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("download recovery snapshot exact retry: %v", err)
	}
	if replayed != progress || len(fixture.remote.pageRequests) != 2 {
		t.Fatalf("exact retry = (%#v, calls=%d), want (%#v, calls=2)", replayed, len(fixture.remote.pageRequests), progress)
	}
}

func TestDownloadRecoverySnapshotRejectsDriftAndMalformedPagesWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutatePage func(*relay.Page)
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{
			name: "forward snapshot",
			mutatePage: func(page *relay.Page) {
				page.Head++
			},
			wantCode: CodeConflict, wantAction: ActionRetry,
		},
		{
			name: "relay generation",
			mutatePage: func(page *relay.Page) {
				page.RelayGeneration[0] ^= 0xff
			},
			wantCode: CodeRemote, wantAction: ActionRestartRecovery,
		},
		{
			name: "empty prefix",
			mutatePage: func(page *relay.Page) {
				page.Arrivals = nil
			},
			wantCode: CodeRemote, wantAction: ActionRestartRecovery,
		},
		{
			name: "arrival gap",
			mutatePage: func(page *relay.Page) {
				page.Arrivals[0].ArrivalSequence = 2
			},
			wantCode: CodeRemote, wantAction: ActionRestartRecovery,
		},
		{
			name: "sealed digest",
			mutatePage: func(page *relay.Page) {
				page.Arrivals[0].EnvelopeDigest[0] ^= 0xff
			},
			wantCode: CodeRemote, wantAction: ActionRestartRecovery,
		},
		{
			name: "mixed prune marker",
			mutatePage: func(page *relay.Page) {
				pruneID := relay.Digest(testArray32(0xc1))
				page.Arrivals[0].PruneID = &pruneID
			},
			wantCode: CodeRemote, wantAction: ActionRestartRecovery,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 1)
			arrival := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
			fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
				page := relay.Page{
					RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
					MembershipGeneration: fixture.binding.MembershipGeneration,
					Head:                 fixture.binding.InventoryArrivalHead,
					Arrivals:             []relay.Arrival{arrival},
				}
				test.mutatePage(&page)
				return page, nil
			}

			_, err := fixture.coordinator.downloadRecoverySnapshot(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			)
			assertProblem(t, err, test.wantCode, PhaseAttachDownload, test.wantAction)
			progress, readErr := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
			if readErr != nil {
				t.Fatalf("CurrentSyncProgress() error = %v", readErr)
			}
			if progress.DownloadedCursor != 0 || progress.AppliedCursor != 0 || progress.RelayHead != 0 {
				t.Fatalf("progress after refused page = %#v, want untouched", progress)
			}
		})
	}
}

func TestDownloadRecoverySnapshotMapsRemoteFailuresWithoutLeakingCauses(t *testing.T) {
	const secretMarker = "download-secret-marker"
	for _, test := range []struct {
		name       string
		remoteErr  error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "unauthenticated", remoteErr: relay.ErrUnauthenticated, wantCode: CodeAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "missing channel", remoteErr: relay.ErrNotFound, wantCode: CodeAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "membership changed", remoteErr: relay.ErrMembershipChanged, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "generation changed", remoteErr: relay.ErrGenerationMismatch, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "rollback", remoteErr: relay.ErrRollback, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "retired", remoteErr: relay.ErrRetired, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "expired", remoteErr: relay.ErrExpired, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "invalid response", remoteErr: relay.ErrInvalidArgument, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "unverified response", remoteErr: relay.ErrUnverified, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "closed", remoteErr: relay.ErrClosed, wantCode: CodeUnavailable, wantAction: ActionRetry},
		{name: "unknown", remoteErr: fmt.Errorf("unknown remote failure"), wantCode: CodeUnavailable, wantAction: ActionRetry},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 1)
			fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
				return relay.Page{}, fmt.Errorf("%s: %w", secretMarker, test.remoteErr)
			}

			_, err := fixture.coordinator.downloadRecoverySnapshot(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			)
			assertProblem(t, err, test.wantCode, PhaseAttachDownload, test.wantAction)
			if strings.Contains(err.Error(), secretMarker) || strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
				t.Fatal("download problem exposed its remote cause")
			}
			progress, readErr := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
			if readErr != nil {
				t.Fatalf("CurrentSyncProgress() error = %v", readErr)
			}
			if progress.DownloadedCursor != 0 || progress.AppliedCursor != 0 || progress.RelayHead != 0 {
				t.Fatalf("progress after remote failure = %#v, want untouched", progress)
			}
		})
	}
}

func TestDownloadRecoverySnapshotRetainsForwardAndCrossedEvidenceBeforeRefusingRollback(t *testing.T) {
	for _, test := range []struct {
		name           string
		membership     uint32
		head           int64
		wantMembership uint32
		wantHead       int64
		wantCode       ProblemCode
		wantAction     ProblemAction
	}{
		{
			name: "forward", membership: 1, head: 2,
			wantMembership: 1, wantHead: 2,
			wantCode: CodeConflict, wantAction: ActionRetry,
		},
		{
			name: "crossed", membership: 2, head: 0,
			wantMembership: 2, wantHead: 1,
			wantCode: CodeRemote, wantAction: ActionRestartRecovery,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 1)
			arrival := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
			fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
				return relay.Page{
					RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
					MembershipGeneration: test.membership,
					Head:                 test.head,
					Arrivals:             []relay.Arrival{arrival},
				}, nil
			}

			_, err := fixture.coordinator.downloadRecoverySnapshot(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			)
			assertProblem(t, err, test.wantCode, PhaseAttachDownload, test.wantAction)

			old := recoveryDownloadWatermark(fixture.recovery.ProjectID, fixture.binding)
			retained, watermarkErr := fixture.store.AdvanceSyncRelayWatermark(context.Background(), old)
			if watermarkErr != nil {
				t.Fatalf("read retained download watermark: %v", watermarkErr)
			}
			if retained.MembershipGeneration != test.wantMembership || retained.RelayHead != test.wantHead {
				t.Fatalf("retained frontier = (%d,%d), want (%d,%d)", retained.MembershipGeneration, retained.RelayHead, test.wantMembership, test.wantHead)
			}

			pageCalls := fixture.remote.pageCalls
			_, err = fixture.coordinator.downloadRecoverySnapshot(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			)
			assertProblem(t, err, CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
			if fixture.remote.pageCalls != pageCalls {
				t.Fatal("retained higher frontier allowed a rolled-back page retry")
			}
		})
	}
}

func TestDownloadRecoverySnapshotRejectsForwardPageBehindConcurrentDurableFrontier(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 1)
	arrival := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
	fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
		stronger := recoveryDownloadWatermark(fixture.recovery.ProjectID, fixture.binding)
		stronger.MembershipGeneration = 3
		stronger.RelayHead = 3
		if _, err := fixture.store.AdvanceSyncRelayWatermark(context.Background(), stronger); err != nil {
			t.Fatalf("advance concurrent stronger frontier: %v", err)
		}
		page := exactRecoveryDownloadPage(fixture, []relay.Arrival{arrival})
		page.MembershipGeneration = 2
		page.Head = 2
		return page, nil
	}

	_, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	assertProblem(t, err, CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	retained, watermarkErr := fixture.store.AdvanceSyncRelayWatermark(
		context.Background(), recoveryDownloadWatermark(fixture.recovery.ProjectID, fixture.binding),
	)
	if watermarkErr != nil || retained.MembershipGeneration != 3 || retained.RelayHead != 3 {
		t.Fatalf("retained concurrent frontier = (%#v,%v), want (3,3)", retained, watermarkErr)
	}
}

func TestDownloadRecoverySnapshotResumesAfterBoundedPartialCommit(t *testing.T) {
	head := int64(recoveryDownloadPageLimit + 2)
	fixture := newRecoveryDownloadFixture(t, head)
	arrivals := make([]relay.Arrival, 0, head)
	var previous protocol.Digest
	for sequence := int64(1); sequence <= head; sequence++ {
		arrival := testRecoveryDownloadArrival(t, fixture.prepared, sequence, sequence, previous)
		arrivals = append(arrivals, arrival)
		previous = protocol.Digest(arrival.EnvelopeDigest)
	}
	pruneID := relay.Digest(testArray32(0xb2))
	prunedAt := time.UnixMilli(3_000).UTC()
	arrivals[len(arrivals)-1].Ciphertext = nil
	arrivals[len(arrivals)-1].PruneID = &pruneID
	arrivals[len(arrivals)-1].PrunedAt = &prunedAt
	failSuffix := true
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		if request.After == int64(recoveryDownloadPageLimit) && failSuffix {
			return relay.Page{}, relay.ErrClosed
		}
		start := int(request.After)
		end := start + request.Limit
		if end > len(arrivals) {
			end = len(arrivals)
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.binding.MembershipGeneration,
			Head:                 fixture.binding.InventoryArrivalHead,
			Arrivals:             append([]relay.Arrival(nil), arrivals[start:end]...),
		}, nil
	}

	_, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	assertProblem(t, err, CodeUnavailable, PhaseAttachDownload, ActionRetry)
	progress, err := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress(partial) error = %v", err)
	}
	if progress.DownloadedCursor != int64(recoveryDownloadPageLimit) || progress.RelayHead != head {
		t.Fatalf("partial progress = %#v, want durable bounded prefix", progress)
	}
	prefix, err := fixture.store.PendingSyncFramesAfter(
		context.Background(), fixture.recovery.ProjectID, 0, recoveryDownloadPageLimit,
	)
	if err != nil {
		t.Fatalf("PendingSyncFramesAfter(partial) error = %v", err)
	}
	if len(prefix) != recoveryDownloadPageLimit {
		t.Fatalf("retained partial prefix = %d frames, want %d", len(prefix), recoveryDownloadPageLimit)
	}
	wantPrefix := make([][]byte, len(prefix))
	for index := range prefix {
		if prefix[index].ArrivalSequence != int64(index+1) || len(prefix[index].SealedEnvelope) == 0 {
			t.Fatalf("retained partial frame %d = %#v, want complete ordered sealed prefix", index, prefix[index])
		}
		wantPrefix[index] = append([]byte(nil), prefix[index].SealedEnvelope...)
	}

	failSuffix = false
	progress, err = fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("resume recovery download: %v", err)
	}
	if progress.DownloadedCursor != head || len(fixture.remote.pageRequests) != 3 {
		t.Fatalf("resumed progress/calls = (%d,%d), want (%d,3)", progress.DownloadedCursor, len(fixture.remote.pageRequests), head)
	}
	wantAfter := []int64{0, int64(recoveryDownloadPageLimit), int64(recoveryDownloadPageLimit)}
	wantLimits := []int{recoveryDownloadPageLimit, 2, 2}
	for index, request := range fixture.remote.pageRequests {
		if request.After != wantAfter[index] || request.Limit != wantLimits[index] {
			t.Fatalf("resume request %d cursor/limit = (%d,%d), want (%d,%d)", index, request.After, request.Limit, wantAfter[index], wantLimits[index])
		}
	}
	retained, err := fixture.store.PendingSyncFramesAfter(
		context.Background(), fixture.recovery.ProjectID, 0, recoveryDownloadPageLimit,
	)
	if err != nil {
		t.Fatalf("PendingSyncFramesAfter(resumed) error = %v", err)
	}
	if len(retained) != recoveryDownloadPageLimit {
		t.Fatalf("retained resumed prefix = %d frames, want %d", len(retained), recoveryDownloadPageLimit)
	}
	for index := range retained {
		if retained[index].ArrivalSequence != int64(index+1) ||
			!bytes.Equal(retained[index].SealedEnvelope, wantPrefix[index]) {
			t.Fatalf("resumed download changed retained prefix frame %d", index)
		}
	}
	suffix, err := fixture.store.PendingSyncFramesAfter(
		context.Background(), fixture.recovery.ProjectID, int64(recoveryDownloadPageLimit), 2,
	)
	if err != nil {
		t.Fatalf("PendingSyncFramesAfter(resumed suffix) error = %v", err)
	}
	if len(suffix) != 2 {
		t.Fatalf("retained resumed suffix = %d frames, want 2", len(suffix))
	}
	for index := range suffix {
		arrival := arrivals[recoveryDownloadPageLimit+index]
		want, frameErr := recoveryDownloadFrame(arrival, fixture.prepared.RelayGeneration)
		if frameErr != nil {
			t.Fatalf("canonical resumed suffix frame %d: %v", index, frameErr)
		}
		if suffix[index].ArrivalSequence != want.ArrivalSequence ||
			suffix[index].EnvelopeDigest != want.EnvelopeDigest ||
			!bytes.Equal(suffix[index].SealedEnvelope, want.SealedEnvelope) ||
			!bytes.Equal(suffix[index].PrunedArrival, want.PrunedArrival) {
			t.Fatalf("resumed suffix frame %d = %#v, want exact ordered frame %#v", index, suffix[index], want)
		}
	}
}

func TestDownloadRecoverySnapshotTreatsImmutablePageConflictsAsRemoteEquivocation(t *testing.T) {
	t.Run("duplicate digest", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 2)
		first := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
		second := first
		second.ArrivalSequence = 2
		fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
			return exactRecoveryDownloadPage(fixture, []relay.Arrival{first, second}), nil
		}

		_, err := fixture.coordinator.downloadRecoverySnapshot(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		)
		assertProblem(t, err, CodeRemote, PhaseAttachDownload, ActionRestartRecovery)
		progress, readErr := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
		if readErr != nil || progress.DownloadedCursor != 0 {
			t.Fatalf("duplicate digest progress = (%#v,%v), want untouched", progress, readErr)
		}
	})

	t.Run("concurrent altered replay", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 1)
		winner := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
		altered := cloneRecoveryDownloadArrival(winner)
		altered.Ciphertext[0] ^= 0x01
		refreshRecoveryDownloadEnvelopeDigest(t, &altered)
		fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
			winnerFrame, frameErr := recoveryDownloadFrame(winner, fixture.prepared.RelayGeneration)
			if frameErr != nil {
				t.Fatalf("prepare concurrent winner: %v", frameErr)
			}
			if _, stageErr := fixture.store.StageSyncPageUnderAuthority(
				context.Background(), fixture.recovery.ProjectID, fixture.binding, 0, 1,
				[]continuitysqlite.OpaqueSyncFrame{winnerFrame},
			); stageErr != nil {
				t.Fatalf("stage concurrent winner: %v", stageErr)
			}
			return exactRecoveryDownloadPage(fixture, []relay.Arrival{altered}), nil
		}

		_, err := fixture.coordinator.downloadRecoverySnapshot(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		)
		assertProblem(t, err, CodeRemote, PhaseAttachDownload, ActionRestartRecovery)
		progress, readErr := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
		if readErr != nil || progress.DownloadedCursor != 1 {
			t.Fatalf("concurrent winner progress = (%#v,%v), want retained winner", progress, readErr)
		}
	})
}

func TestDownloadRecoverySnapshotComparesConcurrentAppliedPrunedArrivalReceipt(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutate     func(*relay.Arrival)
		wantRemote bool
	}{
		{name: "exact replay"},
		{name: "altered prune id", wantRemote: true, mutate: func(arrival *relay.Arrival) {
			(*arrival.PruneID)[0] ^= 0xff
		}},
		{name: "altered reference fact", wantRemote: true, mutate: func(arrival *relay.Arrival) {
			arrival.FactID = relay.FactID("fact-download-2-altered")
		}},
		{name: "altered reference chain", wantRemote: true, mutate: func(arrival *relay.Arrival) {
			arrival.PreviousEnvelopeDigest[0] ^= 0xff
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 2)
			first := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
			second := testRecoveryDownloadArrival(t, fixture.prepared, 2, 2, protocol.Digest(first.EnvelopeDigest))
			pruneID := relay.Digest(testArray32(0xb3))
			prunedAt := time.UnixMilli(4_000).UTC()
			second.Ciphertext = nil
			second.PruneID = &pruneID
			second.PrunedAt = &prunedAt
			winner := []relay.Arrival{first, second}
			loser := []relay.Arrival{cloneRecoveryDownloadArrival(first), cloneRecoveryDownloadArrival(second)}
			if test.mutate != nil {
				test.mutate(&loser[1])
			}

			fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
				promoteConcurrentRecoveryDownloadPage(t, fixture, winner)
				return exactRecoveryDownloadPage(fixture, loser), nil
			}
			progress, err := fixture.coordinator.downloadRecoverySnapshot(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
			)
			if test.wantRemote {
				assertProblem(t, err, CodeRemote, PhaseAttachDownload, ActionRestartRecovery)
				return
			}
			if err != nil || progress.DownloadedCursor != 2 || progress.AppliedCursor != 2 {
				t.Fatalf("exact concurrent applied prune = (%#v, %v), want idempotent applied progress", progress, err)
			}
		})
	}
}

func TestDownloadRecoverySnapshotBoundsPagesAndHonorsCancellationAfterRemoteSuccess(t *testing.T) {
	t.Run("over requested limit", func(t *testing.T) {
		head := int64(recoveryDownloadPageLimit + 1)
		fixture := newRecoveryDownloadFixture(t, head)
		arrivals := make([]relay.Arrival, 0, head)
		var previous protocol.Digest
		for sequence := int64(1); sequence <= head; sequence++ {
			arrival := testRecoveryDownloadArrival(t, fixture.prepared, sequence, sequence, previous)
			arrivals = append(arrivals, arrival)
			previous = protocol.Digest(arrival.EnvelopeDigest)
		}
		fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
			return exactRecoveryDownloadPage(fixture, arrivals), nil
		}

		_, err := fixture.coordinator.downloadRecoverySnapshot(
			context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		)
		assertProblem(t, err, CodeRemote, PhaseAttachDownload, ActionRestartRecovery)
	})

	t.Run("canceled successful response", func(t *testing.T) {
		fixture := newRecoveryDownloadFixture(t, 1)
		arrival := testRecoveryDownloadArrival(t, fixture.prepared, 1, 1, protocol.Digest{})
		ctx, cancel := context.WithCancel(context.Background())
		fixture.remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
			cancel()
			return exactRecoveryDownloadPage(fixture, []relay.Arrival{arrival}), nil
		}

		_, err := fixture.coordinator.downloadRecoverySnapshot(
			ctx, fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled successful Page error = %v, want context.Canceled", err)
		}
		progress, readErr := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
		if readErr != nil || progress.DownloadedCursor != 0 {
			t.Fatalf("canceled Page progress = (%#v,%v), want untouched", progress, readErr)
		}
	})
}

func TestDownloadRecoverySnapshotValidatesLocalBindingsBeforePaging(t *testing.T) {
	for _, test := range []struct {
		name       string
		mutate     func(*continuity.ProjectID, *credential.TrustedProjectCredential, *continuitysqlite.SyncAuthorityBinding)
		wantCode   ProblemCode
		wantPhase  ProblemPhase
		wantAction ProblemAction
	}{
		{
			name: "invalid project",
			mutate: func(projectID *continuity.ProjectID, _ *credential.TrustedProjectCredential, _ *continuitysqlite.SyncAuthorityBinding) {
				*projectID = " invalid"
			},
			wantCode: CodeInvalid, wantPhase: PhaseRecoveryValidation, wantAction: ActionCorrectInput,
		},
		{
			name: "credential project mismatch",
			mutate: func(projectID *continuity.ProjectID, _ *credential.TrustedProjectCredential, _ *continuitysqlite.SyncAuthorityBinding) {
				*projectID = testProjectID(77)
			},
			wantCode: CodeInvalid, wantPhase: PhaseRecoveryValidation, wantAction: ActionRestartRecovery,
		},
		{
			name: "invalid prepared credential",
			mutate: func(_ *continuity.ProjectID, prepared *credential.TrustedProjectCredential, _ *continuitysqlite.SyncAuthorityBinding) {
				prepared.WriteGeneration = 0
			},
			wantCode: CodeInvalid, wantPhase: PhaseRecoveryValidation, wantAction: ActionCorrectInput,
		},
		{
			name: "authority channel mismatch",
			mutate: func(_ *continuity.ProjectID, _ *credential.TrustedProjectCredential, binding *continuitysqlite.SyncAuthorityBinding) {
				binding.ChannelID[0] ^= 0xff
			},
			wantCode: CodeConflict, wantPhase: PhaseAttachDownload, wantAction: ActionRestartRecovery,
		},
		{
			name: "authority precedes registration",
			mutate: func(_ *continuity.ProjectID, _ *credential.TrustedProjectCredential, binding *continuitysqlite.SyncAuthorityBinding) {
				binding.MembershipGeneration = 0
			},
			wantCode: CodeConflict, wantPhase: PhaseAttachDownload, wantAction: ActionRestartRecovery,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRecoveryDownloadFixture(t, 1)
			projectID := fixture.recovery.ProjectID
			prepared := fixture.prepared
			binding := fixture.binding
			test.mutate(&projectID, &prepared, &binding)

			_, err := fixture.coordinator.downloadRecoverySnapshot(context.Background(), projectID, prepared, binding)
			assertProblem(t, err, test.wantCode, test.wantPhase, test.wantAction)
			if fixture.remote.pageCalls != 0 {
				t.Fatalf("invalid local binding made %d Page calls", fixture.remote.pageCalls)
			}
		})
	}
}

func TestRecoveryDownloadStoreErrorsMapByConflictFieldWithoutLeakingDetails(t *testing.T) {
	const secretMarker = "store-secret-marker"
	for _, test := range []struct {
		name       string
		err        error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "concurrent cursor", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorCursor, Field: "expected_after", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "membership floor", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorCursor, Field: "membership_generation", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "arrival floor", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorCursor, Field: "inventory_arrival_head", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "altered frame", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "frame_bytes", Detail: secretMarker}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "duplicate digest", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "envelope_digest", Detail: secretMarker}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "authority race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "candidate race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority_candidate", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "unverifiable applied prune", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "applied_pruned_unverifiable", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "channel corruption", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "channel_id", Detail: secretMarker}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "other conflict", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "other", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "field store", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Field: "sync_authority", Detail: secretMarker}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "transient store", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Detail: secretMarker}, wantCode: CodeUnavailable, wantAction: ActionRetry},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := mapRecoveryDownloadStoreError(context.Background(), test.err)
			assertProblem(t, err, test.wantCode, PhaseAttachDownload, test.wantAction)
			if strings.Contains(err.Error(), secretMarker) || strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
				t.Fatal("mapped store problem exposed its detail")
			}
		})
	}
}

type recoveryDownloadFixture struct {
	store       *continuitysqlite.Store
	coordinator *Coordinator
	remote      *remoteFixture
	recovery    credential.ProjectRecoveryCredential
	prepared    credential.TrustedProjectCredential
	binding     continuitysqlite.SyncAuthorityBinding
}

func newRecoveryDownloadFixture(t *testing.T, head int64) recoveryDownloadFixture {
	t.Helper()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 1, []uint32{recovery.WriteGeneration})
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	registration, err := mustCoordinator(t, store, seedRemote).bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind prepared download credential: %v", err)
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 1, ArrivalHead: head}
	remote := exactRecoveryRegistrationInventoryRemote(
		recovery, registration, snapshot, []relay.EnvironmentInventoryRecord{recoveryRegistrationInventoryRecord(registration)},
	)
	coordinator := mustCoordinator(t, store, remote)
	binding, err := coordinator.convergeRegisteredRecoveryAuthority(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("converge recovery download authority: %v", err)
	}
	return recoveryDownloadFixture{
		store: store, coordinator: coordinator, remote: remote,
		recovery: recovery, prepared: prepared, binding: binding,
	}
}

func testRecoveryDownloadArrival(
	t *testing.T,
	prepared credential.TrustedProjectCredential,
	arrivalSequence,
	environmentSequence int64,
	previous protocol.Digest,
) relay.Arrival {
	t.Helper()
	sealed := protocol.SealedFact{
		Header: protocol.FactHeader{
			ProtocolVersion:        protocol.ProtocolVersionV1,
			CipherSuite:            protocol.CipherSuiteXChaCha20Poly1305,
			ChannelID:              prepared.ChannelID,
			FactID:                 continuity.FactID(fmt.Sprintf("fact-download-%d", arrivalSequence)),
			EnvironmentID:          prepared.Certificate.EnvironmentID,
			EnvironmentSequence:    environmentSequence,
			KeyGeneration:          prepared.WriteGeneration,
			PreviousEnvelopeDigest: previous,
			CertificateID:          protocol.CertificateID(prepared.Certificate),
			Nonce:                  protocol.Nonce(testArray24(byte(0x70 + arrivalSequence))),
		},
		Ciphertext: testBytes(byte(0x80+arrivalSequence), relay.MinimumCiphertextBytes),
		Signature:  protocol.Signature(testArray64(byte(0x90 + arrivalSequence))),
	}
	wire, err := sealed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal test download envelope: %v", err)
	}
	if len(wire) == 0 {
		t.Fatal("test download envelope is empty")
	}
	digest := protocol.EnvelopeDigest(sealed)
	return relay.Arrival{
		Envelope: relay.Envelope{
			ProtocolVersion:        sealed.Header.ProtocolVersion,
			CipherSuite:            sealed.Header.CipherSuite,
			ChannelID:              relay.ChannelID(sealed.Header.ChannelID),
			FactID:                 relay.FactID(sealed.Header.FactID),
			EnvironmentID:          relay.EnvironmentID(sealed.Header.EnvironmentID),
			EnvironmentSequence:    sealed.Header.EnvironmentSequence,
			KeyGeneration:          sealed.Header.KeyGeneration,
			PreviousEnvelopeDigest: relay.Digest(sealed.Header.PreviousEnvelopeDigest),
			CertificateID:          relay.Digest(sealed.Header.CertificateID),
			Nonce:                  relay.Nonce(sealed.Header.Nonce),
			Ciphertext:             append([]byte(nil), sealed.Ciphertext...),
			Signature:              relay.Signature(sealed.Signature),
			EnvelopeDigest:         relay.Digest(digest),
		},
		ArrivalSequence: arrivalSequence,
		CiphertextSize:  int64(len(sealed.Ciphertext)),
		ArrivedAt:       time.UnixMilli(1_000 + arrivalSequence).UTC(),
	}
}

func exactRecoveryDownloadPage(fixture recoveryDownloadFixture, arrivals []relay.Arrival) relay.Page {
	return relay.Page{
		RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
		MembershipGeneration: fixture.binding.MembershipGeneration,
		Head:                 fixture.binding.InventoryArrivalHead,
		Arrivals:             append([]relay.Arrival(nil), arrivals...),
	}
}

func recoveryDownloadWatermark(
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
) continuitysqlite.SyncRelayWatermark {
	return continuitysqlite.SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            binding.ChannelID,
		RelayGeneration:      binding.RelayGeneration,
		AdminPublicKey:       binding.AdminPublicKey,
		MembershipGeneration: binding.MembershipGeneration,
		RelayHead:            binding.InventoryArrivalHead,
	}
}

func promoteConcurrentRecoveryDownloadPage(t *testing.T, fixture recoveryDownloadFixture, arrivals []relay.Arrival) {
	t.Helper()
	if len(arrivals) != 2 {
		t.Fatalf("concurrent promotion arrivals = %d, want 2", len(arrivals))
	}
	opaque := make([]continuitysqlite.OpaqueSyncFrame, len(arrivals))
	for index, arrival := range arrivals {
		frame, err := recoveryDownloadFrame(arrival, fixture.prepared.RelayGeneration)
		if err != nil {
			t.Fatalf("prepare concurrent recovery frame %d: %v", index, err)
		}
		opaque[index] = frame
	}
	root := continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              continuity.FactID(arrivals[0].FactID),
		ProjectID:           fixture.recovery.ProjectID,
		SubjectKind:         continuity.RecordProjectIdentity,
		SubjectID:           continuity.SubjectID(fixture.recovery.ProjectID),
		FactKind:            continuity.FactProjectRegistered,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(`{"observation":{"observed_at_millis":1,"harness_session_id":"conversation-1","branch":"issue/loaf-93","worktree":"/workspace/loaf"},"label":"Loaf"}`),
		EnvironmentID:       continuity.EnvironmentID(arrivals[0].EnvironmentID),
		EnvironmentSequence: arrivals[0].EnvironmentSequence,
		HLCWallMillis:       100,
		EnvelopeVersion:     1,
	}
	sealed := continuitysqlite.VerifiedSyncFrame{
		ArrivalSequence:        arrivals[0].ArrivalSequence,
		PreviousEnvelopeDigest: [32]byte(arrivals[0].PreviousEnvelopeDigest),
		EnvelopeDigest:         [32]byte(arrivals[0].EnvelopeDigest),
		CertificateID:          [32]byte(arrivals[0].CertificateID),
		KeyGeneration:          arrivals[0].KeyGeneration,
		Nonce:                  [24]byte(arrivals[0].Nonce),
		Fact:                   root,
	}
	if _, err := fixture.store.StageSyncPageUnderAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, 0, 2,
		[]continuitysqlite.OpaqueSyncFrame{opaque[0]},
	); err != nil {
		t.Fatalf("stage concurrent recovery root: %v", err)
	}
	if _, err := fixture.store.ApplySyncBatch(
		context.Background(), fixture.recovery.ProjectID, fixture.binding,
		[]continuitysqlite.VerifiedSyncFrame{sealed}, 1_000, 100,
	); err != nil {
		t.Fatalf("apply concurrent recovery root: %v", err)
	}
	if _, err := fixture.store.StageSyncPageUnderAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, 1, 2,
		[]continuitysqlite.OpaqueSyncFrame{opaque[1]},
	); err != nil {
		t.Fatalf("stage concurrent recovery prune: %v", err)
	}
	prunedArrival, err := protocol.ParsePrunedArrival(opaque[1].PrunedArrival)
	if err != nil {
		t.Fatalf("parse concurrent pruned arrival: %v", err)
	}
	pruned := continuitysqlite.VerifiedTerminalPrunedFrame{
		Reference: continuitysqlite.VerifiedPruneReference{
			FactID:                 prunedArrival.Reference.FactID,
			EnvironmentID:          prunedArrival.Reference.EnvironmentID,
			EnvironmentSequence:    prunedArrival.Reference.EnvironmentSequence,
			ArrivalSequence:        prunedArrival.Reference.ArrivalSequence,
			EnvelopeDigest:         [32]byte(prunedArrival.Reference.EnvelopeDigest),
			CertificateID:          [32]byte(prunedArrival.Reference.CertificateID),
			PreviousEnvelopeDigest: [32]byte(prunedArrival.Reference.PreviousEnvelopeDigest),
			KeyGeneration:          prunedArrival.Reference.KeyGeneration,
			Nonce:                  [24]byte(prunedArrival.Reference.Nonce),
		},
		PruneCertificateID: [32]byte(prunedArrival.PruneID),
		FactKind:           continuity.FactScratchpadMessageRecorded,
		HLC:                continuity.HybridTime{WallMillis: 101},
	}
	candidate, err := fixture.store.StageVerifiedTerminalCandidateChunk(
		context.Background(), fixture.recovery.ProjectID, fixture.binding,
		[]continuitysqlite.VerifiedTerminalCandidateFrame{
			{Inbox: opaque[1], Pruned: &pruned},
		},
		1_000, 100,
	)
	if err != nil {
		t.Fatalf("stage concurrent terminal candidate: %v", err)
	}
	if _, err := fixture.store.PromoteTerminalCandidate(
		context.Background(), fixture.recovery.ProjectID,
		continuitysqlite.TerminalCandidateCheckpoint{
			CandidateID:            candidate.CandidateID,
			ThroughArrivalSequence: candidate.ThroughArrivalSequence,
			FrameCount:             candidate.FrameCount,
			RollingCandidateDigest: candidate.RollingCandidateDigest,
		},
	); err != nil {
		t.Fatalf("promote concurrent terminal candidate: %v", err)
	}
}

func cloneRecoveryDownloadArrival(arrival relay.Arrival) relay.Arrival {
	cloned := arrival
	cloned.Ciphertext = append([]byte(nil), arrival.Ciphertext...)
	if arrival.PruneID != nil {
		pruneID := *arrival.PruneID
		cloned.PruneID = &pruneID
	}
	if arrival.PrunedAt != nil {
		prunedAt := *arrival.PrunedAt
		cloned.PrunedAt = &prunedAt
	}
	return cloned
}

func refreshRecoveryDownloadEnvelopeDigest(t *testing.T, arrival *relay.Arrival) {
	t.Helper()
	sealed := protocol.SealedFact{
		Header: protocol.FactHeader{
			ProtocolVersion:        arrival.ProtocolVersion,
			CipherSuite:            arrival.CipherSuite,
			ChannelID:              protocol.ChannelID(arrival.ChannelID),
			FactID:                 continuity.FactID(arrival.FactID),
			EnvironmentID:          continuity.EnvironmentID(arrival.EnvironmentID),
			EnvironmentSequence:    arrival.EnvironmentSequence,
			KeyGeneration:          arrival.KeyGeneration,
			PreviousEnvelopeDigest: protocol.Digest(arrival.PreviousEnvelopeDigest),
			CertificateID:          protocol.Digest(arrival.CertificateID),
			Nonce:                  protocol.Nonce(arrival.Nonce),
		},
		Ciphertext: arrival.Ciphertext,
		Signature:  protocol.Signature(arrival.Signature),
	}
	if _, err := sealed.MarshalBinary(); err != nil {
		t.Fatalf("marshal refreshed test arrival: %v", err)
	}
	arrival.EnvelopeDigest = relay.Digest(protocol.EnvelopeDigest(sealed))
}

func testArray64(value byte) [64]byte {
	var result [64]byte
	for index := range result {
		result[index] = value
	}
	return result
}

func testArray24(value byte) [24]byte {
	var result [24]byte
	for index := range result {
		result[index] = value
	}
	return result
}
