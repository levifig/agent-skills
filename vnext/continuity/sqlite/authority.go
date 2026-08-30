package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	maximumSyncAuthorityEnvironments   = 256
	maximumEnvironmentCertificateBytes = 8_192
	maximumEnvironmentRetirementBytes  = 4_096
)

// SyncEnvironmentMode identifies the retention and expiry policy carried by a
// relay environment certificate. It deliberately duplicates the small wire
// vocabulary at this persistence boundary so cryptography and relay packages do
// not become SQLite dependencies.
type SyncEnvironmentMode string

const (
	SyncEnvironmentTrusted   SyncEnvironmentMode = "trusted"
	SyncEnvironmentEphemeral SyncEnvironmentMode = "ephemeral"
)

// SyncEnvironmentRetirement is the immutable terminal fence for one
// environment. RetirementBytes is opaque canonical signed material and is
// copied on both ingress and egress.
type SyncEnvironmentRetirement struct {
	RelayGeneration          [32]byte
	MembershipGeneration     uint32
	FinalEnvironmentSequence int64
	FinalEnvelopeDigest      [32]byte
	RetirementID             [32]byte
	RetirementBytes          []byte
}

// SyncEnvironmentCertificate is the opaque verification material a
// joining client needs for one environment. CertificateBytes is retained
// exactly as received; this package does not parse or verify its contents.
type SyncEnvironmentCertificate struct {
	EnvironmentID            string
	CertificateID            [32]byte
	CertificateBytes         []byte
	Mode                     SyncEnvironmentMode
	ExpiresAtMillis          int64
	JoinMembershipGeneration uint32
	Retirement               *SyncEnvironmentRetirement
}

// SyncAuthority is the persistence-local, already-verified channel authority
// snapshot. Environment certificates are required to be strictly sorted by
// EnvironmentID and are installed as one complete inventory.
type SyncAuthority struct {
	ChannelID            SyncChannelID
	RelayGeneration      [32]byte
	AdminPublicKey       [32]byte
	MembershipGeneration uint32
	InventoryArrivalHead int64
	Environments         []SyncEnvironmentCertificate
}

