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
)

// recoveryTerminalSealedFrame opens one exact retained envelope under the
// pinned recovery authority. Certificate bytes come from the canonical local
// authority snapshot; the protected credential contributes only its project
// root for the envelope-selected generation key.
func (coordinator *Coordinator) recoveryTerminalSealedFrame(
	ctx context.Context,
	projectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
	binding continuitysqlite.SyncAuthorityBinding,
	inbox continuitysqlite.OpaqueSyncFrame,
) (continuitysqlite.VerifiedSyncFrame, error) {
	if ctx == nil {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeInvalid, PhaseAttachActivation, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.VerifiedSyncFrame{}, err
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) ||
		projectID.Validate() != nil || prepared.Validate() != nil {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeInvalid, PhaseAttachActivation, ActionCorrectInput)
	}
	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	if projectID != prepared.ProjectID || coordinator.remote.Endpoint() != prepared.RelayURL ||
		writerEnvironmentID != prepared.Certificate.EnvironmentID ||
		!recoveryDownloadBindingMatchesCredential(binding, prepared) || len(inbox.SealedEnvelope) == 0 ||
		inbox.PrunedArrival != nil || inbox.Quarantined || inbox.ArrivalSequence < 1 {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeInvalid, PhaseAttachActivation, ActionRestartRecovery)
	}

	sealed, err := protocol.ParseSealedFact(inbox.SealedEnvelope)
	if err != nil || sealed.Header.ChannelID != protocol.ChannelID(binding.ChannelID) ||
		protocol.EnvelopeDigest(sealed) != protocol.Digest(inbox.EnvelopeDigest) {
		return continuitysqlite.VerifiedSyncFrame{}, malformedRecoveryTerminalPrunedFrame()
	}
	states, err := coordinator.store.CurrentSyncEnvironmentStates(
		ctx, projectID, binding, []continuity.EnvironmentID{sealed.Header.EnvironmentID},
	)
	if err != nil {
		return continuitysqlite.VerifiedSyncFrame{}, mapRecoveryTerminalStoreError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.VerifiedSyncFrame{}, err
	}
	if len(states) != 1 {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	stored := states[0].Certificate
	certificate, err := protocol.ParseEnvironmentCertificate(stored.CertificateBytes)
	if err != nil {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	canonicalCertificate, err := certificate.MarshalBinary()
	if err != nil || !bytes.Equal(canonicalCertificate, stored.CertificateBytes) ||
		protocol.CertificateID(certificate) != protocol.Digest(stored.CertificateID) ||
		certificate.ProjectID != projectID || certificate.ChannelID != protocol.ChannelID(binding.ChannelID) ||
		certificate.EnvironmentID != sealed.Header.EnvironmentID ||
		certificate.MembershipGeneration != stored.JoinMembershipGeneration ||
		certificate.ExpiresAtMillis != stored.ExpiresAtMillis ||
		!recoveryTerminalCertificateModeMatches(certificate.Mode, stored.Mode) ||
		crypto.VerifyEnvironmentCertificate(certificate, prepared.AdminPublicKey) != nil {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	if sealed.Header.CertificateID != protocol.Digest(stored.CertificateID) {
		return continuitysqlite.VerifiedSyncFrame{}, malformedRecoveryTerminalPrunedFrame()
	}
	key, err := crypto.DeriveGenerationKey(prepared.ProjectRoot, projectID, sealed.Header.KeyGeneration)
	if err != nil {
		return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeAuthorization, PhaseAttachActivation, ActionCheckRecoveryAuthority)
	}
	fact, err := crypto.OpenFact(sealed, key, certificate, prepared.AdminPublicKey)
	if err != nil {
		if contextErr := contextError(ctx, err); contextErr != nil {
			return continuitysqlite.VerifiedSyncFrame{}, contextErr
		}
		if errors.Is(err, crypto.ErrAuthenticationFailed) || errors.Is(err, crypto.ErrInvalidGenerationKey) ||
			errors.Is(err, crypto.ErrGenerationBinding) {
			return continuitysqlite.VerifiedSyncFrame{}, newProblem(CodeAuthorization, PhaseAttachActivation, ActionCheckRecoveryAuthority)
		}
		return continuitysqlite.VerifiedSyncFrame{}, malformedRecoveryTerminalPrunedFrame()
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.VerifiedSyncFrame{}, err
	}
	return continuitysqlite.VerifiedSyncFrame{
		ArrivalSequence:        inbox.ArrivalSequence,
		PreviousEnvelopeDigest: [32]byte(sealed.Header.PreviousEnvelopeDigest),
		EnvelopeDigest:         inbox.EnvelopeDigest,
		CertificateID:          [32]byte(sealed.Header.CertificateID),
		KeyGeneration:          sealed.Header.KeyGeneration,
		Nonce:                  [24]byte(sealed.Header.Nonce),
		Fact:                   fact,
	}, nil
}

func recoveryTerminalCertificateModeMatches(protocolMode protocol.EnvironmentMode, storedMode continuitysqlite.SyncEnvironmentMode) bool {
	return (protocolMode == protocol.EnvironmentTrusted && storedMode == continuitysqlite.SyncEnvironmentTrusted) ||
		(protocolMode == protocol.EnvironmentEphemeral && storedMode == continuitysqlite.SyncEnvironmentEphemeral)
}

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
		return continuitysqlite.VerifiedTerminalPrunedFrame{}, mapRecoveryTerminalStoreError(ctx, err)
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

func mapRecoveryTerminalStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorConflict:
		if syncErr.Field == "sync_authority_candidate" {
			return newProblem(CodeConflict, PhaseAttachActivation, ActionRetry)
		}
		return newProblem(CodeConflict, PhaseAttachActivation, ActionRestartRecovery)
	case continuitysqlite.SyncErrorCursor, continuitysqlite.SyncErrorCertificate, continuitysqlite.SyncErrorNotFound:
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
