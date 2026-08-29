package crypto

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

func TestPruneBootstrapKeysAreTypedDomainSeparatedAndFullyBound(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	base, err := DerivePruneBootstrapKey(root, "project-1", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive bootstrap key: %v", err)
	}
	repeated, err := DerivePruneBootstrapKey(root, "project-1", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("repeat bootstrap-key derivation: %v", err)
	}
	if base != repeated {
		t.Fatal("same bootstrap derivation inputs produced different keys")
	}
	differentProject, err := DerivePruneBootstrapKey(root, "project-2", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive different-project bootstrap key: %v", err)
	}
	if base == differentProject {
		t.Fatal("different projects produced the same typed bootstrap key")
	}
	if reflect.TypeOf(base) == reflect.TypeOf(GenerationKey{}) {
		t.Fatal("bootstrap and content-generation keys have the same Go type")
	}
	baseCopy := base.Bytes()
	baseCopy[0] ^= 1
	if baseCopy == base.Bytes() {
		t.Fatal("bootstrap key Bytes returned aliased material")
	}
	if _, err := DerivePruneBootstrapKey(root, "project-1", protocol.PruneBootstrapPurposeVersionV1+1); !errors.Is(err, ErrPruneBootstrapKeyDerivation) {
		t.Fatalf("unsupported purpose derivation error = %v, want %v", err, ErrPruneBootstrapKeyDerivation)
	}
	if _, err := NewPruneBootstrapKey("project-1", protocol.PruneBootstrapPurposeVersionV1+1, base.Bytes()); !errors.Is(err, ErrInvalidPruneBootstrapKey) {
		t.Fatalf("unsupported explicit purpose error = %v, want %v", err, ErrInvalidPruneBootstrapKey)
	}

	baseBytes := base.Bytes()
	for generation := uint32(1); generation <= protocol.MaxAllowedKeyGenerations+1; generation++ {
		contentKey, err := DeriveGenerationKey(root, "project-1", generation)
		if err != nil {
			t.Fatalf("derive content generation %d: %v", generation, err)
		}
		contentBytes := contentKey.Bytes()
		if baseBytes == contentBytes {
			t.Fatalf("bootstrap key reused content-generation %d material", generation)
		}
	}

	capsule := bootstrapOuter(testBootstrapPlaintext())
	first, err := derivePruneBootstrapAEADKey(base, capsule)
	if err != nil {
		t.Fatalf("derive per-prune key: %v", err)
	}
	repeatFirst, err := derivePruneBootstrapAEADKey(base, capsule)
	if err != nil {
		t.Fatalf("repeat per-prune derivation: %v", err)
	}
	if first != repeatFirst {
		t.Fatal("same per-prune derivation inputs produced different keys")
	}
	if reflect.TypeOf(first) == reflect.TypeOf(base) {
		t.Fatal("base and per-prune keys have the same Go type")
	}

	mutations := []struct {
		name   string
		mutate func(*protocol.PruneBootstrap)
	}{
		{name: "channel", mutate: func(value *protocol.PruneBootstrap) { value.ChannelID[0] ^= 1 }},
		{name: "relay", mutate: func(value *protocol.PruneBootstrap) { value.RelayGeneration[0] ^= 1 }},
		{name: "prune", mutate: func(value *protocol.PruneBootstrap) { value.PruneID[0] ^= 1 }},
		{name: "membership", mutate: func(value *protocol.PruneBootstrap) { value.MembershipGeneration++ }},
		{name: "barrier", mutate: func(value *protocol.PruneBootstrap) { value.BarrierArrivalSequence++ }},
		{name: "closure", mutate: func(value *protocol.PruneBootstrap) { value.ClosureReferenceDigest[0] ^= 1 }},
		{name: "manifest count", mutate: func(value *protocol.PruneBootstrap) { value.ManifestCount++ }},
		{name: "manifest digest", mutate: func(value *protocol.PruneBootstrap) { value.ManifestDigest[0] ^= 1 }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := capsule
			test.mutate(&candidate)
			derived, err := derivePruneBootstrapAEADKey(base, candidate)
			if err != nil {
				t.Fatalf("derive mutated per-prune key: %v", err)
			}
			if derived.material == first.material {
				t.Fatal("stable outer binding did not change per-prune key")
			}
		})
	}
}

