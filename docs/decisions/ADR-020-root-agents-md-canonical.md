---
id: ADR-020
title: Root AGENTS.md as the Canonical Project Instruction File
status: Accepted
date: 2026-07-15
supersedes: ADR-010
---

# ADR-020: Root AGENTS.md as the Canonical Project Instruction File

## Context

ADR-010 consolidated per-harness prompt overlays into one `.agents/AGENTS.md` file and used root `AGENTS.md` as a convenience symlink. The consolidation remains sound, but the root symlink adds indirection at the standard discovery path and places a project instruction document inside the directory used by Loaf for state and configuration.

## Decision

Loaf uses `AGENTS.md` in the project root as the canonical real project instruction file. AGENTS.md-native harnesses read and write that file directly. Claude Code retains `.claude/CLAUDE.md` as a compatibility symlink to `../AGENTS.md`. The retired `.agents/AGENTS.md` path is migrated and removed; `.agents/` remains the home of Loaf project state and configuration.

This decision supersedes ADR-010's placement of the canonical file under `.agents/` while preserving ADR-010's single-overlay and deduplicated-write model.

## Rationale

- Root `AGENTS.md` is the native discovery path and should be the real source of truth.
- Removing the root symlink eliminates an unnecessary hop without reintroducing duplicate overlays.
- Keeping `.agents/` for Loaf state and configuration makes the boundary between project instructions and framework state explicit.
- Claude Code still requires its native path, so one compatibility symlink remains appropriate.
- Install, upgrade, and doctor can migrate the old layout without losing user-authored content.

## Alternatives Considered

- **Keep ADR-010 unchanged.** Rejected because the root standard path remains an indirection and `.agents/` continues to mix project instructions with Loaf state.
- **Duplicate root AGENTS.md into `.claude/CLAUDE.md`.** Rejected because two real files can drift and would undo ADR-010's central consolidation benefit.
- **Remove the Claude path.** Rejected because Claude Code still consumes `.claude/CLAUDE.md`; the compatibility symlink provides that path without duplicating content.

## Consequences

- Fresh `loaf init` and `loaf install` create a real root `AGENTS.md`.
- Fenced-section target routing maps AGENTS.md-native harnesses directly to root `AGENTS.md`; Claude's path resolves to the same file.
- `.claude/CLAUDE.md` points to `../AGENTS.md`.
- `loaf install` and `loaf install --upgrade` migrate the old root-symlink plus `.agents/AGENTS.md` layout before writing managed content.
- `loaf doctor` reports a root symlink or remaining `.agents/AGENTS.md` as fixable drift and checks the fenced version in root `AGENTS.md` without mutation. `--fix` offers each repair behind a default-no y/N prompt and preserves legacy content and backups when accepted; `--fix --force` accepts every offered repair without prompting, while non-interactive `--fix` skips repairs safely.
