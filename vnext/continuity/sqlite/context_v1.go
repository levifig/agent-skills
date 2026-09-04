package sqlite

import (
	"context"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	contextFocusedJournalLimitV1       = 1
	contextProjectJournalLimitV1       = 10
	contextWrapLimitV1                 = 2
	contextSparkLimitV1                = 10
	contextIdeaLimitV1                 = 10
	contextDecisionLimitV1             = 10
	contextCheckpointLimitV1           = 1
	contextFindingLimitV1              = 10
	contextHandoffLimitV1              = 1
	contextExternalReferenceLimitV1    = 10
	contextVerificationEvidenceLimitV1 = 10
)

type contextFocusRelationsV1 struct {
	checkpointExplorationID continuity.SubjectID
}

type contextSubjectRankV1 uint8

const (
	contextRankFocusV1 contextSubjectRankV1 = iota
	contextRankScopeV1
	contextRankBranchV1
	contextRankProjectV1
	contextRankCountV1
)

func resolveContextFocusRelationsV1(ctx context.Context, facts []storedFactV1, focus *continuity.SubjectRef) (contextFocusRelationsV1, error) {
	if focus == nil {
		return contextFocusRelationsV1{}, nil
	}
	found := false
	relations := contextFocusRelationsV1{}
	for _, fact := range facts {
		if err := ctx.Err(); err != nil {
			return contextFocusRelationsV1{}, err
		}
		if fact.subject != *focus {
			continue
		}
		found = true
		if focus.Kind == continuity.RecordCheckpoint && fact.kind == continuity.FactCheckpointRecorded {
			payload, err := decodeStoredWireV1[wireCheckpointRecordedV1](string(fact.content))
			if err != nil {
				return contextFocusRelationsV1{}, corruptFactProblemV1()
			}
			relations.checkpointExplorationID = continuity.SubjectID(payload.ExplorationID)
			return relations, nil
		}
	}
	if !found || (focus.Kind == continuity.RecordCheckpoint && relations.checkpointExplorationID == "") {
		return contextFocusRelationsV1{}, &continuity.Problem{Code: continuity.ProblemReferenceNotFound, Field: "focus", Detail: "does not identify an existing same-project continuity record"}
	}
	return relations, nil
}

func deriveContextDigestV1(ctx context.Context, snapshot continuity.Snapshot, request continuity.ContextRequest, relations contextFocusRelationsV1) (continuity.ContextDigest, error) {
	if ctx == nil {
		return continuity.ContextDigest{}, &continuity.Problem{Code: continuity.ProblemInvalid, Field: "context", Detail: "must not be nil"}
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}

	focusedJournal, projectJournal, err := selectContextJournalV1(ctx, snapshot.EffectiveJournal.Entries, request)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	wraps, err := selectContextWrapsV1(ctx, snapshot.LatestWraps.Wraps, request.Focus)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	sparks, err := selectContextSparksV1(ctx, snapshot.ActiveSparks.Sparks, request)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	ideas, err := selectContextIdeasV1(ctx, snapshot.CurrentIdeas.Ideas, request.Focus)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	decisions, err := selectContextDecisionsV1(ctx, snapshot.CurrentDecisions.Decisions, request)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	checkpoints, err := selectContextCheckpointsV1(ctx, snapshot.LatestCheckpoints.Checkpoints, request.Focus, relations)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	findings, err := selectContextFindingsV1(ctx, snapshot.CurrentFindings.Findings, request)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	handoffs, err := selectContextHandoffsV1(ctx, snapshot.LatestHandoffs.Handoffs, request.Focus)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}

	digest := continuity.ContextDigest{
		Project:        snapshot.Project,
		Focus:          copyOptionalSubjectV1(request.Focus),
		Scope:          request.Scope,
		Branch:         request.Branch,
		AtMillis:       snapshot.AtMillis,
		FocusedJournal: focusedJournal,
		ProjectJournal: projectJournal,
		Wraps:          wraps,
		Sparks:         sparks,
		Ideas:          ideas,
		Decisions:      decisions,
		Checkpoints:    checkpoints,
		Findings:       findings,
		Handoffs:       handoffs,
	}
	selected := contextSelectedSubjectRanksV1(digest, request)
	references, err := selectContextExternalReferencesV1(ctx, snapshot.ExternalReferences.References, request.Focus, selected)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	evidence, err := selectContextVerificationEvidenceV1(ctx, snapshot.VerificationEvidence.Evidence, selected)
	if err != nil {
		return continuity.ContextDigest{}, err
	}
	if err := ctx.Err(); err != nil {
		return continuity.ContextDigest{}, err
	}
	digest.ExternalReferences = references
	digest.VerificationEvidence = evidence
	return digest, nil
}

