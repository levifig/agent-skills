package coordinator

import (
	"context"
	"math"
	"testing"

	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestScanRecoveryInventoryAcceptsOnlyExactRegisteredWriter(t *testing.T) {
	writerID := testEnvironmentID(200)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	coordinator := mustCoordinator(t, store, remote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 6)
	expectedWriter := recoveryInventoryWriterFromRegistration(registration)

	t.Run("exact writer with later concurrent join", func(t *testing.T) {
		records := append(testGuardInventoryRecords(t, recovery), recoveryRegistrationInventoryRecord(registration))
		records = append(records, testInventoryRecord(t, recovery, 201, 7))
		snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 7, ArrivalHead: 22}
		remote := inventoryRemote(recovery, snapshot, records)
		coordinator := mustCoordinator(t, store, remote)
		var pages int
		result, err := coordinator.scanRecoveryInventory(
			context.Background(),
			recovery,
			recoveryOwnerAuthorization(recovery),
			crypto.AdminPublicKey(recovery.AdminSeed),
			writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: registration.targetMembershipGeneration,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					pages++
					return nil
				},
			},
		)
		if err != nil {
			t.Fatalf("scan exact registered writer: %v", err)
		}
		if result.snapshot != snapshot || pages != 2 || len(remote.environmentRequests) != 2 {
			t.Fatalf("scan result = {%#v pages=%d requests=%d}, want {%#v 2 2}", result.snapshot, pages, len(remote.environmentRequests), snapshot)
		}
	})

	t.Run("valid but different writer certificate", func(t *testing.T) {
		other := regenerateRecoveryRegistrationCertificate(t, recovery, registration)
		records := append(testGuardInventoryRecords(t, recovery), recoveryRegistrationInventoryRecord(other))
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: 20}, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 6,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		if callbacks != 1 {
			t.Fatalf("callbacks = %d, want only the verified prefix page", callbacks)
		}
	})

	t.Run("retired writer", func(t *testing.T) {
		records := append(testGuardInventoryRecords(t, recovery), retiredRecoveryRegistrationInventoryRecord(t, recovery, registration))
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 7, ArrivalHead: 20}, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 6,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		if callbacks != 1 {
			t.Fatalf("callbacks = %d, want only the verified prefix page", callbacks)
		}
	})

	for _, test := range []struct {
		name   string
		mutate func(*protocol.EnvironmentCertificate)
	}{
		{
			name: "ephemeral writer mode",
			mutate: func(certificate *protocol.EnvironmentCertificate) {
				certificate.Mode = protocol.EnvironmentEphemeral
				certificate.ExpiresAtMillis = 1 << 62
			},
		},
		{
			name: "expiring trusted writer",
			mutate: func(certificate *protocol.EnvironmentCertificate) {
				certificate.ExpiresAtMillis = 1 << 62
			},
		},
		{
			name: "writer joined at another membership",
			mutate: func(certificate *protocol.EnvironmentCertificate) {
				certificate.MembershipGeneration++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := mutatedRecoveryRegistrationInventoryRecord(t, recovery, registration, test.mutate)
			records := testGuardInventoryRecords(t, recovery)
			if record.MembershipGeneration == 7 {
				records = append(records, testInventoryRecord(t, recovery, 100, 6))
			}
			records = append(records, record)
			snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: record.MembershipGeneration, ArrivalHead: 20}
			remote := inventoryRemote(recovery, snapshot, records)
			coordinator := mustCoordinator(t, store, remote)
			var callbacks int
			_, err := coordinator.scanRecoveryInventory(
				context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
				recoveryInventoryScanOptions{
					minimumMembershipGeneration: 6,
					expectedLocalWriter:         &expectedWriter,
					onPage: func(verifiedRecoveryInventoryPage) error {
						callbacks++
						return nil
					},
				},
			)
			assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
			if callbacks != 1 {
				t.Fatalf("callbacks = %d, want only the verified prefix page", callbacks)
			}
		})
	}

	t.Run("missing writer", func(t *testing.T) {
		records := append(testGuardInventoryRecords(t, recovery), testInventoryRecord(t, recovery, 201, 6))
		remote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: 20}, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 6,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		if callbacks != 1 {
			t.Fatalf("callbacks = %d, want only the verified prefix page before crossing the missing writer", callbacks)
		}
	})

	t.Run("membership below successful registration", func(t *testing.T) {
		remote := inventoryRemote(
			recovery,
			relay.EnvironmentInventorySnapshot{MembershipGeneration: 5, ArrivalHead: 20},
			testGuardInventoryRecords(t, recovery),
		)
		coordinator := mustCoordinator(t, store, remote)
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{minimumMembershipGeneration: 6, expectedLocalWriter: &expectedWriter},
		)
		assertProblem(t, err, CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
		if len(remote.environmentRequests) != 1 {
			t.Fatalf("inventory requests = %d, want first-page refusal", len(remote.environmentRequests))
		}
	})
}

