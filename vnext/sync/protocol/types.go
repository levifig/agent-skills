// Package protocol defines the versioned, transport-neutral private-sync wire
// contract. Its binary encodings are independent of Go struct layout.
package protocol

import (
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	// ProtocolVersionV1 is the first private-sync envelope protocol.
	ProtocolVersionV1 uint16 = 1
	// CipherSuiteXChaCha20Poly1305 identifies HKDF-SHA-256 generation keys,
	// XChaCha20-Poly1305 fact encryption, and Ed25519 signatures.
	CipherSuiteXChaCha20Poly1305 uint16 = 1
	// CertificateVersionV1 is the first environment certificate format.
	CertificateVersionV1 uint16 = 1

	// MaxPlaintextBytes bounds one exact canonical continuity wire fact.
	MaxPlaintextBytes = 1_100_000
	// MaxCiphertextBytes includes the XChaCha20-Poly1305 authentication tag.
	MaxCiphertextBytes = MaxPlaintextBytes + 16
	// MaxEnvelopeBytes bounds one complete signed fact envelope.
	MaxEnvelopeBytes = 1_102_000
	// MaxCertificateBytes bounds one admin-signed environment certificate.
	MaxCertificateBytes = 8_192
	// MaxControlObjectBytes bounds one future signed membership or prune object.
	MaxControlObjectBytes = 1_048_576
	// MaxAllowedKeyGenerations bounds certificate verification work.
	MaxAllowedKeyGenerations = 64
	// MaxPageFrames bounds one staged pull/apply page before cryptographic work.
	MaxPageFrames = 256
)

var (
	// ErrUnsupportedProtocolVersion identifies an unknown fact protocol.
	ErrUnsupportedProtocolVersion = errors.New("sync protocol: unsupported protocol version")
	// ErrUnsupportedCipherSuite identifies an unknown cryptographic suite.
	ErrUnsupportedCipherSuite = errors.New("sync protocol: unsupported cipher suite")
	// ErrInvalidHeader identifies malformed authenticated routing metadata.
	ErrInvalidHeader = errors.New("sync protocol: invalid fact header")
	// ErrInvalidEnvelope identifies malformed or noncanonical sealed bytes.
	ErrInvalidEnvelope = errors.New("sync protocol: invalid sealed fact")
	// ErrInvalidCertificate identifies a malformed environment certificate.
	ErrInvalidCertificate = errors.New("sync protocol: invalid environment certificate")
	// ErrCertificateExpired identifies an environment used at or after expiry.
	ErrCertificateExpired = errors.New("sync protocol: environment certificate expired")
	// ErrTooLarge identifies an object that exceeds a fixed protocol limit.
	ErrTooLarge = errors.New("sync protocol: object exceeds fixed limit")
)

// ChannelID is a random opaque project relay channel identifier.
type ChannelID [32]byte

// RelayGeneration is a random opaque relay-database incarnation identifier.
// Clients pin it and treat replacement as recovery-required state.
type RelayGeneration [32]byte

// Digest is a SHA-256 digest used for immutable object identity and chaining.
type Digest [32]byte

// Nonce is one XChaCha20-Poly1305 nonce.
type Nonce [24]byte

// PublicKey is one Ed25519 public key.
type PublicKey [32]byte

// Signature is one Ed25519 signature.
type Signature [64]byte

// EnvironmentMode distinguishes durable and explicitly expiring replicas.
type EnvironmentMode uint8

const (
	// EnvironmentTrusted identifies a durable attached replica.
	EnvironmentTrusted EnvironmentMode = 1
	// EnvironmentEphemeral identifies an expiring, non-persisted replica.
	EnvironmentEphemeral EnvironmentMode = 2
)

// FactHeader is the complete authenticated but relay-visible routing header.
type FactHeader struct {
	ProtocolVersion        uint16
	CipherSuite            uint16
	ChannelID              ChannelID
	FactID                 continuity.FactID
	EnvironmentID          continuity.EnvironmentID
	EnvironmentSequence    int64
	KeyGeneration          uint32
	PreviousEnvelopeDigest Digest
	CertificateID          Digest
	Nonce                  Nonce
}

