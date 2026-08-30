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

func TestPromoteSyncAuthorityRecoverySuccessorTerminalizesExactReadyAttempt(t *testing.T) {
	tests := []struct {
		name  string
		stage func(*testing.T, *Store, continuity.ProjectID) SyncAuthorityRecoveryState
	}{
		{name: "generation one", stage: stageReadyGenerationOneSyncAuthorityRecoveryV1},
		{name: "retained predecessor", stage: stageReadyPredecessorSyncAuthorityRecoveryV1},
		{name: "canonical predecessor", stage: stageReadyCanonicalPredecessorSyncAuthorityRecoveryV1},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "recovery-promotion-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-recovery-promotion-" + syncSlug(test.name))
			state := test.stage(t, store, projectID)

			receipt, err := store.PromoteSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
			)
			if err != nil {
				t.Fatalf("PromoteSyncAuthorityRecoverySuccessor() error = %v", err)
			}
			wantReceipt, err := newSyncAuthorityRecoveryTerminalReceiptV1(
				SyncAuthorityRecoveryPromoted, state.Transition, state.Successor.Checkpoint(),
			)
			if err != nil {
				t.Fatalf("newSyncAuthorityRecoveryTerminalReceiptV1() error = %v", err)
			}
			if receipt != wantReceipt {
				t.Fatalf("promotion receipt = %#v, want %#v", receipt, wantReceipt)
			}

			binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
			if err != nil {
				t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
			}
			wantBinding := SyncAuthorityBinding{
				ChannelID:              state.Successor.Snapshot.ChannelID,
				RelayGeneration:        state.Successor.Snapshot.RelayGeneration,
				AdminPublicKey:         state.Successor.Snapshot.AdminPublicKey,
				MembershipGeneration:   state.Successor.Snapshot.MembershipGeneration,
				InventoryArrivalHead:   state.Successor.Snapshot.InventoryArrivalHead,
				AuthorityDigestVersion: state.Successor.AuthorityDigestVersion,
				AuthorityDigest:        state.Successor.AuthorityDigest,
			}
			if binding != wantBinding {
				t.Fatalf("canonical binding = %#v, want %#v", binding, wantBinding)
			}
			if current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID); err != nil || found {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after promotion) = (%#v, %v, %v), want (_, false, nil)", current, found, err)
			}
			assertPromotedSyncAuthorityRecoveryRowsV1(t, store, state)

			retried, err := store.PromoteSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
			)
			if err != nil || retried != receipt {
				t.Fatalf("exact promotion retry = (%#v, %v), want (%#v, nil)", retried, err, receipt)
			}
			if _, err := store.PromoteSyncAuthorityCandidate(
				context.Background(), projectID, state.Successor.Checkpoint(),
			); err == nil {
				t.Fatal("ordinary promotion replay unexpectedly exposed recovery terminalization")
			}
		})
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorCarriesPredecessorChangesAcrossCanonicalLeap(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-canonical-leap")
	projectID := continuity.ProjectID("project-recovery-promotion-canonical-leap")
	state := stageReadyCanonicalPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("PromoteSyncAuthorityRecoverySuccessor() error = %v", err)
	}
	got, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	want := SyncAuthority{
		ChannelID:            state.Successor.Snapshot.ChannelID,
		RelayGeneration:      state.Successor.Snapshot.RelayGeneration,
		AdminPublicKey:       state.Successor.Snapshot.AdminPublicKey,
		MembershipGeneration: state.Successor.Snapshot.MembershipGeneration,
		InventoryArrivalHead: state.Successor.Snapshot.InventoryArrivalHead,
		Environments:         canonicalRecoveryPromotionEnvironmentsV1(),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical authority after recovery leap = %#v, want %#v", got, want)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorRejectsStaleRelayObservation(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-stale-watermark")
	projectID := continuity.ProjectID("project-recovery-promotion-stale-watermark")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	advanced := syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, state.Successor.Snapshot)
	advanced.RelayHead++
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(newer observation) error = %v", err)
	}
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(stale watermark) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorCursor)
		assertSyncAuthorityRecoveryProblemFieldV1(t, err, "inventory_arrival_head")
	}
	assertActiveSyncAuthorityRecoveryRowsV1(t, store, state)
	assertSyncAuthorityRecoveryTerminalReceiptCountV1(t, store, projectID, 0)
}

