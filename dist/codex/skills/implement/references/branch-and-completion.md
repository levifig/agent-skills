# Branch and Completion

## Contents
- Branch Management
- Team Routing
- Diagram Consideration
- Exploration Before Implementation
- Linear Status Management
- Handoff Readiness
- Timestamps for User Context
- Task Completion

Detailed reference for branch setup, Linear routing, and completion during implementation.

## Branch Management

**All new development work should happen on a dedicated branch.**

### Getting Branch Name

1. **If Linear issue exists**: Use the `branchName` field from `get_issue` response
   - Linear auto-generates branch names like `username/plt-123-issue-title`
   - These are pre-formatted and consistent with team conventions

2. **If no Linear issue**: Create branch name from the work description
   - Format: `feature/<description>` or `fix/<description>`
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

**Important:** All implementation agents will work on this branch. The branch should be ready for PR when work completes. Journal entries are tagged with the observed branch automatically, so continuity stays branch-scoped.

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

For multi-file or multi-service changes, consider adding architecture diagrams to the linked spec, report, ADR, or implementation notes.

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

If yes to any, capture the diagram in a durable artifact such as a spec, report, ADR, or implementation note, and log the reference with `loaf journal log`.

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
4. Log findings with `loaf journal log` and reference durable artifacts
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
| Work begun | In Progress |
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

## Handoff Readiness

**The journal must ALWAYS be handoff-ready.** After every significant action:

1. Log what just happened with `loaf journal log`
2. Reference task/spec/report/commit IDs rather than duplicating long prose
3. Log completed agent work with outcomes
4. Ensure anyone could pick up the work immediately from `loaf journal recent`

---

## Timestamps for User Context

**Print the current date and timestamp when:**

- Waiting for user input or decision
- Completing a phase of work
- Encountering a blocker
- Wrapping up the conversation

Format: `[YYYY-MM-DD HH:MM UTC]`

Generate with: `date -u +"%Y-%m-%d %H:%M UTC"`

---

## Task Completion

When a task-coupled unit of work completes:

1. **Update task status** (local file or Linear sub-issue)
2. **Check spec progress:**
   - Local-tasks mode: list all tasks for the spec; if all done → mark
     spec `complete`, else spec stays `implementing`
   - Linear-native mode: query the parent rollup's sub-issues via
     `list_issues` with `parent: <parent-id>`; if all are `completed`-type,
     close the parent and mark the local spec `complete`, else both stay
     in flight
3. **Write a `wrap` journal entry** if the conversation holds synthesis worth
   saving (next steps, abandoned paths); skip it otherwise — nothing is
   "closed," a conversation that ends without a wrap leaves a valid journal

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
