package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestJournalOriginsMigrationBackfillsSchema9ObservableEnvelope(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	store, projectID := openMigrationFixture(t, root, SchemaMigrations()[:9])
	defer store.Close()
	insertMigrationJournalRow(t, store.db, projectID, "journal-nine", "decision", "migration", "observable branch", "feat/origins", "/worktree", "harness-session-nine")
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()); err != nil {
		t.Fatalf("ApplyMigrations(9->11) error = %v", err)
	}

	var mechanism, branch, worktree, harness string
	if err := store.db.QueryRowContext(ctx, `SELECT capture_mechanism, branch, worktree, harness_session_id FROM journal_origins WHERE journal_entry_id = 'journal-nine'`).Scan(&mechanism, &branch, &worktree, &harness); err != nil {
		t.Fatalf("read backfilled origin error = %v", err)
	}
	if mechanism != "unknown" || branch != "feat/origins" || worktree != "/worktree" || harness != "harness-session-nine" {
		t.Fatalf("origin envelope = %q/%q/%q/%q, want unknown and observable values", mechanism, branch, worktree, harness)
	}
	var observedHarness, sourceEvent, head sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT observed_harness, source_event, head FROM journal_origins WHERE journal_entry_id = 'journal-nine'`).Scan(&observedHarness, &sourceEvent, &head); err != nil {
		t.Fatalf("read unobservable origin fields error = %v", err)
	}
	if observedHarness.Valid || sourceEvent.Valid || head.Valid {
		t.Fatalf("origin inferred unobservable values: harness=%v event=%v head=%v", observedHarness, sourceEvent, head)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_entries`); got != 1 {
		t.Fatalf("journal rows after 9->11 = %d, want 1", got)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_search WHERE journal_search MATCH 'observable'`); got != 1 {
		t.Fatalf("journal search rows after 9->11 = %d, want 1", got)
	}
	if got := rawForeignKeyViolations(t, store.path); got != 0 {
		t.Fatalf("foreign-key violations after 9->11 = %d, want 0", got)
	}
}

func TestJournalOriginsMigrationChecksumDriftIsRejectedWithoutMutation(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	store, projectID := openMigrationFixture(t, root, SchemaMigrations()[:9])
	defer store.Close()
	insertMigrationJournalRow(t, store.db, projectID, "journal-checksum", "decision", "migration", "checksum", "main", "/worktree", "harness-session")
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()); err != nil {
		t.Fatalf("ApplyMigrations(9->11) error = %v", err)
	}
	originsBefore := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins`)
	registrationsBefore := rawCount(t, store.path, `SELECT COUNT(*) FROM schema_migrations WHERE version = 11`)
	drifted := journalOriginsMigration()
	drifted.SQL += "\n-- drift"
	if err := ApplyMigrations(ctx, store.db, []SchemaMigration{drifted}); err == nil {
		t.Fatal("ApplyMigrations(drifted 11) error = nil, want checksum rejection")
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins`); got != originsBefore {
		t.Fatalf("origins after checksum rejection = %d, want %d", got, originsBefore)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM schema_migrations WHERE version = 11`); got != registrationsBefore {
		t.Fatalf("migration11 registrations after checksum rejection = %d, want %d", got, registrationsBefore)
	}
}

