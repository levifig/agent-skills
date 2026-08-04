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
- Make a skill conflict cost one skill rather than every skill, so an unowned directory in the shared store no longer aborts the whole sync, and fix the generated OpenCode command links that have pointed at a directory which stopped existing when skills moved to the canonical store.
- Harden ownership before any mass retirement is activated, so migration cannot delete what Loaf does not provably own.
- Rider: stop reporting retirements that are already absent or paths Loaf does not own, and remove manifest entries whose window has expired.

**Out** (deferred, not rejected)

- `loaf skills` for third-party installation — the next Change in the arc, unblocked by neutrality because foreign content can then be installed without being rewritten.
- The skills audit: taxonomy, whether Loaf's `orchestration` survives standalone or is absorbed, and description quality. That remains the sweep carrier gating 2.0.0, and all naming now belongs to it — including whether `orchestration` becomes `orchestrate` and whether it survives standalone at all. This Change makes collisions survivable so that naming can be decided on its merits instead of under collision pressure.
- Skill-to-agent binding. Two gaps surfaced while scoping the prefix and are recorded rather than fixed: the `implementer` profile declares no `skills:` list despite promising "speciality determined by skills loaded at spawn time", where `librarian` and `background-runner` both declare one; and Loaf has no way to mark a skill as always loaded for the root session, since `skills:` lists bind only to subagents and no skill declares `context: fork`. Both are binding architecture, not naming, and neither blocks this Change.
- Runtime deprecation state — self-pruning manifests or report-once acknowledgement. Manifest hygiene here is authored, not stateful.

**Cut** (explicitly rejected)

- A Loaf-private canonical store. `vercel-labs/skills` hard-codes `~/.agents/skills` as canonical and gives no symlink to agents that read it directly, explicitly to prevent redundant copies and double-listing; ADR-018 already reached the same destination from primary documentation.
- Per-target rendered bodies. If a body must differ per harness, that is a content problem to solve in the body, not a topology to support.
- Writing Loaf skills to `~/.config/agents/skills`. Amp reads it at higher precedence than the canonical store, so anything placed there shadows canonical updates and recreates the stale-flavor problem this Change exists to remove.
- Any further blind string replacement over authored prose, for any reason.

## Observable Workflow

`loaf build` produces skill bodies that are byte-identical across every target; only frontmatter, which sidecars legitimately own, differs. `loaf install --to all` performs one canonical write per skill to `~/.agents/skills/<name>`, plans it once rather than once per target, and reports a shared conflict once rather than four times. A directory Loaf cannot prove it owns costs that one skill and nothing else: the rest install, and the skipped are named. Codex, Cursor, OpenCode, and Amp all read that directory natively, so nothing is copied anywhere else. Claude Code is untouched and continues to ship through its plugin channel. `loaf upgrade` no longer reports retirements that are already absent, no longer warns about `~/.gemini`, and no longer refuses every target because one directory in a shared store belongs to another vendor.

## Rabbit Holes and No-Gos

Rewriting prose for neutrality is not an invitation to edit skills for quality, structure, or taxonomy — that is the skills audit, and pulling it in would make the diff unreviewable. Labeled harness sections are for facts that are genuinely product-specific; they are not a licence to reintroduce per-target content under a new name, and a section that merely restates behaviour the model can infer should be prose instead. Ownership must never be resolved by force: a directory Loaf cannot prove it owns is left alone and un-managed, never overwritten and never deleted, and a digest mismatch means "another vendor's now" at least as often as it means "the user edited it". The reference implementation validates the destination but is not a migration design — Loaf is a vendor publisher with upgrade and ownership obligations that `vercel-labs/skills` does not carry.

## Decisions

Provenance: ADR-018 and the deprecation manifest that implements it; primary documentation for Codex, Cursor, OpenCode, and Amp; source reading of `vercel-labs/skills` (`src/installer.ts`, `src/agents.ts`); a sandbox-HOME install experiment; operator interview; and a constructive shape review that refuted one premise and reshaped three units.

