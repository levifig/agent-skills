package crypto

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

var (
	// ErrInvalidPruneBootstrapKey identifies missing or malformed typed
	// bootstrap key material.
	ErrInvalidPruneBootstrapKey = errors.New("sync crypto: invalid prune bootstrap key")
	// ErrPruneBootstrapKeyDerivation identifies failure of either domain-
	// separated HKDF stage.
	ErrPruneBootstrapKeyDerivation = errors.New("sync crypto: prune bootstrap key derivation failed")
	// ErrPruneBootstrapKeyBinding identifies a mismatch between typed base-key
	// selectors and the capsule selectors. It never reports bound values.
	ErrPruneBootstrapKeyBinding = errors.New("sync crypto: prune bootstrap key binding mismatch")
	// ErrPruneBootstrapAuthenticationFailed collapses wrong key, project, AAD,
	// ciphertext, and authentication-tag failures into one secret-free class.
	ErrPruneBootstrapAuthenticationFailed = errors.New("sync crypto: prune bootstrap authentication failed")
	// ErrPruneBootstrapPlaintextBinding identifies an authenticated disagreement
	// between duplicated outer and inner fields.
	ErrPruneBootstrapPlaintextBinding = errors.New("sync crypto: prune bootstrap plaintext binding mismatch")
)

// PruneBootstrapKey is the stable project-scoped base authority used only for
// deletion-anchor metadata. It is computationally independent of content
// GenerationKey values.
type PruneBootstrapKey struct {
	projectID       continuity.ProjectID
	protocolVersion uint16
	cipherSuite     uint16
	purposeVersion  uint16
	material        [32]byte
}

// String prevents accidental plaintext formatting of bootstrap key material.
func (PruneBootstrapKey) String() string { return "[REDACTED prune bootstrap key]" }

// GoString prevents %#v from formatting bootstrap key material.
func (PruneBootstrapKey) GoString() string {
	return "crypto.PruneBootstrapKey([REDACTED])"
}

// ProjectID returns the project identity bound to this key.
func (key PruneBootstrapKey) ProjectID() continuity.ProjectID { return key.projectID }

// ProtocolVersion returns the protocol selector bound to this key.
func (key PruneBootstrapKey) ProtocolVersion() uint16 { return key.protocolVersion }

// CipherSuite returns the cryptographic suite bound to this key.
func (key PruneBootstrapKey) CipherSuite() uint16 { return key.cipherSuite }

// PurposeVersion returns the stable prune-bootstrap derivation purpose.
func (key PruneBootstrapKey) PurposeVersion() uint16 { return key.purposeVersion }

// Bytes returns a copy for protected credential serialization.
func (key PruneBootstrapKey) Bytes() [32]byte { return key.material }

// NewPruneBootstrapKey validates explicit bootstrap material carried by a
// credential. Protocol and suite selectors are fixed by the purpose version.
func NewPruneBootstrapKey(
	projectID continuity.ProjectID,
	purposeVersion uint16,
	material [32]byte,
) (PruneBootstrapKey, error) {
	if projectID.Validate() != nil || purposeVersion != protocol.PruneBootstrapPurposeVersionV1 || zeroBytes(material[:]) {
		return PruneBootstrapKey{}, ErrInvalidPruneBootstrapKey
	}
	return PruneBootstrapKey{
		projectID:       projectID,
		protocolVersion: protocol.ProtocolVersionV1,
		cipherSuite:     protocol.CipherSuiteXChaCha20Poly1305,
		purposeVersion:  purposeVersion,
		material:        material,
	}, nil
}

// DerivePruneBootstrapKey derives the stable typed base key directly from one
// ProjectRoot through a domain distinct from content-generation keys.
func DerivePruneBootstrapKey(
	root ProjectRoot,
	projectID continuity.ProjectID,
	purposeVersion uint16,
) (PruneBootstrapKey, error) {
	if zeroBytes(root[:]) || purposeVersion != protocol.PruneBootstrapPurposeVersionV1 {
		return PruneBootstrapKey{}, ErrPruneBootstrapKeyDerivation
	}
	salt, err := protocol.PruneBootstrapKeySalt(projectID)
	if err != nil {
		return PruneBootstrapKey{}, ErrPruneBootstrapKeyDerivation
	}
	info, err := protocol.PruneBootstrapKeyInfo(
		protocol.ProtocolVersionV1,
		protocol.CipherSuiteXChaCha20Poly1305,
		purposeVersion,
	)
	if err != nil {
		return PruneBootstrapKey{}, ErrPruneBootstrapKeyDerivation
	}
	derived, err := hkdf.Key(sha256.New, root[:], salt, string(info), 32)
	if err != nil || len(derived) != 32 {
		return PruneBootstrapKey{}, ErrPruneBootstrapKeyDerivation
	}
	var material [32]byte
	copy(material[:], derived)
	key, err := NewPruneBootstrapKey(projectID, purposeVersion, material)
	if err != nil {
		return PruneBootstrapKey{}, ErrPruneBootstrapKeyDerivation
	}
	return key, nil
}

