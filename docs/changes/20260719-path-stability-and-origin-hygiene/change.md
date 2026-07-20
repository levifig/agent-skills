---
change: path-stability-and-origin-hygiene
created: 2026-07-19
branch: path-stability-and-origin-hygiene
---

<!-- Frontmatter must open the file at byte one — parsers depend on it. No status-like frontmatter (readiness/status/state): readiness is derived — a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Path Stability and Origin Hygiene

## Problem

Codex's post-ship review of intent-exploration-foundation surfaced four defects that make installed Loaf policy and diagnostics untrustworthy between releases. First, the Codex installer resolves the `loaf` PATH entry through `EvalSymlinks` into the versioned Homebrew Cellar target, so every rendered surface — `CODEX_HOME/AGENTS.md` guidance, the SessionStart hook, and every execpolicy prefix in `loaf.rules` — is invalidated by every release, and Codex tasks already running with injected policy strand on paths that no longer exist. Second, the Intent, Exploration, and legacy-conversion writers introduced relationship origin values (`intent-create`, `exploration-create`, `legacy-conversion`) that `doctor` rejects — its invariant allows only `imported`/`manual`/`command` — so a healthy schema-12 database permanently warns on ten real rows, and the recommended `state repair relationship-origin` matches zero of them because it only backfills missing origins. The writer, validator, and repair plan each hold a private copy of the vocabulary and have drifted. Third, live help disagrees with implemented commands: `loaf state migrate --help` lists three of six dispatchable sources, and leaf subcommands like `loaf conversation handle add --help` fail with `unknown option`, undermining the maintenance skill's discover-syntax-from-live-help rule. Fourth, `journal context` still presents five historically-resolved blockers as unresolved and its newest project synthesis is from June — closure facts were never written.

## Hypothesis

Rendering the stable PATH entrypoint instead of its versioned resolution, closing the origin vocabulary behind one registry that writers, doctor, and repair all consume, and restoring help parity makes installed Codex policy survive upgrades and makes doctor and live help trustworthy authorities again — hardening the exact surfaces `change-native-execution-migration` will build on, before it is shaped.

## Scope

**In**

- A single relationship-origin registry in `internal/state` feeding the writers, the doctor invariant SQL, and `state repair relationship-origin`; the Intent, Exploration, and legacy-conversion writers normalize to `command`; repair gains a reclassify mode that migrates existing unknown-origin rows backup-first; doctor-clean tests cover Intent creation, Exploration creation, and legacy conversion.
- Codex-managed rendering (AGENTS.md guidance, hooks, execpolicy prefixes) uses the un-canonicalized PATH entrypoint (e.g. `/opt/homebrew/bin/loaf`); `EvalSymlinks` canonicalization is retained for forbidden-roots validation only; `loaf install --upgrade` repins existing installs.
- Help-surface parity: `state migrate` help lists all six sources with a dispatcher/help parity test; leaf subcommands (`conversation handle add`, and the Exploration conversation equivalent) honor `-h`/`--help`.
- Real-state remediation as Definition of Done evidence: reclassify the live unknown-origin rows, write `unblock` closures for the five stale blockers, and write a fresh `wrap(project)` synthesis.

**Out** (deferred, not rejected)

- Retained cross-context Exploration dogfood — not a code change; tracked as an Intent that gates shaping of `change-native-execution-migration` (candidate topic: the release-finalization-in-worktree-topologies question discovered 2026-07-19).
- Handoff and research artifact authority (`.agents/handoffs/`, `.agents/reports/` vs SQLite) — owned by the terminal guidance/artifact sweep, per the successor protocol.
- Structural staleness semantics for block/task/handoff/wrap surfaces (expiry, closure prompts) — terminal sweep; this Change only writes the missing closure facts.
- Non-Codex harness adapters for execpolicy-style trust — the central registry already anticipates them; nothing is claimed here.

**Cut** (explicitly rejected)

- Rendering literal `loaf` and trusting PATH resolution in execpolicy prefixes — it would authorize any earlier-on-PATH binary named `loaf`, surrendering the known-binary posture Decision 19 of journal-reliability-foundation established.
- Restoring versioned absolute paths with a repin-on-upgrade workflow — the review's core complaint (every release invalidates rendered policy and strands running tasks) would remain by design.
- Expanding the origin vocabulary with per-ceremony values — an open-ended registry recreates the drift class this Change exists to kill; operation detail belongs in `reason`.

## Observable Workflow

