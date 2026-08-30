package coordinator

import (
	"context"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/relay"
)

// completedRecoveryRegistration is the secret-free output of the private
// registration-call atom. It records either an observed Exact classification
// or one well-formed RegisterEnvironment response while the local guard stayed
// intact. A mandatory later full inventory scan must establish durable, exact
// post-registration authority before promotion or activation; this value is
// not attach, download, activation, or credential-installation success.
type completedRecoveryRegistration struct {
	guard recoveryRegistrationGuard
	state relay.ChannelState
}

func (completedRecoveryRegistration) String() string {
	return "[REDACTED completed recovery registration]"
}

func (completedRecoveryRegistration) GoString() string {
	return "coordinator.completedRecoveryRegistration([REDACTED])"
}

// registerPreparedRecoveryEnvironment classifies one exact protected
// registration, seals its recovery-authority guard, and performs at most one
// idempotent relay mutation. Every remote call receives a fresh request;
// bearer authority and request values remain call-local.
func (coordinator *Coordinator) registerPreparedRecoveryEnvironment(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (completedRecoveryRegistration, error) {
	if err := validatePreparationContext(ctx); err != nil {
		return completedRecoveryRegistration{}, err
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return completedRecoveryRegistration{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return completedRecoveryRegistration{}, err
	}

	initialStatus, initialErr := coordinator.remote.ClassifyEnvironmentRegistration(
		ctx,
		freshRecoveryRegistrationRequest(recovery, registration),
	)
	if initialErr != nil {
		if contextErr := contextError(ctx, initialErr); contextErr != nil {
			return completedRecoveryRegistration{}, contextErr
		}
	}
	if err := ctx.Err(); err != nil {
		return completedRecoveryRegistration{}, err
	}

	var (
		guard        recoveryRegistrationGuard
		observedHead int64
	)
	if initialErr == nil {
		if initialStatus.Disposition != relay.EnvironmentRegistrationExact && initialStatus.Disposition != relay.EnvironmentRegistrationAbsent {
			return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
		}
		if err := validateRecoveryRegistrationStatus(recovery, registration, initialStatus, initialStatus.Disposition, 0); err != nil {
			return completedRecoveryRegistration{}, err
		}
		observedHead = initialStatus.State.Head
		if err := coordinator.advanceRecoveryRegistrationWatermark(ctx, expectedProjectID, recovery, observedHead); err != nil {
			return completedRecoveryRegistration{}, err
		}
	}
	if initialErr == nil && initialStatus.Disposition == relay.EnvironmentRegistrationExact {
		if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
			return completedRecoveryRegistration{}, err
		}
		var err error
		guard, err = coordinator.reconstructRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration)
		if err != nil {
			return completedRecoveryRegistration{}, err
		}
		localHead, err := coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard)
		if err != nil {
			return completedRecoveryRegistration{}, err
		}
		if initialStatus.State.Head < maximumRecoveryRegistrationHead(guard.inventorySnapshot.ArrivalHead, localHead) {
			return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
		}
		return completedRecoveryRegistration{guard: guard, state: initialStatus.State}, nil
	}

	if initialErr != nil {
		firstMembershipMissingChannel := registration.targetMembershipGeneration == 1 &&
			(errors.Is(initialErr, relay.ErrUnauthenticated) || errors.Is(initialErr, relay.ErrNotFound))
		if !firstMembershipMissingChannel {
			return completedRecoveryRegistration{}, mapEnvironmentRegistrationRelayError(ctx, initialErr)
		}
		if err := coordinator.advanceRecoveryRegistrationWatermark(ctx, expectedProjectID, recovery, 0); err != nil {
			return completedRecoveryRegistration{}, err
		}
	}

	var (
		err         error
		reusedReady bool
	)
	guard, reusedReady, err = coordinator.currentReadyRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration)
	if err != nil {
		return completedRecoveryRegistration{}, err
	}
	if !reusedReady {
		guard, err = coordinator.stageRecoveryRegistrationGuardAtOrAbove(ctx, expectedProjectID, recovery, registration, observedHead)
		if err != nil {
			return completedRecoveryRegistration{}, err
		}
	}
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return completedRecoveryRegistration{}, err
	}
	localHead, err := coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard)
	if err != nil {
		return completedRecoveryRegistration{}, err
	}
	observedHead = maximumRecoveryRegistrationHead(observedHead, guard.inventorySnapshot.ArrivalHead, localHead)
	if err := coordinator.advanceRecoveryRegistrationWatermark(ctx, expectedProjectID, recovery, observedHead); err != nil {
		return completedRecoveryRegistration{}, err
	}
	if err := ctx.Err(); err != nil {
		return completedRecoveryRegistration{}, err
	}

	finalStatus, err := coordinator.remote.ClassifyEnvironmentRegistration(
		ctx,
		freshRecoveryRegistrationRequest(recovery, registration),
	)
	if err != nil {
		return completedRecoveryRegistration{}, mapEnvironmentRegistrationRelayError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return completedRecoveryRegistration{}, err
	}
	if finalStatus.Disposition != relay.EnvironmentRegistrationExact && finalStatus.Disposition != relay.EnvironmentRegistrationAbsent {
		return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
	}
	if err := validateRecoveryRegistrationStatus(recovery, registration, finalStatus, finalStatus.Disposition, observedHead); err != nil {
		return completedRecoveryRegistration{}, err
	}

	// The final classifier is an external call and may race with local work.
	// Re-read both the immutable registration binding and the complete local
	// guard after it returns, before either accepting Exact or mutating Absent.
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return completedRecoveryRegistration{}, err
	}
	localHead, err = coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard)
	if err != nil {
		return completedRecoveryRegistration{}, err
	}
	if finalStatus.State.Head < localHead {
		return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
	}
	observedHead = finalStatus.State.Head
	if err := coordinator.advanceRecoveryRegistrationWatermark(ctx, expectedProjectID, recovery, observedHead); err != nil {
		return completedRecoveryRegistration{}, err
	}
	// The watermark transaction is another local boundary. Make the exact
	// binding and guard check the final operation before either completion or
	// the relay mutation.
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return completedRecoveryRegistration{}, err
	}
	localHead, err = coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard)
	if err != nil {
		return completedRecoveryRegistration{}, err
	}
	if finalStatus.State.Head < localHead {
		return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
	}
	if finalStatus.Disposition == relay.EnvironmentRegistrationExact {
		return completedRecoveryRegistration{guard: guard, state: finalStatus.State}, nil
	}
	if err := ctx.Err(); err != nil {
		return completedRecoveryRegistration{}, err
	}

	state, err := coordinator.remote.RegisterEnvironment(
		ctx,
		freshRecoveryRegistrationRequest(recovery, registration),
	)
	if err != nil {
		return completedRecoveryRegistration{}, mapEnvironmentRegistrationRelayError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return completedRecoveryRegistration{}, err
	}
	if err := validateRecoveryRegistrationSuccess(recovery, registration, observedHead, state); err != nil {
		return completedRecoveryRegistration{}, err
	}
	// RegisterEnvironment is also an external call. A successful response
	// completes this private call atom only if the exact local binding and guard
	// still survive the call. Later attach stages must independently full-scan
	// the post-registration authority before any promotion or activation.
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return completedRecoveryRegistration{}, err
	}
	localHead, err = coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard)
	if err != nil {
		return completedRecoveryRegistration{}, err
	}
	if state.Head < localHead {
		return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
	}
	if err := coordinator.advanceRecoveryRegistrationWatermark(ctx, expectedProjectID, recovery, state.Head); err != nil {
		return completedRecoveryRegistration{}, err
	}
	if err := coordinator.validateRecoveryRegistrationBinding(ctx, expectedProjectID, recovery, registration); err != nil {
		return completedRecoveryRegistration{}, err
	}
	localHead, err = coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard)
	if err != nil {
		return completedRecoveryRegistration{}, err
	}
	if state.Head < localHead {
		return completedRecoveryRegistration{}, malformedRecoveryRegistrationResponse()
	}
	return completedRecoveryRegistration{guard: guard, state: state}, nil
}