func TestPromoteSyncAuthorityRecoverySuccessorRollsBackReceiptAndCanonicalTogether(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-rollback")
	projectID := continuity.ProjectID("project-recovery-promotion-rollback")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	beforeCandidates := syncAuthorityCandidatePersistedRowsV2(t, store, projectID)
	beforeWatermark := readSyncAuthorityRecoveryWatermarkForTestV1(t, store, projectID, state.Successor.Snapshot)
	if _, err := store.db.Exec(`
CREATE TRIGGER reject_recovery_predecessor_delete
BEFORE DELETE ON continuity_sync_authority_candidates
WHEN OLD.project_id = '` + string(projectID) + `' AND OLD.role = 'recovery-predecessor'
BEGIN
  SELECT RAISE(ABORT, 'reject predecessor cleanup');
END`); err != nil {
		t.Fatalf("install predecessor cleanup trigger: %v", err)
	}
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(trigger rollback) error = nil")
	}
	assertActiveSyncAuthorityRecoveryRowsV1(t, store, state)
	assertSyncAuthorityRecoveryTerminalReceiptCountV1(t, store, projectID, 0)
	assertRecoveryCandidateRoleV1(t, store, projectID, state.Transition.PredecessorCandidateID, syncAuthorityCandidateRoleRecoveryPredecessorV1)
	assertRecoveryCandidateRoleV1(t, store, projectID, state.Transition.SuccessorCandidateID, syncAuthorityCandidateRoleRecoverySuccessorV1)
	current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != state {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after rollback) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, state)
	}
	afterCandidates := syncAuthorityCandidatePersistedRowsV2(t, store, projectID)
	if !reflect.DeepEqual(afterCandidates, beforeCandidates) {
		t.Fatalf("candidate graphs after rollback = %#v, want %#v", afterCandidates, beforeCandidates)
	}
	afterWatermark := readSyncAuthorityRecoveryWatermarkForTestV1(t, store, projectID, state.Successor.Snapshot)
	if afterWatermark != beforeWatermark {
		t.Fatalf("retained watermark after rollback = %#v, want %#v", afterWatermark, beforeWatermark)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorReceiptReplayIgnoresLaterCanonicalCorruption(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-replay-canonical")
	projectID := continuity.ProjectID("project-recovery-promotion-replay-canonical")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	want, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil {
		t.Fatalf("PromoteSyncAuthorityRecoverySuccessor() error = %v", err)
	}
	changedDigest := state.Successor.AuthorityDigest
	changedDigest[0] ^= 0xff
	if _, err := store.db.Exec(`
UPDATE continuity_sync_authorities
SET authority_digest = ?
WHERE project_id = ?`, changedDigest[:], string(projectID)); err != nil {
		t.Fatalf("corrupt later canonical digest: %v", err)
	}
	got, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil || got != want {
		t.Fatalf("receipt-first replay after canonical corruption = (%#v, %v), want (%#v, nil)", got, err, want)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorReplaysOldReceiptDuringDifferentActiveAttempt(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "recovery-promotion-old-replay")
	firstStore, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(first store) error = %v", err)
	}
	t.Cleanup(func() { firstStore.Close() })
	projectID := continuity.ProjectID("project-recovery-promotion-old-replay")
	oldState := stageReadyPredecessorSyncAuthorityRecoveryV1(t, firstStore, projectID)
	want, err := firstStore.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, oldState.Transition, oldState.Successor.Checkpoint(),
	)
	if err != nil {
		t.Fatalf("PromoteSyncAuthorityRecoverySuccessor(first attempt) error = %v", err)
	}

	nextStore, err := Open(stateRoot, "environment-next")
	if err != nil {
		t.Fatalf("Open(next writer) error = %v", err)
	}
	t.Cleanup(func() { nextStore.Close() })
	nextState := stageNextReadySyncAuthorityRecoveryV1(t, nextStore, projectID, "environment-next")
	if nextState.Transition.AttemptID == oldState.Transition.AttemptID {
		t.Fatal("different recovery attempts reused an attempt identity")
	}
	got, err := nextStore.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, oldState.Transition, oldState.Successor.Checkpoint(),
	)
	if err != nil || got != want {
		t.Fatalf("old receipt replay during new attempt = (%#v, %v), want (%#v, nil)", got, err, want)
	}
	current, found, err := nextStore.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != nextState {
		t.Fatalf("different active attempt after old replay = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, nextState)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorAuditsTerminalReceiptOnReplay(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-receipt-corruption")
	projectID := continuity.ProjectID("project-recovery-promotion-receipt-corruption")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err != nil {
		t.Fatalf("PromoteSyncAuthorityRecoverySuccessor() error = %v", err)
	}
	corruptDigest := testAuthorityDigest(0xe1)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_recovery_terminal_receipts
SET receipt_digest = ?
WHERE project_id = ? AND attempt_id = ?`,
		corruptDigest[:], string(projectID), state.Transition.AttemptID[:],
	); err != nil {
		t.Fatalf("corrupt recovery terminal receipt: %v", err)
	}
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(corrupt receipt) error = nil")
	} else {
		var problem *SyncError
		if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "sync_authority_recovery_terminal_receipt" {
			t.Fatalf("corrupt receipt replay error = %#v, want fielded store corruption", err)
		}
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorTerminalReplayRejectsMutatedIntent(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-terminal-mismatch")
	projectID := continuity.ProjectID("project-recovery-promotion-terminal-mismatch")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	want, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil {
		t.Fatalf("PromoteSyncAuthorityRecoverySuccessor() error = %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*SyncAuthorityRecoveryTransition, *SyncAuthorityCandidateCheckpoint)
	}{
		{name: "attempt", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.AttemptID[0] ^= 0xff
		}},
		{name: "predecessor", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.PredecessorCandidateID[0] ^= 0xff
		}},
		{name: "successor", mutate: func(transition *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			transition.SuccessorCandidateID[0] ^= 0xff
			checkpoint.CandidateID = transition.SuccessorCandidateID
		}},
		{name: "writer environment", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.WriterEnvironmentID = "environment-other"
		}},
		{name: "writer certificate", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.WriterCertificateID[0] ^= 0xff
		}},
		{name: "target generation", mutate: func(transition *SyncAuthorityRecoveryTransition, _ *SyncAuthorityCandidateCheckpoint) {
			transition.TargetMembershipGeneration++
		}},
		{name: "page count", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.PageCount++
		}},
		{name: "environment count", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.EnvironmentCount++
		}},
		{name: "through environment", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.ThroughEnvironmentID = "environment-z"
		}},
		{name: "rolling digest", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.RollingEnvironmentDigest[0] ^= 0xff
		}},
		{name: "authority digest", mutate: func(_ *SyncAuthorityRecoveryTransition, checkpoint *SyncAuthorityCandidateCheckpoint) {
			checkpoint.AuthorityDigest[0] ^= 0xff
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transition := state.Transition
			checkpoint := state.Successor.Checkpoint()
			test.mutate(&transition, &checkpoint)
			if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, transition, checkpoint,
			); err == nil {
				t.Fatal("PromoteSyncAuthorityRecoverySuccessor(mutated terminal intent) error = nil")
			}
		})
	}
	retried, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil || retried != want {
		t.Fatalf("exact retry after mutation refusals = (%#v, %v), want (%#v, nil)", retried, err, want)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorRefusesAbortedAttempt(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-aborted")
	projectID := continuity.ProjectID("project-recovery-promotion-aborted")
	predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 5),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 5, 5)
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
	ready := state.Successor.Checkpoint()
	ready.Ready = true
	ready.AuthorityDigest = testAuthorityDigest(0xf1)
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, ready,
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(aborted attempt) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorConflict)
	}
	var canonical int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authorities WHERE project_id = ?`, string(projectID)).Scan(&canonical); err != nil {
		t.Fatalf("count canonical authority: %v", err)
	}
	if canonical != 0 {
		t.Fatalf("canonical authority rows = %d, want zero", canonical)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorReauditsCandidateGraph(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-reaudit")
	projectID := continuity.ProjectID("project-recovery-promotion-reaudit")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	if _, err := store.db.Exec(`
DELETE FROM continuity_sync_authority_candidate_membership_events
WHERE project_id = ? AND candidate_id = ?
  AND membership_generation = ?`,
		string(projectID), state.Transition.SuccessorCandidateID[:], state.Transition.TargetMembershipGeneration,
	); err != nil {
		t.Fatalf("remove exact writer join event: %v", err)
	}
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(corrupt candidate graph) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorStore)
	}
	assertSyncAuthorityRecoveryTerminalReceiptCountV1(t, store, projectID, 0)
	var transitions, canonical int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions WHERE project_id = ?`, string(projectID)).Scan(&transitions); err != nil {
		t.Fatalf("count recovery transitions: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authorities WHERE project_id = ?`, string(projectID)).Scan(&canonical); err != nil {
		t.Fatalf("count canonical authority: %v", err)
	}
	if transitions != 1 || canonical != 0 {
		t.Fatalf("reaudit refusal rows = transition:%d canonical:%d, want 1/0", transitions, canonical)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorConcurrentExactCallersAndReopen(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "recovery-promotion-concurrent")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-recovery-promotion-concurrent")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, stores[0], projectID)
	type result struct {
		receipt SyncAuthorityRecoveryTerminalReceipt
		err     error
	}
	results := make([]result, len(stores))
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			results[index].receipt, results[index].err = stores[index].PromoteSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
			)
		}(index)
	}
	close(start)
	group.Wait()
	if results[0].err != nil || results[1].err != nil || results[0].receipt != results[1].receipt {
		t.Fatalf("concurrent exact recovery promotions = %#v", results)
	}

	reopened, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(retry) error = %v", err)
	}
	t.Cleanup(func() { reopened.Close() })
	retried, err := reopened.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil || retried != results[0].receipt {
		t.Fatalf("reopened exact retry = (%#v, %v), want (%#v, nil)", retried, err, results[0].receipt)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorRefusesNonReadyAndMutatedIntent(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-refusals")
	projectID := continuity.ProjectID("project-recovery-promotion-refusals")
	predecessorSnapshot, _, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	environments := syncAuthorityRecoveryEnvironmentsV1(5, 2)
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 8, 5)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments[:4], true),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if state.Successor.Ready {
		t.Fatalf("staging successor = %#v, want non-ready", state.Successor)
	}
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(non-ready) error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemFieldV1(t, err, "successor_checkpoint")
	}

	changedTransition := state.Transition
	changedTransition.WriterCertificateID[0] ^= 0xff
	readyCheckpoint := state.Successor.Checkpoint()
	readyCheckpoint.Ready = true
	readyCheckpoint.AuthorityDigest = state.Successor.RollingEnvironmentDigest
	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, changedTransition, readyCheckpoint,
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(changed transition) error = nil")
	}
	current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != state {
		t.Fatalf("current recovery state = (%#v, %v, %v), want unchanged", current, found, err)
	}
}

