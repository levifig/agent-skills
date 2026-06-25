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

**Options:**

- `-t, --target <name>` - Build a specific target only

**Usage:**
```bash
loaf build
```

---

## Install Management

### `loaf install`
Install Loaf to detected AI tool configurations

**Options:**

- `--to <target>` - Target to install to (or "all")
- `--upgrade` - Update only already-installed targets
- `-y, --yes` - Assume 'yes' to safe migrations (merge content, back up, and replace real files with symlinks)
- `--no-yes` - Force interactive prompts even when stdin is not a TTY (testing)

**Usage:**
```bash
loaf install
```

---

## Init Management

### `loaf init`
Initialize a project with Loaf structure

**Options:**

- `--no-symlinks` - Skip symlink creation prompts

**Usage:**
```bash
loaf init
```

---

## Release Management

### `loaf release`
Create a new release with changelog, version bump, and tag

**Options:**

- `--dry-run` - Preview release without making changes
- `--bump <type>` - Skip interactive bump choice (prerelease, release, major, minor, patch)
- `--base <ref>` - Use commits since <ref> instead of last tag (e.g. main)
- `--tag` - Force git tag creation (overrides --pre-merge default)
- `--no-tag` - Skip git tag creation
- `--gh` - Force GitHub release draft (overrides --pre-merge default)
- `--no-gh` - Skip GitHub release draft
- `--pre-merge` - Shortcut for --no-tag --no-gh --base <auto-detected>
- `--post-merge` - Finalize release after squash-merge
- `--version-file <path>` - Override version file path (repeatable). Replaces configured version files and root auto-detection.
- `-y, --yes` - Skip confirmation prompt

**Usage:**
```bash
loaf release
```

---

## Search Management

### `loaf search`
Search Tier-1 SQLite artifact bodies and journal entries

**Options:**

- `<query>` - Search terms matched through SQLite FTS5
- `--all-projects` - Search every registered project instead of only the current project
- `--limit <n>` - Maximum results to return (default: 20)
- `--json` - Output tiered hits, stable entity addresses, snippets, global database scope, and project identity as JSON

**Usage:**
```bash
loaf search
```

---

## State Management

### `loaf state`
Manage native SQLite state

Existing TypeScript-era projects can keep running supported commands in
markdown-only compatibility mode until SQLite is initialized. Use
`loaf state migrate markdown --apply` to import `.agents/` Markdown into SQLite
without rewriting the source Markdown files.

