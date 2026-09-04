package coordinator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestRegisterPreparedRecoveryEnvironmentFreshLaterMembership(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	wantAbsent := registrationChannelState(recovery, 5, 19)
	wantRegistered := registrationChannelState(recovery, 6, 20)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: wantAbsent}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return wantRegistered, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("register prepared recovery environment: %v", err)
	}
	if got.state != wantRegistered {
		t.Fatalf("registered state = %#v, want %#v", got.state, wantRegistered)
	}
	assertReadyRecoveryRegistrationGuard(t, got.guard, registration, snapshot, 2, 5)
	if remote.classifyCalls != 2 || remote.registerCalls != 1 {
		t.Fatalf("classification/registration calls = %d/%d, want 2/1", remote.classifyCalls, remote.registerCalls)
	}
	assertExactRegistrationRequests(t, append(append([]relay.RegisterEnvironmentRequest(nil), remote.classifyRequests...), remote.registerRequests...), recovery, registration)
	assertNoRegistrationPostMutationCalls(t, remote)
	assertNoCanonicalSyncAuthority(t, store, recovery.ProjectID)
}

func TestRegisterPreparedRecoveryEnvironmentRejectsInventoryHeadBelowInitialClassificationBeforeStaging(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 10}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 100)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return registrationChannelState(recovery, 6, 101), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) || err == nil {
		t.Fatalf("regressed inventory head = (%#v, %v), want refusal", got, err)
	}
	if remote.registerCalls != 0 {
		t.Fatalf("regressed inventory reached register %d times", remote.registerCalls)
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
}

func TestRegisterPreparedRecoveryEnvironmentRejectsRegisterHeadBelowFinalClassification(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 10}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		head := int64(10)
		if remote.classifyCalls == 2 {
			head = 100
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return registrationChannelState(recovery, 6, 20), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("regressed register head returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 1 {
		t.Fatalf("regressed register calls = %d, want exactly one", remote.registerCalls)
	}
}

func TestRegisterPreparedRecoveryEnvironmentCarriesMonotonicObservedHeadFloor(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 100}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		head := int64(100)
		if remote.classifyCalls == 2 {
			head = 120
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return registrationChannelState(recovery, 6, 120), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || got.state != registrationChannelState(recovery, 6, 120) || got.guard.inventorySnapshot.ArrivalHead != 100 {
		t.Fatalf("monotonic registration = (%#v, %v)", got, err)
	}
	if remote.classifyCalls != 2 || remote.registerCalls != 1 {
		t.Fatalf("monotonic calls classify/register = %d/%d, want 2/1", remote.classifyCalls, remote.registerCalls)
	}
}

func TestRegisterPreparedRecoveryEnvironmentSealsHigherInventoryHead(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 20}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		head := int64(10)
		if remote.classifyCalls == 2 {
			head = 20
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return registrationChannelState(recovery, 6, 21), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || got.state != registrationChannelState(recovery, 6, 21) || got.guard.inventorySnapshot != snapshot {
		t.Fatalf("higher inventory head registration = (%#v, %v)", got, err)
	}
	if remote.classifyCalls != 2 || remote.registerCalls != 1 {
		t.Fatalf("higher inventory head calls classify/register = %d/%d, want 2/1", remote.classifyCalls, remote.registerCalls)
	}
	wantFrontier := testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 21)
	if got, err := store.AdvanceSyncRelayWatermark(
		context.Background(),
		testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 20),
	); err != nil || got != wantFrontier {
		t.Fatalf("lower relay-head observation = (%#v, %v), want retained %#v", got, err, wantFrontier)
	}
}

func TestRegisterPreparedRecoveryEnvironmentFreshFirstMembership(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if remote.classifyCalls == 1 {
			return relay.EnvironmentRegistrationStatus{}, relay.ErrNotFound
		}
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 0, 0),
		}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return registrationChannelState(recovery, 1, 1), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("register first recovery environment: %v", err)
	}
	if got.guard.targetMembershipGeneration != 1 || got.guard.inventorySnapshot != (relay.EnvironmentInventorySnapshot{}) || got.guard.candidate != nil {
		t.Fatalf("first-membership guard = %#v", got.guard)
	}
	if got.state != registrationChannelState(recovery, 1, 1) {
		t.Fatalf("first-membership state = %#v", got.state)
	}
	if remote.classifyCalls != 2 || remote.createCalls != 1 || len(remote.environmentRequests) != 1 || remote.registerCalls != 1 {
		t.Fatalf("first-membership calls classify/create/inventory/register = %d/%d/%d/%d, want 2/1/1/1", remote.classifyCalls, remote.createCalls, len(remote.environmentRequests), remote.registerCalls)
	}
	assertNoActiveSyncAuthorityCandidate(t, store, recovery.ProjectID)
	assertNoCanonicalSyncAuthority(t, store, recovery.ProjectID)
	assertExactRegistrationRequests(t, append(append([]relay.RegisterEnvironmentRequest(nil), remote.classifyRequests...), remote.registerRequests...), recovery, registration)
	assertNoRegistrationPostMutationCalls(t, remote)
}

func TestRegisterPreparedRecoveryEnvironmentFirstMembershipMissingChannelCannotRegressRetainedHead(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 1, 5)); err != nil {
		t.Fatalf("seed retained relay head: %v", err)
	}
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{}, relay.ErrNotFound
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("headless missing channel returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.classifyCalls != 1 || remote.createCalls != 0 || len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
		t.Fatalf("headless missing channel calls classify/create/inventory/register = %d/%d/%d/%d, want 1/0/0/0", remote.classifyCalls, remote.createCalls, len(remote.environmentRequests), remote.registerCalls)
	}
}

func TestRegisterPreparedRecoveryEnvironmentCrossProcessFinalWatermarkAdvancePreventsRegister(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	concurrentStore := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		head := int64(19)
		if remote.classifyCalls == 2 {
			if _, err := concurrentStore.AdvanceSyncRelayWatermark(
				context.Background(),
				testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 5, 100),
			); err != nil {
				t.Fatalf("concurrent final relay-head advance: %v", err)
			}
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("failed final watermark returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 0 {
		t.Fatalf("failed final watermark reached register %d times", remote.registerCalls)
	}
	candidate, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !candidate.Ready || candidate.Snapshot.InventoryArrivalHead != snapshot.ArrivalHead {
		t.Fatalf("guard after failed final watermark = (%#v, %v, %v)", candidate, found, readErr)
	}
}

