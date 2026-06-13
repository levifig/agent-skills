package state

import (
	"context"
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
	if result.BackupPath == "" {
		t.Fatal("BackupPath is empty")
	}
	if isWithinRoot(result.BackupPath, root.Path()) {
		t.Fatalf("BackupPath = %q, want outside repository %q", result.BackupPath, root.Path())
	}
	if !strings.HasPrefix(result.BackupPath, filepath.Join(stateHome, "loaf", "projects")+string(filepath.Separator)) {
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
	if result.CreatedAt == "" {
		t.Fatal("CreatedAt is empty")
	}

	backupStore, err := OpenStore(result.BackupPath)
	if err != nil {
		t.Fatalf("OpenStore(backup) error = %v", err)
	}
	defer backupStore.Close()
	version, err := backupStore.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("backup SchemaVersion() error = %v", err)
	}
	if version != CurrentSchemaVersion() {
		t.Fatalf("backup schema version = %d, want %d", version, CurrentSchemaVersion())
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
