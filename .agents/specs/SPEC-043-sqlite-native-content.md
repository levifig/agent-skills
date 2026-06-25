---
id: SPEC-043
title: "SQLite-Native Artifact Bodies, Retrieval & Search"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-B)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
created: 2026-06-22T08:56:30Z
status: complete
branch: feat/sqlite-native-content
---

# SPEC-043: SQLite-Native Artifact Bodies, Retrieval & Search

## Problem Statement

SPEC-040 centralized operational **metadata** in one global SQLite database but left artifact
**bodies** as in-tree `.agents/<type>/*.md` files, indexed by a `sources` pointer (`path`+`hash`)
and read from disk (`readImportedSourceBody`, `internal/state/task_show.go:166`). Consequences:
bodies stay branch/worktree-variant and non-cross-project; there is **no search** of any kind;
writing a `.md` registers nothing in state (proven: this session's report was absent from
`loaf report list`); `report` has no `show`; and `plan`/`handoff`/`council` are stubbed
`unsupported artifact kind` (`internal/state/markdown_migration.go:361`). This spec is the
**additive, non-breaking core** of the WS-B state-content model: it makes SQLite *able* to be the
source of truth for bodies and adds retrieval + search — **without removing any in-tree files**.
The git-render/finalization layer is SPEC-044; the breaking ephemeral cutover is SPEC-045; `docs/`
Tier-2 indexing is SPEC-046; status-vocabulary unification is SPEC-049.

## Strategic Alignment

- **Vision:** Advances *Structured Execution* and "mechanical enforcement, not a prompt library" —
  bodies and journals become queryable state, serving the "no lost context" pillar.
- **Architecture / CLI-as-protocol:** Fits `docs/ARCHITECTURE.md:55-57` (one XDG-global,
  project-partitioned SQLite DB). Generalizes the existing in-SQLite body precedent
  `session_state_snapshots.content`.
