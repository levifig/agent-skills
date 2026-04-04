---
id: SPEC-022
title: Native Agent Profile Routing
source: >-
  direct — conversation about PR #15 review revealing gap between profile
  definitions and usage
created: '2026-04-03T12:23:18.000Z'
status: drafting
appetite: 1-2 days
---

# SPEC-022: Native Agent Profile Routing

## Problem Statement

Loaf defines three functional profiles (Smith/Implementer, Sentinel/Reviewer, Ranger/Researcher) with clear tool boundaries and behavioral contracts (SPEC-014). These profiles are built and distributed to Claude Code, Cursor, and OpenCode (SPEC-020). But **nothing makes Claude use them.**

When an agent is spawned today, Claude defaults to generic `general-purpose` subagents unless the orchestration skill's delegation instructions happen to be in context. The profiles exist as inert artifacts — correctly built, correctly placed, never activated. A user installing Loaf today gets profiles in `plugins/loaf/agents/` that sit unused while Claude spawns anonymous agents with no tool restrictions, no behavioral contracts, and no naming convention.

SPEC-014 delivered R8-R10 (profile definitions with `subagent_type` routing). This spec delivers the **enforcement layer** that makes profile routing the default when Loaf is installed.

## Strategic Alignment

- **Vision:** Advances the "Autonomous Execution" pillar — profiles are the mechanism that gives subagents bounded autonomy with predictable tool access
- **Personas:** Benefits all Loaf users who delegate work to subagents — consistent behavior regardless of which Claude session spawns the agent
- **Architecture:** Respects the constraint that "profiles are Claude Code infrastructure — other targets activate knowledge through skills alone" (ARCHITECTURE.md). This spec adds enforcement for targets that support agents and improves skill instructions for those that don't

## Solution Direction

**Hook-assisted routing (medium enforcement):** A PreToolUse hook on the `Agent` tool that intercepts subagent spawning. When a known Loaf profile is specified, the hook injects the profile's behavioral contract and suggests a lore-convention name. When a generic agent is spawned, the hook warns with available profiles. The hook never blocks — it nudges, enriches, and makes profiles the path of least resistance.

Three complementary layers:

1. **Build-time:** Generate a profile registry manifest (`profiles.json`) listing available profiles, their tool boundaries, and naming conventions. Distributed alongside existing agent artifacts.

2. **Hook-time:** PreToolUse hook on `Agent` (and `Task` if applicable) reads the registry, enriches profile-based spawns, and warns on generic spawns. Output is a prompt injection that the harness passes to the subagent.

3. **Instruction-time:** SessionStart hook outputs available profiles as part of session context. Orchestration skill updated with stronger default-routing language: "All subagent spawning MUST use a Loaf profile" (instruction, not enforcement).

### Target Coverage

| Target | Agent Support | Routing Mechanism |
|--------|--------------|-------------------|
| Claude Code | Yes | PreToolUse hook + profile injection |
| Cursor | Yes | PreToolUse hook (if Agent event available) + skill instructions |
| OpenCode | Yes | Skill instructions (hook surface TBD) |
| Codex | No | Skill instructions only |
| Gemini | No | Skill instructions only |
| Amp | No | Skill instructions only |

## Scope

### In Scope

- Profile registry manifest generation at build time
- PreToolUse hook on Agent tool for Claude Code
- PreToolUse hook generation for Cursor (if Agent event is in their hook surface)
- SessionStart profile announcement (extend existing hook)
- Orchestration skill delegation.md update with mandatory profile language
- Lore name suggestion in hook output (Dwarvish for Smiths, Elvish for Sentinels, Mannish for Rangers)
- Profile content injection into subagent prompt via hook output

### Out of Scope

- Blocking generic agent spawns (explicit design choice — hook never blocks)
- Agent support for Codex, Gemini, or Amp targets
- Changing profile definitions (those are SPEC-014 deliverables, already complete)
- Skill preloading at spawn time (orchestrator decides skills, not the hook)
- Model selection per profile (deferred — all profiles inherit parent model)

### Rabbit Holes

- **Lore name generation complexity.** Don't build a name generator. Use a small static list per race (10-15 names each) and cycle through them. The name is display flavor, not identity.
- **Profile capability negotiation.** Don't try to dynamically validate whether a target supports the profile's tool set. Build-time sidecar verification (SPEC-020) already handles this.
- **Hook chaining with existing hooks.** Don't compose profile routing with other PreToolUse hooks. Keep it independent — the hook reads context, emits text, done.

### No-Gos

- Do not block Agent tool calls. Ever. This is hook-assisted, not hook-enforced.
- Do not modify the subagent's prompt directly — emit guidance text that the harness injects. The subagent sees the profile content as context, not as a system override.
- Do not add agent support to targets that don't have it (Codex, Gemini, Amp).

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Cursor doesn't expose Agent/Task as hook events | Medium | Medium | Degrade to skill instructions only for Cursor. Check their 18-event surface during implementation. |
| Hook output too verbose, pollutes subagent context | Medium | Low | Keep profile injection to essentials: tool boundary, behavioral contract, naming. No full SOUL.md. |
| Claude ignores hook warnings and spawns generic anyway | Low | Low | Acceptable — this is nudge-based by design. Profile content still enriches compliant spawns. |
| Performance overhead on every Agent call | Low | Low | Hook reads a small JSON manifest + emits text. No subprocess, no file I/O beyond the registry read. |

## Open Questions

- [ ] Does Cursor's hook surface include `subagentStart` or similar Agent-level events? (SPEC-020 mentions 18+ events but doesn't confirm Agent routing)
- [ ] Should the hook inject profile content as a prompt-type hook (text injection) or as a command-type hook (loaf CLI call that returns text)?
- [ ] Should the profile registry include skill recommendations per profile, or keep that purely in the orchestration skill?

## Test Conditions

- [ ] T1: `loaf build` generates `profiles.json` manifest in Claude Code plugin output
- [ ] T2: Spawning `Agent(subagent_type: "implementer")` triggers hook that injects Smith behavioral contract
- [ ] T3: Spawning `Agent()` (generic) triggers hook that warns with available profiles
- [ ] T4: SessionStart output includes available profile names and their purposes
- [ ] T5: Lore name suggestion appears in hook output (e.g., "Suggested name: Borin — <purpose>")
- [ ] T6: Hook output does not exceed 500 tokens (keeps subagent context lean)
- [ ] T7: `loaf build` succeeds for all 6 targets (no regressions)
- [ ] T8: Orchestration skill delegation.md references mandatory profile usage
- [ ] T9: Targets without agent support (Codex, Gemini, Amp) build clean with no profile artifacts
- [ ] T10: Hook emits valid JSON response (not blocked, passed with profile context)

## Circuit Breaker

At 50% appetite: If Cursor Agent-event integration proves complex, defer Cursor to instruction-only and ship Claude Code + skill updates.

At 75% appetite: If hook injection architecture requires runtime-plugin.ts changes beyond the profile manifest, ship with manifest + SessionStart + skill updates only. The hook can follow as a fast-follow.
