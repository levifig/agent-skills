package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestCurrentSyncPruneWitnessAuthorityUnderBindingStreamsLargeV2History(t *testing.T) {
	t.Parallel()

	const environmentCount = 4_097
	store := openSyncStore(t, "prune-witness-large-v2")
	projectID := continuity.ProjectID("project-prune-witness-large-v2")
	environments := syncAuthorityCandidateManyEnvironmentsV2(environmentCount)
	for index := 0; index < environmentCount-1; index++ {
		environmentID := environments[index].EnvironmentID
		environments[index].Retirement = &SyncEnvironmentRetirement{
			RelayGeneration:      syncAuthorityCandidateBootstrapSnapshotV2(1).RelayGeneration,
			MembershipGeneration: uint32(environmentCount + index + 1),
			RetirementID:         sha256.Sum256([]byte("prune-witness-retirement:" + environmentID)),
			RetirementBytes:      []byte("retirement bytes for " + environmentID),
		}
	}
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(environmentCount*2 - 1)
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, environments)
	binding := seedAndBindSyncPruneWitnessAuthorityV2(t, store, projectID, authority)

	current, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
		context.Background(), projectID, binding, binding.MembershipGeneration,
	)
	if err != nil {
		t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(current) error = %v", err)
	}
	if current.Binding != binding || current.MembershipGeneration != binding.MembershipGeneration || len(current.Environments) != 1 {
		t.Fatalf("current prune witnesses = %#v, want exact binding and one active environment", current)
	}
	if !syncEnvironmentCertificateEqual(current.Environments[0], authority.Environments[environmentCount-1]) {
		t.Fatalf("current prune witness = %#v, want final environment", current.Environments[0])
	}

	const historicalGeneration = 100
	historical, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
		context.Background(), projectID, binding, historicalGeneration,
	)
	if err != nil {
		t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(historical) error = %v", err)
	}
	if historical.MembershipGeneration != historicalGeneration || len(historical.Environments) != historicalGeneration {
		t.Fatalf("historical prune witnesses = generation %d, count %d; want %d and %d", historical.MembershipGeneration, len(historical.Environments), historicalGeneration, historicalGeneration)
	}
	for index := range historical.Environments {
		if !syncEnvironmentCertificateEqual(historical.Environments[index], authority.Environments[index]) {
			t.Fatalf("historical prune witness %d differs from canonical environment", index)
		}
	}
	historical.Environments[0].CertificateBytes[0] ^= 0xff
	historical.Environments[0].Retirement.RetirementBytes[0] ^= 0xff

	reloaded, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
		context.Background(), projectID, binding, historicalGeneration,
	)
	if err != nil {
		t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(after caller mutation) error = %v", err)
	}
	if !syncEnvironmentCertificateEqual(reloaded.Environments[0], authority.Environments[0]) {
		t.Fatal("CurrentSyncPruneWitnessAuthorityUnderBinding() exposed mutable certificate or retirement bytes")
	}
}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingEnforcesActiveBound(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		count     int
		wantError bool
	}{
		{name: "exactly 256", count: maximumSyncPruneWitnessEnvironments},
		{name: "257", count: maximumSyncPruneWitnessEnvironments + 1, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "prune-witness-active-bound-"+test.name)
			projectID := continuity.ProjectID(fmt.Sprintf("project-prune-witness-active-bound-%d", test.count))
			snapshot := syncAuthorityCandidateBootstrapSnapshotV2(test.count)
			authority := syncAuthorityFromSnapshotForBindingTest(snapshot, syncAuthorityCandidateManyEnvironmentsV2(test.count))
			binding := seedAndBindSyncPruneWitnessAuthorityV2(t, store, projectID, authority)

			got, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
				context.Background(), projectID, binding, uint32(test.count),
			)
			if test.wantError {
				assertSyncPruneWitnessError(t, err, SyncErrorConflict, "prune_witness_authority")
				if got.Binding != (SyncAuthorityBinding{}) || got.MembershipGeneration != 0 || got.Environments != nil {
					t.Fatalf("overflow result = %#v, want no partial witnesses", got)
				}
				return
			}
			if err != nil || len(got.Environments) != test.count {
				t.Fatalf("bounded prune witnesses = (%d, %v), want (%d, nil)", len(got.Environments), err, test.count)
			}
		})
	}
}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingCancelsDuringLargeV2Scan(t *testing.T) {
	t.Parallel()

	const environmentCount = 1_024
	store := openSyncStore(t, "prune-witness-mid-scan-cancel")
	projectID := continuity.ProjectID("project-prune-witness-mid-scan-cancel")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(environmentCount)
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, syncAuthorityCandidateManyEnvironmentsV2(environmentCount))
	binding := seedAndBindSyncPruneWitnessAuthorityV2(t, store, projectID, authority)
	ctx := newSyncPruneWitnessCancelAfterChecksContext(context.Background(), 128)

	got, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
		ctx, projectID, binding, binding.MembershipGeneration,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(mid-scan cancellation) error = %v, want context.Canceled", err)
	}
	if got.Binding != (SyncAuthorityBinding{}) || got.MembershipGeneration != 0 || got.Environments != nil {
		t.Fatalf("mid-scan cancellation result = %#v, want no partial witnesses", got)
	}
	if checks := ctx.Checks(); checks < 128 {
		t.Fatalf("context checks = %d, want cancellation after scan progress", checks)
	}
}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingSelectsHistoricalV1Witnesses(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "prune-witness-v1")
	projectID := continuity.ProjectID("project-prune-witness-v1")
	authority := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)

	tests := []struct {
		generation uint32
		wantIDs    []string
	}{
		{generation: 1, wantIDs: []string{"environment-a"}},
		{generation: 2, wantIDs: []string{"environment-a", "environment-b"}},
		{generation: 3, wantIDs: []string{"environment-b"}},
	}
	for _, test := range tests {
		got, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
			context.Background(), projectID, binding, test.generation,
		)
		if err != nil {
			t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(%d) error = %v", test.generation, err)
		}
		if got.Binding != binding || got.MembershipGeneration != test.generation || len(got.Environments) != len(test.wantIDs) {
			t.Fatalf("prune witnesses at generation %d = %#v, want IDs %v", test.generation, got, test.wantIDs)
		}
		for index, wantID := range test.wantIDs {
			if got.Environments[index].EnvironmentID != wantID {
				t.Fatalf("prune witness %d at generation %d = %q, want %q", index, test.generation, got.Environments[index].EnvironmentID, wantID)
			}
		}
	}

	mutable, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 2)
	if err != nil {
		t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(mutable) error = %v", err)
	}
	mutable.Environments[0].CertificateBytes[0] ^= 0xff
	mutable.Environments[0].Retirement.RetirementBytes[0] ^= 0xff
	reloaded, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 2)
	if err != nil || !syncEnvironmentCertificateEqual(reloaded.Environments[0], authority.Environments[0]) {
		t.Fatalf("defensive v1 prune witness = (%#v, %v), want exact canonical environment", reloaded, err)
	}
}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingAllowsEmptyHistoricalSet(t *testing.T) {
	t.Parallel()

	for _, digestVersion := range []uint16{1, 2} {
		t.Run(fmt.Sprintf("version %d", digestVersion), func(t *testing.T) {
			store := openSyncStore(t, fmt.Sprintf("prune-witness-empty-v%d", digestVersion))
			projectID := continuity.ProjectID(fmt.Sprintf("project-prune-witness-empty-v%d", digestVersion))
			authority := syncPruneWitnessAuthorityWithEmptyGenerationV1()
			var binding SyncAuthorityBinding
			if digestVersion == 1 {
				if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
					t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
				}
				binding = currentSyncAuthorityBindingForTest(t, store, projectID)
			} else {
				authority.InventoryArrivalHead = 1
				binding = seedAndBindSyncPruneWitnessAuthorityV2(t, store, projectID, authority)
			}

			got, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
				context.Background(), projectID, binding, 2,
			)
			if err != nil {
				t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(empty) error = %v", err)
			}
			if got.Binding != binding || got.MembershipGeneration != 2 || len(got.Environments) != 0 {
				t.Fatalf("empty historical prune witnesses = %#v, want exact empty generation 2 set", got)
			}
		})
	}
}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingAuditsCompleteV2History(t *testing.T) {
	t.Parallel()

	for _, environmentID := range []string{"environment-a", "environment-b"} {
		t.Run("corrupt "+environmentID, func(t *testing.T) {
			store := openSyncStore(t, "prune-witness-corrupt-"+environmentID)
			projectID := continuity.ProjectID("project-prune-witness-corrupt-" + environmentID)
			authority := testSyncAuthority()
			authority.InventoryArrivalHead = 1
			binding := seedAndBindSyncPruneWitnessAuthorityV2(t, store, projectID, authority)
			if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_certificates
SET certificate_bytes = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), environmentID); err != nil {
				t.Fatalf("corrupt canonical environment: %v", err)
			}
			_, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(
				context.Background(), projectID, binding, binding.MembershipGeneration,
			)
			assertSyncPruneWitnessError(t, err, SyncErrorStore, "sync_authority")
		})
	}

}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingRequiresExactStableAuthority(t *testing.T) {
	t.Parallel()

	t.Run("binding drift", func(t *testing.T) {
		store := openSyncStore(t, "prune-witness-binding-drift")
		projectID := continuity.ProjectID("project-prune-witness-binding-drift")
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority()); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		binding.AuthorityDigest[0] ^= 0xff
		_, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 1)
		assertSyncPruneWitnessError(t, err, SyncErrorConflict, "sync_authority")
	})

	for _, test := range []struct {
		name      string
		wantField string
		advance   func(*SyncRelayWatermark)
	}{
		{
			name:      "advanced membership frontier",
			wantField: "membership_generation",
			advance: func(watermark *SyncRelayWatermark) {
				watermark.MembershipGeneration++
			},
		},
		{
			name:      "advanced arrival frontier",
			wantField: "inventory_arrival_head",
			advance: func(watermark *SyncRelayWatermark) {
				watermark.RelayHead++
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "prune-witness-frontier-"+test.wantField)
			projectID := continuity.ProjectID("project-prune-witness-frontier-" + test.wantField)
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority()); err != nil {
				t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
			}
			binding := currentSyncAuthorityBindingForTest(t, store, projectID)
			advanced := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
			test.advance(&advanced)
			if got, err := store.AdvanceSyncRelayWatermark(context.Background(), advanced); err != nil || got != advanced {
				t.Fatalf("AdvanceSyncRelayWatermark(advanced) = (%#v, %v), want (%#v, nil)", got, err, advanced)
			}
			_, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 1)
			assertSyncPruneWitnessError(t, err, SyncErrorCursor, test.wantField)
		})
	}

	t.Run("active ordinary candidate", func(t *testing.T) {
		store := openSyncStore(t, "prune-witness-candidate")
		projectID := continuity.ProjectID("project-prune-witness-candidate")
		authority := testSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
		_, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 1)
		assertSyncPruneWitnessError(t, err, SyncErrorConflict, "sync_authority_candidate")
	})

	t.Run("active recovery transition", func(t *testing.T) {
		fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "prune-witness-transition")
		binding := SyncAuthorityBinding{
			ChannelID:              fixture.predecessor.Snapshot.ChannelID,
			RelayGeneration:        fixture.predecessor.Snapshot.RelayGeneration,
			AdminPublicKey:         fixture.predecessor.Snapshot.AdminPublicKey,
			MembershipGeneration:   fixture.predecessor.Snapshot.MembershipGeneration,
			InventoryArrivalHead:   fixture.predecessor.Snapshot.InventoryArrivalHead,
			AuthorityDigestVersion: fixture.predecessor.Snapshot.BaseAuthorityDigestVersion,
			AuthorityDigest:        fixture.predecessor.Snapshot.BaseAuthorityDigest,
		}
		_, err := fixture.store.CurrentSyncPruneWitnessAuthorityUnderBinding(
			context.Background(), fixture.projectID, binding, binding.MembershipGeneration,
		)
		assertSyncPruneWitnessError(t, err, SyncErrorConflict, "sync_authority_recovery_transition")
	})
}

