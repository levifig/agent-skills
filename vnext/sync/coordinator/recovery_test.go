package coordinator

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestPrepareRecoveryReadsPopulatedInventoryAndReturnsFreshCredential(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testRecoveryCredential(t)
	records := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator := mustCoordinator(t, store, remote)

	first, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if err != nil {
		t.Fatalf("prepare recovery: %v", err)
	}
	assertPreparedCredential(t, first, recovery, writerID, 6)
	if len(remote.environmentRequests) != 2 {
		t.Fatalf("environment inventory calls = %d, want 2", len(remote.environmentRequests))
	}
	assertInventoryRequest(t, remote.environmentRequests[0], recovery, "", nil)
	lastFirstPage := records[3].EnvironmentID
	assertInventoryRequest(t, remote.environmentRequests[1], recovery, lastFirstPage, &snapshot)
	if remote.createCalls != 0 {
		t.Fatalf("CreateChannel calls = %d, want 0", remote.createCalls)
	}
	assertNoMutationCalls(t, remote)

	if len(first.LastObservedCheckpoint) != 0 {
		t.Fatal("prepared credential confused recovery checkpoint with an observed relay checkpoint")
	}
	recovery.LastSignedCheckpoint[0] ^= 0xff
	if len(first.LastObservedCheckpoint) != 0 {
		t.Fatal("returned observed checkpoint changed with recovery credential mutation")
	}
	recovery.LastSignedCheckpoint[0] ^= 0xff

	second, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if err != nil {
		t.Fatalf("prepare recovery again: %v", err)
	}
	if first.EnvironmentSeed == second.EnvironmentSeed {
		t.Fatal("recovery preparation reused an environment seed")
	}
	if first.EnvironmentRelayAuthorization == second.EnvironmentRelayAuthorization {
		t.Fatal("recovery preparation reused a relay bearer")
	}
	assertNoMutationCalls(t, remote)
}

func TestPrepareRecoveryCreatesEmptyChannelWithExactIdempotentInput(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)

	for i := 0; i < 2; i++ {
		prepared, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
		if err != nil {
			t.Fatalf("prepare recovery call %d: %v", i+1, err)
		}
		assertPreparedCredential(t, prepared, recovery, writerID, 1)
	}
	if remote.createCalls != 2 || len(remote.createRequests) != 2 {
		t.Fatalf("CreateChannel calls = %d, want 2", remote.createCalls)
	}
	adminPublic := crypto.AdminPublicKey(recovery.AdminSeed)
	ownerSecret := recovery.OwnerRelayAuthorization.Secret()
	ownerHash, err := relay.HashTokenSecret(relay.RelayTokenSecret(ownerSecret))
	if err != nil {
		t.Fatalf("hash owner secret: %v", err)
	}
	want := relay.Channel{
		ChannelID:       relay.ChannelID(recovery.ChannelID),
		RelayGeneration: relay.RelayGeneration(recovery.RelayGeneration),
		AdminPublicKey:  relay.PublicKey(adminPublic),
		OwnerToken: relay.TokenRegistration{
			TokenID:   relay.RelayTokenID(recovery.OwnerRelayAuthorization.ID()),
			TokenHash: ownerHash,
		},
	}
	for i, request := range remote.createRequests {
		if request != want {
			t.Fatalf("CreateChannel request %d did not match the recovery authority", i+1)
		}
	}
	assertNoMutationCalls(t, remote)
}

func TestPrepareRecoveryRequiresExplicitAuthorizationForEmptyChannel(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)

	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
		t.Fatal("unauthorized empty-channel preparation returned credential material")
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionAuthorizeEmptyChannel)
	if remote.createCalls != 0 {
		t.Fatalf("CreateChannel calls = %d, want 0", remote.createCalls)
	}
	assertNoMutationCalls(t, remote)
}

func TestPrepareRecoveryValidatesLocalInputsBeforeRemoteMutation(t *testing.T) {
	validRecovery := testRecoveryCredential(t)
	validStore := openCoordinatorStore(t, testEnvironmentID(200))

	tests := []struct {
		name       string
		store      *continuitysqlite.Store
		expected   continuity.ProjectID
		recovery   credential.ProjectRecoveryCredential
		endpoint   string
		wantAction ProblemAction
	}{
		{
			name:       "invalid expected project",
			store:      validStore,
			recovery:   validRecovery,
			endpoint:   validRecovery.RelayURL,
			wantAction: ActionCorrectInput,
		},
		{
			name:       "invalid recovery",
			store:      validStore,
			expected:   validRecovery.ProjectID,
			endpoint:   validRecovery.RelayURL,
			wantAction: ActionCorrectInput,
		},
		{
			name:       "project mismatch",
			store:      validStore,
			expected:   testProjectID(44),
			recovery:   validRecovery,
			endpoint:   validRecovery.RelayURL,
			wantAction: ActionRestartRecovery,
		},
		{
			name:       "endpoint mismatch",
			store:      validStore,
			expected:   validRecovery.ProjectID,
			recovery:   validRecovery,
			endpoint:   "https://other-relay.example.test",
			wantAction: ActionRestartRecovery,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := emptyInventoryRemote(validRecovery)
			remote.endpoint = test.endpoint
			coordinator := mustCoordinator(t, test.store, remote)
			got, err := coordinator.PrepareRecovery(context.Background(), test.expected, test.recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
			if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
				t.Fatal("invalid preparation returned credential material")
			}
			assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, test.wantAction)
			if remote.createCalls != 0 || len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
				t.Fatal("invalid local input reached a mutating or inventory remote call")
			}
		})
	}
}

