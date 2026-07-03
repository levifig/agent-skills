# Batch Orchestration

## Contents
- Orchestration Options
- Batch Resolution and Wave Planning
- Option Handling
- Batch Execution Model
- Blocked-State Recovery

Detailed reference for running specs, task ranges, or task lists with dependency waves.

## Orchestration Options

| Option | Behavior |
|--------|----------|
| `--dry-run` | Show dependency/wave execution plan, do not run agents |
| `--parallel` | Run tasks in the same wave concurrently (max 3 at once) |
| `--continue` | Resume a blocked orchestration from the recorded task/wave |
| `--skip TASK-XXX` | Mark one blocked task as skipped and continue |
| `--abort` | Mark orchestration as aborted and stop remaining work |

## Batch Resolution and Wave Planning

For `SPEC-XXX`, `TASK-XXX..YYY`, and `TASK-XXX,YYY,ZZZ`:

1. Resolve selected tasks and validate each task file exists.
2. Extract `depends_on` from each task and build a dependency graph.
3. Group tasks into dependency waves:
   - Wave 1: tasks with no unresolved dependencies
   - Wave N: tasks whose dependencies are completed in earlier waves
4. If `--parallel` is set, allow parallel execution only within a wave and only for non-conflicting tasks.
5. Present execution plan (tasks, waves, mode, total count) and ask for confirmation unless `--dry-run`.
6. Track progress in the journal and in task statuses: log wave boundaries and the current task with `loaf journal log`, and drive each task's status with `loaf task update`. The journal plus task statuses are the durable record of where the batch is.

## Option Handling (`--continue`, `--skip`, `--abort`)

1. Recover batch progress from the journal: `loaf journal recent --since-last-wrap` (or `loaf journal context`) plus `loaf task list --json` to see which tasks are still open.
2. If `--continue`: resume from the last logged wave and task.
3. If `--skip TASK-XXX`: mark that task `skipped` via `loaf task update`, log the reason with `loaf journal log`, continue the same wave.
4. If `--abort`: log `block(orchestration): aborted`, print a summary, and stop.
5. If no in-flight batch is evident from the journal, report that and ask for fresh selection input.

## Batch Execution Model

When input resolves to multiple tasks, run a wave-based loop:

1. Set orchestration mode (`sequential` by default, `parallel` only with `--parallel`).
2. For each wave:
   - Log the wave start with `loaf journal log`
   - Run each task (sequentially, or concurrently within safety limits)
   - For each task: set `in_progress` -> spawn agent -> run task verification -> mark `done`/`failed` via `loaf task update`
3. If any task fails verification, stop immediately and log `block(orchestration): <task> failed <reason>`.
4. Consider a wave complete only when all its tasks are `done` or skipped.
5. Continue until all waves complete, then log a closing entry summarizing the batch.

## Blocked-State Recovery

When blocked, always print:

- Failed task ID and title
- Wave number and current progress
- Failure reason + key error output
- Recovery commands:

```bash
{{IMPLEMENT_CMD}} --continue
{{IMPLEMENT_CMD}} --skip TASK-XXX
{{IMPLEMENT_CMD}} --abort
```

Use these semantics:

- `--continue`: after fixes are applied, retry from the blocked task
- `--skip`: skip only the specified task and continue remaining tasks in the current wave
- `--abort`: finalize the orchestration as aborted with no further execution
