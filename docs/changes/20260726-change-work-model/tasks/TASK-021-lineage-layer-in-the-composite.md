---
change: change-work-model
id: TASK-021
title: Lineage validation joins the gate composite
blocks:
  - TASK-022
---

# TASK-021 — Lineage validation joins the gate composite

## Objective

Close C4-2: `runChangeCheck` applies `applyLineageValidation` (change.go:341) before the structural composite, but the gate's `changeCohortStructuralReport` (change_release_gate.go:117) skips it — so global structural findings, duplicate Change slugs foremost, never gate. Two date-distinct folders declaring the same slug can both verify and pass preflight while `check` fails both. After this task the gate's structural tier is byte-for-byte the same composite `check` applies.

## Scope boundaries

**In:** `changeCohortStructuralReport` in `internal/cli/change_release_gate.go` (it already has access to the loaded node set, or gains it via its caller); `internal/cli/change.go` only if the extraction needs a shared entry point; fixtures in `internal/cli/change_release_gate_test.go`.

**Out:** What `applyLineageValidation` itself validates (unchanged). The display ladder (TASK-022, which depends on this). The do-not-touch list: `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-4 board finding C4-2; TASK-016's thesis this completes: "the composite is shared with check, not re-implemented — one helper, two consumers."

## Acquisition

```bash
loaf journal log "skill(implement): TASK-021 — lineage layer in the composite"
```

## Steps

- [ ] The gate's structural report includes lineage validation over the full loaded node set, through the same helper `check` uses — one composite, two consumers, no drift surface left between them.
- [ ] Fixture: two folders with distinct dates and the same slug, both flip-executed with passing committed receipts, block at preflight with a structural finding naming the duplication; `check` on either folder reports the same finding.
- [ ] Fixture: the existing single-member happy path stays green (no over-blocking from lineage findings that belong to other changes — findings scope to the member being judged, matching check's scoping rules).

## Verification

- `go test ./internal/cli/ -run 'ReleaseCohortGate|ChangeCheck|Lineage'` green including both fixtures.
