---
id: SPEC-014
title: Skill Activation Redesign — foundations decomposition + agent elimination
source: brainstorm
created: '2026-03-24T23:30:00.000Z'
status: complete
---

# SPEC-014: Harness Redesign — Functional Profiles, Skill Activation, Build Cleanup

## Problem Statement

Loaf's knowledge activation has two structural problems (a third — TDD discipline — is addressed downstream in SPEC-017):

1. **The foundations skill is too broad to trigger precisely.** Its description ("Establishes code quality, commit conventions, documentation standards, and security patterns") covers 17 reference files spanning git workflow, debugging, security, code review, and documentation. When the context is "merge this PR," the skill's description doesn't match — and the squash merge conventions in `commits.md` don't load. Concrete incident: 2026-03-24, squash merge conventions missed during PR merge.

2. **Role-based agents add indirection without value.** All 8 role agents (pm, backend-dev, frontend-dev, qa, dba, design, devops, power-systems) are thin routers with 2-sentence system prompts that say "Your skills tell you how." Skills contain all domain knowledge. The routing layer adds nothing that description-based skill activation doesn't already provide. Meanwhile, some targets (Codex, Gemini) don't support agents at all and work fine with skills alone.

These problems compound: if skills are the only knowledge layer that works across all targets, they need precise descriptions for context-based activation. Broad skills and role-agent routing both work against this.

## Strategic Alignment

