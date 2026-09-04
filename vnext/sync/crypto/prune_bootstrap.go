package crypto

import (
	"crypto/rand"
	"io"

	"github.com/levifig/loaf/vnext/sync/protocol"
)

// SealPruneBootstrap canonically encodes and seals one payload-free deletion
// anchor capsule. A fresh random nonce is generated for each call; persistence
// is responsible for retaining and retrying the first returned bytes exactly.
func SealPruneBootstrap(
	plaintext protocol.PruneBootstrapPlaintext,
	base PruneBootstrapKey,
) (protocol.PruneBootstrap, error) {
	return sealPruneBootstrapWithRandom(plaintext, base, rand.Reader)
}

func sealPruneBootstrapWithRandom(
	plaintext protocol.PruneBootstrapPlaintext,
	base PruneBootstrapKey,
	random io.Reader,
) (protocol.PruneBootstrap, error) {
	if err := plaintext.Validate(); err != nil {
		return protocol.PruneBootstrap{}, err
	}
	if err := validatePruneBootstrapKey(base); err != nil {
		return protocol.PruneBootstrap{}, err
	}
	if plaintext.ProjectID != base.projectID || plaintext.ProtocolVersion != base.protocolVersion ||
		plaintext.CipherSuite != base.cipherSuite || plaintext.BootstrapPurposeVersion != base.purposeVersion {
		return protocol.PruneBootstrap{}, ErrPruneBootstrapKeyBinding
	}
	encoded, err := plaintext.MarshalBinary()
	if err != nil {
		return protocol.PruneBootstrap{}, err
	}
	if len(encoded) > protocol.MaxPruneBootstrapPlaintextBytes {
		return protocol.PruneBootstrap{}, protocol.ErrTooLarge
	}
	capsule := protocol.PruneBootstrap{
		CapsuleVersion:          plaintext.CapsuleVersion,
		ProtocolVersion:         plaintext.ProtocolVersion,
		CipherSuite:             plaintext.CipherSuite,
		BootstrapPurposeVersion: plaintext.BootstrapPurposeVersion,
		ChannelID:               plaintext.ChannelID,
		RelayGeneration:         plaintext.RelayGeneration,
		PruneID:                 plaintext.PruneID,
		MembershipGeneration:    plaintext.MembershipGeneration,
		BarrierArrivalSequence:  plaintext.BarrierArrivalSequence,
		ClosureReferenceDigest:  plaintext.ClosureReferenceDigest,
		ManifestCount:           plaintext.ManifestCount,
		ManifestDigest:          plaintext.ManifestDigest,
	}
	if random == nil {
		return protocol.PruneBootstrap{}, ErrRandomSource
	}
	if _, err := io.ReadFull(random, capsule.Nonce[:]); err != nil {
		return protocol.PruneBootstrap{}, ErrRandomSource
	}
	return sealPruneBootstrapEncoded(capsule, encoded, base)
}

func sealPruneBootstrapEncoded(
	capsule protocol.PruneBootstrap,
	plaintext []byte,
	base PruneBootstrapKey,
) (protocol.PruneBootstrap, error) {
	if len(plaintext) < 1 {
		return protocol.PruneBootstrap{}, protocol.ErrInvalidPruneBootstrapPlaintext
	}
	if len(plaintext) > protocol.MaxPruneBootstrapPlaintextBytes {
		return protocol.PruneBootstrap{}, protocol.ErrTooLarge
	}
	key, err := derivePruneBootstrapAEADKey(base, capsule)
	if err != nil {
		return protocol.PruneBootstrap{}, err
	}
	aead, err := newPruneBootstrapXChaCha(key)
	if err != nil {
		return protocol.PruneBootstrap{}, ErrInvalidPruneBootstrapKey
	}
	aad, err := protocol.PruneBootstrapAAD(capsule, base.projectID)
	if err != nil {
		return protocol.PruneBootstrap{}, err
	}
	ciphertext := aead.Seal(nil, capsule.Nonce[:], plaintext, aad)
	capsule.Ciphertext = append([]byte(nil), ciphertext...)
	if err := capsule.Validate(); err != nil {
		return protocol.PruneBootstrap{}, err
	}
	return capsule, nil
}

// OpenPruneBootstrap authenticates and strictly decodes one retained capsule.
// Wrong key, project, associated data, ciphertext, or tag are deliberately
// indistinguishable. Only an authenticated outer/inner disagreement returns a
// plaintext-binding error.
func OpenPruneBootstrap(
	capsule protocol.PruneBootstrap,
	base PruneBootstrapKey,
) (protocol.PruneBootstrapPlaintext, error) {
	if err := capsule.Validate(); err != nil {
		return protocol.PruneBootstrapPlaintext{}, err
	}
	if err := validatePruneBootstrapKey(base); err != nil {
		return protocol.PruneBootstrapPlaintext{}, err
	}
	key, err := derivePruneBootstrapAEADKey(base, capsule)
	if err != nil {
		return protocol.PruneBootstrapPlaintext{}, err
	}
	aead, err := newPruneBootstrapXChaCha(key)
	if err != nil {
		return protocol.PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapKey
	}
	aad, err := protocol.PruneBootstrapAAD(capsule, base.projectID)
	if err != nil {
		return protocol.PruneBootstrapPlaintext{}, err
	}
	encoded, err := aead.Open(nil, capsule.Nonce[:], capsule.Ciphertext, aad)
	if err != nil {
		return protocol.PruneBootstrapPlaintext{}, ErrPruneBootstrapAuthenticationFailed
	}
	plaintext, err := protocol.ParsePruneBootstrapPlaintext(encoded)
	if err != nil {
		return protocol.PruneBootstrapPlaintext{}, err
	}
	if !pruneBootstrapBindingsMatch(capsule, plaintext, base) {
		return protocol.PruneBootstrapPlaintext{}, ErrPruneBootstrapPlaintextBinding
	}
	return plaintext, nil
}

func pruneBootstrapBindingsMatch(
	capsule protocol.PruneBootstrap,
	plaintext protocol.PruneBootstrapPlaintext,
	base PruneBootstrapKey,
) bool {
	return plaintext.CapsuleVersion == capsule.CapsuleVersion &&
		plaintext.ProtocolVersion == capsule.ProtocolVersion &&
		plaintext.CipherSuite == capsule.CipherSuite &&
		plaintext.BootstrapPurposeVersion == capsule.BootstrapPurposeVersion &&
		plaintext.ProjectID == base.projectID &&
		plaintext.ChannelID == capsule.ChannelID &&
		plaintext.RelayGeneration == capsule.RelayGeneration &&
		plaintext.PruneID == capsule.PruneID &&
		plaintext.MembershipGeneration == capsule.MembershipGeneration &&
		plaintext.BarrierArrivalSequence == capsule.BarrierArrivalSequence &&
		plaintext.ClosureReferenceDigest == capsule.ClosureReferenceDigest &&
		plaintext.ManifestCount == capsule.ManifestCount &&
		plaintext.ManifestDigest == capsule.ManifestDigest
}
