package verifier_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	synccrypto "github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
	"github.com/levifig/loaf/vnext/sync/relay/verifier"
)

type verifierFixture struct {
	adminSeed       synccrypto.AdminSeed
	adminPublic     protocol.PublicKey
	environmentSeed synccrypto.EnvironmentSeed
	certificate     protocol.EnvironmentCertificate
	certificateWire []byte
	channel         relay.ChannelAuthority
	authority       relay.EnvironmentAuthority
	envelope        protocol.SealedFact
	relayEnvelope   relay.Envelope
	progress        protocol.ProgressAcknowledgement
	relayAck        relay.Acknowledgement
	retirement      protocol.TerminalRetirement
	relayRetirement relay.Retirement
	prune           protocol.PruneCertificate
	relayPrune      relay.PruneCertificate
	pruneAuthority  relay.PruneAuthority
}

func TestVerifierAcceptsCanonicalAuthenticatedObjects(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()

	certificateAuthority := relay.EnvironmentCertificateAuthority{
		ChannelAuthority:     f.channel,
		EnvironmentID:        f.authority.EnvironmentID,
		CertificateID:        f.authority.CertificateID,
		CertificateBytes:     append([]byte(nil), f.certificateWire...),
		Mode:                 f.authority.Mode,
		ExpiresAtMillis:      f.authority.ExpiresAtMillis,
		MembershipGeneration: f.authority.MembershipGeneration,
	}
	tests := []struct {
		name string
		fn   func() error
	}{
		{name: "certificate", fn: func() error {
			return v.VerifyEnvironmentCertificate(context.Background(), certificateAuthority)
		}},
		{name: "envelope", fn: func() error {
			return v.VerifyEnvelope(context.Background(), f.authority, f.relayEnvelope)
		}},
		{name: "progress acknowledgement", fn: func() error {
			return v.VerifyAcknowledgement(context.Background(), f.authority, f.relayAck)
		}},
		{name: "retirement", fn: func() error {
			return v.VerifyRetirement(context.Background(), f.authority, f.relayRetirement)
		}},
		{name: "prune certificate", fn: func() error {
			return v.VerifyPruneCertificate(context.Background(), f.pruneAuthority, f.relayPrune)
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); err != nil {
				t.Fatalf("Verify%s() error = %v", test.name, err)
			}
		})
	}
}

func TestVerifierRejectsEnvironmentCertificateAuthorityMismatches(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()
	base := relay.EnvironmentCertificateAuthority{
		ChannelAuthority:     f.channel,
		EnvironmentID:        f.authority.EnvironmentID,
		CertificateID:        f.authority.CertificateID,
		CertificateBytes:     append([]byte(nil), f.certificateWire...),
		Mode:                 f.authority.Mode,
		ExpiresAtMillis:      f.authority.ExpiresAtMillis,
		MembershipGeneration: f.authority.MembershipGeneration,
	}
	tests := []struct {
		name   string
		mutate func(*relay.EnvironmentCertificateAuthority)
	}{
		{name: "channel", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.ChannelID[0] ^= 1 }},
		{name: "relay generation", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.RelayGeneration = relay.RelayGeneration{} }},
		{name: "administrator public key", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.AdminPublicKey[0] ^= 1 }},
		{name: "environment", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.EnvironmentID = "environment-other" }},
		{name: "certificate id", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.CertificateID[0] ^= 1 }},
		{name: "certificate bytes", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.CertificateBytes[0] ^= 1 }},
		{name: "mode", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.Mode = relay.EphemeralEnvironment }},
		{name: "expiry", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.ExpiresAtMillis++ }},
		{name: "membership generation", mutate: func(value *relay.EnvironmentCertificateAuthority) { value.MembershipGeneration++ }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			candidate.CertificateBytes = append([]byte(nil), base.CertificateBytes...)
			test.mutate(&candidate)
			if err := v.VerifyEnvironmentCertificate(context.Background(), candidate); err == nil {
				t.Fatalf("VerifyEnvironmentCertificate() accepted %s mismatch", test.name)
			}
		})
	}
}

