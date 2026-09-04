package sqlite

import (
	"sort"

	"github.com/levifig/loaf/vnext/continuity"
)

type optionalFocusKeyV1 struct {
	present bool
	kind    continuity.RecordKind
	id      continuity.SubjectID
}

func foldProjectIdentityV1(corpus projectCorpusV1) (continuity.ProjectIdentity, error) {
	subject := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(corpus.projectID)}
	root, facts, err := canonicalSubjectFactsV1(corpus, subject)
	if err != nil {
		return continuity.ProjectIdentity{}, err
	}
	rootPayload, err := decodeStoredWireV1[wireProjectRegistrationV1](string(root.content))
	if err != nil {
		return continuity.ProjectIdentity{}, corruptFactProblemV1()
	}
	head := root
	label := rootPayload.Label
	headObservation := rootPayload.Observation.domain()
	eligible := map[continuity.FactID]bool{root.factID: true}
	for _, fact := range facts {
		if err := corpus.contextErrV1(); err != nil {
			return continuity.ProjectIdentity{}, err
		}
		switch fact.kind {
		case continuity.FactProjectRegistered:
			continue
		case continuity.FactProjectLabelRevised:
			payload, err := decodeStoredWireV1[wireProjectLabelRevisionV1](string(fact.content))
			if err != nil {
				return continuity.ProjectIdentity{}, corruptFactProblemV1()
			}
			allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Revises), eligible, continuity.FactProjectRegistered, continuity.FactProjectLabelRevised)
			if err != nil {
				return continuity.ProjectIdentity{}, err
			}
			eligible[fact.factID] = allowed
			if allowed {
				head = fact
				label = payload.Label
				headObservation = payload.Observation.domain()
			}
		default:
			return continuity.ProjectIdentity{}, corruptFactProblemV1()
		}
	}
	return continuity.ProjectIdentity{
		Record:                recordVersionV1(root, head),
		Label:                 label,
		RegisteredObservation: rootPayload.Observation.domain(),
		HeadObservation:       headObservation,
	}, nil
}

func foldJournalV1(corpus projectCorpusV1) ([]continuity.JournalEntry, error) {
	entries := make([]continuity.JournalEntry, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordJournalEntry) {
		root, facts, err := canonicalSubjectFactsV1(corpus, subject)
		if err != nil {
			return nil, err
		}
		rootPayload, err := decodeStoredWireV1[wireJournalRecordedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		head := root
		content := rootPayload.Content.domain()
		headObservation := rootPayload.Observation.domain()
		eligible := map[continuity.FactID]bool{root.factID: true}
		for _, fact := range facts {
			if err := corpus.contextErrV1(); err != nil {
				return nil, err
			}
			switch fact.kind {
			case continuity.FactJournalRecorded:
				continue
			case continuity.FactJournalCorrectionRecorded:
				payload, err := decodeStoredWireV1[wireJournalCorrectionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Corrects), eligible, continuity.FactJournalRecorded, continuity.FactJournalCorrectionRecorded)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					head = fact
					content = payload.Content.domain()
					headObservation = payload.Observation.domain()
				}
			default:
				return nil, corruptFactProblemV1()
			}
		}
		entries = append(entries, continuity.JournalEntry{
			Record:              recordVersionV1(root, head),
			Content:             content,
			RecordedObservation: rootPayload.Observation.domain(),
			HeadObservation:     headObservation,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return factStampLessV1(entries[right].Record.Root, entries[left].Record.Root)
	})
	return entries, nil
}

func foldWrapsV1(corpus projectCorpusV1) ([]continuity.Wrap, error) {
	winners := make(map[optionalFocusKeyV1]continuity.Wrap)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordWrap) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		root := corpus.roots[subject]
		payload, err := decodeStoredWireV1[wireWrapRecordedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		focus := payload.Focus.domainOptional()
		if focus != nil {
			if err := requireEarlierSubjectV1(corpus, root, *focus); err != nil {
				return nil, err
			}
		}
		wrap := continuity.Wrap{
			Record:          recordVersionV1(root, root),
			Focus:           copyOptionalSubjectV1(focus),
			Scope:           payload.Scope,
			Synthesis:       payload.Synthesis,
			HeadObservation: payload.Observation.domain(),
		}
		key := optionalFocusKeyFromV1(focus)
		current, found := winners[key]
		if !found || factStampLessV1(current.Record.Root, wrap.Record.Root) {
			winners[key] = wrap
		}
	}
	wraps := make([]continuity.Wrap, 0, len(winners))
	for _, wrap := range winners {
		wraps = append(wraps, wrap)
	}
	sort.Slice(wraps, func(left, right int) bool {
		return factStampLessV1(wraps[right].Record.Root, wraps[left].Record.Root)
	})
	return wraps, nil
}

