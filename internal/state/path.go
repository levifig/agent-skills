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
	StateHome string
}

// DatabasePath returns the intended SQLite database path for a project root.
func (r PathResolver) DatabasePath(root project.Root) (string, error) {
	stateHome, err := r.stateHome()
	if err != nil {
		return "", err
	}
	if isWithinRoot(stateHome, root.Path()) {
		return "", fmt.Errorf("state home must be outside project root")
	}
	projectID := ProjectID(root)
	return filepath.Join(stateHome, "loaf", "projects", projectID, databaseFileName), nil
}

func (r PathResolver) stateHome() (string, error) {
	if r.StateHome != "" {
		return cleanAbsoluteStateHome("state home", r.StateHome)
	}
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		if stateHome, ok := cleanAbsoluteXDGStateHome(value); ok {
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

func cleanAbsoluteStateHome(name string, value string) (string, error) {
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%s must be an absolute path", name)
	}
	return filepath.Clean(value), nil
}

func cleanAbsoluteXDGStateHome(value string) (string, bool) {
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
