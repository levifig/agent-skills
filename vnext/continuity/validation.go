package continuity

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maximumShortBytes      = 512
	maximumLocatorBytes    = 4096
	maximumCheckpointBytes = 4096
	maximumProseBytes      = 65536
	maximumListItems       = 64
)

// Validate checks one fact identity without changing it.
func (id FactID) Validate() error {
	return validateOpaqueID("fact_id", string(id), false)
}

// Validate checks one project identity without changing it.
func (id ProjectID) Validate() error {
	return validateOpaqueID("project_id", string(id), false)
}

// Validate checks one subject identity without changing it.
func (id SubjectID) Validate() error {
	return validateOpaqueID("subject_id", string(id), false)
}

// Validate checks one environment identity without changing it.
func (id EnvironmentID) Validate() error {
	return validateOpaqueID("environment_id", string(id), false)
}

// Validate checks optional observation provenance without changing it.
func (observation Observation) Validate() error {
	if observation.ObservedAtMillis < 0 {
		return invalid("observation.observed_at_millis", "must be zero or positive")
	}
	if err := validateOptionalText("observation.harness_session_id", observation.HarnessSessionID, maximumLocatorBytes, false); err != nil {
		return err
	}
	if err := validateOptionalText("observation.branch", observation.Branch, maximumLocatorBytes, false); err != nil {
		return err
	}
	return validateOptionalText("observation.worktree", observation.Worktree, maximumLocatorBytes, false)
}

func validateDurableReference(reference SubjectRef) error {
	if err := reference.ID.Validate(); err != nil {
		return refield(err, "reference.id")
	}
	switch reference.Kind {
	case RecordProjectIdentity,
		RecordJournalEntry,
		RecordWrap,
		RecordSpark,
		RecordIdea,
		RecordDecision,
		RecordExploration,
		RecordCheckpoint,
		RecordFinding,
		RecordHandoff,
		RecordExternalReference:
		return nil
	default:
		return invalid("reference.kind", "must identify an attachable durable continuity family")
	}
}

// Validate checks project registration content.
func (payload ProjectRegistrationPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	return validateRequiredShort("label", payload.Label)
}

// Validate checks project label revision content.
func (payload ProjectLabelRevisionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("revises", string(payload.Revises), false); err != nil {
		return err
	}
	return validateRequiredShort("label", payload.Label)
}

// Validate checks journal entry content.
func (payload JournalRecordedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	return payload.Content.Validate()
}

// Validate checks journal correction content.
func (payload JournalCorrectionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("corrects", string(payload.Corrects), false); err != nil {
		return err
	}
	return payload.Content.Validate()
}

// Validate checks complete journal content.
func (content JournalContent) Validate() error {
	switch content.Category {
	case JournalNote, JournalSkill, JournalCommit, JournalDecision, JournalDiscover, JournalBlock, JournalUnblock, JournalSpark, JournalTodo, JournalFinding, JournalWrap:
	default:
		return invalid("content.category", "is not a recognized journal category")
	}
	if err := validateOptionalShort("content.scope", content.Scope); err != nil {
		return err
	}
	return validateRequiredProse("content.text", content.Text, maximumProseBytes)
}

// Validate checks wrap content.
func (payload WrapRecordedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOptionalReference("focus", payload.Focus); err != nil {
		return err
	}
	if err := validateOptionalShort("scope", payload.Scope); err != nil {
		return err
	}
	return validateRequiredProse("synthesis", payload.Synthesis, maximumProseBytes)
}

// Validate checks spark capture content.
func (payload SparkCapturedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOptionalShort("scope", payload.Scope); err != nil {
		return err
	}
	return validateRequiredProse("text", payload.Text, maximumProseBytes)
}

// Validate checks spark dismissal content.
func (payload SparkDismissedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOptionalProse("reason", payload.Reason, maximumProseBytes)
}

// Validate checks spark promotion content.
func (payload SparkPromotionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOpaqueID("idea_id", string(payload.IdeaID), false)
}

// Validate checks idea creation content.
func (payload IdeaCreatedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	return payload.Content.Validate()
}

// Validate checks idea revision content.
func (payload IdeaRevisionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("revises", string(payload.Revises), false); err != nil {
		return err
	}
	return payload.Content.Validate()
}

// Validate checks complete idea content.
func (content IdeaContent) Validate() error {
	if err := validateRequiredShort("content.label", content.Label); err != nil {
		return err
	}
	return validateOptionalProse("content.text", content.Text, maximumProseBytes)
}

// Validate checks idea resolution content.
func (payload IdeaResolutionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOptionalProse("resolution", payload.Resolution, maximumProseBytes)
}

// Validate checks idea archive content.
func (payload IdeaArchivePayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOptionalProse("reason", payload.Reason, maximumProseBytes)
}