func TestRegisterPreparedRecoveryEnvironmentCrossProcessMembershipAdvancePreventsRegister(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	concurrentStore := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if remote.classifyCalls == 2 {
			if _, err := concurrentStore.AdvanceSyncRelayWatermark(
				context.Background(),
				testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 19),
			); err != nil {
				t.Fatalf("concurrent membership advance: %v", err)
			}
		}
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 5, 19),
		}, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("stale membership classification returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 0 {
		t.Fatalf("stale membership classification reached register %d times", remote.registerCalls)
	}
	candidate, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !candidate.Ready ||
		candidate.Snapshot.MembershipGeneration != 5 || candidate.Snapshot.InventoryArrivalHead != 19 {
		t.Fatalf("guard after membership advance = (%#v, %v, %v)", candidate, found, readErr)
	}
}

func TestRegisterPreparedRecoveryEnvironmentPostRegisterStoreFailureWithholdsCompletion(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		head := int64(19)
		if remote.classifyCalls == 2 {
			head = 20
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		if err := store.Close(); err != nil {
			t.Fatalf("close store before response watermark: %v", err)
		}
		return registrationChannelState(recovery, 6, 21), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("failed response watermark returned completion %#v", got)
	}
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	if remote.registerCalls != 1 {
		t.Fatalf("failed response watermark register calls = %d, want one", remote.registerCalls)
	}
	reopened := openCoordinatorStoreAt(t, root, writerID)
	candidate, found, readErr := reopened.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !candidate.Ready || candidate.Snapshot.InventoryArrivalHead != snapshot.ArrivalHead {
		t.Fatalf("guard after failed response watermark = (%#v, %v, %v)", candidate, found, readErr)
	}
	assertStaticRegistrationProblem(t, err, "store is closed")
}

func TestRegisterPreparedRecoveryEnvironmentCrossProcessWatermarkAdvanceAcceptsCurrentResponse(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	concurrentStore := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 20}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 5, 20),
		}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		if _, err := concurrentStore.AdvanceSyncRelayWatermark(
			context.Background(),
			testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 5, 100),
		); err != nil {
			t.Fatalf("concurrent relay-head advance: %v", err)
		}
		return registrationChannelState(recovery, 6, 100), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || got.state != registrationChannelState(recovery, 6, 100) {
		t.Fatalf("registration after concurrent current head = (%#v, %v)", got, err)
	}
	if remote.classifyCalls != 2 || remote.registerCalls != 1 {
		t.Fatalf("concurrent current-head calls classify/register = %d/%d, want 2/1", remote.classifyCalls, remote.registerCalls)
	}
	wantFrontier := testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 100)
	gotFrontier, err := store.AdvanceSyncRelayWatermark(
		context.Background(),
		testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 20),
	)
	if err != nil || gotFrontier != wantFrontier {
		t.Fatalf("lower current-response frontier = (%#v, %v), want retained %#v", gotFrontier, err, wantFrontier)
	}
}

func TestRegisterPreparedRecoveryEnvironmentCrossProcessWatermarkAdvanceRejectsStaleResponseAndRetry(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	concurrentStore := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 20}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 5, 20),
		}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		if _, err := concurrentStore.AdvanceSyncRelayWatermark(
			context.Background(),
			testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 5, 100),
		); err != nil {
			t.Fatalf("concurrent relay-head advance: %v", err)
		}
		return registrationChannelState(recovery, 6, 20), nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("stale response after concurrent advance returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	ready, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !ready.Ready || ready.Snapshot.InventoryArrivalHead != snapshot.ArrivalHead {
		t.Fatalf("ready guard after stale response = (%#v, %v, %v)", ready, found, readErr)
	}

	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationExact,
			State:       registrationChannelState(recovery, 6, 20),
		}, nil
	}
	retry, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(retry, completedRecoveryRegistration{}) {
		t.Fatalf("stale exact retry after concurrent advance returned completion %#v", retry)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 1 {
		t.Fatalf("stale retry registered again: calls=%d, want one", remote.registerCalls)
	}
	readyAfterRetry, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || readyAfterRetry != ready {
		t.Fatalf("ready guard changed after stale retry = (%#v, %v, %v), want %#v", readyAfterRetry, found, readErr, ready)
	}
}

func TestRegisterPreparedRecoveryEnvironmentCrossProcessUnknownOutcomeRetainsHeadAcrossReopen(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	concurrentStore := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 20}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 5, 20),
		}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		if _, err := concurrentStore.AdvanceSyncRelayWatermark(
			context.Background(),
			testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 5, 100),
		); err != nil {
			t.Fatalf("concurrent relay-head advance: %v", err)
		}
		return relay.ChannelState{}, errors.New("lost response after concurrent relay-head advance")
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("unknown concurrent outcome returned completion %#v", got)
	}
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	readyBeforeReopen, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !readyBeforeReopen.Ready {
		t.Fatalf("ready guard after unknown concurrent outcome = (%#v, %v, %v)", readyBeforeReopen, found, readErr)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before concurrent-head retry: %v", err)
	}
	reopened := openCoordinatorStoreAt(t, root, writerID)
	coordinator = mustCoordinator(t, reopened, remote)

	exactHead := int64(20)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationExact,
			State:       registrationChannelState(recovery, 6, exactHead),
		}, nil
	}
	stale, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(stale, completedRecoveryRegistration{}) {
		t.Fatalf("stale exact retry after reopen returned completion %#v", stale)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	readyAfterStaleRetry, found, readErr := reopened.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || readyAfterStaleRetry != readyBeforeReopen {
		t.Fatalf("ready guard changed across stale reopen retry = (%#v, %v, %v), want %#v", readyAfterStaleRetry, found, readErr, readyBeforeReopen)
	}

	exactHead = 100
	recovered, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || recovered.state != registrationChannelState(recovery, 6, 100) {
		t.Fatalf("current exact retry after reopen = (%#v, %v)", recovered, err)
	}
	if remote.registerCalls != 1 {
		t.Fatalf("unknown-outcome retries registered again: calls=%d, want one", remote.registerCalls)
	}
	if len(remote.environmentRequests) != 2 {
		t.Fatalf("unknown-outcome retries reread inventory: calls=%d, want original two pages", len(remote.environmentRequests))
	}
}

