package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuityContextDerivesEveryFixedLayer(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-context", 100)
	projectID := seedCompleteSnapshotProjectV1(t, store)
	focus := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{
		Focus:    &focus,
		Scope:    "continuity",
		Branch:   "issue/loaf-96",
		AtMillis: 250,
	})
	if digest.AtMillis != 250 || digest.Scope != "continuity" || digest.Branch != "issue/loaf-96" || digest.Project.Identity.Label != "Loaf vNext" {
		t.Fatalf("context header = %#v", digest)
	}
	if digest.Focus == nil || digest.Focus == &focus || *digest.Focus != focus {
		t.Fatalf("context focus = %#v, want copied %#v", digest.Focus, focus)
	}
	if got := contextSubjectIDsV1(digest.FocusedJournal.Entries); !reflect.DeepEqual(got, []continuity.SubjectID{"journal-1"}) {
		t.Fatalf("focused journal = %v", got)
	}
	if digest.FocusedJournal.Selection != (continuity.ContextSelection{AvailableCount: 1, ShownCount: 1}) {
		t.Fatalf("focused journal selection = %#v", digest.FocusedJournal.Selection)
	}
	if got := len(digest.ProjectJournal.Entries); got != 0 || digest.ProjectJournal.Entries == nil {
		t.Fatalf("project journal = %#v", digest.ProjectJournal)
	}
	if got := contextSubjectIDsV1(digest.Wraps.Wraps); !reflect.DeepEqual(got, []continuity.SubjectID{"wrap-focus", "wrap-project"}) {
		t.Fatalf("wraps = %v", got)
	}
	if got := contextSubjectIDsV1(digest.Sparks.Sparks); !reflect.DeepEqual(got, []continuity.SubjectID{"spark-active"}) {
		t.Fatalf("sparks = %v", got)
	}
	if got := contextSubjectIDsV1(digest.Ideas.Ideas); !reflect.DeepEqual(got, []continuity.SubjectID{"idea-active"}) {
		t.Fatalf("active ideas = %v", got)
	}
	if got := len(digest.Decisions.Decisions); got != 2 {
		t.Fatalf("decisions = %#v", digest.Decisions)
	}
	if got := contextSubjectIDsV1(digest.Checkpoints.Checkpoints); !reflect.DeepEqual(got, []continuity.SubjectID{"checkpoint-latest"}) {
		t.Fatalf("checkpoints = %v", got)
	}
	if got := len(digest.Findings.Findings); got != 2 {
		t.Fatalf("findings = %#v", digest.Findings)
	}
	if got := contextSubjectIDsV1(digest.Handoffs.Handoffs); !reflect.DeepEqual(got, []continuity.SubjectID{"handoff-focus"}) {
		t.Fatalf("handoffs = %v", got)
	}
	if got := len(digest.ExternalReferences.References); got != 1 || digest.ExternalReferences.References[0].ReferenceID != "reference-1" || len(digest.ExternalReferences.References[0].MatchingAttachments) != 1 || digest.ExternalReferences.References[0].MatchingAttachments[0].Target != (continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-active"}) {
		t.Fatalf("external references = %#v", digest.ExternalReferences)
	}
	if got := len(digest.VerificationEvidence.Evidence); got != 2 {
		t.Fatalf("verification evidence = %#v", digest.VerificationEvidence)
	}
	assertContextCollectionsNonNilV1(t, digest)

	focus.ID = "caller-mutated"
	if digest.Focus.ID != "journal-1" {
		t.Fatalf("returned focus aliases caller: %#v", digest.Focus)
	}
}

