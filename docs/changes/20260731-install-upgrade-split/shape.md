<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Split `loaf install` and `loaf upgrade`

## Problem

One command conflates two operations with different natural scopes. `loaf install --upgrade` couples the machine-global harness content sync (`~/.cursor`, `~/.codex`, OpenCode, Amp) with project-file enforcement that fires wherever the command is run: project-root resolution never fails, so running it from a non-Loaf directory scaffolds `AGENTS.md`, a fenced section, and `.claude/CLAUDE.md` right there. Meanwhile nothing detects drift after a release: `.loaf-version` markers are written on install but never read back, `loaf config check` diffs hook IDs only, and the binary goes stale silently — v2.0.0-alpha.17 shipped while the PATH binary and every harness config dir sat at alpha.16, the same failure shape as the alpha.15 receipt incident one release earlier.

## Hypothesis

If onboarding and upgrading become separate commands that both know whether they are standing in a Loaf-powered repo, then `loaf upgrade` becomes a safe run-anywhere habit, project scaffolding stops littering non-Loaf directories, and post-release drift is caught where it is visible — upgrade output, doctor, session start — instead of by incident.

## Scope

**In**

- A tiered Loaf-repo detector: SQLite project record (authoritative) → fenced `AGENTS.md` marker or `.agents/loaf.json` (strong) → legacy `.agents/` folders such as `specs/`, `drafts/`, `sessions/` (weak, requires explicit user confirmation) → none.
- A new `loaf upgrade` command: harness content sync from the installed distribution plus deprecation cleanup always; project-surface refresh (fenced sections, symlinks, migrations, and MCP-recommendation refresh of `.agents/loaf.json`) only when the detector confirms a Loaf repo. The `--dry-run --json` plan surface moves here, and the plan builder becomes command-aware: every apply/follow-up command it emits names `loaf upgrade`, never the removed flag. `--to <target>` filters to already-installed targets; naming an uninstalled target errors with a pointer to `loaf install --to <target>`.
- `loaf install` retargeted to onboarding: outside a Loaf repo it asks before scaffolding project files; inside one it no-ops the project part and suggests `loaf upgrade`; `--to <target>` remains valid for adding a net-new harness. The consent gate covers **all** project writes — `AGENTS.md`, fenced sections, symlinks, and the MCP-recommendation writes to `.agents/loaf.json` — not just file scaffolding.
- Hard removal of `install --upgrade`: the flag errors with a pointer to `loaf upgrade`.
- Install-channel detection (Homebrew keg, npm, dev checkout) with a best-effort, non-blocking binary currency advisory that prints the exact upgrade command and never executes it.
- Harness-level drift surfacing: a doctor check comparing each installed harness's `.loaf-version` marker to the binary version, and a one-line SessionStart nudge when stale (cuttable unit).
- Reference sweep: loaf-reference skill docs, doctor remediation strings, help text, CLI reference, rebuilt `dist/` and `plugins/` artifacts.

**Out** (deferred, not rejected)

- Executing the package-manager upgrade (`brew upgrade loaf` with re-exec continuation) — named follow-up once detect-and-advise proves insufficient.
- Claude Code content delivery — stays on the native plugin-marketplace channel, untouched by `loaf upgrade`.

**Cut** (explicitly rejected)

- Any new state entity, lifecycle, or status for installs or upgrades.
- Network-gated behavior: the currency advisory is best-effort with a short timeout and silent skip; upgrade must be fully functional offline.
- Adding `loaf upgrade` (or any install/upgrade leaf) to the Codex basic-command authority prefixes — it is operator authority.

## Observable Workflow

