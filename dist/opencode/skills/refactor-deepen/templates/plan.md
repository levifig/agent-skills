# PLAN Template

Minimal artifact shape for `/refactor-deepen` output. A PLAN is leaner than a
SPEC: it captures one deepening proposal end-to-end without the
problem/strategic-alignment/risks scaffolding a feature SPEC carries. Use this
template when terminating the `/refactor-deepen` grilling loop.

**Location:** `.agents/plans/{YYYYMMDD-HHMMSS}-{slug}.md`

The filename **is** the plan's identity. Use the timestamp at write-time and
slugify the title (lowercase, hyphens, no punctuation). PLAN files follow the
same temporal-record naming as `.agents/sessions/`, `.agents/ideas/`,
`.agents/drafts/`, and `.agents/councils/` — write-once snapshots, not
sequentially-numbered contracts. Subsequent deepenings of the same module
write a new file rather than updating an existing one.

## Contents
- Frontmatter
- Required Sections
- Filename
- Lazy Directory Creation
- Example

## Frontmatter

| Field | Required | Notes |
|-------|----------|-------|
| `title` | Yes | One-line description of the deepening, not the candidate name |
| `created` | Yes | ISO 8601 UTC, e.g. `2026-05-02T01:30:00Z` (must match the filename timestamp) |
| `status` | Yes | `drafting` on first write — lifecycle states are deferred to a follow-up spec |
| `spec` | Yes | `SPEC-NNN` if the plan is scoped under a spec; `null` otherwise (do not omit the key) |
| `related` | No | List of related artifact IDs (`ADR-*`, `SPEC-*`, idea filenames, other plan filenames) |

PLAN files do **not** carry an `id` frontmatter field. The filename is the
identity, mirroring sessions and ideas.

## Required Sections

The five sections below are the *minimal shape* defined in SPEC-034. Order is
load-bearing: each section assumes the previous one is settled. Do not reorder.

1. **Candidate** — what's being considered for deepening. Name the **module**
   verbatim (use the canonical glossary term — run `loaf kb glossary check`
   before writing this section). One paragraph; describe the current shape and
   why it reads as shallow (interface surface ≈ implementation surface,
   leaky callers, scattered locality, etc.).

2. **Dependency Category** — exactly one of the four categories from
   [references/deepening.md](../references/deepening.md):
   - `in-process` — code inside the same runtime, no external surface
   - `local-substitutable` — local dependency with a swappable implementation
   - `ports-and-adapters` — true seam with multiple realizable adapters
   - `true-external` — dependency on a system Loaf does not own

   The category determines the seam discipline that applies. State the
   category explicitly, then briefly justify the classification (one or two
   sentences). Plans that span categories are a smell — split the plan.

3. **Proposed Deepened Module** — the new shape. Describe the **interface**
   the module would expose, what its **implementation** would hide, and where
   the **seam** would land. This is the section the parallel
   INTERFACE-DESIGN sub-agents feed; if their three designs converged on one,
   record that one and reference the rejected variants in *Rejected
   Alternatives*. Use the eight-term vocabulary verbatim (Module, Interface,
   Implementation, Depth, Seam, Adapter, Leverage, Locality).

4. **What Survives in Tests** — which test characteristics the deepening
   preserves and which it invalidates. Be concrete: name the tests (or test
   types) that keep passing across the seam unchanged, and the tests that
   must move, be rewritten, or be deleted. This section exists because the
   deletion test (see `references/language.md`) only earns its keep when the
   test surface survives the deepening — if every test breaks, the seam is
   the wrong one.

5. **Rejected Alternatives** — designs explored and ruled out. Include the
   two non-chosen INTERFACE-DESIGN sub-agent outputs, plus any other shape
   the grilling loop surfaced and discarded. Each entry: a one-line
   description and a one-paragraph rationale for rejection. "Rejected
   because the user picked another" is not a rationale — name the load-bearing
   tradeoff.

## Filename

