---
id: SPEC-032
title: Session Routing by claude_session_id
source: direct
created: '2026-04-27T22:55:00.000Z'
status: implementing
---

# SPEC-032: Session Routing by claude_session_id

## Problem Statement

`loaf session log` and three sibling commands (`archive`, plus two helper paths) route by branch only — they call `findActiveSessionForBranch(agentsDir, branch)` and ignore `claude_session_id`. When a branch has multiple sessions (one active, several stopped — common after `/clear`, compaction, or normal session lifecycle), `loaf session log` may write to a session file other than the conversation's actual one. Other commands that already received hook JSON (`session start`, `session end`, several PreCompact paths) correctly use the priority-ordered chain `findSessionByClaudeId(...) || findActiveSessionForBranch(...)`. The defect is in commands that don't currently parse hook JSON for `session_id`.

**Concrete evidence:** the most recent commit on `main`, `81b1808a chore: record session journals with dev.30 post-merge wrap (#misrouted)`, literally records that `loaf session log` wrote to the wrong session during a release. STRATEGY priority 1 makes the architectural rule explicit: *"session files are keyed by `claude_session_id`; branch is a property of the session, not its identity. `loaf session log` should route by `claude_session_id` with branch-matching as a fallback only when the incoming `session_id` is missing."*

This spec finishes a half-completed migration. The two routing helpers (`findSessionByClaudeId` and `findActiveSessionForBranch`) are correct as-is. The fix is in the call sites.

## Strategic Alignment

- **Vision:** Session reliability is foundational to Loaf's structured-execution pillar. Misrouting silently corrupts the audit trail every release, undermining every downstream feature that consumes session journals (wrap, enrich, retros).
- **Personas:** Solo developer hits this on every `/release` cycle (the `#misrouted` commit is the smoking gun). Team lead can't trust session files for retros if entries land in the wrong place.
- **Architecture:** Reinforces the architectural rule from STRATEGY priority 1. Two routing helpers already exist; this spec ensures every command uses the priority-ordered chain.
- **No strategic tensions surfaced.** This is finishing work the codebase already started.

## Solution Direction

Two changes:

1. **CLI parsing helper for hook stdin** — extract a `parseHookSessionId()` helper that reads stdin JSON when `--from-hook` is set and returns `session_id` if present. Centralizes the parsing currently inlined in a few places.

2. **Routing chain in every subcommand** — replace bare `findActiveSessionForBranch(agentsDir, branch)` with a 3-tier priority-ordered chain:

   ```
   1. session_id from --session-id <id> CLI flag (explicit override)
   2. session_id from hook stdin (only when --from-hook is set)
   3. Branch-based lookup, picking most-recently-updated active session
      ↳ When this tier fires, emit to stderr:
        "WARN: no session_id signal — falling back to branch routing
         for branch '<branch>'. Pass --session-id <id> to silence."
   ```

   The branch fallback is intentionally retained as the third tier (and only the third tier) so existing skill self-logging continues to work without an immediate refactor. The stderr warning makes branch-fallback misroutes visible in the moment instead of silently corrupting the wrong session.

## Scope

### In Scope

- New `parseHookSessionId(): string | undefined` helper in `cli/lib/session/`. Reads stdin JSON and returns `session_id` if present; returns `undefined` otherwise.
- New `resolveCurrentSession()` chain helper that runs the 3-tier resolution and returns the resolved session (or `null`).
- Refactor of routing in: `loaf session log` (line 1810), `loaf session archive` (line 2015), and any other bare-branch call sites surfaced by grep — currently lines 883, 1810, 2015, 2354 in `cli/commands/session.ts`.
- New `--session-id <id>` CLI flag on `log`, `archive`, `end`, `wrap` for explicit override (low-traffic path; supports diagnostics, tests, and future skill migration).
- Stderr warning emission when the chain falls through to branch routing.
- Tests for each subcommand under (a) `--from-hook` with `session_id` in JSON, (b) `--session-id` flag, (c) neither (branch fallback + warning).

### Out of Scope

