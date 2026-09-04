package coordinator

import (
	"context"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
	"github.com/levifig/loaf/vnext/sync/relay"
)

// persistRecoveryPruneInventory verifies one exact relay prune snapshot while
// advancing its secret-free durable checkpoint after every complete page.
// Retrying after an interrupted or commit-ambiguous attempt resumes from the
// exact retained cursor and rolling digest.
func (coordinator *Coordinator) persistRecoveryPruneInventory(
	ctx context.Context,
	expectedProjectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
	binding continuitysqlite.SyncAuthorityBinding,
) (continuitysqlite.SyncRecoveryPruneCandidate, error) {
	if ctx == nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeInvalid, PhasePruneInventory, ActionCorrectInput)
	}
	if err := ctx.Err(); err != nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, err
	}
	if expectedProjectID.Validate() != nil || prepared.Validate() != nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionCorrectInput)
	}
	if coordinator == nil || coordinator.store == nil || nilRemote(coordinator.remote) {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	writerEnvironmentID := coordinator.store.WriterEnvironmentID()
	if writerEnvironmentID.Validate() != nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRepairLocalStore)
	}
	if expectedProjectID != prepared.ProjectID || coordinator.remote.Endpoint() != prepared.RelayURL ||
		writerEnvironmentID != prepared.Certificate.EnvironmentID {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeInvalid, PhaseRecoveryValidation, ActionRestartRecovery)
	}
	if !recoveryDownloadBindingMatchesCredential(binding, prepared) {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	}

	current, found, err := coordinator.store.CurrentSyncRecoveryPruneCandidate(ctx, expectedProjectID)
	if err != nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, mapRecoveryPruneInventoryStoreError(ctx, err)
	}
	revalidatePersistedPrefix := false
	if found {
		if current.ProjectID != expectedProjectID || current.Snapshot.Authority != binding {
			return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
		}
		if _, checkpointErr := recoveryPruneInventoryCheckpointFromCandidateV1(expectedProjectID, binding, current); checkpointErr != nil {
			return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
		}
		revalidatePersistedPrefix = true
	}
	var expectedPreflight *continuitysqlite.SyncRecoveryPruneCandidate
	if found {
		expectedPreflight = &current
	}
	if err := coordinator.store.VerifySyncRecoveryPrunePreflight(
		ctx, expectedProjectID, binding, expectedPreflight,
	); err != nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, mapRecoveryPruneInventoryStoreError(ctx, err)
	}

	persisted := current
	havePersisted := found
	var revalidatedPageCount, revalidatedPruneCount, revalidatedTargetCount int64
	result, err := coordinator.scanRecoveryPruneInventory(
		ctx,
		expectedProjectID,
		prepared,
		binding,
		recoveryPruneInventoryScanOptions{
			onPage: func(page verifiedRecoveryPruneInventoryPage) error {
				snapshot, persistedPage, pageErr := recoveryPruneCandidatePageFromVerifiedV1(binding, page)
				if pageErr != nil {
					return newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
				}
				if revalidatePersistedPrefix {
					if snapshot != current.Snapshot {
						return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
					}
					if verifyErr := coordinator.store.VerifySyncRecoveryPruneCandidatePageRecords(
						ctx, expectedProjectID, snapshot, current.Checkpoint(), persistedPage,
					); verifyErr != nil {
						return mapRecoveryPruneInventoryStoreError(ctx, verifyErr)
					}
					if revalidatedPageCount == math.MaxInt64 ||
						revalidatedPruneCount > math.MaxInt64-persistedPage.PagePruneCount ||
						revalidatedTargetCount > math.MaxInt64-persistedPage.PageTargetCount {
						return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
					}
					revalidatedPageCount++
					revalidatedPruneCount += persistedPage.PagePruneCount
					revalidatedTargetCount += persistedPage.PageTargetCount
					if page.checkpoint.throughPruneSequence < current.ThroughPruneSequence {
						return nil
					}
					if page.checkpoint.throughPruneSequence != current.ThroughPruneSequence || page.more == current.Ready ||
						current.PageCount != revalidatedPageCount || current.PruneCount != revalidatedPruneCount ||
						current.TargetCount != revalidatedTargetCount ||
						current.LastMembershipGeneration != page.checkpoint.lastMembershipGeneration ||
						current.RollingInventoryDigest != continuitysqlite.SyncRecoveryPruneRollingDigest(page.checkpoint.rollingDigest) ||
						(current.Ready && current.InventoryDigest != continuitysqlite.SyncRecoveryPruneInventoryDigest(page.inventoryDigest)) {
						return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
					}
					revalidatePersistedPrefix = false
					return nil
				}
				var expected *continuitysqlite.SyncRecoveryPruneCandidateCheckpoint
				if havePersisted {
					checkpoint := persisted.Checkpoint()
					expected = &checkpoint
				}
				before := persisted
				next, storeErr := coordinator.store.StageVerifiedSyncRecoveryPruneCandidatePage(
					ctx, expectedProjectID, snapshot, expected, persistedPage,
				)
				if storeErr != nil {
					return mapRecoveryPruneInventoryStoreError(ctx, storeErr)
				}
				if !exactRecoveryPruneCandidatePageAdvanceV1(expectedProjectID, before, havePersisted, next, snapshot, persistedPage) {
					return newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
				}
				persisted = next
				havePersisted = true
				return nil
			},
		},
	)
	if err != nil {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, err
	}
	if revalidatePersistedPrefix || !havePersisted || !persisted.Ready || persisted.ProjectID != expectedProjectID ||
		persisted.Snapshot.Authority != binding ||
		persisted.Snapshot.PruneHead != result.snapshot.PruneHead ||
		persisted.ThroughPruneSequence != result.checkpoint.throughPruneSequence ||
		persisted.LastMembershipGeneration != result.checkpoint.lastMembershipGeneration ||
		persisted.RollingInventoryDigest != continuitysqlite.SyncRecoveryPruneRollingDigest(result.checkpoint.rollingDigest) ||
		persisted.InventoryDigest != continuitysqlite.SyncRecoveryPruneInventoryDigest(result.inventoryDigest) {
		return continuitysqlite.SyncRecoveryPruneCandidate{}, newProblem(CodeInternal, PhasePruneInventory, ActionRepairLocalStore)
	}
	return persisted, nil
}

