package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncAuthorityRecoverySuccessorBeginsResumesAndAppendsAcrossStoreHandles(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-recovery-successor-resume")
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, stores[0], projectID, environments)

	watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 11)
	if _, err := stores[0].AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 11, 5)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	secondPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[4:], false)
	if firstPage.Environments[len(firstPage.Environments)-1].EnvironmentID >= string(start.WriterEnvironmentID) ||
		secondPage.Environments[0].EnvironmentID != string(start.WriterEnvironmentID) {
		t.Fatalf("writer is not isolated to the later final page: first=%#v final=%#v", firstPage, secondPage)
	}

	first, err := stores[0].BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if first.Successor.Ready || first.Successor.PageCount != 1 || first.Successor.EnvironmentCount != 4 {
		t.Fatalf("first recovery state = %#v", first)
	}
	if first.Transition.PredecessorCandidateID != predecessor.CandidateID ||
		first.Transition.SuccessorCandidateID != first.Successor.CandidateID ||
		first.Transition.WriterEnvironmentID != continuity.EnvironmentID(environments[len(environments)-1].EnvironmentID) ||
		first.Transition.WriterCertificateID != environments[len(environments)-1].CertificateID ||
		first.Transition.TargetMembershipGeneration != 2 {
		t.Fatalf("first transition = %#v", first.Transition)
	}

	replayed, err := stores[1].BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil || replayed != first {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition(exact replay) = (%#v, %v), want (%#v, nil)", replayed, err, first)
	}
	ready, err := stores[1].AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), projectID, first.Transition, first.Successor.Checkpoint(), start.SuccessorSnapshot, secondPage,
	)
	if err != nil {
		t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage() error = %v", err)
	}
	if !ready.Successor.Ready || ready.Successor.PageCount != 2 || ready.Successor.EnvironmentCount != 5 {
		t.Fatalf("ready recovery state = %#v", ready)
	}

	replayedReady, err := stores[0].AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), projectID, first.Transition, first.Successor.Checkpoint(), start.SuccessorSnapshot, secondPage,
	)
	if err != nil || replayedReady != ready {
		t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage(exact replay) = (%#v, %v), want (%#v, nil)", replayedReady, err, ready)
	}
	current, found, err := stores[0].CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != ready {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, ready)
	}
	assertRecoveryCandidateRoleV1(t, stores[0], projectID, predecessor.CandidateID, syncAuthorityCandidateRoleRecoveryPredecessorV1)
	assertRecoveryCandidateRoleV1(t, stores[0], projectID, ready.Successor.CandidateID, syncAuthorityCandidateRoleRecoverySuccessorV1)
}

func TestSyncAuthorityRecoverySuccessorRequiresDirectPredecessorBaseAndRegisteredWriter(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SyncAuthorityRecoveryTransitionStart, []SyncEnvironmentCertificate)
		field  string
	}{
		{
			name: "predecessor base version",
			mutate: func(start *SyncAuthorityRecoveryTransitionStart, _ []SyncEnvironmentCertificate) {
				start.SuccessorSnapshot.BaseAuthorityDigestVersion = 1
			},
			field: "base_authority_digest",
		},
		{
			name: "predecessor base digest",
			mutate: func(start *SyncAuthorityRecoveryTransitionStart, _ []SyncEnvironmentCertificate) {
				start.SuccessorSnapshot.BaseAuthorityDigest[0] ^= 0xff
			},
			field: "base_authority_digest",
		},
		{
			name: "writer certificate",
			mutate: func(start *SyncAuthorityRecoveryTransitionStart, _ []SyncEnvironmentCertificate) {
				start.WriterCertificateID[0] ^= 0xff
			},
			field: "writer_certificate_id",
		},
		{
			name: "writer membership",
			mutate: func(_ *SyncAuthorityRecoveryTransitionStart, environments []SyncEnvironmentCertificate) {
				environments[0].JoinMembershipGeneration = 2
				environments[1].JoinMembershipGeneration = 1
			},
			field: "target_membership_generation",
		},
		{
			name: "writer mode",
			mutate: func(_ *SyncAuthorityRecoveryTransitionStart, environments []SyncEnvironmentCertificate) {
				writer := &environments[len(environments)-1]
				writer.Mode = SyncEnvironmentEphemeral
				writer.ExpiresAtMillis = 1
			},
			field: "writer_mode",
		},
		{
			name: "writer retirement",
			mutate: func(start *SyncAuthorityRecoveryTransitionStart, environments []SyncEnvironmentCertificate) {
				start.SuccessorSnapshot.MembershipGeneration = 3
				writer := &environments[len(environments)-1]
				writer.Retirement = &SyncEnvironmentRetirement{
					RelayGeneration:      start.SuccessorSnapshot.RelayGeneration,
					MembershipGeneration: 3,
					RetirementID:         sha256.Sum256([]byte("recovery-writer-retirement")),
					RetirementBytes:      []byte("recovery writer retirement bytes"),
				}
			},
			field: "writer_retirement",
		},
		{
			name: "local writer mismatch",
			mutate: func(start *SyncAuthorityRecoveryTransitionStart, environments []SyncEnvironmentCertificate) {
				start.WriterEnvironmentID = continuity.EnvironmentID(environments[0].EnvironmentID)
				start.WriterCertificateID = environments[0].CertificateID
			},
			field: "writer_environment_id",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store := openSyncStore(t, "recovery-successor-binding-"+test.name)
			projectID := continuity.ProjectID("project-recovery-successor-binding")
			predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
			watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7)
			if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
				t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
			}
			environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
			start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 7, 2)
			test.mutate(&start, environments)
			page := syncAuthorityCandidatePageV2("", environments, false)
			if _, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, page); err == nil {
				t.Fatal("BeginSyncAuthorityRecoveryTransition() error = nil")
			} else {
				assertSyncAuthorityRecoveryProblemFieldV1(t, err, test.field)
			}
			if _, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID); err != nil || found {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after refusal) = (_, %v, %v)", found, err)
			}
			current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
			if err != nil || !found || current != predecessor {
				t.Fatalf("CurrentSyncAuthorityCandidate(predecessor) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, predecessor)
			}
		})
	}
}

