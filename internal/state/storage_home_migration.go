package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

const (
	StorageHomeActionCopy            = "copy-legacy-to-data"
	StorageHomeActionAlreadyMigrated = "already-migrated"
	StorageHomeActionNoLegacyState   = "no-legacy-state"
)

// StorageHomeMigrationPlan describes the XDG_STATE_HOME to XDG_DATA_HOME move.
type StorageHomeMigrationPlan struct {
	Version              int      `json:"version"`
	ProjectRoot          string   `json:"project_root"`
	DatabasePath         string   `json:"database_path"`
	LegacyDatabasePath   string   `json:"legacy_database_path"`
	DatabaseExists       bool     `json:"database_exists"`
	LegacyDatabaseExists bool     `json:"legacy_database_exists"`
	Action               string   `json:"action"`
	Applied              bool     `json:"applied"`
	Warnings             []string `json:"warnings,omitempty"`
}

// PreviewStorageHomeMigration reports whether an old XDG_STATE_HOME database
// can be copied into the durable XDG_DATA_HOME location.
func PreviewStorageHomeMigration(root project.Root, resolver PathResolver) (StorageHomeMigrationPlan, error) {
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	legacyPath, err := resolver.LegacyDatabasePath(root)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}

	plan := StorageHomeMigrationPlan{
		Version:            1,
		ProjectRoot:        root.Path(),
		DatabasePath:       databasePath,
		LegacyDatabasePath: legacyPath,
	}
	if databasePath == legacyPath {
		plan.Action = StorageHomeActionAlreadyMigrated
		return plan, nil
	}

	plan.DatabaseExists = regularFileExists(databasePath)
	plan.LegacyDatabaseExists = regularFileExists(legacyPath)
	switch {
	case plan.DatabaseExists:
		plan.Action = StorageHomeActionAlreadyMigrated
		if plan.LegacyDatabaseExists {
			plan.Warnings = append(plan.Warnings, "legacy state database remains in the old state home; leaving it untouched")
		}
	case plan.LegacyDatabaseExists:
		plan.Action = StorageHomeActionCopy
	default:
		plan.Action = StorageHomeActionNoLegacyState
	}
	return plan, nil
}

// ApplyStorageHomeMigration copies a legacy XDG_STATE_HOME database to the
// XDG_DATA_HOME location without deleting or rewriting the legacy file.
func ApplyStorageHomeMigration(ctx context.Context, root project.Root, resolver PathResolver) (StorageHomeMigrationPlan, error) {
	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if plan.Action != StorageHomeActionCopy {
		return plan, nil
	}

	legacyStore, err := OpenStore(plan.LegacyDatabasePath)
	if err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("open legacy state database: %w", err)
	}
	if _, err := legacyStore.SchemaVersion(ctx); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("read legacy state database schema: %w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("read legacy state database schema: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(plan.DatabasePath), 0o700); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("create data state directory: %w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, fmt.Errorf("create data state directory: %w", err)
	}
	if err := copySQLiteDatabase(ctx, legacyStore, plan.DatabasePath, 0o600); err != nil {
		if closeErr := legacyStore.Close(); closeErr != nil {
			return StorageHomeMigrationPlan{}, fmt.Errorf("%w; close legacy state database: %v", err, closeErr)
		}
		return StorageHomeMigrationPlan{}, err
	}
	if err := legacyStore.Close(); err != nil {
		return StorageHomeMigrationPlan{}, fmt.Errorf("close legacy state database: %w", err)
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		return StorageHomeMigrationPlan{}, err
	}
	if status.Mode != ModeSQLiteReady {
		return StorageHomeMigrationPlan{}, fmt.Errorf("copied state database is not ready: %s", status.Mode)
	}
	plan.Applied = true
	plan.DatabaseExists = true
	plan.LegacyDatabaseExists = true
	plan.Action = StorageHomeActionAlreadyMigrated
	plan.Warnings = append(plan.Warnings, "legacy state database left untouched; remove it manually after verifying the data-home database")
	return plan, nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func copySQLiteDatabase(ctx context.Context, source *Store, destination string, mode os.FileMode) error {
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("create destination state database: %s already exists", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat destination state database: %w", err)
	}

	copied := false
	defer func() {
		if !copied {
			_ = os.Remove(destination)
		}
	}()
	if _, err := source.db.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("copy state database: %w", err)
	}
	if err := os.Chmod(destination, mode); err != nil {
		return fmt.Errorf("set destination state database permissions: %w", err)
	}
	copied = true
	return nil
}
