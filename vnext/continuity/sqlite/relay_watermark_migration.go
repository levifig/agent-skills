package sqlite

import (
	"bytes"
	"database/sql"
	"fmt"
	"sort"
)

func collectV4SyncRelayWatermarkSeeds(tx *sql.Tx) ([]syncRelayWatermarkRecordV1, error) {
	seeds := make(map[syncRelayWatermarkKeyV1]syncRelayWatermarkRecordV1)
	observe := func(record syncRelayWatermarkRecordV1) error {
		if retained, found := seeds[record.key]; found {
			if retained.adminPublicKey != record.adminPublicKey {
				return fmt.Errorf("continuity v4 relay identity carries conflicting admin public keys")
			}
			if retained.relayHead >= record.relayHead {
				return nil
			}
		}
		seeds[record.key] = record
		return nil
	}

	projectRows, err := tx.Query(`
SELECT
  project.project_id,
  project.channel_id,
  project.relay_generation,
  project.admin_public_key,
  authority.inventory_arrival_head
FROM continuity_sync_projects AS project
LEFT JOIN continuity_sync_authorities AS authority
  ON authority.project_id = project.project_id
ORDER BY project.project_id`)
	if err != nil {
		return nil, fmt.Errorf("list continuity v4 sync relay observations: %w", err)
	}
	for projectRows.Next() {
		var projectID string
		var channelID, relayGeneration, adminPublicKey []byte
		var inventoryArrivalHead sql.NullInt64
		if err := projectRows.Scan(&projectID, &channelID, &relayGeneration, &adminPublicKey, &inventoryArrivalHead); err != nil {
			return nil, closeMigrationRows(projectRows, fmt.Errorf("scan continuity v4 sync relay observation: %w", err))
		}
		record, err := parseSyncRelayWatermarkRecordV1(projectID, channelID, relayGeneration, adminPublicKey, 0, 0, 0)
		if err != nil {
			return nil, closeMigrationRows(projectRows, fmt.Errorf("continuity v4 sync relay observation is invalid"))
		}
		if err := observe(record); err != nil {
			return nil, closeMigrationRows(projectRows, err)
		}
		if inventoryArrivalHead.Valid {
			record.relayHead = inventoryArrivalHead.Int64
			if record.relayHead < 0 {
				return nil, closeMigrationRows(projectRows, fmt.Errorf("continuity v4 canonical relay observation is invalid"))
			}
			if err := observe(record); err != nil {
				return nil, closeMigrationRows(projectRows, err)
			}
		}
	}
	if err := projectRows.Err(); err != nil {
		return nil, closeMigrationRows(projectRows, fmt.Errorf("iterate continuity v4 sync relay observations: %w", err))
	}
	if err := projectRows.Close(); err != nil {
		return nil, fmt.Errorf("close continuity v4 sync relay observations: %w", err)
	}

	candidateRows, err := tx.Query(`
SELECT project_id, channel_id, relay_generation, admin_public_key, inventory_arrival_head
FROM continuity_sync_authority_candidates
ORDER BY project_id, candidate_id`)
	if err != nil {
		return nil, fmt.Errorf("list continuity v4 authority candidate relay observations: %w", err)
	}
	for candidateRows.Next() {
		var projectID string
		var channelID, relayGeneration, adminPublicKey []byte
		var relayHead int64
		if err := candidateRows.Scan(&projectID, &channelID, &relayGeneration, &adminPublicKey, &relayHead); err != nil {
			return nil, closeMigrationRows(candidateRows, fmt.Errorf("scan continuity v4 authority candidate relay observation: %w", err))
		}
		record, err := parseSyncRelayWatermarkRecordV1(projectID, channelID, relayGeneration, adminPublicKey, 0, relayHead, 0)
		if err != nil {
			return nil, closeMigrationRows(candidateRows, fmt.Errorf("continuity v4 authority candidate relay observation is invalid"))
		}
		if err := observe(record); err != nil {
			return nil, closeMigrationRows(candidateRows, err)
		}
	}
	if err := candidateRows.Err(); err != nil {
		return nil, closeMigrationRows(candidateRows, fmt.Errorf("iterate continuity v4 authority candidate relay observations: %w", err))
	}
	if err := candidateRows.Close(); err != nil {
		return nil, fmt.Errorf("close continuity v4 authority candidate relay observations: %w", err)
	}

	ordered := make([]syncRelayWatermarkRecordV1, 0, len(seeds))
	for _, seed := range seeds {
		ordered = append(ordered, seed)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].key.projectID != ordered[right].key.projectID {
			return ordered[left].key.projectID < ordered[right].key.projectID
		}
		if comparison := bytes.Compare(ordered[left].key.channelID[:], ordered[right].key.channelID[:]); comparison != 0 {
			return comparison < 0
		}
		return bytes.Compare(ordered[left].key.relayGeneration[:], ordered[right].key.relayGeneration[:]) < 0
	})
	return ordered, nil
}

