---
title: Triage resolution graph for sparks, ideas, tasks, and specs
captured: 2026-05-22T10:16:28Z
status: raw
tags: [triage, sparks, ideas, tasks, graph, closure]
related:
  - 20260522-101624-sqlite-backed-operational-state.md
  - 20260411-003000-auto-scaffold-spark-ideas.md
---

# Triage resolution graph for sparks, ideas, tasks, and specs

## Nugget

Triage items should have first-class relationships and closure records: a spark can become an idea, an idea can become a task or spec, and a task can resolve one or more source items. Loaf should be able to close the whole chain and explain why an item will not resurface.

## Problem/Opportunity

Today closure depends on scattered conventions: `resolve(spark)` entries in journals, `status: archived` in idea frontmatter, archived task files, and manual memory of what implemented what. This makes "do not show this in triage again" fragile.

## Initial Context

PR #50 closed several loose tasks and ideas, but cleanup required manual updates across tasks, raw ideas, and journals. A resolution graph would support commands like `loaf triage close SPARK-123 --resolved-by TASK-184` or `loaf idea resolve IDEA-042 --by TASK-185`.

---

*Captured via /idea -- shape with /shape when ready*
