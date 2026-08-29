package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	applicationID = 1280267825
	schemaLine    = "vnext"
	schemaVersion = 2
)

const schemaTableV1SQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 1),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const schemaTableSQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 2),
  schema_checksum TEXT NOT NULL CHECK (
    length(schema_checksum) = 64
    AND schema_checksum NOT GLOB '*[^0-9a-f]*'
  )
) STRICT`

const factsTableSQL = `CREATE TABLE continuity_facts (
  fact_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  subject_kind TEXT NOT NULL CHECK (subject_kind IN (
    'project-identity',
    'journal-entry',
    'wrap',
    'spark',
    'idea',
    'decision',
    'exploration',
    'checkpoint',
    'finding',
    'handoff',
    'scratchpad',
    'external-reference',
    'verification-evidence'
  )),
  subject_id TEXT NOT NULL CHECK (
    length(subject_id) BETWEEN 1 AND 128
    AND subject_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  fact_kind TEXT NOT NULL CHECK (
    (subject_kind = 'project-identity' AND fact_kind IN (
      'project.registered', 'project.label-revised'
    ))
    OR (subject_kind = 'journal-entry' AND fact_kind IN (
      'journal.recorded', 'journal.correction-recorded'
    ))
    OR (subject_kind = 'wrap' AND fact_kind = 'wrap.recorded')
    OR (subject_kind = 'spark' AND fact_kind IN (
      'spark.captured', 'spark.dismissed', 'spark.promoted-to-idea'
    ))
    OR (subject_kind = 'idea' AND fact_kind IN (
      'idea.created', 'idea.revised', 'idea.resolved', 'idea.archived',
      'idea.promoted-to-external-reference'
    ))
    OR (subject_kind = 'decision' AND fact_kind IN (
      'decision.opened', 'decision.resolved', 'decision.superseded'
    ))
    OR (subject_kind = 'exploration' AND fact_kind = 'exploration.started')
    OR (subject_kind = 'checkpoint' AND fact_kind = 'checkpoint.recorded')
    OR (subject_kind = 'finding' AND fact_kind IN (
      'finding.recorded', 'finding.corrected', 'finding.retracted'
    ))
    OR (subject_kind = 'handoff' AND fact_kind = 'handoff.recorded')
    OR (subject_kind = 'scratchpad' AND fact_kind IN (
      'scratchpad.opened', 'scratchpad.participant-introduced',
      'scratchpad.message-recorded', 'scratchpad.claim-recorded',
      'scratchpad.claim-released', 'scratchpad.closed'
    ))
    OR (subject_kind = 'external-reference' AND fact_kind IN (
      'external-reference.registered', 'external-reference.attached',
      'external-reference.detached'
    ))
    OR (subject_kind = 'verification-evidence'
      AND fact_kind = 'verification-evidence.recorded')
  ),
  payload_version INTEGER NOT NULL CHECK (payload_version = 1),
  content_json TEXT NOT NULL CHECK (
    json_valid(content_json)
    AND json_type(content_json) = 'object'
    AND length(CAST(content_json AS BLOB)) BETWEEN 2 AND 1048576
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  hlc_wall_millis INTEGER NOT NULL CHECK (hlc_wall_millis >= 0),
  hlc_logical INTEGER NOT NULL CHECK (hlc_logical BETWEEN 0 AND 2147483647),
  envelope_version INTEGER NOT NULL CHECK (envelope_version = 1),
  UNIQUE (project_id, environment_id, environment_sequence),
  CHECK (subject_kind <> 'project-identity' OR subject_id = project_id)
) STRICT, WITHOUT ROWID`

const projectIdentityIndexSQL = `CREATE UNIQUE INDEX ux_continuity_project_identity
ON continuity_facts(project_id)
WHERE fact_kind = 'project.registered'`

const projectOrderIndexSQL = `CREATE INDEX ix_continuity_facts_project_order
ON continuity_facts(
  project_id,
  hlc_wall_millis,
  hlc_logical,
  environment_id,
  fact_id
)`

const subjectOrderIndexSQL = `CREATE INDEX ix_continuity_facts_subject_order
ON continuity_facts(
  project_id,
  subject_kind,
  subject_id,
  hlc_wall_millis,
  hlc_logical,
  environment_id,
  fact_id
)`

const projectIdentityTriggerSQL = `CREATE TRIGGER continuity_facts_require_project_identity
BEFORE INSERT ON continuity_facts
WHEN NEW.fact_kind <> 'project.registered'
 AND NOT EXISTS (
   SELECT 1
   FROM continuity_facts
   WHERE project_id = NEW.project_id
     AND fact_kind = 'project.registered'
 )
BEGIN
  SELECT RAISE(ABORT, 'project identity fact required');
END`

const syncProjectsTableSQL = `CREATE TABLE continuity_sync_projects (
  project_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  channel_id BLOB NOT NULL CHECK (
    length(channel_id) = 32 AND channel_id <> zeroblob(32)
  ),
  relay_generation BLOB NOT NULL CHECK (
    length(relay_generation) = 32 AND relay_generation <> zeroblob(32)
  ),
  admin_public_key BLOB NOT NULL CHECK (
    length(admin_public_key) = 32 AND admin_public_key <> zeroblob(32)
  ),
  membership_generation INTEGER NOT NULL CHECK (
    membership_generation BETWEEN 1 AND 4294967295
  ),
  activation_state TEXT NOT NULL CHECK (
    activation_state IN ('staging', 'attached')
  ),
  downloaded_cursor INTEGER NOT NULL CHECK (downloaded_cursor >= 0),
  applied_cursor INTEGER NOT NULL CHECK (applied_cursor >= 0),
  relay_head INTEGER NOT NULL CHECK (relay_head >= 0),
  CHECK (applied_cursor <= downloaded_cursor),
  CHECK (downloaded_cursor <= relay_head)
) STRICT, WITHOUT ROWID`

const syncInboxTableSQL = `CREATE TABLE continuity_sync_inbox (
  project_id TEXT NOT NULL,
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  sealed_envelope BLOB NOT NULL CHECK (
    length(sealed_envelope) BETWEEN 1 AND 1102000
  ),
  state TEXT NOT NULL CHECK (state IN ('staged', 'quarantined')),
  PRIMARY KEY (project_id, arrival_sequence),
  UNIQUE (project_id, envelope_digest),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncReceiptsTableSQL = `CREATE TABLE continuity_sync_receipts (
  project_id TEXT NOT NULL,
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  fact_id TEXT NOT NULL CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  PRIMARY KEY (project_id, arrival_sequence),
  UNIQUE (project_id, fact_id),
  UNIQUE (project_id, environment_id, environment_sequence),
  UNIQUE (project_id, key_generation, nonce),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncEnvironmentHeadsTableSQL = `CREATE TABLE continuity_sync_environment_heads (
  project_id TEXT NOT NULL CHECK (
    length(project_id) BETWEEN 1 AND 128
    AND project_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  highest_sequence INTEGER NOT NULL CHECK (highest_sequence > 0),
  hlc_wall_millis INTEGER NOT NULL CHECK (hlc_wall_millis >= 0),
  hlc_logical INTEGER NOT NULL CHECK (hlc_logical BETWEEN 0 AND 2147483647),
  sealed_sequence INTEGER NOT NULL CHECK (
    sealed_sequence BETWEEN 0 AND highest_sequence
  ),
  previous_envelope_digest BLOB CHECK (
    previous_envelope_digest IS NULL
    OR length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB CHECK (
    envelope_digest IS NULL
    OR (length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32))
  ),
  certificate_id BLOB CHECK (
    certificate_id IS NULL
    OR (length(certificate_id) = 32 AND certificate_id <> zeroblob(32))
  ),
  key_generation INTEGER CHECK (
    key_generation IS NULL OR key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB CHECK (nonce IS NULL OR length(nonce) = 24),
  CHECK (
    (sealed_sequence = 0
      AND previous_envelope_digest IS NULL
      AND envelope_digest IS NULL
      AND certificate_id IS NULL
      AND key_generation IS NULL
      AND nonce IS NULL)
    OR
    (sealed_sequence > 0
      AND previous_envelope_digest IS NOT NULL
      AND envelope_digest IS NOT NULL
      AND certificate_id IS NOT NULL
      AND key_generation IS NOT NULL
      AND nonce IS NOT NULL)
  ),
  PRIMARY KEY (project_id, environment_id)
) STRICT, WITHOUT ROWID`

const syncOutboxTableSQL = `CREATE TABLE continuity_sync_outbox (
  fact_id TEXT NOT NULL PRIMARY KEY,
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL UNIQUE CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  sealed_envelope BLOB NOT NULL CHECK (
    length(sealed_envelope) BETWEEN 1 AND 1102000
  ),
  UNIQUE (project_id, environment_id, environment_sequence),
  UNIQUE (project_id, key_generation, nonce),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id),
  FOREIGN KEY (fact_id) REFERENCES continuity_facts(fact_id)
) STRICT, WITHOUT ROWID`

const syncTombstonesTableSQL = `CREATE TABLE continuity_sync_tombstones (
  fact_id TEXT NOT NULL PRIMARY KEY CHECK (
    length(fact_id) BETWEEN 1 AND 128
    AND fact_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  environment_sequence INTEGER NOT NULL CHECK (environment_sequence > 0),
  arrival_sequence INTEGER NOT NULL CHECK (arrival_sequence > 0),
  previous_envelope_digest BLOB NOT NULL CHECK (
    length(previous_envelope_digest) = 32
  ),
  envelope_digest BLOB NOT NULL CHECK (
    length(envelope_digest) = 32 AND envelope_digest <> zeroblob(32)
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  key_generation INTEGER NOT NULL CHECK (
    key_generation BETWEEN 1 AND 4294967295
  ),
  nonce BLOB NOT NULL CHECK (length(nonce) = 24),
  prune_certificate_id BLOB NOT NULL CHECK (
    length(prune_certificate_id) = 32
    AND prune_certificate_id <> zeroblob(32)
  ),
  UNIQUE (project_id, environment_id, environment_sequence),
  UNIQUE (project_id, arrival_sequence),
  UNIQUE (project_id, key_generation, nonce),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const syncEnvironmentCertificatesTableSQL = `CREATE TABLE continuity_sync_environment_certificates (
  project_id TEXT NOT NULL,
  environment_id TEXT NOT NULL CHECK (
    length(environment_id) BETWEEN 1 AND 128
    AND environment_id NOT GLOB '*[^A-Za-z0-9_.:-]*'
  ),
  certificate_id BLOB NOT NULL CHECK (
    length(certificate_id) = 32 AND certificate_id <> zeroblob(32)
  ),
  certificate_bytes BLOB NOT NULL CHECK (
    length(certificate_bytes) BETWEEN 1 AND 8192
  ),
  mode TEXT NOT NULL CHECK (mode IN ('trusted', 'ephemeral')),
  expires_at_millis INTEGER NOT NULL CHECK (expires_at_millis >= 0),
  join_membership_generation INTEGER NOT NULL CHECK (
    join_membership_generation BETWEEN 1 AND 4294967295
  ),
  retirement_relay_generation BLOB CHECK (
    retirement_relay_generation IS NULL
    OR (length(retirement_relay_generation) = 32 AND retirement_relay_generation <> zeroblob(32))
  ),
  retirement_membership_generation INTEGER CHECK (
    retirement_membership_generation IS NULL
    OR retirement_membership_generation BETWEEN 1 AND 4294967295
  ),
  retirement_final_environment_sequence INTEGER CHECK (
    retirement_final_environment_sequence IS NULL
    OR retirement_final_environment_sequence >= 0
  ),
  retirement_final_envelope_digest BLOB CHECK (
    retirement_final_envelope_digest IS NULL
    OR length(retirement_final_envelope_digest) = 32
  ),
  retirement_id BLOB CHECK (
    retirement_id IS NULL
    OR (length(retirement_id) = 32 AND retirement_id <> zeroblob(32))
  ),
  retirement_bytes BLOB CHECK (
    retirement_bytes IS NULL
    OR length(retirement_bytes) BETWEEN 1 AND 4096
  ),
  CHECK (
    (mode = 'trusted' AND expires_at_millis = 0)
    OR (mode = 'ephemeral' AND expires_at_millis > 0)
  ),
  CHECK (
    (retirement_membership_generation IS NULL
      AND retirement_relay_generation IS NULL
      AND retirement_final_environment_sequence IS NULL
      AND retirement_final_envelope_digest IS NULL
      AND retirement_id IS NULL
      AND retirement_bytes IS NULL)
    OR
    (retirement_membership_generation IS NOT NULL
      AND retirement_relay_generation IS NOT NULL
      AND retirement_final_environment_sequence IS NOT NULL
      AND retirement_final_envelope_digest IS NOT NULL
      AND retirement_id IS NOT NULL
      AND retirement_bytes IS NOT NULL)
  ),
  CHECK (
    retirement_membership_generation IS NULL
    OR retirement_membership_generation >= join_membership_generation
  ),
  CHECK (
    retirement_final_environment_sequence IS NULL
    OR (retirement_final_environment_sequence = 0
      AND retirement_final_envelope_digest = zeroblob(32))
    OR (retirement_final_environment_sequence > 0
      AND retirement_final_envelope_digest <> zeroblob(32))
  ),
  PRIMARY KEY (project_id, environment_id),
  UNIQUE (project_id, certificate_id),
  FOREIGN KEY (project_id) REFERENCES continuity_sync_projects(project_id)
) STRICT, WITHOUT ROWID`

const schemaV1DDL = schemaTableV1SQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n"

const syncSchemaDDL = syncProjectsTableSQL + ";\n" +
	syncInboxTableSQL + ";\n" +
	syncReceiptsTableSQL + ";\n" +
	syncEnvironmentHeadsTableSQL + ";\n" +
	syncOutboxTableSQL + ";\n" +
	syncTombstonesTableSQL + ";\n" +
	syncEnvironmentCertificatesTableSQL + ";\n"

const schemaDDL = schemaTableSQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n" +
	syncSchemaDDL

type schemaObject struct {
	kind  string
	name  string
	table string
	sql   string
}

func expectedSchemaObjects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableSQL},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", sql: syncEnvironmentCertificatesTableSQL},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", sql: syncEnvironmentHeadsTableSQL},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", sql: syncInboxTableSQL},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", sql: syncOutboxTableSQL},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", sql: syncProjectsTableSQL},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", sql: syncReceiptsTableSQL},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", sql: syncTombstonesTableSQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func expectedSchemaV1Objects() []schemaObject {
	return []schemaObject{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", sql: projectOrderIndexSQL},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", sql: subjectOrderIndexSQL},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", sql: projectIdentityIndexSQL},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", sql: factsTableSQL},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", sql: schemaTableV1SQL},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func checksumSchema() string {
	sum := sha256.Sum256([]byte(schemaDDL))
	return hex.EncodeToString(sum[:])
}

func checksumSchemaV1() string {
	sum := sha256.Sum256([]byte(schemaV1DDL))
	return hex.EncodeToString(sum[:])
}

func initializeSchemaIfEmpty(db *sql.DB) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin continuity schema transaction: %w", err)
	}
	defer tx.Rollback()

	var objectCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&objectCount); err != nil {
		return false, fmt.Errorf("inspect continuity schema inventory in transaction: %w", err)
	}
	if objectCount != 0 {
		return false, nil
	}
	var existingApplicationID, existingUserVersion int
	if err := tx.QueryRow(`PRAGMA application_id`).Scan(&existingApplicationID); err != nil {
		return false, fmt.Errorf("read empty continuity application id: %w", err)
	}
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&existingUserVersion); err != nil {
		return false, fmt.Errorf("read empty continuity schema version: %w", err)
	}
	if existingApplicationID != 0 || existingUserVersion != 0 {
		return false, fmt.Errorf("empty continuity database carries foreign schema identity")
	}

	if _, err := tx.Exec(schemaDDL); err != nil {
		return false, fmt.Errorf("create continuity schema: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		return false, fmt.Errorf("set continuity application id: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return false, fmt.Errorf("set continuity schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		schemaVersion,
		checksumSchema(),
	); err != nil {
		return false, fmt.Errorf("record continuity schema identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit continuity schema: %w", err)
	}
	return true, nil
}

func validateSchema(db *sql.DB) error {
	return validateSchemaVersion(db, schemaVersion, checksumSchema(), expectedSchemaObjects())
}

type schemaQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
}

