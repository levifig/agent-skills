package sqlite

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestPromoteSyncAuthorityCandidateBootstrapsCanonicalAuthorityAndRetainsReceipt(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promotion-bootstrap")
	projectID := continuity.ProjectID("project-authority-candidate-promotion-bootstrap")
	snapshot, environments, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 5)

	receipt, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
	if err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
	}
	wantReceipt := SyncAuthorityCandidateReceipt{
		ProjectID:                projectID,
		CandidateID:              ready.CandidateID,
		Snapshot:                 snapshot,
		PageCount:                ready.PageCount,
		EnvironmentCount:         ready.EnvironmentCount,
		ThroughEnvironmentID:     ready.ThroughEnvironmentID,
		RollingEnvironmentDigest: ready.RollingEnvironmentDigest,
		AuthorityDigestVersion:   ready.AuthorityDigestVersion,
		AuthorityDigest:          ready.AuthorityDigest,
	}
	if receipt != wantReceipt {
		t.Fatalf("promotion receipt = %#v, want %#v", receipt, wantReceipt)
	}

	if _, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID); err != nil || found {
		t.Fatalf("CurrentSyncAuthorityCandidate(after promotion) = (_, %v, %v), want (_, false, nil)", found, err)
	}
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	if progress.ActivationState != SyncActivationStaging || progress.ChannelID != snapshot.ChannelID ||
		progress.DownloadedCursor != 0 || progress.AppliedCursor != 0 || progress.RelayHead != 0 {
		t.Fatalf("bootstrap progress = %#v", progress)
	}
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	wantAuthority := SyncAuthority{
		ChannelID:            snapshot.ChannelID,
		RelayGeneration:      snapshot.RelayGeneration,
		AdminPublicKey:       snapshot.AdminPublicKey,
		MembershipGeneration: snapshot.MembershipGeneration,
		InventoryArrivalHead: snapshot.InventoryArrivalHead,
		Environments:         environments,
	}
	if !reflect.DeepEqual(authority, wantAuthority) {
		t.Fatalf("canonical authority = %#v, want %#v", authority, wantAuthority)
	}
	binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
	}
	if binding != (SyncAuthorityBinding{
		ChannelID:              snapshot.ChannelID,
		RelayGeneration:        snapshot.RelayGeneration,
		AdminPublicKey:         snapshot.AdminPublicKey,
		MembershipGeneration:   snapshot.MembershipGeneration,
		InventoryArrivalHead:   snapshot.InventoryArrivalHead,
		AuthorityDigestVersion: ready.AuthorityDigestVersion,
		AuthorityDigest:        ready.AuthorityDigest,
	}) {
		t.Fatalf("canonical binding = %#v", binding)
	}

	var promoted, pages, candidateEnvironments, events int64
	if err := store.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidates WHERE project_id = ? AND candidate_id = ? AND state = 'promoted'),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages WHERE project_id = ? AND candidate_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments WHERE project_id = ? AND candidate_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events WHERE project_id = ? AND candidate_id = ?)`,
		string(projectID), ready.CandidateID[:], string(projectID), ready.CandidateID[:],
		string(projectID), ready.CandidateID[:], string(projectID), ready.CandidateID[:],
	).Scan(&promoted, &pages, &candidateEnvironments, &events); err != nil {
		t.Fatalf("read promoted candidate retention: %v", err)
	}
	if promoted != 1 || pages != 0 || candidateEnvironments != 0 || events != 0 {
		t.Fatalf("promoted retention = (%d, %d, %d, %d), want (1, 0, 0, 0)", promoted, pages, candidateEnvironments, events)
	}
}

func TestPromoteSyncAuthorityCandidateExactRetryUsesImmutableReceiptFirst(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promotion-retry")
	projectID := continuity.ProjectID("project-authority-candidate-promotion-retry")
	_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 3)
	want, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
	if err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
	}
	corruptDigest := sha256.Sum256([]byte("corrupt canonical digest after known promotion"))
	if _, err := store.db.Exec(`UPDATE continuity_sync_authorities SET authority_digest = ? WHERE project_id = ?`, corruptDigest[:], string(projectID)); err != nil {
		t.Fatalf("corrupt mutable canonical state: %v", err)
	}

	got, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
	if err != nil || got != want {
		t.Fatalf("exact lost-response retry = (%#v, %v), want (%#v, nil)", got, err, want)
	}
	tests := []struct {
		name   string
		code   SyncErrorCode
		mutate func(*SyncAuthorityCandidateCheckpoint)
	}{
		{name: "candidate id", code: SyncErrorConflict, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.CandidateID[0] ^= 0xff }},
		{name: "page count", code: SyncErrorConflict, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.PageCount++ }},
		{name: "environment count", code: SyncErrorConflict, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.EnvironmentCount++ }},
		{name: "through environment", code: SyncErrorConflict, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.ThroughEnvironmentID += ":changed" }},
		{name: "rolling digest", code: SyncErrorConflict, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.RollingEnvironmentDigest[0] ^= 0xff }},
		{name: "ready", code: SyncErrorInvalid, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.Ready = false }},
		{name: "authority digest", code: SyncErrorConflict, mutate: func(value *SyncAuthorityCandidateCheckpoint) { value.AuthorityDigest[0] ^= 0xff }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			altered := ready.Checkpoint()
			test.mutate(&altered)
			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, altered); err == nil {
				t.Fatal("altered retry error = nil")
			} else {
				assertSyncErrorCode(t, err, test.code)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidateRejectsCorruptPromotedReceiptWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Store, continuity.ProjectID, SyncAuthorityCandidate)
	}{
		{
			name: "cryptographic header identity",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ SyncAuthorityCandidate) {
				alteredChannel := sha256.Sum256([]byte("altered promoted receipt channel"))
				if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidates SET channel_id = ? WHERE project_id = ?`, alteredChannel[:], string(projectID)); err != nil {
					t.Fatalf("corrupt promoted header: %v", err)
				}
			},
		},
		{
			name: "infeasible page count",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate) {
				if _, err := store.db.Exec(`UPDATE continuity_sync_authority_candidates SET page_count = ? WHERE project_id = ?`, candidate.EnvironmentCount+1, string(projectID)); err != nil {
					t.Fatalf("corrupt promoted page count: %v", err)
				}
			},
		},
		{
			name: "retained child",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID, candidate SyncAuthorityCandidate) {
				pageDigest := sha256.Sum256([]byte("retained promoted receipt child page"))
				rollingDigest := sha256.Sum256([]byte("retained promoted receipt child rolling"))
				if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 1, NULL, ?, 1, 0, ?, 1, ?)`,
					string(projectID), candidate.CandidateID[:], candidate.ThroughEnvironmentID,
					pageDigest[:], rollingDigest[:],
				); err != nil {
					t.Fatalf("insert retained promoted child: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-candidate-promoted-corruption-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-candidate-promoted-corruption-" + syncSlug(test.name))
			_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 3)
			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
				t.Fatalf("promote fixture: %v", err)
			}
			test.mutate(t, store, projectID, ready)
			before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
				t.Fatal("corrupt promoted retry error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
			after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("corrupt promoted retry changed state:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidatePublicValidation(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promotion-validation")
	projectID := continuity.ProjectID("project-authority-candidate-promotion-validation")
	_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 1)
	checkpoint := ready.Checkpoint()

	var nilStore *Store
	if _, err := nilStore.PromoteSyncAuthorityCandidate(context.Background(), projectID, checkpoint); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate(nil store) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	if _, err := store.PromoteSyncAuthorityCandidate(nil, projectID, checkpoint); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate(nil context) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorInvalid)
	}
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), "invalid project id", checkpoint); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate(invalid project) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorInvalid)
	}
	staging := checkpoint
	staging.Ready = false
	staging.AuthorityDigest = [32]byte{}
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, staging); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate(staging checkpoint) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorInvalid)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PromoteSyncAuthorityCandidate(canceled, projectID, checkpoint); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate(canceled context) error = nil")
	} else if err != context.Canceled {
		t.Fatalf("canceled promotion error = %v, want context.Canceled", err)
	}
}

func TestPromoteSyncAuthorityCandidateAdvancesV1AuthorityWithoutChangingProgress(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promotion-advance")
	projectID := continuity.ProjectID("project-authority-candidate-promotion-advance")
	canonical := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, canonical); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	environmentAFirst := syncAuthorityPromotionMetadataV2("v1-progress-a-1", [32]byte{}, canonical.Environments[0].CertificateID)
	environmentASecond := syncAuthorityPromotionMetadataV2("v1-progress-a-2", environmentAFirst.digest, canonical.Environments[0].CertificateID)
	environmentAThird := syncAuthorityPromotionMetadataV2("v1-progress-a-3", environmentASecond.digest, canonical.Environments[0].CertificateID)
	environmentAThird.digest = canonical.Environments[0].Retirement.FinalEnvelopeDigest
	environmentBFirst := syncAuthorityPromotionMetadataV2("v1-progress-b-1", [32]byte{}, canonical.Environments[1].CertificateID)
	environmentBSecond := syncAuthorityPromotionMetadataV2("v1-progress-b-2", environmentBFirst.digest, canonical.Environments[1].CertificateID)
	protected := []struct {
		arrivalSequence     int64
		factID              string
		environmentID       string
		environmentSequence int64
		metadata            sealedEnvelopeMetadataV1
	}{
		{1, "fact-v1-progress-a-1", "environment-a", 1, environmentAFirst},
		{2, "fact-v1-progress-b-1", "environment-b", 1, environmentBFirst},
		{3, "fact-v1-progress-a-2", "environment-a", 2, environmentASecond},
		{4, "fact-v1-progress-b-2", "environment-b", 2, environmentBSecond},
		{5, "fact-v1-progress-a-3", "environment-a", 3, environmentAThird},
	}
	for _, row := range protected {
		insertSyncAuthorityPromotionReceiptV2(t, store, projectID, row.arrivalSequence, row.factID, row.environmentID, row.environmentSequence, row.metadata)
		insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, row.arrivalSequence, row.factID, row.environmentID, row.environmentSequence, row.metadata)
	}
	insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 3, environmentAThird)
	insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-b", 2, environmentBSecond)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET activation_state = 'attached', downloaded_cursor = 7, applied_cursor = 5, relay_head = 9
WHERE project_id = ?`, string(projectID)); err != nil {
		t.Fatalf("seed attached progress: %v", err)
	}
	baseDigest, err := frozenSyncAuthorityDigestV1(projectID, canonical)
	if err != nil {
		t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
	}
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	environments = append(environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-c",
		CertificateID:            sha256.Sum256([]byte("authority-promotion-environment-c")),
		CertificateBytes:         []byte("authority promotion environment c certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 4,
	})
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, 1, baseDigest)
	snapshot.MembershipGeneration = 4
	snapshot.InventoryArrivalHead = 1
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("stage advancing authority candidate: %v", err)
	}
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
	}
	progress, err := store.CurrentSyncProgress(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncProgress() error = %v", err)
	}
	if progress.ActivationState != SyncActivationAttached || progress.DownloadedCursor != 7 ||
		progress.AppliedCursor != 5 || progress.RelayHead != 9 {
		t.Fatalf("progress changed across authority promotion: %#v", progress)
	}
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	if authority.MembershipGeneration != 4 || authority.InventoryArrivalHead != 1 || !reflect.DeepEqual(authority.Environments, environments) {
		t.Fatalf("advanced authority = %#v", authority)
	}
}

