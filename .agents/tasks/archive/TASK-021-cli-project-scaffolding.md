---
id: TASK-021
title: CLI project scaffolding
spec: SPEC-008
status: done
priority: P1
created: '2026-03-16T16:27:15.466Z'
updated: '2026-03-16T16:27:15.466Z'
files:
  - cli/index.ts
  - package.json
  - tsconfig.json
  - tsup.config.ts
verify: npm link && loaf --help && loaf --version
done: '`loaf` command is globally available via npm link, shows help and version'
completed_at: '2026-03-16T16:27:15.466Z'
---

# TASK-021: CLI project scaffolding

## Description

Set up the TypeScript CLI project skeleton with Commander.js, tsup bundling, and the `loaf` entry point. This establishes the foundation that all other SPEC-008 tasks build on.

## Acceptance Criteria

- [ ] `cli/` directory created with `index.ts` entry point
- [ ] Commander.js setup with global `--help` and `--version` flags
- [ ] `--version` reads from package.json
- [ ] `loaf` with no subcommand shows help (not an error)
- [ ] tsup.config.ts bundles CLI to single JS file
- [ ] tsconfig.json with `strict: true`
- [ ] package.json has `bin` field pointing to bundled output
- [ ] package.json has publish-ready fields (main, exports, etc.)
- [ ] Commander.js, tsup, typescript added as dependencies/devDependencies
- [ ] `npm link` makes `loaf` available as a global command
- [ ] `tsc --noEmit` passes with no errors

## Context

See SPEC-008 for full context. This is the first task — no dependencies.
Circuit breaker stage: 30%.

## Work Log

<!-- Updated by session as work progresses -->
