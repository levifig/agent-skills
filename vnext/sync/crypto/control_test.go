package crypto

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/sync/protocol"
)

func TestTypedControlSignaturesUseCertifiedAuthoritiesAndExactBindings(t *testing.T) {
	t.Parallel()

	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign environment certificate: %v", err)
	}

	progress := controlProgressAcknowledgement(certificate)
	signedProgress, err := SignProgressAcknowledgement(progress, certificate, adminPublic, environmentSeed)
	if err != nil {
		t.Fatalf("sign progress acknowledgement: %v", err)
	}
	if err := VerifyProgressAcknowledgement(signedProgress, certificate, adminPublic); err != nil {
		t.Fatalf("verify progress acknowledgement: %v", err)
	}

	wrongEnvironment := testEnvironmentSeed(t, 0x70)
	if _, err := SignProgressAcknowledgement(progress, certificate, adminPublic, wrongEnvironment); !errors.Is(err, ErrControlBinding) {
		t.Fatalf("wrong environment signing error = %v, want %v", err, ErrControlBinding)
	}
	tamperedProgress := signedProgress
	tamperedProgress.ChannelID[0] ^= 1
	if err := VerifyProgressAcknowledgement(tamperedProgress, certificate, adminPublic); !errors.Is(err, ErrControlBinding) {
		t.Fatalf("progress channel binding error = %v, want %v", err, ErrControlBinding)
	}
	tamperedProgress = signedProgress
	tamperedProgress.EnvironmentSignature[0] ^= 1
	if err := VerifyProgressAcknowledgement(tamperedProgress, certificate, adminPublic); !errors.Is(err, ErrInvalidControlSignature) {
		t.Fatalf("progress signature error = %v, want %v", err, ErrInvalidControlSignature)
	}

	pruneAcknowledgement := controlPruneAcknowledgement(certificate, signedProgress)
	signedPruneAcknowledgement, err := SignPruneAcknowledgement(pruneAcknowledgement, certificate, adminPublic, environmentSeed)
	if err != nil {
		t.Fatalf("sign prune acknowledgement: %v", err)
	}
	if err := VerifyPruneAcknowledgement(signedPruneAcknowledgement, certificate, adminPublic); err != nil {
		t.Fatalf("verify prune acknowledgement: %v", err)
	}
	tamperedPruneAcknowledgement := signedPruneAcknowledgement
	tamperedPruneAcknowledgement.CertificateID[0] ^= 1
	if err := VerifyPruneAcknowledgement(tamperedPruneAcknowledgement, certificate, adminPublic); !errors.Is(err, ErrControlBinding) {
		t.Fatalf("prune certificate binding error = %v, want %v", err, ErrControlBinding)
	}

	retirement := controlTerminalRetirement(certificate)
	signedRetirement, err := SignTerminalRetirement(retirement, certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign terminal retirement: %v", err)
	}
	if err := VerifyTerminalRetirement(signedRetirement, certificate, adminPublic); err != nil {
		t.Fatalf("verify terminal retirement: %v", err)
	}
	tamperedRetirement := signedRetirement
	tamperedRetirement.EnvironmentID = "environment-other"
	if err := VerifyTerminalRetirement(tamperedRetirement, certificate, adminPublic); !errors.Is(err, ErrControlBinding) {
		t.Fatalf("retirement environment binding error = %v, want %v", err, ErrControlBinding)
	}

	wrongAdmin := testAdminSeed(t, 0x11)
	if _, err := SignTerminalRetirement(retirement, certificate, wrongAdmin); !errors.Is(err, ErrInvalidCertificateSignature) {
		t.Fatalf("wrong admin terminal signing error = %v, want %v", err, ErrInvalidCertificateSignature)
	}
	if _, err := SignProgressAcknowledgement(progress, certificate, AdminPublicKey(wrongAdmin), environmentSeed); !errors.Is(err, ErrInvalidCertificateSignature) {
		t.Fatalf("wrong admin environment signing error = %v, want %v", err, ErrInvalidCertificateSignature)
	}
}

