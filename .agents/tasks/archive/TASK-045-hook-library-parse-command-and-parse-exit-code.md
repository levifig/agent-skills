---
id: TASK-045
title: Hook library — parse_command and parse_exit_code
spec: SPEC-015
status: done
priority: P1
created: '2026-03-27T16:27:22.010Z'
updated: '2026-03-27T16:34:48.647Z'
completed_at: '2026-03-27T16:34:48.646Z'
---

# TASK-045: Hook library — parse_command and parse_exit_code

## Description

Add two new functions to `content/hooks/lib/json-parser.sh`:

1. **`parse_command`** — Extracts `tool_input.command` from PreToolUse/PostToolUse JSON. Used by all three workflow hooks to match on `gh pr create`, `gh pr merge`, `git push`.

2. **`parse_exit_code`** — Extracts the exit code from PostToolUse `tool_result` JSON. Used by the post-merge hook to gate on successful merge. The exact JSON path needs to be confirmed during implementation (likely `tool_result.stdout` exit code or a dedicated field — inspect a PostToolUse hook's stdin to confirm).

Both functions should follow the existing pattern: try jq first, fall back to grep/sed. Export both functions like the existing ones.

## Acceptance Criteria

- [ ] `parse_command` extracts command string from `{"tool_input":{"command":"..."}}` JSON
- [ ] `parse_exit_code` extracts exit code from PostToolUse JSON (confirm schema during implementation)
- [ ] Both functions use jq-with-fallback pattern matching existing functions
- [ ] Both functions are exported (`export -f`)
- [ ] Existing functions (`parse_file_path`, `parse_tool_name`, etc.) remain unchanged

## Context

See SPEC-015 § "Hook Implementation" for the hook library extension requirement. All three workflow hooks depend on these functions.
