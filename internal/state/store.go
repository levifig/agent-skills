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

	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/levifig/loaf/internal/project"
)

const sqliteDriverName = "sqlite3"

// Store owns a SQLite connection for Loaf operational state.
type Store struct {
	db   *sql.DB
	path string
}

// OpenStore opens an existing SQLite database path.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open(sqliteDriverName, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open state database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping state database: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

func sqliteDSN(path string) string {
	values := url.Values{}
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "journal_mode(wal)")
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

// Initialize creates the project database, applies migrations, and records the project row.
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

	if err := store.ApplyMigrations(ctx); err != nil {
		return Status{}, err
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

// UpsertProject records the project identity row after migrations are applied.
func (s *Store) UpsertProject(ctx context.Context, root project.Root) error {
	now := time.Now().UTC().Format(time.RFC3339)
	projectID := ProjectID(root)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO projects (id, identity_hash, created_at, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  identity_hash = excluded.identity_hash,
  updated_at = excluded.updated_at
`, projectID, projectID, now, now)
	if err != nil {
		return fmt.Errorf("upsert project: %w", err)
	}
	return nil
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
