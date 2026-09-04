package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func (store *Store) EnvironmentInventory(ctx context.Context, request relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error) {
	if err := validateContext(ctx); err != nil {
		return relay.EnvironmentInventoryPage{}, err
	}
	if err := request.Validate(); err != nil {
		return relay.EnvironmentInventoryPage{}, err
	}
	if err := store.readLock(); err != nil {
		return relay.EnvironmentInventoryPage{}, err
	}
	defer store.readUnlock()

	var page relay.EnvironmentInventoryPage
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		channel, err := store.authenticateInventory(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		currentHead, err := channelHead(ctx, tx, channel.authority.ChannelID)
		if err != nil {
			return err
		}
		snapshot := relay.EnvironmentInventorySnapshot{
			MembershipGeneration: channel.membershipGeneration,
			ArrivalHead:          currentHead,
		}
		if request.Snapshot != nil {
			snapshot = *request.Snapshot
			if snapshot.MembershipGeneration != channel.membershipGeneration {
				return relay.ErrMembershipChanged
			}
			if snapshot.ArrivalHead > currentHead {
				return relay.ErrRollback
			}
		}
		page = relay.EnvironmentInventoryPage{
			Channel:      channel.authority,
			Snapshot:     snapshot,
			Environments: make([]relay.EnvironmentInventoryRecord, 0, request.Limit),
		}
		rows, err := tx.QueryContext(ctx, `
SELECT e.environment_id, e.certificate_id, e.certificate_bytes, e.mode,
       e.expires_at_millis, e.membership_generation,
       COALESCE((
         SELECT MAX(a.environment_sequence)
         FROM relay_arrivals AS a
         WHERE a.channel_id = e.channel_id AND a.environment_id = e.environment_id
           AND a.arrival_sequence <= ?
       ), 0),
       e.retired_at_millis, e.retirement_generation, e.retirement_relay_generation,
       e.retirement_certificate_id, e.retirement_final_environment_sequence,
       e.retirement_final_envelope_digest, e.retirement_id, e.retirement_bytes
FROM relay_environments AS e
WHERE e.channel_id = ? AND e.environment_id > ?
ORDER BY e.environment_id
LIMIT ?`,
			snapshot.ArrivalHead,
			channel.authority.ChannelID[:],
			string(request.AfterEnvironmentID),
			request.Limit+1,
		)
		if err != nil {
			return fmt.Errorf("query relay environment inventory: %w", err)
		}
		for rows.Next() {
			record, err := scanEnvironmentInventory(rows)
			if err != nil {
				return closeRowsWithError(rows, err, "relay environment inventory")
			}
			page.Environments = append(page.Environments, record)
		}
		if err := rows.Err(); err != nil {
			return closeRowsWithError(rows, fmt.Errorf("iterate relay environment inventory: %w", err), "relay environment inventory")
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close relay environment inventory: %w", err)
		}
		if len(page.Environments) > request.Limit {
			page.More = true
			page.Environments = page.Environments[:request.Limit]
		}
		return nil
	})
	if err != nil {
		return relay.EnvironmentInventoryPage{}, err
	}
	return page, nil
}

