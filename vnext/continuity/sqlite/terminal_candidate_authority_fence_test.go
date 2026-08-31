package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"sort"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestStageVerifiedTerminalCandidateChunkV2RequiresExactRelayFrontierWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		wantField string
		advance   func(*SyncRelayWatermark)
	}{
		{
			name:      "membership",
			wantField: "membership_generation",
			advance: func(watermark *SyncRelayWatermark) {
				watermark.MembershipGeneration++
			},
		},
		{
			name:      "arrival head",
			wantField: "inventory_arrival_head",
			advance: func(watermark *SyncRelayWatermark) {
				watermark.RelayHead++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "stage-"+syncSlug(test.name), 2)
			advanced := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
			test.advance(&advanced)
			if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
				t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, advanced)
			}

			candidate, err := store.StageVerifiedTerminalCandidateChunk(
				context.Background(), projectID, binding, frames, 1_000, 100,
			)
			assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, test.wantField)
			if candidate != (TerminalCandidate{}) {
				t.Fatalf("StageVerifiedTerminalCandidateChunk() = %#v, want zero candidate", candidate)
			}
			assertNoActiveTerminalCandidateV1(t, store, projectID)
		})
	}
}

func TestStageVerifiedTerminalCandidateChunkV2AllowsBoundedProgressBeforeCutoff(t *testing.T) {
	t.Parallel()

	store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(t, "bounded-progress", 3)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque[:1],
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(first page) error = %v", err)
	}
	first, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames[:1], 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(first chunk) error = %v", err)
	}
	if first.ThroughArrivalSequence != 1 {
		t.Fatalf("first candidate through = %d, want 1", first.ThroughArrivalSequence)
	}
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 1, binding.InventoryArrivalHead, opaque[1:],
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority(final page) error = %v", err)
	}
	complete, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames[1:], 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(final chunk) error = %v", err)
	}
	if complete.ThroughArrivalSequence != binding.InventoryArrivalHead {
		t.Fatalf("complete candidate through = %d, want %d", complete.ThroughArrivalSequence, binding.InventoryArrivalHead)
	}
	if _, err := store.PromoteTerminalCandidate(
		context.Background(), projectID, terminalCandidateCheckpointV1(complete),
	); err != nil {
		t.Fatalf("PromoteTerminalCandidate() error = %v", err)
	}
}

func TestStageVerifiedTerminalCandidateChunkV2ReplayDoesNotBypassFrontierFence(t *testing.T) {
	t.Parallel()

	store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "stale-replay", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	advanced := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	advanced.MembershipGeneration++
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
		t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, advanced)
	}

	_, err = store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames, 1_000, 100,
	)
	assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, "membership_generation")
	current, found, currentErr := store.CurrentTerminalCandidate(context.Background(), projectID)
	if currentErr != nil || !found || current != candidate {
		t.Fatalf("CurrentTerminalCandidate(after refused replay) = (%#v, %v, %v), want %#v", current, found, currentErr, candidate)
	}
}

func TestTerminalCandidateV2RejectsProgressBeyondAuthorityCutoff(t *testing.T) {
	t.Parallel()

	t.Run("stage", func(t *testing.T) {
		store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "stage-progress-beyond", 2)
		stageTerminalCandidatePostCutoffArrivalV1(t, store, projectID, binding)

		_, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames, 1_000, 100,
		)
		assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, "relay_head")
		assertNoActiveTerminalCandidateV1(t, store, projectID)
	})

	t.Run("promotion", func(t *testing.T) {
		store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "promotion-progress-beyond", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, binding, frames, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}
		stageTerminalCandidatePostCutoffArrivalV1(t, store, projectID, binding)

		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
		assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, "downloaded_cursor")
		assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	})
}

func TestPromoteTerminalCandidateV2RequiresExactRelayFrontierWithoutMutation(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		wantField string
		advance   func(*SyncRelayWatermark)
	}{
		{
			name:      "membership",
			wantField: "membership_generation",
			advance: func(watermark *SyncRelayWatermark) {
				watermark.MembershipGeneration++
			},
		},
		{
			name:      "arrival head",
			wantField: "inventory_arrival_head",
			advance: func(watermark *SyncRelayWatermark) {
				watermark.RelayHead++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "promote-"+syncSlug(test.name), 2)
			candidate, err := store.StageVerifiedTerminalCandidateChunk(
				context.Background(), projectID, binding, frames, 1_000, 100,
			)
			if err != nil {
				t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
			}
			advanced := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
			test.advance(&advanced)
			if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
				t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, advanced)
			}

			_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
			assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, test.wantField)
			assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
		})
	}
}

func TestPromoteTerminalCandidateV2RequiresCompleteAuthorityCutoff(t *testing.T) {
	t.Parallel()

	store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "partial", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames[:1], 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk(prefix) error = %v", err)
	}

	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorCursor, "inventory_arrival_head")
	assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
}

