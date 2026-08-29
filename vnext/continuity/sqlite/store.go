// Package sqlite persists the closed continuity fact model in one local SQLite
// database without exposing a raw database handle.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/levifig/loaf/vnext/continuity"
)

const (
	databaseFileName = "continuity.sqlite"
	sqliteDriverName = "sqlite3"
	openRetryBudget  = 5 * time.Second
	openRetryStart   = 10 * time.Millisecond
	openRetryMaximum = 100 * time.Millisecond
)

// Store owns one private continuity database connection.
type Store struct {
	mu            sync.RWMutex
	db            *sql.DB
	environmentID continuity.EnvironmentID
	wallMillis    func() int64
	closed        bool
}

// Open opens or creates the vNext continuity database below stateRoot.
func Open(stateRoot string, environmentID continuity.EnvironmentID) (*Store, error) {
	if err := validateEnvironmentID(environmentID); err != nil {
		return nil, err
	}
	privateDirectory, err := preparePrivateDirectory(stateRoot)
	if err != nil {
		return nil, err
	}
	databasePath := filepath.Join(privateDirectory, databaseFileName)
	before, err := prepareDatabaseFile(databasePath)
	if err != nil {
		return nil, err
	}
	if err := secureSQLiteSidecars(databasePath); err != nil {
		return nil, err
	}

	db, err := openDatabase(databasePath)
	if err != nil {
		return nil, err
	}
	closeOnError := func(openErr error) (*Store, error) {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("%w (close continuity database: %v)", openErr, closeErr)
		}
		return nil, openErr
	}

	after, err := inspectRegularPrivateFile(databasePath)
	if err != nil {
		return closeOnError(err)
	}
	if !os.SameFile(before, after) {
		return closeOnError(fmt.Errorf("continuity database changed while opening"))
	}

	empty, err := schemaIsEmpty(db)
	if err != nil {
		return closeOnError(err)
	}
	if empty {
		if _, err := initializeSchemaIfEmpty(db); err != nil {
			return closeOnError(err)
		}
	} else {
		if err := migrateSchema(db); err != nil {
			return closeOnError(err)
		}
	}
	if err := validateSchema(db); err != nil {
		return closeOnError(err)
	}
	if err := verifySQLiteFiles(databasePath); err != nil {
		return closeOnError(err)
	}
	return &Store{
		db:            db,
		environmentID: environmentID,
		wallMillis:    func() int64 { return time.Now().UnixMilli() },
	}, nil
}

// Close closes the continuity database connection.
func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed || store.db == nil {
		return nil
	}
	store.closed = true
	if err := store.db.Close(); err != nil {
		return storeUnavailableProblemV1()
	}
	return nil
}

func validateEnvironmentID(environmentID continuity.EnvironmentID) error {
	if !validOpaqueID(string(environmentID)) {
		return fmt.Errorf("environment id must be 1-128 characters from A-Z, a-z, 0-9, _, ., :, or -")
	}
	return nil
}

func validOpaqueID(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		switch {
		case character >= 'A' && character <= 'Z':
		case character >= 'a' && character <= 'z':
		case character >= '0' && character <= '9':
		case strings.ContainsRune("_.:-", character):
		default:
			return false
		}
	}
	return true
}

