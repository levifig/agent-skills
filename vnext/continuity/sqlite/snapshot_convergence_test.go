package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuitySnapshotUsesLeastMintAndOnlyItsCausalClosure(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-root-collision")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))

	rootA := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeJournalRecordedV1(continuity.JournalRecordedPayload{Observation: snapshotObservationV1(1, "root-a"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "root A"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-root-a", continuity.RecordJournalEntry, "journal-collision", continuity.FactJournalRecorded, rootA, "environment-a", 1, 200, 0))
	rootZ := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeJournalRecordedV1(continuity.JournalRecordedPayload{Observation: snapshotObservationV1(2, "root-z"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "root Z"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-root-z", continuity.RecordJournalEntry, "journal-collision", continuity.FactJournalRecorded, rootZ, "environment-z", 1, 201, 0))
	correctionA := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeJournalCorrectionV1(continuity.JournalCorrectionPayload{Observation: snapshotObservationV1(3, "branch-a"), Corrects: "fact-root-a", Content: continuity.JournalContent{Category: continuity.JournalDecision, Text: "canonical closure"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-correction-a", continuity.RecordJournalEntry, "journal-collision", continuity.FactJournalCorrectionRecorded, correctionA, "environment-a", 2, 202, 0))
	correctionZ := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeJournalCorrectionV1(continuity.JournalCorrectionPayload{Observation: snapshotObservationV1(4, "branch-z"), Corrects: "fact-root-z", Content: continuity.JournalContent{Category: continuity.JournalDecision, Text: "losing closure"}})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-correction-z", continuity.RecordJournalEntry, "journal-collision", continuity.FactJournalCorrectionRecorded, correctionZ, "environment-z", 2, 203, 0))

	snapshot := mustSnapshotV1(t, store, projectID, 0)
	if len(snapshot.EffectiveJournal.Entries) != 1 {
		t.Fatalf("journal entries = %#v", snapshot.EffectiveJournal.Entries)
	}
	entry := snapshot.EffectiveJournal.Entries[0]
	if entry.Record.Root.FactID != "fact-root-a" || entry.Record.Head.FactID != "fact-correction-a" || entry.Content.Text != "canonical closure" {
		t.Fatalf("canonical root closure = %#v", entry)
	}
}

func TestContinuitySnapshotKeysLatestRecordsByStructuralOptionalFocus(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-focus-keys")
	observation := snapshotObservationV1(1, "main")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: observation, Label: "Loaf"}))
	mustAppendV1(t)(store.RecordJournalEntry(context.Background(), projectID, "fact-journal", "shared", continuity.JournalRecordedPayload{Observation: observation, Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "journal"}}))
	mustAppendV1(t)(store.CreateIdea(context.Background(), projectID, "fact-idea", "shared", continuity.IdeaCreatedPayload{Observation: observation, Content: continuity.IdeaContent{Label: "idea"}}))
	mustAppendV1(t)(store.RecordWrap(context.Background(), projectID, "fact-project-wrap-old", "project-wrap-old", continuity.WrapRecordedPayload{Observation: observation, Scope: "same", Synthesis: "old project"}))
	mustAppendV1(t)(store.RecordWrap(context.Background(), projectID, "fact-project-wrap", "project-wrap", continuity.WrapRecordedPayload{Observation: observation, Scope: "different", Synthesis: "project"}))
	journalFocus := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "shared"}
	ideaFocus := continuity.SubjectRef{Kind: continuity.RecordIdea, ID: "shared"}
	mustAppendV1(t)(store.RecordWrap(context.Background(), projectID, "fact-journal-wrap", "journal-wrap", continuity.WrapRecordedPayload{Observation: observation, Focus: &journalFocus, Scope: "same", Synthesis: "journal"}))
	mustAppendV1(t)(store.RecordWrap(context.Background(), projectID, "fact-idea-wrap", "idea-wrap", continuity.WrapRecordedPayload{Observation: observation, Focus: &ideaFocus, Scope: "same", Synthesis: "idea"}))

	snapshot := mustSnapshotV1(t, store, projectID, 0)
	if len(snapshot.LatestWraps.Wraps) != 3 {
		t.Fatalf("latest structural wraps = %#v", snapshot.LatestWraps.Wraps)
	}
	seen := make(map[string]string)
	for _, wrap := range snapshot.LatestWraps.Wraps {
		key := "project"
		if wrap.Focus != nil {
			key = string(wrap.Focus.Kind) + ":" + string(wrap.Focus.ID)
		}
		seen[key] = wrap.Synthesis
	}
	if seen["project"] != "project" || seen["journal-entry:shared"] != "journal" || seen["idea:shared"] != "idea" {
		t.Fatalf("structural focus winners = %v", seen)
	}
}

func TestContinuitySnapshotKeepsScratchpadClaimSemanticHeads(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-claim-branches")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))

	seedClaimScratchpadV1(t, store, projectID, "scratchpad-renewal", 200)
	claim := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadClaimV1(continuity.ScratchpadClaimPayload{Observation: snapshotObservationV1(3, "claim"), ClaimID: "claim-renewal", ParticipantID: "participant", Resource: "resource", ExpiresAtMillis: 300})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-claim-root", continuity.RecordScratchpad, "scratchpad-renewal", continuity.FactScratchpadClaimRecorded, claim, "environment-renewal", 3, 202, 0))
	winningRenewal := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadClaimV1(continuity.ScratchpadClaimPayload{Observation: snapshotObservationV1(4, "winning"), ClaimID: "claim-renewal", ParticipantID: "participant", Resource: "resource", ExpiresAtMillis: 400})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-renewal-winning", continuity.RecordScratchpad, "scratchpad-renewal", continuity.FactScratchpadClaimRecorded, winningRenewal, "environment-renewal", 4, 203, 0))
	losingRenewal := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadClaimV1(continuity.ScratchpadClaimPayload{Observation: snapshotObservationV1(5, "losing"), ClaimID: "claim-renewal", ParticipantID: "participant", Resource: "resource", ExpiresAtMillis: 350})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-renewal-losing", continuity.RecordScratchpad, "scratchpad-renewal", continuity.FactScratchpadClaimRecorded, losingRenewal, "environment-renewal", 5, 204, 0))

	seedClaimScratchpadV1(t, store, projectID, "scratchpad-release", 210)
	releasedClaim := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadClaimV1(continuity.ScratchpadClaimPayload{Observation: snapshotObservationV1(3, "claim"), ClaimID: "claim-release", ParticipantID: "participant", Resource: "resource", ExpiresAtMillis: 300})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-release-claim-root", continuity.RecordScratchpad, "scratchpad-release", continuity.FactScratchpadClaimRecorded, releasedClaim, "environment-release", 3, 212, 0))
	release := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadClaimReleaseV1(continuity.ScratchpadClaimReleasePayload{Observation: snapshotObservationV1(4, "release"), ClaimID: "claim-release", ReleasedBy: "participant", Reason: "done"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-claim-release", continuity.RecordScratchpad, "scratchpad-release", continuity.FactScratchpadClaimReleased, release, "environment-release", 4, 213, 0))
	postReleaseRenewal := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadClaimV1(continuity.ScratchpadClaimPayload{Observation: snapshotObservationV1(5, "post-release"), ClaimID: "claim-release", ParticipantID: "participant", Resource: "resource", ExpiresAtMillis: 500})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-post-release-renewal", continuity.RecordScratchpad, "scratchpad-release", continuity.FactScratchpadClaimRecorded, postReleaseRenewal, "environment-release", 5, 214, 0))

	snapshot := mustSnapshotV1(t, store, projectID, 0)
	renewalScratchpad := scratchpadByIDV1(t, snapshot.Scratchpads.Scratchpads, "scratchpad-renewal")
	if renewalScratchpad.Record.Head.FactID != "fact-renewal-winning" || len(renewalScratchpad.Claims) != 1 || renewalScratchpad.Claims[0].ExpiresAtMillis != 400 || renewalScratchpad.Claims[0].Head.FactID != "fact-renewal-winning" {
		t.Fatalf("losing renewal changed semantic heads: %#v", renewalScratchpad)
	}
	releaseScratchpad := scratchpadByIDV1(t, snapshot.Scratchpads.Scratchpads, "scratchpad-release")
	if releaseScratchpad.Record.Head.FactID != "fact-claim-release" || len(releaseScratchpad.Claims) != 0 {
		t.Fatalf("post-release renewal changed terminal head: %#v", releaseScratchpad)
	}
}

func TestContinuitySnapshotRejectsImpossibleExternalEdgeTransitions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		secondKind continuity.FactKind
	}{
		{name: "attach after attach", secondKind: continuity.FactExternalReferenceAttached},
		{name: "detach after detach", secondKind: continuity.FactExternalReferenceDetached},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
			projectID := continuity.ProjectID("project-impossible-edge")
			mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))
			targetContent := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
				return encodeJournalRecordedV1(continuity.JournalRecordedPayload{Observation: snapshotObservationV1(1, "target"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "target"}})
			})
			insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-target", continuity.RecordJournalEntry, "journal-target", continuity.FactJournalRecorded, targetContent, "environment-edge", 1, 200, 0))
			referenceContent := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
				return encodeExternalReferenceRegistrationV1(continuity.ExternalReferenceRegistrationPayload{Observation: snapshotObservationV1(1, "reference"), Locator: "opaque:edge"})
			})
			insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-reference", continuity.RecordExternalReference, "reference-edge", continuity.FactExternalReferenceRegistered, referenceContent, "environment-edge", 2, 201, 0))
			target := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-target"}
			firstAttach := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
				return encodeExternalReferenceAttachmentV1(continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(2, "attach"), Target: target})
			})
			insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-attach-first", continuity.RecordExternalReference, "reference-edge", continuity.FactExternalReferenceAttached, firstAttach, "environment-edge", 3, 202, 0))

			predecessor := continuity.FactID("fact-attach-first")
			if test.secondKind == continuity.FactExternalReferenceDetached {
				firstDetach := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
					return encodeExternalReferenceDetachmentV1(continuity.ExternalReferenceDetachmentPayload{Observation: snapshotObservationV1(3, "detach"), Target: target, Predecessor: predecessor, Reason: "first"})
				})
				insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-detach-first", continuity.RecordExternalReference, "reference-edge", continuity.FactExternalReferenceDetached, firstDetach, "environment-edge", 4, 203, 0))
				predecessor = "fact-detach-first"
			}
			if test.secondKind == continuity.FactExternalReferenceAttached {
				impossibleAttach := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
					return encodeExternalReferenceAttachmentV1(continuity.ExternalReferenceAttachmentPayload{Observation: snapshotObservationV1(4, "impossible"), Target: target, Predecessor: predecessor})
				})
				insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-impossible", continuity.RecordExternalReference, "reference-edge", continuity.FactExternalReferenceAttached, impossibleAttach, "environment-edge", 5, 204, 0))
			} else {
				impossibleDetach := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
					return encodeExternalReferenceDetachmentV1(continuity.ExternalReferenceDetachmentPayload{Observation: snapshotObservationV1(4, "impossible"), Target: target, Predecessor: predecessor, Reason: "second"})
				})
				insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, "fact-impossible", continuity.RecordExternalReference, "reference-edge", continuity.FactExternalReferenceDetached, impossibleDetach, "environment-edge", 5, 204, 0))
			}

			_, err := store.Snapshot(context.Background(), projectID, continuity.SnapshotRequest{})
			assertProblemCodeV1(t, err, continuity.ProblemCorruptFact)
		})
	}
}

