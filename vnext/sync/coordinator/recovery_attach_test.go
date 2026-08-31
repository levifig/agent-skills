package coordinator

import (
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
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestAttachPreparedRecoveryPromotesAndActivatesAllSealedSnapshot(t *testing.T) {
	fixture := newRecoveryAttachFixture(t, 1)
	arrival, fact := signedRecoveryAttachArrival(t, fixture.recovery, fixture.prepared)
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		if request.After != 0 || request.Limit != 1 {
			t.Fatalf("recovery attach page request = after %d limit %d", request.After, request.Limit)
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
			Head:                 1,
			Arrivals:             []relay.Arrival{arrival},
		}, nil
	}
	fixture.remote.prune = func(_ context.Context, request relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
				ArrivalHead:          1,
			},
		}, nil
	}

	progress, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	if err != nil {
		t.Fatalf("attach prepared recovery: %v", err)
	}
	if progress.ProjectID != fixture.recovery.ProjectID || progress.ChannelID != continuitysqlite.SyncChannelID(fixture.prepared.ChannelID) ||
		progress.ActivationState != continuitysqlite.SyncActivationAttached ||
		progress.DownloadedCursor != 1 || progress.AppliedCursor != 1 || progress.RelayHead != 1 {
		t.Fatalf("attached recovery progress = %#v", progress)
	}
	retained, err := fixture.store.ExportFact(context.Background(), fact.FactID)
	if err != nil || retained.FactID != fact.FactID {
		t.Fatalf("retained recovery root = (%#v, %v)", retained, err)
	}
	if _, found, err := fixture.store.CurrentTerminalCandidate(
		context.Background(), fixture.recovery.ProjectID,
	); err != nil || found {
		t.Fatalf("terminal candidate after attach = (_, %t, %v), want absent", found, err)
	}
	if _, found, err := fixture.store.CurrentSyncRecoveryPruneCandidate(
		context.Background(), fixture.recovery.ProjectID,
	); err != nil || found {
		t.Fatalf("recovery prune candidate after attach = (_, %t, %v), want absent", found, err)
	}

	replayed, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	if err != nil || replayed != progress {
		t.Fatalf("exact recovery attach replay = (%#v, %v), want (%#v, nil)", replayed, err, progress)
	}
}

func TestAttachPreparedRecoveryActivatesAlreadyPromotedSnapshot(t *testing.T) {
	fixture := newRecoveryAttachFixture(t, 1)
	arrival, fact := signedRecoveryAttachArrival(t, fixture.recovery, fixture.prepared)
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		if request.After != 0 || request.Limit != 1 {
			t.Fatalf("recovery attach page request = after %d limit %d", request.After, request.Limit)
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
			Head:                 1,
			Arrivals:             []relay.Arrival{arrival},
		}, nil
	}
	fixture.remote.prune = func(_ context.Context, _ relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
				ArrivalHead:          1,
			},
		}, nil
	}
	registration, err := fixture.coordinator.bindPreparedRecoveryRegistration(
		fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
	)
	if err != nil {
		t.Fatalf("bind promoted recovery attach: %v", err)
	}
	if _, err := fixture.coordinator.registerPreparedRecoveryEnvironment(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, registration,
	); err != nil {
		t.Fatalf("register promoted recovery attach: %v", err)
	}
	binding, err := fixture.coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, registration,
	)
	if err != nil {
		t.Fatalf("converge promoted recovery attach: %v", err)
	}
	if _, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding,
	); err != nil {
		t.Fatalf("download promoted recovery attach: %v", err)
	}
	recoveryPrunes, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding,
	)
	if err != nil {
		t.Fatalf("persist promoted recovery prune inventory: %v", err)
	}
	frames, err := fixture.store.PendingSyncFramesAfter(
		context.Background(), fixture.recovery.ProjectID, 0, 1,
	)
	if err != nil || len(frames) != 1 {
		t.Fatalf("read promoted recovery frame = (%d, %v)", len(frames), err)
	}
	sealed, err := fixture.coordinator.recoveryTerminalSealedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding, frames[0],
	)
	if err != nil {
		t.Fatalf("verify promoted recovery frame: %v", err)
	}
	candidate, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), fixture.recovery.ProjectID, binding, recoveryPrunes,
		[]continuitysqlite.VerifiedTerminalCandidateFrame{{Inbox: frames[0], Sealed: &sealed}}, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("stage promoted recovery candidate: %v", err)
	}
	if _, err := fixture.store.PromoteTerminalCandidate(
		context.Background(), fixture.recovery.ProjectID, candidate.Checkpoint(),
	); err != nil {
		t.Fatalf("promote recovery before interrupted activation: %v", err)
	}
	before, err := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
	if err != nil || before.ActivationState != continuitysqlite.SyncActivationStaging || before.AppliedCursor != 1 {
		t.Fatalf("promoted recovery progress before resume = (%#v, %v)", before, err)
	}
	concurrent, done, err := fixture.coordinator.activateConcurrentRecoveryPromotion(
		context.Background(), fixture.recovery.ProjectID, binding,
	)
	if err != nil || !done || concurrent.ActivationState != continuitysqlite.SyncActivationAttached || concurrent.AppliedCursor != 1 {
		t.Fatalf("concurrent recovery promotion reclassification = (%#v, %t, %v)", concurrent, done, err)
	}

	progress, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	if err != nil {
		t.Fatalf("activate already-promoted recovery attach: %v", err)
	}
	if progress.ActivationState != continuitysqlite.SyncActivationAttached || progress.AppliedCursor != 1 {
		t.Fatalf("already-promoted recovery progress = %#v", progress)
	}
	if retained, err := fixture.store.ExportFact(context.Background(), fact.FactID); err != nil || retained.FactID != fact.FactID {
		t.Fatalf("already-promoted recovery root = (%#v, %v)", retained, err)
	}
}

