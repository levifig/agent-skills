package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

type persistedSyncAuthorityRecoveryStateV1 struct {
	value       SyncAuthorityRecoveryState
	predecessor *persistedSyncAuthorityCandidateV2
	successor   persistedSyncAuthorityCandidateV2
}

// readAndValidateSyncAuthorityRecoveryAppendStateV1 audits only the fixed-size
// transition, candidate headers, exact last pages, relay watermark, and
// canonical base needed to authorize one bounded STAGING append. It
// deliberately does not stream either complete candidate graph. The append
// path runs the complete recovery-state audit before READY can become durable;
// public reads, replacement, abort, and promotion also retain their complete
// audits.
func readAndValidateSyncAuthorityRecoveryAppendStateV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	localWriterEnvironmentID continuity.EnvironmentID,
) (persistedSyncAuthorityRecoveryStateV1, bool, error) {
	return readAndValidateSyncAuthorityRecoveryStateWithAuditV1(
		ctx, tx, projectID, localWriterEnvironmentID, false,
	)
}

func readAndValidateSyncAuthorityRecoveryStateV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	localWriterEnvironmentID continuity.EnvironmentID,
) (persistedSyncAuthorityRecoveryStateV1, bool, error) {
	return readAndValidateSyncAuthorityRecoveryStateWithAuditV1(
		ctx, tx, projectID, localWriterEnvironmentID, true,
	)
}

