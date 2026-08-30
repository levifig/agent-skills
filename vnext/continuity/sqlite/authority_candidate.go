package sqlite

import (
	"context"
	"database/sql"

	"github.com/levifig/loaf/vnext/continuity"
)

// SyncAuthoritySnapshot is the immutable, already-verified header of one
// paged authority candidate. The base fields are either both zero (bootstrap)
// or an exact nonzero version-and-digest pair for the currently pinned
// authority.
type SyncAuthoritySnapshot struct {
	ChannelID                  SyncChannelID
	RelayGeneration            [32]byte
	AdminPublicKey             [32]byte
	MembershipGeneration       uint32
	InventoryArrivalHead       int64
	BaseAuthorityDigestVersion uint16
	BaseAuthorityDigest        [32]byte
}

// SyncAuthorityPage is one bounded, already-verified slice of an authority
// inventory. ThroughEnvironmentID must equal the final environment ID. The
// empty AfterEnvironmentID denotes the first page.
type SyncAuthorityPage struct {
	AfterEnvironmentID   string
	ThroughEnvironmentID string
	Environments         []SyncEnvironmentCertificate
	More                 bool
}

// SyncAuthorityCandidate is the fixed-size durable checkpoint for the one
// active paged authority candidate of a project. It deliberately exposes no
// aggregate environment slice.
type SyncAuthorityCandidate struct {
	ProjectID                continuity.ProjectID
	CandidateID              [32]byte
	Snapshot                 SyncAuthoritySnapshot
	PageCount                int64
	EnvironmentCount         int64
	ThroughEnvironmentID     string
	RollingEnvironmentDigest [32]byte
	Ready                    bool
	AuthorityDigestVersion   uint16
	AuthorityDigest          [32]byte
}

// SyncAuthorityCandidateCheckpoint is the exact compare-and-swap token
// required to discard an active candidate without deleting a newer resume.
type SyncAuthorityCandidateCheckpoint struct {
	CandidateID              [32]byte
	PageCount                int64
	EnvironmentCount         int64
	ThroughEnvironmentID     string
	RollingEnvironmentDigest [32]byte
	Ready                    bool
	AuthorityDigest          [32]byte
}

// Checkpoint returns the exact compare-and-swap token for candidate.
func (candidate SyncAuthorityCandidate) Checkpoint() SyncAuthorityCandidateCheckpoint {
	return SyncAuthorityCandidateCheckpoint{
		CandidateID:              candidate.CandidateID,
		PageCount:                candidate.PageCount,
		EnvironmentCount:         candidate.EnvironmentCount,
		ThroughEnvironmentID:     candidate.ThroughEnvironmentID,
		RollingEnvironmentDigest: candidate.RollingEnvironmentDigest,
		Ready:                    candidate.Ready,
		AuthorityDigest:          candidate.AuthorityDigest,
	}
}

