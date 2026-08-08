<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Hooks-Entry Reconciliation

## Problem

Codex and Cursor `hooks.json` files are managed as whole-file `hook-projection` artifacts guarded by a digest over Loaf-owned entries. Any divergence between that digest and what the installed manifest recorded — including the operator deliberately deleting one Loaf entry — makes every subsequent `loaf upgrade` refuse the file. On the canary machine this has been live since 2026-08-02: the operator emptied `~/.codex/hooks.json` to disable the SessionStart journal hook, and every upgrade since (including the 0.2.20 release verification) reports a refusal that cannot be resolved without either restoring a hook the operator does not want or hand-editing state. The whole-file model also conflates surfaces Loaf has no business judging: the live Cursor file carries 33 entries Loaf never shipped (a third-party herdr hook and a 32-entry pre-0.2 generation from 2026-03-25), and their mere presence is entangled with Loaf's digest arithmetic. Evidence: `research/hook-entry-classification.json` and `research/hook-conflict-report.html`, generated from the live files against 0.2.20 dist.

## Hypothesis

If Loaf reconciles hook files per entry — adding, updating, and removing only its own entries at each hook point, recognizing them by construction, and treating every other entry as invisible — then the drift-refusal class disappears, operator disable-intent survives upgrades as explicit recorded state, and shared hook files stop being a coordination hazard between Loaf, other tools, and the operator. The 0.2.21 upgrade on the canary machine is the proof: zero drift refusals, the Codex disable-intent preserved, and every non-Loaf entry preserved untouched.

## Scope

**In**

- Entry-level reconciliation for Codex `~/.codex/hooks.json` and Cursor `~/.cursor/hooks.json` on install and upgrade: converge Loaf's own entries (add missing-and-enabled, update present-but-different, remove retired), never touch any other entry.
- A built hook catalog per target — `(target, event, hook_id, desired entry template)` derived from `config/hooks.yaml` at build time and readable by the installed CLI — as the single identity authority for reconciliation, absorption, and the verb surface.
- Ownership by construction: trusted Loaf-executable invocation or exact manifest-recorded Loaf-managed file paths under a closed normalization algorithm, plus a frozen enumerated legacy allowlist for Loaf's own older generations. No marker, digest, or open-ended provenance judgment is load-bearing.
- User-scoped, host-local hook-enablement records in the global SQLite state (mint-once opaque IDs, UNIQUE natural key `(target, event, hook_id)`), with absence-of-record meaning enabled, plus a durable per-target absorption marker.
- One-time absorption at the first 0.2.21 reconcile of a prior install, restricted to the closed cohort of hook IDs the prior version shipped: cohort entries absent from the live file become `disabled` records instead of refusals or re-adds. Fresh installs default everything to enabled.
- A minimal `loaf hooks` verb surface: `list`, `enable`, `disable`, with enable/disable immediately reprojecting the affected file.
- Retirement of the `hook-projection` artifact kind: drift-refusal planning, digest guard, and whole-file merge paths deleted; integrity preconditions retained; installed manifests tolerate and drop the obsolete rows.
- `loaf config check` hook diagnosis updated to enablement-aware states, with `--fix` routed through the reconciler.
- Canary acceptance evidence recorded from this machine's real upgrade.

**Out** (deferred, not rejected)

- Claude Code `settings.json` hooks — only relevant if the plugin path is retired (spark `journal:5c2f05fd`, the de-pluginization track).
- OpenCode and Amp runtime plugin files — `plugin` artifact kind, a different mechanism (broader event coverage lives in spark `journal:4dfde516`).
- Any richer hooks surface (grouping, profiles, per-project enablement, cross-host portability) — nothing here forecloses it; the enablement relation is deliberately minimal.

**Cut** (explicitly rejected)

- Adjudicating, claiming, retiring, or "cleaning up" non-Loaf entries — including the 2026-03-25 legacy generation and its five entries that functionally duplicate shipped enforcement hooks. Other entries' existence or inexistence makes no difference to reconciliation, ever. Legacy recognition is a closed enumerated allowlist; nothing outside it can ever be claimed.
- Ongoing deletion-inference: after absorption, hand-deleting a Loaf entry is not a disable gesture — the next reconcile re-adds it. Disable is `loaf hooks disable`.
- Drift refusals for hook files, under any name: no code path may refuse because content diverged from a recorded digest or because foreign entries exist.

## Observable Workflow

