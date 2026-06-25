---
id: SPEC-046
title: "docs/ Tier-2 Indexing & Cross-Project Search"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-B)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
created: 2026-06-22T09:13:21Z
status: complete
branch: feat/docs-tier2-indexing
---

# SPEC-046: docs/ Tier-2 Indexing & Cross-Project Search

## Problem Statement

SPEC-043 establishes the two-tier content model and `loaf search` (FTS5) over **Tier-1**
SQLite-resident bodies (specs, reports, ephemerals) and journals. It declares **Tier-2** —
git-native durable docs (`docs/decisions/` ADRs, `docs/ARCHITECTURE.md`, `docs/STRATEGY.md`,
`docs/VISION.md`, plus the rest of `docs/`) — "git-native source + SQLite-indexed" but declares
`docs/` Tier-2 indexing out-of-scope, leaving the open question *"which branch's docs, and re-index
trigger"* unresolved. SPEC-046 owns it.

That open question is the whole reason this is a separate spec. Tier-1 bodies are branch-immune by
construction (they live in SQLite, project-partitioned). Tier-2 docs are **not**: they are ordinary
git-tracked files that differ between branches and worktrees. ADR-013 makes `.agents/` resolve to
the main worktree (ADR-013:12-14), but `docs/` is **source code, not agentic state** — ADR-013 does
**not** cover it. So "index `docs/`" has no well-defined answer until we say *which checkout's
`docs/`*, *when it gets re-indexed*, and *what happens to the index when the working tree differs
from `HEAD`*. Without that, an index built from one branch silently mis-answers `loaf search` on
another — exactly the branch-variance defect SPEC-043 set out to eliminate, reintroduced through
the Tier-2 door.