- **Supersedes (intent, realized non-breakingly):** SPEC-040:160 ("keep authored prose in files
  unless the row is the durable record") — for Loaf-managed artifacts the **row becomes able to be
  the durable record**. SPEC-043 does this **dual-source**: the existing markdown import path
  stays; the in-tree `.md` remain authoritative until SPEC-045 cuts over. Nothing is deleted here.
- **Coordinates with:** **SPEC-044** (render/finalization — out of scope here), **SPEC-045**
  (ephemeral cutover/removal — out of scope here), **SPEC-046** (`docs/` indexing), **SPEC-049**
  (status unification — explicitly out of scope here), **SPEC-048** (session convergence; sessions
  become SQLite-bodied), **SPEC-041** (adopt its `PLAN-*` semantics for the `plan` entity; do not
  re-derive). Honors **SPEC-038** (committed internal renders may keep IDs; external exports pass
  `ValidateExternalMarkdownExport`).

## Solution Direction

- **Body store.** Generalize `session_state_snapshots.content` into the shared-contract
  `artifact_bodies` table for Loaf-managed entities. `sources` becomes **provenance-only**;
  `body_source_id` FK stays **nullable**. **Precedence (dual-source, non-breaking):** if a SQLite
  body exists for an entity, it wins; otherwise fall back to the `.md` via `body_source_id`. So
  existing entities keep working unchanged, new ones can be SQLite-bodied.
- **Un-stub `plan`/`handoff`/`council` storage** (`markdown_migration.go:361`) so the live
  `handoff`/`council` skills have somewhere to land; adopt SPEC-041's `PLAN-*` definition for
  `plan`.
- **Uniform verbs** across entities: `new / edit / show / list / link`; add **`report show`** and
  **`brainstorm capture`** (both missing today).
- **Body-edit UX** (the make-or-break input channel — see Resolved Decisions): `--body-file <path>`
  / `--body -` (stdin, agent-primary), `$EDITOR` round-trip (human, mirrors `git commit`),
  `--message` for one-line ephemerals.
- **Write-side enforcement hook** so an artifact cannot be written-but-unregistered (the behavioral
  contract that stops free-handing `.md`). The auto-generated `cli-reference` skill is regenerated
  as a build step (it documents verbs, it is not the contract).
- **Search (`loaf search`)** over Tier-1 bodies + journal entries via **FTS5**, which is compiled
  into the embedded `ncruces/go-sqlite3` SQLite and reached through `CREATE VIRTUAL TABLE … USING
  fts5(…)` — **no Go import, no `go.mod` change** (verified: `ext/fts5` does not exist as a package;
  README lists full-text search as a built-in capability; a `CGO_ENABLED=0` binary runs `MATCH`).

## Resolved Decisions (load-bearing — settled in-spec, not deferred)

1. **Body-edit input channel:** support all three — `--body-file`/`--body -` (stdin), `$EDITOR`
   round-trip, `--message`. Without an input channel, `new`/`edit` are untestable for multi-line
   bodies and agents route around the CLI.
2. **Body store shape:** use the shared-contract `artifact_bodies` table:
   `id`, `project_id`, `entity_kind`, `entity_id`, `body_kind`, `content`, `content_hash`,
   `source_id`, `created_at`, `updated_at`, with
   `UNIQUE(project_id, entity_kind, entity_id, body_kind)`. Do not add per-entity body columns.
3. **`sources` fate:** provenance-only; keep `body_source_id` nullable; `sources` stores path/hash/
   line/import metadata only and never stores body content.
4. **FTS maintenance:** external-content FTS5 with **Go-side upserts in the same code path as body
   writes** (single source of truth, no triggers to drift).
5. **Search defaults:** `loaf search` queries the current project by default; `--all-projects` is
   explicit. Tier-1 results are entity-addressed, JSON includes a `tier` discriminator, and snippets
   must redact or omit planted secret-like content.
6. **Body CLI precedence:** explicit body flags win in this order: `--body-file <path>`, `--body -`,
   `--message <text>`, then `$EDITOR` when no non-interactive body input is supplied. Non-UTF8 input
   is rejected before it reaches SQLite or FTS.
7. **Status vocabulary:** **unchanged** here. SPEC-043 only preserves existing per-entity
   validation; unification is SPEC-049.

## Scope

### In Scope
- Track-0 schema migration: body store; `sources`→provenance (FK nullable); `plan`/`handoff`/
  `council` tables; FTS5 virtual table(s); update `status.go` `entity_kind` allowlists + the
  `local_entities` CTE for the new entities (else `loaf state doctor` mis-sees them).
- Bodies-in-SQLite **write/read path** for Loaf artifacts (dual-source precedence); `report show`;
  `brainstorm capture`; uniform `new/edit/show/list/link`.
- Body-edit UX; Write-side enforcement hook; `cli-reference` regeneration build step.
- `loaf search` (FTS5) over bodies + journals.
- Rewire the ~17 `internal/state` files that read bodies from disk to the dual-source accessor.

### Out of Scope
- Git render, finalization commit, drift gate (**SPEC-044**).
- Removing in-tree `.md` / ephemeral cutover (**SPEC-045**).
- `docs/` Tier-2 indexing (**SPEC-046**).
- Status-vocabulary unification (**SPEC-049**).
- Session-skill convergence beyond storage (**SPEC-048**); `/shape` `PLAN-*` routing (**SPEC-041**).

### Rabbit Holes
- SQLite as a full version/document store — store current body + provenance, not VCS history.
- FTS as a backdoor for raw transcripts (honor SPEC-040's redaction boundary — see Risks).
- Over-generalizing accessors — one dual-source body reader, not per-entity special cases.

### No-Gos
- No new module dependency (FTS5 is the existing driver's built-in; only `ext/unicode` for the
  tokenizer would be a new import — flag if needed).
- No file deletion and no status-vocab change in this spec (those are SPEC-045 / SPEC-049).
- Don't break `CGO_ENABLED=0`; don't write repo markdown as a side effect of mutations.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| FTS5 virtual table fails under `CGO_ENABLED=0` | Low | High | Track-0 smoke: `CREATE VIRTUAL TABLE … USING fts5` runs + `govulncheck`; verified feasible already |
| Write concurrency on the single global DB drops writes | Med | High | `busy_timeout`+WAL, busy-retry in the journal write path, chunk long migrations, concurrency stress test; note Track-0 migration blocks **all** projects |
| FTS indexes secrets/PII in bodies, queryable cross-project | Med | High | Decide indexed-vs-stored fields; redaction/exclusion on ingest; planted-secret negative test; honor `secret_boundary_test.go` naming guard |
| Body migration loses content | Low | High | Two-guarantee parity: byte-exact archival backup (raw bytes + SHA-256) **and** faithful round-trip; never gate on lossy re-render |
| Unbounded body TEXT in global DB (pasted logs) | Med | Med | Soft size limit + on-disk-with-`sources`-pointer escape hatch; reject non-UTF8 on ingest |

## Open Questions
- [x] Body column vs `bodies` table: use `artifact_bodies` per shared-contract lock.
- [x] FTS field set + ranking + snippet/redaction: index entity kind/id/body kind/content plus
      journal entry type/scope/message; start with SQLite `bm25` ordering and snippets that omit or
      redact secret-like content; default scope is current project.
- [x] `--body` stdin vs `--body-file` ergonomics: support both; `--body-file` has highest
      precedence for file-backed multi-line input, `--body -` is the agent-primary stdin path.

## Test Conditions
- [x] A spec/report created via `loaf <entity> new` stores its body in SQLite; `loaf <entity> show`
      displays it with **no in-tree file present**; a multi-paragraph body round-trips byte-exact.
- [x] Existing entities with only a `.md` still `show` correctly (dual-source fallback) — **nothing
      regresses**; `git status` shows no deletions.
- [x] `loaf search "<term>"` returns hits across ideas/sparks/sessions/specs/reports + journals,
      including a report body `loaf report list` cannot surface today.
- [x] Edit-then-search: an old term stops matching and a new term matches after `loaf <entity> edit`.
- [x] `report show` and `brainstorm capture` exist; `plan`/`handoff`/`council` have SQLite storage
      and appear correctly in `loaf state doctor`.
- [x] `CGO_ENABLED=0 go build` + `govulncheck` pass with FTS5 enabled.
- [x] A planted secret in a body is excluded from / redacted in FTS results (privacy test).
- [x] Concurrency stress: parallel `loaf session log` + a long write transaction do not drop
      journal writes.

## Priority Order

Tracks ship in order; non-breaking throughout. Drop from the end if scope tightens.

1. **Track 0 — Body-store schema + FTS5.** Migration: `artifact_bodies`,
   `artifact_search`, `plan`/`handoff`/`council` tables, indexes, schema docs, and `status.go`
   allowlist/CTE updates. *Go/no-go:* `CREATE VIRTUAL TABLE … USING fts5` runs under
   `CGO_ENABLED=0`; migration docs mirror executable SQL; `go test ./internal/state` is green.
   *(non-breaking)*
2. **Track 1 — Dual-source body access + import backfill.** Shared read/write accessor,
   Markdown-import population of `artifact_bodies`, source fallback when no SQLite body exists, and
   Go-side FTS upserts/deletes in the same transaction. *Go/no-go:* imported Markdown bodies
   round-trip byte-exact; existing `.md` entities unaffected; no repo files are deleted or written
   by state mutations. *(non-breaking)*
3. **Track 2 — Body write UX + missing entity verbs.** `new/edit/show/list/link` body paths for
   body-capable entities; `report show`; `brainstorm capture`; first-class `plan`/`handoff`/
   `council` storage; CLI reference regeneration. *Go/no-go:* create→show works with no in-tree
   file; multi-paragraph bodies round-trip; body input precedence is covered by tests.
   *(non-breaking)*
4. **Track 3 — Write-side registration enforcement.** Harness-portable hook/check that warns or
   blocks direct `.agents` artifact-body writes that bypass SQLite registration, without blocking
   generated durable renders or non-artifact docs. *Go/no-go:* direct unregistered body write is
   caught; generated/source-controlled exceptions are explicit and tested. *(non-breaking)*
5. **Track 4 — Tier-1 search.** `loaf search` (FTS5) over SQLite bodies + journals; current-project
   default, `--all-projects`, JSON `tier`, ranking, snippets, and planted-secret redaction.
   *Go/no-go:* correct cross-entity hits incl. a body `loaf report list` cannot surface today;
   edit-then-search updates the index. *(non-breaking)*

**First shippable PR = Tracks 0-4 in this spec branch** — kills the motivating pain ("no search";
"written report invisible to state") with zero file removal, zero status change, zero render.
Everything irreversible lives in SPEC-044/045/049.

## Coordination notes (named, not solved here)
- **SPEC-035 / `TASKS.json`:** tasks remain **dual-source** in SPEC-043 (no removal). Retiring the
  `TASKS.json` write/rebuild subsystem (single source = SQLite) is **SPEC-045** cutover work.
- **SPEC-029 / librarian:** enrichment currently edits the session `.md`. Retargeting enrichment to
  `journal_entries` rows is **SPEC-048** convergence work; SPEC-043 only provides the storage.
- **SPEC-037:** "Durable" here means **storage-tier** (the row can be the source of truth), **not**
  immutable — specs remain mutable per SPEC-037.
