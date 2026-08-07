<!-- brief.md is the optional archeological kickstart — the original unshaped ask.
     May accrete parked problem-space concepts while the change is captured; freezes when shape.md exists.
     Superseded by shape.md; never mechanically load-bearing.
     A brief-only folder is legal and non-executable (captured, not shaped). -->

# State Dedupe

## Problem Statement

The SQLite artifact-body migration of 2026-06-24 (~13:03Z) left duplicate rows across the tasks, specs, and reports tables, carrying raw pre-normalization status vocabulary. The duplicates are mostly invisible to the canonical list commands but fully visible to the housekeeping scanner, so the two surfaces disagree about how much work exists: the scanner reports 299 tasks / 38 specs / 14 reports where the canonical views hold 233 / 26 / 11. Sixty-three of the duplicate task rows were exact-title copies of already-archived TASK-190..252 (archived during the 2026-08-07 housekeeping pass, which is when the event was discovered).

Two defects ride along. First, list surfaces lie: `loaf task list --json` returns zero rows for done-status tasks that provably exist, and three duplicate report rows are unreachable through any CLI command at all — they can be seen only by reading the database directly. Second, one report row (`transitional-surfaces-do-not-deepen`, status active) cites a source markdown path and content hash that do not exist on disk and have no git history — a broken evidence reference whose original content may be unrecoverable.

## Who Has It

The operator, at every housekeeping pass (scanner counts can't be trusted as distinct-work counts) and at any workflow that consumes `--json` list output (a done-task query that returns nothing is worse than an error — it reads as "no work"). Agents inherit the same blindness: the librarian pass had to bypass the CLI and read SQLite directly to produce a truthful disposition report.

## Current Alternatives

Read the database directly (what the librarian did — works, but bypasses the protocol layer the CLI exists to be), or ignore the mismatch and let scanner counts drift further from reality with each migration.

## Constraints

- Dedupe is data surgery on the global production database: it rides the Recovery Tiers discipline — `loaf state backup` first, verify after, and the operation must be idempotent and re-runnable.
- Canonical rows win; duplicate rows with pre-normalization status vocab are the ones to retire. The 2026-06-24 timestamp cluster identifies the event, but matching should be by content identity (title/body), not timestamp alone.
- The list surfaces must be fixed to report what exists: done-status tasks appear in `loaf task list --json`, and no row reachable by the scanner may be unreachable by every list command.
- The broken-reference report row needs an explicit disposition, not silent repair: archive-as-moot (SPEC-047 already shipped the simplification it guarded against deepening) or an explicit unrecoverable marker. Fabricating replacement content is off the table.
- Housekeeping scanner and canonical list commands must agree on counts when the dedupe lands — that agreement is the acceptance signal.

## Sources and Research Links

- Journal `finding(state)` of 2026-08-07 (the discovery, with counts).
- Journal `decision(housekeeping)` of 2026-08-07 (the 66-task archive that surfaced the event).
- Librarian disposition report, 2026-08-07 session: raw duplicate report row IDs `report:3feb9615…`, `report:fb1b930d…`, `report:e65a4327…`; the zero-rows `loaf task list --json` reproduction; the `transitional-surfaces-do-not-deepen` broken reference.

## Open Questions

- [ ] Whether the June-24 migration path is still live (could re-create duplicates on the next migration) or was a one-time event — determines whether the fix needs a guard or just the cleanup.
- [ ] Whether `loaf task list`'s zero-rows-for-done is the same defect as the duplicate invisibility or an independent filter bug.
