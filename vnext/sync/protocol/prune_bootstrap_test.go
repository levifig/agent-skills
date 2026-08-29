package protocol

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestPruneBootstrapCanonicalRoundTripsAndDigestScope(t *testing.T) {
	t.Parallel()

	plaintext := testPruneBootstrapPlaintext()
	plaintextWire, err := plaintext.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal plaintext: %v", err)
	}
	decodedPlaintext, err := ParsePruneBootstrapPlaintext(plaintextWire)
	if err != nil {
		t.Fatalf("parse plaintext: %v", err)
	}
	if !reflect.DeepEqual(decodedPlaintext, plaintext) {
		t.Fatalf("plaintext round trip = %#v, want %#v", decodedPlaintext, plaintext)
	}
	if _, err := ParsePruneBootstrapPlaintext(append(append([]byte(nil), plaintextWire...), 0)); !errors.Is(err, ErrInvalidPruneBootstrapPlaintext) {
		t.Fatalf("plaintext trailing-byte error = %v, want %v", err, ErrInvalidPruneBootstrapPlaintext)
	}

	capsule := testPruneBootstrap(plaintext)
	capsuleWire, err := capsule.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal capsule: %v", err)
	}
	decodedCapsule, err := ParsePruneBootstrap(capsuleWire)
	if err != nil {
		t.Fatalf("parse capsule: %v", err)
	}
	if !reflect.DeepEqual(decodedCapsule, capsule) {
		t.Fatalf("capsule round trip = %#v, want %#v", decodedCapsule, capsule)
	}
	if got, want := PruneBootstrapDigest(decodedCapsule), sha256.Sum256(capsuleWire); got != want {
		t.Fatalf("capsule digest = %x, want complete-wire digest %x", got, want)
	}
	changedCiphertext := capsule
	changedCiphertext.Ciphertext = append([]byte(nil), capsule.Ciphertext...)
	changedCiphertext.Ciphertext[0] ^= 1
	if PruneBootstrapDigest(changedCiphertext) == PruneBootstrapDigest(capsule) {
		t.Fatal("capsule digest omitted ciphertext")
	}
	if _, err := ParsePruneBootstrap(append(append([]byte(nil), capsuleWire...), 0)); !errors.Is(err, ErrInvalidPruneBootstrap) {
		t.Fatalf("capsule trailing-byte error = %v, want %v", err, ErrInvalidPruneBootstrap)
	}
}

func TestPruneBootstrapAADBindsEveryClearFieldAndCredentialProject(t *testing.T) {
	t.Parallel()

	capsule := testPruneBootstrap(testPruneBootstrapPlaintext())
	baseline, err := PruneBootstrapAAD(capsule, "project-1")
	if err != nil {
		t.Fatalf("build baseline AAD: %v", err)
	}
	mutations := []struct {
		name   string
		mutate func(*PruneBootstrap)
	}{
		{name: "capsule version", mutate: func(value *PruneBootstrap) { value.CapsuleVersion++ }},
		{name: "protocol version", mutate: func(value *PruneBootstrap) { value.ProtocolVersion++ }},
		{name: "cipher suite", mutate: func(value *PruneBootstrap) { value.CipherSuite++ }},
		{name: "purpose version", mutate: func(value *PruneBootstrap) { value.BootstrapPurposeVersion++ }},
		{name: "channel", mutate: func(value *PruneBootstrap) { value.ChannelID[0] ^= 1 }},
		{name: "relay generation", mutate: func(value *PruneBootstrap) { value.RelayGeneration[0] ^= 1 }},
		{name: "prune", mutate: func(value *PruneBootstrap) { value.PruneID[0] ^= 1 }},
		{name: "membership generation", mutate: func(value *PruneBootstrap) { value.MembershipGeneration++ }},
		{name: "barrier", mutate: func(value *PruneBootstrap) { value.BarrierArrivalSequence++ }},
		{name: "closure reference", mutate: func(value *PruneBootstrap) { value.ClosureReferenceDigest[0] ^= 1 }},
		{name: "manifest count", mutate: func(value *PruneBootstrap) { value.ManifestCount++ }},
		{name: "manifest digest", mutate: func(value *PruneBootstrap) { value.ManifestDigest[0] ^= 1 }},
		{name: "nonce", mutate: func(value *PruneBootstrap) { value.Nonce[0] ^= 1 }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := capsule
			test.mutate(&candidate)
			mutated, err := pruneBootstrapAADUnchecked(candidate, "project-1")
			if err != nil {
				t.Fatalf("build mutated AAD: %v", err)
			}
			if bytes.Equal(mutated, baseline) {
				t.Fatal("clear-field mutation did not change AAD")
			}
		})
	}

	projectAAD, err := PruneBootstrapAAD(capsule, "project-2")
	if err != nil {
		t.Fatalf("build different-project AAD: %v", err)
	}
	if bytes.Equal(projectAAD, baseline) {
		t.Fatal("credential project did not change AAD")
	}
	ciphertextMutation := capsule
	ciphertextMutation.Ciphertext = append([]byte(nil), capsule.Ciphertext...)
	ciphertextMutation.Ciphertext[0] ^= 1
	ciphertextAAD, err := PruneBootstrapAAD(ciphertextMutation, "project-1")
	if err != nil {
		t.Fatalf("build ciphertext-mutated AAD: %v", err)
	}
	if !bytes.Equal(ciphertextAAD, baseline) {
		t.Fatal("ciphertext was included in prune-bootstrap AAD")
	}
}