func TestContinuityReadOnlyTransactionIsDeferredAndWALConsistent(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	reader := openAppendStoreV1(t, stateRoot, "environment-reader", 100)
	projectID := continuity.ProjectID("project-wal-read")
	mustAppendV1(t)(reader.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))
	writer := openAppendStoreV1(t, stateRoot, "environment-writer", 200)

	tx, err := reader.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin read-only transaction: %v", err)
	}
	rows, err := tx.Query(`SELECT fact_id FROM continuity_facts WHERE project_id = ? ORDER BY hlc_wall_millis, hlc_logical, environment_id COLLATE BINARY, fact_id COLLATE BINARY`, string(projectID))
	if err != nil {
		tx.Rollback()
		t.Fatalf("open stable read cursor: %v", err)
	}
	if !rows.Next() {
		rows.Close()
		tx.Rollback()
		t.Fatal("stable read cursor did not contain project identity")
	}
	var firstFactID string
	if err := rows.Scan(&firstFactID); err != nil || firstFactID != "fact-project" {
		rows.Close()
		tx.Rollback()
		t.Fatalf("first stable fact = %q, %v", firstFactID, err)
	}

	writeContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := writer.RecordJournalEntry(writeContext, projectID, "fact-during-read", "journal-during-read", continuity.JournalRecordedPayload{Observation: snapshotObservationV1(2, "writer"), Content: continuity.JournalContent{Category: continuity.JournalNote, Text: "committed during read"}}); err != nil {
		rows.Close()
		tx.Rollback()
		t.Fatalf("WAL writer while read cursor open: %v", err)
	}
	if rows.Next() {
		var unexpected string
		if err := rows.Scan(&unexpected); err != nil {
			t.Fatalf("scan unexpected stable-read fact: %v", err)
		}
		t.Fatalf("read-only transaction observed post-snapshot fact %q", unexpected)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("stable read cursor: %v", err)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close stable read cursor: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit stable read transaction: %v", err)
	}

	snapshot := mustSnapshotV1(t, reader, projectID, 0)
	if len(snapshot.EffectiveJournal.Entries) != 1 || snapshot.EffectiveJournal.Entries[0].Record.Subject.ID != "journal-during-read" {
		t.Fatalf("post-commit Snapshot missed WAL write: %#v", snapshot.EffectiveJournal.Entries)
	}
}

