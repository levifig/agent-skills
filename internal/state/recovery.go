package state

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/levifig/loaf/internal/project"
)

const (
	BackupNotRecoveryReadyCode             = "backup-not-recovery-ready"
	RestoreDestinationNotAbsoluteCode      = "restore-destination-not-absolute"
	RestoreDestinationExistsCode           = "restore-destination-exists"
	RestoreDestinationInvalidParentCode    = "restore-destination-invalid-parent"
	RestoreDestinationAliasesBackupCode    = "restore-destination-aliases-backup"
	RestoreDestinationIsLiveDatabaseCode   = "restore-destination-is-live-database"
	RestoreLiveDatabasePathUnavailableCode = "restore-live-database-path-unavailable"
	RestoreDestinationReplacedCode         = "restore-destination-replaced"
	RestoreDestinationSidecarsExistCode    = "restore-destination-sidecars-exist"
	RestoreCopyMismatchCode                = "restore-copy-mismatch"
	RestoreVerificationFailedCode          = "restore-verification-failed"
)

// RecoveryError is a stable, path-bearing error returned by isolated restore
// validation and rehearsal. The source backup is always identified, even when
// validation fails before a restore target is created.
type RecoveryError struct {
	Code        string `json:"code"`
	BackupPath  string `json:"backup_path"`
	RestorePath string `json:"restore_path"`
	Err         error  `json:"-"`
}

func (e *RecoveryError) Error() string {
	if e == nil {
		return "backup recovery failed"
	}
	message := e.Code
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return fmt.Sprintf("%s (backup=%q restore=%q)", message, e.BackupPath, e.RestorePath)
}

