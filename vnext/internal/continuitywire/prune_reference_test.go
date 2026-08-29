package continuitywire

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	pruneReferenceVectorHex = "0000001c6c6f61662e73796e632e7072756e652d7265666572656e63652e7631000000090000000b666163742d7072756e65640000000d656e7669726f6e6d656e742d6100000008000000000000000200000008000000000000000b00000020f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff000102030405060708090a0b0c0d0e0f00000020f4c096647d5e33bf9e9c33903f454a68bce748887ebc05a5c5be9f808e439ec400000020eff0f1f2f3f4f5f6f7f8f9fafbfcfdfeff000102030405060708090a0b0c0d0e000000040000000700000018f1f2f3f4f5f6f7f8f9fafbfcfdfeff000102030405060708"
	pruneReferenceDigestHex = "775aa0ca23c795b0aa15f5abbee41014fe59b2429a6fc030cb3ae600c149fcdd"
)

func TestPruneReferencePublishedVectorRoundTripAndDigest(t *testing.T) {
	t.Parallel()

	wantWire, err := hex.DecodeString(pruneReferenceVectorHex)
	if err != nil {
		t.Fatalf("decode wire vector: %v", err)
	}
	wantDigest, err := hex.DecodeString(pruneReferenceDigestHex)
	if err != nil {
		t.Fatalf("decode digest vector: %v", err)
	}
	reference := pruneReferenceVectorV1()
	wire, err := EncodePruneReference(reference)
	if err != nil {
		t.Fatalf("EncodePruneReference() error = %v", err)
	}
	if !bytes.Equal(wire, wantWire) {
		t.Fatalf("EncodePruneReference() = %x, want published %x", wire, wantWire)
	}
	digest, err := PruneReferenceDigest(reference)
	if err != nil {
		t.Fatalf("PruneReferenceDigest() error = %v", err)
	}
	if !bytes.Equal(digest[:], wantDigest) || digest != sha256.Sum256(wantWire) {
		t.Fatalf("PruneReferenceDigest() = %x, want published %x", digest, wantDigest)
	}
	decoded, err := DecodePruneReference(wire)
	if err != nil {
		t.Fatalf("DecodePruneReference() error = %v", err)
	}
	if decoded != reference {
		t.Fatalf("DecodePruneReference() = %#v, want %#v", decoded, reference)
	}
}

func TestPruneReferenceDecodeRejectsNoncanonicalAndOversizedWire(t *testing.T) {
	t.Parallel()

	canonical, err := hex.DecodeString(pruneReferenceVectorHex)
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}
	tests := []struct {
		name string
		wire []byte
		want error
	}{
		{name: "empty", wire: nil, want: ErrInvalidPruneReference},
		{name: "wrong domain", wire: append([]byte(nil), canonical...), want: ErrInvalidPruneReference},
		{name: "wrong field count", wire: append([]byte(nil), canonical...), want: ErrInvalidPruneReference},
		{name: "trailing byte", wire: append(append([]byte(nil), canonical...), 0), want: ErrInvalidPruneReference},
		{name: "oversized", wire: make([]byte, MaxPruneReferenceBytes+1), want: ErrPruneReferenceTooLarge},
	}
	tests[1].wire[4] ^= 1
	fieldCountOffset := 4 + len(pruneReferenceDomainV1)
	tests[2].wire[fieldCountOffset+3]--
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodePruneReference(test.wire); !errors.Is(err, test.want) {
				t.Fatalf("DecodePruneReference() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPruneReferenceValidationAndExactBound(t *testing.T) {
	t.Parallel()

	valid := pruneReferenceVectorV1()
	invalid := []struct {
		name   string
		mutate func(*PruneReference)
	}{
		{name: "fact id", mutate: func(value *PruneReference) { value.FactID = "bad id" }},
		{name: "environment id", mutate: func(value *PruneReference) { value.EnvironmentID = "" }},
		{name: "environment sequence", mutate: func(value *PruneReference) { value.EnvironmentSequence = 0 }},
		{name: "arrival sequence", mutate: func(value *PruneReference) { value.ArrivalSequence = 0 }},
		{name: "envelope digest", mutate: func(value *PruneReference) { value.EnvelopeDigest = [32]byte{} }},
		{name: "certificate id", mutate: func(value *PruneReference) { value.CertificateID = [32]byte{} }},
		{name: "first sequence predecessor", mutate: func(value *PruneReference) { value.EnvironmentSequence = 1 }},
		{name: "key generation", mutate: func(value *PruneReference) { value.KeyGeneration = 0 }},
	}
	for _, test := range invalid {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidPruneReference) {
				t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidPruneReference)
			}
			if _, err := EncodePruneReference(candidate); !errors.Is(err, ErrInvalidPruneReference) {
				t.Fatalf("EncodePruneReference() error = %v, want %v", err, ErrInvalidPruneReference)
			}
		})
	}

	maximum := valid
	maximum.FactID = continuity.FactID(strings.Repeat("f", 128))
	maximum.EnvironmentID = continuity.EnvironmentID(strings.Repeat("e", 128))
	wire, err := EncodePruneReference(maximum)
	if err != nil {
		t.Fatalf("EncodePruneReference(maximum) error = %v", err)
	}
	if got, want := len(wire), 468; got != want {
		t.Fatalf("maximum wire size = %d, want exact %d", got, want)
	}
	if len(wire) > MaxPruneReferenceBytes {
		t.Fatalf("maximum wire size = %d, limit %d", len(wire), MaxPruneReferenceBytes)
	}

	zeroNonce := valid
	zeroNonce.Nonce = [24]byte{}
	if err := zeroNonce.Validate(); err != nil {
		t.Fatalf("zero nonce Validate() error = %v", err)
	}
}

func TestPruneReferenceDecodeDoesNotAliasInput(t *testing.T) {
	t.Parallel()

	wire, err := hex.DecodeString(pruneReferenceVectorHex)
	if err != nil {
		t.Fatalf("decode vector: %v", err)
	}
	original := append([]byte(nil), wire...)
	decoded, err := DecodePruneReference(wire)
	if err != nil {
		t.Fatalf("DecodePruneReference() error = %v", err)
	}
	for index := range wire {
		wire[index] ^= 0xff
	}
	reencoded, err := EncodePruneReference(decoded)
	if err != nil {
		t.Fatalf("EncodePruneReference(decoded) error = %v", err)
	}
	if !bytes.Equal(reencoded, original) {
		t.Fatal("decoded reference aliases input wire")
	}
}

func pruneReferenceVectorV1() PruneReference {
	var envelopeDigest, certificateID, previousEnvelopeDigest [32]byte
	for index := range envelopeDigest {
		envelopeDigest[index] = byte(0xf0 + index)
		previousEnvelopeDigest[index] = byte(0xef + index)
	}
	certificateBytes, err := hex.DecodeString("f4c096647d5e33bf9e9c33903f454a68bce748887ebc05a5c5be9f808e439ec4")
	if err != nil {
		panic(err)
	}
	copy(certificateID[:], certificateBytes)
	var nonce [24]byte
	for index := range nonce {
		nonce[index] = byte(0xf1 + index)
	}
	return PruneReference{
		FactID:                 "fact-pruned",
		EnvironmentID:          "environment-a",
		EnvironmentSequence:    2,
		ArrivalSequence:        11,
		EnvelopeDigest:         envelopeDigest,
		CertificateID:          certificateID,
		PreviousEnvelopeDigest: previousEnvelopeDigest,
		KeyGeneration:          7,
		Nonce:                  nonce,
	}
}