func TestPromoteSyncAuthorityCandidateRejectsV1CanonicalInventoryBeyondFixedBoundWithoutMutation(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promotion-v1-fixed-bound")
	projectID := continuity.ProjectID("project-authority-candidate-promotion-v1-fixed-bound")
	environments := syncAuthorityCandidateManyEnvironmentsV2(maximumSyncAuthorityEnvironments)
	authority := SyncAuthority{
		ChannelID:            testSyncChannelID("authority-candidate-promotion-v1-fixed-bound-channel"),
		RelayGeneration:      sha256.Sum256([]byte("authority-candidate-promotion-v1-fixed-bound-relay")),
		AdminPublicKey:       sha256.Sum256([]byte("authority-candidate-promotion-v1-fixed-bound-admin")),
		MembershipGeneration: maximumSyncAuthorityEnvironments,
		Environments:         environments,
	}
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, binding.AuthorityDigestVersion, binding.AuthorityDigest)
	ready := stageSyncAuthorityCandidateInventoryV2(t, store, projectID, snapshot, environments)

	extra := SyncEnvironmentCertificate{
		EnvironmentID:            "environment:0257",
		CertificateID:            sha256.Sum256([]byte("authority-candidate-promotion-v1-fixed-bound-extra")),
		CertificateBytes:         []byte("authority candidate promotion v1 fixed bound extra certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 1,
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_certificates(
  project_id, environment_id, certificate_id, certificate_bytes, mode,
  expires_at_millis, join_membership_generation
) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), extra.EnvironmentID, extra.CertificateID[:], extra.CertificateBytes,
		string(extra.Mode), extra.ExpiresAtMillis, extra.JoinMembershipGeneration,
	); err != nil {
		t.Fatalf("insert 257th canonical environment: %v", err)
	}
	before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate(257 v1 environments) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
		problem, ok := err.(*SyncError)
		if !ok || problem.Detail != "pinned v1 authority inventory exceeds the fixed bound" {
			t.Fatalf("257-environment promotion error = %v, want fixed-bound audit", err)
		}
	}
	after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed v1 fixed-bound promotion changed state:\n got %#v\nwant %#v", after, before)
	}
}

func TestPromoteSyncAuthorityCandidateAdvancesV2WithAppendOnlyCanonicalDelta(t *testing.T) {
	store := openSyncStore(t, "authority-candidate-promotion-v2-delta")
	projectID := continuity.ProjectID("project-authority-candidate-promotion-v2-delta")
	_, _, _, bootstrap := stageReadySyncAuthorityCandidateV2(t, store, projectID, 3)
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, bootstrap.Checkpoint()); err != nil {
		t.Fatalf("promote v2 bootstrap: %v", err)
	}
	canonical, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	before := terminalLogicalRowsV1(t, store, projectID)

	if _, err := store.db.Exec(`
CREATE TEMP TABLE authority_promotion_canonical_mutations(
  mutation TEXT NOT NULL,
  environment_id TEXT NOT NULL
) STRICT;
CREATE TEMP TRIGGER authority_promotion_reject_environment_delete
BEFORE DELETE ON continuity_sync_environment_certificates
BEGIN
  SELECT RAISE(ABORT, 'canonical environment delete');
END;
CREATE TEMP TRIGGER authority_promotion_reject_non_delta_update
BEFORE UPDATE ON continuity_sync_environment_certificates
WHEN OLD.environment_id <> 'environment:0002'
  OR OLD.project_id IS NOT NEW.project_id
  OR OLD.environment_id IS NOT NEW.environment_id
  OR OLD.certificate_id IS NOT NEW.certificate_id
  OR OLD.certificate_bytes IS NOT NEW.certificate_bytes
  OR OLD.mode IS NOT NEW.mode
  OR OLD.expires_at_millis IS NOT NEW.expires_at_millis
  OR OLD.join_membership_generation IS NOT NEW.join_membership_generation
  OR OLD.retirement_id IS NOT NULL
  OR NEW.retirement_id IS NULL
BEGIN
  SELECT RAISE(ABORT, 'non-append canonical environment update');
END;
CREATE TEMP TRIGGER authority_promotion_record_environment_update
AFTER UPDATE ON continuity_sync_environment_certificates
BEGIN
  INSERT INTO authority_promotion_canonical_mutations VALUES('update', NEW.environment_id);
END;
CREATE TEMP TRIGGER authority_promotion_record_environment_insert
AFTER INSERT ON continuity_sync_environment_certificates
BEGIN
  INSERT INTO authority_promotion_canonical_mutations VALUES('insert', NEW.environment_id);
END;`); err != nil {
		t.Fatalf("install canonical delta audit triggers: %v", err)
	}

	environments := cloneSyncAuthorityCandidateEnvironmentsV2(canonical.Environments)
	environments[1].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          canonical.RelayGeneration,
		MembershipGeneration:     4,
		FinalEnvironmentSequence: 0,
		RetirementID:             sha256.Sum256([]byte("authority-promotion-v2-delta-retirement")),
		RetirementBytes:          []byte("authority promotion v2 delta retirement"),
	}
	environments = append(environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment:0004",
		CertificateID:            sha256.Sum256([]byte("authority-promotion-v2-delta-environment-4")),
		CertificateBytes:         []byte("authority promotion v2 delta environment 4"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 5,
	})
	snapshot := syncAuthoritySnapshotFromAuthorityV2(canonical, binding.AuthorityDigestVersion, binding.AuthorityDigest)
	snapshot.MembershipGeneration = 5
	snapshot.InventoryArrivalHead++
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("stage v2 advancement: %v", err)
	}
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("promote v2 advancement: %v", err)
	}

	rows, err := store.db.Query(`SELECT mutation, environment_id FROM authority_promotion_canonical_mutations ORDER BY mutation, environment_id`)
	if err != nil {
		t.Fatalf("read canonical delta audit: %v", err)
	}
	var mutations []string
	for rows.Next() {
		var mutation, environmentID string
		if err := rows.Scan(&mutation, &environmentID); err != nil {
			rows.Close()
			t.Fatalf("scan canonical delta audit: %v", err)
		}
		mutations = append(mutations, mutation+":"+environmentID)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close canonical delta audit: %v", err)
	}
	if !reflect.DeepEqual(mutations, []string{"insert:environment:0004", "update:environment:0002"}) {
		t.Fatalf("canonical environment mutations = %#v", mutations)
	}
	advanced, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil || !reflect.DeepEqual(advanced.Environments, environments) {
		t.Fatalf("advanced v2 authority = (%#v, %v), want exact delta inventory", advanced, err)
	}
	after := terminalLogicalRowsV1(t, store, projectID)
	for _, environmentID := range []string{"environment:0001", "environment:0003"} {
		if syncAuthorityPromotionCanonicalEnvironmentRowV2(before, environmentID) != syncAuthorityPromotionCanonicalEnvironmentRowV2(after, environmentID) {
			t.Fatalf("unchanged canonical row %s was rewritten", environmentID)
		}
	}
}

func TestPromoteSyncAuthorityCandidatePreservesExactSealedOutboxAtRetirementFence(t *testing.T) {
	store, projectID, outbox := syncAuthorityPromotionLocalOutboxFixtureV2(t, "retirement-outbox-success")
	ready := stageSyncAuthorityRetirementCandidateV2(t, store, projectID, 1, outbox.EnvelopeDigest)
	before := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID)

	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
	}
	if after := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID); !reflect.DeepEqual(after, before) {
		t.Fatalf("data plane changed across retirement promotion:\n got %#v\nwant %#v", after, before)
	}
	pending, err := store.PendingSealedOutbox(context.Background(), projectID, 16)
	if err != nil || len(pending) != 1 || !sealedOutboxFrameEqual(pending[0], outbox) {
		t.Fatalf("retained outbox = (%#v, %v), want exact %#v", pending, err, outbox)
	}
}

