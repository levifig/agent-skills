package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/levifig/loaf/vnext/sync/relay"
)

func (store *Store) Tombstone(ctx context.Context, request relay.TombstoneRequest) (relay.TombstoneResult, error) {
	if err := validateContext(ctx); err != nil {
		return relay.TombstoneResult{}, err
	}
	if err := request.Validate(); err != nil {
		return relay.TombstoneResult{}, err
	}
	certificate := request.Certificate
	if request.Authorization.ChannelID != certificate.ChannelID {
		return relay.TombstoneResult{}, fmt.Errorf("%w: prune channel binding", relay.ErrInvalidArgument)
	}
	if err := store.readLock(); err != nil {
		return relay.TombstoneResult{}, err
	}
	defer store.readUnlock()

	var result relay.TombstoneResult
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		channel, err := authenticateOwner(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		existing, err := readExistingPrune(ctx, tx, certificate)
		if err != nil {
			return err
		}
		if existing {
			if err := verifyPruneClosure(ctx, tx, certificate); err != nil {
				return err
			}
			head, err := channelHead(ctx, tx, certificate.ChannelID)
			if err != nil {
				return err
			}
			result = relay.TombstoneResult{Duplicate: true, Tombstoned: len(certificate.Targets), RelayHead: head}
			return nil
		}
		if certificate.MembershipGeneration != channel.membershipGeneration {
			return relay.ErrMembershipChanged
		}
		head, err := channelHead(ctx, tx, certificate.ChannelID)
		if err != nil {
			return err
		}
		if certificate.Barrier > head {
			return relay.ErrSourceGap
		}
		if err := verifyPruneClosure(ctx, tx, certificate); err != nil {
			return err
		}
		if err := verifyPruneTargets(ctx, tx, certificate); err != nil {
			return err
		}
		pruneAuthority, err := readPruneAuthority(ctx, tx, channel.authority, certificate)
		if err != nil {
			return err
		}
		if err := store.verifier.VerifyPruneCertificate(ctx, pruneAuthority, certificate); err != nil {
			return fmt.Errorf("%w: prune certificate", relay.ErrUnverified)
		}
		currentPruneHead, err := pruneHead(ctx, tx, certificate.ChannelID)
		if err != nil {
			return err
		}
		pruneSequence, err := nextSequence(currentPruneHead)
		if err != nil {
			return fmt.Errorf("prune sequence: %w", err)
		}
		nowMillis := store.now().UTC().UnixMilli()
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relay_prune_certificates(
  channel_id,
  prune_sequence,
  prune_id,
  membership_generation,
  barrier_arrival_sequence,
  closure_fact_id,
  closure_environment_id,
  closure_environment_sequence,
  closure_arrival_sequence,
  closure_envelope_digest,
  closure_certificate_id,
  certificate_id,
  certificate_bytes,
  target_count,
  created_at_millis
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			certificate.ChannelID[:],
			pruneSequence,
			certificate.PruneID[:],
			certificate.MembershipGeneration,
			certificate.Barrier,
			string(certificate.Closure.FactID),
			string(certificate.Closure.EnvironmentID),
			certificate.Closure.EnvironmentSequence,
			certificate.Closure.ArrivalSequence,
			certificate.Closure.EnvelopeDigest[:],
			certificate.Closure.CertificateID[:],
			certificate.CertificateID[:],
			certificate.CertificateBytes,
			len(certificate.Targets),
			nowMillis,
		); err != nil {
			return fmt.Errorf("insert relay prune certificate: %w", err)
		}
		for _, target := range certificate.Targets {
			update, err := tx.ExecContext(ctx, `
UPDATE relay_arrivals
SET ciphertext = NULL, prune_id = ?, pruned_at_millis = ?
WHERE channel_id = ?
  AND arrival_sequence = ?
  AND fact_id = ?
  AND environment_id = ?
  AND environment_sequence = ?
  AND envelope_digest = ?
  AND certificate_id = ?
  AND ciphertext IS NOT NULL
  AND prune_id IS NULL`,
				certificate.PruneID[:],
				nowMillis,
				certificate.ChannelID[:],
				target.ArrivalSequence,
				string(target.FactID),
				string(target.EnvironmentID),
				target.EnvironmentSequence,
				target.EnvelopeDigest[:],
				target.CertificateID[:],
			)
			if err != nil {
				return fmt.Errorf("tombstone relay arrival: %w", err)
			}
			updated, err := update.RowsAffected()
			if err != nil {
				return fmt.Errorf("read tombstoned relay arrival count: %w", err)
			}
			if updated != 1 {
				return relay.ErrImmutableConflict
			}
		}
		result = relay.TombstoneResult{Tombstoned: len(certificate.Targets), RelayHead: head}
		return nil
	})
	if err != nil {
		return relay.TombstoneResult{}, err
	}
	return result, nil
}

