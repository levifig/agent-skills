package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	// PruneBootstrapCapsuleVersionV1 is the first canonical encrypted deletion
	// anchor capsule format.
	PruneBootstrapCapsuleVersionV1 uint16 = 1
	// PruneBootstrapPurposeVersionV1 identifies the first stable bootstrap-key
	// derivation and transcript purpose. It is independent of content keys.
	PruneBootstrapPurposeVersionV1 uint16 = 1

	// MaxPruneBootstrapPlaintextBytes bounds the canonical metadata plaintext
	// while leaving ample room for its enclosing signed prune certificate.
	MaxPruneBootstrapPlaintextBytes = 256 * 1_024
	// MaxPruneBootstrapCiphertextBytes adds exactly one Poly1305 tag.
	MaxPruneBootstrapCiphertextBytes = MaxPruneBootstrapPlaintextBytes + 16
	// MaxPruneBootstrapBytes bounds the complete capsule wire. The fixed
	// allowance is the exact transcript framing, domain, field prefixes, and
	// clear-field width for the v1 outer format.
	MaxPruneBootstrapBytes = MaxPruneBootstrapCiphertextBytes + pruneBootstrapOuterTranscriptAllowance
	// MaxPruneBootstrapEntryBytes bounds one payload-free deleted-fact anchor.
	MaxPruneBootstrapEntryBytes = 256
	// MaxPrunedArrivalBytes bounds one opaque relay tombstone arrival.
	MaxPrunedArrivalBytes = 1_024
)

var (
	// ErrInvalidPruneBootstrap identifies a malformed opaque bootstrap capsule.
	ErrInvalidPruneBootstrap = errors.New("sync protocol: invalid prune bootstrap")
	// ErrInvalidPruneBootstrapPlaintext identifies malformed decrypted anchor metadata.
	ErrInvalidPruneBootstrapPlaintext = errors.New("sync protocol: invalid prune bootstrap plaintext")
	// ErrInvalidPruneBootstrapEntry identifies one malformed deleted-fact anchor.
	ErrInvalidPruneBootstrapEntry = errors.New("sync protocol: invalid prune bootstrap entry")
	// ErrInvalidPrunedArrival identifies a malformed opaque tombstone arrival.
	ErrInvalidPrunedArrival = errors.New("sync protocol: invalid pruned arrival")
)

const (
	pruneBootstrapDomain            = "loaf.sync.prune-bootstrap.v1"
	pruneBootstrapAADDomain         = "loaf.sync.prune-bootstrap.aad.v1"
	pruneBootstrapPlaintextDomain   = "loaf.sync.prune-bootstrap.plaintext.v1"
	pruneBootstrapEntryDomain       = "loaf.sync.prune-bootstrap.entry.v1"
	prunedArrivalDomain             = "loaf.sync.pruned-arrival.v1"
	pruneBootstrapKeySaltDomain     = "loaf.sync.prune-bootstrap-key.salt.v1"
	pruneBootstrapKeyInfoDomain     = "loaf.sync.prune-bootstrap-key.info.v1"
	pruneBootstrapAEADKeySaltDomain = "loaf.sync.prune-bootstrap-aead-key.salt.v1"
	pruneBootstrapAEADKeyInfoDomain = "loaf.sync.prune-bootstrap-aead-key.info.v1"

	pruneBootstrapClearFieldCount     = 13
	pruneBootstrapFieldCount          = pruneBootstrapClearFieldCount + 1
	pruneBootstrapPlaintextFieldCount = 16
	pruneBootstrapEntryFieldCount     = 4
	prunedArrivalFieldCount           = 4

	pruneBootstrapOuterTranscriptAllowance = 8 + len(pruneBootstrapDomain) +
		pruneBootstrapFieldCount*4 +
		4*2 + // four uint16 selectors
		len(ChannelID{}) + len(RelayGeneration{}) + len(Digest{}) +
		4 + 8 + len(Digest{}) + 4 + len(Digest{}) + len(Nonce{})
)