// Validate rejects unsupported or structurally invalid routing metadata.
func (header FactHeader) Validate() error {
	if header.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if header.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if isZero(header.ChannelID[:]) || header.FactID.Validate() != nil || header.EnvironmentID.Validate() != nil {
		return ErrInvalidHeader
	}
	if header.EnvironmentSequence < 1 || header.KeyGeneration < 1 || isZero(header.CertificateID[:]) {
		return ErrInvalidHeader
	}
	previousIsZero := isZero(header.PreviousEnvelopeDigest[:])
	if (header.EnvironmentSequence == 1) != previousIsZero {
		return ErrInvalidHeader
	}
	return nil
}

// SealedFact is one immutable signed E2E fact envelope.
type SealedFact struct {
	Header     FactHeader
	Ciphertext []byte
	Signature  Signature
}

// Validate rejects malformed envelope structure before cryptographic work.
func (fact SealedFact) Validate() error {
	if err := fact.Header.Validate(); err != nil {
		return err
	}
	if len(fact.Ciphertext) < 16 {
		return ErrInvalidEnvelope
	}
	if len(fact.Ciphertext) > MaxCiphertextBytes {
		return ErrTooLarge
	}
	return nil
}

// EnvironmentCertificate is one project-admin authorization for a mint-once
// environment signing identity.
type EnvironmentCertificate struct {
	Version               uint16
	ProtocolVersion       uint16
	CipherSuite           uint16
	ProjectID             continuity.ProjectID
	ChannelID             ChannelID
	EnvironmentID         continuity.EnvironmentID
	EnvironmentPublicKey  PublicKey
	Mode                  EnvironmentMode
	MembershipGeneration  uint32
	AllowedKeyGenerations []uint32
	ExpiresAtMillis       int64
	AdminSignature        Signature
}

// Validate rejects unsupported, ambiguous, or noncanonical certificate data.
func (certificate EnvironmentCertificate) Validate() error {
	if certificate.Version != CertificateVersionV1 {
		return ErrInvalidCertificate
	}
	if certificate.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if certificate.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if certificate.ProjectID.Validate() != nil || certificate.EnvironmentID.Validate() != nil || isZero(certificate.ChannelID[:]) || isZero(certificate.EnvironmentPublicKey[:]) {
		return ErrInvalidCertificate
	}
	if certificate.Mode != EnvironmentTrusted && certificate.Mode != EnvironmentEphemeral {
		return ErrInvalidCertificate
	}
	if certificate.MembershipGeneration < 1 || certificate.ExpiresAtMillis < 0 {
		return ErrInvalidCertificate
	}
	if certificate.Mode == EnvironmentEphemeral && certificate.ExpiresAtMillis == 0 {
		return ErrInvalidCertificate
	}
	if len(certificate.AllowedKeyGenerations) < 1 || len(certificate.AllowedKeyGenerations) > MaxAllowedKeyGenerations {
		return ErrInvalidCertificate
	}
	var previous uint32
	for index, generation := range certificate.AllowedKeyGenerations {
		if generation < 1 || (index > 0 && generation <= previous) {
			return ErrInvalidCertificate
		}
		previous = generation
	}
	return nil
}

// AllowsGeneration reports whether the signed certificate authorizes exactly
// this generation.
func (certificate EnvironmentCertificate) AllowsGeneration(generation uint32) bool {
	for _, allowed := range certificate.AllowedKeyGenerations {
		if allowed == generation {
			return true
		}
		if allowed > generation {
			return false
		}
	}
	return false
}

// ValidateAt applies the certificate's optional terminal expiry. Unix
// milliseconds at or after expiry are invalid.
func (certificate EnvironmentCertificate) ValidateAt(unixMillis int64) error {
	if err := certificate.Validate(); err != nil {
		return err
	}
	if unixMillis < 0 {
		return ErrInvalidCertificate
	}
	if certificate.ExpiresAtMillis != 0 && unixMillis >= certificate.ExpiresAtMillis {
		return ErrCertificateExpired
	}
	return nil
}

func isZero(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