Manual restore from a backup is explicit until a guarded restore command exists:
verify the backup with `loaf state backup verify <backup>`, preserve the current
`$(loaf state path)` file, copy the verified backup to that path, then run
`loaf state doctor` and `loaf state status`.
For agents, `loaf state backup verify <backup> --json` also returns
`restore_database_path`, `restore_preserve_path`, and
`restore_validation_commands` for the current checkout.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf state path` | Print the resolved SQLite database path |
| `loaf state status` | Show SQLite readiness and markdown-only compatibility status |
| `loaf state init` | Initialize an empty SQLite state database |
| `loaf state doctor` | Diagnose SQLite state health |
| `loaf state repair legacy-project-database` | Archive migrated per-project SQLite leftovers |
| `loaf state repair relationship-origin` | Preview or apply guarded relationship provenance backfills |
| `loaf state migrate markdown` | Import existing .agents Markdown artifacts into SQLite |
| `loaf state migrate storage-home` | Copy legacy XDG_STATE_HOME SQLite state into XDG_DATA_HOME |
| `loaf state backup` | Create a SQLite database backup under the global data-home backups directory |
| `loaf state backup verify` | Verify an existing SQLite database backup |
| `loaf state export` | Export SQLite state for review or migration |
| `loaf state export all` | Export a complete project-scoped SQLite snapshot |
| `loaf state export triage` | Export a triage summary from SQLite state |
| `loaf state export session` | Export one session from SQLite state |
| `loaf state export spec` | Export one spec from SQLite state |
| `loaf state export release-readiness` | Export a release-readiness report from SQLite state |

**Options:**

- `loaf state path`:
  - `--json` - Output contract version, database path, scope, and project root as JSON
  - `--verbose` - Output command, scope, project root, and database path

- `loaf state status`:
  - `--json` - Output readiness mode, diagnostics, global database scope, and project identity as JSON

- `loaf state init`:
  - `--json` - Output initialized status, global database scope, and project identity as JSON

- `loaf state doctor`:
  - `--fix` - Initialize missing SQLite state when safe
  - `--dry-run` - Show the repair plan without applying fixes
  - `--json` - Output diagnostics, repair plan, global database scope, and project identity as JSON

- `loaf state repair legacy-project-database`:
  - `--dry-run` - Preview archive paths without writing
  - `--apply` - Move legacy SQLite files into the archive directory
  - `--json` - Output archive plan/result, global database scope, and project identity as JSON

- `loaf state repair relationship-origin`:
  - `--origin <imported|manual>` - Provenance value to backfill
  - `--dry-run` - Preview affected rows without writing
  - `--apply` - Backfill missing origins after creating a SQLite backup
  - `--json` - Output repair plan/result, global database scope, and project identity as JSON

- `loaf state migrate markdown`:
  - `--dry-run` - Preview import counts without creating a database
  - `--apply` - Initialize SQLite and import Markdown artifacts
  - `--resume` - Resume the Markdown import after an interrupted attempt
  - `--json` - Output migration contract, scope, project context, and counts as JSON

- `loaf state migrate storage-home`:
  - `--dry-run` - Preview the storage-home migration
  - `--apply` - Copy the legacy database without deleting it
  - `--json` - Output migration contract, global database paths, action, and project identity when available

- `loaf state backup`:
  - `--json` - Output backup verification, checksum, schema version, project count, and current project identity as JSON

- `loaf state backup verify`:
  - `--json` - Output backup verification, restore guidance, schema version, and captured project identities as JSON

- `loaf state export`:
  - `--format <format>` - Output format for the selected export kind

- `loaf state export all`:
  - `--format <format>` - Output format: json
  - `--json` - Alias for --format json

- `loaf state export triage`:
  - `--format <format>` - Output format: markdown

- `loaf state export session`:
  - `--format <format>` - Output format: markdown

- `loaf state export spec`:
  - `--format <format>` - Output format: markdown

- `loaf state export release-readiness`:
  - `--format <format>` - Output format: markdown

**Usage:**
```bash
loaf state status
loaf state migrate markdown --dry-run
loaf state migrate markdown --apply
loaf state backup
loaf state backup verify /path/to/backup.sqlite
loaf state status
```

---

## Project Management

### `loaf project`
Manage durable project identity

Project IDs are stable SQLite identities, not path or name hashes. Use
`loaf project rename --dry-run` for display-name previews and
`loaf project move --dry-run` before recording checkout path moves.

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf project list` | List registered projects in the global SQLite database |
| `loaf project show` | Show the current project identity |
| `loaf project identity` | Alias for project show |
| `loaf project rename` | Rename the friendly project name |
| `loaf project move` | Record a checkout path move |

**Options:**

- `loaf project list`:
  - `--json` - Output database path, project IDs, friendly names, and current paths as JSON

- `loaf project show`:
  - `--json` - Output project ID, friendly name, current path, and database path as JSON

- `loaf project identity`:
  - `--json` - Output project ID, friendly name, current path, and database path as JSON

- `loaf project rename`:
  - `--dry-run` - Validate and preview without writing
  - `--json` - Output project ID, friendly name, current path, database path, and applied status as JSON

- `loaf project move`:
  - `<from> [to]` - Previous and optional new absolute project paths
  - `--from <path>` - Previous absolute project path
  - `--to <path>` - New absolute project path; defaults to the current project root
  - `--dry-run` - Validate and preview without writing
  - `--json` - Output project ID, friendly name, current path, database path, and applied status as JSON

**Usage:**
```bash
loaf project show
loaf project identity --json
loaf project rename "Loaf" --dry-run
loaf project rename "Loaf"
loaf project move /old/path/to/loaf /new/path/to/loaf --dry-run
loaf project move --from /old/path/to/loaf --dry-run
loaf project move --from /old/path/to/loaf
loaf project show --json
```

---

## Migrate Management

### `loaf migrate`
Run native migration workflows

