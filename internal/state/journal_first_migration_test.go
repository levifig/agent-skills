package state

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

// seedJournalFirstFixture builds a production-shaped v9 database: sessions with
// and without harness ids, journal entries with session_id and NULL
// harness_session_id, entry_type='session' lifecycle noise, entry_type='wrap'
// synthesis, events/aliases with entity_kind='session', and a handoff carrying
// a session_id. It returns the project id.
func seedJournalFirstFixture(t *testing.T, databasePath string, projectID string) {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	now := "2026-07-01T00:00:00Z"

	statements := []struct {
		sql  string
		args []any
	}{
		// Sessions: one with a harness id, one without.
		{`INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, NULL, ?, ?)`, []any{"session-with-hsid", projectID, "hsid-abc", "feat/journal-first", "stopped", now, now}},
		{`INSERT INTO sessions (id, project_id, harness_session_id, branch, status, body_source_id, created_at, updated_at) VALUES (?, ?, NULL, ?, ?, NULL, ?, ?)`, []any{"session-no-hsid", projectID, "main", "active", now, now}},

		// Journal entries.
		// (a) real entry linked to a session that has a harness id, NULL hsid -> backfilled.
		{journalInsertSQL, journalArgs("j-backfill", projectID, "decision", "arch", "chose journal-first", "feat/journal-first", "session-with-hsid", nil, now)},
		// (b) real wrap synthesis linked to the hsid session -> preserved, backfilled.
		{journalInsertSQL, journalArgs("j-wrap", projectID, "wrap", "", "tried X abandoned Y next Z", "feat/journal-first", "session-with-hsid", nil, now)},
		// (c) entry linked to a session WITHOUT a harness id -> not backfillable, survives.
		{journalInsertSQL, journalArgs("j-no-hsid", projectID, "note", "", "no harness id available", "main", "session-no-hsid", nil, now)},
		// (d) entry that already has a harness id -> untouched.
		{journalInsertSQL, journalArgs("j-preset", projectID, "note", "", "preset harness id", "main", "session-no-hsid", strPtr("hsid-preset"), now)},
		// (e) untagged project-scoped entry (no session) -> survives.
		{journalInsertSQL, journalArgs("j-untagged", projectID, "note", "", "untagged project entry", "", "", nil, now)},
		// noise: entry_type='session' lifecycle markers -> purged.
		{journalInsertSQL, journalArgs("j-noise-1", projectID, "session", "start", "=== SESSION STARTED ===", "main", "session-with-hsid", nil, now)},
		{journalInsertSQL, journalArgs("j-noise-2", projectID, "session", "stop", "=== SESSION STOPPED ===", "main", "session-with-hsid", nil, now)},
		{journalInsertSQL, journalArgs("j-noise-3", projectID, "session", "end", "session ended", "main", "session-no-hsid", nil, now)},

		// Handoff carrying a session_id -> retargeted to harness id.
		{`INSERT INTO handoffs (id, project_id, session_id, task_id, title, status, body_source_id, created_at, updated_at) VALUES (?, ?, ?, NULL, ?, ?, NULL, ?, ?)`, []any{"handoff-1", projectID, "session-with-hsid", "Handoff", "done", now, now}},

		// events/aliases with entity_kind='session' -> deleted.
		{`INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, 'session', ?, 'status_changed', 'active', 'stopped', 'noise', ?, ?)`, []any{"event-session-1", projectID, "session-with-hsid", now, now}},
		{`INSERT INTO events (id, project_id, entity_kind, entity_id, event_type, from_status, to_status, note, created_at, updated_at) VALUES (?, ?, 'spec', ?, 'status_changed', 'todo', 'done', 'kept', ?, ?)`, []any{"event-spec-1", projectID, "spec-x", now, now}},
		{`INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at) VALUES (?, ?, 'session', ?, 'session', 'session-1', ?, ?)`, []any{"alias-session-1", projectID, "session-with-hsid", now, now}},
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed statement %q error = %v", statement.sql, err)
		}
	}
	// Mirror the journal rows into the v9 FTS index so we can prove it is rebuilt.
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
SELECT rowid, project_id, id, COALESCE(session_id, ''), entry_type, COALESCE(scope, ''), message FROM journal_entries
`); err != nil {
		t.Fatalf("seed journal_search error = %v", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		store.Close()
		t.Fatalf("begin seeded journal-search rebuild: %v", err)
	}
	if _, err := rebuildAndVerifyJournalSearch(ctx, tx); err != nil {
		tx.Rollback()
		store.Close()
		t.Fatalf("rebuild seeded journal-search fixture: %v", err)
	}
	if err := tx.Commit(); err != nil {
		store.Close()
		t.Fatalf("commit seeded journal-search rebuild: %v", err)
	}
}

const journalInsertSQL = `INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, session_id, spec_id, task_id, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, NULL, NULL, ?, ?)`

func journalArgs(id, projectID, entryType, scope, message, branch, sessionID string, harnessSessionID *string, now string) []any {
	return []any{
		id, projectID, entryType, emptyToNil(scope), message, emptyToNil(branch), harnessSessionID, emptyToNil(sessionID), now, now,
	}
}

func strPtr(s string) *string { return &s }

func TestPreviewJournalFirstMigrationUsesCopyRun(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	result, err := PreviewJournalFirstMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("PreviewJournalFirstMigration() error = %v", err)
	}
	if result.Action != JournalFirstMigrationActionDryRun || result.Applied || !result.CopyRun {
		t.Fatalf("preview action/applied/copy_run = %q/%t/%t, want dry-run/false/true", result.Action, result.Applied, result.CopyRun)
	}
	if result.BackupPath != "" {
		t.Fatalf("preview backup path = %q, want empty (dry-run must not back up)", result.BackupPath)
	}
	// 8 journal rows seeded, 3 are entry_type='session' noise -> 5 survive.
	if result.JournalEntriesBefore != 8 || result.NoiseEntriesPurged != 3 || result.JournalEntriesAfter != 5 {
		t.Fatalf("preview journal counts before/purged/after = %d/%d/%d, want 8/3/5", result.JournalEntriesBefore, result.NoiseEntriesPurged, result.JournalEntriesAfter)
	}
	// Backfill is measured pre-purge: j-backfill, j-wrap, j-noise-1, j-noise-2 all
	// link to session-with-hsid with a NULL harness_session_id.
	if result.HarnessSessionBackfill != 4 {
		t.Fatalf("preview backfillable = %d, want 4", result.HarnessSessionBackfill)
	}
	if result.SessionEventsDeleted != 1 || result.SessionAliasesDeleted != 1 {
		t.Fatalf("preview session events/aliases = %d/%d, want 1/1", result.SessionEventsDeleted, result.SessionAliasesDeleted)
	}
	if result.SessionsDropped != 2 {
		t.Fatalf("preview sessions dropped = %d, want 2", result.SessionsDropped)
	}
	if result.JournalSearchRows != 5 {
		t.Fatalf("preview journal_search rows = %d, want 5", result.JournalSearchRows)
	}
	if result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("preview schema version = %d, want %d", result.SchemaVersion, CurrentSchemaVersion())
	}

	// The live database must be untouched by a dry-run.
	if got := rawTableExists(t, status.DatabasePath, "sessions"); !got {
		t.Fatalf("live sessions table missing after preview; dry-run must not mutate live state")
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries`); got != 8 {
		t.Fatalf("live journal_entries after preview = %d, want 8 (untouched)", got)
	}
}