func TestAttachPreparedRecoveryRejectsInvalidClockBeforeMutation(t *testing.T) {
	fixture := newRecoveryAttachFixture(t, 1)
	before := *fixture.remote
	_, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: -1},
	)
	assertProblem(t, err, CodeInvalid, PhaseAttachActivation, ActionCorrectInput)
	if fixture.remote.createCalls != before.createCalls || fixture.remote.classifyCalls != before.classifyCalls ||
		fixture.remote.registerCalls != before.registerCalls || fixture.remote.pageCalls != before.pageCalls ||
		fixture.remote.pruneCalls != before.pruneCalls || len(fixture.remote.environmentRequests) != len(before.environmentRequests) {
		t.Fatalf("invalid clock reached a recovery remote")
	}
	if _, found, stateErr := fixture.store.CurrentSyncAuthorityCandidate(
		context.Background(), fixture.recovery.ProjectID,
	); stateErr != nil || found {
		t.Fatalf("invalid clock retained authority candidate = (_, %t, %v)", found, stateErr)
	}
}

func TestAttachPreparedRecoveryResumesActiveTerminalCandidate(t *testing.T) {
	fixture := newRecoveryAttachFixture(t, 2)
	rootArrival, _ := signedRecoveryAttachArrival(t, fixture.recovery, fixture.prepared)
	journalArrival := signedRecoveryAttachJournalArrival(
		t, fixture.recovery, fixture.prepared, protocol.Digest(rootArrival.EnvelopeDigest),
	)
	arrivals := []relay.Arrival{rootArrival, journalArrival}
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		if request.After != 0 || request.Limit != 2 {
			t.Fatalf("partial recovery attach page request = after %d limit %d", request.After, request.Limit)
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
			Head:                 2,
			Arrivals:             append([]relay.Arrival(nil), arrivals...),
		}, nil
	}
	fixture.remote.prune = func(_ context.Context, _ relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
				ArrivalHead:          2,
			},
		}, nil
	}
	registration, err := fixture.coordinator.bindPreparedRecoveryRegistration(
		fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
	)
	if err != nil {
		t.Fatalf("bind partial recovery attach: %v", err)
	}
	if _, err := fixture.coordinator.registerPreparedRecoveryEnvironment(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, registration,
	); err != nil {
		t.Fatalf("register partial recovery attach: %v", err)
	}
	binding, err := fixture.coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, registration,
	)
	if err != nil {
		t.Fatalf("converge partial recovery attach: %v", err)
	}
	if _, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding,
	); err != nil {
		t.Fatalf("download partial recovery attach: %v", err)
	}
	recoveryPrunes, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding,
	)
	if err != nil {
		t.Fatalf("persist partial recovery prune inventory: %v", err)
	}
	prefix, err := fixture.store.PendingSyncFramesAfter(
		context.Background(), fixture.recovery.ProjectID, 0, 1,
	)
	if err != nil || len(prefix) != 1 {
		t.Fatalf("read partial recovery prefix = (%d, %v)", len(prefix), err)
	}
	sealed, err := fixture.coordinator.recoveryTerminalSealedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding, prefix[0],
	)
	if err != nil {
		t.Fatalf("verify partial recovery prefix: %v", err)
	}
	candidate, err := fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
		context.Background(), fixture.recovery.ProjectID, binding, recoveryPrunes,
		[]continuitysqlite.VerifiedTerminalCandidateFrame{{Inbox: prefix[0], Sealed: &sealed}}, 1_000, 100,
	)
	if err != nil || candidate.ThroughArrivalSequence != 1 {
		t.Fatalf("stage partial recovery terminal candidate = (%#v, %v)", candidate, err)
	}
	pruneCalls := fixture.remote.pruneCalls
	wrongRecovery := fixture.recovery
	wrongPrepared := fixture.prepared
	wrongRootBytes := fixture.prepared.ProjectRoot.Bytes()
	wrongRootBytes[0] ^= 0xff
	wrongRoot, err := crypto.ProjectRootFromBytes(wrongRootBytes[:])
	if err != nil {
		t.Fatalf("construct wrong recovery attach root: %v", err)
	}
	wrongRecovery.ProjectRoot = wrongRoot
	wrongPrepared.ProjectRoot = wrongRoot
	_, err = fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, wrongRecovery, wrongPrepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	assertProblem(t, err, CodeAuthorization, PhaseAttachActivation, ActionCheckRecoveryAuthority)
	if progress, progressErr := fixture.store.CurrentSyncProgress(
		context.Background(), fixture.recovery.ProjectID,
	); progressErr != nil || progress.AppliedCursor != 0 || progress.ActivationState != continuitysqlite.SyncActivationStaging {
		t.Fatalf("wrong-root resume changed canonical progress = (%#v, %v)", progress, progressErr)
	}

	progress, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	if err != nil {
		t.Fatalf("resume partial recovery attach: %v", err)
	}
	if progress.ActivationState != continuitysqlite.SyncActivationAttached || progress.AppliedCursor != 2 ||
		progress.DownloadedCursor != 2 || progress.RelayHead != 2 {
		t.Fatalf("resumed recovery progress = %#v", progress)
	}
	if fixture.remote.pruneCalls != pruneCalls+2 {
		t.Fatalf("wrong-root and correct resumes did not each revalidate prune inventory: %d -> %d", pruneCalls, fixture.remote.pruneCalls)
	}
}