func readAndValidateSyncAuthorityRecoveryStateWithAuditV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	localWriterEnvironmentID continuity.EnvironmentID,
	fullAudit bool,
) (persistedSyncAuthorityRecoveryStateV1, bool, error) {
	transition, found, err := readAndAuditSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID)
	if err != nil || !found {
		return persistedSyncAuthorityRecoveryStateV1{}, found, err
	}
	if transition.WriterEnvironmentID != localWriterEnvironmentID {
		return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("transition writer does not match the local store writer")
	}

	successor, found, err := readSyncAuthorityCandidateByRoleV1(
		ctx, tx, projectID, transition.SuccessorCandidateID, syncAuthorityCandidateRoleRecoverySuccessorV1,
	)
	if err != nil {
		return persistedSyncAuthorityRecoveryStateV1{}, false, err
	}
	if !found {
		return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("successor candidate is missing")
	}
	if fullAudit {
		if err := streamAndValidateSyncAuthorityCandidateV2(ctx, tx, successor); err != nil {
			return persistedSyncAuthorityRecoveryStateV1{}, false, err
		}
	}
	var watermarkErr error
	if fullAudit {
		watermarkErr = requireRetainedSyncAuthorityRecoveryWatermarkV1(ctx, tx, projectID, successor.candidate.Snapshot)
	} else {
		_, watermarkErr = readAndValidateRetainedSyncAuthorityRecoveryWatermarkV1(ctx, tx, projectID, successor.candidate.Snapshot)
	}
	if watermarkErr != nil {
		return persistedSyncAuthorityRecoveryStateV1{}, false, watermarkErr
	}

	state := persistedSyncAuthorityRecoveryStateV1{
		value: SyncAuthorityRecoveryState{
			Transition: transition,
			Successor:  successor.candidate,
		},
		successor: successor,
	}
	canonicalBase, err := readCanonicalSyncAuthorityBaseV2(ctx, tx, projectID)
	if err != nil {
		return persistedSyncAuthorityRecoveryStateV1{}, false, err
	}
	if transition.PredecessorCandidateID == ([32]byte{}) {
		if canonicalBase.found || transition.TargetMembershipGeneration != 1 ||
			successor.candidate.Snapshot.BaseAuthorityDigestVersion != 0 ||
			successor.candidate.Snapshot.BaseAuthorityDigest != ([32]byte{}) {
			return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("generation-one successor has an invalid authority base")
		}
		ordinaryExists, err := syncAuthorityRecoveryOrdinaryCandidateExistsV1(ctx, tx, projectID)
		if err != nil {
			return persistedSyncAuthorityRecoveryStateV1{}, false, err
		}
		if ordinaryExists {
			return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("generation-one transition coexists with an ordinary authority candidate")
		}
	} else {
		predecessor, predecessorFound, err := readSyncAuthorityCandidateByRoleV1(
			ctx, tx, projectID, transition.PredecessorCandidateID, syncAuthorityCandidateRoleRecoveryPredecessorV1,
		)
		if err != nil {
			return persistedSyncAuthorityRecoveryStateV1{}, false, err
		}
		if !predecessorFound || !predecessor.candidate.Ready {
			return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("predecessor candidate is missing or not ready")
		}
		if fullAudit {
			if err := streamAndValidateSyncAuthorityCandidateV2(ctx, tx, predecessor); err != nil {
				return persistedSyncAuthorityRecoveryStateV1{}, false, err
			}
		}
		if err := validateSyncAuthorityCandidateBaseV2(
			predecessor.candidate.Snapshot, canonicalBase.digestVersion, canonicalBase.digest, canonicalBase.found,
		); err != nil {
			return persistedSyncAuthorityRecoveryStateV1{}, false, err
		}
		if err := validateSyncAuthorityCandidateHeaderAgainstCanonicalV2(predecessor.candidate.Snapshot, canonicalBase); err != nil {
			return persistedSyncAuthorityRecoveryStateV1{}, false, err
		}
		if fullAudit {
			if err := validateReadySyncAuthorityCandidateAgainstCanonicalV2(ctx, tx, predecessor, canonicalBase); err != nil {
				return persistedSyncAuthorityRecoveryStateV1{}, false, err
			}
		}
		if predecessor.candidate.Snapshot.MembershipGeneration != transition.TargetMembershipGeneration-1 ||
			successor.candidate.Snapshot.ChannelID != predecessor.candidate.Snapshot.ChannelID ||
			successor.candidate.Snapshot.RelayGeneration != predecessor.candidate.Snapshot.RelayGeneration ||
			successor.candidate.Snapshot.AdminPublicKey != predecessor.candidate.Snapshot.AdminPublicKey ||
			successor.candidate.Snapshot.InventoryArrivalHead < predecessor.candidate.Snapshot.InventoryArrivalHead ||
			successor.candidate.Snapshot.BaseAuthorityDigestVersion != 2 ||
			successor.candidate.Snapshot.BaseAuthorityDigest != predecessor.candidate.AuthorityDigest {
			return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("successor is not directly based on the retained predecessor")
		}
		predecessorCopy := predecessor
		state.predecessor = &predecessorCopy
	}
	if successor.candidate.Snapshot.MembershipGeneration < transition.TargetMembershipGeneration {
		return persistedSyncAuthorityRecoveryStateV1{}, false, corruptSyncAuthorityRecoveryTransitionV1("successor membership is below the recovery target")
	}
	if fullAudit && successor.candidate.Ready {
		if state.predecessor != nil {
			if err := validateReadySyncAuthorityRecoverySuccessorExtensionV1(ctx, tx, state); err != nil {
				return persistedSyncAuthorityRecoveryStateV1{}, false, err
			}
		}
		if err := validateReadySyncAuthorityRecoveryWriterV1(ctx, tx, state, true); err != nil {
			return persistedSyncAuthorityRecoveryStateV1{}, false, err
		}
	}
	return state, true, nil
}

func syncAuthorityRecoveryOrdinaryCandidateExistsV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
) (bool, error) {
	var exists int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_authority_candidates
  WHERE project_id = ? AND role = 'ordinary'
)`, string(projectID)).Scan(&exists); err != nil {
		return false, syncTransactionProblem(ctx)
	}
	return exists != 0, nil
}

func readSyncAuthorityCandidateByRoleV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
	role string,
) (persistedSyncAuthorityCandidateV2, bool, error) {
	var persisted persistedSyncAuthorityCandidateV2
	var retainedCandidateID, channelID, relayGeneration, adminPublicKey, baseDigest, rollingDigest, authorityDigest []byte
	var membershipGeneration, inventoryArrivalHead, pageCount, environmentCount, authorityDigestVersion int64
	var baseVersion sql.NullInt64
	err := tx.QueryRowContext(ctx, `
