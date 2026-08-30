package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/levifig/loaf/vnext/continuity"
)

const maximumVerifiedPruneTargets = 1_024

// VerifiedPruneReference is the persistence-local exact identity of one
// already-authenticated relay arrival. Every field is immutable protocol
// material; this package does not verify its cryptographic origin.
type VerifiedPruneReference struct {
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

// VerifiedPrunePlan is one already-verified scratchpad prune authorization.
// PruneCertificateID is the authenticated digest of the complete verified
// certificate: it binds ChannelID, MembershipGeneration,
// BarrierArrivalSequence, Closure, and Targets as one indivisible plan. The
// relay verifier enforces that mapping before constructing this value; callers
// must not synthesize fields or mix fields from different certificates.
// Targets must be strictly sorted by arrival sequence and are defensively
// copied before persistence work begins.
type VerifiedPrunePlan struct {
	ChannelID              SyncChannelID
	MembershipGeneration   uint32
	BarrierArrivalSequence int64
	PruneCertificateID     [32]byte
	Closure                VerifiedPruneReference
	Targets                []VerifiedPruneReference
}

type retainedPruneTombstoneV1 struct {
	reference          VerifiedPruneReference
	pruneCertificateID [32]byte
}

// ApplyVerifiedPrune atomically replaces verified, closed-scratchpad facts
// with exact durable tombstones after proving the local replica is attached
// and fully converged at the plan barrier. Its caller must supply the complete
// plan authenticated by PruneCertificateID; this persistence boundary trusts
// the preceding verifier and never accepts a synthesized or mixed plan.
func (store *Store) ApplyVerifiedPrune(ctx context.Context, projectID continuity.ProjectID, plan VerifiedPrunePlan) error {
	prepared, err := prepareVerifiedPrunePlanV1(projectID, plan)
	if err != nil {
		return err
	}
	if store == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	if err := requirePruneReadinessV1(ctx, tx, projectID, prepared); err != nil {
		return err
	}
	closure, err := requireExactPruneFactV1(ctx, tx, projectID, prepared.Closure)
	if err != nil {
		return err
	}
	if closure.subject.Kind != continuity.RecordScratchpad || closure.kind != continuity.FactScratchpadClosed {
		return syncProblem(SyncErrorConflict, "closure", "does not identify a retained scratchpad closure")
	}
	if _, found, err := readPruneTombstoneCollisionV1(ctx, tx, projectID, prepared.Closure); err != nil {
		return err
	} else if found {
		return syncProblem(SyncErrorStore, "closure", "retained closure is also tombstoned")
	}

	liveTargets := make([]storedFactV1, 0, len(prepared.Targets))
	tombstonedTargets := 0
	for index, target := range prepared.Targets {
		if err := requireExactPruneReceiptV1(ctx, tx, projectID, target); err != nil {
			return refieldPruneErrorV1(err, fmt.Sprintf("targets[%d]", index))
		}
		if present, err := pruneTargetInOutboxV1(ctx, tx, projectID, target); err != nil {
			return err
		} else if present {
			return syncProblem(SyncErrorConflict, fmt.Sprintf("targets[%d]", index), "is retained in the sealed outbox")
		}

		tombstone, tombstoned, err := readPruneTombstoneCollisionV1(ctx, tx, projectID, target)
		if err != nil {
			return err
		}
		fact, found, err := readFactByIDV1(ctx, tx, target.FactID)
		if err != nil {
			return err
		}
		if tombstoned {
			if found {
				return syncProblem(SyncErrorStore, fmt.Sprintf("targets[%d]", index), "is both live and tombstoned")
			}
			if !pruneTombstoneMatchesV1(tombstone, target, prepared.PruneCertificateID) {
				return syncProblem(SyncErrorConflict, fmt.Sprintf("targets[%d]", index), "conflicts with a retained prune tombstone")
			}
			tombstonedTargets++
			continue
		}
		if !found {
			return syncProblem(SyncErrorNotFound, fmt.Sprintf("targets[%d]", index), "does not identify a retained fact or exact tombstone")
		}
		if !pruneFactMatchesReferenceV1(fact, projectID, target) {
			return syncProblem(SyncErrorConflict, fmt.Sprintf("targets[%d]", index), "does not match the retained fact")
		}
		if fact.subject != closure.subject {
			return syncProblem(SyncErrorConflict, fmt.Sprintf("targets[%d]", index), "does not belong to the closure scratchpad")
		}
		if !prunableScratchpadFactKindV1(fact.kind) {
			return syncProblem(SyncErrorConflict, fmt.Sprintf("targets[%d]", index), "is not a prunable scratchpad fact")
		}
		liveTargets = append(liveTargets, fact)
	}
	if tombstonedTargets != 0 && tombstonedTargets != len(prepared.Targets) {
		return syncProblem(SyncErrorConflict, "targets", "mixes live facts with an already-applied prune")
	}

	facts, err := loadProjectFactsV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	remove := make(map[continuity.FactID]struct{}, len(liveTargets))
	for _, fact := range liveTargets {
		remove[fact.factID] = struct{}{}
	}
	retained := make([]storedFactV1, 0, len(facts)-len(liveTargets))
	for _, fact := range facts {
		if _, pruned := remove[fact.factID]; !pruned {
			retained = append(retained, fact)
		}
	}
	if len(retained) == 0 {
		return syncProblem(SyncErrorCandidate, "", "retained corpus has no project identity")
	}
	if _, err := foldProjectSnapshotV1(ctx, projectID, 0, retained); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return syncProblem(SyncErrorCandidate, "", "retained corpus is not valid after pruning")
	}

	if tombstonedTargets == len(prepared.Targets) {
		if err := tx.Commit(); err != nil {
			return syncProblem(SyncErrorStore, "", "prune retry commit outcome is unknown")
		}
		return nil
	}
	for index, target := range prepared.Targets {
		if err := insertPruneTombstoneV1(ctx, tx, projectID, target, prepared.PruneCertificateID); err != nil {
			return refieldPruneErrorV1(err, fmt.Sprintf("targets[%d]", index))
		}
		result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_facts
WHERE project_id = ? AND fact_id = ? AND environment_id = ? AND environment_sequence = ?`,
			string(projectID), string(target.FactID), string(target.EnvironmentID), target.EnvironmentSequence)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "prune commit outcome is unknown; retry the exact verified plan")
	}
	return nil
}

func prepareVerifiedPrunePlanV1(projectID continuity.ProjectID, plan VerifiedPrunePlan) (VerifiedPrunePlan, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return VerifiedPrunePlan{}, err
	}
	if plan.ChannelID == (SyncChannelID{}) {
		return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if plan.MembershipGeneration == 0 {
		return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "membership_generation", "must begin at one")
	}
	if plan.BarrierArrivalSequence < 1 {
		return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "barrier_arrival_sequence", "must begin at one")
	}
	if plan.PruneCertificateID == [32]byte{} {
		return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "prune_certificate_id", "must be nonzero")
	}
	if len(plan.Targets) < 1 || len(plan.Targets) > maximumVerifiedPruneTargets {
		return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "targets", "must contain between 1 and 1024 references")
	}
	if err := validateVerifiedPruneReferenceV1(plan.Closure, "closure"); err != nil {
		return VerifiedPrunePlan{}, err
	}
	if plan.Closure.ArrivalSequence > plan.BarrierArrivalSequence {
		return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "closure.arrival_sequence", "must not exceed the prune barrier")
	}
	prepared := plan
	prepared.Targets = append([]VerifiedPruneReference(nil), plan.Targets...)
	seenFacts := make(map[continuity.FactID]struct{}, len(prepared.Targets))
	seenSources := make(map[string]struct{}, len(prepared.Targets))
	previousArrival := int64(0)
	closureSource := environmentSequenceKeyV1(plan.Closure.EnvironmentID, plan.Closure.EnvironmentSequence)
	for index, target := range prepared.Targets {
		field := fmt.Sprintf("targets[%d]", index)
		if err := validateVerifiedPruneReferenceV1(target, field); err != nil {
			return VerifiedPrunePlan{}, err
		}
		if target.ArrivalSequence > plan.BarrierArrivalSequence {
			return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, field+".arrival_sequence", "must not exceed the prune barrier")
		}
		if target.ArrivalSequence <= previousArrival {
			return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "targets", "must be strictly sorted by arrival sequence")
		}
		previousArrival = target.ArrivalSequence
		if _, duplicate := seenFacts[target.FactID]; duplicate {
			return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "targets", "contains duplicate fact identities")
		}
		seenFacts[target.FactID] = struct{}{}
		source := environmentSequenceKeyV1(target.EnvironmentID, target.EnvironmentSequence)
		if _, duplicate := seenSources[source]; duplicate {
			return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, "targets", "contains duplicate source identities")
		}
		seenSources[source] = struct{}{}
		if target.FactID == plan.Closure.FactID || source == closureSource || target.ArrivalSequence == plan.Closure.ArrivalSequence {
			return VerifiedPrunePlan{}, syncProblem(SyncErrorInvalid, field, "must not identify the closure")
		}
	}
	return prepared, nil
}

func validateVerifiedPruneReferenceV1(reference VerifiedPruneReference, field string) error {
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
		return refieldPruneErrorV1(err, field)
	}
	return nil
}

func requirePruneReadinessV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, plan VerifiedPrunePlan) error {
	binding, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	}
	if err != nil {
		return err
	}
	if binding.ChannelID != plan.ChannelID {
		return syncProblem(SyncErrorConflict, "channel_id", "does not match the pinned authority")
	}
	if binding.MembershipGeneration != plan.MembershipGeneration {
		return syncProblem(SyncErrorConflict, "membership_generation", "does not match the pinned authority")
	}
	environmentIDs := make([]continuity.EnvironmentID, 0, len(plan.Targets)+1)
	environmentIDs = append(environmentIDs, plan.Closure.EnvironmentID)
	for _, target := range plan.Targets {
		environmentIDs = append(environmentIDs, target.EnvironmentID)
	}
	authorityEnvironments, err := readCanonicalSyncEnvironmentCertificatesV2(ctx, tx, projectID, binding, environmentIDs)
	if err != nil {
		return err
	}
	for index, reference := range append([]VerifiedPruneReference{plan.Closure}, plan.Targets...) {
		if authorityEnvironments[reference.EnvironmentID].CertificateID != reference.CertificateID {
			field := "closure"
			if index > 0 {
				field = fmt.Sprintf("targets[%d]", index-1)
			}
			return syncProblem(SyncErrorCertificate, field+".certificate_id", "does not match pinned authority")
		}
	}
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return syncProblem(SyncErrorStore, "project_id", "pinned authority has no sync progress")
	}
	if progress.ChannelID != plan.ChannelID {
		return syncProblem(SyncErrorStore, "channel_id", "sync progress disagrees with pinned authority")
	}
	if progress.ActivationState != SyncActivationAttached {
		return syncProblem(SyncErrorNotAttached, "activation_state", "project is not attached")
	}
	if progress.AppliedCursor < plan.BarrierArrivalSequence {
		return syncProblem(SyncErrorCursor, "barrier_arrival_sequence", "is ahead of the applied cursor")
	}
	if progress.DownloadedCursor != progress.AppliedCursor || progress.AppliedCursor != progress.RelayHead {
		return syncProblem(SyncErrorCursor, "cursor", "local download and apply state is not fully converged")
	}
	var inbox, outbox int
	if err := tx.QueryRowContext(ctx, `SELECT
  (SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_outbox WHERE project_id = ?)`, string(projectID), string(projectID)).Scan(&inbox, &outbox); err != nil {
		return syncTransactionProblem(ctx)
	}
	if inbox != 0 {
		return syncProblem(SyncErrorCursor, "inbox", "must be empty before pruning")
	}
	if outbox != 0 {
		return syncProblem(SyncErrorConflict, "outbox", "must be empty before pruning")
	}
	return nil
}

func requireExactPruneFactV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference VerifiedPruneReference) (storedFactV1, error) {
	if err := requireExactPruneReceiptV1(ctx, tx, projectID, reference); err != nil {
		return storedFactV1{}, err
	}
	fact, found, err := readFactByIDV1(ctx, tx, reference.FactID)
	if err != nil {
		return storedFactV1{}, err
	}
	if !found {
		return storedFactV1{}, syncProblem(SyncErrorNotFound, "fact_id", "does not identify a retained fact")
	}
	if !pruneFactMatchesReferenceV1(fact, projectID, reference) {
		return storedFactV1{}, syncProblem(SyncErrorConflict, "fact_id", "does not match the retained fact")
	}
	return fact, nil
}

func requireExactPruneReceiptV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference VerifiedPruneReference) error {
	var factID continuity.FactID
	var environmentID continuity.EnvironmentID
	var environmentSequence, arrivalSequence int64
	var previousDigest, digest, certificateID, nonce []byte
	var keyGeneration uint32
	err := tx.QueryRowContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, arrival_sequence,
       previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce
FROM continuity_sync_receipts
WHERE project_id = ? AND fact_id = ?`, string(projectID), string(reference.FactID)).Scan(
		&factID, &environmentID, &environmentSequence, &arrivalSequence,
		&previousDigest, &digest, &certificateID, &keyGeneration, &nonce)
	if errors.Is(err, sql.ErrNoRows) {
		return syncProblem(SyncErrorNotFound, "receipt", "has no retained immutable receipt")
	}
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if factID != reference.FactID || environmentID != reference.EnvironmentID || environmentSequence != reference.EnvironmentSequence ||
		arrivalSequence != reference.ArrivalSequence || !bytes.Equal(previousDigest, reference.PreviousEnvelopeDigest[:]) ||
		!bytes.Equal(digest, reference.EnvelopeDigest[:]) || !bytes.Equal(certificateID, reference.CertificateID[:]) ||
		keyGeneration != reference.KeyGeneration || !bytes.Equal(nonce, reference.Nonce[:]) {
		return syncProblem(SyncErrorConflict, "receipt", "does not match all immutable reference fields")
	}
	return nil
}

