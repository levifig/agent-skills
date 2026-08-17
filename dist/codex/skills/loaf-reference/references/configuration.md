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
- **Issue identity** — `issue.authority` (`local`, `linear`, or `github`) and
  `issue.prefix`. Prefix is the local alias token (`VCAM`) or, when authority is
  `linear`, the Linear team key (`ENG`). `loaf init` and a newly created loaf.json
  record a derived local prefix; `loaf issue identity --prefix` / `--authority`
  is the setter. `--fix` never invents these on an existing file.
- **Tracker / integration election** — `integrations.linear.enabled` and the other
  `integrations.*` toggles. Linear execution identity is `issue.authority`, not
  the historical skill-mode toggle.
- **Which harnesses to install** — the targets passed to `loaf install --to <target>`
  (or `--to all`).
- **Codex basic command policy** — `loaf install --to codex --codex-basic-commands` explicitly installs path-pinned managed command rules and a matching `CODEX_HOME/AGENTS.md` guidance block for the centrally classified basic Loaf leaves, including hardened journal logging and approved readers, outside the workspace sandbox. This is narrower than Full access or adding the global Loaf data directory as a writable root; unclassified and operator commands remain gated. Do not enable it implicitly.
- **Codex lifecycle hooks** — Codex `0.144.1` rejects Loaf's legacy flat hook schema. Loaf installs a current-schema `SessionStart` matcher group with a trusted executable rendered at install; POSIX installs keep the path-pinned command and omit the Windows variant, while Windows installs render both command fields to the same `cmd.exe /C` wrapper. Startup is model-visible proven only for the isolated `darwin-arm64` smoke; other lifecycle modes and automatic completion remain gated, and managed journal guidance plus explicit CLI retrieval remain the fallback.

## Hand-edit only where no setter exists

Some fields (for example `integrations.github.account`) have no CLI setter. Edit
`.agents/loaf.json` narrowly for those, leave everything else to the CLI, and
immediately re-validate with `loaf config check --json`. Never hand-edit
Loaf-managed hook files — regenerate them through `loaf build` and `loaf install`.

## Refresh installed targets

`loaf upgrade` updates already-installed targets and applies deprecation-manifest cleanup (removing retired skills and targets). Run it after config or skill changes so installed harnesses match the source. It is safe to run from anywhere: project surfaces are refreshed only inside a detected Loaf repo, and `--to <target>` narrows the sync to one already-installed target.

`loaf install` is for onboarding — deploying Loaf into a folder that does not have it, or adding a harness that is not installed yet. Inside a Loaf repo it no-ops the project part and points at `loaf upgrade`.

If the Codex Auto-journal rule was previously enabled, upgrade refreshes it only while the installed body still matches Loaf's recorded digest. Unowned or locally modified rule files are preserved and reported as conflicts.

## Confirm project alignment

`loaf doctor` checks symlinks, stale files, and version drift across the checkout without changing anything. `loaf doctor --fix` offers each safe repair separately and defaults each y/N prompt to no; declined repairs remain reported failures. Use `loaf doctor --fix --force` only when every offered repair should be accepted without prompting, including in non-interactive automation. This catches drift that config-level checks do not.

## Record the decisions

Log the project-owned choices you settled with the user:
`loaf journal log "decision(config): ..."`.