func TestPrepareRecoveryDefensivelyRejectsInvalidWriterBeforeRemoteCalls(t *testing.T) {
	recovery := testRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := &Coordinator{store: &continuitysqlite.Store{}, remote: remote}

	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
	if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
		t.Fatal("invalid writer returned credential material")
	}
	assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	if remote.endpointCalls != 0 || remote.createCalls != 0 || len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
		t.Fatal("invalid writer reached remote")
	}
}

func TestPrepareRecoveryRejectsRegisteredWriter(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testRecoveryCredential(t)
	record := testInventoryRecord(t, recovery, 200, 1)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, []relay.EnvironmentInventoryRecord{record})
	coordinator := mustCoordinator(t, store, remote)

	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
		t.Fatal("registered writer returned credential material")
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionUseExistingCredential)
	assertNoMutationCalls(t, remote)
}

func TestPrepareRecoveryRejectsRetiredRegisteredWriter(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testRecoveryCredential(t)
	record := testRetiredInventoryRecord(t, recovery, 200, 1, 2, 1)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 2, ArrivalHead: 1}, []relay.EnvironmentInventoryRecord{record})
	coordinator := mustCoordinator(t, store, remote)

	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
		t.Fatal("retired registered writer returned credential material")
	}
	assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionUseExistingCredential)
	assertNoMutationCalls(t, remote)
}

func TestPrepareRecoveryVerifiesCanonicalCertificateAndProjectBinding(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)

	tests := []struct {
		name   string
		record func(*testing.T) relay.EnvironmentInventoryRecord
	}{
		{
			name: "noncanonical certificate bytes",
			record: func(t *testing.T) relay.EnvironmentInventoryRecord {
				record := testInventoryRecord(t, recovery, 1, 1)
				record.CertificateBytes = append(record.CertificateBytes, 0)
				return record
			},
		},
		{
			name: "invalid administrator signature",
			record: func(t *testing.T) relay.EnvironmentInventoryRecord {
				record := testInventoryRecord(t, recovery, 1, 1)
				record.CertificateBytes[len(record.CertificateBytes)-1] ^= 0x80
				certificate, err := protocol.ParseEnvironmentCertificate(record.CertificateBytes)
				if err != nil {
					t.Fatalf("parse signature-mutated certificate: %v", err)
				}
				record.CertificateID = relay.Digest(protocol.CertificateID(certificate))
				return record
			},
		},
		{
			name: "project mismatch",
			record: func(t *testing.T) relay.EnvironmentInventoryRecord {
				otherProject := recovery
				otherProject.ProjectID = testProjectID(99)
				return testInventoryRecord(t, otherProject, 1, 1)
			},
		},
		{
			name: "outer certificate binding",
			record: func(t *testing.T) relay.EnvironmentInventoryRecord {
				record := testInventoryRecord(t, recovery, 1, 1)
				record.CertificateID[0] ^= 0xff
				return record
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := test.record(t)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 1}, []relay.EnvironmentInventoryRecord{record})
			coordinator := mustCoordinator(t, store, remote)
			got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
			if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
				t.Fatal("invalid certificate returned credential material")
			}
			assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestPrepareRecoveryVerifiesTerminalRetirementAndIgnoresRelayTimestamp(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	valid := testRetiredInventoryRecord(t, recovery, 1, 1, 2, 2)
	if !valid.Retirement.RetiredAt.IsZero() {
		t.Fatal("fixture must exercise an absent relay timestamp")
	}

	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 2, ArrivalHead: 2}, []relay.EnvironmentInventoryRecord{valid})
	coordinator := mustCoordinator(t, store, remote)
	prepared, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if err != nil {
		t.Fatalf("prepare with verified retirement: %v", err)
	}
	assertPreparedCredential(t, prepared, recovery, testEnvironmentID(200), 3)

	for _, test := range []struct {
		name   string
		mutate func(*relay.EnvironmentInventoryRecord)
	}{
		{
			name: "producer head differs from final fence",
			mutate: func(record *relay.EnvironmentInventoryRecord) {
				record.ProducerHead--
			},
		},
		{
			name: "noncanonical retirement bytes",
			mutate: func(record *relay.EnvironmentInventoryRecord) {
				record.Retirement.RetirementBytes = append(record.Retirement.RetirementBytes, 0)
			},
		},
		{
			name: "retirement relay generation drift",
			mutate: func(record *relay.EnvironmentInventoryRecord) {
				record.Retirement.RelayGeneration[0] ^= 0xff
			},
		},
		{
			name: "retirement certificate binding drift",
			mutate: func(record *relay.EnvironmentInventoryRecord) {
				record.Retirement.CertificateID[0] ^= 0xff
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := cloneInventoryRecord(valid)
			test.mutate(&record)
			remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 2, ArrivalHead: 2}, []relay.EnvironmentInventoryRecord{record})
			coordinator := mustCoordinator(t, store, remote)
			got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
			if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
				t.Fatal("invalid retirement returned credential material")
			}
			assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestPrepareRecoveryRejectsMalformedOrDriftingInventory(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	validSnapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}
	validRecords := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}

	tests := []struct {
		name   string
		mutate func(*remoteFixture)
	}{
		{
			name: "channel mismatch",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Channel.ChannelID[0] ^= 0xff
				remote.environmentPages[""] = page
			},
		},
		{
			name: "relay generation mismatch",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Channel.RelayGeneration[0] ^= 0xff
				remote.environmentPages[""] = page
			},
		},
		{
			name: "admin mismatch",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Channel.AdminPublicKey[0] ^= 0xff
				remote.environmentPages[""] = page
			},
		},
		{
			name: "snapshot drift",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[validRecords[3].EnvironmentID]
				page.Snapshot.ArrivalHead++
				remote.environmentPages[validRecords[3].EnvironmentID] = page
			},
		},
		{
			name: "later empty page",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[validRecords[3].EnvironmentID]
				page.Environments = nil
				page.More = false
				remote.environmentPages[validRecords[3].EnvironmentID] = page
			},
		},
		{
			name: "reordered page",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments[1], page.Environments[2] = page.Environments[2], page.Environments[1]
				remote.environmentPages[""] = page
			},
		},
		{
			name: "duplicate across pages",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[validRecords[3].EnvironmentID]
				page.Environments[0].EnvironmentID = validRecords[3].EnvironmentID
				remote.environmentPages[validRecords[3].EnvironmentID] = page
			},
		},
		{
			name: "oversize page",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments = append(page.Environments, testInventoryRecord(t, recovery, 9, 9))
				remote.environmentPages[""] = page
			},
		},
		{
			name: "empty page claims more",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments = nil
				page.More = true
				remote.environmentPages[""] = page
			},
		},
		{
			name: "short page claims more",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments = page.Environments[:1]
				page.More = true
				remote.environmentPages[""] = page
			},
		},
		{
			name: "nonempty zero membership",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Snapshot.MembershipGeneration = 0
				remote.environmentPages[""] = page
			},
		},
		{
			name: "empty nonzero membership",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments = nil
				page.More = false
				remote.environmentPages[""] = page
			},
		},
		{
			name: "membership generation overflow",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Snapshot.MembershipGeneration = math.MaxUint32
				page.More = false
				remote.environmentPages[""] = page
			},
		},
		{
			name: "record membership beyond snapshot",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments[0].MembershipGeneration = page.Snapshot.MembershipGeneration + 1
				remote.environmentPages[""] = page
			},
		},
		{
			name: "negative producer head",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments[0].ProducerHead = -1
				remote.environmentPages[""] = page
			},
		},
		{
			name: "producer head beyond snapshot arrival",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments[0].ProducerHead = page.Snapshot.ArrivalHead + 1
				remote.environmentPages[""] = page
			},
		},
		{
			name: "duplicate record membership generation",
			mutate: func(remote *remoteFixture) {
				page := remote.environmentPages[""]
				page.Environments[1].MembershipGeneration = page.Environments[0].MembershipGeneration
				remote.environmentPages[""] = page
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := inventoryRemote(recovery, validSnapshot, validRecords)
			test.mutate(remote)
			coordinator := mustCoordinator(t, store, remote)
			got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
			if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
				t.Fatal("malformed inventory returned credential material")
			}
			assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestPrepareRecoveryRequiresMatchingMembershipEventCount(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	records := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 6}, records)
	coordinator := mustCoordinator(t, store, remote)
	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
		t.Fatal("incomplete membership history returned credential material")
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
	if len(remote.environmentRequests) != 2 {
		t.Fatalf("inventory calls = %d, want complete two-page read before count refusal", len(remote.environmentRequests))
	}
}

