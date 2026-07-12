package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// BackupResult describes a repository-external SQLite database backup.
type BackupResult struct {
	ContractVersion               int                        `json:"contract_version"`
	DatabaseScope                 string                     `json:"database_scope"`
	DatabasePath                  string                     `json:"database_path"`
	BackupPath                    string                     `json:"backup_path"`
	Bytes                         int64                      `json:"bytes"`
	SHA256                        string                     `json:"sha256"`
	CreatedAt                     string                     `json:"created_at"`
	Verified                      bool                       `json:"verified"`
	SchemaVersion                 int                        `json:"schema_version"`
	ProjectCount                  int                        `json:"project_count"`
	ProjectID                     string                     `json:"project_id"`
	ProjectName                   string                     `json:"project_name"`
	ProjectCurrentPath            string                     `json:"project_current_path"`
	IntegrityCheck                string                     `json:"integrity_check"`
	ForeignKeyCheck               string                     `json:"foreign_key_check"`
	JournalRetrievalReady         bool                       `json:"journal_retrieval_ready"`
	JournalSearchParity           JournalSearchParity        `json:"journal_search_parity"`
	JournalProvenanceIntegrity    JournalProvenanceIntegrity `json:"journal_provenance_integrity"`
	RecoveryTier                  string                     `json:"recovery_tier"`
	RequestedDestinationDirectory string                     `json:"requested_destination_directory"`
	ResolvedDestinationDirectory  string                     `json:"resolved_destination_directory"`
	DeviceLossProtected           bool                       `json:"device_loss_protected"`
	DeviceLossProtectionBasis     string                     `json:"device_loss_protection_basis"`
	SQLiteValid                   bool                       `json:"sqlite_valid"`
	RecoveryReady                 bool                       `json:"recovery_ready"`
	JournalWatermark              JournalWatermark           `json:"journal_watermark"`
}

// BackupVerificationResult describes a read-only verification of an existing SQLite backup.
type BackupVerificationResult struct {
	ContractVersion            int                        `json:"contract_version"`
	DatabaseScope              string                     `json:"database_scope"`
	BackupPath                 string                     `json:"backup_path"`
	Bytes                      int64                      `json:"bytes"`
	SHA256                     string                     `json:"sha256"`
	Verified                   bool                       `json:"verified"`
	SchemaVersion              int                        `json:"schema_version"`
	ProjectCount               int                        `json:"project_count"`
	Projects                   []ProjectIdentity          `json:"projects"`
	IntegrityCheck             string                     `json:"integrity_check"`
	ForeignKeyCheck            string                     `json:"foreign_key_check"`
	RestoreDatabasePath        string                     `json:"restore_database_path,omitempty"`
	RestorePreservePath        string                     `json:"restore_preserve_path,omitempty"`
	RestoreValidationCommands  []string                   `json:"restore_validation_commands,omitempty"`
	JournalRetrievalReady      bool                       `json:"journal_retrieval_ready"`
	JournalSearchParity        JournalSearchParity        `json:"journal_search_parity"`
	JournalProvenanceIntegrity JournalProvenanceIntegrity `json:"journal_provenance_integrity"`
	SQLiteValid                bool                       `json:"sqlite_valid"`
	RecoveryReady              bool                       `json:"recovery_ready"`
	JournalWatermark           JournalWatermark           `json:"journal_watermark"`
}