func TestVerifierRejectsNilAndCanceledContexts(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()
	certificateAuthority := relay.EnvironmentCertificateAuthority{
		ChannelAuthority:     f.channel,
		EnvironmentID:        f.authority.EnvironmentID,
		CertificateID:        f.authority.CertificateID,
		CertificateBytes:     append([]byte(nil), f.certificateWire...),
		Mode:                 f.authority.Mode,
		ExpiresAtMillis:      f.authority.ExpiresAtMillis,
		MembershipGeneration: f.authority.MembershipGeneration,
	}
	calls := []struct {
		name string
		call func(context.Context) error
	}{
		{name: "certificate", call: func(ctx context.Context) error { return v.VerifyEnvironmentCertificate(ctx, certificateAuthority) }},
		{name: "envelope", call: func(ctx context.Context) error { return v.VerifyEnvelope(ctx, f.authority, f.relayEnvelope) }},
		{name: "acknowledgement", call: func(ctx context.Context) error { return v.VerifyAcknowledgement(ctx, f.authority, f.relayAck) }},
		{name: "retirement", call: func(ctx context.Context) error { return v.VerifyRetirement(ctx, f.authority, f.relayRetirement) }},
		{name: "prune certificate", call: func(ctx context.Context) error { return v.VerifyPruneCertificate(ctx, f.pruneAuthority, f.relayPrune) }},
	}
	for _, call := range calls {
		call := call
		t.Run(call.name+"/nil", func(t *testing.T) {
			if err := call.call(nil); !errors.Is(err, relay.ErrInvalidArgument) {
				t.Fatalf("nil context error = %v, want ErrInvalidArgument", err)
			}
		})
		t.Run(call.name+"/canceled", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := call.call(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled context error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestVerifierRejectsEnvelopeOuterAndInnerMismatches(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()

	outer := []struct {
		name   string
		mutate func(*relay.Envelope)
	}{
		{name: "protocol version", mutate: func(value *relay.Envelope) { value.ProtocolVersion++ }},
		{name: "cipher suite", mutate: func(value *relay.Envelope) { value.CipherSuite++ }},
		{name: "channel", mutate: func(value *relay.Envelope) { value.ChannelID[0] ^= 1 }},
		{name: "fact id", mutate: func(value *relay.Envelope) { value.FactID += "-changed" }},
		{name: "environment id", mutate: func(value *relay.Envelope) { value.EnvironmentID = "environment-other" }},
		{name: "environment sequence", mutate: func(value *relay.Envelope) { value.EnvironmentSequence++ }},
		{name: "key generation", mutate: func(value *relay.Envelope) { value.KeyGeneration++ }},
		{name: "previous digest", mutate: func(value *relay.Envelope) { value.PreviousEnvelopeDigest[0] ^= 1 }},
		{name: "certificate id", mutate: func(value *relay.Envelope) { value.CertificateID[0] ^= 1 }},
		{name: "nonce", mutate: func(value *relay.Envelope) { value.Nonce[0] ^= 1 }},
		{name: "ciphertext", mutate: func(value *relay.Envelope) { value.Ciphertext[0] ^= 1 }},
		{name: "signature", mutate: func(value *relay.Envelope) { value.Signature[0] ^= 1 }},
		{name: "envelope digest", mutate: func(value *relay.Envelope) { value.EnvelopeDigest[0] ^= 1 }},
	}
	for _, test := range outer {
		test := test
		t.Run("outer/"+test.name, func(t *testing.T) {
			candidate := cloneRelayEnvelope(f.relayEnvelope)
			test.mutate(&candidate)
			if err := v.VerifyEnvelope(context.Background(), f.authority, candidate); err == nil {
				t.Fatalf("VerifyEnvelope() accepted %s mismatch", test.name)
			}
		})
	}

	inner := []struct {
		name   string
		mutate func(*protocol.FactHeader)
	}{
		{name: "channel", mutate: func(value *protocol.FactHeader) { value.ChannelID[0] ^= 1 }},
		{name: "fact id", mutate: func(value *protocol.FactHeader) { value.FactID += "-changed" }},
		{name: "environment id", mutate: func(value *protocol.FactHeader) { value.EnvironmentID = "environment-other" }},
		{name: "sequence", mutate: func(value *protocol.FactHeader) {
			value.EnvironmentSequence = 2
			value.PreviousEnvelopeDigest = digest(0x91)
		}},
		{name: "generation", mutate: func(value *protocol.FactHeader) { value.KeyGeneration++ }},
		{name: "previous digest", mutate: func(value *protocol.FactHeader) {
			value.EnvironmentSequence = 2
			value.PreviousEnvelopeDigest = digest(0x92)
		}},
		{name: "certificate id", mutate: func(value *protocol.FactHeader) { value.CertificateID[0] ^= 1 }},
		{name: "nonce", mutate: func(value *protocol.FactHeader) { value.Nonce[0] ^= 1 }},
	}
	for _, test := range inner {
		test := test
		t.Run("signed-inner/"+test.name, func(t *testing.T) {
			sealed := f.envelope
			test.mutate(&sealed.Header)
			sealed.Signature = signFact(t, sealed.Header, sealed.Ciphertext, f.environmentSeed)
			candidate := cloneRelayEnvelope(f.relayEnvelope)
			candidate.Signature = relay.Signature(sealed.Signature)
			candidate.EnvelopeDigest = relay.Digest(protocol.EnvelopeDigest(sealed))
			if err := v.VerifyEnvelope(context.Background(), f.authority, candidate); err == nil {
				t.Fatalf("VerifyEnvelope() accepted signed-inner %s mismatch", test.name)
			}
		})
	}
}

func TestVerifierRejectsAcknowledgementOuterAndInnerMismatches(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()

	outer := []struct {
		name   string
		mutate func(*relay.Acknowledgement)
	}{
		{name: "channel", mutate: func(value *relay.Acknowledgement) { value.ChannelID[0] ^= 1 }},
		{name: "environment", mutate: func(value *relay.Acknowledgement) { value.EnvironmentID = "environment-other" }},
		{name: "membership", mutate: func(value *relay.Acknowledgement) { value.MembershipGeneration++ }},
		{name: "applied arrival", mutate: func(value *relay.Acknowledgement) { value.AppliedArrivalSequence++ }},
		{name: "producer sequence", mutate: func(value *relay.Acknowledgement) { value.ProducerSequence++ }},
		{name: "producer digest", mutate: func(value *relay.Acknowledgement) { value.ProducerEnvelopeDigest[0] ^= 1 }},
		{name: "certificate", mutate: func(value *relay.Acknowledgement) { value.CertificateID[0] ^= 1 }},
		{name: "acknowledgement digest", mutate: func(value *relay.Acknowledgement) { value.AcknowledgementDigest[0] ^= 1 }},
		{name: "acknowledgement bytes", mutate: func(value *relay.Acknowledgement) { value.AcknowledgementBytes = append(value.AcknowledgementBytes, 0) }},
	}
	for _, test := range outer {
		test := test
		t.Run("outer/"+test.name, func(t *testing.T) {
			candidate := cloneRelayAcknowledgement(f.relayAck)
			test.mutate(&candidate)
			if err := v.VerifyAcknowledgement(context.Background(), f.authority, candidate); err == nil {
				t.Fatalf("VerifyAcknowledgement() accepted %s mismatch", test.name)
			}
		})
	}

	inner := []struct {
		name   string
		mutate func(*protocol.ProgressAcknowledgement)
	}{
		{name: "channel", mutate: func(value *protocol.ProgressAcknowledgement) { value.ChannelID[0] ^= 1 }},
		{name: "relay generation", mutate: func(value *protocol.ProgressAcknowledgement) { value.RelayGeneration[0] ^= 1 }},
		{name: "environment", mutate: func(value *protocol.ProgressAcknowledgement) { value.EnvironmentID = "environment-other" }},
		{name: "certificate", mutate: func(value *protocol.ProgressAcknowledgement) { value.CertificateID[0] ^= 1 }},
		{name: "membership", mutate: func(value *protocol.ProgressAcknowledgement) { value.MembershipGeneration++ }},
		{name: "applied arrival", mutate: func(value *protocol.ProgressAcknowledgement) { value.AppliedArrivalSequence++ }},
		{name: "producer sequence", mutate: func(value *protocol.ProgressAcknowledgement) { value.ProducerSequence++ }},
		{name: "producer digest", mutate: func(value *protocol.ProgressAcknowledgement) { value.ProducerEnvelopeDigest[0] ^= 1 }},
	}
	for _, test := range inner {
		test := test
		t.Run("signed-inner/"+test.name, func(t *testing.T) {
			progress := f.progress
			test.mutate(&progress)
			progress, wire := rawSignProgress(t, progress, f.environmentSeed)
			candidate := cloneRelayAcknowledgement(f.relayAck)
			candidate.AcknowledgementBytes = wire
			candidate.AcknowledgementDigest = relay.Digest(protocol.ProgressAcknowledgementDigest(progress))
			if err := v.VerifyAcknowledgement(context.Background(), f.authority, candidate); err == nil {
				t.Fatalf("VerifyAcknowledgement() accepted signed-inner %s mismatch", test.name)
			}
		})
	}
}

func TestVerifierBindsRetirementRelayGenerationToAuthority(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()
	retirement := f.retirement
	retirement.RelayGeneration[0] ^= 1
	retirement, wire := signRetirement(t, retirement, f.certificate, f.adminSeed)
	candidate := f.relayRetirement
	candidate.RelayGeneration = relay.RelayGeneration(retirement.RelayGeneration)
	candidate.RetirementBytes = wire
	candidate.RetirementID = relay.Digest(protocol.TerminalRetirementID(retirement))
	if err := v.VerifyRetirement(context.Background(), f.authority, candidate); err == nil {
		t.Fatal("VerifyRetirement() accepted a fence signed for another relay generation")
	}
}

func TestVerifierRejectsRetirementOuterAndInnerMismatches(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()

	outer := []struct {
		name   string
		mutate func(*relay.Retirement)
	}{
		{name: "channel", mutate: func(value *relay.Retirement) { value.ChannelID[0] ^= 1 }},
		{name: "relay generation", mutate: func(value *relay.Retirement) { value.RelayGeneration[0] ^= 1 }},
		{name: "environment", mutate: func(value *relay.Retirement) { value.EnvironmentID = "environment-other" }},
		{name: "certificate", mutate: func(value *relay.Retirement) { value.CertificateID[0] ^= 1 }},
		{name: "membership", mutate: func(value *relay.Retirement) { value.MembershipGeneration++ }},
		{name: "final sequence", mutate: func(value *relay.Retirement) { value.FinalEnvironmentSequence++ }},
		{name: "final digest", mutate: func(value *relay.Retirement) { value.FinalEnvelopeDigest[0] ^= 1 }},
		{name: "retirement id", mutate: func(value *relay.Retirement) { value.RetirementID[0] ^= 1 }},
		{name: "retirement bytes", mutate: func(value *relay.Retirement) { value.RetirementBytes = append(value.RetirementBytes, 0) }},
	}
	for _, test := range outer {
		test := test
		t.Run("outer/"+test.name, func(t *testing.T) {
			candidate := cloneRelayRetirement(f.relayRetirement)
			test.mutate(&candidate)
			if err := v.VerifyRetirement(context.Background(), f.authority, candidate); err == nil {
				t.Fatalf("VerifyRetirement() accepted %s mismatch", test.name)
			}
		})
	}

	inner := []struct {
		name   string
		mutate func(*protocol.TerminalRetirement)
	}{
		{name: "channel", mutate: func(value *protocol.TerminalRetirement) { value.ChannelID[0] ^= 1 }},
		{name: "relay generation", mutate: func(value *protocol.TerminalRetirement) { value.RelayGeneration[0] ^= 1 }},
		{name: "environment", mutate: func(value *protocol.TerminalRetirement) { value.EnvironmentID = "environment-other" }},
		{name: "certificate", mutate: func(value *protocol.TerminalRetirement) { value.CertificateID[0] ^= 1 }},
		{name: "membership", mutate: func(value *protocol.TerminalRetirement) { value.MembershipGeneration++ }},
		{name: "final sequence", mutate: func(value *protocol.TerminalRetirement) { value.FinalEnvironmentSequence++ }},
		{name: "final digest", mutate: func(value *protocol.TerminalRetirement) { value.FinalEnvelopeDigest[0] ^= 1 }},
	}
	for _, test := range inner {
		test := test
		t.Run("signed-inner/"+test.name, func(t *testing.T) {
			retirement := f.retirement
			test.mutate(&retirement)
			retirement, wire := rawSignRetirement(t, retirement, f.adminSeed)
			candidate := cloneRelayRetirement(f.relayRetirement)
			candidate.RetirementBytes = wire
			candidate.RetirementID = relay.Digest(protocol.TerminalRetirementID(retirement))
			if err := v.VerifyRetirement(context.Background(), f.authority, candidate); err == nil {
				t.Fatalf("VerifyRetirement() accepted signed-inner %s mismatch", test.name)
			}
		})
	}
}

func TestVerifierBindsPruneRelayGenerationAndChannelToAuthority(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()

	for _, name := range []string{"relay generation", "channel"} {
		name := name
		t.Run(name, func(t *testing.T) {
			certificate := cloneProtocolPruneCertificate(f.prune)
			if name == "relay generation" {
				certificate.RelayGeneration[0] ^= 1
				certificate.Capsule.RelayGeneration = certificate.RelayGeneration
				certificate.CapsuleDigest = protocol.PruneBootstrapDigest(certificate.Capsule)
				for index := range certificate.Acknowledgements {
					certificate.Acknowledgements[index].RelayGeneration = certificate.RelayGeneration
					certificate.Acknowledgements[index].CapsuleDigest = certificate.CapsuleDigest
					certificate.Acknowledgements[index] = rawSignPruneAcknowledgement(t, certificate.Acknowledgements[index], f.environmentSeed)
				}
			} else {
				certificate.ChannelID[0] ^= 1
				certificate.Capsule.ChannelID = certificate.ChannelID
				certificate.CapsuleDigest = protocol.PruneBootstrapDigest(certificate.Capsule)
				for index := range certificate.Acknowledgements {
					certificate.Acknowledgements[index].ChannelID = certificate.ChannelID
					certificate.Acknowledgements[index].CapsuleDigest = certificate.CapsuleDigest
					certificate.Acknowledgements[index] = rawSignPruneAcknowledgement(t, certificate.Acknowledgements[index], f.environmentSeed)
				}
			}
			certificate = rawSignPruneCertificate(t, certificate, f.adminSeed)
			var err error
			wire, err := certificate.MarshalBinary()
			if err != nil {
				t.Fatalf("marshal prune certificate: %v", err)
			}
			candidate := f.relayPrune
			candidate.CertificateBytes = wire
			candidate.CertificateID = relay.Digest(protocol.PruneCertificateID(certificate))
			if err := v.VerifyPruneCertificate(context.Background(), f.pruneAuthority, candidate); err == nil {
				t.Fatalf("VerifyPruneCertificate() accepted a certificate with mismatched %s", name)
			}
		})
	}
}

func TestVerifierRejectsPruneOuterInnerAndAuthorityMismatches(t *testing.T) {
	t.Parallel()
	f := newVerifierFixture(t)
	v := verifier.New()

	outer := []struct {
		name   string
		mutate func(*relay.PruneCertificate)
	}{
		{name: "channel", mutate: func(value *relay.PruneCertificate) { value.ChannelID[0] ^= 1 }},
		{name: "prune id", mutate: func(value *relay.PruneCertificate) { value.PruneID[0] ^= 1 }},
		{name: "membership", mutate: func(value *relay.PruneCertificate) { value.MembershipGeneration++ }},
		{name: "barrier", mutate: func(value *relay.PruneCertificate) { value.Barrier++ }},
		{name: "closure fact", mutate: func(value *relay.PruneCertificate) { value.Closure.FactID += "-changed" }},
		{name: "closure environment", mutate: func(value *relay.PruneCertificate) { value.Closure.EnvironmentID = "environment-other" }},
		{name: "closure sequence", mutate: func(value *relay.PruneCertificate) { value.Closure.EnvironmentSequence++ }},
		{name: "closure arrival", mutate: func(value *relay.PruneCertificate) { value.Closure.ArrivalSequence-- }},
		{name: "closure envelope digest", mutate: func(value *relay.PruneCertificate) { value.Closure.EnvelopeDigest[0] ^= 1 }},
		{name: "closure certificate", mutate: func(value *relay.PruneCertificate) { value.Closure.CertificateID[0] ^= 1 }},
		{name: "closure previous digest", mutate: func(value *relay.PruneCertificate) { value.Closure.PreviousEnvelopeDigest[0] ^= 1 }},
		{name: "closure key generation", mutate: func(value *relay.PruneCertificate) { value.Closure.KeyGeneration++ }},
		{name: "closure nonce", mutate: func(value *relay.PruneCertificate) { value.Closure.Nonce[0] ^= 1 }},
		{name: "certificate id", mutate: func(value *relay.PruneCertificate) { value.CertificateID[0] ^= 1 }},
		{name: "certificate bytes", mutate: func(value *relay.PruneCertificate) { value.CertificateBytes = append(value.CertificateBytes, 0) }},
		{name: "target fact", mutate: func(value *relay.PruneCertificate) { value.Targets[0].FactID += "-changed" }},
		{name: "target environment", mutate: func(value *relay.PruneCertificate) { value.Targets[0].EnvironmentID = "environment-other" }},
		{name: "target sequence", mutate: func(value *relay.PruneCertificate) { value.Targets[0].EnvironmentSequence++ }},
		{name: "target arrival", mutate: func(value *relay.PruneCertificate) { value.Targets[0].ArrivalSequence++ }},
		{name: "target envelope digest", mutate: func(value *relay.PruneCertificate) { value.Targets[0].EnvelopeDigest[0] ^= 1 }},
		{name: "target certificate", mutate: func(value *relay.PruneCertificate) { value.Targets[0].CertificateID[0] ^= 1 }},
		{name: "target previous digest", mutate: func(value *relay.PruneCertificate) { value.Targets[0].PreviousEnvelopeDigest[0] ^= 1 }},
		{name: "target key generation", mutate: func(value *relay.PruneCertificate) { value.Targets[0].KeyGeneration++ }},
		{name: "target nonce", mutate: func(value *relay.PruneCertificate) { value.Targets[0].Nonce[0] ^= 1 }},
	}
	for _, test := range outer {
		test := test
		t.Run("outer/"+test.name, func(t *testing.T) {
			candidate := cloneRelayPruneCertificate(f.relayPrune)
			test.mutate(&candidate)
			if err := v.VerifyPruneCertificate(context.Background(), f.pruneAuthority, candidate); err == nil {
				t.Fatalf("VerifyPruneCertificate() accepted %s mismatch", test.name)
			}
		})
	}

	inner := []struct {
		name   string
		mutate func(*protocol.PruneCertificate)
	}{
		{name: "closure", mutate: func(value *protocol.PruneCertificate) {
			value.Closure.EnvelopeDigest[0] ^= 1
			value.ClosureDigest = protocol.PruneReferenceDigest(value.Closure)
			value.Capsule.ClosureReferenceDigest = value.ClosureDigest
			value.CapsuleDigest = protocol.PruneBootstrapDigest(value.Capsule)
			value.Acknowledgements[0].ClosureReferenceDigest = value.ClosureDigest
			value.Acknowledgements[0].CapsuleDigest = value.CapsuleDigest
		}},
		{name: "target", mutate: func(value *protocol.PruneCertificate) {
			value.Manifest.Targets[0].EnvelopeDigest[0] ^= 1
			value.ManifestDigest = protocol.PruneManifestDigest(value.Manifest)
			value.Capsule.ManifestDigest = value.ManifestDigest
			value.CapsuleDigest = protocol.PruneBootstrapDigest(value.Capsule)
			value.Acknowledgements[0].ManifestDigest = value.ManifestDigest
			value.Acknowledgements[0].CapsuleDigest = value.CapsuleDigest
		}},
		{name: "vote frontier", mutate: func(value *protocol.PruneCertificate) {
			value.Acknowledgements[0].AppliedArrivalSequence++
		}},
	}
	for _, test := range inner {
		test := test
		t.Run("signed-inner/"+test.name, func(t *testing.T) {
			certificate := cloneProtocolPruneCertificate(f.prune)
			test.mutate(&certificate)
			certificate.Acknowledgements[0] = rawSignPruneAcknowledgement(t, certificate.Acknowledgements[0], f.environmentSeed)
			certificate = rawSignPruneCertificate(t, certificate, f.adminSeed)
			var err error
			wire, err := certificate.MarshalBinary()
			if err != nil {
				t.Fatalf("marshal prune certificate: %v", err)
			}
			candidate := f.relayPrune
			candidate.CertificateBytes = wire
			candidate.CertificateID = relay.Digest(protocol.PruneCertificateID(certificate))
			if err := v.VerifyPruneCertificate(context.Background(), f.pruneAuthority, candidate); err == nil {
				t.Fatalf("VerifyPruneCertificate() accepted signed-inner %s mismatch", test.name)
			}
		})
	}

	for _, name := range []string{"missing environment", "extra environment", "environment mismatch", "stored acknowledgement mismatch"} {
		name := name
		t.Run("authority/"+name, func(t *testing.T) {
			authority := clonePruneAuthority(f.pruneAuthority)
			switch name {
			case "missing environment":
				authority.Environments = nil
			case "extra environment":
				authority.Environments = append(authority.Environments, authority.Environments[0])
			case "environment mismatch":
				authority.Environments[0].EnvironmentID = "environment-other"
			case "stored acknowledgement mismatch":
				authority.Acknowledgements[0].AcknowledgementDigest[0] ^= 1
			}
			if err := v.VerifyPruneCertificate(context.Background(), authority, f.relayPrune); err == nil {
				t.Fatalf("VerifyPruneCertificate() accepted %s", name)
			}
		})
	}
}

func newVerifierFixture(t *testing.T) verifierFixture {
	t.Helper()
	adminSeed := seededAdmin(0x11)
	adminPublic := synccrypto.AdminPublicKey(adminSeed)
	environmentSeed := seededEnvironment(0x41)
	var channelID protocol.ChannelID
	var relayGeneration protocol.RelayGeneration
	for index := range channelID {
		channelID[index] = 0x10 + byte(index)
		relayGeneration[index] = 0x80 + byte(index)
	}
	environmentID := continuity.EnvironmentID("environment-a")
	certificate := protocol.EnvironmentCertificate{
		Version:               protocol.CertificateVersionV1,
		ProtocolVersion:       protocol.ProtocolVersionV1,
		CipherSuite:           protocol.CipherSuiteXChaCha20Poly1305,
		ProjectID:             "project-1",
		ChannelID:             channelID,
		EnvironmentID:         environmentID,
		EnvironmentPublicKey:  synccrypto.EnvironmentPublicKey(environmentSeed),
		Mode:                  protocol.EnvironmentTrusted,
		MembershipGeneration:  1,
		AllowedKeyGenerations: []uint32{1, 2},
	}
	var err error
	certificate, err = synccrypto.SignEnvironmentCertificate(certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign certificate: %v", err)
	}
	certificateWire, err := certificate.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal certificate: %v", err)
	}
	channel := relay.ChannelAuthority{
		ChannelID:       relay.ChannelID(channelID),
		RelayGeneration: relay.RelayGeneration(relayGeneration),
		AdminPublicKey:  relay.PublicKey(adminPublic),
	}
	authority := relay.EnvironmentAuthority{
		ChannelAuthority:     channel,
		EnvironmentID:        relay.EnvironmentID(environmentID),
		CertificateID:        relay.Digest(protocol.CertificateID(certificate)),
		CertificateBytes:     append([]byte(nil), certificateWire...),
		Mode:                 relay.TrustedEnvironment,
		ExpiresAtMillis:      certificate.ExpiresAtMillis,
		MembershipGeneration: certificate.MembershipGeneration,
	}

	envelope := makeEnvelope(t, certificate, channelID, relayGeneration, environmentSeed)
	relayEnvelope := toRelayEnvelope(envelope)
	progress := protocol.ProgressAcknowledgement{
		Version:                protocol.ControlVersionV1,
		ProtocolVersion:        protocol.ProtocolVersionV1,
		CipherSuite:            protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:              channelID,
		RelayGeneration:        relayGeneration,
		EnvironmentID:          environmentID,
		CertificateID:          protocol.CertificateID(certificate),
		MembershipGeneration:   1,
		AppliedArrivalSequence: 2,
		ProducerSequence:       2,
		ProducerEnvelopeDigest: envelopeDigest(t, certificate, channelID, relayGeneration, environmentSeed, "fact-closure", 2, protocol.EnvelopeDigest(envelope)),
	}
	progress, progressWire := signProgress(t, progress, certificate, adminPublic, environmentSeed)
	relayAck := toRelayAcknowledgement(progress, progressWire)

	retirement := protocol.TerminalRetirement{
		Version:                  protocol.ControlVersionV1,
		ProtocolVersion:          protocol.ProtocolVersionV1,
		CipherSuite:              protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                channelID,
		RelayGeneration:          relayGeneration,
		EnvironmentID:            environmentID,
		CertificateID:            protocol.CertificateID(certificate),
		MembershipGeneration:     2,
		FinalEnvironmentSequence: 2,
		FinalEnvelopeDigest:      progress.ProducerEnvelopeDigest,
	}
	retirement, retirementWire := signRetirement(t, retirement, certificate, adminSeed)
	relayRetirement := toRelayRetirement(retirement, retirementWire)

	target := protocol.PruneReference{
		FactID:              envelope.Header.FactID,
		EnvironmentID:       envelope.Header.EnvironmentID,
		EnvironmentSequence: 1,
		ArrivalSequence:     1,
		EnvelopeDigest:      protocol.EnvelopeDigest(envelope),
		CertificateID:       protocol.CertificateID(certificate),
		KeyGeneration:       1,
		Nonce:               envelope.Header.Nonce,
	}
	closure := protocol.PruneReference{
		FactID:                 "fact-closure",
		EnvironmentID:          environmentID,
		EnvironmentSequence:    2,
		ArrivalSequence:        2,
		EnvelopeDigest:         progress.ProducerEnvelopeDigest,
		CertificateID:          protocol.CertificateID(certificate),
		PreviousEnvelopeDigest: target.EnvelopeDigest,
		KeyGeneration:          1,
		Nonce:                  nonce(0xb0),
	}
	manifest := protocol.PruneManifest{Targets: []protocol.PruneReference{target}}
	closureDigest := protocol.PruneReferenceDigest(closure)
	manifestDigest := protocol.PruneManifestDigest(manifest)
	pruneID := digest(0xc0)
	capsule := protocol.PruneBootstrap{
		CapsuleVersion:          protocol.PruneBootstrapCapsuleVersionV1,
		ProtocolVersion:         protocol.ProtocolVersionV1,
		CipherSuite:             protocol.CipherSuiteXChaCha20Poly1305,
		BootstrapPurposeVersion: protocol.PruneBootstrapPurposeVersionV1,
		ChannelID:               channelID,
		RelayGeneration:         relayGeneration,
		PruneID:                 pruneID,
		MembershipGeneration:    1,
		BarrierArrivalSequence:  2,
		ClosureReferenceDigest:  closureDigest,
		ManifestCount:           1,
		ManifestDigest:          manifestDigest,
		Nonce:                   nonce(0xd0),
		Ciphertext:              bytes.Repeat([]byte{0xd1}, 16),
	}
	capsuleDigest := protocol.PruneBootstrapDigest(capsule)
	pruneAcknowledgement := protocol.PruneAcknowledgement{
		Version:                       protocol.ControlVersionV1,
		ProtocolVersion:               protocol.ProtocolVersionV1,
		CipherSuite:                   protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                     channelID,
		RelayGeneration:               relayGeneration,
		EnvironmentID:                 environmentID,
		CertificateID:                 protocol.CertificateID(certificate),
		MembershipGeneration:          1,
		ProgressAcknowledgementDigest: protocol.ProgressAcknowledgementDigest(progress),
		AppliedArrivalSequence:        progress.AppliedArrivalSequence,
		ProducerSequence:              progress.ProducerSequence,
		ProducerEnvelopeDigest:        progress.ProducerEnvelopeDigest,
		PruneID:                       pruneID,
		BarrierArrivalSequence:        2,
		ClosureReferenceDigest:        closureDigest,
		ManifestCount:                 1,
		ManifestDigest:                manifestDigest,
		CapsuleDigest:                 capsuleDigest,
	}
	pruneAcknowledgement, err = synccrypto.SignPruneAcknowledgement(pruneAcknowledgement, certificate, adminPublic, environmentSeed)
	if err != nil {
		t.Fatalf("sign prune acknowledgement: %v", err)
	}
	prune := protocol.PruneCertificate{
		Version:                    protocol.ControlVersionV1,
		ProtocolVersion:            protocol.ProtocolVersionV1,
		CipherSuite:                protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:                  channelID,
		RelayGeneration:            relayGeneration,
		PruneID:                    pruneAcknowledgement.PruneID,
		MembershipGeneration:       1,
		BarrierArrivalSequence:     2,
		Closure:                    closure,
		ClosureDigest:              closureDigest,
		ManifestCount:              1,
		ManifestDigest:             manifestDigest,
		Manifest:                   manifest,
		CapsuleDigest:              capsuleDigest,
		Capsule:                    capsule,
		ActiveAcknowledgementCount: 1,
		Acknowledgements:           []protocol.PruneAcknowledgement{pruneAcknowledgement},
	}
	prune, err = synccrypto.SignPruneCertificate(prune, []protocol.EnvironmentCertificate{certificate}, adminSeed)
	if err != nil {
		t.Fatalf("sign prune certificate: %v", err)
	}
	pruneWire, err := prune.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal prune certificate: %v", err)
	}
	relayPrune := toRelayPrune(prune, pruneWire)
	return verifierFixture{
		adminSeed:       adminSeed,
		adminPublic:     adminPublic,
		environmentSeed: environmentSeed,
		certificate:     certificate,
		certificateWire: certificateWire,
		channel:         channel,
		authority:       authority,
		envelope:        envelope,
		relayEnvelope:   relayEnvelope,
		progress:        progress,
		relayAck:        relayAck,
		retirement:      retirement,
		relayRetirement: relayRetirement,
		prune:           prune,
		relayPrune:      relayPrune,
		pruneAuthority: relay.PruneAuthority{
			Channel:          channel,
			Environments:     []relay.EnvironmentAuthority{authority},
			Acknowledgements: []relay.Acknowledgement{relayAck},
		},
	}
}

func makeEnvelope(t *testing.T, certificate protocol.EnvironmentCertificate, channelID protocol.ChannelID, generation protocol.RelayGeneration, environmentSeed synccrypto.EnvironmentSeed) protocol.SealedFact {
	t.Helper()
	header := protocol.FactHeader{
		ProtocolVersion:     protocol.ProtocolVersionV1,
		CipherSuite:         protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:           channelID,
		FactID:              "fact-target",
		EnvironmentID:       certificate.EnvironmentID,
		EnvironmentSequence: 1,
		KeyGeneration:       1,
		CertificateID:       protocol.CertificateID(certificate),
		Nonce:               nonce(0x61),
	}
	ciphertext := bytes.Repeat([]byte{0x51}, relay.MinimumCiphertextBytes)
	return protocol.SealedFact{Header: header, Ciphertext: ciphertext, Signature: signFact(t, header, ciphertext, environmentSeed)}
}

func envelopeDigest(t *testing.T, certificate protocol.EnvironmentCertificate, channelID protocol.ChannelID, generation protocol.RelayGeneration, environmentSeed synccrypto.EnvironmentSeed, factID continuity.FactID, sequence int64, previous protocol.Digest) protocol.Digest {
	t.Helper()
	header := protocol.FactHeader{
		ProtocolVersion:        protocol.ProtocolVersionV1,
		CipherSuite:            protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:              channelID,
		FactID:                 factID,
		EnvironmentID:          certificate.EnvironmentID,
		EnvironmentSequence:    sequence,
		KeyGeneration:          1,
		PreviousEnvelopeDigest: previous,
		CertificateID:          protocol.CertificateID(certificate),
		Nonce:                  nonce(0x71),
	}
	ciphertext := bytes.Repeat([]byte{0x62}, relay.MinimumCiphertextBytes)
	sealed := protocol.SealedFact{Header: header, Ciphertext: ciphertext, Signature: signFact(t, header, ciphertext, environmentSeed)}
	return protocol.EnvelopeDigest(sealed)
}

func signFact(t *testing.T, header protocol.FactHeader, ciphertext []byte, seed synccrypto.EnvironmentSeed) protocol.Signature {
	t.Helper()
	transcript, err := protocol.FactSignatureTranscript(header, ciphertext)
	if err != nil {
		t.Fatalf("fact transcript: %v", err)
	}
	var signature protocol.Signature
	copy(signature[:], ed25519.Sign(ed25519.NewKeyFromSeed(seed[:]), transcript))
	return signature
}

func signProgress(t *testing.T, progress protocol.ProgressAcknowledgement, certificate protocol.EnvironmentCertificate, adminPublic protocol.PublicKey, environmentSeed synccrypto.EnvironmentSeed) (protocol.ProgressAcknowledgement, []byte) {
	t.Helper()
	signed, err := synccrypto.SignProgressAcknowledgement(progress, certificate, adminPublic, environmentSeed)
	if err != nil {
		t.Fatalf("sign progress: %v", err)
	}
	wire, err := signed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal progress: %v", err)
	}
	return signed, wire
}

func rawSignProgress(t *testing.T, progress protocol.ProgressAcknowledgement, environmentSeed synccrypto.EnvironmentSeed) (protocol.ProgressAcknowledgement, []byte) {
	t.Helper()
	body, err := protocol.ProgressAcknowledgementBodyTranscript(progress)
	if err != nil {
		t.Fatalf("raw progress transcript: %v", err)
	}
	copy(progress.EnvironmentSignature[:], ed25519.Sign(ed25519.NewKeyFromSeed(environmentSeed[:]), body))
	wire, err := progress.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal raw progress: %v", err)
	}
	return progress, wire
}