func TestPrepareRecoveryRejectsMembershipOverflowOnFirstPage(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	records := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: math.MaxUint32}, records)
	coordinator := mustCoordinator(t, store, remote)
	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
		t.Fatal("overflowed membership returned credential material")
	}
	assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
	if len(remote.environmentRequests) != 1 {
		t.Fatalf("inventory calls = %d, want immediate first-page refusal", len(remote.environmentRequests))
	}
}

func TestPrepareRecoveryUsesFreshAuthorizationAndFixedSnapshotRequests(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	records := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 9}
	remote := inventoryRemote(recovery, snapshot, records)
	pages := remote.environmentPages
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		page := pages[request.AfterEnvironmentID]
		if request.Authorization.Owner != nil {
			request.Authorization.Owner.TokenID = relay.RelayTokenID{}
			request.Authorization.Owner.TokenSecret = relay.RelayTokenSecret{}
		}
		if request.Snapshot != nil {
			request.Snapshot.ArrivalHead++
		}
		return page, nil
	}
	coordinator := mustCoordinator(t, store, remote)
	if _, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{}); err != nil {
		t.Fatalf("prepare recovery: %v", err)
	}
	if len(remote.environmentRequests) != 2 {
		t.Fatalf("inventory calls = %d, want 2", len(remote.environmentRequests))
	}
	assertInventoryRequest(t, remote.environmentRequests[0], recovery, "", nil)
	assertInventoryRequest(t, remote.environmentRequests[1], recovery, records[3].EnvironmentID, &snapshot)
}

func TestPrepareRecoveryStreamsEnvironmentInventoryWithoutLifetimeCap(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(900))
	recovery := testRecoveryCredential(t)
	records := make([]relay.EnvironmentInventoryRecord, 257)
	for i := range records {
		records[i] = testInventoryRecord(t, recovery, uint16(i+1), uint32(i+1))
	}
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 257}, records)
	coordinator := mustCoordinator(t, store, remote)

	got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
	if err != nil {
		t.Fatalf("prepare recovery across large inventory: %v", err)
	}
	assertPreparedCredential(t, got, recovery, testEnvironmentID(900), 258)
	if len(remote.environmentRequests) != 65 {
		t.Fatalf("inventory calls = %d, want 65", len(remote.environmentRequests))
	}
	assertNoMutationCalls(t, remote)
}