1. **The canonical store is `~/.agents/skills`.** ADR-018 already decided this from primary documentation, and `vercel-labs/skills` independently hard-codes the same path, classifying agents that read it as "universal" and giving them no symlink because doing so "prevents redundant symlinks and double-listing of skills". All four non-Claude targets are universal by that definition. This forecloses per-target flavors: a canonical store holds exactly one copy.
2. **Skill bodies are harness-neutral by authoring, not by build-time substitution.** Describe the behaviour and let the model select its own tool. This forecloses naming a harness-specific tool as though it were the only one, and requires the content rewrite to land with the build change rather than after it. Every former build token is retired: `{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`, `{{SUBAGENT_MECHANISM}}`, and `{{TODO_TOOL}}` become behavioural prose; `{{IMPLEMENT_CMD}}` / `{{RESUME_CMD}}` / `{{ORCHESTRATE_CMD}}` become an authored `/implement` (or a prose reference to the implement workflow), since all three already resolved to the same constant for every target; and `{{AGENTS_FILE}}` is retired rather than kept as the last legitimate token (see Decision 9).
3. **Genuinely harness-specific facts live in explicitly labeled sections inside one body.** There is no target identity at read time — every harness opens the same file — so a per-target rendering of the same position is incoherent under a canonical store. A labeled section is legible to every reader and correct for the one it names. This is what makes the byte-identical invariant hold without an exception clause, and it reframes intentional cross-harness documentation as correct content rather than leaked vocabulary.
4. **No `loaf-` prefix.** Shaping treated name discipline as the only available defense in a shared store, but the store's actual contents refute the premise. Four third-party skills — `helmor-cli`, `i-have-adhd`, `improve`, and `thermo-nuclear-code-quality-review` — already sit alongside Loaf's thirty-five without incident, and the one name that ever collided, `orchestration`, is not on disk at all. Renaming thirty-five directories to route around a single collision buys a namespace the ecosystem does not enforce, and the next vendor to publish `loaf-anything` reopens it. Naming is the skills audit's subject: a name too generic to squat deserves a better name, not a mechanical prefix on thirty-four innocent siblings.
5. **The defect is blast radius, not collision.** `syncManagedSkillsDirIfExists` completes its ownership preflight before staging anything and returns on the first unowned or digest-mismatched directory, so one foreign directory does not cost one skill — it aborts the entire sync and installs zero skills across all four targets. That is what the `orchestration` collision actually did, and reporting it four times was a symptom rather than the disease. Conflicts therefore become per-skill: install everything Loaf provably owns, skip what it does not, and name the skipped. This survives the collision class a prefix would only have postponed, and it is worth having whether or not any skill is ever renamed.
6. **Amp needs no bridge.** Amp's manual lists `~/.config/agents/skills` first and `~/.agents/skills` second in precedence, so it reads the canonical store directly; ADR-018's relocation was deliberate and correct and is preserved, not reversed. Writing to the higher-precedence path would shadow every later canonical update.
7. **Claude Code stays on its plugin channel.** It gets plugin-scoped naming for free, and adding it to the canonical store would double-list every skill. With no rename in play, its skill names stay identical to the canonical store's, so a body that names a sibling skill resolves the same way on both channels.
8. **Ownership hardening stands on its own.** Retired-skill cleanup treats the presence of `SKILL.md` as proof of Loaf ownership and calls `os.RemoveAll` under `--yes` without consulting the digest manifest. Shaping justified this as a precondition for mass retirement; dropping the prefix removes the mass retirement but not the hazard, because four directories Loaf does not own are in the shared store today and the existing relocation entries already walk prior skill homes. It is now an independently landable unit rather than half of an atomic pair.
9. **`{{AGENTS_FILE}}` is retired; project instructions are named `AGENTS.md` in prose.** ADR-020 makes root `AGENTS.md` the canonical real file, with `.claude/CLAUDE.md` only a compatibility symlink to it. Inside Loaf-managed projects the path distinction collapses to one file, so a per-harness token is wrong: every harness should be told to read and write `AGENTS.md`. Where the Claude compatibility path is itself the fact (creating or checking the symlink), that is a labeled harness-specific path under Decision 3, not a reason to keep a token. Outside Loaf-managed projects the same authoring rule applies — name the standard file, and label any product-specific compatibility path when it matters.

## Planning Contract

