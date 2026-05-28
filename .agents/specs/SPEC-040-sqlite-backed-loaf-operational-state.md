---
id: SPEC-040
title: SQLite-backed Loaf operational state
source: >
  shaped from raw ideas 20260522-101624-sqlite-backed-operational-state,
  20260522-101625-loaf-cli-tui-state-workbench,
  20260522-101626-generated-markdown-review-exports,
  20260522-101627-structured-session-transcripts-and-reports,
  and 20260522-101628-triage-resolution-graph
created: '2026-05-23T01:10:29Z'
status: drafting
branch: feat/sqlite-operational-state
source_sessions:
  - id: 019e525b-f98e-73a1-a280-73cda4c7852a
    role: shaped
    note: SQLite operational state shaping
related_specs:
  - SPEC-010
  - SPEC-023
  - SPEC-029
  - SPEC-032
  - SPEC-036
  - SPEC-037
  - SPEC-038
  - SPEC-039
---

# SPEC-040: SQLite-backed Loaf operational state

## Problem Statement

Loaf's `.agents/` Markdown model has accumulated too many jobs:

1. Human-readable artifacts: specs, reports, drafts, session summaries, task bodies, and knowledge prose.
2. Queryable operational state: statuses, timestamps, task/spec relationships, idea lineage, spark resolution, backend mappings, hook events, and session routing.
3. Review exports: PR audit packets, release readiness summaries, triage closure reports, and session reports.

The result is fragile. A task can be complete, an idea can be implemented, and a spark can still resurface because the closure relationship is implicit prose spread across session entries, frontmatter, archived files, and `TASKS.json`. The CLI has to grep and parse Markdown to answer questions that are naturally relational, while agents still need to remember which files to edit and which conventions close the loop.

SPEC-010 deliberately introduced `TASKS.json` as structured metadata because direct Markdown editing was too brittle. SPEC-036 then centralized `.agents/` because agentic state is project-scoped, not branch-scoped. SPEC-039 already names XDG-backed SQLite as the intended durable home for the future ledger. This spec makes that storage model explicit and defines the migration path.

## Strategic Alignment

- **Vision:** Loaf should make agentic work mechanically reliable. Operational truth belongs in a structured store that the CLI owns, not scattered across prose conventions.
- **Personas:** Solo developers get a dependable local state workbench and fewer recurring cleanup loops. Team leads get cleaner external artifacts, deterministic status/reporting commands, and private mappings to external systems.
- **Architecture:** The CLI remains the protocol layer. Skills route through CLI commands. Hooks enforce invariants. Markdown remains valuable, but as authored prose or generated export, not as the operational database.

## Solution Direction

Introduce a project-scoped SQLite store for Loaf operational state, stored outside the repository under XDG paths and accessed only through the Loaf CLI.

The storage boundary becomes:

- **SQLite operational state:** specs, tasks, ideas, sparks, brainstorms, shaping drafts, sessions, reports, journal entries, status transitions, relationships, classifications/tags, bundles, provenance, source links, backend mappings, hook events, triage resolution state, and generated-export metadata.
- **Auth/secrets state:** outside the repository and outside this spec's schema, preferably OS keychain or a future auth-specific store. SQLite must not store Linear tokens or other secrets.
- **Markdown authored prose:** durable human-authored specs, ADRs, reports, drafts, knowledge files, changelog prose, and task/spec bodies when a human-readable document is the artifact.
- **Generated Markdown exports:** session reports, PR audit packets, release readiness reports, triage closure packets, spec snapshots, and review bundles generated from SQLite when Git/review needs a stable artifact.
- **External systems:** Linear/GitHub-native IDs and outcome language only. Internal Loaf IDs stay private per SPEC-038.

Keep `SPEC-*` as the internal mutable work-definition identity. This spec does not introduce `WORK-*` or rename specs. The database may use generic table names such as `entities` or `relationships`, but user-facing commands and skill guidance continue to talk about specs, tasks, ideas, sparks, sessions, and reports.

Database row IDs become canonical. Human-facing IDs such as `SPEC-040`, `TASK-184`, idea filenames, branch names, worktree paths, and harness session IDs become aliases, observed context, or compatibility handles. They can correlate rows, but they should not be required for writes to succeed.

