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

//go:embed migrations/0010_journal_first.sql
var journalFirstSQL string

//go:embed migrations/0011_journal_origins_and_deferrals.sql
var journalOriginsAndDeferralsSQL string

//go:embed migrations/0012_intents_and_explorations.sql
var intentsAndExplorationsSQL string

//go:embed migrations/0013_hook_enablement.sql
var hookEnablementSQL string

//go:embed migrations/0014_issues_and_identity.sql
var issuesAndIdentitySQL string

//go:embed migrations/0015_issue_criterion_claims.sql
var issueCriterionClaimsSQL string

//go:embed migrations/0016_releases.sql
var releasesSQL string

//go:embed migrations/0017_issue_started_workspace.sql
var issueStartedWorkspaceSQL string

//go:embed migrations/0018_drop_findings_verdicts_runs.sql
var dropFindingsVerdictsRunsSQL string

//go:embed migrations/0019_work_contracts.sql
var workContractsSQL string

//go:embed migrations/0020_facts_and_journal_envelope.sql
var factsAndJournalEnvelopeSQL string

//go:embed migrations/0021_drop_document_layer.sql
var dropDocumentLayerSQL string

//go:embed migrations/0022_prune_fossil_relationship_edges.sql
var pruneFossilRelationshipEdgesSQL string

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
		{
			Version: 11,
			Name:    "journal_origins_and_deferrals",
			SQL:     normalizeMigrationSQL(journalOriginsAndDeferralsSQL),
		},
		{
			Version: 12,
			Name:    "intents_and_explorations",
			SQL:     normalizeMigrationSQL(intentsAndExplorationsSQL),
		},
		{
			Version: 13,
			Name:    "hook_enablement",
			SQL:     normalizeMigrationSQL(hookEnablementSQL),
		},
		{
			Version: 14,
			Name:    "issues_and_identity",
			SQL:     normalizeMigrationSQL(issuesAndIdentitySQL),
		},
		{
			Version: 15,
			Name:    "issue_criterion_claims",
			SQL:     normalizeMigrationSQL(issueCriterionClaimsSQL),
		},
		{
			Version: 16,
			Name:    "releases",
			SQL:     normalizeMigrationSQL(releasesSQL),
		},
		{
			Version: 17,
			Name:    "issue_started_workspace",
			SQL:     normalizeMigrationSQL(issueStartedWorkspaceSQL),
		},
		{
			Version: 18,
			Name:    "drop_findings_verdicts_runs",
			SQL:     normalizeMigrationSQL(dropFindingsVerdictsRunsSQL),
		},
		{
			Version: 19,
			Name:    "work_contracts",
			SQL:     normalizeMigrationSQL(workContractsSQL),
		},
		{
			Version: 20,
			Name:    "facts_and_journal_envelope",
			SQL:     normalizeMigrationSQL(factsAndJournalEnvelopeSQL),
		},
		{
			Version: 21,
			Name:    "drop_document_layer",
			SQL:     normalizeMigrationSQL(dropDocumentLayerSQL),
		},
		{
			Version: 22,
			Name:    "prune_fossil_relationship_edges",
			SQL:     normalizeMigrationSQL(pruneFossilRelationshipEdgesSQL),
		},
	}
}

// journalFirstMigrationVersion is the schema version introduced by the
// journal-first transformation. It is intentionally excluded from
// SchemaMigrations() so that opening a store does not auto-apply this
// destructive migration; it runs only through the explicit journal-first
// migrate command, which takes a mandatory pre-migration backup first.
const journalFirstMigrationVersion = 10

// JournalFirstMigration returns the destructive journal-first migration. It is
// applied explicitly (not on store open) and is recorded in
// schema_migrations like any other migration.
func JournalFirstMigration() SchemaMigration {
	return SchemaMigration{
		Version: journalFirstMigrationVersion,
		Name:    "journal_first",
		SQL:     normalizeMigrationSQL(journalFirstSQL),
	}
}

// CurrentSchemaVersion returns the highest auto-applied Go-owned migration
// version. The journal-first migration remains intentionally
// excluded from SchemaMigrations() so it never auto-applies on store open.
func CurrentSchemaVersion() int {
	migrations := SchemaMigrations()
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// acceptableSchemaVersion reports whether an applied schema version is a valid
// ready state. The baseline and an explicitly journal-first-transformed
// database are both acceptable; schema 9 remains a behind-schema state until
// migration 11 is applied.
func acceptableSchemaVersion(version int) bool {
	return version == CurrentSchemaVersion() || version == journalFirstMigrationVersion
}

// Checksum returns the deterministic content hash stored with applied migrations.
func (m SchemaMigration) Checksum() string {
	sum := sha256.Sum256([]byte(m.SQL))
	return hex.EncodeToString(sum[:])
}

func normalizeMigrationSQL(sql string) string {
	return strings.TrimSpace(sql) + "\n"
}
