---
id: TASK-011
title: Add build-time command substitution
spec: SPEC-002
status: done
priority: P1
created: '2026-01-24T02:45:00.000Z'
updated: '2026-01-24T02:45:00.000Z'
files:
  - build/targets/claude-code.js
  - build/targets/cursor.js
  - build/targets/opencode.js
  - build/build.js
verify: >-
  npm run build && grep -r 'loaf:implement' plugins/ && grep -r '/implement'
  dist/cursor/
done: 'Claude Code output has /loaf:implement, Cursor/OpenCode has /implement'
session: 20260124-151610-orchestration-spec-002.md
completed_at: '2026-01-24T02:45:00.000Z'
---

# TASK-011: Add build-time command substitution

## Description

Add build-time placeholder substitution for target-specific command invocations. This ensures skill output suggests the correct command form for each target.

## Placeholders

| Placeholder | Claude Code | Cursor/OpenCode |
|-------------|-------------|-----------------|
| `{{IMPLEMENT_CMD}}` | `/loaf:implement` | `/implement` |
| `{{RESUME_CMD}}` | `/loaf:resume` | `/resume` |

## Acceptance Criteria

- [ ] Placeholders defined and documented
- [ ] Claude Code transformer replaces with scoped commands
- [ ] Cursor transformer replaces with unscoped commands
- [ ] OpenCode transformer replaces with unscoped commands
- [ ] `npm run build` succeeds
- [ ] Verify output in each target directory

## Context

See SPEC-002 for substitution details.

## Work Log

<!-- Updated by session as work progresses -->