func (store *Store) PruneInventory(ctx context.Context, request relay.PruneInventoryRequest) (relay.PruneInventoryPage, error) {
	if err := validateContext(ctx); err != nil {
		return relay.PruneInventoryPage{}, err
	}
	if err := request.Validate(); err != nil {
		return relay.PruneInventoryPage{}, err
	}
	if err := store.readLock(); err != nil {
		return relay.PruneInventoryPage{}, err
	}
	defer store.readUnlock()

	var page relay.PruneInventoryPage
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		channel, err := store.authenticateInventory(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		currentPruneHead, err := pruneHead(ctx, tx, channel.authority.ChannelID)
		if err != nil {
			return err
		}
		currentArrivalHead, err := channelHead(ctx, tx, channel.authority.ChannelID)
		if err != nil {
			return err
		}
		snapshot := relay.PruneInventorySnapshot{
			MembershipGeneration: channel.membershipGeneration,
			ArrivalHead:          currentArrivalHead,
			PruneHead:            currentPruneHead,
		}
		if request.Snapshot != nil {
			snapshot = *request.Snapshot
			if snapshot.MembershipGeneration != channel.membershipGeneration {
				return relay.ErrMembershipChanged
			}
			if snapshot.ArrivalHead > currentArrivalHead || snapshot.PruneHead > currentPruneHead {
				return relay.ErrRollback
			}
		}
		if request.After > snapshot.PruneHead {
			return fmt.Errorf("%w: prune inventory cursor exceeds snapshot", relay.ErrInvalidArgument)
		}
		page = relay.PruneInventoryPage{
			Channel:  channel.authority,
			Snapshot: snapshot,
			Prunes:   make([]relay.PruneInventoryRecord, 0, request.Limit),
		}
		rows, err := tx.QueryContext(ctx, `
SELECT prune_sequence, prune_id, membership_generation, barrier_arrival_sequence,
       closure_fact_id, closure_environment_id, closure_environment_sequence,
       closure_arrival_sequence, closure_envelope_digest, closure_certificate_id,
       closure_previous_envelope_digest, closure_key_generation, closure_nonce,
       certificate_id, certificate_bytes, target_count, created_at_millis
FROM relay_prune_certificates
WHERE channel_id = ? AND prune_sequence > ? AND prune_sequence <= ?
ORDER BY prune_sequence
LIMIT ?`,
			channel.authority.ChannelID[:],
			request.After,
			snapshot.PruneHead,
			request.Limit+1,
		)
		if err != nil {
			return fmt.Errorf("query relay prune inventory: %w", err)
		}
		targetCounts := make([]int, 0, request.Limit+1)
		for rows.Next() {
			record, targetCount, err := scanPruneInventory(rows, channel.authority.ChannelID)
			if err != nil {
				return closeRowsWithError(rows, err, "relay prune inventory")
			}
			page.Prunes = append(page.Prunes, record)
			targetCounts = append(targetCounts, targetCount)
		}
		if err := rows.Err(); err != nil {
			return closeRowsWithError(rows, fmt.Errorf("iterate relay prune inventory: %w", err), "relay prune inventory")
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close relay prune inventory: %w", err)
		}
		if len(page.Prunes) > request.Limit {
			page.More = true
			page.Prunes = page.Prunes[:request.Limit]
			targetCounts = targetCounts[:request.Limit]
		}
		for index := range page.Prunes {
			targets, err := readPruneInventoryTargets(ctx, tx, channel.authority.ChannelID, page.Prunes[index].Certificate.PruneID, targetCounts[index])
			if err != nil {
				return err
			}
			page.Prunes[index].Certificate.Targets = targets
			if err := page.Prunes[index].Certificate.Validate(); err != nil {
				return fmt.Errorf("relay prune inventory record is corrupt: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return relay.PruneInventoryPage{}, err
	}
	return page, nil
}

func (store *Store) authenticateInventory(ctx context.Context, tx *sql.Tx, authorization relay.InventoryAuthorization) (channelRecord, error) {
	switch {
	case authorization.Owner != nil && authorization.Environment == nil:
		return authenticateOwner(ctx, tx, *authorization.Owner)
	case authorization.Owner == nil && authorization.Environment != nil:
		environment, err := authenticateEnvironment(ctx, tx, *authorization.Environment)
		if err != nil {
			return channelRecord{}, err
		}
		if err := requireActiveEnvironment(environment, store.now().UTC().UnixMilli()); err != nil {
			return channelRecord{}, err
		}
		return channelRecord{
			authority:            environment.authority.ChannelAuthority,
			membershipGeneration: environment.currentMembership,
		}, nil
	default:
		return channelRecord{}, fmt.Errorf("%w: inventory authorization", relay.ErrInvalidArgument)
	}
}

func scanEnvironmentInventory(scanner rowScanner) (relay.EnvironmentInventoryRecord, error) {
	var record relay.EnvironmentInventoryRecord
	var certificateID, certificateBytes, retirementRelayGeneration, retirementCertificateID []byte
	var finalEnvelopeDigest, retirementID, retirementBytes []byte
	var mode string
	var retiredAt, retirementGeneration, finalEnvironmentSequence sql.NullInt64
	if err := scanner.Scan(
		&record.EnvironmentID,
		&certificateID,
		&certificateBytes,
		&mode,
		&record.ExpiresAtMillis,
		&record.MembershipGeneration,
		&record.ProducerHead,
		&retiredAt,
		&retirementGeneration,
		&retirementRelayGeneration,
		&retirementCertificateID,
		&finalEnvironmentSequence,
		&finalEnvelopeDigest,
		&retirementID,
		&retirementBytes,
	); err != nil {
		return relay.EnvironmentInventoryRecord{}, fmt.Errorf("scan relay environment inventory: %w", err)
	}
	if !scanFixed(record.CertificateID[:], certificateID) {
		return relay.EnvironmentInventoryRecord{}, fmt.Errorf("relay environment inventory contains an invalid certificate id")
	}
	record.CertificateBytes = append([]byte(nil), certificateBytes...)
	record.Mode = relay.EnvironmentMode(mode)
	if retiredAt.Valid {
		retirement := &relay.EnvironmentRetirement{
			RetiredAt:       millisTime(retiredAt.Int64),
			RetirementBytes: append([]byte(nil), retirementBytes...),
		}
		if !retirementGeneration.Valid || retirementGeneration.Int64 < 1 ||
			retirementGeneration.Int64 > int64(^uint32(0)) ||
			!finalEnvironmentSequence.Valid || finalEnvironmentSequence.Int64 < 0 ||
			!scanFixed(retirement.RelayGeneration[:], retirementRelayGeneration) ||
			!scanFixed(retirement.CertificateID[:], retirementCertificateID) ||
			!scanFixed(retirement.FinalEnvelopeDigest[:], finalEnvelopeDigest) ||
			!scanFixed(retirement.RetirementID[:], retirementID) {
			return relay.EnvironmentInventoryRecord{}, fmt.Errorf("relay environment inventory contains an invalid retirement")
		}
		retirement.MembershipGeneration = uint32(retirementGeneration.Int64)
		retirement.FinalEnvironmentSequence = finalEnvironmentSequence.Int64
		if (retirement.FinalEnvironmentSequence == 0) != zeroDigest(retirement.FinalEnvelopeDigest) {
			return relay.EnvironmentInventoryRecord{}, fmt.Errorf("relay environment inventory contains an invalid final fence")
		}
		record.Retirement = retirement
	} else if retirementGeneration.Valid || retirementRelayGeneration != nil || retirementCertificateID != nil ||
		finalEnvironmentSequence.Valid || finalEnvelopeDigest != nil || retirementID != nil || retirementBytes != nil {
		return relay.EnvironmentInventoryRecord{}, fmt.Errorf("relay environment inventory contains a partial retirement")
	}
	return record, nil
}

func zeroDigest(value relay.Digest) bool {
	return value == (relay.Digest{})
}

func scanPruneInventory(scanner rowScanner, channelID relay.ChannelID) (relay.PruneInventoryRecord, int, error) {
	var record relay.PruneInventoryRecord
	var pruneID, closureEnvelopeDigest, closureCertificateID, closurePreviousEnvelopeDigest, closureNonce, certificateID, certificateBytes []byte
	var createdAtMillis int64
	var targetCount int
	var closureKeyGeneration int64
	record.Certificate.ChannelID = channelID
	if err := scanner.Scan(
		&record.PruneSequence,
		&pruneID,
		&record.Certificate.MembershipGeneration,
		&record.Certificate.Barrier,
		&record.Certificate.Closure.FactID,
		&record.Certificate.Closure.EnvironmentID,
		&record.Certificate.Closure.EnvironmentSequence,
		&record.Certificate.Closure.ArrivalSequence,
		&closureEnvelopeDigest,
		&closureCertificateID,
		&closurePreviousEnvelopeDigest,
		&closureKeyGeneration,
		&closureNonce,
		&certificateID,
		&certificateBytes,
		&targetCount,
		&createdAtMillis,
	); err != nil {
		return relay.PruneInventoryRecord{}, 0, fmt.Errorf("scan relay prune inventory: %w", err)
	}
	if !scanFixed(record.Certificate.PruneID[:], pruneID) ||
		!scanFixed(record.Certificate.Closure.EnvelopeDigest[:], closureEnvelopeDigest) ||
		!scanFixed(record.Certificate.Closure.CertificateID[:], closureCertificateID) ||
		!scanFixed(record.Certificate.Closure.PreviousEnvelopeDigest[:], closurePreviousEnvelopeDigest) ||
		closureKeyGeneration < 1 || closureKeyGeneration > int64(^uint32(0)) ||
		!scanFixed(record.Certificate.Closure.Nonce[:], closureNonce) ||
		!scanFixed(record.Certificate.CertificateID[:], certificateID) {
		return relay.PruneInventoryRecord{}, 0, fmt.Errorf("relay prune inventory contains invalid fixed-width metadata")
	}
	record.Certificate.Closure.KeyGeneration = uint32(closureKeyGeneration)
	record.Certificate.CertificateBytes = append([]byte(nil), certificateBytes...)
	record.CreatedAt = millisTime(createdAtMillis)
	return record, targetCount, nil
}

func readPruneInventoryTargets(ctx context.Context, tx *sql.Tx, channelID relay.ChannelID, pruneID relay.Digest, want int) ([]relay.PruneTarget, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, arrival_sequence,
       envelope_digest, certificate_id, previous_envelope_digest,
       key_generation, nonce
FROM relay_arrivals
WHERE channel_id = ? AND prune_id = ?
ORDER BY arrival_sequence`, channelID[:], pruneID[:])
	if err != nil {
		return nil, fmt.Errorf("query relay prune inventory targets: %w", err)
	}
	targets := make([]relay.PruneTarget, 0, want)
	for rows.Next() {
		var target relay.PruneTarget
		var envelopeDigest, certificateID, previousEnvelopeDigest, nonce []byte
		var keyGeneration int64
		if err := rows.Scan(
			&target.FactID,
			&target.EnvironmentID,
			&target.EnvironmentSequence,
			&target.ArrivalSequence,
			&envelopeDigest,
			&certificateID,
			&previousEnvelopeDigest,
			&keyGeneration,
			&nonce,
		); err != nil {
			return nil, closeRowsWithError(rows, fmt.Errorf("scan relay prune inventory target: %w", err), "relay prune inventory targets")
		}
		if keyGeneration < 1 || keyGeneration > int64(^uint32(0)) ||
			!scanFixed(target.EnvelopeDigest[:], envelopeDigest) || !scanFixed(target.CertificateID[:], certificateID) ||
			!scanFixed(target.PreviousEnvelopeDigest[:], previousEnvelopeDigest) || !scanFixed(target.Nonce[:], nonce) {
			return nil, closeRowsWithError(rows, fmt.Errorf("relay prune inventory target contains invalid fixed-width metadata"), "relay prune inventory targets")
		}
		target.KeyGeneration = uint32(keyGeneration)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, closeRowsWithError(rows, fmt.Errorf("iterate relay prune inventory targets: %w", err), "relay prune inventory targets")
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close relay prune inventory targets: %w", err)
	}
	if len(targets) != want {
		return nil, fmt.Errorf("relay prune inventory target count = %d, want %d", len(targets), want)
	}
	return targets, nil
}

func closeRowsWithError(rows *sql.Rows, err error, label string) error {
	if closeErr := rows.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close %s: %w", label, closeErr))
	}
	return err
}
