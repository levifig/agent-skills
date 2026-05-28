---
title: "SPEC-020 Hook Migration Inventory"
type: reference
created: 2026-03-31T13:00:00Z
status: active
tags: [hooks, migration, spec-020]
related: [specs/SPEC-020-target-convergence-amp.md]
---

# SPEC-020 Hook Migration Inventory

Complete before/after accounting of all hooks affected by SPEC-020.

## Hooks Removed (→ skill instructions): 24

### Pre-tool (19)

| Hook | Was | Becomes |
|------|-----|---------|
| `format-check` | Non-blocking, Edit\|Write | `foundations` skill: "Run formatters before committing" |
| `tdd-advisory` | Non-blocking, Edit\|Write | `foundations` skill: "Write tests alongside implementation" |
| `validate-changelog` | Non-blocking, Edit\|Write | `documentation-standards` skill |
| `python-type-check` | Non-blocking, Edit\|Write | `python-development` skill |
| `python-type-check-progressive` | Non-blocking, Edit\|Write | `python-development` skill |
| `python-ruff-lint` | Non-blocking, Edit\|Write | `python-development` skill |
| `python-pytest-validation` | Non-blocking, Edit\|Write | `python-development` skill |
| `python-pytest-execution` | Non-blocking, Edit\|Write | `python-development` skill |
| `python-bandit-scan` | Non-blocking, Edit\|Write | `python-development` skill |
| `typescript-tsc-check` | Non-blocking, Edit\|Write | `typescript-development` skill |
| `typescript-bundle-analysis` | Non-blocking, Edit\|Write | `typescript-development` skill |
| `rails-migration-safety` | Non-blocking, Edit\|Write | `ruby-development` skill |
| `rails-migration-safety-deep` | Non-blocking, Edit\|Write | `ruby-development` skill |
| `rails-test-execution` | Non-blocking, Edit\|Write | `ruby-development` skill |
| `rails-brakeman-scan` | Non-blocking, Edit\|Write | `ruby-development` skill |
| `infra-validate-k8s` | Non-blocking, Edit\|Write | `infrastructure-management` skill |
| `infra-dockerfile-lint` | Non-blocking, Edit\|Write | `infrastructure-management` skill |
| `infra-k8s-dry-run` | Non-blocking, Edit\|Write | `infrastructure-management` skill |
| `infra-terraform-plan` | Non-blocking, Edit\|Write | `infrastructure-management` skill |

### Post-tool (7)

| Hook | Was | Becomes |
|------|-----|---------|
| `python-ruff-check` | Edit\|Write | `python-development` skill |
| `typescript-eslint-check` | Edit\|Write | `typescript-development` skill |
| `rails-rubocop-check` | Edit\|Write | `ruby-development` skill |
| `design-a11y-validation` | Edit\|Write | `interface-design` skill |
| `design-token-check` | Edit\|Write | `interface-design` skill |
| `design-a11y-audit` | Edit\|Write | `interface-design` skill |
| `changelog-reminder` | Bash (post git commit) | `documentation-standards` skill |

## Hooks Absorbed (→ `loaf session`): 6

| Hook | Was | Becomes |
|------|-----|---------|
| `session-start-soul` | SessionStart | `loaf session start` (SOUL.md validation) |
| `session-start` | SessionStart | `loaf session start` (context output) |
| `kb-session-start` | SessionStart | `loaf session start` (stale knowledge count) |
| `session-end` | SessionEnd | `loaf session end` (completion checklist) |
| `kb-session-end` | SessionEnd | `loaf session end` (knowledge consolidation) |
| `pre-compact-archive` | PreCompact | `loaf session log` (compact journal entry) |

## Hooks Staying: 11

| Hook | Type | Why it stays |
|------|------|-------------|
| `check-secrets` | Pre-tool, **blocking** | Can't trust agent — enforcement |
| `security-audit` | Pre-tool, non-blocking | Dangerous Bash pattern detection |
| `validate-push` | Pre-tool, **blocking** | Git workflow gate |
| `workflow-pre-pr` | Pre-tool, **blocking** | PR format enforcement |
| `workflow-pre-merge` | Pre-tool, **prompt** | Squash merge reminder (text injection) |
| `workflow-pre-push` | Pre-tool, advisory | Push reminder |
| `validate-commit` | Pre-tool, non-blocking | Conventional commits validation |
| `detect-linear-magic` | Pre-tool, non-blocking | Side-effect (Linear status updates) |
| `generate-task-board` | Post-tool | Side-effect (regenerate TASKS.md) |
| `kb-staleness-nudge` | Post-tool | CLI-wrapping (calls `loaf kb check`) |
| `workflow-post-merge` | Post-tool | Advisory post-merge checklist |

## Hooks Added (new, journal model): ~6

| Hook | Type | Purpose |
|------|------|---------|
| PostToolUse `Bash(git commit)` | Auto-log + nudge | `commit(SHA): "message"` + decision prompt |
| PostToolUse `Bash(gh pr create)` | Auto-log | `pr(#N): created "title"` |
| PostToolUse `Bash(gh pr merge)` | Auto-log | `merge(#N): squash, SHA→SHA` |
| Stop | Nudge | "You did work but didn't journal why" → points to `orchestration` skill |
| PreCompact | Nudge | "Flush decisions before compaction" → points to `orchestration` skill |
| SessionStart/End | Journal lifecycle | `resume`/`pause` entries via `loaf session start/end` |

## Net Accounting

| | Before | After |
|---|:------:|:-----:|
| Total hooks | ~42 scripts + 4 shared libs | ~11 staying + ~6 new journal hooks |
| Shell scripts | 42 | ~5 (side-effect + CLI-wrapping only) |
| Shared libraries | 4 | 0 (logic moves to `loaf check` TypeScript) |
| `loaf check` commands | 0 | 5-6 (enforcement checks) |
| `loaf session` commands | 0 | 4 (start, end, log, archive) |
| Skill instruction additions | 0 | ~20 (verification sections in 8 skills) |
