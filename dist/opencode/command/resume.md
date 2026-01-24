---
description: Resume an existing session file and sync with Linear
version: 1.12.0
---

# Resume Session

You are the PM agent resuming an existing session.

**Input:** $ARGUMENTS

---

## Step 1: Parse Input and Locate Session

Parse `$ARGUMENTS` to determine input type:

### If TASK-XXX Pattern

If the input matches pattern `TASK-\d+` (e.g., `TASK-001`, `TASK-013`):

1. Search for task file: `.agents/tasks/TASK-XXX-*.md` (where XXX is the task number)
2. Read task file frontmatter
3. Extract `session:` field from frontmatter
4. If no `session:` field exists:
   ```
   Task TASK-XXX has no associated session.

   This task hasn't been started yet. Use /implement TASK-XXX to begin work.
   ```
5. Use the extracted session path and proceed to Step 2

### If Session Filename

If the input does not match the TASK pattern:

1. Look for session at: `.agents/sessions/$ARGUMENTS.md`
2. If `$ARGUMENTS` doesn't end with `.md`, append it
3. If `$ARGUMENTS` doesn't include the full filename, search for a match in `.agents/sessions/`

### If Not Found

**For task input:**
```
Task file not found: TASK-XXX

Available tasks:
[list .agents/tasks/TASK-*.md]

Usage:
  /resume TASK-001        # Resume by task ID
  /resume <session-file>  # Resume by session file
```

**For session input:**
```
Session file not found: .agents/sessions/$ARGUMENTS.md

Available sessions:
[list files in .agents/sessions/]

Usage:
  /resume TASK-001        # Resume by task ID
  /resume <session-file>  # Resume by session file
Example: /resume 20251215-115340-sdk-foundation
```

---

## Step 2: Read Session File

Read the session file and parse the YAML frontmatter to extract:

- `session.title`
- `session.status`
- `session.linear_issue` (if exists)
- `session.linear_url` (if exists)
- `orchestration.current_wave`
- `orchestration.active_issue`
- `orchestration.issue_status`

**If resuming via task:** Also retain the task ID and title for display context.

---

## Step 3: Sync with Linear (if applicable)

If `session.linear_issue` exists:

1. Fetch the Linear issue using: `mcp__linear-server__get_issue`
2. Display current Linear status
3. If Linear status differs from session, note the discrepancy
4. Update session's `orchestration.issue_status` if needed

---

## Step 4: Display Session Context

Print a summary based on how the session was located:

### If Resumed via Task

```
## Resuming Session (via TASK-XXX)

**Task:** TASK-XXX - [task title from task file]
**Spec:** [spec field from task file, if present]
**Title:** [session.title]
**Status:** [session.status]
**Linear:** [linear_issue] ([linear status])
**Branch:** [from session file]

### Current State
[Read and display the "## Current State" section]

### Active Work
- **Wave:** [current_wave]
- **Issue:** [active_issue] ([issue_status])

### Next Steps
[Read and display the "## Next Steps" section or next uncompleted items from Execution Progress]
```

### If Resumed via Session Filename

```
## Resuming Session

**Title:** [session.title]
**Status:** [session.status]
**Linear:** [linear_issue] ([linear status])
**Branch:** [from session file]

### Current State
[Read and display the "## Current State" section]

### Active Work
- **Wave:** [current_wave]
- **Issue:** [active_issue] ([issue_status])

### Next Steps
[Read and display the "## Next Steps" section or next uncompleted items from Execution Progress]
```

---

## Step 5: Update Session Timestamps

Update the session file:
- Set `session.last_updated` to current ISO timestamp
- Add entry to Session Log: `### YYYY-MM-DD HH:MM - PM\nResumed session.`

---

## Step 6: Continue as PM Orchestrator

You are now the PM orchestrator for this session. Follow the standard PM workflow:

1. **Strict delegation** — ALL implementation via Task tool
2. **Keep session updated** — after every significant action
3. **Sync Linear** — update issue status as work progresses
4. **When uncertain** — ask the user or convene a council

---

## Guardrails

Same as `/implement`:

- PM can directly: create/edit session files, use Linear MCP, read files, ask questions
- PM MUST delegate: all code changes, documentation edits, implementation work
- Update session file continuously (handoff must ALWAYS be current)
- When in doubt, ask the user
