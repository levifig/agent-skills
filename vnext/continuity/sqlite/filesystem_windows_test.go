//go:build windows

package sqlite

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestContinuitySQLiteOpenRunsInsideWindowsUserProfile(t *testing.T) {
	stateRoot := filepath.Join(testTempDir(t), "state")
	store, err := Open(stateRoot, "windows-environment")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestContinuitySQLiteOpenRejectsWindowsReparseStateRoot(t *testing.T) {
	target := filepath.Join(testTempDir(t), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("create target: %v", err)
	}
	link := filepath.Join(testTempDir(t), "state-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create Windows symlink without developer-mode privilege: %v", err)
	}
	assertOpenRefused(t, link)
}

func TestContinuitySQLiteOpenRejectsWindowsReparseAncestorBeforeCreation(t *testing.T) {
	parent := testTempDir(t)
	target := testTempDir(t)
	link := filepath.Join(parent, "reparse-parent")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create Windows symlink without developer-mode privilege: %v", err)
	}
	stateRoot := filepath.Join(link, "new-state")
	assertOpenRefused(t, stateRoot)
	if _, err := os.Lstat(filepath.Join(target, "new-state")); !os.IsNotExist(err) {
		t.Fatalf("Open created state through a reparse ancestor: %v", err)
	}
}

func TestContinuitySQLiteRejectsWindowsUNCStateRoot(t *testing.T) {
	if err := validateStateRootLocationPlatform(`\\server\share\state`); err == nil {
		t.Fatal("UNC state root validation error = nil, want refusal")
	}
}

func TestContinuitySQLiteWindowsDSNHasEmptyAuthority(t *testing.T) {
	dsn := databaseDSN(`C:\Users\loaf user\state #%\vnext\continuity.sqlite`)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if parsed.Scheme != "file" || parsed.Host != "" {
		t.Fatalf("DSN scheme/host = %q/%q, want file with empty authority: %s", parsed.Scheme, parsed.Host, dsn)
	}
	if parsed.Path != `/C:/Users/loaf user/state #%/vnext/continuity.sqlite` {
		t.Fatalf("DSN path = %q, want drive-letter URL path", parsed.Path)
	}
}

func TestContinuitySQLiteOpenRejectsWindowsRootOutsideUserProfile(t *testing.T) {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		t.Skip("SystemRoot is unavailable")
	}
	assertOpenRefused(t, filepath.Join(systemRoot, "Temp", "loaf-continuity-contract"))
}

func assertPathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	if want&os.ModeDir != 0 && !info.IsDir() {
		t.Fatalf("%s is not a directory", path)
	}
	if want&os.ModeDir == 0 && !info.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file", path)
	}
}
