# Linear Integration

Guidelines for writing Linear issue updates, comments, and commit messages with Linear integration.

## Contents

- Configuration
- MCP Server Naming
- Multi-Workspace Guidance
- Identity Adapter
- Progress Update Format
- Issue Description Format
- Status Conventions
- Magic Words (Git Integration)
- Branch Naming
- Team Routing
- When to Create Issues
- Blocker Format
- Critical Rules

## Configuration

This skill reads from `.agents/loaf.json`:

```json
{
  "integrations": {
    "linear": { "enabled": true }
  },
  "linear": {
    "workspace": "your-workspace-slug",
    "mcp_server_name": "linear-enline",
    "project": { "id": "...", "name": "..." },
    "known_teams": [{ "name": "Backend", "id": "..." }],
    "default_team": "Platform",
    "team_keywords": {
      "Security": ["security", "auth", "vulnerability"],
      "Backend": ["api", "database", "service"]
    }
  }
}
```

**Required**: Check that `integrations.linear.enabled` is `true` and that
`linear.workspace` is configured before using Linear features. Ask user if missing.

Linear MCP uses `https://mcp.linear.app/mcp` (SSE deprecated) and includes tools for initiatives, initiative updates, project milestones, project updates, and project labels.

## MCP Server Naming

Skills invoke Linear MCP tools via a configured server name rather than a
hard-coded one. Set `linear.mcp_server_name` in `.agents/loaf.json` (e.g.,
`"linear-enline"`). Skills reference this when looking up MCP tools so the
same skill content works across workspaces without edits.

If `linear.mcp_server_name` is unset, default to `"linear"` and warn the user
once per session that an explicit name is recommended for multi-workspace
setups.

## Multi-Workspace Guidance

Users with multiple Linear workspaces (e.g., employer + personal) should
configure **project-scoped** MCP entries with distinct names rather than
user-scoped entries:

```jsonc
// <project>/.mcp.json
{
  "mcpServers": {
    "linear-enline": {
      "url": "https://mcp.linear.app/mcp",
      "workspace_hint": "enline"
    }
  }
}

// <another-project>/.mcp.json
{
  "mcpServers": {
    "linear-personal": {
      "url": "https://mcp.linear.app/mcp",
      "workspace_hint": "personal"
    }
  }
}
```

| Trade-off | Project-scoped (recommended) | User-scoped |
|-----------|------------------------------|-------------|
| Cross-project leakage | No — each project only sees its own workspace | Yes — any project can hit any workspace |
| Auth per workspace | Each project authenticates independently | One auth for all |
| Config discoverability | Lives with the project in version control | Hidden in user config |
| Setup overhead | Per-project (small) | Once (but conflates workspaces) |

Match the `linear.mcp_server_name` in each project's `.agents/loaf.json` to
the name used in that project's `.mcp.json`. That way the Loaf skills invoke
the right workspace automatically.

## Identity Adapter

When `issue_identity.authority = linear`, Linear owns identity, title, status,
and assignment. Loaf owns shaping state: body, definition-of-done criteria,
claims, and the started worktree. The Loaf issue is the work unit. Linear MCP
is an overlay — never drive Loaf status from MCP tools.

`loaf issue new` delegates identity: Linear mints the identifier, and that
key becomes the local alias. The local counter is not advanced. If Linear is
offline, refuse — capture via `loaf spark` or `loaf idea`. Do not mint a
local alias as a fallback.

If Linear created an issue but the local bind failed, adopt it:

```text
loaf issue pull <linear-key>
loaf issue pull <linear-key> --tree
```

`--tree` also adopts the sub-issue tree with parent edges intact.

### Commands

```text
loaf issue pull <linear-key> [--tree] [--json]
loaf issue push <ref> [--json]
loaf issue reconcile [<ref>] [--take-local|--take-tracker] [--json]
```

| Command | Writes? | What it does |
|---------|---------|----------------|
| `loaf issue pull` | Yes | Adopt an existing Linear issue as a local row. The Linear key becomes the alias |
| `loaf issue push` | Yes | Write `loaf issue render` as the Linear description. Status is written only when the local status event is newer than the tracker. Never renames the Linear issue |
| `loaf issue reconcile` | Yes with a take flag | Compare local and Linear. Title drift updates the local title (tracker wins). Status drift is reported; `--take-local` or `--take-tracker` resolves it. Description drift is reported only |

Do not create records with `loaf task` or `loaf spec`. Parent/child structure
is `loaf issue promote` (or `loaf issue new --parent`), not a `spec`-labeled
Linear rollup.

## Progress Update Format

```markdown
## Progress
- [x] Completed item description
- [x] Another completed item
- [ ] Pending item description

## Blockers
None currently.
```