func selectContextJournalV1(ctx context.Context, entries []continuity.JournalEntry, request continuity.ContextRequest) (continuity.ContextJournalLayer, continuity.ContextJournalLayer, error) {
	focusedCandidates := make([]continuity.JournalEntry, 0, 1)
	selected := make(map[continuity.SubjectRef]struct{})
	if request.Focus != nil && request.Focus.Kind == continuity.RecordJournalEntry {
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return continuity.ContextJournalLayer{}, continuity.ContextJournalLayer{}, err
			}
			if entry.Record.Subject == *request.Focus {
				focusedCandidates = append(focusedCandidates, entry)
				selected[entry.Record.Subject] = struct{}{}
				break
			}
		}
	}

	projectCandidates := make([]continuity.JournalEntry, 0, len(entries)-len(focusedCandidates))
	appendMatches := func(matches func(continuity.JournalEntry) bool) error {
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, exists := selected[entry.Record.Subject]; exists || !matches(entry) {
				continue
			}
			selected[entry.Record.Subject] = struct{}{}
			projectCandidates = append(projectCandidates, entry)
		}
		return nil
	}
	if request.Scope != "" {
		if err := appendMatches(func(entry continuity.JournalEntry) bool { return entry.Content.Scope == request.Scope }); err != nil {
			return continuity.ContextJournalLayer{}, continuity.ContextJournalLayer{}, err
		}
	}
	if request.Branch != "" {
		if err := appendMatches(func(entry continuity.JournalEntry) bool { return entry.HeadObservation.Branch == request.Branch }); err != nil {
			return continuity.ContextJournalLayer{}, continuity.ContextJournalLayer{}, err
		}
	}
	if err := appendMatches(func(continuity.JournalEntry) bool { return true }); err != nil {
		return continuity.ContextJournalLayer{}, continuity.ContextJournalLayer{}, err
	}

	focusedShown := contextShownCountV1(len(focusedCandidates), contextFocusedJournalLimitV1)
	projectShown := contextShownCountV1(len(projectCandidates), contextProjectJournalLimitV1)
	focusedEntries := make([]continuity.JournalEntry, focusedShown)
	copy(focusedEntries, focusedCandidates[:focusedShown])
	projectEntries := make([]continuity.JournalEntry, projectShown)
	copy(projectEntries, projectCandidates[:projectShown])
	return continuity.ContextJournalLayer{Selection: contextSelectionV1(len(focusedCandidates), focusedShown), Entries: focusedEntries}, continuity.ContextJournalLayer{Selection: contextSelectionV1(len(projectCandidates), projectShown), Entries: projectEntries}, nil
}

func selectContextWrapsV1(ctx context.Context, wraps []continuity.Wrap, focus *continuity.SubjectRef) (continuity.ContextWrapLayer, error) {
	candidates := make([]continuity.Wrap, 0, contextWrapLimitV1)
	selected := make(map[continuity.SubjectRef]struct{})
	for _, wrap := range wraps {
		if err := ctx.Err(); err != nil {
			return continuity.ContextWrapLayer{}, err
		}
		if optionalSubjectEqualV1(wrap.Focus, focus) {
			candidates = append(candidates, wrap)
			selected[wrap.Record.Subject] = struct{}{}
		}
	}
	if focus != nil {
		for _, wrap := range wraps {
			if err := ctx.Err(); err != nil {
				return continuity.ContextWrapLayer{}, err
			}
			if _, exists := selected[wrap.Record.Subject]; !exists && wrap.Focus == nil {
				candidates = append(candidates, wrap)
				selected[wrap.Record.Subject] = struct{}{}
			}
		}
	}
	shown := contextShownCountV1(len(candidates), contextWrapLimitV1)
	values := make([]continuity.Wrap, shown)
	copy(values, candidates[:shown])
	return continuity.ContextWrapLayer{Selection: contextSelectionV1(len(candidates), shown), Wraps: values}, nil
}