func TestPruneBootstrapEntriesAreClosedBoundedAndOrdered(t *testing.T) {
	t.Parallel()

	entry := testPruneBootstrapPlaintext().Entries[0]
	wire, err := entry.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	decoded, err := ParsePruneBootstrapEntry(wire)
	if err != nil {
		t.Fatalf("parse entry: %v", err)
	}
	if decoded != entry {
		t.Fatalf("entry round trip = %#v, want %#v", decoded, entry)
	}

	for _, kind := range []continuity.FactKind{
		continuity.FactScratchpadParticipantIntroduced,
		continuity.FactScratchpadMessageRecorded,
		continuity.FactScratchpadClaimRecorded,
		continuity.FactScratchpadClaimReleased,
	} {
		candidate := entry
		candidate.FactKind = kind
		if err := candidate.Validate(); err != nil {
			t.Fatalf("allowed kind %q: %v", kind, err)
		}
	}
	for _, kind := range []continuity.FactKind{
		continuity.FactScratchpadOpened,
		continuity.FactScratchpadClosed,
		continuity.FactJournalRecorded,
		"scratchpad.unknown",
	} {
		candidate := entry
		candidate.FactKind = kind
		if err := candidate.Validate(); !errors.Is(err, ErrInvalidPruneBootstrapEntry) {
			t.Fatalf("disallowed kind %q error = %v, want %v", kind, err, ErrInvalidPruneBootstrapEntry)
		}
	}
	negativeClock := entry
	negativeClock.HLC.WallMillis = -1
	if err := negativeClock.Validate(); !errors.Is(err, ErrInvalidPruneBootstrapEntry) {
		t.Fatalf("negative HLC error = %v, want %v", err, ErrInvalidPruneBootstrapEntry)
	}

	maximum := testPruneBootstrapPlaintext()
	maximum.Entries = make([]PruneBootstrapEntry, MaxPruneTargets)
	for index := range maximum.Entries {
		maximum.Entries[index] = entry
		maximum.Entries[index].PruneReferenceDigest = sha256.Sum256([]byte(fmt.Sprintf("reference-%04d", index)))
		maximum.Entries[index].HLC = continuity.HybridTime{WallMillis: int64(index), Logical: int32(index)}
	}
	maximum.EntryCount = MaxPruneTargets
	maximum.ManifestCount = MaxPruneTargets
	maximum.ManifestDigest = sha256.Sum256([]byte("maximum manifest"))
	maximumWire, err := maximum.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal maximum plaintext: %v", err)
	}
	if len(maximumWire) > MaxPruneBootstrapPlaintextBytes {
		t.Fatalf("maximum plaintext encoded to %d bytes, limit %d", len(maximumWire), MaxPruneBootstrapPlaintextBytes)
	}
	parsed, err := ParsePruneBootstrapPlaintext(maximumWire)
	if err != nil {
		t.Fatalf("parse maximum plaintext: %v", err)
	}
	if len(parsed.Entries) != MaxPruneTargets {
		t.Fatalf("maximum plaintext decoded %d entries", len(parsed.Entries))
	}

	reordered := testPruneBootstrapPlaintext()
	reordered.Entries[0], reordered.Entries[1] = reordered.Entries[1], reordered.Entries[0]
	reorderedWire, err := reordered.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal reordered entries: %v", err)
	}
	parsedReordered, err := ParsePruneBootstrapPlaintext(reorderedWire)
	if err != nil {
		t.Fatalf("parse reordered entries: %v", err)
	}
	if parsedReordered.Entries[0] != reordered.Entries[0] {
		t.Fatal("entry order was not preserved")
	}
}