func TestCurrentSyncPruneWitnessAuthorityUnderBindingValidatesInputsAndLifecycle(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "prune-witness-inputs")
	projectID := continuity.ProjectID("project-prune-witness-inputs")
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority()); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)

	invalidBinding := binding
	invalidBinding.AuthorityDigest = [32]byte{}
	_, err := store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, invalidBinding, 1)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	_, err = store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), "invalid project", binding, 1)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	_, err = store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 0)
	assertSyncPruneWitnessError(t, err, SyncErrorInvalid, "membership_generation")
	_, err = store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, binding.MembershipGeneration+1)
	assertSyncPruneWitnessError(t, err, SyncErrorInvalid, "membership_generation")
	_, err = store.CurrentSyncPruneWitnessAuthorityUnderBinding(nil, projectID, binding, 1)
	assertSyncErrorCode(t, err, SyncErrorInvalid)
	_, err = (*Store)(nil).CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 1)
	assertSyncErrorCode(t, err, SyncErrorStore)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.CurrentSyncPruneWitnessAuthorityUnderBinding(canceled, projectID, binding, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CurrentSyncPruneWitnessAuthorityUnderBinding(canceled) error = %v, want context.Canceled", err)
	}

	missingStore := openSyncStore(t, "prune-witness-missing")
	_, err = missingStore.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 1)
	assertSyncErrorCode(t, err, SyncErrorNotFound)

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	_, err = store.CurrentSyncPruneWitnessAuthorityUnderBinding(context.Background(), projectID, binding, 1)
	assertSyncErrorCode(t, err, SyncErrorStore)
}