func TestSyncAuthorityRecoverySuccessorWriterExpiryHasExactField(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-writer-expiry")
	projectID := continuity.ProjectID("project-recovery-successor-writer-expiry")
	environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
	watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 6)
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 6, 2)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	conn, err := store.db.Conn(context.Background())
	if err != nil {
		t.Fatalf("open corruption connection: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA ignore_check_constraints = ON`); err != nil {
		conn.Close()
		t.Fatalf("disable check constraints: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), `
UPDATE continuity_sync_authority_candidate_environments
SET expires_at_millis = 1
WHERE project_id = ? AND candidate_id = ? AND environment_id = ?`,
		string(projectID), state.Successor.CandidateID[:], string(state.Transition.WriterEnvironmentID),
	); err != nil {
		conn.Close()
		t.Fatalf("seed writer expiry: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA ignore_check_constraints = OFF`); err != nil {
		conn.Close()
		t.Fatalf("restore check constraints: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close corruption connection: %v", err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin writer validation transaction: %v", err)
	}
	defer tx.Rollback()
	err = validateReadySyncAuthorityRecoveryWriterV1(
		context.Background(), tx, persistedSyncAuthorityRecoveryStateV1{value: state}, false,
	)
	assertSyncAuthorityRecoveryProblemFieldV1(t, err, "writer_expiry")
}

func TestSyncAuthorityRecoverySuccessorReentersWatermarkFloorAndReplacesOnlyStaleState(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-watermark")
	projectID := continuity.ProjectID("project-recovery-successor-watermark")
	predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
	watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 10)
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(10) error = %v", err)
	}
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	staleStart := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 9, 5)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	if _, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, staleStart, firstPage); err == nil {
		t.Fatal("BeginSyncAuthorityRecoveryTransition(below floor) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorCursor)
	}

	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 10, 5)
	state, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if _, err := store.ReplaceSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(), start.SuccessorSnapshot, firstPage,
	); err == nil {
		t.Fatal("ReplaceSyncAuthorityRecoverySuccessor(non-stale) error = nil")
	}

	watermark.RelayHead = 12
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(12) error = %v", err)
	}
	secondPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[4:], false)
	if _, err := store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(), start.SuccessorSnapshot, secondPage,
	); err == nil {
		t.Fatal("AppendVerifiedSyncAuthorityRecoverySuccessorPage(stale) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorCursor)
	}

	freshSnapshot := start.SuccessorSnapshot
	freshSnapshot.InventoryArrivalHead = 12
	replaced, err := store.ReplaceSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(), freshSnapshot, firstPage,
	)
	if err != nil {
		t.Fatalf("ReplaceSyncAuthorityRecoverySuccessor() error = %v", err)
	}
	if replaced.Successor.CandidateID == state.Successor.CandidateID ||
		replaced.Transition.SuccessorCandidateID != replaced.Successor.CandidateID ||
		replaced.Transition.PredecessorCandidateID != predecessor.CandidateID {
		t.Fatalf("replaced recovery state = %#v", replaced)
	}
	if recoveryCandidateExistsV1(t, store, projectID, state.Successor.CandidateID) {
		t.Fatal("stale successor survived replacement")
	}
	replayed, err := store.ReplaceSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(), freshSnapshot, firstPage,
	)
	if err != nil || replayed != replaced {
		t.Fatalf("ReplaceSyncAuthorityRecoverySuccessor(exact replay) = (%#v, %v), want (%#v, nil)", replayed, err, replaced)
	}
	assertRecoveryCandidateRoleV1(t, store, projectID, predecessor.CandidateID, syncAuthorityCandidateRoleRecoveryPredecessorV1)
}

