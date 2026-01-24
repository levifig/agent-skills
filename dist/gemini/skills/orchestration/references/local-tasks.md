# Local Task Management

Break specs into atomic tasks using local files when Linear isn't available.

## Task Abstraction Layer

Tasks work identically whether backed by Linear or local files.

### Configuration

```yaml
# .agents/loaf.yaml
task_management:
  backend: linear  # or "local"

  linear:
    team: ProjectName
    default_labels: []

  local:
    archive_completed: true
    directory: .agents/tasks
```

### Abstracted Operations

| Operation | Linear | Local |
|-----------|--------|-------|
| Create task | Create issue | Write `.agents/tasks/active/TASK-*.md` |
| Fetch task | Get issue | Read task file |
| Update status | Update issue | Edit frontmatter |
| List tasks | List issues | Glob task files |
| Complete | Move to Done | Move to archive |

## Local Task Format

```yaml
---
id: TASK-001
title: OAuth Provider Integration
spec: SPEC-001
status: todo  # todo | in_progress | review | done
priority: P1  # P0 (urgent) | P1 (high) | P2 (normal) | P3 (low)
created: 2026-01-23T14:30:00Z
updated: 2026-01-23T14:30:00Z
session: .agents/sessions/YYYYMMDD-HHMMSS-*.md  # When picked up
files:
  - src/auth/oauth.py
  - tests/auth/test_oauth.py
verify: pytest tests/auth/test_oauth.py
done: OAuth flow works for Google and GitHub
---

# TASK-001: OAuth Provider Integration

## Description

Implement OAuth provider integration for Google and GitHub following
the approach outlined in SPEC-001.

## Acceptance Criteria

- [ ] Google OAuth flow completes successfully
- [ ] GitHub OAuth flow completes successfully
- [ ] Tokens stored securely in session
- [ ] Error handling for failed auth
- [ ] Unit tests pass

## Context

See SPEC-001 for full context and constraints.
Reference ADR-001 for session storage decisions.

## Work Log

<!-- Updated by session as work progresses -->
```

**Location:** `.agents/tasks/active/TASK-001-oauth-provider.md`

## Task Lifecycle

```
todo → in_progress → review → done
  │        │           │        │
  └────────┴───────────┴────────┘
           can return to earlier states
```

| Status | Meaning |
|--------|---------|
| `todo` | Ready to work, not started |
| `in_progress` | Actively being worked |
| `review` | Implementation complete, needs verification |
| `done` | Verified complete, ready for archive |

## Creating Tasks from Specs

### Input

- Spec ID (e.g., `SPEC-001`)
- Optional: priority override

### Task Breakdown Rules

1. **One concern per task** - Don't mix backend + tests + frontend
2. **Clear done condition** - Observable, verifiable outcome
3. **Verification command** - How to prove it works
4. **File hints** - Which files will likely be modified

### Example Breakdown

```
SPEC-001: User Authentication with OAuth
        ↓
TASK-001: OAuth Provider Integration
  - Google OAuth client setup
  - GitHub OAuth client setup
  - Token exchange logic
  verify: pytest tests/auth/test_oauth.py

TASK-002: Session Management
  - Session cookie handling
  - Session storage (Redis/DB)
  - Session expiry logic
  verify: pytest tests/auth/test_session.py

TASK-003: Login UI Components
  - Login page layout
  - Provider buttons
  - Error states
  verify: npm run test:e2e -- auth
```

## Directory Structure

```
.agents/tasks/
├── active/                    # Current work
│   ├── TASK-001-oauth-provider.md
│   ├── TASK-002-session-management.md
│   └── TASK-003-login-ui.md
└── archive/                   # Completed work
    └── 2026-01/              # Organized by month
        └── TASK-000-initial-setup.md
```

## Task ID Generation

Format: `TASK-{number}-{slug}`

```bash
# Find next available number
ls .agents/tasks/active/ .agents/tasks/archive/*/ 2>/dev/null | \
  grep -oE 'TASK-[0-9]+' | \
  sort -t- -k2 -n | \
  tail -1 | \
  awk -F- '{print $2 + 1}'
```

## Archiving Tasks

When a task is done:

1. Update status to `done`
2. Add completion timestamp
3. Move to archive:

```bash
mkdir -p .agents/tasks/archive/$(date +%Y-%m)
mv .agents/tasks/active/TASK-001-*.md .agents/tasks/archive/$(date +%Y-%m)/
```

## Session Integration

When `/implement TASK-001`:

1. Read task file for context
2. Read linked spec for full picture
3. Create session with traceability:

```yaml
session:
  title: "OAuth Provider Integration"
  task: TASK-001
  spec: SPEC-001
  status: in_progress

traceability:
  requirement: "2.1 User Authentication"
  architecture:
    - "Session Management"
  decisions:
    - ADR-001
```

## Priority Levels

| Priority | Meaning | Response |
|----------|---------|----------|
| P0 | Urgent/blocking | Drop everything |
| P1 | High | Work next |
| P2 | Normal | Scheduled work |
| P3 | Low | When time permits |

## Listing Tasks

### All Active Tasks

```bash
for f in .agents/tasks/active/TASK-*.md; do
  id=$(grep '^id:' "$f" | cut -d: -f2 | tr -d ' ')
  title=$(grep '^title:' "$f" | cut -d: -f2-)
  status=$(grep '^status:' "$f" | cut -d: -f2 | tr -d ' ')
  echo "$id [$status] $title"
done
```

### Tasks for a Spec

```bash
grep -l "spec: SPEC-001" .agents/tasks/active/*.md
```

## Work Log Updates

As work progresses, append to the Work Log section:

```markdown
## Work Log

### 2026-01-23 14:30 UTC
Started OAuth integration. Set up Google OAuth client credentials.

### 2026-01-23 15:45 UTC
Google OAuth working. Moving to GitHub integration.

### 2026-01-23 17:00 UTC
Both providers working. Tests pass. Moving to review.
```

## Verification

Before marking `done`:

1. Run the `verify` command from frontmatter
2. Check all acceptance criteria are checked
3. Ensure no regressions in related tests

```bash
# Run task verification
verify_cmd=$(grep '^verify:' TASK-001-*.md | cut -d: -f2-)
eval "$verify_cmd"
```

## Local vs Linear Comparison

| Feature | Local | Linear |
|---------|-------|--------|
| No external dependency | yes | no |
| Rich UI | no | yes |
| Team collaboration | git-based | native |
| Notifications | none | email/slack |
| Reporting | manual | built-in |
| Offline work | yes | limited |

**Use local when:**
- Solo project
- No Linear access
- Offline development
- Simple task tracking

**Use Linear when:**
- Team collaboration needed
- Rich workflow automation
- Integration with other tools
- Reporting requirements