`loaf migrate markdown` is the upgrade path for existing `.agents/`
projects with no SQLite database. Start with `--dry-run`, then use `--apply`
when the artifact counts and unimported file classifications look right.

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
  - `--json` - Output migration contract, scope, project context, and counts as JSON

- `loaf migrate storage-home`:
  - `--dry-run` - Preview the storage-home migration
  - `--apply` - Copy the legacy database without deleting it
  - `--json` - Output migration contract, global database paths, action, and project identity when available

- `loaf migrate worktree-storage`:
  - `--apply` - Perform the migration; dry-run is the default
  - `--force-from-worktree` - On conflict, keep the worktree-local copy
  - `--force-from-main` - On conflict, keep the main-worktree copy

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
  - `--json` - Output tasks, diagnostics, global database scope, and project identity as JSON
  - `--active` - Hide completed tasks
  - `--status <status>` - Only show tasks with status: in_progress, blocked, todo, review, done, archived

- `loaf task show`:
  - `--json` - Output task details, relationships, global database scope, and project identity as JSON

- `loaf task create`:
  - `--title <title>` - Task title
  - `--spec <id>` - Associated spec ID (e.g., SPEC-010)
  - `--priority <level>` - Priority level: P0, P1, P2, P3
  - `--depends-on <ids>` - Comma-separated task IDs
  - `--json` - Output created task, event, global database scope, and project identity as JSON

- `loaf task update`:
  - `--status <status>` - New status: in_progress, blocked, todo, review, done
  - `--priority <level>` - New priority: P0, P1, P2, P3
  - `--depends-on <ids>` - Replace depends_on (comma-separated task IDs)
  - `--session <file>` - Set or clear session reference (use "none" to clear)
  - `--spec <id>` - Set or change associated spec
  - `--json` - Output updated task, event, global database scope, and project identity as JSON

- `loaf task archive`:
  - `--spec <id>` - Archive all done tasks for a spec
  - `--json` - Output archive result, archived tasks, global database scope, and project identity as JSON

- `loaf task refresh`:
  - `--json` - Output compatibility summary as JSON

- `loaf task sync`:
  - `--import` - Import orphan .md files not in the index
  - `--push` - Push compatibility index metadata into .md frontmatter
  - `--json` - Output compatibility summary as JSON

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
  - `--json` - Output specs, diagnostics, task counts, global database scope, and project identity as JSON

- `loaf spec show`:
  - `--json` - Output spec details, task counts, relationships, global database scope, and project identity as JSON

- `loaf spec archive`:
  - `--json` - Output archive result, archived specs, global database scope, and project identity as JSON

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
| `loaf report show` | Show one report |
| `loaf report generate` | Generate a report from state |
| `loaf report create` | Create a report draft |
| `loaf report finalize` | Mark a report draft as final |
| `loaf report archive` | Archive a finalized report |

**Options:**

- `loaf report list`:
  - `--type <type>` - Filter by report type
  - `--status <status>` - Filter by status; Loaf lifecycle statuses: draft, final, archived
  - `--json` - Output reports, diagnostics, global database scope, and project identity as JSON

- `loaf report show`:
  - `--json` - Output report details, relationships, global database scope, and project identity as JSON

- `loaf report generate`:
  - `--format <format>` - Output format: markdown
  - `--json` - Output contract, command, project context, and markdown content as JSON

- `loaf report create`:
  - `--type <type>` - Report type
  - `--source <source>` - Report source
  - `--body-file <path>` - Read Markdown body from a UTF-8 file
  - `--body -` - Read Markdown body from stdin
  - `--message <text>` - Use inline Markdown body text
  - `--json` - Output created report, event, global database scope, and project identity as JSON

- `loaf report finalize`:
  - `--json` - Output report status transition, event, global database scope, and project identity as JSON

- `loaf report archive`:
  - `--json` - Output report status transition, event, global database scope, and project identity as JSON

**Usage:**
```bash
loaf report list
loaf report create release-readiness --type audit --source manual
loaf report finalize report-release-readiness
loaf report archive report-release-readiness
loaf report generate release-readiness
```

---

## Plan Management

