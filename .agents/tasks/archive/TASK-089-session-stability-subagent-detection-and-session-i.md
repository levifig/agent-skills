---
id: TASK-089
title: Session stability — subagent detection and session ID tagging
spec: SPEC-027
status: done
priority: P1
created: '2026-04-07T10:10:52.668Z'
updated: '2026-04-07T10:30:26.324Z'
completed_at: '2026-04-07T10:30:26.324Z'
---

# TASK-089: Session stability — subagent detection and session ID tagging

## Description

Implement SPEC-027 Part B: session stability via subagent detection and session ID tagging.

**Subagent detection:** When `loaf session start` is invoked as a hook, parse JSON from stdin. If `agent_id` is present → exit 0 immediately (no session creation, no archiving, no journal entry). Add `--force` flag to bypass detection.

**Session ID tagging:** Extract `session_id` from hook JSON, write to frontmatter as `claude_session_id`. On subsequent SessionStart calls, compare incoming session_id with stored one: same ID → resume (no PAUSE); different ID → write PAUSE header, update claude_session_id.

## Key Files

- `cli/commands/session.ts` — session start action (line ~890), `SessionFrontmatter` interface (line 46), `getOrCreateSession` (line 537)
- `plugins/loaf/bin/loaf` — build artifact (rebuild after changes)

## Acceptance Criteria

- [ ] `loaf session start` parses hook JSON from stdin when available
- [ ] `agent_id` in hook JSON → exit 0, no side effects
- [ ] `claude_session_id` written to session frontmatter on SessionStart
- [ ] Same `session_id` across calls → resume without PAUSE header
- [ ] Different `session_id` → PAUSE header + updated `claude_session_id`
- [ ] `loaf session start --force` creates new session regardless of agent_id
- [ ] Spawning 3 subagents from a session results in exactly 1 session file
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes
- [ ] `loaf build` succeeds

## Verification

```bash
npm run typecheck && npm run test && loaf build
```

## Context

See SPEC-027 Part B. Claude Code hook JSON exposes `agent_id` only for subagents — this is the discriminator. `session_id` is always present.
