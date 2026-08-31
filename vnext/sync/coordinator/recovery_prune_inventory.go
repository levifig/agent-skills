package coordinator

import (
	"bytes"
	"context"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

type recoveryPruneID [32]byte

func (recoveryPruneID) String() string { return "[REDACTED recovery prune ID]" }

func (recoveryPruneID) GoString() string {
	return "coordinator.recoveryPruneID([REDACTED])"
}

type recoveryPruneCertificateID [32]byte

func (recoveryPruneCertificateID) String() string {
	return "[REDACTED recovery prune certificate ID]"
}

func (recoveryPruneCertificateID) GoString() string {
	return "coordinator.recoveryPruneCertificateID([REDACTED])"
}

// verifiedRecoveryPruneTarget is the minimum authenticated deletion metadata
// needed by the later downloaded-arrival join. It excludes capsule bytes,
// certificate bytes, payloads, bearer material, and the project root.
type verifiedRecoveryPruneTarget struct {
	reference continuitysqlite.VerifiedPruneReference
	factKind  continuity.FactKind
	hlc       continuity.HybridTime
}

func (verifiedRecoveryPruneTarget) String() string {
	return "[REDACTED verified recovery prune target]"
}

func (verifiedRecoveryPruneTarget) GoString() string {
	return "coordinator.verifiedRecoveryPruneTarget([REDACTED])"
}

// verifiedRecoveryPrune is a secret-free projection of one fully verified
// signed certificate and authenticated bootstrap capsule.
type verifiedRecoveryPrune struct {
	pruneSequence          int64
	pruneID                recoveryPruneID
	pruneCertificateID     recoveryPruneCertificateID
	membershipGeneration   uint32
	barrierArrivalSequence int64
	closure                continuitysqlite.VerifiedPruneReference
	scratchpadSubject      continuity.SubjectID
	targets                []verifiedRecoveryPruneTarget
}

func (verifiedRecoveryPrune) String() string {
	return "[REDACTED verified recovery prune]"
}

func (verifiedRecoveryPrune) GoString() string {
	return "coordinator.verifiedRecoveryPrune([REDACTED])"
}

type verifiedRecoveryPruneInventoryPage struct {
	snapshot           relay.PruneInventorySnapshot
	afterPruneSequence int64
	prunes             []verifiedRecoveryPrune
	more               bool
}

func (verifiedRecoveryPruneInventoryPage) String() string {
	return "[REDACTED verified recovery prune inventory page]"
}

func (verifiedRecoveryPruneInventoryPage) GoString() string {
	return "coordinator.verifiedRecoveryPruneInventoryPage([REDACTED])"
}

type recoveryPruneInventoryScanOptions struct {
	firstRequestSnapshot    *relay.PruneInventorySnapshot
	firstAfterPruneSequence int64
	// firstMembershipGeneration is the generation of the last verified prune
	// and fences monotonicity when a caller resumes from a verified suffix.
	firstMembershipGeneration uint32
	onPage                    func(verifiedRecoveryPruneInventoryPage) error
}

type recoveryPruneInventoryScanResult struct {
	snapshot relay.PruneInventorySnapshot
}

// scanRecoveryPruneInventory streams one exact prune snapshot through bounded
// pages. Each callback receives only after the complete page has passed relay
// projection, historical witness, signature, and capsule authentication.
func (coordinator *Coordinator) scanRecoveryPruneInventory(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
	binding continuitysqlite.SyncAuthorityBinding,
	options recoveryPruneInventoryScanOptions,
) (recoveryPruneInventoryScanResult, error) {
	if ctx == nil {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhasePruneInventory, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return recoveryPruneInventoryScanResult{}, err
	}
	if expectedProjectID.Validate() != nil || prepared.Validate() != nil {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	if writerEnvironmentID.Validate() != nil {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	}
	if expectedProjectID != prepared.ProjectID || coordinator.remote.Endpoint() != prepared.RelayURL ||
		writerEnvironmentID != prepared.Certificate.EnvironmentID {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}
	if !recoveryDownloadBindingMatchesCredential(binding, prepared) {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	}
	if options.firstAfterPruneSequence < 0 ||
		(options.firstAfterPruneSequence == 0 && options.firstMembershipGeneration != 0) ||
		(options.firstAfterPruneSequence != 0 && (options.firstRequestSnapshot == nil ||
			options.firstMembershipGeneration == 0 ||
			options.firstMembershipGeneration > binding.MembershipGeneration)) {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhasePruneInventory, ActionRestartRecovery)
	}
	if options.firstRequestSnapshot != nil && !validRecoveryPruneInventorySnapshot(
		*options.firstRequestSnapshot, binding, options.firstAfterPruneSequence,
	) {
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInvalid, PhasePruneInventory, ActionRestartRecovery)
	}
	if err := coordinator.validateRecoveryPruneCredential(
		ctx, expectedProjectID, prepared, binding, writerEnvironmentID,
	); err != nil {
		return recoveryPruneInventoryScanResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return recoveryPruneInventoryScanResult{}, err
	}
	bootstrapKey, err := crypto.DerivePruneBootstrapKey(
		prepared.ProjectRoot, expectedProjectID, protocol.PruneBootstrapPurposeVersionV1,
	)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return recoveryPruneInventoryScanResult{}, contextErr
		}
		return recoveryPruneInventoryScanResult{}, newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
	}
	if err := ctx.Err(); err != nil {
		return recoveryPruneInventoryScanResult{}, err
	}

	wantChannel := relay.ChannelAuthority{
		ChannelID:       relay.ChannelID(binding.ChannelID),
		RelayGeneration: relay.RelayGeneration(binding.RelayGeneration),
		AdminPublicKey:  relay.PublicKey(binding.AdminPublicKey),
	}
	var (
		pinnedSnapshot               relay.PruneInventorySnapshot
		haveSnapshot                 bool
		after                        = options.firstAfterPruneSequence
		previousMembershipGeneration = options.firstMembershipGeneration
	)
	for {
		if err := ctx.Err(); err != nil {
			return recoveryPruneInventoryScanResult{}, err
		}
		pageStartAfter := after
		authorization := recoveryDownloadAuthorization(prepared)
		request := relay.PruneInventoryRequest{
			Authorization: relay.InventoryAuthorization{Environment: &authorization},
			After:         after,
			Limit:         relay.MaxPruneInventoryPage,
		}
		if haveSnapshot {
			snapshot := pinnedSnapshot
			request.Snapshot = &snapshot
		} else if options.firstRequestSnapshot != nil {
			snapshot := *options.firstRequestSnapshot
			request.Snapshot = &snapshot
		}
		page, err := coordinator.remote.PruneInventory(ctx, request)
		if err != nil {
			return recoveryPruneInventoryScanResult{}, mapRecoveryPruneInventoryRelayError(ctx, err)
		}
		if err := ctx.Err(); err != nil {
			return recoveryPruneInventoryScanResult{}, err
		}
		if page.Channel != wantChannel ||
			!validRecoveryPruneInventorySnapshot(page.Snapshot, binding, after) ||
			len(page.Prunes) > relay.MaxPruneInventoryPage ||
			(page.More && len(page.Prunes) != relay.MaxPruneInventoryPage) {
			return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
		}
		if !haveSnapshot {
			pinnedSnapshot = page.Snapshot
			haveSnapshot = true
			if options.firstRequestSnapshot != nil && pinnedSnapshot != *options.firstRequestSnapshot {
				return recoveryPruneInventoryScanResult{}, newProblem(CodeConflict, PhasePruneInventory, ActionRetry)
			}
		} else if page.Snapshot != pinnedSnapshot {
			return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
		}

		if len(page.Prunes) == 0 {
			if page.More || after != pinnedSnapshot.PruneHead {
				return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
			}
			if options.onPage != nil {
				if err := ctx.Err(); err != nil {
					return recoveryPruneInventoryScanResult{}, err
				}
				if err := options.onPage(verifiedRecoveryPruneInventoryPage{
					snapshot: pinnedSnapshot, afterPruneSequence: pageStartAfter,
				}); err != nil {
					return recoveryPruneInventoryScanResult{}, err
				}
				if err := ctx.Err(); err != nil {
					return recoveryPruneInventoryScanResult{}, err
				}
			}
			return recoveryPruneInventoryScanResult{snapshot: pinnedSnapshot}, nil
		}
		verifiedPrunes := make([]verifiedRecoveryPrune, 0, len(page.Prunes))
		witnessCache := make(map[uint32][]protocol.EnvironmentCertificate, len(page.Prunes))
		pagePruneIDs := make(map[recoveryPruneID]struct{}, len(page.Prunes))
		pageCertificateIDs := make(map[recoveryPruneCertificateID]struct{}, len(page.Prunes))
		for _, record := range page.Prunes {
			if err := ctx.Err(); err != nil {
				return recoveryPruneInventoryScanResult{}, err
			}
			if after == math.MaxInt64 || record.PruneSequence != after+1 ||
				record.PruneSequence > pinnedSnapshot.PruneHead {
				return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
			}
			verified, err := coordinator.verifyRecoveryPruneInventoryRecord(
				ctx, expectedProjectID, binding, bootstrapKey, wantChannel,
				pinnedSnapshot, record, witnessCache,
			)
			if err != nil {
				return recoveryPruneInventoryScanResult{}, err
			}
			if previousMembershipGeneration != 0 &&
				verified.membershipGeneration < previousMembershipGeneration {
				return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
			}
			if _, duplicate := pagePruneIDs[verified.pruneID]; duplicate {
				return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
			}
			if _, duplicate := pageCertificateIDs[verified.pruneCertificateID]; duplicate {
				return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
			}
			pagePruneIDs[verified.pruneID] = struct{}{}
			pageCertificateIDs[verified.pruneCertificateID] = struct{}{}
			previousMembershipGeneration = verified.membershipGeneration
			verifiedPrunes = append(verifiedPrunes, verified)
			after = record.PruneSequence
		}
		if page.More {
			if after >= pinnedSnapshot.PruneHead {
				return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
			}
		} else if after != pinnedSnapshot.PruneHead {
			return recoveryPruneInventoryScanResult{}, malformedRecoveryPruneInventory()
		}
		if options.onPage != nil {
			if err := ctx.Err(); err != nil {
				return recoveryPruneInventoryScanResult{}, err
			}
			callbackPage := verifiedRecoveryPruneInventoryPage{
				snapshot:           pinnedSnapshot,
				afterPruneSequence: pageStartAfter,
				prunes:             cloneVerifiedRecoveryPrunes(verifiedPrunes),
				more:               page.More,
			}
			if err := options.onPage(callbackPage); err != nil {
				return recoveryPruneInventoryScanResult{}, err
			}
			if err := ctx.Err(); err != nil {
				return recoveryPruneInventoryScanResult{}, err
			}
		}
		if !page.More {
			return recoveryPruneInventoryScanResult{snapshot: pinnedSnapshot}, nil
		}
	}
}