The journal becomes an append-only event table, not the intake backlog. A journal row can always be written with optional columns such as observed worktree, branch, harness session, spec, task, commit, and source hook. If a related session/spec/task row is unknown at write time, the entry still lands; later commands can add relationship rows without rewriting the journal entry. Ideas, sparks, brainstorms, and shaping drafts have their own tables because they are backlog/state objects, not merely journal notes.

## Relationship To Other Specs

- **SPEC-010** introduced the managed metadata pattern because Markdown frontmatter was too fragile as a mutation surface. SPEC-040 supersedes `TASKS.json` as the target operational store while preserving the lesson: metadata mutations go through CLI commands.
- **SPEC-023** keeps backend routing behind the CLI. SPEC-040 provides the local structured state that backend adapters read and write.
- **SPEC-029** enriches journals from JSONL. SPEC-040 stores structured session/transcript/report rows and can generate journal/report views from them.
- **SPEC-032** makes session routing depend on `claude_session_id`. SPEC-040 preserves harness-session identifiers as routing/provenance fields without making Markdown session files the routing database.
- **SPEC-036** centralizes `.agents/` in the main worktree. SPEC-040 continues that project-scoped model but moves canonical operational state to XDG-backed SQLite.
- **SPEC-037** defines specs as mutable internal work definitions. SPEC-040 stores spec lifecycle and relationships without making specs durable truth.
- **SPEC-038** bans internal artifact leakage in external surfaces. SPEC-040 stores the private mapping and generates compliant exports.
- **SPEC-039** owns Linear OAuth and GraphQL behavior. SPEC-040 owns the local ledger/state substrate that SPEC-039 maps to external Linear IDs.

## Scope

### In Scope

- Define the SQLite storage location policy:
  - project-scoped database outside the repository
  - XDG-backed path
  - no secrets, no unnecessary PII, and raw transcript capture only when explicitly designed with redaction/export controls
  - explicit handling for single-checkout and git-worktree projects
- Define an initial schema for:
  - specs
  - tasks
  - ideas
  - sparks
  - brainstorms
  - shaping drafts
  - sessions
  - reports
  - journal entries
  - events/status transitions
  - typed relationships
  - classifications/tags and bundles
  - source/provenance links
  - backend mappings
  - hook events
  - generated exports
- Add SQLite lifecycle commands:
  - `loaf state init`
  - `loaf state status`
  - `loaf state doctor`
  - `loaf state migrate markdown --dry-run`
  - `loaf state migrate markdown --apply`
  - `loaf state export <kind>`
- Make existing command families state-backed or state-aware:
  - `loaf spec list/show/archive`
  - `loaf task list/show/create/update/archive/refresh/sync`
  - `loaf session list/show/log/enrich/report`
  - `loaf report list/create/finalize/archive/generate`
  - `loaf housekeeping`
- Add first-class triage/lineage commands:
  - `loaf idea list/show/promote/archive/resolve`
  - `loaf spark list/show/promote/resolve`
  - `loaf brainstorm list/show/promote/archive`
  - `loaf tag list/add/remove`
  - `loaf bundle list/show/create/update`
  - `loaf link create/list/remove`
  - `loaf trace <id>`
- Generate Markdown exports from state for review and handoff:
  - session reports
  - triage closure packets
  - PR/release readiness packets
  - spec/task snapshots where useful
- Update workflow skills so they query and mutate state through CLI commands instead of editing Markdown/frontmatter for lifecycle changes.
- Preserve current Markdown artifacts during migration. Imported files become source links and optional prose bodies, not disposable input.

### Out of Scope

- Replacing `SPEC-*` terminology with `WORK-*`, `BRIEF-*`, or another identity.
- Implementing Linear OAuth, token refresh, or GraphQL requests. SPEC-039 owns that.
- Storing Linear access/refresh tokens in SQLite.
- Building a full TUI, GUI, web app, daemon, mobile client, or `loafd`.
- QMD-backed KB fragment retrieval. Knowledge prose remains Markdown; QMD-style indexing belongs to a separate KB/retrieval spec.
- Cross-machine sync, hosted sync, or multi-user collaboration.
- Rewriting all historical `.agents/` artifacts by hand.
- Making generated Markdown the source of truth.
- Adding a third-party SQLite dependency without an explicit dependency decision.

### Rabbit Holes

