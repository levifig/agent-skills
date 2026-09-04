package sqlite

import (
	"context"

	"github.com/levifig/loaf/vnext/continuity"
)

// RegisterProject records a project's mint-once identity.
func (store *Store) RegisterProject(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, payload continuity.ProjectRegistrationPayload) (continuity.AppendReceipt, error) {
	content, err := encodeProjectRegistrationV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}, continuity.FactProjectRegistered, content)
}

// ReviseProjectLabel records a causally linked project-label replacement.
func (store *Store) ReviseProjectLabel(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, payload continuity.ProjectLabelRevisionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeProjectLabelRevisionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}, continuity.FactProjectLabelRevised, content)
}

// RecordJournalEntry records a new project-journal entry.
func (store *Store) RecordJournalEntry(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, journalID continuity.SubjectID, payload continuity.JournalRecordedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeJournalRecordedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: journalID}, continuity.FactJournalRecorded, content)
}

// CorrectJournalEntry records a complete replacement for one journal entry.
func (store *Store) CorrectJournalEntry(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, journalID continuity.SubjectID, payload continuity.JournalCorrectionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeJournalCorrectionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: journalID}, continuity.FactJournalCorrectionRecorded, content)
}

// RecordWrap records a new synthesis checkpoint.
func (store *Store) RecordWrap(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, wrapID continuity.SubjectID, payload continuity.WrapRecordedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeWrapRecordedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordWrap, ID: wrapID}, continuity.FactWrapRecorded, content)
}

// CaptureSpark records a new spark.
func (store *Store) CaptureSpark(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, sparkID continuity.SubjectID, payload continuity.SparkCapturedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeSparkCapturedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordSpark, ID: sparkID}, continuity.FactSparkCaptured, content)
}

// DismissSpark records a terminal spark dismissal.
func (store *Store) DismissSpark(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, sparkID continuity.SubjectID, payload continuity.SparkDismissedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeSparkDismissedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordSpark, ID: sparkID}, continuity.FactSparkDismissed, content)
}

// PromoteSparkToIdea links an active spark to an already-created idea.
func (store *Store) PromoteSparkToIdea(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, sparkID continuity.SubjectID, payload continuity.SparkPromotionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeSparkPromotionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordSpark, ID: sparkID}, continuity.FactSparkPromotedToIdea, content)
}

// CreateIdea records a new idea.
func (store *Store) CreateIdea(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, ideaID continuity.SubjectID, payload continuity.IdeaCreatedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeIdeaCreatedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: ideaID}, continuity.FactIdeaCreated, content)
}

// ReviseIdea records a complete replacement for an active idea.
func (store *Store) ReviseIdea(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, ideaID continuity.SubjectID, payload continuity.IdeaRevisionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeIdeaRevisionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: ideaID}, continuity.FactIdeaRevised, content)
}

// ResolveIdea records a terminal idea resolution.
func (store *Store) ResolveIdea(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, ideaID continuity.SubjectID, payload continuity.IdeaResolutionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeIdeaResolutionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: ideaID}, continuity.FactIdeaResolved, content)
}

// ArchiveIdea records a terminal idea archive.
func (store *Store) ArchiveIdea(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, ideaID continuity.SubjectID, payload continuity.IdeaArchivePayload) (continuity.AppendReceipt, error) {
	content, err := encodeIdeaArchiveV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: ideaID}, continuity.FactIdeaArchived, content)
}

// PromoteIdeaToExternalReference links an active idea to an existing opaque reference.
func (store *Store) PromoteIdeaToExternalReference(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, ideaID continuity.SubjectID, payload continuity.IdeaPromotionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeIdeaPromotionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: ideaID}, continuity.FactIdeaPromotedToExternalReference, content)
}

// OpenDecision records a new open decision.
func (store *Store) OpenDecision(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, decisionID continuity.SubjectID, payload continuity.DecisionOpenedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeDecisionOpenedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordDecision, ID: decisionID}, continuity.FactDecisionOpened, content)
}