func TestContinuitySnapshotCancellationDoesNotPoisonStore(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-root", 100)
	projectID := continuity.ProjectID("project-read-cancellation")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Loaf"}))

	connection, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve sole connection: %v", err)
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, err = store.Snapshot(waitContext, projectID, continuity.SnapshotRequest{})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		connection.Close()
		t.Fatalf("connection-wait Snapshot error = %v, want deadline exceeded", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("release sole connection: %v", err)
	}
	if snapshot := mustSnapshotV1(t, store, projectID, 0); snapshot.Project.Identity.Record.ProjectID != projectID {
		t.Fatalf("store did not recover after canceled wait: %#v", snapshot.Project.Identity)
	}

	facts := readSnapshotFactsV1(t, store, projectID)
	foldContext := newCancelAfterChecksContextV1(4)
	_, err = foldProjectSnapshotV1(foldContext, projectID, 0, facts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("in-memory fold cancellation = %v, want context.Canceled", err)
	}
}

func seedClaimScratchpadV1(t *testing.T, store *Store, projectID continuity.ProjectID, scratchpadID continuity.SubjectID, wallMillis int64) {
	t.Helper()
	environmentID := continuity.EnvironmentID("environment-" + string(scratchpadID))
	opened := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadOpenedV1(continuity.ScratchpadOpenedPayload{Observation: snapshotObservationV1(1, "open"), Label: string(scratchpadID)})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, continuity.FactID("fact-"+string(scratchpadID)+"-root"), continuity.RecordScratchpad, scratchpadID, continuity.FactScratchpadOpened, opened, environmentID, 1, wallMillis, 0))
	participant := canonicalSnapshotContentV1(t, func() (canonicalContentV1, error) {
		return encodeScratchpadParticipantV1(continuity.ScratchpadParticipantPayload{Observation: snapshotObservationV1(2, "participant"), ParticipantID: "participant", Name: "writer"})
	})
	insertSnapshotStoredFactV1(t, store, snapshotStoredFactV1(projectID, continuity.FactID("fact-"+string(scratchpadID)+"-participant"), continuity.RecordScratchpad, scratchpadID, continuity.FactScratchpadParticipantIntroduced, participant, environmentID, 2, wallMillis+1, 0))
}

type cancelAfterChecksContextV1 struct {
	mu        sync.Mutex
	remaining int
	done      chan struct{}
	canceled  bool
}

func newCancelAfterChecksContextV1(checks int) *cancelAfterChecksContextV1 {
	return &cancelAfterChecksContextV1{remaining: checks, done: make(chan struct{})}
}

func (ctx *cancelAfterChecksContextV1) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (ctx *cancelAfterChecksContextV1) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *cancelAfterChecksContextV1) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.canceled {
		return context.Canceled
	}
	ctx.remaining--
	if ctx.remaining > 0 {
		return nil
	}
	ctx.canceled = true
	close(ctx.done)
	return context.Canceled
}

func (ctx *cancelAfterChecksContextV1) Value(any) any {
	return nil
}