func TestRegisterPreparedRecoveryEnvironmentUnknownBeforeCommitRetriesExactBytes(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		if remote.registerCalls == 1 {
			return relay.ChannelState{}, errors.New("unknown transport outcome")
		}
		return registrationChannelState(recovery, 6, 20), nil
	}

	first, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(first, completedRecoveryRegistration{}) {
		t.Fatalf("unknown outcome returned completion %#v", first)
	}
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	second, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || second.state != registrationChannelState(recovery, 6, 20) {
		t.Fatalf("retry completion = (%#v, %v)", second, err)
	}
	if remote.registerCalls != 2 || !reflect.DeepEqual(remote.registerRequests[0], remote.registerRequests[1]) {
		t.Fatalf("register retry requests = %#v, want two exact byte-identical requests", remote.registerRequests)
	}
	assertNoRegistrationPostMutationCalls(t, remote)
}

func TestRegisterPreparedRecoveryEnvironmentUnknownBeforeCommitSurvivesHeadAdvance(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 10}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	advanced := false
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		head := int64(10)
		if advanced {
			head = 100
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		if remote.registerCalls == 1 {
			return relay.ChannelState{}, errors.New("unknown before commit")
		}
		return registrationChannelState(recovery, 6, 101), nil
	}

	first, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(first, completedRecoveryRegistration{}) {
		t.Fatalf("unknown outcome returned completion %#v", first)
	}
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	inventoryCallsAfterUnknown := len(remote.environmentRequests)
	readyBeforeRetry, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !readyBeforeRetry.Ready {
		t.Fatalf("ready guard after unknown outcome = (%#v, %v, %v)", readyBeforeRetry, found, readErr)
	}
	advanced = true

	second, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || second.state != registrationChannelState(recovery, 6, 101) {
		t.Fatalf("head-advanced protected retry = (%#v, %v), want completion", second, err)
	}
	if remote.registerCalls != 2 || !reflect.DeepEqual(remote.registerRequests[0], remote.registerRequests[1]) {
		t.Fatalf("head-advanced register retries = %#v, want two exact byte-identical requests", remote.registerRequests)
	}
	if len(remote.environmentRequests) != inventoryCallsAfterUnknown {
		t.Fatalf("head-advanced retry reread stale pinned inventory: before=%d after=%d", inventoryCallsAfterUnknown, len(remote.environmentRequests))
	}
	if second.guard.inventorySnapshot != snapshot || second.guard.candidate == nil || !second.guard.candidate.Ready {
		t.Fatalf("head-advanced retry did not preserve exact ready guard: %#v", second.guard)
	}
	readyAfterRetry, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || readyAfterRetry != readyBeforeRetry {
		t.Fatalf("ready guard changed across retry = (%#v, %v, %v), want %#v", readyAfterRetry, found, readErr, readyBeforeRetry)
	}
	allRequests := append(append([]relay.RegisterEnvironmentRequest(nil), remote.classifyRequests...), remote.registerRequests...)
	assertExactRegistrationRequests(t, allRequests, recovery, registration)
	wantFrontier := testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 101)
	if got, err := store.AdvanceSyncRelayWatermark(
		context.Background(),
		testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 6, 10),
	); err != nil || got != wantFrontier {
		t.Fatalf("lower retry frontier = (%#v, %v), want retained %#v", got, err, wantFrontier)
	}
}

func TestRegisterPreparedRecoveryEnvironmentLostResponseRetainsFinalHeadAcrossReopen(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 10}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	committed := false
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if committed {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, 20)}, nil
		}
		head := int64(10)
		if remote.classifyCalls == 2 {
			head = 100
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		committed = true
		return relay.ChannelState{}, errors.New("lost response after commit")
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("lost response returned completion %#v", got)
	}
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	readyBeforeReopen, found, readErr := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || !readyBeforeReopen.Ready {
		t.Fatalf("ready guard before reopen = (%#v, %v, %v)", readyBeforeReopen, found, readErr)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before retry: %v", err)
	}
	reopened := openCoordinatorStoreAt(t, root, writerID)
	coordinator = mustCoordinator(t, reopened, remote)

	got, err = coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("regressed exact retry returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 1 || remote.classifyCalls != 3 {
		t.Fatalf("regressed retry calls classify/register = %d/%d, want 3/1", remote.classifyCalls, remote.registerCalls)
	}
	if len(remote.environmentRequests) != 2 {
		t.Fatalf("regressed retry replayed inventory: calls=%d, want original two pages", len(remote.environmentRequests))
	}
	readyAfterReopen, found, readErr := reopened.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
	if readErr != nil || !found || readyAfterReopen != readyBeforeReopen {
		t.Fatalf("ready guard changed across regressed retry = (%#v, %v, %v), want %#v", readyAfterReopen, found, readErr, readyBeforeReopen)
	}
}

func TestRegisterPreparedRecoveryEnvironmentLostResponseConvergesAtRetainedOrHigherHead(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 10}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	committed := false
	exactHead := int64(100)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if committed {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, exactHead)}, nil
		}
		head := int64(10)
		if remote.classifyCalls == 2 {
			head = 100
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		committed = true
		return relay.ChannelState{}, errors.New("lost response after commit")
	}

	_, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	for _, head := range []int64{100, 120} {
		exactHead = head
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if err != nil || got.state != registrationChannelState(recovery, 6, head) {
			t.Fatalf("exact convergence at head %d = (%#v, %v)", head, got, err)
		}
	}
	if remote.registerCalls != 1 || len(remote.environmentRequests) != 2 {
		t.Fatalf("exact convergence replayed mutation/inventory: register=%d inventory=%d", remote.registerCalls, len(remote.environmentRequests))
	}
}

func TestRegisterPreparedRecoveryEnvironmentRegeneratedCertificateCannotResetRetainedHead(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 10}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	original := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	if _, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, original); err != nil {
		t.Fatalf("stage original guard: %v", err)
	}
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 5, 100)); err != nil {
		t.Fatalf("seed retained relay head: %v", err)
	}
	regenerated := regenerateRecoveryRegistrationCertificate(t, recovery, original)
	remote.environmentRequests = nil
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, 20)}, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, regenerated)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("regenerated certificate reset retained head: %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.classifyCalls != 1 || remote.registerCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatalf("regenerated retry calls classify/register/inventory = %d/%d/%d, want 1/0/0", remote.classifyCalls, remote.registerCalls, len(remote.environmentRequests))
	}
}

