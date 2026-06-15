package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/levifig/loaf/internal/project"
)

const (
	LegacyProjectDatabaseArchiveAction = "archive-legacy-project-database"
	LegacyProjectDatabaseNoopAction    = "no-legacy-project-database"
)

// RelationshipOriginRepairOptions controls a guarded relationship provenance backfill.
type RelationshipOriginRepairOptions struct {
	Origin string
	Apply  bool
}

// RelationshipOriginRepairResult describes a dry-run or applied relationship provenance repair.
type RelationshipOriginRepairResult struct {
	ContractVersion    int    `json:"contract_version"`
	DatabaseScope      string `json:"database_scope"`
	DatabasePath       string `json:"database_path"`
	BackupPath         string `json:"backup_path,omitempty"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path"`
	Origin             string `json:"origin"`
	Matched            int    `json:"matched"`
	Updated            int    `json:"updated"`
	Applied            bool   `json:"applied"`
	GeneratedAt        string `json:"generated_at"`
}

// LegacyProjectDatabaseArchiveResult describes a guarded legacy project database archive.
type LegacyProjectDatabaseArchiveResult struct {
	ContractVersion    int      `json:"contract_version"`
	DatabaseScope      string   `json:"database_scope"`
	ProjectRoot        string   `json:"project_root"`
	ProjectID          string   `json:"project_id"`
	ProjectName        string   `json:"project_name"`
	ProjectCurrentPath string   `json:"project_current_path"`
	DatabasePath       string   `json:"database_path"`
	LegacyDatabasePath string   `json:"legacy_database_path"`
	ArchivePath        string   `json:"archive_path,omitempty"`
	Action             string   `json:"action"`
	MatchedPaths       []string `json:"matched_paths"`
	ArchivedPaths      []string `json:"archived_paths"`
	Applied            bool     `json:"applied"`
	GeneratedAt        string   `json:"generated_at"`
	Warnings           []string `json:"warnings"`
}

// RepairMissingRelationshipOrigins backfills missing relationship origin values
// for the current project only. It is dry-run unless options.Apply is true.
func RepairMissingRelationshipOrigins(ctx context.Context, root project.Root, resolver PathResolver, options RelationshipOriginRepairOptions) (RelationshipOriginRepairResult, error) {
	if options.Origin != "imported" && options.Origin != "manual" {
		return RelationshipOriginRepairResult{}, fmt.Errorf("relationship origin must be imported or manual")
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		return RelationshipOriginRepairResult{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return RelationshipOriginRepairResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return RelationshipOriginRepairResult{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		return RelationshipOriginRepairResult{}, fmt.Errorf("open state database for relationship origin repair: %w", err)
	}
	defer store.Close()

	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return RelationshipOriginRepairResult{}, err
	}
	matched, err := store.countMissingRelationshipOrigins(ctx, identity.ID)
	if err != nil {
		return RelationshipOriginRepairResult{}, err
	}

	result := RelationshipOriginRepairResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      status.DatabaseScope,
		DatabasePath:       status.DatabasePath,
		ProjectID:          identity.ID,
		ProjectName:        identity.FriendlyName,
		ProjectCurrentPath: identity.CurrentPath,
		Origin:             options.Origin,
		Matched:            matched,
		Applied:            options.Apply,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if !options.Apply || matched == 0 {
		return result, nil
	}

	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		return RelationshipOriginRepairResult{}, fmt.Errorf("backup state database before relationship origin repair: %w", err)
	}
	result.BackupPath = backup.BackupPath

	updated, err := store.backfillMissingRelationshipOrigins(ctx, identity.ID, options.Origin, result.GeneratedAt)
	if err != nil {
		return RelationshipOriginRepairResult{}, err
	}
	result.Updated = updated
	return result, nil
}

