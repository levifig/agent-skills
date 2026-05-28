---
title: "Loaf CLI reference skill for agents"
captured: 2026-03-25T01:22:45Z
status: raw
tags: [skill, cli, agent-guidance, discoverability]
related: [knowledge-base, implement, orchestration]
---

# Loaf CLI Reference Skill

## Nugget

A non-user-invocable reference skill that teaches agents when and why to use each
`loaf` CLI command. Not a man page — a routing table that maps agent situations to
the right command.

## Problem/Opportunity

Agents currently learn about `loaf` commands through scattered references across
skills (knowledge-base mentions `loaf kb review`, implement mentions `loaf task`).
No single place teaches "here's the full CLI, here's the decision tree for which
command to use." As the CLI grows (kb, task, spec, init, release, build, install),
command discoverability becomes a real problem.

## Initial Context

- Should be a **reference skill** (`user-invocable: false`) — agents load it for
  context, users don't invoke it
- Focus on **when/why** (routing logic), not **how** (flags/syntax) — `--help` exists
  for that
- Could be auto-generated from CLI command metadata at build time, keeping it in sync
- Risk: goes stale if manually maintained. Auto-generation from command registrations
  would solve this.
- Consider: should this be a skill, or a section in CLAUDE.md, or generated --help
  output that agents can read?

---

*Captured via /idea — shape with /shape when ready*
