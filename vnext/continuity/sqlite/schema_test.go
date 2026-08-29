package sqlite

import (
	"bytes"
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
	if userVersion != 3 {
		t.Fatalf("user version = %d, want 3", userVersion)
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

func TestContinuitySQLiteV3SyncSchemaIsExactAndCredentialFree(t *testing.T) {
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
		"continuity_sync_terminal_candidate_frames",
		"continuity_sync_terminal_candidates",
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

func TestContinuitySQLiteSchemaDDLGoldenChecksums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ddl      string
		checksum string
		bytes    int
	}{
		{name: "v1", ddl: schemaV1DDL, checksum: "6fcc409d4d49d1f7702e57ea96c493623ef37eec4e1aae6ec888f542532f0004", bytes: 4322},
		{name: "v2", ddl: schemaV2DDL, checksum: "f7edc0566cc24ee50009d7b70389aeb4a6bb4558dcba89a56df0f0ddfc6a64ab", bytes: 14202},
		{name: "v3", ddl: schemaDDL, checksum: "d1163e6ba25279c5b332ce19d383d7709d4dc00b49928e32e8a58ccf70aaa3af", bytes: 19161},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if len(test.ddl) != test.bytes {
				t.Fatalf("DDL bytes = %d, want %d", len(test.ddl), test.bytes)
			}
			if got := checksumDDL(test.ddl); got != test.checksum {
				t.Fatalf("DDL checksum = %s, want %s", got, test.checksum)
			}
		})
	}
}

func TestContinuitySQLiteMigratesExactV2InboxToTaggedV3(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV2ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v2 database for seed: %v", err)
	}
	sealed := [][]byte{{0x00, 0x01, 0xff}, bytes.Repeat([]byte{0xa5}, 257)}
	if _, err := db.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES('project-v2', ?, ?, ?, 1, 'attached', 2, 0, 2)`,
		schemaDigestBytes(0x11), schemaDigestBytes(0x12), schemaDigestBytes(0x13)); err != nil {
		db.Close()
		t.Fatalf("seed v2 project: %v", err)
	}
	for index, value := range sealed {
		state := "staged"
		if index == 1 {
			state = "quarantined"
		}
		if _, err := db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, sealed_envelope, state
) VALUES('project-v2', ?, ?, ?, ?)`,
			index+1, schemaDigestBytes(byte(0x21+index)), value, state); err != nil {
			db.Close()
			t.Fatalf("seed v2 inbox row %d: %v", index+1, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v2 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-v3")
	if err != nil {
		t.Fatalf("Open(v2) error = %v", err)
	}
	defer store.Close()

	rows, err := store.db.Query(`
SELECT arrival_sequence, frame_kind, frame_bytes, state
FROM continuity_sync_inbox
WHERE project_id = 'project-v2'
ORDER BY arrival_sequence`)
	if err != nil {
		t.Fatalf("read migrated inbox: %v", err)
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var arrival int
		var kind, state string
		var frame []byte
		if err := rows.Scan(&arrival, &kind, &frame, &state); err != nil {
			t.Fatalf("scan migrated inbox: %v", err)
		}
		if arrival != index+1 || kind != "sealed" || !bytes.Equal(frame, sealed[index]) {
			t.Fatalf("migrated row %d = arrival %d kind %q bytes %x", index, arrival, kind, frame)
		}
		wantState := "staged"
		if index == 1 {
			wantState = "quarantined"
		}
		if state != wantState {
			t.Fatalf("migrated row %d state = %q, want %q", index, state, wantState)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated inbox: %v", err)
	}
	if index != len(sealed) {
		t.Fatalf("migrated inbox rows = %d, want %d", index, len(sealed))
	}

	var pruned, candidates, frames, legacy int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox WHERE frame_kind = 'pruned'`).Scan(&pruned); err != nil {
		t.Fatalf("count synthesized pruned rows: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidates`).Scan(&candidates); err != nil {
		t.Fatalf("count migrated candidates: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames`).Scan(&frames); err != nil {
		t.Fatalf("count migrated candidate frames: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_inbox_v2'`).Scan(&legacy); err != nil {
		t.Fatalf("inspect legacy inbox object: %v", err)
	}
	if pruned != 0 || candidates != 0 || frames != 0 || legacy != 0 {
		t.Fatalf("migration residuals: pruned=%d candidates=%d frames=%d legacy=%d", pruned, candidates, frames, legacy)
	}
}

func TestContinuitySQLiteRefusesDriftedV2BeforeMigration(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV2ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v2 database for drift: %v", err)
	}
	payload := []byte{0x00, 0x7f, 0xff}
	if _, err := db.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES('project-drift-v2', ?, ?, ?, 1, 'attached', 1, 0, 1)`,
		schemaDigestBytes(0x71), schemaDigestBytes(0x72), schemaDigestBytes(0x73)); err != nil {
		db.Close()
		t.Fatalf("seed drifted v2 project: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, sealed_envelope, state
) VALUES('project-drift-v2', 1, ?, ?, 'staged')`, schemaDigestBytes(0x74), payload); err != nil {
		db.Close()
		t.Fatalf("seed drifted v2 inbox: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE continuity_v2_intruder(value TEXT)`); err != nil {
		db.Close()
		t.Fatalf("create v2 schema drift: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close drifted v2 database: %v", err)
	}

	if store, err := Open(stateRoot, "environment-v3"); err == nil {
		store.Close()
		t.Fatal("Open(drifted v2) error = nil, want refusal")
	}
	db, err = openDatabase(databasePath)
	if err != nil {
		t.Fatalf("reopen refused v2 database: %v", err)
	}
	defer db.Close()
	var version, candidates, legacy int
	var retained []byte
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read refused v2 version: %v", err)
	}
	if err := db.QueryRow(`
SELECT sealed_envelope
FROM continuity_sync_inbox
WHERE project_id = 'project-drift-v2' AND arrival_sequence = 1`).Scan(&retained); err != nil {
		t.Fatalf("read refused v2 inbox bytes: %v", err)
	}
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM sqlite_schema
WHERE name IN (
  'continuity_sync_terminal_candidates',
  'continuity_sync_terminal_candidate_frames'
)`).Scan(&candidates); err != nil {
		t.Fatalf("inspect refused v2 candidate objects: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_inbox_v2'`).Scan(&legacy); err != nil {
		t.Fatalf("inspect refused v2 temporary inbox: %v", err)
	}
	if version != 2 || !bytes.Equal(retained, payload) || candidates != 0 || legacy != 0 {
		t.Fatalf("refused v2 mutated: version=%d bytes=%x candidates=%d legacy=%d", version, retained, candidates, legacy)
	}
}