func TestContinuityContextOrdersFocusScopeBranchBeforeProjectRemainder(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-selectors", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-selectors")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Selectors"}))

	focus := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-focus"}
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal-focus", focus.ID, continuity.JournalRecordedPayload{
		Observation: snapshotObservationV1(2, "branch-exact"),
		Content:     continuity.JournalContent{Category: continuity.JournalNote, Scope: "scope-exact", Text: "focus"},
	}))
	for index := 0; index < 11; index++ {
		id := continuity.SubjectID(fmt.Sprintf("journal-scope-%02d", index))
		mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, continuity.FactID("fact-"+id), id, continuity.JournalRecordedPayload{
			Observation: snapshotObservationV1(int64(10+index), "other-branch"),
			Content:     continuity.JournalContent{Category: continuity.JournalNote, Scope: "scope-exact", Text: string(id)},
		}))
	}
	for index := 0; index < 2; index++ {
		id := continuity.SubjectID(fmt.Sprintf("journal-branch-%02d", index))
		mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, continuity.FactID("fact-"+id), id, continuity.JournalRecordedPayload{
			Observation: snapshotObservationV1(int64(30+index), "branch-exact"),
			Content:     continuity.JournalContent{Category: continuity.JournalNote, Scope: "other-scope", Text: string(id)},
		}))
	}
	for _, candidate := range []struct {
		id     continuity.SubjectID
		scope  string
		branch string
	}{
		{id: "journal-scope-case", scope: "Scope-Exact", branch: "other-branch"},
		{id: "journal-branch-case", scope: "other-scope", branch: "Branch-Exact"},
	} {
		mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, continuity.FactID("fact-"+candidate.id), candidate.id, continuity.JournalRecordedPayload{
			Observation: snapshotObservationV1(40, candidate.branch),
			Content:     continuity.JournalContent{Category: continuity.JournalNote, Scope: candidate.scope, Text: string(candidate.id)},
		}))
	}

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus, Scope: "scope-exact", Branch: "branch-exact"})
	if got := contextSubjectIDsV1(digest.FocusedJournal.Entries); !reflect.DeepEqual(got, []continuity.SubjectID{"journal-focus"}) {
		t.Fatalf("focused journal = %v", got)
	}
	wantProject := []continuity.SubjectID{
		"journal-scope-10", "journal-scope-09", "journal-scope-08", "journal-scope-07", "journal-scope-06",
		"journal-scope-05", "journal-scope-04", "journal-scope-03", "journal-scope-02", "journal-scope-01",
	}
	if got := contextSubjectIDsV1(digest.ProjectJournal.Entries); !reflect.DeepEqual(got, wantProject) {
		t.Fatalf("project journal precedence = %v, want %v", got, wantProject)
	}
	if digest.ProjectJournal.Selection != (continuity.ContextSelection{AvailableCount: 15, ShownCount: 10, Truncated: true}) {
		t.Fatalf("project journal selection = %#v", digest.ProjectJournal.Selection)
	}
}

