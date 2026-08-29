package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	progressAcknowledgementBodyDomain = "loaf.sync.acknowledgement.body.v1"
	progressAcknowledgementDomain     = "loaf.sync.acknowledgement.v1"
	pruneAcknowledgementBodyDomain    = "loaf.sync.prune-acknowledgement.body.v1"
	pruneAcknowledgementDomain        = "loaf.sync.prune-acknowledgement.v1"
	terminalRetirementBodyDomain      = "loaf.sync.terminal-retirement.body.v1"
	terminalRetirementDomain          = "loaf.sync.terminal-retirement.v1"
	pruneReferenceDomain              = "loaf.sync.prune-reference.v1"
	pruneManifestDomain               = "loaf.sync.prune-manifest.v1"
	pruneCertificateBodyDomain        = "loaf.sync.prune-certificate.body.v1"
	pruneCertificateDomain            = "loaf.sync.prune-certificate.v1"

	progressAcknowledgementBodyFieldCount = 11
	progressAcknowledgementFieldCount     = progressAcknowledgementBodyFieldCount + 1
	pruneAcknowledgementBodyFieldCount    = 17
	pruneAcknowledgementFieldCount        = pruneAcknowledgementBodyFieldCount + 1
	terminalRetirementBodyFieldCount      = 10
	terminalRetirementFieldCount          = terminalRetirementBodyFieldCount + 1
	pruneReferenceFieldCount              = 9
	pruneManifestFieldCount               = 2
	pruneCertificateBodyFieldCount        = 15
	pruneCertificateFieldCount            = pruneCertificateBodyFieldCount + 1
)

// ProgressAcknowledgementBodyTranscript returns the exact bytes signed by an
// environment when it asserts applied and producer progress.
func ProgressAcknowledgementBodyTranscript(acknowledgement ProgressAcknowledgement) ([]byte, error) {
	if err := acknowledgement.Validate(); err != nil {
		return nil, err
	}
	return encodeTranscript(progressAcknowledgementBodyDomain, progressAcknowledgementBodyFields(acknowledgement)...)
}

// MarshalBinary encodes a progress acknowledgement as canonical signed bytes.
func (acknowledgement ProgressAcknowledgement) MarshalBinary() ([]byte, error) {
	if err := acknowledgement.Validate(); err != nil {
		return nil, err
	}
	fields := append(progressAcknowledgementBodyFields(acknowledgement), acknowledgement.EnvironmentSignature[:])
	encoded, err := encodeTranscript(progressAcknowledgementDomain, fields...)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidAcknowledgement)
	}
	if len(encoded) > MaxProgressAcknowledgementBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParseProgressAcknowledgement strictly decodes one canonical signed progress
