// Package continuitywire defines the exact internal plaintext boundary between
// continuity persistence and the vNext private-sync protocol.
package continuitywire

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	// Version1 is the first persisted-fact wire version.
	Version1 uint16 = 1
	// MaxFactBytes bounds one exact canonical persisted-fact wire value.
	MaxFactBytes = 1_052_672

	maximumPayloadBytes = 1_048_576
)

// ErrInvalidFact reports a fact that does not match the closed persisted-fact
// wire. It never includes payload content.
var ErrInvalidFact = errors.New("invalid continuity fact wire")

// Fact is one exact plaintext persisted fact. CanonicalPayload contains the
// byte-exact canonical JSON object stored in continuity_facts.content_json.
type Fact struct {
	WireVersion         uint16
	FactID              continuity.FactID
	ProjectID           continuity.ProjectID
	SubjectKind         continuity.RecordKind
	SubjectID           continuity.SubjectID
	FactKind            continuity.FactKind
	PayloadVersion      uint16
	CanonicalPayload    json.RawMessage
	EnvironmentID       continuity.EnvironmentID
	EnvironmentSequence int64
	HLCWallMillis       int64
	HLCLogical          int32
	EnvelopeVersion     uint16
}

type encodedFactV1 struct {
	WireVersion         uint16                   `json:"wire_version"`
	FactID              continuity.FactID        `json:"fact_id"`
	ProjectID           continuity.ProjectID     `json:"project_id"`
	SubjectKind         continuity.RecordKind    `json:"subject_kind"`
	SubjectID           continuity.SubjectID     `json:"subject_id"`
	FactKind            continuity.FactKind      `json:"fact_kind"`
	PayloadVersion      uint16                   `json:"payload_version"`
	CanonicalPayload    json.RawMessage          `json:"payload_json"`
	EnvironmentID       continuity.EnvironmentID `json:"environment_id"`
	EnvironmentSequence int64                    `json:"environment_sequence"`
	HLCWallMillis       int64                    `json:"hlc_wall_millis"`
	HLCLogical          int32                    `json:"hlc_logical"`
	EnvelopeVersion     uint16                   `json:"envelope_version"`
}

// Encode validates fact and returns its canonical fixed-field JSON encoding.
func Encode(fact Fact) ([]byte, error) {
	if err := Validate(fact); err != nil {
		return nil, err
	}
	wire := encodedFactV1{
		WireVersion:         fact.WireVersion,
		FactID:              fact.FactID,
		ProjectID:           fact.ProjectID,
		SubjectKind:         fact.SubjectKind,
		SubjectID:           fact.SubjectID,
		FactKind:            fact.FactKind,
		PayloadVersion:      fact.PayloadVersion,
		CanonicalPayload:    fact.CanonicalPayload,
		EnvironmentID:       fact.EnvironmentID,
		EnvironmentSequence: fact.EnvironmentSequence,
		HLCWallMillis:       fact.HLCWallMillis,
		HLCLogical:          fact.HLCLogical,
		EnvelopeVersion:     fact.EnvelopeVersion,
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(wire); err != nil {
		return nil, invalidFact("cannot encode")
	}
	encoded := buffer.Bytes()
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		return nil, invalidFact("cannot encode")
	}
	encoded = encoded[:len(encoded)-1]
	if len(encoded) > MaxFactBytes {
		return nil, invalidFact("encoding exceeds the size limit")
	}
	return append([]byte(nil), encoded...), nil
}

// Decode strictly decodes one canonical fixed-field JSON fact.
func Decode(encoded []byte) (Fact, error) {
	if len(encoded) < 2 || len(encoded) > MaxFactBytes {
		return Fact{}, invalidFact("encoding size is outside the limit")
	}
	var wire encodedFactV1
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return Fact{}, invalidFact("cannot decode")
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Fact{}, invalidFact("contains trailing data")
	}
	fact := Fact{
		WireVersion:         wire.WireVersion,
		FactID:              wire.FactID,
		ProjectID:           wire.ProjectID,
		SubjectKind:         wire.SubjectKind,
		SubjectID:           wire.SubjectID,
		FactKind:            wire.FactKind,
		PayloadVersion:      wire.PayloadVersion,
		CanonicalPayload:    append(json.RawMessage(nil), wire.CanonicalPayload...),
		EnvironmentID:       wire.EnvironmentID,
		EnvironmentSequence: wire.EnvironmentSequence,
		HLCWallMillis:       wire.HLCWallMillis,
		HLCLogical:          wire.HLCLogical,
		EnvelopeVersion:     wire.EnvelopeVersion,
	}
	canonical, err := Encode(fact)
	if err != nil {
		return Fact{}, err
	}
	if !bytes.Equal(canonical, encoded) {
		return Fact{}, invalidFact("encoding is not canonical")
	}
	return fact, nil
}

// Validate checks the closed persisted-field shape without changing any bytes.
// The persistence adapter additionally validates the fact-kind-specific payload.
func Validate(fact Fact) error {
	if fact.WireVersion != Version1 {
		return invalidFact("unsupported wire version")
	}
	if err := fact.FactID.Validate(); err != nil {
		return invalidFact("invalid fact id")
	}
	if err := fact.ProjectID.Validate(); err != nil {
		return invalidFact("invalid project id")
	}
	if err := fact.SubjectID.Validate(); err != nil {
		return invalidFact("invalid subject id")
	}
	definition, ok := continuity.DefinitionFor(fact.FactKind)
	if !ok || definition.Record != fact.SubjectKind {
		return invalidFact("fact kind does not match the closed subject family")
	}
	if fact.SubjectKind == continuity.RecordProjectIdentity && fact.SubjectID != continuity.SubjectID(fact.ProjectID) {
		return invalidFact("project identity subject does not match project")
	}
	if fact.PayloadVersion != 1 {
		return invalidFact("unsupported payload version")
	}
	if len(fact.CanonicalPayload) < 2 || len(fact.CanonicalPayload) > maximumPayloadBytes ||
		fact.CanonicalPayload[0] != '{' || fact.CanonicalPayload[len(fact.CanonicalPayload)-1] != '}' ||
		!json.Valid(fact.CanonicalPayload) {
		return invalidFact("payload is not a bounded JSON object")
	}
	if err := fact.EnvironmentID.Validate(); err != nil {
		return invalidFact("invalid environment id")
	}
	if fact.EnvironmentSequence < 1 {
		return invalidFact("environment sequence must begin at one")
	}
	if fact.HLCWallMillis < 0 || fact.HLCLogical < 0 {
		return invalidFact("hybrid clock is negative")
	}
	if fact.EnvelopeVersion != 1 {
		return invalidFact("unsupported continuity envelope version")
	}
	return nil
}

// Equal reports byte-exact equality across every immutable persisted field.
func Equal(left, right Fact) bool {
	return left.WireVersion == right.WireVersion &&
		left.FactID == right.FactID &&
		left.ProjectID == right.ProjectID &&
		left.SubjectKind == right.SubjectKind &&
		left.SubjectID == right.SubjectID &&
		left.FactKind == right.FactKind &&
		left.PayloadVersion == right.PayloadVersion &&
		bytes.Equal(left.CanonicalPayload, right.CanonicalPayload) &&
		left.EnvironmentID == right.EnvironmentID &&
		left.EnvironmentSequence == right.EnvironmentSequence &&
		left.HLCWallMillis == right.HLCWallMillis &&
		left.HLCLogical == right.HLCLogical &&
		left.EnvelopeVersion == right.EnvelopeVersion
}

func invalidFact(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidFact, detail)
}
