---
id: SPEC-042
title: Worktree-aware config resolution and ephemeral-branch session fallback
source: direct — bug reports from GridSight release-agent flows (v0.16.0)
created: '2026-05-28T00:18:00Z'
status: implementing
branch: feat/worktree-aware-config-and-session-fallback
related_specs:
  - SPEC-032
  - SPEC-036
---

# SPEC-042: Worktree-aware config resolution and ephemeral-branch session fallback

## Problem Statement

Two infrastructure bugs surfaced by the same release-agent flow on multi-worktree projects (e.g., GridSight v0.16.0 release, PR #237). Both bite every future release run on a centralized-storage repo.

**Bug A — config resolution ignores `.moved-to`.** Under SPEC-036's centralization, a linked worktree's `.agents/` contains only a `.moved-to` back-pointer to the main worktree's `.agents/`. `findAgentsDir` follows it correctly (sessions, tasks, IDs all work), but `cli/lib/config/agents-config.ts` reads `loaf.json` directly via `join(projectRoot, ".agents", "loaf.json")` — bypassing the back-pointer. Result: `loaf release --pre-merge` in a centralized worktree fails with "No version files found" because `release.versionFiles` from `loaf.json` is unreadable. The same call path is used by post-merge, release-only-pr, kb-glossary, and MCP detection/recommendations. The write path (`writeLoafConfigRaw`, `mergeLoafConfigIntegrations`) has the symmetric latent bug: it would create a stray `loaf.json` next to the `.moved-to` pointer.

**Bug B — session log can't resolve on ephemeral branches.** Under SPEC-032's 3-tier chain, a release agent spawned in a worktree on `release/v0.16.0` (parent: orchestrator's `cwt/*` branch) can't log decisions. Tier 1 (no flag) and Tier 2 (no `claude_session_id` signal across sub-agent spawns) miss; Tier 3 (branch routing) finds no session matching `release/v0.16.0`. The existing "exactly one active session → adopt" heuristic (`cli/lib/session/find.ts:264`) fails when multiple active sessions exist, and even when it fires it does the wrong thing: it **mutates the adopted session's `branch:` frontmatter** to the calling branch, erasing the session's origin.

Both bugs share a root: code paths that *predate* SPEC-036/SPEC-032 still assume single-worktree, branch-as-identity semantics.

## Strategic Alignment

- **Vision:** Reinforces "Session Continuity" — work survives across worktrees and sub-agent spawns.
- **Personas:** Solo developer running parallel worktrees; team lead running release agents from an orchestrator session. Both currently hit visible failures in the release flow.
- **Architecture:** Treats `.agents/` as project-scoped (SPEC-036) and `claude_session_id` as session identity, branch as session property (SPEC-032). Both invariants are already documented in STRATEGY.md; this spec closes the remaining call-site gaps.
- **Tension note:** STRATEGY.md explicitly anticipates removing the branch-fallback tier in a future spec once skills pass `--session-id` per-process. Track B should *correct* the fallback without entrenching it — minimal change, no new config surface, no per-prefix gating.

## Solution Direction

### Track A — Worktree-aware config resolution

Make `agents-config.ts` route reads and writes through `findMainWorktreeRoot` (already used by `findAgentsDir`). Single helper that:

- In a single-checkout repo: returns `projectRoot/.agents/loaf.json` (current behavior).
- In a linked worktree: returns the main worktree's `.agents/loaf.json` (via `findMainWorktreeRoot`).
- Falls through to current behavior if no worktree probe succeeds.

Both `readLoafConfig` and `writeLoafConfigRaw` use this helper. All callers (release pre-merge, post-merge, release-only-pr, kb-glossary, MCP detection, MCP recommendations, integration merges) are healed in one change — no per-caller refactor.

### Track B — Ephemeral-branch session fallback

Two surgical changes to `cli/lib/session/find.ts` / `resolve.ts`:

1. **Stop destructive branch-rewrite on adoption.** `findActiveSessionForBranch` currently rewrites the adopted session's `branch:` field to the calling branch (`find.ts:280`) and the spec-linked rename path does the same (`find.ts:229-242`). The session's `branch:` represents its origin and is referenced by `findActiveSessionForBranch` itself — overwriting it on every adoption breaks Tier 3 for the *next* call. Stop mutating. The log entry implicitly records its calling context via the entry text and timestamp; the session frontmatter stays put.
2. **Extend adoption to most-recent active.** When branch routing finds no match, fall back to the most-recent active session by `last_updated`/`last_entry`, regardless of count. Emit a stronger stderr WARN naming the target file and its branch: `WARN: no session for branch '<branch>'; logging to most-recent active session '<file>' (origin branch '<other-branch>'). Pass --session-id <id> to silence.`

No prefix matching, no Conventional Branch prefix list. The fallback treats all branches uniformly; recency does the disambiguation. This avoids hard-coding patterns that would need maintenance and keeps the branch-fallback tier minimal per STRATEGY's eventual-removal note.

## Scope

### In Scope

- **Track A:**
  - New internal helper in `agents-config.ts` resolving the effective `loaf.json` path via `findMainWorktreeRoot` (reuse, don't reimplement).
  - `loafConfigPath`, `readLoafConfig`, `writeLoafConfigRaw`, `mergeLoafConfigIntegrations` all route through it.
  - Unit tests against tmp fixtures: read follows `.moved-to`; write lands in centralized location; single-checkout repo unchanged.
  - Integration test mimicking the GridSight repro: `loaf release --pre-merge --base <tag>` in a migrated linked worktree finds `versionFiles` from centralized config.

- **Track B:**
  - Remove the two `writeFileSync` calls in `find.ts` that mutate adopted sessions' `branch:` frontmatter.
  - Replace `allSessions.length === 1` gating with "most-recent active, regardless of count" in the no-branch-match fallback.
  - Update WARN text to name the chosen session file and its origin branch.
  - Tests: branch frontmatter is never mutated by `findActiveSessionForBranch`; multi-active fallback returns most-recent; `loaf session log` from `release/v0.16.0` lands in orchestrator's session and leaves its frontmatter intact.

### Out of Scope

- Changes to Tier 1 (`--session-id`) or Tier 2 (`--from-hook`) — they're the documented correct paths.
- Auto-starting sessions on `release/*` or any branch pattern. Considered and rejected: adds session lifecycle to `loaf release`, and the orchestrator's session is the right home for release decisions anyway.
- Configurable ephemeral-branch patterns or per-prefix gating. Considered and rejected: the destructive-rewrite is the real bug; once removed, prefix gating buys nothing.
- Changes to `loaf release` command code beyond what falls out of fixing `agents-config.ts`.
- `tasksDir` and other `.agents/`-relative paths in release — already use `findAgentsDir` (which is SPEC-036-aware) per spot check; flag as follow-up if any case is found broken.
- Cross-worktree session locking, conflict resolution, or session sync.
- Refactor of skill self-logging to pass `--session-id` per-process. STRATEGY notes this as the *eventual* path; out of scope here.

### Rabbit Holes

- Conventional Branch prefix taxonomy. Tempting but counter-productive: ephemeral-vs-permanent is not the discriminator; the destructive rewrite is.
- Per-caller refactors to pass `mainWorktreeRoot` explicitly into `readLoafConfig`. Resist: fix at the helper level, callers stay unchanged.
- Auto-migrating `loaf.json` on first access. Resist: SPEC-036 migration is explicit by design.
- Adding a "session ownership" header to log entries. Resist: WARN to stderr is sufficient; the journal is append-only and the entry text + timestamp already encode context.

### No-Gos

- **Hard-coding ephemeral branch prefixes** (`release/*`, `cwt/*`, etc.). Per user direction and Conventional Branch awareness, all branches are equal.
- **Auto-starting sessions** from `loaf release` or any branch-checkout event.
- **Silent fallback in agents-config** that creates a stray `loaf.json` next to a `.moved-to` pointer. Writes must follow the back-pointer.
- **Mutating session frontmatter** as a side effect of read/log operations. Sessions are written by their owner; consumers only append entries.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Removing branch-rewrite breaks "started on main, branched off" flows | Med | Med | Test asserts adoption returns the session without mutating; current adoption flow still works for the linear case because find-by-branch still finds the session on subsequent calls via the most-recent-active fallback |
| `findMainWorktreeRoot` probe fails on non-git directories (CI scripts, tmp dirs) | Low | Low | Helper falls through to current path on probe-null, preserving today's behavior |
| Most-recent-active fallback routes a release log into a *stale* orchestrator session | Low | Med | WARN names the target file + origin branch; user can see and redirect with `--session-id` |
| Track A integration test relies on `loaf migrate worktree-storage` fixture setup | Med | Low | Use the existing test harness pattern in `worktree-storage.test.ts` |

## Open Questions

- [ ] Should the WARN text vary between "no active sessions at all" (no fallback target) and "fell back to most-recent"? Probably yes — silent null return is bad UX.
- [ ] When the most-recent-active session is `status: stopped` vs `active`, does the fallback prefer active? (Current code already filters to `status === "active"` — keep that.)
- [ ] Do we want a CHANGELOG entry per track or one entry covering both?

## Test Conditions

**Track A:**
- [ ] `readLoafConfig` from a worktree containing only `.moved-to` returns the main worktree's config (unit, tmp fixture).
- [ ] `writeLoafConfigRaw` / `mergeLoafConfigIntegrations` from a centralized worktree write to the main worktree's `loaf.json`, leaving the linked worktree's `.agents/` containing only `.moved-to` (unit, tmp fixture).
- [ ] Single-checkout repo behavior unchanged: existing `agents-config.test.ts` cases pass without modification.
- [ ] `loaf release --pre-merge --base <prev-tag>` in a migrated linked worktree completes the pre-merge phase without `--version-file` override (integration; replicates GridSight repro).

**Track B:**
- [ ] `findActiveSessionForBranch` does not mutate any session's `branch:` frontmatter under any code path (unit, snapshot frontmatter before/after).
- [ ] When called from a branch with no matching session and >1 active sessions, returns the most-recent by `last_updated`, leaves frontmatter intact, and emits the WARN (unit).
- [ ] `loaf session log "decision(release): ..."` invoked on `release/v0.16.0` with an active orchestrator session on `cwt/foo` appends to the orchestrator's session file with the WARN visible on stderr (integration).
- [ ] `loaf session log` with `--session-id` passed continues to bypass Tier 3 entirely (regression; existing test suffices).

## Priority Order

Two PRs off branch `feat/worktree-aware-config-and-session-fallback`, sequential.

1. **Track A — worktree-aware config resolution.** Pure bug fix, no behavior questions. Go/no-go: Track A test conditions pass and the GridSight repro succeeds in CI fixture before starting Track B.
2. **Track B — ephemeral-branch session fallback.** Behavior change to session adoption. Depends only on Track A having merged so the test harness can rely on centralized config. Go/no-go: Track B test conditions pass, including the regression test that `--session-id` still bypasses Tier 3.
