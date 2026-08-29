package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuityContextConvergesAcrossPhysicalInsertionOrder(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-context-permutation")
	first := openAppendStoreV1(t, filepath.Join(testTempDir(t), "first"), "environment-first", 100)
	seedCompleteSnapshotProjectWithIDV1(t, first, projectID)
	facts := readSnapshotFactsV1(t, first, projectID)
	focus := continuity.SubjectRef{Kind: continuity.RecordJournalEntry, ID: "journal-1"}
	request := continuity.ContextRequest{Focus: &focus, Scope: "continuity", Branch: "issue/loaf-96", AtMillis: 250}
	want := mustContextV1(t, first, projectID, request)

	second := openAppendStoreV1(t, filepath.Join(testTempDir(t), "second"), "environment-second", 900)
	projectSubject := continuity.SubjectRef{Kind: continuity.RecordProjectIdentity, ID: continuity.SubjectID(projectID)}
	insertSnapshotStoredFactV1(t, second, factsForSubjectRootV1(t, facts, projectSubject))
	for index := len(facts) - 1; index >= 0; index-- {
		if facts[index].subject == projectSubject && facts[index].kind == continuity.FactProjectRegistered {
			continue
		}
		insertSnapshotStoredFactV1(t, second, facts[index])
	}

	got := mustContextV1(t, second, projectID, request)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("physical insertion order changed ContextDigest:\nfirst=%#v\nsecond=%#v", want, got)
	}
}

func TestContinuityContextAtMillisOnlyEchoesWhenScratchpadClaimsChange(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-at", 100)
	projectID := seedCompleteSnapshotProjectV1(t, store)
	before := mustContextV1(t, store, projectID, continuity.ContextRequest{AtMillis: 249})
	after := mustContextV1(t, store, projectID, continuity.ContextRequest{AtMillis: 301})
	before.AtMillis = 0
	after.AtMillis = 0
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("scratchpad claim instant changed derived context:\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestContinuityContextCancellationDoesNotPoisonStore(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-cancel", 100)
	projectID := continuity.ProjectID("project-context-cancellation")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Context"}))

	connection, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("reserve sole connection: %v", err)
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	_, err = store.DeriveContext(waitContext, projectID, continuity.ContextRequest{})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		connection.Close()
		t.Fatalf("connection-wait DeriveContext error = %v, want deadline exceeded", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("release sole connection: %v", err)
	}
	if digest := mustContextV1(t, store, projectID, continuity.ContextRequest{}); digest.Project.Identity.Record.ProjectID != projectID {
		t.Fatalf("store did not recover after canceled context wait: %#v", digest.Project.Identity)
	}

	snapshot := mustSnapshotV1(t, store, projectID, 0)
	foldContext := newCancelAfterChecksContextV1(4)
	_, err = deriveContextDigestV1(foldContext, snapshot, continuity.ContextRequest{}, contextFocusRelationsV1{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("in-memory context cancellation = %v, want context.Canceled", err)
	}
}

func TestContinuityContextAndCloseRaceReturnsOnlyCompleteResults(t *testing.T) {
	t.Parallel()

	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state"), "environment-close", 100)
	projectID := continuity.ProjectID("project-context-close-race")
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{Observation: snapshotObservationV1(1, "main"), Label: "Context"}))

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
			digest, err := store.DeriveContext(context.Background(), projectID, continuity.ContextRequest{})
			if err == nil && (digest.Project.Identity.Record.ProjectID != projectID || digest.ProjectJournal.Entries == nil || digest.ExternalReferences.References == nil) {
				err = errors.New("DeriveContext returned an incomplete successful projection")
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
			t.Fatalf("DeriveContext %d: %v", index, err)
		}
	}
}