func readPruneAuthority(ctx context.Context, tx *sql.Tx, channel relay.ChannelAuthority, certificate relay.PruneCertificate) (relay.PruneAuthority, error) {
	authority := relay.PruneAuthority{
		Channel:          channel,
		Environments:     make([]relay.EnvironmentAuthority, 0),
		Acknowledgements: make([]relay.Acknowledgement, 0),
	}
	rows, err := tx.QueryContext(ctx, `
SELECT e.environment_id, e.certificate_id, e.certificate_bytes, e.mode,
       e.expires_at_millis, e.token_expires_at_millis, e.membership_generation,
       COALESCE((
         SELECT MAX(a.environment_sequence)
         FROM relay_arrivals AS a
         WHERE a.channel_id = e.channel_id AND a.environment_id = e.environment_id
       ), 0),
       COALESCE((
         SELECT a.envelope_digest
         FROM relay_arrivals AS a
         WHERE a.channel_id = e.channel_id AND a.environment_id = e.environment_id
         ORDER BY a.environment_sequence DESC
         LIMIT 1
       ), zeroblob(32)),
       k.membership_generation,
       k.applied_arrival_sequence,
       k.producer_sequence,
       k.producer_envelope_digest,
       k.certificate_id,
       k.acknowledgement_digest,
       k.acknowledgement_bytes
FROM relay_environments AS e
LEFT JOIN relay_acknowledgements AS k
  ON k.channel_id = e.channel_id AND k.environment_id = e.environment_id
WHERE e.channel_id = ? AND e.retired_at_millis IS NULL
ORDER BY e.environment_id
LIMIT ?`, certificate.ChannelID[:], relay.MaxPruneAuthorityEnvironments+1)
	if err != nil {
		return relay.PruneAuthority{}, fmt.Errorf("query relay prune authority: %w", err)
	}
	for rows.Next() {
		if len(authority.Environments) == relay.MaxPruneAuthorityEnvironments {
			return relay.PruneAuthority{}, closeRowsWithError(rows, fmt.Errorf("%w: prune authority environment bound", relay.ErrInvalidArgument), "relay prune authority")
		}
		var environment relay.EnvironmentAuthority
		var acknowledgement relay.Acknowledgement
		var environmentID, mode string
		var producerHead int64
		var environmentCertificateID, certificateBytes, producerDigest []byte
		var membership sql.NullInt64
		var applied, producer sql.NullInt64
		var acknowledgementProducerDigest, acknowledgementCertificateID, acknowledgementDigest, acknowledgementBytes []byte
		if err := rows.Scan(
			&environmentID,
			&environmentCertificateID,
			&certificateBytes,
			&mode,
			&environment.ExpiresAtMillis,
			&environment.RelayTokenExpiresAtMillis,
			&environment.MembershipGeneration,
			&producerHead,
			&producerDigest,
			&membership,
			&applied,
			&producer,
			&acknowledgementProducerDigest,
			&acknowledgementCertificateID,
			&acknowledgementDigest,
			&acknowledgementBytes,
		); err != nil {
			return relay.PruneAuthority{}, closeRowsWithError(rows, fmt.Errorf("scan relay prune authority: %w", err), "relay prune authority")
		}
		if !membership.Valid || !applied.Valid || !producer.Valid ||
			membership.Int64 != int64(certificate.MembershipGeneration) ||
			applied.Int64 < certificate.Barrier || producer.Int64 != producerHead {
			return relay.PruneAuthority{}, closeRowsWithError(rows, relay.ErrAcknowledgementRequired, "relay prune authority")
		}
		environment.ChannelAuthority = channel
		environment.EnvironmentID = relay.EnvironmentID(environmentID)
		environment.CertificateBytes = append([]byte(nil), certificateBytes...)
		environment.Mode = relay.EnvironmentMode(mode)
		if !scanFixed(environment.CertificateID[:], environmentCertificateID) {
			return relay.PruneAuthority{}, closeRowsWithError(rows, fmt.Errorf("relay prune authority contains an invalid environment certificate id"), "relay prune authority")
		}
		acknowledgement = relay.Acknowledgement{
			ChannelID:              certificate.ChannelID,
			EnvironmentID:          environment.EnvironmentID,
			MembershipGeneration:   uint32(membership.Int64),
			AppliedArrivalSequence: applied.Int64,
			ProducerSequence:       producer.Int64,
			AcknowledgementBytes:   append([]byte(nil), acknowledgementBytes...),
		}
		var actualProducerDigest relay.Digest
		if !scanFixed(actualProducerDigest[:], producerDigest) ||
			!scanFixed(acknowledgement.ProducerEnvelopeDigest[:], acknowledgementProducerDigest) ||
			!scanFixed(acknowledgement.CertificateID[:], acknowledgementCertificateID) ||
			!scanFixed(acknowledgement.AcknowledgementDigest[:], acknowledgementDigest) {
			return relay.PruneAuthority{}, closeRowsWithError(rows, fmt.Errorf("relay prune authority contains invalid fixed-width acknowledgement metadata"), "relay prune authority")
		}
		if acknowledgement.CertificateID != environment.CertificateID ||
			acknowledgement.ProducerEnvelopeDigest != actualProducerDigest {
			return relay.PruneAuthority{}, closeRowsWithError(rows, relay.ErrAcknowledgementRequired, "relay prune authority")
		}
		if err := acknowledgement.Validate(); err != nil {
			return relay.PruneAuthority{}, closeRowsWithError(rows, fmt.Errorf("relay prune authority contains an invalid acknowledgement: %w", err), "relay prune authority")
		}
		authority.Environments = append(authority.Environments, environment)
		authority.Acknowledgements = append(authority.Acknowledgements, acknowledgement)
	}
	if err := rows.Err(); err != nil {
		return relay.PruneAuthority{}, closeRowsWithError(rows, fmt.Errorf("iterate relay prune authority: %w", err), "relay prune authority")
	}
	if err := rows.Close(); err != nil {
		return relay.PruneAuthority{}, fmt.Errorf("close relay prune authority: %w", err)
	}
	return authority, nil
}

