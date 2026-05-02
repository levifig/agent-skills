# Strategy

Loaf is an opinionated agentic framework for AI coding assistants. It ships skills, profiles, hooks, and a CLI to six harnesses. This document records what the project has proven through implementation and what the evidence says to build next.

## Who This Serves

**Solo developer.** Uses Loaf with Claude Code (or another AI tool) for personal and professional projects. Wants structured workflow that reduces context-switching overhead, enforces quality without manual effort, and preserves work across sessions. Technical, comfortable with CLI, values autonomy. Measures Loaf by how much friction it removes compared to vanilla AI-assisted development. Will abandon the framework if it gets in the way more than it helps.

**Team lead.** Installs Loaf across multiple developers and tools. Wants consistent agent behavior regardless of which developer or AI tool is used. Values quality gates, audit trails, and predictable outcomes. Needs the framework to work without requiring every team member to understand its internals. Measures Loaf by whether it reduces variance in agent output quality across the team. Will not adopt if the onboarding cost exceeds the consistency benefit.

## What Has Been Proven

24 specs shipped, 6 in progress. The evidence clusters into four themes.

**Skills are the highest-leverage investment.** They work across all six targets. Better skill descriptions and organization improve every target simultaneously. Profiles and hooks are Claude Code infrastructure that other targets cannot use.

This was proven by SPEC-014 (skill activation redesign) and SPEC-020 (target convergence) -- the same 31 skills now deploy to Claude Code, Cursor, OpenCode, Codex, Gemini, and Amp from a single source tree. The implication for both personas: invest in skill quality first, harness-specific features second.

ADR-010 (prompt-overlay consolidation, shipped v2.0.0-dev.28) extended target convergence to the project overlay file itself: five of six targets now write to `.agents/AGENTS.md` directly; the sixth target's native path is a symlink to it. A single managed fenced section replaces six. For the team lead persona, this means onboarding additional harnesses adds no duplicated content-maintenance overhead; for the solo developer, the file they edit and the file every harness reads are the same file.

**Sessions must survive everything.** Context compaction, `/clear`, tool restarts, and cross-conversation handoffs all create new Claude session IDs pointing at the same logical session. Any architecture that assumes 1 session = 1 conversation fails in practice.

SPEC-027 (session stability), SPEC-023 (session continuity on `/clear`), and SPEC-030 (Librarian agent) addressed this incrementally. Session splits are now detected and consolidated on start. The `## Current State` section provides handoff context that survives compaction. The model is stabilizing but remains the most failure-prone surface -- it touches every other feature.

SPEC-029 (journal enrichment) extended session completeness by adding post-hoc JSONL review. The first real test revealed a new session routing tension: `loaf session log` routes by branch, but `loaf session enrich` routes by `claude_session_id`. When multiple conversations contribute to one session, these routing mechanisms can disagree. This is the next session reliability challenge.

**Hook primitives have hard behavioral constraints.** These are platform limits discovered through SPEC-026 and SPEC-030, not design choices. They constrain every future hook design:

- **Prompt hooks** are binary gates -- any non-empty LLM response is treated as rejection. Cannot express "this looks fine, proceed." Unusable for advisory guidance; use only for validation that returns empty on success.
- **Agent hooks** have read-only tool access (Read, Grep, Glob, WebFetch). Useful for observation, not mutation.
- **Command hooks** are the correct primitive for side effects and context injection. Exit 0 with stdout for injection, exit 1 for warning, exit 2 to block.
- **Stop hooks** can create circular re-triggers when they write to files the hook chain monitors. State writes must be idempotent or guarded.
- **PreCompact prompt hooks** do not work outside REPL sessions. Use command hooks for PreCompact context injection.
- **`plugin.json`** silently drops non-matcher lifecycle events. `hooks.json` is the canonical registration path for session events.
- **Plugin caching** serves stale hook handlers during local development. Marketplace remove/re-add is the only reliable cache-bust. This is the single largest development-cycle friction point.

**The CLI is the correct protocol layer.** Skills should describe what to do. The CLI should execute it deterministically. Hooks should enforce invariants. This three-layer separation emerged through SPEC-008 (CLI), SPEC-009 (knowledge management), SPEC-012 (cleanup), and SPEC-019 (release).

