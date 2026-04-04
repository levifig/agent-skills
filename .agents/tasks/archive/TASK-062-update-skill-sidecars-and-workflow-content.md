---
id: TASK-062
title: Update all skill sidecars and workflow content for profile model
spec: SPEC-014
status: done
priority: P1
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-062: Update all skill sidecars and workflow content for profile model

Update every skill that references the old agent model to use profile-based spawning.

## Remove agent: fields from sidecars

Remove `agent:` field from all `.claude-code.yaml` and `.opencode.yaml` sidecars:
- python-development, ruby-development, go-development, typescript-development
- database-design, infrastructure-management, interface-design, power-systems-modeling
- orchestration (currently `agent: "{{AGENT:pm}}"`)
- All OpenCode sidecars that reference `{{AGENT:pm}}`

For reference skills that had `context: fork` + `agent:`, decide:
- If skill should still run in a subagent → use `agent: implementer` (or reviewer/researcher as appropriate)
- If skill is reference-only (loaded into main context) → remove both `context:` and `agent:`

## Update workflow skill content

Update SKILL.md content in workflow skills that reference `{{AGENT:...}}` patterns:

### `implement/SKILL.md`
- Replace the agent spawning table (backend-dev, frontend-dev, etc.) with profile-based spawning
- All implementation work → `subagent_type: "implementer"` (skills loaded in prompt)
- Code review → `subagent_type: "reviewer"`
- Research → `subagent_type: "researcher"`

### `orchestration/SKILL.md` + references
- Update delegation references to use concept names (implementer, reviewer, researcher)
- Update `references/delegation.md`, `references/parallel-agents.md`, etc.

### `council-session/SKILL.md`
- Update council spawning to use profiles instead of role agents

### `pm.md` content (already being deleted in TASK-060)
- Ensure nothing in remaining skills references PM agent

## Update agent markdown files

Update `content/agents/context-archiver.md`:
- Replace `{{AGENT:backend-dev}}` reference with profile concept

## Test
- Zero `{{AGENT:` patterns in any source file
- Zero `agent:` fields referencing old role names in any sidecar
- Skills reference concept names only (implementer, reviewer, researcher)
- `loaf build` succeeds

## Relates to
- R2, R10, R15