func TestRecoveryRegistrationStoreErrorMappersClassifyStoreFailures(t *testing.T) {
	const forbidden = "recovery-registration-store-sensitive-detail"
	for _, test := range []struct {
		name       string
		mapper     func(context.Context, error) error
		err        error
		wantCode   ProblemCode
		wantPhase  ProblemPhase
		wantAction ProblemAction
	}{
		{name: "inventory unavailable", mapper: mapRecoveryRegistrationStoreError, err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Detail: forbidden}, wantCode: CodeUnavailable, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRetry},
		{name: "inventory corruption", mapper: mapRecoveryRegistrationStoreError, err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Field: "sync_authority", Detail: forbidden}, wantCode: CodeInternal, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRepairLocalStore},
		{name: "registration unavailable", mapper: mapRecoveryRegistrationAtomStoreError, err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Detail: forbidden}, wantCode: CodeUnavailable, wantPhase: PhaseEnvironmentRegistration, wantAction: ActionRetry},
		{name: "registration corruption", mapper: mapRecoveryRegistrationAtomStoreError, err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Field: "sync_authority_candidate", Detail: forbidden}, wantCode: CodeInternal, wantPhase: PhaseEnvironmentRegistration, wantAction: ActionRepairLocalStore},
	} {
		t.Run(test.name, func(t *testing.T) {
			problem := test.mapper(context.Background(), test.err)
			assertProblem(t, problem, test.wantCode, test.wantPhase, test.wantAction)
			assertStaticRegistrationProblem(t, problem, forbidden)
		})
	}
}

func TestRegisterPreparedRecoveryEnvironmentUnknownAfterCommitConvergesWithoutMutation(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	committed := false
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if committed {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 7, 31)}, nil
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		committed = true
		return relay.ChannelState{}, errors.New("lost response after commit")
	}

	_, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	registeredCalls := remote.registerCalls
	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || got.state != registrationChannelState(recovery, 7, 31) {
		t.Fatalf("exact lost-response convergence = (%#v, %v)", got, err)
	}
	if remote.registerCalls != registeredCalls {
		t.Fatalf("exact retry registered again: before=%d after=%d", registeredCalls, remote.registerCalls)
	}
	if len(remote.environmentRequests) != 2 {
		t.Fatalf("exact retry reread remote inventory: calls=%d, want initial two pages only", len(remote.environmentRequests))
	}
	assertNoRegistrationPostMutationCalls(t, remote)
}

func TestRegisterPreparedRecoveryEnvironmentFinalClassificationConvergesAndExactAllowsAdvance(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if remote.classifyCalls == 1 {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 8, 44)}, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || got.state != registrationChannelState(recovery, 8, 44) {
		t.Fatalf("concurrent exact convergence = (%#v, %v)", got, err)
	}
	if remote.classifyCalls != 2 || remote.registerCalls != 0 {
		t.Fatalf("concurrent exact calls classify/register = %d/%d, want 2/0", remote.classifyCalls, remote.registerCalls)
	}

	remote.classifyCalls = 0
	remote.classifyRequests = nil
	remote.environmentRequests = nil
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 9, 55)}, nil
	}
	advanced, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil || advanced.state != registrationChannelState(recovery, 9, 55) {
		t.Fatalf("advanced exact completion = (%#v, %v)", advanced, err)
	}
	if remote.classifyCalls != 1 || remote.registerCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatalf("advanced exact remote calls classify/register/inventory = %d/%d/%d, want 1/0/0", remote.classifyCalls, remote.registerCalls, len(remote.environmentRequests))
	}
	assertNoRegistrationPostMutationCalls(t, remote)
}

func TestRegisterPreparedRecoveryEnvironmentInitialExactRevalidatesBindingAfterClassifier(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	if _, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration); err != nil {
		t.Fatalf("seed exact-path guard: %v", err)
	}
	remote.environmentRequests = nil
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		remote.endpoint = "https://changed-relay.example.test"
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, 20)}, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("drifted initial exact binding returned completion %#v", got)
	}
	assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	if remote.registerCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatal("drifted initial exact binding reached inventory or register")
	}
}

func TestRegisterPreparedRecoveryEnvironmentRejectsRelayConflictsAndMalformedValues(t *testing.T) {
	for _, test := range []struct {
		name       string
		classify   func(credential.ProjectRecoveryCredential, int) (relay.EnvironmentRegistrationStatus, error)
		register   func(credential.ProjectRecoveryCredential) (relay.ChannelState, error)
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "immutable collision", classify: func(_ credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{}, relay.ErrImmutableConflict
		}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "membership advance", classify: func(_ credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{}, relay.ErrMembershipChanged
		}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "malformed disposition", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{State: registrationChannelState(recovery, 5, 19)}, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "classifier wrong channel", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			status := relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}
			status.State.ChannelID[0] ^= 0xff
			return status, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "classifier wrong relay generation", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			status := relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}
			status.State.RelayGeneration[0] ^= 0xff
			return status, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "malformed absent membership", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 4, 19)}, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "malformed exact membership below target", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 5, 19)}, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "malformed exact head", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, -1)}, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "final absent head below guard", classify: func(recovery credential.ProjectRecoveryCredential, call int) (relay.EnvironmentRegistrationStatus, error) {
			head := int64(19)
			if call == 2 {
				head = 18
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "final exact head below guard", classify: func(recovery credential.ProjectRecoveryCredential, call int) (relay.EnvironmentRegistrationStatus, error) {
			if call == 2 {
				return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, 18)}, nil
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "registration rollback head", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}, register: func(recovery credential.ProjectRecoveryCredential) (relay.ChannelState, error) {
			return registrationChannelState(recovery, 6, 18), nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "registration membership below target", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}, register: func(recovery credential.ProjectRecoveryCredential) (relay.ChannelState, error) {
			return registrationChannelState(recovery, 5, 20), nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "registration wrong channel", classify: func(recovery credential.ProjectRecoveryCredential, _ int) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}, register: func(recovery credential.ProjectRecoveryCredential) (relay.ChannelState, error) {
			state := registrationChannelState(recovery, 6, 20)
			state.ChannelID[0] ^= 0xff
			return state, nil
		}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
	} {
		t.Run(test.name, func(t *testing.T) {
			writerID := testEnvironmentID(200)
			store := openCoordinatorStore(t, writerID)
			recovery := testBindableRecoveryCredential(t)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
			remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
				return test.classify(recovery, remote.classifyCalls)
			}
			if test.register != nil {
				remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
					return test.register(recovery)
				}
			}

			got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
				t.Fatalf("refusal returned completion %#v", got)
			}
			assertProblem(t, err, test.wantCode, PhaseEnvironmentRegistration, test.wantAction)
			if test.register == nil && remote.registerCalls != 0 {
				t.Fatalf("pre-registration refusal reached register %d times", remote.registerCalls)
			}
			if test.register != nil && remote.registerCalls != 1 {
				t.Fatalf("malformed success register calls = %d, want 1", remote.registerCalls)
			}
			assertNoRegistrationPostMutationCalls(t, remote)
		})
	}
}

