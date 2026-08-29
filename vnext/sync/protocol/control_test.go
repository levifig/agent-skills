package protocol

import (
	"bytes"
	"errors"
	"fmt"
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

	mutated = acknowledgement
	mutated.CapsuleDigest[0] ^= 0xff
	mutatedBody, err = PruneAcknowledgementBodyTranscript(mutated)
	if err != nil {
		t.Fatalf("build capsule-mutated prune acknowledgement body: %v", err)
	}
	if bytes.Equal(mutatedBody, body) {
		t.Fatal("capsule digest mutation did not change signed transcript")
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

	ciphertextOffset := bytes.Index(wire, certificate.Capsule.Ciphertext)
	if ciphertextOffset < 0 {
		t.Fatal("encoded prune certificate does not contain its capsule ciphertext")
	}
	wire[ciphertextOffset] ^= 1
	if !bytes.Equal(decoded.Capsule.Ciphertext, certificate.Capsule.Ciphertext) {
		t.Fatal("parsed prune certificate capsule aliases its input wire")
	}
	decoded.Capsule.Ciphertext[0] ^= 1
	if bytes.Equal(decoded.Capsule.Ciphertext, certificate.Capsule.Ciphertext) {
		t.Fatal("parsed prune certificate capsule aliases its source value")
	}
}

func TestPruneCertificateRequiresExactCapsuleBindingsAndWitnessAgreement(t *testing.T) {
	t.Parallel()

	valid := testPruneCertificate()
	mutations := []struct {
		name   string
		mutate func(*PruneBootstrap)
	}{
		{name: "capsule version", mutate: func(value *PruneBootstrap) { value.CapsuleVersion++ }},
		{name: "protocol version", mutate: func(value *PruneBootstrap) { value.ProtocolVersion++ }},
		{name: "cipher suite", mutate: func(value *PruneBootstrap) { value.CipherSuite++ }},
		{name: "bootstrap purpose", mutate: func(value *PruneBootstrap) { value.BootstrapPurposeVersion++ }},
		{name: "channel", mutate: func(value *PruneBootstrap) { value.ChannelID[0] ^= 1 }},
		{name: "relay generation", mutate: func(value *PruneBootstrap) { value.RelayGeneration[0] ^= 1 }},
		{name: "prune", mutate: func(value *PruneBootstrap) { value.PruneID[0] ^= 1 }},
		{name: "membership generation", mutate: func(value *PruneBootstrap) { value.MembershipGeneration++ }},
		{name: "barrier", mutate: func(value *PruneBootstrap) { value.BarrierArrivalSequence++ }},
		{name: "closure digest", mutate: func(value *PruneBootstrap) { value.ClosureReferenceDigest[0] ^= 1 }},
		{name: "manifest count", mutate: func(value *PruneBootstrap) { value.ManifestCount++ }},
		{name: "manifest digest", mutate: func(value *PruneBootstrap) { value.ManifestDigest[0] ^= 1 }},
	}
	for _, test := range mutations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneTestPruneCertificate(valid)
			test.mutate(&candidate.Capsule)
			candidate.CapsuleDigest = PruneBootstrapDigest(candidate.Capsule)
			for index := range candidate.Acknowledgements {
				candidate.Acknowledgements[index].CapsuleDigest = candidate.CapsuleDigest
			}
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidPruneCertificate) {
				t.Fatalf("capsule binding error = %v, want %v", err, ErrInvalidPruneCertificate)
			}
		})
	}

	capsuleTamper := cloneTestPruneCertificate(valid)
	capsuleTamper.Capsule.Ciphertext[0] ^= 1
	if err := capsuleTamper.Validate(); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("capsule digest mismatch error = %v, want %v", err, ErrInvalidPruneCertificate)
	}
	digestTamper := cloneTestPruneCertificate(valid)
	digestTamper.CapsuleDigest[0] ^= 1
	if err := digestTamper.Validate(); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("capsule digest tamper error = %v, want %v", err, ErrInvalidPruneCertificate)
	}

	witnessMismatch := cloneTestPruneCertificate(valid)
	witnessMismatch.Acknowledgements[0].CapsuleDigest[0] ^= 1
	if err := witnessMismatch.Validate(); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("witness capsule disagreement error = %v, want %v", err, ErrInvalidPruneCertificate)
	}

	zeroDigest := cloneTestPruneCertificate(valid)
	zeroDigest.CapsuleDigest = Digest{}
	if err := zeroDigest.Validate(); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("zero capsule digest error = %v, want %v", err, ErrInvalidPruneCertificate)
	}

	oversizedCapsule := cloneTestPruneCertificate(valid)
	oversizedCapsule.Capsule.Ciphertext = make([]byte, MaxPruneBootstrapCiphertextBytes+1)
	if err := oversizedCapsule.Validate(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized capsule error = %v, want %v", err, ErrTooLarge)
	}
}

