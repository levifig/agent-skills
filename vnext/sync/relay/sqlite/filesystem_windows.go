//go:build windows

package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
)

func validateDatabaseDirectoryPath(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve relay database directory: %w", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(path) {
		return fmt.Errorf("relay database directory cannot resolve through a symlink or reparse point")
	}
	return nil
}

// The relay database path is deployment-configured rather than tied to one
// user's LocalAppData. Windows DACL validation is therefore an explicit
// deployment responsibility; the common path logic still rejects every
// observed symlink/reparse resolution and inode replacement.
func validatePrivateDirectoryMode(_ os.FileInfo) error {
	return nil
}

func validatePrivateFileMode(path string, _ os.FileInfo) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve relay SQLite file: %w", err)
	}
	if filepath.Clean(resolved) != filepath.Clean(path) {
		return fmt.Errorf("relay SQLite file cannot resolve through a symlink or reparse point")
	}
	return nil
}

func securePrivateFileMode(path string, before os.FileInfo) (os.FileInfo, error) {
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, fmt.Errorf("file changed while validating Windows security boundary")
	}
	if err := validatePrivateFileMode(path, after); err != nil {
		return nil, err
	}
	return after, nil
}
