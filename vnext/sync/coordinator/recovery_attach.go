package coordinator

import (
	"context"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
)

const recoveryAttachTerminalChunkFrames = 16

// RecoveryAttachOptions supplies the trusted clock boundary used while the
// complete retained history is verified. The coordinator never reads a local
// wall clock implicitly.
type RecoveryAttachOptions struct {
	TrustedNowMillis        int64
	MaximumFutureSkewMillis int64
}

// AttachPreparedRecovery registers one already-protected recovery credential,
// pins the complete post-registration authority, downloads its exact retained
// prefix, verifies the full sealed/pruned history, atomically promotes it, and
// activates the channel. Credentials remain call-local and no canonical fact
// changes until the complete terminal candidate promotes successfully.
func (coordinator *Coordinator) AttachPreparedRecovery(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	recovery credential.ProjectRecoveryCredential,
	prepared credential.TrustedProjectCredential,
	options RecoveryAttachOptions,
) (continuitysqlite.SyncProgress, error) {
	if ctx == nil || options.TrustedNowMillis < 0 || options.MaximumFutureSkewMillis < 0 {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseAttachActivation, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.SyncProgress{}, err
	}
	registration, err := coordinator.bindPreparedRecoveryRegistration(expectedProjectID, recovery, prepared)
	if err != nil {
		return continuitysqlite.SyncProgress{}, err
	}
	_, recoveryAuthorityActive, err := coordinator.store.CurrentSyncAuthorityRecoverySuccessor(ctx, expectedProjectID)
	if err != nil {
		return continuitysqlite.SyncProgress{}, mapRecoveryAttachStoreError(ctx, err)
	}
	var binding continuitysqlite.SyncAuthorityBinding
	if recoveryAuthorityActive {
		binding, err = coordinator.convergeRegisteredRecoveryAuthority(
			ctx, expectedProjectID, recovery, registration,
		)
		if err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
	} else if retainedBinding, retained, retainedErr := coordinator.currentPreparedRecoveryBinding(
		ctx, expectedProjectID, prepared,
	); retainedErr != nil {
		return continuitysqlite.SyncProgress{}, retainedErr
	} else if retained {
		binding = retainedBinding
	} else {
		if _, err := coordinator.registerPreparedRecoveryEnvironment(
			ctx, expectedProjectID, recovery, registration,
		); err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
		binding, err = coordinator.convergeRegisteredRecoveryAuthority(
			ctx, expectedProjectID, recovery, registration,
		)
		if err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
	}
	progress, err := coordinator.downloadRecoverySnapshot(ctx, expectedProjectID, prepared, binding)
	if err != nil {
		return continuitysqlite.SyncProgress{}, err
	}
	if recoveryAttachProgressAtCutoff(progress, binding) {
		return coordinator.activatePreparedRecovery(ctx, expectedProjectID, binding)
	}

	candidate, terminalActive, err := coordinator.store.CurrentTerminalCandidate(ctx, expectedProjectID)
	if err != nil {
		return continuitysqlite.SyncProgress{}, mapRecoveryAttachStoreError(ctx, err)
	}
	var recoveryPrunes continuitysqlite.SyncRecoveryPruneCandidate
	if terminalActive {
		if !recoveryAttachCandidateMatches(candidate, expectedProjectID, binding, progress) {
			return continuitysqlite.SyncProgress{}, newProblem(CodeConflict, PhaseAttachActivation, ActionRestartRecovery)
		}
		var pruneFound bool
		recoveryPrunes, pruneFound, err = coordinator.store.CurrentSyncRecoveryPruneCandidate(ctx, expectedProjectID)
		if err != nil {
			return continuitysqlite.SyncProgress{}, mapRecoveryAttachStoreError(ctx, err)
		}
		if !pruneFound {
			if concurrent, done, concurrentErr := coordinator.activateConcurrentRecoveryPromotion(
				ctx, expectedProjectID, binding,
			); concurrentErr != nil || done {
				return concurrent, concurrentErr
			}
			return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
		}
		if !recoveryPrunes.Ready || recoveryPrunes.ProjectID != expectedProjectID ||
			recoveryPrunes.Snapshot.Authority != binding {
			return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
		}
		if err := coordinator.revalidateRecoveryAttachPruneInventory(
			ctx, expectedProjectID, prepared, binding, recoveryPrunes,
		); err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
	} else {
		recoveryPrunes, err = coordinator.persistRecoveryPruneInventory(
			ctx, expectedProjectID, prepared, binding,
		)
		if err != nil {
			if concurrent, done, concurrentErr := coordinator.activateConcurrentRecoveryPromotion(
				ctx, expectedProjectID, binding,
			); concurrentErr != nil || done {
				return concurrent, concurrentErr
			}
			return continuitysqlite.SyncProgress{}, err
		}
	}

	after := progress.AppliedCursor
	for after < binding.InventoryArrivalHead {
		if err := ctx.Err(); err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
		limit := recoveryAttachTerminalChunkFrames
		if terminalActive && after < candidate.ThroughArrivalSequence &&
			candidate.ThroughArrivalSequence-after < int64(limit) {
			limit = int(candidate.ThroughArrivalSequence - after)
		}
		frames, err := coordinator.store.PendingSyncFramesAfter(
			ctx, expectedProjectID, after, limit,
		)
		if err != nil {
			return continuitysqlite.SyncProgress{}, mapRecoveryAttachStoreError(ctx, err)
		}
		if len(frames) == 0 {
			return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
		}
		verified := make([]continuitysqlite.VerifiedTerminalCandidateFrame, len(frames))
		for index := range frames {
			verified[index].Inbox = frames[index]
			switch {
			case len(frames[index].SealedEnvelope) != 0 && len(frames[index].PrunedArrival) == 0:
				sealed, verifyErr := coordinator.recoveryTerminalSealedFrame(
					ctx, expectedProjectID, prepared, binding, frames[index],
				)
				if verifyErr != nil {
					return continuitysqlite.SyncProgress{}, verifyErr
				}
				verified[index].Sealed = &sealed
			case len(frames[index].SealedEnvelope) == 0 && len(frames[index].PrunedArrival) != 0:
				pruned, verifyErr := coordinator.recoveryTerminalPrunedFrame(
					ctx, expectedProjectID, binding, recoveryPrunes, frames[index],
				)
				if verifyErr != nil {
					return continuitysqlite.SyncProgress{}, verifyErr
				}
				verified[index].Pruned = &pruned
			default:
				return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
			}
		}
		next, stageErr := coordinator.store.StageVerifiedRecoveryTerminalCandidateChunk(
			ctx, expectedProjectID, binding, recoveryPrunes, verified,
			options.TrustedNowMillis, options.MaximumFutureSkewMillis,
		)
		if stageErr != nil {
			if concurrent, done, concurrentErr := coordinator.activateConcurrentRecoveryPromotion(
				ctx, expectedProjectID, binding,
			); concurrentErr != nil || done {
				return concurrent, concurrentErr
			}
			return continuitysqlite.SyncProgress{}, mapRecoveryAttachStoreError(ctx, stageErr)
		}
		lastArrival := frames[len(frames)-1].ArrivalSequence
		if !recoveryAttachCandidateMatches(next, expectedProjectID, binding, progress) ||
			next.ThroughArrivalSequence < lastArrival || lastArrival <= after {
			return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
		}
		candidate = next
		terminalActive = true
		after = lastArrival
	}
	if candidate.ThroughArrivalSequence != binding.InventoryArrivalHead {
		return continuitysqlite.SyncProgress{}, newProblem(CodeConflict, PhaseAttachActivation, ActionRestartRecovery)
	}
	if _, err := coordinator.store.PromoteTerminalCandidate(
		ctx, expectedProjectID, candidate.Checkpoint(),
	); err != nil {
		if concurrent, done, concurrentErr := coordinator.activateConcurrentRecoveryPromotion(
			ctx, expectedProjectID, binding,
		); concurrentErr != nil || done {
			return concurrent, concurrentErr
		}
		return continuitysqlite.SyncProgress{}, mapRecoveryAttachStoreError(ctx, err)
	}
	return coordinator.activatePreparedRecovery(ctx, expectedProjectID, binding)
}

