package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuityStoreRequiresPinnedAuthorityBeforeStaging(t *testing.T) {
	t.Parallel()

	store, err := Open(testTempDir(t), "environment-local")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	projectID := continuity.ProjectID("project-authority-required")

	if _, err := store.StageSyncPage(context.Background(), projectID, testSyncChannelID("channel-a"), 0, 0, nil); err == nil {
		t.Fatal("StageSyncPage() error = nil, want missing authority")
	} else {
		assertSyncErrorCode(t, err, SyncErrorNotFound)
	}
	var stagedRows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_projects WHERE project_id = ?`, string(projectID)).Scan(&stagedRows); err != nil {
		t.Fatalf("count authority-free staging rows: %v", err)
	}
	if stagedRows != 0 {
		t.Fatalf("StageSyncPage() created %d authority-free rows", stagedRows)
	}

	authority := testSyncAuthority()
	progress, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority)
	if err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	if progress.ProjectID != projectID || progress.ChannelID != authority.ChannelID ||
		progress.ActivationState != SyncActivationStaging || progress.DownloadedCursor != 0 ||
		progress.AppliedCursor != 0 || progress.RelayHead != 0 {
		t.Fatalf("installed progress = %#v", progress)
	}
	if _, err := store.StageSyncPage(context.Background(), projectID, authority.ChannelID, 0, 0, nil); err != nil {
		t.Fatalf("StageSyncPage(after authority) error = %v", err)
	}
}

func TestContinuityStoreInstallsAndReadsExactSyncAuthority(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-exact")
	projectID := continuity.ProjectID("project-authority-exact")
	authority := testSyncAuthority()

	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	got, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority() error = %v", err)
	}
	if !syncAuthorityEqual(got, authority) {
		t.Fatalf("CurrentSyncAuthority() = %#v, want %#v", got, authority)
	}
	if len(got.Environments) != 2 || got.Environments[0].EnvironmentID != "environment-a" || got.Environments[1].EnvironmentID != "environment-b" {
		t.Fatalf("environment inventory order = %#v, want sorted", got.Environments)
	}

	got.RelayGeneration[0] ^= 0xff
	got.AdminPublicKey[0] ^= 0xff
	got.Environments[0].CertificateBytes[0] ^= 0xff
	got.Environments[0].Retirement.RetirementBytes[0] ^= 0xff
	reloaded, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(after caller mutation) error = %v", err)
	}
	if !syncAuthorityEqual(reloaded, authority) {
		t.Fatalf("authority changed through returned buffers = %#v, want %#v", reloaded, authority)
	}

	retry, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority)
	if err != nil || retry != (SyncProgress{ProjectID: projectID, ChannelID: authority.ChannelID, ActivationState: SyncActivationStaging}) {
		t.Fatalf("InstallVerifiedSyncAuthority(exact retry) = %#v, %v", retry, err)
	}
}

func TestContinuityStoreRejectsSyncAuthorityIdentityAndInventoryRollback(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-conflicts")
	projectID := continuity.ProjectID("project-authority-conflicts")
	authority := testSyncAuthority()
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}

	tests := []struct {
		name  string
		edit  func(*SyncAuthority)
		code  SyncErrorCode
		field string
	}{
		{name: "channel", edit: func(value *SyncAuthority) { value.ChannelID = testSyncChannelID("channel-other") }, code: SyncErrorConflict, field: "channel_id"},
		{name: "relay generation", edit: func(value *SyncAuthority) {
			value.RelayGeneration[0]++
			value.Environments[0].Retirement.RelayGeneration = value.RelayGeneration
		}, code: SyncErrorConflict, field: "relay_generation"},
		{name: "admin public key", edit: func(value *SyncAuthority) { value.AdminPublicKey[0]++ }, code: SyncErrorConflict, field: "admin_public_key"},
		{name: "membership rollback", edit: func(value *SyncAuthority) { value.MembershipGeneration-- }, code: SyncErrorConflict, field: "membership_generation"},
		{name: "missing environment", edit: func(value *SyncAuthority) { value.Environments = value.Environments[:1] }, code: SyncErrorConflict, field: "environments"},
		{name: "changed environment", edit: func(value *SyncAuthority) { value.Environments[0].CertificateID[0]++ }, code: SyncErrorConflict, field: "environment"},
		{name: "retirement change", edit: func(value *SyncAuthority) { value.Environments[0].Retirement.RetirementID[0]++ }, code: SyncErrorConflict, field: "retirement"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneSyncAuthority(authority)
			test.edit(&candidate)
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, candidate); err == nil {
				t.Fatal("InstallVerifiedSyncAuthority() error = nil, want refusal")
			} else {
				assertSyncErrorCode(t, err, test.code)
				var problem *SyncError
				if !errors.As(err, &problem) || problem.Field != test.field {
					t.Fatalf("error = %v, want field %q", err, test.field)
				}
			}
		})
	}

	advanced := cloneSyncAuthority(authority)
	advanced.MembershipGeneration++
	advanced.Environments = append(advanced.Environments, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-c",
		CertificateID:            testAuthorityDigest(0x53),
		CertificateBytes:         []byte("environment-c-certificate"),
		Mode:                     SyncEnvironmentTrusted,
		ExpiresAtMillis:          0,
		JoinMembershipGeneration: 4,
	})
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, advanced); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(membership advance) error = %v", err)
	}
	got, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(after membership advance) error = %v", err)
	}
	if got.MembershipGeneration != advanced.MembershipGeneration || len(got.Environments) != 3 {
		t.Fatalf("advanced authority = %#v", got)
	}
}

func TestContinuityStoreInstallsTerminalRetirementOnceOnMembershipAdvance(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-retirement-transition")
	projectID := continuity.ProjectID("project-authority-retirement-transition")
	initial := testSyncAuthority()
	initial.MembershipGeneration = 2
	initial.Environments[0].Retirement = nil
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, initial); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(initial) error = %v", err)
	}

	sameMembership := cloneSyncAuthority(initial)
	sameMembership.Environments[0].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:          initial.RelayGeneration,
		MembershipGeneration:     initial.MembershipGeneration,
		FinalEnvironmentSequence: 3,
		FinalEnvelopeDigest:      testAuthorityDigest(0x52),
		RetirementID:             testAuthorityDigest(0x53),
		RetirementBytes:          []byte("environment-a-retirement"),
	}
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, sameMembership); err == nil {
		t.Fatal("InstallVerifiedSyncAuthority(same membership retirement) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
		var problem *SyncError
		if !errors.As(err, &problem) || problem.Field != "retirement" {
			t.Fatalf("same membership retirement error = %v, want retirement conflict", err)
		}
	}

	retired := cloneSyncAuthority(sameMembership)
	retired.MembershipGeneration++
	retired.Environments[0].Retirement.MembershipGeneration = retired.MembershipGeneration
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, retired); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(retirement transition) error = %v", err)
	}
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, retired); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(retirement retry) error = %v", err)
	}
	got, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(after retirement transition) error = %v", err)
	}
	if !syncAuthorityEqual(got, retired) {
		t.Fatalf("authority after retirement transition = %#v, want %#v", got, retired)
	}

	rollback := cloneSyncAuthority(retired)
	rollback.Environments[0].Retirement = nil
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, rollback); err == nil {
		t.Fatal("InstallVerifiedSyncAuthority(retirement rollback) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
	changed := cloneSyncAuthority(retired)
	changed.Environments[0].Retirement.RetirementID[0]++
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, changed); err == nil {
		t.Fatal("InstallVerifiedSyncAuthority(retirement change) error = nil")
	} else {
		assertSyncErrorCode(t, err, SyncErrorConflict)
	}
}

func TestContinuityStoreRejectsInvalidSyncAuthorityBeforeAnyRowsCommit(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-validation")
	projectID := continuity.ProjectID("project-authority-validation")
	base := testSyncAuthority()
	tests := []struct {
		name string
		edit func(*SyncAuthority)
	}{
		{name: "zero channel", edit: func(value *SyncAuthority) { value.ChannelID = SyncChannelID{} }},
		{name: "zero relay generation", edit: func(value *SyncAuthority) { value.RelayGeneration = [32]byte{} }},
		{name: "zero admin key", edit: func(value *SyncAuthority) { value.AdminPublicKey = [32]byte{} }},
		{name: "zero membership", edit: func(value *SyncAuthority) { value.MembershipGeneration = 0 }},
		{name: "empty inventory", edit: func(value *SyncAuthority) { value.Environments = nil }},
		{name: "duplicate environment", edit: func(value *SyncAuthority) { value.Environments = append(value.Environments, value.Environments[1]) }},
		{name: "duplicate certificate id", edit: func(value *SyncAuthority) {
			value.Environments[1].CertificateID = value.Environments[0].CertificateID
		}},
		{name: "reordered environment", edit: func(value *SyncAuthority) {
			value.Environments[0], value.Environments[1] = value.Environments[1], value.Environments[0]
		}},
		{name: "zero certificate id", edit: func(value *SyncAuthority) { value.Environments[0].CertificateID = [32]byte{} }},
		{name: "empty certificate", edit: func(value *SyncAuthority) { value.Environments[0].CertificateBytes = nil }},
		{name: "invalid mode", edit: func(value *SyncAuthority) { value.Environments[0].Mode = SyncEnvironmentMode("unknown") }},
		{name: "trusted expiry", edit: func(value *SyncAuthority) { value.Environments[0].ExpiresAtMillis = 1 }},
		{name: "ephemeral expiry", edit: func(value *SyncAuthority) {
			value.Environments[1].Mode = SyncEnvironmentEphemeral
			value.Environments[1].ExpiresAtMillis = 0
		}},
		{name: "join membership", edit: func(value *SyncAuthority) {
			value.Environments[0].JoinMembershipGeneration = value.MembershipGeneration + 1
		}},
		{name: "retirement relay generation", edit: func(value *SyncAuthority) { value.Environments[0].Retirement.RelayGeneration[0]++ }},
		{name: "retirement digest", edit: func(value *SyncAuthority) { value.Environments[0].Retirement.FinalEnvironmentSequence = 0 }},
		{name: "retirement id", edit: func(value *SyncAuthority) { value.Environments[0].Retirement.RetirementID = [32]byte{} }},
		{name: "retirement bytes", edit: func(value *SyncAuthority) { value.Environments[0].Retirement.RetirementBytes = nil }},
		{name: "duplicate membership generation", edit: func(value *SyncAuthority) {
			value.Environments[1].JoinMembershipGeneration = value.Environments[0].Retirement.MembershipGeneration
		}},
		{name: "gapped membership generation", edit: func(value *SyncAuthority) {
			value.MembershipGeneration++
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := cloneSyncAuthority(base)
			test.edit(&candidate)
			if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, candidate); err == nil {
				t.Fatal("InstallVerifiedSyncAuthority() error = nil, want invalid input refusal")
			} else {
				assertSyncErrorCode(t, err, SyncErrorInvalid)
			}
			if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
				t.Fatal("invalid install created sync authority")
			} else {
				assertSyncErrorCode(t, err, SyncErrorNotFound)
			}
		})
	}
}

func TestContinuityStoreRejectsCorruptFixedWidthChannelIdentity(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-corrupt-channel")
	projectID := continuity.ProjectID("project-authority-corrupt-channel")
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority()); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatalf("enable corruption fixture: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_projects
SET channel_id = ?
WHERE project_id = ?`, make([]byte, 31), string(projectID)); err != nil {
		t.Fatalf("corrupt channel identity: %v", err)
	}

	if _, err := store.CurrentSyncProgress(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncProgress() error = nil, want corrupt channel refusal")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
	if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthority() error = nil, want corrupt channel refusal")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func TestContinuityStoreRejectsCorruptRetirementMembershipBeforeNarrowing(t *testing.T) {
	t.Parallel()

	store := openSyncStore(t, "authority-corrupt-retirement-membership")
	projectID := continuity.ProjectID("project-authority-corrupt-retirement-membership")
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, testSyncAuthority()); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority() error = %v", err)
	}
	if _, err := store.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatalf("enable corruption fixture: %v", err)
	}
	if _, err := store.db.Exec(`
UPDATE continuity_sync_environment_certificates
SET retirement_membership_generation = 4294967299
WHERE project_id = ? AND environment_id = 'environment-a'`, string(projectID)); err != nil {
		t.Fatalf("corrupt retirement membership: %v", err)
	}

	if _, err := store.CurrentSyncAuthority(context.Background(), projectID); err == nil {
		t.Fatal("CurrentSyncAuthority() error = nil, want corrupt retirement refusal")
	} else {
		assertSyncErrorCode(t, err, SyncErrorStore)
	}
}

