package coordinator

import (
	"bytes"
	"context"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/relay"
)

var errReclassifyRecoveryAuthority = errors.New("reclassify recovery authority")

// convergeRegisteredRecoveryAuthority installs the complete, verified
// post-registration authority as canonical state. It is restartable from only
// the protected registration and recovery authority: every attempt
// independently reclassifies the immutable relay registration before routing
// active recovery, an ordinary predecessor guard, or committed canonical
// success. It never downloads, prunes, applies, or activates sync data.
func (coordinator *Coordinator) convergeRegisteredRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (continuitysqlite.SyncAuthorityBinding, error) {
	if err := validatePreparationContext(ctx); err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}

	for attempt := 0; attempt < 2; attempt++ {
		binding, err := coordinator.convergeRegisteredRecoveryAuthorityOnce(
			ctx, expectedProjectID, recovery, registration,
		)
		if !errors.Is(err, errReclassifyRecoveryAuthority) {
			return binding, err
		}
	}
	return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRetry)
}

func (coordinator *Coordinator) convergeRegisteredRecoveryAuthorityOnce(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (continuitysqlite.SyncAuthorityBinding, error) {
	observed, err := coordinator.classifyRegisteredRecoveryAuthority(ctx, expectedProjectID, recovery, registration)
	if err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}

	active, found, err := coordinator.store.CurrentSyncAuthorityRecoverySuccessor(ctx, expectedProjectID)
	if err != nil {
		if retryRecoveryAuthorityMutation(err) {
			return continuitysqlite.SyncAuthorityBinding{}, errReclassifyRecoveryAuthority
		}
		return continuitysqlite.SyncAuthorityBinding{}, mapRecoveryAuthorityStoreError(ctx, err)
	}
	if found {
		if err := validateRegisteredRecoveryAuthorityState(expectedProjectID, recovery, registration, active); err != nil {
			return continuitysqlite.SyncAuthorityBinding{}, err
		}
		return coordinator.convergeActiveRecoveryAuthority(
			ctx, expectedProjectID, recovery, registration, observed, active,
		)
	}

	candidate, candidateFound, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, expectedProjectID)
	if err != nil {
		if retryRecoveryAuthorityMutation(err) {
			return continuitysqlite.SyncAuthorityBinding{}, errReclassifyRecoveryAuthority
		}
		return continuitysqlite.SyncAuthorityBinding{}, mapRecoveryAuthorityStoreError(ctx, err)
	}
	if candidateFound {
		if err := validateRecoveryAuthorityPredecessor(expectedProjectID, recovery, registration, observed, candidate); err != nil {
			return continuitysqlite.SyncAuthorityBinding{}, err
		}
		candidateCopy := candidate
		return coordinator.scanAndPromoteRecoveryAuthority(
			ctx, expectedProjectID, recovery, registration, observed, &candidateCopy, nil,
		)
	}

	if binding, found, err := coordinator.currentPromotedRecoveryAuthority(
		ctx, expectedProjectID, recovery, registration, observed,
	); err != nil || found {
		return binding, err
	}
	if registration.targetMembershipGeneration != 1 {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	return coordinator.scanAndPromoteRecoveryAuthority(
		ctx, expectedProjectID, recovery, registration, observed, nil, nil,
	)
}

func (coordinator *Coordinator) classifyRegisteredRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (relay.ChannelState, error) {
	status, err := coordinator.remote.ClassifyEnvironmentRegistration(
		ctx, freshRecoveryRegistrationRequest(recovery, registration),
	)
	if err != nil {
		return relay.ChannelState{}, mapEnvironmentRegistrationRelayError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return relay.ChannelState{}, err
	}
	if err := validateRecoveryRegistrationStatus(
		recovery, registration, status, relay.EnvironmentRegistrationExact, 0,
	); err != nil {
		return relay.ChannelState{}, err
	}
	if err := coordinator.advanceRecoveryRegistrationWatermark(
		ctx, expectedProjectID, recovery,
		status.State.MembershipGeneration, status.State.Head,
	); err != nil {
		return relay.ChannelState{}, err
	}
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return relay.ChannelState{}, err
	}
	return status.State, nil
}

