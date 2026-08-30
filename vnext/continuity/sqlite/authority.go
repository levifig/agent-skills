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

// SyncEnvironmentState is the bounded attach state for one requested
// environment under an exact verified authority binding.
type SyncEnvironmentState struct {
	Certificate      SyncEnvironmentCertificate
	ConsumedSequence int64
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

// SyncAuthorityBinding is the fixed-size identity and digest binding for one
// project's canonical sync authority. It intentionally excludes the complete
// environment inventory.
type SyncAuthorityBinding struct {
	ChannelID              SyncChannelID
	RelayGeneration        [32]byte
	AdminPublicKey         [32]byte
	MembershipGeneration   uint32
	InventoryArrivalHead   int64
	AuthorityDigestVersion uint16
	AuthorityDigest        [32]byte
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
		activeCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
		if err != nil {
			return SyncProgress{}, err
		}
		if activeCandidate {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before compatibility install")
		}
		_, err = readCanonicalSyncAuthorityBindingV2(ctx, tx, projectID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return SyncProgress{}, err
		default:
			return SyncProgress{}, syncProblem(SyncErrorStore, "sync_authority", "canonical authority exists without sync progress")
		}
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
		binding, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, projectID)
		if err != nil {
			return SyncProgress{}, err
		}
		if binding.AuthorityDigestVersion != 1 {
			return SyncProgress{}, syncProblem(SyncErrorConflict, "sync_authority", "compatibility install cannot reconcile a version 2 canonical authority")
		}
		persisted, err := readSyncAuthorityV1(ctx, tx, projectID)
		if err != nil {
			return SyncProgress{}, err
		}
		if !sameSyncAuthorityV1(persisted, authority) {
			activeCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
			if err != nil {
				return SyncProgress{}, err
			}
			if activeCandidate {
				return SyncProgress{}, syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before compatibility install")
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
	}

	if err := tx.Commit(); err != nil {
		return SyncProgress{}, syncProblem(SyncErrorStore, "", "sync authority commit outcome is unknown")
	}
	return progress, nil
}

func sameSyncAuthorityV1(left, right SyncAuthority) bool {
	if left.ChannelID != right.ChannelID || left.RelayGeneration != right.RelayGeneration ||
		left.AdminPublicKey != right.AdminPublicKey || left.MembershipGeneration != right.MembershipGeneration ||
		left.InventoryArrivalHead != right.InventoryArrivalHead || len(left.Environments) != len(right.Environments) {
		return false
	}
	for index, environment := range left.Environments {
		if !syncEnvironmentCertificateEqual(environment, right.Environments[index]) {
			return false
		}
	}
	return true
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
	authority, _, _, found, err := readCanonicalSyncAuthorityForCandidateV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthority{}, err
	}
	if !found {
		return SyncAuthority{}, syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthority{}, syncTransactionProblem(ctx)
	}
	return authority, nil
}

