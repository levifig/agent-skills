---
id: SPEC-016
title: 'Council Advisory Redesign — dynamic specialists, structured output'
source: direct
created: '2026-03-27T20:50:19.000Z'
status: drafting
---

# SPEC-016: Council Advisory Redesign — Racial Composition, Structured Output

## Problem Statement

The current council-session skill is coupled to Loaf's fixed agent roster (backend-dev, frontend-dev, dba, etc.), requires a PM agent to orchestrate, pre-synthesizes recommendations that hide dissenting views, and bundles advisory + decision-recording + implementation into one monolithic workflow. The user never used it via `/council-session` — they expect councils to be triggered naturally from conversation and to function as an **advisory board**, not a decision-making pipeline.

Key gaps:
- Council members are limited to existing Loaf agents, which may not match the question's domain
- The user doesn't see individual member reasoning — only a synthesis
- The workflow pushes toward a decision and implementation, but the user wants to review and decide independently

## Strategic Alignment

- **Vision:** Fits "agent-creates, human-curates" — agents provide perspectives, human makes the call
- **Personas:** Serves the product owner who needs structured input from multiple domains to make informed decisions
- **Architecture:** Pure skill rewrite — no CLI changes, no new agents. The calling session acts as coordinator (no PM agent). Uses Agent tool for council member spawning. Designed to work within SPEC-014's profile model

## Solution Direction

### The Warden as Coordinator

The Warden (main session, see SPEC-014) acts as coordinator: frames the question, determines composition, spawns council members, collects responses, and presents results. No separate PM agent is spawned.

### Council Composition — Racial Model

When a council is triggered, the Warden convenes members like the Council of Elrond — each race brings its perspective to the table:

| Member | Race | Perspective | Backed by |
|---|---|---|---|
| **Smiths** (Implementers/Dwarves) | Dwarf | "Can we build this? What are the trade-offs?" | Relevant Loaf skills (e.g., `python-development`, `database-design`) |
| **Rangers** (Product/UX) | Human | "What do users need? Does this serve the vision?" | VISION.md + STRATEGY.md + insights from scouting |

Each member is:
1. A perspective rooted in their racial archetype (technical feasibility for Smiths, user advocacy for Rangers)
2. Backed by relevant Loaf skills if available, or prompt-only for domains without a matching skill
3. Given a perspective bias to ensure genuine diversity of viewpoint

Council size is **adaptive** — the Warden determines 3-7 members based on the question's breadth and complexity. Present the proposed composition to the user before spawning.

Sentinels (Reviewers) do NOT sit on councils — they come after decisions, not during. Rangers serve dual roles: as researchers they scout and report; at the council they advocate for the users they've been protecting. The Ranger's council voice is informed by their scouting — they're the defenders of the product who speak for the people.

### Ranger Seat (Product/UX Default)

The Warden includes **Rangers** by default when the question touches user experience, product direction, or feature scope. Rangers argue from the user's perspective — asking "should we?" not just "can we?" — informed by their knowledge of the people they protect.

**Skip condition:** When the question is purely technical, exploratory, and has no direct impact on UX or product direction (e.g., "which serialization format for internal RPC?"), the Warden may convene Smiths only. The Warden states this judgment when presenting the proposed composition — user can override.

### Uniform Question

Claude generates a **single, well-framed question** from the conversation context. Every council member receives the identical question along with the same background context. No member gets privileged information.

**Strategic context always provided:** Every council member receives VISION.md + STRATEGY.md + ARCHITECTURE.md alongside the framed question, regardless of the question's domain. This prevents "technically correct but strategically wrong" advice. Members choose how much weight to give strategic context, but they can't claim they didn't have it.

### Two-Round Deliberation

- **Round 1:** All members answer independently, in parallel. No cross-talk.
- **Round 2:** Each member sees all Round 1 positions and can revise their position, rebut others, or strengthen their argument. Run in parallel.

### Structured Per-Member Output

Each member produces:

