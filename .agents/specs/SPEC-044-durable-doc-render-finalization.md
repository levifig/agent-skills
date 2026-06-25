---
id: SPEC-044
title: "Durable-Doc Render, Finalization & Drift Gate"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-B)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
created: 2026-06-22T09:13:21Z
status: drafting
branch: feat/durable-doc-render-finalization
related_specs:
  - SPEC-043
  - SPEC-045
  - SPEC-047
  - SPEC-040
  - SPEC-038
  - SPEC-037
---

# SPEC-044: Durable-Doc Render, Finalization & Drift Gate

## Problem Statement

Once SPEC-043 makes SQLite the source of truth for Loaf-managed artifact bodies, durable docs
(specs, reports, and the spec-/report-shaped projections of ADRs) still need to exist as
**PR-reviewable markdown in git** — that is the whole point of keeping them durable rather than
ephemeral. But "render SQLite to a committed `.md`" is harder than it sounds, and the existing
machinery cannot do it safely:

1. **The existing renderer is not deterministic.** `renderSpecMarkdown`
   (`internal/state/export.go:837-893`) emits live task counts
   (`export.go:847`), `Created`/`Updated` timestamps (`export.go:848-853`), and absolute
   project/database paths via `renderMarkdownExportContext` (`export.go:705-727`,
   `export.go:719-724`). `ExportAllJSON` stamps `time.Now()` (`export.go:221`). Re-running a render
   produces a different file every time, so it cannot back a byte-for-byte drift gate.

2. **The proven `dist`/`plugins` gate cannot be copied.** That gate works because the artifacts
   regenerate **in-tree from in-tree source** and CI diffs them with `git diff --exit-code`
   (`.github/workflows/build.yml:50-66`). A render gate has no such source on a fresh CI checkout:
   the global `$XDG_DATA_HOME` SQLite database **does not exist** on a clean clone, so CI cannot
   re-render from the DB and diff against the committed render. A different gate shape is required.

3. **There is no policy for hand-edited renders.** SPEC-043 leaves "hand-edited committed render:
   re-import vs reject" as an open question. Without a decided policy, a reviewer
   who edits a committed render in a PR silently forks truth away from SQLite.

4. **Renderer changes would silently rot every committed render.** Any future change to the render
   template would make every previously committed render "drift" against the new renderer, with no
   way to tell an intentional renderer upgrade from accidental divergence.

This spec owns the render/finalization half of WS-B: the deterministic renderer, the out-of-tree
render store, the finalization commit, and a drift gate that actually works at CI checkout time.

## Strategic Alignment

- **Vision:** Advances *Structured Execution* and the "mechanical enforcement, not a prompt
  library" stance — a render that is byte-deterministic and gate-enforced replaces "trust the author
  not to hand-edit" with an enforced contract. Durable docs stay PR-reviewable so human review and
  git history remain first-class.
- **Architecture / CLI-as-protocol:** The CLI owns the render; the committed `.md` is a projection,
  never a source. Reuses the render layer in `internal/state/export.go` but adds a **new
  deterministic body renderer** alongside the existing summary renderers (those keep their live
  counts/timestamps for human-facing `state export`; the deterministic renderer is a separate code
  path for committed durable docs).
