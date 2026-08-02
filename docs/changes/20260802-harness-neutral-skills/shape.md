<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Harness-Neutral Skills and the Canonical Store

## Problem

Loaf renders every skill five times, once per target, by running seventeen blind string replacements over finished prose — `Claude Code`→`OpenCode`, `CLAUDE.md`→`AGENTS.md`, `subagent`→`subtask agent`, `TodoWrite`→`update_plan`, and so on. This is an artifact of content authored Claude-Code-first and find-replaced outward, and it produces 20–27 divergent files per target pair. It is also wrong in a way that ships: `content/skills/foundations/references/permissions.md` contains a fenced allowed-tools list reading `TodoWrite, TodoRead`, and because both names match independently the built docs instruct Codex to configure `update_plan, update_plan` and OpenCode to configure `native task/todo surface when available, native task/todo surface when available`. Substitution ran inside a code fence where the literal string is the artifact, while leaving the surrounding prose it should have handled to chance.

That divergence then collides with the install path. All four non-Claude targets record `skills_dir=~/.agents/skills` (`internal/cli/install_target.go:308`), and `syncManagedSkillsDirIfExists` is last-writer-wins, so installing four targets whose builds differ means three of them read another harness's skills. Installing opencode, cursor, codex, and amp in sequence into a sandbox HOME confirms it: after each install the shared directory holds exactly that target's flavor and no other's.

Separately, and worse, Amp does not read `~/.agents/skills` at all — it reads `~/.config/agents/skills`. Loaf's own June relocation (`amp-skills-to-agents-home` in `config/deprecations.json`) moved Amp's skills out of the only directory Amp scans, and that directory is empty on the dogfooding machine. The Amp target has most likely delivered zero skills since June.

Finally, one conflict currently freezes everything. Orca installed its own `orchestration` skill into `~/.agents/skills/orchestration`, a name Loaf's manifest claims. The content-hash ownership guard correctly refuses to overwrite it, and because all four targets share that destination the refusal blocks skill sync for every target at once — reported as four conflicts when it is one directory.

## Hypothesis

Skill content does not actually need to differ per harness. Models know their own tool inventory; a skill that says "ask one question at a time, with a recommendation, using your harness's structured question tool if it has one" works everywhere, while `{{INTERVIEW_TOOL}}` merely guesses at what the model already knows. The most-used skills collection in the ecosystem — `mattpocock/skills`, roughly 143k stars — ships one `SKILL.md` per skill with no per-harness rendering whatsoever, and its only harness-awareness is an unanswered issue about which agents-file a setup script should edit. If Loaf's skill bodies become target-invariant, the divergence that motivates per-target copies disappears, one canonical copy legitimately serves every harness that reads the shared store, the clobber bug is removed rather than routed around, and third-party skills become installable later without any rendering step at all.

## Scope

**In**

- Delete the blind prose substitution in `substituteNativeBuildHarnessLanguage` and rewrite the affected skill content to be harness-neutral by authoring.
- Keep an explicit, narrow per-target mechanism for literal config blocks, where the exact string is the artifact rather than a description of one.
- Install exactly one canonical copy per skill to `~/.agents/skills`, matching the model `vercel-labs/skills` implements.
- Bridge Amp with a symlink from `~/.config/agents/skills` into the canonical store, restoring skill delivery to that target.
- Prefix Loaf's canonical skills `loaf-*`, with `retired_skills` manifest entries for the unprefixed names.
- Migrate existing installs: leave vendor-replaced and foreign directories untouched, retire Loaf's own stale unprefixed copies.
- Rider: silence deprecation-report lines for retirements that are already absent, and remove the expired `gemini` retired-target entry.

**Out** (deferred, not rejected)

- `loaf skills` for third-party skill installation — the next Change in the arc, unblocked by neutrality because foreign content can then be installed without being rewritten.
- The skills audit: taxonomy, whether Loaf's `orchestration` survives as a standalone skill or is absorbed, and description quality. That remains the sweep carrier gating 2.0.0.
- Publishing Loaf skills to harnesses Loaf has no build target for.

**Cut** (explicitly rejected)

- A Loaf-private canonical store. `vercel-labs/skills` hard-codes `~/.agents/skills` as canonical and symlinks only harnesses that cannot read it, explicitly to prevent redundant copies and double-listing; a parallel private store would fragment a working convention for no gain.
- Per-target skill flavors. If content must diverge per harness, the divergence is a content bug to fix, not a topology to support.
- Any further blind string replacement over authored prose, for any reason.

## Observable Workflow

`loaf build` produces skill bodies that are byte-identical across every target; only frontmatter, which sidecars legitimately own, differs. `loaf install --to all` writes one copy of each skill to `~/.agents/skills/loaf-<name>` and creates `~/.config/agents/skills/loaf-<name>` as a symlink into it, and installing targets in any order leaves the same bytes on disk. Codex, Cursor, and OpenCode read the canonical directory natively, so nothing is copied for them. Claude Code is untouched and continues to ship through its plugin channel. `loaf upgrade` no longer reports retirements that are already absent, no longer warns about `~/.gemini`, and no longer refuses an entire target because one directory in a shared store belongs to another vendor.

