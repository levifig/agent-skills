package sqlite

import (
	"sort"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
)

type externalEdgeFoldV1 struct {
	fact   storedFactV1
	target continuity.SubjectRef
	active bool
}

func foldExternalReferencesV1(corpus projectCorpusV1) ([]continuity.ExternalReference, error) {
	references := make([]continuity.ExternalReference, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordExternalReference) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		reference, err := foldExternalReferenceV1(corpus, subject)
		if err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	sort.Slice(references, func(left, right int) bool {
		return factStampLessV1(references[right].Record.Root, references[left].Record.Root)
	})
	return references, nil
}

func foldExternalReferenceV1(corpus projectCorpusV1, subject continuity.SubjectRef) (continuity.ExternalReference, error) {
	root, facts, err := canonicalSubjectFactsV1(corpus, subject)
	if err != nil {
		return continuity.ExternalReference{}, err
	}
	registered, err := decodeStoredWireV1[wireExternalReferenceRegistrationV1](string(root.content))
	if err != nil {
		return continuity.ExternalReference{}, corruptFactProblemV1()
	}

	edges := make(map[continuity.SubjectRef]externalEdgeFoldV1)
	eligible := make(map[continuity.FactID]bool)
	head := root
	for _, fact := range facts {
		if err := corpus.contextErrV1(); err != nil {
			return continuity.ExternalReference{}, err
		}
		if fact.kind == continuity.FactExternalReferenceRegistered {
			continue
		}
		if !storedFactLessV1(root, fact) {
			return continuity.ExternalReference{}, corruptFactProblemV1()
		}
		var target continuity.SubjectRef
		var predecessor continuity.FactID
		active := false
		switch fact.kind {
		case continuity.FactExternalReferenceAttached:
			payload, err := decodeStoredWireV1[wireExternalReferenceAttachmentV1](string(fact.content))
			if err != nil {
				return continuity.ExternalReference{}, corruptFactProblemV1()
			}
			target = payload.Target.domain()
			predecessor = continuity.FactID(payload.Predecessor)
			active = true
		case continuity.FactExternalReferenceDetached:
			payload, err := decodeStoredWireV1[wireExternalReferenceDetachmentV1](string(fact.content))
			if err != nil {
				return continuity.ExternalReference{}, corruptFactProblemV1()
			}
			target = payload.Target.domain()
			predecessor = continuity.FactID(payload.Predecessor)
		default:
			return continuity.ExternalReference{}, corruptFactProblemV1()
		}
		if err := requireEarlierSubjectV1(corpus, fact, target); err != nil {
			return continuity.ExternalReference{}, err
		}
		factEligible, err := eligibleExternalEdgeFactV1(corpus, fact, target, predecessor, eligible)
		if err != nil {
			return continuity.ExternalReference{}, err
		}
		eligible[fact.factID] = factEligible
		if !factEligible {
			continue
		}
		edges[target] = externalEdgeFoldV1{fact: fact, target: target, active: active}
		head = fact
	}

	view := continuity.ExternalReference{
		Record:                recordVersionV1(root, head),
		Locator:               registered.Locator,
		RegisteredObservation: registered.Observation.domain(),
		Attachments:           make([]continuity.ExternalReferenceAttachment, 0, len(edges)),
	}
	for _, edge := range edges {
		if !edge.active {
			continue
		}
		observation, err := observationForStoredFactV1(edge.fact)
		if err != nil {
			return continuity.ExternalReference{}, corruptFactProblemV1()
		}
		view.Attachments = append(view.Attachments, continuity.ExternalReferenceAttachment{
			Target:      edge.target,
			Stamp:       factStampV1(edge.fact),
			Observation: observation,
		})
	}
	sort.Slice(view.Attachments, func(left, right int) bool {
		if view.Attachments[left].Target.Kind != view.Attachments[right].Target.Kind {
			return strings.Compare(string(view.Attachments[left].Target.Kind), string(view.Attachments[right].Target.Kind)) < 0
		}
		return strings.Compare(string(view.Attachments[left].Target.ID), string(view.Attachments[right].Target.ID)) < 0
	})
	headObservation, err := observationForStoredFactV1(head)
	if err != nil {
		return continuity.ExternalReference{}, corruptFactProblemV1()
	}
	view.HeadObservation = headObservation
	return view, nil
}

func eligibleExternalEdgeFactV1(corpus projectCorpusV1, fact storedFactV1, target continuity.SubjectRef, predecessor continuity.FactID, eligible map[continuity.FactID]bool) (bool, error) {
	if predecessor == "" {
		if fact.kind != continuity.FactExternalReferenceAttached {
			return false, corruptFactProblemV1()
		}
		return true, nil
	}
	previous, ok := corpus.byFactID[predecessor]
	if !ok || previous.subject != fact.subject || !storedFactLessV1(previous, fact) {
		return false, corruptFactProblemV1()
	}
	var previousTarget continuity.SubjectRef
	switch previous.kind {
	case continuity.FactExternalReferenceAttached:
		payload, err := decodeStoredWireV1[wireExternalReferenceAttachmentV1](string(previous.content))
		if err != nil {
			return false, corruptFactProblemV1()
		}
		previousTarget = payload.Target.domain()
	case continuity.FactExternalReferenceDetached:
		payload, err := decodeStoredWireV1[wireExternalReferenceDetachmentV1](string(previous.content))
		if err != nil {
			return false, corruptFactProblemV1()
		}
		previousTarget = payload.Target.domain()
	default:
		return false, corruptFactProblemV1()
	}
	if previousTarget != target {
		return false, corruptFactProblemV1()
	}
	if (fact.kind == continuity.FactExternalReferenceAttached && previous.kind != continuity.FactExternalReferenceDetached) ||
		(fact.kind == continuity.FactExternalReferenceDetached && previous.kind != continuity.FactExternalReferenceAttached) {
		return false, corruptFactProblemV1()
	}
	return eligible[predecessor], nil
}

func foldVerificationEvidenceV1(corpus projectCorpusV1) ([]continuity.VerificationEvidence, error) {
	evidence := make([]continuity.VerificationEvidence, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordVerificationEvidence) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		root := corpus.roots[subject]
		payload, err := decodeStoredWireV1[wireVerificationEvidenceV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		target := payload.Target.domain()
		if err := requireEarlierSubjectV1(corpus, root, target); err != nil {
			return nil, err
		}
		evidence = append(evidence, continuity.VerificationEvidence{
			Record:          recordVersionV1(root, root),
			Target:          target,
			Check:           payload.Check,
			Method:          payload.Method,
			Outcome:         continuity.VerificationOutcome(payload.Outcome),
			Detail:          payload.Detail,
			HeadObservation: payload.Observation.domain(),
		})
	}
	sort.Slice(evidence, func(left, right int) bool {
		return factStampLessV1(evidence[right].Record.Root, evidence[left].Record.Root)
	})
	return evidence, nil
}
