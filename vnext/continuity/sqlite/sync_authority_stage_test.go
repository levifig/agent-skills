package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestStageSyncPageUnderAuthorityStagesOnlyAtExactCanonicalSnapshot(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-stage-authority")
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	stale := currentSyncAuthorityBindingForTest(t, store, projectID)
	current := promoteSyncAuthorityArrivalHeadForTest(t, store, projectID, 1)

	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, stale, 0, 0, nil,
	); err == nil {
		t.Fatal("StageSyncPageUnderAuthority(stale binding) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress(after stale binding) error = %v", err)
	}
	if progress.DownloadedCursor != 0 || progress.RelayHead != 0 {
		t.Fatalf("progress after stale binding = %#v, want untouched cursors", progress)
	}

	frame := testOpaqueFrame(1, "authority-bound")
	for _, relayHead := range []int64{0, 2} {
		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, current, 0, relayHead, []OpaqueSyncFrame{frame},
		)
		assertSyncErrorCode(t, err, SyncErrorCursor)
		progress, err = store.CurrentSyncProgress(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncProgress(after relay head %d) error = %v", relayHead, err)
		}
		if progress.DownloadedCursor != 0 || progress.RelayHead != 0 {
			t.Fatalf("progress after relay head %d = %#v, want untouched cursors", relayHead, progress)
		}
	}

	progress, err = store.StageSyncPageUnderAuthority(
		context.Background(), projectID, current, 0, 1, []OpaqueSyncFrame{frame},
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(current binding) error = %v", err)
	}
	if progress.DownloadedCursor != 1 || progress.RelayHead != 1 {
		t.Fatalf("authority-bound progress = %#v, want downloaded/head 1", progress)
	}
	pending, err := store.PendingSyncFrames(context.Background(), projectID, 1)
	if err != nil {
		t.Fatalf("PendingSyncFrames() error = %v", err)
	}
	if len(pending) != 1 || !opaqueSyncFrameEqual(pending[0], frame) {
		t.Fatalf("pending = %#v, want exact authority-bound frame", pending)
	}
	replayed, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, current, 0, 1, []OpaqueSyncFrame{frame},
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(exact retry) error = %v", err)
	}
	if replayed != progress {
		t.Fatalf("exact retry progress = %#v, want %#v", replayed, progress)
	}
	pending, err = store.PendingSyncFrames(context.Background(), projectID, 2)
	if err != nil {
		t.Fatalf("PendingSyncFrames(after exact retry) error = %v", err)
	}
	if len(pending) != 1 || !opaqueSyncFrameEqual(pending[0], frame) {
		t.Fatalf("pending after exact retry = %#v, want one exact frame", pending)
	}

	advanced := promoteSyncAuthorityArrivalHeadForTest(t, store, projectID, 2)
	_, err = store.StageSyncPageUnderAuthority(
		context.Background(), projectID, current, 0, 1, []OpaqueSyncFrame{frame},
	)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	afterPromotion, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress(after stale retry) error = %v", err)
	}
	if afterPromotion != progress {
		t.Fatalf("progress after stale retry = %#v, want %#v", afterPromotion, progress)
	}

	replayed, err = store.StageSyncPageUnderAuthority(
		context.Background(), projectID, advanced, 0, 2, []OpaqueSyncFrame{frame},
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(replay after promotion) error = %v", err)
	}
	if replayed.DownloadedCursor != 1 || replayed.AppliedCursor != 0 || replayed.RelayHead != 2 {
		t.Fatalf("replay after promotion progress = %#v, want downloaded 1 and relay head 2", replayed)
	}
	second := testOpaqueFrame(2, "authority-bound-second")
	progress, err = store.StageSyncPageUnderAuthority(
		context.Background(), projectID, advanced, 1, 2, []OpaqueSyncFrame{second},
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(suffix after promotion) error = %v", err)
	}
	if progress.DownloadedCursor != 2 || progress.AppliedCursor != 0 || progress.RelayHead != 2 {
		t.Fatalf("suffix after promotion progress = %#v, want downloaded/head 2", progress)
	}
	pending, err = store.PendingSyncFrames(context.Background(), projectID, 2)
	if err != nil {
		t.Fatalf("PendingSyncFrames(after promoted suffix) error = %v", err)
	}
	if len(pending) != 2 || !opaqueSyncFrameEqual(pending[0], frame) || !opaqueSyncFrameEqual(pending[1], second) {
		t.Fatalf("pending after promoted suffix = %#v, want exact two-frame history", pending)
	}
}

func TestStageSyncPageUnderAuthorityRequiresKnownExactFrontier(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-stage-authority-frontier")
	authority := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	frontier := syncRelayWatermarkFromAuthorityForTestV1(projectID, authority)
	seedSyncRelayFrontierForTestV1(t, store, frontier, false)

	_, err = store.StageSyncPageUnderAuthority(context.Background(), projectID, binding, 0, 0, nil)
	assertSyncFrontierCursorFieldV1(t, err, "membership_generation")
	progress, readErr := store.CurrentSyncProgress(context.Background(), projectID)
	if readErr != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", readErr)
	}
	if progress.DownloadedCursor != 0 || progress.RelayHead != 0 {
		t.Fatalf("progress after unknown frontier = %#v, want untouched cursors", progress)
	}
}
