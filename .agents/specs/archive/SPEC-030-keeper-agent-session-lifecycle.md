---
id: SPEC-030
title: Librarian agent — session lifecycle management via task-driven journaling
source: 20260409-140251-session-lifecycle-states.md
created: '2026-04-09T15:30:00.000Z'
status: complete
---

# SPEC-030: Librarian Agent — Session Lifecycle Management via Task-Driven Journaling

## Problem Statement

Session lifecycle management has two problems:

1. **Prompt hooks misused for advisory work.** The Stop hook (`session-state-update`), `implement-routing`, and `journal-nudge` are prompt-type hooks used for side effects (writing summaries, nudging behavior). Prompt hooks are binary gates, but we use them for advisory work — the result is conversation bleed, blocking legitimate tool calls, and noisy feedback injected into the user's flow.

2. **No dedicated agent for session lifecycle.** Session housekeeping (wrap-ups, state updates, archive readiness) falls on the main conversation agent, fragmenting its attention between the user's work and bookkeeping.

**Constraints discovered during implementation:** Claude Code agent hooks (`type: "agent"`) are read-only — they can Read/Grep/Glob but cannot Edit or Write. This rules out using agent hooks for session state writes. The Librarian operates as a spawned agent (via the Agent tool), not a hook agent.

## Strategic Alignment

- **Vision:** Loaf agents have clear, lore-grounded responsibilities. The fellowship gains a dedicated steward for the living record of work.
- **Personas:** All agent users benefit — the Warden stops doing bookkeeping, Smiths stop being interrupted by stop hooks, sessions get consistent lifecycle management.
- **Architecture:** Uses Claude Code's native `agent` hook type (haiku subagent with tool access, up to 50 turns). Replaces prompt hooks with the correct hook primitive.

## The Librarian

**Librarians** — Ents who tend the living record. Patient, thorough, and long-memoried, they shepherd session files through their lifecycle as Treebeard shepherded the forests. Read + Edit access to session files and knowledge artifacts. They do not forge code or scout the web — they tend what already exists.

Instance names follow the Entish tradition — slow, deliberate, tree-rooted:
`{TreeName} — {concise purpose description}`
Example: `Bregalad — session state update on stop`

### Fellowship Table (Updated)

| Profile | Concept | Race | Tool Access | Use For |
|---------|---------|------|-------------|---------|
| **implementer** | Smith | Dwarf | Full write | Code, tests, config, docs |
| **reviewer** | Sentinel | Elf | Read-only | Audits, reviews |
| **researcher** | Ranger | Human | Read + web | Research, comparison |
| **librarian** | Librarian | Ent | Read + Edit (.agents/) | Session lifecycle, state, wrap |
| **background-runner** | System | — | Read + Edit | Async non-blocking tasks |
| **context-archiver** | System | — | Read + Edit + Serena | Session preservation (absorbed by Librarian) |

### Behavioral Contract

- Tend session files: Current State, journal quality, wrap summaries, lifecycle transitions.
- Never modify code, tests, or config — only `.agents/` artifacts.
- Never research, review, or orchestrate — those are other profiles' work.
- Work quickly and silently. The user should not notice the Librarian unless something is wrong.
- Default to sonnet model for summary quality. Downgrade to haiku after validating output quality — the summaries need to be contextually useful, not just mechanical.

## Solution Direction

### Track 1: Hook Cleanup + Task-Driven Journaling

**Remove problematic hooks:**
- `session-state-update` (Stop prompt) — removed. Caused conversation bleed.
- `implement-routing` (PreToolUse prompt on Edit|Write) — removed. Blocked legitimate edits.
- `journal-nudge` (PostToolUse prompt on Agent|WebFetch|WebSearch) — removed. Replaced by task events.

**Add event-driven journal hooks:**
- `TaskCompleted` session event → `loaf session log --from-hook` → logs `task(completed|cancelled): owner: subject`
- `UserPromptSubmit` session event → `loaf session context --for-prompt` → injects session context + orchestration conventions

**Replace PostCompact bash script with CLI:**
- `loaf session context --for-resumption` — prints rich resumption context (state + spec + journal + git)

**Session consolidation on start:**
- `findSessionByClaudeId` scans active + archive, prioritizes current branch, merges split sessions, deletes duplicates

**Journal entry sources (layered):**

| Source | Mechanism | When |
|--------|-----------|------|
| Skills | `loaf session log` in Critical Rules | Self-logging on invocation |
| Git events | PostToolUse command hooks | Commits, PRs, merges |
| Task events | `TaskCompleted` session hook | Task completed/cancelled |
| Context | `UserPromptSubmit` command hook | Every user prompt |
| PreCompact | Prompt hook (stays) | Emergency journal flush |

### Track 2: Librarian Agent Profile