func signRetirement(t *testing.T, retirement protocol.TerminalRetirement, certificate protocol.EnvironmentCertificate, adminSeed synccrypto.AdminSeed) (protocol.TerminalRetirement, []byte) {
	t.Helper()
	signed, err := synccrypto.SignTerminalRetirement(retirement, certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign retirement: %v", err)
	}
	wire, err := signed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal retirement: %v", err)
	}
	return signed, wire
}

func rawSignRetirement(t *testing.T, retirement protocol.TerminalRetirement, adminSeed synccrypto.AdminSeed) (protocol.TerminalRetirement, []byte) {
	t.Helper()
	body, err := protocol.TerminalRetirementBodyTranscript(retirement)
	if err != nil {
		t.Fatalf("raw retirement transcript: %v", err)
	}
	copy(retirement.AdminSignature[:], ed25519.Sign(ed25519.NewKeyFromSeed(adminSeed[:]), body))
	wire, err := retirement.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal raw retirement: %v", err)
	}
	return retirement, wire
}

func rawSignPruneAcknowledgement(t *testing.T, acknowledgement protocol.PruneAcknowledgement, environmentSeed synccrypto.EnvironmentSeed) protocol.PruneAcknowledgement {
	t.Helper()
	body, err := protocol.PruneAcknowledgementBodyTranscript(acknowledgement)
	if err != nil {
		t.Fatalf("raw prune acknowledgement transcript: %v", err)
	}
	copy(acknowledgement.EnvironmentSignature[:], ed25519.Sign(ed25519.NewKeyFromSeed(environmentSeed[:]), body))
	return acknowledgement
}

