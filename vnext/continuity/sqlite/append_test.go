package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuityStoreAdmitsEveryTypedFactKind(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-1")
	observation := appendObservationV1()

	registration := mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	mustAppendV1(t)(store.ReviseProjectLabel(ctx, projectID, "fact-project-label", continuity.ProjectLabelRevisionPayload{Observation: observation, Revises: registration.FactID, Label: "Loaf vNext"}))

	journal := mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal", "journal-1", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalDiscover, Scope: "continuity", Text: "Typed facts."}}))
	mustAppendV1(t)(store.CorrectJournalEntry(ctx, projectID, "fact-journal-correction", "journal-1", continuity.JournalCorrectionPayload{Observation: observation, Corrects: journal.FactID, Content: continuity.JournalContent{Category: continuity.JournalDecision, Scope: "continuity", Text: "Keep typed facts."}}))
	mustAppendV1(t)(store.RecordWrap(ctx, projectID, "fact-wrap", "wrap-1", continuity.WrapRecordedPayload{Observation: observation, Focus: &continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}, Synthesis: "The append contract is coherent."}))

	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference", "reference-1", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:LOAF-96"}))

	mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-target", "idea-target", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Target idea"}}))
	sparkDismiss := mustAppendV1(t)(store.CaptureSpark(ctx, projectID, "fact-spark-dismiss", "spark-dismiss", continuity.SparkCapturedPayload{Observation: observation, Text: "Dismiss me."}))
	mustAppendV1(t)(store.DismissSpark(ctx, projectID, "fact-spark-dismissed", "spark-dismiss", continuity.SparkDismissedPayload{Observation: observation, Predecessor: sparkDismiss.FactID}))
	sparkPromote := mustAppendV1(t)(store.CaptureSpark(ctx, projectID, "fact-spark-promote", "spark-promote", continuity.SparkCapturedPayload{Observation: observation, Text: "Promote me."}))
	mustAppendV1(t)(store.PromoteSparkToIdea(ctx, projectID, "fact-spark-promoted", "spark-promote", continuity.SparkPromotionPayload{Observation: observation, Predecessor: sparkPromote.FactID, IdeaID: "idea-target"}))

	ideaRevise := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-revise", "idea-revise", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Revise me"}}))
	ideaRevised := mustAppendV1(t)(store.ReviseIdea(ctx, projectID, "fact-idea-revised", "idea-revise", continuity.IdeaRevisionPayload{Observation: observation, Revises: ideaRevise.FactID, Content: continuity.IdeaContent{Label: "Revised idea"}}))
	mustAppendV1(t)(store.ResolveIdea(ctx, projectID, "fact-idea-resolved", "idea-revise", continuity.IdeaResolutionPayload{Observation: observation, Predecessor: ideaRevised.FactID, Resolution: "Shipped"}))
	ideaArchive := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-archive", "idea-archive", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Archive me"}}))
	mustAppendV1(t)(store.ArchiveIdea(ctx, projectID, "fact-idea-archived", "idea-archive", continuity.IdeaArchivePayload{Observation: observation, Predecessor: ideaArchive.FactID}))
	ideaPromote := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea-promote", "idea-promote", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Externalize me"}}))
	mustAppendV1(t)(store.PromoteIdeaToExternalReference(ctx, projectID, "fact-idea-promoted", "idea-promote", continuity.IdeaPromotionPayload{Observation: observation, Predecessor: ideaPromote.FactID, ReferenceID: "reference-1"}))

	decisionSource := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-decision-source", "decision-source", continuity.DecisionOpenedPayload{Observation: observation, Question: "Which store?"}))
	decisionResolved := mustAppendV1(t)(store.ResolveDecision(ctx, projectID, "fact-decision-resolved", "decision-source", continuity.DecisionResolutionPayload{Observation: observation, Predecessor: decisionSource.FactID, Resolution: "SQLite"}))
	mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-decision-successor", "decision-successor", continuity.DecisionOpenedPayload{Observation: observation, Question: "Which sync envelope?"}))
	mustAppendV1(t)(store.SupersedeDecision(ctx, projectID, "fact-decision-superseded", "decision-source", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: decisionResolved.FactID, SuccessorID: "decision-successor"}))

	mustAppendV1(t)(store.StartExploration(ctx, projectID, "fact-exploration", "exploration-1", continuity.ExplorationStartedPayload{Observation: observation, Label: "Continuity"}))
	mustAppendV1(t)(store.RecordCheckpoint(ctx, projectID, "fact-checkpoint", "checkpoint-1", continuity.CheckpointRecordedPayload{Observation: observation, ExplorationID: "exploration-1", CurrentFraming: "Facts first.", Conclusions: "SQLite.", UnresolvedQuestion: "Sync envelope?", NextAction: "Implement append."}))

	finding := mustAppendV1(t)(store.RecordFinding(ctx, projectID, "fact-finding", "finding-1", continuity.FindingRecordedPayload{Observation: observation, Content: continuity.FindingContent{Summary: "Canonical bytes matter."}}))
	findingCorrected := mustAppendV1(t)(store.CorrectFinding(ctx, projectID, "fact-finding-corrected", "finding-1", continuity.FindingCorrectionPayload{Observation: observation, Corrects: finding.FactID, Content: continuity.FindingContent{Summary: "Exact canonical bytes matter."}}))
	mustAppendV1(t)(store.RetractFinding(ctx, projectID, "fact-finding-retracted", "finding-1", continuity.FindingRetractionPayload{Observation: observation, Predecessor: findingCorrected.FactID}))
	mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff", "handoff-1", continuity.HandoffRecordedPayload{Observation: observation, Purpose: "Continue LOAF-96."}))

	attachment := mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-attached", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}}))
	mustAppendV1(t)(store.DetachExternalReference(ctx, projectID, "fact-detached", "reference-1", continuity.ExternalReferenceDetachmentPayload{Observation: observation, Target: continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}, Predecessor: attachment.FactID}))
	mustAppendV1(t)(store.RecordVerificationEvidence(ctx, projectID, "fact-evidence", "evidence-1", continuity.VerificationEvidencePayload{Observation: observation, Target: continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}, Check: "go test", Method: "native", Outcome: continuity.VerificationPassed, Detail: "Pass."}))

	var distinctKinds int
	if err := store.db.QueryRow(`SELECT COUNT(DISTINCT fact_kind) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&distinctKinds); err != nil {
		t.Fatalf("count distinct persisted kinds: %v", err)
	}
	if distinctKinds != len(continuity.FactCatalog()) {
		t.Fatalf("distinct persisted kinds = %d, want %d", distinctKinds, len(continuity.FactCatalog()))
	}
	var nonText int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE typeof(content_json) <> 'text'`).Scan(&nonText); err != nil {
		t.Fatalf("inspect content storage class: %v", err)
	}
	if nonText != 0 {
		t.Fatalf("facts with non-TEXT canonical content = %d", nonText)
	}
}