Two further questions ride here because they only make sense once Tier-2 is indexed: the output
contract for Tier-2 hits (a file with a real path and line — unlike Tier-1's entity-id addressing),
and whether `loaf search` defaults to the current project or spans all projects in the global DB.

## Strategic Alignment

- **Vision / Architecture:** Completes the "searchable knowledge, no lost context" pillar by
  bringing the *durable design record* (ADRs, ARCHITECTURE, STRATEGY, VISION) into the same
  retrieval surface as operational state, while honoring `docs/ARCHITECTURE.md:57` (one
  XDG-global, project-partitioned SQLite database) — the Tier-2 index is just more rows in that DB.
- **Supersedes / Coordinates-with (explicit):**
  - **Depends on SPEC-043** for all search infrastructure: the FTS5 virtual tables, the
    `loaf search` command surface, and the body/provenance schema. **SPEC-043 declares `docs/`
    Tier-2 indexing out-of-scope; SPEC-046 owns it** and is the authoritative owner of `docs/`
    indexing.
  - **Resolves SPEC-043's open questions** "docs/ indexing strategy: which branch's docs, and
    re-index trigger" — these are answered here, not in SPEC-043.
  - **Coordinates with the Tier-2 ADR home (WS-D / SPEC-050):** the roadmap notes ADR-013:12
    wrongly claims ADRs/knowledge live in `.agents/`; they live in `docs/decisions/` (verified:
    `docs/decisions/ADR-001…ADR-015`). This spec **assumes** ADRs are git-native under `docs/`
    and indexes them there; it does **not** itself rewrite ADR-013 (a companion-ADR / WS-D
    concern). If SPEC-050 corrects ADR-013:12, this spec's index source is already aligned.
  - **Honors ADR-013** by contrast, not extension: ADR-013's main-worktree resolution applies to
    `.agents/` only. `docs/` is git-managed source; this spec defines its **own** worktree/branch
    rule (below) precisely because ADR-013 does not reach it.
  - **Honors SPEC-038:** indexing reads committed/working-tree doc content verbatim; it neither
    rewrites IDs nor exports. The index is a derived read-model, not an external export, so
    `ValidateExternalMarkdownExport` does not apply.
- **Non-breaking.** Adds an index table + extends `loaf search`; introduces no schema removal, no
  file moves, no behavior change to existing commands. Ships as a follow-on after SPEC-043 lands.

## Solution Direction

**Index, never move.** Tier-2 docs stay authoritative on disk in git. SQLite gets a derived,
disposable **doc index** (path, content snapshot for FTS, content hash, indexed-at, the
git ref/worktree it was indexed from, project id). The index is a cache: it can be dropped and
rebuilt from the working tree at any time with no data loss.

**Branch/worktree rule (the core decision):** index the **working tree of the invoking checkout**,
resolved to the **repository the `loaf` command runs in** (current `git rev-parse --show-toplevel`),
**not** the main worktree. Rationale: `docs/` is code; a search run while working on branch X must
reflect branch X's `docs/`. This deliberately diverges from ADR-013's `.agents/` rule because the
content class differs. The index row records the resolved `toplevel` path + the current `HEAD` ref
(via `internal/project/project.go` git probes, reusing `git rev-parse`) so a stale index from
another branch is detectable and re-scoped rather than silently served.

**Re-index trigger:** lazy + verifiable, no daemon.
- **On demand:** `loaf search` checks each candidate doc's on-disk hash against the indexed hash;
  stale/missing docs are re-indexed transparently before results return (bounded by `docs/` size).
- **Explicit:** `loaf docs index [--rebuild]` forces a full (re)scan — for CI, post-`git checkout`,
  or first run.
- **Optional hook:** a `post-commit` (and/or `post-checkout`) enforcement hook may call
  `loaf docs index` so the index tracks the branch automatically; advisory, not fail-closed.
  Whether to wire it by default is an open question (cost vs. freshness).

**Search output contract:** `loaf search` already returns Tier-1 hits by **entity addressing**
(`spec:SPEC-043`, `idea:<id>`, journal entry). Tier-2 hits use **file addressing**:
`path:line` (e.g. `docs/decisions/ADR-013-…md:53`) plus a snippet. The result schema carries a
`tier` discriminator (`tier1`/`tier2`) and a `locator` that is an entity ref for Tier-1 and a
`path:line` for Tier-2, so human output and `--json` both disambiguate the two without guessing.

**Cross-project scope:** default **current project** (the one resolved from cwd); `--all-projects`
widens to every project partition in the global DB. Tier-2 cross-project hits always render the
absolute (or project-relative + project-name) path so a hit from another repo is unambiguous.
The global DB already partitions by project id (`docs/ARCHITECTURE.md:57`), so cross-project
search is a scope flag over existing rows, not new plumbing.

## Scope

### In Scope
- A `docs_index` table (project-partitioned) storing per-doc: path (project-relative), content
  snapshot for FTS, content hash, indexed-from ref/worktree toplevel, indexed-at; plus the FTS5
  virtual table over Tier-2 doc content (reusing SPEC-043's FTS5 setup — no new module dep).
- The branch/worktree resolution rule for `docs/` (working tree of invoking checkout) and the
  staleness-detection + lazy re-index path.
- `loaf docs index [--rebuild]` to (re)scan `docs/` into the index.
- Extending `loaf search` to span Tier-1 bodies/journals **and** Tier-2 docs, with the
  `tier`/`locator` output contract (`path:line` for Tier-2) in both human and `--json` modes.
- `--all-projects` scope flag; default current-project.
- Which `docs/` paths are in scope: `docs/decisions/*` (ADRs), `docs/ARCHITECTURE.md`,
  `docs/STRATEGY.md`, `docs/VISION.md`, and `docs/**/*.md` generally (decide globs in breakdown);
  exclude generated/index files (`docs/decisions/README.md` if purely generated).
- No default git hook in this spec. Freshness is provided by lazy search-time staleness checks plus
  explicit `loaf docs index --rebuild`; a future hook can call the same command if the cost/benefit
  becomes clear.

### Out of Scope
- Any change that makes SQLite the *source* for Tier-2 docs — they stay git-native source
  (that is the entire Tier-2 contract; moving them is SPEC-045's ephemeral concern, not this).
- Tier-1 search mechanics, FTS5 driver setup, body schema — owned by SPEC-043.
- Render/finalization/drift-gate for durable docs — SPEC-044.
- Rewriting ADR-013:12's mis-statement about ADR location (WS-D / SPEC-050).
- A TUI / `loaf browse` (SPEC-043 deferred it; still deferred).
- Indexing non-`docs/` repo content (source code, READMEs outside docs/) — explicitly not a
  code-search tool.

### Rabbit Holes
- **Building a file watcher / daemon.** No background indexer; lazy-on-search + explicit + optional
  git hook is the ceiling. A persistent watcher is a different product.
- **Reconciling working-tree vs `HEAD` divergence perfectly.** Index the working tree as-is; record
  the ref for staleness, do not try to diff staged/unstaged/committed states.
- **Cross-project ranking/relevance tuning.** Use FTS5's default ranking; do not build a scoring
  model. `--all-projects` lists hits grouped by project, not globally re-ranked.
- **Indexing git history.** Only the current working tree is indexed; git is the history store.

### No-Gos
- No new module dependency — Tier-2 FTS5 reuses SPEC-043's `ncruces/go-sqlite3` setup.
- Do not resolve `docs/` to the main worktree (would reintroduce branch-variance for code content).
- Do not treat the index as durable: it must be fully rebuildable from disk (`--rebuild`); never a
  source of truth.
- Do not block `loaf search` on a full re-scan when only a few docs are stale (lazy, bounded).

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Index reflects wrong branch's `docs/` | Med | High | Index the invoking checkout's working tree + record ref; staleness check re-scopes before serving; `--rebuild` escape hatch |
| Stale index after `git checkout`/`rebase` with no hook | Med | Med | Hash-based staleness detection on every `loaf search`; optional `post-checkout` hook; `loaf docs index --rebuild` |
| Lazy re-index slows `loaf search` on large `docs/` | Low | Med | Hash-compare is cheap; stale worktree indexes refresh on demand; explicit `--rebuild` remains available |
| Tier-2 `path:line` collides/confuses with Tier-1 entity refs in output | Med | Med | Explicit `tier` discriminator + distinct `locator` shape in schema and human output |
| Cross-project search leaks paths from unrelated repos confusingly | Low | Low | Default current-project; `--all-projects` groups by project + shows project name + absolute path |
| ADR-013:12 mis-statement makes "where do ADRs live" ambiguous during breakdown | Low | Low | This spec fixes the source of truth (index `docs/decisions/`); SPEC-050 corrects the prose |

## Open Questions
- [x] Re-index trigger default: lazy search-time staleness checks plus explicit
      `loaf docs index [--rebuild]`. No default post-commit/post-checkout hook in this spec.
- [x] Exact `docs/` glob: index Markdown under `docs/**/*.md`, including ADRs and top-level durable
      docs; skip non-Markdown schema/assets and generated index files.
- [x] Default search scope: current project; `--all-projects` is the explicit widening flag.
- [x] Command shape: top-level family `loaf docs index [--rebuild]`; search remains
      `loaf search <query>`.
- [x] Working-tree vs `HEAD`: index working-tree content from the invoking checkout and record
      branch/ref metadata for stale-scope detection.

## Test Conditions
- [x] `loaf docs index` populates `docs_index` from the invoking checkout's `docs/`; a subsequent
      `loaf search "<term-in-an-ADR>"` returns the ADR with a `docs/decisions/…md:line` locator.
- [x] `loaf search` returns **both** Tier-1 hits (entity-addressed) and Tier-2 hits (`path:line`)
      in one result set, each tagged with its `tier`; `--json` carries the discriminator.
- [x] Editing a `docs/` file and re-running `loaf search` reflects the new content **without** an
      explicit re-index (lazy staleness detection refreshes the current worktree index).
- [x] `git checkout`-ing a branch with different `docs/` content and running `loaf search` returns
      results from the **checked-out branch's** docs, not another branch's (working-tree rule).
- [x] `loaf docs index --rebuild` drops and rebuilds the index; results are unchanged vs. a fresh
      index (index is purely derived).
- [x] `loaf search` defaults to the current project; `--all-projects` returns hits across project
      partitions, each labeled with project + an unambiguous path.
- [x] Tier-2 docs are never written/moved by indexing — `git status` stays clean after any
      `loaf search` / `loaf docs index`.
- [x] `CGO_ENABLED=0 go build` stays green (no new dependency; reuses SPEC-043 FTS5 setup).

## Priority Order

Tracks ship in order; all non-breaking. Hard dependency: **SPEC-043 must have landed Tracks 0–2**
(schema + FTS5 + `loaf search`) before any track here can start.

1. **Track 0 — Index schema + scan.** `docs_index` table + Tier-2 FTS5 table; `loaf docs index
   [--rebuild]` scans the invoking checkout's `docs/` (records path, hash, ref). *Go/no-go:*
   `--rebuild` is idempotent; build green under `CGO_ENABLED=0`. *(non-breaking)*
2. **Track 1 — Search integration + output contract.** Extend `loaf search` to span Tier-2 with
   the `tier`/`locator` (`path:line`) discriminator in human + `--json` output. *Go/no-go:* mixed
   Tier-1/Tier-2 result set disambiguates correctly. *(non-breaking)*
3. **Track 2 — Lazy staleness + branch correctness.** Hash-based staleness detection re-indexes
   changed docs on `loaf search`; working-tree resolution returns the invoking branch's docs.
   *Go/no-go:* the branch-switch test condition passes. *(non-breaking)*
4. **Track 3 — Cross-project scope.** `--all-projects` flag + current-project default + path
   disambiguation across partitions. *Go/no-go:* cross-project hits are unambiguous. *(non-breaking)*
5. **Track 4 — Documentation/reference updates.** Update CLI reference, agent help, generated
   outputs, and the native cutover test map so `loaf docs index` and Tier-2 search are documented
   with the same contract the implementation enforces. *(non-breaking)*