func TestPromoteSyncAuthorityRecoverySuccessorFailsClosedOnActiveTerminalCorruption(t *testing.T) {
	store := openSyncStore(t, "recovery-promotion-active-terminal")
	projectID := continuity.ProjectID("project-recovery-promotion-active-terminal")
	state := stageReadyPredecessorSyncAuthorityRecoveryV1(t, store, projectID)
	receipt, err := newSyncAuthorityRecoveryTerminalReceiptV1(
		SyncAuthorityRecoveryPromoted, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil {
		t.Fatalf("newSyncAuthorityRecoveryTerminalReceiptV1() error = %v", err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := insertSyncAuthorityRecoveryTerminalReceiptV1(context.Background(), tx, receipt); err != nil {
		tx.Rollback()
		t.Fatalf("insertSyncAuthorityRecoveryTerminalReceiptV1() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("seed terminal corruption commit error = %v", err)
	}

	if _, err := store.PromoteSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(),
	); err == nil {
		t.Fatal("PromoteSyncAuthorityRecoverySuccessor(active and terminal) error = nil")
	} else {
		var problem *SyncError
		if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "sync_authority_recovery_transition" {
			t.Fatalf("active and terminal error = %#v, want fielded store corruption", err)
		}
	}
	assertActiveSyncAuthorityRecoveryRowsV1(t, store, state)
}

func stageReadyGenerationOneSyncAuthorityRecoveryV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
) SyncAuthorityRecoveryState {
	t.Helper()
	environments := syncAuthorityRecoveryEnvironmentsV1(1, 1)
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, snapshot, 0),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID,
		SyncAuthorityRecoveryTransitionStart{
			WriterEnvironmentID:        continuity.EnvironmentID(environments[0].EnvironmentID),
			WriterCertificateID:        environments[0].CertificateID,
			TargetMembershipGeneration: 1,
			SuccessorSnapshot:          snapshot,
		},
		syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if !state.Successor.Ready || state.Transition.PredecessorCandidateID != ([32]byte{}) {
		t.Fatalf("generation-one ready recovery state = %#v", state)
	}
	return state
}

func stageReadyPredecessorSyncAuthorityRecoveryV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
) SyncAuthorityRecoveryState {
	t.Helper()
	environments := syncAuthorityRecoveryEnvironmentsV1(2, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(t, store, projectID, environments)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 8, 2)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	if !state.Successor.Ready || state.Transition.PredecessorCandidateID != predecessor.CandidateID {
		t.Fatalf("predecessor ready recovery state = %#v", state)
	}
	return state
}

