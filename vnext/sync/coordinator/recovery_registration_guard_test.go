package coordinator

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestStageRecoveryRegistrationGuardStagesReadyCandidateAndResumesAcrossReopen(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	first, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("stage recovery registration guard: %v", err)
	}
	assertReadyRecoveryRegistrationGuard(t, first, registration, snapshot, 2, int64(len(records)))
	assertNoCanonicalSyncAuthority(t, store, recovery.ProjectID)
	persisted, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if err != nil || !found || first.candidate == nil || !reflect.DeepEqual(persisted, *first.candidate) {
		t.Fatalf("persisted ready candidate = (%#v, %v, %v), want %#v", persisted, found, err, first.candidate)
	}

	second, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("exact guard retry: %v", err)
	}
	if !reflect.DeepEqual(second, first) {
		t.Fatalf("exact guard retry = %#v, want %#v", second, first)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close store before reopen: %v", err)
	}
	reopened := openCoordinatorStoreAt(t, root, writerID)
	reopenedCoordinator := mustCoordinator(t, reopened, remote)
	third, err := reopenedCoordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("guard retry after reopen: %v", err)
	}
	if !reflect.DeepEqual(third, first) {
		t.Fatalf("guard retry after reopen = %#v, want %#v", third, first)
	}
	assertNoCanonicalSyncAuthority(t, reopened, recovery.ProjectID)
	if remote.createCalls != 0 {
		t.Fatalf("CreateChannel calls = %d, want 0 for target > 1", remote.createCalls)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardAdvancesFromExactCanonicalBaseWithoutPromotion(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)
	baseSnapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 2}
	baseAuthority := syncAuthorityFromGuardRecords(t, recovery, baseSnapshot, records[:2])
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, baseAuthority); err != nil {
		t.Fatalf("install canonical base: %v", err)
	}
	before, err := store.CurrentSyncAuthorityBinding(context.Background(), recovery.ProjectID)
	if err != nil {
		t.Fatalf("read canonical base binding: %v", err)
	}

	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 9}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	guard, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("stage advancing guard: %v", err)
	}
	assertReadyRecoveryRegistrationGuard(t, guard, registration, snapshot, 2, int64(len(records)))
	if guard.candidate == nil || guard.candidate.Snapshot.BaseAuthorityDigestVersion != before.AuthorityDigestVersion ||
		guard.candidate.Snapshot.BaseAuthorityDigest != before.AuthorityDigest {
		t.Fatal("ready candidate did not bind the exact canonical base")
	}
	after, err := store.CurrentSyncAuthorityBinding(context.Background(), recovery.ProjectID)
	if err != nil || after != before {
		t.Fatalf("canonical authority after guard = (%#v, %v), want unchanged %#v", after, err, before)
	}
	if remote.createCalls != 0 {
		t.Fatalf("CreateChannel calls = %d, want 0 for target > 1", remote.createCalls)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardRejectsMembershipMismatchBeforeCandidateWrite(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)[:4]
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 4, ArrivalHead: 8}, records)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
		t.Fatalf("mismatched inventory returned guard %#v", got)
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	if remote.createCalls != 0 {
		t.Fatalf("CreateChannel calls = %d, want 0 for target > 1 failure", remote.createCalls)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardVerifiesWholeFirstPageBeforeCandidateWrite(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)
	records[3].CertificateBytes = append([]byte(nil), records[3].CertificateBytes...)
	records[3].CertificateBytes[len(records[3].CertificateBytes)-1] ^= 0xff
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 9}, records)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) || err == nil {
		t.Fatalf("corrupt first page guard = (%#v, %v), want refusal", got, err)
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	if remote.createCalls != 0 {
		t.Fatalf("CreateChannel calls = %d, want 0 for target > 1", remote.createCalls)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardUsesEphemeralEmptyGuardForFirstMembership(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)

	first, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("stage empty-channel guard: %v", err)
	}
	second, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("exact empty-channel guard retry: %v", err)
	}
	want := recoveryRegistrationGuard{targetMembershipGeneration: 1, inventorySnapshot: relay.EnvironmentInventorySnapshot{}}
	if !reflect.DeepEqual(first, want) || !reflect.DeepEqual(second, want) {
		t.Fatalf("empty guards = (%#v, %#v), want %#v", first, second, want)
	}
	if remote.createCalls != 2 || len(remote.createRequests) != 2 || !reflect.DeepEqual(remote.createRequests[0], remote.createRequests[1]) {
		t.Fatalf("CreateChannel exact replays = %d requests %#v", remote.createCalls, remote.createRequests)
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	assertNoCanonicalSyncAuthority(t, store, recovery.ProjectID)
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardTreatsFirstMembershipAsObservationNotReservation(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	var injectedBinding continuitysqlite.SyncAuthorityBinding
	remote.create = func(_ context.Context, channel relay.Channel) (relay.ChannelState, error) {
		record := testInventoryRecord(t, recovery, 1, 1)
		authority := syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, []relay.EnvironmentInventoryRecord{record})
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, authority); err != nil {
			t.Fatalf("inject concurrent canonical authority: %v", err)
		}
		var err error
		injectedBinding, err = store.CurrentSyncAuthorityBinding(context.Background(), recovery.ProjectID)
		if err != nil {
			t.Fatalf("read injected canonical authority: %v", err)
		}
		return relay.ChannelState{ChannelID: channel.ChannelID, RelayGeneration: channel.RelayGeneration}, nil
	}
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)

	guard, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("first-membership observation after concurrent local change: %v", err)
	}
	if guard.targetMembershipGeneration != 1 || guard.inventorySnapshot != (relay.EnvironmentInventorySnapshot{}) || guard.candidate != nil {
		t.Fatalf("first-membership observation = %#v", guard)
	}
	after, err := store.CurrentSyncAuthorityBinding(context.Background(), recovery.ProjectID)
	if err != nil || after != injectedBinding {
		t.Fatalf("canonical authority after guard = (%#v, %v), want injected %#v", after, err, injectedBinding)
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	if remote.createCalls != 1 || len(remote.environmentRequests) != 1 {
		t.Fatalf("guard remote calls = create:%d inventory:%d", remote.createCalls, len(remote.environmentRequests))
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardRequiresPristineLocalStateBeforeEmptyCreate(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	for _, test := range []struct {
		name string
		seed func(*testing.T, *continuitysqlite.Store)
	}{
		{name: "canonical authority", seed: func(t *testing.T, store *continuitysqlite.Store) {
			record := testInventoryRecord(t, recovery, 1, 1)
			authority := syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, []relay.EnvironmentInventoryRecord{record})
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, authority); err != nil {
				t.Fatalf("install canonical authority: %v", err)
			}
		}},
		{name: "active candidate", seed: func(t *testing.T, store *continuitysqlite.Store) {
			record := testInventoryRecord(t, recovery, 1, 1)
			environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
			if err != nil {
				t.Fatalf("translate candidate environment: %v", err)
			}
			_, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), recovery.ProjectID, continuitysqlite.SyncAuthoritySnapshot{
				ChannelID:            continuitysqlite.SyncChannelID(recovery.ChannelID),
				RelayGeneration:      [32]byte(recovery.RelayGeneration),
				AdminPublicKey:       [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
				MembershipGeneration: 1,
			}, continuitysqlite.SyncAuthorityPage{
				ThroughEnvironmentID: environment.EnvironmentID,
				Environments:         []continuitysqlite.SyncEnvironmentCertificate{environment},
				More:                 true,
			})
			if err != nil {
				t.Fatalf("stage active candidate: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openCoordinatorStore(t, writerID)
			test.seed(t, store)
			remote := emptyInventoryRemote(recovery)
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)

			got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
				t.Fatalf("non-pristine local state returned guard %#v", got)
			}
			assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
			if remote.createCalls != 0 || len(remote.environmentRequests) != 0 {
				t.Fatal("non-pristine local state reached empty-channel remote workflow")
			}
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestStageRecoveryRegistrationGuardRejectsNonzeroCreateStateBeforeInventory(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	remote.create = func(_ context.Context, channel relay.Channel) (relay.ChannelState, error) {
		return relay.ChannelState{
			ChannelID:            channel.ChannelID,
			RelayGeneration:      channel.RelayGeneration,
			MembershipGeneration: 1,
			Head:                 0,
		}, nil
	}
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)

	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
		t.Fatalf("nonzero create state returned guard %#v", got)
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	if remote.createCalls != 1 || len(remote.environmentRequests) != 0 {
		t.Fatalf("nonzero create workflow calls = create:%d inventory:%d", remote.createCalls, len(remote.environmentRequests))
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardRejectsMismatchedCreateStateBeforeInventory(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	remote.create = func(_ context.Context, channel relay.Channel) (relay.ChannelState, error) {
		state := relay.ChannelState{ChannelID: channel.ChannelID, RelayGeneration: channel.RelayGeneration}
		state.ChannelID[0] ^= 0xff
		return state, nil
	}
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)

	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
		t.Fatalf("mismatched create state returned guard %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseChannelCreation, ActionRestartRecovery)
	if remote.createCalls != 1 || len(remote.environmentRequests) != 0 {
		t.Fatalf("mismatched create workflow calls = create:%d inventory:%d", remote.createCalls, len(remote.environmentRequests))
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardRejectsUnsafeEmptyChannelStateWithoutCandidate(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	for _, test := range []struct {
		name     string
		snapshot relay.EnvironmentInventorySnapshot
		records  func(*testing.T) []relay.EnvironmentInventoryRecord
		wantCode ProblemCode
	}{
		{name: "arrival drift", snapshot: relay.EnvironmentInventorySnapshot{ArrivalHead: 1}, wantCode: CodeRemote},
		{name: "membership drift", snapshot: relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, records: func(t *testing.T) []relay.EnvironmentInventoryRecord {
			return []relay.EnvironmentInventoryRecord{testInventoryRecord(t, recovery, 1, 1)}
		}, wantCode: CodeConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openCoordinatorStore(t, writerID)
			var records []relay.EnvironmentInventoryRecord
			if test.records != nil {
				records = test.records(t)
			}
			remote := inventoryRemote(recovery, test.snapshot, records)
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)

			got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
				t.Fatalf("unsafe empty inventory returned guard %#v", got)
			}
			assertProblem(t, err, test.wantCode, PhaseEnvironmentInventory, ActionRestartRecovery)
			assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestStageRecoveryRegistrationGuardRetainsResumableCandidateAfterPageFault(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, records)
	pages := remote.environmentPages
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		if request.AfterEnvironmentID != "" {
			return relay.EnvironmentInventoryPage{}, context.Canceled
		}
		return pages[request.AfterEnvironmentID], nil
	}
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("faulted guard = (%#v, %v), want zero and context.Canceled", got, err)
	}
	staging, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || staging.Ready || staging.PageCount != 1 {
		t.Fatalf("resumable candidate = (%#v, %v, %v), want one staging page", staging, found, readErr)
	}
	if remote.environmentRequests[0].Snapshot != nil {
		t.Fatalf("fresh guard first request snapshot = %#v, want nil", remote.environmentRequests[0].Snapshot)
	}

	retryRequestStart := len(remote.environmentRequests)
	currentUnpinnedSnapshot := snapshot
	currentUnpinnedSnapshot.ArrivalHead++
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		if request.Snapshot == nil {
			page := pages[request.AfterEnvironmentID]
			page.Snapshot = currentUnpinnedSnapshot
			return page, nil
		}
		if *request.Snapshot != snapshot {
			return relay.EnvironmentInventoryPage{}, relay.ErrMembershipChanged
		}
		return pages[request.AfterEnvironmentID], nil
	}
	ready, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("resume guard after page fault: %v", err)
	}
	assertReadyRecoveryRegistrationGuard(t, ready, registration, snapshot, 2, int64(len(records)))
	if ready.candidate == nil || ready.candidate.CandidateID != staging.CandidateID {
		t.Fatalf("resumed candidate ID = %#v, want retained %x", ready.candidate, staging.CandidateID)
	}
	firstRetryRequest := remote.environmentRequests[retryRequestStart]
	if firstRetryRequest.AfterEnvironmentID != "" || firstRetryRequest.Snapshot == nil || *firstRetryRequest.Snapshot != snapshot {
		t.Fatalf("first retry request = %#v, want first page pinned to %#v", firstRetryRequest, snapshot)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardPreservesCandidateWhenPinnedMembershipChanges(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, records)
	pages := remote.environmentPages
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		if request.AfterEnvironmentID != "" {
			return relay.EnvironmentInventoryPage{}, context.Canceled
		}
		return pages[request.AfterEnvironmentID], nil
	}
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	if _, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration); !errors.Is(err, context.Canceled) {
		t.Fatalf("initial page fault error = %v, want context.Canceled", err)
	}
	staging, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if err != nil || !found || staging.Ready || staging.PageCount != 1 {
		t.Fatalf("retained staging candidate = (%#v, %v, %v)", staging, found, err)
	}

	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		if request.Snapshot == nil || *request.Snapshot != snapshot {
			t.Fatalf("membership-change retry request = %#v, want pinned %#v", request, snapshot)
		}
		return relay.EnvironmentInventoryPage{}, relay.ErrMembershipChanged
	}
	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
		t.Fatalf("membership-changed retry returned guard %#v", got)
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRetry)
	after, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !reflect.DeepEqual(after, staging) {
		t.Fatalf("candidate after membership change = (%#v, %v, %v), want unchanged %#v", after, found, readErr, staging)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardRejectsCandidateWideMembershipCorruption(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	for _, test := range []struct {
		name   string
		mutate func(*testing.T, []relay.EnvironmentInventoryRecord)
	}{
		{name: "cross-page duplicate and gap", mutate: func(_ *testing.T, records []relay.EnvironmentInventoryRecord) {
			records[4].MembershipGeneration = 4
			certificate := mustResignInventoryCertificate(t, recovery, records[4])
			records[4].CertificateID = certificate.CertificateID
			records[4].CertificateBytes = certificate.CertificateBytes
		}},
		{name: "historical canonical mutation", mutate: func(t *testing.T, records []relay.EnvironmentInventoryRecord) {
			records[0] = mutateGuardRecordPublicKey(t, recovery, records[0])
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openCoordinatorStore(t, writerID)
			records := testGuardInventoryRecords(t, recovery)
			if test.name == "historical canonical mutation" {
				base := syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 2}, records[:2])
				if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, base); err != nil {
					t.Fatalf("install canonical base: %v", err)
				}
			}
			test.mutate(t, records)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 9}, records)
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

			got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) || err == nil {
				t.Fatalf("corrupt inventory guard = (%#v, %v), want refusal", got, err)
			}
			candidate, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
			if readErr != nil || (found && candidate.Ready) {
				t.Fatalf("corrupt inventory candidate = (%#v, %v, %v), want absent or staging", candidate, found, readErr)
			}
			if test.name == "cross-page duplicate and gap" && (!found || candidate.PageCount != 1 || candidate.EnvironmentCount != relay.MaxEnvironmentInventoryPage) {
				t.Fatalf("invalid later page changed prior checkpoint: candidate=(%#v, %v)", candidate, found)
			}
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestPrepareRecoveryDefersCandidateWideCoverageToRegistrationGuard(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := testGuardInventoryRecords(t, recovery)
	records[4].MembershipGeneration = 4
	resigned := mustResignInventoryCertificate(t, recovery, records[4])
	records[4].CertificateID = resigned.CertificateID
	records[4].CertificateBytes = resigned.CertificateBytes
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 9}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)

	prepared, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if err != nil {
		t.Fatalf("disposable recovery preflight: %v", err)
	}
	registration, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind prospective registration: %v", err)
	}
	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) || err == nil {
		t.Fatalf("candidate-wide corrupt guard = (%#v, %v), want refusal", got, err)
	}
	candidate, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || candidate.Ready || candidate.PageCount != 1 {
		t.Fatalf("candidate-wide corrupt checkpoint = (%#v, %v, %v), want retained staging first page", candidate, found, readErr)
	}
	if remote.createCalls != 0 || remote.registerCalls != 0 {
		t.Fatalf("candidate-wide proof called mutation endpoints: create:%d register:%d", remote.createCalls, remote.registerCalls)
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardStagesVerifiedRetirement(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := []relay.EnvironmentInventoryRecord{
		testRetiredInventoryRecord(t, recovery, 1, 1, 6, 3),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: 9}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 7)

	guard, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("stage guard with retirement: %v", err)
	}
	assertReadyRecoveryRegistrationGuard(t, guard, registration, snapshot, 2, int64(len(records)))
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardDoesNotImposeLifetimeEnvironmentCap(t *testing.T) {
	writerID := testEnvironmentID(900)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	records := make([]relay.EnvironmentInventoryRecord, 257)
	for index := range records {
		records[index] = testInventoryRecord(t, recovery, uint16(index+1), uint32(index+1))
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 257, ArrivalHead: 13}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 258)

	guard, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("stage large recovery guard: %v", err)
	}
	assertReadyRecoveryRegistrationGuard(t, guard, registration, snapshot, 65, int64(len(records)))
	if len(remote.environmentRequests) != 65 {
		t.Fatalf("inventory requests = %d, want 65", len(remote.environmentRequests))
	}
	assertNoMutationCalls(t, remote)
}

