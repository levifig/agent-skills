---
id: SPEC-053
title: Breaking-Change Migration Mechanism & Taxonomy Decisions
source: "roadmap:20260621-020342-loaf-restructuring-roadmap (WS-G)"
created: 2026-06-22T09:13:21Z
status: complete
branch: feat/migration-mechanism-taxonomy
source_sessions:
  - id: 20260621-001541-session
    role: shaped
---

# SPEC-053: Breaking-Change Migration Mechanism & Taxonomy Decisions

## Problem Statement

Every breaking change in the restructuring program — dropping Gemini (SPEC-047), moving
ephemeral artifact bodies into SQLite and deleting in-tree `.md` (SPEC-045), relocating
non-Claude install destinations to `~/.agents/` (SPEC-052), retiring or externalizing skills, and
converting language/domain packs to optional recommended/curated/vendor installs — fails the same
already-installed user in the same way:
**Loaf has no mechanism to clean up after itself.**

`loaf install --upgrade` today only re-syncs the targets that still exist. The sync
(`syncTargetDirIfExists`, `internal/cli/install_target.go:211-227`) mirrors a *source dir*
into a *destination dir*, removing stale **entries inside a dir Loaf still visits**
(`install_target.go:223` — `os.RemoveAll` per orphaned child; proven by
`install_target_test.go:30,66,69`). It has two blind spots that the program's breaking changes
walk straight into:

1. **A target dropped from the build is never revisited, so its tree is never cleaned.** When
   Gemini leaves `installValidTargets` (`install.go:41`) and `installDisplayNames`
   (`install.go:32-39`), upgrade simply stops touching `~/.gemini` (or its `~/.agents/skills`
   share) — the orphaned skills sit forever. Same for a retired skill once its source dir is gone.
2. **Relocation is invisible.** SPEC-052 points Codex/Cursor/OpenCode skills at `~/.agents/skills`.
   Some targets already install there (`install_target.go:121,155,166`); others install to
   per-tool config dirs. Upgrade has no concept of "old destination → new destination," so a
   relocation leaves the *old* install live and adds a *second* copy at the new path.

Bodies face the same gap from the other direction. Ephemeral `.md` files in
`.agents/{ideas,sparks,sessions,brainstorms,drafts,tasks}/` become SQLite-only after SPEC-045.
There is no defined one-time, reversible, backed-up migration that moves their content into the
body store and then removes the files — and doing this irreversibly against a user's working
tree violates the CLAUDE.md non-negotiable on breaking changes.

Finally, the deep-evaluation report (`report-loaf-skills-deep-audit`,
`.agents/reports/20260620-214448-audit-loaf-skills-deep-audit.md`) left four **taxonomy
decisions** open that each imply a user-visible change and therefore must ride this gate:
externalizing `thermo-nuclear-code-quality-review`; tightening `debugging`'s routing; making
language/domain packs optional/recommended/vendor install surfaces; and resolving the `librarian` profile (previously missing its
cross-target sidecars — only `content/agents/librarian.claude-code.yaml` existed, no
`.cursor.yaml`/`.opencode.yaml`, so its tool boundary was lost on four of five harnesses).

This spec is the **gate**: it builds the migration mechanism, defines the deprecation/tombstone
model and reversible body migration, and records the taxonomy decisions — all behind explicit
user sign-off. **No breaking change in the program ships until this lands.**

## Strategic Alignment

**Vision/Architecture.** Implements the roadmap's WS-G and §4 breaking-change ledger
(`roadmap:20260621-020342-loaf-restructuring-roadmap:232-243,273-283`). Upholds the
CLAUDE.md non-negotiable "ask before significant or breaking changes" by making the program's
breaking changes opt-in, reversible, and backed up. Honors ADR-013: `.agents/` resolves to the
**main** worktree, and moving ephemerals to SQLite *shrinks* that surface (companion ADR) rather
than deleting `.agents/` — the migration removes ephemeral bodies only, not the directory's role.

**Coordinates-with / Gates:**
- **SPEC-045** (Ephemeral-to-SQLite Cutover, BREAKING) — *hard-gated on this spec.* SPEC-045's
  cutover consumes the reversible body-migration + backup defined here. SPEC-045 must not delete
  any in-tree `.md` until `loaf migrate markdown` (import) and `loaf state backup` are proven.