// PruneBootstrap is the relay-opaque canonical capsule retained with one
// physical prune. Project identity is deliberately absent from cleartext and
// is supplied by the credential when constructing associated data.
type PruneBootstrap struct {
	CapsuleVersion          uint16
	ProtocolVersion         uint16
	CipherSuite             uint16
	BootstrapPurposeVersion uint16
	ChannelID               ChannelID
	RelayGeneration         RelayGeneration
	PruneID                 Digest
	MembershipGeneration    uint32
	BarrierArrivalSequence  int64
	ClosureReferenceDigest  Digest
	ManifestCount           uint32
	ManifestDigest          Digest
	Nonce                   Nonce
	Ciphertext              []byte
}

// Validate rejects unsupported, malformed, or oversized capsule data before
// cryptographic work.
func (capsule PruneBootstrap) Validate() error {
	if err := validatePruneBootstrapClear(capsule); err != nil {
		return err
	}
	if len(capsule.Ciphertext) < 16 {
		return ErrInvalidPruneBootstrap
	}
	if len(capsule.Ciphertext) > MaxPruneBootstrapCiphertextBytes {
		return ErrTooLarge
	}
	return nil
}

func validatePruneBootstrapClear(capsule PruneBootstrap) error {
	if capsule.CapsuleVersion != PruneBootstrapCapsuleVersionV1 ||
		capsule.BootstrapPurposeVersion != PruneBootstrapPurposeVersionV1 {
		return ErrInvalidPruneBootstrap
	}
	if capsule.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if capsule.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if isZero(capsule.ChannelID[:]) || isZero(capsule.RelayGeneration[:]) ||
		isZero(capsule.PruneID[:]) || capsule.MembershipGeneration < 1 ||
		capsule.BarrierArrivalSequence < 1 || isZero(capsule.ClosureReferenceDigest[:]) ||
		capsule.ManifestCount < 1 || capsule.ManifestCount > MaxPruneTargets ||
		isZero(capsule.ManifestDigest[:]) {
		return ErrInvalidPruneBootstrap
	}
	return nil
}

// PruneBootstrapPlaintext is the strictly bounded metadata needed to restore
// authenticated deletion anchors. It intentionally contains no fact payload or
// prose. Every outer selector and prune binding is repeated inside the AEAD.
type PruneBootstrapPlaintext struct {
	CapsuleVersion          uint16
	ProtocolVersion         uint16
	CipherSuite             uint16
	BootstrapPurposeVersion uint16
	ProjectID               continuity.ProjectID
	ChannelID               ChannelID
	RelayGeneration         RelayGeneration
	PruneID                 Digest
	MembershipGeneration    uint32
	BarrierArrivalSequence  int64
	ClosureReferenceDigest  Digest
	ManifestCount           uint32
	ManifestDigest          Digest
	LegacySubject           continuity.SubjectID
	EntryCount              uint32
	Entries                 []PruneBootstrapEntry
}

// Validate rejects unsupported selectors, malformed bindings, count drift,
// duplicate references, or non-prunable entries.
func (plaintext PruneBootstrapPlaintext) Validate() error {
	if plaintext.CapsuleVersion != PruneBootstrapCapsuleVersionV1 ||
		plaintext.BootstrapPurposeVersion != PruneBootstrapPurposeVersionV1 {
		return ErrInvalidPruneBootstrapPlaintext
	}
	if plaintext.ProtocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if plaintext.CipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if plaintext.ProjectID.Validate() != nil || plaintext.LegacySubject.Validate() != nil ||
		isZero(plaintext.ChannelID[:]) || isZero(plaintext.RelayGeneration[:]) ||
		isZero(plaintext.PruneID[:]) || plaintext.MembershipGeneration < 1 ||
		plaintext.BarrierArrivalSequence < 1 || isZero(plaintext.ClosureReferenceDigest[:]) ||
		plaintext.ManifestCount < 1 || plaintext.ManifestCount > MaxPruneTargets ||
		isZero(plaintext.ManifestDigest[:]) || plaintext.EntryCount != plaintext.ManifestCount ||
		uint64(len(plaintext.Entries)) != uint64(plaintext.EntryCount) {
		return ErrInvalidPruneBootstrapPlaintext
	}
	seen := make(map[Digest]struct{}, len(plaintext.Entries))
	for _, entry := range plaintext.Entries {
		if err := entry.Validate(); err != nil {
			return ErrInvalidPruneBootstrapPlaintext
		}
		if _, exists := seen[entry.PruneReferenceDigest]; exists {
			return ErrInvalidPruneBootstrapPlaintext
		}
		seen[entry.PruneReferenceDigest] = struct{}{}
	}
	return nil
}

