---
title: Expand librarian to documentation writer with writing personality
status: raw
created: 2026-04-11T02:01:00Z
tags: [agents, librarian, documentation, reflect, writing]
related: [content/agents/librarian.md, content/skills/reflect/SKILL.md]
---

# Librarian as documentation writer

## Idea

The librarian profile already tends session files and journal entries. Expand it to own ALL documentation writing — session wraps, strategic doc updates (ARCHITECTURE.md, STRATEGY.md, VISION.md), knowledge base files, and ADRs. The `/reflect` skill should delegate its writing to the librarian instead of the orchestrator writing directly.

## Why

- **Consistent voice** — the librarian develops a writing personality (concise, precise, proper markdown formatting, backtick code identifiers, uppercase spec IDs). Today different agents write docs in different styles.
- **Quality enforcement** — a dedicated writer profile can enforce formatting conventions that the orchestrator and implementer don't internalize (we just caught missing backticks and lowercase IDs in the wrap summary)
- **Separation of concerns** — the orchestrator decides WHAT to reflect on, the librarian decides HOW to write it. Same pattern as enrichment (CLI decides what to extract, librarian decides what to journal).
- **Personality** — the Ent archetype (patient, thorough, long-memoried) is perfect for documentation. Treebeard didn't rush.

## Scope expansion

Current librarian scope: `.agents/` artifacts only (sessions, journals, state)

Proposed scope: `.agents/` + `docs/` (strategic docs, knowledge base, ADRs, changelogs)

This means the librarian can write to `docs/ARCHITECTURE.md`, `docs/STRATEGY.md`, `docs/VISION.md`, and `docs/knowledge/`. Still can't touch code, tests, or config — that's Smith work.

## Integration points

- `/reflect` → delegates writing to librarian
- `/wrap` → delegates wrap summary to librarian  
- `/housekeeping` → librarian already involved, extend to KB updates
- Enrichment → librarian already does this
- ADR writing → currently done by implementer, should be librarian
