---
id: TASK-192
title: Add TypeScript legacy-command delegation bridge
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T15:13:09Z'
updated: '2026-05-28T16:14:54Z'
depends_on:
  - TASK-191
files:
  - cmd/loaf/main.go
  - internal/legacy/
  - dist-cli/
  - package.json
  - cli/index.ts
verify: >-
  go test ./... && npm run build:cli && go run ./cmd/loaf --help && build a
  temporary Go binary that delegates `task list --json` to dist-cli/index.js
  from a temporary .agents/TASKS.json fixture
done: >-
  Go front controller delegates unmigrated commands to the bundled TypeScript
  CLI while keeping `state` commands native and exposing one public command
completed_at: '2026-05-28T16:14:54Z'
---

# TASK-192: Add TypeScript legacy-command delegation bridge

## Description

Implement the transitional bridge that lets the Go `loaf` front controller own
dispatch while preserving existing TypeScript command behavior. `state` remains
Go-native. Unknown or unmigrated top-level commands delegate to the bundled
TypeScript CLI with argv, cwd, stdio, and exit code preserved.

This task should make the migration shape concrete enough for release/build
work to package one public `loaf` command.

## Acceptance Criteria

- [x] Go dispatch keeps `state` commands native.
- [x] Unmigrated commands delegate to the existing TypeScript CLI.
- [x] Delegation preserves stdout, stderr, stdin, cwd, env, and exit code.
- [x] `--help` and `--version` behavior is explicitly defined and tested during the bridge period.
- [x] Failure to locate the TypeScript fallback produces an actionable error.
- [x] Tests cover native command, delegated command, delegated failure, and exit-code propagation.
- [x] No second public command name is introduced.

## Context

SPEC-040 and ADR-014 both reject a big-bang rewrite and require one public
`loaf` surface. This bridge is the mechanism that lets Go migration happen
command-by-command.

## Verification

```bash
go test ./...
npm run build:cli
go run ./cmd/loaf --help
tmp_root=$(mktemp -d)
repo_root=$PWD
mkdir -p "$tmp_root/project/.agents"
printf '{"version":1,"next_id":1,"tasks":{},"specs":{}}\n' > "$tmp_root/project/.agents/TASKS.json"
go build -o "$tmp_root/loaf-go" ./cmd/loaf
(cd "$tmp_root/project" && LOAF_LEGACY_CLI="$repo_root/dist-cli/index.js" "$tmp_root/loaf-go" task list --json)
```
