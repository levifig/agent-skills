---
id: SPEC-027
title: Implement ad-hoc task creation and session stability
source: direct
created: '2026-04-07T00:01:56.000Z'
status: complete
---

# SPEC-027: Implement Ad-hoc Task Creation and Session Stability

## Problem Statement

Two related friction points in the `/implement` workflow:

**1. Ad-hoc input is a dead end.** When `/implement` receives free-text description instead of `TASK-XXX` or `SPEC-XXX`, it stops and asks the user whether to create a Linear issue or local task. This defeats the purpose — the user already expressed intent by invoking `/implement`. The description *is* the task; the skill should create it and proceed.

**2. Subagent spawns destroy the parent session.** Each `Task` tool invocation triggers `SessionStart`, which archives the current session and creates a new one. During a single conversation that spawns 2 explore agents, this produced 6 sessions in 4 minutes — each one archiving the last. Journal entries written between spawns are lost to archived files.

**3. Compaction loses context that's already externalized.** When Claude Code compacts the conversation, it produces a lossy summary. But with disciplined journaling, all important state (decisions, discoveries, progress, blockers) already lives in the session file. The session journal should serve as the compaction resumption protocol — not a separate snapshot mechanism.

## Strategic Alignment

- **Vision:** Autonomous execution (Phase 5) requires stable sessions that survive agent delegation. Ad-hoc task creation removes friction on the path to overnight `loaf implement`.
- **Personas:** Power users who think in terms of "do this" rather than "work on TASK-042." Also: any `/implement` session that spawns subagents (which is all of them).
- **Architecture:** Extends existing task-coupled flow (no new abstractions). Session stability uses Claude Code's existing `agent_id` hook input — no custom locking.

## Solution Direction

### Part A: Ad-hoc Task Auto-Creation

When `/implement` receives free text that doesn't match any known pattern (`TASK-XXX`, `SPEC-XXX`, Linear ID):

1. **Parse the description.** If single sentence → use as task title. If multi-sentence → first sentence = title, remainder = acceptance criteria in task body.
2. **Create the task.** `loaf task create --title "<parsed title>"` with criteria written to the task `.md` file body.
3. **Fall through to task-coupled flow.** The result is a `TASK-XXX` ID that enters the existing session/plan creation pipeline unchanged.

**Error case:** If input *looks* like a task ID (`TASK-XXX` pattern) but doesn't exist in TASKS.json, show an error with the option to create a new task from the raw text. Don't silently create — the user probably has a typo.

### Part B: Session Stability via Subagent Detection

Claude Code's hook JSON input includes `agent_id` only when running inside a subagent. This is the discriminator.

**Changes to `loaf session start`:**

1. Parse hook JSON from stdin (when invoked as a hook command).
2. If `agent_id` is present in the input → **exit 0 immediately**. No session creation, no archiving, no journal entry. Subagents are session-unaware.
3. If no `agent_id` → proceed with normal session lifecycle (existing behavior).

**Session ID tagging:**

1. Extract `session_id` from hook JSON input during SessionStart.
2. Write it into the session file frontmatter as `claude_session_id`.
3. On subsequent SessionStart calls (new Claude conversation, same branch), compare the incoming `session_id` with the stored one:
   - **Same ID** → same conversation, resume session (no PAUSE header)
   - **Different ID** → new conversation, write PAUSE header, update `claude_session_id`
4. This replaces the current "archive old session, create new one" behavior with in-place continuation.

### Part C: Compaction-Aware Sessions

The session journal already captures decisions, discoveries, blockers, and progress. With `claude_session_id` from Part B, the model can detect post-compaction state (same session_id, no memory of recent work). The session file becomes the resumption protocol.

**PreCompact flow:**

1. The existing prompt nudge fires, telling the model to flush un-journaled entries via `loaf session log`.
2. The `compact.sh` hook writes a `compact(session): context compaction triggered` marker.
3. No file snapshots needed — the journal IS the snapshot.

**Post-compaction resumption:**

1. SessionStart fires (same `claude_session_id` → no PAUSE header, no new session).
2. The model reads the session file's `## Journal` section to reconstruct context.
3. Recent journal entries (decisions, progress, current task) provide the resumption state.

**PreCompact wrap-up and post-compaction resume:**

1. Expand the PreCompact prompt nudge to request a condensed wrap-up: flush journal entries AND write a state summary to the session file's `## Current State` section.
2. Consider a command hook (`loaf wrap --pre-compact`) as a harder guarantee — generates a machine-readable summary written to the session file before compaction.
3. Post-compaction: a `PostCompact` prompt nudge tells the model to re-read the session file. The summary it wrote minutes ago becomes the resumption context.

**Cleanup:**

1. Remove or retire `archive-context.sh` — it references stale `.work/` paths and the `.context-snapshots/` approach is superseded by the journal-as-resumption model.
2. Strengthen the PreCompact nudge to emphasize that journal entries are compaction insurance, not just audit trail.

### Part D: Session Management Policy and Rename Nudges

**Compact vs new session policy** — document in orchestration skill's `references/session-management.md`:

