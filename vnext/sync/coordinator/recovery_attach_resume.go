package coordinator

import (
	"context"
	"math"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/credential"
)

// revalidateRecoveryAttachPruneInventory reopens the complete pinned prune
// inventory under the credential supplied to this attempt and compares every
// authenticated page with the immutable READY local projection. This prevents
// a terminal prefix staged under one project root from being promoted by a
// later attempt carrying another structurally valid root.
func (coordinator *Coordinator) revalidateRecoveryAttachPruneInventory(
	ctx context.Context,
	projectID continuity.ProjectID,
	prepared credential.TrustedProjectCredential,
	binding continuitysqlite.SyncAuthorityBinding,
	expected continuitysqlite.SyncRecoveryPruneCandidate,
) error {
	if !expected.Ready || expected.ProjectID != projectID || expected.Snapshot.Authority != binding {
		return newProblem(CodeInternal, PhaseAttachActivation, ActionRepairLocalStore)
	}
	var pageCount, pruneCount, targetCount int64
	result, err := coordinator.scanRecoveryPruneInventory(
		ctx, projectID, prepared, binding,
		recoveryPruneInventoryScanOptions{onPage: func(page verifiedRecoveryPruneInventoryPage) error {
			snapshot, persistedPage, pageErr := recoveryPruneCandidatePageFromVerifiedV1(binding, page)
			if pageErr != nil || snapshot != expected.Snapshot {
				return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
			}
			if verifyErr := coordinator.store.VerifySyncRecoveryPruneCandidatePageRecords(
				ctx, projectID, snapshot, expected.Checkpoint(), persistedPage,
			); verifyErr != nil {
				return mapRecoveryPruneInventoryStoreError(ctx, verifyErr)
			}
			if pageCount == math.MaxInt64 || pruneCount > math.MaxInt64-persistedPage.PagePruneCount ||
				targetCount > math.MaxInt64-persistedPage.PageTargetCount {
				return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
			}
			pageCount++
			pruneCount += persistedPage.PagePruneCount
			targetCount += persistedPage.PageTargetCount
			return nil
		}},
	)
	if err != nil {
		return err
	}
	if expected.PageCount != pageCount || expected.PruneCount != pruneCount || expected.TargetCount != targetCount ||
		expected.Snapshot.PruneHead != result.snapshot.PruneHead ||
		expected.ThroughPruneSequence != result.checkpoint.throughPruneSequence ||
		expected.LastMembershipGeneration != result.checkpoint.lastMembershipGeneration ||
		expected.RollingInventoryDigest != continuitysqlite.SyncRecoveryPruneRollingDigest(result.checkpoint.rollingDigest) ||
		expected.InventoryDigest != continuitysqlite.SyncRecoveryPruneInventoryDigest(result.inventoryDigest) {
		return newProblem(CodeConflict, PhasePruneInventory, ActionRestartRecovery)
	}
	return nil
}
