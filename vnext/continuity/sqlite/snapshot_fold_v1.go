package sqlite

import (
	"context"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
)

type projectCorpusV1 struct {
	ctx       context.Context
	projectID continuity.ProjectID
	byFactID  map[continuity.FactID]storedFactV1
	bySubject map[continuity.SubjectRef][]storedFactV1
	roots     map[continuity.SubjectRef]storedFactV1
}

func foldProjectSnapshotV1(ctx context.Context, projectID continuity.ProjectID, atMillis int64, facts []storedFactV1) (continuity.Snapshot, error) {
	corpus, err := indexProjectCorpusV1(ctx, projectID, facts)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	project, err := foldProjectIdentityV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	journal, err := foldJournalV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	wraps, err := foldWrapsV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	sparks, err := foldSparksV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	ideas, err := foldIdeasV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	decisions, err := foldDecisionsV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	explorations, err := foldExplorationsV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	checkpoints, err := foldCheckpointsV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	findings, err := foldFindingsV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	handoffs, err := foldHandoffsV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	references, err := foldExternalReferencesV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	evidence, err := foldVerificationEvidenceV1(corpus)
	if err != nil {
		return continuity.Snapshot{}, err
	}
	if err := corpus.contextErrV1(); err != nil {
		return continuity.Snapshot{}, err
	}
	return continuity.Snapshot{
		AtMillis:             atMillis,
		Project:              continuity.ProjectIdentityProjection{Identity: project},
		EffectiveJournal:     continuity.EffectiveJournalProjection{Entries: journal},
		LatestWraps:          continuity.LatestWrapsProjection{Wraps: wraps},
		ActiveSparks:         continuity.ActiveSparksProjection{Sparks: sparks},
		CurrentIdeas:         continuity.CurrentIdeasProjection{Ideas: ideas},
		CurrentDecisions:     continuity.CurrentDecisionsProjection{Decisions: decisions},
		Explorations:         continuity.ExplorationsProjection{Explorations: explorations},
		LatestCheckpoints:    continuity.LatestCheckpointsProjection{Checkpoints: checkpoints},
		CurrentFindings:      continuity.CurrentFindingsProjection{Findings: findings},
		LatestHandoffs:       continuity.LatestHandoffsProjection{Handoffs: handoffs},
		ExternalReferences:   continuity.ExternalReferencesProjection{References: references},
		VerificationEvidence: continuity.VerificationEvidenceProjection{Evidence: evidence},
	}, nil
}

func indexProjectCorpusV1(ctx context.Context, projectID continuity.ProjectID, facts []storedFactV1) (projectCorpusV1, error) {
	corpus := projectCorpusV1{
		ctx:       ctx,
		projectID: projectID,
		byFactID:  make(map[continuity.FactID]storedFactV1, len(facts)),
		bySubject: make(map[continuity.SubjectRef][]storedFactV1),
		roots:     make(map[continuity.SubjectRef]storedFactV1),
	}
	for index, fact := range facts {
		if err := corpus.contextErrV1(); err != nil {
			return projectCorpusV1{}, err
		}
		if fact.projectID != projectID {
			return projectCorpusV1{}, corruptFactProblemV1()
		}
		if index > 0 && !storedFactLessV1(facts[index-1], fact) {
			return projectCorpusV1{}, corruptFactProblemV1()
		}
		if _, duplicate := corpus.byFactID[fact.factID]; duplicate {
			return projectCorpusV1{}, corruptFactProblemV1()
		}
		corpus.byFactID[fact.factID] = fact
		corpus.bySubject[fact.subject] = append(corpus.bySubject[fact.subject], fact)
	}
	for subject, subjectFacts := range corpus.bySubject {
		if err := corpus.contextErrV1(); err != nil {
			return projectCorpusV1{}, err
		}
		rootKind, ok := rootFactKindV1(subject.Kind)
		if !ok {
			return projectCorpusV1{}, corruptFactProblemV1()
		}
		var root storedFactV1
		found := false
		for _, fact := range subjectFacts {
			if fact.kind == rootKind {
				root = fact
				found = true
				break
			}
		}
		if !found {
			return projectCorpusV1{}, corruptFactProblemV1()
		}
		corpus.roots[subject] = root
	}
	projectSubject := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}
	if _, ok := corpus.roots[projectSubject]; !ok {
		return projectCorpusV1{}, corruptFactProblemV1()
	}
	return corpus, nil
}

func canonicalSubjectFactsV1(corpus projectCorpusV1, subject continuity.SubjectRef) (storedFactV1, []storedFactV1, error) {
	if err := corpus.contextErrV1(); err != nil {
		return storedFactV1{}, nil, err
	}
	root, ok := corpus.roots[subject]
	if !ok {
		return storedFactV1{}, nil, corruptFactProblemV1()
	}
	facts := corpus.bySubject[subject]
	if len(facts) == 0 {
		return storedFactV1{}, nil, corruptFactProblemV1()
	}
	return root, facts, nil
}

func requireEarlierSubjectV1(corpus projectCorpusV1, fact storedFactV1, reference continuity.SubjectRef) error {
	if err := corpus.contextErrV1(); err != nil {
		return err
	}
	root, ok := corpus.roots[reference]
	if !ok || !storedFactLessV1(root, fact) {
		return corruptFactProblemV1()
	}
	return nil
}

