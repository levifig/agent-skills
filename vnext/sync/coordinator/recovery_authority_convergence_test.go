package coordinator

import (
	"bytes"
	"context"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestConvergeRegisteredRecoveryAuthorityPromotesGenerationOne(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	registration := bindGuardRegistration(t, mustCoordinator(t, store, seedRemote), recovery, writerID, 1)
	records := []relay.EnvironmentInventoryRecord{recoveryRegistrationInventoryRecord(registration)}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 1, ArrivalHead: 0}
	remote := exactRecoveryRegistrationInventoryRemote(recovery, registration, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("converge generation-one recovery authority: %v", err)
	}
	assertConvergedRecoveryAuthority(t, store, recovery, registration, binding, snapshot)
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 1)
}

func TestConvergeRegisteredRecoveryAuthorityPromotesLaterGuard(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, fixture.snapshot, fixture.records)
	coordinator := mustCoordinator(t, fixture.store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("converge later recovery authority: %v", err)
	}
	assertConvergedRecoveryAuthority(t, fixture.store, fixture.recovery, fixture.registration, binding, fixture.snapshot)
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 2)
}

func TestConvergeRegisteredRecoveryAuthorityResumesStagingSuffixAcrossReopen(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	interrupted := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, fixture.snapshot, fixture.records)
	pages := interrupted.environmentPages
	interrupted.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		if request.AfterEnvironmentID != "" {
			return relay.EnvironmentInventoryPage{}, relay.ErrClosed
		}
		return pages[request.AfterEnvironmentID], nil
	}
	coordinator := mustCoordinator(t, fixture.store, interrupted)
	_, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	assertProblem(t, err, CodeUnavailable, PhaseEnvironmentInventory, ActionRetry)
	active, found, err := fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.recovery.ProjectID)
	if err != nil || !found || active.Successor.Ready {
		t.Fatalf("interrupted recovery successor = (%#v, %v, %v), want STAGING", active, found, err)
	}
	wantCursor := active.Successor.ThroughEnvironmentID

	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close interrupted store: %v", err)
	}
	reopened := openCoordinatorStoreAt(t, fixture.root, fixture.writerID)
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, fixture.snapshot, fixture.records)
	coordinator = mustCoordinator(t, reopened, remote)
	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("resume staging recovery suffix: %v", err)
	}
	assertConvergedRecoveryAuthority(t, reopened, fixture.recovery, fixture.registration, binding, fixture.snapshot)
	if len(remote.environmentRequests) != 1 {
		t.Fatalf("resume inventory requests = %d, want one suffix request", len(remote.environmentRequests))
	}
	assertInventoryRequest(t, remote.environmentRequests[0], fixture.recovery, relay.EnvironmentID(wantCursor), &fixture.snapshot)
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 1)
}

func TestConvergeRegisteredRecoveryAuthorityPromotesReadyAtExactFrontierWithoutInventory(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	ready := seedRecoveryAuthoritySuccessor(t, fixture, 2)
	if !ready.Successor.Ready {
		t.Fatal("seeded recovery successor is not READY")
	}
	remote := exactRecoveryRegistrationInventoryRemote(
		fixture.recovery,
		fixture.registration,
		fixture.snapshot,
		nil,
	)
	coordinator := mustCoordinator(t, fixture.store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("promote ready recovery successor: %v", err)
	}
	assertConvergedRecoveryAuthority(t, fixture.store, fixture.recovery, fixture.registration, binding, fixture.snapshot)
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 0)
}

func TestConvergeRegisteredRecoveryAuthorityRecognizesPromotedRetryAcrossReopen(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	ready := seedRecoveryAuthoritySuccessor(t, fixture, 2)
	if _, err := fixture.store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), fixture.recovery.ProjectID, ready.Transition, ready.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("seed promoted recovery successor: %v", err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatalf("close promoted store: %v", err)
	}
	reopened := openCoordinatorStoreAt(t, fixture.root, fixture.writerID)
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, fixture.snapshot, nil)
	coordinator := mustCoordinator(t, reopened, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("recognize promoted recovery retry: %v", err)
	}
	assertConvergedRecoveryAuthority(t, reopened, fixture.recovery, fixture.registration, binding, fixture.snapshot)
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 1, 0)
}

