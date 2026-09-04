package protocol

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSealedFactBinaryRoundTripPinsHeaderAndDigest(t *testing.T) {
	t.Parallel()

	envelope := testSealedFact()
	wire, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal sealed fact: %v", err)
	}
	decoded, err := ParseSealedFact(wire)
	if err != nil {
		t.Fatalf("parse sealed fact: %v", err)
	}
	if relation := CompareImmutable(envelope, decoded); relation != ImmutableExactReplay {
		t.Fatalf("round-trip relation = %v, want exact replay", relation)
	}
	if got := EnvelopeDigest(envelope); got != EnvelopeDigest(decoded) {
		t.Fatalf("round-trip digest changed: %x != %x", got, EnvelopeDigest(decoded))
	}

	trailing := append(append([]byte(nil), wire...), 0)
	if _, err := ParseSealedFact(trailing); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("trailing-byte error = %v, want %v", err, ErrInvalidEnvelope)
	}
}

func TestAADTranscriptIsUnambiguousAndBindsEveryHeaderField(t *testing.T) {
	t.Parallel()

	header := testSealedFact().Header
	baseline, err := AADTranscript(header)
	if err != nil {
		t.Fatalf("build baseline AAD: %v", err)
	}

	mutations := []struct {
		name   string
		mutate func(*FactHeader)
	}{
		{name: "protocol version", mutate: func(value *FactHeader) { value.ProtocolVersion++ }},
		{name: "suite", mutate: func(value *FactHeader) { value.CipherSuite++ }},
		{name: "channel", mutate: func(value *FactHeader) { value.ChannelID[0]++ }},
		{name: "fact", mutate: func(value *FactHeader) { value.FactID += "x" }},
		{name: "environment", mutate: func(value *FactHeader) { value.EnvironmentID += "x" }},
		{name: "sequence", mutate: func(value *FactHeader) { value.EnvironmentSequence++ }},
		{name: "generation", mutate: func(value *FactHeader) { value.KeyGeneration++ }},
		{name: "previous digest", mutate: func(value *FactHeader) { value.PreviousEnvelopeDigest[0]++ }},
		{name: "certificate", mutate: func(value *FactHeader) { value.CertificateID[0]++ }},
		{name: "nonce", mutate: func(value *FactHeader) { value.Nonce[0]++ }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mutated := header
			test.mutate(&mutated)
			got, err := aadTranscriptUnchecked(mutated)
			if err != nil {
				t.Fatalf("build mutated AAD: %v", err)
			}
			if bytes.Equal(got, baseline) {
				t.Fatal("mutating a bound header field did not change AAD")
			}
		})
	}

	left := header
	left.FactID = continuity.FactID("fact_a")
	left.EnvironmentID = continuity.EnvironmentID("environment_bc")
	right := header
	right.FactID = continuity.FactID("fact_ab")
	right.EnvironmentID = continuity.EnvironmentID("environment_c")
	leftAAD, err := AADTranscript(left)
	if err != nil {
		t.Fatalf("build left AAD: %v", err)
	}
	rightAAD, err := AADTranscript(right)
	if err != nil {
		t.Fatalf("build right AAD: %v", err)
	}
	if bytes.Equal(leftAAD, rightAAD) {
		t.Fatal("length-shifted fields produced the same AAD")
	}
}

func TestCertificateBinaryRoundTripAndIdentifier(t *testing.T) {
	t.Parallel()

	certificate := testCertificate()
	wire, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal certificate: %v", err)
	}
	decoded, err := ParseEnvironmentCertificate(wire)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	decodedWire, err := decoded.MarshalBinary()
	if err != nil {
		t.Fatalf("remarshal certificate: %v", err)
	}
	if !bytes.Equal(wire, decodedWire) {
		t.Fatalf("certificate bytes changed:\n got %x\nwant %x", decodedWire, wire)
	}
	if CertificateID(certificate) != CertificateID(decoded) {
		t.Fatal("certificate identifier changed across round trip")
	}
}

func TestCertificateRejectsNoncanonicalGenerationSet(t *testing.T) {
	t.Parallel()

	for _, generations := range [][]uint32{{}, {2, 1}, {1, 1}} {
		certificate := testCertificate()
		certificate.AllowedKeyGenerations = generations
		if err := certificate.Validate(); !errors.Is(err, ErrInvalidCertificate) {
			t.Fatalf("generations %v: error = %v, want %v", generations, err, ErrInvalidCertificate)
		}
	}
}

func TestImmutableComparisonDistinguishesReplayAndConflict(t *testing.T) {
	t.Parallel()

	first := testSealedFact()
	if got := CompareImmutable(first, first); got != ImmutableExactReplay {
		t.Fatalf("same envelope relation = %v, want exact replay", got)
	}

	sameFactDifferentBytes := first
	sameFactDifferentBytes.Ciphertext = append([]byte(nil), first.Ciphertext...)
	sameFactDifferentBytes.Ciphertext[0] ^= 0xff
	if got := CompareImmutable(first, sameFactDifferentBytes); got != ImmutableConflict {
		t.Fatalf("same-fact relation = %v, want conflict", got)
	}

	sameSequenceDifferentFact := first
	sameSequenceDifferentFact.Header.FactID = "fact_22222222222222222222222222222222"
	if got := CompareImmutable(first, sameSequenceDifferentFact); got != ImmutableConflict {
		t.Fatalf("same-sequence relation = %v, want conflict", got)
	}

	distinct := first
	distinct.Header.FactID = "fact_33333333333333333333333333333333"
	distinct.Header.EnvironmentSequence++
	if got := CompareImmutable(first, distinct); got != ImmutableDistinct {
		t.Fatalf("distinct relation = %v, want distinct", got)
	}
}

