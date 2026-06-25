---
id: SPEC-052
title: "~/.agents Install Convention & Harness Install Parity"
source: "roadmap:20260621-020342-loaf-restructuring-roadmap (WS-F)"
source_sessions:
  - id: 20260621-001541-session
    role: shaped
created: 2026-06-22T09:13:21Z
status: complete
branch: feat/agents-install-convention-parity
---

# SPEC-052: ~/.agents Install Convention & Harness Install Parity

## Problem Statement

Loaf installs skills to inconsistent, per-harness directories, and the migration to a single
`~/.agents/` convention is half-done. Cursor, Codex, and Gemini already write skills to
`~/.agents/skills/` (`internal/cli/install_target.go:121,155,166`), but:

- **OpenCode** still installs skills into its own config dir (`syncTargetSubdir(... "skills")`
  under `$XDG_CONFIG_HOME/opencode/`, `install_target.go:110-116,499`) — not `~/.agents/`.
- **Amp** installs skills to `~/.config/agents/skills/` (`install_target.go:175-177`), which is
  neither `~/.agents/skills/` (the convention) nor Amp's documented native skills path. The
  roadmap asserts "Amp already reads `~/.agents/skills/` natively" — this claim is **unverified
  against Amp's manual** and must be confirmed before relocating Amp.
- The **install marker** (`.loaf-version`) and **installed-detection** still key on each harness's
  *config dir* (`install_target.go:284`, `install.go:507-517`), while the actual skill *content*
  now lives under `~/.agents/skills/`. So Loaf can report a harness as "installed" while its skills
  sit in a relocated, shared directory it does not own — and a relocation leaves **orphaned skill
  dirs** at the old per-harness path with nothing to clean them up.

There is no single source of truth for "where do skills go for harness X," no capability record of
which harness can actually read a configurable shared dir, and no orphan-cleanup on relocation.
This is the last structural gap before the harness matrix is coherent — and because it moves
already-installed users' files, it is **breaking** and must be gated.

## Strategic Alignment

- **Vision / Architecture:** Advances "the loaf CLI is the harness" by making install destinations
  a deliberate, tested convention rather than per-target accretion. A shared `~/.agents/skills/`
  reduces N per-harness skill copies to one canonical location for every harness that can read it.
- **Coordinates with SPEC-047 (WS-A, keystone parity contract):** SPEC-047 owns the build-time
  parity-matrix test (skill reachability + hook-surface semantics across the five first-class
  harnesses). SPEC-052 owns the **install-side** half of clause 4 of that contract: the
  `~/.agents/` destination convention for Codex, Cursor, OpenCode, and Amp's *skills*; Claude Code
  remains the documented plugin exception. SPEC-052 **adopts** SPEC-047's harness list and tier
  definitions — it does not re-derive them. Gemini removal is owned by SPEC-047; SPEC-052 only
  cleans up Gemini's installed files as orphan removal (it must not re-introduce a Gemini target).
- **Hard-depends on SPEC-053 (WS-G, migration mechanism):** SPEC-052 is breaking (path
  relocation). The orphan-cleanup / upgrade semantics, deprecation window, and one-time relocation
  it needs are the **mechanism** SPEC-053 defines. **Nothing in SPEC-052's breaking tracks ships
  before SPEC-053's mechanism exists** (roadmap §3, "F … needs WS-G migration"). SPEC-053 also
  owns the related taxonomy decisions (opt-in install packs, librarian) that this spec must not
  pre-empt.
- **Last in sequence (roadmap §3: A -> B -> C -> (D ∥ E) -> G -> F).** SPEC-052 follows the stable
  build matrix (SPEC-047) and the migration gate (SPEC-053).
- **Supersedes:** the implicit per-harness skill-destination logic in
  `internal/cli/install_target.go` — replaced by one capability-driven destination resolver.
- **Honors:** ADR-013 (`.agents/` resolves to the **main worktree** for *project-local* state).
  This spec governs the **global** `~/.agents/` user install dir, a distinct location; it must not
  be confused with project `.agents/`. A companion note in docs/decisions/ records the distinction.

## Solution Direction

1. **Per-harness capability investigation (do first, gate everything on its findings).** For each
   of Codex, Cursor, OpenCode, Amp: confirm — from each harness's own documentation, not
   inference — whether it can read a configurable/shared skills (and agents) directory, what that
   path is, and how it is configured (env var, settings key, native default). Record findings in a
   capability table checked into docs/decisions/ (a small ADR). **No relocation ships for a harness
   whose support is unverified.** Specifically resolve the open Amp question: does Amp read
   `~/.agents/skills/` natively, or only `~/.config/agents/skills/` (its current target,
   `install_target.go:176`)? The answer decides whether Amp moves or stays.

