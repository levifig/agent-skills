---
change: change-work-model
id: TASK-014
title: Back the Verification Contract with fixtures
blocked-by:
  - TASK-007
  - TASK-008
  - TASK-009
---

# TASK-014 — Back the Verification Contract with fixtures

## Objective

Cover the V1, V2 and V5 assertions that no test currently makes. Several of these criteria read as satisfied — and TASK-005's checkbox for receipt expiry was flipped — while the suite asserts none of them. This task closes the residual gaps that belong to no single fix, after the fixes it depends on have landed.

## Scope boundaries

**In:** fixtures in `internal/cli/change_release_gate_test.go` and `change_verify_test.go` for the cases enumerated below.

**Out:** Cases owned by their fixing task: the failing-receipt negative (`TASK-007`), the criteria-form fixtures (`TASK-008`), the flip non-events (`TASK-009`), the conversion rule (`TASK-010`), the state-agreement fixture (`TASK-011`). Do not restate the Verification Contract — if a criterion turns out to be untestable as written, report it rather than quietly narrowing it.

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — V1, V2, V5 in full; each names cases the suite does not cover.
- Review board finding M6 and the criteria ledger, which records exactly which assertions are unbacked.
- `internal/cli/change_verify_test.go` — two tests today, the thinnest coverage on the most safety-critical path.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-014 — verification contract coverage"
```

## Steps

- [x] V1 residuals: a commit satisfying the path grade but not the flip grade still blocks; a second cohort member left shaped-only blocks identically; a change with no `target_release` never gates; the lower-cohort warning fires without blocking.
- [x] V2: `--bump prerelease` succeeds in every gate state — missing execution, missing receipt, failing receipt, expired receipt — and the same fixture's `--post-merge` finalization stays blocked until the cohort completes.
- [x] V5 residuals: editing `shape.md` criteria expires the receipt; editing `plan.md` leaves the digest intact and the gate reports the re-verify demand (never "receipt expired"); advancing HEAD past the verified commit with any non-receipt path blocks with that demand while the receipt's own commit alone does not; a retarget after verification triggers the re-verify demand rather than blind trust or permanent invalidation.
  - Present: criteria-expire, freshness + receipt-own-commit, retarget-after-verify, plan.md-staleness (named tests).
  - Previously reported untestable under the auto-re-run reading of plan.md immunity; R2-2 amended shape.md (Decision 13/V5) to block-with-re-verify-demand. INTENT-20260727-exempt-plan-md-from-receipt-freshness-re-run stays open as UX refinement.
- [x] Any criterion found untestable as written is reported in the commit body with what would make it testable — never silently dropped.

## Verification

- `go test ./internal/cli/` green with every fixture above present.
- V1, V2 and V5 each trace to at least one named test, and the criteria ledger on the round-1 board can be re-graded green.