// StageVerifiedSyncAuthorityCandidatePage creates, resumes, or exactly replays
// one bounded page of an already-verified authority snapshot. It never changes
// the canonical authority or any sync data plane table.
func (store *Store) StageVerifiedSyncAuthorityCandidatePage(
	ctx context.Context,
	projectID continuity.ProjectID,
	snapshot SyncAuthoritySnapshot,
	page SyncAuthorityPage,
) (SyncAuthorityCandidate, error) {
	prepared, err := prepareSyncAuthorityCandidatePageV2(projectID, snapshot, page)
	if err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if store == nil {
		return SyncAuthorityCandidate{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityCandidate{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityCandidate{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityCandidate{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncAuthorityCandidate{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()

	canonicalBase, err := readCanonicalSyncAuthorityBaseV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if err := validateSyncAuthorityCandidateBaseV2(snapshot, canonicalBase.digestVersion, canonicalBase.digest, canonicalBase.found); err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if err := validateSyncAuthorityCandidateHeaderAgainstCanonicalV2(snapshot, canonicalBase); err != nil {
		return SyncAuthorityCandidate{}, err
	}
	candidateID, headerDigest, err := deriveSyncAuthorityCandidateIdentityV2(projectID, snapshot)
	if err != nil {
		return SyncAuthorityCandidate{}, syncProblem(SyncErrorInvalid, "snapshot", "cannot be encoded by the authority candidate codec")
	}
	current, active, err := readActiveSyncAuthorityCandidateHeaderV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if active {
		if current.candidate.CandidateID != candidateID || current.candidate.Snapshot != snapshot || current.headerDigest != headerDigest {
			return SyncAuthorityCandidate{}, syncProblem(SyncErrorConflict, "snapshot", "does not match the active authority candidate")
		}
		if current.candidate.Ready {
			current, active, err = readAndValidateActiveSyncAuthorityCandidateV2(ctx, tx, projectID)
			if err != nil {
				return SyncAuthorityCandidate{}, err
			}
			if !active {
				return SyncAuthorityCandidate{}, corruptSyncAuthorityCandidateV2("ready candidate disappeared during validation")
			}
			if err := validateCanonicalSyncAuthorityForCandidateV2(ctx, tx, projectID, canonicalBase); err != nil {
				return SyncAuthorityCandidate{}, err
			}
			if err := validateFullSyncAuthorityCandidateAgainstCanonicalV2(ctx, tx, current, canonicalBase); err != nil {
				return SyncAuthorityCandidate{}, err
			}
		}
		replayed, exact, err := exactSyncAuthorityCandidatePageReplayV2(ctx, tx, current, prepared)
		if err != nil {
			return SyncAuthorityCandidate{}, err
		}
		if replayed {
			if !exact {
				return SyncAuthorityCandidate{}, syncProblem(SyncErrorConflict, "page", "changes an already staged authority page")
			}
			if err := validateSyncAuthorityCandidatePageAgainstCanonicalV2(ctx, tx, projectID, current.candidate.Snapshot, prepared, canonicalBase, !prepared.More); err != nil {
				return SyncAuthorityCandidate{}, err
			}
			if err := tx.Commit(); err != nil {
				return SyncAuthorityCandidate{}, syncTransactionProblem(ctx)
			}
			return current.candidate, nil
		}
		if current.candidate.Ready {
			return SyncAuthorityCandidate{}, syncProblem(SyncErrorConflict, "page", "ready authority candidate is immutable")
		}
		if prepared.AfterEnvironmentID != current.candidate.ThroughEnvironmentID {
			return SyncAuthorityCandidate{}, syncProblem(SyncErrorConflict, "after_environment_id", "is not the active candidate cursor")
		}
		if err := validateSyncAuthorityCandidatePageAgainstCanonicalV2(ctx, tx, projectID, snapshot, prepared, canonicalBase, !prepared.More); err != nil {
			return SyncAuthorityCandidate{}, err
		}
		if err := appendSyncAuthorityCandidatePageV2(ctx, tx, current, prepared, headerDigest); err != nil {
			return SyncAuthorityCandidate{}, err
		}
	} else {
		if prepared.AfterEnvironmentID != "" {
			return SyncAuthorityCandidate{}, syncProblem(SyncErrorInvalid, "after_environment_id", "must be empty for the first page")
		}
		if err := validateSyncAuthorityCandidatePageAgainstCanonicalV2(ctx, tx, projectID, snapshot, prepared, canonicalBase, !prepared.More); err != nil {
			return SyncAuthorityCandidate{}, err
		}
		if err := insertFirstSyncAuthorityCandidatePageV2(ctx, tx, projectID, candidateID, snapshot, prepared, headerDigest); err != nil {
			return SyncAuthorityCandidate{}, err
		}
	}

	var next persistedSyncAuthorityCandidateV2
	var found bool
	if !prepared.More {
		next, found, err = readAndValidateActiveSyncAuthorityCandidateV2(ctx, tx, projectID)
	} else {
		next, found, err = readActiveSyncAuthorityCandidateHeaderV2(ctx, tx, projectID)
	}
	if err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if !found || next.candidate.CandidateID != candidateID {
		return SyncAuthorityCandidate{}, syncProblem(SyncErrorStore, "sync_authority_candidate", "candidate disappeared during staging")
	}
	canonicalBase, err = readCanonicalSyncAuthorityBaseV2(ctx, tx, projectID)
	if err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if err := validateSyncAuthorityCandidateBaseV2(snapshot, canonicalBase.digestVersion, canonicalBase.digest, canonicalBase.found); err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if err := validateSyncAuthorityCandidateHeaderAgainstCanonicalV2(snapshot, canonicalBase); err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if err := validateSyncAuthorityCandidatePageAgainstCanonicalV2(ctx, tx, projectID, snapshot, prepared, canonicalBase, !prepared.More); err != nil {
		return SyncAuthorityCandidate{}, err
	}
	if !prepared.More {
		if err := validateCanonicalSyncAuthorityForCandidateV2(ctx, tx, projectID, canonicalBase); err != nil {
			return SyncAuthorityCandidate{}, err
		}
		if err := validateFullSyncAuthorityCandidateAgainstCanonicalV2(ctx, tx, next, canonicalBase); err != nil {
			return SyncAuthorityCandidate{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityCandidate{}, syncProblem(SyncErrorStore, "", "authority candidate page commit outcome is unknown")
	}
	return next.candidate, nil
}

// CurrentSyncAuthorityCandidate returns the fixed-size active staging or ready
// checkpoint after fully revalidating its persisted page stream.
func (store *Store) CurrentSyncAuthorityCandidate(ctx context.Context, projectID continuity.ProjectID) (SyncAuthorityCandidate, bool, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncAuthorityCandidate{}, false, err
	}
	if store == nil {
		return SyncAuthorityCandidate{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncAuthorityCandidate{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncAuthorityCandidate{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncAuthorityCandidate{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncAuthorityCandidate{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	candidate, found, err := readAndValidateActiveSyncAuthorityCandidateV2(ctx, tx, projectID)
	if err != nil || !found {
		return SyncAuthorityCandidate{}, found, err
	}
	if err := tx.Commit(); err != nil {
		return SyncAuthorityCandidate{}, false, syncTransactionProblem(ctx)
	}
	return candidate.candidate, true, nil
}

// DiscardSyncAuthorityCandidate deletes exactly one active staging or ready
// candidate checkpoint. Candidate children cascade; canonical sync state is
// untouched.
func (store *Store) DiscardSyncAuthorityCandidate(ctx context.Context, projectID continuity.ProjectID, checkpoint SyncAuthorityCandidateCheckpoint) error {
	if err := validateSyncAuthorityCandidateCheckpointV2(checkpoint); err != nil {
		return err
	}
	if err := validateSyncProjectID(projectID); err != nil {
		return err
	}
	if store == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	var promoted int
	if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM continuity_sync_authority_candidates
  WHERE project_id = ? AND candidate_id = ? AND state = 'promoted'
)`, string(projectID), checkpoint.CandidateID[:]).Scan(&promoted); err != nil {
		return syncTransactionProblem(ctx)
	}
	if promoted != 0 {
		return syncProblem(SyncErrorConflict, "candidate_id", "identifies a promoted authority candidate")
	}
	current, found, err := readAndValidateActiveSyncAuthorityCandidateV2(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		if err := tx.Commit(); err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	if current.candidate.Checkpoint() != checkpoint {
		return syncProblem(SyncErrorConflict, "checkpoint", "does not match the active authority candidate")
	}
	state := "staging"
	var authorityDigest any
	if checkpoint.Ready {
		state = "ready"
		authorityDigest = checkpoint.AuthorityDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_authority_candidates
WHERE project_id = ? AND candidate_id = ? AND state = ?
  AND page_count = ? AND environment_count = ? AND through_environment_id = ?
  AND rolling_environment_digest = ?
  AND ((? IS NULL AND authority_digest IS NULL) OR authority_digest = ?)`,
		string(projectID), checkpoint.CandidateID[:], state, checkpoint.PageCount,
		checkpoint.EnvironmentCount, checkpoint.ThroughEnvironmentID,
		checkpoint.RollingEnvironmentDigest[:], authorityDigest, authorityDigest,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "checkpoint", "active authority candidate changed")
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "authority candidate discard outcome is unknown")
	}
	return nil
}

func validateSyncAuthorityCandidateCheckpointV2(checkpoint SyncAuthorityCandidateCheckpoint) error {
	if checkpoint.CandidateID == ([32]byte{}) || checkpoint.PageCount < 1 || checkpoint.EnvironmentCount < 1 ||
		!validOpaqueID(checkpoint.ThroughEnvironmentID) || checkpoint.RollingEnvironmentDigest == ([32]byte{}) ||
		(checkpoint.Ready != (checkpoint.AuthorityDigest != ([32]byte{}))) {
		return syncProblem(SyncErrorInvalid, "checkpoint", "is invalid")
	}
	return nil
}

func corruptSyncAuthorityCandidateV2(detail string) error {
	return syncProblem(SyncErrorStore, "sync_authority_candidate", detail)
}
