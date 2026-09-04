//go:build !windows

package sqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateStateRootLocationPlatform(path string) error {
	return validateExistingUnixPathComponents(path)
}

func validateStateRootPlatform(path string, info os.FileInfo) error {
	if err := validateExistingUnixPathComponents(path); err != nil {
		return err
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("state root must not be writable by group or other principals")
	}
	return nil
}

func validatePrivateDirectoryPlatform(path string, info os.FileInfo) error {
	if err := validateExistingUnixPathComponents(path); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("continuity state directory must have mode 0700")
	}
	return nil
}

func validatePrivateFilePlatform(path string, info os.FileInfo) error {
	if err := validateExistingUnixPathComponents(path); err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("continuity SQLite file must have mode 0600")
	}
	return nil
}

func securePrivateFilePlatform(path string, before os.FileInfo) (os.FileInfo, error) {
	if before.Mode().Perm() != 0o600 {
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, err
		}
	}
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, fmt.Errorf("file changed while securing permissions")
	}
	return after, nil
}

func validateExistingUnixPathComponents(path string) error {
	root := filepath.VolumeName(path) + string(filepath.Separator)
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	for index, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 && index > 0 {
			return fmt.Errorf("path component %s is a symlink", component)
		}
	}
	return nil
}

func databaseURLPath(path string) string {
	return filepath.ToSlash(path)
}
