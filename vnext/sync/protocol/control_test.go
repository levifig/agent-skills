package protocol

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestProgressAcknowledgementCanonicalRoundTripAndDigest(t *testing.T) {
	t.Parallel()

	acknowledgement := testProgressAcknowledgement("environment-a", 0x10)
	wire, err := acknowledgement.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal progress acknowledgement: %v", err)
	}
	decoded, err := ParseProgressAcknowledgement(wire)
	if err != nil {
		t.Fatalf("parse progress acknowledgement: %v", err)
	}
	decodedWire, err := decoded.MarshalBinary()
	if err != nil {
		t.Fatalf("remarshal progress acknowledgement: %v", err)
	}
	if !bytes.Equal(decodedWire, wire) {
		t.Fatal("progress acknowledgement changed across canonical round trip")
	}
	if got, want := ProgressAcknowledgementDigest(decoded), ProgressAcknowledgementDigest(acknowledgement); got != want || got == (Digest{}) {
		t.Fatalf("progress acknowledgement digest = %x, want nonzero %x", got, want)
	}

	trailing := append(append([]byte(nil), wire...), 0)
	if _, err := ParseProgressAcknowledgement(trailing); !errors.Is(err, ErrInvalidAcknowledgement) {
		t.Fatalf("trailing-byte error = %v, want %v", err, ErrInvalidAcknowledgement)
	}
}

func TestPruneAcknowledgementCanonicalRoundTripAndBindings(t *testing.T) {
	t.Parallel()

	acknowledgement := testPruneAcknowledgement("environment-a", 0x20)
	body, err := PruneAcknowledgementBodyTranscript(acknowledgement)
	if err != nil {
		t.Fatalf("build prune acknowledgement body: %v", err)
	}
	wire, err := acknowledgement.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune acknowledgement: %v", err)
	}
	decoded, err := ParsePruneAcknowledgement(wire)
	if err != nil {
		t.Fatalf("parse prune acknowledgement: %v", err)
	}
	decodedBody, err := PruneAcknowledgementBodyTranscript(decoded)
	if err != nil {
		t.Fatalf("rebuild prune acknowledgement body: %v", err)
	}
	if !bytes.Equal(decodedBody, body) || PruneAcknowledgementDigest(decoded) != PruneAcknowledgementDigest(acknowledgement) {
		t.Fatal("prune acknowledgement transcript or digest changed across round trip")
	}

	mutated := acknowledgement
	mutated.ManifestDigest[0] ^= 0xff
	mutatedBody, err := PruneAcknowledgementBodyTranscript(mutated)
	if err != nil {
		t.Fatalf("build mutated prune acknowledgement body: %v", err)
	}
	if bytes.Equal(mutatedBody, body) {
		t.Fatal("manifest digest mutation did not change signed transcript")
	}
}

func TestTerminalRetirementCanonicalRoundTripAndIdentifier(t *testing.T) {
	t.Parallel()

	retirement := testTerminalRetirement(0x30)
	wire, err := retirement.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal terminal retirement: %v", err)
	}
	decoded, err := ParseTerminalRetirement(wire)
	if err != nil {
		t.Fatalf("parse terminal retirement: %v", err)
	}
	if got, want := TerminalRetirementID(decoded), TerminalRetirementID(retirement); got != want || got == (Digest{}) {
		t.Fatalf("terminal retirement id = %x, want nonzero %x", got, want)
	}

	empty := retirement
	empty.FinalEnvironmentSequence = 0
	empty.FinalEnvelopeDigest = Digest{}
	if _, err := empty.MarshalBinary(); err != nil {
		t.Fatalf("marshal empty-producer retirement: %v", err)
	}
}

func TestPruneReferenceManifestAndCertificateCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	certificate := testPruneCertificate()
	referenceWire, err := certificate.Closure.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune closure reference: %v", err)
	}
	reference, err := ParsePruneReference(referenceWire)
	if err != nil {
		t.Fatalf("parse prune closure reference: %v", err)
	}
	if reference != certificate.Closure || PruneReferenceDigest(reference) != certificate.ClosureDigest {
		t.Fatal("prune closure reference changed across round trip")
	}

	manifestWire, err := certificate.Manifest.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune manifest: %v", err)
	}
	manifest, err := ParsePruneManifest(manifestWire)
	if err != nil {
		t.Fatalf("parse prune manifest: %v", err)
	}
	if PruneManifestDigest(manifest) != certificate.ManifestDigest {
		t.Fatal("prune manifest digest changed across round trip")
	}

	wire, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune certificate: %v", err)
	}
	decoded, err := ParsePruneCertificate(wire)
	if err != nil {
		t.Fatalf("parse prune certificate: %v", err)
	}
	decodedWire, err := decoded.MarshalBinary()
	if err != nil {
		t.Fatalf("remarshal prune certificate: %v", err)
	}
	if !bytes.Equal(decodedWire, wire) || PruneCertificateID(decoded) != PruneCertificateID(certificate) {
		t.Fatal("prune certificate changed across canonical round trip")
	}
}

func TestControlValidationRejectsNoncanonicalSentinelsAndOrdering(t *testing.T) {
	t.Parallel()

	progress := testProgressAcknowledgement("environment-a", 0x40)
	progress.ProducerSequence = 0
	if err := progress.Validate(); !errors.Is(err, ErrInvalidAcknowledgement) {
		t.Fatalf("zero producer with digest error = %v, want %v", err, ErrInvalidAcknowledgement)
	}

	pruneAcknowledgement := testPruneAcknowledgement("environment-a", 0x41)
	pruneAcknowledgement.AppliedArrivalSequence = pruneAcknowledgement.BarrierArrivalSequence - 1
	if err := pruneAcknowledgement.Validate(); !errors.Is(err, ErrInvalidPruneAcknowledgement) {
		t.Fatalf("prune acknowledgement before barrier error = %v, want %v", err, ErrInvalidPruneAcknowledgement)
	}

	retirement := testTerminalRetirement(0x42)
	retirement.FinalEnvelopeDigest = Digest{}
	if err := retirement.Validate(); !errors.Is(err, ErrInvalidRetirement) {
		t.Fatalf("nonempty retirement with zero digest error = %v, want %v", err, ErrInvalidRetirement)
	}

	manifest := testPruneCertificate().Manifest
	manifest.Targets[0], manifest.Targets[1] = manifest.Targets[1], manifest.Targets[0]
	if err := manifest.Validate(); !errors.Is(err, ErrInvalidPruneManifest) {
		t.Fatalf("unsorted manifest error = %v, want %v", err, ErrInvalidPruneManifest)
	}

	certificate := testPruneCertificate()
	certificate.Acknowledgements[0], certificate.Acknowledgements[1] = certificate.Acknowledgements[1], certificate.Acknowledgements[0]
	if err := certificate.Validate(); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("unsorted acknowledgement set error = %v, want %v", err, ErrInvalidPruneCertificate)
	}
}