// ArchiveLegacyProjectDatabase moves a migrated per-project SQLite database out
// of the legacy project path. It refuses to archive when migration is still due.
func ArchiveLegacyProjectDatabase(root project.Root, resolver PathResolver, apply bool) (LegacyProjectDatabaseArchiveResult, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return LegacyProjectDatabaseArchiveResult{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return LegacyProjectDatabaseArchiveResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return LegacyProjectDatabaseArchiveResult{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	plan, err := PreviewStorageHomeMigration(root, resolver)
	if err != nil {
		return LegacyProjectDatabaseArchiveResult{}, err
	}
	now := time.Now().UTC()
	result := LegacyProjectDatabaseArchiveResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      status.DatabaseScope,
		ProjectRoot:        root.Path(),
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		DatabasePath:       plan.DatabasePath,
		LegacyDatabasePath: plan.LegacyDatabasePath,
		MatchedPaths:       []string{},
		ArchivedPaths:      []string{},
		Applied:            apply,
		GeneratedAt:        now.Format(time.RFC3339Nano),
		Warnings:           []string{},
	}
	if plan.DatabasePath == plan.LegacyDatabasePath || !plan.LegacyDatabaseExists {
		result.Action = LegacyProjectDatabaseNoopAction
		return result, nil
	}
	if plan.Action != StorageHomeActionAlreadyMigrated || !plan.DatabaseExists {
		return LegacyProjectDatabaseArchiveResult{}, fmt.Errorf("legacy project database still needs migration; run `loaf state migrate storage-home --dry-run`")
	}

	archiveDir := filepath.Join(filepath.Dir(plan.DatabasePath), "legacy-archives")
	if isWithinRoot(archiveDir, root.Path()) {
		return LegacyProjectDatabaseArchiveResult{}, fmt.Errorf("legacy archive directory must be outside project root")
	}
	archivePath, err := nextLegacyProjectArchivePath(archiveDir, ProjectID(root), now)
	if err != nil {
		return LegacyProjectDatabaseArchiveResult{}, err
	}
	result.Action = LegacyProjectDatabaseArchiveAction
	result.ArchivePath = archivePath
	result.MatchedPaths = existingSQLiteFileSet(plan.LegacyDatabasePath)
	if len(result.MatchedPaths) == 0 {
		result.Action = LegacyProjectDatabaseNoopAction
		return result, nil
	}
	if !apply {
		result.Applied = false
		return result, nil
	}

	if err := os.MkdirAll(archiveDir, 0o700); err != nil {
		return LegacyProjectDatabaseArchiveResult{}, fmt.Errorf("create legacy archive directory: %w", err)
	}
	for _, sourcePath := range result.MatchedPaths {
		targetPath := archiveTargetPath(plan.LegacyDatabasePath, archivePath, sourcePath)
		if err := os.Rename(sourcePath, targetPath); err != nil {
			return LegacyProjectDatabaseArchiveResult{}, fmt.Errorf("archive legacy state file %s: %w", sourcePath, err)
		}
		result.ArchivedPaths = append(result.ArchivedPaths, targetPath)
	}
	result.Warnings = append(result.Warnings, "legacy database archived, not deleted")
	return result, nil
}

func (s *Store) countMissingRelationshipOrigins(ctx context.Context, projectID string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM relationships
WHERE project_id = ?
  AND (origin IS NULL OR TRIM(origin) = '')
`, projectID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count missing relationship origins: %w", err)
	}
	return count, nil
}

func (s *Store) backfillMissingRelationshipOrigins(ctx context.Context, projectID string, origin string, updatedAt string) (int, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE relationships
SET origin = ?,
    updated_at = ?
WHERE project_id = ?
  AND (origin IS NULL OR TRIM(origin) = '')
`, origin, updatedAt, projectID)
	if err != nil {
		return 0, fmt.Errorf("backfill missing relationship origins: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count backfilled relationship origins: %w", err)
	}
	return int(rows), nil
}

func existingSQLiteFileSet(databasePath string) []string {
	paths := []string{}
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		if regularFileExists(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

func archiveTargetPath(sourceDatabasePath string, archiveDatabasePath string, sourcePath string) string {
	switch sourcePath {
	case sourceDatabasePath + "-wal":
		return archiveDatabasePath + "-wal"
	case sourceDatabasePath + "-shm":
		return archiveDatabasePath + "-shm"
	default:
		return archiveDatabasePath
	}
}

func nextLegacyProjectArchivePath(archiveDir string, projectID string, now time.Time) (string, error) {
	stamp := fmt.Sprintf("%s-%09d", now.Format("20060102-150405"), now.Nanosecond())
	for i := 0; i < 1000; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%03d", i)
		}
		path := filepath.Join(archiveDir, fmt.Sprintf("legacy-project-%s-%s%s.sqlite", projectID, stamp, suffix))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, nil
		} else if err != nil {
			return "", fmt.Errorf("check legacy archive path: %w", err)
		}
	}
	return "", fmt.Errorf("allocate legacy archive path: too many archives for timestamp %s", stamp)
}
