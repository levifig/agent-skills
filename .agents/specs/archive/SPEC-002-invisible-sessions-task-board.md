---
id: SPEC-002
title: Invisible Sessions and Task Board
created: '2026-01-24T02:30:00.000Z'
status: complete
requirement: >-
  Simplify user workflow by making sessions invisible and adding a generated
  task board
---

# SPEC-002: Invisible Sessions and Task Board

## Problem Statement

Current workflow has friction:

1. **Session management overhead** - Users must understand and manage both tasks AND sessions. Sessions are implementation details that should be invisible.

2. **Command scoping confusion** - `/implement` works in Cursor/OpenCode but requires `/loaf:implement` in Claude Code. Suggestions in skill output don't account for target differences.

3. **No task visibility** - No single view of all tasks. Users must glob files or run shell commands to see task status.

4. **Resume friction** - `/resume` requires knowing session filenames. Users think in tasks, not sessions.

## Proposed Solution

### 1. Invisible Sessions

Sessions become an implementation detail:
- `/loaf:implement TASK-XXX` automatically creates/manages session
- Task file gains `session:` field pointing to active session
- Users never directly create or name sessions for implementation work
- Sessions still exist for non-implementation work (research, architecture)

### 2. Task-Based Resume

`/loaf:resume` accepts task arguments:
- `/loaf:resume TASK-002` → reads task → finds `session:` field → loads session
- `/loaf:resume <session-file>` still works for non-task sessions
- Session provides context recovery (resumption prompt, decisions, agent history)

### 3. Generated Task Board

`.agents/TASKS.md` generated via hook:
- **In Progress** section at top
- **To Do** section with priority subgroups (`### P1 - High`, etc.)
- **Completed** section at bottom, reverse chronological
- Markdown links to task files and specs (Obsidian-compatible)
- Regenerated on any task file change

### 4. Target-Aware Command Substitution

Build-time placeholders for target-specific commands:
- `{{IMPLEMENT_CMD}}` → `/loaf:implement` (Claude Code) or `/implement` (Cursor/OpenCode)
- `{{RESUME_CMD}}` → `/loaf:resume` (Claude Code) or `/resume` (Cursor/OpenCode)
- `{{ORCHESTRATE_CMD}}` → `/loaf:orchestrate` (Claude Code) or `/orchestrate` (Cursor/OpenCode)
- Processed by target transformers in `build/targets/*.js`

## Scope

### In Scope

**P1: Core Changes**
- Task board generation hook
- TASKS.md format and structure
- Build-time command substitution

**P2: Session Invisibility**
- Update `/implement` to auto-create sessions without user visibility
- Update task file to track session reference
- Update `/resume` to accept task arguments

**P3: Documentation**
- Update orchestration skill references
- Update command documentation

### Out of Scope (Rabbit Holes)

- Real-time board updates (hook-based is sufficient)
- Web UI for task board (markdown is enough)
- Task dependencies/blocking (keep simple for now)
- Kanban columns beyond status (no "Review" column, etc.)

### No-Gos

- Don't remove session files entirely - they serve agentic purposes
- Don't break existing `/resume <session-file>` functionality
- Don't add task board to git tracking (it's generated)

## Design Decisions

### TASKS.md Format

```markdown
# Tasks

## In Progress
- [TASK-001](tasks/TASK-001-verification-reference.md) - Create verification reference ([SPEC-001](specs/SPEC-001-loaf-self-sufficiency.md))

## To Do

### P1 - High
- [TASK-002](tasks/TASK-002-finishing-work-reference.md) - Finishing work reference ([SPEC-001](specs/SPEC-001-loaf-self-sufficiency.md))

### P2 - Normal
- [TASK-003](tasks/TASK-003-auto-fix-rules-reference.md) - Auto-fix rules reference ([SPEC-001](specs/SPEC-001-loaf-self-sufficiency.md))

### P3 - Low
- [TASK-006](tasks/TASK-006-lean-commands-refactor.md) - Lean commands refactor ([SPEC-001](specs/SPEC-001-loaf-self-sufficiency.md))

---

## Completed
- 2026-01-24 14:35 - [TASK-001](tasks/archive/2026-01/TASK-001-verification-reference.md) - Verification reference ([SPEC-001](specs/SPEC-001-loaf-self-sufficiency.md))
```

