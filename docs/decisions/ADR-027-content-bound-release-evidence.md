---
id: ADR-027
title: "Content-bound release evidence — receipts vouch where history shape cannot"
status: Accepted
date: 2026-08-07
supersedes: null
superseded_by: null
related:
  - ADR-022
  - ADR-023
  - ADR-024
  - ADR-026
---

# ADR-027: Content-bound release evidence

## Context

Cutting `0.2.20` — the first *stable* candidate ever evaluated by the release cohort gate — surfaced a latent contradiction between two evidence systems that had never been tested together. Verify receipts bind content: they digest the scope tree they verified (ADR-024), which is why they survived the squash merge that landed the versioning reset. Execution provenance bound history shape: `changeFolderExecuted` scanned `git log HEAD -- <folder>` for a commit whose diff flips `- [ ]` → `- [x]` while touching code outside `docs/changes/` (ADR-023). The squash merge preserved every byte of the tree and rewrote every commit of the history — the receipt sailed through, the flip evidence ceased to exist, and a fully implemented, receipt-verified Change was refused as `targets 0.2.20 but is not executed`.

The gap hid for two months because prerelease candidates only warn and the one prior stable-path arc fast-forwarded (28 linear commits, flips intact). It was going to bite every squash-merged stable-target Change from then on, and the operator's requirement was explicit: all merge strategies are first-class, including strategies not yet adopted — a squash-cleanup-then-merge-commit flow can manufacture the same broken shape before the merge commit ever happens.

A second instance of the same class followed immediately. Post-merge guardrail 5 demanded the release commit diff both `CHANGELOG.md` and a version file — the shape only ceremony-minted release commits have. A *self-carrying* release, where the version flip landed as the Change's own receipt-verified content (the reset itself), can never honestly produce that commit, while guardrail 4 had already proven every version file equals the candidate.

## Decision

**Evidence that gates a release binds content — trees and receipts — never history shape.** History-derived signals may enrich display and remain a valid fast path, but no release gate refuses on the shape of history alone when content-bound evidence vouches. Three concrete rules implement this:

1. **Execution grade is a disjunct.** A cohort member grades executed when a flip transition exists in ancestry (the ADR-023 rule, still first) **or** when its verify receipt is fresh against HEAD and every committed task checkbox is checked. A receipt is unfakeable without the implementation in the tree — `loaf change verify` only passes against actual content — so the shaping-only attack ADR-023 guards against stays blocked: checked boxes with nothing vouching for them refuse, and a stale or digest-mismatched receipt vouches for nothing. Refusals for the receipt-less squash shape name the cause and the one-command remedy (PR #154).

2. **Release commits may be changelog-only under proof.** Post-merge guardrail 5 accepts a release commit with no version-file diff exactly when guardrail 4's own comparison attests every version file already equals the candidate — the same predicate, handed down, never recomputed. The changelog demand never relaxes (PR #155).

3. **Cohort receipts re-verify at the final tree.** In a multi-Change cohort, a later Change's content commits stale every earlier member's receipt, so all members re-verify on the last branch before merge — receipt commits are content-free and digest-excluded, so the ordering terminates, and receipts re-minted pre-squash carry through the squash unchanged (proven: the recomputed post-squash digest was byte-identical to the pre-squash receipt's).

## Consequences

- Fast-forward, merge-commit, rebase, squash, and any cleanup-then-merge hybrid produce identical gate outcomes, by construction rather than by convention. The merge strategy is a style choice again.
- The flip discipline — commit packets unchecked, flip in delivering commits — remains best practice for in-branch auditability, but is no longer load-bearing at the release gate. One structural exception stands: a Change's final flip must precede `loaf change verify`, because the receipt digests the tree and `tasks/**` is not excluded.
- Guardrail 5's version-file branch is unreachable in the composed pipeline (guardrail 4 aborts first) and survives as the function's own contract, pinned by tests — recorded plainly rather than implied to guard production.
- The first stable candidate through any gate is that gate's first real test. This arc is the second instance of the pattern (the alpha.16 evidence-canary incident was the first); gates changed since the last stable cut should expect their collision at the next one.

## Evidence

- PR #153 refused post-squash with `targets 0.2.20 but is not executed`; PR #154 (receipt-vouched execution) and PR #155 (guardrail relaxation) each landed through adversarial review gates, including an independently re-run falsification of the guardrail fix; the `v0.2.20` ceremony then passed all nine post-merge guardrails on a changelog-only release commit — both rules proven live in one run.
- Journal: `finding(release-gate)` and `decision(release-gate)` entries of 2026-08-07; `finding(cohort)` recording the receipt-ordering rule; change records `docs/changes/20260806-receipt-vouched-execution/` and `docs/changes/20260807-self-carried-release-guardrail/`.
