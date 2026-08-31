package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

const maximumSyncPruneWitnessEnvironments = maximumSyncAuthorityEnvironments

// SyncPruneWitnessAuthority is the bounded set of environments that were
// active at one historical membership generation under an exact authority
// binding. Environments remains strictly sorted by EnvironmentID.
type SyncPruneWitnessAuthority struct {
	Binding              SyncAuthorityBinding
	MembershipGeneration uint32
	Environments         []SyncEnvironmentCertificate
}

// CurrentSyncPruneWitnessAuthorityUnderBinding audits the complete canonical
// authority while retaining only the bounded witness set active at the
// requested historical membership generation. The returned snapshot is not a
// lease; a later mutation must compare the exact binding again.
func (store *Store) CurrentSyncPruneWitnessAuthorityUnderBinding(
	ctx context.Context,
	projectID continuity.ProjectID,
	verifiedAuthority SyncAuthorityBinding,
	membershipGeneration uint32,
) (SyncPruneWitnessAuthority, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	if err := validateSyncAuthorityBindingV2(verifiedAuthority); err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	if membershipGeneration == 0 || membershipGeneration > verifiedAuthority.MembershipGeneration {
		return SyncPruneWitnessAuthority{}, syncProblem(SyncErrorInvalid, "membership_generation", "is outside the verified authority binding")
	}
	if store == nil {
		return SyncPruneWitnessAuthority{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncPruneWitnessAuthority{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncPruneWitnessAuthority{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncPruneWitnessAuthority{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncPruneWitnessAuthority{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	binding, err := requireExactCanonicalSyncAuthorityBindingV2(ctx, tx, projectID, verifiedAuthority)
	if err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	if err := requireKnownExactSyncRelayWatermarkV1(
		ctx, tx, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
	); err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	activeCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
	if err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	if activeCandidate {
		return SyncPruneWitnessAuthority{}, syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before reading prune witnesses")
	}

	var environments []SyncEnvironmentCertificate
	switch binding.AuthorityDigestVersion {
	case 1:
		environments, err = readSyncPruneWitnessAuthorityV1(ctx, tx, projectID, binding, membershipGeneration)
	case 2:
		environments, err = readSyncPruneWitnessAuthorityV2(ctx, tx, projectID, binding, membershipGeneration)
	default:
		err = syncProblem(SyncErrorStore, "sync_authority", "pinned authority digest version is unsupported")
	}
	if err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	if err := ctx.Err(); err != nil {
		return SyncPruneWitnessAuthority{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyncPruneWitnessAuthority{}, syncTransactionProblem(ctx)
	}
	return SyncPruneWitnessAuthority{
		Binding:              binding,
		MembershipGeneration: membershipGeneration,
		Environments:         environments,
	}, nil
}

func readSyncPruneWitnessAuthorityV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	binding SyncAuthorityBinding,
	membershipGeneration uint32,
) ([]SyncEnvironmentCertificate, error) {
	authority, digestVersion, digest, found, err := readCanonicalSyncAuthorityForCandidateV2(ctx, tx, projectID)
	if err != nil {
		return nil, err
	}
	if !found || digestVersion != binding.AuthorityDigestVersion || digest != binding.AuthorityDigest ||
		authority.ChannelID != binding.ChannelID || authority.RelayGeneration != binding.RelayGeneration ||
		authority.AdminPublicKey != binding.AdminPublicKey || authority.MembershipGeneration != binding.MembershipGeneration ||
		authority.InventoryArrivalHead != binding.InventoryArrivalHead {
		return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned version one authority does not match its binding")
	}

	witnesses := make([]SyncEnvironmentCertificate, 0, len(authority.Environments))
	for _, environment := range authority.Environments {
		if environment.JoinMembershipGeneration <= membershipGeneration &&
			(environment.Retirement == nil || environment.Retirement.MembershipGeneration > membershipGeneration) {
			witnesses = append(witnesses, environment)
		}
	}
	return witnesses, nil
}

func readSyncPruneWitnessAuthorityV2(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	binding SyncAuthorityBinding,
	membershipGeneration uint32,
) ([]SyncEnvironmentCertificate, error) {
	headerDigest, err := syncAuthorityHeaderDigestV2(projectID, SyncAuthoritySnapshot{
		ChannelID:            binding.ChannelID,
		RelayGeneration:      binding.RelayGeneration,
		AdminPublicKey:       binding.AdminPublicKey,
		MembershipGeneration: binding.MembershipGeneration,
		InventoryArrivalHead: binding.InventoryArrivalHead,
	})
	if err != nil {
		return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority header cannot be encoded")
	}
	rolling, err := syncAuthorityCandidateRollingSeedV2(headerDigest)
	if err != nil {
		return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority digest cannot be initialized")
	}
	rows, err := tx.QueryContext(ctx, canonicalSyncAuthorityInventoryQueryV2, string(projectID))
	if err != nil {
		return nil, syncTransactionProblem(ctx)
	}

	witnesses := make([]SyncEnvironmentCertificate, 0, maximumSyncPruneWitnessEnvironments)
	previousEnvironmentID := ""
	var environmentCount int64
	witnessOverflow := false
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		environment, err := scanSyncEnvironmentCertificateV1(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if err := validateSyncAuthorityCandidateEnvironmentV2(environment, 0); err != nil ||
			environment.EnvironmentID <= previousEnvironmentID ||
			environment.JoinMembershipGeneration > binding.MembershipGeneration ||
			(environment.Retirement != nil && (environment.Retirement.RelayGeneration != binding.RelayGeneration ||
				environment.Retirement.MembershipGeneration > binding.MembershipGeneration)) {
			rows.Close()
			return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority inventory is corrupt")
		}
		environmentCount, err = checkedSyncAuthorityCandidateAdvanceV2(environmentCount, 1)
		if err != nil {
			rows.Close()
			return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority inventory overflows")
		}
		rolling, _, err = advanceSyncAuthorityCandidateRollingV2(headerDigest, rolling, environmentCount, environment)
		if err != nil {
			rows.Close()
			return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority environment cannot be encoded")
		}
		if environment.JoinMembershipGeneration <= membershipGeneration &&
			(environment.Retirement == nil || environment.Retirement.MembershipGeneration > membershipGeneration) {
			if len(witnesses) == maximumSyncPruneWitnessEnvironments {
				witnessOverflow = true
			} else if !witnessOverflow {
				witnesses = append(witnesses, environment)
			}
		}
		previousEnvironmentID = environment.EnvironmentID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, syncTransactionProblem(ctx)
	}
	if err := rows.Close(); err != nil {
		return nil, syncTransactionProblem(ctx)
	}
	if environmentCount < 1 {
		return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority inventory is empty")
	}
	digest, err := finalizeSyncAuthorityDigestV2(headerDigest, environmentCount, rolling)
	if err != nil || digest != binding.AuthorityDigest {
		return nil, syncProblem(SyncErrorStore, "sync_authority", "pinned authority metadata is stale")
	}
	// Canonical v2 promotion validates exact membership-event coverage before
	// persisting this digest. Re-sorting lifetime history here would violate the
	// bounded recovery read; the exact digest above binds every promoted row.
	if witnessOverflow {
		return nil, syncProblem(SyncErrorConflict, "prune_witness_authority", "exceeds the bounded active environment set")
	}
	return witnesses, nil
}