func selectContextSparksV1(ctx context.Context, sparks []continuity.Spark, request continuity.ContextRequest) (continuity.ContextSparkLayer, error) {
	candidates := make([]continuity.Spark, 0, len(sparks))
	selected := make(map[continuity.SubjectRef]struct{})
	if request.Focus != nil && request.Focus.Kind == continuity.RecordSpark {
		for _, spark := range sparks {
			if err := ctx.Err(); err != nil {
				return continuity.ContextSparkLayer{}, err
			}
			if spark.Record.Subject == *request.Focus {
				candidates = append(candidates, spark)
				selected[spark.Record.Subject] = struct{}{}
				break
			}
		}
	}
	if request.Scope != "" {
		for _, spark := range sparks {
			if err := ctx.Err(); err != nil {
				return continuity.ContextSparkLayer{}, err
			}
			if _, exists := selected[spark.Record.Subject]; !exists && spark.Scope == request.Scope {
				candidates = append(candidates, spark)
				selected[spark.Record.Subject] = struct{}{}
			}
		}
	}
	for _, spark := range sparks {
		if err := ctx.Err(); err != nil {
			return continuity.ContextSparkLayer{}, err
		}
		if _, exists := selected[spark.Record.Subject]; !exists {
			candidates = append(candidates, spark)
			selected[spark.Record.Subject] = struct{}{}
		}
	}
	shown := contextShownCountV1(len(candidates), contextSparkLimitV1)
	values := make([]continuity.Spark, shown)
	copy(values, candidates[:shown])
	return continuity.ContextSparkLayer{Selection: contextSelectionV1(len(candidates), shown), Sparks: values}, nil
}

func selectContextIdeasV1(ctx context.Context, ideas []continuity.Idea, focus *continuity.SubjectRef) (continuity.ContextIdeaLayer, error) {
	candidates := make([]continuity.Idea, 0, len(ideas))
	selected := make(map[continuity.SubjectRef]struct{})
	if focus != nil && focus.Kind == continuity.RecordIdea {
		for _, idea := range ideas {
			if err := ctx.Err(); err != nil {
				return continuity.ContextIdeaLayer{}, err
			}
			if idea.Disposition == continuity.IdeaActive && idea.Record.Subject == *focus {
				candidates = append(candidates, idea)
				selected[idea.Record.Subject] = struct{}{}
				break
			}
		}
	}
	for _, idea := range ideas {
		if err := ctx.Err(); err != nil {
			return continuity.ContextIdeaLayer{}, err
		}
		if _, exists := selected[idea.Record.Subject]; !exists && idea.Disposition == continuity.IdeaActive {
			candidates = append(candidates, idea)
			selected[idea.Record.Subject] = struct{}{}
		}
	}
	shown := contextShownCountV1(len(candidates), contextIdeaLimitV1)
	values := make([]continuity.Idea, shown)
	copy(values, candidates[:shown])
	return continuity.ContextIdeaLayer{Selection: contextSelectionV1(len(candidates), shown), Ideas: values}, nil
}

func selectContextDecisionsV1(ctx context.Context, decisions []continuity.Decision, request continuity.ContextRequest) (continuity.ContextDecisionLayer, error) {
	candidates := make([]continuity.Decision, 0, len(decisions))
	selected := make(map[continuity.SubjectRef]struct{})
	if request.Focus != nil && request.Focus.Kind == continuity.RecordDecision {
		for _, decision := range decisions {
			if err := ctx.Err(); err != nil {
				return continuity.ContextDecisionLayer{}, err
			}
			if decision.Record.Subject == *request.Focus {
				candidates = append(candidates, decision)
				selected[decision.Record.Subject] = struct{}{}
				break
			}
		}
	}
	if request.Scope != "" {
		for _, decision := range decisions {
			if err := ctx.Err(); err != nil {
				return continuity.ContextDecisionLayer{}, err
			}
			if _, exists := selected[decision.Record.Subject]; !exists && decision.Scope == request.Scope {
				candidates = append(candidates, decision)
				selected[decision.Record.Subject] = struct{}{}
			}
		}
	}
	for _, decision := range decisions {
		if err := ctx.Err(); err != nil {
			return continuity.ContextDecisionLayer{}, err
		}
		if _, exists := selected[decision.Record.Subject]; !exists {
			candidates = append(candidates, decision)
			selected[decision.Record.Subject] = struct{}{}
		}
	}
	shown := contextShownCountV1(len(candidates), contextDecisionLimitV1)
	values := make([]continuity.Decision, shown)
	copy(values, candidates[:shown])
	return continuity.ContextDecisionLayer{Selection: contextSelectionV1(len(candidates), shown), Decisions: values}, nil
}

