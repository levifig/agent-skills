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

- Back up: `loaf state backup` writes under the global data-home backups directory.
- Verify: `loaf state backup verify <backup>` — add `--json` for
  `restore_database_path`, `restore_preserve_path`, and `restore_validation_commands`.
- Restore is explicit until a guarded restore command exists: verify the backup,
  preserve the current `$(loaf state path)` file, copy the verified backup to that
  path, then run `loaf state doctor` and `loaf state status`.
- Ephemeral Markdown: verify with
  `loaf state verify-ephemerals <manifest|backup-dir|backup-id>`, then restore and
  stage with `loaf state restore-ephemerals <manifest|backup-dir|backup-id>`.
