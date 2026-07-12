package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/levifig/loaf/internal/project"
)

func TestInitializeAppliesMigrationsAndRecordsProject(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()

	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if status.Mode != ModeSQLiteReady {
		t.Fatalf("Mode = %q, want %q", status.Mode, ModeSQLiteReady)
	}
	if status.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, CurrentSchemaVersion())
	}
	if !status.DatabaseExists {
		t.Fatal("DatabaseExists = false, want true")
	}
	if !filepath.IsAbs(status.DatabasePath) {
		t.Fatalf("DatabasePath = %q, want absolute path", status.DatabasePath)
	}
	if _, err := os.Stat(status.DatabasePath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	count, err := store.AppliedMigrationCount(context.Background())
	if err != nil {
		t.Fatalf("AppliedMigrationCount() error = %v", err)
	}
	if count != len(SchemaMigrations()) {
		t.Fatalf("AppliedMigrationCount() = %d, want %d", count, len(SchemaMigrations()))
	}

	var projectID string
	err = store.db.QueryRowContext(context.Background(), `SELECT id FROM projects WHERE id = ?`, projectIDForTest(t, store, root)).Scan(&projectID)
	if err != nil {
		t.Fatalf("project row missing: %v", err)
	}
}

func TestInitializeIsIdempotent(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()

	first, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	second, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}

	if first.DatabasePath != second.DatabasePath {
		t.Fatalf("DatabasePath changed: %q -> %q", first.DatabasePath, second.DatabasePath)
	}
	if second.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", second.SchemaVersion, CurrentSchemaVersion())
	}
}

func TestOpenStoreReadOnlyDoesNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := OpenStoreReadOnly(path); err == nil {
		t.Fatal("OpenStoreReadOnly() error = nil, want missing database error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want missing database to stay missing", path, err)
	}
}

func TestOpenStoreReadOnlyRejectsInvalidDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.sqlite")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := OpenStoreReadOnly(path); err == nil {
		t.Fatal("OpenStoreReadOnly() error = nil, want invalid database error")
	} else if !strings.Contains(err.Error(), "validate state database read-only") {
		t.Fatalf("OpenStoreReadOnly() error = %v, want validation context", err)
	}
}

func TestProjectIdentityIsStableAcrossRenameAndMove(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	identity, err := store.ProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ProjectIdentityForRoot() error = %v", err)
	}
	if identity.ID == ProjectID(root) {
		t.Fatalf("project ID = legacy path hash %q, want path-independent generated ID", identity.ID)
	}
	if identity.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", identity.ContractVersion, StateJSONContractVersion)
	}
	if identity.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", identity.DatabaseScope)
	}
	if identity.FriendlyName != filepath.Base(root.Path()) {
		t.Fatalf("FriendlyName = %q, want folder name", identity.FriendlyName)
	}

	renamed, err := store.RenameProject(context.Background(), root, "Friendly Loaf")
	if err != nil {
		t.Fatalf("RenameProject() error = %v", err)
	}
	if renamed.ID != identity.ID {
		t.Fatalf("rename changed project ID: %q -> %q", identity.ID, renamed.ID)
	}
	if renamed.DatabaseScope != "global" {
		t.Fatalf("renamed DatabaseScope = %q, want global", renamed.DatabaseScope)
	}
	if renamed.FriendlyName != "Friendly Loaf" {
		t.Fatalf("FriendlyName = %q, want Friendly Loaf", renamed.FriendlyName)
	}

	newRoot, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot(new) error = %v", err)
	}
	moved, err := store.MoveProject(context.Background(), newRoot, root.Path(), newRoot.Path())
	if err != nil {
		t.Fatalf("MoveProject() error = %v", err)
	}
	if moved.Project.ID != identity.ID {
		t.Fatalf("move changed project ID: %q -> %q", identity.ID, moved.Project.ID)
	}
	if moved.DatabaseScope != "global" || moved.Project.DatabaseScope != "global" {
		t.Fatalf("moved scopes = %q/%q, want global/global", moved.DatabaseScope, moved.Project.DatabaseScope)
	}
	if moved.Project.CurrentPath != newRoot.Path() {
		t.Fatalf("CurrentPath = %q, want %q", moved.Project.CurrentPath, newRoot.Path())
	}
}

