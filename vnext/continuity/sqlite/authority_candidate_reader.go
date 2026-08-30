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

type persistedSyncAuthorityCandidatePageV2 struct {
	pageNumber                int64
	page                      SyncAuthorityPage
	environmentCount          int64
	pageDigest                [32]byte
	resultingEnvironmentCount int64
	resultingRollingDigest    [32]byte
}

type persistedSyncAuthorityCandidateV2 struct {
	candidate    SyncAuthorityCandidate
	state        string
	headerDigest [32]byte
}

type syncAuthorityMembershipEventV2 struct {
	kind          string
	environmentID string
}

const syncAuthorityCandidatePageByCursorSelectV2 = `
SELECT
  page_number, after_environment_id, through_environment_id,
  environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
FROM continuity_sync_authority_candidate_pages
WHERE project_id = ? AND candidate_id = ?`

const syncAuthorityCandidateFirstPageByCursorQueryV2 = syncAuthorityCandidatePageByCursorSelectV2 + `
  AND after_environment_id IS NULL`

const syncAuthorityCandidateSubsequentPageByCursorQueryV2 = syncAuthorityCandidatePageByCursorSelectV2 + `
  AND after_environment_id = ?`

func readActiveSyncAuthorityCandidateHeaderV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (persistedSyncAuthorityCandidateV2, bool, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  candidate_id, state, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head,
  base_authority_digest_version, base_authority_digest,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
FROM continuity_sync_authority_candidates
WHERE project_id = ? AND state IN ('staging', 'ready')
ORDER BY candidate_id`, string(projectID))
	if err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	var persisted persistedSyncAuthorityCandidateV2
	var candidateID, channelID, relayGeneration, adminPublicKey, baseDigest, rollingDigest, authorityDigest []byte
	var membershipGeneration, inventoryArrivalHead, pageCount, environmentCount, authorityDigestVersion int64
	var baseVersion sql.NullInt64
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return persistedSyncAuthorityCandidateV2{}, false, syncTransactionProblem(ctx)
		}
		return persistedSyncAuthorityCandidateV2{}, false, nil
	}
	if err := rows.Scan(
		&candidateID, &persisted.state, &channelID, &relayGeneration, &adminPublicKey,
		&membershipGeneration, &inventoryArrivalHead, &baseVersion, &baseDigest,
		&pageCount, &environmentCount, &persisted.candidate.ThroughEnvironmentID,
		&rollingDigest, &authorityDigestVersion, &authorityDigest,
	); err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, syncTransactionProblem(ctx)
	}
	if rows.Next() {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("more than one active candidate exists")
	}
	if err := rows.Err(); err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, syncTransactionProblem(ctx)
	}
	persisted.candidate.ProjectID = projectID
	persisted.candidate.PageCount = pageCount
	persisted.candidate.EnvironmentCount = environmentCount
	if authorityDigestVersion >= 0 && authorityDigestVersion <= math.MaxUint16 {
		persisted.candidate.AuthorityDigestVersion = uint16(authorityDigestVersion)
	}
	if len(candidateID) != sha256.Size || isZeroDigestBytesV2(candidateID) ||
		len(channelID) != len(persisted.candidate.Snapshot.ChannelID) || isZeroDigestBytesV2(channelID) ||
		len(relayGeneration) != len(persisted.candidate.Snapshot.RelayGeneration) || isZeroDigestBytesV2(relayGeneration) ||
		len(adminPublicKey) != len(persisted.candidate.Snapshot.AdminPublicKey) || isZeroDigestBytesV2(adminPublicKey) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 || inventoryArrivalHead < 0 ||
		pageCount < 1 || environmentCount < 1 || !validOpaqueID(persisted.candidate.ThroughEnvironmentID) ||
		len(rollingDigest) != sha256.Size || isZeroDigestBytesV2(rollingDigest) || authorityDigestVersion != 2 {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("active candidate header is malformed")
	}
	copy(persisted.candidate.CandidateID[:], candidateID)
	copy(persisted.candidate.Snapshot.ChannelID[:], channelID)
	copy(persisted.candidate.Snapshot.RelayGeneration[:], relayGeneration)
	copy(persisted.candidate.Snapshot.AdminPublicKey[:], adminPublicKey)
	copy(persisted.candidate.RollingEnvironmentDigest[:], rollingDigest)
	persisted.candidate.Snapshot.MembershipGeneration = uint32(membershipGeneration)
	persisted.candidate.Snapshot.InventoryArrivalHead = inventoryArrivalHead
	baseAbsent := !baseVersion.Valid && baseDigest == nil
	basePresent := baseVersion.Valid && (baseVersion.Int64 == 1 || baseVersion.Int64 == 2) && len(baseDigest) == sha256.Size && !isZeroDigestBytesV2(baseDigest)
	if !baseAbsent && !basePresent {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("active candidate base authority is malformed")
	}
	if basePresent {
		persisted.candidate.Snapshot.BaseAuthorityDigestVersion = uint16(baseVersion.Int64)
		copy(persisted.candidate.Snapshot.BaseAuthorityDigest[:], baseDigest)
	}
	switch persisted.state {
	case "staging":
		if authorityDigest != nil {
			return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("staging candidate has a final digest")
		}
	case "ready":
		if len(authorityDigest) != sha256.Size || isZeroDigestBytesV2(authorityDigest) {
			return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("ready candidate final digest is malformed")
		}
		persisted.candidate.Ready = true
		copy(persisted.candidate.AuthorityDigest[:], authorityDigest)
	default:
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("active candidate state is malformed")
	}
	candidateIDDerived, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, persisted.candidate.Snapshot)
	if err != nil || candidateIDDerived != persisted.candidate.CandidateID {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("active candidate identity is stale")
	}
	persisted.headerDigest = headerDigest
	lastPage, found, err := readAndValidateSyncAuthorityCandidatePageByNumberV2(ctx, tx, persisted, persisted.candidate.PageCount)
	if err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, err
	}
	if !found || lastPage.page.ThroughEnvironmentID != persisted.candidate.ThroughEnvironmentID ||
		lastPage.resultingEnvironmentCount != persisted.candidate.EnvironmentCount ||
		lastPage.resultingRollingDigest != persisted.candidate.RollingEnvironmentDigest ||
		lastPage.page.More == persisted.candidate.Ready {
		return persistedSyncAuthorityCandidateV2{}, false, corruptSyncAuthorityCandidateV2("active candidate checkpoint is stale")
	}
	return persisted, true, nil
}

func readAndValidateSyncAuthorityCandidatePageByAfterV2(ctx context.Context, tx *sql.Tx, current persistedSyncAuthorityCandidateV2, after string) (persistedSyncAuthorityCandidatePageV2, bool, error) {
	if after == "" {
		return readAndValidateSyncAuthorityCandidatePageQueryV2(
			ctx, tx, current, syncAuthorityCandidateFirstPageByCursorQueryV2,
			string(current.candidate.ProjectID), current.candidate.CandidateID[:],
		)
	}
	return readAndValidateSyncAuthorityCandidatePageQueryV2(
		ctx, tx, current, syncAuthorityCandidateSubsequentPageByCursorQueryV2,
		string(current.candidate.ProjectID), current.candidate.CandidateID[:], after,
	)
}

func readAndValidateSyncAuthorityCandidatePageByNumberV2(ctx context.Context, tx *sql.Tx, current persistedSyncAuthorityCandidateV2, pageNumber int64) (persistedSyncAuthorityCandidatePageV2, bool, error) {
	return readAndValidateSyncAuthorityCandidatePageQueryV2(ctx, tx, current, `
SELECT
  page_number, after_environment_id, through_environment_id,
  environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
FROM continuity_sync_authority_candidate_pages
WHERE project_id = ? AND candidate_id = ? AND page_number = ?`,
		string(current.candidate.ProjectID), current.candidate.CandidateID[:], pageNumber)
}

func readAndValidateSyncAuthorityCandidatePageQueryV2(ctx context.Context, tx *sql.Tx, current persistedSyncAuthorityCandidateV2, query string, arguments ...any) (persistedSyncAuthorityCandidatePageV2, bool, error) {
	var page persistedSyncAuthorityCandidatePageV2
	var after sql.NullString
	var more int64
	var pageDigest, resultingRolling []byte
	err := tx.QueryRowContext(ctx, query, arguments...).Scan(
		&page.pageNumber, &after, &page.page.ThroughEnvironmentID,
		&page.environmentCount, &more, &pageDigest,
		&page.resultingEnvironmentCount, &resultingRolling,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return persistedSyncAuthorityCandidatePageV2{}, false, nil
	}
	if err != nil {
		return persistedSyncAuthorityCandidatePageV2{}, false, syncTransactionProblem(ctx)
	}
	if after.Valid {
		page.page.AfterEnvironmentID = after.String
	}
	if more == 1 {
		page.page.More = true
	} else if more != 0 {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate page marker is malformed")
	}
	if page.pageNumber < 1 || page.pageNumber > current.candidate.PageCount || page.environmentCount < 1 ||
		page.environmentCount > maximumSyncAuthorityCandidatePageEnvironments || !validOpaqueID(page.page.ThroughEnvironmentID) ||
		len(pageDigest) != sha256.Size || isZeroDigestBytesV2(pageDigest) || len(resultingRolling) != sha256.Size || isZeroDigestBytesV2(resultingRolling) {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate page header is malformed")
	}
	copy(page.pageDigest[:], pageDigest)
	copy(page.resultingRollingDigest[:], resultingRolling)
	priorCount := int64(0)
	priorThrough := ""
	priorRolling, err := syncAuthorityCandidateRollingSeedV2(current.headerDigest)
	if err != nil {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate rolling seed cannot be derived")
	}
	if page.pageNumber > 1 {
		var previousMore int64
		var previousRolling []byte
		if err := tx.QueryRowContext(ctx, `
SELECT through_environment_id, resulting_environment_count, resulting_rolling_digest, more
FROM continuity_sync_authority_candidate_pages
WHERE project_id = ? AND candidate_id = ? AND page_number = ?`,
			string(current.candidate.ProjectID), current.candidate.CandidateID[:], page.pageNumber-1,
		).Scan(&priorThrough, &priorCount, &previousRolling, &previousMore); err != nil {
			return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate previous page is missing")
		}
		if !validOpaqueID(priorThrough) || priorCount < 1 || len(previousRolling) != sha256.Size || isZeroDigestBytesV2(previousRolling) || previousMore != 1 {
			return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate previous checkpoint is malformed")
		}
		copy(priorRolling[:], previousRolling)
	}
	if page.page.AfterEnvironmentID != priorThrough {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate page cursor is not contiguous")
	}
	expectedEvents := make(map[uint32]syncAuthorityMembershipEventV2, maximumSyncAuthorityCandidatePageEnvironments*2)
	environments, environmentDigests, rolling, err := readAndValidateSyncAuthorityCandidatePageEnvironmentsV2(
		ctx, tx, current, page, priorCount, priorRolling, priorThrough,
		make(map[[32]byte]struct{}, maximumSyncAuthorityCandidatePageEnvironments), expectedEvents,
	)
	if err != nil {
		return persistedSyncAuthorityCandidatePageV2{}, false, err
	}
	page.page.Environments = environments
	resultingCount, err := checkedSyncAuthorityCandidateAdvanceV2(priorCount, int64(len(environments)))
	if err != nil {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate environment count overflows")
	}
	derivedPageDigest, err := syncAuthorityCandidatePageDigestV2(current.candidate.CandidateID, page.pageNumber, page.page, resultingCount, rolling, environmentDigests)
	if err != nil || derivedPageDigest != page.pageDigest || page.resultingEnvironmentCount != resultingCount || page.resultingRollingDigest != rolling {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate page result is stale")
	}
	if err := readAndValidateBoundedSyncAuthorityCandidateEventsV2(ctx, tx, current, environments, expectedEvents); err != nil {
		return persistedSyncAuthorityCandidatePageV2{}, false, err
	}
	if page.pageNumber < current.candidate.PageCount && !page.page.More {
		return persistedSyncAuthorityCandidatePageV2{}, false, corruptSyncAuthorityCandidateV2("candidate has an early final page")
	}
	return page, true, nil
}

func readAndValidateBoundedSyncAuthorityCandidateEventsV2(ctx context.Context, tx *sql.Tx, current persistedSyncAuthorityCandidateV2, environments []SyncEnvironmentCertificate, expected map[uint32]syncAuthorityMembershipEventV2) error {
	for _, environment := range environments {
		rows, err := tx.QueryContext(ctx, `
SELECT membership_generation, event_kind
FROM continuity_sync_authority_candidate_membership_events
WHERE project_id = ? AND candidate_id = ? AND environment_id = ?
ORDER BY membership_generation`, string(current.candidate.ProjectID), current.candidate.CandidateID[:], environment.EnvironmentID)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		observed := 0
		for rows.Next() {
			var generation int64
			var kind string
			if err := rows.Scan(&generation, &kind); err != nil {
				rows.Close()
				return syncTransactionProblem(ctx)
			}
			if generation < 1 || generation > math.MaxUint32 || expected[uint32(generation)] != (syncAuthorityMembershipEventV2{kind: kind, environmentID: environment.EnvironmentID}) {
				rows.Close()
				return corruptSyncAuthorityCandidateV2("candidate membership event does not match its environment")
			}
			observed++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return syncTransactionProblem(ctx)
		}
		if err := rows.Close(); err != nil {
			return syncTransactionProblem(ctx)
		}
		want := 1
		if environment.Retirement != nil {
			want = 2
		}
		if observed != want {
			return corruptSyncAuthorityCandidateV2("candidate membership events are incomplete")
		}
	}
	return nil
}

func isZeroDigestBytesV2(value []byte) bool {
	return len(value) == sha256.Size && bytes.Equal(value, make([]byte, sha256.Size))
}

func readAndValidateActiveSyncAuthorityCandidateV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (persistedSyncAuthorityCandidateV2, bool, error) {
	persisted, found, err := readActiveSyncAuthorityCandidateHeaderV2(ctx, tx, projectID)
	if err != nil || !found {
		return persistedSyncAuthorityCandidateV2{}, found, err
	}
	if err := streamAndValidateSyncAuthorityCandidateV2(ctx, tx, persisted); err != nil {
		return persistedSyncAuthorityCandidateV2{}, false, err
	}
	return persisted, true, nil
}

func streamAndValidateSyncAuthorityCandidateV2(ctx context.Context, tx *sql.Tx, persisted persistedSyncAuthorityCandidateV2) error {
	var pageRows, environmentRows int64
	if err := tx.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*)
   FROM continuity_sync_authority_candidate_pages
   WHERE project_id = ? AND candidate_id = ?),
  (SELECT COUNT(*)
   FROM continuity_sync_authority_candidate_environments
   WHERE project_id = ? AND candidate_id = ?)`,
		string(persisted.candidate.ProjectID), persisted.candidate.CandidateID[:],
		string(persisted.candidate.ProjectID), persisted.candidate.CandidateID[:],
	).Scan(&pageRows, &environmentRows); err != nil {
		return syncTransactionProblem(ctx)
	}
	if pageRows != persisted.candidate.PageCount || environmentRows != persisted.candidate.EnvironmentCount {
		return corruptSyncAuthorityCandidateV2("candidate child row counts are stale")
	}
	rows, err := tx.QueryContext(ctx, `
SELECT
  p.page_number, p.after_environment_id, p.through_environment_id,
  p.environment_count, p.more, p.page_digest,
  p.resulting_environment_count, p.resulting_rolling_digest,
  e.environment_id, e.environment_ordinal, e.page_number,
  e.certificate_id, e.certificate_bytes, e.mode, e.expires_at_millis,
  e.join_membership_generation, e.retirement_relay_generation,
  e.retirement_membership_generation, e.retirement_final_environment_sequence,
  e.retirement_final_envelope_digest, e.retirement_id, e.retirement_bytes
FROM continuity_sync_authority_candidate_pages AS p
JOIN continuity_sync_authority_candidate_environments AS e
  ON e.project_id = p.project_id
 AND e.candidate_id = p.candidate_id
 AND e.page_number = p.page_number
WHERE p.project_id = ? AND p.candidate_id = ?
ORDER BY p.page_number, e.environment_ordinal`, string(persisted.candidate.ProjectID), persisted.candidate.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()
	rolling, err := syncAuthorityCandidateRollingSeedV2(persisted.headerDigest)
	if err != nil {
		return corruptSyncAuthorityCandidateV2("candidate rolling seed cannot be derived")
	}
	var activePage persistedSyncAuthorityCandidatePageV2
	var pageEnvironments []SyncEnvironmentCertificate
	var pageEnvironmentDigests [][32]byte
	var cumulativeEnvironmentCount, observedPages int64
	previousEnvironmentID := ""
	finalizePage := func() error {
		if activePage.pageNumber == 0 {
			return nil
		}
		if int64(len(pageEnvironments)) != activePage.environmentCount || len(pageEnvironments) < 1 ||
			len(pageEnvironments) > maximumSyncAuthorityCandidatePageEnvironments ||
			pageEnvironments[len(pageEnvironments)-1].EnvironmentID != activePage.page.ThroughEnvironmentID {
			return corruptSyncAuthorityCandidateV2("candidate page children are incomplete")
		}
		var advanceErr error
		cumulativeEnvironmentCount, advanceErr = checkedSyncAuthorityCandidateAdvanceV2(cumulativeEnvironmentCount, int64(len(pageEnvironments)))
		if advanceErr != nil {
			return corruptSyncAuthorityCandidateV2("candidate environment count overflows")
		}
		activePage.page.Environments = pageEnvironments
		pageDigest, digestErr := syncAuthorityCandidatePageDigestV2(
			persisted.candidate.CandidateID, activePage.pageNumber, activePage.page,
			cumulativeEnvironmentCount, rolling, pageEnvironmentDigests,
		)
		if digestErr != nil || pageDigest != activePage.pageDigest || activePage.resultingEnvironmentCount != cumulativeEnvironmentCount ||
			activePage.resultingRollingDigest != rolling {
			return corruptSyncAuthorityCandidateV2("candidate page result is stale")
		}
		previousEnvironmentID = activePage.page.ThroughEnvironmentID
		observedPages++
		return nil
	}
	for rows.Next() {
		page, environment, ordinal, environmentPageNumber, err := scanSyncAuthorityCandidateJoinedEnvironmentV2(rows)
		if err != nil {
			return err
		}
		if activePage.pageNumber == 0 || page.pageNumber != activePage.pageNumber {
			if err := finalizePage(); err != nil {
				return err
			}
			expectedPageNumber, advanceErr := checkedSyncAuthorityCandidateAdvanceV2(observedPages, 1)
			if advanceErr != nil || page.pageNumber != expectedPageNumber || page.environmentCount < 1 ||
				page.environmentCount > maximumSyncAuthorityCandidatePageEnvironments || !validOpaqueID(page.page.ThroughEnvironmentID) {
				return corruptSyncAuthorityCandidateV2("candidate page sequence is malformed")
			}
			if page.page.AfterEnvironmentID != previousEnvironmentID {
				return corruptSyncAuthorityCandidateV2("candidate pages are not cursor-contiguous")
			}
			if page.pageNumber < persisted.candidate.PageCount && !page.page.More {
				return corruptSyncAuthorityCandidateV2("candidate has an early final page")
			}
			if page.pageNumber == persisted.candidate.PageCount && page.page.More == persisted.candidate.Ready {
				return corruptSyncAuthorityCandidateV2("candidate state and final page disagree")
			}
			activePage = page
			pageEnvironments = make([]SyncEnvironmentCertificate, 0, maximumSyncAuthorityCandidatePageEnvironments)
			pageEnvironmentDigests = make([][32]byte, 0, maximumSyncAuthorityCandidatePageEnvironments)
		} else if !samePersistedSyncAuthorityCandidatePageHeaderV2(activePage, page) {
			return corruptSyncAuthorityCandidateV2("candidate page header changes between children")
		}
		expectedOrdinal, advanceErr := checkedSyncAuthorityCandidateAdvanceV2(cumulativeEnvironmentCount, int64(len(pageEnvironments)+1))
		if advanceErr != nil || ordinal != expectedOrdinal || environmentPageNumber != activePage.pageNumber || environment.EnvironmentID <= previousEnvironmentID {
			return corruptSyncAuthorityCandidateV2("candidate environment ordering is malformed")
		}
		if len(pageEnvironments) > 0 && environment.EnvironmentID <= pageEnvironments[len(pageEnvironments)-1].EnvironmentID {
			return corruptSyncAuthorityCandidateV2("candidate page environments are not strictly sorted")
		}
		if err := validateSyncAuthorityCandidateEnvironmentV2(environment, len(pageEnvironments)); err != nil ||
			environment.JoinMembershipGeneration > persisted.candidate.Snapshot.MembershipGeneration ||
			(environment.Retirement != nil && (environment.Retirement.RelayGeneration != persisted.candidate.Snapshot.RelayGeneration ||
				environment.Retirement.MembershipGeneration > persisted.candidate.Snapshot.MembershipGeneration)) {
			return corruptSyncAuthorityCandidateV2("candidate environment row is malformed")
		}
		var environmentDigest [32]byte
		rolling, environmentDigest, err = advanceSyncAuthorityCandidateRollingV2(persisted.headerDigest, rolling, ordinal, environment)
		if err != nil {
			return corruptSyncAuthorityCandidateV2("candidate environment digest cannot be derived")
		}
		pageEnvironments = append(pageEnvironments, environment)
		pageEnvironmentDigests = append(pageEnvironmentDigests, environmentDigest)
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := finalizePage(); err != nil {
		return err
	}
	if observedPages != persisted.candidate.PageCount || cumulativeEnvironmentCount != persisted.candidate.EnvironmentCount ||
		previousEnvironmentID != persisted.candidate.ThroughEnvironmentID || rolling != persisted.candidate.RollingEnvironmentDigest {
		return corruptSyncAuthorityCandidateV2("candidate accumulator is stale")
	}
	if err := streamAndValidateSyncAuthorityCandidateEventsV2(ctx, tx, persisted); err != nil {
		return err
	}
	if persisted.candidate.Ready {
		authorityDigest, err := finalizeSyncAuthorityDigestV2(persisted.headerDigest, cumulativeEnvironmentCount, rolling)
		if err != nil || authorityDigest != persisted.candidate.AuthorityDigest {
			return corruptSyncAuthorityCandidateV2("ready candidate authority digest is stale")
		}
	}
	return nil
}

func scanSyncAuthorityCandidateJoinedEnvironmentV2(scanner interface{ Scan(dest ...any) error }) (persistedSyncAuthorityCandidatePageV2, SyncEnvironmentCertificate, int64, int64, error) {
	var page persistedSyncAuthorityCandidatePageV2
	var environment SyncEnvironmentCertificate
	var after sql.NullString
	var more int64
	var pageDigest, resultingRolling []byte
	var ordinal, environmentPageNumber, expiresAtMillis, joinMembershipGeneration int64
	var certificateID, certificateBytes, retirementRelayGeneration, retirementFinalDigest, retirementID, retirementBytes []byte
	var retirementMembershipGeneration, retirementFinalSequence sql.NullInt64
	var mode string
	if err := scanner.Scan(
		&page.pageNumber, &after, &page.page.ThroughEnvironmentID,
		&page.environmentCount, &more, &pageDigest,
		&page.resultingEnvironmentCount, &resultingRolling,
		&environment.EnvironmentID, &ordinal, &environmentPageNumber,
		&certificateID, &certificateBytes, &mode, &expiresAtMillis,
		&joinMembershipGeneration, &retirementRelayGeneration,
		&retirementMembershipGeneration, &retirementFinalSequence,
		&retirementFinalDigest, &retirementID, &retirementBytes,
	); err != nil {
		return persistedSyncAuthorityCandidatePageV2{}, SyncEnvironmentCertificate{}, 0, 0, syncTransactionProblem(nil)
	}
	if after.Valid {
		page.page.AfterEnvironmentID = after.String
	}
	if more == 1 {
		page.page.More = true
	} else if more != 0 {
		return persistedSyncAuthorityCandidatePageV2{}, SyncEnvironmentCertificate{}, 0, 0, corruptSyncAuthorityCandidateV2("candidate page marker is malformed")
	}
	if len(pageDigest) != sha256.Size || isZeroDigestBytesV2(pageDigest) || len(resultingRolling) != sha256.Size || isZeroDigestBytesV2(resultingRolling) ||
		ordinal < 1 || environmentPageNumber < 1 || len(certificateID) != sha256.Size || isZeroDigestBytesV2(certificateID) ||
		len(certificateBytes) < 1 || len(certificateBytes) > maximumEnvironmentCertificateBytes || expiresAtMillis < 0 ||
		joinMembershipGeneration < 1 || joinMembershipGeneration > math.MaxUint32 {
		return persistedSyncAuthorityCandidatePageV2{}, SyncEnvironmentCertificate{}, 0, 0, corruptSyncAuthorityCandidateV2("candidate joined page row is malformed")
	}
	copy(page.pageDigest[:], pageDigest)
	copy(page.resultingRollingDigest[:], resultingRolling)
	copy(environment.CertificateID[:], certificateID)
	environment.CertificateBytes = append([]byte(nil), certificateBytes...)
	environment.Mode = SyncEnvironmentMode(mode)
	environment.ExpiresAtMillis = expiresAtMillis
	environment.JoinMembershipGeneration = uint32(joinMembershipGeneration)
	retirementPresent := retirementRelayGeneration != nil || retirementMembershipGeneration.Valid || retirementFinalSequence.Valid ||
		retirementFinalDigest != nil || retirementID != nil || retirementBytes != nil
	if retirementPresent {
		if len(retirementRelayGeneration) != sha256.Size || isZeroDigestBytesV2(retirementRelayGeneration) ||
			!retirementMembershipGeneration.Valid || retirementMembershipGeneration.Int64 < 1 || retirementMembershipGeneration.Int64 > math.MaxUint32 ||
			!retirementFinalSequence.Valid || retirementFinalSequence.Int64 < 0 || len(retirementFinalDigest) != sha256.Size ||
			len(retirementID) != sha256.Size || isZeroDigestBytesV2(retirementID) || len(retirementBytes) < 1 ||
			len(retirementBytes) > maximumEnvironmentRetirementBytes ||
			(retirementFinalSequence.Int64 == 0) != isZeroDigestBytesV2(retirementFinalDigest) {
			return persistedSyncAuthorityCandidatePageV2{}, SyncEnvironmentCertificate{}, 0, 0, corruptSyncAuthorityCandidateV2("candidate joined retirement row is malformed")
		}
		retirement := &SyncEnvironmentRetirement{
			MembershipGeneration:     uint32(retirementMembershipGeneration.Int64),
			FinalEnvironmentSequence: retirementFinalSequence.Int64,
			RetirementBytes:          append([]byte(nil), retirementBytes...),
		}
		copy(retirement.RelayGeneration[:], retirementRelayGeneration)
		copy(retirement.FinalEnvelopeDigest[:], retirementFinalDigest)
		copy(retirement.RetirementID[:], retirementID)
		environment.Retirement = retirement
	}
	return page, environment, ordinal, environmentPageNumber, nil
}

func samePersistedSyncAuthorityCandidatePageHeaderV2(left, right persistedSyncAuthorityCandidatePageV2) bool {
	return left.pageNumber == right.pageNumber && left.page.AfterEnvironmentID == right.page.AfterEnvironmentID &&
		left.page.ThroughEnvironmentID == right.page.ThroughEnvironmentID && left.page.More == right.page.More &&
		left.environmentCount == right.environmentCount && left.pageDigest == right.pageDigest &&
		left.resultingEnvironmentCount == right.resultingEnvironmentCount && left.resultingRollingDigest == right.resultingRollingDigest
}

func streamAndValidateSyncAuthorityCandidateEventsV2(ctx context.Context, tx *sql.Tx, persisted persistedSyncAuthorityCandidateV2) error {
	rows, err := tx.QueryContext(ctx, `
SELECT
  event.membership_generation, event.event_kind, event.environment_id,
  environment.join_membership_generation,
  environment.retirement_membership_generation
FROM continuity_sync_authority_candidate_membership_events AS event
LEFT JOIN continuity_sync_authority_candidate_environments AS environment
  ON environment.project_id = event.project_id
 AND environment.candidate_id = event.candidate_id
 AND environment.environment_id = event.environment_id
WHERE event.project_id = ? AND event.candidate_id = ?
ORDER BY event.membership_generation`, string(persisted.candidate.ProjectID), persisted.candidate.CandidateID[:])
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer rows.Close()
	var observedEvents, previousGeneration int64
	for rows.Next() {
		var generation int64
		var kind, environmentID string
		var joinGeneration, retirementGeneration sql.NullInt64
		if err := rows.Scan(&generation, &kind, &environmentID, &joinGeneration, &retirementGeneration); err != nil {
			return syncTransactionProblem(ctx)
		}
		if generation < 1 || generation > math.MaxUint32 || generation <= previousGeneration || !validOpaqueID(environmentID) || !joinGeneration.Valid {
			return corruptSyncAuthorityCandidateV2("candidate membership event is malformed")
		}
		valid := kind == "join" && generation == joinGeneration.Int64
		if kind == "retirement" {
			valid = retirementGeneration.Valid && generation == retirementGeneration.Int64
		}
		if !valid {
			return corruptSyncAuthorityCandidateV2("candidate membership event does not match its environment")
		}
		observedEvents++
		if persisted.candidate.Ready && generation != observedEvents {
			return corruptSyncAuthorityCandidateV2("ready candidate membership coverage has a gap")
		}
		previousGeneration = generation
	}
	if err := rows.Err(); err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	var expectedEvents int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*) + COALESCE(SUM(
  CASE WHEN retirement_membership_generation IS NULL THEN 0 ELSE 1 END
), 0)
FROM continuity_sync_authority_candidate_environments
WHERE project_id = ? AND candidate_id = ?`, string(persisted.candidate.ProjectID), persisted.candidate.CandidateID[:]).Scan(&expectedEvents); err != nil {
		return syncTransactionProblem(ctx)
	}
	if observedEvents != expectedEvents {
		return corruptSyncAuthorityCandidateV2("candidate membership events are incomplete")
	}
	if persisted.candidate.Ready && (observedEvents != int64(persisted.candidate.Snapshot.MembershipGeneration) || previousGeneration != observedEvents) {
		return corruptSyncAuthorityCandidateV2("ready candidate membership coverage is incomplete")
	}
	return nil
}

func readAndValidateSyncAuthorityCandidatePageEnvironmentsV2(
	ctx context.Context,
	tx *sql.Tx,
	persisted persistedSyncAuthorityCandidateV2,
	page persistedSyncAuthorityCandidatePageV2,
	priorEnvironmentCount int64,
	priorRolling [32]byte,
	priorEnvironmentID string,
	seenCertificateIDs map[[32]byte]struct{},
	expectedEvents map[uint32]syncAuthorityMembershipEventV2,
) ([]SyncEnvironmentCertificate, [][32]byte, [32]byte, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT
  environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation, retirement_relay_generation,
  retirement_membership_generation, retirement_final_environment_sequence,
  retirement_final_envelope_digest, retirement_id, retirement_bytes
FROM continuity_sync_authority_candidate_environments
WHERE project_id = ? AND candidate_id = ? AND page_number = ?
ORDER BY environment_ordinal`, string(persisted.candidate.ProjectID), persisted.candidate.CandidateID[:], page.pageNumber)
	if err != nil {
		return nil, nil, [32]byte{}, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	environments := make([]SyncEnvironmentCertificate, 0, maximumSyncAuthorityCandidatePageEnvironments)
	environmentDigests := make([][32]byte, 0, maximumSyncAuthorityCandidatePageEnvironments)
	rolling := priorRolling
	previousEnvironmentID := priorEnvironmentID
	for rows.Next() {
		environment, ordinal, pageNumber, err := scanSyncAuthorityCandidateEnvironmentV2(rows)
		if err != nil {
			return nil, nil, [32]byte{}, err
		}
		expectedOrdinal, advanceErr := checkedSyncAuthorityCandidateAdvanceV2(priorEnvironmentCount, int64(len(environments)+1))
		if advanceErr != nil || ordinal != expectedOrdinal || pageNumber != page.pageNumber || environment.EnvironmentID <= previousEnvironmentID {
			return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate environment ordering is malformed")
		}
		if err := validateSyncAuthorityCandidateEnvironmentV2(environment, len(environments)); err != nil ||
			environment.JoinMembershipGeneration > persisted.candidate.Snapshot.MembershipGeneration ||
			(environment.Retirement != nil && (environment.Retirement.RelayGeneration != persisted.candidate.Snapshot.RelayGeneration ||
				environment.Retirement.MembershipGeneration > persisted.candidate.Snapshot.MembershipGeneration)) {
			return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate environment row is malformed")
		}
		if _, duplicate := seenCertificateIDs[environment.CertificateID]; duplicate {
			return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate certificate identity is duplicated")
		}
		seenCertificateIDs[environment.CertificateID] = struct{}{}
		if _, duplicate := expectedEvents[environment.JoinMembershipGeneration]; duplicate {
			return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate membership generation is duplicated")
		}
		expectedEvents[environment.JoinMembershipGeneration] = syncAuthorityMembershipEventV2{kind: "join", environmentID: environment.EnvironmentID}
		if environment.Retirement != nil {
			if _, duplicate := expectedEvents[environment.Retirement.MembershipGeneration]; duplicate {
				return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate membership generation is duplicated")
			}
			expectedEvents[environment.Retirement.MembershipGeneration] = syncAuthorityMembershipEventV2{kind: "retirement", environmentID: environment.EnvironmentID}
		}
		var environmentDigest [32]byte
		rolling, environmentDigest, err = advanceSyncAuthorityCandidateRollingV2(persisted.headerDigest, rolling, ordinal, environment)
		if err != nil {
			return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate environment digest cannot be derived")
		}
		environments = append(environments, environment)
		environmentDigests = append(environmentDigests, environmentDigest)
		previousEnvironmentID = environment.EnvironmentID
	}
	if err := rows.Err(); err != nil {
		return nil, nil, [32]byte{}, syncTransactionProblem(ctx)
	}
	if int64(len(environments)) != page.environmentCount || len(environments) < 1 || len(environments) > maximumSyncAuthorityCandidatePageEnvironments ||
		previousEnvironmentID != page.page.ThroughEnvironmentID {
		return nil, nil, [32]byte{}, corruptSyncAuthorityCandidateV2("candidate page children are incomplete")
	}
	return environments, environmentDigests, rolling, nil
}