// InstallVerifiedSyncAuthority pins or advances one project's verified relay
// authority and complete environment inventory in a single serializable
// transaction. The first install creates the staging sync project row, so
// pages cannot create authority-free state. Exact retries are idempotent;
// identity changes and removals are immutable conflicts.
func (store *Store) InstallVerifiedSyncAuthority(ctx context.Context, projectID continuity.ProjectID, authority SyncAuthority) (SyncProgress, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncProgress{}, err
	}
	if err := validateSyncAuthorityIdentity(authority); err != nil {
		return SyncProgress{}, err
	}
	if authority.InventoryArrivalHead != 0 {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "inventory_arrival_head", "compatibility installs require zero until paged authority staging is available")
	}
	if store == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncProgress{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncProgress{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncProgress{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return SyncProgress{}, err
	}
	if !found {
		if err := validateSyncAuthority(authority); err != nil {
			return SyncProgress{}, err
		}
		authorityDigest, err := frozenSyncAuthorityDigestV1(projectID, authority)
		if err != nil {
			return SyncProgress{}, syncProblem(SyncErrorInvalid, "sync_authority", "cannot be encoded by the frozen authority codec")
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state,
  downloaded_cursor, applied_cursor, relay_head
) VALUES(?, ?, ?, ?, ?, 'staging', 0, 0, 0)`,
			string(projectID),
			authority.ChannelID[:],
			authority.RelayGeneration[:],
			authority.AdminPublicKey[:],
			authority.MembershipGeneration,
		); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		for _, environment := range authority.Environments {
			if err := insertSyncEnvironmentCertificateV1(ctx, tx, projectID, environment); err != nil {
				return SyncProgress{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) VALUES(?, 1, ?, 0)`, string(projectID), authorityDigest[:]); err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		progress = SyncProgress{
			ProjectID:       projectID,
			ChannelID:       authority.ChannelID,
			ActivationState: SyncActivationStaging,
		}
	} else {
		persisted, err := readSyncAuthorityV1(ctx, tx, projectID)
		if err != nil {
			return SyncProgress{}, err
		}
		if err := reconcileSyncAuthorityV1(ctx, tx, projectID, persisted, authority); err != nil {
			return SyncProgress{}, err
		}
		persistedDigest, err := frozenSyncAuthorityDigestV1(projectID, persisted)
		if err != nil {
			return SyncProgress{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata cannot be rederived")
		}
		candidateDigest, err := frozenSyncAuthorityDigestV1(projectID, authority)
		if err != nil {
			return SyncProgress{}, syncProblem(SyncErrorInvalid, "sync_authority", "cannot be encoded by the frozen authority codec")
		}
		result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_authorities
SET digest_version = 1,
    authority_digest = ?,
    inventory_arrival_head = 0
WHERE project_id = ?
  AND digest_version = 1
  AND authority_digest = ?
  AND inventory_arrival_head = 0`, candidateDigest[:], string(projectID), persistedDigest[:])
		if err != nil {
			return SyncProgress{}, syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return SyncProgress{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata changed during reconciliation")
		}
	}

	if err := tx.Commit(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "sync authority commit outcome is unknown")
	}
	return progress, nil
}

// CurrentSyncAuthority returns a defensive copy of the project's pinned
// authority and its sorted environment inventory.
func (store *Store) CurrentSyncAuthority(ctx context.Context, projectID continuity.ProjectID) (SyncAuthority, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthority{}, err
	}
	if store == nil {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthority{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthority{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	authority, err := readSyncAuthorityV1(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthority{}, syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	}
	if err != nil {
		return SyncAuthority{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	return authority, nil
}

func validateSyncProjectID(projectID continuity.ProjectID) error {
	if err := projectID.Validate(); err != nil {
		return syncProblem(SyncErrorInvalid, "project_id", "is invalid")
	}
	return nil
}

func frozenSyncAuthorityDigestV1(projectID continuity.ProjectID, authority SyncAuthority) ([32]byte, error) {
	if authority.InventoryArrivalHead != 0 {
		return [32]byte{}, errors.New("frozen authority digest v1 requires inventory arrival head zero")
	}
	transcript, err := terminalCandidateAuthorityTranscriptV1(projectID, authority)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(transcript), nil
}

func validateSyncAuthority(authority SyncAuthority) error {
	if err := validateSyncAuthorityIdentity(authority); err != nil {
		return err
	}
	if len(authority.Environments) > maximumSyncAuthorityEnvironments {
		return syncProblem(SyncErrorInvalid, "environments", "exceeds the bounded inventory")
	}
	seenCertificateIDs := make(map[[32]byte]struct{}, len(authority.Environments))
	seenMembershipEvents := make(map[uint32]string, len(authority.Environments)*2)
	previousEnvironmentID := ""
	for index, environment := range authority.Environments {
		if !validOpaqueID(environment.EnvironmentID) {
			return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].environment_id", index), "is invalid")
		}
		if index > 0 && environment.EnvironmentID <= previousEnvironmentID {
			return syncProblem(SyncErrorInvalid, "environments", "must be strictly sorted without duplicates")
		}
		previousEnvironmentID = environment.EnvironmentID
		if environment.CertificateID == [32]byte{} {
			return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].certificate_id", index), "must be nonzero")
		}
		if _, duplicate := seenCertificateIDs[environment.CertificateID]; duplicate {
			return syncProblem(SyncErrorInvalid, "environments", "contains duplicate certificate identities")
		}
		seenCertificateIDs[environment.CertificateID] = struct{}{}
		if len(environment.CertificateBytes) < 1 || len(environment.CertificateBytes) > maximumEnvironmentCertificateBytes {
			return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].certificate_bytes", index), "size is outside the protocol bound")
		}
		if environment.JoinMembershipGeneration == 0 || environment.JoinMembershipGeneration > authority.MembershipGeneration {
			return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].join_membership_generation", index), "is outside the authority membership")
		}
		if previous, duplicate := seenMembershipEvents[environment.JoinMembershipGeneration]; duplicate {
			return syncProblem(SyncErrorInvalid, "environments", fmt.Sprintf("membership generation %d is claimed by both %s and environment %q join", environment.JoinMembershipGeneration, previous, environment.EnvironmentID))
		}
		seenMembershipEvents[environment.JoinMembershipGeneration] = fmt.Sprintf("environment %q join", environment.EnvironmentID)
		switch environment.Mode {
		case SyncEnvironmentTrusted:
			if environment.ExpiresAtMillis != 0 {
				return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].expires_at_millis", index), "trusted environments must not expire")
			}
		case SyncEnvironmentEphemeral:
			if environment.ExpiresAtMillis <= 0 {
				return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].expires_at_millis", index), "ephemeral environments require a positive expiry")
			}
		default:
			return syncProblem(SyncErrorInvalid, fmt.Sprintf("environments[%d].mode", index), "is invalid")
		}
		if environment.Retirement != nil {
			if err := validateSyncEnvironmentRetirement(authority, environment, *environment.Retirement, index); err != nil {
				return err
			}
			if previous, duplicate := seenMembershipEvents[environment.Retirement.MembershipGeneration]; duplicate {
				return syncProblem(SyncErrorInvalid, "environments", fmt.Sprintf("membership generation %d is claimed by both %s and environment %q retirement", environment.Retirement.MembershipGeneration, previous, environment.EnvironmentID))
			}
			seenMembershipEvents[environment.Retirement.MembershipGeneration] = fmt.Sprintf("environment %q retirement", environment.EnvironmentID)
		}
	}
	if uint64(len(seenMembershipEvents)) != uint64(authority.MembershipGeneration) {
		return syncProblem(SyncErrorInvalid, "environments", "membership event generations must exactly cover the pinned authority")
	}
	return nil
}

func validateSyncAuthorityIdentity(authority SyncAuthority) error {
	if authority.ChannelID == (SyncChannelID{}) {
		return syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if authority.RelayGeneration == [32]byte{} {
		return syncProblem(SyncErrorInvalid, "relay_generation", "must be a nonzero 32-byte value")
	}
	if authority.AdminPublicKey == [32]byte{} {
		return syncProblem(SyncErrorInvalid, "admin_public_key", "must be a nonzero 32-byte value")
	}
	if authority.MembershipGeneration == 0 {
		return syncProblem(SyncErrorInvalid, "membership_generation", "must begin at one")
	}
	if authority.InventoryArrivalHead < 0 {
		return syncProblem(SyncErrorInvalid, "inventory_arrival_head", "must not be negative")
	}
	return nil
}

func validateSyncEnvironmentRetirement(authority SyncAuthority, environment SyncEnvironmentCertificate, retirement SyncEnvironmentRetirement, index int) error {
	prefix := fmt.Sprintf("environments[%d].retirement", index)
	if retirement.RelayGeneration == [32]byte{} || retirement.RelayGeneration != authority.RelayGeneration {
		return syncProblem(SyncErrorInvalid, prefix+".relay_generation", "must match the pinned relay generation")
	}
	if retirement.MembershipGeneration == 0 || retirement.MembershipGeneration > authority.MembershipGeneration ||
		retirement.MembershipGeneration < environment.JoinMembershipGeneration {
		return syncProblem(SyncErrorInvalid, prefix+".membership_generation", "is outside the environment membership range")
	}
	if retirement.FinalEnvironmentSequence < 0 {
		return syncProblem(SyncErrorInvalid, prefix+".final_environment_sequence", "must not be negative")
	}
	if (retirement.FinalEnvironmentSequence == 0) != (retirement.FinalEnvelopeDigest == [32]byte{}) {
		return syncProblem(SyncErrorInvalid, prefix+".final_envelope_digest", "must be zero exactly when the final sequence is zero")
	}
	if retirement.RetirementID == [32]byte{} {
		return syncProblem(SyncErrorInvalid, prefix+".retirement_id", "must be nonzero")
	}
	if len(retirement.RetirementBytes) < 1 || len(retirement.RetirementBytes) > maximumEnvironmentRetirementBytes {
		return syncProblem(SyncErrorInvalid, prefix+".retirement_bytes", "size is outside the protocol bound")
	}
	return nil
}

func readSyncAuthorityV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (SyncAuthority, error) {
	authority, err := readLegacySyncAuthorityV3(ctx, tx, projectID)
	if err != nil {
		return SyncAuthority{}, err
	}
	var digestBytes []byte
	var digestVersion int64
	var inventoryArrivalHead int64
	err = tx.QueryRowContext(ctx, `
SELECT digest_version, authority_digest, inventory_arrival_head
FROM continuity_sync_authorities
WHERE project_id = ?`, string(projectID)).Scan(&digestVersion, &digestBytes, &inventoryArrivalHead)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is missing")
	}
	if err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	if digestVersion != 1 || len(digestBytes) != sha256.Size || bytes.Equal(digestBytes, make([]byte, sha256.Size)) || inventoryArrivalHead != 0 {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is corrupt")
	}
	wantDigest, err := frozenSyncAuthorityDigestV1(projectID, authority)
	if err != nil || !bytes.Equal(digestBytes, wantDigest[:]) {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is stale")
	}
	authority.InventoryArrivalHead = inventoryArrivalHead
	return authority, nil
}

func readLegacySyncAuthorityV3(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (SyncAuthority, error) {
	var authority SyncAuthority
	var channelID, relayGeneration, adminPublicKey []byte
	var membershipGeneration int64
	err := tx.QueryRowContext(ctx, `
SELECT channel_id, relay_generation, admin_public_key, membership_generation
FROM continuity_sync_projects
WHERE project_id = ?`, string(projectID)).Scan(
		&channelID,
		&relayGeneration,
		&adminPublicKey,
		&membershipGeneration,
	)
	if err != nil {
		return SyncAuthority{}, err
	}
	if len(channelID) != len(authority.ChannelID) || len(relayGeneration) != len(authority.RelayGeneration) || len(adminPublicKey) != len(authority.AdminPublicKey) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority row is corrupt")
	}
	copy(authority.ChannelID[:], channelID)
	copy(authority.RelayGeneration[:], relayGeneration)
	copy(authority.AdminPublicKey[:], adminPublicKey)
	authority.MembershipGeneration = uint32(membershipGeneration)
	rows, err := tx.QueryContext(ctx, `
SELECT
  environment_id,
  certificate_id,
  certificate_bytes,
  mode,
  expires_at_millis,
  join_membership_generation,
  retirement_relay_generation,
  retirement_membership_generation,
  retirement_final_environment_sequence,
  retirement_final_envelope_digest,
  retirement_id,
  retirement_bytes
FROM continuity_sync_environment_certificates
WHERE project_id = ?
ORDER BY environment_id`, string(projectID))
	if err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	defer rows.Close()
	for rows.Next() {
		environment, err := scanSyncEnvironmentCertificateV1(rows)
		if err != nil {
			return SyncAuthority{}, err
		}
		authority.Environments = append(authority.Environments, environment)
	}
	if err := rows.Err(); err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	if err := validateSyncAuthority(authority); err != nil {
		return SyncAuthority{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority inventory is corrupt")
	}
	return authority, nil
}

func scanSyncEnvironmentCertificateV1(scanner interface {
	Scan(dest ...any) error
}) (SyncEnvironmentCertificate, error) {
	var environment SyncEnvironmentCertificate
	var certificateID, certificateBytes, retirementRelayGeneration, retirementFinalDigest, retirementID, retirementBytes []byte
	var expiresAtMillis, joinMembershipGeneration int64
	var retirementMembershipGeneration, retirementFinalSequence sql.NullInt64
	var mode string
	if err := scanner.Scan(
		&environment.EnvironmentID,
		&certificateID,
		&certificateBytes,
		&mode,
		&expiresAtMillis,
		&joinMembershipGeneration,
		&retirementRelayGeneration,
		&retirementMembershipGeneration,
		&retirementFinalSequence,
		&retirementFinalDigest,
		&retirementID,
		&retirementBytes,
	); err != nil {
		return SyncEnvironmentCertificate{}, syncTransactionProblem(nil)
	}
	if len(certificateID) != len(environment.CertificateID) || len(certificateBytes) < 1 || len(certificateBytes) > maximumEnvironmentCertificateBytes ||
		expiresAtMillis < 0 || joinMembershipGeneration < 1 || joinMembershipGeneration > math.MaxUint32 {
		return SyncEnvironmentCertificate{}, syncProblem(SyncErrorStore, "sync_authority", "environment certificate row is corrupt")
	}
	copy(environment.CertificateID[:], certificateID)
	environment.CertificateBytes = append([]byte(nil), certificateBytes...)
	environment.Mode = SyncEnvironmentMode(mode)
	environment.ExpiresAtMillis = expiresAtMillis
	environment.JoinMembershipGeneration = uint32(joinMembershipGeneration)
	retirementPresent := retirementRelayGeneration != nil || retirementMembershipGeneration.Valid || retirementFinalSequence.Valid ||
		retirementFinalDigest != nil || retirementID != nil || retirementBytes != nil
	if retirementPresent {
		if !retirementMembershipGeneration.Valid || !retirementFinalSequence.Valid || retirementRelayGeneration == nil ||
			retirementFinalDigest == nil || retirementID == nil || retirementBytes == nil {
			return SyncEnvironmentCertificate{}, syncProblem(SyncErrorStore, "sync_authority", "environment retirement row is partial")
		}
		if retirementMembershipGeneration.Int64 < 1 || retirementMembershipGeneration.Int64 > math.MaxUint32 || retirementFinalSequence.Int64 < 0 {
			return SyncEnvironmentCertificate{}, syncProblem(SyncErrorStore, "sync_authority", "environment retirement row is corrupt")
		}
		retirement := &SyncEnvironmentRetirement{
			FinalEnvironmentSequence: retirementFinalSequence.Int64,
			MembershipGeneration:     uint32(retirementMembershipGeneration.Int64),
			RetirementBytes:          append([]byte(nil), retirementBytes...),
		}
		if len(retirementRelayGeneration) != len(retirement.RelayGeneration) || len(retirementFinalDigest) != len(retirement.FinalEnvelopeDigest) || len(retirementID) != len(retirement.RetirementID) {
			return SyncEnvironmentCertificate{}, syncProblem(SyncErrorStore, "sync_authority", "environment retirement row has invalid fixed-width fields")
		}
		copy(retirement.RelayGeneration[:], retirementRelayGeneration)
		copy(retirement.FinalEnvelopeDigest[:], retirementFinalDigest)
		copy(retirement.RetirementID[:], retirementID)
		environment.Retirement = retirement
	}
	return environment, nil
}

func insertSyncEnvironmentCertificateV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, environment SyncEnvironmentCertificate) error {
	var retirementRelayGeneration, retirementFinalDigest, retirementID, retirementBytes any
	var retirementMembershipGeneration, retirementFinalSequence any
	if environment.Retirement != nil {
		retirementRelayGeneration = environment.Retirement.RelayGeneration[:]
		retirementMembershipGeneration = environment.Retirement.MembershipGeneration
		retirementFinalSequence = environment.Retirement.FinalEnvironmentSequence
		retirementFinalDigest = environment.Retirement.FinalEnvelopeDigest[:]
		retirementID = environment.Retirement.RetirementID[:]
		retirementBytes = environment.Retirement.RetirementBytes
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_environment_certificates(
  project_id,
  environment_id,
  certificate_id,
  certificate_bytes,
  mode,
  expires_at_millis,
  join_membership_generation,
  retirement_relay_generation,
  retirement_membership_generation,
  retirement_final_environment_sequence,
  retirement_final_envelope_digest,
  retirement_id,
  retirement_bytes
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(projectID),
		environment.EnvironmentID,
		environment.CertificateID[:],
		environment.CertificateBytes,
		string(environment.Mode),
		environment.ExpiresAtMillis,
		environment.JoinMembershipGeneration,
		retirementRelayGeneration,
		retirementMembershipGeneration,
		retirementFinalSequence,
		retirementFinalDigest,
		retirementID,
		retirementBytes,
	); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func reconcileSyncAuthorityV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, current, candidate SyncAuthority) error {
	if current.ChannelID != candidate.ChannelID {
		return syncProblem(SyncErrorConflict, "channel_id", "does not match the pinned sync authority")
	}
	if current.RelayGeneration != candidate.RelayGeneration {
		return syncProblem(SyncErrorConflict, "relay_generation", "does not match the pinned sync authority")
	}
	if current.AdminPublicKey != candidate.AdminPublicKey {
		return syncProblem(SyncErrorConflict, "admin_public_key", "does not match the pinned sync authority")
	}
	if candidate.MembershipGeneration < current.MembershipGeneration {
		return syncProblem(SyncErrorConflict, "membership_generation", "regressed below the pinned authority")
	}
	currentByEnvironment := make(map[string]SyncEnvironmentCertificate, len(current.Environments))
	for _, environment := range current.Environments {
		currentByEnvironment[environment.EnvironmentID] = environment
	}
	candidateByEnvironment := make(map[string]SyncEnvironmentCertificate, len(candidate.Environments))
	for _, environment := range candidate.Environments {
		candidateByEnvironment[environment.EnvironmentID] = environment
	}
	for _, environment := range current.Environments {
		candidateEnvironment, ok := candidateByEnvironment[environment.EnvironmentID]
		if !ok {
			return syncProblem(SyncErrorConflict, "environments", "omits an already pinned environment")
		}
		if !syncEnvironmentCertificateFieldsEqual(environment, candidateEnvironment) {
			return syncProblem(SyncErrorConflict, "environment", "changes an already pinned certificate")
		}
		switch {
		case syncEnvironmentCertificateEqual(environment, candidateEnvironment):
			continue
		case environment.Retirement == nil && candidateEnvironment.Retirement != nil:
			if candidate.MembershipGeneration == current.MembershipGeneration {
				return syncProblem(SyncErrorConflict, "retirement", "requires an advancing membership generation")
			}
		default:
			return syncProblem(SyncErrorConflict, "retirement", "changes an already pinned terminal retirement")
		}
	}
	if candidate.MembershipGeneration == current.MembershipGeneration && len(candidate.Environments) != len(current.Environments) {
		return syncProblem(SyncErrorConflict, "environments", "changes inventory without advancing membership")
	}
	for _, environment := range candidate.Environments {
		if _, exists := currentByEnvironment[environment.EnvironmentID]; exists {
			continue
		}
		if candidate.MembershipGeneration == current.MembershipGeneration {
			return syncProblem(SyncErrorConflict, "environments", "adds an environment without advancing membership")
		}
	}
	if err := validateSyncAuthority(candidate); err != nil {
		return err
	}
	for _, environment := range current.Environments {
		candidateEnvironment := candidateByEnvironment[environment.EnvironmentID]
		if environment.Retirement == nil && candidateEnvironment.Retirement != nil {
			if err := updateSyncEnvironmentRetirementV1(ctx, tx, projectID, candidateEnvironment); err != nil {
				return err
			}
		}
	}
	for _, environment := range candidate.Environments {
		if _, exists := currentByEnvironment[environment.EnvironmentID]; exists {
			continue
		}
		if err := insertSyncEnvironmentCertificateV1(ctx, tx, projectID, environment); err != nil {
			return err
		}
	}
	if candidate.MembershipGeneration != current.MembershipGeneration {
		result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_projects
SET membership_generation = ?
WHERE project_id = ?`, candidate.MembershipGeneration, string(projectID))
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if err := requireOneAffectedV1(result, ctx); err != nil {
			return err
		}
	}
	return nil
}