func selectContextCheckpointsV1(ctx context.Context, checkpoints []continuity.Checkpoint, focus *continuity.SubjectRef, relations contextFocusRelationsV1) (continuity.ContextCheckpointLayer, error) {
	candidates := make([]continuity.Checkpoint, 0, contextCheckpointLimitV1)
	var explorationID continuity.SubjectID
	switch {
	case focus != nil && focus.Kind == continuity.RecordExploration:
		explorationID = focus.ID
	case focus != nil && focus.Kind == continuity.RecordCheckpoint:
		explorationID = relations.checkpointExplorationID
	}
	if explorationID != "" {
		for _, checkpoint := range checkpoints {
			if err := ctx.Err(); err != nil {
				return continuity.ContextCheckpointLayer{}, err
			}
			if checkpoint.ExplorationID == explorationID {
				candidates = append(candidates, checkpoint)
				break
			}
		}
	} else if focus == nil || (focus.Kind != continuity.RecordExploration && focus.Kind != continuity.RecordCheckpoint) {
		if len(checkpoints) > 0 {
			candidates = append(candidates, checkpoints[0])
		}
	}
	shown := contextShownCountV1(len(candidates), contextCheckpointLimitV1)
	values := make([]continuity.Checkpoint, shown)
	copy(values, candidates[:shown])
	return continuity.ContextCheckpointLayer{Selection: contextSelectionV1(len(candidates), shown), Checkpoints: values}, nil
}

func selectContextFindingsV1(ctx context.Context, findings []continuity.Finding, request continuity.ContextRequest) (continuity.ContextFindingLayer, error) {
	candidates := make([]continuity.Finding, 0, len(findings))
	selected := make(map[continuity.SubjectRef]struct{})
	if request.Focus != nil && request.Focus.Kind == continuity.RecordFinding {
		for _, finding := range findings {
			if err := ctx.Err(); err != nil {
				return continuity.ContextFindingLayer{}, err
			}
			if finding.Record.Subject == *request.Focus {
				candidates = append(candidates, finding)
				selected[finding.Record.Subject] = struct{}{}
				break
			}
		}
	}
	if request.Scope != "" {
		for _, finding := range findings {
			if err := ctx.Err(); err != nil {
				return continuity.ContextFindingLayer{}, err
			}
			if _, exists := selected[finding.Record.Subject]; !exists && finding.Content.Scope == request.Scope {
				candidates = append(candidates, finding)
				selected[finding.Record.Subject] = struct{}{}
			}
		}
	}
	for _, finding := range findings {
		if err := ctx.Err(); err != nil {
			return continuity.ContextFindingLayer{}, err
		}
		if _, exists := selected[finding.Record.Subject]; !exists {
			candidates = append(candidates, finding)
			selected[finding.Record.Subject] = struct{}{}
		}
	}
	shown := contextShownCountV1(len(candidates), contextFindingLimitV1)
	values := make([]continuity.Finding, shown)
	copy(values, candidates[:shown])
	return continuity.ContextFindingLayer{Selection: contextSelectionV1(len(candidates), shown), Findings: values}, nil
}

func selectContextHandoffsV1(ctx context.Context, handoffs []continuity.Handoff, focus *continuity.SubjectRef) (continuity.ContextHandoffLayer, error) {
	candidates := make([]continuity.Handoff, 0, contextHandoffLimitV1)
	for _, handoff := range handoffs {
		if err := ctx.Err(); err != nil {
			return continuity.ContextHandoffLayer{}, err
		}
		if optionalSubjectEqualV1(handoff.Focus, focus) {
			candidates = append(candidates, handoff)
			break
		}
	}
	shown := contextShownCountV1(len(candidates), contextHandoffLimitV1)
	values := make([]continuity.Handoff, shown)
	copy(values, candidates[:shown])
	return continuity.ContextHandoffLayer{Selection: contextSelectionV1(len(candidates), shown), Handoffs: values}, nil
}