func TestRecoveryInventoryTranslationClonesCertificateAndRetirementBytes(t *testing.T) {
	recovery := testBindableRecoveryCredential(t)
	record := testRetiredInventoryRecord(t, recovery, 1, 1, 2, 3)
	wantCertificate := append([]byte(nil), record.CertificateBytes...)
	wantRetirement := append([]byte(nil), record.Retirement.RetirementBytes...)

	got, err := syncEnvironmentCertificateFromRecoveryInventory(record)
	if err != nil {
		t.Fatalf("translate retired inventory record: %v", err)
	}
	record.CertificateBytes[0] ^= 0xff
	record.Retirement.RetirementBytes[0] ^= 0xff
	if !bytes.Equal(got.CertificateBytes, wantCertificate) || got.Retirement == nil || !bytes.Equal(got.Retirement.RetirementBytes, wantRetirement) {
		t.Fatal("translated environment aliases source certificate or retirement bytes")
	}
	got.CertificateBytes[1] ^= 0xff
	got.Retirement.RetirementBytes[1] ^= 0xff
	if bytes.Equal(got.CertificateBytes, record.CertificateBytes) || bytes.Equal(got.Retirement.RetirementBytes, record.Retirement.RetirementBytes) {
		t.Fatal("translated environment and source unexpectedly share mutable bytes")
	}
}