func TestContinuityContextRanksOneHopRecordsBeforeCaps(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-one-hop-ranks", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-one-hop-ranks")
	projectRef := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}
	focus := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-focus"}
	scopeTarget := continuity.SubjectRef{Kind: continuity.RecordDecision, ID: "decision-scope"}
	branchTarget := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-branch"}

	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "One-hop ranks"}))
	mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-focus", focus.ID, continuity.IdeaCreatedPayload{Observation: snapshotObservationV1(2, "main"), Content: continuity.IdeaContent{Label: "Focus"}}))
	mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-decision-scope", scopeTarget.ID, continuity.DecisionOpenedPayload{Observation: snapshotObservationV1(3, "main"), Scope: "scope-exact", Question: "Scope?"}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal-branch", branchTarget.ID, continuity.JournalRecordedPayload{Observation: snapshotObservationV1(4, "branch-exact"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "Branch"}}))

	appendReference := func(id continuity.SubjectID, targets ...continuity.SubjectRef) {
		t.Helper()
		mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, continuity.FactID("fact-"+id), id, continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(10, "main"), Locator: "opaque:" + string(id)}))
		for index, target := range targets {
			mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, continuity.FactID(fmt.Sprintf("fact-%s-attach-%d", id, index)), id, continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(11, "main"), Target: target}))
		}
	}
	appendEvidence := func(id continuity.SubjectID, target continuity.SubjectRef) {
		t.Helper()
		mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, continuity.FactID("fact-"+id), id, continuity.VerificationEvidencePayload{Observation: snapshotObservationV1(12, "main"), Target: target, Check: "rank", Method: "test", Outcome: continuity.VerificationPassed, Detail: string(id)}))
	}

	appendReference("reference-focus", focus)
	appendReference("reference-scope", scopeTarget)
	appendReference("reference-branch", branchTarget)
	appendReference("reference-multi", projectRef, focus)
	appendEvidence("evidence-focus", focus)
	appendEvidence("evidence-scope", scopeTarget)
	appendEvidence("evidence-branch", branchTarget)
	for index := 0; index < 10; index++ {
		suffix := fmt.Sprintf("%02d", index)
		appendReference(continuity.SubjectID("reference-project-"+suffix), projectRef)
		appendEvidence(continuity.SubjectID("evidence-project-"+suffix), projectRef)
	}

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus, Scope: "scope-exact", Branch: "branch-exact"})
	wantReferences := []continuity.SubjectID{"reference-multi", "reference-focus", "reference-scope", "reference-branch"}
	if got := contextReferenceIDsV1(digest.ExternalReferences.References); len(got) != 10 || !reflect.DeepEqual(got[:len(wantReferences)], wantReferences) {
		t.Fatalf("ranked references = %v, want prefix %v", got, wantReferences)
	}
	if digest.ExternalReferences.Selection != (continuity.ContextSelection{AvailableCount: 14, ShownCount: 10, Truncated: true}) {
		t.Fatalf("reference selection = %#v", digest.ExternalReferences.Selection)
	}
	multi := digest.ExternalReferences.References[0]
	if len(multi.MatchingAttachments) != 2 || multi.MatchingAttachments[0].Target != focus && multi.MatchingAttachments[1].Target != focus {
		t.Fatalf("multi-target focus reference = %#v", multi)
	}
	wantEvidence := []continuity.SubjectID{"evidence-focus", "evidence-scope", "evidence-branch"}
	if got := contextSubjectIDsV1(digest.VerificationEvidence.Evidence); len(got) != 10 || !reflect.DeepEqual(got[:len(wantEvidence)], wantEvidence) {
		t.Fatalf("ranked evidence = %v, want prefix %v", got, wantEvidence)
	}
	if digest.VerificationEvidence.Selection != (continuity.ContextSelection{AvailableCount: 13, ShownCount: 10, Truncated: true}) {
		t.Fatalf("evidence selection = %#v", digest.VerificationEvidence.Selection)
	}
}