func TestAttachPreparedRecoveryReverifiesAheadMultiChunkCandidate(t *testing.T) {
	const historyLength = 17
	fixture := newRecoveryAttachFixture(t, historyLength)
	root, _ := signedRecoveryAttachArrival(t, fixture.recovery, fixture.prepared)
	arrivals := make([]relay.Arrival, 0, historyLength)
	arrivals = append(arrivals, root)
	previous := protocol.Digest(root.EnvelopeDigest)
	for sequence := int64(2); sequence <= historyLength; sequence++ {
		arrival := signedRecoveryAttachJournalArrivalAt(
			t, fixture.recovery, fixture.prepared, sequence, previous,
		)
		arrivals = append(arrivals, arrival)
		previous = protocol.Digest(arrival.EnvelopeDigest)
	}
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		start := int(request.After)
		if start < 0 || start >= len(arrivals) || request.Limit < 1 {
			return relay.Page{}, relay.ErrInvalidArgument
		}
		end := start + request.Limit
		if end > len(arrivals) {
			end = len(arrivals)
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
			Head:                 historyLength,
			Arrivals:             append([]relay.Arrival(nil), arrivals[start:end]...),
		}, nil
	}
	fixture.remote.prune = func(_ context.Context, _ relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
				ArrivalHead:          historyLength,
			},
		}, nil
	}
	registration, err := fixture.coordinator.bindPreparedRecoveryRegistration(
		fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
	)
	if err != nil {
		t.Fatalf("bind multi-chunk recovery attach: %v", err)
	}
	if _, err := fixture.coordinator.registerPreparedRecoveryEnvironment(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, registration,
	); err != nil {
		t.Fatalf("register multi-chunk recovery attach: %v", err)
	}
	binding, err := fixture.coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, registration,
	)
	if err != nil {
		t.Fatalf("converge multi-chunk recovery attach: %v", err)
	}
	if _, err := fixture.coordinator.downloadRecoverySnapshot(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding,
	); err != nil {
		t.Fatalf("download multi-chunk recovery attach: %v", err)
	}
	recoveryPrunes, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding,
	)
	if err != nil {
		t.Fatalf("persist multi-chunk recovery prune inventory: %v", err)
	}
	var candidate continuitysqlite.TerminalCandidate
	for after := int64(0); after < historyLength; {
		frames, readErr := fixture.store.PendingSyncFramesAfter(
			context.Background(), fixture.recovery.ProjectID, after, recoveryAttachTerminalChunkFrames,
		)
		if readErr != nil {
			t.Fatalf("read multi-chunk recovery frames: %v", readErr)
		}
		verified := make([]continuitysqlite.VerifiedTerminalCandidateFrame, len(frames))
		for index := range frames {
			sealed, verifyErr := fixture.coordinator.recoveryTerminalSealedFrame(
				context.Background(), fixture.recovery.ProjectID, fixture.prepared, binding, frames[index],
			)
			if verifyErr != nil {
				t.Fatalf("verify multi-chunk recovery frame %d: %v", frames[index].ArrivalSequence, verifyErr)
			}
			verified[index] = continuitysqlite.VerifiedTerminalCandidateFrame{Inbox: frames[index], Sealed: &sealed}
		}
		candidate, err = fixture.store.StageVerifiedRecoveryTerminalCandidateChunk(
			context.Background(), fixture.recovery.ProjectID, binding, recoveryPrunes, verified, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("stage multi-chunk recovery candidate: %v", err)
		}
		after = frames[len(frames)-1].ArrivalSequence
	}
	if candidate.ThroughArrivalSequence != historyLength {
		t.Fatalf("ahead recovery candidate through = %d", candidate.ThroughArrivalSequence)
	}
	pageCalls := fixture.remote.pageCalls

	progress, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	if err != nil {
		t.Fatalf("reverify ahead multi-chunk recovery candidate: %v", err)
	}
	if progress.ActivationState != continuitysqlite.SyncActivationAttached || progress.AppliedCursor != historyLength {
		t.Fatalf("ahead multi-chunk recovery progress = %#v", progress)
	}
	if fixture.remote.pageCalls != pageCalls {
		t.Fatalf("ahead candidate resume refetched retained arrivals: %d -> %d", pageCalls, fixture.remote.pageCalls)
	}
}

