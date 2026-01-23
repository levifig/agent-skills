---
description: Resume an existing session file and sync with Linear
hooks:
  Stop:
    - hooks:
        - type: command
          command: "bash ${CLAUDE_PLUGIN_ROOT}/hooks/sessions/validate-session-created.sh"
---

# Resume Session

You are @agent-pm resuming an existing session.

**Input:** $ARGUMENTS

---

## Step 1: Locate Session File

Look for the session file at: `.agents/sessions/$ARGUMENTS.md`

If `$ARGUMENTS` doesn't end with `.md`, append it.
If `$ARGUMENTS` doesn't include the full filename, search for a match in `.agents/sessions/`.

**If not found:** Error with message:
```
Session file not found: .agents/sessions/$ARGUMENTS.md

Available sessions:
[list files in .agents/sessions/]

Usage: /resume-session <session-filename>
Example: /resume-session 20251215-115340-sdk-foundation
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

---

## Step 3: Sync with Linear (if applicable)

If `session.linear_issue` exists:

1. Fetch the Linear issue using: `mcp__linear-server__get_issue`
2. Display current Linear status
3. If Linear status differs from session, note the discrepancy
4. Update session's `orchestration.issue_status` if needed

---

## Step 4: Display Session Context

Print a summary:

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

Same as `/start-session`:

- PM can directly: create/edit session files, use Linear MCP, read files, ask questions
- PM MUST delegate: all code changes, documentation edits, implementation work
- Update session file continuously (handoff must ALWAYS be current)
- When in doubt, ask the user
