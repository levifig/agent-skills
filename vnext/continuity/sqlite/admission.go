package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

func admitFactV1(ctx context.Context, tx *sql.Tx, intent appendIntentV1) error {
	if intent.kind != continuity.FactProjectRegistered {
		if err := requireProjectV1(ctx, tx, intent.projectID); err != nil {
			return err
		}
	}

	switch intent.kind {
	case continuity.FactProjectRegistered:
		return admitProjectRegistrationV1(ctx, tx, intent)
	case continuity.FactProjectLabelRevised:
		payload, err := decodeStoredWireV1[wireProjectLabelRevisionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireCurrentPredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Revises, "revises", continuity.FactProjectRegistered, continuity.FactProjectLabelRevised)
	case continuity.FactJournalRecorded:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactJournalCorrectionRecorded:
		payload, err := decodeStoredWireV1[wireJournalCorrectionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireCurrentPredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Corrects, "corrects", continuity.FactJournalRecorded, continuity.FactJournalCorrectionRecorded)
	case continuity.FactWrapRecorded:
		payload, err := decodeStoredWireV1[wireWrapRecordedV1](string(intent.content))
		if err != nil {
			return err
		}
		return admitNewSubjectWithFocusV1(ctx, tx, intent, payload.Focus)
	case continuity.FactSparkCaptured:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactSparkDismissed:
		payload, err := decodeStoredWireV1[wireSparkDismissedV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactSparkCaptured}, []continuity.FactKind{continuity.FactSparkDismissed, continuity.FactSparkPromotedToIdea})
	case continuity.FactSparkPromotedToIdea:
		payload, err := decodeStoredWireV1[wireSparkPromotionV1](string(intent.content))
		if err != nil {
			return err
		}
		if err := requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactSparkCaptured}, []continuity.FactKind{continuity.FactSparkDismissed, continuity.FactSparkPromotedToIdea}); err != nil {
			return err
		}
		return requireReferenceV1(ctx, tx, intent.projectID, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: continuity.SubjectID(payload.IdeaID)}, "idea_id")
	case continuity.FactIdeaCreated:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactIdeaRevised:
		payload, err := decodeStoredWireV1[wireIdeaRevisionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Revises, "revises", []continuity.FactKind{continuity.FactIdeaCreated, continuity.FactIdeaRevised}, []continuity.FactKind{continuity.FactIdeaResolved, continuity.FactIdeaArchived, continuity.FactIdeaPromotedToExternalReference})
	case continuity.FactIdeaResolved:
		payload, err := decodeStoredWireV1[wireIdeaResolutionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactIdeaCreated, continuity.FactIdeaRevised}, []continuity.FactKind{continuity.FactIdeaResolved, continuity.FactIdeaArchived, continuity.FactIdeaPromotedToExternalReference})
	case continuity.FactIdeaArchived:
		payload, err := decodeStoredWireV1[wireIdeaArchiveV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactIdeaCreated, continuity.FactIdeaRevised}, []continuity.FactKind{continuity.FactIdeaResolved, continuity.FactIdeaArchived, continuity.FactIdeaPromotedToExternalReference})
	case continuity.FactIdeaPromotedToExternalReference:
		payload, err := decodeStoredWireV1[wireIdeaPromotionV1](string(intent.content))
		if err != nil {
			return err
		}
		if err := requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactIdeaCreated, continuity.FactIdeaRevised}, []continuity.FactKind{continuity.FactIdeaResolved, continuity.FactIdeaArchived, continuity.FactIdeaPromotedToExternalReference}); err != nil {
			return err
		}
		return requireReferenceV1(ctx, tx, intent.projectID, continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: continuity.SubjectID(payload.ReferenceID)}, "reference_id")
	case continuity.FactDecisionOpened:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactDecisionResolved:
		payload, err := decodeStoredWireV1[wireDecisionResolutionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactDecisionOpened}, []continuity.FactKind{continuity.FactDecisionResolved, continuity.FactDecisionSuperseded})
	case continuity.FactDecisionSuperseded:
		payload, err := decodeStoredWireV1[wireDecisionSupersessionV1](string(intent.content))
		if err != nil {
			return err
		}
		if err := requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactDecisionOpened, continuity.FactDecisionResolved}, []continuity.FactKind{continuity.FactDecisionSuperseded}); err != nil {
			return err
		}
		return admitDecisionSuccessorV1(ctx, tx, intent.projectID, intent.subject.ID, continuity.SubjectID(payload.SuccessorID))
	case continuity.FactExplorationStarted:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactCheckpointRecorded:
		payload, err := decodeStoredWireV1[wireCheckpointRecordedV1](string(intent.content))
		if err != nil {
			return err
		}
		if err := requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject); err != nil {
			return err
		}
		return requireReferenceV1(ctx, tx, intent.projectID, continuity.SubjectRef{Kind: continuity.RecordExploration, ID: continuity.SubjectID(payload.ExplorationID)}, "exploration_id")
	case continuity.FactFindingRecorded:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactFindingCorrected:
		payload, err := decodeStoredWireV1[wireFindingCorrectionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Corrects, "corrects", []continuity.FactKind{continuity.FactFindingRecorded, continuity.FactFindingCorrected}, []continuity.FactKind{continuity.FactFindingRetracted})
	case continuity.FactFindingRetracted:
		payload, err := decodeStoredWireV1[wireFindingRetractionV1](string(intent.content))
		if err != nil {
			return err
		}
		return requireActivePredecessorV1(ctx, tx, intent.projectID, intent.subject, payload.Predecessor, "predecessor", []continuity.FactKind{continuity.FactFindingRecorded, continuity.FactFindingCorrected}, []continuity.FactKind{continuity.FactFindingRetracted})
	case continuity.FactHandoffRecorded:
		payload, err := decodeStoredWireV1[wireHandoffRecordedV1](string(intent.content))
		if err != nil {
			return err
		}
		return admitNewSubjectWithFocusV1(ctx, tx, intent, payload.Focus)
	case continuity.FactExternalReferenceRegistered:
		return requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject)
	case continuity.FactExternalReferenceAttached, continuity.FactExternalReferenceDetached:
		return admitExternalReferenceEdgeV1(ctx, tx, intent)
	case continuity.FactVerificationEvidenceRecorded:
		payload, err := decodeStoredWireV1[wireVerificationEvidenceV1](string(intent.content))
		if err != nil {
			return err
		}
		if err := requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject); err != nil {
			return err
		}
		return requireReferenceV1(ctx, tx, intent.projectID, payload.Target.domain(), "target")
	default:
		return &continuity.Problem{Code: continuity.ProblemInvalid, Field: "kind", Detail: "is not part of the closed continuity catalog"}
	}
}