Create a dedicated agent profile for session lifecycle work, spawned via the Agent tool (not hooks):

- `content/agents/librarian.md` — Ent lore, behavioral contract
- `content/agents/librarian.claude-code.yaml` — sonnet model, orchestration skill, Read+Edit+Glob+Grep tools
- SOUL.md updated with Librarian entry in fellowship table
- Used by `/wrap`, `/housekeeping`, and manual agent spawning

### Track 3: Wrap Skill — Interactive + Scripted Session Close

`/wrap` is a hybrid: the interactive agent writes the summary (has conversation context), then scripted CLI commands handle the bookkeeping. Callable from `/release` as a sub-skill.

**Interactive agent steps (conversation context required):**

| Step | Action | Why interactive |
|------|--------|----------------|
| Flush journal | Log unrecorded decisions, discoveries, progress | Only the conversation agent knows what it hasn't logged |
| Write wrap summary | Synthesize what was done, why, loose ends | Needs full conversation context — journal alone can't capture "why it was hard" |
| Write final state | Wrap summary replaces `## Current State` | Part of the summary — the wrap report IS the final state |

**Scripted steps (CLI, deterministic):**

| Step | Action | Implementation |
|------|--------|---------------|
| Close journal | Append `session(end)` + `session(stop)` markers | `loaf session end` (already exists) |
| Mark complete | Set frontmatter `status: complete` | Part of `loaf session end` |
| Remove dangling state | Strip old `## Current State` if wrap summary exists | Scripted check in `loaf session end` |
| Persist decisions | Extract decisions to linked spec changelog | Part of `loaf session archive` |
| KB staleness | Flag knowledge files touched during session | Part of `loaf session end` |

**Flow:** Interactive agent (flush + summary) → `loaf session end` (close + cleanup)

**No archive** — session stays in `sessions/` with status `complete`. Archival is housekeeping's job.

### Track 4: Housekeeping — Fully Scripted, Trigger-Scheduled

Housekeeping is **fully scripted** — no `claude -p` needed. All operations are deterministic file scans and timestamp checks. Triggered via a state flag, not cron.

**Trigger mechanism:**

```
SessionEnd (`loaf session end`)
  → read .agents/.loaf-state → last_housekeeping older than 24h?
    → yes: set housekeeping_pending = true

SessionStart (`loaf session start`)
  → read .agents/.loaf-state → housekeeping_pending?
    → yes: run housekeeping inline, update last_housekeeping, clear flag
```

State file: `.agents/.loaf-state` (gitignored, local machine state)

**Housekeeping operations (all scripted):**

| Step | Action | Implementation |
|------|--------|---------------|
| Scan sessions | List active + stopped sessions | Read `sessions/*.md` frontmatter |
| Detect orphans | Sessions whose branch no longer exists | `git branch --list` vs session `branch:` field |
| Triage orphans | Archive empty orphans, flag non-empty for review | Count journal entries + commits (heuristic) |
| Detect splits | Multiple sessions with same `claude_session_id` | Already implemented in `findSessionByClaudeId` |
| Consolidate splits | Merge journals, delete duplicates | Already implemented in `consolidateSession` |
| Age-based archival | `complete` sessions older than N days → archive | Timestamp compare + rename to `archive/` |
| Session/spec linkage | Verify spec `session:` field → file exists | File existence check |
| Fix broken linkage | Update spec if session renamed/moved | Deterministic repair |
| KB staleness | Scan stale knowledge files | `loaf kb review` (exists) |
| Report | Print structured summary of actions taken | Scripted output |

**Future: `claude -p` for complex decisions.** If orphan triage needs genuine judgment (e.g., "this session has 50 entries but the branch was force-deleted — what happened?"), a piped Librarian can be invoked. But day-one implementation is pure scripting.

### Track 5: Absorb Context Archiver (Future)

The context-archiver agent handles PreCompact session preservation. The Librarian's responsibilities are a superset — absorb context-archiver into the Librarian profile. Deferred: the PreCompact prompt hook + compact.sh script work adequately today.

## Scope

### In Scope
- Librarian agent profile (`content/agents/librarian.md`) with lore and behavioral contract
- Remove `session-state-update`, `implement-routing`, and `journal-nudge` hooks
- TaskCompleted session hook for journal auto-entries
- UserPromptSubmit hook for orchestration context injection
- PostCompact CLI command for rich resumption context
- Session `claude_session_id`-first lookup with split consolidation
- SOUL.md update with Librarian entry in fellowship table
- Wrap skill: composable session close, callable from `/release`
- Housekeeping skill: scheduling-ready, autonomous, idempotent

### Out of Scope
- JSONL enrichment pipeline (SPEC-029)
- Journal quality analysis (future Librarian capability)
- Context-archiver absorption (deferred — current PreCompact mechanism works)

