---
captured: 2026-03-24T22:27:30Z
status: raw
tags: [codex, cross-model-review, council, multi-agent]
---

# Codex Review Agent Skill

## Nugget

A skill that enables Claude to send requests to OpenAI's Codex CLI (via pipe or SDK) for independent code/plan/spec review. This creates a cross-model review loop where Codex provides a second opinion, particularly useful for council deliberations where 1-2 council seats could be Codex-backed agents.

## Problem/Opportunity

Currently, all review and council opinions come from Claude models. Having a structurally different model (Codex/GPT) review the same artifact surfaces blind spots that same-model review can't catch. This is especially valuable for specs, architectural decisions, and pre-merge code review where diverse perspectives reduce groupthink.

## Initial Context

- Codex CLI exists and can accept piped input or be invoked via SDK
- Council skill already supports multi-agent deliberation — this would plug in as a council participant type
- Could also be standalone: `/loaf:codex-review` for ad-hoc second opinions on plans, commits, diffs, specs
- Need to figure out: authentication flow, output parsing, latency tolerance, cost implications
- Related: council-session skill, code-review skill, architecture skill

---

*Captured via /loaf:idea -- shape with /loaf:shape when ready*