func TestPreviewMoveProjectValidatesWithoutWriting(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	identity, err := store.ProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ProjectIdentityForRoot() error = %v", err)
	}
	newRoot, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot(new) error = %v", err)
	}
	preview, err := store.PreviewMoveProject(context.Background(), newRoot, root.Path(), newRoot.Path())
	if err != nil {
		t.Fatalf("PreviewMoveProject() error = %v", err)
	}
	if preview.Action != "dry-run" {
		t.Fatalf("Action = %q, want dry-run", preview.Action)
	}
	if preview.DatabaseScope != "global" || preview.Project.DatabaseScope != "global" {
		t.Fatalf("preview scopes = %q/%q, want global/global", preview.DatabaseScope, preview.Project.DatabaseScope)
	}
	if preview.Project.ID != identity.ID || preview.Project.CurrentPath != newRoot.Path() {
		t.Fatalf("preview project = %#v, want same ID %q previewing %s", preview.Project, identity.ID, newRoot.Path())
	}
	after, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LookupProjectIdentityForRoot(root) error = %v", err)
	}
	if after.CurrentPath != root.Path() {
		t.Fatalf("CurrentPath after preview = %q, want original %q", after.CurrentPath, root.Path())
	}
	if got := countCurrentProjectPaths(t, store, identity.ID); got != 1 {
		t.Fatalf("current project paths after preview = %d, want 1", got)
	}
}

func TestPreviewRenameProjectValidatesWithoutWriting(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	identity, err := store.ProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ProjectIdentityForRoot() error = %v", err)
	}
	preview, err := store.PreviewRenameProject(context.Background(), root, "Preview Loaf")
	if err != nil {
		t.Fatalf("PreviewRenameProject() error = %v", err)
	}
	if preview.Action != "dry-run" || preview.FromName != identity.FriendlyName || preview.ToName != "Preview Loaf" {
		t.Fatalf("preview = %#v, want dry-run from %q to Preview Loaf", preview, identity.FriendlyName)
	}
	if preview.DatabaseScope != "global" || preview.Project.DatabaseScope != "global" {
		t.Fatalf("preview scopes = %q/%q, want global/global", preview.DatabaseScope, preview.Project.DatabaseScope)
	}
	if preview.Project.ID != identity.ID || preview.Project.FriendlyName != "Preview Loaf" {
		t.Fatalf("preview project = %#v, want same ID %q with preview name", preview.Project, identity.ID)
	}
	after, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LookupProjectIdentityForRoot(root) error = %v", err)
	}
	if after.FriendlyName != identity.FriendlyName {
		t.Fatalf("FriendlyName after preview = %q, want original %q", after.FriendlyName, identity.FriendlyName)
	}
}

func TestRenameProjectRequiresRegisteredIdentity(t *testing.T) {
	root := projectRoot(t)
	unknownRoot, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot(unknown) error = %v", err)
	}
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if _, err := store.RenameProject(context.Background(), unknownRoot, "Unknown"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RenameProject(unknown) error = %v, want sql.ErrNoRows", err)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM projects`); got != 1 {
		t.Fatalf("projects = %d, want only initialized project row", got)
	}
}

func TestListProjectsReturnsRegisteredIdentities(t *testing.T) {
	root := projectRoot(t)
	otherRoot, err := project.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveRoot(other) error = %v", err)
	}
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	loafIdentity, err := store.RenameProject(context.Background(), root, "Loaf")
	if err != nil {
		t.Fatalf("RenameProject() error = %v", err)
	}
	otherIdentity, err := store.EnsureProject(context.Background(), otherRoot)
	if err != nil {
		t.Fatalf("EnsureProject(other) error = %v", err)
	}

	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if projects.DatabasePath != status.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", projects.DatabasePath, status.DatabasePath)
	}
	if projects.ContractVersion != StateJSONContractVersion {
		t.Fatalf("projects ContractVersion = %d, want %d", projects.ContractVersion, StateJSONContractVersion)
	}
	if projects.DatabaseScope != "global" {
		t.Fatalf("projects DatabaseScope = %q, want global", projects.DatabaseScope)
	}
	if len(projects.Projects) != 2 {
		t.Fatalf("projects = %#v, want two registered identities", projects.Projects)
	}
	byID := map[string]ProjectIdentity{}
	for _, project := range projects.Projects {
		if project.DatabaseScope != "global" {
			t.Fatalf("listed project DatabaseScope = %q, want global", project.DatabaseScope)
		}
		byID[project.ID] = project
	}
	if byID[loafIdentity.ID].FriendlyName != "Loaf" || byID[loafIdentity.ID].CurrentPath != root.Path() {
		t.Fatalf("loaf project = %#v, want renamed identity at %s", byID[loafIdentity.ID], root.Path())
	}
	if byID[otherIdentity.ID].CurrentPath != otherRoot.Path() {
		t.Fatalf("other project = %#v, want path %s", byID[otherIdentity.ID], otherRoot.Path())
	}
}