func TestControlParsersEnforceHardBounds(t *testing.T) {
	t.Parallel()

	if _, err := ParseProgressAcknowledgement(make([]byte, MaxProgressAcknowledgementBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize progress acknowledgement error = %v, want %v", err, ErrTooLarge)
	}
	if _, err := ParsePruneAcknowledgement(make([]byte, MaxPruneAcknowledgementBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize prune acknowledgement error = %v, want %v", err, ErrTooLarge)
	}
	if _, err := ParseTerminalRetirement(make([]byte, MaxTerminalRetirementBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize terminal retirement error = %v, want %v", err, ErrTooLarge)
	}
	if _, err := ParsePruneReference(make([]byte, MaxPruneReferenceBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize prune reference error = %v, want %v", err, ErrTooLarge)
	}
	if _, err := ParsePruneCertificate(make([]byte, MaxControlObjectBytes+1)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize prune certificate error = %v, want %v", err, ErrTooLarge)
	}
}

func TestControlBodyTranscriptsExcludeOnlyTheSignature(t *testing.T) {
	t.Parallel()

	progress := testProgressAcknowledgement("environment-a", 0x43)
	progressBody, err := ProgressAcknowledgementBodyTranscript(progress)
	if err != nil {
		t.Fatalf("build progress acknowledgement body: %v", err)
	}
	mutatedProgress := progress
	mutatedProgress.EnvironmentSignature[0] ^= 0xff
	mutatedProgressBody, err := ProgressAcknowledgementBodyTranscript(mutatedProgress)
	if err != nil {
		t.Fatalf("build progress acknowledgement body after signature mutation: %v", err)
	}
	if !bytes.Equal(progressBody, mutatedProgressBody) || ProgressAcknowledgementDigest(progress) == ProgressAcknowledgementDigest(mutatedProgress) {
		t.Fatal("progress body included its signature or full digest omitted it")
	}

	retirement := testTerminalRetirement(0x44)
	retirementBody, err := TerminalRetirementBodyTranscript(retirement)
	if err != nil {
		t.Fatalf("build terminal retirement body: %v", err)
	}
	mutatedRetirement := retirement
	mutatedRetirement.AdminSignature[0] ^= 0xff
	mutatedRetirementBody, err := TerminalRetirementBodyTranscript(mutatedRetirement)
	if err != nil {
		t.Fatalf("build terminal retirement body after signature mutation: %v", err)
	}
	if !bytes.Equal(retirementBody, mutatedRetirementBody) || TerminalRetirementID(retirement) == TerminalRetirementID(mutatedRetirement) {
		t.Fatal("retirement body included its signature or full identifier omitted it")
	}

	certificate := testPruneCertificate()
	certificateBody, err := PruneCertificateBodyTranscript(certificate)
	if err != nil {
		t.Fatalf("build prune certificate body: %v", err)
	}
	mutatedCertificate := certificate
	mutatedCertificate.AdminSignature[0] ^= 0xff
	mutatedCertificateBody, err := PruneCertificateBodyTranscript(mutatedCertificate)
	if err != nil {
		t.Fatalf("build prune certificate body after signature mutation: %v", err)
	}
	if !bytes.Equal(certificateBody, mutatedCertificateBody) || PruneCertificateID(certificate) == PruneCertificateID(mutatedCertificate) {
		t.Fatal("prune certificate body included its signature or full identifier omitted it")
	}
}

func TestPruneReferenceBoundAccommodatesMaximumContinuityIdentifiers(t *testing.T) {
	t.Parallel()

	reference := testPruneReference(
		continuity.FactID(strings.Repeat("f", 128)),
		continuity.EnvironmentID(strings.Repeat("e", 128)),
		1,
		1,
		0x45,
	)
	wire, err := reference.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal maximum-identifier prune reference: %v", err)
	}
	if len(wire) > MaxPruneReferenceBytes {
		t.Fatalf("maximum-identifier reference encoded to %d bytes, limit %d", len(wire), MaxPruneReferenceBytes)
	}
	if _, err := ParsePruneReference(wire); err != nil {
		t.Fatalf("parse maximum-identifier prune reference: %v", err)
	}
}

func testProgressAcknowledgement(environmentID continuity.EnvironmentID, seed byte) ProgressAcknowledgement {
	return ProgressAcknowledgement{
		Version:                ControlVersionV1,
		ProtocolVersion:        ProtocolVersionV1,
		CipherSuite:            CipherSuiteXChaCha20Poly1305,
		ChannelID:              testControlChannelID(),
		RelayGeneration:        testControlRelayGeneration(),
		EnvironmentID:          environmentID,
		CertificateID:          testControlDigest(seed),
		MembershipGeneration:   7,
		AppliedArrivalSequence: 19,
		ProducerSequence:       3,
		ProducerEnvelopeDigest: testControlDigest(seed + 1),
		EnvironmentSignature:   testControlSignature(seed + 2),
	}
}

func testPruneAcknowledgement(environmentID continuity.EnvironmentID, seed byte) PruneAcknowledgement {
	progress := testProgressAcknowledgement(environmentID, seed)
	return PruneAcknowledgement{
		Version:                       ControlVersionV1,
		ProtocolVersion:               ProtocolVersionV1,
		CipherSuite:                   CipherSuiteXChaCha20Poly1305,
		ChannelID:                     progress.ChannelID,
		RelayGeneration:               progress.RelayGeneration,
		EnvironmentID:                 environmentID,
		CertificateID:                 progress.CertificateID,
		MembershipGeneration:          progress.MembershipGeneration,
		ProgressAcknowledgementDigest: ProgressAcknowledgementDigest(progress),
		AppliedArrivalSequence:        progress.AppliedArrivalSequence,
		ProducerSequence:              progress.ProducerSequence,
		ProducerEnvelopeDigest:        progress.ProducerEnvelopeDigest,
		PruneID:                       testControlDigest(0xa0),
		BarrierArrivalSequence:        17,
		ClosureReferenceDigest:        testControlDigest(0xa1),
		ManifestCount:                 2,
		ManifestDigest:                testControlDigest(0xa2),
		EnvironmentSignature:          testControlSignature(seed + 3),
	}
}

func testTerminalRetirement(seed byte) TerminalRetirement {
	return TerminalRetirement{
		Version:                  ControlVersionV1,
		ProtocolVersion:          ProtocolVersionV1,
		CipherSuite:              CipherSuiteXChaCha20Poly1305,
		ChannelID:                testControlChannelID(),
		RelayGeneration:          testControlRelayGeneration(),
		EnvironmentID:            "environment-retired",
		CertificateID:            testControlDigest(seed),
		MembershipGeneration:     8,
		FinalEnvironmentSequence: 3,
		FinalEnvelopeDigest:      testControlDigest(seed + 1),
		AdminSignature:           testControlSignature(seed + 2),
	}
}

func testPruneCertificate() PruneCertificate {
	targets := []PruneReference{
		testPruneReference("fact-a", "environment-a", 1, 4, 0x51),
		testPruneReference("fact-b", "environment-b", 2, 9, 0x61),
	}
	manifest := PruneManifest{Targets: targets}
	closure := testPruneReference("fact-close", "environment-a", 3, 12, 0x71)
	manifestDigest := PruneManifestDigest(manifest)
	closureDigest := PruneReferenceDigest(closure)
	first := testPruneAcknowledgement("environment-a", 0x81)
	second := testPruneAcknowledgement("environment-b", 0x91)
	for _, acknowledgement := range []*PruneAcknowledgement{&first, &second} {
		acknowledgement.PruneID = testControlDigest(0xa0)
		acknowledgement.BarrierArrivalSequence = 17
		acknowledgement.ClosureReferenceDigest = closureDigest
		acknowledgement.ManifestCount = uint32(len(targets))
		acknowledgement.ManifestDigest = manifestDigest
	}
	return PruneCertificate{
		Version:                    ControlVersionV1,
		ProtocolVersion:            ProtocolVersionV1,
		CipherSuite:                CipherSuiteXChaCha20Poly1305,
		ChannelID:                  testControlChannelID(),
		RelayGeneration:            testControlRelayGeneration(),
		PruneID:                    testControlDigest(0xa0),
		MembershipGeneration:       7,
		BarrierArrivalSequence:     17,
		Closure:                    closure,
		ClosureDigest:              closureDigest,
		ManifestCount:              uint32(len(targets)),
		ManifestDigest:             manifestDigest,
		Manifest:                   manifest,
		ActiveAcknowledgementCount: 2,
		Acknowledgements:           []PruneAcknowledgement{first, second},
		AdminSignature:             testControlSignature(0xb0),
	}
}

func testPruneReference(factID continuity.FactID, environmentID continuity.EnvironmentID, environmentSequence, arrivalSequence int64, seed byte) PruneReference {
	return PruneReference{
		FactID:              factID,
		EnvironmentID:       environmentID,
		EnvironmentSequence: environmentSequence,
		ArrivalSequence:     arrivalSequence,
		EnvelopeDigest:      testControlDigest(seed),
		CertificateID:       testControlDigest(seed + 1),
	}
}

func testControlChannelID() ChannelID {
	var value ChannelID
	for index := range value {
		value[index] = byte(index + 1)
	}
	return value
}

func testControlRelayGeneration() RelayGeneration {
	var value RelayGeneration
	for index := range value {
		value[index] = byte(0x21 + index)
	}
	return value
}

func testControlDigest(seed byte) Digest {
	var value Digest
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}

func testControlSignature(seed byte) Signature {
	var value Signature
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}
