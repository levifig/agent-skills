---
id: TASK-036
title: 'KB lifecycle hooks (SessionStart, PostToolUse, SessionEnd)'
spec: SPEC-009
status: done
priority: P2
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
depends_on:
  - TASK-035
files:
  - content/hooks/session/kb-session-start.sh
  - content/hooks/post-tool/kb-staleness-nudge.sh
  - content/hooks/session/kb-session-end.sh
  - config/hooks.yaml
verify: loaf build && npm run typecheck
done: >-
  SessionStart hook surfaces stale knowledge count. PostToolUse hook nudges once
  per session when editing covered-file paths. SessionEnd hook prompts for
  knowledge consolidation if covered files were edited. All hooks registered in
  hooks.yaml.
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-036: KB lifecycle hooks

## Description

Wire up the three lifecycle hooks that make staleness awareness automatic during
agent sessions. These are bash scripts that call `loaf kb` commands and format output.

## Acceptance Criteria

- [ ] **SessionStart hook** calls `loaf kb status --json`, formats output:
  "N knowledge files. M stale." (or nothing if no stale files)
- [ ] **PostToolUse hook** on Edit/Write:
  - Parses `$TOOL_INPUT` for file path
  - Calls `loaf kb check --file <path> --json`
  - If any matching knowledge files are stale, outputs nudge
  - Per-session single nudge: uses temp file (`/tmp/loaf-kb-nudged-$$`) to track
    already-nudged knowledge files; skips if already nudged this session
  - Temp file cleaned up by SessionEnd hook
- [ ] **SessionEnd hook** checks if covered files were modified, prompts for
  knowledge consolidation
- [ ] All hooks registered in `config/hooks.yaml` with correct matchers, skills,
  timeouts
- [ ] PostToolUse hook matcher: `Edit|Write` (same pattern as existing hooks)
- [ ] `loaf build` succeeds with new hooks included

## Context

See SPEC-009 for full context. Follow hook patterns from
`content/hooks/post-tool/orchestration-generate-task-board.sh` (parses $TOOL_INPUT)
and `config/hooks.yaml` DSL.

**Circuit breaker:** If hitting 75% appetite, simplify to SessionStart-only.
