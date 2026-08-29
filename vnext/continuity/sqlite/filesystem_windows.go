//go:build windows

package sqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Windows does not expose DACLs through os.FileMode. The store therefore
// admits state only below the current user's LocalAppData root, whose ACL is
// established by Windows, and rejects every observed reparse point. The
// inherited ACL remains an explicit trust assumption rather than a DACL proof.
func validateStateRootLocationPlatform(path string) error {
	if isWindowsUNC(path) {
		return fmt.Errorf("state root cannot use a Windows UNC path")
	}
	root, err := windowsUserDataRoot()
	if err != nil {
		return err
	}
	if !pathWithinWindowsRoot(path, root) {
		return fmt.Errorf("state root must be below the current user's LocalAppData root")
	}
	if _, err := validateObservedWindowsPath(root); err != nil {
		return fmt.Errorf("validate Windows LocalAppData root: %w", err)
	}
	return validateExistingWindowsPathComponents(root, path)
}

func validateStateRootPlatform(path string, _ os.FileInfo) error {
	root, err := windowsUserDataRoot()
	if err != nil {
		return err
	}
	resolvedRoot, err := validateObservedWindowsPath(root)
	if err != nil {
		return fmt.Errorf("validate Windows LocalAppData root: %w", err)
	}
	resolved, err := validateObservedWindowsPath(path)
	if err != nil {
		return fmt.Errorf("validate Windows state root: %w", err)
	}
	if !pathWithinWindowsRoot(resolved, resolvedRoot) {
		return fmt.Errorf("resolved state root must remain below the current user's LocalAppData root")
	}
	return validateWindowsPathComponents(resolvedRoot, resolved)
}

func validatePrivateDirectoryPlatform(path string, _ os.FileInfo) error {
	if _, err := validateObservedWindowsPath(path); err != nil {
		return fmt.Errorf("validate Windows continuity state directory: %w", err)
	}
	return nil
}

func validatePrivateFilePlatform(path string, _ os.FileInfo) error {
	if _, err := validateObservedWindowsPath(path); err != nil {
		return fmt.Errorf("validate Windows continuity SQLite file: %w", err)
	}
	return nil
}

func securePrivateFilePlatform(path string, before os.FileInfo) (os.FileInfo, error) {
	after, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, fmt.Errorf("file changed while validating Windows security boundary")
	}
	if err := validatePrivateFilePlatform(path, after); err != nil {
		return nil, err
	}
	return after, nil
}

func validateObservedWindowsPath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(path)) {
		return "", fmt.Errorf("path resolves through a symlink or reparse point")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	reparse, err := windowsReparsePoint(info)
	if err != nil {
		return "", err
	}
	if reparse {
		return "", fmt.Errorf("path is a Windows reparse point")
	}
	return resolved, nil
}

func validateWindowsPathComponents(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		reparse, err := windowsReparsePoint(info)
		if err != nil {
			return err
		}
		if reparse {
			return fmt.Errorf("Windows path component %s is a reparse point", component)
		}
	}
	return nil
}

func validateExistingWindowsPathComponents(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
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
		reparse, err := windowsReparsePoint(info)
		if err != nil {
			return err
		}
		if reparse {
			return fmt.Errorf("Windows path component %s is a reparse point", component)
		}
	}
	return nil
}

func windowsUserDataRoot() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve Windows LocalAppData root: %w", err)
	}
	if !filepath.IsAbs(root) || isWindowsUNC(root) {
		return "", fmt.Errorf("Windows LocalAppData root must be an absolute local path")
	}
	return filepath.Clean(root), nil
}

func pathWithinWindowsRoot(path, root string) bool {
	if !strings.EqualFold(filepath.VolumeName(path), filepath.VolumeName(root)) {
		return false
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func isWindowsUNC(path string) bool {
	clean := filepath.Clean(path)
	return strings.HasPrefix(clean, `\\`)
}

func databaseURLPath(path string) string {
	urlPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && !strings.HasPrefix(urlPath, "/") {
		return "/" + urlPath
	}
	return urlPath
}
