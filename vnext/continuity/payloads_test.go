package continuity

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConcreteContinuityPayloadsCoverEveryFactKind(t *testing.T) {
	t.Parallel()

	observation := validObservation()
	tests := []struct {
		kind     FactKind
		validate func() error
	}{
		{FactProjectRegistered, func() error { return (ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}).Validate() }},
		{FactProjectLabelRevised, func() error {
			return (ProjectLabelRevisionPayload{Observation: observation, Revises: "fact-project", Label: "Loaf vNext"}).Validate()
		}},
		{FactJournalRecorded, func() error {
			return (JournalRecordedPayload{Observation: observation, Content: validJournalContent()}).Validate()
		}},
		{FactJournalCorrectionRecorded, func() error {
			return (JournalCorrectionPayload{Observation: observation, Corrects: "fact-journal", Content: validJournalContent()}).Validate()
		}},
		{FactWrapRecorded, func() error {
			return (WrapRecordedPayload{Observation: observation, Focus: validFocus(), Scope: "continuity", Synthesis: "The schema is complete."}).Validate()
		}},
		{FactSparkCaptured, func() error {
			return (SparkCapturedPayload{Observation: observation, Scope: "sync", Text: "Converge private facts."}).Validate()
		}},
		{FactSparkDismissed, func() error {
			return (SparkDismissedPayload{Observation: observation, Predecessor: "fact-spark", Reason: "Included in the chosen design."}).Validate()
		}},
		{FactSparkPromotedToIdea, func() error {
			return (SparkPromotionPayload{Observation: observation, Predecessor: "fact-spark", IdeaID: "idea-1"}).Validate()
		}},
		{FactIdeaCreated, func() error {
			return (IdeaCreatedPayload{Observation: observation, Content: validIdeaContent()}).Validate()
		}},
		{FactIdeaRevised, func() error {
			return (IdeaRevisionPayload{Observation: observation, Revises: "fact-idea", Content: validIdeaContent()}).Validate()
		}},
		{FactIdeaResolved, func() error {
			return (IdeaResolutionPayload{Observation: observation, Predecessor: "fact-idea", Resolution: "Implement the fact log."}).Validate()
		}},
		{FactIdeaArchived, func() error {
			return (IdeaArchivePayload{Observation: observation, Predecessor: "fact-idea", Reason: "Retained for retrieval."}).Validate()
		}},
		{FactIdeaPromotedToExternalReference, func() error {
			return (IdeaPromotionPayload{Observation: observation, Predecessor: "fact-idea", ReferenceID: "reference-1"}).Validate()
		}},
		{FactDecisionOpened, func() error {
			return (DecisionOpenedPayload{Observation: observation, Scope: "continuity", Question: "Which local store?", Context: "Writes must be transactional."}).Validate()
		}},
		{FactDecisionResolved, func() error {
			return (DecisionResolutionPayload{Observation: observation, Predecessor: "fact-decision", Resolution: "SQLite", Rationale: "It gives durable indexed transactions."}).Validate()
		}},
		{FactDecisionSuperseded, func() error {
			return (DecisionSupersessionPayload{Observation: observation, Predecessor: "fact-decision", SuccessorID: "decision-2", Rationale: "The trust boundary became explicit."}).Validate()
		}},
		{FactExplorationStarted, func() error {
			return (ExplorationStartedPayload{Observation: observation, Label: "Continuity model", Purpose: "Resolve persistence semantics."}).Validate()
		}},
		{FactCheckpointRecorded, func() error { return validCheckpointPayload(observation).Validate() }},
		{FactFindingRecorded, func() error {
			return (FindingRecordedPayload{Observation: observation, Content: validFindingContent()}).Validate()
		}},
		{FactFindingCorrected, func() error {
			return (FindingCorrectionPayload{Observation: observation, Corrects: "fact-finding", Content: validFindingContent()}).Validate()
		}},
		{FactFindingRetracted, func() error {
			return (FindingRetractionPayload{Observation: observation, Predecessor: "fact-finding", Reason: "Recorded as a trust limit instead."}).Validate()
		}},
		{FactHandoffRecorded, func() error { return validHandoffPayload(observation).Validate() }},
		{FactScratchpadOpened, func() error {
			return (ScratchpadOpenedPayload{Observation: observation, Focus: validFocus(), Label: "LOAF-96 review"}).Validate()
		}},
		{FactScratchpadParticipantIntroduced, func() error {
			return (ScratchpadParticipantPayload{Observation: observation, ParticipantID: "participant-1", Name: "storage-review", Focus: validFocus()}).Validate()
		}},
		{FactScratchpadMessageRecorded, func() error {
			return (ScratchpadMessagePayload{Observation: observation, ParticipantID: "participant-1", Text: "Reviewing append ordering."}).Validate()
		}},
		{FactScratchpadClaimRecorded, func() error {
			return (ScratchpadClaimPayload{Observation: observation, ClaimID: "claim-1", ParticipantID: "participant-1", Resource: "vnext/continuity/sqlite", ExpiresAtMillis: observation.ObservedAtMillis + 1}).Validate()
		}},
		{FactScratchpadClaimReleased, func() error {
			return (ScratchpadClaimReleasePayload{Observation: observation, ClaimID: "claim-1", ReleasedBy: "participant-1", Reason: "Review complete."}).Validate()
		}},
		{FactScratchpadClosed, func() error {
			return (ScratchpadClosePayload{Observation: observation, ClosedBy: "participant-1", Reason: "Review complete."}).Validate()
		}},
		{FactExternalReferenceRegistered, func() error {
			return (ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:LOAF-96"}).Validate()
		}},
		{FactExternalReferenceAttached, func() error {
			return (ExternalReferenceAttachmentPayload{Observation: observation, Target: *validFocus(), Predecessor: ""}).Validate()
		}},
		{FactExternalReferenceDetached, func() error {
			return (ExternalReferenceDetachmentPayload{Observation: observation, Target: *validFocus(), Predecessor: "fact-attachment", Reason: "No longer relevant."}).Validate()
		}},
		{FactVerificationEvidenceRecorded, func() error {
			return (VerificationEvidencePayload{Observation: observation, Target: *validFocus(), Check: "go test", Method: "native", Outcome: VerificationPassed, Detail: "Focused tests pass."}).Validate()
		}},
	}

	if len(tests) != len(FactCatalog()) {
		t.Fatalf("typed payload count = %d, fact catalog count = %d", len(tests), len(FactCatalog()))
	}
	seen := make(map[FactKind]struct{}, len(tests))
	for _, test := range tests {
		if _, duplicate := seen[test.kind]; duplicate {
			t.Errorf("duplicate payload case for %q", test.kind)
		}
		seen[test.kind] = struct{}{}
		if err := test.validate(); err != nil {
			t.Errorf("valid %q payload rejected: %v", test.kind, err)
		}
	}
	for _, definition := range FactCatalog() {
		if _, ok := seen[definition.Kind]; !ok {
			t.Errorf("fact kind %q has no concrete payload", definition.Kind)
		}
	}
}

