//go:build !windows

package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestContinuitySQLiteOpenRejectsInsecureModes(t *testing.T) {
	t.Parallel()

	t.Run("state root", func(t *testing.T) {
		t.Parallel()
		root := filepath.Join(testTempDir(t), "state")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatalf("create state root: %v", err)
		}
		if err := os.Chmod(root, 0o770); err != nil {
			t.Fatalf("weaken state root: %v", err)
		}
		assertOpenRefused(t, root)
	})

	t.Run("private directory", func(t *testing.T) {
		t.Parallel()
		root := testTempDir(t)
		if err := os.Mkdir(filepath.Join(root, "vnext"), 0o755); err != nil {
			t.Fatalf("create private directory: %v", err)
		}
		assertOpenRefused(t, root)
	})

	t.Run("database", func(t *testing.T) {
		t.Parallel()
		root := testTempDir(t)
		privateDirectory := filepath.Join(root, "vnext")
		if err := os.Mkdir(privateDirectory, 0o700); err != nil {
			t.Fatalf("create private directory: %v", err)
		}
		file, err := os.OpenFile(filepath.Join(privateDirectory, databaseFileName), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
		if err != nil {
			t.Fatalf("create database: %v", err)
		}
		file.Close()
		assertOpenRefused(t, root)
	})
}

func TestContinuitySQLiteOpenRejectsUnixSymlinkAncestorBeforeCreation(t *testing.T) {
	t.Parallel()

	parent := testTempDir(t)
	target := testTempDir(t)
	link := filepath.Join(parent, "linked-parent")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create symlink: %v", err)
	}
	stateRoot := filepath.Join(link, "new-state")
	assertOpenRefused(t, stateRoot)
	if _, err := os.Lstat(filepath.Join(target, "new-state")); !os.IsNotExist(err) {
		t.Fatalf("Open created state through a symlink ancestor: %v", err)
	}
}

func TestContinuitySQLiteSecuresPreexistingSidecarsInsidePrivateDirectory(t *testing.T) {
	t.Parallel()

	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		suffix := suffix
		t.Run(suffix, func(t *testing.T) {
			t.Parallel()
			root := testTempDir(t)
			privateDirectory := filepath.Join(root, "vnext")
			if err := os.Mkdir(privateDirectory, 0o700); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			databasePath := filepath.Join(privateDirectory, databaseFileName)
			for _, path := range []string{databasePath, databasePath + suffix} {
				mode := os.FileMode(0o600)
				if path != databasePath {
					mode = 0o644
				}
				file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, mode)
				if err != nil {
					t.Fatalf("create %s: %v", filepath.Base(path), err)
				}
				if err := file.Close(); err != nil {
					t.Fatalf("close %s: %v", filepath.Base(path), err)
				}
			}

			if err := secureSQLiteSidecars(databasePath); err != nil {
				t.Fatalf("secure sidecars: %v", err)
			}
			info, err := os.Lstat(databasePath + suffix)
			if err != nil {
				t.Fatalf("reinspect preexisting sidecar: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("preexisting sidecar mode = %o, want secured 0600", info.Mode().Perm())
			}
		})
	}
}

func assertPathMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	if info.Mode() != want {
		t.Fatalf("mode for %s = %v, want %v", path, info.Mode(), want)
	}
}