func TestContinuityContextUsesFixedCapsAndTypedTerminalPolicies(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-caps", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-caps")
	projectRef := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Caps"}))

	for index := 0; index < 11; index++ {
		suffix := fmt.Sprintf("%02d", index)
		observation := snapshotObservationV1(int64(10+index), "main")
		mustAppendV1(t)(store.CaptureSpark(ctx, projectID, continuity.FactID("fact-spark-"+suffix), continuity.SubjectID("spark-"+suffix), continuity.SparkCapturedPayload{Observation: observation, Text: suffix}))
		mustAppendV1(t)(store.CreateIdea(ctx, projectID, continuity.FactID("fact-idea-"+suffix), continuity.SubjectID("idea-"+suffix), continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: suffix}}))
		mustAppendV1(t)(store.OpenDecision(ctx, projectID, continuity.FactID("fact-decision-"+suffix), continuity.SubjectID("decision-"+suffix), continuity.DecisionOpenedPayload{Observation: observation, Question: suffix}))
		mustAppendV1(t)(store.RecordFinding(ctx, projectID, continuity.FactID("fact-finding-"+suffix), continuity.SubjectID("finding-"+suffix), continuity.FindingRecordedPayload{Observation: observation, Content: continuity.FindingContent{Summary: suffix}}))
		mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, continuity.FactID("fact-reference-"+suffix), continuity.SubjectID("reference-"+suffix), continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:" + suffix}))
		mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, continuity.FactID("fact-attach-"+suffix), continuity.SubjectID("reference-"+suffix), continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: projectRef}))
		mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, continuity.FactID("fact-evidence-"+suffix), continuity.SubjectID("evidence-"+suffix), continuity.VerificationEvidencePayload{Observation: observation, Target: projectRef, Check: "check", Method: "method", Outcome: continuity.VerificationPassed, Detail: suffix}))
	}
	terminalIdea := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-terminal-root", "idea-terminal", continuity.IdeaCreatedPayload{Observation: snapshotObservationV1(30, "main"), Content: continuity.IdeaContent{Label: "terminal"}}))
	mustAppendV1(t)(store.ResolveIdea(ctx, projectID, "fact-idea-terminal", "idea-terminal", continuity.IdeaResolutionPayload{Observation: snapshotObservationV1(31, "main"), Predecessor: terminalIdea.FactID, Resolution: "done"}))
	resolvedDecision := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-decision-terminal-root", "decision-terminal", continuity.DecisionOpenedPayload{Observation: snapshotObservationV1(32, "main"), Question: "terminal"}))
	mustAppendV1(t)(store.ResolveDecision(ctx, projectID, "fact-decision-terminal", "decision-terminal", continuity.DecisionResolutionPayload{Observation: snapshotObservationV1(33, "main"), Predecessor: resolvedDecision.FactID, Resolution: "done", Rationale: "kept"}))
	retractedFinding := mustAppendV1(t)(store.RecordFinding(ctx, projectID, "fact-finding-terminal-root", "finding-terminal", continuity.FindingRecordedPayload{Observation: snapshotObservationV1(34, "main"), Content: continuity.FindingContent{Summary: "terminal"}}))
	mustAppendV1(t)(store.RetractFinding(ctx, projectID, "fact-finding-terminal", "finding-terminal", continuity.FindingRetractionPayload{Observation: snapshotObservationV1(35, "main"), Predecessor: retractedFinding.FactID, Reason: "kept"}))

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{})
	for name, selection := range map[string]continuity.ContextSelection{
		"sparks":     digest.Sparks.Selection,
		"ideas":      digest.Ideas.Selection,
		"decisions":  digest.Decisions.Selection,
		"findings":   digest.Findings.Selection,
		"references": digest.ExternalReferences.Selection,
		"evidence":   digest.VerificationEvidence.Selection,
	} {
		wantAvailable := 11
		if name == "decisions" || name == "findings" {
			wantAvailable = 12
		}
		if selection != (continuity.ContextSelection{AvailableCount: wantAvailable, ShownCount: 10, Truncated: true}) {
			t.Errorf("%s selection = %#v", name, selection)
		}
	}
	for _, idea := range digest.Ideas.Ideas {
		if idea.Disposition != continuity.IdeaActive {
			t.Fatalf("terminal idea leaked into active context: %#v", idea)
		}
	}
	if digest.ProjectJournal.Entries == nil || digest.Wraps.Wraps == nil || digest.Checkpoints.Checkpoints == nil || digest.Handoffs.Handoffs == nil {
		t.Fatal("empty singleton layers must be non-nil")
	}
}