- **SPEC-052** (~/.agents Install Convention & Harness Install Parity, BREAKING) — *depends on this
  spec's* relocation/orphan-cleanup in `loaf install --upgrade`.
- **SPEC-047** (Build Integrity, Parity Contract & Target Simplification) — drops Gemini from the
  *build* (independent, non-breaking server-side). The **user-side** orphan cleanup of an already
  installed Gemini surface is delivered here. SPEC-047 may land first; the user-facing Gemini
  removal is gated behind this mechanism.
- **SPEC-043** (SQLite-Native Artifact Bodies) — supplies the body store (`session_state_snapshots.content`
  precedent generalized) and `migrate markdown` body-import path this spec wraps in reversibility.
- **SPEC-048/049** (Session convergence / status vocabulary) — sessions become SQLite-only bodies;
  their `.md` removal is an ephemeral migration governed by this gate.

**Supersedes / resolves conflicts:**
- Coordinates with **SPEC-035** (TASKS.json subsystem still live): retiring or migrating tasks'
  in-tree representation is a breaking change and must route through this gate's body migration,
  not re-create dual-source drift.
- Coordinates with **SPEC-029** (librarian enrichment edits session `.md`) and the now-wired
  `librarian` profile: the taxonomy decision here keeps the profile as a durable artifact handler
  while reconciling SPEC-029's markdown-session assumption with SQLite-backed sessions.
- Coordinates with **SPEC-037** ("specs are mutable") vs the program's durable-render model:
  spec/ADR renders remain git-committed (not ephemeral), so they are **out of scope** for the
  ephemeral body migration here.

## Solution Direction

Deliver three things, all opt-in and reversible, gated on user sign-off:

1. **An upgrade/cleanup mechanism in `loaf install --upgrade`** that knows about (a) targets and
   skills that no longer exist (orphans), (b) skills externalized to vendor sources, and
   (c) destinations that have moved (relocation). Driven by a declarative
   **deprecation manifest** (retired targets, retired skills, externalized skills, old→new path maps)
   so future breaking changes register an entry instead of writing bespoke cleanup code. The first
   implementation lands the manifest loader, report model, and test-only simulated entries while
   leaving production breaking entries empty until sign-off.

2. **A tombstone/alias + deprecation-window model.** A removed skill or target leaves a tombstone
   recording what it was, when it was retired, and what (if anything) replaces it. Aliases let a
   renamed surface keep routing during the deprecation window. Upgrade reports what it removed and
   why; nothing is deleted silently. Tombstones live in the static package manifest first; SQLite
   can later mirror applied migration events if a query surface is needed.

3. **A reversible, backed-up ephemeral body migration.** `loaf state backup` snapshots the SQLite
   DB and the affected `.agents/` tree before any destructive step; `loaf migrate markdown`
   imports ephemeral bodies into the body store (idempotent, dry-run capable — see SPEC-040's
   TASK-197 dry-run precedent) and only then removes the in-tree `.md`, recording a manifest that a
   `loaf migrate markdown --rollback` can replay from the backup.

The **taxonomy decisions** are recorded here as accepted/decided items (each needs sign-off) and
implemented as deprecation-manifest entries so they flow through the same mechanism:
- **Externalize `thermo-nuclear-code-quality-review` as a vendor skill.** It is not retired or
  deleted. Upgrade reports old installed copies as externalized and points users at the vendor
  source; the future `loaf skill add`/update/remove installer is a follow-on package-management
  spec with source pinning and provenance.
- **`debugging` routing:** tighten the description under SPEC-051 and do not set
  `disable-model-invocation` in this slice. Explicitly **not** `user-invocable: false` — that hides
  it from the command menu but does **not** stop the model from routing to it.
- **Language/domain packs → recommended/curated/vendor optional skills.** Default install keeps
  Loaf core. Language/domain reference packs become recommended optional skills, and third-party
  skills become vendor skills. De-selection/removal waits for the installer/profile model.
- **`librarian` profile:** wire it as Loaf's durable artifact handler. It gains the missing
  cross-target sidecars (`.cursor.yaml`, `.opencode.yaml`) and wrap/housekeeping/orchestration
  caller guidance so its `.agents/` boundary survives on first-class harnesses.

