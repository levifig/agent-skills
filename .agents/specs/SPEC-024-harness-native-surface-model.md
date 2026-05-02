---
id: SPEC-024
title: Harness-Native Surface Model
source: >-
  direct — audit of target convergence, duplicate skill discovery, and Gemini
  native subagent/hook capabilities
created: '2026-04-04T12:13:54Z'
status: implementing
---

# SPEC-024: Harness-Native Surface Model

## Problem Statement

Loaf currently mixes two different ideas:

1. **Shared authoring** — one source tree for skills, agents, hooks, and templates
2. **Shared runtime contract** — multiple harnesses treated as if they consume the same installed artifact

SPEC-020 was correct to converge duplicated build logic, but the current implementation overextends that convergence into runtime behavior. Cursor, Codex, and Gemini currently install skills into the same shared root, while their surrounding runtime surfaces already differ materially:

- Cursor has native agents and hooks
- Codex has a reduced Bash-only hook model
- OpenCode has commands and runtime plugins
- Gemini now has native subagents, hooks, extensions, and settings surfaces

This produces three problems:

1. **Harness leverage is left on the table.** Gemini is still modeled as “skills only” even though its native surface is now richer.
2. **Runtime boundaries are blurry.** A shared install root makes it harder to reason about which harness owns which artifacts and why duplicate discovery happens.
3. **Docs and implementation drift.** The README advertises native Codex and Gemini skill locations while the installer writes a converged shared root.

The issue is not that shared source is wrong. The issue is that Loaf has no explicit policy for which surfaces are portable and which must remain harness-native.

## Strategic Alignment

- **Vision:** Advances Loaf as an agent-agnostic CLI by making the cross-harness layer intentional instead of accidental, while still leveraging each harness where it is strongest.
- **Personas:** Helps framework users who install Loaf into multiple tools and expect each tool to feel native rather than partially aliased through another tool's layout.
- **Architecture:** Preserves the “skills as universal knowledge layer” principle from `docs/ARCHITECTURE.md`, but refines it: shared knowledge source does not imply shared runtime surface.

## Solution Direction

Adopt a **surface-first target model**.

### 1. Separate canonical source from runtime delivery

Loaf keeps one canonical source tree under `content/`, but each target is evaluated surface by surface:

- `skills`
- `agents`
- `hooks`
- `commands`
- `runtime plugins`
- `settings/config`
- `install roots`

Shared authoring stays. Shared runtime is opt-in and must be justified.

### 2. Define portable vs native surfaces explicitly

**Portable surface**
- A surface is portable only when the final rendered artifact is intentionally identical across a declared target family.
- Portability must be verified by tests, not assumed.

**Native surface**
- A surface is native when the harness exposes distinct semantics, lifecycle, eventing, metadata, or UX.
- Native surfaces are always built, installed, and documented per target.

### 3. Policy by surface

**Skills**
- Skills remain canonically authored once.
- Skills may be delivered through a shared artifact family only when outputs are intentionally identical and no harness-specific behavior is being hidden.
- Shared skill families are a delivery optimization, not the default worldview.

**Agents**
- Agents are always harness-native.
- Agent sidecars remain the source of truth for harness-specific metadata and capabilities.

**Hooks**
- Hooks are always harness-native.
- Hook generation should map from Loaf hook intent to each harness's native event model rather than collapsing to a lowest common denominator.

**Commands / plugins / settings**
- Always harness-native.
- These are the main place to exploit harness-specific strengths.

**Task tracking**
- Two tiers: portable (Loaf tasks — `.agents/tasks/` files) and harness-native (e.g., Claude Code TaskCreate, or equivalent per-harness progress APIs).
- Loaf tasks are the system of record for spec-driven, cross-session, auditable work. Harness-native tasks are ephemeral single-session progress indicators.
- Loaf should provide guidance (skill-level heuristic) on when to use which tier. Optionally, Loaf may bridge the two: e.g., `loaf task --ephemeral` creates harness-native tasks where supported, falls back to in-memory otherwise.
- The heuristic: use Loaf tasks when work spans sessions, is tied to a spec, or needs cross-harness visibility. Use harness-native tasks for single-session execution tracking within a conversation.

### 4. Immediate target classification

**Claude Code**
- Native for skills, agents, hooks, MCP packaging

**OpenCode**
- Native for skills, agents, commands, runtime plugins

**Cursor**
- Skills may remain in a portable family if the artifact is identical
- Agents, hooks, templates, and runtime behavior stay native

**Codex**
- Skills may remain in a portable family if the artifact is identical
- Hooks stay native to Codex's reduced hook contract

**Gemini**
- No longer modeled as “skills only”
- Gemini becomes a first-class native target for agents, hooks, and settings where supported
- Gemini skills may remain in a portable family only if the artifact still matches the family after Gemini-native features are added elsewhere

**Amp**
- Skills may remain in a portable family if the artifact is identical
- Runtime plugin remains native

### 5. Installer rule

Installers must be driven by the declared surface model, not by ad hoc target history.

That means:
- no shared install root unless the target family explicitly shares that surface
- no writing duplicate artifacts into multiple locations by default
- no claiming a native install location in docs if the installer does not actually use it
- no target treated as “skills only” when its native harness supports richer surfaces

### 6. Build-system rule

The build graph should express:

- canonical source generation
- optional portable artifact families
- native per-target adapters

A target can consume both:
- a portable skill family artifact
- its own native agents/hooks/plugins/settings outputs

This keeps the build efficient without flattening the runtime model.

## Scope

### In Scope