func validateSchemaVersion(db schemaQuerier, version int, checksum string, want []schemaObject) error {
	var gotApplicationID int
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&gotApplicationID); err != nil {
		return fmt.Errorf("read continuity application id: %w", err)
	}
	if gotApplicationID != applicationID {
		return fmt.Errorf("continuity application id = %d, want %d", gotApplicationID, applicationID)
	}

	var gotVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&gotVersion); err != nil {
		return fmt.Errorf("read continuity schema version: %w", err)
	}
	if gotVersion != version {
		return fmt.Errorf("continuity schema version = %d, want %d", gotVersion, version)
	}

	var gotLine, gotChecksum string
	var gotRowVersion int
	if err := db.QueryRow(
		`SELECT schema_line, schema_version, schema_checksum FROM continuity_schema WHERE singleton = 1`,
	).Scan(&gotLine, &gotRowVersion, &gotChecksum); err != nil {
		return fmt.Errorf("read continuity schema identity: %w", err)
	}
	if gotLine != schemaLine || gotRowVersion != version || gotChecksum != checksum {
		return fmt.Errorf("continuity schema identity does not match this build")
	}

	rows, err := db.Query(`
SELECT type, name, tbl_name, sql
FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%'
ORDER BY type, name`)
	if err != nil {
		return fmt.Errorf("inspect continuity schema: %w", err)
	}
	defer rows.Close()

	index := 0
	for rows.Next() {
		var got schemaObject
		if err := rows.Scan(&got.kind, &got.name, &got.table, &got.sql); err != nil {
			return fmt.Errorf("scan continuity schema object: %w", err)
		}
		if index >= len(want) {
			return fmt.Errorf("unexpected continuity schema object %s %s", got.kind, got.name)
		}
		expected := want[index]
		if got.kind != expected.kind || got.name != expected.name || got.table != expected.table || normalizeSQL(got.sql) != normalizeSQL(expected.sql) {
			return fmt.Errorf("continuity schema object %s %s differs from this build", got.kind, got.name)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate continuity schema objects: %w", err)
	}
	if index != len(want) {
		return fmt.Errorf("continuity schema has %d recognized objects, want %d", index, len(want))
	}
	return nil
}

