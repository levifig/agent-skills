---
id: SPEC-023
title: CLI Backend Abstraction — Linear/Local Task Routing
source: >-
  direct — branch-local draft from feat/decouple-mcps promoting Loaf task
  operations behind a backend router
created: '2026-04-03T15:30:00Z'
status: drafting
---

# SPEC-023: CLI Backend Abstraction — Linear/Local Task Routing

## Problem Statement

Loaf skills reference Linear MCP tools directly (~80 references across 12+ skill files). This creates three problems:

1. **Skills aren't backend-agnostic.** A project without Linear gets a degraded experience — instructions say "create Linear issue" with no alternative path, or say "(if configured)" which the LLM may ignore.
2. **LLM compliance is unreliable.** Telling an agent "check config before calling Linear" works ~90% of the time. Not enough for a framework that bills itself as opinionated.
3. **Direct MCP calls bypass the CLI.** The `loaf task` CLI already handles local task management. Linear operations should flow through the same interface — same commands, different backend.

## Architectural Principle

This spec codifies a pattern that Loaf has been evolving toward:

| Layer | Role | Example |
|-------|------|---------|
| **Skills** | Behavior — what to do, when, why | "Create a task for each acceptance criterion" |
| **CLI** | Protocol — how to do it, deterministically | `loaf task create --title "..." --spec SPEC-XXX` |
| **Hooks** | Enforcement — gates that block bad actions | `loaf check --hook validate-commit` |

Skills never call external APIs directly. The CLI is the protocol layer that routes to the appropriate backend. Hooks enforce invariants. This separation means:

- Skills are **target-agnostic** (work on all 6 harnesses)
- Backends are **swappable** (toggle in config, no skill rewrite)
- Enforcement is **deterministic** (scripts, not LLM judgment)

## Solution

### 1. Backend Router in `loaf task`

`loaf task *` commands gain a backend router. Config determines which backend handles each operation:

```json
// .agents/loaf.json
{
  "integrations": {
    "linear": {
      "enabled": true,
      "workspace": "your-workspace",
      "default_team": "Platform"
    }
  }
}
```

When `linear.enabled: true` AND Linear MCP is reachable, `loaf task` routes to Linear. Otherwise, routes to local files. The CLI handles the fallback — skills don't need to know.

**Key design choice:** The CLI calls Linear MCP tools as a subprocess/client, not by importing Linear SDK. This keeps the dependency optional and runtime-only.

### 2. Command Mapping

Every operation skills currently do via direct Linear MCP calls gets a CLI equivalent:

| Skill currently says | Spec says instead | CLI routes to |
|---|---|---|
| "Create Linear issue" | `loaf task create --title "..." --priority P1` | Linear `save_issue` or local `.md` file |
| "Update Linear status" | `loaf task update TASK-XXX --status in-progress` | Linear `save_issue` or local TASKS.json |
| "Get Linear issue" | `loaf task show TASK-XXX` | Linear `get_issue` or local `.md` read |
| "List project issues" | `loaf task list` | Linear `list_issues` or local TASKS.json |
| "Get branch name from Linear" | `loaf task branch TASK-XXX` | Linear `get_issue` → `branchName` or local slug |
| "Suggest team" | `loaf task assign --suggest "description"` | Config-based team keywords (already in suggest-team.py) |

### 3. Skill Updates

Skills drop all direct Linear MCP references. Replace with `loaf task *` commands:

**Before:**
```markdown
### Task Creation
**Linear backend:** Create issues with title, description, labels, priority.
**Local backend:** Use `loaf task create --spec SPEC-XXX --title "..."`.
```

**After:**
```markdown
### Task Creation
Use `loaf task create --spec SPEC-XXX --title "..." --priority P1`.
```

One instruction. One path. The CLI handles the rest.

### 4. Hook Updates

Hooks that reference Linear directly (`detect-linear-magic.py`, `suggest-team.py`, etc.) become `loaf` subcommands in TypeScript:

| Current | New |
|---|---|
| `python3 orchestration-detect-linear-magic.py` | `loaf check --hook detect-magic-words` |
| `python3 suggest-team.py "description"` | `loaf task assign --suggest "description"` |
| `python3 get-config.py linear.workspace` | `loaf config get integrations.linear.workspace` |
| `bash extract-magic-words.sh HEAD~10..HEAD` | `loaf task refs HEAD~10..HEAD` |

This also completes the Python → TypeScript migration for all scripts in the task/Linear domain.

### 5. Config Command

New `loaf config` subcommand for reading/writing `.agents/loaf.json`:

```bash
loaf config get integrations.linear.enabled    # → true
loaf config set integrations.linear.enabled false
loaf config get integrations.linear             # → full section as JSON
```

`loaf install` writes initial config. Users can toggle at any time — takes effect on the next `loaf task` invocation.

## Rabbit Holes

- **Two-way sync (Linear ↔ local).** Out of scope. Pick one backend per project. Don't try to mirror.
- **Linear MCP availability detection.** The CLI should fail gracefully if Linear MCP isn't running even when `enabled: true`. Fall back to local with a warning, don't crash.
- **GitHub Issues backend.** Future work. The abstraction supports it, but this spec only implements Linear and local.
- **Offline mode.** If Linear is configured but unreachable, queue operations locally and sync later — too complex. Just fall back to local with a warning.

## Non-Goals

- Replacing Linear's UI or duplicating its full feature set
- Supporting arbitrary issue trackers (just Linear and local for now)
- Build-time conditional compilation of skills (runtime routing instead)

### 6. Complete Script Migration (Python + Bash → TypeScript)

All 38 non-TS scripts (13 hook scripts, 25 skill scripts) migrate to TypeScript:

- **Protocol operations** (task ops, config reads, validation) → `loaf` subcommands
- **Utility functions** (formatting, extraction, simple checks) → bundled JS modules invoked via `node`

After migration, Loaf has **zero Python or Bash runtime dependencies**. A project using Loaf only needs Node.js.

| Category | Count | Migration target |
|---|---|---|
| Hook scripts (bash) | 10 | `loaf check`/`loaf session` subcommands |
| Hook scripts (Python) | 3 | `loaf check` subcommands |
| Skill scripts (bash) | 12 | `loaf` subcommands or bundled JS modules |
| Skill scripts (Python) | 13 | `loaf` subcommands or bundled JS modules |

## Appetite

**Large.** The `loaf task` CLI already exists with 8 subcommands for local. The work is:
1. Add backend router + Linear backend to existing commands
2. Port all 38 Python/bash scripts to TS (`loaf` subcommands or bundled modules)
3. Update ~12 skill files (mechanical: replace "Linear MCP" references with `loaf task` commands)
4. Add `loaf config` subcommand
5. Add `loaf task branch` and `loaf task refs` subcommands
6. Remove Python and Bash runtime dependencies

## Verification

- [ ] `loaf task create` routes to Linear when enabled, local when disabled
- [ ] Toggling `integrations.linear.enabled` takes effect immediately (no rebuild)
- [ ] No skill file contains direct Linear MCP tool references
- [ ] All Python/bash scripts in orchestration are replaced with TS `loaf` subcommands
- [ ] `loaf task list` output is identical format regardless of backend
- [ ] Graceful fallback when Linear MCP is unreachable
- [ ] Existing tests pass + new backend routing tests
