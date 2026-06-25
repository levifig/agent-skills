---
id: SPEC-045
title: Ephemeral-to-SQLite Cutover
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-B)"
created: 2026-06-22T09:13:21Z
status: drafting
branch: feat/ephemeral-to-sqlite-cutover
source_sessions:
  - id: 20260621-001541-session
    role: shaped
related_specs:
  - SPEC-040
  - SPEC-043
  - SPEC-044
  - SPEC-053
  - SPEC-035
  - SPEC-029
  - SPEC-036
---

# SPEC-045: Ephemeral-to-SQLite Cutover

## Problem Statement

SPEC-043 makes SQLite the source of truth for artifact bodies and proves import
parity, but it stops short of the destructive half: the in-tree ephemeral `.md`
files still exist alongside the SQLite rows. Until they are removed, Loaf carries
**dual-source drift** — two homes for the same content, where one is authoritative
and the other is a stale shadow that agents and humans can still hand-edit. That
shadow is exactly the disease SPEC-040 set out to cure: branch/worktree variance,
non-cross-project bodies, and "writing a `.md` registers nothing."

There are 407 tracked ephemeral files today (`git ls-files`):

| Directory | Tracked files |
|-----------|---------------|
| `.agents/tasks/` | 248 |
| `.agents/sessions/` | 97 |
| `.agents/ideas/` | 53 |
| `.agents/drafts/` | 9 |
| `.agents/sparks/` | 0 |
| `.agents/brainstorms/` | 0 |
| **Total** | **407** |

(The roadmap's "~159" predates accretion; the real removal surface is ~407 and
must be recounted at execution time, not hard-coded.)

This is **Loaf's first destructive operation against tracked content**. The risk
is not theoretical: a botched cutover loses human-authored idea/draft prose and
session history with no clean recovery, and it dangles ~10 in-tree provenance
references (`source:` / `source_sessions:` lines) in surviving specs. The
governing requirement is therefore **reversibility**: byte-exact backup before
any deletion, byte-verification before any deletion, and a tested one-command
rollback. Reversal restores the **original file bytes**, never a re-render.

## Strategic Alignment

- **Vision / Architecture:** Completes the SPEC-040 promise that "Markdown stops
  being the operational database" for ephemerals. Honors the two-tier content
  model: ephemerals (ideas/sparks/sessions/brainstorms/drafts/tasks) become
  SQLite-only; durable docs (specs/reports) stay SQLite-sourced and rendered to
  git (SPEC-043, SPEC-044); Tier-2 `docs/` stay git-native.
- **Supersedes / amends (explicit):**
  - **SPEC-040:172** No-Go "Do not remove current Markdown artifacts until
    import/export parity is proven" — this spec is the deliberate, gated lift of
    that No-Go *for ephemerals only*, executed only after SPEC-043 proves parity.
    Durable specs/reports are NOT removed (they render to git per SPEC-044).
  - **SPEC-040:160** Rabbit Hole "keep authored prose in files unless the row
    itself is the durable record" — for ephemerals the **row is now the durable
    record**; amend SPEC-040:160 to record that the ephemeral exception has been
    exercised (the row is the durable record; the file is gone).
  - **SPEC-040:418-433** Migration Phases 2-4 assume "Markdown remains the
    fallback and source link." After cutover there is no markdown fallback for
    ephemerals; SQLite is the only home. Record this as the terminal state of
    SPEC-040's migration ladder.
  - **SPEC-035 (TASKS.json)** — `.agents/TASKS.json` is still live on disk and is
    a second structured task source. Tasks moving SQLite-only re-creates
    dual-source drift unless TASKS.json is removed in the same cutover. This spec
    resolves SPEC-035 by removing TASKS.json authority: single source = SQLite.
  - **SPEC-029 (JSONL journal sync, archived)** — librarian enrichment that edits
    session `.md` (SPEC-029) has no file to edit after cutover. Retarget
    enrichment to write `journal_entries` rows; the SPEC-029 markdown-edit path is
    retired.
