# Batch Orchestration

## Contents
- Orchestration Options
- Batch Resolution and Dependency-Ready Scheduling
- Option Handling
- Batch Execution Model
- Blocked-State Recovery

Detailed reference for running a parent issue or a set of issue refs with dependency-ready scheduling.

## Orchestration Options

| Option | Behavior |
|--------|----------|
| `--dry-run` | Show dependency-ready execution plan, do not run agents |
| `--parallel` | Run issues in the same dependency-ready group concurrently (max 3 at once) |
| `--continue` | Resume a blocked orchestration from the recorded issue/group |
| `--skip <ref>` | Skip one blocked issue and continue |
| `--abort` | Mark orchestration as aborted and stop remaining work |

## Batch Resolution and Dependency-Ready Scheduling

For a parent ref (`loaf issue tree <ref>`) or a named set of refs:

1. Resolve the selected refs and validate each issue exists (`loaf issue show <ref>`).
2. Read `blocks` / `blocked_by` edges and parent/child structure. Parent/child is not a sequencing edge — only `blocks` / `blocked_by` are.
3. Group unblocked delivery children into dependency-ready rounds:
   - First round: issues with no unresolved predecessors
   - Each subsequent round: issues whose predecessors are `done`, `cancelled`, or `duplicate`
4. If `--parallel` is set, allow parallel execution only within a dependency-ready round, max 3, and only when each agent has its own started worktree.
5. Present execution plan (issues, dependency-ready rounds, mode, total count) and ask for confirmation unless `--dry-run`.
6. Track progress in the journal: log round boundaries and the current ref with `loaf journal log`. Status moves through `loaf issue start` (to `active`) and, after landing, `loaf issue status <ref> done`. The journal plus issue statuses are the durable record of where the batch is.

Parents with children are not the implementation target. Dispatch leaf delivery children that are on `loaf issue frontier`.

## Option Handling (`--continue`, `--skip`, `--abort`)

1. Recover batch progress from the journal: `loaf journal recent --since-last-wrap` (or `loaf journal context`) plus `loaf issue list --json` and `loaf issue list --started` to see which issues are still open or claimed.
2. If `--continue`: resume from the last logged dependency-ready round and issue.
3. If `--skip <ref>`: log the reason with `loaf journal log`, continue the same dependency-ready round. Do not mark the skipped issue `done`.
4. If `--abort`: log `block(orchestration): aborted`, print a summary, and stop.
5. If no in-flight batch is evident from the journal, report that and ask for fresh selection input.

## Batch Execution Model

When input resolves to multiple issues, run a dependency-ready round loop:

1. Set orchestration mode (`sequential` by default, `parallel` only with `--parallel`).
2. For each dependency-ready round:
   - Log the round start with `loaf journal log`
   - For each issue: `loaf issue list --started`, then `loaf issue start <ref>` unless already started, spawn one agent into `started_worktree`, run `loaf issue verify <ref>` (V-tier; writes nothing)
3. If any issue fails verification, stop immediately and log `block(orchestration): <ref> failed <reason>`.
4. Consider a round complete only when all its issues have landed (`loaf issue status <ref> done` via ship) or were skipped.
5. Continue until all rounds complete, then log a closing entry summarizing the batch.

## Blocked-State Recovery

When blocked, always print:

- Failed issue ref and title
- Dependency-ready round and current progress
- Failure reason + key error output
- Recovery options:

Re-invoke the implement workflow with:

- `--continue` — after fixes are applied, retry from the blocked issue
- `--skip <ref>` — skip only the specified issue and continue remaining issues in the current dependency-ready round
- `--abort` — finalize the orchestration as aborted with no further execution
