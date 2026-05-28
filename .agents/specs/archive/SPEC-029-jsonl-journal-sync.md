---
id: SPEC-029
title: JSONL-Driven Journal Enrichment
source: spark — session 20260409-110153
created: '2026-04-09T12:30:00.000Z'
status: complete
---

# SPEC-029: JSONL-Driven Journal Enrichment

## Problem Statement

The session journal relies on the model manually calling `loaf session log` throughout a conversation. This is lossy — the model forgets, skips entries, or logs imprecisely. After context compaction, the model may have forgotten half the session's work entirely. The journal-nudge PostToolUse hook helps but is best-effort.

Meanwhile, Claude Code's JSONL conversation logs capture **everything** — every tool call, every response, every timestamp. This is the session's black box recorder.

Rather than replacing the existing journaling mechanism (which works well enough for mechanical events via PostToolUse hooks), this spec adds a **complementary enrichment step** that reviews the JSONL after the fact and fills in what the model missed — decisions, discoveries, and context that didn't get logged in the moment.

## Strategic Alignment

- **Vision:** Advances "Autonomous Execution" — journal becomes more complete and reliable without increasing the model's bookkeeping burden during work.
- **Personas:** Benefits all Loaf users — enrichment runs at lifecycle points, invisible during active work.
- **Architecture:** The CLI extracts a clean conversation summary from the JSONL (including subagent transcripts), then delegates to the librarian agent via `claude --agent librarian -p` for semantic review and journal writing. Two concerns: deterministic extraction (CLI) and judgment-based enrichment (librarian).

## Solution Direction

Add a `loaf session enrich` command that extracts a conversation summary from the JSONL log and delegates to the librarian agent for journal enrichment.

### Architecture — CLI extracts, librarian enriches

**CLI command (`loaf session enrich`)** — discovery, extraction, invocation, watermark:
1. Find the active session file (or accept a specific file argument)
2. Read `claude_session_id` and `enriched_at` from session frontmatter
3. Discover the JSONL path via project directory + session ID
4. **Extract conversation summary** from JSONL + subagent transcripts → write to `.agents/tmp/<session-id>-enrichment.txt`
5. If summary is empty (no new entries since `enriched_at`), exit 0 (no-op)
6. Spawn `claude --agent librarian -p` with the temp file path (isolated from Loaf hooks)
7. On success: advance `enriched_at` to the **latest JSONL entry timestamp** in the summary
8. On failure: do NOT advance `enriched_at`, report error

**JSONL extractor** (`cli/lib/journal/extractor.ts`) — deterministic, testable:
1. Read the main JSONL line-by-line
2. Discover and read subagent JSONLs from `<session_id>/subagents/agent-*.jsonl`
3. Parse each line as JSON, filter to `user` and `assistant` types only
4. Apply `enriched_at` timestamp cutoff (only entries after last enrichment)
5. From `user` entries: extract `message.content` text
6. From `assistant` entries: extract `text` blocks (full text) and `tool_use` blocks (name + key params)
7. Skip `thinking` blocks
8. Include subagent content with `[Subagent: {description}]` markers (from `*.meta.json`)
9. Enforce **100KB summary cap** — truncate oldest entries first (keep newest, most likely to contain unlogged decisions)
10. Track and return the **latest JSONL timestamp** in the processed entries (for watermark advancement)
11. Write summary to `.agents/tmp/<session-id>-enrichment.txt`

The `.agents/tmp/` directory is gitignored. Temp files persist until housekeeping archives the session, enabling iterative enrichment (re-run with different prompts, retry after failure). Housekeeping cleans up temp files when moving sessions to archive.

**Librarian agent** — judgment-based, uses Read + Edit on `.agents/` files:
1. Read the conversation summary from `.agents/tmp/<session-id>-enrichment.txt` (via Read tool)
2. Read the existing session journal (via Read tool)
3. Identify decisions, discoveries, and context not already captured
4. Append missing entries to the session journal (via Edit tool)

Both the temp file and session file are within `.agents/`, so the librarian's scope constraint is satisfied.

### Why split extraction from enrichment?

