---
description: >-
  Triages canonical native tracker candidates without creating a local intake
  queue. Use when deciding which existing records should advance, defer, close,
  or return to discovery. Produces verified native dispositions and explicit
  gaps.
user-invocable: true
version: 0.5.0
---

# Triage

Operate only on native tracker candidates through [`project-management/v1`](../project-management/SKILL.md). Triage decides disposition from current evidence; it does not maintain a parallel queue or invent universal workflow states.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- As the first action, run `loaf journal log "skill(triage): <concise intent>"` against the current private local journal. If the write fails, report the failure and continue only when the work can safely proceed; never put invocation bookkeeping in the tracker.
- Confirm the exact native destination and candidate references before reading or mutating.
- Read each candidate, its current workflow state, and relevant recent comments before deciding.
- Route an unrecorded raw direction to pitch or shape; do not capture it in a local issue or intake ledger.
- Base dispositions on evidence, strategic fit supplied by the user or project context, duplication, readiness, and blockers.
- Prefer the next complete rideable operator journey over isolated component layers. If a candidate is foundation work, retain its explicit link to the immediate journey that consumes it; triage does not invent or shape that journey.
- Read valid native workflow states before any transition. Never assume names such as backlog, canceled, or done exist.
- Use comments only to explain a disposition when collaboration benefits; the native state and fields carry the disposition itself.
- Re-read every changed candidate and return partial or unsupported results independently.

## Verification

- Every candidate disposition names its native reference and observed starting state.
- Every mutation used a runtime-supported provider operation and was verified by readback.
- No candidate disappeared because one independent mutation failed.
- Ambiguous destination, permissions, or workflow mapping is reported without mutation.
- The final result distinguishes advanced, deferred, closed, unchanged, failed, and indeterminate records in provider-native terms.
- Comparisons distinguish complete operator outcomes from enabling layers without treating foundation depth as product progress.

## Quick Reference

| Evidence | Disposition |
|----------|-------------|
| Problem still unclear | Return to pitch |
| Narrative clear, contract incomplete | Advance to shape |
| Duplicate or superseded | Use supported native fields/state and explain with evidence |
| Blocked by missing decision | Preserve native state or use a supported deferred state |
| Ready and already shaped | Leave canonical fields intact; hand to implement |

## Topics

No supporting references. Use the [tracker update](templates/tracker-update.md) when a self-contained disposition comment helps collaborators.
