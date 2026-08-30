package sqlite

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestRecoverySuccessorIntermediateAppendDefersCompleteAuditUntilReady(t *testing.T) {
	tests := []struct {
		name                       string
		corruptRetainedPredecessor bool
	}{
		{name: "old-successor-prefix"},
		{name: "old-predecessor-prefix", corruptRetainedPredecessor: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := stageBoundedRecoverySuccessorV1(t, "intermediate-"+test.name)
			corruptCandidateID := fixture.state.Successor.CandidateID
			if test.corruptRetainedPredecessor {
				corruptCandidateID = fixture.predecessor.CandidateID
			}
			corruptOldRecoveryCandidatePageV1(t, fixture.store, fixture.projectID, corruptCandidateID)

			thirdPage := syncAuthorityCandidatePageV2(
				fixture.secondPage.ThroughEnvironmentID, fixture.environments[8:12], true,
			)
			state, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
				context.Background(), fixture.projectID, fixture.state.Transition, fixture.state.Successor.Checkpoint(),
				fixture.start.SuccessorSnapshot, thirdPage,
			)
			if err != nil {
				t.Fatalf("bounded intermediate append error = %v", err)
			}
			if state.Successor.Ready || state.Successor.PageCount != 3 || state.Successor.EnvironmentCount != 12 ||
				state.Successor.ThroughEnvironmentID != thirdPage.ThroughEnvironmentID {
				t.Fatalf("bounded intermediate successor = %#v, want three-page staging checkpoint", state.Successor)
			}

			beforeFinal := syncAuthorityCandidatePersistedRowsV2(t, fixture.store, fixture.projectID)
			finalPage := syncAuthorityCandidatePageV2(thirdPage.ThroughEnvironmentID, fixture.environments[12:], false)
			if _, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
				context.Background(), fixture.projectID, state.Transition, state.Successor.Checkpoint(),
				fixture.start.SuccessorSnapshot, finalPage,
			); err == nil {
				t.Fatal("final append across corrupt prefix error = nil")
			} else {
				assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorStore)
			}
			afterFinal := syncAuthorityCandidatePersistedRowsV2(t, fixture.store, fixture.projectID)
			if !reflect.DeepEqual(afterFinal, beforeFinal) {
				t.Fatalf("candidate rows changed across refused final append\nbefore: %#v\nafter:  %#v", beforeFinal, afterFinal)
			}

			var persistedState string
			var pageCount, environmentCount int64
			var throughEnvironmentID string
			if err := fixture.store.db.QueryRow(`
SELECT state, page_count, environment_count, through_environment_id
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ?`,
				string(fixture.projectID), state.Successor.CandidateID[:],
			).Scan(&persistedState, &pageCount, &environmentCount, &throughEnvironmentID); err != nil {
				t.Fatalf("read successor checkpoint after final refusal: %v", err)
			}
			if persistedState != "staging" || pageCount != 3 || environmentCount != 12 || throughEnvironmentID != thirdPage.ThroughEnvironmentID {
				t.Fatalf("successor checkpoint after final refusal = {%q %d %d %q}, want staging third-page checkpoint",
					persistedState, pageCount, environmentCount, throughEnvironmentID)
			}
		})
	}
}

