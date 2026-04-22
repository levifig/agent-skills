# Session Management

## Contents
- Branch Management
- Team Routing
- Diagram Consideration
- Plan Mode Integration
- Linear Status Management
- Handoff State Requirements
- Timestamps for User Context
- Transcript Archival
- Task Completion

Detailed reference for session lifecycle management during implementation.

## Branch Management

**All new development work should happen on a dedicated branch.**

### Getting Branch Name

1. **If Linear issue exists**: Use the `branchName` field from `get_issue` response
   - Linear auto-generates branch names like `username/plt-123-issue-title`
   - These are pre-formatted and consistent with team conventions

2. **If no Linear issue**: Create branch name from session description
   - Format: `feature/<session-description>` or `fix/<session-description>`
   - Use kebab-case, keep it concise

### Branch Workflow

```bash
# 1. Check current branch status
git status

# 2. Create and checkout the branch (use Linear's branchName if available)
git checkout -b <branch-name>

# 3. Confirm branch creation
git branch --show-current
```

### Record in Session

Add branch info to session frontmatter:

```yaml
session:
  title: "..."
  branch: "username/plt-123-issue-title"  # Track the working branch
  linear_issue: "PLT-123"
```

**Important:** All implementation agents will work on this branch. The branch should be ready for PR when work completes.

---

## Team Routing

When creating Linear issues, suggest the appropriate team:

1. **Analyze task description** for keywords (see `linear-workflow` Skill)
2. **Check known_teams** in `.agents/loaf.json`
3. **If team is new to project**, ask user for confirmation:
   > "This task seems best suited for the **Security** team (matched: 'auth', 'vulnerability').
   > Security hasn't been used in this project yet. Add this team?"
4. **If user confirms**, add team to `known_teams` in config
5. **Create issue** with suggested team

### Team Suggestion Example

```
Task: "Fix authentication bypass vulnerability in API"
         |
Keywords matched: "authentication", "vulnerability", "API"
         |
Top suggestions:
  1. Security (score: 2) -- "authentication", "vulnerability"
  2. Backend (score: 1) -- "API"
         |
Suggest Security, confirm if new to project
```

Use Linear MCP's `list_teams` (if configured) to get all workspace teams for validation.

---

## Diagram Consideration

For multi-file or multi-service changes, consider adding architecture diagrams to the session file.

### When to Create Diagrams

| Scenario | Diagram Type |
|----------|--------------|
| Changes span 3+ services | Component diagram (interaction points) |
| Data flow modifications | Sequence diagram (trace data path) |
| Schema/model changes | ERD (table relationships) |
| New API endpoints | Sequence diagram (request/response) |
| State machine logic | State diagram (transitions) |

### Quick Check

Ask yourself:
1. Will this work touch multiple services or layers?
2. Is there a data flow that needs to be understood?
3. Would a visual help communicate the approach?

If yes to any, add an `## Architecture Diagrams` section to the session file.

### Diagram Template

```markdown
## Architecture Diagrams

### [Descriptive Name]

```mermaid
[Use flowchart, sequenceDiagram, erDiagram, or stateDiagram-v2]
```

**Purpose**: Why this diagram clarifies the work
**Files involved**: `path/to/file1.py`, `path/to/file2.py`
```

See `foundations` skill `reference/diagrams.md` for Mermaid syntax and best practices.

---

## Exploration Before Implementation

For complex tasks, explore before implementing:

### When to Explore First

- Task requires exploring unfamiliar codebase areas
- Multiple valid implementation approaches exist
- Dependencies between tasks need mapping
- User should approve approach before work begins

### Exploration Pattern

```
1. Use Task(Explore) or Task(Plan) to investigate codebase
2. Map existing patterns and conventions
3. Identify integration points
4. Document findings in session file
5. Present approach to user for approval before spawning
```

### Skip Exploration When

- Task is straightforward (single file, clear change)
- User has provided explicit detailed instructions
- Pattern is well-established in codebase

---

## Linear Status Management

**Keep Linear status synchronized with actual work state:**

| Work State | Linear Status (sub-issue) |
|------------|---------------------------|
| Session started | In Progress |
| Blocked/waiting for user | In Progress (add blocker comment) |
| Work completed | Done (or In Review if PR pending) |

### Parent rollup auto-close

In Linear-native mode, the **parent** rollup issue (labeled `spec`) is not
moved manually during sub-issue work. It flips to Done automatically when
the last sub-issue flips to Done, and only then. Procedure:

1. After moving a sub-issue to a `completed`-type state, call
   `list_issues` with `parent: <parent-id>`.
2. If every sub-issue is in a `completed`-type state, move the parent to
   `completed` via `update_issue`.
3. If any sub-issue is still in an open state (including `blocked`), the
   parent stays where it is — the spec is not done.

Never set the parent to In Progress manually — a parent in Linear-native
mode reflects a rollup of its sub-issues, not its own work.

### BlockedBy pre-flight

Before moving a sub-issue to In Progress, confirm every issue in its
`blockedBy` field is in a `completed`-type state. If not, refuse to start
and report the blockers. This is a hard gate in Linear-native mode —
never implement through open `blockedBy`.

---

## Handoff State Requirements

**The session file must ALWAYS be handoff-ready.** After every significant action:

1. Update `## Current State` to reflect what just happened
2. Update `orchestration.current_task` in frontmatter
3. Log completed agent work with outcomes
4. Ensure anyone could pick up the work immediately

---

## Timestamps for User Context

**Print the current date and timestamp when:**

- Waiting for user input or decision
- Completing a phase of work
- Encountering a blocker
- Session ends or pauses

Format: `[YYYY-MM-DD HH:MM UTC]`

Generate with: `date -u +"%Y-%m-%d %H:%M UTC"`

---

## Transcript Archival

After `/compact` or `/clear`, archive conversation transcripts for future reference.

### Process

1. **Get transcript path** from Claude Code output after compaction
2. **Create transcripts directory** if needed:
   ```bash
   mkdir -p .agents/transcripts
   ```
3. **Copy transcript** with descriptive name:
   ```bash
   cp /path/to/transcript.jsonl .agents/transcripts/YYYYMMDD-HHMMSS-description.jsonl
   ```
4. **Update session frontmatter**:
   ```yaml
   transcripts:
     - 20260123-143500-pre-compact.jsonl
   ```

### When to Archive

| Event | Action |
|-------|--------|
| Before `/compact` | Archive current transcript |
| Before `/clear` | Archive current transcript |
| Session end | Archive final transcript |

### Benefits

- **Audit trail** - Full history of decisions and work
- **Knowledge extraction** - Mining past sessions for patterns
- **Debugging** - Understanding how errors occurred
- **Training** - Learning from past sessions

---

## Task Completion

When a task-coupled session completes:

1. **Update task status** (local file or Linear sub-issue)
2. **Check spec progress:**
   - Local-tasks mode: list all tasks for the spec; if all done → mark
     spec `complete`, else spec stays `implementing`
   - Linear-native mode: query the parent rollup's sub-issues via
     `list_issues` with `parent: <parent-id>`; if all are `completed`-type,
     close the parent and mark the local spec `complete`, else both stay
     in flight
3. **Archive session** (standard process)

### Spec Completion Check

```bash
# Local-tasks mode: any open tasks for this spec?
loaf task list --spec SPEC-001 --status open --json

# Linear-native mode: query the Linear parent's sub-issues
# (via get_issue + list_issues with parent filter)
# The parent itself only flips to Done when every sub-issue is Done.
```

Never mark the local spec `complete` while its Linear parent still has
open sub-issues — the two sources of truth should agree on "done."