func TestContinuityPayloadValidationRejectsNonCanonicalAndUnboundedValues(t *testing.T) {
	t.Parallel()

	observation := validObservation()
	invalidUTF8 := string([]byte{0xff})
	tests := []struct {
		name     string
		validate func() error
		field    string
	}{
		{name: "unknown category", validate: func() error { return (JournalContent{Category: "unknown", Text: "entry"}).Validate() }, field: "content.category"},
		{name: "outer label whitespace", validate: func() error { return (IdeaContent{Label: " idea ", Text: "body"}).Validate() }, field: "content.label"},
		{name: "invalid utf8", validate: func() error { return (SparkCapturedPayload{Observation: observation, Text: invalidUTF8}).Validate() }, field: "text"},
		{name: "nul", validate: func() error {
			return (SparkCapturedPayload{Observation: observation, Text: "private\x00text"}).Validate()
		}, field: "text"},
		{name: "carriage return", validate: func() error { return (SparkCapturedPayload{Observation: observation, Text: "one\r\ntwo"}).Validate() }, field: "text"},
		{name: "oversized prose", validate: func() error {
			return (WrapRecordedPayload{Observation: observation, Synthesis: strings.Repeat("x", 65537)}).Validate()
		}, field: "synthesis"},
		{name: "oversized utf8 bytes", validate: func() error { return (IdeaContent{Label: strings.Repeat("é", 257), Text: "body"}).Validate() }, field: "content.label"},
		{name: "too many checkpoint items", validate: func() error {
			payload := validCheckpointPayload(observation)
			payload.Items = make([]CheckpointItem, 65)
			return payload.Validate()
		}, field: "items"},
		{name: "unknown checkpoint item", validate: func() error {
			payload := validCheckpointPayload(observation)
			payload.Items[0].Kind = "unknown"
			return payload.Validate()
		}, field: "items[0].kind"},
		{name: "derived focus", validate: func() error {
			return (WrapRecordedPayload{Observation: observation, Focus: &SubjectRef{Kind: RecordDerivedContext, ID: "context-1"}, Synthesis: "checkpoint"}).Validate()
		}, field: "focus.kind"},
		{name: "scratchpad durable target", validate: func() error {
			return (VerificationEvidencePayload{Observation: observation, Target: SubjectRef{Kind: RecordScratchpad, ID: "scratchpad-1"}, Check: "go test", Method: "native", Outcome: VerificationPassed, Detail: "pass"}).Validate()
		}, field: "target.kind"},
		{name: "expired claim", validate: func() error {
			return (ScratchpadClaimPayload{Observation: observation, ClaimID: "claim-1", ParticipantID: "participant-1", Resource: "file", ExpiresAtMillis: observation.ObservedAtMillis}).Validate()
		}, field: "expires_at_millis"},
		{name: "unknown verification outcome", validate: func() error {
			return (VerificationEvidencePayload{Observation: observation, Target: *validFocus(), Check: "go test", Method: "native", Outcome: "unknown", Detail: "result"}).Validate()
		}, field: "outcome"},
		{name: "external URL", validate: func() error {
			return (ExternalReferenceRegistrationPayload{Observation: observation, Locator: "https://example.invalid/private"}).Validate()
		}, field: "locator"},
		{name: "external locator whitespace", validate: func() error {
			return (ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque ref"}).Validate()
		}, field: "locator"},
		{name: "external reference graph", validate: func() error {
			return (ExternalReferenceAttachmentPayload{Observation: observation, Target: SubjectRef{Kind: RecordExternalReference, ID: "reference-2"}}).Validate()
		}, field: "target.kind"},
		{name: "negative observation", validate: func() error { observation.ObservedAtMillis = -1; return observation.Validate() }, field: "observation.observed_at_millis"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.validate()
			problem, ok := err.(*Problem)
			if !ok {
				t.Fatalf("error type = %T, want *Problem", err)
			}
			if problem.Code != ProblemInvalid || problem.Field != test.field {
				t.Fatalf("problem = %#v, want invalid field %q", problem, test.field)
			}
		})
	}
}

