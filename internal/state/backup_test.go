package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupCreatesSQLiteCopyOutsideRepository(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	result, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if result.DatabasePath != status.DatabasePath {
		t.Fatalf("DatabasePath = %q, want %q", result.DatabasePath, status.DatabasePath)
	}
	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty")
	}
	if isWithinRoot(result.BackupPath, root.Path()) {
		t.Fatalf("BackupPath = %q, want outside repository %q", result.BackupPath, root.Path())
	}
	if !strings.HasPrefix(result.BackupPath, filepath.Join(stateHome, "loaf", "backups")+string(filepath.Separator)) {
		t.Fatalf("BackupPath = %q, want under state home %q", result.BackupPath, stateHome)
	}
	if !strings.HasSuffix(result.BackupPath, ".sqlite") {
		t.Fatalf("BackupPath = %q, want sqlite suffix", result.BackupPath)
	}
	info, err := os.Stat(result.BackupPath)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if info.Size() <= 0 {
		t.Fatalf("backup file size = %d, want > 0", info.Size())
	}
	if result.Bytes != info.Size() {
		t.Fatalf("Bytes = %d, want %d", result.Bytes, info.Size())
	}
	if result.SHA256 != testFileSHA256(t, result.BackupPath) {
		t.Fatalf("SHA256 = %q, want actual backup digest", result.SHA256)
	}
	if result.CreatedAt == "" {
		t.Fatal("CreatedAt is empty")
	}
	if !result.Verified {
		t.Fatal("Verified = false, want true")
	}
	if result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", result.SchemaVersion, CurrentSchemaVersion())
	}
	if result.ProjectCount != 1 {
		t.Fatalf("ProjectCount = %d, want 1", result.ProjectCount)
	}
	if result.ProjectID != status.ProjectID {
		t.Fatalf("ProjectID = %q, want %q", result.ProjectID, status.ProjectID)
	}
	if result.ProjectName != status.ProjectName {
		t.Fatalf("ProjectName = %q, want %q", result.ProjectName, status.ProjectName)
	}
	if result.ProjectCurrentPath != status.ProjectCurrentPath {
		t.Fatalf("ProjectCurrentPath = %q, want %q", result.ProjectCurrentPath, status.ProjectCurrentPath)
	}
	if result.IntegrityCheck != "ok" {
		t.Fatalf("IntegrityCheck = %q, want ok", result.IntegrityCheck)
	}
	if result.ForeignKeyCheck != "ok" {
		t.Fatalf("ForeignKeyCheck = %q, want ok", result.ForeignKeyCheck)
	}
	assertNoSQLiteSidecars(t, result.BackupPath)

	backupStore, err := OpenStoreReadOnly(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStoreReadOnly(backup) error = %v", err)
	}
	defer backupStore.Close()
	version, err := backupStore.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("backup SchemaVersion() error = %v", err)
	}
	if version != CurrentSchemaVersion() {
		t.Fatalf("backup schema version = %d, want %d", version, CurrentSchemaVersion())
	}
	assertNoSQLiteSidecars(t, result.BackupPath)
}

func TestBackupReportsGlobalProjectCount(t *testing.T) {
	firstRoot := projectRoot(t)
	secondRoot := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), firstRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(firstRoot) error = %v", err)
	}
	if _, err := Initialize(context.Background(), secondRoot, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize(secondRoot) error = %v", err)
	}

	result, err := Backup(context.Background(), firstRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.ProjectCount != 2 {
		t.Fatalf("ProjectCount = %d, want 2", result.ProjectCount)
	}
}

