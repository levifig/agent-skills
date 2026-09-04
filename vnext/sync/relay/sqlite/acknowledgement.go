package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func (store *Store) Acknowledge(ctx context.Context, request relay.AcknowledgeRequest) error {
	if err := validateContext(ctx); err != nil {
		return err
	}
	if err := request.Authorization.Validate(); err != nil {
		return err
	}
	if err := request.Acknowledgement.Validate(); err != nil {
		return err
	}
	acknowledgement := request.Acknowledgement
	if request.Authorization.ChannelID != acknowledgement.ChannelID ||
		request.Authorization.EnvironmentID != acknowledgement.EnvironmentID ||
		request.Authorization.CertificateID != acknowledgement.CertificateID {
		return fmt.Errorf("%w: acknowledgement authorization binding", relay.ErrInvalidArgument)
	}
	if err := store.readLock(); err != nil {
		return err
	}
	defer store.readUnlock()

	return transact(ctx, store.db, func(tx *sql.Tx) error {
		environment, err := authenticateEnvironment(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}

		var existingMembership uint32
		var existingApplied, existingProducer int64
		var existingProducerDigest, existingCertificateID, existingDigest, existingBytes []byte
		var persistedConflict, persistedRollback bool
		err = tx.QueryRowContext(ctx, `
SELECT membership_generation, applied_arrival_sequence, producer_sequence, producer_envelope_digest,
       certificate_id, acknowledgement_digest, acknowledgement_bytes
FROM relay_acknowledgements
WHERE channel_id = ? AND environment_id = ?`,
			acknowledgement.ChannelID[:], string(acknowledgement.EnvironmentID),
		).Scan(
			&existingMembership,
			&existingApplied,
			&existingProducer,
			&existingProducerDigest,
			&existingCertificateID,
			&existingDigest,
			&existingBytes,
		)
		switch {
		case err == nil:
			samePosition := existingMembership == acknowledgement.MembershipGeneration &&
				existingApplied == acknowledgement.AppliedArrivalSequence &&
				existingProducer == acknowledgement.ProducerSequence
			if samePosition {
				if bytes.Equal(existingProducerDigest, acknowledgement.ProducerEnvelopeDigest[:]) &&
					bytes.Equal(existingCertificateID, acknowledgement.CertificateID[:]) &&
					bytes.Equal(existingDigest, acknowledgement.AcknowledgementDigest[:]) &&
					bytes.Equal(existingBytes, acknowledgement.AcknowledgementBytes) {
					return nil
				}
				persistedConflict = true
			}
			if existingMembership > acknowledgement.MembershipGeneration ||
				existingApplied > acknowledgement.AppliedArrivalSequence ||
				existingProducer > acknowledgement.ProducerSequence {
				persistedRollback = true
			}
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return fmt.Errorf("read relay acknowledgement: %w", err)
		}

		if err := requireActiveEnvironment(environment, store.now().UTC().UnixMilli()); err != nil {
			return err
		}
		if persistedConflict {
			return relay.ErrImmutableConflict
		}
		if persistedRollback {
			return relay.ErrRollback
		}
		if acknowledgement.MembershipGeneration != environment.currentMembership {
			return relay.ErrMembershipChanged
		}
		head, err := channelHead(ctx, tx, acknowledgement.ChannelID)
		if err != nil {
			return err
		}
		if acknowledgement.AppliedArrivalSequence > head {
			return relay.ErrSourceGap
		}
		producer, producerDigest, err := producerFence(ctx, tx, acknowledgement.ChannelID, acknowledgement.EnvironmentID)
		if err != nil {
			return err
		}
		if acknowledgement.ProducerSequence < producer {
			return relay.ErrRollback
		}
		if acknowledgement.ProducerSequence > producer {
			return relay.ErrSourceGap
		}
		if acknowledgement.ProducerEnvelopeDigest != producerDigest {
			return relay.ErrImmutableConflict
		}
		if err := store.verifier.VerifyAcknowledgement(ctx, environment.authority, acknowledgement); err != nil {
			return fmt.Errorf("%w: acknowledgement", relay.ErrUnverified)
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO relay_acknowledgements(
  channel_id,
  environment_id,
  membership_generation,
  applied_arrival_sequence,
  producer_sequence,
  producer_envelope_digest,
  certificate_id,
  acknowledgement_digest,
  acknowledgement_bytes,
  acknowledged_at_millis
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(channel_id, environment_id) DO UPDATE SET
  membership_generation = excluded.membership_generation,
  applied_arrival_sequence = excluded.applied_arrival_sequence,
  producer_sequence = excluded.producer_sequence,
  producer_envelope_digest = excluded.producer_envelope_digest,
  certificate_id = excluded.certificate_id,
  acknowledgement_digest = excluded.acknowledgement_digest,
  acknowledgement_bytes = excluded.acknowledgement_bytes,
  acknowledged_at_millis = excluded.acknowledged_at_millis`,
			acknowledgement.ChannelID[:],
			string(acknowledgement.EnvironmentID),
			acknowledgement.MembershipGeneration,
			acknowledgement.AppliedArrivalSequence,
			acknowledgement.ProducerSequence,
			acknowledgement.ProducerEnvelopeDigest[:],
			acknowledgement.CertificateID[:],
			acknowledgement.AcknowledgementDigest[:],
			acknowledgement.AcknowledgementBytes,
			store.now().UTC().UnixMilli(),
		); err != nil {
			return fmt.Errorf("write relay acknowledgement: %w", err)
		}
		return nil
	})
}