func TestAttachPreparedRecoveryRejectsHostileEnvelopeWithoutCanonicalMutation(t *testing.T) {
	const historyLength = 17
	fixture := newRecoveryAttachFixture(t, historyLength)
	root, fact := signedRecoveryAttachArrival(t, fixture.recovery, fixture.prepared)
	arrivals := make([]relay.Arrival, 0, historyLength)
	arrivals = append(arrivals, root)
	previous := protocol.Digest(root.EnvelopeDigest)
	for sequence := int64(2); sequence <= historyLength; sequence++ {
		arrival := signedRecoveryAttachJournalArrivalAt(
			t, fixture.recovery, fixture.prepared, sequence, previous,
		)
		arrivals = append(arrivals, arrival)
		previous = protocol.Digest(arrival.EnvelopeDigest)
	}
	arrivals[historyLength-1].Signature[0] ^= 0xff
	refreshRecoveryDownloadEnvelopeDigest(t, &arrivals[historyLength-1])
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		start := int(request.After)
		if start < 0 || start >= len(arrivals) || request.Limit < 1 {
			return relay.Page{}, relay.ErrInvalidArgument
		}
		end := start + request.Limit
		if end > len(arrivals) {
			end = len(arrivals)
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
			Head:                 historyLength,
			Arrivals:             append([]relay.Arrival(nil), arrivals[start:end]...),
		}, nil
	}
	fixture.remote.prune = func(_ context.Context, _ relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
				ArrivalHead:          historyLength,
			},
		}, nil
	}

	_, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	assertProblem(t, err, CodeRemote, PhaseAttachActivation, ActionRestartRecovery)
	if _, err := fixture.store.ExportFact(context.Background(), fact.FactID); err == nil {
		t.Fatal("hostile recovery envelope became canonical")
	}
	progress, err := fixture.store.CurrentSyncProgress(context.Background(), fixture.recovery.ProjectID)
	if err != nil {
		t.Fatalf("read hostile recovery progress: %v", err)
	}
	if progress.ActivationState != continuitysqlite.SyncActivationStaging || progress.AppliedCursor != 0 ||
		progress.DownloadedCursor != historyLength || progress.RelayHead != historyLength {
		t.Fatalf("hostile recovery progress = %#v", progress)
	}
	if candidate, found, candidateErr := fixture.store.CurrentTerminalCandidate(
		context.Background(), fixture.recovery.ProjectID,
	); candidateErr != nil || !found || candidate.ThroughArrivalSequence != 16 {
		t.Fatalf("hostile recovery retained verified prefix = (%#v, %t, %v)", candidate, found, candidateErr)
	}
	if _, found, err := fixture.store.CurrentSyncRecoveryPruneCandidate(
		context.Background(), fixture.recovery.ProjectID,
	); err != nil || !found {
		t.Fatalf("hostile recovery lost retained prune checkpoint = (_, %t, %v)", found, err)
	}
}

