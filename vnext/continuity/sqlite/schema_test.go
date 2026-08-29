package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestContinuitySQLiteSchemaRejectsInvalidFacts(t *testing.T) {
	t.Parallel()

	store, err := Open(testTempDir(t), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	testsBeforeIdentity := []struct {
		name        string
		factID      string
		projectID   string
		subjectKind string
		subjectID   string
		factKind    string
		content     string
		sequence    int
	}{
		{name: "missing identity", factID: "fact-missing", projectID: "project-missing", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{}`, sequence: 1},
		{name: "unknown subject", factID: "fact-subject", projectID: "project-a", subjectKind: "derived-context", subjectID: "context-1", factKind: "journal.recorded", content: `{}`, sequence: 1},
		{name: "unknown fact", factID: "fact-kind", projectID: "project-a", subjectKind: "project-identity", subjectID: "project-a", factKind: "project.unknown", content: `{}`, sequence: 1},
		{name: "mismatched project identity", factID: "fact-mismatch", projectID: "project-a", subjectKind: "project-identity", subjectID: "project-b", factKind: "project.registered", content: `{}`, sequence: 1},
	}
	for _, test := range testsBeforeIdentity {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := rawInsertFact(store, test.factID, test.projectID, test.subjectKind, test.subjectID, test.factKind, test.content, test.sequence); err == nil {
				t.Fatal("insert error = nil, want schema refusal")
			}
		})
	}

	if err := rawInsertFact(store, "fact-project", "project-a", "project-identity", "project-a", "project.registered", `{}`, 1); err != nil {
		t.Fatalf("insert valid project identity: %v", err)
	}

	testsAfterIdentity := []struct {
		name        string
		factID      string
		subjectKind string
		subjectID   string
		factKind    string
		content     string
		sequence    int
	}{
		{name: "duplicate identity", factID: "fact-project-2", subjectKind: "project-identity", subjectID: "project-a", factKind: "project.registered", content: `{}`, sequence: 2},
		{name: "kind mismatch", factID: "fact-mismatch-2", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "wrap.recorded", content: `{}`, sequence: 2},
		{name: "invalid json", factID: "fact-json", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{`, sequence: 2},
		{name: "non-object json", factID: "fact-array", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `[]`, sequence: 2},
		{name: "oversized json", factID: "fact-large", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{"value":"` + strings.Repeat("x", 1048576) + `"}`, sequence: 2},
		{name: "duplicate environment sequence", factID: "fact-sequence", subjectKind: "journal-entry", subjectID: "journal-1", factKind: "journal.recorded", content: `{}`, sequence: 1},
	}
	for _, test := range testsAfterIdentity {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if err := rawInsertFact(store, test.factID, "project-a", test.subjectKind, test.subjectID, test.factKind, test.content, test.sequence); err == nil {
				t.Fatal("insert error = nil, want schema refusal")
			}
		})
	}
}

func rawInsertFact(store *Store, factID, projectID, subjectKind, subjectID, factKind, content string, sequence int) error {
	_, err := store.db.Exec(`
INSERT INTO continuity_facts(
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
) VALUES(?, ?, ?, ?, ?, 1, ?, 'environment-a', ?, 1, 0, 1)`,
		factID,
		projectID,
		subjectKind,
		subjectID,
		factKind,
		content,
		sequence,
	)
	return err
}

func TestContinuitySQLiteMigratesExactV1SchemaWithoutChangingFacts(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV1ContinuityDatabase(t, stateRoot)

	store, err := Open(stateRoot, "environment-b")
	if err != nil {
		t.Fatalf("Open(v1) error = %v", err)
	}
	defer store.Close()

	var userVersion int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read migrated user version: %v", err)
	}
	if userVersion != 2 {
		t.Fatalf("user version = %d, want 2", userVersion)
	}

	fact, err := store.ExportFact(context.Background(), "fact-project")
	if err != nil {
		t.Fatalf("ExportFact() error = %v", err)
	}
	if fact.ProjectID != "project-v1" || fact.EnvironmentID != "environment-a" ||
		fact.EnvironmentSequence != 1 || fact.HLCWallMillis != 100 || fact.HLCLogical != 2 ||
		string(fact.CanonicalPayload) != v1ProjectPayload {
		t.Fatalf("migrated fact = %#v, payload %s", fact, fact.CanonicalPayload)
	}

	var headSequence, headWall, headLogical int64
	if err := store.db.QueryRow(`
SELECT highest_sequence, hlc_wall_millis, hlc_logical
FROM continuity_sync_environment_heads
WHERE project_id = 'project-v1' AND environment_id = 'environment-a'
	`).Scan(&headSequence, &headWall, &headLogical); err != nil {
		t.Fatalf("read migrated environment head: %v", err)
	}
	if headSequence != 1 || headWall != 100 || headLogical != 2 {
		t.Fatalf("migrated head = (%d,%d,%d), want (1,100,2)", headSequence, headWall, headLogical)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reopened, err := Open(stateRoot, "environment-c")
	if err != nil {
		t.Fatalf("reopen migrated database error = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened database: %v", err)
	}
	assertPathMode(t, databasePath, 0o600)
}

func TestContinuitySQLiteRefusesDriftedV1BeforeMigration(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV1ContinuityDatabase(t, stateRoot)
	db, err := sql.Open(sqliteDriverName, databaseDSN(databasePath))
	if err != nil {
		t.Fatalf("open raw v1 database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE intruder (value TEXT)`); err != nil {
		db.Close()
		t.Fatalf("create v1 drift: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close drifted v1 database: %v", err)
	}

	if store, err := Open(stateRoot, "environment-b"); err == nil {
		store.Close()
		t.Fatal("Open(drifted v1) error = nil, want refusal")
	}
	db, err = sql.Open(sqliteDriverName, databaseDSN(databasePath))
	if err != nil {
		t.Fatalf("reopen drifted v1 database: %v", err)
	}
	defer db.Close()
	var userVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read refused v1 version: %v", err)
	}
	if userVersion != 1 {
		t.Fatalf("refused v1 user version = %d, want unchanged 1", userVersion)
	}
}

func TestContinuitySQLiteV2SyncSchemaIsExactAndCredentialFree(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	rows, err := store.db.Query(`
SELECT name
FROM sqlite_schema
WHERE type = 'table' AND name LIKE 'continuity_sync_%'
ORDER BY name`)
	if err != nil {
		t.Fatalf("list sync tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			t.Fatalf("scan sync table: %v", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close sync tables: %v", err)
	}
	wantTables := []string{
		"continuity_sync_environment_certificates",
		"continuity_sync_environment_heads",
		"continuity_sync_inbox",
		"continuity_sync_outbox",
		"continuity_sync_projects",
		"continuity_sync_receipts",
		"continuity_sync_tombstones",
	}
	if strings.Join(tables, ",") != strings.Join(wantTables, ",") {
		t.Fatalf("sync tables = %v, want %v", tables, wantTables)
	}

	banned := []string{
		"credential",
		"token",
		"secret",
		"tracker",
		"provider",
		"subject_kind",
		"subject_id",
		"fact_kind",
		"payload",
		"content_json",
		"observation",
	}
	var columns []string
	for _, table := range tables {
		columnRows, err := store.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatalf("inspect %s columns: %v", table, err)
		}
		for columnRows.Next() {
			var column string
			if err := columnRows.Scan(&column); err != nil {
				columnRows.Close()
				t.Fatalf("scan %s column: %v", table, err)
			}
			columns = append(columns, table+"."+column)
			for _, fragment := range banned {
				if strings.Contains(column, fragment) {
					columnRows.Close()
					t.Fatalf("sync column %s.%s contains forbidden semantic or credential fragment %q", table, column, fragment)
				}
			}
		}
		if err := columnRows.Close(); err != nil {
			t.Fatalf("close %s columns: %v", table, err)
		}
	}
	sort.Strings(columns)
	if len(columns) == 0 {
		t.Fatal("sync schema exposed no inspected columns")
	}
}

const v1ProjectPayload = `{"observation":{"observed_at_millis":1,"harness_session_id":"","branch":"","worktree":""},"label":"Loaf"}`

func createV1ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v1 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v1 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v1 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v1 database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin v1 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV1DDL); err != nil {
		t.Fatalf("create v1 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		t.Fatalf("set v1 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatalf("set v1 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 1, ?)`,
		checksumSchemaV1(),
	); err != nil {
		t.Fatalf("record v1 schema identity: %v", err)
	}
	if _, err := tx.Exec(`
INSERT INTO continuity_facts(
  fact_id, project_id, subject_kind, subject_id, fact_kind, payload_version,
  content_json, environment_id, environment_sequence, hlc_wall_millis,
  hlc_logical, envelope_version
) VALUES(
  'fact-project', 'project-v1', 'project-identity', 'project-v1',
  'project.registered', 1, ?, 'environment-a', 1, 100, 2, 1
)`, v1ProjectPayload); err != nil {
		t.Fatalf("insert v1 fact: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v1 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v1 database: %v", err)
	}
	return databasePath
}
