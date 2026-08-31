# Loaf

An opinionated agentic framework that makes AI coding assistants structured, portable, and self-improving.

## Core Pillars

### Portable Knowledge

Write skills once, deploy to supported harnesses. Skills are the universal knowledge layer that works everywhere. Profiles and hooks adapt per target. Better skill descriptions improve all targets simultaneously.

### Structured Execution

The Loaf Flow runs pitch → shape → implement → ship → release, at two scales. `/pitch` grills a problem into an authored brief: a change-scale `brief.md` on an existing project, or a new project's `BRIEF.md` that bootstrap turns into operating documents and a captured initial arc. `/shape` turns a brief into a bounded Change, which carries the product and verification contract through implementation, review, and shipping; release remains a separate project-level operation.

### Rideable Progress

Loaf evolves through complete, useful end-to-end operator journeys, not finished horizontal layers. Every milestone leaves a real rider able to do something they could not do before. A narrow increment keeps security, determinism, recovery, honest failure, and data safety intact; dogfood earns the next layer of breadth or abstraction. The operating method lives in [Rideable Increments](../content/skills/foundations/references/rideable-increments.md).

### Bounded Autonomy

Functional profiles define what agents can mechanically touch (tool access). Skills define what they know (domain knowledge). The Warden coordinates but never implements. This separation makes agent behavior predictable and auditable.

### Continuity

> **Revision 2026-08-26 (LOAF-90):** Continuity is personal-first and environment-agnostic. Supersedes: continuity described only as project-journal handoff on a single machine.

Loaf's durable memory — journal, wraps, handoffs, decisions, work-unit refs, verification records — lives in one operator's **personal memory substrate**. Every environment the operator uses (laptop, second machine, Cursor Cloud Agent, Amp Orb, CI) attaches to the same substrate or **hard-refuses** Loaf-flow work. There is no silent empty universe and no memoryless degraded mode. A never-attached trusted machine remains a complete local-first replica; attach is the multi-environment join, not a prerequisite for first use.

Work survives context loss, compaction, tool restarts, and cross-conversation handoffs because the substrate is external to any single harness session. The project journal is a **projection** of grow-only ledger facts, not a hand-authored file. Change artifacts and compatible task records keep intent and execution inspectable outside any one conversation. Context at conversation start is a derived digest — never a lifecycle transition.

Sharing with other humans happens only through explicit **promote surfaces** (PR bodies, committed files, external trackers). The substrate itself is private.

## What Success Looks Like

A developer installs Loaf and immediately gets:

- **Consistent agent behavior across tools** -- same skills, same conventions, different runtimes
- **Bounded work that prevents scope creep** -- Changes define the intended outcome, boundaries, and proof before implementation
- **Useful progress at every milestone** -- each increment completes a safe operator journey and produces real dogfood learning
- **Project journal history that enables handoff** -- pick up where you left off, or hand off to a colleague
- **Hooks that enforce quality without friction** -- secrets scanning, commit conventions, push guards
- **Domain expertise that loads automatically** -- the right engineering standards for the current task
- **Same project, any session, any harness, any host** — a cloud agent picks up the same journal, wraps, and open work definitions as the operator's laptop after attach
- **Fail-loud provisioning** — misconfigured credentials or unknown project identity refuse with cause and remedy; no quarantine buffer

## What Loaf Is Not

**Not a prompt library.** Loaf is a framework with mechanical enforcement (hooks, profiles, tool boundaries), not a collection of system prompts.

**Not Claude-only.** Multi-target by design. Claude Code is the primary development target, but skills are authored once and built for all supported harnesses.

**Not opinionated about what you build.** Opinionated about *how* you build it. The Change contract, conventions, and quality checks are explicit; the domain knowledge is yours.

**Not a team memory product.** Collaboration is deliberate export through trackers and git; the synced core is one operator's facts, E2E-encrypted on the wire.

## Product boundary

| Layer | Owns |
|-------|------|
| Trackers (Linear v1) | Work definition, DoD text, workflow state |
| Git | Code and promoted artifacts |
| Harnesses | Execution, tool boundaries |
| Loaf substrate | Context, journal, wraps, handoffs, decisions, continuity digest, ref mappings |
