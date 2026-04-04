---
id: TASK-068
title: Refactor Cursor target + fix prompt hook filter bug
spec: SPEC-020
status: todo
priority: P0
created: '2026-04-04T16:41:22.294Z'
updated: '2026-04-04T16:41:22.294Z'
---

# TASK-068: Refactor Cursor target + fix prompt hook filter bug

Rewrite `cursor.ts` skills/agents to use shared modules. Fix the prompt hook filter bug.

## Scope

**Skills refactor:** Rewrite `copySkills()` (lines 81-137) to read from `dist/skills/`. Cursor adds `assets/` directory handling beyond the base.

**Agents refactor:** Rewrite `copyAgents()` to use shared `copyAgents()` module with `sidecarRequired: false`.

**Bug fix:** Remove prompt hook filter at ~line 269. The current code filters out `type: "prompt"` hooks from both preToolHooks and postToolHooks. Cursor natively supports prompt hooks — this filter is unnecessarily restrictive. The `workflow-pre-merge` prompt hook should now appear in Cursor's generated `hooks.json`.

## Constraints

- Functionally identical output except prompt hooks now pass through
- `assets/` directory handling preserved
- Hook generation logic NOT refactored (Phase 3)

## Verification

- [ ] Functionally identical skill/agent output
- [ ] `workflow-pre-merge` prompt hook appears in Cursor's generated `hooks.json`
- [ ] `assets/` dirs still copied correctly
- [ ] `npm run typecheck` and `npm run test` pass
