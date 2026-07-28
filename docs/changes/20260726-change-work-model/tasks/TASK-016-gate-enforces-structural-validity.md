---
change: change-work-model
id: TASK-016
title: Gate enforces the full structural tier
blocked-by:
  - TASK-015
---

# TASK-016 — Gate enforces the full structural tier

## Objective

Close C3-2: the cohort gate blocks on `report.Violations` alone, so executability gaps, task-hygiene findings, and conversion findings never gate — a member with an empty Planning Contract but flips and a passing receipt releases while `loaf change check` fails on the same folder. After this task, "structurally valid" in the gate means what it means at `check`: the composite surface.

## Scope boundaries

**In:** the cohort loop in `internal/cli/change_release_gate.go` (~:59); extracting a shared composite-validity helper if `check` and the gate would otherwise duplicate logic; fixtures in `internal/cli/change_release_gate_test.go`.

**Out:** What `check` itself validates (unchanged). Warnings never block — only violations, task findings, conversion findings, and executability gaps. Receipt internals (verify lane). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`, or any test file other than `change_release_gate_test.go`.

## Context pointers

- Round-3 board finding C3-2; shape.md Scope: cohort members must be "materialized, **structurally valid**, executed at the flip grade, and receipt-verified".
- Decision 15 nuance: executability is contract-section completeness, never checkbox completion — an unchecked task on a verified member stays legal descoped work; do not gate on checkbox counts.
- `evaluateChangeNode` (violations/executability) and `loadChangeTasks` (task findings, produced outside `evaluateChangeNode`) — the split that made this hole.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-016 — gate structural tier"
```

## Steps

- [ ] Cohort members block when the composite structural surface fails: violations, task-hygiene findings, conversion findings, or executability gaps — with block messages distinguishing "structurally invalid" from "not executable (contract gaps: …)".
- [ ] The composite is shared with `check`, not re-implemented — one helper, two consumers.
- [ ] Fixtures: a member with an empty Planning Contract (gap, zero violations) blocks; a member with banned task frontmatter blocks; warnings alone never block; a fully valid member with flips and receipt still proceeds.

## Verification

- `go test ./internal/cli/ -run 'ReleaseCohortGate|ChangeCheck'` green including the new fixtures.
- On this repo: `loaf change check docs/changes/20260726-change-work-model` and the gate agree about this change's own folder.
