package relay

import (
	"bytes"
	"errors"
	"testing"
)

func TestRelayTokenSecretHashRequiresHighEntropyInputAndVerifiesPresentedSecret(t *testing.T) {
	t.Parallel()

	if _, err := HashTokenSecret(RelayTokenSecret{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("HashTokenSecret(zero) error = %v, want ErrInvalidArgument", err)
	}

	var secret RelayTokenSecret
	var wrong RelayTokenSecret
	for index := range secret {
		secret[index] = 0x42
		wrong[index] = 0x24
	}
	hash, err := HashTokenSecret(secret)
	if err != nil {
		t.Fatalf("HashTokenSecret() error = %v", err)
	}
	if !VerifyTokenSecret(hash, secret) {
		t.Fatal("VerifyTokenSecret(correct) = false, want true")
	}
	if VerifyTokenSecret(hash, wrong) {
		t.Fatal("VerifyTokenSecret(wrong) = true, want false")
	}
	if VerifyTokenSecret(hash, RelayTokenSecret{}) {
		t.Fatal("VerifyTokenSecret(empty) = true, want false")
	}
}

func TestRelayEnvelopeValidationEnforcesProtocolBoundsAndRequiredRoutingFields(t *testing.T) {
	t.Parallel()

	valid := testValidEnvelope()
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate(valid) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{name: "unsupported protocol", mutate: func(value *Envelope) { value.ProtocolVersion++ }},
		{name: "unsupported suite", mutate: func(value *Envelope) { value.CipherSuite++ }},
		{name: "zero channel", mutate: func(value *Envelope) { value.ChannelID = ChannelID{} }},
		{name: "zero certificate", mutate: func(value *Envelope) { value.CertificateID = Digest{} }},
		{name: "zero signature", mutate: func(value *Envelope) { value.Signature = Signature{} }},
		{name: "short ciphertext", mutate: func(value *Envelope) { value.Ciphertext = make([]byte, MinimumCiphertextBytes-1) }},
		{name: "oversized ciphertext", mutate: func(value *Envelope) { value.Ciphertext = make([]byte, MaxCiphertextBytes+1) }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := valid
			test.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestRelayEnvelopeValidationAcceptsZeroNonceAsCanonicalProtocolValue(t *testing.T) {
	t.Parallel()

	envelope := testValidEnvelope()
	envelope.Nonce = Nonce{}
	if err := envelope.Validate(); err != nil {
		t.Fatalf("Validate(zero nonce) error = %v, want nil", err)
	}
}

func TestRelayAcknowledgementValidationBindsProducerSequenceToEnvelopeDigest(t *testing.T) {
	t.Parallel()

	envelope := testValidEnvelope()
	acknowledgement := Acknowledgement{
		ChannelID:              envelope.ChannelID,
		EnvironmentID:          envelope.EnvironmentID,
		MembershipGeneration:   1,
		AppliedArrivalSequence: 0,
		ProducerSequence:       0,
		CertificateID:          envelope.CertificateID,
		AcknowledgementDigest:  envelope.EnvelopeDigest,
		AcknowledgementBytes:   []byte("opaque-signed-acknowledgement"),
	}
	if err := acknowledgement.Validate(); err != nil {
		t.Fatalf("Validate(zero producer frontier) error = %v", err)
	}

	acknowledgement.ProducerSequence = 1
	if err := acknowledgement.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate(nonzero producer with zero digest) error = %v, want ErrInvalidArgument", err)
	}
	acknowledgement.ProducerEnvelopeDigest = envelope.EnvelopeDigest
	if err := acknowledgement.Validate(); err != nil {
		t.Fatalf("Validate(nonzero producer frontier) error = %v", err)
	}
	acknowledgement.ProducerSequence = 0
	if err := acknowledgement.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate(zero producer with nonzero digest) error = %v, want ErrInvalidArgument", err)
	}

	acknowledgement.ProducerEnvelopeDigest = Digest{}
	acknowledgement.AcknowledgementBytes = make([]byte, MaxAcknowledgementBytes+1)
	if err := acknowledgement.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate(oversized acknowledgement) error = %v, want ErrInvalidArgument", err)
	}
}

