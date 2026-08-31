package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuitySnapshotConvergesAcrossPhysicalInsertionOrder(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-permutation")
	first := openAppendStoreV1(t, filepath.Join(testTempDir(t), "first"), "environment-first", 100)
	seedCompleteSnapshotProjectWithIDV1(t, first, projectID)
	facts := readSnapshotFactsV1(t, first, projectID)
	want := mustSnapshotV1(t, first, projectID, 250)

	second := openAppendStoreV1(t, filepath.Join(testTempDir(t), "second"), "environment-second", 900)
	projectSubject := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}
	insertSnapshotStoredFactV1(t, second, factsForSubjectRootV1(t, facts, projectSubject))
	for index := len(facts) - 1; index >= 0; index-- {
		if facts[index].subject == projectSubject && facts[index].kind == continuity.FactProjectRegistered {
			continue
		}
		insertSnapshotStoredFactV1(t, second, facts[index])
	}

	got := mustSnapshotV1(t, second, projectID, 250)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("physical insertion order changed Snapshot:\nfirst=%#v\nsecond=%#v", want, got)
	}
}

func TestContinuitySnapshotOrdersExactTiesByEnvironmentThenFactID(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-ties")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))

	insertJournal := func(factID continuity.FactID, subjectID continuity.SubjectID, environmentID continuity.EnvironmentID, sequence int64, text string) {
		content := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
			return encodeJournalRecordedV1(continuity.JournalRecordedPayload{
				Observation: snapshotObservationV1(1, "main"),
				Content:     continuity.JournalContent{Category: continuity.JournalNote, Text: text},
			})
		})
		insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, factID, continuity.RecordJournalEntry, subjectID, continuity.FactJournalRecorded, content, environmentID, sequence, 200, 0))
	}
	insertJournal("fact-tie-a", "journal-a", "environment-a", 999, "a")
	insertJournal("fact-tie-z", "journal-z", "environment-a", 1, "z")
	insertJournal("fact-tie-m", "journal-m", "environment-z", 500, "m")

	snapshot := mustSnapshotV1(t, store, projectID, 0)
	got := make([]continuity.SubjectID, 0, len(snapshot.EffectiveJournal.Entries))
	for _, entry := range snapshot.EffectiveJournal.Entries {
		got = append(got, entry.Record.Subject.ID)
	}
	want := []continuity.SubjectID{"journal-m", "journal-z", "journal-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tied journal order = %v, want environment then fact ID descending; environment sequence must be ignored", got)
	}
}