- **Coordinates with:**
  - **SPEC-043** (depends on) — provides the body store, ephemeral SQLite-only
    write paths, and parity import. SPEC-045 cannot start until SPEC-043 Track 1
    (bodies + uniform verbs) and the parity import test land.
  - **SPEC-044** (depends on) — provides the deterministic body renderer and drift
    gate, required so durable docs survive in git while ephemerals are removed,
    and so reversal output can be byte-compared where relevant.
  - **SPEC-053** (HARD GATE) — the breaking-change migration mechanism. SPEC-045
    MUST NOT merge before SPEC-053 ships `loaf install --upgrade` semantics, a
    one-time reversible backed-up state migration, and the deprecation/tombstone
    taxonomy. Cutover is a breaking change to anyone reading/editing
    `.agents/{ideas,sessions,drafts,tasks}/*.md` directly.
  - **SPEC-036 / ADR-013** — `.agents/` resolves to the MAIN worktree. Cutover
    *shrinks* the `.agents/` surface (ephemerals leave) but does NOT delete
    `.agents/` (specs renders, `loaf.json`, durable docs remain). A companion ADR
    records this reduction AND corrects **ADR-013:12**, which wrongly asserts
    "ADRs, knowledge ... all live in one place" inside `.agents/` — ADRs and
    knowledge live in `docs/decisions/` and `docs/` (Tier-2). The companion ADR
    fixes that factual error as part of recording the surface reduction.

## Solution Direction

A **3-phase hard-barrier cutover protocol** — copy, verify, delete — where no
phase proceeds until the prior phase's invariant holds. Each barrier is a refusal
point, not a warning.

1. **COPY (backup + import-confirm).** Import is already done by SPEC-043; this
   phase produces a **byte-exact archival backup** of every ephemeral file slated
   for removal: the raw original file bytes plus a SHA-256 manifest, stored
   out-of-tree (XDG, alongside `loaf state backup`). Backup captures the file as
   committed/on-disk, never a re-render.
2. **VERIFY (byte barrier).** For every file, confirm the SQLite body imported by
   SPEC-043 reproduces the original bytes (or that the original bytes are captured
   verbatim in the backup manifest). The barrier is: **if any file fails byte
   verification, the cutover aborts and deletes nothing.** Verification covers all
   407 (recounted) files; partial verification is not permitted to proceed.
3. **DELETE (`git rm`).** Only after VERIFY passes for the entire set: `git rm`
   the ephemeral files in one staged change, and in the **same change** rewrite or
   tombstone the dangling in-tree provenance references so the repo stays
   internally consistent.

**Reversibility (the spine of this spec):**
- `loaf state restore-ephemerals <backup-id>` (one command) restores the exact
  original bytes from the backup manifest to their original paths and re-stages
  them. Restore writes the **stored bytes**, never a re-render — a re-render is
  not guaranteed byte-identical (`renderSpecMarkdown` emits timestamps/live
  counts/absolute paths and is not deterministic; SPEC-044's deterministic body
  renderer is for durable docs, not ephemeral round-trip restore).
- Restore is **tested** in CI: backup → delete → restore → `git diff` shows the
  restored files byte-identical to pre-cutover.

**Provenance reference handling.** ~10 surviving specs carry `source:` /
`source_sessions:` lines pointing at ephemeral paths/IDs that will dangle. For
each: if the referenced ephemeral is in SQLite, rewrite the reference to the
stable SQLite alias/ID; otherwise tombstone it with a note that the source was
absorbed into SQLite at cutover. SPEC-040:160 and SPEC-040:172 get the
amendment notes described above. This rewrite is part of the DELETE change so the
tree never has a dangling reference at any commit.

**Cross-branch cutover protocol.** Ephemeral `.md` differ per branch (the exact
problem being solved). `.agents/` resolves to the main worktree (ADR-013), so
cutover runs against the **main worktree's** `.agents/`. But other live branches
may carry ephemeral files not present on main. The protocol: (a) cutover lands on
main; (b) a documented merge/rebase procedure handles branches whose ephemeral
files were created after they diverged — those files import into SQLite on the
branch, then the branch drops them on merge; (c) `loaf check` flags any
re-introduced ephemeral `.md` so a stale branch cannot silently resurrect the
dual-source state. The detailed branch-reconciliation steps are decided in
breakdown (see Open Questions).

**TASKS.json removal.** `.agents/TASKS.json` is `git rm`'d in the same cutover,
its authority retired, single source = SQLite (resolves SPEC-035). Backup
captures it like any other ephemeral.

**Enforcement after cutover.** The Write-side hook from SPEC-043 (artifact
written-but-unregistered guard) is extended: a `loaf check` rule **fails** when a
tracked ephemeral `.md` reappears under `.agents/{ideas,sparks,sessions,brainstorms,drafts,tasks}/`,
so the cutover cannot silently regress.

## Scope

### In Scope
- 3-phase hard-barrier cutover protocol (copy → byte-verify → delete) with abort
  semantics: nothing is deleted unless the entire set verifies.