func TestVerifyBackupReportsAllProjectsWithoutLiveState(t *testing.T) {
	firstRoot := projectRoot(t)
	secondRoot := projectRoot(t)
	stateHome := t.TempDir()
	firstStatus, err := Initialize(context.Background(), firstRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(firstRoot) error = %v", err)
	}
	secondStatus, err := Initialize(context.Background(), secondRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize(secondRoot) error = %v", err)
	}
	backup, err := Backup(context.Background(), firstRoot, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if err := os.Remove(firstStatus.DatabasePath); err != nil {
		t.Fatalf("remove live database error = %v", err)
	}

	result, err := VerifyBackup(context.Background(), backup.BackupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}

	if result.ContractVersion != StateJSONContractVersion {
		t.Fatalf("ContractVersion = %d, want %d", result.ContractVersion, StateJSONContractVersion)
	}
	if result.DatabaseScope != "global" {
		t.Fatalf("DatabaseScope = %q, want global", result.DatabaseScope)
	}
	if result.BackupPath != backup.BackupPath {
		t.Fatalf("BackupPath = %q, want %q", result.BackupPath, backup.BackupPath)
	}
	if result.Bytes != backup.Bytes {
		t.Fatalf("Bytes = %d, want %d", result.Bytes, backup.Bytes)
	}
	if result.SHA256 != backup.SHA256 {
		t.Fatalf("SHA256 = %q, want %q", result.SHA256, backup.SHA256)
	}
	if !result.Verified {
		t.Fatal("Verified = false, want true")
	}
	if result.SchemaVersion != CurrentSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", result.SchemaVersion, CurrentSchemaVersion())
	}
	if result.ProjectCount != 2 || len(result.Projects) != 2 {
		t.Fatalf("projects = %d/%d, want two projects", result.ProjectCount, len(result.Projects))
	}
	seen := map[string]bool{}
	for _, project := range result.Projects {
		seen[project.ID] = true
		if project.DatabasePath != backup.BackupPath {
			t.Fatalf("project DatabasePath = %q, want backup path %q", project.DatabasePath, backup.BackupPath)
		}
	}
	if !seen[firstStatus.ProjectID] || !seen[secondStatus.ProjectID] {
		t.Fatalf("verified projects = %#v, want %q and %q", seen, firstStatus.ProjectID, secondStatus.ProjectID)
	}
	if result.IntegrityCheck != "ok" {
		t.Fatalf("IntegrityCheck = %q, want ok", result.IntegrityCheck)
	}
	if result.ForeignKeyCheck != "ok" {
		t.Fatalf("ForeignKeyCheck = %q, want ok", result.ForeignKeyCheck)
	}
}

func TestBackupCreatesTimestampedFilesWithoutOverwriting(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	first, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("first Backup() error = %v", err)
	}
	second, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("second Backup() error = %v", err)
	}

	if first.BackupPath == second.BackupPath {
		t.Fatalf("backup path reused: %q", first.BackupPath)
	}
	if _, err := os.Stat(first.BackupPath); err != nil {
		t.Fatalf("first backup missing after second backup: %v", err)
	}
	if _, err := os.Stat(second.BackupPath); err != nil {
		t.Fatalf("second backup missing: %v", err)
	}
}

func TestNextBackupPathIncludesNanoseconds(t *testing.T) {
	path, err := nextBackupPath(t.TempDir(), time.Date(2026, 5, 28, 21, 15, 41, 193211000, time.UTC))
	if err != nil {
		t.Fatalf("nextBackupPath() error = %v", err)
	}
	if !strings.HasSuffix(path, "loaf-20260528-211541-193211000.sqlite") {
		t.Fatalf("path = %q, want nanosecond timestamp", path)
	}
}

func TestBackupRequiresInitializedSQLiteState(t *testing.T) {
	root := projectRoot(t)
	_, err := Backup(context.Background(), root, PathResolver{StateHome: t.TempDir()})
	if err == nil {
		t.Fatal("Backup() error = nil, want missing-state error")
	}
	if !strings.Contains(err.Error(), "SQLite state database is not initialized") {
		t.Fatalf("error = %v, want initialized-state message", err)
	}
}

func TestBackupRejectsInvalidSQLiteState(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	path := mustDatabasePath(t, root, stateHome)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Backup(context.Background(), root, PathResolver{StateHome: stateHome})
	if err == nil {
		t.Fatal("Backup() error = nil, want invalid-state error")
	}
	if !strings.Contains(err.Error(), "state database is invalid; run `loaf state doctor`") {
		t.Fatalf("error = %v, want doctor message", err)
	}
}

func TestVerifyNoForeignKeyViolationsReportsDetails(t *testing.T) {
	root := projectRoot(t)
	stateHome := t.TempDir()
	if _, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome}); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store := openTestStore(t, root, stateHome)
	if _, err := store.db.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys error = %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO aliases (id, project_id, entity_kind, entity_id, namespace, alias, created_at, updated_at)
VALUES ('alias-orphaned-project', 'project-missing', 'task', 'task-missing', 'task', 'TASK-MISSING', '2026-06-13T10:00:00Z', '2026-06-13T10:00:00Z')
`); err != nil {
		t.Fatalf("insert orphaned alias fixture error = %v", err)
	}

	_, err := verifyNoForeignKeyViolations(context.Background(), store)
	if err == nil {
		t.Fatal("verifyNoForeignKeyViolations() error = nil, want detailed violation")
	}
	for _, want := range []string{"SQLite foreign key violation", "aliases", "projects", "constraint"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
}

func assertNoSQLiteSidecars(t *testing.T, path string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("backup sidecar %s exists or stat failed: %v", sidecar, err)
		}
	}
}

func testFileSHA256(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