- **No migration of existing sessions** (A1). Sessions on disk are not renamed, moved, or backfilled. Routing fix only affects new writes.
- **No environment variable fallback** (A2). `CLAUDE_SESSION_ID` reading is explicitly excluded — it would re-introduce stale-shell and cross-pane misroute risks.
- **No filename change**. Session filenames remain timestamp-based (`YYYYMMDD-HHMMSS-session.md`). `claude_session_id` is the *routing* key, not the filename.
- `loaf session start` direct-CLI mode — already uses the hook-aware chain at lines 1394 + 1398. Verify only.
- The "split detection / consolidate on start" logic from SPEC-027/030 — already correct, not touched.
- `loaf session enrich` — already routes by `claude_session_id` correctly.
- **Skill self-logging refactor** — out of scope. Branch fallback (with warning) keeps skills working today. A follow-up spec will refactor skills to source `session_id` per-process; SPEC-032 ships the warning surface that makes that future migration's regression testing tractable.

### Rabbit Holes

- **Renaming session files to `<claude_session_id>-session.md`.** Tempting but unnecessary. Filename is human-readable timestamp; routing is via frontmatter. Don't conflate.
- **Cross-session deduplication on log.** Out of scope. The fix is "write to the right session," not "merge wrong-session entries." Misrouted entries on disk stay where they are.
- **CLAUDE_SESSION_ID env var fallback.** Rejected on parallel-session safety grounds. Stale env in long-lived shells (Claude Code session A exits, shell stays open with old env, new session B starts; logs from the stale shell route to A) and cross-pane copy/paste hazards make this fragile. Claude Code does not reliably export this env var today, so it would also be forward-compatible plumbing for a feature that may never arrive.
- **Sentinel file (`.agents/.loaf-current-session`) for persistent "current session" pointer.** Rejected on the same grounds as the env var: two Claude Code instances in the same project would clobber each other's pointer on every SessionStart. The "current session" concept is **not parallel-safe** as a global file or env var — it must always be derived from per-process context (hook stdin, explicit flag, or in the future a per-process discovery mechanism that doesn't depend on shared state).
- **Auto-detecting non-TTY stdin without `--from-hook`.** Rejected. A user piping anything (`echo "decision(x): y" | loaf session log`) would get surprising parsing as hook JSON instead of entry text. Strict opt-in via `--from-hook`.
- **A new "session lookup" function.** Two helpers (`findSessionByClaudeId`, `findActiveSessionForBranch`) are enough. The chain composes them; no third helper.
- **Auto-recency heuristic for branch fallback** beyond what `findActiveSessionForBranch` already does. The existing helper picks an appropriate candidate; if it doesn't yet pick most-recently-updated active, that's a small adjustment in this spec, not a redesign.

### No-Gos

- Do not delete or merge existing branch-keyed sessions. They are historical record.
- Do not modify `findSessionByClaudeId` or `findActiveSessionForBranch` themselves — they are correct. The fix is in the call sites.
- Do not introduce *any* mechanism that stores "current session" globally (file, env var, lockfile, named pipe, shared memory, etc.). Parallel Claude Code sessions in the same project must remain isolated. The "current session" must be derived from per-process context only.
- Do not auto-detect non-TTY stdin. Strict `--from-hook` opt-in.
- Do not silently suppress the stderr warning. Branch-fallback events must be visible.

## Implementation Notes

### Routing chain helper signature

```typescript
async function resolveCurrentSession(
  agentsDir: string,
  branch: string,
  opts: {
    sessionIdFlag?: string;
    parseStdin?: boolean;  // true only when --from-hook is set
  }
): Promise<{
  filePath: string;
  data: SessionFrontmatter;
  content: string;
} | null>
```

Returns the resolved session, or `null`. Caller decides whether to error or no-op.

### Resolution order

```
1. opts.sessionIdFlag         → findSessionByClaudeId(agentsDir, opts.sessionIdFlag, branch)
2. parseHookSessionId() (when opts.parseStdin)
                              → findSessionByClaudeId(agentsDir, <parsed-id>, branch)
3. findActiveSessionForBranch(agentsDir, branch)
                              → emits stderr WARN before returning the result
```

