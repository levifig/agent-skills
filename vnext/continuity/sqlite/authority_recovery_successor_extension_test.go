package sqlite

import (
	"context"
	"crypto/sha256"
	"sort"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestSyncAuthorityRecoveryReadySuccessorRejectsNonMonotonicPredecessorExtension(t *testing.T) {
	tests := []struct {
		name        string
		predecessor func() ([]SyncEnvironmentCertificate, uint32)
		mutate      func([]SyncEnvironmentCertificate) []SyncEnvironmentCertificate
	}{
		{
			name: "omitted participant replaced at predecessor generation",
			mutate: func(environments []SyncEnvironmentCertificate) []SyncEnvironmentCertificate {
				environments = environments[1:]
				environments = append(environments, SyncEnvironmentCertificate{
					EnvironmentID:            "environment-attacker",
					CertificateID:            sha256.Sum256([]byte("recovery-extension-attacker")),
					CertificateBytes:         []byte("recovery extension attacker certificate"),
					Mode:                     SyncEnvironmentTrusted,
					JoinMembershipGeneration: 1,
				})
				return environments
			},
		},
		{
			name: "modified predecessor certificate",
			mutate: func(environments []SyncEnvironmentCertificate) []SyncEnvironmentCertificate {
				environments[0].CertificateBytes = []byte("modified predecessor certificate")
				return environments
			},
		},
		{
			name: "substituted predecessor membership events",
			mutate: func(environments []SyncEnvironmentCertificate) []SyncEnvironmentCertificate {
				environments[0].JoinMembershipGeneration, environments[1].JoinMembershipGeneration =
					environments[1].JoinMembershipGeneration, environments[0].JoinMembershipGeneration
				return environments
			},
		},
		{
			name: "modified terminal retirement",
			predecessor: func() ([]SyncEnvironmentCertificate, uint32) {
				environments := syncAuthorityCandidateManyEnvironmentsV2(2)
				environments[0].Retirement = &SyncEnvironmentRetirement{
					RelayGeneration:      syncAuthorityCandidateBootstrapSnapshotV2(3).RelayGeneration,
					MembershipGeneration: 3,
					RetirementID:         sha256.Sum256([]byte("recovery-extension-predecessor-retirement")),
					RetirementBytes:      []byte("recovery extension predecessor retirement"),
				}
				return environments, 3
			},
			mutate: func(environments []SyncEnvironmentCertificate) []SyncEnvironmentCertificate {
				environments[0].Retirement.RetirementBytes = []byte("modified terminal retirement")
				return environments
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			store := openSyncStore(t, "recovery-successor-extension-"+test.name)
			projectID := continuity.ProjectID("project-recovery-successor-extension")
			predecessorEnvironments := syncAuthorityCandidateManyEnvironmentsV2(2)
			predecessorMembership := uint32(2)
			if test.predecessor != nil {
				predecessorEnvironments, predecessorMembership = test.predecessor()
			}
			predecessorSnapshot, predecessor := stageReadyRecoveryExtensionCandidateV1(
				t, store, projectID, predecessorEnvironments, predecessorMembership,
			)
			if _, err := store.AdvanceSyncRelayWatermark(
				context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7),
			); err != nil {
				t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
			}

			writer := recoveryExtensionWriterV1(predecessorMembership + 1)
			successorEnvironments := test.mutate(cloneSyncAuthorityCandidateEnvironmentsV2(predecessorEnvironments))
			successorEnvironments = append(successorEnvironments, writer)
			sortSyncAuthorityRecoveryEnvironmentsV1(successorEnvironments)
			start := syncAuthorityRecoveryStartV1(
				predecessor, writer, 7, predecessorMembership+1,
			)
			if _, err := store.BeginSyncAuthorityRecoveryTransition(
				context.Background(), projectID, start,
				syncAuthorityCandidatePageV2("", successorEnvironments, false),
			); err == nil {
				t.Fatal("BeginSyncAuthorityRecoveryTransition(non-monotonic READY successor) error = nil")
			}
			if _, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID); err != nil || found {
				t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after refusal) = (_, %v, %v)", found, err)
			}
			current, found, err := store.CurrentSyncAuthorityCandidate(context.Background(), projectID)
			if err != nil || !found || current != predecessor {
				t.Fatalf("CurrentSyncAuthorityCandidate(after refusal) = (%#v, %v, %v), want predecessor", current, found, err)
			}
		})
	}
}

