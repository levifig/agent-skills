package sqlite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestStageSyncPageUnderAuthorityRejectsCompetingTransitions(t *testing.T) {
	t.Parallel()

	t.Run("recovery transition", func(t *testing.T) {
		fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "stage-authority-fence")
		binding := SyncAuthorityBinding{
			ChannelID:              fixture.predecessor.Snapshot.ChannelID,
			RelayGeneration:        fixture.predecessor.Snapshot.RelayGeneration,
			AdminPublicKey:         fixture.predecessor.Snapshot.AdminPublicKey,
			MembershipGeneration:   fixture.predecessor.Snapshot.MembershipGeneration,
			InventoryArrivalHead:   fixture.predecessor.Snapshot.InventoryArrivalHead,
			AuthorityDigestVersion: fixture.predecessor.Snapshot.BaseAuthorityDigestVersion,
			AuthorityDigest:        fixture.predecessor.Snapshot.BaseAuthorityDigest,
		}

		_, err := fixture.store.StageSyncPageUnderAuthority(
			context.Background(), fixture.projectID, binding, 0, binding.InventoryArrivalHead, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_recovery_transition")
	})

	t.Run("stale terminal candidate", func(t *testing.T) {
		_, store, projectID, _, frames := terminalCandidateMixedFixtureV1(t, "stage-authority-fence", 2)
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}
		advanced := promoteSyncAuthorityArrivalHeadForTest(t, store, projectID, 2)

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, advanced, 2, 2, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority")
		current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
		if currentErr != nil || !found || current != candidate {
			t.Fatalf("CurrentTerminalCandidate() = (%#v, %v, %v), want unchanged %#v", current, found, currentErr, candidate)
		}
		if _, err := store.StageSyncPage(
			context.Background(), projectID, advanced.ChannelID, 2, 2, nil,
		); err != nil {
			t.Fatalf("StageSyncPage(raw compatibility) error = %v", err)
		}
		assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)
	})

	t.Run("ordinary authority candidate", func(t *testing.T) {
		store := openSyncStore(t, "stage-authority-candidate-fence")
		projectID := continuity.ProjectID("project-stage-authority-candidate-fence")
		authority := testActiveSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		candidate := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)

		_, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_candidate")
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("CurrentSyncAuthorityCandidate() = (%#v, %v, %v), want unchanged %#v", current, found, err, candidate)
		}

		progress, err := store.StageSyncPage(
			context.Background(), projectID, binding.ChannelID, 0, binding.InventoryArrivalHead, nil,
		)
		if err != nil {
			t.Fatalf("StageSyncPage(raw compatibility) error = %v", err)
		}
		if progress.DownloadedCursor != 0 || progress.AppliedCursor != 0 || progress.RelayHead != 0 {
			t.Fatalf("raw compatibility progress = %#v, want zero cursors", progress)
		}
		current, found, err = store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("CurrentSyncAuthorityCandidate(after raw stage) = (%#v, %v, %v), want unchanged %#v", current, found, err, candidate)
		}
	})
}

func TestStageSyncPageUnderAuthorityAllowsMatchingTerminalCandidate(t *testing.T) {
	t.Parallel()

	store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(
		t, "page-staging", 3,
	)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque[:1],
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(first page) error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames[:1], 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}

	replay, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque[:1],
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(exact replay) error = %v", err)
	}
	if replay.DownloadedCursor != 1 || replay.RelayHead != binding.InventoryArrivalHead {
		t.Fatalf("exact replay progress = %#v, want downloaded 1 at authority cutoff", replay)
	}
	assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)

	empty, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, nil,
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(empty page) error = %v", err)
	}
	if empty != replay {
		t.Fatalf("empty page progress = %#v, want %#v", empty, replay)
	}
	assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)

	complete, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, opaque[1:],
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(suffix) error = %v", err)
	}
	if complete.DownloadedCursor != binding.InventoryArrivalHead || complete.RelayHead != binding.InventoryArrivalHead {
		t.Fatalf("suffix progress = %#v, want downloaded authority cutoff", complete)
	}
	assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)
}