2. **A single destination resolver.** Replace scattered `filepath.Join(homeDir, ".agents", ...)`
   literals with one function that maps `(target, capability) -> skills/agents destination`,
   driven by the capability table. Harnesses that read `~/.agents/` resolve there; any that
   provably cannot keep their native path with a documented exception. Claude Code is excluded (it
   uses the plugin mechanism — the one documented exception, `install.go:373-380`).

3. **Decouple the install marker from the skills location.** Today `.loaf-version` and
   installed-detection key on each harness's config dir while skills live under `~/.agents/`
   (`install.go:507-517`). Establish a clear record of *what was installed where* so the upgrade
   path can find and clean relocated content — the input SPEC-053's orphan-cleanup consumes.

4. **Orphan cleanup on relocation (via SPEC-053's mechanism).** When `loaf install --upgrade`
   relocates a harness's skills to `~/.agents/skills/`, it removes the old per-harness skill copy
   (e.g. OpenCode's `$XDG_CONFIG_HOME/opencode/skills/`, Amp's `~/.config/agents/skills/`) so users
   are not left with two diverging skill sets. This reuses SPEC-053's deprecation/tombstone window,
   not a bespoke remover.

5. **Reconcile with the parity contract (SPEC-047).** Once destinations are capability-driven,
   feed the install-destination cells into SPEC-047's parity matrix so a future harness or skill
   cannot silently regress to a per-harness path.

## Scope

### In Scope
- Per-harness capability investigation (Codex, Cursor, OpenCode, Amp) with a documented capability
  table / mini-ADR; explicit resolution of the Amp `~/.agents/skills/` native-read question.
- A single capability-driven destination resolver for skills (and agents where supported),
  replacing the per-target literals in `internal/cli/install_target.go`.
- Relocating OpenCode skills to `~/.agents/skills/` **iff** verified-supported; relocating Amp
  skills to `~/.agents/skills/` **iff** verified-supported (otherwise Amp's native path stays, with
  a documented exception).
- Decoupling install-marker / installed-detection from the per-harness config dir so relocation is
  trackable.
- Orphan cleanup of old per-harness skill dirs on `--upgrade` (delegating to SPEC-053's mechanism).
- Tests covering destination resolution per harness, upgrade-relocation + orphan removal, and the
  Claude Code plugin exception.
- A companion docs/decisions/ note distinguishing global `~/.agents/` (install) from project
  `.agents/` (ADR-013).

### Out of Scope
- The migration mechanism itself (deprecation window, tombstone/alias, upgrade semantics) — owned
  by **SPEC-053**; SPEC-052 consumes it.
- Gemini's removal as a target — owned by **SPEC-047**; SPEC-052 only cleans up its installed files.
- The Amp **plugin** rebuild and its `.amp/plugins/` path — owned by **SPEC-047 (WS-A)**. SPEC-052
  governs Amp *skills* destination only; the plugin path is separate (roadmap §1, clause 4 note).
- The build-time parity-matrix test mechanism — owned by **SPEC-047**; SPEC-052 contributes cells.
- Opt-in language/domain install packs and the librarian profile decision — owned by **SPEC-053**.
- Any change to project-local `.agents/` resolution (ADR-013) or `loaf.json` location (SPEC-042).
- SQLite body/state migration — owned by SPEC-043/SPEC-045/SPEC-053.

### Rabbit Holes
- **Re-architecting the whole install flow.** Keep the change surgical: a resolver + marker
  decoupling + orphan cleanup. Do not rewrite hook merging, MCP, fenced-section, or symlink logic.
- **Inventing a config-discovery framework** for harness paths. A static, documented capability
  table is sufficient; do not build runtime probing of harness settings files.
- **Speculative support for harnesses that do not document a shared dir.** If a harness cannot read
  `~/.agents/`, leave it on its native path with a one-line documented exception — do not coerce it.

### No-Gos
- Do **not** ship any relocation before SPEC-053's migration/orphan-cleanup mechanism exists.
- Do **not** relocate a harness whose `~/.agents/` support is unverified against its own docs.
- Do **not** relocate or change Claude Code's plugin install (the documented exception).
- Do **not** leave orphaned skill dirs at old paths after an `--upgrade` relocation.
- Do **not** re-introduce Gemini as an install target while cleaning up its files.
- Do **not** assume the roadmap's "Amp reads `~/.agents/skills/` natively" claim — verify it.

## Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|-----------|------------|
| Amp does not actually read `~/.agents/skills/` natively (roadmap claim unverified) | Relocating Amp skills silently breaks Amp skill loading | Medium | Capability investigation first; keep Amp on `~/.config/agents/skills/` with a documented exception if unverified |
| Relocation strands users with skills at two paths | Diverging/duplicate skill sets, stale behavior | Medium | Orphan cleanup on `--upgrade` via SPEC-053; ship behind the migration gate |
| Shared `~/.agents/skills/` collides with a user's own non-Loaf skills | Loaf overwrites or wipes user content (`syncTargetDirIfExists` clears dest, `install_target.go:218-226`) | Low/Medium | Scope deletion to Loaf-managed entries only; never bulk-clear a shared dir; verify in capability ADR |
| Install marker keyed on wrong dir after relocation | `--upgrade` cannot find old content to clean | Medium | Decouple marker/detection from per-harness config dir (Solution 3) |
| Shipping before SPEC-053 | Breaking change with no cleanup path | Low (gated) | Hard go/no-go gate on SPEC-053; CI/spec dependency enforced |
| Drift from SPEC-047 parity contract | Future skill regresses to a per-harness path | Low | Feed destination cells into SPEC-047's parity matrix |

## Open Questions

- Does Amp read `~/.agents/skills/` natively, or only `~/.config/agents/skills/`? (Decides whether
  Amp moves.) — resolved by the capability investigation.
- Can OpenCode be configured/told to read `~/.agents/skills/`, or does it require its own config
  dir? If it must stay, is that a documented exception or a blocker for OpenCode parity?
- Should `agents/` (not just `skills/`) also relocate to `~/.agents/agents/` where supported, or is
  this spec skills-only for now? (Cursor currently installs agents to its config dir,
  `install_target.go:127`.)
- Should the install marker live in `~/.agents/` (per-Loaf, global) rather than per-harness config
  dirs, given content is shared? Coordinate with SPEC-053's upgrade semantics.
- How does shared `~/.agents/skills/` interact with multiple installed harnesses — one install
  serves all, or per-harness markers over a shared body? (Detection currently assumes per-harness.)

## Test Conditions

- [x] A capability table / mini-ADR exists in docs/decisions/ recording, per harness (Codex,
      Cursor, OpenCode, Amp), whether it reads a configurable/shared skills dir, the path, and the
      doc citation; the Amp `~/.agents/skills/` question is explicitly answered.
- [x] A single destination resolver maps `(target, capability) -> destination`; no per-target
      `~/.agents` path literals remain scattered in `install_target.go`.
- [x] For every harness verified to support `~/.agents/`, `loaf install --to <harness>` writes
      skills under `~/.agents/skills/` (test asserts the path).
- [x] OpenCode skills install to `~/.agents/skills/` iff verified-supported; otherwise the test
      asserts its native path with a documented exception.
- [x] Amp skills install to the resolver's path matching the verified capability (not a hardcoded
      `~/.config/agents/skills/` unless that is the verified answer).
- [x] `loaf install --upgrade` relocating a harness removes the old per-harness skill dir (no
      orphans), via SPEC-053's mechanism — asserted by a test that seeds the old path and checks it
      is gone after upgrade.
- [x] Claude Code install path and plugin mechanism are unchanged (documented exception) — asserted.
- [x] Installed-detection / marker correctly identifies a relocated install (does not report
      "not installed" after relocation).
- [x] Shared `~/.agents/skills/` cleanup never deletes non-Loaf-managed entries (test with a
      foreign skill present).
- [x] No relocation code path executes unless SPEC-053's migration mechanism is present (gate
      asserted in test or guarded at call site).
