package continuitywire

import (
	"bytes"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestFactCanonicalRoundTripPreservesPersistedBytes(t *testing.T) {
	t.Parallel()

	fact := testFact()
	encoded, err := Encode(fact)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	want := []byte(`{"wire_version":1,"fact_id":"fact-1","project_id":"project-1","subject_kind":"project-identity","subject_id":"project-1","fact_kind":"project.registered","payload_version":1,"payload_json":{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"Loaf <private>"},"environment_id":"environment-a","environment_sequence":1,"hlc_wall_millis":100,"hlc_logical":2,"envelope_version":1}`)
	if !bytes.Equal(encoded, want) {
		t.Fatalf("Encode() = %s, want %s", encoded, want)
	}

	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !Equal(decoded, fact) {
		t.Fatalf("Decode() = %#v, want %#v", decoded, fact)
	}
	if !bytes.Equal(decoded.CanonicalPayload, fact.CanonicalPayload) {
		t.Fatalf("payload = %s, want byte-exact %s", decoded.CanonicalPayload, fact.CanonicalPayload)
	}
}

func TestFactDecodeRejectsUnknownTrailingAndNoncanonicalWire(t *testing.T) {
	t.Parallel()

	canonical, err := Encode(testFact())
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	tests := map[string][]byte{
		"unknown field":  bytes.Replace(canonical, []byte(`,"fact_id"`), []byte(`,"unknown":true,"fact_id"`), 1),
		"trailing value": append(append([]byte(nil), canonical...), []byte(` {}`)...),
		"whitespace":     append([]byte(" "), canonical...),
		"reordered":      bytes.Replace(canonical, []byte(`{"wire_version":1,"fact_id":"fact-1"`), []byte(`{"fact_id":"fact-1","wire_version":1`), 1),
	}
	for name, encoded := range tests {
		name, encoded := name, encoded
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode(encoded); err == nil {
				t.Fatal("Decode() error = nil, want refusal")
			}
		})
	}
}

func TestFactValidationRejectsInvalidPersistedFields(t *testing.T) {
	t.Parallel()

	tests := map[string]func(*Fact){
		"wire version":         func(fact *Fact) { fact.WireVersion = 2 },
		"fact id":              func(fact *Fact) { fact.FactID = "bad id" },
		"project id":           func(fact *Fact) { fact.ProjectID = "" },
		"subject family":       func(fact *Fact) { fact.SubjectKind = continuity.RecordJournalEntry },
		"subject id":           func(fact *Fact) { fact.SubjectID = "bad id" },
		"fact kind":            func(fact *Fact) { fact.FactKind = "tracker.synced" },
		"payload version":      func(fact *Fact) { fact.PayloadVersion = 2 },
		"payload shape":        func(fact *Fact) { fact.CanonicalPayload = []byte(`[]`) },
		"environment id":       func(fact *Fact) { fact.EnvironmentID = "" },
		"environment sequence": func(fact *Fact) { fact.EnvironmentSequence = 0 },
		"wall clock":           func(fact *Fact) { fact.HLCWallMillis = -1 },
		"logical clock":        func(fact *Fact) { fact.HLCLogical = -1 },
		"envelope version":     func(fact *Fact) { fact.EnvelopeVersion = 2 },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fact := testFact()
			mutate(&fact)
			if err := Validate(fact); err == nil {
				t.Fatal("Validate() error = nil, want refusal")
			}
		})
	}
}

func TestMaxFactBytesIsExactAndEnforced(t *testing.T) {
	t.Parallel()

	if MaxFactBytes != 1_052_672 {
		t.Fatalf("MaxFactBytes = %d, want 1052672", MaxFactBytes)
	}
	if _, err := Decode(make([]byte, MaxFactBytes+1)); !errors.Is(err, ErrInvalidFact) {
		t.Fatalf("Decode(oversized fact) error = %v, want %v", err, ErrInvalidFact)
	}
}

func testFact() Fact {
	return Fact{
		WireVersion:         Version1,
		FactID:              "fact-1",
		ProjectID:           "project-1",
		SubjectKind:         continuity.RecordProjectIdentity,
		SubjectID:           "project-1",
		FactKind:            continuity.FactProjectRegistered,
		PayloadVersion:      1,
		CanonicalPayload:    []byte(`{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"Loaf <private>"}`),
		EnvironmentID:       "environment-a",
		EnvironmentSequence: 1,
		HLCWallMillis:       100,
		HLCLogical:          2,
		EnvelopeVersion:     1,
	}
}
