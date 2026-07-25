# Loaf

An opinionated agentic framework that makes AI coding assistants structured, portable, and self-improving.

## Core Pillars

### Portable Knowledge

Write skills once, deploy to supported harnesses. Skills are the universal knowledge layer that works everywhere. Profiles and hooks adapt per target. Better skill descriptions improve all targets simultaneously.

### Structured Execution

Ideas may be explored before `/shape` turns the chosen direction into a bounded Change. The Change carries the product and verification contract through implementation, review, and shipping; release remains a separate project-level operation.

### Bounded Autonomy

Functional profiles define what agents can mechanically touch (tool access). Skills define what they know (domain knowledge). The Warden coordinates but never implements. This separation makes agent behavior predictable and auditable.

### Continuity

Work survives context loss, compaction, tool restarts, and cross-conversation handoffs. The project journal is external memory, while Change artifacts and compatible task records keep intent and execution inspectable outside any one conversation.

## What Success Looks Like

A developer installs Loaf and immediately gets:

- **Consistent agent behavior across tools** -- same skills, same conventions, different runtimes
- **Bounded work that prevents scope creep** -- Changes define the intended outcome, boundaries, and proof before implementation
- **Project journal history that enables handoff** -- pick up where you left off, or hand off to a colleague
- **Hooks that enforce quality without friction** -- secrets scanning, commit conventions, push guards
- **Domain expertise that loads automatically** -- the right engineering standards for the current task

## What Loaf Is Not

**Not a prompt library.** Loaf is a framework with mechanical enforcement (hooks, profiles, tool boundaries), not a collection of system prompts.

**Not Claude-only.** Multi-target by design. Claude Code is the primary development target, but skills are authored once and built for all supported harnesses.

**Not opinionated about what you build.** Opinionated about *how* you build it. The Change contract, conventions, and quality checks are explicit; the domain knowledge is yours.