func TestProjectPathsAllowOnlyOneCurrentPath(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	projectID := projectIDForTest(t, store, root)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = store.db.ExecContext(context.Background(), `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES ('duplicate-current-path', ?, ?, 1, ?, ?, ?, ?)
`, projectID, filepath.Join(root.Path(), "other"), now, now, now, now)
	if err == nil {
		t.Fatal("insert duplicate current project path error = nil, want unique constraint failure")
	}
}

func TestStateCommandsFailWhenProjectIdentityMappingIsMissing(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if _, err := store.db.ExecContext(context.Background(), `DROP TABLE project_paths`); err != nil {
		t.Fatalf("drop project_paths error = %v", err)
	}

	if _, err := store.ListTasks(context.Background(), root, TaskListOptions{}); err == nil {
		t.Fatal("ListTasks() error = nil, want project identity mapping error")
	} else if !strings.Contains(err.Error(), "read project path mapping") {
		t.Fatalf("ListTasks() error = %q, want project identity mapping error", err)
	}
}

func TestMoveProjectUnknownFromPathDoesNotCreateProject(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	databasePath, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
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
	defer store.Close()
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	_, err = store.MoveProject(context.Background(), root, filepath.Join(t.TempDir(), "missing"), root.Path())
	if err == nil {
		t.Fatal("MoveProject() error = nil, want unknown --from rejection")
	}
	var count int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM projects`).Scan(&count); err != nil {
		t.Fatalf("count projects error = %v", err)
	}
	if count != 0 {
		t.Fatalf("projects = %d, want no row after rejected move", count)
	}
}

func TestMoveProjectRequiresExistingTargetDirectory(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	missingTarget := filepath.Join(t.TempDir(), "missing-target")
	if _, err := store.PreviewMoveProject(context.Background(), root, root.Path(), missingTarget); err == nil {
		t.Fatal("PreviewMoveProject() error = nil, want missing target rejection")
	} else if !strings.Contains(err.Error(), "target path does not exist") {
		t.Fatalf("PreviewMoveProject() error = %q, want missing target path rejection", err)
	}
	if _, err := store.MoveProject(context.Background(), root, root.Path(), missingTarget); err == nil {
		t.Fatal("MoveProject() error = nil, want missing target rejection")
	} else if !strings.Contains(err.Error(), "target path does not exist") {
		t.Fatalf("MoveProject() error = %q, want missing target path rejection", err)
	}

	identity, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("LookupProjectIdentityForRoot() error = %v", err)
	}
	if identity.CurrentPath != root.Path() {
		t.Fatalf("CurrentPath = %q, want unchanged %q", identity.CurrentPath, root.Path())
	}
}

func TestProjectIdentityRekeysLegacyPathHashRows(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	databasePath, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
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
	defer store.Close()
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	legacyID := ProjectID(root)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO projects (id, identity_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
`, legacyID, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy project error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO ideas (id, project_id, title, status, created_at, updated_at)
VALUES ('idea-legacy', ?, 'Legacy Idea', 'open', ?, ?)
`, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy idea error = %v", err)
	}

	identity, err := store.EnsureProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ProjectIdentityForRoot() error = %v", err)
	}
	if identity.ID == legacyID {
		t.Fatalf("identity ID = legacy path hash %q, want generated ID", identity.ID)
	}
	var ideaProjectID string
	if err := store.db.QueryRowContext(context.Background(), `SELECT project_id FROM ideas WHERE id = 'idea-legacy'`).Scan(&ideaProjectID); err != nil {
		t.Fatalf("read rekeyed idea error = %v", err)
	}
	if ideaProjectID != identity.ID {
		t.Fatalf("idea project_id = %q, want %q", ideaProjectID, identity.ID)
	}
}

func TestLookupProjectIdentityDoesNotFallBackToLegacyPathHash(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	databasePath, err := (PathResolver{StateHome: stateHome}).DatabasePath(root)
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
	defer store.Close()
	if err := store.ApplyMigrations(context.Background()); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	legacyID := ProjectID(root)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO projects (id, identity_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
`, legacyID, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy project error = %v", err)
	}

	if _, err := store.LookupProjectIdentityForRoot(context.Background(), root); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("LookupProjectIdentityForRoot() error = %v, want sql.ErrNoRows", err)
	}
}