## Scope

### In Scope
- Deprecation manifest format (retired targets, retired skills, externalized/vendor skills,
  renamed/aliased surfaces, old→new
  path maps) consumed by `loaf install --upgrade`; destructive production entries stay empty until
  signed off, while non-destructive externalization entries can provide upgrade guidance.
- Orphan cleanup for whole dropped targets (Gemini) and individual retired skills, including
  surfaces under `~/.agents/skills` and per-tool config dirs, proven with simulated/test-only
  manifest entries before real breaking entries are registered.
- Relocation handling: detect an install at an old destination, migrate/relink to the new one, and
  remove the stale dir — idempotent, safe to re-run, and proven with simulated/test-only manifest
  entries before SPEC-052 registers real path maps.
- Tombstone + alias model and a deprecation window with user-visible upgrade reporting.
- `loaf state backup` (DB + affected `.agents/` snapshot) and reversible `loaf migrate markdown`
  with `--dry-run` and `--rollback`, used by SPEC-045's cutover.
- Recording the four taxonomy decisions as accepted; adding the externalized thermo manifest entry;
  leaving `debugging` as description-only for SPEC-051; recording recommended/curated/vendor
  optional-skill packaging; and adding librarian cross-target sidecars plus caller guidance.
- Tests: upgrade removes a simulated dropped target; relocation produces exactly one copy at the new
  path with the old removed; migration round-trips (import → rollback restores bytes); tombstones
  are emitted and reported.

### Out of Scope
- The actual Gemini *build* removal (SPEC-047), `~/.agents/` *destination* logic (SPEC-052), and the
  body-store *schema* (SPEC-043) — this spec consumes them, it does not build them.
