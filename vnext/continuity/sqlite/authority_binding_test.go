package sqlite

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestCurrentSyncAuthorityBindingReadsV1AndV2(t *testing.T) {
	t.Parallel()

	t.Run("v1", func(t *testing.T) {
		store := openSyncStore(t, "authority-binding-v1")
		projectID := continuity.ProjectID("project-authority-binding-v1")
		authority := testSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		digest, err := frozenSyncAuthorityDigestV1(projectID, authority)
		if err != nil {
			t.Fatalf("frozenSyncAuthorityDigestV1() error = %v", err)
		}
		binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
		}
		assertSyncAuthorityBindingForTest(t, binding, authority, 1, digest)
	})

	t.Run("v2 with zero head", func(t *testing.T) {
		store := openSyncStore(t, "authority-binding-v2-zero-head")
		projectID := continuity.ProjectID("project-authority-binding-v2-zero-head")
		authority := testSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		digest := setCanonicalSyncAuthorityMetadataV2ForBindingTest(t, store, projectID, authority)
		binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
		}
		assertSyncAuthorityBindingForTest(t, binding, authority, 2, digest)
	})

	t.Run("v2 with positive head", func(t *testing.T) {
		store := openSyncStore(t, "authority-binding-v2-positive-head")
		projectID := continuity.ProjectID("project-authority-binding-v2-positive-head")
		authority := testSyncAuthority()
		if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
			t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
		}
		authority.InventoryArrivalHead = 7
		digest := setCanonicalSyncAuthorityMetadataV2ForBindingTest(t, store, projectID, authority)
		binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
		if err != nil {
			t.Fatalf("CurrentSyncAuthorityBinding() error = %v", err)
		}
		assertSyncAuthorityBindingForTest(t, binding, authority, 2, digest)
	})
}

func TestCurrentSyncAuthorityBindingDoesNotScanUnboundedInventory(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-binding-v2-no-inventory-scan")
	projectID := continuity.ProjectID("project-authority-binding-v2-no-inventory-scan")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(300)
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, syncAuthorityCandidateManyEnvironmentsV2(300))
	digest := seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)
	execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), authority.Environments[299].EnvironmentID)

	binding, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding(300 environments with corrupt child) error = %v", err)
	}
	assertSyncAuthorityBindingForTest(t, binding, authority, 2, digest)
	if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthority(corrupt 300-environment inventory) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestCurrentSyncAuthorityBindingFailsClosedOnMalformedRows(t *testing.T) {
	t.Parallel()

	zero := make([]byte, 32)
	tests := []struct {
		name   string
		mutate func(*testing.T, *Store, continuity.ProjectID)
	}{
		{
			name: "short channel id",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET channel_id = X'01' WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "zero channel id",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET channel_id = ? WHERE project_id = ?`, zero, string(projectID))
			},
		},
		{
			name: "short relay generation",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET relay_generation = X'01' WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "zero relay generation",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET relay_generation = ? WHERE project_id = ?`, zero, string(projectID))
			},
		},
		{
			name: "short admin public key",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET admin_public_key = X'01' WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "zero admin public key",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET admin_public_key = ? WHERE project_id = ?`, zero, string(projectID))
			},
		},
		{
			name: "zero membership generation",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET membership_generation = 0 WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "overflow membership generation",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_projects SET membership_generation = 4294967296 WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "negative inventory head",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_authorities SET inventory_arrival_head = -1 WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "v1 nonzero inventory head",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_authorities SET digest_version = 1, inventory_arrival_head = 1 WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "zero digest version",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_authorities SET digest_version = 0 WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "unsupported digest version",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_authorities SET digest_version = 3 WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "short authority digest",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_authorities SET authority_digest = X'01' WHERE project_id = ?`, string(projectID))
			},
		},
		{
			name: "zero authority digest",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `UPDATE continuity_sync_authorities SET authority_digest = ? WHERE project_id = ?`, zero, string(projectID))
			},
		},
		{
			name: "missing authority metadata",
			mutate: func(t *testing.T, store *Store, projectID continuity.ProjectID) {
				execAuthorityBindingCorruptionForTest(t, store, `DELETE FROM continuity_sync_authorities WHERE project_id = ?`, string(projectID))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "authority-binding-corrupt-"+syncSlug(test.name))
			projectID := continuity.ProjectID("project-authority-binding-corrupt-" + syncSlug(test.name))
			authority := testSyncAuthority()
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
				t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
			}
			setCanonicalSyncAuthorityMetadataV2ForBindingTest(t, store, projectID, authority)
			test.mutate(t, store, projectID)
			if _, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID); err == nil {
				t.Fatal("CurrentSyncAuthorityBinding(corrupt rows) error = nil")
			} else {
				var problem *SyncError
				if !errors.As(err, &problem) || problem.Code != SyncErrorStore || problem.Field != "sync_authority" {
					t.Fatalf("CurrentSyncAuthorityBinding(corrupt rows) error = %#v, want fielded authority store error", err)
				}
			}
		})
	}
}

