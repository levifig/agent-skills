package sqlite

import "github.com/levifig/loaf/vnext/continuity"

func (payload wireProjectRegistrationV1) validate() error {
	return (continuity.ProjectRegistrationPayload{Observation: payload.Observation.domain(), Label: payload.Label}).Validate()
}

func (payload wireProjectLabelRevisionV1) validate() error {
	return (continuity.ProjectLabelRevisionPayload{Observation: payload.Observation.domain(), Revises: continuity.FactID(payload.Revises), Label: payload.Label}).Validate()
}

func (payload wireJournalRecordedV1) validate() error {
	return (continuity.JournalRecordedPayload{Observation: payload.Observation.domain(), Content: payload.Content.domain()}).Validate()
}

func (payload wireJournalCorrectionV1) validate() error {
	return (continuity.JournalCorrectionPayload{Observation: payload.Observation.domain(), Corrects: continuity.FactID(payload.Corrects), Content: payload.Content.domain()}).Validate()
}

func (payload wireWrapRecordedV1) validate() error {
	return (continuity.WrapRecordedPayload{Observation: payload.Observation.domain(), Focus: payload.Focus.domainOptional(), Scope: payload.Scope, Synthesis: payload.Synthesis}).Validate()
}

func (payload wireSparkCapturedV1) validate() error {
	return (continuity.SparkCapturedPayload{Observation: payload.Observation.domain(), Scope: payload.Scope, Text: payload.Text}).Validate()
}

func (payload wireSparkDismissedV1) validate() error {
	return (continuity.SparkDismissedPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), Reason: payload.Reason}).Validate()
}

func (payload wireSparkPromotionV1) validate() error {
	return (continuity.SparkPromotionPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), IdeaID: continuity.SubjectID(payload.IdeaID)}).Validate()
}

func (payload wireIdeaCreatedV1) validate() error {
	return (continuity.IdeaCreatedPayload{Observation: payload.Observation.domain(), Content: payload.Content.domain()}).Validate()
}

func (payload wireIdeaRevisionV1) validate() error {
	return (continuity.IdeaRevisionPayload{Observation: payload.Observation.domain(), Revises: continuity.FactID(payload.Revises), Content: payload.Content.domain()}).Validate()
}

func (payload wireIdeaResolutionV1) validate() error {
	return (continuity.IdeaResolutionPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), Resolution: payload.Resolution}).Validate()
}

func (payload wireIdeaArchiveV1) validate() error {
	return (continuity.IdeaArchivePayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), Reason: payload.Reason}).Validate()
}

func (payload wireIdeaPromotionV1) validate() error {
	return (continuity.IdeaPromotionPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), ReferenceID: continuity.SubjectID(payload.ReferenceID)}).Validate()
}

func (payload wireDecisionOpenedV1) validate() error {
	return (continuity.DecisionOpenedPayload{Observation: payload.Observation.domain(), Scope: payload.Scope, Question: payload.Question, Context: payload.Context}).Validate()
}

func (payload wireDecisionResolutionV1) validate() error {
	return (continuity.DecisionResolutionPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), Resolution: payload.Resolution, Rationale: payload.Rationale}).Validate()
}

func (payload wireDecisionSupersessionV1) validate() error {
	return (continuity.DecisionSupersessionPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), SuccessorID: continuity.SubjectID(payload.SuccessorID), Rationale: payload.Rationale}).Validate()
}

func (payload wireExplorationStartedV1) validate() error {
	return (continuity.ExplorationStartedPayload{Observation: payload.Observation.domain(), Label: payload.Label, Purpose: payload.Purpose}).Validate()
}

func (payload wireCheckpointRecordedV1) validate() error {
	items := make([]continuity.CheckpointItem, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, continuity.CheckpointItem{Kind: continuity.CheckpointItemKind(item.Kind), Text: item.Text})
	}
	return (continuity.CheckpointRecordedPayload{
		Observation:        payload.Observation.domain(),
		ExplorationID:      continuity.SubjectID(payload.ExplorationID),
		CurrentFraming:     payload.CurrentFraming,
		Conclusions:        payload.Conclusions,
		UnresolvedQuestion: payload.UnresolvedQuestion,
		NextAction:         payload.NextAction,
		Items:              items,
	}).Validate()
}

