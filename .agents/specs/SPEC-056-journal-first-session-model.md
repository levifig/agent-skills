---
id: SPEC-056
status: done
title: "Journal-first: project journal replaces the session entity"
---

# SPEC-056: Journal-First — Project Journal Replaces the Session Entity

## Problem Statement

Loaf's session model treats sessions as first-class entities with a six-state lifecycle (`active`, `in_progress`, `paused`, `stopped`, `done`, `archived`), rotation logic, branch scoping, and per-session state snapshots. Production data shows the lifecycle doesn't maintain itself: of 276 sessions, 19 are stuck "active" and 21 "paused" (starts that never closed), 60 (22%) contain zero journal entries, `session_state_snapshots` has never held a single row, and ~24% of all 4,541 journal entries are lifecycle noise (`=== SESSION STARTED ===` / `STOPPED`). The entity costs ~2,000 LOC of lifecycle code plus housekeeping machinery that exists largely to sweep up the garbage the lifecycle itself produces.

Meanwhile the thing users actually need — a durable, searchable, project-scoped record of what happened across all conversations — already exists: `journal_entries` is project-anchored (`project_id NOT NULL`) in the global SQLite DB in `XDG_DATA_HOME`, with `harness_session_id` available as a plain column. The session entity adds ceremony without adding information, and its rotation/status semantics actively fight the core requirement: **concurrent conversations on the same project — across branches, worktrees, even harnesses — must be seamless and conflict-free.**

## Strategic Alignment

- **Vision:** Directly serves "Session journals are external memory" and the three-artifact model (spec, tasks, journal). The journal was always the load-bearing concept; the session entity was scaffolding around it.
- **Personas:** The multi-worktree solo operator running parallel conversations — currently rotation logic pauses one conversation's session when another starts on the same branch; journal-first makes parallelism structurally conflict-free.
- **Architecture:** Global SQLite partitioned by `project_id` unchanged. **Tensions to note (for /loaf:reflect post-ship):** reverses SPEC-048's anointed `start → log → end --wrap` lifecycle; supersedes the SPEC-032 three-tier session router; shrinks SPEC-049's scope (session statuses cease to exist); ARCHITECTURE.md §Session Lifecycle, §Session Routing, and §Session Enrichment describe machinery this spec deletes.

## Solution Direction

**The journal becomes the only session-related structure.** Entries are project-scoped events tagged with an opaque `harness_session_id` correlation column — a grouping tag nobody opens, closes, or transitions. No entity, no statuses, no rotation: two conversations logging concurrently just interleave rows with different tags, which is correct by construction.

**Wrap becomes an optional checkpoint entry type, not a lifecycle transition.** Rationale (the derivation question): almost everything a wrap contains is derivable from raw entries — *except* synthesis that exists only in the dying conversation's context: "tried X, abandoned because Y, next is Z." Raw entries are events; that connective narrative evaporates with the context window unless written down. So wrap survives as a voluntary, high-value entry type, written when there's synthesis worth saving. Nothing is ever "unwrapped"; a conversation that dies abruptly leaves a perfectly valid journal. Wrap scope under concurrency: a wrap claims the writing conversation's own entries (its `harness_session_id`); manual/untagged wraps fall back to branch scope — implementer's call on mechanics.

**Continuity is derived, layered, and ephemeral.** At conversation start, the hook emits: latest project-level wrap entry + recent entries scoped to the current branch/worktree + open (`in_progress`/`pending`) tasks from the tasks table. Computed at read time, shown, discarded — never persisted. Auto-persisting arrival syntheses would re-pollute the journal with derived noise; only deliberate checkpoints get written.

**CLI surface:** `loaf journal` namespace (`log`, `recent`, `search`, `show`, `context`, export); `loaf session` deleted outright — no alias, no shim. Hooks stop writing start/stop marker entries entirely; the `session` entry type dies. SessionStart hook shrinks to: resolve project, emit layered digest, preserve subagent silent-exit. Stop hook obligation disappears.

**Downstream conversions:** `handoffs.session_id` FK → plain `harness_session_id TEXT` provenance tag; `journal_search` FTS dropped and rebuilt keyed on the new shape; `events`/`aliases` rows with `entity_kind='session'` deleted; `session_state_snapshots` dropped (empty); `loaf session enrich` deleted (it exists to patch holes in an entity that no longer exists; resurrectable from git).

**Migration (global, all projects, destructive by consent):** back up the DB file; backfill `harness_session_id` onto journal rows from their session records; purge the ~1,088 lifecycle-noise entries; drop `journal_entries.session_id`; drop `sessions` and `session_state_snapshots`; rebuild FTS.