func TestPruneControlV1RejectsPreCapsuleFieldCounts(t *testing.T) {
	t.Parallel()

	acknowledgement := testPruneAcknowledgement("environment-legacy", 0x46)
	legacyAcknowledgementFields := append([][]byte(nil), pruneAcknowledgementBodyFields(acknowledgement)[:17]...)
	legacyAcknowledgementFields = append(legacyAcknowledgementFields, acknowledgement.EnvironmentSignature[:])
	legacyAcknowledgement, err := encodeTranscript(pruneAcknowledgementDomain, legacyAcknowledgementFields...)
	if err != nil {
		t.Fatalf("encode legacy prune acknowledgement: %v", err)
	}
	if _, err := ParsePruneAcknowledgement(legacyAcknowledgement); !errors.Is(err, ErrInvalidPruneAcknowledgement) {
		t.Fatalf("legacy prune acknowledgement error = %v, want %v", err, ErrInvalidPruneAcknowledgement)
	}

	certificate := testPruneCertificate()
	certificateFields, err := pruneCertificateBodyFields(certificate)
	if err != nil {
		t.Fatalf("build current prune certificate fields: %v", err)
	}
	legacyCertificateFields := append([][]byte(nil), certificateFields[:13]...)
	legacyCertificateFields = append(legacyCertificateFields, certificateFields[15:]...)
	legacyCertificateFields = append(legacyCertificateFields, certificate.AdminSignature[:])
	legacyCertificate, err := encodeTranscript(pruneCertificateDomain, legacyCertificateFields...)
	if err != nil {
		t.Fatalf("encode legacy prune certificate: %v", err)
	}
	if _, err := ParsePruneCertificate(legacyCertificate); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("legacy prune certificate error = %v, want %v", err, ErrInvalidPruneCertificate)
	}

	malformedNestedFields := append([][]byte(nil), certificateFields...)
	malformedNestedFields[14] = append(append([]byte(nil), malformedNestedFields[14]...), 0)
	malformedNestedFields = append(malformedNestedFields, certificate.AdminSignature[:])
	malformedNested, err := encodeTranscript(pruneCertificateDomain, malformedNestedFields...)
	if err != nil {
		t.Fatalf("encode malformed nested prune capsule: %v", err)
	}
	if _, err := ParsePruneCertificate(malformedNested); !errors.Is(err, ErrInvalidPruneCertificate) {
		t.Fatalf("malformed nested capsule error = %v, want %v", err, ErrInvalidPruneCertificate)
	}
}

