---
change: receipt-vouched-execution
id: TASK-001
title: Receipt-vouched execution grading
---

# TASK-001 — Receipt-vouched execution grading

## Objective

The release cohort gate grades a Change executed via flip-in-history **or** fresh-receipt-and-all-boxes-checked, refuses receipt-less squash shapes with a message naming cause and remedy, and proves both the new pass and the preserved attack-block in tests.

## Scope boundaries

**In:** `internal/cli/change_provenance.go`, `internal/cli/change_release_gate.go`, their test files.

**Out:** receipt semantics (`change_verify.go`, `change_receipt_status.go` stay untouched as evidence sources), display/render surfaces of provenance, artifact rebuild (TASK-002).

## Context pointers

- Contract: `shape.md` — Decisions 1 and 4, Planning Contract (approach, self-application)
- Scan: `internal/cli/change_provenance.go:28` (`changeFolderExecuted`), formatter at `:256`
- Gate composition: `internal/cli/change_release_gate.go` (`evaluateChangeReleaseGate` — receipt verdict already computed around line 88)
- Attack replay: the existing `targets 2.0.0 but is not executed` test in `internal/cli/change_release_gate_test.go:72`

## Acquisition

```bash
loaf journal log "skill(implement): TASK-001 — receipt-vouched execution grading"
```

## Steps

- [x] Add the disjunct: executed when flip-scan passes, or when the folder's receipt verdict is fresh and every task checkbox in the folder is checked at HEAD (reuse the existing receipt verdict; reuse or extract the unchecked-box reader)
- [x] Upgrade `formatChangeExecutionBlock`: checked packets + code + no vouching receipt names the squash cause and the `loaf change verify` remedy
- [x] Test matrix: squash shape + fresh receipt passes; shaping-only (no receipt) still blocks; stale/digest-mismatched receipt blocks; flip path passes with no receipt; existing attack replay untouched and green
- [x] Suite green

## Verification

- `go test ./internal/cli -run 'TestChangeExecut|TestReleaseCohort' -count=1` exits 0 (V1)
- `go test ./internal/cli -run TestReleaseGateBlocksShapingOnlyMerge -count=1` exits 0 (V3)
- `npm run test` exits 0 (V2)
