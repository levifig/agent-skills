<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Receipt-Vouched Execution

## Problem

The release cohort gate's execution-provenance check is history-bound: `changeFolderExecuted` scans `git log HEAD -- <folder>` for a commit whose diff flips `- [ ]` → `- [x]` while touching code outside `docs/changes/`. Squash merges rewrite that history — the task packets arrive on main created-already-checked, the shape ADR-023 deliberately grades as "not executed" because it is indistinguishable from a shaping-only merge. Result: `loaf release --post-merge` blocked the fully-implemented, receipt-verified 0.2.20 reset with `targets 0.2.20 but is not executed`. The defect hid for two months because prerelease candidates only warn and the one prior stable-path arc fast-forwarded; every squash-merged stable-target Change from 0.2.20 onward hits the wall.

## Hypothesis

If execution evidence anchors to what every merge strategy preserves (content: trees and receipts) instead of what most strategies rewrite (history shape), the gate is correct under fast-forward, merge-commit, rebase, squash, and any cleanup-then-merge hybrid — by construction, including strategies not yet chosen — while remaining at least as attack-resistant as the flip scan it supplements.

## Scope

**In**

- Gate rule change: a cohort member grades executed when **either** a flip transition exists in ancestry (current rule, kept for fast-forward/merge-commit flows) **or** the folder's verify receipt is fresh against HEAD **and** every task checkbox in the folder is checked. The receipt gate at `evaluateChangeReleaseGate` already computes freshness; the execution grade reuses that verdict rather than recomputing.
- Refusal-message upgrade: when packets are checked and code is present but no receipt vouches, the block names the likely squash cause and the remedy (`loaf change verify`, commit the receipt).
- Tests: the shaping-only attack still blocks (checked boxes, no receipt; stale receipt; receipt digest mismatch), the squash shape passes with a fresh receipt, the flip path still passes without a receipt, and empty-tasks edge cases keep their current grading.
- Artifact rebuild and capability-evidence re-record against the new binary (the alpha.16 lesson), so the 0.2.20 ceremony's evidence gate stays green.
- A `[Unreleased]` changelog line, riding into the 0.2.20 section.

**Out** (deferred, not rejected)

- Ship-skill advisory about merge strategies — unnecessary once the gate is strategy-proof; revisit only if a future evidence kind becomes history-bound again.
- Demoting the flip scan's *display* surfaces — grading only; whatever renders provenance today keeps rendering it.
- The 0.2.20 ceremony itself — runs after this lands, per the reset Change's runbook.

**Cut** (explicitly rejected)

- Scanning PR refs (`refs/pull/N/head`) for lost flips: reintroduces history-shape dependence plus a remote-state dependence, and dies the day a PR ref is garbage-collected.
- Weakening the shaping-only defense: creation-already-checked without a fresh receipt stays blocked, exactly as ADR-023 intends.
- Manufactured provenance (repair commits that re-flip boxes): the workaround class this fix exists to make unnecessary.

## Observable Workflow

- A squash-merged Change with a fresh committed receipt and all boxes checked releases cleanly: `loaf release --post-merge` proceeds where it refused today.
- A shaping-only merge — checked boxes, no work — still refuses, with the same certainty as before.
- The refusal for a receipt-less squash names the cause and the remedy instead of the bare "is not executed".
- This Change itself squash-merges and then passes the gate it fixed — the landing is the validation.

## Rabbit Holes and No-Gos

- **Generalizing the receipt system.** The receipt already exists and already binds trees; this Change consumes its verdict, it does not extend receipt semantics.
- **Re-litigating flip discipline.** "Commit packets unchecked, flip in delivering commits" stays best practice for in-branch audit; it just stops being load-bearing at the release gate.
- **Empty Verification Contracts.** A Change with no V-entries mints a trivially-passing receipt; structural executability (`loaf change check`) already polices contract emptiness, and this Change does not add a second police.

## Decisions

Provenance: incident diagnosis and operator interview, this session (journal `finding(release-gate)` and `decision(release-gate)` entries of 2026-08-06).