func TestPruneCertificateUsesAdministratorSignatureAndTypedEnvironmentVotes(t *testing.T) {
	t.Parallel()

	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	certificate, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeed)), adminSeed)
	if err != nil {
		t.Fatalf("sign environment certificate: %v", err)
	}

	pruneCertificate := controlPruneCertificate(t, certificate, adminPublic, environmentSeed)
	environmentCertificates := []protocol.EnvironmentCertificate{certificate}
	signed, err := SignPruneCertificate(pruneCertificate, environmentCertificates, adminSeed)
	if err != nil {
		t.Fatalf("sign prune certificate: %v", err)
	}
	if err := VerifyPruneCertificate(signed, environmentCertificates, adminPublic); err != nil {
		t.Fatalf("verify prune certificate: %v", err)
	}
	if err := VerifyPruneAcknowledgement(signed.Acknowledgements[0], certificate, adminPublic); err != nil {
		t.Fatalf("verify embedded environment acknowledgement: %v", err)
	}

	tampered := signed
	tampered.AdminSignature[0] ^= 1
	if err := VerifyPruneCertificate(tampered, environmentCertificates, adminPublic); !errors.Is(err, ErrInvalidControlSignature) {
		t.Fatalf("prune certificate signature error = %v, want %v", err, ErrInvalidControlSignature)
	}

	if _, err := SignPruneCertificate(pruneCertificate, environmentCertificates, AdminSeed{}); !errors.Is(err, ErrInvalidSigningKey) {
		t.Fatalf("zero admin signing error = %v, want %v", err, ErrInvalidSigningKey)
	}
}