func TestStageRecoveryRegistrationGuardValidatesContextBeforeActions(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	if _, err := coordinator.stageRecoveryRegistrationGuard(nil, recovery.ProjectID, recovery, registration); err == nil {
		t.Fatal("nil context error = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.stageRecoveryRegistrationGuard(ctx, recovery.ProjectID, recovery, registration); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", err)
	}
	if len(remote.environmentRequests) != 0 || remote.createCalls != 0 {
		t.Fatal("invalid context reached remote workflow")
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardRejectsForgedBindingAndProjectBeforeActions(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	for _, test := range []struct {
		name     string
		expected continuity.ProjectID
		mutate   func(*preparedRecoveryRegistration)
	}{
		{name: "expected project", expected: testProjectID(99)},
		{name: "target mismatch", expected: recovery.ProjectID, mutate: func(registration *preparedRecoveryRegistration) {
			registration.targetMembershipGeneration++
		}},
		{name: "certificate id mismatch", expected: recovery.ProjectID, mutate: func(registration *preparedRecoveryRegistration) {
			registration.certificateID[0] ^= 0xff
		}},
		{name: "writer mismatch", expected: recovery.ProjectID, mutate: func(registration *preparedRecoveryRegistration) {
			registration.environment.EnvironmentID = relay.EnvironmentID(testEnvironmentID(201))
		}},
		{name: "certificate bytes mismatch", expected: recovery.ProjectID, mutate: func(registration *preparedRecoveryRegistration) {
			registration.environment.CertificateBytes = append([]byte(nil), registration.environment.CertificateBytes...)
			registration.environment.CertificateBytes[len(registration.environment.CertificateBytes)-1] ^= 0xff
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openCoordinatorStore(t, writerID)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5}, testGuardInventoryRecords(t, recovery))
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
			if test.mutate != nil {
				test.mutate(&registration)
			}

			got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), test.expected, recovery, registration)
			if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
				t.Fatalf("forged input returned guard %#v", got)
			}
			assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
			if len(remote.environmentRequests) != 0 || remote.createCalls != 0 {
				t.Fatal("forged input reached remote workflow")
			}
			assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestStageRecoveryRegistrationGuardRejectsIncompatibleCanonicalBaseBeforeRemote(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	for _, test := range []struct {
		name string
		base func(*testing.T) continuitysqlite.SyncAuthority
	}{
		{name: "identity mismatch", base: func(t *testing.T) continuitysqlite.SyncAuthority {
			records := testGuardInventoryRecords(t, recovery)[:2]
			authority := syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 2}, records)
			authority.ChannelID[0] ^= 0xff
			return authority
		}},
		{name: "membership beyond pre-registration target", base: func(t *testing.T) continuitysqlite.SyncAuthority {
			records := append(testGuardInventoryRecords(t, recovery), testInventoryRecord(t, recovery, 6, 6))
			return syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 6}, records)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openCoordinatorStore(t, writerID)
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, test.base(t)); err != nil {
				t.Fatalf("install incompatible canonical base: %v", err)
			}
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5}, testGuardInventoryRecords(t, recovery))
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

			got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
				t.Fatalf("incompatible base returned guard %#v", got)
			}
			assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
			if len(remote.environmentRequests) != 0 || remote.createCalls != 0 {
				t.Fatal("incompatible base reached remote workflow")
			}
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestStageRecoveryRegistrationGuardRejectsIncompatibleActiveCandidateBeforeRemote(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	record := testInventoryRecord(t, recovery, 1, 1)
	environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
	if err != nil {
		t.Fatalf("translate candidate environment: %v", err)
	}
	if _, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), recovery.ProjectID, continuitysqlite.SyncAuthoritySnapshot{
		ChannelID:            continuitysqlite.SyncChannelID(recovery.ChannelID),
		RelayGeneration:      [32]byte(recovery.RelayGeneration),
		AdminPublicKey:       [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
		MembershipGeneration: 4,
	}, continuitysqlite.SyncAuthorityPage{
		ThroughEnvironmentID: environment.EnvironmentID,
		Environments:         []continuitysqlite.SyncEnvironmentCertificate{environment},
		More:                 true,
	}); err != nil {
		t.Fatalf("stage incompatible active candidate: %v", err)
	}
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)

	got, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, recoveryRegistrationGuard{}) {
		t.Fatalf("incompatible active candidate returned guard %#v", got)
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	if len(remote.environmentRequests) != 0 || remote.createCalls != 0 {
		t.Fatal("incompatible active candidate reached remote workflow")
	}
	assertNoMutationCalls(t, remote)
}