func TestContinuityContextResolvesOldCheckpointFocusAndDirectAttachments(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-relations", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-relations")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Relations"}))
	mustAppendV1(t)(store.StartExploration(ctx, projectID, "fact-exploration", "exploration-1", continuity.ExplorationStartedPayload{Observation: snapshotObservationV1(2, "main"), Label: "Explore", Purpose: "Context"}))
	mustAppendV1(t)(store.RecordCheckpoint(ctx, projectID, "fact-checkpoint-old", "checkpoint-old", continuity.CheckpointRecordedPayload{Observation: snapshotObservationV1(3, "main"), ExplorationID: "exploration-1", CurrentFraming: "old", Conclusions: "old", UnresolvedQuestion: "old?", NextAction: "continue"}))
	mustAppendV1(t)(store.RecordCheckpoint(ctx, projectID, "fact-checkpoint-latest", "checkpoint-latest", continuity.CheckpointRecordedPayload{Observation: snapshotObservationV1(4, "main"), ExplorationID: "exploration-1", CurrentFraming: "latest", Conclusions: "latest", UnresolvedQuestion: "latest?", NextAction: "resume"}))
	focus := continuity.SubjectRef{Kind: continuity.RecordCheckpoint, ID: "checkpoint-old"}
	mustAppendV1(t)(store.RecordWrap(ctx, projectID, "fact-wrap-focus", "wrap-focus", continuity.WrapRecordedPayload{Observation: snapshotObservationV1(5, "main"), Focus: &focus, Synthesis: "old checkpoint context"}))
	mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff-focus", "handoff-focus", continuity.HandoffRecordedPayload{Observation: snapshotObservationV1(6, "main"), Focus: &focus, Purpose: "resume"}))
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference", "reference-focus", continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(7, "main"), Locator: "opaque:checkpoint"}))
	mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-reference-attach", "reference-focus", continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(8, "main"), Target: focus}))
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence", "evidence-focus", continuity.VerificationEvidencePayload{Observation: snapshotObservationV1(9, "main"), Target: focus, Check: "resume", Method: "manual", Outcome: continuity.VerificationPassed, Detail: "visible"}))
	exploration := continuity.SubjectRef{Kind: continuity.RecordExploration, ID: "exploration-1"}
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference-exploration", "reference-exploration", continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(10, "main"), Locator: "opaque:exploration"}))
	mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-reference-exploration-attach", "reference-exploration", continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(11, "main"), Target: exploration}))
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence-exploration", "evidence-exploration", continuity.VerificationEvidencePayload{Observation: snapshotObservationV1(12, "main"), Target: exploration, Check: "exploration", Method: "manual", Outcome: continuity.VerificationPassed, Detail: "must stay excluded"}))

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus})
	if got := contextSubjectIDsV1(digest.Checkpoints.Checkpoints); !reflect.DeepEqual(got, []continuity.SubjectID{"checkpoint-latest"}) {
		t.Fatalf("old checkpoint focus resolved to %v", got)
	}
	if got := contextSubjectIDsV1(digest.Wraps.Wraps); !reflect.DeepEqual(got, []continuity.SubjectID{"wrap-focus"}) {
		t.Fatalf("focused wraps = %v", got)
	}
	if got := contextSubjectIDsV1(digest.Handoffs.Handoffs); !reflect.DeepEqual(got, []continuity.SubjectID{"handoff-focus"}) {
		t.Fatalf("focused handoff = %v", got)
	}
	if got := len(digest.ExternalReferences.References); got != 1 || len(digest.ExternalReferences.References[0].MatchingAttachments) != 1 {
		t.Fatalf("focused attachment = %#v", digest.ExternalReferences)
	}
	if got := len(digest.VerificationEvidence.Evidence); got != 1 || digest.VerificationEvidence.Evidence[0].Record.Subject.ID != "evidence-focus" {
		t.Fatalf("focused evidence = %#v", digest.VerificationEvidence)
	}
}

