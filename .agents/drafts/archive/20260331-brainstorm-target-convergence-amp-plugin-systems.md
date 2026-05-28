---
title: "Brainstorm: Target Convergence, Amp Support & Plugin System Landscape"
type: brainstorm
created: 2026-03-31T00:00:00Z
status: active
tags: [targets, amp, opencode, codex, cursor, plugins, hooks, convergence]
related: [SPEC-020, SPEC-018]
---

# Brainstorm: Target Convergence, Amp Support & Plugin System Landscape

**Date:** 2026-03-31
**Session:** amp-target-exploration (Amp thread)

## Problem

Loaf targets 5 AI coding tools but duplicates significant build logic across them. Adding an Amp target exposed the duplication and prompted a broader investigation into how all tools' plugin/hook systems compare — revealing opportunities for convergence.

## Evidence Level

Use the sections below with three confidence buckets in mind:

- **Doc-confirmed** — Explicitly supported by current vendor docs or references linked at the bottom of this draft.
- **Observed in current Loaf** — Verified against the repository's current target implementations and output layout.
- **Inference / proposal** — A design conclusion drawn from the documented tool behavior plus Loaf's current architecture.

## Research: Plugin & Hook System Landscape

### Two meanings of "plugin"

The word "plugin" means fundamentally different things across tools:

1. **Distribution plugins** — Packaging bundles containing skills, agents, commands, hooks, MCP servers, rules, or apps. Users install via marketplace or plugin directory. (Claude Code, Codex, Cursor)
2. **Runtime plugins** — TypeScript code that intercepts events, modifies tool calls, runs shell scripts, and can register tools/commands in-process. (Amp, OpenCode)

### Distribution Plugin Comparison (Claude Code vs Codex vs Cursor)

These three are in the same family, but the late corrections matter: Codex plugins do **not** bundle hooks or agents, while Claude Code and Cursor do.

| Aspect | Claude Code | Codex | Cursor |
|---|---|---|---|
| **Manifest path** | `.claude-plugin/plugin.json` | `.codex-plugin/plugin.json` | `.cursor-plugin/plugin.json` |
| **Manifest fields** | Core metadata + component path overrides (`skills`, `agents`, `commands`, `hooks`, `mcpServers`, `lspServers`) | Core metadata + `homepage`, `keywords`, `interface`, app/MCP fields | Core metadata + component path overrides (`rules`, `skills`, `agents`, `commands`, `hooks`, `mcpServers`) |
| **Skills dir** | `skills/` | `skills/` (or custom via `"skills": "./skills/"`) | `skills/` |
| **Skill format** | `skills/{name}/SKILL.md` with frontmatter | `skills/{name}/SKILL.md` with frontmatter | `skills/{name}/SKILL.md` with frontmatter |
| **Agents dir** | `agents/` (Markdown) | ❌ Not in plugins (`.codex/agents/*.toml` standalone) | `agents/` (Markdown) |
| **Commands dir** | `commands/` | ❌ No commands | `commands/` |
| **Hooks bundled in plugin?** | ✅ `hooks/hooks.json` or inline `hooks` in `plugin.json` | ❌ Hooks stay standalone in `.codex/hooks.json` | ✅ `hooks/hooks.json` or inline `hooks` in `plugin.json` |
| **MCP servers** | `.mcp.json` or inline `mcpServers` | Bundled in plugin | `mcp.json` or inline `mcpServers` |
| **LSP servers** | `.lsp.json` or inline `lspServers` | ❌ | ❌ |
| **Rules** | ❌ | ❌ | `rules/*.mdc` |
| **Apps / connectors** | ❌ | `.app.json` (OAuth connectors) | ❌ |
| **Settings** | `settings.json` (agent defaults / user config support) | ❌ | ❌ |
| **Marketplace file** | `.claude-plugin/marketplace.json` | `.agents/plugins/marketplace.json` | `.cursor-plugin/marketplace.json` |
| **Cache path** | `~/.claude/plugins/cache/` | `~/.codex/plugins/cache/` | `~/.cursor/plugins/` |
| **Install command** | `/plugin install` | Plugin directory UI / `codex /plugins` | Marketplace panel |
| **Skill invocation** | `/plugin-name:skill` | `@plugin-name` | `/skill-name` |

