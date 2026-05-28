---
title: "Harness-portable session enrichment (decouple /wrap from Claude Code JSONL)"
captured: 2026-05-18T23:54:26Z
status: raw
tags: [session, enrichment, harness-portability, wrap, acp]
related: [SPEC-032, SPEC-036]
---

# Harness-portable session enrichment (decouple /wrap from Claude Code JSONL)

## Nugget

`/wrap` and post-compact preservation enrich session journals by reading `<claude_session_id>.jsonl` from the Claude Code project dir. Both inputs (the ID and the JSONL file itself) are unavailable when Loaf runs under ACP — different harness, no Claude Code hook stdin carrying `session_id`, no JSONL written. Result: enrichment silently no-ops outside Claude Code. Proposed direction is a **hybrid** model: primary path is **agent-driven** — `/wrap` prompts the current agent to summarize its own conversation context into journal entries; works in every harness, no external transcript needed. Optional **JSONL boost** layered on top when running under Claude Code, to recover decisions/discoveries from pre-compaction conversation the agent no longer sees in-context.

## Problem/Opportunity

- **Functional gap, not cosmetic.** Cross-harness Loaf users (Zed agent panel via ACP, future Cursor/opencode integrations, etc.) lose enrichment entirely without noticing — the journal is missing the structured decision/discovery entries `/wrap` would have appended.
- **Underlying assumption to undo.** The current design assumes Claude Code is the only host. Loaf builds to 6 targets but enrichment only works in one of them.
- **The agent already has the context.** Inside any harness, the agent invoking `/wrap` *is* the conversation. Asking the agent to summarize itself dodges the per-host transcript-location problem entirely.

## Initial Context

- Surfaced 2026-05-19 during SPEC-036 shaping when discussing why every `loaf session log` call emits `WARN: no session_id signal` in ACP context.
- Adjacent to **SPEC-032** (claude_session_id routing) — that spec solved "which session does this command belong to?". This idea is the sibling concern: "where does enrichment get conversation history from?".
- Adjacent to **SPEC-036** (worktree-aware storage routing) — same shaping conversation, different concern. SPEC-036 is about *where* files live; this idea is about *how* enrichment sources its input across harnesses.
- Existing flow to study: `cli/commands/session.ts:967-1002` (`deriveClaudeProjectDir`, `buildEnrichmentPrompt`).
- Open shape questions for future `/loaf:shape`:
  - Does the agent-driven path need any host-side primitive (e.g., a `/transcript` API), or can the skill prompt do it all from in-context state?
  - When both paths are available (Claude Code), should JSONL be canonical and agent-driven be the fallback, or vice versa? Concretely: which one wins if they disagree on what happened?
  - Should this generalize to a `TranscriptSource` interface (per the original shape table in the SPEC-036 chat), or stay simpler — agent-driven only, with JSONL as an inline branch?
  - Does post-compact preservation need its own treatment, or does the same hybrid serve it?

---

*Captured via /loaf:idea — shape with /loaf:shape when ready*