The operator runs `loaf upgrade`. Hook files reconcile silently: Loaf's entries converge to the shipped shape at each hook point, everything else is preserved, and the plan/output reports per-entry actions (`add`, `update`, `remove`, `absorbed as disabled`) instead of file-level conflicts. On this machine the first 0.2.21 upgrade prints the Codex `session-start-loaf` absorption once, and every subsequent upgrade is a quiet no-op.

```
$ loaf hooks list --target codex
  session-start-loaf   SessionStart   disabled   (absorbed 2026-08-08)

$ loaf hooks enable session-start-loaf --target codex
  enabled — entry restored to ~/.codex/hooks.json

$ loaf hooks disable session-start-loaf --target codex
  disabled — entry removed from ~/.codex/hooks.json
```

`loaf hooks list` shows every hook the current version ships for installed targets with its event, effective enablement, and whether it is currently projected into the file. Deleting a Loaf entry by hand no longer signals anything: the next reconcile restores it, because the file is a projection of enablement state, not an authority over it. Enable/disable run the full reconciler and report every action taken — on a converged target that is exactly one entry. `loaf config check` reports enablement-aware hook health: disabled-and-absent and enabled-and-in-sync are healthy, enabled-but-stale or enabled-and-missing mean a reconcile is needed, disabled-but-present means a reprojection is pending, and foreign entries are never mentioned.

## Rabbit Holes and No-Gos

- **Cleaning the March 2026 generation.** Five of those 32 entries double-fire enforcement alongside shipped hooks. Tempting — and explicitly not this Change's business. If the operator wants them gone, that is a hand edit or a separate tool; reconciliation must not grow a "cleanup" mode.
- **A general write-time guard framework.** The state-dedupe session concluded specific fixes beat frameworks; the enablement table needs its UNIQUE constraint, the absorption marker, and nothing more.
- **Directory-level path containment.** Claiming everything under `~/.cursor/hooks/` would swallow the March generation. Ownership matches exact normalized manifest-recorded file paths only.
- **Enablement scoping creep.** Records are user-scoped per target per host. No per-project, per-branch, per-profile, or cross-host enablement in this Change.
- **Open-ended legacy archaeology.** The legacy allowlist is frozen at the enumerated maps that exist today; discovering further "probably ours" generations later means a new Change with new evidence, never a quiet widening.

## Decisions

Provenance: structured interview (2026-08-08), a reaction artifact over the live files (`research/hook-conflict-report.html`), the Aug 5 journal decisions (`journal:dcf9875d`, `journal:0e33fc06`), cross-session coordination with the state-dedupe shape (identity discipline, `decision(identity)` of 2026-08-08), and an adversarial Codex review (gpt-5.6-sol, 2026-08-08) whose ten blocking findings and three advisories are folded in below.