func TestApplyJournalFirstMigrationTransformsLiveDatabase(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	applied, err := ApplyJournalFirstMigration(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}
	if !applied.Applied || applied.CopyRun || applied.Action != JournalFirstMigrationActionApply {
		t.Fatalf("apply action/applied/copy_run = %q/%t/%t, want apply/true/false", applied.Action, applied.Applied, applied.CopyRun)
	}
	if applied.BackupPath == "" {
		t.Fatalf("apply backup path is empty; a pre-migration backup is mandatory")
	}
	if _, err := os.Stat(applied.BackupPath); err != nil {
		t.Fatalf("stat backup path %q: %v", applied.BackupPath, err)
	}
	if applied.NoiseEntriesPurged != 3 || applied.JournalEntriesAfter != 5 || applied.HarnessSessionBackfill != 4 {
		t.Fatalf("apply counts purged/after/backfill = %d/%d/%d, want 3/5/4", applied.NoiseEntriesPurged, applied.JournalEntriesAfter, applied.HarnessSessionBackfill)
	}
	if applied.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("apply schema version = %d, want %d", applied.SchemaVersion, CurrentSchemaVersion())
	}

	// sessions and session_state_snapshots must be gone.
	if rawTableExists(t, status.DatabasePath, "sessions") {
		t.Fatalf("sessions table still present after apply")
	}
	if rawTableExists(t, status.DatabasePath, "session_state_snapshots") {
		t.Fatalf("session_state_snapshots table still present after apply")
	}

	// journal_entries.session_id column must be gone; harness_session_id present.
	if rawColumnExists(t, status.DatabasePath, "journal_entries", "session_id") {
		t.Fatalf("journal_entries.session_id column still present after apply")
	}
	if !rawColumnExists(t, status.DatabasePath, "journal_entries", "harness_session_id") {
		t.Fatalf("journal_entries.harness_session_id column missing after apply")
	}

	// handoffs.session_id gone; harness_session_id present and backfilled.
	if rawColumnExists(t, status.DatabasePath, "handoffs", "session_id") {
		t.Fatalf("handoffs.session_id column still present after apply")
	}
	if !rawColumnExists(t, status.DatabasePath, "handoffs", "harness_session_id") {
		t.Fatalf("handoffs.harness_session_id column missing after apply")
	}
	if got := rawString(t, status.DatabasePath, `SELECT harness_session_id FROM handoffs WHERE id = 'handoff-1'`); got != "hsid-abc" {
		t.Fatalf("handoff harness_session_id after apply = %q, want hsid-abc", got)
	}

	// Backfilled harness ids on journal rows.
	if got := rawString(t, status.DatabasePath, `SELECT harness_session_id FROM journal_entries WHERE id = 'j-backfill'`); got != "hsid-abc" {
		t.Fatalf("j-backfill harness_session_id after apply = %q, want hsid-abc", got)
	}
	if got := rawString(t, status.DatabasePath, `SELECT harness_session_id FROM journal_entries WHERE id = 'j-wrap'`); got != "hsid-abc" {
		t.Fatalf("j-wrap harness_session_id after apply = %q, want hsid-abc", got)
	}
	// Preset harness id untouched.
	if got := rawString(t, status.DatabasePath, `SELECT harness_session_id FROM journal_entries WHERE id = 'j-preset'`); got != "hsid-preset" {
		t.Fatalf("j-preset harness_session_id after apply = %q, want hsid-preset", got)
	}

	// Noise purged; wrap synthesis preserved; non-noise entries all survive.
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE entry_type = 'session'`); got != 0 {
		t.Fatalf("session-type journal rows after apply = %d, want 0", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE id = 'j-wrap'`); got != 1 {
		t.Fatalf("j-wrap survived count = %d, want 1", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries`); got != 5 {
		t.Fatalf("journal_entries after apply = %d, want 5", got)
	}

	// events/aliases session rows deleted; non-session rows kept.
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM events WHERE entity_kind = 'session'`); got != 0 {
		t.Fatalf("session events after apply = %d, want 0", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM events WHERE entity_kind = 'spec'`); got != 1 {
		t.Fatalf("spec events after apply = %d, want 1 (preserved)", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM aliases WHERE entity_kind = 'session'`); got != 0 {
		t.Fatalf("session aliases after apply = %d, want 0", got)
	}

	// FTS rebuilt with harness_session_id column, repopulated, still matches.
	if !rawColumnExists(t, status.DatabasePath, "journal_search", "harness_session_id") {
		t.Fatalf("journal_search.harness_session_id column missing after apply")
	}
	if rawColumnExists(t, status.DatabasePath, "journal_search", "session_id") {
		t.Fatalf("journal_search.session_id column still present after apply")
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_search`); got != 5 {
		t.Fatalf("journal_search rows after apply = %d, want 5 (== surviving journal rows)", got)
	}
	if got := rawString(t, status.DatabasePath, `SELECT journal_entry_id FROM journal_search WHERE journal_search MATCH 'abandoned'`); got != "j-wrap" {
		t.Fatalf("FTS match for surviving wrap = %q, want j-wrap", got)
	}

	// Integrity + FK checks clean.
	if got := rawString(t, status.DatabasePath, `PRAGMA integrity_check`); got != "ok" {
		t.Fatalf("integrity_check after apply = %q, want ok", got)
	}
	if n := rawForeignKeyViolations(t, status.DatabasePath); n != 0 {
		t.Fatalf("foreign_key_check violations after apply = %d, want 0", n)
	}

	// schema_migrations records version 10.
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM schema_migrations WHERE version = 10`); got != 1 {
		t.Fatalf("schema_migrations version 10 rows = %d, want 1", got)
	}
}

func TestApplyJournalFirstMigrationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(ctx, root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	if _, err := ApplyJournalFirstMigration(ctx, root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("first ApplyJournalFirstMigration() error = %v", err)
	}
	// Re-open the migrated store directly and re-run the shared migration set:
	// migration 10 is checksum-recorded, so re-applying must be a no-op.
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
		t.Fatalf("idempotent journal-first re-apply error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, []SchemaMigration{journalOriginsMigration()}); err != nil {
		t.Fatalf("idempotent journal-origins re-apply error = %v", err)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM schema_migrations WHERE version = 10`); got != 1 {
		t.Fatalf("schema_migrations version 10 rows after re-apply = %d, want 1", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries`); got != 5 {
		t.Fatalf("journal_entries after re-apply = %d, want 5 (unchanged)", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_search`); got != 5 {
		t.Fatalf("journal_search after re-apply = %d, want 5 (unchanged)", got)
	}
}

func TestJournalFirstMigrationRebuildsModifiedDerivedState(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)
	if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.db.ExecContext(ctx, `UPDATE journal_search SET message = 'diverged derived content' WHERE journal_entry_id = 'j-wrap'`); err != nil {
		t.Fatalf("modify derived journal row: %v", err)
	}
	result := journalFirstMigrationBaseResult(status, JournalFirstMigrationActionApply)
	if err := runJournalFirstMigration(ctx, store, &result); err != nil {
		t.Fatalf("runJournalFirstMigration() error = %v", err)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("post-rebuild journal parity = %#v, want ready", parity)
	}
}

func TestJournalFirstMigrationRefusesAfterBackupConcurrentCanonicalUpdate(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)
	result, err := applyJournalFirstMigrationWithHooks(ctx, root, resolver, &journalFirstApplyHooks{afterBackup: func(string) error {
		writer, openErr := OpenStore(status.DatabasePath)
		if openErr != nil {
			return openErr
		}
		defer writer.Close()
		_, execErr := writer.db.ExecContext(ctx, `UPDATE journal_entries SET message = 'changed after backup' WHERE id = 'j-wrap'`)
		return execErr
	}})
	if err == nil || result.BackupPath == "" {
		t.Fatalf("apply result=%#v err=%v, want stale-source refusal with retained backup", result, err)
	}
	if got := rawString(t, status.DatabasePath, `SELECT message FROM journal_entries WHERE id = 'j-wrap'`); got != "changed after backup" {
		t.Fatalf("live canonical update = %q, want retained concurrent change", got)
	}
	backup, openErr := OpenStoreReadOnly(result.BackupPath)
	if openErr != nil {
		t.Fatalf("OpenStoreReadOnly(backup) error = %v", openErr)
	}
	defer backup.Close()
	if got := rawString(t, result.BackupPath, `SELECT message FROM journal_entries WHERE id = 'j-wrap'`); got == "changed after backup" {
		t.Fatalf("backup unexpectedly contains post-backup update")
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM sessions`); got == 0 {
		t.Fatalf("stale refusal ran destructive migration; sessions=%d", got)
	}
}

func TestInspectAcceptsMigratedJournalFirstDatabase(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}

	migrated, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() after migration error = %v", err)
	}
	if migrated.Mode != ModeSQLiteReady {
		t.Fatalf("Inspect().Mode after migration = %q, want %q; diagnostics = %+v", migrated.Mode, ModeSQLiteReady, migrated.Diagnostics)
	}
	if migrated.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("Inspect().SchemaVersion after migration = %d, want %d", migrated.SchemaVersion, CurrentSchemaVersion())
	}
	for _, diagnostic := range migrated.Diagnostics {
		if diagnostic.Code == "schema-version-mismatch" || diagnostic.Code == "schema-checksum-mismatch" || diagnostic.Code == "schema-migration-missing" {
			t.Fatalf("Inspect() after migration reported schema diagnostic %q: %s", diagnostic.Code, diagnostic.Message)
		}
	}
}

