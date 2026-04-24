---
name: cli-reference
description: >-
  Documents the Loaf CLI commands and when to use them. Reference for
  /loaf:implement, /loaf:implement, and all loaf subcommands. Use when you need to know
  which CLI command to invoke. Not for skill documentation (use the skill's own
  SKILL.md) or for understa...
user-invocable: false
version: 2.0.0-dev.30
---

# Loaf CLI Reference

Quick reference for all Loaf CLI commands. Each command includes its purpose, common usage patterns, and when to use it.

## Contents
- Critical Rules
- Verification
- Quick Decision Guide
- Global Commands
- Build Commands
- Task Management
- Spec Management
- Knowledge Base
- Session Management
- Project Setup
- Utility Commands
- Command Substitution Reference

## Critical Rules

- **Always run `loaf build`** after modifying skills, agents, or hooks before installing
- **Always run `loaf install`** after building to propagate changes to target tools
- **Use `loaf task refresh`** after manually editing task files to keep the TASKS.json index in sync
- **Never skip `loaf check`** before committing -- it runs enforcement hooks (secrets scanning, linting)

## Verification

- `loaf build` exits cleanly with no errors for all targets
- `loaf check` passes all enforcement hooks before committing
- `loaf task list` reflects the expected state after task updates

## Quick Decision Guide

**Need to start working?** → `/loaf:implement TASK-XXX`

**Need to continue after restart?** → `loaf session start` then `/loaf:implement`

**Need to coordinate agents?** → `/loaf:implement`

**Made changes to skills?** → `loaf build && loaf install --to <target>`

**Want to see what's in progress?** → `loaf task list --active`

**Ready to archive completed work?** → `loaf task archive TASK-XXX`

**Need to check knowledge freshness?** → `loaf kb check`

---

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

## Build Commands

### `loaf build`
Builds all distribution targets from content source.

**Use when:**
- After modifying skills, agents, or hooks
- Before installing to tools
- Testing changes locally

**Usage:**
```bash
loaf build                      # Build all targets
loaf build --target claude-code # Specific target only
```

**Targets:** claude-code, opencode, cursor, codex, gemini

### `loaf install`
Installs Loaf distribution to detected AI tools.

**Use when:**
- First-time setup
- Updating existing installations
- Installing to new tools

**Usage:**
```bash
loaf install                   # Interactive install to detected tools
loaf install --to all          # Install to all detected tools
loaf install --to cursor       # Install to specific target
loaf install --upgrade         # Update only already-installed
```

---

## Task Management

### `loaf task`
Manages project tasks from TASKS.json index.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf task list` | Show task board grouped by status |
| `loaf task show TASK-XXX` | Display single task details |
| `loaf task status` | Show task statistics and overview |
| `loaf task create --id TASK-XXX` | Create new task file |
| `loaf task update TASK-XXX` | Update task status, priority, or session |
| `loaf task archive TASK-XXX` | Archive completed task |
| `loaf task refresh` | Regenerate TASKS.json index from files |
| `loaf task sync` | Sync frontmatter with TASKS.json index |

**Usage:**
```bash
loaf task list --active        # Hide completed
loaf task update TASK-075 --status in_progress
loaf task update TASK-075 --session 20250331-120000-cli-ref.md
```

---

## Spec Management

### `loaf spec`
Manages specification lifecycle and task relationships.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf spec list` | Show all specs with status |
| `loaf spec archive SPEC-XXX` | Archive completed spec |

**Usage:**
```bash
loaf spec list
loaf spec archive SPEC-020
```

---

## Knowledge Base

### `loaf kb`
Manages project knowledge files.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf kb validate` | Validate knowledge file frontmatter |
| `loaf kb status` | Show knowledge base overview |
| `loaf kb check` | Check for stale knowledge |
| `loaf kb review <file>` | Review specific knowledge file |
| `loaf kb init` | Initialize knowledge base for project |
| `loaf kb import <url>` | Import knowledge from external source |

**Usage:**
```bash
loaf kb validate
loaf kb status
loaf kb check
loaf kb review docs/knowledge/hooks.md
```

---

## Session Management

### `loaf session`
Manages session journals.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf session start` | Start/resume session for current branch |
| `loaf session end` | End session with progress summary |
| `loaf session log [entry]` | Log entry to session journal |
| `loaf session archive` | Archive completed session |

**Usage:**
```bash
loaf session start              # Start or resume session for current branch
loaf session log "decide(scope): description"
loaf session end                # Pause session with summary
loaf session archive            # Archive session
```

---

## Project Setup

### `loaf init`
Initializes a new Loaf project structure.

**Use when:**
- Starting a new project with Loaf
- Setting up .agents/ directory structure

**Usage:**
```bash
loaf init
```

**Creates:**
- `.agents/` directory
- `TASKS.json` index
- Default configuration

### `loaf setup`
Sets up Loaf development environment.

**Use when:**
- Setting up for Loaf framework development
- Installing pre-commit hooks

**Usage:**
```bash
loaf setup              # Interactive setup
loaf setup --hooks      # Install git hooks only
```

---

## Utility Commands

### `loaf housekeeping`
Reviews and archives agent artifacts.

**Use when:**
- Reviewing completed sessions
- Maintaining .agents/ directory
- Preparing for reflection

**Usage:**
```bash
loaf housekeeping            # Interactive review
loaf housekeeping --dry-run  # Show what would be archived
```

### `loaf release` / `loaf ship`
Orchestrates release ritual.

**Use when:**
- Preparing a release
- Running pre-flight checks
- Creating changelog

**Usage:**
```bash
loaf release            # Interactive release
loaf ship --dry-run     # Test release flow
```

### `loaf version`
Shows version and target info.

**Usage:**
```bash
loaf version            # Show CLI version
loaf version --targets  # Show available targets
```

---

## Command Substitution Reference

The following placeholders are substituted at build time per target:

| Placeholder | Claude Code | OpenCode | Cursor |
|-------------|-------------|----------|--------|
| `/loaf:implement` | `/loaf:implement` | `/loaf:implement` | `@loaf/loaf:implement` |
| `/loaf:implement` | `/loaf:implement` | `/loaf:implement` | `@loaf/loaf:implement` |