SELECT
  candidate_id, state, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ? AND role = ?`, string(projectID), candidateID[:], role).Scan(
		&retainedCandidateID, &persisted.state, &channelID, &relayGeneration, &adminPublicKey,
		&membershipGeneration, &inventoryArrivalHead, &baseVersion, &baseDigest,
		&pageCount, &environmentCount, &persisted.candidate.ThroughEnvironmentID,
		&rollingDigest, &authorityDigestVersion, &authorityDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return persistedSyncAuthorityCandidateV2{}, false, nil
	}
	if err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, syncTransactionProblem(ctx)
	}
	persisted.candidate.ProjectID = projectID
	persisted.candidate.PageCount = pageCount
	persisted.candidate.EnvironmentCount = environmentCount
	if authorityDigestVersion >= 0 && authorityDigestVersion <= math.MaxUint16 {
		persisted.candidate.AuthorityDigestVersion = uint16(authorityDigestVersion)
	}
	if len(retainedCandidateID) != sha256.Size || isZeroDigestBytesV2(retainedCandidateID) ||
		len(channelID) != len(persisted.candidate.Snapshot.ChannelID) || isZeroDigestBytesV2(channelID) ||
		len(relayGeneration) != len(persisted.candidate.Snapshot.RelayGeneration) || isZeroDigestBytesV2(relayGeneration) ||
		len(adminPublicKey) != len(persisted.candidate.Snapshot.AdminPublicKey) || isZeroDigestBytesV2(adminPublicKey) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 || inventoryArrivalHead < 0 ||
		pageCount < 1 || environmentCount < 1 || !validOpaqueID(persisted.candidate.ThroughEnvironmentID) ||
		len(rollingDigest) != sha256.Size || isZeroDigestBytesV2(rollingDigest) || authorityDigestVersion != 2 {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("recovery candidate header is malformed")
	}
	copy(persisted.candidate.CandidateID[:], retainedCandidateID)
	copy(persisted.candidate.Snapshot.ChannelID[:], channelID)
	copy(persisted.candidate.Snapshot.RelayGeneration[:], relayGeneration)
	copy(persisted.candidate.Snapshot.AdminPublicKey[:], adminPublicKey)
	copy(persisted.candidate.RollingEnvironmentDigest[:], rollingDigest)
	persisted.candidate.Snapshot.MembershipGeneration = uint32(membershipGeneration)
	persisted.candidate.Snapshot.InventoryArrivalHead = inventoryArrivalHead
	baseAbsent := !baseVersion.Valid && baseDigest == nil
	basePresent := baseVersion.Valid && (baseVersion.Int64 == 1 || baseVersion.Int64 == 2) && len(baseDigest) == sha256.Size && !isZeroDigestBytesV2(baseDigest)
	if !baseAbsent && !basePresent {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("recovery candidate base authority is malformed")
	}
	if basePresent {
		persisted.candidate.Snapshot.BaseAuthorityDigestVersion = uint16(baseVersion.Int64)
		copy(persisted.candidate.Snapshot.BaseAuthorityDigest[:], baseDigest)
	}
	switch persisted.state {
	case "staging":
		if authorityDigest != nil {
			return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("staging recovery candidate has a final digest")
		}
	case "ready":
		if len(authorityDigest) != sha256.Size || isZeroDigestBytesV2(authorityDigest) {
			return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("ready recovery candidate final digest is malformed")
		}
		persisted.candidate.Ready = true
		copy(persisted.candidate.AuthorityDigest[:], authorityDigest)
	default:
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("recovery candidate state is malformed")
	}
	derivedCandidateID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, persisted.candidate.Snapshot)
	if err != nil || derivedCandidateID != persisted.candidate.CandidateID || persisted.candidate.CandidateID != candidateID {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("recovery candidate identity is stale")
	}
	persisted.headerDigest = headerDigest
	if persisted.candidate.Ready {
		derivedAuthorityDigest, err := finalizeSyncAuthorityDigestV2(
			persisted.headerDigest, persisted.candidate.EnvironmentCount, persisted.candidate.RollingEnvironmentDigest,
		)
		if err != nil || derivedAuthorityDigest != persisted.candidate.AuthorityDigest {
			return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("ready recovery candidate final digest is stale")
		}
	}
	lastPage, found, err := readAndValidateSyncAuthorityCandidatePageByNumberV2(ctx, tx, persisted, persisted.candidate.PageCount)
	if err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, err
	}
	if !found || lastPage.page.ThroughEnvironmentID != persisted.candidate.ThroughEnvironmentID ||
		lastPage.resultingEnvironmentCount != persisted.candidate.EnvironmentCount ||
		lastPage.resultingRollingDigest != persisted.candidate.RollingEnvironmentDigest ||
		lastPage.page.More == persisted.candidate.Ready {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("recovery candidate checkpoint is stale")
	}
	return persisted, true, nil
}

func validateReadySyncAuthorityRecoveryWriterV1(
	ctx context.Context,
	tx *sql.Tx,
	state persistedSyncAuthorityRecoveryStateV1,
	persisted bool,
) error {
	transition := state.value.Transition
	var certificateID, retirementID []byte
	var mode string
	var expiresAtMillis, joinMembershipGeneration int64
	err := tx.QueryRowContext(ctx, `
