---
id: TASK-184
title: Preserve unknown task and spec frontmatter during metadata sync
status: done
priority: P1
created: '2026-05-19T19:11:07.751Z'
updated: '2026-05-19T19:19:55.198Z'
completed_at: '2026-05-19T19:19:55.197Z'
---

# TASK-184: Preserve unknown task and spec frontmatter during metadata sync

## Description

`loaf task update` syncs task/spec markdown frontmatter from `TASKS.json`.
That sync must not delete valid frontmatter fields that are intentionally not
indexed, such as spec `branch` and `adr` metadata.

## Acceptance Criteria

- [x] Sync replaces known indexed fields while preserving unknown fields.
- [x] Optional indexed fields still disappear when the index no longer contains them.
- [x] Regression coverage protects task and spec frontmatter preservation.

## Verification

```bash
npm test -- cli/lib/tasks/migrate.test.ts
```
