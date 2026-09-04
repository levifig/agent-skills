package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

// legacyVerifiedPruneReferenceV1 is retained only to decode terminal history
// created before scratchpad support was deferred. No current API creates one.
type legacyVerifiedPruneReferenceV1 struct {
	FactID                 continuity.FactID
	EnvironmentID          continuity.EnvironmentID
	EnvironmentSequence    int64
	ArrivalSequence        int64
	EnvelopeDigest         [32]byte
	CertificateID          [32]byte
	PreviousEnvelopeDigest [32]byte
	KeyGeneration          uint32
	Nonce                  [24]byte
}

// VerifiedPruneReference is retained only so older sync recovery records can
// be decoded and verified without reintroducing a current Scratchpad writer or
// prune operation. New continuity code must not construct these references.
//
// Deprecated: Scratchpad pruning is not a current vNext capability.
type VerifiedPruneReference = legacyVerifiedPruneReferenceV1

type retainedPruneTombstoneV1 struct {
	reference                legacyVerifiedPruneReferenceV1
	pruneCertificateID       [32]byte
	prunedArrivalDigest      [32]byte
	prunedArrivalDigestKnown bool
}

func validateVerifiedPruneReferenceV1(reference legacyVerifiedPruneReferenceV1, field string) error {
	if err := reference.FactID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, field+".fact_id", "is invalid")
	}
	if !validOpaqueID(string(reference.EnvironmentID)) {
		return syncProblem(SyncErrorInvalid, field+".environment_id", "is invalid")
	}
	if reference.EnvironmentSequence < 1 {
		return syncProblem(SyncErrorInvalid, field+".environment_sequence", "must begin at one")
	}
	if reference.ArrivalSequence < 1 {
		return syncProblem(SyncErrorInvalid, field+".arrival_sequence", "must begin at one")
	}
	metadata := sealedEnvelopeMetadataV1{
		previousDigest: reference.PreviousEnvelopeDigest,
		digest:         reference.EnvelopeDigest,
		certificateID:  reference.CertificateID,
		keyGeneration:  reference.KeyGeneration,
		nonce:          reference.Nonce,
	}
	if err := validateSealedMetadataV1(reference.EnvironmentSequence, metadata); err != nil {
		var problem *SyncError
		if !errors.As(err, &problem) {
			return err
		}
		clone := *problem
		if clone.Field == "" {
			clone.Field = field
		} else {
			clone.Field = field + "." + clone.Field
		}
		return &clone
	}
	return nil
}

func insertPruneTombstoneV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference legacyVerifiedPruneReferenceV1, pruneCertificateID [32]byte, prunedArrivalDigest *[32]byte) error {
	var retainedPrunedArrivalDigest any
	if prunedArrivalDigest != nil {
		retainedPrunedArrivalDigest = prunedArrivalDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_tombstones(
  fact_id, project_id, environment_id, environment_sequence, arrival_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce,
  prune_certificate_id, pruned_arrival_digest
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(reference.FactID), string(projectID), string(reference.EnvironmentID), reference.EnvironmentSequence, reference.ArrivalSequence,
		reference.PreviousEnvelopeDigest[:], reference.EnvelopeDigest[:], reference.CertificateID[:], reference.KeyGeneration, reference.Nonce[:],
		pruneCertificateID[:], retainedPrunedArrivalDigest)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}

func prunableScratchpadFactKindV1(kind continuity.FactKind) bool {
	switch kind {
	case "scratchpad.participant-introduced",
		"scratchpad.message-recorded",
		"scratchpad.claim-recorded",
		"scratchpad.claim-released":
		return true
	default:
		return false
	}
}
