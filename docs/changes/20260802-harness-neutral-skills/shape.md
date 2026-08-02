<!-- shape.md is the change contract. Identity lives in change.json — no status-like frontmatter. Readiness is derived: a draft PR is shaping; `loaf change check` derives structural executability from the sections below. -->

# Harness-Neutral Skills and the Canonical Store

## Problem

Loaf renders every skill once per target by running blind string replacement over already-finished prose. The pass is an eight-pair token replacer followed by a sixteen-pair replacer bracketed by two whole-document `ReplaceAll` calls, mapping `Claude Code`→`OpenCode`, `CLAUDE.md`→`AGENTS.md`, `subagent`→`subtask agent`, `TodoWrite`→`update_plan`, and the rest. It is an artifact of content authored Claude-Code-first and find-replaced outward, and it leaves 20 skill bodies differing between any two targets — the higher whole-file counts of 20 to 27 include legitimate sidecar frontmatter, which targets own and which is not a defect.

The replacement is wrong in a way that ships. `content/skills/foundations/references/permissions.md` contains a fenced allowed-tools list reading `TodoWrite, TodoRead`; both names match independently, so the built docs instruct Codex to configure `update_plan, update_plan` and OpenCode to configure `native task/todo surface when available, native task/todo surface when available`. Substitution ran inside a code fence, where the literal string is the artifact, while the surrounding prose it should have handled was left to chance.

That divergence then collides with the install path. All four non-Claude targets resolve skills to `~/.agents/skills` (`internal/cli/install_target.go:294`–`308`) and each calls `syncManagedSkillsDirIfExists` with its own distribution. A target's tree is admitted when its installed digest still matches the previous manifest and is then republished as an update, so four target-scoped writers share one destination and the last one wins. Installing opencode, cursor, codex, and amp in sequence into a sandbox HOME confirms it: after each install the shared directory holds exactly that target's flavor and no other's. Three of four harnesses therefore read another harness's skills.

One conflict currently freezes all of it. Orca installed its own `orchestration` skill into `~/.agents/skills/orchestration`, a name Loaf's manifest claims. The content-hash ownership guard correctly refuses to overwrite it, and because every target shares that destination the refusal blocks skill sync for all of them — reported as four conflicts when it is one directory.

The shared destination broke one more thing on its way in. OpenCode command files are generated with their relative links rewritten to point back into the skill directory by name (`internal/cli/build_opencode.go:118`–`119`), so a command at `~/.config/opencode/commands/` reaches for `../skills/<name>/references/…` — which resolves to `~/.config/opencode/skills/`, a directory that has not existed since skills moved to `~/.agents/skills`. There are 60 such dangling links across 19 installed command files. The command *body* is inlined and self-contained; its references and templates are not.

## Hypothesis

Skill content does not need to be rendered per harness. Models know their own tool inventory, so a skill that says "ask one question at a time, with a recommendation, using your harness's structured question tool if it has one" works everywhere, while `{{INTERVIEW_TOOL}}` merely guesses at what the model already has in context. Where a fact genuinely is harness-specific — an exact allowlist entry, a configuration mechanism that differs by product — one body can carry every harness's variant in an explicitly labeled section and let the reader take the one that applies. The most-used skills collection in the ecosystem, `mattpocock/skills`, ships one `SKILL.md` per skill with no per-harness rendering at all. If Loaf's bodies become target-invariant the same way, the divergence that motivates per-target copies disappears, one canonical copy legitimately serves every harness, the last-writer-wins bug is removed rather than routed around, and third-party skills become installable later without any rendering step.

## Scope

**In**

- Delete blind prose substitution from the build, and rewrite the affected content to be harness-neutral by authoring.
- Where a fact is genuinely harness-specific, carry every variant in explicitly labeled sections inside one body, so no rendering path survives.
- Collapse skill installation to exactly one canonical write to `~/.agents/skills`, across the install orchestration layers as well as the per-target installers.
- Give Loaf's skills `loaf-` prefixed identity under a complete contract: directory name, frontmatter `name`, agent `skills:` lists, cross-skill loads named in bodies, permission patterns, manifest keys, and Claude plugin names.
- Harden ownership before any mass retirement is activated, so migration cannot delete what Loaf does not provably own.
- Rider: stop reporting retirements that are already absent or paths Loaf does not own, and remove manifest entries whose window has expired.

**Out** (deferred, not rejected)

- `loaf skills` for third-party installation — the next Change in the arc, unblocked by neutrality because foreign content can then be installed without being rewritten.
- The skills audit: taxonomy, whether Loaf's `orchestration` survives standalone or is absorbed, and description quality. That remains the sweep carrier gating 2.0.0.
- Runtime deprecation state — self-pruning manifests or report-once acknowledgement. Manifest hygiene here is authored, not stateful.