## Rabbit Holes and No-Gos

Rewriting skill prose for neutrality is not an invitation to edit skills for quality, structure, or taxonomy — that is the skills audit, and pulling it in here would make the diff unreviewable. Building a general templating or conditional-content engine is likewise out; the literal-config exception should be the smallest mechanism that works, and if it starts attracting feature requests that is a signal to stop. The `vercel-labs/skills` registry supports 75+ agents; Loaf supports five, and this Change does not change that number. Ownership must never be resolved by force: a directory Loaf cannot prove it owns is left alone and un-managed, never overwritten and never deleted, and a hash mismatch means "someone else's now" at least as often as it means "the user edited it".

## Decisions

Provenance: source reading of `vercel-labs/skills` (`src/installer.ts`, `src/agents.ts`), primary harness documentation for Codex, Cursor, OpenCode, and Amp, a sandbox-HOME install experiment, and operator interview across two sessions.

1. **The canonical store is `~/.agents/skills`, not a Loaf-private directory.** `getCanonicalSkillsDir()` hard-codes it, and agents whose `skillsDir` equals `.agents/skills` are treated as "universal" and deliberately given no symlink, because doing so "prevents redundant symlinks and double-listing of skills". Codex, Cursor, and OpenCode are all universal by that definition. Adopting the same store means Loaf participates in the convention instead of shadowing it; it forecloses per-target flavors, since a canonical store holds exactly one copy.
2. **Skill bodies are harness-neutral by authoring, not by build-time substitution.** Describe the behavior and let the model select its own tool. This forecloses naming specific tools in prose and requires the content rewrite to land with the build change rather than after it.
3. **Literal config blocks are the sole exception and are authored explicitly per target.** Where a fenced block is configuration the user or harness consumes verbatim, the exact string matters and neutral prose cannot substitute for it. This inverts today's behavior, which rewrites literal config and leaves prose to chance.
4. **Loaf's canonical skills are prefixed `loaf-*`.** Sharing a canonical store with other vendors makes name discipline the only available defense — the ecosystem offers no namespacing, scoping, or collision mechanism at all. Prefixing dissolves the Orca `orchestration` conflict rather than contesting it, and is no longer deferrable to the skills audit now that the shared store is the chosen destination. Typed commands are unaffected: OpenCode's `/shape` comes from a separately generated `command/shape.md`, and Claude Code's from the plugin.
5. **Amp is bridged by symlink into the canonical store.** Amp reads `~/.config/agents/skills` and not `~/.agents/skills`, which makes it non-universal in exactly the sense the reference implementation defines, and a symlink is the mechanism that implementation uses for that case.
6. **Claude Code is unchanged.** It ships through `plugins/loaf/` and gets plugin-scoped naming for free; adding it to the canonical store would double-list every skill.

## Planning Contract

### Approach

Neutrality lands before topology within the Change, because a single canonical copy is only correct once the bodies are genuinely identical — shipping the store first would install one arbitrary flavor for everyone. The build change and the content rewrite are separate units but must land together to keep the tree green: the build unit adds a test asserting cross-target body identity, and that test fails until the content unit completes.

### Placement

Substitution lives in `internal/cli/build_codex.go` (`substituteNativeBuildHarnessLanguage`, `nativeBuildHarnessLanguages`) despite serving every target; the neutrality unit should move whatever survives to a clearly-named home rather than leave a cross-target concern in a per-target file. Install destinations resolve through `installSkillsDestination` in `internal/cli/install_target.go`. Deprecation reporting is `internal/cli/install_deprecations.go`, with data in `config/deprecations.json`.

### Risks

The load-bearing unknown is whether OpenCode, Cursor, and Amp follow symlinks when scanning skill directories. Codex documents that it does — "Codex supports symlinked skill folders and follows the symlink target when scanning these locations" — but only Amp actually needs a symlink under this design, so the exposure is narrower than it first appears: if Amp does not traverse symlinks, that one target falls back to a copy, which is the same fallback the reference implementation offers for platforms without symlink support. Windows needs that fallback regardless, since symlink creation there requires privilege; the reference implementation uses junctions.

A second risk is double-listing. Cursor reads both `~/.agents/skills` and `~/.cursor/skills`, and OpenCode reads `~/.agents/skills`, `~/.config/opencode/skills`, and `~/.claude/skills`. Stale Loaf copies left behind in any of those paths would surface alongside the canonical ones under a different name, so migration must retire them rather than merely stop managing them.

### Sequencing

The content rewrite is the long pole and is independent of the install work once the build contract is fixed, so it can proceed in parallel with topology. Prefixing must land after the canonical destination is settled but before migration, because migration retires the unprefixed names the prefix change orphans. The deprecation-noise rider is independent of everything else and can land at any point.

## Implementation Units

