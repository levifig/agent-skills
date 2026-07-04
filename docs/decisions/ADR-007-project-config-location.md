---
id: ADR-007
title: Project Config in .agents/loaf.json
status: Proposed
date: 2026-03-14
---

# ADR-007: Project Config in `.agents/loaf.json`

## Decision

Project-level Loaf configuration lives in `.agents/loaf.json`. JSON format. Namespaced under `.agents/` to avoid conflicts with other tools.

## Context

Loaf needs a project-level config for knowledge imports, staleness thresholds, and future implementation settings (model choices, review tools, commit conventions). Config needs to be committed to git and shared with the team.

## Rationale

- `.agents/` is Loaf's established project directory (already had specs, tasks, and sessions at the time; the session entity was later removed by SPEC-056 / ADR-019)
- `loaf.json` is namespaced — no conflicts with `.eslintrc`, `tsconfig.json`, etc.
- JSON: machine-readable, parseable by agents and CLI, no YAML ambiguity
- Designed to grow: v1 uses `knowledge` section, future adds `implementation`, `defaults`

## Alternatives Considered

- `.loafrc` — non-standard, unclear format
- `package.json` section — couples to Node.js
- YAML — ambiguity concerns, less universal tooling
- `.agents/config.json` — too generic, could conflict with other agent tools

## Consequences

- All Loaf project settings in one file
- CLI reads/writes this file
- Schema must be forward-compatible (new sections don't break old parsers)