**Cut** (explicitly rejected)

- A Loaf-private canonical store. `vercel-labs/skills` hard-codes `~/.agents/skills` as canonical and gives no symlink to agents that read it directly, explicitly to prevent redundant copies and double-listing; ADR-018 already reached the same destination from primary documentation.
- Per-target rendered bodies. If a body must differ per harness, that is a content problem to solve in the body, not a topology to support.
- Writing Loaf skills to `~/.config/agents/skills`. Amp reads it at higher precedence than the canonical store, so anything placed there shadows canonical updates and recreates the stale-flavor problem this Change exists to remove.
- Any further blind string replacement over authored prose, for any reason.

## Observable Workflow

`loaf build` produces skill bodies that are byte-identical across every target; only frontmatter, which sidecars legitimately own, differs. `loaf install --to all` performs one canonical write per skill to `~/.agents/skills/loaf-<name>`, plans it once rather than once per target, and reports a shared conflict once rather than four times. Codex, Cursor, OpenCode, and Amp all read that directory natively, so nothing is copied anywhere else. Claude Code is untouched and continues to ship through its plugin channel. `loaf upgrade` no longer reports retirements that are already absent, no longer warns about `~/.gemini`, and no longer refuses every target because one directory in a shared store belongs to another vendor.

## Rabbit Holes and No-Gos

Rewriting prose for neutrality is not an invitation to edit skills for quality, structure, or taxonomy — that is the skills audit, and pulling it in would make the diff unreviewable. Labeled harness sections are for facts that are genuinely product-specific; they are not a licence to reintroduce per-target content under a new name, and a section that merely restates behaviour the model can infer should be prose instead. Ownership must never be resolved by force: a directory Loaf cannot prove it owns is left alone and un-managed, never overwritten and never deleted, and a digest mismatch means "another vendor's now" at least as often as it means "the user edited it". The reference implementation validates the destination but is not a migration design — Loaf is a vendor publisher with upgrade and ownership obligations that `vercel-labs/skills` does not carry.

## Decisions

Provenance: ADR-018 and the deprecation manifest that implements it; primary documentation for Codex, Cursor, OpenCode, and Amp; source reading of `vercel-labs/skills` (`src/installer.ts`, `src/agents.ts`); a sandbox-HOME install experiment; operator interview; and a constructive shape review that refuted one premise and reshaped three units.

1. **The canonical store is `~/.agents/skills`.** ADR-018 already decided this from primary documentation, and `vercel-labs/skills` independently hard-codes the same path, classifying agents that read it as "universal" and giving them no symlink because doing so "prevents redundant symlinks and double-listing of skills". All four non-Claude targets are universal by that definition. This forecloses per-target flavors: a canonical store holds exactly one copy.
2. **Skill bodies are harness-neutral by authoring, not by build-time substitution.** Describe the behaviour and let the model select its own tool. This forecloses naming a harness-specific tool as though it were the only one, and requires the content rewrite to land with the build change rather than after it.
3. **Genuinely harness-specific facts live in explicitly labeled sections inside one body.** There is no target identity at read time — every harness opens the same file — so a per-target rendering of the same position is incoherent under a canonical store. A labeled section is legible to every reader and correct for the one it names. This is what makes the byte-identical invariant hold without an exception clause, and it reframes intentional cross-harness documentation as correct content rather than leaked vocabulary.
4. **The `loaf-` prefix is an identity contract, not a directory rename.** OpenCode requires frontmatter `name` to match the containing directory, so a renamed directory with an unrenamed `name` is invalid. Bare names also resolve outside installation, in agent `skills:` lists, in bodies that instruct an agent to load another skill, and in the link rewriting that points OpenCode command files back at their skill directory. Every one of those surfaces moves together or the rename is broken. Sharing a canonical store with other vendors makes name discipline the only available defense, since the ecosystem offers no namespacing or scoping mechanism.
5. **Every skill takes the prefix, invocable or not.** Seventeen of the thirty-five are `user-invocable: false` and could obviously be renamed at no cost, but the other eighteen are equally free: they reach users through a separately generated command file or through plugin scoping, and neither reads the skill directory name. Partial prefixing would not close the collision class anyway, since `research`, `implement`, `ship`, `release`, and `debugging` are all invocable and all generic enough for another vendor to claim.
6. **Amp needs no bridge.** Amp's manual lists `~/.config/agents/skills` first and `~/.agents/skills` second in precedence, so it reads the canonical store directly; ADR-018's relocation was deliberate and correct and is preserved, not reversed. Writing to the higher-precedence path would shadow every later canonical update.
7. **Claude Code stays on its plugin channel.** It gets plugin-scoped naming for free, and adding it to the canonical store would double-list every skill. Its plugin skill names are still part of the identity contract in Decision 4, because bodies that name other skills are shared across both channels.
8. **Ownership hardening precedes any mass retirement.** Retired-skill cleanup currently treats the presence of `SKILL.md` as proof of Loaf ownership and calls `os.RemoveAll` under `--yes` without consulting the digest manifest. Activating 35 retirement entries against that code could delete another vendor's directory, so the naming and migration work lands as one atomic unit.

