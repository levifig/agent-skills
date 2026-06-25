# Agent Delegation

## Contents

- Core Principle
- What Orchestrator Does vs Delegates
- Agent Capability Matrix
- Delegation Decision Tree
- Spawn Patterns
- Anti-Patterns
- Agent Access Hierarchy

## Core Principle

**You are the orchestrator, not the implementer.** All code changes, documentation edits, and implementation work MUST be delegated to specialized agents.

## What Orchestrator Does vs Delegates

### Orchestrator Does Directly

| Action | Tool |
|--------|------|
| Create/edit session files | Write |
| Create/edit council files | Write |
| Track tasks | task list or chat checklist |
| Manage external issues | Linear, GitHub |
| Read files for context | Read, Grep, Glob |
| Ask clarifying questions | built-in chat clarification (OpenCode: `question`) |
| Assign background agent work | task list or chat checklist (background agent read via task list or chat checklist) |

### Orchestrator MUST Delegate

- Any code in `backend/`, `frontend/`, `src/`, `tests/`
- Documentation in `docs/`
- Configuration files (`.yaml`, `.json`, `.toml`)
- Infrastructure files (`Dockerfile`, `docker-compose.yml`)
- Database migrations
- Test files

**NO EXCEPTIONS** - even "trivial" 1-line fixes go through specialized agents.

## Agent Capability Matrix

### Implementation Profiles

| Profile | Focus | Skills to Load |
|---------|-------|---------------|
| `implementer` | Backend code | Language skill (python, ruby, go, typescript) + domain skills |
| `implementer` | Frontend code | typescript-development + interface-design |
| `implementer` | Ruby on Rails | ruby-development |
| `implementer` | Database | database-design + language skill |
| `implementer` | Infrastructure | infrastructure-management |

### Quality Assurance Profiles

| Profile | Focus | Skills to Load |
|---------|-------|---------------|
| `implementer` | Tests | foundations + language skill |
| `implementer` | Security | foundations + language skill |

### Review & Advisory Profiles

| Profile | Focus | Skills to Load |
|---------|-------|---------------|
| `reviewer` | Code review | relevant domain skills |
| `reviewer` | UI/UX review | interface-design |
| `researcher` | Research | relevant domain skills |

## Delegation Decision Tree

```
What type of work is needed?

|-- Code Implementation
|   |-- Python/FastAPI/Backend --> implementer (language skill)
|   |-- React/Next.js/Frontend --> implementer (typescript-development + interface-design)
|   |-- Ruby on Rails --> implementer (ruby-development)
|   +-- Database Schema/Migrations --> implementer (database-design)

|-- Infrastructure & Operations
|   |-- Docker/K8s/CI/CD --> implementer (infrastructure-management)
|   +-- Database Performance --> implementer (database-design)

|-- Quality Assurance
|   |-- Test Implementation --> implementer (foundations + language skill)
|   +-- Security Audit --> implementer (foundations)

|-- Code Review
|   |-- Backend Review --> reviewer (language skill)
|   +-- Frontend Review --> reviewer (typescript-development)

|-- Documentation & Design
|   |-- Technical Documentation --> implementer (foundations)
|   |-- UI/UX Design --> reviewer (interface-design)
|   +-- Product Requirements --> researcher

+-- Complex Decision?
    +-- Council (5-7 background agent, odd number)
```

## Spawn Patterns

**OpenCode requirement:** Interview the user with the `question` tool before drafting a plan or research strategy.

### Sequential (Dependencies)

Use when output of one agent is input to another:

```python
# Step 1: Schema first
Agent(agent_type="implementer", prompt="Create users table... Follow database-design skill.")

# Wait for completion

# Step 2: Implementation uses schema
Agent(agent_type="implementer", prompt="Implement user service... Follow python-development skill.")

# Wait for completion

# Step 3: Tests use implementation
Agent(agent_type="implementer", prompt="Write user tests... Follow foundations + python-development skills.")
```

**Common sequences:**
- Schema -> Code -> Tests
- Design -> Implementation -> Code review -> Testing -> Security
- Implementation -> Tests -> Code review -> Security

### Parallel (Independent)

Use when work is truly independent:

```python
# Both can run simultaneously
Agent(agent_type="implementer", prompt="Implement API... Follow python-development skill.")
Agent(agent_type="implementer", prompt="Build UI... Follow typescript-development + interface-design skills.")
```

**Requirements for parallel:**
- No dependencies between tasks
- Defined API contract (for API + UI)
- Separate files/components

### Spawning Best Practices

1. **Be specific in prompts** - Include file paths, requirements, constraints
2. **One concern per agent** - Don't ask a backend implementer to also write tests
3. **Include context** - Session file, issue ID, previous outcomes
4. **Reference session** - `Session: .agents/sessions/YYYYMMDD-HHMMSS-name.md`
5. **Include skill hints** - Name the skills that should guide the agent's work

### Skill Hints

When delegating, explicitly name the skills that should guide the agent's work. This creates deterministic contracts instead of leaving skill selection to the model's discretion.

```python
# Explicit: agent knows which patterns to follow
prompt="... Follow python-development skill for FastAPI conventions.
        Follow database-design skill for schema decisions."

# Implicit: agent may or may not pick the right skill
prompt="... Build the API endpoint."
```

**When to include skill hints:**
- The task spans multiple skill domains (e.g., backend code + database schema)
- You want a specific skill's conventions followed (e.g., "follow ruby-development, not python-development")
- The agent has access to multiple language skills and the choice matters

**When to skip:**
- Single-domain tasks where the agent only has one relevant skill
- The task description already clearly implies the domain

### Example Task() Call

```python
Task(
    agent_type="implementer",
    prompt="""
    Implement POST /api/v1/users endpoint.

    Requirements:
    - Validate email format
    - Hash password with bcrypt
    - Return 201 with user object
    - Handle duplicate email (409)

    Follow python-development skill for FastAPI conventions.
    Follow foundations skill for commit and security patterns.

    Files:
    - src/api/users.py
    - src/models/user.py

    Session: .agents/sessions/20251210-143052-user-registration.md
    Linear: BACK-123
    """
)
```

## Anti-Patterns

| Anti-Pattern | Better Approach |
|--------------|-----------------|
| Orchestrator implementing code | Orchestrator delegates, always use specialized agents |
| Asking backend implementer for React | Spawn implementer with frontend skills |
| Single agent for database + backend + tests | Sequential: implementer (database-design), implementer (language skill), implementer (foundations) |
| Parallel spawns with hidden dependencies | Make dependencies explicit, spawn sequentially |
| Spawning without session context | Create session first, reference in prompts |
| Council for simple decisions | Single agent or orchestrator judgment |

## Agent Access Hierarchy

| Agent Type | External Issue Access | Reports To |
|------------|----------------------|------------|
| Orchestrator | Read/Write | User |
| Implementation agents | None | Orchestrator (via session) |
| Review agents (backend/frontend devs) | None | Orchestrator (via session) |
| Product agent | Read-only | Orchestrator |

**Key**: Only orchestrator writes to external issue trackers. All other agents report through session files.
