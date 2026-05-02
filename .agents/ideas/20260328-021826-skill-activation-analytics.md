---
captured: 2026-03-28T02:18:26Z
status: raw
tags: [skills, analytics, activation, testing]
related: [SPEC-014]
---

# Skill Activation Analytics

## Idea

Instrument which skills actually trigger on which prompts to measure whether description
improvements help. Could be built into `loaf build` as a test harness or as a standalone
`loaf skill test` command that runs sample prompts against skill descriptions and reports
activation rates.

## Why

SPEC-014 will restructure skill descriptions and activation patterns. Without measurement,
we can't tell if the changes actually improve triggering precision. This provides the
feedback loop.

## Source

Promoted from brainstorm spark (20260324-brainstorm-skill-activation-agent-elimination.md).
