package coordinator

import (
	"context"
	"errors"
	"testing"

	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestPersistRecoveryPruneInventoryResumesExactVerifiedCheckpoint(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 12)
	stageRecoveryPrunePersistencePrefix(t, fixture, 12)
	records := make([]relay.PruneInventoryRecord, 5)
	for index := range records {
		records[index] = testRecoveryPruneInventoryRecord(t, fixture, int64(index+1), byte(0xb0+index))
	}
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            int64(len(records)),
	}
	pages := recoveryPruneInventoryPages(fixture, snapshot, records)
	interrupted := errors.New("fixture prune inventory interruption")
	fixture.remote.prune = func(ctx context.Context, request relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
		if request.After == relay.MaxPruneInventoryPage {
			return relay.PruneInventoryPage{}, interrupted
		}
		return pages(ctx, request)
	}

	_, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	assertProblem(t, err, CodeUnavailable, PhasePruneInventory, ActionRetry)
	partial, found, err := fixture.store.CurrentSyncRecoveryPruneCandidate(context.Background(), fixture.recovery.ProjectID)
	if err != nil || !found {
		t.Fatalf("current interrupted prune candidate = {found=%t err=%v}, want retained candidate", found, err)
	}
	if partial.Ready || partial.PageCount != 1 || partial.PruneCount != 4 || partial.TargetCount != 4 ||
		partial.ThroughPruneSequence != 4 || partial.LastMembershipGeneration != fixture.binding.MembershipGeneration ||
		partial.Snapshot != (continuitysqlite.SyncRecoveryPruneSnapshot{Authority: fixture.binding, PruneHead: snapshot.PruneHead}) {
		t.Fatalf("interrupted prune candidate = %#v, want exact first-page checkpoint", partial)
	}
	if len(fixture.remote.pruneRequests) != 2 {
		t.Fatalf("interrupted prune requests = %d, want first page plus failed suffix", len(fixture.remote.pruneRequests))
	}

	equivocatedRecords := append([]relay.PruneInventoryRecord(nil), records...)
	equivocatedRecords[0] = testRecoveryPruneInventoryRecord(t, fixture, 1, 0xd0)
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, equivocatedRecords)
	fixture.remote.pruneRequests = nil
	_, err = fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	assertProblem(t, err, CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	afterEquivocation, found, err := fixture.store.CurrentSyncRecoveryPruneCandidate(context.Background(), fixture.recovery.ProjectID)
	if err != nil || !found || afterEquivocation != partial {
		t.Fatalf("candidate after prefix equivocation = (%#v, %t, %v), want unchanged %#v", afterEquivocation, found, err, partial)
	}
	if len(fixture.remote.pruneRequests) != 1 || fixture.remote.pruneRequests[0].After != 0 {
		t.Fatalf("equivocated prefix requests = %#v, want one from-zero revalidation", fixture.remote.pruneRequests)
	}

	fixture.remote.prune = pages
	fixture.remote.pruneRequests = nil
	ready, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("resume persisted recovery prune inventory: %v", err)
	}
	if !ready.Ready || ready.CandidateID != partial.CandidateID || ready.PageCount != 2 ||
		ready.PruneCount != 5 || ready.TargetCount != 5 || ready.ThroughPruneSequence != 5 ||
		ready.LastMembershipGeneration != fixture.binding.MembershipGeneration ||
		ready.RollingInventoryDigest == partial.RollingInventoryDigest || ready.InventoryDigest == (continuitysqlite.SyncRecoveryPruneInventoryDigest{}) {
		t.Fatalf("resumed prune candidate = %#v, want exact ready continuation", ready)
	}
	if len(fixture.remote.pruneRequests) != 2 || fixture.remote.pruneRequests[0].After != 0 ||
		fixture.remote.pruneRequests[0].Snapshot != nil || fixture.remote.pruneRequests[1].After != 4 ||
		fixture.remote.pruneRequests[1].Snapshot == nil || *fixture.remote.pruneRequests[1].Snapshot != snapshot {
		t.Fatalf("resume requests = %#v, want verified prefix plus exact pinned suffix", fixture.remote.pruneRequests)
	}

	fixture.remote.pruneRequests = nil
	replayed, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("replay ready recovery prune inventory: %v", err)
	}
	if replayed != ready {
		t.Fatalf("ready replay = %#v, want %#v", replayed, ready)
	}
	if len(fixture.remote.pruneRequests) != 0 {
		t.Fatalf("ready replay requests = %#v, want local exact-checkpoint reuse", fixture.remote.pruneRequests)
	}
	newerWatermark := recoveryDownloadWatermark(fixture.recovery.ProjectID, fixture.binding)
	newerWatermark.RelayHead++
	if _, err := fixture.store.AdvanceSyncRelayWatermark(context.Background(), newerWatermark); err != nil {
		t.Fatalf("advance retained relay watermark: %v", err)
	}
	_, err = fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	assertProblem(t, err, CodeConflict, PhasePruneInventory, ActionRetry)
	if len(fixture.remote.pruneRequests) != 0 {
		t.Fatalf("stale ready replay requests = %#v, want local snapshot-fence rejection", fixture.remote.pruneRequests)
	}

	mismatched := fixture.prepared
	mismatched.RelayURL = "https://different-relay.example.test"
	_, err = fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, mismatched, fixture.binding,
	)
	assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	if len(fixture.remote.pruneRequests) != 0 {
		t.Fatalf("mismatched ready replay requests = %#v, want pre-relay rejection", fixture.remote.pruneRequests)
	}
}

func TestPersistRecoveryPruneInventoryPersistsEmptySnapshot(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 0)
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, nil)

	ready, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("persist empty recovery prune inventory: %v", err)
	}
	if !ready.Ready || ready.PageCount != 1 || ready.PruneCount != 0 || ready.TargetCount != 0 ||
		ready.ThroughPruneSequence != 0 || ready.LastMembershipGeneration != 0 ||
		ready.InventoryDigest == (continuitysqlite.SyncRecoveryPruneInventoryDigest{}) {
		t.Fatalf("empty prune candidate = %#v, want one ready empty checkpoint", ready)
	}
}

func stageRecoveryPrunePersistencePrefix(t *testing.T, fixture recoveryDownloadFixture, head int64) {
	t.Helper()
	frames := make([]continuitysqlite.OpaqueSyncFrame, 0, head)
	var previous protocol.Digest
	for sequence := int64(1); sequence <= head; sequence++ {
		arrival := testRecoveryDownloadArrival(t, fixture.prepared, sequence, sequence, previous)
		frame, err := recoveryDownloadFrame(arrival, fixture.prepared.RelayGeneration)
		if err != nil {
			t.Fatalf("prepare recovery prune persistence arrival %d: %v", sequence, err)
		}
		frames = append(frames, frame)
		previous = protocol.Digest(arrival.Envelope.EnvelopeDigest)
	}
	if _, err := fixture.store.StageSyncPageUnderAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, 0, head, frames,
	); err != nil {
		t.Fatalf("stage recovery prune persistence prefix: %v", err)
	}
}