Compute the filename from the current UTC timestamp at write-time and a
slugified title:

```bash
slug="cli-lib-install-deepening"   # lowercase, hyphens, no punctuation
date -u +%Y%m%d-%H%M%S | xargs -I{} echo ".agents/plans/{}-${slug}.md"
```

Example: `.agents/plans/20260502-033000-cli-lib-install-deepening.md`.

The sequential-ID allocation race that an `id: PLAN-NNN` scheme would carry
is gone — there is no shared counter to contend over. Same-second filename
collisions remain theoretically possible if two `/refactor-deepen` runs
write within the same UTC second; in practice this is unlikely at human
typing speed but not impossible at scripted speed. The broader plan-
lifecycle question (list / archive / doctor recognition) is tracked
separately in idea `20260501-231922`.

## Lazy Directory Creation

The `.agents/plans/` directory is created **on first plan write**, never
upfront. Skill logic must `mkdir -p .agents/plans` immediately before writing
the plan file, not at skill load time and not as part of any setup step.

Rationale: a repository that never invokes `/refactor-deepen` should never
acquire an empty `.agents/plans/` directory. Lazy creation keeps tree noise
proportional to actual usage, matching how `.agents/specs/` and
`.agents/sessions/` already behave.

## Linear-Native Mode: Fail Fast

PLAN files are **local-only storage**. Per SPEC-034 line 81, write commands
must fail fast in Linear-native mode rather than silently degrade. The
consuming skill (`/refactor-deepen`) is responsible for:

1. Reading `.agents/loaf.json` and checking `integrations.linear.enabled`.
2. If true: aborting before `mkdir -p .agents/plans` and before any write,
   with the verbatim error
   `"Linear-native plan storage pending artifact-taxonomy spec — local mode only for now."`
3. If false: proceeding with the write as documented above.

This template intentionally does **not** wrap the write in a
defensive shell guard — the gate belongs to the skill's invocation flow,
not to the artifact format. See `../SKILL.md` for the canonical rule.

## Example

Filename: `.agents/plans/20260502-013000-deepen-session-journal-append.md`

```yaml
---
title: "Deepen session journal append into a self-managing module"
created: "2026-05-02T01:30:00Z"
status: drafting
spec: SPEC-034
related:
  - 20260501-231922-plan-lifecycle-cli-doctor-housekeeping
---

# Deepen session journal append into a self-managing module

## Candidate

The session-journal append path currently lives as a free function in
`cli/lib/session.ts` with callers reaching past it to format timestamps and
classify entry types. Interface surface is roughly equal to implementation
surface — every caller that wants to log re-derives the entry-shape rules.

## Dependency Category

`in-process`. The append path has no external surface; all dependencies
(filesystem, clock) are already local-substitutable through existing test
fakes.

## Proposed Deepened Module

A `SessionJournal` module exposing a single `append(entry)` interface.
Implementation hides timestamp formatting, entry-type validation, blank-line
rules, and frontmatter `last_entry` updates. Seam: the public `append`
function. Leverage: every caller drops to one line; locality: blank-line and
PAUSE-header rules live in exactly one place.

## What Survives in Tests

Surviving:
- All existing journal-format tests (entry-type vocabulary, timestamp
  format, blank-line rules) move behind the new interface unchanged.
- The frontmatter-update integration test continues to assert
  `last_entry` is updated post-append.

Invalidated:
- Two tests that assert internal helper signatures (`formatEntry`,
  `classifyType`) must be deleted — those helpers are now private.

## Rejected Alternatives

### Append + Format split (sub-agent variant 2)

Two-method interface (`format`, `append`). Rejected because callers always
want both; splitting them re-exposes the implementation seam the deepening
is meant to hide.

### Streaming writer (sub-agent variant 3)

A `JournalWriter` class with `open`/`write`/`close`. Rejected because the
session journal's write pattern is one-entry-at-a-time on demand, not a
streamed batch — the lifecycle methods would be vestigial.
```
