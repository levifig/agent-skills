// Package crypto implements the fixed cryptographic suite selected by ADR-031.
package crypto

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

var (
	// ErrRandomSource reports failure of the operating-system random source.
	ErrRandomSource = errors.New("sync crypto: secure random source failed")
	// ErrInvalidProjectRoot reports missing or malformed project root material.
	ErrInvalidProjectRoot = errors.New("sync crypto: invalid project root")
	// ErrInvalidGenerationKey reports missing or malformed generation material.
	ErrInvalidGenerationKey = errors.New("sync crypto: invalid generation key")
	// ErrKeyDerivation reports a failed or invalid HKDF operation.
	ErrKeyDerivation = errors.New("sync crypto: generation key derivation failed")
	// ErrInvalidSigningKey reports missing or malformed Ed25519 seed material.
	ErrInvalidSigningKey = errors.New("sync crypto: invalid signing key")
	// ErrInvalidCertificateSignature reports a failed project-admin signature.
	ErrInvalidCertificateSignature = errors.New("sync crypto: invalid environment certificate signature")
	// ErrAuthorityKeyReuse reports reuse of one Ed25519 identity for both the
	// project administrator and an attached environment.
	ErrAuthorityKeyReuse = errors.New("sync crypto: admin and environment signing keys must be distinct")
	// ErrInvalidEnvironmentSignature reports a failed environment signature.
	ErrInvalidEnvironmentSignature = errors.New("sync crypto: invalid sealed fact signature")
	// ErrCertificateBinding reports a mismatch between a certificate and fact.
	ErrCertificateBinding = errors.New("sync crypto: environment certificate binding mismatch")
	// ErrGenerationNotAllowed reports use of a generation outside a certificate.
	ErrGenerationNotAllowed = errors.New("sync crypto: key generation is not authorized")
	// ErrGenerationBinding reports a project or suite mismatch for key material.
	ErrGenerationBinding = errors.New("sync crypto: generation key binding mismatch")
	// ErrAuthenticationFailed reports an AEAD authentication failure.
	ErrAuthenticationFailed = errors.New("sync crypto: ciphertext authentication failed")
	// ErrPlaintextBinding reports disagreement between outer and inner fact data.
	ErrPlaintextBinding = errors.New("sync crypto: plaintext fact binding mismatch")
	// ErrInvalidPlaintext reports a noncanonical or unsupported continuity fact.
	ErrInvalidPlaintext = errors.New("sync crypto: invalid continuity plaintext")
)

// ProjectRoot is one project's independent HKDF input keying material.
type ProjectRoot [32]byte

// String prevents accidental plaintext formatting of project root material.
func (ProjectRoot) String() string { return "[REDACTED project root]" }

// GoString prevents %#v from formatting project root material.
func (ProjectRoot) GoString() string { return "crypto.ProjectRoot([REDACTED])" }

// Bytes returns a copy for protected credential serialization.
func (root ProjectRoot) Bytes() [32]byte { return [32]byte(root) }

// ProjectRootFromBytes validates and copies project root material.
func ProjectRootFromBytes(material []byte) (ProjectRoot, error) {
	if len(material) != len(ProjectRoot{}) || zeroBytes(material) {
		return ProjectRoot{}, ErrInvalidProjectRoot
	}
	var root ProjectRoot
	copy(root[:], material)
	return root, nil
}

// GenerateProjectRoot generates a fresh independent project root.
func GenerateProjectRoot() (ProjectRoot, error) {
	var root ProjectRoot
	if _, err := rand.Read(root[:]); err != nil {
		return ProjectRoot{}, ErrRandomSource
	}
	return root, nil
}

// GenerateChannelID generates a fresh random opaque relay channel.
func GenerateChannelID() (protocol.ChannelID, error) {
	var channel protocol.ChannelID
	if _, err := rand.Read(channel[:]); err != nil {
		return protocol.ChannelID{}, ErrRandomSource
	}
	return channel, nil
}

// GenerateRelayGeneration generates a fresh relay-database incarnation ID. It
// is independent metadata and is never derived from project authority.
func GenerateRelayGeneration() (protocol.RelayGeneration, error) {
	var generation protocol.RelayGeneration
	if _, err := rand.Read(generation[:]); err != nil {
		return protocol.RelayGeneration{}, ErrRandomSource
	}
	return generation, nil
}

// GenerationKey pairs one explicit generation with its AEAD key bytes.
type GenerationKey struct {
	projectID   continuity.ProjectID
	cipherSuite uint16
	generation  uint32
	material    [32]byte
}

// String prevents accidental plaintext formatting of generation key material.
func (GenerationKey) String() string { return "[REDACTED generation key]" }

