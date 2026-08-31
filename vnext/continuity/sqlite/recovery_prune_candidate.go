package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	maximumSyncRecoveryPrunePagePrunesV1     int64 = 4
	maximumSyncRecoveryPruneTargetsV1        int64 = 1_024
	maximumSyncRecoveryPruneCandidatePagesV1       = math.MaxInt64 / maximumSyncRecoveryPrunePagePrunesV1
)

// SyncRecoveryPruneSnapshot pins one verified relay prune inventory to the
// exact canonical authority that authorized its arrival prefix.
type SyncRecoveryPruneSnapshot struct {
	Authority SyncAuthorityBinding
	PruneHead int64
}

// SyncRecoveryPruneRollingDigest is the caller-verified rolling commitment to
// every prune inventory record consumed through one checkpoint.
type SyncRecoveryPruneRollingDigest [32]byte

// SyncRecoveryPruneInventoryDigest is the caller-verified final commitment to
// one complete prune inventory snapshot.
type SyncRecoveryPruneInventoryDigest [32]byte

// VerifiedSyncRecoveryPruneTarget is the minimal exact identity and
// ordering metadata of one already-authenticated pruned arrival.
type VerifiedSyncRecoveryPruneTarget struct {
	Reference legacyVerifiedPruneReferenceV1
	FactKind  continuity.FactKind
	HLC       continuity.HybridTime
}

func (VerifiedSyncRecoveryPruneTarget) String() string {
	return "[REDACTED verified recovery prune target]"
}

func (VerifiedSyncRecoveryPruneTarget) GoString() string {
	return "sqlite.VerifiedSyncRecoveryPruneTarget([REDACTED])"
}

// VerifiedSyncRecoveryPruneRecord is the minimal projection of one
// already-authenticated prune certificate. PruneID identifies relay inventory
// ownership; PruneCertificateID authorizes the exact target set.
type VerifiedSyncRecoveryPruneRecord struct {
	PruneSequence        int64
	PruneID              [32]byte
	PruneCertificateID   [32]byte
	MembershipGeneration uint32
	Targets              []VerifiedSyncRecoveryPruneTarget
}

func (VerifiedSyncRecoveryPruneRecord) String() string {
	return "[REDACTED verified recovery prune record]"
}

func (VerifiedSyncRecoveryPruneRecord) GoString() string {
	return "sqlite.VerifiedSyncRecoveryPruneRecord([REDACTED])"
}

// SyncRecoveryPruneTargetMatch is the exact joined prune-record and target
// projection for one arrival under an immutable READY recovery candidate.
// PruneID binds the opaque relay arrival; the remaining fields are sufficient
// to construct and revalidate a verified terminal pruned frame.
type SyncRecoveryPruneTargetMatch struct {
	PruneID              [32]byte
	PruneCertificateID   [32]byte
	MembershipGeneration uint32
	Reference            legacyVerifiedPruneReferenceV1
	FactKind             continuity.FactKind
	HLC                  continuity.HybridTime
}

func (SyncRecoveryPruneTargetMatch) String() string {
	return "[REDACTED recovery prune target match]"
}

func (SyncRecoveryPruneTargetMatch) GoString() string {
	return "sqlite.SyncRecoveryPruneTargetMatch([REDACTED])"
}

// SyncRecoveryPruneCandidatePage is one bounded, already-verified checkpoint
// advance. ResultingRollingDigest and LastMembershipGeneration are cumulative
// values after this page. Records is the exact minimal projection covered
// by the checkpoint. InventoryDigest is nonzero only on the final page.
type SyncRecoveryPruneCandidatePage struct {
	AfterPruneSequence       int64
	PagePruneCount           int64
	PageTargetCount          int64
	LastMembershipGeneration uint32
	ResultingRollingDigest   SyncRecoveryPruneRollingDigest
	InventoryDigest          SyncRecoveryPruneInventoryDigest
	More                     bool
	Records                  []VerifiedSyncRecoveryPruneRecord
}

func (SyncRecoveryPruneCandidatePage) String() string {
	return "[REDACTED recovery prune candidate page]"
}

func (SyncRecoveryPruneCandidatePage) GoString() string {
	return "sqlite.SyncRecoveryPruneCandidatePage([REDACTED])"
}

// SyncRecoveryPruneCandidate is the fixed-size durable checkpoint for one
// verified recovery prune inventory. It contains no certificate, manifest,
// bootstrap, payload, bearer-authority, or project-root bytes.
type SyncRecoveryPruneCandidate struct {
	ProjectID                continuity.ProjectID
	CandidateID              [32]byte
	Snapshot                 SyncRecoveryPruneSnapshot
	PageCount                int64
	PruneCount               int64
	TargetCount              int64
	ThroughPruneSequence     int64
	LastMembershipGeneration uint32
	RollingInventoryDigest   SyncRecoveryPruneRollingDigest
	Ready                    bool
	InventoryDigest          SyncRecoveryPruneInventoryDigest
}

// SyncRecoveryPruneCandidateCheckpoint is the exact compare-and-swap token
// required to advance or discard one active recovery prune candidate.
type SyncRecoveryPruneCandidateCheckpoint struct {
	CandidateID              [32]byte
	PageCount                int64
	PruneCount               int64
	TargetCount              int64
	ThroughPruneSequence     int64
	LastMembershipGeneration uint32
	RollingInventoryDigest   SyncRecoveryPruneRollingDigest
	Ready                    bool
	InventoryDigest          SyncRecoveryPruneInventoryDigest
}

// Checkpoint returns the exact compare-and-swap token for candidate.
func (candidate SyncRecoveryPruneCandidate) Checkpoint() SyncRecoveryPruneCandidateCheckpoint {
	return SyncRecoveryPruneCandidateCheckpoint{
		CandidateID:              candidate.CandidateID,
		PageCount:                candidate.PageCount,
		PruneCount:               candidate.PruneCount,
		TargetCount:              candidate.TargetCount,
		ThroughPruneSequence:     candidate.ThroughPruneSequence,
		LastMembershipGeneration: candidate.LastMembershipGeneration,
		RollingInventoryDigest:   candidate.RollingInventoryDigest,
		Ready:                    candidate.Ready,
		InventoryDigest:          candidate.InventoryDigest,
	}
}

// SyncRecoveryPruneTargetByArrival resolves one authenticated target from the
// exact immutable READY recovery candidate. A false result is authoritative
// only because this method rejects staging, stale, missing, and recreated
// candidates before consulting the point index.
func (store *Store) SyncRecoveryPruneTargetByArrival(
	ctx context.Context,
	projectID continuity.ProjectID,
	expected SyncRecoveryPruneCandidate,
	arrivalSequence int64,
) (SyncRecoveryPruneTargetMatch, bool, error) {
	if err := validateSyncRecoveryPruneCandidateV1(expected); err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, err
	}
	if expected.ProjectID != projectID {
		return SyncRecoveryPruneTargetMatch{}, false, syncProblem(SyncErrorInvalid, "candidate", "does not match the requested project")
	}
	if !expected.Ready {
		return SyncRecoveryPruneTargetMatch{}, false, syncProblem(SyncErrorInvalid, "candidate", "must be ready")
	}
	if arrivalSequence < 1 || arrivalSequence > expected.Snapshot.Authority.InventoryArrivalHead {
		return SyncRecoveryPruneTargetMatch{}, false, syncProblem(SyncErrorInvalid, "arrival_sequence", "is outside the exact recovery prefix")
	}
	if store == nil {
		return SyncRecoveryPruneTargetMatch{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncRecoveryPruneTargetMatch{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncRecoveryPruneTargetMatch{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, err
	}
	if err := requireExactSyncRecoveryPruneCandidateV1(ctx, tx, projectID, expected); err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, err
	}
	match, found, err := readSyncRecoveryPruneTargetMatchV1(ctx, tx, projectID, expected.CandidateID, arrivalSequence)
	if err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, err
	}
	if !found {
		if err := validateSyncRecoveryPruneCandidateIndexV1(ctx, tx, expected); err != nil {
			return SyncRecoveryPruneTargetMatch{}, false, err
		}
	}
	if found && match.MembershipGeneration > expected.Snapshot.Authority.MembershipGeneration {
		return SyncRecoveryPruneTargetMatch{}, false, corruptSyncRecoveryPruneCandidateV1("indexed prune target generation exceeds the ready authority")
	}
	if err := tx.Commit(); err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, syncTransactionProblem(ctx)
	}
	return match, found, nil
}

