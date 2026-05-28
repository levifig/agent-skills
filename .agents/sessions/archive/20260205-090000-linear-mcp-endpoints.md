---
session:
  title: "Linear MCP endpoint update"
  status: archived
  created: "2026-02-05T00:00:00Z"
  archived_at: "2026-03-27T23:06:02Z"
  archived_by: cleanup
---

# Session: Linear MCP endpoint update

**Date:** 2026-02-05
**Owner:** orchestrating PM

## User request
- Update usage of Linear MCP endpoints per 2026-02-05 changelog.

## Notes
- Changelog summary: SSE endpoint deprecated/removed; migrate to https://mcp.linear.app/mcp. New tools added (initiatives, project milestones/updates/labels) and improved URL/image loading.
- Codebase already uses https://mcp.linear.app/mcp in build target and generated plugin config.
- Linear MCP usage/docs found in:
  - build/targets/claude-code.js (mcp-remote endpoint)
  - plugins/loaf/.claude-plugin/plugin.json (generated endpoint)
  - src/commands/{implement,resume}.md (tool usage guidance)
  - src/skills/orchestration/references/linear.md (config)
  - src/skills/foundations/references/permissions.md (allowed Linear tools list)
  - src/SETUP.md, plugins/loaf/SETUP.md (MCP setup docs)
- Implementation complete:
  - No legacy /sse references found.
  - Added new Linear MCP tool allowlist entries.
  - Updated setup and Linear docs with endpoint change/tool additions.

## Plan references
- .agents/plans/20260205-093000-linear-mcp-endpoints.md (approved)
