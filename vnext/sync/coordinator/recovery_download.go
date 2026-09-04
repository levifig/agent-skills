package coordinator

import (
	"bytes"
	"context"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

const (
	maximumRecoveryDownloadPageBytes = 4 * protocol.MaxEnvelopeBytes
	recoveryDownloadPageLimit        = maximumRecoveryDownloadPageBytes / protocol.MaxEnvelopeBytes
)

// downloadRecoverySnapshot stages the exact relay prefix authorized by one
// verified recovery binding. Bearer material is rebuilt for each request and
// is never retained by the coordinator or persistence layer.
func (coordinator *Coordinator) downloadRecoverySnapshot(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
	binding continuitysqlite.SyncAuthorityBinding,
) (continuitysqlite.SyncProgress, error) {
	if ctx == nil {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseAttachDownload, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.SyncProgress{}, err
	}
	if expectedProjectID.Validate() != nil || prepared.Validate() != nil {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	if writerEnvironmentID.Validate() != nil {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	}
	if expectedProjectID != prepared.ProjectID ||
		coordinator.remote.Endpoint() != prepared.RelayURL ||
		writerEnvironmentID != prepared.Certificate.EnvironmentID {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}
	if !recoveryDownloadBindingMatchesCredential(binding, prepared) {
		return continuitysqlite.SyncProgress{}, newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	}

	certificateBytes, err := prepared.Certificate.MarshalBinary()
	if err != nil {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}
	certificateID := protocol.CertificateID(prepared.Certificate)
	states, err := coordinator.store.CurrentSyncEnvironmentStates(
		ctx, expectedProjectID, binding, []continuity.EnvironmentID{writerEnvironmentID},
	)
	if err != nil {
		return continuitysqlite.SyncProgress{}, mapRecoveryDownloadStoreError(ctx, err)
	}
	if len(states) != 1 {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
	localCertificate := states[0].Certificate
	if localCertificate.EnvironmentID != string(writerEnvironmentID) ||
		localCertificate.CertificateID != [32]byte(certificateID) ||
		!bytes.Equal(localCertificate.CertificateBytes, certificateBytes) ||
		localCertificate.Mode != continuitysqlite.SyncEnvironmentTrusted ||
		localCertificate.ExpiresAtMillis != 0 ||
		localCertificate.JoinMembershipGeneration != prepared.Certificate.MembershipGeneration ||
		localCertificate.Retirement != nil {
		return continuitysqlite.SyncProgress{}, newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	}

	progress, err := coordinator.store.CurrentSyncProgress(ctx, expectedProjectID)
	if err != nil {
		return continuitysqlite.SyncProgress{}, mapRecoveryDownloadStoreError(ctx, err)
	}
	if !validRecoveryDownloadProgressShape(progress, expectedProjectID, binding) {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
	if progress.DownloadedCursor > binding.InventoryArrivalHead || progress.RelayHead > binding.InventoryArrivalHead {
		if _, err := coordinator.store.CurrentSyncEnvironmentStates(
			ctx, expectedProjectID, binding, []continuity.EnvironmentID{writerEnvironmentID},
		); err != nil {
			return continuitysqlite.SyncProgress{}, mapRecoveryDownloadStoreError(ctx, err)
		}
		return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
	if progress.ActivationState == continuitysqlite.SyncActivationAttached &&
		(progress.AppliedCursor != binding.InventoryArrivalHead ||
			progress.DownloadedCursor != binding.InventoryArrivalHead ||
			progress.RelayHead != binding.InventoryArrivalHead) {
		return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
	if progress.DownloadedCursor == binding.InventoryArrivalHead {
		progress, err = coordinator.store.StageSyncPageUnderAuthority(
			ctx,
			expectedProjectID,
			binding,
			binding.InventoryArrivalHead,
			binding.InventoryArrivalHead,
			nil,
		)
		if err != nil {
			return continuitysqlite.SyncProgress{}, mapRecoveryDownloadStoreError(ctx, err)
		}
		if !validRecoveryDownloadProgress(progress, expectedProjectID, binding) {
			return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
		}
		return progress, nil
	}

	for progress.DownloadedCursor < binding.InventoryArrivalHead {
		after := progress.DownloadedCursor
		remaining := binding.InventoryArrivalHead - after
		pageLimit := recoveryDownloadPageLimit
		if remaining < int64(pageLimit) {
			pageLimit = int(remaining)
		}
		page, err := coordinator.remote.Page(ctx, relay.PageRequest{
			Authorization: recoveryDownloadAuthorization(prepared),
			After:         after,
			Limit:         pageLimit,
		})
		if err != nil {
			return continuitysqlite.SyncProgress{}, mapRecoveryDownloadRelayError(ctx, err)
		}
		if err := ctx.Err(); err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
		if err := coordinator.retainAndValidateRecoveryDownloadPageSnapshot(
			ctx, expectedProjectID, page, binding,
		); err != nil {
			return continuitysqlite.SyncProgress{}, err
		}
		if len(page.Arrivals) == 0 || len(page.Arrivals) > pageLimit || int64(len(page.Arrivals)) > remaining {
			return continuitysqlite.SyncProgress{}, malformedRecoveryDownloadPage()
		}

		frames := make([]continuitysqlite.OpaqueSyncFrame, 0, len(page.Arrivals))
		pageBytes := 0
		expectedArrival := after + 1
		for _, arrival := range page.Arrivals {
			if err := ctx.Err(); err != nil {
				return continuitysqlite.SyncProgress{}, err
			}
			if arrival.ArrivalSequence != expectedArrival ||
				arrival.ArrivalSequence > binding.InventoryArrivalHead ||
				arrival.ChannelID != relay.ChannelID(binding.ChannelID) {
				return continuitysqlite.SyncProgress{}, malformedRecoveryDownloadPage()
			}
			frame, err := recoveryDownloadFrame(arrival, prepared.RelayGeneration)
			if err != nil {
				return continuitysqlite.SyncProgress{}, malformedRecoveryDownloadPage()
			}
			frameBytes := len(frame.SealedEnvelope) + len(frame.PrunedArrival)
			if frameBytes > maximumRecoveryDownloadPageBytes-pageBytes {
				return continuitysqlite.SyncProgress{}, malformedRecoveryDownloadPage()
			}
			pageBytes += frameBytes
			frames = append(frames, frame)
			expectedArrival++
		}

		progress, err = coordinator.store.StageSyncPageUnderAuthority(
			ctx,
			expectedProjectID,
			binding,
			after,
			binding.InventoryArrivalHead,
			frames,
		)
		if err != nil {
			return continuitysqlite.SyncProgress{}, mapRecoveryDownloadStoreError(ctx, err)
		}
		lastArrival := frames[len(frames)-1].ArrivalSequence
		if !validRecoveryDownloadProgress(progress, expectedProjectID, binding) ||
			progress.RelayHead != binding.InventoryArrivalHead ||
			progress.DownloadedCursor < lastArrival {
			return continuitysqlite.SyncProgress{}, newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
		}
	}
	return progress, nil
}

func recoveryDownloadBindingMatchesCredential(
	binding continuitysqlite.SyncAuthorityBinding,
	prepared credential.TrustedProjectCredential,
) bool {
	return binding.ChannelID == continuitysqlite.SyncChannelID(prepared.ChannelID) &&
		binding.RelayGeneration == [32]byte(prepared.RelayGeneration) &&
		binding.AdminPublicKey == [32]byte(prepared.AdminPublicKey) &&
		binding.MembershipGeneration >= prepared.Certificate.MembershipGeneration &&
		binding.InventoryArrivalHead >= 0 &&
		binding.AuthorityDigestVersion == 2 &&
		binding.AuthorityDigest != ([32]byte{})
}

func validRecoveryDownloadProgress(
	progress continuitysqlite.SyncProgress,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
) bool {
	if !validRecoveryDownloadProgressShape(progress, projectID, binding) ||
		progress.DownloadedCursor > binding.InventoryArrivalHead ||
		progress.RelayHead > binding.InventoryArrivalHead {
		return false
	}
	if progress.DownloadedCursor == binding.InventoryArrivalHead && progress.RelayHead != binding.InventoryArrivalHead {
		return false
	}
	if progress.ActivationState == continuitysqlite.SyncActivationAttached &&
		(progress.AppliedCursor != binding.InventoryArrivalHead ||
			progress.DownloadedCursor != binding.InventoryArrivalHead ||
			progress.RelayHead != binding.InventoryArrivalHead) {
		return false
	}
	return true
}

func validRecoveryDownloadProgressShape(
	progress continuitysqlite.SyncProgress,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
) bool {
	if progress.ProjectID != projectID || progress.ChannelID != binding.ChannelID ||
		(progress.ActivationState != continuitysqlite.SyncActivationStaging &&
			progress.ActivationState != continuitysqlite.SyncActivationAttached) {
		return false
	}
	return progress.AppliedCursor >= 0 &&
		progress.DownloadedCursor >= progress.AppliedCursor &&
		progress.RelayHead >= 0
}

func recoveryDownloadAuthorization(prepared credential.TrustedProjectCredential) relay.EnvironmentAuthorization {
	bearerID := prepared.EnvironmentRelayAuthorization.ID()
	bearerSecret := prepared.EnvironmentRelayAuthorization.Secret()
	return relay.EnvironmentAuthorization{
		ChannelID:       relay.ChannelID(prepared.ChannelID),
		RelayGeneration: relay.RelayGeneration(prepared.RelayGeneration),
		EnvironmentID:   relay.EnvironmentID(prepared.Certificate.EnvironmentID),
		CertificateID:   relay.Digest(protocol.CertificateID(prepared.Certificate)),
		TokenID:         relay.RelayTokenID(bearerID),
		TokenSecret:     relay.RelayTokenSecret(bearerSecret),
	}
}

func (coordinator *Coordinator) retainAndValidateRecoveryDownloadPageSnapshot(
	ctx context.Context,
	projectID continuity.ProjectID,
	page relay.Page,
	binding continuitysqlite.SyncAuthorityBinding,
) error {
	if page.RelayGeneration != relay.RelayGeneration(binding.RelayGeneration) ||
		page.MembershipGeneration == 0 || page.Head < 0 {
		return malformedRecoveryDownloadPage()
	}
	observation := continuitysqlite.SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            binding.ChannelID,
		RelayGeneration:      binding.RelayGeneration,
		AdminPublicKey:       binding.AdminPublicKey,
		MembershipGeneration: page.MembershipGeneration,
		RelayHead:            page.Head,
	}
	retained, err := coordinator.store.AdvanceSyncRelayWatermark(ctx, observation)
	if err != nil {
		return mapRecoveryDownloadWatermarkError(ctx, err)
	}
	if page.MembershipGeneration == binding.MembershipGeneration &&
		page.Head == binding.InventoryArrivalHead {
		if retained != observation {
			return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
		}
		return nil
	}
	if page.MembershipGeneration >= binding.MembershipGeneration &&
		page.Head >= binding.InventoryArrivalHead &&
		(page.MembershipGeneration > binding.MembershipGeneration ||
			page.Head > binding.InventoryArrivalHead) {
		if retained != observation {
			return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
		}
		return newProblem(CodeConflict, PhaseAttachDownload, ActionRetry)
	}
	return malformedRecoveryDownloadPage()
}

func recoveryDownloadFrame(
	arrival relay.Arrival,
	relayGeneration protocol.RelayGeneration,
) (continuitysqlite.OpaqueSyncFrame, error) {
	frame := continuitysqlite.OpaqueSyncFrame{
		ArrivalSequence: arrival.ArrivalSequence,
		EnvelopeDigest:  [32]byte(arrival.EnvelopeDigest),
	}
	switch {
	case arrival.PruneID == nil && arrival.PrunedAt == nil:
		sealed, err := sealedRecoveryArrivalBytes(arrival)
		if err != nil {
			return continuitysqlite.OpaqueSyncFrame{}, err
		}
		frame.SealedEnvelope = sealed
	case arrival.PruneID != nil && arrival.PrunedAt != nil:
		pruned, err := prunedRecoveryArrivalBytes(arrival, relayGeneration)
		if err != nil {
			return continuitysqlite.OpaqueSyncFrame{}, err
		}
		frame.PrunedArrival = pruned
	default:
		return continuitysqlite.OpaqueSyncFrame{}, protocol.ErrInvalidPrunedArrival
	}
	return frame, nil
}

func sealedRecoveryArrivalBytes(arrival relay.Arrival) ([]byte, error) {
	if arrival.PruneID != nil || arrival.PrunedAt != nil || arrival.Ciphertext == nil ||
		arrival.CiphertextSize != int64(len(arrival.Ciphertext)) {
		return nil, protocol.ErrInvalidEnvelope
	}
	if err := arrival.Envelope.Validate(); err != nil {
		return nil, protocol.ErrInvalidEnvelope
	}
	sealed := protocol.SealedFact{
		Header: protocol.FactHeader{
			ProtocolVersion:        arrival.ProtocolVersion,
			CipherSuite:            arrival.CipherSuite,
			ChannelID:              protocol.ChannelID(arrival.ChannelID),
			FactID:                 continuity.FactID(arrival.FactID),
			EnvironmentID:          continuity.EnvironmentID(arrival.EnvironmentID),
			EnvironmentSequence:    arrival.EnvironmentSequence,
			KeyGeneration:          arrival.KeyGeneration,
			PreviousEnvelopeDigest: protocol.Digest(arrival.PreviousEnvelopeDigest),
			CertificateID:          protocol.Digest(arrival.CertificateID),
			Nonce:                  protocol.Nonce(arrival.Nonce),
		},
		Ciphertext: arrival.Ciphertext,
		Signature:  protocol.Signature(arrival.Signature),
	}
	wire, err := sealed.MarshalBinary()
	if err != nil || protocol.EnvelopeDigest(sealed) != protocol.Digest(arrival.EnvelopeDigest) {
		return nil, protocol.ErrInvalidEnvelope
	}
	return wire, nil
}

func prunedRecoveryArrivalBytes(
	arrival relay.Arrival,
	relayGeneration protocol.RelayGeneration,
) ([]byte, error) {
	if arrival.PruneID == nil || arrival.PrunedAt == nil || arrival.Ciphertext != nil ||
		arrival.ProtocolVersion != protocol.ProtocolVersionV1 ||
		arrival.CipherSuite != protocol.CipherSuiteXChaCha20Poly1305 ||
		arrival.CiphertextSize < relay.MinimumCiphertextBytes ||
		arrival.CiphertextSize > relay.MaxCiphertextBytes ||
		relayGeneration == (protocol.RelayGeneration{}) {
		return nil, protocol.ErrInvalidPrunedArrival
	}
	pruned := protocol.PrunedArrival{
		ChannelID:       protocol.ChannelID(arrival.ChannelID),
		RelayGeneration: relayGeneration,
		PruneID:         protocol.Digest(*arrival.PruneID),
		Reference: protocol.PruneReference{
			FactID:                 continuity.FactID(arrival.FactID),
			EnvironmentID:          continuity.EnvironmentID(arrival.EnvironmentID),
			EnvironmentSequence:    arrival.EnvironmentSequence,
			ArrivalSequence:        arrival.ArrivalSequence,
			EnvelopeDigest:         protocol.Digest(arrival.EnvelopeDigest),
			CertificateID:          protocol.Digest(arrival.CertificateID),
			PreviousEnvelopeDigest: protocol.Digest(arrival.PreviousEnvelopeDigest),
			KeyGeneration:          arrival.KeyGeneration,
			Nonce:                  protocol.Nonce(arrival.Nonce),
		},
	}
	return pruned.MarshalBinary()
}

func malformedRecoveryDownloadPage() error {
	return newProblem(CodeRemote, PhaseAttachDownload, ActionRestartRecovery)
}

func mapRecoveryDownloadRelayError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	switch {
	case errors.Is(err, relay.ErrUnauthenticated), errors.Is(err, relay.ErrNotFound):
		return newProblem(CodeAuthorization, PhaseAttachDownload, ActionCheckRecoveryAuthority)
	case errors.Is(err, relay.ErrMembershipChanged):
		return newProblem(CodeConflict, PhaseAttachDownload, ActionRetry)
	case errors.Is(err, relay.ErrGenerationMismatch), errors.Is(err, relay.ErrRollback),
		errors.Is(err, relay.ErrRetired), errors.Is(err, relay.ErrExpired):
		return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	case errors.Is(err, relay.ErrInvalidArgument), errors.Is(err, relay.ErrUnverified),
		errors.Is(err, relay.ErrImmutableConflict), errors.Is(err, relay.ErrSourceGap),
		errors.Is(err, relay.ErrPreviousDigest), errors.Is(err, relay.ErrNonceReuse),
		errors.Is(err, relay.ErrAcknowledgementRequired):
		return malformedRecoveryDownloadPage()
	default:
		return newProblem(CodeUnavailable, PhaseAttachDownload, ActionRetry)
	}
}

func mapRecoveryDownloadStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorCursor:
		if syncErr.Field == "expected_after" {
			return newProblem(CodeConflict, PhaseAttachDownload, ActionRetry)
		}
		return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	case continuitysqlite.SyncErrorConflict:
		switch syncErr.Field {
		case "frame_bytes", "envelope_digest":
			return malformedRecoveryDownloadPage()
		case "sync_authority", "sync_authority_candidate":
			return newProblem(CodeConflict, PhaseAttachDownload, ActionRetry)
		case "applied_pruned_unverifiable":
			return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
		case "channel_id":
			return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
		default:
			return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
		}
	case continuitysqlite.SyncErrorCertificate, continuitysqlite.SyncErrorNotFound:
		return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseAttachDownload, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
}

func mapRecoveryDownloadWatermarkError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorCursor:
		return malformedRecoveryDownloadPage()
	case continuitysqlite.SyncErrorConflict:
		return newProblem(CodeConflict, PhaseAttachDownload, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhaseAttachDownload, ActionRetry)
	default:
		return newProblem(CodeInternal, PhaseAttachDownload, ActionRepairLocalStore)
	}
}
