---
id: ADR-016
title: "Artifact Storage Trichotomy — Nouns in SQLite, Verbs in Git, Markdown is a Render"
status: Accepted
date: 2026-06-24
supersedes: null
superseded_by: null
---

# ADR-016: Artifact Storage Trichotomy — Nouns in SQLite, Verbs in Git, Markdown is a Render

## Context

SPEC-040 made one global SQLite database the canonical store for operational **metadata**, with
markdown demoted to compatibility/export. The WS-B program (SPEC-043 and siblings) extends this to
artifact **bodies**. As that work was scoped, two independent sessions — the loaf restructuring
program and a parallel session migrating a large "thermonuclear" code-review pipeline's output into
a project's SQLite — hit the same unanswered question: *what, exactly, belongs in the database, and
what does not?*

That review pipeline made the trap concrete. It produced three different kinds of artifact:

- **26 JSON files** — structured findings, verdicts, dedup/index/state (row-shaped data).
- **282 markdown files** — the report, per-verdict write-ups, maps, critic, fact-check (narrative).
- **8 scripts** — stint runners, dedup/aggregate/synthesis generators (executable code).

Treating all of these as "a report to store" conflates three categories with fundamentally
different properties. Code cannot be diffed, linted, reviewed, or `git blame`d inside a database.
A markdown narrative cannot be queried or filtered. Only the structured rows can be listed,
filtered, and exported. Without a stated rule, implementers will stuff scripts into SQLite, store
reports as opaque blobs, and lose the ability to query the substance — defeating the whole reason
for centralizing state.

## Decision

Every Loaf-managed artifact resolves into exactly one of three categories, each with a fixed home:

1. **Nouns → SQLite rows (the things you query).**
   Entities you `list / filter / show / link / export`: reports, **findings**, **verdicts**,
   sessions, specs, tasks, ideas, sparks, relationships, **runs**. These are the source of truth
   and live as structured rows in the global SQLite database.

2. **Verbs → git, never SQLite (the code you run).**
   Generator scripts, orchestration/stint runners, build tooling. Code lives in git — either as
   project-local tooling or graduated into loaf itself as a real command — because it must be
   diffable, lintable, reviewable, and `git blame`-able. **SQLite stores a provenance pointer**
   (`run → {generator ref, version/hash, timestamp}`), **never the script body.**

3. **Renders → derived on demand (markdown is not a store).**
   Markdown, JSON, CSV, HTML are *projections* produced from the nouns via a `--format` contract.
   Markdown is one output target among several, never the source of truth. A committed markdown
   render (e.g. a spec/ADR in git for PR review) is a deterministic projection guarded by a drift
   check, not an authored original.

The boundary test: **if you would want to query, filter, or relate it → noun (SQLite). If you would
want to diff, review, or execute it → verb (git). If it is a formatted presentation of a noun →
render (on demand).**

## Consequences

### Positive
- A citable rule that prevents code from being stuffed into the database and reports from being
  stored as un-queryable blobs.
- Forces the entity model to be honest: a report is `report → finding → verdict` rows, not a text
  blob (SPEC-054); orchestration is a queryable `run`, not a pile of state files.
- Makes "stored in SQLite" actually deliver the goal (filter/export holistically) instead of just
  relocating opacity.
- Keeps generators reviewable and version-controlled while still recording, in the DB, exactly
  which generator+version produced an output.

### Negative
- Requires a deeper entity model (finding/verdict/run) and a render subsystem, not just a body
  column — more schema and more CLI surface.
- A two-place story for some artifacts (rows in SQLite + generator code in git) that must be tied
  together by provenance pointers, which adds bookkeeping.

### Neutral
- Markdown remains everywhere, but reframed as a `--format` target rather than the canonical store.
- Governs the WS-B program: SPEC-043 (bodies), SPEC-044 (durable-doc git render + drift gate),
  SPEC-050 (orchestration scripts graduate to git/loaf, not the DB), SPEC-053 (migration imports
  outputs + provenance, never code), and SPEC-054 (finding/verdict/run decomposition + the
  `--format` export contract). Extends SPEC-040 ("markdown is compatibility/export"); complements
  ADR-013 (where state physically lives / worktree resolution).

## Alternatives Considered

### Store whole reports (and their generators) as blobs in SQLite
Trivial to migrate, but opaque: you cannot filter findings by severity/dimension, cannot diff or
review a generator, and cannot `git blame` logic. It relocates the artifacts without delivering the
query/export capability that justified centralizing them. Rejected — it is the trap this ADR exists
to forbid.

### Keep everything as files in git (no SQLite bodies at all)
The status quo before WS-B. Loses cross-project querying, branch-immune reads, and full-text
search — the motivations for SPEC-040/043. Rejected.

### A single "documents" table that stores any artifact body as text
Uniform but structureless — it cannot express that a report *has* findings that *have* verdicts, so
the high-value query/filter/export operations remain impossible. Rejected in favor of a typed noun
model (SPEC-054).
