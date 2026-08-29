package sqlite

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
)

func TestContinuitySQLiteOpenCreatesPrivateStore(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state #% ü")
	store, err := Open(stateRoot, continuity.EnvironmentID("env.test:1"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if store.environmentID != "env.test:1" {
		t.Fatalf("environment id = %q, want env.test:1", store.environmentID)
	}
	assertPathMode(t, filepath.Join(stateRoot, "vnext"), os.ModeDir|0o700)
	databasePath := filepath.Join(stateRoot, "vnext", databaseFileName)
	assertPathMode(t, databasePath, 0o600)

	pragmas := []struct {
		query string
		want  int
	}{
		{query: "PRAGMA foreign_keys", want: 1},
		{query: "PRAGMA busy_timeout", want: 5000},
		{query: "PRAGMA synchronous", want: 2},
		{query: "PRAGMA trusted_schema", want: 0},
		{query: "PRAGMA application_id", want: applicationID},
		{query: "PRAGMA user_version", want: schemaVersion},
	}
	for _, pragma := range pragmas {
		var got int
		if err := store.db.QueryRow(pragma.query).Scan(&got); err != nil {
			t.Errorf("%s error = %v", pragma.query, err)
			continue
		}
		if got != pragma.want {
			t.Errorf("%s = %d, want %d", pragma.query, got, pragma.want)
		}
	}
	var journalMode string
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal mode = %q, want wal", journalMode)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		path := databasePath + suffix
		if _, err := os.Lstat(path); err == nil {
			assertPathMode(t, path, 0o600)
		} else if !os.IsNotExist(err) {
			t.Errorf("inspect %s: %v", path, err)
		}
	}
}

func TestContinuitySQLiteOpenReopensExactSchema(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	store, err := Open(stateRoot, "environment-a")
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	reopened, err := Open(stateRoot, "environment-b")
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestContinuitySQLiteConcurrentOpenInitializesOnce(t *testing.T) {
	t.Parallel()

	const openers = 12
	stateRoot := filepath.Join(testTempDir(t), "state")
	start := make(chan struct{})
	errorsByOpener := make([]error, openers)
	var wait sync.WaitGroup
	wait.Add(openers)
	for opener := 0; opener < openers; opener++ {
		opener := opener
		go func() {
			defer wait.Done()
			<-start
			store, err := Open(stateRoot, continuity.EnvironmentID("environment-"+string(rune('a'+opener))))
			if err == nil {
				err = store.Close()
			}
			errorsByOpener[opener] = err
		}()
	}
	close(start)
	wait.Wait()

	for opener, err := range errorsByOpener {
		if err != nil {
			t.Errorf("Open() caller %d error = %v", opener, err)
		}
	}
}

func TestContinuitySQLiteOpenRejectsInvalidRootAndEnvironment(t *testing.T) {
	t.Parallel()

	temp := testTempDir(t)
	tests := []struct {
		name        string
		stateRoot   string
		environment continuity.EnvironmentID
	}{
		{name: "empty root", stateRoot: "", environment: "environment-a"},
		{name: "relative root", stateRoot: "relative", environment: "environment-a"},
		{name: "unclean root", stateRoot: filepath.Join(temp, "state") + string(filepath.Separator) + ".", environment: "environment-a"},
		{name: "filesystem root", stateRoot: filepath.VolumeName(temp) + string(filepath.Separator), environment: "environment-a"},
		{name: "empty environment", stateRoot: filepath.Join(temp, "empty-environment"), environment: ""},
		{name: "environment with space", stateRoot: filepath.Join(temp, "space-environment"), environment: "environment a"},
		{name: "oversized environment", stateRoot: filepath.Join(temp, "large-environment"), environment: continuity.EnvironmentID(strings.Repeat("a", 129))},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			store, err := Open(test.stateRoot, test.environment)
			if err == nil {
				store.Close()
				t.Fatal("Open() error = nil, want refusal")
			}
		})
	}
}

func TestContinuitySQLiteOpenRejectsSymlinkedStateSurfaces(t *testing.T) {
	t.Parallel()

	t.Run("state root", func(t *testing.T) {
		t.Parallel()
		target := testTempDir(t)
		link := filepath.Join(testTempDir(t), "state-link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("create symlink: %v", err)
		}
		assertOpenRefused(t, link)
	})

	t.Run("private directory", func(t *testing.T) {
		t.Parallel()
		root := testTempDir(t)
		if err := os.Symlink(testTempDir(t), filepath.Join(root, "vnext")); err != nil {
			t.Skipf("create symlink: %v", err)
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
		target := filepath.Join(testTempDir(t), "target.sqlite")
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatalf("create target: %v", err)
		}
		file.Close()
		if err := os.Symlink(target, filepath.Join(privateDirectory, databaseFileName)); err != nil {
			t.Skipf("create symlink: %v", err)
		}
		assertOpenRefused(t, root)
	})

	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		suffix := suffix
		t.Run("sidecar "+suffix, func(t *testing.T) {
			t.Parallel()
			root := testTempDir(t)
			privateDirectory := filepath.Join(root, "vnext")
			if err := os.Mkdir(privateDirectory, 0o700); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			database, err := os.OpenFile(filepath.Join(privateDirectory, databaseFileName), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if err != nil {
				t.Fatalf("create database: %v", err)
			}
			database.Close()
			if err := os.Symlink(filepath.Join(testTempDir(t), "sidecar-target"), filepath.Join(privateDirectory, databaseFileName+suffix)); err != nil {
				t.Skipf("create sidecar symlink: %v", err)
			}
			assertOpenRefused(t, root)
		})
	}
}

func TestContinuitySQLiteOpenRejectsSchemaDrift(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	store, err := Open(stateRoot, "environment-a")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	databasePath := filepath.Join(stateRoot, "vnext", databaseFileName)
	db, err := sql.Open(sqliteDriverName, databaseDSN(databasePath))
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE intruder (id TEXT)`); err != nil {
		db.Close()
		t.Fatalf("create drift: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	assertOpenRefused(t, stateRoot)
}

func TestContinuitySQLiteOpenRejectsForeignIdentityOnEmptyDatabase(t *testing.T) {
	t.Parallel()

	stateRoot := filepath.Join(testTempDir(t), "state")
	privateDirectory := filepath.Join(stateRoot, "vnext")
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		t.Fatalf("create private directory: %v", err)
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("create database: %v", err)
	}
	file.Close()

	db, err := sql.Open(sqliteDriverName, databaseDSN(databasePath))
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	if _, err := db.Exec(`PRAGMA application_id = 42`); err != nil {
		db.Close()
		t.Fatalf("set foreign identity: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	assertOpenRefused(t, stateRoot)
}

func TestContinuitySQLiteCloseAcceptsNilStore(t *testing.T) {
	t.Parallel()

	var store *Store
	if err := store.Close(); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
}

func assertOpenRefused(t *testing.T, stateRoot string) {
	t.Helper()

	store, err := Open(stateRoot, "environment-a")
	if err == nil {
		store.Close()
		t.Fatal("Open() error = nil, want refusal")
	}
}
