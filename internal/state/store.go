package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite3 "github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/levifig/loaf/internal/project"
)

const sqliteDriverName = "sqlite3"

const (
	openStoreRetryBudget = 5 * time.Second
	openStoreRetryStart  = 10 * time.Millisecond
	openStoreRetryCap    = 100 * time.Millisecond
)

// Store owns a SQLite connection for Loaf operational state.
type Store struct {
	db       *sql.DB
	path     string
	readOnly bool
}

// OpenStore opens an existing SQLite database path.
func OpenStore(path string) (*Store, error) {
	deadline := time.Now().Add(openStoreRetryBudget)
	delay := openStoreRetryStart
	for {
		db, openErr := sql.Open(sqliteDriverName, sqliteDSN(path))
		if openErr != nil {
			if !retryableSQLiteOpenError(openErr) || time.Now().After(deadline) {
				return nil, fmt.Errorf("open state database: %w", openErr)
			}
			if sleepErr := sleepBeforeSQLiteRetry(deadline, delay); sleepErr != nil {
				return nil, fmt.Errorf("open state database: %w", openErr)
			}
			delay = nextSQLiteRetryDelay(delay)
			continue
		}
		db.SetMaxOpenConns(1)
		pingErr := db.Ping()
		if pingErr == nil {
			return &Store{db: db, path: path}, nil
		} else {
			if closeErr := db.Close(); closeErr != nil {
				return nil, fmt.Errorf("ping state database: %w (close state database: %v)", pingErr, closeErr)
			}
			if !retryableSQLiteOpenError(pingErr) || time.Now().After(deadline) {
				return nil, fmt.Errorf("ping state database: %w", pingErr)
			}
			if sleepErr := sleepBeforeSQLiteRetry(deadline, delay); sleepErr != nil {
				return nil, fmt.Errorf("ping state database: %w", pingErr)
			}
			delay = nextSQLiteRetryDelay(delay)
		}
	}
}

func retryableSQLiteOpenError(err error) bool {
	return errors.Is(err, sqlite3.BUSY) || errors.Is(err, sqlite3.LOCKED)
}

func sleepBeforeSQLiteRetry(deadline time.Time, delay time.Duration) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return context.DeadlineExceeded
	}
	if delay >= remaining {
		delay = remaining
		time.Sleep(delay)
		return context.DeadlineExceeded
	}
	time.Sleep(delay)
	return nil
}

func nextSQLiteRetryDelay(delay time.Duration) time.Duration {
	delay *= 2
	if delay > openStoreRetryCap {
		return openStoreRetryCap
	}
	return delay
}

// OpenStoreReadOnly opens an existing SQLite database without creating or mutating it.
func OpenStoreReadOnly(path string) (*Store, error) {
	return openStoreReadOnly(path, sqliteReadOnlyDSN(path))
}

func openStoreReadOnlyForBackup(path string) (*Store, error) {
	return openStoreReadOnly(path, sqliteReadOnlyBackupDSN(path))
}

func openStoreReadOnly(path, dsn string) (*Store, error) {
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open state database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping state database read-only: %w", err)
	}
	var schemaVersion int
	if err := db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion); err != nil {
		db.Close()
		return nil, fmt.Errorf("validate state database read-only: %w", err)
	}
	return &Store{db: db, path: path, readOnly: true}, nil
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "journal_mode(wal)")
	values.Add("_pragma", "synchronous(full)")
	values.Add("_pragma", "foreign_keys(on)")
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: values.Encode(),
	}).String()
}

func sqliteReadOnlyDSN(path string) string {
	values := url.Values{}
	values.Add("mode", "ro")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "foreign_keys(on)")
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: values.Encode(),
	}).String()
}

func sqliteReadOnlyBackupDSN(path string) string {
	values := url.Values{}
	values.Add("mode", "ro")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "synchronous(full)")
	values.Add("_pragma", "foreign_keys(on)")
	return (&url.URL{
		Scheme:   "file",
		Path:     filepath.ToSlash(path),
		RawQuery: values.Encode(),
	}).String()
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Path returns the opened database path.
func (s *Store) Path() string {
	return s.path
}

// Initialize creates the global database, applies migrations, and records the project row.
func Initialize(ctx context.Context, root project.Root, resolver PathResolver) (Status, error) {
	path, err := resolver.DatabasePath(root)
	if err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Status{}, fmt.Errorf("create state database directory: %w", err)
	}

	store, err := OpenStore(path)
	if err != nil {
		return Status{}, err
	}
	defer store.Close()

	bootstrapped, err := store.BootstrapIfEmpty(ctx)
	if err != nil {
		return Status{}, err
	}
	if !bootstrapped {
		if err := store.RequireCurrentSchema(ctx); err != nil {
			return Status{}, err
		}
	}
	if err := store.UpsertProject(ctx, root); err != nil {
		return Status{}, err
	}
	return Inspect(root, resolver)
}

