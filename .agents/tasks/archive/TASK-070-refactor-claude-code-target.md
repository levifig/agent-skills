---
id: TASK-070
title: Refactor Claude Code target to use shared intermediate
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T19:34:06.891Z'
completed_at: '2026-04-04T19:34:06.891Z'
---

# TASK-070: Refactor Claude Code target to use shared intermediate

Rewrite `claude-code.ts` skills/agents to use shared modules. Most complex target.

## Scope

**Skills:** Rewrite `copySkills()` (lines 281-319) to read from `dist/skills/`. Claude Code uses `loadSkillExtensions()` for sidecar merge and applies `/loaf:` scoping via the `knownCommands` list. The per-target transform: copy from intermediate -> merge Claude Code sidecar -> apply `/loaf:` scoping pass.

**Agents:** Rewrite `copyAgents()` (lines 263-279) to use shared `copyAgents()` with `sidecarRequired: true`.

## Constraints

- Functionally identical `plugins/loaf/` output
- Plugin JSON, hooks, templates all unchanged
- `loadSkillExtensions()` stays as the Claude-specific sidecar merger
- `/loaf:` scoping pass preserved for multi-plugin disambiguation
- Hook copying (`copyAllHooks`) stays unchanged

## Verification

- [ ] Functionally identical `plugins/loaf/` output
- [ ] Plugin JSON structure unchanged
- [ ] Agent frontmatter unchanged
- [ ] `/loaf:` scoped commands present where expected
- [ ] `npm run typecheck` and `npm run test` pass