func TestContinuityStoreReplaysExactIntentBeforeCurrentState(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	store := openAppendStoreV1(t, stateRoot, "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-replay")
	payload := continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}

	inserted := mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-replay", payload))
	replayed := mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-replay", payload))
	if inserted.Replayed || !replayed.Replayed {
		t.Fatalf("replay flags = inserted %v, replayed %v", inserted.Replayed, replayed.Replayed)
	}
	replayed.Replayed = false
	if replayed != inserted {
		t.Fatalf("replay receipt = %#v, inserted = %#v", replayed, inserted)
	}

	_, err := store.RegisterProject(ctx, projectID, "fact-replay", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Different"})
	assertProblemCodeV1(t, err, continuity.ProblemFactConflict)

	otherEnvironment := openAppendStoreV1(t, stateRoot, "environment-b", 100)
	crossEnvironmentReplay := mustAppendV1(t)(otherEnvironment.RegisterProject(ctx, projectID, "fact-replay", payload))
	if !crossEnvironmentReplay.Replayed || crossEnvironmentReplay.EnvironmentID != "environment-a" {
		t.Fatalf("cross-environment replay = %#v, want original environment receipt", crossEnvironmentReplay)
	}
	crossEnvironmentReplay.Replayed = false
	if crossEnvironmentReplay != inserted {
		t.Fatalf("cross-environment receipt = %#v, inserted = %#v", crossEnvironmentReplay, inserted)
	}

	var rows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE fact_id = 'fact-replay'`).Scan(&rows); err != nil {
		t.Fatalf("count replay rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("replay row count = %d, want 1", rows)
	}
}

func TestContinuityStoreRejectsReplayAgainstTamperedCanonicalContent(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-tampered-replay")
	payload := continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", payload))
	if _, err := store.db.Exec(`UPDATE continuity_facts SET content_json = content_json || ' ' WHERE fact_id = 'fact-project'`); err != nil {
		t.Fatalf("tamper canonical content: %v", err)
	}

	_, err := store.RegisterProject(ctx, projectID, "fact-project", payload)
	assertProblemCodeV1(t, err, continuity.ProblemCorruptFact)
}

func TestContinuityStoreNormalizesNilAndEmptyListsForReplay(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-lists")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))

	nilPayload := continuity.HandoffRecordedPayload{Observation: appendObservationV1(), Purpose: "Continue."}
	emptyPayload := nilPayload
	emptyPayload.SuggestedSkills = []string{}
	mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff", "handoff-1", nilPayload))
	replayed := mustAppendV1(t)(store.RecordHandoff(ctx, projectID, "fact-handoff", "handoff-1", emptyPayload))
	if !replayed.Replayed {
		t.Fatal("nil-to-empty retry was not recognized as an exact replay")
	}

	var content string
	if err := store.db.QueryRow(`SELECT content_json FROM continuity_facts WHERE fact_id = 'fact-handoff'`).Scan(&content); err != nil {
		t.Fatalf("read handoff content: %v", err)
	}
	if want := `"suggested_skills":[]`; !containsStringV1(content, want) {
		t.Fatalf("stored handoff content = %s, want %s", content, want)
	}
}

func TestContinuityStoreRejectsReuseOfSingleFactSubjects(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-single-facts")
	observation := appendObservationV1()
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-target", "journal-target", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "Target."}}))
	mustAppendV1(t)(store.StartExploration(ctx, projectID, "fact-exploration", "exploration-1", continuity.ExplorationStartedPayload{Observation: observation, Label: "Continuity"}))
	target := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-target"}

	for _, test := range []struct {
		name   string
		slug   string
		append func(continuity.FactID) (continuity.AppendReceipt, error)
	}{
		{name: "wrap", slug: "wrap", append: func(factID continuity.FactID) (continuity.AppendReceipt, error) {
			return store.RecordWrap(ctx, projectID, factID, "wrap-1", continuity.WrapRecordedPayload{Observation: observation, Focus: &target, Synthesis: "Checkpoint."})
		}},
		{name: "checkpoint", slug: "checkpoint", append: func(factID continuity.FactID) (continuity.AppendReceipt, error) {
			return store.RecordCheckpoint(ctx, projectID, factID, "checkpoint-1", continuity.CheckpointRecordedPayload{Observation: observation, ExplorationID: "exploration-1", CurrentFraming: "Frame.", Conclusions: "Conclusion.", UnresolvedQuestion: "Question?", NextAction: "Continue."})
		}},
		{name: "handoff", slug: "handoff", append: func(factID continuity.FactID) (continuity.AppendReceipt, error) {
			return store.RecordHandoff(ctx, projectID, factID, "handoff-1", continuity.HandoffRecordedPayload{Observation: observation, Focus: &target, Purpose: "Continue."})
		}},
		{name: "verification evidence", slug: "verification-evidence", append: func(factID continuity.FactID) (continuity.AppendReceipt, error) {
			return store.RecordVerificationEvidence(ctx, projectID, factID, "evidence-1", continuity.VerificationEvidencePayload{Observation: observation, Target: target, Check: "go test", Method: "native", Outcome: continuity.VerificationPassed, Detail: "Pass."})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mustAppendV1(t)(test.append(continuity.FactID("fact-" + test.slug + "-first")))
			_, err := test.append(continuity.FactID("fact-" + test.slug + "-second"))
			assertProblemCodeV1(t, err, continuity.ProblemSubjectAlreadyRegistered)
		})
	}
}

func TestContinuityStoreEnforcesCausalAndTerminalState(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-causal")
	observation := appendObservationV1()

	_, err := store.CreateIdea(ctx, projectID, "fact-before-project", "idea-before", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Before"}})
	assertProblemCodeV1(t, err, continuity.ProblemProjectNotRegistered)
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	created := mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea", "idea-1", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Idea"}}))

	_, err = store.ReviseIdea(ctx, projectID, "fact-missing-predecessor", "idea-1", continuity.IdeaRevisionPayload{Observation: observation, Revises: "missing", Content: continuity.IdeaContent{Label: "Missing"}})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceNotFound)
	revised := mustAppendV1(t)(store.ReviseIdea(ctx, projectID, "fact-revised", "idea-1", continuity.IdeaRevisionPayload{Observation: observation, Revises: created.FactID, Content: continuity.IdeaContent{Label: "Revised"}}))
	_, err = store.ReviseIdea(ctx, projectID, "fact-stale", "idea-1", continuity.IdeaRevisionPayload{Observation: observation, Revises: created.FactID, Content: continuity.IdeaContent{Label: "Stale"}})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceMismatch)
	mustAppendV1(t)(store.ArchiveIdea(ctx, projectID, "fact-archived", "idea-1", continuity.IdeaArchivePayload{Observation: observation, Predecessor: revised.FactID}))
	_, err = store.ResolveIdea(ctx, projectID, "fact-after-terminal", "idea-1", continuity.IdeaResolutionPayload{Observation: observation, Predecessor: revised.FactID, Resolution: "Too late"})
	assertProblemCodeV1(t, err, continuity.ProblemPreconditionFailed)
}

func TestContinuityStoreExternalReferenceEdgesAreCausal(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-edges")
	observation := appendObservationV1()
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal", "journal-1", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "Target"}}))
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference", "reference-1", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:edge"}))
	target := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}
	attached := mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-attach", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: target}))
	_, err := store.AttachExternalReference(ctx, projectID, "fact-attach-again", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: target})
	assertProblemCodeV1(t, err, continuity.ProblemPreconditionFailed)
	detached := mustAppendV1(t)(store.DetachExternalReference(ctx, projectID, "fact-detach", "reference-1", continuity.ExternalReferenceDetachmentPayload{Observation: observation, Target: target, Predecessor: attached.FactID}))
	_, err = store.AttachExternalReference(ctx, projectID, "fact-reattach-stale", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: target, Predecessor: attached.FactID})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceMismatch)
	mustAppendV1(t)(store.AttachExternalReference(ctx, projectID, "fact-reattach", "reference-1", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: target, Predecessor: detached.FactID}))
}

func TestContinuityStoreScopesSubjectIdentityByFamilyAndAllowsDuplicateOpaqueLocators(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-family-identity")
	observation := appendObservationV1()
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-journal", "shared-subject", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "Journal family."}}))
	mustAppendV1(t)(store.CreateIdea(ctx, projectID, "fact-idea", "shared-subject", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "Idea family."}}))
	_, err := store.RecordJournalEntry(ctx, projectID, "fact-journal-duplicate", "shared-subject", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "Duplicate journal root."}})
	assertProblemCodeV1(t, err, continuity.ProblemSubjectAlreadyRegistered)
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference-1", "reference-1", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:same-locator"}))
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectID, "fact-reference-2", "reference-2", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:same-locator"}))
}

func TestContinuityStoreRejectsCrossProjectAndForbiddenDurableReferences(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	observation := appendObservationV1()
	projectA := continuity.ProjectID("project-reference-a")
	projectB := continuity.ProjectID("project-reference-b")
	mustAppendV1(t)(store.RegisterProject(ctx, projectA, "fact-project-a", continuity.ProjectRegistrationPayload{Observation: observation, Label: "A"}))
	mustAppendV1(t)(store.RegisterProject(ctx, projectB, "fact-project-b", continuity.ProjectRegistrationPayload{Observation: observation, Label: "B"}))
	mustAppendV1(t)(store.RecordJournalEntry(ctx, projectA, "fact-journal-a", "journal-a", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "A-only target."}}))
	mustAppendV1(t)(store.RegisterExternalReference(ctx, projectB, "fact-reference-b", "reference-b", continuity.ExternalReferenceRegistrationPayload{Observation: observation, Locator: "opaque:reference-b"}))

	foreignTarget := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-a"}
	_, err := store.RecordWrap(ctx, projectB, "fact-wrap-foreign", "wrap-foreign", continuity.WrapRecordedPayload{Observation: observation, Focus: &foreignTarget, Synthesis: "Must remain project-local."})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceNotFound)
	_, err = store.AttachExternalReference(ctx, projectB, "fact-attach-foreign", "reference-b", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: foreignTarget})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceNotFound)

	mustAppendV1(t)(store.CreateIdea(ctx, projectB, "fact-idea-b", "idea-b", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "B idea"}}))
	_, err = store.ReviseIdea(ctx, projectB, "fact-revise-foreign", "idea-b", continuity.IdeaRevisionPayload{Observation: observation, Revises: "fact-journal-a", Content: continuity.IdeaContent{Label: "Wrong predecessor"}})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceMismatch)

	_, err = store.AttachExternalReference(ctx, projectB, "fact-reference-chain", "reference-b", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: continuity.SubjectRef{Kind: continuity.RecordExternalReference, ID: "reference-b"}})
	assertProblemCodeV1(t, err, continuity.ProblemInvalid)
	mustAppendV1(t)(store.AttachExternalReference(ctx, projectB, "fact-attach-project", "reference-b", continuity.ExternalReferenceAttachmentPayload{Observation: observation, Target: continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectB)}}))
}

func TestContinuityStoreDecisionSupersessionRejectsCycles(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-decision-cycle")
	observation := appendObservationV1()
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	decisionA := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-a", "decision-a", continuity.DecisionOpenedPayload{Observation: observation, Question: "A?"}))
	decisionB := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-b", "decision-b", continuity.DecisionOpenedPayload{Observation: observation, Question: "B?"}))
	decisionC := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-c", "decision-c", continuity.DecisionOpenedPayload{Observation: observation, Question: "C?"}))
	decisionD := mustAppendV1(t)(store.OpenDecision(ctx, projectID, "fact-d", "decision-d", continuity.DecisionOpenedPayload{Observation: observation, Question: "D?"}))
	_, err := store.SupersedeDecision(ctx, projectID, "fact-self", "decision-d", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: decisionD.FactID, SuccessorID: "decision-d"})
	assertProblemCodeV1(t, err, continuity.ProblemReferenceMismatch)
	resolvedA := mustAppendV1(t)(store.ResolveDecision(ctx, projectID, "fact-a-resolved", "decision-a", continuity.DecisionResolutionPayload{Observation: observation, Predecessor: decisionA.FactID, Resolution: "Use B."}))
	mustAppendV1(t)(store.SupersedeDecision(ctx, projectID, "fact-a-superseded", "decision-a", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: resolvedA.FactID, SuccessorID: "decision-b"}))

	_, err = store.SupersedeDecision(ctx, projectID, "fact-already-superseded-successor", "decision-b", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: decisionB.FactID, SuccessorID: "decision-a"})
	assertProblemCodeV1(t, err, continuity.ProblemPreconditionFailed)
	mustAppendV1(t)(store.SupersedeDecision(ctx, projectID, "fact-b-superseded", "decision-b", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: decisionB.FactID, SuccessorID: "decision-c"}))
	_, err = store.SupersedeDecision(ctx, projectID, "fact-chain-cycle", "decision-c", continuity.DecisionSupersessionPayload{Observation: observation, Predecessor: decisionC.FactID, SuccessorID: "decision-a"})
	assertProblemCodeV1(t, err, continuity.ProblemPreconditionFailed)
}

func TestContinuityStoreSerializesConcurrentSequencesAndReplays(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	ctx := context.Background()
	projectID := continuity.ProjectID("project-concurrent")
	bootstrap := openAppendStoreV1(t, stateRoot, "shared-environment", 500)
	mustAppendV1(t)(bootstrap.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))

	const writers = 12
	stores := make([]*Store, writers)
	for index := range stores {
		stores[index] = openAppendStoreV1(t, stateRoot, "shared-environment", 500)
	}
	start := make(chan struct{})
	receipts := make([]continuity.AppendReceipt, writers)
	errorsByWriter := make([]error, writers)
	var wait sync.WaitGroup
	wait.Add(writers)
	for index := range stores {
		index := index
		go func() {
			defer wait.Done()
			<-start
			receipts[index], errorsByWriter[index] = stores[index].RecordJournalEntry(ctx, projectID, continuity.FactID(fmt.Sprintf("fact-%02d", index)), continuity.SubjectID(fmt.Sprintf("journal-%02d", index)), continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: fmt.Sprintf("entry %d", index)}})
		}()
	}
	close(start)
	wait.Wait()

	sequences := make([]int, 0, writers)
	clocks := make([]continuity.HybridTime, 0, writers)
	for index, err := range errorsByWriter {
		if err != nil {
			t.Fatalf("writer %d: %v", index, err)
		}
		sequences = append(sequences, int(receipts[index].EnvironmentSequence))
		clocks = append(clocks, receipts[index].Clock)
	}
	sort.Ints(sequences)
	for index, sequence := range sequences {
		if want := index + 2; sequence != want {
			t.Fatalf("sorted sequence[%d] = %d, want %d", index, sequence, want)
		}
	}
	sort.Slice(clocks, func(left, right int) bool {
		if clocks[left].WallMillis != clocks[right].WallMillis {
			return clocks[left].WallMillis < clocks[right].WallMillis
		}
		return clocks[left].Logical < clocks[right].Logical
	})
	for index := 1; index < len(clocks); index++ {
		if clocks[index-1] == clocks[index] {
			t.Fatalf("duplicate project HLC %#v", clocks[index])
		}
	}
}

func TestContinuityStoreSerializesConcurrentRetainedFactIdentities(t *testing.T) {
	for _, test := range []struct {
		name          string
		factID        func(int) continuity.FactID
		subjectID     func(int) continuity.SubjectID
		text          func(int) string
		wantSuccesses int
		wantReplays   int
		wantLoserCode continuity.ProblemCode
	}{
		{
			name:          "same intent replays",
			factID:        func(int) continuity.FactID { return "fact-shared" },
			subjectID:     func(int) continuity.SubjectID { return "journal-shared" },
			text:          func(int) string { return "same" },
			wantSuccesses: 8,
			wantReplays:   7,
		},
		{
			name:          "same fact id conflicts",
			factID:        func(int) continuity.FactID { return "fact-shared" },
			subjectID:     func(int) continuity.SubjectID { return "journal-shared" },
			text:          func(index int) string { return fmt.Sprintf("different-%d", index) },
			wantSuccesses: 1,
			wantLoserCode: continuity.ProblemFactConflict,
		},
		{
			name:          "same subject root conflicts",
			factID:        func(index int) continuity.FactID { return continuity.FactID(fmt.Sprintf("fact-%d", index)) },
			subjectID:     func(int) continuity.SubjectID { return "journal-shared" },
			text:          func(index int) string { return fmt.Sprintf("different-%d", index) },
			wantSuccesses: 1,
			wantLoserCode: continuity.ProblemSubjectAlreadyRegistered,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateRoot := filepath.Join(testTempDir(t), "state")
			projectID := continuity.ProjectID("project-concurrent-identity")
			bootstrap := openAppendStoreV1(t, stateRoot, "shared-environment", 500)
			mustAppendV1(t)(bootstrap.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))

			const writers = 8
			stores := make([]*Store, writers)
			for index := range stores {
				stores[index] = openAppendStoreV1(t, stateRoot, "shared-environment", 500)
			}
			start := make(chan struct{})
			receipts := make([]continuity.AppendReceipt, writers)
			errorsByWriter := make([]error, writers)
			var wait sync.WaitGroup
			wait.Add(writers)
			for index := range stores {
				index := index
				go func() {
					defer wait.Done()
					<-start
					receipts[index], errorsByWriter[index] = stores[index].RecordJournalEntry(
						context.Background(),
						projectID,
						test.factID(index),
						test.subjectID(index),
						continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: test.text(index)}},
					)
				}()
			}
			close(start)
			wait.Wait()

			successes := 0
			replays := 0
			for index, err := range errorsByWriter {
				if err == nil {
					successes++
					if receipts[index].Replayed {
						replays++
					}
					continue
				}
				assertProblemCodeV1(t, err, test.wantLoserCode)
			}
			if successes != test.wantSuccesses || replays != test.wantReplays {
				t.Fatalf("successes/replays = %d/%d, want %d/%d", successes, replays, test.wantSuccesses, test.wantReplays)
			}
			next := mustAppendV1(t)(bootstrap.RecordJournalEntry(context.Background(), projectID, "fact-next", "journal-next", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "next"}}))
			if next.EnvironmentSequence != 3 {
				t.Fatalf("next sequence = %d, want 3 after one committed concurrent intent", next.EnvironmentSequence)
			}
		})
	}
}

func TestContinuityStoreSerializesConcurrentRevisionsFromOnePredecessor(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	ctx := context.Background()
	projectID := continuity.ProjectID("project-concurrent-revision")
	bootstrap := openAppendStoreV1(t, stateRoot, "shared-environment", 500)
	mustAppendV1(t)(bootstrap.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	created := mustAppendV1(t)(bootstrap.CreateIdea(ctx, projectID, "fact-idea", "idea-1", continuity.IdeaCreatedPayload{Observation: appendObservationV1(), Content: continuity.IdeaContent{Label: "Original"}}))
	stores := []*Store{
		openAppendStoreV1(t, stateRoot, "shared-environment", 500),
		openAppendStoreV1(t, stateRoot, "shared-environment", 500),
	}

	start := make(chan struct{})
	receipts := make([]continuity.AppendReceipt, len(stores))
	errorsByWriter := make([]error, len(stores))
	var wait sync.WaitGroup
	wait.Add(len(stores))
	for index := range stores {
		index := index
		go func() {
			defer wait.Done()
			<-start
			receipts[index], errorsByWriter[index] = stores[index].ReviseIdea(ctx, projectID, continuity.FactID(fmt.Sprintf("fact-revision-%d", index)), "idea-1", continuity.IdeaRevisionPayload{Observation: appendObservationV1(), Revises: created.FactID, Content: continuity.IdeaContent{Label: fmt.Sprintf("Revision %d", index)}})
		}()
	}
	close(start)
	wait.Wait()

	successes := 0
	for index, err := range errorsByWriter {
		if err == nil {
			successes++
			if receipts[index].EnvironmentSequence != 3 {
				t.Fatalf("winning revision sequence = %d, want 3", receipts[index].EnvironmentSequence)
			}
			continue
		}
		assertProblemCodeV1(t, err, continuity.ProblemReferenceMismatch)
	}
	if successes != 1 {
		t.Fatalf("successful concurrent revisions = %d, want 1", successes)
	}
	next := mustAppendV1(t)(bootstrap.RecordJournalEntry(ctx, projectID, "fact-next", "journal-next", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "next"}}))
	if next.EnvironmentSequence != 4 {
		t.Fatalf("next sequence = %d, want 4 after one committed revision", next.EnvironmentSequence)
	}
}

func TestContinuityStoreSeparatesEnvironmentSequencesWhileOrderingProjectClocks(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	ctx := context.Background()
	projectID := continuity.ProjectID("project-multi-environment")
	bootstrap := openAppendStoreV1(t, stateRoot, "bootstrap-environment", 500)
	mustAppendV1(t)(bootstrap.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	stores := map[continuity.EnvironmentID]*Store{
		"environment-a": openAppendStoreV1(t, stateRoot, "environment-a", 500),
		"environment-b": openAppendStoreV1(t, stateRoot, "environment-b", 500),
	}

	const writesPerEnvironment = 6
	start := make(chan struct{})
	type result struct {
		receipt continuity.AppendReceipt
		err     error
	}
	results := make(chan result, len(stores)*writesPerEnvironment)
	var wait sync.WaitGroup
	for environmentID, store := range stores {
		environmentID, store := environmentID, store
		for index := 0; index < writesPerEnvironment; index++ {
			index := index
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				receipt, err := store.RecordJournalEntry(ctx, projectID, continuity.FactID(fmt.Sprintf("fact-%s-%d", environmentID, index)), continuity.SubjectID(fmt.Sprintf("journal-%s-%d", environmentID, index)), continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "concurrent"}})
				results <- result{receipt: receipt, err: err}
			}()
		}
	}
	close(start)
	wait.Wait()
	close(results)

	sequences := make(map[continuity.EnvironmentID][]int)
	var clocks []continuity.HybridTime
	for result := range results {
		if result.err != nil {
			t.Fatalf("multi-environment append: %v", result.err)
		}
		sequences[result.receipt.EnvironmentID] = append(sequences[result.receipt.EnvironmentID], int(result.receipt.EnvironmentSequence))
		clocks = append(clocks, result.receipt.Clock)
	}
	for environmentID, values := range sequences {
		sort.Ints(values)
		for index, value := range values {
			if want := index + 1; value != want {
				t.Fatalf("%s sequence[%d] = %d, want %d", environmentID, index, value, want)
			}
		}
	}
	sort.Slice(clocks, func(left, right int) bool {
		if clocks[left].WallMillis != clocks[right].WallMillis {
			return clocks[left].WallMillis < clocks[right].WallMillis
		}
		return clocks[left].Logical < clocks[right].Logical
	})
	for index := 1; index < len(clocks); index++ {
		if clocks[index-1].WallMillis > clocks[index].WallMillis || clocks[index-1] == clocks[index] {
			t.Fatalf("project clocks are not strictly ordered: %#v then %#v", clocks[index-1], clocks[index])
		}
	}
}

func TestContinuityStoreContextCancellationWhileWaitingConsumesNothing(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	projectID := continuity.ProjectID("project-context")
	holder := openAppendStoreV1(t, stateRoot, "environment-a", 100)
	waiter := openAppendStoreV1(t, stateRoot, "environment-a", 100)
	mustAppendV1(t)(holder.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))

	tx, err := holder.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatalf("hold immediate transaction: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = waiter.RecordJournalEntry(ctx, projectID, "fact-waited", "journal-waited", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "waited"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked append error = %v, want context deadline", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("release immediate transaction: %v", err)
	}

	receipt := mustAppendV1(t)(waiter.RecordJournalEntry(context.Background(), projectID, "fact-waited", "journal-waited", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "waited"}}))
	if receipt.EnvironmentSequence != 2 {
		t.Fatalf("retry sequence = %d, want 2", receipt.EnvironmentSequence)
	}
}

func TestContinuityStorePreservesCancellationAfterTransactionBegins(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	projectID := continuity.ProjectID("project-mid-transaction-context")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))

	ctx, cancel := context.WithCancel(context.Background())
	store.mu.Lock()
	store.wallMillis = func() int64 {
		cancel()
		return 100
	}
	store.mu.Unlock()
	_, err := store.RecordJournalEntry(ctx, projectID, "fact-canceled", "journal-canceled", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "canceled"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-transaction append error = %v, want context canceled", err)
	}

	store.mu.Lock()
	store.wallMillis = func() int64 { return 101 }
	store.mu.Unlock()
	receipt := mustAppendV1(t)(store.RecordJournalEntry(context.Background(), projectID, "fact-after-cancel", "journal-after-cancel", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "accepted"}}))
	if receipt.EnvironmentSequence != 2 {
		t.Fatalf("post-cancel sequence = %d, want 2", receipt.EnvironmentSequence)
	}
}

func TestContinuityStoreCloseRacesWithoutPanicsOrPartialWrites(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	store := openAppendStoreV1(t, stateRoot, "environment-a", 100)
	projectID := continuity.ProjectID("project-close-race")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))

	start := make(chan struct{})
	appendErrors := make([]error, 8)
	closeErrors := make([]error, 2)
	var wait sync.WaitGroup
	wait.Add(len(appendErrors) + 2)
	for index := range appendErrors {
		index := index
		go func() {
			defer wait.Done()
			<-start
			_, appendErrors[index] = store.RecordJournalEntry(context.Background(), projectID, continuity.FactID(fmt.Sprintf("fact-%d", index)), continuity.SubjectID(fmt.Sprintf("journal-%d", index)), continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "race"}})
		}()
	}
	for closer := range closeErrors {
		closer := closer
		go func() {
			defer wait.Done()
			<-start
			closeErrors[closer] = store.Close()
		}()
	}
	close(start)
	wait.Wait()
	for closer, err := range closeErrors {
		if err != nil {
			t.Fatalf("closer %d: %v", closer, err)
		}
	}
	successes := 0
	for _, err := range appendErrors {
		if err != nil {
			assertProblemCodeV1(t, err, continuity.ProblemStoreClosed)
			continue
		}
		successes++
	}
	_, err := store.RecordJournalEntry(context.Background(), projectID, "fact-after-close", "journal-after-close", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "closed"}})
	assertProblemCodeV1(t, err, continuity.ProblemStoreClosed)

	reopened := openAppendStoreV1(t, stateRoot, "environment-b", 100)
	var persisted int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM continuity_facts WHERE project_id = ?`, string(projectID)).Scan(&persisted); err != nil {
		t.Fatalf("count close-race facts after reopen: %v", err)
	}
	if want := 1 + successes; persisted != want {
		t.Fatalf("persisted close-race facts = %d, want registration plus %d successful appends", persisted, successes)
	}
}

