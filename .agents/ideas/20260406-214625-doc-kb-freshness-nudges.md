---
title: Doc/KB freshness nudges at workflow front
status: raw
created: "2026-04-06T21:46:25Z"
tags: [knowledge-base, shape, implement, hooks]
---

# Doc/KB freshness nudges at workflow front

Doc/KB staleness checks exist at the middle and end of the workflow (post-commit nudge, release pre-flight) but not at the front where they'd prevent drift.

## Gaps

- `/shape` spec template has no "affected documentation" section — specs don't identify which docs the work will invalidate
- `/implement` breakdown doesn't generate doc-update tasks from the spec
- `workflow-pre-pr` hook doesn't cross-check branch diff against KB `covers:` fields

## Wasted nudge

- `session end` calls `consumeKnowledgeNudges` — prints recommendations to a conversation that's already over (SessionEnd fires as the agent quits). Remove it.

## Effective nudge points

- **Session start** — model is alive, can act (already surfaces stale count, could surface which files)
- **PreCompact** — last conscious moment before amnesia (already flushes journal, could flag KB)
- **`/shape`** — front-load by identifying affected docs in the spec
- **`/implement`** — generate doc tasks alongside code tasks
- **`workflow-pre-pr`** — last gate before review