func TestConvergeRegisteredRecoveryAuthorityReplacesSameHeadMembershipDriftInSameAttempt(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	staging := seedRecoveryAuthoritySuccessor(t, fixture, 1)
	if staging.Successor.Ready {
		t.Fatal("seeded recovery successor is READY")
	}
	records := append(append([]relay.EnvironmentInventoryRecord(nil), fixture.records...), testInventoryRecord(t, fixture.recovery, 201, 7))
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 7, ArrivalHead: fixture.snapshot.ArrivalHead}
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, snapshot, records)
	pages := remote.environmentPages
	var replacement continuitysqlite.SyncAuthorityRecoveryState
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		if request.AfterEnvironmentID != "" {
			var found bool
			var err error
			replacement, found, err = fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.recovery.ProjectID)
			if err != nil || !found {
				t.Fatalf("replacement recovery successor = (%#v, %v, %v), want active", replacement, found, err)
			}
		}
		return pages[request.AfterEnvironmentID], nil
	}
	coordinator := mustCoordinator(t, fixture.store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("replace same-head membership drift: %v", err)
	}
	assertConvergedRecoveryAuthority(t, fixture.store, fixture.recovery, fixture.registration, binding, snapshot)
	if replacement.Transition.AttemptID != staging.Transition.AttemptID ||
		replacement.Transition.PredecessorCandidateID != staging.Transition.PredecessorCandidateID ||
		replacement.Transition.SuccessorCandidateID == staging.Transition.SuccessorCandidateID {
		t.Fatalf("replacement transition = %#v, want stable attempt/predecessor and new successor from %#v", replacement.Transition, staging.Transition)
	}
	if len(remote.environmentRequests) == 0 || remote.environmentRequests[0].AfterEnvironmentID != "" || remote.environmentRequests[0].Snapshot != nil {
		t.Fatalf("same-head replacement first inventory request = %#v, want fresh unpinned scan", remote.environmentRequests)
	}
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 2)
}

func TestConvergeRegisteredRecoveryAuthorityReplacesHigherHeadSuccessor(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	staging := seedRecoveryAuthoritySuccessor(t, fixture, 1)
	if staging.Successor.Ready {
		t.Fatal("seeded recovery successor is READY")
	}
	snapshot := fixture.snapshot
	snapshot.ArrivalHead++
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, snapshot, fixture.records)
	coordinator := mustCoordinator(t, fixture.store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("replace higher-head recovery successor: %v", err)
	}
	assertConvergedRecoveryAuthority(t, fixture.store, fixture.recovery, fixture.registration, binding, snapshot)
	if len(remote.environmentRequests) == 0 || remote.environmentRequests[0].AfterEnvironmentID != "" || remote.environmentRequests[0].Snapshot != nil {
		t.Fatalf("replacement first inventory request = %#v, want fresh unpinned scan", remote.environmentRequests)
	}
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 2)
}

func TestConvergeRegisteredRecoveryAuthorityReplacesReadySuccessorAfterForwardObservation(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	ready := seedRecoveryAuthoritySuccessor(t, fixture, 2)
	if !ready.Successor.Ready {
		t.Fatal("seeded recovery successor is not READY")
	}
	records := append(append([]relay.EnvironmentInventoryRecord(nil), fixture.records...), testInventoryRecord(t, fixture.recovery, 201, 7))
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 7, ArrivalHead: fixture.snapshot.ArrivalHead}
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, snapshot, records)
	coordinator := mustCoordinator(t, fixture.store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("replace ready recovery successor after forward observation: %v", err)
	}
	assertConvergedRecoveryAuthority(t, fixture.store, fixture.recovery, fixture.registration, binding, snapshot)
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 2)
}