func TestPromoteSyncAuthorityCandidateRejectsCorruptSealedOutboxBytesWithoutMutation(t *testing.T) {
	store, projectID, outbox := syncAuthorityPromotionLocalOutboxFixtureV2(t, "corrupt-outbox-bytes")
	ready := stageSyncAuthorityRetirementCandidateV2(t, store, projectID, 1, outbox.EnvelopeDigest)
	if _, err := store.db.Exec(`
UPDATE continuity_sync_outbox
SET sealed_envelope = ?
WHERE project_id = ?`, []byte("schema-valid but digest-mismatched bytes"), string(projectID)); err != nil {
		t.Fatalf("corrupt outbox bytes: %v", err)
	}
	before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate() error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed promotion changed state:\n got %#v\nwant %#v", after, before)
	}
}

func TestPromoteSyncAuthorityCandidateRejectsOrphanProtectedReceiptWithoutMutation(t *testing.T) {
	store, projectID, outbox := syncAuthorityPromotionLocalOutboxFixtureV2(t, "orphan-protected-receipt")
	ready := stageSyncAuthorityRetirementCandidateV2(t, store, projectID, 1, outbox.EnvelopeDigest)
	digest := sha256.Sum256([]byte("orphan protected receipt digest"))
	certificateID := testSyncCertificateID("environment-a")
	nonce := testNonce("authority-promotion-orphan-protected-receipt")
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_receipts(
  project_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
) VALUES(?, 2, 'fact-orphan-protected-receipt', 'environment-a', 1,
  zeroblob(32), ?, ?, 1, ?)`, string(projectID), digest[:], certificateID[:], nonce[:]); err != nil {
		t.Fatalf("insert orphan protected receipt: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_heads(
  project_id, environment_id, highest_sequence, hlc_wall_millis, hlc_logical,
  sealed_sequence, previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce
) VALUES(?, 'environment-a', 1, 1, 0, 1, zeroblob(32), ?, ?, 1, ?)`,
		string(projectID), digest[:], certificateID[:], nonce[:]); err != nil {
		t.Fatalf("insert exact orphan receipt head: %v", err)
	}
	before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
		t.Fatal("PromoteSyncAuthorityCandidate() error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed promotion changed state:\n got %#v\nwant %#v", after, before)
	}
}

func TestPromoteSyncAuthorityCandidateRejectsProtectedRetirementConflictsWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		stage  func(*testing.T, *Store, continuity.ProjectID, SealedOutboxFrame) SyncAuthorityCandidate
		mutate func(*testing.T, *Store, continuity.ProjectID)
	}{
		{
			name: "retained source above final",
			stage: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ SealedOutboxFrame) SyncAuthorityCandidate {
				return stageSyncAuthorityRetirementCandidateV2(t, store, projectID, 0, [32]byte{})
			},
		},
		{
			name: "wrong final digest",
			stage: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ SealedOutboxFrame) SyncAuthorityCandidate {
				return stageSyncAuthorityRetirementCandidateV2(t, store, projectID, 1, sha256.Sum256([]byte("wrong retirement final digest")))
			},
		},
		{
			name: "protected certificate mismatch",
			stage: func(t *testing.T, store *Store, projectID continuity.ProjectID, outbox SealedOutboxFrame) SyncAuthorityCandidate {
				return stageSyncAuthorityRetirementCandidateV2(t, store, projectID, 1, outbox.EnvelopeDigest)
			},
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				wrongCertificate := sha256.Sum256([]byte("different protected certificate"))
				if _, err := store.db.Exec(`UPDATE continuity_sync_outbox SET certificate_id = ? WHERE project_id = ?`, wrongCertificate[:], string(projectID)); err != nil {
					t.Fatalf("corrupt outbox certificate: %v", err)
				}
				if _, err := store.db.Exec(`UPDATE continuity_sync_environment_heads SET certificate_id = ? WHERE project_id = ?`, wrongCertificate[:], string(projectID)); err != nil {
					t.Fatalf("corrupt head certificate: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, outbox := syncAuthorityPromotionLocalOutboxFixtureV2(t, syncSlug(test.name))
			ready := test.stage(t, store, projectID, outbox)
			if test.mutate != nil {
				test.mutate(t, store, projectID)
			}
			before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
				t.Fatal("PromoteSyncAuthorityCandidate() error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorConflict)
			}
			after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("failed promotion changed state:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidateRejectsUnsealedAndOmittedLocalEnvironments(t *testing.T) {
	t.Run("unsealed fact under retirement", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "authority-promotion-unsealed-retirement")
		snapshot := SyncAuthoritySnapshot{
			ChannelID:            testSyncChannelID("authority-promotion-unsealed-channel"),
			RelayGeneration:      sha256.Sum256([]byte("authority-promotion-unsealed-relay")),
			AdminPublicKey:       sha256.Sum256([]byte("authority-promotion-unsealed-admin")),
			MembershipGeneration: 2,
			InventoryArrivalHead: 1,
		}
		environment := SyncEnvironmentCertificate{
			EnvironmentID:            "environment-local",
			CertificateID:            sha256.Sum256([]byte("authority-promotion-unsealed-certificate")),
			CertificateBytes:         []byte("authority promotion unsealed certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: 1,
			Retirement: &SyncEnvironmentRetirement{
				RelayGeneration:          snapshot.RelayGeneration,
				MembershipGeneration:     2,
				FinalEnvironmentSequence: 1,
				FinalEnvelopeDigest:      sha256.Sum256([]byte("unprovable final digest")),
				RetirementID:             sha256.Sum256([]byte("authority-promotion-unsealed-retirement")),
				RetirementBytes:          []byte("authority promotion unsealed retirement"),
			},
		}
		ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", []SyncEnvironmentCertificate{environment}, false),
		)
		if err != nil {
			t.Fatalf("stage retired bootstrap candidate: %v", err)
		}
		before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
		if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
			t.Fatal("unsealed retirement promotion error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("unsealed failure changed state:\n got %#v\nwant %#v", after, before)
		}
	})

	t.Run("omitted local environment", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "authority-promotion-omitted-local")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(1)
		environment := syncAuthorityCandidateManyEnvironmentsV2(1)[0]
		ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", []SyncEnvironmentCertificate{environment}, false),
		)
		if err != nil {
			t.Fatalf("stage omitting bootstrap candidate: %v", err)
		}
		if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
			t.Fatal("omitted local environment promotion error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
	})
}

func TestPromoteSyncAuthorityCandidateBootstrapRejectsForeignKeyDisabledOrphansWithoutMutation(t *testing.T) {
	for _, orphan := range []string{
		"inbox",
		"receipt",
		"outbox",
		"tombstone",
		"terminal header",
		"terminal frame",
		"promoted authority header",
		"authority page",
		"authority environment",
		"authority membership event",
	} {
		t.Run(orphan, func(t *testing.T) {
			store := openSyncStore(t, "authority-promotion-bootstrap-orphan-"+syncSlug(orphan))
			projectID := continuity.ProjectID("project-authority-promotion-bootstrap-orphan-" + syncSlug(orphan))
			_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 2)
			if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				t.Fatalf("disable foreign keys: %v", err)
			}
			insertSyncAuthorityPromotionBootstrapOrphanV2(t, store, projectID, orphan)
			before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
				t.Fatal("bootstrap promotion with orphan error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
			}
			after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("orphan refusal changed state:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidateLeavesActiveTerminalCandidateStaleAndUntouched(t *testing.T) {
	_, store, projectID, authority, frames := terminalCandidateMixedFixtureV1(t, "authority-promotion-terminal-stale", 2)
	terminalCandidate, err := store.StageVerifiedTerminalCandidateChunk(
		context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID), frames, 1_000, 100,
	)
	if err != nil {
		t.Fatalf("StageVerifiedTerminalCandidateChunk() error = %v", err)
	}
	baseBinding := currentSyncAuthorityBindingForTest(t, store, projectID)
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(authority.Environments)
	nextMembership := authority.MembershipGeneration + 1
	environments = append(environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-z",
		CertificateID:            sha256.Sum256([]byte("authority-promotion-terminal-stale-z")),
		CertificateBytes:         []byte("authority promotion terminal stale environment z"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: nextMembership,
	})
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, baseBinding.AuthorityDigestVersion, baseBinding.AuthorityDigest)
	snapshot.MembershipGeneration = nextMembership
	snapshot.InventoryArrivalHead = 1
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("stage authority advancement over terminal candidate: %v", err)
	}
	before := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID)
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate() error = %v", err)
	}
	if after := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID); !reflect.DeepEqual(after, before) {
		t.Fatalf("terminal data plane changed:\n got %#v\nwant %#v", after, before)
	}
	current, found, err := store.CurrentTerminalCandidate(context.Background(), projectID)
	if err != nil || !found || current != terminalCandidate {
		t.Fatalf("CurrentTerminalCandidate(after authority advance) = (%#v, %v, %v), want unchanged", current, found, err)
	}
	if _, err := store.PromoteTerminalCandidate(context.Background(), projectID, terminalCandidateCheckpointV1(terminalCandidate)); err == nil {
		t.Fatal("stale terminal candidate promotion error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestPromoteSyncAuthorityCandidateConcurrentExactCallersAndReopen(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "authority-candidate-promotion-concurrent")
	stores := openSyncAuthorityCandidateStoresV2(t, stateRoot)
	projectID := continuity.ProjectID("project-authority-candidate-promotion-concurrent")
	_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, stores[0], projectID, 9)
	type result struct {
		receipt SyncAuthorityCandidateReceipt
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
			results[index].receipt, results[index].err = stores[index].PromoteSyncAuthorityCandidate(
				context.Background(), projectID, ready.Checkpoint(),
			)
		}(index)
	}
	close(start)
	group.Wait()
	if results[0].err != nil || results[1].err != nil || results[0].receipt != results[1].receipt {
		t.Fatalf("concurrent exact promotion results = %#v", results)
	}

	reopened, err := Open(stateRoot, "environment-local")
	if err != nil {
		t.Fatalf("Open(retry) error = %v", err)
	}
	t.Cleanup(func() { reopened.Close() })
	retried, err := reopened.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
	if err != nil || retried != results[0].receipt {
		t.Fatalf("reopened exact retry = (%#v, %v), want (%#v, nil)", retried, err, results[0].receipt)
	}
}

