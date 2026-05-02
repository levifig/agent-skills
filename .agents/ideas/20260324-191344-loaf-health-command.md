---
captured: 2026-03-24T19:13:44Z
status: raw
tags: [cli, project-health, knowledge, hooks]
related: [SPEC-009]
---

# loaf health — Project Health Aggregation Command

## Idea

A generic `loaf health` (or `loaf doctor`) command that aggregates project hygiene
signals into a single view: stale knowledge files, unprocessed sparks from brainstorms,
overdue tasks, spec status, and other health indicators.

Designed to be the **SessionStart aggregation point**, replacing individual per-feature
hooks with a single `loaf health --json` call that surfaces everything an agent needs
at session start.

## Context

Emerged during SPEC-009 shaping. The SessionStart hook needs to surface multiple
signals (kb staleness, sparks, tasks), but building that aggregation into SPEC-009
would expand scope beyond knowledge management. Better as a standalone command that
each feature contributes health checks to.

## Constraints

- Must support `--json` for hook consumption
- Should be extensible — each `loaf` subsystem registers its own health checks
- Shape after SPEC-009 ships — real experience with `loaf kb status` will inform design
- SPEC-009's kb-only SessionStart hook is designed to be replaced by `loaf health`

## Related

- SPEC-009 (knowledge management) — ships kb-only SessionStart hook first
- `20260317-000913-spec-lifecycle-cli.md` — spec status staleness is a health signal