// CurrentSyncAuthorityBinding returns the project's fixed-size canonical
// authority binding without reading its environment inventory.
func (store *Store) CurrentSyncAuthorityBinding(ctx context.Context, projectID continuity.ProjectID) (SyncAuthorityBinding, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthorityBinding{}, err
	}
	if store == nil {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityBinding{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncAuthorityBinding{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	binding, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	}
	if err != nil {
		return SyncAuthorityBinding{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityBinding{}, syncTransactionProblem(ctx)
	}
	return binding, nil
}

const syncEnvironmentReceiptConsumedSequenceQueryV2 = `
SELECT environment_sequence
FROM continuity_sync_receipts
WHERE project_id = ? AND environment_id = ?
ORDER BY environment_sequence DESC
LIMIT 1`

const syncEnvironmentTombstoneConsumedSequenceQueryV2 = `
SELECT environment_sequence
FROM continuity_sync_tombstones
WHERE project_id = ? AND environment_id = ?
ORDER BY environment_sequence DESC
LIMIT 1`

// CurrentSyncEnvironmentStates returns exact point-read attach state for a
// bounded list of environments under the supplied canonical authority binding.
// Results preserve caller order and never materialize the complete authority or
// protected-history inventories.
func (store *Store) CurrentSyncEnvironmentStates(
	ctx context.Context,
	projectID continuity.ProjectID,
	verifiedAuthority SyncAuthorityBinding,
	environmentIDs []continuity.EnvironmentID,
) ([]SyncEnvironmentState, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return nil, err
	}
	if err := validateSyncAuthorityBindingV2(verifiedAuthority); err != nil {
		return nil, err
	}
	if len(environmentIDs) < 1 || len(environmentIDs) > maximumSyncAuthorityEnvironments {
		return nil, syncProblem(SyncErrorInvalid, "environment_ids", "must contain between one and 256 identities")
	}
	seenEnvironmentIDs := make(map[continuity.EnvironmentID]struct{}, len(environmentIDs))
	for _, environmentID := range environmentIDs {
		if err := environmentID.Validate(); err != nil {
			return nil, syncProblem(SyncErrorInvalid, "environment_ids", "contains an invalid environment identity")
		}
		if _, duplicate := seenEnvironmentIDs[environmentID]; duplicate {
			return nil, syncProblem(SyncErrorInvalid, "environment_ids", "must not contain duplicates")
		}
		seenEnvironmentIDs[environmentID] = struct{}{}
	}
	if store == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return nil, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return nil, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	binding, err := requireExactCanonicalSyncAuthorityBindingV2(ctx, tx, projectID, verifiedAuthority)
	if err != nil {
		return nil, err
	}
	activeCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if activeCandidate {
		return nil, syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before reading attach state")
	}

	states := make([]SyncEnvironmentState, 0, len(environmentIDs))
	seenCertificateIDs := make(map[[32]byte]struct{}, len(environmentIDs))
	seenMembershipEvents := make(map[uint32]struct{}, len(environmentIDs)*2)
	for _, environmentID := range environmentIDs {
		certificate, found, err := readCanonicalSyncEnvironmentCertificateV2(ctx, tx, projectID, binding, environmentID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, syncProblem(SyncErrorCertificate, "environment_id", "is not in pinned authority")
		}
		if _, duplicate := seenCertificateIDs[certificate.CertificateID]; duplicate {
			return nil, syncProblem(SyncErrorStore, "sync_authority", "requested environments share a certificate identity")
		}
		seenCertificateIDs[certificate.CertificateID] = struct{}{}
		if _, duplicate := seenMembershipEvents[certificate.JoinMembershipGeneration]; duplicate {
			return nil, syncProblem(SyncErrorStore, "sync_authority", "requested environments share a membership event")
		}
		seenMembershipEvents[certificate.JoinMembershipGeneration] = struct{}{}
		if certificate.Retirement != nil {
			if _, duplicate := seenMembershipEvents[certificate.Retirement.MembershipGeneration]; duplicate {
				return nil, syncProblem(SyncErrorStore, "sync_authority", "requested environments share a membership event")
			}
			seenMembershipEvents[certificate.Retirement.MembershipGeneration] = struct{}{}
		}
		consumedSequence, err := readConsumedSyncEnvironmentSequenceV2(ctx, tx, projectID, environmentID)
		if err != nil {
			return nil, err
		}
		states = append(states, SyncEnvironmentState{
			Certificate:      certificate,
			ConsumedSequence: consumedSequence,
		})
	}
	if err := tx.Commit(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	return states, nil
}

func readConsumedSyncEnvironmentSequenceV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	environmentID continuity.EnvironmentID,
) (int64, error) {
	var consumedSequence int64
	for _, query := range [...]string{
		syncEnvironmentReceiptConsumedSequenceQueryV2,
		syncEnvironmentTombstoneConsumedSequenceQueryV2,
	} {
		var sequence int64
		err := tx.QueryRowContext(ctx, query, string(projectID), string(environmentID)).Scan(&sequence)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return 0, syncTransactionProblem(ctx)
		}
		if sequence < 1 {
			return 0, syncProblem(SyncErrorStore, "sync_history", "consumed environment sequence is corrupt")
		}
		if sequence > consumedSequence {
			consumedSequence = sequence
		}
	}
	return consumedSequence, nil
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

func validateSyncAuthorityBindingV2(binding SyncAuthorityBinding) error {
	if binding.ChannelID == (SyncChannelID{}) {
		return syncProblem(SyncErrorInvalid, "channel_id", "is invalid")
	}
	if binding.RelayGeneration == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, "relay_generation", "must be nonzero")
	}
	if binding.AdminPublicKey == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, "admin_public_key", "must be nonzero")
	}
	if binding.MembershipGeneration == 0 {
		return syncProblem(SyncErrorInvalid, "membership_generation", "must begin at one")
	}
	if binding.InventoryArrivalHead < 0 {
		return syncProblem(SyncErrorInvalid, "inventory_arrival_head", "must not be negative")
	}
	if binding.AuthorityDigestVersion != 1 && binding.AuthorityDigestVersion != 2 {
		return syncProblem(SyncErrorInvalid, "authority_digest_version", "must be one or two")
	}
	if binding.AuthorityDigest == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, "authority_digest", "must be nonzero")
	}
	if binding.AuthorityDigestVersion == 1 && binding.InventoryArrivalHead != 0 {
		return syncProblem(SyncErrorInvalid, "inventory_arrival_head", "version one requires zero")
	}
	return nil
}