### Shell Hook System Comparison (Claude Code vs Codex vs Cursor)

All three use JSON hook configs that launch shell commands, but the delivery boundary and event surface differ more than the early brainstorm assumed.

| Aspect | Claude Code | Codex | Cursor |
|---|---|---|---|
| **Config file** | `hooks/hooks.json` in plugin, or inline `hooks` in `plugin.json` | `.codex/hooks.json` alongside repo/user config | `hooks/hooks.json` in plugin, or inline `hooks` in `plugin.json` |
| **Events** | `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `SessionStart`, `Stop`, `UserPromptSubmit`, `SubagentStart/Stop`, `TaskCreated/Completed`, `PermissionRequest`, `Notification`, `StopFailure`, `TeammateIdle`, and more | `PreToolUse`, `PostToolUse`, `SessionStart`, `Stop`, `UserPromptSubmit` | `sessionStart`, `sessionEnd`, `preToolUse`, `postToolUse`, `postToolUseFailure`, `stop`, `subagentStart/Stop`, `before/afterShellExecution`, `before/afterMCPExecution`, `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `preCompact`, `afterAgentResponse`, `afterAgentThought`, `beforeTabFileRead`, `afterTabFileEdit` |
| **Matcher** | Tool name regex (all tools) | Tool name regex (currently only `Bash`); `SessionStart` matches `startup|resume` | Event-specific filters / matcher support |
| **Blocking** | Exit code + JSON output | Exit code `2` + `decision: "block"` / `permissionDecision: "deny"` | Exit code; `failClosed: true` available for stricter behavior |
| **Input delivery** | Env vars + stdin JSON | stdin JSON only | Hook event payload with hook-specific fields; some hooks support input/context mutation |
| **Prompt hooks** | ✅ `type: prompt` | ❌ | ❌ |
| **Timeout** | Configurable (ms) | Configurable (seconds) | Configurable (seconds) |

### Runtime Plugin Comparison (Amp vs OpenCode)

Both use Bun, both support TypeScript, and the core hook-running logic overlaps heavily.

