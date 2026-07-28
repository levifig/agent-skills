---
change: change-work-model
id: TASK-017
title: Gate reads the receipt from committed HEAD
blocks:
  - TASK-019
---

# TASK-017 — Gate reads the receipt from committed HEAD

## Objective

Close C3-3: `changeReceiptStatus` loads `receipts/verify.json` from the working tree, so an uncommitted receipt with `verified_commit == HEAD` satisfies the gate — evidence that exists on one machine only, contradicting ADR-023's "evidence is always committed before it is read." After this task, gate-context receipt reads come from committed HEAD, and an uncommitted receipt blocks with a commit-the-receipt remedy.

## Scope boundaries

**In:** receipt loading inside `changeReceiptStatus` in `internal/cli/change_verify.go` (~:256) — read via `git show HEAD:<path>` through the existing `outputCommand` seam so tests can drive it; a distinct block reason for present-but-uncommitted (suggested: `receipt not committed; run: git add <path> && commit`); fixtures in `internal/cli/change_verify_test.go` **only**.

**Out:** `loaf change verify`'s own read/write flow (filesystem, unchanged — it writes the receipt; committing is the operator's act the block message teaches). `changeReceiptStatus`'s signature (stable — the gate lane consumes it). Freshness and digest semantics (unchanged; they now operate on the committed receipt's content). Do not touch `change_release_gate.go`, `change_release_gate_test.go`, `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, or `package.json`.

## Context pointers

- Round-3 board finding C3-3; ADR-023 "The gate is a pure reader" and "evidence is always committed before it is read".
- The bootstrap rule stays: the receipt's own commit never stales it (existing fixtures assert this — they must stay green with HEAD-reads).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-017 — receipt reads from HEAD"
```

## Steps

- [ ] Gate-context receipt load reads `HEAD:<folder>/receipts/verify.json` via `outputCommand`; missing-at-HEAD distinguishes "missing receipt" (never verified) from "receipt not committed" (working-tree-only) with the remedy named.
- [ ] Freshness (`verified_commit` vs HEAD, later non-receipt paths) and digest expiry operate on the committed receipt; the receipt's-own-commit exemption stays green.
- [ ] Fixtures: uncommitted passing receipt blocks with the not-committed reason; the same receipt committed proceeds; a committed receipt with a dirty working tree still proceeds (working tree is irrelevant to the gate).

## Verification

- `go test ./internal/cli/ -run 'ChangeVerify|ReceiptFreshness'` green; existing `ReleaseCohortGate` fixtures (other lane's file, untouched) still pass at integration.
