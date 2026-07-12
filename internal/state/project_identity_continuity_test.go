package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/levifig/loaf/internal/project"
)

func TestProjectIdentityUnknownLookupReturnsActionableTypedError(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	knownRoots := []project.Root{projectRootInTempDir(t), projectRootInTempDir(t)}
	status, err := Initialize(ctx, knownRoots[0], PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.EnsureProject(ctx, knownRoots[1]); err != nil {
		t.Fatalf("EnsureProject(knownRoots[1]) error = %v", err)
	}

	unknownRoot := projectRootInTempDir(t)
	projectsBefore := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`)
	pathsBefore := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths`)
	_, err = store.LookupProjectIdentityForRoot(ctx, unknownRoot)
	if err == nil {
		t.Fatal("LookupProjectIdentityForRoot() error = nil, want unregistered identity error")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("LookupProjectIdentityForRoot() error = %v, want sql.ErrNoRows", err)
	}
	var typedErr *UnregisteredProjectIdentityError
	if !errors.As(err, &typedErr) {
		t.Fatalf("LookupProjectIdentityForRoot() error = %T, want *UnregisteredProjectIdentityError", err)
	}
	if typedErr.CurrentPath != unknownRoot.Path() {
		t.Fatalf("CurrentPath = %q, want %q", typedErr.CurrentPath, unknownRoot.Path())
	}
	if typedErr.Code != ProjectIdentityUnregisteredCode {
		t.Fatalf("Code = %q, want %q", typedErr.Code, ProjectIdentityUnregisteredCode)
	}
	wantPaths := []string{knownRoots[0].Path(), knownRoots[1].Path()}
	sort.Strings(wantPaths)
	if fmt.Sprint(typedErr.KnownCurrentPaths) != fmt.Sprint(wantPaths) {
		t.Fatalf("KnownCurrentPaths = %#v, want %#v", typedErr.KnownCurrentPaths, wantPaths)
	}
	wantCommands := make([]string, 0, len(wantPaths))
	for _, knownPath := range wantPaths {
		wantCommands = append(wantCommands, fmt.Sprintf("loaf project move --from '%s' --to '%s' --dry-run", strings.ReplaceAll(knownPath, "'", "'\\''"), strings.ReplaceAll(unknownRoot.Path(), "'", "'\\''")))
	}
	if fmt.Sprint(typedErr.MoveCommands) != fmt.Sprint(wantCommands) {
		t.Fatalf("MoveCommands = %#v, want %#v", typedErr.MoveCommands, wantCommands)
	}
	if typedErr.InitCommand != "loaf state init" {
		t.Fatalf("InitCommand = %q, want %q", typedErr.InitCommand, "loaf state init")
	}
	wantError := fmt.Sprintf("project identity is not registered for %s\nif this checkout moved, run one of:\n  %s\n  %s\notherwise initialize it explicitly with:\n  loaf state init", unknownRoot.Path(), wantCommands[0], wantCommands[1])
	if err.Error() != wantError {
		t.Fatalf("error string = %q, want %q", err, wantError)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`); got != projectsBefore {
		t.Fatalf("projects after unknown lookup = %d, want %d", got, projectsBefore)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths`); got != pathsBefore {
		t.Fatalf("project_paths after unknown lookup = %d, want %d", got, pathsBefore)
	}
}

