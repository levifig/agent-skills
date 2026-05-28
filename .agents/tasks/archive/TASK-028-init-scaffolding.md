---
id: TASK-028
title: '`loaf init` — scaffolding + templates'
spec: SPEC-007
status: done
priority: P1
created: '2026-03-16T16:27:15.460Z'
updated: '2026-03-17T00:16:46.232Z'
files:
  - cli/commands/init.ts
  - cli/index.ts
verify: loaf init (in a temp directory) && ls -la .agents/ docs/ CHANGELOG.md
done: >-
  `loaf init` creates full directory structure, templates, symlinks (with
  prompt), and loaf.json
completed_at: '2026-03-17T00:16:46.232Z'
---

# TASK-028: loaf init — scaffolding + templates

## Description

Create the `loaf init` CLI command that scaffolds a project for Loaf. Creates directories, template files, config, and symlinks. Never overwrites existing files.

## What to create

**Directories:**
- `.agents/` with `sessions/`, `ideas/`, `specs/`, `tasks/`
- `docs/knowledge/`, `docs/decisions/`

**Template files (if missing):**
- `.agents/AGENTS.md` — generic agent instructions template
- `.agents/loaf.json` — `{ "knowledge": { "local": ["docs/knowledge", "docs/decisions"] } }`
- `docs/VISION.md` — template with sections to fill
- `docs/STRATEGY.md` — template with sections to fill
- `docs/ARCHITECTURE.md` — template with sections to fill
- `CHANGELOG.md` — Keep-a-Changelog with `[Unreleased]`

**Symlinks (ask before creating):**
- `.claude/CLAUDE.md` → `.agents/AGENTS.md`
- `./AGENTS.md` → `.agents/AGENTS.md`

## Acceptance Criteria

- [ ] `loaf init` creates `.agents/` with all subdirectories
- [ ] Creates `.agents/AGENTS.md` with useful template
- [ ] Creates `docs/knowledge/`, `docs/decisions/`
- [ ] Creates strategic doc templates if missing
- [ ] Creates `CHANGELOG.md` in root if missing
- [ ] Creates `.agents/loaf.json` with default config
- [ ] Does NOT overwrite any existing files
- [ ] Asks before creating symlinks
- [ ] Symlinks point correctly when approved
- [ ] Colored output consistent with other loaf commands
- [ ] Registered in cli/index.ts

## Context

See SPEC-007 for full context. Circuit breaker 50%.

## Work Log

<!-- Updated by session as work progresses -->