func TestSyncAuthorityRecoverySuccessorAbortRestoresPredecessorAndRefusesReadyEvidence(t *testing.T) {
	t.Run("staging", func(t *testing.T) {
		store := openSyncStore(t, "recovery-successor-abort-staging")
		projectID := continuity.ProjectID("project-recovery-successor-abort-staging")
		predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
		watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 3)
		if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
			t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
		}
		environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
		start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 3, 5)
		page := syncAuthorityCandidatePageV2("", environments[:4], true)
		state, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, page)
		if err != nil {
			t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
		}
		if err := store.AbortSyncAuthorityRecoveryTransition(
			context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
		); err != nil {
			t.Fatalf("AbortSyncAuthorityRecoveryTransition() error = %v", err)
		}
		if err := store.AbortSyncAuthorityRecoveryTransition(
			context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
		); err != nil {
			t.Fatalf("AbortSyncAuthorityRecoveryTransition(exact replay) error = %v", err)
		}
		if recoveryCandidateExistsV1(t, store, projectID, state.Successor.CandidateID) {
			t.Fatal("aborted successor survived")
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != predecessor {
			t.Fatalf("CurrentSyncAuthorityCandidate(after abort) = (%#v, %v, %v), want predecessor", current, found, err)
		}
		assertRecoveryCandidateRoleV1(t, store, projectID, predecessor.CandidateID, syncAuthorityCandidateRoleOrdinaryV1)
		assertRecoveryWatermarkHeadV1(t, store, projectID, start.SuccessorSnapshot, 3)
	})

	t.Run("ready", func(t *testing.T) {
		store := openSyncStore(t, "recovery-successor-abort-ready")
		projectID := continuity.ProjectID("project-recovery-successor-abort-ready")
		environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
		predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
		watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 4)
		if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
			t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
		}
		start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 4, 2)
		page := syncAuthorityCandidatePageV2("", environments, false)
		state, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, page)
		if err != nil {
			t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
		}
		if err := store.AbortSyncAuthorityRecoveryTransition(
			context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
		); err == nil {
			t.Fatal("AbortSyncAuthorityRecoveryTransition(ready) error = nil")
		}
		current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
		if err != nil || !found || current != state {
			t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after refused abort) = (%#v, %v, %v)", current, found, err)
		}
	})
}

func TestSyncAuthorityRecoverySuccessorAbortPreservesAdvancedWatermark(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-abort-watermark")
	projectID := continuity.ProjectID("project-recovery-successor-abort-watermark")
	predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 1),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 9, 5)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments[:4], true),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if err := store.AbortSyncAuthorityRecoveryTransition(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("AbortSyncAuthorityRecoveryTransition() error = %v", err)
	}
	assertRecoveryWatermarkHeadV1(t, store, projectID, start.SuccessorSnapshot, 9)
}

func TestSyncAuthorityRecoverySuccessorAbortReceiptPreventsAttemptABA(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-abort-aba")
	projectID := continuity.ProjectID("project-recovery-successor-abort-aba")
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 7, 5)
	page := syncAuthorityCandidatePageV2("", environments[:4], true)

	first, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, page)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition(first) error = %v", err)
	}
	if first.Transition.AttemptID == ([32]byte{}) {
		t.Fatal("first recovery attempt ID is zero")
	}
	if err := store.AbortSyncAuthorityRecoveryTransition(
		context.Background(), projectID, first.Transition, first.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("AbortSyncAuthorityRecoveryTransition(first) error = %v", err)
	}

	second, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, page)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition(second) error = %v", err)
	}
	if second.Transition.AttemptID == ([32]byte{}) || second.Transition.AttemptID == first.Transition.AttemptID {
		t.Fatalf("second recovery attempt ID = %x, first = %x", second.Transition.AttemptID, first.Transition.AttemptID)
	}
	if err := store.AbortSyncAuthorityRecoveryTransition(
		context.Background(), projectID, first.Transition, first.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("AbortSyncAuthorityRecoveryTransition(stale exact replay) error = %v", err)
	}
	current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != second {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after stale abort) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, second)
	}
}