func TestProjectIdentityUnknownLookupQuotesMovePathsDeterministically(t *testing.T) {
	ctx := context.Background()
	stateHome := t.TempDir()
	knownDir := filepath.Join(t.TempDir(), "known path's")
	if err := os.MkdirAll(knownDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(known) error = %v", err)
	}
	knownRoot, err := project.ResolveRoot(knownDir)
	if err != nil {
		t.Fatalf("ResolveRoot(known) error = %v", err)
	}
	unknownRoot := projectRootInTempDir(t)
	status, err := Initialize(ctx, knownRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	_, err = store.LookupProjectIdentityForRoot(ctx, unknownRoot)
	var typedErr *UnregisteredProjectIdentityError
	if !errors.As(err, &typedErr) {
		t.Fatalf("LookupProjectIdentityForRoot() error = %T, want *UnregisteredProjectIdentityError", err)
	}
	wantCommand := fmt.Sprintf("loaf project move --from '%s' --to '%s' --dry-run", strings.ReplaceAll(knownRoot.Path(), "'", "'\\''"), strings.ReplaceAll(unknownRoot.Path(), "'", "'\\''"))
	if len(typedErr.MoveCommands) != 1 || typedErr.MoveCommands[0] != wantCommand {
		t.Fatalf("MoveCommands = %#v, want [%q]", typedErr.MoveCommands, wantCommand)
	}
}

func TestProjectIDLookupDoesNotCreateIdentity(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if _, err := store.projectID(ctx, root); err == nil {
		t.Fatal("projectID() error = nil, want unregistered identity error")
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("projectID() error = %v, want sql.ErrNoRows", err)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`); got != 0 {
		t.Fatalf("projects after lookup = %d, want 0", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths`); got != 0 {
		t.Fatalf("project_paths after lookup = %d, want 0", got)
	}
}

func TestProjectRegistrationIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	first, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("first EnsureProject() error = %v", err)
	}
	second, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("second EnsureProject() error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("project ID changed: %q -> %q", first.ID, second.ID)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`); got != 1 {
		t.Fatalf("projects = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths WHERE is_current = 1`); got != 1 {
		t.Fatalf("current project paths = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects WHERE id <> ?`, first.ID); got != 0 {
		t.Fatalf("orphan projects = %d, want 0", got)
	}
}

func TestProjectRegistrationConvergesAcrossIndependentStores(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	setup, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(setup) error = %v", err)
	}
	if err := setup.ApplyMigrations(ctx); err != nil {
		setup.Close()
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if err := setup.Close(); err != nil {
		t.Fatalf("Close(setup) error = %v", err)
	}
	stores := make([]*Store, 2)
	for i := range stores {
		stores[i], err = OpenStore(databasePath)
		if err != nil {
			t.Fatalf("OpenStore(%d) error = %v", i, err)
		}
		defer stores[i].Close()
	}

	identities := make([]ProjectIdentity, len(stores))
	errs := make([]error, len(stores))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			identities[i], errs[i] = stores[i].EnsureProject(ctx, root)
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("EnsureProject(%d) error = %v", i, err)
		}
	}
	if identities[0].ID != identities[1].ID {
		t.Fatalf("concurrent project IDs = %q and %q, want one durable ID", identities[0].ID, identities[1].ID)
	}
	check, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore(check) error = %v", err)
	}
	defer check.Close()
	if got := countIdentityRows(t, check, `SELECT COUNT(*) FROM projects`); got != 1 {
		t.Fatalf("projects = %d, want 1", got)
	}
	if got := countIdentityRows(t, check, `SELECT COUNT(*) FROM project_paths WHERE path = ? AND is_current = 1`, root.Path()); got != 1 {
		t.Fatalf("current mapping = %d, want 1", got)
	}
	if got := countIdentityRows(t, check, `SELECT COUNT(*) FROM projects WHERE id = ?`, identities[0].ID); got != 1 {
		t.Fatalf("durable project row = %d, want 1", got)
	}
}