func insertV5SyncRelayWatermarkSeeds(tx *sql.Tx, seeds []syncRelayWatermarkRecordV1) error {
	for _, seed := range seeds {
		if _, err := tx.Exec(`
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key, relay_head
) VALUES(?, ?, ?, ?, ?)`,
			string(seed.key.projectID),
			seed.key.channelID[:],
			seed.key.relayGeneration[:],
			seed.adminPublicKey[:],
			seed.relayHead,
		); err != nil {
			return fmt.Errorf("seed continuity v5 relay watermark: %w", err)
		}
	}
	return nil
}

func collectV7SyncRelayWatermarkSeeds(tx *sql.Tx) ([]syncRelayWatermarkRecordV1, error) {
	seeds := make(map[syncRelayWatermarkKeyV1]syncRelayWatermarkRecordV1)
	observe := func(record syncRelayWatermarkRecordV1) error {
		retained, found := seeds[record.key]
		if !found {
			seeds[record.key] = record
			return nil
		}
		if retained.adminPublicKey != record.adminPublicKey {
			return fmt.Errorf("continuity v7 relay identity carries conflicting admin public keys")
		}
		if record.membershipGeneration > retained.membershipGeneration {
			retained.membershipGeneration = record.membershipGeneration
		}
		if record.relayHead > retained.relayHead {
			retained.relayHead = record.relayHead
		}
		retained.membershipFloorKnown = retained.membershipFloorKnown && record.membershipFloorKnown
		seeds[record.key] = retained
		return nil
	}

	legacyRows, err := tx.Query(`
SELECT project_id, channel_id, relay_generation, admin_public_key, relay_head
FROM continuity_sync_relay_watermarks
ORDER BY project_id, channel_id, relay_generation`)
	if err != nil {
		return nil, fmt.Errorf("list continuity v7 relay watermarks: %w", err)
	}
	for legacyRows.Next() {
		var projectID string
		var channelID, relayGeneration, adminPublicKey []byte
		var relayHead int64
		if err := legacyRows.Scan(&projectID, &channelID, &relayGeneration, &adminPublicKey, &relayHead); err != nil {
			return nil, closeMigrationRows(legacyRows, fmt.Errorf("scan continuity v7 relay watermark: %w", err))
		}
		record, err := parseSyncRelayWatermarkRecordV1(
			projectID, channelID, relayGeneration, adminPublicKey, 0, relayHead, 0,
		)
		if err != nil {
			return nil, closeMigrationRows(legacyRows, fmt.Errorf("continuity v7 relay watermark is invalid"))
		}
		if err := observe(record); err != nil {
			return nil, closeMigrationRows(legacyRows, err)
		}
	}
	if err := legacyRows.Err(); err != nil {
		return nil, closeMigrationRows(legacyRows, fmt.Errorf("iterate continuity v7 relay watermarks: %w", err))
	}
	if err := legacyRows.Close(); err != nil {
		return nil, fmt.Errorf("close continuity v7 relay watermarks: %w", err)
	}

	projectRows, err := tx.Query(`
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
ORDER BY project.project_id`)
	if err != nil {
		return nil, fmt.Errorf("list continuity v7 sync authority frontiers: %w", err)
	}
	for projectRows.Next() {
		var projectID string
		var channelID, relayGeneration, adminPublicKey []byte
		var membershipGeneration int64
		var inventoryArrivalHead sql.NullInt64
		if err := projectRows.Scan(
			&projectID, &channelID, &relayGeneration, &adminPublicKey,
			&membershipGeneration, &inventoryArrivalHead,
		); err != nil {
			return nil, closeMigrationRows(projectRows, fmt.Errorf("scan continuity v7 sync authority frontier: %w", err))
		}
		record, err := parseSyncRelayWatermarkRecordV1(
			projectID, channelID, relayGeneration, adminPublicKey,
			membershipGeneration, 0, 1,
		)
		if err != nil {
			return nil, closeMigrationRows(projectRows, fmt.Errorf("continuity v7 sync authority frontier is invalid"))
		}
		if err := observe(record); err != nil {
			return nil, closeMigrationRows(projectRows, err)
		}
		if inventoryArrivalHead.Valid {
			record.relayHead = inventoryArrivalHead.Int64
			if record.relayHead < 0 {
				return nil, closeMigrationRows(projectRows, fmt.Errorf("continuity v7 canonical frontier is invalid"))
			}
			if err := observe(record); err != nil {
				return nil, closeMigrationRows(projectRows, err)
			}
		}
	}
	if err := projectRows.Err(); err != nil {
		return nil, closeMigrationRows(projectRows, fmt.Errorf("iterate continuity v7 sync authority frontiers: %w", err))
	}
	if err := projectRows.Close(); err != nil {
		return nil, fmt.Errorf("close continuity v7 sync authority frontiers: %w", err)
	}

	candidateRows, err := tx.Query(`
SELECT project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head
FROM continuity_sync_authority_candidates
ORDER BY project_id, candidate_id`)
	if err != nil {
		return nil, fmt.Errorf("list continuity v7 authority candidate frontiers: %w", err)
	}
	for candidateRows.Next() {
		var projectID string
		var channelID, relayGeneration, adminPublicKey []byte
		var membershipGeneration, relayHead int64
		if err := candidateRows.Scan(
			&projectID, &channelID, &relayGeneration, &adminPublicKey,
			&membershipGeneration, &relayHead,
		); err != nil {
			return nil, closeMigrationRows(candidateRows, fmt.Errorf("scan continuity v7 authority candidate frontier: %w", err))
		}
		record, err := parseSyncRelayWatermarkRecordV1(
			projectID, channelID, relayGeneration, adminPublicKey,
			membershipGeneration, relayHead, 1,
		)
		if err != nil {
			return nil, closeMigrationRows(candidateRows, fmt.Errorf("continuity v7 authority candidate frontier is invalid"))
		}
		if err := observe(record); err != nil {
			return nil, closeMigrationRows(candidateRows, err)
		}
	}
	if err := candidateRows.Err(); err != nil {
		return nil, closeMigrationRows(candidateRows, fmt.Errorf("iterate continuity v7 authority candidate frontiers: %w", err))
	}
	if err := candidateRows.Close(); err != nil {
		return nil, fmt.Errorf("close continuity v7 authority candidate frontiers: %w", err)
	}

	ordered := make([]syncRelayWatermarkRecordV1, 0, len(seeds))
	for _, seed := range seeds {
		ordered = append(ordered, seed)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].key.projectID != ordered[right].key.projectID {
			return ordered[left].key.projectID < ordered[right].key.projectID
		}
		if comparison := bytes.Compare(ordered[left].key.channelID[:], ordered[right].key.channelID[:]); comparison != 0 {
			return comparison < 0
		}
		return bytes.Compare(ordered[left].key.relayGeneration[:], ordered[right].key.relayGeneration[:]) < 0
	})
	return ordered, nil
}

