---
description: Review agent artifacts and manage lifecycle
hooks:
  Stop:
    - hooks:
        - type: command
          command: "bash ${CLAUDE_PLUGIN_ROOT}/hooks/sessions/validate-session-created.sh"
---

Review ALL agent artifacts in `.agents/` and provide hygiene recommendations.

**CRITICAL: This command must review EVERY file, not samples or averages.**
**Never reference `.agents/` files in output docs outside `.agents/`.**
**After user confirmation, auto-move archived artifacts and update `.agents/` links (no manual steps).**
**Archived artifacts are retained indefinitely.**

## 1. Sessions Review (COMPREHENSIVE)

For EACH session file in `.agents/sessions/` and `.agents/sessions/archive/`:
- Require `archived_at` and `archived_by` when status is `archived`
- Require `archived_by` to be filled when archiving
- After user confirmation, auto-move archived sessions and update `.agents/` references

### A. Read and Summarize
- Session title and status
- Linear issue ID (if any) → Query Linear for current status
- Created date and last updated
- Current state from frontmatter/content
- Council/report summaries present?

### B. Check for Issues
- [ ] References missing files? (reports, handoffs, debt) → **Stale**
- [ ] Linear issue is closed? → **Ready for archive (status + move)**
- [ ] Status is "completed" (not archived yet)? → **Ready for archive (status + move)**
- [ ] File already in archive but status not `archived`? → **Fix status**
- [ ] Archived session missing `archived_at` or `archived_by`? → **Add archive metadata**
- [ ] >7 days since last update with no activity? → **Review for staleness**
- [ ] Council file exists but no session summary? → **Update before archive**
- [ ] Reports linked but unprocessed? → **Update before archive (status + move)**
- [ ] Reports processed but not summarized? → **Update before archive (status + move)**
- [ ] Reports archived without `archived_at` or `archived_by`? → **Add timestamp + actor + update frontmatter**
- [ ] Archived items missing `archived_by`? → **Add actor**
- [ ] Archived items missing link updates? → **Update `.agents/` references**

### C. Check for Extraction Needs
- [ ] Contains `lessons_learned`? → Extract to relevant docs
- [ ] Contains `decisions` not in ADRs? → Suggest ADR creation
- [ ] Contains `remaining_work` or `next_steps`? → Check if tracked in Linear
- [ ] Contains `issues_discovered`? → Check if tracked in Linear
- [ ] Contains `technical_debt`? → Check if tracked in Linear
- [ ] Council outcomes missing? → Summarize in session before archive
- [ ] Reports processed without summary? → Add conclusions/action points to session
- [ ] Session archived but council/report summaries missing? → Reopen, update, re-archive
- [ ] Archived files missing link updates inside `.agents/`? → **Update `.agents/` references to archived paths**

### D. Present to User (Per Session)
For each session, show:
- Summary (2-3 lines)
- Issues found (list)
- Extraction recommendations (specific items with destinations)
- Council/report summary gaps
- Recommendation: **Extract & Archive (status + move)** / **Archive (status + move)** / **Keep** (auto-move + update links after confirmation)
- Archive location: `.agents/sessions/archive/`

## 2. Session Archival Process (Deliberative)

**Never auto-archive.** For each session flagged:

1. Show extraction checklist with specific items
2. Confirm council outcomes and report summaries are captured
3. Ask user: "Extract and archive (status + move)" / "Archive (status + move)" / "Keep"
4. If extracting, perform extractions first
5. Archive only after user confirmation
6. Update status to `archived`, set `archived_at` and `archived_by`
7. After user confirmation, auto-move the file and update `.agents/` references to the archived path
8. Archive indefinitely (no deletion policy)
9. Keep `.agents/` references inside `.agents/` only
10. Archive action includes auto-update of links (no manual steps)

## 3. Councils Review

For EACH council file in `.agents/councils/` and `.agents/councils/archive/`:
- Topic and date
- Linked session (if any) → Does session still exist?
- Decision outcome → Is it captured in an ADR?
- Session summary → Is the council outcome summarized in the linked session?
- Cleanup status → Archive (set status, `archived_at`, `archived_by`, move to `.agents/councils/archive/`) only after session summary exists
- Archived council while session missing summary → **Move back until summary captured**
- Require `archived_by` to be filled when archiving
- After user confirmation, auto-move archived councils and update `.agents/` references (update links inside `.agents/` only; no manual steps)
Flag if:
- No linked session → **Orphaned**
- Session missing outcome summary → **Update session before archive**
- >14 days old → **Review for staleness**
- Decision should be an ADR → **Suggest creation**
- Council marked completed but no session summary → **Block archive**
- Archived council but status not `archived` → **Fix status**
- Archived council missing `archived_at` or `archived_by` → **Add archive metadata**