func TestPromoteTerminalCandidateV2CompleteAuthorityCutoffAndReceiptRetry(t *testing.T) {
	t.Parallel()

	store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "complete", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	checkpoint := terminalCandidateCheckpointV1(candidate)
	receipt, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
	if err != nil {
		t.Fatalf("PromoteTerminalCandidate() error = %v", err)
	}
	if receipt.ResultingAppliedCursor != binding.InventoryArrivalHead {
		t.Fatalf("ResultingAppliedCursor = %d, want %d", receipt.ResultingAppliedCursor, binding.InventoryArrivalHead)
	}

	advanced := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	advanced.MembershipGeneration++
	advanced.RelayHead++
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
		t.Fatalf("AdvanceSyncRelayWatermark(after promotion) = (%#v, %v), want (%#v, nil)", got, err, advanced)
	}
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	authority.MembershipGeneration = advanced.MembershipGeneration
	authority.InventoryArrivalHead = advanced.RelayHead
	setCanonicalSyncAuthorityMetadataV2ForBindingTest(t, store, projectID, authority)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET membership_generation = ?, downloaded_cursor = ?, relay_head = ?
WHERE project_id = ?`, advanced.MembershipGeneration, advanced.RelayHead, advanced.RelayHead, string(projectID)); err != nil {
		t.Fatalf("advance mutable sync progress: %v", err)
	}
	replayed, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
	if err != nil || replayed != receipt {
		t.Fatalf("PromoteTerminalCandidate(exact retry after frontier drift) = (%#v, %v), want (%#v, nil)", replayed, err, receipt)
	}
}

func TestPromoteTerminalCandidateExactFinalCASRollsBack(t *testing.T) {
	t.Parallel()

	store, projectID, binding, frames := terminalCandidateV2AuthorityFenceFixture(t, "final-cas", 2)
	candidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, binding, frames, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	if _, err := store.db.Exec(`
CREATE TEMP TRIGGER mutate_terminal_promotion_progress
AFTER INSERT ON continuity_sync_receipts
BEGIN
  UPDATE continuity_sync_projects
  SET relay_head = relay_head + 1
  WHERE project_id = NEW.project_id;