- **Supersedes / Coordinates-with (explicit):**
  - **Coordinates with SPEC-043 (WS-B, bodies):** SPEC-043 owns the body store, retrieval verbs,
    `loaf search`, and the migration. **This spec promotes the render/finalization layer that
    SPEC-043 explicitly declares out-of-scope.** The deterministic renderer, the finalization commit,
    and the render-drift gate are owned here; SPEC-043 should reference SPEC-044 for that work.
    SPEC-043 still owns the body store this renderer reads from — **hard dependency**.
  - **Resolves the render store path + namespacing and hand-edited render policy** that SPEC-043
    leaves out-of-scope — decided below.
  - **Coordinates with SPEC-045 (WS-B, cutover):** SPEC-045 (BREAKING, gated on SPEC-053) removes
    in-tree ephemeral `.md`. SPEC-044 only governs **durable** docs, which stay in git; the two are
    complementary and must agree on which artifact kinds are durable vs ephemeral (durable: specs,
    reports; ephemeral cutover owned by SPEC-045).
  - **Soft CI-ordering dependency on SPEC-047 (WS-A, build/parity):** the drift gate is a **new,
    independent CI workflow step** that does NOT touch the `dist`/`plugins` verifier
    (`build.yml:50-66`) or the parity-matrix work. Written so it can land before or after SPEC-047.
  - **Honors SPEC-038:** committed internal renders (specs/ADRs) may keep `SPEC-*`/`TASK-*` IDs;
    any external export still passes `ValidateExternalMarkdownExport` (`export.go:665-672`). The
    deterministic durable renderer targets the **internal** audience.
  - **Amends SPEC-040:455** (no side-effect markdown): preserved — there is still no
    write-on-every-mutation; the only git write is the deliberate finalization commit, matching
    SPEC-043's framing.
  - **Coordinates with SPEC-037** ("specs are mutable"): a committed render is a **snapshot at
    finalization**, not a frozen artifact; re-finalization re-renders. The render is not the mutable
    source — SQLite is. This reconciles "mutable spec" with "durable committed render."

## Solution Direction

Four pieces, layered:

1. **A deterministic body renderer.** A new render path (distinct from the live-summary
   `renderSpecMarkdown`/`renderSessionMarkdown`) that emits a durable doc's body with **no
   timestamps, no live counts, no absolute paths**, with **locked section/field ordering** and
   stable list ordering. Volatile fields (created/updated, task tallies, DB path) are either omitted
   or sourced from immutable provenance, never `time.Now()` or a live query. Determinism is the
   contract: rendering the same SQLite row twice, or rendering it under two different
   `$XDG_DATA_HOME` homes, yields **byte-identical** output.

2. **An out-of-tree render store.** Renders during normal work go to an **XDG cache** location
   (`$XDG_CACHE_HOME/loaf/renders/`, falling back via the existing Go `PathResolver`),
   **namespaced by project ID and branch** so parallel branches/worktrees don't collide. These are
   ephemeral scratch outputs (`loaf <entity> render`), never committed, never authoritative.

3. **Render templates + a finalization commit.** Render templates define the markdown projection
   for each durable kind (spec, report, ADR-projection). At ship time, a **single finalization
   step** renders the durable docs deterministically and writes them into their git locations
   (`.agents/specs/…`, `docs/decisions/…`) as one reviewable commit — the same
   generated-artifact-committed-with-source pattern Loaf already uses for `dist`/`plugins`. Each
   committed render carries a **renderer-contract-version stamp** (e.g. a trailing HTML comment or
   frontmatter field) recording the renderer version that produced it.

4. **A render-drift gate (self-consistency round-trip) + local pre-push check.** Because the global
   SQLite DB does not exist at CI checkout, the gate cannot re-render-from-DB-and-diff like
   `dist`/`plugins`. Instead it runs a **self-consistency round-trip**: parse the committed render
   back into a body representation, **re-render** it deterministically, and assert **byte-equality**
   with the committed file. Hand edits, stale renderer output, or partial renders fail the round
   trip. The same check runs locally as a **pre-push hook** so drift is caught before CI. The
   renderer-contract-version stamp lets the gate distinguish "this render predates a renderer
   upgrade" (actionable: re-render) from "this render was hand-edited" (rejected) — and lets a
   deliberate renderer change re-render all committed docs in one sweep instead of silently failing
   every file.

**Hand-edit policy = REJECT + redirect.** A render is never a source. If the gate detects a
hand-edited committed render, it **fails** with a message instructing the author to run
`loaf <entity> edit <ref>` (which writes SQLite) and then re-render + re-finalize. The gate never
silently re-imports a hand edit back into SQLite — that would make the render a covert write path
and reopen the dual-source drift this whole program closes.

## Scope

### In Scope
- A **new deterministic body renderer** for durable docs (specs, reports, ADR-projections),
  separate from the live-summary renderers in `export.go`. No timestamps, no live counts, no
  absolute paths; locked ordering.