// StageVerifiedSyncRecoveryPruneCandidatePage creates, advances, or exactly
// replays one bounded, already-verified prune inventory checkpoint. The exact
// predecessor token makes an unknown commit outcome safely retryable without
// permitting a stale writer to overwrite a newer checkpoint.
func (store *Store) StageVerifiedSyncRecoveryPruneCandidatePage(
	ctx context.Context,
	projectID continuity.ProjectID,
	snapshot SyncRecoveryPruneSnapshot,
	expected *SyncRecoveryPruneCandidateCheckpoint,
	page SyncRecoveryPruneCandidatePage,
) (SyncRecoveryPruneCandidate, error) {
	preparedPage, err := prepareSyncRecoveryPruneCandidatePageV1(page)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	next, err := prepareSyncRecoveryPruneCandidateSuccessorV1(projectID, snapshot, expected, preparedPage)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if err := validateSyncRecoveryPruneCandidatePageRecordsV1(snapshot, expected, preparedPage); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if store == nil {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SyncRecoveryPruneCandidate{}, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if err := requireExactSyncRecoveryPruneSnapshotFenceV1(ctx, tx, projectID, snapshot); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if err := requireNoPromotedTerminalReceiptAtRecoveryCutoffV1(ctx, tx, projectID, snapshot.Authority); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	activeAuthorityCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if activeAuthorityCandidate {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before recovery prune inventory staging")
	}
	_, terminalCandidateFound, err := readActiveTerminalCandidateV1(ctx, tx, projectID)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if terminalCandidateFound {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "terminal_candidate", "must be discarded before recovery prune inventory staging")
	}

	current, found, err := readSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if found {
		if err := validateSyncRecoveryPruneCandidateAppendBoundaryV1(ctx, tx, current); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
	}
	if expected == nil {
		if found {
			replayed := next
			replayed.CandidateID = current.CandidateID
			if current == replayed {
				if current.Ready {
					if err := validateSyncRecoveryPruneCandidateIndexV1(ctx, tx, current); err != nil {
						return SyncRecoveryPruneCandidate{}, err
					}
				}
				if err := requireExactSyncRecoveryPruneCandidatePageRecordsV1(ctx, tx, current, preparedPage); err != nil {
					return SyncRecoveryPruneCandidate{}, err
				}
				if err := commitSyncRecoveryPruneCandidatePageV1(ctx, tx.Commit); err != nil {
					return SyncRecoveryPruneCandidate{}, err
				}
				return current, nil
			}
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "checkpoint", "an active recovery prune candidate already exists")
		}
		next.CandidateID, err = newSyncRecoveryPruneCandidateIDV1()
		if err != nil {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorStore, "", "recovery prune candidate identity generation failed")
		}
		if err := insertSyncRecoveryPruneCandidateV1(ctx, tx, next); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
		if err := insertSyncRecoveryPruneCandidatePageRecordsV1(ctx, tx, next, preparedPage); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
	} else {
		if !found {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "checkpoint", "active recovery prune candidate is missing")
		}
		if current == next {
			if current.Ready {
				if err := validateSyncRecoveryPruneCandidateIndexV1(ctx, tx, current); err != nil {
					return SyncRecoveryPruneCandidate{}, err
				}
			}
			if err := requireExactSyncRecoveryPruneCandidatePageRecordsV1(ctx, tx, current, preparedPage); err != nil {
				return SyncRecoveryPruneCandidate{}, err
			}
			if err := commitSyncRecoveryPruneCandidatePageV1(ctx, tx.Commit); err != nil {
				return SyncRecoveryPruneCandidate{}, err
			}
			return current, nil
		}
		if current.Checkpoint() != *expected {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "checkpoint", "does not match the active recovery prune candidate")
		}
		if current.Snapshot != snapshot {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "snapshot", "does not match the active recovery prune candidate")
		}
		if err := updateSyncRecoveryPruneCandidateV1(ctx, tx, current, next); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
		if err := insertSyncRecoveryPruneCandidatePageRecordsV1(ctx, tx, next, preparedPage); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
	}

	persisted, found, err := readSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if !found || persisted != next {
		return SyncRecoveryPruneCandidate{}, corruptSyncRecoveryPruneCandidateV1("candidate changed during staging")
	}
	if err := requireExactSyncRecoveryPruneCandidatePageRecordsV1(ctx, tx, persisted, preparedPage); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if persisted.Ready {
		if err := validateSyncRecoveryPruneCandidateIndexV1(ctx, tx, persisted); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
	}
	if err := requireExactSyncRecoveryPruneSnapshotFenceV1(ctx, tx, projectID, snapshot); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if err := commitSyncRecoveryPruneCandidatePageV1(ctx, tx.Commit); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	return persisted, nil
}