**Rules:**
- No tables, just lists with markdown links
- Priority groups as `###` under To Do
- Completed: `YYYY-MM-DD HH:MM - [TASK-XXX](path) - Title (SPEC-XXX)`
- Links are relative from `.agents/TASKS.md`
- Completed tasks link to archive location

### Task Directory Structure

```
.agents/tasks/
├── TASK-001-description.md      # Active tasks (root, not active/)
├── TASK-002-description.md
└── archive/
    └── 2026-01/                  # Archived by month
        └── TASK-000-setup.md
```

**Change from current:** Tasks move from `.agents/tasks/active/` to `.agents/tasks/` (root).

### Session Reference in Task

```yaml
# Task frontmatter
---
id: TASK-002
title: "Finishing work reference"
spec: SPEC-001
status: in_progress
session: 20260124-143000-finishing-work.md  # Added when /implement starts
---
```

### Build-Time Substitution

In skill/command files:
```markdown
Ready for next task:
{{IMPLEMENT_CMD}} TASK-002

Or run all tasks:
{{ORCHESTRATE_CMD}} SPEC-002
```

Target transformer (`build/targets/claude-code.js`):
```javascript
content = content.replace(/\{\{IMPLEMENT_CMD\}\}/g, '/loaf:implement');
content = content.replace(/\{\{RESUME_CMD\}\}/g, '/loaf:resume');
content = content.replace(/\{\{ORCHESTRATE_CMD\}\}/g, '/loaf:orchestrate');
```

Target transformer (`build/targets/cursor.js`):
```javascript
content = content.replace(/\{\{IMPLEMENT_CMD\}\}/g, '/implement');
content = content.replace(/\{\{RESUME_CMD\}\}/g, '/resume');
content = content.replace(/\{\{ORCHESTRATE_CMD\}\}/g, '/orchestrate');
```

### Hook for Board Generation

```yaml
# src/config/hooks.yaml
post-tool:
  - name: generate-task-board
    match:
      tool: Write
      path: ".agents/tasks/**/*.md"
    run: scripts/generate-task-board.sh
```

## Test Conditions

- [ ] `TASKS.md` generated correctly with all active tasks
- [ ] `TASKS.md` updates when task status changes
- [ ] `TASKS.md` links work in Obsidian/VS Code preview
- [ ] `/loaf:implement TASK-XXX` creates session without user interaction
- [ ] Task file gets `session:` field populated
- [ ] `/loaf:resume TASK-XXX` finds and loads correct session
- [ ] `/loaf:resume <session-file>` still works for non-task sessions
- [ ] Build output uses correct command form per target
- [ ] `npm run build` succeeds

## Files to Create

| File | Purpose |
|------|---------|
| `scripts/generate-task-board.sh` | Hook script for TASKS.md generation |
| `.agents/TASKS.md` | Generated task board (not in git) |

## Files to Modify

| File | Changes |
|------|---------|
| `src/commands/implement.md` | Auto-create session, update task with session reference |
| `src/commands/resume.md` | Accept task argument, find session from task |
| `src/config/hooks.yaml` | Add generate-task-board hook |
| `src/skills/orchestration/references/local-tasks.md` | Update directory structure |
| `src/skills/orchestration/references/sessions.md` | Document invisible sessions |
| `build/targets/*.js` | Add command substitution logic |

## Migration

1. Move existing tasks from `.agents/tasks/active/` to `.agents/tasks/`
2. Remove empty `.agents/tasks/active/` directory
3. Run board generation to create initial `TASKS.md`

## Circuit Breaker

At 50% appetite: If P1 (board generation + command substitution) not complete, defer P2 (session invisibility) to future spec.