func TestPrepareRecoveryPreservesContextErrors(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	remote := emptyInventoryRemote(recovery)
	coordinator := mustCoordinator(t, store, remote)
	if _, err := coordinator.PrepareRecovery(nil, recovery.ProjectID, recovery, RecoveryPreparationOptions{}); err == nil {
		t.Fatal("nil context was accepted")
	} else {
		assertProblem(t, err, CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if remote.createCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatal("nil-context preparation called remote")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	remote = emptyInventoryRemote(recovery)
	coordinator = mustCoordinator(t, store, remote)
	if _, err := coordinator.PrepareRecovery(canceled, recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true}); err != context.Canceled {
		t.Fatalf("pre-canceled error = %v, want context.Canceled", err)
	}
	if remote.createCalls != 0 || len(remote.environmentRequests) != 0 {
		t.Fatal("pre-canceled preparation called remote")
	}

	remote = emptyInventoryRemote(recovery)
	remote.inventoryErr = context.DeadlineExceeded
	coordinator = mustCoordinator(t, store, remote)
	if _, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{}); err != context.DeadlineExceeded {
		t.Fatalf("remote deadline error = %v, want context.DeadlineExceeded", err)
	}

	remote = emptyInventoryRemote(recovery)
	ctx, cancelAfterRead := context.WithCancel(context.Background())
	pages := remote.environmentPages
	remote.inventory = func(_ context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
		cancelAfterRead()
		return pages[request.AfterEnvironmentID], nil
	}
	coordinator = mustCoordinator(t, store, remote)
	if _, err := coordinator.PrepareRecovery(ctx, recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true}); err != context.Canceled {
		t.Fatalf("post-read cancellation error = %v, want context.Canceled", err)
	}
}

func TestPrepareRecoveryMapsRelayErrorsWithoutLeakingSecrets(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	secretMarker := "relay-owner-secret-marker"

	tests := []struct {
		name       string
		remoteErr  error
		wantCode   ProblemCode
		wantPhase  ProblemPhase
		wantAction ProblemAction
	}{
		{name: "invalid", remoteErr: relay.ErrInvalidArgument, wantCode: CodeRemote, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRestartRecovery},
		{name: "unauthenticated", remoteErr: relay.ErrUnauthenticated, wantCode: CodeAuthorization, wantPhase: PhaseChannelAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "missing", remoteErr: relay.ErrNotFound, wantCode: CodeAuthorization, wantPhase: PhaseChannelAuthorization, wantAction: ActionCheckRecoveryAuthority},
		{name: "immutable conflict", remoteErr: relay.ErrImmutableConflict, wantCode: CodeRemote, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRestartRecovery},
		{name: "membership drift", remoteErr: relay.ErrMembershipChanged, wantCode: CodeConflict, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRetry},
		{name: "generation mismatch", remoteErr: relay.ErrGenerationMismatch, wantCode: CodeConflict, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRestartRecovery},
		{name: "rollback", remoteErr: relay.ErrRollback, wantCode: CodeConflict, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRestartRecovery},
		{name: "closed", remoteErr: relay.ErrClosed, wantCode: CodeUnavailable, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRetry},
		{name: "unknown", remoteErr: errors.New(secretMarker), wantCode: CodeUnavailable, wantPhase: PhaseEnvironmentInventory, wantAction: ActionRetry},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			remote := emptyInventoryRemote(recovery)
			remote.inventoryErr = fmt.Errorf("%s: %w", secretMarker, test.remoteErr)
			coordinator := mustCoordinator(t, store, remote)
			_, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
			assertProblem(t, err, test.wantCode, test.wantPhase, test.wantAction)
			for _, formatted := range []string{err.Error(), fmt.Sprintf("%v", err), fmt.Sprintf("%#v", err)} {
				if strings.Contains(formatted, secretMarker) {
					t.Fatalf("mapped error leaked remote detail: %q", formatted)
				}
			}
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestPrepareRecoveryHidesChannelExistenceAndOwnerAuthorizationState(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)

	var messages []string
	for _, remoteErr := range []error{relay.ErrUnauthenticated, relay.ErrNotFound} {
		remote := emptyInventoryRemote(recovery)
		remote.inventoryErr = remoteErr
		coordinator := mustCoordinator(t, store, remote)
		_, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{})
		assertProblem(t, err, CodeAuthorization, PhaseChannelAuthorization, ActionCheckRecoveryAuthority)
		messages = append(messages, err.Error())
	}

	remote := emptyInventoryRemote(recovery)
	remote.create = func(context.Context, relay.Channel) (relay.ChannelState, error) {
		return relay.ChannelState{}, relay.ErrImmutableConflict
	}
	coordinator := mustCoordinator(t, store, remote)
	_, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
	assertProblem(t, err, CodeAuthorization, PhaseChannelAuthorization, ActionCheckRecoveryAuthority)
	messages = append(messages, err.Error())

	remote = emptyInventoryRemote(recovery)
	remote.inventoryErr = relay.ErrUnauthenticated
	coordinator = mustCoordinator(t, store, remote)
	_, err = coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
	assertProblem(t, err, CodeAuthorization, PhaseChannelAuthorization, ActionCheckRecoveryAuthority)
	messages = append(messages, err.Error())

	for index := 1; index < len(messages); index++ {
		if messages[index] != messages[0] {
			t.Fatalf("authorization-state errors differ: %q != %q", messages[index], messages[0])
		}
	}
}

