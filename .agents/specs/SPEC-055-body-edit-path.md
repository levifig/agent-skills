---
id: SPEC-055
state_id: "spec:351cba440e08a4678fb027ec"
status: in_progress
title: Sanctioned Artifact Body-Edit Path
---

# SPEC-055: Sanctioned Artifact Body-Edit Path

## Problem Statement

After the SQLite cutover (SPEC-043/044/045), artifact bodies live in `artifact_bodies`
and reach git only as deterministic renders via `loaf <entity> finalize`. Two failClosed
gates protect that model: the PreToolUse `artifact-body-write` guard (`check.go:469`) blocks
raw `Edit`/`Write` to `.agents/` bodies, and the `render-drift` gate (`check.go:281,289`)
blocks committing a render that isn't byte-identical to its deterministic self-render. **Both
redirect the user to `loaf <entity> edit`, then `loaf <entity> finalize` — but no `edit` verb
exists**, and until this spec there was also **no `loaf spec new`** create path. `loaf spec`
exposed only list/show/render/finalize/archive; generic entities expose new/show/list/link.
`check.go:567` even emits the literal placeholder `loaf <entity> <verb> --body-file <path>`.

The result: **the only sanctioned ways to author or change an artifact body were commands
that did not exist** — the gates were dead ends. This directly blocked two conformance fixes
(SPEC-040 cutover amendment notes / G3, SPEC-053 signoff correction / G7) and pointed users at
phantom commands. It is the keystone gap from the SPEC-043..054 conformance review (TASK-407),
discovered the hard way: authoring this very spec was blocked by the gap it documents.

## Strategic Alignment

- **Architecture (ADR-016):** Directly serves the trichotomy's "markdown is a render, not a
  store" rule — it makes the SQLite body the *editable source of truth* with git renders
  derived on demand, closing the loop SPEC-043/044 opened. Without create+edit verbs, "the row
  is the source" is true but unusable.
- **Architecture (ADR-013):** Edits target project-scoped SQLite state; no new file-location
  semantics.
- *No VISION/STRATEGY docs exist in-repo; ADRs are the governing anchors.*

## Solution Direction

Provide first-class `new` (create) and `edit` (mutate) verbs that **write the SQLite artifact
body only** and compose with the existing `finalize` verb for the git render — never writing
`.agents/` files directly (so they route *through* the body-write gate instead of tripping it).
Input via the `new` flag triad (`--body-file` / `--body -` / `--message`) plus an `$EDITOR`
fallback when no flag is given. On first edit of a body that exists only as a legacy source
`.md` (`has_body:false`), import the source body into SQLite first so nothing is lost. Surface
a `has_body` signal in the show read-models so the import decision and tests have a
deterministic signal. Replace the placeholder redirects with concrete, per-kind invocable
commands.

Workflow stays **two-step** (`new`/`edit` → `finalize`): the verb keeps the SQLite body
authoritative; `finalize` writes the deterministic render explicitly. The stored body is
**frontmatter-free prose** (metadata lives in entity columns; `markdownArtifactBodyContent`
already strips frontmatter at `artifact_body.go:246`), so import and re-render must be lossless
and idempotent.

## Scope

### In Scope
- **`new` (create) verb for spec** — the sanctioned SQLite-native create path (implemented as
  the Track 1 bootstrap; this spec was authored via it).
- `edit` verb for **spec** and **report** (Track 1), then the generic prose kinds
  **plan/council/handoff/idea/brainstorm/draft** (Track 2), via a shared edit helper.
- Input: `--body-file <path>` / `--body -` (stdin) / `--message <text>`, plus `$EDITOR`
  launched on the current body when no flag is given.
- Import-on-first-edit: when `has_body:false`, import the legacy source body into SQLite
  before applying the edit; no content lost.
- Surface `has_body` in spec/report/generic show read-models.
- Two-step composition with `finalize`; byte-exact round-trip through `ReRenderDurableRender`.
- Replace placeholder redirects (`check.go:281,289,567`) with concrete per-kind commands.
- Conflict detection: if a legacy `.md` diverges from the imported SQLite body, **refuse with
  a clear message** (re-import or discard) — never silently drop either side.
- Tests: create, edit, import-on-first-edit, byte-exact round-trip, redirect concreteness,
  conflict-refusal, gate-non-self-trip, `$EDITOR` path.

### Out of Scope
- Changing the durable render contract / `RenderDurableDocument` format (SPEC-044 owns).
- Editing **tasks** (`loaf task update`) and **sessions** (`loaf session log`) — their
  redirects already name real verbs.
- **sparks** (capture-only one-liners) — excluded.
- Weakening the body-write gate; a TUI multi-field editor; one-sweep backfill of all legacy
  bodies; Linear body editing (SPEC-039 territory).

### Rabbit Holes
- A general interactive spec/field editor beyond body prose.
- Auto-resolving DB↔file divergence with merge logic — detect and refuse instead.
- Eager backfill of every legacy source body — import stays lazy, per-edit.

