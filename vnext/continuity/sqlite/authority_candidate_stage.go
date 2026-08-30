package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

const maximumSyncAuthorityCandidatePageEnvironments = 4

func prepareSyncAuthorityCandidatePageV2(projectID continuity.ProjectID, snapshot SyncAuthoritySnapshot, page SyncAuthorityPage) (SyncAuthorityPage, error) {
	if err := validateSyncAuthoritySnapshotV2(projectID, snapshot); err != nil {
		return SyncAuthorityPage{}, err
	}
	if len(page.Environments) < 1 || len(page.Environments) > maximumSyncAuthorityCandidatePageEnvironments {
		return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "environments", "page must contain between one and four environments")
	}
	if page.AfterEnvironmentID != "" && !validOpaqueID(page.AfterEnvironmentID) {
		return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "after_environment_id", "is invalid")
	}
	if !validOpaqueID(page.ThroughEnvironmentID) {
		return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "through_environment_id", "is invalid")
	}
	prepared := page
	prepared.Environments = cloneSyncAuthorityCandidateEnvironmentsV2Internal(page.Environments)
	seenCertificateIDs := make(map[[32]byte]struct{}, len(prepared.Environments))
	seenEvents := make(map[uint32]struct{}, len(prepared.Environments)*2)
	previousEnvironmentID := prepared.AfterEnvironmentID
	for index, environment := range prepared.Environments {
		if err := validateSyncAuthorityCandidateEnvironmentV2(environment, index); err != nil {
			return SyncAuthorityPage{}, err
		}
		if environment.EnvironmentID <= previousEnvironmentID {
			return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "environments", "must be strictly sorted after the page cursor")
		}
		previousEnvironmentID = environment.EnvironmentID
		if _, duplicate := seenCertificateIDs[environment.CertificateID]; duplicate {
			return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "environments", "contains duplicate certificate identities")
		}
		seenCertificateIDs[environment.CertificateID] = struct{}{}
		if environment.JoinMembershipGeneration > snapshot.MembershipGeneration {
			return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "join_membership_generation", "exceeds the candidate authority membership")
		}
		if _, duplicate := seenEvents[environment.JoinMembershipGeneration]; duplicate {
			return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "membership_generation", "is claimed by more than one candidate event")
		}
		seenEvents[environment.JoinMembershipGeneration] = struct{}{}
		if environment.Retirement != nil {
			if environment.Retirement.RelayGeneration != snapshot.RelayGeneration || environment.Retirement.MembershipGeneration > snapshot.MembershipGeneration {
				return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "retirement", "is outside the candidate authority")
			}
			if _, duplicate := seenEvents[environment.Retirement.MembershipGeneration]; duplicate {
				return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "membership_generation", "is claimed by more than one candidate event")
			}
			seenEvents[environment.Retirement.MembershipGeneration] = struct{}{}
		}
	}
	if prepared.ThroughEnvironmentID != prepared.Environments[len(prepared.Environments)-1].EnvironmentID {
		return SyncAuthorityPage{}, syncProblem(SyncErrorInvalid, "through_environment_id", "must equal the final environment ID")
	}
	return prepared, nil
}

func cloneSyncAuthorityCandidateEnvironmentsV2Internal(environments []SyncEnvironmentCertificate) []SyncEnvironmentCertificate {
	cloned := make([]SyncEnvironmentCertificate, len(environments))
	for index, environment := range environments {
		cloned[index] = environment
		cloned[index].CertificateBytes = append([]byte(nil), environment.CertificateBytes...)
		if environment.Retirement != nil {
			retirement := *environment.Retirement
			retirement.RetirementBytes = append([]byte(nil), environment.Retirement.RetirementBytes...)
			cloned[index].Retirement = &retirement
		}
	}
	return cloned
}

func exactSyncAuthorityCandidatePageReplayV2(ctx context.Context, tx *sql.Tx, current persistedSyncAuthorityCandidateV2, candidatePage SyncAuthorityPage) (bool, bool, error) {
	persistedPage, found, err := readAndValidateSyncAuthorityCandidatePageByAfterV2(ctx, tx, current, candidatePage.AfterEnvironmentID)
	if err != nil || !found {
		return found, false, err
	}
	if persistedPage.page.ThroughEnvironmentID != candidatePage.ThroughEnvironmentID || persistedPage.page.More != candidatePage.More ||
		len(persistedPage.page.Environments) != len(candidatePage.Environments) {
		return true, false, nil
	}
	for index := range candidatePage.Environments {
		if !syncEnvironmentCertificateEqual(persistedPage.page.Environments[index], candidatePage.Environments[index]) {
			return true, false, nil
		}
	}
	return true, true, nil
}

