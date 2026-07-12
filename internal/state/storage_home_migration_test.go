package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	storageHomeSIGKILLChildEnv  = "LOAF_STORAGE_HOME_SIGKILL_CHILD"
	storageHomeSIGKILLRootEnv   = "LOAF_STORAGE_HOME_SIGKILL_ROOT"
	storageHomeSIGKILLSignalEnv = "LOAF_STORAGE_HOME_SIGKILL_SIGNAL"
)

func TestPreviewStorageHomeMigrationPlansLegacyCopy(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})

	plan, err := PreviewStorageHomeMigration(root, PathResolver{})
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}

	if plan.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", plan.ContractVersion, StateJSONContractVersion)
	}
	if plan.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", plan.DatabaseScope)
	}
	if plan.MigrationScope != "project" {
		t.Fatalf("MigrationScope = %q, want project", plan.MigrationScope)
	}
	if plan.ProjectID != "" {
		t.Fatalf("ProjectID = %q, want empty before apply", plan.ProjectID)
	}
	if plan.Action != StorageHomeActionCopy {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionCopy)
	}
	if plan.Applied {
		t.Fatal("Applied = true, want false for dry-run")
	}
	if plan.DatabaseExists {
		t.Fatal("DatabaseExists = true, want false before apply")
	}
	if !plan.LegacyDatabaseExists {
		t.Fatal("LegacyDatabaseExists = false, want true")
	}
	if plan.LegacyDatabasePath != legacyPath {
		t.Fatalf("LegacyDatabasePath = %q, want %q", plan.LegacyDatabasePath, legacyPath)
	}
	if !strings.HasPrefix(plan.DatabasePath, dataHome) {
		t.Fatalf("DatabasePath = %q, want under data home %q", plan.DatabasePath, dataHome)
	}
}

func TestApplyStorageHomeMigrationCopiesLegacyDatabaseWithoutDeletingIt(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})

	plan, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	if plan.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", plan.ContractVersion, StateJSONContractVersion)
	}
	if plan.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", plan.DatabaseScope)
	}
	if plan.MigrationScope != "project" {
		t.Fatalf("MigrationScope = %q, want project", plan.MigrationScope)
	}
	if plan.ProjectID == "" {
		t.Fatal("ProjectID is empty after apply")
	}
	if plan.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", plan.ProjectName, filepath.Base(root.Path()))
	}
	if plan.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", plan.ProjectCurrentPath, root.Path())
	}
	if !plan.Applied {
		t.Fatal("Applied = false, want true")
	}
	if plan.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionAlreadyMigrated)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy database stat error = %v, want legacy file preserved", err)
	}
	if _, err := os.Stat(plan.DatabasePath); err != nil {
		t.Fatalf("data-home database stat error = %v", err)
	}

	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q after migration", status.Mode, ModeSQLiteReady)
	}
	if status.DatabasePath != plan.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", status.DatabasePath, plan.DatabasePath)
	}

	second, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("second ApplyStorageHomeMigration() error = %v", err)
	}
	if second.Applied {
		t.Fatal("second Applied = true, want idempotent no-op")
	}
	if second.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("second Action = %q, want %q", second.Action, StorageHomeActionAlreadyMigrated)
	}
	if second.DatabaseScope != "global" || second.MigrationScope != "project" {
		t.Fatalf("second scopes = %q/%q, want global/project", second.DatabaseScope, second.MigrationScope)
	}
}

func TestStorageHomeMigrationUsesProjectDataDatabaseBeforeStateHome(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	projectPath := initializeProjectDataDatabase(t, root, PathResolver{})
	initializeLegacyStateDatabase(t, root, PathResolver{})

	plan, err := PreviewStorageHomeMigration(root, PathResolver{})
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}
	if plan.Action != StorageHomeActionCopy {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionCopy)
	}
	if plan.DatabaseScope != "global" || plan.MigrationScope != "project" {
		t.Fatalf("scopes = %q/%q, want global/project", plan.DatabaseScope, plan.MigrationScope)
	}
	if plan.LegacyDatabasePath != projectPath {
		t.Fatalf("LegacyDatabasePath = %q, want project data path %q", plan.LegacyDatabasePath, projectPath)
	}

	applied, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	if !applied.Applied {
		t.Fatal("Applied = false, want true")
	}
	if applied.ProjectID == "" {
		t.Fatal("ProjectID is empty after project database migration")
	}
	if applied.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", applied.ProjectCurrentPath, root.Path())
	}
	if applied.DatabasePath == projectPath {
		t.Fatalf("DatabasePath = %q, want global path distinct from project path", applied.DatabasePath)
	}
	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
}

func TestApplyStorageHomeMigrationUpgradesCopiedLegacySchema(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	resolver := PathResolver{}
	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		t.Fatalf("LegacyDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("create legacy database dir error = %v", err)
	}
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if err := ApplyMigrations(ctx, legacyStore.db, SchemaMigrations()[:1]); err != nil {
		t.Fatalf("apply legacy schema migration error = %v", err)
	}
	if err := legacyStore.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("close legacy store error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	if !plan.Applied {
		t.Fatal("Applied = false, want true")
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy database stat error = %v, want legacy file preserved", err)
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	if status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentSchemaVersion())
	}
}