func TestAuthorityCandidateCrossAPIGuards(t *testing.T) {
	t.Run("compatibility exact retry bypasses active candidate without mutation", func(t *testing.T) {
		store := openSyncStore(t, "authority-promotion-install-exact-active")
		projectID := continuity.ProjectID("project-authority-promotion-install-exact-active")
		authority := testActiveSyncAuthority()
		wantProgress, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority)
		if err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority(initial) error = %v", err)
		}
		candidate := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
		before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
		for _, trigger := range []string{
			`CREATE TRIGGER reject_exact_install_authority_write
BEFORE UPDATE ON continuity_sync_authorities
BEGIN SELECT RAISE(ABORT, 'exact install wrote authority metadata'); END`,
			`CREATE TRIGGER reject_exact_install_project_write
BEFORE UPDATE ON continuity_sync_projects
BEGIN SELECT RAISE(ABORT, 'exact install wrote project metadata'); END`,
			`CREATE TRIGGER reject_exact_install_environment_write
BEFORE UPDATE ON continuity_sync_environment_certificates
BEGIN SELECT RAISE(ABORT, 'exact install wrote environment metadata'); END`,
		} {
			if _, err := store.db.Exec(trigger); err != nil {
				t.Fatalf("install exact-retry no-write trigger: %v", err)
			}
		}

		gotProgress, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority)
		if err != nil || gotProgress != wantProgress {
			t.Fatalf("InstallVerifiedSyncAuthority(exact with active candidate) = (%#v, %v), want (%#v, nil)", gotProgress, err, wantProgress)
		}
		after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("exact compatibility retry changed state:\n got %#v\nwant %#v", after, before)
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("candidate after exact compatibility retry = (%#v, %v, %v), want unchanged", current, found, err)
		}

		different := cloneSyncAuthority(authority)
		different.MembershipGeneration++
		different.Environments = append(different.Environments, SyncEnvironmentCertificate{
			EnvironmentID:            "environment-z",
			CertificateID:            testSyncCertificateID("environment-z"),
			CertificateBytes:         []byte("environment-z-certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: different.MembershipGeneration,
		})
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, different); err == nil {
			t.Fatal("InstallVerifiedSyncAuthority(different with active candidate) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		if changed := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...); !reflect.DeepEqual(changed, before) {
			t.Fatalf("rejected compatibility install changed state:\n got %#v\nwant %#v", changed, before)
		}
	})

	t.Run("compatibility install rejects active candidate", func(t *testing.T) {
		store := openSyncStore(t, "authority-promotion-install-guard")
		projectID := continuity.ProjectID("project-authority-promotion-install-guard")
		_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 3)
		authority := testActiveSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err == nil {
			t.Fatal("InstallVerifiedSyncAuthority(active candidate) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != ready {
			t.Fatalf("candidate changed after rejected install: (%#v, %v, %v)", current, found, err)
		}
	})

	t.Run("compatibility install rejects staging candidate", func(t *testing.T) {
		store := openSyncStore(t, "authority-promotion-install-staging-guard")
		projectID := continuity.ProjectID("project-authority-promotion-install-staging-guard")
		snapshot := syncAuthorityCandidateBootstrapSnapshotV2(3)
		staging, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, snapshot,
			syncAuthorityCandidatePageV2("", syncAuthorityCandidateManyEnvironmentsV2(1), true),
		)
		if err != nil {
			t.Fatalf("stage authority candidate: %v", err)
		}
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testActiveSyncAuthority()); err == nil {
			t.Fatal("InstallVerifiedSyncAuthority(staging candidate) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != staging {
			t.Fatalf("staging candidate changed after rejected install: (%#v, %v, %v)", current, found, err)
		}
	})

	t.Run("activation rejects active but permits promoted receipt", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "authority-promotion-activation-guard")
		authority := testActiveSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		baseDigest, err := frozenSyncAuthorityDigestV1(projectID, authority)
		if err != nil {
			t.Fatalf("derive base digest: %v", err)
		}
		snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, 1, baseDigest)
		ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
			context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", authority.Environments, false),
		)
		if err != nil {
			t.Fatalf("stage authority candidate: %v", err)
		}
		if _, err := store.ActivateStagedSync(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID)); err == nil {
			t.Fatal("ActivateStagedSync(active authority candidate) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
			t.Fatalf("promote authority candidate: %v", err)
		}
		if _, err := store.ActivateStagedSync(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID)); err != nil {
			t.Fatalf("ActivateStagedSync(promoted receipt) error = %v", err)
		}
	})

	t.Run("attached activation exact retry bypasses active candidate without mutation", func(t *testing.T) {
		store, projectID := storeWithLocalRoot(t, "authority-promotion-attached-exact-active")
		authority := testActiveSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		binding := currentSyncAuthorityBindingForTest(t, store, projectID)
		wantProgress, err := store.ActivateStagedSync(context.Background(), projectID, binding)
		if err != nil {
			t.Fatalf("ActivateStagedSync(initial) error = %v", err)
		}
		candidate := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
		before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

		gotProgress, err := store.ActivateStagedSync(context.Background(), projectID, binding)
		if err != nil || gotProgress != wantProgress {
			t.Fatalf("ActivateStagedSync(attached with active candidate) = (%#v, %v), want (%#v, nil)", gotProgress, err, wantProgress)
		}
		after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("attached activation retry changed state:\n got %#v\nwant %#v", after, before)
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != candidate {
			t.Fatalf("candidate after attached activation retry = (%#v, %v, %v), want unchanged", current, found, err)
		}
	})

	t.Run("activation rejects staging candidate", func(t *testing.T) {
		store := openSyncStore(t, "authority-promotion-activation-staging-guard")
		projectID := continuity.ProjectID("project-authority-promotion-activation-staging-guard")
		authority := testActiveSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		staging := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, false)
		if _, err := store.ActivateStagedSync(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID)); err == nil {
			t.Fatal("ActivateStagedSync(staging authority candidate) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
		if err != nil || !found || current != staging {
			t.Fatalf("staging candidate changed after rejected activation: (%#v, %v, %v)", current, found, err)
		}
	})

	for _, ready := range []bool{false, true} {
		state := "staging"
		if ready {
			state = "ready"
		}
		t.Run("staged discard rejects "+state+" candidate", func(t *testing.T) {
			store := openSyncStore(t, "authority-promotion-discard-"+state+"-guard")
			projectID := continuity.ProjectID("project-authority-promotion-discard-" + state + "-guard")
			authority := testActiveSyncAuthority()
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
				t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
			}
			candidate := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, ready)
			if err := store.DiscardStagedSync(context.Background(), projectID, authority.ChannelID); err == nil {
				t.Fatalf("DiscardStagedSync(%s candidate) error = nil", state)
			} else {
				assertSyncErrorCode(t, err, SyncErrorConflict)
			}
			current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
			if err != nil || !found || current != candidate {
				t.Fatalf("%s candidate changed after rejected discard: (%#v, %v, %v)", state, current, found, err)
			}
		})
	}

	t.Run("staged discard rejects permanent receipt", func(t *testing.T) {
		store := openSyncStore(t, "authority-promotion-discard-guard")
		projectID := continuity.ProjectID("project-authority-promotion-discard-guard")
		_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 2)
		receipt, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
		if err != nil {
			t.Fatalf("promote bootstrap candidate: %v", err)
		}
		if err := store.DiscardStagedSync(context.Background(), projectID, ready.Snapshot.ChannelID); err == nil {
			t.Fatal("DiscardStagedSync(promoted receipt) error = nil")
		} else {
			assertSyncErrorCode(t, err, SyncErrorConflict)
		}
		got, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint())
		if err != nil || got != receipt {
			t.Fatalf("receipt after rejected discard = (%#v, %v), want (%#v, nil)", got, err, receipt)
		}
	})
}