- Durable spec/ADR/report renders — they stay git-committed and are not ephemeral-migrated here
  (SPEC-044's drift gate owns their sync).
- The taxonomy *content edits* beyond what the manifest needs (description rewrites are SPEC-051;
  skill de-bloat is SPEC-050). This spec records the decisions and the visibility/removal wiring.
- Raw transcript capture/redaction (explicitly deferred by SPEC-040).

### Rabbit Holes
- Building a general-purpose plugin/package manager. The manifest can point to vendor sources, but
  `loaf skill add`/update/remove, source pinning, and remote fetching are a follow-on spec — not a
  dependency resolver hidden inside upgrade cleanup.
- A full undo/redo system for all state. Reversibility here means a one-time backup + rollback of
  the migration, not transactional time-travel.
- Auto-detecting "old" installs heuristically across every possible historical layout. Support the
  known prior paths (`~/.gemini`, per-tool config dirs, prior `~/.agents` layouts) explicitly.

### No-Gos
- No destructive change to a user's `.agents/` tree or install without a verified backup and
  explicit confirmation (CLAUDE.md non-negotiable).
- No silent removal — every orphan/relocation/migration action is reported.
- Do **not** use `user-invocable: false` as the `debugging` routing fix (does not stop model
  routing).
- Do not delete the `.agents/` directory or change ADR-013's main-worktree resolution.
- No breaking change in SPEC-045 / SPEC-052 / user-side SPEC-047 ships before this gate is signed
  off and proven.

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Migration deletes user content irreversibly | Data loss | Mandatory `loaf state backup` + manifest + `--dry-run`/`--rollback` before any delete; tests assert byte round-trip |
| Relocation creates duplicate installs | Confusing/broken state | Relocation is idempotent: detect old path, move/relink, remove stale; test asserts exactly one copy |
| Dropped-target cleanup touches a path the user repurposed | Removes unrelated files | Only remove paths Loaf provably wrote (fenced markers / known install layout); confirm before removing unrecognized content |
| Global DB + parallel agents during migration | Write contention / partial migration | Run migration as a single guarded operation; rely on WAL/busy_timeout; refuse to migrate with active writers |
| Description-only `debugging` still over-routes | Model routes to a broad reference skill too often | Validate under SPEC-051 routing eval before applying stronger policy |
| Deprecation window too short/long | Stale aliases or premature removal | Window is an explicit manifest field per entry; default documented; user can override |

## Open Questions

1. Deprecation window length — one release, N releases, or a date? Decision for mechanism:
   per-entry `window` field with a documented default of one release; entries may override.
2. Should tombstones live in SQLite (queryable via `loaf`) or as a static manifest in the package,
   or both? Decision for mechanism: static package manifest first, with optional future SQLite
   event mirroring for applied cleanups.
3. `librarian`: Decision: wire it as the durable artifact handler, with first-class sidecars and
   caller guidance from wrap/housekeeping/orchestration.
4. Where does `loaf state backup` write, and what is its retention/cleanup policy? Decision for
   first backup slice: `XDG_DATA_HOME/loaf/backups/<timestamp>/`, verified by manifest and explicit
   restore guidance; retention policy deferred to a later housekeeping spec.
5. For optional recommended/curated/vendor packs: is de-selection on upgrade an explicit flag, or
   inferred from a saved install profile? Where is the install profile stored? Still open; production
   optional-pack removals do not land in the first mechanism slice.
6. Does relocation need to preserve user-local edits at the old path, or is the install dir
   considered fully Loaf-owned (overwrite-safe)? Decision for install cleanup: only delete
   Loaf-owned paths with `.loaf-version` or exact managed child paths; unmarked directories are
   reported as skipped, not removed.

## Test Conditions

- [x] `loaf install --upgrade --yes` removes a previously-installed but now-dropped target's
      surface (Gemini simulation) and reports what it removed; without `--yes`, upgrade reports
      `confirmation-required`.
- [x] `loaf install --upgrade --yes` removes a retired skill's installed files via a manifest
      entry; without `--yes`, upgrade reports `confirmation-required`.
- [x] Relocation from an old destination to `~/.agents/skills` results in exactly one copy at the
      new path and the old path removed; re-running upgrade is a no-op.
- [x] `loaf state backup` produces a restorable DB snapshot, and
      `loaf migrate markdown --backup` snapshots the affected `.agents/` tree.
- [x] `loaf migrate markdown --dry-run` reports the bodies it would import and the files it would
      remove, changing nothing.
- [x] `loaf migrate markdown` imports ephemeral bodies, then `--rollback` restores the original
      `.md` files byte-for-byte from the backup.
- [x] Migration refuses to run (with a clear message) without a successful backup.
- [x] A retired surface emits a tombstone; upgrade output cites the tombstone reason.
- [x] `thermo-nuclear-code-quality-review` is registered as an externalized vendor skill and is
      reported on upgrade without removal.
- [x] `debugging` remains description-only for now; no `disable-model-invocation` or
      `user-invocable: false` policy change lands in SPEC-053.
- [x] Language/domain packs are recorded as future recommended/curated/vendor optional skills;
      production removals defer until install profiles exist.
- [x] `librarian` decision applied: present with `.cursor.yaml` and `.opencode.yaml` sidecars so
      its durable-artifact-handler boundary holds on first-class harnesses.
- [x] No destructive action runs without confirmation (or `--yes`) and a verified backup.

## Priority Order

Tracks are sequenced so the mechanism exists before any decision that depends on it. This whole
spec is the **go/no-go gate** for SPEC-045, SPEC-052, and user-side SPEC-047.

1. **Deprecation manifest + orphan cleanup in `--upgrade`** — non-breaking by itself (no entries
   yet); unblocks every later removal. *Gate: lands before any breaking change registers an entry.*
2. **Relocation handling** — non-breaking until SPEC-052 adds path maps. *Gate for SPEC-052.*
3. **`loaf state backup` + reversible `loaf migrate markdown` (dry-run/rollback)** — non-breaking;
   builds on SPEC-043 body import. *Gate for SPEC-045 cutover.*
4. **Tombstone/alias + deprecation-window model + upgrade reporting** — non-breaking.
5. **Taxonomy decisions wired as manifest entries / sidecars** — *each is user-visible; needs
   sign-off:* externalize `thermo`, keep `debugging` description-only, record recommended/curated/
   vendor optional packs, and wire `librarian` as durable artifact handler. *Gated.*
6. **Sign-off checkpoint** — confirm backup/rollback proven and decisions accepted; only then do
   SPEC-045/052 and user-side SPEC-047 removals ship. *Hard gate.*