func (e *RecoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// BackupRestoreResult is the evidence packet from an isolated backup restore
// rehearsal. It deliberately does not claim live activation or device-loss
// protection.
type BackupRestoreResult struct {
	ContractVersion       int                 `json:"contract_version"`
	DatabaseScope         string              `json:"database_scope"`
	BackupPath            string              `json:"backup_path"`
	RestorePath           string              `json:"restore_path"`
	SourceSHA256          string              `json:"source_sha256"`
	RestoredSHA256        string              `json:"restored_sha256"`
	ExactCopy             bool                `json:"exact_copy"`
	DisposableRehearsal   bool                `json:"disposable_rehearsal"`
	LiveDatabasePath      string              `json:"live_database_path"`
	LiveDatabaseMutated   bool                `json:"live_database_mutated"`
	SQLiteValid           bool                `json:"sqlite_valid"`
	IntegrityCheck        string              `json:"integrity_check"`
	ForeignKeyCheck       string              `json:"foreign_key_check"`
	SchemaVersion         int                 `json:"schema_version"`
	ProjectCount          int                 `json:"project_count"`
	Projects              []ProjectIdentity   `json:"projects"`
	JournalRetrievalReady bool                `json:"journal_retrieval_ready"`
	JournalSearchParity   JournalSearchParity `json:"journal_search_parity"`
	WatermarkPresent      bool                `json:"watermark_present"`
	JournalWatermark      JournalWatermark    `json:"journal_watermark"`
	RecoveryReady         bool                `json:"recovery_ready"`
}

type recoveryOperations struct {
	beforeCreate          func(finalPath string) error
	afterCreate           func(finalPath string, target *os.File) error
	afterCopy             func(finalPath string) error
	beforeFinalValidation func(finalPath string) error
	copy                  func(context.Context, string, *os.File) error
	hash                  func(string) (string, error)
	verify                func(context.Context, string) (BackupVerificationResult, error)
}

// RehearseBackupRestore copies a verified backup into an empty, isolated
// database path and verifies the copy without opening or mutating live state.
func RehearseBackupRestore(ctx context.Context, root project.Root, resolver PathResolver, backupPath, restorePath string) (BackupRestoreResult, error) {
	return rehearseBackupRestore(ctx, root, resolver, backupPath, restorePath, nil)
}

func rehearseBackupRestore(ctx context.Context, root project.Root, resolver PathResolver, backupPath, restorePath string, ops *recoveryOperations) (BackupRestoreResult, error) {
	if ops == nil {
		ops = &recoveryOperations{}
	}
	result := BackupRestoreResult{
		ContractVersion:     StateJSONContractVersion,
		DatabaseScope:       "global",
		BackupPath:          backupPath,
		RestorePath:         restorePath,
		DisposableRehearsal: true,
		LiveDatabaseMutated: false,
	}
	hashSource := ops.hash
	if hashSource == nil {
		hashSource = fileSHA256
	}
	if sourceSHA, hashErr := hashSource(backupPath); hashErr == nil {
		result.SourceSHA256 = sourceSHA
	}
	source, err := VerifyBackup(ctx, backupPath)
	if err != nil {
		return result, recoveryError(BackupNotRecoveryReadyCode, backupPath, restorePath, err)
	}
	result.SourceSHA256 = source.SHA256
	result.SQLiteValid = source.SQLiteValid
	result.IntegrityCheck = source.IntegrityCheck
	result.ForeignKeyCheck = source.ForeignKeyCheck
	result.SchemaVersion = source.SchemaVersion
	result.ProjectCount = source.ProjectCount
	result.Projects = source.Projects
	result.JournalRetrievalReady = source.JournalRetrievalReady
	result.JournalSearchParity = source.JournalSearchParity
	result.WatermarkPresent = source.JournalWatermark.Present
	result.JournalWatermark = source.JournalWatermark
	if !source.Verified || !source.SQLiteValid || !source.RecoveryReady || !source.JournalRetrievalReady {
		return result, recoveryError(BackupNotRecoveryReadyCode, backupPath, restorePath, fmt.Errorf("backup is not structurally and retrieval ready"))
	}
	livePath, err := resolver.DatabasePath(root)
	if err != nil {
		return result, recoveryError(RestoreLiveDatabasePathUnavailableCode, backupPath, restorePath, err)
	}
	result.LiveDatabasePath = livePath
	resolvedRestore, err := validateRestoreDestination(backupPath, livePath, restorePath)
	if err != nil {
		return result, err
	}
	if err := validateRestoreSidecars(resolvedRestore, backupPath, restorePath); err != nil {
		return result, err
	}
	if ops.beforeCreate != nil {
		if err := ops.beforeCreate(resolvedRestore); err != nil {
			return result, recoveryError(RestoreDestinationExistsCode, backupPath, restorePath, err)
		}
	}
	target, err := os.OpenFile(resolvedRestore, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return result, recoveryError(RestoreDestinationExistsCode, backupPath, restorePath, err)
		}
		return result, recoveryError(RestoreDestinationInvalidParentCode, backupPath, restorePath, err)
	}
	targetInfo, err := target.Stat()
	if err != nil {
		_ = target.Close()
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	created := true
	defer func() {
		if target != nil {
			_ = target.Close()
		}
		if created {
			cleanupOwnedPath(resolvedRestore, targetInfo)
		}
	}()
	if ops.afterCreate != nil {
		if err := ops.afterCreate(resolvedRestore, target); err != nil {
			return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
		}
	}
	copyBytes := ops.copy
	if copyBytes == nil {
		copyBytes = copyRestoreBytes
	}
	if err := copyBytes(ctx, backupPath, target); err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	if err := target.Chmod(0o600); err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	if err := target.Sync(); err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	if err := target.Close(); err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	target = nil
	if ops.afterCopy != nil {
		if err := ops.afterCopy(resolvedRestore); err != nil {
			return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
		}
	}
	if err := requireOwnedRestorePath(resolvedRestore, targetInfo, backupPath, restorePath); err != nil {
		return result, err
	}
	if err := validateRestoreSidecars(resolvedRestore, backupPath, restorePath); err != nil {
		return result, err
	}
	hashFile := ops.hash
	if hashFile == nil {
		hashFile = fileSHA256
	}
	restoredSHA, err := hashFile(resolvedRestore)
	if err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	result.RestoredSHA256 = restoredSHA
	sourceSHA, err := hashFile(backupPath)
	if err != nil || sourceSHA != source.SHA256 || restoredSHA != source.SHA256 || sourceSHA != restoredSHA {
		if err == nil {
			err = fmt.Errorf("source or restored checksum differs from verified source")
		}
		return result, recoveryError(RestoreCopyMismatchCode, backupPath, restorePath, err)
	}
	result.ExactCopy = true
	if err := requireOwnedRestorePath(resolvedRestore, targetInfo, backupPath, restorePath); err != nil {
		return result, err
	}
	verify := ops.verify
	if verify == nil {
		verify = VerifyBackup
	}
	restored, err := verify(ctx, resolvedRestore)
	if err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	result.SQLiteValid = restored.SQLiteValid
	result.IntegrityCheck = restored.IntegrityCheck
	result.ForeignKeyCheck = restored.ForeignKeyCheck
	result.SchemaVersion = restored.SchemaVersion
	result.ProjectCount = restored.ProjectCount
	result.Projects = restored.Projects
	result.JournalRetrievalReady = restored.JournalRetrievalReady
	result.JournalSearchParity = restored.JournalSearchParity
	result.WatermarkPresent = restored.JournalWatermark.Present
	result.JournalWatermark = restored.JournalWatermark
	if err := compareRestoreEvidence(source, restored); err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	if err := compareRestoreRows(ctx, backupPath, resolvedRestore); err != nil {
		return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
	}
	if err := requireOwnedRestorePath(resolvedRestore, targetInfo, backupPath, restorePath); err != nil {
		return result, err
	}
	if err := validateRestoreSidecars(resolvedRestore, backupPath, restorePath); err != nil {
		return result, err
	}
	if ops.beforeFinalValidation != nil {
		if err := ops.beforeFinalValidation(resolvedRestore); err != nil {
			return result, recoveryError(RestoreVerificationFailedCode, backupPath, restorePath, err)
		}
	}
	if err := requireOwnedRestorePath(resolvedRestore, targetInfo, backupPath, restorePath); err != nil {
		return result, err
	}
	finalSHA, err := hashOwnedRestorePath(resolvedRestore, targetInfo)
	if err != nil {
		return result, recoveryError(RestoreDestinationReplacedCode, backupPath, restorePath, err)
	}
	if finalSHA != source.SHA256 {
		return result, recoveryError(RestoreCopyMismatchCode, backupPath, restorePath, fmt.Errorf("published restore checksum differs from verified source"))
	}
	result.RecoveryReady = restored.RecoveryReady
	created = false
	return result, nil
}

func recoveryError(code, backupPath, restorePath string, err error) error {
	return &RecoveryError{Code: code, BackupPath: backupPath, RestorePath: restorePath, Err: err}
}

func validateRestoreDestination(backupPath, livePath, restorePath string) (string, error) {
	if !filepath.IsAbs(restorePath) {
		return "", recoveryError(RestoreDestinationNotAbsoluteCode, backupPath, restorePath, fmt.Errorf("restore path must be absolute"))
	}
	clean := filepath.Clean(restorePath)
	parent := filepath.Dir(clean)
	parentInfo, err := os.Stat(parent)
	if err != nil || !parentInfo.IsDir() {
		return "", recoveryError(RestoreDestinationInvalidParentCode, backupPath, restorePath, fmt.Errorf("restore parent must be an existing directory"))
	}
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", recoveryError(RestoreDestinationInvalidParentCode, backupPath, restorePath, err)
	}
	resolved := filepath.Join(resolvedParent, filepath.Base(clean))
	resolvedBackup, err := filepath.EvalSymlinks(backupPath)
	if err != nil {
		return "", recoveryError(RestoreDestinationAliasesBackupCode, backupPath, restorePath, err)
	}
	if samePath(resolved, resolvedBackup) {
		return "", recoveryError(RestoreDestinationAliasesBackupCode, backupPath, restorePath, fmt.Errorf("restore target aliases backup"))
	}
	resolvedLive := resolvePathForContainment(livePath)
	if samePath(resolved, resolvedLive) {
		return "", recoveryError(RestoreDestinationIsLiveDatabaseCode, backupPath, restorePath, fmt.Errorf("restore target aliases live database"))
	}
	if _, err := os.Lstat(clean); err == nil {
		return "", recoveryError(RestoreDestinationExistsCode, backupPath, restorePath, fmt.Errorf("restore target already exists"))
	} else if !os.IsNotExist(err) {
		return "", recoveryError(RestoreDestinationExistsCode, backupPath, restorePath, err)
	}
	return resolved, nil
}

