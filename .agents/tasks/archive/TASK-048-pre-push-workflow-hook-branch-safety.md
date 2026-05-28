---
id: TASK-048
title: Pre-push workflow hook + branch safety
spec: SPEC-015
status: done
priority: P2
created: '2026-03-27T16:27:36.312Z'
updated: '2026-03-27T16:40:24.673Z'
depends_on:
  - TASK-045
completed_at: '2026-03-27T16:40:24.672Z'
---

# TASK-048: Pre-push workflow hook + branch safety

## Description

Create the advisory pre-push hook — lightweight reminders before pushing. Two files:

1. **`content/hooks/pre-tool/workflow-pre-push.sh`** — Bash script that:
   - Reads JSON stdin via `parse_command` from hook library
   - Matches commands containing `git push`
   - Outputs `pre-push.md` to stdout
   - Detects branch name and flags from the command
   - Extra warning to stderr if force-pushing to main/master
   - Non-blocking (always exit 0)

2. **`content/hooks/instructions/pre-push.md`** — Advisory content:
   - Branch naming convention reminder (`<type>/<description>`)
   - Force-push warning for protected branches
   - Reminder to check for uncommitted housekeeping files

**Note:** This hook coexists with the existing `foundations-validate-push.sh` which does hard validation (version bump, CHANGELOG, build). This hook adds softer advisory reminders. Both fire on `git push` — that's fine, they serve different purposes.

**Circuit breaker:** This is P2. At 50% appetite, skip this task entirely. Pre-PR and post-merge hooks deliver the core value.

## Acceptance Criteria

- [ ] Hook script only outputs when Bash command contains `git push`
- [ ] Advisory content includes branch naming convention
- [ ] Force-push to main/master emits extra warning to stderr
- [ ] Non-blocking (always exit 0)
- [ ] Doesn't conflict with existing `foundations-validate-push.sh`

## Context

See SPEC-015 § "Pre-Push Hook" for the advisory content. Circuit breaker at 50% appetite says skip this task.
