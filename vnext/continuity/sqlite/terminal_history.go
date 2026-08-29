package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

// PendingSyncFramesAfter returns one bounded opaque inbox page after an
// explicit staged arrival. It lets a caller restart verified-history
// enumeration without advancing or otherwise mutating local sync state.
func (store *Store) PendingSyncFramesAfter(ctx context.Context, projectID continuity.ProjectID, afterArrivalSequence int64, limit int) ([]OpaqueSyncFrame, error) {
	if err := projectID.Validate(); err != nil {
		return nil, syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	if afterArrivalSequence < 0 {
		return nil, syncProblem(SyncErrorInvalid, "after_arrival_sequence", "must be nonnegative")
	}
	if limit < 1 || limit > maximumSyncPageFrames {
		return nil, syncProblem(SyncErrorInvalid, "limit", "must be between 1 and 256")
	}
	if store == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return nil, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	if afterArrivalSequence < progress.AppliedCursor || afterArrivalSequence > progress.DownloadedCursor {
		return nil, syncProblem(SyncErrorCursor, "after_arrival_sequence", "is outside the retained staged range")
	}
	frames, err := pendingSyncFramesAfterV1(ctx, tx, projectID, afterArrivalSequence, progress.DownloadedCursor, limit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return frames, nil
}

func pendingSyncFramesAfterV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, afterArrivalSequence, downloadedCursor int64, limit int) ([]OpaqueSyncFrame, error) {
	var rowsBeyondDownloadedCursor int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_inbox
  WHERE project_id = ? AND arrival_sequence > ?
)`, string(projectID), downloadedCursor).Scan(&rowsBeyondDownloadedCursor); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	if rowsBeyondDownloadedCursor != 0 {
		return nil, syncProblem(SyncErrorStore, "", "staged inbox outruns the downloaded cursor")
	}
	rows, err := tx.QueryContext(ctx, `
SELECT arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
FROM continuity_sync_inbox
WHERE project_id = ? AND arrival_sequence > ? AND arrival_sequence <= ?
ORDER BY arrival_sequence
LIMIT ?`, string(projectID), afterArrivalSequence, downloadedCursor, limit)
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	frames := make([]OpaqueSyncFrame, 0, limit)
	for rows.Next() {
		var arrivalSequence int64
		var digest []byte
		var frameBytes []byte
		var frameKind, state string
		if err := rows.Scan(&arrivalSequence, &digest, &frameKind, &frameBytes, &state); err != nil {
			rows.Close()
			return nil, syncTransactionProblem(ctx)
		}
		expectedArrivalSequence := afterArrivalSequence + int64(len(frames)) + 1
		frame, err := opaqueSyncFrameFromColumnsV1(arrivalSequence, digest, frameKind, frameBytes, state)
		if err != nil || frame.ArrivalSequence != expectedArrivalSequence {
			rows.Close()
			return nil, syncProblem(SyncErrorStore, "", "staged inbox is inconsistent")
		}
		frames = append(frames, frame)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	expectedCount := downloadedCursor - afterArrivalSequence
	if expectedCount > int64(limit) {
		expectedCount = int64(limit)
	}
	if int64(len(frames)) != expectedCount {
		return nil, syncProblem(SyncErrorStore, "", "downloaded cursor outruns the staged inbox")
	}
	return frames, nil
}

func validateOrdinarySyncFrameAuthorityV1(authority SyncAuthority, frames []preparedVerifiedSyncFrame, isFirstSeenEnvelope []bool, trustedNowMillis int64) error {
	if len(frames) != len(isFirstSeenEnvelope) {
		return syncProblem(SyncErrorStore, "", "terminal-history classification is inconsistent")
	}
	for index, frame := range frames {
		environment, found := pinnedOrdinarySyncEnvironmentV1(authority, frame.fact.environmentID)
		if !found {
			return syncProblem(SyncErrorCertificate, "certificate_id", "environment is not in pinned authority")
		}
		if frame.certificateID != environment.CertificateID {
			return syncProblem(SyncErrorCertificate, "certificate_id", "does not match pinned authority")
		}
		if isFirstSeenEnvelope[index] && ordinarySyncEnvironmentRequiresTerminalHistoryV1(environment, trustedNowMillis) {
			return syncProblem(SyncErrorTerminalHistoryRequired, "", "")
		}
	}
	return nil
}

func pinnedOrdinarySyncEnvironmentV1(authority SyncAuthority, environmentID continuity.EnvironmentID) (SyncEnvironmentCertificate, bool) {
	for _, environment := range authority.Environments {
		if environment.EnvironmentID == string(environmentID) {
			return environment, true
		}
	}
	return SyncEnvironmentCertificate{}, false
}

func ordinarySyncEnvironmentRequiresTerminalHistoryV1(environment SyncEnvironmentCertificate, trustedNowMillis int64) bool {
	return environment.Retirement != nil ||
		(environment.Mode == SyncEnvironmentEphemeral && trustedNowMillis >= environment.ExpiresAtMillis)
}
