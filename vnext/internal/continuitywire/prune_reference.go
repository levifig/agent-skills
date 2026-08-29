package continuitywire

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	pruneReferenceDomainV1     = "loaf.sync.prune-reference.v1"
	pruneReferenceFieldCountV1 = 9

	// MaxPruneReferenceBytes bounds one exact opaque sync arrival reference.
	MaxPruneReferenceBytes = 512
)

var (
	// ErrInvalidPruneReference identifies malformed or noncanonical reference
	// bytes without exposing any reference content.
	ErrInvalidPruneReference = errors.New("invalid continuity prune reference wire")
	// ErrPruneReferenceTooLarge identifies input rejected by the fixed bound
	// before parsing or allocation.
	ErrPruneReferenceTooLarge = errors.New("continuity prune reference exceeds fixed limit")
)

// PruneReference is the exact opaque identity retained for one sync arrival.
// It contains no decrypted continuity subject or fact kind.
type PruneReference struct {
	FactID                 continuity.FactID
	EnvironmentID          continuity.EnvironmentID
	EnvironmentSequence    int64
	ArrivalSequence        int64
	EnvelopeDigest         [32]byte
	CertificateID          [32]byte
	PreviousEnvelopeDigest [32]byte
	KeyGeneration          uint32
	Nonce                  [24]byte
}

// Validate checks the complete closed reference shape.
func (reference PruneReference) Validate() error {
	if reference.FactID.Validate() != nil || reference.EnvironmentID.Validate() != nil ||
		reference.EnvironmentSequence < 1 || reference.ArrivalSequence < 1 || reference.KeyGeneration < 1 ||
		zeroPruneReferenceBytes(reference.EnvelopeDigest[:]) || zeroPruneReferenceBytes(reference.CertificateID[:]) ||
		(reference.EnvironmentSequence == 1) != zeroPruneReferenceBytes(reference.PreviousEnvelopeDigest[:]) {
		return ErrInvalidPruneReference
	}
	return nil
}

// EncodePruneReference returns the canonical V1 reference transcript.
func EncodePruneReference(reference PruneReference) ([]byte, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	fields := [][]byte{
		[]byte(reference.FactID),
		[]byte(reference.EnvironmentID),
		pruneReferenceInt64Bytes(reference.EnvironmentSequence),
		pruneReferenceInt64Bytes(reference.ArrivalSequence),
		reference.EnvelopeDigest[:],
		reference.CertificateID[:],
		reference.PreviousEnvelopeDigest[:],
		pruneReferenceUint32Bytes(reference.KeyGeneration),
		reference.Nonce[:],
	}
	return encodePruneReferenceTranscriptV1(fields)
}

// DecodePruneReference strictly decodes one canonical V1 reference transcript.
func DecodePruneReference(encoded []byte) (PruneReference, error) {
	if len(encoded) > MaxPruneReferenceBytes {
		return PruneReference{}, ErrPruneReferenceTooLarge
	}
	fields, err := parsePruneReferenceTranscriptV1(encoded)
	if err != nil || len(fields[4]) != len([32]byte{}) || len(fields[5]) != len([32]byte{}) ||
		len(fields[6]) != len([32]byte{}) || len(fields[8]) != len([24]byte{}) {
		return PruneReference{}, ErrInvalidPruneReference
	}
	environmentSequence, ok := parsePruneReferenceInt64(fields[2])
	if !ok {
		return PruneReference{}, ErrInvalidPruneReference
	}
	arrivalSequence, ok := parsePruneReferenceInt64(fields[3])
	if !ok {
		return PruneReference{}, ErrInvalidPruneReference
	}
	keyGeneration, ok := parsePruneReferenceUint32(fields[7])
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
	if err := reference.Validate(); err != nil {
		return PruneReference{}, err
	}
	canonical, err := EncodePruneReference(reference)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return PruneReference{}, ErrInvalidPruneReference
	}
	return reference, nil
}

// PruneReferenceDigest returns the SHA-256 identity of the complete canonical
// reference transcript.
func PruneReferenceDigest(reference PruneReference) ([32]byte, error) {
	encoded, err := EncodePruneReference(reference)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func encodePruneReferenceTranscriptV1(fields [][]byte) ([]byte, error) {
	if len(fields) != pruneReferenceFieldCountV1 {
		return nil, ErrInvalidPruneReference
	}
	total := uint64(8 + len(pruneReferenceDomainV1))
	for _, field := range fields {
		if uint64(len(field)) > uint64(^uint32(0)) {
			return nil, ErrPruneReferenceTooLarge
		}
		total += 4 + uint64(len(field))
		if total > MaxPruneReferenceBytes {
			return nil, ErrPruneReferenceTooLarge
		}
	}
	encoded := make([]byte, 0, int(total))
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(pruneReferenceDomainV1)))
	encoded = append(encoded, pruneReferenceDomainV1...)
	encoded = binary.BigEndian.AppendUint32(encoded, pruneReferenceFieldCountV1)
	for _, field := range fields {
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field)))
		encoded = append(encoded, field...)
	}
	return encoded, nil
}

func parsePruneReferenceTranscriptV1(encoded []byte) ([][]byte, error) {
	if len(encoded) < 8 {
		return nil, ErrInvalidPruneReference
	}
	domainLength := binary.BigEndian.Uint32(encoded[:4])
	if uint64(domainLength) > uint64(len(encoded)-8) {
		return nil, ErrInvalidPruneReference
	}
	offset := 4
	if string(encoded[offset:offset+int(domainLength)]) != pruneReferenceDomainV1 {
		return nil, ErrInvalidPruneReference
	}
	offset += int(domainLength)
	if offset+4 > len(encoded) || binary.BigEndian.Uint32(encoded[offset:offset+4]) != pruneReferenceFieldCountV1 {
		return nil, ErrInvalidPruneReference
	}
	offset += 4
	fields := make([][]byte, 0, pruneReferenceFieldCountV1)
	for range pruneReferenceFieldCountV1 {
		if offset+4 > len(encoded) {
			return nil, ErrInvalidPruneReference
		}
		fieldLength := binary.BigEndian.Uint32(encoded[offset : offset+4])
		offset += 4
		if uint64(fieldLength) > uint64(len(encoded)-offset) {
			return nil, ErrInvalidPruneReference
		}
		field := append([]byte(nil), encoded[offset:offset+int(fieldLength)]...)
		fields = append(fields, field)
		offset += int(fieldLength)
	}
	if offset != len(encoded) {
		return nil, ErrInvalidPruneReference
	}
	return fields, nil
}

func pruneReferenceInt64Bytes(value int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(value))
}

func pruneReferenceUint32Bytes(value uint32) []byte {
	return binary.BigEndian.AppendUint32(nil, value)
}

func parsePruneReferenceInt64(value []byte) (int64, bool) {
	if len(value) != 8 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(value)), true
}

func parsePruneReferenceUint32(value []byte) (uint32, bool) {
	if len(value) != 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(value), true
}

func zeroPruneReferenceBytes(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
