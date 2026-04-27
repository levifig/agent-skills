---
id: TASK-119
title: 'E2E smoke test: dev.30 misrouting repro + stderr WARN assertion'
spec: SPEC-032
status: todo
priority: P2
created: '2026-04-27T22:45:08.084Z'
updated: '2026-04-27T22:45:08.084Z'
depends_on:
  - TASK-117
  - TASK-118
---

# TASK-119: E2E smoke test: dev.30 misrouting repro + stderr WARN assertion

## Description

End-to-end verification that SPEC-032 actually fixes the v2.0.0-dev.30 misrouting bug recorded in commit `81b1808a chore: record session journals with dev.30 post-merge wrap (#misrouted)`.

Build a fixture matching the live state observed during shaping: 1 active session + 4 stopped sessions on `main`, each with distinct `claude_session_id`. Invoke `loaf session log --from-hook` with hook JSON containing the active session_id. Assert the new entry lands in the correct (active) session file and not in any of the four stopped ones.

Also assert the stderr WARN surface fires correctly on Tier 3 fallback — both:
- WARN fires when `loaf session log "..."` is invoked with no flag and no `--from-hook`
- WARN does NOT fire when `--session-id` or `--from-hook` provides a valid session_id

This is the regression gate — if a future change reintroduces branch-only routing, this test catches it.

**File hints:**
- New: `cli/commands/session.misrouting.e2e.test.ts` (or extend an existing E2E test file)
- Fixture pattern: see `cli/lib/journal/extractor.test.ts` for how Loaf builds session-file fixtures

## Acceptance Criteria

- [ ] Fixture creates 5 session files on the same branch (1 active, 4 stopped) with distinct `claude_session_id` values
- [ ] `loaf session log --from-hook` with stdin `{"session_id":"<active-id>", ...}` adds entry to the active session file ONLY
- [ ] None of the 4 stopped session files are modified
- [ ] No stderr WARN is emitted on the hook-driven path
- [ ] `loaf session log "regression(test): no signal"` (no flag, no hook) writes to the active session AND emits the stderr WARN
- [ ] WARN text matches the literal from the spec exactly
- [ ] Test runs in CI (added to `npm run test` default scope)
- [ ] Test name explicitly references the dev.30 incident so future maintainers can trace the regression target

## Verification

```bash
npm run test -- cli/commands/session.misrouting.e2e.test.ts
```
