---
id: SPEC-035
title: "TASKS.json staleness + housekeeping sweep gap"
source: ideas/20260325-012120-task-staleness-prevention.md, ideas/20260410-221800-revisit-tasks-json.md
created: 2026-05-01T23:46:45Z
status: drafting
shape_status: stub
source_ideas:
  - 20260325-012120-task-staleness-prevention
  - 20260410-221800-revisit-tasks-json
---

# SPEC-035: TASKS.json staleness + housekeeping sweep gap

> **Stub status:** This spec is a draft pointer to two raw ideas. Substantive shaping (problem decomposition, solution direction, scope boundaries, test conditions) deferred to a future `/loaf:shape SPEC-035` session. Source ideas carry the full thinking; this file reserves the SPEC-035 slot and unifies the framing.

## Stub Problem Framing

Two related, recurring frictions in Loaf's task management:

1. **TASKS.json drift.** Two sources of truth (`TASKS.json` index + per-task `.md` frontmatter) with no automatic reconciliation. Drift surfaces as: archived tasks still showing `todo`, manual `.md` edits not reflected in JSON, `next_id` falling behind, missing `obsolete` status for superseded work.

2. **Housekeeping sweep gap.** `/housekeeping` skill recommends "archive done tasks via `loaf task archive`" but doesn't compute the diff (TASKS.json `status==done` MINUS files already in `archive/`) or pass IDs to the CLI. So the same pile of done-but-unarchived tasks survives multiple housekeeping passes. Observed concretely on 2026-05-01: 18 done tasks (TASK-103, 105, 111–115, 120–128, 136, 148, 149) accumulated unarchived through several `/housekeeping` invocations until manually swept.

The two are linked but separable: (1) is an architectural data-model question (single vs dual source of truth); (2) is a skill-prose responsibility gap that should hold regardless of (1)'s resolution.

## Source Ideas (Substantive Thinking Lives Here)

| Idea | Captured | Direction |
|---|---|---|
| [`20260325-012120-task-staleness-prevention`](../ideas/20260325-012120-task-staleness-prevention.md) | 2026-03-25 | Diagnose dual-source drift; propose three options (eliminate JSON / make it a cache / auto-sync hook); add `obsolete` status |
| [`20260410-221800-revisit-tasks-json`](../ideas/20260410-221800-revisit-tasks-json.md) | 2026-04-10 | Narrower follow-up: just drop TASKS.json. Frontmatter is fast enough at <200 tasks; sync hooks are merge-conflict magnets |

## Shaping Prompts (For Future `/shape` Session)

When this spec is shaped, the interview should resolve:

- **Architecture choice:** eliminate TASKS.json, make it a cache, or auto-sync hook? (Three options from the staleness-prevention idea.)
- **`obsolete` status:** worth adding alongside `done` and `todo`?
- **Housekeeping responsibility:** is the sweep behavior part of `/housekeeping` skill prose, a new CLI subcommand (`loaf task archive --all-done`), or both?
- **Migration path:** if TASKS.json is dropped, how does `loaf task list` performance hold? Is there a soft-deprecation window?
- **Scope boundary:** is the housekeeping sweep gap tackled as a same-spec sub-track, or as its own micro-spec sequenced before the architectural change?
- **Linear-native interaction:** in Linear-native mode, tasks live in Linear, not `.agents/tasks/`. How do these changes interact? (Likely: changes apply only in local-tasks mode; Linear-native mode unaffected.)

## Provenance

This spec was drafted as a stub during SPEC-034 shaping when the housekeeping sweep gap surfaced concretely (18 tasks accumulated unarchived). The user indicated existing ideas already covered this ground and requested SPEC-035 be drafted from them rather than recapturing as new ideas. Full shaping deferred until the user has appetite to interview through the architecture choice.