func TestSyncAuthorityRecoverySuccessorAbortRejectsNeverStartedAttempt(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-abort-never-started")
	receipt := testSyncAuthorityRecoveryTerminalReceiptV1(0x91, SyncAuthorityRecoveryAborted)
	err := store.AbortSyncAuthorityRecoveryTransition(
		context.Background(), receipt.Transition.ProjectID, receipt.Transition, receipt.SuccessorCheckpoint,
	)
	if err == nil {
		t.Fatal("AbortSyncAuthorityRecoveryTransition(never started) error = nil")
	}
	assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorConflict)
}

func TestSyncAuthorityRecoverySuccessorAbortReceiptRequiresExactRetry(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-abort-exact-receipt")
	projectID := continuity.ProjectID("project-recovery-successor-abort-exact-receipt")
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 7, 5)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments[:4], true),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	checkpoint := state.Successor.Checkpoint()
	if err := store.AbortSyncAuthorityRecoveryTransition(context.Background(), projectID, state.Transition, checkpoint); err != nil {
		t.Fatalf("AbortSyncAuthorityRecoveryTransition() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*SyncAuthorityRecoveryTransition, *SyncAuthorityCandidateCheckpoint)
	}{
		{name: "attempt-id", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.AttemptID[0] ^= 0xff
		}},
		{name: "predecessor-candidate-id", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.PredecessorCandidateID[0] ^= 0xff
		}},
		{name: "successor-candidate-id", mutate: func(transition *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			transition.SuccessorCandidateID[0] ^= 0xff
			checkpoint.CandidateID = transition.SuccessorCandidateID
		}},
		{name: "writer-environment-id", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.WriterEnvironmentID = "environment-other"
		}},
		{name: "writer-certificate-id", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.WriterCertificateID[0] ^= 0xff
		}},
		{name: "target-membership-generation", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.TargetMembershipGeneration++
		}},
		{name: "page-count", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.PageCount++
		}},
		{name: "environment-count", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.EnvironmentCount--
		}},
		{name: "through-environment-id", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.ThroughEnvironmentID = "environment-other"
		}},
		{name: "rolling-environment-digest", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.RollingEnvironmentDigest[0] ^= 0xff
		}},
		{name: "ready-and-authority-digest", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.Ready = true
			checkpoint.AuthorityDigest = sha256.Sum256([]byte("different terminal authority"))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedTransition := state.Transition
			changedCheckpoint := checkpoint
			test.mutate(&changedTransition, &changedCheckpoint)
			err := store.AbortSyncAuthorityRecoveryTransition(
				context.Background(), projectID, changedTransition, changedCheckpoint,
			)
			if err == nil {
				t.Fatal("AbortSyncAuthorityRecoveryTransition(changed retry) error = nil")
			}
		})
	}
}

func TestSyncAuthorityRecoverySuccessorRejectsActiveAndTerminalAttempt(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-active-terminal-corruption")
	projectID := continuity.ProjectID("project-recovery-successor-active-terminal-corruption")
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 7, 5)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments[:4], true),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	receipt, err := newSyncAuthorityRecoveryTerminalReceiptV1(
		SyncAuthorityRecoveryAborted, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil {
		t.Fatalf("newSyncAuthorityRecoveryTerminalReceiptV1() error = %v", err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin corrupt receipt transaction: %v", err)
	}
	if err := insertSyncAuthorityRecoveryTerminalReceiptV1(context.Background(), tx, receipt); err != nil {
		tx.Rollback()
		t.Fatalf("insert active-attempt receipt: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit active-attempt receipt: %v", err)
	}

	if _, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID); err == nil || found {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(active and terminal) = (_, %v, %v), want store error", found, err)
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorStore)
		assertSyncAuthorityRecoveryProblemFieldV1(t, err, "sync_authority_recovery_transition")
	}
	if err := store.AbortSyncAuthorityRecoveryTransition(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("AbortSyncAuthorityRecoveryTransition(active and terminal) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorStore)
		assertSyncAuthorityRecoveryProblemFieldV1(t, err, "sync_authority_recovery_transition")
	}
	if !recoveryCandidateExistsV1(t, store, projectID, state.Successor.CandidateID) {
		t.Fatal("active successor was mutated after corruption refusal")
	}
}

