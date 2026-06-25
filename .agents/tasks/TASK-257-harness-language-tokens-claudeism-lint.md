---
id: TASK-257
title: Add harness-language tokens and Claude-ism lint
spec: SPEC-047
status: done
priority: P1
created: '2026-06-24T12:03:41Z'
updated: '2026-06-24T12:41:10Z'
completed_at: '2026-06-24T12:41:10Z'
depends_on:
  - TASK-256
files:
  - internal/cli/build_codex.go
  - internal/cli/build_cursor.go
  - internal/cli/build_opencode.go
  - internal/cli/build_amp.go
  - content/skills/
  - dist/
  - .agents/tasks/TASK-257-harness-language-tokens-claudeism-lint.md
verify: >-
  go test ./internal/cli -run 'HarnessLanguage|Claudeism|BuildTarget' -count=1
  && npm run build
done: >-
  The fixed SPEC-047 harness token set resolves per target and the build fails
  with file:line output for unresolved tokens or non-Claude leakage outside an
  explicit allowlist.
---

# TASK-257: Add harness-language tokens and Claude-ism lint

## Description

Extend command substitution into a fixed harness-language token set:
`{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`, `{{SUBAGENT_MECHANISM}}`,
`{{TODO_TOOL}}`, and `{{AGENTS_FILE}}`. Resolve tokens per target according to
SPEC-047, then fail non-Claude builds when raw Claude-specific language leaks.

## Acceptance Criteria

- [x] The fixed token set resolves for Claude Code, Codex, Cursor, OpenCode, and
  Amp.
- [x] Build output contains no unresolved `{{...}}` tokens.
- [x] Non-Claude output contains no raw `CLAUDE.md`, `AskUserQuestionTool`,
  `TodoWrite`, or Claude-specific command names.
- [x] `subagent` leakage is allowed only through an explicit file:reason allowlist
  when the target natively uses that word.
- [x] Lint failures report file and line.
- [x] Tests seed at least one failure and one allowlisted pass.

## Verification

```bash
go test ./internal/cli -run 'HarnessLanguage|Claudeism|BuildTarget' -count=1
npm run build
```
