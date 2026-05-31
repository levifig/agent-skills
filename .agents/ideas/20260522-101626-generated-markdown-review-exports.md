---
title: Generated Markdown exports for Git and review
captured: 2026-05-22T10:16:26Z
status: absorbed
absorbed_by: SPEC-040
tags: [reports, markdown, git, review, export]
related:
  - 20260522-101624-sqlite-backed-operational-state.md
  - 20260522-101625-loaf-cli-tui-state-workbench.md
  - SPEC-040
---

# Generated Markdown exports for Git and review

## Nugget

Markdown should become a generated view at trust boundaries: PR summaries, release-readiness reports, session reports, triage closure packets, and audit artifacts. The canonical operational state can live in SQLite, while Git gets stable, readable, standardized Markdown outputs.

## Problem/Opportunity

Human review still benefits from Markdown in the repository, but hand-maintained operational Markdown drifts and hides relationships. Generated reports can be formatted for their purpose instead of forcing sessions, tasks, and ideas to double as both state and review documents.

## Initial Context

The PR #50 release-prep work already needed a curated changelog, task archive evidence, verification evidence, and triage closure context. A report generator could produce those artifacts directly from structured state, making review easier without making Git the state database.

---

*Captured via /idea -- shape with /shape when ready*
