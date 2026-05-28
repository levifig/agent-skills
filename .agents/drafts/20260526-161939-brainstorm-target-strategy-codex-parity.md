---
title: "Brainstorm: Target Strategy and Codex Parity"
type: brainstorm
created: 2026-05-26T16:19:39Z
status: active
tags:
  - target-strategy
  - codex
  - hooks
  - parity
related:
  - docs/STRATEGY.md
  - docs/ARCHITECTURE.md
  - config/targets.yaml
  - config/hooks.yaml
---

# Brainstorm: Target Strategy and Codex Parity

**Date:** 2026-05-26
**Session:** Reassess Loaf's supported harness targets after confirming Codex hooks support, then decide whether to prune weak targets and promote Codex toward Claude Code parity.

## Session Journey

The discussion started with a narrower question: can Loaf, as a harness, be functional without hooks? The answer was: Loaf can degrade into skills plus manual CLI commands, but the current reliability harness cannot fully survive without another event source. Hooks carry session lifecycle, compaction recovery, journal automation, KB/task refresh, and enforcement.

That reframed the target strategy. If hooks are load-bearing, then skills-only targets are no longer just "less complete"; they represent a different product tier. The user proposed discontinuing Amp and Gemini, focusing on Claude Code, improving OpenCode, keeping Cursor, and bringing Codex to full parity now that Codex has hooks.

The follow-up parity pass clarified the current state:

- Cursor is closest to Claude Code parity but still has risks around PATH-dependent `loaf`, generated event names, and unproven lifecycle behavior.
- OpenCode has useful content support, but its runtime plugin does not currently execute all session lifecycle hooks and appears stale against current OpenCode event naming.
- Codex is not parity today; Loaf's current Codex target is only skills plus Bash-matching enforcement hooks. But the Codex platform now exposes enough hook surface to make parity a plausible target-implementation problem rather than an upstream blocker.

The emerging direction is not "support fewer tools because fewer is easier." It is: support fewer tools because Loaf's product value is the harness behavior, and harness behavior requires real lifecycle and hook semantics.

## Current Target Reading

### Claude Code

Claude Code remains the reference target. It has the strongest Loaf integration today:

- skills
- functional agent profiles
- bundled plugin distribution
- plugin-local `loaf` binary
- hook registration for pre-tool, post-tool, session lifecycle, task completion, user prompt context injection, and compaction recovery

This should remain the behavioral source of truth for parity work.

### Cursor

Cursor is a near-parity target, not a completed parity target.

It currently generates skills, agents, hook scripts, and `hooks.json`. That is the right shape. The remaining concern is whether generated event names and runtime behavior match Cursor's actual hook surface for all lifecycle events, especially `TaskCompleted`, `UserPromptSubmit`, `PreCompact`, and `PostCompact`.

Cursor also depends on `loaf` being available on `PATH`, unlike Claude Code's plugin-local binary flow. That may be acceptable, but it should be explicit in the parity contract rather than hidden as an install-time warning.

### OpenCode

OpenCode is useful but below parity.

It generates skills, agents, commands, and a runtime plugin. The problem is behavioral coverage:

- hook data includes more lifecycle hooks than the runtime handler dispatches
- prompt/context injection paths do not appear equivalent to Claude Code
- current compaction event naming likely needs revalidation against upstream OpenCode docs
- successful hook stdout is not clearly propagated as model-visible context

OpenCode should stay in scope only if the next spec treats it as a real runtime-integration audit, not a docs/table cleanup.

### Codex

Codex is the strategic pivot.

Current Loaf support is thin: the Codex target builds skills and emits a small `PreToolUse` enforcement-only `hooks.json`. That is below Cursor and OpenCode in current Loaf implementation.

But platform capability has changed. Codex now supports hooks for session start, tool events, permission requests, prompt submission, and stop behavior. That makes Codex parity realistic enough to deserve first-class investment.