func verifyPruneClosure(ctx context.Context, tx *sql.Tx, certificate relay.PruneCertificate) error {
	return verifyRetainedPruneReference(ctx, tx, certificate.ChannelID, certificate.Closure, "closure")
}

func verifyPruneTargets(ctx context.Context, tx *sql.Tx, certificate relay.PruneCertificate) error {
	for _, target := range certificate.Targets {
		if err := verifyRetainedPruneReference(ctx, tx, certificate.ChannelID, target, "target"); err != nil {
			return err
		}
		var retainedClosure int
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM relay_prune_certificates
  WHERE channel_id = ? AND closure_arrival_sequence = ?
)`, certificate.ChannelID[:], target.ArrivalSequence).Scan(&retainedClosure); err != nil {
			return fmt.Errorf("check retained relay prune closure: %w", err)
		}
		if retainedClosure != 0 {
			return relay.ErrImmutableConflict
		}
	}
	return nil
}

func verifyRetainedPruneReference(ctx context.Context, tx *sql.Tx, channelID relay.ChannelID, reference relay.PruneTarget, label string) error {
	var factID, environmentID string
	var environmentSequence, arrivalSequence int64
	var envelopeDigest, certificateID, ciphertext []byte
	var pruneID []byte
	err := tx.QueryRowContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, arrival_sequence,
       envelope_digest, certificate_id, ciphertext, prune_id
FROM relay_arrivals
WHERE channel_id = ? AND arrival_sequence = ?`,
		channelID[:], reference.ArrivalSequence,
	).Scan(
		&factID,
		&environmentID,
		&environmentSequence,
		&arrivalSequence,
		&envelopeDigest,
		&certificateID,
		&ciphertext,
		&pruneID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return relay.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read relay prune %s: %w", label, err)
	}
	if relay.FactID(factID) != reference.FactID || relay.EnvironmentID(environmentID) != reference.EnvironmentID ||
		environmentSequence != reference.EnvironmentSequence || arrivalSequence != reference.ArrivalSequence ||
		!bytes.Equal(envelopeDigest, reference.EnvelopeDigest[:]) || !bytes.Equal(certificateID, reference.CertificateID[:]) ||
		ciphertext == nil || pruneID != nil {
		return relay.ErrImmutableConflict
	}
	return nil
}