func TestCurrentSyncAuthorityBindingValidatesPublicInputsAndMissingState(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-binding-public-inputs")
	projectID := continuity.ProjectID("project-authority-binding-missing")
	if _, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthorityBinding(missing) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorNotFound)
	}
	if _, err := store.CurrentSyncAuthorityBinding(nil, projectID); err == nil {
		t.Fatal("CurrentSyncAuthorityBinding(nil context) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorInvalid)
	}
	if _, err := (*Store)(nil).CurrentSyncAuthorityBinding(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthorityBinding(nil store) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	if _, err := store.CurrentSyncAuthorityBinding(context.Background(), "invalid project id with spaces"); err == nil {
		t.Fatal("CurrentSyncAuthorityBinding(invalid project) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorInvalid)
	}
}

func TestCurrentSyncAuthorityBindingFailsClosedOnOrphanEnvironment(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-binding-orphan-environment")
	projectID := continuity.ProjectID("project-authority-binding-orphan-environment")
	if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys for corruption fixture: %v", err)
	}
	certificateID := sha256.Sum256([]byte("authority-binding-orphan-environment-certificate"))
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_certificates(
  project_id, environment_id, certificate_id, certificate_bytes,
  mode, expires_at_millis, join_membership_generation
) VALUES(?, 'environment-orphan', ?, X'01', 'trusted', 0, 1)`,
		string(projectID), certificateID[:],
	); err != nil {
		t.Fatalf("insert orphan authority environment: %v", err)
	}

	if _, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthorityBinding(orphan environment) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestCurrentSyncAuthorityReturnsCompleteDefensiveV2Inventory(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-v2-current-300")
	projectID := continuity.ProjectID("project-authority-v2-current-300")
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(300)
	authority := syncAuthorityFromSnapshotForBindingTest(snapshot, syncAuthorityCandidateManyEnvironmentsV2(300))
	seedCanonicalSyncAuthorityForBindingTest(t, store, projectID, authority)

	got, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	if !syncAuthorityEqual(got, authority) || len(got.Environments) != 300 {
		t.Fatalf("CurrentSyncAuthority() returned %d environments, want complete 300-environment v2 authority", len(got.Environments))
	}
	got.Environments[299].CertificateBytes[0] ^= 0xff
	reloaded, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(after caller mutation) error = %v", err)
	}
	if !syncAuthorityEqual(reloaded, authority) {
		t.Fatal("CurrentSyncAuthority() exposed mutable persisted v2 authority bytes")
	}
}

func TestInstallVerifiedSyncAuthorityRefusesV2BindingBeforeInventoryScan(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-binding-v2-install-refusal")
	projectID := continuity.ProjectID("project-authority-binding-v2-install-refusal")
	authority := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(v1 seed) error = %v", err)
	}
	setCanonicalSyncAuthorityMetadataV2ForBindingTest(t, store, projectID, authority)
	before, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthorityBinding(before corruption) error = %v", err)
	}
	execAuthorityBindingCorruptionForTest(t, store, `
UPDATE continuity_sync_environment_certificates
SET certificate_id = X'01'
WHERE project_id = ? AND environment_id = ?`, string(projectID), authority.Environments[0].EnvironmentID)
	advanced := cloneSyncAuthority(authority)
	advanced.MembershipGeneration++
	advanced.Environments = append(advanced.Environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-z",
		CertificateID:            sha256.Sum256([]byte("authority-binding-v2-install-refusal-environment-z")),
		CertificateBytes:         []byte("authority-binding-v2-install-refusal-environment-z-certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: advanced.MembershipGeneration,
	})

	_, err = store.InstallVerifiedSyncAuthority(context.Background(), projectID, advanced)
	assertSyncErrorCode(t, err, SyncErrorConflict)
	after, err := store.CurrentSyncAuthorityBinding(context.Background(), projectID)
	if err != nil || after != before {
		t.Fatalf("CurrentSyncAuthorityBinding(after refusal) = (%#v, %v), want unchanged %#v", after, err, before)
	}
	var membershipGeneration, environmentCount int64
	if err := store.db.QueryRow(`SELECT membership_generation FROM continuity_sync_projects WHERE project_id = ?`, string(projectID)).Scan(&membershipGeneration); err != nil {
		t.Fatalf("read membership after refusal: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_environment_certificates WHERE project_id = ?`, string(projectID)).Scan(&environmentCount); err != nil {
		t.Fatalf("count environments after refusal: %v", err)
	}
	if membershipGeneration != int64(authority.MembershipGeneration) || environmentCount != int64(len(authority.Environments)) {
		t.Fatalf("authority after refusal = membership %d, environments %d; want %d, %d", membershipGeneration, environmentCount, authority.MembershipGeneration, len(authority.Environments))
	}
}

