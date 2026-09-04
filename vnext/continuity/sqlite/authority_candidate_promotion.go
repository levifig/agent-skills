package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	"github.com/levifig/loaf/vnext/internal/continuitywire"
)

// SyncAuthorityCandidateReceipt is the permanent fixed-size receipt for one
// promoted authority candidate. It is reconstructed entirely from the retained
// candidate header and therefore remains available even when mutable canonical
// or protected state later changes.
type SyncAuthorityCandidateReceipt struct {
	ProjectID                continuity.ProjectID
	CandidateID              [32]byte
	Snapshot                 SyncAuthoritySnapshot
	PageCount                int64
	EnvironmentCount         int64
	ThroughEnvironmentID     string
	RollingEnvironmentDigest [32]byte
	AuthorityDigestVersion   uint16
	AuthorityDigest          [32]byte
}

// PromoteSyncAuthorityCandidate atomically replaces the canonical authority
// with the exact ready candidate named by expected. An exact retry returns the
// retained immutable receipt before consulting canonical or protected mutable
// state.
func (store *Store) PromoteSyncAuthorityCandidate(
	ctx context.Context,
	projectID continuity.ProjectID,
	expected SyncAuthorityCandidateCheckpoint,
) (SyncAuthorityCandidateReceipt, error) {
	if err := validateSyncAuthorityCandidateCheckpointV2(expected); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if !expected.Ready {
		return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorInvalid, "checkpoint", "must identify a ready authority candidate")
	}
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if store == nil {
		return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncAuthorityCandidateReceipt{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}

	if receipt, found, err := readPromotedSyncAuthorityCandidateReceiptV2(ctx, tx, projectID, expected.CandidateID); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	} else if found {
		if !syncAuthorityCandidateReceiptMatchesCheckpointV2(receipt, expected) {
			return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorConflict, "checkpoint", "does not match the promoted authority candidate")
		}
		if err := tx.Commit(); err != nil {
			return SyncAuthorityCandidateReceipt{}, syncTransactionProblem(ctx)
		}
		return receipt, nil
	}

	candidate, found, err := readAndValidateActiveSyncAuthorityCandidateV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if !found || !candidate.candidate.Ready || candidate.candidate.Checkpoint() != expected {
		return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorConflict, "checkpoint", "does not match the active ready authority candidate")
	}
	if err := requireKnownExactSyncRelayWatermarkV1(
		ctx, tx, syncAuthorityRecoveryWatermarkFromSnapshotV1(projectID, candidate.candidate.Snapshot),
	); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	base, err := readCanonicalSyncAuthorityBaseV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if err := validateSyncAuthorityCandidateBaseV2(
		candidate.candidate.Snapshot,
		base.digestVersion,
		base.digest,
		base.found,
	); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if err := validateReadySyncAuthorityCandidateAgainstCanonicalV2(ctx, tx, candidate, base); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if !base.found {
		if err := validateSyncAuthorityPromotionBootstrapAbsenceV2(ctx, tx, projectID, candidate.candidate.CandidateID); err != nil {
			return SyncAuthorityCandidateReceipt{}, err
		}
	}
	if err := validateSyncAuthorityPromotionProtectedStateV2(ctx, tx, candidate); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}

	receipt := syncAuthorityCandidateReceiptV2(candidate.candidate)
	if err := applySyncAuthorityPromotionV2(ctx, tx, candidate, base, receipt); err != nil {
		return SyncAuthorityCandidateReceipt{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityCandidateReceipt{}, syncProblem(SyncErrorStore, "", "authority candidate promotion outcome is unknown; retry the exact checkpoint")
	}
	return receipt, nil
}

func syncAuthorityCandidateReceiptV2(candidate SyncAuthorityCandidate) SyncAuthorityCandidateReceipt {
	return SyncAuthorityCandidateReceipt{
		ProjectID:                candidate.ProjectID,
		CandidateID:              candidate.CandidateID,
		Snapshot:                 candidate.Snapshot,
		PageCount:                candidate.PageCount,
		EnvironmentCount:         candidate.EnvironmentCount,
		ThroughEnvironmentID:     candidate.ThroughEnvironmentID,
		RollingEnvironmentDigest: candidate.RollingEnvironmentDigest,
		AuthorityDigestVersion:   candidate.AuthorityDigestVersion,
		AuthorityDigest:          candidate.AuthorityDigest,
	}
}

func syncAuthorityCandidateReceiptMatchesCheckpointV2(receipt SyncAuthorityCandidateReceipt, checkpoint SyncAuthorityCandidateCheckpoint) bool {
	return receipt.CandidateID == checkpoint.CandidateID &&
		receipt.PageCount == checkpoint.PageCount &&
		receipt.EnvironmentCount == checkpoint.EnvironmentCount &&
		receipt.ThroughEnvironmentID == checkpoint.ThroughEnvironmentID &&
		receipt.RollingEnvironmentDigest == checkpoint.RollingEnvironmentDigest &&
		checkpoint.Ready && receipt.AuthorityDigest == checkpoint.AuthorityDigest
}

func readPromotedSyncAuthorityCandidateReceiptV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
) (SyncAuthorityCandidateReceipt, bool, error) {
	receipt := SyncAuthorityCandidateReceipt{ProjectID: projectID}
	var persistedCandidateID, channelID, relayGeneration, adminPublicKey []byte
	var baseVersion sql.NullInt64
	var baseDigest, rollingDigest, authorityDigest []byte
	var membershipGeneration, authorityDigestVersion int64
	var childPages, childEnvironments, childEvents int64
	err := tx.QueryRowContext(ctx, `
SELECT
  candidate_id, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest,
  EXISTS (SELECT 1
   FROM continuity_sync_authority_candidate_pages AS page
   WHERE page.project_id = candidate.project_id AND page.candidate_id = candidate.candidate_id),
  EXISTS (SELECT 1
   FROM continuity_sync_authority_candidate_environments AS environment
   WHERE environment.project_id = candidate.project_id AND environment.candidate_id = candidate.candidate_id),
  EXISTS (SELECT 1
   FROM continuity_sync_authority_candidate_membership_events AS event
   WHERE event.project_id = candidate.project_id AND event.candidate_id = candidate.candidate_id)
FROM continuity_sync_authority_candidates AS candidate
WHERE project_id = ? AND candidate_id = ? AND state = 'promoted'`, string(projectID), candidateID[:]).Scan(
		&persistedCandidateID, &channelID, &relayGeneration, &adminPublicKey,
		&membershipGeneration, &receipt.Snapshot.InventoryArrivalHead,
		&baseVersion, &baseDigest, &receipt.PageCount, &receipt.EnvironmentCount,
		&receipt.ThroughEnvironmentID, &rollingDigest, &authorityDigestVersion, &authorityDigest,
		&childPages, &childEnvironments, &childEvents,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthorityCandidateReceipt{}, false, nil
	}
	if err != nil {
		return SyncAuthorityCandidateReceipt{}, false, syncTransactionProblem(ctx)
	}
	if len(persistedCandidateID) != sha256.Size || isZeroDigestBytesV2(persistedCandidateID) ||
		len(channelID) != len(receipt.Snapshot.ChannelID) || isZeroDigestBytesV2(channelID) ||
		len(relayGeneration) != len(receipt.Snapshot.RelayGeneration) || isZeroDigestBytesV2(relayGeneration) ||
		len(adminPublicKey) != len(receipt.Snapshot.AdminPublicKey) || isZeroDigestBytesV2(adminPublicKey) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 || receipt.Snapshot.InventoryArrivalHead < 0 ||
		receipt.PageCount < 1 || receipt.EnvironmentCount < receipt.PageCount || receipt.PageCount > math.MaxInt64/maximumSyncAuthorityCandidatePageEnvironments ||
		receipt.EnvironmentCount > receipt.PageCount*maximumSyncAuthorityCandidatePageEnvironments || !validOpaqueID(receipt.ThroughEnvironmentID) ||
		len(rollingDigest) != sha256.Size || isZeroDigestBytesV2(rollingDigest) || authorityDigestVersion != 2 ||
		len(authorityDigest) != sha256.Size || isZeroDigestBytesV2(authorityDigest) ||
		childPages != 0 || childEnvironments != 0 || childEvents != 0 {
		return SyncAuthorityCandidateReceipt{}, false, corruptSyncAuthorityCandidateV2("promoted candidate receipt is malformed")
	}
	copy(receipt.CandidateID[:], persistedCandidateID)
	copy(receipt.Snapshot.ChannelID[:], channelID)
	copy(receipt.Snapshot.RelayGeneration[:], relayGeneration)
	copy(receipt.Snapshot.AdminPublicKey[:], adminPublicKey)
	copy(receipt.RollingEnvironmentDigest[:], rollingDigest)
	copy(receipt.AuthorityDigest[:], authorityDigest)
	receipt.Snapshot.MembershipGeneration = uint32(membershipGeneration)
	receipt.AuthorityDigestVersion = uint16(authorityDigestVersion)
	baseAbsent := !baseVersion.Valid && baseDigest == nil
	basePresent := baseVersion.Valid && (baseVersion.Int64 == 1 || baseVersion.Int64 == 2) &&
		len(baseDigest) == sha256.Size && !isZeroDigestBytesV2(baseDigest)
	if !baseAbsent && !basePresent {
		return SyncAuthorityCandidateReceipt{}, false, corruptSyncAuthorityCandidateV2("promoted candidate base authority is malformed")
	}
	if basePresent {
		receipt.Snapshot.BaseAuthorityDigestVersion = uint16(baseVersion.Int64)
		copy(receipt.Snapshot.BaseAuthorityDigest[:], baseDigest)
	}
	derivedCandidateID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, receipt.Snapshot)
	if err != nil || derivedCandidateID != receipt.CandidateID {
		return SyncAuthorityCandidateReceipt{}, false, corruptSyncAuthorityCandidateV2("promoted candidate identity is stale")
	}
	derivedAuthorityDigest, err := finalizeSyncAuthorityDigestV2(headerDigest, receipt.EnvironmentCount, receipt.RollingEnvironmentDigest)
	if err != nil || derivedAuthorityDigest != receipt.AuthorityDigest {
		return SyncAuthorityCandidateReceipt{}, false, corruptSyncAuthorityCandidateV2("promoted candidate authority digest is stale")
	}
	return receipt, true, nil
}

