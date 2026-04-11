---
title: Auto-scaffold idea files when sparks are logged
status: raw
created: 2026-04-11T00:30:00Z
tags: [cli, sparks, ideas, workflow]
related: [cli/commands/session.ts, content/skills/wrap/SKILL.md, content/skills/release/SKILL.md]
---

# Auto-scaffold idea files when sparks are logged

## Problem

`loaf session log "spark(scope): ..."` writes to the journal but doesn't create an idea file. Two separate actions — the second is easy to forget. During SPEC-029, 2 of 5 sparks had journal entries but no idea files. They would have been lost after archival without manual pre-merge checking.

## Proposed fix (layered)

### Primary: auto-scaffold on spark log
When `loaf session log` detects a `spark()` type entry, also create a minimal idea file in `.agents/ideas/`. The journal entry becomes a pointer (`*(captured)*`), the idea file is the durable artifact. One command, both artifacts.

### Safety net: pre-merge spark audit
Add to the release skill's housekeeping check (Step 3): scan session journal for `spark()` entries, match against `.agents/ideas/` filenames, flag uncaptured sparks. Could also be a CLI command: `loaf session sparks --uncaptured`.

### Nice-to-have: wrap enforcement
Before wrap summary, run `loaf session sparks --uncaptured` and require idea file creation for any hits. Redundant if primary fix works.

## Discovered

During SPEC-029 pre-merge review — manually found 2 uncaptured sparks that would have been lost.