func TestStageRecoveryRegistrationGuardDoesNotPersistCredentialAuthority(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 6, []uint32{recovery.WriteGeneration})
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind prepared registration: %v", err)
	}
	if _, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration); err != nil {
		t.Fatalf("stage registration guard: %v", err)
	}

	ownerSecret := recovery.OwnerRelayAuthorization.Secret()
	ownerTokenID := recovery.OwnerRelayAuthorization.ID()
	preparedSecret := prepared.EnvironmentRelayAuthorization.Secret()
	preparedTokenID := prepared.EnvironmentRelayAuthorization.ID()
	preparedTokenHash, err := relay.HashTokenSecret(relay.RelayTokenSecret(preparedSecret))
	if err != nil {
		t.Fatalf("hash prepared token: %v", err)
	}
	for name, forbidden := range map[string][]byte{
		"admin seed":            recovery.AdminSeed[:],
		"project root":          recovery.ProjectRoot[:],
		"owner bearer secret":   ownerSecret[:],
		"owner token id":        ownerTokenID[:],
		"prepared seed":         prepared.EnvironmentSeed[:],
		"prepared token id":     preparedTokenID[:],
		"prepared token secret": preparedSecret[:],
		"prepared token hash":   preparedTokenHash[:],
	} {
		assertDirectoryOmitsBytes(t, root, name, forbidden)
	}
	assertNoMutationCalls(t, remote)
}

