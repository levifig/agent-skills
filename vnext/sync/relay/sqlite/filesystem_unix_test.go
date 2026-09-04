//go:build !windows

package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelaySQLiteOpenCreatesPrivateFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	databasePath := filepath.Join(directory, "relay.sqlite")
	store, err := Open(databasePath, allowTestVerifier{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { closeTestStore(t, store) })
	assertRelayFileMode(t, databasePath, 0o600)
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		path := databasePath + suffix
		if _, err := os.Lstat(path); err == nil {
			assertRelayFileMode(t, path, 0o600)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect SQLite sidecar %s: %v", suffix, err)
		}
	}
}

func TestRelaySQLiteOpenRejectsInsecureDirectoryDatabaseAndSymlinks(t *testing.T) {
	t.Parallel()

	t.Run("directory mode", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "relay-state")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create relay directory: %v", err)
		}
		if err := os.Chmod(directory, 0o770); err != nil {
			t.Fatalf("weaken relay directory: %v", err)
		}
		assertRelayOpenRefused(t, filepath.Join(directory, "relay.sqlite"))
	})

	t.Run("writable ancestor", func(t *testing.T) {
		ancestor := filepath.Join(t.TempDir(), "shared")
		directory := filepath.Join(ancestor, "relay-state")
		if err := os.Mkdir(ancestor, 0o700); err != nil {
			t.Fatalf("create ancestor directory: %v", err)
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create relay directory: %v", err)
		}
		if err := os.Chmod(ancestor, 0o777); err != nil {
			t.Fatalf("weaken ancestor directory: %v", err)
		}
		assertRelayOpenRefused(t, filepath.Join(directory, "relay.sqlite"))
	})

	t.Run("database mode", func(t *testing.T) {
		databasePath := filepath.Join(t.TempDir(), "relay.sqlite")
		file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("create relay database: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close relay database: %v", err)
		}
		if err := os.Chmod(databasePath, 0o640); err != nil {
			t.Fatalf("weaken relay database: %v", err)
		}
		assertRelayOpenRefused(t, databasePath)
	})

	t.Run("directory symlink", func(t *testing.T) {
		parent := t.TempDir()
		target := t.TempDir()
		link := filepath.Join(parent, "linked-state")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("create directory symlink: %v", err)
		}
		databasePath := filepath.Join(link, "relay.sqlite")
		assertRelayOpenRefused(t, databasePath)
		if _, err := os.Lstat(filepath.Join(target, "relay.sqlite")); !os.IsNotExist(err) {
			t.Fatalf("Open created a database through a symlink ancestor: %v", err)
		}
	})

	t.Run("database symlink", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target.sqlite")
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("create symlink target: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close symlink target: %v", err)
		}
		databasePath := filepath.Join(t.TempDir(), "relay.sqlite")
		if err := os.Symlink(target, databasePath); err != nil {
			t.Skipf("create database symlink: %v", err)
		}
		assertRelayOpenRefused(t, databasePath)
	})

	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		suffix := suffix
		t.Run("sidecar symlink "+suffix, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "relay.sqlite")
			file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if err != nil {
				t.Fatalf("create relay database: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("close relay database: %v", err)
			}
			if err := os.Symlink(filepath.Join(t.TempDir(), "target"), databasePath+suffix); err != nil {
				t.Skipf("create sidecar symlink: %v", err)
			}
			assertRelayOpenRefused(t, databasePath)
		})
	}
}

func TestRelayDatabasePathIdentityDetectsReplacement(t *testing.T) {
	t.Parallel()

	t.Run("database", func(t *testing.T) {
		databasePath := filepath.Join(t.TempDir(), "relay.sqlite")
		identity, err := prepareDatabasePath(databasePath)
		if err != nil {
			t.Fatalf("prepareDatabasePath() error = %v", err)
		}
		if err := os.Rename(databasePath, databasePath+".old"); err != nil {
			t.Fatalf("rename database: %v", err)
		}
		file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("replace database: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close replacement database: %v", err)
		}
		if err := verifyDatabasePathIdentity(databasePath, identity); err == nil {
			t.Fatal("verifyDatabasePathIdentity(replaced database) error = nil, want refusal")
		}
	})

	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		directory := filepath.Join(root, "relay-state")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("create relay directory: %v", err)
		}
		databasePath := filepath.Join(directory, "relay.sqlite")
		identity, err := prepareDatabasePath(databasePath)
		if err != nil {
			t.Fatalf("prepareDatabasePath() error = %v", err)
		}
		if err := os.Rename(directory, directory+".old"); err != nil {
			t.Fatalf("rename relay directory: %v", err)
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("replace relay directory: %v", err)
		}
		file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("replace database: %v", err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close replacement database: %v", err)
		}
		if err := verifyDatabasePathIdentity(databasePath, identity); err == nil {
			t.Fatal("verifyDatabasePathIdentity(replaced directory) error = nil, want refusal")
		}
	})
}

func TestRelaySQLiteSecuresPreexistingSidecars(t *testing.T) {
	t.Parallel()

	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		suffix := suffix
		t.Run(suffix, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "relay.sqlite")
			for _, path := range []string{databasePath, databasePath + suffix} {
				file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatalf("create %s: %v", filepath.Base(path), err)
				}
				if err := file.Close(); err != nil {
					t.Fatalf("close %s: %v", filepath.Base(path), err)
				}
			}
			if err := os.Chmod(databasePath+suffix, 0o644); err != nil {
				t.Fatalf("weaken preexisting sidecar: %v", err)
			}
			if err := secureSQLiteSidecars(databasePath); err != nil {
				t.Fatalf("secureSQLiteSidecars() error = %v", err)
			}
			assertRelayFileMode(t, databasePath+suffix, 0o600)
		})
	}
}

func assertRelayOpenRefused(t *testing.T, databasePath string) {
	t.Helper()
	store, err := Open(databasePath, allowTestVerifier{})
	if err == nil {
		store.Close()
		t.Fatal("Open() error = nil, want refusal")
	}
}

func assertRelayFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("mode for %s = %o, want %o", path, info.Mode().Perm(), want)
	}
}
