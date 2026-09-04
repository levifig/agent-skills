//go:build !windows

package sqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func validateDatabaseDirectoryPath(path string) error {
	if err := validateDirectoryPathComponents(path, true); err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve relay database directory: %w", err)
	}
	if filepath.Clean(resolved) == filepath.Clean(path) {
		return nil
	}
	return validateDirectoryPathComponents(resolved, false)
}

func validateDirectoryPathComponents(path string, allowRootOwnedSystemAlias bool) error {
	root := filepath.VolumeName(path) + string(filepath.Separator)
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect relay database path root: %w", err)
	}
	if err := validateTrustedDirectoryComponent(root, rootInfo); err != nil {
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
			return fmt.Errorf("relay database directory does not exist")
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			owner, ownerErr := fileOwnerUID(info)
			if ownerErr != nil {
				return fmt.Errorf("inspect relay database path component %s ownership: %w", component, ownerErr)
			}
			if !allowRootOwnedSystemAlias || index != 0 || owner != 0 {
				return fmt.Errorf("relay database path component %s is an untrusted symlink", component)
			}
			continue
		}
		if err := validateTrustedDirectoryComponent(current, info); err != nil {
			return err
		}
	}
	return nil
}

func validatePrivateDirectoryMode(info os.FileInfo) error {
	// The transport-neutral relay accepts a deployment-selected absolute file
	// path instead of owning a per-user state root. Directory creation and read
	// visibility therefore remain deployment concerns, while the adapter refuses
	// any directory that another principal can modify and keeps DB files private.
	owner, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("inspect relay database directory ownership: %w", err)
	}
	if owner != uint32(os.Geteuid()) {
		return fmt.Errorf("relay database directory must be owned by the service user")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("relay database directory must not be writable by group or other principals")
	}
	return nil
}

func validatePrivateFileMode(_ string, info os.FileInfo) error {
	owner, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("inspect relay SQLite file ownership: %w", err)
	}
	if owner != uint32(os.Geteuid()) {
		return fmt.Errorf("relay SQLite file must be owned by the service user")
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("relay SQLite file must have mode 0600")
	}
	return nil
}

func securePrivateFileMode(path string, before os.FileInfo) (os.FileInfo, error) {
	if err := validatePrivateFileOwner(before); err != nil {
		return nil, err
	}
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
	if err := validatePrivateFileMode(path, after); err != nil {
		return nil, err
	}
	return after, nil
}

func validateTrustedDirectoryComponent(path string, info os.FileInfo) error {
	if !info.IsDir() {
		return fmt.Errorf("relay database path component %s is not a directory", filepath.Base(path))
	}
	owner, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("inspect relay database path component %s ownership: %w", filepath.Base(path), err)
	}
	serviceUID := uint32(os.Geteuid())
	if owner != 0 && owner != serviceUID {
		return fmt.Errorf("relay database path component %s has an untrusted owner", filepath.Base(path))
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("relay database path component %s is writable by another principal without sticky protection", filepath.Base(path))
	}
	return nil
}

func validatePrivateFileOwner(info os.FileInfo) error {
	owner, err := fileOwnerUID(info)
	if err != nil {
		return fmt.Errorf("inspect relay SQLite file ownership: %w", err)
	}
	if owner != uint32(os.Geteuid()) {
		return fmt.Errorf("relay SQLite file must be owned by the service user")
	}
	return nil
}

func fileOwnerUID(info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, fmt.Errorf("unsupported file ownership metadata")
	}
	return stat.Uid, nil
}