func recoveryPruneInventoryCheckpointFromCandidateV1(
	projectID continuity.ProjectID,
	binding continuitysqlite.SyncAuthorityBinding,
	candidate continuitysqlite.SyncRecoveryPruneCandidate,
) (recoveryPruneInventoryCheckpoint, error) {
	if candidate.ProjectID != projectID || candidate.Snapshot.Authority != binding {
		return recoveryPruneInventoryCheckpoint{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	snapshot := relay.PruneInventorySnapshot{
		MembershipGeneration: candidate.Snapshot.Authority.MembershipGeneration,
		ArrivalHead:          candidate.Snapshot.Authority.InventoryArrivalHead,
		PruneHead:            candidate.Snapshot.PruneHead,
	}
	checkpoint, err := newRecoveryPruneInventoryCheckpointV1(projectID, binding, snapshot)
	if err != nil {
		return recoveryPruneInventoryCheckpoint{}, err
	}
	checkpoint.throughPruneSequence = candidate.ThroughPruneSequence
	checkpoint.lastMembershipGeneration = candidate.LastMembershipGeneration
	checkpoint.rollingDigest = recoveryPruneInventoryRollingDigest(candidate.RollingInventoryDigest)
	if err := validateRecoveryPruneInventoryCheckpointV1(projectID, binding, checkpoint); err != nil {
		return recoveryPruneInventoryCheckpoint{}, err
	}
	if candidate.Ready {
		final, err := finalizeRecoveryPruneInventoryDigestV1(checkpoint)
		if err != nil || continuitysqlite.SyncRecoveryPruneInventoryDigest(final) != candidate.InventoryDigest {
			return recoveryPruneInventoryCheckpoint{}, errInvalidRecoveryPruneInventoryDigestV1
		}
	}
	return checkpoint, nil
}

func recoveryPruneCandidatePageFromVerifiedV1(
	binding continuitysqlite.SyncAuthorityBinding,
	page verifiedRecoveryPruneInventoryPage,
) (continuitysqlite.SyncRecoveryPruneSnapshot, continuitysqlite.SyncRecoveryPruneCandidatePage, error) {
	if page.snapshot.MembershipGeneration != binding.MembershipGeneration ||
		page.snapshot.ArrivalHead != binding.InventoryArrivalHead || len(page.prunes) > relay.MaxPruneInventoryPage {
		return continuitysqlite.SyncRecoveryPruneSnapshot{}, continuitysqlite.SyncRecoveryPruneCandidatePage{}, errInvalidRecoveryPruneInventoryDigestV1
	}
	var targetCount int64
	records := make([]continuitysqlite.VerifiedSyncRecoveryPruneRecord, len(page.prunes))
	for pruneIndex, prune := range page.prunes {
		targetCount += int64(len(prune.targets))
		targets := make([]continuitysqlite.VerifiedSyncRecoveryPruneTarget, len(prune.targets))
		for targetIndex, target := range prune.targets {
			targets[targetIndex] = continuitysqlite.VerifiedSyncRecoveryPruneTarget{
				Reference: target.reference,
				FactKind:  target.factKind,
				HLC:       target.hlc,
			}
		}
		records[pruneIndex] = continuitysqlite.VerifiedSyncRecoveryPruneRecord{
			PruneSequence:        prune.pruneSequence,
			PruneID:              [32]byte(prune.pruneID),
			PruneCertificateID:   [32]byte(prune.pruneCertificateID),
			MembershipGeneration: prune.membershipGeneration,
			Targets:              targets,
		}
	}
	return continuitysqlite.SyncRecoveryPruneSnapshot{
		Authority: binding,
		PruneHead: page.snapshot.PruneHead,
	}, continuitysqlite.SyncRecoveryPruneCandidatePage{
		AfterPruneSequence:       page.afterPruneSequence,
		PagePruneCount:           int64(len(page.prunes)),
		PageTargetCount:          targetCount,
		LastMembershipGeneration: page.checkpoint.lastMembershipGeneration,
		ResultingRollingDigest:   continuitysqlite.SyncRecoveryPruneRollingDigest(page.checkpoint.rollingDigest),
		InventoryDigest:          continuitysqlite.SyncRecoveryPruneInventoryDigest(page.inventoryDigest),
		More:                     page.more,
		Records:                  records,
	}, nil
}

func exactRecoveryPruneCandidatePageAdvanceV1(
	projectID continuity.ProjectID,
	before continuitysqlite.SyncRecoveryPruneCandidate,
	haveBefore bool,
	next continuitysqlite.SyncRecoveryPruneCandidate,
	snapshot continuitysqlite.SyncRecoveryPruneSnapshot,
	page continuitysqlite.SyncRecoveryPruneCandidatePage,
) bool {
	if next.ProjectID != projectID || next.Snapshot != snapshot || next.Ready != !page.More ||
		next.ThroughPruneSequence != page.AfterPruneSequence+page.PagePruneCount ||
		next.PruneCount != next.ThroughPruneSequence ||
		next.LastMembershipGeneration != page.LastMembershipGeneration ||
		next.RollingInventoryDigest != page.ResultingRollingDigest ||
		next.InventoryDigest != page.InventoryDigest {
		return false
	}
	if !haveBefore {
		return next.PageCount == 1 && next.TargetCount == page.PageTargetCount
	}
	if before.Ready {
		return next == before
	}
	return next.ProjectID == before.ProjectID && next.CandidateID == before.CandidateID &&
		next.PageCount == before.PageCount+1 && next.TargetCount == before.TargetCount+page.PageTargetCount
}
