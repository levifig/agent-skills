---
id: TASK-037
title: QMD soft integration + kb init command
spec: SPEC-009
status: done
priority: P2
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
depends_on:
  - TASK-033
files:
  - cli/lib/kb/qmd.ts
  - cli/commands/kb.ts
verify: npm run typecheck && npm run test
done: >-
  loaf kb init creates docs/knowledge/ and docs/decisions/ idempotently. If QMD
  is installed, registers collections. If not, skips gracefully with a message.
  qmd.ts exports isQmdAvailable() and collection management functions.
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-037: QMD soft integration + kb init command

## Description

Implement the QMD integration module (soft dependency pattern) and the `kb init`
command that scaffolds the knowledge directory structure.

## Acceptance Criteria

- [ ] `cli/lib/kb/qmd.ts` implements:
  - `isQmdAvailable(): boolean` — checks via `which qmd`
  - `registerCollection(name, path)` — calls `qmd collection add`
  - `removeCollection(name)` — calls `qmd collection remove`
  - All QMD calls wrapped in try/catch with helpful error messages
- [ ] `loaf kb init`:
  - Creates `docs/knowledge/` and `docs/decisions/` if missing
  - Creates/updates `.agents/loaf.json` with knowledge config
  - If QMD available: registers `{repo}-knowledge` and `{repo}-decisions` collections
  - If QMD not available: prints info message suggesting install, continues without error
  - Idempotent: re-running doesn't error, duplicate, or overwrite
  - Reports what it did (created/skipped/registered)
- [ ] `--json` support for init output
- [ ] Unit tests for QMD availability detection (mock execSync)
- [ ] Integration tests for init idempotency
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

## Context

See SPEC-009 for full context. QMD lightweight mode (BM25-only) is sufficient — no
model downloads needed. Collection naming: `{repo-folder}-knowledge`,
`{repo-folder}-decisions`.