// ApplyMigrations applies all Go-owned schema migrations.
func (s *Store) ApplyMigrations(ctx context.Context) error {
	return ApplyMigrations(ctx, s.db, SchemaMigrations())
}

// SchemaVersion returns the highest applied migration version.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var version sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	if !version.Valid {
		return 0, nil
	}
	return int(version.Int64), nil
}

// DatabasePath returns the SQLite path backing this store.
func (s *Store) DatabasePath() string {
	return s.path
}

// ValidateCurrentSchema rejects version drift, missing migrations, and checksum drift.
func (s *Store) ValidateCurrentSchema(ctx context.Context) (int, error) {
	version, err := s.SchemaVersion(ctx)
	if err != nil {
		return 0, err
	}
	if !acceptableSchemaVersion(version) {
		return version, fmt.Errorf("schema version %d does not match expected version %d", version, CurrentSchemaVersion())
	}
	for _, migration := range SchemaMigrations() {
		var checksum string
		err := s.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).Scan(&checksum)
		switch {
		case err == nil && checksum != migration.Checksum():
			return version, fmt.Errorf("schema migration %d checksum does not match Go-owned migration", migration.Version)
		case errors.Is(err, sql.ErrNoRows):
			return version, fmt.Errorf("schema migration %d is missing", migration.Version)
		case err != nil:
			return version, fmt.Errorf("read schema migration %d: %w", migration.Version, err)
		}
	}
	if err := s.validateOptionalJournalFirstMigration(ctx); err != nil {
		return version, err
	}
	return version, nil
}

// validateOptionalJournalFirstMigration verifies the journal-first migration
// row, when present, matches the Go-owned migration checksum. The migration is
// applied out-of-band (never auto-applied on open), so its absence is valid on
// a pre-migration database; only a recorded-but-drifted checksum is an error.
func (s *Store) validateOptionalJournalFirstMigration(ctx context.Context) error {
	migration := JournalFirstMigration()
	var checksum string
	err := s.db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).Scan(&checksum)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("read schema migration %d: %w", migration.Version, err)
	case checksum != migration.Checksum():
		return fmt.Errorf("schema migration %d checksum does not match Go-owned migration", migration.Version)
	default:
		return nil
	}
}

// ValidateProjectPathInvariants rejects inconsistent durable project path metadata.
func (s *Store) ValidateProjectPathInvariants(ctx context.Context) error {
	diagnostics, valid, err := inspectProjectPathInvariants(ctx, s)
	if err != nil {
		return err
	}
	if valid {
		return nil
	}
	if len(diagnostics) == 0 {
		return fmt.Errorf("project path invariants are invalid")
	}
	return fmt.Errorf("%s", diagnostics[0].Message)
}

// AppliedMigrationCount returns the number of applied migrations.
func (s *Store) AppliedMigrationCount(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count schema migrations: %w", err)
	}
	return count, nil
}

// ApplyMigrations applies migrations in order and rejects checksum drift.
func ApplyMigrations(ctx context.Context, db *sql.DB, migrations []SchemaMigration) error {
	if err := validateSchemaMigrations(migrations); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, schemaMigrationsDDL); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	for _, migration := range migrations {
		if err := applyMigration(ctx, tx, migration); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}

func validateSchemaMigrations(migrations []SchemaMigration) error {
	previousVersion := 0
	for _, migration := range migrations {
		if migration.Version <= previousVersion {
			return fmt.Errorf("schema migration version %d must be greater than previous version %d", migration.Version, previousVersion)
		}
		if strings.TrimSpace(migration.Name) == "" {
			return fmt.Errorf("schema migration %d must have a name", migration.Version)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			return fmt.Errorf("schema migration %d SQL cannot be empty", migration.Version)
		}
		previousVersion = migration.Version
	}
	return nil
}

func applyMigration(ctx context.Context, tx *sql.Tx, migration SchemaMigration) error {
	var checksum string
	err := tx.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).Scan(&checksum)
	switch {
	case err == nil:
		if checksum != migration.Checksum() {
			return fmt.Errorf("schema migration %d checksum mismatch", migration.Version)
		}
		return nil
	case errors.Is(err, sql.ErrNoRows):
		// Apply below.
	default:
		return fmt.Errorf("read schema migration %d: %w", migration.Version, err)
	}

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply schema migration %d: %w", migration.Version, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations (version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		migration.Version,
		migration.Name,
		migration.Checksum(),
		now,
	)
	if err != nil {
		return fmt.Errorf("record schema migration %d: %w", migration.Version, err)
	}
	return nil
}
