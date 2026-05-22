---
id: TASK-178
title: Fix release post-merge guardrail to accept real squash subjects
status: done
priority: P1
created: '2026-05-19T18:55:58.924Z'
updated: '2026-05-19T19:02:44.430Z'
completed_at: '2026-05-19T19:02:44.429Z'
---

# TASK-178: Fix release post-merge guardrail to accept real squash subjects

## Description

`loaf release --post-merge` should accept normal squash subjects such as
`feat: ... (#N)` and derive release safety from version files plus
`CHANGELOG.md`, not from a required `chore: release vX.Y.Z` subject.

## Acceptance Criteria

- [ ] Post-merge guardrails accept feature/fix squash subjects.
- [ ] Release version is derived from version files at HEAD.
- [ ] PR-number suffix lookup still works for branch cleanup.

## Verification

```bash
npm test -- cli/lib/release/post-merge.test.ts cli/commands/release.test.ts
```