func TestRegisterPreparedRecoveryEnvironmentFirstMembershipSecondAuthorizationFailureDoesNotRegister(t *testing.T) {
	for _, secondErr := range []error{relay.ErrNotFound, relay.ErrUnauthenticated} {
		writerID := testEnvironmentID(200)
		store := openCoordinatorStore(t, writerID)
		recovery := testBindableRecoveryCredential(t)
		remote := emptyInventoryRemote(recovery)
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			if remote.classifyCalls == 1 {
				return relay.EnvironmentRegistrationStatus{}, relay.ErrNotFound
			}
			return relay.EnvironmentRegistrationStatus{}, secondErr
		}

		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("second authorization failure returned completion %#v", got)
		}
		assertProblem(t, err, CodeAuthorization, PhaseEnvironmentRegistration, ActionCheckRecoveryAuthority)
		if remote.classifyCalls != 2 || remote.createCalls != 1 || len(remote.environmentRequests) != 1 || remote.registerCalls != 0 {
			t.Fatalf("second authorization failure calls classify/create/inventory/register = %d/%d/%d/%d", remote.classifyCalls, remote.createCalls, len(remote.environmentRequests), remote.registerCalls)
		}
	}
}

func TestRegisterPreparedRecoveryEnvironmentRejectsImpossibleFirstMembershipAbsentHead(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationAbsent,
			State:       registrationChannelState(recovery, 0, 1),
		}, nil
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("impossible G0/H1 status returned completion %#v", got)
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.createCalls != 0 || len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
		t.Fatalf("impossible G0/H1 reached workflow create/inventory/register = %d/%d/%d", remote.createCalls, len(remote.environmentRequests), remote.registerCalls)
	}
}

func TestRegisterPreparedRecoveryEnvironmentRejectsMutatedBindingAndMissingGuard(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)
	for _, test := range []struct {
		name   string
		mutate func(*preparedRecoveryRegistration)
	}{
		{name: "target", mutate: func(registration *preparedRecoveryRegistration) { registration.targetMembershipGeneration++ }},
		{name: "certificate bytes", mutate: func(registration *preparedRecoveryRegistration) { registration.environment.CertificateBytes[0] ^= 0xff }},
		{name: "token id", mutate: func(registration *preparedRecoveryRegistration) { registration.environment.Token.TokenID[0] ^= 0xff }},
		{name: "token hash", mutate: func(registration *preparedRecoveryRegistration) { registration.environment.Token.TokenHash[0] ^= 0xff }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openCoordinatorStore(t, writerID)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
			test.mutate(&registration)
			got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
				t.Fatalf("mutated binding returned completion %#v", got)
			}
			assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
			if remote.classifyCalls != 0 || remote.registerCalls != 0 || len(remote.environmentRequests) != 0 {
				t.Fatal("mutated binding reached remote workflow")
			}
		})
	}

	store := openCoordinatorStore(t, writerID)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, 20)}, nil
	}
	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("missing exact guard returned completion %#v", got)
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatal("missing exact guard reached inventory or register")
	}
}

func TestRegisterPreparedRecoveryEnvironmentRevalidatesGuardBeforeRegister(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	t.Run("later candidate removed", func(t *testing.T) {
		store := openCoordinatorStore(t, writerID)
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			if remote.classifyCalls == 2 {
				candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
				if err != nil || !found {
					t.Fatalf("read candidate to remove: %#v %v %v", candidate, found, err)
				}
				if err := store.DiscardSyncAuthorityCandidate(context.Background(), recovery.ProjectID, candidate.Checkpoint()); err != nil {
					t.Fatalf("remove candidate during final classification: %v", err)
				}
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("removed candidate returned completion %#v", got)
		}
		assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		if remote.registerCalls != 0 {
			t.Fatal("removed candidate reached register")
		}
	})

	t.Run("later candidate replaced", func(t *testing.T) {
		store := openCoordinatorStore(t, writerID)
		records := testGuardInventoryRecords(t, recovery)
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, records)
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			if remote.classifyCalls == 2 {
				candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
				if err != nil || !found {
					t.Fatalf("read candidate to replace: %#v %v %v", candidate, found, err)
				}
				if err := store.DiscardSyncAuthorityCandidate(context.Background(), recovery.ProjectID, candidate.Checkpoint()); err != nil {
					t.Fatalf("remove candidate to replace: %v", err)
				}
				environments := make([]continuitysqlite.SyncEnvironmentCertificate, 0, len(records))
				for _, record := range records {
					environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
					if err != nil {
						t.Fatalf("translate replacement candidate: %v", err)
					}
					environments = append(environments, environment)
				}
				replacementSnapshot := continuitysqlite.SyncAuthoritySnapshot{
					ChannelID:                  continuitysqlite.SyncChannelID(recovery.ChannelID),
					RelayGeneration:            [32]byte(recovery.RelayGeneration),
					AdminPublicKey:             [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
					MembershipGeneration:       5,
					InventoryArrivalHead:       20,
					BaseAuthorityDigestVersion: 0,
				}
				after := ""
				for start := 0; start < len(environments); start += relay.MaxEnvironmentInventoryPage {
					end := start + relay.MaxEnvironmentInventoryPage
					if end > len(environments) {
						end = len(environments)
					}
					_, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), recovery.ProjectID, replacementSnapshot, continuitysqlite.SyncAuthorityPage{
						AfterEnvironmentID:   after,
						ThroughEnvironmentID: environments[end-1].EnvironmentID,
						Environments:         environments[start:end],
						More:                 end < len(environments),
					})
					if err != nil {
						t.Fatalf("stage replacement candidate page: %v", err)
					}
					after = environments[end-1].EnvironmentID
				}
			}
			head := int64(19)
			if remote.classifyCalls == 2 {
				head = 20
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, head)}, nil
		}
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("replaced candidate returned completion %#v", got)
		}
		assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		if remote.registerCalls != 0 {
			t.Fatal("replaced candidate reached register")
		}
	})

	t.Run("later candidate removed during final exact", func(t *testing.T) {
		store := openCoordinatorStore(t, writerID)
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			if remote.classifyCalls == 2 {
				candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
				if err != nil || !found {
					t.Fatalf("read candidate during exact classification: %#v %v %v", candidate, found, err)
				}
				if err := store.DiscardSyncAuthorityCandidate(context.Background(), recovery.ProjectID, candidate.Checkpoint()); err != nil {
					t.Fatalf("remove candidate during exact classification: %v", err)
				}
				return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationExact, State: registrationChannelState(recovery, 6, 20)}, nil
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("stale exact guard returned completion %#v", got)
		}
		assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		if remote.registerCalls != 0 {
			t.Fatal("stale exact guard reached register")
		}
	})

	t.Run("later candidate removed during register", func(t *testing.T) {
		store := openCoordinatorStore(t, writerID)
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
		}
		remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
			candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID)
			if err != nil || !found {
				t.Fatalf("read candidate during register: %#v %v %v", candidate, found, err)
			}
			if err := store.DiscardSyncAuthorityCandidate(context.Background(), recovery.ProjectID, candidate.Checkpoint()); err != nil {
				t.Fatalf("remove candidate during register: %v", err)
			}
			return registrationChannelState(recovery, 6, 20), nil
		}
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("register-time stale guard returned completion %#v", got)
		}
		assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		if remote.registerCalls != 1 {
			t.Fatalf("register-time drift calls = %d, want one attempted registration", remote.registerCalls)
		}
	})

	t.Run("first membership local authority injected", func(t *testing.T) {
		store := openCoordinatorStore(t, writerID)
		remote := emptyInventoryRemote(recovery)
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			if remote.classifyCalls == 1 {
				return relay.EnvironmentRegistrationStatus{}, relay.ErrNotFound
			}
			record := testInventoryRecord(t, recovery, 1, 1)
			authority := syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, []relay.EnvironmentInventoryRecord{record})
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, authority); err != nil {
				t.Fatalf("inject local authority: %v", err)
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 0, 0)}, nil
		}
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("injected first-membership authority returned completion %#v", got)
		}
		assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		if remote.registerCalls != 0 {
			t.Fatal("injected first-membership authority reached register")
		}
	})

	t.Run("first membership local authority injected during register", func(t *testing.T) {
		store := openCoordinatorStore(t, writerID)
		remote := emptyInventoryRemote(recovery)
		coordinator := mustCoordinator(t, store, remote)
		registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
		remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
			if remote.classifyCalls == 1 {
				return relay.EnvironmentRegistrationStatus{}, relay.ErrUnauthenticated
			}
			return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 0, 0)}, nil
		}
		remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
			record := testInventoryRecord(t, recovery, 1, 1)
			authority := syncAuthorityFromGuardRecords(t, recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, []relay.EnvironmentInventoryRecord{record})
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), recovery.ProjectID, authority); err != nil {
				t.Fatalf("inject local authority during register: %v", err)
			}
			return registrationChannelState(recovery, 1, 1), nil
		}
		got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
		if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
			t.Fatalf("register-time first-membership drift returned completion %#v", got)
		}
		assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		if remote.registerCalls != 1 {
			t.Fatalf("first-membership register-time drift calls = %d, want one", remote.registerCalls)
		}
	})
}