func TestPruneCertificateRequiresExactAuthenticatedWitnessSet(t *testing.T) {
	t.Parallel()

	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeedA := testEnvironmentSeed(t, 0x40)
	environmentSeedB := testEnvironmentSeed(t, 0x70)
	certificateA, err := SignEnvironmentCertificate(testUnsignedCertificate(EnvironmentPublicKey(environmentSeedA)), adminSeed)
	if err != nil {
		t.Fatalf("sign environment A certificate: %v", err)
	}
	unsignedB := testUnsignedCertificate(EnvironmentPublicKey(environmentSeedB))
	unsignedB.EnvironmentID = "environment-b"
	unsignedB.MembershipGeneration = 3
	certificateB, err := SignEnvironmentCertificate(unsignedB, adminSeed)
	if err != nil {
		t.Fatalf("sign environment B certificate: %v", err)
	}
	environmentCertificates := []protocol.EnvironmentCertificate{certificateA, certificateB}
	pruneCertificate := controlPruneCertificateForWitnesses(
		t,
		environmentCertificates,
		adminPublic,
		[]EnvironmentSeed{environmentSeedA, environmentSeedB},
	)
	signed, err := SignPruneCertificate(pruneCertificate, environmentCertificates, adminSeed)
	if err != nil {
		t.Fatalf("sign prune certificate: %v", err)
	}
	if err := VerifyPruneCertificate(signed, environmentCertificates, adminPublic); err != nil {
		t.Fatalf("verify prune certificate: %v", err)
	}

	zeroVote := cloneControlPruneCertificate(pruneCertificate)
	zeroVote.Acknowledgements[0].EnvironmentSignature = protocol.Signature{}
	if _, err := SignPruneCertificate(zeroVote, environmentCertificates, adminSeed); !errors.Is(err, ErrInvalidControlSignature) {
		t.Fatalf("zero witness signature error = %v, want %v", err, ErrInvalidControlSignature)
	}

	tamperedVote := cloneControlPruneCertificate(signed)
	tamperedVote.Acknowledgements[1].EnvironmentSignature[0] ^= 1
	body, err := protocol.PruneCertificateBodyTranscript(tamperedVote)
	if err != nil {
		t.Fatalf("build tampered prune certificate body: %v", err)
	}
	copy(tamperedVote.AdminSignature[:], ed25519.Sign(ed25519.NewKeyFromSeed(adminSeed[:]), body))
	if err := VerifyPruneCertificate(tamperedVote, environmentCertificates, adminPublic); !errors.Is(err, ErrInvalidControlSignature) {
		t.Fatalf("tampered witness signature error = %v, want %v", err, ErrInvalidControlSignature)
	}

	missingVote := cloneControlPruneCertificate(pruneCertificate)
	missingVote.Acknowledgements = missingVote.Acknowledgements[:1]
	missingVote.ActiveAcknowledgementCount = 1
	if _, err := SignPruneCertificate(missingVote, environmentCertificates, adminSeed); !errors.Is(err, ErrControlBinding) {
		t.Fatalf("missing witness error = %v, want %v", err, ErrControlBinding)
	}

	duplicateVotes := cloneControlPruneCertificate(pruneCertificate)
	duplicateVotes.Acknowledgements[1] = duplicateVotes.Acknowledgements[0]
	if _, err := SignPruneCertificate(duplicateVotes, environmentCertificates, adminSeed); !errors.Is(err, protocol.ErrInvalidPruneCertificate) {
		t.Fatalf("duplicate witness error = %v, want %v", err, protocol.ErrInvalidPruneCertificate)
	}

	reorderedVotes := cloneControlPruneCertificate(pruneCertificate)
	reorderedVotes.Acknowledgements[0], reorderedVotes.Acknowledgements[1] = reorderedVotes.Acknowledgements[1], reorderedVotes.Acknowledgements[0]
	if _, err := SignPruneCertificate(reorderedVotes, environmentCertificates, adminSeed); !errors.Is(err, protocol.ErrInvalidPruneCertificate) {
		t.Fatalf("reordered witness error = %v, want %v", err, protocol.ErrInvalidPruneCertificate)
	}

	for name, witnesses := range map[string][]protocol.EnvironmentCertificate{
		"missing certificate":   {certificateA},
		"duplicate certificate": {certificateA, certificateA},
		"reordered certificate": {certificateB, certificateA},
	} {
		witnesses := witnesses
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := VerifyPruneCertificate(signed, witnesses, adminPublic); !errors.Is(err, ErrControlBinding) {
				t.Fatalf("VerifyPruneCertificate() error = %v, want %v", err, ErrControlBinding)
			}
		})
	}
}

func TestVerifySealedFactSignatureIsOpaqueAndExpiryIndependent(t *testing.T) {
	t.Parallel()

	root := testProjectRoot(t)
	key, err := DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive generation key: %v", err)
	}
	adminSeed := testAdminSeed(t, 0x10)
	adminPublic := AdminPublicKey(adminSeed)
	environmentSeed := testEnvironmentSeed(t, 0x40)
	unsigned := testUnsignedCertificate(EnvironmentPublicKey(environmentSeed))
	unsigned.Mode = protocol.EnvironmentEphemeral
	unsigned.ExpiresAtMillis = 10_000
	certificate, err := SignEnvironmentCertificate(unsigned, adminSeed)
	if err != nil {
		t.Fatalf("sign expiring certificate: %v", err)
	}
	sealed, err := SealFact(testPlaintextFact(), key, certificate, adminPublic, environmentSeed, protocol.Digest{}, 9_999)
	if err != nil {
		t.Fatalf("seal fact before expiry: %v", err)
	}

	// The relay-facing verifier has no generation key or clock input and only
	// authenticates the opaque signed envelope. Historical verification remains
	// possible after expiry; admission expiry is checked separately.
	if err := VerifySealedFactSignature(sealed, certificate, adminPublic); err != nil {
		t.Fatalf("verify opaque sealed fact: %v", err)
	}
	if _, err := OpenFactForAdmission(sealed, key, certificate, adminPublic, 10_000); !errors.Is(err, protocol.ErrCertificateExpired) {
		t.Fatalf("expired admission error = %v, want %v", err, protocol.ErrCertificateExpired)
	}

	// Re-signing a changed ciphertext requires the environment authority but
	// not the AEAD key. The signature-only verifier deliberately stops before
	// decryption, while OpenFact still rejects the ciphertext authentication.
	mutated := sealed
	mutated.Ciphertext = append([]byte(nil), sealed.Ciphertext...)
	mutated.Ciphertext[0] ^= 1
	transcript, err := protocol.FactSignatureTranscript(mutated.Header, mutated.Ciphertext)
	if err != nil {
		t.Fatalf("build mutated fact transcript: %v", err)
	}
	copy(mutated.Signature[:], ed25519.Sign(ed25519.NewKeyFromSeed(environmentSeed[:]), transcript))
	if err := VerifySealedFactSignature(mutated, certificate, adminPublic); err != nil {
		t.Fatalf("verify re-signed opaque ciphertext: %v", err)
	}
	if _, err := OpenFact(mutated, key, certificate, adminPublic); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("mutated ciphertext open error = %v, want %v", err, ErrAuthenticationFailed)
	}
}