## 4. Reports Review

For EACH file in `.agents/reports/` and `.agents/reports/archive/`:
- Require frontmatter fields: `status`, `session_reference`, `processed_at`, `archived_at`, `archived_by`
- Treat missing frontmatter as **unprocessed**
- Reports may be archived (status + move) only after:
  - Report is processed
  - Linked session is archived
  - Session captures key conclusions and action points (no dead links)
  - Report frontmatter includes `archived_at` and `archived_by`
  - `archived_by` is always filled when archiving (required)
- If unprocessed, prompt to add frontmatter and summarize findings in the session
- Archived report but status not `archived` → **Fix status**
- Archived report missing `archived_at` or `archived_by` → **Add archive metadata**
- Archived report while linked session not archived → **Move back until session archived**
- Archived report but session missing conclusions/action points → **Move back until updated**
- After user confirmation, auto-move archived reports and update `.agents/` references (update links inside `.agents/` only; no manual steps)
### Report Frontmatter Template

```yaml
---
report:
  status: processed
  session_reference: ".agents/sessions/YYYYMMDD-HHMMSS-title.md"
  processed_at: "YYYY-MM-DDTHH:MM:SSZ"
  archived_at: "YYYY-MM-DDTHH:MM:SSZ"
  archived_by: "agent-pm"  # Optional; fill when archived (enforced by /review-sessions)
---
```

**Archive location:** `.agents/reports/archive/` (move after processed + linked session archived + `archived_at` + `archived_by` set).

## 5. Summary Table

Present comprehensive summary:

```
SESSIONS (N total):
  ✓ Ready for archive (status + move): N (list titles)
  ⚠ Need extraction: N (list specific items)
  ○ Keep active: N

COUNCILS (N total):
  ✓ Clean: N
  ⚠ Orphaned/stale: N
  ⚠ Missing session summary: N
  ⚠ Pending archive: N

REPORTS (N total):
  ✓ Processed and archived (status + move): N
  ⚠ Missing frontmatter: N
  ⚠ Awaiting session summary: N
```

**Retention**: archived indefinitely (no deletion policy).

## 6. CHANGELOG Integration Check

For each session with completed or near-complete work:

### A. Check for CHANGELOG Draft
- [ ] Does session have `## CHANGELOG Draft` section?
- [ ] If yes, has it been integrated to CHANGELOG.md?
- [ ] If no draft exists, should one be created?

### B. CHANGELOG Status Report

```
CHANGELOG DRAFTS:
  ✓ Integrated: N sessions
  ⚠ Pending: N sessions (list titles)
  ○ No draft needed: N sessions
```

### C. Pending Draft Actions

For sessions with unintegrated CHANGELOG drafts:
```
Session: [session-title]
Linear: [PLT-XXX]
Draft status: Not integrated

→ Run /update-changelog to integrate
```

For completed sessions without drafts:
```
Session: [session-title]
Linear: [PLT-XXX]
Has user-facing changes: [Yes/No]

→ Spawn Product agent to create draft? [Y/n]
```

### D. Auto-Suggest

If any sessions have pending CHANGELOG drafts, display:

```
─────────────────────────────────────────
CHANGELOG ACTION NEEDED

[N] session(s) have CHANGELOG drafts ready for integration.

Run /update-changelog to integrate pending entries.
─────────────────────────────────────────
```

---

## 7. Where Knowledge Belongs

**No document outside `.agents/` should reference `.agents/` files.**

Reference for extraction destinations:

| Information Type | Destination |
|------------------|-------------|
| Work tracking | Linear issues |
| Implementation details | Git commits |
| Architectural decisions | `docs/decisions/ADR-XXX-*.md` |
| Migration patterns | `docs/DATABASE_ARCHITECTURE.md` |
| API contracts | `docs/api/` |
| Test patterns | `docs/TESTING.md` |
| Remaining work | Linear backlog |
| User-facing changes | CHANGELOG.md (via `/update-changelog`) |
| Council outcomes | Session summary (link council file) |
| Report conclusions | Session summary (link report file) |
| Archived artifacts | `.agents/<type>/archive/` + status `archived` |
