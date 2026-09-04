package protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	aadDomain                 = "loaf.sync.fact.aad.v1"
	factSignatureDomain       = "loaf.sync.fact.signature.v1"
	envelopeDomain            = "loaf.sync.fact.envelope.v1"
	certificateBodyDomain     = "loaf.sync.environment-certificate.body.v1"
	certificateDomain         = "loaf.sync.environment-certificate.v1"
	generationKeySaltDomain   = "loaf.sync.generation-key.salt.v1"
	generationKeyInfoDomain   = "loaf.sync.generation-key.info.v1"
	factHeaderFieldCount      = 10
	sealedFactFieldCount      = factHeaderFieldCount + 2
	certificateBodyFieldCount = 11
	certificateFieldCount     = certificateBodyFieldCount + 1
)

// AADTranscript returns the exact associated data for fact encryption.
func AADTranscript(header FactHeader) ([]byte, error) {
	if err := header.Validate(); err != nil {
		return nil, err
	}
	return aadTranscriptUnchecked(header)
}

func aadTranscriptUnchecked(header FactHeader) ([]byte, error) {
	return encodeTranscript(aadDomain, headerFields(header)...)
}

// FactSignatureTranscript returns the exact bytes an environment signs.
func FactSignatureTranscript(header FactHeader, ciphertext []byte) ([]byte, error) {
	if err := header.Validate(); err != nil {
		return nil, err
	}
	if len(ciphertext) < 16 {
		return nil, ErrInvalidEnvelope
	}
	if len(ciphertext) > MaxCiphertextBytes {
		return nil, ErrTooLarge
	}
	fields := append(headerFields(header), ciphertext)
	return encodeTranscript(factSignatureDomain, fields...)
}

