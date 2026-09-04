package sqlite

import (
	"strings"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuityCodecNormalizesCollectionsAndPreservesLiteralHTMLCharacters(t *testing.T) {
	t.Parallel()

	observation := continuity.Observation{ObservedAtMillis: 1}
	nilPayload := continuity.HandoffRecordedPayload{
		Observation: observation,
		Purpose:     strings.Repeat("<&>", 1024),
	}
	emptyPayload := nilPayload
	emptyPayload.SuggestedSkills = []string{}

	nilEncoded, err := encodeHandoffRecordedV1(nilPayload)
	if err != nil {
		t.Fatalf("encode nil-list handoff: %v", err)
	}
	emptyEncoded, err := encodeHandoffRecordedV1(emptyPayload)
	if err != nil {
		t.Fatalf("encode empty-list handoff: %v", err)
	}
	if nilEncoded != emptyEncoded {
		t.Fatalf("nil and empty lists encoded differently:\n%s\n%s", nilEncoded, emptyEncoded)
	}
	if strings.Contains(string(nilEncoded), `\u003c`) || strings.Contains(string(nilEncoded), `\u003e`) || strings.Contains(string(nilEncoded), `\u0026`) {
		t.Fatalf("canonical content HTML-escaped literal characters: %s", nilEncoded)
	}
	if !strings.Contains(string(nilEncoded), `"suggested_skills":[]`) {
		t.Fatalf("canonical content did not normalize the list: %s", nilEncoded)
	}
}

func TestContinuityCodecRejectsContentAboveSchemaLimit(t *testing.T) {
	t.Parallel()

	payload := wireHandoffRecordedV1{
		Observation:     wireObservationV1{},
		Purpose:         strings.Repeat("x", maximumContentBytes),
		SuggestedSkills: []string{},
	}
	_, err := encodeWireV1(payload)
	problem, ok := err.(*continuity.Problem)
	if !ok {
		t.Fatalf("encode oversized content error type = %T, want *continuity.Problem", err)
	}
	if problem.Code != continuity.ProblemInvalid || problem.Field != "content" {
		t.Fatalf("encode oversized content problem = %#v", problem)
	}
}

func TestContinuityCodecEnforcesExactSchemaByteBoundary(t *testing.T) {
	t.Parallel()

	payload := wireHandoffRecordedV1{Observation: wireObservationV1{}, SuggestedSkills: []string{}}
	base, err := encodeWireV1(payload)
	if err != nil {
		t.Fatalf("encode base wire payload: %v", err)
	}
	payload.Purpose = strings.Repeat("x", maximumContentBytes-len(base))
	exact, err := encodeWireV1(payload)
	if err != nil {
		t.Fatalf("encode exact boundary: %v", err)
	}
	if len(exact) != maximumContentBytes {
		t.Fatalf("exact encoded length = %d, want %d", len(exact), maximumContentBytes)
	}
	payload.Purpose += "x"
	_, err = encodeWireV1(payload)
	problem, ok := err.(*continuity.Problem)
	if !ok || problem.Code != continuity.ProblemInvalid {
		t.Fatalf("one-byte-over error = %#v (%T), want invalid", err, err)
	}
}

func TestContinuityCodecRejectsNonCanonicalStoredContent(t *testing.T) {
	t.Parallel()

	payload := continuity.ProjectRegistrationPayload{
		Observation: continuity.Observation{ObservedAtMillis: 1},
		Label:       "Loaf",
	}
	canonical, err := encodeProjectRegistrationV1(payload)
	if err != nil {
		t.Fatalf("encode canonical project registration: %v", err)
	}
	if _, err := canonicalizeStoredContentV1(continuity.FactProjectRegistered, 1, string(canonical)); err != nil {
		t.Fatalf("canonical stored content rejected: %v", err)
	}

	tests := map[string]string{
		"leading whitespace": " " + string(canonical),
		"unknown field":      `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"Loaf","unknown":"x"}`,
		"missing field":      `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""}}`,
		"duplicate field":    `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"Loaf","label":"Loaf"}`,
		"reordered fields":   `{"label":"Loaf","observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""}}`,
		"trailing token":     string(canonical) + ` {}`,
		"alternate escaping": strings.Replace(string(canonical), "Loaf", `\u004c\u006f\u0061\u0066`, 1),
		"invalid utf8":       string([]byte{'{', 0xff, '}'}),
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := canonicalizeStoredContentV1(continuity.FactProjectRegistered, 1, content)
			problem, ok := err.(*continuity.Problem)
			if !ok || problem.Code != continuity.ProblemCorruptFact {
				t.Fatalf("canonicalize error = %#v (%T), want corrupt-fact", err, err)
			}
		})
	}
}