1. **Converge own entries per hook point; all other entries are invisible.** Add, update, or remove Loaf's entries only; other entries' existence or inexistence makes no difference. Forecloses drift refusals, foreign-entry adjudication, and any dependency on what else lives in the file.
2. **Absorb once, then project — gated by a durable marker and restricted to a closed cohort.** Absorption runs when no per-target absorption marker exists and a prior install is detected; it considers only hook IDs the prior installed version actually shipped (the 0.2.20 cohort is enumerated in the catalog; the prior version comes from the installed manifest's `package_version`, defaulting to the full pre-0.2.21 cohort when no manifest exists). Cohort entries absent from the file become `disabled` records; hooks introduced after the prior version are never absorption candidates and project as enabled. The marker is a per-target SQLite record written in the same transaction as the absorbed records — never inferred from manifest rows, which TASK-004 deletes. Supersedes the interview's earlier "verb + inference" answer.
3. **Mutation converges.** A modified Loaf entry is neither a conflict nor a fork; it is updated in place to the desired shape. For fail-closed enforcement hooks this is a security property: silently weakened enforcement cannot survive an upgrade, and opting out requires an explicit recorded act.
4. **Identity is `(target, event, hook_id)` from the built hook catalog.** `hook_id` is the stable id in `config/hooks.yaml` (`check-secrets`, `session-start-loaf`, `kb-staleness-nudge`, …), emitted per target at build time with the desired entry template so the installed CLI reconciles without the repo present. Enablement rows carry mint-once opaque primary keys plus a REAL UNIQUE constraint on the natural key. ID reuse is a semantic contract: the same ID always means the same behavior; a hook whose semantics change takes a new ID.
5. **Records tombstone; history is never rewritten.** Rows for retired hook IDs are retained and inert (reconciliation only projects catalog hooks). `absorbed_at` is immutable provenance; `updated_at` tracks toggles. A retired ID that returns in a later version inherits its enablement history — prior disable-intent is honored, and opting back in is as explicit as opting out was. `loaf hooks list` shows the current catalog only.
6. **Ownership by construction, with a closed recognition set.** An entry is Loaf's iff (a) its command invokes the Loaf executable in one of three exact forms — the `{{LOAF_EXECUTABLE}}` template form; a shell-quoted absolute path equal to the currently resolved trusted executable or a per-target previously recorded install path (recorded in state per TASK-001's schema, written by the installer per TASK-002); or the bare first token `loaf` as Cursor entries ship today — AND its normalized command matches a catalog signature or a catalog identity stem (current or historical) per Decision 13's pairing map, never a command that merely resembles one; (b) its command references a file path that normalizes to an exact Loaf-managed `hook-file` destination recorded in the installed manifest; or (c) it matches the frozen legacy allowlist (`legacyLoafHookSignatures`, `legacyLoafCommands`, and the Codex matcher-group shape as they exist at 0.2.20 — a closed list, never extended by inference). `legacyLoafPromptPrefixes` is retained as a bounded accepted risk: prefix recognition applies to `prompt`-type entries only, the live evidence contains zero foreign prompt-type entries (fixture-backed), and a foreign prompt sharing a legacy prefix being claimed is a documented residual, not a contract violation. Path normalization is a specified closed algorithm: shell-aware token extraction, `$HOME`/`~` expansion, target-root anchoring of manifest-relative destinations, quote stripping, and platform separator normalization; no symlink resolution and no directory containment. Windows parity is concrete, not asserted: template-form handlers carry `command` and `commandWindows` with identical values; resolved-path handlers carry the platform-appropriate quoting per the existing `isExactCodexJournalHookCommand`/`Windows` rules, which the recognition normalizer mirrors for both fields. Golden tests assert all 17 shipped Cursor entries and the Codex entry are recognized as Loaf's (positive), and all 32 legacy-generation entries plus the herdr entries stay unclaimed (negative). The Cursor `loaf-managed: true` marker keeps being written as human-legible provenance but is not load-bearing. A foreign entry that deliberately imitates a Loaf command stays foreign when it fails the exact-form-plus-signature-or-stem test; imitation that passes it is the operator's deliberate construction, and possible duplicate execution is their choice, not Loaf's to police.
7. **Prior-install detection is best-effort with a named residual.** A prior install exists when the installed manifest has a hook-projection row or the live file contains recognition-set entries. A pre-manifest install whose operator deleted every Loaf entry is indistinguishable from a fresh install and will project enabled defaults; this residual is accepted and documented rather than guessed at.
8. **Drift refusals die; integrity preconditions stay.** The deleted refusal class is exactly ownership/digest drift. Reconciliation still fails closed, preserving the file unchanged, on integrity violations: unparseable JSON, non-object top level, unsupported structural shapes, symlinked or non-regular destinations, I/O errors, and concurrent modification detected between read and atomic write. These are error paths with actionable messages, not plan-surface conflicts.
9. **Foreign entries are preserved value-identical and order-stable; formatting is not the promise.** Reconciliation parses the whole file (it must), but never classifies or mutates a non-Loaf entry beyond the ownership predicate returning false. The guarantee is JSON-value identity and relative order of every foreign entry and unknown field; serialization may normalize whitespace. Golden tests compare parsed values against captured raw fixtures of the live files.
10. **SQLite is the authority; the file is a projection; writers serialize through a per-target lock; what the lock cannot cover is named case by case.** Apply recomputes actions from live state at execution time (the plan is display, never a replayed script). Every writer — reconcile or verb — holds a per-target advisory lock (a lock file beside the hooks file) from state read through file publication; contention waits briefly then fails with an actionable error. Ordering inside the lock: read file and records, compute, write absorbed/enablement records and the absorption marker in one transaction, then project the file atomically with a pre-rename re-read comparison. Outside the lock's guarantee, three distinct cases: a crash after Loaf's record commit but before projection is retry-safe eventual convergence — the file is at most one reconcile behind the records and the next reconcile converges; an unlocked third-party write detected by the pre-rename comparison is an integrity abort that preserves the third party's bytes; and a third-party write landing inside the final comparison-to-rename window is an accepted, potentially irrecoverable lost write — SQLite holds no copy of foreign content, so no later reconcile can restore it, and the contract says so instead of calling it convergence. The named verb-versus-upgrade interleavings are tested. Verb writes follow the same order: record first, then reproject, all inside the lock.
11. **Enablement is host-local; syncing hook files across machines is unsupported.** The files already embed machine-specific absolute executable paths, so they were never portable. Each machine's reconciler converges its own file to its own records; a synced-in deletion is re-added at the next reconcile and a synced-in foreign entry is ignored. A portable enablement identity is out of scope by the anti-creep rabbit hole.
12. **Fresh installs default to enabled.** With no prior install detected, all catalog hooks project as enabled and no absorption runs.
13. **Pairing installed entries to catalog IDs is deterministic and closed.** Per event section, owned entries map to `hook_id` in three passes: exact match against the current desired template; a closed signature-to-ID map (each catalog ID's current and historical normalized command signatures); then identity-stem match — each catalog ID declares the identity token embedded in its command (`--hook check-secrets`, `journal context --from-hook`, `task refresh`, …), matched as an exact normalized shell-token sequence, never substring containment (`--hook check-secrets-disabled` does not match the `check-secrets` stem), and a Loaf-executable-form entry whose command contains exactly one catalog identity stem pairs to that ID even when the surrounding command was mutated, which is what lets Decision 3 converge a weakened `loaf check --hook check-secrets --advisory` back to the desired shape instead of orphaning it as foreign. Stems and signatures are generated with the catalog and validated mutually non-overlapping by a build test — ambiguity is impossible by construction, not by runtime fallback; a Loaf-executable-form command containing zero catalog stems is the operator's own loaf-invoking hook and stays foreign, and one containing multiple stems is an integrity error, not a guess. Multiple owned entries pairing to one ID converge to a single entry (the first survives converged, extras are removed — they are Loaf's by construction). Owned entries pairing to no current ID are a retired Loaf generation and are removed. Foreign entries never enter pairing.

## Planning Contract

### Approach

Replace the `hook-projection` artifact pipeline with a reconciler that operates on parsed hook files per event section. Desired entries come from the built hook catalog for the target; enablement comes from the state table; recognition of Loaf's existing entries (current or legacy shape) uses the closed by-construction predicate. The reconciler produces a per-entry action list used by the plan surface for display, applies by recomputing from live state with the existing atomic-write machinery, and verifies by re-reading and re-running recognition — not by digest.

### Hook catalog

The build emits, per target, the catalog `(target, event, hook_id, desired entry template)` derived from `config/hooks.yaml` — the same source that names `check-secrets`, `session-start-loaf`, and `kb-staleness-nudge` today. The catalog is the identity authority for reconciliation pairing, absorption cohorts, verb enumeration, and `config check` diagnosis. It also carries the 0.2.20 cohort enumeration used by this migration's absorption. Emission lands early in the reconciler unit so nothing downstream invents its own identity; dist/build parity tests update alongside.

### Recognition and normalization

The ownership predicate implements Decision 6 exactly. Path normalization: extract candidate path tokens with shell-aware splitting (respecting quotes), expand `$HOME` and `~`, anchor manifest-relative `hook-file` destinations at the target's home root (`~/.cursor` + destination), strip quoting, normalize separators per platform, and compare as absolute lexical paths — no symlink resolution, no directory prefixes. Codex executable identity reuses the existing trusted-executable resolution and the `isExactCodexJournalHookCommand(Windows)` quoting rules, tightened to require the trusted path, with `command`/`commandWindows` parity specified for both platforms. Positive fixtures: all four real path-backed entries (`kb-staleness-nudge.sh` plus the three `cat` instruction entries). Negative fixtures: every one of the 32 legacy-generation paths and the herdr entries.

### Absorption and migration marker

Absorption is a per-target, run-once migration: gate on the SQLite absorption marker (Decision 2), detect prior installs per Decision 7, restrict candidates to the prior version's cohort from the catalog, write `disabled` records plus the marker in one transaction, then converge. Required coverage matrix: fresh install, no-manifest legacy upgrade, normal upgrade, repeat upgrade, reinstall after marker exists, downgrade-then-re-upgrade, and the cohort test (one previously shipped hook deleted plus one newly introduced hook — only the former absorbs as disabled).

### Crash safety and concurrency

Decision 10's lock-and-ordering contract. The named race — upgrade reads enabled, verb commits disabled, upgrade writes the stale add — is prevented by the lock while both writers run, and resolved by eventual convergence if a writer crashes between record commit and projection. Third-party writers that do not honor the lock are handled by the pre-rename re-read comparison, with a named accepted residual: a third-party write landing in the window between that comparison and the atomic rename is lost, exactly as it would be between any two uncoordinated writers to one file — no mechanism short of OS-mandatory locking closes it, and Loaf does not pretend otherwise. Tests cover: the lock serializing verb-versus-upgrade; injected crash between record commit and file projection; injected crash between projection and manifest-row cleanup; and the read-back concurrent-modification abort for unlocked third-party writes.

### `loaf config check`

Diagnosis becomes enablement-aware with five states: disabled-and-correctly-absent (healthy), enabled-and-in-sync (healthy), enabled-but-stale — present but differing from the desired catalog template — (needs reconcile), enabled-and-missing (needs reconcile), disabled-but-present (needs reprojection); foreign/unknown entries are never reported. `--fix` routes through the reconciler rather than a private refresh path.

### Verb projection semantics

`enable`/`disable` run the full reconciler for the target (one code path, no targeted-projection variant) and report every action taken. On a converged target that is exactly one entry; when the reconcile finds other drift (a stale entry, a retired generation), the verb reports those actions too rather than pretending single-entry surgery. H3 is stated against a converged target for this reason.

### Placement

- Enablement records, absorption markers: `internal/state` (new table + accessors), user-scoped — no `project_id`. Opaque mint-once IDs, `UNIQUE(target, event, hook_id)`, immutable `absorbed_at`, tombstone retention.
- Hook catalog emission: the per-target builders (`build_cursor`/`build_codex` paths) plus a reader in `internal/cli`.
- Reconciler: `internal/cli`, replacing the hook-projection branches in `install_plan.go`, `install_target.go` (`mergeHookFiles`/`mergeCodexHookFiles`), and `build_manifest.go` (digest/refusal helpers).
- Verb surface: `internal/cli/hooks.go` (`runHooks`), registered in `cli.go` dispatch, consuming the catalog.
- Fixtures: sanitized raw captures of the live machine's hooks files exist at `research/fixtures/codex-hooks-live.json` and `research/fixtures/cursor-hooks-live.json` (usernames normalized), alongside the derived classification and the predicate-free before/after comparator `research/compare_hook_files.py` that TASK-005 invokes. The classification script is marked as 0.2.20-predicate provenance evidence, not the acceptance oracle.

### Risks

- **Codex placeholder and quoting.** The dist entry carries `{{LOAF_EXECUTABLE}}`; installed entries carry a shell-quoted absolute path plus `commandWindows` parity. Equality and recognition normalize both forms via the tightened trusted-executable rules.
- **Absorption misfire on fresh installs.** Guarded by Decisions 2, 7, and 12 and the coverage matrix; the no-prior-install regression test is mandatory.
- **Cohort accuracy.** The 0.2.20 cohort enumeration must match what 0.2.20 actually shipped (17 Cursor entries, 1 Codex entry, by hook ID); the catalog test pins it against the 0.2.20 dist fixtures.
- **Pre-manifest fully-deleted installs.** Decision 7's named residual: such machines re-add enabled defaults once; the operator disables via the verb. Documented in release notes.

### Sequencing

TASK-002 needs TASK-001's records and emits the catalog it consumes (catalog work is TASK-002's first step, not TASK-003's); TASK-003 needs TASK-001 and TASK-002; TASK-004 lands after TASK-002 has replaced the code paths it deletes and owns the `config check` rework; TASK-005 runs last against the real machine. Units below are ordered by likelihood-of-change for review, which happens to match this sequence.

### Coordination

The state-dedupe Change (docs/changes/20260807-state-dedupe, in flight) owns the identity discipline this Change's schema conforms to. No shared tables; merge order is irrelevant at the schema level. If both land migrations in the same release window, release notes should present them as one state-hygiene story.

## Implementation Units

- **TASK-001 — Hook-enablement state.** User-scoped enablement table, absorption markers, and accessors in `internal/state`: mint-once opaque IDs, `UNIQUE(target, event, hook_id)`, absence-means-enabled reads, immutable `absorbed_at`, tombstone retention, transactional absorb-and-mark write.
- **TASK-002 — Hook catalog and entry-level reconciler.** Catalog emission per target; closed recognition predicate with the specified normalization; per-event convergence; cohort-restricted run-once absorption; integrity preconditions; crash-safe recompute-on-apply; golden tests from sanitized live fixtures proving foreign entries survive value-identical, plus the full migration coverage matrix and Windows parity cases.
- **TASK-003 — `loaf hooks` verb surface.** `list`/`enable`/`disable` over the enablement records with immediate reprojection, consuming the catalog; record-then-reproject ordering; catalog-only listing with absorption provenance.
- **TASK-004 — Retire the hook-projection kind and rework `config check`.** Delete digest and drift-refusal paths; keep integrity preconditions; installed-manifest reader drops obsolete rows; plan output speaks per-entry actions; `config check` five-state diagnosis with `--fix` through the reconciler; dist/build parity tests updated.
- **TASK-005 — Canary acceptance evidence.** Real `loaf upgrade` on this machine recorded as change evidence: zero drift refusals, Codex `session-start-loaf` absorbed to disabled, all 33 Cursor foreign entries (the 32-entry legacy generation plus one herdr) and the Codex herdr entry preserved value-identical, idempotent second run, verb round-trip.

## Verification Contract

- **V1.** The full Go suite passes, including the reconciler's migration coverage matrix (fresh install, no-manifest upgrade, normal/repeat upgrade, reinstall, downgrade-re-upgrade, cohort old-deleted-plus-new-introduced), integrity precondition cases (malformed JSON, non-object top level, unsupported shape, symlink destination, concurrent modification), recognition fixtures (all 17 shipped Cursor entries plus the Codex entry positive; 32 legacy plus 2 herd negative), pairing cases (exact match, historical signature, duplicate-owned convergence, retired-generation removal, non-overlap catalog validation), lock serialization and crash-injection ordering tests, and Windows `commandWindows` parity shapes. Command: `go test ./...`. Expect: exit 0.
- **V2.** All targets build with the retired artifact kind, the emitted hook catalog, and updated parity tests. Command: `loaf build`. Expect: exit 0.
- **V3.** The Change stays structurally executable. Command: `loaf change check docs/changes/20260808-hooks-entry-reconciliation`. Expect: exit 0.

- **H1.** Canary evidence shows the first 0.2.21 upgrade absorbing the Codex `session-start-loaf` entry as disabled (no re-add, no refusal), and a second run reporting nothing to do.
- **H2.** Evidence shows every non-Loaf entry in both live files value-identical and order-stable across the upgrade — the Codex herdr entry and all 33 Cursor foreign entries (32 legacy-generation plus one herdr) — proven by `research/compare_hook_files.py` output whose reported differences name only expected Loaf entries.
- **H3.** `loaf hooks disable` / `enable` round-trip on a real, already-converged target edits exactly one entry in the file and nothing else, and its output names every action taken.
- **H4.** `loaf config check` on the canary reports the disabled Codex hook as healthy-absent, not missing.

## Definition of Done

- Reconciliation replaces the hook-projection pipeline for Codex and Cursor; no code path can refuse a hooks file for drift, and integrity failures preserve the file unchanged with actionable errors.
- The canary machine upgrades with zero drift refusals; its disable-intent exists as a queryable record with immutable absorption provenance; re-running is a no-op.
- Non-Loaf entries are value-identical and order-stable in every recorded run and golden test.
- `loaf hooks list/enable/disable` works against installed targets without the repo present, from the built catalog.
- `loaf config check` distinguishes the five enablement states (including enabled-but-stale) and `--fix` converges through the reconciler.
- V1–V3 green; H1–H4 reviewed with evidence in the Change folder.

## Durable Outputs

- ADR candidate: ownership-by-construction with closed recognition sets for shared config surfaces — the JSON-entry analog of readlink containment and fenced sections, proven against a live foreign-entry population.
- ARCHITECTURE.md: hook model section updated from whole-file managed artifacts to entry-level reconciliation with host-local enablement state and the built hook catalog.
- Knowledge note: the absorb-once-with-durable-marker pattern (cohort-restricted inference at migration, pure projection after) for any future managed-surface conversion.

## Open Questions

- [UK] The exact per-entry action vocabulary in plan output (`add`/`update`/`remove`/`absorb` naming and formatting) — react to the first implemented plan output during TASK-002 review.
