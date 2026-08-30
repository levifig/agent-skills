package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
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
	if userVersion != schemaVersion {
		t.Fatalf("user version = %d, want %d", userVersion, schemaVersion)
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

func TestContinuitySQLiteV7SyncSchemaIsExactAndCredentialFree(t *testing.T) {
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
		"continuity_sync_authorities",
		"continuity_sync_authority_candidate_environments",
		"continuity_sync_authority_candidate_membership_events",
		"continuity_sync_authority_candidate_pages",
		"continuity_sync_authority_candidates",
		"continuity_sync_authority_recovery_terminal_receipts",
		"continuity_sync_authority_recovery_transitions",
		"continuity_sync_environment_certificates",
		"continuity_sync_environment_heads",
		"continuity_sync_inbox",
		"continuity_sync_outbox",
		"continuity_sync_projects",
		"continuity_sync_receipts",
		"continuity_sync_relay_watermarks",
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
		{name: "v3", ddl: schemaV3DDL, checksum: "d1163e6ba25279c5b332ce19d383d7709d4dc00b49928e32e8a58ccf70aaa3af", bytes: 19161},
		{name: "v4", ddl: schemaV4DDL, checksum: "150cc2b8dfcaecda0fefcfbdff02aa924644984ae7bdef4480f884dc63fe95ca", bytes: 28556},
		{name: "v5", ddl: schemaV5DDL, checksum: "4f81496ceac409a49ddd980fed0fe3f037cc7bff8e45f7b4f0a4e3a1aba985e4", bytes: 29204},
		{name: "v6", ddl: schemaV6DDL, checksum: "1aa97f7f4f453f8bf0a659a346949e8865900dbc9675b8737c481238bf69843e", bytes: 31313},
		{name: "v7", ddl: schemaV7DDL, checksum: "cc8885f15ec98c010752282222ece44fcd9e8378a212aa177d762112fca1e930", bytes: 34272},
		{name: "v8", ddl: schemaDDL, checksum: "c65fb57b1fe6e50c71246b4b654aff767dd8ef7236c483070a634b033c6e67e9", bytes: 34469},
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

func TestContinuitySQLiteMigratesExactV5SchemaToV6(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	createV5ContinuityDatabase(t, stateRoot)
	store, err := Open(stateRoot, "environment-v6")
	if err != nil {
		t.Fatalf("Open(v5) error = %v", err)
	}
	defer store.Close()
	if err := validateSchema(store.db); err != nil {
		t.Fatalf("validate migrated v6 schema: %v", err)
	}
	var nonOrdinary, transitions int64
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_candidates WHERE role <> 'ordinary'`).Scan(&nonOrdinary); err != nil {
		t.Fatalf("count migrated candidate roles: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_transitions`).Scan(&transitions); err != nil {
		t.Fatalf("count migrated transitions: %v", err)
	}
	if nonOrdinary != 0 || transitions != 0 {
		t.Fatalf("migration synthesized transition state: roles=%d transitions=%d", nonOrdinary, transitions)
	}
}

func TestContinuitySQLiteMigratesExactV6RecoveryTransitionsToV7(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV6ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v6 database: %v", err)
	}
	seedV6RecoveryTransitions(t, db)
	before := snapshotV6RecoveryTransitions(t, db, false)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v6 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-v7")
	if err != nil {
		t.Fatalf("Open(v6) error = %v", err)
	}
	defer store.Close()
	if err := validateSchema(store.db); err != nil {
		t.Fatalf("validate migrated v7 schema: %v", err)
	}
	after := snapshotV6RecoveryTransitions(t, store.db, true)
	if strings.Join(after, "|") != strings.Join(before, "|") {
		t.Fatalf("migrated recovery transition bytes changed:\nbefore=%v\nafter=%v", before, after)
	}
	var attemptMismatches, receipts int64
	if err := store.db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_authority_recovery_transitions
WHERE attempt_id <> successor_candidate_id`).Scan(&attemptMismatches); err != nil {
		t.Fatalf("inspect migrated attempt identities: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_terminal_receipts`).Scan(&receipts); err != nil {
		t.Fatalf("count migrated terminal receipts: %v", err)
	}
	if attemptMismatches != 0 || receipts != 0 {
		t.Fatalf("migration attempt mismatches=%d terminal receipts=%d", attemptMismatches, receipts)
	}
	foreignKeys, err := store.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("check migrated foreign keys: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("migrated v7 schema has a foreign-key violation")
	}
}