func TestScanRecoveryInventoryResumesPinnedSuffixWithoutReplayingPrefix(t *testing.T) {
	writerID := testEnvironmentID(3)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	coordinator := mustCoordinator(t, store, seedRemote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 3)
	expectedWriter := recoveryInventoryWriterFromRegistration(registration)
	records := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		recoveryRegistrationInventoryRecord(registration),
		testInventoryRecord(t, recovery, 4, 4),
		testInventoryRecord(t, recovery, 5, 5),
		testInventoryRecord(t, recovery, 6, 6),
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: 20}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator = mustCoordinator(t, store, remote)
	startAfter := records[3].EnvironmentID
	var verified verifiedRecoveryInventoryPage

	result, err := coordinator.scanRecoveryInventory(
		context.Background(),
		recovery,
		recoveryOwnerAuthorization(recovery),
		crypto.AdminPublicKey(recovery.AdminSeed),
		writerID,
		recoveryInventoryScanOptions{
			minimumMembershipGeneration: 3,
			firstRequestSnapshot:        &snapshot,
			firstAfterEnvironmentID:     startAfter,
			expectedLocalWriter:         &expectedWriter,
			onPage: func(page verifiedRecoveryInventoryPage) error {
				verified = page
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("scan pinned suffix: %v", err)
	}
	if result.snapshot != snapshot || len(remote.environmentRequests) != 1 {
		t.Fatalf("suffix result = {%#v requests=%d}, want {%#v 1}", result.snapshot, len(remote.environmentRequests), snapshot)
	}
	assertInventoryRequest(t, remote.environmentRequests[0], recovery, startAfter, &snapshot)
	if verified.afterEnvironmentID != startAfter || len(verified.environments) != 2 || verified.more {
		t.Fatalf("verified suffix page = %#v, want after %q with two terminal records", verified, startAfter)
	}
}

func TestScanRecoveryInventoryRequiresExpectedWriterInPinnedSuffix(t *testing.T) {
	writerID := testEnvironmentID(5)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	coordinator := mustCoordinator(t, store, seedRemote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 5)
	expectedWriter := recoveryInventoryWriterFromRegistration(registration)
	prefix := []relay.EnvironmentInventoryRecord{
		testInventoryRecord(t, recovery, 1, 1),
		testInventoryRecord(t, recovery, 2, 2),
		testInventoryRecord(t, recovery, 3, 3),
		testInventoryRecord(t, recovery, 4, 4),
	}
	startAfter := prefix[len(prefix)-1].EnvironmentID
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 6, ArrivalHead: 20}

	t.Run("exact writer", func(t *testing.T) {
		records := append(append([]relay.EnvironmentInventoryRecord(nil), prefix...), recoveryRegistrationInventoryRecord(registration))
		records = append(records, testInventoryRecord(t, recovery, 6, 6))
		remote := inventoryRemote(recovery, snapshot, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 5,
				firstRequestSnapshot:        &snapshot,
				firstAfterEnvironmentID:     startAfter,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		if err != nil || callbacks != 1 {
			t.Fatalf("scan suffix containing exact writer = {err=%v callbacks=%d}, want {nil 1}", err, callbacks)
		}
	})

	t.Run("missing writer crossing page is not staged", func(t *testing.T) {
		records := append([]relay.EnvironmentInventoryRecord(nil), prefix...)
		for id := uint16(6); id <= 10; id++ {
			records = append(records, testInventoryRecord(t, recovery, id, uint32(id)))
		}
		crossingSnapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: 10, ArrivalHead: 20}
		remote := inventoryRemote(recovery, crossingSnapshot, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 5,
				firstRequestSnapshot:        &crossingSnapshot,
				firstAfterEnvironmentID:     startAfter,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		if callbacks != 0 || len(remote.environmentRequests) != 1 {
			t.Fatalf("missing-writer crossing = {callbacks=%d requests=%d}, want {0 1}", callbacks, len(remote.environmentRequests))
		}
	})

	t.Run("different writer", func(t *testing.T) {
		other := regenerateRecoveryRegistrationCertificate(t, recovery, registration)
		records := append(append([]relay.EnvironmentInventoryRecord(nil), prefix...), recoveryRegistrationInventoryRecord(other))
		records = append(records, testInventoryRecord(t, recovery, 6, 6))
		remote := inventoryRemote(recovery, snapshot, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 5,
				firstRequestSnapshot:        &snapshot,
				firstAfterEnvironmentID:     startAfter,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		if callbacks != 0 {
			t.Fatalf("callbacks = %d, want writer-mismatching page withheld", callbacks)
		}
	})

	t.Run("cursor requires pinned snapshot", func(t *testing.T) {
		remote := inventoryRemote(recovery, snapshot, nil)
		coordinator := mustCoordinator(t, store, remote)
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{firstAfterEnvironmentID: startAfter, expectedLocalWriter: &expectedWriter},
		)
		assertProblem(t, err, CodeInvalid, PhaseEnvironmentInventory, ActionRestartRecovery)
		if len(remote.environmentRequests) != 0 {
			t.Fatalf("unpinned resume made %d remote requests", len(remote.environmentRequests))
		}
	})

	t.Run("pinned snapshot drift", func(t *testing.T) {
		records := append(append([]relay.EnvironmentInventoryRecord(nil), prefix...), recoveryRegistrationInventoryRecord(registration))
		records = append(records, testInventoryRecord(t, recovery, 6, 6))
		drifted := snapshot
		drifted.ArrivalHead++
		remote := inventoryRemote(recovery, drifted, records)
		coordinator := mustCoordinator(t, store, remote)
		var callbacks int
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 5,
				firstRequestSnapshot:        &snapshot,
				firstAfterEnvironmentID:     startAfter,
				expectedLocalWriter:         &expectedWriter,
				onPage: func(verifiedRecoveryInventoryPage) error {
					callbacks++
					return nil
				},
			},
		)
		assertProblem(t, err, CodeConflict, PhaseEnvironmentInventory, ActionRetry)
		if callbacks != 0 || len(remote.environmentRequests) != 1 {
			t.Fatalf("snapshot drift = {callbacks=%d requests=%d}, want {0 1}", callbacks, len(remote.environmentRequests))
		}
	})

	t.Run("terminal membership does not require a successor generation", func(t *testing.T) {
		records := append(append([]relay.EnvironmentInventoryRecord(nil), prefix...), recoveryRegistrationInventoryRecord(registration))
		records = append(records, testInventoryRecord(t, recovery, 6, 6))
		terminal := relay.EnvironmentInventorySnapshot{MembershipGeneration: math.MaxUint32, ArrivalHead: 20}
		remote := inventoryRemote(recovery, terminal, records)
		coordinator := mustCoordinator(t, store, remote)
		_, err := coordinator.scanRecoveryInventory(
			context.Background(), recovery, recoveryOwnerAuthorization(recovery), crypto.AdminPublicKey(recovery.AdminSeed), writerID,
			recoveryInventoryScanOptions{
				minimumMembershipGeneration: 5,
				firstRequestSnapshot:        &terminal,
				firstAfterEnvironmentID:     startAfter,
				expectedLocalWriter:         &expectedWriter,
			},
		)
		if err != nil {
			t.Fatalf("scan terminal-membership suffix: %v", err)
		}
	})
}