Each step returns immediately on a non-null result. The stderr warning fires only on tier 3.

### Stderr warning message

```
WARN: no session_id signal — falling back to branch routing for branch '<branch>'.
      Pass --session-id <id> to silence.
```

The message names the branch and points at the workaround. It does NOT block, exit non-zero, or otherwise alter the command's success path. Goal: visibility, not friction.

### Subcommand migration list

Replace bare `findActiveSessionForBranch` with `resolveCurrentSession` at:

- Line 883 (helper used by some path — confirm during implementation; may need same migration or may already be reached via a hook-aware path).
- Line 1810 — `loaf session log` action body. Primary fix.
- Line 2015 — `loaf session archive` action body.
- Line 2354 — secondary path (likely housekeeping or report; confirm during implementation).

Hook-aware paths at lines 1394+1398, 1634+1635, 2671+2672, 2742+2743, 2831+2832 already use the correct pattern. Verify they still work after `resolveCurrentSession` lands; ideally migrate them to the helper for code consistency, but that is a refactor concern, not a correctness one.

### Follow-up spec footnote

Skill self-logging migration is a future spec. Today every skill calls `loaf session log "skill(name): ..."` from Bash with no `--session-id` and no `--from-hook`, falling through to branch routing (now with a visible warning). The follow-up spec will refactor skills so each call sources `session_id` from per-process context — never from a shared state file or env var. Candidate mechanisms: a hook-aware wrapper that pipes hook JSON through to skill Bash calls, or a Claude-Code-side feature that exposes per-conversation state to spawned tools. The exact mechanism is a future spec; SPEC-032 ships the stderr warning that makes the eventual cutover testable.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Backward compat: existing scripts calling `loaf session log` with no flag/stdin keep working | High | Low | Tier 3 (branch fallback) preserves current behavior. Stderr warning makes the silent path visible without breaking it. |
| Stderr warning is noisy enough that users start filtering it (`2>/dev/null`) | Medium | Medium | The warning fires only when no session_id is available — i.e., only in environments not yet migrated. Volume is bounded. Skills should pipe `--session-id` once the follow-up spec ships. |
| Hook JSON has `session_id` but it points to a session file that doesn't exist (legacy hook env, or session was deleted manually) | Low | Low | `findSessionByClaudeId` returns `null`; chain falls through to branch lookup as if no signal was given. Warning fires, branch routing proceeds. |
| Multiple active sessions on same branch with different `claude_session_id` (edge case after some bug or manual session manipulation) | Low | Low | Tiers 1–2 win over branch lookup when session_id is available. The rare tier-3 path picks most-recent active session and warns. |
| `parseHookSessionId` reads stdin destructively when called outside hook context | Medium | Medium | Only read stdin when `--from-hook` is set. Auto-detect rejected per A5/No-Gos. |
| Tier 2 (stdin parse) introduces a subtle race when stdin is being consumed elsewhere in the action | Low | Low | `parseHookSessionId` reads early, before any other stdin consumer; cache the result. |

## Open Questions

All resolved during shape:

- [x] **A1 — Migration of existing sessions:** No migration. Existing sessions stay as-is; routing fix only affects new writes.
- [x] **A2 — Environment variable fallback:** Rejected. `CLAUDE_SESSION_ID` reading is excluded on parallel-session safety grounds (stale shells, cross-pane copy/paste). Claude Code does not reliably export this env var today, making it forward-compatible plumbing for a feature that may never arrive.
- [x] **A3 — `--session-id` flag:** Add to `log`, `archive`, `end`, `wrap`. Useful for diagnostics, testing, and future skill migration.
- [x] **A4 — Branch fallback:** Drop branch as primary; keep as third-tier last-resort with stderr warning. (Option B from the shape interview.) Full branch removal is deferred to a future spec that also handles skill self-logging.
- [x] **A5 — Hook stdin parsing for non-`--from-hook` callers:** Reject auto-detection. Only parse stdin when `--from-hook` is set.

## Test Conditions