func TestApplyJournalFirstMigrationReRunIsCleanNoOp(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err != nil {
		t.Fatalf("first ApplyJournalFirstMigration() error = %v", err)
	}

	// The real command entry point must remain reachable on an already-migrated
	// (post-0011) database: Inspect must report ModeSQLiteReady, not ModeInvalid, so
	// the re-run is an idempotent no-op rather than an "invalid database" error.
	second, err := ApplyJournalFirstMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("second ApplyJournalFirstMigration() error = %v", err)
	}
	if second.JournalEntriesAfter != 5 || second.NoiseEntriesPurged != 0 {
		t.Fatalf("re-run counts after/purged = %d/%d, want 5/0 (noise already purged)", second.JournalEntriesAfter, second.NoiseEntriesPurged)
	}
	if second.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("re-run schema version = %d, want %d", second.SchemaVersion, CurrentSchemaVersion())
	}

	// Dry-run must also stay reachable on a migrated database.
	preview, err := PreviewJournalFirstMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("PreviewJournalFirstMigration() on migrated database error = %v", err)
	}
	if !preview.CopyRun || preview.JournalEntriesAfter != 5 {
		t.Fatalf("preview copy_run/after on migrated database = %t/%d, want true/5", preview.CopyRun, preview.JournalEntriesAfter)
	}

	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries`); got != 5 {
		t.Fatalf("journal_entries after re-run = %d, want 5 (unchanged)", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM schema_migrations WHERE version = 10`); got != 1 {
		t.Fatalf("schema_migrations version 10 rows after re-run = %d, want 1", got)
	}
}

