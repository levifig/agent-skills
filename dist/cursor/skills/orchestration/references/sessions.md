# Session Management

Sessions are coordination artifacts for active work. They are archived (set status, `archived_at`, `archived_by`, move to `.agents/sessions/archive/`) when work completes to preserve an audit trail.

## Contents

- Compact vs New Session
- When to Use Sessions
- Session Types
- Session File Format
- Lifecycle States
- Updating During Work
- Handoff Protocol
- Completing a Session
- Start Protocol
- Hook Integration
- Anti-Patterns

## Compact vs New Session

| Scenario | Action |
|----------|--------|
| Picking up previous work, same scope | Compact or resume existing conversation |
| Switching to entirely different scope | New conversation (new session) |
| Finished and archived a spec | New conversation |
| Context full mid-task | Auto-compact (journal survives) |
| Quick unrelated question | New conversation (don't pollute working session) |

**Rule of thumb:** if you'd need the same session file, compact. If you'd need a different one, start fresh.

## When to Use Sessions

- Multi-step work requiring agent coordination
- Handoffs between agents
- Tracking progress during implementation
- Context preservation across agent spawns

## Session Types

### Implementation Sessions (Invisible)

When implementing tasks via `/implement TASK-XXX`:

- Session created automatically with filename `YYYYMMDD-HHMMSS-session.md`
- Task file updated with `session:` field linking to session
- No user interaction needed for session naming
- Resume via `loaf session start` then `/implement TASK-XXX`

Users work with tasks; sessions are an implementation detail.

### Explicit Sessions

For non-task work, sessions are still created explicitly:

- Research sessions (`/research`)
- Architecture decisions (`/architecture`)
- Council deliberations (`/council`)

These sessions may not have a linked task but still follow standard session format.

## Session File Format

**Link policy**: Documents outside `.agents/` must not reference `.agents/` files. Keep `.agents/` links contained within `.agents/` artifacts, and update them when files move to `.agents/<type>/archive/`.

### Location & Naming

```
.agents/sessions/YYYYMMDD-HHMMSS-session.md
```

**Archive location:** `.agents/sessions/archive/`

**Transcripts location:** `.agents/transcripts/`

Transcripts are Claude Code conversation exports (`.jsonl` files) archived after context compaction. They preserve the full audit trail of agent interactions.

```
.agents/
├── sessions/
│   ├── YYYYMMDD-HHMMSS-session.md
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

### Naming Convention

All session files use the fixed format: `YYYYMMDD-HHMMSS-session.md`

The timestamp is the unique identifier. Descriptions, spec links, and branch names belong in frontmatter and the `# Session:` heading, not the filename. This keeps references stable — spec and task files can point to session filenames without them ever breaking.

### Required Frontmatter

```yaml
---
session:
  title: "Clear description of work"           # REQUIRED
  status: in_progress                          # REQUIRED: in_progress|paused|completed|archived
  created: "2025-12-04T14:30:00Z"              # REQUIRED: ISO 8601
  last_updated: "2025-12-04T14:30:00Z"         # REQUIRED: ISO 8601
  last_archived: "2025-12-04T16:00:00Z"        # Set by PreCompact hook before compaction
  archive_reason: "pre-compact"                # Why archived (pre-compact, manual, etc.)
  archived_at: "2025-12-04T18:10:00Z"          # Required when archived
  task: TASK-001                               # If implementation work (links to .agents/tasks/)
  linear_issue: "BACK-123"                     # Optional
  linear_url: "https://linear.app/..."         # Optional
  branch: "username/back-123-feature"          # Optional: working branch
  transcripts: []                              # Archived Claude Code transcripts (filenames only)
                                               # Example: ["2a244262-8599-4bef-8bb8-3feea33d14e2.jsonl"]
  referenced_sessions: []                      # Cross-session references (see below)

orchestration:
  current_task: "What's actively being worked" # REQUIRED
  spawned_agents:
    - agent: implementer
      task: "Brief task description"
      status: completed                        # pending|in_progress|completed
      summary: "Outcome summary"

background_agents:                             # Background work running independently
  - id: "bg-20260123-143000-security-scan"     # ID: bg-YYYYMMDD-HHMMSS-description
    agent: background-runner                              # Agent type
    task: "Full security audit"                # Brief description
    status: running                            # running|completed|failed
    result_location: null                      # Path to report when complete
---
```

### Cross-Session References

Track decisions imported from past sessions via Serena memory:

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

<!-- Generated by PreCompact hook before compaction -->
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

## Session Log (Compact Inline Journal)

Sessions use an **append-only structured journal** format — a running log of what happened, what was decided, and what's next. Think "conventional commits meets bullet journal."

### Format

```markdown
## Journal

[YYYY-MM-DD HH:MM] resume(branch-name): from commit abc1234, context summary
[YYYY-MM-DD HH:MM] decision(scope): description of decision
[YYYY-MM-DD HH:MM] discover(scope): something learned

[YYYY-MM-DD HH:MM] block(scope): what is blocked
[YYYY-MM-DD HH:MM] hypothesis: theory being tested

[YYYY-MM-DD HH:MM] unblock(scope): how it was resolved
[YYYY-MM-DD HH:MM] commit(abc1234): commit message

--- PAUSE YYYY-MM-DD HH:MM ---

[YYYY-MM-DD HH:MM] resume(branch-name): duration paused, last action summary
```

### Entry Types

| Type | Use For | Written By | Scope Convention |
|------|---------|------------|------------------|
| `resume(scope)` | Session started/resumed | Hook (auto) | Branch name |
| `pause` | Session ended | Hook (auto) | — |
| `commit(SHA)` | Code committed | Hook (auto) | Short SHA |
| `pr(#N)` | PR created/updated | Hook (auto) | PR number |
| `merge(#N)` | PR merged | Hook (auto) | PR number |
| `decision(scope)` | Key decisions | Agent | Topic |
| `discover(scope)` | Something learned | Agent | Topic |
| `block(scope)` | Blocker encountered | Agent | Topic |
| `unblock(scope)` | Blocker resolved | Agent | Topic |
| `spark(scope)` | Ideas to promote | Agent | Topic |
| `resolve(spark)` | Spark triaged via `/idea` | Agent/User | Spark slug → disposition [timestamp] |
| `todo(scope)` | Action items | Agent | Topic |
| `finding(scope)` | Findings from analysis | Agent | Topic |
| `hypothesis` | Theory being tested | Agent | — |
| `try` | Approach attempted | Agent | — |
| `reject` | Approach abandoned | Agent | — |
| `skill(name)` | Skill invoked with context | Skill (self-log) | Skill name |
| `idea(slug)` | Idea captured or promoted | Agent/Skill | Idea slug or `.agents/ideas/` ref |
| `spec(id)` | Spec created, updated, or approved | Agent/Skill | Spec ID (e.g. `SPEC-024`) |
| `report(slug)` | Report generated | Agent/Skill | Report slug or `.agents/reports/` ref |
| `council(slug)` | Council convened or concluded | Agent/Skill | Council slug or `.agents/councils/` ref |
| `brainstorm(slug)` | Brainstorm session held | Agent/Skill | Draft slug or topic |
| `plan(slug)` | Plan created or updated | Agent/Skill | Plan slug or topic |
| `draft(slug)` | Draft document created | Agent/Skill | Draft slug or `.agents/drafts/` ref |

### Format Rules

1. **Timestamp:** `YYYY-MM-DD HH:MM` (no seconds)
2. **Entry format:** `[<timestamp>] <type>(<scope>): <description>`
3. **Blank line separator:** Insert when:
   - Gap ≥ 5 minutes since last entry, OR
   - State transition (block/unblock/pause/resume)
4. **PAUSE header:** `--- PAUSE YYYY-MM-DD HH:MM ---` (auto-generated by `loaf session end`)
5. **Resume after pause:** Always starts new section after `--- PAUSE ---`

### Example Session

```markdown
---
spec: SPEC-020
branch: feat/target-convergence
status: active
created: 2026-03-31T14:30:00Z
last_entry: 2026-04-02T09:15:00Z
---

# Session: Target Convergence

## Journal

[2026-03-31 14:30] resume(feat/target-convergence): from commit abc1234, 3 tasks pending
[2026-03-31 14:30] decision(hooks): remove bash wrappers, go direct
[2026-03-31 14:32] discover(cursor): prompt hooks filtered at line 269

[2026-03-31 15:10] block(parity): Codex output differs by trailing newline
[2026-03-31 15:11] hypothesis: gray-matter serialization adds trailing \n

[2026-03-31 16:45] unblock(parity): confirmed gray-matter quirk, fixed with trim
[2026-03-31 16:46] commit(def5678): fix: trim frontmatter for Codex byte parity

--- PAUSE 2026-03-31 17:00 ---

[2026-04-02 09:15] resume(feat/target-convergence): 16 hours paused, last: verification
```

### CLI Commands

```bash
loaf session start    # Find/create session, append resume entry, output context
loaf session end      # Append pause entry with summary
loaf session log      # Append typed entry: loaf session log "decide(scope): desc"
loaf session archive  # Move to archive when branch merges
```

### Consistency

- **`loaf session log`** validates and appends entries in correct format
- **Read-only agents** (reviewer, researcher) CAN write journal entries via `loaf session log` (Bash command, not file edit)
- **Concurrent writes** are safe: atomic append, timestamps provide ordering

## Lifecycle States

| State | Description |
|-------|-------------|
| `in_progress` | Work actively happening |
| `paused` | Temporarily stopped |
| `completed` | Work finished; ready to archive (set status, `archived_at`, `archived_by`, move) after extraction |
| `archived` | Closed and preserved for audit (set `archived_at`/`archived_by`, status set + moved to `.agents/sessions/archive/`) |

## Updating During Work

After each significant action, append entries to the session journal:

1. **Use `loaf session log`** to append entries in the correct format
2. **Add `decision`, `discover`, `block`, `spark`, `wrap` entries** during normal work
3. **Hooks auto-append:** `resume`, `commit(SHA)`, `pr(#N)`, `merge(#N)`, `pause`
4. **Blank line rules:** Inserted automatically by `loaf session log` based on time gaps and state transitions

**Cross-agent protocol:** When any agent starts work on a branch, `loaf session start` outputs the last 15-20 journal entries. The agent reads context, continues work, appends entries.

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
- [ ] Use `/housekeeping` for auto-move + link updates after confirmation

## Start Protocol

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
- `loaf session context for-compact` logs compact marker and injects flush instructions
- Model flushes unrecorded decisions/discoveries to journal
- Model writes structured `## Current State` summary
- After compaction: `loaf session context for-resumption` prints rich resumption context (session, spec, journal, git)

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
