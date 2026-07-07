---
topics:
  - hooks
  - lifecycle
  - validation
  - enforcement
covers:
  - config/hooks.yaml
  - internal/cli/check.go
  - content/hooks/**/*
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-07-03'
---

# Hook System

Hooks run at lifecycle events to enforce rules, inject context, and capture journal entries. Three dispatch types serve different purposes.

## Dispatch Types

| Type | Field | Behavior | Example |
|------|-------|----------|---------|
| **command** | `command:` | Runs a CLI command | `loaf check --hook check-secrets` |
| **command** | `instruction:` | Injects markdown file content (rendered at build time) | `instructions/pre-merge.md` |
| **prompt** | `prompt:` | Injects inline text into model context | Journal nudge reminder |
| **script** | `script:` | Runs a shell/Python script | `hooks/pre-commit/scan-secrets.sh` |

Enforcement hooks without explicit `type:` auto-dispatch as `loaf check --hook <id>` at build time. Command hooks with `instruction:` instead of `command:` inject a markdown file's content as the hook output -- used for advisory checklists (pre-merge, pre-push, post-merge).

## Hook Events

| Event | Timing | Can Block | Use Case |
|-------|--------|:---------:|----------|
| PreToolUse | Before Edit/Write/Bash | Yes | Secrets check, security audit, commit validation |
| PostToolUse | After Edit/Write/Bash | No | Task board refresh, KB staleness, journal auto-entries |
| SessionStart | Conversation begins | No | Emit the layered continuity digest |
| PreCompact | Before context archival | No | Inject journal-flush guidance |
| PostCompact | After context compaction | No | Re-emit the continuity digest |
| UserPromptSubmit | Every user message | No | Context injection, orchestration conventions |
| TaskCompleted | Task marked complete | No | Journal auto-entry for task events |

