---
id: TASK-065
title: 'Extract shared content modules (skills, agents, commands)'
spec: SPEC-020
status: done
priority: P0
created: '2026-04-04T16:41:22.293Z'
updated: '2026-04-04T19:33:37.192Z'
completed_at: '2026-04-04T19:33:37.191Z'
---

# TASK-065: Extract shared content modules (skills, agents, commands)

Create three shared modules in `cli/lib/build/lib/` that extract duplicated logic from all 5 target transformers.

## Scope

**`skills.ts`** — `copySkills(options: CopySkillsOptions)`: discovers skills, loads base frontmatter, applies markdown transforms, copies subdirectories (references/, templates/, scripts/), distributes shared templates. The `mergeFrontmatter` callback lets targets inject sidecar fields.

**`agents.ts`** — `copyAgents(options: CopyAgentsOptions)`: discovers agents, loads base frontmatter + optional target sidecar, merges with defaults, writes formatted output. `sidecarRequired` flag differentiates Claude Code (required) from Cursor/OpenCode (optional).

**`commands.ts`** — `createCommandSubstituter(targetName)`: returns a `(content: string) => string` function. Universal unscoped substitution (`/implement`, `/resume`). Claude Code retains optional post-substitution `/loaf:` scoping pass applied separately.

## Constraints

- Types and modules only — no target files changed in this task
- Interfaces match the spec signatures (`CopySkillsOptions`, `CopyAgentsOptions`)
- Extract common patterns from existing targets; don't invent new abstractions
- Reuse existing utilities (`copyDirWithTransform`, `loadSkillFrontmatter`, etc.)

## Verification

- [ ] `npm run typecheck` passes
- [ ] New modules export the specified interfaces
- [ ] No target files modified
- [ ] `npm run test` passes
