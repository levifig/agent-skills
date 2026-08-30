package sqlite

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncAuthorityCandidateCodecV2VectorsAndRepagingInvariance(t *testing.T) {
	projectID := continuity.ProjectID("project:authority-candidate-codec")
	snapshot := syncAuthorityCandidateTestSnapshotV2(5)
	environments := syncAuthorityCandidateTestEnvironmentsV2(5)

	candidateID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
	if err != nil {
		t.Fatalf("deriveSyncAuthorityCandidateIdentityV2() error = %v", err)
	}
	if got, want := hex.EncodeToString(candidateID[:]), "5f4f0c61f03fbea4b47f030e19b96e3fd6d03b9dd6f9868ab1a21815edd459a7"; got != want {
		t.Errorf("candidate ID = %s, want %s", got, want)
	}
	if got, want := hex.EncodeToString(headerDigest[:]), "0bba4fdf49ce890b5dd26a7fa1b296d1c2e3bfaffe6ed6dac1c5a644680082da"; got != want {
		t.Errorf("header digest = %s, want %s", got, want)
	}

	var finalDigest [32]byte
	for _, pageSizes := range [][]int{{4, 1}, {2, 3}, {1, 1, 1, 1, 1}} {
		rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
		if err != nil {
			t.Fatalf("syncAuthorityCandidateRollingSeedV2() error = %v", err)
		}
		ordinal := int64(0)
		offset := 0
		for _, pageSize := range pageSizes {
			for _, environment := range environments[offset : offset+pageSize] {
				ordinal++
				rolling, _, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, ordinal, environment)
				if err != nil {
					t.Fatalf("advanceSyncAuthorityCandidateRollingV2() error = %v", err)
				}
			}
			offset += pageSize
		}
		got, err := finalizeSyncAuthorityDigestV2(headerDigest, ordinal, rolling)
		if err != nil {
			t.Fatalf("finalizeSyncAuthorityDigestV2() error = %v", err)
		}
		if finalDigest == ([32]byte{}) {
			finalDigest = got
		} else if got != finalDigest {
			t.Fatalf("final digest for page sizes %v = %x, want %x", pageSizes, got, finalDigest)
		}
	}
	if got, want := hex.EncodeToString(finalDigest[:]), "f5014ed8371d74e153e14edcba0549a3bde1d729584e493fb9987834c8ed5f75"; got != want {
		t.Errorf("authority digest = %s, want %s", got, want)
	}

	withoutBase := snapshot
	withoutBase.BaseAuthorityDigestVersion = 0
	withoutBase.BaseAuthorityDigest = [32]byte{}
	withoutBaseID, withoutBaseHeaderDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, withoutBase)
	if err != nil {
		t.Fatalf("derive without base error = %v", err)
	}
	if withoutBaseID == candidateID {
		t.Fatal("candidate identity did not bind base authority")
	}
	if withoutBaseHeaderDigest != headerDigest {
		t.Fatal("authority header digest unexpectedly bound base authority")
	}

	otherProjectID, otherHeaderDigest, err := deriveSyncAuthorityCandidateIdentityV2("project:other", snapshot)
	if err != nil {
		t.Fatalf("derive other project error = %v", err)
	}
	if otherProjectID == candidateID {
		t.Fatal("candidate identity did not bind project")
	}
	if otherHeaderDigest == headerDigest {
		t.Fatal("authority header digest did not bind project")
	}
	otherRolling, err := syncAuthorityCandidateRollingSeedV2(otherHeaderDigest)
	if err != nil {
		t.Fatalf("other rolling seed error = %v", err)
	}
	for index, environment := range environments {
		otherRolling, _, err = advanceSyncAuthorityCandidateRollingV2(otherHeaderDigest, otherRolling, int64(index+1), environment)
		if err != nil {
			t.Fatalf("other rolling advance error = %v", err)
		}
	}
	otherFinalDigest, err := finalizeSyncAuthorityDigestV2(otherHeaderDigest, int64(len(environments)), otherRolling)
	if err != nil {
		t.Fatalf("other final digest error = %v", err)
	}
	if otherFinalDigest == finalDigest {
		t.Fatal("authority digest did not bind project")
	}

	rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
	if err != nil {
		t.Fatalf("page vector rolling seed error = %v", err)
	}
	environmentDigests := make([][32]byte, 0, 2)
	for index, environment := range environments[:2] {
		var environmentDigest [32]byte
		rolling, environmentDigest, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, int64(index+1), environment)
		if err != nil {
			t.Fatalf("page vector rolling advance error = %v", err)
		}
		environmentDigests = append(environmentDigests, environmentDigest)
	}
	pageDigest, err := syncAuthorityCandidatePageDigestV2(candidateID, 1, SyncAuthorityPage{
		ThroughEnvironmentID: environments[1].EnvironmentID,
		Environments:         environments[:2],
		More:                 true,
	}, 2, rolling, environmentDigests)
	if err != nil {
		t.Fatalf("syncAuthorityCandidatePageDigestV2() error = %v", err)
	}
	if got, want := hex.EncodeToString(pageDigest[:]), "f411ff32f15a538ea39bc9ed3335c27f43713d2d13ede79e296b2a2d6d86016f"; got != want {
		t.Errorf("page digest = %s, want %s", got, want)
	}
}