### Approach

The work forms three landing groups rather than seven independent commits. The first fixes what a canonical artifact *is*: remove substitution, settle the labeled-section convention, rewrite the content, and prove bodies are invariant. The second changes install topology: one canonical write, per-skill conflict isolation, and ownership hardening. Shaping bound these together because a canonical write needed final names; dropping the prefix dissolves that coupling, and each now lands on its own merits. The third is an independent reporting rider.

Implementation corrected one assumption here. Shaping expected the invariance test to stay red until the content rewrite landed; it goes green as soon as substitution is removed, because substitution is what *created* the divergence — one authored body copied without rewriting is identical by construction. What survives is shared Claude-first *wording*, which is a neutrality problem for the rewrite and for H1, not an identity problem for V1. The group still lands together, but for a different reason: removing the rewriters leaves content saying things that are now inaccurate on some harnesses, and the rewrite is what makes them true again.

### Placement

Substitution lives in `internal/cli/build_codex.go` (`substituteNativeBuildHarnessLanguage`, `nativeBuildHarnessLanguages`) despite serving every target; whatever survives should move to a home named for the cross-target concern. Skill destinations resolve through `installSkillsDestination` in `internal/cli/install_target.go`, but a single canonical write also requires the orchestration layers in `internal/cli/install.go` and `internal/cli/install_plan.go`, which currently iterate targets and independently plan and sync the shared store — without those, dry-run keeps reporting one shared collision four times even after content becomes identical. Deprecation reporting is `internal/cli/install_deprecations.go`, with data in `config/deprecations.json`.

### Risks

The destructive-migration path is the sharpest risk. `install_deprecations.go` treats any directory containing `SKILL.md` as Loaf-owned and removes it wholesale under `--yes`, and the same helper backs the general ownership check. Until that consults the digest manifest, any retirement entry is a potential deletion of someone else's work. This is why naming and migration land atomically and why the regression test must exercise the real destructive path rather than an ordinary install conflict.

Double-listing is the second risk. Cursor reads both `~/.agents/skills` and `~/.cursor/skills`; OpenCode reads `~/.agents/skills`, `~/.config/opencode/skills`, and `~/.claude/skills`; Amp reads `~/.config/agents/skills` ahead of the canonical store. Stale Loaf copies in any of those paths would surface alongside canonical ones under a different name, and on Amp would take precedence over them. Migration must retire them, not merely stop managing them.

### Sequencing

Group one lands together: removing the rewriters leaves content that is inaccurate on some harnesses until the rewrite corrects it, so neither is a standalone commit. Group two no longer needs to land as one unit: with the prefix gone there is no mass retirement to protect against, so ownership hardening is landable on its own and conflict isolation depends only on the canonical write preceding it. The rider is independent and can land at any point. Within group one the content rewrite is the long pole and can proceed in parallel with group two's install work, provided group two does not land first.

### Derive surface sets, never enumerate them

Group one learned this the expensive way and group two will face it again. Any unit whose scope is "everything of a kind" must derive that set from the code that produces it, and the derivation belongs in the delegation brief so the implementer produces the list rather than trusting an authored one.

The content rewrite's boundary was wrong four consecutive times, always by omission: `content/skills/` missed `content/templates/`, which reaches every target through `copyNativeSharedTemplates`; fixing that missed `content/hooks/instructions/` and `content/agents/`, where a single file ships into three target trees; deriving the directory list from `internal/cli/build_*.go` finally produced a complete inventory — and a Markdown-only grep over that correct list still missed `content/skills/*/scripts/`. Two distinct failures live here: a wrong list, and a right list searched with too narrow a filter. Shipped scripts and configuration carry the same obligations as prose, because a printed string in a copied script is user-facing content.

The permission fences showed the same shape from the other side. Four review rounds each removed the single bad allowlist entry they found, and a fifth kept appearing; what converged was authoring the rule that generates them — `Bash(cmd *)` matches any arguments and therefore cannot exclude a flag — with the least obvious case worked out in the document.

TASK-006 is where this bites next. "Retire Loaf's stale copies across every path a harness scans" is exactly the shape that fails: derive the path set from the harness search-path table and the install code, ask for the disposition of every entry including the ones deliberately left, and treat silence about an entry as indistinguishable from missing it.

