package state

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strings"
)

//go:embed migrations/0001_initial.sql
var initialSchemaSQL string

//go:embed migrations/0002_session_state_snapshots.sql
var sessionStateSnapshotsSQL string

//go:embed migrations/0003_project_identity_and_relationship_origin.sql
var projectIdentityAndRelationshipOriginSQL string

//go:embed migrations/0004_project_path_current_uniqueness.sql
var projectPathCurrentUniquenessSQL string

//go:embed migrations/0005_artifact_bodies_and_search.sql
var artifactBodiesAndSearchSQL string

//go:embed migrations/0006_journal_search.sql
var journalSearchSQL string

//go:embed migrations/0007_findings_verdicts_runs.sql
var findingsVerdictsRunsSQL string

//go:embed migrations/0008_docs_index.sql
var docsIndexSQL string

//go:embed migrations/0009_spec_branch_and_source.sql
var specBranchAndSourceSQL string

const schemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  checksum TEXT NOT NULL,
  applied_at TEXT NOT NULL
)`

// SchemaMigration is a Go-owned SQLite migration definition. The storage
// package applies these in version order once the SQLite driver is introduced.
type SchemaMigration struct {
	Version int
	Name    string
	SQL     string
}

// SchemaMigrations returns the ordered schema migrations for Loaf state.
func SchemaMigrations() []SchemaMigration {
	return []SchemaMigration{
		{
			Version: 1,
			Name:    "initial_operational_state",
			SQL:     normalizeMigrationSQL(initialSchemaSQL),
		},
		{
			Version: 2,
			Name:    "session_state_snapshots",
			SQL:     normalizeMigrationSQL(sessionStateSnapshotsSQL),
		},
		{
			Version: 3,
			Name:    "project_identity_and_relationship_origin",
			SQL:     normalizeMigrationSQL(projectIdentityAndRelationshipOriginSQL),
		},
		{
			Version: 4,
			Name:    "project_path_current_uniqueness",
			SQL:     normalizeMigrationSQL(projectPathCurrentUniquenessSQL),
		},
		{
			Version: 5,
			Name:    "artifact_bodies_and_search",
			SQL:     normalizeMigrationSQL(artifactBodiesAndSearchSQL),
		},
		{
			Version: 6,
			Name:    "journal_search",
			SQL:     normalizeMigrationSQL(journalSearchSQL),
		},
		{
			Version: 7,
			Name:    "findings_verdicts_runs",
			SQL:     normalizeMigrationSQL(findingsVerdictsRunsSQL),
		},
		{
			Version: 8,
			Name:    "docs_index",
			SQL:     normalizeMigrationSQL(docsIndexSQL),
		},
		{
			Version: 9,
			Name:    "spec_branch_and_source",
			SQL:     normalizeMigrationSQL(specBranchAndSourceSQL),
		},
	}
}

// CurrentSchemaVersion returns the highest Go-owned migration version.
func CurrentSchemaVersion() int {
	migrations := SchemaMigrations()
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// Checksum returns the deterministic content hash stored with applied migrations.
func (m SchemaMigration) Checksum() string {
	sum := sha256.Sum256([]byte(m.SQL))
	return hex.EncodeToString(sum[:])
}

func normalizeMigrationSQL(sql string) string {
	return strings.TrimSpace(sql) + "\n"
}