func TestGeneratePruneIDUsesIndependentRandomIdentity(t *testing.T) {
	t.Parallel()

	first, err := GeneratePruneID()
	if err != nil {
		t.Fatalf("generate first prune ID: %v", err)
	}
	second, err := GeneratePruneID()
	if err != nil {
		t.Fatalf("generate second prune ID: %v", err)
	}
	if first == (protocol.Digest{}) || second == (protocol.Digest{}) || first == second {
		t.Fatal("independent prune identity generation returned zero or duplicate output")
	}
	if _, err := generatePruneIDWithRandom(bytes.NewReader(make([]byte, len(protocol.Digest{})))); !errors.Is(err, ErrRandomSource) {
		t.Fatalf("zero random prune ID error = %v, want %v", err, ErrRandomSource)
	}
	if _, err := generatePruneIDWithRandom(failingBootstrapReader{}); !errors.Is(err, ErrRandomSource) {
		t.Fatalf("random-source failure = %v, want %v", err, ErrRandomSource)
	}
}

func TestSealOpenPruneBootstrapAuthenticatesEveryBinding(t *testing.T) {
	t.Parallel()

	base, err := DerivePruneBootstrapKey(testProjectRoot(t), "project-1", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive bootstrap key: %v", err)
	}
	plaintext := testBootstrapPlaintext()
	nonce := controlNonce(0xa0)
	capsule, err := sealPruneBootstrapWithRandom(plaintext, base, bytes.NewReader(nonce[:]))
	if err != nil {
		t.Fatalf("seal bootstrap: %v", err)
	}
	opened, err := OpenPruneBootstrap(capsule, base)
	if err != nil {
		t.Fatalf("open bootstrap: %v", err)
	}
	if !reflect.DeepEqual(opened, plaintext) {
		t.Fatalf("opened plaintext = %#v, want %#v", opened, plaintext)
	}

	mutations := []struct {
		name   string
		mutate func(*protocol.PruneBootstrap)
	}{
		{name: "channel", mutate: func(value *protocol.PruneBootstrap) { value.ChannelID[0] ^= 1 }},
		{name: "relay", mutate: func(value *protocol.PruneBootstrap) { value.RelayGeneration[0] ^= 1 }},
		{name: "prune", mutate: func(value *protocol.PruneBootstrap) { value.PruneID[0] ^= 1 }},
		{name: "membership", mutate: func(value *protocol.PruneBootstrap) { value.MembershipGeneration++ }},
		{name: "barrier", mutate: func(value *protocol.PruneBootstrap) { value.BarrierArrivalSequence++ }},
		{name: "closure", mutate: func(value *protocol.PruneBootstrap) { value.ClosureReferenceDigest[0] ^= 1 }},
		{name: "manifest count", mutate: func(value *protocol.PruneBootstrap) { value.ManifestCount++ }},
		{name: "manifest digest", mutate: func(value *protocol.PruneBootstrap) { value.ManifestDigest[0] ^= 1 }},
		{name: "nonce", mutate: func(value *protocol.PruneBootstrap) { value.Nonce[0] ^= 1 }},
		{name: "ciphertext", mutate: func(value *protocol.PruneBootstrap) { value.Ciphertext[0] ^= 1 }},
		{name: "tag", mutate: func(value *protocol.PruneBootstrap) { value.Ciphertext[len(value.Ciphertext)-1] ^= 1 }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneBootstrapCapsule(capsule)
			test.mutate(&candidate)
			if _, err := OpenPruneBootstrap(candidate, base); !errors.Is(err, ErrPruneBootstrapAuthenticationFailed) {
				t.Fatalf("OpenPruneBootstrap() error = %v, want %v", err, ErrPruneBootstrapAuthenticationFailed)
			}
		})
	}

	wrongProjectMaterial, err := NewPruneBootstrapKey("project-2", protocol.PruneBootstrapPurposeVersionV1, base.Bytes())
	if err != nil {
		t.Fatalf("relabel bootstrap key project: %v", err)
	}
	if _, err := OpenPruneBootstrap(capsule, wrongProjectMaterial); !errors.Is(err, ErrPruneBootstrapAuthenticationFailed) {
		t.Fatalf("wrong-project error = %v, want %v", err, ErrPruneBootstrapAuthenticationFailed)
	}
	contentKey, err := DeriveGenerationKey(testProjectRoot(t), "project-1", 7)
	if err != nil {
		t.Fatalf("derive content key: %v", err)
	}
	relabeledContentKey, err := NewPruneBootstrapKey("project-1", protocol.PruneBootstrapPurposeVersionV1, contentKey.Bytes())
	if err != nil {
		t.Fatalf("relabel content material: %v", err)
	}
	if _, err := OpenPruneBootstrap(capsule, relabeledContentKey); !errors.Is(err, ErrPruneBootstrapAuthenticationFailed) {
		t.Fatalf("typed-substitution error = %v, want %v", err, ErrPruneBootstrapAuthenticationFailed)
	}
	wrongProjectPlaintext := cloneBootstrapPlaintext(plaintext)
	wrongProjectPlaintext.ProjectID = "project-2"
	if _, err := SealPruneBootstrap(wrongProjectPlaintext, base); !errors.Is(err, ErrPruneBootstrapKeyBinding) {
		t.Fatalf("seal wrong-project error = %v, want %v", err, ErrPruneBootstrapKeyBinding)
	}
}

