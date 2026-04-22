---
id: ADR-011
title: Linear-Native Mode — Deliberation vs Execution Split
status: Accepted
date: 2026-04-22
---

# ADR-011: Linear-Native Mode — Deliberation vs Execution Split

## Decision

When `integrations.linear.enabled` is `true` in `.agents/loaf.json`, Loaf's spec/task orchestration skills split artifacts across two layers:

- **Deliberation layer (local, git-tracked):** specs in `.agents/specs/`, ADRs in `docs/decisions/`, councils in `.agents/councils/`, strategic docs in `docs/`. Canonical.
- **Execution layer (Linear):** tasks as sub-issues under a `spec`-labeled parent rollup issue. No local `TASK-NNN.md` files are created for new work.

The parent Linear issue is a **canonical-elsewhere rollup** — summary + link to the local spec file, not a re-host. Per-task dependencies use Linear's `blockedBy` field, which `/implement` enforces as a hard pre-flight gate.

When `integrations.linear.enabled` is `false`, the existing local-tasks flow is preserved unchanged.

Target → layer mapping:

| Artifact | Location | Rationale |
|---|---|---|
| Specs | `.agents/specs/` (always) | Deliberation: needs git history + code-adjacent visibility |
| ADRs | `docs/decisions/` (always) | Deliberation: immutable decision record |
| Councils | `.agents/councils/` (always) | Deliberation: multi-agent reasoning trace |
| Tasks (local mode) | `.agents/tasks/` + `TASKS.json` | Simple, offline, solo-friendly |
| Tasks (Linear mode) | Linear sub-issues of `spec`-labeled parent | Real-time state, dashboards, blocking graph |

## Context

Pre-ADR-011, skills referenced Linear MCP tools directly with no explicit mode boundary: ~80 references across 12+ skill files. Whether and how to use Linear was implicit. Some skills assumed Linear; some assumed local files; the result was inconsistent behavior when `integrations.linear.enabled` was toggled.

STRATEGY.md's Priority 5 (Backend abstraction) proposed a pluggable-backend CLI layer as the fix, but shaping that spec revealed that skills also needed to change — just adding a CLI abstraction below hard-coded tool names would still leave skills assuming one backend.

Meanwhile, a real-project shipping experience (GridSight Data Service, SPEC-002) crystallized the conceptual split: specs and tasks serve different purposes and want different storage. Specs encode deliberation — problem, solution direction, scope, rabbit holes, strategic tensions — and evolve alongside code in PRs. Tasks encode execution — status, blockers, assignees, comments, dashboards — and want real-time tracker features.

ADR-010 established the consolidation pattern at the overlay-file layer (`.agents/AGENTS.md` canonical + per-harness symlinks). ADR-011 applies the same canonical-elsewhere principle to the spec/task artifact model.

## Rationale

- **Deliberation artifacts belong with code.** Git history, code-adjacent review, survive the tracker being down or switched, travel with the branch. Specs, ADRs, councils fit here.
- **Execution artifacts belong in the tracker.** Real-time state queries, blocking graphs, assignees, notifications, dashboards. Tasks fit here.
- **Canonical-elsewhere over duplication.** Same principle as ADR-010: one source of truth per artifact type; the other surface is a pointer. Prevents drift.
- **Mode-aware skills over skill rewrites.** Skills branch on one config flag. Same skill content, different backend. Sets up SPEC-023's narrower backend abstraction as the next step.
- **BlockedBy as a hard pre-flight gate.** Linear's dependency graph becomes a runtime invariant. The orchestrator cannot implement through open blockers even by accident. Local-tasks mode keeps dependencies advisory (no reliable query mechanism exists without Linear).

## Alternatives Considered

- **Full backend abstraction first (SPEC-023 as originally scoped).** Rejected as premature: without the mode-awareness precedent, the abstraction would have to infer the deliberation/execution split or leak it into every caller. Better to prove the split in skills first, then extract.
- **Copy the spec text into the Linear parent issue.** Rejected: duplication drifts. The parent is an entry point, not a mirror. Summary + link.
- **Local tasks AND Linear issues (both, synchronized).** Rejected: sync burden, source-of-truth ambiguity, no meaningful use case for solo or team workflow.
- **Drop local-tasks mode entirely, require Linear.** Rejected: Loaf works for solo developers offline; hard dependency on Linear disqualifies that persona.
- **Skip the `spec` label convention, use issue type alone.** Rejected: Linear's issue-type system isn't extensible per-team; a label is cheap, grep-able across projects, and doesn't fight Linear's native workflow.

## Consequences

- `/breakdown`, `/implement`, `/housekeeping`, `/shape`, `/council` are now mode-aware. Same skill content, different backend.
- Spec frontmatter gains optional `linear_parent` and `linear_parent_url` fields, populated by `/breakdown` on first run.
- `/implement`'s Linear-native routing enforces `blockedBy` as a pre-flight gate and auto-closes the parent when the last sub-issue is `completed`.
- `/housekeeping` adds mode-aware reconciliation (warnings, not auto-fixes): spec ↔ Linear parent status matching, orphaned `linear_parent` references, pre-Linear local task detection.
- `/council` files live locally even in Linear-native mode; `linear_parent` added to frontmatter when present so Linear readers can trace back.
- `orchestration/references/linear.md` gains `spec` label convention, parent-vs-child structure, `mcp_server_name` config, and project-scoped vs user-scoped `.mcp.json` guidance for multi-workspace setups.
- SPEC-023 (backend abstraction) narrows: most of the work becomes extracting Linear-specific calls into a `tracker` CLI subcommand and adding a second backend (GitHub Issues would be next), not rewriting skills.
- No migration shim. Projects that toggle Linear on mid-flight keep existing local tasks; only new breakdowns go Linear-native. Migration is user-initiated.

## Shipped

- PR #34 merged 2026-04-22 as `v2.0.0-dev.29`.
- 6 skill updates across `breakdown`, `implement`, `housekeeping`, `shape`, `council`, plus the `orchestration/references/linear.md` reference doc.