### `loaf plan`
Manage plans in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf plan new` | Create a plan in SQLite state |
| `loaf plan show` | Show one plan from SQLite state |
| `loaf plan list` | List plans from SQLite state |
| `loaf plan link` | Link a plan to another entity |

**Options:**

- `loaf plan new`:
  - `--title <title>` - Artifact title
  - `--body-file <path>` - Read Markdown body from a UTF-8 file
  - `--body -` - Read Markdown body from stdin
  - `--message <text>` - Use inline Markdown body text
  - `--spec <spec>` - Optional related spec
  - `--json` - Output created artifact, event, global database scope, and project identity as JSON

- `loaf plan show`:
  - `--json` - Output artifact details, relationships, global database scope, and project identity as JSON

- `loaf plan list`:
  - `--all` - Include archived artifacts
  - `--status <status>` - Filter by status
  - `--json` - Output artifacts, global database scope, and project identity as JSON

- `loaf plan link`:
  - `--type <type>` - Relationship type; defaults to related_to
  - `--reason <text>` - Relationship reason
  - `--json` - Output relationship ID, source/target, global database scope, and project identity as JSON

**Usage:**
```bash
loaf plan new
loaf plan show
loaf plan list
```

---

## Handoff Management

### `loaf handoff`
Manage handoffs in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf handoff new` | Create a handoff in SQLite state |
| `loaf handoff show` | Show one handoff from SQLite state |
| `loaf handoff list` | List handoffs from SQLite state |
| `loaf handoff link` | Link a handoff to another entity |

**Options:**

- `loaf handoff new`:
  - `--title <title>` - Artifact title
  - `--body-file <path>` - Read Markdown body from a UTF-8 file
  - `--body -` - Read Markdown body from stdin
  - `--message <text>` - Use inline Markdown body text
  - `--session <session>` - Optional related session
  - `--task <task>` - Optional related task
  - `--json` - Output created artifact, event, global database scope, and project identity as JSON

- `loaf handoff show`:
  - `--json` - Output artifact details, relationships, global database scope, and project identity as JSON

- `loaf handoff list`:
  - `--all` - Include archived artifacts
  - `--status <status>` - Filter by status
  - `--json` - Output artifacts, global database scope, and project identity as JSON

- `loaf handoff link`:
  - `--type <type>` - Relationship type; defaults to related_to
  - `--reason <text>` - Relationship reason
  - `--json` - Output relationship ID, source/target, global database scope, and project identity as JSON

**Usage:**
```bash
loaf handoff new
loaf handoff show
loaf handoff list
```

---

## Council Management

### `loaf council`
Manage councils in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf council new` | Create a council in SQLite state |
| `loaf council show` | Show one council from SQLite state |
| `loaf council list` | List councils from SQLite state |
| `loaf council link` | Link a council to another entity |

**Options:**

- `loaf council new`:
  - `--title <title>` - Artifact title
  - `--body-file <path>` - Read Markdown body from a UTF-8 file
  - `--body -` - Read Markdown body from stdin
  - `--message <text>` - Use inline Markdown body text
  - `--spec <spec>` - Optional related spec
  - `--json` - Output created artifact, event, global database scope, and project identity as JSON

- `loaf council show`:
  - `--json` - Output artifact details, relationships, global database scope, and project identity as JSON

- `loaf council list`:
  - `--all` - Include archived artifacts
  - `--status <status>` - Filter by status
  - `--json` - Output artifacts, global database scope, and project identity as JSON

- `loaf council link`:
  - `--type <type>` - Relationship type; defaults to related_to
  - `--reason <text>` - Relationship reason
  - `--json` - Output relationship ID, source/target, global database scope, and project identity as JSON

**Usage:**
```bash
loaf council new
loaf council show
loaf council list
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
  - `--json` - Output per-file frontmatter errors and warnings as JSON

- `loaf kb status`:
  - `--json` - Output knowledge file totals, coverage counts, stale count, review age, and directories as JSON

- `loaf kb check`:
  - `--file <path>` - Reverse lookup: find knowledge files covering this path
  - `--json` - Output per-file staleness, coverage, commit, and review metadata as JSON

- `loaf kb review`:
  - `--json` - Output updated knowledge frontmatter as JSON

- `loaf kb init`:
  - `--json` - Output directory actions, config status, and QMD collections as JSON

