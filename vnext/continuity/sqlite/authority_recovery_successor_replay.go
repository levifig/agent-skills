package sqlite

import (
	"context"
	"database/sql"
)

// exactSyncAuthorityRecoverySuccessorPageReplayV1 recognizes an exact replay
// only when the replayed page and the checkpoint immediately before that page
// both match. The existing page replay check audits the page itself; this
// additional check binds an append retry to the compare-and-swap checkpoint
// that authorized the original append.
//
// The returned booleans retain the generic candidate replay contract: replayed
// reports that a persisted page uses the requested cursor, while exact reports
// that both its contents and its prior checkpoint match the request. A
// malformed predecessor page is corruption, even when the requested page is
// otherwise an exact replay.
func exactSyncAuthorityRecoverySuccessorPageReplayV1(
	ctx context.Context,
	tx *sql.Tx,
	current persistedSyncAuthorityCandidateV2,
	expectedSuccessor SyncAuthorityCandidateCheckpoint,
	candidatePage SyncAuthorityPage,
) (bool, bool, error) {
	replayed, exact, err := exactSyncAuthorityCandidatePageReplayV2(ctx, tx, current, candidatePage)
	if err != nil || !replayed || !exact {
		return replayed, exact, err
	}

	persistedPage, found, err := readAndValidateSyncAuthorityCandidatePageByAfterV2(
		ctx, tx, current, candidatePage.AfterEnvironmentID,
	)
	if err != nil {
		return true, false, err
	}
	if !found {
		return true, false, corruptSyncAuthorityCandidateV2("candidate replay page disappeared during audit")
	}
	prior, err := syncAuthorityRecoverySuccessorPriorCheckpointV1(ctx, tx, current, persistedPage)
	if err != nil {
		return true, false, err
	}
	if !syncAuthorityRecoverySuccessorCheckpointEqualV1(prior, expectedSuccessor) {
		return true, false, nil
	}
	return true, true, nil
}

func syncAuthorityRecoverySuccessorPriorCheckpointV1(
	ctx context.Context,
	tx *sql.Tx,
	current persistedSyncAuthorityCandidateV2,
	persistedPage persistedSyncAuthorityCandidatePageV2,
) (SyncAuthorityCandidateCheckpoint, error) {
	if persistedPage.pageNumber < 1 || persistedPage.pageNumber > current.candidate.PageCount {
		return SyncAuthorityCandidateCheckpoint{}, corruptSyncAuthorityCandidateV2("candidate replay page number is malformed")
	}

	prior := SyncAuthorityCandidateCheckpoint{
		CandidateID:          current.candidate.CandidateID,
		PageCount:            0,
		EnvironmentCount:     0,
		ThroughEnvironmentID: "",
		Ready:                false,
		AuthorityDigest:      [32]byte{},
	}
	if persistedPage.pageNumber == 1 {
		rolling, err := syncAuthorityCandidateRollingSeedV2(current.headerDigest)
		if err != nil {
			return SyncAuthorityCandidateCheckpoint{}, corruptSyncAuthorityCandidateV2("candidate rolling seed cannot be derived")
		}
		prior.RollingEnvironmentDigest = rolling
		return prior, nil
	}

	previous, found, err := readAndValidateSyncAuthorityCandidatePageByNumberV2(
		ctx, tx, current, persistedPage.pageNumber-1,
	)
	if err != nil {
		return SyncAuthorityCandidateCheckpoint{}, err
	}
	if !found {
		return SyncAuthorityCandidateCheckpoint{}, corruptSyncAuthorityCandidateV2("candidate previous page is missing")
	}
	if previous.pageNumber != persistedPage.pageNumber-1 || !previous.page.More ||
		previous.resultingEnvironmentCount < 1 || previous.resultingRollingDigest == ([32]byte{}) ||
		!validOpaqueID(previous.page.ThroughEnvironmentID) {
		return SyncAuthorityCandidateCheckpoint{}, corruptSyncAuthorityCandidateV2("candidate previous checkpoint is malformed")
	}
	prior.PageCount = previous.pageNumber
	prior.EnvironmentCount = previous.resultingEnvironmentCount
	prior.ThroughEnvironmentID = previous.page.ThroughEnvironmentID
	prior.RollingEnvironmentDigest = previous.resultingRollingDigest
	return prior, nil
}

func syncAuthorityRecoverySuccessorCheckpointEqualV1(
	left SyncAuthorityCandidateCheckpoint,
	right SyncAuthorityCandidateCheckpoint,
) bool {
	return left.CandidateID == right.CandidateID &&
		left.PageCount == right.PageCount &&
		left.EnvironmentCount == right.EnvironmentCount &&
		left.ThroughEnvironmentID == right.ThroughEnvironmentID &&
		left.RollingEnvironmentDigest == right.RollingEnvironmentDigest &&
		left.Ready == right.Ready &&
		left.AuthorityDigest == right.AuthorityDigest
}
