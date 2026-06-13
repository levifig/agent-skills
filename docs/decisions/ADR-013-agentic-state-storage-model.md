---
id: ADR-013
title: Agentic state is project-scoped, not branch-scoped
status: Accepted
date: 2026-05-19
---

# ADR-013: Agentic State Is Project-Scoped, Not Branch-Scoped

## Decision

`.agents/` is project-scoped state. It always resolves to the **main worktree's** `.agents/` directory, regardless of which linked worktree a `loaf` command is invoked from. No artifact kind is exempt — sessions, specs, tasks, plans, councils, ADRs, knowledge, and ideas all live in one place.

The native project resolver (`internal/project/project.go`) probes via `git rev-parse --git-dir` vs `--git-common-dir`. When they differ, the caller is in a linked worktree, and Loaf resolves `.agents/` through the main worktree. In the main checkout, in submodules, and outside any git context, the original parent-walk behavior is preserved.

Migration is a hard cut. `loaf migrate worktree-storage` moves a linked worktree's local `.agents/` into the main worktree's `.agents/`. Until migration completes, a top-level refusal nudge gates every `loaf` invocation except `migrate`, `help`, and `--version`, exiting with code `2`. There is no silent fallback to the pre-A3 layout.

## Context

`.agents/` was originally treated as branch-scoped content — the same trade Loaf makes for source code itself. `findAgentsDir()` walked up from `process.cwd()` to the nearest `.agents/`. In a `git worktree add`-style checkout, that resolved to the linked worktree's own copy — a separate tree from the main one.

Three concrete fallouts, all observed in real use on the `feat/worktree-storage` branch itself:

1. **Session misrouting.** A session created in worktree A was invisible to worktree B. `claude_session_id` Tier 1/2 routing fell through to branch-based routing or spawned a duplicate session.
2. **ID allocation clashes.** Two worktrees could each mint `TASK-166` in parallel, producing collisions at merge.
3. **Knowledge fragmentation.** KB, ideas, drafts, councils, reports — branch-scoped by accident, project-scoped by intent.

The underlying error was categorical. `.agents/` is not branch content; it's project/process state — agentic working notes that travel with the human across worktrees, not with the code under review. Once you frame it that way, the worktree-aware resolver is the obvious shape.

## Alternatives Considered

- **A1: sessions-only routing.** Make only `.agents/sessions/` resolve to the main worktree; leave specs/tasks/plans/KB branch-local. Rejected: addresses misrouting but leaves the ID-clash and knowledge-fragmentation fallouts. Partial fix to a categorical bug.

- **A2: per-artifact-kind split.** Sessions, KB, and ID allocators centralized; specs, tasks, and plans branch-local. Rejected: requires per-call-site refactor and dual-view scanning. The deeper property A2 would buy is that mutations to specs, tasks, and plans would flow through the same git-native gates as code — PR review, conflict resolution via three-way merge, and atomic landing on main. Under A3, those mutations happen out-of-band: they land directly in the main worktree's `.agents/` without traversing a PR, and reviewers won't see them as diff entries. The tradeoff is acknowledged and accepted (see *Consequences → Negative*); under the squash-merge workflow, reviewers reach spec/task context via the PR description and canonical archive paths rather than the diff, and the complexity cost of A2 wasn't being repaid by review value that was actually getting collected.

- **Symlinks.** Symlink each linked worktree's `.agents/` into the main one. Rejected: fragile across `git worktree add` (Git doesn't preserve the link on new worktree creation), gets accidentally committed, confuses contributors, and the failure mode is silent divergence.

- **Storage under `.git/loaf/`.** Hide agentic state inside Git's internal directory tree. Rejected: not an inspectable directory by convention. Users open `.agents/` with their file browser; `.git/` is off-limits territory. Breaks the principle that agentic state should be readable, editable, and grep-able.

## Consequences

### Positive

- **Cross-worktree session continuity.** Sessions, journal entries, and `claude_session_id` routing all converge on one store regardless of which worktree is active.
- **Collision-free IDs.** Parallel `loaf task new` calls from different worktrees against one shared `TASKS.json` allocate distinct IDs.
- **Single shared knowledge view.** KB, ideas, councils, drafts, and reports are reachable from any worktree without sync.
- **No per-call-site refactor.** Every existing caller of `findAgentsDir()` picks up the new behavior transparently.
- **Single resolver.** One probe, one path, one mental model. The "where does this artifact live?" question has one answer.

### Negative

- **Sessions, specs, and tasks no longer appear in PR diffs.** Reviewers see only code changes. Spec and task context is reachable via the PR description and repo paths. Acceptable trade: review attention belongs on code; context is one link away.
- **Mutations to agentic state bypass git-native review and merge gates.** Changes to specs, tasks, plans, and journal entries land directly in the main worktree's `.agents/` without flowing through PR review or three-way merge resolution. Reviewers encounter them via the canonical archive paths and the PR description, not as diff entries on the feature branch. Concurrent edits to the same `.agents/` content from different worktrees are coordinated at the filesystem layer (lock files + atomic writes), not by Git. Accepted as the cost of treating agentic state as project-scoped working memory rather than branch content.
- **Pre-existing worktrees require migration.** Anyone using `git worktree add` against a Loaf project on a version that predates this ADR must run `loaf migrate worktree-storage` before their next `loaf` command works.
- **The "spec on main, tasks+code on branch" convention is retired.** It was a workaround for the bug this ADR fixes. See *Follow-on*.

## Compliance

The migration command and refusal nudge represent a hard cut. Pre-A3 layouts (a linked worktree with a populated local `.agents/` and no valid `.moved-to` back-pointer) are **refused** with exit code `2`, not silently fallback'd. The contract is documented and enforced in:

- `internal/cli/worktree_storage_migration.go` — migration logic, dry-run default, conflict policy, back-pointer protocol, pre-A3 detection, and command allow-list (`migrate`, `help`, `--version`)
- `internal/project/project.go` — worktree probe and main-worktree redirect

Observability for the resolver is opt-in via `LOAF_DEBUG_RESOLVE=1|true|yes|on` (case-insensitive truthy values), which surfaces otherwise-suppressed stderr from the git probe.

## Follow-on

- The **"spec on main, tasks+code on branch"** convention is retired by this ADR. It was a workaround for the worktree-fragmentation bug A3 fixes; under A3 it is a no-op (all `.agents/` content always lives in the main worktree's directory regardless of which branch's PR is in flight). Update private memory and any PR templates accordingly.
- Reviewers reach spec and task context via the PR description. If PR templates exist in this project, they should include an `Implements: SPEC-NNN — see .agents/specs/SPEC-NNN` line so reviewers have a one-click path from the PR to the canonical artifact.
- An open follow-up captures the symlink-handling design call deferred from TASK-170: see [.agents/ideas/20260519-093959-migrate-symlink-handling.md](../../.agents/ideas/20260519-093959-migrate-symlink-handling.md). Do not extend symlink-touching logic in `internal/cli/worktree_storage_migration.go` without resolving that idea first.

## Related

- [SPEC-036](../../.agents/specs/SPEC-036-worktree-aware-agents-storage-routing.md) — Worktree-aware `.agents/` storage routing
- TASK-166 (commit `8665fc3e`) — `findAgentsDir` worktree probe
- TASK-170 (commit `91eebe18`) — `loaf migrate worktree-storage` + pre-A3 refusal nudge
