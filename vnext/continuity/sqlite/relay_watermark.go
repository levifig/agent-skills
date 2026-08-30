package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

// SyncRelayWatermark is a public monotonic observation for one exact project,
// channel, and relay generation. It remains independent from candidate and
// canonical authority lifecycles.
type SyncRelayWatermark struct {
	ProjectID       continuity.ProjectID
	ChannelID       SyncChannelID
	RelayGeneration [32]byte
	AdminPublicKey  [32]byte
	RelayHead       int64
}

type syncRelayWatermarkKeyV1 struct {
	projectID       continuity.ProjectID
	channelID       SyncChannelID
	relayGeneration [32]byte
}

type syncRelayWatermarkRecordV1 struct {
	key            syncRelayWatermarkKeyV1
	adminPublicKey [32]byte
	relayHead      int64
}

// AdvanceSyncRelayWatermark retains a monotonic relay head for one exact
// public relay identity. Every matching canonical, candidate, and prior
// watermark observation is audited in the same serializable transaction.
func (store *Store) AdvanceSyncRelayWatermark(ctx context.Context, watermark SyncRelayWatermark) (SyncRelayWatermark, error) {
	if err := validateSyncRelayWatermarkV1(watermark); err != nil {
		return SyncRelayWatermark{}, err
	}
	if store == nil {
		return SyncRelayWatermark{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncRelayWatermark{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncRelayWatermark{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncRelayWatermark{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncRelayWatermark{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	floor, err := auditSyncRelayWatermarkSourcesV1(ctx, tx, watermark)
	if err != nil {
		return SyncRelayWatermark{}, err
	}
	if watermark.RelayHead < floor {
		return SyncRelayWatermark{}, syncProblem(SyncErrorCursor, "relay_head", "regressed below the retained watermark")
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key, relay_head
) VALUES(?, ?, ?, ?, ?)
ON CONFLICT(project_id, channel_id, relay_generation) DO UPDATE SET
  relay_head = excluded.relay_head
WHERE continuity_sync_relay_watermarks.admin_public_key = excluded.admin_public_key
  AND continuity_sync_relay_watermarks.relay_head < excluded.relay_head`,
		string(watermark.ProjectID),
		watermark.ChannelID[:],
		watermark.RelayGeneration[:],
		watermark.AdminPublicKey[:],
		watermark.RelayHead,
	); err != nil {
		return SyncRelayWatermark{}, syncTransactionProblem(ctx)
	}

	retained, found, err := readSyncRelayWatermarkV1(ctx, tx, syncRelayWatermarkKeyFromValueV1(watermark))
	if err != nil {
		return SyncRelayWatermark{}, err
	}
	if !found {
		return SyncRelayWatermark{}, corruptSyncRelayWatermarkProblemV1()
	}
	if retained.adminPublicKey != watermark.AdminPublicKey {
		return SyncRelayWatermark{}, syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
	}
	if retained.relayHead > watermark.RelayHead {
		return SyncRelayWatermark{}, syncProblem(SyncErrorCursor, "relay_head", "regressed below the retained watermark")
	}
	if retained.relayHead != watermark.RelayHead {
		return SyncRelayWatermark{}, corruptSyncRelayWatermarkProblemV1()
	}
	if err := tx.Commit(); err != nil {
		return SyncRelayWatermark{}, syncProblem(SyncErrorStore, "", "relay watermark commit outcome is unknown")
	}
	return retained.value(), nil
}

func validateSyncRelayWatermarkV1(watermark SyncRelayWatermark) error {
	if err := validateSyncProjectID(watermark.ProjectID); err != nil {
		return err
	}
	if watermark.ChannelID == (SyncChannelID{}) {
		return syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if watermark.RelayGeneration == [32]byte{} {
		return syncProblem(SyncErrorInvalid, "relay_generation", "must be a nonzero 32-byte value")
	}
	if watermark.AdminPublicKey == [32]byte{} {
		return syncProblem(SyncErrorInvalid, "admin_public_key", "must be a nonzero 32-byte value")
	}
	if watermark.RelayHead < 0 {
		return syncProblem(SyncErrorInvalid, "relay_head", "must not be negative")
	}
	return nil
}

func syncRelayWatermarkKeyFromValueV1(watermark SyncRelayWatermark) syncRelayWatermarkKeyV1 {
	return syncRelayWatermarkKeyV1{
		projectID:       watermark.ProjectID,
		channelID:       watermark.ChannelID,
		relayGeneration: watermark.RelayGeneration,
	}
}

func (record syncRelayWatermarkRecordV1) value() SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:       record.key.projectID,
		ChannelID:       record.key.channelID,
		RelayGeneration: record.key.relayGeneration,
		AdminPublicKey:  record.adminPublicKey,
		RelayHead:       record.relayHead,
	}
}

// auditBoundedSyncRelayWatermarkSourcesV1 audits the exact retained watermark
// and the single current project/canonical source without scanning historical
// authority-candidate headers.
func auditBoundedSyncRelayWatermarkSourcesV1(ctx context.Context, tx *sql.Tx, watermark SyncRelayWatermark) (int64, error) {
	key := syncRelayWatermarkKeyFromValueV1(watermark)
	floor := int64(0)
	observe := func(record syncRelayWatermarkRecordV1) error {
		if record.key != key {
			return nil
		}
		if record.adminPublicKey != watermark.AdminPublicKey {
			return syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
		}
		if record.relayHead > floor {
			floor = record.relayHead
		}
		return nil
	}

	retained, found, err := readSyncRelayWatermarkV1(ctx, tx, key)
	if err != nil {
		return 0, err
	}
	if found {
		if err := observe(retained); err != nil {
			return 0, err
		}
	}

	var projectID string
	var channelID, relayGeneration, adminPublicKey []byte
	var relayHead int64
	var inventoryArrivalHead sql.NullInt64
	err = tx.QueryRowContext(ctx, `
SELECT
  project.project_id,
  project.channel_id,
  project.relay_generation,
  project.admin_public_key,
  project.relay_head,
  authority.inventory_arrival_head
FROM continuity_sync_projects AS project
LEFT JOIN continuity_sync_authorities AS authority
  ON authority.project_id = project.project_id
WHERE project.project_id = ?`, string(watermark.ProjectID)).Scan(
		&projectID,
		&channelID,
		&relayGeneration,
		&adminPublicKey,
		&relayHead,
		&inventoryArrivalHead,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return 0, syncTransactionProblem(ctx)
	default:
		record, recordErr := parseSyncRelayWatermarkRecordV1(projectID, channelID, relayGeneration, adminPublicKey, relayHead)
		if recordErr != nil {
			return 0, corruptSyncRelayWatermarkProblemV1()
		}
		if err := observe(record); err != nil {
			return 0, err
		}
		if inventoryArrivalHead.Valid {
			record.relayHead = inventoryArrivalHead.Int64
			if record.relayHead < 0 {
				return 0, corruptSyncRelayWatermarkProblemV1()
			}
			if err := observe(record); err != nil {
				return 0, err
			}
		}
	}
	return floor, nil
}

func auditSyncRelayWatermarkSourcesV1(ctx context.Context, tx *sql.Tx, watermark SyncRelayWatermark) (int64, error) {
	floor, err := auditBoundedSyncRelayWatermarkSourcesV1(ctx, tx, watermark)
	if err != nil {
		return 0, err
	}
	key := syncRelayWatermarkKeyFromValueV1(watermark)
	observe := func(record syncRelayWatermarkRecordV1) error {
		if record.key != key {
			return nil
		}
		if record.adminPublicKey != watermark.AdminPublicKey {
			return syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
		}
		if record.relayHead > floor {
			floor = record.relayHead
		}
		return nil
	}

	rows, err := tx.QueryContext(ctx, `
SELECT project_id, channel_id, relay_generation, admin_public_key, inventory_arrival_head
FROM continuity_sync_authority_candidates
WHERE project_id = ?
ORDER BY candidate_id`, string(watermark.ProjectID))
	if err != nil {
		return 0, syncTransactionProblem(ctx)
	}
	for rows.Next() {
		var candidateProjectID string
		var candidateChannelID, candidateRelayGeneration, candidateAdminPublicKey []byte
		var candidateHead int64
		if err := rows.Scan(&candidateProjectID, &candidateChannelID, &candidateRelayGeneration, &candidateAdminPublicKey, &candidateHead); err != nil {
			rows.Close()
			return 0, syncTransactionProblem(ctx)
		}
		record, err := parseSyncRelayWatermarkRecordV1(candidateProjectID, candidateChannelID, candidateRelayGeneration, candidateAdminPublicKey, candidateHead)
		if err != nil {
			rows.Close()
			return 0, corruptSyncRelayWatermarkProblemV1()
		}
		if err := observe(record); err != nil {
			rows.Close()
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return 0, syncTransactionProblem(ctx)
	}
	return floor, nil
}

func readSyncRelayWatermarkV1(ctx context.Context, tx *sql.Tx, key syncRelayWatermarkKeyV1) (syncRelayWatermarkRecordV1, bool, error) {
	var projectID string
	var channelID, relayGeneration, adminPublicKey []byte
	var relayHead int64
	err := tx.QueryRowContext(ctx, `
SELECT project_id, channel_id, relay_generation, admin_public_key, relay_head
FROM continuity_sync_relay_watermarks
WHERE project_id = ? AND channel_id = ? AND relay_generation = ?`,
		string(key.projectID), key.channelID[:], key.relayGeneration[:],
	).Scan(&projectID, &channelID, &relayGeneration, &adminPublicKey, &relayHead)
	if errors.Is(err, sql.ErrNoRows) {
		return syncRelayWatermarkRecordV1{}, false, nil
	}
	if err != nil {
		return syncRelayWatermarkRecordV1{}, false, syncTransactionProblem(ctx)
	}
	record, err := parseSyncRelayWatermarkRecordV1(projectID, channelID, relayGeneration, adminPublicKey, relayHead)
	if err != nil || record.key != key {
		return syncRelayWatermarkRecordV1{}, false, corruptSyncRelayWatermarkProblemV1()
	}
	return record, true, nil
}

func parseSyncRelayWatermarkRecordV1(projectID string, channelID, relayGeneration, adminPublicKey []byte, relayHead int64) (syncRelayWatermarkRecordV1, error) {
	if !validOpaqueID(projectID) || len(channelID) != 32 || bytes.Equal(channelID, make([]byte, 32)) ||
		len(relayGeneration) != 32 || bytes.Equal(relayGeneration, make([]byte, 32)) ||
		len(adminPublicKey) != 32 || bytes.Equal(adminPublicKey, make([]byte, 32)) || relayHead < 0 {
		return syncRelayWatermarkRecordV1{}, errors.New("invalid durable relay observation")
	}
	record := syncRelayWatermarkRecordV1{
		key: syncRelayWatermarkKeyV1{
			projectID: continuity.ProjectID(projectID),
		},
		relayHead: relayHead,
	}
	copy(record.key.channelID[:], channelID)
	copy(record.key.relayGeneration[:], relayGeneration)
	copy(record.adminPublicKey[:], adminPublicKey)
	return record, nil
}

func corruptSyncRelayWatermarkProblemV1() error {
	return syncProblem(SyncErrorStore, "relay_watermark", "durable relay observation is invalid")
}
