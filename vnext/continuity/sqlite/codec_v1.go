package sqlite

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
)

const maximumContentBytes = 1_048_576

type canonicalContentV1 string

func encodeWireV1[T wireValueV1](payload T) (canonicalContentV1, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return "", &continuity.Problem{Code: continuity.ProblemInvalid, Field: "content", Detail: "cannot encode canonical content"}
	}
	encoded := buffer.Bytes()
	if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
		return "", &continuity.Problem{Code: continuity.ProblemInvalid, Field: "content", Detail: "cannot encode canonical content"}
	}
	encoded = encoded[:len(encoded)-1]
	if len(encoded) > maximumContentBytes {
		return "", &continuity.Problem{Code: continuity.ProblemInvalid, Field: "content", Detail: "canonical content exceeds 1048576 bytes"}
	}
	return canonicalContentV1(string(encoded)), nil
}

func decodeWireV1[T wireValueV1](content string) (T, error) {
	var payload T
	if len(content) < 2 || len(content) > maximumContentBytes {
		return payload, corruptContentV1()
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, corruptContentV1()
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return payload, corruptContentV1()
	}
	if err := payload.validate(); err != nil {
		return payload, corruptContentV1()
	}
	return payload, nil
}

func decodeStoredWireV1[T wireValueV1](content string) (T, error) {
	payload, err := decodeWireV1[T](content)
	if err != nil {
		return payload, err
	}
	canonical, err := encodeWireV1(payload)
	if err != nil || string(canonical) != content {
		return payload, corruptContentV1()
	}
	return payload, nil
}

func canonicalizeStoredContentV1(kind continuity.FactKind, payloadVersion int, content string) (canonicalContentV1, error) {
	if payloadVersion != 1 {
		return "", corruptContentV1()
	}
	switch kind {
	case continuity.FactProjectRegistered:
		return canonicalizeWireV1[wireProjectRegistrationV1](content)
	case continuity.FactProjectLabelRevised:
		return canonicalizeWireV1[wireProjectLabelRevisionV1](content)
	case continuity.FactJournalRecorded:
		return canonicalizeWireV1[wireJournalRecordedV1](content)
	case continuity.FactJournalCorrectionRecorded:
		return canonicalizeWireV1[wireJournalCorrectionV1](content)
	case continuity.FactWrapRecorded:
		return canonicalizeWireV1[wireWrapRecordedV1](content)
	case continuity.FactSparkCaptured:
		return canonicalizeWireV1[wireSparkCapturedV1](content)
	case continuity.FactSparkDismissed:
		return canonicalizeWireV1[wireSparkDismissedV1](content)
	case continuity.FactSparkPromotedToIdea:
		return canonicalizeWireV1[wireSparkPromotionV1](content)
	case continuity.FactIdeaCreated:
		return canonicalizeWireV1[wireIdeaCreatedV1](content)
	case continuity.FactIdeaRevised:
		return canonicalizeWireV1[wireIdeaRevisionV1](content)
	case continuity.FactIdeaResolved:
		return canonicalizeWireV1[wireIdeaResolutionV1](content)
	case continuity.FactIdeaArchived:
		return canonicalizeWireV1[wireIdeaArchiveV1](content)
	case continuity.FactIdeaPromotedToExternalReference:
		return canonicalizeWireV1[wireIdeaPromotionV1](content)
	case continuity.FactDecisionOpened:
		return canonicalizeWireV1[wireDecisionOpenedV1](content)
	case continuity.FactDecisionResolved:
		return canonicalizeWireV1[wireDecisionResolutionV1](content)
	case continuity.FactDecisionSuperseded:
		return canonicalizeWireV1[wireDecisionSupersessionV1](content)
	case continuity.FactExplorationStarted:
		return canonicalizeWireV1[wireExplorationStartedV1](content)
	case continuity.FactCheckpointRecorded:
		payload, err := decodeWireV1[wireCheckpointRecordedV1](content)
		if err != nil {
			return "", err
		}
		payload.Items = normalizeCheckpointItemsV1(payload.Items)
		return requireCanonicalV1(payload, content)
	case continuity.FactFindingRecorded:
		return canonicalizeWireV1[wireFindingRecordedV1](content)
	case continuity.FactFindingCorrected:
		return canonicalizeWireV1[wireFindingCorrectionV1](content)
	case continuity.FactFindingRetracted:
		return canonicalizeWireV1[wireFindingRetractionV1](content)
	case continuity.FactHandoffRecorded:
		payload, err := decodeWireV1[wireHandoffRecordedV1](content)
		if err != nil {
			return "", err
		}
		payload.SuggestedSkills = normalizeStringsV1(payload.SuggestedSkills)
		return requireCanonicalV1(payload, content)
	case continuity.FactExternalReferenceRegistered:
		return canonicalizeWireV1[wireExternalReferenceRegistrationV1](content)
	case continuity.FactExternalReferenceAttached:
		return canonicalizeWireV1[wireExternalReferenceAttachmentV1](content)
	case continuity.FactExternalReferenceDetached:
		return canonicalizeWireV1[wireExternalReferenceDetachmentV1](content)
	case continuity.FactVerificationEvidenceRecorded:
		return canonicalizeWireV1[wireVerificationEvidenceV1](content)
	default:
		return "", corruptContentV1()
	}
}

