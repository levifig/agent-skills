---
id: TASK-259
title: Add source-derived five-target parity matrix test
spec: SPEC-047
status: done
priority: P1
created: '2026-06-24T12:03:41Z'
updated: '2026-06-24T12:53:35Z'
completed_at: '2026-06-24T12:53:35Z'
depends_on:
  - TASK-253
  - TASK-254
  - TASK-255
  - TASK-256
  - TASK-257
  - TASK-258
files:
  - internal/cli/build_test.go
  - cli/scripts/build-go.mjs
  - cli/scripts/verify-go-artifacts.mjs
  - bin/native/darwin-arm64/loaf
  - plugins/loaf/bin/native/darwin-arm64/loaf
  - config/hooks.yaml
  - content/skills/
  - .agents/tasks/TASK-259-source-derived-five-target-parity-matrix.md
verify: >-
  go test ./internal/cli -run 'ParityMatrix|BuildTarget' -count=1 && npm run
  build && npm run test && git diff --exit-code -- dist plugins
done: >-
  A source-derived parity matrix test enumerates exactly Claude Code, Codex,
  Cursor, OpenCode, and Amp and fails on skill reachability, hook semantic, or
  harness-language gaps.
---

# TASK-259: Add source-derived five-target parity matrix test

## Description

Add the durable regression guard for SPEC-047: derive expectations from
`content/skills`, skill sidecars, and `config/hooks.yaml`, build the five
first-class targets, and assert that generated output preserves reachability,
hook semantics, and harness-language hygiene.

## Acceptance Criteria

- [x] The parity test enumerates exactly Claude Code, Codex, Cursor, OpenCode,
  and Amp.
- [x] Expectations are derived from source inputs, not a hand-maintained static
  matrix.
- [x] Every `user-invocable` workflow skill is reachable through the target's
  native idiom.
- [x] Advisory hooks stay advisory and enforcement hooks stay enforcing on every
  supported hook surface.
- [x] The test fails on a seeded reachability gap.
- [x] The test fails on a seeded hook semantic gap.
- [x] The test fails on a seeded harness-language leak.

## Verification

```bash
go test ./internal/cli -run 'ParityMatrix|BuildTarget' -count=1
npm run build
npm run test
git diff --exit-code -- dist plugins
```