// PruneBootstrapEntry is one ordered payload-free deletion anchor.
type PruneBootstrapEntry struct {
	PruneReferenceDigest Digest
	FactKind             continuity.FactKind
	HLC                  continuity.HybridTime
}

// Validate permits exactly the four fact kinds physically prunable in v1.
func (entry PruneBootstrapEntry) Validate() error {
	if isZero(entry.PruneReferenceDigest[:]) || entry.HLC.WallMillis < 0 || entry.HLC.Logical < 0 {
		return ErrInvalidPruneBootstrapEntry
	}
	switch entry.FactKind {
	case continuity.FactKind("scratchpad.participant-introduced"),
		continuity.FactKind("scratchpad.message-recorded"),
		continuity.FactKind("scratchpad.claim-recorded"),
		continuity.FactKind("scratchpad.claim-released"):
		return nil
	default:
		return ErrInvalidPruneBootstrapEntry
	}
}

// PrunedArrival is the bounded canonical relay representation of one arrival
// whose ciphertext was removed under an authenticated prune.
type PrunedArrival struct {
	ChannelID       ChannelID
	RelayGeneration RelayGeneration
	PruneID         Digest
	Reference       PruneReference
}

// Validate rejects malformed opaque tombstone identity.
func (arrival PrunedArrival) Validate() error {
	if isZero(arrival.ChannelID[:]) || isZero(arrival.RelayGeneration[:]) ||
		isZero(arrival.PruneID[:]) || arrival.Reference.Validate() != nil {
		return ErrInvalidPrunedArrival
	}
	return nil
}

