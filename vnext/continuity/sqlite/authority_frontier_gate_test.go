package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestStageVerifiedSyncAuthorityCandidatePageUsesFreshFirstPageButNotReplayToEstablishMembershipFloor(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-authority-frontier-stage-first")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	page := syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), false)
	want := syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead)
	seedSyncRelayFrontierForTestV1(t, store, want, false)

	candidate, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(fresh first page) error = %v", err)
	}
	if !candidate.Ready {
		t.Fatalf("fresh candidate = %#v, want ready", candidate)
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, want, true)

	seedSyncRelayFrontierForTestV1(t, store, want, false)
	replayed, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(exact replay) error = %v", err)
	}
	if replayed.Checkpoint() != candidate.Checkpoint() {
		t.Fatalf("exact replay checkpoint = %#v, want %#v", replayed.Checkpoint(), candidate.Checkpoint())
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, want, false)
}

func TestStageVerifiedSyncAuthorityCandidatePageBlocksNonReplayAppendWhileMembershipFloorUnknown(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-authority-frontier-stage-append")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(5)
	environments := syncAuthorityCandidateManyEnvironmentsV2(5)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	first, err := store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, firstPage)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(first) error = %v", err)
	}
	want := syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead)
	seedSyncRelayFrontierForTestV1(t, store, want, false)
	secondPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[4:], false)

	_, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, secondPage)
	assertSyncFrontierCursorFieldV1(t, err, "membership_generation")
	current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("CurrentSyncAuthorityCandidate() = (%#v, %v, %v)", current, found, err)
	}
	if current.Checkpoint() != first.Checkpoint() {
		t.Fatalf("candidate changed after refused append: got %#v want %#v", current.Checkpoint(), first.Checkpoint())
	}
	assertSyncRelayWatermarkRowWithKnown(t, store, want, false)
}

func TestPromoteSyncAuthorityCandidateRequiresKnownExactComponentwiseFrontier(t *testing.T) {
	t.Parallel()

	t.Run("unknown exact frontier", func(t *testing.T) {
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		projectID := continuity.ProjectID("project-authority-frontier-promote-unknown")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
		candidate := stageReadySyncAuthorityCandidateFromSnapshot(
			t, store, projectID, snapshot, syncAuthorityCandidateManyEnvironmentsV2(1),
		)
		want := syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead)
		seedSyncRelayFrontierForTestV1(t, store, want, false)

		_, err = store.PromoteSyncAuthorityCandidate(context.Background(), projectID, candidate.Checkpoint())
		assertSyncFrontierCursorFieldV1(t, err, "membership_generation")
		assertActiveAuthorityCandidateCheckpointForTestV1(t, store, projectID, candidate.Checkpoint())
	})

	t.Run("higher same-head membership", func(t *testing.T) {
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		projectID := continuity.ProjectID("project-authority-frontier-promote-membership")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
		candidate := stageReadySyncAuthorityCandidateFromSnapshot(
			t, store, projectID, snapshot, syncAuthorityCandidateManyEnvironmentsV2(1),
		)
		advanced := syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead)
		advanced.MembershipGeneration++
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
			t.Fatalf("AdvanceSyncRelayWatermark(higher membership) = (%#v, %v), want (%#v, nil)", got, err, advanced)
		}

		_, err = store.PromoteSyncAuthorityCandidate(context.Background(), projectID, candidate.Checkpoint())
		assertSyncFrontierCursorFieldV1(t, err, "membership_generation")
		assertActiveAuthorityCandidateCheckpointForTestV1(t, store, projectID, candidate.Checkpoint())
	})

	t.Run("terminal receipt replay remains historical", func(t *testing.T) {
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		projectID := continuity.ProjectID("project-authority-frontier-promote-replay")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
		candidate := stageReadySyncAuthorityCandidateFromSnapshot(
			t, store, projectID, snapshot, syncAuthorityCandidateManyEnvironmentsV2(1),
		)
		receipt, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, candidate.Checkpoint())
		if err != nil {
			t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
		}
		advanced := syncRelayWatermarkFromSnapshot(projectID, snapshot, snapshot.InventoryArrivalHead)
		advanced.MembershipGeneration++
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
			t.Fatalf("AdvanceSyncRelayWatermark(after promotion) = (%#v, %v)", got, err)
		}
		replayed, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, candidate.Checkpoint())
		if err != nil || replayed != receipt {
			t.Fatalf("PromoteSyncAuthorityCandidate(receipt replay) = (%#v, %v), want (%#v, nil)", replayed, err, receipt)
		}
	})
}

