---
name: loaf-reference
description: >-
  Documents how agents operate the Loaf CLI: command discovery via loaf --help,
  JSON diagnosis surfaces, config-aware maintenance, and troubleshooting. Use
  when unsure which loaf command to invoke, how to validate project state, or
  when asked to upgrade, diagnose, repair, configure, or bring a Loaf project
  current. Not for workflow guidance (workflow skills own their CLI contracts)
  or build internals.
version: 2.0.0-alpha.9
---

# Loaf Reference

## Contents
- Operating Rules
- Journal Context (contract v2)
- Command Index
- Topics

The Loaf operating manual for agents: how to discover commands, diagnose project state, and keep configuration current. It teaches reading the CLI, not memorizing it.

**Note:** This file is auto-generated from native CLI reference metadata. Do not edit manually.

## Operating Rules

- Get exact, current syntax live: `loaf --help` lists every command, `loaf <command> --help` details one. This index is a map, not the contract.
- Prefer `--json` surfaces when diagnosing: `loaf config check --json`, `loaf state doctor --json`, `loaf change check --json`. Parse the structured output instead of scraping human-readable text.
- Run the deterministic CLI command before hand-editing anything it manages; the command owns its files.
- Use `--fix` only for safe, mechanical repairs, and review what it changed.
- Ask the user for project-owned choices — GitHub account, tracker or integration election, which harnesses to install — never guess them.
- Never hand-edit Loaf-managed hook files; regenerate them through `loaf build` and `loaf install`.
- Re-run the relevant check after any change and confirm it passes.
- Log meaningful decisions to the journal: `loaf journal log "decision(scope): ..."`.

## Journal Context (contract v2)

`loaf journal context` is an active-truth read model, not the former latest-arbitrary-wrap plus branch entries plus open tasks summary. Consume its named layers and diagnostics rather than inferring state from an omitted layer.

| Layer | Truth it supplies |
|-------|-------------------|
| `project-synthesis` | The latest `wrap(project)` synthesis. |
| `scoped-checkpoint` | The latest non-project wrap, only when no project synthesis exists; it is explicitly labeled as a fallback. |
| `active-lineage` | Journal evidence associated with active Change lineage. |
| `unresolved-blockers` | Blocks that do not have a later exact-scope unblock. |
| `deferred-intent` | Open deferred-intent decision and spark pairs. |
| `active-changes` | Git-derived active Change evidence and worktree state. |
| `branch-recency` | Recent entries on the selected branch after entries already surfaced as active truth are removed. |
| `transitional-tasks` | Open task-board records during the Markdown-to-native transition. |

Each layer reports `source_available`, `available_count`, `shown_count`, `truncated`, and an exact `expand_command`; paginated layers also return a cursor. `source_available: false` means the source could not be derived and is not an empty result. In particular, an unavailable Change source marks both `active-changes` and `active-lineage` unavailable and emits a diagnostic.

Use `--branch` to select `branch-recency` scope and bind state cursors. It does not override active Change provenance or reasons, which always use the actual Git branch. Use `--layer` to request one canonical layer. `--limit` accepts 1 through 100 only with `--layer`; `--cursor` also requires `--layer` and cannot expand the intrinsic one-item `project-synthesis` or `scoped-checkpoint` layers. Reuse the returned `expand_command` verbatim: cursors are bound to their layer, project, branch, snapshot, and limit. `--json` is the stable machine surface; human output retains the same counts, unavailable markers, and expansion command.

## Command Index

Names and one-line purposes only. Run `loaf <command> --help` for options, arguments, and current usage.