END`); err != nil {
		t.Fatalf("create terminal promotion progress trigger: %v", err)
	}

	_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
	assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_progress")
	assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	if progress.DownloadedCursor != binding.InventoryArrivalHead || progress.RelayHead != binding.InventoryArrivalHead {
		t.Fatalf("progress after rolled-back final CAS = %#v, want exact cutoff %d", progress, binding.InventoryArrivalHead)
	}
}

func TestTerminalCandidateMutationsRejectActiveRecoveryTransitionButReceiptRetryWins(t *testing.T) {
	t.Parallel()

	t.Run("stage", func(t *testing.T) {
		fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "terminal-stage-fence")
		_, frames, _ := terminalHotPathFramesV2(t, fixture.projectID, testSyncAuthority().Environments[0], 1)
		binding := SyncAuthorityBinding{
			ChannelID:              fixture.predecessor.Snapshot.ChannelID,
			RelayGeneration:        fixture.predecessor.Snapshot.RelayGeneration,
			AdminPublicKey:         fixture.predecessor.Snapshot.AdminPublicKey,
			MembershipGeneration:   fixture.predecessor.Snapshot.MembershipGeneration,
			InventoryArrivalHead:   fixture.predecessor.Snapshot.InventoryArrivalHead,
			AuthorityDigestVersion: fixture.predecessor.Snapshot.BaseAuthorityDigestVersion,
			AuthorityDigest:        fixture.predecessor.Snapshot.BaseAuthorityDigest,
		}
		candidate, err := fixture.store.StageVerifiedTerminalCandidateChunk(
			context.Background(), fixture.projectID, binding, frames, 1_000, 100,
		)
		assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_recovery_transition")
		if candidate != (TerminalCandidate{}) {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() = %#v, want zero candidate", candidate)
		}
		assertNoActiveTerminalCandidateV1(t, fixture.store, fixture.projectID)
	})

	t.Run("promotion", func(t *testing.T) {
		store, projectID, authority, frames := terminalCandidateV1RecoveryFixture(t, "promotion", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}
		beginTerminalCandidateRecoveryTransitionV1(t, store, projectID, authority, int64(len(frames)))

		_, err = store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(candidate))
		assertTerminalCandidateAuthorityFenceProblem(t, err, SyncErrorConflict, "sync_authority_recovery_transition")
		assertTerminalCandidatePromotionUnchangedV1(t, store, projectID, candidate)
	})

	t.Run("permanent receipt retry", func(t *testing.T) {
		store, projectID, authority, frames := terminalCandidateV1RecoveryFixture(t, "receipt", 2)
		candidate, err := store.StageVerifiedTerminalCandidateChunk(
			context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100,
		)
		if err != nil {
			t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
		}
		checkpoint := terminalCandidateCheckpointV1(candidate)
		receipt, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
		if err != nil {
			t.Fatalf("PromoteTerminalCandidate() error = %v", err)
		}
		beginTerminalCandidateRecoveryTransitionV1(t, store, projectID, authority, int64(len(frames)))

		replayed, err := store.PromoteTerminalCandidate(context.Background(), projectID, checkpoint)
		if err != nil || replayed != receipt {
			t.Fatalf("PromoteTerminalCandidate(exact retry during recovery transition) = (%#v, %v), want (%#v, nil)", replayed, err, receipt)
		}
	})
}

func terminalCandidateV1RecoveryFixture(
	t *testing.T,
	suffix string,
	count int,
) (*Store, continuity.ProjectID, SyncAuthority, []VerifiedTerminalCandidateFrame) {
	t.Helper()
	store := openSyncStore(t, "terminal-v1-recovery-fence-"+suffix)
	projectID := continuity.ProjectID("project-terminal-v1-recovery-fence-" + suffix)
	authority := cloneTerminalCandidateAuthorityV1(testSyncAuthority())
	opaque, frames, finalDigest := terminalHotPathFramesV2(t, projectID, authority.Environments[0], count)
	authority.Environments[0].Retirement.FinalEnvironmentSequence = int64(count)
	authority.Environments[0].Retirement.FinalEnvelopeDigest = finalDigest
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	if _, err := store.StageSyncPage(
		context.Background(), projectID, authority.ChannelID, 0, int64(len(opaque)), opaque,
	); err != nil {
		t.Fatalf("StageSyncPage() error = %v", err)
	}
	return store, projectID, authority, frames
}

func terminalCandidateV2AuthorityFenceFixture(
	t *testing.T,
	suffix string,
	count int,
) (*Store, continuity.ProjectID, SyncAuthorityBinding, []VerifiedTerminalCandidateFrame) {
	t.Helper()
	store, projectID, binding, opaque, frames := terminalCandidateV2AuthorityFencePrepared(t, suffix, count)
	if _, err := store.StageSyncPageUnderAuthority(
		context.Background(), projectID, binding, 0, binding.InventoryArrivalHead, opaque,
	); err != nil {
		t.Fatalf("StageSyncPageUnderAuthority() error = %v", err)
	}
	return store, projectID, binding, frames
}

func terminalCandidateV2AuthorityFencePrepared(
	t *testing.T,
	suffix string,
	count int,
) (*Store, continuity.ProjectID, SyncAuthorityBinding, []OpaqueSyncFrame, []VerifiedTerminalCandidateFrame) {
	t.Helper()
	store := openSyncStore(t, "terminal-v2-authority-fence-"+suffix)
	projectID := continuity.ProjectID("project-terminal-v2-authority-fence-" + suffix)
	environments := syncAuthorityCandidateManyEnvironmentsV2(1)
	opaque, frames, finalDigest := terminalHotPathFramesV2(t, projectID, environments[0], count)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(2)
	snapshot.InventoryArrivalHead = int64(len(opaque))
	environments[0].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          snapshot.RelayGeneration,
		MembershipGeneration:     snapshot.MembershipGeneration,
		FinalEnvironmentSequence: int64(count),
		FinalEnvelopeDigest:      finalDigest,
		RetirementID:             sha256.Sum256([]byte("terminal-v2-authority-fence-retirement:" + suffix)),
		RetirementBytes:          []byte("terminal v2 authority fence retirement " + suffix),
	}
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, environments)
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	watermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil || got != watermark {
		t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, watermark)
	}
	return store, projectID, binding, opaque, frames
}

func stageTerminalCandidatePostCutoffArrivalV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	binding SyncAuthorityBinding,
) {
	t.Helper()
	sealed := []byte("terminal candidate post-cutoff arrival")
	digest := sha256.Sum256(sealed)
	arrival := binding.InventoryArrivalHead + 1
	if _, err := store.StageSyncPage(
		context.Background(), projectID, binding.ChannelID, binding.InventoryArrivalHead, arrival,
		[]OpaqueSyncFrame{{ArrivalSequence: arrival, EnvelopeDigest: digest, SealedEnvelope: sealed}},
	); err != nil {
		t.Fatalf("StageSyncPage(post-cutoff arrival) error = %v", err)
	}
}

func beginTerminalCandidateRecoveryTransitionV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	authority SyncAuthority,
	arrivalHead int64,
) {
	t.Helper()
	predecessor := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
	writer := SyncEnvironmentCertificate{
		EnvironmentID:            string(store.WriterEnvironmentID()),
		CertificateID:            sha256.Sum256([]byte("terminal-candidate-recovery-writer:" + string(projectID))),
		CertificateBytes:         []byte("terminal candidate recovery writer " + string(projectID)),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: authority.MembershipGeneration + 1,
	}
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(authority.Environments)
	environments = append(environments, writer)
	sort.Slice(environments, func(left, right int) bool {
		return environments[left].EnvironmentID < environments[right].EnvironmentID
	})
	start := syncAuthorityRecoveryStartV1(
		predecessor, writer, arrivalHead, authority.MembershipGeneration+1,
	)
	if _, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
	); err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
}

func assertTerminalCandidateAuthorityFenceProblem(
	t *testing.T,
	err error,
	wantCode SyncErrorCode,
	wantField string,
) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) {
		t.Fatalf("error = %v, want *SyncError code %q at %q", err, wantCode, wantField)
	}
	if problem.Code != wantCode || problem.Field != wantField {
		t.Fatalf("error = %#v, want code %q at %q", problem, wantCode, wantField)
	}
}