func rawSignPruneCertificate(t *testing.T, certificate protocol.PruneCertificate, adminSeed synccrypto.AdminSeed) protocol.PruneCertificate {
	t.Helper()
	body, err := protocol.PruneCertificateBodyTranscript(certificate)
	if err != nil {
		t.Fatalf("raw prune certificate transcript: %v", err)
	}
	copy(certificate.AdminSignature[:], ed25519.Sign(ed25519.NewKeyFromSeed(adminSeed[:]), body))
	return certificate
}

func toRelayEnvelope(value protocol.SealedFact) relay.Envelope {
	return relay.Envelope{
		ProtocolVersion:        value.Header.ProtocolVersion,
		CipherSuite:            value.Header.CipherSuite,
		ChannelID:              relay.ChannelID(value.Header.ChannelID),
		FactID:                 relay.FactID(value.Header.FactID),
		EnvironmentID:          relay.EnvironmentID(value.Header.EnvironmentID),
		EnvironmentSequence:    value.Header.EnvironmentSequence,
		KeyGeneration:          value.Header.KeyGeneration,
		PreviousEnvelopeDigest: relay.Digest(value.Header.PreviousEnvelopeDigest),
		CertificateID:          relay.Digest(value.Header.CertificateID),
		Nonce:                  relay.Nonce(value.Header.Nonce),
		Ciphertext:             append([]byte(nil), value.Ciphertext...),
		Signature:              relay.Signature(value.Signature),
		EnvelopeDigest:         relay.Digest(protocol.EnvelopeDigest(value)),
	}
}