func TestProjectRegistrationFailureRollsBackIdentityAndPath(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
CREATE TRIGGER fail_project_registration
BEFORE INSERT ON project_paths
BEGIN
  SELECT RAISE(ABORT, 'forced registration failure');
END;
`); err != nil {
		t.Fatalf("create failure trigger error = %v", err)
	}
	if _, err := store.EnsureProject(ctx, root); err == nil {
		t.Fatal("EnsureProject() error = nil, want forced registration failure")
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`); got != 0 {
		t.Fatalf("projects after failed registration = %d, want 0", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths`); got != 0 {
		t.Fatalf("project_paths after failed registration = %d, want 0", got)
	}
}

func TestProjectRegistrationRetainsLegacyRekey(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	legacyID := ProjectID(root)
	legacyPath := filepath.Join(t.TempDir(), "legacy-checkout")
	legacyHistoryPath := filepath.Join(t.TempDir(), "legacy-history")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := store.db.ExecContext(ctx, `
	INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
	VALUES (?, ?, 'Legacy Name', ?, ?, ?, ?)
	`, legacyID, legacyID, legacyPath, now, now, now); err != nil {
		t.Fatalf("insert legacy project error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
	INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
	VALUES ('legacy-path-current', ?, ?, 1, ?, ?, ?, ?),
	       ('legacy-path-history', ?, ?, 0, ?, ?, ?, ?)
	`, legacyID, legacyPath, now, now, now, now, legacyID, legacyHistoryPath, now, now, now, now); err != nil {
		t.Fatalf("insert legacy project paths error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO ideas (id, project_id, title, status, created_at, updated_at)
VALUES ('idea-legacy-continuity', ?, 'Legacy Idea', 'open', ?, ?)
`, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy idea error = %v", err)
	}
	identity, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}
	if identity.ID == legacyID {
		t.Fatalf("identity ID = %q, want generated ID", identity.ID)
	}
	var ideaProjectID string
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM ideas WHERE id = 'idea-legacy-continuity'`).Scan(&ideaProjectID); err != nil {
		t.Fatalf("read rekeyed idea error = %v", err)
	}
	if ideaProjectID != identity.ID {
		t.Fatalf("idea project_id = %q, want %q", ideaProjectID, identity.ID)
	}
	var currentPath string
	if err := store.db.QueryRowContext(ctx, `SELECT current_path FROM projects WHERE id = ?`, identity.ID).Scan(&currentPath); err != nil {
		t.Fatalf("read rekeyed current path error = %v", err)
	}
	if currentPath != root.Path() {
		t.Fatalf("current_path = %q, want %q", currentPath, root.Path())
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths WHERE project_id = ?`, identity.ID); got != 3 {
		t.Fatalf("rekeyed project paths = %d, want 3", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths WHERE project_id = ? AND is_current = 1 AND path = ?`, identity.ID, root.Path()); got != 1 {
		t.Fatalf("new current project path = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths WHERE project_id = ? AND is_current = 1`, identity.ID); got != 1 {
		t.Fatalf("current project paths after rekey = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`); got != 1 {
		t.Fatalf("projects after rekey = %d, want 1", got)
	}
}

func TestProjectRegistrationLegacyRekeyFailureRollsBackEverything(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	legacyID := ProjectID(root)
	legacyPath := filepath.Join(t.TempDir(), "legacy-checkout")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := store.db.ExecContext(ctx, `
	INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
	VALUES (?, ?, 'Legacy Name', ?, ?, ?, ?)
	`, legacyID, legacyID, legacyPath, now, now, now); err != nil {
		t.Fatalf("insert legacy project error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
	INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
	VALUES ('legacy-path-failure', ?, ?, 1, ?, ?, ?, ?)
	`, legacyID, legacyPath, now, now, now, now); err != nil {
		t.Fatalf("insert legacy project path error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
	INSERT INTO ideas (id, project_id, title, status, created_at, updated_at)
	VALUES ('idea-legacy-failure', ?, 'Legacy Idea', 'open', ?, ?)
	`, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy idea error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
	CREATE TRIGGER fail_legacy_rekey
	BEFORE UPDATE OF project_id ON ideas
	BEGIN
	  SELECT RAISE(ABORT, 'forced legacy rekey failure');
	END;
	`); err != nil {
		t.Fatalf("create legacy rekey trigger error = %v", err)
	}
	if _, err := store.EnsureProject(ctx, root); err == nil {
		t.Fatal("EnsureProject() error = nil, want forced legacy rekey failure")
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects`); got != 1 {
		t.Fatalf("projects after failed legacy rekey = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects WHERE id = ?`, legacyID); got != 1 {
		t.Fatalf("legacy project after failed rekey = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths`); got != 1 {
		t.Fatalf("project paths after failed legacy rekey = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM project_paths WHERE project_id = ? AND path = ? AND is_current = 1`, legacyID, legacyPath); got != 1 {
		t.Fatalf("legacy current path after failed rekey = %d, want 1", got)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM ideas WHERE project_id = ?`, legacyID); got != 1 {
		t.Fatalf("legacy scoped row after failed rekey = %d, want 1", got)
	}
}

func TestProjectRegistrationLegacyRekeyRebuildsJournalSearchProjectIDs(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	legacyID := ProjectID(root)
	legacyPath := filepath.Join(t.TempDir(), "legacy-checkout")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, 'Legacy Name', ?, ?, ?, ?)`, legacyID, legacyID, legacyPath, now, now, now); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES ('legacy-path', ?, ?, 1, ?, ?, ?, ?)`, legacyID, legacyPath, now, now, now, now); err != nil {
		t.Fatalf("insert legacy path: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message, observed_branch, observed_worktree,
  harness_session_id, session_id, spec_id, task_id, created_at, updated_at
) VALUES ('legacy-journal', ?, 'decision', 'rekey', 'legacy journal', 'main', NULL, 'hsid-rekey', NULL, NULL, NULL, ?, ?)`, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy journal row: %v", err)
	}
	var rowID int64
	if err := store.db.QueryRowContext(ctx, `SELECT rowid FROM journal_entries WHERE id = 'legacy-journal'`).Scan(&rowID); err != nil {
		t.Fatalf("read legacy journal rowid: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
VALUES (?, ?, 'legacy-journal', 'hsid-rekey', 'decision', 'rekey', 'legacy journal')`, rowID, legacyID); err != nil {
		t.Fatalf("insert legacy journal_search row: %v", err)
	}

	identity, err := store.EnsureProject(ctx, root)
	if err != nil {
		t.Fatalf("EnsureProject() error = %v", err)
	}
	if identity.ID == legacyID {
		t.Fatalf("identity ID = %q, want generated ID", identity.ID)
	}
	var canonicalProjectID, derivedProjectID string
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM journal_entries WHERE id = 'legacy-journal'`).Scan(&canonicalProjectID); err != nil {
		t.Fatalf("read rekeyed canonical journal row: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM journal_search WHERE journal_entry_id = 'legacy-journal'`).Scan(&derivedProjectID); err != nil {
		t.Fatalf("read rekeyed derived journal row: %v", err)
	}
	if canonicalProjectID != identity.ID || derivedProjectID != identity.ID {
		t.Fatalf("rekeyed project IDs canonical=%q derived=%q identity=%q, want all identity", canonicalProjectID, derivedProjectID, identity.ID)
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		t.Fatalf("InspectJournalSearchParity() error = %v", err)
	}
	if !parity.Ready {
		t.Fatalf("rekeyed journal parity = %#v, want ready", parity)
	}
}

func TestProjectRegistrationLegacyRekeyJournalSearchFailureRollsBackCanonicalAndIndex(t *testing.T) {
	ctx := context.Background()
	root := projectRootInTempDir(t)
	databasePath := filepath.Join(t.TempDir(), "state.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("ApplyMigrations() error = %v", err)
	}
	legacyID := ProjectID(root)
	legacyPath := filepath.Join(t.TempDir(), "legacy-checkout")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, friendly_name, current_path, last_seen_at, created_at, updated_at)
VALUES (?, ?, 'Legacy Name', ?, ?, ?, ?)`, legacyID, legacyID, legacyPath, now, now, now); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO project_paths (id, project_id, path, is_current, first_seen_at, last_seen_at, created_at, updated_at)
VALUES ('legacy-path', ?, ?, 1, ?, ?, ?, ?)`, legacyID, legacyPath, now, now, now, now); err != nil {
		t.Fatalf("insert legacy path: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_entries (
  id, project_id, entry_type, scope, message, observed_branch, observed_worktree,
  harness_session_id, session_id, spec_id, task_id, created_at, updated_at
) VALUES ('legacy-journal', ?, 'decision', 'rekey', 'changed canonical', 'main', NULL, 'hsid-rekey', NULL, NULL, NULL, ?, ?)`, legacyID, now, now); err != nil {
		t.Fatalf("insert legacy journal row: %v", err)
	}
	var rowID int64
	if err := store.db.QueryRowContext(ctx, `SELECT rowid FROM journal_entries WHERE id = 'legacy-journal'`).Scan(&rowID); err != nil {
		t.Fatalf("read legacy journal rowid: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE journal_search`); err != nil {
		t.Fatalf("drop journal_search: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
CREATE TABLE journal_search (
  rowid INTEGER PRIMARY KEY,
  project_id TEXT NOT NULL,
  journal_entry_id TEXT NOT NULL,
  session_id TEXT,
  entry_type TEXT NOT NULL,
  scope TEXT NOT NULL,
  message TEXT NOT NULL CHECK(message = 'prior derived')
)`); err != nil {
		t.Fatalf("create failing journal_search: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `
INSERT INTO journal_search(rowid, project_id, journal_entry_id, session_id, entry_type, scope, message)
VALUES (?, ?, 'legacy-journal', 'hsid-rekey', 'decision', 'rekey', 'prior derived')`, rowID, legacyID); err != nil {
		t.Fatalf("seed prior derived row: %v", err)
	}
	if _, err := store.EnsureProject(ctx, root); err == nil {
		t.Fatal("EnsureProject() error = nil, want journal parity rebuild failure")
	}
	var canonicalProjectID, derivedProjectID string
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM journal_entries WHERE id = 'legacy-journal'`).Scan(&canonicalProjectID); err != nil {
		t.Fatalf("read canonical journal row after rollback: %v", err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT project_id FROM journal_search WHERE journal_entry_id = 'legacy-journal'`).Scan(&derivedProjectID); err != nil {
		t.Fatalf("read derived journal row after rollback: %v", err)
	}
	if canonicalProjectID != legacyID || derivedProjectID != legacyID {
		t.Fatalf("project IDs after rollback canonical=%q derived=%q legacy=%q, want legacy", canonicalProjectID, derivedProjectID, legacyID)
	}
	if got := countIdentityRows(t, store, `SELECT COUNT(*) FROM projects WHERE id <> ?`, legacyID); got != 0 {
		t.Fatalf("generated projects after rollback = %d, want zero", got)
	}
}

func projectRootInTempDir(t *testing.T) project.Root {
	t.Helper()
	dir := t.TempDir()
	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot(%s) error = %v", dir, err)
	}
	return root
}

func countIdentityRows(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows query %q error = %v", query, err)
	}
	return count
}