- [x] Gemini's installed skill files are removed on upgrade as orphans; no Gemini target is
      re-introduced.
- [x] Destination-resolution cells are referenced by SPEC-047's parity-matrix test (or a stub
      asserting the contract), so a per-harness regression fails the build.

## Priority Order

Tracks within SPEC-052, sequenced after SPEC-047 (stable matrix) and SPEC-053 (migration gate).

1. **Capability investigation + ADR** — *non-breaking*. Produces the capability table; the Amp
   question is answered. **Go/no-go gate:** no relocation track proceeds for a harness unverified
   here.
2. **Destination resolver + marker decoupling** — *non-breaking* (refactor; preserves current
   destinations until track 3). Centralizes path logic and makes relocation trackable.
3. **Parity-matrix integration** — *non-breaking*. Feed destination cells into SPEC-047's matrix.
4. **OpenCode / Amp relocation + orphan cleanup** — **BREAKING; gated on SPEC-053.** Only the
   verified-supported harnesses move; old per-harness dirs are cleaned via SPEC-053's mechanism.
   **Go/no-go gate:** SPEC-053 migration mechanism shipped + user sign-off (CLAUDE.md: ask before
   breaking changes; roadmap §4 ledger).
5. **Gemini orphan cleanup on upgrade** — **BREAKING; gated on SPEC-053** (coordinated with
   SPEC-047's Gemini removal).