func migrateSchema(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read continuity schema version before migration: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 1 {
		return fmt.Errorf("continuity schema version = %d, want 1 or %d", version, schemaVersion)
	}
	if err := validateSchemaVersion(db, 1, checksumSchemaV1(), expectedSchemaV1Objects()); err != nil {
		return fmt.Errorf("validate continuity v1 before migration: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin continuity v2 migration: %w", err)
	}
	defer tx.Rollback()

	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("recheck continuity schema version in migration: %w", err)
	}
	if version == schemaVersion {
		return nil
	}
	if version != 1 {
		return fmt.Errorf("continuity schema changed during migration")
	}
	if err := validateSchemaVersion(tx, 1, checksumSchemaV1(), expectedSchemaV1Objects()); err != nil {
		return fmt.Errorf("revalidate continuity v1 in migration: %w", err)
	}
	if err := validateV1EnvironmentHistory(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE continuity_schema`); err != nil {
		return fmt.Errorf("drop continuity v1 schema identity: %w", err)
	}
	if _, err := tx.Exec(schemaTableSQL + ";\n" + syncSchemaDDL); err != nil {
		return fmt.Errorf("create continuity v2 sync schema: %w", err)
	}
	if _, err := tx.Exec(`
INSERT INTO continuity_sync_environment_heads(
  project_id,
  environment_id,
  highest_sequence,
  hlc_wall_millis,
  hlc_logical,
  sealed_sequence
)
SELECT
  fact.project_id,
  fact.environment_id,
  fact.environment_sequence,
  fact.hlc_wall_millis,
  fact.hlc_logical,
  0
FROM continuity_facts AS fact
WHERE NOT EXISTS (
  SELECT 1
  FROM continuity_facts AS later
  WHERE later.project_id = fact.project_id
    AND later.environment_id = fact.environment_id
    AND later.environment_sequence > fact.environment_sequence
)`); err != nil {
		return fmt.Errorf("seed continuity environment heads: %w", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		return fmt.Errorf("set continuity v2 schema version: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, ?, ?, ?)`,
		schemaLine,
		schemaVersion,
		checksumSchema(),
	); err != nil {
		return fmt.Errorf("record continuity v2 schema identity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit continuity v2 migration: %w", err)
	}
	return nil
}

