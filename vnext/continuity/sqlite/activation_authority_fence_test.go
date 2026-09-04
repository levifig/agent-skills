package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestActivateStagedSyncRequiresExactLocalAuthorityCutoff(t *testing.T) {
	t.Parallel()

	t.Run("v1 above", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "activation-authority-cutoff-v1-above")
		installTestSyncAuthority(t, store, projectID, testSyncChannelID("activation-authority-cutoff-v1-above"))
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		if binding.AuthorityDigestVersion != 1 || binding.InventoryArrivalHead != 0 {
			t.Fatalf("v1 binding = %#v, want digest version 1 and authority head 0", binding)
		}
		if _, err := store.StageSyncPage(context.Background(), projectID, binding.ChannelID, 0, 1, nil); err != nil {
			t.Fatalf("StageSyncPage(v1 above) error = %v", err)
		}

		_, err := store.ActivateStagedSync(context.Background(), projectID, binding)
		assertActivationAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
		assertActivationAuthorityFenceProgress(t, store, projectID, SyncActivationStaging, 0, 0, 1)
	})

	t.Run("v2 below", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "activation-authority-cutoff-v2-below")
		environments := syncAuthorityCandidateManyEnvironmentsV2(1)
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
		snapshot.InventoryArrivalHead = 1
		authority := syncAuthorityFromSnapshotForBindingTest(snapshot, environments)
		digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
		binding := syncAuthorityBindingForTest(authority, 2, digest)
		watermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
			t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, watermark)
		}

		_, err := store.ActivateStagedSync(context.Background(), projectID, binding)
		assertActivationAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
		assertActivationAuthorityFenceProgress(t, store, projectID, SyncActivationStaging, 0, 0, 0)
	})
}

func TestActivateStagedSyncRejectsActiveRecoveryAndTerminalCandidates(t *testing.T) {
	t.Parallel()

	t.Run("recovery transition", func(t *testing.T) {
		fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "activation-authority-fence")
		binding := SyncAuthorityBinding{
			ChannelID:              fixture.predecessor.Snapshot.ChannelID,
			RelayGeneration:        fixture.predecessor.Snapshot.RelayGeneration,
			AdminPublicKey:         fixture.predecessor.Snapshot.AdminPublicKey,
			MembershipGeneration:   fixture.predecessor.Snapshot.MembershipGeneration,
			InventoryArrivalHead:   fixture.predecessor.Snapshot.InventoryArrivalHead,
			AuthorityDigestVersion: fixture.predecessor.Snapshot.BaseAuthorityDigestVersion,
			AuthorityDigest:        fixture.predecessor.Snapshot.BaseAuthorityDigest,
		}

		_, err := fixture.store.ActivateStagedSync(context.Background(), fixture.projectID, binding)
		assertActivationAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_recovery_transition")
	})

	t.Run("terminal candidate", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "activation-authority-fence", 2)
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		if _, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames, 1_000, 100,
		); err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}

		_, err := store.ActivateStagedSync(context.Background(), projectID, binding)
		assertActivationAuthorityFenceProblem(t, err, SyncErrorConflict, "terminal_candidate")
	})
}

func TestActivateStagedSyncAttachedRetryIgnoresActiveTerminalCandidate(t *testing.T) {
	t.Parallel()

	_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "activation-attached-terminal-retry", 2)
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET activation_state = 'attached'
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("seed attached activation receipt: %v", err)
	}
	want := syncProgressForActivationFenceTest(t, store, projectID)

	got, err := store.ActivateStagedSync(context.Background(), projectID, binding)
	if err != nil || got != want {
		t.Fatalf("ActivateStagedSync(attached terminal retry) = (%#v, %v), want (%#v, nil)", got, err, want)
	}
	current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after activation retry) = (%#v, %v, %v), want unchanged %#v", current, found, err, candidate)
	}
}