- Designing a universal object graph for all future Loaf concepts. Start with the operational surfaces Loaf already has.
- Treating SQLite as a document store for every Markdown body. Store metadata, relationships, excerpts, hashes, and source links; keep authored prose in files unless the row itself is the durable record.
- Dumping every idea, spark, or brainstorm into the journal. The journal records observations and events; backlog items live in their own tables.
- Full transcript ingestion of every tool result. Capture structured summaries and pointers first; raw/noisy transcript storage can remain harness-native.
- Automatic bidirectional sync with every backend. Define explicit sync/status commands before any background reconciliation.
- Perfect migration of every old archived artifact. Preserve source links and import what is structurally knowable.

### No-Gos

- Do not store passwords, access tokens, refresh tokens, API keys, or other secrets in SQLite.
- Do not make SQLite a Git-tracked repository file.
- Do not expose `SPEC-*`, `TASK-*`, `.agents/...`, tracks, or phases in generated external artifacts.
- Do not remove current Markdown artifacts until import/export parity is proven.
- Do not require a session ID, branch, worktree, spec, or task before recording a journal entry.
- Do not require Linear or any external tracker.
- Do not require a long-running daemon for the first implementation.

## Data Model Direction

The schema should prefer explicit tables for high-value operational concepts over a vague all-purpose blob table. A small generic relationship layer is acceptable, but core entities should remain understandable and queryable.

Initial tables:

```text
projects
aliases
specs
tasks
ideas
sparks
brainstorms
shaping_drafts
sessions
reports
journal_entries
events
relationships
tags
entity_tags
bundles
bundle_members
sources
backend_mappings
hook_events
exports
schema_migrations
```

Key principles:

- Every row has stable internal ID, created timestamp, updated timestamp, and provenance.
- Human-facing IDs and filenames are aliases. They are unique only within the relevant namespace and may change as the system evolves.
- Status changes are events, not just overwritten fields.
- Journal entries are append-only observations with nullable correlation fields. They should not fail because an optional related entity is missing.
- Relationships are typed and explainable: `promoted_to`, `resolved_by`, `implements`, `blocked_by`, `derived_from`, `supersedes`, `exported_as`.
- Ideas, sparks, brainstorms, and shaping drafts form a rough-draft backlog. They can be tagged, bundled, promoted into shaping, absorbed by a finalized spec, resolved by tasks, or archived with a reason.
- Tags/classifications are many-to-many across specs, tasks, ideas, sparks, brainstorms, reports, sessions, and journal entries. Bundles are named collections built from tags and explicit membership.
- Source links preserve the Markdown file path, line/hash where available, and import timestamp.
- Backend mappings keep private internal-to-external identity translation.
- Generated exports record what state/version produced them.
- Schema migrations are versioned and reversible where practical.

## Command Surface

### State Lifecycle

```bash
loaf state init
loaf state status
loaf state status --json
loaf state doctor
loaf state doctor --fix
loaf state path
loaf state backup
```

### Migration

```bash
loaf state migrate markdown --dry-run
loaf state migrate markdown --apply
loaf state migrate markdown --resume
loaf state migrate markdown --json
```

Migration imports current `.agents/` material, including specs, tasks, ideas, sparks, sessions, reports, `TASKS.json`, and known archive directories. Dry-run output must show counts, skipped files, conflicts, inferred relationships, and destructive-risk warnings before `--apply`.

### Specs And Tasks

Existing commands stay recognizable:

```bash
loaf spec list
loaf spec show SPEC-040
loaf spec archive SPEC-040

loaf task list
loaf task show TASK-184
loaf task create --spec SPEC-040 --title "..."
loaf task update TASK-184 --status done
loaf task archive --spec SPEC-040
```

The implementation should route these through SQLite once state is initialized. Pre-SQLite compatibility can remain as a fallback during migration, but the fallback must be visible in `loaf state status`.

### Ideas, Sparks, And Triage

```bash
loaf idea list
loaf idea show IDEA-20260522-101624
loaf idea capture --title "..."
loaf idea promote IDEA-20260522-101624 --to-spec SPEC-040
loaf idea resolve IDEA-20260522-101624 --by SPEC-040
loaf idea archive IDEA-20260522-101624 --reason "..."

loaf spark list
loaf spark capture --scope architecture --text "..."
loaf spark promote SPARK-... --to-idea IDEA-...
loaf spark resolve SPARK-... --by IDEA-... --reason "..."

loaf brainstorm list
loaf brainstorm show BRAINSTORM-...
loaf brainstorm promote BRAINSTORM-... --to-idea IDEA-...
loaf brainstorm archive BRAINSTORM-... --reason "..."

loaf trace IDEA-20260522-101624
loaf link create IDEA-... SPEC-040 --type promoted_to
loaf link list SPEC-040
```