func TestInstallVerifiedSyncAuthorityRefusesOrphanEnvironmentWithoutAdoption(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-binding-install-orphan-environment")
	projectID := continuity.ProjectID("project-authority-binding-install-orphan-environment")
	if _, err := store.db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys for corruption fixture: %v", err)
	}
	orphanCertificateID := sha256.Sum256([]byte("authority-binding-install-orphan-environment-certificate"))
	orphanCertificateBytes := []byte{0x00, 0x01, 0xff}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_environment_certificates(
  project_id, environment_id, certificate_id, certificate_bytes,
  mode, expires_at_millis, join_membership_generation
) VALUES(?, 'environment-orphan', ?, ?, 'trusted', 0, 1)`,
		string(projectID), orphanCertificateID[:], orphanCertificateBytes,
	); err != nil {
		t.Fatalf("insert orphan authority environment: %v", err)
	}

	authority := testSyncAuthority()
	if authority.Environments[0].EnvironmentID == "environment-orphan" {
		t.Fatal("test authority unexpectedly requests the orphan environment ID")
	}
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err == nil {
		t.Fatal("InstallVerifiedSyncAuthority(orphan environment) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}

	var projectCount, metadataCount, environmentCount int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_projects WHERE project_id = ?`, string(projectID)).Scan(&projectCount); err != nil {
		t.Fatalf("count project rows after refused install: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authorities WHERE project_id = ?`, string(projectID)).Scan(&metadataCount); err != nil {
		t.Fatalf("count authority rows after refused install: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_environment_certificates WHERE project_id = ?`, string(projectID)).Scan(&environmentCount); err != nil {
		t.Fatalf("count environment rows after refused install: %v", err)
	}
	if projectCount != 0 || metadataCount != 0 || environmentCount != 1 {
		t.Fatalf("rows after refused install = project %d, authority %d, environments %d; want 0, 0, 1", projectCount, metadataCount, environmentCount)
	}

	var gotCertificateID, gotCertificateBytes []byte
	var gotMode string
	var gotExpiresAtMillis, gotJoinMembershipGeneration int64
	if err := store.db.QueryRow(`
SELECT certificate_id, certificate_bytes, mode, expires_at_millis, join_membership_generation
FROM continuity_sync_environment_certificates
WHERE project_id = ? AND environment_id = 'environment-orphan'`, string(projectID)).Scan(
		&gotCertificateID,
		&gotCertificateBytes,
		&gotMode,
		&gotExpiresAtMillis,
		&gotJoinMembershipGeneration,
	); err != nil {
		t.Fatalf("read orphan environment after refused install: %v", err)
	}
	if string(gotCertificateID) != string(orphanCertificateID[:]) || string(gotCertificateBytes) != string(orphanCertificateBytes) ||
		gotMode != "trusted" || gotExpiresAtMillis != 0 || gotJoinMembershipGeneration != 1 {
		t.Fatalf("orphan environment changed after refused install: certificate %x, bytes %x, mode %q, expiry %d, join %d",
			gotCertificateID, gotCertificateBytes, gotMode, gotExpiresAtMillis, gotJoinMembershipGeneration)
	}
}

func TestTerminalCandidateIdentityFromAuthorityBindingPreservesV1Vector(t *testing.T) {
	t.Parallel()

	projectID := continuity.ProjectID("project-terminal-candidate-binding-v1")
	authority := testSyncAuthority()
	authorityDigest, wantCandidateID, err := deriveTerminalCandidateIdentityV1(projectID, authority, 17)
	if err != nil {
		t.Fatalf("deriveTerminalCandidateIdentityV1() error = %v", err)
	}
	binding := syncAuthorityBindingForTest(authority, 1, authorityDigest)
	gotCandidateID, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, binding, 17)
	if err != nil {
		t.Fatalf("deriveTerminalCandidateIDFromAuthorityBindingV1() error = %v", err)
	}
	if gotCandidateID != wantCandidateID {
		t.Fatalf("candidate ID from binding = %x, want frozen v1 identity %x", gotCandidateID, wantCandidateID)
	}

	validMutations := []struct {
		name string
		edit func(*SyncAuthorityBinding)
	}{
		{name: "channel", edit: func(value *SyncAuthorityBinding) { value.ChannelID[0] ^= 0xff }},
		{name: "relay", edit: func(value *SyncAuthorityBinding) { value.RelayGeneration[0] ^= 0xff }},
		{name: "admin", edit: func(value *SyncAuthorityBinding) { value.AdminPublicKey[0] ^= 0xff; value.AuthorityDigest[0] ^= 0x01 }},
		{name: "membership", edit: func(value *SyncAuthorityBinding) { value.MembershipGeneration++; value.AuthorityDigest[0] ^= 0x02 }},
		{name: "head", edit: func(value *SyncAuthorityBinding) {
			value.AuthorityDigestVersion = 2
			value.InventoryArrivalHead = 1
			value.AuthorityDigest[0] ^= 0x04
		}},
		{name: "digest version", edit: func(value *SyncAuthorityBinding) { value.AuthorityDigestVersion = 2; value.AuthorityDigest[0] ^= 0x08 }},
		{name: "digest", edit: func(value *SyncAuthorityBinding) { value.AuthorityDigest[0] ^= 0x10 }},
	}
	for _, mutation := range validMutations {
		mutated := binding
		mutation.edit(&mutated)
		candidateID, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, mutated, 17)
		if err != nil {
			t.Fatalf("derive terminal candidate after valid %s mutation: %v", mutation.name, err)
		}
		if candidateID == wantCandidateID {
			t.Fatalf("valid %s binding mutation did not change candidate identity", mutation.name)
		}
	}

	invalidMutations := []struct {
		name string
		edit func(*SyncAuthorityBinding)
	}{
		{name: "zero channel", edit: func(value *SyncAuthorityBinding) { value.ChannelID = SyncChannelID{} }},
		{name: "zero relay", edit: func(value *SyncAuthorityBinding) { value.RelayGeneration = [32]byte{} }},
		{name: "zero admin", edit: func(value *SyncAuthorityBinding) { value.AdminPublicKey = [32]byte{} }},
		{name: "zero membership", edit: func(value *SyncAuthorityBinding) { value.MembershipGeneration = 0 }},
		{name: "negative head", edit: func(value *SyncAuthorityBinding) { value.InventoryArrivalHead = -1 }},
		{name: "v1 positive head", edit: func(value *SyncAuthorityBinding) { value.InventoryArrivalHead = 1 }},
		{name: "zero digest version", edit: func(value *SyncAuthorityBinding) { value.AuthorityDigestVersion = 0 }},
		{name: "unsupported digest version", edit: func(value *SyncAuthorityBinding) { value.AuthorityDigestVersion = 3 }},
		{name: "zero digest", edit: func(value *SyncAuthorityBinding) { value.AuthorityDigest = [32]byte{} }},
	}
	for _, mutation := range invalidMutations {
		mutated := binding
		mutation.edit(&mutated)
		if _, err := deriveTerminalCandidateIDFromAuthorityBindingV1(projectID, mutated, 17); err == nil {
			t.Fatalf("derive terminal candidate after invalid %s mutation error = nil", mutation.name)
		}
	}
}