func insertV8SyncRelayWatermarkSeeds(tx *sql.Tx, seeds []syncRelayWatermarkRecordV1) error {
	for _, seed := range seeds {
		if _, err := tx.Exec(`
INSERT INTO continuity_sync_relay_watermarks(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
) VALUES(?, ?, ?, ?, ?, ?, ?)`,
			string(seed.key.projectID), seed.key.channelID[:], seed.key.relayGeneration[:], seed.adminPublicKey[:],
			seed.membershipGeneration, seed.relayHead, boolIntV2(seed.membershipFloorKnown),
		); err != nil {
			return fmt.Errorf("seed continuity v8 relay frontier: %w", err)
		}
	}
	return nil
}

func validateV8SyncRelayWatermarkSeeds(tx *sql.Tx, want []syncRelayWatermarkRecordV1) error {
	rows, err := tx.Query(`
SELECT project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, relay_head, membership_floor_known
FROM continuity_sync_relay_watermarks
ORDER BY project_id, channel_id, relay_generation`)
	if err != nil {
		return fmt.Errorf("list continuity v8 relay frontiers after copy: %w", err)
	}
	index := 0
	for rows.Next() {
		var projectID string
		var channelID, relayGeneration, adminPublicKey []byte
		var membershipGeneration, relayHead, membershipFloorKnown int64
		if err := rows.Scan(
			&projectID, &channelID, &relayGeneration, &adminPublicKey,
			&membershipGeneration, &relayHead, &membershipFloorKnown,
		); err != nil {
			return closeMigrationRows(rows, fmt.Errorf("scan continuity v8 relay frontier after copy: %w", err))
		}
		got, err := parseSyncRelayWatermarkRecordV1(
			projectID, channelID, relayGeneration, adminPublicKey,
			membershipGeneration, relayHead, membershipFloorKnown,
		)
		if err != nil || index >= len(want) || got != want[index] {
			return closeMigrationRows(rows, fmt.Errorf("continuity v8 relay frontier copy differs from the audited source join"))
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return closeMigrationRows(rows, fmt.Errorf("iterate continuity v8 relay frontiers after copy: %w", err))
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close continuity v8 relay frontiers after copy: %w", err)
	}
	if index != len(want) {
		return fmt.Errorf("continuity v8 relay frontier copy omitted an audited source join")
	}
	return nil
}