### Rabbit Holes
- Making the Librarian a full orchestrator — it tends, it doesn't coordinate
- Giving the Librarian write access to code — it touches only `.agents/` artifacts
- Agent hooks for write operations — Claude Code agent hooks are read-only

### No-Gos
- The Librarian must not modify code files. Tool access is scoped to `.agents/` paths only.
- Do not re-introduce prompt hooks for advisory side effects on Stop or PostToolUse.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Task hooks fire too frequently, cluttering journal | Low | Low | Only log TaskCompleted; skip in_progress and other status changes |
| Agent forgets to use Task* tools consistently | Medium | Medium | UserPromptSubmit hook injects orchestration conventions; hooks are catch-all, not sole source |
| Librarian model quality on sonnet insufficient for state summaries | Low | Medium | Start with sonnet; validate during /wrap testing |

## Resolved Questions

- [x] Can agent hooks scope tool access? **No — read-only (Read, Grep, Glob, WebFetch, WebSearch). No Edit/Write/Bash.** Librarian operates as spawned agent instead.
- [x] Should Librarian run on every Stop? **No — removed Stop hook entirely. Librarian is invoked explicitly via /wrap and housekeeping.**
- [x] How do journal entries get created without prompt hooks? **TaskCompleted hook, git event hooks (PostToolUse on Bash), UserPromptSubmit context injection, and skill self-logging.**

## Test Conditions

- [x] `implement-routing` no longer blocks non-implementation edits
- [x] `session-state-update` Stop hook removed — no conversation bleed
- [x] `journal-nudge` removed — no prompt-based advisory noise
- [x] SOUL.md and fellowship table updated with Librarian profile
- [x] Librarian agent profile builds to all targets
- [x] `loaf build` succeeds, typecheck passes, tests pass
- [x] TaskCompleted session hook → journal entry logged (hook registered, not yet verified in live test)
- [x] UserPromptSubmit hook injects session context on every prompt
- [x] Live test: no Stop hook feedback bleeding into conversation
- [x] PostCompact prints rich resumption context (state + spec + journal + git)
- [x] Session consolidation: splits detected and merged on session start
- [x] `/wrap` closes session (complete status), does not archive
- [ ] `/wrap` callable from `/release` without interactive prompts (release flow not yet tested)
- [x] `/housekeeping` runs autonomously (no AskUserQuestion)
- [x] `/housekeeping` archives completed sessions past age threshold

## Priority Order

1. **Track 1: Hook Cleanup + Task-Driven Journaling** — Remove problematic hooks, add task/session event hooks, extend CLI. ✅
2. **Track 2: Librarian Agent Profile** — Agent profile with lore, behavioral contract, sidecar. ✅
3. **Track 3: Wrap Skill** — Composable session close, callable from `/release`. ✅
4. **Track 4: Housekeeping Skill** — Scheduling-ready autonomous maintenance. ✅
5. **Track 5: Context Archiver Absorption** — Absorbed into Librarian. ✅

## Changelog

- 2026-04-10 — Session feat/librarian-session-lifecycle archived: 12 decision(s) extracted
  [2026-04-09 00:22] decision(spec-030): Tracks 1-2 complete, Tracks 3-4 specced. Wrap and housekeeping deferred to separate implementation passes.
  [2026-04-09 00:22] decision(architecture): wrap is interactive+scripted (no pipe), housekeeping fully scripted, triggered via .loaf-state flag on SessionEnd/SessionStart
  [2026-04-09 00:22] decision(naming): keeper renamed to librarian — Ents who tend the library of session records
  [2026-04-09 00:45] decision(session-lifecycle): new claude_session_id creates fresh session file, closes stale one with stopped status
  [2026-04-09 00:45] decision(session-lifecycle): filename collision avoidance via counter suffix when sessions created in same second
  [2026-04-10 03:36] decision(track-5): absorbed context-archiver into Librarian — deleted agent, updated all fellowship tables and orchestration references
  [2026-04-10 10:40] decision(journal): git events auto-logged by hooks — removed manual commit logging from Journal Discipline instruction
  [2026-04-10 11:30] decision(hooks): PreCompact prompt hooks unsupported outside REPL — all hooks use type:command
  [2026-04-10 11:30] decision(hooks): consolidated 3 journal PostToolUse hooks into 2 (git commit + gh pr) with specific if conditions
  [2026-04-10 11:30] decision(hooks): all hooks moved from plugin.json to hooks/hooks.json for reliable registration
  [2026-04-10 11:30] decision(task-tracking): task-before-tool rule — create task before any mutating tool use, no threshold debate
  [2026-04-10 11:30] decision(journal): TaskCompleted logs description not subject — richer context for compaction recovery