func TestSyncAuthorityRecoveryReadySuccessorAllowsMonotonicRetirementAndJoin(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-extension-valid")
	projectID := continuity.ProjectID("project-recovery-successor-extension-valid")
	predecessorEnvironments := syncAuthorityCandidateManyEnvironmentsV2(2)
	predecessorSnapshot, predecessor := stageReadyRecoveryExtensionCandidateV1(
		t, store, projectID, predecessorEnvironments, 2,
	)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 9),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}

	writer := recoveryExtensionWriterV1(3)
	successorEnvironments := cloneSyncAuthorityCandidateEnvironmentsV2(predecessorEnvironments)
	successorEnvironments[0].Retirement = &SyncEnvironmentRetirement{
		RelayGeneration:      predecessorSnapshot.RelayGeneration,
		MembershipGeneration: 4,
		RetirementID:         sha256.Sum256([]byte("recovery-extension-appended-retirement")),
		RetirementBytes:      []byte("recovery extension appended retirement"),
	}
	successorEnvironments = append(successorEnvironments, writer, SyncEnvironmentCertificate{
		EnvironmentID:            "environment-new-after-writer",
		CertificateID:            sha256.Sum256([]byte("recovery-extension-new-after-writer")),
		CertificateBytes:         []byte("recovery extension new certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 5,
	})
	sortSyncAuthorityRecoveryEnvironmentsV1(successorEnvironments)
	start := syncAuthorityRecoveryStartV1(predecessor, writer, 9, 5)
	state, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start,
		syncAuthorityCandidatePageV2("", successorEnvironments, false),
	)
	if err != nil {
		t.Fatalf("BeginSyncAuthorityRecoveryTransition(valid extension) error = %v", err)
	}
	if !state.Successor.Ready {
		t.Fatalf("valid recovery successor = %#v, want READY", state.Successor)
	}
	current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
	if err != nil || !found || current != state {
		t.Fatalf("CurrentSyncAuthorityRecoverySuccessor() = (%#v, %v, %v), want (%#v, true, nil)", current, found, err, state)
	}
}

func TestSyncAuthorityRecoveryReadySuccessorExtensionProofRunsOnFinalAppendAndReplacement(t *testing.T) {
	t.Run("final append", func(t *testing.T) {
		store := openSyncStore(t, "recovery-successor-extension-final-append")
		projectID := continuity.ProjectID("project-recovery-successor-extension-final-append")
		predecessorEnvironments := syncAuthorityCandidateManyEnvironmentsV2(2)
		predecessorSnapshot, predecessor := stageReadyRecoveryExtensionCandidateV1(
			t, store, projectID, predecessorEnvironments, 2,
		)
		if _, err := store.AdvanceSyncRelayWatermark(
			context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7),
		); err != nil {
			t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
		}
		writer := recoveryExtensionWriterV1(3)
		successorEnvironments := cloneSyncAuthorityCandidateEnvironmentsV2(predecessorEnvironments)
		successorEnvironments[0].CertificateBytes = []byte("modified predecessor certificate on final append")
		successorEnvironments = append(successorEnvironments, writer)
		sortSyncAuthorityRecoveryEnvironmentsV1(successorEnvironments)
		start := syncAuthorityRecoveryStartV1(predecessor, writer, 7, 3)
		firstPage := syncAuthorityCandidatePageV2("", successorEnvironments[:2], true)
		staging, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
		if err != nil {
			t.Fatalf("BeginSyncAuthorityRecoveryTransition(staging) error = %v", err)
		}
		finalPage := syncAuthorityCandidatePageV2(firstPage.ThroughEnvironmentID, successorEnvironments[2:], false)
		if _, err := store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
			context.Background(), projectID, staging.Transition, staging.Successor.Checkpoint(), start.SuccessorSnapshot, finalPage,
		); err == nil {
			t.Fatal("AppendVerifiedSyncAuthorityRecoverySuccessorPage(non-monotonic READY successor) error = nil")
		}
		current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
		if err != nil || !found || current != staging {
			t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after refused final append) = (%#v, %v, %v), want staging", current, found, err)
		}
	})

	t.Run("final replacement", func(t *testing.T) {
		store := openSyncStore(t, "recovery-successor-extension-final-replacement")
		projectID := continuity.ProjectID("project-recovery-successor-extension-final-replacement")
		predecessorEnvironments := syncAuthorityCandidateManyEnvironmentsV2(2)
		predecessorSnapshot, predecessor := stageReadyRecoveryExtensionCandidateV1(
			t, store, projectID, predecessorEnvironments, 2,
		)
		watermark := syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 7)
		if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
			t.Fatalf("AdvanceSyncRelayWatermark(7) error = %v", err)
		}
		writer := recoveryExtensionWriterV1(3)
		validEnvironments := append(cloneSyncAuthorityCandidateEnvironmentsV2(predecessorEnvironments), writer)
		sortSyncAuthorityRecoveryEnvironmentsV1(validEnvironments)
		start := syncAuthorityRecoveryStartV1(predecessor, writer, 7, 3)
		firstPage := syncAuthorityCandidatePageV2("", validEnvironments[:2], true)
		stale, err := store.BeginSyncAuthorityRecoveryTransition(context.Background(), projectID, start, firstPage)
		if err != nil {
			t.Fatalf("BeginSyncAuthorityRecoveryTransition(stale staging) error = %v", err)
		}
		watermark.RelayHead = 8
		if _, err := store.AdvanceSyncRelayWatermark(context.Background(), watermark); err != nil {
			t.Fatalf("AdvanceSyncRelayWatermark(8) error = %v", err)
		}
		replacementSnapshot := start.SuccessorSnapshot
		replacementSnapshot.InventoryArrivalHead = 8
		invalidEnvironments := cloneSyncAuthorityCandidateEnvironmentsV2(validEnvironments)
		for index := range invalidEnvironments {
			if invalidEnvironments[index].EnvironmentID == predecessorEnvironments[0].EnvironmentID {
				invalidEnvironments[index].CertificateBytes = []byte("modified predecessor certificate on replacement")
			}
		}
		if _, err := store.ReplaceSyncAuthorityRecoverySuccessor(
			context.Background(), projectID, stale.Transition, stale.Successor.Checkpoint(), replacementSnapshot,
			syncAuthorityCandidatePageV2("", invalidEnvironments, false),
		); err == nil {
			t.Fatal("ReplaceSyncAuthorityRecoverySuccessor(non-monotonic READY successor) error = nil")
		}
		current, found, err := store.CurrentSyncAuthorityRecoverySuccessor(context.Background(), projectID)
		if err != nil || !found || current != stale {
			t.Fatalf("CurrentSyncAuthorityRecoverySuccessor(after refused replacement) = (%#v, %v, %v), want stale", current, found, err)
		}
	})
}