func syncAuthorityFromSnapshotForBindingTest(snapshot SyncAuthoritySnapshot, environments []SyncEnvironmentCertificate) SyncAuthority {
	return SyncAuthority{
		ChannelID:            snapshot.ChannelID,
		RelayGeneration:      snapshot.RelayGeneration,
		AdminPublicKey:       snapshot.AdminPublicKey,
		MembershipGeneration: snapshot.MembershipGeneration,
		InventoryArrivalHead: snapshot.InventoryArrivalHead,
		Environments:         cloneSyncAuthorityCandidateEnvironmentsV2(environments),
	}
}

func syncAuthorityBindingForTest(authority SyncAuthority, digestVersion uint16, digest [32]byte) SyncAuthorityBinding {
	return SyncAuthorityBinding{
		ChannelID:              authority.ChannelID,
		RelayGeneration:        authority.RelayGeneration,
		AdminPublicKey:         authority.AdminPublicKey,
		MembershipGeneration:   authority.MembershipGeneration,
		InventoryArrivalHead:   authority.InventoryArrivalHead,
		AuthorityDigestVersion: digestVersion,
		AuthorityDigest:        digest,
	}
}

func assertSyncAuthorityBindingForTest(t *testing.T, binding SyncAuthorityBinding, authority SyncAuthority, digestVersion uint16, digest [32]byte) {
	t.Helper()
	want := syncAuthorityBindingForTest(authority, digestVersion, digest)
	if binding != want {
		t.Fatalf("sync authority binding = %#v, want %#v", binding, want)
	}
}

