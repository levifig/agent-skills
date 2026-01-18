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

## 1. Sessions Review (COMPREHENSIVE)

For EACH session file in `.agents/sessions/`:

### A. Read and Summarize
- Session title and status
- Linear issue ID (if any) → Query Linear for current status
- Created date and last updated
- Current state from frontmatter/content

### B. Check for Issues
- [ ] References deleted files? (reports, handoffs, debt) → **Stale**
- [ ] Linear issue is closed? → **Ready for deletion**
- [ ] Status is "completed" or "archived"? → **Ready for deletion**
- [ ] >7 days since last update with no activity? → **Review for staleness**

### C. Check for Extraction Needs
- [ ] Contains `lessons_learned`? → Extract to relevant docs
- [ ] Contains `decisions` not in ADRs? → Suggest ADR creation
- [ ] Contains `remaining_work` or `next_steps`? → Check if tracked in Linear
- [ ] Contains `issues_discovered`? → Check if tracked in Linear
- [ ] Contains `technical_debt`? → Check if tracked in Linear

### D. Present to User (Per Session)
For each session, show:
- Summary (2-3 lines)
- Issues found (list)
- Extraction recommendations (specific items with destinations)
- Recommendation: **Extract & Delete** / **Delete** / **Keep**

## 2. Session Deletion Process (Deliberative)

**Never auto-delete.** For each session flagged:

1. Show extraction checklist with specific items
2. Ask user: "Extract and delete" / "Delete" / "Keep"
3. If extracting, perform extractions first
4. Delete only after user confirmation

## 3. Councils Review

For EACH council file in `.agents/councils/`:
- Topic and date
- Linked session (if any) → Does session still exist?
- Decision outcome → Is it captured in an ADR?

Flag if:
- No linked session → **Orphaned**
- >14 days old → **Review for staleness**
- Decision should be an ADR → **Suggest creation**

## 4. Reports Review

For EACH file in `.agents/reports/`:
- List with age
- Reports are ephemeral → Recommend deletion

## 5. Summary Table

Present comprehensive summary:

```
SESSIONS (N total):
  ✓ Ready for deletion: N (list titles)
  ⚠ Need extraction: N (list specific items)
  ○ Keep active: N

COUNCILS (N total):
  ✓ Clean: N
  ⚠ Orphaned/stale: N

REPORTS (N total):
  → Recommend delete all
```

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