func validateSyncAuthorityPromotionBootstrapAbsenceV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
) error {
	var orphaned int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_inbox WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_receipts WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_outbox WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_tombstones WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_terminal_candidates WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_terminal_candidate_frames WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidates
  WHERE project_id = ? AND candidate_id IS NOT ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidate_pages
  WHERE project_id = ? AND candidate_id IS NOT ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidate_environments
  WHERE project_id = ? AND candidate_id IS NOT ?
  UNION ALL
  SELECT 1 FROM continuity_sync_authority_candidate_membership_events
  WHERE project_id = ? AND candidate_id IS NOT ?
)`,
		string(projectID), string(projectID), string(projectID), string(projectID), string(projectID), string(projectID),
		string(projectID), candidateID[:], string(projectID), candidateID[:],
		string(projectID), candidateID[:], string(projectID), candidateID[:],
	).Scan(&orphaned); err != nil {
		return syncTransactionProblem(ctx)
	}
	if orphaned != 0 {
		return syncProblem(SyncErrorStore, "sync_authority", "sync state exists without a canonical project")
	}
	return nil
}

func activeSyncAuthorityCandidateExistsV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidates
  WHERE project_id = ? AND state IN ('staging', 'ready')
)`, string(projectID)).Scan(&exists); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return exists != 0, nil
}

func anySyncAuthorityCandidateExistsV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidates
  WHERE project_id = ?
)`, string(projectID)).Scan(&exists); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return exists != 0, nil
}

func validateSyncAuthorityPromotionProtectedStateV2(ctx context.Context, tx *sql.Tx, candidate persistedSyncAuthorityCandidateV2) error {
	projectID := string(candidate.candidate.ProjectID)
	candidateID := candidate.candidate.CandidateID[:]
	if err := validateSyncAuthorityPromotionCanonicalFactsV2(ctx, tx, candidate.candidate.ProjectID); err != nil {
		return err
	}
	if err := validateSyncAuthorityPromotionFactIdentitiesV2(ctx, tx, candidate.candidate.ProjectID); err != nil {
		return err
	}
	if err := validateSyncAuthorityPromotionEnvelopeInventoryV2(ctx, tx, candidate.candidate.ProjectID); err != nil {
		return err
	}
	if err := validateSyncAuthorityPromotionArrivalInventoryV2(ctx, tx, candidate.candidate.ProjectID); err != nil {
		return err
	}
	if err := validateSyncAuthorityPromotionFactFrontiersV2(ctx, tx, candidate.candidate.ProjectID); err != nil {
		return err
	}
	if err := validateSyncAuthorityPromotionOutboxBytesV2(ctx, tx, candidate.candidate.ProjectID); err != nil {
		return err
	}

	var missingEnvironment int
	if err := tx.QueryRowContext(ctx, `
WITH referenced(environment_id) AS (
  SELECT environment_id FROM continuity_facts WHERE project_id = ?
  UNION ALL
  SELECT environment_id FROM continuity_sync_environment_heads WHERE project_id = ?
  UNION ALL
  SELECT environment_id FROM continuity_sync_receipts WHERE project_id = ?
  UNION ALL
  SELECT environment_id FROM continuity_sync_outbox WHERE project_id = ?
  UNION ALL
  SELECT environment_id FROM continuity_sync_tombstones WHERE project_id = ?
)
SELECT EXISTS (
  SELECT 1
  FROM referenced
  LEFT JOIN continuity_sync_authority_candidate_environments AS candidate
    ON candidate.project_id = ? AND candidate.candidate_id = ?
   AND candidate.environment_id = referenced.environment_id
  WHERE candidate.environment_id IS NULL
)`, projectID, projectID, projectID, projectID, projectID, projectID, candidateID).Scan(&missingEnvironment); err != nil {
		return syncTransactionProblem(ctx)
	}
	if missingEnvironment != 0 {
		return syncProblem(SyncErrorConflict, "environments", "omits an environment referenced by protected local state")
	}

	var changedCertificate int
	if err := tx.QueryRowContext(ctx, `
