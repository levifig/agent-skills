---
id: TASK-024
title: Convert build core to TypeScript
spec: SPEC-008
status: done
priority: P2
created: '2026-03-16T16:27:15.466Z'
updated: '2026-03-16T16:27:15.466Z'
depends_on:
  - TASK-023
files:
  - cli/lib/build/orchestrator.ts
  - cli/lib/build/lib/version.ts
  - cli/lib/build/lib/substitutions.ts
  - cli/lib/build/lib/sidecar.ts
  - cli/lib/build/lib/shared-templates.ts
verify: tsc --noEmit && loaf build && diff -r dist/ dist-backup/
done: >-
  Build orchestrator and utilities are TypeScript. `loaf build` output
  unchanged.
completed_at: '2026-03-16T16:27:15.466Z'
---

# TASK-024: Convert build core to TypeScript

## Description

Convert the build orchestrator and utility modules from vanilla JS to TypeScript. Define types for config structures and the target transformer interface.

## Conversions

| From (JS) | To (TS) | Lines |
|-----------|---------|-------|
| `build.js` | `orchestrator.ts` | ~125 |
| `lib/version.js` | `lib/version.ts` | ~32 |
| `lib/substitutions.js` | `lib/substitutions.ts` | ~77 |
| `lib/sidecar.js` | `lib/sidecar.ts` | ~122 |
| `lib/shared-templates.js` | `lib/shared-templates.ts` | ~63 |

Total: ~420 lines

## Acceptance Criteria

- [ ] All 5 files converted to TypeScript
- [ ] Types defined for hooks.yaml config structure
- [ ] Types defined for targets.yaml config structure
- [ ] Target transformer interface defined (used by TASK-025)
- [ ] `tsc --noEmit` passes with no errors
- [ ] `loaf build` output unchanged from JS version
- [ ] Old JS files removed from `cli/lib/build/`

## Implementation Notes

- Use `strict: true` but allow pragmatic escape hatches for YAML config handling
- The orchestrator dynamically loads target modules — may need explicit imports for tsup bundling
- gray-matter and yaml packages need @types or type declarations

## Context

See SPEC-008 for full context. Depends on TASK-023 (build command working with JS).
Circuit breaker stage: 50%.

## Work Log

<!-- Updated by session as work progresses -->
