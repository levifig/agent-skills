---
id: SPEC-026
title: "Instruction-file hooks with native if-filtering"
source: "direct — SPEC-020 incomplete migration, json-parser.sh deletion broke 3 advisory hooks"
created: 2026-04-06T17:25:27Z
status: complete
---

# SPEC-026: Instruction-File Hooks with Native If-Filtering

## Problem Statement

Three advisory hooks (`workflow-pre-merge`, `workflow-pre-push`, `workflow-post-merge`) error on every Bash tool call. They source `content/hooks/lib/json-parser.sh`, which was deleted during SPEC-020's shared-library elimination. SPEC-020's risk matrix flagged this: "Remaining hooks must be audited for library dependencies" — but the audit missed these three.

The hooks parse stdin JSON to check if the command matches a pattern (`gh pr merge`, `git push`), then output an instruction file. This reimplements what Claude Code/Cursor's native `if` conditions and OpenCode's TS `matchesIfCondition()` already do.

## Strategic Alignment

- **Vision:** Hooks should leverage harness-native capabilities, not reimplement them in bash.
- **Architecture:** Introduces `instruction:` field — a new hook dispatch mechanism for hooks that output static content gated by command pattern.

## Solution Direction

Add an `instruction:` field to hooks.yaml for hooks whose sole job is outputting a static file when a command pattern matches. Each target builder resolves the file path per-target and generates the appropriate `cat` command. The `if:` condition handles filtering natively — no JSON parsing, no bash scripts.

## Scope

### In Scope

- New `instruction:` field in hooks.yaml schema and `HookDefinition` type
- Convert 3 hooks from `script:` to `instruction:` + `if:`
- Update Claude Code, Cursor, and OpenCode target builders to generate `cat` commands from `instruction:` field
- Delete 3 broken bash scripts and the prematurely restored `json-parser.sh`
- Revert the `lib/` copy addition in claude-code.ts (from earlier this session)

### Out of Scope

- TS-first hook authoring (separate future work, logged as spark)
- Converting other script hooks (kb-staleness-nudge, compact.sh)
- Changes to instruction file content

### Rabbit Holes

- **Generalizing instruction: to all hooks** — Only these three need it. Don't redesign the hook system.
- **OpenCode path resolution complexity** — The TS runtime already resolves `__dirname`; keep it simple.

### No-Gos

- Don't use `type: prompt` with `if:` (prompt+if doesn't pre-filter on Claude Code)
- Don't inline instruction content into hooks.yaml (files are 5-58 lines)

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Cursor `if` behavior differs from CC | Low | Med | Test with Cursor if available; fallback is script approach for Cursor only |
| OpenCode command path resolution | Low | Low | `__dirname` + relative path is well-established pattern in the runtime plugin |

## Open Questions

None — design decided (instruction: field), all three targets audited.

## Test Conditions

- [ ] `loaf build` succeeds
- [ ] No `PreToolUse:Bash hook error` or `PostToolUse:Bash hook error` in Claude Code
- [ ] `gh pr merge` shows pre-merge instruction content
- [ ] `git push` shows pre-push reminders
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes
- [ ] No `json-parser.sh` references remain in source (`content/`, `cli/`)

## Circuit Breaker

If the `instruction:` field proves complex to thread through all three builders, fall back to hardcoded `cat` commands per-hook in each builder (less elegant, still eliminates the scripts).

## Files to Modify

| File | Change |
|------|--------|
| `config/hooks.yaml` | Convert 3 hooks: `script:` → `instruction:` + `if:` |
| `cli/lib/build/types.ts` | Add `instruction?:` to `HookDefinition` |
| `cli/lib/build/targets/claude-code.ts` | Handle `instruction:` → `cat "${CLAUDE_PLUGIN_ROOT}/hooks/<path>"`. Revert lib/ copy. |
| `cli/lib/build/targets/cursor.ts` | Handle `instruction:` → `cat "$HOME/.cursor/hooks/<path>"` |
| `cli/lib/build/targets/opencode.ts` | Handle `instruction:` → command with resolved path in TS runtime |
| `content/hooks/pre-tool/workflow-pre-merge.sh` | Delete |
| `content/hooks/pre-tool/workflow-pre-push.sh` | Delete |
| `content/hooks/post-tool/workflow-post-merge.sh` | Delete |
| `content/hooks/lib/json-parser.sh` | Delete (revert premature creation) |
