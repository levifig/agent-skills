---
name: loaf-reference
description: >-
  Documents how agents operate the Loaf CLI: command discovery via loaf --help,
  JSON diagnosis surfaces, guided config maintenance, and troubleshooting. Use
  when unsure which loaf command to invoke or how to validate project state. Not
  for workflow ...
user-invocable: false
version: 2.0.0-alpha.5
---

# Loaf Reference

## Contents
- Operating Rules
- Command Index
- Topics

The Loaf operating manual for agents: how to discover commands, diagnose project
state, and keep configuration current. It teaches reading the CLI, not
memorizing it.

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
| `loaf change` | Shape-first Change artifacts: git-canonical work context under docs/changes/ | init, check |
| `loaf render` | Maintain committed durable Markdown renders | sweep |
| `loaf state` | Manage native SQLite state | path, status, init, doctor, repair legacy-project-database, repair relationship-origin, migrate markdown, migrate storage-home, migrate lifecycle-statuses, backup, backup verify, restore-ephemerals, verify-ephemerals, export, export all, export triage, export spec, export release-readiness |
| `loaf journal` | Record and read the project-scoped journal (the durable record across all conversations) | log, recent, search, show, context, export |
| `loaf project` | Manage durable project identity | list, show, identity, rename, move, delete |
| `loaf migrate` | Run native migration workflows | markdown, storage-home, lifecycle-statuses, worktree-storage |
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
| Command routing | [references/command-routing.md](references/command-routing.md) | Deciding which command a task needs; locating the JSON diagnosis surfaces |
| Troubleshooting | [references/troubleshooting.md](references/troubleshooting.md) | Diagnosing state, config, or alignment failures; isolating a throwaway database |