func samePath(left, right string) bool {
	left, _ = filepath.Abs(filepath.Clean(left))
	right, _ = filepath.Abs(filepath.Clean(right))
	return left == right
}

func copyRestoreBytes(ctx context.Context, sourcePath string, target *os.File) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	_, err = io.Copy(target, source)
	return err
}

func cleanupOwnedPath(path string, ownedInfo os.FileInfo) {
	info, err := os.Lstat(path)
	if err != nil || !os.SameFile(ownedInfo, info) {
		return
	}
	_ = os.Remove(path)
}

func hashOwnedRestorePath(path string, ownedInfo os.FileInfo) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return "", err
	}
	if !os.SameFile(ownedInfo, openedInfo) {
		_ = file.Close()
		return "", fmt.Errorf("published restore inode changed before hashing")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		_ = file.Close()
		return "", err
	}
	finalInfo, err := file.Stat()
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !os.SameFile(ownedInfo, finalInfo) || !os.SameFile(ownedInfo, pathInfo) {
		return "", fmt.Errorf("published restore inode changed during hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateRestoreSidecars(path, backupPath, restorePath string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		sidecar := path + suffix
		if _, err := os.Lstat(sidecar); err == nil {
			return recoveryError(RestoreDestinationSidecarsExistCode, backupPath, restorePath, fmt.Errorf("restore sidecar exists: %s", sidecar))
		} else if !os.IsNotExist(err) {
			return recoveryError(RestoreDestinationSidecarsExistCode, backupPath, restorePath, err)
		}
	}
	return nil
}