func seedAndBindSyncPruneWitnessAuthorityV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	authority SyncAuthority,
) SyncAuthorityBinding {
	t.Helper()
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	binding := syncAuthorityBindingForTest(authority, 2, digest)
	wantWatermark := syncRelayWatermarkFromAuthorityBindingV1(projectID, binding)
	if got, err := store.AdvanceSyncRelayWatermark(context.Background(), wantWatermark); err != nil || got != wantWatermark {
		t.Fatalf("AdvanceSyncRelayWatermark() = (%#v, %v), want (%#v, nil)", got, err, wantWatermark)
	}
	return binding
}

func syncPruneWitnessAuthorityWithEmptyGenerationV1() SyncAuthority {
	relayGeneration := testAuthorityDigest(0x61)
	return SyncAuthority{
		ChannelID:            testSyncChannelID("prune-witness-empty-channel"),
		RelayGeneration:      relayGeneration,
		AdminPublicKey:       testAuthorityDigest(0x62),
		MembershipGeneration: 3,
		Environments: []SyncEnvironmentCertificate{
			{
				EnvironmentID:            "environment-a",
				CertificateID:            testAuthorityDigest(0x63),
				CertificateBytes:         []byte("environment-a-certificate"),
				Mode:                     SyncEnvironmentTrusted,
				JoinMembershipGeneration: 1,
				Retirement: &SyncEnvironmentRetirement{
					RelayGeneration:      relayGeneration,
					MembershipGeneration: 2,
					RetirementID:         testAuthorityDigest(0x64),
					RetirementBytes:      []byte("environment-a-retirement"),
				},
			},
			{
				EnvironmentID:            "environment-b",
				CertificateID:            testAuthorityDigest(0x65),
				CertificateBytes:         []byte("environment-b-certificate"),
				Mode:                     SyncEnvironmentTrusted,
				JoinMembershipGeneration: 3,
			},
		},
	}
}