func TestRecoverySuccessorReadyReplayRequiresCompleteAudit(t *testing.T) {
	tests := []struct {
		name                       string
		corruptRetainedPredecessor bool
	}{
		{name: "old-successor-prefix"},
		{name: "old-predecessor-prefix", corruptRetainedPredecessor: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := stageBoundedRecoverySuccessorV1(t, "ready-replay-"+test.name)
			thirdPage := syncAuthorityCandidatePageV2(
				fixture.secondPage.ThroughEnvironmentID, fixture.environments[8:12], true,
			)
			state, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
				context.Background(), fixture.projectID, fixture.state.Transition, fixture.state.Successor.Checkpoint(),
				fixture.start.SuccessorSnapshot, thirdPage,
			)
			if err != nil {
				t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage(third) error = %v", err)
			}
			priorFinalCheckpoint := state.Successor.Checkpoint()
			finalPage := syncAuthorityCandidatePageV2(thirdPage.ThroughEnvironmentID, fixture.environments[12:], false)
			state, err = fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
				context.Background(), fixture.projectID, state.Transition, priorFinalCheckpoint,
				fixture.start.SuccessorSnapshot, finalPage,
			)
			if err != nil {
				t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage(final) error = %v", err)
			}
			if !state.Successor.Ready {
				t.Fatalf("final successor = %#v, want READY", state.Successor)
			}

			corruptCandidateID := state.Successor.CandidateID
			if test.corruptRetainedPredecessor {
				corruptCandidateID = fixture.predecessor.CandidateID
			}
			corruptOldRecoveryCandidatePageV1(t, fixture.store, fixture.projectID, corruptCandidateID)
			beforeReplay := syncAuthorityCandidatePersistedRowsV2(t, fixture.store, fixture.projectID)
			if _, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
				context.Background(), fixture.projectID, state.Transition, priorFinalCheckpoint,
				fixture.start.SuccessorSnapshot, finalPage,
			); err == nil {
				t.Fatal("ready replay across corrupt prefix error = nil")
			} else {
				assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorStore)
			}
			afterReplay := syncAuthorityCandidatePersistedRowsV2(t, fixture.store, fixture.projectID)
			if !reflect.DeepEqual(afterReplay, beforeReplay) {
				t.Fatalf("candidate rows changed across refused READY replay\nbefore: %#v\nafter:  %#v", beforeReplay, afterReplay)
			}
		})
	}
}

func TestRecoverySuccessorIntermediateAppendDefersHistoricalWatermarkSourceAuditUntilReady(t *testing.T) {
	fixture := stageBoundedRecoverySuccessorV1(t, "historical-watermark-source")
	insertEquivocatingPromotedRecoveryWatermarkSourceV1(t, fixture)

	thirdPage := syncAuthorityCandidatePageV2(
		fixture.secondPage.ThroughEnvironmentID, fixture.environments[8:12], true,
	)
	state, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, fixture.state.Transition, fixture.state.Successor.Checkpoint(),
		fixture.start.SuccessorSnapshot, thirdPage,
	)
	if err != nil {
		t.Fatalf("bounded intermediate append across historical source equivocation error = %v", err)
	}

	beforeFinal := syncAuthorityCandidatePersistedRowsV2(t, fixture.store, fixture.projectID)
	finalPage := syncAuthorityCandidatePageV2(thirdPage.ThroughEnvironmentID, fixture.environments[12:], false)
	if _, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, state.Transition, state.Successor.Checkpoint(),
		fixture.start.SuccessorSnapshot, finalPage,
	); err == nil {
		t.Fatal("final append across historical source equivocation error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorConflict)
	}
	afterFinal := syncAuthorityCandidatePersistedRowsV2(t, fixture.store, fixture.projectID)
	if !reflect.DeepEqual(afterFinal, beforeFinal) {
		t.Fatalf("candidate rows changed across historical-source refusal\nbefore: %#v\nafter:  %#v", beforeFinal, afterFinal)
	}
}