func preparePrivateDirectory(stateRoot string) (string, error) {
	if stateRoot == "" || !filepath.IsAbs(stateRoot) {
		return "", fmt.Errorf("state root must be an absolute path")
	}
	if filepath.Clean(stateRoot) != stateRoot {
		return "", fmt.Errorf("state root must already be clean")
	}
	if filepath.Dir(stateRoot) == stateRoot {
		return "", fmt.Errorf("state root cannot be a filesystem root")
	}
	if err := validateStateRootLocationPlatform(stateRoot); err != nil {
		return "", err
	}

	var stateRootBefore os.FileInfo
	if info, err := os.Lstat(stateRoot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("state root must be a real directory")
		}
		if err := validateStateRootPlatform(stateRoot, info); err != nil {
			return "", err
		}
		stateRootBefore = info
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect state root: %w", err)
	}

	privateDirectory := filepath.Join(stateRoot, "vnext")
	var privateDirectoryBefore os.FileInfo
	if info, err := os.Lstat(privateDirectory); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("continuity state directory must be a real directory")
		}
		if err := validatePrivateDirectoryPlatform(privateDirectory, info); err != nil {
			return "", err
		}
		privateDirectoryBefore = info
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect continuity state directory: %w", err)
	}
	if err := os.MkdirAll(privateDirectory, 0o700); err != nil {
		return "", fmt.Errorf("create continuity state directory: %w", err)
	}
	stateRootAfter, err := os.Lstat(stateRoot)
	if err != nil {
		return "", fmt.Errorf("reinspect state root: %w", err)
	}
	if stateRootAfter.Mode()&os.ModeSymlink != 0 || !stateRootAfter.IsDir() {
		return "", fmt.Errorf("state root must be a real directory")
	}
	if stateRootBefore != nil && !os.SameFile(stateRootBefore, stateRootAfter) {
		return "", fmt.Errorf("state root changed while opening")
	}
	if err := validateStateRootPlatform(stateRoot, stateRootAfter); err != nil {
		return "", err
	}
	info, err := os.Lstat(privateDirectory)
	if err != nil {
		return "", fmt.Errorf("inspect continuity state directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("continuity state directory must be a real directory")
	}
	if privateDirectoryBefore != nil && !os.SameFile(privateDirectoryBefore, info) {
		return "", fmt.Errorf("continuity state directory changed while opening")
	}
	if err := validatePrivateDirectoryPlatform(privateDirectory, info); err != nil {
		return "", err
	}
	return privateDirectory, nil
}

func prepareDatabaseFile(path string) (os.FileInfo, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("continuity database must be a real file")
		}
		if err := validatePrivateFilePlatform(path, info); err != nil {
			return nil, err
		}
		return info, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect continuity database: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return inspectRegularPrivateFile(path)
		}
		return nil, fmt.Errorf("create continuity database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close new continuity database: %w", err)
	}
	return inspectRegularPrivateFile(path)
}

func inspectRegularPrivateFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect continuity database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("continuity database must be a real file")
	}
	if err := validatePrivateFilePlatform(path, info); err != nil {
		return nil, err
	}
	return info, nil
}

func databaseDSN(path string) string {
	values := url.Values{}
	values.Set("mode", "rwc")
	values.Set("_txlock", "immediate")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(on)")
	values.Add("_pragma", "journal_mode(wal)")
	values.Add("_pragma", "synchronous(full)")
	values.Add("_pragma", "trusted_schema(off)")
	return (&url.URL{
		Scheme:   "file",
		Path:     databaseURLPath(path),
		RawQuery: values.Encode(),
	}).String()
}

func openDatabase(path string) (*sql.DB, error) {
	deadline := time.Now().Add(openRetryBudget)
	delay := openRetryStart
	for {
		db, err := sql.Open(sqliteDriverName, databaseDSN(path))
		if err == nil {
			db.SetMaxOpenConns(1)
			db.SetMaxIdleConns(1)
			err = db.PingContext(context.Background())
		}
		if err == nil {
			return db, nil
		}
		if db != nil {
			if closeErr := db.Close(); closeErr != nil {
				return nil, fmt.Errorf("ping continuity database: %w (close continuity database: %v)", err, closeErr)
			}
		}
		if !retryableOpenError(err) || time.Now().After(deadline) {
			return nil, fmt.Errorf("ping continuity database: %w", err)
		}
		time.Sleep(delay)
		delay *= 2
		if delay > openRetryMaximum {
			delay = openRetryMaximum
		}
	}
}

func retryableOpenError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database is busy")
}

func schemaIsEmpty(db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect continuity schema inventory: %w", err)
	}
	return count == 0, nil
}

func verifySQLiteFiles(databasePath string) error {
	if _, err := inspectRegularPrivateFile(databasePath); err != nil {
		return err
	}
	return secureSQLiteSidecars(databasePath)
}

func secureSQLiteSidecars(databasePath string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		path := databasePath + suffix
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect continuity SQLite sidecar: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("continuity SQLite sidecar %s must be a real file", filepath.Base(path))
		}
		secured, err := securePrivateFilePlatform(path, info)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("secure continuity SQLite sidecar %s: %w", filepath.Base(path), err)
		}
		if secured.Mode()&os.ModeSymlink != 0 || !secured.Mode().IsRegular() || !os.SameFile(info, secured) {
			return fmt.Errorf("continuity SQLite sidecar %s changed while securing permissions", filepath.Base(path))
		}
		if err := validatePrivateFilePlatform(path, secured); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}
