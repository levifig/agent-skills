---
id: TASK-071
title: Rework ~30 skill descriptions for two-tier routing
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T19:34:08.947Z'
completed_at: '2026-04-04T19:34:08.946Z'
---

# TASK-071: Rework ~30 skill descriptions for two-tier routing

Rewrite all skill `description` fields for cross-harness routing using the two-tier strategy.

## Scope

**Tier 1 — First 250 characters:** Must be self-sufficient for Claude Code's truncation routing. Pattern: `[Action verb] [core domain] [technologies]. [Trigger phrases].` This is the ONLY text Claude Code's routing model sees.

**Tier 2 — Full description (up to 1024 chars):** Used by Cursor, Codex, OpenCode, Amp (no truncation). Add negative routing for confusable skills ("Not for..."), success criteria for workflow skills ("Produces..."), edge-case disambiguation.

**Build-time verification:** Add logging during build that reports each description's char count and first-250 preview so routing quality can be verified without manual inspection.

## Constraints

- All descriptions must start with third-person action verbs
- Routing-critical phrases within first 250 chars
- No manual truncation in source — Claude Code target truncates at build time (TASK-074)
- Confusable skill pairs (e.g., debugging vs foundations, implement vs orchestration) need clear negative routing

## Verification

- [ ] Every description starts with third-person action verb
- [ ] Every description has routing-critical phrases within first 250 chars
- [ ] Full descriptions include negative routing for confusable skills
- [ ] Workflow skills include success criteria ("Produces...")
- [ ] Build output logs char count + first-250 preview for each skill