- Define a formal surface matrix for all supported targets
- Refactor target builders and installers to follow that matrix
- Promote Gemini from “skills only” to a native-capability target
- Add tests for portable skill family equivalence and native-surface divergence
- Align README/install docs with actual install behavior
- Add cleanup behavior for stale legacy installs that cause duplicate discovery

### Out of Scope

- Full redesign of skill content or all skill descriptions
- Reworking Claude Code MCP policy beyond surface classification
- New harness support beyond the current target set
- Backwards-compatible migrations for every historical install path

### Rabbit Holes

- **Per-harness skill forking.** Do not fork skill content per harness unless a real behavior difference requires it. Shared source remains the default.
- **Universal runtime abstraction.** Do not invent a meta-hook or meta-agent runtime that hides harness semantics. The whole point is to expose them.
- **Perfect migration coverage.** Do not spend effort reconstructing every past install layout. Clean up the current known legacy roots and move on.

### No-Gos

- Do not keep using shared runtime roots as an undocumented convergence hack.
- Do not ship Gemini as “skills only” after this spec.
- Do not let docs, build outputs, and installer paths disagree.
- Do not add dormant sidecar mechanisms for targets that have no actual divergent fields.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Gemini native hook/agent support is broader or narrower than assumed | Medium | High | Start with documented Gemini surfaces only; defer unsupported event mappings |
| Shared skill family starts drifting silently across targets | Medium | Medium | Add parity tests that fail the build when family artifacts diverge unexpectedly |
| Installer cleanup removes user-managed files in legacy locations | Low | High | Only remove Loaf-managed artifacts using markers/signatures; never delete unknown user content |
| Build system becomes harder to understand after introducing surface families | Medium | Medium | Centralize the surface matrix in one config/module and generate docs/tests from it |

## Open Questions

Resolved during breakdown (2026-05-01) and Codex review:

- [x] **Gemini agents in first wave?** No — TASK-155 is P2 and re-evaluated after hooks (TASK-153) and settings (TASK-154) land. Drop if hooks+settings alone provide sufficient parity per spec priority order #2.
- [x] **Surface-matrix location?** Dedicated TS module (`cli/lib/build/surface-matrix.ts`), not `targets.yaml`. Typed source of truth; generate yaml/docs from it if needed. Resolved in TASK-150.
- [x] **Cursor/Codex/Gemini/Amp as one portable family?** Yes for first wave (`portable:skills-v1`). Parity gate (TASK-158) is the early-warning system that signals when divergence forces a split. Re-check after Gemini native surfaces land.
- [x] **Auto-cleanup during `install --upgrade`?** No — detection + manual-cleanup guidance only (TASK-156). Auto-clean carries Risk row 3 safety concerns; deferred to follow-up spec. T7 wording updated above.
- [x] **Bridge harness-native task tracking?** No — skill-level guidance only for SPEC-024. The Loaf-task vs harness-native heuristic is documented in TASK-157 acceptance (T11 guidance layer). `loaf task --ephemeral` bridge deferred.
- [x] **Which harnesses expose task APIs?** Out of scope for SPEC-024. Only Claude Code (TaskCreate/TaskUpdate) is currently classified `native` for the task-tracking surface in TASK-150's matrix. Other targets default to `portable` (Loaf tasks) until their APIs are catalogued in a follow-up.

Implementation-level questions raised by Codex review (resolved):

- [x] **Gemini hook/settings surfaces in scope for TASK-153/154?** Documented Gemini surfaces only at implementation time. Map existing Loaf hook intents (`config/hooks.yaml`) that have clean Gemini equivalents; skip unmapped intents with a logged warning. No speculative event names. Risk row 1 mitigation.
- [x] **What counts as "known stale roots" for TASK-156?** Static enumerated list, no heuristics. Specifically: shared install roots written by the pre-SPEC-024 installer that the new matrix-driven installer no longer uses, plus paths the README documented but the installer didn't actually use (No-Go #3). Detection logic is a static list keyed by target.
- [x] **Parity comparison — pre- or post-sidecar?** Fully rendered tree per target. Sidecars are native-target-specific transforms; portable-family targets don't apply them. If a target needs target-specific skill transformation, the matrix moves it to native — the parity gate is the signal.

## Test Conditions

- [ ] T1: A single source of truth defines which surfaces are portable vs native for every target
- [ ] T2: `loaf build` produces native agents/hooks/plugins/settings only for targets that support them
- [ ] T3: Gemini build output includes at least one native surface beyond skills
- [ ] T4: Portable skill-family targets either produce byte-identical skill trees or fail with a clear parity error
- [ ] T5: OpenCode remains intentionally divergent and does not participate in the portable skill family
- [ ] T6: Installer writes artifacts only to declared roots for that target/surface
- [ ] T7: Legacy Loaf-managed duplicate installs can be detected and flagged for cleanup without touching user-owned files (manual-cleanup guidance is the first-wave deliverable; auto-clean deferred)
- [ ] T8: README install-location documentation matches the actual installer behavior
- [ ] T9: A multi-target user can install Codex, Cursor, and Gemini without duplicate skill discovery caused by stale Loaf-managed paths
- [ ] T10: Existing target-specific native capabilities still work after the refactor (Cursor hooks, Codex hook install, OpenCode commands/runtime plugin)
- [ ] T11: The surface matrix includes task tracking as a surface, with clear portable/native classification per target

## Priority Order

1. **Surface-model refactor** + Gemini hook/settings support. Go/no-go: “skills only” language eliminated.
2. **Gemini native agents** — drop if hooks alone provide sufficient parity.
3. **Legacy cleanup** — scope down to detection + warning if it slows the core refactor.
