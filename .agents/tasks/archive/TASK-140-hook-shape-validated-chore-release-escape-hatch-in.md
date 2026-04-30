---
id: TASK-140
title: >-
  Hook shape-validated chore: release escape hatch in workflow-pre-pr +
  validate-push
spec: SPEC-031
status: done
priority: P1
created: '2026-04-29T17:27:52.043Z'
updated: '2026-04-29T19:12:43.506Z'
completed_at: '2026-04-29T19:12:43.506Z'
---

# TASK-140: Hook shape-validated chore: release escape hatch in workflow-pre-pr + validate-push

## Description

Update `workflow-pre-pr` and `validate-push` in `cli/commands/check.ts` to accept HEAD commits matching the shape `^chore: release v<semver>( \(#\d+\))?$` as a legitimate pre-merge release escape hatch, alongside existing tagged-HEAD logic. The check is shape-validated, not prefix-only — `chore: release notes draft` must still be rejected. The prior `release:` shape is dropped entirely. Lands in the same PR as TASK-141 (atomic shape cutover — splitting them either rejects the new commit or accepts the old one). Implements SPEC-031 Task 4.

## Acceptance Criteria

- [ ] HEAD commit `chore: release v1.2.3` passes `workflow-pre-pr` and `validate-push`.
- [ ] HEAD commit `chore: release v1.2.3 (#42)` passes both hooks.
- [ ] HEAD commit `chore: release notes draft` is rejected (shape validation, not prefix matching).
- [ ] HEAD commit `release: vX.Y.Z` is rejected (now an unknown Conventional Commits type per TASK-141).
- [ ] Test fixtures in `cli/commands/check.test.ts` cover the accept and reject cases above.

## Verification

```bash
npm run typecheck && npm run test -- check.test.ts
```