```
# anywhere, after a release
loaf upgrade                 # syncs all installed harness config dirs from the installed keg,
                             # runs deprecation cleanup, then: "binary 2.0.0-alpha.16;
                             # 2.0.0-alpha.17 available — run: brew upgrade loaf"

# in a directory that is not a Loaf repo
loaf install                 # "No Loaf project detected here. Deploy Loaf to this folder? [y/N]"
loaf upgrade                 # global part only; no project files created

# in a Loaf repo
loaf install                 # "Loaf is already deployed here. Run `loaf upgrade` to refresh."
loaf install --to cursor     # still valid: onboard a net-new harness
loaf upgrade                 # global (skipped when current) + project refresh
loaf upgrade --to cursor     # filters to that installed target; errors with a
                             # pointer to `loaf install --to` if cursor is absent
loaf install --upgrade       # error: flag removed; use `loaf upgrade`

# legacy signals only (.agents/specs/ present, no marker, no SQLite record)
loaf upgrade                 # "Legacy Loaf artifacts found. Is this a Loaf project? [y/N]"
                             #   yes → full upgrade path (including migrations)
                             #   no  → global part only, offer `loaf install`
```

At the next conversation start in any harness after a stale release, the SessionStart context output carries one line: `Loaf content in this harness is 2.0.0-alpha.16; binary is 2.0.0-alpha.17 — run loaf upgrade`.

## Rabbit Holes and No-Gos

- **The re-exec self-upgrade protocol.** Executing `brew upgrade loaf` and continuing with the new binary is a real protocol (binary replacement mid-run, channel divergence, npm/dev fallbacks). It is the named follow-up, not a stretch goal here.
- **Detector scope creep.** The tiered detector answers one question — "is this a Loaf repo?" — for two commands. It is not a general project-registry or discovery feature.
- **Blocking on the network.** The currency check must never delay or fail the content sync. Short timeout, silent skip, advisory line only.
- **Rewriting the installer.** `installTargetDistribution`, the deprecation manifest machinery, and the plan builder are reused as-is. This Change moves command boundaries; it does not redesign the install engine.
- **Prompt hangs in automation.** Every new prompt (deploy consent, legacy confirmation) needs a deterministic non-TTY behavior: report what consent is required and exit cleanly, never hang or assume yes.

## Decisions