There is no SessionEnd or Stop journal obligation under journal-first (SPEC-056): the SessionEnd hook was removed, and no hook writes back to a session record (the general Stop-circularity caution still governs any future stateful hook — see [ARCHITECTURE.md](../ARCHITECTURE.md#hook-type-behavioral-constraints)).

### Hook JSON Context

Harnesses pass JSON on stdin to hooks. Key fields:

| Field | Present In | Purpose |
|-------|-----------|---------|
| `tool.name` / `tool_name` | pre-tool, post-tool | Which tool triggered the hook |
| `tool.input` / `tool_input` | pre-tool, post-tool | Tool arguments (command, file_path, content) |
| `tool_response` | post-tool only | Tool output (stdout/stderr); used by `--from-hook` to extract PR URLs |
| `session_id` | session hooks | Claude conversation identifier; recorded as the journal entry's `harness_session_id` |
| `agent_id` | session hooks | Present only for subagents; journal `--from-hook` exits silently when set (subagents write nothing) |
| `hook_event_name` | session events (TaskCompleted, etc.) | Event type identifier — session events use this instead of `tool_name` |
| `task_subject`, `task_description` | TaskCompleted | Task details for journal logging |

## Registration

Hooks are defined in `config/hooks.yaml` grouped under `pre-tool`, `post-tool`, or `session`. For Claude Code, hooks are registered in `hooks/hooks.json` inside the plugin directory — `plugin.json` silently drops non-matcher lifecycle events. See [build-system.md](build-system.md) for details on how hooks are distributed to targets.

### Fields

| Field | Required | Notes |
|-------|----------|-------|
| `id` | Yes | Unique hook identifier |
| `skill` | Yes | Owning skill name |
| `type` | No | `script`, `command`, or `prompt` (enforcement hooks default to command) |
| `matcher` | No | Tool name filter: `"Edit\|Write\|Bash"` |
| `if` | No | Conditional: `"Bash(git commit:*)"` — hook only fires when invocation matches |
| `blocking` | No | `true` if hook can block tool execution |
| `failClosed` | No | `true` to block on hook failure (enforcement hooks) |
| `timeout` | No | Timeout in milliseconds |
| `event` | No | Session event: `SessionStart`, `PreCompact`, `PostCompact`, `TaskCompleted`, `UserPromptSubmit` |

## Enforcement Hooks

Native enforcement and workflow checks run via `loaf check --hook <id>` in `internal/cli/check.go`:

| Hook | Skill | Matcher | Blocking | What It Does |
|------|-------|---------|:--------:|-------------|
| `check-secrets` | security-compliance | Edit\|Write\|Bash | Yes (failClosed) | Scans file content and Bash commands for hardcoded secrets |
| `security-audit` | security-compliance | Bash | Yes (failClosed) | Blocks dangerous shell patterns; runs Trivy/Semgrep/npm-audit when available |
| `github-account` | git-workflow | Bash | Yes (failClosed) | **Tier 1** (default identity mechanism): converges the active GitHub CLI account to match `.agents/loaf.json` before `gh` commands (pass-with-warning), exempting `gh auth` administration; blocks only when the switch fails. The zero-footprint fallback beneath the opt-in `loaf shim enable gh` shim (tier 3) |
| `validate-push` | git-workflow | Bash | Advisory | Verifies version bump, CHANGELOG, build before push |
| `workflow-pre-pr` | git-workflow | Bash | Advisory | Checks PR format, CHANGELOG entry, unpushed base-branch commits |
| `validate-commit` | orchestration | Bash | Yes (failClosed) | Validates Conventional Commits format, blocks AI attribution |

Security hooks (`check-secrets`, `security-audit`), `github-account`, and `validate-commit` use `failClosed: true`. Workflow hooks (`validate-push`, `workflow-pre-pr`) are advisory (`blocking: false`) -- they warn but do not block, since `/ship` and `/release` orchestrate the same checks at their respective PR-landing and publication gates.

Residual risk on `github-account`: convergence *writes* the shared global gh account pointer on every mismatched `gh` call -- including read-only ones like `gh pr view`, which the earlier pure-read check only observed -- so concurrent sessions on different identities collide on that pointer more frequently; the race window's shape is unchanged, its trigger rate is not. `gh auth` administration is exempt (identity management is the user's domain), passing through with no probe or switch.

This convergence is **tier 1** of the tiered identity model (tier 2, harness-level command rewriting, is deliberately deferred): the default, zero-footprint mechanism, always on wherever Loaf hooks run, that mutates the shared pointer and accepts the disclosed frequency cost as its price. **Tier 3** is the opt-in per-invocation shim (`loaf shim enable gh`): it resolves the configured account's token into `GH_TOKEN` for a single `gh` child process and never writes gh's global state, so concurrent sessions on different identities stop racing the pointer entirely -- and agents keep typing bare `gh`. The tiers coexist rather than compete: tier 1 stays the net wherever the shim can't reach (absolute-path `gh` invocations, non-shim machines, GUI-launched apps). The shim's enable/disable/consent model and its residual-exposure matrix live in the loaf-reference configuration and troubleshooting references.

## Instruction Hooks

Inject markdown file content at tool invocation via `type: command` with `instruction:` field:

| Hook | Event/Condition | Purpose |
|------|-----------------|---------|
| `workflow-pre-merge` | `Bash(gh pr merge:*)` | Squash merge conventions checklist |
| `workflow-pre-push` | `Bash(git push:*)` | Pre-push advisory reminders |
| `workflow-post-merge` | `Bash(gh pr merge:*)` | Post-merge housekeeping checklist |

Instruction files live in `content/hooks/instructions/` and are rendered into hook output at build time.

## Prompt Hooks

Inject inline text into the model's context. Prompt hooks are binary gates — any non-empty LLM response blocks the action. Use only for validation that returns empty on success, never for advisory nudges. See [ARCHITECTURE.md](../ARCHITECTURE.md#hook-type-behavioral-constraints) for full behavioral constraints.

| Hook | Event/Condition | Purpose |
|------|-----------------|---------|
| `session-pre-compact-nudge` | PreCompact | Require journal flush and next-actions summary before compaction |
| `session-post-compact-nudge` | PostCompact | Re-read the continuity digest after compaction |

## Journal Auto-Entry Hooks

Command hooks that auto-log to the project journal via `loaf journal log --from-hook`, which parses the hook JSON (including `tool_response` for PR URLs) to determine entry type. `--from-hook` reads the harness payload on stdin and exits silently for subagent invocations:

| Hook | Condition | Logs |
|------|-----------|------|
| `journal-git-events` | `Bash(git commit:*)` | Commits and merges — `commit(SHA): message` |
| `journal-gh-events` | `Bash(gh pr:*)` | PR creation and merges — `pr(create): title (#N)`, `pr(merge): #N merged` |
| `journal-task-completed` | TaskCompleted event | Task completions — `task(complete): subject` |

The `detect-linear-magic` hook runs as a pre-tool command (`loaf journal log --detect-linear`) and only fires when Linear integration is enabled in `.agents/loaf.json`.

## Conversation Hooks

Registered under `session:` in hooks.yaml with an `event:` field. Journal-first (SPEC-056) removed the session entity, so these hooks emit the derived continuity digest or auto-log events — none of them open, close, or mutate a session record:

| Hook | Event | Dispatch | Purpose |
|------|-------|----------|---------|
| `session-start-loaf` | SessionStart | command (`loaf journal context --from-hook`) | Emit the layered continuity digest (latest wrap + branch entries + open tasks); silent for subagents |
| `session-context-inject` | UserPromptSubmit | command (`loaf journal context for-prompt`) | Inject implementation conventions on prompt submit |
| `pre-compact` | PreCompact | command (`loaf journal context for-compact`) | Inject journal-flush guidance before compaction; writes nothing itself |
| `post-compact` | PostCompact | command (`loaf journal context for-resumption`) | Re-emit the continuity digest for post-compaction resumption |
| `session-pre-compact-nudge` | PreCompact | prompt | Require journal flush + next-actions summary (see Prompt Hooks) |
| `session-post-compact-nudge` | PostCompact | prompt | Re-read the digest after compaction (see Prompt Hooks) |
| `journal-task-completed` | TaskCompleted | command (`loaf journal log --from-hook`) | Auto-logs task completions to the journal |

`loaf journal context --from-hook` and `loaf journal log --from-hook` use `agent_id` from hook JSON to detect subagents — subagent invocations exit silently and write nothing.

## Side-Effect Hooks

Post-tool hooks that perform background maintenance:

| Hook | Condition | Purpose |
|------|-----------|---------|
| `generate-task-board` | Edit\|Write | Regenerates TASKS.md when task files change (`loaf task refresh`) |
| `kb-staleness-nudge` | Edit\|Write | Tracks covered edits and nudges when covered knowledge is stale |

## Migration History

As of SPEC-020, ~30 shell hooks were migrated to skill instructions (Verification sections in SKILL.md) or replaced by `loaf check` enforcement hooks. Eliminated categories:
- Language hooks (python-\*, typescript-\*, rails-\*)
- Infrastructure hooks (k8s, dockerfile, terraform)
- Design hooks (a11y, tokens)
- Quality hooks (format-check, tdd-advisory, validate-changelog)

The 4 shared bash libraries (`json-parser.sh`, `config-reader.sh`, `agent-detector.sh`, `timeout-manager.sh`) were also removed.

## Hook Type Behavioral Constraints

Hook types have hard behavioral limits discovered through SPEC-026 and SPEC-030. See [ARCHITECTURE.md](../ARCHITECTURE.md#hook-type-behavioral-constraints) for the full list. Key constraints:

- **Prompt hooks** are binary gates — any non-empty LLM response blocks. Never use for advisory guidance.
- **Agent hooks** are read-only (no Edit/Write/Bash). Max 50 turns.
- **Command hooks** are the correct primitive for context injection and side effects.
- **Stop hooks** risk circular re-triggers when writing to monitored files.
- **PreCompact prompt hooks** don't work outside REPL sessions.

## Cross-References

- [build-system.md](build-system.md) — how hooks get distributed to targets
- [skill-architecture.md](skill-architecture.md) — how skills own hooks via `skill:` field in hooks.yaml