func TestPrepareRecoveryRejectsCreateChannelResponseDrift(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)

	for _, test := range []struct {
		name   string
		mutate func(*relay.ChannelState)
	}{
		{name: "channel", mutate: func(state *relay.ChannelState) { state.ChannelID[0] ^= 0xff }},
		{name: "generation", mutate: func(state *relay.ChannelState) { state.RelayGeneration[0] ^= 0xff }},
		{name: "negative head", mutate: func(state *relay.ChannelState) { state.Head = -1 }},
		{name: "empty membership with nonzero head", mutate: func(state *relay.ChannelState) { state.Head = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote := emptyInventoryRemote(recovery)
			remote.create = func(_ context.Context, channel relay.Channel) (relay.ChannelState, error) {
				state := relay.ChannelState{ChannelID: channel.ChannelID, RelayGeneration: channel.RelayGeneration}
				test.mutate(&state)
				return state, nil
			}
			coordinator := mustCoordinator(t, store, remote)
			_, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
			assertProblem(t, err, CodeRemote, PhaseChannelCreation, ActionRestartRecovery)
			if len(remote.environmentRequests) != 0 {
				t.Fatal("invalid create response reached inventory")
			}
			assertNoMutationCalls(t, remote)
		})
	}
}

func TestPrepareRecoveryPinsCreateStateAsInventoryLowerBound(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	records := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 19}

	for _, test := range []struct {
		name  string
		state relay.ChannelState
	}{
		{name: "membership rollback", state: relay.ChannelState{MembershipGeneration: 6}},
		{name: "arrival rollback", state: relay.ChannelState{MembershipGeneration: 5, Head: 20}},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote := inventoryRemote(recovery, snapshot, records)
			remote.create = func(_ context.Context, channel relay.Channel) (relay.ChannelState, error) {
				state := test.state
				state.ChannelID = channel.ChannelID
				state.RelayGeneration = channel.RelayGeneration
				return state, nil
			}
			coordinator := mustCoordinator(t, store, remote)
			got, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
			if !reflect.DeepEqual(got, credential.TrustedProjectCredential{}) {
				t.Fatal("create-to-inventory rollback returned credential material")
			}
			assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			if len(remote.environmentRequests) != 1 {
				t.Fatalf("inventory calls = %d, want first-page refusal", len(remote.environmentRequests))
			}
		})
	}

	remote := inventoryRemote(recovery, snapshot, records)
	remote.create = func(_ context.Context, channel relay.Channel) (relay.ChannelState, error) {
		return relay.ChannelState{
			ChannelID:            channel.ChannelID,
			RelayGeneration:      channel.RelayGeneration,
			MembershipGeneration: 1,
			Head:                 1,
		}, nil
	}
	coordinator := mustCoordinator(t, store, remote)
	prepared, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
	if err != nil {
		t.Fatalf("prepare across concurrent remote advancement: %v", err)
	}
	assertPreparedCredential(t, prepared, recovery, testEnvironmentID(200), 6)
}

func TestPrepareRecoveryMapsCreateChannelErrors(t *testing.T) {
	store := openCoordinatorStore(t, testEnvironmentID(200))
	recovery := testRecoveryCredential(t)
	secretMarker := "create-secret-marker"

	for _, test := range []struct {
		name       string
		remoteErr  error
		wantCode   ProblemCode
		wantPhase  ProblemPhase
		wantAction ProblemAction
	}{
		{name: "generation mismatch", remoteErr: relay.ErrGenerationMismatch, wantCode: CodeConflict, wantPhase: PhaseChannelCreation, wantAction: ActionRestartRecovery},
		{name: "rollback", remoteErr: relay.ErrRollback, wantCode: CodeConflict, wantPhase: PhaseChannelCreation, wantAction: ActionRestartRecovery},
		{name: "invalid", remoteErr: relay.ErrInvalidArgument, wantCode: CodeRemote, wantPhase: PhaseChannelCreation, wantAction: ActionRestartRecovery},
		{name: "closed", remoteErr: relay.ErrClosed, wantCode: CodeUnavailable, wantPhase: PhaseChannelCreation, wantAction: ActionRetry},
		{name: "unknown", remoteErr: errors.New(secretMarker), wantCode: CodeUnavailable, wantPhase: PhaseChannelCreation, wantAction: ActionRetry},
	} {
		t.Run(test.name, func(t *testing.T) {
			remote := emptyInventoryRemote(recovery)
			remote.create = func(context.Context, relay.Channel) (relay.ChannelState, error) {
				return relay.ChannelState{}, fmt.Errorf("%s: %w", secretMarker, test.remoteErr)
			}
			coordinator := mustCoordinator(t, store, remote)
			_, err := coordinator.PrepareRecovery(context.Background(), recovery.ProjectID, recovery, RecoveryPreparationOptions{CreateEmptyChannel: true})
			assertProblem(t, err, test.wantCode, test.wantPhase, test.wantAction)
			if strings.Contains(fmt.Sprintf("%#v", err), secretMarker) {
				t.Fatal("create error leaked remote detail")
			}
			if len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
				t.Fatal("failed create reached later remote phase")
			}
		})
	}
}