func (coordinator *Coordinator) currentPreparedRecoveryBinding(
	ctx context.Context,
	projectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
) (continuitysqlite.SyncAuthorityBinding, bool, error) {
	if _, found, err := coordinator.store.CurrentSyncAuthorityCandidate(ctx, projectID); err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryAttachStoreError(ctx, err)
	} else if found {
		return continuitysqlite.SyncAuthorityBinding{}, false, nil
	}
	progress, err := coordinator.store.CurrentSyncProgress(ctx, projectID)
	if err != nil {
		var syncErr *continuitysqlite.SyncError
		if errors.As(err, &syncErr) && syncErr.Code == continuitysqlite.SyncErrorNotFound {
			return continuitysqlite.SyncAuthorityBinding{}, false, nil
		}
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryAttachStoreError(ctx, err)
	}
	binding, err := coordinator.store.CurrentSyncAuthorityBinding(ctx, projectID)
	if err != nil {
		return continuitysqlite.SyncAuthorityBinding{}, false, mapRecoveryAttachStoreError(ctx, err)
	}
	if !recoveryDownloadBindingMatchesCredential(binding, prepared) ||
		progress.ProjectID != projectID || progress.ChannelID != binding.ChannelID ||
		!validRecoveryDownloadProgress(progress, projectID, binding) {
		return continuitysqlite.SyncAuthorityBinding{}, false,
			newProblem(CodeConflict, PhaseAttachActivation, ActionRestartRecovery)
	}
	return binding, true, nil
}