func TestStageSyncPageUnderAuthorityCASZeroRollsBack(t *testing.T) {
	t.Parallel()

	t.Run("new page", func(t *testing.T) {
		store, projectID, binding := stageAuthorityFenceFixture(t, "stage-authority-cas-new", 1)
		want := syncProgressForStageAuthorityFenceTest(t, store, projectID)
		frame := testOpaqueFrame(1, "stage-authority-cas-new")
		installStageAuthorityCASIgnoreTrigger(t, store)

		_, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, 1, []OpaqueSyncFrame{frame},
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityFenceState(t, store, projectID, want, 0)
	})

	t.Run("empty page", func(t *testing.T) {
		store, projectID, binding := stageAuthorityFenceFixture(t, "stage-authority-cas-empty", 0)
		want := syncProgressForStageAuthorityFenceTest(t, store, projectID)
		installStageAuthorityCASIgnoreTrigger(t, store)

		_, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, 0, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityFenceState(t, store, projectID, want, 0)
	})

	t.Run("exact replay", func(t *testing.T) {
		store, projectID, binding := stageAuthorityFenceFixture(t, "stage-authority-cas-replay", 1)
		frame := testOpaqueFrame(1, "stage-authority-cas-replay")
		want, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, 1, []OpaqueSyncFrame{frame},
		)
		if err != nil {
			t.Fatalf("StageSyncPageUnderAuthority(initial) error = %v", err)
		}
		installStageAuthorityCASIgnoreTrigger(t, store)

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, 1, []OpaqueSyncFrame{frame},
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityFenceState(t, store, projectID, want, 1)
	})
}

func TestStageSyncPageUnderAuthorityCASDetectsPostInsertDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body func(SyncAuthorityBinding) string
	}{
		{
			name: "project header",
			body: func(SyncAuthorityBinding) string {
				drifted := testAuthorityDigest(0xd1)
				return fmt.Sprintf(`
UPDATE continuity_sync_projects
SET admin_public_key = X'%x'
WHERE project_id = NEW.project_id;`, drifted)
			},
		},
		{
			name: "canonical authority",
			body: func(SyncAuthorityBinding) string {
				drifted := testAuthorityDigest(0xd2)
				return fmt.Sprintf(`
UPDATE continuity_sync_authorities
SET authority_digest = X'%x'
WHERE project_id = NEW.project_id;`, drifted)
			},
		},
		{
			name: "relay watermark",
			body: func(SyncAuthorityBinding) string {
				return `
UPDATE continuity_sync_relay_watermarks
SET membership_generation = membership_generation + 1
WHERE project_id = NEW.project_id;`
			},
		},
		{
			name: "recovery transition",
			body: func(SyncAuthorityBinding) string {
				attemptID := testAuthorityDigest(0xd7)
				writerCertificateID := testAuthorityDigest(0xd8)
				return fmt.Sprintf(`
INSERT INTO continuity_sync_authority_recovery_transitions(
  project_id, attempt_id, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation
)
SELECT
  NEW.project_id, X'%x', NULL, candidate_id,
  'environment-trigger', X'%x', 1
FROM continuity_sync_authority_candidates
WHERE project_id = NEW.project_id AND state = 'promoted'
ORDER BY candidate_id
LIMIT 1;`, attemptID, writerCertificateID)
			},
		},
		{
			name: "active terminal candidate",
			body: func(binding SyncAuthorityBinding) string {
				candidateID := testAuthorityDigest(0xd5)
				rollingDigest := testAuthorityDigest(0xd6)
				return fmt.Sprintf(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES(
  NEW.project_id, X'%x', 'staging', X'%x', X'%x',
  %d, X'%x', 1, 1, 1, X'%x', NULL, NULL
);`, candidateID, binding.ChannelID, binding.RelayGeneration,
					binding.MembershipGeneration, binding.AuthorityDigest, rollingDigest)
			},
		},
		{
			name: "active authority candidate",
			body: func(binding SyncAuthorityBinding) string {
				candidateID := testAuthorityDigest(0xd3)
				rollingDigest := testAuthorityDigest(0xd4)
				return fmt.Sprintf(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
)
SELECT
  NEW.project_id, X'%x', 'staging', project.channel_id, project.relay_generation,
  project.admin_public_key, project.membership_generation, authority.inventory_arrival_head,
  authority.digest_version, authority.authority_digest,
  1, 1, 'environment-trigger', X'%x', 2, NULL, 'ordinary'
FROM continuity_sync_projects AS project
JOIN continuity_sync_authorities AS authority ON authority.project_id = project.project_id
WHERE project.project_id = NEW.project_id
  AND authority.digest_version = %d AND authority.authority_digest = X'%x';`,
					candidateID, rollingDigest, binding.AuthorityDigestVersion, binding.AuthorityDigest)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding := stageAuthorityFenceFixture(t, "stage-authority-post-insert-"+syncSlug(test.name), 1)
			want := syncProgressForStageAuthorityFenceTest(t, store, projectID)
			installStageAuthorityAfterInboxInsertTrigger(t, store, test.body(binding))
			frame := testOpaqueFrame(1, "stage-authority-post-insert-"+test.name)

			_, err := store.StageSyncPageUnderAuthority(
				context.Background(), projectID, binding, 0, 1, []OpaqueSyncFrame{frame},
			)
			assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
			assertStageAuthorityFenceState(t, store, projectID, want, 0)
			if got := currentSyncAuthorityBindingForTest(t, store, projectID); got != binding {
				t.Fatalf("authority binding after post-insert drift = %#v, want %#v", got, binding)
			}
			assertSyncRelayWatermarkRow(t, store, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding))
			var candidates int
			if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND state IN ('staging', 'ready')`, string(projectID)).Scan(&candidates); err != nil {
				t.Fatalf("count authority candidates: %v", err)
			}
			if candidates != 0 {
				t.Fatalf("authority candidates after post-insert drift = %d, want 0", candidates)
			}
			var terminalCandidates int
			if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_terminal_candidates
WHERE project_id = ? AND state = 'staging'`, string(projectID)).Scan(&terminalCandidates); err != nil {
				t.Fatalf("count terminal candidates: %v", err)
			}
			if terminalCandidates != 0 {
				t.Fatalf("terminal candidates after post-insert drift = %d, want 0", terminalCandidates)
			}
			var recoveryTransitions int
			if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_authority_recovery_transitions
WHERE project_id = ?`, string(projectID)).Scan(&recoveryTransitions); err != nil {
				t.Fatalf("count recovery transitions: %v", err)
			}
			if recoveryTransitions != 0 {
				t.Fatalf("recovery transitions after post-insert drift = %d, want 0", recoveryTransitions)
			}
		})
	}
}

func TestStageSyncPageUnderAuthorityRejectsStaleTerminalProgress(t *testing.T) {
	t.Parallel()

	t.Run("applied prefix", func(t *testing.T) {
		store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(
			t, "stale-applied-prefix", 2,
		)
		if _, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
		); err != nil {
			t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
		}
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames[:1], 1_000, 100,
		)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}
		if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET applied_cursor = 1
WHERE project_id = ?`, string(projectID)); err != nil {
			t.Fatalf("advance applied cursor: %v", err)
		}

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 2, binding.InventoryArrivalHead, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "applied_cursor")
		assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)
	})

	t.Run("through beyond downloaded", func(t *testing.T) {
		store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(
			t, "stale-downloaded-prefix", 2,
		)
		if _, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
		); err != nil {
			t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
		}
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}
		if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = 1
