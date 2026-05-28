---
id: TASK-186
title: Strengthen git workflow changelog discipline guidance
status: done
priority: P2
created: '2026-05-19T19:11:08.566Z'
updated: '2026-05-19T19:19:55.937Z'
completed_at: '2026-05-19T19:19:55.937Z'
---

# TASK-186: Strengthen git workflow changelog discipline guidance

## Description

Make the git workflow guidance explicit that generated changelog entries are
drafts and release bumps need a human pass to remove internal scaffolding
language.

## Acceptance Criteria

- [x] Source git-workflow guidance states the review step clearly.
- [x] Generated target copies are rebuilt.

## Verification

```bash
npm run build
```