type remoteFixture struct {
	endpoint string

	endpointCalls int
	createCalls   int
	classifyCalls int
	registerCalls int
	pageCalls     int
	pruneCalls    int

	createRequests      []relay.Channel
	environmentRequests []relay.EnvironmentInventoryRequest
	classifyRequests    []relay.RegisterEnvironmentRequest
	registerRequests    []relay.RegisterEnvironmentRequest
	pageRequests        []relay.PageRequest

	create           func(context.Context, relay.Channel) (relay.ChannelState, error)
	classify         func(context.Context, relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error)
	register         func(context.Context, relay.RegisterEnvironmentRequest) (relay.ChannelState, error)
	page             func(context.Context, relay.PageRequest) (relay.Page, error)
	inventory        func(context.Context, relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error)
	inventoryErr     error
	environmentPages map[relay.EnvironmentID]relay.EnvironmentInventoryPage
}

func (remote *remoteFixture) Endpoint() string {
	remote.endpointCalls++
	return remote.endpoint
}

func (remote *remoteFixture) CreateChannel(ctx context.Context, channel relay.Channel) (relay.ChannelState, error) {
	remote.createCalls++
	remote.createRequests = append(remote.createRequests, channel)
	if remote.create != nil {
		return remote.create(ctx, channel)
	}
	return relay.ChannelState{ChannelID: channel.ChannelID, RelayGeneration: channel.RelayGeneration}, nil
}

func (remote *remoteFixture) RegisterEnvironment(ctx context.Context, request relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
	remote.registerCalls++
	remote.registerRequests = append(remote.registerRequests, cloneRegisterEnvironmentRequest(request))
	if remote.register != nil {
		return remote.register(ctx, request)
	}
	return relay.ChannelState{}, relay.ErrInvalidArgument
}

func (remote *remoteFixture) ClassifyEnvironmentRegistration(ctx context.Context, request relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error) {
	remote.classifyCalls++
	remote.classifyRequests = append(remote.classifyRequests, cloneRegisterEnvironmentRequest(request))
	if remote.classify != nil {
		return remote.classify(ctx, request)
	}
	return relay.EnvironmentRegistrationStatus{}, relay.ErrInvalidArgument
}

func cloneRegisterEnvironmentRequest(request relay.RegisterEnvironmentRequest) relay.RegisterEnvironmentRequest {
	cloned := request
	cloned.Environment.CertificateBytes = append([]byte(nil), request.Environment.CertificateBytes...)
	return cloned
}

func (remote *remoteFixture) Page(ctx context.Context, request relay.PageRequest) (relay.Page, error) {
	remote.pageCalls++
	remote.pageRequests = append(remote.pageRequests, request)
	if remote.page != nil {
		return remote.page(ctx, request)
	}
	return relay.Page{}, relay.ErrInvalidArgument
}

func (remote *remoteFixture) EnvironmentInventory(ctx context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
	copyRequest := request
	if request.Authorization.Owner != nil {
		copyOwner := *request.Authorization.Owner
		copyRequest.Authorization.Owner = &copyOwner
	}
	if request.Authorization.Environment != nil {
		copyEnvironment := *request.Authorization.Environment
		copyRequest.Authorization.Environment = &copyEnvironment
	}
	if request.Snapshot != nil {
		copySnapshot := *request.Snapshot
		copyRequest.Snapshot = &copySnapshot
	}
	remote.environmentRequests = append(remote.environmentRequests, copyRequest)
	if remote.inventory != nil {
		return remote.inventory(ctx, request)
	}
	if remote.inventoryErr != nil {
		return relay.EnvironmentInventoryPage{}, remote.inventoryErr
	}
	page, ok := remote.environmentPages[request.AfterEnvironmentID]
	if !ok {
		return relay.EnvironmentInventoryPage{}, relay.ErrInvalidArgument
	}
	page.Environments = append([]relay.EnvironmentInventoryRecord(nil), page.Environments...)
	return page, nil
}

func (remote *remoteFixture) PruneInventory(_ context.Context, _ relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
	remote.pruneCalls++
	return relay.PruneInventoryPage{}, relay.ErrInvalidArgument
}

func inventoryRemote(recovery credential.ProjectRecoveryCredential, snapshot relay.EnvironmentInventorySnapshot, records []relay.EnvironmentInventoryRecord) *remoteFixture {
	adminPublic := crypto.AdminPublicKey(recovery.AdminSeed)
	channel := relay.ChannelAuthority{
		ChannelID:       relay.ChannelID(recovery.ChannelID),
		RelayGeneration: relay.RelayGeneration(recovery.RelayGeneration),
		AdminPublicKey:  relay.PublicKey(adminPublic),
	}
	pages := make(map[relay.EnvironmentID]relay.EnvironmentInventoryPage)
	after := relay.EnvironmentID("")
	for start := 0; start < len(records); start += relay.MaxEnvironmentInventoryPage {
		end := start + relay.MaxEnvironmentInventoryPage
		if end > len(records) {
			end = len(records)
		}
		pageRecords := append([]relay.EnvironmentInventoryRecord(nil), records[start:end]...)
		pages[after] = relay.EnvironmentInventoryPage{
			Channel:      channel,
			Snapshot:     snapshot,
			Environments: pageRecords,
			More:         end < len(records),
		}
		after = pageRecords[len(pageRecords)-1].EnvironmentID
	}
	if len(records) == 0 {
		pages[""] = relay.EnvironmentInventoryPage{Channel: channel, Snapshot: snapshot}
	}
	return &remoteFixture{
		endpoint:         recovery.RelayURL,
		environmentPages: pages,
	}
}

func emptyInventoryRemote(recovery credential.ProjectRecoveryCredential) *remoteFixture {
	return inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
}