func insertFirstSyncAuthorityCandidatePageV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
	snapshot SyncAuthoritySnapshot,
	page SyncAuthorityPage,
	headerDigest [32]byte,
) error {
	rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
	if err != nil {
		return syncProblem(SyncErrorStore, "sync_authority_candidate", "rolling seed cannot be derived")
	}
	environmentDigests := make([][32]byte, 0, len(page.Environments))
	for index, environment := range page.Environments {
		var environmentDigest [32]byte
		rolling, environmentDigest, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, int64(index+1), environment)
		if err != nil {
			return syncProblem(SyncErrorInvalid, "environment", "cannot be encoded by the authority candidate codec")
		}
		environmentDigests = append(environmentDigests, environmentDigest)
	}
	environmentCount := int64(len(page.Environments))
	pageDigest, err := syncAuthorityCandidatePageDigestV2(candidateID, 1, page, environmentCount, rolling, environmentDigests)
	if err != nil {
		return syncProblem(SyncErrorInvalid, "page", "cannot be encoded by the authority candidate codec")
	}
	state := "staging"
	var authorityDigest [32]byte
	var authorityDigestValue any
	if !page.More {
		authorityDigest, err = finalizeSyncAuthorityDigestV2(headerDigest, environmentCount, rolling)
		if err != nil {
			return syncProblem(SyncErrorInvalid, "page", "cannot finalize the authority candidate")
		}
		state = "ready"
		authorityDigestValue = authorityDigest[:]
	}
	var baseVersion, baseDigest any
	if snapshot.BaseAuthorityDigestVersion != 0 {
		baseVersion = snapshot.BaseAuthorityDigestVersion
		baseDigest = snapshot.BaseAuthorityDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, 2, ?)`,
		string(projectID), candidateID[:], state, snapshot.ChannelID[:], snapshot.RelayGeneration[:],
		snapshot.AdminPublicKey[:], snapshot.MembershipGeneration, snapshot.InventoryArrivalHead,
		baseVersion, baseDigest, environmentCount, page.ThroughEnvironmentID, rolling[:], authorityDigestValue,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return err
	}
	if err := insertSyncAuthorityCandidatePageRowsV2(ctx, tx, projectID, candidateID, 1, page, pageDigest, environmentCount, rolling, 0); err != nil {
		return err
	}
	return nil
}

func appendSyncAuthorityCandidatePageV2(
	ctx context.Context,
	tx *sql.Tx,
	current persistedSyncAuthorityCandidateV2,
	page SyncAuthorityPage,
	headerDigest [32]byte,
) error {
	pageNumber, err := checkedSyncAuthorityCandidateAdvanceV2(current.candidate.PageCount, 1)
	if err != nil {
		return syncProblem(SyncErrorInvalid, "page_count", "would overflow")
	}
	environmentCount, err := checkedSyncAuthorityCandidateAdvanceV2(current.candidate.EnvironmentCount, int64(len(page.Environments)))
	if err != nil {
		return syncProblem(SyncErrorInvalid, "environment_count", "would overflow")
	}
	rolling := current.candidate.RollingEnvironmentDigest
	environmentDigests := make([][32]byte, 0, len(page.Environments))
	for index, environment := range page.Environments {
		ordinal, err := checkedSyncAuthorityCandidateAdvanceV2(current.candidate.EnvironmentCount, int64(index+1))
		if err != nil {
			return syncProblem(SyncErrorInvalid, "environment_count", "would overflow")
		}
		var environmentDigest [32]byte
		rolling, environmentDigest, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, ordinal, environment)
		if err != nil {
			return syncProblem(SyncErrorInvalid, "environment", "cannot be encoded by the authority candidate codec")
		}
		environmentDigests = append(environmentDigests, environmentDigest)
	}
	pageDigest, err := syncAuthorityCandidatePageDigestV2(current.candidate.CandidateID, pageNumber, page, environmentCount, rolling, environmentDigests)
	if err != nil {
		return syncProblem(SyncErrorInvalid, "page", "cannot be encoded by the authority candidate codec")
	}
	if err := insertSyncAuthorityCandidatePageRowsV2(ctx, tx, current.candidate.ProjectID, current.candidate.CandidateID, pageNumber, page, pageDigest, environmentCount, rolling, current.candidate.EnvironmentCount); err != nil {
		return err
	}
	state := "staging"
	var authorityDigest [32]byte
	var authorityDigestValue any
	if !page.More {
		authorityDigest, err = finalizeSyncAuthorityDigestV2(headerDigest, environmentCount, rolling)
		if err != nil {
			return syncProblem(SyncErrorInvalid, "page", "cannot finalize the authority candidate")
		}
		state = "ready"
		authorityDigestValue = authorityDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_authority_candidates
SET state = ?, page_count = ?, environment_count = ?, through_environment_id = ?,
    rolling_environment_digest = ?, authority_digest = ?
WHERE project_id = ? AND candidate_id = ? AND state = 'staging'
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ? AND authority_digest IS NULL`,
		state, pageNumber, environmentCount, page.ThroughEnvironmentID, rolling[:], authorityDigestValue,
		string(current.candidate.ProjectID), current.candidate.CandidateID[:], current.candidate.PageCount,
		current.candidate.EnvironmentCount, current.candidate.ThroughEnvironmentID, current.candidate.RollingEnvironmentDigest[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "checkpoint", "active authority candidate changed")
	}
	return nil
}

