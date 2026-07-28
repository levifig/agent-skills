---
change: change-work-model
id: TASK-025
title: Committed-HEAD content source for every evidence surface
---

# TASK-025 — Committed-HEAD content source for every evidence surface

## Objective

Close C5-3 as a class and resolve `INTENT-20260728-gate-structural-tier-reads-task-files-from-the-working-tree-instead-of-committed-head`. The split becomes explicit and total: `loaf change check` reads the working tree (author feedback, unchanged); the release gate and the `verified` state rung read committed HEAD (evidence) — nodes, task files, everything. No evidence surface reads the filesystem again.

## Scope boundaries

**In:** `changeStructurallyCleanForState` in `internal/cli/change_state.go` (HEAD nodes via the existing loader); the structural composite's task-file reads when invoked for gate/state — thread a content source through `composeChangeCheckReport`/`loadChangeTasks` so check passes working-tree reads and gate/state pass HEAD reads (`readCommittedOptional` is the existing seam); fixtures in `internal/cli/change_state_test.go` and `internal/cli/change_release_gate_test.go`.

**Out:** `check`'s own working-tree semantics (unchanged by design — say so in the commit body). Receipt reads (already HEAD since TASK-017). Do not touch `bin/`, `plugins/`, `dist/`, `config/target-capabilities.json`, `package.json`.

## Context pointers

- Round-5 board C5-3; the Intent being resolved (its body names `applyChangeStructuralFindings` as the seam); TASK-017's HEAD-read precedent for receipts.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-025 — HEAD content source for evidence"
```

## Steps

- [x] The verified rung's structural guard loads nodes at HEAD (`loadChangeNodesAtHEADWithOutput`) and evaluates the member's committed content — a dirty working tree cannot change the guard's verdict in either direction.
- [x] The composite's task-file input takes a content source: working tree for `check`, committed HEAD for gate and state — one composite, two explicit sources, no silent filesystem fallback on the evidence path.
- [x] Fixtures: an uncommitted rename of one duplicate-slug folder changes neither gate nor state verdicts (both still see the committed duplication) while `check` sees the working tree; an uncommitted banned-frontmatter edit does not demote a committed-clean member's state; a task folder deleted only in the working tree still yields its committed findings on the evidence path.
- [x] After landing, resolve the Intent: `loaf intent resolve` pointing at this task and commit range — the orchestrator will run it if loaf commands are unavailable to you; note it in your final output either way.

## Verification

- `go test ./internal/cli/ -run 'ChangeState|ReleaseCohortGate|ChangeCheck' -count=1` green including the three dirty-tree fixtures.