func TestContinuityPayloadValidationAcceptsLosslessLegacyMinimums(t *testing.T) {
	t.Parallel()

	observation := validObservation()
	tests := []struct {
		name     string
		validate func() error
	}{
		{name: "decision journal category", validate: func() error {
			return (JournalContent{Category: JournalDecision, Scope: "sqlite", Text: "Use typed facts."}).Validate()
		}},
		{name: "idea label only", validate: func() error {
			return (IdeaCreatedPayload{Observation: observation, Content: IdeaContent{Label: "Legacy idea"}}).Validate()
		}},
		{name: "idea resolution without prose", validate: func() error {
			return (IdeaResolutionPayload{Observation: observation, Predecessor: "fact-idea"}).Validate()
		}},
		{name: "idea archive without reason", validate: func() error {
			return (IdeaArchivePayload{Observation: observation, Predecessor: "fact-idea"}).Validate()
		}},
		{name: "spark dismissal without reason", validate: func() error {
			return (SparkDismissedPayload{Observation: observation, Predecessor: "fact-spark"}).Validate()
		}},
		{name: "decision without context", validate: func() error {
			return (DecisionOpenedPayload{Observation: observation, Question: "Which store?"}).Validate()
		}},
		{name: "decision resolution without rationale", validate: func() error {
			return (DecisionResolutionPayload{Observation: observation, Predecessor: "fact-decision", Resolution: "SQLite"}).Validate()
		}},
		{name: "exploration label only", validate: func() error {
			return (ExplorationStartedPayload{Observation: observation, Label: "Legacy exploration"}).Validate()
		}},
		{name: "finding summary only", validate: func() error {
			return (FindingRecordedPayload{Observation: observation, Content: FindingContent{Summary: "Legacy finding"}}).Validate()
		}},
		{name: "handoff purpose only", validate: func() error {
			return (HandoffRecordedPayload{Observation: observation, Purpose: "Continue from the archived handoff."}).Validate()
		}},
		{name: "scratchpad close without reason", validate: func() error { return (ScratchpadClosePayload{Observation: observation}).Validate() }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.validate(); err != nil {
				t.Fatalf("legacy-minimum payload rejected: %v", err)
			}
		})
	}
}

