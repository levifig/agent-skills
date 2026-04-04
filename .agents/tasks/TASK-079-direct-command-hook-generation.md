---
id: TASK-079
title: Generate direct loaf check commands in hook configs
spec: SPEC-020
status: todo
priority: P0
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
---

# TASK-079: Generate direct `loaf check` commands in hook configs

Rewrite hook generation to emit direct `loaf check` commands instead of bash wrapper script references.

## Scope

**Claude Code (`plugin.json`):** Replace bash script references with:
```jsonc
"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook <id>"
```

**Cursor (`hooks.json`):** Replace with PATH-based:
```jsonc
"command": "loaf check --hook <id>"
```

**Codex (new):** Generate `dist/codex/.codex/hooks.json` with Bash-matching enforcement hooks only:
```jsonc
"command": "loaf check --hook <id>"
```
Only 5 hooks: `check-secrets`, `validate-push`, `validate-commit`, `workflow-pre-pr`, `security-audit`. No Edit/Write matchers (Codex can't intercept them).

**Remove migrated hooks from `hooks.yaml`:** The ~20 hooks that became skill instructions (TASK-073) are removed from `hooks.yaml`. Remaining hooks: 5 enforcement, 1 prompt, 1 advisory, 3-4 side-effect, 6 session lifecycle.

## Constraints

- Per-target binary path: `${CLAUDE_PLUGIN_ROOT}/bin/loaf` (CC), PATH-based (Cursor/Codex)
- Per-target event names: `PreToolUse` (CC/Codex) vs `preToolUse` (Cursor)
- Per-target matcher names: Codex only supports `Bash`
- `failClosed` emitted per TASK-078
- No bash wrapper references in any generated config

## Verification

- [ ] Claude Code `plugin.json` has direct `loaf check` commands
- [ ] Cursor `hooks.json` has direct `loaf check` commands
- [ ] Codex `dist/codex/.codex/hooks.json` exists with Bash-only hooks
- [ ] ~20 migrated hooks removed from `hooks.yaml`
- [ ] No bash wrapper references in any generated config
- [ ] `loaf build` succeeds for all targets