func TestPromoteSyncAuthorityCandidateStreamsFourThousandNinetySevenEnvironments(t *testing.T) {
	store := openSyncStore(t, "authority-promotion-4097")
	projectID := continuity.ProjectID("project-authority-promotion-4097")
	_, _, _, ready := stageReadySyncAuthorityCandidateV2(t, store, projectID, 4_097)
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate(4097) error = %v", err)
	}
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(4097) error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, binding.AuthorityDigestVersion, binding.AuthorityDigest)
	ready = stageSyncAuthorityCandidateInventoryV2(t, store, projectID, snapshot, authority.Environments)
	assertSyncAuthorityPromotionCompositePlansV2(t, store, projectID, ready)
	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate(4097 v2 canonical revalidation) error = %v", err)
	}
	var environments int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_environment_certificates WHERE project_id = ?`, string(projectID)).Scan(&environments); err != nil {
		t.Fatalf("count promoted environments: %v", err)
	}
	if environments != 4_097 {
		t.Fatalf("promoted environment count = %d, want 4097", environments)
	}
}

func TestPromoteSyncAuthorityCandidatePreservesExactReceiptTombstoneDuplicate(t *testing.T) {
	store, projectID, authority := syncAuthorityPromotionProtectedHistoryFixtureV2(t, "exact-pruned-duplicate")
	metadata := syncAuthorityPromotionMetadataV2("exact-pruned-duplicate", [32]byte{}, authority.Environments[0].CertificateID)
	insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-pruned-duplicate", "environment-a", 1, metadata)
	insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-pruned-duplicate", "environment-a", 1, metadata)
	insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
	setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
	ready := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
	before := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID)

	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate(exact receipt+tombstone) error = %v", err)
	}
	if after := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID); !reflect.DeepEqual(after, before) {
		t.Fatalf("exact pruned duplicate changed data plane:\n got %#v\nwant %#v", after, before)
	}
}

func TestPromoteSyncAuthorityCandidatePreservesValidProtectedHLCBoundaries(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *Store, continuity.ProjectID, SyncAuthority)
	}{
		{
			name: "pruned sealed predecessor has no clock to compare",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("pruned-hlc-boundary", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-pruned-hlc-boundary", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-pruned-hlc-boundary", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-live-after-pruned-boundary", "environment-a", 2)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET highest_sequence = 2
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("extend pruned-boundary head: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "retained live sealed predecessor increases into suffix",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("live-hlc-boundary", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-live-hlc-boundary-sealed", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-live-hlc-boundary-sealed", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-live-hlc-boundary-unsealed", "environment-a", 2)
				if _, err := store.db.Exec(`
UPDATE continuity_facts
SET hlc_wall_millis = 2
WHERE project_id = ? AND fact_id = 'fact-live-hlc-boundary-unsealed'`, string(projectID)); err != nil {
					t.Fatalf("advance live-boundary fact clock: %v", err)
				}
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET highest_sequence = 2, hlc_wall_millis = 2
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("advance live-boundary head: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "retained live predecessor increases into pruned final head",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				first := syncAuthorityPromotionMetadataV2("live-before-pruned-final-first", [32]byte{}, authority.Environments[0].CertificateID)
				second := syncAuthorityPromotionMetadataV2("live-before-pruned-final-second", first.digest, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-live-before-pruned-final-first", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-live-before-pruned-final-first", "environment-a", 1, first)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 2, "fact-live-before-pruned-final-second", "environment-a", 2, second)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-live-before-pruned-final-second", "environment-a", 2, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, second)
				if _, err := store.db.Exec(`
UPDATE continuity_facts
SET hlc_wall_millis = 10
WHERE project_id = ? AND fact_id = 'fact-live-before-pruned-final-first'`, string(projectID)); err != nil {
					t.Fatalf("advance live-before-pruned-final fact clock: %v", err)
				}
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET hlc_wall_millis = 20
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("advance live-before-pruned-final head clock: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, authority := syncAuthorityPromotionProtectedHistoryFixtureV2(t, syncSlug(test.name))
			test.seed(t, store, projectID, authority)
			ready := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
			before := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID)

			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
				t.Fatalf("PromoteSyncAuthorityCandidate(valid HLC boundary) error = %v", err)
			}
			if after := syncAuthorityPromotionDataPlaneRowsV2(t, store, projectID); !reflect.DeepEqual(after, before) {
				t.Fatalf("valid HLC boundary changed data plane:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidateRejectsCrossCoordinateFactIdentityWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *Store, continuity.ProjectID, SyncAuthority) (int64, [32]byte)
	}{
		{
			name: "environment changes",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) (int64, [32]byte) {
				metadata := syncAuthorityPromotionMetadataV2("cross-environment-fact", [32]byte{}, authority.Environments[1].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-cross-coordinate", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-cross-coordinate", "environment-b", 1, metadata)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-cross-coordinate", "environment-b", 1, metadata)
				insertSyncAuthorityPromotionUnsealedHeadV2(t, store, projectID, "environment-a", 1, 1)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-b", 1, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
				return 0, [32]byte{}
			},
		},
		{
			name: "environment sequence changes",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) (int64, [32]byte) {
				metadata := syncAuthorityPromotionMetadataV2("cross-sequence-fact", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-cross-coordinate", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-cross-coordinate", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-cross-coordinate", "environment-a", 2)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET highest_sequence = 2
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("extend cross-sequence head: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
				return 1, metadata.digest
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, authority := syncAuthorityPromotionProtectedHistoryFixtureV2(t, "cross-coordinate-"+syncSlug(test.name))
			if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
				t.Fatalf("disable foreign keys: %v", err)
			}
			finalSequence, finalDigest := test.seed(t, store, projectID, authority)
			ready := stageSyncAuthorityRetirementCandidateForEnvironmentV2(
				t, store, projectID, "environment-a", finalSequence, finalDigest,
			)
			before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
				t.Fatal("PromoteSyncAuthorityCandidate(cross-coordinate fact) error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
				problem, ok := err.(*SyncError)
				if !ok || problem.Detail != "persisted fact identity changes environment coordinates" {
					t.Fatalf("cross-coordinate fact error = %v, want fact-identity audit", err)
				}
			}
			after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("cross-coordinate fact refusal changed state:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidateRejectsCorruptProtectedHistoryWithoutMutation(t *testing.T) {
	tests := []struct {
		name       string
		wantDetail string
		seed       func(*testing.T, *Store, continuity.ProjectID, SyncAuthority)
	}{
		{
			name:       "schema valid fact has noncanonical payload",
			wantDetail: "persisted fact is not canonical",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, _ SyncAuthority) {
				if _, err := store.db.Exec(`
UPDATE continuity_facts
SET content_json = '{}'
WHERE project_id = ? AND fact_kind = 'project.registered'`, string(projectID)); err != nil {
					t.Fatalf("corrupt canonical fact payload: %v", err)
				}
			},
		},
		{
			name: "conflicting duplicate metadata",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				certificateID := authority.Environments[0].CertificateID
				receipt := syncAuthorityPromotionMetadataV2("conflicting-duplicate-receipt", [32]byte{}, certificateID)
				tombstone := syncAuthorityPromotionMetadataV2("conflicting-duplicate-tombstone", [32]byte{}, certificateID)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-conflicting-duplicate", "environment-a", 1, receipt)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-conflicting-duplicate", "environment-a", 1, tombstone)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, tombstone)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "duplicate sequence changes fact identity",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("duplicate-sequence-fact", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-sequence-owner", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-sequence-owner", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-sequence-impostor", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "same arrival changes source identity",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				first := syncAuthorityPromotionMetadataV2("same-arrival-first", [32]byte{}, authority.Environments[0].CertificateID)
				second := syncAuthorityPromotionMetadataV2("same-arrival-second", [32]byte{}, authority.Environments[1].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-same-arrival-first", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-same-arrival-first", "environment-a", 1, first)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-same-arrival-second", "environment-b", 1, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, first)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-b", 1, second)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "exact pruned duplicate changes arrival",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("duplicate-arrival", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-duplicate-arrival", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-duplicate-arrival", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "tombstone has no receipt",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("tombstone-without-receipt", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-tombstone-without-receipt", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "receipt arrival frontier has a gap",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				certificateID := authority.Environments[0].CertificateID
				first := syncAuthorityPromotionMetadataV2("receipt-frontier-first", [32]byte{}, certificateID)
				second := syncAuthorityPromotionMetadataV2("receipt-frontier-third", first.digest, certificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-receipt-frontier-first", "environment-a", 1)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-receipt-frontier-third", "environment-a", 2)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-receipt-frontier-first", "environment-a", 1, first)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 3, "fact-receipt-frontier-third", "environment-a", 2, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, second)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 3)
			},
		},
		{
			name: "receipt exceeds applied cursor",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("receipt-beyond-cursor", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-receipt-beyond-cursor", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-receipt-beyond-cursor", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 0)
			},
		},
		{
			name: "sequence gap",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				previous := sha256.Sum256([]byte("missing sequence one"))
				metadata := syncAuthorityPromotionMetadataV2("sequence-gap", previous, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-sequence-gap", "environment-a", 2, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "broken previous digest",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				certificateID := authority.Environments[0].CertificateID
				first := syncAuthorityPromotionMetadataV2("broken-chain-first", [32]byte{}, certificateID)
				wrongPrevious := sha256.Sum256([]byte("not the first digest"))
				second := syncAuthorityPromotionMetadataV2("broken-chain-second", wrongPrevious, certificateID)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-broken-chain-1", "environment-a", 1, first)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-broken-chain-2", "environment-a", 2, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, second)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "unsealed suffix gap",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				sealed := syncAuthorityPromotionMetadataV2("unsealed-gap-sealed", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-unsealed-gap-sealed", "environment-a", 1, sealed)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-unsealed-gap-sealed", "environment-a", 1, sealed)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-unsealed-gap-live", "environment-a", 3)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, sealed)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET highest_sequence = 3
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("extend corrupt unsealed-gap head: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "live fact overlaps tombstone",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("live-tombstone-overlap", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-live-tombstone-overlap", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-live-tombstone-overlap", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-live-tombstone-overlap", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "fully sealed live fact clock differs from head",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("sealed-live-head-clock", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-sealed-live-head-clock", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-sealed-live-head-clock", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET hlc_wall_millis = 2
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("corrupt fully sealed head clock: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "first unsealed fact clock does not increase from live sealed predecessor",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				metadata := syncAuthorityPromotionMetadataV2("nonincreasing-live-boundary", [32]byte{}, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-nonincreasing-live-boundary-sealed", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-nonincreasing-live-boundary-sealed", "environment-a", 1, metadata)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-nonincreasing-live-boundary-unsealed", "environment-a", 2)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, metadata)
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET highest_sequence = 2
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("extend nonincreasing live-boundary head: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 1)
			},
		},
		{
			name: "retained sealed live fact clocks regress",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				first := syncAuthorityPromotionMetadataV2("regressing-sealed-live-first", [32]byte{}, authority.Environments[0].CertificateID)
				second := syncAuthorityPromotionMetadataV2("regressing-sealed-live-second", first.digest, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-regressing-sealed-live-first", "environment-a", 1)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-regressing-sealed-live-second", "environment-a", 2)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-regressing-sealed-live-first", "environment-a", 1, first)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 2, "fact-regressing-sealed-live-second", "environment-a", 2, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, second)
				if _, err := store.db.Exec(`