- **Render templates** for each durable kind (the markdown projection).
- An **out-of-tree render store** under `$XDG_CACHE_HOME/loaf/renders/`, namespaced by project ID +
  branch; `loaf <entity> render` writes there.
- A **single finalization commit** step that renders durable docs into their git locations at ship
  time (`.agents/specs/…`, `docs/decisions/…`).
- A **renderer-contract-version stamp** on every committed render + a sweep command to re-render all
  committed durable docs when the renderer version changes.
- A **render-drift gate** as a **new, independent CI workflow step** implemented as a
  self-consistency round-trip (parse committed render → re-render → byte-diff), plus a **local
  pre-push check** (and `loaf check --hook <id>` parity).
- **Hand-edit = reject + redirect** policy wired into the gate's failure message.
- Determinism tests: render-twice byte-equality and render-across-two-`$XDG_DATA_HOME`-homes
  byte-equality.

### Out of Scope
- The body store, retrieval verbs (`new/edit/show/list/link`), `loaf search`, and the `.agents/*.md`
  body migration — **SPEC-043** owns these (hard dependency).
- Ephemeral-to-SQLite cutover / removal of in-tree ephemeral `.md` — **SPEC-045** (BREAKING).
- `docs/` Tier-2 indexing and cross-project search — **SPEC-046**.
- The `dist`/`plugins` parity verifier and target simplification — **SPEC-047** (WS-A); the drift
  gate must not touch it.
- A TUI / `loaf browse`.
- Deciding the canonical unified status vocabulary (SPEC-043 / SPEC-049).

### Rabbit Holes
- Making the live-summary renderers (`renderSpecMarkdown`, etc.) deterministic in place — leave
  them as human-facing `state export`; write a **separate** deterministic path so live exports keep
  their counts/timestamps.
- Building a Markdown↔SQLite *round-tripping importer* (re-importing hand edits). The round-trip is
  for **verification only** (re-render and diff), never to re-import edits — that is exactly the
  hand-edit path we reject.
- A pretty-diff/three-way-merge UI for drift. Fail with a clear "edit via CLI then re-render"
  message; that's enough.
- Per-field render configurability / themes. One canonical projection per durable kind.
- Trying to mirror the DB at CI checkout (impossible — DB is global/out-of-tree). Use the
  round-trip, not a DB re-render.

### No-Gos
- A committed render is **never** the source and is **never** hand-edited; the gate enforces this.
- The drift gate does **not** modify, extend, or share code with the `dist`/`plugins` verifier
  (`build.yml:50-66`); it is a standalone step.
- No `time.Now()`, live counts, or absolute paths in the deterministic renderer output.
- No silent re-import of hand-edited renders back into SQLite.
- The finalization commit is the **only** git write — no write-on-every-mutation side effects
  (honors SPEC-040:455).
- No new module dependency (renderer is pure Go over the SPEC-043 body store).

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Renderer is subtly non-deterministic (map iteration, locale, line endings) | High | High | Sort all collections explicitly; fixed `\n`; render-twice + two-XDG-home byte-equality tests in CI |
| Round-trip gate can't reconstruct a body well enough to re-render byte-identically | Med | High | Keep render projection lossless w.r.t. its own re-render (verify re-render, not reconstruct-from-prose); store a normalized body the renderer re-emits verbatim |
| Renderer version bump silently fails every committed render | Med | Med | Contract-version stamp distinguishes upgrade-vs-edit; provide a one-command re-render sweep + a single finalization commit |
| Reviewer hand-edits a committed render in a PR | Med | Med | Gate rejects with redirect to `loaf <entity> edit`; local pre-push check catches before CI |
| Drift gate ordering tangles with SPEC-047 CI changes | Low | Med | Independent workflow step; no shared code with `dist`/`plugins` verifier |
| Finalization commit churns docs on unrelated renderer noise | Med | Med | Determinism tests + contract-version gating; finalize only changed durable docs |
| Render store path collisions across branches/worktrees | Low | Med | Namespace XDG cache by project ID + branch; treat as disposable scratch |

## Open Questions

- [ ] Renderer-contract-version stamp location: frontmatter field vs trailing HTML comment vs both
      (must survive markdown round-trip and be invisible-ish in review).