func TestJournalExportSucceedsAfterJournalFirstMigration(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}

	// Spec Test Condition (line 91): `loaf journal export` produces valid
	// markdown and JSONL for a project — including on a migrated database.
	md, err := ExportJournalMarkdown(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ExportJournalMarkdown() after migration error = %v", err)
	}
	if !strings.Contains(md.Content, "tried X abandoned Y next Z") {
		t.Fatalf("markdown export missing surviving wrap synthesis; got:\n%s", md.Content)
	}
	if strings.Contains(md.Content, "=== SESSION STARTED ===") {
		t.Fatalf("markdown export contains purged noise; got:\n%s", md.Content)
	}

	jsonl, err := ExportJournalJSONL(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ExportJournalJSONL() after migration error = %v", err)
	}
	lines := 0
	for _, line := range strings.Split(strings.TrimSpace(jsonl.Content), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("JSONL export line is not valid JSON: %v\nline: %s", err, line)
		}
		lines++
	}
	if lines != 5 {
		t.Fatalf("JSONL export line count after migration = %d, want 5 (surviving journal rows)", lines)
	}
}

func TestBackupVerifiesMigratedJournalFirstDatabase(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)

	if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}

	// Backup of a migrated database must verify, not reject on version.
	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Backup() on migrated database error = %v", err)
	}
	if backup.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("Backup().SchemaVersion = %d, want %d", backup.SchemaVersion, CurrentSchemaVersion())
	}
	verification, err := VerifyBackup(ctx, backup.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() on migrated backup error = %v", err)
	}
	if verification.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("VerifyBackup().SchemaVersion = %d, want %d", verification.SchemaVersion, CurrentSchemaVersion())
	}
}

func TestJournalFirstMigrationExcludedFromAutoApply(t *testing.T) {
	if CurrentSchemaVersion() != 12 {
		t.Fatalf("CurrentSchemaVersion() = %d, want 12 (migration 10 must not auto-apply on store open)", CurrentSchemaVersion())
	}
	for _, m := range SchemaMigrations() {
		if m.Version == journalFirstMigrationVersion {
			t.Fatalf("journal-first migration %d must be excluded from SchemaMigrations()", journalFirstMigrationVersion)
		}
	}
	if got := SchemaMigrations()[len(SchemaMigrations())-1].Version; got <= journalOriginsMigrationVersion {
		t.Fatalf("last auto-applied migration = %d, want a version above %d", got, journalOriginsMigrationVersion)
	}
	m := JournalFirstMigration()
	if m.Version != 10 || m.Name != "journal_first" {
		t.Fatalf("JournalFirstMigration() = %d/%q, want 10/journal_first", m.Version, m.Name)
	}
	if len(m.Checksum()) != 64 {
		t.Fatalf("JournalFirstMigration().Checksum() length = %d, want 64", len(m.Checksum()))
	}
}