## Planning Contract

### Approach

The work forms three landing groups rather than seven independent commits. The first fixes what a canonical artifact *is*: remove substitution, settle the labeled-section convention, rewrite the content, and prove bodies are invariant. The second changes install topology, naming, and migration together, because each is unsafe without the others — a canonical write needs final names, and retirement needs ownership hardening. The third is an independent reporting rider.

### Placement

Substitution lives in `internal/cli/build_codex.go` (`substituteNativeBuildHarnessLanguage`, `nativeBuildHarnessLanguages`) despite serving every target; whatever survives should move to a home named for the cross-target concern. Skill destinations resolve through `installSkillsDestination` in `internal/cli/install_target.go`, but a single canonical write also requires the orchestration layers in `internal/cli/install.go` and `internal/cli/install_plan.go`, which currently iterate targets and independently plan and sync the shared store — without those, dry-run keeps reporting one shared collision four times even after content becomes identical. Deprecation reporting is `internal/cli/install_deprecations.go`, with data in `config/deprecations.json`.

### Risks

The destructive-migration path is the sharpest risk. `install_deprecations.go` treats any directory containing `SKILL.md` as Loaf-owned and removes it wholesale under `--yes`, and the same helper backs the general ownership check. Until that consults the digest manifest, any retirement entry is a potential deletion of someone else's work. This is why naming and migration land atomically and why the regression test must exercise the real destructive path rather than an ordinary install conflict.

Double-listing is the second risk. Cursor reads both `~/.agents/skills` and `~/.cursor/skills`; OpenCode reads `~/.agents/skills`, `~/.config/opencode/skills`, and `~/.claude/skills`; Amp reads `~/.config/agents/skills` ahead of the canonical store. Stale Loaf copies in any of those paths would surface alongside canonical ones under a different name, and on Amp would take precedence over them. Migration must retire them, not merely stop managing them.

### Sequencing

Group one lands together: the identity test added with the build change fails until the content rewrite completes, so neither is a standalone commit. Group two lands together for the safety reason above. The rider is independent and can land at any point. Within group one the content rewrite is the long pole and can proceed in parallel with group two's install work, provided group two does not land first.

## Implementation Units

- **TASK-001 — Harness-neutral build contract.** Remove blind prose substitution, decide the fate of each token, relocate what survives out of the Codex-specific file, and add the invariance tests the rewrite must satisfy.
- **TASK-002 — Labeled harness-section convention.** The authoring convention that lets one body carry genuinely product-specific facts, replacing the substitution that currently corrupts them.
- **TASK-003 — Harness-neutral content rewrite.** Rewrite the affected prose so bodies are identical for every target, converting real per-harness facts into labeled sections, without editing skills for quality or structure.
- **TASK-004 — Single canonical write.** One planned and executed write to `~/.agents/skills` across the orchestration layers as well as the per-target installers, with order-invariance and single-conflict-report tests.
- **TASK-005 — Prefix identity contract.** Directory, frontmatter `name`, agent `skills:` lists, cross-skill loads, permission patterns, manifest keys, and plugin names moved together.
- **TASK-006 — Ownership hardening and migration.** Make retirement digest-aware before activating it, retire Loaf's stale copies across every path a harness scans, leave foreign and vendor-replaced directories untouched, and land atomically with TASK-005.
- **TASK-007 — Deprecation report noise.** Stop reporting already-absent retirements and unowned paths, and remove expired entries from the authored manifest. Reporting and authored data only; no runtime state.

## Verification Contract

<!-- Executable (machine-checkable): each V-entry declares Command and Expect for loaf change verify. -->

