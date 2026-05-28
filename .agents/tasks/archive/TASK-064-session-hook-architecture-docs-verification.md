---
id: TASK-064
title: 'SessionStart hook, ARCHITECTURE.md update, end-to-end verification'
spec: SPEC-014
status: done
priority: P2
created: '2026-04-04T16:41:22.302Z'
updated: '2026-04-04T16:41:22.302Z'
completed_at: '2026-04-04T16:41:22.302Z'
---

# TASK-064: SessionStart hook, ARCHITECTURE.md update, end-to-end verification

Final integration: create the self-healing SessionStart hook, update architecture docs, and verify all test conditions.

## SessionStart hook

Create a SessionStart hook that:
1. Checks if SOUL.md is referenced and loadable
2. If present → passes silently
3. If missing → injects Warden identity from `content/templates/soul.md` and warns the user

Register in `config/hooks.yaml` as a session hook.

## Update ARCHITECTURE.md

Update `docs/ARCHITECTURE.md` to reflect:
- Profile model (implementer, reviewer, researcher) with Tolkien naming
- Warden as coordinator (SOUL.md)
- Skills as the universal knowledge layer
- Removal of role-based agents
- Council composition rules

## End-to-end verification

Run through all test conditions from the spec:

### Build integrity
- [ ] R0: `loaf build` succeeds for all 5 targets
- [ ] R1: `npm run typecheck` and `npm run test` pass
- [ ] R2: Zero `{{AGENT:` patterns in built output

### Foundations decomposition
- [ ] R3: Each new skill has SKILL.md + sidecar + references/
- [ ] R4: Slimmed foundations no longer references moved files
- [ ] R5: Manual test — "merge this PR" scenario
- [ ] R6: Manual test — "debug this flaky test" scenario

### Profile model
- [ ] R7: 8 role-agent files deleted; 3 profiles exist
- [ ] R8: Reviewer has read-only tool access
- [ ] R9: Researcher has read + web only
- [ ] R10: Implementer can be spawned in parallel

### Warden + orchestration
- [ ] R11: SOUL.md defines Warden identity
- [ ] R12: AGENTS.md references SOUL.md
- [ ] R13: SessionStart hook validates SOUL.md
- [ ] R14: ARCHITECTURE.md reflects profile model
- [ ] R15: Skills use concept names only

### Description audit
- [ ] R16: Every skill has audited description

## Relates to
- R0-R16 (all)