WITH sealed(environment_id, certificate_id) AS (
  SELECT environment_id, certificate_id FROM continuity_sync_receipts WHERE project_id = ?
  UNION ALL
  SELECT environment_id, certificate_id FROM continuity_sync_outbox WHERE project_id = ?
  UNION ALL
  SELECT environment_id, certificate_id FROM continuity_sync_tombstones WHERE project_id = ?
  UNION ALL
  SELECT environment_id, certificate_id
  FROM continuity_sync_environment_heads
  WHERE project_id = ? AND sealed_sequence > 0
)
SELECT EXISTS (
  SELECT 1
  FROM sealed
  LEFT JOIN continuity_sync_authority_candidate_environments AS candidate
    ON candidate.project_id = ? AND candidate.candidate_id = ?
   AND candidate.environment_id = sealed.environment_id
  WHERE candidate.environment_id IS NULL OR candidate.certificate_id IS NOT sealed.certificate_id
)`, projectID, projectID, projectID, projectID, projectID, candidateID).Scan(&changedCertificate); err != nil {
		return syncTransactionProblem(ctx)
	}
	if changedCertificate != 0 {
		return syncProblem(SyncErrorConflict, "certificate_id", "changes a certificate referenced by protected local state")
	}

	var orphanedProtectedState int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_receipts AS receipt
  LEFT JOIN continuity_facts AS fact
    ON fact.fact_id = receipt.fact_id
   AND fact.project_id = receipt.project_id
   AND fact.environment_id = receipt.environment_id
   AND fact.environment_sequence = receipt.environment_sequence
  LEFT JOIN continuity_sync_tombstones AS tombstone
    ON tombstone.fact_id = receipt.fact_id
   AND tombstone.project_id = receipt.project_id
   AND tombstone.arrival_sequence = receipt.arrival_sequence
   AND tombstone.environment_id = receipt.environment_id
   AND tombstone.environment_sequence = receipt.environment_sequence
  WHERE receipt.project_id = ? AND fact.fact_id IS NULL AND tombstone.fact_id IS NULL
  UNION ALL
  SELECT 1
  FROM continuity_sync_outbox AS outbox
  LEFT JOIN continuity_facts AS fact
    ON fact.fact_id = outbox.fact_id
   AND fact.project_id = outbox.project_id
   AND fact.environment_id = outbox.environment_id
   AND fact.environment_sequence = outbox.environment_sequence
  WHERE outbox.project_id = ? AND fact.fact_id IS NULL
  UNION ALL
  SELECT 1
  FROM continuity_sync_environment_heads AS head
  LEFT JOIN continuity_facts AS fact
    ON fact.project_id = head.project_id
   AND fact.environment_id = head.environment_id
   AND fact.environment_sequence = head.highest_sequence
  LEFT JOIN continuity_sync_tombstones AS tombstone
    ON tombstone.project_id = head.project_id
   AND tombstone.environment_id = head.environment_id
   AND tombstone.environment_sequence = head.highest_sequence
  WHERE head.project_id = ? AND fact.fact_id IS NULL AND tombstone.fact_id IS NULL
  UNION ALL
  SELECT 1
  FROM continuity_facts AS fact
  LEFT JOIN continuity_sync_environment_heads AS head
    ON head.project_id = fact.project_id AND head.environment_id = fact.environment_id
  WHERE fact.project_id = ? AND (head.environment_id IS NULL OR fact.environment_sequence > head.highest_sequence)
  UNION ALL
  SELECT 1
  FROM continuity_sync_tombstones AS tombstone
  LEFT JOIN continuity_sync_environment_heads AS head
    ON head.project_id = tombstone.project_id AND head.environment_id = tombstone.environment_id
  WHERE tombstone.project_id = ? AND (head.environment_id IS NULL OR tombstone.environment_sequence > head.highest_sequence)
)`, projectID, projectID, projectID, projectID, projectID).Scan(&orphanedProtectedState); err != nil {
		return syncTransactionProblem(ctx)
	}
	if orphanedProtectedState != 0 {
		return syncProblem(SyncErrorStore, "sync_authority", "protected local sync state is orphaned")
	}

	var retirementConflict int
	if err := tx.QueryRowContext(ctx, `
WITH retained(environment_id, environment_sequence, envelope_digest) AS (
  SELECT environment_id, environment_sequence, envelope_digest
  FROM continuity_sync_receipts WHERE project_id = ?
  UNION ALL
  SELECT environment_id, environment_sequence, envelope_digest
  FROM continuity_sync_outbox WHERE project_id = ?
  UNION ALL
  SELECT environment_id, environment_sequence, envelope_digest
  FROM continuity_sync_tombstones WHERE project_id = ?
)
SELECT EXISTS (
  SELECT 1
  FROM retained
  JOIN continuity_sync_authority_candidate_environments AS candidate
    ON candidate.project_id = ? AND candidate.candidate_id = ?
   AND candidate.environment_id = retained.environment_id
  WHERE candidate.retirement_id IS NOT NULL
    AND (
      retained.environment_sequence > candidate.retirement_final_environment_sequence
      OR (
        retained.environment_sequence = candidate.retirement_final_environment_sequence
        AND retained.envelope_digest IS NOT candidate.retirement_final_envelope_digest
      )
    )
  UNION ALL
  SELECT 1
  FROM continuity_facts AS fact
  JOIN continuity_sync_authority_candidate_environments AS candidate
    ON candidate.project_id = ? AND candidate.candidate_id = ?
   AND candidate.environment_id = fact.environment_id
  LEFT JOIN continuity_sync_receipts AS receipt
    ON receipt.project_id = fact.project_id AND receipt.fact_id = fact.fact_id
   AND receipt.environment_id = fact.environment_id
   AND receipt.environment_sequence = fact.environment_sequence
  LEFT JOIN continuity_sync_outbox AS outbox
    ON outbox.project_id = fact.project_id AND outbox.fact_id = fact.fact_id
   AND outbox.environment_id = fact.environment_id
   AND outbox.environment_sequence = fact.environment_sequence
  LEFT JOIN continuity_sync_tombstones AS tombstone
    ON tombstone.project_id = fact.project_id AND tombstone.fact_id = fact.fact_id
   AND tombstone.environment_id = fact.environment_id
   AND tombstone.environment_sequence = fact.environment_sequence
  WHERE fact.project_id = ? AND candidate.retirement_id IS NOT NULL
    AND receipt.fact_id IS NULL AND outbox.fact_id IS NULL AND tombstone.fact_id IS NULL
)`, projectID, projectID, projectID, projectID, candidateID, projectID, candidateID, projectID).Scan(&retirementConflict); err != nil {
		return syncTransactionProblem(ctx)
	}
	if retirementConflict != 0 {
		return syncProblem(SyncErrorConflict, "retirement", "conflicts with retained or unsealed local history")
	}
	return nil
}

const syncAuthorityPromotionCanonicalFactsQueryV2 = `
SELECT
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
FROM continuity_facts
WHERE project_id = ?`

