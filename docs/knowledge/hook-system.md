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
consumers:
  - implementer
  - reviewer
last_reviewed: '2026-04-04'
---

# Hook System

Hooks run at lifecycle events to enforce rules, inject context, and capture journal entries. Three dispatch types serve different purposes.

## Dispatch Types

| Type | Field | Behavior | Example |
|------|-------|----------|---------|
| **command** | `command:` | Runs a CLI command | `loaf check --hook check-secrets` |
| **prompt** | `prompt:` | Injects text into model context | Squash merge conventions reminder |
| **script** | `script:` | Runs a shell/Python script | `hooks/session/compact.sh` |

Enforcement hooks without explicit `type:` auto-dispatch as `loaf check --hook <id>` at build time.

## Hook Events

| Event | Timing | Can Block | Use Case |
|-------|--------|:---------:|----------|
| PreToolUse | Before Edit/Write/Bash | Yes | Secrets check, security audit, commit validation |
| PostToolUse | After Edit/Write/Bash | No | Task board refresh, KB staleness, journal auto-entries |
| SessionStart | Session begins | No | Start session journal, surface context |
| SessionEnd | Session ends | No | End session with progress summary |
| PreCompact | Before context archival | No | State preservation, journal flush |
| Stop | Before response sent | No | Journal nudge — require unlogged decisions be captured |

## Registration

Hooks are defined in `config/hooks.yaml` grouped under `pre-tool`, `post-tool`, or `session`.

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
| `event` | No | Session event: `SessionStart`, `SessionEnd`, `PreCompact`, `Stop` |

## Enforcement Hooks

Five hooks run as TypeScript via `loaf check --hook <id>` in `cli/commands/check.ts`:

| Hook | Skill | Matcher | What It Does |
|------|-------|---------|-------------|
| `check-secrets` | security-compliance | Edit\|Write\|Bash | Scans file content and Bash commands for hardcoded secrets |
| `security-audit` | security-compliance | Bash | Blocks dangerous shell patterns |
| `validate-push` | git-workflow | Bash | Verifies version bump, CHANGELOG, build before push |
| `workflow-pre-pr` | git-workflow | Bash | Enforces PR format and CHANGELOG entry |
| `validate-commit` | orchestration | Bash | Validates Conventional Commits format |

All use `failClosed: true` — if the hook process crashes, the action is blocked.

## Prompt Hooks

Inject advisory or mandatory text into the model's context:

| Hook | Event/Condition | Purpose |
|------|-----------------|---------|
| `workflow-pre-merge` | `Bash(gh pr merge:*)` | Squash merge conventions |
| `workflow-pre-push` | `Bash(git push:*)` | Pre-push checklist reminder |
| `workflow-post-merge` | `Bash(gh pr merge:*)` | Post-merge housekeeping checklist |
| `session-stop-nudge` | Stop | Require unlogged journal entries before responding |
| `session-pre-compact-nudge` | PreCompact | Require journal flush before compaction |

## Journal Auto-Entry Hooks

Three post-tool command hooks auto-log to the session journal:

| Hook | Condition | Logs |
|------|-----------|------|
| `journal-post-commit` | `Bash(git commit:*)` | `commit(SHA): "message"` |
| `journal-post-pr` | `Bash(gh pr create:*)` | PR creation entry |
| `journal-post-merge` | `Bash(gh pr merge:*)` | Merge entry |

All run `loaf session log --from-hook` which parses the tool output for commit/PR/merge details.

## Migration History

As of SPEC-020, ~30 shell hooks were migrated to skill instructions (Verification sections in SKILL.md) or replaced by `loaf check` enforcement hooks. Eliminated categories:
- Language hooks (python-\*, typescript-\*, rails-\*)
- Infrastructure hooks (k8s, dockerfile, terraform)
- Design hooks (a11y, tokens)
- Quality hooks (format-check, tdd-advisory, validate-changelog)

The 4 shared bash libraries (`json-parser.sh`, `config-reader.sh`, `agent-detector.sh`, `timeout-manager.sh`) were also removed.

## Cross-References

- [build-system.md](build-system.md) — how hooks get distributed to targets
- [skill-architecture.md](skill-architecture.md) — how skills own hooks via `skill:` field in hooks.yaml
