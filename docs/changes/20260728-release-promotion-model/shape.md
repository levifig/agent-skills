<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Release Promotion Model — Channels, Designation-Commit Promotion, Gate at RC

## Problem

The release machinery has exactly one channel concept: the `releaseVersionIsPrerelease` predicate (`release_dry_run.go:876`). Everything a real prerelease cycle needs is missing or wrong:

1. **The label can never transition.** `bumpReleaseVersion` preserves the prerelease label verbatim — there is no alpha→beta→rc path, and no stable→prerelease entry at all (`bumpReleaseVersion("2.0.0","prerelease")` returns `""`), so opening 2.1.0-alpha.1 after 2.0.0 stable is impossible.
2. **Nothing controls what an rc contains.** The cohort gate fires only when the candidate is stable; every prerelease bypasses (`change_release_gate.go:43-48`). An rc — whose semantic contract is "this exact content intends to be the stable" — is cut with zero evidence checks, so stabilization never formally opens.
3. **The changelog is channel-blind.** Every release, rc included, must splice a `## [version]` section (post-merge guardrails 5/6), forcing curated changelog ceremony onto disposable candidates and leaving the stable section as just another entry instead of the authoritative rollup.
4. **Distribution is channel-blind.** CI updates the Homebrew formula unconditionally on any `v*` tag and omits `--prerelease` on its create-fallback; the moment a stable exists, the next alpha would point the tap at a prerelease and `brew upgrade` would hand stable users an alpha.
5. **Guidance is stale.** The release skill predates the gate entirely (never mentions `--post-merge`, `target_release`, receipts), and the `--bump` help text has three-way drift including retired lineage-freeze language (`cli_reference.go:90`).

## Hypothesis

Encoding channels as alpha→beta→rc→stable-with-stable-as-a-promoted-rc gives the release the same evidence discipline the change work model gave the work: the gate opens stabilization at rc cut, promotion is a designation event whose byte-identity is mechanically enforced on the code tree, the stable changelog becomes the authoritative rollup users actually need, and stable consumers can never be handed a prerelease. If this ships before 2.0.0, the first stable in the project's history ships through the promote path — the model proves itself on the release it was designed for.

## Scope

**In**

- Rung model in the version primitives: parse prerelease label + numeric suffix, ladder ordering alpha < beta < rc, cycle entry, rung advance, promotion math.
- CLI surface: `--promote [rung|release]`, `--channel` as outcome assertion, `--bump release` retirement, flag validation matrix, help-surface sync across all three copies (including removing stale lineage-freeze text).
- Gate placement: cohort gate fires on rc candidates (mapped to the cohort by core literal) and at promotion; alpha/beta remain the valve; post-merge gates by prepared rung.
- Designation-commit promotion: diff-since-last-rc restricted to version files + CHANGELOG, stable rollup derivation seeded from cohort `shape.md` lines, collapse of the cycle's prerelease sections, mechanical changelog validation.
- Changeset-pattern release notes: each change authors its user-facing fragment as `release-notes.md` in its own folder (never deleted, archived with the change); consumption derives from the release range plus execution provenance; CHANGELOG.md becomes a cut-time projection; the `[Unreleased]` buffer retires; the `workflow-pre-pr` hook re-points to per-change note presence with a no-user-facing-impact escape.
- Per-channel changelog behavior, write path included: alpha/beta cuts aggregate ranged notes into their sections, rc cuts touch no changelog (the writer is rung-aware, not just the guardrails), feat-during-rc preflight warning, rc GitHub Release notes auto-derived from the commit range.
- Channel-routed distribution: CI derives the channel from the tag, `--prerelease` on the create-fallback, `loaf` formula tracks stable only, `loaf-dev` formula tracks the latest prerelease.
- Release skill rewrite: channel ceremony, `/promote` flow, human channel choice with agent suggestion, current CLI vocabulary (gate, receipts, post-merge).

**Out** (deferred, not rejected)