func controlProgressAcknowledgement(certificate protocol.EnvironmentCertificate) protocol.ProgressAcknowledgement {
	return protocol.ProgressAcknowledgement{
		Version:                protocol.ControlVersionV1,
		ProtocolVersion:        protocol.ProtocolVersionV1,
		CipherSuite:            protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:              certificate.ChannelID,
		RelayGeneration:        controlRelayGeneration(),
		EnvironmentID:          certificate.EnvironmentID,
		CertificateID:          protocol.CertificateID(certificate),
		MembershipGeneration:   3,
		AppliedArrivalSequence: 10,
		ProducerSequence:       2,
		ProducerEnvelopeDigest: controlDigest(0x20),
	}
}

func controlPruneAcknowledgement(certificate protocol.EnvironmentCertificate, progress protocol.ProgressAcknowledgement) protocol.PruneAcknowledgement {
	return protocol.PruneAcknowledgement{
		Version:                       protocol.ControlVersionV1,
		ProtocolVersion:               protocol.ProtocolVersionV1,
		CipherSuite:                   protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                     certificate.ChannelID,
		RelayGeneration:               progress.RelayGeneration,
		EnvironmentID:                 certificate.EnvironmentID,
		CertificateID:                 protocol.CertificateID(certificate),
		MembershipGeneration:          progress.MembershipGeneration,
		ProgressAcknowledgementDigest: protocol.ProgressAcknowledgementDigest(progress),
		AppliedArrivalSequence:        7,
		ProducerSequence:              0,
		PruneID:                       controlDigest(0x30),
		BarrierArrivalSequence:        7,
		ClosureReferenceDigest:        controlDigest(0x31),
		ManifestCount:                 1,
		ManifestDigest:                controlDigest(0x32),
	}
}

func controlTerminalRetirement(certificate protocol.EnvironmentCertificate) protocol.TerminalRetirement {
	return protocol.TerminalRetirement{
		Version:                  protocol.ControlVersionV1,
		ProtocolVersion:          protocol.ProtocolVersionV1,
		CipherSuite:              protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                certificate.ChannelID,
		RelayGeneration:          controlRelayGeneration(),
		EnvironmentID:            certificate.EnvironmentID,
		CertificateID:            protocol.CertificateID(certificate),
		MembershipGeneration:     3,
		FinalEnvironmentSequence: 2,
		FinalEnvelopeDigest:      controlDigest(0x40),
	}
}

