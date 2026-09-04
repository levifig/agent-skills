package sqlite

type wireObservationV1 struct {
	ObservedAtMillis int64  `json:"observed_at_millis"`
	HarnessSessionID string `json:"harness_session_id"`
	Branch           string `json:"branch"`
	Worktree         string `json:"worktree"`
}

type wireSubjectRefV1 struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type wireJournalContentV1 struct {
	Category string `json:"category"`
	Scope    string `json:"scope"`
	Text     string `json:"text"`
}

type wireIdeaContentV1 struct {
	Label string `json:"label"`
	Text  string `json:"text"`
}

type wireFindingContentV1 struct {
	Scope          string `json:"scope"`
	Summary        string `json:"summary"`
	Detail         string `json:"detail"`
	Recommendation string `json:"recommendation"`
}

type wireCheckpointItemV1 struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type wireProjectRegistrationV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Label       string            `json:"label"`
}

type wireProjectLabelRevisionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Revises     string            `json:"revises"`
	Label       string            `json:"label"`
}

type wireJournalRecordedV1 struct {
	Observation wireObservationV1    `json:"observation"`
	Content     wireJournalContentV1 `json:"content"`
}

type wireJournalCorrectionV1 struct {
	Observation wireObservationV1    `json:"observation"`
	Corrects    string               `json:"corrects"`
	Content     wireJournalContentV1 `json:"content"`
}

type wireWrapRecordedV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Focus       *wireSubjectRefV1 `json:"focus"`
	Scope       string            `json:"scope"`
	Synthesis   string            `json:"synthesis"`
}

type wireSparkCapturedV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Scope       string            `json:"scope"`
	Text        string            `json:"text"`
}

type wireSparkDismissedV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	Reason      string            `json:"reason"`
}

type wireSparkPromotionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	IdeaID      string            `json:"idea_id"`
}

type wireIdeaCreatedV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Content     wireIdeaContentV1 `json:"content"`
}

type wireIdeaRevisionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Revises     string            `json:"revises"`
	Content     wireIdeaContentV1 `json:"content"`
}

type wireIdeaResolutionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	Resolution  string            `json:"resolution"`
}

type wireIdeaArchiveV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	Reason      string            `json:"reason"`
}

type wireIdeaPromotionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	ReferenceID string            `json:"reference_id"`
}

type wireDecisionOpenedV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Scope       string            `json:"scope"`
	Question    string            `json:"question"`
	Context     string            `json:"context"`
}

type wireDecisionResolutionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	Resolution  string            `json:"resolution"`
	Rationale   string            `json:"rationale"`
}

type wireDecisionSupersessionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	SuccessorID string            `json:"successor_id"`
	Rationale   string            `json:"rationale"`
}

type wireExplorationStartedV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Label       string            `json:"label"`
	Purpose     string            `json:"purpose"`
}

type wireCheckpointRecordedV1 struct {
	Observation        wireObservationV1      `json:"observation"`
	ExplorationID      string                 `json:"exploration_id"`
	CurrentFraming     string                 `json:"current_framing"`
	Conclusions        string                 `json:"conclusions"`
	UnresolvedQuestion string                 `json:"unresolved_question"`
	NextAction         string                 `json:"next_action"`
	Items              []wireCheckpointItemV1 `json:"items"`
}

type wireFindingRecordedV1 struct {
	Observation wireObservationV1    `json:"observation"`
	Content     wireFindingContentV1 `json:"content"`
}

type wireFindingCorrectionV1 struct {
	Observation wireObservationV1    `json:"observation"`
	Corrects    string               `json:"corrects"`
	Content     wireFindingContentV1 `json:"content"`
}

type wireFindingRetractionV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Predecessor string            `json:"predecessor"`
	Reason      string            `json:"reason"`
}

type wireHandoffRecordedV1 struct {
	Observation       wireObservationV1 `json:"observation"`
	Focus             *wireSubjectRefV1 `json:"focus"`
	Purpose           string            `json:"purpose"`
	Situation         string            `json:"situation"`
	NextActions       string            `json:"next_actions"`
	QuestionsAndRisks string            `json:"questions_and_risks"`
	SuggestedSkills   []string          `json:"suggested_skills"`
}

type wireExternalReferenceRegistrationV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Locator     string            `json:"locator"`
}

type wireExternalReferenceAttachmentV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Target      wireSubjectRefV1  `json:"target"`
	Predecessor string            `json:"predecessor"`
}

type wireExternalReferenceDetachmentV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Target      wireSubjectRefV1  `json:"target"`
	Predecessor string            `json:"predecessor"`
	Reason      string            `json:"reason"`
}

type wireVerificationEvidenceV1 struct {
	Observation wireObservationV1 `json:"observation"`
	Target      wireSubjectRefV1  `json:"target"`
	Check       string            `json:"check"`
	Method      string            `json:"method"`
	Outcome     string            `json:"outcome"`
	Detail      string            `json:"detail"`
}

type wireValueV1 interface {
	wireProjectRegistrationV1 |
		wireProjectLabelRevisionV1 |
		wireJournalRecordedV1 |
		wireJournalCorrectionV1 |
		wireWrapRecordedV1 |
		wireSparkCapturedV1 |
		wireSparkDismissedV1 |
		wireSparkPromotionV1 |
		wireIdeaCreatedV1 |
		wireIdeaRevisionV1 |
		wireIdeaResolutionV1 |
		wireIdeaArchiveV1 |
		wireIdeaPromotionV1 |
		wireDecisionOpenedV1 |
		wireDecisionResolutionV1 |
		wireDecisionSupersessionV1 |
		wireExplorationStartedV1 |
		wireCheckpointRecordedV1 |
		wireFindingRecordedV1 |
		wireFindingCorrectionV1 |
		wireFindingRetractionV1 |
		wireHandoffRecordedV1 |
		wireExternalReferenceRegistrationV1 |
		wireExternalReferenceAttachmentV1 |
		wireExternalReferenceDetachmentV1 |
		wireVerificationEvidenceV1
	validate() error
}