func recoveryAttachProgressAtCutoff(
	progress continuitysqlite.SyncProgress,
	binding continuitysqlite.SyncAuthorityBinding,
) bool {
	return progress.AppliedCursor == binding.InventoryArrivalHead &&
		progress.DownloadedCursor == binding.InventoryArrivalHead &&
		progress.RelayHead == binding.InventoryArrivalHead
}

func recoveryAttachCandidateMatches(
	candidate continuitysqlite.TerminalCandidate,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	progress continuitysqlite.SyncProgress,
) bool {
	return candidate.ProjectID == projectID && candidate.CandidateID != ([32]byte{}) &&
		candidate.ChannelID == binding.ChannelID && candidate.RelayGeneration == binding.RelayGeneration &&
		candidate.MembershipGeneration == binding.MembershipGeneration && candidate.AuthorityDigest == binding.AuthorityDigest &&
		candidate.StartArrivalSequence > 0 && candidate.StartArrivalSequence-1 == progress.AppliedCursor &&
		candidate.ThroughArrivalSequence >= candidate.StartArrivalSequence &&
		candidate.ThroughArrivalSequence <= binding.InventoryArrivalHead && candidate.FrameCount > 0
}

func (coordinator *Coordinator) activatePreparedRecovery(
	ctx context.Context,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
) (continuitysqlite.SyncProgress, error) {
	progress, err := coordinator.store.ActivateStagedSync(ctx, projectID, binding)
	if err != nil {
		return continuitysqlite.SyncProgress{}, mapRecoveryActivationStoreError(ctx, err)
	}
	if progress.ProjectID != projectID || progress.ChannelID != binding.ChannelID ||
		progress.ActivationState != continuitysqlite.SyncActivationAttached ||
		!recoveryAttachProgressAtCutoff(progress, binding) {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	return progress, nil
}

func (coordinator *Coordinator) activateConcurrentRecoveryPromotion(
	ctx context.Context,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
) (continuitysqlite.SyncProgress, bool, error) {
	progress, err := coordinator.store.CurrentSyncProgress(ctx, projectID)
	if err != nil {
		return continuitysqlite.SyncProgress{}, false, mapRecoveryAttachStoreError(ctx, err)
	}
	if !recoveryAttachProgressAtCutoff(progress, binding) {
		return continuitysqlite.SyncProgress{}, false, nil
	}
	attached, err := coordinator.activatePreparedRecovery(ctx, projectID, binding)
	return attached, err == nil, err
}

func mapRecoveryActivationStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	if syncErr.Code == continuitysqlite.SyncErrorStore && syncErr.Field == "" {
		return newProblem(CodeUnavailable, PhaseAttachActivation, ActionRetry)
	}
	if syncErr.Code == continuitysqlite.SyncErrorConflict {
		switch syncErr.Field {
		case "terminal_candidate", "sync_authority_candidate", "sync_recovery_prune_candidate", "sync_progress", "checkpoint",
			"sync_authority", "sync_authority_recovery_transition":
			return newProblem(CodeConflict, PhaseAttachActivation, ActionRetry)
		}
	}
	return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
}

func mapRecoveryAttachStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseAttachActivation, ActionRetry)
	case continuitysqlite.SyncErrorConflict:
		switch syncErr.Field {
		case "checkpoint", "sync_authority_candidate", "sync_authority_recovery_transition",
			"sync_recovery_prune_candidate", "terminal_candidate", "sync_progress":
			return newProblem(CodeConflict, PhaseAttachActivation, ActionRetry)
		default:
			return newProblem(CodeConflict, PhaseAttachActivation, ActionRestartRecovery)
		}
	case continuitysqlite.SyncErrorCursor:
		return newProblem(CodeConflict, PhaseAttachActivation, ActionRetry)
	case continuitysqlite.SyncErrorInvalid, continuitysqlite.SyncErrorArrivalGap,
		continuitysqlite.SyncErrorEnvironmentGap, continuitysqlite.SyncErrorEnvelopeChain,
		continuitysqlite.SyncErrorCertificate, continuitysqlite.SyncErrorNonceReuse,
		continuitysqlite.SyncErrorHLC, continuitysqlite.SyncErrorCandidate,
		continuitysqlite.SyncErrorActivation, continuitysqlite.SyncErrorTerminalHistoryRequired,
		continuitysqlite.SyncErrorRecoveryRequired:
		return newProblem(CodeRemote, PhaseAttachActivation, ActionRestartRecovery)
	case continuitysqlite.SyncErrorNotFound, continuitysqlite.SyncErrorNotAttached:
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	default:
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
}