- **TASK-001 — Harness-neutral build contract.** Remove blind prose substitution, decide the fate of each of the five tokens, relocate what survives out of the Codex-specific file, and add the cross-target body-identity test that the content rewrite must satisfy.
- **TASK-002 — Literal config block mechanism.** The narrow per-target path for fenced content consumed verbatim, replacing the substitution that currently corrupts it.
- **TASK-003 — Harness-neutral content rewrite.** Rewrite the affected prose across the skill corpus so bodies are identical for every target, without editing skills for quality or structure.
- **TASK-004 — Canonical single-flavor install.** One copy to `~/.agents/skills`, no per-target skill fan-out, Amp bridged by symlink with a copy fallback, and a regression test that installing every target in sequence leaves one set of bytes.
- **TASK-005 — Prefix and retirement of unprefixed names.** Rename in the canonical store and add `retired_skills` entries for the old names.
- **TASK-006 — Migration and symlink-aware ownership.** Retire Loaf's own stale copies across every path a harness scans, leave vendor-replaced and foreign directories untouched and un-managed, and resolve or explicitly defer the open dangling-symlink recovery intent.
- **TASK-007 — Deprecation report noise.** Silence already-absent retirements, stop warning about unowned retired paths on every run, age out expired entries, and remove the `gemini` entry.

## Verification Contract

<!-- Executable (machine-checkable): each V-entry declares Command and Expect for loaf change verify. -->

- **V1.** Skill bodies are byte-identical across every built target; only frontmatter differs. Command: `go test ./internal/cli/ -run TestSkillBodiesAreTargetInvariant`. Expect: exit 0.
- **V2.** No blind prose substitution remains in the build path. Command: `go test ./internal/cli/ -run TestNoHarnessProseSubstitution`. Expect: exit 0.
- **V3.** Literal config blocks render their exact per-target strings, with no duplicated or prose-substituted tool names. Command: `go test ./internal/cli/ -run TestLiteralConfigBlocksRenderVerbatim`. Expect: exit 0.
- **V4.** Every target resolves skills to the canonical store, and Amp additionally receives a link or copy at its own path. Command: `go test ./internal/cli/ -run TestCanonicalSkillsDestination`. Expect: exit 0.
- **V5.** Installing all targets in sequence leaves one flavor on disk regardless of order. Command: `go test ./internal/cli/ -run TestSequentialInstallLeavesOneFlavor`. Expect: exit 0.
- **V6.** Migration leaves foreign and vendor-replaced directories untouched while retiring Loaf's own stale copies. Command: `go test ./internal/cli/ -run TestMigrationPreservesUnownedSkills`. Expect: exit 0.
- **V7.** The deprecation report omits already-absent retirements and does not warn about unowned paths. Command: `go test ./internal/cli/ -run TestDeprecationReportOmitsAbsent`. Expect: exit 0.
- **V8.** The full build and test suite are green. Command: `npm run build && npm run test`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** A reviewer confirms the rewritten prose reads naturally on a non-Claude harness and leaks no Claude-specific vocabulary, and that neutrality did not quietly become an editorial pass.
- **H2.** An installed smoke on Amp surfaces Loaf skills, proving the invisibility bug is actually fixed rather than inferred from paths.
- **H3.** A reviewer confirms no directory Loaf could not prove it owned was overwritten or deleted during migration.

## Definition of Done

- V1 through V8 pass, and the H-tier observations are recorded with evidence rather than assertion.
- An installed smoke exists for Amp skill visibility, and for symlink traversal on any target that depends on it.
- The dogfooding machine completes `loaf upgrade` with no conflicts, no absent-retirement lines, and no `~/.gemini` warning.
- `loaf change check` reports zero violations.

## Durable Outputs

An ADR recording that the canonical store is `~/.agents/skills` and why a Loaf-private store was rejected, since that decision will look arbitrary later without the reference-implementation evidence behind it. A knowledge-base entry capturing the per-harness skill search paths and which harnesses are universal, because that table was expensive to assemble and is the input to every future target decision. An authoring rule in the project guidelines stating that skill prose describes behavior and never names a harness-specific tool, with the literal-config exception spelled out.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]/[UU] with a route. -->

- [KU] Do OpenCode, Cursor, and Amp follow symlinks when scanning skill directories? Codex documents that it does. → Installed smoke in TASK-004; only Amp's answer is load-bearing, and a copy fallback covers a negative.
- [KU] Is Amp's skill invisibility real, or is there an undocumented path it also scans? → Installed smoke in TASK-004 before this is stated as fact anywhere user-visible.
- [KU] Does `{{AGENTS_FILE}}` survive as the one legitimate token? ADR-020 makes root `AGENTS.md` canonical with `.claude/CLAUDE.md` symlinked to it, which may already collapse the distinction inside Loaf-managed projects but not outside them. → Decide in TASK-001.
- [UU] Do harnesses that scan several paths deduplicate a skill discovered through more than one of them, or list it twice? Cursor and OpenCode each scan three or more locations. → Blindspot pass over harness discovery behavior during TASK-006, since the answer sets how aggressive migration must be.
- [KU] Should the unprefixed-name retirement use the standard one-release window, given that it retires 35 entries at once rather than the usual one or two? → Decide in TASK-005.
