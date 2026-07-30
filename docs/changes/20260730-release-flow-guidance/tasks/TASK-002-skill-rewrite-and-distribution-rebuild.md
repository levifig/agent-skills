---
change: release-flow-guidance
id: TASK-002
title: Skill rewrite and distribution rebuild
blocked-by:
  - TASK-001
---

# TASK-002 — Skill rewrite and distribution rebuild

## Objective

The release skill states the release-PR flow as the default for every release: prepare on a release branch (`loaf release --pre-merge`), squash-merge the release PR into one `chore: release vX (#PR)` commit carrying the curated changelog, then finalize with `loaf release --post-merge` on the default branch. Step 5 loses its branch-protection conditional entirely; direct `--bump` on the default branch is demoted to a named exception used only on explicit user request, with the alpha.16 incident as the recorded reason. Distribution targets are rebuilt and committed.

## Scope boundaries

**In:** `content/skills/release/SKILL.md` Steps 4–5 (and any cross-references to them in the same file), `loaf build`, committed `dist/` and `plugins/` outputs.

**Out:** CLI code (TASK-001); other skills; changelog format guidance.

## Context pointers

- Contract: `shape.md` — Scope → In; Decision 2; Definition of Done (no branch-protection conditional survives).
- Current text: `content/skills/release/SKILL.md` — Step 5 "Protected-Branch Handoff".
- Prose rule: paragraphs are single lines, never hard-wrapped.

## Steps

- [x] Rewrite Step 5 as the default release-PR flow (rename away from "Protected-Branch Handoff"); fold branch protection in as one sentence of rationale, not a condition.
- [x] Reframe Step 4 to route through the PR flow; document the direct mode as the explicit exception.
- [x] Update the Quick Reference table and Critical Rules to match.
- [x] `loaf build`; commit rebuilt `dist/` and `plugins/` with the source change.

## Verification

- `bin/loaf build` exit 0.
- `grep -i "branch protection" content/skills/release/SKILL.md` shows no conditional gating of the PR flow.
