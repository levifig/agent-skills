package sqlite

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncAuthorityRecoverySuccessorReplacementAcceptsSameHeadMembershipAdvance(t *testing.T) {
	for _, test := range []struct {
		name  string
		ready bool
	}{
		{name: "staging"},
		{name: "ready", ready: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "recovery-successor-same-head-membership-"+test.name)
			projectID := continuity.ProjectID("project-recovery-successor-same-head-membership-" + test.name)
			environmentCount := 6
			initialMembership := uint32(6)
			if test.ready {
				environmentCount = 4
				initialMembership = 4
			}
			environments := syncAuthorityRecoveryEnvironmentsV1(environmentCount, 2)
			predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(
				t, store, projectID, environments,
			)
			watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 20)
			if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
				t.Fatalf("AdvanceSyncRelayWatermark(predecessor) error = %v", err)
			}

			start := syncAuthorityRecoveryStartV1(
				predecessor, environments[len(environments)-1], 20, initialMembership,
			)
			initialPage := syncAuthorityCandidatePageV2("", environments[:4], true)
			if test.ready {
				initialPage = syncAuthorityCandidatePageV2("", environments, false)
			}
			stale, err := store.BeginSyncAuthorityRecoveryTransition(
				context.Background(), projectID, start, initialPage,
			)
			if err != nil {
				t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
			}
			if stale.Successor.Ready != test.ready {
				t.Fatalf("initial successor ready = %v, want %v", stale.Successor.Ready, test.ready)
			}

			advanced := syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, start.SuccessorSnapshot)
			replacementMembership := initialMembership + 1
			advanced.MembershipGeneration = replacementMembership
			if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
				t.Fatalf("AdvanceSyncRelayWatermark(same-head membership) = (%#v, %v), want (%#v, nil)", got, err, advanced)
			}
			replacementSnapshot := start.SuccessorSnapshot
			replacementSnapshot.MembershipGeneration = replacementMembership
			replacementPage := initialPage
			if test.ready {
				replacementEnvironments := cloneSyncAuthorityCandidateEnvironmentsV2(environments)
				replacementEnvironments[0].Retirement = &SyncEnvironmentRetirement{
					RelayGeneration:      replacementSnapshot.RelayGeneration,
					MembershipGeneration: replacementMembership,
					RetirementID:         sha256.Sum256([]byte("recovery replacement same-head retirement")),
					RetirementBytes:      []byte("recovery replacement same-head retirement"),
				}
				sortSyncAuthorityRecoveryEnvironmentsV1(replacementEnvironments)
				replacementPage = syncAuthorityCandidatePageV2("", replacementEnvironments, false)
			}
			replaced, err := store.ReplaceSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(),
				replacementSnapshot, replacementPage,
			)
			if err != nil {
				t.Fatalf("ReplaceSyncAuthorityRecoverySuccessor(same-head membership) error = %v", err)
			}
			if replaced.Transition.AttemptID != stale.Transition.AttemptID ||
				replaced.Transition.PredecessorCandidateID != stale.Transition.PredecessorCandidateID ||
				replaced.Transition.SuccessorCandidateID == stale.Transition.SuccessorCandidateID ||
				replaced.Successor.Snapshot != replacementSnapshot || replaced.Successor.Ready != test.ready {
				t.Fatalf("same-head membership replacement = %#v, stale = %#v", replaced, stale)
			}
			if recoveryCandidateExistsV1(t, store, projectID, stale.Successor.CandidateID) {
				t.Fatal("stale successor survived same-head membership replacement")
			}

			later := advanced
			later.MembershipGeneration = replacementMembership + 1
			later.RelayHead = 21
			if got, err := store.AdvanceSyncRelayWatermark(context.Background(), later); err != nil || got != later {
				t.Fatalf("AdvanceSyncRelayWatermark(after replacement) = (%#v, %v), want (%#v, nil)", got, err, later)
			}
			replayed, err := store.ReplaceSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(),
				replacementSnapshot, replacementPage,
			)
			if err != nil || replayed != replaced {
				t.Fatalf("ReplaceSyncAuthorityRecoverySuccessor(exact stale replay) = (%#v, %v), want (%#v, nil)", replayed, err, replaced)
			}
		})
	}
}

func TestSyncAuthorityRecoverySuccessorReplacementRejectsCrossedFrontierRegressions(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-crossed-frontier")
	projectID := continuity.ProjectID("project-recovery-successor-crossed-frontier")
	environments := syncAuthorityRecoveryEnvironmentsV1(6, 2)
	predecessorSnapshot, predecessor := stageReadySyncAuthorityRecoveryPredecessorV1(
		t, store, projectID, environments,
	)
	watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 20)
	if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark(predecessor) error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, environments[len(environments)-1], 20, 6)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	stale, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}

	membershipAdvance := syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, start.SuccessorSnapshot)
	membershipAdvance.MembershipGeneration = 7
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), membershipAdvance); err != nil || got != membershipAdvance {
		t.Fatalf("AdvanceSyncRelayWatermark(membership) = (%#v, %v)", got, err)
	}
	headAdvance := syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, start.SuccessorSnapshot)
	headAdvance.RelayHead = 21
	wantFloor := membershipAdvance
	wantFloor.RelayHead = 21
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), headAdvance); err != nil || got != wantFloor {
		t.Fatalf("AdvanceSyncRelayWatermark(crossed head) = (%#v, %v), want (%#v, nil)", got, err, wantFloor)
	}

	for _, test := range []struct {
		name     string
		snapshot SyncAuthoritySnapshot
		field    string
	}{
		{
			name: "head below joined floor",
			snapshot: func() SyncAuthoritySnapshot {
				value := start.SuccessorSnapshot
				value.MembershipGeneration = 7
				return value
			}(),
			field: "inventory_arrival_head",
		},
		{
			name: "membership below joined floor",
			snapshot: func() SyncAuthoritySnapshot {
				value := start.SuccessorSnapshot
				value.InventoryArrivalHead = 21
				return value
			}(),
			field: "membership_generation",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.ReplaceSyncAuthorityRecoverySuccessor(
				context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(),
				test.snapshot, firstPage,
			); err == nil {
				t.Fatal("ReplaceSyncAuthorityRecoverySuccessor(crossed regression) error = nil")
			} else {
				assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorCursor)
				assertSyncAuthorityRecoveryProblemFieldV1(t, err, test.field)
			}
			current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
			if err != nil || !found || current != stale {
				t.Fatalf("recovery state after crossed regression = (%#v, %v, %v), want stale %#v", current, found, err, stale)
			}
		})
	}

	replacementSnapshot := start.SuccessorSnapshot
	replacementSnapshot.MembershipGeneration = 7
	replacementSnapshot.InventoryArrivalHead = 21
	replaced, err := store.ReplaceSyncAuthorityRecoverySuccessor(
		context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(),
		replacementSnapshot, firstPage,
	)
	if err != nil {
		t.Fatalf("ReplaceSyncAuthorityRecoverySuccessor(joined frontier) error = %v", err)
	}
	if replaced.Successor.Snapshot != replacementSnapshot ||
		replaced.Transition.AttemptID != stale.Transition.AttemptID {
		t.Fatalf("joined-frontier replacement = %#v, stale = %#v", replaced, stale)
	}
}