func canonicalizeWireV1[T wireValueV1](content string) (canonicalContentV1, error) {
	payload, err := decodeStoredWireV1[T](content)
	if err != nil {
		return "", err
	}
	return encodeWireV1(payload)
}

func requireCanonicalV1[T wireValueV1](payload T, content string) (canonicalContentV1, error) {
	canonical, err := encodeWireV1(payload)
	if err != nil || string(canonical) != content {
		return "", corruptContentV1()
	}
	return canonical, nil
}

func corruptContentV1() error {
	return &continuity.Problem{Code: continuity.ProblemCorruptFact, Field: "content", Detail: "stored fact content is not canonical v1 data"}
}

func encodeProjectRegistrationV1(payload continuity.ProjectRegistrationPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireProjectRegistrationV1{Observation: toWireObservationV1(payload.Observation), Label: payload.Label})
}

func encodeProjectLabelRevisionV1(payload continuity.ProjectLabelRevisionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireProjectLabelRevisionV1{Observation: toWireObservationV1(payload.Observation), Revises: string(payload.Revises), Label: payload.Label})
}

func encodeJournalRecordedV1(payload continuity.JournalRecordedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireJournalRecordedV1{Observation: toWireObservationV1(payload.Observation), Content: toWireJournalContentV1(payload.Content)})
}

func encodeJournalCorrectionV1(payload continuity.JournalCorrectionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireJournalCorrectionV1{Observation: toWireObservationV1(payload.Observation), Corrects: string(payload.Corrects), Content: toWireJournalContentV1(payload.Content)})
}

func encodeWrapRecordedV1(payload continuity.WrapRecordedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireWrapRecordedV1{Observation: toWireObservationV1(payload.Observation), Focus: toWireOptionalSubjectRefV1(payload.Focus), Scope: payload.Scope, Synthesis: payload.Synthesis})
}

func encodeSparkCapturedV1(payload continuity.SparkCapturedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireSparkCapturedV1{Observation: toWireObservationV1(payload.Observation), Scope: payload.Scope, Text: payload.Text})
}

func encodeSparkDismissedV1(payload continuity.SparkDismissedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireSparkDismissedV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), Reason: payload.Reason})
}

func encodeSparkPromotionV1(payload continuity.SparkPromotionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireSparkPromotionV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), IdeaID: string(payload.IdeaID)})
}

func encodeIdeaCreatedV1(payload continuity.IdeaCreatedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireIdeaCreatedV1{Observation: toWireObservationV1(payload.Observation), Content: toWireIdeaContentV1(payload.Content)})
}

func encodeIdeaRevisionV1(payload continuity.IdeaRevisionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireIdeaRevisionV1{Observation: toWireObservationV1(payload.Observation), Revises: string(payload.Revises), Content: toWireIdeaContentV1(payload.Content)})
}

func encodeIdeaResolutionV1(payload continuity.IdeaResolutionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireIdeaResolutionV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), Resolution: payload.Resolution})
}

func encodeIdeaArchiveV1(payload continuity.IdeaArchivePayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireIdeaArchiveV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), Reason: payload.Reason})
}

func encodeIdeaPromotionV1(payload continuity.IdeaPromotionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireIdeaPromotionV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), ReferenceID: string(payload.ReferenceID)})
}

func encodeDecisionOpenedV1(payload continuity.DecisionOpenedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireDecisionOpenedV1{Observation: toWireObservationV1(payload.Observation), Scope: payload.Scope, Question: payload.Question, Context: payload.Context})
}

func encodeDecisionResolutionV1(payload continuity.DecisionResolutionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireDecisionResolutionV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), Resolution: payload.Resolution, Rationale: payload.Rationale})
}

func encodeDecisionSupersessionV1(payload continuity.DecisionSupersessionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireDecisionSupersessionV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), SuccessorID: string(payload.SuccessorID), Rationale: payload.Rationale})
}

func encodeExplorationStartedV1(payload continuity.ExplorationStartedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireExplorationStartedV1{Observation: toWireObservationV1(payload.Observation), Label: payload.Label, Purpose: payload.Purpose})
}