WHERE project_id = ?`, string(projectID)); err != nil {
			t.Fatalf("regress downloaded cursor: %v", err)
		}

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)
	})
}

func TestStageSyncPageUnderAuthorityCASDetectsTerminalCandidateDrift(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		installBody func(TerminalCandidate) string
	}{
		{
			name: "header mutation before progress update",
			installBody: func(TerminalCandidate) string {
				drifted := testAuthorityDigest(0xe1)
				return fmt.Sprintf(`
UPDATE continuity_sync_terminal_candidates
SET rolling_candidate_digest = X'%x'
WHERE project_id = NEW.project_id AND state = 'staging';`, drifted)
			},
		},
		{
			name: "promotion before progress update",
			installBody: func(TerminalCandidate) string {
				corpus := testAuthorityDigest(0xe2)
				return fmt.Sprintf(`
UPDATE continuity_sync_terminal_candidates
SET state = 'promoted', post_promotion_corpus_digest = X'%x',
    resulting_applied_cursor = through_arrival_sequence
WHERE project_id = NEW.project_id AND state = 'staging';`, corpus)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(
				t, "terminal-drift-"+syncSlug(test.name), 2,
			)
			wantProgress, err := store.StageSyncPageUnderAuthority(
				context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque[:1],
			)
			if err != nil {
				t.Fatalf("StageSyncPageUnderAuthority(first page) error = %v", err)
			}
			candidate, err := store.StageVerifiedTerminalCandidateChunk(
				context.Background(), projectID, binding, frames[:1], 1_000, 100,
			)
			if err != nil {
				t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
			}
			installStageAuthorityAfterInboxInsertTrigger(t, store, test.installBody(candidate))

			_, err = store.StageSyncPageUnderAuthority(
				context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, opaque[1:],
			)
			assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
			assertStageAuthorityFenceState(t, store, projectID, wantProgress, 1)
			assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)
		})
	}
}

func TestStageSyncPageUnderAuthorityPostUpdateControlAudit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body func(SyncAuthorityBinding) string
	}{
		{
			name: "project header",
			body: func(SyncAuthorityBinding) string {
				drifted := testAuthorityDigest(0xf1)
				return fmt.Sprintf(`
