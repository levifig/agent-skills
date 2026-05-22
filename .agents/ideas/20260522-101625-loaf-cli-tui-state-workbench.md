---
title: Loaf CLI/TUI as the primary state workbench
captured: 2026-05-22T10:16:25Z
status: raw
tags: [cli, tui, state, workflow, interface]
related:
  - 20260315-014055-loaf-interactive-harness.md
  - 20260522-101624-sqlite-backed-operational-state.md
---

# Loaf CLI/TUI as the primary state workbench

## Nugget

If Loaf owns structured state, the CLI and a future TUI should become the primary interface for browsing, filtering, updating, and relating work. Git should not be the main UI for operational state; it should receive generated reviewable exports.

## Problem/Opportunity

Today users inspect `.agents/` files directly or rely on narrow CLI commands. A richer CLI/TUI could show unresolved triage queues, task boards, session histories, relationship graphs, release readiness, and follow-up status without requiring manual Markdown spelunking.

## Initial Context

This builds on the existing interactive harness idea but narrows the purpose: Loaf as the visualizer and controller for its own state model. The UI can remain terminal-first while still giving structured views that are hard to maintain in raw files.

---

*Captured via /idea -- shape with /shape when ready*