| Field | Description |
|-------|-------------|
| **Take** | Adapts to question type: ✅/❌ for yes/no decisions, ranked pick for multi-option comparisons (e.g., "1st: X, 2nd: Y, avoid: Z"), position statement for open-ended questions. Always a forced commitment, no hedging. |
| **Confidence** | High / Medium / Low with brief justification |
| **Position** | Their recommended approach (1-2 sentences) |
| **Pros/Cons** | Per option under consideration |
| **Nuances** | Edge cases, hidden trade-offs, things others might miss |
| **Suggestions** | Alternative approaches or modifications |

Take + confidence are complementary: "My take is ❌ but I'm only Medium confident because..." or "1st: Postgres, 2nd: CockroachDB — High confidence" is more useful than either alone.

Output density is **adaptive** — concise (~200 words) for straightforward topics, detailed (~500 words) for complex ones. Claude judges based on question complexity.

### Advisory Output

The council produces:
1. The framed question
2. Each member's full structured response (Round 1 + Round 2 revisions)
3. A **convergence summary**: where members agree, where they diverge, and the key trade-offs
4. **No recommendation** — the council presents, the user decides

After the user has reviewed, suggest relevant next steps (ADR, implementation, further research) without auto-proceeding.

### Council File

Persist every council to `.agents/councils/` for future reference. Archive via `loaf task archive` conventions after the decision is captured.

## Scope

### In Scope

- Rewrite `council-session` SKILL.md with the advisory model
- Council composition with racial model (Smiths/Dwarves for technical, Rangers/Humans for product/UX)
- Two-round deliberation flow
- Structured per-member output format
- Updated council file template
- Model-invoked triggering (already done — `user-invocable: false`)

### Out of Scope

- CLI commands for council management
- Persistent council member profiles across sessions
- Voting or scoring mechanisms
- Changes to other skills or agents
- Council history search or analytics

### Rabbit Holes

- **Council member prompt engineering perfection** — good enough perspectives that produce diverse views are fine; don't over-optimize the persona prompts
- **Round 2 convergence loops** — two rounds max, no iterating until consensus
- **Skill matching heuristics** — simple keyword/domain matching is sufficient; don't build a recommendation engine

### No-Gos

- Council must never auto-decide or auto-implement
- No filtering or ranking member output before showing to user
- Don't require odd numbers of members (that's a voting artifact — we're not voting)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Round 2 adds too much latency | Medium | Medium | Round 2 agents run in parallel; total is ~2x single round |
| Members echo each other (low diversity) | Low | High | Racial perspective bias forces divergent viewpoints |
| Skill-backed members over-index on skill content | Low | Medium | Prompt instructs member to use skill as background, not as script |

## Open Questions

- [x] Should the convergence summary be generated by a separate agent or by the orchestrating Claude? → By the coordinator (main session). No separate agent needed.
- [x] How should the skill reference existing Loaf skills for council member backing — by name, by domain keyword, or by a mapping table? → By domain keyword matching. Simple, not a recommendation engine (per rabbit holes).

## Test Conditions

- [ ] "Call a council about X" triggers the skill from natural language
- [ ] Council composition (Smiths + Rangers) is proposed and shown before spawning
- [ ] Each member receives the identical framed question
- [ ] Round 1 responses are shown individually with full structured output
- [ ] Round 2 revisions are shown per-member
- [ ] Convergence summary identifies agreement and disagreement points
- [ ] No recommendation is made — user is asked to decide
- [ ] Council file is persisted to `.agents/councils/`
- [ ] Next steps are suggested but not auto-executed
- [ ] Rangers included when question touches UX or product direction
- [ ] Rangers omitted when question is purely technical (Smiths only) — Warden states this judgment
- [ ] User can override the product/UX seat decision
- [ ] Each member provides a take (✅/❌, ranked pick, or position) alongside confidence
- [ ] All council members receive VISION.md + STRATEGY.md + ARCHITECTURE.md context

## Priority Order

1. **Single-round advisory model** — core council flow with prompt-only members. Go/no-go: council produces actionable output.
2. **Round 2 deliberation** — can be dropped if single-round is sufficient.
3. **Skill-backed members** — follow-up if prompt-only members prove inadequate.
