---
title: Consider leaving Claude Code plugin system — install via CLI like other harnesses
status: raw
created: 2026-04-10T19:21:00Z
tags: [architecture, plugins, install]
related: [SPEC-024]
---

# Leave Claude Code plugin system

## Problem

The plugin system has caching issues and limits agent/skill visibility. Agents defined in plugins may not be discoverable outside the plugin context (e.g., `--agent librarian` requires the plugin to be installed). Other harnesses (Cursor, OpenCode, Codex, Gemini, Amp) already install via CLI to standard paths.

## Opportunity

Unify the install model: `loaf install --to claude-code` would install to `.claude/` (project-level) or `~/.config/claude/` (user-level) just like it does for other harnesses. Agents, skills, and hooks would be in standard discovery paths, not locked inside a plugin cache.

## Connects to

- SPEC-024 (Harness-Native Surface Model) — already tracks the install convergence gap
- The `--agent` resolution path research from SPEC-029 — plugins → `.claude/agents/` → built-in

## Discovered

During SPEC-029 implementation — `--agent librarian` only resolves from the plugin install, creating a dependency on the plugin system for the enrichment feature.