`loaf trace <id>` is the important user-facing payoff: it should answer why something exists, what it became, what resolved it, and why it should or should not resurface in triage.

### Classification And Bundles

```bash
loaf tag list
loaf tag add IDEA-... sqlite
loaf tag remove TASK-... sqlite
loaf tag show sqlite

loaf bundle create sqlite-backend --tag sqlite --tag state
loaf bundle show sqlite-backend
loaf bundle add sqlite-backend SPEC-040
loaf bundle remove sqlite-backend IDEA-...
```

Tags classify rows. Bundles collect related rows so a user can ask for "everything about SQLite backend state" without depending on filename prefixes, branch names, or one canonical parent artifact.

### Sessions And Reports

```bash
loaf session list
loaf session show <session>
loaf session log "decision(scope): ..."
loaf session enrich
loaf session report <session>

loaf report generate session <session>
loaf report generate triage
loaf report generate release-readiness
loaf report list
```

Session journal files may remain as generated or compatibility views during migration. The target is structured session rows plus generated reports, not hand-maintained Markdown as the routing database.

`loaf session log` should write a `journal_entries` row even if no session row can be resolved. Known context is stored in nullable columns or relationship rows; unknown context stays null and can be correlated later.

### Exports

```bash
loaf state export spec SPEC-040 --format markdown
loaf state export session <session> --format markdown
loaf state export triage --format markdown
loaf state export release-readiness --format markdown
loaf state export all --format json
```

Generated exports should include enough provenance to be reviewable without becoming canonical operational state.

## Migration Strategy

### Phase 1: Read-Only Import And Inspection

- Add schema, path resolution, migrations, and read-only import.
- `loaf state migrate markdown --dry-run` reports what would be imported.
- `loaf state status` reports whether the project is Markdown-only, SQLite-ready, or migrated.
- Existing commands keep their current behavior.

### Phase 2: State-Backed Reads

- `list`, `show`, `status`, `trace`, and `housekeeping` read from SQLite when initialized.
- Markdown remains the fallback and source link.
- Tests compare SQLite-backed output against current Markdown-backed fixtures.

### Phase 3: State-Backed Mutations

- `create`, `update`, `archive`, `resolve`, and `link` write SQLite first.
- Ideas, sparks, brainstorms, shaping drafts, and finalized specs move through explicit state transitions rather than journal conventions.
- Markdown frontmatter updates become generated compatibility output or explicit export.
- Hooks and skills stop editing operational frontmatter directly.

### Phase 4: Generated Reports And Cleanup

- Generate review artifacts from SQLite.
- Update skills to point humans/agents at CLI commands and generated exports.
- Retire `TASKS.json` authority. Keep import/export only if needed for compatibility.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| SQLite dependency adds install friction | Medium | High | Require explicit dependency decision; prefer a maintained Node SQLite path compatible with Node 22 and bundled CLI distribution |
| Migration loses meaning from old Markdown | Medium | High | Dry-run import, source links, non-destructive migration, skipped-file reports |
| Agents bypass CLI and edit Markdown anyway | High | Medium | Update skills, add warnings/checks, keep Markdown lifecycle fields generated or marked compatibility-only |
| External artifacts leak internal IDs via generated exports | Medium | High | Run SPEC-038 validators on export commands |
| XDG path makes state harder to find | Medium | Medium | Add `loaf state path`, `status`, `backup`, and clear doctor output |
| Worktree/main-worktree routing conflicts with XDG state | Medium | High | Key database identity from main worktree/project identity per SPEC-036; test linked worktrees |
| Schema overfits current workflow | Medium | Medium | Version schema, keep migration events, avoid premature daemon/TUI assumptions |
| Raw transcripts contain sensitive content | Medium | High | Store summaries/pointers by default, not full raw tool output; document redaction/export controls |

## Open Questions

