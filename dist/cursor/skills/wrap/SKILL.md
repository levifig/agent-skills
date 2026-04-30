---
name: wrap
description: >-
  Responsible session shutdown: flushes journal entries, surfaces loose ends,
  prompts for action on uncommitted/unpushed work, and generates a structured
  summary that replaces Current State. Use at the end of a work session or when
  the user asks "wrap up." Not for archiving (use housekeeping) or capturing
  ideas (use idea). Produces a Session Wrap-Up section and closes the session
  with done status.
version: 2.0.0-dev.33
---

# Wrap

Responsible session shutdown — everything that needs a conscious model before the conversation ends.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Interactive Steps
- Scripted Close
- Report Format

## Critical Rules

- Log `skill(wrap): <context>` to the session journal as the first action (e.g. "end-of-session summary" or "user requested wrap-up")
- **Use `AskUserQuestion` for all decisions and confirmations** — commit, push, stash, or skip choices. Never use inline text questions for permission prompts
- Never commit, push, or archive without explicit user confirmation
- Flush journal entries BEFORE generating the report — unrecorded decisions are lost after this conversation
- Pull from live data (git, filesystem), not memory or assumptions
- Keep the report concise — one screen, not a wall of text
- Scope to THIS session, not the full backlog
- Do NOT archive — session stays with `done` status. Archival is housekeeping's job

## Verification

- All decisions and discoveries from this session are in the journal
- Uncommitted/unpushed state is surfaced with clear action prompts
- Stale KB files are flagged if any
- `## Session Wrap-Up` section written to session file (replaces `## Current State`)
- `loaf session end --wrap` run after writing the summary
- Session status is `done` (session stays open for further journal entries until `SessionEnd` fires)

## Quick Reference

| Section | Source |
|---------|--------|
| Shipped | `git log` since session start |
| Pending | `git status` + unpushed commits |
| Decisions | Session journal `decision()` entries |
| Ideas | Session journal `spark()` entries + new `.agents/ideas/` files |
| Loose ends | Unresolved `todo()`/`block()`, stale KB |

## Interactive Steps

These steps require conversation context — only the model can do them.

### Step 1: Flush Journal

Before anything else, complete the session journal:

1. **Enrich from conversation log** — run `loaf session enrich` to fill in gaps from the JSONL conversation log. This catches decisions, discoveries, and context that weren't manually logged. If enrichment fails, continue — manual flush is the fallback.
2. **Manual review** — review the conversation for anything enrichment missed:
   - Decisions — design choices, trade-offs, direction changes not yet logged as `decision()` entries
   - Discoveries — anything learned that future sessions would benefit from
   - Todos — action items that came up but weren't captured
3. **Log each** via `loaf session log` before proceeding. This is the last chance — the journal IS the external memory.

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
| No version bump | "This session shipped work but no release was run — bump version now?" |
| Stale KB files | "N stale knowledge file(s) — address now or defer?" |
| Unresolved blocks | "Block on <scope> still open — note for next session?" |
| No changelog entries | "N commit(s) on branch but `[Unreleased]` is empty — add changelog entries?" |
| No `/loaf:housekeeping` this session | "No housekeeping run this session — run `/loaf:housekeeping` now?" |

**Detection logic:**
- **Changelog entries:** check if the current branch has commits vs the base branch (e.g., `git rev-list --count origin/main..HEAD`) AND `CHANGELOG.md` `[Unreleased]` section has no list items (`^[-*]\s`). If both are true, prompt. **Skip when HEAD is tagged** (post-release state).
- **Housekeeping:** scan session journal for `skill(housekeeping)` entry. If absent and the session had significant work, suggest it.
- **Version bump:** scan session journal for `decision(release)` entry. If absent and the session has commits, offer to run `loaf release --bump prerelease --no-gh --yes`.

### Step 4: Generate Report

Assemble the report per the format below. Omit empty sections — don't show "None" placeholders.

### Step 5: Write Wrap Summary to Session File

Write the `## Session Wrap-Up` section into the active session file, **replacing** `## Current State`. The wrap summary IS the final state — it's a superset that includes everything Current State had plus the structured report.

Use the Edit tool to replace `## Current State (...)` and everything below it (up to `## Journal`) with the wrap-up section. The session file layout after wrap should be:

```
# Session: Title

## Session Wrap-Up        ← you write this (replaces Current State)
...

## Journal                ← append-only log
```

## Scripted Close

After writing the wrap summary, run:

```bash
loaf session end --wrap
```

This handles the mechanical bookkeeping:
- Appends `session(wrap)` marker to the journal (NOT `session(end)` or `session(stop)`)
- Sets session status to `done`
- Persists decisions to linked spec changelog
- Strips any remaining `## Current State` section (if the Edit didn't fully replace it)
- Flags stale knowledge files

**The session stays open for further work** (merge commits, changelog fixes, etc.). The `SessionEnd` hook writes the actual `session(stop)` marker when the conversation ends. This prevents journal entries appearing after stop markers.

**Do not archive.** The session stays in `sessions/` with `done` status. Archival is housekeeping's job.

## Composability

When called from `/release`, wrap runs the same steps but skips the version bump prompt (release already handles it). The `/release` skill should invoke `/wrap` first, then proceed with release steps.

## Suggests Next

After the wrap-up report, suggest `/housekeeping` if it wasn't run this session and artifacts need attention.

## Report Format

Use backtick formatting for code identifiers, file paths, spec/task IDs, version numbers, status values, and CLI commands. Use uppercase for spec and task IDs (`SPEC-029`, not `spec-029`).

```markdown
## Session Wrap-Up

**Shipped** (`vX.Y.Z`, PR #N squash-merged to main)
- `abc1234` feat: description (`SPEC-NNN`) (#N)

**Pending**
- N uncommitted file(s): list key ones
- N unpushed commit(s) on branch

**Decisions**
- `SPEC-NNN`: decision description
- Scope: decision description

**Ideas Captured**
- `idea-slug` — one-line description

**Loose Ends**
- `TASK-NNN` (description) still in backlog
- Stale KB files

**What's Next**
- Run `/triage` to process ideas
- Follow-ups from this session's work
```
