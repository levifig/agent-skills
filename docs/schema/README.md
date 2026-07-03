# Loaf Operational State Schema

This packet mirrors the Go-owned SQLite schema used by the native state runtime.

Files:

- `0001_initial.sql`: exact mirror of `internal/state/migrations/0001_initial.sql`
- `0002_session_state_snapshots.sql`: exact mirror of `internal/state/migrations/0002_session_state_snapshots.sql`
- `0003_project_identity_and_relationship_origin.sql`: exact mirror of `internal/state/migrations/0003_project_identity_and_relationship_origin.sql`
- `0004_project_path_current_uniqueness.sql`: exact mirror of `internal/state/migrations/0004_project_path_current_uniqueness.sql`
- `0005_artifact_bodies_and_search.sql`: exact mirror of `internal/state/migrations/0005_artifact_bodies_and_search.sql`
- `0006_journal_search.sql`: exact mirror of `internal/state/migrations/0006_journal_search.sql`
- `0007_findings_verdicts_runs.sql`: exact mirror of `internal/state/migrations/0007_findings_verdicts_runs.sql`
- `0008_docs_index.sql`: exact mirror of `internal/state/migrations/0008_docs_index.sql`
- `0009_spec_branch_and_source.sql`: exact mirror of `internal/state/migrations/0009_spec_branch_and_source.sql`
- `operational-state.dbml`: editable relational model for design review
- `operational-state.mmd`: Mermaid ER diagram for quick visual inspection

`TestSchemaDocumentationMirrorsExecutableMigration` keeps the `0001`–`0009` SQL mirror exact and checks that the DBML and Mermaid views include every executable table, column, and Mermaid relationship label. It validates against the **auto-applied baseline** (`SchemaMigrations()`, migrations `0001`–`0009`); the `operational-state.*` views therefore model that baseline, with the journal-first end state documented as an annotated delta in each file's header.

Migration `0010_journal_first.sql` is the explicit, out-of-band journal-first (SPEC-056) transformation — the source of truth lives at `internal/state/migrations/0010_journal_first.sql`. It is deliberately excluded from `SchemaMigrations()` so it never auto-applies on store open; it runs only via `loaf state migrate journal-first --apply` (which backs up the DB file first), and is therefore not covered by the baseline mirror test. When changing the schema, update the Go migration and this packet in the same change.