func readPruneTombstoneCollisionV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference VerifiedPruneReference) (retainedPruneTombstoneV1, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, arrival_sequence,
       previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce,
       prune_certificate_id
FROM continuity_sync_tombstones
WHERE project_id = ? AND (
  fact_id = ? OR arrival_sequence = ? OR (environment_id = ? AND environment_sequence = ?)
)`, string(projectID), string(reference.FactID), reference.ArrivalSequence, string(reference.EnvironmentID), reference.EnvironmentSequence)
	if err != nil {
		return retainedPruneTombstoneV1{}, false, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	var retained retainedPruneTombstoneV1
	found := false
	for rows.Next() {
		if found {
			return retainedPruneTombstoneV1{}, false, syncProblem(SyncErrorConflict, "tombstone", "reference identities collide with different tombstones")
		}
		var previousDigest, digest, certificateID, nonce, pruneCertificateID []byte
		if err := rows.Scan(
			&retained.reference.FactID, &retained.reference.EnvironmentID, &retained.reference.EnvironmentSequence,
			&retained.reference.ArrivalSequence, &previousDigest, &digest, &certificateID,
			&retained.reference.KeyGeneration, &nonce, &pruneCertificateID,
		); err != nil {
			return retainedPruneTombstoneV1{}, false, syncTransactionProblem(ctx)
		}
		if len(previousDigest) != 32 || len(digest) != 32 || len(certificateID) != 32 || len(nonce) != 24 || len(pruneCertificateID) != 32 {
			return retainedPruneTombstoneV1{}, false, syncProblem(SyncErrorStore, "tombstone", "retained metadata is corrupt")
		}
		copy(retained.reference.PreviousEnvelopeDigest[:], previousDigest)
		copy(retained.reference.EnvelopeDigest[:], digest)
		copy(retained.reference.CertificateID[:], certificateID)
		copy(retained.reference.Nonce[:], nonce)
		copy(retained.pruneCertificateID[:], pruneCertificateID)
		found = true
	}
	if err := rows.Err(); err != nil {
		return retainedPruneTombstoneV1{}, false, syncTransactionProblem(ctx)
	}
	return retained, found, nil
}

func pruneTargetInOutboxV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference VerifiedPruneReference) (bool, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) FROM continuity_sync_outbox
WHERE project_id = ? AND (
  fact_id = ? OR (environment_id = ? AND environment_sequence = ?)
)`, string(projectID), string(reference.FactID), string(reference.EnvironmentID), reference.EnvironmentSequence).Scan(&count); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return count != 0, nil
}