func insertSyncAuthorityCandidatePageRowsV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
	pageNumber int64,
	page SyncAuthorityPage,
	pageDigest [32]byte,
	resultingEnvironmentCount int64,
	resultingRolling [32]byte,
	priorEnvironmentCount int64,
) error {
	var after any
	if page.AfterEnvironmentID != "" {
		after = page.AfterEnvironmentID
	}
	more := 0
	if page.More {
		more = 1
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), candidateID[:], pageNumber, after, page.ThroughEnvironmentID,
		len(page.Environments), more, pageDigest[:], resultingEnvironmentCount, resultingRolling[:],
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return err
	}
	for index, environment := range page.Environments {
		ordinal, err := checkedSyncAuthorityCandidateAdvanceV2(priorEnvironmentCount, int64(index+1))
		if err != nil {
			return syncProblem(SyncErrorInvalid, "environment_count", "would overflow")
		}
		if err := insertSyncAuthorityCandidateEnvironmentV2(ctx, tx, projectID, candidateID, pageNumber, ordinal, environment); err != nil {
			return err
		}
	}
	return nil
}

func insertSyncAuthorityCandidateEnvironmentV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, pageNumber, ordinal int64, environment SyncEnvironmentCertificate) error {
	var retirementRelayGeneration, retirementMembershipGeneration, retirementFinalSequence, retirementFinalDigest, retirementID, retirementBytes any
	if environment.Retirement != nil {
		retirementRelayGeneration = environment.Retirement.RelayGeneration[:]
		retirementMembershipGeneration = environment.Retirement.MembershipGeneration
		retirementFinalSequence = environment.Retirement.FinalEnvironmentSequence
		retirementFinalDigest = environment.Retirement.FinalEnvelopeDigest[:]
		retirementID = environment.Retirement.RetirementID[:]
		retirementBytes = environment.Retirement.RetirementBytes
	}
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation, retirement_relay_generation,
  retirement_membership_generation, retirement_final_environment_sequence,
  retirement_final_envelope_digest, retirement_id, retirement_bytes
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID), candidateID[:], environment.EnvironmentID, ordinal, pageNumber,
		environment.CertificateID[:], environment.CertificateBytes, string(environment.Mode), environment.ExpiresAtMillis,
		environment.JoinMembershipGeneration, retirementRelayGeneration, retirementMembershipGeneration,
		retirementFinalSequence, retirementFinalDigest, retirementID, retirementBytes,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return err
	}
	if err := insertSyncAuthorityCandidateMembershipEventV2(ctx, tx, projectID, candidateID, environment.JoinMembershipGeneration, "join", environment.EnvironmentID); err != nil {
		return err
	}
	if environment.Retirement != nil {
		if err := insertSyncAuthorityCandidateMembershipEventV2(ctx, tx, projectID, candidateID, environment.Retirement.MembershipGeneration, "retirement", environment.EnvironmentID); err != nil {
			return err
		}
	}
	return nil
}

func insertSyncAuthorityCandidateMembershipEventV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, candidateID [32]byte, generation uint32, kind, environmentID string) error {
	result, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authority_candidate_membership_events(
  project_id, candidate_id, membership_generation, event_kind, environment_id
) VALUES(?, ?, ?, ?, ?)`, string(projectID), candidateID[:], generation, kind, environmentID)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return requireOneAffectedV1(result, ctx)
}
