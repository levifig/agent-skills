# CLI Boundary

Reading `loaf issue` — the skill teaches reading the CLI, not wrapping it. For the rest of the Loaf CLI surface, see the `loaf-reference` skill. Issue commands require initialized SQLite state (`loaf state init`, or `loaf state migrate markdown --apply`).

## Contents
- `loaf issue new`
- `loaf issue show` / `list` / `tree` / `frontier`
- `loaf issue edit` / `status`
- `loaf issue dod`
- `loaf issue promote`
- `loaf issue check`
- `loaf issue verify`
- `loaf issue link` / `bucket`
- `loaf issue render`
- `loaf release suggest` / `cut`
- What shape does not run

## `loaf issue new <title> [options]`

```text
loaf issue new <title> [--body <text>|--body -|--body-file <path>|--message <text>]
  [--kind delivery|decision] [--parent <ref>] [--fog <text>] [--status <status>] [--json]
```

Creates the issue row. Default kind is `delivery`; default status is `triage`. `--status` accepts the write statuses `triage`, `backlog`, `todo`, `active`, `done` (it still records the initial triage event). `--fog` parks questions not yet sharp enough to be issues; this flag exists only on create — `loaf issue edit` replaces the body and does not mutate `fog`.

`--body -` reads stdin; `--body-file` reads a UTF-8 file; `--message` is inline body at lower precedence than `--body-file` and `--body -`. A hyphen-leading title is positional after `--`:

```bash
loaf issue new --parent LOAF-42 --status backlog -- "--help is missing from the man page"
```

A delivery body must state the problem and, before `loaf issue check` will pass, contain the substring `out of scope` (case-insensitive). A decision issue needs a sharp question (`?` in the title or body), not a body contract.

## `loaf issue show` / `list` / `tree` / `frontier`

```text
loaf issue show <ref> [--json]
loaf issue list [--status <status>] [--kind delivery|decision] [--archived] [--started] [--json]
loaf issue tree [<ref>] [--archived] [--json]
loaf issue frontier [--json]
```

`show` prints identity, parent, fog, body, definition of done, and children. `list` hides archived issues unless `--archived`. `--status` filters by `triage`, `backlog`, `todo`, `active`, `done`, `cancelled`, `duplicate`. `tree` prints from a ref, or the whole project when omitted. `frontier` lists non-archived `triage`/`backlog`/`todo` issues that are not blocked — derived at read time, useful when checking whether this work is already covered.

Prefer `--json` when diagnosing rather than scraping the human-readable text.

## `loaf issue edit` / `status`

```text
loaf issue edit <ref> [--body-file <path>|--body -|--message <text>] [--json]
loaf issue status <ref> <status> [--duplicate-of <ref>] [--json]
```

`edit` **replaces** the body. Rewrite the full problem-plus-out-of-scope text; there is no patch form. `status` write-statuses (`triage`, `backlog`, `todo`, `active`, `done`) update in place; `cancelled` and `duplicate` archive through the remove path (`--duplicate-of` is required when status is `duplicate`). Shape leaves status at `triage` unless the user asks otherwise — shaped is derived, not a status.

## `loaf issue dod`

```text
loaf issue dod add <ref> <text> [--command <cmd>] [--expect <expect>] [--tier V|H] [--serves <parent-position>] [--json]
loaf issue dod list <ref> [--json]
loaf issue dod remove <ref> <position> [--json]
loaf issue dod claim <child> <child-position> <parent-position> [--json]
loaf issue dod unclaim <child> <child-position> <parent-position> [--json]
```

V-tier is used when `--command` is present, otherwise H, unless `--tier` overrides. `--serves` records that the new child criterion claims that parent position. Positions are 1-based and compact after `remove`. Authoring guidance and the expect grammar live in the Decomposition topic.

## `loaf issue promote <ref> <position> [--json]`

Promotes the criterion at the 1-based position into a child **delivery** issue. The parent criterion stays in place. The child is minted in `triage` with a copy of the criterion and a claim already recorded, so coverage for that parent position holds by construction.

## `loaf issue check <ref> [--json] [--human <reason>]`

Derives readiness from the issue row, not from markdown headings.

- **Delivery** — shaped when the body is nonempty (the problem), at least one criterion exists, and the body contains an explicit out-of-scope statement. Prints `issue <ref> is shaped` when ready.
- **Decision** — ready when the title or body contains `?`. Prints `issue <ref> is ready`.
- **Children present** — coverage is a failure (every parent criterion must be claimed). Containment is a report (every child criterion must claim a parent criterion); each orphan prints a ready-to-paste `loaf issue new --parent … --status backlog -- …` remedy.

`--human <reason>` publishes ready-for-human instead of ready-for-agent when a tracker authority is configured. Shape's own gate is the derived verdict, not the publication.

`--json` emits `{issue, kind, shaped, covered, ready, failures, orphans, …}`. Exit code 1 when not ready.

## `loaf issue verify <ref> [--json]`

Runs the issue's V-tier criteria (`--command` plus `--expect`) from the repository root. Honors `exit <N>` and `` contains `text` ``. Writes nothing; exits non-zero on any failure. H-tier rows are skipped. This is implement's preflight, not shape's gate.

A criterion passes when the command ran, the exit code matched, and every `contains` matched. Unenforceable expect clauses are warned and recorded as advisory — never quietly decorative.

## `loaf issue link` / `bucket`

```text
loaf issue link <from> blocks|relates-to <to> [--json]
loaf issue link <from> remove <type> <to> [--json]
loaf issue bucket <ref> now|next|later|none [--json]
```

Stored relationship types are `blocks` and `relates_to`. Use `blocks` for a real sequencing constraint; do not encode order in `loaf issue tree`. Buckets are labels only and are never read as a constraint.

## `loaf issue render <ref> [--json]`

Emits markdown suitable to paste as a PR body with no manual editing: title, body, definition-of-done checkboxes (checked only when status is `done`), and children. Nothing plan-shaped is committed; if a PR is opened, this output *is* the body.

## `loaf release suggest` / `cut`

Releases are retroactive. Shape does not bind an issue to a version.

```text
loaf release suggest [--base <ref>] [--json]
loaf release cut [--base <ref>] [--bump <type>] [--includes <version|tag>] [--no-tag] [--no-gh] [--dry-run]
```

`suggest` reports landed work since the last version tag and writes nothing. `cut` records a release from landed work. Neither is a shaping step.

## What shape does not run

`loaf issue start` / `stop` create and remove the issue worktree — implement's job, after the issue is shaped. `loaf issue export` dumps the project snapshot. Do not call them from this skill.