func TestPruneReferenceV1RejectsLegacySixFieldEncoding(t *testing.T) {
	t.Parallel()

	reference := testPruneReference("fact-legacy", "environment-legacy", 1, 1, 0x47)
	legacyWire, err := encodeTranscript(pruneReferenceDomain, pruneReferenceFields(reference)[:6]...)
	if err != nil {
		t.Fatalf("encode legacy prune reference: %v", err)
	}
	if _, err := ParsePruneReference(legacyWire); !errors.Is(err, ErrInvalidPruneReference) {
		t.Fatalf("legacy six-field parse error = %v, want %v", err, ErrInvalidPruneReference)
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
	pruneAcknowledgement = testPruneAcknowledgement("environment-a", 0x41)
	pruneAcknowledgement.CapsuleDigest = Digest{}
	if err := pruneAcknowledgement.Validate(); !errors.Is(err, ErrInvalidPruneAcknowledgement) {
		t.Fatalf("zero capsule digest error = %v, want %v", err, ErrInvalidPruneAcknowledgement)
	}

	retirement := testTerminalRetirement(0x42)
	retirement.FinalEnvelopeDigest = Digest{}
	if err := retirement.Validate(); !errors.Is(err, ErrInvalidRetirement) {
		t.Fatalf("nonempty retirement with zero digest error = %v, want %v", err, ErrInvalidRetirement)
	}

	pruneReference := testPruneReference("fact-reference", "environment-reference", 1, 1, 0x43)
	if err := pruneReference.Validate(); err != nil {
		t.Fatalf("valid prune reference error = %v", err)
	}
	invalidPruneReferences := []struct {
		name   string
		mutate func(*PruneReference)
	}{
		{name: "zero key generation", mutate: func(value *PruneReference) { value.KeyGeneration = 0 }},
		{name: "first envelope has previous digest", mutate: func(value *PruneReference) { value.PreviousEnvelopeDigest = testControlDigest(0x44) }},
		{name: "later envelope lacks previous digest", mutate: func(value *PruneReference) {
			value.EnvironmentSequence = 2
			value.PreviousEnvelopeDigest = Digest{}
		}},
	}
	for _, test := range invalidPruneReferences {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := pruneReference
			test.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidPruneReference) {
				t.Fatalf("prune reference validation error = %v, want %v", err, ErrInvalidPruneReference)
			}
		})
	}
	zeroNonceReference := pruneReference
	zeroNonceReference.Nonce = Nonce{}
	if err := zeroNonceReference.Validate(); err != nil {
		t.Fatalf("zero nonce prune reference error = %v", err)
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

func TestPruneCertificateMaximumTargetsAndAcknowledgementsFitControlBound(t *testing.T) {
	t.Parallel()

	const (
		closureArrival = int64(1)
		maxIdentifier  = 128
	)
	closure := testPruneReference(
		continuity.FactID(strings.Repeat("c", maxIdentifier)),
		continuity.EnvironmentID(strings.Repeat("d", maxIdentifier)),
		1,
		closureArrival,
		0x48,
	)
	targets := make([]PruneReference, MaxPruneTargets)
	barrier := int64(MaxPruneTargets + 1)
	for index := range targets {
		targets[index] = testPruneReference(
			continuity.FactID(fmt.Sprintf("%s-%04d", strings.Repeat("f", maxIdentifier-5), index)),
			continuity.EnvironmentID(strings.Repeat("e", maxIdentifier)),
			int64(index+2),
			int64(index+2),
			byte(index+0x49),
		)
		targets[index].PreviousEnvelopeDigest = testControlDigest(byte(index + 0x99))
	}
	manifest := PruneManifest{Targets: targets}
	manifestDigest := PruneManifestDigest(manifest)
	closureDigest := PruneReferenceDigest(closure)
	pruneID := testControlDigest(0x4a)
	capsule := testControlCapsule(
		testControlChannelID(),
		testControlRelayGeneration(),
		pruneID,
		1,
		barrier,
		closureDigest,
		MaxPruneTargets,
		manifestDigest,
		MaxPruneBootstrapCiphertextBytes,
	)
	capsuleWire, err := capsule.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal maximum prune capsule: %v", err)
	}
	if len(capsuleWire) != MaxPruneBootstrapBytes {
		t.Fatalf("maximum prune capsule encoded to %d bytes, want exact bound %d", len(capsuleWire), MaxPruneBootstrapBytes)
	}
	capsuleDigest := PruneBootstrapDigest(capsule)
	acknowledgements := make([]PruneAcknowledgement, MaxPruneAcknowledgements)
	for index := range acknowledgements {
		environmentID := continuity.EnvironmentID(fmt.Sprintf("%s-%04d", strings.Repeat("a", maxIdentifier-5), index))
		acknowledgements[index] = testPruneAcknowledgement(environmentID, byte(index+0x4b))
		acknowledgements[index].ChannelID = testControlChannelID()
		acknowledgements[index].RelayGeneration = testControlRelayGeneration()
		acknowledgements[index].MembershipGeneration = 1
		acknowledgements[index].PruneID = pruneID
		acknowledgements[index].AppliedArrivalSequence = barrier
		acknowledgements[index].BarrierArrivalSequence = barrier
		acknowledgements[index].ClosureReferenceDigest = closureDigest
		acknowledgements[index].ManifestCount = MaxPruneTargets
		acknowledgements[index].ManifestDigest = manifestDigest
		acknowledgements[index].CapsuleDigest = capsuleDigest
	}
	certificate := PruneCertificate{
		Version:                    ControlVersionV1,
		ProtocolVersion:            ProtocolVersionV1,
		CipherSuite:                CipherSuiteXChaCha20Poly1305,
		ChannelID:                  testControlChannelID(),
		RelayGeneration:            testControlRelayGeneration(),
		PruneID:                    pruneID,
		MembershipGeneration:       1,
		BarrierArrivalSequence:     barrier,
		Closure:                    closure,
		ClosureDigest:              closureDigest,
		ManifestCount:              MaxPruneTargets,
		ManifestDigest:             manifestDigest,
		Manifest:                   manifest,
		CapsuleDigest:              capsuleDigest,
		Capsule:                    capsule,
		ActiveAcknowledgementCount: MaxPruneAcknowledgements,
		Acknowledgements:           acknowledgements,
	}
	wire, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal maximum prune certificate: %v", err)
	}
	if len(wire) > MaxControlObjectBytes {
		t.Fatalf("maximum prune certificate encoded to %d bytes, limit %d", len(wire), MaxControlObjectBytes)
	}
	decoded, err := ParsePruneCertificate(wire)
	if err != nil {
		t.Fatalf("parse maximum prune certificate: %v", err)
	}
	if len(decoded.Manifest.Targets) != MaxPruneTargets || len(decoded.Acknowledgements) != MaxPruneAcknowledgements {
		t.Fatalf("maximum prune certificate decoded as %d targets and %d acknowledgements", len(decoded.Manifest.Targets), len(decoded.Acknowledgements))
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
		CapsuleDigest:                 testControlDigest(0xa3),
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
	capsule := testControlCapsule(
		testControlChannelID(),
		testControlRelayGeneration(),
		testControlDigest(0xa0),
		7,
		17,
		closureDigest,
		uint32(len(targets)),
		manifestDigest,
		48,
	)
	capsuleDigest := PruneBootstrapDigest(capsule)
	first.CapsuleDigest = capsuleDigest
	second.CapsuleDigest = capsuleDigest
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
		CapsuleDigest:              capsuleDigest,
		Capsule:                    capsule,
		ActiveAcknowledgementCount: 2,
		Acknowledgements:           []PruneAcknowledgement{first, second},
		AdminSignature:             testControlSignature(0xb0),
	}
}