## Implementation Units

- **TASK-001 — Harness-neutral build contract.** Remove blind prose substitution, decide the fate of each token, relocate what survives out of the Codex-specific file, and add the invariance tests the rewrite must satisfy.
- **TASK-002 — Labeled harness-section convention.** The authoring convention that lets one body carry genuinely product-specific facts, replacing the substitution that currently corrupts them.
- **TASK-003 — Harness-neutral content rewrite.** Rewrite the affected prose so bodies are identical for every target, converting real per-harness facts into labeled sections, without editing skills for quality or structure.
- **TASK-004 — Single canonical write.** One planned and executed write to `~/.agents/skills` across the orchestration layers as well as the per-target installers, with order-invariance and single-conflict-report tests.
- **TASK-005 — Per-skill conflict isolation and command-link repair.** An unowned directory costs one skill instead of the whole store, the skipped are named, and generated OpenCode command links resolve from where the command is actually installed.
- **TASK-006 — Ownership hardening and migration.** Make retirement digest-aware before it can run against a store holding directories Loaf does not own, retire Loaf's stale copies across every path a harness scans, and leave foreign and vendor-replaced directories untouched.
- **TASK-007 — Deprecation report noise.** Stop reporting already-absent retirements and unowned paths, and remove expired entries from the authored manifest. Reporting and authored data only; no runtime state.

## Verification Contract

<!-- Executable (machine-checkable): each V-entry declares Command and Expect for loaf change verify. -->

- **V1.** The built skill tree is byte-identical across every target, frontmatter included, except for fields a target sidecar legitimately owns. Command: `go test ./internal/cli/ -run TestSkillTreeIsTargetInvariant`. Expect: exit 0.
- **V2.** No blind prose substitution remains anywhere in the build path. Command: `go test ./internal/cli/ -run TestNoHarnessProseSubstitution`. Expect: exit 0.
- **V3.** Every labeled harness section is present and carries its exact strings, with no duplicated or prose-substituted tool names. Command: `go test ./internal/cli/ -run TestLabeledHarnessSectionsRenderVerbatim`. Expect: exit 0.
- **V4.** An unowned or digest-mismatched directory in the shared store costs exactly that one skill: every other skill still installs, and the skipped are reported by name. Command: `go test ./internal/cli/ -run TestSkillConflictIsolation`. Expect: exit 0.
- **V5.** Install plans and executes exactly one canonical write per skill and reports a shared conflict once, regardless of target order. Command: `go test ./internal/cli/ -run TestSingleCanonicalWrite`. Expect: exit 0.
- **V6.** Destructive migration under `--yes` preserves a foreign `orchestration`, a manifest entry whose digest no longer matches, and a dangling symlink, while retiring Loaf's provable copies from every prior skill home. Command: `go test ./internal/cli/ -run TestDestructiveMigrationPreservesUnowned`. Expect: exit 0.
- **V7.** Every rewritten reference or template link in a generated OpenCode command resolves to a real file from the location the command is installed to. Command: `go test ./internal/cli/ -run TestGeneratedCommandLinksResolve`. Expect: exit 0.
- **V8.** The deprecation report omits already-absent retirements and unowned paths, still reports a retirement with something genuinely present, and carries no expired manifest entries. Command: `go test ./internal/cli/ -run TestDeprecationReport`. Expect: exit 0.
- **V9.** The full build and test suite are green. Command: `npm run build && npm run test`. Expect: exit 0.

<!-- Human review (H-tier): review material, never gate input. -->

