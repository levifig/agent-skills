package coordinator

import (
	"context"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

// recoveryTerminalPrunedFrame binds the exact retained relay tombstone bytes
// to the authenticated projection in one immutable READY prune inventory. The
// store repeats the point join inside terminal staging; this read constructs
// the only projection that callers are allowed to submit there.
func (coordinator *Coordinator) recoveryTerminalPrunedFrame(
	ctx context.Context,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	recoveryPrunes continuitysqlite.SyncRecoveryPruneCandidate,
	inbox continuitysqlite.OpaqueSyncFrame,
) (continuitysqlite.VerifiedTerminalPrunedFrame, error) {
	if ctx == nil {
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, newProblem(CodeInvalid, PhaseAttachActivation, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, err
	}
	if coordinator == nil || coordinator.store == nil || projectID.Validate() != nil ||
		recoveryPrunes.ProjectID != projectID || recoveryPrunes.Snapshot.Authority != binding ||
		!recoveryPrunes.Ready || len(inbox.PrunedArrival) == 0 || inbox.SealedEnvelope != nil ||
		inbox.Quarantined || inbox.ArrivalSequence < 1 {
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, newProblem(CodeInvalid, PhaseAttachActivation, ActionCorrectInput)
	}
	parsed, err := protocol.ParsePrunedArrival(inbox.PrunedArrival)
	if err != nil || parsed.ChannelID != protocol.ChannelID(binding.ChannelID) ||
		parsed.RelayGeneration != protocol.RelayGeneration(binding.RelayGeneration) ||
		parsed.Reference.ArrivalSequence != inbox.ArrivalSequence ||
		protocol.Digest(inbox.EnvelopeDigest) != parsed.Reference.EnvelopeDigest {
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, malformedRecoveryTerminalPrunedFrame()
	}
	match, found, err := coordinator.store.SyncRecoveryPruneTargetByArrival(
		ctx, projectID, recoveryPrunes, inbox.ArrivalSequence,
	)
	if err != nil {
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, mapRecoveryTerminalPruneStoreError(ctx, err)
	}
	reference := continuitysqlite.VerifiedPruneReference{
		FactID:                 parsed.Reference.FactID,
		EnvironmentID:          parsed.Reference.EnvironmentID,
		EnvironmentSequence:    parsed.Reference.EnvironmentSequence,
		ArrivalSequence:        parsed.Reference.ArrivalSequence,
		EnvelopeDigest:         [32]byte(parsed.Reference.EnvelopeDigest),
		CertificateID:          [32]byte(parsed.Reference.CertificateID),
		PreviousEnvelopeDigest: [32]byte(parsed.Reference.PreviousEnvelopeDigest),
		KeyGeneration:          parsed.Reference.KeyGeneration,
		Nonce:                  [24]byte(parsed.Reference.Nonce),
	}
	if !found || match.PruneID != [32]byte(parsed.PruneID) || match.Reference != reference {
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, malformedRecoveryTerminalPrunedFrame()
	}
	return continuitysqlite.VerifiedTerminalPrunedFrame{
		PruneID:            match.PruneID,
		Reference:          match.Reference,
		PruneCertificateID: match.PruneCertificateID,
		FactKind:           match.FactKind,
		HLC:                match.HLC,
	}, nil
}

func malformedRecoveryTerminalPrunedFrame() error {
	return newProblem(CodeRemote, PhaseAttachActivation, ActionRestartRecovery)
}

func mapRecoveryTerminalPruneStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorConflict, continuitysqlite.SyncErrorCursor,
		continuitysqlite.SyncErrorCertificate, continuitysqlite.SyncErrorNotFound:
		return newProblem(CodeConflict, PhaseAttachActivation, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseAttachActivation, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
}