- [ ] Exact XDG split: should the project database live under `$XDG_STATE_HOME/loaf/` or `$XDG_DATA_HOME/loaf/`?
- [ ] How should project identity be derived for moved repositories: absolute path hash, git remote, git common-dir, explicit project UUID, or a combination?
- [ ] Which SQLite package is acceptable for Loaf's Node 22 CLI distribution?
- [ ] Should Markdown compatibility views be generated automatically after every mutation, or only by explicit export commands?
- [ ] What is the minimum session transcript row shape that works across Claude Code, Codex, OpenCode, Cursor, Gemini, and Amp?
- [ ] Should `TASK-*` remain as local internal task IDs when Linear is active, or should Linear-backed projects only store private mapping rows plus Linear-native task IDs?
- [ ] What redaction policy applies to generated reports that include prompt/response excerpts?
- [ ] Should `loaf state backup` produce a `.sqlite` copy, JSON export, or both?

## Test Conditions

- [ ] `loaf state init` creates a project-scoped SQLite database outside the repository and prints its path without creating secrets.
- [ ] `loaf state path` prints the same path from the main worktree and linked worktrees for the same project.
- [ ] `loaf state migrate markdown --dry-run` imports nothing and reports counts for specs, tasks, ideas, sparks, sessions, reports, relationships, and skipped files.
- [ ] `loaf state migrate markdown --apply` imports current `.agents/` artifacts without deleting or rewriting source Markdown.
- [ ] Import preserves source file paths and enough provenance to trace a row back to the original artifact.
- [ ] `loaf trace` can show spark -> idea -> spec/task -> resolved/exported lineage for an imported fixture.
- [ ] `loaf trace` can show brainstorm -> idea -> shaping draft -> finalized spec -> task lineage for an imported fixture.
- [ ] `loaf session log` writes a journal entry when no session/spec/task can be resolved, preserving observed branch/worktree/harness context as nullable fields.
- [ ] `loaf idea resolve ... --by ...` prevents the same raw idea from resurfacing in triage.
- [ ] `loaf spark resolve ... --by ...` prevents the same spark from resurfacing in triage.
- [ ] Tags classify ideas, sparks, brainstorms, specs, tasks, reports, sessions, and journal entries through a many-to-many table.
- [ ] Bundles can collect rows by tag query and explicit membership, then display the full related set.
- [ ] Existing `loaf task list`, `loaf spec list`, `loaf session list`, and `loaf report list` can read from SQLite after migration.
- [ ] Existing mutation commands write through SQLite when state is initialized.
- [ ] Markdown exports are generated from SQLite and pass SPEC-038 leak validators for external-targeted formats.
- [ ] Generated internal exports may include internal IDs, but are clearly marked internal.
- [ ] No command stores Linear tokens, API keys, or other secrets in SQLite.
- [ ] Tests cover linked git worktrees and prove they resolve to the same project state.
- [ ] `loaf state doctor` detects missing DB, schema mismatch, stale compatibility exports, and Markdown-only fallback mode.

## Priority Order

1. **Track A - Storage foundation.** Path resolution, schema migration table, SQLite open/close helpers, project identity, `state init/status/path/doctor`. Go/no-go: linked worktrees resolve the same DB and no repo file is created.
2. **Track B - Import and read model.** Non-destructive Markdown/TASKS/session/report import plus state-backed `list/show/status/trace` reads. Go/no-go: fixture output matches current CLI reads and `trace` proves relationship value.
3. **Track C - Mutations and triage closure.** State-backed create/update/archive/resolve/link commands for specs, tasks, ideas, sparks, brainstorms, shaping drafts, sessions, and reports. Go/no-go: resolved ideas/sparks/brainstorms no longer resurface and source provenance remains intact.
4. **Track D - Generated exports.** Markdown/JSON report generation for session, triage, spec/task snapshots, PR/release readiness. Go/no-go: exports are deterministic, reviewable, and pass boundary validation.
5. **Track E - Skill and hook migration.** Update workflow skills and hook paths to use CLI state commands instead of Markdown/frontmatter mutation. Go/no-go: source skills no longer instruct agents to manually mutate operational frontmatter.

## Success Metric

After SPEC-040 ships, Loaf can answer these without grep-based inference:

- Which task/spec resolved this idea?
- Why is this spark no longer in triage?
- Which brainstorms, ideas, sparks, shaping drafts, specs, and tasks share the same classification bundle?
- Which journal entries were observed during a branch/worktree/session even if no session row existed yet?
- Which sessions, reports, tasks, commits, and external IDs relate to this spec?
- Which generated report was produced from which state?
- Which backend mapping connects a private Loaf identity to an external system identity?

Markdown remains useful, but it stops being the operational database.
