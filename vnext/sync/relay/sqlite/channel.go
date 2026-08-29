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

func (store *Store) CreateChannel(ctx context.Context, channel relay.Channel) (relay.ChannelState, error) {
	if err := validateContext(ctx); err != nil {
		return relay.ChannelState{}, err
	}
	if err := channel.Validate(); err != nil {
		return relay.ChannelState{}, err
	}
	if channel.RelayGeneration != store.generation {
		return relay.ChannelState{}, relay.ErrGenerationMismatch
	}
	if err := store.readLock(); err != nil {
		return relay.ChannelState{}, err
	}
	defer store.readUnlock()

	createdAtMillis := store.now().UTC().UnixMilli()
	if createdAtMillis < 0 {
		return relay.ChannelState{}, fmt.Errorf("%w: current time", relay.ErrInvalidArgument)
	}
	state := relay.ChannelState{
		ChannelID:       channel.ChannelID,
		RelayGeneration: store.generation,
		CreatedAt:       millisTime(createdAtMillis),
	}
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		var adminPublicKey, ownerTokenID, ownerTokenHash []byte
		var membership uint32
		var existingCreatedAtMillis int64
		err := tx.QueryRowContext(ctx, `
SELECT admin_public_key, owner_token_id, owner_token_hash,
       membership_generation, created_at_millis
FROM relay_channels
WHERE channel_id = ?`, channel.ChannelID[:]).Scan(
			&adminPublicKey,
			&ownerTokenID,
			&ownerTokenHash,
			&membership,
			&existingCreatedAtMillis,
		)
		switch {
		case err == nil:
			if !bytes.Equal(adminPublicKey, channel.AdminPublicKey[:]) ||
				!bytes.Equal(ownerTokenID, channel.OwnerToken.TokenID[:]) ||
				!bytes.Equal(ownerTokenHash, channel.OwnerToken.TokenHash[:]) {
				return relay.ErrImmutableConflict
			}
			head, err := channelHead(ctx, tx, channel.ChannelID)
			if err != nil {
				return err
			}
			state.MembershipGeneration = membership
			state.Head = head
			state.CreatedAt = millisTime(existingCreatedAtMillis)
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("read relay channel identity: %w", err)
		}
		var tokenCount int
		if err := tx.QueryRowContext(ctx, `
SELECT (
  SELECT COUNT(*) FROM relay_channels WHERE owner_token_id = ?
) + (
  SELECT COUNT(*) FROM relay_environments WHERE token_id = ?
)`, channel.OwnerToken.TokenID[:], channel.OwnerToken.TokenID[:]).Scan(&tokenCount); err != nil {
			return fmt.Errorf("check relay owner token identity: %w", err)
		}
		if tokenCount != 0 {
			return relay.ErrImmutableConflict
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relay_channels(
  channel_id,
  admin_public_key,
  owner_token_id,
  owner_token_hash,
  membership_generation,
  created_at_millis
) VALUES(?, ?, ?, ?, 0, ?)`,
			channel.ChannelID[:],
			channel.AdminPublicKey[:],
			channel.OwnerToken.TokenID[:],
			channel.OwnerToken.TokenHash[:],
			createdAtMillis,
		); err != nil {
			return fmt.Errorf("insert relay channel: %w", err)
		}
		return nil
	})
	if err != nil {
		return relay.ChannelState{}, err
	}
	return state, nil
}

func (store *Store) RegisterEnvironment(ctx context.Context, request relay.RegisterEnvironmentRequest) (relay.ChannelState, error) {
	if err := validateContext(ctx); err != nil {
		return relay.ChannelState{}, err
	}
	if err := request.Authorization.Validate(); err != nil {
		return relay.ChannelState{}, err
	}
	if err := request.Environment.Validate(); err != nil {
		return relay.ChannelState{}, err
	}
	if request.Authorization.ChannelID != request.Environment.ChannelID {
		return relay.ChannelState{}, fmt.Errorf("%w: channel binding", relay.ErrInvalidArgument)
	}
	if err := store.readLock(); err != nil {
		return relay.ChannelState{}, err
	}
	defer store.readUnlock()

	var state relay.ChannelState
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		channel, err := authenticateOwner(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		var certificateID, certificateBytes, tokenID, tokenHash []byte
		var mode string
		var expiresAtMillis, tokenExpiresAtMillis int64
		var joinedGeneration uint32
		var retiredAt sql.NullInt64
		err = tx.QueryRowContext(ctx, `
SELECT certificate_id, certificate_bytes, mode, expires_at_millis,
       membership_generation, token_id, token_hash, token_expires_at_millis, retired_at_millis
FROM relay_environments
WHERE channel_id = ? AND environment_id = ?`,
			request.Environment.ChannelID[:], string(request.Environment.EnvironmentID),
		).Scan(
			&certificateID,
			&certificateBytes,
			&mode,
			&expiresAtMillis,
			&joinedGeneration,
			&tokenID,
			&tokenHash,
			&tokenExpiresAtMillis,
			&retiredAt,
		)
		switch {
		case err == nil:
			if !bytes.Equal(certificateID, request.Environment.CertificateID[:]) ||
				!bytes.Equal(certificateBytes, request.Environment.CertificateBytes) ||
				mode != string(request.Environment.Mode) || expiresAtMillis != request.Environment.ExpiresAtMillis ||
				joinedGeneration != request.Environment.MembershipGeneration ||
				!bytes.Equal(tokenID, request.Environment.Token.TokenID[:]) ||
				!bytes.Equal(tokenHash, request.Environment.Token.TokenHash[:]) ||
				tokenExpiresAtMillis != request.Environment.RelayTokenExpiresAtMillis {
				return relay.ErrImmutableConflict
			}
			// A lost-response retry proves the immutable registration already
			// committed, but it must not imply that a now-retired or now-expired
			// identity remains usable.
			if retiredAt.Valid {
				return relay.ErrRetired
			}
			nowMillis := store.now().UTC().UnixMilli()
			if (expiresAtMillis != 0 && nowMillis >= expiresAtMillis) ||
				(tokenExpiresAtMillis != 0 && nowMillis >= tokenExpiresAtMillis) {
				return relay.ErrExpired
			}
			head, err := channelHead(ctx, tx, request.Environment.ChannelID)
			if err != nil {
				return err
			}
			state = relay.ChannelState{
				ChannelID:            channel.authority.ChannelID,
				RelayGeneration:      channel.authority.RelayGeneration,
				MembershipGeneration: channel.membershipGeneration,
				Head:                 head,
				CreatedAt:            millisTime(channel.createdAtMillis),
			}
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("read relay environment identity: %w", err)
		}
		if channel.membershipGeneration == math.MaxUint32 || request.Environment.MembershipGeneration != channel.membershipGeneration+1 {
			return relay.ErrMembershipChanged
		}
		nowMillis := store.now().UTC().UnixMilli()
		if (request.Environment.ExpiresAtMillis != 0 && nowMillis >= request.Environment.ExpiresAtMillis) ||
			(request.Environment.RelayTokenExpiresAtMillis != 0 && nowMillis >= request.Environment.RelayTokenExpiresAtMillis) {
			return relay.ErrExpired
		}
		var conflictCount int
		if err := tx.QueryRowContext(ctx, `
SELECT (
  SELECT COUNT(*) FROM relay_environments
  WHERE (channel_id = ? AND (environment_id = ? OR certificate_id = ?)) OR token_id = ?
) + (
  SELECT COUNT(*) FROM relay_channels WHERE owner_token_id = ?
)`,
			request.Environment.ChannelID[:],
			string(request.Environment.EnvironmentID),
			request.Environment.CertificateID[:],
			request.Environment.Token.TokenID[:],
			request.Environment.Token.TokenID[:],
		).Scan(&conflictCount); err != nil {
			return fmt.Errorf("check relay environment identity: %w", err)
		}
		if conflictCount != 0 {
			return relay.ErrImmutableConflict
		}
		if err := store.verifier.VerifyEnvironmentCertificate(ctx, channel.authority, request.Environment); err != nil {
			return fmt.Errorf("%w: environment certificate", relay.ErrUnverified)
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO relay_environments(
  channel_id,
  environment_id,
  certificate_id,
  certificate_bytes,
  mode,
  expires_at_millis,
  membership_generation,
  token_id,
  token_hash,
  token_expires_at_millis,
  retired_at_millis,
  retirement_generation,
  retirement_relay_generation,
  retirement_certificate_id,
  retirement_final_environment_sequence,
  retirement_final_envelope_digest,
  retirement_id,
  retirement_bytes,
  created_at_millis
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, ?)`,
			request.Environment.ChannelID[:],
			string(request.Environment.EnvironmentID),
			request.Environment.CertificateID[:],
			request.Environment.CertificateBytes,
			string(request.Environment.Mode),
			request.Environment.ExpiresAtMillis,
			request.Environment.MembershipGeneration,
			request.Environment.Token.TokenID[:],
			request.Environment.Token.TokenHash[:],
			request.Environment.RelayTokenExpiresAtMillis,
			nowMillis,
		); err != nil {
			return fmt.Errorf("insert relay environment: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE relay_channels
SET membership_generation = ?
WHERE channel_id = ?`, request.Environment.MembershipGeneration, request.Environment.ChannelID[:]); err != nil {
			return fmt.Errorf("advance relay membership: %w", err)
		}
		head, err := channelHead(ctx, tx, request.Environment.ChannelID)
		if err != nil {
			return err
		}
		state = relay.ChannelState{
			ChannelID:            channel.authority.ChannelID,
			RelayGeneration:      channel.authority.RelayGeneration,
			MembershipGeneration: request.Environment.MembershipGeneration,
			Head:                 head,
			CreatedAt:            millisTime(channel.createdAtMillis),
		}
		return nil
	})
	if err != nil {
		return relay.ChannelState{}, err
	}
	return state, nil
}

func (store *Store) RetireEnvironment(ctx context.Context, request relay.RetireEnvironmentRequest) (relay.ChannelState, error) {
	if err := validateContext(ctx); err != nil {
		return relay.ChannelState{}, err
	}
	if err := request.Authorization.Validate(); err != nil {
		return relay.ChannelState{}, err
	}
	if err := request.Retirement.Validate(); err != nil {
		return relay.ChannelState{}, err
	}
	if request.Authorization.ChannelID != request.Retirement.ChannelID {
		return relay.ChannelState{}, fmt.Errorf("%w: channel binding", relay.ErrInvalidArgument)
	}
	if err := store.readLock(); err != nil {
		return relay.ChannelState{}, err
	}
	defer store.readUnlock()

	var state relay.ChannelState
	err := transact(ctx, store.db, func(tx *sql.Tx) error {
		channel, err := authenticateOwner(ctx, tx, request.Authorization)
		if err != nil {
			return err
		}
		var environmentCertificateID []byte
		var retiredAt sql.NullInt64
		var retirementGeneration, finalEnvironmentSequence sql.NullInt64
		var retirementRelayGeneration, retirementCertificateID, finalEnvelopeDigest, retirementID, retirementBytes []byte
		if err := tx.QueryRowContext(ctx, `
SELECT certificate_id, retired_at_millis, retirement_generation, retirement_relay_generation,
       retirement_certificate_id, retirement_final_environment_sequence,
       retirement_final_envelope_digest, retirement_id, retirement_bytes
FROM relay_environments
WHERE channel_id = ? AND environment_id = ?`,
			request.Retirement.ChannelID[:], string(request.Retirement.EnvironmentID),
		).Scan(
			&environmentCertificateID,
			&retiredAt,
			&retirementGeneration,
			&retirementRelayGeneration,
			&retirementCertificateID,
			&finalEnvironmentSequence,
			&finalEnvelopeDigest,
			&retirementID,
			&retirementBytes,
		); errors.Is(err, sql.ErrNoRows) {
			return relay.ErrNotFound
		} else if err != nil {
			return fmt.Errorf("read relay environment retirement: %w", err)
		}
		if retiredAt.Valid {
			if retirementGeneration.Valid && uint32(retirementGeneration.Int64) == request.Retirement.MembershipGeneration &&
				bytes.Equal(retirementRelayGeneration, request.Retirement.RelayGeneration[:]) &&
				bytes.Equal(retirementCertificateID, request.Retirement.CertificateID[:]) &&
				finalEnvironmentSequence.Valid && finalEnvironmentSequence.Int64 == request.Retirement.FinalEnvironmentSequence &&
				bytes.Equal(finalEnvelopeDigest, request.Retirement.FinalEnvelopeDigest[:]) &&
				bytes.Equal(retirementID, request.Retirement.RetirementID[:]) &&
				bytes.Equal(retirementBytes, request.Retirement.RetirementBytes) {
				head, err := channelHead(ctx, tx, request.Retirement.ChannelID)
				if err != nil {
					return err
				}
				state = relay.ChannelState{
					ChannelID:            channel.authority.ChannelID,
					RelayGeneration:      channel.authority.RelayGeneration,
					MembershipGeneration: channel.membershipGeneration,
					Head:                 head,
					CreatedAt:            millisTime(channel.createdAtMillis),
				}
				return nil
			}
			return relay.ErrImmutableConflict
		}
		if request.Retirement.RelayGeneration != channel.authority.RelayGeneration {
			return relay.ErrGenerationMismatch
		}
		if !bytes.Equal(environmentCertificateID, request.Retirement.CertificateID[:]) {
			return relay.ErrImmutableConflict
		}
		if channel.membershipGeneration == math.MaxUint32 || request.Retirement.MembershipGeneration != channel.membershipGeneration+1 {
			return relay.ErrMembershipChanged
		}
		producerSequence, producerDigest, err := producerFence(ctx, tx, request.Retirement.ChannelID, request.Retirement.EnvironmentID)
		if err != nil {
			return err
		}
		if producerSequence != request.Retirement.FinalEnvironmentSequence || producerDigest != request.Retirement.FinalEnvelopeDigest {
			return relay.ErrImmutableConflict
		}
		if err := store.verifier.VerifyRetirement(ctx, channel.authority, request.Retirement); err != nil {
			return fmt.Errorf("%w: retirement", relay.ErrUnverified)
		}
		nowMillis := store.now().UTC().UnixMilli()
		if _, err := tx.ExecContext(ctx, `
UPDATE relay_environments
SET retired_at_millis = ?, retirement_generation = ?, retirement_relay_generation = ?,
    retirement_certificate_id = ?, retirement_final_environment_sequence = ?,
    retirement_final_envelope_digest = ?, retirement_id = ?, retirement_bytes = ?
WHERE channel_id = ? AND environment_id = ?`,
			nowMillis,
			request.Retirement.MembershipGeneration,
			request.Retirement.RelayGeneration[:],
			request.Retirement.CertificateID[:],
			request.Retirement.FinalEnvironmentSequence,
			request.Retirement.FinalEnvelopeDigest[:],
			request.Retirement.RetirementID[:],
			request.Retirement.RetirementBytes,
			request.Retirement.ChannelID[:],
			string(request.Retirement.EnvironmentID),
		); err != nil {
			return fmt.Errorf("retire relay environment: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE relay_channels
SET membership_generation = ?
WHERE channel_id = ?`, request.Retirement.MembershipGeneration, request.Retirement.ChannelID[:]); err != nil {
			return fmt.Errorf("advance relay membership after retirement: %w", err)
		}
		head, err := channelHead(ctx, tx, request.Retirement.ChannelID)
		if err != nil {
			return err
		}
		state = relay.ChannelState{
			ChannelID:            channel.authority.ChannelID,
			RelayGeneration:      channel.authority.RelayGeneration,
			MembershipGeneration: request.Retirement.MembershipGeneration,
			Head:                 head,
			CreatedAt:            millisTime(channel.createdAtMillis),
		}
		return nil
	})
	if err != nil {
		return relay.ChannelState{}, err
	}
	return state, nil
}