func TestContinuitySQLiteConcurrentOpenMigratesPopulatedV6RecoveryTransitions(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV6ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v6 database: %v", err)
	}
	seedV6RecoveryTransitions(t, db)
	before := snapshotV6RecoveryTransitions(t, db, false)
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v6 database: %v", err)
	}

	const openers = 2
	start := make(chan struct{})
	errorsByOpener := make([]error, openers)
	var wait sync.WaitGroup
	wait.Add(openers)
	for opener := 0; opener < openers; opener++ {
		opener := opener
		go func() {
			defer wait.Done()
			<-start
			environmentID := continuity.EnvironmentID("environment-v7-a")
			if opener == 1 {
				environmentID = "environment-v7-b"
			}
			store, err := Open(stateRoot, environmentID)
			if err == nil {
				err = store.Close()
			}
			errorsByOpener[opener] = err
		}()
	}
	close(start)
	wait.Wait()
	for opener, err := range errorsByOpener {
		if err != nil {
			t.Fatalf("concurrent Open(populated v6) caller %d: %v", opener, err)
		}
	}

	db, err = openDatabase(databasePath)
	if err != nil {
		t.Fatalf("reopen concurrently migrated v7 database: %v", err)
	}
	defer db.Close()
	if err := validateSchema(db); err != nil {
		t.Fatalf("validate concurrently migrated v7 schema: %v", err)
	}
	after := snapshotV6RecoveryTransitions(t, db, true)
	if strings.Join(after, "|") != strings.Join(before, "|") {
		t.Fatalf("concurrently migrated recovery transition bytes changed:\nbefore=%v\nafter=%v", before, after)
	}
	var attemptMismatches, receipts, legacyTables int64
	if err := db.QueryRow(`
SELECT COUNT(*)
FROM continuity_sync_authority_recovery_transitions
WHERE attempt_id <> successor_candidate_id`).Scan(&attemptMismatches); err != nil {
		t.Fatalf("inspect concurrently migrated attempt identities: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authority_recovery_terminal_receipts`).Scan(&receipts); err != nil {
		t.Fatalf("count concurrently migrated terminal receipts: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_authority_recovery_transitions_v6'`).Scan(&legacyTables); err != nil {
		t.Fatalf("inspect concurrently migrated legacy table: %v", err)
	}
	if attemptMismatches != 0 || receipts != 0 || legacyTables != 0 {
		t.Fatalf("concurrent migration mismatches=%d receipts=%d legacy tables=%d", attemptMismatches, receipts, legacyTables)
	}
	foreignKeys, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("check concurrently migrated foreign keys: %v", err)
	}
	defer foreignKeys.Close()
	if foreignKeys.Next() {
		t.Fatal("concurrently migrated v7 schema has a foreign-key violation")
	}
}

func TestContinuitySQLiteRefusesDriftedV6BeforeV7Migration(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV6ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v6 database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE continuity_v6_intruder(value TEXT)`); err != nil {
		db.Close()
		t.Fatalf("create v6 drift: %v", err)
	}
	before := schemaIdentitySnapshot(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close drifted v6 database: %v", err)
	}

	if store, err := Open(stateRoot, "environment-v7"); err == nil {
		store.Close()
		t.Fatal("Open(drifted v6) error = nil, want refusal")
	}
	db, err = openDatabase(databasePath)
	if err != nil {
		t.Fatalf("reopen refused v6 database: %v", err)
	}
	defer db.Close()
	after := schemaIdentitySnapshot(t, db)
	if after != before {
		t.Fatalf("refused v6 migration mutated schema identity: before=%#v after=%#v", before, after)
	}
	var attemptColumns, receiptTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('continuity_sync_authority_recovery_transitions') WHERE name = 'attempt_id'`).Scan(&attemptColumns); err != nil {
		t.Fatalf("inspect refused v6 attempt column: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_authority_recovery_terminal_receipts'`).Scan(&receiptTables); err != nil {
		t.Fatalf("inspect refused v6 receipt table: %v", err)
	}
	if attemptColumns != 0 || receiptTables != 0 {
		t.Fatalf("refused v6 migration created attempt columns=%d receipt tables=%d", attemptColumns, receiptTables)
	}
}