// validateSyncAuthorityPromotionCanonicalFactsV2 reuses the production stored
// fact scanner one bounded row at a time. It does not fold or retain the corpus.
func validateSyncAuthorityPromotionCanonicalFactsV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	rows, err := tx.QueryContext(ctx, syncAuthorityPromotionCanonicalFactsQueryV2, string(projectID))
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	for rows.Next() {
		fact, err := scanStoredFactRowsV1(ctx, rows)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return syncProblem(SyncErrorStore, "", "persisted fact is not canonical")
		}
		if err := continuitywire.Validate(storedFactWireV1(fact)); err != nil {
			return syncProblem(SyncErrorStore, "", "persisted fact is not canonical")
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

type syncAuthorityPromotionFactIdentityRowV2 struct {
	factID              continuity.FactID
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	rowKind             int64
}

// The facts, outbox, and tombstone tables are WITHOUT ROWID and their primary
// keys begin with fact_id. Pin those primary B-trees so SQLite cannot choose a
// project-order index and materialize an unbounded ORDER BY sorter.
const syncAuthorityPromotionFactIdentityQueryV2 = `
SELECT fact_id, environment_id, environment_sequence, 0 AS row_kind
FROM continuity_facts INDEXED BY sqlite_autoindex_continuity_facts_1
WHERE project_id = ?
UNION ALL
SELECT fact_id, environment_id, environment_sequence, 1 AS row_kind
FROM continuity_sync_receipts
WHERE project_id = ?
UNION ALL
SELECT fact_id, environment_id, environment_sequence, 2 AS row_kind
FROM continuity_sync_outbox INDEXED BY sqlite_autoindex_continuity_sync_outbox_1
WHERE project_id = ?
UNION ALL
SELECT fact_id, environment_id, environment_sequence, 3 AS row_kind
FROM continuity_sync_tombstones INDEXED BY sqlite_autoindex_continuity_sync_tombstones_1
WHERE project_id = ?
ORDER BY fact_id, row_kind`

func validateSyncAuthorityPromotionFactIdentitiesV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	rows, err := tx.QueryContext(ctx, syncAuthorityPromotionFactIdentityQueryV2,
		string(projectID), string(projectID), string(projectID), string(projectID),
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	var current syncAuthorityPromotionFactIdentityRowV2
	var haveCurrent bool
	for rows.Next() {
		var row syncAuthorityPromotionFactIdentityRowV2
		if err := rows.Scan(&row.factID, &row.environmentID, &row.environmentSequence, &row.rowKind); err != nil {
			return syncProblem(SyncErrorStore, "", "cannot read protected fact identity")
		}
		if row.factID.Validate() != nil || row.environmentID.Validate() != nil || row.environmentSequence < 1 ||
			row.rowKind < 0 || row.rowKind > 3 {
			return syncProblem(SyncErrorStore, "", "protected fact identity is corrupt")
		}
		if haveCurrent && row.factID == current.factID {
			if row.environmentID != current.environmentID || row.environmentSequence != current.environmentSequence {
				return syncProblem(SyncErrorStore, "", "persisted fact identity changes environment coordinates")
			}
			continue
		}
		current = row
		haveCurrent = true
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

type syncAuthorityPromotionEnvelopeRowV2 struct {
	entry   envelopeInventoryEntryV1
	factID  continuity.FactID
	rowKind int64
}

const syncAuthorityPromotionEnvelopeInventoryQueryV2 = `
SELECT environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce, fact_id, 0 AS row_kind
FROM continuity_sync_receipts
WHERE project_id = ?
UNION ALL
SELECT environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce, fact_id, 0 AS row_kind
FROM continuity_sync_outbox
WHERE project_id = ?
UNION ALL
SELECT environment_id, environment_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce, fact_id, 0 AS row_kind
FROM continuity_sync_tombstones
WHERE project_id = ?
UNION ALL
SELECT environment_id, sealed_sequence, previous_envelope_digest,
       envelope_digest, certificate_id, key_generation, nonce, NULL AS fact_id, 1 AS row_kind
FROM continuity_sync_environment_heads
WHERE project_id = ? AND sealed_sequence > 0
ORDER BY environment_id, environment_sequence, row_kind`

func validateSyncAuthorityPromotionEnvelopeInventoryV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	rows, err := tx.QueryContext(ctx, syncAuthorityPromotionEnvelopeInventoryQueryV2,
		string(projectID), string(projectID), string(projectID), string(projectID),
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	var current syncAuthorityPromotionEnvelopeRowV2
	var haveCurrent, currentHasSource, currentHasHead bool
	var currentEnvironment continuity.EnvironmentID
	var environmentCertificateID [32]byte
	var lastSequence int64
	var lastDigest [32]byte
	var headSeen bool

	finishCurrent := func() error {
		if !haveCurrent {
			return nil
		}
		if !currentHasSource {
			return syncProblem(SyncErrorStore, "", "sealed environment head has no exact retained metadata")
		}
		if currentHasHead {
			if headSeen {
				return syncProblem(SyncErrorStore, "", "sealed environment head is duplicated")
			}
			headSeen = true
		}
		lastSequence = current.entry.environmentSequence
		lastDigest = current.entry.metadata.digest
		return nil
	}
	finishEnvironment := func() error {
		if err := finishCurrent(); err != nil {
			return err
		}
		if haveCurrent && !headSeen {
			return syncProblem(SyncErrorStore, "", "retained sealed metadata has no exact environment head")
		}
		return nil
	}

	for rows.Next() {
		row, err := scanSyncAuthorityPromotionEnvelopeRowV2(rows)
		if err != nil {
			return err
		}
		if !haveCurrent || row.entry.environmentID != currentEnvironment || row.entry.environmentSequence != current.entry.environmentSequence {
			if haveCurrent && row.entry.environmentID != currentEnvironment {
				if err := finishEnvironment(); err != nil {
					return err
				}
				currentEnvironment = ""
				environmentCertificateID = [32]byte{}
				lastSequence = 0
				lastDigest = [32]byte{}
				headSeen = false
				haveCurrent = false
			} else if haveCurrent {
				if err := finishCurrent(); err != nil {
					return err
				}
				if headSeen {
					return syncProblem(SyncErrorStore, "", "retained sealed metadata extends beyond its environment head")
				}
			}

			if currentEnvironment == "" {
				currentEnvironment = row.entry.environmentID
				environmentCertificateID = row.entry.metadata.certificateID
			}
			if row.entry.environmentSequence != lastSequence+1 ||
				(lastSequence > 0 && row.entry.metadata.previousDigest != lastDigest) {
				return syncProblem(SyncErrorStore, "", "persisted envelope chain is not contiguous")
			}
			if row.entry.metadata.certificateID != environmentCertificateID {
				return syncProblem(SyncErrorStore, "", "persisted environment certificate changed")
			}
			current = row
			haveCurrent = true
			currentHasSource = row.rowKind == 0
			currentHasHead = row.rowKind == 1
			continue
		}

		if !sealedMetadataEqualV1(current.entry.metadata, row.entry.metadata) {
			return syncProblem(SyncErrorStore, "", "persisted envelope sequence metadata conflicts")
		}
		if row.rowKind == 0 {
			if currentHasSource && row.factID != current.factID {
				return syncProblem(SyncErrorStore, "", "persisted envelope sequence fact identity conflicts")
			}
			current.factID = row.factID
			currentHasSource = true
		} else if currentHasHead {
			return syncProblem(SyncErrorStore, "", "sealed environment head is duplicated")
		} else {
			currentHasHead = true
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := finishEnvironment(); err != nil {
		return err
	}

	return validateSyncAuthorityPromotionGenerationNoncesV2(ctx, tx, projectID)
}

func scanSyncAuthorityPromotionEnvelopeRowV2(scanner interface {
	Scan(dest ...any) error
}) (syncAuthorityPromotionEnvelopeRowV2, error) {
	var row syncAuthorityPromotionEnvelopeRowV2
	var previousDigest, digest, certificateID, nonce []byte
	var keyGeneration int64
	var factID sql.NullString
	if err := scanner.Scan(
		&row.entry.environmentID,
		&row.entry.environmentSequence,
		&previousDigest,
		&digest,
		&certificateID,
		&keyGeneration,
		&nonce,
		&factID,
		&row.rowKind,
	); err != nil {
		return syncAuthorityPromotionEnvelopeRowV2{}, syncProblem(SyncErrorStore, "", "cannot read sealed envelope metadata")
	}
	if row.entry.environmentID.Validate() != nil || row.entry.environmentSequence < 1 ||
		len(previousDigest) != len(row.entry.metadata.previousDigest) ||
		len(digest) != len(row.entry.metadata.digest) ||
		len(certificateID) != len(row.entry.metadata.certificateID) ||
		len(nonce) != len(row.entry.metadata.nonce) ||
		keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
		(row.rowKind != 0 && row.rowKind != 1) ||
		(row.rowKind == 0 && (!factID.Valid || continuity.FactID(factID.String).Validate() != nil)) ||
		(row.rowKind == 1 && factID.Valid) {
		return syncAuthorityPromotionEnvelopeRowV2{}, syncProblem(SyncErrorStore, "", "sealed envelope metadata is corrupt")
	}
	copy(row.entry.metadata.previousDigest[:], previousDigest)
	copy(row.entry.metadata.digest[:], digest)
	copy(row.entry.metadata.certificateID[:], certificateID)
	row.entry.metadata.keyGeneration = uint32(keyGeneration)
	copy(row.entry.metadata.nonce[:], nonce)
	if factID.Valid {
		row.factID = continuity.FactID(factID.String)
	}
	if err := validateSealedMetadataV1(row.entry.environmentSequence, row.entry.metadata); err != nil {
		return syncAuthorityPromotionEnvelopeRowV2{}, syncProblem(SyncErrorStore, "", "sealed envelope metadata is corrupt")
	}
	return row, nil
}

const syncAuthorityPromotionGenerationNoncesQueryV2 = `
SELECT key_generation, nonce, environment_id, environment_sequence
FROM continuity_sync_receipts
WHERE project_id = ?
UNION ALL
SELECT key_generation, nonce, environment_id, environment_sequence
FROM continuity_sync_outbox
WHERE project_id = ?
UNION ALL
SELECT key_generation, nonce, environment_id, environment_sequence
FROM continuity_sync_tombstones
WHERE project_id = ?
ORDER BY key_generation, nonce, environment_id, environment_sequence`

func validateSyncAuthorityPromotionGenerationNoncesV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	rows, err := tx.QueryContext(ctx, syncAuthorityPromotionGenerationNoncesQueryV2,
		string(projectID), string(projectID), string(projectID),
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	var previousGeneration int64
	var previousNonce [24]byte
	var ownerEnvironment continuity.EnvironmentID
	var ownerSequence int64
	var havePrevious bool
	for rows.Next() {
		var keyGeneration, environmentSequence int64
		var nonceBytes []byte
		var environmentID continuity.EnvironmentID
		if err := rows.Scan(&keyGeneration, &nonceBytes, &environmentID, &environmentSequence); err != nil {
			return syncProblem(SyncErrorStore, "", "cannot read sealed envelope nonce ownership")
		}
		if keyGeneration < 1 || keyGeneration > math.MaxUint32 || len(nonceBytes) != len(previousNonce) ||
			environmentID.Validate() != nil || environmentSequence < 1 {
			return syncProblem(SyncErrorStore, "", "sealed envelope nonce ownership is corrupt")
		}
		var nonce [24]byte
		copy(nonce[:], nonceBytes)
		if havePrevious && keyGeneration == previousGeneration && nonce == previousNonce {
			if environmentID != ownerEnvironment || environmentSequence != ownerSequence {
				return syncProblem(SyncErrorStore, "", "persisted generation nonce is reused")
			}
			continue
		}
		previousGeneration = keyGeneration
		previousNonce = nonce
		ownerEnvironment = environmentID
		ownerSequence = environmentSequence
		havePrevious = true
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

type syncAuthorityPromotionArrivalRowV2 struct {
	arrivalSequence     int64
	factID              continuity.FactID
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	metadata            sealedEnvelopeMetadataV1
	rowKind             int64
}

// Receipt and tombstone schemas each enforce arrival uniqueness independently.
// This ordered stream enforces the contiguous receipt frontier and the
// cross-table exact-duplicate rule left open by those per-table indexes.
const syncAuthorityPromotionArrivalInventoryQueryV2 = `
SELECT arrival_sequence, fact_id, environment_id, environment_sequence,
       previous_envelope_digest, envelope_digest, certificate_id,
       key_generation, nonce, 0 AS row_kind
FROM continuity_sync_receipts
WHERE project_id = ?
UNION ALL
SELECT arrival_sequence, fact_id, environment_id, environment_sequence,
       previous_envelope_digest, envelope_digest, certificate_id,
       key_generation, nonce, 1 AS row_kind
FROM continuity_sync_tombstones
WHERE project_id = ?
ORDER BY arrival_sequence, row_kind`

func validateSyncAuthorityPromotionArrivalInventoryV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	var appliedCursor int64
	if found {
		appliedCursor = progress.AppliedCursor
	}

	rows, err := tx.QueryContext(ctx, syncAuthorityPromotionArrivalInventoryQueryV2, string(projectID), string(projectID))
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	var current syncAuthorityPromotionArrivalRowV2
	var haveCurrent bool
	var haveReceipt bool
	expectedReceiptArrival := int64(1)
	finishArrival := func() error {
		if !haveCurrent {
			return nil
		}
		if !haveReceipt {
			return syncProblem(SyncErrorStore, "", "persisted tombstone has no exact receipt")
		}
		if current.arrivalSequence != expectedReceiptArrival {
			return syncProblem(SyncErrorStore, "", "persisted receipt frontier is not contiguous")
		}
		expectedReceiptArrival++
		return nil
	}
	for rows.Next() {
		row, err := scanSyncAuthorityPromotionArrivalRowV2(rows)
		if err != nil {
			return err
		}
		if haveCurrent && row.arrivalSequence == current.arrivalSequence {
			if row.rowKind == current.rowKind || row.factID != current.factID ||
				row.environmentID != current.environmentID || row.environmentSequence != current.environmentSequence ||
				!sealedMetadataEqualV1(row.metadata, current.metadata) {
				return syncProblem(SyncErrorStore, "", "persisted arrival identity conflicts")
			}
			haveReceipt = haveReceipt || row.rowKind == 0
			continue
		}
		if err := finishArrival(); err != nil {
			return err
		}
		current = row
		haveCurrent = true
		haveReceipt = row.rowKind == 0
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := finishArrival(); err != nil {
		return err
	}
	if expectedReceiptArrival-1 != appliedCursor {
		return syncProblem(SyncErrorStore, "", "persisted receipt frontier does not match applied cursor")
	}
	return nil
}

func scanSyncAuthorityPromotionArrivalRowV2(scanner interface {
	Scan(dest ...any) error
}) (syncAuthorityPromotionArrivalRowV2, error) {
	var row syncAuthorityPromotionArrivalRowV2
	var previousDigest, digest, certificateID, nonce []byte
	var keyGeneration int64
	if err := scanner.Scan(
		&row.arrivalSequence,
		&row.factID,
		&row.environmentID,
		&row.environmentSequence,
		&previousDigest,
		&digest,
		&certificateID,
		&keyGeneration,
		&nonce,
		&row.rowKind,
	); err != nil {
		return syncAuthorityPromotionArrivalRowV2{}, syncProblem(SyncErrorStore, "", "cannot read protected arrival identity")
	}
	if row.arrivalSequence < 1 || row.factID.Validate() != nil || row.environmentID.Validate() != nil ||
		row.environmentSequence < 1 || len(previousDigest) != len(row.metadata.previousDigest) ||
		len(digest) != len(row.metadata.digest) || len(certificateID) != len(row.metadata.certificateID) ||
		keyGeneration < 1 || keyGeneration > math.MaxUint32 || len(nonce) != len(row.metadata.nonce) ||
		(row.rowKind != 0 && row.rowKind != 1) {
		return syncAuthorityPromotionArrivalRowV2{}, syncProblem(SyncErrorStore, "", "protected arrival identity is corrupt")
	}
	copy(row.metadata.previousDigest[:], previousDigest)
	copy(row.metadata.digest[:], digest)
	copy(row.metadata.certificateID[:], certificateID)
	row.metadata.keyGeneration = uint32(keyGeneration)
	copy(row.metadata.nonce[:], nonce)
	if err := validateSealedMetadataV1(row.environmentSequence, row.metadata); err != nil {
		return syncAuthorityPromotionArrivalRowV2{}, syncProblem(SyncErrorStore, "", "protected arrival identity is corrupt")
	}
	return row, nil
}

type syncAuthorityPromotionFactFrontierRowV2 struct {
	environmentID       continuity.EnvironmentID
	environmentSequence int64
	factID              continuity.FactID
	rowKind             int64
	highestSequence     int64
	sealedSequence      int64
	clock               continuity.HybridTime
}

const syncAuthorityPromotionFactFrontierQueryV2 = `
SELECT environment_id, 0 AS environment_sequence, NULL AS fact_id, 0 AS row_kind,
       highest_sequence, sealed_sequence, hlc_wall_millis, hlc_logical
FROM continuity_sync_environment_heads
WHERE project_id = ?
UNION ALL
SELECT environment_id, environment_sequence, fact_id, 1 AS row_kind,
       NULL, NULL, hlc_wall_millis, hlc_logical
FROM continuity_facts
WHERE project_id = ?
UNION ALL
SELECT environment_id, environment_sequence, fact_id, 2 AS row_kind,
       NULL, NULL, NULL, NULL
FROM continuity_sync_receipts
WHERE project_id = ?
UNION ALL
SELECT environment_id, environment_sequence, fact_id, 3 AS row_kind,
       NULL, NULL, NULL, NULL
FROM continuity_sync_outbox
WHERE project_id = ?
UNION ALL
SELECT environment_id, environment_sequence, fact_id, 4 AS row_kind,
       NULL, NULL, NULL, NULL
FROM continuity_sync_tombstones
WHERE project_id = ?
ORDER BY environment_id, environment_sequence, row_kind`

func validateSyncAuthorityPromotionFactFrontiersV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	rows, err := tx.QueryContext(ctx, syncAuthorityPromotionFactFrontierQueryV2,
		string(projectID), string(projectID), string(projectID), string(projectID), string(projectID),
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	var environmentID continuity.EnvironmentID
	var highestSequence, sealedSequence int64
	var headClock continuity.HybridTime
	var haveEnvironment bool
	var currentSequence int64
	var haveSequence, hasFact, hasReceipt, hasOutbox, hasTombstone bool
	var factID, sourceFactID continuity.FactID
	var factClock continuity.HybridTime
	var lastUnsealedSequence int64
	var lastLiveFactSequence int64
	var lastLiveFactClock continuity.HybridTime
	var haveLastLiveFact bool

	finishSequence := func() error {
		if !haveSequence {
			return nil
		}
		hasSource := hasReceipt || hasOutbox || hasTombstone
		if currentSequence > highestSequence {
			return syncProblem(SyncErrorStore, "", "protected fact exceeds its environment head")
		}
		if currentSequence <= sealedSequence {
			if !hasSource {
				return syncProblem(SyncErrorStore, "", "sealed fact has no retained envelope source")
			}
			if hasFact && (hasTombstone || factID != sourceFactID) {
				return syncProblem(SyncErrorStore, "", "live fact conflicts with retained sealed identity")
			}
			if hasFact {
				if haveLastLiveFact && !hybridTimeLessV1(lastLiveFactClock, factClock) {
					return syncProblem(SyncErrorStore, "", "retained live fact clocks do not increase")
				}
				lastLiveFactSequence = currentSequence
				lastLiveFactClock = factClock
				haveLastLiveFact = true
			}
			return nil
		}
		if currentSequence != lastUnsealedSequence+1 || !hasFact || hasSource {
			return syncProblem(SyncErrorStore, "", "unsealed environment suffix is not a contiguous live-fact chain")
		}
		if haveLastLiveFact && !hybridTimeLessV1(lastLiveFactClock, factClock) {
			return syncProblem(SyncErrorStore, "", "retained live fact clocks do not increase")
		}
		lastUnsealedSequence = currentSequence
		lastLiveFactSequence = currentSequence
		lastLiveFactClock = factClock
		haveLastLiveFact = true
		return nil
	}
	finishEnvironment := func() error {
		if !haveEnvironment {
			return nil
		}
		if err := finishSequence(); err != nil {
			return err
		}
		if lastUnsealedSequence != highestSequence {
			return syncProblem(SyncErrorStore, "", "environment head exceeds its represented fact frontier")
		}
		if highestSequence > sealedSequence && (!haveLastLiveFact || lastLiveFactSequence != highestSequence || lastLiveFactClock != headClock) {
			return syncProblem(SyncErrorStore, "", "environment head clock does not match its final live fact")
		}
		if highestSequence == sealedSequence && haveLastLiveFact {
			switch {
			case lastLiveFactSequence == highestSequence && lastLiveFactClock != headClock:
				return syncProblem(SyncErrorStore, "", "sealed environment head clock does not match its retained live fact")
			case lastLiveFactSequence < highestSequence && !hybridTimeLessV1(lastLiveFactClock, headClock):
				return syncProblem(SyncErrorStore, "", "sealed environment head clock does not follow its retained live facts")
			}
		}
		return nil
	}
	startSequence := func(row syncAuthorityPromotionFactFrontierRowV2) {
		currentSequence = row.environmentSequence
		haveSequence = true
		hasFact = false
		hasReceipt = false
		hasOutbox = false
		hasTombstone = false
		factID = ""
		sourceFactID = ""
		factClock = continuity.HybridTime{}
	}
	addRow := func(row syncAuthorityPromotionFactFrontierRowV2) error {
		switch row.rowKind {
		case 1:
			if hasFact {
				return syncProblem(SyncErrorStore, "", "environment sequence has duplicate live facts")
			}
			hasFact = true
			factID = row.factID
			factClock = row.clock
		case 2, 3, 4:
			if (hasReceipt || hasOutbox || hasTombstone) && sourceFactID != row.factID {
				return syncProblem(SyncErrorStore, "", "environment sequence changes sealed fact identity")
			}
			sourceFactID = row.factID
			switch row.rowKind {
			case 2:
				hasReceipt = true
			case 3:
				hasOutbox = true
			case 4:
				hasTombstone = true
			}
		default:
			return syncProblem(SyncErrorStore, "", "protected fact frontier row is corrupt")
		}
		return nil
	}

	for rows.Next() {
		row, err := scanSyncAuthorityPromotionFactFrontierRowV2(rows)
		if err != nil {
			return err
		}
		if !haveEnvironment || row.environmentID != environmentID {
			if err := finishEnvironment(); err != nil {
				return err
			}
			if row.rowKind != 0 {
				return syncProblem(SyncErrorStore, "", "protected environment has no exact head")
			}
			environmentID = row.environmentID
			highestSequence = row.highestSequence
			sealedSequence = row.sealedSequence
			headClock = row.clock
			haveEnvironment = true
			haveSequence = false
			lastUnsealedSequence = row.sealedSequence
			lastLiveFactSequence = 0
			lastLiveFactClock = continuity.HybridTime{}
			haveLastLiveFact = false
			continue
		}
		if row.rowKind == 0 {
			return syncProblem(SyncErrorStore, "", "protected environment head is duplicated")
		}
		if !haveSequence || row.environmentSequence != currentSequence {
			if err := finishSequence(); err != nil {
				return err
			}
			startSequence(row)
		}
		if err := addRow(row); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return finishEnvironment()
}

func scanSyncAuthorityPromotionFactFrontierRowV2(scanner interface {
	Scan(dest ...any) error
}) (syncAuthorityPromotionFactFrontierRowV2, error) {
	var row syncAuthorityPromotionFactFrontierRowV2
	var factID sql.NullString
	var highestSequence, sealedSequence, wallMillis, logical sql.NullInt64
	if err := scanner.Scan(
		&row.environmentID,
		&row.environmentSequence,
		&factID,
		&row.rowKind,
		&highestSequence,
		&sealedSequence,
		&wallMillis,
		&logical,
	); err != nil {
		return syncAuthorityPromotionFactFrontierRowV2{}, syncProblem(SyncErrorStore, "", "cannot read protected fact frontier")
	}
	if row.environmentID.Validate() != nil || row.rowKind < 0 || row.rowKind > 4 {
		return syncAuthorityPromotionFactFrontierRowV2{}, syncProblem(SyncErrorStore, "", "protected fact frontier is corrupt")
	}
	if row.rowKind == 0 {
		if row.environmentSequence != 0 || factID.Valid || !highestSequence.Valid || highestSequence.Int64 < 1 ||
			!sealedSequence.Valid || sealedSequence.Int64 < 0 || sealedSequence.Int64 > highestSequence.Int64 ||
			!wallMillis.Valid || wallMillis.Int64 < 0 || !logical.Valid || logical.Int64 < 0 || logical.Int64 > math.MaxInt32 {
			return syncAuthorityPromotionFactFrontierRowV2{}, syncProblem(SyncErrorStore, "", "protected environment head is corrupt")
		}
		row.highestSequence = highestSequence.Int64
		row.sealedSequence = sealedSequence.Int64
		row.clock = continuity.HybridTime{WallMillis: wallMillis.Int64, Logical: int32(logical.Int64)}
		return row, nil
	}
	if row.environmentSequence < 1 || !factID.Valid || continuity.FactID(factID.String).Validate() != nil ||
		highestSequence.Valid || sealedSequence.Valid {
		return syncAuthorityPromotionFactFrontierRowV2{}, syncProblem(SyncErrorStore, "", "protected fact frontier is corrupt")
	}
	row.factID = continuity.FactID(factID.String)
	if row.rowKind == 1 {
		if !wallMillis.Valid || wallMillis.Int64 < 0 || !logical.Valid || logical.Int64 < 0 || logical.Int64 > math.MaxInt32 {
			return syncAuthorityPromotionFactFrontierRowV2{}, syncProblem(SyncErrorStore, "", "protected live fact frontier is corrupt")
		}
		row.clock = continuity.HybridTime{WallMillis: wallMillis.Int64, Logical: int32(logical.Int64)}
		return row, nil
	}
	if wallMillis.Valid || logical.Valid {
		return syncAuthorityPromotionFactFrontierRowV2{}, syncProblem(SyncErrorStore, "", "protected sealed source frontier is corrupt")
	}
	return row, nil
}

func validateSyncAuthorityPromotionOutboxBytesV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) error {
	rows, err := tx.QueryContext(ctx, `
SELECT envelope_digest, sealed_envelope
FROM continuity_sync_outbox
WHERE project_id = ?
ORDER BY fact_id`, string(projectID))
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()

	for rows.Next() {
		var digestBytes, sealedEnvelope []byte
		if err := rows.Scan(&digestBytes, &sealedEnvelope); err != nil {
			return syncTransactionProblem(ctx)
		}
		if len(digestBytes) != sha256.Size || isZeroDigestBytesV2(digestBytes) ||
			len(sealedEnvelope) < 1 || len(sealedEnvelope) > maximumSealedEnvelopeBytes {
			return syncProblem(SyncErrorStore, "", "sealed outbox row is corrupt")
		}
		var persistedDigest [sha256.Size]byte
		copy(persistedDigest[:], digestBytes)
		if sha256.Sum256(sealedEnvelope) != persistedDigest {
			return syncProblem(SyncErrorStore, "", "sealed outbox row is corrupt")
		}
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func applySyncAuthorityPromotionV2(
	ctx context.Context,
	tx *sql.Tx,
	candidate persistedSyncAuthorityCandidateV2,
	base canonicalSyncAuthorityBaseV2,
	receipt SyncAuthorityCandidateReceipt,
) error {
	projectID := string(receipt.ProjectID)
	if base.found {
		result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET membership_generation = ?
WHERE project_id = ? AND channel_id = ? AND relay_generation = ?
  AND admin_public_key = ? AND membership_generation = ?`,
			receipt.Snapshot.MembershipGeneration, projectID, base.authority.ChannelID[:],
			base.authority.RelayGeneration[:], base.authority.AdminPublicKey[:], base.authority.MembershipGeneration,
		)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority header changed during promotion")
		}
	} else {
		result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state,
  downloaded_cursor, applied_cursor, relay_head
) SELECT ?, ?, ?, ?, ?, 'staging', 0, 0, 0
WHERE NOT EXISTS (SELECT 1 FROM continuity_sync_projects WHERE project_id = ?)`,
			projectID, receipt.Snapshot.ChannelID[:], receipt.Snapshot.RelayGeneration[:],
			receipt.Snapshot.AdminPublicKey[:], receipt.Snapshot.MembershipGeneration, projectID,
		)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority appeared during bootstrap promotion")
		}
	}

	var appendedRetirements, newEnvironments int64
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*)
   FROM continuity_sync_environment_certificates AS canonical
   JOIN continuity_sync_authority_candidate_environments AS candidate
     ON candidate.project_id = canonical.project_id
    AND candidate.environment_id = canonical.environment_id
   WHERE canonical.project_id = ? AND candidate.candidate_id = ?
     AND canonical.retirement_id IS NULL AND candidate.retirement_id IS NOT NULL),
  (SELECT COUNT(*)
   FROM continuity_sync_authority_candidate_environments AS candidate
   LEFT JOIN continuity_sync_environment_certificates AS canonical
     ON canonical.project_id = candidate.project_id
    AND canonical.environment_id = candidate.environment_id
   WHERE candidate.project_id = ? AND candidate.candidate_id = ?
     AND canonical.environment_id IS NULL)`,
		projectID, receipt.CandidateID[:], projectID, receipt.CandidateID[:],
	).Scan(&appendedRetirements, &newEnvironments); err != nil {
		return syncTransactionProblem(ctx)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_environment_certificates AS canonical
SET retirement_relay_generation = (
      SELECT candidate.retirement_relay_generation
      FROM continuity_sync_authority_candidate_environments AS candidate
      WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
        AND candidate.environment_id = canonical.environment_id
    ),
    retirement_membership_generation = (
      SELECT candidate.retirement_membership_generation
      FROM continuity_sync_authority_candidate_environments AS candidate
      WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
        AND candidate.environment_id = canonical.environment_id
    ),
    retirement_final_environment_sequence = (
      SELECT candidate.retirement_final_environment_sequence
      FROM continuity_sync_authority_candidate_environments AS candidate
      WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
        AND candidate.environment_id = canonical.environment_id
    ),
    retirement_final_envelope_digest = (
      SELECT candidate.retirement_final_envelope_digest
      FROM continuity_sync_authority_candidate_environments AS candidate
      WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
        AND candidate.environment_id = canonical.environment_id
    ),
    retirement_id = (
      SELECT candidate.retirement_id
      FROM continuity_sync_authority_candidate_environments AS candidate
      WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
        AND candidate.environment_id = canonical.environment_id
    ),
    retirement_bytes = (
      SELECT candidate.retirement_bytes
      FROM continuity_sync_authority_candidate_environments AS candidate
      WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
        AND candidate.environment_id = canonical.environment_id
    )
WHERE canonical.project_id = ? AND canonical.retirement_id IS NULL
  AND EXISTS (
    SELECT 1
    FROM continuity_sync_authority_candidate_environments AS candidate
    WHERE candidate.project_id = canonical.project_id AND candidate.candidate_id = ?
      AND candidate.environment_id = canonical.environment_id
      AND candidate.certificate_id IS canonical.certificate_id
      AND candidate.certificate_bytes IS canonical.certificate_bytes
      AND candidate.mode = canonical.mode
      AND candidate.expires_at_millis = canonical.expires_at_millis
      AND candidate.join_membership_generation = canonical.join_membership_generation
      AND candidate.retirement_id IS NOT NULL
  )`,
		receipt.CandidateID[:], receipt.CandidateID[:], receipt.CandidateID[:],
		receipt.CandidateID[:], receipt.CandidateID[:], receipt.CandidateID[:],
		projectID, receipt.CandidateID[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != appendedRetirements {
		return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority retirements changed during promotion")
	}
	result, err = tx.ExecContext(ctx, `
INSERT INTO continuity_sync_environment_certificates(
  project_id, environment_id, certificate_id, certificate_bytes, mode,
  expires_at_millis, join_membership_generation,
  retirement_relay_generation, retirement_membership_generation,
  retirement_final_environment_sequence, retirement_final_envelope_digest,
  retirement_id, retirement_bytes
)
SELECT
  project_id, environment_id, certificate_id, certificate_bytes, mode,
  expires_at_millis, join_membership_generation,
  retirement_relay_generation, retirement_membership_generation,
  retirement_final_environment_sequence, retirement_final_envelope_digest,
  retirement_id, retirement_bytes
FROM continuity_sync_authority_candidate_environments
WHERE project_id = ? AND candidate_id = ?
  AND NOT EXISTS (
    SELECT 1
    FROM continuity_sync_environment_certificates AS canonical
    WHERE canonical.project_id = continuity_sync_authority_candidate_environments.project_id
      AND canonical.environment_id = continuity_sync_authority_candidate_environments.environment_id
  )
ORDER BY environment_ordinal`, projectID, receipt.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != newEnvironments {
		return syncProblem(SyncErrorConflict, "candidate", "authority candidate inventory changed during promotion")
	}

	if base.found {
		result, err = tx.ExecContext(ctx, `
UPDATE continuity_sync_authorities
SET digest_version = ?, authority_digest = ?, inventory_arrival_head = ?
WHERE project_id = ? AND digest_version = ? AND authority_digest = ?
  AND inventory_arrival_head = ?`,
			receipt.AuthorityDigestVersion, receipt.AuthorityDigest[:], receipt.Snapshot.InventoryArrivalHead,
			projectID, base.digestVersion, base.digest[:], base.authority.InventoryArrivalHead,
		)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority metadata changed during promotion")
		}
	} else {
		result, err = tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) SELECT ?, ?, ?, ?
WHERE NOT EXISTS (SELECT 1 FROM continuity_sync_authorities WHERE project_id = ?)`,
			projectID, receipt.AuthorityDigestVersion, receipt.AuthorityDigest[:], receipt.Snapshot.InventoryArrivalHead, projectID,
		)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority metadata appeared during bootstrap promotion")
		}
	}
	promotedBase := canonicalSyncAuthorityBaseV2{
		authority: SyncAuthority{
			ChannelID:            receipt.Snapshot.ChannelID,
			RelayGeneration:      receipt.Snapshot.RelayGeneration,
			AdminPublicKey:       receipt.Snapshot.AdminPublicKey,
			MembershipGeneration: receipt.Snapshot.MembershipGeneration,
			InventoryArrivalHead: receipt.Snapshot.InventoryArrivalHead,
		},
		digestVersion: receipt.AuthorityDigestVersion,
		digest:        receipt.AuthorityDigest,
		found:         true,
	}
	resultingBase, err := readCanonicalSyncAuthorityBaseV2(ctx, tx, receipt.ProjectID)
	if err != nil {
		return err
	}
	if !resultingBase.found || resultingBase.authority.ChannelID != promotedBase.authority.ChannelID ||
		resultingBase.authority.RelayGeneration != promotedBase.authority.RelayGeneration ||
		resultingBase.authority.AdminPublicKey != promotedBase.authority.AdminPublicKey ||
		resultingBase.authority.MembershipGeneration != promotedBase.authority.MembershipGeneration ||
		resultingBase.authority.InventoryArrivalHead != promotedBase.authority.InventoryArrivalHead ||
		resultingBase.digestVersion != promotedBase.digestVersion || resultingBase.digest != promotedBase.digest {
		return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority binding changed during promotion")
	}
	if err := validateReadySyncAuthorityCandidateAgainstCanonicalV2(ctx, tx, candidate, resultingBase); err != nil {
		return err
	}

	result, err = tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidate_membership_events
WHERE project_id = ? AND candidate_id = ?`, projectID, receipt.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != int64(receipt.Snapshot.MembershipGeneration) {
		return syncProblem(SyncErrorConflict, "candidate", "authority candidate membership events changed during promotion")
	}
	result, err = tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidate_environments
WHERE project_id = ? AND candidate_id = ?`, projectID, receipt.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != receipt.EnvironmentCount {
		return syncProblem(SyncErrorConflict, "candidate", "authority candidate environments changed during promotion")
	}
	result, err = tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidate_pages
WHERE project_id = ? AND candidate_id = ?`, projectID, receipt.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != receipt.PageCount {
		return syncProblem(SyncErrorConflict, "candidate", "authority candidate pages changed during promotion")
	}

	var baseVersion, baseDigest any
	if receipt.Snapshot.BaseAuthorityDigestVersion != 0 {
		baseVersion = receipt.Snapshot.BaseAuthorityDigestVersion
		baseDigest = receipt.Snapshot.BaseAuthorityDigest[:]
	}
	result, err = tx.ExecContext(ctx, `
UPDATE continuity_sync_authority_candidates
SET state = 'promoted'
WHERE project_id = ? AND candidate_id = ? AND state = 'ready'
  AND channel_id = ? AND relay_generation = ? AND admin_public_key = ?
  AND membership_generation = ? AND inventory_arrival_head = ?
  AND ((? IS NULL AND base_authority_digest_version IS NULL AND base_authority_digest IS NULL)
       OR (base_authority_digest_version = ? AND base_authority_digest = ?))
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ? AND authority_digest_version = ? AND authority_digest = ?`,
		projectID, receipt.CandidateID[:], receipt.Snapshot.ChannelID[:], receipt.Snapshot.RelayGeneration[:],
		receipt.Snapshot.AdminPublicKey[:], receipt.Snapshot.MembershipGeneration, receipt.Snapshot.InventoryArrivalHead,
		baseVersion, baseVersion, baseDigest, receipt.PageCount, receipt.EnvironmentCount,
		receipt.ThroughEnvironmentID, receipt.RollingEnvironmentDigest[:], receipt.AuthorityDigestVersion,
		receipt.AuthorityDigest[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "candidate", "authority candidate changed during promotion")
	}
	var childPages, childEnvironments, childEvents int64
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages WHERE project_id = ? AND candidate_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments WHERE project_id = ? AND candidate_id = ?),
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events WHERE project_id = ? AND candidate_id = ?)`,
		projectID, receipt.CandidateID[:], projectID, receipt.CandidateID[:], projectID, receipt.CandidateID[:],
	).Scan(&childPages, &childEnvironments, &childEvents); err != nil {
		return syncTransactionProblem(ctx)
	}
	if childPages != 0 || childEnvironments != 0 || childEvents != 0 {
		return syncProblem(SyncErrorConflict, "candidate", "authority candidate children survived promotion")
	}
	wantBinding := SyncAuthorityBinding{
		ChannelID:              receipt.Snapshot.ChannelID,
		RelayGeneration:        receipt.Snapshot.RelayGeneration,
		AdminPublicKey:         receipt.Snapshot.AdminPublicKey,
		MembershipGeneration:   receipt.Snapshot.MembershipGeneration,
		InventoryArrivalHead:   receipt.Snapshot.InventoryArrivalHead,
		AuthorityDigestVersion: receipt.AuthorityDigestVersion,
		AuthorityDigest:        receipt.AuthorityDigest,
	}
	gotBinding, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, receipt.ProjectID)
	if err != nil {
		return err
	}
	if gotBinding != wantBinding {
		return syncProblem(SyncErrorConflict, "sync_authority", "canonical authority binding changed during promotion")
	}
	return nil
}
