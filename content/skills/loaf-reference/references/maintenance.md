# Config-Aware Loaf Maintenance

## Contents
- Protocol
- Fact Sources
- Planning Surfaces
- Consent Boundaries
- Managed AGENTS.md Fenced Section
- What Maintenance Never Does

This protocol serves natural-language requests to upgrade, diagnose, repair, configure, or bring a Loaf project current. It is a hidden operator layer: interpret facts, ask only for missing project-owned choices, sequence approved deterministic operations, and verify convergence. Discover exact current syntax from `loaf <command> --help` instead of memorizing flags.

## Protocol

0. **Classify the request.** A diagnose-only request stops after step 1 with facts; a plan request stops after step 2 with the mutation ledger; only an explicit repair, upgrade, or bring-current request proceeds to apply, and then only for the mutation classes the user's request actually named. Never let the protocol's shape carry a diagnosis into a mutation.
1. **Diagnose.** Start with `loaf config check --json` for project intent and installed hook health. Add `loaf version` (running executable), `loaf state status --json` and `loaf state doctor --json` (SQLite readiness, schema version, repair plan), and `loaf doctor --json` (project alignment: symlinks, stale files, fenced-content drift or tamper). All four are read-only.
2. **Plan.** For installed-target convergence, use `loaf install --upgrade --dry-run --json`: it reports intended creates, updates, retirements, preserved conflicts, deprecation actions, project-file effects, and whether explicit consent is required, without writing anything.
3. **Ask.** Only for project-owned choices the facts cannot answer (for example integration election in `.agents/loaf.json`, or consent to destructive deprecation cleanup). Machine-observed facts are never questions. Present one complete mutation ledger — every intended operation with its consent requirement — and obtain approval for the ledger as a whole before applying any part of it.
4. **Apply.** Use the existing explicit operations the ledger named: `loaf config check --fix`, `loaf install --upgrade` (with `-y` only after consent), `loaf doctor --fix --force` (the `--force` form is required for non-interactive execution; plain `--fix` prompts and silently skips repairs without a TTY — if a repair genuinely needs interactive judgment, stop and report that it requires an operator), `loaf state migrate schema --apply`, `loaf state migrate deferrals --apply`. A project-owned election is recorded by editing `.agents/loaf.json` in the checkout — a durable, reviewable change — and validating with `loaf config check --json`. Never invent a bypass.
5. **Verify.** Rerun the diagnosis surfaces and confirm they converge; report any check that still fails rather than declaring success, and never loop back into apply without a changed ledger.

## Fact Sources

| Fact | Source | Authority |
|------|--------|-----------|
| Team-owned project intent (integrations, knowledge dirs) | `.agents/loaf.json` via `loaf config check --json` | Shared config; never records machine-local install state |
| Running executable version and provenance | `loaf version` | Observed local fact; the package manager owns acquisition |
| SQLite readiness, schema version, repair plan | `loaf state status --json`, `loaf state doctor --json` | Behind-schema state returns the exact backup-first `loaf state migrate schema --apply` action |
| Project alignment (symlinks, stale files, fenced content) | `loaf doctor --json` | Read-only; repairs go through the explicit `--fix` contract |
| Installed target ownership and drift | `loaf install --upgrade --dry-run --json` | Content-addressed ownership manifests decide owned versus foreign |

## Planning Surfaces

- `loaf doctor --json` never prompts and never repairs; it carries the identical check identities and outcomes as the human output plus repair IDs for planning.
- `loaf install --upgrade --dry-run --json` is deterministic and byte-for-byte non-mutating; applying the reported plan through the existing commands must produce the predicted effects, after which diagnosis reports convergence.
- `loaf state migrate deferrals --dry-run --json` previews the legacy-deferral conversion manifest; apply is backup-first, preserves every legacy row, and is rerunnable.
- `loaf migrate markdown --dry-run --json` reports `mode: simulation` (full apply against a disposable snapshot, with `import_report`) when the project is registered, or `mode: inventory` (file counts only, no `import_report`) otherwise. See the markdown-migration reference for origin/status authority and the dry-run/apply parity precondition.

## Consent Boundaries

- Database migrations (`state migrate schema --apply`, `state migrate deferrals --apply`) are backup-first and require the operator's explicit go-ahead on real state.
- Destructive deprecation cleanup during `install --upgrade` requires explicit consent (`-y`); missing consent must surface as a reported requirement, not an assumed yes.
- Locally modified or unowned destination content is preserved and reported, never overwritten.

## Managed AGENTS.md Fenced Section

The managed `<!-- loaf:managed:start … -->` section is fingerprint-identified (`sha256=` of the body). `loaf doctor`'s `fenced-content` check compares the actual body to the constant this binary generates: tamper (stored fingerprint disagrees with the body) is reported first and must be reconciled by hand — `loaf install --upgrade` will refuse that state; content drift (intact section whose body differs from the generated constant) is remedied with `loaf install --upgrade`. A pending one-time stamp-strip transition from a legacy `v… sha256=…` header is not itself a warning.

A pre-change Loaf binary that encounters the new `sha256=`-only header treats it as a malformed fingerprint and refuses to overwrite the section. The remedy is to upgrade the binary (for example `brew upgrade loaf`), then rerun `loaf install --upgrade` with the current release — do not force the old binary through.

## What Maintenance Never Does

- Never invokes Homebrew, npm, or any package manager, and never claims a newer remote version exists without evidence from the owning package manager.
- Never hardcodes a Cellar, checkout, or binary path; `loaf` resolves on `PATH`.
- Never infers machine-local installed-target intent from Git-tracked config, and never writes machine-specific fields into `.agents/loaf.json`.
- Never applies a database migration automatically as a side effect of diagnosis.