- **V1.** The built skill tree is byte-identical across every target, frontmatter included, except for fields a target sidecar legitimately owns. Command: `go test ./internal/cli/ -run TestSkillTreeIsTargetInvariant`. Expect: exit 0.
- **V2.** No blind prose substitution remains anywhere in the build path. Command: `go test ./internal/cli/ -run TestNoHarnessProseSubstitution`. Expect: exit 0.
- **V3.** Every labeled harness section is present and carries its exact strings, with no duplicated or prose-substituted tool names. Command: `go test ./internal/cli/ -run TestLabeledHarnessSectionsRenderVerbatim`. Expect: exit 0.
- **V4.** Frontmatter `name` matches the containing directory for every built skill, and every agent `skills:` reference and cross-skill load resolves to a real skill. Command: `go test ./internal/cli/ -run TestSkillIdentityContract`. Expect: exit 0.
- **V5.** Install plans and executes exactly one canonical write per skill and reports a shared conflict once, regardless of target order. Command: `go test ./internal/cli/ -run TestSingleCanonicalWrite`. Expect: exit 0.
- **V6.** Destructive migration under `--yes` preserves a foreign `orchestration`, a manifest entry whose digest no longer matches, and a dangling symlink, while retiring Loaf's provable copies from every prior skill home. Command: `go test ./internal/cli/ -run TestDestructiveMigrationPreservesUnowned`. Expect: exit 0.
- **V7.** Bare typed commands still exist, invoke the intended prefixed skill behaviour, and every rewritten reference or template link in a generated command resolves to a real file from its installed location. Command: `go test ./internal/cli/ -run TestBareCommandsResolveToPrefixedSkills`. Expect: exit 0.
- **V8.** The deprecation report omits already-absent retirements and unowned paths, still reports a retirement with something genuinely present, and carries no expired manifest entries. Command: `go test ./internal/cli/ -run TestDeprecationReport`. Expect: exit 0.
- **V9.** The full build and test suite are green. Command: `npm run build && npm run test`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** A reviewer confirms the rewritten prose reads naturally on every harness and carries no *accidental* target assumption, while intentional labeled cross-harness material is preserved rather than stripped — and that neutrality did not become an editorial pass.
- **H2.** An installed discovery smoke shows a prefixed Loaf skill actually listed and loadable in Codex, Cursor, OpenCode, and Amp. Path existence is not discovery.
- **H3.** A before-and-after tree-hash receipt for the migration is recorded as machine evidence, and a reviewer reads it. A reviewer's unaided confirmation is not strong enough for a destructive contract.

## Definition of Done

- V1 through V9 pass, and the H-tier observations are recorded with evidence rather than assertion.
- The migration receipt exists and shows nothing unowned was modified.
- Discovery smokes exist for all four canonical-store harnesses, not only one.
- The dogfooding machine completes `loaf upgrade` with no conflicts, no absent-retirement lines, no `~/.gemini` warning, and no duplicate skill listings in any harness.
- `loaf change check` reports zero violations.

## Durable Outputs

An amendment to ADR-018 rather than a competing ADR: its destination decision stands and is reinforced, but it needs the universal-agent rationale, the Amp precedence ordering that makes `~/.config/agents/skills` unsafe to write, and the single-canonical-write consequence. A knowledge-base entry capturing the per-harness skill search paths in precedence order, because that table was expensive to assemble, was misread once already, and is the input to every future target decision. An authoring rule in the project guidelines stating that skill prose describes behaviour and never names a harness-specific tool as though it were the only one, with the labeled-section convention spelled out as the sanctioned way to carry product-specific facts.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]; a route is named for each. -->

- [KU] Do Cursor and OpenCode deduplicate a skill discovered through more than one of their search paths, or list it twice? This sets how aggressive migration must be. → Installed discovery smoke in TASK-006.
- [KU] What is the right labeled-section shape — a subsection per harness, a table, or a single "if your harness is X" paragraph? The choice affects readability for the four-fifths of readers a section does not apply to. → Decide in TASK-002 against the real counterexamples in `permissions.md`, `background-agents.md`, and `orchestration/references/`.
- [KU] Does `{{AGENTS_FILE}}` survive as the one legitimate token, or become a labeled section? ADR-020 makes root `AGENTS.md` canonical with `.claude/CLAUDE.md` symlinked, which may collapse the distinction inside Loaf-managed projects but not outside them. → Decide in TASK-001.
- [KU] Should the unprefixed-name retirement use the standard one-release window, given that it retires 35 entries at once rather than the usual one or two? → Decide in TASK-005, after ownership hardening makes the retirement safe at all.
- [KU] Do Claude Code plugin skill names need the prefix for cross-skill loads to resolve identically on both channels, or can bodies stop naming sibling skills entirely? → Decide in TASK-005; the second option is simpler and may be better regardless.