func TestOpenPruneBootstrapRejectsAuthenticatedOuterInnerMismatch(t *testing.T) {
	t.Parallel()

	base, err := DerivePruneBootstrapKey(testProjectRoot(t), "project-1", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive bootstrap key: %v", err)
	}
	inner := testBootstrapPlaintext()
	outer := bootstrapOuter(inner)
	outer.Nonce = controlNonce(0xb0)
	mutations := []struct {
		name   string
		mutate func(*protocol.PruneBootstrapPlaintext)
	}{
		{name: "project", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.ProjectID = "project-2" }},
		{name: "channel", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.ChannelID[0] ^= 1 }},
		{name: "relay", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.RelayGeneration[0] ^= 1 }},
		{name: "prune", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.PruneID[0] ^= 1 }},
		{name: "membership", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.MembershipGeneration++ }},
		{name: "barrier", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.BarrierArrivalSequence++ }},
		{name: "closure", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.ClosureReferenceDigest[0] ^= 1 }},
		{name: "manifest count", mutate: func(value *protocol.PruneBootstrapPlaintext) {
			value.ManifestCount++
			value.EntryCount++
			entry := value.Entries[len(value.Entries)-1]
			entry.PruneReferenceDigest[0] ^= 0x7f
			value.Entries = append(value.Entries, entry)
		}},
		{name: "manifest digest", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.ManifestDigest[0] ^= 1 }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneBootstrapPlaintext(inner)
			test.mutate(&candidate)
			encoded, err := candidate.MarshalBinary()
			if err != nil {
				t.Fatalf("marshal mismatched inner: %v", err)
			}
			sealed, err := sealPruneBootstrapEncoded(outer, encoded, base)
			if err != nil {
				t.Fatalf("seal authenticated mismatch: %v", err)
			}
			if _, err := OpenPruneBootstrap(sealed, base); !errors.Is(err, ErrPruneBootstrapPlaintextBinding) {
				t.Fatalf("OpenPruneBootstrap() error = %v, want %v", err, ErrPruneBootstrapPlaintextBinding)
			}
		})
	}

	selectorMismatches := []struct {
		name   string
		mutate func(*protocol.PruneBootstrapPlaintext)
	}{
		{name: "capsule version", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.CapsuleVersion++ }},
		{name: "protocol version", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.ProtocolVersion++ }},
		{name: "cipher suite", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.CipherSuite++ }},
		{name: "purpose version", mutate: func(value *protocol.PruneBootstrapPlaintext) { value.BootstrapPurposeVersion++ }},
	}
	for _, test := range selectorMismatches {
		candidate := cloneBootstrapPlaintext(inner)
		test.mutate(&candidate)
		if pruneBootstrapBindingsMatch(outer, candidate, base) {
			t.Fatalf("%s mismatch passed exact binding comparison", test.name)
		}
	}
}

