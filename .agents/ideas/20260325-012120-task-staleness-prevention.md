---
title: "Task/spec staleness prevention + obsolete status"
captured: 2026-03-25T01:21:20Z
status: raw
tags: [cli, task-management, data-model, staleness]
related: [SPEC-010]
---

# Task/Spec Staleness Prevention + Obsolete Status

## Nugget

TASKS.json, task .md files, and spec statuses drift out of sync because there are
two sources of truth and no automatic reconciliation. The task system also lacks an
`obsolete` status for superseded work. Fix the data model so staleness can't happen.

## Problem/Opportunity

During SPEC-009 post-merge cleanup, we found:
- TASKS.json had `next_id: 33` when we'd created tasks up to 039
- Archived tasks still showed `todo` status in the JSON
- Task statuses edited in .md files weren't reflected in TASKS.json
- No `obsolete` status — had to use `done` for superseded tasks, losing the distinction
- Spec statuses manually edited without validation

Root cause: TASKS.json is a denormalized index that only updates via `loaf task`
commands, but .md files are also edited directly (by implement skill, manual edits,
and session workflows).

## Initial Context

**Original intent for TASKS.json:** GUI/TUI backend for humans to interact with tasks
without parsing .md files. But with ~40-50 task files, scanning frontmatter is
milliseconds — the performance problem TASKS.json solves doesn't exist yet.

**Options to consider:**
- **Eliminate TASKS.json** — single source of truth in .md frontmatter, derive on read
- **Make it a cache** — rebuild from .md files when any mtime is newer than JSON mtime.
  Fast reads for GUI, no sync problem, but adds mtime checking logic
- **Auto-sync hook** — SessionEnd or post-commit rebuilds the index. Still two sources,
  but drift window is smaller

**`obsolete` status:** Add to the task system's recognized statuses. Display separately
from `done` in `loaf task list`. Means "superseded/no longer relevant" vs "completed
as specified."

---

*Captured via /idea — shape with /shape when ready*