func (coordinator *Coordinator) validateRecoveryPruneCredential(
	ctx context.Context,
	projectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
	binding continuitysqlite.SyncAuthorityBinding,
	writerEnvironmentID continuity.EnvironmentID,
) error {
	certificateBytes, err := prepared.Certificate.MarshalBinary()
	if err != nil {
		return newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}
	states, err := coordinator.store.CurrentSyncEnvironmentStates(
		ctx, projectID, binding, []continuity.EnvironmentID{writerEnvironmentID},
	)
	if err != nil {
		return mapRecoveryPruneInventoryStoreError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(states) != 1 {
		return newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
	}
	local := states[0].Certificate
	if local.EnvironmentID != string(writerEnvironmentID) ||
		local.CertificateID != [32]byte(protocol.CertificateID(prepared.Certificate)) ||
		!bytes.Equal(local.CertificateBytes, certificateBytes) ||
		local.Mode != continuitysqlite.SyncEnvironmentTrusted ||
		local.ExpiresAtMillis != 0 ||
		local.JoinMembershipGeneration != prepared.Certificate.MembershipGeneration ||
		local.Retirement != nil {
		return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	}
	return nil
}

func validRecoveryPruneInventorySnapshot(
	snapshot relay.PruneInventorySnapshot,
	binding continuitysqlite.SyncAuthorityBinding,
	after int64,
) bool {
	return snapshot.MembershipGeneration == binding.MembershipGeneration &&
		snapshot.ArrivalHead == binding.InventoryArrivalHead &&
		snapshot.PruneHead >= 0 && snapshot.PruneHead <= snapshot.ArrivalHead &&
		after >= 0 && after <= snapshot.PruneHead
}

func (coordinator *Coordinator) verifyRecoveryPruneInventoryRecord(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	bootstrapKey crypto.PruneBootstrapKey,
	channel relay.ChannelAuthority,
	snapshot relay.PruneInventorySnapshot,
	record relay.PruneInventoryRecord,
	witnessCache map[uint32][]protocol.EnvironmentCertificate,
) (verifiedRecoveryPrune, error) {
	if err := record.Certificate.Validate(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return verifiedRecoveryPrune{}, contextErr
		}
		return verifiedRecoveryPrune{}, malformedRecoveryPruneInventory()
	}
	parsed, err := protocol.ParsePruneCertificate(record.Certificate.CertificateBytes)
	if err != nil || !recoveryPruneCertificateMatches(parsed, record.Certificate, channel) ||
		parsed.MembershipGeneration > binding.MembershipGeneration ||
		parsed.BarrierArrivalSequence > snapshot.ArrivalHead {
		if contextErr := ctx.Err(); contextErr != nil {
			return verifiedRecoveryPrune{}, contextErr
		}
		return verifiedRecoveryPrune{}, malformedRecoveryPruneInventory()
	}
	if err := ctx.Err(); err != nil {
		return verifiedRecoveryPrune{}, err
	}
	witnesses, found := witnessCache[parsed.MembershipGeneration]
	if !found {
		witnesses, err = coordinator.recoveryPruneWitnessCertificates(
			ctx, expectedProjectID, binding, parsed.MembershipGeneration,
		)
		if err != nil {
			return verifiedRecoveryPrune{}, err
		}
		witnessCache[parsed.MembershipGeneration] = witnesses
	}
	if err := ctx.Err(); err != nil {
		return verifiedRecoveryPrune{}, err
	}
	if err := crypto.VerifyPruneCertificate(
		parsed, witnesses, protocol.PublicKey(binding.AdminPublicKey),
	); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return verifiedRecoveryPrune{}, contextErr
		}
		return verifiedRecoveryPrune{}, malformedRecoveryPruneInventory()
	}
	if err := ctx.Err(); err != nil {
		return verifiedRecoveryPrune{}, err
	}
	plaintext, err := crypto.OpenPruneBootstrap(parsed.Capsule, bootstrapKey)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return verifiedRecoveryPrune{}, contextErr
		}
		// Wrong protected authority, authentication-tag corruption, and
		// authenticated plaintext disagreement share one static oracle.
		return verifiedRecoveryPrune{}, newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	}
	if err := ctx.Err(); err != nil {
		return verifiedRecoveryPrune{}, err
	}
	if !recoveryPruneBootstrapMatchesManifest(plaintext, parsed.Manifest) {
		return verifiedRecoveryPrune{}, malformedRecoveryPruneInventory()
	}
	targets := make([]verifiedRecoveryPruneTarget, len(parsed.Manifest.Targets))
	for index, reference := range parsed.Manifest.Targets {
		if err := ctx.Err(); err != nil {
			return verifiedRecoveryPrune{}, err
		}
		entry := plaintext.Entries[index]
		targets[index] = verifiedRecoveryPruneTarget{
			reference: recoveryVerifiedPruneReference(reference),
			factKind:  entry.FactKind,
			hlc:       entry.HLC,
		}
	}
	return verifiedRecoveryPrune{
		pruneSequence:          record.PruneSequence,
		pruneID:                recoveryPruneID(parsed.PruneID),
		pruneCertificateID:     recoveryPruneCertificateID(protocol.PruneCertificateID(parsed)),
		membershipGeneration:   parsed.MembershipGeneration,
		barrierArrivalSequence: parsed.BarrierArrivalSequence,
		closure:                recoveryVerifiedPruneReference(parsed.Closure),
		scratchpadSubject:      plaintext.ScratchpadSubject,
		targets:                targets,
	}, nil
}

