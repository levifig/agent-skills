# Troubleshooting

Diagnosing state, config, and alignment failures, and working against throwaway
state safely.

## Which diagnostic

Three `doctor`/`check` surfaces cover different layers — reach for the one that
matches the symptom:

- `loaf config check` — `.agents/loaf.json` validity and the installed
  Loaf-managed hook config. Use it when config looks stale or missing, or hooks
  misfire. The full repair workflow lives in the configuration reference, linked
  from SKILL.md.
- `loaf state doctor` — SQLite state health (missing database, schema drift).
  `--fix` initializes missing state when safe; `--dry-run` prints the repair plan.
  Pair with `loaf state status` for readiness and markdown-only compatibility mode.
- `loaf doctor` — checkout alignment: symlinks, stale files, version drift.
  `--fix` applies safe auto-fixes.

Rule of thumb: config → the JSON config file and hooks; `state doctor` → the
database; `doctor` → the checkout's Loaf wiring.

## Isolating a throwaway database

Every real `loaf` command writes the global SQLite database
(`~/.local/share/loaf/loaf.sqlite`). Before scratch experiments, point `LOAF_DB`
at an absolute temp path so production state is untouched:

```bash
export LOAF_DB="$(mktemp -d)/loaf.sqlite"
loaf journal recent      # operates on the isolated database
rm -f "$LOAF_DB"         # clean up when done
```

An absolute `LOAF_DB` is used verbatim as the SQLite file and overrides
`XDG_DATA_HOME`; a relative value is ignored (standard XDG resolution applies).

## State backup and restore

Backups have explicit tiers. `loaf state backup` creates a `local_rollback` snapshot under the global data-home backup directory; project-scoped replay is the ordinary rollback path for later migrations; and `loaf state backup --to /absolute/external/directory` creates an `external_disaster_copy` at an operator-selected non-temporary external destination. The external path is not proof of off-device durability, so `device_loss_protected` remains false. Verify with `loaf state backup verify <backup>` and inspect SQLite validity, journal retrieval readiness, recovery readiness, checksum, and the journal watermark.

Use `loaf state backup restore <backup> --to /absolute/empty/rehearsal/loaf.sqlite` for an isolated disposable restore rehearsal. The command requires an empty absolute target, proves exact bytes and table/search/watermark parity, and never activates or replaces the live database. There is no automated live mutation lease and no concurrent restore claim.

If a verified copy must be activated, stop or terminate every harness, Loaf process, background writer, and process that might retain an open database connection, then verify universal quiescence first. Verify the durable backup and isolated rehearsal, retain a preserve-current backup, then while quiesced move the old main database and any matching `-wal` and `-shm` sidecars together into durable quarantine; never mix sidecars from different databases. Install the verified copy with mode `0600`, start current Loaf, run `loaf state doctor`, `loaf state status`, and a known journal retrieval check, and if validation fails re-quiesce and activate the preserve-current copy.

Ephemeral Markdown remains separate: verify with `loaf state verify-ephemerals <manifest|backup-dir|backup-id>`, then restore and stage with `loaf state restore-ephemerals <manifest|backup-dir|backup-id>`.
