---
change: release-promotion-model
id: TASK-008
title: Release notes and the changelog projection
blocked-by:
  - TASK-002
blocks:
  - TASK-004
  - TASK-005
  - TASK-007
---

# TASK-008 — Release notes and the changelog projection

## Objective

CHANGELOG.md becomes a cut-time projection fed by per-change release notes, and the write path is rung-aware. Each change may carry `release-notes.md` in its own folder (user-facing fragment, never deleted, archived with the change); note collection derives from the release range plus execution provenance, so a note participates only when its change's delivering commits landed in the range. `writeReleaseChangelog` and the dry-run preview key on the rung: rc cuts write nothing, alpha/beta cuts splice a section aggregated from the ranged notes (note-less landed changes fall back to `shape.md` Problem/Hypothesis lines with a warning), promotion's rollup is validated by TASK-004. The `[Unreleased]` buffer retires: the heading and stub disappear from the projection, and `workflow-pre-pr` re-points from buffer emptiness to per-change note presence. The no-impact escape's carrier is the note file itself: a change with nothing user-facing writes `release-notes.md` containing the explicit no-user-facing-impact declaration — one surface, auditable in the folder, skipped by aggregation. Touched change folders are detected per-PR via `git diff <base>..HEAD --name-only` (the `isReleaseOnlyPR` pattern in `check.go`).

## Scope boundaries

**In:** `internal/cli/release_dry_run.go` changelog read/write plumbing (`releaseChangelogSection`, `extractReleaseUnreleasedBody`, `writeReleaseChangelog`, `insertReleaseChangelog`, `createReleaseChangelog`, the preview at `:288-294` and apply at `:481-484`), the `loaf init` CHANGELOG template (`internal/cli/init.go:274-283`), the live CHANGELOG.md buffer removal, the note-collection helper (range + provenance), `runNativeWorkflowPrePR` in `internal/cli/check.go:834-869`, tests.

**Out:** Guardrail verification of the produced sections (TASK-005); the promotion rollup validation and seed emission surface (TASK-004 — this task provides the collection helper it calls); flag parsing (TASK-002); skill prose teaching the authoring discipline (TASK-007).

## Context pointers

- Contract: `shape.md` — Decision 3; Planning Contract → Per-channel guardrails (the write-path sentence); Observable Workflow.
- Provenance grade for "landed in the range": `change_provenance.go` — reuse the existing execution-provenance derivation, never a new heuristic.
- Current buffer machinery: `release_dry_run.go:904-970` (curated content wins verbatim today — that precedence dies with the buffer).

## Acquisition

```bash
loaf journal log "skill(implement): TASK-008 — release notes and changelog projection"
# Read internal/cli/release_dry_run.go:280-300,470-490,900-1170 and check.go:830-880 before editing.
```

## Steps

- [ ] Note collection: changes with delivering commits in the release range contribute their `release-notes.md`; note-less landed changes contribute the `shape.md` fallback with a warning naming the folder.
- [ ] The no-impact declaration's recognized form is exact and defined HERE (TASK-007 teaches it, never redefines it): a note whose entire content is the single line `No user-facing impact.` — the collection helper recognizes it mechanically and aggregation skips it.
- [ ] Rung-aware write path: rc writes no CHANGELOG section (preview says so explicitly); alpha/beta splice the aggregated section; stable path delegates to the promotion rollup.
- [ ] Retire the `[Unreleased]` buffer physically in this task's own delivering commit: remove the heading, stub, and preamble sentence from the live CHANGELOG.md; rewrite `insertReleaseChangelog`'s anchor semantics (new sections land directly above the previous top release; a missing anchor is the new normal, never the current silent bottom-append fallback at `release_dry_run.go:1136-1138`); update both scaffolds that still emit the buffer — `createReleaseChangelog` (`release_dry_run.go:1167-1181`) and the `loaf init` CHANGELOG template (`internal/cli/init.go:274-283`).
- [ ] Re-point `workflow-pre-pr`: detect touched change folders via `git diff <base>..HEAD --name-only` (reuse the `isReleaseOnlyPR` pattern); block when a touched change folder lacks `release-notes.md`; the no-impact declaration inside the note satisfies the check and aggregation skips it; keep the existing release-flow escape untouched.
- [ ] Tests under `TestReleaseNotesProjection`: range-and-provenance collection (landed vs merely-authored), rc writes nothing, alpha/beta aggregation, fallback warning, hook accept/block/escape cases.

## Verification

- `go test ./internal/cli -run 'TestReleaseNotesProjection' -count=1` green, including the placement assertions (sections land above the previous top release; no `[Unreleased]` anchor anywhere in the output).
- TASK-005's later `TestReleasePostMergeChannel` suite builds its fixtures on the sections this task produces — congruence is proven there, after both land (the suite does not exist yet at this task's completion).