func (coordinator *Coordinator) recoveryPruneWitnessCertificates(
	ctx context.Context,
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	membershipGeneration uint32,
) ([]protocol.EnvironmentCertificate, error) {
	authority, err := coordinator.store.CurrentSyncPruneWitnessAuthorityUnderBinding(
		ctx, projectID, binding, membershipGeneration,
	)
	if err != nil {
		return nil, mapRecoveryPruneInventoryStoreError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if authority.Binding != binding || authority.MembershipGeneration != membershipGeneration ||
		len(authority.Environments) > relay.MaxPruneAuthorityEnvironments {
		return nil, newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
	}
	certificates := make([]protocol.EnvironmentCertificate, 0, len(authority.Environments))
	previousEnvironmentID := ""
	for _, persisted := range authority.Environments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if persisted.EnvironmentID <= previousEnvironmentID ||
			persisted.JoinMembershipGeneration == 0 ||
			persisted.JoinMembershipGeneration > membershipGeneration ||
			(persisted.Retirement != nil && persisted.Retirement.MembershipGeneration <= membershipGeneration) {
			return nil, newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
		}
		certificate, err := protocol.ParseEnvironmentCertificate(persisted.CertificateBytes)
		if err != nil || !recoveryPruneWitnessCertificateMatches(projectID, binding, persisted, certificate) {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			return nil, newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
		}
		if err := crypto.VerifyEnvironmentCertificate(
			certificate, protocol.PublicKey(binding.AdminPublicKey),
		); err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			return nil, newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
		}
		certificates = append(certificates, certificate)
		previousEnvironmentID = persisted.EnvironmentID
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return certificates, nil
}

