package state

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"strings"
)

//go:embed migrations/0001_initial.sql
var initialSchemaSQL string

//go:embed migrations/0002_session_state_snapshots.sql
var sessionStateSnapshotsSQL string

const schemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY NOT NULL,
  name TEXT NOT NULL,
  checksum TEXT NOT NULL,
  applied_at TEXT NOT NULL
)`

// SchemaMigration is a Go-owned SQLite migration definition. The storage
// package applies these in version order once the SQLite driver is introduced.
type SchemaMigration struct {
	Version int
	Name    string
	SQL     string
}

// SchemaMigrations returns the ordered schema migrations for Loaf state.
func SchemaMigrations() []SchemaMigration {
	return []SchemaMigration{
		{
			Version: 1,
			Name:    "initial_operational_state",
			SQL:     normalizeMigrationSQL(initialSchemaSQL),
		},
		{
			Version: 2,
			Name:    "session_state_snapshots",
			SQL:     normalizeMigrationSQL(sessionStateSnapshotsSQL),
		},
	}
}

// CurrentSchemaVersion returns the highest Go-owned migration version.
func CurrentSchemaVersion() int {
	migrations := SchemaMigrations()
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].Version
}

// Checksum returns the deterministic content hash stored with applied migrations.
func (m SchemaMigration) Checksum() string {
	sum := sha256.Sum256([]byte(m.SQL))
	return hex.EncodeToString(sum[:])
}

func normalizeMigrationSQL(sql string) string {
	return strings.TrimSpace(sql) + "\n"
}