func TestContinuityStoreRejectsClockOverflowWithoutConsumingSequence(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	ctx := context.Background()
	projectID := continuity.ProjectID("project-clock")
	mustAppendV1(t)(store.RegisterProject(ctx, projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"}))
	if _, err := store.db.Exec(`UPDATE continuity_facts SET hlc_wall_millis = 200, hlc_logical = ? WHERE fact_id = 'fact-project'`, math.MaxInt32); err != nil {
		t.Fatalf("inject logical overflow: %v", err)
	}
	_, err := store.RecordJournalEntry(ctx, projectID, "fact-overflow", "journal-overflow", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "blocked"}})
	assertProblemCodeV1(t, err, continuity.ProblemClockExhausted)

	store.mu.Lock()
	store.wallMillis = func() int64 { return 201 }
	store.mu.Unlock()
	receipt := mustAppendV1(t)(store.RecordJournalEntry(ctx, projectID, "fact-after-overflow", "journal-after", continuity.JournalRecordedPayload{Observation: appendObservationV1(), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "accepted"}}))
	if receipt.EnvironmentSequence != 2 || receipt.Clock != (continuity.HybridTime{WallMillis: 201}) {
		t.Fatalf("post-overflow receipt = %#v, want gapless sequence 2 at 201:0", receipt)
	}
}