UPDATE continuity_sync_projects
SET admin_public_key = X'%x'
WHERE project_id = NEW.project_id;`, drifted)
			},
		},
		{
			name: "project progress",
			body: func(SyncAuthorityBinding) string {
				return `
UPDATE continuity_sync_projects
SET activation_state = 'attached'
WHERE project_id = NEW.project_id;`
			},
		},
		{
			name: "canonical authority",
			body: func(SyncAuthorityBinding) string {
				drifted := testAuthorityDigest(0xf2)
				return fmt.Sprintf(`
UPDATE continuity_sync_authorities
SET authority_digest = X'%x'
WHERE project_id = NEW.project_id;`, drifted)
			},
		},
		{
			name: "relay watermark",
			body: func(SyncAuthorityBinding) string {
				return `
UPDATE continuity_sync_relay_watermarks
SET membership_generation = membership_generation + 1
WHERE project_id = NEW.project_id;`
			},
		},
		{
			name: "recovery transition",
			body: func(SyncAuthorityBinding) string {
				attemptID := testAuthorityDigest(0xf3)
				writerCertificateID := testAuthorityDigest(0xf4)
				return fmt.Sprintf(`
INSERT INTO continuity_sync_authority_recovery_transitions(
  project_id, attempt_id, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation
)
SELECT
  NEW.project_id, X'%x', NULL, candidate_id,
  'environment-trigger', X'%x', 1
FROM continuity_sync_authority_candidates
WHERE project_id = NEW.project_id AND state = 'promoted'
ORDER BY candidate_id
LIMIT 1;`, attemptID, writerCertificateID)
			},
		},
		{
			name: "active authority candidate",
			body: func(binding SyncAuthorityBinding) string {
				candidateID := testAuthorityDigest(0xf5)
				rollingDigest := testAuthorityDigest(0xf6)
				return fmt.Sprintf(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
)
SELECT
  NEW.project_id, X'%x', 'staging', project.channel_id, project.relay_generation,
  project.admin_public_key, project.membership_generation, authority.inventory_arrival_head,
  authority.digest_version, authority.authority_digest,
  1, 1, 'environment-trigger', X'%x', 2, NULL, 'ordinary'
FROM continuity_sync_projects AS project
JOIN continuity_sync_authorities AS authority ON authority.project_id = project.project_id
WHERE project.project_id = NEW.project_id
  AND authority.digest_version = %d AND authority.authority_digest = X'%x';`,
					candidateID, rollingDigest, binding.AuthorityDigestVersion, binding.AuthorityDigest)
			},
		},
		{
			name: "staged inbox",
			body: func(SyncAuthorityBinding) string {
				return `
UPDATE continuity_sync_inbox
SET frame_bytes = X'01'
WHERE project_id = NEW.project_id AND arrival_sequence = 1;`
			},
		},
		{
			name: "staged inbox state",
			body: func(SyncAuthorityBinding) string {
				return `
UPDATE continuity_sync_inbox
SET state = 'quarantined'
WHERE project_id = NEW.project_id AND arrival_sequence = 1;`
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding := stageAuthorityFenceFixture(
				t, "post-update-control-"+syncSlug(test.name), 1,
			)
			wantProgress := syncProgressForStageAuthorityFenceTest(t, store, projectID)
			installStageAuthorityAfterProgressUpdateTrigger(t, store, test.body(binding))
			frame := testOpaqueFrame(1, "post-update-control-"+test.name)

			_, err := store.StageSyncPageUnderAuthority(
				context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, []OpaqueSyncFrame{frame},
			)
			assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
			assertStageAuthorityFenceState(t, store, projectID, wantProgress, 0)
			if got := currentSyncAuthorityBindingForTest(t, store, projectID); got != binding {
				t.Fatalf("authority binding after post-update drift = %#v, want %#v", got, binding)
			}
			assertSyncRelayWatermarkRow(t, store, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding))
			assertStageAuthorityNoCompetingControlRows(t, store, projectID)
		})
	}
}

