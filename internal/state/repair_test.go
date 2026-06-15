package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepairMissingRelationshipOriginsDryRunDoesNotWrite(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithoutOrigin(t, store, projectID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := RepairMissingRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "imported"})
	if err != nil {
		t.Fatalf("RepairMissingRelationshipOrigins() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", result.ProjectID, projectID)
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if result.Applied {
		t.Fatal("Applied = true, want dry-run")
	}
	if result.Matched != 1 {
		t.Fatalf("Matched = %d, want 1", result.Matched)
	}
	if result.Updated != 0 {
		t.Fatalf("Updated = %d, want 0 for dry-run", result.Updated)
	}

	store = openTestStore(t, root, stateHome)
	defer store.Close()
	assertMissingRelationshipOrigins(t, store, projectID, 1)
}

func TestRepairMissingRelationshipOriginsApplyBackfillsCurrentProject(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	projectID := projectIDForTest(t, store, root)
	insertRelationshipWithoutOrigin(t, store, projectID)
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	result, err := RepairMissingRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "imported", Apply: true})
	if err != nil {
		t.Fatalf("RepairMissingRelationshipOrigins() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID != projectID {
		t.Fatalf("ProjectID = %q, want %q", result.ProjectID, projectID)
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if result.Matched != 1 {
		t.Fatalf("Matched = %d, want 1", result.Matched)
	}
	if result.Updated != 1 {
		t.Fatalf("Updated = %d, want 1", result.Updated)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty for applied repair")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("repair backup does not exist: %v", err)
	}

	store = openTestStore(t, root, stateHome)
	defer store.Close()
	assertMissingRelationshipOrigins(t, store, projectID, 0)
	var origin string
	if err := store.db.QueryRowContext(context.Background(), `SELECT origin FROM relationships WHERE id = 'relationship-without-origin'`).Scan(&origin); err != nil {
		t.Fatalf("read repaired relationship origin error = %v", err)
	}
	if origin != "imported" {
		t.Fatalf("origin = %q, want imported", origin)
	}
}

func TestRepairMissingRelationshipOriginsRejectsUnknownOrigin(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	_, err := RepairMissingRelationshipOrigins(context.Background(), root, PathResolver{StateHome: stateHome}, RelationshipOriginRepairOptions{Origin: "guessed", Apply: true})
	if err == nil {
		t.Fatal("RepairMissingRelationshipOrigins() error = nil, want invalid origin error")
	}
}

func TestArchiveLegacyProjectDatabaseDryRunDoesNotMoveFiles(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	if _, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{}); err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}

	result, err := ArchiveLegacyProjectDatabase(root, PathResolver{}, false)
	if err != nil {
		t.Fatalf("ArchiveLegacyProjectDatabase() error = %v", err)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if result.Applied {
		t.Fatal("Applied = true, want dry-run")
	}
	if result.Action != LegacyProjectDatabaseArchiveAction {
		t.Fatalf("Action = %q, want %q", result.Action, LegacyProjectDatabaseArchiveAction)
	}
	if len(result.MatchedPaths) != 1 || result.MatchedPaths[0] != legacyPath {
		t.Fatalf("MatchedPaths = %#v, want legacy path %q", result.MatchedPaths, legacyPath)
	}
	if _, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("legacy database moved during dry-run: %v", err)
	}
	if result.ArchivePath == "" {
		t.Fatal("ArchivePath is empty")
	}
	if _, err := os.Stat(result.ArchivePath); !os.IsNotExist(err) {
		t.Fatalf("archive path exists during dry-run; err = %v", err)
	}
}

func TestArchiveLegacyProjectDatabaseApplyMovesDatabaseAndSidecars(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyPath := initializeLegacyStateDatabase(t, root, PathResolver{})
	sidecarPath := legacyPath + "-wal"
	if _, err := ApplyStorageHomeMigration(context.Background(), root, PathResolver{}); err != nil {
		t.Fatalf("ApplyStorageHomeMigration() error = %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte("sidecar"), 0o600); err != nil {
		t.Fatalf("write legacy sidecar error = %v", err)
	}

	result, err := ArchiveLegacyProjectDatabase(root, PathResolver{}, true)
	if err != nil {
		t.Fatalf("ArchiveLegacyProjectDatabase() error = %v", err)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectID == "" {
		t.Fatal("ProjectID is empty")
	}
	if result.ProjectName != filepath.Base(root.Path()) {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, filepath.Base(root.Path()))
	}
	if result.ProjectCurrentPath != root.Path() {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, root.Path())
	}
	if !result.Applied {
		t.Fatal("Applied = false, want true")
	}
	if len(result.ArchivedPaths) != 2 {
		t.Fatalf("ArchivedPaths = %#v, want database and sidecar", result.ArchivedPaths)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy database still exists after archive; err = %v", err)
	}
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Fatalf("legacy sidecar still exists after archive; err = %v", err)
	}
	for _, path := range result.ArchivedPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("archived path %q missing: %v", path, err)
		}
	}
}

func TestArchiveLegacyProjectDatabaseRejectsPendingMigration(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	initializeLegacyStateDatabase(t, root, PathResolver{})
	databasePath, err := (PathResolver{}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		t.Fatalf("create global database dir error = %v", err)
	}
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(global) error = %v", err)
	}
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations(global) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(global) error = %v", err)
	}

	_, err = ArchiveLegacyProjectDatabase(root, PathResolver{}, true)
	if err == nil {
		t.Fatal("ArchiveLegacyProjectDatabase() error = nil, want pending migration error")
	}
	if !strings.Contains(err.Error(), "legacy project database still needs migration") {
		t.Fatalf("error = %v, want pending migration error", err)
	}
}

func insertRelationshipWithoutOrigin(t *testing.T, store *Store, projectID string) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO relationships (id, project_id, from_entity_kind, from_entity_id, to_entity_kind, to_entity_id, relationship_type, reason, created_at, updated_at)
VALUES ('relationship-without-origin', ?, 'task', 'task-one', 'spec', 'spec-one', 'implements', 'legacy row', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`, projectID); err != nil {
		t.Fatalf("insert relationship without origin error = %v", err)
	}
}

func assertMissingRelationshipOrigins(t *testing.T, store *Store, projectID string, want int) {
	t.Helper()
	var got int
	if err := store.db.QueryRowContext(context.Background(), `
SELECT COUNT(*)
FROM relationships
WHERE project_id = ?
  AND (origin IS NULL OR TRIM(origin) = '')
`, projectID).Scan(&got); err != nil {
		t.Fatalf("count missing relationship origins error = %v", err)
	}
	if got != want {
		t.Fatalf("missing relationship origins = %d, want %d", got, want)
	}
}