func TestContinuityStoreCloseIsIdempotentAndBlocksNewAppends(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-a", 100)
	if err := store.Close(); err != nil {
		t.Fatalf("first Close(): %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close(): %v", err)
	}
	_, err := store.RegisterProject(context.Background(), "project-closed", "fact-closed", continuity.ProjectRegistrationPayload{Observation: appendObservationV1(), Label: "Loaf"})
	assertProblemCodeV1(t, err, continuity.ProblemStoreClosed)
}

func openAppendStoreV1(t *testing.T, stateRoot string, environmentID continuity.EnvironmentID, wallMillis int64) *Store {
	t.Helper()
	store, err := Open(stateRoot, environmentID)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	store.mu.Lock()
	store.wallMillis = func() int64 { return wallMillis }
	store.mu.Unlock()
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close(): %v", err)
		}
	})
	return store
}

func appendObservationV1() continuity.Observation {
	return continuity.Observation{ObservedAtMillis: 1, HarnessSessionID: "conversation-1", Branch: "issue/loaf-93", Worktree: "/workspace/loaf"}
}

func mustAppendV1(t *testing.T) func(continuity.AppendReceipt, error) continuity.AppendReceipt {
	t.Helper()
	return func(receipt continuity.AppendReceipt, err error) continuity.AppendReceipt {
		t.Helper()
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		return receipt
	}
}

func assertProblemCodeV1(t *testing.T, err error, want continuity.ProblemCode) {
	t.Helper()
	var problem *continuity.Problem
	if !errors.As(err, &problem) {
		t.Fatalf("error = %#v (%T), want *continuity.Problem", err, err)
	}
	if problem.Code != want {
		t.Fatalf("problem code = %q, want %q (%v)", problem.Code, want, problem)
	}
}

func containsStringV1(value, fragment string) bool {
	for index := 0; index+len(fragment) <= len(value); index++ {
		if value[index:index+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
