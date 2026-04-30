---
id: TASK-138
title: 'loaf release re-inserts `_No unreleased changes yet._` stub under [Unreleased]'
spec: SPEC-031
status: done
priority: P2
created: '2026-04-29T17:27:51.932Z'
updated: '2026-04-29T19:02:03.454Z'
completed_at: '2026-04-29T19:02:03.454Z'
---

# TASK-138: loaf release re-inserts `_No unreleased changes yet._` stub under [Unreleased]

## Description

After `loaf release` moves entries from `[Unreleased]` into a new `## [X.Y.Z]` section, re-insert the `_No unreleased changes yet._` stub line under `[Unreleased]` so `workflow-pre-pr` does not block the subsequent `gh pr create`. The fix lives in `cli/commands/release.ts` and must apply to both the curated-entries path and the auto-generated path. Implements SPEC-031 Task 2.

## Acceptance Criteria

- [ ] After `loaf release`, the `[Unreleased]` section contains a literal `_No unreleased changes yet._` line on the curated path.
- [ ] After `loaf release`, the `[Unreleased]` section contains a literal `_No unreleased changes yet._` line on the auto-generated path.
- [ ] Test asserts the exact stub string under `[Unreleased]`, not just the presence of the `[Unreleased]` header.
- [ ] No ceremony commit is needed to restore the stub — it is present in the same commit `loaf release` produces.
- [ ] `workflow-pre-pr` succeeds against the post-release CHANGELOG without any manual edit.

## Verification

```bash
npm run typecheck && npm run test -- release
```