func requireOwnedRestorePath(path string, ownedInfo os.FileInfo, backupPath, restorePath string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return recoveryError(RestoreDestinationReplacedCode, backupPath, restorePath, err)
	}
	if !os.SameFile(ownedInfo, info) {
		return recoveryError(RestoreDestinationReplacedCode, backupPath, restorePath, fmt.Errorf("restore target inode changed"))
	}
	return nil
}

func compareRestoreEvidence(source, restored BackupVerificationResult) error {
	if !restored.Verified || !restored.SQLiteValid || !restored.RecoveryReady || !restored.JournalRetrievalReady {
		return fmt.Errorf("restored database is not recovery-ready")
	}
	sourceProjects := append([]ProjectIdentity(nil), source.Projects...)
	restoredProjects := append([]ProjectIdentity(nil), restored.Projects...)
	for index := range sourceProjects {
		sourceProjects[index].DatabasePath = ""
	}
	for index := range restoredProjects {
		restoredProjects[index].DatabasePath = ""
	}
	if source.SchemaVersion != restored.SchemaVersion || source.ProjectCount != restored.ProjectCount ||
		source.Projects == nil || restored.Projects == nil || !reflect.DeepEqual(sourceProjects, restoredProjects) ||
		source.IntegrityCheck != restored.IntegrityCheck || source.ForeignKeyCheck != restored.ForeignKeyCheck ||
		source.JournalRetrievalReady != restored.JournalRetrievalReady || source.JournalSearchParity != restored.JournalSearchParity ||
		source.JournalWatermark != restored.JournalWatermark {
		return fmt.Errorf("restore evidence differs from source backup")
	}
	return nil
}