func TestConvergeRegisteredRecoveryAuthorityReclassifiesAfterScanBeforePromotion(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	initialSnapshot := fixture.snapshot
	advancedSnapshot := relay.EnvironmentInventorySnapshot{
		MembershipGeneration: initialSnapshot.MembershipGeneration + 1,
		ArrivalHead:          initialSnapshot.ArrivalHead + 1,
	}
	advancedRecords := append(
		append([]relay.EnvironmentInventoryRecord(nil), fixture.records...),
		testInventoryRecord(t, fixture.recovery, 201, advancedSnapshot.MembershipGeneration),
	)
	initialRemote := inventoryRemote(fixture.recovery, initialSnapshot, fixture.records)
	advancedRemote := inventoryRemote(fixture.recovery, advancedSnapshot, advancedRecords)
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, initialSnapshot, fixture.records)
	var firstAttempt continuitysqlite.SyncAuthorityRecoveryTransition
	var replacementAttempt continuitysqlite.SyncAuthorityRecoveryTransition
	remote.classify = func(context.Context, relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		snapshot := initialSnapshot
		if remote.classifyCalls >= 2 {
			snapshot = advancedSnapshot
		}
		if remote.classifyCalls == 2 {
			active, found, err := fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.recovery.ProjectID)
			if err != nil || !found || !active.Successor.Ready {
				t.Fatalf("post-scan successor = (%#v, %v, %v), want READY", active, found, err)
			}
			firstAttempt = active.Transition
		}
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationExact,
			State:       registrationChannelState(fixture.recovery, snapshot.MembershipGeneration, snapshot.ArrivalHead),
		}, nil
	}
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		pages := initialRemote.environmentPages
		if request.Snapshot != nil && *request.Snapshot == advancedSnapshot ||
			request.Snapshot == nil && remote.classifyCalls >= 3 {
			pages = advancedRemote.environmentPages
		}
		if request.AfterEnvironmentID != "" && request.Snapshot != nil && *request.Snapshot == advancedSnapshot {
			active, found, err := fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.recovery.ProjectID)
			if err != nil || !found {
				t.Fatalf("replacement successor = (%#v, %v, %v), want active", active, found, err)
			}
			replacementAttempt = active.Transition
		}
		return pages[request.AfterEnvironmentID], nil
	}
	coordinator := mustCoordinator(t, fixture.store, remote)

	binding, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	if err != nil {
		t.Fatalf("reclassify after inventory scan: %v", err)
	}
	assertConvergedRecoveryAuthority(t, fixture.store, fixture.recovery, fixture.registration, binding, advancedSnapshot)
	if firstAttempt.AttemptID == ([32]byte{}) || replacementAttempt.AttemptID != firstAttempt.AttemptID ||
		replacementAttempt.PredecessorCandidateID != firstAttempt.PredecessorCandidateID ||
		replacementAttempt.SuccessorCandidateID == firstAttempt.SuccessorCandidateID {
		t.Fatalf("replacement attempt = %#v, want same attempt/predecessor and new successor from %#v", replacementAttempt, firstAttempt)
	}
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 4, 4)
}

func TestConvergeRegisteredRecoveryAuthorityRejectsCrossedPostScanObservation(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, fixture.snapshot, fixture.records)
	remote.classify = func(context.Context, relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		snapshot := fixture.snapshot
		if remote.classifyCalls == 2 {
			snapshot.MembershipGeneration++
			snapshot.ArrivalHead--
		}
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationExact,
			State:       registrationChannelState(fixture.recovery, snapshot.MembershipGeneration, snapshot.ArrivalHead),
		}, nil
	}
	coordinator := mustCoordinator(t, fixture.store, remote)

	_, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	assertProblem(t, err, CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	active, found, activeErr := fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.recovery.ProjectID)
	if activeErr != nil || !found || !active.Successor.Ready || active.Successor.Snapshot.MembershipGeneration != fixture.snapshot.MembershipGeneration ||
		active.Successor.Snapshot.InventoryArrivalHead != fixture.snapshot.ArrivalHead {
		t.Fatalf("successor after crossed observation = (%#v, %v, %v), want retained READY scan", active, found, activeErr)
	}
	if _, bindingErr := fixture.store.CurrentSyncAuthorityBinding(context.Background(), fixture.recovery.ProjectID); !recoveryAuthorityStoreErrorCode(bindingErr, continuitysqlite.SyncErrorConflict) {
		t.Fatalf("canonical binding after crossed observation error = %v, want active-transition conflict", bindingErr)
	}
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 2, 2)
}