func (payload wireFindingRecordedV1) validate() error {
	return (continuity.FindingRecordedPayload{Observation: payload.Observation.domain(), Content: payload.Content.domain()}).Validate()
}

func (payload wireFindingCorrectionV1) validate() error {
	return (continuity.FindingCorrectionPayload{Observation: payload.Observation.domain(), Corrects: continuity.FactID(payload.Corrects), Content: payload.Content.domain()}).Validate()
}

func (payload wireFindingRetractionV1) validate() error {
	return (continuity.FindingRetractionPayload{Observation: payload.Observation.domain(), Predecessor: continuity.FactID(payload.Predecessor), Reason: payload.Reason}).Validate()
}

func (payload wireHandoffRecordedV1) validate() error {
	return (continuity.HandoffRecordedPayload{
		Observation:       payload.Observation.domain(),
		Focus:             payload.Focus.domainOptional(),
		Purpose:           payload.Purpose,
		Situation:         payload.Situation,
		NextActions:       payload.NextActions,
		QuestionsAndRisks: payload.QuestionsAndRisks,
		SuggestedSkills:   append([]string(nil), payload.SuggestedSkills...),
	}).Validate()
}

func (payload wireExternalReferenceRegistrationV1) validate() error {
	return (continuity.ExternalReferenceRegistrationPayload{Observation: payload.Observation.domain(), Locator: payload.Locator}).Validate()
}

func (payload wireExternalReferenceAttachmentV1) validate() error {
	return (continuity.ExternalReferenceAttachmentPayload{Observation: payload.Observation.domain(), Target: payload.Target.domain(), Predecessor: continuity.FactID(payload.Predecessor)}).Validate()
}

func (payload wireExternalReferenceDetachmentV1) validate() error {
	return (continuity.ExternalReferenceDetachmentPayload{Observation: payload.Observation.domain(), Target: payload.Target.domain(), Predecessor: continuity.FactID(payload.Predecessor), Reason: payload.Reason}).Validate()
}

func (payload wireVerificationEvidenceV1) validate() error {
	return (continuity.VerificationEvidencePayload{
		Observation: payload.Observation.domain(),
		Target:      payload.Target.domain(),
		Check:       payload.Check,
		Method:      payload.Method,
		Outcome:     continuity.VerificationOutcome(payload.Outcome),
		Detail:      payload.Detail,
	}).Validate()
}

func (observation wireObservationV1) domain() continuity.Observation {
	return continuity.Observation{
		ObservedAtMillis: observation.ObservedAtMillis,
		HarnessSessionID: observation.HarnessSessionID,
		Branch:           observation.Branch,
		Worktree:         observation.Worktree,
	}
}

func (reference wireSubjectRefV1) domain() continuity.SubjectRef {
	return continuity.SubjectRef{Kind: continuity.RecordKind(reference.Kind), ID: continuity.SubjectID(reference.ID)}
}

func (reference *wireSubjectRefV1) domainOptional() *continuity.SubjectRef {
	if reference == nil {
		return nil
	}
	domain := reference.domainValue()
	return &domain
}

func (reference wireSubjectRefV1) domainValue() continuity.SubjectRef {
	return continuity.SubjectRef{Kind: continuity.RecordKind(reference.Kind), ID: continuity.SubjectID(reference.ID)}
}

func (content wireJournalContentV1) domain() continuity.JournalContent {
	return continuity.JournalContent{Category: continuity.JournalCategory(content.Category), Scope: content.Scope, Text: content.Text}
}

func (content wireIdeaContentV1) domain() continuity.IdeaContent {
	return continuity.IdeaContent{Label: content.Label, Text: content.Text}
}

func (content wireFindingContentV1) domain() continuity.FindingContent {
	return continuity.FindingContent{Scope: content.Scope, Summary: content.Summary, Detail: content.Detail, Recommendation: content.Recommendation}
}
