---
change: change-work-model
id: TASK-003
title: Check over both layouts and projections
blocked-by:
  - TASK-001
  - TASK-002
blocks:
  - TASK-004
  - TASK-005
  - TASK-006
---

# TASK-003 — loaf change check over both layouts, and the projections

## Objective

Validate both layouts with a deprecation notice naming the removal boundary; emit `loaf change tasks --json` and `loaf change show` projections; report brief-only folders as captured; enforce task hygiene; extend artifact-names to scan the slug remainder after `TASK-NNN-`.

## Scope boundaries

**In:** Dual-layout check; deprecation notice; tasks/show projections; brief-only derived state; task relation hygiene; artifact-names slug-remainder scan; dogfood conversion of this change folder via sanctioned atomic replace once check exists.

**Out:** Gate rewrite (`TASK-004`); receipts (`TASK-005`); skill sweep (`TASK-006`).

## Context pointers

- `change.md` TASK-003, V4/V6/V8, H2; `internal/cli/check_artifact_names.go`.

## Acquisition

```bash
loaf journal log "skill(implement): TASK-003 — check and projections"
```

## Steps

- [x] `loaf change check` validates both layouts; legacy emits deprecation notice with named removal boundary.
- [x] Malformed `change.json` beside valid `change.md` reports a violation (no fallback).
- [x] `loaf change tasks --json` stable parent/children index; `loaf change show` derives PR set from squash subjects.
- [x] Brief-only folders legal, non-executable, reported as captured.
- [x] Task hygiene: duplicate numbers, dangling/self/cross-change/unknown-key relations, cycles → violations; zero-checkbox → warning.
- [x] Artifact-names strips `TASK-NNN-` and scans slug remainder.
- [x] Convert this unit via sanctioned atomic replace (values verbatim, all checkboxes unchecked).

## Verification

- V4/V6/V8 fixtures; this folder converts and still checks clean.
