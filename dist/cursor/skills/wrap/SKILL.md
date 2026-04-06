---
name: wrap
description: >-
  Responsible session shutdown: flushes journal entries, surfaces loose ends,
  prompts for action on uncommitted/unpushed work, and generates a structured
  summary. Use at the end of a work session or when the user asks "wrap up." Not
  for archiving (use housekeeping) or capturing ideas (use idea).
version: 2.0.0-dev.14
---

# Wrap

Responsible session shutdown — everything that needs a conscious model before the conversation ends.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Process
- Report Format

## Critical Rules

- Log `skill(wrap): <context>` to the session journal as the first action (e.g. "end-of-session summary" or "user requested wrap-up")
- Never commit, push, or archive without explicit user confirmation
- Flush journal entries BEFORE generating the report — unrecorded decisions are lost after this conversation
- Pull from live data (git, filesystem), not memory or assumptions
- Keep the report concise — one screen, not a wall of text
- Scope to THIS session, not the full backlog

## Verification

- All decisions and discoveries from this session are in the journal
- Uncommitted/unpushed state is surfaced with clear action prompts
- Stale KB files are flagged if any
- Report covers all non-empty sections

## Quick Reference

| Section | Source |
|---------|--------|
| Shipped | `git log` since session start |
| Pending | `git status` + unpushed commits |
| Decisions | Session journal `decision()` entries |
| Ideas | Session journal `spark()` entries + new `.agents/ideas/` files |
| Loose ends | Unresolved `todo()`/`block()`, stale KB |

## Process

### Step 1: Flush Journal

Before anything else, review the conversation for unrecorded work:

1. **Decisions** — any design choices, trade-offs, or direction changes not yet logged as `decision()` entries
2. **Discoveries** — anything learned that future sessions would benefit from
3. **Todos** — action items that came up but weren't captured

Log each via `loaf session log` before proceeding. This is the last chance — SessionEnd fires after the model is gone.

### Step 2: Gather Data

Run in parallel:

1. **Session journal** — read the active session file for this branch
2. **Commits this session** — `git log --oneline` since session start
3. **Working tree** — `git status --short`
4. **Unpushed commits** — `git log --oneline origin/<branch>..HEAD`
5. **Ideas this session** — `.agents/ideas/` files created today
6. **Stale KB** — `loaf kb` count if available

### Step 3: Prompt for Action

Surface each loose end with a clear action the user can take. Ask once, respect the answer.

| Loose End | Prompt |
|-----------|--------|
| Uncommitted changes | "N file(s) uncommitted — commit, stash, or leave for next session?" |
| Unpushed commits | "N commit(s) on <branch> not pushed — push now?" |
| Stale KB files | "N stale knowledge file(s) — address now or defer?" |
| Unresolved blocks | "Block on <scope> still open — note for next session?" |
| No `/housekeeping` this session | "No housekeeping run this session — run `/housekeeping` now?" |

To check if housekeeping ran, scan the session journal for a `skill(housekeeping)` entry. If absent and the session had significant work (commits, archived specs, etc.), suggest it.

### Step 4: Generate Report

Assemble the report per the format below. Omit empty sections — don't show "None" placeholders.

## Report Format

```markdown
## Session Wrap-Up

**Shipped**
- commit-hash message (PR #N if merged)
- commit-hash message

**Pending**
- N uncommitted file(s): list key ones
- N unpushed commit(s) on branch

**Decisions**
- scope: decision description

**Ideas Captured**
- idea-slug — one-line description

**Loose Ends**
- unresolved todo/block items
- stale KB files

**What's Next**
- natural follow-ups from this session's work
- backlog items surfaced during work
```