The JSONL is ~2/3 noise (progress events, hook events, attachment deltas). Even small files have this ratio. Empirical data: largest file 8.2MB/1502 lines, only ~1035 relevant entries. Pre-filtering is always worthwhile:
- **Focused input** — librarian gets only the conversation, not system noise
- **Lower token cost** — summary is a fraction of the raw JSONL size
- **Bounded** — 100KB cap prevents context blowout on large sessions
- **Testable** — extraction is deterministic and unit-testable; enrichment quality is iterated via prompt
- **One code path** — no conditional logic based on file size

### Extracted summary format

The extractor produces a readable text summary:

```
[2026-04-10 14:21] User: reshape the spec for enrich-only
[2026-04-10 14:22] Assistant: Here's the reshaped spec. Key changes: ...
[2026-04-10 14:23] Tool: Edit .agents/specs/SPEC-029.md
[2026-04-10 14:25] Tool: Bash — git commit -m "feat: reshape SPEC-029"
[2026-04-10 14:26] User: looks good, commit it
[2026-04-10 14:26] Assistant: Committed. The branch now has 4 commits ahead.

--- Subagent: Research --agent flag resolution ---
[2026-04-10 14:30] Assistant: The --agent flag resolves from plugins → ...
[2026-04-10 14:31] Tool: WebFetch https://code.claude.com/docs/en/sub-agents
```

