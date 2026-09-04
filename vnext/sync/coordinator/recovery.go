package coordinator

import (
	"bytes"
	"context"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
	relayverifier "github.com/levifig/loaf/vnext/sync/relay/verifier"
)

// RecoveryPreparationOptions controls the only authorized relay mutation in
// recovery preparation.
type RecoveryPreparationOptions struct {
	CreateEmptyChannel bool
}

// PrepareRecovery validates recovery authority and performs a bounded preflight
// over one pinned relay inventory, then mints a prospective, unregistered
// trusted credential. The caller must durably protect its exact encoding before
// a later attach stages and validates candidate-wide authority coverage, and
// every retry after protection must reuse that credential rather than prepare a
// new one. This method never registers an environment or writes local
// persistence; it is not recovery or attach success.
func (coordinator *Coordinator) PrepareRecovery(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	options RecoveryPreparationOptions,
) (credential.TrustedProjectCredential, error) {
	if err := validatePreparationContext(ctx); err != nil {
		return credential.TrustedProjectCredential{}, err
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return credential.TrustedProjectCredential{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	if expectedProjectID.Validate() != nil || recovery.Validate() != nil {
		return credential.TrustedProjectCredential{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if expectedProjectID != recovery.ProjectID {
		return credential.TrustedProjectCredential{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}
	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	if writerEnvironmentID.Validate() != nil {
		return credential.TrustedProjectCredential{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	}
	if coordinator.remote.Endpoint() != recovery.RelayURL {
		return credential.TrustedProjectCredential{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}

	adminPublic := crypto.AdminPublicKey(recovery.AdminSeed)
	ownerAuthorization := recoveryOwnerAuthorization(recovery)
	var createdState *relay.ChannelState
	if options.CreateEmptyChannel {
		state, err := coordinator.createRecoveryChannel(ctx, recovery, adminPublic)
		if err != nil {
			return credential.TrustedProjectCredential{}, err
		}
		createdState = &state
	}

	membershipGeneration, err := coordinator.readRecoveryInventory(
		ctx,
		recovery,
		ownerAuthorization,
		adminPublic,
		writerEnvironmentID,
		createdState,
	)
	if err != nil {
		return credential.TrustedProjectCredential{}, err
	}
	if err := ctx.Err(); err != nil {
		return credential.TrustedProjectCredential{}, err
	}

	prepared, err := mintRecoveryCredential(recovery, adminPublic, writerEnvironmentID, membershipGeneration+1)
	if err != nil {
		return credential.TrustedProjectCredential{}, err
	}
	return prepared, nil
}

func validatePreparationContext(ctx context.Context) error {
	if ctx == nil {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func recoveryOwnerAuthorization(recovery credential.ProjectRecoveryCredential) relay.OwnerAuthorization {
	return relay.OwnerAuthorization{
		ChannelID:       relay.ChannelID(recovery.ChannelID),
		RelayGeneration: relay.RelayGeneration(recovery.RelayGeneration),
		TokenID:         relay.RelayTokenID(recovery.OwnerRelayAuthorization.ID()),
		TokenSecret:     relay.RelayTokenSecret(recovery.OwnerRelayAuthorization.Secret()),
	}
}

func (coordinator *Coordinator) createRecoveryChannel(
	ctx context.Context,
	recovery credential.ProjectRecoveryCredential,
	adminPublic protocol.PublicKey,
) (relay.ChannelState, error) {
	ownerHash, err := relay.HashTokenSecret(relay.RelayTokenSecret(recovery.OwnerRelayAuthorization.Secret()))
	if err != nil {
		return relay.ChannelState{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	channel := relay.Channel{
		ChannelID:       relay.ChannelID(recovery.ChannelID),
		RelayGeneration: relay.RelayGeneration(recovery.RelayGeneration),
		AdminPublicKey:  relay.PublicKey(adminPublic),
		OwnerToken: relay.TokenRegistration{
			TokenID:   relay.RelayTokenID(recovery.OwnerRelayAuthorization.ID()),
			TokenHash: ownerHash,
		},
	}
	state, err := coordinator.remote.CreateChannel(ctx, channel)
	if err != nil {
		return relay.ChannelState{}, mapCreateChannelError(ctx, err)
	}
	if state.ChannelID != channel.ChannelID || state.RelayGeneration != channel.RelayGeneration || state.Head < 0 ||
		(state.MembershipGeneration == 0 && state.Head != 0) {
		return relay.ChannelState{}, newProblem(CodeRemote, PhaseChannelCreation, ActionRestartRecovery)
	}
	return state, nil
}

func (coordinator *Coordinator) readRecoveryInventory(
	ctx context.Context,
	recovery credential.ProjectRecoveryCredential,
	ownerAuthorization relay.OwnerAuthorization,
	adminPublic protocol.PublicKey,
	writerEnvironmentID continuity.EnvironmentID,
	createdState *relay.ChannelState,
) (uint32, error) {
	result, err := coordinator.scanRecoveryInventory(
		ctx,
		recovery,
		ownerAuthorization,
		adminPublic,
		writerEnvironmentID,
		recoveryInventoryScanOptions{
			createdState:          createdState,
			requireNextMembership: true,
		},
	)
	if err != nil {
		return 0, err
	}
	return result.snapshot.MembershipGeneration, nil
}

type verifiedRecoveryInventoryPage struct {
	snapshot           relay.EnvironmentInventorySnapshot
	afterEnvironmentID relay.EnvironmentID
	environments       []relay.EnvironmentInventoryRecord
	more               bool
}

// recoveryInventoryExpectedLocalWriter contains only the public certificate
// identity needed to recognize the exact protected registration in a relay
// inventory. It deliberately excludes relay token identifiers and hashes.
type recoveryInventoryExpectedLocalWriter struct {
	certificateID        relay.Digest
	certificateBytes     []byte
	membershipGeneration uint32
}

type recoveryInventoryScanOptions struct {
	createdState                *relay.ChannelState
	expectedMembership          *uint32
	firstRequestSnapshot        *relay.EnvironmentInventorySnapshot
	firstAfterEnvironmentID     relay.EnvironmentID
	minimumMembershipGeneration uint32
	minimumArrivalHead          int64
	expectedLocalWriter         *recoveryInventoryExpectedLocalWriter
	requireNextMembership       bool
	onPage                      func(verifiedRecoveryInventoryPage) error
}

type recoveryInventoryScanResult struct {
	snapshot relay.EnvironmentInventorySnapshot
}

// scanRecoveryInventory reads one pinned inventory through bounded pages. It
// invokes onPage only after every object in that page has passed structural and
// cryptographic verification. Candidate-wide membership coverage remains the
// persistence layer's responsibility when a callback stages the verified page.
func (coordinator *Coordinator) scanRecoveryInventory(
	ctx context.Context,
	recovery credential.ProjectRecoveryCredential,
	ownerAuthorization relay.OwnerAuthorization,
	adminPublic protocol.PublicKey,
	writerEnvironmentID continuity.EnvironmentID,
	options recoveryInventoryScanOptions,
) (recoveryInventoryScanResult, error) {
	if options.firstAfterEnvironmentID != "" && options.firstRequestSnapshot == nil {
		return recoveryInventoryScanResult{}, newProblem(CodeInvalid, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	var (
		pinnedSnapshot relay.EnvironmentInventorySnapshot
		haveSnapshot   bool
		after          = options.firstAfterEnvironmentID
		eventCount     uint64
		writerSeen     bool
	)
	fullMembershipHistory := after == ""
	writerMustAppear := options.expectedLocalWriter != nil && relay.EnvironmentID(writerEnvironmentID) > after
	verifier := relayverifier.New()
	wantChannel := relay.ChannelAuthority{
		ChannelID:       relay.ChannelID(recovery.ChannelID),
		RelayGeneration: relay.RelayGeneration(recovery.RelayGeneration),
		AdminPublicKey:  relay.PublicKey(adminPublic),
	}

	for {
		if err := ctx.Err(); err != nil {
			return recoveryInventoryScanResult{}, err
		}
		pageStartAfter := after
		requestOwner := ownerAuthorization
		request := relay.EnvironmentInventoryRequest{
			Authorization:      relay.InventoryAuthorization{Owner: &requestOwner},
			AfterEnvironmentID: after,
			Limit:              relay.MaxEnvironmentInventoryPage,
		}
		if haveSnapshot {
			snapshot := pinnedSnapshot
			request.Snapshot = &snapshot
		} else if options.firstRequestSnapshot != nil {
			snapshot := *options.firstRequestSnapshot
			request.Snapshot = &snapshot
		}
		page, err := coordinator.remote.EnvironmentInventory(ctx, request)
		if err != nil {
			return recoveryInventoryScanResult{}, mapInventoryError(ctx, err)
		}
		if page.Channel != wantChannel || page.Snapshot.ArrivalHead < 0 || page.Snapshot.ArrivalHead < options.minimumArrivalHead ||
			page.Snapshot.MembershipGeneration < options.minimumMembershipGeneration ||
			len(page.Environments) > relay.MaxEnvironmentInventoryPage ||
			(page.More && len(page.Environments) != relay.MaxEnvironmentInventoryPage) {
			return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		if !haveSnapshot {
			pinnedSnapshot = page.Snapshot
			haveSnapshot = true
			if options.createdState != nil && (pinnedSnapshot.MembershipGeneration < options.createdState.MembershipGeneration ||
				pinnedSnapshot.ArrivalHead < options.createdState.Head) {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if options.requireNextMembership && pinnedSnapshot.MembershipGeneration == math.MaxUint32 {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if options.expectedMembership != nil && pinnedSnapshot.MembershipGeneration != *options.expectedMembership {
				return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if options.firstRequestSnapshot != nil && pinnedSnapshot != *options.firstRequestSnapshot {
				return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRetry)
			}
			if pinnedSnapshot.MembershipGeneration == 0 {
				if pinnedSnapshot.ArrivalHead != 0 || len(page.Environments) != 0 || page.More {
					return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
				}
				if options.createdState == nil {
					return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionAuthorizeEmptyChannel)
				}
				if options.expectedLocalWriter != nil {
					return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
				}
				return recoveryInventoryScanResult{snapshot: pinnedSnapshot}, nil
			}
		} else if page.Snapshot != pinnedSnapshot {
			return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		if len(page.Environments) == 0 {
			return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
		}

		pageMemberships := make(map[uint32]struct{}, len(page.Environments)*2)
		verifiedEnvironments := make([]relay.EnvironmentInventoryRecord, 0, len(page.Environments))
		for _, record := range page.Environments {
			if record.EnvironmentID <= after {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if writerMustAppear && !writerSeen && continuity.EnvironmentID(record.EnvironmentID) > writerEnvironmentID {
				return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			after = record.EnvironmentID
			if record.ProducerHead < 0 || record.ProducerHead > pinnedSnapshot.ArrivalHead {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if _, duplicate := pageMemberships[record.MembershipGeneration]; duplicate {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			pageMemberships[record.MembershipGeneration] = struct{}{}
			if record.MembershipGeneration == 0 || record.MembershipGeneration > pinnedSnapshot.MembershipGeneration {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}

			authority := relay.EnvironmentAuthority{
				ChannelAuthority:     page.Channel,
				EnvironmentID:        record.EnvironmentID,
				CertificateID:        record.CertificateID,
				CertificateBytes:     record.CertificateBytes,
				Mode:                 record.Mode,
				ExpiresAtMillis:      record.ExpiresAtMillis,
				MembershipGeneration: record.MembershipGeneration,
			}
			certificateAuthority := relay.EnvironmentCertificateAuthority(authority)
			if err := verifier.VerifyEnvironmentCertificate(ctx, certificateAuthority); err != nil {
				if contextErr := contextError(ctx, err); contextErr != nil {
					return recoveryInventoryScanResult{}, contextErr
				}
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			certificate, err := protocol.ParseEnvironmentCertificate(record.CertificateBytes)
			if err != nil || certificate.ProjectID != recovery.ProjectID {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if continuity.EnvironmentID(record.EnvironmentID) == writerEnvironmentID {
				if options.expectedLocalWriter == nil {
					return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionUseExistingCredential)
				}
				expected := options.expectedLocalWriter
				if record.CertificateID != expected.certificateID ||
					!bytes.Equal(record.CertificateBytes, expected.certificateBytes) ||
					record.Mode != relay.TrustedEnvironment || record.ExpiresAtMillis != 0 ||
					record.MembershipGeneration != expected.membershipGeneration || record.Retirement != nil {
					return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
				}
				writerSeen = true
			}
			eventCount++

			if record.Retirement != nil {
				retirement := record.Retirement
				if retirement.MembershipGeneration == 0 ||
					retirement.MembershipGeneration > pinnedSnapshot.MembershipGeneration ||
					retirement.MembershipGeneration <= record.MembershipGeneration ||
					record.ProducerHead != retirement.FinalEnvironmentSequence {
					return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
				}
				if _, duplicate := pageMemberships[retirement.MembershipGeneration]; duplicate {
					return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
				}
				pageMemberships[retirement.MembershipGeneration] = struct{}{}
				relayRetirement := relay.Retirement{
					ChannelID:                page.Channel.ChannelID,
					RelayGeneration:          retirement.RelayGeneration,
					EnvironmentID:            record.EnvironmentID,
					CertificateID:            retirement.CertificateID,
					MembershipGeneration:     retirement.MembershipGeneration,
					FinalEnvironmentSequence: retirement.FinalEnvironmentSequence,
					FinalEnvelopeDigest:      retirement.FinalEnvelopeDigest,
					RetirementID:             retirement.RetirementID,
					RetirementBytes:          retirement.RetirementBytes,
				}
				if err := verifier.VerifyRetirement(ctx, authority, relayRetirement); err != nil {
					if contextErr := contextError(ctx, err); contextErr != nil {
						return recoveryInventoryScanResult{}, contextErr
					}
					return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
				}
				eventCount++
			}
			if eventCount > uint64(pinnedSnapshot.MembershipGeneration) {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			verifiedEnvironments = append(verifiedEnvironments, cloneRecoveryInventoryRecord(record))
		}

		if !page.More {
			if fullMembershipHistory && eventCount != uint64(pinnedSnapshot.MembershipGeneration) {
				return recoveryInventoryScanResult{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
			if writerMustAppear && !writerSeen {
				return recoveryInventoryScanResult{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
			}
		}
		if options.onPage != nil {
			if err := options.onPage(verifiedRecoveryInventoryPage{
				snapshot:           pinnedSnapshot,
				afterEnvironmentID: pageStartAfter,
				environments:       verifiedEnvironments,
				more:               page.More,
			}); err != nil {
				return recoveryInventoryScanResult{}, err
			}
		}
		if !page.More {
			return recoveryInventoryScanResult{snapshot: pinnedSnapshot}, nil
		}
	}
}

func recoveryInventoryWriterFromRegistration(registration preparedRecoveryRegistration) recoveryInventoryExpectedLocalWriter {
	return recoveryInventoryExpectedLocalWriter{
		certificateID:        registration.certificateID,
		certificateBytes:     append([]byte(nil), registration.environment.CertificateBytes...),
		membershipGeneration: registration.targetMembershipGeneration,
	}
}

func cloneRecoveryInventoryRecord(record relay.EnvironmentInventoryRecord) relay.EnvironmentInventoryRecord {
	record.CertificateBytes = append([]byte(nil), record.CertificateBytes...)
	if record.Retirement != nil {
		retirement := *record.Retirement
		retirement.RetirementBytes = append([]byte(nil), retirement.RetirementBytes...)
		record.Retirement = &retirement
	}
	return record
}

func mintRecoveryCredential(
	recovery credential.ProjectRecoveryCredential,
	adminPublic protocol.PublicKey,
	environmentID continuity.EnvironmentID,
	membershipGeneration uint32,
) (credential.TrustedProjectCredential, error) {
	environmentSeed, err := crypto.GenerateEnvironmentSeed()
	if err != nil {
		return credential.TrustedProjectCredential{}, newProblem(CodeInternal, PhaseCredentialGeneration, ActionRetry)
	}
	bearer, err := credential.GenerateRelayBearer()
	if err != nil {
		return credential.TrustedProjectCredential{}, newProblem(CodeInternal, PhaseCredentialGeneration, ActionRetry)
	}
	certificate, err := crypto.SignEnvironmentCertificate(protocol.EnvironmentCertificate{
		Version:               protocol.CertificateVersionV1,
		ProtocolVersion:       protocol.ProtocolVersionV1,
		CipherSuite:           protocol.CipherSuiteXChaCha20Poly1305,
		ProjectID:             recovery.ProjectID,
		ChannelID:             recovery.ChannelID,
		EnvironmentID:         environmentID,
		EnvironmentPublicKey:  crypto.EnvironmentPublicKey(environmentSeed),
		Mode:                  protocol.EnvironmentTrusted,
		MembershipGeneration:  membershipGeneration,
		AllowedKeyGenerations: []uint32{recovery.WriteGeneration},
	}, recovery.AdminSeed)
	if err != nil {
		return credential.TrustedProjectCredential{}, newProblem(CodeInternal, PhaseCredentialGeneration, ActionRestartRecovery)
	}
	prepared := credential.TrustedProjectCredential{
		ProjectID:                     recovery.ProjectID,
		RelayURL:                      recovery.RelayURL,
		RelayGeneration:               recovery.RelayGeneration,
		ChannelID:                     recovery.ChannelID,
		AdminPublicKey:                adminPublic,
		Certificate:                   certificate,
		EnvironmentSeed:               environmentSeed,
		EnvironmentRelayAuthorization: bearer,
		ProjectRoot:                   recovery.ProjectRoot,
		WriteGeneration:               recovery.WriteGeneration,
		MinimumProtocolVersion:        protocol.ProtocolVersionV1,
	}
	if err := prepared.Validate(); err != nil {
		return credential.TrustedProjectCredential{}, newProblem(CodeInternal, PhaseCredentialGeneration, ActionRestartRecovery)
	}
	return prepared, nil
}

func mapCreateChannelError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, relay.ErrImmutableConflict) || errors.Is(err, relay.ErrUnauthenticated) || errors.Is(err, relay.ErrNotFound) {
		return newProblem(CodeAuthorization, PhaseChannelAuthorization, ActionCheckRecoveryAuthority)
	}
	if errors.Is(err, relay.ErrGenerationMismatch) || errors.Is(err, relay.ErrRollback) {
		return newProblem(CodeConflict, PhaseChannelCreation, ActionRestartRecovery)
	}
	if errors.Is(err, relay.ErrInvalidArgument) || errors.Is(err, relay.ErrUnverified) {
		return newProblem(CodeRemote, PhaseChannelCreation, ActionRestartRecovery)
	}
	return newProblem(CodeUnavailable, PhaseChannelCreation, ActionRetry)
}

func mapInventoryError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, relay.ErrUnauthenticated) || errors.Is(err, relay.ErrNotFound) {
		return newProblem(CodeAuthorization, PhaseChannelAuthorization, ActionCheckRecoveryAuthority)
	}
	if errors.Is(err, relay.ErrMembershipChanged) {
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRetry)
	}
	if errors.Is(err, relay.ErrGenerationMismatch) || errors.Is(err, relay.ErrRollback) {
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	if errors.Is(err, relay.ErrInvalidArgument) || errors.Is(err, relay.ErrUnverified) || errors.Is(err, relay.ErrImmutableConflict) {
		return newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	return newProblem(CodeUnavailable, PhaseEnvironmentInventory, ActionRetry)
}
