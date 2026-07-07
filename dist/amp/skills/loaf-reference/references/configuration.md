# Configuration Maintenance

The guided answer to "is this project's Loaf config current, and what fixes it?"
Work top to bottom; stop when the checks pass.

## Diagnose first

Run `loaf config check --json` and read the structured findings — never scrape the
human-readable output. It validates `.agents/loaf.json` and the installed
Loaf-managed hook config, and flags missing safe defaults, stale installed-target
config, and malformed fields.

## Safe repairs

`loaf config check --fix` creates missing safe defaults and refreshes stale
installed-target hook config. Review what it changed, then re-run
`loaf config check --json` to confirm it passes. `--fix` only touches mechanical,
reversible repairs — it never invents project-owned values.

## Project-owned choices (ask the user)

These are decisions, not defaults. The CLI cannot guess them — ask, then record:

- **GitHub account** — `integrations.github.account`, the login the project's `gh`
  commands must run as. The `github-account` enforcement hook checks it against
  `gh auth status`; a mismatch tells the user to run `gh auth switch`.
- **Tracker / integration election** — `integrations.linear.enabled` and the other
  `integrations.*` toggles.
- **Which harnesses to install** — the targets passed to `loaf install --to <target>`
  (or `--to all`).

## Hand-edit only where no setter exists

Some fields (for example `integrations.github.account`) have no CLI setter. Edit
`.agents/loaf.json` narrowly for those, leave everything else to the CLI, and
immediately re-validate with `loaf config check --json`. Never hand-edit
Loaf-managed hook files — regenerate them through `loaf build` and `loaf install`.

## Refresh installed targets

`loaf install --upgrade` updates already-installed targets and applies
deprecation-manifest cleanup (removing retired skills and targets). Run it after
config or skill changes so installed harnesses match the source.

## Confirm project alignment

`loaf doctor` checks symlinks, stale files, and version drift across the checkout;
`loaf doctor --fix` applies safe auto-fixes. This catches drift that config-level
checks do not.

## Record the decisions

Log the project-owned choices you settled with the user:
`loaf journal log "decision(config): ..."`.
