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
