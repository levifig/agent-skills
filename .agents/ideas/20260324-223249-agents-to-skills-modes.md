---
captured: 2026-03-24T22:32:49Z
status: raw
tags: [architecture, agents, skills, modes, ampcode]
---

# Migrate Role-Based Agents to Skill-Based Modes

## Nugget

Replace the current role-based agent system (PM, Backend Dev, QA, etc.) with a skill-based mode system. Instead of spawning a "PM agent," PM capabilities are always-active skills. Instead of calling a "QA agent," entering a review context activates QA mode with its skills loaded. Think AmpCode's named subagents (Oracle, Librarian, Painter) — functional identities, not anthropomorphized roles.

## Problem/Opportunity

Current agents are thin routers: they load a system prompt and delegate to skills. The agent abstraction adds indirection without proportional value. A mode/skill model would:
- Make capabilities always-available rather than gated behind agent invocation
- Let context (the user's request) determine which skills activate, not explicit agent selection
- Reduce the orchestration overhead of multi-agent delegation
- Align with the CLI-driven harness direction already underway (SPEC-010 was the first step)

## Initial Context

- Prior art captured in: `.claude/plans/foamy-pondering-summit.md` (line 150-152) and project memory (`project_agent_to_skill_harness.md`)
- AmpCode reference model: https://ampcode.com/manual#subagents — Oracle, Librarian, Painter pattern
- Current agents to transform: pm, backend-dev, frontend-dev, qa, dba, devops, design, power-systems
- Some agents (like `implement`) may remain as subagents for isolation, but with functional names not role names
- Council sessions would need rethinking — modes composing perspectives instead of agent seats
- SPEC-010 (task CLI) already absorbed pm's task management role as a proof point

---

*Captured via /loaf:idea -- shape with /loaf:shape when ready*
