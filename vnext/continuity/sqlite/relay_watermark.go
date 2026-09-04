package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"

	"github.com/levifig/loaf/vnext/continuity"
)

// SyncRelayWatermark is a public monotonic authority observation for one exact
// project, channel, and relay generation. MembershipGeneration and RelayHead
// are independent coordinates: the retained value is their componentwise
// join, which may be a rejection floor rather than one observed relay state.
type SyncRelayWatermark struct {
	ProjectID            continuity.ProjectID
	ChannelID            SyncChannelID
	RelayGeneration      [32]byte
	AdminPublicKey       [32]byte
	MembershipGeneration uint32
	RelayHead            int64
}

type syncRelayWatermarkKeyV1 struct {
	projectID       continuity.ProjectID
	channelID       SyncChannelID
	relayGeneration [32]byte
}

type syncRelayWatermarkRecordV1 struct {
	key                  syncRelayWatermarkKeyV1
	adminPublicKey       [32]byte
	membershipGeneration uint32
	relayHead            int64
	membershipFloorKnown bool
}

// AdvanceSyncRelayWatermark retains the componentwise join of one complete,
// already-verified observation and every matching durable source. A returned
// value different from watermark means the observation was crossed or stale
// and must not drive authority routing, even though its higher coordinate was
// retained. Every matching canonical, candidate, and prior observation is
// audited in the same serializable transaction.
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

	retained, err := retainVerifiedSyncRelayWatermarkObservationV1(ctx, tx, watermark)
	if err != nil {
		return SyncRelayWatermark{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyncRelayWatermark{}, syncProblem(SyncErrorStore, "", "relay watermark commit outcome is unknown")
	}
	return retained.value(), nil
}

func retainVerifiedSyncRelayWatermarkObservationV1(
	ctx context.Context,
	tx *sql.Tx,
	observation SyncRelayWatermark,
) (syncRelayWatermarkRecordV1, error) {
	floor, err := auditSyncRelayWatermarkSourcesV1(ctx, tx, observation)
	if err != nil {
		return syncRelayWatermarkRecordV1{}, err
	}
	return retainSyncRelayWatermarkRecordV1(ctx, tx, joinVerifiedSyncRelayWatermarkV1(floor, observation))
}

func advanceVerifiedSyncRelayWatermarkObservationV1(
	ctx context.Context,
	tx *sql.Tx,
	observation SyncRelayWatermark,
) error {
	retained, err := retainVerifiedSyncRelayWatermarkObservationV1(ctx, tx, observation)
	if err != nil {
		return err
	}
	return requireSyncRelayObservationAtExactWatermarkV1(observation, retained)
}

// advanceVerifiedCompatibilityAuthorityWatermarkV1 retains a fresh verified
// v1 authority membership observation without treating its required zero
// inventory-arrival head as a real channel-head observation. Any stronger head
// evidence remains durable; the compatibility surface can establish only the
// membership coordinate.
func advanceVerifiedCompatibilityAuthorityWatermarkV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	authority SyncAuthority,
) error {
	observation := syncRelayWatermarkFromAuthorityV1(projectID, authority)
	floor, err := auditSyncRelayWatermarkSourcesV1(ctx, tx, observation)
	if err != nil {
		return err
	}
	if observation.MembershipGeneration < floor.membershipGeneration {
		return syncProblem(SyncErrorCursor, "membership_generation", "does not match the retained authority watermark")
	}
	floor.membershipGeneration = observation.MembershipGeneration
	floor.membershipFloorKnown = true
	_, err = retainSyncRelayWatermarkRecordV1(ctx, tx, floor)
	return err
}

func requireKnownExactSyncRelayWatermarkV1(
	ctx context.Context,
	tx *sql.Tx,
	want SyncRelayWatermark,
) error {
	floor, err := auditSyncRelayWatermarkSourcesV1(ctx, tx, want)
	if err != nil {
		return err
	}
	return requireSyncRelayObservationAtExactWatermarkV1(want, floor)
}