// seedSessionPreserveFixture adds, on top of the baseline fixture, session-type
// rows that exercise the safe purge predicate: several machine-generated shapes
// spanning distinct families, one synthetic user-authored session(scope) row
// whose message matches no machine shape, and two NULL-scope rows. The user row
// and both NULL-scope rows must survive as entry_type='legacy_session'; the
// machine rows must be purged.
//
// The NULL-scope rows guard the SQL three-valued-logic hazard: machine writers
// always set scope, so a NULL scope is not provably machine-generated and must
// be preserved — even when the message happens to match a machine shape. Without
// coalescing NULL scope to the empty string, the NOT(...) demotion predicate
// evaluates to NULL (not true) on these rows, so they would slip past step (2a)
// and be destroyed by the unconditional DELETE in step (2b).
func seedSessionPreserveFixture(t *testing.T, databasePath string, projectID string) {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	now := "2026-07-02T00:00:00Z"

	statements := []struct {
		sql  string
		args []any
	}{
		// Machine shapes across several families (all purged).
		{journalInsertSQL, journalArgs("j-mach-resume", projectID, "session", "resume", "=== SESSION RESUMED === (session 96f8726b)", "main", "", nil, now)},
		{journalInsertSQL, journalArgs("j-mach-clear", projectID, "session", "clear", "=== CONTEXT CLEARED ===", "main", "", nil, now)},
		{journalInsertSQL, journalArgs("j-mach-commit", projectID, "session", "end", "at commit 92ae0ef, 3 decisions, 1 entries", "main", "", nil, now)},
		{journalInsertSQL, journalArgs("j-mach-wrapsum", projectID, "session", "wrap", "at commit 13b4436, 6 commits, 12 decisions, 7 entries", "main", "", nil, now)},
		{journalInsertSQL, journalArgs("j-mach-ctx", projectID, "session", "context", "from commit 92ae0ef", "main", "", nil, now)},
		{journalInsertSQL, journalArgs("j-mach-merge", projectID, "session", "merge", "consolidated from 20260422-011533-session.md", "main", "", nil, now)},
		// Synthetic USER-authored session(scope) row: matches NO machine shape.
		// It must be preserved as legacy_session, not purged.
		{journalInsertSQL, journalArgs("j-user-session", projectID, "session", "planning", "handwritten note that happens to use the session type", "main", "", nil, now)},
		// NULL-scope row whose MESSAGE matches a machine shape ("=== SESSION
		// STARTED ==="). A NULL scope is not provably machine-generated, so this
		// row must be PRESERVED regardless of message shape. This is the exact
		// regression the COALESCE(scope,'') hardening fixes.
		{journalInsertSQL, journalArgs("j-null-machinemsg", projectID, "session", "", "=== SESSION STARTED ===", "main", "", nil, now)},
		// NULL-scope row with a plainly non-machine message: also preserved.
		{journalInsertSQL, journalArgs("j-null-human", projectID, "session", "", "a plain thought written with no scope", "main", "", nil, now)},
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed preserve statement %q error = %v", statement.sql, err)
		}
	}
}

// TestJournalFirstMigrationPreservesUnknownSessionRows asserts the safe purge
// predicate: known machine-generated session rows are purged with a per-family
// breakdown, an unknown user-authored session(scope) row is preserved as
// legacy_session (content untouched), and re-running is a clean no-op.
func TestJournalFirstMigrationPreservesUnknownSessionRows(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	stateHome := t.TempDir()
	resolver := PathResolver{StateHome: stateHome}
	status, err := Initialize(ctx, root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	seedJournalFirstFixture(t, status.DatabasePath, status.ProjectID)
	seedSessionPreserveFixture(t, status.DatabasePath, status.ProjectID)
	rebuildJournalSearchFixture(t, status.DatabasePath)

	// Baseline fixture seeds 3 machine session rows; preserve fixture adds 6
	// machine + 1 user row + 2 NULL-scope rows. So 9 purged, 3 preserved as
	// legacy_session (the user row and both NULL-scope rows).
	//
	// Dry-run first: the preview measures counts via the Go family predicates
	// (COALESCE(scope,'')) against a copy and must agree exactly with the SQL
	// apply below. If the Go predicates diverged from the SQL on NULL-scope rows,
	// these counts would differ from the apply's — the precise divergence the
	// COALESCE hardening prevents. This runs against a copy and leaves live state
	// untouched, so the apply below still sees the full unmigrated fixture.
	preview, err := PreviewJournalFirstMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("PreviewJournalFirstMigration() error = %v", err)
	}
	if !preview.CopyRun || preview.Applied {
		t.Fatalf("preview copy_run/applied = %t/%t, want true/false", preview.CopyRun, preview.Applied)
	}
	if preview.NoiseEntriesPurged != 9 {
		t.Fatalf("dry-run noise purged = %d, want 9 (machine rows only)", preview.NoiseEntriesPurged)
	}
	if preview.SessionRowsPreservedAsLegacy != 3 {
		t.Fatalf("dry-run preserved as legacy_session = %d, want 3 (user row + 2 NULL-scope rows)", preview.SessionRowsPreservedAsLegacy)
	}

	result, err := ApplyJournalFirstMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
	}
	if result.NoiseEntriesPurged != 9 {
		t.Fatalf("noise purged = %d, want 9 (machine rows only)", result.NoiseEntriesPurged)
	}
	if result.SessionRowsPreservedAsLegacy != 3 {
		t.Fatalf("preserved as legacy_session = %d, want 3 (user row + 2 NULL-scope rows)", result.SessionRowsPreservedAsLegacy)
	}

	// Per-family breakdown must sum to the purged total and name the right shapes.
	families := map[string]int{}
	total := 0
	for _, family := range result.PurgeBreakdown {
		families[family.Family] = family.Count
		total += family.Count
	}
	if total != result.NoiseEntriesPurged {
		t.Fatalf("purge breakdown sum = %d, want %d", total, result.NoiseEntriesPurged)
	}
	// Baseline: start_marker(1) stop_marker(1) session_ended(1).
	// Preserve:  resume_marker(1) context_cleared(1) commit_summary(2 = end + wrap)
	//            context_arrival(1) merge_consolidated(1).
	wantFamilies := map[string]int{
		"start_marker":       1,
		"stop_marker":        1,
		"session_ended":      1,
		"resume_marker":      1,
		"context_cleared":    1,
		"commit_summary":     2,
		"context_arrival":    1,
		"merge_consolidated": 1,
	}
	for name, want := range wantFamilies {
		if families[name] != want {
			t.Fatalf("family %q count = %d, want %d; full breakdown = %+v", name, families[name], want, result.PurgeBreakdown)
		}
	}

	// No entry_type='session' rows survive; the user row is renamed, not deleted.
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE entry_type = 'session'`); got != 0 {
		t.Fatalf("session-type rows after apply = %d, want 0", got)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE id = 'j-user-session' AND entry_type = 'legacy_session'`); got != 1 {
		t.Fatalf("user session row not preserved as legacy_session; count = %d, want 1", got)
	}
	if got := rawString(t, status.DatabasePath, `SELECT message FROM journal_entries WHERE id = 'j-user-session'`); got != "handwritten note that happens to use the session type" {
		t.Fatalf("preserved row content changed = %q", got)
	}
	// The preserved row is FTS-indexed like any other row.
	if got := rawString(t, status.DatabasePath, `SELECT journal_entry_id FROM journal_search WHERE journal_search MATCH 'handwritten'`); got != "j-user-session" {
		t.Fatalf("FTS match for preserved row = %q, want j-user-session", got)
	}

	// NULL-scope rows must survive as legacy_session — never deleted — even when
	// the message matches a machine shape. This is the SQL 3VL regression guard:
	// a bare `scope = 'start'` on a NULL scope yields NULL, so the NOT(...)
	// demotion would not fire and the DELETE would destroy these rows.
	for _, id := range []string{"j-null-machinemsg", "j-null-human"} {
		if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE id = ? AND entry_type = 'legacy_session'`, id); got != 1 {
			t.Fatalf("NULL-scope row %q not preserved as legacy_session; count = %d, want 1", id, got)
		}
		// Its scope stays NULL (content untouched — only entry_type changed).
		if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE id = ? AND scope IS NULL`, id); got != 1 {
			t.Fatalf("NULL-scope row %q lost its NULL scope; count = %d, want 1", id, got)
		}
	}

	// Idempotent re-run: nothing left to purge or preserve.
	second, err := ApplyJournalFirstMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("second ApplyJournalFirstMigration() error = %v", err)
	}
	if second.NoiseEntriesPurged != 0 || second.SessionRowsPreservedAsLegacy != 0 {
		t.Fatalf("re-run purged/preserved = %d/%d, want 0/0", second.NoiseEntriesPurged, second.SessionRowsPreservedAsLegacy)
	}
	if got := rawCount(t, status.DatabasePath, `SELECT COUNT(*) FROM journal_entries WHERE entry_type = 'legacy_session'`); got != 3 {
		t.Fatalf("legacy_session rows after re-run = %d, want 3 (unchanged)", got)
	}
}