func scanSyncAuthorityCandidateEnvironmentV2(scanner interface{ Scan(dest ...any) error }) (SyncEnvironmentCertificate, int64, int64, error) {
	var environment SyncEnvironmentCertificate
	var ordinal, pageNumber, expiresAtMillis, joinMembershipGeneration int64
	var certificateID, certificateBytes, retirementRelayGeneration, retirementFinalDigest, retirementID, retirementBytes []byte
	var retirementMembershipGeneration, retirementFinalSequence sql.NullInt64
	var mode string
	if err := scanner.Scan(
		&environment.EnvironmentID, &ordinal, &pageNumber,
		&certificateID, &certificateBytes, &mode, &expiresAtMillis,
		&joinMembershipGeneration, &retirementRelayGeneration,
		&retirementMembershipGeneration, &retirementFinalSequence,
		&retirementFinalDigest, &retirementID, &retirementBytes,
	); err != nil {
		return SyncEnvironmentCertificate{}, 0, 0, syncTransactionProblem(nil)
	}
	if ordinal < 1 || pageNumber < 1 || len(certificateID) != sha256.Size || bytes.Equal(certificateID, make([]byte, sha256.Size)) ||
		len(certificateBytes) < 1 || len(certificateBytes) > maximumEnvironmentCertificateBytes || expiresAtMillis < 0 ||
		joinMembershipGeneration < 1 || joinMembershipGeneration > math.MaxUint32 {
		return SyncEnvironmentCertificate{}, 0, 0, corruptSyncAuthorityCandidateV2("candidate environment scalar is malformed")
	}
	copy(environment.CertificateID[:], certificateID)
	environment.CertificateBytes = append([]byte(nil), certificateBytes...)
	environment.Mode = SyncEnvironmentMode(mode)
	environment.ExpiresAtMillis = expiresAtMillis
	environment.JoinMembershipGeneration = uint32(joinMembershipGeneration)
	retirementPresent := retirementRelayGeneration != nil || retirementMembershipGeneration.Valid || retirementFinalSequence.Valid ||
		retirementFinalDigest != nil || retirementID != nil || retirementBytes != nil
	if retirementPresent {
		if len(retirementRelayGeneration) != sha256.Size || bytes.Equal(retirementRelayGeneration, make([]byte, sha256.Size)) ||
			!retirementMembershipGeneration.Valid || retirementMembershipGeneration.Int64 < 1 || retirementMembershipGeneration.Int64 > math.MaxUint32 ||
			!retirementFinalSequence.Valid || retirementFinalSequence.Int64 < 0 || len(retirementFinalDigest) != sha256.Size ||
			len(retirementID) != sha256.Size || bytes.Equal(retirementID, make([]byte, sha256.Size)) ||
			len(retirementBytes) < 1 || len(retirementBytes) > maximumEnvironmentRetirementBytes ||
			(retirementFinalSequence.Int64 == 0) != bytes.Equal(retirementFinalDigest, make([]byte, sha256.Size)) {
			return SyncEnvironmentCertificate{}, 0, 0, corruptSyncAuthorityCandidateV2("candidate retirement row is malformed")
		}
		retirement := &SyncEnvironmentRetirement{
			MembershipGeneration:     uint32(retirementMembershipGeneration.Int64),
			FinalEnvironmentSequence: retirementFinalSequence.Int64,
			RetirementBytes:          append([]byte(nil), retirementBytes...),
		}
		copy(retirement.RelayGeneration[:], retirementRelayGeneration)
		copy(retirement.FinalEnvelopeDigest[:], retirementFinalDigest)
		copy(retirement.RetirementID[:], retirementID)
		environment.Retirement = retirement
	}
	return environment, ordinal, pageNumber, nil
}