UPDATE continuity_facts
SET hlc_wall_millis = CASE fact_id
  WHEN 'fact-regressing-sealed-live-first' THEN 30
  WHEN 'fact-regressing-sealed-live-second' THEN 20
END
WHERE project_id = ? AND fact_id IN ('fact-regressing-sealed-live-first', 'fact-regressing-sealed-live-second')`, string(projectID)); err != nil {
					t.Fatalf("regress retained sealed live fact clocks: %v", err)
				}
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET hlc_wall_millis = 20
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("set regressing sealed live head clock: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "pruned final head clock regresses from retained live fact",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				first := syncAuthorityPromotionMetadataV2("regressing-pruned-final-first", [32]byte{}, authority.Environments[0].CertificateID)
				second := syncAuthorityPromotionMetadataV2("regressing-pruned-final-second", first.digest, authority.Environments[0].CertificateID)
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-regressing-pruned-final-first", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-regressing-pruned-final-first", "environment-a", 1, first)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 2, "fact-regressing-pruned-final-second", "environment-a", 2, second)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-regressing-pruned-final-second", "environment-a", 2, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, second)
				if _, err := store.db.Exec(`
UPDATE continuity_facts
SET hlc_wall_millis = 30
WHERE project_id = ? AND fact_id = 'fact-regressing-pruned-final-first'`, string(projectID)); err != nil {
					t.Fatalf("advance retained live fact before pruned final: %v", err)
				}
				if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_heads
SET hlc_wall_millis = 20
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
					t.Fatalf("set regressing pruned final head clock: %v", err)
				}
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "mint once certificate changed",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				first := syncAuthorityPromotionMetadataV2("certificate-first", [32]byte{}, authority.Environments[0].CertificateID)
				second := syncAuthorityPromotionMetadataV2("certificate-second", first.digest, authority.Environments[1].CertificateID)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 1, "fact-certificate-1", "environment-a", 1, first)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-certificate-2", "environment-a", 2, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 2, second)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
		{
			name: "generation nonce reused by another environment",
			seed: func(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) {
				first := syncAuthorityPromotionMetadataV2("nonce-owner-a", [32]byte{}, authority.Environments[0].CertificateID)
				second := syncAuthorityPromotionMetadataV2("nonce-owner-b", [32]byte{}, authority.Environments[1].CertificateID)
				second.nonce = first.nonce
				insertSyncAuthorityPromotionFactV2(t, store, projectID, "fact-nonce-a", "environment-a", 1)
				insertSyncAuthorityPromotionReceiptV2(t, store, projectID, 1, "fact-nonce-a", "environment-a", 1, first)
				insertSyncAuthorityPromotionTombstoneV2(t, store, projectID, 2, "fact-nonce-b", "environment-b", 1, second)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", 1, first)
				insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-b", 1, second)
				setSyncAuthorityPromotionProgressV2(t, store, projectID, 2)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, projectID, authority := syncAuthorityPromotionProtectedHistoryFixtureV2(t, syncSlug(test.name))
			test.seed(t, store, projectID, authority)
			ready := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
			before := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)

			if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err == nil {
				t.Fatal("PromoteSyncAuthorityCandidate(corrupt protected history) error = nil")
			} else {
				assertSyncErrorCode(t, err, SyncErrorStore)
				if test.wantDetail != "" {
					problem, ok := err.(*SyncError)
					if !ok || problem.Detail != test.wantDetail {
						t.Fatalf("corrupt protected-history error = %v, want detail %q", err, test.wantDetail)
					}
				}
			}
			after := append(terminalLogicalRowsV1(t, store, projectID), syncAuthorityCandidatePersistedRowsV2(t, store, projectID)...)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("failed promotion changed state:\n got %#v\nwant %#v", after, before)
			}
		})
	}
}

func TestPromoteSyncAuthorityCandidateStreamsLargeProtectedHistory(t *testing.T) {
	store, projectID, authority := syncAuthorityPromotionProtectedHistoryFixtureV2(t, "large-protected-history")
	const historyLength = 5_000
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatalf("begin large history seed: %v", err)
	}
	receiptStatement, err := tx.Prepare(`
INSERT INTO continuity_sync_receipts(
  project_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
) VALUES(?, ?, ?, 'environment-a', ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		t.Fatalf("prepare large receipt history seed: %v", err)
	}
	tombstoneStatement, err := tx.Prepare(`
INSERT INTO continuity_sync_tombstones(
  fact_id, project_id, environment_id, environment_sequence, arrival_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation,
  nonce, prune_certificate_id
) VALUES(?, ?, 'environment-a', ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		receiptStatement.Close()
		tx.Rollback()
		t.Fatalf("prepare large tombstone history seed: %v", err)
	}
	certificateID := authority.Environments[0].CertificateID
	pruneCertificateID := sha256.Sum256([]byte("large protected history prune certificate"))
	var previous [32]byte
	var final sealedEnvelopeMetadataV1
	for sequence := int64(1); sequence <= historyLength; sequence++ {
		metadata := syncAuthorityPromotionMetadataV2(fmt.Sprintf("large-history-%d", sequence), previous, certificateID)
		metadata.keyGeneration = uint32(sequence)
		factID := fmt.Sprintf("fact-large-history-%d", sequence)
		if _, err := receiptStatement.Exec(
			string(projectID), sequence, factID, sequence,
			metadata.previousDigest[:], metadata.digest[:], metadata.certificateID[:], metadata.keyGeneration,
			metadata.nonce[:],
		); err != nil {
			receiptStatement.Close()
			tombstoneStatement.Close()
			tx.Rollback()
			t.Fatalf("insert large receipt history sequence %d: %v", sequence, err)
		}
		if _, err := tombstoneStatement.Exec(
			factID, string(projectID), sequence, sequence,
			metadata.previousDigest[:], metadata.digest[:], metadata.certificateID[:], metadata.keyGeneration,
			metadata.nonce[:], pruneCertificateID[:],
		); err != nil {
			receiptStatement.Close()
			tombstoneStatement.Close()
			tx.Rollback()
			t.Fatalf("insert large tombstone history sequence %d: %v", sequence, err)
		}
		previous = metadata.digest
		final = metadata
	}
	if err := receiptStatement.Close(); err != nil {
		tombstoneStatement.Close()
		tx.Rollback()
		t.Fatalf("close large receipt history statement: %v", err)
	}
	if err := tombstoneStatement.Close(); err != nil {
		tx.Rollback()
		t.Fatalf("close large tombstone history statement: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit large history seed: %v", err)
	}
	insertSyncAuthorityPromotionHeadV2(t, store, projectID, "environment-a", historyLength, final)
	setSyncAuthorityPromotionProgressV2(t, store, projectID, historyLength)
	assertSyncAuthorityPromotionStreamingPlanV2(
		t, store, syncAuthorityPromotionEnvelopeInventoryQueryV2,
		string(projectID), string(projectID), string(projectID), string(projectID),
	)
	assertSyncAuthorityPromotionStreamingPlanV2(
		t, store, syncAuthorityPromotionGenerationNoncesQueryV2,
		string(projectID), string(projectID), string(projectID),
	)
	assertSyncAuthorityPromotionStreamingPlanV2(
		t, store, syncAuthorityPromotionArrivalInventoryQueryV2,
		string(projectID), string(projectID),
	)
	assertSyncAuthorityPromotionStreamingPlanV2(
		t, store, syncAuthorityPromotionFactFrontierQueryV2,
		string(projectID), string(projectID), string(projectID), string(projectID), string(projectID),
	)
	assertSyncAuthorityPromotionStreamingPlanV2(
		t, store, syncAuthorityPromotionFactIdentityQueryV2,
		string(projectID), string(projectID), string(projectID), string(projectID),
	)
	assertSyncAuthorityPromotionSingleStreamPlanV2(
		t, store, syncAuthorityPromotionCanonicalFactsQueryV2, "continuity_facts", string(projectID),
	)
	ready := stageSyncAuthorityGuardCandidateV2(t, store, projectID, authority, true)
	assertSyncAuthorityPromotionCompositePlansV2(t, store, projectID, ready)

	if _, err := store.PromoteSyncAuthorityCandidate(context.Background(), projectID, ready.Checkpoint()); err != nil {
		t.Fatalf("PromoteSyncAuthorityCandidate(%d protected rows) error = %v", historyLength, err)
	}
	var retainedReceipts, retainedTombstones int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_receipts WHERE project_id = ?`, string(projectID)).Scan(&retainedReceipts); err != nil {
		t.Fatalf("count retained large receipt history: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_tombstones WHERE project_id = ?`, string(projectID)).Scan(&retainedTombstones); err != nil {
		t.Fatalf("count retained large tombstone history: %v", err)
	}
	if retainedReceipts != historyLength || retainedTombstones != historyLength {
		t.Fatalf("retained protected history = (%d receipts, %d tombstones), want (%d, %d)", retainedReceipts, retainedTombstones, historyLength, historyLength)
	}
}