func (coordinator *Coordinator) convergeActiveRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	observed relay.ChannelState,
	active continuitysqlite.SyncAuthorityRecoveryState,
) (continuitysqlite.SyncAuthorityBinding, error) {
	snapshot := active.Successor.Snapshot
	if observed.Head < snapshot.InventoryArrivalHead || observed.MembershipGeneration < snapshot.MembershipGeneration {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	if observed.Head > snapshot.InventoryArrivalHead || observed.MembershipGeneration > snapshot.MembershipGeneration {
		return coordinator.scanAndPromoteRecoveryAuthority(
			ctx, expectedProjectID, recovery, registration, observed, nil, &active,
		)
	}
	if active.Successor.Ready {
		return coordinator.promoteRecoveryAuthority(
			ctx, expectedProjectID, recovery, registration, active,
			relay.EnvironmentInventorySnapshot{
				MembershipGeneration: snapshot.MembershipGeneration,
				ArrivalHead:          snapshot.InventoryArrivalHead,
			},
		)
	}
	return coordinator.resumeAndPromoteRecoveryAuthority(
		ctx, expectedProjectID, recovery, registration, active,
	)
}

func validateRegisteredRecoveryAuthorityState(
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	state continuitysqlite.SyncAuthorityRecoveryState,
) error {
	transition := state.Transition
	snapshot := state.Successor.Snapshot
	writerEnvironmentID := continuity.EnvironmentID(registration.environment.EnvironmentID)
	if transition.ProjectID != expectedProjectID || transition.WriterEnvironmentID != writerEnvironmentID ||
		transition.WriterCertificateID != [32]byte(registration.certificateID) ||
		transition.TargetMembershipGeneration != registration.targetMembershipGeneration ||
		snapshot.ChannelID != continuitysqlite.SyncChannelID(recovery.ChannelID) ||
		snapshot.RelayGeneration != [32]byte(recovery.RelayGeneration) ||
		snapshot.AdminPublicKey != [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) ||
		snapshot.MembershipGeneration < registration.targetMembershipGeneration {
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	if registration.targetMembershipGeneration == 1 {
		if transition.PredecessorCandidateID != ([32]byte{}) ||
			snapshot.BaseAuthorityDigestVersion != 0 || snapshot.BaseAuthorityDigest != ([32]byte{}) {
			return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
	} else if transition.PredecessorCandidateID == ([32]byte{}) ||
		snapshot.BaseAuthorityDigestVersion != 2 || snapshot.BaseAuthorityDigest == ([32]byte{}) {
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	return nil
}

func validateRecoveryAuthorityPredecessor(
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	observed relay.ChannelState,
	candidate continuitysqlite.SyncAuthorityCandidate,
) error {
	if registration.targetMembershipGeneration <= 1 || !candidate.Ready || candidate.ProjectID != expectedProjectID ||
		candidate.Snapshot.ChannelID != continuitysqlite.SyncChannelID(recovery.ChannelID) ||
		candidate.Snapshot.RelayGeneration != [32]byte(recovery.RelayGeneration) ||
		candidate.Snapshot.AdminPublicKey != [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) ||
		candidate.Snapshot.MembershipGeneration != registration.targetMembershipGeneration-1 ||
		candidate.Snapshot.InventoryArrivalHead > observed.Head || candidate.AuthorityDigestVersion != 2 ||
		candidate.AuthorityDigest == ([32]byte{}) {
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	return nil
}

func (coordinator *Coordinator) resumeAndPromoteRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	active continuitysqlite.SyncAuthorityRecoveryState,
) (continuitysqlite.SyncAuthorityBinding, error) {
	pinned := relay.EnvironmentInventorySnapshot{
		MembershipGeneration: active.Successor.Snapshot.MembershipGeneration,
		ArrivalHead:          active.Successor.Snapshot.InventoryArrivalHead,
	}
	expectedWriter := recoveryInventoryWriterFromRegistration(registration)
	state := active
	result, err := coordinator.scanRecoveryInventory(
		ctx,
		recovery,
		recoveryOwnerAuthorization(recovery),
		crypto.AdminPublicKey(recovery.AdminSeed),
		coordinator.store.WriterEnvironmentID(),
		recoveryInventoryScanOptions{
			firstRequestSnapshot:        &pinned,
			firstAfterEnvironmentID:     relay.EnvironmentID(active.Successor.ThroughEnvironmentID),
			minimumMembershipGeneration: pinned.MembershipGeneration,
			minimumArrivalHead:          pinned.ArrivalHead,
			expectedLocalWriter:         &expectedWriter,
			onPage: func(page verifiedRecoveryInventoryPage) error {
				persistedPage, err := recoveryAuthorityPageFromVerified(page)
				if err != nil {
					return err
				}
				before := state.Successor
				next, err := coordinator.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
					ctx,
					expectedProjectID,
					state.Transition,
					before.Checkpoint(),
					state.Successor.Snapshot,
					persistedPage,
				)
				if err != nil {
					if retryRecoveryAuthorityMutation(err) {
						return errReclassifyRecoveryAuthority
					}
					return mapRecoveryAuthorityStoreError(ctx, err)
				}
				if !exactRecoveryAuthorityPageAdvance(before, next.Successor, persistedPage) {
					return errReclassifyRecoveryAuthority
				}
				state = next
				return nil
			},
		},
	)
	if err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}
	if !state.Successor.Ready || state.Successor.Snapshot.MembershipGeneration != result.snapshot.MembershipGeneration ||
		state.Successor.Snapshot.InventoryArrivalHead != result.snapshot.ArrivalHead {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	return coordinator.promoteRecoveryAuthority(ctx, expectedProjectID, recovery, registration, state, result.snapshot)
}

func (coordinator *Coordinator) scanAndPromoteRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	observed relay.ChannelState,
	predecessor *continuitysqlite.SyncAuthorityCandidate,
	replacement *continuitysqlite.SyncAuthorityRecoveryState,
) (continuitysqlite.SyncAuthorityBinding, error) {
	expectedWriter := recoveryInventoryWriterFromRegistration(registration)
	var (
		state     continuitysqlite.SyncAuthorityRecoveryState
		haveState bool
	)
	result, err := coordinator.scanRecoveryInventory(
		ctx,
		recovery,
		recoveryOwnerAuthorization(recovery),
		crypto.AdminPublicKey(recovery.AdminSeed),
		coordinator.store.WriterEnvironmentID(),
		recoveryInventoryScanOptions{
			minimumMembershipGeneration: observed.MembershipGeneration,
			minimumArrivalHead:          observed.Head,
			expectedLocalWriter:         &expectedWriter,
			onPage: func(page verifiedRecoveryInventoryPage) error {
				persistedPage, err := recoveryAuthorityPageFromVerified(page)
				if err != nil {
					return err
				}
				if !haveState {
					if err := coordinator.advanceRecoveryRegistrationWatermark(
						ctx, expectedProjectID, recovery,
						page.snapshot.MembershipGeneration, page.snapshot.ArrivalHead,
					); err != nil {
						return err
					}
					snapshot := newRecoveryAuthoritySnapshot(recovery, page.snapshot, predecessor, replacement)
					if replacement == nil {
						start := continuitysqlite.SyncAuthorityRecoveryTransitionStart{
							WriterEnvironmentID:        coordinator.store.WriterEnvironmentID(),
							WriterCertificateID:        [32]byte(registration.certificateID),
							TargetMembershipGeneration: registration.targetMembershipGeneration,
							SuccessorSnapshot:          snapshot,
						}
						if predecessor != nil {
							start.PredecessorCheckpoint = predecessor.Checkpoint()
						}
						state, err = coordinator.store.BeginSyncAuthorityRecoveryTransition(
							ctx, expectedProjectID, start, persistedPage,
						)
					} else {
						state, err = coordinator.store.ReplaceSyncAuthorityRecoverySuccessor(
							ctx,
							expectedProjectID,
							replacement.Transition,
							replacement.Successor.Checkpoint(),
							snapshot,
							persistedPage,
						)
					}
					if err != nil {
						if retryRecoveryAuthorityMutation(err) {
							return errReclassifyRecoveryAuthority
						}
						return mapRecoveryAuthorityStoreError(ctx, err)
					}
					if !exactInitialRecoveryAuthorityPage(state.Successor, persistedPage) {
						return errReclassifyRecoveryAuthority
					}
					haveState = true
					return nil
				}

				before := state.Successor
				next, err := coordinator.store.AppendVerifiedSyncAuthorityRecoverySuccessorPage(
					ctx,
					expectedProjectID,
					state.Transition,
					before.Checkpoint(),
					state.Successor.Snapshot,
					persistedPage,
				)
				if err != nil {
					if retryRecoveryAuthorityMutation(err) {
						return errReclassifyRecoveryAuthority
					}
					return mapRecoveryAuthorityStoreError(ctx, err)
				}
				if !exactRecoveryAuthorityPageAdvance(before, next.Successor, persistedPage) {
					return errReclassifyRecoveryAuthority
				}
				state = next
				return nil
			},
		},
	)
	if err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}
	if !haveState || !state.Successor.Ready ||
		state.Successor.Snapshot.MembershipGeneration != result.snapshot.MembershipGeneration ||
		state.Successor.Snapshot.InventoryArrivalHead != result.snapshot.ArrivalHead {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	return coordinator.promoteRecoveryAuthority(ctx, expectedProjectID, recovery, registration, state, result.snapshot)
}

func newRecoveryAuthoritySnapshot(
	recovery credential.ProjectRecoveryCredential,
	inventory relay.EnvironmentInventorySnapshot,
	predecessor *continuitysqlite.SyncAuthorityCandidate,
	replacement *continuitysqlite.SyncAuthorityRecoveryState,
) continuitysqlite.SyncAuthoritySnapshot {
	snapshot := continuitysqlite.SyncAuthoritySnapshot{
		ChannelID:            continuitysqlite.SyncChannelID(recovery.ChannelID),
		RelayGeneration:      [32]byte(recovery.RelayGeneration),
		AdminPublicKey:       [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
		MembershipGeneration: inventory.MembershipGeneration,
		InventoryArrivalHead: inventory.ArrivalHead,
	}
	switch {
	case predecessor != nil:
		snapshot.BaseAuthorityDigestVersion = predecessor.AuthorityDigestVersion
		snapshot.BaseAuthorityDigest = predecessor.AuthorityDigest
	case replacement != nil:
		snapshot.BaseAuthorityDigestVersion = replacement.Successor.Snapshot.BaseAuthorityDigestVersion
		snapshot.BaseAuthorityDigest = replacement.Successor.Snapshot.BaseAuthorityDigest
	}
	return snapshot
}

func recoveryAuthorityPageFromVerified(page verifiedRecoveryInventoryPage) (continuitysqlite.SyncAuthorityPage, error) {
	environments := make([]continuitysqlite.SyncEnvironmentCertificate, 0, len(page.environments))
	for _, record := range page.environments {
		environment, err := syncEnvironmentCertificateFromRecoveryInventory(record)
		if err != nil {
			return continuitysqlite.SyncAuthorityPage{}, err
		}
		environments = append(environments, environment)
	}
	if len(environments) == 0 {
		return continuitysqlite.SyncAuthorityPage{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	return continuitysqlite.SyncAuthorityPage{
		AfterEnvironmentID:   string(page.afterEnvironmentID),
		ThroughEnvironmentID: environments[len(environments)-1].EnvironmentID,
		Environments:         environments,
		More:                 page.more,
	}, nil
}

func exactInitialRecoveryAuthorityPage(
	candidate continuitysqlite.SyncAuthorityCandidate,
	page continuitysqlite.SyncAuthorityPage,
) bool {
	return candidate.PageCount == 1 && candidate.EnvironmentCount == int64(len(page.Environments)) &&
		candidate.ThroughEnvironmentID == page.ThroughEnvironmentID && candidate.Ready != page.More
}

func exactRecoveryAuthorityPageAdvance(
	before continuitysqlite.SyncAuthorityCandidate,
	after continuitysqlite.SyncAuthorityCandidate,
	page continuitysqlite.SyncAuthorityPage,
) bool {
	return after.PageCount == before.PageCount+1 &&
		after.EnvironmentCount == before.EnvironmentCount+int64(len(page.Environments)) &&
		after.ThroughEnvironmentID == page.ThroughEnvironmentID && after.Ready != page.More
}

func (coordinator *Coordinator) promoteRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	state continuitysqlite.SyncAuthorityRecoveryState,
	snapshot relay.EnvironmentInventorySnapshot,
) (continuitysqlite.SyncAuthorityBinding, error) {
	if err := coordinator.requireCurrentRecoveryAuthorityObservation(
		ctx, expectedProjectID, recovery, registration, snapshot,
	); err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}
	receipt, err := coordinator.store.PromoteSyncAuthorityRecoverySuccessor(
		ctx, expectedProjectID, state.Transition, state.Successor.Checkpoint(),
	)
	if err != nil {
		if retryRecoveryAuthorityMutation(err) {
			return continuitysqlite.SyncAuthorityBinding{}, errReclassifyRecoveryAuthority
		}
		return continuitysqlite.SyncAuthorityBinding{}, mapRecoveryAuthorityStoreError(ctx, err)
	}
	if receipt.Outcome != continuitysqlite.SyncAuthorityRecoveryPromoted || receipt.Transition != state.Transition ||
		receipt.SuccessorCheckpoint != state.Successor.Checkpoint() {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	binding, found, err := coordinator.currentPromotedRecoveryAuthority(
		ctx, expectedProjectID, recovery, registration,
		relay.ChannelState{
			ChannelID:            relay.ChannelID(recovery.ChannelID),
			RelayGeneration:      relay.RelayGeneration(recovery.RelayGeneration),
			MembershipGeneration: snapshot.MembershipGeneration,
			Head:                 snapshot.ArrivalHead,
		},
	)
	if err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, err
	}
	if !found {
		return continuitysqlite.SyncAuthorityBinding{}, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	return binding, nil
}

func (coordinator *Coordinator) requireCurrentRecoveryAuthorityObservation(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	want relay.EnvironmentInventorySnapshot,
) error {
	observed, err := coordinator.classifyRegisteredRecoveryAuthority(
		ctx, expectedProjectID, recovery, registration,
	)
	if err != nil {
		return err
	}
	if observed.MembershipGeneration == want.MembershipGeneration && observed.Head == want.ArrivalHead {
		return nil
	}
	if observed.MembershipGeneration >= want.MembershipGeneration && observed.Head >= want.ArrivalHead {
		return errReclassifyRecoveryAuthority
	}
	return newProblem(CodeRemote, PhaseEnvironmentInventory, ActionRestartRecovery)
}

func (coordinator *Coordinator) currentPromotedRecoveryAuthority(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	observed relay.ChannelState,
) (continuitysqlite.SyncAuthorityBinding, bool, error) {
	binding, err := coordinator.store.CurrentSyncAuthorityBinding(ctx, expectedProjectID)
	if err != nil {
		if recoveryAuthorityStoreErrorCode(err, continuitysqlite.SyncErrorNotFound) {
			return continuitysqlite.SyncAuthorityBinding{}, false, nil
		}
		if retryRecoveryAuthorityMutation(err) {
			return continuitysqlite.SyncAuthorityBinding{}, false, errReclassifyRecoveryAuthority
		}
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryAuthorityStoreError(ctx, err)
	}
	if binding.ChannelID != continuitysqlite.SyncChannelID(recovery.ChannelID) ||
		binding.RelayGeneration != [32]byte(recovery.RelayGeneration) ||
		binding.AdminPublicKey != [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) ||
		binding.MembershipGeneration < registration.targetMembershipGeneration || binding.AuthorityDigestVersion != 2 ||
		binding.MembershipGeneration != observed.MembershipGeneration || binding.InventoryArrivalHead != observed.Head {
		return continuitysqlite.SyncAuthorityBinding{}, false, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	writerEnvironmentID := continuity.EnvironmentID(registration.environment.EnvironmentID)
	states, err := coordinator.store.CurrentSyncEnvironmentStates(
		ctx, expectedProjectID, binding, []continuity.EnvironmentID{writerEnvironmentID},
	)
	if err != nil {
		if recoveryAuthorityStoreErrorCode(err, continuitysqlite.SyncErrorCertificate) {
			return continuitysqlite.SyncAuthorityBinding{}, false, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
		}
		if retryRecoveryAuthorityMutation(err) {
			return continuitysqlite.SyncAuthorityBinding{}, false, errReclassifyRecoveryAuthority
		}
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryAuthorityStoreError(ctx, err)
	}
	if len(states) != 1 {
		return continuitysqlite.SyncAuthorityBinding{}, false, newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	certificate := states[0].Certificate
	if certificate.EnvironmentID != string(writerEnvironmentID) ||
		certificate.CertificateID != [32]byte(registration.certificateID) ||
		!bytes.Equal(certificate.CertificateBytes, registration.environment.CertificateBytes) ||
		certificate.Mode != continuitysqlite.SyncEnvironmentTrusted || certificate.ExpiresAtMillis != 0 ||
		certificate.JoinMembershipGeneration != registration.targetMembershipGeneration || certificate.Retirement != nil {
		return continuitysqlite.SyncAuthorityBinding{}, false, newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	}
	return binding, true, nil
}

func retryRecoveryAuthorityMutation(err error) bool {
	return recoveryAuthorityStoreErrorCode(err, continuitysqlite.SyncErrorCursor) ||
		recoveryAuthorityStoreErrorCode(err, continuitysqlite.SyncErrorConflict)
}

func recoveryAuthorityStoreErrorCode(err error, code continuitysqlite.SyncErrorCode) bool {
	var syncErr *continuitysqlite.SyncError
	return errors.As(err, &syncErr) && syncErr.Code == code
}

func mapRecoveryAuthorityStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorCursor:
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRetry)
	case continuitysqlite.SyncErrorConflict, continuitysqlite.SyncErrorCertificate, continuitysqlite.SyncErrorNotFound:
		return newProblem(CodeConflict, PhaseEnvironmentInventory, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseEnvironmentInventory, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseEnvironmentInventory, ActionRepairLocalStore)
	}
}
