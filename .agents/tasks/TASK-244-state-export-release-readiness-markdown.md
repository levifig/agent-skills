---
id: TASK-244
title: Export release-readiness Markdown from SQLite state
spec: SPEC-040
status: done
priority: P1
created: '2026-05-28T22:34:35Z'
updated: '2026-05-28T22:41:31Z'
completed_at: '2026-05-28T22:41:31Z'
depends_on:
  - TASK-243
files:
  - internal/state/
  - internal/cli/
  - .agents/tasks/TASK-244-state-export-release-readiness-markdown.md
verify: >-
  go test ./internal/state ./internal/cli -run 'TestExportReleaseReadinessMarkdown|TestRunnerStateExportReleaseReadinessMarkdown' -count=1 && go test ./internal/state ./internal/cli
done: >-
  `loaf state export release-readiness --format markdown` renders deterministic
  external-safe release-readiness Markdown from SQLite status, task, spec,
  session, report, relationship, and generated-export state without mutating the
  database or repository files.
---

# TASK-244: Export release-readiness Markdown from SQLite state

## Description

Close the SPEC-040 generated-export gap for
`loaf state export release-readiness --format markdown`. The export should be
generated from SQLite state and printed to stdout.

This task should not implement the separate `loaf report generate` command
family, create persisted report rows, or write repository Markdown.

## Acceptance Criteria

- [x] `loaf state export release-readiness --format markdown` parses as a
  native Go state command.
- [x] The export renders release-readiness status from SQLite state, including
  unresolved work counts, archive/readiness warnings, source provenance coverage,
  generated-export coverage, and recent reports/sessions.
- [x] The export is external-safe: it does not expose `SPEC-*`, `TASK-*`,
  `.agents/...`, track, or phase identifiers.
- [x] The export is deterministic and does not mutate SQLite state.
- [x] The export does not create repository files.
- [x] Missing and invalid SQLite state fail with the same state-export errors as
  existing export commands.
- [x] Tests cover the state package exporter and public `Runner` command path.

## Verification

```bash
go test ./internal/state ./internal/cli -run 'TestExportReleaseReadinessMarkdown|TestRunnerStateExportReleaseReadinessMarkdown' -count=1
go test ./internal/state ./internal/cli
```
