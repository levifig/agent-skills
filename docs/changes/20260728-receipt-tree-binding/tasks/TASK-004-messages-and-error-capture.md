---
change: receipt-tree-binding
id: TASK-004
title: Messages and error capture
blocked-by:
  - TASK-003
---

# TASK-004 — Messages and error capture

## Objective

Every receipt-related surface names the folder, the cause, and a copy-pasteable remedy: `commandOutput` captures stderr into returned errors (killing the bare `exit status N` class repo-wide), gate blocks read per the DX wordings (stale content naming drifted sections, criteria changed, unsupported schema — never "invalid" or "corrupt"), and drift refinement uses `scope_sections` ("content changed under `internal/`, `content/`") with no ancestry anywhere.

## Scope boundaries

**In:** `internal/cli/check.go` `commandOutput`, block/reason rendering from TASK-003's enum, gate output in `change_release_gate.go`, message tests.

**Out:** Predicate mechanics (TASK-003); anything in the release/promotion surface (the promotion change owns rc gating and ceremony output); skill prose.

## Context pointers

- Contract: `shape.md` — Observable Workflow; Failure-mode table.
- Council board: Portability/DX card — exact wordings for stale, criteria-changed, and old-schema blocks; the `exit status 128` support-ticket anatomy.
- `commandOutput`: `internal/cli/check.go:1391-1396`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — messages and error capture"
# Read internal/cli/check.go:1380-1400 and the TASK-003 enum rendering before editing.
```

## Steps

- [x] `commandOutput` includes captured stderr in returned errors; audit its call sites for double-printing.
- [x] Render the DX wordings from the reason enum; every block names folder, cause, and the `loaf change verify <folder>` + commit remedy.
- [x] Drift blocks name the drifted top-level sections from `scope_sections`.
- [x] Message tests: no raw `exit status` string reaches gate output in any fixture state; wordings match the contract.

## Verification

- `go test ./internal/cli -run 'TestChangeReceiptFreshness|TestReleaseCohort' -count=1` green with message assertions.
- H1 transcript over the fixtures reads as reasoned verdicts end to end.
