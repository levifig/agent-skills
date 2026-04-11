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

## 3. Multi-conversation JSONL (multi-JSONL discovery)

A session can span multiple Claude conversations (after `/clear`, restarts, new conversations on the same branch). The session file only stores one `claude_session_id`. Enrichment only reviews that one conversation's JSONL, missing content from other conversations.

**Fix:** Discover ALL conversation IDs from `session(start)` journal entries (each contains `(session XXXXXXXX)`). Process all corresponding JSONLs, not just the one in frontmatter.

## Discovered

First real enrichment test on the SPEC-029 session — librarian added 2 entries, both with quality issues.