func TestPruneBootstrapValidationRejectsCountDuplicatesAndUnsupportedSelectors(t *testing.T) {
	t.Parallel()

	valid := testPruneBootstrapPlaintext()
	tests := []struct {
		name   string
		mutate func(*PruneBootstrapPlaintext)
	}{
		{name: "capsule version", mutate: func(value *PruneBootstrapPlaintext) { value.CapsuleVersion++ }},
		{name: "protocol version", mutate: func(value *PruneBootstrapPlaintext) { value.ProtocolVersion++ }},
		{name: "cipher suite", mutate: func(value *PruneBootstrapPlaintext) { value.CipherSuite++ }},
		{name: "purpose version", mutate: func(value *PruneBootstrapPlaintext) { value.BootstrapPurposeVersion++ }},
		{name: "manifest count", mutate: func(value *PruneBootstrapPlaintext) { value.ManifestCount++ }},
		{name: "entry count", mutate: func(value *PruneBootstrapPlaintext) { value.EntryCount++ }},
		{name: "duplicate reference", mutate: func(value *PruneBootstrapPlaintext) {
			value.Entries[1].PruneReferenceDigest = value.Entries[0].PruneReferenceDigest
		}},
		{name: "zero subject", mutate: func(value *PruneBootstrapPlaintext) { value.ScratchpadSubject = "" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := clonePruneBootstrapPlaintext(valid)
			test.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want refusal")
			}
		})
	}
}

func TestPrunedArrivalStrictCanonicalBoundAndExactReference(t *testing.T) {
	t.Parallel()

	reference := testPruneReference(
		continuity.FactID(strings.Repeat("f", 128)),
		continuity.EnvironmentID(strings.Repeat("e", 128)),
		2,
		9,
		0x72,
	)
	reference.PreviousEnvelopeDigest = testControlDigest(0x71)
	arrival := PrunedArrival{
		ChannelID:       testControlChannelID(),
		RelayGeneration: testControlRelayGeneration(),
		PruneID:         testControlDigest(0x73),
		Reference:       reference,
	}
	wire, err := arrival.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal pruned arrival: %v", err)
	}
	if len(wire) > MaxPrunedArrivalBytes {
		t.Fatalf("maximum-ID pruned arrival encoded to %d bytes, limit %d", len(wire), MaxPrunedArrivalBytes)
	}
	decoded, err := ParsePrunedArrival(wire)
	if err != nil {
		t.Fatalf("parse pruned arrival: %v", err)
	}
	if decoded != arrival {
		t.Fatalf("pruned arrival round trip = %#v, want %#v", decoded, arrival)
	}
	if _, err := ParsePrunedArrival(append(append([]byte(nil), wire...), 0)); !errors.Is(err, ErrInvalidPrunedArrival) {
		t.Fatalf("trailing-byte error = %v, want %v", err, ErrInvalidPrunedArrival)
	}
	if _, err := ParsePrunedArrival(make([]byte, MaxPrunedArrivalBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize error = %v, want %v", err, ErrTooLarge)
	}
}

