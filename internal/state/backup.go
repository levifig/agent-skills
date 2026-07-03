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
	"time"

	"github.com/levifig/loaf/internal/project"
)

// BackupResult describes a repository-external SQLite database backup.
type BackupResult struct {
	ContractVersion    int    `json:"contract_version"`
	DatabaseScope      string `json:"database_scope"`
	DatabasePath       string `json:"database_path"`
	BackupPath         string `json:"backup_path"`
	Bytes              int64  `json:"bytes"`
	SHA256             string `json:"sha256"`
	CreatedAt          string `json:"created_at"`
	Verified           bool   `json:"verified"`
	SchemaVersion      int    `json:"schema_version"`
	ProjectCount       int    `json:"project_count"`
	ProjectID          string `json:"project_id"`
	ProjectName        string `json:"project_name"`
	ProjectCurrentPath string `json:"project_current_path"`
	IntegrityCheck     string `json:"integrity_check"`
	ForeignKeyCheck    string `json:"foreign_key_check"`
}

// BackupVerificationResult describes a read-only verification of an existing SQLite backup.
type BackupVerificationResult struct {
	ContractVersion           int               `json:"contract_version"`
	DatabaseScope             string            `json:"database_scope"`
	BackupPath                string            `json:"backup_path"`
	Bytes                     int64             `json:"bytes"`
	SHA256                    string            `json:"sha256"`
	Verified                  bool              `json:"verified"`
	SchemaVersion             int               `json:"schema_version"`
	ProjectCount              int               `json:"project_count"`
	Projects                  []ProjectIdentity `json:"projects"`
	IntegrityCheck            string            `json:"integrity_check"`
	ForeignKeyCheck           string            `json:"foreign_key_check"`
	RestoreDatabasePath       string            `json:"restore_database_path,omitempty"`
	RestorePreservePath       string            `json:"restore_preserve_path,omitempty"`
	RestoreValidationCommands []string          `json:"restore_validation_commands,omitempty"`
}

// Backup creates a timestamped SQLite backup under the project's state directory.
func Backup(ctx context.Context, root project.Root, resolver PathResolver) (BackupResult, error) {
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

	now := time.Now().UTC()
	backupDir := filepath.Join(filepath.Dir(status.DatabasePath), "backups")
	if isWithinRoot(backupDir, root.Path()) {
		return BackupResult{}, fmt.Errorf("backup directory must be outside project root")
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return BackupResult{}, fmt.Errorf("create state backup directory: %w", err)
	}

	backupPath, err := nextBackupPath(backupDir, now)
	if err != nil {
		return BackupResult{}, err
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("open state database for backup: %w", err)
	}
	defer store.Close()

	if _, err := store.db.ExecContext(ctx, `VACUUM INTO ?`, backupPath); err != nil {
		return BackupResult{}, fmt.Errorf("backup state database: %w", err)
	}
	if err := os.Chmod(backupPath, 0o600); err != nil {
		return BackupResult{}, fmt.Errorf("set state backup permissions: %w", err)
	}
	info, err := os.Stat(backupPath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("stat state backup: %w", err)
	}
	sha256Sum, err := fileSHA256(backupPath)
	if err != nil {
		return BackupResult{}, fmt.Errorf("checksum state backup: %w", err)
	}
	verification, err := verifyBackup(ctx, backupPath, root)
	if err != nil {
		return BackupResult{}, err
	}

	return BackupResult{
		ContractVersion:    StateJSONContractVersion,
		DatabaseScope:      "global",
		DatabasePath:       status.DatabasePath,
		BackupPath:         backupPath,
		Bytes:              info.Size(),
		SHA256:             sha256Sum,
		CreatedAt:          now.Format(time.RFC3339Nano),
		Verified:           true,
		SchemaVersion:      verification.schemaVersion,
		ProjectCount:       verification.projectCount,
		ProjectID:          verification.projectID,
		ProjectName:        verification.projectName,
		ProjectCurrentPath: verification.projectCurrentPath,
		IntegrityCheck:     verification.integrityCheck,
		ForeignKeyCheck:    verification.foreignKeyCheck,
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

	return BackupVerificationResult{
		ContractVersion: StateJSONContractVersion,
		DatabaseScope:   "global",
		BackupPath:      backupPath,
		Bytes:           info.Size(),
		SHA256:          sha256Sum,
		Verified:        true,
		SchemaVersion:   version,
		ProjectCount:    len(projects.Projects),
		Projects:        projects.Projects,
		IntegrityCheck:  integrityCheck,
		ForeignKeyCheck: foreignKeyCheck,
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

type backupVerification struct {
	schemaVersion      int
	projectCount       int
	projectID          string
	projectName        string
	projectCurrentPath string
	integrityCheck     string
	foreignKeyCheck    string
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
	return backupVerification{
		schemaVersion:      version,
		projectCount:       projectCount,
		projectID:          identity.ID,
		projectName:        identity.FriendlyName,
		projectCurrentPath: identity.CurrentPath,
		integrityCheck:     integrityCheck,
		foreignKeyCheck:    foreignKeyCheck,
	}, nil
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