func TestSyncAuthorityRecoveryReadySuccessorRejectsWriterAlreadyInPredecessor(t *testing.T) {
	store := openSyncStore(t, "recovery-successor-extension-existing-writer")
	projectID := continuity.ProjectID("project-recovery-successor-extension-existing-writer")
	writer := recoveryExtensionWriterV1(2)
	predecessorEnvironments := []SyncEnvironmentCertificate{
		{
			EnvironmentID:            "environment-before-local",
			CertificateID:            sha256.Sum256([]byte("recovery-extension-before-local")),
			CertificateBytes:         []byte("recovery extension before local certificate"),
			Mode:                     SyncEnvironmentTrusted,
			JoinMembershipGeneration: 1,
		},
		writer,
	}
	sortSyncAuthorityRecoveryEnvironmentsV1(predecessorEnvironments)
	predecessorSnapshot, predecessor := stageReadyRecoveryExtensionCandidateV1(
		t, store, projectID, predecessorEnvironments, 2,
	)
	if _, err := store.AdvanceSyncRelayWatermark(
		context.Background(), syncRelayWatermarkFromSnapshot(projectID, predecessorSnapshot, 6),
	); err != nil {
		t.Fatalf("AdvanceSyncRelayWatermark() error = %v", err)
	}
	successorEnvironments := append(cloneSyncAuthorityCandidateEnvironmentsV2(predecessorEnvironments), SyncEnvironmentCertificate{
		EnvironmentID:            "environment-new-member",
		CertificateID:            sha256.Sum256([]byte("recovery-extension-new-member")),
		CertificateBytes:         []byte("recovery extension new member certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: 3,
	})
	sortSyncAuthorityRecoveryEnvironmentsV1(successorEnvironments)
	start := syncAuthorityRecoveryStartV1(predecessor, writer, 6, 3)
	if _, err := store.BeginSyncAuthorityRecoveryTransition(
		context.Background(), projectID, start,
		syncAuthorityCandidatePageV2("", successorEnvironments, false),
	); err == nil {
		t.Fatal("BeginSyncAuthorityRecoveryTransition(writer already in predecessor) error = nil")
	}
}

func stageReadyRecoveryExtensionCandidateV1(
	t *testing.T,
	store *Store,
	projectID continuity.ProjectID,
	environments []SyncEnvironmentCertificate,
	membershipGeneration uint32,
) (SyncAuthoritySnapshot, SyncAuthorityCandidate) {
	t.Helper()
	snapshot := syncAuthorityCandidateBootstrapSnapshotV2(int(membershipGeneration))
	candidate, err := store.StageVerifiedSyncAuthorityCandidatePage(
		context.Background(), projectID, snapshot,
		syncAuthorityCandidatePageV2("", environments, false),
	)
	if err != nil {
		t.Fatalf("StageVerifiedSyncAuthorityCandidatePage(predecessor) error = %v", err)
	}
	if !candidate.Ready {
		t.Fatalf("predecessor = %#v, want READY", candidate)
	}
	return snapshot, candidate
}

func recoveryExtensionWriterV1(joinMembershipGeneration uint32) SyncEnvironmentCertificate {
	return SyncEnvironmentCertificate{
		EnvironmentID:            "environment-local",
		CertificateID:            sha256.Sum256([]byte("recovery-extension-local-writer")),
		CertificateBytes:         []byte("recovery extension local writer certificate"),
		Mode:                     SyncEnvironmentTrusted,
		JoinMembershipGeneration: joinMembershipGeneration,
	}
}

func sortSyncAuthorityRecoveryEnvironmentsV1(environments []SyncEnvironmentCertificate) {
	sort.Slice(environments, func(left, right int) bool {
		return environments[left].EnvironmentID < environments[right].EnvironmentID
	})
}