func seedCanonicalSyncAuthorityForBindingTest(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) [32]byte {
	t.Helper()
	ctx := context.Background()
	digest := syncAuthorityCandidateAuthorityDigestForTestV2(t, projectID, authority)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin canonical v2 seed: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state,
  downloaded_cursor, applied_cursor, relay_head
) VALUES(?, ?, ?, ?, ?, 'staging', 0, 0, 0)`,
		string(projectID), authority.ChannelID[:], authority.RelayGeneration[:], authority.AdminPublicKey[:], authority.MembershipGeneration,
	); err != nil {
		t.Fatalf("insert canonical v2 project: %v", err)
	}
	for _, environment := range authority.Environments {
		if err := insertSyncEnvironmentCertificateV1(ctx, tx, projectID, environment); err != nil {
			t.Fatalf("insert canonical v2 environment: %v", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authorities(project_id, digest_version, authority_digest, inventory_arrival_head)
VALUES(?, 2, ?, ?)`, string(projectID), digest[:], authority.InventoryArrivalHead); err != nil {
		t.Fatalf("insert canonical v2 metadata: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit canonical v2 seed: %v", err)
	}
	return digest
}

func setCanonicalSyncAuthorityMetadataV2ForBindingTest(t *testing.T, store *Store, projectID continuity.ProjectID, authority SyncAuthority) [32]byte {
	t.Helper()
	digest := syncAuthorityCandidateAuthorityDigestForTestV2(t, projectID, authority)
	result, err := store.db.Exec(`
UPDATE continuity_sync_authorities
SET digest_version = 2, authority_digest = ?, inventory_arrival_head = ?
WHERE project_id = ?`, digest[:], authority.InventoryArrivalHead, string(projectID))
	if err != nil {
		t.Fatalf("set canonical v2 metadata: %v", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("set canonical v2 metadata affected = %d, error = %v", affected, err)
	}
	return digest
}

func execAuthorityBindingCorruptionForTest(t *testing.T, store *Store, query string, args ...any) {
	t.Helper()
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatalf("enable corruption fixture: %v", err)
	}
	if _, err := store.db.Exec(query, args...); err != nil {
		t.Fatalf("mutate authority binding: %v", err)
	}
}
