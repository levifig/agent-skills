# Loaf

An opinionated agentic framework that makes AI coding assistants structured, portable, and self-improving.

## Core Pillars

### Portable Knowledge

Write skills once, deploy to six harnesses. Skills are the universal knowledge layer that works everywhere. Profiles and hooks adapt per target. Better skill descriptions improve all targets simultaneously.

### Structured Execution

Every change flows through a deliberate pipeline: Idea, Spec, Tasks, Code, Learnings. No scope creep, no lost context, no "what were we doing?" Each phase has clear inputs, outputs, and quality gates.

### Bounded Autonomy

Functional profiles define what agents can mechanically touch (tool access). Skills define what they know (domain knowledge). The Warden coordinates but never implements. This separation makes agent behavior predictable and auditable.

### Session Continuity

Work survives context loss, compaction, tool restarts, and cross-conversation handoffs. Session journals are external memory. The pipeline's three-artifact model (spec, tasks, journal) means no single failure point can lose the thread.

## What Success Looks Like

A developer installs Loaf and immediately gets:

- **Consistent agent behavior across tools** -- same skills, same conventions, different runtimes
- **A pipeline that prevents scope creep** -- specs bound the work before code is written
- **Session history that enables handoff** -- pick up where you left off, or hand off to a colleague
- **Hooks that enforce quality without friction** -- secrets scanning, commit conventions, push guards
- **Domain expertise that loads automatically** -- the right engineering standards for the current task

## What Loaf Is Not

**Not a prompt library.** Loaf is a framework with mechanical enforcement (hooks, profiles, tool boundaries), not a collection of system prompts.

**Not Claude-only.** Multi-target by design. Claude Code is the primary development target, but skills are authored once and built for all supported harnesses.

**Not opinionated about what you build.** Opinionated about *how* you build it. The pipeline, conventions, and quality gates are fixed; the domain knowledge is yours.