func TestConvergeRegisteredRecoveryAuthorityRejectsCanonicalRetryBehindObservedFrontier(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	ready := seedRecoveryAuthoritySuccessor(t, fixture, 2)
	if _, err := fixture.store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), fixture.recovery.ProjectID, ready.Transition, ready.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("seed promoted recovery successor: %v", err)
	}
	observed := fixture.snapshot
	observed.MembershipGeneration++
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, observed, nil)
	coordinator := mustCoordinator(t, fixture.store, remote)

	_, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	binding, bindingErr := fixture.store.CurrentSyncAuthorityBinding(context.Background(), fixture.recovery.ProjectID)
	if bindingErr != nil || binding.MembershipGeneration != fixture.snapshot.MembershipGeneration ||
		binding.InventoryArrivalHead != fixture.snapshot.ArrivalHead {
		t.Fatalf("canonical binding after rejected retry = (%#v, %v), want unchanged %#v", binding, bindingErr, fixture.snapshot)
	}
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 1, 0)
}

func TestConvergeRegisteredRecoveryAuthorityBoundsRepeatedPrePromotionAdvance(t *testing.T) {
	fixture := newLaterRecoveryConvergenceFixture(t)
	ready := seedRecoveryAuthoritySuccessor(t, fixture, 2)
	firstAdvance := fixture.snapshot
	firstAdvance.MembershipGeneration++
	secondAdvance := firstAdvance
	secondAdvance.MembershipGeneration++
	records := append(
		append([]relay.EnvironmentInventoryRecord(nil), fixture.records...),
		testInventoryRecord(t, fixture.recovery, 201, firstAdvance.MembershipGeneration),
	)
	remote := exactRecoveryRegistrationInventoryRemote(fixture.recovery, fixture.registration, firstAdvance, records)
	remote.classify = func(context.Context, relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		snapshot := fixture.snapshot
		switch remote.classifyCalls {
		case 2, 3:
			snapshot = firstAdvance
		case 4:
			snapshot = secondAdvance
		}
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationExact,
			State:       registrationChannelState(fixture.recovery, snapshot.MembershipGeneration, snapshot.ArrivalHead),
		}, nil
	}
	coordinator := mustCoordinator(t, fixture.store, remote)

	_, err := coordinator.convergeRegisteredRecoveryAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.recovery, fixture.registration,
	)
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRetry)
	active, found, activeErr := fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.recovery.ProjectID)
	if activeErr != nil || !found || !active.Successor.Ready ||
		active.Transition.AttemptID != ready.Transition.AttemptID ||
		active.Transition.PredecessorCandidateID != ready.Transition.PredecessorCandidateID ||
		active.Transition.SuccessorCandidateID == ready.Transition.SuccessorCandidateID ||
		active.Successor.Snapshot.MembershipGeneration != firstAdvance.MembershipGeneration ||
		active.Successor.Snapshot.InventoryArrivalHead != firstAdvance.ArrivalHead {
		t.Fatalf("successor after bounded advances = (%#v, %v, %v), want safe READY replacement in original attempt", active, found, activeErr)
	}
	if _, bindingErr := fixture.store.CurrentSyncAuthorityBinding(context.Background(), fixture.recovery.ProjectID); !recoveryAuthorityStoreErrorCode(bindingErr, continuitysqlite.SyncErrorConflict) {
		t.Fatalf("canonical binding after bounded advances error = %v, want active-transition conflict", bindingErr)
	}
	assertRecoveryAuthorityOnlyRemoteCalls(t, remote, 4, 2)
}

type laterRecoveryConvergenceFixture struct {
	root         string
	writerID     continuity.EnvironmentID
	store        *continuitysqlite.Store
	recovery     credential.ProjectRecoveryCredential
	registration preparedRecoveryRegistration
	guard        recoveryRegistrationGuard
	snapshot     relay.EnvironmentInventorySnapshot
	records      []relay.EnvironmentInventoryRecord
}