// JournalWatermark identifies the latest canonical journal entry present in a
// completed backup snapshot. An empty watermark is an intentional statement
// that the snapshot contained no journal entries.
type JournalWatermark struct {
	Present        bool   `json:"present"`
	JournalEntryID string `json:"journal_entry_id,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

const (
	RecoveryTierLocalRollback         = "local_rollback"
	RecoveryTierExternalDisasterCopy  = "external_disaster_copy"
	ProtectionBasisNone               = "none"
	ProtectionBasisOperatorSelected   = "operator_selected_non_temporary_external_destination"
	BackupDestinationNotAbsoluteCode  = "backup-destination-not-absolute"
	BackupDestinationVolatileCode     = "backup-destination-volatile"
	BackupDestinationNotDirectoryCode = "backup-destination-not-directory"
)

// BackupDestinationError is returned when an explicit destination cannot be
// used safely. Requested and Resolved preserve both the operator input and the
// symlink-resolved path used for containment diagnostics.
type BackupDestinationError struct {
	Code      string `json:"code"`
	Requested string `json:"requested"`
	Resolved  string `json:"resolved"`
}

// backupOperations contains per-invocation seams used by deterministic tests.
// Public callers pass nil and use the production operations below; no mutable
// process-global hooks are involved.
type backupOperations struct {
	now           func() time.Time
	volatileRoots []string
	afterReserve  func(string) error
	beforeVacuum  func(string) error
	afterVacuum   func(string) error
	verify        func(context.Context, string, project.Root) (backupVerification, error)
}

func (e *BackupDestinationError) Error() string {
	if e == nil {
		return "invalid backup destination"
	}
	return fmt.Sprintf("%s: requested=%q resolved=%q", e.Code, e.Requested, e.Resolved)
}

// Backup creates a timestamped SQLite backup under the project's state
// directory. It preserves the local rollback default used by existing callers.
func Backup(ctx context.Context, root project.Root, resolver PathResolver) (BackupResult, error) {
	return BackupWithDestination(ctx, root, resolver, "")
}

// BackupWithDestination creates a timestamped SQLite backup. An empty
// destinationDir uses the local state-home rollback directory. A non-empty
// destination must be an absolute, non-volatile directory and is classified
// as an operator-selected external disaster copy.
func BackupWithDestination(ctx context.Context, root project.Root, resolver PathResolver, destinationDir string) (BackupResult, error) {
	return backupWithDestination(ctx, root, resolver, destinationDir, nil)
}

func backupWithDestination(ctx context.Context, root project.Root, resolver PathResolver, destinationDir string, ops *backupOperations) (BackupResult, error) {
	if ops == nil {
		ops = &backupOperations{}
	}
	status, err := Inspect(root, resolver)
	if err != nil {
		return BackupResult{}, err
	}
	switch status.Mode {
	case ModeMarkdownOnly:
		return BackupResult{}, fmt.Errorf("SQLite state database is not initialized; run `loaf state init` or `loaf state migrate markdown --apply` first")
	case ModeInvalid:
		return BackupResult{}, fmt.Errorf("state database is invalid; run `loaf state doctor`")
	}

	recoveryTier := RecoveryTierLocalRollback
	protectionBasis := ProtectionBasisNone
	requestedDestination := destinationDir
	backupDir := filepath.Join(filepath.Dir(status.DatabasePath), "backups")
	if destinationDir != "" {
		recoveryTier = RecoveryTierExternalDisasterCopy
		protectionBasis = ProtectionBasisOperatorSelected
		backupDir = destinationDir
	}
	projectRoot, err := resolvedProjectRoot(root.Path())
	if err != nil {
		return BackupResult{}, err
	}
	resolvedDestination, err := prepareBackupDestination(backupDir, destinationDir != "", projectRoot, ops.volatileRoots)
	if err != nil {
		return BackupResult{}, err
	}
	backupDirForOutput := resolvedDestination
	if destinationDir == "" {
		backupDirForOutput = filepath.Clean(backupDir)
	}

	now := time.Now().UTC()
	if ops.now != nil {
		now = ops.now().UTC()
	}
	backupPath, err := reserveBackupPath(backupDirForOutput, now)
	if err != nil {
		return BackupResult{}, err
	}
	reservationCompleted := false
	defer func() {
		if !reservationCompleted {
			_ = os.Remove(backupPath)
		}
	}()
	partial := BackupResult{
		ContractVersion:               StateJSONContractVersion,
		DatabaseScope:                 "global",
		DatabasePath:                  status.DatabasePath,
		BackupPath:                    backupPath,
		CreatedAt:                     now.Format(time.RFC3339Nano),
		RecoveryTier:                  recoveryTier,
		RequestedDestinationDirectory: requestedDestination,
		ResolvedDestinationDirectory:  resolvedDestination,
		DeviceLossProtected:           false,
		DeviceLossProtectionBasis:     protectionBasis,
	}
	if ops.afterReserve != nil {
		if err := ops.afterReserve(backupPath); err != nil {
			return partial, err
		}
	}
	store, err := openStoreReadOnlyForBackup(status.DatabasePath)
	if err != nil {
		return partial, fmt.Errorf("open state database for backup: %w", err)
	}
	defer store.Close()

	if ops.beforeVacuum != nil {
		if err := ops.beforeVacuum(backupPath); err != nil {
			return partial, err
		}
	}
	if _, err := store.db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		return partial, fmt.Errorf("backup state database: %w", err)
	}
	reservationCompleted = true
	if ops.afterVacuum != nil {
		if err := ops.afterVacuum(backupPath); err != nil {
			return partial, err
		}
	}
	if err := os.Chmod(backupPath, 0o600); err != nil {
		return partial, fmt.Errorf("set state backup permissions: %w", err)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		return partial, fmt.Errorf("stat state backup: %w", err)
	}
	partial.Bytes = info.Size()
	sha256Sum, err := fileSHA256(backupPath)
	if err != nil {
		return partial, fmt.Errorf("checksum state backup: %w", err)
	}
	partial.SHA256 = sha256Sum
	verify := ops.verify
	if verify == nil {
		verify = verifyBackup
	}
	verification, err := verify(ctx, backupPath, root)
	if err != nil {
		return partial, err
	}

	return BackupResult{
		ContractVersion:               StateJSONContractVersion,
		DatabaseScope:                 "global",
		DatabasePath:                  status.DatabasePath,
		BackupPath:                    backupPath,
		Bytes:                         info.Size(),
		SHA256:                        sha256Sum,
		CreatedAt:                     now.Format(time.RFC3339Nano),
		Verified:                      verification.sqliteValid,
		SchemaVersion:                 verification.schemaVersion,
		ProjectCount:                  verification.projectCount,
		ProjectID:                     verification.projectID,
		ProjectName:                   verification.projectName,
		ProjectCurrentPath:            verification.projectCurrentPath,
		IntegrityCheck:                verification.integrityCheck,
		ForeignKeyCheck:               verification.foreignKeyCheck,
		JournalRetrievalReady:         verification.journalRetrievalReady,
		JournalSearchParity:           verification.journalSearchParity,
		RecoveryTier:                  recoveryTier,
		RequestedDestinationDirectory: requestedDestination,
		ResolvedDestinationDirectory:  resolvedDestination,
		DeviceLossProtected:           false,
		DeviceLossProtectionBasis:     protectionBasis,
		SQLiteValid:                   verification.sqliteValid,
		JournalProvenanceIntegrity:    verification.journalProvenanceIntegrity,
		RecoveryReady:                 verification.sqliteValid && verification.journalRetrievalReady && verification.journalProvenanceIntegrity.Ready,
		JournalWatermark:              verification.journalWatermark,
	}, nil
}

// VerifyBackup verifies an existing SQLite backup without consulting or mutating live state.
func VerifyBackup(ctx context.Context, backupPath string) (BackupVerificationResult, error) {
	info, err := os.Stat(backupPath)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("stat state backup: %w", err)
	}
	if info.IsDir() {
		return BackupVerificationResult{}, fmt.Errorf("state backup path is a directory: %s", backupPath)
	}
	sha256Sum, err := fileSHA256(backupPath)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("checksum state backup: %w", err)
	}

	store, err := OpenStoreReadOnly(backupPath)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("open state backup for verification: %w", err)
	}
	defer store.Close()

	integrityCheck, err := verifySQLiteIntegrity(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup integrity: %w", err)
	}
	foreignKeyCheck, err := verifyNoForeignKeyViolations(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup foreign keys: %w", err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup schema version: %w", err)
	}
	if !acceptableSchemaVersion(version) {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup schema version: got %d, want %d", version, CurrentSchemaVersion())
	}
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup projects: %w", err)
	}
	if len(projects.Projects) == 0 {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup project count: empty projects table")
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup journal search parity: %w", err)
	}
	provenance, err := InspectJournalProvenanceIntegrity(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup journal provenance: %w", err)
	}
	watermark, err := readJournalWatermark(ctx, store)
	if err != nil {
		return BackupVerificationResult{}, fmt.Errorf("verify state backup journal watermark: %w", err)
	}

	return BackupVerificationResult{
		ContractVersion:            StateJSONContractVersion,
		DatabaseScope:              "global",
		BackupPath:                 backupPath,
		Bytes:                      info.Size(),
		SHA256:                     sha256Sum,
		Verified:                   true,
		SchemaVersion:              version,
		ProjectCount:               len(projects.Projects),
		Projects:                   projects.Projects,
		IntegrityCheck:             integrityCheck,
		ForeignKeyCheck:            foreignKeyCheck,
		JournalRetrievalReady:      parity.Ready,
		JournalSearchParity:        parity,
		JournalProvenanceIntegrity: provenance,
		SQLiteValid:                true,
		RecoveryReady:              parity.Ready && provenance.Ready,
		JournalWatermark:           watermark,
	}, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func prepareBackupDestination(requested string, explicit bool, projectRoot string, volatileRoots []string) (string, error) {
	if volatileRoots == nil {
		volatileRoots = defaultVolatileRoots()
	}
	if explicit && !filepath.IsAbs(requested) {
		return "", &BackupDestinationError{Code: BackupDestinationNotAbsoluteCode, Requested: requested, Resolved: ""}
	}
	clean := filepath.Clean(requested)
	resolved, err := resolveDestinationAfterNearestAncestor(clean)
	if err != nil {
		return "", destinationErrorForPath(err, requested, resolved)
	}
	if explicit && pathWithinVolatileRoots(resolved, volatileRoots) {
		return "", &BackupDestinationError{Code: BackupDestinationVolatileCode, Requested: requested, Resolved: resolved}
	}
	if isWithinRoot(resolved, projectRoot) {
		return "", fmt.Errorf("backup directory must be outside project root")
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return "", fmt.Errorf("create state backup directory: %w", err)
	}
	resolved, err = filepath.EvalSymlinks(clean)
	if err != nil {
		return "", destinationErrorForPath(err, requested, resolved)
	}
	resolved, err = filepath.Abs(filepath.Clean(resolved))
	if err != nil {
		return "", destinationErrorForPath(err, requested, resolved)
	}
	if explicit && pathWithinVolatileRoots(resolved, volatileRoots) {
		return "", &BackupDestinationError{Code: BackupDestinationVolatileCode, Requested: requested, Resolved: resolved}
	}
	if isWithinRoot(resolved, projectRoot) {
		return "", fmt.Errorf("backup directory must be outside project root")
	}
	return resolved, nil
}

func resolvedProjectRoot(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve project root: %w", err)
	}
	return filepath.Abs(filepath.Clean(resolved))
}

func resolveDestinationAfterNearestAncestor(path string) (string, error) {
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return path, &BackupDestinationError{Code: BackupDestinationNotDirectoryCode, Resolved: path}
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		return resolved, resolveErr
	} else if !os.IsNotExist(err) {
		return path, err
	} else if info, lstatErr := os.Lstat(path); lstatErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return path, &BackupDestinationError{Code: BackupDestinationNotDirectoryCode, Resolved: path}
	}

	ancestor := path
	var tail []string
	for {
		info, err := os.Lstat(ancestor)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				resolved, resolveErr := filepath.EvalSymlinks(ancestor)
				if resolveErr != nil {
					return ancestor, &BackupDestinationError{Code: BackupDestinationNotDirectoryCode, Resolved: ancestor}
				}
				info, resolveErr = os.Stat(resolved)
				if resolveErr != nil || !info.IsDir() {
					return ancestor, &BackupDestinationError{Code: BackupDestinationNotDirectoryCode, Resolved: resolved}
				}
				return filepath.Join(append([]string{resolved}, tail...)...), nil
			}
			if !info.IsDir() {
				return ancestor, &BackupDestinationError{Code: BackupDestinationNotDirectoryCode, Resolved: ancestor}
			}
			resolved, resolveErr := filepath.EvalSymlinks(ancestor)
			if resolveErr != nil {
				return ancestor, resolveErr
			}
			return filepath.Join(append([]string{resolved}, tail...)...), nil
		}
		if !os.IsNotExist(err) {
			return ancestor, err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return ancestor, err
		}
		tail = append([]string{filepath.Base(ancestor)}, tail...)
		ancestor = parent
	}
}

func destinationErrorForPath(err error, requested, resolved string) error {
	var destinationErr *BackupDestinationError
	if errors.As(err, &destinationErr) {
		if destinationErr.Requested == "" {
			destinationErr.Requested = requested
		}
		if destinationErr.Resolved == "" {
			destinationErr.Resolved = resolved
		}
		return destinationErr
	}
	return err
}

func pathWithinVolatileRoot(path string) bool {
	return pathWithinVolatileRoots(path, defaultVolatileRoots())
}

func pathWithinVolatileRoots(path string, roots []string) bool {
	lexicalPath := filepath.Clean(path)
	if runtime.GOOS == "linux" && pathWithinComponents(lexicalPath, "/var/run") {
		return true
	}
	path = resolvePathForContainment(path)
	for _, root := range roots {
		if pathWithinComponents(lexicalPath, filepath.Clean(root)) {
			return true
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		resolvedRoot, err = filepath.Abs(filepath.Clean(resolvedRoot))
		if err != nil {
			continue
		}
		if pathWithinComponents(path, resolvedRoot) {
			return true
		}
	}
	return false
}

func pathWithinComponents(path, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func resolvePathForContainment(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	ancestor := path
	var tail []string
	for {
		if resolved, err := filepath.EvalSymlinks(ancestor); err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return path
		}
		tail = append([]string{filepath.Base(ancestor)}, tail...)
		ancestor = parent
	}
}

func defaultVolatileRoots() []string {
	roots := []string{os.TempDir(), "/tmp", "/private/tmp", "/var/tmp"}
	switch runtime.GOOS {
	case "darwin":
		roots = append(roots, "/var/folders", "/private/var/folders")
	case "linux":
		roots = append(roots, "/dev/shm", "/run/user", "/run/lock", "/run/shm")
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(runtimeDir) {
			roots = append(roots, runtimeDir)
		}
	}
	return roots
}

type backupVerification struct {
	schemaVersion              int
	projectCount               int
	projectID                  string
	projectName                string
	projectCurrentPath         string
	integrityCheck             string
	foreignKeyCheck            string
	journalRetrievalReady      bool
	journalSearchParity        JournalSearchParity
	journalProvenanceIntegrity JournalProvenanceIntegrity
	sqliteValid                bool
	journalWatermark           JournalWatermark
}

func verifyBackup(ctx context.Context, backupPath string, root project.Root) (backupVerification, error) {
	store, err := OpenStoreReadOnly(backupPath)
	if err != nil {
		return backupVerification{}, fmt.Errorf("open state backup for verification: %w", err)
	}
	defer store.Close()

	integrityCheck, err := verifySQLiteIntegrity(ctx, store)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup integrity: %w", err)
	}
	foreignKeyCheck, err := verifyNoForeignKeyViolations(ctx, store)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup foreign keys: %w", err)
	}
	version, err := store.SchemaVersion(ctx)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup schema version: %w", err)
	}
	if !acceptableSchemaVersion(version) {
		return backupVerification{}, fmt.Errorf("verify state backup schema version: got %d, want %d", version, CurrentSchemaVersion())
	}
	var projectCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&projectCount); err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup project count: %w", err)
	}
	if projectCount <= 0 {
		return backupVerification{}, fmt.Errorf("verify state backup project count: empty projects table")
	}
	identity, err := store.LookupProjectIdentityForRoot(ctx, root)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup project identity: %w", err)
	}
	if identity.ID == "" {
		return backupVerification{}, fmt.Errorf("verify state backup project identity: empty project id")
	}
	parity, err := InspectJournalSearchParity(ctx, store)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup journal search parity: %w", err)
	}
	provenance, err := InspectJournalProvenanceIntegrity(ctx, store)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup journal provenance: %w", err)
	}
	watermark, err := readJournalWatermark(ctx, store)
	if err != nil {
		return backupVerification{}, fmt.Errorf("verify state backup journal watermark: %w", err)
	}
	return backupVerification{
		schemaVersion:              version,
		projectCount:               projectCount,
		projectID:                  identity.ID,
		projectName:                identity.FriendlyName,
		projectCurrentPath:         identity.CurrentPath,
		integrityCheck:             integrityCheck,
		foreignKeyCheck:            foreignKeyCheck,
		journalRetrievalReady:      parity.Ready,
		journalSearchParity:        parity,
		journalProvenanceIntegrity: provenance,
		sqliteValid:                true,
		journalWatermark:           watermark,
	}, nil
}

func readJournalWatermark(ctx context.Context, store *Store) (JournalWatermark, error) {
	var watermark JournalWatermark
	err := store.db.QueryRowContext(ctx, `
SELECT id, created_at
FROM journal_entries
ORDER BY created_at DESC, id DESC
LIMIT 1`).Scan(&watermark.JournalEntryID, &watermark.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return JournalWatermark{}, nil
	}
	if err != nil {
		return JournalWatermark{}, err
	}
	watermark.Present = true
	return watermark, nil
}

func verifySQLiteIntegrity(ctx context.Context, store *Store) (string, error) {
	var integrityCheck string
	if err := store.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrityCheck); err != nil {
		return "", err
	}
	if integrityCheck != "ok" {
		return "", fmt.Errorf("%s", integrityCheck)
	}
	return integrityCheck, nil
}

func verifyNoForeignKeyViolations(ctx context.Context, store *Store) (string, error) {
	rows, err := store.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var tableName, parentTable string
		var rowID sql.NullInt64
		var foreignKeyID int
		if err := rows.Scan(&tableName, &rowID, &parentTable, &foreignKeyID); err != nil {
			return "", err
		}
		return "", errors.New(formatSQLiteForeignKeyViolation(tableName, rowID, parentTable, foreignKeyID))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "ok", nil
}

func formatSQLiteForeignKeyViolation(tableName string, rowID sql.NullInt64, parentTable string, foreignKeyID int) string {
	rowLabel := "unknown row"
	if rowID.Valid {
		rowLabel = fmt.Sprintf("rowid %d", rowID.Int64)
	}
	return fmt.Sprintf("SQLite foreign key violation in %s %s referencing %s constraint %d", tableName, rowLabel, parentTable, foreignKeyID)
}

func nextBackupPath(backupDir string, now time.Time) (string, error) {
	stamp := fmt.Sprintf("%s-%09d", now.Format("20060102-150405"), now.Nanosecond())
	for i := 0; i < 1000; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%03d", i)
		}
		path := filepath.Join(backupDir, fmt.Sprintf("loaf-%s%s.sqlite", stamp, suffix))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, nil
		} else if err != nil {
			return "", fmt.Errorf("check state backup path: %w", err)
		}
	}
	return "", fmt.Errorf("allocate state backup path: too many backups for timestamp %s", stamp)
}

func reserveBackupPath(backupDir string, now time.Time) (string, error) {
	stamp := fmt.Sprintf("%s-%09d", now.Format("20060102-150405"), now.Nanosecond())
	for i := 0; i < 1000; i++ {
		suffix := ""
		if i > 0 {
			suffix = fmt.Sprintf("-%03d", i)
		}
		path := filepath.Join(backupDir, fmt.Sprintf("loaf-%s%s.sqlite", stamp, suffix))
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if closeErr := file.Close(); closeErr != nil {
				_ = os.Remove(path)
				return "", fmt.Errorf("close reserved state backup path: %w", closeErr)
			}
			return path, nil
		}
		if os.IsExist(err) {
			continue
		}
		return "", fmt.Errorf("reserve state backup path: %w", err)
	}
	return "", fmt.Errorf("allocate state backup path: too many backups for timestamp %s", stamp)
}
