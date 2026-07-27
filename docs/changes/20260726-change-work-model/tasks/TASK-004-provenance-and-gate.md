---
change: change-work-model
id: TASK-004
title: Execution provenance and the gate rewrite
blocked-by:
  - TASK-003
relates-to:
  - TASK-005
---

# TASK-004 — Execution provenance and the gate rewrite

## Objective

Restructure release preflight to compute the candidate version first and gate stable candidates on their cohort (byte-equal `target_release`). Finalization paths (`--bump release`, `--post-merge`) gate the stable cohort. Derive executed-ness (path grade display; flip grade for gating). Retire lineage freeze replay while materializing the live 2.0.0 promise carrier. Redefine `loaf change list` as the units/cohort projection. Ships provenance-only — receipt enforcement arrives with TASK-005.

## Scope boundaries

**In:** Candidate-first preflight; cohort gate messages; path/flip provenance; carrier materialization + freeze retirement; `loaf change list` redefinition; lower-cohort warnings; retarget surfacing in preflight.

**Out:** Receipt write/freshness (`TASK-005`). Watch: this task is heavyweight — split if writing the packet reveals more than one vertical slice (sanctioned).

## Context pointers

- Decisions 9/10/15/17; Planning Contract Provenance precision; V1/V2; `release.go`, `release_dry_run.go`, `change_lineage.go`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-004 — provenance and gate rewrite"
```

## Steps

- [x] Candidate version computed before gating; stable candidates gate cohort; prerelease candidates bypass; finalization gates stable target.
- [x] Path-grade executed display; flip-grade gate for `target_release` members; negative fixtures for non-flips.
- [x] Block messages distinguish not-materialized / not-executed / legacy-convert-first.
- [x] Materialize carrier with `target_release: 2.0.0` atomically with freeze-replay retirement.
- [x] `loaf change list` becomes units/cohort projection (`--target` filters).
- [x] Consider splitting if scope exceeds one coherent commit/slice.

## Verification

- V1/V2 regression fixtures; existing freeze tests replaced or retired with carrier held.