func testSyncAuthority() SyncAuthority {
	relayGeneration := testAuthorityDigest(0x11)
	return SyncAuthority{
		ChannelID:            testSyncChannelID("channel-a"),
		RelayGeneration:      relayGeneration,
		AdminPublicKey:       testAuthorityDigest(0x22),
		MembershipGeneration: 3,
		Environments: []SyncEnvironmentCertificate{
			{
				EnvironmentID:            "environment-a",
				CertificateID:            testAuthorityDigest(0x31),
				CertificateBytes:         []byte("environment-a-certificate"),
				Mode:                     SyncEnvironmentTrusted,
				ExpiresAtMillis:          0,
				JoinMembershipGeneration: 1,
				Retirement: &SyncEnvironmentRetirement{
					RelayGeneration:          relayGeneration,
					MembershipGeneration:     3,
					FinalEnvironmentSequence: 3,
					FinalEnvelopeDigest:      testAuthorityDigest(0x32),
					RetirementID:             testAuthorityDigest(0x33),
					RetirementBytes:          []byte("environment-a-retirement"),
				},
			},
			{
				EnvironmentID:            "environment-b",
				CertificateID:            testAuthorityDigest(0x41),
				CertificateBytes:         []byte("environment-b-certificate"),
				Mode:                     SyncEnvironmentEphemeral,
				ExpiresAtMillis:          1_000,
				JoinMembershipGeneration: 2,
			},
		},
	}
}

