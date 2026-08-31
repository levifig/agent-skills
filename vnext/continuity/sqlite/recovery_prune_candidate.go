package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
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

// SyncRecoveryPruneCandidatePage is one bounded, already-verified checkpoint
// advance. ResultingRollingDigest and LastMembershipGeneration are cumulative
// values after this page. InventoryDigest is nonzero only on the final page.
type SyncRecoveryPruneCandidatePage struct {
	AfterPruneSequence       int64
	PagePruneCount           int64
	PageTargetCount          int64
	LastMembershipGeneration uint32
	ResultingRollingDigest   SyncRecoveryPruneRollingDigest
	InventoryDigest          SyncRecoveryPruneInventoryDigest
	More                     bool
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
	next, err := prepareSyncRecoveryPruneCandidateSuccessorV1(projectID, snapshot, expected, page)
	if err != nil {
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

	current, found, err := readAndValidateSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if expected == nil {
		if found {
			replayed := next
			replayed.CandidateID = current.CandidateID
			if current == replayed {
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
	} else {
		if !found {
			return SyncRecoveryPruneCandidate{}, syncProblem(SyncErrorConflict, "checkpoint", "active recovery prune candidate is missing")
		}
		if current == next {
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
	}

	persisted, found, err := readAndValidateSyncRecoveryPruneCandidateV1(ctx, tx, projectID)
	if err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if !found || persisted != next {
		return SyncRecoveryPruneCandidate{}, corruptSyncRecoveryPruneCandidateV1("candidate changed during staging")
	}
	if err := requireExactSyncRecoveryPruneSnapshotFenceV1(ctx, tx, projectID, snapshot); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	if err := commitSyncRecoveryPruneCandidatePageV1(ctx, tx.Commit); err != nil {
		return SyncRecoveryPruneCandidate{}, err
	}
	return persisted, nil
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
		string(projectID), checkpoint.CandidateID[:], state,
		current.Snapshot.Authority.ChannelID[:], current.Snapshot.Authority.RelayGeneration[:],
		current.Snapshot.Authority.AdminPublicKey[:], current.Snapshot.Authority.MembershipGeneration,
		current.Snapshot.Authority.InventoryArrivalHead, current.Snapshot.Authority.AuthorityDigestVersion,
		current.Snapshot.Authority.AuthorityDigest[:], current.Snapshot.PruneHead,
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
	if err := tx.Commit(); err != nil {
		return syncProblem(SyncErrorStore, "", "recovery prune candidate discard outcome is unknown")
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

func readAndValidateSyncRecoveryPruneCandidateV1(
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