func TestInstallVerifiedSyncAuthorityUsesFreshInstallButNotExactReplayToEstablishMembershipFloor(t *testing.T) {
	t.Parallel()

	t.Run("fresh install", func(t *testing.T) {
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		projectID := continuity.ProjectID("project-authority-frontier-install-fresh")
		authority := testSyncAuthority()
		floor := syncRelayWatermarkFromAuthorityForTestV1(projectID, authority)
		floor.MembershipGeneration--
		seedSyncRelayFrontierForTestV1(t, store, floor, false)

		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority(fresh) error = %v", err)
		}
		assertSyncRelayWatermarkRowWithKnown(t, store, syncRelayWatermarkFromAuthorityForTestV1(projectID, authority), true)
	})

	t.Run("exact replay", func(t *testing.T) {
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		projectID := continuity.ProjectID("project-authority-frontier-install-replay")
		authority := testSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority(first) error = %v", err)
		}
		want := syncRelayWatermarkFromAuthorityForTestV1(projectID, authority)
		seedSyncRelayFrontierForTestV1(t, store, want, false)

		_, err = store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority)
		assertSyncFrontierCursorFieldV1(t, err, "membership_generation")
		assertSyncRelayWatermarkRowWithKnown(t, store, want, false)
	})
}

func TestCanonicalAttachPathsRequireKnownExactComponentwiseFrontier(t *testing.T) {
	t.Parallel()
	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-authority-frontier-attach")
	authority := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	want := syncRelayWatermarkFromAuthorityForTestV1(projectID, authority)
	seedSyncRelayFrontierForTestV1(t, store, want, false)

	_, err = store.CurrentSyncEnvironmentStates(
		context.Background(), projectID, binding, []continuity.EnvironmentID{continuity.EnvironmentID(authority.Environments[0].EnvironmentID)},
	)
	assertSyncFrontierCursorFieldV1(t, err, "membership_generation")

	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET activation_state = 'attached'
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("seed attached activation state: %v", err)
	}
	_, err = store.ActivateStagedSync(context.Background(), projectID, binding)
	assertSyncFrontierCursorFieldV1(t, err, "membership_generation")
	var activationState string
	if err := store.db.QueryRow(`SELECT activation_state FROM continuity_sync_projects WHERE project_id = ?`, string(projectID)).Scan(&activationState); err != nil {
		t.Fatalf("read activation state: %v", err)
	}
	if activationState != "attached" {
		t.Fatalf("activation state = %q, want attached", activationState)
	}
}

func seedSyncRelayFrontierForTestV1(t *testing.T, store *Store, frontier SyncRelayWatermark, known bool) {
	t.Helper()
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
) VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, channel_id, relay_generation) DO UPDATE SET
  admin_public_key = excluded.admin_public_key,
  membership_generation = excluded.membership_generation,
  relay_head = excluded.relay_head,
  membership_floor_known = excluded.membership_floor_known`,
		string(frontier.ProjectID), frontier.ChannelID[:], frontier.RelayGeneration[:], frontier.AdminPublicKey[:],
		frontier.MembershipGeneration, frontier.RelayHead, boolIntV2(known),
	); err != nil {
		t.Fatalf("seed relay frontier: %v", err)
	}
}

func syncRelayWatermarkFromAuthorityForTestV1(projectID continuity.ProjectID, authority SyncAuthority) SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            authority.ChannelID,
		RelayGeneration:      authority.RelayGeneration,
		AdminPublicKey:       authority.AdminPublicKey,
		MembershipGeneration: authority.MembershipGeneration,
		RelayHead:            authority.InventoryArrivalHead,
	}
}

func assertActiveAuthorityCandidateCheckpointForTestV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	want SyncAuthorityCandidateCheckpoint,
) {
	t.Helper()
	candidate, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("CurrentSyncAuthorityCandidate() = (%#v, %v, %v)", candidate, found, err)
	}
	if candidate.Checkpoint() != want {
		t.Fatalf("active authority candidate checkpoint = %#v, want %#v", candidate.Checkpoint(), want)
	}
}

func assertSyncFrontierCursorFieldV1(t *testing.T, err error, field string) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != SyncErrorCursor || problem.Field != field {
		t.Fatalf("sync error = %#v, want cursor field %q", err, field)
	}
}
