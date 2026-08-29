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
	schemaVersion = 1
)

const schemaTableSQL = `CREATE TABLE continuity_schema (
  singleton INTEGER NOT NULL PRIMARY KEY CHECK (singleton = 1),
  schema_line TEXT NOT NULL CHECK (schema_line = 'vnext'),
  schema_version INTEGER NOT NULL CHECK (schema_version = 1),
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

const schemaDDL = schemaTableSQL + ";\n" +
	factsTableSQL + ";\n" +
	projectIdentityIndexSQL + ";\n" +
	projectOrderIndexSQL + ";\n" +
	subjectOrderIndexSQL + ";\n" +
	projectIdentityTriggerSQL + ";\n"

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
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", sql: projectIdentityTriggerSQL},
	}
}

func checksumSchema() string {
	sum := sha256.Sum256([]byte(schemaDDL))
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
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
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
	if gotVersion != schemaVersion {
		return fmt.Errorf("continuity schema version = %d, want %d", gotVersion, schemaVersion)
	}

	var gotLine, gotChecksum string
	var gotRowVersion int
	if err := db.QueryRow(
		`SELECT schema_line, schema_version, schema_checksum FROM continuity_schema WHERE singleton = 1`,
	).Scan(&gotLine, &gotRowVersion, &gotChecksum); err != nil {
		return fmt.Errorf("read continuity schema identity: %w", err)
	}
	if gotLine != schemaLine || gotRowVersion != schemaVersion || gotChecksum != checksumSchema() {
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

	want := expectedSchemaObjects()
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

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
