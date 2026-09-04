package crypto

import (
	"crypto/ed25519"
	"crypto/rand"

	"github.com/levifig/loaf/vnext/internal/continuitywire"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

// SealFact canonically encodes, encrypts, and environment-signs one exact
// continuity fact. It generates a fresh random nonce and returns immutable bytes
// that callers must persist before first upload.
func SealFact(
	fact continuitywire.Fact,
	key GenerationKey,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
	environmentSeed EnvironmentSeed,
	previousEnvelopeDigest protocol.Digest,
	atMillis int64,
) (protocol.SealedFact, error) {
	if err := continuitywire.Validate(fact); err != nil {
		return protocol.SealedFact{}, ErrInvalidPlaintext
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return protocol.SealedFact{}, err
	}
	if err := certificate.ValidateAt(atMillis); err != nil {
		return protocol.SealedFact{}, err
	}
	if err := validateSealAuthority(fact, key, certificate, environmentSeed); err != nil {
		return protocol.SealedFact{}, err
	}
	plaintext, err := continuitywire.Encode(fact)
	if err != nil || len(plaintext) > protocol.MaxPlaintextBytes {
		return protocol.SealedFact{}, ErrInvalidPlaintext
	}
	var nonce protocol.Nonce
	if _, err := rand.Read(nonce[:]); err != nil {
		return protocol.SealedFact{}, ErrRandomSource
	}
	header := protocol.FactHeader{
		ProtocolVersion:        protocol.ProtocolVersionV1,
		CipherSuite:            protocol.CipherSuiteXChaCha20Poly1305,
		ChannelID:              certificate.ChannelID,
		FactID:                 fact.FactID,
		EnvironmentID:          fact.EnvironmentID,
		EnvironmentSequence:    fact.EnvironmentSequence,
		KeyGeneration:          key.Generation(),
		PreviousEnvelopeDigest: previousEnvelopeDigest,
		CertificateID:          protocol.CertificateID(certificate),
		Nonce:                  nonce,
	}
	return sealEncoded(header, plaintext, key, environmentSeed)
}

func validateSealAuthority(fact continuitywire.Fact, key GenerationKey, certificate protocol.EnvironmentCertificate, environmentSeed EnvironmentSeed) error {
	if key.generation < 1 || zeroBytes(key.material[:]) {
		return ErrInvalidGenerationKey
	}
	if zeroBytes(environmentSeed[:]) {
		return ErrInvalidSigningKey
	}
	if fact.ProjectID != certificate.ProjectID || fact.EnvironmentID != certificate.EnvironmentID || !seedMatchesPublic(environmentSeed[:], certificate.EnvironmentPublicKey) {
		return ErrCertificateBinding
	}
	if key.projectID != fact.ProjectID || key.cipherSuite != certificate.CipherSuite {
		return ErrGenerationBinding
	}
	if !certificate.AllowsGeneration(key.Generation()) {
		return ErrGenerationNotAllowed
	}
	return nil
}

func sealEncoded(header protocol.FactHeader, plaintext []byte, key GenerationKey, environmentSeed EnvironmentSeed) (protocol.SealedFact, error) {
	if err := header.Validate(); err != nil {
		return protocol.SealedFact{}, err
	}
	if key.generation != header.KeyGeneration || zeroBytes(key.material[:]) {
		return protocol.SealedFact{}, ErrInvalidGenerationKey
	}
	if len(plaintext) < 1 || len(plaintext) > protocol.MaxPlaintextBytes {
		return protocol.SealedFact{}, ErrInvalidPlaintext
	}
	if zeroBytes(environmentSeed[:]) {
		return protocol.SealedFact{}, ErrInvalidSigningKey
	}
	aead, err := newXChaCha(key)
	if err != nil {
		return protocol.SealedFact{}, ErrInvalidGenerationKey
	}
	aad, err := protocol.AADTranscript(header)
	if err != nil {
		return protocol.SealedFact{}, err
	}
	ciphertext := aead.Seal(nil, header.Nonce[:], plaintext, aad)
	transcript, err := protocol.FactSignatureTranscript(header, ciphertext)
	if err != nil {
		return protocol.SealedFact{}, err
	}
	signed := ed25519.Sign(ed25519.NewKeyFromSeed(environmentSeed[:]), transcript)
	sealed := protocol.SealedFact{Header: header, Ciphertext: ciphertext}
	copy(sealed.Signature[:], signed)
	if err := sealed.Validate(); err != nil {
		return protocol.SealedFact{}, err
	}
	return sealed, nil
}

// OpenFact opens an already-retained historical fact. It verifies project-admin
// and environment signatures before AEAD-open, strictly decodes the continuity
// wire, and checks every duplicated binding. It intentionally does not apply
// certificate expiry: expiry gates new admission, not retained history.
func OpenFact(
	sealed protocol.SealedFact,
	key GenerationKey,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) (continuitywire.Fact, error) {
	if err := sealed.Validate(); err != nil {
		return continuitywire.Fact{}, err
	}
	if key.generation != sealed.Header.KeyGeneration || zeroBytes(key.material[:]) {
		return continuitywire.Fact{}, ErrInvalidGenerationKey
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return continuitywire.Fact{}, err
	}
	if key.projectID != certificate.ProjectID || key.cipherSuite != sealed.Header.CipherSuite {
		return continuitywire.Fact{}, ErrGenerationBinding
	}
	if err := verifySealedFactSignatureWithVerifiedCertificate(sealed, certificate); err != nil {
		return continuitywire.Fact{}, err
	}
	aead, err := newXChaCha(key)
	if err != nil {
		return continuitywire.Fact{}, ErrInvalidGenerationKey
	}
	aad, err := protocol.AADTranscript(sealed.Header)
	if err != nil {
		return continuitywire.Fact{}, err
	}
	plaintext, err := aead.Open(nil, sealed.Header.Nonce[:], sealed.Ciphertext, aad)
	if err != nil {
		return continuitywire.Fact{}, ErrAuthenticationFailed
	}
	fact, err := continuitywire.Decode(plaintext)
	if err != nil {
		return continuitywire.Fact{}, ErrInvalidPlaintext
	}
	if fact.FactID != sealed.Header.FactID ||
		fact.ProjectID != certificate.ProjectID ||
		fact.EnvironmentID != sealed.Header.EnvironmentID ||
		fact.EnvironmentSequence != sealed.Header.EnvironmentSequence {
		return continuitywire.Fact{}, ErrPlaintextBinding
	}
	return fact, nil
}

// VerifySealedFactSignature verifies the opaque envelope structure, its
// administrator-certified environment identity, exact certificate bindings,
// and environment signature. It intentionally does not require generation-key
// material, apply certificate expiry, decrypt ciphertext, or inspect the
// continuity plaintext, so an opaque relay can authenticate an envelope
// without learning its contents.
func VerifySealedFactSignature(
	sealed protocol.SealedFact,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
) error {
	if err := sealed.Validate(); err != nil {
		return err
	}
	if err := VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return err
	}
	return verifySealedFactSignatureWithVerifiedCertificate(sealed, certificate)
}