func updateSyncEnvironmentRetirementV1(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID, environment SyncEnvironmentCertificate) error {
	retirement := environment.Retirement
	if retirement == nil {
		return syncProblem(SyncErrorStore, "sync_authority", "retirement update is missing terminal data")
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_environment_certificates
SET retirement_relay_generation = ?,
    retirement_membership_generation = ?,
    retirement_final_environment_sequence = ?,
    retirement_final_envelope_digest = ?,
    retirement_id = ?,
    retirement_bytes = ?
WHERE project_id = ? AND environment_id = ?
  AND retirement_id IS NULL`,
		retirement.RelayGeneration[:],
		retirement.MembershipGeneration,
		retirement.FinalEnvironmentSequence,
		retirement.FinalEnvelopeDigest[:],
		retirement.RetirementID[:],
		retirement.RetirementBytes,
		string(projectID),
		environment.EnvironmentID,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return err
	}
	return nil
}

func syncEnvironmentCertificateEqual(left, right SyncEnvironmentCertificate) bool {
	if !syncEnvironmentCertificateFieldsEqual(left, right) {
		return false
	}
	if (left.Retirement == nil) != (right.Retirement == nil) {
		return false
	}
	if left.Retirement == nil {
		return true
	}
	return syncEnvironmentRetirementEqual(*left.Retirement, *right.Retirement)
}

func syncEnvironmentCertificateFieldsEqual(left, right SyncEnvironmentCertificate) bool {
	return left.EnvironmentID == right.EnvironmentID && left.CertificateID == right.CertificateID &&
		bytes.Equal(left.CertificateBytes, right.CertificateBytes) && left.Mode == right.Mode &&
		left.ExpiresAtMillis == right.ExpiresAtMillis && left.JoinMembershipGeneration == right.JoinMembershipGeneration
}

func syncEnvironmentRetirementEqual(left, right SyncEnvironmentRetirement) bool {
	return left.RelayGeneration == right.RelayGeneration &&
		left.MembershipGeneration == right.MembershipGeneration &&
		left.FinalEnvironmentSequence == right.FinalEnvironmentSequence &&
		left.FinalEnvelopeDigest == right.FinalEnvelopeDigest &&
		left.RetirementID == right.RetirementID &&
		bytes.Equal(left.RetirementBytes, right.RetirementBytes)
}
