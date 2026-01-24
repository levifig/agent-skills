---
description: Decompose specs into atomic work items (Linear or local)
version: 1.13.2
---

# Breakdown Command

Decompose specifications into atomic, implementable tasks.

**Input:** $ARGUMENTS

---

## Purpose

Tasks are the smallest unit of work that can be:
- Assigned to a single specialized agent
- Completed within model context limits
- Verified with a clear done condition

See `orchestration/local-tasks` reference for task format and abstraction layer.

---

## Task Breakdown Philosophy: Separation of Concerns

**The primary principle for task breakdown is separation of concerns.**

### Right-Sizing Rules

| Rule | Guideline |
|------|-----------|
| **One agent type** | Task should be completable by ONE agent (backend-dev, frontend-dev, dba, qa, devops) |
| **One concern** | Task touches one layer, one service, or one component |
| **Context-appropriate** | Small enough to fit in model context with room for exploration |
| **Not over-fragmented** | Don't split what naturally belongs together |

### The Right Size Test

Ask these questions:
1. Can a single specialized agent complete this? → If no, split by agent type
2. Does it touch multiple unrelated concerns? → If yes, split by concern
3. Will the agent need to hold too much context? → If yes, split into phases
4. Am I splitting just to have more tasks? → If yes, merge back

### Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Split backend + tests into separate tasks | Keep tests with the code they test (same agent) |
| Create a task per file | Group files by concern/feature |
| Split a single function across tasks | Keep atomic changes together |
| Separate "implement" and "verify" tasks | Every task includes its own verification |

---

## Task Backend Detection

Check `.agents/loaf.yaml` for backend configuration:

```yaml
task_management:
  backend: linear  # or "local"
```

**If no config exists:** Ask user which backend to use.

---

## Process

### Step 1: Parse Input

`$ARGUMENTS` should reference a spec.

Examples:
- "SPEC-001"
- "user auth spec"
- "docs/specs/SPEC-001-user-auth.md"

**If unclear:** List available specs and ask which to break down.

### Step 2: Read the Spec