// MarshalBinary encodes a complete prune-bootstrap capsule canonically.
func (capsule PruneBootstrap) MarshalBinary() ([]byte, error) {
	if err := capsule.Validate(); err != nil {
		return nil, err
	}
	fields := append(pruneBootstrapClearFields(capsule), capsule.Ciphertext)
	encoded, err := encodeTranscript(pruneBootstrapDomain, fields...)
	if err != nil {
		return nil, pruneBootstrapEncodingError(err, ErrInvalidPruneBootstrap)
	}
	if len(encoded) > MaxPruneBootstrapBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneBootstrap strictly decodes one complete canonical outer capsule.
func ParsePruneBootstrap(encoded []byte) (PruneBootstrap, error) {
	if len(encoded) > MaxPruneBootstrapBytes {
		return PruneBootstrap{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneBootstrapDomain, pruneBootstrapFieldCount)
	if err != nil {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	capsule, err := parsePruneBootstrapClearFields(fields[:pruneBootstrapClearFieldCount])
	if err != nil {
		return PruneBootstrap{}, err
	}
	capsule.Ciphertext = append([]byte(nil), fields[pruneBootstrapClearFieldCount]...)
	if err := capsule.Validate(); err != nil {
		return PruneBootstrap{}, err
	}
	if !canonicalPruneBootstrapEncoding(encoded, capsule.MarshalBinary) {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	return capsule, nil
}

// PruneBootstrapDigest returns the SHA-256 identity of the complete canonical
// outer capsule. Invalid in-memory values return the zero digest.
func PruneBootstrapDigest(capsule PruneBootstrap) Digest {
	encoded, err := capsule.MarshalBinary()
	if err != nil {
		return Digest{}
	}
	return sha256.Sum256(encoded)
}

// PruneBootstrapAAD returns the exact associated data for capsule encryption.
// The credential project is authenticated but deliberately absent from relay
// cleartext. Ciphertext is the only outer field excluded.
func PruneBootstrapAAD(capsule PruneBootstrap, credentialProjectID continuity.ProjectID) ([]byte, error) {
	if err := validatePruneBootstrapClear(capsule); err != nil {
		return nil, err
	}
	if credentialProjectID.Validate() != nil {
		return nil, ErrInvalidPruneBootstrap
	}
	return pruneBootstrapAADUnchecked(capsule, credentialProjectID)
}

func pruneBootstrapAADUnchecked(capsule PruneBootstrap, credentialProjectID continuity.ProjectID) ([]byte, error) {
	fields := append(pruneBootstrapClearFields(capsule), []byte(credentialProjectID))
	encoded, err := encodeTranscript(pruneBootstrapAADDomain, fields...)
	if err != nil {
		return nil, pruneBootstrapEncodingError(err, ErrInvalidPruneBootstrap)
	}
	return encoded, nil
}

// MarshalBinary encodes decrypted bootstrap metadata canonically.
func (plaintext PruneBootstrapPlaintext) MarshalBinary() ([]byte, error) {
	if err := plaintext.Validate(); err != nil {
		return nil, err
	}
	encodedEntries := make([][]byte, 0, len(plaintext.Entries))
	for _, entry := range plaintext.Entries {
		encoded, err := entry.MarshalBinary()
		if err != nil {
			return nil, pruneBootstrapEncodingError(err, ErrInvalidPruneBootstrapPlaintext)
		}
		encodedEntries = append(encodedEntries, encoded)
	}
	entryList, err := encodePruneBootstrapEntryList(encodedEntries)
	if err != nil {
		return nil, err
	}
	fields := [][]byte{
		uint16Bytes(plaintext.CapsuleVersion),
		uint16Bytes(plaintext.ProtocolVersion),
		uint16Bytes(plaintext.CipherSuite),
		uint16Bytes(plaintext.BootstrapPurposeVersion),
		[]byte(plaintext.ProjectID),
		plaintext.ChannelID[:],
		plaintext.RelayGeneration[:],
		plaintext.PruneID[:],
		uint32Bytes(plaintext.MembershipGeneration),
		int64Bytes(plaintext.BarrierArrivalSequence),
		plaintext.ClosureReferenceDigest[:],
		uint32Bytes(plaintext.ManifestCount),
		plaintext.ManifestDigest[:],
		[]byte(plaintext.LegacySubject),
		uint32Bytes(plaintext.EntryCount),
		entryList,
	}
	encoded, err := encodeTranscript(pruneBootstrapPlaintextDomain, fields...)
	if err != nil {
		return nil, pruneBootstrapEncodingError(err, ErrInvalidPruneBootstrapPlaintext)
	}
	if len(encoded) > MaxPruneBootstrapPlaintextBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneBootstrapPlaintext strictly decodes canonical decrypted metadata.
func ParsePruneBootstrapPlaintext(encoded []byte) (PruneBootstrapPlaintext, error) {
	if len(encoded) > MaxPruneBootstrapPlaintextBytes {
		return PruneBootstrapPlaintext{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneBootstrapPlaintextDomain, pruneBootstrapPlaintextFieldCount)
	if err != nil {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	plaintext, err := parsePruneBootstrapPlaintextFields(fields)
	if err != nil {
		return PruneBootstrapPlaintext{}, err
	}
	if err := plaintext.Validate(); err != nil {
		return PruneBootstrapPlaintext{}, err
	}
	if !canonicalPruneBootstrapEncoding(encoded, plaintext.MarshalBinary) {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	return plaintext, nil
}

// MarshalBinary encodes one payload-free prune-bootstrap entry canonically.
func (entry PruneBootstrapEntry) MarshalBinary() ([]byte, error) {
	if err := entry.Validate(); err != nil {
		return nil, err
	}
	encoded, err := encodeTranscript(
		pruneBootstrapEntryDomain,
		entry.PruneReferenceDigest[:],
		[]byte(entry.FactKind),
		int64Bytes(entry.HLC.WallMillis),
		int32Bytes(entry.HLC.Logical),
	)
	if err != nil {
		return nil, pruneBootstrapEncodingError(err, ErrInvalidPruneBootstrapEntry)
	}
	if len(encoded) > MaxPruneBootstrapEntryBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneBootstrapEntry strictly decodes one canonical deleted-fact anchor.
func ParsePruneBootstrapEntry(encoded []byte) (PruneBootstrapEntry, error) {
	if len(encoded) > MaxPruneBootstrapEntryBytes {
		return PruneBootstrapEntry{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneBootstrapEntryDomain, pruneBootstrapEntryFieldCount)
	if err != nil || len(fields[0]) != len(Digest{}) {
		return PruneBootstrapEntry{}, ErrInvalidPruneBootstrapEntry
	}
	wallMillis, ok := parseInt64(fields[2])
	if !ok {
		return PruneBootstrapEntry{}, ErrInvalidPruneBootstrapEntry
	}
	logical, ok := parseInt32(fields[3])
	if !ok {
		return PruneBootstrapEntry{}, ErrInvalidPruneBootstrapEntry
	}
	entry := PruneBootstrapEntry{
		FactKind: continuity.FactKind(string(fields[1])),
		HLC:      continuity.HybridTime{WallMillis: wallMillis, Logical: logical},
	}
	copy(entry.PruneReferenceDigest[:], fields[0])
	if err := entry.Validate(); err != nil {
		return PruneBootstrapEntry{}, err
	}
	if !canonicalPruneBootstrapEncoding(encoded, entry.MarshalBinary) {
		return PruneBootstrapEntry{}, ErrInvalidPruneBootstrapEntry
	}
	return entry, nil
}

// MarshalBinary encodes one exact opaque pruned arrival canonically.
func (arrival PrunedArrival) MarshalBinary() ([]byte, error) {
	if err := arrival.Validate(); err != nil {
		return nil, err
	}
	reference, err := arrival.Reference.MarshalBinary()
	if err != nil {
		return nil, pruneBootstrapEncodingError(err, ErrInvalidPrunedArrival)
	}
	encoded, err := encodeTranscript(
		prunedArrivalDomain,
		arrival.ChannelID[:],
		arrival.RelayGeneration[:],
		arrival.PruneID[:],
		reference,
	)
	if err != nil {
		return nil, pruneBootstrapEncodingError(err, ErrInvalidPrunedArrival)
	}
	if len(encoded) > MaxPrunedArrivalBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePrunedArrival strictly decodes one canonical opaque tombstone arrival.
func ParsePrunedArrival(encoded []byte) (PrunedArrival, error) {
	if len(encoded) > MaxPrunedArrivalBytes {
		return PrunedArrival{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, prunedArrivalDomain, prunedArrivalFieldCount)
	if err != nil || len(fields[0]) != len(ChannelID{}) || len(fields[1]) != len(RelayGeneration{}) || len(fields[2]) != len(Digest{}) {
		return PrunedArrival{}, ErrInvalidPrunedArrival
	}
	reference, err := ParsePruneReference(fields[3])
	if err != nil {
		return PrunedArrival{}, pruneBootstrapEncodingError(err, ErrInvalidPrunedArrival)
	}
	arrival := PrunedArrival{Reference: reference}
	copy(arrival.ChannelID[:], fields[0])
	copy(arrival.RelayGeneration[:], fields[1])
	copy(arrival.PruneID[:], fields[2])
	if err := arrival.Validate(); err != nil {
		return PrunedArrival{}, err
	}
	if !canonicalPruneBootstrapEncoding(encoded, arrival.MarshalBinary) {
		return PrunedArrival{}, ErrInvalidPrunedArrival
	}
	return arrival, nil
}

// PruneBootstrapKeySalt returns the project-scoped salt for the stable typed
// bootstrap key derived directly from ProjectRoot.
func PruneBootstrapKeySalt(projectID continuity.ProjectID) ([]byte, error) {
	if projectID.Validate() != nil {
		return nil, ErrInvalidPruneBootstrap
	}
	transcript, err := encodeTranscript(pruneBootstrapKeySaltDomain, []byte(projectID))
	if err != nil {
		return nil, ErrInvalidPruneBootstrap
	}
	digest := sha256.Sum256(transcript)
	return digest[:], nil
}

// PruneBootstrapKeyInfo returns the selector-bound context for the stable
// bootstrap-key derivation.
func PruneBootstrapKeyInfo(protocolVersion, cipherSuite, purposeVersion uint16) ([]byte, error) {
	if err := validatePruneBootstrapSelectors(protocolVersion, cipherSuite, purposeVersion); err != nil {
		return nil, err
	}
	return encodeTranscript(
		pruneBootstrapKeyInfoDomain,
		uint16Bytes(protocolVersion),
		uint16Bytes(cipherSuite),
		uint16Bytes(purposeVersion),
	)
}

// PruneBootstrapAEADKeySalt returns the project, relay incarnation, and exact
// prune-bound salt for the second typed key derivation.
func PruneBootstrapAEADKeySalt(
	projectID continuity.ProjectID,
	channelID ChannelID,
	relayGeneration RelayGeneration,
	pruneID Digest,
) ([]byte, error) {
	if projectID.Validate() != nil || isZero(channelID[:]) || isZero(relayGeneration[:]) || isZero(pruneID[:]) {
		return nil, ErrInvalidPruneBootstrap
	}
	transcript, err := encodeTranscript(
		pruneBootstrapAEADKeySaltDomain,
		[]byte(projectID),
		channelID[:],
		relayGeneration[:],
		pruneID[:],
	)
	if err != nil {
		return nil, ErrInvalidPruneBootstrap
	}
	digest := sha256.Sum256(transcript)
	return digest[:], nil
}

// PruneBootstrapAEADKeyInfo returns every remaining stable cleartext binding,
// excluding only the AEAD nonce and ciphertext, for one per-prune key.
func PruneBootstrapAEADKeyInfo(
	capsuleVersion,
	protocolVersion,
	cipherSuite,
	purposeVersion uint16,
	membershipGeneration uint32,
	barrierArrivalSequence int64,
	closureReferenceDigest Digest,
	manifestCount uint32,
	manifestDigest Digest,
) ([]byte, error) {
	if capsuleVersion != PruneBootstrapCapsuleVersionV1 {
		return nil, ErrInvalidPruneBootstrap
	}
	if err := validatePruneBootstrapSelectors(protocolVersion, cipherSuite, purposeVersion); err != nil {
		return nil, err
	}
	if membershipGeneration < 1 || barrierArrivalSequence < 1 || isZero(closureReferenceDigest[:]) ||
		manifestCount < 1 || manifestCount > MaxPruneTargets || isZero(manifestDigest[:]) {
		return nil, ErrInvalidPruneBootstrap
	}
	return encodeTranscript(
		pruneBootstrapAEADKeyInfoDomain,
		uint16Bytes(capsuleVersion),
		uint16Bytes(protocolVersion),
		uint16Bytes(cipherSuite),
		uint16Bytes(purposeVersion),
		uint32Bytes(membershipGeneration),
		int64Bytes(barrierArrivalSequence),
		closureReferenceDigest[:],
		uint32Bytes(manifestCount),
		manifestDigest[:],
	)
}

func validatePruneBootstrapSelectors(protocolVersion, cipherSuite, purposeVersion uint16) error {
	if protocolVersion != ProtocolVersionV1 {
		return ErrUnsupportedProtocolVersion
	}
	if cipherSuite != CipherSuiteXChaCha20Poly1305 {
		return ErrUnsupportedCipherSuite
	}
	if purposeVersion != PruneBootstrapPurposeVersionV1 {
		return ErrInvalidPruneBootstrap
	}
	return nil
}

func pruneBootstrapClearFields(capsule PruneBootstrap) [][]byte {
	return [][]byte{
		uint16Bytes(capsule.CapsuleVersion),
		uint16Bytes(capsule.ProtocolVersion),
		uint16Bytes(capsule.CipherSuite),
		uint16Bytes(capsule.BootstrapPurposeVersion),
		capsule.ChannelID[:],
		capsule.RelayGeneration[:],
		capsule.PruneID[:],
		uint32Bytes(capsule.MembershipGeneration),
		int64Bytes(capsule.BarrierArrivalSequence),
		capsule.ClosureReferenceDigest[:],
		uint32Bytes(capsule.ManifestCount),
		capsule.ManifestDigest[:],
		capsule.Nonce[:],
	}
}

func parsePruneBootstrapClearFields(fields [][]byte) (PruneBootstrap, error) {
	if len(fields) != pruneBootstrapClearFieldCount || len(fields[4]) != len(ChannelID{}) ||
		len(fields[5]) != len(RelayGeneration{}) || len(fields[6]) != len(Digest{}) ||
		len(fields[9]) != len(Digest{}) || len(fields[11]) != len(Digest{}) || len(fields[12]) != len(Nonce{}) {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	capsuleVersion, ok := parseUint16(fields[0])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	purposeVersion, ok := parseUint16(fields[3])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	membershipGeneration, ok := parseUint32(fields[7])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	barrier, ok := parseInt64(fields[8])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	manifestCount, ok := parseUint32(fields[10])
	if !ok {
		return PruneBootstrap{}, ErrInvalidPruneBootstrap
	}
	capsule := PruneBootstrap{
		CapsuleVersion:          capsuleVersion,
		ProtocolVersion:         protocolVersion,
		CipherSuite:             cipherSuite,
		BootstrapPurposeVersion: purposeVersion,
		MembershipGeneration:    membershipGeneration,
		BarrierArrivalSequence:  barrier,
		ManifestCount:           manifestCount,
	}
	copy(capsule.ChannelID[:], fields[4])
	copy(capsule.RelayGeneration[:], fields[5])
	copy(capsule.PruneID[:], fields[6])
	copy(capsule.ClosureReferenceDigest[:], fields[9])
	copy(capsule.ManifestDigest[:], fields[11])
	copy(capsule.Nonce[:], fields[12])
	return capsule, nil
}

func parsePruneBootstrapPlaintextFields(fields [][]byte) (PruneBootstrapPlaintext, error) {
	if len(fields) != pruneBootstrapPlaintextFieldCount || len(fields[5]) != len(ChannelID{}) ||
		len(fields[6]) != len(RelayGeneration{}) || len(fields[7]) != len(Digest{}) ||
		len(fields[10]) != len(Digest{}) || len(fields[12]) != len(Digest{}) {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	capsuleVersion, ok := parseUint16(fields[0])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	purposeVersion, ok := parseUint16(fields[3])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	membershipGeneration, ok := parseUint32(fields[8])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	barrier, ok := parseInt64(fields[9])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	manifestCount, ok := parseUint32(fields[11])
	if !ok {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	entryCount, ok := parseUint32(fields[14])
	if !ok || entryCount < 1 || entryCount > MaxPruneTargets {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	entryBytes, err := parsePruneBootstrapEntryList(fields[15])
	if err != nil || uint32(len(entryBytes)) != entryCount {
		return PruneBootstrapPlaintext{}, ErrInvalidPruneBootstrapPlaintext
	}
	entries := make([]PruneBootstrapEntry, 0, len(entryBytes))
	for _, encoded := range entryBytes {
		entry, err := ParsePruneBootstrapEntry(encoded)
		if err != nil {
			return PruneBootstrapPlaintext{}, pruneBootstrapEncodingError(err, ErrInvalidPruneBootstrapPlaintext)
		}
		entries = append(entries, entry)
	}
	plaintext := PruneBootstrapPlaintext{
		CapsuleVersion:          capsuleVersion,
		ProtocolVersion:         protocolVersion,
		CipherSuite:             cipherSuite,
		BootstrapPurposeVersion: purposeVersion,
		ProjectID:               continuity.ProjectID(string(fields[4])),
		MembershipGeneration:    membershipGeneration,
		BarrierArrivalSequence:  barrier,
		ManifestCount:           manifestCount,
		LegacySubject:           continuity.SubjectID(string(fields[13])),
		EntryCount:              entryCount,
		Entries:                 entries,
	}
	copy(plaintext.ChannelID[:], fields[5])
	copy(plaintext.RelayGeneration[:], fields[6])
	copy(plaintext.PruneID[:], fields[7])
	copy(plaintext.ClosureReferenceDigest[:], fields[10])
	copy(plaintext.ManifestDigest[:], fields[12])
	return plaintext, nil
}

func encodePruneBootstrapEntryList(entries [][]byte) ([]byte, error) {
	if len(entries) < 1 || len(entries) > MaxPruneTargets {
		return nil, ErrInvalidPruneBootstrapPlaintext
	}
	total := 4
	for _, entry := range entries {
		if len(entry) < 1 || len(entry) > MaxPruneBootstrapEntryBytes {
			return nil, ErrInvalidPruneBootstrapPlaintext
		}
		total += 4 + len(entry)
		if total > MaxPruneBootstrapPlaintextBytes {
			return nil, ErrTooLarge
		}
	}
	encoded := make([]byte, 0, total)
	encoded = appendUint32(encoded, uint32(len(entries)))
	for _, entry := range entries {
		encoded = appendUint32(encoded, uint32(len(entry)))
		encoded = append(encoded, entry...)
	}
	return encoded, nil
}

func parsePruneBootstrapEntryList(encoded []byte) ([][]byte, error) {
	if len(encoded) < 4 || len(encoded) > MaxPruneBootstrapPlaintextBytes {
		return nil, ErrInvalidPruneBootstrapPlaintext
	}
	count := binary.BigEndian.Uint32(encoded[:4])
	if count < 1 || count > MaxPruneTargets {
		return nil, ErrInvalidPruneBootstrapPlaintext
	}
	entries := make([][]byte, 0, int(count))
	offset := 4
	for range count {
		if offset+4 > len(encoded) {
			return nil, ErrInvalidPruneBootstrapPlaintext
		}
		length := binary.BigEndian.Uint32(encoded[offset : offset+4])
		offset += 4
		if length < 1 || length > MaxPruneBootstrapEntryBytes || uint64(length) > uint64(len(encoded)-offset) {
			return nil, ErrInvalidPruneBootstrapPlaintext
		}
		entries = append(entries, append([]byte(nil), encoded[offset:offset+int(length)]...))
		offset += int(length)
	}
	if offset != len(encoded) {
		return nil, ErrInvalidPruneBootstrapPlaintext
	}
	return entries, nil
}

func int32Bytes(value int32) []byte {
	return binary.BigEndian.AppendUint32(nil, uint32(value))
}

func parseInt32(value []byte) (int32, bool) {
	if len(value) != 4 {
		return 0, false
	}
	return int32(binary.BigEndian.Uint32(value)), true
}

func canonicalPruneBootstrapEncoding(encoded []byte, marshal func() ([]byte, error)) bool {
	canonical, err := marshal()
	return err == nil && bytes.Equal(encoded, canonical)
}

func pruneBootstrapEncodingError(err, invalid error) error {
	if err == ErrTooLarge {
		return ErrTooLarge
	}
	return invalid
}