// Validate checks idea promotion content.
func (payload IdeaPromotionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOpaqueID("reference_id", string(payload.ReferenceID), false)
}

// Validate checks decision opening content.
func (payload DecisionOpenedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOptionalShort("scope", payload.Scope); err != nil {
		return err
	}
	if err := validateRequiredProse("question", payload.Question, maximumProseBytes); err != nil {
		return err
	}
	return validateOptionalProse("context", payload.Context, maximumProseBytes)
}

// Validate checks decision resolution content.
func (payload DecisionResolutionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	if err := validateRequiredProse("resolution", payload.Resolution, maximumProseBytes); err != nil {
		return err
	}
	return validateOptionalProse("rationale", payload.Rationale, maximumProseBytes)
}

// Validate checks decision supersession content.
func (payload DecisionSupersessionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	if err := validateOpaqueID("successor_id", string(payload.SuccessorID), false); err != nil {
		return err
	}
	return validateOptionalProse("rationale", payload.Rationale, maximumProseBytes)
}

// Validate checks exploration start content.
func (payload ExplorationStartedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateRequiredShort("label", payload.Label); err != nil {
		return err
	}
	return validateOptionalProse("purpose", payload.Purpose, maximumProseBytes)
}

// Validate checks exploration checkpoint content.
func (payload CheckpointRecordedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("exploration_id", string(payload.ExplorationID), false); err != nil {
		return err
	}
	fields := []struct {
		name  string
		value string
	}{
		{name: "current_framing", value: payload.CurrentFraming},
		{name: "conclusions", value: payload.Conclusions},
		{name: "unresolved_question", value: payload.UnresolvedQuestion},
		{name: "next_action", value: payload.NextAction},
	}
	for _, field := range fields {
		if err := validateRequiredProse(field.name, field.value, maximumCheckpointBytes); err != nil {
			return err
		}
	}
	if len(payload.Items) > maximumListItems {
		return invalid("items", fmt.Sprintf("must contain at most %d items", maximumListItems))
	}
	for index, item := range payload.Items {
		switch item.Kind {
		case CheckpointCandidate, CheckpointEvidence, CheckpointConstraint:
		default:
			return invalid(fmt.Sprintf("items[%d].kind", index), "is not a recognized checkpoint item kind")
		}
		if err := validateRequiredProse(fmt.Sprintf("items[%d].text", index), item.Text, maximumCheckpointBytes); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks finding creation content.
func (payload FindingRecordedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	return payload.Content.Validate()
}

// Validate checks finding correction content.
func (payload FindingCorrectionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("corrects", string(payload.Corrects), false); err != nil {
		return err
	}
	return payload.Content.Validate()
}

// Validate checks complete finding content.
func (content FindingContent) Validate() error {
	if err := validateOptionalShort("content.scope", content.Scope); err != nil {
		return err
	}
	if err := validateRequiredProse("content.summary", content.Summary, maximumProseBytes); err != nil {
		return err
	}
	if err := validateOptionalProse("content.detail", content.Detail, maximumProseBytes); err != nil {
		return err
	}
	return validateOptionalProse("content.recommendation", content.Recommendation, maximumProseBytes)
}

// Validate checks finding retraction content.
func (payload FindingRetractionPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOptionalProse("reason", payload.Reason, maximumProseBytes)
}

// Validate checks handoff content.
func (payload HandoffRecordedPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateOptionalReference("focus", payload.Focus); err != nil {
		return err
	}
	if err := validateRequiredProse("purpose", payload.Purpose, maximumProseBytes); err != nil {
		return err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "situation", value: payload.Situation},
		{name: "next_actions", value: payload.NextActions},
	} {
		if err := validateOptionalProse(field.name, field.value, maximumProseBytes); err != nil {
			return err
		}
	}
	if err := validateOptionalProse("questions_and_risks", payload.QuestionsAndRisks, maximumProseBytes); err != nil {
		return err
	}
	if len(payload.SuggestedSkills) > maximumListItems {
		return invalid("suggested_skills", fmt.Sprintf("must contain at most %d skills", maximumListItems))
	}
	for index, skill := range payload.SuggestedSkills {
		if err := validateRequiredShort(fmt.Sprintf("suggested_skills[%d]", index), skill); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks opaque external reference registration content.
func (payload ExternalReferenceRegistrationPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	return validateOpaqueLocator("locator", payload.Locator)
}

// Validate checks external reference attachment content.
func (payload ExternalReferenceAttachmentPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateAttachmentTarget("target", payload.Target); err != nil {
		return err
	}
	return validateOpaqueID("predecessor", string(payload.Predecessor), true)
}

// Validate checks external reference detachment content.
func (payload ExternalReferenceDetachmentPayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateAttachmentTarget("target", payload.Target); err != nil {
		return err
	}
	if err := validateOpaqueID("predecessor", string(payload.Predecessor), false); err != nil {
		return err
	}
	return validateOptionalProse("reason", payload.Reason, maximumProseBytes)
}

// Validate checks verification evidence content.
func (payload VerificationEvidencePayload) Validate() error {
	if err := payload.Observation.Validate(); err != nil {
		return err
	}
	if err := validateReference("target", payload.Target); err != nil {
		return err
	}
	if err := validateRequiredShort("check", payload.Check); err != nil {
		return err
	}
	if err := validateRequiredShort("method", payload.Method); err != nil {
		return err
	}
	switch payload.Outcome {
	case VerificationPassed, VerificationFailed, VerificationIndeterminate:
	default:
		return invalid("outcome", "is not a recognized verification outcome")
	}
	return validateRequiredProse("detail", payload.Detail, maximumProseBytes)
}

func validateReference(field string, reference SubjectRef) error {
	if err := validateDurableReference(reference); err != nil {
		return refieldPrefix(err, "reference", field)
	}
	return nil
}

func validateAttachmentTarget(field string, reference SubjectRef) error {
	if err := validateReference(field, reference); err != nil {
		return err
	}
	if reference.Kind == RecordExternalReference {
		return invalid(field+".kind", "cannot attach an opaque reference to another opaque reference")
	}
	return nil
}

func validateOptionalReference(field string, reference *SubjectRef) error {
	if reference == nil {
		return nil
	}
	return validateReference(field, *reference)
}

func validateOpaqueID(field, value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if len(value) < 1 || len(value) > 128 {
		return invalid(field, "must contain 1 to 128 characters")
	}
	for _, character := range value {
		switch {
		case character >= 'A' && character <= 'Z':
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("_.:-", character):
		default:
			return invalid(field, "contains a character outside the opaque identity alphabet")
		}
	}
	return nil
}

func validateRequiredShort(field, value string) error {
	return validateRequiredText(field, value, maximumShortBytes, true)
}

func validateOptionalShort(field, value string) error {
	return validateOptionalText(field, value, maximumShortBytes, true)
}

func validateRequiredProse(field, value string, maximum int) error {
	return validateRequiredText(field, value, maximum, false)
}

func validateOptionalProse(field, value string, maximum int) error {
	return validateOptionalText(field, value, maximum, false)
}

func validateRequiredText(field, value string, maximum int, trimmed bool) error {
	if err := validateText(field, value, maximum); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		return invalid(field, "must contain non-whitespace text")
	}
	if trimmed && strings.TrimSpace(value) != value {
		return invalid(field, "must not have outer whitespace")
	}
	return nil
}

func validateOptionalText(field, value string, maximum int, trimmed bool) error {
	if value == "" {
		return nil
	}
	return validateRequiredText(field, value, maximum, trimmed)
}

func validateText(field, value string, maximum int) error {
	if !utf8.ValidString(value) {
		return invalid(field, "must be valid UTF-8")
	}
	if strings.ContainsRune(value, '\x00') {
		return invalid(field, "must not contain NUL")
	}
	if strings.ContainsRune(value, '\r') {
		return invalid(field, "must use LF line endings")
	}
	for _, character := range value {
		if (character < 0x20 && character != '\n' && character != '\t') || character == '\u2028' || character == '\u2029' {
			return invalid(field, "contains an unsafe control character")
		}
	}
	if len(value) > maximum {
		return invalid(field, fmt.Sprintf("must contain at most %d UTF-8 bytes", maximum))
	}
	return nil
}

func validateOpaqueLocator(field, value string) error {
	if len(value) < 1 || len(value) > maximumShortBytes {
		return invalid(field, fmt.Sprintf("must contain 1 to %d characters", maximumShortBytes))
	}
	if strings.Contains(value, "//") {
		return invalid(field, "must be an opaque locator rather than a URL")
	}
	for _, character := range value {
		switch {
		case character >= 'A' && character <= 'Z':
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("_.:/#-", character):
		default:
			return invalid(field, "contains a character outside the opaque locator alphabet")
		}
	}
	return nil
}

func invalid(field, detail string) error {
	return &Problem{Code: ProblemInvalid, Field: field, Detail: detail}
}

func refield(err error, field string) error {
	problem, ok := err.(*Problem)
	if !ok {
		return err
	}
	return &Problem{Code: problem.Code, Field: field, Detail: problem.Detail}
}

func refieldPrefix(err error, oldPrefix, newPrefix string) error {
	problem, ok := err.(*Problem)
	if !ok {
		return err
	}
	field := problem.Field
	if field == oldPrefix {
		field = newPrefix
	} else if strings.HasPrefix(field, oldPrefix+".") {
		field = newPrefix + strings.TrimPrefix(field, oldPrefix)
	}
	return &Problem{Code: problem.Code, Field: field, Detail: problem.Detail}
}
