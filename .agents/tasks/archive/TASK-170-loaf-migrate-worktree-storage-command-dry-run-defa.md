---
id: TASK-170
title: >-
  loaf migrate worktree-storage command (dry-run default, back-pointer file,
  round-trip test)
spec: SPEC-036
status: done
priority: P2
created: '2026-05-18T23:58:56.163Z'
updated: '2026-05-19T09:48:46.260Z'
depends_on:
  - TASK-166
session: .agents/sessions/20260519-090118-session.md
completed_at: '2026-05-19T09:48:46.260Z'
---

# TASK-170: loaf migrate worktree-storage command (dry-run default, back-pointer file, round-trip test)

## Description

Implement `loaf migrate worktree-storage` **and** the pre-A3 refusal nudge in one task. Under A3 they're tightly coupled and trivially small — splitting would be ceremony.

**Migration command:**
- Dry-run by default; mutation requires explicit `--apply` (or equivalent confirmation flag)
- In a linked worktree: move every subdirectory and file under the worktree-local `.agents/` to the main worktree's `.agents/`, preserving structure (sessions to sessions/, ideas to ideas/, etc.)
- Conflict policy: if a file exists in both locations, prefer the most recently modified version and log the loser; provide `--force-from-worktree` and `--force-from-main` overrides for explicit user choice
- Write a back-pointer file (e.g., `.agents/.moved-to`) in the worktree-local location containing the main worktree's absolute path
- Idempotent: re-running on an already-migrated worktree exits with "nothing to do"
- Run from the main checkout: no-op exit ("nothing to migrate") — or refuse with a clear message; decide and document

**Refusal nudge:**
- Single check at the CLI's top-level command dispatcher (before any subcommand runs)
- Pre-A3 detection signal: in a linked worktree where the worktree-local `.agents/` contains content AND the back-pointer file is absent (or its target is missing)
- When pre-A3 detected: refuse every loaf command except `loaf migrate worktree-storage` with a clear message pointing to that command
- Single-checkout repos and the main worktree itself never trigger the refusal

**Observability (from TASK-166 review, Q3):**
- The `findMainWorktreeRoot` probe in `cli/lib/tasks/resolve.ts` silently swallows all git failures (no git installed, corrupted repo, "not a git repository", etc.) and falls through to parent-walk. This is correct on the hot path but masks diagnostics when the refusal nudge misfires or fails to fire.
- Add a debug knob (env var `LOAF_DEBUG_RESOLVE=1`) that, when set, writes the git invocation's stderr to `process.stderr` instead of suppressing it. Wire this through `findMainWorktreeRoot` (small change to its `try/catch`) and surface it in the migrate command's help/docs as the recommended way to diagnose "why did the nudge fire?" / "why didn't the nudge fire?"
- Single source of truth: the env var name lives in one constant referenced by both `resolve.ts` and the migrate command.

## Acceptance Criteria

- [ ] `loaf migrate worktree-storage` exists as a subcommand
- [ ] Default behavior is dry-run; output lists what would move without mutating
- [ ] `--apply` (or equivalent) performs the actual move with confirmation
- [ ] Conflict policy implemented and documented; `--force-from-worktree` / `--force-from-main` overrides work
- [ ] Back-pointer file written after successful migration
- [ ] Idempotent: re-run on migrated worktree is a no-op
- [ ] Round-trip test: prepare a fake pre-A3 layout → dry-run (no changes) → apply (changes correct) → re-apply (no-op)
- [ ] Refusal nudge: pre-A3 worktree refuses every loaf command except `migrate` with consistent messaging
- [ ] Refusal nudge: single-checkout repos and the main worktree are never affected
- [ ] Refusal message is centralized (single source of truth)

## Files

- New: `cli/commands/migrate.ts`
- New: `cli/lib/migrate/worktree-storage.ts`
- Wiring: `cli/index.ts` (top-level dispatcher) for the refusal check
- New tests alongside

## Verification

```bash
npm run test -- migrate
npm run test -- pre-a3-detection
loaf migrate worktree-storage --help
```

## Context

See SPEC-036. Track B. Depends on TASK-166 for the resolver behavior that migration produces post-state for.