| Command | Purpose | Subcommands |
|---------|---------|-------------|
| `loaf build` | Build skill distributions for agent harnesses | — |
| `loaf install` | Install Loaf to detected AI tool configurations | — |
| `loaf config` | Validate and refresh project Loaf config | check |
| `loaf init` | Initialize a project with Loaf structure | — |
| `loaf release` | Create a new release with changelog, version bump, and tag | — |
| `loaf search` | Search SQLite artifact bodies, journal entries, and indexed docs | — |
| `loaf docs` | Manage docs/ indexing | index |
| `loaf change` | Shape-first Change artifacts: git-canonical work context under docs/changes/ | init, check, list |
| `loaf render` | Maintain committed durable Markdown renders | sweep |
| `loaf state` | Manage native SQLite state | path, status, init, doctor, repair legacy-project-database, repair relationship-origin, repair journal-search, migrate markdown, migrate storage-home, migrate schema, migrate deferrals, migrate lifecycle-statuses, backup, backup verify, backup restore, restore-ephemerals, verify-ephemerals, export, export all, export triage, export spec, export release-readiness |
| `loaf journal` | Record and read the project-scoped journal (the durable record across all conversations) | log, recent, search, show, context, export, defer |
| `loaf project` | Manage durable project identity | list, show, identity, rename, move, delete |
| `loaf migrate` | Run native migration workflows | markdown, storage-home, schema, lifecycle-statuses, worktree-storage |
| `loaf task` | Manage project tasks | list, show, status, create, update, archive, refresh, sync |
| `loaf spec` | Manage project specs | new, list, show, status, render, finalize, archive, delete |
| `loaf report` | Manage durable reports (research, audits, investigations) | list, show, render, generate, create, finalize, archive |
| `loaf finding` | Manage report findings and verdicts in native SQLite state | list, show, create, verdict, import-json |
| `loaf run` | Manage provenance runs for generated findings and reports | list, show, create, complete |
| `loaf plan` | Manage plans in native SQLite state | new, show, list, link |
| `loaf handoff` | Manage handoffs in native SQLite state | new, show, list, link |
| `loaf council` | Manage councils in native SQLite state | new, show, list, link |
| `loaf kb` | Knowledge base management | glossary, validate, status, check, review, init, import |
| `loaf setup` | One-step bootstrap: init + build + install | — |
| `loaf version` | Show version info and project statistics | — |
| `loaf housekeeping` | Scan project artifacts and recommend housekeeping actions | — |
| `loaf trace` | Trace relationships for one state entity | — |
| `loaf brainstorm` | Manage brainstorms in native SQLite state | capture, list, show, promote, archive |
| `loaf idea` | Manage ideas in native SQLite state | list, show, capture, promote, resolve, archive |
| `loaf intent` | Manage tracked Intent in native SQLite state; disposition is derived from append-only facts | create, defer, resume, resolve, show, list |
| `loaf intake` | Read the deterministic local intake projection; triage judgment stays with humans and Skills | list |
| `loaf exploration` | Manage relational Exploration continuity: immutable portable checkpoints, no lifecycle status, no current pointer | create, checkpoint, list, context, conversation |
| `loaf conversation` | Manage logical conversations and machine-local provenance handles; handles never imply portable context | create, show, list, handle, observe |
| `loaf spark` | Manage sparks in native SQLite state | list, show, capture, resolve, promote |
| `loaf tag` | Manage tags in native SQLite state | list, show, add, remove |
| `loaf bundle` | Manage bundles in native SQLite state | list, create, update, show, add, remove |
| `loaf link` | Manage explicit relationships in native SQLite state | create, list, remove |
| `loaf check` | Run enforcement hook checks | — |
| `loaf doctor` | Diagnose Loaf project alignment (symlinks, stale files, version drift) | — |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Configuration maintenance | [references/configuration.md](references/configuration.md) | Checking whether a project's Loaf config is current and repairing it; wiring project-owned choices |
| Config-aware maintenance protocol | [references/maintenance.md](references/maintenance.md) | Upgrading, diagnosing, repairing, or bringing a project current: diagnose, plan, ask, apply, verify |
| Command routing | [references/command-routing.md](references/command-routing.md) | Deciding which command a task needs; locating the JSON diagnosis surfaces |
| Troubleshooting | [references/troubleshooting.md](references/troubleshooting.md) | Diagnosing state, config, or alignment failures; isolating a throwaway database |
