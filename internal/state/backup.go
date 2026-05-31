package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/levifig/loaf/internal/project"
)

// BackupResult describes a repository-external SQLite database backup.
type BackupResult struct {
	DatabasePath string `json:"database_path"`
	BackupPath   string `json:"backup_path"`
	Bytes        int64  `json:"bytes"`
	CreatedAt    string `json:"created_at"`
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

	return BackupResult{
		DatabasePath: status.DatabasePath,
		BackupPath:   backupPath,
		Bytes:        info.Size(),
		CreatedAt:    now.Format(time.RFC3339Nano),
	}, nil
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