func verifySealedFactSignatureWithVerifiedCertificate(
	sealed protocol.SealedFact,
	certificate protocol.EnvironmentCertificate,
) error {
	if err := verifyEnvelopeBindings(sealed.Header, certificate); err != nil {
		return err
	}
	transcript, err := protocol.FactSignatureTranscript(sealed.Header, sealed.Ciphertext)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(certificate.EnvironmentPublicKey[:]), transcript, sealed.Signature[:]) {
		return ErrInvalidEnvironmentSignature
	}
	return nil
}

// OpenFactForAdmission applies certificate expiry at a trusted Unix-millisecond
// admission time before opening. New relay/client admissions must use this
// entry point; historical rebuilds use OpenFact.
func OpenFactForAdmission(
	sealed protocol.SealedFact,
	key GenerationKey,
	certificate protocol.EnvironmentCertificate,
	adminPublic protocol.PublicKey,
	atMillis int64,
) (continuitywire.Fact, error) {
	if err := certificate.ValidateAt(atMillis); err != nil {
		return continuitywire.Fact{}, err
	}
	return OpenFact(sealed, key, certificate, adminPublic)
}

func verifyEnvelopeBindings(header protocol.FactHeader, certificate protocol.EnvironmentCertificate) error {
	if header.ProtocolVersion != certificate.ProtocolVersion ||
		header.CipherSuite != certificate.CipherSuite ||
		header.ChannelID != certificate.ChannelID ||
		header.EnvironmentID != certificate.EnvironmentID ||
		header.CertificateID != protocol.CertificateID(certificate) {
		return ErrCertificateBinding
	}
	if !certificate.AllowsGeneration(header.KeyGeneration) {
		return ErrGenerationNotAllowed
	}
	return nil
}