func controlPruneCertificate(t *testing.T, certificate protocol.EnvironmentCertificate, adminPublic protocol.PublicKey, environmentSeed EnvironmentSeed) protocol.PruneCertificate {
	t.Helper()
	return controlPruneCertificateForWitnesses(
		t,
		[]protocol.EnvironmentCertificate{certificate},
		adminPublic,
		[]EnvironmentSeed{environmentSeed},
	)
}

func controlPruneCertificateForWitnesses(
	t *testing.T,
	certificates []protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
	environmentSeeds []EnvironmentSeed,
) protocol.PruneCertificate {
	t.Helper()
	if len(certificates) == 0 || len(certificates) != len(environmentSeeds) {
		t.Fatal("control prune witnesses must have matching non-empty certificates and seeds")
	}
	certificate := certificates[0]
	closure := protocol.PruneReference{
		FactID:              "fact-close",
		EnvironmentID:       certificate.EnvironmentID,
		EnvironmentSequence: 2,
		ArrivalSequence:     4,
		EnvelopeDigest:      controlDigest(0x50),
		CertificateID:       protocol.CertificateID(certificate),
	}
	target := protocol.PruneReference{
		FactID:              "fact-target",
		EnvironmentID:       certificate.EnvironmentID,
		EnvironmentSequence: 1,
		ArrivalSequence:     7,
		EnvelopeDigest:      controlDigest(0x60),
		CertificateID:       protocol.CertificateID(certificate),
	}
	manifest := protocol.PruneManifest{Targets: []protocol.PruneReference{target}}
	acknowledgements := make([]protocol.PruneAcknowledgement, 0, len(certificates))
	for index, witnessCertificate := range certificates {
		progress := controlProgressAcknowledgement(witnessCertificate)
		pruneAcknowledgement := controlPruneAcknowledgement(witnessCertificate, progress)
		pruneAcknowledgement.ClosureReferenceDigest = protocol.PruneReferenceDigest(closure)
		pruneAcknowledgement.ManifestCount = 1
		pruneAcknowledgement.ManifestDigest = protocol.PruneManifestDigest(manifest)
		signedAcknowledgement, err := SignPruneAcknowledgement(pruneAcknowledgement, witnessCertificate, adminPublic, environmentSeeds[index])
		if err != nil {
			t.Fatalf("sign prune acknowledgement: %v", err)
		}
		acknowledgements = append(acknowledgements, signedAcknowledgement)
	}
	return protocol.PruneCertificate{
		Version:                    protocol.ControlVersionV1,
		ProtocolVersion:            protocol.ProtocolVersionV1,
		CipherSuite:                protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                  certificate.ChannelID,
		RelayGeneration:            controlRelayGeneration(),
		PruneID:                    acknowledgements[0].PruneID,
		MembershipGeneration:       acknowledgements[0].MembershipGeneration,
		BarrierArrivalSequence:     acknowledgements[0].BarrierArrivalSequence,
		Closure:                    closure,
		ClosureDigest:              protocol.PruneReferenceDigest(closure),
		ManifestCount:              1,
		ManifestDigest:             protocol.PruneManifestDigest(manifest),
		Manifest:                   manifest,
		ActiveAcknowledgementCount: uint32(len(acknowledgements)),
		Acknowledgements:           acknowledgements,
	}
}

func cloneControlPruneCertificate(certificate protocol.PruneCertificate) protocol.PruneCertificate {
	cloned := certificate
	cloned.Manifest.Targets = append([]protocol.PruneReference(nil), certificate.Manifest.Targets...)
	cloned.Acknowledgements = append([]protocol.PruneAcknowledgement(nil), certificate.Acknowledgements...)
	return cloned
}

func controlRelayGeneration() protocol.RelayGeneration {
	var value protocol.RelayGeneration
	for index := range value {
		value[index] = byte(0x80 + index)
	}
	return value
}

func controlDigest(seed byte) protocol.Digest {
	var value protocol.Digest
	for index := range value {
		value[index] = seed + byte(index)
	}
	return value
}