func stageReadyCanonicalPredecessorSyncAuthorityRecoveryV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
) SyncAuthorityRecoveryState {
	t.Helper()
	canonical := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
	}
	predecessorSnapshot := syncAuthoritySnapshotFromAuthorityV2(
		canonical, binding.AuthorityDigestVersion, binding.AuthorityDigest,
	)
	predecessorSnapshot.MembershipGeneration = canonical.MembershipGeneration + 1
	predecessorSnapshot.InventoryArrivalHead = 7
	successorEnvironments := canonicalRecoveryPromotionEnvironmentsV1()
	predecessorEnvironments := successorEnvironments[:len(successorEnvironments)-1]
	predecessor, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, predecessorSnapshot,
		syncAuthorityCandidatePageV2("", predecessorEnvironments, false),
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(predecessor) error = %v", err)
	}
	if !predecessor.Ready {
		t.Fatalf("canonical predecessor = %#v, want READY", predecessor)
	}
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(
		predecessor, successorEnvironments[len(successorEnvironments)-1], 8, canonical.MembershipGeneration+2,
	)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start,
		syncAuthorityCandidatePageV2("", successorEnvironments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition(canonical predecessor) error = %v", err)
	}
	if !state.Successor.Ready || state.Transition.PredecessorCandidateID != predecessor.CandidateID {
		t.Fatalf("canonical predecessor recovery state = %#v", state)
	}
	return state
}

