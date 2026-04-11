---
title: Configure Warden (Arandil) as the default --agent for Claude Code
status: raw
created: 2026-04-11T02:00:00Z
tags: [architecture, agents, warden, claude-code]
related: [SOUL.md, content/agents/]
---

# Warden as default --agent

## Idea

Configure the local `claude` command to run with `--agent warden` (or `--agent arandil`) by default. Move the Warden/Arandil identity from `SOUL.md` (which is a system prompt template) to a proper top-level agent definition that replaces Claude Code's default behavior.

## Why

Today the Warden identity is injected via `SOUL.md` as a system prompt addition. But `--agent` is the proper mechanism for giving a session a persistent identity with behavioral contracts, tool boundaries, and model selection. Making the Warden a real agent means:

- The orchestrator identity is mechanically enforced, not just prompted
- `claude agents` lists it alongside the other profiles
- Other Loaf users can customize or replace the default agent
- `--agent` settings can be configured in `.claude/settings.json` so every session starts as the Warden without manual flags

## How

1. Create `content/agents/warden.md` (or `arandil.md`) — the Warden agent definition with SOUL.md content as the system prompt
2. Configure `settings.json` with `"agent": "warden"` as the default
3. `loaf install --to claude-code` sets this up
4. SOUL.md becomes the template for the agent definition, not a standalone file
