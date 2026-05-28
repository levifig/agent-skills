---
id: TASK-116
title: Add parseHookSessionId and resolveCurrentSession helpers
spec: SPEC-032
status: done
priority: P1
created: '2026-04-27T22:44:55.496Z'
updated: '2026-04-28T09:27:17.493Z'
session: 20260427-224828-session.md
completed_at: '2026-04-28T09:27:17.492Z'
---

# TASK-116: Add parseHookSessionId and resolveCurrentSession helpers

## Description

Extract the helpers that the rest of the spec depends on. Two new functions in `cli/lib/session/`:

1. **`parseHookSessionId(): string | undefined`** — reads stdin JSON and returns the `session_id` field if present. Returns `undefined` on missing field, malformed JSON, or empty stdin. Does NOT auto-detect non-TTY stdin — caller must opt in by passing `--from-hook` (the helper is only invoked when the caller signals hook mode).

2. **`resolveCurrentSession(agentsDir, branch, opts)`** — the 3-tier routing chain:
   - Tier 1: `opts.sessionIdFlag` → `findSessionByClaudeId`
   - Tier 2: `opts.parseStdin === true` → `parseHookSessionId()` → `findSessionByClaudeId`
   - Tier 3: `findActiveSessionForBranch(agentsDir, branch)` — emits stderr WARN before returning

   Returns the resolved session (`{ filePath, data, content }`) or `null`. The stderr WARN message is the literal `WARN: no session_id signal — falling back to branch routing for branch '<branch>'. Pass --session-id <id> to silence.`

The two existing helpers `findSessionByClaudeId` and `findActiveSessionForBranch` in `cli/commands/session.ts` are not modified — they are correct as-is.

**File hints:**
- New: `cli/lib/session/index.ts` (or `cli/lib/session/resolve.ts` if the lib doesn't exist yet)
- New tests: `cli/lib/session/resolve.test.ts`
- Reference: `cli/commands/session.ts:659` (findSessionByClaudeId), `cli/commands/session.ts:738` (findActiveSessionForBranch)

## Acceptance Criteria

- [ ] `parseHookSessionId()` returns `session_id` for valid hook JSON on stdin
- [ ] `parseHookSessionId()` returns `undefined` for malformed JSON, missing field, empty stdin
- [ ] `resolveCurrentSession` Tier 1: `--session-id X` resolves to session with `claude_session_id: X` regardless of branch; no stderr output
- [ ] `resolveCurrentSession` Tier 2: `parseStdin: true` with `{"session_id":"X"}` on stdin resolves to session with `claude_session_id: X`; no stderr output
- [ ] `resolveCurrentSession` Tier 3: no flag, no stdin signal → `findActiveSessionForBranch` AND emits exact WARN message to stderr
- [ ] When Tier 1 or Tier 2 resolves to `null` (session_id given but no matching file), chain falls through to Tier 3 (with WARN)
- [ ] WARN message includes the literal `WARN: no session_id signal` and names the branch
- [ ] WARN goes to stderr (not stdout) — verified via process.stderr write
- [ ] Unit tests cover all four resolution paths plus the fall-through case

## Verification

```bash
npm run typecheck
npm run test -- cli/lib/session/resolve.test.ts
```