func installTestSyncAuthority(t *testing.T, store *Store, projectID continuity.ProjectID, channelID SyncChannelID) {
	t.Helper()
	authority := testActiveSyncAuthority()
	authority.ChannelID = channelID
	if _, err := store.InstallVerifiedSyncAuthority(context.Background(), projectID, authority); err != nil {
		t.Fatalf("InstallVerifiedSyncAuthority(%x) error = %v", channelID, err)
	}
}

func testActiveSyncAuthority() SyncAuthority {
	relayGeneration := testAuthorityDigest(0x11)
	return SyncAuthority{
		ChannelID:            testSyncChannelID("channel-a"),
		RelayGeneration:      relayGeneration,
		AdminPublicKey:       testAuthorityDigest(0x22),
		MembershipGeneration: 3,
		Environments: []SyncEnvironmentCertificate{
			{
				EnvironmentID:            "environment-a",
				CertificateID:            testSyncCertificateID("environment-a"),
				CertificateBytes:         []byte("environment-a-certificate"),
				Mode:                     SyncEnvironmentTrusted,
				JoinMembershipGeneration: 1,
			},
			{
				EnvironmentID:            "environment-b",
				CertificateID:            testSyncCertificateID("environment-b"),
				CertificateBytes:         []byte("environment-b-certificate"),
				Mode:                     SyncEnvironmentEphemeral,
				ExpiresAtMillis:          10_000,
				JoinMembershipGeneration: 2,
			},
			{
				EnvironmentID:            "environment-local",
				CertificateID:            sha256.Sum256([]byte("local certificate")),
				CertificateBytes:         []byte("environment-local-certificate"),
				Mode:                     SyncEnvironmentTrusted,
				JoinMembershipGeneration: 3,
			},
		},
	}
}