// assertion.
func ParseProgressAcknowledgement(encoded []byte) (ProgressAcknowledgement, error) {
	if len(encoded) > MaxProgressAcknowledgementBytes {
		return ProgressAcknowledgement{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, progressAcknowledgementDomain, progressAcknowledgementFieldCount)
	if err != nil {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	acknowledgement, err := parseProgressAcknowledgementBody(fields[:progressAcknowledgementBodyFieldCount])
	if err != nil {
		return ProgressAcknowledgement{}, err
	}
	if len(fields[progressAcknowledgementBodyFieldCount]) != len(Signature{}) {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	copy(acknowledgement.EnvironmentSignature[:], fields[progressAcknowledgementBodyFieldCount])
	if err := acknowledgement.Validate(); err != nil {
		return ProgressAcknowledgement{}, err
	}
	if !canonicalControlEncoding(encoded, acknowledgement.MarshalBinary) {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	return acknowledgement, nil
}

// ProgressAcknowledgementDigest returns the immutable SHA-256 identity of the
// complete signed acknowledgement. Invalid values return the zero digest.
func ProgressAcknowledgementDigest(acknowledgement ProgressAcknowledgement) Digest {
	return digestControlObject(acknowledgement.MarshalBinary)
}

// PruneAcknowledgementBodyTranscript returns the exact bytes signed by an
// environment when it votes for one immutable prune proposal.
func PruneAcknowledgementBodyTranscript(acknowledgement PruneAcknowledgement) ([]byte, error) {
	if err := acknowledgement.Validate(); err != nil {
		return nil, err
	}
	return encodeTranscript(pruneAcknowledgementBodyDomain, pruneAcknowledgementBodyFields(acknowledgement)...)
}

// MarshalBinary encodes a prune acknowledgement as canonical signed bytes.
func (acknowledgement PruneAcknowledgement) MarshalBinary() ([]byte, error) {
	if err := acknowledgement.Validate(); err != nil {
		return nil, err
	}
	fields := append(pruneAcknowledgementBodyFields(acknowledgement), acknowledgement.EnvironmentSignature[:])
	encoded, err := encodeTranscript(pruneAcknowledgementDomain, fields...)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneAcknowledgement)
	}
	if len(encoded) > MaxPruneAcknowledgementBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneAcknowledgement strictly decodes one canonical environment vote.
func ParsePruneAcknowledgement(encoded []byte) (PruneAcknowledgement, error) {
	if len(encoded) > MaxPruneAcknowledgementBytes {
		return PruneAcknowledgement{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneAcknowledgementDomain, pruneAcknowledgementFieldCount)
	if err != nil {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	acknowledgement, err := parsePruneAcknowledgementBody(fields[:pruneAcknowledgementBodyFieldCount])
	if err != nil {
		return PruneAcknowledgement{}, err
	}
	if len(fields[pruneAcknowledgementBodyFieldCount]) != len(Signature{}) {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	copy(acknowledgement.EnvironmentSignature[:], fields[pruneAcknowledgementBodyFieldCount])
	if err := acknowledgement.Validate(); err != nil {
		return PruneAcknowledgement{}, err
	}
	if !canonicalControlEncoding(encoded, acknowledgement.MarshalBinary) {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	return acknowledgement, nil
}

// PruneAcknowledgementDigest returns the immutable SHA-256 identity of the
// complete signed vote. Invalid values return the zero digest.
func PruneAcknowledgementDigest(acknowledgement PruneAcknowledgement) Digest {
	return digestControlObject(acknowledgement.MarshalBinary)
}

// TerminalRetirementBodyTranscript returns the exact terminal source-chain
// fence signed by the project administrator.
func TerminalRetirementBodyTranscript(retirement TerminalRetirement) ([]byte, error) {
	if err := retirement.Validate(); err != nil {
		return nil, err
	}
	return encodeTranscript(terminalRetirementBodyDomain, terminalRetirementBodyFields(retirement)...)
}

// MarshalBinary encodes a terminal retirement as canonical signed bytes.
func (retirement TerminalRetirement) MarshalBinary() ([]byte, error) {
	if err := retirement.Validate(); err != nil {
		return nil, err
	}
	fields := append(terminalRetirementBodyFields(retirement), retirement.AdminSignature[:])
	encoded, err := encodeTranscript(terminalRetirementDomain, fields...)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidRetirement)
	}
	if len(encoded) > MaxTerminalRetirementBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParseTerminalRetirement strictly decodes one canonical terminal fence.
func ParseTerminalRetirement(encoded []byte) (TerminalRetirement, error) {
	if len(encoded) > MaxTerminalRetirementBytes {
		return TerminalRetirement{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, terminalRetirementDomain, terminalRetirementFieldCount)
	if err != nil {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	retirement, err := parseTerminalRetirementBody(fields[:terminalRetirementBodyFieldCount])
	if err != nil {
		return TerminalRetirement{}, err
	}
	if len(fields[terminalRetirementBodyFieldCount]) != len(Signature{}) {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	copy(retirement.AdminSignature[:], fields[terminalRetirementBodyFieldCount])
	if err := retirement.Validate(); err != nil {
		return TerminalRetirement{}, err
	}
	if !canonicalControlEncoding(encoded, retirement.MarshalBinary) {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	return retirement, nil
}

// TerminalRetirementID returns the immutable SHA-256 identity of the complete
// signed terminal fence. Invalid values return the zero digest.
func TerminalRetirementID(retirement TerminalRetirement) Digest {
	return digestControlObject(retirement.MarshalBinary)
}

// MarshalBinary encodes an exact opaque arrival reference canonically.
func (reference PruneReference) MarshalBinary() ([]byte, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	encoded, err := encodeTranscript(pruneReferenceDomain, pruneReferenceFields(reference)...)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneReference)
	}
	if len(encoded) > MaxPruneReferenceBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneReference strictly decodes one canonical opaque arrival identity.
func ParsePruneReference(encoded []byte) (PruneReference, error) {
	if len(encoded) > MaxPruneReferenceBytes {
		return PruneReference{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneReferenceDomain, pruneReferenceFieldCount)
	if err != nil {
		return PruneReference{}, ErrInvalidPruneReference
	}
	reference, err := parsePruneReferenceFields(fields)
	if err != nil {
		return PruneReference{}, err
	}
	if err := reference.Validate(); err != nil {
		return PruneReference{}, err
	}
	if !canonicalControlEncoding(encoded, reference.MarshalBinary) {
		return PruneReference{}, ErrInvalidPruneReference
	}
	return reference, nil
}

// PruneReferenceDigest identifies one exact opaque arrival reference.
func PruneReferenceDigest(reference PruneReference) Digest {
	return digestControlObject(reference.MarshalBinary)
}

// MarshalBinary encodes an exact arrival-ordered prune manifest canonically.
func (manifest PruneManifest) MarshalBinary() ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	encodedTargets := make([][]byte, 0, len(manifest.Targets))
	for _, target := range manifest.Targets {
		encoded, err := target.MarshalBinary()
		if err != nil {
			return nil, controlEncodingError(err, ErrInvalidPruneManifest)
		}
		encodedTargets = append(encodedTargets, encoded)
	}
	targetList, err := encodeControlObjectList(encodedTargets, MaxPruneTargets)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneManifest)
	}
	encoded, err := encodeTranscript(pruneManifestDomain, uint32Bytes(uint32(len(manifest.Targets))), targetList)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneManifest)
	}
	if len(encoded) > MaxControlObjectBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneManifest strictly decodes one canonical ordered target set.
func ParsePruneManifest(encoded []byte) (PruneManifest, error) {
	if len(encoded) > MaxControlObjectBytes {
		return PruneManifest{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneManifestDomain, pruneManifestFieldCount)
	if err != nil {
		return PruneManifest{}, ErrInvalidPruneManifest
	}
	count, ok := parseUint32(fields[0])
	if !ok || count < 1 || count > MaxPruneTargets {
		return PruneManifest{}, ErrInvalidPruneManifest
	}
	targetBytes, err := parseControlObjectList(fields[1], MaxPruneTargets)
	if err != nil || uint32(len(targetBytes)) != count {
		return PruneManifest{}, ErrInvalidPruneManifest
	}
	manifest := PruneManifest{Targets: make([]PruneReference, 0, len(targetBytes))}
	for _, item := range targetBytes {
		target, err := ParsePruneReference(item)
		if err != nil {
			return PruneManifest{}, controlEncodingError(err, ErrInvalidPruneManifest)
		}
		manifest.Targets = append(manifest.Targets, target)
	}
	if err := manifest.Validate(); err != nil {
		return PruneManifest{}, err
	}
	if !canonicalControlEncoding(encoded, manifest.MarshalBinary) {
		return PruneManifest{}, ErrInvalidPruneManifest
	}
	return manifest, nil
}

// PruneManifestDigest identifies one exact sorted physical-prune target set.
func PruneManifestDigest(manifest PruneManifest) Digest {
	return digestControlObject(manifest.MarshalBinary)
}

// PruneCertificateBodyTranscript returns the exact self-contained physical-
// prune authority signed by the project administrator.
func PruneCertificateBodyTranscript(certificate PruneCertificate) ([]byte, error) {
	if err := certificate.Validate(); err != nil {
		return nil, err
	}
	return pruneCertificateBodyTranscriptUnchecked(certificate)
}

// MarshalBinary encodes a prune certificate as canonical signed bytes.
func (certificate PruneCertificate) MarshalBinary() ([]byte, error) {
	if err := certificate.Validate(); err != nil {
		return nil, err
	}
	fields, err := pruneCertificateBodyFields(certificate)
	if err != nil {
		return nil, err
	}
	fields = append(fields, certificate.AdminSignature[:])
	encoded, err := encodeTranscript(pruneCertificateDomain, fields...)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	if len(encoded) > MaxControlObjectBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParsePruneCertificate strictly decodes one canonical signed prune authority.
func ParsePruneCertificate(encoded []byte) (PruneCertificate, error) {
	if len(encoded) > MaxControlObjectBytes {
		return PruneCertificate{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, pruneCertificateDomain, pruneCertificateFieldCount)
	if err != nil {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	certificate, err := parsePruneCertificateBody(fields[:pruneCertificateBodyFieldCount])
	if err != nil {
		return PruneCertificate{}, err
	}
	if len(fields[pruneCertificateBodyFieldCount]) != len(Signature{}) {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	copy(certificate.AdminSignature[:], fields[pruneCertificateBodyFieldCount])
	if err := certificate.Validate(); err != nil {
		return PruneCertificate{}, err
	}
	if !canonicalControlEncoding(encoded, certificate.MarshalBinary) {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	return certificate, nil
}

// PruneCertificateID returns the immutable SHA-256 identity of the complete
// signed prune authority. Invalid values return the zero digest.
func PruneCertificateID(certificate PruneCertificate) Digest {
	return digestControlObject(certificate.MarshalBinary)
}

func progressAcknowledgementBodyFields(acknowledgement ProgressAcknowledgement) [][]byte {
	return [][]byte{
		uint16Bytes(acknowledgement.Version),
		uint16Bytes(acknowledgement.ProtocolVersion),
		uint16Bytes(acknowledgement.CipherSuite),
		acknowledgement.ChannelID[:],
		acknowledgement.RelayGeneration[:],
		[]byte(acknowledgement.EnvironmentID),
		acknowledgement.CertificateID[:],
		uint32Bytes(acknowledgement.MembershipGeneration),
		int64Bytes(acknowledgement.AppliedArrivalSequence),
		int64Bytes(acknowledgement.ProducerSequence),
		acknowledgement.ProducerEnvelopeDigest[:],
	}
}

func parseProgressAcknowledgementBody(fields [][]byte) (ProgressAcknowledgement, error) {
	if len(fields) != progressAcknowledgementBodyFieldCount || len(fields[3]) != len(ChannelID{}) ||
		len(fields[4]) != len(RelayGeneration{}) || len(fields[6]) != len(Digest{}) || len(fields[10]) != len(Digest{}) {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	version, ok := parseUint16(fields[0])
	if !ok {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	membershipGeneration, ok := parseUint32(fields[7])
	if !ok {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	appliedArrivalSequence, ok := parseInt64(fields[8])
	if !ok {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	producerSequence, ok := parseInt64(fields[9])
	if !ok {
		return ProgressAcknowledgement{}, ErrInvalidAcknowledgement
	}
	acknowledgement := ProgressAcknowledgement{
		Version:                version,
		ProtocolVersion:        protocolVersion,
		CipherSuite:            cipherSuite,
		EnvironmentID:          continuity.EnvironmentID(string(fields[5])),
		MembershipGeneration:   membershipGeneration,
		AppliedArrivalSequence: appliedArrivalSequence,
		ProducerSequence:       producerSequence,
	}
	copy(acknowledgement.ChannelID[:], fields[3])
	copy(acknowledgement.RelayGeneration[:], fields[4])
	copy(acknowledgement.CertificateID[:], fields[6])
	copy(acknowledgement.ProducerEnvelopeDigest[:], fields[10])
	return acknowledgement, nil
}

func pruneAcknowledgementBodyFields(acknowledgement PruneAcknowledgement) [][]byte {
	return [][]byte{
		uint16Bytes(acknowledgement.Version),
		uint16Bytes(acknowledgement.ProtocolVersion),
		uint16Bytes(acknowledgement.CipherSuite),
		acknowledgement.ChannelID[:],
		acknowledgement.RelayGeneration[:],
		[]byte(acknowledgement.EnvironmentID),
		acknowledgement.CertificateID[:],
		uint32Bytes(acknowledgement.MembershipGeneration),
		acknowledgement.ProgressAcknowledgementDigest[:],
		int64Bytes(acknowledgement.AppliedArrivalSequence),
		int64Bytes(acknowledgement.ProducerSequence),
		acknowledgement.ProducerEnvelopeDigest[:],
		acknowledgement.PruneID[:],
		int64Bytes(acknowledgement.BarrierArrivalSequence),
		acknowledgement.ClosureReferenceDigest[:],
		uint32Bytes(acknowledgement.ManifestCount),
		acknowledgement.ManifestDigest[:],
	}
}

func parsePruneAcknowledgementBody(fields [][]byte) (PruneAcknowledgement, error) {
	if len(fields) != pruneAcknowledgementBodyFieldCount || len(fields[3]) != len(ChannelID{}) ||
		len(fields[4]) != len(RelayGeneration{}) || len(fields[6]) != len(Digest{}) || len(fields[8]) != len(Digest{}) ||
		len(fields[11]) != len(Digest{}) || len(fields[12]) != len(Digest{}) || len(fields[14]) != len(Digest{}) ||
		len(fields[16]) != len(Digest{}) {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	version, ok := parseUint16(fields[0])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	membershipGeneration, ok := parseUint32(fields[7])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	appliedArrivalSequence, ok := parseInt64(fields[9])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	producerSequence, ok := parseInt64(fields[10])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	barrierArrivalSequence, ok := parseInt64(fields[13])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	manifestCount, ok := parseUint32(fields[15])
	if !ok {
		return PruneAcknowledgement{}, ErrInvalidPruneAcknowledgement
	}
	acknowledgement := PruneAcknowledgement{
		Version:                version,
		ProtocolVersion:        protocolVersion,
		CipherSuite:            cipherSuite,
		EnvironmentID:          continuity.EnvironmentID(string(fields[5])),
		MembershipGeneration:   membershipGeneration,
		AppliedArrivalSequence: appliedArrivalSequence,
		ProducerSequence:       producerSequence,
		BarrierArrivalSequence: barrierArrivalSequence,
		ManifestCount:          manifestCount,
	}
	copy(acknowledgement.ChannelID[:], fields[3])
	copy(acknowledgement.RelayGeneration[:], fields[4])
	copy(acknowledgement.CertificateID[:], fields[6])
	copy(acknowledgement.ProgressAcknowledgementDigest[:], fields[8])
	copy(acknowledgement.ProducerEnvelopeDigest[:], fields[11])
	copy(acknowledgement.PruneID[:], fields[12])
	copy(acknowledgement.ClosureReferenceDigest[:], fields[14])
	copy(acknowledgement.ManifestDigest[:], fields[16])
	return acknowledgement, nil
}

func terminalRetirementBodyFields(retirement TerminalRetirement) [][]byte {
	return [][]byte{
		uint16Bytes(retirement.Version),
		uint16Bytes(retirement.ProtocolVersion),
		uint16Bytes(retirement.CipherSuite),
		retirement.ChannelID[:],
		retirement.RelayGeneration[:],
		[]byte(retirement.EnvironmentID),
		retirement.CertificateID[:],
		uint32Bytes(retirement.MembershipGeneration),
		int64Bytes(retirement.FinalEnvironmentSequence),
		retirement.FinalEnvelopeDigest[:],
	}
}

func parseTerminalRetirementBody(fields [][]byte) (TerminalRetirement, error) {
	if len(fields) != terminalRetirementBodyFieldCount || len(fields[3]) != len(ChannelID{}) ||
		len(fields[4]) != len(RelayGeneration{}) || len(fields[6]) != len(Digest{}) || len(fields[9]) != len(Digest{}) {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	version, ok := parseUint16(fields[0])
	if !ok {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	membershipGeneration, ok := parseUint32(fields[7])
	if !ok {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	finalEnvironmentSequence, ok := parseInt64(fields[8])
	if !ok {
		return TerminalRetirement{}, ErrInvalidRetirement
	}
	retirement := TerminalRetirement{
		Version:                  version,
		ProtocolVersion:          protocolVersion,
		CipherSuite:              cipherSuite,
		EnvironmentID:            continuity.EnvironmentID(string(fields[5])),
		MembershipGeneration:     membershipGeneration,
		FinalEnvironmentSequence: finalEnvironmentSequence,
	}
	copy(retirement.ChannelID[:], fields[3])
	copy(retirement.RelayGeneration[:], fields[4])
	copy(retirement.CertificateID[:], fields[6])
	copy(retirement.FinalEnvelopeDigest[:], fields[9])
	return retirement, nil
}

func pruneReferenceFields(reference PruneReference) [][]byte {
	return [][]byte{
		[]byte(reference.FactID),
		[]byte(reference.EnvironmentID),
		int64Bytes(reference.EnvironmentSequence),
		int64Bytes(reference.ArrivalSequence),
		reference.EnvelopeDigest[:],
		reference.CertificateID[:],
		reference.PreviousEnvelopeDigest[:],
		uint32Bytes(reference.KeyGeneration),
		reference.Nonce[:],
	}
}

func parsePruneReferenceFields(fields [][]byte) (PruneReference, error) {
	if len(fields) != pruneReferenceFieldCount || len(fields[4]) != len(Digest{}) || len(fields[5]) != len(Digest{}) ||
		len(fields[6]) != len(Digest{}) || len(fields[8]) != len(Nonce{}) {
		return PruneReference{}, ErrInvalidPruneReference
	}
	environmentSequence, ok := parseInt64(fields[2])
	if !ok {
		return PruneReference{}, ErrInvalidPruneReference
	}
	arrivalSequence, ok := parseInt64(fields[3])
	if !ok {
		return PruneReference{}, ErrInvalidPruneReference
	}
	keyGeneration, ok := parseUint32(fields[7])
	if !ok {
		return PruneReference{}, ErrInvalidPruneReference
	}
	reference := PruneReference{
		FactID:              continuity.FactID(string(fields[0])),
		EnvironmentID:       continuity.EnvironmentID(string(fields[1])),
		EnvironmentSequence: environmentSequence,
		ArrivalSequence:     arrivalSequence,
		KeyGeneration:       keyGeneration,
	}
	copy(reference.EnvelopeDigest[:], fields[4])
	copy(reference.CertificateID[:], fields[5])
	copy(reference.PreviousEnvelopeDigest[:], fields[6])
	copy(reference.Nonce[:], fields[8])
	return reference, nil
}

func pruneCertificateBodyTranscriptUnchecked(certificate PruneCertificate) ([]byte, error) {
	fields, err := pruneCertificateBodyFields(certificate)
	if err != nil {
		return nil, err
	}
	encoded, err := encodeTranscript(pruneCertificateBodyDomain, fields...)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	if len(encoded) > MaxControlObjectBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

func pruneCertificateBodyFields(certificate PruneCertificate) ([][]byte, error) {
	closure, err := certificate.Closure.MarshalBinary()
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	manifest, err := certificate.Manifest.MarshalBinary()
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	acknowledgements := make([][]byte, 0, len(certificate.Acknowledgements))
	for _, acknowledgement := range certificate.Acknowledgements {
		encoded, err := acknowledgement.MarshalBinary()
		if err != nil {
			return nil, controlEncodingError(err, ErrInvalidPruneCertificate)
		}
		acknowledgements = append(acknowledgements, encoded)
	}
	acknowledgementList, err := encodeControlObjectList(acknowledgements, MaxPruneAcknowledgements)
	if err != nil {
		return nil, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	return [][]byte{
		uint16Bytes(certificate.Version),
		uint16Bytes(certificate.ProtocolVersion),
		uint16Bytes(certificate.CipherSuite),
		certificate.ChannelID[:],
		certificate.RelayGeneration[:],
		certificate.PruneID[:],
		uint32Bytes(certificate.MembershipGeneration),
		int64Bytes(certificate.BarrierArrivalSequence),
		closure,
		certificate.ClosureDigest[:],
		uint32Bytes(certificate.ManifestCount),
		certificate.ManifestDigest[:],
		manifest,
		uint32Bytes(certificate.ActiveAcknowledgementCount),
		acknowledgementList,
	}, nil
}

func parsePruneCertificateBody(fields [][]byte) (PruneCertificate, error) {
	if len(fields) != pruneCertificateBodyFieldCount || len(fields[3]) != len(ChannelID{}) ||
		len(fields[4]) != len(RelayGeneration{}) || len(fields[5]) != len(Digest{}) || len(fields[9]) != len(Digest{}) ||
		len(fields[11]) != len(Digest{}) {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	version, ok := parseUint16(fields[0])
	if !ok {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	membershipGeneration, ok := parseUint32(fields[6])
	if !ok {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	barrierArrivalSequence, ok := parseInt64(fields[7])
	if !ok {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	manifestCount, ok := parseUint32(fields[10])
	if !ok {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	activeAcknowledgementCount, ok := parseUint32(fields[13])
	if !ok || activeAcknowledgementCount < 1 || activeAcknowledgementCount > MaxPruneAcknowledgements {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	closure, err := ParsePruneReference(fields[8])
	if err != nil {
		return PruneCertificate{}, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	manifest, err := ParsePruneManifest(fields[12])
	if err != nil {
		return PruneCertificate{}, controlEncodingError(err, ErrInvalidPruneCertificate)
	}
	acknowledgementBytes, err := parseControlObjectList(fields[14], MaxPruneAcknowledgements)
	if err != nil || uint32(len(acknowledgementBytes)) != activeAcknowledgementCount {
		return PruneCertificate{}, ErrInvalidPruneCertificate
	}
	acknowledgements := make([]PruneAcknowledgement, 0, len(acknowledgementBytes))
	for _, item := range acknowledgementBytes {
		acknowledgement, err := ParsePruneAcknowledgement(item)
		if err != nil {
			return PruneCertificate{}, controlEncodingError(err, ErrInvalidPruneCertificate)
		}
		acknowledgements = append(acknowledgements, acknowledgement)
	}
	certificate := PruneCertificate{
		Version:                    version,
		ProtocolVersion:            protocolVersion,
		CipherSuite:                cipherSuite,
		MembershipGeneration:       membershipGeneration,
		BarrierArrivalSequence:     barrierArrivalSequence,
		Closure:                    closure,
		ManifestCount:              manifestCount,
		Manifest:                   manifest,
		ActiveAcknowledgementCount: activeAcknowledgementCount,
		Acknowledgements:           acknowledgements,
	}
	copy(certificate.ChannelID[:], fields[3])
	copy(certificate.RelayGeneration[:], fields[4])
	copy(certificate.PruneID[:], fields[5])
	copy(certificate.ClosureDigest[:], fields[9])
	copy(certificate.ManifestDigest[:], fields[11])
	return certificate, nil
}

func encodeControlObjectList(items [][]byte, maximum int) ([]byte, error) {
	if len(items) < 1 || len(items) > maximum {
		return nil, ErrTooLarge
	}
	total := 4
	for _, item := range items {
		if len(item) < 1 || uint64(len(item)) > uint64(^uint32(0)) {
			return nil, ErrTooLarge
		}
		total += 4 + len(item)
		if total > MaxControlObjectBytes {
			return nil, ErrTooLarge
		}
	}
	encoded := make([]byte, 0, total)
	encoded = appendUint32(encoded, uint32(len(items)))
	for _, item := range items {
		encoded = appendUint32(encoded, uint32(len(item)))
		encoded = append(encoded, item...)
	}
	return encoded, nil
}

func parseControlObjectList(encoded []byte, maximum int) ([][]byte, error) {
	if len(encoded) < 4 || len(encoded) > MaxControlObjectBytes {
		return nil, ErrTooLarge
	}
	count := binary.BigEndian.Uint32(encoded[:4])
	if count < 1 || uint64(count) > uint64(maximum) {
		return nil, ErrInvalidPruneCertificate
	}
	items := make([][]byte, 0, int(count))
	offset := 4
	for range count {
		if offset+4 > len(encoded) {
			return nil, ErrInvalidPruneCertificate
		}
		length := binary.BigEndian.Uint32(encoded[offset : offset+4])
		offset += 4
		if length < 1 || uint64(length) > uint64(len(encoded)-offset) {
			return nil, ErrInvalidPruneCertificate
		}
		items = append(items, append([]byte(nil), encoded[offset:offset+int(length)]...))
		offset += int(length)
	}
	if offset != len(encoded) {
		return nil, ErrInvalidPruneCertificate
	}
	return items, nil
}

func digestControlObject(marshal func() ([]byte, error)) Digest {
	encoded, err := marshal()
	if err != nil {
		return Digest{}
	}
	return sha256.Sum256(encoded)
}

func canonicalControlEncoding(encoded []byte, marshal func() ([]byte, error)) bool {
	canonical, err := marshal()
	return err == nil && bytes.Equal(encoded, canonical)
}

func controlEncodingError(err, invalid error) error {
	if err == ErrTooLarge {
		return ErrTooLarge
	}
	return invalid
}