- Byte-exact archival backup: raw original file bytes + SHA-256 manifest,
  out-of-tree, captured before any deletion. Restore = stored bytes, never render.
- `loaf state restore-ephemerals <backup-id>` (one-command rollback) + CI test
  proving byte-identical round-trip (backup → delete → restore → `git diff` clean).
- `git rm` of the recounted ephemeral set (~407 files across
  ideas/sparks/sessions/brainstorms/drafts/tasks, incl. archive subdirs) in a
  single staged change.
- Removal of `.agents/TASKS.json` and retirement of its authority (resolves
  SPEC-035); single source = SQLite.
- Rewrite/tombstone of the ~10 dangling in-tree provenance references
  (`source:` / `source_sessions:`) in surviving specs; amend SPEC-040:160 and
  SPEC-040:172 with cutover-exercised notes.
- Companion ADR: records the ADR-013 surface reduction (ephemerals leave the tree)
  AND corrects the ADR-013:12 factual error (ADRs/knowledge live in `docs/`, not
  `.agents/`).
- Retarget SPEC-029 librarian enrichment from editing session `.md` to writing
  `journal_entries` rows.
- `loaf check` rule that fails when an ephemeral `.md` reappears under the
  ephemeral directories (anti-regression / cross-branch guard).
- Cross-branch cutover procedure documentation + the check that flags
  re-introduced ephemerals.

### Out of Scope
- Bodies-in-SQLite, FTS5 search, uniform verbs, parity import (SPEC-043 owns).
- Deterministic durable-doc render, finalization, drift gate (SPEC-044 owns).
- Removing durable specs/reports from git — they stay (rendered per SPEC-044).
- The breaking-change migration mechanism / `loaf install --upgrade` semantics
  (SPEC-053 owns; this spec consumes it as a hard gate).
- `docs/` Tier-2 indexing & cross-project search (SPEC-046).
- Session-model skill convergence (SPEC-048) — though SPEC-029 enrichment
  retargeting is in scope here because it directly edits a file being removed.
- Rewriting git history to purge old ephemeral bytes — files are `git rm`'d going
  forward; history retains them (and the backup is the authoritative restore path).

### Rabbit Holes
- Re-rendering to "verify" parity. The byte barrier compares stored original bytes;
  it does not trust a non-deterministic re-render. Restore replays stored bytes.
- A universal cross-branch auto-merge tool. Document the manual procedure + the
  regression check; do not build a branch-graph rewriter.
- Purging ephemeral bytes from git history (BFG/filter-repo). Out of scope and
  high-risk; the backup is the recovery mechanism, not history surgery.
- Generalizing restore into a full state time-machine. One command restores this
  cutover's backup set; broader versioning is future work.

### No-Gos
- Do NOT delete any file before its byte verification passes AND the full backup
  manifest is written and self-verified.
- Do NOT restore via re-render — restore writes the exact stored bytes only.
- Do NOT merge before SPEC-053 ships the migration mechanism (hard gate).
- Do NOT remove durable specs/reports or `loaf.json` (SPEC-042 untouched) — only
  ephemerals.
- Do NOT leave a dangling provenance reference at any commit — rewrite/tombstone
  rides in the same change as `git rm`.
- Do NOT delete `.agents/` itself — the surface shrinks, it does not disappear.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Body loss on delete (import incomplete/corrupt) | Med | Critical | Byte-verify barrier aborts before any delete; out-of-tree byte-exact backup + manifest; tested one-command restore |
| Non-deterministic render mistaken for parity check | Med | High | Verification compares stored original bytes, not re-renders; restore replays stored bytes only |
| Dangling provenance refs after `git rm` | High | Med | Rewrite/tombstone the ~10 refs in the SAME change; CI check for dangling `.agents/<ephemeral>/` refs |
| Cross-branch ephemeral resurrection (stale branch re-adds `.md`) | High | Med | `loaf check` rule fails on reappearing ephemeral `.md`; documented merge procedure |
| Cutover ships before migration mechanism exists | Low | Critical | Hard gate on SPEC-053; priority order marks the go/no-go |
| TASKS.json removed but a consumer still reads it | Med | Med | Audit + retire authority in same change; SPEC-035 reconciled; doctor flags presence |
| Recounted file set drifts from spec (407 stale) | Med | Low | Recount at execution; verify set = `git ls-files` of ephemeral dirs, not a hard-coded number |
| SPEC-029 enrichment writes to a deleted file | Med | Med | Retarget enrichment to `journal_entries` rows in this spec's scope |