func newLaterRecoveryConvergenceFixture(t *testing.T) laterRecoveryConvergenceFixture {
	t.Helper()
	root := t.TempDir()
	writerID := testEnvironmentID(200)
	store := openCoordinatorStoreAt(t, root, writerID)
	recovery := testBindableRecoveryCredential(t)
	guardSnapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	guardRecords := testGuardInventoryRecords(t, recovery)
	guardRemote := inventoryRemote(recovery, guardSnapshot, guardRecords)
	coordinator := mustCoordinator(t, store, guardRemote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	guard, err := coordinator.stageRecoveryRegistrationGuard(context.Background(), recovery.ProjectID, recovery, registration)
	if err != nil {
		t.Fatalf("stage later recovery guard: %v", err)
	}
	records := append(append([]relay.EnvironmentInventoryRecord(nil), guardRecords...), recoveryRegistrationInventoryRecord(registration))
	return laterRecoveryConvergenceFixture{
		root:         root,
		writerID:     writerID,
		store:        store,
		recovery:     recovery,
		registration: registration,
		guard:        guard,
		snapshot:     relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: 20},
		records:      records,
	}
}

func exactRecoveryRegistrationInventoryRemote(
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	snapshot relay.EnvironmentInventorySnapshot,
	records []relay.EnvironmentInventoryRecord,
) *remoteFixture {
	remote := inventoryRemote(recovery, snapshot, records)
	remote.classify = func(context.Context, relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
		return relay.EnvironmentRegistrationStatus{
			Disposition: relay.EnvironmentRegistrationExact,
			State:       registrationChannelState(recovery, snapshot.MembershipGeneration, snapshot.ArrivalHead),
		}, nil
	}
	return remote
}

func seedRecoveryAuthoritySuccessor(
	t *testing.T,
	fixture laterRecoveryConvergenceFixture,
	pageCount int,
) continuitysqlite.SyncAuthorityRecoveryState {
	t.Helper()
	pages := recoveryAuthorityPages(t, fixture.records)
	if pageCount < 1 || pageCount > len(pages) {
		t.Fatalf("seed page count = %d, want 1..%d", pageCount, len(pages))
	}
	if fixture.guard.candidate == nil {
		t.Fatal("later recovery fixture has no predecessor candidate")
	}
	snapshot := recoveryAuthoritySnapshot(fixture.recovery, fixture.snapshot, fixture.guard.candidate)
	start := continuitysqlite.SyncAuthorityRecoveryTransitionStart{
		WriterEnvironmentID:        fixture.writerID,
		WriterCertificateID:        [32]byte(fixture.registration.certificateID),
		TargetMembershipGeneration: fixture.registration.targetMembershipGeneration,
		PredecessorCheckpoint:      fixture.guard.candidate.Checkpoint(),
		SuccessorSnapshot:          snapshot,
	}
	state, err := fixture.store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), fixture.recovery.ProjectID, start, pages[0],
	)
	if err != nil {
		t.Fatalf("begin seeded recovery successor: %v", err)
	}
	for index := 1; index < pageCount; index++ {
		state, err = fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
			context.Background(), fixture.recovery.ProjectID, state.Transition, state.Successor.Checkpoint(), snapshot, pages[index],
		)
		if err != nil {
			t.Fatalf("append seeded recovery successor page %d: %v", index, err)
		}
	}
	return state
}