// initializeBehindSchemaFixture creates the global database at an older schema
// version (migrations 1..upToVersion applied, 0008/0009 pending) with a project
// row, then seeds the baseline journal-first fixture. It returns the resolved
// status-shaped project id and database path.
func initializeBehindSchemaFixture(t *testing.T, root project.Root, resolver PathResolver, upToVersion int) (projectID string, databasePath string) {
	t.Helper()
	ctx := context.Background()
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("create database dir error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := ApplyMigrations(ctx, store.db, SchemaMigrations()[:upToVersion]); err != nil {
		t.Fatalf("apply behind-schema migrations error = %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject() error = %v", err)
	}
	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		t.Fatalf("LookupProjectIdentityForRoot() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store error = %v", err)
	}
	seedJournalFirstFixture(t, databasePath, identity.ID)
	return identity.ID, databasePath
}

// TestJournalFirstMigrationRunsOnBehindSchemaDatabase asserts that a database
// stuck below the current schema (0008/0009 pending) — which Inspect marks
// ModeInvalid — is still reachable by both dry-run and apply. Both apply the
// pending non-destructive migrations before the journal-first step and succeed
// end-to-end.
func TestJournalFirstMigrationRunsOnBehindSchemaDatabase(t *testing.T) {
	ctx := context.Background()

	// Dry-run on a behind-schema (v7) database.
	t.Run("dry-run", func(t *testing.T) {
		root := projectRoot(t)
		resolver := PathResolver{StateHome: t.TempDir()}
		_, databasePath := initializeBehindSchemaFixture(t, root, resolver, 7)

		// Guard: Inspect must consider this invalid (pending migrations) so the
		// test exercises the behind-schema gate path, not the ready path.
		pre, err := Inspect(root, resolver)
		if err != nil {
			t.Fatalf("Inspect() error = %v", err)
		}
		if pre.Mode != ModeInvalid || pre.SchemaVersion != 7 {
			t.Fatalf("pre-migration mode/version = %q/%d, want invalid/7", pre.Mode, pre.SchemaVersion)
		}

		preview, err := PreviewJournalFirstMigration(ctx, root, resolver)
		if err != nil {
			t.Fatalf("PreviewJournalFirstMigration() on behind-schema database error = %v", err)
		}
		if !preview.CopyRun || preview.Applied {
			t.Fatalf("preview copy_run/applied = %t/%t, want true/false", preview.CopyRun, preview.Applied)
		}
		if preview.NoiseEntriesPurged != 3 || preview.JournalEntriesAfter != 5 {
			t.Fatalf("preview purged/after = %d/%d, want 3/5", preview.NoiseEntriesPurged, preview.JournalEntriesAfter)
		}
		if preview.SchemaVersion != CurrentSchemaVersion() {
			t.Fatalf("preview schema version = %d, want %d", preview.SchemaVersion, CurrentSchemaVersion())
		}
		// Live database must be untouched by the dry-run: still v7, sessions intact.
		if got := rawTableExists(t, databasePath, "sessions"); !got {
			t.Fatalf("live sessions table missing after dry-run on behind-schema database")
		}
		post, err := Inspect(root, resolver)
		if err != nil {
			t.Fatalf("Inspect() after dry-run error = %v", err)
		}
		if post.SchemaVersion != 7 {
			t.Fatalf("live schema version after dry-run = %d, want 7 (untouched)", post.SchemaVersion)
		}
	})

	// Apply on a behind-schema (v7) database.
	t.Run("apply", func(t *testing.T) {
		root := projectRoot(t)
		resolver := PathResolver{StateHome: t.TempDir()}
		_, databasePath := initializeBehindSchemaFixture(t, root, resolver, 7)

		applied, err := ApplyJournalFirstMigration(ctx, root, resolver)
		if err != nil {
			t.Fatalf("ApplyJournalFirstMigration() on behind-schema database error = %v", err)
		}
		if !applied.Applied || applied.CopyRun {
			t.Fatalf("apply applied/copy_run = %t/%t, want true/false", applied.Applied, applied.CopyRun)
		}
		if applied.BackupPath == "" {
			t.Fatalf("apply backup path is empty; a pre-migration backup is mandatory")
		}
		if applied.NoiseEntriesPurged != 3 || applied.JournalEntriesAfter != 5 {
			t.Fatalf("apply purged/after = %d/%d, want 3/5", applied.NoiseEntriesPurged, applied.JournalEntriesAfter)
		}
		if applied.SchemaVersion != CurrentSchemaVersion() {
			t.Fatalf("apply schema version = %d, want %d", applied.SchemaVersion, CurrentSchemaVersion())
		}
		// Live database is now fully migrated and Inspect-ready.
		final, err := Inspect(root, resolver)
		if err != nil {
			t.Fatalf("Inspect() after apply error = %v", err)
		}
		if final.Mode != ModeSQLiteReady || final.SchemaVersion != CurrentSchemaVersion() {
			t.Fatalf("post-apply mode/version = %q/%d, want sqlite-ready/%d", final.Mode, final.SchemaVersion, CurrentSchemaVersion())
		}
		if rawTableExists(t, databasePath, "sessions") {
			t.Fatalf("sessions table still present after apply on behind-schema database")
		}
		// Pending migrations 8 and 9 were recorded on the way through.
		if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM schema_migrations WHERE version IN (8, 9, 10, 11)`); got != 4 {
			t.Fatalf("recorded migrations 8/9/10/11 = %d, want 4", got)
		}
	})
}

// TestJournalFirstMigrationRefusesGenuinelyInvalidDatabase asserts that a
// database invalid for reasons other than being behind schema (here: checksum
// drift on a recorded migration) is still refused by both dry-run and apply with
// a clear error, rather than being treated as a pending upgrade.
func TestJournalFirstMigrationRefusesGenuinelyInvalidDatabase(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	resolver := PathResolver{StateHome: t.TempDir()}
	_, databasePath := initializeBehindSchemaFixture(t, root, resolver, 7)

	// Corrupt a recorded migration checksum so the database is invalid for a
	// reason unrelated to pending migrations.
	corrupt, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if _, err := corrupt.db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'drifted' WHERE version = 3`); err != nil {
		t.Fatalf("corrupt checksum error = %v", err)
	}
	if err := corrupt.Close(); err != nil {
		t.Fatalf("close store error = %v", err)
	}

	if pre, err := Inspect(root, resolver); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	} else if pre.Mode != ModeInvalid {
		t.Fatalf("mode = %q, want invalid", pre.Mode)
	}

	if _, err := PreviewJournalFirstMigration(ctx, root, resolver); err == nil {
		t.Fatalf("PreviewJournalFirstMigration() on drifted database succeeded, want refusal")
	} else if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("preview refusal error = %q, want it to mention invalid state", err.Error())
	}
	if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err == nil {
		t.Fatalf("ApplyJournalFirstMigration() on drifted database succeeded, want refusal")
	} else if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("apply refusal error = %q, want it to mention invalid state", err.Error())
	}
	// The drifted database must be left untouched by the refusal.
	if got := rawTableExists(t, databasePath, "sessions"); !got {
		t.Fatalf("sessions table missing after refusal; refusal must not mutate")
	}
}

