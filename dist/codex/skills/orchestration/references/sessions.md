# Session Management

Sessions are coordination artifacts for active work. They are archived (set status, `archived_at`, `archived_by`, move to `.agents/sessions/archive/`) when work completes to preserve an audit trail.

## When to Use Sessions

- Multi-step work requiring agent coordination
- Handoffs between agents
- Tracking progress during implementation
- Context preservation across agent spawns

## Session File Format

**Link policy**: Documents outside `.agents/` must not reference `.agents/` files. Keep `.agents/` links contained within `.agents/` artifacts, and update them when files move to `.agents/<type>/archive/`.

### Location & Naming

```
.agents/sessions/YYYYMMDD-HHMMSS-<description>.md
```

**Archive location:** `.agents/sessions/archive/`

**Transcripts location:** `.agents/transcripts/`

Transcripts are Claude Code conversation exports (`.jsonl` files) archived after context compaction. They preserve the full audit trail of agent interactions.

```
.agents/
├── sessions/
│   ├── YYYYMMDD-HHMMSS-auth-feature.md
│   └── archive/
└── transcripts/              # Claude Code transcripts
    ├── 2a244262-8599-4bef-8bb8-3feea33d14e2.jsonl
    └── archive/
```

**Why keep original filenames:** The UUID-like hash is unique and matches Claude Code's internal reference, making correlation easier if needed.

**Generate timestamps:**
```bash
# Filename timestamp
date -u +"%Y%m%d-%H%M%S"

# ISO timestamp (for YAML)
date -u +"%Y-%m-%dT%H:%M:%SZ"
```

**Good**: `20251204-143000-weather-fallback.md`
**Bad**: `weather-fallback.md` (missing timestamp)
**Bad**: `PLT-123-weather-fallback.md` (Linear ID in filename)

### Required Frontmatter

```yaml
---
session:
  title: "Clear description of work"           # REQUIRED
  status: in_progress                          # REQUIRED: in_progress|paused|completed|archived
  created: "2025-12-04T14:30:00Z"              # REQUIRED: ISO 8601
  last_updated: "2025-12-04T14:30:00Z"         # REQUIRED: ISO 8601
  last_archived: "2025-12-04T16:00:00Z"        # Set by context-archiver before compaction
  archive_reason: "pre-compact"                # Why archived (pre-compact, manual, etc.)
  archived_at: "2025-12-04T18:10:00Z"          # Required when archived
  archived_by: "agent-pm"                      # Optional; fill when archived (enforced by /review-sessions)
  linear_issue: "BACK-123"                     # Optional
  linear_url: "https://linear.app/..."         # Optional
  branch: "username/back-123-feature"          # Optional: working branch
  transcripts: []                              # Archived Claude Code transcripts (filenames only)
                                               # Example: ["2a244262-8599-4bef-8bb8-3feea33d14e2.jsonl"]
  referenced_sessions: []                      # Cross-session references (see below)

plans: []  # List of plan files in .agents/plans/ used by this session
           # Example: ["20251204-143500-api-design.md", "20251204-150000-frontend.md"]

orchestration:
  current_task: "What's actively being worked" # REQUIRED
  spawned_agents:
    - agent: backend-dev
      task: "Brief task description"
      status: completed                        # pending|in_progress|completed
      summary: "Outcome summary"

background_agents:                             # Background work running independently
  - id: "bg-20260123-143000-security-scan"     # ID: bg-YYYYMMDD-HHMMSS-description
    agent: background-runner                   # Agent type
    task: "Full security audit"                # Brief description
    status: running                            # running|completed|failed
    result_location: null                      # Path to report when complete
---
```

### Cross-Session References

Track decisions imported from past sessions via `/reference-session`:

```yaml
session:
  # ... other fields ...
  referenced_sessions:
    - session: "20250115-140000-auth-jwt.md"     # Source session filename
      imported_at: "2025-01-23T14:30:00Z"        # When imported
      content_type: decisions                     # decisions|context|all
      decisions_imported:                         # List of decision titles
        - "JWT token rotation strategy"
        - "Refresh token storage approach"
    - session: "20250110-090000-auth-oauth.md"
      imported_at: "2025-01-23T14:35:00Z"
      content_type: context
      summary: "OAuth provider integration patterns"
```