func TestAttachPreparedRecoveryAuthenticatesMixedSealedAndPrunedHistory(t *testing.T) {
	downloadFixture := newRecoveryDownloadFixture(t, 3)
	fixture := recoveryAttachFixture{
		store: downloadFixture.store, coordinator: downloadFixture.coordinator, remote: downloadFixture.remote,
		recovery: downloadFixture.recovery, prepared: downloadFixture.prepared,
	}
	root, rootFact := signedRecoveryAttachArrival(t, fixture.recovery, fixture.prepared)
	pruneID := protocol.Digest(testArray32(0xd1))
	target := signedRecoveryAttachPrunedTarget(
		t, fixture.recovery, fixture.prepared, protocol.Digest(root.EnvelopeDigest),
	)
	closure := signedRecoveryAttachJournalArrivalAt(
		t, fixture.recovery, fixture.prepared, 3, protocol.Digest(target.EnvelopeDigest),
	)
	record := signedRecoveryAttachPruneRecord(t, fixture, pruneID, target, closure)
	target.Ciphertext = nil
	target.PruneID = (*relay.Digest)(&pruneID)
	prunedAt := time.UnixMilli(2_000).UTC()
	target.PrunedAt = &prunedAt
	arrivals := []relay.Arrival{root, target, closure}
	fixture.remote.page = func(_ context.Context, request relay.PageRequest) (relay.Page, error) {
		if request.After != 0 || request.Limit != 3 {
			return relay.Page{}, relay.ErrInvalidArgument
		}
		return relay.Page{
			RelayGeneration:      relay.RelayGeneration(fixture.prepared.RelayGeneration),
			MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
			Head:                 3,
			Arrivals:             append([]relay.Arrival(nil), arrivals...),
		}, nil
	}
	fixture.remote.prune = func(_ context.Context, request relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		if request.After != 0 || request.Limit != relay.MaxPruneInventoryPage {
			return relay.PruneInventoryPage{}, relay.ErrInvalidArgument
		}
		return relay.PruneInventoryPage{
			Channel: relay.ChannelAuthority{
				ChannelID:       relay.ChannelID(fixture.prepared.ChannelID),
				RelayGeneration: relay.RelayGeneration(fixture.prepared.RelayGeneration),
				AdminPublicKey:  relay.PublicKey(fixture.prepared.AdminPublicKey),
			},
			Snapshot: relay.PruneInventorySnapshot{
				MembershipGeneration: fixture.prepared.Certificate.MembershipGeneration,
				ArrivalHead:          3,
				PruneHead:            1,
			},
			Prunes: []relay.PruneInventoryRecord{record},
		}, nil
	}

	progress, err := fixture.coordinator.AttachPreparedRecovery(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.prepared,
		RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
	)
	if err != nil {
		t.Fatalf("attach mixed sealed/pruned recovery history: %v", err)
	}
	if progress.ActivationState != continuitysqlite.SyncActivationAttached || progress.AppliedCursor != 3 {
		t.Fatalf("mixed sealed/pruned recovery progress = %#v", progress)
	}
	if _, err := fixture.store.ExportFact(context.Background(), rootFact.FactID); err != nil {
		t.Fatalf("mixed recovery root is not canonical: %v", err)
	}
	if _, err := fixture.store.ExportFact(context.Background(), continuity.FactID(target.FactID)); err == nil {
		t.Fatal("pruned recovery target was resurrected as a canonical fact")
	}
}

func TestAttachPreparedRecoveryResumesAuthorityConvergenceStates(t *testing.T) {
	for _, test := range []struct {
		name           string
		seedTransition bool
	}{
		{name: "ready predecessor guard"},
		{name: "active recovery successor", seedTransition: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newLaterRecoveryConvergenceFixture(t)
			if test.seedTransition {
				seedRecoveryAuthoritySuccessor(t, fixture, 1)
			}
			prepared := testPreparedRecoveryCredential(
				t, fixture.recovery, fixture.writerID, 6, []uint32{fixture.recovery.WriteGeneration},
			)
			remote := exactRecoveryRegistrationInventoryRemote(
				fixture.recovery, fixture.registration, fixture.snapshot, fixture.records,
			)
			remote.page = func(context.Context, relay.PageRequest) (relay.Page, error) {
				return relay.Page{}, errors.New("stop after authority convergence")
			}
			coordinator := mustCoordinator(t, fixture.store, remote)

			_, err := coordinator.AttachPreparedRecovery(
				context.Background(), fixture.recovery.ProjectID, fixture.recovery, prepared,
				RecoveryAttachOptions{TrustedNowMillis: 1_000, MaximumFutureSkewMillis: 100},
			)
			assertProblem(t, err, CodeUnavailable, PhaseAttachDownload, ActionRetry)
			if _, found, stateErr := fixture.store.CurrentSyncAuthorityRecoverySuccessor(
				context.Background(), fixture.recovery.ProjectID,
			); stateErr != nil || found {
				t.Fatalf("recovery authority successor after resume = (_, %t, %v), want absent", found, stateErr)
			}
			binding, bindingErr := fixture.store.CurrentSyncAuthorityBinding(
				context.Background(), fixture.recovery.ProjectID,
			)
			if bindingErr != nil || binding.MembershipGeneration != fixture.snapshot.MembershipGeneration ||
				binding.InventoryArrivalHead != fixture.snapshot.ArrivalHead {
				t.Fatalf("promoted recovery authority after resume = (%#v, %v)", binding, bindingErr)
			}
		})
	}
}

func TestRecoveryActivationStoreErrorsSeparateRetryableRacesFromLocalCorruption(t *testing.T) {
	const secretMarker = "activation-store-secret-marker"
	for _, test := range []struct {
		name       string
		err        error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "transient store", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Detail: secretMarker}, wantCode: CodeUnavailable, wantAction: ActionRetry},
		{name: "terminal candidate race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "terminal_candidate", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "authority candidate race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority_candidate", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "prune candidate race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_recovery_prune_candidate", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "progress race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_progress", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "checkpoint race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "checkpoint", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "authority drift", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "authority transition race", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority_recovery_transition", Detail: secretMarker}, wantCode: CodeConflict, wantAction: ActionRetry},
		{name: "local invariant", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorActivation, Field: "applied_cursor", Detail: secretMarker}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "field store", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Field: "sync_progress", Detail: secretMarker}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "unknown", err: errors.New(secretMarker), wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := mapRecoveryActivationStoreError(context.Background(), test.err)
			assertProblem(t, err, test.wantCode, PhaseAttachActivation, test.wantAction)
			if strings.Contains(err.Error(), secretMarker) || strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
				t.Fatal("mapped activation problem exposed its detail")
			}
		})
	}
}