// ResolveDecision records a decision resolution.
func (store *Store) ResolveDecision(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, decisionID continuity.SubjectID, payload continuity.DecisionResolutionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeDecisionResolutionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordDecision, ID: decisionID}, continuity.FactDecisionResolved, content)
}

// SupersedeDecision links a decision to an existing successor without creating it.
func (store *Store) SupersedeDecision(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, decisionID continuity.SubjectID, payload continuity.DecisionSupersessionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeDecisionSupersessionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordDecision, ID: decisionID}, continuity.FactDecisionSuperseded, content)
}

// StartExploration records a new exploration identity.
func (store *Store) StartExploration(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, explorationID continuity.SubjectID, payload continuity.ExplorationStartedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeExplorationStartedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordExploration, ID: explorationID}, continuity.FactExplorationStarted, content)
}

// RecordCheckpoint records a new checkpoint for an existing exploration.
func (store *Store) RecordCheckpoint(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, checkpointID continuity.SubjectID, payload continuity.CheckpointRecordedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeCheckpointRecordedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordCheckpoint, ID: checkpointID}, continuity.FactCheckpointRecorded, content)
}

// RecordFinding records a new finding.
func (store *Store) RecordFinding(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, findingID continuity.SubjectID, payload continuity.FindingRecordedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeFindingRecordedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordFinding, ID: findingID}, continuity.FactFindingRecorded, content)
}

// CorrectFinding records a complete finding replacement.
func (store *Store) CorrectFinding(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, findingID continuity.SubjectID, payload continuity.FindingCorrectionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeFindingCorrectionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordFinding, ID: findingID}, continuity.FactFindingCorrected, content)
}

// RetractFinding records a terminal finding retraction.
func (store *Store) RetractFinding(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, findingID continuity.SubjectID, payload continuity.FindingRetractionPayload) (continuity.AppendReceipt, error) {
	content, err := encodeFindingRetractionV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordFinding, ID: findingID}, continuity.FactFindingRetracted, content)
}

// RecordHandoff records a new context-transfer packet.
func (store *Store) RecordHandoff(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, handoffID continuity.SubjectID, payload continuity.HandoffRecordedPayload) (continuity.AppendReceipt, error) {
	content, err := encodeHandoffRecordedV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordHandoff, ID: handoffID}, continuity.FactHandoffRecorded, content)
}

// RegisterExternalReference records an opaque external locator.
func (store *Store) RegisterExternalReference(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, referenceID continuity.SubjectID, payload continuity.ExternalReferenceRegistrationPayload) (continuity.AppendReceipt, error) {
	content, err := encodeExternalReferenceRegistrationV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: referenceID}, continuity.FactExternalReferenceRegistered, content)
}

// AttachExternalReference records an active edge from an opaque reference.
func (store *Store) AttachExternalReference(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, referenceID continuity.SubjectID, payload continuity.ExternalReferenceAttachmentPayload) (continuity.AppendReceipt, error) {
	content, err := encodeExternalReferenceAttachmentV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: referenceID}, continuity.FactExternalReferenceAttached, content)
}

// DetachExternalReference records an inactive edge from an opaque reference.
func (store *Store) DetachExternalReference(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, referenceID continuity.SubjectID, payload continuity.ExternalReferenceDetachmentPayload) (continuity.AppendReceipt, error) {
	content, err := encodeExternalReferenceDetachmentV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: referenceID}, continuity.FactExternalReferenceDetached, content)
}

// RecordVerificationEvidence records immutable external verification evidence.
func (store *Store) RecordVerificationEvidence(ctx context.Context, projectID continuity.ProjectID, factID continuity.FactID, evidenceID continuity.SubjectID, payload continuity.VerificationEvidencePayload) (continuity.AppendReceipt, error) {
	content, err := encodeVerificationEvidenceV1(payload)
	if err != nil {
		return continuity.AppendReceipt{}, err
	}
	return store.appendFactV1(ctx, projectID, factID, continuity.SubjectRef{Kind: continuity.RecordVerificationEvidence, ID: evidenceID}, continuity.FactVerificationEvidenceRecorded, content)
}