func toRelayAcknowledgement(value protocol.ProgressAcknowledgement, wire []byte) relay.Acknowledgement {
	return relay.Acknowledgement{
		ChannelID:              relay.ChannelID(value.ChannelID),
		EnvironmentID:          relay.EnvironmentID(value.EnvironmentID),
		MembershipGeneration:   value.MembershipGeneration,
		AppliedArrivalSequence: value.AppliedArrivalSequence,
		ProducerSequence:       value.ProducerSequence,
		ProducerEnvelopeDigest: relay.Digest(value.ProducerEnvelopeDigest),
		CertificateID:          relay.Digest(value.CertificateID),
		AcknowledgementDigest:  relay.Digest(protocol.ProgressAcknowledgementDigest(value)),
		AcknowledgementBytes:   append([]byte(nil), wire...),
	}
}

func toRelayRetirement(value protocol.TerminalRetirement, wire []byte) relay.Retirement {
	return relay.Retirement{
		ChannelID:                relay.ChannelID(value.ChannelID),
		RelayGeneration:          relay.RelayGeneration(value.RelayGeneration),
		EnvironmentID:            relay.EnvironmentID(value.EnvironmentID),
		CertificateID:            relay.Digest(value.CertificateID),
		MembershipGeneration:     value.MembershipGeneration,
		FinalEnvironmentSequence: value.FinalEnvironmentSequence,
		FinalEnvelopeDigest:      relay.Digest(value.FinalEnvelopeDigest),
		RetirementID:             relay.Digest(protocol.TerminalRetirementID(value)),
		RetirementBytes:          append([]byte(nil), wire...),
	}
}