// VerifySyncRecoveryPrunePreflight checks the complete local authority,
// watermark, progress, exclusion, and candidate fence before a caller sends
// recovery authorization to a relay. A nil expected candidate requires that
// no recovery prune candidate exists.
func (store *Store) VerifySyncRecoveryPrunePreflight(
	ctx context.Context,
	projectID continuity.ProjectID,
	authority SyncAuthorityBinding,
	expected *SyncRecoveryPruneCandidate,
) error {
	if err := validateSyncProjectID(projectID); err != nil {
		return err
	}
	if err := validateSyncAuthorityBindingV2(authority); err != nil {
		return err
	}
	if authority.AuthorityDigestVersion != 2 {
		return syncProblem(SyncErrorInvalid, "authority_digest_version", "recovery prune inventory requires version two")
	}
	if expected != nil {
		if err := validateSyncRecoveryPruneCandidateV1(*expected); err != nil {
			return err
		}
		if expected.ProjectID != projectID || expected.Snapshot.Authority != authority {
			return syncProblem(SyncErrorInvalid, "candidate", "does not match the preflight project and authority")
		}
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
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return err
	}
	if err := requireExactSyncRecoveryPruneSnapshotFenceV1(
		ctx, tx, projectID, SyncRecoveryPruneSnapshot{Authority: authority},
	); err != nil {
		return err
	}
	activeAuthorityCandidate, err := activeSyncAuthorityCandidateExistsV2(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if activeAuthorityCandidate {
		return syncProblem(SyncErrorConflict, "sync_authority_candidate", "must be promoted or discarded before recovery prune inventory staging")
	}
	_, terminalCandidateFound, err := readActiveTerminalCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if terminalCandidateFound {
		return syncProblem(SyncErrorConflict, "terminal_candidate", "must be discarded before recovery prune inventory staging")
	}
	current, found, err := readSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if expected == nil {
		if found {
			return syncProblem(SyncErrorConflict, "checkpoint", "an active recovery prune candidate already exists")
		}
	} else if !found || current != *expected {
		return syncProblem(SyncErrorConflict, "checkpoint", "does not match the active recovery prune candidate")
	}
	if err := tx.Commit(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

// VerifySyncRecoveryPruneCandidatePageRecords compares one bounded,
// already-authenticated page with the exact active candidate projection. The
// checkpoint fence prevents a verified page from being applied to a recreated
// or concurrently advanced candidate.
func (store *Store) VerifySyncRecoveryPruneCandidatePageRecords(
	ctx context.Context,
	projectID continuity.ProjectID,
	snapshot SyncRecoveryPruneSnapshot,
	checkpoint SyncRecoveryPruneCandidateCheckpoint,
	page SyncRecoveryPruneCandidatePage,
) error {
	preparedPage, err := prepareSyncRecoveryPruneCandidatePageV1(page)
	if err != nil {
		return err
	}
	if err := validateSyncProjectID(projectID); err != nil {
		return err
	}
	if err := validateSyncRecoveryPruneSnapshotV1(snapshot); err != nil {
		return err
	}
	if err := validateSyncRecoveryPruneCandidateCheckpointV1(checkpoint); err != nil {
		return err
	}
	if err := validateSyncRecoveryPruneCandidatePageRecordsV1(snapshot, nil, preparedPage); err != nil {
		return err
	}
	if preparedPage.AfterPruneSequence < 0 || preparedPage.PagePruneCount < 0 ||
		preparedPage.AfterPruneSequence > checkpoint.ThroughPruneSequence ||
		preparedPage.PagePruneCount > checkpoint.ThroughPruneSequence-preparedPage.AfterPruneSequence {
		return syncProblem(SyncErrorInvalid, "records", "does not fall within the exact candidate prefix")
	}
	if preparedPage.PagePruneCount == 0 && checkpoint.PruneCount != 0 {
		return syncProblem(SyncErrorInvalid, "records", "empty projection is valid only for an empty candidate")
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
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	if err := requireExactSyncRecoveryPruneSnapshotFenceV1(ctx, tx, projectID, snapshot); err != nil {
		return err
	}
	current, found, err := readSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found || current.Snapshot != snapshot || current.Checkpoint() != checkpoint {
		return syncProblem(SyncErrorConflict, "checkpoint", "does not match the active recovery prune candidate")
	}
	if err := requireExactSyncRecoveryPruneCandidatePageRecordsV1(ctx, tx, current, preparedPage); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

// CurrentSyncRecoveryPruneCandidate returns the structurally revalidated
// active checkpoint. It deliberately does not require the canonical authority
// still to match, so a stale candidate can be identified and discarded.
func (store *Store) CurrentSyncRecoveryPruneCandidate(
	ctx context.Context,
	projectID continuity.ProjectID,
) (SyncRecoveryPruneCandidate, bool, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncRecoveryPruneCandidate{}, false, err
	}
	if store == nil {
		return SyncRecoveryPruneCandidate{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	if ctx == nil {
		return SyncRecoveryPruneCandidate{}, false, syncProblem(SyncErrorInvalid, "context", "must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return SyncRecoveryPruneCandidate{}, false, err
	}

	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed || store.db == nil {
		return SyncRecoveryPruneCandidate{}, false, syncProblem(SyncErrorStore, "", "store is closed")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SyncRecoveryPruneCandidate{}, false, syncTransactionProblem(ctx)
	}
	defer tx.Rollback()
	candidate, found, err := readAndValidateSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil || !found {
		return SyncRecoveryPruneCandidate{}, found, err
	}
	if err := tx.Commit(); err != nil {
		return SyncRecoveryPruneCandidate{}, false, syncTransactionProblem(ctx)
	}
	return candidate, true, nil
}

// DiscardSyncRecoveryPruneCandidate deletes exactly one active staging or ready
// checkpoint. Canonical authority, inbox, facts, and progress are untouched.
func (store *Store) DiscardSyncRecoveryPruneCandidate(
	ctx context.Context,
	projectID continuity.ProjectID,
	checkpoint SyncRecoveryPruneCandidateCheckpoint,
) error {
	if err := validateSyncRecoveryPruneCandidateCheckpointV1(checkpoint); err != nil {
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
	if err := requireNoSyncAuthorityRecoveryTransitionV1(ctx, tx, projectID); err != nil {
		return err
	}
	current, found, err := readAndValidateSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		if err := tx.Commit(); err != nil {
			return syncTransactionProblem(ctx)
		}
		return nil
	}
	if current.Checkpoint() != checkpoint {
		return syncProblem(SyncErrorConflict, "checkpoint", "does not match the active recovery prune candidate")
	}
	terminalCandidateFound, err := activeTerminalCandidateExistsV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if terminalCandidateFound {
		return syncProblem(SyncErrorConflict, "terminal_candidate", "must be promoted or discarded before the recovery prune candidate")
	}
	if err := deleteExactSyncRecoveryPruneCandidateV1(ctx, tx, current); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "recovery prune candidate discard outcome is unknown")
	}
	return nil
}

func deleteExactSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	candidate SyncRecoveryPruneCandidate,
) error {
	checkpoint := candidate.Checkpoint()
	state := "staging"
	var inventoryDigest any
	if checkpoint.Ready {
		state = "ready"
		inventoryDigest = checkpoint.InventoryDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
DELETE FROM continuity_sync_recovery_prune_candidates
WHERE project_id = ? AND candidate_id = ? AND state = ?
  AND channel_id = ? AND relay_generation = ? AND admin_public_key = ?
  AND membership_generation = ? AND inventory_arrival_head = ?
  AND authority_digest_version = ? AND authority_digest = ? AND prune_head = ?
  AND page_count = ? AND prune_count = ? AND target_count = ?
  AND through_prune_sequence = ? AND last_membership_generation = ?
  AND rolling_inventory_digest = ?
  AND ((? IS NULL AND inventory_digest IS NULL) OR inventory_digest = ?)`,
		string(candidate.ProjectID), checkpoint.CandidateID[:], state,
		candidate.Snapshot.Authority.ChannelID[:], candidate.Snapshot.Authority.RelayGeneration[:],
		candidate.Snapshot.Authority.AdminPublicKey[:], candidate.Snapshot.Authority.MembershipGeneration,
		candidate.Snapshot.Authority.InventoryArrivalHead, candidate.Snapshot.Authority.AuthorityDigestVersion,
		candidate.Snapshot.Authority.AuthorityDigest[:], candidate.Snapshot.PruneHead,
		checkpoint.PageCount, checkpoint.PruneCount, checkpoint.TargetCount,
		checkpoint.ThroughPruneSequence, checkpoint.LastMembershipGeneration,
		checkpoint.RollingInventoryDigest[:], inventoryDigest, inventoryDigest,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "checkpoint", "active recovery prune candidate changed")
	}
	return nil
}

func prepareSyncRecoveryPruneCandidatePageV1(
	page SyncRecoveryPruneCandidatePage,
) (SyncRecoveryPruneCandidatePage, error) {
	if len(page.Records) > int(maximumSyncRecoveryPrunePagePrunesV1) {
		return SyncRecoveryPruneCandidatePage{}, syncProblem(SyncErrorInvalid, "records", "contains more than four prune records")
	}
	prepared := page
	prepared.Records = make([]VerifiedSyncRecoveryPruneRecord, len(page.Records))
	for index := range page.Records {
		if len(page.Records[index].Targets) > int(maximumSyncRecoveryPruneTargetsV1) {
			return SyncRecoveryPruneCandidatePage{}, syncProblem(SyncErrorInvalid, "records.targets", "contains more than 1024 prune targets")
		}
		prepared.Records[index] = page.Records[index]
		prepared.Records[index].Targets = append(
			[]VerifiedSyncRecoveryPruneTarget(nil), page.Records[index].Targets...,
		)
	}
	return prepared, nil
}

func validateSyncRecoveryPruneCandidatePageRecordsV1(
	snapshot SyncRecoveryPruneSnapshot,
	expected *SyncRecoveryPruneCandidateCheckpoint,
	page SyncRecoveryPruneCandidatePage,
) error {
	if int64(len(page.Records)) != page.PagePruneCount {
		return syncProblem(SyncErrorInvalid, "records", "does not exactly cover the page prune count")
	}
	if page.PagePruneCount == 0 {
		return nil
	}
	lastMembershipGeneration := uint32(0)
	if expected != nil {
		lastMembershipGeneration = expected.LastMembershipGeneration
	}
	seenPruneIDs := make(map[[32]byte]struct{}, len(page.Records))
	seenPruneCertificateIDs := make(map[[32]byte]struct{}, len(page.Records))
	seenArrivals := make(map[int64]struct{})
	seenFacts := make(map[continuity.FactID]struct{})
	type sourceKey struct {
		environmentID       continuity.EnvironmentID
		environmentSequence int64
	}
	seenSources := make(map[sourceKey]struct{})
	seenEnvelopes := make(map[[32]byte]struct{})
	type nonceKey struct {
		generation uint32
		nonce      [24]byte
	}
	seenNonces := make(map[nonceKey]struct{})
	var targetCount int64
	for recordIndex, record := range page.Records {
		wantSequence := page.AfterPruneSequence + int64(recordIndex) + 1
		field := fmt.Sprintf("records[%d]", recordIndex)
		if record.PruneSequence != wantSequence {
			return syncProblem(SyncErrorInvalid, field+".prune_sequence", "does not continue the exact page cursor")
		}
		if record.PruneID == ([32]byte{}) {
			return syncProblem(SyncErrorInvalid, field+".prune_id", "must be nonzero")
		}
		if record.PruneCertificateID == ([32]byte{}) {
			return syncProblem(SyncErrorInvalid, field+".prune_certificate_id", "must be nonzero")
		}
		if _, duplicate := seenPruneIDs[record.PruneID]; duplicate {
			return syncProblem(SyncErrorInvalid, field+".prune_id", "is duplicated within the page")
		}
		if _, duplicate := seenPruneCertificateIDs[record.PruneCertificateID]; duplicate {
			return syncProblem(SyncErrorInvalid, field+".prune_certificate_id", "is duplicated within the page")
		}
		seenPruneIDs[record.PruneID] = struct{}{}
		seenPruneCertificateIDs[record.PruneCertificateID] = struct{}{}
		if record.MembershipGeneration == 0 || record.MembershipGeneration > snapshot.Authority.MembershipGeneration ||
			record.MembershipGeneration < lastMembershipGeneration {
			return syncProblem(SyncErrorInvalid, field+".membership_generation", "is not monotonic within the pinned authority")
		}
		lastMembershipGeneration = record.MembershipGeneration
		if len(record.Targets) < 1 || len(record.Targets) > int(maximumSyncRecoveryPruneTargetsV1) {
			return syncProblem(SyncErrorInvalid, field+".targets", "must contain between one and 1024 targets")
		}
		var previousArrivalSequence int64
		for targetIndex, target := range record.Targets {
			targetField := fmt.Sprintf("%s.targets[%d]", field, targetIndex)
			if err := validateVerifiedPruneReferenceV1(target.Reference, targetField+".reference"); err != nil {
				return err
			}
			if target.Reference.ArrivalSequence > snapshot.Authority.InventoryArrivalHead {
				return syncProblem(SyncErrorInvalid, targetField+".reference.arrival_sequence", "exceeds the pinned recovery arrival prefix")
			}
			if targetIndex > 0 && target.Reference.ArrivalSequence <= previousArrivalSequence {
				return syncProblem(SyncErrorInvalid, targetField+".reference.arrival_sequence", "must be strictly increasing within the prune record")
			}
			previousArrivalSequence = target.Reference.ArrivalSequence
			if !prunableScratchpadFactKindV1(target.FactKind) {
				return syncProblem(SyncErrorInvalid, targetField+".fact_kind", "is not a prunable scratchpad fact kind")
			}
			if target.HLC.WallMillis < 0 || target.HLC.Logical < 0 {
				return syncProblem(SyncErrorInvalid, targetField+".hlc", "must be nonnegative")
			}
			if _, duplicate := seenArrivals[target.Reference.ArrivalSequence]; duplicate {
				return syncProblem(SyncErrorInvalid, targetField+".reference.arrival_sequence", "is duplicated within the page")
			}
			if _, duplicate := seenFacts[target.Reference.FactID]; duplicate {
				return syncProblem(SyncErrorInvalid, targetField+".reference.fact_id", "is duplicated within the page")
			}
			source := sourceKey{target.Reference.EnvironmentID, target.Reference.EnvironmentSequence}
			if _, duplicate := seenSources[source]; duplicate {
				return syncProblem(SyncErrorInvalid, targetField+".reference.environment_sequence", "is duplicated within the page")
			}
			if _, duplicate := seenEnvelopes[target.Reference.EnvelopeDigest]; duplicate {
				return syncProblem(SyncErrorInvalid, targetField+".reference.envelope_digest", "is duplicated within the page")
			}
			nonce := nonceKey{target.Reference.KeyGeneration, target.Reference.Nonce}
			if _, duplicate := seenNonces[nonce]; duplicate {
				return syncProblem(SyncErrorInvalid, targetField+".reference.nonce", "is duplicated within the page generation")
			}
			seenArrivals[target.Reference.ArrivalSequence] = struct{}{}
			seenFacts[target.Reference.FactID] = struct{}{}
			seenSources[source] = struct{}{}
			seenEnvelopes[target.Reference.EnvelopeDigest] = struct{}{}
			seenNonces[nonce] = struct{}{}
			targetCount++
		}
	}
	if targetCount != page.PageTargetCount {
		return syncProblem(SyncErrorInvalid, "records.targets", "does not exactly cover the page target count")
	}
	if lastMembershipGeneration != page.LastMembershipGeneration {
		return syncProblem(SyncErrorInvalid, "last_membership_generation", "does not match the final page record")
	}
	return nil
}

func prepareSyncRecoveryPruneCandidateSuccessorV1(
	projectID continuity.ProjectID,
	snapshot SyncRecoveryPruneSnapshot,
	expected *SyncRecoveryPruneCandidateCheckpoint,
	page SyncRecoveryPruneCandidatePage,
) (SyncRecoveryPruneCandidate, error) {
	if err := validateSyncProjectID(projectID); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if err := validateSyncRecoveryPruneSnapshotV1(snapshot); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	var base SyncRecoveryPruneCandidateCheckpoint
	if expected != nil {
		if err := validateSyncRecoveryPruneCandidateCheckpointV1(*expected); err != nil {
			return SyncRecoveryPruneCandidate{}, err
		}
		if expected.Ready {
			if page.AfterPruneSequence != expected.ThroughPruneSequence || page.PagePruneCount != 0 ||
				page.PageTargetCount != 0 || page.LastMembershipGeneration != expected.LastMembershipGeneration ||
				page.ResultingRollingDigest != expected.RollingInventoryDigest || page.More ||
				page.InventoryDigest != expected.InventoryDigest {
				return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "checkpoint", "ready recovery prune candidate is immutable")
			}
			candidate := SyncRecoveryPruneCandidate{
				ProjectID:                projectID,
				CandidateID:              expected.CandidateID,
				Snapshot:                 snapshot,
				PageCount:                expected.PageCount,
				PruneCount:               expected.PruneCount,
				TargetCount:              expected.TargetCount,
				ThroughPruneSequence:     expected.ThroughPruneSequence,
				LastMembershipGeneration: expected.LastMembershipGeneration,
				RollingInventoryDigest:   expected.RollingInventoryDigest,
				Ready:                    true,
				InventoryDigest:          expected.InventoryDigest,
			}
			if err := validateSyncRecoveryPruneCandidateV1(candidate); err != nil {
				return SyncRecoveryPruneCandidate{}, err
			}
			return candidate, nil
		}
		base = *expected
	}
	if page.AfterPruneSequence < 0 || page.AfterPruneSequence != base.ThroughPruneSequence {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "after_prune_sequence", "is not the expected checkpoint cursor")
	}
	if page.PagePruneCount < 0 || page.PagePruneCount > maximumSyncRecoveryPrunePagePrunesV1 {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "page_prune_count", "must be between zero and four")
	}
	if page.More && page.PagePruneCount != maximumSyncRecoveryPrunePagePrunesV1 {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "page_prune_count", "nonfinal pages must contain four prunes")
	}
	if page.PagePruneCount == 0 {
		if expected != nil || snapshot.PruneHead != 0 || page.More || page.PageTargetCount != 0 || page.LastMembershipGeneration != 0 {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "page_prune_count", "empty page is valid only for an empty first and final inventory")
		}
	} else {
		if page.PageTargetCount < page.PagePruneCount ||
			(page.PageTargetCount-1)/maximumSyncRecoveryPruneTargetsV1 >= page.PagePruneCount {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "page_target_count", "is outside the bounded per-prune target range")
		}
		if page.LastMembershipGeneration == 0 || page.LastMembershipGeneration > snapshot.Authority.MembershipGeneration ||
			page.LastMembershipGeneration < base.LastMembershipGeneration {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "last_membership_generation", "is not a monotonic generation within the snapshot")
		}
	}
	if page.ResultingRollingDigest == (SyncRecoveryPruneRollingDigest{}) {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "resulting_rolling_digest", "must be nonzero")
	}
	if page.More == (page.InventoryDigest != (SyncRecoveryPruneInventoryDigest{})) {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "inventory_digest", "must be nonzero only on the final page")
	}
	if page.AfterPruneSequence > math.MaxInt64-page.PagePruneCount {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "through_prune_sequence", "overflows")
	}
	through := page.AfterPruneSequence + page.PagePruneCount
	if (page.More && through >= snapshot.PruneHead) || (!page.More && through != snapshot.PruneHead) {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "through_prune_sequence", "does not match the pinned prune head")
	}
	if base.PageCount >= maximumSyncRecoveryPruneCandidatePagesV1 || base.TargetCount > math.MaxInt64-page.PageTargetCount {
		return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorInvalid, "checkpoint", "candidate totals overflow")
	}
	next := SyncRecoveryPruneCandidate{
		ProjectID:                projectID,
		Snapshot:                 snapshot,
		PageCount:                base.PageCount + 1,
		PruneCount:               through,
		TargetCount:              base.TargetCount + page.PageTargetCount,
		ThroughPruneSequence:     through,
		LastMembershipGeneration: page.LastMembershipGeneration,
		RollingInventoryDigest:   page.ResultingRollingDigest,
		Ready:                    !page.More,
		InventoryDigest:          page.InventoryDigest,
	}
	if expected != nil {
		next.CandidateID = expected.CandidateID
	}
	if err := validateSyncRecoveryPruneCandidateStateV1(next); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	return next, nil
}

func validateSyncRecoveryPruneSnapshotV1(snapshot SyncRecoveryPruneSnapshot) error {
	if err := validateSyncAuthorityBindingV2(snapshot.Authority); err != nil {
		return err
	}
	if snapshot.Authority.AuthorityDigestVersion != 2 {
		return syncProblem(SyncErrorInvalid, "authority_digest_version", "recovery prune inventory requires version two")
	}
	if snapshot.PruneHead < 0 || snapshot.PruneHead > snapshot.Authority.InventoryArrivalHead {
		return syncProblem(SyncErrorInvalid, "prune_head", "must be within the exact authority arrival prefix")
	}
	return nil
}

func validateSyncRecoveryPruneCandidateCheckpointV1(checkpoint SyncRecoveryPruneCandidateCheckpoint) error {
	if checkpoint.CandidateID == ([32]byte{}) || checkpoint.PageCount < 1 ||
		checkpoint.PageCount > maximumSyncRecoveryPruneCandidatePagesV1 || checkpoint.PruneCount < 0 ||
		checkpoint.PruneCount != checkpoint.ThroughPruneSequence || checkpoint.TargetCount < 0 ||
		checkpoint.RollingInventoryDigest == (SyncRecoveryPruneRollingDigest{}) ||
		checkpoint.Ready != (checkpoint.InventoryDigest != (SyncRecoveryPruneInventoryDigest{})) {
		return syncProblem(SyncErrorInvalid, "checkpoint", "is invalid")
	}
	if checkpoint.PruneCount == 0 {
		if checkpoint.PageCount != 1 || checkpoint.TargetCount != 0 || checkpoint.LastMembershipGeneration != 0 || !checkpoint.Ready {
			return syncProblem(SyncErrorInvalid, "checkpoint", "is invalid")
		}
		return nil
	}
	if checkpoint.PruneCount <= (checkpoint.PageCount-1)*maximumSyncRecoveryPrunePagePrunesV1 ||
		checkpoint.PruneCount > checkpoint.PageCount*maximumSyncRecoveryPrunePagePrunesV1 ||
		checkpoint.TargetCount < checkpoint.PruneCount ||
		(checkpoint.TargetCount-1)/maximumSyncRecoveryPruneTargetsV1 >= checkpoint.PruneCount ||
		checkpoint.LastMembershipGeneration == 0 ||
		(!checkpoint.Ready && checkpoint.PruneCount != checkpoint.PageCount*maximumSyncRecoveryPrunePagePrunesV1) {
		return syncProblem(SyncErrorInvalid, "checkpoint", "is invalid")
	}
	return nil
}

func validateSyncRecoveryPruneCandidateV1(candidate SyncRecoveryPruneCandidate) error {
	if candidate.CandidateID == ([32]byte{}) {
		return syncProblem(SyncErrorInvalid, "candidate_id", "must be nonzero")
	}
	if err := validateSyncRecoveryPruneCandidateCheckpointV1(candidate.Checkpoint()); err != nil {
		return err
	}
	return validateSyncRecoveryPruneCandidateStateV1(candidate)
}

func validateSyncRecoveryPruneCandidateStateV1(candidate SyncRecoveryPruneCandidate) error {
	if err := validateSyncProjectID(candidate.ProjectID); err != nil {
		return err
	}
	if err := validateSyncRecoveryPruneSnapshotV1(candidate.Snapshot); err != nil {
		return err
	}
	checkpoint := candidate.Checkpoint()
	checkpoint.CandidateID = [32]byte{1}
	if err := validateSyncRecoveryPruneCandidateCheckpointV1(checkpoint); err != nil {
		return err
	}
	if candidate.LastMembershipGeneration > candidate.Snapshot.Authority.MembershipGeneration ||
		candidate.ThroughPruneSequence > candidate.Snapshot.PruneHead ||
		(candidate.Ready && candidate.ThroughPruneSequence != candidate.Snapshot.PruneHead) ||
		(!candidate.Ready && candidate.ThroughPruneSequence >= candidate.Snapshot.PruneHead) ||
		(candidate.PruneCount == 0 && candidate.Snapshot.PruneHead != 0) {
		return syncProblem(SyncErrorInvalid, "candidate", "is inconsistent with the recovery prune snapshot")
	}
	return nil
}

func newSyncRecoveryPruneCandidateIDV1() ([32]byte, error) {
	for {
		var candidateID [32]byte
		if _, err := rand.Read(candidateID[:]); err != nil {
			return [32]byte{}, err
		}
		if candidateID != ([32]byte{}) {
			return candidateID, nil
		}
	}
}

func requireExactSyncRecoveryPruneSnapshotFenceV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	snapshot SyncRecoveryPruneSnapshot,
) error {
	binding, err := requireExactCanonicalSyncAuthorityBindingV2(ctx, tx, projectID, snapshot.Authority)
	if err != nil {
		return err
	}
	if binding.AuthorityDigestVersion != 2 {
		return syncProblem(SyncErrorConflict, "authority_digest_version", "recovery prune inventory requires canonical version two authority")
	}
	if err := requireKnownExactSyncRelayWatermarkV1(
		ctx, tx, syncRelayWatermarkFromAuthorityBindingV1(projectID, binding),
	); err != nil {
		return err
	}
	progress, found, err := readSyncProgressV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found {
		return syncProblem(SyncErrorNotFound, "project_id", "has no staged sync state")
	}
	if progress.ChannelID != binding.ChannelID {
		return syncProblem(SyncErrorConflict, "channel_id", "does not match the exact recovery prune authority")
	}
	if progress.ActivationState != SyncActivationStaging {
		return syncProblem(SyncErrorConflict, "activation_state", "recovery prune inventory requires staging sync state")
	}
	if progress.DownloadedCursor != binding.InventoryArrivalHead || progress.RelayHead != binding.InventoryArrivalHead {
		return syncProblem(SyncErrorCursor, "downloaded_cursor", "does not cover the exact recovery prune arrival prefix")
	}
	return nil
}

func requireNoPromotedTerminalReceiptAtRecoveryCutoffV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	authority SyncAuthorityBinding,
) error {
	var candidateCount int64
	var candidateID []byte
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), MIN(candidate_id)
FROM continuity_sync_terminal_candidates
WHERE project_id = ? AND state = 'promoted'
  AND channel_id = ? AND relay_generation = ?
  AND membership_generation = ? AND authority_digest = ?
  AND resulting_applied_cursor = ?`,
		string(projectID), authority.ChannelID[:], authority.RelayGeneration[:],
		authority.MembershipGeneration, authority.AuthorityDigest[:], authority.InventoryArrivalHead,
	).Scan(&candidateCount, &candidateID); err != nil {
		return syncTransactionProblem(ctx)
	}
	if candidateCount == 0 {
		return nil
	}
	if candidateCount != 1 || len(candidateID) != 32 || isZeroDigestBytesV2(candidateID) {
		return syncProblem(SyncErrorStore, "", "promoted terminal recovery receipt inventory is corrupt")
	}
	var exactCandidateID [32]byte
	copy(exactCandidateID[:], candidateID)
	receipt, found, err := readPromotedTerminalCandidateReceiptV1(ctx, tx, projectID, exactCandidateID)
	if err != nil {
		return err
	}
	if !found || receipt.ChannelID != authority.ChannelID || receipt.RelayGeneration != authority.RelayGeneration ||
		receipt.MembershipGeneration != authority.MembershipGeneration || receipt.AuthorityDigest != authority.AuthorityDigest ||
		receipt.ResultingAppliedCursor != authority.InventoryArrivalHead {
		return syncProblem(SyncErrorStore, "", "promoted terminal recovery receipt inventory is inconsistent")
	}
	return syncProblem(SyncErrorConflict, "terminal_candidate", "promoted terminal history already covers the recovery cutoff")
}

func insertSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	candidate SyncRecoveryPruneCandidate,
) error {
	state := "staging"
	var inventoryDigest any
	if candidate.Ready {
		state = "ready"
		inventoryDigest = candidate.InventoryDigest[:]
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO continuity_sync_recovery_prune_candidates(
  project_id, candidate_id, state,
  channel_id, relay_generation, admin_public_key,
  membership_generation, inventory_arrival_head,
  authority_digest_version, authority_digest, prune_head,
  page_count, prune_count, target_count, through_prune_sequence,
  last_membership_generation, rolling_inventory_digest, inventory_digest
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		string(candidate.ProjectID), candidate.CandidateID[:], state,
		candidate.Snapshot.Authority.ChannelID[:], candidate.Snapshot.Authority.RelayGeneration[:],
		candidate.Snapshot.Authority.AdminPublicKey[:], candidate.Snapshot.Authority.MembershipGeneration,
		candidate.Snapshot.Authority.InventoryArrivalHead, candidate.Snapshot.Authority.AuthorityDigestVersion,
		candidate.Snapshot.Authority.AuthorityDigest[:], candidate.Snapshot.PruneHead,
		candidate.PageCount, candidate.PruneCount, candidate.TargetCount, candidate.ThroughPruneSequence,
		candidate.LastMembershipGeneration, candidate.RollingInventoryDigest[:], inventoryDigest,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	return nil
}

func updateSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	current,
	next SyncRecoveryPruneCandidate,
) error {
	state := "staging"
	var inventoryDigest any
	if next.Ready {
		state = "ready"
		inventoryDigest = next.InventoryDigest[:]
	}
	currentState := "staging"
	var currentInventoryDigest any
	if current.Ready {
		currentState = "ready"
		currentInventoryDigest = current.InventoryDigest[:]
	}
	result, err := tx.ExecContext(ctx, `
UPDATE continuity_sync_recovery_prune_candidates
SET state = ?, page_count = ?, prune_count = ?, target_count = ?,
    through_prune_sequence = ?, last_membership_generation = ?,
    rolling_inventory_digest = ?, inventory_digest = ?
WHERE project_id = ? AND candidate_id = ? AND state = ?
  AND channel_id = ? AND relay_generation = ? AND admin_public_key = ?
  AND membership_generation = ? AND inventory_arrival_head = ?
  AND authority_digest_version = ? AND authority_digest = ? AND prune_head = ?
  AND page_count = ? AND prune_count = ? AND target_count = ?
  AND through_prune_sequence = ? AND last_membership_generation = ?
  AND rolling_inventory_digest = ?
  AND ((? IS NULL AND inventory_digest IS NULL) OR inventory_digest = ?)`,
		state, next.PageCount, next.PruneCount, next.TargetCount,
		next.ThroughPruneSequence, next.LastMembershipGeneration,
		next.RollingInventoryDigest[:], inventoryDigest,
		string(current.ProjectID), current.CandidateID[:], currentState,
		current.Snapshot.Authority.ChannelID[:], current.Snapshot.Authority.RelayGeneration[:],
		current.Snapshot.Authority.AdminPublicKey[:], current.Snapshot.Authority.MembershipGeneration,
		current.Snapshot.Authority.InventoryArrivalHead, current.Snapshot.Authority.AuthorityDigestVersion,
		current.Snapshot.Authority.AuthorityDigest[:], current.Snapshot.PruneHead,
		current.PageCount, current.PruneCount, current.TargetCount,
		current.ThroughPruneSequence, current.LastMembershipGeneration,
		current.RollingInventoryDigest[:], currentInventoryDigest, currentInventoryDigest,
	)
	if err != nil {
		return syncTransactionProblem(ctx)
	}
	if err := requireOneAffectedV1(result, ctx); err != nil {
		return syncProblem(SyncErrorConflict, "checkpoint", "active recovery prune candidate changed")
	}
	return nil
}

func insertSyncRecoveryPruneCandidatePageRecordsV1(
	ctx context.Context,
	tx *sql.Tx,
	candidate SyncRecoveryPruneCandidate,
	page SyncRecoveryPruneCandidatePage,
) error {
	for _, record := range page.Records {
		result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO continuity_sync_recovery_prune_records(
  project_id, candidate_id, prune_sequence, prune_id,
  prune_certificate_id, membership_generation, target_count
) VALUES(?, ?, ?, ?, ?, ?, ?)`,
			string(candidate.ProjectID), candidate.CandidateID[:], record.PruneSequence,
			record.PruneID[:], record.PruneCertificateID[:], record.MembershipGeneration,
			len(record.Targets),
		)
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return syncTransactionProblem(ctx)
		}
		if affected != 1 {
			return syncProblem(SyncErrorConflict, "records", "reuses a persisted prune identity")
		}
		for targetIndex, target := range record.Targets {
			reference := target.Reference
			result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO continuity_sync_recovery_prune_targets(
  project_id, candidate_id, prune_sequence, target_ordinal,
  fact_id, environment_id, environment_sequence, arrival_sequence,
  envelope_digest, certificate_id, previous_envelope_digest,
  key_generation, nonce, fact_kind, hlc_wall_millis, hlc_logical
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				string(candidate.ProjectID), candidate.CandidateID[:], record.PruneSequence, targetIndex+1,
				string(reference.FactID), string(reference.EnvironmentID), reference.EnvironmentSequence,
				reference.ArrivalSequence, reference.EnvelopeDigest[:], reference.CertificateID[:],
				reference.PreviousEnvelopeDigest[:], reference.KeyGeneration, reference.Nonce[:],
				string(target.FactKind), target.HLC.WallMillis, target.HLC.Logical,
			)
			if err != nil {
				return syncTransactionProblem(ctx)
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return syncTransactionProblem(ctx)
			}
			if affected != 1 {
				return syncProblem(SyncErrorConflict, "records.targets", "reuses a persisted target identity")
			}
		}
	}
	return nil
}

func requireExactSyncRecoveryPruneCandidatePageRecordsV1(
	ctx context.Context,
	tx *sql.Tx,
	candidate SyncRecoveryPruneCandidate,
	page SyncRecoveryPruneCandidatePage,
) error {
	for _, expected := range page.Records {
		persisted, found, err := readSyncRecoveryPruneRecordV1(
			ctx, tx, candidate.ProjectID, candidate.CandidateID, expected.PruneSequence,
		)
		if err != nil {
			return err
		}
		if !found || !syncRecoveryPruneRecordsEqualV1(persisted, expected) {
			return syncProblem(SyncErrorConflict, "records", "does not exactly replay the persisted prune projection")
		}
	}
	return nil
}

func requireExactSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	expected SyncRecoveryPruneCandidate,
) error {
	if err := requireExactSyncRecoveryPruneSnapshotFenceV1(ctx, tx, projectID, expected.Snapshot); err != nil {
		return err
	}
	current, found, err := readSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if !found || current != expected || !current.Ready {
		return syncProblem(SyncErrorConflict, "checkpoint", "does not match the exact ready recovery prune candidate")
	}
	return nil
}

func readSyncRecoveryPruneTargetMatchV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
	arrivalSequence int64,
) (SyncRecoveryPruneTargetMatch, bool, error) {
	match := SyncRecoveryPruneTargetMatch{}
	var pruneID, pruneCertificateID []byte
	var factID, environmentID, factKind string
	var envelopeDigest, certificateID, previousEnvelopeDigest, nonce []byte
	var membershipGeneration, keyGeneration, hlcLogical int64
	err := tx.QueryRowContext(ctx, `
SELECT records.prune_id, records.prune_certificate_id, records.membership_generation,
       targets.fact_id, targets.environment_id, targets.environment_sequence,
       targets.arrival_sequence, targets.envelope_digest, targets.certificate_id,
       targets.previous_envelope_digest, targets.key_generation, targets.nonce,
       targets.fact_kind, targets.hlc_wall_millis, targets.hlc_logical
FROM continuity_sync_recovery_prune_targets AS targets
JOIN continuity_sync_recovery_prune_records AS records
  ON records.project_id = targets.project_id
 AND records.candidate_id = targets.candidate_id
 AND records.prune_sequence = targets.prune_sequence
WHERE targets.project_id = ? AND targets.candidate_id = ?
  AND targets.arrival_sequence = ?`,
		string(projectID), candidateID[:], arrivalSequence,
	).Scan(
		&pruneID, &pruneCertificateID, &membershipGeneration,
		&factID, &environmentID, &match.Reference.EnvironmentSequence,
		&match.Reference.ArrivalSequence, &envelopeDigest, &certificateID,
		&previousEnvelopeDigest, &keyGeneration, &nonce,
		&factKind, &match.HLC.WallMillis, &hlcLogical,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncRecoveryPruneTargetMatch{}, false, nil
	}
	if err != nil {
		return SyncRecoveryPruneTargetMatch{}, false, syncTransactionProblem(ctx)
	}
	if len(pruneID) != len(match.PruneID) || isZeroDigestBytesV2(pruneID) ||
		len(pruneCertificateID) != len(match.PruneCertificateID) || isZeroDigestBytesV2(pruneCertificateID) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 ||
		len(envelopeDigest) != len(match.Reference.EnvelopeDigest) ||
		len(certificateID) != len(match.Reference.CertificateID) ||
		len(previousEnvelopeDigest) != len(match.Reference.PreviousEnvelopeDigest) ||
		len(nonce) != len(match.Reference.Nonce) || keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
		hlcLogical < 0 || hlcLogical > math.MaxInt32 {
		return SyncRecoveryPruneTargetMatch{}, false, corruptSyncRecoveryPruneCandidateV1("indexed prune target match is malformed")
	}
	copy(match.PruneID[:], pruneID)
	copy(match.PruneCertificateID[:], pruneCertificateID)
	match.MembershipGeneration = uint32(membershipGeneration)
	match.Reference.FactID = continuity.FactID(factID)
	match.Reference.EnvironmentID = continuity.EnvironmentID(environmentID)
	match.Reference.KeyGeneration = uint32(keyGeneration)
	copy(match.Reference.EnvelopeDigest[:], envelopeDigest)
	copy(match.Reference.CertificateID[:], certificateID)
	copy(match.Reference.PreviousEnvelopeDigest[:], previousEnvelopeDigest)
	copy(match.Reference.Nonce[:], nonce)
	match.FactKind = continuity.FactKind(factKind)
	match.HLC.Logical = int32(hlcLogical)
	if match.Reference.ArrivalSequence != arrivalSequence ||
		validateVerifiedPruneReferenceV1(match.Reference, "persisted_target.reference") != nil ||
		!prunableScratchpadFactKindV1(match.FactKind) || match.HLC.WallMillis < 0 {
		return SyncRecoveryPruneTargetMatch{}, false, corruptSyncRecoveryPruneCandidateV1("indexed prune target match is inconsistent")
	}
	return match, true, nil
}

func readSyncRecoveryPruneRecordV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
	candidateID [32]byte,
	pruneSequence int64,
) (VerifiedSyncRecoveryPruneRecord, bool, error) {
	record := VerifiedSyncRecoveryPruneRecord{PruneSequence: pruneSequence}
	var pruneID, pruneCertificateID []byte
	var membershipGeneration, targetCount int64
	err := tx.QueryRowContext(ctx, `
SELECT prune_id, prune_certificate_id, membership_generation, target_count
FROM continuity_sync_recovery_prune_records
WHERE project_id = ? AND candidate_id = ? AND prune_sequence = ?`,
		string(projectID), candidateID[:], pruneSequence,
	).Scan(&pruneID, &pruneCertificateID, &membershipGeneration, &targetCount)
	if errors.Is(err, sql.ErrNoRows) {
		return VerifiedSyncRecoveryPruneRecord{}, false, nil
	}
	if err != nil {
		return VerifiedSyncRecoveryPruneRecord{}, false, syncTransactionProblem(ctx)
	}
	if len(pruneID) != len(record.PruneID) || isZeroDigestBytesV2(pruneID) ||
		len(pruneCertificateID) != len(record.PruneCertificateID) || isZeroDigestBytesV2(pruneCertificateID) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 ||
		targetCount < 1 || targetCount > maximumSyncRecoveryPruneTargetsV1 {
		return VerifiedSyncRecoveryPruneRecord{}, false, corruptSyncRecoveryPruneCandidateV1("prune record is malformed")
	}
	copy(record.PruneID[:], pruneID)
	copy(record.PruneCertificateID[:], pruneCertificateID)
	record.MembershipGeneration = uint32(membershipGeneration)
	record.Targets = make([]VerifiedSyncRecoveryPruneTarget, 0, targetCount)
	rows, err := tx.QueryContext(ctx, `
SELECT target_ordinal, fact_id, environment_id, environment_sequence,
       arrival_sequence, envelope_digest, certificate_id,
       previous_envelope_digest, key_generation, nonce,
       fact_kind, hlc_wall_millis, hlc_logical
FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ? AND prune_sequence = ?
ORDER BY target_ordinal`, string(projectID), candidateID[:], pruneSequence)
	if err != nil {
		return VerifiedSyncRecoveryPruneRecord{}, false, syncTransactionProblem(ctx)
	}
	for rows.Next() {
		var target VerifiedSyncRecoveryPruneTarget
		var ordinal, keyGeneration, hlcLogical int64
		var factID, environmentID, factKind string
		var envelopeDigest, certificateID, previousEnvelopeDigest, nonce []byte
		if err := rows.Scan(
			&ordinal, &factID, &environmentID, &target.Reference.EnvironmentSequence,
			&target.Reference.ArrivalSequence, &envelopeDigest, &certificateID,
			&previousEnvelopeDigest, &keyGeneration, &nonce,
			&factKind, &target.HLC.WallMillis, &hlcLogical,
		); err != nil {
			return VerifiedSyncRecoveryPruneRecord{}, false, closeRowsWithSyncRecoveryPruneErrorV1(rows, ctx)
		}
		if ordinal != int64(len(record.Targets)+1) || len(envelopeDigest) != len(target.Reference.EnvelopeDigest) ||
			len(certificateID) != len(target.Reference.CertificateID) ||
			len(previousEnvelopeDigest) != len(target.Reference.PreviousEnvelopeDigest) ||
			len(nonce) != len(target.Reference.Nonce) || keyGeneration < 1 || keyGeneration > math.MaxUint32 ||
			hlcLogical < 0 || hlcLogical > math.MaxInt32 {
			return VerifiedSyncRecoveryPruneRecord{}, false, closeRowsWithCorruptSyncRecoveryPruneErrorV1(rows, "prune target is malformed")
		}
		target.Reference.FactID = continuity.FactID(factID)
		target.Reference.EnvironmentID = continuity.EnvironmentID(environmentID)
		target.Reference.KeyGeneration = uint32(keyGeneration)
		copy(target.Reference.EnvelopeDigest[:], envelopeDigest)
		copy(target.Reference.CertificateID[:], certificateID)
		copy(target.Reference.PreviousEnvelopeDigest[:], previousEnvelopeDigest)
		copy(target.Reference.Nonce[:], nonce)
		target.FactKind = continuity.FactKind(factKind)
		target.HLC.Logical = int32(hlcLogical)
		if err := validateVerifiedPruneReferenceV1(target.Reference, "persisted_target.reference"); err != nil ||
			!prunableScratchpadFactKindV1(target.FactKind) || target.HLC.WallMillis < 0 {
			return VerifiedSyncRecoveryPruneRecord{}, false, closeRowsWithCorruptSyncRecoveryPruneErrorV1(rows, "prune target is inconsistent")
		}
		record.Targets = append(record.Targets, target)
	}
	if err := rows.Err(); err != nil {
		return VerifiedSyncRecoveryPruneRecord{}, false, closeRowsWithSyncRecoveryPruneErrorV1(rows, ctx)
	}
	if err := rows.Close(); err != nil {
		return VerifiedSyncRecoveryPruneRecord{}, false, syncTransactionProblem(ctx)
	}
	if int64(len(record.Targets)) != targetCount {
		return VerifiedSyncRecoveryPruneRecord{}, false, corruptSyncRecoveryPruneCandidateV1("prune target count is inconsistent")
	}
	return record, true, nil
}

func syncRecoveryPruneRecordsEqualV1(left, right VerifiedSyncRecoveryPruneRecord) bool {
	if left.PruneSequence != right.PruneSequence || left.PruneID != right.PruneID ||
		left.PruneCertificateID != right.PruneCertificateID ||
		left.MembershipGeneration != right.MembershipGeneration || len(left.Targets) != len(right.Targets) {
		return false
	}
	for index := range left.Targets {
		if left.Targets[index] != right.Targets[index] {
			return false
		}
	}
	return true
}

func closeRowsWithSyncRecoveryPruneErrorV1(rows *sql.Rows, ctx context.Context) error {
	if err := rows.Close(); err != nil {
		return syncTransactionProblem(ctx)
	}
	return syncTransactionProblem(ctx)
}

func closeRowsWithCorruptSyncRecoveryPruneErrorV1(rows *sql.Rows, detail string) error {
	if err := rows.Close(); err != nil {
		return corruptSyncRecoveryPruneCandidateV1(detail + "; row close failed")
	}
	return corruptSyncRecoveryPruneCandidateV1(detail)
}

func readAndValidateSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
) (SyncRecoveryPruneCandidate, bool, error) {
	candidate, found, err := readSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil || !found {
		return candidate, found, err
	}
	if err := validateSyncRecoveryPruneCandidateIndexV1(ctx, tx, candidate); err != nil {
		return SyncRecoveryPruneCandidate{}, false, err
	}
	return candidate, true, nil
}

func readSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
) (SyncRecoveryPruneCandidate, bool, error) {
	candidate := SyncRecoveryPruneCandidate{ProjectID: projectID}
	var candidateID, channelID, relayGeneration, adminPublicKey, authorityDigest []byte
	var rollingDigest, inventoryDigest []byte
	var state string
	var membershipGeneration, authorityDigestVersion, lastMembershipGeneration int64
	err := tx.QueryRowContext(ctx, `
SELECT candidate_id, state,
       channel_id, relay_generation, admin_public_key,
       membership_generation, inventory_arrival_head,
       authority_digest_version, authority_digest, prune_head,
       page_count, prune_count, target_count, through_prune_sequence,
       last_membership_generation, rolling_inventory_digest, inventory_digest
FROM continuity_sync_recovery_prune_candidates
WHERE project_id = ?`, string(projectID)).Scan(
		&candidateID, &state,
		&channelID, &relayGeneration, &adminPublicKey,
		&membershipGeneration, &candidate.Snapshot.Authority.InventoryArrivalHead,
		&authorityDigestVersion, &authorityDigest, &candidate.Snapshot.PruneHead,
		&candidate.PageCount, &candidate.PruneCount, &candidate.TargetCount, &candidate.ThroughPruneSequence,
		&lastMembershipGeneration, &rollingDigest, &inventoryDigest,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncRecoveryPruneCandidate{}, false, nil
	}
	if err != nil {
		return SyncRecoveryPruneCandidate{}, false, syncTransactionProblem(ctx)
	}
	if len(candidateID) != len(candidate.CandidateID) || len(channelID) != len(candidate.Snapshot.Authority.ChannelID) ||
		len(relayGeneration) != len(candidate.Snapshot.Authority.RelayGeneration) ||
		len(adminPublicKey) != len(candidate.Snapshot.Authority.AdminPublicKey) ||
		len(authorityDigest) != len(candidate.Snapshot.Authority.AuthorityDigest) ||
		len(rollingDigest) != len(candidate.RollingInventoryDigest) ||
		(inventoryDigest != nil && (len(inventoryDigest) != len(candidate.InventoryDigest) || isZeroDigestBytesV2(inventoryDigest))) ||
		membershipGeneration < 1 || membershipGeneration > math.MaxUint32 ||
		authorityDigestVersion != 2 || lastMembershipGeneration < 0 || lastMembershipGeneration > math.MaxUint32 ||
		(state != "staging" && state != "ready") {
		return SyncRecoveryPruneCandidate{}, false, corruptSyncRecoveryPruneCandidateV1("candidate checkpoint is malformed")
	}
	copy(candidate.CandidateID[:], candidateID)
	copy(candidate.Snapshot.Authority.ChannelID[:], channelID)
	copy(candidate.Snapshot.Authority.RelayGeneration[:], relayGeneration)
	copy(candidate.Snapshot.Authority.AdminPublicKey[:], adminPublicKey)
	copy(candidate.Snapshot.Authority.AuthorityDigest[:], authorityDigest)
	copy(candidate.RollingInventoryDigest[:], rollingDigest)
	copy(candidate.InventoryDigest[:], inventoryDigest)
	candidate.Snapshot.Authority.MembershipGeneration = uint32(membershipGeneration)
	candidate.Snapshot.Authority.AuthorityDigestVersion = uint16(authorityDigestVersion)
	candidate.LastMembershipGeneration = uint32(lastMembershipGeneration)
	candidate.Ready = state == "ready"
	if err := validateSyncRecoveryPruneCandidateV1(candidate); err != nil {
		return SyncRecoveryPruneCandidate{}, false, corruptSyncRecoveryPruneCandidateV1("candidate checkpoint is inconsistent")
	}
	return candidate, true, nil
}

func validateSyncRecoveryPruneCandidateAppendBoundaryV1(
	ctx context.Context,
	tx *sql.Tx,
	candidate SyncRecoveryPruneCandidate,
) error {
	if candidate.PruneCount == 0 {
		return nil
	}
	record, found, err := readSyncRecoveryPruneRecordV1(
		ctx, tx, candidate.ProjectID, candidate.CandidateID, candidate.ThroughPruneSequence,
	)
	if err != nil {
		return err
	}
	if !found || record.PruneSequence != candidate.ThroughPruneSequence ||
		record.MembershipGeneration != candidate.LastMembershipGeneration {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune append boundary does not match the checkpoint")
	}
	return nil
}

func validateSyncRecoveryPruneCandidateIndexV1(
	ctx context.Context,
	tx *sql.Tx,
	candidate SyncRecoveryPruneCandidate,
) error {
	var recordCount, firstSequence, lastSequence, indexedTargetCount int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*), COALESCE(MIN(prune_sequence), 0),
       COALESCE(MAX(prune_sequence), 0), COALESCE(SUM(target_count), 0)
FROM continuity_sync_recovery_prune_records
WHERE project_id = ? AND candidate_id = ?`,
		string(candidate.ProjectID), candidate.CandidateID[:],
	).Scan(&recordCount, &firstSequence, &lastSequence, &indexedTargetCount); err != nil {
		return syncTransactionProblem(ctx)
	}
	var targetCount int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ?`,
		string(candidate.ProjectID), candidate.CandidateID[:],
	).Scan(&targetCount); err != nil {
		return syncTransactionProblem(ctx)
	}
	if recordCount != candidate.PruneCount || indexedTargetCount != candidate.TargetCount ||
		targetCount != candidate.TargetCount {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune cardinality does not match the checkpoint")
	}
	if candidate.PruneCount == 0 {
		if firstSequence != 0 || lastSequence != 0 {
			return corruptSyncRecoveryPruneCandidateV1("empty prune index contains a sequence")
		}
		return nil
	}
	if firstSequence != 1 || lastSequence != candidate.ThroughPruneSequence {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune sequence coverage is incomplete")
	}
	var lastMembershipGeneration int64
	if err := tx.QueryRowContext(ctx, `
SELECT membership_generation
FROM continuity_sync_recovery_prune_records
WHERE project_id = ? AND candidate_id = ? AND prune_sequence = ?`,
		string(candidate.ProjectID), candidate.CandidateID[:], candidate.ThroughPruneSequence,
	).Scan(&lastMembershipGeneration); err != nil {
		return syncTransactionProblem(ctx)
	}
	if lastMembershipGeneration != int64(candidate.LastMembershipGeneration) {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune generation does not match the checkpoint")
	}
	var malformedRecordCardinality int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM (
  SELECT record.prune_sequence
  FROM continuity_sync_recovery_prune_records AS record
  LEFT JOIN continuity_sync_recovery_prune_targets AS target
    ON target.project_id = record.project_id
   AND target.candidate_id = record.candidate_id
   AND target.prune_sequence = record.prune_sequence
  WHERE record.project_id = ? AND record.candidate_id = ?
  GROUP BY record.prune_sequence, record.target_count
  HAVING COUNT(target.arrival_sequence) <> record.target_count
      OR MIN(target.target_ordinal) <> 1
      OR MAX(target.target_ordinal) <> record.target_count
)`, string(candidate.ProjectID), candidate.CandidateID[:]).Scan(&malformedRecordCardinality); err != nil {
		return syncTransactionProblem(ctx)
	}
	if malformedRecordCardinality != 0 {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune target ordinals are incomplete")
	}
	var malformedGenerations int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM continuity_sync_recovery_prune_records AS current
LEFT JOIN continuity_sync_recovery_prune_records AS previous
  ON previous.project_id = current.project_id
 AND previous.candidate_id = current.candidate_id
 AND previous.prune_sequence = current.prune_sequence - 1
WHERE current.project_id = ? AND current.candidate_id = ?
  AND (
    current.membership_generation > ?
    OR (
      current.prune_sequence > 1
      AND (
        previous.prune_sequence IS NULL
        OR previous.membership_generation > current.membership_generation
      )
    )
  )`,
		string(candidate.ProjectID), candidate.CandidateID[:], candidate.Snapshot.Authority.MembershipGeneration,
	).Scan(&malformedGenerations); err != nil {
		return syncTransactionProblem(ctx)
	}
	if malformedGenerations != 0 {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune generations are inconsistent")
	}
	var targetsBeyondSnapshot int64
	if err := tx.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM continuity_sync_recovery_prune_targets
WHERE project_id = ? AND candidate_id = ? AND arrival_sequence > ?`,
		string(candidate.ProjectID), candidate.CandidateID[:], candidate.Snapshot.Authority.InventoryArrivalHead,
	).Scan(&targetsBeyondSnapshot); err != nil {
		return syncTransactionProblem(ctx)
	}
	if targetsBeyondSnapshot != 0 {
		return corruptSyncRecoveryPruneCandidateV1("indexed prune target exceeds the pinned arrival prefix")
	}
	return nil
}

func requireNoActiveSyncRecoveryPruneCandidateV1(
	ctx context.Context,
	tx *sql.Tx,
	projectID continuity.ProjectID,
) error {
	_, found, err := readAndValidateSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if found {
		return syncProblem(SyncErrorConflict, "sync_recovery_prune_candidate", "requires the dedicated recovery prune workflow")
	}
	return nil
}

func corruptSyncRecoveryPruneCandidateV1(detail string) error {
	return syncProblem(SyncErrorStore, "sync_recovery_prune_candidate", detail)
}

func commitSyncRecoveryPruneCandidatePageV1(ctx context.Context, commit func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "recovery prune candidate page commit outcome is unknown")
	}
	return nil
}
