---
name: cli-reference
description: >-
  Documents the Loaf CLI commands and when to use them. Reference for
  /loaf:implement, /loaf:implement, and all loaf subcommands. Use when you need to know
  which CLI command to invoke. Not for skill documentation (use the skill's own
  SKILL.md) or for understa...
user-invocable: false
version: 2.0.0-dev.35
---

# Loaf CLI Reference

Quick reference for all Loaf CLI commands. Each command includes its purpose, common usage patterns, and when to use it.

**Note:** This file is auto-generated from the CLI source code. Do not edit manually.

## Global Commands

### /loaf:implement
Orchestrates implementation sessions through agent delegation and batch execution.

**Use when:**
- User asks "implement this" or "start working on TASK-XXX"
- Starting a new spec implementation
- Resuming work after context loss

**Usage:**
- /loaf:implement TASK-XXX — Load task, auto-create session
- /loaf:implement SPEC-XXX — Resolve all tasks, build dependency waves
- /loaf:implement TASK-XXX..YYY — Expand range, build waves
- /loaf:implement "description" — Ad-hoc session

### /loaf:implement
Coordinates multi-agent work: agent delegation, session management, Linear integration.

**Use when:**
- Managing sessions and delegating to agents
- Running council workflows
- Coordinating cross-cutting work

---

## Build Management

### `loaf build`
Build skill distributions for agent harnesses

**Usage:**
```bash
loaf build
```

---

## Install Management

### `loaf install`
Install Loaf to detected AI tool configurations

**Usage:**
```bash
loaf install
```

---

## Init Management

### `loaf init`
Initialize a project with Loaf structure

**Usage:**
```bash
loaf init
```

---

## Release Management

### `loaf release`
Create a new release with changelog, version bump, and tag

**Usage:**
```bash
loaf release
```

---

## Task Management

### `loaf task`
Manage project tasks

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf task list` | Show task board grouped by status |
| `loaf task show` | Display a single task's details |
| `loaf task status` | Show task summary counts |
| `loaf task create` | Create a new task |
| `loaf task update` | Update a task's metadata |
| `loaf task archive` | Move completed tasks to archive and update TASKS.json |
| `loaf task refresh` | Rebuild TASKS.json from task and spec files |
| `loaf task sync` | Sync between TASKS.json and .md files |

**Options:**

- `loaf task list`:
  - `--json` — Output raw JSON
  - `--active` — Hide completed tasks

- `loaf task show`:
  - `--json` — Output task entry as JSON

- `loaf task create`:
  - `--title <title>` — Task title
  - `--spec <id>` — Associated spec ID (e.g., SPEC-010)
  - `--priority <level>` — Priority level (P0/P1/P2/P3)
  - `--depends-on <ids>` — Comma-separated task IDs

- `loaf task update`:
  - `--status <status>` — New status: todo, in_progress, blocked, review, done
  - `--priority <level>` — New priority: P0, P1, P2, P3
  - `--depends-on <ids>` — Replace depends_on (comma-separated task IDs)
  - `--session <file>` — Set or clear session reference (use "none" to clear)
  - `--spec <id>` — Set or change associated spec

- `loaf task archive`:
  - `--spec <id>` — Archive all done tasks for a spec

- `loaf task sync`:
  - `--import` — Import orphan .md files not in the index
  - `--push` — Push TASKS.json metadata into .md frontmatter

**Usage:**
```bash
loaf task list
loaf task show
loaf task status
```

---

## Spec Management

### `loaf spec`
Manage project specs

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf spec list` | Show specs with status and task counts |
| `loaf spec archive` | Move completed specs to archive and update TASKS.json |

**Options:**

- `loaf spec list`:
  - `--json` — Output raw JSON

**Usage:**
```bash
loaf spec list
loaf spec archive
```

---

## Kb Management