func recoveryPruneWitnessCertificateMatches(
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	persisted continuitysqlite.SyncEnvironmentCertificate,
	certificate protocol.EnvironmentCertificate,
) bool {
	var mode continuitysqlite.SyncEnvironmentMode
	switch certificate.Mode {
	case protocol.EnvironmentTrusted:
		mode = continuitysqlite.SyncEnvironmentTrusted
	case protocol.EnvironmentEphemeral:
		mode = continuitysqlite.SyncEnvironmentEphemeral
	default:
		return false
	}
	wire, err := certificate.MarshalBinary()
	return err == nil && certificate.ProjectID == projectID &&
		certificate.ChannelID == protocol.ChannelID(binding.ChannelID) &&
		string(certificate.EnvironmentID) == persisted.EnvironmentID &&
		[32]byte(protocol.CertificateID(certificate)) == persisted.CertificateID &&
		bytes.Equal(wire, persisted.CertificateBytes) &&
		mode == persisted.Mode && certificate.ExpiresAtMillis == persisted.ExpiresAtMillis &&
		certificate.MembershipGeneration == persisted.JoinMembershipGeneration
}

func recoveryPruneCertificateMatches(
	parsed protocol.PruneCertificate,
	outer relay.PruneCertificate,
	channel relay.ChannelAuthority,
) bool {
	if parsed.ChannelID != protocol.ChannelID(outer.ChannelID) ||
		parsed.ChannelID != protocol.ChannelID(channel.ChannelID) ||
		parsed.RelayGeneration != protocol.RelayGeneration(channel.RelayGeneration) ||
		parsed.PruneID != protocol.Digest(outer.PruneID) ||
		parsed.MembershipGeneration != outer.MembershipGeneration ||
		parsed.BarrierArrivalSequence != outer.Barrier ||
		!recoveryPruneReferenceMatches(parsed.Closure, outer.Closure) ||
		protocol.PruneCertificateID(parsed) != protocol.Digest(outer.CertificateID) ||
		len(parsed.Manifest.Targets) != len(outer.Targets) {
		return false
	}
	for index, target := range parsed.Manifest.Targets {
		if !recoveryPruneReferenceMatches(target, outer.Targets[index]) {
			return false
		}
	}
	return true
}