```text
$ loaf install --upgrade                       # Codex files now carry the stable entrypoint
$ grep -c Cellar ~/.config/codex/rules/loaf.rules
0
$ brew upgrade loaf                            # next release lands
$ grep loaf ~/.config/codex/hooks.json         # rendered command still resolves — nothing strands

$ loaf state repair relationship-origin --dry-run
  10 relationship row(s) with unknown origin would be reclassified to 'command'
$ loaf state repair relationship-origin --apply    # backup-first
$ loaf doctor                                  # zero relationship-origin diagnostics

$ loaf state migrate --help                    # lists markdown, storage-home, schema,
                                               # lifecycle-statuses, journal-first, deferrals
$ loaf conversation handle add --help          # prints leaf usage, exit 0
```

## Rabbit Holes and No-Gos

- Do not redesign the execpolicy trust model or add new basic leaves — only change which path string is rendered; classification and gating stay byte-identical.
- Do not touch installed-distribution-authority's resolver (PR #120) — `EvalSymlinks` remains correct for finding the distribution root; this Change only changes what is rendered into Codex-managed files.
- Do not build staleness detection, blocker expiry, or wrap reminders — that is terminal-sweep territory; here, only the missing closure facts get written.
- Do not migrate the seven pre-existing `command`/`imported` writers or invent a writer-side enforcement framework beyond the registry and its tests.
- Do not grow repair into a general origin-rewriting tool — reclassify maps exactly the three named legacy values to `command`; foreign origins stay visible as warnings, never silently rewritten.

## Decisions

Provenance: accepted 2026-07-19 during hotfix shaping, interviewed against Codex's post-ship review of intent-exploration-foundation; PATH rendering and origin vocabulary were explicit user decisions, the rest shaped autonomously. Decision 4 was amended during implementation (journaled `decision(state)` 2026-07-19) when the source-literal parity scan exposed a fourth legacy origin.

1. **Codex-managed surfaces render the stable PATH entrypoint; canonicalization is validation-only.** This amends Decision 19 of journal-reliability-foundation, which pinned "the canonical absolute Loaf executable" into rendered prefixes. The un-canonicalized `LookPath` result (`/opt/homebrew/bin/loaf`) is absolute and survives upgrades because Homebrew repoints the symlink; `EvalSymlinks` still runs to enforce forbidden-roots and existence checks against the real target. Forecloses literal-`loaf` PATH trust and versioned Cellar pins alike. Journaled as `decision(codex-policy)` 2026-07-19. Trust boundary, stated for H1: rendered policy now trusts wherever the entrypoint symlink points at execution time, not a fixed target — validation gates install/upgrade time (both the entrypoint and its canonical target are checked against forbidden roots), and a post-install retarget of the symlink is out of scope by design, exactly as a post-install replacement of the Cellar binary was under the old pin. Both models trust the writability of a user-owned Homebrew prefix; the amendment trades a version-frozen path for upgrade continuity without narrowing what an attacker with prefix write access could already do.
2. **The relationship-origin vocabulary is closed at mechanism level: `imported`, `manual`, `command`.** `origin` answers "by what mechanism did this row appear"; `reason` carries the operation ("recorded by intent create"). The three new writers normalize to `command`, matching every pre-existing CLI writer. Forecloses per-ceremony origin values and the open-ended registry they imply. Journaled as `decision(state)` 2026-07-19. Implementation note: the parity scan (Decision 3) found the closed vocabulary already breached beyond the shaped three — `run.go` and `finding.go` inlined origin `system` on five run/finding/verdict relationship writers; all five were normalized to `command` in this Change.
3. **One registry, three consumers, and the invariant is executable.** Writers, the doctor SQL, and repair all derive from a single Go registry, and each state-creating ceremony gets a doctor-clean-after-writer test — the only structure that prevents recurrence when the next ceremony lands.
4. **Repair reclassifies the named legacy values; it does not invent.** `state repair relationship-origin` keeps its missing-origin backfill and adds reclassification of exactly `intent-create`, `legacy-conversion`, `exploration-create`, and `system` to `command` — dry-run first, backup-first on apply, idempotent on rerun. Bare invocation (no `--origin`) is reclassification-only, which is what the workflow above runs; the backfill requires an explicit `--origin imported|manual` because inventing missing provenance is an operator judgment, while reclassifying the named legacy values is not. `system` was amended in during implementation: the parity scan exposed it as a shipped writer-invented origin (run/finding ceremonies in released alphas), so user databases need the same reclassify path; the local database was confirmed clean of it (its ten unknowns are `legacy-conversion`×8 + `intent-create`×2). Genuinely foreign origins keep warning rather than being laundered into `command`; surfacing them is doctor's job. The live row count is treated as a floor, not an exact target — any Intent/Exploration ceremony run before this ships adds rows the repair must also catch.
5. **Journal staleness is remediated as data, not solved as code.** Five `unblock` closures and one `wrap(project)` synthesis are Definition of Done evidence; the structural fix is named for the terminal sweep.
6. **Exploration dogfood is a successor gate, not hotfix scope.** A tracked Intent requires one real Exploration spanning two conversations/harnesses, resumed from its checkpoint, before `change-native-execution-migration` is shaped — honoring the reviewer's sequencing without padding this hotfix.