func toRelayPrune(value protocol.PruneCertificate, wire []byte) relay.PruneCertificate {
	return relay.PruneCertificate{
		ChannelID:            relay.ChannelID(value.ChannelID),
		PruneID:              relay.Digest(value.PruneID),
		MembershipGeneration: value.MembershipGeneration,
		Barrier:              value.BarrierArrivalSequence,
		Closure:              toRelayPruneTarget(value.Closure),
		CertificateID:        relay.Digest(protocol.PruneCertificateID(value)),
		CertificateBytes:     append([]byte(nil), wire...),
		Targets:              mapRelayPruneTargets(value.Manifest.Targets),
	}
}

func toRelayPruneTarget(value protocol.PruneReference) relay.PruneTarget {
	return relay.PruneTarget{
		FactID:                 relay.FactID(value.FactID),
		EnvironmentID:          relay.EnvironmentID(value.EnvironmentID),
		EnvironmentSequence:    value.EnvironmentSequence,
		ArrivalSequence:        value.ArrivalSequence,
		EnvelopeDigest:         relay.Digest(value.EnvelopeDigest),
		CertificateID:          relay.Digest(value.CertificateID),
		PreviousEnvelopeDigest: relay.Digest(value.PreviousEnvelopeDigest),
		KeyGeneration:          value.KeyGeneration,
		Nonce:                  relay.Nonce(value.Nonce),
	}
}

