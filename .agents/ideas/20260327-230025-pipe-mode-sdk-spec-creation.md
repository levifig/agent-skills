---
title: "Pipe mode / SDK for spec and task creation"
captured: 2026-03-27T23:00:25Z
status: raw
tags: [autonomous-execution, cli, sdk]
related: []
origin: drafts/20260314-brainstorm-loaf-cli-knowledge-harness.md
---

# Pipe mode / SDK for spec and task creation

## Nugget

Use Claude Code SDK or `claude --pipe` for AI-assisted spec generation, task breakdown, and knowledge file scaffolding. Enables programmatic creation of structured artifacts without interactive sessions.

## Problem/Opportunity

Creating specs, breaking down tasks, and scaffolding knowledge files currently requires an interactive agent session. A pipe/SDK mode would enable batch operations, CI integration, and the overnight implementation loop described in VISION.md.

## Initial Context

Claude Code SDK now exists. Aligns with the "Autonomous Execution" pillar. The `verify:` and `done:` fields on tasks already support machine-checkable completion criteria.

---

*Captured via /idea -- shape with /shape when ready*
