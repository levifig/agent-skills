---
id: SPEC-054
title: "Rich Artifact Entity Model & Export-Format Contract"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-B)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
  - id: 18a7629a-8146-44dd-a787-82f19edb9264
    role: referenced
    note: "gridsight thermonuclear-pipeline migration session — worked example + motivating analysis"
created: 2026-06-24T09:30:00Z
status: drafting
branch: feat/rich-artifact-entity-model
governed_by: ADR-016
---

# SPEC-054: Rich Artifact Entity Model & Export-Format Contract

## Problem Statement

SPEC-043 stores a report as a single **body** (a text blob — "option A"). But the high-value
artifacts Loaf produces — code reviews, audits, the deep-evaluation report from this very session,
the gridsight "thermonuclear" pipeline output — are **finding-shaped**: they decompose into
findings (dimension, severity, confidence, location) and verdicts (confirmed/refuted/partial,
rationale). You cannot `filter`, `rank`, or `export` a markdown blob; the stated goal — *"list and
filter and access and print and export holistically"* (`loaf findings list --severity critical
--status confirmed --export csv`) — only works if the substance is **rows**. Loaf's entity model
today (`session/spec/task/idea/report/relationship`) is too thin to express `report → finding →
verdict`, and has no `run` for resumable orchestration. The migrator's "99 skipped files"
(councils/drafts/plans) is the same thinness. Per **ADR-016** (artifact storage trichotomy), this
is the "noun" layer that must be modeled as queryable rows.

## Strategic Alignment

- **Governed by ADR-016** (nouns→SQLite, verbs→git, markdown→render). This spec implements the
  *noun-depth* half of that decision; the verb rule (generators stay in git/loaf with a provenance
  pointer) is honored here and enforced in SPEC-050/053.
- **Depends on SPEC-043** (the body store + dual-source accessor + FTS5). SPEC-054 adds typed child
  entities on top of 043's foundation.
- **Coordinates with:** **SPEC-044** (durable-doc git render + drift gate — *committed* renders) vs
  this spec's **ad-hoc `--format` export** (rows → md/json/csv/html on demand); the two share a
  renderer but differ in purpose. **SPEC-028** (reports become parent rows + findings, superseding
  the blob model). **SPEC-053** (the gridsight artifact migration is a consumer; this spec defines
  the target schema). Honors **SPEC-038** (CSV/JSON/MD exports run through
  `ValidateExternalMarkdownExport` for external audiences; internal keeps IDs).
- **Worked examples (two, cross-project):** the gridsight thermonuclear report (26 row-shaped JSON
  finding/verdict files + 282 markdown renders + 8 generator scripts) and this session's
  deep-evaluation report (findings with severity + confirmed/refuted verdicts). Both are already
  forward-compatible: the substance was emitted as structured data *before* being rendered to prose.

## Solution Direction

- **New noun entities (rows):**
  - `finding` — child of a report (or review/run): `dimension`, `severity`, `confidence`,
    `status`, `location` (path:line ref), title + narrative body.
  - `verdict` — adjudication of a finding (or claim): `confirmed | refuted | partial`, rationale,
    optional reproduction notes.
  - `run` — resumable orchestration: `generator_ref` + `version/hash`, state, stints/steps,
    timestamps. The **provenance pointer** ADR-016 requires — records *which generator+version*
    produced outputs, **never the generator code itself**.
- **Report decomposition (option C):** a report becomes a parent row + `finding`/`verdict` rows,
  with narrative as text fields, and an **optional cached rendered markdown** on the report row for
  instant `loaf report show`. Markdown stops being the artifact and becomes a `--format` target.
- **Unified export-format contract:** `--format md | json | csv | html` across queryable entities
  (`loaf findings list --severity critical --status confirmed --export csv`). Rows render on
  demand; this is the "render subsystem" that turns *stored in SQLite* into *print and export*.
- **Loaders:** because finding/verdict data is already row-shaped JSON in real pipelines, provide
  importers that load JSON-shaped findings into rows (the migration is "load 26 JSON files," not
  "parse 282 markdown files").

## Scope

### In Scope
- `finding`, `verdict`, `run` entities: schema (migration on top of SPEC-043's), CRUD, and
  `list`/`show`/`filter`/`link`.
- Report decomposition to option C (parent row + child finding/verdict rows + cached render).
- The unified `--format md|json|csv|html` export contract across queryable entities.
- JSON loaders for row-shaped finding/verdict data (the importer the gridsight migration consumes).
- `run` provenance pointer (generator ref + version/hash), per ADR-016.

### Out of Scope
- The generator/orchestration **code** itself — stays in git or graduates into loaf (ADR-016;
  graduation tracked in SPEC-050 / a future orchestration spec).
- The durable-doc **git-committed** render + finalization + drift gate (**SPEC-044** owns that;
  this spec is on-demand multi-format export, not committed renders).
- SPEC-043's body store (depended on, not modified).
- The actual gridsight project migration (consumes this schema; not Loaf's repo work).
- Status-vocabulary unification (**SPEC-049**) — `finding`/`verdict` get their own validated status
  sets here; cross-entity unification is 049's.

### Rabbit Holes
- Storing generator scripts or `run` step-code in SQLite — forbidden by ADR-016 (No-Go).
- Over-modeling the verdict/finding schema for hypothetical pipelines — model what the two worked
  examples actually need.
- Rebuilding a renderer — share SPEC-044's deterministic renderer for the markdown `--format`.

### No-Gos
- Code in SQLite (ADR-016). `run` stores a pointer + hash, never the body.
- Markdown as source of truth for findings — the rows are the source; markdown is a render.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Finding/verdict schema too rigid for diverse pipelines | Med | Med | Model from the 2 worked examples; keep a freeform `metadata`/body field; iterate |
| Export `--format` duplicates SPEC-044's renderer | Med | Low | Share one deterministic renderer; 054 = on-demand export, 044 = committed render |
| CSV/JSON export leaks internal IDs to external audiences | Low | Med | Route external exports through `ValidateExternalMarkdownExport` (SPEC-038) |
| Scope creep into building the orchestration engine | Med | Med | `run` is a provenance row only; the engine is git/loaf code (out of scope) |

## Open Questions
- [ ] `finding`/`verdict` canonical status sets (coordinate with SPEC-049).
- [ ] Is `run` in this spec, or split to its own (orchestration-provenance) spec? Lean: include the
      provenance row here; defer a full run/stint engine.
- [ ] Cached-render staleness on the report row — regenerate on read vs on write (tie to SPEC-044).
- [ ] CSV schema per entity (columns) and HTML export target (static file vs stdout).

## Test Conditions
- [ ] `loaf findings list --severity critical --status confirmed` returns matching rows across
      reports; `--export csv|json|md` produces the corresponding format.
- [ ] This session's deep-evaluation report imports into queryable `finding`/`verdict` rows; a
      severity/status filter returns the expected subset.
- [ ] A `report show` renders parent + findings + verdicts (from rows), with an optional cached
      markdown for instant display.
- [ ] A `run` row records `generator_ref` + `version/hash` + state; the generator code is **not**
      stored in SQLite (assert no script body column exists).
- [ ] Importing row-shaped finding JSON (the gridsight pipeline's `find.<dimension>.json` /
      `VERDICTS.json` shape) yields finding/verdict rows without parsing the markdown renders.
- [ ] An external-audience `--format` export passes `ValidateExternalMarkdownExport` (no leaked IDs).

## Priority Order

Tracks ship in order; non-breaking (additive entities on SPEC-043's foundation). Drop from the end.

1. **Track 0 — Schema.** `finding`/`verdict`/`run` tables + relationships on top of SPEC-043's
   migration; validated per-entity status sets. *Go/no-go:* migrates cleanly; doctor sees the new
   entities (per SPEC-043's `status.go` allowlist work).
2. **Track 1 — Decompose + CRUD.** Report → parent + finding/verdict rows (option C, cached
   render); `list`/`show`/`filter`/`link`; JSON loaders. *Go/no-go:* the deep-evaluation report
   decomposes and filters correctly.
3. **Track 2 — Export-format contract.** `--format md|json|csv|html` across queryable entities,
   sharing SPEC-044's deterministic renderer; external exports gated by SPEC-038. *Go/no-go:*
   `loaf findings list --severity critical --export csv` round-trips.
4. **Track 3 — `run` provenance.** `run` row + generator pointer/hash (no code body). *Go/no-go:* a
   run records provenance; assert no script-body column. *(Droppable / could split to its own spec.)*