## Scope

### In Scope
- Schema migration as above, idempotent, with pre-migration backup
- `loaf journal` command namespace; deletion of the entire `loaf session` namespace including `enrich`, `archive`, `list`, `show`
- Hook rewrite (SessionStart digest, subagent detection, removal of Stop/SessionEnd session mutation, TaskCompleted + PreCompact retargeted to `loaf journal log`)
- Skill/agent/template convergence (~80 references), including wrap, implement, housekeeping, handoff, triage, cli-reference
- Journal export to markdown and JSONL (SQLite stays canonical)
- Housekeeping simplification (session sweeping removed)
- ARCHITECTURE.md session sections rewritten to describe the journal-first model

### Out of Scope
- Conversation-grouped browse views (`--group-by conversation`, time-gap clustering) — flat timeline + FTS only
- Entry-type taxonomy redesign beyond killing `session` and formalizing `wrap` (SPEC-049 owns vocabulary)
- Journal retention/pruning policies
- Cross-project journal queries
- Any replacement for enrich

### Rabbit Holes
- Building "smart" arrival digests (LLM synthesis at start) — the digest is a deterministic query, not a model call
- Reintroducing open-loop tracking — "recent branch entries with no subsequent wrap" *is* the signal; don't build state for it
- Perfecting noise-purge heuristics — match the known marker patterns, report the count, move on

### No-Gos
- No new session-like entity under another name (no `conversations` table)
- No persisted derived context — start digests are ephemeral
- No JSONL as primary storage
- No `loaf session` compatibility alias — clean break

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Synthesis loss if wraps never get written | Med | Med | /wrap skill stays prominent; layered digest degrades gracefully to raw entries |
| Same-branch parallel conversations interleave in digest | Med | Low | Harness-id tags let consumers filter; interleaving is honest, not corrupt |
| Skill churn regression across ~80 refs | Med | Med | Repo-wide grep gate in test conditions; SPEC-051 routing evals rerun if descriptions change |
| Global migration damages other projects' data | Low | High | Mandatory DB file backup before migrate; dry-run mode reporting row counts |

## Resolved Questions

- Start digest feed: journal + open tasks (`in_progress`/`pending` from tasks table). Resolved 2026-07-03.
- Wrap fate: optional checkpoint entry type, no lifecycle, no stop-hook obligation. Resolved 2026-07-03.
- CLI naming: `loaf journal` namespace, `loaf session` deleted with no alias. Resolved 2026-07-03.
- Migration: backfill + purge noise, global across all projects, with backup. Resolved 2026-07-03.
- Enrich: deleted. Browse model: flat timeline + FTS only. Resolved 2026-07-03.

## Test Conditions

- [ ] Two concurrent conversations (different worktrees, same project) log and wrap with zero cross-contamination — integration test
- [ ] Fresh conversation start emits latest project wrap + unwrapped branch entries + open tasks; fresh branch degrades to project wrap only
- [ ] `loaf session <anything>` → unknown command; `loaf journal log/recent/search/show/context` function
- [ ] Migration on a production-shaped fixture: harness ids backfilled, noise purge count reported, zero non-noise rows lost, FTS hits preserved for surviving entries
- [ ] Post-migration schema contains no `sessions`/`session_state_snapshots`; `schema_migrations` records the step
- [ ] Untagged manual entry lands project-scoped and appears in timeline and search
- [ ] Subagent SessionStart exits silently, writes nothing
- [ ] Repo grep: zero `loaf session` references in skills/hooks/templates/docs
- [ ] `loaf journal export` produces valid markdown and JSONL for a project

## Priority Order

Tracks ship in order, one conventional commit per track after its gate passes. Branch: `feat/journal-first`. PR opened after final verification.

1. **Schema + migration** — new shape, backfill, purge, drops, FTS rebuild. Go/no-go: migration passes on a copy of the real DB before anything else lands.
2. **`loaf journal` CLI** — namespace + digest query + export; `loaf session` deleted. Go/no-go: CLI test conditions pass.
3. **Hooks** — start digest, subagent guard, lifecycle hook removal. Go/no-go: concurrent-conversation integration test passes.
4. **Skills/agents/templates convergence** — the ~80 refs. Go/no-go: grep gate clean.
5. **Cleanup + docs** — housekeeping, export polish, ARCHITECTURE.md rewrite. Can be trimmed if scope tightens, but docs shouldn't ship stale.

<!-- loaf:render kind=spec contract=durable-doc-v1 -->