func TestSyncAuthorityRecoverySuccessorAppendReplayRequiresExactPriorCheckpoint(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-append-replay-checkpoint")
	projectID := continuity.ProjectID("project-recovery-successor-append-replay-checkpoint")
	environments := syncAuthorityRecoveryEnvironmentsV1(6, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 7, 6)
	firstPage := syncAuthorityCandidatePageV2("", environments[:2], true)
	first, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	secondPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[2:4], true)
	second, err := store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), projectID, first.Transition, first.Successor.Checkpoint(), start.SuccessorSnapshot, secondPage,
	)
	if err != nil {
		t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage() error = %v", err)
	}

	for _, mutation := range syncAuthorityRecoveryReplayCheckpointMutationsV1() {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			changed := first.Successor.Checkpoint()
			mutation.mutate(&changed)
			if _, err := store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
				context.Background(), projectID, first.Transition, changed, start.SuccessorSnapshot, secondPage,
			); err == nil {
				t.Fatal("AppendVerifiedSyncAuthorityRecoverySuccessorPage(changed prior checkpoint) error = nil")
			}
			current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
			if err != nil || !found || current != second {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, second)
			}
		})
	}
}

func TestSyncAuthorityRecoverySuccessorSupportsGenerationOneWithoutPredecessor(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-generation-one")
	projectID := continuity.ProjectID("project-recovery-successor-generation-one")
	environments := syncAuthorityRecoveryEnvironmentsV1(1, 1)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	snapshot.InventoryArrivalHead = 0
	watermark := syncRelayWatermarkFromSnapshot(projectID, snapshot, 0)
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := SyncAuthorityRecoveryTransitionStart{
		WriterEnvironmentID:        continuity.EnvironmentID(environments[0].EnvironmentID),
		WriterCertificateID:        environments[0].CertificateID,
		TargetMembershipGeneration: 1,
		SuccessorSnapshot:          snapshot,
	}
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if state.Transition.PredecessorCandidateID != ([32]byte{}) || !state.Successor.Ready {
		t.Fatalf("generation-one state = %#v", state)
	}
}

func TestSyncAuthorityRecoverySuccessorGenerationOneRefusesExistingAuthorityState(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *Store, continuity.ProjectID)
	}{
		{
			name: "canonical authority",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority()); err != nil {
					t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
				}
			},
		},
		{
			name: "ordinary candidate",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "recovery-successor-generation-one-refusal-"+test.name)
			projectID := continuity.ProjectID("project-recovery-successor-generation-one-refusal")
			test.seed(t, store, projectID)
			environments := syncAuthorityRecoveryEnvironmentsV1(1, 1)
			snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
			start := SyncAuthorityRecoveryTransitionStart{
				WriterEnvironmentID:        continuity.EnvironmentID(environments[0].EnvironmentID),
				WriterCertificateID:        environments[0].CertificateID,
				TargetMembershipGeneration: 1,
				SuccessorSnapshot:          snapshot,
			}
			if _, err := store.BeginSyncAuthorityRecoveryTransition(
				context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
			); err == nil {
				t.Fatal("BeginSyncAuthorityRecoveryTransition() error = nil")
			} else {
				assertSyncAuthorityRecoveryProblemFieldV1(t, err, "predecessor_checkpoint")
			}
			if _, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID); err != nil || found {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (_, %v, %v)", found, err)
			}
		})
	}
}

func TestSyncAuthorityRecoverySuccessorConcurrentExactBeginConverges(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-recovery-successor-concurrent-begin")
	environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, stores[0], projectID, environments)
	watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8)
	if _, err := stores[0].AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 8, 2)
	page := syncAuthorityCandidatePageV2("", environments, false)

	type result struct {
		state SyncAuthorityRecoveryState
		err   error
	}
	results := make([]result, len(stores))
	begin := make(chan struct{})
	var group sync.WaitGroup
	for index := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-begin
			results[index].state, results[index].err = stores[index].BeginSyncAuthorityRecoveryTransition(
				context.Background(), projectID, start, page,
			)
		}(index)
	}
	close(begin)
	group.Wait()
	if results[0].err != nil || results[1].err != nil || results[0].state != results[1].state ||
		results[0].state.Transition.AttemptID == ([32]byte{}) || !results[0].state.Successor.Ready {
		t.Fatalf("concurrent exact begin results = %#v", results)
	}
}

func TestSyncAuthorityRecoverySuccessorConcurrentDifferentBeginsLeaveOneWinner(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-recovery-successor-concurrent-different-begin")
	environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, stores[0], projectID, environments)
	if _, err := stores[0].AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	starts := [2]SyncAuthorityRecoveryTransitionStart{
		syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 8, 2),
		syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 9, 2),
	}
	page := syncAuthorityCandidatePageV2("", environments, false)
	results := runConcurrentRecoveryMutationsV1(stores, func(index int) (SyncAuthorityRecoveryState, error) {
		return stores[index].BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, starts[index], page)
	})
	winner := requireOneRecoveryMutationWinnerV1(t, results)
	current, found, err := stores[0].CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != results[winner].state {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (%#v, %v, %v), winner = %#v", current, found, err, results[winner])
	}
	assertRecoveryCandidateCountsV1(t, stores[0], projectID, 2, 1)
}

