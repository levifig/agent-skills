package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

const databaseFileName = "loaf.sqlite"

// PathResolver computes the project-scoped SQLite path without creating it.
type PathResolver struct {
	DataHome  string
	StateHome string
	CacheHome string
}

// DatabasePath returns the intended global SQLite database path.
func (r PathResolver) DatabasePath(root project.Root) (string, error) {
	dataHome, err := r.dataHome()
	if err != nil {
		return "", err
	}
	if isWithinRoot(dataHome, root.Path()) {
		return "", fmt.Errorf("data home must be outside project root")
	}
	return filepath.Join(dataHome, "loaf", databaseFileName), nil
}

// ProjectDatabasePath returns the prior project-sharded XDG_DATA_HOME SQLite
// location used before Loaf converged on one global database file.
func (r PathResolver) ProjectDatabasePath(root project.Root) (string, error) {
	dataHome, err := r.dataHome()
	if err != nil {
		return "", err
	}
	if isWithinRoot(dataHome, root.Path()) {
		return "", fmt.Errorf("data home must be outside project root")
	}
	projectID := ProjectID(root)
	return filepath.Join(dataHome, "loaf", "projects", projectID, databaseFileName), nil
}

// RenderCachePath returns a path under the XDG cache home for disposable render output.
func (r PathResolver) RenderCachePath(root project.Root, parts ...string) (string, error) {
	cacheHome, err := r.cacheHome()
	if err != nil {
		return "", err
	}
	if isWithinRoot(cacheHome, root.Path()) {
		return "", fmt.Errorf("cache home must be outside project root")
	}
	segments := append([]string{cacheHome, "loaf", "renders"}, parts...)
	return filepath.Join(segments...), nil
}

// LegacyDatabasePath returns the old XDG_STATE_HOME SQLite location used before
// durable operational state moved to XDG_DATA_HOME.
func (r PathResolver) LegacyDatabasePath(root project.Root) (string, error) {
	stateHome, err := r.legacyStateHome()
	if err != nil {
		return "", err
	}
	if isWithinRoot(stateHome, root.Path()) {
		return "", fmt.Errorf("legacy state home must be outside project root")
	}
	projectID := ProjectID(root)
	return filepath.Join(stateHome, "loaf", "projects", projectID, databaseFileName), nil
}

func (r PathResolver) dataHome() (string, error) {
	if r.DataHome != "" {
		return cleanAbsoluteHome("data home", r.DataHome)
	}
	if r.StateHome != "" {
		return cleanAbsoluteHome("state home override", r.StateHome)
	}
	if value := os.Getenv("XDG_DATA_HOME"); value != "" {
		if dataHome, ok := cleanAbsoluteXDGHome(value); ok {
			return dataHome, nil
		}
	}
	if runtime.GOOS == "windows" {
		if value := os.Getenv("LOCALAPPDATA"); value != "" {
			return filepath.Join(value, "loaf", "data"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve data home: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

func (r PathResolver) legacyStateHome() (string, error) {
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		if stateHome, ok := cleanAbsoluteXDGHome(value); ok {
			return stateHome, nil
		}
	}
	if runtime.GOOS == "windows" {
		if value := os.Getenv("LOCALAPPDATA"); value != "" {
			return filepath.Join(value, "loaf", "state"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve state home: %w", err)
	}
	return filepath.Join(home, ".local", "state"), nil
}

func (r PathResolver) cacheHome() (string, error) {
	if r.CacheHome != "" {
		return cleanAbsoluteHome("cache home", r.CacheHome)
	}
	if value := os.Getenv("XDG_CACHE_HOME"); value != "" {
		if cacheHome, ok := cleanAbsoluteXDGHome(value); ok {
			return cacheHome, nil
		}
	}
	if runtime.GOOS == "windows" {
		if value := os.Getenv("LOCALAPPDATA"); value != "" {
			return filepath.Join(value, "loaf", "cache"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache home: %w", err)
	}
	return filepath.Join(home, ".cache"), nil
}

func cleanAbsoluteHome(name string, value string) (string, error) {
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be an absolute path", name)
	}
	return filepath.Clean(value), nil
}

func cleanAbsoluteXDGHome(value string) (string, bool) {
	if !filepath.IsAbs(value) {
		return "", false
	}
	return filepath.Clean(value), true
}

func isWithinRoot(path string, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ProjectID hashes the canonical project root so state paths avoid embedding
// repository or user path segments.
func ProjectID(root project.Root) string {
	sum := sha256.Sum256([]byte(root.Path()))
	return hex.EncodeToString(sum[:])
}
