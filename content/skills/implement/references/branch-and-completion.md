# Branch and Completion

## Contents
- Branch Management
- Team Routing
- Diagram Consideration
- Exploration Before Implementation
- Linear Status Management
- Handoff Readiness
- Timestamps for User Context
- Issue Completion

Detailed reference for branch setup, Linear routing, and completion during implementation.

## Branch Management

**All new development work should happen on a dedicated branch.**

### Getting Branch Name

`loaf issue start <ref>` is the claim. It creates `issue/<alias-or-id>` in lowercase (`issue/loaf-42`, id suffix when that name is claimed), a sibling worktree, and moves status to `active`.

Do not `git checkout -b` as a substitute for start. Check `loaf issue list --started` first. Never send two agents into the same worktree. Do not run `loaf issue stop` from inside that worktree.

### Branch Workflow

```bash
loaf issue list --started
loaf issue start <ref>
```

Work only in `started_worktree`. The branch should be ready for PR when work completes. Journal entries are tagged with the observed branch automatically.

---

## Team Routing

When creating Linear issues, suggest the appropriate team:

1. **Analyze task description** for keywords (see `linear-workflow` Skill)
2. **Check known_teams** in `.agents/loaf.json`
3. **If team is new to project**, ask user for confirmation:
   > "This task seems best suited for the **Security** team (matched: 'auth', 'vulnerability').
   > Security hasn't been used in this project yet. Add this team?"
4. **If user confirms**, add team to `known_teams` in config
5. **Create via `loaf issue new`** so identity can be delegated; do not create in Linear MCP and forget `loaf issue pull`

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

For multi-file or multi-service changes, consider adding architecture diagrams to the issue, a report, ADR, or implementation notes.

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

If yes to any, capture the diagram in a durable artifact such as a report, ADR, or implementation note, and log the reference with `loaf journal log`.

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
1. Spawn an explore or plan agent to investigate the codebase
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

**Keep Loaf status synchronized with actual work state.** Linear is an overlay (`loaf issue pull` / `push` / `reconcile`); never drive Loaf status from Linear MCP tools.

| Work State | Loaf status |
|------------|-------------|
| Work begun | `active` via `loaf issue start` |
| Blocked/waiting | Stay `active`; log `block(scope)` and leave a Linear comment if the overlay is on |
| Work landed | `done` via `loaf issue status <ref> done` (usually ship), then `loaf issue stop <ref>` |

### Parent vs children

Parents with children are not the implementation target. Dispatch leaf delivery children on `loaf issue frontier`. A parent is not marked `done` because a child landed.

`loaf issue link A blocks B` is the sequencing edge. An issue with an open predecessor does not appear on the frontier. Do not start a blocked successor.

### Blocked-by pre-flight

Before `loaf issue start`, confirm the ref is on `loaf issue frontier`. If it is blocked, refuse and report the predecessors. Never implement through an open `blocks` edge.

---

## Handoff Readiness

**The journal must ALWAYS be handoff-ready.** After every significant action:

1. Log what just happened with `loaf journal log`
2. Reference issue/report/commit IDs rather than duplicating long prose
3. Log completed agent work with outcomes
4. Ensure anyone could pick up the work immediately from `loaf journal recent`

---

## Timestamps for User Context

**Print the current date and timestamp when:**

- Waiting for user input or decision
- Completing a coherent unit of work
- Encountering a blocker
- Wrapping up the conversation

Format: `[YYYY-MM-DD HH:MM UTC]`

Generate with: `date -u +"%Y-%m-%d %H:%M UTC"`

---

## Issue Completion

When an issue-coupled unit of work completes:

1. **Open or update the PR** with body `loaf issue render <ref>` — no manual editing
2. **Land via ship** — review definition of done, `loaf issue verify <ref>`, squash merge, then `loaf issue status <ref> done` and `loaf issue stop <ref>`
3. **Write a `wrap` journal entry** if the conversation holds synthesis worth saving (next steps, abandoned paths); skip it otherwise — nothing is "closed," a conversation that ends without a wrap leaves a valid journal

```bash
loaf issue show <ref>
loaf issue tree <ref>
loaf issue list --started
```

Do not mark a parent `done` while delivery children are still open. Do not flip Loaf status from Linear MCP tools; use `loaf issue reconcile` if the overlay has drifted.