- **Vision:** Loaf as a portable agentic harness — skills-first architecture works across all targets, not just Claude Code
- **Architecture:** Follows the CLI-driven harness direction (SPEC-010 absorbed PM's task management). Skills are the universal knowledge layer; agents are infrastructure, not identity.
- **Best practices alignment:** Claude Platform, agentskills.io, Cursor, and Codex all recommend focused, well-described skills as the primary activation mechanism.

## Solution Direction

### Track A: Foundations Decomposition

Split the monolithic `foundations` skill into 4-5 focused skills, each with a precise description optimized for context-based triggering. Reference file content stays the same — this is reorganization, not rewriting.

**Proposed split:**

| New Skill | References (moved from foundations) | Description Focus |
|---|---|---|
| **git-workflow** | `commits.md` | Branching strategy, commit conventions, PR creation, squash merge workflow |
| **debugging** | `debugging.md`, `hypothesis-tracking.md`, `test-debugging.md` | Systematic debugging, hypothesis tracking, flaky test investigation |
| **security-compliance** | `security.md`, `security-review.md` | Threat modeling, secrets management, compliance checks |
| **documentation-standards** | `documentation.md`, `documentation-review.md`, `diagrams.md` | ADRs, API docs, changelogs, Mermaid diagrams |
| **foundations** (slimmed) | `code-style.md`, `tdd.md`, `verification.md`, `code-review.md`, `review.md`, `permissions.md`, `observability.md`, `production-readiness.md` | Code style, naming conventions, TDD, verification, review discipline |

Each new skill gets:
- `SKILL.md` with precise description (action verb opener, user-intent phrases, negative routing)
- `SKILL.claude-code.yaml` sidecar (`user-invocable: false`, no `agent:` field)
- `references/` directory with moved files
- Relevant hooks reassigned via `skill:` field in `config/hooks.yaml`

### Track B: Agent Convergence to Functional Profiles

Converge 8 role-based agents into **3 functional profiles** defined by tool access boundaries, not domain identity. Keep 2 system agents unchanged. The main session becomes the coordinator — core orchestration principles are always active via SOUL.md, detailed procedures load on demand via skills.

**Research context:** AmpCode, Cursor, GitHub Copilot, and Claude Code's own built-in agents all use functional boundaries (tool access, context isolation), not role-based identity. Nobody in the industry ships role-based agents. See Deep Agents (LangChain) for the eval-driven quality loop that complements this model.

**The profile model — Tolkien-themed, race-aligned:**

Each profile maps to a Middle-earth race. The race carries meaning: it tells you the tool boundary, the naming style, and the perspective the profile brings.

| Role | Race | Concept | Tool Access | Name Style |
|---|---|---|---|---|
| **Warden** | Wizard | Coordinator — persistent AI partner, orchestrates, advises, delegates | Full | One persistent name (Warden's true name, defined in SOUL.md) |
| **Smith** | Dwarf | Implementer — forges code, tests, config, docs into existence | Full write | Dwarvish: hard, short — Borin, Náin, Dwalin |
| **Sentinel** | Elf | Reviewer — watches, guards, verifies. Sees what others miss. | Read-only | Elvish: flowing, musical — Galathir, Celindor, Ithren |
| **Ranger** | Human | Researcher — scouts far, gathers intelligence, reports back | Read + Web (no Write, no Bash) | Mannish: Anglo-Saxon — Aldric, Haleth, Beren |
| **Background runner** | — | Async non-blocking tasks | Read + Edit | System agent, haiku (unchanged) |
| **Context archiver** | — | Session preservation pre-compaction | Read + Edit + Serena | System agent, haiku (unchanged) |

Skills use concept names only ("spawn an implementer"). Profile definitions contain both lore and concept names in their content ("Smith (Implementer)"). SOUL.md maps lore to concept. This keeps skills lore-agnostic.

Smiths have specialities via skills — a Smith spawned with `python-development` + `database-design` becomes a backend engineer. A Smith spawned with `infrastructure-management` becomes a devops engineer. Same profile, different forge.

**The Warden — SOUL.md:**

The Warden is the coordinator's persistent identity — a Wizard who guides the fellowship but walks not in their stead. Defined in a new `SOUL.md` file:
- Warden persona and behavioral principles (delegation, session management, quality gates)
- References the fellowship by lore + concept name (but doesn't define profiles — those live in `content/agents/implementer.md`, `reviewer.md`, `researcher.md`)
- Council conventions (composition rules, who sits at the table)
- Loaded at SessionStart (hook) and referenced from AGENTS.md (cross-compatibility)

SOUL.md replaces the "~20 lines of orchestration principles in CLAUDE.md" concept. AGENTS.md references SOUL.md. The SessionStart hook ensures SOUL.md is always loaded.

**The Council:**

A Council convenes like the Council of Elrond — the races bring their perspectives to the table. No special title needed; they're Smiths and Rangers called to deliberate:

| Member | Race | Perspective | When present |
|---|---|---|---|
| **Smiths** | Dwarf | "Can we build this? What are the trade-offs?" | Always (technical) |
| **Rangers** | Human | "What do users need? Does this serve the vision?" | Default for non-technical questions (SPEC-016 product/UX seat) |
| **The Warden** | Wizard | Orchestrates, presents, doesn't vote | Always |

Sentinels (Reviewers) do NOT sit on councils — they come after, not during. Rangers serve dual roles: as researchers they scout; at the council they advocate for users. "Smiths only" = purely technical. "Smiths and Rangers" = product + technical. The Warden always orchestrates.

**Profiles vs spawned identity:**

A **profile** is a reusable capability shell — it defines only: tool boundary, behavioral contract, and reporting shape. A **spawned instance** receives its identity at runtime: name, concrete purpose, owned scope, and task-specific skill context.

Example: `Task(name: "Borin — auth API implementation", subagent_type: "implementer", prompt: "Implement the auth module. [loads: python-development, database-design]")`. The profile (`implementer`) gives tool access. The prompt gives purpose, scope, and skill context. The Dwarvish name is display-level lore — the system sees `implementer`.

This distinction is critical: **the implementer profile is NOT "backend-dev renamed."** It's a tool access shell. A spawned implementer becomes a backend developer, a DBA, a devops engineer, or a test author depending entirely on the prompt and skills loaded at spawn time. Skill context lives entirely in the spawn prompt — profiles have no default skill preloads.

**Why 3 profiles, not 8 agents or 0 agents:**
- Each profile has a **mechanically enforceable tool boundary** that makes its output trustworthy for a specific purpose
- **Sentinel** (Reviewer/Elf) is read-only — it *cannot* modify what it's reviewing, so its audits are independent
- **Ranger** (Researcher/Human) can read codebase + search web but *cannot* write or execute anything — purely observational. Output flows to the Warden as a structured report.
- **Smith** (Implementer/Dwarf) covers all "write" work regardless of domain — skills provide domain knowledge, not the profile
- A separate "test writer" profile was considered and rejected: same tool access as implementer, and the behavioral constraint ("only write test files") is prompt-level either way
- The coordinator doesn't need a profile — it's the main session with orchestration skills loaded

**What happens to each agent:**

| Agent | Action | Rationale |
|---|---|---|
| pm | **Remove → Warden** (Wizard) | Main session IS the Warden. SOUL.md defines identity. |
| backend-dev | **Remove → Smith** (Dwarf) | Domain knowledge via language/framework skills. |
| frontend-dev | **Remove → Smith** (Dwarf) | Domain knowledge via language/framework skills. |
| qa | **Remove → Sentinel** (Elf) | Testing discipline in skills. Read-only audit. |
| dba | **Remove → Smith** (Dwarf) | database-design skill activates by context. |
| design | **Remove → Sentinel** (Elf) | interface-design skill activates by context. Read-only review. |
| devops | **Remove → Smith** (Dwarf) | infrastructure-management skill activates by context. |
| power-systems | **Remove → Smith** (Dwarf) | power-systems-modeling skill activates by context. |
| background-runner | **Keep** | Real procedural system prompt for async execution. |
| context-archiver | **Keep** | Real procedural system prompt for session preservation. |

**Instance naming — purpose first, lore second:**

Each spawned instance gets a name combining functional purpose with a Middle-earth-style name matching its race. Names are generated on the fly by the Warden (original names in the linguistic style, not canonical characters), not reused within a session. The naming style signals the profile at a glance:

```
Implementer: Task(name: "Borin — auth API implementation", subagent_type: "implementer", ...)
Implementer: Task(name: "Náin — schema migration", subagent_type: "implementer", ...)
Reviewer:    Task(name: "Galathir — audit auth module", subagent_type: "reviewer", ...)
Researcher:  Task(name: "Aldric — survey auth libraries", subagent_type: "researcher", ...)
```

The `subagent_type` is always the concept name. The Dwarvish/Elvish/Mannish instance name is display-level lore — the naming style signals the profile at a glance, but the system uses concept names.

**Agent reference updates in skills:**

Workflow skills that currently reference `{{AGENT:backend-dev}}` etc. will be updated to reference profile-based spawning. The `{{AGENT:slug}}` substitution pattern is removed. Skills use **concept names only**: `Spawn an implementer for this task` or `Spawn a reviewer to audit this change`. Claude Code maps these to the Agent tool with `subagent_type: "implementer"` etc.

The lore layer (Smith, Sentinel, Ranger) is applied by SOUL.md and the profile definitions — not by skills. This keeps skills lore-agnostic: if the theme changes, skills don't need updating.

**Warden enforcement via SOUL.md:**

SOUL.md defines the Warden's persistent identity and orchestration principles. AGENTS.md references SOUL.md (ensuring cross-compatibility). Example content:

```markdown
# Arandil — The Warden

You are Arandil, the Warden — a Wizard who guides the fellowship
but walks not in their stead. You orchestrate, advise, and delegate.

## The Fellowship — Profiles
- **Smith** (Implementer/Dwarf) — full write. Forges code, tests, config, docs.
  Speciality via skills at spawn time. Dwarvish instance names.
- **Sentinel** (Reviewer/Elf) — read-only. Watches, guards, verifies.
  Elvish instance names.
- **Ranger** (Researcher/Human) — read + web. Scouts far, reports back.
  Mannish instance names.

## Principles
- Delegate forging to Smiths — don't write production code directly
- Delegate verification to Sentinels
- Delegate scouting to Rangers
- Sessions are mandatory for implementation work
- Tasks are tracked via `loaf task` CLI

## Council
Convene Smiths + Rangers for deliberation. Rangers advocate for users,
informed by their scouting. Sentinels come after, not during.
Smiths only for purely technical questions. The Warden orchestrates,
never votes.
```

Detailed procedures stay in skills (loaded on demand).

**Self-healing SessionStart hook:**

A SessionStart hook validates that SOUL.md is referenced and loadable. If present, it passes silently. If missing, the hook injects the Warden identity from a canonical template (`content/templates/soul.md`) and warns the user. This ensures every session has the Warden identity regardless of file state.

**Council session redesign:**

Handled separately in SPEC-016, which uses the racial composition model (Smiths + Rangers, Warden as coordinator, adaptive takes, strategic context always provided).

### Track B2: TDD Flow and Spec-as-Eval

Extracted to **SPEC-017** (TDD Harness). SPEC-014 establishes the profile model; SPEC-017 defines how those profiles are orchestrated in a TDD cycle, including the test-writing pattern, reviewer audit protocol, eval accumulation, and spec template evolution to binary Rs.

### Track C: Description Audit

Audit and rewrite descriptions for all ~24 skills (reference and workflow) to optimize triggering. Descriptions are the *only* thing loaded at startup — Claude sees all skill descriptions in its system prompt and uses them to decide which skills to load. For `user-invocable: false` skills, the description is the entire triggering mechanism.

**Description formula (100-300 chars, max 1024):**

1. **Third-person action verb opener:** "Covers...", "Establishes...", "Coordinates..."
2. **Specific scope terms:** Include key terms Claude will match against ("commit messages", "PR creation", "squash merge")
3. **User-intent phrases:** "Use when creating PRs, writing commit messages, or merging branches"
4. **Negative routing:** "Not for code style (use foundations) or security checks (use security-compliance)" — critical for disambiguation between skills that could overlap
5. **Success criteria** (workflow skills only): "Produces a bounded spec with test conditions and circuit breaker"

**Example (git-workflow):**
```yaml
description: >-
  Covers branching strategy, commit message conventions, PR creation,
  and squash merge workflow. Use when creating branches, writing commit
  messages, creating or merging pull requests, or managing git history.
  Not for code style (use foundations) or CI/CD pipelines (use
  infrastructure-management).
```

### Track D: Build System + Config Cleanup

**Build system:**
- Remove `{{AGENT:...}}` substitution pattern from `cli/lib/build/lib/substitutions.ts` (`buildAgentMap()`, `substituteAgentNames()`)
- Replace agent discovery (`discoverAgents()`) with profile-aware discovery — system agents (background-runner, context-archiver) + profile definitions (implementer, reviewer, researcher)
- Remove 8 role-agent files from `content/agents/`; add `implementer.md`, `reviewer.md`, `researcher.md`
- Profile definitions specify only: tool access boundary and behavioral contract. Skill context and naming are provided at spawn time by the coordinator, not baked into the profile.
- Keep system agent processing for background-runner and context-archiver (Claude Code only)
- Clean up agent-specific types in `cli/lib/build/types.ts`
- Update agent output generation in claude-code.ts, cursor.ts, opencode.ts for profiles

**Config cleanup:**
- Remove `plugin-groups` from `config/hooks.yaml` entirely — they are legacy dead code (the build system ignores them, confirmed in code comments: "Legacy plugin groupings kept for documentation purposes"). Skill discovery is via filesystem scan; hook assignment is via each hook's `skill:` field.
- Update hook `skill:` fields to reference new skill names where appropriate (e.g., `check-secrets` → `skill: security-compliance`)
- Remove `agent:` field from all skill sidecar files (`.claude-code.yaml`)

**SOUL.md + AGENTS.md update:**
- Create `SOUL.md` defining Arandil (the Warden), referencing fellowship profiles by lore + concept name, and orchestration principles
- Update `.agents/AGENTS.md` to reference SOUL.md
- Create `content/templates/soul.md` as canonical source for build distribution
- Add SessionStart hook that validates SOUL.md is loadable; injects from template if missing

## Scope

### In Scope

- Split foundations into 4-5 focused skills (new SKILL.md, sidecars, moved references)
- Slim down foundations to code quality core
- Replace 8 role-agent files with `content/agents/implementer.md`, `reviewer.md`, `researcher.md`
- Update all workflow skills that reference `{{AGENT:...}}` patterns to use profile-based spawning
- Core orchestration principles always active via SOUL.md; detailed skill loads on demand (no PM agent gate)
- Create SOUL.md with Warden identity, fellowship profiles, and council conventions; reference from AGENTS.md
- TDD flow and spec template evolution handled in SPEC-017
- Audit and rewrite descriptions for all ~24 skills
- Remove `plugin-groups` section from `config/hooks.yaml` (legacy dead code)
- Update hook `skill:` fields to reference new skill names
- Remove `agent:` field from all skill sidecars
- Update build system for profile-aware agent discovery
- Remove `{{AGENT:...}}` substitution pattern and `buildAgentMap()`/`substituteAgentNames()`
- Remove role-agent sidecar files
- Update `docs/ARCHITECTURE.md` to reflect profiles model
- Update `.agents/AGENTS.md` to reference SOUL.md
- Create canonical `content/templates/soul.md` template
- Add self-healing SessionStart hook (validates + backfills SOUL.md)

### Out of Scope

- Council redesign (handled in SPEC-016)
- Rewriting reference file content (only reorganization)
- Changes to the CLI tool itself (commands, types, etc.)
- Skill evaluation framework or activation analytics (separate idea)
- Changes to system agents (background-runner, context-archiver)
- Middle-earth name catalog (generated on the fly by the Warden in racial style, no static list needed)
- Agent Teams / peer-to-peer communication (future, additive to this model)
- Cross-harness testing beyond build output verification (manual testing on Cursor, Codex, etc.)

### Rabbit Holes

- Trying to build a "modes" or "presets" system — Claude Code doesn't have this primitive. Use skills + ephemeral agents instead.
- Over-engineering dynamic agent construction with project detection and smart skill selection — start simple (explicit skill lists in workflow skills), iterate later.
- Rewriting reference file content to match new skill boundaries — move files, don't rewrite them.
- Optimizing for targets that don't support agents (Codex, Gemini) beyond ensuring skills work — they already work.

### No-Gos

- Don't create role-based agents to replace role agents — profiles are functional, defined by tool access, not domain identity
- Don't embed domain knowledge in profile definitions — skills are the knowledge layer
- Don't break cross-harness build output — all targets must still build cleanly
- Don't change skill content beyond descriptions and organization — reference material stays intact
- Don't make the coordinator a separate spawned agent — main session IS the coordinator

## Dependencies

| Dependency | Type | Status | Notes |
|---|---|---|---|
| None | Hard | — | No hard dependencies. Touches content, config, and build code but doesn't require new CLI features or external systems. |
| SPEC-017 | Soft (downstream) | Drafting | TDD harness builds on top of SPEC-014's profile model. Ship SPEC-014 first. |
| SPEC-016 | Soft (downstream) | Drafting | Council redesign aligns with profile model but can ship independently. |

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Skill descriptions don't trigger as precisely as expected | Medium | Medium | Test with real prompts before and after. Iterate descriptions based on observed activation. |
| Behavioral constraints ignored (implementer writes production code when told to write tests) | Medium | Low | Advisory only — addressed in SPEC-017's TDD orchestration pattern. Reviewer catches violations. |
| Build system cleanup breaks a target | Low | High | Run `loaf build` for all targets after every change. Diff output against pre-change baseline. |
| Some workflow skills depend on agent behavior we haven't identified | Low | Medium | Grep for all `{{AGENT:` patterns and `agent:` sidecar fields. Map every reference before removing. |
| Orchestration principles in SOUL.md add too much weight to system prompt | Low | Medium | Keep to ~20 lines. Principles only, not procedures. |
| Parallel implementers conflict on shared files | Low | Medium | Coordinator scopes each implementer to independent concerns. Code review catches conflicts. |

## Resolved Questions

- **Plugin-groups:** Remove entirely. They're legacy dead code — the build system ignores them (confirmed in `hooks.yaml` comments and build target code). Skill discovery is via filesystem scan; hook assignment is via each hook's `skill:` field.
- **Hook reassignment:** Update hook `skill:` fields to reference the most relevant new skill (e.g., `check-secrets` → `security-compliance`, `validate-push` → `git-workflow`). This is organizational metadata only — hooks fire by `matcher:`, not by skill group. But correct attribution keeps config understandable.
- **Description length:** 100-300 chars is the sweet spot. Include scope terms, user-intent phrases, and negative routing. Max 1024 per spec. See Track C for the formula.

## Open Questions

- [x] Should the foundations `scripts/` directory (check-commit-msg.sh, check-secrets.sh, etc.) be redistributed to new skills, or kept centrally? → Keep centrally. Scripts are referenced by hooks via `skill:` field, not by SKILL.md. Moving them adds complexity without benefit. The `skill:` field on each hook points to the correct new skill for attribution.
- [x] The `implement` skill currently delegates to agents for parallel execution. Without role agents, should it use built-in `general-purpose` for isolation, or construct ephemeral agents with skills preloaded? → Use profile-based spawning: `subagent_type: "implementer"` or `"reviewer"`. Claude Code's Agent tool handles the rest.
- [x] Should `orchestration` gain a `disable-model-invocation: false` + `user-invocable: false` combo? → Yes. Orchestration becomes a reference skill — always available to the model (detailed procedures for SOUL.md principles), never in the user's `/` menu. Users interact through `/implement`.
- [x] How should profile definitions be structured? → As agent `.md` files with frontmatter (same format as today). Slimmer: tool boundary + behavioral contract only. No skill lists, no domain knowledge.
- [x] What defines the orchestration principles? → SOUL.md — the Warden's persistent identity file. Referenced from AGENTS.md, validated by SessionStart hook. See Track B for example content.

## Test Conditions

Binary requirements — each passes (✅) or fails (❌), no partial credit.

### Build integrity
- [ ] R0: `loaf build` succeeds for all 5 targets; agent-capable targets emit profiles; skill-only targets unchanged
- [ ] R1: `npm run typecheck` and `npm run test` pass
- [ ] R2: Zero `{{AGENT:` patterns in any built output

### Foundations decomposition
- [ ] R3: Each new skill (git-workflow, debugging, security-compliance, documentation-standards) has SKILL.md + sidecar + references/
- [ ] R4: Slimmed foundations no longer references moved files
- [ ] R5: A prompt "merge this PR" activates git-workflow (not just foundations)
- [ ] R6: A prompt "debug this flaky test" activates debugging (not just foundations)

### Profile model
- [ ] R7: 8 role-agent files deleted; 3 profile definitions exist (`implementer.md`, `reviewer.md`, `researcher.md`) with lore names in content
- [ ] R8: Reviewer profile has read-only tool access (mechanically enforced); `subagent_type: "reviewer"`
- [ ] R9: Researcher profile has read + web only, no write or bash access (mechanically enforced); `subagent_type: "researcher"`
- [ ] R10: Multiple implementer instances can run in parallel with purpose-first lore names; `subagent_type: "implementer"`

### Warden + orchestration
- [ ] R11: SOUL.md defines Warden identity, fellowship profiles, and council conventions
- [ ] R12: AGENTS.md references SOUL.md; SOUL.md sourced from canonical template
- [ ] R13: SessionStart hook detects missing SOUL.md and injects from template
- [ ] R14: `docs/ARCHITECTURE.md` reflects the profile model with Tolkien naming
- [ ] R15: Skills reference concept names only (e.g., "spawn an implementer"); lore applied by SOUL.md and profile definitions, not skills

### Description audit
- [ ] R16: Every skill has an audited description with action verb opener, user-intent phrases, and negative routing

## Circuit Breaker

**At 50% appetite:** Ship foundations decomposition (Track A) + description audit for new skills only. Role agents still exist but are flagged for removal. Build system untouched.

**At 75% appetite:** Ship foundations decomposition + agent convergence to profiles (Tracks A + B) + build cleanup (Track D). Description audit limited to reference skills.

**At 100% appetite:** All four tracks including comprehensive description audit and SOUL.md with Arandil identity.
