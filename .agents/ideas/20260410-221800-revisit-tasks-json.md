---
title: Revisit TASKS.json — consider dropping in favor of frontmatter parsing
status: raw
created: 2026-04-10T22:18:00Z
tags: [architecture, tasks, cli]
related: [cli/commands/task.ts]
---

# Revisit TASKS.json

## Problem

TASKS.json is a machine-readable index of all tasks, auto-synced by PostToolUse hooks when task files change. It duplicates information already in task file frontmatter. The sync hooks add complexity and the JSON file is a merge conflict magnet.

## Trade-off

- **Keep:** `loaf task list` is fast (reads one JSON file vs scanning all .md files)
- **Drop:** one source of truth (frontmatter), no sync hooks, no merge conflicts, simpler

## Question

Is the performance difference meaningful? Most projects have <200 tasks (many archived). Parsing frontmatter from all `.md` files in `.agents/tasks/` is likely sub-100ms.

## Discovered

During SPEC-029 implementation — noticed TASKS.json was still being maintained despite earlier assumption it was removed.
