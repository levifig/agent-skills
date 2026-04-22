---
id: ADR-010
title: Consolidate Prompt Overlay Around AGENTS.md
status: Accepted
date: 2026-04-17
---

# ADR-010: Consolidate Prompt Overlay Around AGENTS.md

## Decision

Loaf's managed prompt-overlay section ("fenced section") is written to a single canonical file: `.agents/AGENTS.md`. Harness-specific paths (`.claude/CLAUDE.md`, `./AGENTS.md`) are symlinks to it. `loaf install` enforces those symlinks; `loaf doctor` flags drift.

Target → file mapping:

| Target | File | Mechanism |
|---|---|---|
| claude-code | `.claude/CLAUDE.md` | Symlink → `.agents/AGENTS.md` |
| cursor | `.agents/AGENTS.md` | Native AGENTS.md support |
| codex | `.agents/AGENTS.md` | Native AGENTS.md support |
| opencode | `.agents/AGENTS.md` | Native AGENTS.md support |
| amp | `.agents/AGENTS.md` | Native AGENTS.md support |
| gemini | `.agents/AGENTS.md` | Native AGENTS.md support |

## Context

SPEC-020 (target convergence) shipped with Cursor mapped to `.cursor/rules/loaf.mdc` and Gemini CLI excluded from the fenced-section install entirely. Both decisions were correct at the time: Cursor had no AGENTS.md support, and Gemini CLI had no project-overlay layer.

By April 2026, AGENTS.md reached critical adoption — 23 tools listed at agents.md, including Cursor, Gemini CLI, Codex, opencode, Amp, VS Code, Zed, Warp, Windsurf, Junie, Devin, and more. Running separate conventions per tool is now the fragmented path.

## Rationale

- One fenced section to maintain, not six.
- `.agents/AGENTS.md` aligns with the Loaf convention (all agent state in `.agents/`) and with the AGENTS.md open standard.
- Symlinks bridge harnesses (like Claude Code) that use different native paths without duplicating content.
- Deduplication at install time (by resolved path via `realpath`) prevents redundant writes when multiple targets share the canonical file.
- `loaf doctor` provides a general-purpose misalignment check — useful for this migration and future convention drift.

## Alternatives Considered

- **Keep per-target files, no consolidation.** Rejected: duplication risk, maintenance overhead, no benefit given AGENTS.md adoption.
- **Cursor-specific `.mdc` rule.** Rejected: `alwaysApply: true` fence in a dedicated rules file gives no feature beyond AGENTS.md for Loaf's current content.
- **Drop fenced section for Gemini entirely.** Rejected: Gemini CLI now supports AGENTS.md; excluding it leaves users without the session-journal discipline block.

## Consequences

- `cli/lib/install/fenced-section.ts` maps `cursor` and `gemini` to `.agents/AGENTS.md`; `.mdc` frontmatter special-casing is removed.
- `installFencedSectionsForTargets` dedupes writes by `realpath` — 5 of 6 targets resolve to the same file.
- `loaf install` enforces `.claude/CLAUDE.md → ../.agents/AGENTS.md` and `./AGENTS.md → .agents/AGENTS.md` symlinks before fenced-section writes land.
- New `loaf doctor` command surfaces symlink drift, stale `.cursor/rules/loaf.mdc`, version mismatches, and duplicate fenced sections.
- No user-facing migration shim — single-user tool.
- `cli/lib/install/symlinks.ts` extracted as a shared helper with a 4-state machine (missing / correct / wrong-symlink / real-file).