func TestSyncAuthorityCandidateCodecV2RejectsMalformedAndOverflowInputs(t *testing.T) {
	projectID := continuity.ProjectID("project:authority-candidate-invalid")
	snapshot := syncAuthorityCandidateTestSnapshotV2(1)
	environment := syncAuthorityCandidateTestEnvironmentsV2(1)[0]
	candidateID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
	if err != nil {
		t.Fatalf("derive fixture error = %v", err)
	}
	rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
	if err != nil {
		t.Fatalf("seed fixture error = %v", err)
	}
	rolling, environmentDigest, err := advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, 1, environment)
	if err != nil {
		t.Fatalf("advance fixture error = %v", err)
	}

	snapshotCases := []struct {
		name      string
		projectID continuity.ProjectID
		mutate    func(*SyncAuthoritySnapshot)
	}{
		{name: "invalid project", projectID: "project with spaces"},
		{name: "zero channel", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.ChannelID = SyncChannelID{} }},
		{name: "zero relay", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.RelayGeneration = [32]byte{} }},
		{name: "zero admin", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.AdminPublicKey = [32]byte{} }},
		{name: "zero membership", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.MembershipGeneration = 0 }},
		{name: "negative head", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.InventoryArrivalHead = -1 }},
		{name: "version without digest", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.BaseAuthorityDigest = [32]byte{} }},
		{name: "digest without version", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.BaseAuthorityDigestVersion = 0 }},
		{name: "unsupported base version", projectID: projectID, mutate: func(value *SyncAuthoritySnapshot) { value.BaseAuthorityDigestVersion = 3 }},
	}
	for _, test := range snapshotCases {
		t.Run(test.name, func(t *testing.T) {
			value := snapshot
			if test.mutate != nil {
				test.mutate(&value)
			}
			if _, _, err := deriveSyncAuthorityCandidateIdentityV2(test.projectID, value); err == nil {
				t.Fatal("derive malformed snapshot error = nil")
			}
		})
	}

	if _, err := syncAuthorityCandidateRollingSeedV2([32]byte{}); err == nil {
		t.Fatal("zero header rolling seed error = nil")
	}
	if _, _, err := advanceSyncAuthorityCandidateRollingV2([32]byte{}, rolling, 1, environment); err == nil {
		t.Fatal("zero header rolling advance error = nil")
	}
	if _, _, err := advanceSyncAuthorityCandidateRollingV2(headerDigest, [32]byte{}, 1, environment); err == nil {
		t.Fatal("zero previous rolling advance error = nil")
	}
	if _, _, err := advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, 0, environment); err == nil {
		t.Fatal("zero ordinal rolling advance error = nil")
	}
	malformedEnvironment := environment
	malformedEnvironment.CertificateID = [32]byte{}
	if _, _, err := advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, 2, malformedEnvironment); err == nil {
		t.Fatal("malformed environment rolling advance error = nil")
	}
	if _, err := finalizeSyncAuthorityDigestV2([32]byte{}, 1, rolling); err == nil {
		t.Fatal("zero header final digest error = nil")
	}
	if _, err := finalizeSyncAuthorityDigestV2(headerDigest, 0, rolling); err == nil {
		t.Fatal("zero count final digest error = nil")
	}
	if _, err := finalizeSyncAuthorityDigestV2(headerDigest, 1, [32]byte{}); err == nil {
		t.Fatal("zero rolling final digest error = nil")
	}
	if _, err := syncAuthorityCandidatePageDigestV2([32]byte{}, 1, SyncAuthorityPage{}, 1, rolling, [][32]byte{environmentDigest}); err == nil {
		t.Fatal("zero candidate page digest error = nil")
	}
	if _, err := syncAuthorityCandidatePageDigestV2(candidateID, 0, SyncAuthorityPage{}, 1, rolling, [][32]byte{environmentDigest}); err == nil {
		t.Fatal("zero page number digest error = nil")
	}
	if _, err := syncAuthorityCandidatePageDigestV2(candidateID, 1, SyncAuthorityPage{}, 1, rolling, nil); err == nil {
		t.Fatal("empty page environment digest error = nil")
	}
	if _, err := syncAuthorityCandidatePageDigestV2(candidateID, 1, SyncAuthorityPage{}, 1, rolling, [][32]byte{{}}); err == nil {
		t.Fatal("zero page environment digest error = nil")
	}
	if _, err := checkedSyncAuthorityCandidateAdvanceV2(-1, 1); err == nil {
		t.Fatal("negative advance error = nil")
	}
	if _, err := checkedSyncAuthorityCandidateAdvanceV2(1, -1); err == nil {
		t.Fatal("negative delta error = nil")
	}
	if _, err := checkedSyncAuthorityCandidateAdvanceV2(math.MaxInt64, 1); err == nil {
		t.Fatal("overflow advance error = nil")
	}
}

func syncAuthorityCandidateTestSnapshotV2(membershipGeneration uint32) SyncAuthoritySnapshot {
	return SyncAuthoritySnapshot{
		ChannelID:                  SyncChannelID(sha256.Sum256([]byte("channel:v2"))),
		RelayGeneration:            sha256.Sum256([]byte("relay:v2")),
		AdminPublicKey:             sha256.Sum256([]byte("admin:v2")),
		MembershipGeneration:       membershipGeneration,
		InventoryArrivalHead:       42,
		BaseAuthorityDigestVersion: 1,
		BaseAuthorityDigest:        sha256.Sum256([]byte("base-authority:v1")),
	}
}

func syncAuthorityCandidateTestEnvironmentsV2(count int) []SyncEnvironmentCertificate {
	environments := make([]SyncEnvironmentCertificate, count)
	for index := range environments {
		name := "environment:" + string(rune('a'+index))
		environments[index] = SyncEnvironmentCertificate{
			EnvironmentID:            name,
			CertificateID:            sha256.Sum256([]byte("certificate:" + name)),
			CertificateBytes:         []byte("certificate bytes for " + name),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: uint32(index + 1),
		}
	}
	return environments
}