func TestJournalOriginsMigrationAppliesAfterExplicitSchema10(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	store, projectID := openMigrationFixture(t, root, SchemaMigrations()[:9])
	defer store.Close()
	insertMigrationJournalRow(t, store.db, projectID, "journal-ten", "decision", "migration", "explicit ten", "main", "/worktree", "harness-session-ten")
	if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
		t.Fatalf("ApplyMigrations(explicit 10) error = %v", err)
	}
	if got, _ := store.SchemaVersion(ctx); got != journalFirstMigrationVersion {
		t.Fatalf("schema version after explicit 10 = %d, want %d", got, journalFirstMigrationVersion)
	}
	postJournalFirst := []SchemaMigration{}
	for _, migration := range SchemaMigrations() {
		if migration.Version > journalFirstMigrationVersion {
			postJournalFirst = append(postJournalFirst, migration)
		}
	}
	if err := ApplyMigrations(ctx, store.db, postJournalFirst); err != nil {
		t.Fatalf("ApplyMigrations(10->current) error = %v", err)
	}
	if got, _ := store.SchemaVersion(ctx); got != CurrentSchemaVersion() {
		t.Fatalf("schema version after 10->current = %d, want %d", got, CurrentSchemaVersion())
	}
	var mechanism, branch, worktree, harness string
	if err := store.db.QueryRowContext(ctx, `SELECT capture_mechanism, branch, worktree, harness_session_id FROM journal_origins WHERE journal_entry_id = 'journal-ten'`).Scan(&mechanism, &branch, &worktree, &harness); err != nil {
		t.Fatalf("read explicit10 origin error = %v", err)
	}
	if mechanism != "unknown" || branch != "main" || worktree != "/worktree" || harness != "harness-session-ten" {
		t.Fatalf("explicit10 origin = %q/%q/%q/%q, want observable envelope", mechanism, branch, worktree, harness)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_search WHERE journal_search MATCH 'explicit'`); got != 1 {
		t.Fatalf("journal search after 10->11 = %d, want 1", got)
	}
}

func TestJournalFirstAfterOriginsPreservesSurvivorsAndDeferrals(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	store, projectID := openMigrationFixture(t, root, SchemaMigrations()[:9])
	defer store.Close()
	seedJournalFirstFixture(t, store.path, projectID)
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()); err != nil {
		t.Fatalf("ApplyMigrations(9->11) error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at) VALUES ('spark-survivor', ?, 'deferred', 'open', 'surviving spark', ?, ?)`, projectID, "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert survivor spark error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at) VALUES ('spark-missing-journal', ?, 'deferred', 'open', 'orphan decision spark', ?, ?)`, projectID, "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert missing-journal spark error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at) VALUES ('spark-cross-decision', ?, 'deferred', 'open', 'cross-project decision spark', ?, ?)`, projectID, "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert cross-project decision spark error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at) VALUES ('alias-spark-survivor', ?, 'spark', 'spark-survivor', 'spark', 'survivor', ?, ?)`, projectID, "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert survivor spark alias error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO projects (id, identity_hash, created_at, updated_at) VALUES ('project-cross', 'identity-cross', ?, ?)`, "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert cross-project project error = %v", err)
	}
	insertMigrationJournalRow(t, store.db, "project-cross", "j-cross-project", "decision", "migration", "cross-project decision", "main", "/cross-worktree", "cross-project-harness")
	if _, err := store.db.ExecContext(ctx, `INSERT INTO sparks (id, project_id, scope, status, text, created_at, updated_at) VALUES ('spark-cross-project', 'project-cross', 'deferred', 'open', 'cross-project spark', ?, ?)`, "2026-07-01T00:00:00Z", "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert cross-project spark error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_origins (journal_entry_id, project_id, envelope_version, capture_mechanism, created_at) VALUES ('j-cross-project', ?, 1, 'unknown', ?)`, projectID, "2026-07-01T00:00:00Z"); err != nil {
		t.Fatalf("insert cross-project origin mismatch error = %v", err)
	}
	for _, row := range []struct {
		operation, journal, spark string
	}{
		{"op-survivor", "j-wrap", "spark-survivor"},
		{"op-missing-journal", "journal-missing", "spark-missing-journal"},
		{"op-missing-spark", "j-untagged", "spark-missing"},
		{"op-cross-decision", "j-cross-project", "spark-cross-decision"},
		{"op-cross-spark", "j-no-hsid", "spark-cross-project"},
	} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO journal_deferrals (project_id, operation_key, journal_entry_id, spark_id, stored_digest, created_at) VALUES (?, ?, ?, ?, ?, ?)`, projectID, row.operation, row.journal, row.spark, strings.Repeat("a", 64), "2026-07-01T00:00:00Z"); err != nil {
			t.Fatalf("insert deferral %s error = %v", row.operation, err)
		}
	}
	beforeCanonical := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_entries`)
	beforeSearch := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_search`)
	if beforeCanonical != 9 || beforeSearch != 9 {
		t.Fatalf("pre-0010 rows = %d/%d, want 9/9", beforeCanonical, beforeSearch)
	}
	result := JournalFirstMigrationResult{}
	if err := runJournalFirstMigration(ctx, store, &result); err != nil {
		t.Fatalf("runJournalFirstMigration after 11 error = %v", err)
	}
	if result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("post-0010 schema version = %d, want %d", result.SchemaVersion, CurrentSchemaVersion())
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_entries`); got != 6 {
		t.Fatalf("surviving journal rows = %d, want 6", got)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_search`); got != 6 {
		t.Fatalf("surviving search rows = %d, want 6", got)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins`); got != 5 {
		t.Fatalf("surviving origins = %d, want 5", got)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins WHERE journal_entry_id LIKE 'j-noise-%'`); got != 0 {
		t.Fatalf("orphaned lifecycle origins = %d, want 0", got)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_deferrals WHERE operation_key = 'op-survivor'`); got != 1 {
		t.Fatalf("surviving deferrals = %d, want 1", got)
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_deferrals`); got != 1 {
		t.Fatalf("deferral orphan pruning left %d rows, want 1", got)
	}
	for _, operation := range []string{"op-missing-journal", "op-missing-spark", "op-cross-decision", "op-cross-spark"} {
		if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_deferrals WHERE operation_key = ?`, operation); got != 0 {
			t.Fatalf("pruned deferral %s = %d, want 0", operation, got)
		}
	}
	if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins WHERE journal_entry_id = 'j-cross-project'`); got != 0 {
		t.Fatalf("cross-project origin rows = %d, want 0", got)
	}
	if got := rawForeignKeyViolations(t, store.path); got != 0 {
		t.Fatalf("foreign-key violations after 11->10 = %d, want 0", got)
	}
}

func TestJournalFirstMigrationRollsBackDestructiveWorkAtPerCallSeams(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hooks journalFirstMigrationHooks
	}{
		{name: "after-migration-10", hooks: journalFirstMigrationHooks{afterMigration10: func(*sql.Tx) error { return errors.New("injected after migration 10") }}},
		{name: "after-prune", hooks: journalFirstMigrationHooks{afterPrune: func(*sql.Tx) error { return errors.New("injected after prune") }}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			store, projectID := openMigrationFixture(t, root, SchemaMigrations()[:9])
			defer store.Close()
			seedJournalFirstFixture(t, store.path, projectID)
			if err := ApplyMigrations(ctx, store.db, SchemaMigrations()); err != nil {
				t.Fatalf("ApplyMigrations(9->11) error = %v", err)
			}
			beforeJournal := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_entries`)
			beforeSearch := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_search`)
			beforeOrigins := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins`)
			result := JournalFirstMigrationResult{}
			if err := runJournalFirstMigrationWithHooks(ctx, store, &result, &tc.hooks); err == nil {
				t.Fatal("runJournalFirstMigrationWithHooks() error = nil, want injected rollback")
			}
			if !rawTableExists(t, store.path, "sessions") {
				t.Fatal("sessions table was committed despite injected failure")
			}
			if got := rawCount(t, store.path, `SELECT COUNT(*) FROM schema_migrations WHERE version = 10`); got != 0 {
				t.Fatalf("schema_migrations version 10 rows = %d, want 0 after rollback", got)
			}
			if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_entries`); got != beforeJournal {
				t.Fatalf("journal rows after rollback = %d, want %d", got, beforeJournal)
			}
			if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_search`); got != beforeSearch {
				t.Fatalf("journal search rows after rollback = %d, want %d", got, beforeSearch)
			}
			if got := rawCount(t, store.path, `SELECT COUNT(*) FROM journal_origins`); got != beforeOrigins {
				t.Fatalf("journal origins after rollback = %d, want %d", got, beforeOrigins)
			}
		})
	}
}

