---
title: Enrichment quality improvements — scope filtering, entry quality, multi-JSONL
status: raw
created: 2026-04-11T01:30:00Z
tags: [enrichment, librarian, quality]
related: [cli/commands/session.ts, content/agents/librarian.md]
---

# Enrichment quality improvements

Three issues discovered during the first real enrichment test (SPEC-029 session):

## 1. Off-topic entries (scope filtering)

The librarian added `discover(scratchpad)` entries about a cross-worktree scratchpad concept into a session about SPEC-029 journal enrichment. The JSONL contained both topics (same conversation), but only SPEC-029 work belongs in this session's journal.

**Fix:** Add scope constraint to enrichment prompt: "Only add entries relevant to the session's linked spec or branch work. If the session has a `spec:` field in frontmatter, filter to that spec's domain. Off-topic discussions from the same conversation are not journal-worthy."

## 2. Verbose garbled entries (entry quality)

The librarian produced entries like:
```
[2026-04-10 20:42] discover(scratchpad): Explored cross-worktree state-sharing problem. Analyzed three mechanics (shared .git/, symlinks, CLI-mediated). Determined $GIT_COMMON_DIR/scratch/ with CLI-mediation works across all worktrees. Symlinks dangling in worktrees...
```

Journal entries should be concise one-liners matching the existing style. The librarian copied JSONL content verbatim instead of distilling.

**Fix:** Add examples of good vs bad entries to the enrichment prompt. Reference the existing journal entries as style guide. "Each entry should be one concise sentence — a conclusion, not a narrative."

## 3. Session routing mismatch (root cause of multi-JSONL problem)

`loaf session log` routes by **branch** (`findActiveSessionForBranch()`), but `loaf session enrich` routes by **`claude_session_id`**. When multiple sessions exist for the same branch, `session log` can write entries to a session file whose `claude_session_id` doesn't match the conversation that produced them. Enrichment then reads the wrong JSONL.

**Root fix:** Route `session log` by `claude_session_id` too — same as `session start`/`session end` already do via `findSessionByClaudeId()`. This makes all session routing consistent: the session file, the journal entries, and the JSONL are all linked by the same conversation ID.

**Challenge:** `loaf session log` is called via Bash tool from within Claude conversations. The `claude_session_id` is available in hook input JSON (stdin) but not when called directly. Options:
- Pass session ID as an env var from Claude Code (check if one exists)
- Add `--session-id <id>` flag to `loaf session log`
- Have the `UserPromptSubmit` hook set an env var that persists for the conversation
- Fall back to branch routing when no session ID is available

**Secondary fix:** Enrich from ALL conversation IDs found in `session(start)` journal entries, not just the one in frontmatter.

## Discovered

First real enrichment test on the SPEC-029 session — librarian added 2 garbled off-topic entries from the wrong JSONL. Second test on the earlier session (correct JSONL) produced 18 high-quality entries. The difference was which `claude_session_id` the session file had.
