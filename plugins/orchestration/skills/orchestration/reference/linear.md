# Linear Integration

Guidelines for writing Linear issue updates, comments, and commit messages with Linear integration.

## Configuration

This skill reads from `.agents/config.json`:

```json
{
  "linear": {
    "workspace": "your-workspace-slug",
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

**Required**: Check that `linear.workspace` is configured before using Linear features. Ask user if missing.

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
- Code review approved
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