func TestContinuityContextDoesNotLeakAttachmentsForCandidatesHiddenByCaps(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-hidden", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-hidden")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Hidden"}))
	for index := 0; index < 11; index++ {
		suffix := fmt.Sprintf("%02d", index)
		mustAppendV1(t)(store.CreateIdea(ctx, projectID, continuity.FactID("fact-idea-"+suffix), continuity.SubjectID("idea-"+suffix), continuity.IdeaCreatedPayload{Observation: snapshotObservationV1(int64(10+index), "main"), Content: continuity.IdeaContent{Label: suffix}}))
	}
	visible := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-10"}
	hidden := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-00"}
	for _, reference := range []struct {
		id      continuity.SubjectID
		targets []continuity.SubjectRef
	}{
		{id: "reference-visible", targets: []continuity.SubjectRef{visible}},
		{id: "reference-hidden", targets: []continuity.SubjectRef{hidden}},
		{id: "reference-mixed", targets: []continuity.SubjectRef{visible, hidden}},
	} {
		mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, continuity.FactID("fact-"+reference.id), reference.id, continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(30, "main"), Locator: "opaque:" + string(reference.id)}))
		for index, target := range reference.targets {
			mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, continuity.FactID(fmt.Sprintf("fact-%s-attach-%d", reference.id, index)), reference.id, continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(31, "main"), Target: target}))
		}
	}
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence-visible", "evidence-visible", continuity.VerificationEvidencePayload{Observation: snapshotObservationV1(32, "main"), Target: visible, Check: "visible", Method: "manual", Outcome: continuity.VerificationPassed, Detail: "visible"}))
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence-hidden", "evidence-hidden", continuity.VerificationEvidencePayload{Observation: snapshotObservationV1(33, "main"), Target: hidden, Check: "hidden", Method: "manual", Outcome: continuity.VerificationPassed, Detail: "hidden"}))

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{})
	if got := contextReferenceIDsV1(digest.ExternalReferences.References); !reflect.DeepEqual(got, []continuity.SubjectID{"reference-mixed", "reference-visible"}) {
		t.Fatalf("selected references = %v", got)
	}
	for _, reference := range digest.ExternalReferences.References {
		if len(reference.MatchingAttachments) != 1 || reference.MatchingAttachments[0].Target != visible {
			t.Fatalf("reference leaked hidden edge: %#v", reference)
		}
	}
	if got := contextSubjectIDsV1(digest.VerificationEvidence.Evidence); !reflect.DeepEqual(got, []continuity.SubjectID{"evidence-visible"}) {
		t.Fatalf("selected evidence = %v", got)
	}
	focus := continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: "reference-mixed"}
	focused := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus})
	if got := contextReferenceIDsV1(focused.ExternalReferences.References); len(got) == 0 || got[0] != "reference-mixed" {
		t.Fatalf("directly focused reference order = %v", got)
	}
	if attachments := focused.ExternalReferences.References[0].MatchingAttachments; len(attachments) != 1 || attachments[0].Target != visible {
		t.Fatalf("directly focused reference leaked capped target: %#v", attachments)
	}
	hiddenReferenceFocus := continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: "reference-hidden"}
	hiddenFocused := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &hiddenReferenceFocus})
	if got := contextReferenceIDsV1(hiddenFocused.ExternalReferences.References); len(got) == 0 || got[0] != "reference-hidden" {
		t.Fatalf("directly focused reference without selected edges = %v", got)
	}
	if attachments := hiddenFocused.ExternalReferences.References[0].MatchingAttachments; attachments == nil || len(attachments) != 0 {
		t.Fatalf("directly focused reference hidden edges = %#v, want non-nil empty", attachments)
	}
}

func TestContinuityContextCannotCapOutDirectExternalReferenceFocus(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-reference-focus-cap", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-reference-focus-cap")
	focus := continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: "reference-focus"}
	wrap := continuity.SubjectRef{Kind: continuity.RecordWrap, ID: "wrap-focus"}
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Reference focus cap"}))
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference-focus", focus.ID, continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(2, "main"), Locator: "opaque:focus"}))
	mustAppendV1(t)(store.RecordWrap(ctx, projectID, "fact-wrap-focus", wrap.ID, continuity.WrapRecordedPayload{Observation: snapshotObservationV1(3, "main"), Focus: &focus, Synthesis: "Focus-derived subject"}))
	for index := 0; index < 11; index++ {
		suffix := fmt.Sprintf("%02d", index)
		referenceID := continuity.SubjectID("reference-inherited-" + suffix)
		mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, continuity.FactID("fact-"+referenceID), referenceID, continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(int64(10+index), "main"), Locator: "opaque:" + string(referenceID)}))
		mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, continuity.FactID("fact-attach-"+referenceID), referenceID, continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(int64(30+index), "main"), Target: wrap}))
	}

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus})
	got := contextReferenceIDsV1(digest.ExternalReferences.References)
	if len(got) != 10 || got[0] != focus.ID || got[1] != "reference-inherited-10" {
		t.Fatalf("focused reference cap order = %v", got)
	}
	if digest.ExternalReferences.Selection != (continuity.ContextSelection{AvailableCount: 12, ShownCount: 10, Truncated: true}) {
		t.Fatalf("focused reference selection = %#v", digest.ExternalReferences.Selection)
	}
	if attachments := digest.ExternalReferences.References[0].MatchingAttachments; attachments == nil || len(attachments) != 0 {
		t.Fatalf("direct focus attachments = %#v, want non-nil empty", attachments)
	}
}

