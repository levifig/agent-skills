# Command Routing

Which command a task needs. For exact flags, run `loaf <command> --help`.

## Decision guide

| Intent | Route |
|--------|-------|
| Shape new work | `loaf change init <slug>`, then `loaf change check` |
| Start working on a task | the implement workflow (/implement); `loaf task`/`loaf spec` stay transitional until the conversion pass — see the TRANSITIONAL note in AGENTS.md |
| Continue after a restart | `loaf journal context` |
| Skills or content changed | `loaf build && loaf install --to <target>` |
| See what is in progress | `loaf task list --active` |
| Archive completed work | `loaf task archive` |
| Check knowledge freshness | `loaf kb check` |
| Validate a Change is ready | `loaf change check --require-executable` |

## JSON diagnosis surfaces

When diagnosing state, prefer the `--json` surface and parse it rather than
scraping human-readable text:

- `loaf config check --json` — config file and installed hook config validity
- `loaf state doctor --json` / `loaf state status --json` — SQLite health and readiness
- `loaf change check --json` — Change violations and derived executability
- `loaf check --hook <id> --json` — one enforcement hook's result
- `loaf kb check --json` — knowledge staleness against git history
- `loaf task list --json` / `loaf journal recent --json` — current work and timeline

Choosing between the `doctor` commands and `LOAF_DB` isolation are covered in
the troubleshooting reference, linked from SKILL.md.
