---
topics:
  - hooks
  - lifecycle
  - validation
  - enforcement
covers:
  - config/hooks.yaml
  - cli/commands/check.ts
  - content/hooks/**/*
  - content/hooks/instructions/**/*
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-05-02'
---

# Hook System

Hooks run at lifecycle events to enforce rules, inject context, and capture journal entries. Three dispatch types serve different purposes.

## Dispatch Types

| Type | Field | Behavior | Example |
|------|-------|----------|---------|
| **command** | `command:` | Runs a CLI command | `loaf check --hook check-secrets` |
| **command** | `instruction:` | Injects markdown file content (rendered at build time) | `instructions/pre-merge.md` |
| **prompt** | `prompt:` | Injects inline text into model context | Journal nudge reminder |
| **script** | `script:` | Runs a shell/Python script | `hooks/session/compact.sh` |

Enforcement hooks without explicit `type:` auto-dispatch as `loaf check --hook <id>` at build time. Command hooks with `instruction:` instead of `command:` inject a markdown file's content as the hook output -- used for advisory checklists (pre-merge, pre-push, post-merge).

## Hook Events

| Event | Timing | Can Block | Use Case |
|-------|--------|:---------:|----------|
| PreToolUse | Before Edit/Write/Bash | Yes | Secrets check, security audit, commit validation |
| PostToolUse | After Edit/Write/Bash | No | Task board refresh, KB staleness, journal auto-entries |
| SessionStart | Session begins | No | Start session journal, surface context |
| SessionEnd | Session ends | No | End session with progress summary |
| PreCompact | Before context archival | No | State preservation, journal flush |
| PostCompact | After context compaction | No | Resume from session file, restore working context |
| UserPromptSubmit | Every user message | No | Context injection, orchestration conventions |
| TaskCompleted | Task marked complete | No | Journal auto-entry for task events |
| Stop | Session stops | No | Session state cleanup (caution: circularity risk) |

### Hook JSON Context

Harnesses pass JSON on stdin to hooks. Key fields:

| Field | Present In | Purpose |
|-------|-----------|---------|
| `tool.name` / `tool_name` | pre-tool, post-tool | Which tool triggered the hook |
| `tool.input` / `tool_input` | pre-tool, post-tool | Tool arguments (command, file_path, content) |
| `tool_response` | post-tool only | Tool output (stdout/stderr); used by `--from-hook` to extract PR URLs |
| `session_id` | session hooks | Claude session identifier; used for new-conversation detection |
| `agent_id` | session hooks | Present only for subagents; `session start` skips when set (prevents subagent session creation) |
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
| `event` | No | Session event: `SessionStart`, `SessionEnd`, `PreCompact`, `PostCompact` |

## Enforcement Hooks

Five hooks run as TypeScript via `loaf check --hook <id>` in `cli/commands/check.ts`:

| Hook | Skill | Matcher | Blocking | What It Does |
|------|-------|---------|:--------:|-------------|
| `check-secrets` | security-compliance | Edit\|Write\|Bash | Yes (failClosed) | Scans file content and Bash commands for hardcoded secrets |
| `security-audit` | security-compliance | Bash | Yes (failClosed) | Blocks dangerous shell patterns; runs Trivy/Semgrep/npm-audit when available |
| `validate-push` | git-workflow | Bash | Advisory | Verifies version bump, CHANGELOG, build before push |
| `workflow-pre-pr` | git-workflow | Bash | Advisory | Checks PR format, CHANGELOG entry, unpushed base-branch commits |
| `validate-commit` | orchestration | Bash | Yes (failClosed) | Validates Conventional Commits format, blocks AI attribution |

Security hooks (`check-secrets`, `security-audit`) and `validate-commit` use `failClosed: true`. Workflow hooks (`validate-push`, `workflow-pre-pr`) are advisory (`blocking: false`) -- they warn but do not block, since the release skill orchestrates the same checks.

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
| `session-pre-compact-nudge` | PreCompact | Require journal flush and state summary before compaction |
| `session-post-compact-nudge` | PostCompact | Resume from session file after compaction |

## Journal Auto-Entry Hooks

Command hooks that auto-log to the session journal via `loaf session log --from-hook`, which parses the hook JSON (including `tool_response` for PR URLs) to determine entry type:

| Hook | Condition | Logs |
|------|-----------|------|
| `journal-git-events` | `Bash(git commit:*)` | Commits and merges — `commit(SHA): message` |
| `journal-gh-events` | `Bash(gh pr:*)` | PR creation and merges — `pr(create): title (#N)`, `pr(merge): #N merged` |
| `journal-task-completed` | TaskCompleted event | Task completions — `task(complete): subject` |

The `detect-linear-magic` hook runs as a pre-tool command (`loaf session log --detect-linear`) and only fires when Linear integration is enabled in `.agents/loaf.json`.

## Session Lifecycle Hooks

Registered under `session:` in hooks.yaml with an `event:` field:

| Hook | Event | Dispatch | Purpose |
|------|-------|----------|---------|
| `session-start-loaf` | SessionStart | command (`loaf session start`) | Create/resume session, surface context |
| `session-end-loaf` | SessionEnd | command (`loaf session end --if-active`) | End session with progress summary and KB follow-up |
| `session-context-inject` | UserPromptSubmit | command (`loaf session context for-prompt`) | Injects session state and orchestration conventions on every user message |
| `pre-compact` | PreCompact | command (`loaf session context for-compact`) | Log compact marker and inject journal flush instructions before compaction |
| `post-compact` | PostCompact | command (`loaf session context for-resumption`) | Print session state, spec, journal, and git context for post-compaction resumption |
| `session-pre-compact-nudge` | PreCompact | prompt | Require journal flush + state summary (see Prompt Hooks) |
| `session-post-compact-nudge` | PostCompact | prompt | Resume from session file (see Prompt Hooks) |
| `journal-task-completed` | TaskCompleted | command (`loaf session log --from-hook`) | Auto-logs task completions to journal |

The `session start` command uses `agent_id` from hook JSON to detect subagents -- subagents skip session creation to avoid polluting the parent session.

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
