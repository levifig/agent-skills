---
id: TASK-060
title: Delete 8 role-agent files and update AGENTS.md
spec: SPEC-014
status: done
priority: P1
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-060: Delete 8 role-agent files and update AGENTS.md

Remove the 8 role-based agent definitions and update AGENTS.md to reference the new profile model.

## Delete role agents

Remove these files (agent .md + all sidecars):
- `content/agents/pm.md` + `pm.claude-code.yaml` + `pm.opencode.yaml`
- `content/agents/backend-dev.md` + `backend-dev.claude-code.yaml` + `backend-dev.opencode.yaml`
- `content/agents/frontend-dev.md` + `frontend-dev.claude-code.yaml` + `frontend-dev.opencode.yaml`
- `content/agents/dba.md` + `dba.claude-code.yaml` + `dba.opencode.yaml`
- `content/agents/devops.md` + `devops.claude-code.yaml` + `devops.opencode.yaml`
- `content/agents/qa.md` + `qa.claude-code.yaml` + `qa.opencode.yaml`
- `content/agents/design.md` + `design.claude-code.yaml` + `design.opencode.yaml`
- `content/agents/power-systems.md` + `power-systems.claude-code.yaml` + `power-systems.opencode.yaml`

**Keep**: `background-runner.*`, `context-archiver.*`

## Update AGENTS.md

Update `.agents/AGENTS.md` to:
- Reference SOUL.md for the Warden identity
- Document the 3 functional profiles (implementer, reviewer, researcher) + 2 system agents
- Remove all references to role-based agents

## Test
- 8 role-agent files and all their sidecars are deleted (24+ files)
- `content/agents/` contains only: implementer, reviewer, researcher, background-runner, context-archiver
- AGENTS.md references SOUL.md
- `loaf build` succeeds

## Relates to
- R7