func TestActivateStagedSyncV1AttachedRetryChecksWatermarkBeforeRecoveryGate(t *testing.T) {
	t.Parallel()

	fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "activation-attached-recovery-retry")
	binding := SyncAuthorityBinding{
		ChannelID:              fixture.predecessor.Snapshot.ChannelID,
		RelayGeneration:        fixture.predecessor.Snapshot.RelayGeneration,
		AdminPublicKey:         fixture.predecessor.Snapshot.AdminPublicKey,
		MembershipGeneration:   fixture.predecessor.Snapshot.MembershipGeneration,
		InventoryArrivalHead:   fixture.predecessor.Snapshot.InventoryArrivalHead,
		AuthorityDigestVersion: fixture.predecessor.Snapshot.BaseAuthorityDigestVersion,
		AuthorityDigest:        fixture.predecessor.Snapshot.BaseAuthorityDigest,
	}
	if _, err := fixture.store.db.Exec(`
UPDATE continuity_sync_projects
SET activation_state = 'attached'
WHERE project_id = ?`, string(fixture.projectID)); err != nil {
		t.Fatalf("seed attached activation receipt: %v", err)
	}
	wantProgress := syncProgressForActivationFenceTest(t, fixture.store, fixture.projectID)
	wantTransition, found, err := fixture.store.CurrentSyncAuthorityRecoveryTransition(context.Background(), fixture.projectID)
	if err != nil || !found {
		t.Fatalf("CurrentSyncAuthorityRecoveryTransition(before) = (%#v, %v, %v), want retained transition", wantTransition, found, err)
	}

	_, err = fixture.store.ActivateStagedSync(context.Background(), fixture.projectID, binding)
	assertActivationAuthorityFenceProblem(t, err, SyncErrorCursor, "membership_generation")
	if got := syncProgressForActivationFenceTest(t, fixture.store, fixture.projectID); got != wantProgress {
		t.Fatalf("activation progress after v1 recovery retry = %#v, want unchanged %#v", got, wantProgress)
	}
	gotTransition, found, err := fixture.store.CurrentSyncAuthorityRecoveryTransition(context.Background(), fixture.projectID)
	if err != nil || !found || gotTransition != wantTransition {
		t.Fatalf("CurrentSyncAuthorityRecoveryTransition(after) = (%#v, %v, %v), want unchanged %#v", gotTransition, found, err, wantTransition)
	}
}

func TestActivateStagedSyncV2AttachedProvesExactCurrentAuthority(t *testing.T) {
	t.Parallel()

	t.Run("exact retry", func(t *testing.T) {
		store, projectID, binding, want := attachedV2ActivationFenceFixture(t, "activation-v2-attached-exact")

		got, err := store.ActivateStagedSync(context.Background(), projectID, binding)
		if err != nil || got != want {
			t.Fatalf("ActivateStagedSync(v2 exact attached retry) = (%#v, %v), want (%#v, nil)", got, err, want)
		}
	})

	t.Run("authority advanced beyond attached progress", func(t *testing.T) {
		store, projectID, binding, want := attachedV2ActivationFenceFixture(t, "activation-v2-attached-behind")
		authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthority() error = %v", err)
		}
		snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, binding.AuthorityDigestVersion, binding.AuthorityDigest)
		snapshot.InventoryArrivalHead++
		watermark := syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, snapshot)
		if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
			t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, watermark)
		}
		candidate := stageSyncAuthorityCandidateInventoryV2(t, store, projectID, snapshot, authority.Environments)
		if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, candidate.Checkpoint()); err != nil {
			t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
		}
		advanced := currentSyncAuthorityBindingForTest(t, store, projectID)

		_, err = store.ActivateStagedSync(context.Background(), projectID, advanced)
		assertActivationAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
		if got := syncProgressForActivationFenceTest(t, store, projectID); got != want {
			t.Fatalf("activation progress after v2 authority advance = %#v, want unchanged %#v", got, want)
		}
	})

	t.Run("active authority candidate", func(t *testing.T) {
		store, projectID, binding, want := attachedV2ActivationFenceFixture(t, "activation-v2-attached-candidate")
		authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthority() error = %v", err)
		}
		candidate := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)

		_, err = store.ActivateStagedSync(context.Background(), projectID, binding)
		assertActivationAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_candidate")
		if got := syncProgressForActivationFenceTest(t, store, projectID); got != want {
			t.Fatalf("activation progress with v2 authority candidate = %#v, want unchanged %#v", got, want)
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("CurrentSyncAuthorityCandidate() = (%#v, %v, %v), want unchanged %#v", current, found, err, candidate)
		}
	})
}

