---
change: change-work-model
id: TASK-007
title: Receipt attests success, not merely execution
blocked-by:
  - TASK-008
relates-to:
  - TASK-005
---

# TASK-007 — Receipt attests success, not merely execution

## Objective

Close the gate hole where a receipt written by a *failing* `loaf change verify` satisfies the cohort gate. After this task, a cohort member whose receipt records any failing criterion blocks the stable cut, and the failure is stated plainly enough that re-verification is mechanical.

## Scope boundaries

**In:** `changeReceiptStatus` and `runChangeVerify` in `internal/cli/change_verify.go`; the block message vocabulary in `formatChangeExecutionBlock`'s neighbourhood (`change_provenance.go`) if a new reason string belongs there; the negative fixture; removal of the dead freshness code described below.

**Out:** Criteria parsing and cwd (`TASK-008` owns both). Derived state names (`TASK-011`). Flip grammar (`TASK-009`). Do not add anti-forgery machinery — a hand-authored receipt stays out of threat model per Rabbit Holes; this task closes the *accident* path only.

## Context pointers

- `docs/changes/20260726-change-work-model/shape.md` — Scope ("executed at the flip grade, and receipt-verified"), Decision 13, V5.
- `docs/changes/20260726-change-work-model/reports/20260727-234403-review-implementation-round-1.html` — finding B1 carries the reproduction transcript, and m2 the dead-code detail.
- `internal/cli/change_verify.go:110-138` (write-then-exit-1) and `:229-300` (`changeReceiptStatus`).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-007 — receipt attests success"
```

Reproduce before fixing — the bug is invisible from the unit tests:

```bash
d=$(mktemp -d) && cd "$d" && git init -q . && export LOAF_DB="$d/loaf.sqlite"
# fixture: prerelease version file, a change declaring target_release, one criterion that fails,
# a flip commit touching an outside path, then commit the failing receipt and cut stable.
# Expect today: "New version: 1.0.0". Expect after this task: release blocked.
```

## Steps

- [x] Gate rejects a receipt whose `results` contain any entry with `ok: false`, with a distinct reason (suggested: `receipt records failing criteria (V1, V3)` naming the failed IDs) so the operator knows what to re-run rather than only that something is stale.
- [x] Decide and implement the write-on-failure policy: either refuse to write the receipt when criteria fail, or write it and rely on the gate check above. Prefer writing it — the evidence is useful and the gate is now the arbiter — but say which you chose in the commit body.
- [x] Delete the dead `logAll` computation and its malformed double-`--pretty` `git log` invocation; collapse the per-commit folder loop and the repo-wide diff into one freshness check.
- [x] Freshness uses commit-by-commit inspection rather than a `verified..HEAD` tree diff, so a touch-then-revert pair still forces a re-run as Decision 13 requires.
- [x] Negative fixture: a committed receipt with a failing criterion blocks finalization; the same fixture with all criteria passing proceeds.
- [x] Fixture: a commit touching only the receipt path does not stale it (the bootstrap case Decision 13 protects), while any other later path does.

## Verification

- `go test ./internal/cli/ -run 'ReleaseCohortGate|ChangeVerify'` green, including the two new fixtures.
- The manual reproduction above now prints a block instead of `New version: 1.0.0`.
- V5's receipt semantics hold: digest binds to `shape.md` criteria, `plan.md` edits do not expire, the receipt's own commit never stales.