func mapRelayPruneTargets(values []protocol.PruneReference) []relay.PruneTarget {
	targets := make([]relay.PruneTarget, len(values))
	for index, value := range values {
		targets[index] = toRelayPruneTarget(value)
	}
	return targets
}

func cloneRelayEnvelope(value relay.Envelope) relay.Envelope {
	value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	return value
}

func cloneRelayAcknowledgement(value relay.Acknowledgement) relay.Acknowledgement {
	value.AcknowledgementBytes = append([]byte(nil), value.AcknowledgementBytes...)
	return value
}

func cloneRelayRetirement(value relay.Retirement) relay.Retirement {
	value.RetirementBytes = append([]byte(nil), value.RetirementBytes...)
	return value
}

func cloneRelayPruneCertificate(value relay.PruneCertificate) relay.PruneCertificate {
	value.CertificateBytes = append([]byte(nil), value.CertificateBytes...)
	value.Targets = append([]relay.PruneTarget(nil), value.Targets...)
	return value
}

func cloneProtocolPruneCertificate(value protocol.PruneCertificate) protocol.PruneCertificate {
	value.Manifest.Targets = append([]protocol.PruneReference(nil), value.Manifest.Targets...)
	value.Capsule.Ciphertext = append([]byte(nil), value.Capsule.Ciphertext...)
	value.Acknowledgements = append([]protocol.PruneAcknowledgement(nil), value.Acknowledgements...)
	return value
}

func clonePruneAuthority(value relay.PruneAuthority) relay.PruneAuthority {
	value.Environments = append([]relay.EnvironmentAuthority(nil), value.Environments...)
	value.Acknowledgements = make([]relay.Acknowledgement, len(value.Acknowledgements))
	for index, acknowledgement := range value.Acknowledgements {
		value.Acknowledgements[index] = cloneRelayAcknowledgement(acknowledgement)
	}
	return value
}

func seededAdmin(start byte) synccrypto.AdminSeed {
	var value synccrypto.AdminSeed
	for index := range value {
		value[index] = start + byte(index)
	}
	return value
}

func seededEnvironment(start byte) synccrypto.EnvironmentSeed {
	var value synccrypto.EnvironmentSeed
	for index := range value {
		value[index] = start + byte(index)
	}
	return value
}

func digest(start byte) protocol.Digest {
	var value protocol.Digest
	for index := range value {
		value[index] = start + byte(index)
	}
	return value
}

func nonce(start byte) protocol.Nonce {
	var value protocol.Nonce
	for index := range value {
		value[index] = start + byte(index)
	}
	return value
}
