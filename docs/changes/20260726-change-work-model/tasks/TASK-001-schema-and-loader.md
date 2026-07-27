---
change: change-work-model
id: TASK-001
title: change.json schema and layout-agnostic loader
blocks:
  - TASK-002
  - TASK-003
---

# TASK-001 — change.json schema and layout-agnostic loader

## Objective

Ship the machine surface for the new Change anatomy: parse `change.json` (closed schema), detect layout by `change.json` presence with legacy `change.md` fallback in the same root, and migrate every `change.md`-resolving consumer onto a layout-agnostic in-memory node. History replay unions both surfaces for retarget events; retention keys on target-declaring units (including legacy `lineage`/`release-after` history) and extends to `change.json`. Sanctioned atomic conversion is recognized as replacement, not deletion.

## Scope boundaries

**In:** `change.json` parse + validation; layout detection; `changeNode` carrying parsed metadata + per-layout contract content; loaders at working tree and committed HEAD; duplicate-slug / folder / branch resolution; cohort grouping by `target_release`; unit-keyed retention + retarget event derivation; conversion recognition.

**Out:** New scaffold/templates (`TASK-002`); full `loaf change check` projections and deprecation notice (`TASK-003`); release preflight candidate-first rewrite and provenance grades (`TASK-004`); receipts (`TASK-005`); skill/guidance sweep (`TASK-006`). Do not flip this change folder to the new layout yet — self-conversion waits until `TASK-003` lands (Definition of Done).

## Context pointers

- Contract: `docs/changes/20260726-change-work-model/change.md` — Scope (anatomy + schema), Decisions 1/9/13, Planning Contract (Approach, Metadata history, Compatibility), Implementation Units TASK-001, Verification V4/V7/V8.
- Code today: `internal/cli/change.go` (loader, resolve, check, init), `internal/cli/change_lineage.go` (HEAD loader, graph, freeze replay, retention).
- Handoff: `HANDOFF-20260727-implement-change-work-model-task-001-first`.

## Acquisition

```bash
cd "$(git rev-parse --show-toplevel)"
loaf journal log "skill(implement): TASK-001 — change.json schema and layout-agnostic loader"
rg -n "loadChangeNodes|resolveChangeFolder|deletedLineage|dependencyMetadata" internal/cli/change*.go
```

## Steps

- [ ] Define closed `change.json` schema: required identity `change` / `created` / `branch`; optional `target_release` as canonical `MAJOR.MINOR.PATCH` (no `v`, leading zeros, prerelease, or build); reject unknown keys and status/lifecycle aliases (`readiness`/`status`/`state`/`completion`/`done` and kin).
- [ ] Detect layout by `change.json` presence; legacy fallback only when absent; malformed `change.json` fails closed (violation / load finding), never silent fallback to `change.md`.
- [ ] Replace single-content `changeNode` assembly with parsed metadata + per-layout contract evaluation surface (`shape.md` new, `change.md` legacy); keep legacy lineage fields readable for today's gate.
- [ ] Migrate consumers: working-tree and HEAD loaders, `findChangeSlug`, folder and branch resolution, init duplicate detection, list/check indexing.
- [ ] History replay unions `change.json` and `change.md` surfaces to derive retarget events (`target_release` mutations including removal-to-none); retention guards target-declaring units (and legacy lineage/`release-after` history) unit-keyed, including `change.json`; atomic same-commit convert is replacement not deletion.
- [ ] Tests cover schema grammar, closed keys, layout detection, fail-closed malformed JSON, dual-surface load, retarget derivation, retention+conversion fixtures; `go test` for touched packages passes.

## Verification

- `go test ./internal/cli/ -run 'Change(JSON|Layout|Node|Retention|Retarget|Convert)|LoadChange' -count=1`
- Existing lineage/release preflight tests still pass (today's gate behavior preserved).
- `loaf change check docs/changes/20260726-change-work-model` remains green on the legacy folder.
- New-layout fixture with valid `change.json` loads; malformed `change.json` beside valid `change.md` does not fall back.
