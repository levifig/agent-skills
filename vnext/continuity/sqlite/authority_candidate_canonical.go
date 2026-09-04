package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

type canonicalSyncAuthorityBaseV2 struct {
	authority     SyncAuthority
	digestVersion uint16
	digest        [32]byte
	found         bool
}

const canonicalSyncAuthorityPageRangeSelectV2 = `
SELECT
  environment_id, certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation, retirement_relay_generation,
  retirement_membership_generation, retirement_final_environment_sequence,
  retirement_final_envelope_digest, retirement_id, retirement_bytes
FROM continuity_sync_environment_certificates
WHERE project_id = ?`

const canonicalSyncAuthorityFirstPageRangeQueryV2 = canonicalSyncAuthorityPageRangeSelectV2 + `
  AND environment_id <= ?
ORDER BY environment_id
LIMIT ?`

const canonicalSyncAuthoritySubsequentPageRangeQueryV2 = canonicalSyncAuthorityPageRangeSelectV2 + `
  AND environment_id > ? AND environment_id <= ?
ORDER BY environment_id
LIMIT ?`

const canonicalSyncAuthorityFirstFinalPageRangeQueryV2 = canonicalSyncAuthorityPageRangeSelectV2 + `
ORDER BY environment_id
LIMIT ?`

const canonicalSyncAuthoritySubsequentFinalPageRangeQueryV2 = canonicalSyncAuthorityPageRangeSelectV2 + `
  AND environment_id > ?
ORDER BY environment_id
LIMIT ?`

const canonicalSyncAuthorityInventoryQueryV2 = `
SELECT
  environment_id, certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation, retirement_relay_generation,
  retirement_membership_generation, retirement_final_environment_sequence,
  retirement_final_envelope_digest, retirement_id, retirement_bytes
FROM continuity_sync_environment_certificates
WHERE project_id = ?
ORDER BY environment_id`

const canonicalSyncAuthorityLegacyInventoryQueryV2 = canonicalSyncAuthorityInventoryQueryV2 + `
LIMIT ?`

const syncAuthorityCandidateOmittedCanonicalInventoryQueryV2 = `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_environment_certificates AS canonical
  LEFT JOIN continuity_sync_authority_candidate_environments AS candidate
    ON candidate.project_id = ?
   AND candidate.candidate_id = ?
   AND candidate.environment_id = canonical.environment_id
  WHERE canonical.project_id = ?
    AND candidate.environment_id IS NULL
    AND (? = 1 OR canonical.environment_id <= ?)
)`

const syncAuthorityCandidateChangedCanonicalInventoryQueryV2 = `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_environments AS candidate
  JOIN continuity_sync_environment_certificates AS canonical
    ON canonical.project_id = candidate.project_id
   AND canonical.environment_id = candidate.environment_id
  WHERE candidate.project_id = ? AND candidate.candidate_id = ?
    AND (
      NOT (
        candidate.certificate_id IS canonical.certificate_id
        AND candidate.certificate_bytes IS canonical.certificate_bytes
        AND candidate.mode = canonical.mode
        AND candidate.expires_at_millis = canonical.expires_at_millis
        AND candidate.join_membership_generation = canonical.join_membership_generation
      )
      OR NOT (
        (canonical.retirement_id IS NULL AND candidate.retirement_id IS NULL)
        OR (
          canonical.retirement_id IS NOT NULL
          AND candidate.retirement_id IS NOT NULL
          AND candidate.retirement_relay_generation IS canonical.retirement_relay_generation
          AND candidate.retirement_membership_generation = canonical.retirement_membership_generation
          AND candidate.retirement_final_environment_sequence = canonical.retirement_final_environment_sequence
          AND candidate.retirement_final_envelope_digest IS canonical.retirement_final_envelope_digest
          AND candidate.retirement_id IS canonical.retirement_id
          AND candidate.retirement_bytes IS canonical.retirement_bytes
        )
        OR (
          canonical.retirement_id IS NULL
          AND candidate.retirement_id IS NOT NULL
          AND ? = 1
          AND candidate.retirement_membership_generation > ?
        )
      )
    )
)`

