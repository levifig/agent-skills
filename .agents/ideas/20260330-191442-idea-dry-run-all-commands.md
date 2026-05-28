---
type: idea
created: '2026-03-30'
status: captured
---

# Idea: CLI Robustness — `--dry-run` for All Commands + Error Handling Hardening

## Nugget

Every `loaf` command that creates, modifies, or deletes files should support `--dry-run` to preview planned changes without executing them. Currently only `release` and `cleanup` have it.

## What It Would Do

Add `--dry-run` to: `build`, `install`, `init`, `setup`, `task`, `spec`. Each would output what it plans to do (files to write, dirs to create, copies to make) without actually doing it.

## Why

Discovered during `/release` skill development — the principle that destructive commands should be previewable applies uniformly. `install` is the highest-value addition since it writes into external config directories (`~/.config/`), where surprises are most costly.

## Priority Order

1. `install` — touches external dirs, highest risk of surprise
2. `build` — regenerable output, but useful for debugging target issues
3. `task` / `spec` — lower risk (writes to `.agents/`), but consistency matters
4. `init` / `setup` — one-time commands, lowest urgency

## Error Handling Hardening

Discovered during SPEC-019 PR review — pre-existing patterns in the release pipeline where bare `catch {}` blocks hide real failures. Should be fixed alongside `--dry-run` since it's the same files and the same "CLI robustness" theme.

**By severity:**

1. `prepareVersionUpdates` silently skips files it can't update — can produce inconsistent versions across `package.json` and `pyproject.toml` with no warning (`cli/lib/release/version.ts`)
2. `getCommitsSince` returns `[]` on git failure — "No unreleased changes" instead of an error (`cli/lib/release/commits.ts`)
3. `getLastTag` returns `null` on git failure — falls through to "all commits in history" (`cli/lib/release/commits.ts`)
4. `detectVersionFiles` silently skips unreadable files — wrong version source without warning (`cli/lib/release/version.ts`)
5. `scanIncompleteTasks` nested bare catches — incomplete task warning silently skipped (`cli/commands/release.ts`)
6. Editor failure catch hides the actual error message (`cli/commands/release.ts`)

**Pattern to apply:** Distinguish "expected failure" (git exit code, file not found) from "unexpected failure" (ENOENT for git binary, EACCES, spawn failures) — propagate the latter. Already applied in `refResolves` during SPEC-019 as the reference pattern.

## Open Questions

- Should `--dry-run` output be machine-readable (JSON) for piping, or just human-readable?
- Should `build --dry-run` show a diff of what would change in existing output files?