func admitProjectRegistrationV1(ctx context.Context, tx *sql.Tx, intent appendIntentV1) error {
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT fact_id FROM continuity_facts WHERE project_id = ? LIMIT 1`, string(intent.projectID)).Scan(&existing)
	if err == nil {
		return &continuity.Problem{Code: continuity.ProblemProjectAlreadyRegistered, Field: "project_id", Detail: "already has a continuity identity"}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return transactionOperationProblemV1(ctx)
	}
	return nil
}

func admitNewSubjectWithFocusV1(ctx context.Context, tx *sql.Tx, intent appendIntentV1, focus *wireSubjectRefV1) error {
	if err := requireNewSubjectV1(ctx, tx, intent.projectID, intent.subject); err != nil {
		return err
	}
	if focus == nil {
		return nil
	}
	return requireReferenceV1(ctx, tx, intent.projectID, focus.domain(), "focus")
}

func requireProjectV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	facts, err := loadSubjectFactsV1(ctx, tx, projectID, continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)})
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return &continuity.Problem{Code: continuity.ProblemProjectNotRegistered, Field: "project_id", Detail: "has no continuity identity"}
	}
	for _, fact := range facts {
		if fact.kind == continuity.FactProjectRegistered {
			return nil
		}
	}
	return corruptFactProblemV1()
}

func requireNewSubjectV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, subject continuity.SubjectRef) error {
	facts, err := loadSubjectFactsV1(ctx, tx, projectID, subject)
	if err != nil {
		return err
	}
	if len(facts) != 0 {
		return &continuity.Problem{Code: continuity.ProblemSubjectAlreadyRegistered, Field: "subject_id", Detail: "already identifies a continuity record in this family"}
	}
	return nil
}

func requireReferenceV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference continuity.SubjectRef, field string) error {
	facts, err := loadSubjectFactsV1(ctx, tx, projectID, reference)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return &continuity.Problem{Code: continuity.ProblemReferenceNotFound, Field: field, Detail: "does not identify an existing same-project continuity record"}
	}
	return nil
}

func requireCurrentPredecessorV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, subject continuity.SubjectRef, predecessor string, field string, allowed ...continuity.FactKind) error {
	return requireActivePredecessorV1(ctx, tx, projectID, subject, predecessor, field, allowed, nil)
}

func requireActivePredecessorV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, subject continuity.SubjectRef, predecessor string, field string, active []continuity.FactKind, terminal []continuity.FactKind) error {
	facts, err := loadSubjectFactsV1(ctx, tx, projectID, subject)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return &continuity.Problem{Code: continuity.ProblemSubjectNotRegistered, Field: "subject_id", Detail: "does not identify an existing continuity record in this family"}
	}
	head := facts[len(facts)-1]
	if containsFactKindV1(terminal, head.kind) {
		return &continuity.Problem{Code: continuity.ProblemPreconditionFailed, Field: "subject_id", Detail: "the continuity record is already terminal"}
	}
	if !containsFactKindV1(active, head.kind) {
		return corruptFactProblemV1()
	}
	if head.factID == continuity.FactID(predecessor) {
		return nil
	}
	_, found, err := readFactByIDV1(ctx, tx, continuity.FactID(predecessor))
	if err != nil {
		return err
	}
	if !found {
		return &continuity.Problem{Code: continuity.ProblemReferenceNotFound, Field: field, Detail: "does not identify an existing predecessor fact"}
	}
	return &continuity.Problem{Code: continuity.ProblemReferenceMismatch, Field: field, Detail: "is not the current predecessor for this continuity record"}
}

func containsFactKindV1(kinds []continuity.FactKind, candidate continuity.FactKind) bool {
	for _, kind := range kinds {
		if kind == candidate {
			return true
		}
	}
	return false
}

func loadSubjectFactsV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, subject continuity.SubjectRef) ([]storedFactV1, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
FROM continuity_facts
WHERE project_id = ? AND subject_kind = ? AND subject_id = ?
ORDER BY hlc_wall_millis, hlc_logical, environment_id, fact_id`, string(projectID), string(subject.Kind), string(subject.ID))
	if err != nil {
		return nil, transactionOperationProblemV1(ctx)
	}
	defer rows.Close()

	var facts []storedFactV1
	for rows.Next() {
		fact, err := scanStoredFactRowsV1(ctx, rows)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, transactionOperationProblemV1(ctx)
	}
	if err := validateSubjectHistoryV1(subject, facts); err != nil {
		return nil, err
	}
	return facts, nil
}