func validateV1EnvironmentHistory(tx *sql.Tx) error {
	var invalidGroups int
	if err := tx.QueryRow(`
SELECT COUNT(*)
FROM (
  SELECT project_id, environment_id
  FROM continuity_facts
  GROUP BY project_id, environment_id
  HAVING MIN(environment_sequence) <> 1
     OR MAX(environment_sequence) <> COUNT(*)
)`).Scan(&invalidGroups); err != nil {
		return fmt.Errorf("validate continuity v1 environment sequences: %w", err)
	}
	if invalidGroups != 0 {
		return fmt.Errorf("continuity v1 contains noncontiguous environment sequences")
	}

	var invalidClocks int
	if err := tx.QueryRow(`
SELECT COUNT(*)
FROM continuity_facts AS current
JOIN continuity_facts AS next
  ON next.project_id = current.project_id
 AND next.environment_id = current.environment_id
 AND next.environment_sequence = current.environment_sequence + 1
WHERE next.hlc_wall_millis < current.hlc_wall_millis
   OR (
     next.hlc_wall_millis = current.hlc_wall_millis
     AND next.hlc_logical <= current.hlc_logical
   )`).Scan(&invalidClocks); err != nil {
		return fmt.Errorf("validate continuity v1 environment clocks: %w", err)
	}
	if invalidClocks != 0 {
		return fmt.Errorf("continuity v1 contains non-increasing environment clocks")
	}
	return nil
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