func requireSyncRelayObservationAtExactWatermarkV1(
	want SyncRelayWatermark,
	floor syncRelayWatermarkRecordV1,
) error {
	if !floor.membershipFloorKnown {
		return syncProblem(SyncErrorCursor, "membership_generation", "retained membership floor requires a fresh verified observation")
	}
	if want.MembershipGeneration != floor.membershipGeneration {
		return syncProblem(SyncErrorCursor, "membership_generation", "does not match the retained authority watermark")
	}
	if want.RelayHead != floor.relayHead {
		return syncProblem(SyncErrorCursor, "inventory_arrival_head", "does not match the retained authority watermark")
	}
	return nil
}

func syncRelayWatermarkFromAuthorityV1(projectID continuity.ProjectID, authority SyncAuthority) SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            authority.ChannelID,
		RelayGeneration:      authority.RelayGeneration,
		AdminPublicKey:       authority.AdminPublicKey,
		MembershipGeneration: authority.MembershipGeneration,
		RelayHead:            authority.InventoryArrivalHead,
	}
}

func syncRelayWatermarkFromAuthorityBindingV1(projectID continuity.ProjectID, binding SyncAuthorityBinding) SyncRelayWatermark {
	return SyncRelayWatermark{
		ProjectID:            projectID,
		ChannelID:            binding.ChannelID,
		RelayGeneration:      binding.RelayGeneration,
		AdminPublicKey:       binding.AdminPublicKey,
		MembershipGeneration: binding.MembershipGeneration,
		RelayHead:            binding.InventoryArrivalHead,
	}
}

func retainSyncRelayWatermarkRecordV1(
	ctx context.Context,
	tx *sql.Tx,
	joined syncRelayWatermarkRecordV1,
) (syncRelayWatermarkRecordV1, error) {
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
) VALUES(?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, channel_id, relay_generation) DO UPDATE SET
	  membership_generation = excluded.membership_generation,
	  relay_head = excluded.relay_head,
	  membership_floor_known = excluded.membership_floor_known
WHERE continuity_sync_relay_watermarks.admin_public_key = excluded.admin_public_key`,
		string(joined.key.projectID), joined.key.channelID[:], joined.key.relayGeneration[:], joined.adminPublicKey[:],
		joined.membershipGeneration, joined.relayHead, boolIntV2(joined.membershipFloorKnown),
	); err != nil {
		return syncRelayWatermarkRecordV1{}, syncTransactionProblem(ctx)
	}

	retained, found, err := readSyncRelayWatermarkV1(ctx, tx, joined.key)
	if err != nil {
		return syncRelayWatermarkRecordV1{}, err
	}
	if !found {
		return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
	}
	if retained.adminPublicKey != joined.adminPublicKey {
		return syncRelayWatermarkRecordV1{}, syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
	}
	if retained != joined {
		return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
	}
	return retained, nil
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
		ProjectID:            record.key.projectID,
		ChannelID:            record.key.channelID,
		RelayGeneration:      record.key.relayGeneration,
		AdminPublicKey:       record.adminPublicKey,
		MembershipGeneration: record.membershipGeneration,
		RelayHead:            record.relayHead,
	}
}

func joinVerifiedSyncRelayWatermarkV1(
	floor syncRelayWatermarkRecordV1,
	observation SyncRelayWatermark,
) syncRelayWatermarkRecordV1 {
	joined := floor
	dominates := observation.MembershipGeneration >= floor.membershipGeneration &&
		observation.RelayHead >= floor.relayHead
	if observation.MembershipGeneration > joined.membershipGeneration {
		joined.membershipGeneration = observation.MembershipGeneration
	}
	if observation.RelayHead > joined.relayHead {
		joined.relayHead = observation.RelayHead
	}
	joined.membershipFloorKnown = floor.membershipFloorKnown || dominates
	return joined
}

// auditBoundedSyncRelayWatermarkSourcesV1 audits the exact retained watermark
// and the single current project/canonical source without scanning historical
// authority-candidate headers.
func auditBoundedSyncRelayWatermarkSourcesV1(
	ctx context.Context,
	tx *sql.Tx,
	watermark SyncRelayWatermark,
) (syncRelayWatermarkRecordV1, error) {
	key := syncRelayWatermarkKeyFromValueV1(watermark)
	floor := syncRelayWatermarkRecordV1{
		key:                  key,
		adminPublicKey:       watermark.AdminPublicKey,
		membershipFloorKnown: true,
	}
	observe := func(record syncRelayWatermarkRecordV1) error {
		if record.key != key {
			return nil
		}
		if record.adminPublicKey != watermark.AdminPublicKey {
			return syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
		}
		if record.membershipGeneration > floor.membershipGeneration {
			floor.membershipGeneration = record.membershipGeneration
		}
		if record.relayHead > floor.relayHead {
			floor.relayHead = record.relayHead
		}
		return nil
	}

	retained, found, err := readSyncRelayWatermarkV1(ctx, tx, key)
	if err != nil {
		return syncRelayWatermarkRecordV1{}, err
	}
	if found {
		floor.membershipFloorKnown = retained.membershipFloorKnown
		if err := observe(retained); err != nil {
			return syncRelayWatermarkRecordV1{}, err
		}
	}

	var projectID string
	var channelID, relayGeneration, adminPublicKey []byte
	var membershipGeneration int64
	var inventoryArrivalHead sql.NullInt64
	err = tx.QueryRowContext(ctx, `