func encodeCheckpointRecordedV1(payload continuity.CheckpointRecordedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	items := make([]wireCheckpointItemV1, 0, len(payload.Items))
	for _, item := range payload.Items {
		items = append(items, wireCheckpointItemV1{Kind: string(item.Kind), Text: item.Text})
	}
	return encodeWireV1(wireCheckpointRecordedV1{
		Observation:        toWireObservationV1(payload.Observation),
		ExplorationID:      string(payload.ExplorationID),
		CurrentFraming:     payload.CurrentFraming,
		Conclusions:        payload.Conclusions,
		UnresolvedQuestion: payload.UnresolvedQuestion,
		NextAction:         payload.NextAction,
		Items:              items,
	})
}

func encodeFindingRecordedV1(payload continuity.FindingRecordedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireFindingRecordedV1{Observation: toWireObservationV1(payload.Observation), Content: toWireFindingContentV1(payload.Content)})
}

func encodeFindingCorrectionV1(payload continuity.FindingCorrectionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireFindingCorrectionV1{Observation: toWireObservationV1(payload.Observation), Corrects: string(payload.Corrects), Content: toWireFindingContentV1(payload.Content)})
}

func encodeFindingRetractionV1(payload continuity.FindingRetractionPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireFindingRetractionV1{Observation: toWireObservationV1(payload.Observation), Predecessor: string(payload.Predecessor), Reason: payload.Reason})
}

func encodeHandoffRecordedV1(payload continuity.HandoffRecordedPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireHandoffRecordedV1{
		Observation:       toWireObservationV1(payload.Observation),
		Focus:             toWireOptionalSubjectRefV1(payload.Focus),
		Purpose:           payload.Purpose,
		Situation:         payload.Situation,
		NextActions:       payload.NextActions,
		QuestionsAndRisks: payload.QuestionsAndRisks,
		SuggestedSkills:   normalizeStringsV1(payload.SuggestedSkills),
	})
}

func encodeExternalReferenceRegistrationV1(payload continuity.ExternalReferenceRegistrationPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireExternalReferenceRegistrationV1{Observation: toWireObservationV1(payload.Observation), Locator: payload.Locator})
}

func encodeExternalReferenceAttachmentV1(payload continuity.ExternalReferenceAttachmentPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireExternalReferenceAttachmentV1{Observation: toWireObservationV1(payload.Observation), Target: toWireSubjectRefV1(payload.Target), Predecessor: string(payload.Predecessor)})
}

func encodeExternalReferenceDetachmentV1(payload continuity.ExternalReferenceDetachmentPayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireExternalReferenceDetachmentV1{Observation: toWireObservationV1(payload.Observation), Target: toWireSubjectRefV1(payload.Target), Predecessor: string(payload.Predecessor), Reason: payload.Reason})
}

func encodeVerificationEvidenceV1(payload continuity.VerificationEvidencePayload) (canonicalContentV1, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	return encodeWireV1(wireVerificationEvidenceV1{
		Observation: toWireObservationV1(payload.Observation),
		Target:      toWireSubjectRefV1(payload.Target),
		Check:       payload.Check,
		Method:      payload.Method,
		Outcome:     string(payload.Outcome),
		Detail:      payload.Detail,
	})
}

func toWireObservationV1(observation continuity.Observation) wireObservationV1 {
	return wireObservationV1{
		ObservedAtMillis: observation.ObservedAtMillis,
		HarnessSessionID: observation.HarnessSessionID,
		Branch:           observation.Branch,
		Worktree:         observation.Worktree,
	}
}

func toWireSubjectRefV1(reference continuity.SubjectRef) wireSubjectRefV1 {
	return wireSubjectRefV1{Kind: string(reference.Kind), ID: string(reference.ID)}
}

func toWireOptionalSubjectRefV1(reference *continuity.SubjectRef) *wireSubjectRefV1 {
	if reference == nil {
		return nil
	}
	wire := toWireSubjectRefV1(*reference)
	return &wire
}

func toWireJournalContentV1(content continuity.JournalContent) wireJournalContentV1 {
	return wireJournalContentV1{Category: string(content.Category), Scope: content.Scope, Text: content.Text}
}

func toWireIdeaContentV1(content continuity.IdeaContent) wireIdeaContentV1 {
	return wireIdeaContentV1{Label: content.Label, Text: content.Text}
}

func toWireFindingContentV1(content continuity.FindingContent) wireFindingContentV1 {
	return wireFindingContentV1{Scope: content.Scope, Summary: content.Summary, Detail: content.Detail, Recommendation: content.Recommendation}
}

func normalizeCheckpointItemsV1(items []wireCheckpointItemV1) []wireCheckpointItemV1 {
	if len(items) == 0 {
		return []wireCheckpointItemV1{}
	}
	return append([]wireCheckpointItemV1(nil), items...)
}

func normalizeStringsV1(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}
