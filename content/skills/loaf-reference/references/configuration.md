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

## GitHub identity enforcement: choosing a tier

`integrations.github.account` (above) names *which* GitHub identity a project's `gh`
commands must run as. *How* that policy is enforced is a separate, per-machine choice:
the repo picks the account, your user-level config picks the mechanism, and a cloned
repo can never install machinery on your machine. Two tiers enforce the same invariant
with different footprints:

- **Tier 1 — the `github-account` hook (default, always on).** Before every `gh`
  command it converges gh's active-account pointer to the configured account. Zero
  setup, but it *writes* the shared global pointer (`~/.config/gh/hosts.yml`) on every
  mismatched call, so concurrent sessions on different identities collide on that
  pointer more often. It is the fallback that runs wherever Loaf hooks fire.
- **Tier 3 — the opt-in `gh` shim.** `loaf shim enable gh` symlinks `gh` through the
  loaf binary. Inside a project with `integrations.github.account` set, each `gh`
  invocation resolves that named account's token from gh's own keychain and execs the
  real `gh` with `GH_TOKEN` set for that one child process — nothing shared, nothing to
  race. Everywhere else it execs the real `gh` untouched, and gh's global pointer is
  only read, never written. Agents keep typing bare `gh`.

Reach for tier 3 on **multi-identity machines** (you hold more than one authenticated
GitHub account) or when you **run concurrent sessions** across worktrees or harnesses
that would otherwise fight over the active-account pointer. On a single-identity
machine tier 1 is enough and needs no setup.

### Enabling, disabling, and checking the shim

- `loaf shim enable gh` — prints the exact machine changes (a symlink under
  `~/.local/share/loaf/shims/`, a `shims.gh` record in `~/.config/loaf/config.json`, and
  a PATH line for your shell profile) and asks before applying any of them. Consent is
  explicit and per-machine; nothing happens without a `y` (or `--yes` for
  non-interactive runs). Requires gh ≥ 2.40.0 — the release that added
  `gh auth token --user` — and refuses older gh rather than silently downgrading.
- `loaf shim disable gh` — removes the symlink and the config record in one command; the
  PATH line is left in place (harmless once the shims directory is empty).
- `loaf shim status` (or `loaf doctor`) — reports shim health. The troubleshooting
  reference lists the states and the residual-exposure cases.

The shim never enables itself: there is no repo-side knob, no auto-detect heuristic, and
no nag. If it isn't turned on for this machine, it doesn't exist here.

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
