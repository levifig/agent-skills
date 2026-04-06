---
id: SPEC-018
title: OpenCode Hooks Parity — verification
source: code-review
created: '2026-03-30T10:50:00.000Z'
status: complete
appetite: Micro (< 1 session)
---

# SPEC-018: OpenCode Hooks Parity

## Problem Statement

The OpenCode build target originally had four gaps surfaced during SPEC-014 review. Three have since been shipped incrementally via the unified runtime plugin generator (`cli/lib/build/lib/hooks/runtime-plugin.ts`):

1. ~~**Hook input mismatch**~~: **Shipped.** `serializeHookPayload()` creates JSON piped via `child.stdin.write(payload)` in `runHook()`. Env vars (`LOAF_HOOK_TYPE`, `LOAF_HOOK_ID`, `LOAF_TOOL_NAME`) are set as supplementary context.

2. ~~**No blocking capability**~~: **Shipped.** Exit code 2 triggers `throw new Error(result.stderr)` via `OpenCodePlatform.rejectPattern`. Confirmed correct for OpenCode's API — `tool.execute.before` blocks via thrown exceptions.

3. ~~**No plugin registration config**~~: **Not needed.** OpenCode auto-discovers `*.{ts,js}` in `~/.config/opencode/plugins/` (directory-based loading). No config file entry required. `loaf install --to opencode` already copies to this path.

4. ~~**No setup documentation**~~: **Not needed.** OpenCode reads `.agents/AGENTS.md` directly via its [rules system](https://opencode.ai/docs/rules/). No separate OPENCODE.md required.

## Solution Direction

### Verify export format — CONFIRMED

The generated plugin uses `export default async function AgentSkillsPlugin(...)`. OpenCode's `getLegacyPlugins()` in `packages/opencode/src/plugin/index.ts` iterates `Object.values(mod)` which includes the `default` export. The function check (`typeof value === "function"`) passes, and the plugin is called with `(input, options)` where `input` has `client` and `$` that our function destructures. No change needed.

### Verify TypeScript transpilation in global path — CONFIRMED

OpenCode's plugin loader uses `await import(row.entry)` in `packages/opencode/src/plugin/loader.ts`. Since OpenCode is a Bun binary (`$: Bun.$` in plugin context), `import()` natively handles `.ts` files regardless of file path. The global `~/.config/opencode/plugins/` path works identically to `.opencode/plugins/`. No change needed.

## Scope

### In Scope

- Verify `export default` works with OpenCode's plugin loader
- Verify `.ts` files in `~/.config/opencode/plugins/` are transpiled (or switch to `.js`)

### Out of Scope

- Changes to Cursor, Codex, Gemini, or Amp targets
- New hooks or hook functionality
- Runtime plugin code changes (already working)
- Rewriting hook scripts

### Rabbit Holes

- Don't add a config file generation step — directory-based loading is sufficient

## Dependencies

| Dependency | Type | Notes |
|---|---|---|
| SPEC-014 | Soft (upstream) | Profile model ships first; this addresses remaining verification items |

## Resolved Questions

- [x] **Does OpenCode's `tool.execute.before` support blocking?** Yes — via `throw new Error()`. Already implemented in `runtime-plugin.ts`. Two limitations: subagent calls bypass hooks (issue #5894), MCP calls bypass hooks (issue #2319).
- [x] **What is the current plugin registration mechanism?** Directory-based auto-loading from `~/.config/opencode/plugins/` and `.opencode/plugins/`. Config field is `plugin` (singular, array of strings) in `opencode.json`/`opencode.jsonc`, but not needed for directory-based loading.
- [x] **Should `loaf install` handle plugin registration automatically?** Already does — copies to `~/.config/opencode/plugins/` which is auto-discovered. Zero config edits needed.

## Test Conditions

- [x] `export default` plugin loads successfully in OpenCode — confirmed via source analysis of `getLegacyPlugins()` in `sst/opencode`
- [x] Plugin functions correctly from `~/.config/opencode/plugins/` path — Bun's native TS import handles `.ts` files in any location