### Format Rules

1. **Checkboxes only** - No emoji, no bullets without checkboxes
2. **Outcome-focused** - "Added user endpoint" not "Wrote code for user endpoint"
3. **Self-contained** - Reader shouldn't need local context
4. **Succinct** - Brief descriptions, no verbose explanations

### Common Mistakes

| Wrong | Correct |
|-------|---------|
| `Working on API` | `- [ ] API implementation` |
| `Done with schema` | `- [x] Schema updated` |
| `Discovery: IN PROGRESS` | `Discovery: COMPLETE` |
| `Journal entry: ...` | *(omit entirely)* |
| `Council decision: ...` | *(omit entirely)* |
| `Week 1 deliverables` | `Initial deliverables` |
| `BACK-52 Port Itera TEM...` | `BACK-52` *(Linear auto-expands)* |
| `/Users/name/Code/.../file.py` | `src/module/file.py` |

## Issue Description Format

The Linear description is `loaf issue push` output — `loaf issue render`, not a hand-authored summary. Do not paste a competing description over the render.

Comments (not the description) still follow the progress-update format above.

**Rules:**
- Concise and actionable
- Clear goal or objective
- No internal workflow references
- No mentions of agents, councils, or sessions

## Status Conventions

Loaf status is `loaf issue status`. Linear status is the tracker's. Resolve drift with `loaf issue reconcile` (`--take-local` or `--take-tracker`). Do not flip Loaf status from Linear MCP tools.

| State | When to Use |
|-------|-------------|
| **Backlog** | Issue created, not started |
| **In Progress** | Work actively started, developer assigned |
| **In Review** | Implementation complete, PR created |
| **Done** | Merged and verified |
| **Blocked** | External dependency prevents progress |

### Transition Criteria

**Backlog -> In Progress:**
- Work has actively started
- Developer is assigned
- Invocation logged to the project journal (for non-trivial work)

**In Progress -> In Review:**
- Implementation is complete
- Tests pass
- PR created

**In Review -> Done:**
- Backend/frontend code review approved
- CI passes
- Merged to main branch

## Magic Words (Git Integration)

### Closing Keywords

Auto-close issue when commit is merged:

| Keyword | Use Case |
|---------|----------|
| `Closes BACK-XXX` | Features, tasks, enhancements |
| `Fixes BACK-XXX` | Bug fixes only |
| `Resolves BACK-XXX` | Alternative to Closes |

### Non-Closing Keywords

Link commit without closing:

| Keyword | Use Case |
|---------|----------|
| `Refs BACK-XXX` | Reference only |
| `Part of BACK-XXX` | Partial work |

### Commit Message Format

```
feat: add new feature

Brief description of the change.

Closes BACK-123
```

**Rules:**
- One closing keyword per issue
- Use the right keyword (`Fixes` = bug, `Closes` = everything else)
- Put keywords in commit body, not subject
- Issue ID only (Linear auto-expands)

### Multiple Issues

```
feat: implement authentication system

Added login, logout, and session management.

Closes BACK-123
Refs BACK-124, BACK-125
```

## Branch Naming

```
TEAM-123-description
```

Examples:
- `PLT-123-add-weather-fallback`
- `BCK-456-fix-batch-processor`

## Team Routing

Teams are suggested contextually based on task description.

### Flow

1. **Analyze task** - Match keywords against `team_keywords` config
2. **Suggest team** - Highest-scoring team becomes suggestion
3. **Check if known** - If team hasn't been used, ask for confirmation
4. **Auto-learn** - When user confirms, team is added to `known_teams`

Use `scripts/suggest-team.py "task desc"` to get suggestions.

## When to Create Issues

Create through `loaf issue new` so identity can be delegated. Do not create in Linear MCP and then forget to `loaf issue pull`.

| Action | Create Issue? |
|--------|---------------|
| Features, bugs, refactoring | Yes |
| Infrastructure changes | Yes |
| Multi-file changes | Yes |
| Typo fixes | No |
| Quick clarifications | No |
| Single-line tweaks | No |
| Uncertain | Ask user |

## Blocker Format

```markdown
## Blockers

### [Blocker Title]
**Impact**: What's blocked by this
**Needed**: What would unblock it
**ETA**: If known, otherwise "TBD"
```

## Critical Rules

### DO
- Use Markdown checkboxes for progress lists
- Keep updates succinct and outcome-focused
- Make updates self-contained
- Use issue ID only (Linear auto-expands titles)

### DON'T
- Use emoji in progress lists
- Reference local files (sessions, councils, plans)
- Use numbered development-stage terminology
- Include absolute file paths
- Duplicate issue titles after IDs