func TestPruneBootstrapRandomFailureFreshNonceAndNoAliasing(t *testing.T) {
	t.Parallel()

	base, err := DerivePruneBootstrapKey(testProjectRoot(t), "project-1", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive bootstrap key: %v", err)
	}
	plaintext := testBootstrapPlaintext()
	if _, err := sealPruneBootstrapWithRandom(plaintext, base, failingBootstrapReader{}); !errors.Is(err, ErrRandomSource) {
		t.Fatalf("seal random-source error = %v, want %v", err, ErrRandomSource)
	}
	if _, err := sealPruneBootstrapWithRandom(plaintext, base, nil); !errors.Is(err, ErrRandomSource) {
		t.Fatalf("nil random-source error = %v, want %v", err, ErrRandomSource)
	}
	first, err := SealPruneBootstrap(plaintext, base)
	if err != nil {
		t.Fatalf("seal first bootstrap: %v", err)
	}
	second, err := SealPruneBootstrap(plaintext, base)
	if err != nil {
		t.Fatalf("seal second bootstrap: %v", err)
	}
	if first.Nonce == second.Nonce || bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("independent seals reused nonce or ciphertext")
	}
	firstSnapshot := append([]byte(nil), first.Ciphertext...)
	plaintext.Entries[0].HLC.WallMillis++
	if !bytes.Equal(first.Ciphertext, firstSnapshot) {
		t.Fatal("sealed ciphertext aliases plaintext input")
	}
	opened, err := OpenPruneBootstrap(first, base)
	if err != nil {
		t.Fatalf("open first bootstrap: %v", err)
	}
	opened.Entries[0].HLC.WallMillis++
	if !bytes.Equal(first.Ciphertext, firstSnapshot) {
		t.Fatal("opened plaintext aliases capsule ciphertext")
	}
}

func TestPruneBootstrapMaximumEntriesSealAndOpen(t *testing.T) {
	t.Parallel()

	plaintext := testBootstrapPlaintext()
	plaintext.Entries = make([]protocol.PruneBootstrapEntry, protocol.MaxPruneTargets)
	for index := range plaintext.Entries {
		plaintext.Entries[index] = protocol.PruneBootstrapEntry{
			PruneReferenceDigest: sha256.Sum256([]byte(fmt.Sprintf("reference-%04d", index))),
			FactKind:             continuity.FactScratchpadMessageRecorded,
			HLC:                  continuity.HybridTime{WallMillis: int64(index), Logical: int32(index)},
		}
	}
	plaintext.EntryCount = protocol.MaxPruneTargets
	plaintext.ManifestCount = protocol.MaxPruneTargets
	plaintext.ManifestDigest = sha256.Sum256([]byte("maximum manifest"))
	base, err := DerivePruneBootstrapKey(testProjectRoot(t), plaintext.ProjectID, protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive bootstrap key: %v", err)
	}
	capsule, err := SealPruneBootstrap(plaintext, base)
	if err != nil {
		t.Fatalf("seal maximum bootstrap: %v", err)
	}
	if len(capsule.Ciphertext) > protocol.MaxPruneBootstrapCiphertextBytes {
		t.Fatalf("maximum entries produced %d ciphertext bytes, limit %d", len(capsule.Ciphertext), protocol.MaxPruneBootstrapCiphertextBytes)
	}
	opened, err := OpenPruneBootstrap(capsule, base)
	if err != nil {
		t.Fatalf("open maximum bootstrap: %v", err)
	}
	if len(opened.Entries) != protocol.MaxPruneTargets {
		t.Fatalf("opened %d entries, want %d", len(opened.Entries), protocol.MaxPruneTargets)
	}
}