const syncAuthorityCandidateInvalidNewInventoryQueryV2 = `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_environments AS candidate
  LEFT JOIN continuity_sync_environment_certificates AS canonical
    ON canonical.project_id = candidate.project_id
   AND canonical.environment_id = candidate.environment_id
  WHERE candidate.project_id = ? AND candidate.candidate_id = ?
    AND canonical.environment_id IS NULL
    AND (? = 0 OR candidate.join_membership_generation <= ?)
)`

func readCanonicalSyncAuthorityBaseV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (canonicalSyncAuthorityBaseV2, error) {
	var base canonicalSyncAuthorityBaseV2
	var channelID, relayGeneration, adminPublicKey []byte
	var membershipGeneration int64
	err := tx.QueryRowContext(ctx, `
SELECT channel_id, relay_generation, admin_public_key, membership_generation
FROM continuity_sync_projects
WHERE project_id = ?`, string(projectID)).Scan(&channelID, &relayGeneration, &adminPublicKey, &membershipGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		var orphaned int
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_authorities WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_environment_certificates WHERE project_id = ?
)`, string(projectID), string(projectID)).Scan(&orphaned); err != nil {
			return canonicalSyncAuthorityBaseV2{}, syncTransactionProblem(ctx)
		}
		if orphaned != 0 {
			return canonicalSyncAuthorityBaseV2{}, syncProblem(SyncErrorStore, "sync_authority", "orphaned canonical authority rows exist")
		}
		return base, nil
	}
	if err != nil {
		return canonicalSyncAuthorityBaseV2{}, syncTransactionProblem(ctx)
	}
	if len(channelID) != len(base.authority.ChannelID) || isZeroDigestBytesV2(channelID) ||
		len(relayGeneration) != len(base.authority.RelayGeneration) || isZeroDigestBytesV2(relayGeneration) ||
		len(adminPublicKey) != len(base.authority.AdminPublicKey) || isZeroDigestBytesV2(adminPublicKey) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 {
		return canonicalSyncAuthorityBaseV2{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority header is corrupt")
	}
	copy(base.authority.ChannelID[:], channelID)
	copy(base.authority.RelayGeneration[:], relayGeneration)
	copy(base.authority.AdminPublicKey[:], adminPublicKey)
	base.authority.MembershipGeneration = uint32(membershipGeneration)
	var digestVersion int64
	var digestBytes []byte
	if err := tx.QueryRowContext(ctx, `
SELECT digest_version, authority_digest, inventory_arrival_head
FROM continuity_sync_authorities
WHERE project_id = ?`, string(projectID)).Scan(&digestVersion, &digestBytes, &base.authority.InventoryArrivalHead); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return canonicalSyncAuthorityBaseV2{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is missing")
		}
		return canonicalSyncAuthorityBaseV2{}, syncTransactionProblem(ctx)
	}
	if (digestVersion != 1 && digestVersion != 2) || len(digestBytes) != sha256.Size || isZeroDigestBytesV2(digestBytes) ||
		base.authority.InventoryArrivalHead < 0 || (digestVersion == 1 && base.authority.InventoryArrivalHead != 0) {
		return canonicalSyncAuthorityBaseV2{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is corrupt")
	}
	base.digestVersion = uint16(digestVersion)
	copy(base.digest[:], digestBytes)
	base.found = true
	return base, nil
}

func validateSyncAuthorityCandidateHeaderAgainstCanonicalV2(snapshot SyncAuthoritySnapshot, base canonicalSyncAuthorityBaseV2) error {
	if !base.found {
		return nil
	}
	if snapshot.ChannelID != base.authority.ChannelID {
		return syncProblem(SyncErrorConflict, "channel_id", "does not match the canonical authority")
	}
	if snapshot.RelayGeneration != base.authority.RelayGeneration {
		return syncProblem(SyncErrorConflict, "relay_generation", "does not match the canonical authority")
	}
	if snapshot.AdminPublicKey != base.authority.AdminPublicKey {
		return syncProblem(SyncErrorConflict, "admin_public_key", "does not match the canonical authority")
	}
	if snapshot.MembershipGeneration < base.authority.MembershipGeneration {
		return syncProblem(SyncErrorConflict, "membership_generation", "regressed below the canonical authority")
	}
	if snapshot.InventoryArrivalHead < base.authority.InventoryArrivalHead {
		return syncProblem(SyncErrorConflict, "inventory_arrival_head", "regressed below the canonical authority")
	}
	return nil
}

func validateSyncAuthorityCandidatePageAgainstCanonicalV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
	page SyncAuthorityPage,
	base canonicalSyncAuthorityBaseV2,
	final bool,
) error {
	if !base.found {
		return nil
	}
	pageByEnvironment := make(map[string]SyncEnvironmentCertificate, len(page.Environments))
	for _, environment := range page.Environments {
		pageByEnvironment[environment.EnvironmentID] = environment
	}
	canonicalInPage := make(map[string]struct{}, len(page.Environments))
	var rows *sql.Rows
	var err error
	if page.AfterEnvironmentID == "" && final {
		rows, err = tx.QueryContext(
			ctx, canonicalSyncAuthorityFirstFinalPageRangeQueryV2,
			string(projectID), maximumSyncAuthorityCandidateBoundedReadRowsV2,
		)
	} else if page.AfterEnvironmentID == "" {
		rows, err = tx.QueryContext(
			ctx, canonicalSyncAuthorityFirstPageRangeQueryV2,
			string(projectID), page.ThroughEnvironmentID, maximumSyncAuthorityCandidateBoundedReadRowsV2,
		)
	} else if final {
		rows, err = tx.QueryContext(
			ctx, canonicalSyncAuthoritySubsequentFinalPageRangeQueryV2,
			string(projectID), page.AfterEnvironmentID, maximumSyncAuthorityCandidateBoundedReadRowsV2,
		)
	} else {
		rows, err = tx.QueryContext(
			ctx, canonicalSyncAuthoritySubsequentPageRangeQueryV2,
			string(projectID), page.AfterEnvironmentID, page.ThroughEnvironmentID, maximumSyncAuthorityCandidateBoundedReadRowsV2,
		)
	}
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	for rows.Next() {
		canonicalEnvironment, err := scanSyncEnvironmentCertificateV1(rows)
		if err != nil {
			rows.Close()
			return err
		}
		candidateEnvironment, found := pageByEnvironment[canonicalEnvironment.EnvironmentID]
		if !found {
			rows.Close()
			return syncProblem(SyncErrorConflict, "environments", "omits a canonical environment from the staged range")
		}
		if !syncEnvironmentCertificateFieldsEqual(canonicalEnvironment, candidateEnvironment) {
			rows.Close()
			return syncProblem(SyncErrorConflict, "environment", "changes a canonical environment certificate")
		}
		switch {
		case syncEnvironmentCertificateEqual(canonicalEnvironment, candidateEnvironment):
		case canonicalEnvironment.Retirement == nil && candidateEnvironment.Retirement != nil:
			if snapshot.MembershipGeneration <= base.authority.MembershipGeneration ||
				candidateEnvironment.Retirement.MembershipGeneration <= base.authority.MembershipGeneration {
				rows.Close()
				return syncProblem(SyncErrorConflict, "retirement", "does not append after canonical membership")
			}
		default:
			rows.Close()
			return syncProblem(SyncErrorConflict, "retirement", "changes a canonical terminal retirement")
		}
		canonicalInPage[canonicalEnvironment.EnvironmentID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	for _, environment := range page.Environments {
		if _, exists := canonicalInPage[environment.EnvironmentID]; exists {
			continue
		}
		if environment.JoinMembershipGeneration <= base.authority.MembershipGeneration {
			return syncProblem(SyncErrorConflict, "join_membership_generation", "new environment does not follow canonical membership")
		}
	}
	return nil
}

// validateReadySyncAuthorityCandidateAgainstCanonicalV2 is the load-bearing
// composite proof for a fully validated persisted READY candidate. Candidate
// event coverage supplies the exact membership-generation stream; the two
// canonical passes below prove that every canonical event is retained and that
// only joins and terminal retirements after the canonical base are appended.
func validateReadySyncAuthorityCandidateAgainstCanonicalV2(
	ctx context.Context,
	tx *sql.Tx,
	candidate persistedSyncAuthorityCandidateV2,
	base canonicalSyncAuthorityBaseV2,
) error {
	if candidate.state != "ready" || !candidate.candidate.Ready {
		return corruptSyncAuthorityCandidateV2("composite validation requires a ready candidate")
	}
	if err := validateCanonicalSyncAuthorityStructureAndDigestV2(ctx, tx, candidate.candidate.ProjectID, base); err != nil {
		return err
	}
	return validateReadySyncAuthorityCandidateInventoryAgainstCanonicalV2(ctx, tx, candidate, base)
}

func validateCanonicalSyncAuthorityStructureAndDigestV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, base canonicalSyncAuthorityBaseV2) error {
	if !base.found {
		return nil
	}
	if base.digestVersion == 1 {
		_, version, digest, found, err := readCanonicalSyncAuthorityForCandidateV2(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if !found || version != base.digestVersion || digest != base.digest {
			return syncProblem(SyncErrorStore, "sync_authority", "pinned v1 authority changed during validation")
		}
		return nil
	}
	snapshot := syncAuthoritySnapshotFromAuthorityV2(base.authority, 0, [32]byte{})
	_, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
	if err != nil {
		return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority header cannot be encoded")
	}
	rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
	if err != nil {
		return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority rolling seed cannot be derived")
	}
	rows, err := tx.QueryContext(ctx, canonicalSyncAuthorityInventoryQueryV2, string(projectID))
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	previousEnvironmentID := ""
	var environmentCount int64
	for rows.Next() {
		environment, err := scanSyncEnvironmentCertificateV1(rows)
		if err != nil {
			rows.Close()
			return err
		}
		if err := validateSyncAuthorityCandidateEnvironmentV2(environment, 0); err != nil || environment.EnvironmentID <= previousEnvironmentID ||
			environment.JoinMembershipGeneration > base.authority.MembershipGeneration ||
			(environment.Retirement != nil && (environment.Retirement.RelayGeneration != base.authority.RelayGeneration ||
				environment.Retirement.MembershipGeneration > base.authority.MembershipGeneration)) {
			rows.Close()
			return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority inventory is corrupt")
		}
		environmentCount, err = checkedSyncAuthorityCandidateAdvanceV2(environmentCount, 1)
		if err != nil {
			rows.Close()
			return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority inventory overflows")
		}
		rolling, _, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, environmentCount, environment)
		if err != nil {
			rows.Close()
			return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority environment cannot be encoded")
		}
		previousEnvironmentID = environment.EnvironmentID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if environmentCount < 1 {
		return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority inventory is empty")
	}
	digest, err := finalizeSyncAuthorityDigestV2(headerDigest, environmentCount, rolling)
	if err != nil || digest != base.digest {
		return syncProblem(SyncErrorStore, "sync_authority", "pinned v2 authority metadata is stale")
	}
	return nil
}

func validateReadySyncAuthorityCandidateInventoryAgainstCanonicalV2(ctx context.Context, tx *sql.Tx, candidate persistedSyncAuthorityCandidateV2, base canonicalSyncAuthorityBaseV2) error {
	if !base.found {
		return nil
	}
	if err := validateSyncAuthorityCandidateHeaderAgainstCanonicalV2(candidate.candidate.Snapshot, base); err != nil {
		return err
	}
	var omitted int
	if err := tx.QueryRowContext(ctx, syncAuthorityCandidateOmittedCanonicalInventoryQueryV2,
		string(candidate.candidate.ProjectID), candidate.candidate.CandidateID[:], string(candidate.candidate.ProjectID),
		boolIntV2(candidate.candidate.Ready), candidate.candidate.ThroughEnvironmentID,
	).Scan(&omitted); err != nil {
		return syncTransactionProblem(ctx)
	}
	if omitted != 0 {
		return syncProblem(SyncErrorConflict, "environments", "omits a canonical environment from the staged prefix")
	}
	var changed int
	membershipAdvanced := candidate.candidate.Snapshot.MembershipGeneration > base.authority.MembershipGeneration
	if err := tx.QueryRowContext(ctx, syncAuthorityCandidateChangedCanonicalInventoryQueryV2,
		string(candidate.candidate.ProjectID), candidate.candidate.CandidateID[:], boolIntV2(membershipAdvanced), base.authority.MembershipGeneration,
	).Scan(&changed); err != nil {
		return syncTransactionProblem(ctx)
	}
	if changed != 0 {
		return syncProblem(SyncErrorConflict, "environment", "changes a canonical environment or retirement")
	}
	var invalidNew int
	if err := tx.QueryRowContext(ctx, syncAuthorityCandidateInvalidNewInventoryQueryV2,
		string(candidate.candidate.ProjectID), candidate.candidate.CandidateID[:], boolIntV2(membershipAdvanced), base.authority.MembershipGeneration,
	).Scan(&invalidNew); err != nil {
		return syncTransactionProblem(ctx)
	}
	if invalidNew != 0 {
		return syncProblem(SyncErrorConflict, "environments", "adds an environment outside an advancing canonical membership")
	}
	return nil
}

func boolIntV2(value bool) int {
	if value {
		return 1
	}
	return 0
}

func readCanonicalSyncAuthorityForCandidateV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (SyncAuthority, uint16, [32]byte, bool, error) {
	var authority SyncAuthority
	var channelID, relayGeneration, adminPublicKey []byte
	var membershipGeneration int64
	err := tx.QueryRowContext(ctx, `
SELECT channel_id, relay_generation, admin_public_key, membership_generation
FROM continuity_sync_projects
WHERE project_id = ?`, string(projectID)).Scan(&channelID, &relayGeneration, &adminPublicKey, &membershipGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthority{}, 0, [32]byte{}, false, nil
	}
	if err != nil {
		return SyncAuthority{}, 0, [32]byte{}, false, syncTransactionProblem(ctx)
	}
	if len(channelID) != len(authority.ChannelID) || bytes.Equal(channelID, make([]byte, len(channelID))) ||
		len(relayGeneration) != len(authority.RelayGeneration) || bytes.Equal(relayGeneration, make([]byte, len(relayGeneration))) ||
		len(adminPublicKey) != len(authority.AdminPublicKey) || bytes.Equal(adminPublicKey, make([]byte, len(adminPublicKey))) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 {
		return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority header is corrupt")
	}
	copy(authority.ChannelID[:], channelID)
	copy(authority.RelayGeneration[:], relayGeneration)
	copy(authority.AdminPublicKey[:], adminPublicKey)
	authority.MembershipGeneration = uint32(membershipGeneration)
	var digestVersion int64
	var digestBytes []byte
	if err := tx.QueryRowContext(ctx, `
SELECT digest_version, authority_digest, inventory_arrival_head
FROM continuity_sync_authorities
WHERE project_id = ?`, string(projectID)).Scan(&digestVersion, &digestBytes, &authority.InventoryArrivalHead); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is missing")
		}
		return SyncAuthority{}, 0, [32]byte{}, false, syncTransactionProblem(ctx)
	}
	if (digestVersion != 1 && digestVersion != 2) || len(digestBytes) != sha256.Size || bytes.Equal(digestBytes, make([]byte, sha256.Size)) ||
		authority.InventoryArrivalHead < 0 || (digestVersion == 1 && authority.InventoryArrivalHead != 0) {
		return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is corrupt")
	}
	var digest [32]byte
	copy(digest[:], digestBytes)
	inventoryQuery := canonicalSyncAuthorityInventoryQueryV2
	inventoryArguments := []any{string(projectID)}
	if digestVersion == 1 {
		inventoryQuery = canonicalSyncAuthorityLegacyInventoryQueryV2
		inventoryArguments = append(inventoryArguments, maximumSyncAuthorityEnvironments+1)
	}
	rows, err := tx.QueryContext(ctx, inventoryQuery, inventoryArguments...)
	if err != nil {
		return SyncAuthority{}, 0, [32]byte{}, false, syncTransactionProblem(ctx)
	}
	previousEnvironmentID := ""
	seenCertificateIDs := make(map[[32]byte]struct{})
	seenEvents := make(map[uint32]struct{})
	for rows.Next() {
		environment, err := scanSyncEnvironmentCertificateV1(rows)
		if err != nil {
			rows.Close()
			return SyncAuthority{}, 0, [32]byte{}, false, err
		}
		if digestVersion == 1 && len(authority.Environments) == maximumSyncAuthorityEnvironments {
			rows.Close()
			return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned v1 authority inventory exceeds the fixed bound")
		}
		if err := validateSyncAuthorityCandidateEnvironmentV2(environment, len(authority.Environments)); err != nil ||
			environment.EnvironmentID <= previousEnvironmentID || environment.JoinMembershipGeneration > authority.MembershipGeneration ||
			(environment.Retirement != nil && (environment.Retirement.RelayGeneration != authority.RelayGeneration || environment.Retirement.MembershipGeneration > authority.MembershipGeneration)) {
			rows.Close()
			return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority inventory is corrupt")
		}
		if _, duplicate := seenCertificateIDs[environment.CertificateID]; duplicate {
			rows.Close()
			return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority certificate is duplicated")
		}
		seenCertificateIDs[environment.CertificateID] = struct{}{}
		if _, duplicate := seenEvents[environment.JoinMembershipGeneration]; duplicate {
			rows.Close()
			return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority membership is duplicated")
		}
		seenEvents[environment.JoinMembershipGeneration] = struct{}{}
		if environment.Retirement != nil {
			if _, duplicate := seenEvents[environment.Retirement.MembershipGeneration]; duplicate {
				rows.Close()
				return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority membership is duplicated")
			}
			seenEvents[environment.Retirement.MembershipGeneration] = struct{}{}
		}
		authority.Environments = append(authority.Environments, environment)
		previousEnvironmentID = environment.EnvironmentID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return SyncAuthority{}, 0, [32]byte{}, false, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return SyncAuthority{}, 0, [32]byte{}, false, syncTransactionProblem(ctx)
	}
	if len(authority.Environments) < 1 || uint64(len(seenEvents)) != uint64(authority.MembershipGeneration) {
		return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority membership coverage is corrupt")
	}
	for generation := uint32(1); generation <= authority.MembershipGeneration; generation++ {
		if _, found := seenEvents[generation]; !found {
			return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority membership coverage has a gap")
		}
		if generation == math.MaxUint32 {
			break
		}
	}
	var derived [32]byte
	if digestVersion == 1 {
		derived, err = frozenSyncAuthorityDigestV1(projectID, authority)
	} else {
		snapshot := syncAuthoritySnapshotFromAuthorityV2(authority, 0, [32]byte{})
		_, headerDigest, deriveErr := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
		if deriveErr != nil {
			err = deriveErr
		} else {
			rolling, rollingErr := syncAuthorityCandidateRollingSeedV2(headerDigest)
			if rollingErr != nil {
				err = rollingErr
			} else {
				for index, environment := range authority.Environments {
					rolling, _, rollingErr = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, int64(index+1), environment)
					if rollingErr != nil {
						err = rollingErr
						break
					}
				}
				if err == nil {
					derived, err = finalizeSyncAuthorityDigestV2(headerDigest, int64(len(authority.Environments)), rolling)
				}
			}
		}
	}
	if err != nil || derived != digest {
		return SyncAuthority{}, 0, [32]byte{}, false, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is stale")
	}
	return authority, uint16(digestVersion), digest, true, nil
}

func syncAuthoritySnapshotFromAuthorityV2(authority SyncAuthority, baseVersion uint16, baseDigest [32]byte) SyncAuthoritySnapshot {
	return SyncAuthoritySnapshot{
		ChannelID:                  authority.ChannelID,
		RelayGeneration:            authority.RelayGeneration,
		AdminPublicKey:             authority.AdminPublicKey,
		MembershipGeneration:       authority.MembershipGeneration,
		InventoryArrivalHead:       authority.InventoryArrivalHead,
		BaseAuthorityDigestVersion: baseVersion,
		BaseAuthorityDigest:        baseDigest,
	}
}

func validateSyncAuthorityCandidateBaseV2(snapshot SyncAuthoritySnapshot, canonicalVersion uint16, canonicalDigest [32]byte, canonicalFound bool) error {
	basePresent := snapshot.BaseAuthorityDigestVersion != 0
	if canonicalFound != basePresent {
		return syncProblem(SyncErrorConflict, "base_authority_digest", "does not match canonical authority presence")
	}
	if canonicalFound && (snapshot.BaseAuthorityDigestVersion != canonicalVersion || snapshot.BaseAuthorityDigest != canonicalDigest) {
		return syncProblem(SyncErrorConflict, "base_authority_digest", "does not identify the exact canonical authority")
	}
	return nil
}
