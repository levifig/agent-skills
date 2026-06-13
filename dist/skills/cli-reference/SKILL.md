---
name: cli-reference
description: >-
  Documents the Loaf CLI commands and when to use them. Reference for
  /implement, /implement, and all loaf subcommands. Use when you
  need to know which CLI command to invoke. Not for skill documentation (use the
  skill's own SKILL.md) or for understanding build internals.
---

# Loaf CLI Reference

Quick reference for all Loaf CLI commands. Each command includes its purpose, common usage patterns, and when to use it.

**Note:** This file is auto-generated from native CLI reference metadata. Do not edit manually.

## Global Commands

### /implement
Orchestrates implementation sessions through agent delegation and batch execution.

**Use when:**
- User asks "implement this" or "start working on TASK-XXX"
- Starting a new spec implementation
- Resuming work after context loss

**Usage:**
- /implement TASK-XXX - Load task, auto-create session
- /implement SPEC-XXX - Resolve all tasks, build dependency waves
- /implement TASK-XXX..YYY - Expand range, build waves
- /implement "description" - Ad-hoc session

### /implement
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

## State Management

### `loaf state`
Manage native SQLite state

Existing TypeScript-era projects can keep running supported commands in
markdown-only compatibility mode until SQLite is initialized. Use
`loaf state migrate markdown --apply` to import `.agents/` Markdown into SQLite
without rewriting the source Markdown files.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf state path` | Print the resolved SQLite database path |
| `loaf state status` | Show SQLite readiness and markdown-only compatibility status |
| `loaf state init` | Initialize an empty SQLite state database |
| `loaf state doctor` | Diagnose SQLite state health |
| `loaf state migrate markdown` | Import existing .agents Markdown artifacts into SQLite |
| `loaf state migrate storage-home` | Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME |
| `loaf state backup` | Create a SQLite database backup |
| `loaf state export` | Export SQLite state for review or migration |

**Options:**

- `loaf state status`:
  - `--json` - Output status as JSON

- `loaf state init`:
  - `--json` - Output initialized status as JSON

- `loaf state doctor`:
  - `--fix` - Initialize missing SQLite state when safe
  - `--json` - Output diagnostics as JSON

- `loaf state migrate markdown`:
  - `--dry-run` - Preview import counts without creating a database
  - `--apply` - Initialize SQLite and import Markdown artifacts
  - `--resume` - Resume the Markdown import after an interrupted attempt
  - `--json` - Output migration details as JSON

- `loaf state migrate storage-home`:
  - `--dry-run` - Preview the storage-home migration
  - `--apply` - Copy the legacy database without deleting it
  - `--json` - Output migration details as JSON

- `loaf state backup`:
  - `--json` - Output backup details as JSON

**Usage:**
```bash
loaf state status
loaf state migrate markdown --dry-run
loaf state migrate markdown --apply
loaf state status
```

---

## Migrate Management

### `loaf migrate`
Run native migration workflows

`loaf migrate markdown` is the upgrade path for existing `.agents/`
projects with no SQLite database. Start with `--dry-run`, then use `--apply`
when the artifact counts and skipped files look right.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf migrate markdown` | Import existing .agents Markdown artifacts into SQLite |
| `loaf migrate storage-home` | Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME |
| `loaf migrate worktree-storage` | Move linked-worktree .agents state to the main worktree |

**Options:**

- `loaf migrate markdown`:
  - `--dry-run` - Preview import counts without creating a database
  - `--apply` - Initialize SQLite and import Markdown artifacts
  - `--resume` - Resume the Markdown import after an interrupted attempt
  - `--json` - Output migration details as JSON

- `loaf migrate storage-home`:
  - `--dry-run` - Preview the storage-home migration
  - `--apply` - Copy the legacy database without deleting it
  - `--json` - Output migration details as JSON

**Usage:**
```bash
loaf migrate markdown --dry-run
loaf migrate markdown --apply
loaf migrate storage-home --dry-run
```

---

## Task Management

### `loaf task`
Manage project tasks

In SQLite-backed projects, task metadata mutations go through the Go-native
state store. Markdown task files and `TASKS.json` remain compatibility/source
artifacts during migration; do not edit them directly for lifecycle changes.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf task list` | Show task board grouped by status |
| `loaf task show` | Display a single task's details |
| `loaf task status` | Show task summary counts |
| `loaf task create` | Create a new task |
| `loaf task update` | Update a task's metadata |
| `loaf task archive` | Archive completed tasks through the task lifecycle |
| `loaf task refresh` | Compatibility: rebuild the Markdown task index from task/spec files |
| `loaf task sync` | Compatibility: sync the Markdown task index and task files |

**Options:**

- `loaf task list`:
  - `--json` - Output raw JSON
  - `--active` - Hide completed tasks
  - `--status <status>` - Only show tasks with status: in_progress, blocked, todo, review, done

- `loaf task show`:
  - `--json` - Output task entry as JSON

- `loaf task create`:
  - `--title <title>` - Task title
  - `--spec <id>` - Associated spec ID (e.g., SPEC-010)
  - `--priority <level>` - Priority level (P0/P1/P2/P3)
  - `--depends-on <ids>` - Comma-separated task IDs

- `loaf task update`:
  - `--status <status>` - New status: todo, in_progress, blocked, review, done
  - `--priority <level>` - New priority: P0, P1, P2, P3
  - `--depends-on <ids>` - Replace depends_on (comma-separated task IDs)
  - `--session <file>` - Set or clear session reference (use "none" to clear)
  - `--spec <id>` - Set or change associated spec

- `loaf task archive`:
  - `--spec <id>` - Archive all done tasks for a spec

- `loaf task sync`:
  - `--import` - Import orphan .md files not in the index
  - `--push` - Push compatibility index metadata into .md frontmatter

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

Spec lifecycle changes go through `loaf spec` commands. Markdown spec files
remain the authored prose artifact, while SQLite state carries operational
status and relationship data when initialized.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf spec list` | Show specs with status and task counts |
| `loaf spec show` | Show spec details |
| `loaf spec archive` | Archive a completed spec |

**Options:**

- `loaf spec list`:
  - `--json` - Output raw JSON

- `loaf spec show`:
  - `--json` - Output raw JSON

- `loaf spec archive`:
  - `--json` - Output raw JSON

**Usage:**
```bash
loaf spec list
loaf spec show
loaf spec archive
```

---

## Report Management

### `loaf report`
Manage durable reports (research, audits, investigations)

In SQLite-backed projects, report lifecycle state is stored in SQLite. Use
generated report commands for review output; create authored Markdown reports
only when a durable prose artifact is explicitly needed.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf report list` | List reports |
| `loaf report generate` | Generate a report from state |
| `loaf report create` | Create a report draft |
| `loaf report finalize` | Mark a report draft as final |
| `loaf report archive` | Archive a finalized report |

**Options:**

- `loaf report list`:
  - `--type <type>` - Filter by report type
  - `--status <status>` - Filter by status
  - `--json` - Output as JSON

- `loaf report generate`:
  - `--format <format>` - Output format

- `loaf report create`:
  - `--type <type>` - Report type
  - `--source <source>` - Report source
  - `--json` - Output as JSON

- `loaf report finalize`:
  - `--json` - Output as JSON

- `loaf report archive`:
  - `--json` - Output as JSON

**Usage:**
```bash
loaf report list
loaf report create release-readiness --type audit --source manual
loaf report finalize report-release-readiness
loaf report archive report-release-readiness
loaf report generate release-readiness
```

---

## Kb Management

### `loaf kb`
Knowledge base management

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf kb glossary` | Domain glossary mutation and lookup |
| `loaf kb validate` | Validate knowledge file frontmatter |
| `loaf kb status` | Show knowledge base overview |
| `loaf kb check` | Check knowledge file staleness against git history |
| `loaf kb review` | Mark a knowledge file as reviewed today |
| `loaf kb init` | Initialize knowledge base directories and QMD collections |
| `loaf kb import` | Import external project knowledge via QMD collection |

**Options:**

- `loaf kb validate`:
  - `--json` - Output results as JSON

- `loaf kb status`:
  - `--json` - Output status as JSON

- `loaf kb check`:
  - `--file <path>` - Reverse lookup: find knowledge files covering this path
  - `--json` - Output results as JSON

- `loaf kb review`:
  - `--json` - Output updated frontmatter as JSON

- `loaf kb init`:
  - `--json` - Output results as JSON

- `loaf kb import`:
  - `--path <path>` - Path to the external project's knowledge directory
  - `--json` - Output results as JSON

**Usage:**
```bash
loaf kb glossary
loaf kb validate
loaf kb status
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
Scan project artifacts and recommend housekeeping actions

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

## Command Substitution Reference

The following placeholders are substituted at build time per target:

| Placeholder | Claude Code | OpenCode | Cursor |
|-------------|-------------|----------|--------|
| `/implement` | `/implement` | `/implement` | `@loaf/implement` |
| `/implement` | `/implement` | `/implement` | `@loaf/implement` |

---

## Quick Decision Guide

**Need to start working?** -> `/implement TASK-XXX`

**Need to continue after restart?** -> `loaf session start` then `/implement`

**Need to coordinate agents?** -> `/implement`

**Made changes to skills?** -> `loaf build && loaf install --to <target>`

**Want to see what's in progress?** -> `loaf task list --active`

**Ready to archive completed work?** -> `loaf task archive TASK-XXX`

**Need to check knowledge freshness?** -> `loaf kb check`