func TestJournalCategoriesPreserveTimelineVocabularyOnly(t *testing.T) {
	t.Parallel()

	categories := []JournalCategory{
		JournalNote,
		JournalSkill,
		JournalCommit,
		JournalDecision,
		JournalDiscover,
		JournalBlock,
		JournalUnblock,
		JournalSpark,
		JournalTodo,
		JournalFinding,
		JournalWrap,
	}
	for _, category := range categories {
		content := JournalContent{Category: category, Text: "timeline label"}
		if err := content.Validate(); err != nil {
			t.Errorf("journal category %q rejected: %v", category, err)
		}
	}
}

func TestContinuityOpaqueIdentitiesAreMintedAndValidated(t *testing.T) {
	t.Parallel()

	factID, err := NewFactID()
	if err != nil {
		t.Fatalf("NewFactID(): %v", err)
	}
	secondFactID, err := NewFactID()
	if err != nil {
		t.Fatalf("NewFactID() second call: %v", err)
	}
	projectID, err := NewProjectID()
	if err != nil {
		t.Fatalf("NewProjectID(): %v", err)
	}
	subjectID, err := NewSubjectID()
	if err != nil {
		t.Fatalf("NewSubjectID(): %v", err)
	}
	environmentID, err := NewEnvironmentID()
	if err != nil {
		t.Fatalf("NewEnvironmentID(): %v", err)
	}

	if factID == secondFactID {
		t.Fatal("two fact identities are equal")
	}
	for name, validate := range map[string]func() error{
		"fact":        factID.Validate,
		"project":     projectID.Validate,
		"subject":     subjectID.Validate,
		"environment": environmentID.Validate,
	} {
		if err := validate(); err != nil {
			t.Errorf("minted %s identity rejected: %v", name, err)
		}
	}

	for name, validate := range map[string]func() error{
		"empty fact":        FactID("").Validate,
		"oversized project": ProjectID(strings.Repeat("a", 129)).Validate,
		"non-ascii subject": SubjectID("subject-λ").Validate,
		"slash environment": EnvironmentID("environment/local").Validate,
	} {
		if err := validate(); err == nil {
			t.Errorf("%s validation error = nil, want rejection", name)
		}
	}
}