// GoString prevents %#v from formatting generation key material.
func (GenerationKey) GoString() string { return "crypto.GenerationKey([REDACTED])" }

// Generation returns the one generation selected by this key.
func (key GenerationKey) Generation() uint32 { return key.generation }

// ProjectID returns the one project bound to this key.
func (key GenerationKey) ProjectID() continuity.ProjectID { return key.projectID }

// CipherSuite returns the one protocol suite bound to this key.
func (key GenerationKey) CipherSuite() uint16 { return key.cipherSuite }

// Bytes returns a copy for protected credential serialization.
func (key GenerationKey) Bytes() [32]byte { return key.material }

// NewGenerationKey validates explicit generation material, including keys
// carried by ephemeral credentials.
func NewGenerationKey(projectID continuity.ProjectID, generation uint32, material [32]byte) (GenerationKey, error) {
	if projectID.Validate() != nil || generation < 1 || zeroBytes(material[:]) {
		return GenerationKey{}, ErrInvalidGenerationKey
	}
	return GenerationKey{
		projectID:   projectID,
		cipherSuite: protocol.CipherSuiteXChaCha20Poly1305,
		generation:  generation,
		material:    material,
	}, nil
}

// DeriveGenerationKey derives exactly one project/suite/generation key with
// HKDF-SHA-256. It never searches or trials a key ring.
func DeriveGenerationKey(root ProjectRoot, projectID continuity.ProjectID, generation uint32) (GenerationKey, error) {
	if zeroBytes(root[:]) || generation < 1 {
		return GenerationKey{}, ErrKeyDerivation
	}
	salt, err := protocol.GenerationKeySalt(projectID)
	if err != nil {
		return GenerationKey{}, ErrKeyDerivation
	}
	info, err := protocol.GenerationKeyInfo(protocol.ProtocolVersionV1, protocol.CipherSuiteXChaCha20Poly1305, generation)
	if err != nil {
		return GenerationKey{}, ErrKeyDerivation
	}
	derived, err := hkdf.Key(sha256.New, root[:], salt, string(info), 32)
	if err != nil || len(derived) != 32 {
		return GenerationKey{}, ErrKeyDerivation
	}
	var material [32]byte
	copy(material[:], derived)
	return NewGenerationKey(projectID, generation, material)
}

// AdminSeed is one project administrator's Ed25519 seed.
type AdminSeed [32]byte

// String prevents accidental plaintext formatting of admin signing material.
func (AdminSeed) String() string { return "[REDACTED admin seed]" }

// GoString prevents %#v from formatting admin signing material.
func (AdminSeed) GoString() string { return "crypto.AdminSeed([REDACTED])" }

// Bytes returns a copy for protected recovery credential serialization.
func (seed AdminSeed) Bytes() [32]byte { return [32]byte(seed) }

// AdminSeedFromBytes validates and copies an Ed25519 administrator seed.
func AdminSeedFromBytes(material []byte) (AdminSeed, error) {
	if len(material) != len(AdminSeed{}) || zeroBytes(material) {
		return AdminSeed{}, ErrInvalidSigningKey
	}
	var seed AdminSeed
	copy(seed[:], material)
	return seed, nil
}

// GenerateAdminSeed generates a fresh project administrator seed.
func GenerateAdminSeed() (AdminSeed, error) {
	var seed AdminSeed
	if _, err := rand.Read(seed[:]); err != nil {
		return AdminSeed{}, ErrRandomSource
	}
	return seed, nil
}

// EnvironmentSeed is one environment's Ed25519 seed.
type EnvironmentSeed [32]byte

// String prevents accidental plaintext formatting of environment authority.
func (EnvironmentSeed) String() string { return "[REDACTED environment seed]" }

// GoString prevents %#v from formatting environment authority.
func (EnvironmentSeed) GoString() string { return "crypto.EnvironmentSeed([REDACTED])" }

// Bytes returns a copy for protected attach credential serialization.
func (seed EnvironmentSeed) Bytes() [32]byte { return [32]byte(seed) }

// EnvironmentSeedFromBytes validates and copies an Ed25519 environment seed.
func EnvironmentSeedFromBytes(material []byte) (EnvironmentSeed, error) {
	if len(material) != len(EnvironmentSeed{}) || zeroBytes(material) {
		return EnvironmentSeed{}, ErrInvalidSigningKey
	}
	var seed EnvironmentSeed
	copy(seed[:], material)
	return seed, nil
}

// GenerateEnvironmentSeed generates a fresh environment signing seed.
func GenerateEnvironmentSeed() (EnvironmentSeed, error) {
	var seed EnvironmentSeed
	if _, err := rand.Read(seed[:]); err != nil {
		return EnvironmentSeed{}, ErrRandomSource
	}
	return seed, nil
}

func zeroBytes(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
