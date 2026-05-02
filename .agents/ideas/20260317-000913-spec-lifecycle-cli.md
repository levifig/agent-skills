---
captured: 2026-03-17T00:09:13Z
status: raw
---

# Spec Lifecycle CLI + AI-Assisted Project Intelligence

## Idea

Extend the Loaf CLI with spec management commands (`loaf spec update`, `loaf spec archive`) and explore AI-powered project intelligence — using Claude/Codex SDK to auto-detect stale statuses, suggest completions, and maintain project health.

## Context

We have `loaf task update/create` for task lifecycle but nothing for specs. Status corrections (marking shipped specs as complete, archiving finished work) are manual. This came up when reviewing 12 specs and finding 3 with stale statuses that didn't match reality.

## Questions to Explore

- Should `loaf spec update SPEC-010 --status complete` mirror the task update pattern?
- Should `loaf spec archive SPEC-008` automate the move-to-archive + metadata update?
- Could `loaf doctor` or `loaf health` use Claude SDK to scan git history, detect shipped specs, suggest status corrections?
- Where's the line between deterministic CLI commands and AI-assisted intelligence?
- Would this be better as a skill (agent-driven) or CLI command (human-driven) or both?

## Potential Scope

- `loaf spec update` / `loaf spec archive` — deterministic, no AI needed
- `loaf health` / `loaf doctor` — AI-assisted, scans git log + specs + tasks, reports inconsistencies
- Could integrate with `/crystallize` to suggest knowledge extraction alongside status fixes