| Aspect | Amp | OpenCode |
|---|---|---|
| **Plugin location** | `.amp/plugins/` | `.opencode/plugins/` |
| **Export pattern** | `export default function(amp: PluginAPI)` | `export const MyPlugin = async ({ client, $ }) => { return { ... } }` |
| **Pre-tool event** | `tool.call` | `tool.execute.before` |
| **Post-tool event** | `tool.result` | `tool.execute.after` |
| **Session start** | `session.start` | `session.created` |
| **Session end / idle** | `agent.end` | `session.idle` |
| **Agent start/end** | ✅ `agent.start`, `agent.end` | ❌ |
| **Compaction** | N/A (Amp doesn't compact) | ✅ `experimental.session.compacting` |
| **Block a tool** | `return { action: 'reject-and-continue', message }` | `throw new Error(msg)` |
| **Modify input** | `return { action: 'modify', input: {...} }` | Mutate `output.args` |
| **Custom tools** | ✅ `amp.registerTool()` with JSON Schema | ✅ `tool()` helper with Zod schema |
| **Custom commands** | ✅ `amp.registerCommand()` (command palette) | ❌ |
| **Shell access** | `ctx.$` (Bun shell) | `$` (Bun shell, from context) |
| **AI helpers** | `amp.ai.ask()` (yes/no/uncertain) | `client` SDK |
| **UI** | `ctx.ui.notify/confirm/input` | TUI events (`tui.toast.show`) |
| **File events** | Via `tool.result` (coarser) | `file.edited`, `file.watcher.updated` |
| **Runtime** | Bun | Bun |
| **Experimental?** | ✅ CLI-only, requires `PLUGINS=all` env var | Stable |
| **Activation header** | `// @i-know-the-amp-plugin-api-is-wip-and-very-experimental-right-now` | None |

### Full Feature Matrix (All 5 Tools)

| Capability | Claude Code | Amp | OpenCode | Codex | Cursor |
|---|---|---|---|---|---|
| **— Distribution / Packaging —** | | | | | |
| Plugin bundle for skills | ✅ `.claude-plugin/plugin.json` | ❌ | ❌ | ✅ `.codex-plugin/plugin.json` | ✅ `.cursor-plugin/plugin.json` |
| Plugin bundle for hooks | ✅ | ❌ | ❌ | ❌ (`.codex/hooks.json` is standalone) | ✅ |
| Plugin bundle for agents | ✅ | ❌ | ❌ | ❌ (standalone `.codex/agents/*.toml`) | ✅ |
| Marketplace / plugin directory | ✅ Official + custom | ❌ | ❌ (npm ecosystem) | ✅ In-app directory | ✅ Cursor Marketplace |
| **— Runtime Plugin —** | | | | | |
| Event-driven TS/JS hooks | ❌ Shell scripts only | ✅ `amp.on(...)` | ✅ Named export hooks | ❌ Shell scripts only | ❌ Shell scripts only |
| Runtime | N/A (shell) | Bun | Bun | N/A (shell) | N/A (shell) |
| Custom tools (code) | ❌ (via MCP) | ✅ `registerTool()` | ✅ `tool()` helper | ❌ (via MCP) | ❌ (via MCP) |
| Custom commands (code) | ❌ (via Markdown) | ✅ `registerCommand()` | ❌ | ❌ | ❌ |
| **— Hook System —** | | | | | |
| Pre-tool | ✅ `PreToolUse` | ✅ `tool.call` | ✅ `tool.execute.before` | ✅ `PreToolUse` (Bash only today) | ✅ `preToolUse`, `beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile` |
| Post-tool | ✅ `PostToolUse`, `PostToolUseFailure` | ✅ `tool.result` | ✅ `tool.execute.after` | ✅ `PostToolUse` (Bash only today) | ✅ `postToolUse`, `postToolUseFailure`, `afterShellExecution`, `afterMCPExecution`, `afterFileEdit` |
| Session lifecycle | ✅ `SessionStart`, `Stop` | ✅ `session.start`, `agent.end` | ✅ `session.created`, `session.idle` | ✅ `SessionStart`, `Stop` | ✅ `sessionStart`, `sessionEnd`, `stop`, `preCompact` |
| Agent lifecycle | ✅ `SubagentStart/Stop` | ✅ `agent.start/end` | ❌ | ❌ | ✅ `subagentStart/Stop`, `afterAgentResponse/Thought` |
| Blocking | ✅ Exit code + structured JSON | ✅ `{ action: 'reject-and-continue' }` | ✅ `throw Error` | ✅ Exit code `2` + JSON block shapes | ✅ Exit code, `failClosed: true` |
| Prompt hooks | ✅ `type: prompt` | ❌ | ❌ | ❌ | ❌ |
| **— Skills —** | | | | | |
| Format | `SKILL.md` + frontmatter | `SKILL.md` + frontmatter | `SKILL.md` + frontmatter | `SKILL.md` + frontmatter | `SKILL.md` + frontmatter |
| Shared `.agents/skills/` discovery | — | ✅ `.agents/skills/`, `~/.config/agents/skills/` | ❌ | ✅ `.agents/skills/`, `~/.agents/skills/` | ✅ `.agents/skills/` plus `.cursor/.claude/.codex` skill dirs |
| User-invocable commands | ✅ `/plugin:skill` | ❌ (model auto-loads) | ✅ Commands from sidecar | ✅ `@plugin` | ✅ `/skill-name` |
| **— Agents —** | | | | | |
| Agent profiles | ✅ `agents/*.md` (Markdown) | ❌ (native agents) | ✅ `agents/*.md` (Markdown) | ✅ `.codex/agents/*.toml` (TOML, not in plugins) | ✅ `agents/*.md` (Markdown) |

### Subagent Format Comparison

| Aspect | Claude Code | Codex | Cursor | Amp | OpenCode |
|---|---|---|---|---|---|
| **Agent format** | `agents/*.md` (Markdown + YAML frontmatter) | `.codex/agents/*.toml` (TOML config) | `agents/*.md` (Markdown + YAML frontmatter) | ❌ (native agents) | `agents/*.md` (Markdown) |
| **Key fields** | `name`, `description`, `model`, `tools`, `disallowedTools`, `skills`, `effort`, `maxTurns`, `isolation` | `name`, `description`, `developer_instructions`, `model`, `model_reasoning_effort`, `sandbox_mode`, `mcp_servers`, `skills.config` | `name`, `description`, `model`, `is_background` | — | `name`, `description` |
| **Instructions** | Markdown body (system prompt) | `developer_instructions` string in TOML | Markdown body (system prompt) | — | Markdown body |
| **Tool scoping** | `tools` / `disallowedTools` | `sandbox_mode` (read-only, workspace-write) | Limited | — | — |
| **Built-in agents** | Explore, Plan, general-purpose | `default`, `worker`, `explorer` | N/A | Task, oracle, librarian, etc. | N/A |
| **Nickname system** | ❌ | ✅ `nickname_candidates` | ❌ | ❌ | ❌ |
| **CSV batch spawn** | ❌ | ✅ `spawn_agents_on_csv` (experimental) | ❌ | ❌ | ❌ |
| **Bundled in plugins?** | ✅ `agents/` in plugin | ❌ Standalone in `.codex/agents/` | ✅ `agents/` in plugin | — | Flat in `agents/` |

**Implication for Loaf:** Codex agents use TOML, not Markdown. To support Codex agents, the build system would need a Markdown→TOML transformer. Since Codex agents cannot be bundled in plugins anyway, this is a separate install concern.

### Skills as Commands Comparison

| Feature | Claude Code | Amp | OpenCode | Codex | Cursor |
|---|---|---|---|---|---|
| **Skills as user commands** | ✅ Slash commands (`/loaf:implement`) | ❌ No slash commands from skills | ✅ Commands generated from sidecar | ✅ `@plugin` invocation | ✅ `/skill-name` |
| **How user invokes** | `/skill-name` in chat | Describe the task; model auto-loads via `skill` tool | `/command-name` in chat | `@plugin-name` mention | `/skill-name` or Agent Decides |
| **How model invokes** | Model calls `skill` tool | Model calls `skill` tool | Model loads skill on match | Model loads skill on match | Model loads based on mode (Always/Agent Decides/Manual) |
| **Controlled by** | `user-invocable` in sidecar | Always model-driven | Presence of `SKILL.opencode.yaml` sidecar | Skill `description` | Rules mode setting |

### Shared Skill Discovery Paths (Amp / Codex / Cursor)

The late `.agents/skills/` finding changes the architecture more than any single hook detail.

| Tool | Project-level skill paths | User-level skill paths | Implication for Loaf |
|---|---|---|---|
| Amp | `.agents/skills/` | `~/.config/agents/skills/` | Shared project artifact works; Amp-specific skill build is optional for install |
| Codex | `.agents/skills/` | `~/.agents/skills/` | Same shared project artifact works; hooks and agents still need Codex-specific handling |
| Cursor | `.agents/skills/`, `.cursor/skills/`, `.claude/skills/`, `.codex/skills/` | `~/.cursor/skills/`, `~/.claude/skills/`, `~/.codex/skills/` | Cursor can consume the shared artifact directly, plus tool-specific fallbacks |

**Implication:** skills should be treated as a canonical content artifact with multiple delivery paths, not as five independent per-target builds.

## Current Loaf vs Target State

| Area | Current Loaf behavior | Desired behavior | Gap severity | Covered by SPEC-020? |
|---|---|---|---|---|
| Shared skill packaging | `copySkills()` duplicated across 5 targets | Single shared `skills.ts` packaging path | High | ✅ |
| Shared agent packaging | `copyAgents()` duplicated across 3 targets | Single shared `agents.ts` for Markdown-based targets | Medium | ✅ |
| Command substitution | `substituteCommands()` duplicated per target | Shared command map + target-specific overrides | Medium | ✅ |
| Amp runtime target | No Amp target exists | `dist/amp/` with shared skills + runtime plugin | High | ✅ |
| OpenCode runtime generation | OpenCode-only inline generator | Shared runtime plugin generator for OpenCode + Amp | High | ✅ |
| Codex shell hooks | Codex target does not emit hooks | Generate/install `.codex/hooks.json` and scripts | High | ✅ |
| Codex agents | No TOML agent support | Future Markdown→TOML transformer and install path | Medium | ❌ |
| Cursor advanced hook parity | Only a subset of Cursor's hook model is reflected in Loaf | Preserve current behavior now; evaluate broader parity later | Medium | Partial |
| Shared `.agents/skills/` install artifact | Skills are treated mostly as per-target outputs | First-class shared skill artifact or converged install story | High | ✅ |
| Distribution repo strategy | Discussed, not concretized | Clear marketplace/distribution approach for CC/Codex/Cursor | Low | Partial |

## Directions Explored

### Direction 1: Shared content layer + runtime plugin convergence

Extract duplicated `copySkills()` (5×), `copyAgents()` (3×), and `substituteCommands()` (5×) into shared modules. Create a shared runtime plugin generator for OpenCode + Amp. Add Amp target.

- **Gains:** ~230 duplicate lines eliminated. Adding new targets becomes easier. Amp support.
- **Risks:** Abstraction may be premature for shell hooks, especially after the Cursor and Codex corrections.
- **Decision:** Still the strongest immediate move. Pursued as SPEC-020.

### Direction 2: Distribution repo (levifig/plugins)

Compiled output for the distribution-plugin family (Claude Code, Codex, Cursor) goes to a separate repo. Users install via each tool's native marketplace or plugin system.

- **Gains:** Clean separation of source vs distribution. Single release point for marketplace bundles.
- **Not replacing:** Shared skill content, runtime plugins, or standalone Codex hooks.

### Direction 3: Layered architecture (replaces the earlier three-tier view)

The earlier three-tier model still describes hook delivery families, but the late `.agents/skills/` discovery means it is no longer the right top-level architecture.

**Layer 1: Canonical content artifacts**
- Skills are built once as shared content (`.agents/skills/` or an equivalent staging artifact)
- Agents remain canonical Markdown in source
- Command placeholder substitution becomes a shared content transform, not a target identity

**Layer 2: Delivery adapters**
- Runtime plugin adapter for Amp + OpenCode
- Shell hook / config adapters for Claude Code, Codex, and Cursor
- Agent adapter layer: Markdown passthrough for Claude/Cursor/OpenCode, future Markdown→TOML for Codex if desired

**Layer 3: Distribution / install packaging**
- Claude Code / Cursor / Codex marketplace bundles
- Standalone Codex hook config (`.codex/hooks.json`)
- Project/user installs to shared skill paths

This keeps the useful convergence work while acknowledging that Cursor's richer hook system and Codex's standalone hook/agent files make a single shell-hook abstraction premature.

## Proposed File Structure

```
cli/lib/build/
├── types.ts                        # Existing (unchanged)
├── lib/
│   ├── sidecar.ts                  # Existing (unchanged)
│   ├── version.ts                  # Existing (unchanged)
│   ├── shared-templates.ts         # Existing (unchanged)
│   ├── copy-utils.ts               # Existing (unchanged)
│   ├── skills.ts                   # NEW: Extract from duplicate copySkills()
│   ├── agents.ts                   # NEW: Extract from duplicate copyAgents()
│   ├── commands.ts                 # NEW: Configurable command substitution
│   └── hooks/
│       └── runtime-plugin.ts       # NEW: TS runtime plugin generation (OpenCode + Amp)
└── targets/
    ├── claude-code.ts              # Shared content + Claude plugin packaging
    ├── opencode.ts                 # Shared content + runtime wrapper
    ├── amp.ts                      # NEW: shared skills + runtime wrapper
    ├── cursor.ts                   # Shared content + Cursor plugin packaging
    ├── codex.ts                    # Shared skills + standalone hooks + plugin metadata
    └── gemini.ts                   # Shared skills only
```

## Distribution Architecture

```
Loaf Source (levifig/loaf)
  │
  ├─ canonical skill build ───────────────────────────────► shared skill artifact
  │                                                          (`.agents/skills/` or staging dir)
  │                                                          ├── used directly by Amp installs
  │                                                          ├── used directly by Codex installs
  │                                                          └── used directly by Cursor installs
  │
  ├─ runtime plugin adapters ─────────────────────────────► dist/opencode/plugins/hooks.ts
  │                                                       └► dist/amp/plugins/loaf.ts
  │
  └─ distribution / hook packaging ──────────────────────► plugins/loaf/
                                                           ├► dist/cursor/ (+ hooks/hooks.json)
                                                           └► dist/codex/ + .codex/hooks.json
```

## Key Insights

1. **"Plugin" means two different things.** Distribution packaging (Claude Code, Codex, Cursor) vs runtime event interception (Amp, OpenCode). Don't conflate them.

2. **The three distribution plugin systems are similar, not identical.** The manifest and skill formats line up well, but Codex keeps hooks and agents outside the plugin bundle.

3. **Amp and OpenCode runtime plugins share ~90% logic.** Both use Bun, both TypeScript, both have pre/post tool interception. Only event names and response patterns differ materially.

4. **Codex has shell hooks, but they are standalone.** The missing artifact is `.codex/hooks.json`, not a plugin-bundled hook section.

5. **Amp's plugin API is experimental.** CLI-only, requires `PLUGINS=all` env var. Worth building for, but not critical path.

6. **Amp does not need agent profiles.** Its native agent system handles that. Skills + runtime plugin is sufficient.

7. **Amp auto-discovers skills.** No user-invocable commands are required. The model loads skills via the `skill` tool based on `name` + `description`.

8. **The Amp skill format is identical to Claude Code's.** Same `SKILL.md` with `name` / `description` frontmatter. No Amp sidecar is needed unless tool-specific overrides emerge.

9. **`.agents/skills/` is the universal skill path.** A single skill artifact can serve Cursor, Codex, and Amp without separate skill builds.

10. **Cursor's hook surface is much richer than first assumed.** It is in the same family as Claude/Codex shell hooks, but not a shallow variant. Treat shared shell-hook generation cautiously.

11. **Codex agent support is a separate problem.** TOML agents and standalone install paths mean agent convergence is not the same task as skill/hook convergence.

## Open Questions

- Should the shared skill artifact be materialized directly in `dist/` (or `.agents/skills/`) or only converge at install time?
- Should the distribution repo be a monorepo with all three plugin targets, or separate repos per tool?
- Should `loaf build` output directly to the distribution repo, or keep `dist/` and copy on release?
- Should Codex hook output live beside plugin packaging as `.codex/hooks.json`, or be treated as a separate install artifact?
- How should Codex's `interface` block (UI metadata, screenshots, branding) be handled — generated or manual?
- Is there enough real commonality to extract shell-hook helpers for Claude/Cursor/Codex, or should runtime plugin generation be the only shared hook abstraction for now?

## Late-Breaking Findings

### Cursor has 19 hook events (not ~5)

The Cursor hook system is far richer than the current Loaf target generates. Key events beyond the basics:

| Hook | Purpose | Unique to Cursor? |
|---|---|---|
| `beforeShellExecution` / `afterShellExecution` | Shell-specific pre/post | ✅ (Claude Code uses generic `PreToolUse`) |
| `beforeMCPExecution` / `afterMCPExecution` | MCP tool pre/post | ✅ |
| `beforeReadFile` / `afterFileEdit` | File-level hooks | ✅ |
| `afterAgentResponse` / `afterAgentThought` | Track agent output | ✅ |
| `beforeTabFileRead` / `afterTabFileEdit` | Tab completion hooks | ✅ |
| `postToolUseFailure` | Tool failure hook | Shared with Claude Code |
| `subagentStart` / `subagentStop` | Subagent lifecycle | Shared with Claude Code |
| `preCompact` | Context compaction | Observational only |

Additional Cursor hook features:
- **`failClosed: true`** option — block on hook failure (default is fail-open)
- **`loop_limit`** on stop hooks — limits auto-continue loops
- **`updated_input`** on pre-tool hooks — can modify tool input before execution
- **`additional_context`** on post-tool hooks — inject context after tool result
- **`env`** on `sessionStart` — set env vars for the session
- **Enterprise / Team / Project / User priority** — MDM-managed hooks for enterprise

### Cursor reads Claude/Codex directories natively

Cursor loads skills and agents from all of these:
- `.agents/skills/`, `.cursor/skills/`, `.claude/skills/`, `.codex/skills/`
- `.cursor/agents/`, `.claude/agents/`, `.codex/agents/`
- Plus user-level equivalents (`~/.cursor/`, `~/.claude/`, `~/.codex/`)

**Implication:** a single `.agents/skills/` output can serve Cursor, Codex, and Amp simultaneously. The distribution plugin is only needed for hooks, MCP, marketplace metadata, and tool-specific packaging.

### Cursor skill frontmatter differences

- `disable-model-invocation: true` = Claude Code's `user-invocable: false` (inverted boolean logic)
- No `allowed-tools` equivalent — Cursor uses subagent `readonly` for tool restriction
- Skills appear in Cursor Settings → Rules → "Agent Decides" section

### Codex subagents use TOML, not Markdown

Codex agents use `.codex/agents/*.toml` with fields such as `name`, `description`, `developer_instructions`, `model`, `model_reasoning_effort`, `sandbox_mode`, `mcp_servers`, and `skills.config`. Markdown body becomes a TOML string field (`developer_instructions`). They are not bundleable in plugins.

## Additional Notes (from Codex best practices)

- **Codex skill install path is `$HOME/.agents/skills` and `.agents/skills`** — same project-level convention as Amp. This simplifies `loaf install` if skills are treated as a shared artifact.
- **Codex uses TOML broadly** — config (`config.toml`), agents (`.toml`), feature flags. If Loaf ever generates Codex-specific config, it should be TOML.
- **Codex has automations** — scheduled recurring prompts (cron-like). No equivalent in the other four tools. Not a Loaf target concern right now.
- **Codex has compaction** (`/compact`) — unlike Amp which uses handoff. Relevant if hook/session mapping ever expands.
- **Codex built-in scaffolding skills** — `$skill-creator`, `$plugin-creator` (dollar-sign prefix convention for built-in skills).

## References

- Amp Plugin API: https://ampcode.com/manual/plugin-api
- Amp Skills/AGENTS.md: https://ampcode.com/manual (Agent Skills section)
- OpenCode Plugins: https://opencode.ai/docs/plugins/
- Codex Plugins: https://developers.openai.com/codex/plugins
- Codex Build Plugins: https://developers.openai.com/codex/plugins/build
- Codex Hooks: https://developers.openai.com/codex/hooks/
- Cursor Plugins: https://cursor.com/docs/plugins
- Cursor Plugins Reference: https://cursor.com/docs/reference/plugins
- Claude Code Plugins: https://code.claude.com/docs/en/plugins
- Claude Code Plugins Reference: https://code.claude.com/docs/en/plugins-reference
- Agent Skills Spec: https://agentskills.io/specification

## Changelog

- 2026-03-31 01:19- Added evidence legend, current-vs-target matrix, and execution-oriented clarification sections.