func recoveryPruneReferenceMatches(reference protocol.PruneReference, target relay.PruneTarget) bool {
	return reference.FactID == continuity.FactID(target.FactID) &&
		reference.EnvironmentID == continuity.EnvironmentID(target.EnvironmentID) &&
		reference.EnvironmentSequence == target.EnvironmentSequence &&
		reference.ArrivalSequence == target.ArrivalSequence &&
		reference.EnvelopeDigest == protocol.Digest(target.EnvelopeDigest) &&
		reference.CertificateID == protocol.Digest(target.CertificateID) &&
		reference.PreviousEnvelopeDigest == protocol.Digest(target.PreviousEnvelopeDigest) &&
		reference.KeyGeneration == target.KeyGeneration &&
		reference.Nonce == protocol.Nonce(target.Nonce)
}

func recoveryPruneBootstrapMatchesManifest(
	plaintext protocol.PruneBootstrapPlaintext,
	manifest protocol.PruneManifest,
) bool {
	if len(plaintext.Entries) != len(manifest.Targets) {
		return false
	}
	for index, target := range manifest.Targets {
		if plaintext.Entries[index].PruneReferenceDigest != protocol.PruneReferenceDigest(target) {
			return false
		}
	}
	return true
}

func recoveryVerifiedPruneReference(reference protocol.PruneReference) continuitysqlite.VerifiedPruneReference {
	return continuitysqlite.VerifiedPruneReference{
		FactID:                 reference.FactID,
		EnvironmentID:          reference.EnvironmentID,
		EnvironmentSequence:    reference.EnvironmentSequence,
		ArrivalSequence:        reference.ArrivalSequence,
		EnvelopeDigest:         [32]byte(reference.EnvelopeDigest),
		CertificateID:          [32]byte(reference.CertificateID),
		PreviousEnvelopeDigest: [32]byte(reference.PreviousEnvelopeDigest),
		KeyGeneration:          reference.KeyGeneration,
		Nonce:                  [24]byte(reference.Nonce),
	}
}