func TestRegisterPreparedRecoveryEnvironmentContextCancellationBoundaries(t *testing.T) {
	writerID := testEnvironmentID(200)
	recovery := testBindableRecoveryCredential(t)

	store := openCoordinatorStore(t, writerID)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	if got, err := coordinator.registerPreparedRecoveryEnvironment(nil, recovery.ProjectID, recovery, registration); !reflect.DeepEqual(got, completedRecoveryRegistration{}) || err == nil {
		t.Fatalf("nil context = (%#v, %v), want refusal", got, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := coordinator.registerPreparedRecoveryEnvironment(canceled, recovery.ProjectID, recovery, registration); !reflect.DeepEqual(got, completedRecoveryRegistration{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled context = (%#v, %v), want context.Canceled", got, err)
	}
	if remote.classifyCalls != 0 {
		t.Fatal("invalid contexts reached classifier")
	}

	ctx, cancelFinal := context.WithCancel(context.Background())
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		if remote.classifyCalls == 2 {
			cancelFinal()
		}
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
	}
	got, err := coordinator.registerPreparedRecoveryEnvironment(ctx, recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation before register = (%#v, %v), want context.Canceled", got, err)
	}
	if remote.registerCalls != 0 {
		t.Fatal("canceled final boundary reached register")
	}
}

func TestRegisterPreparedRecoveryEnvironmentFirstMembershipJoinedCancellationDoesNotCreate(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 1)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{}, errors.Join(context.Canceled, relay.ErrUnauthenticated)
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("joined cancellation = (%#v, %v), want context.Canceled", got, err)
	}
	if remote.createCalls != 0 || len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
		t.Fatalf("joined cancellation reached mutation workflow create/inventory/register = %d/%d/%d", remote.createCalls, len(remote.environmentRequests), remote.registerCalls)
	}
}

func TestRegisterPreparedRecoveryEnvironmentDefensivelyClonesRequestsAndKeepsSecretsCallLocal(t *testing.T) {
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	prepared := testPreparedRecoveryCredential(t, recovery, writerID, 6, []uint32{recovery.WriteGeneration})
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration, err := coordinator.bindPreparedRecoveryRegistration(recovery.ProjectID, recovery, prepared)
	if err != nil {
		t.Fatalf("bind registration: %v", err)
	}
	wantCertificate := append([]byte(nil), registration.environment.CertificateBytes...)
	remote.classify = func(_ context.Context, request relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		request.Environment.CertificateBytes[0] ^= 0xff
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
	}
	remote.register = func(_ context.Context, request relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		request.Environment.CertificateBytes[0] ^= 0xff
		return registrationChannelState(recovery, 6, 20), nil
	}
	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("registration with mutating transport: %v", err)
	}
	if !bytes.Equal(registration.environment.CertificateBytes, wantCertificate) {
		t.Fatal("transport request mutation changed prepared registration certificate bytes")
	}
	assertExactRegistrationRequests(t, append(append([]relay.RegisterEnvironmentRequest(nil), remote.classifyRequests...), remote.registerRequests...), recovery, registration)
	assertCompletedRegistrationSecretFreeSchema(t, got)
	if fmt.Sprintf("%v", got) != "[REDACTED completed recovery registration]" || fmt.Sprintf("%#v", got) != "coordinator.completedRecoveryRegistration([REDACTED])" {
		t.Fatalf("completed registration formatting = %v / %#v", got, got)
	}

	ownerSecret := recovery.OwnerRelayAuthorization.Secret()
	ownerTokenID := recovery.OwnerRelayAuthorization.ID()
	preparedSecret := prepared.EnvironmentRelayAuthorization.Secret()
	preparedTokenID := prepared.EnvironmentRelayAuthorization.ID()
	preparedTokenHash, hashErr := relay.HashTokenSecret(relay.RelayTokenSecret(preparedSecret))
	if hashErr != nil {
		t.Fatalf("hash prepared token: %v", hashErr)
	}
	for name, forbidden := range map[string][]byte{
		"owner secret":        ownerSecret[:],
		"owner token id":      ownerTokenID[:],
		"prepared secret":     preparedSecret[:],
		"prepared token id":   preparedTokenID[:],
		"prepared token hash": preparedTokenHash[:],
	} {
		assertDirectoryOmitsBytes(t, root, name, forbidden)
	}
	assertNoRegistrationPostMutationCalls(t, remote)
}

