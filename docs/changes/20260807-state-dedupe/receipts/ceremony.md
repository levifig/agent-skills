# Production Repair Ceremony — Receipts

Executed 2026-08-09 (UTC 03:28–04:06), operator present, against `~/.local/share/loaf/loaf.sqlite`, using a binary built from main at `34761008` (the landed squash of PR #159). Every apply was rehearsed in preview with the identical invocation before running.

## Backups and rollback manifests (retained in `~/.local/share/loaf/backups/`)

| Step | Backup | Rollback manifest |
|---|---|---|
| Pre-surgery | `loaf-20260809-032843-120664000.sqlite` (verified) | — |
| alias-orphans apply | `loaf-20260809-033934-282482000.sqlite` | `alias-orphan-rollback-20260809T033935Z.json` |
| journal-duplicates apply | `loaf-20260809-040450-901042000.sqlite` | `journal-duplicate-rollback-20260809T040459Z.json` |
| lifecycle-statuses apply | `loaf-20260809-040538-031233000.sqlite` | `lifecycle-status-rollback-20260809T040538Z.json` |

## alias-orphans

Preview matched the review-time rehearsal exactly: 191 orphans — 168 retire (proven), 23 unproven, 1 dead alias, 150 orphaned sources, 1 named disposition. First apply exited 0 with post-apply verification passing.

Operator dispositions (23 rows):

- 3 June-13 archived task orphans with no title twin — retired (content preserved in git history and the manifest).
- 14 spark orphans across 7 titles that already had an aliased June-24 survivor — retired; realiasing would have resurfaced duplicates.
- 3 spark pairs with no aliased survivor — June-24 member realiased (`release-post-merge-guardrail-inverts-conventional-commits`, `session-lifecycle-states`, `startup-context-from-previous-wrap-up`), June-13 member retired.

Named disposition: `report:7644bb23d2664de93b6cb6a5` archived as moot with a `status_normalized` event recording the unrecoverable evidence and the SPEC-047 rationale.

## journal-duplicates

Preview: 1,349 June-13-window rows, 1,156 June-24-window rows, 866 proven pairs, 614 unproven rows in 153 ambiguous groups. Multiplicity analysis: 119 groups equal across windows, 34 groups with fewer June-24 copies (the markdown's later truth). Operator policy: retire every June-13 member of a cross-window triple — 324 `--retire` dispositions generated from the migration's own classification.

Apply retired 1,190 rows, exit 0. Post-apply: zero pairs, zero unproven; the June-13 window settled at its 159 genuinely-unique rows. A first flags-file generation bug (`--retire None` × 324) was caught by the rehearse-in-preview discipline before any invocation reached apply.

## lifecycle-statuses (first run ever)

14 entities rewritten, 2 events rewritten, zero mappable legacy statuses remaining. 53 out-of-vocabulary free-text statuses (e.g. `raw`, `absorbed`, `captured`, `active`, `unknown`) surfaced as warnings and deliberately left untouched — TASK-408 territory.

## Acceptance

- `loaf state doctor`: alias-parity clear — 27 projects, 189 table checks, raw == alias-reachable everywhere, `multi_alias=0`, `dangling_aliases=0`, exit 0.
- Housekeeping scanner equals canonical list output exactly: tasks 233, specs 26, reports 11 (formerly 299/38/14), ideas 57, sparks 62, brainstorms 4.
- `loaf task list --status done --json` returns the truthful count (zero — all done tasks were archived pre-ceremony).
- Zero `(entry_type, scope, message)` journal twins remain across the two import windows.
