# Loaf

_Last updated: 2026-08-29_

An opinionated agentic framework that makes AI coding assistants structured, portable, and self-improving.

## Core Pillars

### Portable Knowledge

Write skills once, deploy to supported harnesses. Skills are the universal knowledge layer that works everywhere. Profiles and hooks adapt per target. Better skill descriptions improve all targets simultaneously.

### Structured Execution

The Loaf Flow runs pitch → shape → implement → ship → release, at two scales. Skills and templates define the ceremonies; the connected issue tracker carries the shared work definition, definition of done, workflow state, hierarchy, assignment, and collaboration. Loaf does not create a second work record to coordinate the same work.

### Rideable Progress

Loaf evolves through complete, useful end-to-end operator journeys, not finished horizontal layers. Every milestone leaves a real rider able to do something they could not do before. A narrow increment keeps security, determinism, recovery, honest failure, and data safety intact; dogfood earns the next layer of breadth or abstraction. The operating method lives in [Rideable Increments](../content/skills/foundations/references/rideable-increments.md).

### Bounded Autonomy

Functional profiles define what agents can mechanically touch (tool access). Skills define what they know (domain knowledge). The Warden coordinates but never implements. This separation makes agent behavior predictable and auditable.

### Continuity

> **Revision 2026-08-26 (LOAF-90):** Continuity is personal-first and environment-agnostic. Supersedes: continuity described only as project-journal handoff on a single machine.

Loaf's durable memory — journal, wraps, sparks, ideas, decisions, explorations, findings, handoffs, opaque work refs, verification records, and derived context — lives in one operator's **personal memory substrate**. Every environment the operator uses (laptop, second machine, Cursor Cloud Agent, Amp Orb, CI) attaches to the same substrate or **hard-refuses** Loaf-flow work. There is no silent empty universe and no memoryless degraded mode. A never-attached trusted machine remains a complete local-first replica; attach is the multi-environment join, not a prerequisite for first use.

Work survives context loss, compaction, tool restarts, and cross-conversation handoffs because the substrate is external to any single harness session. Private memory retains the connective narrative and opaque references needed to resume; the tracker remains canonical for the shared work itself. Context at conversation start is a derived digest — never a lifecycle transition.

Scratchpad is deferred from the immediate vNext product boundary. A future conversation-oriented coordination surface may revisit it, but current Flow, continuity, milestones, and cutover do not depend on it.

Sharing with other humans happens only through explicit **promote surfaces** (PR bodies, committed files, external trackers). The substrate itself is private.

## What Success Looks Like

A developer installs Loaf and immediately gets:

- **Consistent agent behavior across tools** -- same skills, same conventions, different runtimes
- **Bounded work that prevents scope creep** -- tracker-native work items define the intended outcome, boundaries, and proof before implementation
- **Useful progress at every milestone** -- each increment completes a safe operator journey and produces real dogfood learning
- **Private continuity that enables resumption** -- the same operator can resume across agents, sessions, harnesses, and environments; colleagues receive work only through explicit tracker or Git promotion
- **Hooks that enforce quality without friction** -- secrets scanning, commit conventions, push guards
- **Domain expertise that loads automatically** -- the right engineering standards for the current task
- **Same project, any session, any harness, any host** — a cloud agent picks up the same private continuity and open-work references as the operator's laptop after attach
- **Fail-loud provisioning** — misconfigured credentials or unknown project identity refuse with cause and remedy; no quarantine buffer

## What Loaf Is Not

**Not a prompt library.** Loaf is a framework with mechanical enforcement (hooks, profiles, tool boundaries), not a collection of system prompts.

**Not harness-led.** Multi-target by design. Skills are authored once and adapted to each supported harness; no harness is the architectural authority for Loaf.

**Not opinionated about what you build.** Opinionated about *how* you build it. The Flow contract, conventions, and quality checks are explicit; the domain knowledge is yours.

**Not a team memory product.** Collaboration is deliberate export through trackers and git; the synced core is one operator's facts, E2E-encrypted on the wire.

## vNext Direction

vNext is a clean implementation under `vnext/`. The shipped runtime is behavioral evidence, a failure corpus, and a one-time migration source; vNext production packages cannot import it or use it as a runtime backend. vNext starts an independent schema identity at `vnext/1` rather than extending the legacy schema line.

At the kernel stage, the only command ceremonies are `version` and `ownership`. Later commands earn their place only when they perform a deterministic Loaf-owned operation. Tracker access remains a harness connection guided by skills and templates, never a Loaf provider client or synchronization layer.

Until cutover, the shipped line retains its existing contract: `/shape` turns a brief into a bounded Change. Change artifacts and compatible task records keep intent and execution inspectable. Changes define the intended outcome, boundaries, and proof before implementation. These compatibility statements describe the legacy runtime, not vNext authority.

## Product Boundary

| Layer | Owns |
|-------|------|
| Loaf | Flow ceremonies, skills, templates, profiles, project identity, private continuity, derived context, and private sync |
| Tracker | Work identity and definition, definition of done, workflow state, hierarchy, assignment, and collaboration |
| Git | Code and deliberately promoted artifacts |
| Harness | Execution, model selection, tool boundaries, service connections, and service credentials |

Each responsibility has one canonical owner. Crossing an authority boundary means using that owner's native surface, not mirroring it into Loaf.

---

## Changelog

- 2026-08-29 - Define the tracker-native vNext ownership and hard boundary, including private resumption, scratchpad scope, schema identity, and command surface.
