---
id: TASK-141
title: >-
  Cut over loaf release commit subject release: vX.Y.Z to chore: release vX.Y.Z
  (atomic with task 4)
spec: SPEC-031
status: done
priority: P1
created: '2026-04-29T17:27:52.097Z'
updated: '2026-04-29T19:12:43.587Z'
completed_at: '2026-04-29T19:12:43.587Z'
---

# TASK-141: Cut over loaf release commit subject release: vX.Y.Z to chore: release vX.Y.Z (atomic with task 4)

## Description

Cut over the `loaf release` commit subject from `release: vX.Y.Z` (not a Conventional Commits type) to `chore: release vX.Y.Z` so commits pass `@commitlint/config-conventional`. Touch-points are grep-verified: `cli/commands/release.ts:468`, `cli/commands/check.ts:642` (regex), `cli/commands/check.ts:649` (error message), `cli/commands/check.test.ts:632, 658, 890` (fixtures), `content/skills/release/SKILL.md:193, 226`, `docs/STRATEGY.md:64`. Lands in the same PR as TASK-140 (atomic shape cutover). Implements SPEC-031 Task 5.

## Acceptance Criteria

- [ ] `loaf release` produces a commit with subject `chore: release vX.Y.Z` (test asserts the exact subject shape, not just observed in fixtures).
- [ ] `release` is removed from the accepted-types regex in `cli/commands/check.ts:642` and from the user-facing "Valid types: ..." error message at `cli/commands/check.ts:649`.
- [ ] Existing `release: prep docs` test fixture is updated to assert rejection at the unknown-type layer (not at the changelog-empty layer).
- [ ] `content/skills/release/SKILL.md` lines 193 and 226 show `chore: release vX.Y.Z` instead of `release: vX.Y.Z`.
- [ ] `docs/STRATEGY.md:64` doctrine is reworded to reflect the chore-shape cutover (no longer references `release:` escape hatch).
- [ ] A commitlint-pinned fixture project accepts the `loaf release` commit on its first commit-msg hook.

## Verification

```bash
npm run typecheck && npm run test -- check.test.ts release
```
