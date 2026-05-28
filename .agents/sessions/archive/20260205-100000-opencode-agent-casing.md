---
session:
  title: "OpenCode agent name casing mismatch"
  status: archived
  created: "2026-02-05T00:00:00Z"
  archived_at: "2026-03-27T23:06:02Z"
  archived_by: cleanup
---

# Session: OpenCode agent name casing mismatch

**Date:** 2026-02-05
**Owner:** orchestrating PM

## User request
- `/review-sessions` breaks in OpenCode because it can't find agent `pm` (registered as `PM`).

## Findings
- **Systemic issue** affecting 8 of 10 agents in OpenCode target.
- OpenCode sidecars use Title Case names (PM, Backend Dev, QA, DBA, DevOps, Design, Power Systems, Frontend Dev).
- ALL references in commands, skills, delegation tables, and councils use lowercase kebab-case (pm, backend-dev, qa, etc.).
- Only `context-archiver` and `background-runner` happen to match (both use kebab-case in OpenCode sidecars too).
- Cursor and Claude Code targets both use lowercase kebab-case consistently.

## Plan references
- .agents/plans/20260205-103000-agent-name-normalization.md (approved)