func TestContinuitySnapshotUsesBranchTolerantTerminalDominance(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-branches")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))

	ideaRoot := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeIdeaCreatedV1(continuity.IdeaCreatedPayload{Observation: snapshotObservationV1(1, "root"), Content: continuity.IdeaContent{Label: "root"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-idea-root", continuity.RecordIdea, "idea-branch", continuity.FactIdeaCreated, ideaRoot, "environment-idea", 1, 200, 0))
	resolved := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeIdeaResolutionV1(continuity.IdeaResolutionPayload{Observation: snapshotObservationV1(2, "resolve"), Predecessor: "fact-idea-root", Resolution: "resolved"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-idea-resolved", continuity.RecordIdea, "idea-branch", continuity.FactIdeaResolved, resolved, "environment-a", 1, 201, 0))
	archived := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeIdeaArchiveV1(continuity.IdeaArchivePayload{Observation: snapshotObservationV1(3, "archive"), Predecessor: "fact-idea-root", Reason: "archived"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-idea-archived", continuity.RecordIdea, "idea-branch", continuity.FactIdeaArchived, archived, "environment-z", 1, 201, 0))
	revised := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeIdeaRevisionV1(continuity.IdeaRevisionPayload{Observation: snapshotObservationV1(4, "revise"), Revises: "fact-idea-root", Content: continuity.IdeaContent{Label: "later branch"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-idea-revised", continuity.RecordIdea, "idea-branch", continuity.FactIdeaRevised, revised, "environment-idea", 2, 202, 0))

	decisionRoot := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeDecisionOpenedV1(continuity.DecisionOpenedPayload{Observation: snapshotObservationV1(1, "root"), Question: "source?"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-decision-root", continuity.RecordDecision, "decision-source", continuity.FactDecisionOpened, decisionRoot, "environment-decision", 1, 210, 0))
	successorRoot := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeDecisionOpenedV1(continuity.DecisionOpenedPayload{Observation: snapshotObservationV1(1, "root"), Question: "successor?"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-successor-root", continuity.RecordDecision, "decision-successor", continuity.FactDecisionOpened, successorRoot, "environment-decision", 2, 211, 0))
	superseded := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeDecisionSupersessionV1(continuity.DecisionSupersessionPayload{Observation: snapshotObservationV1(2, "supersede"), Predecessor: "fact-decision-root", SuccessorID: "decision-successor", Rationale: "new question"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-decision-superseded", continuity.RecordDecision, "decision-source", continuity.FactDecisionSuperseded, superseded, "environment-decision", 3, 212, 0))
	decisionResolution := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeDecisionResolutionV1(continuity.DecisionResolutionPayload{Observation: snapshotObservationV1(3, "resolve"), Predecessor: "fact-decision-root", Resolution: "SQLite", Rationale: "local"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-decision-resolved", continuity.RecordDecision, "decision-source", continuity.FactDecisionResolved, decisionResolution, "environment-decision", 4, 213, 0))

	findingRoot := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeFindingRecordedV1(continuity.FindingRecordedPayload{Observation: snapshotObservationV1(1, "root"), Content: continuity.FindingContent{Summary: "root"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-finding-root", continuity.RecordFinding, "finding-branch", continuity.FactFindingRecorded, findingRoot, "environment-finding", 1, 220, 0))
	retracted := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeFindingRetractionV1(continuity.FindingRetractionPayload{Observation: snapshotObservationV1(2, "retract"), Predecessor: "fact-finding-root", Reason: "retracted"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-finding-retracted", continuity.RecordFinding, "finding-branch", continuity.FactFindingRetracted, retracted, "environment-finding", 2, 221, 0))
	corrected := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeFindingCorrectionV1(continuity.FindingCorrectionPayload{Observation: snapshotObservationV1(3, "correct"), Corrects: "fact-finding-root", Content: continuity.FindingContent{Summary: "later branch"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-finding-corrected", continuity.RecordFinding, "finding-branch", continuity.FactFindingCorrected, corrected, "environment-finding", 3, 222, 0))

	referenceRoot := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeExternalReferenceRegistrationV1(continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(1, "root"), Locator: "opaque:branch"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference-root", continuity.RecordExternalReference, "reference-branch", continuity.FactExternalReferenceRegistered, referenceRoot, "environment-reference", 1, 240, 0))
	target := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "idea-branch"}
	firstAttachment := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeExternalReferenceAttachmentV1(continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(2, "attach"), Target: target})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference-attached-first", continuity.RecordExternalReference, "reference-branch", continuity.FactExternalReferenceAttached, firstAttachment, "environment-reference", 2, 241, 0))
	detachmentA := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeExternalReferenceDetachmentV1(continuity.ExternalReferenceDetachmentPayload{Observation: snapshotObservationV1(3, "detach-a"), Target: target, Predecessor: "fact-reference-attached-first", Reason: "detached branch A"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference-detached-a", continuity.RecordExternalReference, "reference-branch", continuity.FactExternalReferenceDetached, detachmentA, "environment-edge-a", 1, 242, 0))
	detachmentZ := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeExternalReferenceDetachmentV1(continuity.ExternalReferenceDetachmentPayload{Observation: snapshotObservationV1(3, "detach-z"), Target: target, Predecessor: "fact-reference-attached-first", Reason: "detached branch Z"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference-detached-z", continuity.RecordExternalReference, "reference-branch", continuity.FactExternalReferenceDetached, detachmentZ, "environment-edge-z", 1, 242, 0))
	attachmentA := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeExternalReferenceAttachmentV1(continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(4, "reattach-a"), Target: target, Predecessor: "fact-reference-detached-a"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference-attached-a", continuity.RecordExternalReference, "reference-branch", continuity.FactExternalReferenceAttached, attachmentA, "environment-edge-a", 2, 243, 0))
	attachmentZ := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeExternalReferenceAttachmentV1(continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(4, "reattach-z"), Target: target, Predecessor: "fact-reference-detached-z"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference-attached-z", continuity.RecordExternalReference, "reference-branch", continuity.FactExternalReferenceAttached, attachmentZ, "environment-edge-z", 2, 243, 0))

	snapshot := mustSnapshotV1(t, store, projectID, 0)
	idea := ideaByIDV1(t, snapshot.CurrentIdeas.Ideas, "idea-branch")
	if idea.Disposition != continuity.IdeaArchived || idea.Record.Head.FactID != "fact-idea-archived" || idea.Content.Label != "later branch" || idea.ContentStamp.FactID != "fact-idea-revised" {
		t.Fatalf("branch-tolerant idea = %#v", idea)
	}
	decision := decisionByIDV1(t, snapshot.CurrentDecisions.Decisions, "decision-source")
	if decision.State != continuity.DecisionSuperseded || decision.Record.Head.FactID != "fact-decision-superseded" || decision.ResolutionStamp.FactID != "fact-decision-resolved" || decision.Resolution != "SQLite" {
		t.Fatalf("branch-tolerant decision = %#v", decision)
	}
	finding := findingByIDV1(t, snapshot.CurrentFindings.Findings, "finding-branch")
	if finding.State != continuity.FindingRetracted || finding.Record.Head.FactID != "fact-finding-retracted" || finding.ContentStamp.FactID != "fact-finding-corrected" || finding.Content.Summary != "later branch" {
		t.Fatalf("branch-tolerant finding = %#v", finding)
	}
	reference := referenceByIDV1(t, snapshot.ExternalReferences.References, "reference-branch")
	if reference.Record.Head.FactID != "fact-reference-attached-z" || len(reference.Attachments) != 1 || reference.Attachments[0].Stamp.FactID != "fact-reference-attached-z" || reference.Attachments[0].Target != target {
		t.Fatalf("branch-tolerant external reference = %#v", reference)
	}
}

func TestContinuitySnapshotRejectsCausallyCorruptHistory(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-causal-corruption")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))
	root := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeJournalRecordedV1(continuity.JournalRecordedPayload{Observation: snapshotObservationV1(1, "main"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "root"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-journal-root", continuity.RecordJournalEntry, "journal-corrupt", continuity.FactJournalRecorded, root, "environment-a", 1, 200, 0))
	correction := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeJournalCorrectionV1(continuity.JournalCorrectionPayload{Observation: snapshotObservationV1(2, "main"), Corrects: "fact-missing", Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "corrupt"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-journal-correction", continuity.RecordJournalEntry, "journal-corrupt", continuity.FactJournalCorrectionRecorded, correction, "environment-a", 2, 201, 0))

	_, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{})
	assertProblemCodeV1(t, err, continuity.ProblemCorruptFact)
}

func TestContinuitySnapshotAndCloseRaceReturnsOnlyCompleteResults(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-close-race")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))

	const readers = 32
	errorsByReader := make([]error, readers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(readers + 4)
	for index := range errorsByReader {
		index := index
		go func() {
			defer wait.Done()
			<-start
			snapshot, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{})
			if err == nil && (snapshot.Project.Identity.Record.ProjectID != projectID || snapshot.EffectiveJournal.Entries == nil) {
				err = errors.New("Snapshot returned an incomplete successful projection")
			}
			errorsByReader[index] = err
		}()
	}
	closeErrors := make([]error, 4)
	for index := range closeErrors {
		index := index
		go func() {
			defer wait.Done()
			<-start
			closeErrors[index] = store.Close()
		}()
	}
	close(start)
	wait.Wait()
	for index, err := range closeErrors {
		if err != nil {
			t.Fatalf("Close %d: %v", index, err)
		}
	}
	for index, err := range errorsByReader {
		if err == nil {
			continue
		}
		var problem *continuity.Problem
		if !errors.As(err, &problem) || problem.Code != continuity.ProblemStoreClosed {
			t.Fatalf("Snapshot %d: %v", index, err)
		}
	}
}

func readSnapshotFactsV1(t *testing.T, store *Store, projectID continuity.ProjectID) []storedFactV1 {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin fact copy: %v", err)
	}
	facts, err := loadProjectFactsV1(context.Background(), tx, projectID)
	if err != nil {
		tx.Rollback()
		t.Fatalf("load fact copy: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit fact copy read: %v", err)
	}
	return facts
}

func factsForSubjectRootV1(t *testing.T, facts []storedFactV1, subject continuity.SubjectRef) storedFactV1 {
	t.Helper()
	rootKind, ok := rootFactKindV1(subject.Kind)
	if !ok {
		t.Fatalf("no root kind for %s", subject.Kind)
	}
	for _, fact := range facts {
		if fact.subject == subject && fact.kind == rootKind {
			return fact
		}
	}
	t.Fatalf("root for %#v not found", subject)
	return storedFactV1{}
}

func snapshotStoredFactV1(projectID continuity.ProjectID, factID continuity.FactID, recordKind continuity.RecordKind, subjectID continuity.SubjectID, factKind continuity.FactKind, content canonicalContentV1, environmentID continuity.EnvironmentID, sequence, wallMillis int64, logical int32) storedFactV1 {
	return storedFactV1{
		factID:              factID,
		projectID:           projectID,
		subject:             continuity.SubjectRef{Kind: recordKind, ID: subjectID},
		kind:                factKind,
		payloadVersion:      payloadVersionV1,
		content:             content,
		environmentID:       environmentID,
		environmentSequence: sequence,
		clock:               continuity.HybridTime{WallMillis: wallMillis, Logical: logical},
		envelopeVersion:     envelopeVersionV1,
	}
}

func insertSnapshotStoredFactV1(t *testing.T, store *Store, fact storedFactV1) {
	t.Helper()
	_, err := store.db.Exec(`
INSERT INTO continuity_facts(
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
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(fact.factID),
		string(fact.projectID),
		string(fact.subject.Kind),
		string(fact.subject.ID),
		string(fact.kind),
		fact.payloadVersion,
		string(fact.content),
		string(fact.environmentID),
		fact.environmentSequence,
		fact.clock.WallMillis,
		fact.clock.Logical,
		fact.envelopeVersion,
	)
	if err != nil {
		t.Fatalf("insert snapshot fact %s: %v", fact.factID, err)
	}
}

func canonicalSnapshotContentV1(t *testing.T, encode func() (canonicalContentV1, error)) canonicalContentV1 {
	t.Helper()
	content, err := encode()
	if err != nil {
		t.Fatalf("encode snapshot fact: %v", err)
	}
	return content
}

func ideaByIDV1(t *testing.T, ideas []continuity.Idea, id continuity.SubjectID) continuity.Idea {
	t.Helper()
	for _, idea := range ideas {
		if idea.Record.Subject.ID == id {
			return idea
		}
	}
	t.Fatalf("idea %s not found", id)
	return continuity.Idea{}
}
