package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuitySnapshotProjectsEveryFamilyAndFactKind(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	projectID := seedCompleteSnapshotProjectV1(t, store)

	snapshot, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{AtMillis: 250})
	if err != nil {
		t.Fatalf("Snapshot(): %v", err)
	}
	if snapshot.AtMillis != 250 || snapshot.Project.Identity.Label != "Loaf vNext" {
		t.Fatalf("snapshot identity = %#v", snapshot.Project.Identity)
	}
	if got := len(snapshot.EffectiveJournal.Entries); got != 1 || snapshot.EffectiveJournal.Entries[0].Content.Text != "Keep typed facts." {
		t.Fatalf("effective journal = %#v", snapshot.EffectiveJournal.Entries)
	}
	if got := len(snapshot.LatestWraps.Wraps); got != 2 {
		t.Fatalf("latest wraps = %d, want 2", got)
	}
	projectWrap := projectWrapV1(t, snapshot.LatestWraps.Wraps)
	if projectWrap.Synthesis != "Project wrap." {
		t.Fatalf("project wrap = %#v", projectWrap)
	}
	if got := len(snapshot.ActiveSparks.Sparks); got != 1 || snapshot.ActiveSparks.Sparks[0].Text != "Active spark." {
		t.Fatalf("active sparks = %#v", snapshot.ActiveSparks.Sparks)
	}
	if got := len(snapshot.CurrentIdeas.Ideas); got != 4 {
		t.Fatalf("current ideas = %d, want 4", got)
	}
	assertIdeaDispositionsV1(t, snapshot.CurrentIdeas.Ideas, map[continuity.SubjectID]continuity.IdeaDisposition{
		"idea-active":   continuity.IdeaActive,
		"idea-archived": continuity.IdeaArchived,
		"idea-promoted": continuity.IdeaPromoted,
		"idea-resolved": continuity.IdeaResolved,
	})
	if got := len(snapshot.CurrentDecisions.Decisions); got != 2 {
		t.Fatalf("current decisions = %d, want 2", got)
	}
	sourceDecision := decisionByIDV1(t, snapshot.CurrentDecisions.Decisions, "decision-source")
	if sourceDecision.State != continuity.DecisionSuperseded || sourceDecision.Resolution != "SQLite" || sourceDecision.ResolutionRationale != "Local and exact." || sourceDecision.SupersessionRationale != "A later question." || sourceDecision.SuccessorID != "decision-successor" {
		t.Fatalf("source decision = %#v", sourceDecision)
	}
	if sourceDecision.ResolutionStamp.FactID != "fact-decision-resolved" || sourceDecision.Record.Head.FactID != "fact-decision-superseded" {
		t.Fatalf("decision provenance = %#v", sourceDecision)
	}
	if got := len(snapshot.Explorations.Explorations); got != 1 || snapshot.Explorations.Explorations[0].Record.Subject.ID != "exploration-1" {
		t.Fatalf("explorations = %#v", snapshot.Explorations.Explorations)
	}
	if got := len(snapshot.LatestCheckpoints.Checkpoints); got != 1 || snapshot.LatestCheckpoints.Checkpoints[0].Record.Subject.ID != "checkpoint-latest" {
		t.Fatalf("latest checkpoints = %#v", snapshot.LatestCheckpoints.Checkpoints)
	}
	if got := len(snapshot.CurrentFindings.Findings); got != 2 {
		t.Fatalf("current findings = %d, want 2", got)
	}
	retracted := findingByIDV1(t, snapshot.CurrentFindings.Findings, "finding-retracted")
	if retracted.State != continuity.FindingRetracted || retracted.Content.Summary != "Exact canonical bytes matter." || retracted.ContentStamp.FactID != "fact-finding-corrected" || retracted.Record.Head.FactID != "fact-finding-retracted" {
		t.Fatalf("retracted finding = %#v", retracted)
	}
	if got := len(snapshot.LatestHandoffs.Handoffs); got != 2 {
		t.Fatalf("latest handoffs = %d, want 2", got)
	}
	if got := len(snapshot.ExternalReferences.References); got != 2 {
		t.Fatalf("external references = %d, want 2", got)
	}
	reference := referenceByIDV1(t, snapshot.ExternalReferences.References, "reference-1")
	if len(reference.Attachments) != 1 || reference.Attachments[0].Target != (continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-active"}) {
		t.Fatalf("active reference attachments = %#v", reference.Attachments)
	}
	if got := len(snapshot.VerificationEvidence.Evidence); got != 2 {
		t.Fatalf("verification evidence = %d, want 2", got)
	}
	if snapshot.EffectiveJournal.Entries == nil || snapshot.LatestWraps.Wraps == nil || snapshot.ActiveSparks.Sparks == nil || snapshot.CurrentIdeas.Ideas == nil || snapshot.CurrentDecisions.Decisions == nil || snapshot.Explorations.Explorations == nil || snapshot.LatestCheckpoints.Checkpoints == nil || snapshot.CurrentFindings.Findings == nil || snapshot.LatestHandoffs.Handoffs == nil || snapshot.ExternalReferences.References == nil || snapshot.VerificationEvidence.Evidence == nil {
		t.Fatal("successful Snapshot returned a nil collection")
	}

	var distinctKinds int
	if err := store.db.QueryRow(`SELECT COUNT(DISTINCT fact_kind) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&distinctKinds); err != nil {
		t.Fatalf("count fact kinds: %v", err)
	}
	if distinctKinds != len(continuity.FactCatalog()) {
		t.Fatalf("distinct fact kinds = %d, want %d", distinctKinds, len(continuity.FactCatalog()))
	}
}

func TestContinuitySnapshotUsesStableFactOrder(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-order")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(900, "main"), Label: "Loaf"}))
	first := mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal-first", "journal-first", continuity.JournalRecordedPayload{Observation: snapshotObservationV1(900, "later-observation"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "First root."}}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal-second", "journal-second", continuity.JournalRecordedPayload{Observation: snapshotObservationV1(1, "earlier-observation"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "Second root."}}))
	mustAppendV1(t)(store.CorrectJournalEntry(ctx, projectID, "fact-journal-correction", "journal-first", continuity.JournalCorrectionPayload{Observation: snapshotObservationV1(1000, "correction"), Corrects: first.FactID, Content: continuity.JournalContent{Category: continuity.JournalDecision, Text: "Corrected first."}}))
	before := mustSnapshotV1(t, store, projectID, 199)
	if got := []continuity.SubjectID{before.EffectiveJournal.Entries[0].Record.Subject.ID, before.EffectiveJournal.Entries[1].Record.Subject.ID}; !reflect.DeepEqual(got, []continuity.SubjectID{"journal-second", "journal-first"}) {
		t.Fatalf("journal order = %v, want recording-root recency", got)
	}
	if before.EffectiveJournal.Entries[1].Content.Text != "Corrected first." {
		t.Fatalf("corrected entry moved or lost content: %#v", before.EffectiveJournal.Entries)
	}
	repeated := mustSnapshotV1(t, store, projectID, 199)
	if !reflect.DeepEqual(repeated, before) {
		t.Fatalf("repeated snapshots differ:\nfirst=%#v\nsecond=%#v", before, repeated)
	}
}

func TestContinuitySnapshotRejectsInvalidRequestsAndCorruptRows(t *testing.T) {
	t.Parallel()

	var nilStore *Store
	_, err := nilStore.Snapshot(context.Background(), "project-1", continuity.SnapshotRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemStoreClosed)

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	_, err = store.Snapshot(nil, "project-1", continuity.SnapshotRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemInvalid)
	_, err = store.Snapshot(context.Background(), "project-1", continuity.SnapshotRequest{AtMillis: -1})
	assertProblemCodeV1(t, err, continuity.ProblemInvalid)
	_, err = store.Snapshot(context.Background(), "missing-project", continuity.SnapshotRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemProjectNotRegistered)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Snapshot(ctx, "project-1", continuity.SnapshotRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Snapshot error = %v, want context.Canceled", err)
	}

	projectID := continuity.ProjectID("project-corrupt")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))
	if _, err := store.db.Exec(`UPDATE continuity_facts SET content_json = content_json || ' ' WHERE fact_id = 'fact-project'`); err != nil {
		t.Fatalf("tamper canonical content: %v", err)
	}
	_, err = store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemCorruptFact)
}

func seedCompleteSnapshotProjectV1(t *testing.T, store *Store) continuity.ProjectID {
	t.Helper()
	return seedCompleteSnapshotProjectWithIDV1(t, store, "project-snapshot")
}

func seedCompleteSnapshotProjectWithIDV1(t *testing.T, store *Store, projectID continuity.ProjectID) continuity.ProjectID {
	t.Helper()

	ctx := context.Background()
	observation := snapshotObservationV1(1, "issue/loaf-96")
	registration := mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	mustAppendV1(t)(store.ReviseProjectLabel(ctx, projectID, "fact-project-revised", continuity.ProjectLabelRevisionPayload{Observation: observation, Revises: registration.FactID, Label: "Loaf vNext"}))

	journal := mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal", "journal-1", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalDiscover, Scope: "continuity", Text: "Typed facts."}}))
	mustAppendV1(t)(store.CorrectJournalEntry(ctx, projectID, "fact-journal-corrected", "journal-1", continuity.JournalCorrectionPayload{Observation: observation, Corrects: journal.FactID, Content: continuity.JournalContent{Category: continuity.JournalDecision, Scope: "continuity", Text: "Keep typed facts."}}))
	journalFocus := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}
	mustAppendV1(t)(store.RecordWrap(ctx, projectID, "fact-wrap-project-old", "wrap-project-old", continuity.WrapRecordedPayload{Observation: observation, Scope: "not-a-key", Synthesis: "Old project wrap."}))
	mustAppendV1(t)(store.RecordWrap(ctx, projectID, "fact-wrap-project", "wrap-project", continuity.WrapRecordedPayload{Observation: observation, Scope: "anything", Synthesis: "Project wrap."}))
	mustAppendV1(t)(store.RecordWrap(ctx, projectID, "fact-wrap-focus", "wrap-focus", continuity.WrapRecordedPayload{Observation: observation, Focus: &journalFocus, Scope: "project", Synthesis: "Focused wrap."}))

	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference", "reference-1", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:LOAF-96"}))
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference-duplicate", "reference-duplicate", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:LOAF-96"}))

	mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-active", "idea-active", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Active idea"}}))
	resolvedIdea := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-resolved-root", "idea-resolved", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Resolve me"}}))
	resolvedIdeaRevision := mustAppendV1(t)(store.ReviseIdea(ctx, projectID, "fact-idea-resolved-revised", "idea-resolved", continuity.IdeaRevisionPayload{Observation: observation, Revises: resolvedIdea.FactID, Content: continuity.IdeaContent{Label: "Resolved idea"}}))
	mustAppendV1(t)(store.ResolveIdea(ctx, projectID, "fact-idea-resolved", "idea-resolved", continuity.IdeaResolutionPayload{Observation: observation, Predecessor: resolvedIdeaRevision.FactID, Resolution: "Done"}))
	archivedIdea := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-archived-root", "idea-archived", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Archive me"}}))
	mustAppendV1(t)(store.ArchiveIdea(ctx, projectID, "fact-idea-archived", "idea-archived", continuity.IdeaArchivePayload{Observation: observation, Predecessor: archivedIdea.FactID, Reason: "No longer useful"}))
	promotedIdea := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-promoted-root", "idea-promoted", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Promote me"}}))
	mustAppendV1(t)(store.PromoteIdeaToExternalReference(ctx, projectID, "fact-idea-promoted", "idea-promoted", continuity.IdeaPromotionPayload{Observation: observation, Predecessor: promotedIdea.FactID, ReferenceID: "reference-1"}))

	mustAppendV1(t)(store.CaptureSpark(ctx, projectID, "fact-spark-active", "spark-active", continuity.SparkCapturedPayload{Observation: observation, Scope: "continuity", Text: "Active spark."}))
	dismissedSpark := mustAppendV1(t)(store.CaptureSpark(ctx, projectID, "fact-spark-dismiss-root", "spark-dismiss", continuity.SparkCapturedPayload{Observation: observation, Text: "Dismiss me."}))
	mustAppendV1(t)(store.DismissSpark(ctx, projectID, "fact-spark-dismissed", "spark-dismiss", continuity.SparkDismissedPayload{Observation: observation, Predecessor: dismissedSpark.FactID, Reason: "Duplicate"}))
	promotedSpark := mustAppendV1(t)(store.CaptureSpark(ctx, projectID, "fact-spark-promote-root", "spark-promote", continuity.SparkCapturedPayload{Observation: observation, Text: "Promote me."}))
	mustAppendV1(t)(store.PromoteSparkToIdea(ctx, projectID, "fact-spark-promoted", "spark-promote", continuity.SparkPromotionPayload{Observation: observation, Predecessor: promotedSpark.FactID, IdeaID: "idea-active"}))

	openedDecision := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-decision-open", "decision-source", continuity.DecisionOpenedPayload{Observation: observation, Scope: "continuity", Question: "Which store?", Context: "Private facts."}))
	resolvedDecision := mustAppendV1(t)(store.ResolveDecision(ctx, projectID, "fact-decision-resolved", "decision-source", continuity.DecisionResolutionPayload{Observation: observation, Predecessor: openedDecision.FactID, Resolution: "SQLite", Rationale: "Local and exact."}))
	mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-decision-successor", "decision-successor", continuity.DecisionOpenedPayload{Observation: observation, Question: "Which sync envelope?"}))
	mustAppendV1(t)(store.SupersedeDecision(ctx, projectID, "fact-decision-superseded", "decision-source", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: resolvedDecision.FactID, SuccessorID: "decision-successor", Rationale: "A later question."}))

	mustAppendV1(t)(store.StartExploration(ctx, projectID, "fact-exploration", "exploration-1", continuity.ExplorationStartedPayload{Observation: observation, Label: "Continuity", Purpose: "Find the clean boundary."}))
	mustAppendV1(t)(store.RecordCheckpoint(ctx, projectID, "fact-checkpoint-old", "checkpoint-old", continuity.CheckpointRecordedPayload{Observation: observation, ExplorationID: "exploration-1", CurrentFraming: "Old frame.", Conclusions: "Old conclusion.", UnresolvedQuestion: "Old question?", NextAction: "Continue."}))
	mustAppendV1(t)(store.RecordCheckpoint(ctx, projectID, "fact-checkpoint-latest", "checkpoint-latest", continuity.CheckpointRecordedPayload{Observation: observation, ExplorationID: "exploration-1", CurrentFraming: "Facts first.", Conclusions: "SQLite.", UnresolvedQuestion: "Sync envelope?", NextAction: "Implement reads.", Items: []continuity.CheckpointItem{{Kind: continuity.CheckpointEvidence, Text: "Typed append passes."}}}))

	mustAppendV1(t)(store.RecordFinding(ctx, projectID, "fact-finding-current", "finding-current", continuity.FindingRecordedPayload{Observation: observation, Content: continuity.FindingContent{Scope: "continuity", Summary: "Current finding."}}))
	retractedFinding := mustAppendV1(t)(store.RecordFinding(ctx, projectID, "fact-finding-root", "finding-retracted", continuity.FindingRecordedPayload{Observation: observation, Content: continuity.FindingContent{Summary: "Canonical bytes matter."}}))
	correctedFinding := mustAppendV1(t)(store.CorrectFinding(ctx, projectID, "fact-finding-corrected", "finding-retracted", continuity.FindingCorrectionPayload{Observation: observation, Corrects: retractedFinding.FactID, Content: continuity.FindingContent{Summary: "Exact canonical bytes matter."}}))
	mustAppendV1(t)(store.RetractFinding(ctx, projectID, "fact-finding-retracted", "finding-retracted", continuity.FindingRetractionPayload{Observation: observation, Predecessor: correctedFinding.FactID, Reason: "Replaced"}))

	mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff-project-old", "handoff-project-old", continuity.HandoffRecordedPayload{Observation: observation, Purpose: "Old project handoff."}))
	mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff-project", "handoff-project", continuity.HandoffRecordedPayload{Observation: observation, Purpose: "Project handoff."}))
	mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff-focus", "handoff-focus", continuity.HandoffRecordedPayload{Observation: observation, Focus: &journalFocus, Purpose: "Focused handoff.", SuggestedSkills: []string{"implement"}}))

	activeAttachment := mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-reference-attach-active", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-active"}}))
	_ = activeAttachment
	detachedAttachment := mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-reference-attach-detached", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: journalFocus}))
	mustAppendV1(t)(store.DetachExternalReference(ctx, projectID, "fact-reference-detach", "reference-1", continuity.ExternalReferenceDetachmentPayload{Observation: observation, Target: journalFocus, Predecessor: detachedAttachment.FactID, Reason: "No longer linked"}))

	evidencePayload := continuity.VerificationEvidencePayload{Observation: observation, Target: journalFocus, Check: "go test", Method: "native", Outcome: continuity.VerificationPassed, Detail: "Pass."}
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence-1", "evidence-1", evidencePayload))
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence-2", "evidence-2", evidencePayload))
	return projectID
}

func snapshotObservationV1(observedAt int64, branch string) continuity.Observation {
	return continuity.Observation{ObservedAtMillis: observedAt, HarnessSessionID: "conversation-snapshot", Branch: branch, Worktree: "/workspace/loaf"}
}

func mustSnapshotV1(t *testing.T, store *Store, projectID continuity.ProjectID, atMillis int64) continuity.Snapshot {
	t.Helper()
	snapshot, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{AtMillis: atMillis})
	if err != nil {
		t.Fatalf("Snapshot(): %v", err)
	}
	return snapshot
}

func assertIdeaDispositionsV1(t *testing.T, ideas []continuity.Idea, want map[continuity.SubjectID]continuity.IdeaDisposition) {
	t.Helper()
	got := make(map[continuity.SubjectID]continuity.IdeaDisposition, len(ideas))
	for _, idea := range ideas {
		got[idea.Record.Subject.ID] = idea.Disposition
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("idea dispositions = %v, want %v", got, want)
	}
}

func decisionByIDV1(t *testing.T, decisions []continuity.Decision, id continuity.SubjectID) continuity.Decision {
	t.Helper()
	for _, decision := range decisions {
		if decision.Record.Subject.ID == id {
			return decision
		}
	}
	t.Fatalf("decision %s not found", id)
	return continuity.Decision{}
}

func findingByIDV1(t *testing.T, findings []continuity.Finding, id continuity.SubjectID) continuity.Finding {
	t.Helper()
	for _, finding := range findings {
		if finding.Record.Subject.ID == id {
			return finding
		}
	}
	t.Fatalf("finding %s not found", id)
	return continuity.Finding{}
}

func referenceByIDV1(t *testing.T, references []continuity.ExternalReference, id continuity.SubjectID) continuity.ExternalReference {
	t.Helper()
	for _, reference := range references {
		if reference.Record.Subject.ID == id {
			return reference
		}
	}
	t.Fatalf("reference %s not found", id)
	return continuity.ExternalReference{}
}

func projectWrapV1(t *testing.T, wraps []continuity.Wrap) continuity.Wrap {
	t.Helper()
	for _, wrap := range wraps {
		if wrap.Focus == nil {
			return wrap
		}
	}
	t.Fatal("project wrap not found")
	return continuity.Wrap{}
}