func TestRecoveryAttachStoreErrorsRetryAuthorityTransitionRace(t *testing.T) {
	const secretMarker = "attach-store-secret-marker"
	err := mapRecoveryAttachStoreError(context.Background(), &continuitysqlite.SyncError{
		Code: continuitysqlite.SyncErrorConflict, Field: "sync_authority_recovery_transition", Detail: secretMarker,
	})
	assertProblem(t, err, CodeConflict, PhaseAttachActivation, ActionRetry)
	if strings.Contains(err.Error(), secretMarker) || strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
		t.Fatal("mapped attach problem exposed its detail")
	}
}

func signedRecoveryAttachArrival(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	prepared credential.TrustedProjectCredential,
) (relay.Arrival, continuitywire.Fact) {
	t.Helper()
	fact := continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              "fact-recovery-attach-root",
		ProjectID:           recovery.ProjectID,
		SubjectKind:         continuity.RecordProjectIdentity,
		SubjectID:           continuity.SubjectID(recovery.ProjectID),
		FactKind:            continuity.FactProjectRegistered,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(`{"observation":{"observed_at_millis":1,"harness_session_id":"recovery-attach","branch":"issue/loaf-93","worktree":"/workspace/loaf"},"label":"Loaf"}`),
		EnvironmentID:       prepared.Certificate.EnvironmentID,
		EnvironmentSequence: 1,
		HLCWallMillis:       100,
		EnvelopeVersion:     1,
	}
	key, err := crypto.DeriveGenerationKey(
		prepared.ProjectRoot, recovery.ProjectID, prepared.WriteGeneration,
	)
	if err != nil {
		t.Fatalf("derive recovery attach generation key: %v", err)
	}
	sealed, err := crypto.SealFact(
		fact, key, prepared.Certificate, prepared.AdminPublicKey,
		prepared.EnvironmentSeed, protocol.Digest{}, 1_000,
	)
	if err != nil {
		t.Fatalf("seal recovery attach root: %v", err)
	}
	return recoveryAttachArrivalFromSealed(sealed, 1), fact
}

func signedRecoveryAttachJournalArrival(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	prepared credential.TrustedProjectCredential,
	previous protocol.Digest,
) relay.Arrival {
	return signedRecoveryAttachJournalArrivalAt(t, recovery, prepared, 2, previous)
}

func signedRecoveryAttachJournalArrivalAt(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	prepared credential.TrustedProjectCredential,
	sequence int64,
	previous protocol.Digest,
) relay.Arrival {
	t.Helper()
	fact := continuitywire.Fact{
		WireVersion:    continuitywire.Version1,
		FactID:         continuity.FactID(fmt.Sprintf("fact-recovery-attach-journal-%d", sequence)),
		ProjectID:      recovery.ProjectID,
		SubjectKind:    continuity.RecordJournalEntry,
		SubjectID:      continuity.SubjectID(fmt.Sprintf("journal-recovery-attach-%d", sequence)),
		FactKind:       continuity.FactJournalRecorded,
		PayloadVersion: 1,
		CanonicalPayload: []byte(fmt.Sprintf(
			`{"observation":{"observed_at_millis":%d,"harness_session_id":"recovery-attach","branch":"issue/loaf-93","worktree":"/workspace/loaf"},"content":{"category":"note","scope":"sync","text":"entry-%d"}}`,
			sequence, sequence,
		)),
		EnvironmentID:       prepared.Certificate.EnvironmentID,
		EnvironmentSequence: sequence,
		HLCWallMillis:       99 + sequence,
		EnvelopeVersion:     1,
	}
	key, err := crypto.DeriveGenerationKey(prepared.ProjectRoot, recovery.ProjectID, prepared.WriteGeneration)
	if err != nil {
		t.Fatalf("derive recovery attach journal generation key: %v", err)
	}
	sealed, err := crypto.SealFact(
		fact, key, prepared.Certificate, prepared.AdminPublicKey, prepared.EnvironmentSeed, previous, 1_000,
	)
	if err != nil {
		t.Fatalf("seal recovery attach journal: %v", err)
	}
	return recoveryAttachArrivalFromSealed(sealed, sequence)
}