func TestApplyStorageHomeMigrationDoesNotOverwriteExistingDataHomeDatabase(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	initializeLegacyStateDatabase(t, root, PathResolver{})
	dataStatus, err := Initialize(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("Initialize(data) error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	if plan.Action != StorageHomeActionAlreadyMigrated {
		t.Fatalf("Action = %q, want %q", plan.Action, StorageHomeActionAlreadyMigrated)
	}
	if plan.DatabasePath != dataStatus.DatabasePath {
		t.Fatalf("DatabasePath = %q, want existing data path %q", plan.DatabasePath, dataStatus.DatabasePath)
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("Warnings empty, want legacy-leftover warning")
	}
}

func TestApplyStorageHomeMigrationIncludesPendingWALFrames(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	defer legacyStore.Close()

	if _, err := legacyStore.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatalf("wal checkpoint error = %v", err)
	}
	wantProjectID := "pending-wal-project"
	wantProjectPath := filepath.Join(root.Path(), "pending-wal-project")
	if _, err := legacyStore.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, wantProjectID, wantProjectID, "Pending WAL Project", wantProjectPath, "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z"); err != nil {
		t.Fatalf("insert WAL-backed project error = %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)
`, "pending-wal-project-path", wantProjectID, wantProjectPath, "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z", "2026-06-12T00:00:00Z"); err != nil {
		t.Fatalf("insert WAL-backed project path error = %v", err)
	}
	if info, err := os.Stat(legacyPath + "-wal"); err != nil {
		t.Fatalf("legacy WAL stat error = %v", err)
	} else if info.Size() == 0 {
		t.Fatal("legacy WAL is empty, want pending frames before migration")
	}

	plan, err := ApplyStorageHomeMigration(ctx, root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	dataStore, err := OpenStore(plan.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(data) error = %v", err)
	}
	defer dataStore.Close()
	var count int
	if err := dataStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE id = ?`, wantProjectID).Scan(&count); err != nil {
		t.Fatalf("query migrated project error = %v", err)
	}
	if count != 1 {
		t.Fatalf("migrated project count = %d, want 1", count)
	}
}

