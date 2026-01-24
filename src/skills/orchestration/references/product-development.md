# Product Development Workflow

A Research → Vision → Architecture → Requirements → Specs → Tasks → Session workflow for structured product development.

## Core Insight

**Separate permanent project knowledge from ephemeral orchestration.**

```
docs/                               # Permanent project knowledge
├── VISION.md                       # Strategic direction (evolves rarely)
├── ARCHITECTURE.md                 # Technical design (evolves with learnings)
├── REQUIREMENTS.md                 # Business/product rules (evolves with feedback)
└── decisions/                      # Architecture Decision Records
    └── ADR-001-database-choice.md

.agents/specs/                      # Feature specifications (temporary)
├── SPEC-001-user-auth.md
└── archive/                        # Completed specs

.agents/                            # Ephemeral orchestration
├── loaf.yaml                       # Project config (task backend, etc.)
├── sessions/                       # Active work contexts
├── plans/                          # Implementation plans
├── councils/                       # Deliberation records
├── transcripts/                    # Archived conversation transcripts
├── tasks/                          # Local tasks (when not using Linear)
│   ├── active/
│   │   └── TASK-001-oauth-provider.md
│   └── archive/
└── debug/                          # Persistent debug sessions
```

## The Hierarchy

```
RESEARCH → VISION → ARCHITECTURE → REQUIREMENTS → SPECS → TASKS → SESSION
    │         │          │              │           │        │        │
/research  (manual)  /architecture    /prd       /specs   /tasks  /implement
    │                    │              │           │        │
    └─► evolves ─────────┴──────────────┴───────────┴────────┘
        VISION                                      (feedback loops)
```

**Each level breaks down the one above:**
- Research informs/evolves Vision
- Vision shapes Architecture
- Architecture constrains Requirements
- Requirements break into Specs
- Specs break into Tasks
- Tasks become Sessions

## Commands Overview

| Command | Input | Output | Purpose |
|---------|-------|--------|---------|
| `/research` | Topic or "project state" | Insights, brainstorm | Zoom out, evolve VISION |
| `/architecture` | Decision question | ARCHITECTURE.md + ADR | Technical decisions |
| `/prd` | Feature area | REQUIREMENTS.md section | Product/business rules |
| `/specs` | Requirement reference | SPEC-001-*.md | Feature specifications |
| `/tasks` | Spec reference | TASK-* files/issues | Atomic work items |
| `/implement` | TASK-ID | Session file | Execute a task |

## Document Formats

### VISION.md

Strategic direction that evolves rarely. Contains:
- Product purpose and target users
- Core principles and values
- Long-term direction
- What makes this product unique

### ARCHITECTURE.md

Technical design that evolves with learnings:
- System architecture overview
- Technology choices and rationale
- Integration patterns
- Deployment model

### REQUIREMENTS.md

Business and product rules organized by domain:

```markdown
## 2. Identity & Access

### 2.1 User Authentication

**Business Rules**
- Users authenticate via OAuth (Google, GitHub)
- Sessions expire after 24 hours of inactivity
- Failed login attempts logged for security

**Acceptance Criteria**
- [ ] User can sign in with Google
- [ ] User can sign in with GitHub
- [ ] Session persists across browser refresh
- [ ] Logout clears session completely
```

### Architecture Decision Records (ADRs)

```yaml
---
id: ADR-001
title: "PostgreSQL as Required Database"
status: Accepted  # Proposed | Accepted | Deprecated | Superseded
date: 2026-01-23
---

# ADR-001: PostgreSQL as Required Database

## Context
[Why this decision is needed]

## Decision
[What was decided]

## Consequences
[Tradeoffs and implications]
```

**Location:** `docs/decisions/ADR-001-*.md`

## Configuration

```yaml
# .agents/loaf.yaml
project:
  name: "My Project"

task_management:
  backend: linear  # or "local"

  linear:
    team: ProjectName

  local:
    archive_completed: true

docs:
  vision: docs/VISION.md
  architecture: docs/ARCHITECTURE.md
  requirements: docs/REQUIREMENTS.md
  decisions: docs/decisions/
  specs: .agents/specs/
```

## Workflow Examples

### Full Workflow (New Feature)

```bash
# 1. Zoom out, check project state
/research "project state"
# → Lessons learned, ideas for auth improvement

# 2. Make architecture decision
/architecture "OAuth vs custom auth"
# → Interview → ADR-001-auth-approach.md

# 3. Capture requirements
/prd "user authentication"
# → Interview → REQUIREMENTS.md section 2.1

# 4. Create specification
/specs "2.1 User Authentication"
# → Interview → SPEC-001-user-auth.md

# 5. Generate tasks
/tasks SPEC-001
# → TASK-001, TASK-002, TASK-003

# 6. Work on tasks
/implement TASK-001
# → Session → Implementation → Done
```

### Quick Fix (Ad-hoc)

```bash
# Create task directly (bug report, small fix)
/implement TASK-099
# PM: "No parent spec. Proceed?"
# User: "Yes, small fix"
# → Session → Fix → Done
```

## Feedback Loops

The workflow is not rigid. Each level can feed back to higher levels:

| Discovery | Action |
|-----------|--------|
| Implementation reveals design flaw | Update ARCHITECTURE.md |
| Edge case invalidates requirement | Update REQUIREMENTS.md |
| Spec too ambitious for appetite | Re-scope or split |
| Task reveals missing requirement | Add to REQUIREMENTS.md |

## Transcript Archival

After `/compact` or `/clear`, archive conversation transcripts for future reference:

1. **Copy transcript** to `.agents/transcripts/`
2. **Link from session file** in frontmatter:

```yaml
session:
  title: "Feature Implementation"
  transcripts:
    - 20260123-143500-pre-compact.jsonl
    - 20260123-160000-final.jsonl
```

This preserves full context for debugging, knowledge extraction, and audit trails.

## Flexibility Preserved

1. **Skip steps** - Quick fixes don't need full workflow
2. **Feedback loops** - Any level can evolve from learnings
3. **Agents adapt** - Not a rigid harness

## Critical Files

| File | Purpose |
|------|---------|
| `docs/VISION.md` | Strategic direction |
| `docs/ARCHITECTURE.md` | Technical design |
| `docs/REQUIREMENTS.md` | Product rules |
| `docs/decisions/ADR-*.md` | Architecture decisions |
| `.agents/specs/SPEC-*.md` | Feature specifications |
| `.agents/tasks/active/TASK-*.md` | Local tasks |
| `.agents/loaf.yaml` | Project configuration |