func openCoordinatorStore(t *testing.T, writerID continuity.EnvironmentID) *continuitysqlite.Store {
	t.Helper()
	store, err := continuitysqlite.Open(t.TempDir(), writerID)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mustCoordinator(t *testing.T, store *continuitysqlite.Store, remote Remote) *Coordinator {
	t.Helper()
	coordinator, err := New(store, remote)
	if err != nil {
		t.Fatalf("new coordinator: %v", err)
	}
	return coordinator
}

func testRecoveryCredential(t *testing.T) credential.ProjectRecoveryCredential {
	t.Helper()
	projectRoot, err := crypto.ProjectRootFromBytes(testBytes(1, len(crypto.ProjectRoot{})))
	if err != nil {
		t.Fatalf("project root: %v", err)
	}
	adminSeed, err := crypto.AdminSeedFromBytes(testBytes(2, len(crypto.AdminSeed{})))
	if err != nil {
		t.Fatalf("admin seed: %v", err)
	}
	var ownerID credential.RelayTokenID
	copy(ownerID[:], testBytes(3, len(ownerID)))
	var ownerSecret credential.RelayTokenSecret
	copy(ownerSecret[:], testBytes(4, len(ownerSecret)))
	owner, err := credential.NewRelayBearer(ownerID, ownerSecret)
	if err != nil {
		t.Fatalf("owner bearer: %v", err)
	}
	recovery := credential.ProjectRecoveryCredential{
		ProjectID:               testProjectID(1),
		RelayURL:                "https://relay.example.test",
		RelayGeneration:         protocol.RelayGeneration(testArray32(5)),
		ChannelID:               protocol.ChannelID(testArray32(6)),
		ProjectRoot:             projectRoot,
		AdminSeed:               adminSeed,
		OwnerRelayAuthorization: owner,
		WriteGeneration:         7,
		LastSignedCheckpoint:    []byte("signed-checkpoint"),
	}
	if err := recovery.Validate(); err != nil {
		t.Fatalf("test recovery credential: %v", err)
	}
	return recovery
}

func testInventoryRecord(t *testing.T, recovery credential.ProjectRecoveryCredential, id uint16, membership uint32) relay.EnvironmentInventoryRecord {
	t.Helper()
	environmentID := testEnvironmentID(id)
	seedBytes := make([]byte, len(crypto.EnvironmentSeed{}))
	for index := range seedBytes {
		seedBytes[index] = byte(index + 0x31)
	}
	seedBytes[0] = byte(id >> 8)
	seedBytes[1] = byte(id)
	environmentSeed, err := crypto.EnvironmentSeedFromBytes(seedBytes)
	if err != nil {
		t.Fatalf("environment seed: %v", err)
	}
	certificate, err := crypto.SignEnvironmentCertificate(protocol.EnvironmentCertificate{
		Version:               protocol.CertificateVersionV1,
		ProtocolVersion:       protocol.ProtocolVersionV1,
		CipherSuite:           protocol.CipherSuiteXChaCha20Poly1305,
		ProjectID:             recovery.ProjectID,
		ChannelID:             recovery.ChannelID,
		EnvironmentID:         environmentID,
		EnvironmentPublicKey:  crypto.EnvironmentPublicKey(environmentSeed),
		Mode:                  protocol.EnvironmentTrusted,
		MembershipGeneration:  membership,
		AllowedKeyGenerations: []uint32{recovery.WriteGeneration},
	}, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign environment certificate: %v", err)
	}
	certificateBytes, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal environment certificate: %v", err)
	}
	return relay.EnvironmentInventoryRecord{
		EnvironmentID:        relay.EnvironmentID(environmentID),
		CertificateID:        relay.Digest(protocol.CertificateID(certificate)),
		CertificateBytes:     certificateBytes,
		Mode:                 relay.TrustedEnvironment,
		MembershipGeneration: membership,
	}
}

func testRetiredInventoryRecord(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	id uint16,
	joinMembership uint32,
	retirementMembership uint32,
	finalSequence int64,
) relay.EnvironmentInventoryRecord {
	t.Helper()
	record := testInventoryRecord(t, recovery, id, joinMembership)
	record.ProducerHead = finalSequence
	certificate, err := protocol.ParseEnvironmentCertificate(record.CertificateBytes)
	if err != nil {
		t.Fatalf("parse retirement certificate: %v", err)
	}
	var finalDigest protocol.Digest
	if finalSequence > 0 {
		finalDigest = protocol.Digest(testArray32(byte(id + 0x40)))
	}
	retirement, err := crypto.SignTerminalRetirement(protocol.TerminalRetirement{
		Version:                  protocol.ControlVersionV1,
		ProtocolVersion:          protocol.ProtocolVersionV1,
		CipherSuite:              protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                recovery.ChannelID,
		RelayGeneration:          recovery.RelayGeneration,
		EnvironmentID:            recordEnvironmentID(record),
		CertificateID:            protocol.Digest(record.CertificateID),
		MembershipGeneration:     retirementMembership,
		FinalEnvironmentSequence: finalSequence,
		FinalEnvelopeDigest:      finalDigest,
	}, certificate, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign terminal retirement: %v", err)
	}
	retirementBytes, err := retirement.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal terminal retirement: %v", err)
	}
	record.Retirement = &relay.EnvironmentRetirement{
		RetiredAt:                time.Time{},
		RelayGeneration:          relay.RelayGeneration(retirement.RelayGeneration),
		CertificateID:            relay.Digest(retirement.CertificateID),
		MembershipGeneration:     retirement.MembershipGeneration,
		FinalEnvironmentSequence: retirement.FinalEnvironmentSequence,
		FinalEnvelopeDigest:      relay.Digest(retirement.FinalEnvelopeDigest),
		RetirementID:             relay.Digest(protocol.TerminalRetirementID(retirement)),
		RetirementBytes:          retirementBytes,
	}
	return record
}