func freshRecoveryRegistrationRequest(
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) relay.RegisterEnvironmentRequest {
	environment := registration.environment
	environment.CertificateBytes = append([]byte(nil), registration.environment.CertificateBytes...)
	return relay.RegisterEnvironmentRequest{
		Authorization: recoveryOwnerAuthorization(recovery),
		Environment:   environment,
	}
}

func recoveryRegistrationWatermark(
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	head int64,
) continuitysqlite.SyncRelayWatermark {
	return continuitysqlite.SyncRelayWatermark{
		ProjectID:       expectedProjectID,
		ChannelID:       continuitysqlite.SyncChannelID(recovery.ChannelID),
		RelayGeneration: [32]byte(recovery.RelayGeneration),
		AdminPublicKey:  [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)),
		RelayHead:       head,
	}
}

func (coordinator *Coordinator) advanceRecoveryRegistrationWatermark(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	head int64,
) error {
	want := recoveryRegistrationWatermark(expectedProjectID, recovery, head)
	retained, err := coordinator.store.AdvanceSyncRelayWatermark(ctx, want)
	if err != nil {
		return mapRecoveryRegistrationWatermarkError(ctx, err)
	}
	// The persistence API refuses lower observations. Requiring the exact input
	// here prevents a future implementation from treating a returned maximum as
	// acceptance of a regressed remote head.
	if retained != want {
		return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
	}
	return nil
}

