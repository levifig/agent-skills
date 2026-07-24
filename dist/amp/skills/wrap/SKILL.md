---
name: wrap
description: >-
  Optional end-of-conversation checkpoint: flushes journal entries, surfaces
  loose ends, prompts for action on uncommitted/unpushed work, and writes a wrap
  entry to the project journal when there is synthesis worth saving. Use at the
  end of a work session or when the user asks "wrap up." Not for archiving (use
  housekeeping) or capturing ideas (use idea). Produces a Session Wrap-Up
  summary and an optional wrap journal entry.
version: 2.0.0-alpha.13
---

# Wrap

An optional checkpoint before the conversation ends — the conscious review of loose ends plus a wrap entry when there is synthesis worth saving.

**Input:** $ARGUMENTS

---

## Contents
- Critical Rules
- Verification
- Quick Reference
- Interactive Steps
- Wrap Entry
- Report Format

## Critical Rules

- Log `skill(wrap): <context>` to the project journal as the first action (e.g. "end-of-session summary" or "user requested wrap-up")
- **Use `Amp UI input` for all decisions and confirmations** — commit, push, stash, or skip choices. Never use inline text questions for permission prompts
- Never commit or push without explicit user confirmation
- Flush journal entries BEFORE generating the report — unrecorded decisions are lost after this conversation
- Pull from live data (git, filesystem), not memory or assumptions
- Keep the report concise — one screen, not a wall of text
- Scope to THIS conversation, not the full backlog
- When delegated Amp check/agent mode or new thread are available, use the `librarian` profile as the
  durable artifact handler for `.agents/`-scoped wrap cleanup, report hygiene,
  and knowledge note preservation. The main wrap flow remains responsible for
  user-facing decisions and commit/push prompts.
- A wrap is optional. Write a `wrap(scope)` entry only when there is synthesis worth saving; a conversation that ends without one leaves a perfectly valid journal. Nothing is ever ended or archived — archival is housekeeping's job

## Verification

- All decisions and discoveries from this conversation are in the journal
- Uncommitted/unpushed state is surfaced with clear action prompts
- Stale KB files are flagged if any
- A `wrap(scope)` journal entry is written when the conversation holds synthesis worth saving (intentions, abandoned paths, next steps); otherwise it is deliberately skipped

## Quick Reference

| Section | Source |
|---------|--------|
| Shipped | `git log` since conversation start |
| Pending | `git status` + unpushed commits |
| Decisions | Journal `decision()` entries |
| Ideas | Journal `spark()` entries + SQLite idea/spark records |
| Loose ends | Unresolved `todo()`/`block()`, stale KB |

## Interactive Steps

These steps require conversation context — only the model can do them.

### Step 1: Flush Journal

Before anything else, complete the journal for this conversation:

1. **Review what wasn't logged** — scan the conversation for anything not yet recorded:
   - Decisions — design choices, trade-offs, direction changes not yet logged as `decision()` entries
   - Discoveries — anything learned that future conversations would benefit from
   - Todos — action items that came up but weren't captured
2. **Log each** via `loaf journal log` before proceeding. This is the last chance — the journal IS the external memory.

### Step 2: Gather Data

Run in parallel:

1. **This conversation's entries** — `loaf journal recent --since-last-wrap` (the entries this wrap will synthesize); widen with `loaf journal recent` or `loaf journal context` when more context is needed
2. **Commits this session** — `git log --oneline` since work began
3. **Working tree** — `git status --short`
4. **Unpushed commits** — `git log --oneline origin/<branch>..HEAD`
5. **Ideas this session** — SQLite idea/spark records created today
6. **Stale KB** — `loaf kb` count if available

### Step 3: Prompt for Action

Surface each loose end with a clear action the user can take. Ask once, respect the answer.

| Loose End | Prompt |
|-----------|--------|
| Uncommitted changes | "N file(s) uncommitted — commit, stash, or leave for next session?" |
| Unpushed commits | "N commit(s) on <branch> not pushed — push now?" |
| Release candidate | "This session landed work that may belong in the next release — run `/release` now?" |
| Stale KB files | "N stale knowledge file(s) — address now or defer?" |
| Unresolved blocks | "Block on <scope> still open — note for next session?" |
| No changelog entries | "N commit(s) on branch but `[Unreleased]` is empty — add changelog entries?" |
| No `/housekeeping` this session | "No housekeeping run this session — run `/housekeeping` now?" |

**Detection logic:**
- **Changelog entries:** check if the current branch has commits vs the base branch (e.g., `git rev-list --count origin/main..HEAD`) AND `CHANGELOG.md` `[Unreleased]` section has no list items (`^[-*]\s`). If both are true, prompt. **Skip when HEAD is tagged** (post-release state).
- **Housekeeping:** scan the journal for a `skill(housekeeping)` entry. If absent and the session had significant work, suggest it.
- **Release candidate:** scan the journal for a `decision(release)` entry. If absent and the session has landed commits, suggest `/release` only when the work forms a coherent release batch.

### Step 4: Generate Report

Assemble the report per the format below. Omit empty sections — don't show "None" placeholders. Keep the summary in the conversation response.

## Wrap Entry

A wrap is a voluntary checkpoint, not a lifecycle step. After surfacing loose ends, decide whether the conversation holds synthesis worth saving — intentions, abandoned paths, next steps: the connective narrative that evaporates with the context window. Almost everything else is already derivable from the raw entries you logged.

When it does, write a single wrap entry:

```bash
loaf journal log "wrap(scope): shipped X; abandoned Y because Z; next is W"
```

Entries are project-scoped and tagged with this conversation's harness id automatically — there is nothing to open, close, or archive. When the conversation holds no synthesis beyond what the raw entries already say, skip the wrap entirely; the journal is complete without it. The next conversation's start digest surfaces the latest wrap alongside recent branch entries and open tasks.

## Composability

When called near `/ship` or `/release`, wrap runs the same steps but keeps PR landing and version publication distinct. `/ship` may wrap a landed PR; `/release` may wrap a published version.

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