Every time a skill tried to call an external tool directly -- Linear MCP, raw git commands, file operations -- reliability dropped. The CLI absorbs that complexity and presents a stable interface to skills. For the team lead persona, this is critical: the CLI is the enforcement layer that makes agent behavior deterministic regardless of which LLM or harness is running.

**Diagnosis and repair must share the same state taxonomy.** Sharing repair helpers is not enough; the detection branches in a diagnostic tool must consult the same classification logic as the repair path, or they will drift apart.

`loaf doctor` v1 (shipped v2.0.0-dev.28, PR #33) silently skipped legacy-layout states (real `./AGENTS.md` or `.claude/CLAUDE.md` without a canonical `.agents/AGENTS.md`) even though `migrateRealFileToSymlink` -- the very helper that repairs those states -- was already written and invocable from the fix path. The diagnostic and repair code paths had diverged: the repair path knew about legacy layouts; the detection path did not. External review caught this in PR #35 (v2.0.0-dev.30); the fix rebuilt detection to surface legacy states as fixable `fail`, and the fix paths to lazily create canonical before delegating to the shared helper. For both personas, this means `loaf doctor --fix` can be trusted to heal every state the diagnostic surfaces as fixable -- because the same state definitions drive both halves of the state machine.

**Personality is decorative, mechanics are durable.** SPEC-033 explored decoupling agent personality (Warden/Fellowship lore) from agent mechanics via a swappable souls catalog. Implementation was complete, reviewed twice (Loaf reviewer + two Codex passes), and pivoted in-flight before merge — the user, after months of personal Loaf use, acknowledged the lore had never landed; Codex flagged SOUL.md as a brittle prompt-injection surface (advisory enforcement only). The shipped artifact (`v2.0.0-dev.32`) keeps the profile neutralization and skill prose audit; the catalog/CLI/install/SessionStart-restoration layers were removed.

The implication for both personas: Loaf's value is the *framework* (mechanical hooks, structured pipeline, profile boundaries, knowledge layer), not a personality layer. Adding identity through prompt content is incompatible with hardening agents — it costs complexity, adds attack surface, and delivers value only when the user already buys into the metaphor. The lore concept is preserved in SPEC-033 archive as a record of what was tried, with a possible future home in a Pi-based harness where it could be load-bearing rather than decorative.

## Current Priorities

Ordered by evidence strength -- what has been proven most urgent by shipping.

1. **Session reliability** (proven: SPEC-027, 028, 030, 032). The foundation everything else builds on. Session splits, compaction, and `/clear` are handled. Housekeeping and archival are automated via `loaf session housekeeping`. The Librarian agent manages session lifecycle within `.agents/`.
   - Remaining gap: session state is still occasionally lost during rapid compaction cycles.
   - The PreCompact flush depends on the model actually writing the state summary before compaction completes -- a race condition Loaf cannot fully control.
   - **v2.0.0-dev.31 (SPEC-032) closed the routing inconsistency.** `loaf session log`, `archive`, `enrich`, and `end --wrap` now resolve through a 3-tier chain (`--session-id` flag → hook stdin → branch-fallback), and the branch-fallback tier emits a stderr WARN naming the missing signal. The dev.30 misroute pattern is gated by an E2E test with sha256 content-hash assertions on stopped sessions. The architectural rule is now enforced in code: **session files are keyed by `claude_session_id`; branch is a property of the session, not its identity.**
   - **Remaining work in this priority:** skill self-logging (`loaf session log "skill(name): ..."` from skill Bash invocations) still falls through to branch-routing because skills don't yet pass `--session-id` and Claude Code doesn't reliably export per-process session state. The branch-fallback tier with its visible WARN is the migration's regression-testable surface. A future spec will refactor skills to source `session_id` per-process and remove the branch tier entirely.

2. **Hook correctness** (proven: SPEC-026, 030). Hooks must use the right primitive for the job. The behavioral constraint documentation is now in ARCHITECTURE.md and tested.
   - Remaining gap: new hooks are still occasionally authored with the wrong type because the failure mode is silent.
   - A prompt hook that should be advisory becomes an accidental gate, and nothing in the build warns about it.
   - v2.0.0-dev.28 surfaced a parallel mismatch: `workflow-pre-pr`'s escape hatch for a consumed `[Unreleased]` section only covered tagged HEAD, but `/loaf:release` uses `--no-tag` (tags land on main post-merge). The hook blocked a legitimate release-commit PR. Worked around with a `[Unreleased]` placeholder line, but the root problem is that hook contracts drift from skill assumptions without any cross-layer validation.

3. **Release flow hardening** (new, exposed by v2.0.0-dev.28 release; expanded by v2.0.0-dev.31). The release skill's step order does not match hook contracts in practice. `validate-push` blocks any push without a version bump + CHANGELOG update; `workflow-pre-pr` blocks any PR whose `[Unreleased]` is empty. `/loaf:release` as currently written pushes before bumping (blocked) OR bumps before pushing and consumes `[Unreleased]` (blocked at PR creation). Path forward: rewrite the release skill's step order to bump → push → PR, and extend `workflow-pre-pr` and `validate-push` to accept a `chore: release v<semver>` HEAD commit as an escape hatch the way tagged HEAD is accepted today (shipped in SPEC-031, replacing the rejected `release:` shape that never passed `@commitlint/config-conventional`).
   - v2.0.0-dev.31 release surfaced three additional friction points concretely. First, `loaf release` CLI silently overrode curated `[Unreleased]` entries with auto-generated commit-subject jargon — the release skill's docs describe a "preserve curated entries" path the CLI doesn't implement. Required a follow-up commit (`382e474d`) to rewrite the changelog from the user's perspective. Second, `validate-push` rejects the `release: vX.Y.Z` commit shape on any project pinned to `@commitlint/config-conventional` — Loaf-using projects that adopt commitlint cannot use the release flow without rewording the commit by hand. Third, bundled artifacts (lockfiles, `plugins/loaf/bin/loaf`, build outputs) recurringly leak into commits whose primary scope is unrelated; in this release Codex review caught this twice across two different PRs (lockfile in PR #36, `plugins/loaf/bin/loaf` in `13ce968d`), each requiring a follow-up to split.
   - v2.0.0-dev.32 (PR #40) hit two of the same friction points again: `loaf release` auto-generated commit-subject CHANGELOG jargon (required `3abf59f5` follow-up to rewrite from user perspective), and emptied `[Unreleased]` after the bump (required `5a567dc7` and `91159be5` ceremony commits to satisfy `workflow-pre-pr`). Recurrence reinforces SPEC-031 as a real release-experience tax — every release pays it.
   - **v2.0.0-dev.33 (SPEC-031, PR #41) shipped** the canonical 5-step release flow: `loaf release --pre-merge` → `gh pr create` → review → `gh pr merge --squash` → `loaf release --post-merge`. The chore-shape commit cutover, hook escape hatches, monorepo discovery, release-only PR classifier, curated-entries preservation, and stub re-insertion all landed atomically in one PR that dogfooded the very flow it implemented. Tag/push/GH-release/branch-cleanup are now a single `--post-merge` invocation.
   - **Meta-learning: pre-push Codex review is a release-flow gate.** The Codex review of the implementation (run between local green and `git push`) caught two P0s — a divergent base resolver in `post-merge.ts` and a missing real-commit subject test — plus one "non-blocking" stub-form finding that turned out to be a real production bug in `workflow-pre-pr`'s empty-section detector (would have allowed feature PRs without changelog entries to silently pass). Outsider review on a complete-but-unpushed branch is a higher-leverage moment than post-PR comments because rework is free; SPEC-031 itself benefited from a pre-breakdown Codex review of the spec text, confirming the pattern across both ends of the workflow.
   - **New polish layer surfaced during the ship.** Three follow-ups that aren't release-flow defects but adjacent build/lockfile hygiene: `TASK-149` (`loaf release`'s build step runs content-only `loaf build`, not full `npm run build` — released a stale CLI bundle, caught by the new CI verifier), `TASK-148` (`extractUnreleasedEntries` flattens `### Added`/`### Changed` subsection headers — "verbatim" was implemented as list-items-only), `TASK-136` (pre-commit guard against bundled-artifact leakage — recurring pattern from dev.31 + dev.32 documented but not yet fixed). These are the next iteration's release-experience tax.
   - **CI flipped from fixer to verifier (ADR-012).** `Build Distributions` workflow used to auto-commit and push build outputs back to `main`; branch protection rejected it (`GH013`) every run. New design: verify-only with diff-on-failure debugging, also runs on `pull_request`. Caught two real determinism bugs (lockfile/install drift on `yaml@2.8.2` vs `2.8.3`, and the TASK-149 content-only build step) that auto-push would have papered over. Build determinism contract is now explicit: bundled deps are part of the contract, lockfile is authoritative, `npm ci` before any commit-able rebuild.

4. **Agent routing enforcement** (next: SPEC-022). Profiles exist and build to all targets, but nothing makes the harness use them. A developer spawning a generic agent gets no tool boundaries, no naming convention, no behavioral contract.
   - The spec proposes hook-assisted routing: a PreToolUse hook on Agent that enriches profile-based spawns and warns on generic ones. Nudge-based, never blocking.
   - For the team lead persona, this is the difference between "we have agent profiles" and "agents actually behave consistently."

5. **Backend abstraction** (next: SPEC-023). Skills reference Linear MCP tools directly (~80 references across 12+ files). The CLI should be the protocol layer with pluggable backends -- same `loaf task` commands, different storage (local files, Linear, eventually GitHub Issues).
   - This also completes the Python/Bash to TypeScript migration (38 scripts remaining), eliminating Python and Bash as runtime dependencies.
   - For the solo developer, this means Loaf works without Linear. For the team lead, the tool choice is a config toggle, not a skill rewrite.
   - **Significant progress from v2.0.0-dev.29 (Linear-native mode, ADR-011):** skills no longer hard-code Linear MCP tool names in every branch — they branch explicitly on `integrations.linear.enabled` in `.agents/loaf.json`. Same skills, two modes (local-tasks and Linear-native). The full abstraction is still ahead, but the skills' mode-awareness is now the contract to extend. SPEC-023's scope narrows: mostly "extract the Linear calls into a `tracker` CLI subcommand" rather than "rewrite N skills to remove MCP coupling."

6. **Harness-native surface leverage** (next: SPEC-024). Each harness has unique runtime capabilities (Cursor native agents, Gemini subagents and hooks, OpenCode runtime plugins). Loaf currently deploys skills as the lowest common denominator.
   - Gemini is still modeled as "skills only" despite now supporting a richer native surface.
   - SPEC-024 proposes a surface-first target model: shared source, per-target native delivery. The payoff is that each target feels native rather than aliased through another tool's layout.
   - Per-harness gap analysis surfaced during SPEC-033 implementation:
     - **Gemini** is the largest gap — modeled as "skills only" but the harness now supports subagents and hooks. SPEC-024 should prioritize Gemini hook + agent registration.
     - **Codex** has agent registration that Loaf doesn't use (we rely on `codex:rescue` agents in our own workflows already). Adding first-class Codex agent support is high-leverage.
     - **Cursor** native `subagent_type` routing composes with SPEC-022.
     - **Amp's** plugin API is upstream-limited: it dispatches exactly four events (`tool.call`/`tool.result`/`agent.start`/`agent.end`) — no session-lifecycle dispatch despite the binary internally emitting `session.start`. Loaf's Amp target is correctly scoped to tool events; session-lifecycle features (SOUL.md self-healing, etc.) are not viable on Amp without upstream changes. SPEC-024 should reflect this constraint rather than try to work around it.

## Strategic Tensions

These are not problems to solve -- they are tradeoffs to manage. Each has surfaced repeatedly during implementation.

**Portability vs. native leverage.** Writing for the lowest common denominator (skills only) leaves harness-specific features unused. But harness-native code (hooks, agents, runtime plugins) is not portable. SPEC-024 proposes a surface-first model: shared authoring with per-target native delivery. The cost is clear -- every native surface adds to the test matrix and maintenance burden. The benefit is that each harness feels native rather than lowest-common-denominator. The tension will not resolve; it requires ongoing judgment about where to invest per-target effort and where skills alone are sufficient.

**Automation vs. explainability.** Hooks that "just work" are invisible, but invisible behavior is hard to debug when it breaks. Plugin caching is the canonical example: the framework cached a stale hook handler, the hook silently misbehaved, and the failure was indistinguishable from a logic error. The `validate-push` hook going from blocking to advisory (SPEC-015 to dev.12) is another -- it blocked valid pushes silently until the behavior was observed and corrected. Every automation decision must weigh the cost of silent failure against the cost of manual intervention. The solo developer can tolerate more automation (they can debug it). The team lead cannot (their developers will file bugs).

**Convention vs. flexibility.** The framework is opinionated about workflow (spec, tasks, code, learn), but projects vary enormously. Too rigid and users fight the framework; too flexible and the opinions do not hold. The current balance -- strict pipeline, flexible domain skills -- has held through 24 shipped specs. But all of that usage has been on Loaf itself, a project that was designed around the pipeline. The first real test is when someone installs Loaf on a project with an existing workflow and existing conventions that conflict with Loaf's opinions.

**Skill depth vs. skill breadth.** 32 skills across 8 languages, 6 workflow phases, and 5 engineering domains. Each skill competes for context window space. Claude's 250-character description truncation means routing quality depends on the first sentence of every skill description. Adding more skills improves coverage but degrades routing accuracy. The SPEC-014 description rewrite improved routing, but the fundamental constraint -- finite context, growing skill count -- remains.

**Test-fixture isolation vs. development speed.** `cli/commands/report.test.ts > "scaffolds a report"` was silently broken for 17+ commits because `cli/commands/check.test.ts` used a cwd-relative fixture (`join(process.cwd(), ".test-check-command")`) that raced against report's subprocesses under vitest's default file parallelism. Per-file runs passed; full-suite runs failed non-deterministically. The current response (v2.0.0-dev.28) migrates `check.test.ts` to `mkdtempSync` and sets `fileParallelism: false` as a defensive default. The tension: parallel test execution is fast, but subprocess-spawning tests must use OS-tmp isolation to prevent cross-file pollution, and nothing in the test authoring path forces this. Options to consider: a lint rule that flags `join(process.cwd(), ...)` in test files; a shared test helper that creates isolated tmpdirs; or a per-file-only default in vitest with opt-in parallelism for pure tests.

**Refactoring artifact taxonomy and ambiguity-funnel discipline (SPEC-034).** Specs are feature-shaped and assume convergent intent; refactoring and bug-fix work want lighter artifacts that reflect their probing nature. SPEC-034 ships PLAN as a minimal ad-hoc shape and surfaces three downstream questions captured as ideas:

- `20260501-225251-spec-plan-tasks-artifact-taxonomy` — the broader SPEC-as-PRD / PLAN-as-strategy / TASKS-as-agent-native taxonomy. Deferred until the first PLAN ships in the wild and the shape can be evaluated against real use.
- `20260501-225335-shape-spawned-ideas-harness` — `/shape` has no discipline for capturing adjacent concepts that surface during interviews. The mechanism (`/idea`, `related:`, `/triage`, `/reflect`) exists; the workflow tying them together does not.
- `20260501-231922-plan-lifecycle-cli-doctor-housekeeping` — plans need lifecycle infrastructure (list, archive, doctor recognition, housekeeping awareness) parallel to specs. Originally Track C of SPEC-034; extracted because lifecycle is a distinct product surface from refactoring-skill scope.
- `20260501-231923-shape-glossary-evolution-deferred` — `/shape` evolution to participate in glossary mutation. Removed from SPEC-034 because `/shape` is an upstream ambiguity funnel; writing to a stability-focused glossary from an ambiguity-resolving step risks polluting canonical vocabulary. Revisit after `/architecture` and `/refactor-deepen` validate the convention.

The tension is real: every workflow skill that wants to surface or stabilize vocabulary must decide *how much commitment to encode*, and the answer depends on where in the convergent-divergent funnel the skill lives. The current resolution (downstream skills mutate; upstream skills observe) is provisional and will be revisited.

## What We Do Not Know Yet

- Whether the pipeline works for teams. All usage so far is solo development on Loaf itself. The team lead persona is designed from first principles, not validated by observation.
- Whether agent routing enforcement (SPEC-022) changes behavior meaningfully or just adds ceremony. The profiles are well-defined, but Claude may ignore routing nudges the way it ignores other soft instructions.
- Whether harness-native leverage (SPEC-024) is worth the maintenance cost. Six targets is already a wide surface. Adding native hooks, agents, and settings per target multiplies the test matrix.
- Whether backend abstraction (SPEC-023) is the right scope -- should it be narrower (just remove Linear references) or wider (full plugin system with arbitrary backends)?
- Whether plugin caching is a solvable development friction or an inherent platform constraint that Loaf must design around permanently.
- Whether enrichment quality (librarian-written journal entries) matches hand-written entries in practice. First test showed scope filtering and entry conciseness issues. Prompt iteration and multi-JSONL discovery are the likely levers.

These are questions that can only be answered by shipping the next round of specs and observing what breaks.
