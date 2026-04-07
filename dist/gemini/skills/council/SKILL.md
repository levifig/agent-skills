---
name: council
description: >-
  Convenes multi-agent council deliberations for decisions requiring diverse
  perspectives. Use when the user asks "call a council", "gather specialists",
  "get a council opinion", or "what do the experts think". Also activate when
  the user wants a structured debate between domain-specific viewpoints. Not for
  single-perspective research (use research) or architectural decisions that
  don't need multi-agent deliberation (use architecture).
version: 2.0.0-dev.16
---

# Council

Convene multi-agent councils for complex decisions requiring diverse expert perspectives.

## Critical Rules

**Always**
- Select odd number of agents (5 or 7) — never even
- Spawn ALL agents in parallel in a single response
- Present composition rationale and get user approval before proceeding
- Document each perspective and create synthesis with consensus/disagreements
- Wait for explicit user decision — council advises, user decides
- Archive council after decision recorded in session
- Log decision to session journal: `loaf session log "decision(scope): council outcome and user's choice"`

**Never**
- Use even number of agents (risks ties)
- Spawn agents sequentially — parallel only
- Proceed without user approval at composition step
- Make the decision yourself — present synthesis, let user choose
- Skip documenting minority perspectives

## Verification

After work completes, verify:
- Council file created at `.agents/councils/{YYYYMMDD}-{HHMMSS}-{topic}.md`
- All 5-7 agents spawned and perspectives collected
- Synthesis includes consensus points, disagreements, trade-offs
- User decision recorded in council file
- Session updated with council outcome summary
- Council archived after session includes outcome

## Quick Reference

### Council Composition

Select 5-7 agents covering relevant domains:
- Technical domains (backend, frontend, infrastructure)
- Quality domains (security, performance, UX)
- Process domains (testing, DevOps, documentation)

### Spawn Pattern

All agents in ONE response:
```
Spawn agent 1 (domain X) + agent 2 (domain Y) + agent 3 (domain Z) + ...
```

Each agent receives:
- Decision question and options
- Domain-specific analysis factors
- Instruction to focus on THEIR expertise only

### Synthesis Structure

1. Consensus points (all agents agree)
2. Key disagreements (different perspectives)
3. Trade-off analysis per option
4. Overall recommendation with confidence level
5. Explicit deferral if genuinely ambiguous

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Council Template | [templates/council.md](templates/council.md) | Creating council files |
| Composition | `orchestration/references/councils.md` | Selecting council agents |
| Delegation | `orchestration/references/delegation.md` | Spawning subagents |
