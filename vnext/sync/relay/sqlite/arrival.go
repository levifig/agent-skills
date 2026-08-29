package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func (store *Store) Append(ctx context.Context, request relay.AppendRequest) (relay.AppendResult, error) {
	if err := validateContext(ctx); err != nil {
		return relay.AppendResult{}, err
	}
	if err := request.Authorization.Validate(); err != nil {
		return relay.AppendResult{}, err
	}
	if err := request.Envelope.Validate(); err != nil {
		return relay.AppendResult{}, err
	}
	if request.Authorization.ChannelID != request.Envelope.ChannelID ||
		request.Authorization.EnvironmentID != request.Envelope.EnvironmentID ||
		request.Authorization.CertificateID != request.Envelope.CertificateID {
		return relay.AppendResult{}, fmt.Errorf("%w: envelope authorization binding", relay.ErrInvalidArgument)
	}
	if err := store.readLock(); err != nil {
		return relay.AppendResult{}, err
	}
	defer store.readUnlock()

	var result relay.AppendResult
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		environment, err := authenticateEnvironment(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		if err := requireActiveEnvironment(environment, store.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if err := store.verifier.VerifyEnvelope(ctx, environment.authority, request.Envelope); err != nil {
			return fmt.Errorf("%w: envelope", relay.ErrUnverified)
		}
		existing, err := findArrivalCollisions(ctx, tx, request.Envelope)
		if err != nil {
			return err
		}
		if len(existing) != 0 {
			for _, arrival := range existing {
				if !sameEnvelopeIdentity(arrival.Envelope, request.Envelope) {
					return relay.ErrImmutableConflict
				}
			}
			head, err := channelHead(ctx, tx, request.Envelope.ChannelID)
			if err != nil {
				return err
			}
			result = relay.AppendResult{Disposition: relay.AppendDuplicate, Arrival: existing[0], RelayHead: head}
			return nil
		}
		producer, err := producerHead(ctx, tx, request.Envelope.ChannelID, request.Envelope.EnvironmentID)
		if err != nil {
			return err
		}
		nextProducer, err := nextSequence(producer)
		if err != nil {
			return fmt.Errorf("source sequence: %w", err)
		}
		switch {
		case request.Envelope.EnvironmentSequence > nextProducer:
			return relay.ErrSourceGap
		case request.Envelope.EnvironmentSequence <= producer:
			return relay.ErrImmutableConflict
		}
		if request.Envelope.EnvironmentSequence > 1 {
			var previousDigest []byte
			if err := tx.QueryRowContext(ctx, `
SELECT envelope_digest
FROM relay_arrivals
WHERE channel_id = ? AND environment_id = ? AND environment_sequence = ?`,
				request.Envelope.ChannelID[:],
				string(request.Envelope.EnvironmentID),
				request.Envelope.EnvironmentSequence-1,
			).Scan(&previousDigest); err != nil {
				return fmt.Errorf("read previous relay envelope: %w", err)
			}
			if !bytes.Equal(previousDigest, request.Envelope.PreviousEnvelopeDigest[:]) {
				return relay.ErrPreviousDigest
			}
		}
		var nonceCount int
		if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM relay_arrivals
WHERE channel_id = ? AND key_generation = ? AND nonce = ?`,
			request.Envelope.ChannelID[:], request.Envelope.KeyGeneration, request.Envelope.Nonce[:],
		).Scan(&nonceCount); err != nil {
			return fmt.Errorf("check relay nonce reuse: %w", err)
		}
		if nonceCount != 0 {
			return relay.ErrNonceReuse
		}
		head, err := channelHead(ctx, tx, request.Envelope.ChannelID)
		if err != nil {
			return err
		}
		arrivalSequence, err := nextSequence(head)
		if err != nil {
			return fmt.Errorf("arrival sequence: %w", err)
		}
		arrivedAtMillis := store.now().UTC().UnixMilli()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relay_arrivals(
  channel_id,
  arrival_sequence,
  fact_id,
  environment_id,
  environment_sequence,
  protocol_version,
  cipher_suite,
  key_generation,
  previous_envelope_digest,
  certificate_id,
  nonce,
  envelope_digest,
  ciphertext,
  ciphertext_size,
  signature,
  arrived_at_millis,
  prune_id,
  pruned_at_millis
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
			request.Envelope.ChannelID[:],
			arrivalSequence,
			string(request.Envelope.FactID),
			string(request.Envelope.EnvironmentID),
			request.Envelope.EnvironmentSequence,
			request.Envelope.ProtocolVersion,
			request.Envelope.CipherSuite,
			request.Envelope.KeyGeneration,
			request.Envelope.PreviousEnvelopeDigest[:],
			request.Envelope.CertificateID[:],
			request.Envelope.Nonce[:],
			request.Envelope.EnvelopeDigest[:],
			request.Envelope.Ciphertext,
			len(request.Envelope.Ciphertext),
			request.Envelope.Signature[:],
			arrivedAtMillis,
		); err != nil {
			return fmt.Errorf("insert relay arrival: %w", err)
		}
		arrival := relay.Arrival{
			Envelope:        cloneEnvelope(request.Envelope),
			ArrivalSequence: arrivalSequence,
			CiphertextSize:  int64(len(request.Envelope.Ciphertext)),
			ArrivedAt:       millisTime(arrivedAtMillis),
		}
		result = relay.AppendResult{Disposition: relay.AppendAccepted, Arrival: arrival, RelayHead: arrivalSequence}
		return nil
	})
	if err != nil {
		return relay.AppendResult{}, err
	}
	return result, nil
}

func (store *Store) Page(ctx context.Context, request relay.PageRequest) (relay.Page, error) {
	if err := validateContext(ctx); err != nil {
		return relay.Page{}, err
	}
	if err := request.Validate(); err != nil {
		return relay.Page{}, err
	}
	if err := store.readLock(); err != nil {
		return relay.Page{}, err
	}
	defer store.readUnlock()

	var page relay.Page
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		environment, err := authenticateEnvironment(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		if err := requireActiveEnvironment(environment, store.now().UTC().UnixMilli()); err != nil {
			return err
		}
		head, err := channelHead(ctx, tx, request.Authorization.ChannelID)
		if err != nil {
			return err
		}
		if request.After > head {
			return relay.ErrRollback
		}
		page = relay.Page{
			RelayGeneration:      environment.authority.RelayGeneration,
			MembershipGeneration: environment.currentMembership,
			Head:                 head,
			Arrivals:             make([]relay.Arrival, 0),
		}
		rows, err := tx.QueryContext(ctx, arrivalSelectSQL+`
WHERE channel_id = ? AND arrival_sequence > ? AND arrival_sequence <= ?
ORDER BY arrival_sequence
LIMIT ?`, request.Authorization.ChannelID[:], request.After, head, request.Limit)
		if err != nil {
			return fmt.Errorf("query relay arrival page: %w", err)
		}
		for rows.Next() {
			arrival, err := scanArrival(rows)
			if err != nil {
				closeErr := rows.Close()
				if closeErr != nil {
					return errors.Join(err, fmt.Errorf("close relay arrival page: %w", closeErr))
				}
				return err
			}
			page.Arrivals = append(page.Arrivals, arrival)
		}
		if err := rows.Err(); err != nil {
			closeErr := rows.Close()
			if closeErr != nil {
				return errors.Join(fmt.Errorf("iterate relay arrival page: %w", err), fmt.Errorf("close relay arrival page: %w", closeErr))
			}
			return fmt.Errorf("iterate relay arrival page: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close relay arrival page: %w", err)
		}
		return nil
	})
	if err != nil {
		return relay.Page{}, err
	}
	return page, nil
}

const arrivalSelectSQL = `
SELECT
  channel_id,
  arrival_sequence,
  fact_id,
  environment_id,
  environment_sequence,
  protocol_version,
  cipher_suite,
  key_generation,
  previous_envelope_digest,
  certificate_id,
  nonce,
  envelope_digest,
  ciphertext,
  ciphertext_size,
  signature,
  arrived_at_millis,
  prune_id,
  pruned_at_millis
FROM relay_arrivals`

type rowScanner interface {
	Scan(...any) error
}

func scanArrival(scanner rowScanner) (relay.Arrival, error) {
	var arrival relay.Arrival
	var channelID, previousDigest, certificateID, nonce, envelopeDigest, signature []byte
	var ciphertext, pruneID []byte
	var arrivedAtMillis int64
	var prunedAtMillis sql.NullInt64
	if err := scanner.Scan(
		&channelID,
		&arrival.ArrivalSequence,
		&arrival.FactID,
		&arrival.EnvironmentID,
		&arrival.EnvironmentSequence,
		&arrival.ProtocolVersion,
		&arrival.CipherSuite,
		&arrival.KeyGeneration,
		&previousDigest,
		&certificateID,
		&nonce,
		&envelopeDigest,
		&ciphertext,
		&arrival.CiphertextSize,
		&signature,
		&arrivedAtMillis,
		&pruneID,
		&prunedAtMillis,
	); err != nil {
		return relay.Arrival{}, fmt.Errorf("scan relay arrival: %w", err)
	}
	if !scanFixed(arrival.ChannelID[:], channelID) ||
		!scanFixed(arrival.PreviousEnvelopeDigest[:], previousDigest) ||
		!scanFixed(arrival.CertificateID[:], certificateID) ||
		!scanFixed(arrival.Nonce[:], nonce) ||
		!scanFixed(arrival.EnvelopeDigest[:], envelopeDigest) ||
		!scanFixed(arrival.Signature[:], signature) {
		return relay.Arrival{}, fmt.Errorf("relay arrival contains invalid fixed-width metadata")
	}
	arrival.Ciphertext = append([]byte(nil), ciphertext...)
	if ciphertext == nil {
		arrival.Ciphertext = nil
	}
	arrival.ArrivedAt = millisTime(arrivedAtMillis)
	if pruneID != nil {
		var value relay.Digest
		if !scanFixed(value[:], pruneID) {
			return relay.Arrival{}, fmt.Errorf("relay arrival contains invalid prune id")
		}
		arrival.PruneID = &value
	}
	if prunedAtMillis.Valid {
		value := millisTime(prunedAtMillis.Int64)
		arrival.PrunedAt = &value
	}
	return arrival, nil
}

func findArrivalCollisions(ctx context.Context, tx *sql.Tx, envelope relay.Envelope) ([]relay.Arrival, error) {
	rows, err := tx.QueryContext(ctx, arrivalSelectSQL+`
WHERE channel_id = ?
  AND (
    fact_id = ?
    OR (environment_id = ? AND environment_sequence = ?)
    OR envelope_digest = ?
  )
ORDER BY arrival_sequence`,
		envelope.ChannelID[:],
		string(envelope.FactID),
		string(envelope.EnvironmentID),
		envelope.EnvironmentSequence,
		envelope.EnvelopeDigest[:],
	)
	if err != nil {
		return nil, fmt.Errorf("query relay arrival conflicts: %w", err)
	}
	arrivals := make([]relay.Arrival, 0, 1)
	for rows.Next() {
		arrival, err := scanArrival(rows)
		if err != nil {
			closeErr := rows.Close()
			if closeErr != nil {
				return nil, errors.Join(err, fmt.Errorf("close relay arrival conflicts: %w", closeErr))
			}
			return nil, err
		}
		arrivals = append(arrivals, arrival)
	}
	if err := rows.Err(); err != nil {
		closeErr := rows.Close()
		if closeErr != nil {
			return nil, errors.Join(fmt.Errorf("iterate relay arrival conflicts: %w", err), fmt.Errorf("close relay arrival conflicts: %w", closeErr))
		}
		return nil, fmt.Errorf("iterate relay arrival conflicts: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close relay arrival conflicts: %w", err)
	}
	return arrivals, nil
}

func sameEnvelopeIdentity(stored, candidate relay.Envelope) bool {
	return stored.ChannelID == candidate.ChannelID &&
		stored.FactID == candidate.FactID &&
		stored.EnvironmentID == candidate.EnvironmentID &&
		stored.EnvironmentSequence == candidate.EnvironmentSequence &&
		stored.EnvelopeDigest == candidate.EnvelopeDigest
}

func cloneEnvelope(envelope relay.Envelope) relay.Envelope {
	clone := envelope
	clone.Ciphertext = append([]byte(nil), envelope.Ciphertext...)
	return clone
}

func nextSequence(current int64) (int64, error) {
	if current < 0 || current == math.MaxInt64 {
		return 0, fmt.Errorf("%w: sequence exhausted", relay.ErrInvalidArgument)
	}
	return current + 1, nil
}