func TestContinuitySQLiteRefusesDriftedOrFutureV5IdentityWithoutMigrationMutation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
	}{
		{
			name: "schema drift",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(`CREATE TABLE continuity_v5_intruder(value TEXT)`); err != nil {
					t.Fatalf("create v5 drift: %v", err)
				}
			},
		},
		{
			name: "future user version",
			mutate: func(t *testing.T, db *sql.DB) {
				if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion+1)); err != nil {
					t.Fatalf("set future user version: %v", err)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stateRoot := filepath.Join(testTempDir(t), "state")
			databasePath := createV5ContinuityDatabase(t, stateRoot)
			db, err := openDatabase(databasePath)
			if err != nil {
				t.Fatalf("open exact v5 database: %v", err)
			}
			test.mutate(t, db)
			before := schemaIdentitySnapshot(t, db)
			if err := db.Close(); err != nil {
				t.Fatalf("close malformed v5 database: %v", err)
			}

			if store, err := Open(stateRoot, "environment-v6"); err == nil {
				store.Close()
				t.Fatal("Open(malformed v5) error = nil, want refusal")
			}
			db, err = openDatabase(databasePath)
			if err != nil {
				t.Fatalf("reopen refused v5 database: %v", err)
			}
			defer db.Close()
			after := schemaIdentitySnapshot(t, db)
			if after != before {
				t.Fatalf("refused v5 migration mutated schema identity: before=%#v after=%#v", before, after)
			}
			var roleColumns, transitionObjects int
			if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('continuity_sync_authority_candidates') WHERE name = 'role'`).Scan(&roleColumns); err != nil {
				t.Fatalf("inspect refused v5 candidate role: %v", err)
			}
			if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name = 'continuity_sync_authority_recovery_transitions'`).Scan(&transitionObjects); err != nil {
				t.Fatalf("inspect refused v5 transition table: %v", err)
			}
			if roleColumns != 0 || transitionObjects != 0 {
				t.Fatalf("refused v5 migration created role=%d transitions=%d", roleColumns, transitionObjects)
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
	if _, err := db.Exec(`
INSERT INTO continuity_sync_environment_certificates(
  project_id, environment_id, certificate_id, certificate_bytes,
  mode, expires_at_millis, join_membership_generation
) VALUES('project-v2', 'environment-v2', ?, X'01', 'trusted', 0, 1)`,
		schemaDigestBytes(0x14)); err != nil {
		db.Close()
		t.Fatalf("seed v2 authority environment: %v", err)
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

func TestContinuitySQLiteMigratesExactV3AuthorityMetadataWithoutChangingTerminalCandidate(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV3ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v3 database for seed: %v", err)
	}
	projectID := continuity.ProjectID("project-v3-authority")
	authority := testSyncAuthority()
	authorityDigest, _, err := deriveTerminalCandidateIdentityV1(projectID, authority, 1)
	if err != nil {
		db.Close()
		t.Fatalf("derive v3 authority digest: %v", err)
	}
	seedV3SyncAuthority(t, db, projectID, authority, &authorityDigest)
	promotedCandidateID := testAuthorityDigest(0xd1)
	promotedAuthorityDigest := testAuthorityDigest(0xd2)
	promotedRollingDigest := testAuthorityDigest(0xd3)
	promotedCorpusDigest := testAuthorityDigest(0xd4)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest,
  post_promotion_corpus_digest, resulting_applied_cursor
) VALUES(?, ?, 'promoted', ?, ?, 1, ?, 1, 1, 1, ?, ?, 1)`,
		string(projectID), promotedCandidateID[:], authority.ChannelID[:], authority.RelayGeneration[:],
		promotedAuthorityDigest[:], promotedRollingDigest[:], promotedCorpusDigest[:],
	); err != nil {
		db.Close()
		t.Fatalf("seed old promoted terminal receipt: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded v3 database: %v", err)
	}

	store, err := Open(stateRoot, "environment-v4")
	if err != nil {
		t.Fatalf("Open(v3) error = %v", err)
	}
	defer store.Close()

	var version, digestVersion int
	var gotDigest []byte
	var arrivalHead int64
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read migrated version: %v", err)
	}
	if err := store.db.QueryRow(`
SELECT digest_version, authority_digest, inventory_arrival_head
FROM continuity_sync_authorities
WHERE project_id = ?`, string(projectID)).Scan(&digestVersion, &gotDigest, &arrivalHead); err != nil {
		t.Fatalf("read migrated authority metadata: %v", err)
	}
	if version != schemaVersion || digestVersion != 1 || !bytes.Equal(gotDigest, authorityDigest[:]) || arrivalHead != 0 {
		t.Fatalf("migrated authority metadata = version %d digest-version %d digest %x head %d", version, digestVersion, gotDigest, arrivalHead)
	}
	got, err := store.CurrentSyncAuthority(context.Background(), projectID)
	if err != nil {
		t.Fatalf("CurrentSyncAuthority(after migration) error = %v", err)
	}
	if !syncAuthorityEqual(got, authority) || got.InventoryArrivalHead != 0 {
		t.Fatalf("migrated authority = %#v, want %#v with head 0", got, authority)
	}
	var activeCandidates, promotedCandidates, candidateFrames, authorityCandidateRows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidates WHERE state = 'staging'`).Scan(&activeCandidates); err != nil {
		t.Fatalf("count migrated active terminal candidate: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidate_frames`).Scan(&candidateFrames); err != nil {
		t.Fatalf("count migrated terminal candidate frames: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_terminal_candidates WHERE state = 'promoted' AND authority_digest = ?`, promotedAuthorityDigest[:]).Scan(&promotedCandidates); err != nil {
		t.Fatalf("count migrated promoted terminal receipt: %v", err)
	}
	if err := store.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidates)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events)`).Scan(&authorityCandidateRows); err != nil {
		t.Fatalf("count synthesized authority candidate rows: %v", err)
	}
	if activeCandidates != 1 || promotedCandidates != 1 || candidateFrames != 0 || authorityCandidateRows != 0 {
		t.Fatalf("migration changed candidates: active=%d promoted=%d frames=%d authority-rows=%d", activeCandidates, promotedCandidates, candidateFrames, authorityCandidateRows)
	}
}

func TestContinuitySQLiteV3MigrationRejectsMismatchedActiveTerminalAuthorityWithoutMutation(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	databasePath := createV3ContinuityDatabase(t, stateRoot)
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v3 database for seed: %v", err)
	}
	validProjectID := continuity.ProjectID("project-v3-a-valid-candidate")
	validAuthority := testSyncAuthority()
	validDigest, _, err := deriveTerminalCandidateIdentityV1(validProjectID, validAuthority, 1)
	if err != nil {
		db.Close()
		t.Fatalf("derive valid v3 authority digest: %v", err)
	}
	seedV3SyncAuthority(t, db, validProjectID, validAuthority, &validDigest)
	projectID := continuity.ProjectID("project-v3-z-mismatched-candidate")
	wrongDigest := testAuthorityDigest(0xe1)
	seedV3SyncAuthority(t, db, projectID, testSyncAuthority(), &wrongDigest)
	before := schemaIdentitySnapshot(t, db)
	if err := db.Close(); err != nil {
		t.Fatalf("close mismatched v3 database: %v", err)
	}

	if store, err := Open(stateRoot, "environment-v4"); err == nil {
		store.Close()
		t.Fatal("Open(v3 mismatched active authority) error = nil, want refusal")
	}
	db, err = openDatabase(databasePath)
	if err != nil {
		t.Fatalf("reopen refused v3 database: %v", err)
	}
	defer db.Close()
	after := schemaIdentitySnapshot(t, db)
	if after != before {
		t.Fatalf("refused v3 migration mutated schema: before=%#v after=%#v", before, after)
	}
	var newObjects int
	if err := db.QueryRow(`