### No-Gos
- `new`/`edit` MUST NOT write `.agents/` files directly (would self-trip
  `artifactBodyWriteAllowed`); they mutate SQLite, `finalize` writes git.
- Do NOT ship partial-kind coverage that leaves a `<entity>`/`<verb>` placeholder in any
  emitted redirect.
- Do NOT strip or mutate renderer-unmodeled content silently on import (lossless or explicit
  error).

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Byte-exact round-trip fragility (failClosed drift gate hard-blocks any import↔render non-idempotency) | High | High | Round-trip tests are a ship gate; treat any non-idempotency as a blocker before merge |
| Import-on-edit data loss (renderer-unmodeled content) | Med | High | Lossless-import guarantee or explicit unmodeled-content error; test |
| Gate self-trip (verb writes files) | Med | High | Verbs mutate SQLite only; "verb does not trip the body-write gate" test |
| Partial-kind coverage re-creates the dead end | Med | Med | Track 2 covers all prose kinds; per-kind redirects; test asserts no placeholder remains |
| CLI surface bloat / flag-semantics drift from `new` | Low | Med | Single shared body-input + edit helper across kinds |
| Markdown-only / invalid DB state | Low | Med | `new`/`edit` require SQLite state (mirror `sqliteStateRequiredError`) |

## Open Questions
- [ ] Exact `has_body` field name/location across `SpecDetail`, `ReportDetail`, generic entity show. *(Track 1 added `HasBody` to `SpecDetail`.)*
- [ ] `$EDITOR` temp-file mechanics + behavior when `$EDITOR` is unset (fall back to requiring a flag).
- [x] Create gap for specs — **Resolved:** `loaf spec new` added as the Track 1 bootstrap (sanctioned SQLite-native create); this spec was created via it, dogfooding the fix.

## Test Conditions
- [ ] `loaf spec new <slug> --title T --body-file <f>` creates a queryable SQLite spec row with body; `loaf spec show --json` returns it with `has_body:true`; no `.agents/` file written before `finalize`.
- [ ] `loaf spec edit SPEC-XXX --body-file <f>` updates the SQLite body; `loaf spec show --json` then returns the new body and `has_body:true`.
- [ ] Editing a `has_body:false` spec imports the legacy source body first (no content lost); show reports `has_body:true` afterward.
- [ ] `new`/`edit` then `loaf spec finalize` produces a render that passes the drift gate (not `Blocked`); re-render via `ReRenderDurableRender` is byte-identical.
- [ ] A body written via `new`/`edit` survives `finalize → ReRenderDurableRender` byte-exact (no frontmatter/whitespace/newline drift).
- [ ] Drift-gate findings and `artifactBodyWriteCommand` emit a concrete invocable command for every supported kind; a test asserts no literal `<entity>`/`<verb>` remains for those kinds.
- [ ] `loaf report edit` and a generic `loaf plan edit` behave consistently with `loaf spec edit`.
- [ ] Raw `Edit`/`Write` to a finalized `.agents/` body is still blocked, and the emitted redirect now names the real verb (negative path).
- [ ] Editing when a legacy `.md` diverges from the SQLite body refuses with a clear message; neither side is silently lost.
- [ ] `loaf spec edit` with no body flag opens `$EDITOR` seeded with the current body; saving applies it; flag-based edits are unaffected.
- [ ] `new`/`edit` do not trip `artifactBodyWriteAllowed` (they mutate SQLite, not files).

## Priority Order

Tracks ship in order. If scope needs cutting, drop from the end.

1. **Track 1 — `loaf spec new` (DONE, bootstrap) + spec/report `edit`** + import-on-first-edit, `has_body`, two-step finalize round-trip, conflict detection, concrete redirects for spec/report. *Go/no-go:* round-trip byte-exact, drift gate green, spec/report redirects concrete, conflict-refusal works. **This unblocks G3/G7 and kills the phantom-command dead end for durable docs.**
2. **Track 2 — generic prose kinds** (plan/council/handoff/idea/brainstorm/draft) `edit` + per-kind concrete redirects. *Go/no-go:* each kind round-trips; no `<entity>`/`<verb>` placeholder remains anywhere.
3. **`$EDITOR` interactive UX** (can fold into Track 1 or drop to flags-only if scope tightens — flags alone unblock everything).

## Addendum — Bootstrapping (resolved in-run)

Authoring this spec surfaced that there was **no `loaf spec new`**, and a direct file Write of
a new spec body to `.agents/specs/` is blocked by the failClosed `artifact-body-write` hook
(SPEC-043..054 predate the hook, `4e04622c`). So the "no sanctioned body-write path" gap
covered **create**, not just **edit**. Per the dogfooding directive, the fix was implemented
in-run: `loaf spec new` (the sanctioned SQLite-native create path) was added, and **this spec
was created with it** rather than by bypassing the gate. Two adjacent gaps were filed during
the same loop: no `loaf spec delete`/`loaf project delete` (cannot remove test/orphan entities
from the shared global DB), and global-DB test isolation (smokes pollute production via
`XDG_DATA_HOME`).

<!-- loaf:render kind=spec contract=durable-doc-v1 -->
