---
topics:
  - changes
  - tasks
  - specs
  - journal
  - orchestration
covers:
  - docs/changes/**/*.md
  - .agents/specs/**/*.md
  - internal/cli/cli.go
  - internal/state/task_*.go
  - content/skills/breakdown/**/*
  - content/skills/implement/**/*
  - content/skills/orchestration/**/*
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-07-28'
---

# Work Records

Loaf uses Change artifacts for new bounded work. Existing specs and tasks remain durable compatibility records with supported CLI surfaces, while the project journal records what happened across conversations. The Change anatomy, task-packet discipline, derived states, and release cohorts are documented in [work-model.md](work-model.md) (rationale: ADR-022, ADR-023); this document covers the compatibility records around it.

## Current Workflow

```
/idea or /brainstorm → /shape → docs/changes/YYYYMMDD-slug/  (change.json + shape.md + tasks/)
                                      ↓
                              loaf change check
                                      ↓
                         /implement → review → /reflect → /ship
                                      ↓
                               project journal
```

`/release` publishes already-landed work separately; changes declaring a `target_release` gate their version's stable cut (see work-model.md). The change PR is the change's whole life — one change, one PR, merged when everything is done; per-task PRs remain the exception for genuinely multi-integration changes.

## Record Types

| Record | Location | Purpose |
|--------|----------|---------|
| Change | `docs/changes/YYYYMMDD-slug/` | Primary bounded-work contract for new work: `change.json` identity, `shape.md` contract, `tasks/` packets (legacy single-file `change.md` supported until the removal boundary) |
| Task | SQLite (`loaf task show/list`) | Existing durable work items with criteria, relationships, and status |
| Spec | `.agents/specs/SPEC-XXX-slug.md` | Existing bounded-work records retained for compatibility and deliberate conversion |
| Journal | SQLite (`loaf journal recent/show`) | Project-scoped decisions, discoveries, commits, and execution context |

`loaf change check` validates Change structure and reports derived executability; `loaf change check --require-executable` requires the implementation contract to be complete. Existing `loaf spec` and `loaf task` commands continue to operate on their records, but they do not define the default artifact for newly shaped work.

## Working Rules

- **Bound the outcome before implementation.** State the problem, scope, rabbit holes, implementation units, verification contract, and definition of done in the Change.
- **Keep implementation units coherent.** Use branches, commits, and pull requests as containment boundaries that can be reviewed and landed independently without inventing generic sequencing labels.
- **Use tasks when they carry durable value.** A task should represent one concern with observable completion and explicit relationships; do not create task ceremony merely to mirror a Change section.
- **Keep progress derived where possible.** Git, pull requests, checks, and journal entries are the evidence. Do not add mutable progress fields to Change frontmatter.
- **Preserve compatibility records deliberately.** Existing specs and tasks remain supported until converted; do not rewrite or delete them just to make the vocabulary look current.

## CLI Commands

### `loaf change`

| Subcommand | Purpose |
|------------|---------|
| `init <slug>` | Scaffold `docs/changes/<YYYYMMDD>-<slug>/` (`change.json` + `shape.md` + seeded `tasks/`; `--brief` for capture-only) |
| `check [path]` | Validate a Change (both layouts) and report derived executability |
| `check [path] --require-executable` | Fail unless the implementation contract is structurally executable |
| `list [--target <X.Y.Z>]` | Units/cohort projection: layout, target, derived state |
| `tasks --json [path]` | Stable-ID task index with relations and derived completion |
| `show [path]` | Layout, target, derived state, and PR set from squash subjects |
| `verify [path]` | Run declared criteria; write `receipts/verify.json` (cohort members) |
| `report new <slug> --kind <kind>` | Stamp an authored report shell under `reports/` (closed registry) |

### `loaf task`

| Subcommand | Purpose |
|------------|---------|
| `list` | Show task board grouped by status |
| `list --status <status>` | Filter tasks to one status |
| `show <id>` | Display single task details |
| `status` | Summary counts |
| `create` | Create new task |
| `update <id>` | Update metadata such as status, priority, relationships, and spec linkage |
| `archive [ids...]` | Archive completed tasks in SQLite state |
| `refresh` | Compatibility diagnostic; no-op in SQLite-backed projects |
| `sync` | Compatibility diagnostic; no-op in SQLite-backed projects |

### `loaf spec`

| Subcommand | Purpose |
|------------|---------|
| `list` | Show existing specs with status |
| `archive [ids...]` | Move completed specs to `archive/` |

### `loaf journal`

| Subcommand | Purpose |
|------------|---------|
| `log [entry]` | Append a project-scoped journal entry |
| `recent` | Show the recent journal timeline (`--branch`, `--since-last-wrap`) |
| `search <query>` | Full-text search across the project journal |
| `show <id>` | Read one journal entry |
| `context` | Emit the layered continuity digest (latest wrap, branch entries, and open tasks) |
| `export` | Export the journal to Markdown or JSONL |

## Storage Provenance and Compatibility

The SQLite cutover recorded by historical work identity `SPEC-045` moved ephemeral task, idea, spark, brainstorm, and draft records into the global state store. `.agents/TASKS.json` and the corresponding ephemeral Markdown directories are rollback material, not compatibility mirrors. `findAgentsDir()` remains relevant for durable `.agents/` content such as existing specs, reports, councils, handoffs, and project config.

Do not recreate `.agents/TASKS.json` as an index. `loaf state restore-ephemerals <backup-id>` may restore it only when explicitly undoing the cutover, and `loaf check --hook ephemeral-provenance` detects tracked ephemeral files that return outside that rollback procedure.

Stale branches may still carry deleted ephemeral files. Rebase or merge current main, keep the deletion side for `.agents/{tasks,ideas,sparks,sessions,brainstorms,drafts}/` and `.agents/TASKS.json`, then run `loaf check --hook ephemeral-provenance`. If rollback is intentional, restore and re-import through the supported state commands rather than maintaining hand-edited mirrors.

The journal-first model was established by historical work identity `SPEC-056`, which superseded the session router associated with `SPEC-032`. These identifiers document provenance, not current workflow instructions.

## Journal Model

The project journal is the only session-related structure. There is no session entity, status, or lifecycle.

- **Project-scoped events, correlated by harness id.** `journal_entries` rows carry `project_id NOT NULL` and an opaque `harness_session_id` that groups one conversation's entries. Nobody opens, closes, or transitions anything.
- **Concurrency-safe by construction.** Conversations across branches, worktrees, or harnesses may interleave rows with different `harness_session_id` tags without a mutable router.
- **Wrap is an optional checkpoint.** `loaf journal log "wrap(scope): …"` records synthesis worth saving; a conversation that ends without one still leaves a valid journal.
- **Continuity is derived and ephemeral.** The SessionStart hook runs `loaf journal context --from-hook` to emit a read-time digest that is shown and discarded. Subagent invocations with `agent_id` present exit silently and write nothing.

## Linear Integration

Linear is an optional task backend. When `integrations.linear.enabled` is true in `.agents/loaf.json`, compatible spec/task workflows use Linear issues for execution visibility; when it is false or absent, they use local `loaf task` records. Git-canonical deliberation artifacts remain with the code rather than being re-hosted in the tracker.

## Cross-References

- [work-model.md](work-model.md) — the Change anatomy, task packets, derived states, and release cohorts
- [cli-design.md](cli-design.md) — CLI design philosophy and command patterns
- [knowledge-management-design.md](knowledge-management-design.md) — knowledge system conventions
- [../changes/](../changes/) — retained Change contracts