func signedRecoveryAttachPrunedTarget(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	prepared credential.TrustedProjectCredential,
	previous protocol.Digest,
) relay.Arrival {
	t.Helper()
	fact := continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              "fact-recovery-attach-pruned-target",
		ProjectID:           recovery.ProjectID,
		SubjectKind:         continuity.RecordScratchpad,
		SubjectID:           "scratchpad-recovery-attach",
		FactKind:            continuity.FactScratchpadMessageRecorded,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(`{"observation":{"observed_at_millis":2,"harness_session_id":"recovery-attach","branch":"issue/loaf-93","worktree":"/workspace/loaf"},"participant_id":"participant-recovery-attach","text":"pruned"}`),
		EnvironmentID:       prepared.Certificate.EnvironmentID,
		EnvironmentSequence: 2,
		HLCWallMillis:       101,
		HLCLogical:          1,
		EnvelopeVersion:     1,
	}
	key, err := crypto.DeriveGenerationKey(prepared.ProjectRoot, recovery.ProjectID, prepared.WriteGeneration)
	if err != nil {
		t.Fatalf("derive recovery attach prune target generation key: %v", err)
	}
	sealed, err := crypto.SealFact(
		fact, key, prepared.Certificate, prepared.AdminPublicKey, prepared.EnvironmentSeed, previous, 1_000,
	)
	if err != nil {
		t.Fatalf("seal recovery attach prune target: %v", err)
	}
	return recoveryAttachArrivalFromSealed(sealed, 2)
}

func signedRecoveryAttachPruneRecord(
	t *testing.T,
	fixture recoveryAttachFixture,
	pruneID protocol.Digest,
	target relay.Arrival,
	closure relay.Arrival,
) relay.PruneInventoryRecord {
	t.Helper()
	targetReference := recoveryAttachPruneReference(target)
	closureReference := recoveryAttachPruneReference(closure)
	manifest := protocol.PruneManifest{Targets: []protocol.PruneReference{targetReference}}
	plaintext := protocol.PruneBootstrapPlaintext{
		CapsuleVersion:          protocol.PruneBootstrapCapsuleVersionV1,
		ProtocolVersion:         protocol.ProtocolVersionV1,
		CipherSuite:             protocol.CipherSuiteXChaCha20Poly1305,
		BootstrapPurposeVersion: protocol.PruneBootstrapPurposeVersionV1,
		ProjectID:               fixture.recovery.ProjectID,
		ChannelID:               fixture.prepared.ChannelID,
		RelayGeneration:         fixture.prepared.RelayGeneration,
		PruneID:                 pruneID,
		MembershipGeneration:    fixture.prepared.Certificate.MembershipGeneration,
		BarrierArrivalSequence:  closureReference.ArrivalSequence,
		ClosureReferenceDigest:  protocol.PruneReferenceDigest(closureReference),
		ManifestCount:           1,
		ManifestDigest:          protocol.PruneManifestDigest(manifest),
		ScratchpadSubject:       "scratchpad-recovery-attach",
		EntryCount:              1,
		Entries: []protocol.PruneBootstrapEntry{{
			PruneReferenceDigest: protocol.PruneReferenceDigest(targetReference),
			FactKind:             continuity.FactScratchpadMessageRecorded,
			HLC:                  continuity.HybridTime{WallMillis: 101, Logical: 1},
		}},
	}
	key, err := crypto.DerivePruneBootstrapKey(
		fixture.prepared.ProjectRoot, fixture.recovery.ProjectID, protocol.PruneBootstrapPurposeVersionV1,
	)
	if err != nil {
		t.Fatalf("derive recovery attach prune bootstrap key: %v", err)
	}
	capsule, err := crypto.SealPruneBootstrap(plaintext, key)
	if err != nil {
		t.Fatalf("seal recovery attach prune bootstrap: %v", err)
	}
	progressDigest := protocol.Digest(testArray32(0xd2))
	acknowledgement, err := crypto.SignPruneAcknowledgement(protocol.PruneAcknowledgement{
		Version:                       protocol.ControlVersionV1,
		ProtocolVersion:               protocol.ProtocolVersionV1,
		CipherSuite:                   protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                     fixture.prepared.ChannelID,
		RelayGeneration:               fixture.prepared.RelayGeneration,
		EnvironmentID:                 fixture.prepared.Certificate.EnvironmentID,
		CertificateID:                 protocol.CertificateID(fixture.prepared.Certificate),
		MembershipGeneration:          fixture.prepared.Certificate.MembershipGeneration,
		ProgressAcknowledgementDigest: progressDigest,
		AppliedArrivalSequence:        closureReference.ArrivalSequence,
		ProducerSequence:              closureReference.EnvironmentSequence,
		ProducerEnvelopeDigest:        closureReference.EnvelopeDigest,
		PruneID:                       pruneID,
		BarrierArrivalSequence:        closureReference.ArrivalSequence,
		ClosureReferenceDigest:        protocol.PruneReferenceDigest(closureReference),
		ManifestCount:                 1,
		ManifestDigest:                protocol.PruneManifestDigest(manifest),
		CapsuleDigest:                 protocol.PruneBootstrapDigest(capsule),
	}, fixture.prepared.Certificate, fixture.prepared.AdminPublicKey, fixture.prepared.EnvironmentSeed)
	if err != nil {
		t.Fatalf("sign recovery attach prune acknowledgement: %v", err)
	}
	certificate, err := crypto.SignPruneCertificate(protocol.PruneCertificate{
		Version:                    protocol.ControlVersionV1,
		ProtocolVersion:            protocol.ProtocolVersionV1,
		CipherSuite:                protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                  fixture.prepared.ChannelID,
		RelayGeneration:            fixture.prepared.RelayGeneration,
		PruneID:                    pruneID,
		MembershipGeneration:       fixture.prepared.Certificate.MembershipGeneration,
		BarrierArrivalSequence:     closureReference.ArrivalSequence,
		Closure:                    closureReference,
		ClosureDigest:              protocol.PruneReferenceDigest(closureReference),
		ManifestCount:              1,
		ManifestDigest:             protocol.PruneManifestDigest(manifest),
		Manifest:                   manifest,
		CapsuleDigest:              protocol.PruneBootstrapDigest(capsule),
		Capsule:                    capsule,
		ActiveAcknowledgementCount: 1,
		Acknowledgements:           []protocol.PruneAcknowledgement{acknowledgement},
	}, []protocol.EnvironmentCertificate{fixture.prepared.Certificate}, fixture.recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign recovery attach prune certificate: %v", err)
	}
	certificateBytes, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal recovery attach prune certificate: %v", err)
	}
	return relay.PruneInventoryRecord{
		PruneSequence: 1,
		Certificate: relay.PruneCertificate{
			ChannelID:            relay.ChannelID(certificate.ChannelID),
			PruneID:              relay.Digest(certificate.PruneID),
			MembershipGeneration: certificate.MembershipGeneration,
			Barrier:              certificate.BarrierArrivalSequence,
			Closure:              recoveryPruneRelayTarget(certificate.Closure),
			CertificateID:        relay.Digest(protocol.PruneCertificateID(certificate)),
			CertificateBytes:     certificateBytes,
			Targets:              []relay.PruneTarget{recoveryPruneRelayTarget(targetReference)},
		},
		CreatedAt: time.UnixMilli(2_000).UTC(),
	}
}