func TestContinuityContextBranchUsesWinningJournalObservation(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-branch", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-winning-branch")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Branch"}))
	corrected := mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal-corrected-root", "journal-corrected", continuity.JournalRecordedPayload{Observation: snapshotObservationV1(2, "recorded-branch"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "before"}}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal-remainder", "journal-remainder", continuity.JournalRecordedPayload{Observation: snapshotObservationV1(3, "other-branch"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "newer remainder"}}))
	mustAppendV1(t)(store.CorrectJournalEntry(ctx, projectID, "fact-journal-corrected-head", "journal-corrected", continuity.JournalCorrectionPayload{Observation: snapshotObservationV1(4, "winning-branch"), Corrects: corrected.FactID, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "after"}}))

	digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Branch: "winning-branch"})
	if got := contextSubjectIDsV1(digest.ProjectJournal.Entries); !reflect.DeepEqual(got, []continuity.SubjectID{"journal-corrected", "journal-remainder"}) {
		t.Fatalf("winning branch precedence = %v", got)
	}
}

func TestContinuityContextRejectsInvalidMissingAndCanceledRequests(t *testing.T) {
	t.Parallel()

	var nilStore *Store
	_, err := nilStore.DeriveContext(context.Background(), "project-1", continuity.ContextRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemStoreClosed)

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-errors", 100)
	_, err = store.DeriveContext(nil, "project-1", continuity.ContextRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemInvalid)
	_, err = store.DeriveContext(context.Background(), "project-1", continuity.ContextRequest{AtMillis: -1})
	assertProblemCodeV1(t, err, continuity.ProblemInvalid)
	_, err = store.DeriveContext(context.Background(), "missing-project", continuity.ContextRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemProjectNotRegistered)

	ctx := context.Background()
	projectID := continuity.ProjectID("project-errors")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Errors"}))
	missing := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-missing"}
	_, err = store.DeriveContext(ctx, projectID, continuity.ContextRequest{Focus: &missing})
	assertContextProblemV1(t, err, continuity.ProblemReferenceNotFound, "focus", "does not identify an existing same-project continuity record")
	wrongProject := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: "some-other-project"}
	_, err = store.DeriveContext(ctx, projectID, continuity.ContextRequest{Focus: &wrongProject})
	assertContextProblemV1(t, err, continuity.ProblemReferenceNotFound, "focus", "does not identify an existing same-project continuity record")
	otherProjectID := continuity.ProjectID("project-errors-other")
	mustAppendV1(t)(store.RegisterProject(ctx, otherProjectID, "fact-other-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(2, "main"), Label: "Other"}))
	mustAppendV1(t)(store.CreateIdea(ctx, otherProjectID, "fact-other-idea", "idea-other", continuity.IdeaCreatedPayload{Observation: snapshotObservationV1(3, "main"), Content: continuity.IdeaContent{Label: "Other"}}))
	otherIdea := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-other"}
	_, err = store.DeriveContext(ctx, projectID, continuity.ContextRequest{Focus: &otherIdea})
	assertContextProblemV1(t, err, continuity.ProblemReferenceNotFound, "focus", "does not identify an existing same-project continuity record")

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.DeriveContext(canceled, projectID, continuity.ContextRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled DeriveContext error = %v, want context.Canceled", err)
	}
}

func TestContinuityContextReportsCorruptionBeforeMissingFocus(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-corrupt-context", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-corrupt-context")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Corrupt context"}))
	if _, err := store.db.Exec(`UPDATE continuity_facts SET content_json = content_json || ' ' WHERE fact_id = 'fact-project'`); err != nil {
		t.Fatalf("tamper canonical content: %v", err)
	}

	missing := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-missing"}
	digest, err := store.DeriveContext(ctx, projectID, continuity.ContextRequest{Focus: &missing})
	assertProblemCodeV1(t, err, continuity.ProblemCorruptFact)
	if !reflect.DeepEqual(digest, continuity.ContextDigest{}) {
		t.Fatalf("corrupt DeriveContext returned partial digest: %#v", digest)
	}
}