func TestPruneBootstrapErrorsAndFormattingAreSecretFree(t *testing.T) {
	t.Parallel()

	base, err := DerivePruneBootstrapKey(testProjectRoot(t), "project-1", protocol.PruneBootstrapPurposeVersionV1)
	if err != nil {
		t.Fatalf("derive bootstrap key: %v", err)
	}
	if strings.Contains(fmt.Sprint(base), fmt.Sprintf("%x", base.Bytes())) || strings.Contains(fmt.Sprintf("%#v", base), fmt.Sprintf("%x", base.Bytes())) {
		t.Fatal("bootstrap key formatting exposed material")
	}
	capsule, err := SealPruneBootstrap(testBootstrapPlaintext(), base)
	if err != nil {
		t.Fatalf("seal bootstrap: %v", err)
	}
	capsule.Ciphertext[0] ^= 1
	_, openErr := OpenPruneBootstrap(capsule, base)
	if openErr == nil {
		t.Fatal("tampered ciphertext error = nil")
	}
	keyBytes := base.Bytes()
	for _, forbidden := range [][]byte{keyBytes[:], []byte("project-1"), []byte("scratchpad-1")} {
		if bytes.Contains([]byte(openErr.Error()), forbidden) {
			t.Fatalf("error leaked protected bytes: %q", openErr)
		}
	}
}

func testBootstrapPlaintext() protocol.PruneBootstrapPlaintext {
	return protocol.PruneBootstrapPlaintext{
		CapsuleVersion:          protocol.PruneBootstrapCapsuleVersionV1,
		ProtocolVersion:         protocol.ProtocolVersionV1,
		CipherSuite:             protocol.CipherSuiteXChaCha20Poly1305,
		BootstrapPurposeVersion: protocol.PruneBootstrapPurposeVersionV1,
		ProjectID:               "project-1",
		ChannelID:               testUnsignedCertificate(protocol.PublicKey{1}).ChannelID,
		RelayGeneration:         controlRelayGeneration(),
		PruneID:                 controlDigest(0xc0),
		MembershipGeneration:    7,
		BarrierArrivalSequence:  19,
		ClosureReferenceDigest:  controlDigest(0xc1),
		ManifestCount:           2,
		ManifestDigest:          controlDigest(0xc2),
		ScratchpadSubject:       "scratchpad-1",
		EntryCount:              2,
		Entries: []protocol.PruneBootstrapEntry{
			{
				PruneReferenceDigest: controlDigest(0xd0),
				FactKind:             continuity.FactScratchpadMessageRecorded,
				HLC:                  continuity.HybridTime{WallMillis: 100, Logical: 2},
			},
			{
				PruneReferenceDigest: controlDigest(0xd1),
				FactKind:             continuity.FactScratchpadClaimReleased,
				HLC:                  continuity.HybridTime{WallMillis: 101, Logical: 0},
			},
		},
	}
}

func bootstrapOuter(plaintext protocol.PruneBootstrapPlaintext) protocol.PruneBootstrap {
	return protocol.PruneBootstrap{
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
	}
}

func cloneBootstrapPlaintext(value protocol.PruneBootstrapPlaintext) protocol.PruneBootstrapPlaintext {
	value.Entries = append([]protocol.PruneBootstrapEntry(nil), value.Entries...)
	return value
}

func cloneBootstrapCapsule(value protocol.PruneBootstrap) protocol.PruneBootstrap {
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	return value
}

type failingBootstrapReader struct{}

func (failingBootstrapReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