### Routing chain
- [ ] `loaf session log --from-hook` with `{"session_id": "X", ...}` on stdin writes to the session file with `claude_session_id: X`, regardless of branch. No stderr warning.
- [ ] `loaf session log "..." --session-id X` writes to the session file with `claude_session_id: X`. No stderr warning.
- [ ] `loaf session log "..."` (no flag, no `--from-hook`) writes to the most-recently-updated active session on the current branch (current behavior preserved). **Emits the stderr warning.**
- [ ] `loaf session archive` resolves the same way under all three signal conditions.
- [ ] `loaf session end --wrap` resolves the same way under all three signal conditions.
- [ ] When `findSessionByClaudeId` returns `null` (session_id given but no matching file), the chain falls through to branch lookup AND emits the stderr warning.
- [ ] No new session file is created by any of these commands. (Creation remains exclusive to `loaf session start`.)

### Regression gates
- [ ] **Multi-session repro:** Repo with one active and four stopped sessions on the same branch (mirroring the current `main` state). Hook fires with `session_id` for the active one. New log entries land in the active session only.
- [ ] **Pre-spec misroute repro:** With branch routing as the sole mechanism (today's behavior), a `loaf session log --from-hook` for an active session_id but no chain logic writes to an arbitrary branch-matching session — fixture-pinned demonstration of the bug. Same fixture under post-spec routing writes correctly.
- [ ] **Skill self-logging unbroken:** `loaf session log "skill(test): ..."` from a non-hook context still appends to the active session on the branch and emits the warning, but does not error or exit non-zero.
- [ ] **No global state introduced:** No new file or env var written by any of the migrated subcommands. Parallel Claude Code sessions remain isolated.

### Stderr warning
- [ ] Warning text includes the literal string `WARN: no session_id signal` and names the branch.
- [ ] Warning fires on tier-3 fallback, regardless of which subcommand triggered it.
- [ ] Warning does NOT fire when tier 1 (`--session-id`) or tier 2 (`--from-hook` with valid stdin) succeeds.
- [ ] Warning is on stderr, not stdout — does not pollute machine-readable output.

### Helpers
- [ ] `parseHookSessionId()` returns `session_id` when given valid hook JSON on stdin.
- [ ] `parseHookSessionId()` returns `undefined` for malformed JSON, missing field, or empty stdin.
- [ ] `parseHookSessionId()` is only called when caller signals `--from-hook` is set.
- [ ] `resolveCurrentSession()` returns `null` only when ALL three tiers fail (no session matches anywhere).

## Priority Order

Single spec, four tasks with explicit dependencies:

1. **Task 1** — Extract `parseHookSessionId` and `resolveCurrentSession` helpers in `cli/lib/session/`. Unit tests for stdin parsing edge cases and chain ordering. Independent.
2. **Task 2** — Migrate `loaf session log` (line 1810) to use `resolveCurrentSession`. Add `--session-id` CLI flag. Wire stderr warning. Integration tests for tiers 1/2/3. Blocked by Task 1.
3. **Task 3** — Migrate `loaf session archive` (line 2015) and other bare-branch call sites (883, 2354). Add `--session-id` flag where applicable. Tests. Blocked by Task 1, parallelizable with Task 2.
4. **Task 4** — End-to-end smoke test reproducing the dev.30 misrouting scenario. Multi-session fixture, hook with session_id, assert correct routing AND assert stderr warning surface. Blocked by Tasks 2 and 3.

**Go/No-Go:**

- Tasks 2 and 3 can each ship as separate PRs (independent of each other), but both block on Task 1.
- Task 4 is a verification pass, not a merge gate.

**Bundling:** User intent is a single feature branch / single PR for SPEC-032 as a whole. The numbered tasks describe internal ordering, not separate PRs.

## Success Metric

After SPEC-032 ships:

- No commit message ever again carries the `#misrouted` flag.
- Every `loaf session log` call from a hook with `session_id` resolves to the conversation's actual session file, regardless of how many sessions exist on the branch.
- Every misrouted entry — caused by a caller that did not pass `--session-id` and was not a hook — is visible in the moment via the stderr warning, instead of silently corrupting the wrong session.
- The branch routing path remains functional but degraded-with-warning, paving the way for a future spec that removes it entirely once skill self-logging is migrated.