func canonicalRecoveryPromotionEnvironmentsV1() []SyncEnvironmentCertificate {
	canonical := testSyncAuthority()
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	environments = append(environments,
		SyncEnvironmentCertificate{
			EnvironmentID:            "environment-c",
			CertificateID:            testSyncCertificateID("recovery-environment-c"),
			CertificateBytes:         []byte("recovery environment-c certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: canonical.MembershipGeneration + 1,
		},
		SyncEnvironmentCertificate{
			EnvironmentID:            "environment-local",
			CertificateID:            testSyncCertificateID("recovery-environment-local"),
			CertificateBytes:         []byte("recovery environment-local certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: canonical.MembershipGeneration + 2,
		},
	)
	return environments
}

func stageNextReadySyncAuthorityRecoveryV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	writerID string,
) SyncAuthorityRecoveryState {
	t.Helper()
	canonical, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(next recovery) error = %v", err)
	}
	binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding(next recovery) error = %v", err)
	}
	predecessorSnapshot := syncAuthoritySnapshotFromAuthorityV2(
		canonical, binding.AuthorityDigestVersion, binding.AuthorityDigest,
	)
	predecessor, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, predecessorSnapshot,
		syncAuthorityCandidatePageV2("", cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments), false),
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(next predecessor) error = %v", err)
	}
	nextHead := canonical.InventoryArrivalHead + 1
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, nextHead),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(next recovery) error = %v", err)
	}
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	writer := SyncEnvironmentCertificate{
		EnvironmentID:            writerID,
		CertificateID:            testSyncCertificateID("recovery-" + writerID),
		CertificateBytes:         []byte("recovery certificate for " + writerID),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: canonical.MembershipGeneration + 1,
	}
	environments = append(environments, writer)
	start := syncAuthorityRecoveryStartV1(
		predecessor, writer, nextHead, canonical.MembershipGeneration+1,
	)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition(next attempt) error = %v", err)
	}
	return state
}