func assertSyncAuthorityPromotionCompositePlansV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	candidate SyncAuthorityCandidate,
) {
	t.Helper()
	tests := []struct {
		name     string
		query    string
		args     []any
		required []string
	}{
		{
			name:  "ready candidate membership event PK stream",
			query: syncAuthorityCandidateMembershipEventStreamQueryV2,
			args:  []any{string(projectID), candidate.CandidateID[:]},
			required: []string{
				"SEARCH event USING PRIMARY KEY (project_id=? AND candidate_id=?)",
				"SEARCH environment USING PRIMARY KEY (project_id=? AND candidate_id=? AND environment_id=?)",
			},
		},
		{
			name:     "canonical v2 structural stream",
			query:    canonicalSyncAuthorityInventoryQueryV2,
			args:     []any{string(projectID)},
			required: []string{"SEARCH continuity_sync_environment_certificates USING PRIMARY KEY"},
		},
		{
			name:     "bounded canonical v1 structural stream",
			query:    canonicalSyncAuthorityLegacyInventoryQueryV2,
			args:     []any{string(projectID), maximumSyncAuthorityEnvironments + 1},
			required: []string{"SEARCH continuity_sync_environment_certificates USING PRIMARY KEY"},
		},
		{
			name:  "canonical omission comparison",
			query: syncAuthorityCandidateOmittedCanonicalInventoryQueryV2,
			args: []any{
				string(projectID), candidate.CandidateID[:], string(projectID),
				1, candidate.ThroughEnvironmentID,
			},
			required: []string{"SEARCH canonical USING PRIMARY KEY", "SEARCH candidate USING PRIMARY KEY"},
		},
		{
			name:  "canonical identity comparison",
			query: syncAuthorityCandidateChangedCanonicalInventoryQueryV2,
			args: []any{
				string(projectID), candidate.CandidateID[:], 0, 0, candidate.Snapshot.MembershipGeneration,
			},
			required: []string{"SEARCH candidate USING PRIMARY KEY", "SEARCH canonical USING PRIMARY KEY"},
		},
		{
			name:  "canonical new-environment comparison",
			query: syncAuthorityCandidateInvalidNewInventoryQueryV2,
			args: []any{
				string(projectID), candidate.CandidateID[:], 0, candidate.Snapshot.MembershipGeneration,
			},
			required: []string{"SEARCH candidate USING PRIMARY KEY", "SEARCH canonical USING PRIMARY KEY"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			details := syncAuthorityCandidateQueryPlanV2(t, store, test.query, test.args...)
			for _, detail := range details {
				if strings.Contains(detail, "USE TEMP B-TREE") {
					t.Fatalf("composite validation uses an unbounded temp sorter: %v", details)
				}
			}
			for _, required := range test.required {
				found := false
				for _, detail := range details {
					found = found || strings.Contains(detail, required)
				}
				if !found {
					t.Fatalf("composite validation plan = %v, want %q", details, required)
				}
			}
		})
	}
}

func assertSyncAuthorityPromotionStreamingPlanV2(t *testing.T, store *Store, query string, arguments ...any) {
	t.Helper()
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+query, arguments...)
	if err != nil {
		t.Fatalf("explain protected-history stream: %v", err)
	}
	defer rows.Close()
	var details []string
	merge := false
	wantIndexedTables := []string{
		"continuity_facts",
		"continuity_sync_receipts",
		"continuity_sync_outbox",
		"continuity_sync_tombstones",
		"continuity_sync_environment_heads",
	}
	indexedTables := make(map[string]bool, len(wantIndexedTables))
	for rows.Next() {
		var id, parent, unused int64
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan protected-history plan: %v", err)
		}
		details = append(details, detail)
		if strings.Contains(detail, "USE TEMP B-TREE") {
			t.Fatalf("protected-history stream uses an unbounded temp sorter: %v", details)
		}
		merge = merge || strings.Contains(detail, "MERGE (UNION ALL)")
		for _, table := range wantIndexedTables {
			forcedPrimary := "INDEXED BY sqlite_autoindex_" + table + "_1"
			if strings.Contains(detail, "SEARCH "+table+" USING") ||
				(strings.Contains(query, table+" "+forcedPrimary) && strings.Contains(detail, "SCAN "+table)) {
				indexedTables[table] = true
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate protected-history plan: %v", err)
	}
	if !merge {
		t.Fatalf("protected-history stream is not merge/index driven: %v", details)
	}
	for _, table := range wantIndexedTables {
		if strings.Contains(query, table) && !indexedTables[table] {
			t.Fatalf("protected-history stream does not use an index for %s: %v", table, details)
		}
	}
}

func assertSyncAuthorityPromotionSingleStreamPlanV2(t *testing.T, store *Store, query string, table string, arguments ...any) {
	t.Helper()
	rows, err := store.db.Query("EXPLAIN QUERY PLAN "+query, arguments...)
	if err != nil {
		t.Fatalf("explain protected-history single stream: %v", err)
	}
	defer rows.Close()
	var details []string
	indexed := false
	for rows.Next() {
		var id, parent, unused int64
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan protected-history single-stream plan: %v", err)
		}
		details = append(details, detail)
		if strings.Contains(detail, "USE TEMP B-TREE") {
			t.Fatalf("protected-history single stream uses an unbounded temp sorter: %v", details)
		}
		if strings.Contains(detail, table) && (strings.Contains(detail, " USING ") || strings.HasPrefix(detail, "SCAN "+table)) {
			indexed = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate protected-history single-stream plan: %v", err)
	}
	if !indexed {
		t.Fatalf("protected-history single stream is not index driven for %s: %v", table, details)
	}
}

func syncAuthorityPromotionProtectedHistoryFixtureV2(t *testing.T, suffix string) (*Store, continuity.ProjectID, SyncAuthority) {
	t.Helper()
	store, projectID := storeWithLocalRoot(t, "authority-promotion-protected-history-"+suffix)
	authority := testActiveSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	return store, projectID, authority
}

func syncAuthorityPromotionMetadataV2(label string, previous [32]byte, certificateID [32]byte) sealedEnvelopeMetadataV1 {
	return sealedEnvelopeMetadataV1{
		previousDigest: previous,
		digest:         sha256.Sum256([]byte("authority promotion metadata digest " + label)),
		certificateID:  certificateID,
		keyGeneration:  1,
		nonce:          testNonce("authority-promotion-metadata-" + label),
	}
}

func insertSyncAuthorityPromotionReceiptV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	arrivalSequence int64,
	factID string,
	environmentID string,
	environmentSequence int64,
	metadata sealedEnvelopeMetadataV1,
) {
	t.Helper()
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_receipts(
  project_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), arrivalSequence, factID, environmentID, environmentSequence,
		metadata.previousDigest[:], metadata.digest[:], metadata.certificateID[:], metadata.keyGeneration, metadata.nonce[:],
	); err != nil {
		t.Fatalf("insert protected receipt: %v", err)
	}
}

func insertSyncAuthorityPromotionFactV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	factID string,
	environmentID string,
	environmentSequence int64,
) {
	t.Helper()
	content, err := encodeJournalRecordedV1(continuity.JournalRecordedPayload{
		Observation: appendObservationV1(),
		Content: continuity.JournalContent{
			Category: continuity.JournalNote,
			Text:     "Authority promotion protected fact " + factID,
		},
	})
	if err != nil {
		t.Fatalf("encode protected fact: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_facts(
  fact_id, project_id, subject_kind, subject_id, fact_kind, payload_version,
  content_json, environment_id, environment_sequence, hlc_wall_millis,
  hlc_logical, envelope_version
) VALUES(?, ?, 'journal-entry', ?, 'journal.recorded', 1, ?, ?, ?, 1, 0, 1)`,
		factID, string(projectID), "entry-"+factID, string(content), environmentID, environmentSequence,
	); err != nil {
		t.Fatalf("insert protected fact: %v", err)
	}
}

func insertSyncAuthorityPromotionTombstoneV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	arrivalSequence int64,
	factID string,
	environmentID string,
	environmentSequence int64,
	metadata sealedEnvelopeMetadataV1,
) {
	t.Helper()
	pruneCertificateID := sha256.Sum256([]byte("authority promotion prune certificate " + factID))
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_tombstones(
  fact_id, project_id, environment_id, environment_sequence, arrival_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation,
  nonce, prune_certificate_id
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		factID, string(projectID), environmentID, environmentSequence, arrivalSequence,
		metadata.previousDigest[:], metadata.digest[:], metadata.certificateID[:], metadata.keyGeneration,
		metadata.nonce[:], pruneCertificateID[:],
	); err != nil {
		t.Fatalf("insert protected tombstone: %v", err)
	}
}

func insertSyncAuthorityPromotionHeadV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	environmentID string,
	sequence int64,
	metadata sealedEnvelopeMetadataV1,
) {
	t.Helper()
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_heads(
  project_id, environment_id, highest_sequence, hlc_wall_millis, hlc_logical,
  sealed_sequence, previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce
) VALUES(?, ?, ?, 1, 0, ?, ?, ?, ?, ?, ?)`,
		string(projectID), environmentID, sequence, sequence, metadata.previousDigest[:], metadata.digest[:],
		metadata.certificateID[:], metadata.keyGeneration, metadata.nonce[:],
	); err != nil {
		t.Fatalf("insert protected head: %v", err)
	}
}

func insertSyncAuthorityPromotionUnsealedHeadV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	environmentID string,
	sequence int64,
	wallMillis int64,
) {
	t.Helper()
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_heads(
  project_id, environment_id, highest_sequence, hlc_wall_millis, hlc_logical,
  sealed_sequence
) VALUES(?, ?, ?, ?, 0, 0)`,
		string(projectID), environmentID, sequence, wallMillis,
	); err != nil {
		t.Fatalf("insert protected unsealed head: %v", err)
	}
}

func setSyncAuthorityPromotionProgressV2(t *testing.T, store *Store, projectID continuity.ProjectID, head int64) {
	t.Helper()
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET downloaded_cursor = ?, applied_cursor = ?, relay_head = ?
WHERE project_id = ?`, head, head, head, string(projectID)); err != nil {
		t.Fatalf("advance protected history progress: %v", err)
	}
}