func mapRecoveryRegistrationWatermarkError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorCursor:
		return newProblem(CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	case continuitysqlite.SyncErrorConflict:
		return newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
	}
}

func validateRecoveryRegistrationStatus(
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	status relay.EnvironmentRegistrationStatus,
	want relay.EnvironmentRegistrationDisposition,
	minimumHead int64,
) error {
	state := status.State
	if status.Disposition != want || state.ChannelID != relay.ChannelID(recovery.ChannelID) ||
		state.RelayGeneration != relay.RelayGeneration(recovery.RelayGeneration) || state.Head < 0 || state.Head < minimumHead {
		return malformedRecoveryRegistrationResponse()
	}
	switch want {
	case relay.EnvironmentRegistrationExact:
		if state.MembershipGeneration < registration.targetMembershipGeneration {
			return malformedRecoveryRegistrationResponse()
		}
	case relay.EnvironmentRegistrationAbsent:
		if state.MembershipGeneration != registration.targetMembershipGeneration-1 ||
			(state.MembershipGeneration == 0 && state.Head != 0) {
			return malformedRecoveryRegistrationResponse()
		}
	default:
		return malformedRecoveryRegistrationResponse()
	}
	return nil
}

func validateRecoveryRegistrationSuccess(
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	minimumHead int64,
	state relay.ChannelState,
) error {
	if state.ChannelID != relay.ChannelID(recovery.ChannelID) ||
		state.RelayGeneration != relay.RelayGeneration(recovery.RelayGeneration) ||
		state.MembershipGeneration < registration.targetMembershipGeneration ||
		state.Head < 0 || state.Head < minimumHead {
		return malformedRecoveryRegistrationResponse()
	}
	return nil
}

func malformedRecoveryRegistrationResponse() error {
	return newProblem(CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
}

func (coordinator *Coordinator) reconstructRecoveryRegistrationGuard(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (recoveryRegistrationGuard, error) {
	if registration.targetMembershipGeneration == 1 {
		guard := recoveryRegistrationGuard{
			targetMembershipGeneration: 1,
			inventorySnapshot:          relay.EnvironmentInventorySnapshot{},
		}
		if _, err := coordinator.revalidateRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration, guard); err != nil {
			return recoveryRegistrationGuard{}, err
		}
		return guard, nil
	}

	previousMembership := registration.targetMembershipGeneration - 1
	base, baseFound, err := coordinator.currentRecoveryRegistrationAtomBase(ctx, expectedProjectID, recovery, previousMembership)
	if err != nil {
		return recoveryRegistrationGuard{}, err
	}
	baseVersion, baseDigest := uint16(0), [32]byte{}
	if baseFound {
		baseVersion = base.AuthorityDigestVersion
		baseDigest = base.AuthorityDigest
	}
	candidate, found, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, expectedProjectID)
	if err != nil {
		return recoveryRegistrationGuard{}, mapRecoveryRegistrationAtomStoreError(ctx, err)
	}
	if !found || !candidate.Ready ||
		!sameRecoveryRegistrationCandidateBase(candidate, expectedProjectID, recovery, previousMembership, baseVersion, baseDigest) ||
		candidate.Snapshot.InventoryArrivalHead < 0 {
		return recoveryRegistrationGuard{}, newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}
	candidateCopy := candidate
	return recoveryRegistrationGuard{
		targetMembershipGeneration: registration.targetMembershipGeneration,
		inventorySnapshot: relay.EnvironmentInventorySnapshot{
			MembershipGeneration: previousMembership,
			ArrivalHead:          candidate.Snapshot.InventoryArrivalHead,
		},
		candidate: &candidateCopy,
	}, nil
}