### `loaf kb`
Knowledge base management

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf kb validate` | Validate knowledge file frontmatter |
| `loaf kb status` | Show knowledge base overview |
| `loaf kb check` | Check knowledge file staleness against git history |
| `loaf kb review` | Mark a knowledge file as reviewed today |
| `loaf kb init` | Initialize knowledge base directories and QMD collections |
| `loaf kb import` | Import external project knowledge via QMD collection |

**Options:**

- `loaf kb validate`:
  - `--json` — Output results as JSON

- `loaf kb status`:
  - `--json` — Output status as JSON

- `loaf kb check`:
  - `--file <path>` — Reverse lookup: find knowledge files covering this path
  - `--json` — Output results as JSON

- `loaf kb review`:
  - `--json` — Output updated frontmatter as JSON

- `loaf kb init`:
  - `--json` — Output results as JSON

- `loaf kb import`:
  - `--path <path>` — Path to the external project's knowledge directory
  - `--json` — Output results as JSON

**Usage:**
```bash
loaf kb validate
loaf kb status
loaf kb check
```

---

## Setup Management

### `loaf setup`
One-step bootstrap: init + build + install

**Usage:**
```bash
loaf setup
```

---

## Version Management

### `loaf version`
Show version info and project statistics

**Usage:**
```bash
loaf version
```

---

## Housekeeping Management

### `loaf housekeeping`
Scan .agents/ artifacts and recommend housekeeping actions

**Usage:**
```bash
loaf housekeeping
```

---

## Check Management

### `loaf check`
Run enforcement hook checks

**Usage:**
```bash
loaf check
```

---

## Session Management

### `loaf session`
Manage session journals

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf session start` | Start/resume session for current branch |
| `loaf session end` | End session with progress summary |
| `loaf session log` | Log entry to session journal |
| `loaf session archive` | Archive completed session |
| `loaf session housekeeping` | Run session housekeeping: orphans, splits, archival, linkage repair |
| `loaf session enrich` | Enrich session journal from JSONL conversation log |
| `loaf session list` | List all active and archived sessions |
| `loaf session state` | Manage session state snapshot |
| `loaf session context` | Session context for hooks and agents |

**Options:**

- `loaf session start`:
  - `--resume` — Resume existing paused session instead of creating new
  - `--force` — Force session creation, bypassing subagent detection

- `loaf session end`:
  - `--if-active` — Exit successfully when no active session exists
  - `--wrap` — Close session as done (used after /loaf:wrap writes summary)
  - `--from-hook` — Invoked from a Stop hook — keep inline chain, silent on no-match
  - `--session-id <id>` — Route to session with this claude_session_id (Tier 1 override)

- `loaf session log`:
  - `--from-hook` — Parse entry from hook stdin
  - `--session-id <id>` — Route to session with this claude_session_id (Tier 1 override)
  - `--detect-linear` — Detect Linear magic words in recent commits

- `loaf session archive`:
  - `--branch <branch>` — Archive session for specific branch (default: current)
  - `--session-id <id>` — Route to session with this claude_session_id (Tier 1 override)

- `loaf session housekeeping`:
  - `--dry-run` — Report what would be done without making changes

- `loaf session enrich`:
  - `--dry-run` — Show what would be added without writing
  - `--model <model>` — Override model for the librarian call
  - `--session-id <id>` — Route to session with this claude_session_id (Tier 1 override)

- `loaf session list`:
  - `--all` — Include archived sessions

**Usage:**
```bash
loaf session start
loaf session end
loaf session log
```

---

## Command Substitution Reference

The following placeholders are substituted at build time per target:

| Placeholder | Claude Code | OpenCode | Cursor |
|-------------|-------------|----------|--------|
| `/loaf:implement` | `/loaf:implement` | `/loaf:implement` | `@loaf/loaf:implement` |
| `/loaf:implement` | `/loaf:implement` | `/loaf:implement` | `@loaf/loaf:implement` |

---

## Quick Decision Guide

**Need to start working?** → `/loaf:implement TASK-XXX`

**Need to continue after restart?** → `loaf session start` then `/loaf:implement`

**Need to coordinate agents?** → `/loaf:implement`

**Made changes to skills?** → `loaf build && loaf install --to <target>`

**Want to see what's in progress?** → `loaf task list --active`

**Ready to archive completed work?** → `loaf task archive TASK-XXX`

**Need to check knowledge freshness?** → `loaf kb check`