The target should be rebuilt rather than patched. Codex needs a generator that translates Loaf's hook model into Codex-native matcher groups, command hooks, `additionalContext`, plugin-bundled hooks, and the current install/trust model.

### Gemini and Amp

Gemini and Amp should be discontinued as active targets.

Gemini is skills-only in the current model. Amp's runtime plugin surface is too limited for Loaf's session lifecycle requirements. Keeping them active preserves breadth at the expense of the thing that makes Loaf valuable: reliable harness behavior.

They can remain as archived/legacy targets or be removed behind a deprecation cycle, but they should not compete for parity work.

## Proposed Target Tiers

| Tier | Target | Contract |
|------|--------|----------|
| Reference | Claude Code | Defines Loaf's expected harness behavior |
| First-class parity | Codex, Cursor | Must support core lifecycle, hooks, profiles or equivalent agent behavior, install, tests |
| Active but lower confidence | OpenCode | Keep only with explicit runtime audit and known unsupported paths |
| Deprecated | Gemini, Amp | No new features; remove or archive after migration window |

## Parity Definition

Target parity should mean behavioral parity, not identical file layout or identical hook names.

A first-class target should support:

1. **Skills** -- all active Loaf skills install and route correctly.
2. **Agent/profile behavior** -- functional tool boundaries exist, or the target has an explicit equivalent.
3. **Pre-tool enforcement** -- secrets, security audit, commit validation, and workflow checks run at the right time.
4. **Post-tool side effects** -- task refresh, KB staleness tracking, and journal auto-entries work.
5. **Session lifecycle** -- start/resume, end/stop, prompt context injection, and compaction or resumption recovery are covered.
6. **Context injection** -- advisory hook output becomes model-visible where intended.
7. **Install/update safety** -- user hooks are preserved and Loaf-managed hooks are replaceable.
8. **Generated artifact tests** -- built target output is parsed and behaviorally checked, not just snapshotted.
9. **Runtime smoke tests** -- at least one command path proves hooks actually execute in the harness or a faithful fixture.

## Recommendation

Shape a spec around a target-policy reset:

1. Deprecate Amp and Gemini as active targets.
2. Define a first-class target parity contract.
3. Rebuild Codex support around the current Codex hook model.
4. Audit Cursor against the parity contract and fix event-name/PATH/runtime gaps.
5. Audit OpenCode separately and decide whether it graduates to first-class or remains best-effort.

Do not start by editing `README.md` tables. The tables are downstream of the decision. Start by writing the target contract and the implementation matrix, then let docs follow.

## Open Questions

- Does Codex have a practical equivalent for `PreCompact` / `PostCompact`, or should Codex parity use `Stop`, `SessionStart`, transcript access, and journal enrichment instead?
- Should Codex subagents map to Loaf's functional profiles, and if so, where should those profile definitions live in the Codex output tree?
- Is Cursor's lowercase generated `taskcompleted` / `userpromptsubmit` event naming actually accepted by Cursor, or is this silent non-parity?
- Should OpenCode remain active if it cannot make hook stdout model-visible as context?
- What deprecation window is appropriate for Amp and Gemini: immediate removal from generated targets, one release of warnings, or archive-only docs first?
- Should target parity be verified with generated-artifact tests only, or with harness-specific runtime smoke tests?

## Sparks

- **Target parity contract** -- Create a durable checklist that every first-class target must satisfy before README tables can claim full support.
- **Codex parity spec** -- Shape a dedicated spec for replacing the current Bash-only Codex generator with full hook lifecycle support.
- **Target deprecation policy** -- Define how Loaf retires weak harness targets without leaving stale install paths, docs, or generated artifacts.
- **Hook event compatibility matrix** -- Maintain a source-of-truth matrix mapping Loaf events to each harness's native event names and known gaps.
- **Runtime fixture harness** -- Build a small test harness that can execute generated hook plugins against representative event payloads for Claude Code, Codex, Cursor, and OpenCode.
