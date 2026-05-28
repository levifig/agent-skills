---
id: TASK-034
title: 'KB validate, status, and review commands'
spec: SPEC-009
status: done
priority: P1
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
depends_on:
  - TASK-033
files:
  - cli/lib/kb/validate.ts
  - cli/commands/kb.ts
verify: npm run typecheck && npm run test
done: >-
  loaf kb validate reports missing topics/last_reviewed, invalid dates, and
  unmatched covers: globs. loaf kb status shows total file count and average
  review age. loaf kb review updates last_reviewed. All support --json.
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-034: KB validate, status, and review commands

## Description

Implement three kb subcommands that use the core library: validate (schema checking),
status (summary view), and review (mark as reviewed). These are the Week 1 deliverables
that make the knowledge system usable before staleness detection ships.

## Acceptance Criteria

- [ ] `cli/lib/kb/validate.ts` checks: required `topics` (min 1), required
  `last_reviewed` (valid ISO 8601 date), `covers:` globs that match no files
  (warning, not error), broken `depends_on` references
- [ ] `loaf kb validate` displays results with ANSI colors (green pass, yellow warn,
  red error) and exits non-zero on errors
- [ ] `loaf kb status` shows: total knowledge file count, files with `covers:` vs
  without, average review age in days (stale count added in TASK-035)
- [ ] `loaf kb review <file>` updates `last_reviewed` to current date, preserving
  all other frontmatter
- [ ] All three commands support `--json` flag
- [ ] Unit tests for validation logic (all error cases)
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

## Context

See SPEC-009 for full context. Follow output patterns from `cli/commands/task.ts`
(ANSI colors, `--json` support). Use `git ls-files -- <glob>` to check if covers:
globs match any tracked files in validate.
