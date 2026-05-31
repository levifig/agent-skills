---
title: Structured session transcripts and generated reports
captured: 2026-05-22T10:16:27Z
status: absorbed
absorbed_by: SPEC-040
tags: [sessions, transcripts, audit, reports, provenance]
related:
  - 20260522-101624-sqlite-backed-operational-state.md
  - 20260522-101626-generated-markdown-review-exports.md
  - SPEC-040
---

# Structured session transcripts and generated reports

## Nugget

Loaf sessions should capture proper structured logs: user prompts, agent responses, action summaries, decisions, tool metadata, and links to tasks or ideas. Raw/noisy tool output can stay local or summarized, while generated session reports provide a clean artifact for Git and audit.

## Problem/Opportunity

The current session journal is trying to be continuity memory, audit log, triage source, release evidence, and review artifact at once. Structured transcript rows plus generated reports would separate raw interaction history from curated audit evidence.

## Initial Context

The desired report shape includes enough prompt/response context for future reference and accountability, but not every tool response. A generated report could include a standardized timeline, decisions, actions taken, verification, unresolved follow-ups, and redacted/summarized tool activity.

---

*Captured via /idea -- shape with /shape when ready*