- Receipt content binding — standalone predecessor Change (`docs/changes/20260728-receipt-tree-binding/`, shaped from `INTENT-20260728-receipt-freshness-must-survive-squash-merges-bind-to-tree-content-not-commit-sha`, gating scope decided by council 2026-07-28): receipts bind to a masked root-tree digest whose exclusion set includes the release-metadata allowlist, so designation-legal ⟺ receipt-neutral. This Change imports its exported `releaseMetadataAllowlist` for the designation check — the allowlist alone, never the full digest-exclusion set, which also masks receipts/ and reports/.
- Capability-evidence decoupling — its own Intent (`INTENT-20260728-capability-evidence-decouples-from-binary-hashes-and-exact-client-versions`).
- Concurrent maintenance branches and overlapping cycles (a 1.x patch train while 2.x stabilizes) — revisit when a maintenance need is real; the model assumes one active cycle on main.
- npm dist-tag channel routing — Loaf does not publish to npm today; becomes an Intent if that changes.

**Cut** (explicitly rejected)

- Backward channel moves (rc→beta). Semver orders `beta < rc`, so a later backward cut sorts older than what is already released; blocked hard, no override.
- Tag-only promotion and artifact promotion. Rejected in interview: both make some surface lie about the version (guardrail 4, CI's tag-vs-package.json verify, the Homebrew formula's `loaf --version` assertion, or the binary's self-report).
- CI-authored promotion commits. The gate reads committed evidence; a bot commit to main outside the PR flow relocates the authorship question without answering it.
- A full semver-precedence engine. Only rung-rank comparison on the recognized ladder is needed.
- Prerelease-expressible `target_release` literals. Cohorts target stables only, by design; rc gating maps the candidate to the cohort, never the cohort to a prerelease.

## Observable Workflow

```
# open the next cycle from stable 2.0.0
loaf release --bump minor --channel alpha      # → 2.1.0-alpha.1

# iterate within the channel
loaf release --bump prerelease                 # → 2.1.0-alpha.2

# advance the ladder
loaf release --promote                         # → 2.1.0-beta.1
loaf release --promote                         # → 2.1.0-rc.1   ← cohort gate fires here
loaf release --promote rc                      # from alpha: deliberate skip, warns

# promote to stable (explicit, rc-only)
loaf release --promote release                 # designation commit: version files + CHANGELOG rollup + regenerated build outputs
loaf release --post-merge                      # tags v2.1.0, publishes, `loaf` formula updates

# guarded automation
loaf release --promote --channel beta          # asserts the result lands on beta, else refuses before mutating

# every change authors its user-facing fragment in its own folder; cuts aggregate them
docs/changes/20260801-some-feature/release-notes.md   # [Unreleased] buffer retired; CHANGELOG is a cut-time projection
```

An rc cut touches version files only — its GitHub Release notes are auto-derived from the commit range, CHANGELOG.md is untouched, and cutting an rc whose range contains `feat:` commits warns "new development during rc; consider cutting a beta." At promotion the CHANGELOG diff shows the cycle's alpha/beta sections collapsing into one authoritative `## [2.1.0]` rollup. `brew install levifig/tap/loaf` only ever resolves stable; `loaf-dev` tracks the latest prerelease for dogfooding.

## Rabbit Holes and No-Gos

- **Do not grow rung math into semver precedence.** The ladder is three known labels and a numeric suffix; unknown labels iterate with `--bump prerelease` but refuse `--promote`. Full identifier-by-identifier comparison is rejected ceremony.
- **Do not let the rollup become generation.** The CLI seeds material (the cycle's `release-notes.md` fragments, the prerelease sections, and `shape.md` Problem/Hypothesis lines only as a warned fallback for note-less changes) and validates the result; the agent judges and writes. Auto-generating the stable changelog from commit subjects is the failure mode Common Changelog curation exists to prevent.
- **Do not relax guardrail 4 anywhere.** Tag equals version files stays absolute on every path; the promotion design was chosen specifically so no surface needs to lie.
- **Do not redefine receipt freshness here.** The receipt-tree-binding predecessor owns the digest, the predicate, and the evidence-boundary constant; this Change imports the boundary for its designation check and never forks it. If the two sets ever need to diverge, the digest exclusion stays the strictly smaller one (council: Correctness).

## Decisions

Provenance: owner-settled design in `INTENT-20260728-release-promotion-model-alpha-beta-rc-and-stable-as-a-promoted-rc` (2026-07-28), refined in the shaping interview of the same day (journal `decision(shape)` entries); code facts from the release-surface survey of this session.

1. **Promotion is a designation commit.** One commit touching only release metadata — version files, CHANGELOG, and the tracked regenerated build outputs the version injection rewrites — tagged and published on existing rails; the preflight blocks if the diff since the last rc touches anything else. Byte-identity attaches to the source tree; artifacts rebuild deterministically with the honest stable version string. Forecloses tag-only and artifact promotion, and any guardrail-4 relaxation.
2. **The gate fires at rc cut and re-checks at promotion.** An rc candidate gates the cohort matching its core literal; alpha and beta remain the valve. Stabilization formally opens when the cohort is executed and receipt-verified.
3. **Changelog policy is per-channel, authored as changeset-pattern release notes.** Each change carries its user-facing fragment as `release-notes.md` in its own folder — authored while context is hot, never deleted, archived with the change; consumption is derived from the release range plus execution provenance, so a note ships only when its work actually landed. CHANGELOG.md becomes a cut-time projection: alpha/beta cuts aggregate the ranged notes into their sections, rcs touch no changelog, and the stable rollup re-aggregates the cycle's notes at promotion, collapsing the prerelease sections. The `[Unreleased]` buffer retires, and `workflow-pre-pr` re-points to per-change note presence; the no-user-facing-impact escape's carrier is the note file itself — a change with nothing user-facing still writes `release-notes.md`, and the declaration's machine-readable form is exact: the note's entire content is the single line `No user-facing impact.`, recognized mechanically by the collection helper and skipped by aggregation. Grounded in industry practice (the stable document is authoritative, prerelease notes are disposable delta notes) and in the buffer's real failure mode: a single global section every PR edits is pure merge contention.
4. **The surface is `--bump` iterates, `--promote` advances, `--channel` asserts.** Bare `--promote` walks one rung; a named rung is a warned skip; `release` must be named explicitly and requires the current version to be an rc. `--channel` never selects a move — it guards the outcome, refusing before mutation when the computed result lands elsewhere; it is required only at cycle entry. `--bump release` retires with an error pointing at `--promote release`.
5. **Direct stable bumps survive as the hotfix path.** `--bump patch` from 2.0.0 producing 2.0.1 stays legal (and gates its cohort as today); the promoted-rc rule governs how cycles end, not whether every stable needs a cycle.
6. **Forward-only is enforced hard.** Backward channel cuts sort older than existing releases under semver precedence; there is no override flag.
7. **Distribution routes by channel with two formulae.** `loaf` tracks stable only; `loaf-dev` in the same tap tracks the latest prerelease; CI routes by the tag's channel. Stable users can never be upgraded onto a prerelease; brew-based dogfood continues via `loaf-dev`.
8. **Alpha is kept over dev.** Semver prerelease identifiers compare ASCII-alphabetically; `dev` would sort after `beta` and mis-order in any range-resolving tooling. (Owner decision, restated from the Intent.)
9. **Agent judges, CLI performs.** Channel choice is human with agent suggestion (cohort states, criteria green-ness, dogfood time — surfaced via AskUserQuestion in the release skill); the rollup is judged agentically and validated mechanically (section exists, version matches, non-empty, sections collapsed). Entry-level tracing stays H-tier review.
10. **Receipt tree-binding is a predecessor, not a passenger.** It must land before the sweep carrier verifies on its branch, independent of this Change's timeline; bundling would couple an urgent correctness fix to a design change. Its council settled the gating scope (masked root-tree digest, exclusion set = receipts ∪ reports ∪ release-metadata allowlist), and this Change consumes the exported boundary for the designation check.
11. **Tag creation requires CI green at HEAD.** Adopted council rider (Release Engineering): rc cut and promotion refuse to tag unless a successful CI run exists for the exact HEAD SHA (`gh run list --commit`), blocking with a named remedy when absent. Closes the tag-before-test ordering hole; the gate still executes nothing itself — a CI conclusion keyed by commit SHA is committed-adjacent evidence read at cut time.

## Planning Contract

### Rung model

Extend the hand-rolled version primitives (`release_dry_run.go:865-902`): split the opaque prerelease string into label + numeric suffix at the last dot (dotless or non-numeric suffixes keep today's reset-to-`.1` behavior on iteration). Recognized ladder: `alpha` (1) < `beta` (2) < `rc` (3); unknown labels are not rungs — `--bump prerelease` still iterates them, `--promote` refuses with a named reason. New moves: cycle entry (core bump + `alpha.1`), rung advance (next or named rung, counter resets to `.1`), promotion (rc → bare core). `releaseSemverLess` stays core-only for cohort warnings.

### Surface and snapshot

`releaseSnapshot` gains the move kind and asserted channel; `--promote` is mutually exclusive with `--bump`, composes with `--dry-run`/`--yes`/`--base`/`--tag`/`--no-gh`; `--channel` composes with `--bump` and `--promote` and is validated after candidate computation, before any mutation. `--promote` and `--channel` are added to `--post-merge`'s incompatibility list (`release_dry_run.go:163-200`) alongside `--bump` — post-merge takes no version-shaping input; it reads the prepared version and derives its rung. All three help surfaces (`release.go`, `agent_help.go`, `cli_reference.go`) rewritten together, retiring the stale lineage-freeze text.

### Gate placement

The trigger set becomes: candidate is stable (byte-equal cohort match, as today) OR candidate's rung is rc (cohort = candidate's core literal). Alpha/beta candidates keep the valve semantics with warnings. Post-merge keys the same logic on the prepared version's rung: rc and stable gate, alpha/beta flow. Promotion preflight re-checks the cohort against the same committed evidence; because receipts bind to the masked root-tree digest whose exclusion set includes receipt-tree-binding's `releaseMetadataAllowlist` alongside its receipts and reports masks, release-metadata commits are receipt-neutral — the rc release commit and the designation commit cannot expire cohort receipts, so promotion re-check passes by construction. Verification's rhythm follows: a receipts sweep at rc cut (receipts are masked, so N receipts land in one commit without staling each other), a re-sweep after rc bugfixes, nothing at promotion.

### Promotion mechanics

Locate the last rc: highest `vX.Y.Z-rc.N` tag for the candidate core (promotion refuses when none exists — stable is a promoted rc). Designation check: `git diff <last-rc-tag>..HEAD --name-only` must stay within receipt-tree-binding's exported `releaseMetadataAllowlist` (version files, CHANGELOG.md, and the tracked regenerated build outputs that version injection rewrites — the allowlist alone, never the full digest-exclusion set), consumed here so designation-legal and receipt-neutral can never drift apart — else block with the offending paths and the remedy "code changed since rc.N; cut another rc". Both rc cut and promotion additionally require a successful CI run at the exact HEAD SHA before tagging (Decision 11). Content-level inspection of regenerated outputs is rejected ceremony: source-path changes are caught by their own paths, and the existing drift check proves generated outputs match source. Rollup: the CLI computes seed material — the `release-notes.md` fragments of every change whose work landed in the cycle (range plus execution provenance; `shape.md` Problem/Hypothesis lines as a warned fallback for note-less changes) and the cycle's prerelease sections — and emits it for the ceremony; the agent writes the rollup section; mechanical validation confirms `## [X.Y.Z]` exists with items and the cycle's prerelease sections are gone. The collapse set is defined exactly: sections whose label is a recognized rung (alpha, beta, rc) and whose core equals the candidate — pre-model history survives untouched, so 2.0.0's promotion collapses `2.0.0-alpha.*` (and any beta/rc) while the legacy `2.0.0-dev.*` and `2.0.0-pre.*` sections stay. The exact emission surface (stdout block vs file under the change folder) is the implementing task's tactical choice.

### Per-channel guardrails

Post-merge guardrails 5 and 6 key on the prepared version's rung: alpha/beta sections must exist with items as today, now produced by the notes projection rather than a hand-curated buffer; rc requires the release commit to stay within the metadata allowlist minus CHANGELOG (version files + regenerated outputs, no changelog section), with GitHub Release notes falling back to an auto-generated commit list for the range; promotion validates the rollup and collapse per above. The rc-cut preflight warns when the range since the last rc/beta contains `feat:` commits. The write path itself is TASK-008's: `writeReleaseChangelog` and the dry-run preview become rung-aware, so an rc cut writes nothing for the guardrails to reject.

### CI and tap

`release.yml` derives the channel from the tag: prerelease tags pass `--prerelease` on the create-fallback and update `Formula/loaf-dev.rb`; stable tags update `Formula/loaf.rb`. `update-homebrew-formula.mjs` already takes a required `--formula` path; the remaining work is the hardcoded Ruby class name (`class Loaf < Formula`, `:27`) becoming parameterized so the dev formula emits `LoafDev`. The initial `loaf-dev.rb` is seeded manually in the tap repo (H-tier — the tap is a separate repository).

### Skill

The release skill rewrite brings the document to the shipped CLI: channel state in context detection, the ladder and its ceremonies, the `/promote` flow, channel suggestion via AskUserQuestion fed by mechanical signals (`loaf change list --target`, receipt states, time since last release), and the post-merge steps that are currently absent. `loaf-reference` and the CLI reference regenerate from the build.

### Sequencing

Rung model first (everything reads it), then the flag surface, then gate placement (TASK-003) and the notes projection (TASK-008) — promotion mechanics (TASK-004) follows the projection, because its seed emission calls TASK-008's collection helper (declared `blocked-by`) — then per-channel guardrails (TASK-005, verifying what the projection produces), distribution (needs only the rung predicate), skill last (documents what shipped). TASK-004 does not start until the receipt-tree-binding predecessor lands on main — the dependency is carried by prose and cohort order because cross-change task relations are deliberately forbidden in the work model. Gate-at-rc also pulls the sweep carrier's whole lift ahead of stabilization: `spec-conversion-and-guidance-sweep` is captured-only today, so under the new trigger set 2.0.0-rc.1 cannot cut until the carrier is shaped, converted, executed, and receipt-verified — earlier than the stable-only gate demanded. That is the discipline working as designed (an rc claims stable intent; alphas and betas flow regardless), named here so the rc block is never read as a regression.

## Implementation Units

- **TASK-001 — Channel rung model.** Label/suffix parsing, ladder ordering, cycle entry, advance, and promotion math in the version primitives, forward-only enforcement, unit tests.
- **TASK-002 — Promote and channel flags.** `--promote [rung|release]`, `--channel` assertion, `--bump release` retirement, flag matrix validation, snapshot fields, three-way help sync.
- **TASK-003 — Gate fires at rc.** Trigger-set extension (rc candidates map to core cohort), valve retention for alpha/beta, post-merge rung keying, fixtures for every rung.
- **TASK-004 — Designation-commit promotion.** Last-rc location, designation diff check, rollup seed emission, collapse + rollup validation, promotion preflight gate re-check.
- **TASK-005 — Per-channel changelog guardrails.** Guardrails 5/6 rung keying, rc notes auto-derivation, feat-during-rc warning, negative fixtures.
- **TASK-006 — Channel-routed distribution.** `release.yml` channel derivation, `--prerelease` fallback, two-formula routing, formula-script parameterization.
- **TASK-007 — Release skill rewrite.** Channel ceremony, `/promote` flow, agent-suggested human channel choice, release-notes authoring discipline, current CLI vocabulary, rebuilt outputs.
- **TASK-008 — Release notes and the changelog projection.** Per-change `release-notes.md` convention, range-plus-provenance note collection, the rung-aware CHANGELOG write path (`writeReleaseChangelog` + dry-run preview: rc writes nothing, alpha/beta aggregate notes, promotion consumes TASK-004's validation), `[Unreleased]` retirement, `workflow-pre-pr` re-point with the no-user-facing-impact escape.

## Verification Contract

- **V1.** Rung math is correct and forward-only: entry, iterate, advance, skip, promote, and every backward move refused. Command: `go test ./internal/cli -run 'TestReleaseRung' -count=1`. Expect: exit 0.
- **V2.** The promote surface behaves: bare walks one rung, named rung warns on skip, `release` requires an rc base, `--channel` refuses a mismatched outcome before mutation, `--bump release` errors with the pointer. Command: `go test ./internal/cli -run 'TestReleasePromote' -count=1`. Expect: exit 0.
- **V3.** The gate fires for rc candidates and at promotion, and alpha/beta keep the valve. Command: `go test ./internal/cli -run 'TestReleaseGateRC' -count=1`. Expect: exit 0.
- **V4.** Designation-commit promotion: diff restriction, rollup validation, prerelease-section collapse per the defined collapse set, CI-green-at-HEAD refusal. Command: `go test ./internal/cli -run 'TestReleaseDesignation' -count=1`. Expect: exit 0.
- **V5.** Per-channel guardrails and the rc-cut feat warning hold with negative fixtures per rung. Command: `go test ./internal/cli -run 'TestReleasePostMergeChannel' -count=1`. Expect: exit 0.
- **V6.** The notes projection behaves: rc cuts leave CHANGELOG untouched, alpha/beta sections aggregate exactly the ranged-and-landed notes and land above the previous top release with no `[Unreleased]` anchor present, note-less changes fall back with a warning, the pre-PR hook accepts note presence and the exact no-impact declaration. Command: `go test ./internal/cli -run 'TestReleaseNotesProjection' -count=1`. Expect: exit 0.
- **V7.** The full suite is green. Command: `go test ./...`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** A promotion dry-run transcript on a fixture repo reads as the intended ceremony: gate re-check, designation check, seed material, validation verdicts.
- **H2.** The rewritten release skill names only flags and commands the shipped CLI accepts.
- **H3.** Observed at first post-landing releases: a prerelease updates `loaf-dev.rb` and not `loaf.rb`; the 2.0.0 stable updates `loaf.rb`; the create-fallback marks prereleases.
- **H4.** The 2.0.0 rollup, seeded from its cohort, reads as one coherent story since the last stable — judged against the reaction artifact in `research/`. Acknowledged: this first rollup is largely fallback-sourced (the note convention is newer than most of its cohort), so H4 judges the fallback path exactly where it will dominate.

## Definition of Done

- All V-entries green at HEAD via `loaf change verify` with the receipt committed.
- `loaf change check` reports zero violations and the change derives executable.
- `npm run build` regenerates the skill and reference surfaces; rebuilt `plugins/` and `dist/` outputs committed with their source.
- The three release help surfaces agree, and no retired vocabulary (lineage freeze, `--bump release`) survives outside the deprecation error itself.

## Durable Outputs

- ADR: the release promotion model — designation-commit promotion, gate-at-rc, per-channel changelog policy, and the rejected alternatives (tag-only, artifact promotion, CI-authored commits).
- `docs/knowledge/work-model.md`: release section updated from valve-only semantics to the channel model.
- Release skill references: channel ceremony and promotion flow (lands with TASK-007).

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. Tags are convention, never parsed by check. -->

- [UK] What a good cohort-seeded stable rollup reads like → reaction artifact in `research/`: a mock 2.0.0 rollup written from the live cohort when TASK-004's seed format lands, judged before TASK-007 encodes the ceremony.
- [KU] Exact seed-material emission surface (stdout block vs file) → TASK-004's tactical choice; recorded in its Verification notes when made.
