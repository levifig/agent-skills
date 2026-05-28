---
title: "Brainstorm: Skill Activation Model + Agent Elimination"
type: brainstorm
created: 2026-03-24T23:30:00Z
status: active
tags: [skill-activation, agents, architecture]
related: [SPEC-014]
---

# Brainstorm: Skill Activation Model + Agent Elimination

**Date:** 2026-03-24
**Session:** skill-organization-refactor

## Problem

Two related issues with Loaf's knowledge activation:

1. **Foundations skill is too broad to trigger precisely.** Its description ("Establishes code quality, commit conventions, documentation standards, and security patterns") doesn't match specific contexts like "merge this PR." Concrete symptom: squash merge conventions (in `commits.md`) weren't loaded during PR merge (2026-03-24).

2. **Role-based agents are thin routers adding indirection without value.** All 8 role agents (pm, backend-dev, frontend-dev, qa, dba, design, devops, power-systems) have 2-sentence system prompts that say "Your skills tell you how." Skills contain all domain knowledge. The routing layer adds nothing that description-based skill activation doesn't already provide.

## Research inputs

- Claude Platform best practices: "Design coherent units... like deciding what a function should do"
- agentskills.io: "Skills scoped too narrowly force multiple skills to load. Skills scoped too broadly become hard to activate precisely."
- Cursor: "Start simple. Add rules only when you notice the agent making the same mistake repeatedly."
- Codex: "Keep each skill focused on one job." Description is the implicit activation mechanism.
- Claude Code subagent docs: Agents are actively invested in (memory, isolation, effort, teams) but built-in agents (Explore, Plan, general-purpose) are functional, not role-based.

## Directions explored

### Direction 1: Kill all role agents, skills only
Remove routing layer entirely. Skills + built-in agents.
- **Gains:** Simplest. No agent selection step.
- **Loses:** Tool scoping, model routing, context isolation.

### Direction 2: Functional agents (AmpCode-style)
Rename from roles to functions (implementer, auditor). Skills preloaded.
- **Challenge:** "Implementer" ≈ built-in general-purpose. No meaningful difference.

### Direction 3: Modes, not agents
Configuration overlays (implementing mode, reviewing mode).
- **Problem:** Claude Code doesn't have a modes primitive. Would need to be implemented as agents with skills preloaded — same as Direction 2.

### Direction 4: Two-layer (skills always-on, agents for isolation only)
All knowledge via skills. Agents exist only for infrastructure (context isolation, tool restriction).
- **Challenge:** When do you actually need custom isolation that built-in agents don't cover?

## Convergence

All four directions converge on the same insight:

> **Skills are the only knowledge layer. Agents are infrastructure, not identity.**

### Decisions made

1. **Kill all 8 role agents.** They're skill routers with trivial system prompts. Description-based skill activation replaces them.
2. **Keep 2 system agents** (background-runner, context-archiver). These have real procedural prompts and serve infrastructure purposes.
3. **Don't create replacement agents.** No "implementer", no "auditor." Built-in Explore/Plan/general-purpose cover isolation cases. If a future workflow needs custom isolation, create it then — named for function, not role.
4. **PM process becomes always-active.** Orchestration skill (task tracking, session management, verification) loads universally, not gated behind a PM agent.
5. **Split foundations** into coherent domain clusters for precise triggering.

### Skill layer after changes

| Type | Skills | Activation |
|---|---|---|
| **Always-on (reference)** | foundations (slimmed), git-workflow, debugging, security, documentation, orchestration, language skills, domain skills | Context/description matching |
| **On-demand (workflow)** | implement, breakdown, research, brainstorm, shape, reflect, council-session, etc. | User-invoked via `/skill-name` |
| **System agents** | background-runner, context-archiver | Spawned by workflow skills for isolation |

### Key challenge: tool scoping

The design agent's read-only restriction is the strongest argument for role agents. But in interactive development:
- You don't need to *prevent* Claude from writing during a review — you just don't ask it to write
- Tool restriction is for autonomous execution safety, not interactive workflows
- If autonomous tool restriction is needed later, create a functional agent *then*

## Next step

Shape as a single spec covering both tracks:
- **Track A:** Foundations decomposition (split into 4-5 focused skills)
- **Track B:** Agent elimination (remove 8 role agents, make process skills always-on)

They ship together because agent elimination depends on skills being granular enough for context-based activation.

## Sparks

- **Council sessions without agents** — if agents are gone, council deliberation needs a new model: skill-composed perspectives? Structured prompting? Multi-pass with different system prompt overlays? *(discarded — covered by SPEC-016)*
- **Skill activation analytics** — instrument which skills actually trigger on which prompts, to measure whether description improvements help. Build into `loaf build` or a test harness. *(promoted)*
- **Plugin-group simplification** — with agents gone, plugin-groups become purely about skill bundling. Simplify the config model. *(discarded — handled within SPEC-014)*
- **Skill presets** — named bundles of skills that activate together, lighter than agents. "Python web dev" = python-development + database-design + infrastructure-management + foundations. *(discarded — description-based activation should suffice)*
