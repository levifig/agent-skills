---
id: TASK-023
title: '`loaf build` command (JS wrapper)'
spec: SPEC-008
status: done
priority: P1
created: '2026-03-16T16:27:15.466Z'
updated: '2026-03-16T16:27:15.466Z'
depends_on:
  - TASK-021
  - TASK-022
files:
  - cli/commands/build.ts
  - cli/lib/build/build.js
  - package.json
verify: loaf build && diff -r dist/ dist-backup/
done: >-
  `loaf build` and `loaf build --target <name>` produce identical output to
  previous npm run build
completed_at: '2026-03-16T16:27:15.466Z'
---

# TASK-023: `loaf build` command (JS wrapper)

## Description

Create the `loaf build` Commander subcommand that wraps the existing (JS) build system. This is the first functional command — it proves the CLI skeleton works and the source reorganization didn't break anything.

Before starting: snapshot current dist/ output for comparison.

## Acceptance Criteria

- [ ] `loaf build` builds all 5 targets successfully
- [ ] `loaf build --target claude-code` builds only Claude Code target
- [ ] `loaf build --target nonexistent` shows helpful error listing valid targets
- [ ] Build output shows colored status per target (building/success/failure)
- [ ] Build output shows total timing info
- [ ] Error output is formatted clearly (not raw stack traces)
- [ ] dist/ output is functionally equivalent to previous `npm run build`
- [ ] Old npm build scripts removed from package.json
- [ ] `loaf` with no subcommand shows help including the build command

## Implementation Notes

- `cli/commands/build.ts` imports JS build modules from `cli/lib/build/`
- TypeScript can import JS modules — tsup handles the bundling
- Use Commander's `.command()`, `.option()`, `.action()` pattern
- Colored output: use ANSI codes directly or a lightweight chalk alternative

## Context

See SPEC-008 for full context. Depends on TASK-021 (CLI skeleton) and TASK-022 (source reorg).
Circuit breaker stage: 30%. Completing this task = first circuit breaker milestone.

## Work Log

<!-- Updated by session as work progresses -->
