---
id: SPEC-036
title: Worktree-aware .agents/ storage routing
source: direct
created: '2026-05-19T00:30:00Z'
status: complete
branch: feat/worktree-storage
adr: docs/decisions/ADR-013-agentic-state-storage-model.md
---

# SPEC-036: Worktree-aware `.agents/` storage routing

## Problem Statement

`findAgentsDir()` walks up from `process.cwd()` to the nearest `.agents/`. In a git worktree, that resolves to the worktree's own checkout — a separate tree from the main checkout. Three concrete fallouts, observed on this branch:

1. **Sessions misroute across worktrees.** A session created in worktree A is invisible to worktree B; `claude_session_id` Tier 1/2 routing falls through to branch routing or spawns a duplicate session.
2. **ID allocation clashes.** Two worktrees can each mint `TASK-166` in parallel, producing collisions at merge.
3. **Project knowledge fragments.** KB, ideas, drafts, reports — branch-scoped by accident, project-scoped by intent.

Underlying category error: `.agents/` is treated as branch-scoped content when it's really project/process state — agentic working notes that travel with the human across worktrees, not with the code under review.

## Strategic Alignment

- **Vision:** No `VISION.md` exists — flagging as a strategic gap (see Strategic Tensions). Working assumption: Loaf optimizes for AI-assisted development workflows where worktrees are a first-class human concurrency tool.
- **Personas:** Developers running parallel branches via `git worktree add`; AI agents that need stable session continuity regardless of which worktree the human invokes them from.
- **Architecture:** All agentic state moves out of the per-worktree filesystem and into a single project-scoped location (the main worktree's `.agents/`). Warrants an ADR (see ADR Companion).

## Solution Direction

`findAgentsDir()` becomes worktree-aware:

- In a git worktree, return `dirname(git rev-parse --git-common-dir)/.agents/` — the **main worktree's** `.agents/` directory
- In a single-checkout git repo, return the same path it returns today (parent-walk for `.agents/`)
- Outside a git context, preserve current behavior verbatim (parent-walk fallback)

Every call site picks up the new behavior automatically — no per-module refactor. A linked worktree no longer carries its own `.agents/` view; all reads and writes converge on the main worktree's directory.

This is **A3**: maximum centralization. Considered alternatives:

- **A1** (sessions only) — rejected: leaves ID clashes and knowledge fragmentation
- **A2** (sessions + ID allocator centralized, specs/tasks branch-local) — rejected: per-call-site refactor, dual-view scanning, and the storage-mode-per-artifact-kind dance buys only "specs/tasks visible in PR diffs," which the squash-merge workflow doesn't load-bear
- **Symlinks** — rejected: fragile across `git worktree add`, accidentally committed
- **Storage under `.git/loaf/`** — rejected: not a normal inspectable directory

PR review surface: under A3, specs/tasks no longer appear in feature-branch PR diffs. Reviewers reach them via the PR description + repo paths. Acceptable trade given that review attention belongs on the *code* changes; spec/task context is already linked from the PR.

## Scope

### In Scope
- Worktree probe via `git rev-parse --git-common-dir`
- `findAgentsDir()` redirects to the main worktree's `.agents/` when in a linked worktree
- One-shot migration command `loaf migrate worktree-storage`: move artifacts from a linked worktree's local `.agents/` to the main worktree's `.agents/` (dry-run default, back-pointer file)
- "Automatically compelled" migration nudge: detect pre-A3 state on every invocation; refuse loaf commands except `migrate` until the user runs it
- ADR documenting the agentic state storage model

### Out of Scope
- Changes to `TASKS.json`'s existence or shape (SPEC-035 owns that)
- Cross-machine sync
- Linear-native mode interactions (Linear-stored artifacts are already cross-tree)
- Updates to `.gitignore` for worktree-local `.agents/` (separate concern)
- Backwards-compatibility or silent fallback to pre-A3 layout

### Rabbit Holes
- Building a cross-worktree lock manager. Resist: append-only journaling + atomic rename is already safe.
- Auto-migrating on first run without consent. Resist: explicit command.
- Symlinking the worktree-local `.agents/` to main as a "transparent" alternative. Resist: brittle, see No-Gos.
- Renaming `findAgentsDir`. Resist: many call sites; surface stability is a feature.

### No-Gos
- **Symlinks.** Fragile across `git worktree add`, get accidentally committed, confuse newcomers.
- **Storing agentic state under `.git/`.** Use the main worktree's `.agents/` so it remains a normal, inspectable directory.
- **Silent fallback** to pre-A3 layout. Migration is a hard cut; un-migrated worktrees refuse commands.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Concurrent journal writes corrupt a session file | Low | Med | Existing atomic-rename pattern; add a cross-worktree contention test |
| Migration moves files the user didn't expect | Med | High | Dry-run by default + explicit confirmation + back-pointer file in worktree-local location |
| Hooks invoked outside a git context resolve unexpectedly | Low | Med | Resolver falls back to current parent-walk behavior transparently |
| Reviewers miss spec/task context now that they're not in the diff | Med | Low | PR template can include "Implements: SPEC-NNN — see .agents/specs/SPEC-NNN" line |
| Migration nudge becomes noisy and gets ignored | Med | Med | Refuse all non-migrate commands; nudge text points at exactly one command |

## Open Questions

- [ ] Migration back-pointer format: sentinel file in the worktree-local `.agents/` (e.g., `.moved-to` containing the absolute path). Confirm format during implementation.
- [ ] Should `loaf migrate worktree-storage` be runnable from the main checkout itself (no-op exit) or refused outside a worktree? Decide and document.
- [ ] Treatment of an emptied worktree-local `.agents/` post-migration: leave the back-pointer + empty dirs, or remove entirely? Default: leave back-pointer, let user clean up.

## Test Conditions

- [ ] In a fresh worktree on a different branch, `loaf session start` finds and resumes the active session from the main worktree (not a new one)
- [ ] Journal entries appended from any worktree reach the same session file
- [ ] `loaf task new` invoked concurrently from two worktrees allocates distinct IDs
- [ ] `loaf spec new` from a worktree allocates a SPEC ID that doesn't collide with the main worktree's draft specs
- [ ] Un-migrated worktrees refuse loaf commands (except `migrate`) with a clear prompt
- [ ] After `loaf migrate worktree-storage`, all artifacts live under the main worktree's `.agents/` and any worktree sees them transparently
- [ ] `loaf session log` outside a git repo continues to work using the original parent-walk resolution
- [ ] Single-checkout repos (no linked worktrees) see no behavior change
- [ ] Removing a worktree (`git worktree remove`) doesn't affect the main worktree's `.agents/` content
- [ ] Migration is dry-run by default and requires explicit confirmation to mutate

## Priority Order

Tracks ship in this order. Each go/no-go gate must pass before the next track starts.

1. **Track A — Foundation.** Worktree probe; `findAgentsDir` redirects to main worktree when in a linked worktree; non-git fallback preserved. Go/no-go: single-resolver tests pass; cross-worktree session continuity test passes (since every call site now resolves the same place automatically); parallel ID allocation test passes.
2. **Track B — Migration + nudge.** `loaf migrate worktree-storage` (dry-run default, back-pointer) and pre-A3 detection wired into the CLI's top-level command dispatcher (single check, refuses every non-migrate command). Go/no-go: round-trip migration test passes; refusal nudge fires correctly in un-migrated state and `migrate` still works.
3. **Track C — ADR.** Documents the decision with alternatives considered. *Can be dropped if scope tightens — manual ADR write-up is acceptable for early adopters.*

## Strategic Tensions

1. **Missing strategic docs.** No `VISION.md`, `STRATEGY.md`, or `ARCHITECTURE.md` to evaluate against. This spec is a candidate to seed `ARCHITECTURE.md` with an explicit "agentic state storage model" section via `/loaf:reflect` after shipping.
2. **Convention retirement.** The "Spec on main, tasks+code on branch" convention becomes obsolete under A3 — all `.agents/` content always lives in the main worktree regardless of which branch's PR is in flight. Update `AGENTS.md` and project memory after the ADR lands.
3. **PR review surface change.** Sessions/specs/tasks no longer appear in PR diffs at all. Documented as a trade — review attention belongs on the *code* changes; everything else is context reachable via the PR description.

## ADR Companion

Landed as [**ADR-013: Agentic State Is Project-Scoped, Not Branch-Scoped**](../../../docs/decisions/ADR-013-agentic-state-storage-model.md) on 2026-05-19. The ADR captures the decision, the four rejected alternatives (A1, A2, symlinks, `.git/loaf/`), and the consequences.

## Provenance

Originated from a conversation on 2026-05-19 diagnosing session log misrouting in worktrees. Initial shape landed as **A2** (sessions/process-state shared, specs/tasks/plans branch-local) with 7 tasks broken down. During pre-implementation review, the user surfaced that A2's per-artifact-kind split bought only the "specs/tasks visible in PR diffs" property, which the squash-merge workflow doesn't actually need. Pivoted to **A3** (radical centralization) with 3 tasks. The A2 framing and four dropped task files are preserved in git history (commit 92b1b046).