func foldSparksV1(corpus projectCorpusV1) ([]continuity.Spark, error) {
	sparks := make([]continuity.Spark, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordSpark) {
		root, facts, err := canonicalSubjectFactsV1(corpus, subject)
		if err != nil {
			return nil, err
		}
		payload, err := decodeStoredWireV1[wireSparkCapturedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		eligible := map[continuity.FactID]bool{root.factID: true}
		terminalFound := false
		for _, fact := range facts {
			if err := corpus.contextErrV1(); err != nil {
				return nil, err
			}
			switch fact.kind {
			case continuity.FactSparkCaptured:
				continue
			case continuity.FactSparkDismissed:
				dismissal, err := decodeStoredWireV1[wireSparkDismissedV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(dismissal.Predecessor), eligible, continuity.FactSparkCaptured)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					terminalFound = true
				}
			case continuity.FactSparkPromotedToIdea:
				promotion, err := decodeStoredWireV1[wireSparkPromotionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				if err := requireEarlierSubjectV1(corpus, fact, continuity.SubjectRef{Kind: continuity.RecordIdea, ID: continuity.SubjectID(promotion.IdeaID)}); err != nil {
					return nil, err
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(promotion.Predecessor), eligible, continuity.FactSparkCaptured)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					terminalFound = true
				}
			default:
				return nil, corruptFactProblemV1()
			}
		}
		if terminalFound {
			continue
		}
		sparks = append(sparks, continuity.Spark{
			Record:          recordVersionV1(root, root),
			Scope:           payload.Scope,
			Text:            payload.Text,
			HeadObservation: payload.Observation.domain(),
		})
	}
	sort.Slice(sparks, func(left, right int) bool {
		return factStampLessV1(sparks[right].Record.Root, sparks[left].Record.Root)
	})
	return sparks, nil
}

