package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestExactSyncAuthorityRecoverySuccessorPageReplayChecksEveryPriorCheckpointField(t *testing.T) {
	tests := []struct {
		name       string
		pageNumber int
	}{
		{name: "page-1-initial-seed", pageNumber: 1},
		{name: "later-page-persisted-result", pageNumber: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, current, page, expected := setupSyncAuthorityRecoveryReplayProofV1(t, test.pageNumber)

			replayed, exact, err := callExactSyncAuthorityRecoverySuccessorPageReplayV1(
				t, store, projectID, current, expected, page,
			)
			if err != nil || !replayed || !exact {
				t.Fatalf("exact replay = (%v, %v, %v), want (true, true, nil)", replayed, exact, err)
			}

			for _, mutation := range syncAuthorityRecoveryReplayCheckpointMutationsV1() {
				t.Run(mutation.name, func(t *testing.T) {
					mutated := expected
					mutation.mutate(&mutated)
					replayed, exact, err := callExactSyncAuthorityRecoverySuccessorPageReplayV1(
						t, store, projectID, current, mutated, page,
					)
					if err != nil || !replayed || exact {
						t.Fatalf("mutated checkpoint replay = (%v, %v, %v), want (true, false, nil)", replayed, exact, err)
					}
				})
			}
		})
	}
}

func TestExactSyncAuthorityRecoverySuccessorPageReplayRejectsCorruptPriorPage(t *testing.T) {
	store, projectID, current, page, expected := setupSyncAuthorityRecoveryReplayProofV1(t, 2)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_environments
SET certificate_bytes = ?
WHERE project_id = ? AND candidate_id = ? AND page_number = 1 AND environment_ordinal = 1`,
		[]byte("tampered prior page certificate"), string(projectID), current.candidate.CandidateID[:],
	); err != nil {
		t.Fatalf("tamper prior page: %v", err)
	}

	replayed, exact, err := callExactSyncAuthorityRecoverySuccessorPageReplayV1(
		t, store, projectID, current, expected, page,
	)
	if err == nil {
		t.Fatalf("corrupt prior page replay = (%v, %v, nil), want corruption error", replayed, exact)
	}
	assertSyncErrorCode(t, err, SyncErrorStore)
}

func setupSyncAuthorityRecoveryReplayProofV1(
	t *testing.T,
	pageNumber int,
) (*Store, continuity.ProjectID, persistedSyncAuthorityCandidateV2, SyncAuthorityPage, SyncAuthorityCandidateCheckpoint) {
	t.Helper()
	if pageNumber != 1 && pageNumber != 2 {
		t.Fatalf("unsupported replay proof page number %d", pageNumber)
	}
	store := openSyncStore(t, "recovery-successor-replay-proof")
	projectID := continuity.ProjectID("project-recovery-successor-replay-proof")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(8)
	environments := syncAuthorityCandidateManyEnvironmentsV2(8)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
	if err != nil {
		t.Fatalf("stage first page: %v", err)
	}
	page := firstPage
	expected := SyncAuthorityCandidateCheckpoint{}
	if pageNumber == 1 {
		headerDigest := [32]byte{}
		_, headerDigest, err = deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
		if err != nil {
			t.Fatalf("derive candidate header: %v", err)
		}
		rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
		if err != nil {
			t.Fatalf("derive candidate rolling seed: %v", err)
		}
		expected = SyncAuthorityCandidateCheckpoint{
			CandidateID:              first.CandidateID,
			RollingEnvironmentDigest: rolling,
		}
	} else {
		secondPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[4:], false)
		if _, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, secondPage); err != nil {
			t.Fatalf("stage second page: %v", err)
		}
		page = secondPage
		expected = first.Checkpoint()
	}

	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatalf("begin inspection transaction: %v", err)
	}
	defer tx.Rollback()
	current, found, err := readActiveSyncAuthorityCandidateHeaderV2(context.Background(), tx, projectID)
	if err != nil {
		t.Fatalf("read candidate header: %v", err)
	}
	if !found {
		t.Fatal("candidate header not found")
	}
	if current.candidate.CandidateID != first.CandidateID {
		t.Fatalf("candidate id = %x, want %x", current.candidate.CandidateID, first.CandidateID)
	}
	return store, projectID, current, page, expected
}

func callExactSyncAuthorityRecoverySuccessorPageReplayV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	current persistedSyncAuthorityCandidateV2,
	expected SyncAuthorityCandidateCheckpoint,
	page SyncAuthorityPage,
) (bool, bool, error) {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatalf("begin replay transaction: %v", err)
	}
	defer tx.Rollback()
	return exactSyncAuthorityRecoverySuccessorPageReplayV1(context.Background(), tx, current, expected, page)
}

func syncAuthorityRecoveryReplayCheckpointMutationsV1() []struct {
	name   string
	mutate func(*SyncAuthorityCandidateCheckpoint)
} {
	return []struct {
		name   string
		mutate func(*SyncAuthorityCandidateCheckpoint)
	}{
		{name: "candidate-id", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.CandidateID[0] ^= 0xff
		}},
		{name: "page-count", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.PageCount++
		}},
		{name: "environment-count", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.EnvironmentCount++
		}},
		{name: "through-environment-id", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.ThroughEnvironmentID = "different-through-environment"
		}},
		{name: "rolling-environment-digest", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.RollingEnvironmentDigest[0] ^= 0xff
		}},
		{name: "ready", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.Ready = !checkpoint.Ready
		}},
		{name: "authority-digest", mutate: func(checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.AuthorityDigest[0] ^= 0xff
		}},
	}
}
