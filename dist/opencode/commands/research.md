---
description: >-
  Conducts bounded project assessment and topic investigation. Use when the user
  asks for the current state, evidence, options, or a researched recommendation.
  Produces a direct synthesis or, when persistence is justified, a
  skill-specific temporary report. Not for implementation or multi-agent
  coordination.
subtask: false
version: 0.5.0
---

# Research

Investigate enough to support a decision, then stop. Research returns through the harness by default; a file is an exception earned by a concrete future use.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(research): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when research can safely proceed.
- Establish the decision, scope, time bound, and needed confidence before broad investigation. Ask one question at a time when missing context materially changes the research.
- Read project evidence first, then authoritative primary sources, then lower-confidence sources only when needed. Cite each material claim at the strongest available source.
- Distinguish observation, inference, recommendation, and uncertainty.
- Return through the harness by default. Persist a report only for asynchronous work, cross-conversation use, a large evidence set, multiple consumers, or an explicit user request.
- When persistence is warranted, create an unused `.agents/reports/YYYYMMDDHHMMSS-slug.md` path. Generate the 14-digit timestamp in UTC, describe the report itself in the slug, and never overwrite an existing file.
- Use the purpose-specific [research report](templates/research-report.md) or [state assessment](templates/state-assessment.md) template. Do not add universal status or lifecycle fields.
- Put tracker or task provenance in the report content when useful, never in the filename.
- Log a concise durable discovery when the result changes future decisions; do not copy the full report into the journal.

## Verification

- The result answers the decision or question that bounded the work.
- Material claims cite sources and label uncertainty honestly.
- Options state meaningful tradeoffs and the recommendation follows from evidence.
- A report file exists only when persistence was justified and its path is returned through the harness.
- Any written report uses a research-owned template, UTC naming, and no universal report status.

## Quick Reference

| Need | Output |
|------|--------|
| Immediate answer or compact investigation | Harness response only |
| Current project overview needed beyond this response | [State assessment](templates/state-assessment.md) |
| Durable evidence, options, or audit trail | [Research report](templates/research-report.md) |
| Parallel or specialist investigation | Use orchestration; research still owns the research template |

## Topics

| Topic | Template | Use When |
|-------|----------|----------|
| Research report | [research-report.md](templates/research-report.md) | Persisting evidence, options, and a recommendation |
| State assessment | [state-assessment.md](templates/state-assessment.md) | Persisting a project state overview and immediate next actions |