SELECT
  project.project_id,
  project.channel_id,
  project.relay_generation,
  project.admin_public_key,
  project.membership_generation,
  authority.inventory_arrival_head
FROM continuity_sync_projects AS project
LEFT JOIN continuity_sync_authorities AS authority
  ON authority.project_id = project.project_id
WHERE project.project_id = ?`, string(watermark.ProjectID)).Scan(
		&projectID,
		&channelID,
		&relayGeneration,
		&adminPublicKey,
		&membershipGeneration,
		&inventoryArrivalHead,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return syncRelayWatermarkRecordV1{}, syncTransactionProblem(ctx)
	default:
		record, recordErr := parseSyncRelayWatermarkRecordV1(
			projectID, channelID, relayGeneration, adminPublicKey, membershipGeneration, 0, 1,
		)
		if recordErr != nil {
			return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
		}
		if err := observe(record); err != nil {
			return syncRelayWatermarkRecordV1{}, err
		}
		if inventoryArrivalHead.Valid {
			record.relayHead = inventoryArrivalHead.Int64
			if record.relayHead < 0 {
				return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
			}
			if err := observe(record); err != nil {
				return syncRelayWatermarkRecordV1{}, err
			}
		}
	}
	return floor, nil
}

func auditSyncRelayWatermarkSourcesV1(
	ctx context.Context,
	tx *sql.Tx,
	watermark SyncRelayWatermark,
) (syncRelayWatermarkRecordV1, error) {
	floor, err := auditBoundedSyncRelayWatermarkSourcesV1(ctx, tx, watermark)
	if err != nil {
		return syncRelayWatermarkRecordV1{}, err
	}
	key := syncRelayWatermarkKeyFromValueV1(watermark)
	observe := func(record syncRelayWatermarkRecordV1) error {
		if record.key != key {
			return nil
		}
		if record.adminPublicKey != watermark.AdminPublicKey {
			return syncProblem(SyncErrorConflict, "admin_public_key", "does not match the retained relay identity")
		}
		if record.membershipGeneration > floor.membershipGeneration {
			floor.membershipGeneration = record.membershipGeneration
		}
		if record.relayHead > floor.relayHead {
			floor.relayHead = record.relayHead
		}
		return nil
	}

	rows, err := tx.QueryContext(ctx, `