**Why track references:**
- Audit trail of where context came from
- Avoids re-importing same decisions
- Enables tracing decision lineage across sessions

See `references/cross-session.md` for full patterns.

### Required Sections

Every session file MUST have:

1. **`## Context`** - Background for anyone picking up this work
2. **`## Current State`** - Where we are now (MUST be handoff-ready)
3. **`## Next Steps`** - Immediate actions for continuation

### Session Template

```markdown
# Session: [Title]

## Context
Background for anyone picking up this work.
What problem are we solving? Why now?

## Current State
Where we are right now. What just happened.
**This section should ALWAYS be handoff-ready.**

## Resumption Prompt

<!-- Generated by context-archiver before compaction -->
<!-- Provides self-contained context for post-compaction continuation -->

> **Context**: [Brief description of work being done]
>
> **Last Action**: [What just happened]
>
> **Immediate Next**: [Concrete next step]
>
> **Key Files**: [Relevant file paths]
>
> **Blockers**: [Any blockers, or "None"]
>
> **Transcript Archive**: If Claude Code provided a transcript path after compaction,
> copy it to `.agents/transcripts/` and add the filename to this session's
> `transcripts:` array in frontmatter.

## Execution Progress

### Wave 1: [Name] (ACTIVE)
- [ ] BACK-123 - Brief description
- [x] BACK-124 - Brief description (completed)

### Wave 2: [Name] (blocked by Wave 1)
- [ ] BACK-125 - Brief description

## Technical Context

### Files to Update
- `path/to/file.py` - What needs to change

### Key Commands
```bash
pytest path/to/tests/ -v
mypy path/to/code/
```

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2

## Decisions

### Decision 1: [Title]
**Decision**: What was decided
**Rationale**: Why

## Plans

Implementation plans for this session (stored in `.agents/plans/`):

| Plan | Status | Description |
|------|--------|-------------|
| [api-design](../plans/20251204-143500-api-design.md) | approved | API endpoint structure |
| [frontend](../plans/20251204-150000-frontend.md) | pending | UI component design |

## Architecture Diagrams

### [Diagram Name]

```mermaid
[diagram content]
```

**Purpose**: Why this diagram helps understand the work
**Files involved**: List of related file paths
**Created**: When diagram was created (update if modified)

<!--
When to add diagrams:
- Multi-service changes: Show interaction points
- Data flow changes: Trace data through system
- Schema modifications: Visualize relationships
- API design: Document request/response flows

For reusable diagrams, store in .agents/diagrams/ instead.
See foundations skill reference/diagrams.md for Mermaid syntax.
-->

## Council Outcomes

### Council: [Topic]
**Outcome**: Decision summary
**Council File**: `.agents/councils/YYYYMMDD-HHMMSS-topic.md`
**Next Steps**: Action items captured
**Archive**: After this summary is captured, set council status to `archived`, set `archived_at`, set `archived_by`, and move to `.agents/councils/archive/` (archive indefinitely).

## Reports Processed

### Report: [Title]
**Key Conclusions**: Summary of findings
**Action Items**: What changed or will change
**Report File**: `.agents/reports/YYYYMMDD-HHMMSS-title.md`
**Archive**: After report is processed and the linked session is archived, set report status to `archived`, add `archived_at`, add `archived_by`, and move to `.agents/reports/archive/` (archive indefinitely).
**Frontmatter**: Require `status`, `session_reference`, and `processed_at`; add `archived_at` and `archived_by` when archived.
**Note**: Reports without frontmatter are treated as unprocessed.

## Blockers
- Current blocker (if any)

---

## Session Log

### YYYY-MM-DD HH:MM - [Agent/Action]
Brief description of what happened.
```

## Lifecycle States

