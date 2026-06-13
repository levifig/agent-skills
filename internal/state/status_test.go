package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestInspectReportsMarkdownOnlyWithoutCreatingFiles(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeMarkdownOnly {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeMarkdownOnly)
	}
	if status.DatabaseExists {
		t.Fatal("DatabaseExists = true, want false")
	}
	if status.DatabaseParentExists {
		t.Fatal("DatabaseParentExists = true, want false before init")
	}
	if _, err := os.Stat(filepath.Dir(status.DatabasePath)); !os.IsNotExist(err) {
		t.Fatalf("database parent exists after Inspect(); err = %v", err)
	}
	assertDiagnostic(t, status.Diagnostics, "database-missing")
	assertDiagnostic(t, status.Diagnostics, "markdown-fallback-active")
}

func TestInspectReportsLegacyStateDatabaseWhenDataHomeIsMissing(t *testing.T) {
	root := projectRoot(t)
	dataHome := t.TempDir()
	stateHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	legacyStatus, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(legacy) error = %v", err)
	}

	status, err := Inspect(root, PathResolver{})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeMarkdownOnly {
		t.Fatalf("Mode = %q, want %q before storage-home migration", status.Mode, ModeMarkdownOnly)
	}
	if status.DatabaseExists {
		t.Fatal("DatabaseExists = true, want false for new data home before migration")
	}
	if !status.LegacyDatabaseExists {
		t.Fatal("LegacyDatabaseExists = false, want true")
	}
	if status.LegacyDatabasePath != legacyStatus.DatabasePath {
		t.Fatalf("LegacyDatabasePath = %q, want %q", status.LegacyDatabasePath, legacyStatus.DatabasePath)
	}
	if !strings.HasPrefix(status.DatabasePath, dataHome+string(filepath.Separator)) {
		t.Fatalf("DatabasePath = %q, want under XDG_DATA_HOME %q", status.DatabasePath, dataHome)
	}
	assertDiagnostic(t, status.Diagnostics, "legacy-state-database-detected")
}

func TestInspectReportsSQLiteReadyWhenDatabaseIsInitialized(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	initialized, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	if status.DatabasePath != initialized.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", status.DatabasePath, initialized.DatabasePath)
	}
	if !status.DatabaseExists {
		t.Fatal("DatabaseExists = false, want true")
	}
	if !status.DatabaseParentExists {
		t.Fatal("DatabaseParentExists = false, want true")
	}
	if status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentSchemaVersion())
	}
	assertDiagnostic(t, status.Diagnostics, "sqlite-ready")
}

func TestInspectReportsInvalidWhenDatabaseFileIsNotSQLite(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "database-open-failed")
}

func TestInspectReportsInvalidSchemaVersionMismatch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	_, err := store.db.ExecContext(context.Background(), `
INSERT INTO schema_migrations (version, name, checksum, applied_at)
VALUES (99, 'future_schema', 'future', '2026-05-28T10:00:00Z')
`)
	if err != nil {
		t.Fatalf("insert future schema migration error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "schema-version-mismatch")
}

func TestInspectReportsInvalidSchemaChecksumMismatch(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	if _, err := store.db.ExecContext(context.Background(), `UPDATE schema_migrations SET checksum = 'drifted' WHERE version = 1`); err != nil {
		t.Fatalf("drift schema migration checksum error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "schema-checksum-mismatch")
}

func TestInspectReportsStaleCompatibilityExportsAsWarnings(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	defer store.Close()

	projectID := ProjectID(root)
	_, err := store.db.ExecContext(context.Background(), `
INSERT INTO ideas (id, project_id, title, status, created_at, updated_at)
VALUES ('idea-stale-export', ?, 'Stale Export Idea', 'open', '2026-05-28T10:00:00Z', '2026-05-28T12:00:00Z');
`, projectID)
	if err != nil {
		t.Fatalf("insert stale export idea fixture error = %v", err)
	}
	_, err = store.db.ExecContext(context.Background(), `
INSERT INTO exports (id, project_id, export_kind, format, path, state_version, source_entity_kind, source_entity_id, generated_at, created_at, updated_at)
VALUES ('export-stale', ?, 'triage', 'markdown', '.agents/exports/triage.md', 1, 'idea', 'idea-stale-export', '2026-05-28T11:00:00Z', '2026-05-28T11:00:00Z', '2026-05-28T11:00:00Z');
`, projectID)
	if err != nil {
		t.Fatalf("insert stale export fixture error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q despite stale export warning", status.Mode, ModeSQLiteReady)
	}
	assertDiagnostic(t, status.Diagnostics, "sqlite-ready")
	assertDiagnostic(t, status.Diagnostics, "stale-compatibility-export")
}

func TestInspectReportsInvalidEmptyFileWithoutOpeningSQLite(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "database-file-empty")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("empty file size = %d, want 0 after Inspect", info.Size())
	}
}

func TestInspectReportsInvalidWhenDatabasePathIsDirectory(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	status, err := Inspect(root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}

	if status.Mode != ModeInvalid {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeInvalid)
	}
	assertDiagnostic(t, status.Diagnostics, "database-path-is-directory")
}

func projectRoot(t *testing.T) project.Root {
	t.Helper()
	root, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	return root
}

func openTestStore(t *testing.T, root project.Root, stateHome string) *Store {
	t.Helper()
	path, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	return store
}

func assertDiagnostic(t *testing.T, diagnostics []Diagnostic, code string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("diagnostic %q not found in %#v", code, diagnostics)
}