Rules:
- Timestamps from JSONL `timestamp` field, converted to local `YYYY-MM-DD HH:MM` format
- User messages: full text content
- Assistant text blocks: full text (richest signal for decisions/discoveries)
- Tool_use blocks: `Tool: {name}` + key param (command for Bash, file_path for Read/Edit/Write)
- Skip tool results (large, noisy — the model's text already summarizes outcomes)
- No thinking blocks
- Subagent content separated by `--- Subagent: {description} ---` markers
- 100KB cap with tail-priority (oldest entries dropped first)

### What stays the same

- **Mechanical events** (commits, PRs, merges) — existing PostToolUse hooks handle these. No changes.
- **Skill self-logging** — Skills continue to log their own invocation. No changes.
- **`loaf session log`** — Remains available for manual logging. No changes.
- **Journal format** — Still `[YYYY-MM-DD HH:MM] type(scope): description`. No changes.

### JSONL conversation log discovery

**Verified:** Claude Code names JSONL files as `<session_id>.jsonl` in the project config directory. Subagent transcripts live in `<session_id>/subagents/agent-*.jsonl` with `agent-*.meta.json` for context.

Discovery strategy:
1. Read `claude_session_id` from session frontmatter
2. Derive project directory: `${CLAUDE_CONFIG_DIR}/projects/` + path-to-dashed-name of cwd
3. Construct main JSONL path: `<project-dir>/<session_id>.jsonl`
4. Discover subagent dir: `<project-dir>/<session_id>/subagents/`
5. Fallback: scan project directory for `<session_id>.jsonl` if path construction fails

The project hash directory name is derived from the working directory path (leading dash, path separators become dashes). Example: `/Users/levifig/Code/levifig/projects/loaf` → `-Users-levifig-Code-levifig-projects-loaf`. Do NOT hardcode it — discover at runtime.

### JSONL structure reference

Empirical data (257 files, largest 8.2MB/1502 lines):

| JSONL `type` | Keep? | What's in it |
|--------------|-------|-------------|
| `user` | **Yes** | `message.content` — user requests, decisions |
| `assistant` | **Yes** | `message.content` — text blocks, tool_use blocks |
| `progress` | No | Streaming progress (~1200 entries in big files, largest noise source) |
| `agent_progress` | No | Subagent progress events |
| `hook_progress` | No | Hook execution progress |
| `attachment` | No | System/config deltas |
| `queue-operation` | No | Internal bookkeeping |

Within assistant `message.content` (array of blocks):
- `{type: "text", text: "..."}` → **keep full text**
- `{type: "tool_use", name: "...", input: {...}}` → **keep name + key param**
- `{type: "thinking", ...}` → **skip**

Subagent meta: `{"agentType":"Explore","description":"Research --agent flag resolution"}`

### `enriched_at` watermark

Session frontmatter gains an `enriched_at` field — ISO 8601 timestamp representing "the latest JSONL entry we've reviewed":

```yaml
---
branch: feat/some-feature
status: active
enriched_at: '2026-04-10T14:30:00.000Z'
---
```

**Critical:** `enriched_at` advances to the **latest JSONL entry timestamp in the extracted summary**, not the current wall-clock time. This ensures the watermark reflects what was actually reviewed.

**Advancement rules:**
- **Summary non-empty + agent succeeds (exit 0):** Advance `enriched_at` to latest JSONL timestamp. The content was reviewed, regardless of whether the librarian added entries.
- **Summary empty (no entries after cutoff):** No-op. `enriched_at` stays unchanged. Exit 0.
- **Agent fails (exit non-zero):** Do NOT advance. The content was not successfully reviewed. Exit 1.

**The CLI writes `enriched_at:`, not the librarian.** Uses existing `readSessionFile()` + `writeFileAtomic()` helpers for safe frontmatter updates.

### Helper session isolation

The enrichment agent runs as a separate `claude -p` process. Without isolation, it would trigger Loaf's SessionStart/SessionEnd hooks, creating spurious session files and potentially interfering with the active session.

**Isolation mechanism:** The CLI sets `LOAF_ENRICHMENT=1` in the **spawned child process's environment only** — never on the parent process. This is process-scoped: other `claude` sessions in other terminals never see it. The child's own subprocesses (e.g., if the librarian calls `loaf session log` via Bash) DO inherit it, which is correct — anything the enrichment agent spawns should also skip session hooks.

```typescript
// Spawning (in the CLI):
spawn('claude', args, { env: { ...process.env, LOAF_ENRICHMENT: '1' } })

// At top of session start/end actions:
if (process.env.LOAF_ENRICHMENT === '1') {
  process.exit(0);
}
```

**Why not `--bare`?** `--bare` skips hooks but also skips OAuth/keychain reads, requiring API key auth. This breaks for subscription users. The env var approach works with any auth method and only suppresses Loaf's own hooks.

**Full invocation:**

```bash
LOAF_ENRICHMENT=1 claude --agent librarian -p \
  --no-session-persistence \
  --permission-mode acceptEdits \
  --max-turns 10 \
  "Enrich the session journal at <session-path>.
   Conversation summary: .agents/tmp/<session-id>-enrichment.txt
   Read both files. Identify missing decisions, discoveries, context.
   Append entries to the journal section. Do not edit frontmatter."
```

Flags:
- `LOAF_ENRICHMENT=1` — suppresses Loaf hook side effects (child-process-scoped)
- `--agent librarian` — librarian profile (session/journal expertise)
- `-p` — non-interactive, synchronous (CLI waits for exit)
- `--no-session-persistence` — don't save enrichment session to disk
- `--permission-mode acceptEdits` — auto-approve Read + Edit without prompts
- `--max-turns 10` — bounded execution
- `--model` (optional) — passed through from CLI's `--model` flag

### `--dry-run` implementation

Two-layer approach:
1. **Technical:** `--disallowedTools "Edit,Write"` strips editing tools from the agent's context
2. **Behavioral:** Append to prompt: "Output the entries you would add, but DO NOT edit any files."

For dry-run, capture the agent's text output and print to stdout. If `--disallowedTools` doesn't override agent-level tools, the behavioral instruction catches it.

### Concurrency

**Wrap flow (active session):** Synchronous. The CLI spawns the librarian via `-p` and waits for exit. The main conversation is blocked during enrichment. Sequence:
1. Wrap skill calls `loaf session enrich` → CLI spawns librarian → **blocks**
2. Librarian reads session, appends entries, exits
3. CLI updates `enriched_at` (with file lock)
4. Main conversation continues with wrap summary generation

No concurrent edits — the librarian finishes before the main conversation proceeds.

**Housekeeping flow (other sessions):** Housekeeping **only enriches sessions with status `stopped` or `done`** — never `active`. An active session may be in use by another conversation. Stopped/done sessions are idle and safe to enrich. The current session's enrichment is handled by wrap, not housekeeping.

### `--agent` resolution

`--agent librarian` resolves from: plugins → `.claude/agents/` → built-in.

Today, the librarian is defined in the Loaf plugin (`plugins/loaf/agents/librarian.md`). This works when Loaf is installed as a Claude Code plugin.

**If `--agent librarian` is not found:** Exit 1 with error: "Librarian agent not found. Ensure Loaf is installed (`loaf install --to claude-code`)." No degraded fallback — a prompt-file fallback can't recreate the agent's model/tools contract.

**Future:** If Loaf moves away from the plugin system, agents move to `.claude/agents/` or `~/.config/claude/agents/`, both of which `--agent` discovers. No code change needed — only the install target changes.

### Librarian profile update

The librarian agent profile (`content/agents/librarian.md`) gets one addition to "What You Tend":

```
- **Journal enrichment** — when invoked with a conversation summary, identify
  missing semantic entries (decisions, discoveries, context) and append them
  to the session journal. The conversation summary is pre-filtered by the CLI;
  you receive clean text, not raw JSONL.
```

**No scope changes needed.** Both the temp file (`.agents/tmp/`) and the session file (`.agents/sessions/`) are within `.agents/`. The constraint "Scope all file operations to `.agents/` paths" is naturally satisfied.

### Lifecycle integration

| Trigger | How | Scope |
|---------|-----|-------|
| **Wrap skill** | Calls `loaf session enrich` | Current active session |
| **Housekeeping skill** | Calls `loaf session enrich <file>` | `stopped` and `done` sessions with `claude_session_id` |
| **Manual** | User runs `loaf session enrich` | Any specified session |

No Stop hook — enrichment spawns a separate agent, too expensive for every turn.

### CLI interface

```bash
loaf session enrich              # Enrich the active session
loaf session enrich <file>       # Enrich a specific session file
loaf session enrich --dry-run    # Show what would be added without writing
loaf session enrich --model <m>  # Override model for the librarian call
```

**Exit codes:**
- `0` — Success (entries added or already up-to-date / no-op)
- `1` — Error (JSONL not found, `claude` not available, agent not found, agent failed)
- Errors go to stderr with actionable messages

## Scope

### In Scope
- `loaf session enrich` CLI command with `--dry-run` and `--model` flags
- **JSONL extractor** (`cli/lib/journal/extractor.ts`) — line-by-line parse, type filter, timestamp cutoff, subagent discovery, summary cap, latest-timestamp tracking, writes to `.agents/tmp/`
- `.agents/tmp/` gitignored directory for extractor output (persists for iterative enrichment, cleaned by housekeeping)
- JSONL + subagent transcript discovery logic
- Agent invocation via `claude --agent librarian -p` with `LOAF_ENRICHMENT=1` isolation
- `enriched_at` watermark management (CLI writes, advances to JSONL timestamp)
- `LOAF_ENRICHMENT` env var check in SessionStart/SessionEnd hooks
- Librarian profile update: enrichment section in "What You Tend"
- Wrap skill integration: call enrich before wrap-up
- Housekeeping skill integration: call enrich on `stopped`/`done` sessions
- Tests: extractor unit tests, discovery tests, integration smoke test

### Out of Scope
- Changing existing PostToolUse hooks (commits, PRs, merges — they work fine)
- Adapters for other harnesses (Cursor, OpenCode, etc.)
- Changing the journal entry format
- Stop hook for enrichment (too expensive per-turn)
- Contract migration (no docs changes needed — enrichment is additive)
- Degraded fallback when librarian agent is not found

### Rabbit Holes
- **Full JSONL parser with schema validation** — Overkill. Line-by-line filter with defensive JSON.parse is sufficient.
- **Running on every Stop** — Spawning a separate agent per turn is too expensive.
- **`--bare` mode for isolation** — Breaks OAuth. Use `LOAF_ENRICHMENT` env var instead.
- **Fallback to `--append-system-prompt-file`** — Can't recreate agent contract. Error out instead.
- **Replacing PostToolUse hooks** — The hooks work. Enrichment is complementary, not a replacement.
- **Parsing thinking blocks** — Internal reasoning, not journal events. Skip.
- **Passing raw JSONL to the librarian** — Even small files are 2/3 noise. Always pre-filter to `.agents/tmp/`.
- **Inlining summary in the prompt** — Summary can be large. Write to temp file instead; librarian reads with Read tool (supports pagination for large summaries).

### No-Gos
- Don't write to the JSONL — it's read-only, owned by Claude Code
- Don't hardcode the project hash path — discover at runtime
- Don't swallow errors — non-zero exit from the agent must surface
- Don't let the librarian edit frontmatter — CLI handles `enriched_at` safely
- Don't advance `enriched_at` on agent failure
- Don't advance `enriched_at` to current time — use latest JSONL entry timestamp
- Don't enrich `active` sessions during housekeeping — only `stopped`/`done`

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| JSONL format changes between Claude Code versions | Medium | Medium | Extractor uses defensive JSON.parse per line. Unknown types skipped. |
| JSONL naming convention changes | Low | High | Filesystem scan fallback in discovery logic. |
| `claude` binary not available | Low | High | Check before spawning. Clear error message. |
| `--agent librarian` not found | Low | High | Exit 1 with clear install instructions. No degraded fallback. |
| Librarian produces low-quality entries | Medium | Low | `--dry-run` for preview. Prompt iterated over time. |
| Summary exceeds 100KB cap | Low | Low | Truncation drops oldest entries. Warning to stderr. |
| `--disallowedTools` doesn't override agent tools | Low | Low | Behavioral prompt instruction as backup for dry-run. |
| Token cost of agent invocation | Medium | Low | Only at lifecycle points. `--model` for cost control. |
| `LOAF_ENRICHMENT` env var not checked by future hooks | Low | Medium | Document in CLAUDE.md. Centralize check in a shared helper. |
| Subagent transcript format changes | Low | Low | Meta.json parsing is defensive. Missing subagents = skip gracefully. |

## Test Conditions

### Extractor
- [ ] Filters JSONL to only user/assistant entries
- [ ] Applies `enriched_at` timestamp cutoff correctly
- [ ] Skips thinking blocks from assistant messages
- [ ] Extracts tool name + key param from tool_use blocks
- [ ] Handles malformed JSONL lines gracefully (skip + warning)
- [ ] Discovers and includes subagent transcripts with markers
- [ ] Enforces 100KB summary cap (oldest entries dropped first)
- [ ] Returns latest JSONL timestamp from processed entries
- [ ] Returns empty result when no entries match cutoff

### CLI command
- [ ] Discovers JSONL path from session_id + project directory derivation
- [ ] Falls back to directory scan when direct path fails
- [ ] Validates: claude binary, session_id, JSONL existence
- [ ] Spawns agent with `LOAF_ENRICHMENT=1` in environment
- [ ] `enriched_at` advances to latest JSONL timestamp (not current time)
- [ ] `enriched_at` does NOT advance when agent fails
- [ ] `enriched_at` does NOT advance on empty summary (no-op)
- [ ] `--dry-run` shows entries without writing them
- [ ] `--agent librarian` not found → exit 1 with install instructions

### Isolation
- [ ] `LOAF_ENRICHMENT=1` causes SessionStart hook to exit early
- [ ] `LOAF_ENRICHMENT=1` causes SessionEnd hook to exit early
- [ ] Enrichment does NOT create spurious session files

### Integration
- [ ] Wrap skill calls enrich before generating wrap-up report
- [ ] Housekeeping enriches only `stopped`/`done` sessions, not `active`
- [ ] `loaf build` succeeds with new command registered

## Priority Order

1. **Extractor + CLI command + hook isolation + librarian update** — JSONL extractor with subagent support, `loaf session enrich` command, JSONL discovery, agent invocation, `enriched_at` watermark, `LOAF_ENRICHMENT` env var in hooks, librarian profile update. Go/no-go: enrichment adds entries to journal via librarian, no spurious sessions created.
2. **Lifecycle integration** — Wrap and housekeeping skill updates. Go/no-go: enrichment runs automatically at session boundaries with correct scope.
