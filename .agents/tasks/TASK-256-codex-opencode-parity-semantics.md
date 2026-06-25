---
id: TASK-256
title: Preserve Codex hook semantics and OpenCode command reachability
spec: SPEC-047
status: done
priority: P1
created: '2026-06-24T12:03:41Z'
updated: '2026-06-24T12:32:20Z'
completed_at: '2026-06-24T12:32:20Z'
depends_on:
  - TASK-255
files:
  - internal/cli/build_codex.go
  - internal/cli/build_opencode.go
  - internal/cli/build_test.go
  - config/hooks.yaml
  - .agents/tasks/TASK-256-codex-opencode-parity-semantics.md
verify: >-
  go test ./internal/cli -run
  'TestRunnerBuildTargetCodex|TestRunnerBuildTargetOpenCode|CodexHook|OpenCodeCommand'
  -count=1
done: >-
  Codex preserves advisory/enforcement and conditional hook fields, and OpenCode
  generates commands for every user-invocable workflow skill rather than only
  sidecar-bearing skills.
---

# TASK-256: Preserve Codex hook semantics and OpenCode command reachability

## Description

Fix two parity gaps after the target matrix is stable: Codex hook generation must
preserve advisory/enforcement semantics and conditional fields, while OpenCode
command generation must be based on workflow skill reachability rather than
sidecar presence.

## Acceptance Criteria

- [x] Codex hooks default `failClosed` to `false`.
- [x] Codex parses `failClosed` as true only when explicitly set to true.
- [x] Codex carries `blocking` and `if` through source parsing and emitted JSON.
- [x] Enforcement hooks in `config/hooks.yaml` still emit as enforcing.
- [x] Every `user-invocable: true` workflow skill gets an OpenCode command.
- [x] `user-invocable: false` reference skills do not get OpenCode commands.

## Verification

```bash
go test ./internal/cli -run 'TestRunnerBuildTargetCodex|TestRunnerBuildTargetOpenCode|CodexHook|OpenCodeCommand' -count=1
```
