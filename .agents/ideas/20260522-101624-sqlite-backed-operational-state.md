---
title: SQLite-backed operational state for Loaf
captured: 2026-05-22T10:16:24Z
status: raw
tags: [sqlite, state, tasks, ideas, sessions, triage, architecture]
related:
  - 20260325-012120-task-staleness-prevention.md
  - 20260315-014055-loaf-interactive-harness.md
---

# SQLite-backed operational state for Loaf

## Nugget

Loaf should stop treating Markdown as the canonical database for operational state. A local SQLite store can hold tasks, sessions, sparks, ideas, links, statuses, timestamps, provenance, and resolution state, while Markdown remains useful for durable prose and generated review artifacts.

## Problem/Opportunity

The current `.agents/` model makes Markdown do two jobs: human-readable artifacts and queryable lifecycle state. That creates recurring cleanup pain: a task can be done, an idea can be implemented, and a spark can still resurface because the relationship between them is implicit text rather than enforced state.

## Initial Context

The immediate trigger was repeated triage cleanup after PR #50: tasks, raw ideas, and session sparks each have different closure conventions. A relational store would let Loaf answer questions like "which task resolved this idea?" and "why is this spark showing up again?" without grep-based inference.

---

*Captured via /idea -- shape with /shape when ready*
