# Markdown Migration (`loaf migrate markdown`)

## Contents
- Commands
- Dry-run mode discriminant
- import_report fields
- Origin collision semantics
- Status authority
- Parity precondition
- Discover live flags

One-way import of legacy `.agents` Markdown into SQLite. Prefer
`loaf migrate markdown --help` (or `loaf state migrate markdown --help`) for
exact flags; this reference states the authority contract agents must respect.

## Commands

| Intent | Command |
|--------|---------|
| Simulate or inventory (no live DB write) | `loaf migrate markdown --dry-run` |
| Apply import | `loaf migrate markdown --apply` |
| Resume interrupted apply | `loaf migrate markdown --resume` (alias of apply) |
| Machine-readable envelope | add `--json` |

Top-level `loaf migrate markdown` and `loaf state migrate markdown` are the same
native path.

## Dry-run mode discriminant

`--dry-run` always sets `mode`:

| `mode` | When | Live DB | `import_report` |
|--------|------|---------|-----------------|
| `simulation` | Database file exists and the project is registered | Unchanged (import runs on a disposable snapshot) | Present |
| `inventory` | No database, or database without a registered project row | Untouched / may be absent | Absent |

Inventory is an honestly labeled file-count preview. It does not detect origin
collisions or status divergences; conflict detection requires an initialized
SQLite project. Simulation runs the full apply pipeline against a verified
throwaway snapshot (`VACUUM INTO` primitive with caller-owned cleanup — not the
public `state.Backup` wrapper), then discards the snapshot.

## import_report fields

Present on simulation dry-run, apply, and resume. Absent on inventory dry-run.

| Field | JSON key | Meaning |
|-------|----------|---------|
| Reclaimed origins | `reclaimed_origins` | Origin rows rewritten as `migration` because they matched the 0011-compatible fingerprint |
| Skipped entries | `skipped_entries` | Journal lines left untouched; each entry names `journal_entry_id` and `capture_mechanism` |
| Status divergences | `status_divergences` | Kept SQLite status disagreed with normalized incoming; includes `entity_kind`, `entity_id`, `stored_status`, `incoming_status` |
| Outcome warnings | `warnings` | Status/provenance outcomes (e.g. out-of-vocabulary source status) |

Plan-level `warnings` keep inventory I/O and parser warnings only. Do not
conflate the two warning lists.

## Origin collision semantics

Apply never aborts because an origin already exists. An origin is reclaimed only
when it is information-free above its paired journal row (full 0011-compatible
fingerprint). Every other non-`migration` origin — `manual`, `skill`, `hook`,
custom mechanisms, future envelopes, dirty or evidence-bearing `unknown` rows,
and journal-copy mismatches — is skipped whole. The skipped list is the audit
trail; there is no `--skip-conflicts` flag.

## Status authority

Stored `unknown` is never authoritative: a real normalized incoming status may
fill it; any other stored status is insert-only and never overwritten. Archived
cannot flip back through re-import. Absent or explicit `unknown` source status
is no-opinion (no fill, no divergence). Out-of-vocabulary values warn and never
fill a stored `unknown`.

## Parity precondition

Simulation and apply report the same `import_report` only when the database and
`.agents` tree are unchanged between the two commands. Loaf takes no
cross-command lock; state that mutates between dry-run and apply can diverge.
Inventory mode makes no parity claim — there is no apply outcome to compare
until a registered project exists.

## Discover live flags

```bash
loaf migrate markdown --help
loaf migrate markdown --dry-run --json
loaf migrate markdown --apply --json
```