func TestContinuityCodecErrorsNeverEchoRejectedContent(t *testing.T) {
	t.Parallel()

	marker := "private-marker-that-must-not-escape"
	content := `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"` + marker + `","unknown":true}`
	_, err := canonicalizeStoredContentV1(continuity.FactProjectRegistered, 1, content)
	if err == nil {
		t.Fatal("corrupt content error = nil")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("corrupt content error echoed marker: %v", err)
	}
}

func TestContinuityCodecRejectsUnknownFactKind(t *testing.T) {
	t.Parallel()

	_, err := canonicalizeStoredContentV1(continuity.FactKind("unknown"), 1, `{}`)
	problem, ok := err.(*continuity.Problem)
	if !ok || problem.Code != continuity.ProblemCorruptFact {
		t.Fatalf("canonicalize unknown kind error = %#v (%T), want corrupt-fact", err, err)
	}
}

func TestContinuityCodecRejectsWrongVersionAndSemanticCorruption(t *testing.T) {
	t.Parallel()

	valid := `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"Loaf"}`
	invalid := `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":""}`
	for name, test := range map[string]struct {
		version int
		content string
	}{
		"wrong version":        {version: 2, content: valid},
		"invalid domain value": {version: 1, content: invalid},
	} {
		_, err := canonicalizeStoredContentV1(continuity.FactProjectRegistered, test.version, test.content)
		problem, ok := err.(*continuity.Problem)
		if !ok || problem.Code != continuity.ProblemCorruptFact {
			t.Errorf("%s error = %#v (%T), want corrupt-fact", name, err, err)
		}
	}
}