func validateSubjectHistoryV1(subject continuity.SubjectRef, facts []storedFactV1) error {
	if len(facts) == 0 {
		return nil
	}
	root, ok := rootFactKindV1(subject.Kind)
	if !ok || facts[0].kind != root {
		return corruptFactProblemV1()
	}
	rootCount := 0
	for _, fact := range facts {
		if fact.kind == root {
			rootCount++
		}
	}
	if rootCount != 1 {
		return corruptFactProblemV1()
	}
	return nil
}

func rootFactKindV1(kind continuity.RecordKind) (continuity.FactKind, bool) {
	switch kind {
	case continuity.RecordProjectIdentity:
		return continuity.FactProjectRegistered, true
	case continuity.RecordJournalEntry:
		return continuity.FactJournalRecorded, true
	case continuity.RecordWrap:
		return continuity.FactWrapRecorded, true
	case continuity.RecordSpark:
		return continuity.FactSparkCaptured, true
	case continuity.RecordIdea:
		return continuity.FactIdeaCreated, true
	case continuity.RecordDecision:
		return continuity.FactDecisionOpened, true
	case continuity.RecordExploration:
		return continuity.FactExplorationStarted, true
	case continuity.RecordCheckpoint:
		return continuity.FactCheckpointRecorded, true
	case continuity.RecordFinding:
		return continuity.FactFindingRecorded, true
	case continuity.RecordHandoff:
		return continuity.FactHandoffRecorded, true
	case continuity.RecordExternalReference:
		return continuity.FactExternalReferenceRegistered, true
	case continuity.RecordVerificationEvidence:
		return continuity.FactVerificationEvidenceRecorded, true
	default:
		return "", false
	}
}

func admitDecisionSuccessorV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, sourceID, successorID continuity.SubjectID) error {
	if sourceID == successorID {
		return &continuity.Problem{Code: continuity.ProblemReferenceMismatch, Field: "successor_id", Detail: "must identify a distinct decision"}
	}
	successor := continuity.SubjectRef{Kind: continuity.RecordDecision, ID: successorID}
	facts, err := loadSubjectFactsV1(ctx, tx, projectID, successor)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return &continuity.Problem{Code: continuity.ProblemReferenceNotFound, Field: "successor_id", Detail: "does not identify an existing same-project decision"}
	}
	if facts[len(facts)-1].kind == continuity.FactDecisionSuperseded {
		return &continuity.Problem{Code: continuity.ProblemPreconditionFailed, Field: "successor_id", Detail: "the successor decision is already superseded"}
	}
	return nil
}

