---
change: receipt-tree-binding
id: TASK-003
title: Freshness predicate and typed verdicts
blocked-by:
  - TASK-001
  - TASK-002
blocks:
  - TASK-004
---

# TASK-003 — Freshness predicate and typed verdicts

## Objective

The commit walk is deleted and freshness becomes a pure function of receipt fields and the pinned HEAD tree: supported schema ∧ current exclusions ∧ current digest spec ∧ criteria digest matches `shape.md@HEAD` ∧ scope digest matches ∧ results cover the criteria ID set ∧ all passing. Every checker state maps to a reasoned block through a typed reason enum (missing, uncommitted, unreadable, unsupported schema, criteria mismatch, content drift naming sections, results gap, failing results); the `cannot inspect receipt` inspection error becomes unreachable code; no reachability is ever consulted for the verdict — diagnostics guard optional object reads with `git cat-file -e` and degrade to shorter prose, never different verdicts.

## Scope boundaries

**In:** `internal/cli/change_verify.go` read path (`changeReceiptStatus`, `changeReceiptAtHEAD`, `formatChangeReceiptBlock`), the reason enum, gate call sites in `change_release_gate.go` that consume the status, fixtures.

**Out:** Digest construction (TASK-001); write path (TASK-002); message wording polish and `commandOutput` (TASK-004) — land mechanics first, wording second.

## Context pointers

- Contract: `shape.md` — Planning Contract → Freshness predicate + Failure-mode table; Decisions 3, 4, 6, 7.
- Council board: Correctness card (predicate and failure table), Git Internals card (no-fallback rule, merge-mode table).
- Current walk: `change_verify.go:493-525`; prose dispatch: `formatChangeReceiptBlock`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — freshness predicate and typed verdicts"
# Read internal/cli/change_verify.go:430-560 and change_release_gate.go:60-113 before editing.
```

## Steps

- [ ] Replace `changeReceiptStatus` internals with the pure predicate; delete the `git log`/`diff-tree` walk and the reachability error branch.
- [ ] Introduce the typed reason enum; derive all rendering from it; `changeReceiptExistsInWorkingTree` demoted to a prose hint that can never change a verdict.
- [ ] Refuse v1/unknown-schema receipts with the named remedy; enforce `schema_version` reading explicitly.
- [ ] Fixtures: post-squash protocol-clone (verify on branch, squash-merge, delete branch, clone via `file://` transport, gate verifies green); N≥2 cohort receipts committed in one sweep commit both stay fresh; touch-then-revert inverse (byte-identical restore is fresh, with a comment recording Decision 4); every enum state produces its block and never an error.

## Verification

- `go test ./internal/cli -run 'TestChangeReceiptFreshness' -count=1` green.
- `go test ./...` green — the retired walk's old fixtures updated, none silently skipped.