func cloneVerifiedRecoveryPrunes(prunes []verifiedRecoveryPrune) []verifiedRecoveryPrune {
	cloned := append([]verifiedRecoveryPrune(nil), prunes...)
	for index := range cloned {
		cloned[index].targets = append([]verifiedRecoveryPruneTarget(nil), prunes[index].targets...)
	}
	return cloned
}

func malformedRecoveryPruneInventory() error {
	return newProblem(CodeRemote, PhasePruneInventory, ActionRestartRecovery)
}

func mapRecoveryPruneInventoryRelayError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	switch {
	case errors.Is(err, relay.ErrUnauthenticated), errors.Is(err, relay.ErrNotFound):
		return newProblem(CodeAuthorization, PhasePruneInventory, ActionCheckRecoveryAuthority)
	case errors.Is(err, relay.ErrMembershipChanged):
		return newProblem(CodeConflict, PhasePruneInventory, ActionRetry)
	case errors.Is(err, relay.ErrGenerationMismatch), errors.Is(err, relay.ErrRollback),
		errors.Is(err, relay.ErrRetired), errors.Is(err, relay.ErrExpired):
		return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	case errors.Is(err, relay.ErrInvalidArgument), errors.Is(err, relay.ErrUnverified),
		errors.Is(err, relay.ErrImmutableConflict), errors.Is(err, relay.ErrSourceGap),
		errors.Is(err, relay.ErrPreviousDigest), errors.Is(err, relay.ErrNonceReuse),
		errors.Is(err, relay.ErrAcknowledgementRequired):
		return malformedRecoveryPruneInventory()
	default:
		return newProblem(CodeUnavailable, PhasePruneInventory, ActionRetry)
	}
}

func mapRecoveryPruneInventoryStoreError(ctx context.Context, err error) error {
	if contextErr := contextError(ctx, err); contextErr != nil {
		return contextErr
	}
	var syncErr *continuitysqlite.SyncError
	if !errors.As(err, &syncErr) {
		return newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
	}
	switch syncErr.Code {
	case continuitysqlite.SyncErrorConflict:
		switch syncErr.Field {
		case "sync_authority", "sync_authority_candidate", "sync_authority_recovery":
			return newProblem(CodeConflict, PhasePruneInventory, ActionRetry)
		case "prune_witness_authority":
			return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
		default:
			return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
		}
	case continuitysqlite.SyncErrorCursor:
		return newProblem(CodeConflict, PhasePruneInventory, ActionRetry)
	case continuitysqlite.SyncErrorCertificate, continuitysqlite.SyncErrorNotFound:
		return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	case continuitysqlite.SyncErrorStore:
		if syncErr.Field != "" {
			return newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
		}
		return newProblem(CodeUnavailable, PhasePruneInventory, ActionRetry)
	default:
		return newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
	}
}