SELECT project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head
FROM continuity_sync_authority_candidates
WHERE project_id = ?
	ORDER BY candidate_id`, string(watermark.ProjectID))
	if err != nil {
		return syncRelayWatermarkRecordV1{}, syncTransactionProblem(ctx)
	}
	for rows.Next() {
		var candidateProjectID string
		var candidateChannelID, candidateRelayGeneration, candidateAdminPublicKey []byte
		var candidateMembership, candidateHead int64
		if err := rows.Scan(
			&candidateProjectID, &candidateChannelID, &candidateRelayGeneration,
			&candidateAdminPublicKey, &candidateMembership, &candidateHead,
		); err != nil {
			rows.Close()
			return syncRelayWatermarkRecordV1{}, syncTransactionProblem(ctx)
		}
		record, err := parseSyncRelayWatermarkRecordV1(
			candidateProjectID, candidateChannelID, candidateRelayGeneration,
			candidateAdminPublicKey, candidateMembership, candidateHead, 1,
		)
		if err != nil {
			rows.Close()
			return syncRelayWatermarkRecordV1{}, corruptSyncRelayWatermarkProblemV1()
		}
		if err := observe(record); err != nil {
			rows.Close()
			return syncRelayWatermarkRecordV1{}, err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return syncRelayWatermarkRecordV1{}, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return syncRelayWatermarkRecordV1{}, syncTransactionProblem(ctx)
	}
	return floor, nil
}

func readSyncRelayWatermarkV1(ctx context.Context, tx *sql.Tx, key syncRelayWatermarkKeyV1) (syncRelayWatermarkRecordV1, bool, error) {
	var projectID string
	var channelID, relayGeneration, adminPublicKey []byte
	var membershipGeneration, relayHead, membershipFloorKnown int64
	err := tx.QueryRowContext(ctx, `
SELECT project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
FROM continuity_sync_relay_watermarks
WHERE project_id = ? AND channel_id = ? AND relay_generation = ?`,
		string(key.projectID), key.channelID[:], key.relayGeneration[:],
	).Scan(
		&projectID, &channelID, &relayGeneration, &adminPublicKey,
		&membershipGeneration, &relayHead, &membershipFloorKnown,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return syncRelayWatermarkRecordV1{}, false, nil
	}
	if err != nil {
		return syncRelayWatermarkRecordV1{}, false, syncTransactionProblem(ctx)
	}
	record, err := parseSyncRelayWatermarkRecordV1(
		projectID, channelID, relayGeneration, adminPublicKey,
		membershipGeneration, relayHead, membershipFloorKnown,
	)
	if err != nil || record.key != key {
		return syncRelayWatermarkRecordV1{}, false, corruptSyncRelayWatermarkProblemV1()
	}
	return record, true, nil
}

func parseSyncRelayWatermarkRecordV1(
	projectID string,
	channelID, relayGeneration, adminPublicKey []byte,
	membershipGeneration, relayHead, membershipFloorKnown int64,
) (syncRelayWatermarkRecordV1, error) {
	if !validOpaqueID(projectID) || len(channelID) != 32 || bytes.Equal(channelID, make([]byte, 32)) ||
		len(relayGeneration) != 32 || bytes.Equal(relayGeneration, make([]byte, 32)) ||
		len(adminPublicKey) != 32 || bytes.Equal(adminPublicKey, make([]byte, 32)) ||
		membershipGeneration < 0 || membershipGeneration > int64(^uint32(0)) || relayHead < 0 ||
		(membershipFloorKnown != 0 && membershipFloorKnown != 1) {
		return syncRelayWatermarkRecordV1{}, errors.New("invalid durable relay observation")
	}
	record := syncRelayWatermarkRecordV1{
		key: syncRelayWatermarkKeyV1{
			projectID: continuity.ProjectID(projectID),
		},
		membershipGeneration: uint32(membershipGeneration),
		relayHead:            relayHead,
		membershipFloorKnown: membershipFloorKnown == 1,
	}
	copy(record.key.channelID[:], channelID)
	copy(record.key.relayGeneration[:], relayGeneration)
	copy(record.adminPublicKey[:], adminPublicKey)
	return record, nil
}

func corruptSyncRelayWatermarkProblemV1() error {
	return syncProblem(SyncErrorStore, "relay_watermark", "durable relay observation is invalid")
}