func TestScanRecoveryInventoryStreamsLargePinnedSuffixWithoutLifetimeCap(t *testing.T) {
	writerID := testEnvironmentID(3)
	store := openCoordinatorStore(t, writerID)
	recovery := testBindableRecoveryCredential(t)
	seedRemote := inventoryRemote(recovery, relay.EnvironmentInventorySnapshot{}, nil)
	coordinator := mustCoordinator(t, store, seedRemote)
	registration := bindGuardRegistration(t, coordinator, recovery, writerID, 3)
	expectedWriter := recoveryInventoryWriterFromRegistration(registration)
	records := make([]relay.EnvironmentInventoryRecord, 261)
	records[0] = testInventoryRecord(t, recovery, 1, 1)
	records[1] = testInventoryRecord(t, recovery, 2, 2)
	records[2] = recoveryRegistrationInventoryRecord(registration)
	for index := 3; index < len(records); index++ {
		records[index] = testInventoryRecord(t, recovery, uint16(index+1), uint32(index+1))
	}
	snapshot := relay.EnvironmentInventorySnapshot{MembershipGeneration: uint32(len(records)), ArrivalHead: 400}
	remote := inventoryRemote(recovery, snapshot, records)
	coordinator = mustCoordinator(t, store, remote)
	startAfter := records[3].EnvironmentID
	var pages, environments int

	_, err := coordinator.scanRecoveryInventory(
		context.Background(),
		recovery,
		recoveryOwnerAuthorization(recovery),
		crypto.AdminPublicKey(recovery.AdminSeed),
		writerID,
		recoveryInventoryScanOptions{
			minimumMembershipGeneration: 3,
			firstRequestSnapshot:        &snapshot,
			firstAfterEnvironmentID:     startAfter,
			expectedLocalWriter:         &expectedWriter,
			onPage: func(page verifiedRecoveryInventoryPage) error {
				pages++
				environments += len(page.environments)
				return nil
			},
		},
	)
	if err != nil {
		t.Fatalf("scan large pinned suffix: %v", err)
	}
	if pages != 65 || environments != 257 || len(remote.environmentRequests) != 65 {
		t.Fatalf("large suffix = {pages=%d environments=%d requests=%d}, want {65 257 65}", pages, environments, len(remote.environmentRequests))
	}
	for index, request := range remote.environmentRequests {
		if request.Limit != relay.MaxEnvironmentInventoryPage || request.Snapshot == nil || *request.Snapshot != snapshot {
			t.Fatalf("request %d = %#v, want bounded pinned request", index, request)
		}
	}
}