SELECT certificate_id, mode, expires_at_millis,
       join_membership_generation, retirement_id
FROM continuity_sync_authority_candidate_environments
WHERE project_id = ? AND candidate_id = ? AND environment_id = ?`,
		string(transition.ProjectID), transition.SuccessorCandidateID[:], string(transition.WriterEnvironmentID),
	).Scan(&certificateID, &mode, &expiresAtMillis, &joinMembershipGeneration, &retirementID)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		return syncTransactionProblem(ctx)
	}
	field := "writer_environment_id"
	invalid := missing
	if !invalid && (len(certificateID) != sha256.Size || isZeroDigestBytesV2(certificateID)) {
		invalid = true
		field = "writer_certificate_id"
	}
	if !invalid {
		var certificate [32]byte
		copy(certificate[:], certificateID)
		if certificate != transition.WriterCertificateID {
			invalid = true
			field = "writer_certificate_id"
		}
	}
	if !invalid && mode != string(SyncEnvironmentTrusted) {
		invalid = true
		field = "writer_mode"
	}
	if !invalid && expiresAtMillis != 0 {
		invalid = true
		field = "writer_expiry"
	}
	if !invalid && retirementID != nil {
		invalid = true
		field = "writer_retirement"
	}
	if !invalid && joinMembershipGeneration != int64(transition.TargetMembershipGeneration) {
		invalid = true
		field = "target_membership_generation"
	}
	if !invalid {
		var exactJoin int
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidate_membership_events
  WHERE project_id = ? AND candidate_id = ?
    AND membership_generation = ? AND event_kind = 'join'
    AND environment_id = ?
)`, string(transition.ProjectID), transition.SuccessorCandidateID[:],
			transition.TargetMembershipGeneration, string(transition.WriterEnvironmentID),
		).Scan(&exactJoin); err != nil {
			return syncTransactionProblem(ctx)
		}
		if exactJoin != 1 {
			invalid = true
			field = "target_membership_generation"
		}
	}
	if !invalid {
		return nil
	}
	if persisted {
		return corruptSyncAuthorityRecoveryTransitionV1("ready successor does not contain the exact active local writer registration")
	}
	return syncProblem(SyncErrorConflict, field, "ready successor does not contain the exact active local writer registration")
}
