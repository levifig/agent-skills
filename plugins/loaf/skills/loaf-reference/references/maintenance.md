# Config-Aware Loaf Maintenance

## Contents
- Protocol
- Install Versus Upgrade
- Fact Sources
- Planning Surfaces
- Consent Boundaries
- Managed AGENTS.md Fenced Section
- What Maintenance Never Does

This protocol serves natural-language requests to upgrade, diagnose, repair, configure, or bring a Loaf project current. It is a hidden operator layer: interpret facts, ask only for missing project-owned choices, sequence approved deterministic operations, and verify convergence. Discover exact current syntax from `loaf <command> --help` instead of memorizing flags.

## Protocol

0. **Classify the request.** A diagnose-only request stops after step 1 with facts; a plan request stops after step 2 with the mutation ledger; only an explicit repair, upgrade, or bring-current request proceeds to apply, and then only for the mutation classes the user's request actually named. Never let the protocol's shape carry a diagnosis into a mutation.
1. **Diagnose.** Start with `loaf config check --json` for project intent and installed hook health. Add `loaf version` (running executable), `loaf state status --json` and `loaf state doctor --json` (SQLite readiness, schema version, repair plan), and `loaf doctor --json` (project alignment: symlinks, stale files, fenced-content drift or tamper). All four are read-only.
2. **Plan.** For installed-target convergence, use `loaf upgrade --dry-run --json`: it reports intended creates, updates, retirements, preserved conflicts, deprecation actions, project-file effects, and whether explicit consent is required, without writing anything.
3. **Ask.** Only for project-owned choices the facts cannot answer (for example integration election in `.agents/loaf.json`, or consent to destructive deprecation cleanup). Machine-observed facts are never questions. Present one complete mutation ledger — every intended operation with its consent requirement — and obtain approval for the ledger as a whole before applying any part of it.
4. **Apply.** Use the existing explicit operations the ledger named: `loaf config check --fix`, `loaf upgrade` (with `-y` only after consent), `loaf doctor --fix --force` (the `--force` form is required for non-interactive execution; plain `--fix` prompts and silently skips repairs without a TTY — if a repair genuinely needs interactive judgment, stop and report that it requires an operator), `loaf state migrate schema --apply`, `loaf state migrate deferrals --apply`. A project-owned election is recorded by editing `.agents/loaf.json` in the checkout — a durable, reviewable change — and validating with `loaf config check --json`. Never invent a bypass.
5. **Verify.** Rerun the diagnosis surfaces and confirm they converge; report any check that still fails rather than declaring success, and never loop back into apply without a changed ledger.

## Install Versus Upgrade

Two commands, split by what they are for. Bringing an existing installation current is always `loaf upgrade`; there is no upgrade flag on install.

- `loaf upgrade` is the maintenance command and the one this protocol applies. It syncs every installed harness config dir from the installed distribution and runs deprecation cleanup wherever it is invoked, then refreshes project surfaces — fenced sections, symlinks, migrations, and the MCP recommendation in `.agents/loaf.json` — only when the Loaf-repo detector confirms the working directory is a Loaf project. A SQLite project record or a fenced `AGENTS.md` marker proceeds on its own; legacy `.agents/` folders alone ask first; nothing detected means the harness half runs and the project half is skipped with a note. `--to <target>` narrows the sync to an already-installed target and errors on an uninstalled one, pointing at `loaf install --to`. A harness that cannot be synced does not stop the rest: the run finishes every remaining target and the project part, names what failed, and exits non-zero — so a zero exit is the signal that the whole refresh landed.
- `loaf install` is onboarding only: deploying Loaf into a folder for the first time, or adding a harness that does not have it yet. Inside a Loaf repo the project half no-ops and suggests `loaf upgrade`.
- `loaf upgrade` closes with a best-effort currency advisory when it can identify the running binary's install channel (Homebrew, npm, or a dev checkout): it prints the available version and that channel's exact command, and never runs it. The check has a one-second budget and degrades silently — no advisory is not evidence that the binary is current.

## Fact Sources

