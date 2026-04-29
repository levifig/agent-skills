# Linear Integration

Guidelines for writing Linear issue updates, comments, and commit messages with Linear integration.

## Contents

- Configuration
- MCP Server Naming
- Multi-Workspace Guidance
- Linear-Native Mode (Parent + Sub-Issues)
- The `spec` Label Convention
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

## Linear-Native Mode (Parent + Sub-Issues)

In Linear-native mode (`integrations.linear.enabled: true`), each spec
produces one parent **rollup issue** and N sub-issues under it.

```
[SPEC-024] Agent framework alignment      ← parent, label: `spec`
├── Split reviewer profile into reviewer/auditor  ← sub-issue, label: type/refactor
├── Harden MCP fallback path               ← sub-issue, label: type/feature
└── Migrate legacy task references         ← sub-issue, label: type/refactor
```

### Parent issue — what it is and isn't

The parent issue is a **dashboard anchor**, not a re-hosting of the spec.

- **Is:** a short summary (1–3 paragraphs) of the problem and solution
  direction + a link to the canonical spec file in the repo.
- **Is not:** a copy of the spec's Scope / Rabbit Holes / Open Questions /
  Risks sections. Those live in the local spec file and evolve there.

### Sample parent description

```markdown
## Summary
Align Loaf's agent profiles with the three-role model (implementer, reviewer, 
researcher). Consolidate historical profile variants and add tool-boundary 
tests so profiles can't drift without a test failing.

## Context
See `.agents/specs/SPEC-024-agent-framework-alignment.md` for full text,
council references, rabbit holes, and strategic tensions.

## Progress
Sub-issues track execution.
```

### Sub-issues

- Each sub-issue has `parentId` set to the parent issue ID.
- Cross-task dependencies use Linear's `blockedBy` field referencing sibling
  sub-issue IDs.
- Sub-issue labels describe the task itself (type, team, area), not the
  parent — don't label sub-issues with `spec`.

### Spec file remains canonical

Even with the parent in Linear, the local spec file is the source of truth
for:

- Problem statement and solution direction
- Scope / in-scope / out-of-scope / rabbit holes / no-gos
- Risks and open questions
- Council references and strategic tensions

When the spec evolves, edit the file and let git track it. The parent
issue's summary is a frozen entry point; only refresh it if the summary
itself (not the rabbit holes or risks) changes meaningfully.

## The `spec` Label Convention

Every spec-parent rollup issue carries a Linear label named `spec`. This lets
anyone in Linear filter for "all spec roots" across projects without having to
know which issues happen to be parents.

| Field | Value |
|-------|-------|
| Name | `spec` |
| Color | `#5e6ad2` (suggested; implementer may adjust) |
| Description | `Parent rollup issue representing a design spec tracked in the repo at .agents/specs/` |
| Scope | Workspace-scoped preferred; fall back to team-scoped if the MCP requires it |

### Who creates it

`/loaf:breakdown` creates the `spec` label on first Linear-native breakdown in a
workspace that doesn't already have it. Subsequent breakdowns reuse the
existing label. Log whether the label was created this run or already
existed — this matters for first-time setup.

### Sub-issues never carry `spec`

`spec` applies only to parents. A sub-issue describing a task uses its own
labels (type groups like `feature`/`bug`/`refactor`, team labels, area
labels) — never `spec`. This keeps the "filter for spec roots" query clean.

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
| `Phase 1: Discovery` | `Discovery - COMPLETE` |
| `Session file: .agents/sessions/...` | *(omit entirely)* |
| `Council decision: ...` | *(omit entirely)* |
| `Week 1 deliverables` | `Initial deliverables` |
| `BACK-52 Port Itera TEM...` | `BACK-52` *(Linear auto-expands)* |
| `/Users/name/Code/.../file.py` | `src/module/file.py` |

## Issue Description Format

```markdown
## Summary
Brief description of the work and its purpose.

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2
- [ ] Criterion 3

## Notes
Any relevant context (keep brief).
```

**Rules:**
- Concise and actionable
- Clear goal or objective
- No internal workflow references
- No mentions of agents, councils, or sessions

## Status Conventions

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
- Session file created (for non-trivial work)

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
- Use phase/stage/week terminology
- Include absolute file paths
- Duplicate issue titles after IDs
