---
title: "Loaf as Standalone Harness — extract the method, own the runtime"
status: raw
created: 2026-03-15T01:40:55Z
updated: 2026-03-15T01:45:00Z
tags: [harness, mcp, agent-sdk, tui, gui, interactive, conductor, soloterm, standalone]
related:
  - .agents/specs/SPEC-008-loaf-cli.md
  - docs/knowledge/agent-harness-vision.md
  - .agents/drafts/brainstorm-loaf-cli-knowledge-harness.md
---

# Loaf as Standalone Harness

## Core Insight

Loaf's skills, hooks, and Shape Up workflow ARE the method for harnessing agentic development. Right now that method is distributed as plugins into other tools (Claude Code, Cursor, etc.). The idea: extract the method itself and make Loaf the harness — either attaching to existing tools OR running standalone.

## Two Modes

**Attach mode** (what exists today, evolved): Loaf distributes skills + hooks + MCP into Claude Code / Cursor / Codex / OpenCode. The tools are the runtime, Loaf is the brain.

**Standalone mode** (the new idea): Loaf IS the runtime. A TUI and/or native macOS GUI that handles the harness internally. It triggers Claude Code CLI / Codex CLI / Cursor CLI / OpenCode via their SDKs or piped commands. The user interacts with Loaf, not with the underlying tools directly.

## Key Shift

Today: Loaf = skills + hooks distributed INTO other tools.
Future: Loaf = the opinionated harness. Other tools = execution backends.

Users bring their own skills (Loaf provides the harness, not necessarily all the content). The core Loaf skills (orchestration, implement, shape, research, etc.) ship as defaults, but the framework is extensible.

## Raw Notes

- MCP server as the integration layer (Loaf exposes tools to agents)
- Agent SDKs (Claude Code SDK, Codex SDK) as the execution backends
- TUI (SoloTerm-style) and/or native macOS GUI as the control surface
- Skills become pluggable capabilities, knowledge becomes context
- Subsumes `/implement`, `/shape`, `/research` into interactive workflows
- Interactive mode: watch agents work, steer, approve in real-time
- Batch mode: overnight implementation loop (Phase 5 vision)
- The harness is opinionated; the skills are swappable
- Think: "what if Loaf was the IDE's agent layer, not a plugin for it?"

## References

- [conductor.build](https://conductor.build) — visual agent orchestration
- [soloterm.com](https://soloterm.com) — terminal-native AI interface
- [ampcode.com](https://ampcode.com) — agents as topical entities (Oracle, Librarian, Painter, etc. — see [subagents](https://ampcode.com/manual#subagents)). Contrast with Loaf's current role-based agents (backend-dev, dba, qa). Topical agents are capability-shaped, not team-shaped — worth exploring for standalone mode.