1. **Executed := flip-in-history OR (fresh receipt AND all boxes checked).** The receipt is the discriminator the attack cannot fake: it only verifies when the implementation is in the tree. Forecloses history-shape dependence for gating.
2. **All merge strategies are first-class.** Operator requirement stated verbatim; the fix must hold for strategies not yet adopted (squash-cleanup-then-merge-commit named as imminent).
3. **Fix lands before 0.2.20.** The blocked release ships through the fixed gate; the fix Change targets 0.2.20 and rides its cohort.
4. **The flip scan survives as the first grading path.** Cheap, still correct where history preserves it, and keeps the provenance display meaningful; the receipt path is the strategy-proof floor, not a replacement.

## Planning Contract

### Approach

`changeFolderExecuted` (internal/cli/change_provenance.go) keeps its scan; the gate's caller composes the new disjunct. The receipt-freshness verdict already computed by the cohort preflight (`evaluateChangeReleaseGate`, internal/cli/change_release_gate.go) is passed into — or evaluated beside — the execution grading so freshness is checked once. All-boxes-checked is a read of the folder's task files at HEAD (the unchecked-box scan already exists for `loaf change check`'s completeness reporting; reuse it). The refusal formatter (`formatChangeExecutionBlock`, internal/cli/change_provenance.go:256) gains the receipt-aware remedy branch. Go tests sit beside the existing gate tests in internal/cli/change_release_gate_test.go and change_provenance tests; the attack replay test (`targets 2.0.0 but is not executed`) keeps passing untouched where no receipt exists.

### Sequencing and self-application

This Change merges by squash and must pass its own fixed gate: implement and flip boxes first, run `loaf change verify` after the final flip, and land the receipt as the branch's last, content-free commit (the receipt digests the tree; a later content commit stales it — proven during the reset Change).

### Risks

- **Go changes stale the binary-pinned capability receipts** — re-record rides TASK-002, before merge, as the reset did.
- **Gate code reviewing its own admission ticket**: the fix is validated by tests replaying both the attack and the squash shape, then empirically by this Change's own landing.

## Implementation Units

- **TASK-001 — Receipt-vouched execution grading.** The disjunct, the refusal-message upgrade, and the test matrix (attack blocked, squash+receipt passes, flip path passes, stale/mismatched receipt blocked).
- **TASK-002 — Rebuild and evidence re-record.** `npm run build`, capability evidence re-recorded against the new binary, changelog line, readiness for the squash-merge landing (verify + receipt as final content-free commit).

## Verification Contract

- **V1.** The new grading is tested. Command: `go test ./internal/cli -run 'TestChangeExecut|TestReleaseCohort' -count=1`. Expect: exit 0.
- **V2.** The whole suite is green. Command: `npm run test`. Expect: exit 0.
- **V3.** The shaping-only attack still blocks. Command: `go test ./internal/cli -run TestReleaseGateBlocksShapingOnlyMerge -count=1`. Expect: exit 0.

Human review (H-tier):

- **H1.** A reviewer confirms the refusal message for a receipt-less squash names the cause and remedy.
- **H2.** After this Change's own squash-merge lands, `loaf release --post-merge` on main proceeds past the execution gate for the 0.2.20 cohort — the empirical validation.

## Definition of Done

- V1–V3 pass and the verify receipt is committed pre-merge as the final content-free commit.
- The Change squash-merges to main and the 0.2.20 cohort gate passes on main (H2).
- The 0.2.20 ceremony is unblocked (its own runbook then proceeds under the reset Change).

## Durable Outputs

- ADR recording content-bound execution evidence as the gate's foundation (supplements ADR-023's provenance model; authored post-implementation if the reviewer or reflect judges it architecturally significant — the grading rule may be small enough to live as this Change's record alone).

## Open Questions

- [KU] Whether the all-boxes-checked reader can be reused verbatim from change check's completeness scan or needs a folder-level helper → TASK-001, implementer's call.
