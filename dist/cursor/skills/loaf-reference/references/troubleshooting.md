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

## gh shim diagnostics

When the opt-in `gh` identity shim is enabled (see the configuration reference),
`loaf shim status` and the `gh-shim` check in `loaf doctor` report one of five states:

- `absent` — the shim is not enabled; run `loaf shim enable gh` to turn it on.
- `healthy` — the symlink exists, resolves first on `PATH`, and points at a live real gh.
- `path-shadowed` — the symlink exists but `PATH` does not resolve `gh` to it yet. Put the
  shims directory ahead of the real gh's directory on `PATH`; a just-enabled shell hasn't
  sourced the new PATH line yet, so restart the shell or `source` the profile.
- `broken-symlink` — config records a shim but the symlink is missing, is not a symlink, or
  points at a target that no longer exists. `loaf shim status` exits non-zero;
  `loaf doctor --fix` can rebuild it, or re-run `loaf shim enable gh`.
- `real-gh-missing` — the recorded real gh moved or was uninstalled. `loaf shim status`
  exits non-zero; re-run `loaf shim enable gh` to record the new path.

### Known residual exposures

The shim covers PATH-resolved `gh` inside identity-configured Loaf projects; a few paths
sit outside its reach by construction. None is a shim malfunction:

- **HTTPS `git push`/`pull` bypasses the shim.** `gh auth setup-git` bakes an *absolute*
  path to the real gh into your gitconfig credential helper
  (`!/opt/homebrew/bin/gh auth git-credential`), so plain git-over-HTTPS calls that binary
  directly and still consults gh's active-account pointer. Tier 1 (the `github-account`
  hook) catches Loaf-hooked sessions here; SSH remotes are unaffected.
- **GUI-launched apps never see the shim.** Apps started from the Dock, Spotlight, or
  launchd don't source your shell profile, so they resolve `gh` via `/etc/paths.d` — the
  real gh. IDEs and GUI git clients see the unshimmed gh unless launched from a shim-aware
  terminal. This is an inherent PATH-shim limit (the same one rbenv and direnv have).
- **`gh auth status` shows `(GH_TOKEN)`.** Under a shimmed invocation, `gh auth status`
  tags the account `(GH_TOKEN)` instead of `(keyring)`. This is cosmetic and
  spike-verified: the token is the named account's own token and `GH_TOKEN` mutates no gh
  state on disk. It does not indicate a problem.
- **A cloned repo's `.agents/loaf.json` selects the identity.** A repo you clone can
  commit `integrations.github.account`, and that value silently picks *which* of your
  already-authenticated identities the shimmed `gh` runs as. It is bounded to identities
  you already hold tokens for — never new credentials — but review an untrusted repo's
  `.agents/loaf.json` before running `gh` inside it.

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
