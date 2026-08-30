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
  project.relay_head,
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
		var relayHead int64
		var inventoryArrivalHead sql.NullInt64
		if err := projectRows.Scan(&projectID, &channelID, &relayGeneration, &adminPublicKey, &relayHead, &inventoryArrivalHead); err != nil {
			return nil, closeMigrationRows(projectRows, fmt.Errorf("scan continuity v4 sync relay observation: %w", err))
		}
		record, err := parseSyncRelayWatermarkRecordV1(projectID, channelID, relayGeneration, adminPublicKey, relayHead)
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
		record, err := parseSyncRelayWatermarkRecordV1(projectID, channelID, relayGeneration, adminPublicKey, relayHead)
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