func TestRegisterPreparedRecoveryEnvironmentRelayErrorsAreStaticAndSecretFree(t *testing.T) {
	const secretMarker = "registration-remote-secret-marker"
	for _, test := range []struct {
		name       string
		remoteErr  error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "unauthenticated", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrUnauthenticated), wantCode: CodeAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "not found", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrNotFound), wantCode: CodeAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "generation", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrGenerationMismatch), wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "rollback", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrRollback), wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "retired", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrRetired), wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "expired", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrExpired), wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "invalid", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrInvalidArgument), wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "unverified", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrUnverified), wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "closed", remoteErr: fmt.Errorf("%s: %w", secretMarker, relay.ErrClosed), wantCode: CodeUnavailable, wantAction: ActionRetry},
		{name: "unknown", remoteErr: errors.New(secretMarker), wantCode: CodeUnavailable, wantAction: ActionRetry},
	} {
		t.Run(test.name, func(t *testing.T) {
			writerID := testEnvironmentID(200)
			store := openCoordinatorStore(t, writerID)
			recovery := testBindableRecoveryCredential(t)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
			coordinator := mustCoordinator(t, store, remote)
			registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
			remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
				return relay.EnvironmentRegistrationStatus{}, test.remoteErr
			}
			got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
			if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
				t.Fatalf("relay error returned completion %#v", got)
			}
			assertProblem(t, err, test.wantCode, PhaseEnvironmentRegistration, test.wantAction)
			for _, formatted := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%#v", err)} {
				if strings.Contains(formatted, secretMarker) {
					t.Fatalf("relay detail leaked: %q", formatted)
				}
			}
			if remote.registerCalls != 0 {
				t.Fatal("classifier error reached register")
			}
		})
	}
}

func TestRegisterPreparedRecoveryEnvironmentMapsRegisterSentinelWithoutLoop(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}, testGuardInventoryRecords(t, recovery))
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	remote.classify = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{Disposition: relay.EnvironmentRegistrationAbsent, State: registrationChannelState(recovery, 5, 19)}, nil
	}
	remote.register = func(_ context.Context, _ relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
		return relay.ChannelState{}, fmt.Errorf("remote detail: %w", relay.ErrExpired)
	}

	got, err := coordinator.registerPreparedRecoveryEnvironment(context.Background(), recovery.ProjectID, recovery, registration)
	if !reflect.DeepEqual(got, completedRecoveryRegistration{}) {
		t.Fatalf("register sentinel returned completion %#v", got)
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	if remote.registerCalls != 1 {
		t.Fatalf("register sentinel calls = %d, want exactly one", remote.registerCalls)
	}
}

func TestRecoveryRegistrationWatermarkContainsOnlyPublicRelayIdentityAndFrontier(t *testing.T) {
	recovery := testBindableRecoveryCredential(t)
	got := recoveryRegistrationWatermark(recovery.ProjectID, recovery, 7, 42)
	want := testRecoveryRegistrationWatermark(recovery.ProjectID, recovery, 7, 42)
	if got != want {
		t.Fatalf("recovery registration watermark = %#v, want %#v", got, want)
	}
	typeOf := reflect.TypeOf(got)
	wantFields := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "ProjectID", typeOf: reflect.TypeOf(continuity.ProjectID(""))},
		{name: "ChannelID", typeOf: reflect.TypeOf(continuitysqlite.SyncChannelID{})},
		{name: "RelayGeneration", typeOf: reflect.TypeOf([32]byte{})},
		{name: "AdminPublicKey", typeOf: reflect.TypeOf([32]byte{})},
		{name: "MembershipGeneration", typeOf: reflect.TypeOf(uint32(0))},
		{name: "RelayHead", typeOf: reflect.TypeOf(int64(0))},
	}
	if typeOf.NumField() != len(wantFields) {
		t.Fatalf("recovery watermark fields = %d, want %d", typeOf.NumField(), len(wantFields))
	}
	for index, wantField := range wantFields {
		field := typeOf.Field(index)
		if field.Name != wantField.name || field.Type != wantField.typeOf {
			t.Fatalf("recovery watermark field %d = {%q %v}, want {%q %v}", index, field.Name, field.Type, wantField.name, wantField.typeOf)
		}
	}
}

func TestRecoveryRegistrationWatermarkErrorsMapStatically(t *testing.T) {
	const forbidden = "relay-watermark-sensitive-store-detail"
	for _, test := range []struct {
		name       string
		err        error
		wantCode   ProblemCode
		wantAction ProblemAction
	}{
		{name: "lower floor", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorCursor, Field: "relay_head", Detail: forbidden}, wantCode: CodeRemote, wantAction: ActionRestartRecovery},
		{name: "admin conflict", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorConflict, Field: "admin_public_key", Detail: forbidden}, wantCode: CodeConflict, wantAction: ActionRestartRecovery},
		{name: "store unavailable", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Detail: forbidden}, wantCode: CodeUnavailable, wantAction: ActionRetry},
		{name: "durable corruption", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorStore, Field: "relay_watermark", Detail: forbidden}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "invalid", err: &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorInvalid, Field: "relay_head", Detail: forbidden}, wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
		{name: "unexpected", err: errors.New(forbidden), wantCode: CodeInternal, wantAction: ActionRepairLocalStore},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := mapRecoveryRegistrationWatermarkError(context.Background(), test.err)
			assertProblem(t, err, test.wantCode, PhaseEnvironmentRegistration, test.wantAction)
			assertStaticRegistrationProblem(t, err, forbidden)
		})
	}
	joined := errors.Join(context.Canceled, &continuitysqlite.SyncError{Code: continuitysqlite.SyncErrorCursor, Detail: forbidden})
	if err := mapRecoveryRegistrationWatermarkError(context.Background(), joined); !errors.Is(err, context.Canceled) {
		t.Fatalf("joined watermark cancellation = %v, want context.Canceled", err)
	}
}