type externalEdgeStateV1 struct {
	found  bool
	active bool
	head   continuity.FactID
}

func admitExternalReferenceEdgeV1(ctx context.Context, tx *sql.Tx, intent appendIntentV1) error {
	facts, err := loadSubjectFactsV1(ctx, tx, intent.projectID, intent.subject)
	if err != nil {
		return err
	}
	if len(facts) == 0 || facts[0].kind != continuity.FactExternalReferenceRegistered {
		return &continuity.Problem{Code: continuity.ProblemSubjectNotRegistered, Field: "subject_id", Detail: "does not identify a registered opaque reference"}
	}

	var target continuity.SubjectRef
	var predecessor string
	if intent.kind == continuity.FactExternalReferenceAttached {
		payload, err := decodeStoredWireV1[wireExternalReferenceAttachmentV1](string(intent.content))
		if err != nil {
			return err
		}
		target = payload.Target.domain()
		predecessor = payload.Predecessor
	} else {
		payload, err := decodeStoredWireV1[wireExternalReferenceDetachmentV1](string(intent.content))
		if err != nil {
			return err
		}
		target = payload.Target.domain()
		predecessor = payload.Predecessor
	}
	if err := requireReferenceV1(ctx, tx, intent.projectID, target, "target"); err != nil {
		return err
	}
	edge, err := foldExternalEdgeV1(facts, target)
	if err != nil {
		return err
	}
	if intent.kind == continuity.FactExternalReferenceAttached {
		if edge.active {
			return &continuity.Problem{Code: continuity.ProblemPreconditionFailed, Field: "target", Detail: "the opaque reference is already attached to this target"}
		}
		if !edge.found {
			if predecessor != "" {
				return classifyPredecessorMismatchV1(ctx, tx, predecessor, "predecessor")
			}
			return nil
		}
		return requireExactEdgePredecessorV1(ctx, tx, predecessor, edge.head)
	}
	if !edge.found || !edge.active {
		return &continuity.Problem{Code: continuity.ProblemPreconditionFailed, Field: "target", Detail: "the opaque reference is not attached to this target"}
	}
	return requireExactEdgePredecessorV1(ctx, tx, predecessor, edge.head)
}

func foldExternalEdgeV1(facts []storedFactV1, target continuity.SubjectRef) (externalEdgeStateV1, error) {
	var edge externalEdgeStateV1
	for _, fact := range facts {
		switch fact.kind {
		case continuity.FactExternalReferenceRegistered:
		case continuity.FactExternalReferenceAttached:
			payload, err := decodeStoredWireV1[wireExternalReferenceAttachmentV1](string(fact.content))
			if err != nil {
				return externalEdgeStateV1{}, err
			}
			if payload.Target.domain() == target {
				edge = externalEdgeStateV1{found: true, active: true, head: fact.factID}
			}
		case continuity.FactExternalReferenceDetached:
			payload, err := decodeStoredWireV1[wireExternalReferenceDetachmentV1](string(fact.content))
			if err != nil {
				return externalEdgeStateV1{}, err
			}
			if payload.Target.domain() == target {
				edge = externalEdgeStateV1{found: true, head: fact.factID}
			}
		default:
			return externalEdgeStateV1{}, corruptFactProblemV1()
		}
	}
	return edge, nil
}

func requireExactEdgePredecessorV1(ctx context.Context, tx *sql.Tx, supplied string, current continuity.FactID) error {
	if continuity.FactID(supplied) == current {
		return nil
	}
	return classifyPredecessorMismatchV1(ctx, tx, supplied, "predecessor")
}

func classifyPredecessorMismatchV1(ctx context.Context, tx *sql.Tx, predecessor string, field string) error {
	if predecessor == "" {
		return &continuity.Problem{Code: continuity.ProblemReferenceMismatch, Field: field, Detail: "does not match the current edge predecessor"}
	}
	_, found, err := readFactByIDV1(ctx, tx, continuity.FactID(predecessor))
	if err != nil {
		return err
	}
	if !found {
		return &continuity.Problem{Code: continuity.ProblemReferenceNotFound, Field: field, Detail: "does not identify an existing predecessor fact"}
	}
	return &continuity.Problem{Code: continuity.ProblemReferenceMismatch, Field: field, Detail: "does not match the current edge predecessor"}
}