func TestContinuityContextAcceptsTerminalAndProjectionHiddenFocus(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-hidden-focus", 100)
	projectID := seedCompleteSnapshotProjectV1(t, store)
	for _, focus := range []continuity.SubjectRef{
		{Kind: continuity.RecordSpark, ID: "spark-dismiss"},
		{Kind: continuity.RecordIdea, ID: "idea-resolved"},
		{Kind: continuity.RecordDecision, ID: "decision-source"},
		{Kind: continuity.RecordFinding, ID: "finding-retracted"},
		{Kind: continuity.RecordCheckpoint, ID: "checkpoint-old"},
	} {
		digest := mustContextV1(t, store, projectID, continuity.ContextRequest{Focus: &focus})
		if digest.Focus == nil || *digest.Focus != focus {
			t.Errorf("focus %v did not survive context: %#v", focus, digest.Focus)
		}
	}
}

func mustContextV1(t *testing.T, store *Store, projectID continuity.ProjectID, request continuity.ContextRequest) continuity.ContextDigest {
	t.Helper()
	digest, err := store.DeriveContext(context.Background(), projectID, request)
	if err != nil {
		t.Fatalf("DeriveContext(): %v", err)
	}
	return digest
}

type contextRecordV1 interface {
	continuity.JournalEntry | continuity.Wrap | continuity.Spark | continuity.Idea | continuity.Decision | continuity.Checkpoint | continuity.Finding | continuity.Handoff | continuity.VerificationEvidence
}

func contextSubjectIDsV1[T contextRecordV1](records []T) []continuity.SubjectID {
	ids := make([]continuity.SubjectID, 0, len(records))
	for _, record := range records {
		switch value := any(record).(type) {
		case continuity.JournalEntry:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Wrap:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Spark:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Idea:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Decision:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Checkpoint:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Finding:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.Handoff:
			ids = append(ids, value.Record.Subject.ID)
		case continuity.VerificationEvidence:
			ids = append(ids, value.Record.Subject.ID)
		}
	}
	return ids
}

func contextReferenceIDsV1(references []continuity.ContextExternalReference) []continuity.SubjectID {
	ids := make([]continuity.SubjectID, 0, len(references))
	for _, reference := range references {
		ids = append(ids, reference.ReferenceID)
	}
	return ids
}

func assertContextCollectionsNonNilV1(t *testing.T, digest continuity.ContextDigest) {
	t.Helper()
	if digest.FocusedJournal.Entries == nil || digest.ProjectJournal.Entries == nil || digest.Wraps.Wraps == nil || digest.Sparks.Sparks == nil || digest.Ideas.Ideas == nil || digest.Decisions.Decisions == nil || digest.Checkpoints.Checkpoints == nil || digest.Findings.Findings == nil || digest.Handoffs.Handoffs == nil || digest.ExternalReferences.References == nil || digest.VerificationEvidence.Evidence == nil {
		t.Fatal("successful DeriveContext returned a nil collection")
	}
}

func assertContextProblemV1(t *testing.T, err error, code continuity.ProblemCode, field, detail string) {
	t.Helper()
	var problem *continuity.Problem
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want continuity problem", err)
	}
	if problem.Code != code || problem.Field != field || problem.Detail != detail {
		t.Fatalf("problem = %#v, want %s at %s: %s", problem, code, field, detail)
	}
}
