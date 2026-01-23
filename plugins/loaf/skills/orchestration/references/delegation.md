# Agent Delegation

## Core Principle

**PM is the orchestrator, not the implementer.** All code changes, documentation edits, and implementation work MUST be delegated to specialized agents.

## What PM Does vs Delegates

### PM Does Directly

| Action | Tool |
|--------|------|
| Create/edit session files | Write |
| Create/edit council files | Write |
| Track tasks | TodoWrite/TodoRead |
| Manage external issues | Linear, GitHub |
| Read files for context | Read, Grep, Glob |
| Ask clarifying questions | AskUserQuestion (OpenCode: `question`) |
| Assign subagent work | TodoWrite (subagents read via TodoRead) |

### PM MUST Delegate

- Any code in `backend/`, `frontend/`, `src/`, `tests/`
- Documentation in `docs/`
- Configuration files (`.yaml`, `.json`, `.toml`)
- Infrastructure files (`Dockerfile`, `docker-compose.yml`)
- Database migrations
- Test files

**NO EXCEPTIONS** - even "trivial" 1-line fixes go through specialized agents.

## Agent Capability Matrix

### Implementation Agents

| Agent | Focus | Use For |
|-------|-------|---------|
| `backend-dev` | Backend | Services, APIs, business logic, backend code review |
| `frontend-dev` | Frontend | UI components, state management, frontend code review |
| `rails-dev` | Ruby on Rails | Controllers, models, views, Hotwire/Stimulus, ActiveRecord |
| `dba` | Database | Schema design, migrations, indexes, query optimization |
| `devops` | Infrastructure | Docker, Kubernetes, CI/CD, monitoring |

### Quality Assurance Agents

| Agent | Focus | Use For |
|-------|-------|---------|
| `qa` | Tests | Unit tests, integration tests, fixtures, coverage |
| `security` | Security | Audits, vulnerabilities, OWASP, secrets, threat modeling |

### Documentation & Planning

| Agent | Focus | Use For |
|-------|-------|---------|
| `docs` | Documentation | API docs, ADRs, READMEs, guides |
| `product` | Planning | Requirements, roadmaps, feature specs, user stories |
| `design` | UI/UX | Interface design, accessibility, design systems |

## Delegation Decision Tree

```
What type of work is needed?

|-- Code Implementation
|   |-- Python/FastAPI/Backend --> backend-dev
|   |-- React/Next.js/Frontend --> frontend-dev
|   |-- Ruby on Rails --> rails-dev
|   +-- Database Schema/Migrations --> dba

|-- Infrastructure & Operations
|   |-- Docker/K8s/CI/CD --> devops
|   +-- Database Performance --> dba

|-- Quality Assurance
|   |-- Test Implementation --> qa
|   +-- Security Audit --> security

|-- Code Review (Domain Dev)
|   |-- Backend Review --> backend-dev
|   +-- Frontend Review --> frontend-dev

|-- Documentation & Design
|   |-- Technical Documentation --> docs
|   |-- UI/UX Design --> design
|   +-- Product Requirements --> product

+-- Complex Decision?
    +-- Council (5-7 agents, odd number)
```

## Spawn Patterns

**OpenCode requirement:** Interview the user with the `question` tool before drafting a plan or research strategy.

### Sequential (Dependencies)

Use when output of one agent is input to another:

```python
# Step 1: Schema first
Task(subagent_type="dba", prompt="Create users table...")

# Wait for completion

# Step 2: Implementation uses schema
Task(subagent_type="backend-dev", prompt="Implement user service...")

# Wait for completion

# Step 3: Tests use implementation
Task(subagent_type="qa", prompt="Write user tests...")
```

**Common sequences:**
- Schema -> Code -> Tests
- Design -> Implementation -> Code review -> Testing -> Security
- Implementation -> Tests -> Code review -> Security

### Parallel (Independent)

Use when work is truly independent:

```python
# Both can run simultaneously
Task(subagent_type="backend-dev", prompt="Implement API...")
Task(subagent_type="frontend-dev", prompt="Build UI...")
```

**Requirements for parallel:**
- No dependencies between tasks
- Defined API contract (for API + UI)
- Separate files/components

### Spawning Best Practices

1. **Be specific in prompts** - Include file paths, requirements, constraints
2. **One concern per agent** - Don't ask backend-dev to also write tests
3. **Include context** - Session file, issue ID, previous outcomes
4. **Reference session** - `Session: .agents/sessions/YYYYMMDD-HHMMSS-name.md`

### Example Task() Call

```python
Task(
    subagent_type="backend-dev",
    prompt="""
    Implement POST /api/v1/users endpoint.

    Requirements:
    - Validate email format
    - Hash password with bcrypt
    - Return 201 with user object
    - Handle duplicate email (409)

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
| PM implementing code | PM orchestrates, always delegate |
| Asking backend-dev for React | Spawn frontend-dev |
| Single agent for database + backend + tests | Sequential: dba, backend-dev, qa |
| Parallel spawns with hidden dependencies | Make dependencies explicit, spawn sequentially |
| Spawning without session context | Create session first, reference in prompts |
| Council for simple decisions | Single agent or PM judgment |

## Agent Access Hierarchy

| Agent Type | External Issue Access | Reports To |
|------------|----------------------|------------|
| PM | Read/Write | User |
| Implementation agents | None | PM (via session) |
| Review agents (backend/frontend devs) | None | PM (via session) |
| Product agent | Read-only | PM |

**Key**: Only PM writes to external issue trackers. All other agents report through session files.