| State | Description |
|-------|-------------|
| `in_progress` | Work actively happening |
| `paused` | Temporarily stopped |
| `completed` | Work finished; ready to archive (set status, `archived_at`, `archived_by`, move) after extraction |
| `archived` | Closed and preserved for audit (set `archived_at`/`archived_by`, status set + moved to `.agents/sessions/archive/`) |

## Updating During Work

After each significant action:

1. Update `Current State`
2. Add to progress checklist
3. Update `Files Modified` if applicable
4. Add entry to `Session Log` with timestamp (`YYYY-MM-DD HH:MM`)
5. Update `Next Steps`

## Handoff Protocol

### Before Handing Off

1. Update session file with completed work
2. Update `Current State` with where you stopped
3. Update `Next Steps` with immediate actions
4. Include `Files Modified` for context

### Session as Handoff Medium

Each agent:
1. Reads current state from session
2. Performs assigned work
3. Updates session with outcomes
4. Sets up context for next agent

## Completing a Session

Sessions are **archived, not deleted** when complete to preserve audit trail (set status to `archived`, set `archived_at` and `archived_by`, and move into `.agents/sessions/archive/`). Archive indefinitely (no deletion policy).

**Archive timing:** only after extraction and council/report summaries are captured.

### Knowledge Extraction Checklist

| Information Type | Where It Belongs |
|------------------|------------------|
| Work tracking | External issue (Linear, GitHub) |
| Implementation details | Git commits/PRs |
| Architectural decisions | ADRs (`docs/decisions/`) |
| API contracts | API documentation |
| Remaining work | External issue backlog |
| Council outcomes | Session summary + council file link |
| Report conclusions | Session summary + report file link |
| Archived artifacts | `.agents/<type>/archive/` + status `archived` + `archived_at` + `archived_by` |

### Archival Checklist

- [ ] External issue updated with final status
- [ ] Decisions captured as ADRs (if architectural)
- [ ] Lessons learned added to relevant docs
- [ ] Remaining work created as issues
- [ ] Council outcomes summarized in this session (link council file)
- [ ] Reports processed and summarized in this session (link report file)
- [ ] Reports archived only after session is archived (status + move)
- [ ] No orphaned references to session file
- [ ] Set session status to `archived`
- [ ] Set `archived_at` and `archived_by`
- [ ] Move file to `.agents/sessions/archive/`
- [ ] Linked councils moved to `.agents/councils/archive/` after session summary
- [ ] Linked reports moved to `.agents/reports/archive/` after session archived + conclusions captured
- [ ] Update `.agents/` references to archived paths (no `.agents` links outside `.agents/`)
- [ ] Archive indefinitely (no deletion policy)
- [ ] Use `/review-sessions` for auto-move + link updates after confirmation

## PM Start Protocol

When starting a new orchestration context:

1. Check for existing active sessions
2. If found, ask user which to resume or start fresh
3. If resuming, read session for context
4. If starting fresh, create new session file

## Hook Integration

### SessionStart Hook
- Lists active sessions
- Provides agent-specific context
- Suggests session review/resume

### SessionEnd Hook
- Displays completion checklist
- Reminds about session file updates
- Shows count of active sessions

### PreCompact Hook
- Identifies recently modified sessions (last 60 minutes)
- Outputs instructions for spawning context-archiver agent
- Context-archiver updates sessions with:
  - Current state (concrete, handoff-ready)
  - `## Resumption Prompt` section for post-compaction continuation
  - Session log entry with timestamp
  - Optional Serena memory for critical decisions
- After compaction:
  1. Read session's Resumption Prompt to continue seamlessly
  2. If transcript path provided, archive it:
     - Copy transcript to `.agents/transcripts/`
     - Add filename to session's `transcripts:` array

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Archive without extraction | Extract outcomes before archiving |
| Use sessions as permanent records | Use proper documentation locations |
| Reference stale sessions | Keep sessions current or archive (status + move) when done |
| Store decisions only in sessions | Create ADRs for important decisions |
| Archive without council/report summaries | Summarize outcomes in session before archive |
| Archive but leave in active folder | Move file to `.agents/sessions/archive/` after setting status |
| Batch session updates | Update after each significant event |
| Keep status as `in_progress` when paused | Update status when state changes |
