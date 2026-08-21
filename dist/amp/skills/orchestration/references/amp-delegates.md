# Amp Implementation and Review Delegates

## Contents

- Scope
- Default Policy
- Isolation and Workdir
- Caller-Prepared Review Snapshots
- Capability Preflight
- Migration from the Standalone Prototype
- Anti-Patterns

Amp-only operating guidance for Loaf's generated `.amp/plugins/loaf.ts` plugin. Other harnesses ignore this file except as cross-product documentation.

## Scope

Loaf's Amp target generates one managed `loaf.ts` plugin: enforcement hooks plus two selectable orchestrator modes. Grok, Luna, and the pinned Sol oracle are callable tools, not selectable picker modes.

| Surface | Model | Reasoning | Role |
|---------|-------|-----------|------|
| `loaf-medium` | `openai/gpt-5.6-sol` | `medium` | Everyday orchestrator. Plans and decides; does not implement substantial code. |
| `loaf-ultra` | `openai/gpt-5.6-sol` | `xhigh` | Hard-problem orchestrator on the Ultra harness. |
| `delegate_grok_implementation` | `xai/grok-4.6` | `high` + Fast | Finite local coding allowlist: Read, finder, shell_command, shell_command_status, apply_patch, view_media |
| `delegate_luna_review` | `openai/gpt-5.6-luna` | `xhigh` | Read/search only: Read, finder |
| `consult_oracle` | `openai/gpt-5.6-sol` | `high` | Read/search only. Prefer this over Amp's built-in `oracle` tool, which cannot be model-pinned on custom agents. |

Grok never receives `include: "all"`, MCP/remote tools, web research, librarian, skill loading, or other expanding capabilities. Luna and the pinned oracle never receive shell, patch, Git, project-state, or external-state tools. Delegates run locally with `parentThreadID` provenance and prohibit commit, push, merge, history rewrite, and shared external mutations.

This is Amp-only. It is not a generic model router and does not change Claude Code, Codex, Cursor, or OpenCode behavior. The file is copied with the shared skill body into every target because Loaf authors one skill tree and uses labeled harness sections; other products ignore the Amp-only operating rules.

## Default Policy

Use **Loaf Medium** (or **Loaf Ultra** for hard open-ended work) as the main thread. Implementation defaults to `delegate_grok_implementation` and review defaults to `delegate_luna_review` unless the user explicitly overrides that role. Prefer `consult_oracle` for high-impact judgment after investigation.

Built-in Amp `medium` cannot be rewritten by a plugin. Do not claim that ordinary Amp Medium always delegates. After a delegate tool is invoked, pinned models, reasoning, features, and least-authority tool lists are exact.

If a required pin is unavailable, report the compatibility failure and stop. There is no silent fallback and no local fallback. Do not silently perform that role with the orchestrating model, substitute another model, change reasoning, expand capabilities, or invent a local fallback.

A wait timeout is not a pin failure. Implementation delegates start a child thread with `createThread` so aborting the parent tool does not cancel Grok. If the parent times out, open the child thread URL, wait, or steer it. Do not treat `Timed out waiting for agent response` as an unavailable model.

## Isolation and Workdir

Workdir is routing context, not a sandbox. Pass a canonical absolute existing directory. Isolation still requires a Loaf-started isolated worktree (`loaf issue start`) or an appropriate Amp runner/orb when isolation matters. Never treat Amp's local executor as a jail.

## Caller-Prepared Review Snapshots

`delegate_luna_review` requires a nonempty caller-prepared `diff` plus `prompt` and `workdir`. The orchestrator prepares the snapshot before calling the tool. Loaf itself must not execute Git to prepare it, and Luna must not run Git or shell. Repository Git configuration, filters, textconv, fsmonitor, and submodule behavior can execute commands.

Include unstaged and staged diffs and untracked file contents when they are part of the review surface.

## Capability Preflight

Before relying on the delegates, confirm the current Amp surface:

```bash
amp plugins show-agent-options --json
```

Require the pinned model ids `openai/gpt-5.6-sol`, `xai/grok-4.6`, and `openai/gpt-5.6-luna`, plus every tool on each finite allowlist. Missing models, missing tools, leftover picker-mode prototypes that collide on tool names, or a plugin API without `createAgent` / `registerAgentMode` / `registerTool` is an actionable compatibility failure for the delegates. Hook enforcement still loads: `registerDelegatedAgents` is isolated from `tool.call` / `tool.result`, so a leftover prototype must not disable Loaf Amp checks. Local `tsc` against Loaf's generated ambient types is not that preflight. Do not substitute, expand a worker allowlist, or continue locally.

## Migration from the Standalone Prototype

The user-owned prototype `~/.config/amp/plugins/delegated-agents.ts` established the createAgent / registerAgentMode / registerTool shape. It is not Loaf-managed. Later local experiments also registered `grok-impl` / `luna-review` as picker modes; those modes are retired.

1. Install or upgrade Loaf's Amp target (`loaf install --to amp` or `loaf upgrade --to amp`).
2. Confirm the generated plugin registered `loaf-medium`, `loaf-ultra`, `delegate_grok_implementation`, `delegate_luna_review`, and `consult_oracle`, and that Grok/Luna are not selectable modes.
3. Optionally remove leftover user-owned prototypes to avoid duplicate tool names.

Loaf must never overwrite or delete `delegated-agents.ts` or any other unrelated user plugin. Retirement of Loaf's managed `plugins/loaf.ts` leaves those files in place.

## Anti-Patterns

| Don't | Do instead |
|-------|------------|
| Implement or review in Loaf Medium yourself when the matching delegate is available | Call `delegate_grok_implementation` or `delegate_luna_review` unless the user overrode that role |
| Stay on built-in Amp medium and expect forced delegation | Switch to Loaf Medium; built-in Medium cannot be rewritten |
| Register Grok or Luna as picker modes | Keep them as tools only |
| Fall back to the orchestrating model when a pin is missing | Fail closed with the preflight diagnostic |
| Treat a wait timeout as an unavailable model | Open the child thread; the pin worked and the child may still be running |
| Ask Luna or Loaf to run `git diff` | Prepare the snapshot in the caller, then pass `diff` |
| Treat `workdir` as a sandbox | Use a Loaf-started isolated worktree or an appropriate runner |
| Delete `~/.config/amp/plugins/delegated-agents.ts` from Loaf | Leave the prototype and unrelated plugins untouched |

2026-08-21 22:30- Distinguish child-thread timeouts from pin failures; Grok starts via createThread.
2026-08-21 21:20- Isolate hook load from delegate registration; orchestrator allowlist may include mcp__*.
2026-08-21 20:50- Retarget to Loaf Medium/Ultra orchestrators; Grok/Luna/oracle are tools only.
2026-08-21 19:40- Clarify shared-skill copy, leftover prototype collisions, and local tsc limits.
2026-08-21 18:10- Initial Amp Grok/Luna delegate operating guidance.