Provenance: shaping interview 2026-07-31 (this conversation), following the alpha.16→17 drift discussion that motivated the Change. Materializes the tracked Intent `INTENT-20260725-split-install-and-upgrade-by-scope-and-detect-a-newer-binary`, which anticipated both halves of this split. Review round 1: Codex (gpt-5.6-luna, xhigh) — nine findings, all dispositioned in [reports/20260801-024619-review-codex.html](reports/20260801-024619-review-codex.html); decisions 7–9 below entered through that round. Review round 2 (post-implementation): Loaf Sentinel on the full branch — APPROVE, 0 P1 / 3 P2 / 3 P3, all dispositioned in [reports/20260801-063956-review-sentinel.html](reports/20260801-063956-review-sentinel.html), fixes in commit 38d73439. Review round 3 (adversarial code pass): Codex (gpt-5.6-luna, xhigh) — REQUEST CHANGES, 2 P1 / 4 P2, all dispositioned in [reports/20260801-153342-review-codex.html](reports/20260801-153342-review-codex.html) (finding 6's scenario refuted, fix taken as hardening), fixes in commit e42fd864. Review round 4 (merge gate, resumed Codex thread): verified F4/F6 fixed and F1/F2/F3/F5 partial, five delta findings — 1 P1 / 4 P2 — all dispositioned in [reports/20260801-180514-review-codex-gate.html](reports/20260801-180514-review-codex-gate.html) (D5 modified: malformed-header fence routes to the legacy confirmation tier), fixes in commit dea0660f. Review round 5 (confirmation): D2/D3/D4/D5 verified fixed; D1-residual + N1 (project readers bypassing the descriptor hardening) dispositioned in [reports/20260801-185612-review-codex-confirm.html](reports/20260801-185612-review-codex-confirm.html), closed uniformly in commit 23733c82. Review round 6 (class closure): D1-residual/N1 verified fixed; N2 + N3 (symlink-migration and MCP/Codex config readers) dispositioned in [reports/20260801-204103-review-codex-closure.html](reports/20260801-204103-review-codex-closure.html) and closed terminally in commit 709b819f with a 75-site audit of every remaining plain read.

1. **Tiered detection with a confirmation floor.** SQLite project record is authoritative; fenced `AGENTS.md` marker or `.agents/loaf.json` is strong enough to proceed (the basis is printed); legacy `.agents/` folders alone trigger an explicit "is this a Loaf project?" prompt — yes routes to the upgrade path, no routes to a proposed install. Forecloses any single-file identity rule.
2. **`install --upgrade` is hard-removed now.** The flag errors with a pointer to `loaf upgrade`; the documented maintenance contract (`--dry-run --json` planning flow) moves to `loaf upgrade` in the same Change. Alpha is the cheapest breaking window. Forecloses a dual-canonical-name period.
3. **Binary currency is detect-and-advise only.** Channel detection plus a best-effort version check, printing the exact command. Execution is foreclosed for this Change (deferred, see Scope Out).
4. **Drift surfacing rides along as cuttable units.** The doctor check and SessionStart nudge complete the "when run" half of the original ask; they are severable if the core split runs long.
5. **`loaf upgrade` is operator authority.** It mutates global user config; it is never added to `basicCommandAuthorityPrefixes`.
6. **Consent semantics carry over unchanged.** Destructive deprecation cleanup keeps explicit `-y`; non-interactive runs without it report the required consent rather than assuming it.
7. **All project writes sit behind the detector gate.** The MCP-recommendation flow writes `.agents/loaf.json`; that is a project write like any other — install may only do it under onboarding consent, and in a Loaf repo it belongs to upgrade's project part. Forecloses "no-op except config writes." (Review round 1, finding 2.)
8. **`upgrade --to` filters, never onboards.** It narrows the sync to already-installed targets; an uninstalled target is an error pointing at `loaf install --to`. Onboarding stays install's exclusive job. (Review round 1, finding 9.)
9. **The plan surface is command-aware.** Every apply/follow-up command the dry-run plan emits names `loaf upgrade`; a machine assertion guards that no emitted command contains the removed flag. (Review round 1, finding 1.)

## Planning Contract

### Placement

- **Detector:** new file in `internal/cli` (working name `loaf_repo_detection.go`) building on `internal/project` root resolution, a state-DB project lookup, and file probes for the fenced marker, `.agents/loaf.json`, and legacy folders. Returns tier + evidence basis, never prompts itself — prompting stays in the command layer.
- **`loaf upgrade`:** new `runUpgrade` in `internal/cli/upgrade.go`, dispatched from `cli.go`. Reuses `installTargetDistribution`, `runInstallDeprecationCleanup`, and the `install_plan.go` plan builder. Gains the `--dry-run`/`--json`/`-y` flags with install's existing semantics.
- **`loaf install`:** keeps harness detection and onboarding; `--upgrade` parsing becomes a hard error naming `loaf upgrade`; project-file enforcement gains the consent prompt (outside) and no-op-with-suggestion (inside) branches.
- **Channel detection:** resolve the running binary's provenance — Homebrew keg path / `INSTALL_RECEIPT.json`, npm global tree, or dev checkout — alongside `resolveInstalledDistributionRoot` in `distribution.go`.
- **Drift surfacing:** one new doctor check (harness `.loaf-version` vs binary version); the SessionStart nudge lands in the shared emitter behind the target-specific `--from-hook` dispatch variants (Claude, Cursor, and Codex each invoke their own form — see `journal.go` dispatch and the `build_{target}.go` hook commands), so every harness gets it from one code path.

### Currency advisory contract

- **Channel identity:** Homebrew — keg path pattern plus `INSTALL_RECEIPT.json`; npm — global tree containing the launcher, package name read from its `package.json`; dev checkout — distribution root inside a git worktree. Anything else: unknown channel, no advisory.
- **Source of truth:** GitHub Releases for the repo, one source for all channels. A prerelease binary compares against the latest release including prereleases; a stable binary compares against the latest stable only.
- **Comparison:** semver with prerelease ordering. Unparseable versions on either side → no advisory.
- **Budget:** one second hard total; any timeout, network, or parse failure degrades to no advisory with identical output and exit code otherwise.
- **Advisory text:** current version, available version, and the channel's exact command — `brew upgrade loaf`, `npm update -g <package>`, or (dev checkout) `git pull && npm run build`.

### Drift marker semantics

- The invoking harness maps to its config dir through the same target table install writes markers with; the nudge concerns only that harness's marker.
- Marker equal to binary version → silent. Marker parses older → nudge/doctor-flag. Marker missing or unparseable → SessionStart stays silent; doctor reports it as unknown state with `loaf upgrade` as the remediation. Marker *newer* than the binary → doctor flags the binary as the stale side (points at the channel upgrade command, not `loaf upgrade`).

### Risks

- **Plan JSON consumers.** The loaf-reference maintenance flow documents the dry-run plan schema. The moved surface must emit a compatible document or version it deliberately — decided inside the upgrade-command task.
- **Shared code paths.** `config check --fix` calls `installTargetDistribution` directly; retargeting install must not change that path's behavior.
- **Prompt behavior under automation.** Both new prompts need explicit non-TTY handling (see Rabbit Holes) and test coverage for it.

### Sequencing

The detector leads — both commands branch on it. The upgrade command follows, then install retargeting (its messages point at an upgrade command that must exist). Channel advisory extends upgrade's output. Drift units build on stamped markers plus the upgrade command's existence. The reference sweep lands last, over final command names and output — its packet is blocked by every other unit so the graph enforces this, not just prose.

## Implementation Units

- **TASK-001 — Tiered Loaf-repo detector.** Detection tiers, evidence basis, non-prompting API, tests per tier.
- **TASK-002 — `loaf upgrade` command.** Global/project split on the detector, moved plan surface, consent semantics, help + agent-help entries.
- **TASK-003 — `loaf install` retargeting.** Onboarding consent, no-op + suggestion, `--to` carve-out, `--upgrade` hard error.
- **TASK-004 — Channel detection and currency advisory.** Homebrew/npm/dev-checkout provenance, best-effort check, advisory line.
- **TASK-005 — Harness drift surfacing.** Doctor check + SessionStart nudge. Cuttable.
- **TASK-006 — Reference sweep.** Skill docs, doctor strings, CLI reference, rebuilt artifacts.

## Verification Contract

The behavioral matrix (detector tiers, prompt routes, advisory cases, drift output per harness) is carried by the unit tests each packet mandates — V1 is the gate that runs them; the V-entries below pin the surfaces a unit test can't: shipped artifacts, emitted plan text, and the consent boundary observed from a real invocation.

- **V1.** Full test suite passes, including the mandated matrix/prompt/advisory/drift tests. Command: `go test ./...`. Expect: exit 0.
- **V2.** The upgrade command exists with its own help surface. Command: `go run ./cmd/loaf upgrade --help`. Expect: exit 0 and contains `loaf upgrade`.
- **V3.** `install --upgrade` is a hard error pointing at the replacement. Command: `go run ./cmd/loaf install --upgrade --dry-run`. Expect: exit 1 and contains `loaf upgrade`.
- **V4.** Compile check passes. Command: `npm run typecheck`. Expect: exit 0.
- **V5.** No live surface — source, content, README, AGENTS.md, or rebuilt artifacts — references the removed flag (tests excluded; the tombstone's wording is specified to avoid the literal phrase, so no source exception exists). The flags precede `--` because `--` ends option parsing, and `-I` skips binary files: `plugins/loaf/bin/native/` carries cross-compiled binaries for platforms other than the build host, which only the release pipeline refreshes, so they embed whatever strings their release shipped. Text surfaces are what this gate judges. Command: `bash -c 'grep -rqI --exclude="*_test.go" -- "install --upgrade" internal/cli/ content/ dist/ plugins/ README.md AGENTS.md'`. Expect: exit 1.
- **V6.** The dry-run plan never emits the removed command in any apply/follow-up text. It runs the built launcher rather than `go run`, which cannot resolve a distribution root from its temporary build path and would exit before emitting a plan — passing the check on empty input. V9 is the positive twin that proves a plan was emitted at all. Command: `bash -c 'LOAF_DB="$(mktemp -d)/v6.sqlite" ./bin/loaf upgrade --dry-run --json | grep -q -- "install --upgrade"'`. Expect: exit 1.
- **V9.** The dry-run plan V6 judges is a real plan that names the new command. Command: `bash -c 'LOAF_DB="$(mktemp -d)/v9.sqlite" ./bin/loaf upgrade --dry-run --json'`. Expect: contains `"command": "upgrade"`.
- **V7.** The full build (binary, CLI reference, all content targets) succeeds, so V5 judges fresh artifacts. Command: `npm run build`. Expect: exit 0.
- **V8.** Install outside a Loaf repo creates nothing without consent (uses the binary V7 built; run after it). Command: `bash -c 'd="$(mktemp -d)"; r="$PWD"; (cd "$d" && LOAF_DB="$d/db.sqlite" "$r/bin/loaf" install </dev/null >/dev/null 2>&1); test ! -e "$d/AGENTS.md" -a ! -e "$d/.agents" -a ! -e "$d/.claude"'`. Expect: exit 0.

- **H1.** Run `loaf upgrade` from a non-Loaf directory (temp dir, isolated `LOAF_DB`): harness dirs sync, no project files appear. Run it in this repo: project surfaces refresh. Run `loaf install` in both: consent prompt outside, no-op suggestion inside.
- **H2.** *(Applies while TASK-005 is in scope — see Definition of Done.)* Downgrade one harness's `.loaf-version` marker by hand: doctor flags it and the SessionStart context output carries the nudge line in each harness variant.
- **H3.** The legacy-confirmation prompt (fixture dir with only `.agents/specs/`) reads clearly and routes yes→upgrade, no→install offer; non-TTY runs report required consent instead of hanging.
- **H4.** The dry-run plan JSON read side-by-side with the documented maintenance flow: schema decision (compatible vs versioned) matches what the loaf-reference doc now teaches.

## Definition of Done

- All V-entries green via `loaf change verify`; H-entries demonstrated in review material.
- The Observable Workflow matrix behaves as written on this machine (dogfood: recover the current alpha.16→17 drift with `brew upgrade loaf` + `loaf upgrade`).
- No `install --upgrade` references remain in shipped content, help, or doctor output (changelog and prior Changes keep their history).
- `dist/` and `plugins/` artifacts rebuilt and committed with the source changes.
- Drift surfacing (TASK-005): shipped here with H2 demonstrated, **or** cut with a named follow-up captured (Intent or brief) before merge — one of the two, never silently dropped.
- `INTENT-20260725-split-install-and-upgrade-by-scope-and-detect-a-newer-binary` gets its terminal disposition via `loaf intent resolve` — owned by TASK-006's closing step.

## Durable Outputs

- Candidate ADR after implementation proves the shape: install/upgrade command boundary and the tiered Loaf-repo detection model.
- loaf-reference skill updates (maintenance and configuration references) — carried by the sweep task, finalized against real output.
- Changelog entry for the breaking `install --upgrade` removal.

## Open Questions

- [KU] Plan JSON schema for the moved `--dry-run --json` surface: byte-compatible or deliberately versioned? → decided inside TASK-002 against the documented consumer flow; H4 reviews the outcome.
- [UK] Exact wording of the two new prompts (deploy consent, legacy confirmation) — recognizable when seen → H3 review during implementation.

Resolved this round: the currency-check source, comparison, and timeout `[KU]` closed into Planning Contract › Currency advisory contract (review round 1, finding 7).
