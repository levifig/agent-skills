package state

import (
	"context"
	"database/sql"
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

// JournalSearchRepairOptions controls a global derived journal-search rebuild.
// The default is a read-only dry run; Apply requires a verified pre-repair
// backup before changing the live database.
type JournalSearchRepairOptions struct {
	Apply bool
}

// JournalSearchRepairError preserves the partial repair result, including a
// verified backup, when an apply fails before commit.
type JournalSearchRepairError struct {
	Result JournalSearchRepairResult
	Err    error
}

func (e *JournalSearchRepairError) Error() string {
	if e == nil || e.Err == nil {
		return "journal search repair failed"
	}
	message := e.Err.Error()
	if e.Result.BackupPath != "" {
		message += fmt.Sprintf("; preserved backup: %s (verified=%t)", e.Result.BackupPath, e.Result.BackupVerified)
	}
	return message
}

func (e *JournalSearchRepairError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// JournalSearchRepairResult describes the pre-repair parity and any rebuild.
type JournalSearchRepairResult struct {
	ContractVersion    int    `json:"contract_version"`
	DatabaseScope      string `json:"database_scope"`
	DatabasePath       string `json:"database_path"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path"`
	CanonicalRows      int    `json:"canonical_rows"`
	IndexRows          int    `json:"index_rows"`
	Missing            int    `json:"missing"`
	Extra              int    `json:"extra"`
	Changed            int    `json:"changed"`
	Applied            bool   `json:"applied"`
	BackupPath         string `json:"backup_path,omitempty"`
	BackupVerified     bool   `json:"backup_verified"`
	Rebuilt            int    `json:"rebuilt"`
	ParityVerified     bool   `json:"parity_verified"`
	GeneratedAt        string `json:"generated_at"`
}

// RepairJournalSearch rebuilds the derived journal-search index globally from
// canonical journal entries. Dry-run is read-only; apply is backup-first and
// verifies exact parity after the transaction commits.
func RepairJournalSearch(ctx context.Context, root project.Root, resolver PathResolver, options JournalSearchRepairOptions) (JournalSearchRepairResult, error) {
	return repairJournalSearch(ctx, root, resolver, options, nil)
}

type journalSearchRepairHook func(context.Context, *sql.Conn) error

func repairJournalSearch(ctx context.Context, root project.Root, resolver PathResolver, options JournalSearchRepairOptions, hook journalSearchRepairHook) (JournalSearchRepairResult, error) {
	status, err := Inspect(root, resolver)
	if err != nil {
		return JournalSearchRepairResult{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return JournalSearchRepairResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return JournalSearchRepairResult{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	result := JournalSearchRepairResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      status.DatabaseScope,
		DatabasePath:       status.DatabasePath,
		ProjectID:          status.ProjectID,
		ProjectName:        status.ProjectName,
		ProjectCurrentPath: status.ProjectCurrentPath,
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if !options.Apply {
		preParity, err := inspectJournalSearchParityReadOnly(ctx, status.DatabasePath)
		if err != nil {
			return JournalSearchRepairResult{}, err
		}
		populateJournalSearchRepairParity(&result, preParity)
		return result, nil
	}

	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("open state database for journal search repair: %w", err)}
	}
	defer store.Close()
	conn, err := store.db.Conn(ctx)
	if err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("obtain state database connection for journal search repair: %w", err)}
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("begin immediate journal search repair: %w", err)}
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	preParity, err := inspectJournalSearchParity(ctx, conn)
	if err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("inspect pre-repair journal search parity: %w", err)}
	}
	populateJournalSearchRepairParity(&result, preParity)
	result.CanonicalRows = preParity.CanonicalRows
	result.IndexRows = preParity.IndexRows
	result.Missing = preParity.Missing
	result.Extra = preParity.Extra
	result.Changed = preParity.Changed

	if preParity.Ready {
		if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
			return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("commit journal search no-op: %w", err)}
		}
		committed = true
		result.Applied = true
		result.ParityVerified = true
		return result, nil
	}

	backup, err := Backup(ctx, root, resolver)
	if err != nil {
		result.BackupPath = backup.BackupPath
		result.BackupVerified = backup.Verified
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("backup state database before journal search repair: %w", err)}
	}
	result.BackupPath = backup.BackupPath
	result.BackupVerified = backup.Verified
	if !backup.Verified || backup.JournalRetrievalReady || backup.JournalSearchParity != preParity {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("state database backup before journal search repair was not verified or did not match pre-repair parity")}
	}

	rebuilt, err := rebuildJournalSearch(ctx, conn)
	if err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("rebuild journal search: %w", err)}
	}
	result.Rebuilt = rebuilt
	if hook != nil {
		if err := hook(ctx, conn); err != nil {
			return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("journal search repair hook: %w", err)}
		}
	}
	postParity, err := inspectJournalSearchParity(ctx, conn)
	if err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("verify journal search repair parity: %w", err)}
	}
	if !postParity.Ready {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("journal search repair did not produce ready parity: canonical_rows=%d, index_rows=%d, missing=%d, extra=%d, changed=%d", postParity.CanonicalRows, postParity.IndexRows, postParity.Missing, postParity.Extra, postParity.Changed)}
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return result, &JournalSearchRepairError{Result: result, Err: fmt.Errorf("commit journal search repair: %w", err)}
	}
	committed = true
	result.Applied = true
	result.ParityVerified = true
	return result, nil
}

func populateJournalSearchRepairParity(result *JournalSearchRepairResult, parity JournalSearchParity) {
	result.CanonicalRows = parity.CanonicalRows
	result.IndexRows = parity.IndexRows
	result.Missing = parity.Missing
	result.Extra = parity.Extra
	result.Changed = parity.Changed
}

func inspectJournalSearchParityReadOnly(ctx context.Context, databasePath string) (JournalSearchParity, error) {
	store, err := OpenStoreReadOnly(databasePath)
	if err != nil {
		return JournalSearchParity{}, fmt.Errorf("open state database read-only for journal search repair: %w", err)
	}
	defer store.Close()
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		return JournalSearchParity{}, fmt.Errorf("inspect journal search parity: %w", err)
	}
	return parity, nil
}

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