func recoveryAuthoritySnapshot(
	recovery credential.ProjectRecoveryCredential,
	snapshot relay.EnvironmentInventorySnapshot,
	predecessor *continuitysqlite.SyncAuthorityCandidate,
) continuitysqlite.SyncAuthoritySnapshot {
	result := continuitysqlite.SyncAuthoritySnapshot{
		ChannelID:            continuitysqlite.SyncChannelID(recovery.ChannelID),
		RelayGeneration:      [32]byte(recovery.RelayGeneration),
		AdminPublicKey:       [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
		MembershipGeneration: snapshot.MembershipGeneration,
		InventoryArrivalHead: snapshot.ArrivalHead,
	}
	if predecessor != nil {
		result.BaseAuthorityDigestVersion = predecessor.AuthorityDigestVersion
		result.BaseAuthorityDigest = predecessor.AuthorityDigest
	}
	return result
}

func recoveryAuthorityPages(t *testing.T, records []relay.EnvironmentInventoryRecord) []continuitysqlite.SyncAuthorityPage {
	t.Helper()
	pages := make([]continuitysqlite.SyncAuthorityPage, 0, (len(records)+relay.MaxEnvironmentInventoryPage-1)/relay.MaxEnvironmentInventoryPage)
	after := ""
	for start := 0; start < len(records); start += relay.MaxEnvironmentInventoryPage {
		end := start + relay.MaxEnvironmentInventoryPage
		if end > len(records) {
			end = len(records)
		}
		environments := make([]continuitysqlite.SyncEnvironmentCertificate, 0, end-start)
		for _, record := range records[start:end] {
			environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
			if err != nil {
				t.Fatalf("translate recovery inventory record: %v", err)
			}
			environments = append(environments, environment)
		}
		through := string(records[end-1].EnvironmentID)
		pages = append(pages, continuitysqlite.SyncAuthorityPage{
			AfterEnvironmentID:   after,
			ThroughEnvironmentID: through,
			Environments:         environments,
			More:                 end < len(records),
		})
		after = through
	}
	return pages
}

func assertConvergedRecoveryAuthority(
	t *testing.T,
	store *continuitysqlite.Store,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	binding continuitysqlite.SyncAuthorityBinding,
	snapshot relay.EnvironmentInventorySnapshot,
) {
	t.Helper()
	if binding.ChannelID != continuitysqlite.SyncChannelID(recovery.ChannelID) ||
		binding.RelayGeneration != [32]byte(recovery.RelayGeneration) ||
		binding.AdminPublicKey != [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) ||
		binding.MembershipGeneration != snapshot.MembershipGeneration ||
		binding.InventoryArrivalHead != snapshot.ArrivalHead || binding.AuthorityDigestVersion != 2 {
		t.Fatalf("converged authority binding = %#v, want recovery snapshot %#v", binding, snapshot)
	}
	states, err := store.CurrentSyncEnvironmentStates(
		context.Background(), recovery.ProjectID, binding, []continuity.EnvironmentID{continuity.EnvironmentID(registration.environment.EnvironmentID)},
	)
	if err != nil || len(states) != 1 {
		t.Fatalf("converged writer state = (%#v, %v), want one exact writer", states, err)
	}
	certificate := states[0].Certificate
	if certificate.EnvironmentID != string(registration.environment.EnvironmentID) ||
		certificate.CertificateID != [32]byte(registration.certificateID) ||
		!bytes.Equal(certificate.CertificateBytes, registration.environment.CertificateBytes) ||
		certificate.Mode != continuitysqlite.SyncEnvironmentTrusted || certificate.ExpiresAtMillis != 0 ||
		certificate.JoinMembershipGeneration != registration.targetMembershipGeneration || certificate.Retirement != nil {
		t.Fatalf("converged writer certificate = %#v, want exact protected registration", certificate)
	}
	if candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), recovery.ProjectID); err != nil || found {
		t.Fatalf("ordinary candidate after convergence = (%#v, %v, %v), want absent", candidate, found, err)
	}
	if active, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), recovery.ProjectID); err != nil || found {
		t.Fatalf("recovery successor after convergence = (%#v, %v, %v), want absent", active, found, err)
	}
}

func assertRecoveryAuthorityOnlyRemoteCalls(t *testing.T, remote *remoteFixture, classifyCalls, inventoryCalls int) {
	t.Helper()
	if remote.classifyCalls != classifyCalls || len(remote.environmentRequests) != inventoryCalls ||
		remote.createCalls != 0 || remote.registerCalls != 0 || remote.pageCalls != 0 || remote.pruneCalls != 0 {
		t.Fatalf(
			"recovery authority remote calls = {classify=%d inventory=%d create=%d register=%d page=%d prune=%d}, want {%d %d 0 0 0 0}",
			remote.classifyCalls, len(remote.environmentRequests), remote.createCalls, remote.registerCalls, remote.pageCalls, remote.pruneCalls,
			classifyCalls, inventoryCalls,
		)
	}
}