func assertPromotedSyncAuthorityRecoveryRowsV1(t *testing.T, store *Store, state SyncAuthorityRecoveryState) {
	t.Helper()
	var transitions, predecessor, successor, successorChildren, receipts int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions WHERE project_id = ?`, string(state.Transition.ProjectID)).Scan(&transitions); err != nil {
		t.Fatalf("count recovery transitions: %v", err)
	}
	if state.Transition.PredecessorCandidateID != ([32]byte{}) {
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_candidates WHERE project_id = ? AND candidate_id = ?`, string(state.Transition.ProjectID), state.Transition.PredecessorCandidateID[:]).Scan(&predecessor); err != nil {
			t.Fatalf("count predecessor: %v", err)
		}
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_candidates WHERE project_id = ? AND candidate_id = ?`, string(state.Transition.ProjectID), state.Transition.SuccessorCandidateID[:]).Scan(&successor); err != nil {
		t.Fatalf("read successor header: %v", err)
	}
	if err := store.db.QueryRow(`SELECT (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages WHERE project_id = ? AND candidate_id = ?) + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments WHERE project_id = ? AND candidate_id = ?) + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events WHERE project_id = ? AND candidate_id = ?)`, string(state.Transition.ProjectID), state.Transition.SuccessorCandidateID[:], string(state.Transition.ProjectID), state.Transition.SuccessorCandidateID[:], string(state.Transition.ProjectID), state.Transition.SuccessorCandidateID[:]).Scan(&successorChildren); err != nil {
		t.Fatalf("count successor children: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_terminal_receipts WHERE project_id = ? AND attempt_id = ? AND outcome = 'promoted'`, string(state.Transition.ProjectID), state.Transition.AttemptID[:]).Scan(&receipts); err != nil {
		t.Fatalf("count promoted terminal receipts: %v", err)
	}
	if transitions != 0 || predecessor != 0 || successor != 0 || successorChildren != 0 || receipts != 1 {
		t.Fatalf("promoted recovery rows = transitions:%d predecessor:%d successor:%d children:%d receipts:%d", transitions, predecessor, successor, successorChildren, receipts)
	}
}

func assertActiveSyncAuthorityRecoveryRowsV1(t *testing.T, store *Store, state SyncAuthorityRecoveryState) {
	t.Helper()
	var transitions, canonical int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions WHERE project_id = ? AND attempt_id = ?`, string(state.Transition.ProjectID), state.Transition.AttemptID[:]).Scan(&transitions); err != nil {
		t.Fatalf("count active transition: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authorities WHERE project_id = ?`, string(state.Transition.ProjectID)).Scan(&canonical); err != nil {
		t.Fatalf("count canonical authority: %v", err)
	}
	if transitions != 1 || canonical != 0 {
		t.Fatalf("active recovery rows = transition:%d canonical:%d, want 1/0", transitions, canonical)
	}
}

func assertSyncAuthorityRecoveryTerminalReceiptCountV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	want int64,
) {
	t.Helper()
	var got int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_terminal_receipts WHERE project_id = ?`, string(projectID)).Scan(&got); err != nil {
		t.Fatalf("count recovery terminal receipts: %v", err)
	}
	if got != want {
		t.Fatalf("recovery terminal receipt count = %d, want %d", got, want)
	}
}

func readSyncAuthorityRecoveryWatermarkForTestV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
) syncRelayWatermarkRecordV1 {
	t.Helper()
	tx, err := store.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("BeginTx(read watermark) error = %v", err)
	}
	defer tx.Rollback()
	key := syncRelayWatermarkKeyFromValueV1(syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, snapshot))
	record, found, err := readSyncRelayWatermarkV1(context.Background(), tx, key)
	if err != nil || !found {
		t.Fatalf("readSyncRelayWatermarkV1() = (%#v, %v, %v), want found", record, found, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit read watermark transaction: %v", err)
	}
	return record
}
