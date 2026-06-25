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
6. For batch runs, store `session.type: orchestration` and maintain `orchestration.current_wave`, `orchestration.current_task`, and wave/task statuses in frontmatter.

## Option Handling (`--continue`, `--skip`, `--abort`)

1. Locate active orchestration session with `loaf session list --json` and inspect it with `loaf session show <ref> --json`.
2. If `--continue`: resume from `orchestration.current_wave` and `orchestration.current_task`.
3. If `--skip TASK-XXX`: mark that task `skipped`, append reason in session log, continue same wave.
4. If `--abort`: set session/orchestration status to `aborted`, record timestamp, print summary, stop.
5. If no active blocked orchestration exists, report that and ask for fresh selection input.

## Batch Execution Model

When input resolves to multiple tasks, run a wave-based loop:

1. Set orchestration mode (`sequential` by default, `parallel` only with `--parallel`).
2. For each wave:
   - Mark wave `in_progress` in session
   - Run each task (sequentially, or concurrently within safety limits)
   - For each task: set `in_progress` -> spawn agent -> run task verification -> mark `completed`/`failed`
3. If any task fails verification, stop immediately and mark session `blocked`.
4. Mark wave `completed` only when all tasks in wave are completed or skipped.
5. Continue until all waves complete, then set session/orchestration status to `completed`.

## Blocked-State Recovery

When blocked, always print:

- Failed task ID and title
- Wave number and current progress
- Failure reason + key error output
- Session file path
- Recovery commands:

```bash
/loaf:implement --continue
/loaf:implement --skip TASK-XXX
/loaf:implement --abort
```

Use these semantics:

- `--continue`: after fixes are applied, retry from blocked task
- `--skip`: skip only the specified task and continue remaining tasks in the current wave
- `--abort`: finalize the orchestration as aborted with no further execution