func readExistingPrune(ctx context.Context, tx *sql.Tx, candidate relay.PruneCertificate) (bool, error) {
	var pruneID, closureEnvelopeDigest, closureCertificateID, certificateID, certificateBytes []byte
	var closureFactID, closureEnvironmentID string
	var closureEnvironmentSequence, closureArrivalSequence int64
	var membership uint32
	var barrier int64
	var targetCount int
	err := tx.QueryRowContext(ctx, `
SELECT prune_id, membership_generation, barrier_arrival_sequence,
       closure_fact_id, closure_environment_id, closure_environment_sequence,
       closure_arrival_sequence, closure_envelope_digest, closure_certificate_id,
       certificate_id, certificate_bytes, target_count
FROM relay_prune_certificates
WHERE channel_id = ? AND (prune_id = ? OR certificate_id = ?)`,
		candidate.ChannelID[:], candidate.PruneID[:], candidate.CertificateID[:],
	).Scan(
		&pruneID,
		&membership,
		&barrier,
		&closureFactID,
		&closureEnvironmentID,
		&closureEnvironmentSequence,
		&closureArrivalSequence,
		&closureEnvelopeDigest,
		&closureCertificateID,
		&certificateID,
		&certificateBytes,
		&targetCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read relay prune certificate: %w", err)
	}
	if !bytes.Equal(pruneID, candidate.PruneID[:]) || membership != candidate.MembershipGeneration ||
		barrier != candidate.Barrier || relay.FactID(closureFactID) != candidate.Closure.FactID ||
		relay.EnvironmentID(closureEnvironmentID) != candidate.Closure.EnvironmentID ||
		closureEnvironmentSequence != candidate.Closure.EnvironmentSequence || closureArrivalSequence != candidate.Closure.ArrivalSequence ||
		!bytes.Equal(closureEnvelopeDigest, candidate.Closure.EnvelopeDigest[:]) ||
		!bytes.Equal(closureCertificateID, candidate.Closure.CertificateID[:]) ||
		!bytes.Equal(certificateID, candidate.CertificateID[:]) ||
		!bytes.Equal(certificateBytes, candidate.CertificateBytes) || targetCount != len(candidate.Targets) {
		return false, relay.ErrImmutableConflict
	}
	want := make(map[int64]relay.PruneTarget, len(candidate.Targets))
	for _, target := range candidate.Targets {
		want[target.ArrivalSequence] = target
	}
	rows, err := tx.QueryContext(ctx, `
SELECT fact_id, environment_id, environment_sequence, arrival_sequence,
       envelope_digest, certificate_id
FROM relay_arrivals
WHERE channel_id = ? AND prune_id = ?
ORDER BY arrival_sequence`, candidate.ChannelID[:], candidate.PruneID[:])
	if err != nil {
		return false, fmt.Errorf("query relay prune targets: %w", err)
	}
	seen := 0
	for rows.Next() {
		var factID, environmentID string
		var environmentSequence, arrivalSequence int64
		var envelopeDigest, environmentCertificateID []byte
		if err := rows.Scan(&factID, &environmentID, &environmentSequence, &arrivalSequence, &envelopeDigest, &environmentCertificateID); err != nil {
			closeErr := rows.Close()
			if closeErr != nil {
				return false, errors.Join(fmt.Errorf("scan relay existing prune target: %w", err), fmt.Errorf("close relay existing prune targets: %w", closeErr))
			}
			return false, fmt.Errorf("scan relay existing prune target: %w", err)
		}
		target, exists := want[arrivalSequence]
		if !exists || target.FactID != relay.FactID(factID) || target.EnvironmentID != relay.EnvironmentID(environmentID) ||
			target.EnvironmentSequence != environmentSequence || !bytes.Equal(target.EnvelopeDigest[:], envelopeDigest) ||
			!bytes.Equal(target.CertificateID[:], environmentCertificateID) {
			if closeErr := rows.Close(); closeErr != nil {
				return false, errors.Join(relay.ErrImmutableConflict, fmt.Errorf("close relay existing prune targets: %w", closeErr))
			}
			return false, relay.ErrImmutableConflict
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		closeErr := rows.Close()
		if closeErr != nil {
			return false, errors.Join(fmt.Errorf("iterate relay existing prune targets: %w", err), fmt.Errorf("close relay existing prune targets: %w", closeErr))
		}
		return false, fmt.Errorf("iterate relay existing prune targets: %w", err)
	}
	if err := rows.Close(); err != nil {
		return false, fmt.Errorf("close relay existing prune targets: %w", err)
	}
	if seen != len(candidate.Targets) {
		return false, relay.ErrImmutableConflict
	}
	return true, nil
}
