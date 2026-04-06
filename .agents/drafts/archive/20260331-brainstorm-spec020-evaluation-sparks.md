---
title: "Brainstorm: SPEC-020 Evaluation — Cross-Harness Sparks"
type: brainstorm
created: 2026-03-31T11:30:00Z
status: active
tags: [cross-harness, hooks, skills, agents, architecture, AAIF]
related: [SPEC-020, SPEC-014, SPEC-018]
---

# Brainstorm: SPEC-020 Evaluation — Cross-Harness Sparks

**Date:** 2026-03-31
**Session:** Deep evaluation of SPEC-020 against all 6 harness docs (Claude Code, Cursor, Codex, OpenCode, Amp, Gemini), Agent Skills spec, AAIF context, and [Nate B Jones video](https://www.youtube.com/watch?v=0cVuMHaYEHE) on industry convergence.

## Context

SPEC-020 was shaped, evaluated against official docs for all harnesses, and refined through multiple rounds. Along the way, ideas surfaced that are outside SPEC-020's scope but worth capturing for future work.

## Sparks

### 1. Enforcement vs. Nudge — a hook design principle

Hooks should only enforce things the agent **cannot be trusted** to do (secrets, git gates, dangerous commands). Everything else (linting, testing, formatting) is a nudge — better as skill instructions where the agent exercises judgment.

**Why it matters:** This principle should govern all future hook additions, not just the SPEC-020 migration. "If the agent should do it but might forget → skill instruction. If the agent must not do it even if it tries → hook." Could become a section in CLAUDE.md or a decision framework doc.

### 2. Cursor `subagentStart` hook for skill injection

Cursor subagents can't auto-load skills (known platform limitation). But Cursor's `subagentStart` hook fires when a subagent spawns. A hook could inject `additionalContext` with skill file paths — solving the limitation at the hook layer. Concrete: map subagent type → relevant skills → inject "Read .agents/skills/{name}/SKILL.md first."

### 3. `loaf skill test` — cross-harness routing validation

Validate skill descriptions route correctly across harnesses. Three checks: (1) truncation safety — first 250 chars contain trigger phrases (Claude Code), (2) disambiguatability — confusable skills distinguishable by description alone, (3) coverage — every skill has negative routing ("Not for..."). Could feed descriptions through a mock routing model and check hit rates.

### 4. `loaf lint` command — skill quality checks

Several ideas converge on a lint/validation command: description truncation safety, skill size (< 500 lines), reference depth (one level), frontmatter completeness, sidecar field audit. Could be `loaf lint` or `loaf skill lint` or part of `loaf build --check`.

### 5. Hooks are the fragmented frontier — Loaf as de facto standard

Skills have converged (SKILL.md, Agent Skills spec, AAIF). Hooks have not — JSON vs TypeScript, 5-30+ events, different exit codes, different blocking models. If `loaf check` becomes a common CLI backend that multiple harnesses call, its interface (stdin JSON, exit codes, stdout messages) is a de facto hook standard. Could Loaf help drive hook standardization via AAIF?

### 6. Skill composition / dependency mechanism (AAIF contribution)

The Agent Skills standard has no `requires-skills` field. Claude Code's `skills` field on agents is a workaround but tool-specific. A standard dependency declaration would enable portable skill chains. AAIF contribution candidate — the standard is governed by the Agentic AI Foundation (Linux Foundation) co-founded by Anthropic, OpenAI, Block.

### 7. Capability negotiation protocol (AAIF contribution)

Before loading a skill, the harness could declare which extension fields it supports (hooks, context fork, dynamic injection, subagent types). Skills could adapt content based on capabilities. No harness implements this today, but as the standard evolves under AAIF governance, it could become real.

### 8. Skill versioning (AAIF contribution)

No standard `version` field in Agent Skills spec. Loaf injects version at build time but it's Loaf-specific. A standard version field + compatibility declaration (`requires-agent-version: "1.0.0+"`) would help ecosystem tooling.

### 9. Skill verification instruction pattern

The ~20 hooks being migrated to skill instructions need a consistent pattern: "After editing [language] files, run `[tool] [flags]`. If [tool] is not available, skip." The "if available" clause matters — hooks could check `command -v`, but skill instructions should be graceful. This pattern should be standardized across all language/infra skills.

### 10. OpenCode's `shell.env` for environment injection

OpenCode fires a `shell.env` event for injecting environment variables into shell execution. Loaf could set `LOAF_PROJECT_ROOT`, `LOAF_SKILL_PATH`, or other context variables that skills and hooks reference. Unique to OpenCode — no equivalent on other harnesses.

### 11. Hook capability matrix as a generated artifact

The cross-harness hook comparison (CC 25 events, Cursor 18+, Codex 5, OpenCode 30+) is useful but will go stale. Could be a `loaf` build artifact — read each target's hook config and emit a comparison table. Living docs > static docs.

### 12. `failClosed` audit across all hooks

Security hooks got `failClosed: true`. But a systematic audit of every remaining hook asking "if this hook crashes, should the action proceed?" could surface non-obvious cases — especially for git workflow hooks where fail-open might allow a broken push.

### 13. Cursor rules (`.mdc`) as a Loaf target

Cursor has `.cursor/rules/*.mdc` files (always-apply, auto-attach, agent-requested, manual) — separate from skills. Some Loaf skill content might be better expressed as always-apply rules. Not clear the complexity is worth it, but worth watching if users request it.

### 14. Install path intelligence

OpenCode scans `.opencode/`, `.claude/`, `.agents/` for skills. `loaf install` could detect which tools are installed and choose the path that maximizes coverage — `.agents/skills/` if multiple tools are present, tool-specific path if only one.

### 15. Performance profiling per skill

Document context window usage (metadata vs. full content), token costs for typical invocations. Could start simple: `loaf build` emits token count per skill in build output. Would inform skill design decisions.

### 16. Distribution registry for portable skills

Central registry of Agent Skills (GitHub-based?) with metadata for filtering by capability. 30+ tools have adopted the standard — a shared registry would accelerate ecosystem growth. Larger than Loaf, but Loaf could be an early contributor. Watch AAIF for registry initiatives.

### 17. Cursor's granular hook events as enforcement targets

Cursor splits `PreToolUse` into `beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile`, `afterFileEdit`, plus Tab hooks. More targeted enforcement than generic pre-tool. Worth exploring when Loaf's hooks.yaml supports per-target event overrides — e.g., a Cursor-only `beforeShellExecution` hook that doesn't exist on Claude Code.

---

## Theme 6: Instruction Adherence (from GSD/gstack/Superpowers Research)

### 18. XML-tagged instruction sections for stronger attention boundaries *(promoted to SPEC-020)*

GSD uses `<step>`, `<deviation_rules>`, `<success_criteria>` XML tags instead of Markdown headings for critical procedural sections. XML tags create stronger attention boundaries — the model is less likely to skip or conflate XML-delimited sections. Worth evaluating for Loaf's workflow skills (implement, shape, research) where step adherence matters most. Not a wholesale format change — selective use for high-stakes procedural sections.

### 19. Token budget tracking in `loaf build` output

gstack tracks token cost per skill at build time and prints a summary table. Knowing that `/implement` costs ~4000 tokens and `python-development` costs ~1500 would inform skill design decisions. Straightforward to add: `content.length / 4` per SKILL.md after template resolution.

### 20. Pressure testing as skill validation

Superpowers validates discipline skills by running scenarios that combine 3+ pressures (time, sunk cost, authority) and documenting whether the model holds to the process. Could become a `loaf skill test` methodology — run each skill through adversarial prompts and check if instructions are followed.

### 21. Anti-rationalization as a first-class skill authoring pattern *(promoted to SPEC-020)*

Superpowers' most effective technique: enumerate the exact thoughts the model will have when trying to skip the process. "If you think 'this is too simple for a spec' — that's when you need one most." Naming the escape hatch before the model reaches for it preempts the rationalization. Meincke et al. (2025) found persuasion techniques doubled compliance from 33% to 72%.

### 22. Preamble tiering — instruction density scaled by consequence *(promoted to SPEC-020)*

gstack's 4-tier preamble system: T1 (lightweight tools) gets minimal boilerplate; T4 (ship, review, QA) gets full behavioral payload. Applied to Loaf as instruction tiering by skill type: reference < cross-cutting < workflow. Key insight: when the model has fewer instructions to follow, it follows each one more reliably.

### 23. `INVOKE_SKILL` — cross-skill composition pattern

gstack's `{{INVOKE_SKILL:plan-ceo-review}}` resolver generates prose that tells the model to read another skill's file and execute specific sections. Enables workflow chaining without copy-pasting instructions. Loaf has no cross-skill composition mechanism — skills are currently independent. Worth exploring for workflow chains (brainstorm → shape → breakdown → implement).

---

*Captured from SPEC-020 evaluation session. Promote individual sparks to ideas via `/loaf:idea` when ready to shape.*
