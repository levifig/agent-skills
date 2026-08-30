package coordinator

import (
	"bytes"
	"context"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

// recoveryRegistrationGuard is the sealed pre-registration result. A nil
// candidate is valid only for the exact first-membership empty-channel case.
// It carries no owner authorization, bearer secret, or prepared credential.
type recoveryRegistrationGuard struct {
	targetMembershipGeneration uint32
	inventorySnapshot          relay.EnvironmentInventorySnapshot
	candidate                  *continuitysqlite.SyncAuthorityCandidate
}

func (recoveryRegistrationGuard) String() string {
	return "[REDACTED recovery registration guard]"
}

func (recoveryRegistrationGuard) GoString() string {
	return "coordinator.recoveryRegistrationGuard([REDACTED])"
}

// stageRecoveryRegistrationGuard proves the relay authority for a later
// registration atom. For target membership one, canonical/candidate absence is
// only an observation immediately before CreateChannel, not a local
// reservation; the registration atom must reclassify local state immediately
// before RegisterEnvironment. This guard may stage a resumable authority
// candidate, but never registers, promotes, or persists credential authority.
func (coordinator *Coordinator) stageRecoveryRegistrationGuard(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (recoveryRegistrationGuard, error) {
	if err := validatePreparationContext(ctx); err != nil {
		return recoveryRegistrationGuard{}, err
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return recoveryRegistrationGuard{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return recoveryRegistrationGuard{}, err
	}

	previousMembership := registration.targetMembershipGeneration - 1
	base, baseFound, err := coordinator.currentRecoveryRegistrationBase(ctx, expectedProjectID, recovery, previousMembership)
	if err != nil {
		return recoveryRegistrationGuard{}, err
	}

	if registration.targetMembershipGeneration == 1 {
		if baseFound {
			return recoveryRegistrationGuard{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		if _, found, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, expectedProjectID); err != nil {
			return recoveryRegistrationGuard{}, mapRecoveryRegistrationStoreError(ctx, err)
		} else if found {
			return recoveryRegistrationGuard{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		createdState, err := coordinator.createRecoveryChannel(ctx, recovery, crypto.AdminPublicKey(recovery.AdminSeed))
		if err != nil {
			return recoveryRegistrationGuard{}, err
		}
		if createdState.MembershipGeneration != 0 || createdState.Head != 0 {
			return recoveryRegistrationGuard{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		expectedEmptyMembership := uint32(0)
		result, err := coordinator.scanRecoveryInventory(
			ctx,
			recovery,
			recoveryOwnerAuthorization(recovery),
			crypto.AdminPublicKey(recovery.AdminSeed),
			coordinator.store.WriterEnvironmentID(),
			recoveryInventoryScanOptions{
				createdState:       &createdState,
				expectedMembership: &expectedEmptyMembership,
			},
		)
		if err != nil {
			return recoveryRegistrationGuard{}, err
		}
		if result.snapshot != (relay.EnvironmentInventorySnapshot{}) {
			return recoveryRegistrationGuard{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		return recoveryRegistrationGuard{
			targetMembershipGeneration: registration.targetMembershipGeneration,
			inventorySnapshot:          result.snapshot,
		}, nil
	}

	baseVersion, baseDigest := uint16(0), [32]byte{}
	if baseFound {
		baseVersion = base.AuthorityDigestVersion
		baseDigest = base.AuthorityDigest
	}
	var compatibleCandidate *continuitysqlite.SyncAuthorityCandidate
	if current, found, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, expectedProjectID); err != nil {
		return recoveryRegistrationGuard{}, mapRecoveryRegistrationStoreError(ctx, err)
	} else if found && !sameRecoveryRegistrationCandidateBase(current, expectedProjectID, recovery, previousMembership, baseVersion, baseDigest) {
		return recoveryRegistrationGuard{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	} else if found {
		currentCopy := current
		compatibleCandidate = &currentCopy
	}
	var firstRequestSnapshot *relay.EnvironmentInventorySnapshot
	if compatibleCandidate != nil {
		pinned := relay.EnvironmentInventorySnapshot{
			MembershipGeneration: compatibleCandidate.Snapshot.MembershipGeneration,
			ArrivalHead:          compatibleCandidate.Snapshot.InventoryArrivalHead,
		}
		firstRequestSnapshot = &pinned
	}

	var staged continuitysqlite.SyncAuthorityCandidate
	result, err := coordinator.scanRecoveryInventory(
		ctx,
		recovery,
		recoveryOwnerAuthorization(recovery),
		crypto.AdminPublicKey(recovery.AdminSeed),
		coordinator.store.WriterEnvironmentID(),
		recoveryInventoryScanOptions{
			expectedMembership:   &previousMembership,
			firstRequestSnapshot: firstRequestSnapshot,
			onPage: func(page verifiedRecoveryInventoryPage) error {
				environments := make([]continuitysqlite.SyncEnvironmentCertificate, 0, len(page.environments))
				for _, record := range page.environments {
					environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
					if err != nil {
						return err
					}
					environments = append(environments, environment)
				}
				snapshot := continuitysqlite.SyncAuthoritySnapshot{
					ChannelID:                  continuitysqlite.SyncChannelID(recovery.ChannelID),
					RelayGeneration:            [32]byte(recovery.RelayGeneration),
					AdminPublicKey:             [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
					MembershipGeneration:       page.snapshot.MembershipGeneration,
					InventoryArrivalHead:       page.snapshot.ArrivalHead,
					BaseAuthorityDigestVersion: baseVersion,
					BaseAuthorityDigest:        baseDigest,
				}
				candidate, err := coordinator.store.StageVerifiedSyncAuthorityCandidatePage(
					ctx,
					expectedProjectID,
					snapshot,
					continuitysqlite.SyncAuthorityPage{
						AfterEnvironmentID:   string(page.afterEnvironmentID),
						ThroughEnvironmentID: string(page.environments[len(page.environments)-1].EnvironmentID),
						Environments:         environments,
						More:                 page.more,
					},
				)
				if err != nil {
					return mapRecoveryRegistrationStoreError(ctx, err)
				}
				if !sameRecoveryRegistrationCandidateBase(candidate, expectedProjectID, recovery, previousMembership, baseVersion, baseDigest) ||
					candidate.Snapshot.InventoryArrivalHead != page.snapshot.ArrivalHead {
					return newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
				}
				staged = candidate
				return nil
			},
		},
	)
	if err != nil {
		return recoveryRegistrationGuard{}, err
	}
	if !staged.Ready || staged.Snapshot.MembershipGeneration != previousMembership ||
		staged.Snapshot.InventoryArrivalHead != result.snapshot.ArrivalHead {
		return recoveryRegistrationGuard{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	sealedCandidate := staged
	return recoveryRegistrationGuard{
		targetMembershipGeneration: registration.targetMembershipGeneration,
		inventorySnapshot:          result.snapshot,
		candidate:                  &sealedCandidate,
	}, nil
}

func (coordinator *Coordinator) validateRecoveryRegistrationBinding(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) error {
	if expectedProjectID.Validate() != nil || recovery.Validate() != nil || expectedProjectID != recovery.ProjectID ||
		len(recovery.LastSignedCheckpoint) != 0 || coordinator.remote.Endpoint() != recovery.RelayURL {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	environment := registration.environment
	if writerEnvironmentID.Validate() != nil {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	}
	if registration.targetMembershipGeneration == 0 ||
		registration.targetMembershipGeneration != environment.MembershipGeneration ||
		registration.certificateID != environment.CertificateID ||
		environment.ChannelID != relay.ChannelID(recovery.ChannelID) ||
		environment.EnvironmentID != relay.EnvironmentID(writerEnvironmentID) ||
		environment.Mode != relay.TrustedEnvironment || environment.ExpiresAtMillis != 0 ||
		environment.RelayTokenExpiresAtMillis != 0 || environment.Validate() != nil {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	certificate, err := protocol.ParseEnvironmentCertificate(environment.CertificateBytes)
	if err != nil {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	canonicalBytes, err := certificate.MarshalBinary()
	if err != nil || !bytes.Equal(canonicalBytes, environment.CertificateBytes) ||
		relay.Digest(protocol.CertificateID(certificate)) != registration.certificateID ||
		crypto.VerifyEnvironmentCertificate(certificate, crypto.AdminPublicKey(recovery.AdminSeed)) != nil ||
		certificate.Version != protocol.CertificateVersionV1 || certificate.ProtocolVersion != protocol.ProtocolVersionV1 ||
		certificate.CipherSuite != protocol.CipherSuiteXChaCha20Poly1305 || certificate.ProjectID != expectedProjectID ||
		certificate.ChannelID != recovery.ChannelID || certificate.EnvironmentID != writerEnvironmentID ||
		certificate.Mode != protocol.EnvironmentTrusted || certificate.ExpiresAtMillis != 0 ||
		certificate.MembershipGeneration != registration.targetMembershipGeneration ||
		len(certificate.AllowedKeyGenerations) != 1 || certificate.AllowedKeyGenerations[0] != recovery.WriteGeneration {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (coordinator *Coordinator) currentRecoveryRegistrationBase(
	ctx context.Context,
	projectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	maximumMembership uint32,
) (continuitysqlite.SyncAuthorityBinding, bool, error) {
	binding, err := coordinator.store.CurrentSyncAuthorityBinding(ctx, projectID)
	if err != nil {
		if contextErr := contextError(ctx, err); contextErr != nil {
			return continuitysqlite.SyncAuthorityBinding{}, false, contextErr
		}
		var syncErr *continuitysqlite.SyncError
		if errors.As(err, &syncErr) && syncErr.Code == continuitysqlite.SyncErrorNotFound {
			return continuitysqlite.SyncAuthorityBinding{}, false, nil
		}
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryRegistrationStoreError(ctx, err)
	}
	if binding.ChannelID != continuitysqlite.SyncChannelID(recovery.ChannelID) ||
		binding.RelayGeneration != [32]byte(recovery.RelayGeneration) ||
		binding.AdminPublicKey != [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) ||
		binding.MembershipGeneration > maximumMembership {
		return continuitysqlite.SyncAuthorityBinding{}, false, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	return binding, true, nil
}

func sameRecoveryRegistrationCandidateBase(
	candidate continuitysqlite.SyncAuthorityCandidate,
	projectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	membershipGeneration uint32,
	baseVersion uint16,
	baseDigest [32]byte,
) bool {
	return candidate.ProjectID == projectID &&
		candidate.Snapshot.ChannelID == continuitysqlite.SyncChannelID(recovery.ChannelID) &&
		candidate.Snapshot.RelayGeneration == [32]byte(recovery.RelayGeneration) &&
		candidate.Snapshot.AdminPublicKey == [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) &&
		candidate.Snapshot.MembershipGeneration == membershipGeneration &&
		candidate.Snapshot.BaseAuthorityDigestVersion == baseVersion &&
		candidate.Snapshot.BaseAuthorityDigest == baseDigest
}

func syncEnvironmentCertificateFromRecoveryInventory(record relay.EnvironmentInventoryRecord) (continuitysqlite.SyncEnvironmentCertificate, error) {
	var mode continuitysqlite.SyncEnvironmentMode
	switch record.Mode {
	case relay.TrustedEnvironment:
		mode = continuitysqlite.SyncEnvironmentTrusted
	case relay.EphemeralEnvironment:
		mode = continuitysqlite.SyncEnvironmentEphemeral
	default:
		return continuitysqlite.SyncEnvironmentCertificate{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	environment := continuitysqlite.SyncEnvironmentCertificate{
		EnvironmentID:            string(record.EnvironmentID),
		CertificateID:            [32]byte(record.CertificateID),
		CertificateBytes:         append([]byte(nil), record.CertificateBytes...),
		Mode:                     mode,
		ExpiresAtMillis:          record.ExpiresAtMillis,
		JoinMembershipGeneration: record.MembershipGeneration,
	}
	if record.Retirement != nil {
		retirement := record.Retirement
		environment.Retirement = &continuitysqlite.SyncEnvironmentRetirement{
			RelayGeneration:          [32]byte(retirement.RelayGeneration),
			MembershipGeneration:     retirement.MembershipGeneration,
			FinalEnvironmentSequence: retirement.FinalEnvironmentSequence,
			FinalEnvelopeDigest:      [32]byte(retirement.FinalEnvelopeDigest),
			RetirementID:             [32]byte(retirement.RetirementID),
			RetirementBytes:          append([]byte(nil), retirement.RetirementBytes...),
		}
	}
	return environment, nil
}

func mapRecoveryRegistrationStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorConflict:
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		return newProblem(CodeUnavailable, PhaseEnvironmentInventory, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
}