func recoveryRegistrationInventoryRecord(registration preparedRecoveryRegistration) relay.EnvironmentInventoryRecord {
	environment := registration.environment
	return relay.EnvironmentInventoryRecord{
		EnvironmentID:        environment.EnvironmentID,
		CertificateID:        environment.CertificateID,
		CertificateBytes:     append([]byte(nil), environment.CertificateBytes...),
		Mode:                 environment.Mode,
		ExpiresAtMillis:      environment.ExpiresAtMillis,
		MembershipGeneration: environment.MembershipGeneration,
	}
}

func mutatedRecoveryRegistrationInventoryRecord(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	mutate func(*protocol.EnvironmentCertificate),
) relay.EnvironmentInventoryRecord {
	t.Helper()
	certificate, err := protocol.ParseEnvironmentCertificate(registration.environment.CertificateBytes)
	if err != nil {
		t.Fatalf("parse registered writer certificate: %v", err)
	}
	mutate(&certificate)
	certificate, err = crypto.SignEnvironmentCertificate(certificate, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign mutated writer certificate: %v", err)
	}
	certificateBytes, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal mutated writer certificate: %v", err)
	}
	var mode relay.EnvironmentMode
	switch certificate.Mode {
	case protocol.EnvironmentTrusted:
		mode = relay.TrustedEnvironment
	case protocol.EnvironmentEphemeral:
		mode = relay.EphemeralEnvironment
	default:
		t.Fatalf("mutated writer mode = %d", certificate.Mode)
	}
	return relay.EnvironmentInventoryRecord{
		EnvironmentID:        registration.environment.EnvironmentID,
		CertificateID:        relay.Digest(protocol.CertificateID(certificate)),
		CertificateBytes:     certificateBytes,
		Mode:                 mode,
		ExpiresAtMillis:      certificate.ExpiresAtMillis,
		MembershipGeneration: certificate.MembershipGeneration,
	}
}

func retiredRecoveryRegistrationInventoryRecord(
	t *testing.T,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) relay.EnvironmentInventoryRecord {
	t.Helper()
	record := recoveryRegistrationInventoryRecord(registration)
	certificate, err := protocol.ParseEnvironmentCertificate(record.CertificateBytes)
	if err != nil {
		t.Fatalf("parse registered writer certificate for retirement: %v", err)
	}
	retirement, err := crypto.SignTerminalRetirement(protocol.TerminalRetirement{
		Version:                  protocol.ControlVersionV1,
		ProtocolVersion:          protocol.ProtocolVersionV1,
		CipherSuite:              protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                recovery.ChannelID,
		RelayGeneration:          recovery.RelayGeneration,
		EnvironmentID:            recordEnvironmentID(record),
		CertificateID:            protocol.Digest(record.CertificateID),
		MembershipGeneration:     registration.targetMembershipGeneration + 1,
		FinalEnvironmentSequence: 0,
	}, certificate, recovery.AdminSeed)
	if err != nil {
		t.Fatalf("sign registered writer retirement: %v", err)
	}
	retirementBytes, err := retirement.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal registered writer retirement: %v", err)
	}
	record.Retirement = &relay.EnvironmentRetirement{
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