func compareRestoreRows(ctx context.Context, backupPath, restorePath string) error {
	source, err := OpenStoreReadOnly(backupPath)
	if err != nil {
		return err
	}
	defer source.Close()
	restored, err := OpenStoreReadOnly(restorePath)
	if err != nil {
		return err
	}
	defer restored.Close()
	sourceCanonical, err := canonicalJournalRows(ctx, source)
	if err != nil {
		return err
	}
	restoredCanonical, err := canonicalJournalRows(ctx, restored)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(sourceCanonical, restoredCanonical) {
		return fmt.Errorf("canonical journal rows differ")
	}
	sourceSearch, err := journalSearchRows(ctx, source)
	if err != nil {
		return err
	}
	restoredSearch, err := journalSearchRows(ctx, restored)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(sourceSearch, restoredSearch) {
		return fmt.Errorf("journal search rows differ")
	}
	for _, table := range []struct {
		name    string
		orderBy string
	}{
		{name: "journal_origins", orderBy: "journal_entry_id"},
		{name: "journal_deferrals", orderBy: "project_id, operation_key"},
	} {
		sourceRows, err := journalProvenanceRows(ctx, source, table.name, table.orderBy)
		if err != nil {
			return err
		}
		restoredRows, err := journalProvenanceRows(ctx, restored, table.name, table.orderBy)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(sourceRows, restoredRows) {
			return fmt.Errorf("%s rows differ", table.name)
		}
	}
	return nil
}

func journalProvenanceRows(ctx context.Context, store *Store, table string, orderBy string) ([]map[string]any, error) {
	exists, err := provenanceTableExists(ctx, store, table)
	if err != nil {
		return nil, err
	}
	if !exists {
		return []map[string]any{}, nil
	}
	rows, err := store.db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s ORDER BY %s", quoteSQLiteIdentifier(table), orderBy))
	if err != nil {
		return nil, fmt.Errorf("read %s rows: %w", table, err)
	}
	defer rows.Close()
	result, err := scanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("scan %s rows: %w", table, err)
	}
	return result, nil
}

func canonicalJournalRows(ctx context.Context, store *Store) ([]string, error) {
	correlation, err := journalSearchCorrelationColumn(ctx, store)
	if err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, fmt.Sprintf(`
SELECT rowid, project_id, id, entry_type, COALESCE(scope, ''), message,
       COALESCE(%s, ''), created_at, updated_at
FROM journal_entries ORDER BY rowid`, correlation))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var rowid int64
		var projectID, id, entryType, scope, message, corr, createdAt, updatedAt string
		if err := rows.Scan(&rowid, &projectID, &id, &entryType, &scope, &message, &corr, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", rowid, projectID, id, entryType, scope, message, corr, createdAt, updatedAt))
	}
	return result, rows.Err()
}

func journalSearchRows(ctx context.Context, store *Store) ([]string, error) {
	correlation, err := journalSearchCorrelationColumn(ctx, store)
	if err != nil {
		return nil, err
	}
	rows, err := store.db.QueryContext(ctx, fmt.Sprintf(`
SELECT rowid, project_id, journal_entry_id, COALESCE(%s, ''), entry_type, scope, message
FROM journal_search ORDER BY rowid`, correlation))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var rowid int64
		var projectID, journalEntryID, corr, entryType, scope, message string
		if err := rows.Scan(&rowid, &projectID, &journalEntryID, &corr, &entryType, &scope, &message); err != nil {
			return nil, err
		}
		result = append(result, fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", rowid, projectID, journalEntryID, corr, entryType, scope, message))
	}
	return result, rows.Err()
}
