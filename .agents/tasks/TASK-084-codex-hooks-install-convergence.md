---
id: TASK-084
title: Codex hook output + install convergence
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
completed_at: '2026-04-04T16:41:22.296Z'
---

# TASK-084: Codex hook output + install convergence

Finalize Codex hook generation and converge install logic around `.agents/skills/`.

## Implementation

### Codex hook output
- `dist/codex/.codex/hooks.json` with Bash-matching enforcement hooks only
- 5 hooks: `check-secrets`, `validate-push`, `validate-commit`, `workflow-pre-pr`, `security-audit`
- No Edit/Write matchers (Codex platform limitation — Bash-only)
- `loaf install --to codex` places at `$CODEX_HOME/hooks.json` (respecting env var)

### Install convergence
Updated `cli/lib/install/installer.ts`:

| Tool | Skills destination | Hooks destination |
|---|---|---|
| Amp | `.agents/skills/` or `~/.config/agents/skills/` | `.amp/plugins/` |
| Codex | `.agents/skills/` or `~/.agents/skills/` | `$CODEX_HOME/hooks.json` |
| Cursor | `.agents/skills/` (native discovery) | Plugin-bundled |
| Claude Code | Plugin-bundled | Plugin-bundled |
| OpenCode | Plugin-specific path | Plugin-specific path |

### User-hooks coexistence
- `loaf install` writes only Loaf-namespaced hook entries
- Existing user hooks in shared config files preserved
- `loaf install --upgrade` replaces only Loaf-owned entries

## Verification

- [x] `loaf install --to codex` installs hooks to `$CODEX_HOME/hooks.json`
- [x] Install convergence works for `.agents/skills/` across Amp, Codex, Cursor
- [x] User hooks preserved in shared config files
- [x] `loaf install --upgrade` replaces only Loaf-owned entries