func TestStageSyncPageUnderAuthorityPostUpdateAuditCoversEarlyRoutes(t *testing.T) {
	t.Parallel()

	t.Run("exact replay", func(t *testing.T) {
		store, projectID, binding := stageAuthorityFenceFixture(t, "post-update-replay", 1)
		frame := testOpaqueFrame(1, "post-update-replay")
		wantProgress, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, []OpaqueSyncFrame{frame},
		)
		if err != nil {
			t.Fatalf("StageSyncPageUnderAuthority(initial) error = %v", err)
		}
		installStageAuthorityAfterProgressUpdateTrigger(t, store, `
UPDATE continuity_sync_inbox
SET frame_bytes = X'01'
WHERE project_id = NEW.project_id AND arrival_sequence = 1;`)

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, []OpaqueSyncFrame{frame},
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityFenceState(t, store, projectID, wantProgress, 1)
		pending, pendingErr := store.PendingSyncFrames(context.Background(), projectID, 2)
		if pendingErr != nil {
			t.Fatalf("PendingSyncFrames(after replay rollback) error = %v", pendingErr)
		}
		if len(pending) != 1 || !opaqueSyncFrameEqual(pending[0], frame) {
			t.Fatalf("pending after replay rollback = %#v, want exact retained frame", pending)
		}
		if got := currentSyncAuthorityBindingForTest(t, store, projectID); got != binding {
			t.Fatalf("authority binding after replay rollback = %#v, want %#v", got, binding)
		}
	})

	t.Run("empty page", func(t *testing.T) {
		store, projectID, binding := stageAuthorityFenceFixture(t, "post-update-empty", 1)
		frame := testOpaqueFrame(1, "post-update-empty")
		wantProgress, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, []OpaqueSyncFrame{frame},
		)
		if err != nil {
			t.Fatalf("StageSyncPageUnderAuthority(initial) error = %v", err)
		}
		installStageAuthorityAfterProgressUpdateTrigger(t, store, `
UPDATE continuity_sync_relay_watermarks
SET membership_generation = membership_generation + 1
WHERE project_id = NEW.project_id;`)

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, nil,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityFenceState(t, store, projectID, wantProgress, 1)
		assertSyncRelayWatermarkRow(t, store, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding))
	})

	t.Run("applied-prefix replay", func(t *testing.T) {
		store, projectID, binding, opaque, verified := applySyncBatchV2AuthorityFencePrepared(
			t, "post-update-applied-replay", 1,
		)
		if _, err := store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
		); err != nil {
			t.Fatalf("StageSyncPageUnderAuthority(initial) error = %v", err)
		}
		wantProgress, err := store.ApplySyncBatch(
			context.Background(), projectID, binding, verified, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("ApplySyncBatch() error = %v", err)
		}
		frameKind, frameBytes, err := opaqueSyncFrameStorageV1(opaque[0])
		if err != nil {
			t.Fatalf("opaqueSyncFrameStorageV1() error = %v", err)
		}
		installStageAuthorityAfterProgressUpdateTrigger(t, store, fmt.Sprintf(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES(NEW.project_id, 1, X'%x', '%s', X'%x', 'staged');`,
			opaque[0].EnvelopeDigest, frameKind, frameBytes))

		_, err = store.StageSyncPageUnderAuthority(
			context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
		)
		assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
		assertStageAuthorityFenceState(t, store, projectID, wantProgress, 0)
	})
}

func TestStageSyncPageUnderAuthorityPostUpdateCandidateAudit(t *testing.T) {
	t.Parallel()

	store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(
		t, "post-update-candidate-audit", 2,
	)
	wantProgress, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque[:1],
	)
	if err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(first page) error = %v", err)
	}
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames[:1], 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	drifted := testAuthorityDigest(0xe3)
	if _, err := store.db.Exec(fmt.Sprintf(`
CREATE TEMP TRIGGER mutate_terminal_candidate_after_authority_stage_progress
AFTER UPDATE OF downloaded_cursor, relay_head ON continuity_sync_projects
BEGIN
  UPDATE continuity_sync_terminal_candidates
  SET rolling_candidate_digest = X'%x'
  WHERE project_id = NEW.project_id AND state = 'staging';
END`, drifted)); err != nil {
		t.Fatalf("create post-update terminal candidate trigger: %v", err)
	}

	_, err = store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, opaque[1:],
	)
	assertStageAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
	assertStageAuthorityFenceState(t, store, projectID, wantProgress, 1)
	assertStageAuthorityTerminalCandidate(t, store, projectID, candidate)
}

func stageAuthorityFenceFixture(
	t *testing.T,
	name string,
	authorityHead int64,
) (*Store, continuity.ProjectID, SyncAuthorityBinding) {
	t.Helper()
	store := openSyncStore(t, name)
	projectID := continuity.ProjectID("project-" + syncSlug(name))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	if authorityHead != 0 {
		binding = promoteSyncAuthorityArrivalHeadForTest(t, store, projectID, authorityHead)
	}
	return store, projectID, binding
}

func installStageAuthorityCASIgnoreTrigger(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`
CREATE TEMP TRIGGER ignore_authority_stage_after_progress_mutation
BEFORE UPDATE OF downloaded_cursor, relay_head ON continuity_sync_projects
BEGIN
  UPDATE continuity_sync_projects
  SET relay_head = relay_head + 1
  WHERE project_id = NEW.project_id;
  SELECT RAISE(IGNORE);
END`); err != nil {
		t.Fatalf("create authority stage CAS trigger: %v", err)
	}
}

func installStageAuthorityAfterInboxInsertTrigger(t *testing.T, store *Store, body string) {
	t.Helper()
	statement := fmt.Sprintf(`
CREATE TEMP TRIGGER mutate_authority_stage_after_inbox_insert
AFTER INSERT ON continuity_sync_inbox
BEGIN
%s
END`, body)
	if _, err := store.db.Exec(statement); err != nil {
		t.Fatalf("create authority stage post-insert trigger: %v", err)
	}
}

func installStageAuthorityAfterProgressUpdateTrigger(t *testing.T, store *Store, body string) {
	t.Helper()
	statement := fmt.Sprintf(`
CREATE TEMP TRIGGER mutate_authority_stage_after_progress_update
AFTER UPDATE OF downloaded_cursor, relay_head ON continuity_sync_projects
BEGIN
%s
END`, body)
	if _, err := store.db.Exec(statement); err != nil {
		t.Fatalf("create authority stage post-update trigger: %v", err)
	}
}

func assertStageAuthorityFenceProblem(t *testing.T, err error, wantCode SyncErrorCode, wantField string) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *SyncError code %q at %q", err, wantCode, wantField)
	}
	if problem.Code != wantCode || problem.Field != wantField {
		t.Fatalf("error = %#v, want code %q at %q", problem, wantCode, wantField)
	}
}

func assertStageAuthorityFenceState(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	wantProgress SyncProgress,
	wantInbox int,
) {
	t.Helper()
	if got := syncProgressForStageAuthorityFenceTest(t, store, projectID); got != wantProgress {
		t.Fatalf("sync progress after authority stage CAS refusal = %#v, want %#v", got, wantProgress)
	}
	var inbox int
	if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_inbox
WHERE project_id = ?`, string(projectID)).Scan(&inbox); err != nil {
		t.Fatalf("count authority stage inbox: %v", err)
	}
	if inbox != wantInbox {
		t.Fatalf("authority stage inbox rows = %d, want %d", inbox, wantInbox)
	}
}

func assertStageAuthorityTerminalCandidate(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	want TerminalCandidate,
) {
	t.Helper()
	candidate, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || candidate != want {
		t.Fatalf("CurrentTerminalCandidate() = (%#v, %v, %v), want (%#v, true, nil)", candidate, found, err, want)
	}
}

func assertStageAuthorityNoCompetingControlRows(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
) {
	t.Helper()
	var activeAuthorityCandidates, recoveryTransitions int
	if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND state IN ('staging', 'ready')`, string(projectID)).Scan(&activeAuthorityCandidates); err != nil {
		t.Fatalf("count active authority candidates: %v", err)
	}
	if activeAuthorityCandidates != 0 {
		t.Fatalf("active authority candidates after rollback = %d, want 0", activeAuthorityCandidates)
	}
	if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_authority_recovery_transitions
WHERE project_id = ?`, string(projectID)).Scan(&recoveryTransitions); err != nil {
		t.Fatalf("count recovery transitions: %v", err)
	}
	if recoveryTransitions != 0 {
		t.Fatalf("recovery transitions after rollback = %d, want 0", recoveryTransitions)
	}
}

func syncProgressForStageAuthorityFenceTest(t *testing.T, store *Store, projectID continuity.ProjectID) SyncProgress {
	t.Helper()
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	return progress
}
