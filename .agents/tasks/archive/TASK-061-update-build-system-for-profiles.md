---
id: TASK-061
title: 'Update build system — remove substitutions, profile-aware discovery'
spec: SPEC-014
status: done
priority: P1
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-061: Update build system — remove substitutions, profile-aware discovery

Remove the `{{AGENT:...}}` substitution system and update agent discovery for the profile model.

## Remove substitution system

In `cli/lib/build/lib/substitutions.ts`:
- Remove `buildAgentMap()` function
- Remove `substituteAgentNames()` function
- Remove all `{{AGENT:...}}` pattern handling

## Update agent discovery

In `cli/lib/build/targets/claude-code.ts` (and other targets):
- Update `discoverAgents()` or agent processing to handle the new set: implementer, reviewer, researcher + system agents
- Remove references to `buildAgentMap()` and `substituteAgentNames()`
- Ensure `agentMap` parameter is removed from `copySkills()` and `copyAgents()` calls

## Update types

In `cli/lib/build/types.ts`:
- Clean up any agent-specific types that reference role agents
- Ensure `AgentFrontmatter` still works for profiles

## Remove plugin-groups from hooks.yaml

In `config/hooks.yaml`:
- Remove the entire `plugin-groups` section (legacy dead code — build system ignores it)

## Test
- `loaf build` succeeds for all 5 targets
- Zero `{{AGENT:` patterns in any built output
- `npm run typecheck` passes
- `npm run test` passes

## Relates to
- R0, R1, R2