func testSyncChannelID(label string) SyncChannelID {
	return SyncChannelID(sha256.Sum256([]byte(label)))
}

func testAuthorityDigest(value byte) [32]byte {
	return sha256.Sum256([]byte{value})
}

func testSyncCertificateID(environmentID string) [32]byte {
	return sha256.Sum256([]byte("certificate:" + environmentID))
}

func cloneSyncAuthority(value SyncAuthority) SyncAuthority {
	clone := value
	clone.Environments = make([]SyncEnvironmentCertificate, len(value.Environments))
	for index, environment := range value.Environments {
		clone.Environments[index] = environment
		environmentBytes := append([]byte(nil), environment.CertificateBytes...)
		clone.Environments[index].CertificateBytes = environmentBytes
		if environment.Retirement != nil {
			retirement := *environment.Retirement
			retirement.RetirementBytes = append([]byte(nil), environment.Retirement.RetirementBytes...)
			clone.Environments[index].Retirement = &retirement
		}
	}
	return clone
}

func syncAuthorityEqual(left, right SyncAuthority) bool {
	if left.ChannelID != right.ChannelID || left.RelayGeneration != right.RelayGeneration ||
		left.AdminPublicKey != right.AdminPublicKey || left.MembershipGeneration != right.MembershipGeneration ||
		len(left.Environments) != len(right.Environments) {
		return false
	}
	for index, environment := range left.Environments {
		other := right.Environments[index]
		if environment.EnvironmentID != other.EnvironmentID || environment.CertificateID != other.CertificateID ||
			!bytes.Equal(environment.CertificateBytes, other.CertificateBytes) || environment.Mode != other.Mode ||
			environment.ExpiresAtMillis != other.ExpiresAtMillis ||
			environment.JoinMembershipGeneration != other.JoinMembershipGeneration {
			return false
		}
		if (environment.Retirement == nil) != (other.Retirement == nil) {
			return false
		}
		if environment.Retirement != nil {
			leftRetirement, rightRetirement := environment.Retirement, other.Retirement
			if leftRetirement.RelayGeneration != rightRetirement.RelayGeneration ||
				leftRetirement.MembershipGeneration != rightRetirement.MembershipGeneration ||
				leftRetirement.FinalEnvironmentSequence != rightRetirement.FinalEnvironmentSequence ||
				leftRetirement.FinalEnvelopeDigest != rightRetirement.FinalEnvelopeDigest ||
				leftRetirement.RetirementID != rightRetirement.RetirementID ||
				!bytes.Equal(leftRetirement.RetirementBytes, rightRetirement.RetirementBytes) {
				return false
			}
		}
	}
	return true
}
