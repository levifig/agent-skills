package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// MarkdownSimulationSnapshotFailedCode identifies snapshot-creation failures
	// distinct from apply-pipeline errors (schema preflight, import, FTS).
	MarkdownSimulationSnapshotFailedCode = "markdown-simulation-snapshot-failed"
)

// MarkdownSimulationSnapshotError reports failure creating or verifying the
// disposable database used by SimulateMarkdownMigration.
type MarkdownSimulationSnapshotError struct {
	Code string `json:"code"`
	Err  error  `json:"-"`
}

func (e *MarkdownSimulationSnapshotError) Error() string {
	if e == nil {
		return "markdown simulation snapshot failed"
	}
	if e.Err == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Err)
}

func (e *MarkdownSimulationSnapshotError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func markdownSimulationSnapshotError(err error) error {
	return &MarkdownSimulationSnapshotError{
		Code: MarkdownSimulationSnapshotFailedCode,
		Err:  err,
	}
}

// markdownSimulationSnapshotOps contains per-invocation seams used by
// deterministic tests. Public callers pass nil.
type markdownSimulationSnapshotOps struct {
	// afterVacuum runs after VACUUM INTO succeeds and before structural verify.
	afterVacuum func(snapshotPath string) error
}

// createMarkdownSimulationSnapshot VACUUM INTOs a loaf-owned temp file.
// Cleanup is registered before creation so partial/ENOSPC/cancel leaves no
// orphan. Hard-fails on integrity_check / foreign_key_check; journal-search
// readiness is not gated (the apply pipeline rebuilds FTS).
func createMarkdownSimulationSnapshot(ctx context.Context, livePath string) (snapshotPath string, cleanup func(), err error) {
	return createMarkdownSimulationSnapshotWithOps(ctx, livePath, nil)
}

func createMarkdownSimulationSnapshotWithOps(ctx context.Context, livePath string, ops *markdownSimulationSnapshotOps) (snapshotPath string, cleanup func(), err error) {
	tempDir, err := os.MkdirTemp("", "loaf-markdown-simulate-*")
	if err != nil {
		return "", nil, markdownSimulationSnapshotError(fmt.Errorf("create snapshot temp dir: %w", err))
	}
	if err := os.Chmod(tempDir, 0o700); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, markdownSimulationSnapshotError(fmt.Errorf("set snapshot temp dir permissions: %w", err))
	}

	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	snapshotPath = filepath.Join(tempDir, fmt.Sprintf("markdown-simulate-%d-%s.sqlite", os.Getpid(), stamp))

	var cleaned bool
	cleanup = func() {
		if cleaned {
			return
		}
		cleaned = true
		removeSQLiteFileTree(snapshotPath)
		_ = os.RemoveAll(tempDir)
	}

	// Register cleanup before VACUUM INTO so creation failure leaves no residue.
	if err := vacuumSQLiteInto(ctx, livePath, snapshotPath); err != nil {
		cleanup()
		return "", func() {}, markdownSimulationSnapshotError(err)
	}
	if ops != nil && ops.afterVacuum != nil {
		if err := ops.afterVacuum(snapshotPath); err != nil {
			cleanup()
			return "", func() {}, markdownSimulationSnapshotError(err)
		}
	}
	if err := verifySQLiteSnapshotStructural(ctx, snapshotPath); err != nil {
		cleanup()
		return "", func() {}, markdownSimulationSnapshotError(err)
	}
	return snapshotPath, cleanup, nil
}

// vacuumSQLiteInto copies livePath to destination via VACUUM INTO.
// Destination must not exist. Sets 0600 permissions on success. Caller owns
// cleanup of destination and -wal/-shm siblings; on failure this helper
// removes a partial destination file when one was created.
func vacuumSQLiteInto(ctx context.Context, livePath, destination string) error {
	if !filepath.IsAbs(destination) {
		return fmt.Errorf("snapshot destination must be an absolute path")
	}
	info, err := os.Stat(destination)
	switch {
	case err == nil:
		// Backup reserves an exclusive empty path before VACUUM INTO; allow
		// that pattern by removing the zero-byte placeholder. Any other
		// existing file is a hard error.
		if !info.Mode().IsRegular() || info.Size() != 0 {
			return fmt.Errorf("snapshot destination already exists: %s", destination)
		}
		if err := os.Remove(destination); err != nil {
			return fmt.Errorf("clear reserved snapshot destination: %w", err)
		}
	case os.IsNotExist(err):
		// Caller-owned destination that does not exist yet.
	default:
		return fmt.Errorf("stat snapshot destination: %w", err)
	}

	partial := false
	defer func() {
		if partial {
			_ = os.Remove(destination)
		}
	}()

	store, err := openStoreReadOnlyForBackup(livePath)
	if err != nil {
		return fmt.Errorf("open live database for snapshot: %w", err)
	}
	defer store.Close()

	partial = true
	if _, err := store.db.ExecContext(ctx, `VACUUM INTO ?`, destination); err != nil {
		return fmt.Errorf("VACUUM INTO snapshot: %w", err)
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return fmt.Errorf("set snapshot permissions: %w", err)
	}
	partial = false
	return nil
}

// verifySQLiteSnapshotStructural hard-fails on integrity_check or
// foreign_key_check failures. Journal-search / provenance readiness are
// intentionally not gated here.
func verifySQLiteSnapshotStructural(ctx context.Context, snapshotPath string) error {
	store, err := OpenStoreReadOnly(snapshotPath)
	if err != nil {
		return fmt.Errorf("open snapshot for verification: %w", err)
	}
	defer store.Close()

	if _, err := verifySQLiteIntegrity(ctx, store); err != nil {
		return fmt.Errorf("snapshot integrity_check failed: %w", err)
	}
	if _, err := verifyNoForeignKeyViolations(ctx, store); err != nil {
		return fmt.Errorf("snapshot foreign_key_check failed: %w", err)
	}
	return nil
}

// removeSQLiteFileTree removes a SQLite main database and its -wal/-shm siblings.
func removeSQLiteFileTree(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	_ = os.Remove(path)
}
