---
id: TASK-047
title: Post-merge workflow hook + housekeeping checklist
spec: SPEC-015
status: done
priority: P1
created: '2026-03-27T16:27:33.339Z'
updated: '2026-03-27T16:40:24.232Z'
depends_on:
  - TASK-045
completed_at: '2026-03-27T16:40:24.232Z'
---

# TASK-047: Post-merge workflow hook + housekeeping checklist

## Description

Create the success-gated post-merge hook — triggers housekeeping after a PR is merged. Two files:

1. **`content/hooks/post-tool/workflow-post-merge.sh`** — Bash script that:
   - Reads JSON stdin via `parse_command` from hook library
   - Matches commands containing `gh pr merge`
   - Uses `parse_exit_code` to check merge succeeded (exit code 0)
   - If merge failed: exits silently
   - If merge succeeded: outputs `post-merge.md` to stdout (non-blocking, exit 0)

2. **`content/hooks/instructions/post-merge.md`** — Housekeeping checklist:
   1. Switch to main, pull (handle rebase if needed)
   2. Mark tasks done (`loaf task update TASK-XXX --status done`)
   3. Mark spec done if all tasks complete
   4. Finalize CHANGELOG — move `[Unreleased]` entries to versioned section if bumping
   5. Bump version in `package.json` (patch for fix, minor for feat, or dev bump)
   6. Rebuild all targets (`loaf build`)
   7. Archive session file (status: complete, archived_at, archived_by)
   8. Commit housekeeping (`chore: bump to X.Y.Z, close TASK-XXX session`)
   9. Delete merged feature branch

**Open question to resolve during implementation:** Confirm the exact JSON field path for exit code in PostToolUse input. Inspect a PostToolUse hook's stdin to find the right path (likely `tool_result.exit_code` or similar). Update `parse_exit_code` in TASK-045 accordingly.

## Acceptance Criteria

- [ ] Hook script only outputs when Bash command contains `gh pr merge`
- [ ] Hook checks exit code — only injects checklist on successful merge (exit 0)
- [ ] Failed merge: hook exits silently (no output, no context waste)
- [ ] Checklist covers all 9 housekeeping steps from the spec
- [ ] Version bump guidance references commit type (feat→minor, fix→patch)
- [ ] Non-blocking (always exit 0 from the hook itself)

## Context

See SPEC-015 § "Post-Merge Hook" for the full checklist and success-gating logic. Source material for the checklist: `implement SKILL.md` AFTER phase and `foundations/references/commits.md`.