## Planning Contract

### Approach — origin registry (U1)

Add the registry to `internal/state` (single source: allowed origins plus the doctor `NOT IN` clause built from it). Change the three INSERT literals — `intent.go` (`'intent-create'`), `intent_conversion.go` (`'legacy-conversion'`), `exploration.go` (`'exploration-create'`) — to `'command'`. Extend `inspectRelationshipOriginInvariants` in `status.go` to build its allowed set from the registry. Extend the `relationship-origin` repair plan: dry-run reports each of the three named legacy origins with count and target value; apply reclassifies them to `command` after the standard backup, and reruns are no-ops; origins outside the named set are reported but never rewritten. Tests: doctor-clean after Intent create, Exploration create, and legacy conversion on fresh fixtures; a registry parity test asserting every origin literal written by `internal/state` code is registry-listed; repair fixture seeded with all three legacy values proving dry-run match, apply, idempotency, and post-repair doctor cleanliness.

### Approach — Codex stable-path rendering (U2)

Split `trustedCodexJournalExecutable` into resolution and rendering concerns: validation continues to canonicalize (`EvalSymlinks` + `Abs`) and enforce forbidden roots and guidance-character checks against the real target; the returned render path becomes the absolute un-canonicalized `LookPath` result. All rendered surfaces — AGENTS.md fenced guidance, `hooks.json` command, `loaf.rules` prefixes — consume the render path. Unit tests use a fake symlink layout (entrypoint symlink → versioned target) asserting rendered content carries the entrypoint, validation still rejects forbidden-root targets, and retargeting the symlink (simulated upgrade) leaves previously rendered files valid without rewrite. `loaf install --upgrade` repins existing installs through the existing digest-owned managed-content merge; the isolated `CODEX_HOME` runtime smoke re-runs to prove model-visible startup is unchanged.

### Approach — help parity (U3)

`writeStateMigrateHelp` lists all six sources; a parity test asserts the dispatcher's source set and the help text agree, so the next source cannot ship unlisted. Leaf subcommand help: route `-h`/`--help` before unknown-option rejection for `conversation handle add` and the Exploration conversation equivalent, with tests asserting exit 0 and usage output.

### Risks

- **Digest ownership on upgrade.** Rendered Codex files are digest-owned; the merge policy refuses unowned or locally modified content. Repinning must flow through the recognized-ownership path — preflight reads the existing ownership tests before touching the renderer.
- **Real-state repair timing.** The repair ships in the next alpha; running it against the live database with a stale installed binary is the known stale-binary hazard. The DoD run uses the checkout binary explicitly or waits for the released binary, and says which.
- **`~/.local/bin` copies.** Where the PATH entry is a real file (launcher + `native/`), render path and canonical path coincide — behavior is unchanged by construction, but a test pins it.
- **Row-count drift.** Every Intent/Exploration ceremony before ship adds unknown-origin rows (this shaping itself may add one via Intent creation). Repair and its tests target the pattern, never a hardcoded count.

### Adjacent-surface notes

Two edits sit adjacent to the unit letter and are deliberate, not drift. Bare `loaf migrate --help` (top level) was fixed alongside U3's contracted surfaces because it fails the identical discover-syntax-from-live-help rule the Problem names and was flagged in this Change's own review round; it received the same registry-derived help treatment and a parity test. The three u8 smoke-evidence files under `docs/changes/20260710-journal-reliability-foundation/research/` were regenerated because capability evidence pins the native binary's SHA-256 and fails the suite on drift by design; regeneration ran exclusively through the sanctioned smoke writers (as in the installed-distribution-authority Change), and the diffs carry only hash, timestamp, and marker.

### Sequencing

U1 and U2 are independent and can proceed in parallel; U3 trails as mechanical cleanup. Real-state DoD steps (repair apply, closures, wrap, live repin) run after implementation lands, with the release. The gating Intent for Exploration dogfood is created during shaping, before implementation starts.

## Implementation Units

