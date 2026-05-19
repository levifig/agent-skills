---
id: TASK-166
title: 'Foundation: worktree probe + dual resolvers + ID allocator scans both views'
spec: SPEC-036
status: done
priority: P1
created: '2026-05-18T23:58:39.458Z'
updated: '2026-05-19T08:56:34.313Z'
session: .agents/sessions/20260519-004429-session.md
completed_at: '2026-05-19T08:56:34.312Z'
---

# TASK-166: Foundation: worktree probe + dual resolvers + ID allocator scans both views

## Description

Make `findAgentsDir()` worktree-aware: in a linked git worktree, return the **main worktree's** `.agents/` directory (via `dirname(git rev-parse --git-common-dir)/.agents/`); in single-checkout or non-git contexts, preserve the current parent-walk behavior verbatim.

This is the **only** code change required by SPEC-036's storage model under A3. Every existing call site in the codebase picks up the new behavior automatically — sessions converge to the main worktree, ID allocators scan the single shared view, kb/ideas/drafts/reports/councils all read from the same place. No per-module refactor, no dual resolvers, no per-artifact routing logic.

## Acceptance Criteria

- [ ] `findAgentsDir()` returns the main worktree's `.agents/` when invoked from a linked worktree
- [ ] `findAgentsDir()` preserves current parent-walk behavior in single-checkout repos
- [ ] `findAgentsDir()` preserves current parent-walk behavior outside a git context
- [ ] Unit test: simulated worktree — both worktree and main checkout resolve to the same path
- [ ] Cross-worktree integration test: start a session in a simulated main worktree, journal an entry from a simulated linked worktree, assert both reach the same file
- [ ] Parallel ID allocation test: two simulated worktrees minting tasks concurrently allocate distinct IDs (validates that single-view scanning is sufficient under A3)
- [ ] Existing test suite remains green — no other call sites required changes

## Files

- `cli/lib/tasks/resolve.ts` (extend `findAgentsDir` with the worktree probe)
- New tests alongside the function

## Verification

```bash
npm run test
npm run typecheck
npm run test -- agents-dir
```

## Context

See SPEC-036 for full context. This is Track A of the spec — the foundation gate for Tracks B and C.