1. Find spec file in `docs/specs/`
2. Read full spec content
3. Extract:
   - Test conditions (become task acceptance criteria)
   - Scope (what's in/out)
   - Implementation notes (technical context)
   - Appetite (guides task sizing)

### Step 3: Identify Task Boundaries

Break down by concern:

| Concern | Task Type |
|---------|-----------|
| Data model changes | DBA task |
| Backend logic | Backend task |
| API endpoints | Backend task |
| UI components | Frontend task |
| Tests | QA task |
| Infrastructure | DevOps task |

**Rules:**
- One concern per task
- Tasks can run in parallel if independent
- Tasks have explicit dependencies if sequential

### Step 4: Interview for Priorities

Ask:
- Which parts are highest priority?
- Any tasks that must be done first?
- Preferred order of implementation?

### Step 5: Draft Task List

For each task:

```yaml
---
id: TASK-XXX
title: [Clear action]
spec: SPEC-001
status: todo
priority: P2
files:
  - [likely file 1]
  - [likely file 2]
verify: [command to verify]
done: [observable outcome]
---

## Description
[What needs to be done]

## Acceptance Criteria
- [ ] [Criterion 1]
- [ ] [Criterion 2]

## Context
See SPEC-001 for full context.
```

### Step 6: Present Task Breakdown

```markdown
## Proposed Tasks for SPEC-001

### TASK-001: [Title]
- **Priority:** P1
- **Verify:** [command]
- **Done:** [outcome]
- **Files:** [list]

### TASK-002: [Title]
- **Priority:** P2
- **Depends on:** TASK-001
- **Verify:** [command]
- **Done:** [outcome]

### TASK-003: [Title]
- **Priority:** P2
- **Verify:** [command]
- **Done:** [outcome]

---

**Task dependencies:**
```
TASK-001 → TASK-002 (sequential)
TASK-003 (can run in parallel)
```

**Approve this breakdown?**
```

### Step 7: Await Approval

**Do NOT create tasks without explicit approval.**

User may:
- Approve as-is
- Adjust priorities
- Combine/split tasks
- Add missing tasks
- Change dependencies

### Step 8: Create Tasks

Based on backend:

#### Linear Backend

```
For each task:
1. Create Linear issue with title, description
2. Set labels from spec
3. Link to spec (as attachment or in description)
4. Set priority
5. Record issue ID
```

#### Local Backend

```bash
# Create task directory if needed
mkdir -p .agents/tasks

# Generate task ID
next_id=$(find_next_task_id)

# Create task file
# .agents/tasks/TASK-{id}-{slug}.md
```

### Step 9: Update Spec Status

After tasks created:

```yaml
# In spec frontmatter
status: implementing
```

### Step 10: Announce Completion

```markdown
## Tasks Created for SPEC-001

| ID | Title | Priority | Backend |
|----|-------|----------|---------|
| TASK-001 | OAuth Provider Integration | P1 | [Linear/Local] |
| TASK-002 | Session Management | P2 | [Linear/Local] |
| TASK-003 | Login UI Components | P2 | [Linear/Local] |

**Spec status:** implementing

**Next:** Use `/implement TASK-001` to begin work, or `/orchestrate SPEC-001` to run all tasks.
```

---

## Task ID Generation

### Linear
IDs come from Linear (e.g., `PLT-123`).

### Local
Sequential numbering:

```bash
# Find next available number
find_next_task_id() {
  local max=$(ls .agents/tasks/ .agents/tasks/archive/*/ 2>/dev/null | \
    grep -oE 'TASK-[0-9]+' | \
    sort -t- -k2 -n | \
    tail -1 | \
    awk -F- '{print $2}')
  echo $((${max:-0} + 1))
}
```

---

## Task Sizing Guide

| Size | Characteristics | Action |
|------|-----------------|--------|
| **Right** | One concern, one agent type, fits in context | Good to go |
| **Too small** | Artificially split, fragment of a concern | Merge with related task |
| **Too large** | Multiple concerns or agent types | Split by concern |

### Sizing by Agent Type

| Agent | Typical Task Scope |
|-------|-------------------|
| `backend-dev` | One service/module, its tests, its docs |
| `frontend-dev` | One component/page, its tests, its styles |
| `dba` | One migration, related schema changes |
| `qa` | Test suite for one feature/area |
| `devops` | One infrastructure concern (CI, deploy, config) |

**If task requires multiple agent types:** Split into separate tasks with dependencies.

---

## Priority Levels

| Priority | Meaning | Response |
|----------|---------|----------|
| P0 | Urgent/blocking | Drop everything |
| P1 | High | Work next |
| P2 | Normal | Scheduled work |
| P3 | Low | When time permits |

Default: P2 (normal).

---

## Verification Commands

Each task needs a `verify` command:

| Task Type | Example Verification |
|-----------|---------------------|
| Backend Python | `pytest tests/auth/test_oauth.py` |
| Backend Rails | `rails test test/models/session_test.rb` |
| Frontend React | `npm run test -- auth.test.tsx` |
| API endpoint | `curl -X POST localhost:3000/auth/login` |
| Database | `psql -c "SELECT * FROM users LIMIT 1"` |

---

## Done Conditions

Write clear, observable outcomes:

**Good:**
- "OAuth flow completes for Google and GitHub"
- "Session persists across page refresh"
- "All auth tests pass"

**Bad:**
- "Auth works" (too vague)
- "Code is clean" (subjective)
- "Implementation done" (not verifiable)

---

## Guardrails

1. **One concern per task** - Don't mix backend + frontend
2. **Clear verification** - How to prove it works
3. **Observable done condition** - Not subjective
4. **File hints** - Help session know where to look
5. **Get approval** - Don't create without confirmation
6. **Update spec status** - Mark as implementing

---

## Common Patterns

### API Feature

```
TASK-001: Database migration (DBA)
TASK-002: API endpoint implementation (Backend)
TASK-003: Unit tests for endpoint (QA)
TASK-004: API documentation (Backend)
```

### UI Feature

```
TASK-001: Component implementation (Frontend)
TASK-002: State management (Frontend)
TASK-003: Component tests (QA)
TASK-004: E2E tests (QA)
```

### Full Stack Feature

```
TASK-001: Database schema (DBA)
TASK-002: API endpoints (Backend)
TASK-003: Backend tests (QA)
TASK-004: UI components (Frontend)
TASK-005: Frontend tests (QA)
TASK-006: E2E integration (QA)
```

---

## Related Commands

- `/shape` — Create specs that get broken down
- `/implement` — Start session for a task
- `/orchestrate` — Run multiple tasks
