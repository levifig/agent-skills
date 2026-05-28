---
id: TASK-025
title: Convert target transformers to TypeScript
spec: SPEC-008
status: done
priority: P2
created: '2026-03-16T16:27:15.466Z'
updated: '2026-03-16T16:27:15.466Z'
depends_on:
  - TASK-024
files:
  - cli/lib/build/targets/claude-code.ts
  - cli/lib/build/targets/opencode.ts
  - cli/lib/build/targets/cursor.ts
  - cli/lib/build/targets/codex.ts
  - cli/lib/build/targets/gemini.ts
verify: tsc --noEmit && loaf build && diff -r dist/ dist-backup/
done: >-
  All 5 target transformers are TypeScript. Build output unchanged. Old build/
  directory removed.
completed_at: '2026-03-16T16:27:15.466Z'
---

# TASK-025: Convert target transformers to TypeScript

## Description

Convert all 5 target transformer modules from vanilla JS to TypeScript. Each implements the target transformer interface defined in TASK-024. This is the largest conversion task (~1953 lines).

## Conversions

| From (JS) | To (TS) | Lines |
|-----------|---------|-------|
| `targets/claude-code.js` | `targets/claude-code.ts` | ~618 |
| `targets/opencode.js` | `targets/opencode.ts` | ~499 |
| `targets/cursor.js` | `targets/cursor.ts` | ~465 |
| `targets/codex.js` | `targets/codex.ts` | ~187 |
| `targets/gemini.js` | `targets/gemini.ts` | ~184 |

Total: ~1953 lines

## Acceptance Criteria

- [ ] All 5 target transformers converted to TypeScript
- [ ] Each implements the target transformer interface from TASK-024
- [ ] Shared utility types for common operations (file copying, frontmatter handling)
- [ ] `tsc --noEmit` passes with no errors
- [ ] `loaf build` output unchanged from JS version
- [ ] Old `build/` directory removed (all build logic now in `cli/lib/build/`)
- [ ] Old JS files in `cli/lib/build/targets/` removed

## Implementation Notes

- claude-code.ts is the most complex (plugins, hooks, MCP, sidecar merging)
- codex.ts and gemini.ts are simpler (skills only)
- Look for patterns to share across targets (they likely have duplicated logic)
- Don't refactor target logic — just convert to TypeScript. Refactoring is TASK-027 territory.

## Context

See SPEC-008 for full context. Depends on TASK-024 (shared types + interface).
Circuit breaker stage: 50%. Completing this task = second circuit breaker milestone.

## Work Log

<!-- Updated by session as work progresses -->