- `loaf kb import`:
  - `--path <path>` - Path to the external project's knowledge directory
  - `--json` - Output QMD import collection status or import error as JSON

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

**Options:**

- `--dry-run` - Show recommendations without prompting for actions
- `--json` - Output housekeeping sections, cleanup candidates, signals, and SQLite-backed project identity when available as JSON
- `--sessions` - Only review sessions
- `--specs` - Only review specs
- `--plans` - Only review plans
- `--drafts` - Only review drafts
- `--handoffs` - Only review handoffs

**Usage:**
```bash
loaf housekeeping
```

---

## Trace Management

### `loaf trace`
Trace relationships for one state entity

**Options:**

- `--json` - Output traced entity, sources, relationships, global database scope, and project identity as JSON

**Usage:**
```bash
loaf trace
```

---

## Brainstorm Management

### `loaf brainstorm`
Manage brainstorms in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf brainstorm capture` | Capture a brainstorm in SQLite state |
| `loaf brainstorm list` | List brainstorms from SQLite state |
| `loaf brainstorm show` | Show one brainstorm from SQLite state |
| `loaf brainstorm promote` | Record brainstorm-to-idea promotion |
| `loaf brainstorm archive` | Archive one or more brainstorms |

**Options:**

- `loaf brainstorm capture`:
  - `--title <title>` - Brainstorm title
  - `--body-file <path>` - Read Markdown body from a UTF-8 file
  - `--body -` - Read Markdown body from stdin
  - `--message <text>` - Use inline Markdown body text
  - `--json` - Output created brainstorm, event, global database scope, and project identity as JSON

- `loaf brainstorm list`:
  - `--all` - Include archived brainstorms
  - `--status <status>` - Filter by status
  - `--json` - Output brainstorms, global database scope, and project identity as JSON

- `loaf brainstorm show`:
  - `--json` - Output brainstorm details, relationships, global database scope, and project identity as JSON

- `loaf brainstorm promote`:
  - `--to-idea <idea>` - Target idea
  - `--json` - Output promotion relationship, global database scope, and project identity as JSON

- `loaf brainstorm archive`:
  - `--reason <text>` - Archive reason
  - `--json` - Output archive result, archived brainstorms, global database scope, and project identity as JSON

**Usage:**
```bash
loaf brainstorm capture
loaf brainstorm list
loaf brainstorm show
```

---

## Idea Management

### `loaf idea`
Manage ideas in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf idea list` | List ideas from SQLite state |
| `loaf idea show` | Show one idea from SQLite state |
| `loaf idea capture` | Capture an idea in SQLite state |
| `loaf idea promote` | Record idea-to-spec promotion |
| `loaf idea resolve` | Resolve an idea by linking it to another entity |
| `loaf idea archive` | Archive one or more ideas |

**Options:**

- `loaf idea list`:
  - `--all` - Include resolved and archived ideas
  - `--status <status>` - Filter by status
  - `--json` - Output ideas, global database scope, and project identity as JSON

- `loaf idea show`:
  - `--json` - Output idea details, relationships, global database scope, and project identity as JSON

- `loaf idea capture`:
  - `--title <title>` - Idea title
  - `--json` - Output created idea, event, global database scope, and project identity as JSON

- `loaf idea promote`:
  - `--to-spec <spec>` - Target spec
  - `--json` - Output promotion relationship, global database scope, and project identity as JSON

- `loaf idea resolve`:
  - `--by <entity>` - Resolving entity
  - `--json` - Output resolution relationship, event, global database scope, and project identity as JSON

- `loaf idea archive`:
  - `--reason <text>` - Archive reason
  - `--json` - Output archive result, archived ideas, global database scope, and project identity as JSON

**Usage:**
```bash
loaf idea list
loaf idea show
loaf idea capture
```

---

## Spark Management