func TestContinuitySQLiteFrozenV2ValidatorRejectsV3WithoutMutation(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	before := schemaIdentitySnapshot(t, store.db)
	if err := validateSchemaVersion(store.db, 2, checksumSchemaV2(), expectedSchemaV2Objects()); err == nil {
		t.Fatal("frozen v2 validation of v3 error = nil, want refusal")
	}
	after := schemaIdentitySnapshot(t, store.db)
	if before != after {
		t.Fatalf("frozen v2 validation mutated v3: before=%#v after=%#v", before, after)
	}
}

func TestContinuitySQLiteMigrationStepsAcceptExactConcurrentAdvance(t *testing.T) {
	t.Parallel()

	t.Run("v1 preflight observes exact v2", func(t *testing.T) {
		t.Parallel()
		stateRoot := filepath.Join(testTempDir(t), "state")
		databasePath := createV2ContinuityDatabase(t, stateRoot)
		db, err := openDatabase(databasePath)
		if err != nil {
			t.Fatalf("open exact v2 database: %v", err)
		}
		defer db.Close()
		before := schemaIdentitySnapshot(t, db)
		if err := migrateSchemaV1ToV2(db); err != nil {
			t.Fatalf("migrateSchemaV1ToV2(exact v2) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, db)
		if before != after {
			t.Fatalf("v1 migration preflight mutated exact v2: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v2 preflight observes exact v3", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV2ToV3(store.db); err != nil {
			t.Fatalf("migrateSchemaV2ToV3(exact v3) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v2 migration preflight mutated exact v3: before=%#v after=%#v", before, after)
		}
	})
}

func TestContinuitySQLiteV3TerminalCandidateConstraints(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES('project-candidate', ?, ?, ?, 1, 'attached', 2, 0, 2)`,
		schemaDigestBytes(0x31), schemaDigestBytes(0x32), schemaDigestBytes(0x33)); err != nil {
		t.Fatalf("seed candidate project: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES
  ('project-candidate', 1, ?, 'sealed', X'01', 'staged'),
  ('project-candidate', 2, ?, 'pruned', X'02', 'staged')`,
		schemaDigestBytes(0x41), schemaDigestBytes(0x42)); err != nil {
		t.Fatalf("seed candidate inbox: %v", err)
	}

	candidateID := schemaDigestBytes(0x51)
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest
) VALUES('project-candidate', ?, 'staging', ?, ?, 1, ?, 1, 2, 2, ?)`,
		candidateID, schemaDigestBytes(0x31), schemaDigestBytes(0x32),
		schemaDigestBytes(0x52), schemaDigestBytes(0x53)); err != nil {
		t.Fatalf("insert staging candidate: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest
) VALUES('project-candidate', ?, 'staging', ?, ?, 1, ?, 1, 1, 1, ?)`,
		schemaDigestBytes(0x54), schemaDigestBytes(0x31), schemaDigestBytes(0x32),
		schemaDigestBytes(0x52), schemaDigestBytes(0x55)); err == nil {
		t.Fatal("second staging candidate error = nil, want partial-index refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest
) VALUES('project-candidate', ?, 'promoted', ?, ?, 1, ?, 1, 1, 1, ?)`,
		schemaDigestBytes(0x56), schemaDigestBytes(0x31), schemaDigestBytes(0x32),
		schemaDigestBytes(0x52), schemaDigestBytes(0x57)); err == nil {
		t.Fatal("promoted candidate without outcome error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES('project-candidate', ?, 'promoted', ?, ?, 1, ?, 1, 1, 1, ?, ?, 1)`,
		schemaDigestBytes(0x56), schemaDigestBytes(0x31), schemaDigestBytes(0x32),
		schemaDigestBytes(0x52), schemaDigestBytes(0x57), schemaDigestBytes(0x58)); err != nil {
		t.Fatalf("insert promoted candidate receipt: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES('project-candidate', ?, 'promoted', ?, ?, 1, ?, 2, 2, 2, ?, ?, 2)`,
		schemaDigestBytes(0x59), schemaDigestBytes(0x31), schemaDigestBytes(0x32),
		schemaDigestBytes(0x52), schemaDigestBytes(0x5a), schemaDigestBytes(0x5b)); err == nil {
		t.Fatal("candidate with mismatched range count error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES('project-candidate', ?, 'promoted', ?, ?, 1, ?, 2, 2, 1, ?, ?, 2)`,
		schemaDigestBytes(0x59), schemaDigestBytes(0x31), schemaDigestBytes(0x32),
		schemaDigestBytes(0x52), schemaDigestBytes(0x5a), schemaDigestBytes(0x5b)); err != nil {
		t.Fatalf("insert second promoted candidate receipt: %v", err)
	}
	var stagingHeaders, promotedHeaders int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidates WHERE state = 'staging'`).Scan(&stagingHeaders); err != nil {
		t.Fatalf("count staging candidates: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidates WHERE state = 'promoted'`).Scan(&promotedHeaders); err != nil {
		t.Fatalf("count promoted candidates: %v", err)
	}
	if stagingHeaders != 1 || promotedHeaders != 2 {
		t.Fatalf("candidate headers staging=%d promoted=%d, want 1 and 2", stagingHeaders, promotedHeaders)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 1, 'sealed', 'fact-orphan', 'environment-orphan',
  1, 100, 0, zeroblob(32), ?, ?, 1, ?, X'7B7D')`,
		schemaDigestBytes(0x5c), schemaDigestBytes(0x41), schemaDigestBytes(0x61),
		bytes.Repeat([]byte{0x02}, 24)); err == nil {
		t.Fatal("candidate frame without header error = nil, want FK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 99, 'sealed', 'fact-no-inbox', 'environment-orphan',
  1, 100, 0, zeroblob(32), ?, ?, 1, ?, X'7B7D')`,
		candidateID, schemaDigestBytes(0x5d), schemaDigestBytes(0x61),
		bytes.Repeat([]byte{0x03}, 24)); err == nil {
		t.Fatal("candidate frame without inbox row error = nil, want FK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 1, 'sealed', 'fact-1', 'environment-source',
  1, 100, 0, zeroblob(32), ?, ?, 1, zeroblob(24), X'01')`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x61)); err == nil {
		t.Fatal("undersized sealed candidate bytes error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, prune_certificate_id, candidate_bytes
) VALUES('project-candidate', ?, 2, 'pruned', 'fact-2', 'environment-source',
  2, 101, 0, ?, ?, ?, 1, ?, ?, zeroblob(257))`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x42), schemaDigestBytes(0x62),
		bytes.Repeat([]byte{0x01}, 24), schemaDigestBytes(0x63)); err == nil {
		t.Fatal("oversized pruned candidate bytes error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, prune_certificate_id, candidate_bytes
) VALUES('project-candidate', ?, 1, 'sealed', 'fact-1', 'environment-source',
  1, 100, 0, zeroblob(32), ?, ?, 1, zeroblob(24), ?, X'7B7D')`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x61), schemaDigestBytes(0x63)); err == nil {
		t.Fatal("sealed candidate with prune certificate error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 1, 'sealed', 'fact-1', 'environment-source',
  1, 100, 0, zeroblob(32), ?, ?, 1, zeroblob(24), X'7B7D')`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x61)); err != nil {
		t.Fatalf("insert sealed candidate frame: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 2, 'pruned', 'fact-2', 'environment-source',
  2, 101, 0, ?, ?, ?, 1, ?, X'01')`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x42), schemaDigestBytes(0x62),
		bytes.Repeat([]byte{0x01}, 24)); err == nil {
		t.Fatal("pruned candidate without prune certificate error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, prune_certificate_id, candidate_bytes
) VALUES('project-candidate', ?, 2, 'pruned', 'fact-2', 'environment-source',
  2, 101, 0, ?, ?, ?, 1, ?, ?, X'01')`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x42), schemaDigestBytes(0x62),
		bytes.Repeat([]byte{0x01}, 24), schemaDigestBytes(0x63)); err != nil {
		t.Fatalf("insert pruned candidate frame: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_inbox(
  project_id, arrival_sequence, envelope_digest, frame_kind, frame_bytes, state
) VALUES
  ('project-candidate', 3, ?, 'sealed', X'03', 'staged'),
  ('project-candidate', 4, ?, 'sealed', X'04', 'staged')`,
		schemaDigestBytes(0x43), schemaDigestBytes(0x44)); err != nil {
		t.Fatalf("seed uniqueness-test inbox rows: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 3, 'sealed', 'fact-3', 'environment-source',
  2, 102, 0, ?, ?, ?, 1, ?, X'7B7D')`,
		candidateID, schemaDigestBytes(0x41), schemaDigestBytes(0x43), schemaDigestBytes(0x64),
		bytes.Repeat([]byte{0x03}, 24)); err == nil {
		t.Fatal("duplicate candidate source sequence error = nil, want UNIQUE refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_terminal_candidate_frames(
  project_id, candidate_id, arrival_sequence, frame_kind, fact_id,
  environment_id, environment_sequence, hlc_wall_millis, hlc_logical,
  previous_envelope_digest, envelope_digest, certificate_id,
  key_generation, nonce, candidate_bytes
) VALUES('project-candidate', ?, 4, 'sealed', 'fact-4', 'environment-other',
  1, 103, 0, zeroblob(32), ?, ?, 1, zeroblob(24), X'7B7D')`,
		candidateID, schemaDigestBytes(0x44), schemaDigestBytes(0x65)); err == nil {
		t.Fatal("duplicate candidate generation nonce error = nil, want UNIQUE refusal")
	}
	foreignKeyRows, err := store.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("run candidate foreign-key check: %v", err)
	}
	if foreignKeyRows.Next() {
		foreignKeyRows.Close()
		t.Fatal("foreign_key_check returned a candidate schema violation")
	}
	if err := foreignKeyRows.Err(); err != nil {
		foreignKeyRows.Close()
		t.Fatalf("iterate candidate foreign-key check: %v", err)
	}
	if err := foreignKeyRows.Close(); err != nil {
		t.Fatalf("close candidate foreign-key check: %v", err)
	}
	if _, err := store.db.Exec(`
DELETE FROM continuity_sync_inbox
WHERE project_id = 'project-candidate' AND arrival_sequence = 1`); err == nil {
		t.Fatal("delete candidate-referenced inbox row error = nil, want FK refusal")
	}
	if _, err := store.db.Exec(`
DELETE FROM continuity_sync_terminal_candidates
WHERE project_id = 'project-candidate' AND candidate_id = ?`, candidateID); err != nil {
		t.Fatalf("discard staging candidate: %v", err)
	}
	var childCount, inboxCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames`).Scan(&childCount); err != nil {
		t.Fatalf("count discarded candidate frames: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_inbox WHERE project_id = 'project-candidate'`).Scan(&inboxCount); err != nil {
		t.Fatalf("count preserved inbox rows: %v", err)
	}
	if childCount != 0 || inboxCount != 4 {
		t.Fatalf("discard result children=%d inbox=%d, want 0 and 4", childCount, inboxCount)
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

func createV2ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v2 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v2 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v2 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v2 database: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatalf("begin v2 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV2DDL); err != nil {
		db.Close()
		t.Fatalf("create v2 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		db.Close()
		t.Fatalf("set v2 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 2`); err != nil {
		db.Close()
		t.Fatalf("set v2 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 2, ?)`,
		checksumSchemaV2(),
	); err != nil {
		db.Close()
		t.Fatalf("record v2 schema identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatalf("commit v2 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v2 database: %v", err)
	}
	return databasePath
}

func schemaDigestBytes(value byte) []byte {
	return bytes.Repeat([]byte{value}, 32)
}

type schemaIdentityState struct {
	userVersion int
	rowVersion  int
	checksum    string
	objects     int
}

func schemaIdentitySnapshot(t *testing.T, db *sql.DB) schemaIdentityState {
	t.Helper()
	var state schemaIdentityState
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&state.userVersion); err != nil {
		t.Fatalf("read schema user version: %v", err)
	}
	if err := db.QueryRow(`
SELECT schema_version, schema_checksum
FROM continuity_schema
WHERE singleton = 1`).Scan(&state.rowVersion, &state.checksum); err != nil {
		t.Fatalf("read schema identity row: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&state.objects); err != nil {
		t.Fatalf("count schema objects: %v", err)
	}
	return state
}