func TestApplyStorageHomeMigrationCopyRebuildsExactJournalSearchWithoutLegacyContamination(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	for _, message := range []string{"legacy-one", "legacy-two", "legacy-three"} {
		if _, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(storage): " + message}); err != nil {
			legacyStore.Close()
			t.Fatalf("LogJournal(%s) error = %v", message, err)
		}
	}
	var firstID, secondID string
	if err := legacyStore.db.QueryRowContext(ctx, `SELECT id FROM journal_entries ORDER BY rowid LIMIT 1`).Scan(&firstID); err != nil {
		legacyStore.Close()
		t.Fatalf("read first legacy journal id: %v", err)
	}
	if err := legacyStore.db.QueryRowContext(ctx, `SELECT id FROM journal_entries ORDER BY rowid LIMIT 1 OFFSET 1`).Scan(&secondID); err != nil {
		legacyStore.Close()
		t.Fatalf("read second legacy journal id: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, firstID); err != nil {
		legacyStore.Close()
		t.Fatalf("delete legacy derived row: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `UPDATE journal_search SET message = 'legacy-stale' WHERE journal_entry_id = ?`, secondID); err != nil {
		legacyStore.Close()
		t.Fatalf("change legacy derived row: %v", err)
	}
	correlationColumn, err := journalSearchCorrelationColumn(ctx, legacyStore)
	if err != nil {
		legacyStore.Close()
		t.Fatalf("read legacy correlation column: %v", err)
	}
	rogueSQL := fmt.Sprintf(`
INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
VALUES ((SELECT COALESCE(MAX(rowid), 0) + 1 FROM journal_search), 'rogue-project', 'rogue-entry', '', 'rogue', '', 'rogue')`, correlationColumn)
	if _, err := legacyStore.db.ExecContext(ctx, rogueSQL); err != nil {
		legacyStore.Close()
		t.Fatalf("insert legacy rogue derived row: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}

	plan, err := ApplyStorageHomeMigration(ctx, root, PathResolver{})
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	store, err := OpenStore(plan.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(copied) error = %v", err)
	}
	defer store.Close()
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity(copied) error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 3 || parity.IndexRows != 3 {
		t.Fatalf("copied journal parity = %#v, want three exact ready rows", parity)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = 'rogue-entry'`); got != 0 {
		t.Fatalf("copied rogue derived rows = %d, want zero", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM journal_search WHERE message = 'legacy-stale'`); got != 0 {
		t.Fatalf("copied stale derived rows = %d, want zero", got)
	}
}

func TestApplyStorageHomeMigrationMergeRebuildsGlobalJournalSearchAfterCanonicalCopy(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath(global) error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if _, err := globalStore.LogJournal(ctx, otherRoot, JournalLogOptions{Entry: "decision(storage): global-row"}); err != nil {
		globalStore.Close()
		t.Fatalf("LogJournal(global) error = %v", err)
	}
	globalCorrelationColumn, err := journalSearchCorrelationColumn(ctx, globalStore)
	if err != nil {
		globalStore.Close()
		t.Fatalf("read global correlation column: %v", err)
	}
	globalRogueSQL := fmt.Sprintf(`
INSERT INTO journal_search(rowid, project_id, journal_entry_id, %s, entry_type, scope, message)
VALUES ((SELECT COALESCE(MAX(rowid), 0) + 1 FROM journal_search), 'rogue-global', 'rogue-global-entry', '', 'rogue', '', 'rogue-global')`, globalCorrelationColumn)
	if _, err := globalStore.db.ExecContext(ctx, globalRogueSQL); err != nil {
		globalStore.Close()
		t.Fatalf("seed global rogue derived row: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}

	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		t.Fatalf("LegacyDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("create legacy database directory: %v", err)
	}
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if err := ApplyMigrations(ctx, legacyStore.db, SchemaMigrations()[:1]); err != nil {
		legacyStore.Close()
		t.Fatalf("apply legacy v1 migration: %v", err)
	}
	legacyID := ProjectID(root)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := legacyStore.db.ExecContext(ctx, `INSERT INTO projects (id, identity_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`, legacyID, legacyID, now, now); err != nil {
		legacyStore.Close()
		t.Fatalf("insert legacy hash project: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy v1) error = %v", err)
	}
	legacyStore, err = OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("reopen legacy error = %v", err)
	}
	if err := legacyStore.ApplyMigrations(ctx); err != nil {
		legacyStore.Close()
		t.Fatalf("upgrade legacy database: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `
UPDATE projects SET friendly_name = 'Legacy Hash', current_path = ?, last_seen_at = ? WHERE id = ?`, root.Path(), now, legacyID); err != nil {
		legacyStore.Close()
		t.Fatalf("set legacy project path: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES ('legacy-root-path', ?, ?, 1, ?, ?, ?, ?)`, legacyID, root.Path(), now, now, now, now); err != nil {
		legacyStore.Close()
		t.Fatalf("insert legacy project path: %v", err)
	}
	for _, message := range []string{"merge-one", "merge-two"} {
		if _, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(storage): " + message}); err != nil {
			legacyStore.Close()
			t.Fatalf("LogJournal(legacy %s) error = %v", message, err)
		}
	}
	var legacyJournalID string
	if err := legacyStore.db.QueryRowContext(ctx, `SELECT id FROM journal_entries ORDER BY rowid LIMIT 1`).Scan(&legacyJournalID); err != nil {
		legacyStore.Close()
		t.Fatalf("read legacy journal id: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `DELETE FROM journal_search WHERE journal_entry_id = ?`, legacyJournalID); err != nil {
		legacyStore.Close()
		t.Fatalf("delete legacy derived row: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy final) error = %v", err)
	}

	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration(merge) error = %v", err)
	}
	if plan.Action != StorageHomeActionMerge {
		t.Fatalf("merge plan action = %q, want %q", plan.Action, StorageHomeActionMerge)
	}
	merged, err := ApplyStorageHomeMigration(ctx, root, resolver)
	if err != nil {
		t.Fatalf("ApplyStorageHomeMigration(merge) error = %v", err)
	}
	mergedStore, err := OpenStore(merged.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore(merged) error = %v", err)
	}
	defer mergedStore.Close()
	parity, err := InspectJournalSearchParity(ctx, mergedStore)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity(merged) error = %v", err)
	}
	if !parity.Ready || parity.CanonicalRows != 3 || parity.IndexRows != 3 {
		t.Fatalf("merged journal parity = %#v, want three exact ready rows", parity)
	}
	if got := countIdentityRows(t, mergedStore, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id LIKE 'rogue-%'`); got != 0 {
		t.Fatalf("merged rogue derived rows = %d, want zero", got)
	}
}

func TestApplyProjectDatabaseMergePreservesJournalProvenance(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	dirty := false
	reconstructable := false
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(merge): provenance", Origin: &JournalOriginInput{EnvelopeVersion: 2, CaptureMechanism: "future", SourceEvent: "merge", ChangePath: "docs/merge.md", ChangeSHA256: strings.Repeat("a", 64), Head: "abc123", Dirty: &dirty, Reconstructable: &reconstructable}})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	deferred, err := legacyStore.DeferJournal(ctx, root, JournalDeferOptions{Intent: "merge intent", Why: "merge reason", Boundary: "merge boundary", Trigger: "merge trigger", OperationID: "merge-op"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("DeferJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	merged, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath})
	if err != nil {
		status, _ := Inspect(root, resolver)
		t.Fatalf("ApplyProjectDatabaseMerge() error = %v (status=%#v diagnostics=%#v)", err, status, status.Diagnostics)
	}
	if !merged.Applied {
		t.Fatal("ApplyProjectDatabaseMerge() Applied = false, want true")
	}
	globalStore, err = OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global after merge) error = %v", err)
	}
	defer globalStore.Close()
	var mechanism, sourceEvent, changePath, changeDigest string
	var envelope int
	if err := globalStore.db.QueryRowContext(ctx, `SELECT envelope_version, capture_mechanism, source_event, change_path, change_sha256 FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, logged.ProjectID, logged.ID).Scan(&envelope, &mechanism, &sourceEvent, &changePath, &changeDigest); err != nil {
		t.Fatalf("read merged origin: %v", err)
	}
	if envelope != 2 || mechanism != "future" || sourceEvent != "merge" || changePath != "docs/merge.md" || changeDigest != strings.Repeat("a", 64) {
		t.Fatalf("merged origin = %d/%q/%q/%q/%q, want exact source fields", envelope, mechanism, sourceEvent, changePath, changeDigest)
	}
	var gotJournal, gotSpark, gotDigest string
	if err := globalStore.db.QueryRowContext(ctx, `SELECT journal_entry_id, spark_id, stored_digest FROM journal_deferrals WHERE project_id = ? AND operation_key = ?`, logged.ProjectID, "merge-op").Scan(&gotJournal, &gotSpark, &gotDigest); err != nil {
		t.Fatalf("read merged deferral: %v", err)
	}
	if gotJournal != deferred.Decision.ID || gotSpark != deferred.Spark.ID || gotDigest != deferred.StoredDigest {
		t.Fatalf("merged deferral = %q/%q/%q, want original pair/digest", gotJournal, gotSpark, gotDigest)
	}
	parity, err := InspectJournalSearchParity(ctx, globalStore)
	if err != nil || !parity.Ready {
		t.Fatalf("merged journal parity = %#v, err=%v, want ready", parity, err)
	}
}

func TestApplyProjectDatabaseMergeBackfillsUnknownOriginWhenSourceTableAbsent(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(merge): unknown origin"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	var sourceFriendlyName, sourceCurrentPath, sourceLastSeenAt, sourceCreatedAt, sourceUpdatedAt string
	var sourcePathID, sourcePathFirstSeenAt, sourcePathLastSeenAt, sourcePathCreatedAt, sourcePathUpdatedAt string
	if err := legacyStore.db.QueryRowContext(ctx, `SELECT COALESCE(friendly_name, ''), COALESCE(current_path, ''), COALESCE(last_seen_at, ''), created_at, updated_at FROM projects WHERE id = ?`, logged.ProjectID).Scan(&sourceFriendlyName, &sourceCurrentPath, &sourceLastSeenAt, &sourceCreatedAt, &sourceUpdatedAt); err != nil {
		legacyStore.Close()
		t.Fatalf("read source project identity: %v", err)
	}
	if err := legacyStore.db.QueryRowContext(ctx, `SELECT id, first_seen_at, last_seen_at, created_at, updated_at FROM project_paths WHERE project_id = ? AND is_current = 1`, logged.ProjectID).Scan(&sourcePathID, &sourcePathFirstSeenAt, &sourcePathLastSeenAt, &sourcePathCreatedAt, &sourcePathUpdatedAt); err != nil {
		legacyStore.Close()
		t.Fatalf("read source project path: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `DROP TABLE journal_origins`); err != nil {
		legacyStore.Close()
		t.Fatalf("drop legacy journal_origins: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	preexistingJournalID := "preexisting-nil-origin"
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global before merge) error = %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, logged.ProjectID, logged.ProjectID, sourceFriendlyName, sourceCurrentPath, sourceLastSeenAt, sourceCreatedAt, sourceUpdatedAt); err != nil {
		globalStore.Close()
		t.Fatalf("insert preexisting project: %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `
INSERT INTO journal_entries (id, project_id, entry_type, scope, message, observed_branch, observed_worktree, harness_session_id, spec_id, task_id, created_at, updated_at)
VALUES (?, ?, 'decision', 'merge', 'preexisting nil origin', NULL, NULL, NULL, NULL, NULL, ?, ?)`, preexistingJournalID, logged.ProjectID, sourceCreatedAt, sourceUpdatedAt); err != nil {
		globalStore.Close()
		t.Fatalf("insert preexisting journal: %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)`, sourcePathID, logged.ProjectID, root.Path(), sourcePathFirstSeenAt, sourcePathLastSeenAt, sourcePathCreatedAt, sourcePathUpdatedAt); err != nil {
		globalStore.Close()
		t.Fatalf("insert preexisting project path: %v", err)
	}
	rebuildTx, err := globalStore.db.BeginTx(ctx, nil)
	if err != nil {
		globalStore.Close()
		t.Fatalf("begin global search rebuild: %v", err)
	}
	if _, err := rebuildAndVerifyJournalSearch(ctx, rebuildTx); err != nil {
		rebuildTx.Rollback()
		globalStore.Close()
		t.Fatalf("rebuild global search: %v", err)
	}
	if err := rebuildTx.Commit(); err != nil {
		globalStore.Close()
		t.Fatalf("commit global search rebuild: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global before merge) error = %v", err)
	}
	merged, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath})
	if err != nil {
		t.Fatalf("ApplyProjectDatabaseMerge() error = %v", err)
	}
	if !merged.Applied {
		t.Fatal("ApplyProjectDatabaseMerge() Applied = false, want true")
	}
	globalStore, err = OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	defer globalStore.Close()
	var mechanism, branch, worktree, session string
	if err := globalStore.db.QueryRowContext(ctx, `
SELECT capture_mechanism, COALESCE(branch, ''), COALESCE(worktree, ''), COALESCE(harness_session_id, '')
FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, logged.ProjectID, logged.ID).Scan(&mechanism, &branch, &worktree, &session); err != nil {
		t.Fatalf("read backfilled origin: %v", err)
	}
	if mechanism != "unknown" || branch != "" || worktree != "" || session != "" {
		t.Fatalf("backfilled origin = %q/%q/%q/%q, want unknown with only observable empty fields", mechanism, branch, worktree, session)
	}
	var preexistingOriginCount int
	if err := globalStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, logged.ProjectID, preexistingJournalID).Scan(&preexistingOriginCount); err != nil {
		t.Fatalf("count preexisting origin: %v", err)
	}
	if preexistingOriginCount != 0 {
		t.Fatalf("preexisting nil-origin row count = %d, want zero", preexistingOriginCount)
	}
}

func TestApplyProjectDatabaseMergePreservesMissingOriginWhenSourceTableExists(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(merge): nil origin"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `DELETE FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, logged.ProjectID, logged.ID); err != nil {
		legacyStore.Close()
		t.Fatalf("delete legacy origin: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}); err != nil {
		t.Fatalf("ApplyProjectDatabaseMerge() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	defer globalStore.Close()
	var count int
	if err := globalStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_origins WHERE project_id = ? AND journal_entry_id = ?`, logged.ProjectID, logged.ID).Scan(&count); err != nil {
		t.Fatalf("count preserved missing origin: %v", err)
	}
	if count != 0 {
		t.Fatalf("preserved missing origin count = %d, want zero", count)
	}
}

func TestApplyProjectDatabaseMergeRejectsChangedSourceMappingWithoutCopy(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(mapping): source"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `UPDATE project_paths SET is_current = 0 WHERE path = ?`, root.Path()); err != nil {
		legacyStore.Close()
		t.Fatalf("change source project mapping: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `UPDATE projects SET current_path = ? WHERE id = ?`, filepath.Join(root.Path(), "moved"), logged.ProjectID); err != nil {
		legacyStore.Close()
		t.Fatalf("change source current path: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}); err == nil {
		t.Fatal("ApplyProjectDatabaseMerge() error = nil, want changed source mapping failure")
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	defer globalStore.Close()
	var count int
	if err := globalStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = ?`, logged.ID).Scan(&count); err != nil {
		t.Fatalf("count source journal after mapping failure: %v", err)
	}
	if count != 0 {
		t.Fatalf("source journal rows after mapping failure = %d, want zero", count)
	}
}

func TestApplyProjectDatabaseMergeRejectsPathOwnedByAnotherProject(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(collision): path"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	otherID := projectIDForTest(t, globalStore, otherRoot)
	if _, err := globalStore.db.ExecContext(ctx, `INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at) VALUES ('collision-root-path', ?, ?, 0, ?, ?, ?, ?)`, otherID, root.Path(), "2026-07-11T00:00:00Z", "2026-07-11T00:00:00Z", "2026-07-11T00:00:00Z", "2026-07-11T00:00:00Z"); err != nil {
		globalStore.Close()
		t.Fatalf("insert conflicting root path: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}); err == nil {
		t.Fatal("ApplyProjectDatabaseMerge() error = nil, want path collision failure")
	}
	globalStore, err = OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global after failure) error = %v", err)
	}
	defer globalStore.Close()
	var count int
	if err := globalStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = ?`, logged.ID).Scan(&count); err != nil {
		t.Fatalf("count path-collision source journal: %v", err)
	}
	if count != 0 {
		t.Fatalf("path-collision source journal rows = %d, want zero", count)
	}
}

func TestApplyProjectDatabaseMergeRejectsCrossProjectEntityCollision(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(collision): entity"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	otherID := projectIDForTest(t, globalStore, otherRoot)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := globalStore.db.ExecContext(ctx, `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at) VALUES (?, ?, 'decision', 'collision', 'other project row', ?, ?)`, logged.ID, otherID, now, now); err != nil {
		globalStore.Close()
		t.Fatalf("insert cross-project entity collision: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}); err == nil {
		t.Fatal("ApplyProjectDatabaseMerge() error = nil, want cross-project entity collision failure")
	}
	globalStore, err = OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global after failure) error = %v", err)
	}
	defer globalStore.Close()
	var projectID, message string
	if err := globalStore.db.QueryRowContext(ctx, `SELECT project_id, message FROM journal_entries WHERE id = ?`, logged.ID).Scan(&projectID, &message); err != nil {
		t.Fatalf("read preserved cross-project row: %v", err)
	}
	if projectID != otherID || message != "other project row" {
		t.Fatalf("preserved cross-project row = %q/%q, want %q/%q", projectID, message, otherID, "other project row")
	}
}

func TestApplyProjectDatabaseMergeRejectsDifferingSameProjectRow(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(collision): differing"})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	projectRow := readMigrationProjectRow(t, legacyStore, logged.ProjectID)
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, projectRow.ID, projectRow.IdentityHash, projectRow.FriendlyName, projectRow.CurrentPath, projectRow.LastSeenAt, projectRow.CreatedAt, projectRow.UpdatedAt); err != nil {
		globalStore.Close()
		t.Fatalf("insert same-project identity row: %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `INSERT INTO journal_entries (id, project_id, entry_type, scope, message, created_at, updated_at) VALUES (?, ?, 'decision', 'collision', 'different destination row', ?, ?)`, logged.ID, logged.ProjectID, projectRow.CreatedAt, projectRow.UpdatedAt); err != nil {
		globalStore.Close()
		t.Fatalf("insert differing same-project row: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}); err == nil {
		t.Fatal("ApplyProjectDatabaseMerge() error = nil, want differing same-project row failure")
	}
	globalStore, err = OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global after failure) error = %v", err)
	}
	defer globalStore.Close()
	var message string
	if err := globalStore.db.QueryRowContext(ctx, `SELECT message FROM journal_entries WHERE id = ?`, logged.ID).Scan(&message); err != nil {
		t.Fatalf("read preserved differing row: %v", err)
	}
	if message != "different destination row" {
		t.Fatalf("preserved differing row message = %q, want destination value", message)
	}
}

func TestApplyProjectDatabaseMergeIdenticalRetryIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(retry): identical", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual"}})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	plan := StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, plan); err != nil {
		t.Fatalf("first ApplyProjectDatabaseMerge() error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, plan); err != nil {
		t.Fatalf("identical retry ApplyProjectDatabaseMerge() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	defer globalStore.Close()
	var count int
	if err := globalStore.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_entries WHERE id = ?`, logged.ID).Scan(&count); err != nil {
		t.Fatalf("count idempotent journal: %v", err)
	}
	if count != 1 {
		t.Fatalf("idempotent journal rows = %d, want 1", count)
	}
}

func TestApplyProjectDatabaseMergeRejectsSamePathProjectWithDifferentRowID(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if _, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(path): row id"}); err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	plan := StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, plan); err != nil {
		t.Fatalf("first ApplyProjectDatabaseMerge() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `UPDATE project_paths SET id = 'changed-path-row-id' WHERE path = ?`, root.Path()); err != nil {
		globalStore.Close()
		t.Fatalf("change project path row id: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, plan); err == nil {
		t.Fatal("identical path retry error = nil, want different row ID conflict")
	}
}

func TestApplyProjectDatabaseMergeRejectsSamePathRowWithDifferentTimestamps(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if _, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(path): timestamps"}); err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	plan := StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, plan); err != nil {
		t.Fatalf("first ApplyProjectDatabaseMerge() error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `UPDATE project_paths SET last_seen_at = 'different-timestamp' WHERE path = ?`, root.Path()); err != nil {
		globalStore.Close()
		t.Fatalf("change project path timestamp: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	if _, err := ApplyProjectDatabaseMerge(ctx, root, resolver, plan); err == nil {
		t.Fatal("timestamp-different path retry error = nil, want complete-row conflict")
	}
}

func TestApplyProjectDatabaseMergeRollsBackAllChangesOnInjectedBeforeCommitFailure(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	logged, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(atomic): rollback", Origin: &JournalOriginInput{EnvelopeVersion: 1, CaptureMechanism: "manual"}})
	if err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	ops := &projectDatabaseMergeOps{beforeCommit: func(context.Context, *sql.Tx) error { return errors.New("injected merge failure") }}
	if _, err := applyProjectDatabaseMergeWithOps(ctx, root, resolver, StorageHomeMigrationPlan{DatabasePath: globalPath, LegacyDatabasePath: legacyPath}, ops); err == nil {
		t.Fatal("ApplyProjectDatabaseMerge() error = nil, want injected before-commit failure")
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global after rollback) error = %v", err)
	}
	defer globalStore.Close()
	for name, query := range map[string]string{
		"project": `SELECT COUNT(*) FROM projects WHERE id = ?`,
		"path":    `SELECT COUNT(*) FROM project_paths WHERE path = ?`,
		"journal": `SELECT COUNT(*) FROM journal_entries WHERE id = ?`,
		"origin":  `SELECT COUNT(*) FROM journal_origins WHERE journal_entry_id = ?`,
		"search":  `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = ?`,
	} {
		var count int
		value := logged.ProjectID
		if err := globalStore.db.QueryRowContext(ctx, query, func() string {
			if name == "path" {
				return root.Path()
			}
			if name == "journal" || name == "origin" || name == "search" {
				return logged.ID
			}
			return value
		}()).Scan(&count); err != nil {
			globalStore.Close()
			t.Fatalf("count rolled-back %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("rolled-back %s rows = %d, want zero", name, count)
		}
	}
}

func TestApplyStorageHomeMigrationCopyParityFailureRemovesDestination(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if _, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(storage): original"}); err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal() error = %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `DROP TABLE journal_search`); err != nil {
		legacyStore.Close()
		t.Fatalf("drop legacy journal_search: %v", err)
	}
	if _, err := legacyStore.db.ExecContext(ctx, `
CREATE TABLE journal_search (
  rowid INTEGER PRIMARY KEY,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  session_id TEXT,
  entry_type TEXT NOT NULL,
  scope TEXT NOT NULL,
  message TEXT NOT NULL CHECK(message = 'prior')
)`); err != nil {
		legacyStore.Close()
		t.Fatalf("create failing legacy journal_search: %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	plan, err := PreviewStorageHomeMigration(root, PathResolver{})
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}
	if plan.Action != StorageHomeActionCopy {
		t.Fatalf("plan action = %q, want %q", plan.Action, StorageHomeActionCopy)
	}
	if _, err := ApplyStorageHomeMigration(ctx, root, PathResolver{}); err == nil {
		t.Fatal("ApplyStorageHomeMigration() error = nil, want copied parity failure")
	}
	if _, err := os.Stat(plan.DatabasePath); !os.IsNotExist(err) {
		t.Fatalf("copied destination stat error = %v, want destination removed", err)
	}
}

func TestApplyStorageHomeMigrationCopyFailuresNeverPublish(t *testing.T) {
	for _, test := range []struct {
		name string
		ops  func(error) *storageHomeMigrationCopyOps
	}{
		{"after-copy", func(injected error) *storageHomeMigrationCopyOps {
			return &storageHomeMigrationCopyOps{afterCopy: func(string) error { return injected }}
		}},
		{"before-publish", func(injected error) *storageHomeMigrationCopyOps {
			return &storageHomeMigrationCopyOps{beforePublish: func(string) error { return injected }}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			root := projectRoot(t)
			dataHome := t.TempDir()
			stateHome := t.TempDir()
			t.Setenv("XDG_DATA_HOME", dataHome)
			t.Setenv("XDG_STATE_HOME", stateHome)
			resolver := PathResolver{DataHome: dataHome}
			legacyPath := initializeLegacyStateDatabase(t, root, resolver)
			before, err := os.ReadFile(legacyPath)
			if err != nil {
				t.Fatalf("read legacy before failure: %v", err)
			}
			plan, err := PreviewStorageHomeMigration(root, resolver)
			if err != nil {
				t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
			}
			injected := errors.New("injected copy publication failure")
			if _, err := applyStorageHomeMigrationWithOps(ctx, root, resolver, test.ops(injected)); !errors.Is(err, injected) {
				t.Fatalf("applyStorageHomeMigrationWithOps() error = %v, want injected failure", err)
			}
			if _, err := os.Stat(plan.DatabasePath); !os.IsNotExist(err) {
				t.Fatalf("final database stat = %v, want absent", err)
			}
			after, err := os.ReadFile(legacyPath)
			if err != nil {
				t.Fatalf("read legacy after failure: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("legacy database bytes changed after failed copy")
			}
			assertNoStorageHomeStagingFiles(t, filepath.Dir(plan.DatabasePath))
		})
	}
}

func TestApplyStorageHomeMigrationConcurrentPublishersPreserveWinner(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{DataHome: dataHome}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyBefore, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy before publishers: %v", err)
	}
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	firstPublished := make(chan struct{})
	infos := make([]os.FileInfo, 2)
	var infoMu sync.Mutex
	results := make(chan error, 2)
	for index := 0; index < 2; index++ {
		index := index
		go func() {
			ops := &storageHomeMigrationCopyOps{beforePublish: func(stagingPath string) error {
				info, statErr := os.Stat(stagingPath)
				if statErr != nil {
					return statErr
				}
				infoMu.Lock()
				infos[index] = info
				infoMu.Unlock()
				ready <- struct{}{}
				<-release
				return nil
			}}
			if index == 0 {
				ops.publish = func(stagingPath, databasePath string) (bool, error) {
					published, publishErr := publishStorageHomeStaging(stagingPath, databasePath)
					close(firstPublished)
					return published, publishErr
				}
			} else {
				ops.publish = func(stagingPath, databasePath string) (bool, error) {
					<-firstPublished
					return publishStorageHomeStaging(stagingPath, databasePath)
				}
			}
			_, applyErr := applyStorageHomeMigrationWithOps(ctx, root, resolver, ops)
			results <- applyErr
		}()
	}
	<-ready
	<-ready
	close(release)
	for index := 0; index < 2; index++ {
		if err := <-results; err != nil {
			t.Fatalf("publisher %d error = %v", index, err)
		}
	}
	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration(after publish) error = %v", err)
	}
	finalInfo, err := os.Stat(plan.DatabasePath)
	if err != nil {
		t.Fatalf("stat final database: %v", err)
	}
	if !os.SameFile(finalInfo, infos[0]) {
		t.Fatal("second publisher overwrote the first publisher inode")
	}
	if _, err := verifyStorageHomeDestination(ctx, root, plan.DatabasePath); err != nil {
		t.Fatalf("verify final winner: %v", err)
	}
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy after publishers: %v", err)
	}
	if !bytes.Equal(legacyAfter, legacyBefore) {
		t.Fatal("legacy database bytes changed during concurrent publish")
	}
	assertNoStorageHomeStagingFiles(t, filepath.Dir(plan.DatabasePath))
}

func TestApplyStorageHomeMigrationSIGKILLBeforePublishLeavesFinalAbsentAndRetryable(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" {
		t.Skip("SIGKILL semantics are unavailable on this platform")
	}
	ctx := context.Background()
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{DataHome: dataHome}
	legacyPath := initializeLegacyStateDatabase(t, root, resolver)
	legacyStore, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if _, err := legacyStore.LogJournal(ctx, root, JournalLogOptions{Entry: "decision(storage-sigkill): durable legacy payload"}); err != nil {
		legacyStore.Close()
		t.Fatalf("LogJournal(legacy) error = %v", err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	legacyBefore, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy before SIGKILL: %v", err)
	}
	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}
	signalPath := filepath.Join(t.TempDir(), "ready")
	child := exec.Command(os.Args[0], "-test.run=^TestApplyStorageHomeMigrationSIGKILLChild$", "-test.v")
	child.Env = append(os.Environ(),
		storageHomeSIGKILLChildEnv+"=1",
		storageHomeSIGKILLRootEnv+"="+root.Path(),
		storageHomeSIGKILLSignalEnv+"="+signalPath,
		"XDG_DATA_HOME="+dataHome,
		"XDG_STATE_HOME="+stateHome,
	)
	var childOutput bytes.Buffer
	child.Stdout = &childOutput
	child.Stderr = &childOutput
	if err := child.Start(); err != nil {
		t.Fatalf("start SIGKILL child: %v", err)
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- child.Wait() }()
	waitForStorageHomeSignal(t, signalPath, waitDone, &childOutput)
	if _, err := os.Stat(plan.DatabasePath); !os.IsNotExist(err) {
		_ = child.Process.Kill()
		t.Fatalf("final database before publish stat = %v, want absent", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatalf("SIGKILL child: %v", err)
	}
	if err := <-waitDone; err == nil {
		t.Fatal("SIGKILL child exited successfully")
	}
	preview, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration(after kill) error = %v", err)
	}
	if preview.Action != StorageHomeActionCopy || preview.DatabaseExists {
		t.Fatalf("preview after kill = %#v, want legacy copy with final absent", preview)
	}
	legacyAfter, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("read legacy after SIGKILL: %v", err)
	}
	if !bytes.Equal(legacyAfter, legacyBefore) {
		t.Fatal("legacy database bytes changed across SIGKILL")
	}
	if _, err := ApplyStorageHomeMigration(ctx, root, resolver); err != nil {
		t.Fatalf("ApplyStorageHomeMigration(retry) error = %v", err)
	}
	store, err := OpenStoreReadOnly(plan.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(final) error = %v", err)
	}
	defer store.Close()
	search, err := store.SearchJournal(ctx, root, SearchOptions{Query: "durable legacy payload"})
	if err != nil || len(search.Results) != 1 {
		t.Fatalf("SearchJournal(final) = %#v, err=%v, want retrievable payload", search.Results, err)
	}
	provenance, err := InspectJournalProvenanceIntegrity(ctx, store)
	if err != nil || !provenance.Ready {
		t.Fatalf("final provenance = %#v, err=%v, want ready", provenance, err)
	}
}