func TestContinuityCodecHasIndependentGoldenJSONForEveryFactKind(t *testing.T) {
	t.Parallel()

	observation := continuity.Observation{ObservedAtMillis: 1}
	observationJSON := `{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""}`
	ideaReference := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-1"}
	ideaReferenceJSON := `{"kind":"idea","id":"idea-1"}`
	tests := []struct {
		kind   continuity.FactKind
		encode func() (canonicalContentV1, error)
		want   string
	}{
		{
			kind: continuity.FactProjectRegistered,
			encode: func() (canonicalContentV1, error) {
				return encodeProjectRegistrationV1(continuity.ProjectRegistrationPayload{Observation: observation, Label: "x"})
			},
			want: `{"observation":` + observationJSON + `,"label":"x"}`,
		},
		{
			kind: continuity.FactProjectLabelRevised,
			encode: func() (canonicalContentV1, error) {
				return encodeProjectLabelRevisionV1(continuity.ProjectLabelRevisionPayload{Observation: observation, Revises: "fact-1", Label: "x"})
			},
			want: `{"observation":` + observationJSON + `,"revises":"fact-1","label":"x"}`,
		},
		{
			kind: continuity.FactJournalRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeJournalRecordedV1(continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "x"}})
			},
			want: `{"observation":` + observationJSON + `,"content":{"category":"note","scope":"","text":"x"}}`,
		},
		{
			kind: continuity.FactJournalCorrectionRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeJournalCorrectionV1(continuity.JournalCorrectionPayload{Observation: observation, Corrects: "fact-1", Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "x"}})
			},
			want: `{"observation":` + observationJSON + `,"corrects":"fact-1","content":{"category":"note","scope":"","text":"x"}}`,
		},
		{
			kind: continuity.FactWrapRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeWrapRecordedV1(continuity.WrapRecordedPayload{Observation: observation, Focus: &ideaReference, Synthesis: "x"})
			},
			want: `{"observation":` + observationJSON + `,"focus":` + ideaReferenceJSON + `,"scope":"","synthesis":"x"}`,
		},
		{
			kind: continuity.FactSparkCaptured,
			encode: func() (canonicalContentV1, error) {
				return encodeSparkCapturedV1(continuity.SparkCapturedPayload{Observation: observation, Text: "x"})
			},
			want: `{"observation":` + observationJSON + `,"scope":"","text":"x"}`,
		},
		{
			kind: continuity.FactSparkDismissed,
			encode: func() (canonicalContentV1, error) {
				return encodeSparkDismissedV1(continuity.SparkDismissedPayload{Observation: observation, Predecessor: "fact-1"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","reason":""}`,
		},
		{
			kind: continuity.FactSparkPromotedToIdea,
			encode: func() (canonicalContentV1, error) {
				return encodeSparkPromotionV1(continuity.SparkPromotionPayload{Observation: observation, Predecessor: "fact-1", IdeaID: "idea-1"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","idea_id":"idea-1"}`,
		},
		{
			kind: continuity.FactIdeaCreated,
			encode: func() (canonicalContentV1, error) {
				return encodeIdeaCreatedV1(continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "x"}})
			},
			want: `{"observation":` + observationJSON + `,"content":{"label":"x","text":""}}`,
		},
		{
			kind: continuity.FactIdeaRevised,
			encode: func() (canonicalContentV1, error) {
				return encodeIdeaRevisionV1(continuity.IdeaRevisionPayload{Observation: observation, Revises: "fact-1", Content: continuity.IdeaContent{Label: "x"}})
			},
			want: `{"observation":` + observationJSON + `,"revises":"fact-1","content":{"label":"x","text":""}}`,
		},
		{
			kind: continuity.FactIdeaResolved,
			encode: func() (canonicalContentV1, error) {
				return encodeIdeaResolutionV1(continuity.IdeaResolutionPayload{Observation: observation, Predecessor: "fact-1"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","resolution":""}`,
		},
		{
			kind: continuity.FactIdeaArchived,
			encode: func() (canonicalContentV1, error) {
				return encodeIdeaArchiveV1(continuity.IdeaArchivePayload{Observation: observation, Predecessor: "fact-1"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","reason":""}`,
		},
		{
			kind: continuity.FactIdeaPromotedToExternalReference,
			encode: func() (canonicalContentV1, error) {
				return encodeIdeaPromotionV1(continuity.IdeaPromotionPayload{Observation: observation, Predecessor: "fact-1", ReferenceID: "reference-1"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","reference_id":"reference-1"}`,
		},
		{
			kind: continuity.FactDecisionOpened,
			encode: func() (canonicalContentV1, error) {
				return encodeDecisionOpenedV1(continuity.DecisionOpenedPayload{Observation: observation, Question: "x"})
			},
			want: `{"observation":` + observationJSON + `,"scope":"","question":"x","context":""}`,
		},
		{
			kind: continuity.FactDecisionResolved,
			encode: func() (canonicalContentV1, error) {
				return encodeDecisionResolutionV1(continuity.DecisionResolutionPayload{Observation: observation, Predecessor: "fact-1", Resolution: "x"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","resolution":"x","rationale":""}`,
		},
		{
			kind: continuity.FactDecisionSuperseded,
			encode: func() (canonicalContentV1, error) {
				return encodeDecisionSupersessionV1(continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: "fact-1", SuccessorID: "decision-2"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","successor_id":"decision-2","rationale":""}`,
		},
		{
			kind: continuity.FactExplorationStarted,
			encode: func() (canonicalContentV1, error) {
				return encodeExplorationStartedV1(continuity.ExplorationStartedPayload{Observation: observation, Label: "x"})
			},
			want: `{"observation":` + observationJSON + `,"label":"x","purpose":""}`,
		},
		{
			kind: continuity.FactCheckpointRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeCheckpointRecordedV1(continuity.CheckpointRecordedPayload{Observation: observation, ExplorationID: "exploration-1", CurrentFraming: "x", Conclusions: "x", UnresolvedQuestion: "x", NextAction: "x"})
			},
			want: `{"observation":` + observationJSON + `,"exploration_id":"exploration-1","current_framing":"x","conclusions":"x","unresolved_question":"x","next_action":"x","items":[]}`,
		},
		{
			kind: continuity.FactFindingRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeFindingRecordedV1(continuity.FindingRecordedPayload{Observation: observation, Content: continuity.FindingContent{Summary: "x"}})
			},
			want: `{"observation":` + observationJSON + `,"content":{"scope":"","summary":"x","detail":"","recommendation":""}}`,
		},
		{
			kind: continuity.FactFindingCorrected,
			encode: func() (canonicalContentV1, error) {
				return encodeFindingCorrectionV1(continuity.FindingCorrectionPayload{Observation: observation, Corrects: "fact-1", Content: continuity.FindingContent{Summary: "x"}})
			},
			want: `{"observation":` + observationJSON + `,"corrects":"fact-1","content":{"scope":"","summary":"x","detail":"","recommendation":""}}`,
		},
		{
			kind: continuity.FactFindingRetracted,
			encode: func() (canonicalContentV1, error) {
				return encodeFindingRetractionV1(continuity.FindingRetractionPayload{Observation: observation, Predecessor: "fact-1"})
			},
			want: `{"observation":` + observationJSON + `,"predecessor":"fact-1","reason":""}`,
		},
		{
			kind: continuity.FactHandoffRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeHandoffRecordedV1(continuity.HandoffRecordedPayload{Observation: observation, Purpose: "x"})
			},
			want: `{"observation":` + observationJSON + `,"focus":null,"purpose":"x","situation":"","next_actions":"","questions_and_risks":"","suggested_skills":[]}`,
		},
		{
			kind: continuity.FactExternalReferenceRegistered,
			encode: func() (canonicalContentV1, error) {
				return encodeExternalReferenceRegistrationV1(continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:x"})
			},
			want: `{"observation":` + observationJSON + `,"locator":"opaque:x"}`,
		},
		{
			kind: continuity.FactExternalReferenceAttached,
			encode: func() (canonicalContentV1, error) {
				return encodeExternalReferenceAttachmentV1(continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: ideaReference})
			},
			want: `{"observation":` + observationJSON + `,"target":` + ideaReferenceJSON + `,"predecessor":""}`,
		},
		{
			kind: continuity.FactExternalReferenceDetached,
			encode: func() (canonicalContentV1, error) {
				return encodeExternalReferenceDetachmentV1(continuity.ExternalReferenceDetachmentPayload{Observation: observation, Target: ideaReference, Predecessor: "fact-1"})
			},
			want: `{"observation":` + observationJSON + `,"target":` + ideaReferenceJSON + `,"predecessor":"fact-1","reason":""}`,
		},
		{
			kind: continuity.FactVerificationEvidenceRecorded,
			encode: func() (canonicalContentV1, error) {
				return encodeVerificationEvidenceV1(continuity.VerificationEvidencePayload{Observation: observation, Target: ideaReference, Check: "x", Method: "x", Outcome: continuity.VerificationPassed, Detail: "x"})
			},
			want: `{"observation":` + observationJSON + `,"target":` + ideaReferenceJSON + `,"check":"x","method":"x","outcome":"passed","detail":"x"}`,
		},
	}

	if len(tests) != len(continuity.FactCatalog()) {
		t.Fatalf("golden codec cases = %d, want %d", len(tests), len(continuity.FactCatalog()))
	}
	seen := make(map[continuity.FactKind]struct{}, len(tests))
	for _, test := range tests {
		encoded, err := test.encode()
		if err != nil {
			t.Errorf("encode %s: %v", test.kind, err)
			continue
		}
		if string(encoded) != test.want {
			t.Errorf("%s encoded = %s, want %s", test.kind, encoded, test.want)
		}
		if _, duplicate := seen[test.kind]; duplicate {
			t.Errorf("duplicate golden for %s", test.kind)
		}
		seen[test.kind] = struct{}{}
		if _, err := canonicalizeStoredContentV1(test.kind, 1, test.want); err != nil {
			t.Errorf("golden %s rejected by strict decoder: %v", test.kind, err)
		}
	}
	for _, definition := range continuity.FactCatalog() {
		if _, ok := seen[definition.Kind]; !ok {
			t.Errorf("fact kind %s has no independent golden", definition.Kind)
		}
	}
}