type syncPruneWitnessCancelAfterChecksContext struct {
	parent      context.Context
	done        chan struct{}
	mu          sync.Mutex
	checks      int
	cancelAfter int
	canceled    bool
}

func newSyncPruneWitnessCancelAfterChecksContext(parent context.Context, cancelAfter int) *syncPruneWitnessCancelAfterChecksContext {
	return &syncPruneWitnessCancelAfterChecksContext{
		parent:      parent,
		done:        make(chan struct{}),
		cancelAfter: cancelAfter,
	}
}

func (ctx *syncPruneWitnessCancelAfterChecksContext) Deadline() (time.Time, bool) {
	return ctx.parent.Deadline()
}

func (ctx *syncPruneWitnessCancelAfterChecksContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *syncPruneWitnessCancelAfterChecksContext) Err() error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.canceled {
		return context.Canceled
	}
	ctx.checks++
	if ctx.checks >= ctx.cancelAfter {
		ctx.canceled = true
		close(ctx.done)
		return context.Canceled
	}
	return nil
}

func (ctx *syncPruneWitnessCancelAfterChecksContext) Value(key any) any {
	return ctx.parent.Value(key)
}

func (ctx *syncPruneWitnessCancelAfterChecksContext) Checks() int {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.checks
}

func assertSyncPruneWitnessError(t *testing.T, err error, code SyncErrorCode, field string) {
	t.Helper()
	assertSyncErrorCode(t, err, code)
	var syncErr *SyncError
	if !errors.As(err, &syncErr) || syncErr.Field != field {
		t.Fatalf("sync error = %#v, want field %q", err, field)
	}
}
