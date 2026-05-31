---
id: TASK-235
title: Prove public loaf bridge dispatch and packaging
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T21:50:30Z'
updated: '2026-05-28T21:53:00Z'
completed_at: '2026-05-28T21:53:00Z'
depends_on:
  - TASK-234
files:
  - cmd/loaf/
  - internal/cli/
  - internal/legacy/
  - cli/scripts/verify-go-artifacts.mjs
  - .agents/tasks/TASK-235-public-loaf-bridge-proof.md
verify: >-
  go test ./cmd/loaf ./internal/cli ./internal/legacy && npm run build
done: >-
  Tests prove the built public `loaf` binary dispatches Go-native state
  commands and delegates unmigrated commands through the TypeScript bridge, and
  artifact verification enforces one packaged public command with synchronized
  Go and TypeScript fallback assets.
---

# TASK-235: Prove public loaf bridge dispatch and packaging

## Description

Close the SPEC-040 Track 0 public-command proof conditions with direct tests and
artifact checks. Existing runner tests cover the Go dispatcher, but this task
proves the actual built `cmd/loaf` binary can exercise both sides of the bridge
without exposing a second public command name.

## Acceptance Criteria

- [x] A test builds the public `loaf` binary from `cmd/loaf`.
- [x] The built binary dispatches `state path` natively without requiring the
  TypeScript fallback.
- [x] The built binary delegates an unmigrated command to the configured
  TypeScript fallback and preserves argv/cwd.
- [x] Artifact verification fails if package metadata exposes more than the
  single public `loaf` command.
- [x] Build verification proves Go binary and TypeScript fallback assets are
  synchronized into the packaged plugin output.

## Verification

```bash
go test ./cmd/loaf ./internal/cli ./internal/legacy
npm run build
```