func openMigrationFixture(t *testing.T, root project.Root, migrations []SchemaMigration) (*Store, string) {
	t.Helper()
	resolver := PathResolver{StateHome: t.TempDir()}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := ApplyMigrations(context.Background(), store.db, migrations); err != nil {
		store.Close()
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if err := store.UpsertProject(context.Background(), root); err != nil {
		store.Close()
		t.Fatalf("UpsertProject() error = %v", err)
	}
	identity, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		store.Close()
		t.Fatalf("LookupProjectIdentityForRoot() error = %v", err)
	}
	return store, identity.ID
}

func insertMigrationJournalRow(t *testing.T, db *sql.DB, projectID, id, entryType, scope, message, branch, worktree, harnessSession string) {
	t.Helper()
	now := "2026-07-01T00:00:00Z"
	if _, err := db.ExecContext(context.Background(), `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, session_id, spec_id, task_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?)`, id, projectID, entryType, scope, message, branch, worktree, harnessSession, now, now); err != nil {
		t.Fatalf("insert journal row error = %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message) SELECT rowid, project_id, id, COALESCE(session_id, ''), entry_type, COALESCE(scope, ''), message FROM journal_entries WHERE id = ?`, id); err != nil {
		t.Fatalf("insert journal search row error = %v", err)
	}
}