func syncAuthorityPromotionLocalOutboxFixtureV2(t *testing.T, suffix string) (*Store, continuity.ProjectID, SealedOutboxFrame) {
	t.Helper()
	store := openAppendStoreV1(t, filepath.Join(testTempDir(t), "state-authority-promotion-"+suffix), "environment-local", 100)
	projectID := continuity.ProjectID("project-authority-promotion-" + suffix)
	mustAppendV1(t)(store.RegisterProject(context.Background(), projectID, "fact-project", continuity.ProjectRegistrationPayload{
		Observation: appendObservationV1(),
		Label:       "Loaf",
	}))
	installTestSyncAuthority(t, store, projectID, testSyncChannelID("channel-a"))
	if _, err := store.ActivateStagedSync(context.Background(), projectID, currentSyncAuthorityBindingForTest(t, store, projectID)); err != nil {
		t.Fatalf("ActivateStagedSync() error = %v", err)
	}
	unsealed, found, err := store.NextUnsealedLocalFact(context.Background(), projectID)
	if err != nil || !found {
		t.Fatalf("NextUnsealedLocalFact() = (_, %v, %v), want fact", found, err)
	}
	sealed := []byte("sealed authority promotion local root " + suffix)
	outbox := SealedOutboxFrame{
		FactID:         unsealed.Fact.FactID,
		EnvelopeDigest: sha256.Sum256(sealed),
		CertificateID:  sha256.Sum256([]byte("local certificate")),
		KeyGeneration:  1,
		Nonce:          testNonce("authority-promotion-outbox-" + suffix),
		SealedEnvelope: sealed,
	}
	if err := store.PersistSealedOutbox(context.Background(), projectID, testSyncChannelID("channel-a"), outbox); err != nil {
		t.Fatalf("PersistSealedOutbox() error = %v", err)
	}
	return store, projectID, outbox
}

func stageSyncAuthorityRetirementCandidateV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	finalSequence int64,
	finalDigest [32]byte,
) SyncAuthorityCandidate {
	t.Helper()
	return stageSyncAuthorityRetirementCandidateForEnvironmentV2(
		t, store, projectID, "environment-local", finalSequence, finalDigest,
	)
}

func stageSyncAuthorityRetirementCandidateForEnvironmentV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	environmentID string,
	finalSequence int64,
	finalDigest [32]byte,
) SyncAuthorityCandidate {
	t.Helper()
	authority, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	environments := cloneSyncAuthorityCandidateEnvironmentsV2(authority.Environments)
	nextMembership := authority.MembershipGeneration + 1
	found := false
	for index := range environments {
		if environments[index].EnvironmentID != environmentID {
			continue
		}
		found = true
		environments[index].Retirement = &SyncEnvironmentRetirement{
			RelayGeneration:          authority.RelayGeneration,
			MembershipGeneration:     nextMembership,
			FinalEnvironmentSequence: finalSequence,
			FinalEnvelopeDigest:      finalDigest,
			RetirementID:             sha256.Sum256([]byte("authority-promotion-retirement-" + string(projectID))),
			RetirementBytes:          []byte("authority promotion retirement " + string(projectID)),
		}
		break
	}
	if !found {
		t.Fatalf("environment %q not found in authority", environmentID)
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, binding.AuthorityDigestVersion, binding.AuthorityDigest)
	snapshot.MembershipGeneration = nextMembership
	snapshot.InventoryArrivalHead = authority.InventoryArrivalHead + 1
	ready, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("stage retirement authority candidate: %v", err)
	}
	return ready
}

func syncAuthorityPromotionDataPlaneRowsV2(t *testing.T, store *Store, projectID continuity.ProjectID) []string {
	t.Helper()
	rows := terminalLogicalRowsV1(t, store, projectID)
	kept := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.HasPrefix(row, "continuity_sync_projects|") ||
			strings.HasPrefix(row, "continuity_sync_environment_certificates|") {
			continue
		}
		kept = append(kept, row)
	}
	return kept
}

func syncAuthorityPromotionCanonicalEnvironmentRowV2(rows []string, environmentID string) string {
	want := `|environment_id=string:"` + environmentID + `"`
	for _, row := range rows {
		if strings.HasPrefix(row, "continuity_sync_environment_certificates|") && strings.Contains(row, want) {
			return row
		}
	}
	return ""
}

func stageSyncAuthorityGuardCandidateV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	authority SyncAuthority,
	ready bool,
) SyncAuthorityCandidate {
	t.Helper()
	binding := currentSyncAuthorityBindingForTest(t, store, projectID)
	snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, binding.AuthorityDigestVersion, binding.AuthorityDigest)
	environments := authority.Environments
	more := false
	if !ready {
		environments = environments[:1]
		more = true
	}
	candidate, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot, syncAuthorityCandidatePageV2("", environments, more),
	)
	if err != nil {
		t.Fatalf("stage %v authority guard candidate: %v", map[bool]string{false: "staging", true: "ready"}[ready], err)
	}
	if candidate.Ready != ready {
		t.Fatalf("guard candidate ready = %v, want %v", candidate.Ready, ready)
	}
	return candidate
}

func stageSyncAuthorityCandidateInventoryV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
	environments []SyncEnvironmentCertificate,
) SyncAuthorityCandidate {
	t.Helper()
	after := ""
	var candidate SyncAuthorityCandidate
	for offset := 0; offset < len(environments); offset += maximumSyncAuthorityCandidatePageEnvironments {
		end := offset + maximumSyncAuthorityCandidatePageEnvironments
		if end > len(environments) {
			end = len(environments)
		}
		page := syncAuthorityCandidatePageV2(after, environments[offset:end], end < len(environments))
		var err error
		candidate, err = store.StageVerifiedSyncAuthorityCandidatePage(context.Background(), projectID, snapshot, page)
		if err != nil {
			t.Fatalf("stage authority candidate inventory page %d: %v", candidate.PageCount+1, err)
		}
		after = page.ThroughEnvironmentID
	}
	if !candidate.Ready || candidate.EnvironmentCount != int64(len(environments)) {
		t.Fatalf("staged authority candidate inventory = %#v, want ready with %d environments", candidate, len(environments))
	}
	return candidate
}

func insertSyncAuthorityPromotionBootstrapOrphanV2(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	kind string,
) {
	t.Helper()
	digest := sha256.Sum256([]byte("authority promotion bootstrap orphan " + kind))
	otherDigest := sha256.Sum256([]byte("authority promotion bootstrap orphan other " + kind))
	nonce := testNonce("authority-promotion-bootstrap-orphan-" + syncSlug(kind))
	var err error
	switch kind {
	case "inbox":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES(?, 1, ?, 'sealed', X'01', 'staged')`, string(projectID), digest[:])
	case "receipt":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_receipts(
  project_id, arrival_sequence, fact_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
) VALUES(?, 1, 'fact-orphan', 'environment:0001', 1, zeroblob(32), ?, ?, 1, ?)`,
			string(projectID), digest[:], otherDigest[:], nonce[:])
	case "outbox":
		sealed := []byte("orphan outbox bytes")
		sealedDigest := sha256.Sum256(sealed)
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_outbox(
  fact_id, project_id, environment_id, environment_sequence,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, sealed_envelope
) VALUES('fact-orphan', ?, 'environment:0001', 1, zeroblob(32), ?, ?, 1, ?, ?)`,
			string(projectID), sealedDigest[:], otherDigest[:], nonce[:], sealed)
	case "tombstone":
		pruneCertificate := sha256.Sum256([]byte("orphan prune certificate"))
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_tombstones(
  fact_id, project_id, environment_id, environment_sequence, arrival_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation,
  nonce, prune_certificate_id
) VALUES('fact-orphan', ?, 'environment:0001', 1, 1, zeroblob(32), ?, ?, 1, ?, ?)`,
			string(projectID), digest[:], otherDigest[:], nonce[:], pruneCertificate[:])
	case "terminal header":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest
) VALUES(?, ?, 'staging', ?, ?, 1, ?, 1, 1, 1, ?)`,
			string(projectID), digest[:], otherDigest[:], digest[:], otherDigest[:], digest[:])
	case "terminal frame":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES(?, ?, 1, 'sealed', 'fact-orphan', 'environment:0001', 1, 1, 0,
  zeroblob(32), ?, ?, 1, ?, X'0102')`,
			string(projectID), digest[:], otherDigest[:], digest[:], nonce[:])
	case "promoted authority header":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
) VALUES(?, ?, 'promoted', ?, ?, ?, 1, 1, 1, 1, 'environment:orphan', ?, 2, ?)`,
			string(projectID), digest[:], otherDigest[:], digest[:], otherDigest[:], digest[:], otherDigest[:])
	case "authority page":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 1, NULL, 'environment:orphan', 1, 0, ?, 1, ?)`,
			string(projectID), digest[:], otherDigest[:], digest[:])
	case "authority environment":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation
) VALUES(?, ?, 'environment:orphan', 1, 1, ?, X'01', 'trusted', 0, 1)`,
			string(projectID), digest[:], otherDigest[:])
	case "authority membership event":
		_, err = store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_membership_events(
  project_id, candidate_id, membership_generation, event_kind, environment_id
) VALUES(?, ?, 1, 'join', 'environment:orphan')`, string(projectID), digest[:])
	default:
		t.Fatalf("unknown orphan fixture %q", kind)
	}
	if err != nil {
		t.Fatalf("insert %s orphan: %v", kind, err)
	}
}
