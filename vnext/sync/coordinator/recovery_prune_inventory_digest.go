package coordinator

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/protocol"
	"github.com/levifig/loaf/vnext/sync/relay"
)

const recoveryPruneInventoryDigestVersionV1 uint16 = 1

const (
	recoveryPruneInventoryHeaderDomainV1      = "loaf.sync.recovery.prune-inventory.header.v1"
	recoveryPruneInventoryRollingSeedDomainV1 = "loaf.sync.recovery.prune-inventory.rolling-seed.v1"
	recoveryPruneInventoryRollingStepDomainV1 = "loaf.sync.recovery.prune-inventory.rolling-step.v1"
	recoveryPruneInventoryFinalDomainV1       = "loaf.sync.recovery.prune-inventory.final.v1"
	recoveryPruneRecordDomainV1               = "loaf.sync.recovery.prune-inventory.record.v1"
	recoveryPruneTargetSeedDomainV1           = "loaf.sync.recovery.prune-inventory.targets.seed.v1"
	recoveryPruneTargetStepDomainV1           = "loaf.sync.recovery.prune-inventory.targets.step.v1"
	recoveryPruneTargetDomainV1               = "loaf.sync.recovery.prune-inventory.target.v1"
	recoveryPruneInventoryDigestErrorV1       = "invalid recovery prune inventory digest input"
	maximumRecoveryPruneDigestFieldsV1        = 16
	maximumRecoveryPruneDigestTranscriptV1    = 4_096
)

var errInvalidRecoveryPruneInventoryDigestV1 = errors.New(recoveryPruneInventoryDigestErrorV1)

type recoveryPruneInventoryHeaderDigest [32]byte

func (recoveryPruneInventoryHeaderDigest) String() string {
	return "[REDACTED recovery prune inventory header digest]"
}

func (recoveryPruneInventoryHeaderDigest) GoString() string {
	return "coordinator.recoveryPruneInventoryHeaderDigest([REDACTED])"
}

type recoveryPruneInventoryRollingDigest [32]byte

func (recoveryPruneInventoryRollingDigest) String() string {
	return "[REDACTED recovery prune inventory rolling digest]"
}

func (recoveryPruneInventoryRollingDigest) GoString() string {
	return "coordinator.recoveryPruneInventoryRollingDigest([REDACTED])"
}

// recoveryPruneInventoryDigest is the finalized commitment to one complete
// authority-bound prune snapshot. It is deliberately not usable as resumable
// rolling state.
type recoveryPruneInventoryDigest [32]byte

func (recoveryPruneInventoryDigest) String() string {
	return "[REDACTED recovery prune inventory digest]"
}

func (recoveryPruneInventoryDigest) GoString() string {
	return "coordinator.recoveryPruneInventoryDigest([REDACTED])"
}

// recoveryPruneInventoryCheckpoint is the indivisible resume capability for
// one verified prefix. Persistence must retain and compare the complete value;
// its rolling digest alone has no authority.
type recoveryPruneInventoryCheckpoint struct {
	snapshot                 relay.PruneInventorySnapshot
	headerDigest             recoveryPruneInventoryHeaderDigest
	throughPruneSequence     int64
	lastMembershipGeneration uint32
	rollingDigest            recoveryPruneInventoryRollingDigest
}

func (recoveryPruneInventoryCheckpoint) String() string {
	return "[REDACTED recovery prune inventory checkpoint]"
}

func (recoveryPruneInventoryCheckpoint) GoString() string {
	return "coordinator.recoveryPruneInventoryCheckpoint([REDACTED])"
}

func newRecoveryPruneInventoryCheckpointV1(
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	snapshot relay.PruneInventorySnapshot,
) (recoveryPruneInventoryCheckpoint, error) {
	header, err := recoveryPruneInventoryHeaderDigestV1(projectID, binding, snapshot)
	if err != nil {
		return recoveryPruneInventoryCheckpoint{}, err
	}
	rolling, err := recoveryPruneInventoryRollingSeedV1(header)
	if err != nil {
		return recoveryPruneInventoryCheckpoint{}, err
	}
	return recoveryPruneInventoryCheckpoint{
		snapshot:      snapshot,
		headerDigest:  header,
		rollingDigest: rolling,
	}, nil
}