- **H1.** A reviewer confirms the rewritten prose reads naturally on every harness and carries no *accidental* target assumption, while intentional labeled cross-harness material is preserved rather than stripped — and that neutrality did not become an editorial pass. This is now the *only* guard on that property: TASK-001 removed the harness-language parity lint, which enforced the incompatible "no Claude-isms on non-Claude targets" contract, and nothing machine-checkable replaced it. If review proves too weak a net, a lint that distinguishes accidental assumption from labeled intent is the follow-up.
- **H2.** An installed discovery smoke shows a Loaf skill actually listed and loadable in Codex, Cursor, OpenCode, and Amp. Path existence is not discovery. The same smoke answers two questions the canonical write raised and documentation could not settle: whether those harnesses deduplicate a skill discovered through more than one search path, and whether they tolerate OpenCode's `subtask` and `user-invocable` frontmatter keys in the one copy they all read. OpenCode documents ignoring unknown fields; Codex documents reading `name` and `description`; Cursor and Amp are unestablished.
- **H3.** A before-and-after tree-hash receipt for the migration is recorded as machine evidence, and a reviewer reads it. A reviewer's unaided confirmation is not strong enough for a destructive contract.

## Definition of Done

- V1 through V9 pass, and the H-tier observations are recorded with evidence rather than assertion.
- Installed-smoke receipts in `config/target-capabilities.json` are re-recorded against the final native binary. This Change touches Go source, so every rebuild changes the binary SHA-256 and the evidence gate correctly fails the suite until the receipts are refreshed — the alpha.16 canary working as designed. Re-record once, after the last Go change lands, not per task.
- The migration receipt exists and shows nothing unowned was modified.
- Discovery smokes exist for all four canonical-store harnesses, not only one.
- The dogfooding machine completes `loaf upgrade` with no conflicts, no absent-retirement lines, no `~/.gemini` warning, and no duplicate skill listings in any harness.
- `loaf change check` reports zero violations.

## Durable Outputs

An amendment to ADR-018 rather than a competing ADR: its destination decision stands and is reinforced, but it needs the universal-agent rationale, the Amp precedence ordering that makes `~/.config/agents/skills` unsafe to write, and the single-canonical-write consequence. A knowledge-base entry capturing the per-harness skill search paths in precedence order, because that table was expensive to assemble, was misread once already, and is the input to every future target decision. An authoring rule in the project guidelines stating that skill prose describes behaviour and never names a harness-specific tool as though it were the only one, with the labeled-section convention spelled out as the sanctioned way to carry product-specific facts.

## Open Questions

<!-- Fog register: tag entries [KU]/[UK]; a route is named for each. -->

- [KU] Do Cursor and OpenCode deduplicate a skill discovered through more than one of their search paths, or list it twice? This sets how aggressive migration must be. → Installed discovery smoke in TASK-006.
- [KU] What is the right labeled-section shape — a subsection per harness, a table, or a single "if your harness is X" paragraph? The choice affects readability for the four-fifths of readers a section does not apply to. → **Resolved in TASK-002:** primary shape is topic parent + `### <Product Name>` subsections for multi-line fences and multi-paragraph mechanisms; compact harness table for one-line facts (slash forms, single tokens); reject inline "if your harness is X" for anything longer than a parenthetical. Documented in `AGENTS.md` (Harness-Neutral Authoring).
- [KU] Does `{{AGENTS_FILE}}` survive as the one legitimate token, or become a labeled section? ADR-020 makes root `AGENTS.md` canonical with `.claude/CLAUDE.md` symlinked, which may collapse the distinction inside Loaf-managed projects but not outside them. → **Resolved in TASK-001 (Decision 9):** retired. Author `AGENTS.md` in prose; carry `.claude/CLAUDE.md` only as a labeled harness-specific path when the symlink itself is the fact.
- [KU] Should the unprefixed-name retirement use the standard one-release window, given that it retires 35 entries at once rather than the usual one or two? → **Moot (Decision 4):** no rename, so no retirement entries are created and no window is needed.
- [KU] Do Claude Code plugin skill names need the prefix for cross-skill loads to resolve identically on both channels, or can bodies stop naming sibling skills entirely? → **Moot (Decision 4):** names stay identical on both channels, so a body naming a sibling resolves the same way in the plugin and in the canonical store.
- [KU] Do Cursor and Amp tolerate the OpenCode-owned frontmatter keys (`subtask`, `user-invocable`) that reach them through the single canonical copy? Product documentation settles OpenCode (unknown fields ignored) and describes Codex as reading `name` and `description`, but says nothing dispositive for Cursor or Amp. Stripping the keys instead would cost real OpenCode behaviour — twelve sidecars are annotated "execute in main context". → Installed discovery smoke, H2.