func TestApplyStorageHomeMigrationSIGKILLChild(t *testing.T) {
	if os.Getenv(storageHomeSIGKILLChildEnv) != "1" {
		return
	}
	root, err := project.ResolveRoot(os.Getenv(storageHomeSIGKILLRootEnv))
	if err != nil {
		t.Fatalf("ResolveRoot(child) error = %v", err)
	}
	ops := &storageHomeMigrationCopyOps{beforePublish: func(string) error {
		if err := os.WriteFile(os.Getenv(storageHomeSIGKILLSignalEnv), []byte("ready\n"), 0o600); err != nil {
			return err
		}
		for {
			time.Sleep(time.Second)
		}
	}}
	if _, err := applyStorageHomeMigrationWithOps(context.Background(), root, PathResolver{}, ops); err != nil {
		t.Fatalf("applyStorageHomeMigrationWithOps(child) error = %v", err)
	}
}

func waitForStorageHomeSignal(t *testing.T, signalPath string, waitDone <-chan error, output *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(signalPath); err == nil {
			return
		}
		select {
		case err := <-waitDone:
			t.Fatalf("storage-home child exited before signal: %v\n%s", err, output.String())
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for storage-home child signal\n%s", output.String())
}

func assertNoStorageHomeStagingFiles(t *testing.T, directory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".loaf-storage-migration-*.sqlite*"))
	if err != nil {
		t.Fatalf("glob staging files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("staging files = %v, want none", matches)
	}
}