func contextSelectedSubjectRanksV1(digest continuity.ContextDigest, request continuity.ContextRequest) map[continuity.SubjectRef]contextSubjectRankV1 {
	selected := make(map[continuity.SubjectRef]contextSubjectRankV1)
	rankContextSubjectV1(selected, digest.Project.Identity.Record.Subject, contextRankProjectV1)
	if digest.Focus != nil {
		rankContextSubjectV1(selected, *digest.Focus, contextRankFocusV1)
	}
	for _, entry := range digest.FocusedJournal.Entries {
		rankContextSubjectV1(selected, entry.Record.Subject, contextRankFocusV1)
	}
	for _, entry := range digest.ProjectJournal.Entries {
		rank := contextRankProjectV1
		if request.Scope != "" && entry.Content.Scope == request.Scope {
			rank = contextRankScopeV1
		} else if request.Branch != "" && entry.HeadObservation.Branch == request.Branch {
			rank = contextRankBranchV1
		}
		rankContextSubjectV1(selected, entry.Record.Subject, rank)
	}
	for _, wrap := range digest.Wraps.Wraps {
		rank := contextRankProjectV1
		if digest.Focus != nil && optionalSubjectEqualV1(wrap.Focus, digest.Focus) {
			rank = contextRankFocusV1
		}
		rankContextSubjectV1(selected, wrap.Record.Subject, rank)
	}
	for _, spark := range digest.Sparks.Sparks {
		rank := contextRankProjectV1
		if digest.Focus != nil && spark.Record.Subject == *digest.Focus {
			rank = contextRankFocusV1
		} else if request.Scope != "" && spark.Scope == request.Scope {
			rank = contextRankScopeV1
		}
		rankContextSubjectV1(selected, spark.Record.Subject, rank)
	}
	for _, idea := range digest.Ideas.Ideas {
		rank := contextRankProjectV1
		if digest.Focus != nil && idea.Record.Subject == *digest.Focus {
			rank = contextRankFocusV1
		}
		rankContextSubjectV1(selected, idea.Record.Subject, rank)
	}
	for _, decision := range digest.Decisions.Decisions {
		rank := contextRankProjectV1
		if digest.Focus != nil && decision.Record.Subject == *digest.Focus {
			rank = contextRankFocusV1
		} else if request.Scope != "" && decision.Scope == request.Scope {
			rank = contextRankScopeV1
		}
		rankContextSubjectV1(selected, decision.Record.Subject, rank)
	}
	for _, checkpoint := range digest.Checkpoints.Checkpoints {
		rank := contextRankProjectV1
		if digest.Focus != nil && (digest.Focus.Kind == continuity.RecordExploration || digest.Focus.Kind == continuity.RecordCheckpoint) {
			rank = contextRankFocusV1
		}
		rankContextSubjectV1(selected, checkpoint.Record.Subject, rank)
	}
	for _, finding := range digest.Findings.Findings {
		rank := contextRankProjectV1
		if digest.Focus != nil && finding.Record.Subject == *digest.Focus {
			rank = contextRankFocusV1
		} else if request.Scope != "" && finding.Content.Scope == request.Scope {
			rank = contextRankScopeV1
		}
		rankContextSubjectV1(selected, finding.Record.Subject, rank)
	}
	for _, handoff := range digest.Handoffs.Handoffs {
		rank := contextRankProjectV1
		if digest.Focus != nil && optionalSubjectEqualV1(handoff.Focus, digest.Focus) {
			rank = contextRankFocusV1
		}
		rankContextSubjectV1(selected, handoff.Record.Subject, rank)
	}
	return selected
}

func rankContextSubjectV1(selected map[continuity.SubjectRef]contextSubjectRankV1, subject continuity.SubjectRef, rank contextSubjectRankV1) {
	current, exists := selected[subject]
	if !exists || rank < current {
		selected[subject] = rank
	}
}