func testGuardInventoryRecords(t *testing.T, recovery credential.ProjectRecoveryCredential) []relay.EnvironmentInventoryRecord {
	t.Helper()
	return []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
}

func bindGuardRegistration(t *testing.T, coordinator *Coordinator, recovery credential.ProjectRecoveryCredential, writerID continuity.EnvironmentID, target uint32) preparedRecoveryRegistration {
	t.Helper()
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, target, []uint32{recovery.WriteGeneration})
	registration, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind guard registration: %v", err)
	}
	return registration
}

func assertReadyRecoveryRegistrationGuard(t *testing.T, guard recoveryRegistrationGuard, registration preparedRecoveryRegistration, snapshot relay.EnvironmentInventorySnapshot, pages, environments int64) {
	t.Helper()
	if guard.candidate == nil || guard.targetMembershipGeneration != registration.targetMembershipGeneration || guard.inventorySnapshot != snapshot || !guard.candidate.Ready ||
		guard.candidate.PageCount != pages || guard.candidate.EnvironmentCount != environments ||
		guard.candidate.Snapshot.MembershipGeneration != snapshot.MembershipGeneration || guard.candidate.Snapshot.InventoryArrivalHead != snapshot.ArrivalHead {
		t.Fatalf("recovery registration guard = %#v, want ready target %d snapshot %#v", guard, registration.targetMembershipGeneration, snapshot)
	}
}

