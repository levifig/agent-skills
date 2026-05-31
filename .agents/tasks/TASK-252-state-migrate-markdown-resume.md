---
id: TASK-252
title: Implement markdown migration resume command
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T23:36:44Z'
updated: '2026-05-28T23:39:54Z'
completed_at: '2026-05-28T23:39:54Z'
depends_on:
  - TASK-251
files:
  - internal/cli/cli.go
  - internal/cli/cli_test.go
  - .agents/tasks/TASK-252-state-migrate-markdown-resume.md
verify: >-
  go test ./internal/cli -run 'TestRunnerStateMigrateMarkdownResume' -count=1
  && go test ./internal/cli
done: >-
  `loaf state migrate markdown --resume` is implemented as an idempotent
  resume/apply path with JSON and human output.
---

# TASK-252: Implement markdown migration resume command

## Description

SPEC-040 documents `loaf state migrate markdown --resume` in the migration
command surface, but the Go parser still returns "not implemented yet". The
current importer is already idempotent, so `--resume` should reuse the apply
path and label output as resume instead of inventing checkpoint semantics.

## Acceptance Criteria

- [x] `loaf state migrate markdown --resume` runs the idempotent migration
  apply path.
- [x] `--resume --json` returns the same structured result as `--apply --json`.
- [x] Human output labels the command as `loaf state migrate markdown --resume`.
- [x] `--resume` cannot be combined with `--apply` or `--dry-run`.
- [x] Tests cover JSON resume, human resume output, and invalid flag
  combinations.

## Verification

```bash
go test ./internal/cli -run 'TestRunnerStateMigrateMarkdownResume' -count=1
go test ./internal/cli
```