func TestJournalFirstMigrationClassifiesSchema10BeforeMigration11(t *testing.T) {
	ctx := context.Background()

	t.Run("legitimate schema10 advances to schema11", func(t *testing.T) {
		root := projectRoot(t)
		resolver := PathResolver{StateHome: t.TempDir()}
		_, databasePath := initializeBehindSchemaFixture(t, root, resolver, 9)
		store, err := OpenStore(databasePath)
		if err != nil {
			t.Fatalf("OpenStore() error = %v", err)
		}
		if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
			store.Close()
			t.Fatalf("apply migration 10 error = %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		result, err := ApplyJournalFirstMigration(ctx, root, resolver)
		if err != nil {
			t.Fatalf("ApplyJournalFirstMigration() error = %v", err)
		}
		if !result.Applied || result.SchemaVersion != CurrentSchemaVersion() {
			t.Fatalf("result applied/schema = %t/%d, want true/%d", result.Applied, result.SchemaVersion, CurrentSchemaVersion())
		}
	})

	t.Run("drifted schema10 refuses without mutation", func(t *testing.T) {
		root := projectRoot(t)
		resolver := PathResolver{StateHome: t.TempDir()}
		_, databasePath := initializeBehindSchemaFixture(t, root, resolver, 9)
		store, err := OpenStore(databasePath)
		if err != nil {
			t.Fatalf("OpenStore() error = %v", err)
		}
		if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
			store.Close()
			t.Fatalf("apply migration 10 error = %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `UPDATE schema_migrations SET checksum = 'drifted' WHERE version = 10`); err != nil {
			store.Close()
			t.Fatalf("drift migration 10 checksum: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		before, err := os.ReadFile(databasePath)
		if err != nil {
			t.Fatalf("read drifted database before refusal: %v", err)
		}
		if _, err := ApplyJournalFirstMigration(ctx, root, resolver); err == nil {
			t.Fatal("ApplyJournalFirstMigration() error = nil, want schema-10 checksum refusal")
		}
		after, err := os.ReadFile(databasePath)
		if err != nil {
			t.Fatalf("read drifted database after refusal: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("drifted schema-10 database bytes changed after refusal")
		}
		if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM schema_migrations WHERE version = 11`); got != 0 {
			t.Fatalf("migration 11 rows after refusal = %d, want zero", got)
		}
		if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'journal_origins'`); got != 0 {
			t.Fatalf("journal_origins table after refusal = %d, want zero", got)
		}
	})

	for name, mutate := range map[string]func(context.Context, *Store) error{
		"missing current project path": func(ctx context.Context, store *Store) error {
			_, err := store.db.ExecContext(ctx, `DELETE FROM project_paths WHERE is_current = 1`)
			return err
		},
		"divergent journal search": func(ctx context.Context, store *Store) error {
			_, err := store.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = 'j-backfill'`)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := projectRoot(t)
			resolver := PathResolver{StateHome: t.TempDir()}
			_, databasePath := initializeBehindSchemaFixture(t, root, resolver, 9)
			store, err := OpenStore(databasePath)
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			if err := ApplyMigrations(ctx, store.db, []SchemaMigration{JournalFirstMigration()}); err != nil {
				store.Close()
				t.Fatalf("apply migration 10 error = %v", err)
			}
			if err := mutate(ctx, store); err != nil {
				store.Close()
				t.Fatalf("mutate schema10 fixture: %v", err)
			}
			if parity, err := InspectJournalSearchParity(ctx, store); err == nil && name == "divergent journal search" && parity.Ready {
				store.Close()
				t.Fatalf("divergent journal search parity = %#v, want not ready", parity)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			before, err := os.ReadFile(databasePath)
			if err != nil {
				t.Fatalf("read schema10 database before refusal: %v", err)
			}
			beforeSearchRows := rawCount(t, databasePath, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = 'j-backfill'`)
			beforeSearchMessage := ""
			if beforeSearchRows > 0 {
				beforeSearchMessage = rawString(t, databasePath, `SELECT message FROM journal_search WHERE journal_entry_id = 'j-backfill'`)
			}
			beforePathRows := rawCount(t, databasePath, `SELECT COUNT(*) FROM project_paths`)
			applied, applyErr := ApplyJournalFirstMigration(ctx, root, resolver)
			if name == "divergent journal search" {
				if applyErr != nil || !applied.Applied || applied.SchemaVersion != CurrentSchemaVersion() {
					t.Fatalf("ApplyJournalFirstMigration() result=%#v error=%v, want FTS-only divergence repaired", applied, applyErr)
				}
				postStore, openErr := OpenStoreReadOnly(databasePath)
				if openErr != nil {
					t.Fatalf("OpenStoreReadOnly() error = %v", openErr)
				}
				parity, parityErr := InspectJournalSearchParity(ctx, postStore)
				postStore.Close()
				if parityErr != nil || !parity.Ready {
					t.Fatalf("post-journal-first parity=%#v error=%v, want ready", parity, parityErr)
				}
				return
			}
			if applyErr == nil {
				t.Fatal("ApplyJournalFirstMigration() error = nil, want schema10 invariant refusal")
			}
			after, err := os.ReadFile(databasePath)
			if err != nil {
				t.Fatalf("read schema10 database after refusal: %v", err)
			}
			afterSearchRows := rawCount(t, databasePath, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = 'j-backfill'`)
			afterSearchMessage := ""
			if afterSearchRows > 0 {
				afterSearchMessage = rawString(t, databasePath, `SELECT message FROM journal_search WHERE journal_entry_id = 'j-backfill'`)
			}
			afterPathRows := rawCount(t, databasePath, `SELECT COUNT(*) FROM project_paths`)
			if !bytes.Equal(before, after) && (beforeSearchRows != afterSearchRows || beforeSearchMessage != afterSearchMessage || beforePathRows != afterPathRows) {
				t.Fatal("schema10 invariant database tables changed after refusal")
			}
			if beforeSearchRows != afterSearchRows || beforeSearchMessage != afterSearchMessage || beforePathRows != afterPathRows {
				t.Fatalf("schema10 invariant table state changed after refusal: search rows %d/%d message %q/%q paths %d/%d", beforeSearchRows, afterSearchRows, beforeSearchMessage, afterSearchMessage, beforePathRows, afterPathRows)
			}
			if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM schema_migrations WHERE version = 11`); got != 0 {
				t.Fatalf("migration 11 rows after schema10 invariant refusal = %d, want zero", got)
			}
			if got := rawCount(t, databasePath, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'journal_origins'`); got != 0 {
				t.Fatalf("journal_origins table after schema10 invariant refusal = %d, want zero", got)
			}
		})
	}
}

// --- raw read helpers (temp-DB house pattern) ---

func rebuildJournalSearchFixture(t *testing.T, databasePath string) {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin journal-search rebuild: %v", err)
	}
	if _, err := rebuildAndVerifyJournalSearch(context.Background(), tx); err != nil {
		tx.Rollback()
		t.Fatalf("rebuild journal-search fixture: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit journal-search fixture: %v", err)
	}
}

func rawCount(t *testing.T, databasePath string, query string, args ...any) int {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var n int
	if err := store.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func rawString(t *testing.T, databasePath string, query string, args ...any) string {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var s string
	if err := store.db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("string query %q: %v", query, err)
	}
	return s
}

func rawTableExists(t *testing.T, databasePath string, table string) bool {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	exists, err := sqliteTableExists(context.Background(), store.db, table)
	if err != nil {
		t.Fatalf("table exists %s: %v", table, err)
	}
	return exists
}

func rawColumnExists(t *testing.T, databasePath string, table string, column string) bool {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	var n int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		t.Fatalf("pragma_table_info %s: %v", table, err)
	}
	return n > 0
}

func rawForeignKeyViolations(t *testing.T, databasePath string) int {
	t.Helper()
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	rows, err := store.db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	return n
}