func TestApplyStorageHomeMigrationMergeParityFailureRollsBackGlobalCanonicalRows(t *testing.T) {
	ctx := context.Background()
	root := projectRoot(t)
	otherRoot := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	resolver := PathResolver{}
	if _, err := Initialize(ctx, otherRoot, resolver); err != nil {
		t.Fatalf("Initialize(other) error = %v", err)
	}
	globalPath, err := resolver.DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath(global) error = %v", err)
	}
	globalStore, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if _, err := globalStore.LogJournal(ctx, otherRoot, JournalLogOptions{Entry: "decision(storage): global-canonical"}); err != nil {
		globalStore.Close()
		t.Fatalf("LogJournal(global) error = %v", err)
	}
	var globalProjectID, globalJournalID, globalMessage, globalHarness string
	var globalRowID int64
	if err := globalStore.db.QueryRowContext(ctx, `
SELECT journal_entries.rowid, journal_entries.project_id, journal_entries.id, journal_entries.message,
       COALESCE(journal_entries.harness_session_id, '') FROM journal_entries`).Scan(&globalRowID, &globalProjectID, &globalJournalID, &globalMessage, &globalHarness); err != nil {
		globalStore.Close()
		t.Fatalf("read global journal row: %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `DROP TABLE journal_search`); err != nil {
		globalStore.Close()
		t.Fatalf("drop global journal_search: %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `
CREATE TABLE journal_search (
  rowid INTEGER PRIMARY KEY,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  session_id TEXT,
  entry_type TEXT NOT NULL,
  scope TEXT NOT NULL,
  message TEXT NOT NULL CHECK(message = 'prior derived')
)`); err != nil {
		globalStore.Close()
		t.Fatalf("create failing global journal_search: %v", err)
	}
	if _, err := globalStore.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
VALUES (?, ?, ?, ?, 'decision', 'storage', 'prior derived')`, globalRowID, globalProjectID, globalJournalID, globalHarness); err != nil {
		globalStore.Close()
		t.Fatalf("seed global prior derived row: %v", err)
	}
	if err := globalStore.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}
	initializeLegacyStateDatabase(t, root, resolver)
	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		t.Fatalf("PreviewStorageHomeMigration() error = %v", err)
	}
	if plan.Action != StorageHomeActionMerge {
		t.Fatalf("plan action = %q, want %q", plan.Action, StorageHomeActionMerge)
	}
	if _, err := ApplyStorageHomeMigration(ctx, root, resolver); err == nil {
		t.Fatal("ApplyStorageHomeMigration() error = nil, want merge parity failure")
	}
	check, err := OpenStore(globalPath)
	if err != nil {
		t.Fatalf("OpenStore(global after rollback) error = %v", err)
	}
	defer check.Close()
	var gotMessage string
	if err := check.db.QueryRowContext(ctx, `SELECT message FROM journal_entries WHERE id = ?`, globalJournalID).Scan(&gotMessage); err != nil {
		t.Fatalf("read global canonical row after rollback: %v", err)
	}
	if gotMessage != globalMessage {
		t.Fatalf("global canonical message after rollback = %q, want %q", gotMessage, globalMessage)
	}
	var priorCount int
	if err := check.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM journal_search WHERE journal_entry_id = ? AND message = 'prior derived'`, globalJournalID).Scan(&priorCount); err != nil {
		t.Fatalf("read global derived row after rollback: %v", err)
	}
	if priorCount != 1 {
		t.Fatalf("global prior derived rows after rollback = %d, want one", priorCount)
	}
	if got := countIdentityRows(t, check, `SELECT COUNT(*) FROM journal_entries WHERE project_id = ?`, ProjectID(root)); got != 0 {
		t.Fatalf("copied root canonical rows after rollback = %d, want zero", got)
	}
}

type migrationProjectRow struct {
	ID           string
	IdentityHash string
	FriendlyName string
	CurrentPath  string
	LastSeenAt   string
	CreatedAt    string
	UpdatedAt    string
}

func readMigrationProjectRow(t *testing.T, store *Store, projectID string) migrationProjectRow {
	t.Helper()
	var row migrationProjectRow
	if err := store.db.QueryRowContext(context.Background(), `
