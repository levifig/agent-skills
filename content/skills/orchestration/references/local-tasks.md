# Working Issues Locally

Orchestration-facing reference for the Loaf issue CLI: pick-up-next, started
worktrees, status, definition of done, and advisory labels. Issue commands
require initialized SQLite state.

## Contents

- Frontier
- Started worktree
- Status vocabulary
- Relationships
- Definition of done
- Buckets
- Command cheat sheet
- LEGACY

## Frontier

```text
loaf issue frontier [--json]
```

Pick-up-next. Derived at read time, never stored. Lists non-archived issues in
`triage`, `backlog`, or `todo` that are not blocked.

| Qualifier | Meaning |
|-----------|---------|
| Open | Status is `triage`, `backlog`, or `todo` — not `active`, `done`, `cancelled`, or `duplicate` |
| Unblocked | No open predecessor via `blocks` / `blocked_by`. A predecessor that is `done`, `cancelled`, or `duplicate` does not block |
| Unclaimed | Not `active` and no started worktree. `loaf issue start` is the claim |

Archived rows are excluded. Kind is not filtered: a `--kind decision` question
can appear; it is not delivery work. Buckets are not read. Prefer `--json`
when diagnosing rather than scraping the human-readable text.

## Started worktree

```text
loaf issue start <ref> [--json]
loaf issue stop <ref> [--force] [--json]
loaf issue list --started [--json]
```

**Invariant:** one agent, one worktree. Check `loaf issue list --started`
before dispatch. Never send two agents into the same path.

`start` walks `parent_id` to the shippable root and creates or joins branch
`issue/<root-alias-or-id>` in lowercase (`issue/loaf-42`, disambiguated with an
id suffix when that name is already claimed). Only the root records
`started_branch` / `started_worktree`. The requested issue becomes `active`.
Base is the repository default branch. Start refuses an already started root, an
archived row, and terminal statuses (`done`, `cancelled`, `duplicate`). A
descendant that does not own a worktree cannot be stopped; stop the root.
Requires a git repository.

`list --started` prints alias, title, `started_branch`, `started_worktree`, and
`(missing)` when the recorded path is gone.

`stop` removes the worktree and clears the started workspace on the row. It
keeps the branch and does not change status. `--force` removes a dirty
worktree. Do not run `stop` from inside the started worktree.

## Status vocabulary

Write statuses that update in place: `triage`, `backlog`, `todo`, `active`,
`done`. `cancelled` and `duplicate` archive through the remove path
(`loaf issue status <ref> duplicate --duplicate-of <surviving>`).

```text
loaf issue status <ref> <status> [--duplicate-of <ref>] [--json]
```

| Status | Meaning |
|--------|---------|
| `triage` | Default at create. Shaped is derived (`loaf issue check`), not a status |
| `backlog` | Filed, worth keeping |
| `todo` | Explicitly ready to work |
| `active` | Started. **Review is a display name for `active`** — there is no `review` write status |
| `done` | Work landed |
| `cancelled` | Archived; abandoned |
| `duplicate` | Archived; `--duplicate-of` required |

There is **no `blocked` status**. Blocked is a relationship. Title and body stay
mutable at every status.

```text
loaf issue list [--status <status>] [--kind delivery|decision] [--archived] [--started] [--json]
```

Archived rows are hidden unless `--archived`. `--status` accepts every value in
the table above.

## Relationships

```text
loaf issue link <from> blocks|relates-to <to> [--json]
loaf issue link <from> remove <type> <to> [--json]
```

Stored types are `blocks` and `relates_to`. `loaf issue link A blocks B` means
A blocks B: B is absent from the frontier until A is `done`, `cancelled`, or
`duplicate`. `relates-to` is not a sequencing constraint.

Do not encode order in `loaf issue tree`. Parent/child is structure; `blocks`
is the dependency. `loaf issue export [--json]` dumps relationships (and
claims) when you need the graph.

## Definition of done

Criteria live on the issue row. `loaf issue show <ref>` prints each as
`position. [V|H] text` with `command=` / `expect=` when present.

```text
loaf issue dod add <ref> <text> [--command <cmd>] [--expect <expect>] [--tier V|H] [--serves <parent-position>] [--json]
loaf issue dod list <ref> [--json]
loaf issue dod remove <ref> <position> [--json]
loaf issue dod claim <child> <child-position> <parent-position> [--json]
loaf issue dod unclaim <child> <child-position> <parent-position> [--json]
loaf issue promote <ref> <position> [--json]
loaf issue check <ref> [--json] [--human <reason>]
loaf issue verify <ref> [--json]
```

| Tier | When | Who checks |
|------|------|------------|
| V | `--command` present, unless `--tier` overrides | `loaf issue verify <ref>` from the repository root. Honors `exit <N>` and `` contains `text` ``. Writes nothing. Non-zero on failure |
| H | No `--command`, unless `--tier` overrides | Human or orchestrator. Verify skips H-tier; that skip is not a pass |

Claims: a child criterion serves a parent criterion. `promote` copies the
parent criterion onto a new delivery child and records the claim.
`--serves` claims a newly added child criterion. `claim` / `unclaim` retarget
an existing pair. Positions are 1-based.

`check` is readiness (shape's gate): delivery is shaped with a nonempty body,
at least one criterion, and an out-of-scope statement; decision is ready on a
sharp `?`. Children add coverage (every parent criterion claimed — failure)
and containment (every child criterion claims a parent — report). `verify` is
implement's preflight and writes nothing — it does not set status and does not
tick boxes.

`loaf issue render <ref>` emits the paste-ready PR body: title, body,
definition-of-done checkboxes (checked only when status is already `done`),
and children. No manual editing.

## Buckets

```text
loaf issue bucket <ref> now|next|later|none [--json]
```

Advisory Now/Next/Later labels. Never read as a constraint. Frontier, start,
and verify ignore them. `none` clears the label.

## Command cheat sheet

```text
loaf issue new <title> [--body <text>|--body -|--body-file <path>|--message <text>]
  [--kind delivery|decision] [--parent <ref>] [--fog <text>] [--status <status>] [--json]
loaf issue show <ref> [--json]
loaf issue tree [<ref>] [--archived] [--json]
loaf issue edit <ref> [--body-file <path>|--body -|--message <text>] [--json]
loaf issue export [--json]
```

`new` default kind is `delivery`; default status is `triage`. `--status` on
create still records the initial triage event, then writes the requested
write-status. `--fog` exists only on create. `edit` replaces the body; there
is no patch form.

## LEGACY

Leftover TASK/INTENT rows move with `loaf issue absorb <ref>` or
`loaf issue absorb --all [--history]`. `loaf task` and `loaf spec` remain
readable against leftover SQLite rows. Writes stay frozen through 0.4.x.
Namespaces delete in 0.5.0 (LOAF-47). Do not create records there. Issues are
the work unit.
