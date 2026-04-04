---
id: TASK-087
title: Remove bundled MCP servers from plugin
spec: SPEC-020
status: done
priority: P1
created: '2026-04-03T15:23:56.470Z'
updated: '2026-04-04T16:47:26.854Z'
completed_at: '2026-04-04T16:47:26.853Z'
---

# TASK-087: Remove bundled MCP servers from plugin

## Objective

Remove all three MCP servers (sequential-thinking, Linear, Serena) from the Claude Code
plugin manifest. Update build system, docs, agent tool declarations, and skill references
to reflect that MCPs are user-configured, not Loaf-bundled.

## Acceptance Criteria

- [ ] `sequential-thinking` removed from plugin.json and build target
- [ ] `linear` removed from plugin.json and build target; `linear-mcp.sh` removed
- [ ] `serena` removed from plugin.json and build target
- [ ] `claude-code.ts` build target no longer injects `mcpServers` into plugin.json
- [ ] Skill docs updated: Linear/Serena references become conditional ("if available")
- [ ] `context-archiver` agent sidecar updated (Serena tool references)
- [ ] README.md and SETUP.md updated
- [ ] `.claude/settings.local.json` permissions cleaned up
- [ ] `loaf build` succeeds
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

## Files to Modify

| File | Change |
|------|--------|
| `plugins/loaf/.claude-plugin/plugin.json` | Remove `mcpServers` section |
| `cli/lib/build/targets/claude-code.ts` | Remove MCP_SERVERS injection |
| `content/hooks/linear-mcp.sh` | Delete |
| `content/agents/context-archiver.claude-code.yaml` | Remove Serena tool refs |
| `content/skills/orchestration/references/cross-session.md` | Make Serena conditional |
| `content/skills/orchestration/references/linear.md` | Make references conditional |
| `content/skills/implement/SKILL.md` | Make Linear MCP conditional |
| `content/skills/cleanup/SKILL.md` | Make Linear MCP conditional |
| `content/skills/foundations/references/permissions.md` | Update MCP sections |
| `content/skills/breakdown/SKILL.md` | Make Linear conditional |
| `README.md` | Update MCP section |
| `content/SETUP.md` | Update MCP section |
| `.claude/settings.local.json` | Remove plugin MCP permissions |