func foldIdeasV1(corpus projectCorpusV1) ([]continuity.Idea, error) {
	ideas := make([]continuity.Idea, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordIdea) {
		root, facts, err := canonicalSubjectFactsV1(corpus, subject)
		if err != nil {
			return nil, err
		}
		rootPayload, err := decodeStoredWireV1[wireIdeaCreatedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		contentFact := root
		content := rootPayload.Content.domain()
		contentObservation := rootPayload.Observation.domain()
		eligible := map[continuity.FactID]bool{root.factID: true}
		var terminal storedFactV1
		terminalFound := false
		for _, fact := range facts {
			if err := corpus.contextErrV1(); err != nil {
				return nil, err
			}
			switch fact.kind {
			case continuity.FactIdeaCreated:
				continue
			case continuity.FactIdeaRevised:
				payload, err := decodeStoredWireV1[wireIdeaRevisionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Revises), eligible, continuity.FactIdeaCreated, continuity.FactIdeaRevised)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					contentFact = fact
					content = payload.Content.domain()
					contentObservation = payload.Observation.domain()
				}
			case continuity.FactIdeaResolved:
				payload, err := decodeStoredWireV1[wireIdeaResolutionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Predecessor), eligible, continuity.FactIdeaCreated, continuity.FactIdeaRevised)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					terminal = fact
					terminalFound = true
				}
			case continuity.FactIdeaArchived:
				payload, err := decodeStoredWireV1[wireIdeaArchiveV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Predecessor), eligible, continuity.FactIdeaCreated, continuity.FactIdeaRevised)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					terminal = fact
					terminalFound = true
				}
			case continuity.FactIdeaPromotedToExternalReference:
				payload, err := decodeStoredWireV1[wireIdeaPromotionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				if err := requireEarlierSubjectV1(corpus, fact, continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: continuity.SubjectID(payload.ReferenceID)}); err != nil {
					return nil, err
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Predecessor), eligible, continuity.FactIdeaCreated, continuity.FactIdeaRevised)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					terminal = fact
					terminalFound = true
				}
			default:
				return nil, corruptFactProblemV1()
			}
		}
		head := contentFact
		idea := continuity.Idea{
			Content:            content,
			ContentStamp:       factStampV1(contentFact),
			Disposition:        continuity.IdeaActive,
			ContentObservation: contentObservation,
			HeadObservation:    contentObservation,
		}
		if terminalFound {
			head = terminal
			observation, err := observationForStoredFactV1(terminal)
			if err != nil {
				return nil, corruptFactProblemV1()
			}
			idea.HeadObservation = observation
			switch terminal.kind {
			case continuity.FactIdeaResolved:
				payload, err := decodeStoredWireV1[wireIdeaResolutionV1](string(terminal.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				idea.Disposition = continuity.IdeaResolved
				idea.Resolution = payload.Resolution
			case continuity.FactIdeaArchived:
				payload, err := decodeStoredWireV1[wireIdeaArchiveV1](string(terminal.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				idea.Disposition = continuity.IdeaArchived
				idea.ArchiveReason = payload.Reason
			case continuity.FactIdeaPromotedToExternalReference:
				payload, err := decodeStoredWireV1[wireIdeaPromotionV1](string(terminal.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				idea.Disposition = continuity.IdeaPromoted
				idea.ExternalReferenceID = continuity.SubjectID(payload.ReferenceID)
			default:
				return nil, corruptFactProblemV1()
			}
		}
		idea.Record = recordVersionV1(root, head)
		ideas = append(ideas, idea)
	}
	sort.Slice(ideas, func(left, right int) bool {
		return factStampLessV1(ideas[right].Record.Head, ideas[left].Record.Head)
	})
	return ideas, nil
}

func foldDecisionsV1(corpus projectCorpusV1) ([]continuity.Decision, error) {
	decisions := make([]continuity.Decision, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordDecision) {
		root, facts, err := canonicalSubjectFactsV1(corpus, subject)
		if err != nil {
			return nil, err
		}
		opened, err := decodeStoredWireV1[wireDecisionOpenedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		eligible := map[continuity.FactID]bool{root.factID: true}
		var resolution storedFactV1
		resolutionFound := false
		var supersession storedFactV1
		supersessionFound := false
		for _, fact := range facts {
			if err := corpus.contextErrV1(); err != nil {
				return nil, err
			}
			switch fact.kind {
			case continuity.FactDecisionOpened:
				continue
			case continuity.FactDecisionResolved:
				payload, err := decodeStoredWireV1[wireDecisionResolutionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Predecessor), eligible, continuity.FactDecisionOpened)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					resolution = fact
					resolutionFound = true
				}
			case continuity.FactDecisionSuperseded:
				payload, err := decodeStoredWireV1[wireDecisionSupersessionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				successor := continuity.SubjectRef{Kind: continuity.RecordDecision, ID: continuity.SubjectID(payload.SuccessorID)}
				if successor == subject {
					return nil, corruptFactProblemV1()
				}
				if err := requireEarlierSubjectV1(corpus, fact, successor); err != nil {
					return nil, err
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Predecessor), eligible, continuity.FactDecisionOpened, continuity.FactDecisionResolved)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					supersession = fact
					supersessionFound = true
				}
			default:
				return nil, corruptFactProblemV1()
			}
		}
		head := root
		decision := continuity.Decision{
			Scope:             opened.Scope,
			Question:          opened.Question,
			Context:           opened.Context,
			State:             continuity.DecisionOpen,
			OpenedObservation: opened.Observation.domain(),
			HeadObservation:   opened.Observation.domain(),
		}
		if resolutionFound {
			payload, err := decodeStoredWireV1[wireDecisionResolutionV1](string(resolution.content))
			if err != nil {
				return nil, corruptFactProblemV1()
			}
			head = resolution
			decision.State = continuity.DecisionResolved
			decision.Resolution = payload.Resolution
			decision.ResolutionRationale = payload.Rationale
			decision.ResolutionStamp = factStampV1(resolution)
			decision.ResolutionObservation = payload.Observation.domain()
			decision.HeadObservation = payload.Observation.domain()
		}
		if supersessionFound {
			payload, err := decodeStoredWireV1[wireDecisionSupersessionV1](string(supersession.content))
			if err != nil {
				return nil, corruptFactProblemV1()
			}
			head = supersession
			decision.State = continuity.DecisionSuperseded
			decision.SuccessorID = continuity.SubjectID(payload.SuccessorID)
			decision.SupersessionRationale = payload.Rationale
			decision.HeadObservation = payload.Observation.domain()
		}
		decision.Record = recordVersionV1(root, head)
		decisions = append(decisions, decision)
	}
	sort.Slice(decisions, func(left, right int) bool {
		return factStampLessV1(decisions[right].Record.Head, decisions[left].Record.Head)
	})
	return decisions, nil
}

func foldExplorationsV1(corpus projectCorpusV1) ([]continuity.Exploration, error) {
	explorations := make([]continuity.Exploration, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordExploration) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		root := corpus.roots[subject]
		payload, err := decodeStoredWireV1[wireExplorationStartedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		explorations = append(explorations, continuity.Exploration{
			Record:          recordVersionV1(root, root),
			Label:           payload.Label,
			Purpose:         payload.Purpose,
			HeadObservation: payload.Observation.domain(),
		})
	}
	sort.Slice(explorations, func(left, right int) bool {
		return factStampLessV1(explorations[right].Record.Root, explorations[left].Record.Root)
	})
	return explorations, nil
}

func foldCheckpointsV1(corpus projectCorpusV1) ([]continuity.Checkpoint, error) {
	winners := make(map[continuity.SubjectID]continuity.Checkpoint)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordCheckpoint) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		root := corpus.roots[subject]
		payload, err := decodeStoredWireV1[wireCheckpointRecordedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		explorationID := continuity.SubjectID(payload.ExplorationID)
		if err := requireEarlierSubjectV1(corpus, root, continuity.SubjectRef{Kind: continuity.RecordExploration, ID: explorationID}); err != nil {
			return nil, err
		}
		items := make([]continuity.CheckpointItem, 0, len(payload.Items))
		for _, item := range payload.Items {
			items = append(items, continuity.CheckpointItem{Kind: continuity.CheckpointItemKind(item.Kind), Text: item.Text})
		}
		checkpoint := continuity.Checkpoint{
			Record:             recordVersionV1(root, root),
			ExplorationID:      explorationID,
			CurrentFraming:     payload.CurrentFraming,
			Conclusions:        payload.Conclusions,
			UnresolvedQuestion: payload.UnresolvedQuestion,
			NextAction:         payload.NextAction,
			Items:              items,
			HeadObservation:    payload.Observation.domain(),
		}
		current, found := winners[explorationID]
		if !found || factStampLessV1(current.Record.Root, checkpoint.Record.Root) {
			winners[explorationID] = checkpoint
		}
	}
	checkpoints := make([]continuity.Checkpoint, 0, len(winners))
	for _, checkpoint := range winners {
		checkpoints = append(checkpoints, checkpoint)
	}
	sort.Slice(checkpoints, func(left, right int) bool {
		return factStampLessV1(checkpoints[right].Record.Root, checkpoints[left].Record.Root)
	})
	return checkpoints, nil
}

func foldFindingsV1(corpus projectCorpusV1) ([]continuity.Finding, error) {
	findings := make([]continuity.Finding, 0)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordFinding) {
		root, facts, err := canonicalSubjectFactsV1(corpus, subject)
		if err != nil {
			return nil, err
		}
		rootPayload, err := decodeStoredWireV1[wireFindingRecordedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		contentFact := root
		content := rootPayload.Content.domain()
		contentObservation := rootPayload.Observation.domain()
		eligible := map[continuity.FactID]bool{root.factID: true}
		var retraction storedFactV1
		retractionFound := false
		for _, fact := range facts {
			if err := corpus.contextErrV1(); err != nil {
				return nil, err
			}
			switch fact.kind {
			case continuity.FactFindingRecorded:
				continue
			case continuity.FactFindingCorrected:
				payload, err := decodeStoredWireV1[wireFindingCorrectionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Corrects), eligible, continuity.FactFindingRecorded, continuity.FactFindingCorrected)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					contentFact = fact
					content = payload.Content.domain()
					contentObservation = payload.Observation.domain()
				}
			case continuity.FactFindingRetracted:
				payload, err := decodeStoredWireV1[wireFindingRetractionV1](string(fact.content))
				if err != nil {
					return nil, corruptFactProblemV1()
				}
				allowed, err := eligiblePredecessorV1(corpus, fact, continuity.FactID(payload.Predecessor), eligible, continuity.FactFindingRecorded, continuity.FactFindingCorrected)
				if err != nil {
					return nil, err
				}
				eligible[fact.factID] = allowed
				if allowed {
					retraction = fact
					retractionFound = true
				}
			default:
				return nil, corruptFactProblemV1()
			}
		}
		head := contentFact
		finding := continuity.Finding{
			Content:            content,
			ContentStamp:       factStampV1(contentFact),
			State:              continuity.FindingCurrent,
			ContentObservation: contentObservation,
			HeadObservation:    contentObservation,
		}
		if retractionFound {
			payload, err := decodeStoredWireV1[wireFindingRetractionV1](string(retraction.content))
			if err != nil {
				return nil, corruptFactProblemV1()
			}
			head = retraction
			finding.State = continuity.FindingRetracted
			finding.RetractionReason = payload.Reason
			finding.HeadObservation = payload.Observation.domain()
		}
		finding.Record = recordVersionV1(root, head)
		findings = append(findings, finding)
	}
	sort.Slice(findings, func(left, right int) bool {
		return factStampLessV1(findings[right].Record.Head, findings[left].Record.Head)
	})
	return findings, nil
}