func TestImmutableComparisonNeverAcceptsInvalidZeroDigestReplay(t *testing.T) {
	t.Parallel()

	left := testSealedFact()
	left.Ciphertext = nil
	right := left
	if EnvelopeDigest(left) != (Digest{}) || EnvelopeDigest(right) != (Digest{}) {
		t.Fatal("invalid fixtures unexpectedly produced a digest")
	}
	if got := CompareImmutable(left, right); got != ImmutableConflict {
		t.Fatalf("invalid same-ID relation = %v, want conflict", got)
	}
}

func TestCryptoProtocolTranscriptVector(t *testing.T) {
	t.Parallel()

	envelope := testSealedFact()
	aad, err := AADTranscript(envelope.Header)
	if err != nil {
		t.Fatalf("build AAD: %v", err)
	}
	signature, err := FactSignatureTranscript(envelope.Header, envelope.Ciphertext)
	if err != nil {
		t.Fatalf("build signature transcript: %v", err)
	}
	wire, err := envelope.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	assertHex := func(name string, value []byte, want string) {
		t.Helper()
		if got := hex.EncodeToString(value); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	assertHex("aad", aad, protocolVectorAADHex)
	assertHex("signature transcript", signature, protocolVectorSignatureHex)
	assertHex("envelope", wire, protocolVectorEnvelopeHex)
	digest := EnvelopeDigest(envelope)
	assertHex("envelope digest", digest[:], protocolVectorDigestHex)
}

func testSealedFact() SealedFact {
	var channel ChannelID
	for index := range channel {
		channel[index] = byte(index)
	}
	var previous Digest
	for index := range previous {
		previous[index] = byte(0x20 + index)
	}
	var certificateID Digest
	for index := range certificateID {
		certificateID[index] = byte(0x40 + index)
	}
	var nonce Nonce
	for index := range nonce {
		nonce[index] = byte(0x60 + index)
	}
	var signature Signature
	for index := range signature {
		signature[index] = byte(0x80 + index)
	}
	return SealedFact{
		Header: FactHeader{
			ProtocolVersion:        ProtocolVersionV1,
			CipherSuite:            CipherSuiteXChaCha20Poly1305,
			ChannelID:              channel,
			FactID:                 "fact_11111111111111111111111111111111",
			EnvironmentID:          "environment_11111111111111111111111111111111",
			EnvironmentSequence:    2,
			KeyGeneration:          7,
			PreviousEnvelopeDigest: previous,
			CertificateID:          certificateID,
			Nonce:                  nonce,
		},
		Ciphertext: []byte{
			0xde, 0xad, 0xbe, 0xef, 0x00, 0x01, 0x02, 0x03,
			0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b,
			0x0c, 0x0d, 0x0e, 0x0f,
		},
		Signature: signature,
	}
}

func testCertificate() EnvironmentCertificate {
	var channel ChannelID
	for index := range channel {
		channel[index] = byte(index)
	}
	var publicKey PublicKey
	for index := range publicKey {
		publicKey[index] = byte(0x20 + index)
	}
	var signature Signature
	for index := range signature {
		signature[index] = byte(0x40 + index)
	}
	return EnvironmentCertificate{
		Version:               CertificateVersionV1,
		ProtocolVersion:       ProtocolVersionV1,
		CipherSuite:           CipherSuiteXChaCha20Poly1305,
		ProjectID:             "project_11111111111111111111111111111111",
		ChannelID:             channel,
		EnvironmentID:         "environment_11111111111111111111111111111111",
		EnvironmentPublicKey:  publicKey,
		Mode:                  EnvironmentTrusted,
		MembershipGeneration:  3,
		AllowedKeyGenerations: []uint32{6, 7},
		ExpiresAtMillis:       0,
		AdminSignature:        signature,
	}
}

const (
	protocolVectorAADHex       = "000000156c6f61662e73796e632e666163742e6161642e76310000000a00000002000100000002000100000020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f00000025666163745f31313131313131313131313131313131313131313131313131313131313131310000002c656e7669726f6e6d656e745f3131313131313131313131313131313131313131313131313131313131313131000000080000000000000002000000040000000700000020202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f00000020404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f00000018606162636465666768696a6b6c6d6e6f7071727374757677"
	protocolVectorSignatureHex = "0000001b6c6f61662e73796e632e666163742e7369676e61747572652e76310000000b00000002000100000002000100000020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f00000025666163745f31313131313131313131313131313131313131313131313131313131313131310000002c656e7669726f6e6d656e745f3131313131313131313131313131313131313131313131313131313131313131000000080000000000000002000000040000000700000020202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f00000020404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f00000018606162636465666768696a6b6c6d6e6f707172737475767700000014deadbeef000102030405060708090a0b0c0d0e0f"
	protocolVectorEnvelopeHex  = "0000001a6c6f61662e73796e632e666163742e656e76656c6f70652e76310000000c00000002000100000002000100000020000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f00000025666163745f31313131313131313131313131313131313131313131313131313131313131310000002c656e7669726f6e6d656e745f3131313131313131313131313131313131313131313131313131313131313131000000080000000000000002000000040000000700000020202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f00000020404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c5d5e5f00000018606162636465666768696a6b6c6d6e6f707172737475767700000014deadbeef000102030405060708090a0b0c0d0e0f00000040808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9fa0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebf"
	protocolVectorDigestHex    = "1143c371bc2c7c82f6298bfbb71d7c94e85715f6bc21dac02ff8c2ecec2ff770"
)