func TestSyncAuthorityRecoverySuccessorConcurrentDifferentAppendsLeaveOneWinner(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-recovery-successor-concurrent-different-append")
	predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, stores[0], projectID, 1)
	if _, err := stores[0].AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	environments := syncAuthorityRecoveryEnvironmentsV1(6, 2)
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 8, 6)
	firstPage := syncAuthorityCandidatePageV2("", environments[:2], true)
	initial, err := stores[0].BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	pages := [2]SyncAuthorityPage{
		syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[2:4], true),
		syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[2:4], true),
	}
	pages[1].Environments[0].CertificateID = sha256.Sum256([]byte("competing-append-certificate"))
	pages[1].Environments[0].CertificateBytes = []byte("competing append certificate bytes")
	results := runConcurrentRecoveryMutationsV1(stores, func(index int) (SyncAuthorityRecoveryState, error) {
		return stores[index].AppendVerifiedSyncAuthorityRecoverySuccessorPage(
			context.Background(), projectID, initial.Transition, initial.Successor.Checkpoint(), start.SuccessorSnapshot, pages[index],
		)
	})
	winner := requireOneRecoveryMutationWinnerV1(t, results)
	current, found, err := stores[0].CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != results[winner].state || current.Successor.PageCount != 2 {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (%#v, %v, %v), winner = %#v", current, found, err, results[winner])
	}
}

func TestSyncAuthorityRecoverySuccessorConcurrentDifferentReplacementsNeverDeleteWinner(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-recovery-successor-concurrent-different-replace")
	predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, stores[0], projectID, 1)
	watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 10)
	if _, err := stores[0].AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(10) error = %v", err)
	}
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 10, 5)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	stale, err := stores[0].BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	watermark.RelayHead = 12
	if _, err := stores[0].AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(12) error = %v", err)
	}
	snapshots := [2]SyncAuthoritySnapshot{start.SuccessorSnapshot, start.SuccessorSnapshot}
	snapshots[0].InventoryArrivalHead = 12
	snapshots[1].InventoryArrivalHead = 13
	results := runConcurrentRecoveryMutationsV1(stores, func(index int) (SyncAuthorityRecoveryState, error) {
		return stores[index].ReplaceSyncAuthorityRecoverySuccessor(
			context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(), snapshots[index], firstPage,
		)
	})
	winner := requireOneRecoveryMutationWinnerV1(t, results)
	loser := 1 - winner
	current, found, err := stores[0].CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != results[winner].state {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (%#v, %v, %v), winner = %#v", current, found, err, results[winner])
	}
	if _, err := stores[loser].ReplaceSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(), snapshots[loser], firstPage,
	); err == nil {
		t.Fatal("ReplaceSyncAuthorityRecoverySuccessor(stale losing checkpoint) error = nil")
	}
	after, found, err := stores[0].CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || after != current || !recoveryCandidateExistsV1(t, stores[0], projectID, current.Successor.CandidateID) {
		t.Fatalf("winner after stale replacement = (%#v, %v, %v), want %#v", after, found, err, current)
	}
	assertRecoveryCandidateCountsV1(t, stores[0], projectID, 2, 1)
}