// MarshalBinary encodes a signed fact as its canonical immutable envelope.
func (fact SealedFact) MarshalBinary() ([]byte, error) {
	if err := fact.Validate(); err != nil {
		return nil, err
	}
	fields := append(headerFields(fact.Header), fact.Ciphertext, fact.Signature[:])
	encoded, err := encodeTranscript(envelopeDomain, fields...)
	if err != nil {
		return nil, ErrInvalidEnvelope
	}
	if len(encoded) > MaxEnvelopeBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParseSealedFact strictly decodes one canonical signed fact envelope.
func ParseSealedFact(encoded []byte) (SealedFact, error) {
	if len(encoded) > MaxEnvelopeBytes {
		return SealedFact{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, envelopeDomain, sealedFactFieldCount)
	if err != nil {
		return SealedFact{}, ErrInvalidEnvelope
	}
	header, err := parseHeader(fields[:factHeaderFieldCount])
	if err != nil {
		return SealedFact{}, err
	}
	if len(fields[10]) < 16 || len(fields[10]) > MaxCiphertextBytes || len(fields[11]) != len(Signature{}) {
		return SealedFact{}, ErrInvalidEnvelope
	}
	fact := SealedFact{Header: header, Ciphertext: append([]byte(nil), fields[10]...)}
	copy(fact.Signature[:], fields[11])
	if err := fact.Validate(); err != nil {
		return SealedFact{}, err
	}
	return fact, nil
}

// EnvelopeDigest returns the immutable SHA-256 identity of the complete
// canonical envelope. Invalid in-memory values hash to the all-zero digest.
func EnvelopeDigest(fact SealedFact) Digest {
	encoded, err := fact.MarshalBinary()
	if err != nil {
		return Digest{}
	}
	return sha256.Sum256(encoded)
}

// ImmutableRelation classifies relay/storage idempotency without accepting an
// ID-only duplicate.
type ImmutableRelation uint8

const (
	// ImmutableDistinct means neither immutable identity collides.
	ImmutableDistinct ImmutableRelation = iota
	// ImmutableExactReplay means both identities and full-envelope digest match.
	ImmutableExactReplay
	// ImmutableConflict means a fact ID or source sequence was reused for
	// different immutable bytes.
	ImmutableConflict
)

// CompareImmutable compares the two relay uniqueness identities and complete
// envelope digests.
func CompareImmutable(left, right SealedFact) ImmutableRelation {
	sameFact := left.Header.FactID == right.Header.FactID
	sameSequence := left.Header.EnvironmentID == right.Header.EnvironmentID && left.Header.EnvironmentSequence == right.Header.EnvironmentSequence
	if !sameFact && !sameSequence {
		return ImmutableDistinct
	}
	leftDigest, leftValid := validEnvelopeDigest(left)
	rightDigest, rightValid := validEnvelopeDigest(right)
	if sameFact && sameSequence && leftValid && rightValid && leftDigest == rightDigest {
		return ImmutableExactReplay
	}
	return ImmutableConflict
}

func validEnvelopeDigest(fact SealedFact) (Digest, bool) {
	encoded, err := fact.MarshalBinary()
	if err != nil {
		return Digest{}, false
	}
	return sha256.Sum256(encoded), true
}

// CertificateBodyTranscript returns the exact bytes signed by the project
// administrator.
func CertificateBodyTranscript(certificate EnvironmentCertificate) ([]byte, error) {
	if err := certificate.Validate(); err != nil {
		return nil, err
	}
	return certificateBodyTranscriptUnchecked(certificate)
}

func certificateBodyTranscriptUnchecked(certificate EnvironmentCertificate) ([]byte, error) {
	return encodeTranscript(certificateBodyDomain, certificateBodyFields(certificate)...)
}

// MarshalBinary encodes a certificate as its canonical immutable bytes.
func (certificate EnvironmentCertificate) MarshalBinary() ([]byte, error) {
	if err := certificate.Validate(); err != nil {
		return nil, err
	}
	fields := append(certificateBodyFields(certificate), certificate.AdminSignature[:])
	encoded, err := encodeTranscript(certificateDomain, fields...)
	if err != nil {
		return nil, ErrInvalidCertificate
	}
	if len(encoded) > MaxCertificateBytes {
		return nil, ErrTooLarge
	}
	return encoded, nil
}

// ParseEnvironmentCertificate strictly decodes canonical certificate bytes.
func ParseEnvironmentCertificate(encoded []byte) (EnvironmentCertificate, error) {
	if len(encoded) > MaxCertificateBytes {
		return EnvironmentCertificate{}, ErrTooLarge
	}
	fields, err := parseTranscript(encoded, certificateDomain, certificateFieldCount)
	if err != nil {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	certificate, err := parseCertificateBody(fields[:certificateBodyFieldCount])
	if err != nil {
		return EnvironmentCertificate{}, err
	}
	if len(fields[11]) != len(Signature{}) {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	copy(certificate.AdminSignature[:], fields[11])
	if err := certificate.Validate(); err != nil {
		return EnvironmentCertificate{}, err
	}
	return certificate, nil
}

// CertificateID returns the SHA-256 identity of the complete signed
// certificate. Invalid in-memory values hash to the all-zero digest.
func CertificateID(certificate EnvironmentCertificate) Digest {
	encoded, err := certificate.MarshalBinary()
	if err != nil {
		return Digest{}
	}
	return sha256.Sum256(encoded)
}

// GenerationKeySalt returns the project-specific HKDF salt.
func GenerationKeySalt(projectID continuity.ProjectID) ([]byte, error) {
	if err := projectID.Validate(); err != nil {
		return nil, ErrInvalidCertificate
	}
	transcript, err := encodeTranscript(generationKeySaltDomain, []byte(projectID))
	if err != nil {
		return nil, ErrInvalidCertificate
	}
	digest := sha256.Sum256(transcript)
	return digest[:], nil
}

// GenerationKeyInfo returns the suite- and generation-specific HKDF context.
func GenerationKeyInfo(protocolVersion, cipherSuite uint16, generation uint32) ([]byte, error) {
	if protocolVersion != ProtocolVersionV1 {
		return nil, ErrUnsupportedProtocolVersion
	}
	if cipherSuite != CipherSuiteXChaCha20Poly1305 {
		return nil, ErrUnsupportedCipherSuite
	}
	if generation < 1 {
		return nil, ErrInvalidHeader
	}
	return encodeTranscript(generationKeyInfoDomain, uint16Bytes(protocolVersion), uint16Bytes(cipherSuite), uint32Bytes(generation))
}

func headerFields(header FactHeader) [][]byte {
	return [][]byte{
		uint16Bytes(header.ProtocolVersion),
		uint16Bytes(header.CipherSuite),
		header.ChannelID[:],
		[]byte(header.FactID),
		[]byte(header.EnvironmentID),
		int64Bytes(header.EnvironmentSequence),
		uint32Bytes(header.KeyGeneration),
		header.PreviousEnvelopeDigest[:],
		header.CertificateID[:],
		header.Nonce[:],
	}
}

func parseHeader(fields [][]byte) (FactHeader, error) {
	if len(fields) != factHeaderFieldCount || len(fields[2]) != len(ChannelID{}) || len(fields[7]) != len(Digest{}) || len(fields[8]) != len(Digest{}) || len(fields[9]) != len(Nonce{}) {
		return FactHeader{}, ErrInvalidEnvelope
	}
	protocolVersion, ok := parseUint16(fields[0])
	if !ok {
		return FactHeader{}, ErrInvalidEnvelope
	}
	cipherSuite, ok := parseUint16(fields[1])
	if !ok {
		return FactHeader{}, ErrInvalidEnvelope
	}
	environmentSequence, ok := parseInt64(fields[5])
	if !ok {
		return FactHeader{}, ErrInvalidEnvelope
	}
	keyGeneration, ok := parseUint32(fields[6])
	if !ok {
		return FactHeader{}, ErrInvalidEnvelope
	}
	header := FactHeader{
		ProtocolVersion:     protocolVersion,
		CipherSuite:         cipherSuite,
		FactID:              continuity.FactID(string(fields[3])),
		EnvironmentID:       continuity.EnvironmentID(string(fields[4])),
		EnvironmentSequence: environmentSequence,
		KeyGeneration:       keyGeneration,
	}
	copy(header.ChannelID[:], fields[2])
	copy(header.PreviousEnvelopeDigest[:], fields[7])
	copy(header.CertificateID[:], fields[8])
	copy(header.Nonce[:], fields[9])
	if err := header.Validate(); err != nil {
		return FactHeader{}, err
	}
	return header, nil
}

func certificateBodyFields(certificate EnvironmentCertificate) [][]byte {
	return [][]byte{
		uint16Bytes(certificate.Version),
		uint16Bytes(certificate.ProtocolVersion),
		uint16Bytes(certificate.CipherSuite),
		[]byte(certificate.ProjectID),
		certificate.ChannelID[:],
		[]byte(certificate.EnvironmentID),
		certificate.EnvironmentPublicKey[:],
		{byte(certificate.Mode)},
		uint32Bytes(certificate.MembershipGeneration),
		generationSetBytes(certificate.AllowedKeyGenerations),
		int64Bytes(certificate.ExpiresAtMillis),
	}
}

func parseCertificateBody(fields [][]byte) (EnvironmentCertificate, error) {
	if len(fields) != certificateBodyFieldCount || len(fields[4]) != len(ChannelID{}) || len(fields[6]) != len(PublicKey{}) || len(fields[7]) != 1 {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	version, ok := parseUint16(fields[0])
	if !ok {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	protocolVersion, ok := parseUint16(fields[1])
	if !ok {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	cipherSuite, ok := parseUint16(fields[2])
	if !ok {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	membershipGeneration, ok := parseUint32(fields[8])
	if !ok {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	allowed, ok := parseGenerationSet(fields[9])
	if !ok {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	expiresAtMillis, ok := parseInt64(fields[10])
	if !ok {
		return EnvironmentCertificate{}, ErrInvalidCertificate
	}
	certificate := EnvironmentCertificate{
		Version:               version,
		ProtocolVersion:       protocolVersion,
		CipherSuite:           cipherSuite,
		ProjectID:             continuity.ProjectID(string(fields[3])),
		EnvironmentID:         continuity.EnvironmentID(string(fields[5])),
		Mode:                  EnvironmentMode(fields[7][0]),
		MembershipGeneration:  membershipGeneration,
		AllowedKeyGenerations: allowed,
		ExpiresAtMillis:       expiresAtMillis,
	}
	copy(certificate.ChannelID[:], fields[4])
	copy(certificate.EnvironmentPublicKey[:], fields[6])
	return certificate, nil
}

func generationSetBytes(generations []uint32) []byte {
	encoded := make([]byte, 4+4*len(generations))
	binary.BigEndian.PutUint32(encoded[:4], uint32(len(generations)))
	for index, generation := range generations {
		binary.BigEndian.PutUint32(encoded[4+4*index:], generation)
	}
	return encoded
}

func parseGenerationSet(encoded []byte) ([]uint32, bool) {
	if len(encoded) < 4 {
		return nil, false
	}
	count := binary.BigEndian.Uint32(encoded[:4])
	if count > MaxAllowedKeyGenerations || uint64(len(encoded)) != 4+4*uint64(count) {
		return nil, false
	}
	generations := make([]uint32, count)
	for index := range generations {
		generations[index] = binary.BigEndian.Uint32(encoded[4+4*index:])
	}
	return generations, true
}

func encodeTranscript(domain string, fields ...[]byte) ([]byte, error) {
	total := 8 + len(domain)
	for _, field := range fields {
		if uint64(len(field)) > uint64(^uint32(0)) {
			return nil, ErrTooLarge
		}
		total += 4 + len(field)
		if total > MaxEnvelopeBytes && domain != certificateDomain && domain != certificateBodyDomain && domain != generationKeySaltDomain && domain != generationKeyInfoDomain {
			return nil, ErrTooLarge
		}
	}
	encoded := make([]byte, 0, total)
	encoded = appendUint32(encoded, uint32(len(domain)))
	encoded = append(encoded, domain...)
	encoded = appendUint32(encoded, uint32(len(fields)))
	for _, field := range fields {
		encoded = appendUint32(encoded, uint32(len(field)))
		encoded = append(encoded, field...)
	}
	return encoded, nil
}

func parseTranscript(encoded []byte, domain string, fieldCount int) ([][]byte, error) {
	if len(encoded) < 8 {
		return nil, ErrInvalidEnvelope
	}
	domainLength := binary.BigEndian.Uint32(encoded[:4])
	if uint64(domainLength) > uint64(len(encoded)-8) {
		return nil, ErrInvalidEnvelope
	}
	offset := 4
	if string(encoded[offset:offset+int(domainLength)]) != domain {
		return nil, ErrInvalidEnvelope
	}
	offset += int(domainLength)
	if offset+4 > len(encoded) || binary.BigEndian.Uint32(encoded[offset:offset+4]) != uint32(fieldCount) {
		return nil, ErrInvalidEnvelope
	}
	offset += 4
	fields := make([][]byte, 0, fieldCount)
	for range fieldCount {
		if offset+4 > len(encoded) {
			return nil, ErrInvalidEnvelope
		}
		fieldLength := binary.BigEndian.Uint32(encoded[offset : offset+4])
		offset += 4
		if uint64(fieldLength) > uint64(len(encoded)-offset) {
			return nil, ErrInvalidEnvelope
		}
		field := append([]byte(nil), encoded[offset:offset+int(fieldLength)]...)
		fields = append(fields, field)
		offset += int(fieldLength)
	}
	if offset != len(encoded) {
		return nil, ErrInvalidEnvelope
	}
	return fields, nil
}

func appendUint32(target []byte, value uint32) []byte {
	return binary.BigEndian.AppendUint32(target, value)
}

func uint16Bytes(value uint16) []byte {
	return binary.BigEndian.AppendUint16(nil, value)
}

func uint32Bytes(value uint32) []byte {
	return binary.BigEndian.AppendUint32(nil, value)
}

func int64Bytes(value int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(value))
}

func parseUint16(value []byte) (uint16, bool) {
	if len(value) != 2 {
		return 0, false
	}
	return binary.BigEndian.Uint16(value), true
}

func parseUint32(value []byte) (uint32, bool) {
	if len(value) != 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(value), true
}

func parseInt64(value []byte) (int64, bool) {
	if len(value) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(value)), true
}

// EqualDigest compares fixed digests without converting them to text.
func EqualDigest(left, right Digest) bool {
	return bytes.Equal(left[:], right[:])
}