func validateRecoveryPruneInventoryCheckpointV1(
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	checkpoint recoveryPruneInventoryCheckpoint,
) error {
	if !validRecoveryPruneInventorySnapshot(checkpoint.snapshot, binding, checkpoint.throughPruneSequence) ||
		checkpoint.headerDigest == (recoveryPruneInventoryHeaderDigest{}) ||
		checkpoint.rollingDigest == (recoveryPruneInventoryRollingDigest{}) ||
		(checkpoint.throughPruneSequence == 0) != (checkpoint.lastMembershipGeneration == 0) ||
		checkpoint.lastMembershipGeneration > checkpoint.snapshot.MembershipGeneration {
		return errInvalidRecoveryPruneInventoryDigestV1
	}
	header, err := recoveryPruneInventoryHeaderDigestV1(projectID, binding, checkpoint.snapshot)
	if err != nil || header != checkpoint.headerDigest {
		return errInvalidRecoveryPruneInventoryDigestV1
	}
	if checkpoint.throughPruneSequence == 0 {
		seed, seedErr := recoveryPruneInventoryRollingSeedV1(header)
		if seedErr != nil || checkpoint.rollingDigest != seed {
			return errInvalidRecoveryPruneInventoryDigestV1
		}
	}
	return nil
}

func recoveryPruneInventoryHeaderDigestV1(
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	snapshot relay.PruneInventorySnapshot,
) (recoveryPruneInventoryHeaderDigest, error) {
	if projectID.Validate() != nil ||
		binding.ChannelID == (continuitysqlite.SyncChannelID{}) ||
		binding.RelayGeneration == ([32]byte{}) ||
		binding.AdminPublicKey == ([32]byte{}) ||
		binding.MembershipGeneration == 0 ||
		binding.InventoryArrivalHead < 0 ||
		(binding.AuthorityDigestVersion != 1 && binding.AuthorityDigestVersion != 2) ||
		binding.AuthorityDigest == ([32]byte{}) ||
		!validRecoveryPruneInventorySnapshot(snapshot, binding, 0) {
		return recoveryPruneInventoryHeaderDigest{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	digest, err := recoveryPruneInventoryHashV1(
		recoveryPruneInventoryHeaderDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		[]byte(projectID),
		binding.ChannelID[:],
		binding.RelayGeneration[:],
		binding.AdminPublicKey[:],
		recoveryPruneUint32V1(binding.MembershipGeneration),
		recoveryPruneInt64V1(binding.InventoryArrivalHead),
		recoveryPruneUint16V1(binding.AuthorityDigestVersion),
		binding.AuthorityDigest[:],
		recoveryPruneUint32V1(snapshot.MembershipGeneration),
		recoveryPruneInt64V1(snapshot.ArrivalHead),
		recoveryPruneInt64V1(snapshot.PruneHead),
	)
	if err != nil || digest == ([32]byte{}) {
		return recoveryPruneInventoryHeaderDigest{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return recoveryPruneInventoryHeaderDigest(digest), nil
}

func recoveryPruneInventoryRollingSeedV1(
	header recoveryPruneInventoryHeaderDigest,
) (recoveryPruneInventoryRollingDigest, error) {
	if header == (recoveryPruneInventoryHeaderDigest{}) {
		return recoveryPruneInventoryRollingDigest{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	digest, err := recoveryPruneInventoryHashV1(
		recoveryPruneInventoryRollingSeedDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		header[:],
	)
	if err != nil || digest == ([32]byte{}) {
		return recoveryPruneInventoryRollingDigest{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return recoveryPruneInventoryRollingDigest(digest), nil
}

func advanceRecoveryPruneInventoryCheckpointV1(
	checkpoint recoveryPruneInventoryCheckpoint,
	prune verifiedRecoveryPrune,
) (recoveryPruneInventoryCheckpoint, error) {
	if checkpoint.headerDigest == (recoveryPruneInventoryHeaderDigest{}) ||
		checkpoint.rollingDigest == (recoveryPruneInventoryRollingDigest{}) ||
		checkpoint.throughPruneSequence < 0 || checkpoint.throughPruneSequence == math.MaxInt64 ||
		prune.pruneSequence != checkpoint.throughPruneSequence+1 ||
		prune.pruneSequence > checkpoint.snapshot.PruneHead ||
		prune.membershipGeneration < checkpoint.lastMembershipGeneration ||
		prune.membershipGeneration > checkpoint.snapshot.MembershipGeneration ||
		prune.barrierArrivalSequence > checkpoint.snapshot.ArrivalHead {
		return recoveryPruneInventoryCheckpoint{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	recordDigest, err := recoveryPruneRecordDigestV1(prune)
	if err != nil {
		return recoveryPruneInventoryCheckpoint{}, err
	}
	next, err := recoveryPruneInventoryHashV1(
		recoveryPruneInventoryRollingStepDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		checkpoint.headerDigest[:],
		checkpoint.rollingDigest[:],
		recoveryPruneInt64V1(prune.pruneSequence),
		recordDigest[:],
	)
	if err != nil || next == ([32]byte{}) {
		return recoveryPruneInventoryCheckpoint{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	checkpoint.throughPruneSequence = prune.pruneSequence
	checkpoint.lastMembershipGeneration = prune.membershipGeneration
	checkpoint.rollingDigest = recoveryPruneInventoryRollingDigest(next)
	return checkpoint, nil
}

func finalizeRecoveryPruneInventoryDigestV1(
	checkpoint recoveryPruneInventoryCheckpoint,
) (recoveryPruneInventoryDigest, error) {
	if checkpoint.headerDigest == (recoveryPruneInventoryHeaderDigest{}) ||
		checkpoint.rollingDigest == (recoveryPruneInventoryRollingDigest{}) ||
		checkpoint.throughPruneSequence != checkpoint.snapshot.PruneHead ||
		(checkpoint.throughPruneSequence == 0) != (checkpoint.lastMembershipGeneration == 0) {
		return recoveryPruneInventoryDigest{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	digest, err := recoveryPruneInventoryHashV1(
		recoveryPruneInventoryFinalDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		checkpoint.headerDigest[:],
		recoveryPruneInt64V1(checkpoint.throughPruneSequence),
		recoveryPruneUint32V1(checkpoint.lastMembershipGeneration),
		checkpoint.rollingDigest[:],
	)
	if err != nil || digest == ([32]byte{}) {
		return recoveryPruneInventoryDigest{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return recoveryPruneInventoryDigest(digest), nil
}

func recoveryPruneRecordDigestV1(prune verifiedRecoveryPrune) ([32]byte, error) {
	if err := validateRecoveryPruneProjectionV1(prune); err != nil {
		return [32]byte{}, err
	}
	closureDigest, err := recoveryPruneReferenceDigestV1(prune.closure)
	if err != nil {
		return [32]byte{}, err
	}
	targetRolling, err := recoveryPruneInventoryHashV1(
		recoveryPruneTargetSeedDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		recoveryPruneInt64V1(prune.pruneSequence),
		prune.pruneID[:],
		recoveryPruneInt64V1(int64(len(prune.targets))),
	)
	if err != nil || targetRolling == ([32]byte{}) {
		return [32]byte{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	for index, target := range prune.targets {
		targetDigest, digestErr := recoveryPruneTargetDigestV1(prune, int64(index+1), target)
		if digestErr != nil {
			return [32]byte{}, digestErr
		}
		targetRolling, err = recoveryPruneInventoryHashV1(
			recoveryPruneTargetStepDomainV1,
			recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
			targetRolling[:],
			recoveryPruneInt64V1(int64(index+1)),
			targetDigest[:],
		)
		if err != nil {
			return [32]byte{}, errInvalidRecoveryPruneInventoryDigestV1
		}
	}
	digest, err := recoveryPruneInventoryHashV1(
		recoveryPruneRecordDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		recoveryPruneInt64V1(prune.pruneSequence),
		prune.pruneID[:],
		prune.pruneCertificateID[:],
		recoveryPruneUint32V1(prune.membershipGeneration),
		recoveryPruneInt64V1(prune.barrierArrivalSequence),
		closureDigest[:],
		[]byte(prune.scratchpadSubject),
		recoveryPruneInt64V1(int64(len(prune.targets))),
		targetRolling[:],
	)
	if err != nil || digest == ([32]byte{}) {
		return [32]byte{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return digest, nil
}

func recoveryPruneTargetDigestV1(
	prune verifiedRecoveryPrune,
	ordinal int64,
	target verifiedRecoveryPruneTarget,
) ([32]byte, error) {
	if ordinal < 1 || ordinal > int64(len(prune.targets)) || !recoveryPrunableFactKindV1(target.factKind) ||
		target.hlc.WallMillis < 0 || target.hlc.Logical < 0 {
		return [32]byte{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	referenceDigest, err := recoveryPruneReferenceDigestV1(target.reference)
	if err != nil {
		return [32]byte{}, err
	}
	digest, err := recoveryPruneInventoryHashV1(
		recoveryPruneTargetDomainV1,
		recoveryPruneUint16V1(recoveryPruneInventoryDigestVersionV1),
		recoveryPruneInt64V1(prune.pruneSequence),
		prune.pruneID[:],
		recoveryPruneInt64V1(ordinal),
		referenceDigest[:],
		[]byte(target.factKind),
		recoveryPruneInt64V1(target.hlc.WallMillis),
		recoveryPruneInt32V1(target.hlc.Logical),
	)
	if err != nil || digest == ([32]byte{}) {
		return [32]byte{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return digest, nil
}

func validateRecoveryPruneProjectionV1(prune verifiedRecoveryPrune) error {
	if prune.pruneSequence < 1 || prune.pruneID == (recoveryPruneID{}) ||
		prune.pruneCertificateID == (recoveryPruneCertificateID{}) ||
		prune.membershipGeneration == 0 || prune.barrierArrivalSequence < 1 ||
		prune.scratchpadSubject.Validate() != nil || len(prune.targets) < 1 ||
		len(prune.targets) > protocol.MaxPruneTargets {
		return errInvalidRecoveryPruneInventoryDigestV1
	}
	closure, err := recoveryProtocolPruneReferenceV1(prune.closure)
	if err != nil || closure.ArrivalSequence > prune.barrierArrivalSequence {
		return errInvalidRecoveryPruneInventoryDigestV1
	}
	manifest := protocol.PruneManifest{Targets: make([]protocol.PruneReference, len(prune.targets))}
	for index, target := range prune.targets {
		if !recoveryPrunableFactKindV1(target.factKind) || target.hlc.WallMillis < 0 || target.hlc.Logical < 0 {
			return errInvalidRecoveryPruneInventoryDigestV1
		}
		reference, referenceErr := recoveryProtocolPruneReferenceV1(target.reference)
		if referenceErr != nil || reference.ArrivalSequence > prune.barrierArrivalSequence ||
			reference.ArrivalSequence == closure.ArrivalSequence || reference.FactID == closure.FactID ||
			(reference.EnvironmentID == closure.EnvironmentID && reference.EnvironmentSequence == closure.EnvironmentSequence) {
			return errInvalidRecoveryPruneInventoryDigestV1
		}
		manifest.Targets[index] = reference
	}
	if manifest.Validate() != nil {
		return errInvalidRecoveryPruneInventoryDigestV1
	}
	return nil
}

func recoveryPrunableFactKindV1(kind continuity.FactKind) bool {
	switch kind {
	case continuity.FactScratchpadParticipantIntroduced,
		continuity.FactScratchpadMessageRecorded,
		continuity.FactScratchpadClaimRecorded,
		continuity.FactScratchpadClaimReleased:
		return true
	default:
		return false
	}
}

func recoveryPruneReferenceDigestV1(reference continuitysqlite.VerifiedPruneReference) ([32]byte, error) {
	protocolReference, err := recoveryProtocolPruneReferenceV1(reference)
	if err != nil {
		return [32]byte{}, err
	}
	digest := [32]byte(protocol.PruneReferenceDigest(protocolReference))
	if digest == ([32]byte{}) {
		return [32]byte{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return digest, nil
}

func recoveryProtocolPruneReferenceV1(reference continuitysqlite.VerifiedPruneReference) (protocol.PruneReference, error) {
	protocolReference := protocol.PruneReference{
		FactID:                 reference.FactID,
		EnvironmentID:          reference.EnvironmentID,
		EnvironmentSequence:    reference.EnvironmentSequence,
		ArrivalSequence:        reference.ArrivalSequence,
		EnvelopeDigest:         protocol.Digest(reference.EnvelopeDigest),
		CertificateID:          protocol.Digest(reference.CertificateID),
		PreviousEnvelopeDigest: protocol.Digest(reference.PreviousEnvelopeDigest),
		KeyGeneration:          reference.KeyGeneration,
		Nonce:                  protocol.Nonce(reference.Nonce),
	}
	if protocolReference.Validate() != nil {
		return protocol.PruneReference{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	return protocolReference, nil
}

func recoveryPruneInventoryHashV1(domain string, fields ...[]byte) ([32]byte, error) {
	transcript, err := encodeRecoveryPruneInventoryTranscriptV1(
		domain, maximumRecoveryPruneDigestTranscriptV1, fields...,
	)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(transcript), nil
}

func encodeRecoveryPruneInventoryTranscriptV1(domain string, limit int, fields ...[]byte) ([]byte, error) {
	if domain == "" || uint64(len(domain)) > math.MaxUint32 || limit < 1 ||
		len(fields) > maximumRecoveryPruneDigestFieldsV1 {
		return nil, errInvalidRecoveryPruneInventoryDigestV1
	}
	total := uint64(8 + len(domain))
	for _, field := range fields {
		if uint64(len(field)) > math.MaxUint32 || total > math.MaxUint64-4-uint64(len(field)) {
			return nil, errInvalidRecoveryPruneInventoryDigestV1
		}
		total += 4 + uint64(len(field))
		if total > uint64(limit) {
			return nil, errInvalidRecoveryPruneInventoryDigestV1
		}
	}
	encoded := make([]byte, 0, int(total))
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(domain)))
	encoded = append(encoded, domain...)
	encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(fields)))
	for _, field := range fields {
		encoded = binary.BigEndian.AppendUint32(encoded, uint32(len(field)))
		encoded = append(encoded, field...)
	}
	return encoded, nil
}

func recoveryPruneUint16V1(value uint16) []byte {
	return binary.BigEndian.AppendUint16(nil, value)
}

func recoveryPruneUint32V1(value uint32) []byte {
	return binary.BigEndian.AppendUint32(nil, value)
}

func recoveryPruneInt32V1(value int32) []byte {
	return binary.BigEndian.AppendUint32(nil, uint32(value))
}

func recoveryPruneInt64V1(value int64) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(value))
}