func TestCurrentSyncAuthorityRecoverySuccessorReturnsFieldedRawCorruption(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		corrupt func(*testing.T, *Store, continuity.ProjectID, SyncAuthorityRecoveryState)
	}{
		{
			name:  "role",
			field: "sync_authority_recovery_transition",
			corrupt: func(t *testing.T, store *Store, projectID continuity.ProjectID, state SyncAuthorityRecoveryState) {
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidates SET role = 'ordinary'
WHERE project_id = ? AND candidate_id = ?`, string(projectID), state.Successor.CandidateID[:]); err != nil {
					t.Fatalf("corrupt successor role: %v", err)
				}
			},
		},
		{
			name:  "link",
			field: "sync_authority_recovery_transition",
			corrupt: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ SyncAuthorityRecoveryState) {
				if _, err := store.db.Exec(`DELETE FROM continuity_sync_authority_recovery_transitions WHERE project_id = ?`, string(projectID)); err != nil {
					t.Fatalf("delete transition link: %v", err)
				}
			},
		},
		{
			name:  "child stream",
			field: "sync_authority_candidate",
			corrupt: func(t *testing.T, store *Store, projectID continuity.ProjectID, state SyncAuthorityRecoveryState) {
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_environments SET certificate_bytes = ?
WHERE project_id = ? AND candidate_id = ? AND environment_id = ?`,
					[]byte("tampered public certificate bytes"), string(projectID), state.Successor.CandidateID[:], string(state.Transition.WriterEnvironmentID),
				); err != nil {
					t.Fatalf("corrupt successor child: %v", err)
				}
			},
		},
		{
			name:  "predecessor child stream",
			field: "sync_authority_candidate",
			corrupt: func(t *testing.T, store *Store, projectID continuity.ProjectID, state SyncAuthorityRecoveryState) {
				if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_environments SET certificate_bytes = ?
WHERE project_id = ? AND candidate_id = ? AND environment_ordinal = 1`,
					[]byte("tampered predecessor certificate bytes"), string(projectID), state.Transition.PredecessorCandidateID[:],
				); err != nil {
					t.Fatalf("corrupt predecessor child: %v", err)
				}
			},
		},
		{
			name:  "durable watermark",
			field: "relay_watermark",
			corrupt: func(t *testing.T, store *Store, projectID continuity.ProjectID, state SyncAuthorityRecoveryState) {
				if _, err := store.db.Exec(`
DELETE FROM continuity_sync_relay_watermarks
WHERE project_id = ? AND channel_id = ? AND relay_generation = ?`,
					string(projectID), state.Successor.Snapshot.ChannelID[:], state.Successor.Snapshot.RelayGeneration[:],
				); err != nil {
					t.Fatalf("delete recovery watermark: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "recovery-successor-current-corruption-"+test.name)
			projectID := continuity.ProjectID("project-recovery-successor-current-corruption")
			environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
			predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
			if _, err := store.AdvanceSyncRelayWatermark(
				context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 5),
			); err != nil {
				t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
			}
			start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 5, 2)
			state, err := store.BeginSyncAuthorityRecoveryTransition(
				context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
			)
			if err != nil {
				t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
			}
			test.corrupt(t, store, projectID, state)
			_, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
			if err == nil || found {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (_, %v, %v), want fielded store error", found, err)
			}
			var problem *SyncError
			if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != test.field {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() error = %#v, want field %q", err, test.field)
			}
		})
	}
}

type recoveryMutationResultV1 struct {
	state SyncAuthorityRecoveryState
	err   error
}

func runConcurrentRecoveryMutationsV1(
	stores [2]*Store,
	mutate func(int) (SyncAuthorityRecoveryState, error),
) [2]recoveryMutationResultV1 {
	var results [2]recoveryMutationResultV1
	begin := make(chan struct{})
	var group sync.WaitGroup
	for index := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-begin
			results[index].state, results[index].err = mutate(index)
		}(index)
	}
	close(begin)
	group.Wait()
	return results
}

func requireOneRecoveryMutationWinnerV1(t *testing.T, results [2]recoveryMutationResultV1) int {
	t.Helper()
	winner := -1
	for index, result := range results {
		if result.err == nil {
			if winner != -1 {
				t.Fatalf("concurrent mutations both succeeded: %#v", results)
			}
			winner = index
		}
	}
	if winner == -1 {
		t.Fatalf("concurrent mutations both failed: %#v", results)
	}
	return winner
}

func syncAuthorityRecoveryStartV1(
	predecessor SyncAuthorityCandidate,
	writer SyncEnvironmentCertificate,
	arrivalHead int64,
	successorMembership uint32,
) SyncAuthorityRecoveryTransitionStart {
	snapshot := predecessor.Snapshot
	snapshot.MembershipGeneration = successorMembership
	snapshot.InventoryArrivalHead = arrivalHead
	snapshot.BaseAuthorityDigestVersion = 2
	snapshot.BaseAuthorityDigest = predecessor.AuthorityDigest
	return SyncAuthorityRecoveryTransitionStart{
		WriterEnvironmentID:        continuity.EnvironmentID(writer.EnvironmentID),
		WriterCertificateID:        writer.CertificateID,
		TargetMembershipGeneration: predecessor.Snapshot.MembershipGeneration + 1,
		PredecessorCheckpoint:      predecessor.Checkpoint(),
		SuccessorSnapshot:          snapshot,
	}
}

func stageReadySyncAuthorityRecoveryPredecessorV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	successorEnvironments []SyncEnvironmentCertificate,
) (SyncAuthoritySnapshot, SyncAuthorityCandidate) {
	t.Helper()
	predecessorEnvironments := make([]SyncEnvironmentCertificate, 0, 1)
	for _, environment := range successorEnvironments {
		if environment.JoinMembershipGeneration != 1 {
			continue
		}
		predecessor := environment
		predecessor.CertificateBytes = append([]byte(nil), environment.CertificateBytes...)
		if predecessor.Retirement != nil && predecessor.Retirement.MembershipGeneration > 1 {
			predecessor.Retirement = nil
		}
		predecessorEnvironments = append(predecessorEnvironments, predecessor)
	}
	if len(predecessorEnvironments) != 1 {
		t.Fatalf("generation-one recovery predecessor environments = %d, want 1", len(predecessorEnvironments))
	}
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	predecessor, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot,
		syncAuthorityCandidatePageV2("", predecessorEnvironments, false),
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(recovery predecessor) error = %v", err)
	}
	if !predecessor.Ready {
		t.Fatalf("recovery predecessor = %#v, want READY", predecessor)
	}
	return snapshot, predecessor
}

func syncAuthorityRecoveryEnvironmentsV1(count int, writerMembership uint32) []SyncEnvironmentCertificate {
	environments := make([]SyncEnvironmentCertificate, count)
	nextMembership := uint32(1)
	for index := 0; index < count; index++ {
		environmentID := "environment-local"
		membership := writerMembership
		if index < count-1 {
			environmentID = fmt.Sprintf("environment-a%04d", index+1)
			for nextMembership == writerMembership {
				nextMembership++
			}
			membership = nextMembership
			nextMembership++
		}
		environments[index] = SyncEnvironmentCertificate{
			EnvironmentID:            environmentID,
			CertificateID:            sha256.Sum256([]byte("recovery-certificate:" + environmentID)),
			CertificateBytes:         []byte("recovery certificate bytes for " + environmentID),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: membership,
		}
	}
	return environments
}

func assertSyncAuthorityRecoveryProblemFieldV1(t *testing.T, err error, field string) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Field != field {
		t.Fatalf("error = %#v, want field %q", err, field)
	}
}

func assertSyncAuthorityRecoveryProblemCodeV1(t *testing.T, err error, code SyncErrorCode) {
	t.Helper()
	var problem *SyncError
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("error = %#v, want code %q", err, code)
	}
}

func assertRecoveryCandidateRoleV1(t *testing.T, store *Store, projectID continuity.ProjectID, candidateID [32]byte, want string) {
	t.Helper()
	var role string
	if err := store.db.QueryRow(`
SELECT role FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ?`, string(projectID), candidateID[:]).Scan(&role); err != nil {
		t.Fatalf("read candidate role: %v", err)
	}
	if role != want {
		t.Fatalf("candidate role = %q, want %q", role, want)
	}
}

func recoveryCandidateExistsV1(t *testing.T, store *Store, projectID continuity.ProjectID, candidateID [32]byte) bool {
	t.Helper()
	var exists int
	if err := store.db.QueryRow(`
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_authority_candidates
  WHERE project_id = ? AND candidate_id = ?
)`, string(projectID), candidateID[:]).Scan(&exists); err != nil {
		t.Fatalf("inspect candidate existence: %v", err)
	}
	return exists != 0
}

func assertRecoveryWatermarkHeadV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
	want int64,
) {
	t.Helper()
	var head int64
	if err := store.db.QueryRow(`
SELECT relay_head FROM continuity_sync_relay_watermarks
WHERE project_id = ? AND channel_id = ? AND relay_generation = ? AND admin_public_key = ?`,
		string(projectID), snapshot.ChannelID[:], snapshot.RelayGeneration[:], snapshot.AdminPublicKey[:],
	).Scan(&head); err != nil {
		t.Fatalf("read recovery watermark: %v", err)
	}
	if head != want {
		t.Fatalf("recovery watermark head = %d, want %d", head, want)
	}
}

func assertRecoveryCandidateCountsV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	wantCandidates int,
	wantTransitions int,
) {
	t.Helper()
	var candidates, transitions int
	if err := store.db.QueryRow(`
SELECT COUNT(*) FROM continuity_sync_authority_candidates WHERE project_id = ?`, string(projectID)).Scan(&candidates); err != nil {
		t.Fatalf("count recovery candidates: %v", err)
	}
	if err := store.db.QueryRow(`
SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions WHERE project_id = ?`, string(projectID)).Scan(&transitions); err != nil {
		t.Fatalf("count recovery transitions: %v", err)
	}
	if candidates != wantCandidates || transitions != wantTransitions {
		t.Fatalf("recovery row counts = candidates:%d transitions:%d, want candidates:%d transitions:%d",
			candidates, transitions, wantCandidates, wantTransitions)
	}
}
