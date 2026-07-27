# CLI Boundary

Reading `loaf change init` and `loaf change check` — the skill teaches reading the CLI, not wrapping it. For the rest of the Loaf CLI surface, see the `loaf-reference` skill.

## `loaf change init <slug> [--brief]`

Scaffolds `docs/changes/<YYYYMMDD>-<slug>/change.json + shape.md` from the Change template, where `<YYYYMMDD>` is the creation day (not a target date) and the branch is named by the bare slug — no date prefix on the branch. Fails if the folder already exists. The slug uses lowercase letters, digits, and single hyphens.

## `loaf change check [folder] [--require-executable] [--json]`

Folder resolution: an explicit `[folder]` argument always wins; otherwise the current git branch is matched against the `branch:` frontmatter across every `docs/changes/*/change.json + shape.md`. Zero or multiple matches is an error naming the candidates found.

Output splits into two tiers:

- **Violations** — always fail (exit code 2), regardless of flags: status-like frontmatter keys (`readiness`, `status`, `state`) or values matching the canonical change-state vocabulary; frontmatter not opening the file at byte one; malformed `YYYYMMDD-slug` folder naming; identity mismatch between `change:`/`created:` and the folder name; missing Product Contract sections (Problem, Hypothesis, Scope, Observable Workflow, Rabbit Holes and No-Gos).
- **Derived executability** — reported, never gating by default. A Change is executable when Planning Contract, Implementation Units, Verification Contract, and Definition of Done are all present and non-empty (bracket placeholders and HTML comments don't count as content). A Change with open gaps is incomplete and non-executable; the report just says what's still missing.

`--require-executable` turns structural executability into a gate (exit code 1 if not structurally executable); it does not prove implementation completion. This is implement's preflight and CI's non-draft-PR check, not something shape itself passes during shaping.

A branch/Change mismatch (current branch doesn't match the Change's `branch:` field) is a warning, never a violation.

`--json` emits `{command, folder, passed, executable, exitCode, findings, warnings, gaps}` for scripted reads; prefer it when diagnosing rather than scraping the human-readable text.


## `loaf change report new <slug> --kind <kind>`

Stamps `reports/YYYYMMDD-HHMMSS-<kind>-<slug>.html` with charset, provenance, and token skeleton; prints design-language guidance. Closed kinds: approval, review, visual, audit, note.

## `loaf change verify [folder]`

Runs executable V-tier criteria declared in `shape.md` and writes `receipts/verify.json` (criteria digest, verified commit, cwd, per-criterion evidence). New-layout-only.

Criteria forms (both parse):

```markdown
- **V1.** Prose. Command: `exact command`. Expect: exit 0.
- **V1.** Prose.
  - Command: `exact command`
  - Expect: exit 0
```

Commands run from the repository root; the receipt records that cwd. H-tier entries (`**H1.** …`) are never gate input. See [decomposition.md](decomposition.md) for authoring guidance.

## `loaf change tasks` / `show`

On-demand projections. See `loaf change --help`.