func TestRelayRetirementValidationEnforcesCanonicalControlBound(t *testing.T) {
	t.Parallel()

	envelope := testValidEnvelope()
	retirement := Retirement{
		ChannelID:            envelope.ChannelID,
		RelayGeneration:      RelayGeneration(envelope.EnvelopeDigest),
		EnvironmentID:        envelope.EnvironmentID,
		CertificateID:        envelope.CertificateID,
		MembershipGeneration: 2,
		RetirementID:         envelope.EnvelopeDigest,
		RetirementBytes:      []byte("opaque-signed-terminal-retirement"),
	}
	if err := retirement.Validate(); err != nil {
		t.Fatalf("Validate(valid retirement) error = %v", err)
	}
	retirement.RetirementBytes = make([]byte, MaxRetirementBytes+1)
	if err := retirement.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate(oversized retirement) error = %v, want ErrInvalidArgument", err)
	}
}

func TestRelayPruneCertificateValidationRequiresDisjointClosureReference(t *testing.T) {
	t.Parallel()

	envelope := testValidEnvelope()
	target := PruneTarget{
		FactID:                 envelope.FactID,
		EnvironmentID:          envelope.EnvironmentID,
		EnvironmentSequence:    1,
		ArrivalSequence:        1,
		EnvelopeDigest:         envelope.EnvelopeDigest,
		CertificateID:          envelope.CertificateID,
		PreviousEnvelopeDigest: envelope.PreviousEnvelopeDigest,
		KeyGeneration:          envelope.KeyGeneration,
		Nonce:                  envelope.Nonce,
	}
	closure := target
	closure.FactID = "fact-close"
	closure.EnvironmentSequence = 2
	closure.ArrivalSequence = 2
	closure.PreviousEnvelopeDigest = envelope.EnvelopeDigest
	closure.EnvelopeDigest[0] ^= 0xff
	certificate := PruneCertificate{
		ChannelID:            envelope.ChannelID,
		PruneID:              closure.EnvelopeDigest,
		MembershipGeneration: 1,
		Barrier:              2,
		Closure:              closure,
		CertificateID:        envelope.EnvelopeDigest,
		CertificateBytes:     []byte("opaque-signed-prune-certificate"),
		Targets:              []PruneTarget{target},
	}
	if err := certificate.Validate(); err != nil {
		t.Fatalf("Validate(valid prune certificate) error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*PruneCertificate)
	}{
		{name: "zero closure", mutate: func(value *PruneCertificate) { value.Closure = PruneTarget{} }},
		{name: "zero target key generation", mutate: func(value *PruneCertificate) { value.Targets[0].KeyGeneration = 0 }},
		{name: "target first envelope has previous digest", mutate: func(value *PruneCertificate) {
			value.Targets[0].PreviousEnvelopeDigest = envelope.EnvelopeDigest
		}},
		{name: "target later envelope lacks previous digest", mutate: func(value *PruneCertificate) {
			value.Targets[0].EnvironmentSequence = 2
			value.Targets[0].PreviousEnvelopeDigest = Digest{}
		}},
		{name: "closure above barrier", mutate: func(value *PruneCertificate) { value.Closure.ArrivalSequence = 3 }},
		{name: "closure target arrival", mutate: func(value *PruneCertificate) { value.Closure.ArrivalSequence = 1 }},
		{name: "closure target fact", mutate: func(value *PruneCertificate) { value.Closure.FactID = value.Targets[0].FactID }},
		{name: "closure target source", mutate: func(value *PruneCertificate) {
			value.Closure.EnvironmentID = value.Targets[0].EnvironmentID
			value.Closure.EnvironmentSequence = value.Targets[0].EnvironmentSequence
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := certificate
			candidate.Targets = append([]PruneTarget(nil), certificate.Targets...)
			test.mutate(&candidate)
			if err := candidate.Validate(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func testValidEnvelope() Envelope {
	var channel ChannelID
	var certificate, envelopeDigest Digest
	var nonce Nonce
	var signature Signature
	for index := range channel {
		channel[index] = byte(index + 1)
		certificate[index] = byte(index + 2)
		envelopeDigest[index] = byte(index + 3)
	}
	for index := range nonce {
		nonce[index] = byte(index + 4)
	}
	for index := range signature {
		signature[index] = byte(index + 5)
	}
	return Envelope{
		ProtocolVersion:     ProtocolVersionV1,
		CipherSuite:         CipherSuiteV1,
		ChannelID:           channel,
		FactID:              "fact-a",
		EnvironmentID:       "environment-a",
		EnvironmentSequence: 1,
		KeyGeneration:       1,
		CertificateID:       certificate,
		Nonce:               nonce,
		Ciphertext:          bytes.Repeat([]byte{0x42}, MinimumCiphertextBytes),
		Signature:           signature,
		EnvelopeDigest:      envelopeDigest,
	}
}