func eligiblePredecessorV1(corpus projectCorpusV1, fact storedFactV1, predecessor continuity.FactID, eligible map[continuity.FactID]bool, allowed ...continuity.FactKind) (bool, error) {
	if err := corpus.contextErrV1(); err != nil {
		return false, err
	}
	previous, ok := corpus.byFactID[predecessor]
	if !ok || previous.subject != fact.subject || !storedFactLessV1(previous, fact) || !containsFactKindV1(allowed, previous.kind) {
		return false, corruptFactProblemV1()
	}
	return eligible[predecessor], nil
}

func (corpus projectCorpusV1) contextErrV1() error {
	return corpus.ctx.Err()
}

func storedFactLessV1(left, right storedFactV1) bool {
	if left.clock.WallMillis != right.clock.WallMillis {
		return left.clock.WallMillis < right.clock.WallMillis
	}
	if left.clock.Logical != right.clock.Logical {
		return left.clock.Logical < right.clock.Logical
	}
	if left.environmentID != right.environmentID {
		return strings.Compare(string(left.environmentID), string(right.environmentID)) < 0
	}
	return strings.Compare(string(left.factID), string(right.factID)) < 0
}

func factStampV1(fact storedFactV1) continuity.FactStamp {
	return continuity.FactStamp{
		Clock:               fact.clock,
		EnvironmentID:       fact.environmentID,
		EnvironmentSequence: fact.environmentSequence,
		FactID:              fact.factID,
	}
}

func factStampLessV1(left, right continuity.FactStamp) bool {
	if left.Clock.WallMillis != right.Clock.WallMillis {
		return left.Clock.WallMillis < right.Clock.WallMillis
	}
	if left.Clock.Logical != right.Clock.Logical {
		return left.Clock.Logical < right.Clock.Logical
	}
	if left.EnvironmentID != right.EnvironmentID {
		return strings.Compare(string(left.EnvironmentID), string(right.EnvironmentID)) < 0
	}
	return strings.Compare(string(left.FactID), string(right.FactID)) < 0
}

func recordVersionV1(root, head storedFactV1) continuity.RecordVersion {
	return continuity.RecordVersion{
		ProjectID: root.projectID,
		Subject:   root.subject,
		Root:      factStampV1(root),
		Head:      factStampV1(head),
	}
}

func optionalSubjectEqualV1(left, right *continuity.SubjectRef) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func copyOptionalSubjectV1(reference *continuity.SubjectRef) *continuity.SubjectRef {
	if reference == nil {
		return nil
	}
	copy := *reference
	return &copy
}

func observationForStoredFactV1(fact storedFactV1) (continuity.Observation, error) {
	switch fact.kind {
	case continuity.FactProjectRegistered:
		payload, err := decodeStoredWireV1[wireProjectRegistrationV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactProjectLabelRevised:
		payload, err := decodeStoredWireV1[wireProjectLabelRevisionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactJournalRecorded:
		payload, err := decodeStoredWireV1[wireJournalRecordedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactJournalCorrectionRecorded:
		payload, err := decodeStoredWireV1[wireJournalCorrectionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactWrapRecorded:
		payload, err := decodeStoredWireV1[wireWrapRecordedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactSparkCaptured:
		payload, err := decodeStoredWireV1[wireSparkCapturedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactSparkDismissed:
		payload, err := decodeStoredWireV1[wireSparkDismissedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactSparkPromotedToIdea:
		payload, err := decodeStoredWireV1[wireSparkPromotionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactIdeaCreated:
		payload, err := decodeStoredWireV1[wireIdeaCreatedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactIdeaRevised:
		payload, err := decodeStoredWireV1[wireIdeaRevisionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactIdeaResolved:
		payload, err := decodeStoredWireV1[wireIdeaResolutionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactIdeaArchived:
		payload, err := decodeStoredWireV1[wireIdeaArchiveV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactIdeaPromotedToExternalReference:
		payload, err := decodeStoredWireV1[wireIdeaPromotionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactDecisionOpened:
		payload, err := decodeStoredWireV1[wireDecisionOpenedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactDecisionResolved:
		payload, err := decodeStoredWireV1[wireDecisionResolutionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactDecisionSuperseded:
		payload, err := decodeStoredWireV1[wireDecisionSupersessionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactExplorationStarted:
		payload, err := decodeStoredWireV1[wireExplorationStartedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactCheckpointRecorded:
		payload, err := decodeStoredWireV1[wireCheckpointRecordedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactFindingRecorded:
		payload, err := decodeStoredWireV1[wireFindingRecordedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactFindingCorrected:
		payload, err := decodeStoredWireV1[wireFindingCorrectionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactFindingRetracted:
		payload, err := decodeStoredWireV1[wireFindingRetractionV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactHandoffRecorded:
		payload, err := decodeStoredWireV1[wireHandoffRecordedV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactExternalReferenceRegistered:
		payload, err := decodeStoredWireV1[wireExternalReferenceRegistrationV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactExternalReferenceAttached:
		payload, err := decodeStoredWireV1[wireExternalReferenceAttachmentV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactExternalReferenceDetached:
		payload, err := decodeStoredWireV1[wireExternalReferenceDetachmentV1](string(fact.content))
		return payload.Observation.domain(), err
	case continuity.FactVerificationEvidenceRecorded:
		payload, err := decodeStoredWireV1[wireVerificationEvidenceV1](string(fact.content))
		return payload.Observation.domain(), err
	default:
		return continuity.Observation{}, corruptFactProblemV1()
	}
}