SELECT id, identity_hash, COALESCE(friendly_name, ''), COALESCE(current_path, ''),
       COALESCE(last_seen_at, ''), created_at, updated_at
FROM projects WHERE id = ?`, projectID).Scan(&row.ID, &row.IdentityHash, &row.FriendlyName, &row.CurrentPath, &row.LastSeenAt, &row.CreatedAt, &row.UpdatedAt); err != nil {
		t.Fatalf("read migration project row: %v", err)
	}
	return row
}

func initializeLegacyStateDatabase(t *testing.T, root project.Root, resolver PathResolver) string {
	t.Helper()
	ctx := context.Background()
	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		t.Fatalf("LegacyDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o700); err != nil {
		t.Fatalf("create legacy database dir error = %v", err)
	}
	store, err := OpenStore(legacyPath)
	if err != nil {
		t.Fatalf("OpenStore(legacy) error = %v", err)
	}
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations(legacy) error = %v", err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject(legacy) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(legacy) error = %v", err)
	}
	return legacyPath
}

func initializeProjectDataDatabase(t *testing.T, root project.Root, resolver PathResolver) string {
	t.Helper()
	projectPath, err := resolver.ProjectDatabasePath(root)
	if err != nil {
		t.Fatalf("ProjectDatabasePath() error = %v", err)
	}
	initializeDatabaseAtPath(t, root, projectPath)
	return projectPath
}

func initializeDatabaseAtPath(t *testing.T, root project.Root, path string) {
	t.Helper()
	ctx := context.Background()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create database dir error = %v", err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore(%s) error = %v", path, err)
	}
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations(%s) error = %v", path, err)
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		t.Fatalf("UpsertProject(%s) error = %v", path, err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(%s) error = %v", path, err)
	}
}