func TestContinuityValidationErrorsDoNotEchoRejectedContent(t *testing.T) {
	t.Parallel()

	marker := "do-not-echo-this-private-value"
	err := (SparkCapturedPayload{Observation: validObservation(), Text: marker + "\x00"}).Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, want rejection")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("validation error echoed rejected content: %v", err)
	}
}

func TestContinuityProblemCodesDistinguishReplayAndAggregateConflicts(t *testing.T) {
	t.Parallel()

	if ProblemFactConflict == ProblemPreconditionFailed {
		t.Fatal("retained fact identity conflicts must differ from aggregate precondition failures")
	}
	if ProblemReferenceMismatch == ProblemPreconditionFailed {
		t.Fatal("reference mismatches must differ from aggregate precondition failures")
	}
}

func TestContinuityPayloadCapsAreInclusiveUTF8ByteLimits(t *testing.T) {
	t.Parallel()

	for name, content := range map[string]IdeaContent{
		"ascii boundary": {Label: strings.Repeat("a", 512), Text: "body"},
		"utf8 boundary":  {Label: strings.Repeat("é", 256), Text: "body"},
	} {
		if err := content.Validate(); err != nil {
			t.Errorf("%s rejected: %v", name, err)
		}
	}

	checkpoint := validCheckpointPayload(validObservation())
	checkpoint.Items = make([]CheckpointItem, 64)
	for index := range checkpoint.Items {
		checkpoint.Items[index] = CheckpointItem{Kind: CheckpointEvidence, Text: "evidence"}
	}
	if err := checkpoint.Validate(); err != nil {
		t.Fatalf("64 checkpoint items rejected: %v", err)
	}
}

func validObservation() Observation {
	return Observation{
		ObservedAtMillis: 1787994000000,
		HarnessSessionID: "conversation-1",
		Branch:           "issue/loaf-93",
		Worktree:         "/workspace/loaf",
	}
}

func validFocus() *SubjectRef {
	return &SubjectRef{Kind: RecordIdea, ID: "idea-1"}
}

func validJournalContent() JournalContent {
	return JournalContent{Category: JournalDiscover, Scope: "continuity", Text: "Typed facts stay canonical."}
}

func validIdeaContent() IdeaContent {
	return IdeaContent{Label: "Private sync", Text: "Converge encrypted typed facts."}
}

func validFindingContent() FindingContent {
	return FindingContent{Scope: "sqlite", Summary: "Path race", Detail: "Path checks retain a same-user race.", Recommendation: "Keep the single-operator trust boundary explicit."}
}

func validCheckpointPayload(observation Observation) CheckpointRecordedPayload {
	return CheckpointRecordedPayload{
		Observation:        observation,
		ExplorationID:      "exploration-1",
		CurrentFraming:     "Facts are canonical.",
		Conclusions:        "SQLite fits.",
		UnresolvedQuestion: "Which sync envelope is smallest?",
		NextAction:         "Implement local append.",
		Items:              []CheckpointItem{{Kind: CheckpointEvidence, Text: "Concurrent opens pass."}},
	}
}

func validHandoffPayload(observation Observation) HandoffRecordedPayload {
	return HandoffRecordedPayload{
		Observation:       observation,
		Focus:             validFocus(),
		Purpose:           "Continue LOAF-96.",
		Situation:         "The schema is committed.",
		NextActions:       "Implement typed append.",
		QuestionsAndRisks: "Keep replay separate from local admissibility.",
		SuggestedSkills:   []string{"go-development", "database-design"},
	}
}

func TestInvalidUTF8FixtureIsActuallyInvalid(t *testing.T) {
	t.Parallel()
	if utf8.ValidString(string([]byte{0xff})) {
		t.Fatal("invalid UTF-8 fixture unexpectedly valid")
	}
}