func readCanonicalSyncAuthorityBindingV2(ctx context.Context, tx *sql.Tx, projectID continuity.ProjectID) (SyncAuthorityBinding, error) {
	var binding SyncAuthorityBinding
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
	if errors.Is(err, sql.ErrNoRows) {
		var orphaned int
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM continuity_sync_authorities WHERE project_id = ?
  UNION ALL
  SELECT 1 FROM continuity_sync_environment_certificates WHERE project_id = ?
)`, string(projectID), string(projectID)).Scan(&orphaned); err != nil {
			return SyncAuthorityBinding{}, syncTransactionProblem(ctx)
		}
		if orphaned != 0 {
			return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "sync_authority", "orphaned canonical authority rows exist")
		}
		return SyncAuthorityBinding{}, sql.ErrNoRows
	}
	if err != nil {
		return SyncAuthorityBinding{}, syncTransactionProblem(ctx)
	}
	if len(channelID) != len(binding.ChannelID) || len(relayGeneration) != len(binding.RelayGeneration) ||
		len(adminPublicKey) != len(binding.AdminPublicKey) || membershipGeneration < 1 || membershipGeneration > math.MaxUint32 {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority binding header is corrupt")
	}
	copy(binding.ChannelID[:], channelID)
	copy(binding.RelayGeneration[:], relayGeneration)
	copy(binding.AdminPublicKey[:], adminPublicKey)
	binding.MembershipGeneration = uint32(membershipGeneration)

	var digestVersion int64
	var digestBytes []byte
	if err := tx.QueryRowContext(ctx, `
SELECT digest_version, authority_digest, inventory_arrival_head
FROM continuity_sync_authorities
WHERE project_id = ?`, string(projectID)).Scan(&digestVersion, &digestBytes, &binding.InventoryArrivalHead); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is missing")
		}
		return SyncAuthorityBinding{}, syncTransactionProblem(ctx)
	}
	if digestVersion < 0 || digestVersion > math.MaxUint16 || len(digestBytes) != len(binding.AuthorityDigest) {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority binding metadata is corrupt")
	}
	binding.AuthorityDigestVersion = uint16(digestVersion)
	copy(binding.AuthorityDigest[:], digestBytes)
	if err := validateSyncAuthorityBindingV2(binding); err != nil {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorStore, "sync_authority", "pinned authority binding is corrupt")
	}
	return binding, nil
}

const canonicalSyncEnvironmentCertificatePointQueryV2 = `
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
WHERE project_id = ? AND environment_id = ?`

func requireExactCanonicalSyncAuthorityBindingV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	expected SyncAuthorityBinding,
) (SyncAuthorityBinding, error) {
	current, err := readCanonicalSyncAuthorityBindingV2(ctx, tx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorNotFound, "project_id", "has no pinned sync authority")
	}
	if err != nil {
		return SyncAuthorityBinding{}, err
	}
	if current != expected {
		return SyncAuthorityBinding{}, syncProblem(SyncErrorConflict, "sync_authority", "does not match the pinned authority binding")
	}
	return current, nil
}

func readCanonicalSyncEnvironmentCertificateV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	binding SyncAuthorityBinding,
	environmentID continuity.EnvironmentID,
) (SyncEnvironmentCertificate, bool, error) {
	// Hot paths intentionally audit only environments touched by the bounded
	// operation. CurrentSyncAuthority remains the explicit full-inventory digest
	// and structural audit for the trusted local canonical database.
	environment, err := scanSyncEnvironmentCertificateV1(tx.QueryRowContext(
		ctx,
		canonicalSyncEnvironmentCertificatePointQueryV2,
		string(projectID),
		string(environmentID),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return SyncEnvironmentCertificate{}, false, nil
	}
	if err != nil {
		return SyncEnvironmentCertificate{}, false, err
	}
	if environment.EnvironmentID != string(environmentID) {
		return SyncEnvironmentCertificate{}, false, syncProblem(SyncErrorStore, "sync_authority", "environment certificate identity is corrupt")
	}
	if err := validateSyncAuthorityCandidateEnvironmentV2(environment, -1); err != nil {
		return SyncEnvironmentCertificate{}, false, syncProblem(SyncErrorStore, "sync_authority", "environment certificate is corrupt")
	}
	if environment.JoinMembershipGeneration > binding.MembershipGeneration {
		return SyncEnvironmentCertificate{}, false, syncProblem(SyncErrorStore, "sync_authority", "environment certificate exceeds the pinned membership generation")
	}
	if environment.Retirement != nil &&
		(environment.Retirement.RelayGeneration != binding.RelayGeneration ||
			environment.Retirement.MembershipGeneration > binding.MembershipGeneration) {
		return SyncEnvironmentCertificate{}, false, syncProblem(SyncErrorStore, "sync_authority", "environment retirement exceeds the pinned authority binding")
	}
	return environment, true, nil
}

func readCanonicalSyncEnvironmentCertificatesV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	binding SyncAuthorityBinding,
	environmentIDs []continuity.EnvironmentID,
) (map[continuity.EnvironmentID]SyncEnvironmentCertificate, error) {
	environments := make(map[continuity.EnvironmentID]SyncEnvironmentCertificate, len(environmentIDs))
	seenCertificateIDs := make(map[[32]byte]continuity.EnvironmentID, len(environmentIDs))
	seenMembershipEvents := make(map[uint32]continuity.EnvironmentID, len(environmentIDs)*2)
	for _, environmentID := range environmentIDs {
		if _, loaded := environments[environmentID]; loaded {
			continue
		}
		environment, found, err := readCanonicalSyncEnvironmentCertificateV2(ctx, tx, projectID, binding, environmentID)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, syncProblem(SyncErrorCertificate, "certificate_id", "environment is not in pinned authority")
		}
		if previous, duplicate := seenCertificateIDs[environment.CertificateID]; duplicate {
			return nil, syncProblem(SyncErrorStore, "sync_authority", fmt.Sprintf("touched environments %q and %q share a certificate identity", previous, environmentID))
		}
		seenCertificateIDs[environment.CertificateID] = environmentID
		if previous, duplicate := seenMembershipEvents[environment.JoinMembershipGeneration]; duplicate {
			return nil, syncProblem(SyncErrorStore, "sync_authority", fmt.Sprintf("touched environments %q and %q share a membership event", previous, environmentID))
		}
		seenMembershipEvents[environment.JoinMembershipGeneration] = environmentID
		if environment.Retirement != nil {
			if previous, duplicate := seenMembershipEvents[environment.Retirement.MembershipGeneration]; duplicate {
				return nil, syncProblem(SyncErrorStore, "sync_authority", fmt.Sprintf("touched environments %q and %q share a membership event", previous, environmentID))
			}
			seenMembershipEvents[environment.Retirement.MembershipGeneration] = environmentID
		}
		environments[environmentID] = environment
	}
	return environments, nil
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
	); errors.Is(err, sql.ErrNoRows) {
		return SyncEnvironmentCertificate{}, sql.ErrNoRows
	} else if err != nil {
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
