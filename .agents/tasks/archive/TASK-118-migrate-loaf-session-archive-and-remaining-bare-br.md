---
id: TASK-118
title: Migrate loaf session archive and remaining bare-branch call sites
spec: SPEC-032
status: done
priority: P1
created: '2026-04-27T22:45:03.139Z'
updated: '2026-04-28T09:27:17.647Z'
depends_on:
  - TASK-116
completed_at: '2026-04-28T09:27:17.646Z'
---

# TASK-118: Migrate loaf session archive and remaining bare-branch call sites

## Description

Migrate the remaining bare-branch call sites in `cli/commands/session.ts` to use `resolveCurrentSession` (from TASK-116). Surfaced by grep:

- **Line 883** — confirm during implementation; may already be reached via a hook-aware path. If not, migrate.
- **Line 2015** — `loaf session archive` action body. Add `--session-id <id>` CLI flag for parity with `loaf session log`.
- **Line 2354** — secondary path (likely housekeeping or report). Confirm during implementation; migrate if it's user-facing routing.

The hook-aware paths (lines 1394+1398, 1634+1635, 2671+2672, 2742+2743, 2831+2832) already use the correct chain pattern. **Optionally** migrate them to call `resolveCurrentSession` for code consistency — but only after the helper proves out in TASK-117 with no behavior changes. If migration risks behavior drift, leave the hook-aware paths alone and document the asymmetry in a code comment.

**File hints:**
- Modify: `cli/commands/session.ts` lines 883, 2015, 2354 (and command flag registration for `archive`)
- Tests: `cli/commands/session.test.ts` — coverage for `loaf session archive --session-id` and stderr WARN on bare archive

## Acceptance Criteria

- [ ] `loaf session archive --session-id X` archives the session with `claude_session_id: X`, regardless of branch
- [ ] `loaf session archive` (no flag) preserves current behavior AND emits stderr WARN on Tier 3 fallback
- [ ] Line 883 call site is either migrated or confirmed to be reached through a hook-aware path that already uses the chain (document in a code comment)
- [ ] Line 2354 call site is either migrated or confirmed to not be on the user-facing routing path (document in a code comment)
- [ ] No new bare `findActiveSessionForBranch` calls outside `findSessionByClaudeId` and `resolveCurrentSession` exist after this task
- [ ] Tests cover `loaf session archive` under all three signal conditions
- [ ] WARN text matches the literal from the spec exactly when Tier 3 fires from `archive`

## Verification

```bash
npm run typecheck
npm run test -- cli/commands/session.test.ts
# After implementation, this grep should return only the chain helper and the resolve helper:
grep -rn "findActiveSessionForBranch" cli/
```