func foldHandoffsV1(corpus projectCorpusV1) ([]continuity.Handoff, error) {
	winners := make(map[optionalFocusKeyV1]continuity.Handoff)
	for _, subject := range subjectsOfKindV1(corpus, continuity.RecordHandoff) {
		if err := corpus.contextErrV1(); err != nil {
			return nil, err
		}
		root := corpus.roots[subject]
		payload, err := decodeStoredWireV1[wireHandoffRecordedV1](string(root.content))
		if err != nil {
			return nil, corruptFactProblemV1()
		}
		focus := payload.Focus.domainOptional()
		if focus != nil {
			if err := requireEarlierSubjectV1(corpus, root, *focus); err != nil {
				return nil, err
			}
		}
		skills := append([]string(nil), payload.SuggestedSkills...)
		if skills == nil {
			skills = []string{}
		}
		handoff := continuity.Handoff{
			Record:            recordVersionV1(root, root),
			Focus:             copyOptionalSubjectV1(focus),
			Purpose:           payload.Purpose,
			Situation:         payload.Situation,
			NextActions:       payload.NextActions,
			QuestionsAndRisks: payload.QuestionsAndRisks,
			SuggestedSkills:   skills,
			HeadObservation:   payload.Observation.domain(),
		}
		key := optionalFocusKeyFromV1(focus)
		current, found := winners[key]
		if !found || factStampLessV1(current.Record.Root, handoff.Record.Root) {
			winners[key] = handoff
		}
	}
	handoffs := make([]continuity.Handoff, 0, len(winners))
	for _, handoff := range winners {
		handoffs = append(handoffs, handoff)
	}
	sort.Slice(handoffs, func(left, right int) bool {
		return factStampLessV1(handoffs[right].Record.Root, handoffs[left].Record.Root)
	})
	return handoffs, nil
}

func subjectsOfKindV1(corpus projectCorpusV1, kind continuity.RecordKind) []continuity.SubjectRef {
	subjects := make([]continuity.SubjectRef, 0)
	for subject := range corpus.bySubject {
		if subject.Kind == kind {
			subjects = append(subjects, subject)
		}
	}
	sort.Slice(subjects, func(left, right int) bool {
		leftRoot := corpus.roots[subjects[left]]
		rightRoot := corpus.roots[subjects[right]]
		return storedFactLessV1(leftRoot, rightRoot)
	})
	return subjects
}

func optionalFocusKeyFromV1(focus *continuity.SubjectRef) optionalFocusKeyV1 {
	if focus == nil {
		return optionalFocusKeyV1{}
	}
	return optionalFocusKeyV1{present: true, kind: focus.Kind, id: focus.ID}
}