## Open Questions
- [ ] Backup format: a single tarball + JSON SHA-256 manifest, or per-file copies
      under a backup-id directory? (Recommend tarball + manifest, addressable by
      backup-id, co-located with `loaf state backup` output.)
- [ ] Cross-branch reconciliation: exact merge/rebase procedure and whether
      `loaf` provides a helper or only documents the manual steps.
- [ ] Whether `archive/` subdirectories of ephemeral dirs are imported+removed in
      this cutover or handled as a separate lower-priority pass.
- [ ] Tombstone format for absorbed provenance refs (frontmatter note vs inline
      comment) — must remain SPEC-038-clean for any externally-exported spec.
- [ ] Does restore need to recreate the SQLite rows too, or only the files (with
      re-import as a follow-up)? (Recommend: restore files only; re-import is the
      forward path.)

## Test Conditions
- [ ] Backup of the full ephemeral set produces raw original bytes + a SHA-256
      manifest out-of-tree; the manifest self-verifies (recomputed hashes match).
- [ ] Byte-verify barrier: if ANY ephemeral file fails verification, the cutover
      aborts and **deletes nothing** (observable: `git status` unchanged).
- [ ] After a successful cutover, `git ls-files .agents/{ideas,sparks,sessions,brainstorms,drafts,tasks}`
      returns empty and `.agents/TASKS.json` is gone.
- [ ] `loaf state restore-ephemerals <backup-id>` restores files byte-identical to
      pre-cutover (`git diff` clean against the pre-cutover tree).
- [ ] CI round-trip: backup → delete → restore → byte-diff is clean (one command,
      tested).
- [ ] No surviving spec contains a dangling `.agents/{ideas,sparks,sessions,brainstorms,drafts,tasks}/`
      `source:`/`source_sessions:` reference after cutover; SPEC-040:160 and
      SPEC-040:172 carry the cutover-exercised amendment notes.
- [ ] A companion ADR exists recording the ADR-013 surface reduction and
      correcting the ADR-013:12 `docs/` vs `.agents/` claim.
- [ ] `loaf check` fails when an ephemeral `.md` is (re)introduced under an
      ephemeral directory.
- [ ] SPEC-029-style enrichment writes a `journal_entries` row and edits no
      session `.md` (none exists to edit).
- [ ] `loaf state doctor` reports zero ephemeral `.md` and no `TASKS.json` after
      cutover (Markdown-only / dual-source state cleared).
- [ ] The cutover refuses to run (clear error) if SPEC-053's migration mechanism /
      `loaf install --upgrade` semantics are not present (gate is enforced, not
      just documented).

## Priority Order

Tracks ship in order. **All tracks are BREAKING and HARD-GATED on SPEC-053**;
none merge before SPEC-043 + SPEC-044 land and SPEC-053's migration mechanism
exists.

1. **Track 0 — Backup + restore primitives (non-destructive).** Byte-exact backup
   (raw bytes + SHA-256 manifest) and `loaf state restore-ephemerals`. *Go/no-go:*
   backup→restore round-trips byte-identical on a fixture; manifest self-verifies.
   *(Buildable independently; touches no tracked content yet.)*
2. **Track 1 — Byte-verify barrier (non-destructive).** Verification that SQLite
   bodies / backup bytes reproduce every ephemeral file; abort-on-any-failure.
   *Go/no-go:* a single injected mismatch aborts and deletes nothing.
3. **Track 2 — Provenance rewrite + companion ADR (non-destructive prep).** Rewrite
   /tombstone the ~10 dangling refs; amend SPEC-040:160/172; author companion ADR
   (surface reduction + ADR-013:12 correction). *Go/no-go:* no dangling ref
   remains in the staged tree; ADR present.
4. **Track 3 — Cutover delete (BREAKING; gated on SPEC-053).** `git rm` the
   recounted ephemeral set + `.agents/TASKS.json` in one change after Tracks 0-2
   pass; resolve SPEC-035 authority. *Go/no-go:* full byte-verify passed; backup
   present; restore tested; SPEC-053 gate satisfied.
5. **Track 4 — SPEC-029 enrichment retarget + regression guard (BREAKING).**
   Retarget enrichment to `journal_entries`; add the `loaf check` anti-regression
   rule and cross-branch procedure docs. *Go/no-go:* enrichment edits no `.md`;
   reappearing ephemeral `.md` fails `loaf check`.
