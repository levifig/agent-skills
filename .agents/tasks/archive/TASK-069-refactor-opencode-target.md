---
id: TASK-069
title: Refactor OpenCode target to use shared intermediate
spec: SPEC-020
status: done
priority: P0
created: '2026-04-04T16:41:22.294Z'
updated: '2026-04-04T19:34:04.385Z'
completed_at: '2026-04-04T19:34:04.384Z'
---

# TASK-069: Refactor OpenCode target to use shared intermediate

Rewrite `opencode.ts` skills/agents to use shared modules.

## Scope

**Skills:** Rewrite `copySkills()` (lines 70-118) to read from `dist/skills/`. OpenCode sidecar merge + version injection.

**Agents:** Rewrite `copyAgents()` (lines 120-143) to use shared `copyAgents()` with `sidecarRequired: false`.

**Commands:** `generateCommandsFromSkills` (lines 145-190) stays target-specific but reads skill frontmatter from the intermediate rather than source.

## Constraints

- Do NOT refactor the runtime plugin generator (`generateHooks`/`generateHooksJs` at lines 197-367) — that gets rewritten in Phase 3
- Functionally identical output
- Command generation logic preserved as-is, just reads from new location

## Verification

- [ ] Functionally identical `dist/opencode/` output
- [ ] Commands still generated correctly from sidecar metadata
- [ ] Runtime plugin (hooks.js) unchanged
- [ ] `npm run typecheck` and `npm run test` pass