func testRecoveryRegistrationWatermark(
	projectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	membershipGeneration uint32,
	head int64,
) continuitysqlite.SyncRelayWatermark {
	return continuitysqlite.SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            continuitysqlite.SyncChannelID(recovery.ChannelID),
		RelayGeneration:      [32]byte(recovery.RelayGeneration),
		AdminPublicKey:       [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
		MembershipGeneration: membershipGeneration,
		RelayHead:            head,
	}
}

func regenerateRecoveryRegistrationCertificate(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) preparedRecoveryRegistration {
	t.Helper()
	certificate, err := protocol.ParseEnvironmentCertificate(registration.environment.CertificateBytes)
	if err != nil {
		t.Fatalf("parse registration certificate: %v", err)
	}
	seed, err := crypto.EnvironmentSeedFromBytes(testBytes(0x79, len(crypto.EnvironmentSeed{})))
	if err != nil {
		t.Fatalf("regenerated environment seed: %v", err)
	}
	certificate.EnvironmentPublicKey = crypto.EnvironmentPublicKey(seed)
	certificate, err = crypto.SignEnvironmentCertificate(certificate, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign regenerated certificate: %v", err)
	}
	certificateBytes, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal regenerated certificate: %v", err)
	}
	certificateID := relay.Digest(protocol.CertificateID(certificate))
	var tokenID relay.RelayTokenID
	copy(tokenID[:], testBytes(0x7a, len(tokenID)))
	tokenHash := relay.TokenHash(testArray32(0x7b))
	regenerated := registration
	regenerated.certificateID = certificateID
	regenerated.environmentTokenID = tokenID
	regenerated.environmentTokenHash = tokenHash
	regenerated.environment.CertificateID = certificateID
	regenerated.environment.CertificateBytes = append([]byte(nil), certificateBytes...)
	regenerated.environment.Token = relay.TokenRegistration{TokenID: tokenID, TokenHash: tokenHash}
	return regenerated
}

func assertSyncWatermarkError(t *testing.T, err error, code continuitysqlite.SyncErrorCode) {
	t.Helper()
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) || syncErr.Code != code {
		t.Fatalf("sync watermark error = %v, want %s", err, code)
	}
}

func assertStaticRegistrationProblem(t *testing.T, err error, forbidden string) {
	t.Helper()
	for _, formatted := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%#v", err)} {
		if strings.Contains(formatted, forbidden) {
			t.Fatalf("registration error leaked store detail: %q", formatted)
		}
	}
}

func registrationChannelState(recovery credential.ProjectRecoveryCredential, membership uint32, head int64) relay.ChannelState {
	return relay.ChannelState{
		ChannelID:            relay.ChannelID(recovery.ChannelID),
		RelayGeneration:      relay.RelayGeneration(recovery.RelayGeneration),
		MembershipGeneration: membership,
		Head:                 head,
	}
}

func assertExactRegistrationRequests(t *testing.T, requests []relay.RegisterEnvironmentRequest, recovery credential.ProjectRecoveryCredential, registration preparedRecoveryRegistration) {
	t.Helper()
	wantOwner := recoveryOwnerAuthorization(recovery)
	for index, request := range requests {
		if request.Authorization != wantOwner || !reflect.DeepEqual(request.Environment, registration.environment) {
			t.Fatalf("registration request %d = %#v, want exact call-local owner and prepared environment", index, request)
		}
		if len(request.Environment.CertificateBytes) == 0 || &request.Environment.CertificateBytes[0] == &registration.environment.CertificateBytes[0] {
			t.Fatalf("registration request %d certificate aliases prepared registration", index)
		}
	}
}

func assertNoRegistrationPostMutationCalls(t *testing.T, remote *remoteFixture) {
	t.Helper()
	if remote.pageCalls != 0 || remote.pruneCalls != 0 {
		t.Fatalf("registration reached data-plane/prune calls page=%d prune=%d", remote.pageCalls, remote.pruneCalls)
	}
}

func assertCompletedRegistrationSecretFreeSchema(t *testing.T, completed completedRecoveryRegistration) {
	t.Helper()
	typeOf := reflect.TypeOf(completed)
	want := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "guard", typeOf: reflect.TypeOf(recoveryRegistrationGuard{})},
		{name: "state", typeOf: reflect.TypeOf(relay.ChannelState{})},
	}
	if typeOf.NumField() != len(want) {
		t.Fatalf("completed registration fields = %d, want %d", typeOf.NumField(), len(want))
	}
	for index, fieldWant := range want {
		field := typeOf.Field(index)
		if field.Name != fieldWant.name || field.Type != fieldWant.typeOf {
			t.Fatalf("completed field %d = {%q %v}, want {%q %v}", index, field.Name, field.Type, fieldWant.name, fieldWant.typeOf)
		}
	}
	forbidden := map[reflect.Type]struct{}{
		reflect.TypeOf(relay.OwnerAuthorization{}):             {},
		reflect.TypeOf(relay.EnvironmentAuthorization{}):       {},
		reflect.TypeOf(relay.RegisterEnvironmentRequest{}):     {},
		reflect.TypeOf(relay.RelayTokenSecret{}):               {},
		reflect.TypeOf(relay.TokenHash{}):                      {},
		reflect.TypeOf(credential.RelayBearer{}):               {},
		reflect.TypeOf(credential.ProjectRecoveryCredential{}): {},
		reflect.TypeOf(credential.TrustedProjectCredential{}):  {},
	}
	visited := make(map[reflect.Type]struct{})
	var inspect func(string, reflect.Type)
	inspect = func(path string, current reflect.Type) {
		if _, found := forbidden[current]; found {
			t.Fatalf("completed registration retains protected authority at %s (%v)", path, current)
		}
		if _, found := visited[current]; found {
			return
		}
		visited[current] = struct{}{}
		switch current.Kind() {
		case reflect.Array, reflect.Pointer, reflect.Slice:
			inspect(path+"[]", current.Elem())
		case reflect.Map:
			inspect(path+"{key}", current.Key())
			inspect(path+"{value}", current.Elem())
		case reflect.Struct:
			for index := 0; index < current.NumField(); index++ {
				field := current.Field(index)
				inspect(path+"."+field.Name, field.Type)
			}
		}
	}
	inspect("completedRecoveryRegistration", typeOf)
}