func recoveryAttachPruneReference(arrival relay.Arrival) protocol.PruneReference {
	return protocol.PruneReference{
		FactID:                 continuity.FactID(arrival.FactID),
		EnvironmentID:          continuity.EnvironmentID(arrival.EnvironmentID),
		EnvironmentSequence:    arrival.EnvironmentSequence,
		ArrivalSequence:        arrival.ArrivalSequence,
		EnvelopeDigest:         protocol.Digest(arrival.EnvelopeDigest),
		CertificateID:          protocol.Digest(arrival.CertificateID),
		PreviousEnvelopeDigest: protocol.Digest(arrival.PreviousEnvelopeDigest),
		KeyGeneration:          arrival.KeyGeneration,
		Nonce:                  protocol.Nonce(arrival.Nonce),
	}
}

func recoveryAttachArrivalFromSealed(sealed protocol.SealedFact, arrivalSequence int64) relay.Arrival {
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

type recoveryAttachFixture struct {
	store       *continuitysqlite.Store
	coordinator *Coordinator
	remote      *remoteFixture
	recovery    credential.ProjectRecoveryCredential
	prepared    credential.TrustedProjectCredential
}

func newRecoveryAttachFixture(t *testing.T, head int64) recoveryAttachFixture {
	t.Helper()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 6, []uint32{recovery.WriteGeneration})
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	coordinator := mustCoordinator(t, store, seedRemote)
	registration, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind prepared recovery attach credential: %v", err)
	}
	preSnapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5}
	preRecords := testGuardInventoryRecords(t, recovery)
	postSnapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: head}
	postRecords := append(append([]relay.EnvironmentInventoryRecord(nil), preRecords...), recoveryRegistrationInventoryRecord(registration))
	preRemote := inventoryRemote(recovery, preSnapshot, preRecords)
	postRemote := inventoryRemote(recovery, postSnapshot, postRecords)
	committed := false
	remote := preRemote
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		pages := preRemote.environmentPages
		if committed {
			pages = postRemote.environmentPages
		}
		page, ok := pages[request.AfterEnvironmentID]
		if !ok {
			return relay.EnvironmentInventoryPage{}, relay.ErrInvalidArgument
		}
		page.Environments = append([]relay.EnvironmentInventoryRecord(nil), page.Environments...)
		return page, nil
	}
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if committed {
			return relay.EnvironmentRegistrationStatus{
				Disposition: relay.EnvironmentRegistrationExact,
				State:       registrationChannelState(recovery, 6, head),
			}, nil
		}
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 5, 0),
		}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		committed = true
		return registrationChannelState(recovery, 6, 0), nil
	}
	return recoveryAttachFixture{
		store: store, coordinator: mustCoordinator(t, store, remote), remote: remote,
		recovery: recovery, prepared: prepared,
	}
}