func TestOpenStoreAppliesConnectionPragmas(t *testing.T) {
	root := projectRoot(t)
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	var foreignKeys int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys error = %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout error = %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout = %d, want at least 5000", busyTimeout)
	}

	var journalMode string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode error = %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var synchronous int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		t.Fatalf("PRAGMA synchronous error = %v", err)
	}
	if synchronous != 2 {
		t.Fatalf("synchronous = %d, want 2 (FULL)", synchronous)
	}
}

func TestOpenStoreReadOnlyForBackupAppliesSafeSnapshotPragmas(t *testing.T) {
	root := projectRoot(t)
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	beforeDigest := testFileSHA256(t, status.DatabasePath)
	store, err := openStoreReadOnlyForBackup(status.DatabasePath)
	if err != nil {
		t.Fatalf("openStoreReadOnlyForBackup() error = %v", err)
	}
	defer store.Close()
	var synchronous, foreignKeys, busyTimeout int
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA synchronous`).Scan(&synchronous); err != nil {
		t.Fatalf("PRAGMA synchronous error = %v", err)
	}
	if synchronous != 2 {
		t.Fatalf("synchronous = %d, want 2 (FULL)", synchronous)
	}
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys error = %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout error = %v", err)
	}
	if busyTimeout < 5000 {
		t.Fatalf("busy_timeout = %d, want at least 5000", busyTimeout)
	}
	var journalMode string
	if err := store.db.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode error = %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if afterDigest := testFileSHA256(t, status.DatabasePath); afterDigest != beforeDigest {
		t.Fatalf("source database digest changed from %s to %s", beforeDigest, afterDigest)
	}
}

func TestApplyMigrationsDetectsChecksumDrift(t *testing.T) {
	db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	migration := SchemaMigration{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"}
	if err := ApplyMigrations(context.Background(), db, []SchemaMigration{migration}); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}

	drifted := SchemaMigration{Version: 1, Name: "one", SQL: "CREATE TABLE two (id TEXT PRIMARY KEY NOT NULL);\n"}
	if err := ApplyMigrations(context.Background(), db, []SchemaMigration{drifted}); err == nil {
		t.Fatal("ApplyMigrations() error = nil, want checksum mismatch")
	}
}

func TestApplyMigrationsRejectsInvalidFutureMigrationSequence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		migrations []SchemaMigration
		want       string
	}{
		{
			name: "duplicate version",
			migrations: []SchemaMigration{
				{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
				{Version: 1, Name: "also_one", SQL: "CREATE TABLE also_one (id TEXT PRIMARY KEY NOT NULL);\n"},
			},
			want: "version 1 must be greater than previous version 1",
		},
		{
			name: "out of order",
			migrations: []SchemaMigration{
				{Version: 2, Name: "two", SQL: "CREATE TABLE two (id TEXT PRIMARY KEY NOT NULL);\n"},
				{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
			},
			want: "version 1 must be greater than previous version 2",
		},
		{
			name: "missing name",
			migrations: []SchemaMigration{
				{Version: 1, SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
			},
			want: "schema migration 1 must have a name",
		},
		{
			name: "empty sql",
			migrations: []SchemaMigration{
				{Version: 1, Name: "one"},
			},
			want: "schema migration 1 SQL cannot be empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
			if err != nil {
				t.Fatalf("sql.Open() error = %v", err)
			}
			defer db.Close()

			err = ApplyMigrations(context.Background(), db, tc.migrations)
			if err == nil {
				t.Fatal("ApplyMigrations() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ApplyMigrations() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestApplyMigrationsRollsBackFailedMigrationBatch(t *testing.T) {
	db, err := sql.Open(sqliteDriverName, filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	migrations := []SchemaMigration{
		{Version: 1, Name: "one", SQL: "CREATE TABLE one (id TEXT PRIMARY KEY NOT NULL);\n"},
		{Version: 2, Name: "bad", SQL: "CREATE TABLE broken (\n"},
	}
	if err := ApplyMigrations(context.Background(), db, migrations); err == nil {
		t.Fatal("ApplyMigrations() error = nil, want failure")
	}

	var tableName string
	err = db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'one'`).Scan(&tableName)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("table one lookup error = %v, want no table after rollback", err)
	}
	var migrationsTable string
	err = db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`).Scan(&migrationsTable)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("schema_migrations lookup error = %v, want no table after rollback", err)
	}
}

func countCurrentProjectPaths(t *testing.T, store *Store, projectID string) int {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM project_paths WHERE project_id = ? AND is_current = 1`, projectID).Scan(&count); err != nil {
		t.Fatalf("count current project paths error = %v", err)
	}
	return count
}
