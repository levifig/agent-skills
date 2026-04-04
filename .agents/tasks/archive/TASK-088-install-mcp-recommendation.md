---
id: TASK-088
title: Add MCP recommendation to loaf install
status: done
priority: P2
created: '2026-04-03T15:23:56.470Z'
updated: '2026-04-04T16:47:28.995Z'
depends_on:
  - TASK-087
session: 20260403-152358-task-088.md
completed_at: '2026-04-04T16:47:28.995Z'
---

# TASK-088: Add MCP recommendation to loaf install

## Objective

After `loaf install` completes target installation, detect existing MCP configurations
and offer to install missing recommended/optional MCPs. Support both global (user-level)
and per-project scopes. Write the user's choice to `.agents/config.json` so `loaf build`
can conditionally include/exclude integration-specific skill instructions.

## Acceptance Criteria

- [ ] `loaf install` detects existing Linear and Serena MCP configurations
- [ ] Offers scope choice: global (user-level) or project-level
- [ ] Claude Code: runs `claude mcp add` with correct args and `--scope` flag
- [ ] Cursor: writes to `.cursor/mcp.json`
- [ ] Other targets: prints manual install command
- [ ] Already-configured MCPs are skipped with a green checkmark
- [ ] User's choice written to `.agents/config.json` under `integrations` key
- [ ] Skills use explicit if/else branching based on `integrations.linear.enabled` in `.agents/config.json` (runtime, not build-time)
- [ ] Hooks/scripts read config at invocation and short-circuit when integration is disabled
- [ ] `--upgrade` mode skips MCP recommendations
- [ ] No MCPs installed without user confirmation
- [ ] Tests cover detection, recommendation, and config write logic

## Design

### Scope Selection

```
  Recommended MCP servers:

  Linear (issue tracking):
    ⚡ Not configured
    Install? [g]lobal / [p]roject / [n]o: g
    → claude mcp add --scope user linear -- npx mcp-remote https://mcp.linear.app/mcp
    ✓ Linear MCP added (global)

  Serena (optional — semantic editing for large codebases):
    ✓ Already configured (global)
```

### Config Output (`.agents/config.json`)

```json
{
  "integrations": {
    "linear": { "enabled": true },
    "serena": { "enabled": true }
  }
}
```

When `linear.enabled: false` (user declined), `loaf build` produces skill variants
without Linear-specific workflows. This uses the existing sidecar/transform pipeline —
not a new architecture, just a new build-time conditional.

### Detection (`cli/lib/detect/mcp.ts`)

```typescript
interface McpStatus {
  name: string;
  configured: boolean;
  scope: "global" | "project" | null;
  tier: "recommended" | "optional";
}
```

Detection per target:
- **Claude Code**: `claude mcp list` or parse `~/.claude/settings.json`
- **Cursor**: Check `.cursor/mcp.json` in project root
- **OpenCode**: Check `$XDG_CONFIG_HOME/opencode/` config
- **Other targets**: Best-effort, fallback to manual instructions

### MCP Registry

| MCP | Tier | Install (Claude Code) |
|-----|------|----------------------|
| Linear | Recommended | `claude mcp add [--scope user] linear -- npx mcp-remote https://mcp.linear.app/mcp` |
| Serena | Optional | `claude mcp add [--scope user] serena -- uvx -p 3.13 --from git+https://github.com/oraios/serena serena start-mcp-server --context claude-code --project-from-cwd` |

### Runtime Conditional (Not Build-Time)

Skills ship with **both paths** — Linear and local instructions coexist. The agent
reads `.agents/config.json` at session start and follows the appropriate branch.

**Skill pattern:**
```markdown
### Task Creation
**If `integrations.linear.enabled` is true in `.agents/config.json`:**
Create Linear issue with title, description, labels, priority.

**Otherwise:** Use `loaf task create --spec SPEC-XXX --title "..."` for local tracking.
```

**Hook pattern:** Scripts read config at invocation, exit 0 silently when disabled.
No build-time stripping — toggling `enabled` takes effect immediately.

This allows dynamic per-project opt-in/out without rebuilding or reinstalling.

## Files to Create/Modify

| File | Change |
|------|--------|
| `cli/lib/detect/mcp.ts` | New: MCP detection and status |
| `cli/commands/install.ts` | Add MCP recommendation step |
| `content/skills/*/SKILL.md` | Add if/else branching for Linear vs local paths |
| `.agents/config.json` | Updated by install (user choice) |
| `cli/lib/detect/mcp.test.ts` | New: tests |
| `cli/commands/install.test.ts` | Update: MCP recommendation tests |