func TestRecoverySuccessorIntermediateAppendIgnoresDataPlaneHeadAndHonorsAuthorityFrontier(t *testing.T) {
	fixture := stageCanonicalBoundedRecoverySuccessorV1(t, "project-watermark-floor")
	firstCheckpoint := fixture.state.Successor.Checkpoint()
	secondPage := syncAuthorityCandidatePageV2(
		fixture.state.Successor.ThroughEnvironmentID, fixture.environments[4:8], true,
	)
	state, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, fixture.state.Transition, firstCheckpoint,
		fixture.start.SuccessorSnapshot, secondPage,
	)
	if err != nil {
		t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage(second) error = %v", err)
	}

	if _, err := fixture.store.StageSyncPage(
		context.Background(), fixture.projectID, fixture.start.SuccessorSnapshot.ChannelID, 0, 9, nil,
	); err != nil {
		t.Fatalf("StageSyncPage(newer project relay head) error = %v", err)
	}
	current, found, err := fixture.store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), fixture.projectID)
	if err != nil || !found || current != state {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(stale but valid) = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, state)
	}
	replayed, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, state.Transition, firstCheckpoint,
		fixture.start.SuccessorSnapshot, secondPage,
	)
	if err != nil || replayed != state {
		t.Fatalf("exact replay after project head advance = (%#v, %v), want (%#v, nil)", replayed, err, state)
	}

	thirdPage := syncAuthorityCandidatePageV2(secondPage.ThroughEnvironmentID, fixture.environments[8:12], true)
	advanced, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, state.Transition, state.Successor.Checkpoint(),
		fixture.start.SuccessorSnapshot, thirdPage,
	)
	if err != nil {
		t.Fatalf("new append after data-plane head advance error = %v", err)
	}

	authorityFrontier := syncRelayWatermarkFromSnapshot(
		fixture.projectID, fixture.start.SuccessorSnapshot, 9,
	)
	if got, err := fixture.store.AdvanceSyncRelayWatermark(context.Background(), authorityFrontier); err != nil || got != authorityFrontier {
		t.Fatalf("AdvanceSyncRelayWatermark(authority frontier) = (%#v, %v), want (%#v, nil)", got, err, authorityFrontier)
	}
	replayed, err = fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, advanced.Transition, state.Successor.Checkpoint(),
		fixture.start.SuccessorSnapshot, thirdPage,
	)
	if err != nil || replayed != advanced {
		t.Fatalf("exact replay after authority-frontier advance = (%#v, %v), want (%#v, nil)", replayed, err, advanced)
	}
	finalPage := syncAuthorityCandidatePageV2(thirdPage.ThroughEnvironmentID, fixture.environments[12:], false)
	if _, err := fixture.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), fixture.projectID, advanced.Transition, advanced.Successor.Checkpoint(),
		fixture.start.SuccessorSnapshot, finalPage,
	); err == nil {
		t.Fatal("new append below authority frontier error = nil")
	} else {
		assertSyncAuthorityRecoveryProblemCodeV1(t, err, SyncErrorCursor)
	}
	assertRawRecoverySuccessorCheckpointV1(t, fixture.store, fixture.projectID, advanced.Successor)
}

type boundedRecoverySuccessorFixtureV1 struct {
	store        *Store
	projectID    continuity.ProjectID
	predecessor  SyncAuthorityCandidate
	start        SyncAuthorityRecoveryTransitionStart
	environments []SyncEnvironmentCertificate
	state        SyncAuthorityRecoveryState
	secondPage   SyncAuthorityPage
}

func stageBoundedRecoverySuccessorV1(t *testing.T, name string) boundedRecoverySuccessorFixtureV1 {
	t.Helper()
	store := openSyncStore(t, "recovery-successor-bounded-append-"+name)
	projectID := continuity.ProjectID("project-recovery-successor-bounded-append-" + name)
	predecessorSnapshot, predecessorEnvironments, _, predecessor := stageReadySyncAuthorityCandidateV2(t, store, projectID, 8)
	writer := SyncEnvironmentCertificate{
		EnvironmentID:            string(store.WriterEnvironmentID()),
		CertificateID:            sha256.Sum256([]byte("bounded-append-local-writer")),
		CertificateBytes:         []byte("bounded append local writer certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 9,
	}
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(predecessorEnvironments)
	environments = append(environments, writer)
	for membership := uint32(10); membership <= 14; membership++ {
		environmentID := fmt.Sprintf("environment-z%04d", membership)
		environments = append(environments, SyncEnvironmentCertificate{
			EnvironmentID:            environmentID,
			CertificateID:            sha256.Sum256([]byte("bounded-append-certificate:" + environmentID)),
			CertificateBytes:         []byte("bounded append certificate bytes for " + environmentID),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: membership,
		})
	}
	sort.Slice(environments, func(left, right int) bool {
		return environments[left].EnvironmentID < environments[right].EnvironmentID
	})
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 9),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	start := syncAuthorityRecoveryStartV1(predecessor, writer, 9, 14)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	state, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	secondPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, environments[4:8], true)
	state, err = store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
		context.Background(), projectID, state.Transition, state.Successor.Checkpoint(), start.SuccessorSnapshot, secondPage,
	)
	if err != nil {
		t.Fatalf("AppendVerifiedSyncAuthorityRecoverySuccessorPage(second) error = %v", err)
	}
	return boundedRecoverySuccessorFixtureV1{
		store:        store,
		projectID:    projectID,
		predecessor:  predecessor,
		start:        start,
		environments: environments,
		state:        state,
		secondPage:   secondPage,
	}
}