| Scenario | Action |
|----------|--------|
| Picking up previous work, same scope | Compact or resume existing conversation |
| Switching to entirely different scope | New conversation (new session) |
| Finished and archived a spec | New conversation |
| Context full mid-task | Auto-compact (journal survives) |
| Quick unrelated question | New conversation (don't pollute working session) |

**`/rename` prompt nudge** — session-creating skills generate a descriptive name and suggest it:

1. After session/plan creation in `/implement`, generate a name from the task/spec context: `Suggestion: /rename SPEC-027-session-stability`
2. SessionStart output includes a suggested rename when a spec is linked to the branch.
3. The name format: `{SPEC-ID}-{short-slug}` or `{TASK-ID}-{short-slug}` depending on input.

## Scope

### In Scope
- Modify `/implement` skill's Input Detection to auto-create tasks from free text
- Smart description parsing (single vs. multi-sentence)
- Error handling for non-existent task IDs
- Subagent detection in `loaf session start` via `agent_id`
- `claude_session_id` tagging in session frontmatter
- Session resume logic based on session ID comparison
- `--force` flag on `loaf session start` to bypass subagent detection and create fresh
- Retire `archive-context.sh` and `.context-snapshots/` mechanism
- Strengthen PreCompact nudge to frame journaling as compaction insurance
- Post-compaction resumption via session file re-read
- Document "compact vs new session" policy in orchestration session management reference
- Add `/rename` prompt nudge to session-creating skills (`/implement`, SessionStart output)

### Out of Scope
- Session renaming from `/implement`
- Rolling journal per branch (captured as future idea)
- Linear issue creation from ad-hoc text (existing flow, unchanged)
- Changes to `loaf session log` (already handles missing sessions gracefully)
- Multi-repo or cross-project session coordination

### Rabbit Holes
- **`CLAUDE_ENV_FILE` bridging.** Tempting to export session_id as an env var via `CLAUDE_ENV_FILE`, but it's buggy on resume and plugin hooks. Don't go there — read from hook JSON only.
- **NLP-level description parsing.** "Smart extraction" means splitting on sentence boundaries (`. ` followed by uppercase), not running intent classification.
- **PID-based process detection.** `agent_id` is the right signal — don't try to correlate PIDs across spawns.
- **Resumption intelligence.** Post-compaction, the model reads journal entries — don't build a separate "state reconstruction" system. The journal format is already structured enough.

### No-Gos
- Don't change the existing `TASK-XXX` or `SPEC-XXX` input paths
- Don't make ad-hoc task creation interactive (zero-friction is the point)
- Don't add session management to subagents — they must remain session-unaware
- Don't archive sessions on new-conversation-same-branch — use PAUSE + resume

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| `agent_id` field removed/renamed in future Claude Code versions | Low | High | Check for field presence, not specific values. Document the dependency. |
| Smart extraction misparsing (URLs with periods, abbreviations) | Low | Low | Conservative splitting — only split on `. ` followed by uppercase letter |
| Session file grows unbounded with PAUSE/resume (no archiving) | Medium | Low | `/housekeeping` can still archive explicitly. `/wrap` suggests archival. |
| Removing `archive-context.sh` loses a safety net | Low | Low | The journal replaces it — and the script was already broken (stale `.work/` paths). |

## Open Questions

- [ ] Should `loaf session start` accept hook JSON via stdin only when invoked as a hook, or always? (Affects testability)
- [ ] Should the `claude_session_id` comparison also check `transcript_path` as a fallback signal?

## Test Conditions

- [ ] `/implement "fix the login button"` creates TASK-XXX with title "fix the login button" and proceeds to session/plan creation without user interaction
- [ ] `/implement "Fix auth flow. Tokens should rotate every 24h. Add refresh endpoint."` creates task with title "Fix auth flow" and remaining sentences as acceptance criteria
- [ ] `/implement TASK-999` (non-existent) shows error message with option to create new task
- [ ] Spawning 3 subagents from a session results in exactly 1 session file (no churn)
- [ ] `loaf session log` entries written between subagent spawns persist in the same session file
- [ ] `loaf session start` with `agent_id` in hook JSON exits 0 with no side effects
- [ ] Session frontmatter contains `claude_session_id` after SessionStart
- [ ] New Claude conversation on same branch writes PAUSE header and updates `claude_session_id`
- [ ] `loaf session start --force` creates new session regardless of agent_id
- [ ] After compaction, session file provides sufficient context to resume work without re-reading the full conversation
- [ ] `compact(session)` marker appears in journal after compaction
- [ ] `archive-context.sh` removed; no `.context-snapshots/` created on compaction
- [ ] PreCompact nudge references journal entries as compaction insurance
- [ ] PreCompact triggers condensed wrap-up (journal flush + state summary written to session file)
- [ ] `PostCompact` nudge tells model to re-read session file for resumption context
- [ ] Post-compaction, model has sufficient context to continue without asking "where were we?"
- [ ] Orchestration session management reference includes compact-vs-new-session policy
- [ ] `/implement` suggests `/rename` with a meaningful name after session creation
- [ ] SessionStart output includes suggested `/rename` when spec is linked to branch
