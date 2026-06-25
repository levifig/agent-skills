# Loaf Operational State Schema

This packet mirrors the Go-owned SQLite schema used by the native state runtime.

Files:

- `0001_initial.sql`: exact mirror of `internal/state/migrations/0001_initial.sql`
- `0002_session_state_snapshots.sql`: exact mirror of `internal/state/migrations/0002_session_state_snapshots.sql`
- `0003_project_identity_and_relationship_origin.sql`: exact mirror of `internal/state/migrations/0003_project_identity_and_relationship_origin.sql`
- `0004_project_path_current_uniqueness.sql`: exact mirror of `internal/state/migrations/0004_project_path_current_uniqueness.sql`
- `0005_artifact_bodies_and_search.sql`: exact mirror of `internal/state/migrations/0005_artifact_bodies_and_search.sql`
- `operational-state.dbml`: editable relational model for design review
- `operational-state.mmd`: Mermaid ER diagram for quick visual inspection

`TestSchemaDocumentationMirrorsExecutableMigration` keeps the SQL mirror exact and checks that the DBML and Mermaid views include every executable table, column, and Mermaid relationship label. When changing the schema, update the Go migration and this packet in the same change.