func stageCanonicalBoundedRecoverySuccessorV1(t *testing.T, name string) boundedRecoverySuccessorFixtureV1 {
	t.Helper()
	store := openSyncStore(t, "recovery-successor-bounded-canonical-"+name)
	projectID := continuity.ProjectID("project-recovery-successor-bounded-canonical-" + name)
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
	predecessor, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, predecessorSnapshot,
		syncAuthorityCandidatePageV2("", cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments), false),
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(predecessor) error = %v", err)
	}
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 8),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	writer := SyncEnvironmentCertificate{
		EnvironmentID:            string(store.WriterEnvironmentID()),
		CertificateID:            sha256.Sum256([]byte("bounded-canonical-local-writer")),
		CertificateBytes:         []byte("bounded canonical local writer certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: canonical.MembershipGeneration + 1,
	}
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	environments = append(environments, writer)
	for membership := canonical.MembershipGeneration + 2; membership <= 14; membership++ {
		environmentID := fmt.Sprintf("environment-z%04d", membership)
		environments = append(environments, SyncEnvironmentCertificate{
			EnvironmentID:            environmentID,
			CertificateID:            sha256.Sum256([]byte("bounded-canonical-certificate:" + environmentID)),
			CertificateBytes:         []byte("bounded canonical certificate bytes for " + environmentID),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: membership,
		})
	}
	sort.Slice(environments, func(left, right int) bool {
		return environments[left].EnvironmentID < environments[right].EnvironmentID
	})
	start := syncAuthorityRecoveryStartV1(predecessor, writer, 8, 14)
	firstPage := syncAuthorityCandidatePageV2("", environments[:4], true)
	state, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition() error = %v", err)
	}
	return boundedRecoverySuccessorFixtureV1{
		store:        store,
		projectID:    projectID,
		predecessor:  predecessor,
		start:        start,
		environments: environments,
		state:        state,
	}
}

func corruptOldRecoveryCandidatePageV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	candidateID [32]byte,
) {
	t.Helper()
	if _, err := store.db.Exec(`
UPDATE continuity_sync_authority_candidate_environments
SET certificate_bytes = ?
WHERE project_id = ? AND candidate_id = ? AND page_number = 1 AND environment_ordinal = 1`,
		[]byte("corrupt old recovery candidate certificate"), string(projectID), candidateID[:],
	); err != nil {
		t.Fatalf("corrupt old candidate page: %v", err)
	}
}

func insertEquivocatingPromotedRecoveryWatermarkSourceV1(
	t *testing.T,
	fixture boundedRecoverySuccessorFixtureV1,
) {
	t.Helper()
	candidateID := sha256.Sum256([]byte("equivocating-promoted-recovery-watermark-source"))
	adminPublicKey := fixture.start.SuccessorSnapshot.AdminPublicKey
	adminPublicKey[0] ^= 0xff
	result, err := fixture.store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, role
)
SELECT
  project_id, ?, 'promoted', channel_id, relay_generation, ?,
  membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest, 'ordinary'
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ?`,
		candidateID[:], adminPublicKey[:], string(fixture.projectID), fixture.predecessor.CandidateID[:],
	)
	if err != nil {
		t.Fatalf("insert equivocating promoted watermark source: %v", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("insert equivocating promoted watermark source affected = %d, err = %v", affected, err)
	}
}

func assertRawRecoverySuccessorCheckpointV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	want SyncAuthorityCandidate,
) {
	t.Helper()
	var state string
	var pageCount, environmentCount int64
	var throughEnvironmentID string
	if err := store.db.QueryRow(`
SELECT state, page_count, environment_count, through_environment_id
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ?`, string(projectID), want.CandidateID[:]).Scan(
		&state, &pageCount, &environmentCount, &throughEnvironmentID,
	); err != nil {
		t.Fatalf("read raw recovery successor checkpoint: %v", err)
	}
	if state != "staging" || pageCount != want.PageCount || environmentCount != want.EnvironmentCount ||
		throughEnvironmentID != want.ThroughEnvironmentID {
		t.Fatalf("raw recovery successor checkpoint = {%q %d %d %q}, want staging checkpoint %#v",
			state, pageCount, environmentCount, throughEnvironmentID, want.Checkpoint())
	}
}
