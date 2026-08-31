package coordinator

import (
	"context"
	"testing"

	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestRecoveryTerminalPrunedFrameUsesExactIndexedCertificateProjection(t *testing.T) {
	fixture, ready, inbox, certificate := recoveryTerminalProjectionFixture(t)
	frame, err := fixture.coordinator.recoveryTerminalPrunedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, ready, inbox,
	)
	if err != nil {
		t.Fatalf("resolve recovery terminal pruned frame: %v", err)
	}
	target := certificate.Manifest.Targets[0]
	if frame.PruneID != [32]byte(certificate.PruneID) ||
		frame.PruneCertificateID != [32]byte(protocol.PruneCertificateID(certificate)) ||
		frame.PruneCertificateID == frame.PruneID || frame.Reference.FactID != target.FactID ||
		frame.Reference.EnvironmentID != target.EnvironmentID ||
		frame.Reference.EnvironmentSequence != target.EnvironmentSequence ||
		frame.Reference.ArrivalSequence != target.ArrivalSequence ||
		frame.Reference.EnvelopeDigest != [32]byte(target.EnvelopeDigest) ||
		frame.Reference.CertificateID != [32]byte(target.CertificateID) ||
		frame.Reference.PreviousEnvelopeDigest != [32]byte(target.PreviousEnvelopeDigest) ||
		frame.Reference.KeyGeneration != target.KeyGeneration || frame.Reference.Nonce != [24]byte(target.Nonce) {
		t.Fatalf("resolved recovery terminal pruned frame = %#v", frame)
	}
}

func TestRecoveryTerminalPrunedFrameRejectsRelayAndReadyIndexDisagreement(t *testing.T) {
	fixture, ready, inbox, _ := recoveryTerminalProjectionFixture(t)
	parsed, err := protocol.ParsePrunedArrival(inbox.PrunedArrival)
	if err != nil {
		t.Fatalf("parse fixture pruned arrival: %v", err)
	}
	parsed.PruneID[0] ^= 0xff
	inbox.PrunedArrival, err = parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal mismatched pruned arrival: %v", err)
	}
	_, err = fixture.coordinator.recoveryTerminalPrunedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, ready, inbox,
	)
	assertProblem(t, err, CodeRemote, PhaseAttachActivation, ActionRestartRecovery)

	missing := inbox
	parsed.PruneID[0] ^= 0xff
	parsed.Reference.ArrivalSequence = 2
	parsed.Reference.EnvironmentSequence = 2
	parsed.Reference.EnvelopeDigest[0] ^= 0x11
	parsed.Reference.PreviousEnvelopeDigest = protocol.Digest(testArray32(0xc1))
	parsed.Reference.Nonce[0] ^= 0x22
	missing.ArrivalSequence = 2
	missing.EnvelopeDigest = [32]byte(parsed.Reference.EnvelopeDigest)
	missing.PrunedArrival, err = parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal missing-target pruned arrival: %v", err)
	}
	_, err = fixture.coordinator.recoveryTerminalPrunedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, ready, missing,
	)
	assertProblem(t, err, CodeRemote, PhaseAttachActivation, ActionRestartRecovery)
}

func recoveryTerminalProjectionFixture(
	t *testing.T,
) (recoveryDownloadFixture, continuitysqlite.SyncRecoveryPruneCandidate, continuitysqlite.OpaqueSyncFrame, protocol.PruneCertificate) {
	t.Helper()
	fixture := newRecoveryDownloadFixture(t, 2)
	stageRecoveryPrunePersistencePrefix(t, fixture, 2)
	record := testRecoveryPruneInventoryRecord(t, fixture, 1, 0xb1)
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: fixture.binding.MembershipGeneration,
		ArrivalHead:          fixture.binding.InventoryArrivalHead,
		PruneHead:            1,
	}
	fixture.remote.prune = recoveryPruneInventoryPages(fixture, snapshot, []relay.PruneInventoryRecord{record})
	ready, err := fixture.coordinator.persistRecoveryPruneInventory(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding,
	)
	if err != nil {
		t.Fatalf("persist recovery terminal prune projection: %v", err)
	}
	certificate := parseRecoveryPruneCertificate(t, record)
	target := certificate.Manifest.Targets[0]
	pruned := protocol.PrunedArrival{
		ChannelID: fixture.prepared.ChannelID, RelayGeneration: fixture.prepared.RelayGeneration,
		PruneID: certificate.PruneID, Reference: target,
	}
	encoded, err := pruned.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal recovery terminal pruned arrival: %v", err)
	}
	inbox := continuitysqlite.OpaqueSyncFrame{
		ArrivalSequence: target.ArrivalSequence,
		EnvelopeDigest:  [32]byte(target.EnvelopeDigest),
		PrunedArrival:   encoded,
	}
	return fixture, ready, inbox, certificate
}
