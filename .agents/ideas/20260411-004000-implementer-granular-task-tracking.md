---
title: Implementer agents should use TaskCreate for granular step tracking
status: raw
created: 2026-04-11T00:40:00Z
tags: [agents, tasks, implementer, workflow]
related: [content/agents/implementer.md, content/skills/implement/SKILL.md]
---

# Implementer agents should use TaskCreate for granular step tracking

## Problem

When the orchestrator delegates multi-step work to an implementer (e.g., "fix 5 review findings"), the implementer does all steps as one opaque block. No granular progress tracking, no visibility into which steps are done/in-progress/failed, no resumability if the agent crashes mid-way.

## Fix (two-part)

### 1. Implementer profile (`content/agents/implementer.md`)
Add to behavioral contract: "Break your work into Task tool tasks (TaskCreate), one per discrete change. Mark each in_progress when starting, completed when done."

### 2. Orchestrator prompt convention
When spawning implementers with multi-step work, include: "Create a Task for each fix before starting it, and mark each complete as you finish."

Both together: profile sets the default, orchestrator reinforces for complex work.

## Benefits

- **Visibility** — progress as each step lands, not just a single "done" at the end
- **Audit trail** — TaskCompleted events auto-log to session journal
- **Resumability** — if agent crashes, remaining tasks are clearly identified
- **Accountability** — each step independently verified before marking complete

## Discovered

During SPEC-029 — "Fix 5 Codex review findings" was one opaque task instead of 5 trackable steps. Raised by user twice across sessions.
