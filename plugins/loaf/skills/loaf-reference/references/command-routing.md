# Command Routing

Which command a task needs. For exact flags, run `loaf <command> --help`.

## Decision guide

| Intent | Route |
|--------|-------|
| Shape new bounded work | `loaf issue new --ref linear:<KEY>|branch:<name>|pr:<N> <title>`, then `loaf issue dod add <ref>` and `loaf issue check <ref>` |
| Start implementing new bounded work | the implement workflow: pick a `linear:`/`branch:`/`pr:` ref from `loaf issue frontier`, then `loaf issue start` on a `branch:` workspace ref |
| Continue an existing task or spec record | `loaf task` and `loaf spec` remain readable for legacy records; new work is issues |
| Continue after a restart | `loaf journal context` |
| Skills or content changed | `loaf build && loaf install --to <target>` |
| See what is in progress | `loaf issue list --status active` and `loaf issue list --started` |
| Remove finished-with work | `loaf issue status <ref> cancelled` or `duplicate --duplicate-of <ref>` (archives; record survives) |
| Check knowledge freshness | `loaf kb check` |
| Validate an issue is shaped, covered, and contained | `loaf issue check <ref>` (non-zero exit names each failure) |
| Import legacy `.agents` Markdown into SQLite | `loaf migrate markdown --dry-run` then `--apply` (see markdown-migration reference) |

## JSON diagnosis surfaces

When diagnosing state, prefer the `--json` surface and parse it rather than
scraping human-readable text:

- `loaf config check --json` — config file and installed hook config validity
- `loaf state doctor --json` / `loaf state status --json` — SQLite health and readiness
- `loaf issue check <ref> --json` — derived readiness, coverage, and containment
- `loaf check --hook <id> --json` — one enforcement hook's result
- `loaf kb check --json` — knowledge staleness against git history
- `loaf issue list --json` / `loaf journal recent --json` — current work and timeline
- `loaf migrate markdown --dry-run --json` — `mode` (`simulation`/`inventory`) plus `import_report` when simulated

Choosing between the `doctor` commands and `LOAF_DB` isolation are covered in
the troubleshooting reference, linked from SKILL.md. Markdown import authority
(`mode`, `import_report`, origin reclaim, insert-only status) lives in the
markdown-migration reference.
