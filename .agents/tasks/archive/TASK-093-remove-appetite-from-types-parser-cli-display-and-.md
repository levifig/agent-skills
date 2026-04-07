---
id: TASK-093
title: 'Remove appetite from types, parser, CLI display, and tests'
spec: SPEC-025
status: done
priority: P1
created: '2026-04-07T10:42:33.953Z'
updated: '2026-04-07T10:47:57.088Z'
completed_at: '2026-04-07T10:47:57.087Z'
---

# TASK-093: Remove appetite from types, parser, CLI display, and tests

## Description

Remove the `appetite` field from all TypeScript types, parsing logic, CLI display, and tests.

## Key Files

- `cli/lib/tasks/types.ts` — `SpecEntry`/`SpecFrontmatter` types
- `cli/lib/tasks/parser.ts` — spec parsing
- `cli/lib/tasks/migrate.ts` — migration logic
- `cli/commands/spec.ts` — CLI display
- `cli/lib/tasks/parser.test.ts`, `migrate.test.ts`, `archive.test.ts`, `scanner.test.ts` — tests

## Acceptance Criteria

- [ ] `appetite` removed from `SpecEntry`/`SpecFrontmatter` types
- [ ] Parser no longer reads/writes appetite
- [ ] CLI spec display no longer shows appetite
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes
- [ ] `grep -r "appetite" cli/` returns zero matches

## Verification

```bash
npm run typecheck && npm run test && ! grep -r "appetite" cli/
```