func selectContextExternalReferencesV1(ctx context.Context, references []continuity.ExternalReference, focus *continuity.SubjectRef, selectedSubjects map[continuity.SubjectRef]contextSubjectRankV1) (continuity.ContextExternalReferenceLayer, error) {
	buckets := make([][]continuity.ContextExternalReference, int(contextRankCountV1))
	selectedReferences := make(map[continuity.SubjectRef]struct{})
	if focus != nil && focus.Kind == continuity.RecordExternalReference {
		for _, reference := range references {
			if err := ctx.Err(); err != nil {
				return continuity.ContextExternalReferenceLayer{}, err
			}
			if reference.Record.Subject == *focus {
				attachments, _, _, err := matchingContextAttachmentsV1(ctx, reference.Attachments, selectedSubjects)
				if err != nil {
					return continuity.ContextExternalReferenceLayer{}, err
				}
				buckets[contextRankFocusV1] = append(buckets[contextRankFocusV1], contextExternalReferenceV1(reference, attachments))
				selectedReferences[reference.Record.Subject] = struct{}{}
				break
			}
		}
	}
	for _, reference := range references {
		if err := ctx.Err(); err != nil {
			return continuity.ContextExternalReferenceLayer{}, err
		}
		if _, exists := selectedReferences[reference.Record.Subject]; exists {
			continue
		}
		matching, rank, matched, err := matchingContextAttachmentsV1(ctx, reference.Attachments, selectedSubjects)
		if err != nil {
			return continuity.ContextExternalReferenceLayer{}, err
		}
		if !matched {
			continue
		}
		buckets[rank] = append(buckets[rank], contextExternalReferenceV1(reference, matching))
		selectedReferences[reference.Record.Subject] = struct{}{}
	}
	candidates := make([]continuity.ContextExternalReference, 0, len(references))
	for rank := contextRankFocusV1; rank < contextRankCountV1; rank++ {
		candidates = append(candidates, buckets[rank]...)
	}
	shown := contextShownCountV1(len(candidates), contextExternalReferenceLimitV1)
	values := make([]continuity.ContextExternalReference, shown)
	copy(values, candidates[:shown])
	return continuity.ContextExternalReferenceLayer{Selection: contextSelectionV1(len(candidates), shown), References: values}, nil
}

func matchingContextAttachmentsV1(ctx context.Context, attachments []continuity.ExternalReferenceAttachment, selectedSubjects map[continuity.SubjectRef]contextSubjectRankV1) ([]continuity.ExternalReferenceAttachment, contextSubjectRankV1, bool, error) {
	matching := make([]continuity.ExternalReferenceAttachment, 0, len(attachments))
	bestRank := contextRankProjectV1
	matched := false
	for _, attachment := range attachments {
		if err := ctx.Err(); err != nil {
			return nil, 0, false, err
		}
		if rank, selected := selectedSubjects[attachment.Target]; selected {
			matching = append(matching, attachment)
			if !matched || rank < bestRank {
				bestRank = rank
			}
			matched = true
		}
	}
	return matching, bestRank, matched, nil
}

func contextExternalReferenceV1(reference continuity.ExternalReference, attachments []continuity.ExternalReferenceAttachment) continuity.ContextExternalReference {
	if attachments == nil {
		attachments = []continuity.ExternalReferenceAttachment{}
	}
	return continuity.ContextExternalReference{
		ProjectID:             reference.Record.ProjectID,
		ReferenceID:           reference.Record.Subject.ID,
		Registration:          reference.Record.Root,
		Locator:               reference.Locator,
		RegisteredObservation: reference.RegisteredObservation,
		MatchingAttachments:   attachments,
	}
}

func selectContextVerificationEvidenceV1(ctx context.Context, evidence []continuity.VerificationEvidence, selectedSubjects map[continuity.SubjectRef]contextSubjectRankV1) (continuity.ContextVerificationEvidenceLayer, error) {
	buckets := make([][]continuity.VerificationEvidence, int(contextRankCountV1))
	for _, record := range evidence {
		if err := ctx.Err(); err != nil {
			return continuity.ContextVerificationEvidenceLayer{}, err
		}
		if rank, targetSelected := selectedSubjects[record.Target]; targetSelected {
			buckets[rank] = append(buckets[rank], record)
		}
	}
	candidates := make([]continuity.VerificationEvidence, 0, len(evidence))
	for rank := contextRankFocusV1; rank < contextRankCountV1; rank++ {
		candidates = append(candidates, buckets[rank]...)
	}
	shown := contextShownCountV1(len(candidates), contextVerificationEvidenceLimitV1)
	values := make([]continuity.VerificationEvidence, shown)
	copy(values, candidates[:shown])
	return continuity.ContextVerificationEvidenceLayer{Selection: contextSelectionV1(len(candidates), shown), Evidence: values}, nil
}

func contextShownCountV1(available, limit int) int {
	if available < limit {
		return available
	}
	return limit
}

func contextSelectionV1(available, shown int) continuity.ContextSelection {
	return continuity.ContextSelection{AvailableCount: available, ShownCount: shown, Truncated: available > shown}
}