SELECT COUNT(*) FROM sqlite_schema
WHERE name IN (
  'continuity_sync_authorities',
  'continuity_sync_authority_candidates',
  'continuity_sync_authority_candidate_pages',
  'continuity_sync_authority_candidate_environments',
  'continuity_sync_authority_candidate_membership_events'
)`).Scan(&newObjects); err != nil {
		t.Fatalf("inspect refused v4 objects: %v", err)
	}
	if newObjects != 0 {
		t.Fatalf("refused v3 migration retained %d v4 objects", newObjects)
	}
}

func TestContinuitySQLiteFrozenV2ValidatorRejectsV5WithoutMutation(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	before := schemaIdentitySnapshot(t, store.db)
	if err := validateSchemaVersion(store.db, 2, checksumSchemaV2(), expectedSchemaV2Objects()); err == nil {
		t.Fatal("frozen v2 validation of v5 error = nil, want refusal")
	}
	after := schemaIdentitySnapshot(t, store.db)
	if before != after {
		t.Fatalf("frozen v2 validation mutated v5: before=%#v after=%#v", before, after)
	}
}

func TestContinuitySQLiteFrozenV3ValidatorRejectsV5WithoutMutation(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	before := schemaIdentitySnapshot(t, store.db)
	if err := validateSchemaVersion(store.db, 3, checksumSchemaV3(), expectedSchemaV3Objects()); err == nil {
		t.Fatal("frozen v3 validation of v5 error = nil, want refusal")
	}
	after := schemaIdentitySnapshot(t, store.db)
	if before != after {
		t.Fatalf("frozen v3 validation mutated v5: before=%#v after=%#v", before, after)
	}
}

func TestContinuitySQLiteFrozenV4ValidatorRejectsV5WithoutMutation(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	before := schemaIdentitySnapshot(t, store.db)
	if err := validateSchemaVersion(store.db, 4, checksumSchemaV4(), expectedSchemaV4Objects()); err == nil {
		t.Fatal("frozen v4 validation of v5 error = nil, want refusal")
	}
	after := schemaIdentitySnapshot(t, store.db)
	if before != after {
		t.Fatalf("frozen v4 validation mutated v5: before=%#v after=%#v", before, after)
	}
}

func TestContinuitySQLiteFrozenV5ValidatorRejectsV6WithoutMutation(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	before := schemaIdentitySnapshot(t, store.db)
	if err := validateSchemaVersion(store.db, 5, checksumSchemaV5(), expectedSchemaV5Objects()); err == nil {
		t.Fatal("frozen v5 validation of v6 error = nil, want refusal")
	}
	after := schemaIdentitySnapshot(t, store.db)
	if before != after {
		t.Fatalf("frozen v5 validation mutated v6: before=%#v after=%#v", before, after)
	}
}

func TestContinuitySQLiteFrozenV7ValidatorRejectsV8WithoutMutation(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	before := schemaIdentitySnapshot(t, store.db)
	if err := validateSchemaVersion(store.db, 7, checksumSchemaV7(), expectedSchemaV7Objects()); err == nil {
		t.Fatal("frozen v7 validation of v8 error = nil, want refusal")
	}
	after := schemaIdentitySnapshot(t, store.db)
	if before != after {
		t.Fatalf("frozen v7 validation mutated v8: before=%#v after=%#v", before, after)
	}
}

func TestContinuitySQLiteConcurrentOpenMigratesExactSchemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		create        func(*testing.T, string) string
		seedAuthority bool
	}{
		{name: "v1", create: createV1ContinuityDatabase},
		{name: "v2", create: createV2ContinuityDatabase, seedAuthority: true},
		{name: "v3", create: createV3ContinuityDatabase, seedAuthority: true},
		{name: "v4", create: createV4ContinuityDatabase},
		{name: "v5", create: createV5ContinuityDatabase},
		{name: "v6", create: createV6ContinuityDatabase},
		{name: "v7", create: createV7ContinuityDatabase},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stateRoot := filepath.Join(testTempDir(t), "state")
			databasePath := test.create(t, stateRoot)
			projectID := continuity.ProjectID("project-concurrent-" + test.name)
			authority := testSyncAuthority()
			var wantAuthorityDigest [32]byte
			if test.seedAuthority {
				db, err := openDatabase(databasePath)
				if err != nil {
					t.Fatalf("open exact %s database for authority seed: %v", test.name, err)
				}
				seedV3SyncAuthority(t, db, projectID, authority, nil)
				if err := db.Close(); err != nil {
					t.Fatalf("close seeded exact %s database: %v", test.name, err)
				}
				wantAuthorityDigest, err = frozenSyncAuthorityDigestV1(projectID, authority)
				if err != nil {
					t.Fatalf("digest exact %s authority: %v", test.name, err)
				}
			}

			const openers = 2
			start := make(chan struct{})
			errorsByOpener := make([]error, openers)
			var wait sync.WaitGroup
			wait.Add(openers)
			for opener := 0; opener < openers; opener++ {
				opener := opener
				go func() {
					defer wait.Done()
					<-start
					environmentID := continuity.EnvironmentID("environment-concurrent-a")
					if opener == 1 {
						environmentID = "environment-concurrent-b"
					}
					store, err := Open(stateRoot, environmentID)
					if err == nil {
						err = store.Close()
					}
					errorsByOpener[opener] = err
				}()
			}
			close(start)
			wait.Wait()
			for opener, err := range errorsByOpener {
				if err != nil {
					t.Fatalf("concurrent Open(exact %s) caller %d: %v", test.name, opener, err)
				}
			}

			db, err := openDatabase(databasePath)
			if err != nil {
				t.Fatalf("reopen concurrently migrated %s database: %v", test.name, err)
			}
			defer db.Close()
			if err := validateSchema(db); err != nil {
				t.Fatalf("validate concurrently migrated %s schema: %v", test.name, err)
			}
			var authorityRows, authorityCandidateRows int
			if err := db.QueryRow(`SELECT COUNT(*) FROM continuity_sync_authorities`).Scan(&authorityRows); err != nil {
				t.Fatalf("count concurrently migrated %s authority metadata: %v", test.name, err)
			}
			if err := db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidates)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events)`).Scan(&authorityCandidateRows); err != nil {
				t.Fatalf("count concurrently migrated %s authority candidates: %v", test.name, err)
			}
			wantAuthorityRows := 0
			if test.seedAuthority {
				wantAuthorityRows = 1
				var digestVersion int
				var digest []byte
				var arrivalHead int64
				if err := db.QueryRow(`
SELECT digest_version, authority_digest, inventory_arrival_head
FROM continuity_sync_authorities
WHERE project_id = ?`, string(projectID)).Scan(&digestVersion, &digest, &arrivalHead); err != nil {
					t.Fatalf("read concurrently migrated %s authority metadata: %v", test.name, err)
				}
				if digestVersion != 1 || !bytes.Equal(digest, wantAuthorityDigest[:]) || arrivalHead != 0 {
					t.Fatalf("concurrently migrated %s authority metadata = version %d digest %x head %d", test.name, digestVersion, digest, arrivalHead)
				}
			}
			if authorityRows != wantAuthorityRows || authorityCandidateRows != 0 {
				t.Fatalf("concurrently migrated %s rows: authority=%d want=%d candidates=%d", test.name, authorityRows, wantAuthorityRows, authorityCandidateRows)
			}
			foreignKeyRows, err := db.Query(`PRAGMA foreign_key_check`)
			if err != nil {
				t.Fatalf("check concurrently migrated %s foreign keys: %v", test.name, err)
			}
			if foreignKeyRows.Next() {
				foreignKeyRows.Close()
				t.Fatalf("concurrently migrated %s database has a foreign-key violation", test.name)
			}
			if err := foreignKeyRows.Err(); err != nil {
				foreignKeyRows.Close()
				t.Fatalf("iterate concurrently migrated %s foreign-key check: %v", test.name, err)
			}
			if err := foreignKeyRows.Close(); err != nil {
				t.Fatalf("close concurrently migrated %s foreign-key check: %v", test.name, err)
			}
		})
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

	t.Run("v2 preflight observes exact v5", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV2ToV3(store.db); err != nil {
			t.Fatalf("migrateSchemaV2ToV3(exact v5) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v2 migration preflight mutated exact v5: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v2 preflight observes exact v3", func(t *testing.T) {
		t.Parallel()
		stateRoot := filepath.Join(testTempDir(t), "state")
		databasePath := createV3ContinuityDatabase(t, stateRoot)
		db, err := openDatabase(databasePath)
		if err != nil {
			t.Fatalf("open exact v3 database: %v", err)
		}
		defer db.Close()
		before := schemaIdentitySnapshot(t, db)
		if err := migrateSchemaV2ToV3(db); err != nil {
			t.Fatalf("migrateSchemaV2ToV3(exact v3) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, db)
		if before != after {
			t.Fatalf("v2 migration preflight mutated exact v3: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v3 preflight observes exact v4", func(t *testing.T) {
		t.Parallel()
		stateRoot := filepath.Join(testTempDir(t), "state")
		databasePath := createV4ContinuityDatabase(t, stateRoot)
		db, err := openDatabase(databasePath)
		if err != nil {
			t.Fatalf("open exact v4 database: %v", err)
		}
		defer db.Close()
		before := schemaIdentitySnapshot(t, db)
		if err := migrateSchemaV3ToV4(db); err != nil {
			t.Fatalf("migrateSchemaV3ToV4(exact v4) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, db)
		if before != after {
			t.Fatalf("v3 migration preflight mutated exact v4: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v3 preflight observes exact v5", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV3ToV4(store.db); err != nil {
			t.Fatalf("migrateSchemaV3ToV4(exact v5) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v3 migration preflight mutated exact v5: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v4 preflight observes exact v5", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV4ToV5(store.db); err != nil {
			t.Fatalf("migrateSchemaV4ToV5(exact v5) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v4 migration preflight mutated exact v5: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v5 preflight observes exact v7", func(t *testing.T) {
		t.Parallel()
		stateRoot := filepath.Join(testTempDir(t), "state")
		databasePath := createV7ContinuityDatabase(t, stateRoot)
		db, err := openDatabase(databasePath)
		if err != nil {
			t.Fatalf("open exact v7 database: %v", err)
		}
		defer db.Close()
		before := schemaIdentitySnapshot(t, db)
		if err := migrateSchemaV5ToV6(db); err != nil {
			t.Fatalf("migrateSchemaV5ToV6(exact v7) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, db)
		if before != after {
			t.Fatalf("v5 migration preflight mutated exact v7: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v6 preflight observes exact v7", func(t *testing.T) {
		t.Parallel()
		stateRoot := filepath.Join(testTempDir(t), "state")
		databasePath := createV7ContinuityDatabase(t, stateRoot)
		db, err := openDatabase(databasePath)
		if err != nil {
			t.Fatalf("open exact v7 database: %v", err)
		}
		defer db.Close()
		before := schemaIdentitySnapshot(t, db)
		if err := migrateSchemaV6ToV7(db); err != nil {
			t.Fatalf("migrateSchemaV6ToV7(exact v7) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, db)
		if before != after {
			t.Fatalf("v6 migration preflight mutated exact v7: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v5 preflight observes exact v8", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV5ToV6(store.db); err != nil {
			t.Fatalf("migrateSchemaV5ToV6(exact v8) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v5 migration preflight mutated exact v8: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v6 preflight observes exact v8", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV6ToV7(store.db); err != nil {
			t.Fatalf("migrateSchemaV6ToV7(exact v8) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v6 migration preflight mutated exact v8: before=%#v after=%#v", before, after)
		}
	})

	t.Run("v7 preflight observes exact v8", func(t *testing.T) {
		t.Parallel()
		store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer store.Close()
		before := schemaIdentitySnapshot(t, store.db)
		if err := migrateSchemaV7ToV8(store.db); err != nil {
			t.Fatalf("migrateSchemaV7ToV8(exact v8) error = %v", err)
		}
		after := schemaIdentitySnapshot(t, store.db)
		if before != after {
			t.Fatalf("v7 migration preflight mutated exact v8: before=%#v after=%#v", before, after)
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

func TestContinuitySQLiteV4AuthorityCandidateConstraints(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(testTempDir(t), "state"), "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	projectID := "project-authority-candidate"
	candidateID := schemaDigestBytes(0x81)
	var initialCandidateRows int
	if err := store.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidates)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events)`).Scan(&initialCandidateRows); err != nil {
		t.Fatalf("count fresh authority candidate rows: %v", err)
	}
	if initialCandidateRows != 0 {
		t.Fatalf("fresh authority candidate rows = %d, want 0", initialCandidateRows)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 1, NULL, 'environment-a', 1, 0, ?, 1, ?)`,
		projectID, candidateID, schemaDigestBytes(0x79), schemaDigestBytes(0x7a)); err == nil {
		t.Fatal("page without authority candidate error = nil, want FK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version
) VALUES(?, ?, 'staging', ?, ?, ?, 1, 0, 1, 1, 'environment-a', ?, 2)`,
		projectID, candidateID, schemaDigestBytes(0x82), schemaDigestBytes(0x83),
		schemaDigestBytes(0x84), schemaDigestBytes(0x85)); err != nil {
		t.Fatalf("insert authority candidate header: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
) VALUES(?, ?, 'ready', ?, ?, ?, 1, 0, 1, 1, 'environment-a', ?, 2, ?)`,
		projectID, schemaDigestBytes(0x86), schemaDigestBytes(0x82), schemaDigestBytes(0x83),
		schemaDigestBytes(0x84), schemaDigestBytes(0x87), schemaDigestBytes(0x88)); err == nil {
		t.Fatal("second active authority candidate error = nil, want partial-index refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version, authority_digest
) VALUES(?, ?, 'promoted', ?, ?, ?, 257, 257, 65, 257, 'environment-257', ?, 2, ?)`,
		projectID, schemaDigestBytes(0x96), schemaDigestBytes(0x82), schemaDigestBytes(0x83),
		schemaDigestBytes(0x84), schemaDigestBytes(0x97), schemaDigestBytes(0x98)); err != nil {
		t.Fatalf("insert authority candidate above compatibility inventory bound: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 1, NULL, 'environment-a', 1, 0, ?, 1, ?)`,
		projectID, candidateID, schemaDigestBytes(0x89), schemaDigestBytes(0x8a)); err != nil {
		t.Fatalf("insert authority candidate page: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 2, 'environment-a', 'environment-b', 1, 0, ?, 2, ?)`,
		projectID, candidateID, schemaDigestBytes(0x8b), schemaDigestBytes(0x8c)); err == nil {
		t.Fatal("second final authority page error = nil, want partial-index refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_pages(
  project_id, candidate_id, page_number, after_environment_id,
  through_environment_id, environment_count, more, page_digest,
  resulting_environment_count, resulting_rolling_digest
) VALUES(?, ?, 2, NULL, 'environment-b', 1, 1, ?, 2, ?)`,
		projectID, candidateID, schemaDigestBytes(0x8b), schemaDigestBytes(0x8c)); err == nil {
		t.Fatal("later authority page with NULL cursor error = nil, want CHECK refusal")
	}
	certificateID := schemaDigestBytes(0x8d)
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation
) VALUES(?, ?, 'environment-a', 1, 1, ?, X'01', 'trusted', 0, 1)`,
		projectID, candidateID, certificateID); err != nil {
		t.Fatalf("insert authority candidate environment: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_membership_events(
  project_id, candidate_id, membership_generation, event_kind, environment_id
) VALUES(?, ?, 1, 'join', 'environment-a')`, projectID, candidateID); err != nil {
		t.Fatalf("insert authority candidate membership event: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation
) VALUES(?, ?, 'environment-orphan', 2, 99, ?, X'01', 'trusted', 0, 2)`,
		projectID, candidateID, schemaDigestBytes(0x8e)); err == nil {
		t.Fatal("environment on missing candidate page error = nil, want FK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation, retirement_relay_generation
) VALUES(?, ?, 'environment-partial', 2, 1, ?, X'01', 'trusted', 0, 2, ?)`,
		projectID, candidateID, schemaDigestBytes(0x8f), schemaDigestBytes(0x90)); err == nil {
		t.Fatal("partial candidate retirement error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation
) VALUES(?, ?, 'environment-duplicate-ordinal', 1, 1, ?, X'01', 'trusted', 0, 2)`,
		projectID, candidateID, schemaDigestBytes(0x95)); err == nil {
		t.Fatal("duplicate candidate environment ordinal error = nil, want candidate-wide UNIQUE refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_environments(
  project_id, candidate_id, environment_id, environment_ordinal, page_number,
  certificate_id, certificate_bytes, mode, expires_at_millis,
  join_membership_generation
) VALUES(?, ?, 'environment-duplicate-certificate', 2, 1, ?, X'01', 'trusted', 0, 2)`,
		projectID, candidateID, certificateID); err == nil {
		t.Fatal("duplicate candidate certificate error = nil, want candidate-wide UNIQUE refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_membership_events(
  project_id, candidate_id, membership_generation, event_kind, environment_id
) VALUES(?, ?, 1, 'retirement', 'environment-a')`, projectID, candidateID); err == nil {
		t.Fatal("duplicate candidate membership generation error = nil, want candidate-wide UNIQUE refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authority_candidate_membership_events(
  project_id, candidate_id, membership_generation, event_kind, environment_id
) VALUES(?, ?, 2, 'join', 'environment-missing')`, projectID, candidateID); err == nil {
		t.Fatal("membership event without candidate environment error = nil, want FK refusal")
	}

	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES('project-authority-metadata-constraints', ?, ?, ?, 1, 'staging', 0, 0, 0)`,
		schemaDigestBytes(0x91), schemaDigestBytes(0x92), schemaDigestBytes(0x93)); err != nil {
		t.Fatalf("insert metadata constraint project: %v", err)
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) VALUES('project-authority-metadata-constraints', 1, ?, 1)`, schemaDigestBytes(0x94)); err == nil {
		t.Fatal("v1 metadata with nonzero head error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) VALUES('project-authority-metadata-constraints', 1, zeroblob(32), 0)`); err == nil {
		t.Fatal("zero authority metadata digest error = nil, want CHECK refusal")
	}
	if _, err := store.db.Exec(`
INSERT INTO continuity_sync_authorities(
  project_id, digest_version, authority_digest, inventory_arrival_head
) VALUES('project-authority-metadata-constraints', 1, zeroblob(31), 0)`); err == nil {
		t.Fatal("malformed authority metadata digest error = nil, want CHECK refusal")
	}

	if _, err := store.db.Exec(`DELETE FROM continuity_sync_authority_candidates WHERE project_id = ? AND candidate_id = ?`, projectID, candidateID); err != nil {
		t.Fatalf("delete authority candidate header: %v", err)
	}
	var children int
	if err := store.db.QueryRow(`
SELECT
  (SELECT COUNT(*) FROM continuity_sync_authority_candidate_pages)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_environments)
  + (SELECT COUNT(*) FROM continuity_sync_authority_candidate_membership_events)`).Scan(&children); err != nil {
		t.Fatalf("count cascaded authority candidate children: %v", err)
	}
	if children != 0 {
		t.Fatalf("authority candidate cascade left %d children", children)
	}
	foreignKeyRows, err := store.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("run v4 foreign-key check: %v", err)
	}
	defer foreignKeyRows.Close()
	if foreignKeyRows.Next() {
		t.Fatal("foreign_key_check returned a v4 authority candidate violation")
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

func createV3ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v3 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v3 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v3 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v3 database: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatalf("begin v3 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV3DDL); err != nil {
		db.Close()
		t.Fatalf("create v3 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		db.Close()
		t.Fatalf("set v3 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 3`); err != nil {
		db.Close()
		t.Fatalf("set v3 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 3, ?)`,
		checksumSchemaV3(),
	); err != nil {
		db.Close()
		t.Fatalf("record v3 schema identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatalf("commit v3 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v3 database: %v", err)
	}
	return databasePath
}

func createV4ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v4 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v4 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v4 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v4 database: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatalf("begin v4 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV4DDL); err != nil {
		db.Close()
		t.Fatalf("create v4 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		db.Close()
		t.Fatalf("set v4 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 4`); err != nil {
		db.Close()
		t.Fatalf("set v4 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 4, ?)`,
		checksumSchemaV4(),
	); err != nil {
		db.Close()
		t.Fatalf("record v4 schema identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatalf("commit v4 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v4 database: %v", err)
	}
	return databasePath
}

func createV5ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v5 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v5 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v5 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v5 database: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatalf("begin v5 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV5DDL); err != nil {
		db.Close()
		t.Fatalf("create v5 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		db.Close()
		t.Fatalf("set v5 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 5`); err != nil {
		db.Close()
		t.Fatalf("set v5 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 5, ?)`,
		checksumSchemaV5(),
	); err != nil {
		db.Close()
		t.Fatalf("record v5 schema identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatalf("commit v5 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v5 database: %v", err)
	}
	return databasePath
}

func createV6ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v6 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v6 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v6 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v6 database: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatalf("begin v6 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV6DDL); err != nil {
		db.Close()
		t.Fatalf("create v6 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		db.Close()
		t.Fatalf("set v6 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 6`); err != nil {
		db.Close()
		t.Fatalf("set v6 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 6, ?)`,
		checksumSchemaV6(),
	); err != nil {
		db.Close()
		t.Fatalf("record v6 schema identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatalf("commit v6 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v6 database: %v", err)
	}
	return databasePath
}

func createV7ContinuityDatabase(t *testing.T, stateRoot string) string {
	t.Helper()

	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create v7 private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create v7 database: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close v7 database file: %v", err)
	}
	db, err := openDatabase(databasePath)
	if err != nil {
		t.Fatalf("open v7 database: %v", err)
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		t.Fatalf("begin v7 schema: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schemaV7DDL); err != nil {
		db.Close()
		t.Fatalf("create v7 schema: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA application_id = 1280267825`); err != nil {
		db.Close()
		t.Fatalf("set v7 application id: %v", err)
	}
	if _, err := tx.Exec(`PRAGMA user_version = 7`); err != nil {
		db.Close()
		t.Fatalf("set v7 user version: %v", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO continuity_schema(singleton, schema_line, schema_version, schema_checksum) VALUES(1, 'vnext', 7, ?)`,
		checksumSchemaV7(),
	); err != nil {
		db.Close()
		t.Fatalf("record v7 schema identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		t.Fatalf("commit v7 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v7 database: %v", err)
	}
	return databasePath
}

func seedV6RecoveryTransitions(t *testing.T, db *sql.DB) {
	t.Helper()
	insertCandidate := func(projectID string, seed byte, state, role string, generation int) []byte {
		t.Helper()
		candidateID := schemaDigestBytes(seed)
		var authorityDigest any
		if state == "ready" {
			authorityDigest = schemaDigestBytes(seed + 1)
		}
		if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  admin_public_key, membership_generation, inventory_arrival_head,
  page_count, environment_count, through_environment_id,
  rolling_environment_digest, authority_digest_version,
  authority_digest, role
) VALUES(?, ?, ?, ?, ?, ?, ?, 0, 1, 1, 'environment-v6', ?, 2, ?, ?)`,
			projectID, candidateID, state, schemaDigestBytes(seed+2), schemaDigestBytes(seed+3),
			schemaDigestBytes(seed+4), generation, schemaDigestBytes(seed+5), authorityDigest, role,
		); err != nil {
			t.Fatalf("insert v6 recovery candidate: %v", err)
		}
		return candidateID
	}
	bootstrapSuccessor := insertCandidate("project-v6-bootstrap", 0x11, "staging", "recovery-successor", 1)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_recovery_transitions(
  project_id, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation
) VALUES('project-v6-bootstrap', NULL, ?, 'environment-writer', ?, 1)`,
		bootstrapSuccessor, schemaDigestBytes(0x21),
	); err != nil {
		t.Fatalf("insert v6 bootstrap transition: %v", err)
	}
	predecessor := insertCandidate("project-v6-predecessor", 0x31, "ready", "recovery-predecessor", 1)
	successor := insertCandidate("project-v6-predecessor", 0x41, "staging", "recovery-successor", 2)
	if _, err := db.Exec(`
INSERT INTO continuity_sync_authority_recovery_transitions(
  project_id, predecessor_candidate_id, successor_candidate_id,
  writer_environment_id, writer_certificate_id, target_membership_generation
) VALUES('project-v6-predecessor', ?, ?, 'environment-writer', ?, 2)`,
		predecessor, successor, schemaDigestBytes(0x51),
	); err != nil {
		t.Fatalf("insert v6 predecessor transition: %v", err)
	}
}

func snapshotV6RecoveryTransitions(t *testing.T, db *sql.DB, includeAttempt bool) []string {
	t.Helper()
	attemptProjection := "''"
	if includeAttempt {
		attemptProjection = "hex(attempt_id)"
	}
	rows, err := db.Query(`
SELECT project_id, ` + attemptProjection + `, COALESCE(hex(predecessor_candidate_id), ''),
       hex(successor_candidate_id), writer_environment_id,
       hex(writer_certificate_id), target_membership_generation
FROM continuity_sync_authority_recovery_transitions
ORDER BY project_id`)
	if err != nil {
		t.Fatalf("snapshot v6 recovery transitions: %v", err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var projectID, attemptID, predecessor, successor, writer, certificate string
		var generation int64
		if err := rows.Scan(&projectID, &attemptID, &predecessor, &successor, &writer, &certificate, &generation); err != nil {
			t.Fatalf("scan v6 recovery transition: %v", err)
		}
		if includeAttempt && attemptID != successor {
			t.Fatalf("migrated attempt ID = %s, want successor %s", attemptID, successor)
		}
		result = append(result, projectID+":"+predecessor+":"+successor+":"+writer+":"+certificate+":"+fmt.Sprint(generation))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate v6 recovery transitions: %v", err)
	}
	return result
}

func seedV3SyncAuthority(t *testing.T, db *sql.DB, projectID continuity.ProjectID, authority SyncAuthority, activeCandidateDigest *[32]byte) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin v3 authority seed: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
INSERT INTO continuity_sync_projects(
  project_id, channel_id, relay_generation, admin_public_key,
  membership_generation, activation_state, downloaded_cursor,
  applied_cursor, relay_head
) VALUES(?, ?, ?, ?, ?, 'staging', 0, 0, 0)`,
		string(projectID), authority.ChannelID[:], authority.RelayGeneration[:], authority.AdminPublicKey[:], authority.MembershipGeneration,
	); err != nil {
		t.Fatalf("seed v3 sync project: %v", err)
	}
	for _, environment := range authority.Environments {
		if err := insertSyncEnvironmentCertificateV1(context.Background(), tx, projectID, environment); err != nil {
			t.Fatalf("seed v3 environment %q: %v", environment.EnvironmentID, err)
		}
	}
	if activeCandidateDigest != nil {
		candidateID := testAuthorityDigest(0xe2)
		rollingDigest := testAuthorityDigest(0xe3)
		if _, err := tx.Exec(`
INSERT INTO continuity_sync_terminal_candidates(
  project_id, candidate_id, state, channel_id, relay_generation,
  membership_generation, authority_digest, start_arrival_sequence,
  through_arrival_sequence, frame_count, rolling_candidate_digest
) VALUES(?, ?, 'staging', ?, ?, ?, ?, 1, 1, 1, ?)`,
			string(projectID), candidateID[:], authority.ChannelID[:], authority.RelayGeneration[:],
			authority.MembershipGeneration, activeCandidateDigest[:], rollingDigest[:],
		); err != nil {
			t.Fatalf("seed v3 active terminal candidate: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit v3 authority seed: %v", err)
	}
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