- [ ] Exact set of durable kinds that get committed renders: specs + reports confirmed; do ADRs
      render from SQLite (Tier-1 projection) or stay git-native Tier-2 source (SPEC-046)? Align with
      SPEC-043's two-tier model (ADRs are Tier-2 git-native).
- [ ] How the round-trip stores/normalizes the body so re-render is byte-identical (normalized body
      column vs canonicalization pass on parse).
- [ ] Pre-push check delivery: dedicated git pre-push hook vs `loaf check --hook render-drift`
      invoked from existing hook wiring.
- [ ] Whether `loaf <entity> render` (scratch) and the finalization render share one code path with
      a "commit" flag, or are distinct commands.
- [ ] Finalization trigger: explicit `loaf <entity> finalize` / `loaf ship` step vs a `/ship` skill
      action (coordinate with ship/release workflows).

## Test Conditions

- [ ] Rendering the same durable artifact twice produces **byte-identical** output (no timestamps,
      counts, or absolute paths differ).
- [ ] Rendering the same artifact under two different `$XDG_DATA_HOME` homes produces
      **byte-identical** output.
- [ ] `loaf <entity> render <ref>` writes a markdown file under `$XDG_CACHE_HOME/loaf/renders/`
      namespaced by project + branch, and creates **no** in-tree file.
- [ ] The finalization step writes the deterministic render into its git location
      (`.agents/specs/…` or `docs/…`) as a reviewable change.
- [ ] Each committed render carries a renderer-contract-version stamp.
- [ ] The render-drift gate **passes** when a committed render matches a fresh deterministic
      re-render (self-consistency round-trip).
- [ ] The render-drift gate **fails** when a committed render is hand-edited, with a message
      instructing `loaf <entity> edit` then re-render (REJECT + redirect; no silent re-import).
- [ ] The render-drift gate is a standalone CI step that does **not** invoke or share code with the
      `dist`/`plugins` verifier and passes on a fresh checkout with **no SQLite DB present**.
- [ ] The local pre-push check fails on a hand-edited committed render before push.
- [ ] Bumping the renderer-contract-version flags affected committed renders as
      upgrade-needed (not hand-edited) and a single sweep re-renders them into one finalization
      commit.
- [ ] The deterministic durable renderer output passes `ValidateExternalMarkdownExport` only where
      an external audience is targeted; internal renders may retain `SPEC-*`/`TASK-*` (SPEC-038).

## Priority Order

Tracks ship in order; if scope tightens, drop from the end. Tracks 1–3 are **non-breaking**
(renderer/store/gate add capability without changing existing behavior). Track 4 (finalization
commit behavior, i.e. writing durable renders into git as the shipping flow) is **gated** —
coordinate with SPEC-045 cutover sequencing and ship/release workflows. Hard dependency on SPEC-043
(body store) throughout.

1. **Track 1 — Deterministic renderer + render store (non-breaking).** New deterministic body
   renderer (no timestamps/counts/absolute paths; locked ordering) + render templates + out-of-tree
   XDG-cache render store namespaced by project/branch; `loaf <entity> render`. *Go/no-go:*
   render-twice and two-`$XDG_DATA_HOME`-home byte-equality tests pass; no in-tree file written.
2. **Track 2 — Contract-version stamp + drift gate (non-breaking).** Renderer-contract-version
   stamp; self-consistency round-trip gate as a standalone CI step + local pre-push check; REJECT +
   redirect hand-edit policy. *Go/no-go:* gate passes on matching renders with no DB present, fails
   on hand edits with the redirect message, never re-imports.
3. **Track 3 — Renderer-version sweep (non-breaking).** One-command re-render of all committed
   durable docs on a renderer-version bump; distinguishes upgrade from hand-edit. *Go/no-go:* a
   version bump flags + re-renders affected docs into a single finalization commit.
4. **Track 4 — Finalization commit flow (gated).** Wire the finalization render-commit into the
   ship flow for durable docs (specs, reports). *Go/no-go (sign-off):* durable docs land in git via
   finalization only; coordinates with SPEC-045 cutover and ship/release workflows; no
   write-on-mutation side effects.