### `loaf spark`
Manage sparks in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf spark list` | List sparks from SQLite state |
| `loaf spark show` | Show one spark from SQLite state |
| `loaf spark capture` | Capture a spark in SQLite state |
| `loaf spark resolve` | Resolve a spark |
| `loaf spark promote` | Record spark-to-idea promotion |

**Options:**

- `loaf spark list`:
  - `--all` - Include resolved sparks
  - `--status <status>` - Filter by status
  - `--json` - Output sparks, global database scope, and project identity as JSON

- `loaf spark show`:
  - `--json` - Output spark details, relationships, global database scope, and project identity as JSON

- `loaf spark capture`:
  - `--scope <scope>` - Spark scope
  - `--text <text>` - Spark text
  - `--json` - Output created spark, event, global database scope, and project identity as JSON

- `loaf spark resolve`:
  - `--reason <text>` - Resolution reason
  - `--json` - Output resolution relationship, event, global database scope, and project identity as JSON

- `loaf spark promote`:
  - `--to-idea <idea>` - Target idea
  - `--json` - Output promotion relationship, global database scope, and project identity as JSON

**Usage:**
```bash
loaf spark list
loaf spark show
loaf spark capture
```

---

## Tag Management

### `loaf tag`
Manage tags in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf tag list` | List tags from SQLite state |
| `loaf tag show` | Show entities with a tag |
| `loaf tag add` | Add a tag to an entity |
| `loaf tag remove` | Remove a tag from an entity |

**Options:**

- `loaf tag list`:
  - `--json` - Output tags, global database scope, and project identity as JSON

- `loaf tag show`:
  - `--json` - Output tagged entities, global database scope, and project identity as JSON

- `loaf tag add`:
  - `--json` - Output tag mutation, entity, global database scope, and project identity as JSON

- `loaf tag remove`:
  - `--json` - Output tag mutation, entity, global database scope, and project identity as JSON

**Usage:**
```bash
loaf tag list
loaf tag show
loaf tag add
```

---

## Bundle Management

### `loaf bundle`
Manage bundles in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf bundle list` | List bundles from SQLite state |
| `loaf bundle create` | Create a bundle |
| `loaf bundle update` | Update a bundle |
| `loaf bundle show` | Show one bundle |
| `loaf bundle add` | Add an entity to a bundle |
| `loaf bundle remove` | Remove an entity from a bundle |

**Options:**

- `loaf bundle list`:
  - `--json` - Output bundles, global database scope, and project identity as JSON

- `loaf bundle create`:
  - `--title <title>` - Bundle title
  - `--tags <tags>` - Comma-separated tag query
  - `--json` - Output created bundle, tags, global database scope, and project identity as JSON

- `loaf bundle update`:
  - `--title <title>` - Bundle title
  - `--tags <tags>` - Comma-separated tag query
  - `--json` - Output updated bundle, tags, global database scope, and project identity as JSON

- `loaf bundle show`:
  - `--json` - Output bundle details, members, global database scope, and project identity as JSON

- `loaf bundle add`:
  - `--json` - Output bundle membership result, global database scope, and project identity as JSON

- `loaf bundle remove`:
  - `--json` - Output bundle membership result, global database scope, and project identity as JSON

**Usage:**
```bash
loaf bundle list
loaf bundle create
loaf bundle update
```

---

## Link Management

### `loaf link`
Manage explicit relationships in native SQLite state

**Subcommands:**

| Subcommand | Purpose |
|------------|---------|
| `loaf link create` | Create an explicit relationship |
| `loaf link list` | List relationships for one entity |
| `loaf link remove` | Remove an explicit relationship |

**Options:**

- `loaf link create`:
  - `--from <entity>` - Source entity
  - `--to <entity>` - Target entity
  - `--type <type>` - Relationship type
  - `--reason <text>` - Relationship reason
  - `--json` - Output relationship ID, source/target, global database scope, and project identity as JSON

- `loaf link list`:
  - `--json` - Output relationships, global database scope, and project identity as JSON

- `loaf link remove`:
  - `--from <entity>` - Source entity
  - `--to <entity>` - Target entity
  - `--type <type>` - Relationship type
  - `--json` - Output removed relationship ID, global database scope, and project identity as JSON

**Usage:**
```bash
loaf link create
loaf link list
loaf link remove
```

---

## Check Management

### `loaf check`
Run enforcement hook checks

**Options:**

- `--hook <id>` - Registered hook ID to run
- `--json` - Output hook result, pass/block status, exit code, warnings, errors, and findings as JSON

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