func recordEnvironmentID(record relay.EnvironmentInventoryRecord) continuity.EnvironmentID {
	return continuity.EnvironmentID(record.EnvironmentID)
}

func cloneInventoryRecord(record relay.EnvironmentInventoryRecord) relay.EnvironmentInventoryRecord {
	record.CertificateBytes = append([]byte(nil), record.CertificateBytes...)
	if record.Retirement != nil {
		retirement := *record.Retirement
		retirement.RetirementBytes = append([]byte(nil), retirement.RetirementBytes...)
		record.Retirement = &retirement
	}
	return record
}

func testProjectID(value byte) continuity.ProjectID {
	return continuity.ProjectID(fmt.Sprintf("project-%03d", value))
}

func testEnvironmentID(value uint16) continuity.EnvironmentID {
	return continuity.EnvironmentID(fmt.Sprintf("environment-%04d", value))
}

func testArray32(value byte) [32]byte {
	var result [32]byte
	for i := range result {
		result[i] = value
	}
	return result
}

func testBytes(value byte, size int) []byte {
	result := make([]byte, size)
	for i := range result {
		result[i] = value
	}
	return result
}

func assertInventoryRequest(t *testing.T, got relay.EnvironmentInventoryRequest, recovery credential.ProjectRecoveryCredential, after relay.EnvironmentID, snapshot *relay.EnvironmentInventorySnapshot) {
	t.Helper()
	if got.Limit != relay.MaxEnvironmentInventoryPage || got.AfterEnvironmentID != after {
		t.Fatalf("inventory request cursor/limit = {%q %d}, want {%q %d}", got.AfterEnvironmentID, got.Limit, after, relay.MaxEnvironmentInventoryPage)
	}
	if got.Authorization.Owner == nil || got.Authorization.Environment != nil {
		t.Fatal("inventory request did not use owner-only authorization")
	}
	wantOwner := relay.OwnerAuthorization{
		ChannelID:       relay.ChannelID(recovery.ChannelID),
		RelayGeneration: relay.RelayGeneration(recovery.RelayGeneration),
		TokenID:         relay.RelayTokenID(recovery.OwnerRelayAuthorization.ID()),
		TokenSecret:     relay.RelayTokenSecret(recovery.OwnerRelayAuthorization.Secret()),
	}
	if *got.Authorization.Owner != wantOwner {
		t.Fatal("inventory request owner authorization did not match recovery credential")
	}
	if !reflect.DeepEqual(got.Snapshot, snapshot) {
		t.Fatalf("inventory request snapshot = %#v, want %#v", got.Snapshot, snapshot)
	}
}

func assertPreparedCredential(t *testing.T, got credential.TrustedProjectCredential, recovery credential.ProjectRecoveryCredential, writerID continuity.EnvironmentID, membership uint32) {
	t.Helper()
	if err := got.Validate(); err != nil {
		t.Fatalf("prepared credential validation: %v", err)
	}
	if got.ProjectID != recovery.ProjectID || got.RelayURL != recovery.RelayURL || got.RelayGeneration != recovery.RelayGeneration || got.ChannelID != recovery.ChannelID {
		t.Fatal("prepared credential changed project or relay authority")
	}
	if got.ProjectRoot != recovery.ProjectRoot || got.WriteGeneration != recovery.WriteGeneration {
		t.Fatal("prepared credential changed project root or write generation")
	}
	if got.Certificate.EnvironmentID != writerID || got.Certificate.MembershipGeneration != membership {
		t.Fatalf("prepared certificate identity/membership = {%q %d}, want {%q %d}", got.Certificate.EnvironmentID, got.Certificate.MembershipGeneration, writerID, membership)
	}
	if got.Certificate.Mode != protocol.EnvironmentTrusted || got.Certificate.ExpiresAtMillis != 0 {
		t.Fatal("prepared certificate is not non-expiring trusted authority")
	}
	if got.Certificate.Version != protocol.CertificateVersionV1 || got.Certificate.ProtocolVersion != protocol.ProtocolVersionV1 || got.Certificate.CipherSuite != protocol.CipherSuiteXChaCha20Poly1305 {
		t.Fatal("prepared certificate protocol suite is not v1")
	}
	if len(got.Certificate.AllowedKeyGenerations) != 1 || got.Certificate.AllowedKeyGenerations[0] != recovery.WriteGeneration {
		t.Fatal("prepared certificate authorizes generations outside recovery write generation")
	}
	if got.MinimumProtocolVersion != protocol.ProtocolVersionV1 {
		t.Fatalf("minimum protocol = %d, want %d", got.MinimumProtocolVersion, protocol.ProtocolVersionV1)
	}
	if len(got.LastObservedCheckpoint) != 0 {
		t.Fatal("prepared credential must not treat recovery checkpoint as an observed relay checkpoint")
	}
}

func assertNoMutationCalls(t *testing.T, remote *remoteFixture) {
	t.Helper()
	if remote.registerCalls != 0 {
		t.Fatalf("RegisterEnvironment calls = %d, want 0", remote.registerCalls)
	}
	if remote.pageCalls != 0 || remote.pruneCalls != 0 {
		t.Fatalf("unrelated remote calls = page:%d prune:%d", remote.pageCalls, remote.pruneCalls)
	}
}