func insertPruneTombstoneV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, reference VerifiedPruneReference, pruneCertificateID [32]byte) error {
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_tombstones(
  fact_id, project_id, environment_id, environment_sequence, arrival_sequence,
  previous_envelope_digest, envelope_digest, certificate_id, key_generation, nonce,
  prune_certificate_id
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(reference.FactID), string(projectID), string(reference.EnvironmentID), reference.EnvironmentSequence, reference.ArrivalSequence,
		reference.PreviousEnvelopeDigest[:], reference.EnvelopeDigest[:], reference.CertificateID[:], reference.KeyGeneration, reference.Nonce[:],
		pruneCertificateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}

func pruneFactMatchesReferenceV1(fact storedFactV1, projectID continuity.ProjectID, reference VerifiedPruneReference) bool {
	return fact.projectID == projectID && fact.factID == reference.FactID &&
		fact.environmentID == reference.EnvironmentID && fact.environmentSequence == reference.EnvironmentSequence
}

func pruneTombstoneMatchesV1(tombstone retainedPruneTombstoneV1, reference VerifiedPruneReference, pruneCertificateID [32]byte) bool {
	return tombstone.reference == reference && tombstone.pruneCertificateID == pruneCertificateID
}

func prunableScratchpadFactKindV1(kind continuity.FactKind) bool {
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

func refieldPruneErrorV1(err error, field string) error {
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