func assertNoActiveSyncAuthorityCandidate(t *testing.T, store *continuitysqlite.Store, projectID continuity.ProjectID) {
	t.Helper()
	candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || found {
		t.Fatalf("active authority candidate = (%#v, %v, %v), want absent", candidate, found, err)
	}
}

func assertNoCanonicalSyncAuthority(t *testing.T, store *continuitysqlite.Store, projectID continuity.ProjectID) {
	t.Helper()
	_, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) || syncErr.Code != continuitysqlite.SyncErrorNotFound {
		t.Fatalf("canonical authority error = %v, want not found", err)
	}
}

func openCoordinatorStoreAt(t *testing.T, root string, writerID continuity.EnvironmentID) *continuitysqlite.Store {
	t.Helper()
	store, err := continuitysqlite.Open(root, writerID)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func syncAuthorityFromGuardRecords(t *testing.T, recovery credential.ProjectRecoveryCredential, snapshot relay.EnvironmentInventorySnapshot, records []relay.EnvironmentInventoryRecord) continuitysqlite.SyncAuthority {
	t.Helper()
	authority := continuitysqlite.SyncAuthority{
		ChannelID:            continuitysqlite.SyncChannelID(recovery.ChannelID),
		RelayGeneration:      [32]byte(recovery.RelayGeneration),
		AdminPublicKey:       [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
		MembershipGeneration: snapshot.MembershipGeneration,
		InventoryArrivalHead: snapshot.ArrivalHead,
		Environments:         make([]continuitysqlite.SyncEnvironmentCertificate, 0, len(records)),
	}
	for _, record := range records {
		environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
		if err != nil {
			t.Fatalf("translate recovery inventory record: %v", err)
		}
		authority.Environments = append(authority.Environments, environment)
	}
	return authority
}

func mutateGuardRecordPublicKey(t *testing.T, recovery credential.ProjectRecoveryCredential, record relay.EnvironmentInventoryRecord) relay.EnvironmentInventoryRecord {
	t.Helper()
	certificate, err := protocol.ParseEnvironmentCertificate(record.CertificateBytes)
	if err != nil {
		t.Fatalf("parse certificate to mutate: %v", err)
	}
	seed, err := crypto.EnvironmentSeedFromBytes(testBytes(0x75, len(crypto.EnvironmentSeed{})))
	if err != nil {
		t.Fatalf("replacement environment seed: %v", err)
	}
	certificate.EnvironmentPublicKey = crypto.EnvironmentPublicKey(seed)
	certificate, err = crypto.SignEnvironmentCertificate(certificate, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign mutated certificate: %v", err)
	}
	encoded, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal mutated certificate: %v", err)
	}
	record.CertificateID = relay.Digest(protocol.CertificateID(certificate))
	record.CertificateBytes = encoded
	return record
}

func mustResignInventoryCertificate(t *testing.T, recovery credential.ProjectRecoveryCredential, record relay.EnvironmentInventoryRecord) relay.EnvironmentInventoryRecord {
	t.Helper()
	certificate, err := protocol.ParseEnvironmentCertificate(record.CertificateBytes)
	if err != nil {
		t.Fatalf("parse certificate to resign: %v", err)
	}
	certificate.MembershipGeneration = record.MembershipGeneration
	certificate, err = crypto.SignEnvironmentCertificate(certificate, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("resign certificate: %v", err)
	}
	encoded, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal resigned certificate: %v", err)
	}
	record.CertificateID = relay.Digest(protocol.CertificateID(certificate))
	record.CertificateBytes = encoded
	return record
}

func assertDirectoryOmitsBytes(t *testing.T, root, name string, forbidden []byte) {
	t.Helper()
	if len(forbidden) == 0 {
		t.Fatalf("empty forbidden byte marker for %s", name)
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(contents, forbidden) {
			t.Fatalf("%s was persisted in %s", name, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect continuity files for %s: %v", name, err)
	}
}
