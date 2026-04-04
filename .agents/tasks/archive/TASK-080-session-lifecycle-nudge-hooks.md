---
id: TASK-080
title: Generate session lifecycle + journal nudge hooks
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T19:34:33.742Z'
completed_at: '2026-04-04T19:34:33.742Z'
---

# TASK-080: Generate session lifecycle + journal nudge hooks

Wire `loaf session` into hook configs and add journal nudge hooks.

## Scope

### Session lifecycle hooks
Generate in hook configs for all applicable targets:
- **SessionStart:** `loaf session start` (Claude Code, Cursor, OpenCode)
- **Stop:** `loaf session end` (Claude Code, Cursor)
- **PostToolUse on `git commit`:** `loaf session log --from-hook` (auto-log commit SHA)
- **PostToolUse on `gh pr create`:** `loaf session log --from-hook` (auto-log PR number)
- **PostToolUse on `gh pr merge`:** `loaf session log --from-hook` (auto-log merge)

### Journal nudge hooks (prompt type)
- **Stop hook:** If edits/commits happened this turn but zero `decide`/`discover`/`conclude` entries written, nudge agent to journal. Points to orchestration skill for entry vocabulary.
- **Post-commit hook:** After auto-logging `commit(SHA)`, nudge for decision rationale.
- **PreCompact hook:** Before context compaction, nudge for journal flush.

### Hook definitions
Add new session and nudge hook definitions to `config/hooks.yaml`.

## Constraints

- Session hooks use `loaf session` directly — no bash wrappers
- Nudge hooks are prompt type (text injection), not command type
- Claude Code binary path: `"${CLAUDE_PLUGIN_ROOT}/bin/loaf" session start`
- Cursor/Codex: PATH-based `loaf session start`

## Verification

- [ ] Generated hook configs contain `loaf session start/end` commands
- [ ] Git-related hooks generate `loaf session log --from-hook` commands
- [ ] Stop nudge hook appears in generated output
- [ ] Post-commit nudge appears in generated output
- [ ] PreCompact nudge appears in generated output
- [ ] `loaf build` succeeds for all targets
