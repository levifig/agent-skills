---
id: TASK-038
title: KB import command (stretch goal)
spec: SPEC-009
status: done
priority: P3
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
depends_on:
  - TASK-037
files:
  - cli/commands/kb.ts
  - .agents/loaf.json
verify: npm run typecheck && npm run test
done: >-
  loaf kb import <name> registers an external QMD collection and updates
  loaf.json imports array. Errors helpfully if QMD not installed.
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-038: KB import command (stretch goal)

## Description

Implement the `kb import` command for registering external project knowledge via QMD
collections. This is the stretch goal — first to be cut if appetite runs out.

## Acceptance Criteria

- [ ] `loaf kb import <name>` registers a QMD collection pointing to the named
  project's knowledge directory
- [ ] Updates `.agents/loaf.json` `imports` array with the new entry
- [ ] Errors helpfully if QMD is not installed (suggests `loaf kb init` first)
- [ ] Prevents duplicate imports (checks existing imports before adding)
- [ ] `--json` support
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

## Context

See SPEC-009 for full context. Keep this simple — accept a name argument, register the
collection. No TUI picker, no fuzzy search, no `local-covers` mapping (all explicitly
out of scope per spec rabbit holes).

**Circuit breaker:** This task is cut entirely if at 75% appetite.
