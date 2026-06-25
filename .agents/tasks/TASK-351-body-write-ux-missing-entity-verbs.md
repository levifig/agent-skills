---
id: TASK-351
title: Add body write UX and missing entity verbs
spec: SPEC-043
status: done
priority: P1
created: '2026-06-24T13:04:19Z'
updated: '2026-06-24T13:57:17Z'
completed_at: '2026-06-24T13:57:17Z'
depends_on:
  - TASK-350
files:
  - internal/cli/cli.go
  - internal/state/
  - internal/cli/cli_reference.go
  - content/skills/cli-reference/SKILL.md
  - dist/
  - plugins/
  - .agents/tasks/TASK-351-body-write-ux-missing-entity-verbs.md
verify: >-
  go test ./internal/cli ./internal/state -run 'Body|ReportShow|BrainstormCapture|Plan|Handoff|Council' -count=1
  && npm run build
done: >-
  Body-capable entities support non-file body creation/editing, report show,
  brainstorm capture, and first-class plan/handoff/council storage with generated
  CLI reference output in sync.
---

# TASK-351: Add body write UX and missing entity verbs

## Description

Add SPEC-043 Track 2: CLI body input UX and the missing verbs/entities needed to
create and retrieve SQLite-bodied artifacts without writing in-tree Markdown.

## Acceptance Criteria

- [x] Body input supports `--body-file <path>`, `--body -`, `--message <text>`, and `$EDITOR` fallback with documented precedence.
- [x] Non-UTF8 body input is rejected before SQLite or FTS writes.
- [x] `loaf report show` displays SQLite-bodied reports and Markdown-fallback reports.
- [x] `loaf brainstorm capture` creates a SQLite-bodied brainstorm artifact.
- [x] `plan`, `handoff`, and `council` have SQLite storage and `new/show/list/link` coverage appropriate to SPEC-043.
- [x] Creating a body-capable artifact can show it with no in-tree `.agents/*.md` file present.
- [x] Generated CLI reference files are rebuilt and committed with the source changes.

## Verification

```bash
go test ./internal/cli ./internal/state -run 'Body|ReportShow|BrainstormCapture|Plan|Handoff|Council' -count=1
npm run build
```
