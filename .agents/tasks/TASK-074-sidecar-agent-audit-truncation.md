---
id: TASK-074
title: 'Sidecar audit, agent profile audit, description truncation'
spec: SPEC-020
status: todo
priority: P1
created: '2026-04-04T16:41:22.295Z'
updated: '2026-04-04T16:41:22.295Z'
---

# TASK-074: Sidecar audit, agent profile audit, description truncation

Three related audits and one build system change.

## Scope

### Sidecar audit
- Review all ~30 `SKILL.claude-code.yaml` + ~13 `SKILL.opencode.yaml` files
- Verify every sidecar field is genuinely target-specific (not universal)
- Promote any universal fields back to base SKILL.md
- Create missing sidecars where needed (Codex, Cursor, Gemini)
- Document invocability semantic mapping: `user-invocable` (CC) vs `disable-model-invocation` (Cursor) vs Codex policy

### Agent profile audit
- Verify 3 profiles (implementer, reviewer, researcher) across all targets
- Validate Cursor `tools` field format — confirm object map works or switch to `readonly: true`
- Document Codex-specific enhancements (`nickname_candidates`, `model_reasoning_effort`)
- Verify Claude Code agents can preload skills via `skills` frontmatter field

### Description truncation (build change)
- Claude Code target truncates descriptions at 250 chars during sidecar merge
- All other targets preserve full description
- Truncation happens in `claude-code.ts` during skill copy, not in shared modules

## Verification

- [ ] Sidecar fields documented as genuinely target-specific or promoted to base
- [ ] Invocability semantic mapping documented and applied
- [ ] Agent `tools` format validated per target
- [ ] Claude Code build output has descriptions truncated at 250 chars
- [ ] Other targets preserve full descriptions
- [ ] `npm run typecheck` and `npm run test` pass
