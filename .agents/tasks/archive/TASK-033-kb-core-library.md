---
id: TASK-033
title: 'KB core library — types, loader, resolve, config'
spec: SPEC-009
status: done
priority: P1
created: '2026-03-24T19:29:16Z'
updated: '2026-03-24T19:29:16Z'
files:
  - cli/lib/kb/types.ts
  - cli/lib/kb/loader.ts
  - cli/lib/kb/resolve.ts
  - cli/commands/kb.ts
  - cli/index.ts
  - .agents/loaf.json
  - package.json
verify: npm run typecheck && npm run test
done: >-
  loader.ts can scan docs/knowledge/ and docs/decisions/, parse frontmatter via
  gray-matter, and return typed KnowledgeFile objects. resolve.ts finds git root
  and loads loaf.json config. picomatch is a direct dependency.
  registerKbCommand() is registered in index.ts (even if subcommands are stubs).
completed_at: '2026-03-24T19:29:16Z'
---

# TASK-033: KB core library — types, loader, resolve, config

## Description

Build the foundation modules that all `loaf kb` commands depend on. This is pure
library code — no user-facing commands yet (except the registered parent command).

## Acceptance Criteria

- [ ] `cli/lib/kb/types.ts` defines: `KnowledgeFile`, `KnowledgeFrontmatter`,
  `KbConfig`, `StalenessResult`, `ValidationResult`
- [ ] `cli/lib/kb/resolve.ts` finds git root (`git rev-parse --show-toplevel`),
  loads `.agents/loaf.json`, returns typed config with defaults
- [ ] `cli/lib/kb/loader.ts` scans configured `local` directories, parses YAML
  frontmatter via `gray-matter`, returns `KnowledgeFile[]`
- [ ] `picomatch` added as direct dependency in `package.json`
- [ ] `.agents/loaf.json` schema expanded: `staleness_threshold_days` (default 30),
  `imports` (default [])
- [ ] `registerKbCommand(program)` registered in `cli/index.ts`
- [ ] Unit tests for loader (valid/invalid frontmatter, multi-dir scanning, missing dirs)
- [ ] Unit tests for resolve (config loading, defaults, missing config)
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

## Context

See SPEC-009 for full context. Follow patterns from `cli/lib/tasks/` (types.ts,
resolve.ts). Use `gray-matter` for frontmatter parsing (already a dependency).