- **U1 — Relationship-origin registry and normalization.** One registry consumed by writers, doctor, and repair; three writers normalized to `command`; repair reclassify mode; doctor-clean and parity tests.
- **U2 — Codex stable-path rendering.** Render the un-canonicalized PATH entrypoint across AGENTS.md, hooks, and execpolicy prefixes; validation keeps canonicalizing; upgrade repins; symlink-retarget test proves upgrade survival.
- **U3 — Help-surface parity.** Six migrate sources listed with a dispatcher/help parity test; leaf `-h`/`--help` honored on `conversation handle add` and the Exploration equivalent.

## Verification Contract

<!-- Executable (machine-checkable): -->

- **V1.** `go test ./... -count=1` and `npm run typecheck` pass; `loaf build` succeeds.
- **V2.** Doctor-clean-after-writer tests: after Intent create, Exploration create, and legacy conversion on fresh fixtures, `inspectRelationshipOriginInvariants` returns zero diagnostics.
- **V3.** Repair fixture seeded with `intent-create`, `legacy-conversion`, and `exploration-create` rows: dry-run reports them all with target `command`; apply reclassifies backup-first; rerun is a no-op; doctor is clean afterward.
- **V4.** Renderer test with a symlinked entrypoint: rendered AGENTS.md, hooks, and rules contain the entrypoint path and no canonicalized segment; forbidden-root targets still rejected; symlink retarget leaves rendered files valid.
- **V5.** Dispatcher/help parity test passes; `loaf state migrate --help` lists all six sources; `loaf conversation handle add --help` and the Exploration equivalent exit 0 with usage.

<!-- Human review: -->

- **H1.** A reviewer confirms the Decision 19 amendment language accurately preserves the validation posture — no security property is silently dropped in the rendered-vs-validated split.
- **H2.** A reviewer confirms the real-state DoD evidence (backup filename, doctor output, closure entries, repinned file diff) is recorded in the PR body.

## Definition of Done

- All Verification Contract items pass; `loaf change check` reports zero violations.
- Real database: `state repair relationship-origin --apply` run backup-first, `loaf doctor` clean of relationship-origin diagnostics.
- Journal hygiene: `unblock` closures written for the five stale blockers, plus a current `wrap(project)` synthesis.
- Real Codex install repinned via `loaf install --upgrade`; no Cellar-versioned path remains in managed files.
- The Exploration-dogfood gating Intent exists and names its revisit trigger (shaping of `change-native-execution-migration`).
- PR merged per project conventions (squash, PR# suffix, no auto-push).

## Durable Outputs

After ship, `/loaf:reflect` decides whether two learnings earn ARCHITECTURE.md entries: the closed mechanism-level origin vocabulary (writers never invent provenance values) and the rendered-vs-validated path split for harness-managed trust surfaces. The Decision 19 amendment lives in this Change and the journal; the original journal-reliability-foundation document remains an unedited historical record.

## Open Questions

- [KU] Does Codex re-read execpolicy rules at task start (making stable paths sufficient without any rewrite), or cache them per session? → U2 isolated `CODEX_HOME` runtime smoke, recorded with the DoD evidence.

Resolved during implementation (routes retained for provenance):

- ~~[KU] digest-owned merge policy~~ → resolved by U2's upgrade-survival test: an owned upgrade after a symlink retarget converges byte-identically with no rewrite, and the existing ownership tests all pass against the render-path split.
- ~~[KU] is `manual` code-written?~~ → resolved yes: `Store.CreateLink` (`internal/state/link.go`) writes `manual` on every explicit `loaf link` relationship, and repair writes it under `--origin manual`; recorded in the registry's doc comment.

## Source Inputs

- Codex post-ship review of intent-exploration-foundation, delivered in conversation 2026-07-19, with reproduced anchors: `internal/state/status.go` (allowed-origin SQL), `internal/cli/install_codex_rules.go` (EvalSymlinks rendering), `internal/cli/cli.go` (migrate help vs dispatcher), and the failing `loaf conversation handle add --help`.
- Journal decisions 2026-07-19: `decision(codex-policy)` (Decision 19 amendment) and `decision(state)` (closed origin vocabulary).
- Prior Changes: `20260710-journal-reliability-foundation` (Decision 19, the amended contract), `20260718-installed-distribution-authority` (PR #120 — executable provenance, explicitly untouched), `20260717-intent-exploration-foundation` (PR #122 — the reviewed work).
- Live-state evidence: ten unknown-origin relationship rows on the real schema-12 database; five unresolved-blocker entries and a June-dated project synthesis in `loaf journal context`.
