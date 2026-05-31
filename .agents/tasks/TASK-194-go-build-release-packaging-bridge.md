---
id: TASK-194
title: Wire Go build and release packaging for one public loaf command
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T15:13:09Z'
updated: '2026-05-28T16:38:40Z'
depends_on:
  - TASK-192
  - TASK-193
files:
  - package.json
  - tsup.config.ts
  - cli/commands/build.ts
  - cli/lib/build/
  - plugins/loaf/bin/loaf
  - .github/workflows/
verify: npm run build && npm run test && go test ./...
done: >-
  Build and release workflows produce one user-facing `loaf` command backed by
  the Go front controller, with TypeScript fallback assets packaged for
  unmigrated commands
completed_at: '2026-05-28T16:38:40Z'
---

# TASK-194: Wire Go build and release packaging for one public loaf command

## Description

Update build, release, and packaged artifact wiring so the Go front controller
is the public `loaf` command while TypeScript fallback assets remain available
for unmigrated commands.

This task should make the packaging model real across local development,
npm/plugin distribution, and CI verification. Keep scope limited to packaging
the bridge; do not port additional command families.

## Acceptance Criteria

- [x] `npm run build` builds the TypeScript fallback and Go front controller in the correct order.
- [x] Packaged/plugin `loaf` entrypoint invokes the Go front controller.
- [x] TypeScript fallback assets are present wherever delegated commands need them.
- [x] CI/build verification detects missing or stale Go binary artifacts.
- [x] Release artifact refresh handles Go output without sweeping unrelated files into commits.
- [x] Existing build target outputs still generate successfully.
- [x] `npm run test`, `npm run build`, and `go test ./...` pass.

## Context

This completes SPEC-040 Track 0. After this task, Track A can implement SQLite
storage helpers and lifecycle commands in Go without reopening the one-command
packaging question.

## Verification

```bash
npm run build
npm run test
go test ./...
```
