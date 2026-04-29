---
id: TASK-130
title: Implement loaf soul command + cli/lib/souls library (list/current/show/use)
spec: SPEC-033
status: done
priority: P0
created: '2026-04-28T22:39:12.450Z'
updated: '2026-04-28T23:05:49.836Z'
depends_on:
  - TASK-129
completed_at: '2026-04-28T23:05:49.835Z'
---

# TASK-130: Implement loaf soul command + cli/lib/souls library

## Description

Build the `cli/lib/souls/` module (catalog reader, divergence detector, copy/install logic) and register `loaf soul` subcommands in `cli/commands/soul.ts`:

- `loaf soul list` — prints catalog souls with one-line descriptions
- `loaf soul current` — reads `loaf.json` and prints active soul (defaults to `none` if field missing)
- `loaf soul show <name>` — prints the catalog SOUL.md without copying or modifying anything
- `loaf soul use <name>` — copies catalog SOUL.md to `.agents/SOUL.md`. If the local file diverges from every catalog soul (sha256 mismatch), errors with a diff hint and requires `--force`. Updates `soul:` field in `loaf.json`.

Schema and install behavior live in TASK-131 — this task only adds the CLI surface and library, with tests stubbing `.agents/loaf.json` reads/writes via fixtures.

## Acceptance Criteria

- [ ] `cli/lib/souls/` module exposes catalog reader, divergence checker (sha256), copy operation, and config read/write helpers
- [ ] `loaf soul list` prints both catalog souls with one-line descriptions
- [ ] `loaf soul show <name>` prints catalog SOUL.md to stdout without filesystem writes
- [ ] `loaf soul use <name>` copies catalog SOUL.md to `.agents/SOUL.md` when no local file or local matches a catalog hash
- [ ] `loaf soul use <name>` errors with "local SOUL.md diverges from catalog; use --force to override" when local file diverges from all catalog hashes
- [ ] `loaf soul use <name> --force` overwrites unconditionally
- [ ] `loaf soul current` prints active soul, defaulting to `none` when field missing
- [ ] Unit + integration tests cover: list, show, use (fresh), use (matching), use (diverged blocked), use --force, current (with/without field)

## Verification

```bash
npm run build:cli && \
npm run typecheck && \
npm run test -- souls
```

## Context

See SPEC-033 test conditions T1–T5. Depends on TASK-129 (catalog files must exist for list/show to work). Does NOT modify `loaf.json` schema — TASK-131 owns that.