// pruneBootstrapAEADKey is one non-serializable, exact-capsule-bound
// XChaCha20-Poly1305 key derived from the stable bootstrap base.
type pruneBootstrapAEADKey struct {
	projectID              continuity.ProjectID
	capsuleVersion         uint16
	protocolVersion        uint16
	cipherSuite            uint16
	purposeVersion         uint16
	channelID              protocol.ChannelID
	relayGeneration        protocol.RelayGeneration
	pruneID                protocol.Digest
	membershipGeneration   uint32
	barrierArrivalSequence int64
	closureReferenceDigest protocol.Digest
	manifestCount          uint32
	manifestDigest         protocol.Digest
	material               [32]byte
}

// String prevents accidental plaintext formatting of per-prune key material.
func (pruneBootstrapAEADKey) String() string {
	return "[REDACTED prune bootstrap AEAD key]"
}

// GoString prevents %#v from formatting per-prune key material.
func (pruneBootstrapAEADKey) GoString() string {
	return "crypto.pruneBootstrapAEADKey([REDACTED])"
}

// derivePruneBootstrapAEADKey performs the second typed HKDF stage. The salt
// binds project/channel/relay/prune; info binds every remaining stable outer
// field. Nonce and ciphertext are intentionally excluded from key derivation.
func derivePruneBootstrapAEADKey(
	base PruneBootstrapKey,
	capsule protocol.PruneBootstrap,
) (pruneBootstrapAEADKey, error) {
	if err := validatePruneBootstrapKey(base); err != nil {
		return pruneBootstrapAEADKey{}, err
	}
	if _, err := protocol.PruneBootstrapAAD(capsule, base.projectID); err != nil {
		return pruneBootstrapAEADKey{}, err
	}
	if base.protocolVersion != capsule.ProtocolVersion || base.cipherSuite != capsule.CipherSuite ||
		base.purposeVersion != capsule.BootstrapPurposeVersion {
		return pruneBootstrapAEADKey{}, ErrPruneBootstrapKeyBinding
	}
	salt, err := protocol.PruneBootstrapAEADKeySalt(
		base.projectID,
		capsule.ChannelID,
		capsule.RelayGeneration,
		capsule.PruneID,
	)
	if err != nil {
		return pruneBootstrapAEADKey{}, ErrPruneBootstrapKeyDerivation
	}
	info, err := protocol.PruneBootstrapAEADKeyInfo(
		capsule.CapsuleVersion,
		capsule.ProtocolVersion,
		capsule.CipherSuite,
		capsule.BootstrapPurposeVersion,
		capsule.MembershipGeneration,
		capsule.BarrierArrivalSequence,
		capsule.ClosureReferenceDigest,
		capsule.ManifestCount,
		capsule.ManifestDigest,
	)
	if err != nil {
		return pruneBootstrapAEADKey{}, ErrPruneBootstrapKeyDerivation
	}
	derived, err := hkdf.Key(sha256.New, base.material[:], salt, string(info), 32)
	if err != nil || len(derived) != 32 {
		return pruneBootstrapAEADKey{}, ErrPruneBootstrapKeyDerivation
	}
	key := pruneBootstrapAEADKey{
		projectID:              base.projectID,
		capsuleVersion:         capsule.CapsuleVersion,
		protocolVersion:        capsule.ProtocolVersion,
		cipherSuite:            capsule.CipherSuite,
		purposeVersion:         capsule.BootstrapPurposeVersion,
		channelID:              capsule.ChannelID,
		relayGeneration:        capsule.RelayGeneration,
		pruneID:                capsule.PruneID,
		membershipGeneration:   capsule.MembershipGeneration,
		barrierArrivalSequence: capsule.BarrierArrivalSequence,
		closureReferenceDigest: capsule.ClosureReferenceDigest,
		manifestCount:          capsule.ManifestCount,
		manifestDigest:         capsule.ManifestDigest,
	}
	copy(key.material[:], derived)
	return key, nil
}

func validatePruneBootstrapKey(key PruneBootstrapKey) error {
	if key.projectID.Validate() != nil || key.protocolVersion != protocol.ProtocolVersionV1 ||
		key.cipherSuite != protocol.CipherSuiteXChaCha20Poly1305 ||
		key.purposeVersion != protocol.PruneBootstrapPurposeVersionV1 || zeroBytes(key.material[:]) {
		return ErrInvalidPruneBootstrapKey
	}
	return nil
}

// GeneratePruneID mints an independent random prune identity before any
// manifest, capsule, key, or nonce derivation.
func GeneratePruneID() (protocol.Digest, error) {
	return generatePruneIDWithRandom(rand.Reader)
}

func generatePruneIDWithRandom(random io.Reader) (protocol.Digest, error) {
	if random == nil {
		return protocol.Digest{}, ErrRandomSource
	}
	var pruneID protocol.Digest
	if _, err := io.ReadFull(random, pruneID[:]); err != nil || zeroBytes(pruneID[:]) {
		return protocol.Digest{}, ErrRandomSource
	}
	return pruneID, nil
}