func testControlCapsule(
	channelID ChannelID,
	relayGeneration RelayGeneration,
	pruneID Digest,
	membershipGeneration uint32,
	barrierArrivalSequence int64,
	closureReferenceDigest Digest,
	manifestCount uint32,
	manifestDigest Digest,
	ciphertextBytes int,
) PruneBootstrap {
	ciphertext := make([]byte, ciphertextBytes)
	for index := range ciphertext {
		ciphertext[index] = byte(0xe0 + index)
	}
	return PruneBootstrap{
		CapsuleVersion:          PruneBootstrapCapsuleVersionV1,
		ProtocolVersion:         ProtocolVersionV1,
		CipherSuite:             CipherSuiteXChaCha20Poly1305,
		BootstrapPurposeVersion: PruneBootstrapPurposeVersionV1,
		ChannelID:               channelID,
		RelayGeneration:         relayGeneration,
		PruneID:                 pruneID,
		MembershipGeneration:    membershipGeneration,
		BarrierArrivalSequence:  barrierArrivalSequence,
		ClosureReferenceDigest:  closureReferenceDigest,
		ManifestCount:           manifestCount,
		ManifestDigest:          manifestDigest,
		Nonce:                   testPruneBootstrapNonce(0xc0),
		Ciphertext:              ciphertext,
	}
}

func cloneTestPruneCertificate(certificate PruneCertificate) PruneCertificate {
	cloned := certificate
	cloned.Manifest.Targets = append([]PruneReference(nil), certificate.Manifest.Targets...)
	cloned.Capsule.Ciphertext = append([]byte(nil), certificate.Capsule.Ciphertext...)
	cloned.Acknowledgements = append([]PruneAcknowledgement(nil), certificate.Acknowledgements...)
	return cloned
}

func testPruneReference(factID continuity.FactID, environmentID continuity.EnvironmentID, environmentSequence, arrivalSequence int64, seed byte) PruneReference {
	var previous Digest
	if environmentSequence > 1 {
		previous = testControlDigest(seed + 2)
	}
	var nonce Nonce
	for index := range nonce {
		nonce[index] = seed + byte(index+3)
	}
	return PruneReference{
		FactID:                 factID,
		EnvironmentID:          environmentID,
		EnvironmentSequence:    environmentSequence,
		ArrivalSequence:        arrivalSequence,
		EnvelopeDigest:         testControlDigest(seed),
		CertificateID:          testControlDigest(seed + 1),
		PreviousEnvelopeDigest: previous,
		KeyGeneration:          7,
		Nonce:                  nonce,
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