| Fact | Source | Authority |
|------|--------|-----------|
| Team-owned project intent (integrations, knowledge dirs) | `.agents/loaf.json` via `loaf config check --json` | Shared config; never records machine-local install state |
| Running executable version and provenance | `loaf version` | Observed local fact; the package manager owns acquisition |
| SQLite readiness, schema version, repair plan | `loaf state status --json`, `loaf state doctor --json` | Behind-schema state returns the exact backup-first `loaf state migrate schema --apply` action |
| Project alignment (symlinks, stale files, fenced content) | `loaf doctor --json` | Read-only; repairs go through the explicit `--fix` contract |
| Installed target ownership and drift | `loaf upgrade --dry-run --json` | Content-addressed ownership manifests decide owned versus foreign |
| Harness content version against the running binary | `loaf doctor --json` (`harness-content-drift`) | Each installed harness's `.loaf-version` marker; report-only, remedied by `loaf upgrade` |

## Planning Surfaces

- `loaf doctor --json` never prompts and never repairs; it carries the identical check identities and outcomes as the human output plus repair IDs for planning.
- `loaf upgrade --dry-run --json` is deterministic and byte-for-byte non-mutating; applying the reported plan through the existing commands must produce the predicted effects, after which diagnosis reports convergence.
- `loaf state migrate deferrals --dry-run --json` previews the legacy-deferral conversion manifest; apply is backup-first, preserves every legacy row, and is rerunnable.
- `loaf migrate markdown --dry-run --json` reports `mode: simulation` (full apply against a disposable snapshot, with `import_report`) when the project is registered, or `mode: inventory` (file counts only, no `import_report`) otherwise. See the markdown-migration reference for origin/status authority and the dry-run/apply parity precondition.

The plan document stays at `contract_version` 1 now that the surface belongs to `loaf upgrade`. Every field keeps its name, type, and meaning; the only value that changed is `command`, which reads `upgrade` because that is the command that applies the plan. One optional object is new: `project_part` reports the Loaf-repo detector gate with `in_scope` (whether project surfaces are planned at all), `tier` (`authoritative`, `strong`, `legacy`, or `none`), `confirmation_required` (the legacy tier needs an explicit yes), and `bases` (the evidence the tier rests on). It is omitted entirely for callers that plan project files unconditionally, so their document is byte-identical to the pre-split one — read it when present, do not require it.

## Consent Boundaries

- Database migrations (`state migrate schema --apply`, `state migrate deferrals --apply`) are backup-first and require the operator's explicit go-ahead on real state.
- Destructive deprecation cleanup during `loaf upgrade` requires explicit consent (`-y`); missing consent must surface as a reported requirement, not an assumed yes.
- Onboarding a folder that is not yet a Loaf project is `loaf install`'s own consent gate: outside a detected Loaf repo it asks before writing `AGENTS.md`, the fenced section, symlinks, or the MCP recommendation into `.agents/loaf.json`, and a non-interactive run reports the required consent instead of assuming it. Maintenance never routes a refresh through install to sidestep that gate.
- Locally modified or unowned destination content is preserved and reported, never overwritten.

## Managed AGENTS.md Fenced Section

The managed `<!-- loaf:managed:start … -->` section is fingerprint-identified (`sha256=` of the body). `loaf doctor`'s `fenced-content` check compares the actual body to the constant this binary generates: tamper (stored fingerprint disagrees with the body) is reported first and must be reconciled by hand — `loaf upgrade` refuses that state, skips the project surfaces that would follow it rather than half-refreshing the repo, and exits non-zero; content drift (intact section whose body differs from the generated constant) is remedied with `loaf upgrade`. A pending one-time stamp-strip transition from a legacy `v… sha256=…` header is not itself a warning.

A pre-change Loaf binary that encounters the new `sha256=`-only header treats it as a malformed fingerprint and refuses to overwrite the section. The remedy is to upgrade the binary (for example `brew upgrade loaf`), then rerun `loaf upgrade` with the current release — do not force the old binary through.

## What Maintenance Never Does

- Never invokes Homebrew, npm, or any package manager, and never claims a newer remote version exists without evidence from the owning package manager.
- Never hardcodes a Cellar, checkout, or binary path; `loaf` resolves on `PATH`.
- Never infers machine-local installed-target intent from Git-tracked config, and never writes machine-specific fields into `.agents/loaf.json`.
- Never applies a database migration automatically as a side effect of diagnosis.
