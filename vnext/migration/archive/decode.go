package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type contentWireV1 struct {
	Source   Source             `json:"source"`
	Project  ProjectMapping     `json:"project"`
	Families FamilyManifest     `json:"included_families"`
	Records  json.RawMessage    `json:"records"`
	Expected ProjectionManifest `json:"expected_projection"`
}

// UnmarshalJSON bounds the structurally amplifying records array before the
// standard decoder can materialize an unbounded []Record value.
func (content *Content) UnmarshalJSON(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var wire contentWireV1
	if err := decoder.Decode(&wire); err != nil {
		return err
	}
	if err := requireJSONEOFV1(decoder); err != nil {
		return err
	}
	records, err := decodeRecordsV1(wire.Records, len(wire.Project.Label))
	if err != nil {
		return err
	}
	*content = Content{
		Source: wire.Source, Project: wire.Project, Families: wire.Families,
		Records: records, Expected: wire.Expected,
	}
	return nil
}

func decodeRecordsV1(encoded []byte, initialPayloadBytes int) ([]Record, error) {
	if initialPayloadBytes > maxAggregatePayloadBytes {
		return nil, fmt.Errorf("archive aggregate payload exceeds %d bytes", maxAggregatePayloadBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode archive records: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return nil, fmt.Errorf("archive records must be an array")
	}
	records := make([]Record, 0, 64)
	payloadBytes := initialPayloadBytes
	for decoder.More() {
		if len(records) >= maxRecords {
			return nil, fmt.Errorf("archive record count exceeds %d", maxRecords)
		}
		var record Record
		if err := decoder.Decode(&record); err != nil {
			return nil, fmt.Errorf("decode archive record %d: %w", len(records), err)
		}
		payloadBytes += recordPayloadBytesV1(record)
		if payloadBytes > maxAggregatePayloadBytes {
			return nil, fmt.Errorf("archive aggregate payload exceeds %d bytes", maxAggregatePayloadBytes)
		}
		records = append(records, record)
	}
	token, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode archive records: %w", err)
	}
	if delimiter, ok = token.(json.Delim); !ok || delimiter != ']' {
		return nil, fmt.Errorf("archive records array is not closed")
	}
	if err := requireJSONEOFV1(decoder); err != nil {
		return nil, fmt.Errorf("decode archive records: %w", err)
	}
	return records, nil
}

func requireJSONEOFV1(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func recordPayloadBytesV1(record Record) int {
	bytes := len(record.SourceID) + len(record.Observation.HarnessSessionID) +
		len(record.Observation.Branch) + len(record.Observation.Worktree)
	if record.Project != nil {
		bytes += len(record.Project.Label)
	}
	if record.Journal != nil {
		bytes += len(record.Journal.Scope) + len(record.Journal.Text)
	}
	if record.Wrap != nil {
		bytes += len(record.Wrap.Scope) + len(record.Wrap.Synthesis)
	}
	return bytes
}
