---
id: TASK-035
title: KB staleness detection + check command
spec: SPEC-009
status: done
priority: P1
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
depends_on:
  - TASK-033
files:
  - cli/lib/kb/staleness.ts
  - cli/commands/kb.ts
verify: npm run typecheck && npm run test
done: >-
  loaf kb check reports which knowledge files are stale (covered paths modified
  after last_reviewed via git log). loaf kb check --file <path> lists which
  knowledge files cover that path. loaf kb status now includes stale count.
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-035: KB staleness detection + check command

## Description

Implement the core innovation: staleness detection via `covers:` + `git log`. This is
the feature no other AI tool provides. Also add the `kb check` command (forward and
reverse lookup) and enhance `kb status` with stale count.

## Acceptance Criteria

- [ ] `cli/lib/kb/staleness.ts` implements:
  - Forward lookup: for each knowledge file with `covers:`, run
    `git log --since={last_reviewed} --format=%H -- <glob1> <glob2>` — if commits
    exist, file is stale
  - Reverse lookup: given a file path, use `picomatch` to test against all `covers:`
    patterns, return matching knowledge files
  - Respects `staleness_threshold_days` from config
- [ ] Uses `execFileNoThrow` from `src/utils/execFileNoThrow.ts` (not raw
  `child_process.exec`) for git commands to prevent command injection
- [ ] `loaf kb check` displays staleness report: stale files (with commit count and
  most recent author), fresh files, files without `covers:`
- [ ] `loaf kb check --file <path>` lists knowledge files whose `covers:` match the
  given path
- [ ] `loaf kb status` now includes stale count (enhances TASK-034's output)
- [ ] Both commands support `--json`
- [ ] Unit tests: mock execFileNoThrow for git log output; test stale/fresh
  classification, threshold logic, no-covers handling, reverse lookup via picomatch
- [ ] Integration tests with git fixtures (if feasible within scope)
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

## Context

See SPEC-009 for full context. Key insight: use `git log --since={date} -- <glob>`
directly — git handles pathspec globs natively. Single process invocation per knowledge
file, not per matched file.

**Important:** Use `execFileNoThrow` (not `exec`) for all shell commands — this
codebase has a safer alternative that prevents injection. See
`src/utils/execFileNoThrow.ts`.