// currentReadyRecoveryRegistrationGuard recovers an already-audited durable
// guard for an exact Absent retry. A READY snapshot at the same membership
// remains complete when only the relay arrival head advances: membership
// changes, not ordinary arrivals, carry authority joins and retirements. This
// path never discards, replaces, promotes, or remotely replays the candidate;
// the caller must retain the newer classified head as its mutation floor.
func (coordinator *Coordinator) currentReadyRecoveryRegistrationGuard(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
) (recoveryRegistrationGuard, bool, error) {
	if registration.targetMembershipGeneration == 1 {
		return recoveryRegistrationGuard{}, false, nil
	}
	candidate, found, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, expectedProjectID)
	if err != nil {
		return recoveryRegistrationGuard{}, false, mapRecoveryRegistrationAtomStoreError(ctx, err)
	}
	if !found || !candidate.Ready {
		return recoveryRegistrationGuard{}, false, nil
	}
	guard, err := coordinator.reconstructRecoveryRegistrationGuard(ctx, expectedProjectID, recovery, registration)
	if err != nil {
		return recoveryRegistrationGuard{}, false, err
	}
	return guard, true, nil
}

func (coordinator *Coordinator) revalidateRecoveryRegistrationGuard(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	registration preparedRecoveryRegistration,
	guard recoveryRegistrationGuard,
) (int64, error) {
	if guard.targetMembershipGeneration != registration.targetMembershipGeneration ||
		guard.inventorySnapshot.MembershipGeneration != registration.targetMembershipGeneration-1 ||
		guard.inventorySnapshot.ArrivalHead < 0 {
		return 0, newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}

	previousMembership := registration.targetMembershipGeneration - 1
	base, baseFound, err := coordinator.currentRecoveryRegistrationAtomBase(ctx, expectedProjectID, recovery, previousMembership)
	if err != nil {
		return 0, err
	}
	localHead := guard.inventorySnapshot.ArrivalHead
	if baseFound {
		localHead = maximumRecoveryRegistrationHead(localHead, base.InventoryArrivalHead)
	}
	candidate, candidateFound, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, expectedProjectID)
	if err != nil {
		return 0, mapRecoveryRegistrationAtomStoreError(ctx, err)
	}

	if registration.targetMembershipGeneration == 1 {
		if baseFound || candidateFound || guard.candidate != nil || guard.inventorySnapshot != (relay.EnvironmentInventorySnapshot{}) {
			return 0, newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
		}
		return localHead, nil
	}
	if guard.candidate == nil || !candidateFound || !candidate.Ready || candidate != *guard.candidate ||
		candidate.Snapshot.MembershipGeneration != previousMembership ||
		candidate.Snapshot.InventoryArrivalHead != guard.inventorySnapshot.ArrivalHead {
		return 0, newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}
	baseVersion, baseDigest := uint16(0), [32]byte{}
	if baseFound {
		baseVersion = base.AuthorityDigestVersion
		baseDigest = base.AuthorityDigest
	}
	if !sameRecoveryRegistrationCandidateBase(candidate, expectedProjectID, recovery, previousMembership, baseVersion, baseDigest) {
		return 0, newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}
	return localHead, nil
}

func maximumRecoveryRegistrationHead(values ...int64) int64 {
	var maximum int64
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func (coordinator *Coordinator) currentRecoveryRegistrationAtomBase(
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
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryRegistrationAtomStoreError(ctx, err)
	}
	if binding.ChannelID != continuitysqlite.SyncChannelID(recovery.ChannelID) ||
		binding.RelayGeneration != [32]byte(recovery.RelayGeneration) ||
		binding.AdminPublicKey != [32]byte(crypto.AdminPublicKey(recovery.AdminSeed)) ||
		binding.MembershipGeneration > maximumMembership {
		return continuitysqlite.SyncAuthorityBinding{}, false, newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}
	return binding, true, nil
}

func mapRecoveryRegistrationAtomStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorConflict:
		return newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseEnvironmentRegistration, ActionRepairLocalStore)
	}
}

func mapEnvironmentRegistrationRelayError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, relay.ErrUnauthenticated) || errors.Is(err, relay.ErrNotFound) {
		return newProblem(CodeAuthorization, PhaseEnvironmentRegistration, ActionCheckRecoveryAuthority)
	}
	if errors.Is(err, relay.ErrMembershipChanged) || errors.Is(err, relay.ErrImmutableConflict) ||
		errors.Is(err, relay.ErrGenerationMismatch) || errors.Is(err, relay.ErrRollback) ||
		errors.Is(err, relay.ErrRetired) || errors.Is(err, relay.ErrExpired) {
		return newProblem(CodeConflict, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}
	if errors.Is(err, relay.ErrInvalidArgument) || errors.Is(err, relay.ErrUnverified) {
		return newProblem(CodeRemote, PhaseEnvironmentRegistration, ActionRestartRecovery)
	}
	return newProblem(CodeUnavailable, PhaseEnvironmentRegistration, ActionRetry)
}
