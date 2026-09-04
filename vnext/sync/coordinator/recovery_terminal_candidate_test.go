package coordinator

import (
	"context"
	"reflect"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

func TestRecoveryTerminalSealedFrameOpensExactPinnedEnvelope(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 1)
	inbox, fact := signedRecoveryTerminalInbox(t, fixture)
	if _, err := fixture.store.StageSyncPageUnderAuthority(
		context.Background(), fixture.recovery.ProjectID, fixture.binding, 0, 1,
		[]continuitysqlite.OpaqueSyncFrame{inbox},
	); err != nil {
		t.Fatalf("stage signed recovery terminal envelope: %v", err)
	}

	frame, err := fixture.coordinator.recoveryTerminalSealedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding, inbox,
	)
	if err != nil {
		t.Fatalf("open signed recovery terminal envelope: %v", err)
	}
	if frame.ArrivalSequence != inbox.ArrivalSequence || frame.EnvelopeDigest != inbox.EnvelopeDigest ||
		frame.CertificateID != [32]byte(protocol.CertificateID(fixture.prepared.Certificate)) ||
		frame.KeyGeneration != fixture.prepared.WriteGeneration || !reflect.DeepEqual(frame.Fact, fact) {
		t.Fatalf("verified recovery terminal sealed frame does not match its exact envelope")
	}
}

func TestRecoveryTerminalSealedFrameRejectsWrongKeyAndAlteredEnvelope(t *testing.T) {
	fixture := newRecoveryDownloadFixture(t, 1)
	inbox, _ := signedRecoveryTerminalInbox(t, fixture)

	altered := inbox
	altered.SealedEnvelope = append([]byte(nil), inbox.SealedEnvelope...)
	altered.SealedEnvelope[len(altered.SealedEnvelope)-1] ^= 0xff
	_, err := fixture.coordinator.recoveryTerminalSealedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding, altered,
	)
	assertProblem(t, err, CodeRemote, PhaseAttachActivation, ActionRestartRecovery)

	wrongRoot := fixture.prepared.ProjectRoot.Bytes()
	wrongRoot[0] ^= 0xff
	fixture.prepared.ProjectRoot, err = crypto.ProjectRootFromBytes(wrongRoot[:])
	if err != nil {
		t.Fatalf("construct wrong recovery root: %v", err)
	}
	_, err = fixture.coordinator.recoveryTerminalSealedFrame(
		context.Background(), fixture.recovery.ProjectID, fixture.prepared, fixture.binding, inbox,
	)
	assertProblem(t, err, CodeAuthorization, PhaseAttachActivation, ActionCheckRecoveryAuthority)
}

func TestMapRecoveryTerminalStoreErrorRetriesAuthorityCandidateRace(t *testing.T) {
	err := mapRecoveryTerminalStoreError(context.Background(), &continuitysqlite.SyncError{
		Code:  continuitysqlite.SyncErrorConflict,
		Field: "sync_authority_candidate",
	})
	assertProblem(t, err, CodeConflict, PhaseAttachActivation, ActionRetry)
}

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

func signedRecoveryTerminalInbox(
	t *testing.T,
	fixture recoveryDownloadFixture,
) (continuitysqlite.OpaqueSyncFrame, continuitywire.Fact) {
	t.Helper()
	fact := continuitywire.Fact{
		WireVersion:         continuitywire.Version1,
		FactID:              "fact-recovery-terminal-sealed",
		ProjectID:           fixture.recovery.ProjectID,
		SubjectKind:         continuity.RecordProjectIdentity,
		SubjectID:           continuity.SubjectID(fixture.recovery.ProjectID),
		FactKind:            continuity.FactProjectRegistered,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(`{"observation":{"observed_at_millis":1,"harness_session_id":"recovery-terminal","branch":"issue/loaf-93","worktree":"/workspace/loaf"},"label":"Loaf"}`),
		EnvironmentID:       fixture.prepared.Certificate.EnvironmentID,
		EnvironmentSequence: 1,
		HLCWallMillis:       100,
		EnvelopeVersion:     1,
	}
	key, err := crypto.DeriveGenerationKey(
		fixture.prepared.ProjectRoot, fixture.recovery.ProjectID, fixture.prepared.WriteGeneration,
	)
	if err != nil {
		t.Fatalf("derive recovery terminal generation key: %v", err)
	}
	sealed, err := crypto.SealFact(
		fact, key, fixture.prepared.Certificate, fixture.prepared.AdminPublicKey,
		fixture.prepared.EnvironmentSeed, protocol.Digest{}, 1_000,
	)
	if err != nil {
		t.Fatalf("seal recovery terminal fact: %v", err)
	}
	wire, err := sealed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal recovery terminal fact: %v", err)
	}
	return continuitysqlite.OpaqueSyncFrame{
		ArrivalSequence: 1,
		EnvelopeDigest:  [32]byte(protocol.EnvelopeDigest(sealed)),
		SealedEnvelope:  wire,
	}, fact
}