func TestPruneBootstrapParsersEnforceHardBoundsAndDoNotAliasInput(t *testing.T) {
	t.Parallel()

	if _, err := ParsePruneBootstrap(make([]byte, MaxPruneBootstrapBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize capsule error = %v, want %v", err, ErrTooLarge)
	}
	if _, err := ParsePruneBootstrapPlaintext(make([]byte, MaxPruneBootstrapPlaintextBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize plaintext error = %v, want %v", err, ErrTooLarge)
	}
	if _, err := ParsePruneBootstrapEntry(make([]byte, MaxPruneBootstrapEntryBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize entry error = %v, want %v", err, ErrTooLarge)
	}
	if got, want := MaxPruneBootstrapBytes-MaxPruneBootstrapCiphertextBytes, pruneBootstrapOuterTranscriptAllowance; got != want {
		t.Fatalf("outer allowance = %d, want %d", got, want)
	}
	maximumCapsule := testPruneBootstrap(testPruneBootstrapPlaintext())
	maximumCapsule.Ciphertext = make([]byte, MaxPruneBootstrapCiphertextBytes)
	maximumWire, err := maximumCapsule.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal maximum capsule: %v", err)
	}
	if len(maximumWire) != MaxPruneBootstrapBytes {
		t.Fatalf("maximum capsule wire = %d bytes, want exact limit %d", len(maximumWire), MaxPruneBootstrapBytes)
	}
	oversizeCapsule := maximumCapsule
	oversizeCapsule.Ciphertext = make([]byte, MaxPruneBootstrapCiphertextBytes+1)
	if err := oversizeCapsule.Validate(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize ciphertext error = %v, want %v", err, ErrTooLarge)
	}

	capsule := testPruneBootstrap(testPruneBootstrapPlaintext())
	wire, err := capsule.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal capsule: %v", err)
	}
	decoded, err := ParsePruneBootstrap(wire)
	if err != nil {
		t.Fatalf("parse capsule: %v", err)
	}
	wire[len(wire)-1] ^= 1
	if !bytes.Equal(decoded.Ciphertext, capsule.Ciphertext) {
		t.Fatal("parsed capsule ciphertext aliases input wire")
	}
	decoded.Ciphertext[0] ^= 1
	if bytes.Equal(decoded.Ciphertext, capsule.Ciphertext) {
		t.Fatal("parsed capsule ciphertext aliases source value")
	}
}

func testPruneBootstrapPlaintext() PruneBootstrapPlaintext {
	return PruneBootstrapPlaintext{
		CapsuleVersion:          PruneBootstrapCapsuleVersionV1,
		ProtocolVersion:         ProtocolVersionV1,
		CipherSuite:             CipherSuiteXChaCha20Poly1305,
		BootstrapPurposeVersion: PruneBootstrapPurposeVersionV1,
		ProjectID:               "project-1",
		ChannelID:               testControlChannelID(),
		RelayGeneration:         testControlRelayGeneration(),
		PruneID:                 testControlDigest(0xc0),
		MembershipGeneration:    7,
		BarrierArrivalSequence:  19,
		ClosureReferenceDigest:  testControlDigest(0xc1),
		ManifestCount:           2,
		ManifestDigest:          testControlDigest(0xc2),
		ScratchpadSubject:       "scratchpad-1",
		EntryCount:              2,
		Entries: []PruneBootstrapEntry{
			{
				PruneReferenceDigest: testControlDigest(0xd0),
				FactKind:             continuity.FactScratchpadMessageRecorded,
				HLC:                  continuity.HybridTime{WallMillis: 100, Logical: 2},
			},
			{
				PruneReferenceDigest: testControlDigest(0xd1),
				FactKind:             continuity.FactScratchpadClaimReleased,
				HLC:                  continuity.HybridTime{WallMillis: 101, Logical: 0},
			},
		},
	}
}

func testPruneBootstrap(plaintext PruneBootstrapPlaintext) PruneBootstrap {
	ciphertext := make([]byte, 48)
	for index := range ciphertext {
		ciphertext[index] = byte(0xe0 + index)
	}
	return PruneBootstrap{
		CapsuleVersion:          plaintext.CapsuleVersion,
		ProtocolVersion:         plaintext.ProtocolVersion,
		CipherSuite:             plaintext.CipherSuite,
		BootstrapPurposeVersion: plaintext.BootstrapPurposeVersion,
		ChannelID:               plaintext.ChannelID,
		RelayGeneration:         plaintext.RelayGeneration,
		PruneID:                 plaintext.PruneID,
		MembershipGeneration:    plaintext.MembershipGeneration,
		BarrierArrivalSequence:  plaintext.BarrierArrivalSequence,
		ClosureReferenceDigest:  plaintext.ClosureReferenceDigest,
		ManifestCount:           plaintext.ManifestCount,
		ManifestDigest:          plaintext.ManifestDigest,
		Nonce:                   testPruneBootstrapNonce(0xe1),
		Ciphertext:              ciphertext,
	}
}

func clonePruneBootstrapPlaintext(value PruneBootstrapPlaintext) PruneBootstrapPlaintext {
	value.Entries = append([]PruneBootstrapEntry(nil), value.Entries...)
	return value
}

func testPruneBootstrapNonce(seed byte) Nonce {
	var value Nonce
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}