func TestActivateStagedSyncCASZeroRollsBack(t *testing.T) {
	t.Parallel()

	store, projectID := storeWithLocalRoot(t, "activation-final-cas")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("activation-final-cas"))
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	want := syncProgressForActivationFenceTest(t, store, projectID)
	if _, err := store.db.Exec(`
CREATE TEMP TRIGGER ignore_activation_after_progress_mutation
BEFORE UPDATE OF activation_state ON continuity_sync_projects
WHEN NEW.activation_state = 'attached'
BEGIN
  UPDATE continuity_sync_projects
  SET relay_head = relay_head + 1
  WHERE project_id = NEW.project_id;
  SELECT RAISE(IGNORE);
END`); err != nil {
		t.Fatalf("create activation CAS trigger: %v", err)
	}

	_, err := store.ActivateStagedSync(context.Background(), projectID, binding)
	assertActivationAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
	if got := syncProgressForActivationFenceTest(t, store, projectID); got != want {
		t.Fatalf("activation progress after CAS rollback = %#v, want %#v", got, want)
	}
}

func assertActivationAuthorityFenceProblem(t *testing.T, err error, wantCode SyncErrorCode, wantField string) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *SyncError code %q at %q", err, wantCode, wantField)
	}
	if problem.Code != wantCode || problem.Field != wantField {
		t.Fatalf("error = %#v, want code %q at %q", problem, wantCode, wantField)
	}
}

func assertActivationAuthorityFenceProgress(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	wantState SyncActivationState,
	wantApplied,
	wantDownloaded,
	wantRelayHead int64,
) {
	t.Helper()
	progress := syncProgressForActivationFenceTest(t, store, projectID)
	if progress.ActivationState != wantState || progress.AppliedCursor != wantApplied ||
		progress.DownloadedCursor != wantDownloaded || progress.RelayHead != wantRelayHead {
		t.Fatalf(
			"activation progress = %#v, want state %q and applied/downloaded/relay %d/%d/%d",
			progress, wantState, wantApplied, wantDownloaded, wantRelayHead,
		)
	}
}

func syncProgressForActivationFenceTest(t *testing.T, store *Store, projectID continuity.ProjectID) SyncProgress {
	t.Helper()
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	return progress
}

func attachedV2ActivationFenceFixture(
	t *testing.T,
	name string,
) (*Store, continuity.ProjectID, SyncAuthorityBinding, SyncProgress) {
	t.Helper()
	store, projectID := storeWithAppliedRoot(t, name)
	binding := promoteSyncAuthorityArrivalHeadForTest(t, store, projectID, 1)
	if binding.AuthorityDigestVersion != 2 {
		t.Fatalf("promoted authority digest version = %d, want 2", binding.AuthorityDigestVersion)
	}
	progress, err := store.ActivateStagedSync(context.Background(), projectID, binding)
	if err != nil {
		t.Fatalf("ActivateStagedSync(initial v2) error = %v", err)
	}
	if progress.ActivationState != SyncActivationAttached || progress.DownloadedCursor != 1 ||
		progress.AppliedCursor != 1 || progress.RelayHead != 1 {
		t.Fatalf("initial v2 activation progress = %#v, want attached 1/1/1", progress)
	}
	return store, projectID, binding, progress
}
