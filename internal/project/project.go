package project

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// WorkingDirectory is the normalized process location used by future project
// identity and worktree-resolution code.
type WorkingDirectory struct {
	path string
}

// ResolveWorkingDirectory returns an absolute, cleaned working directory. An
// empty start path means the current process directory.
func ResolveWorkingDirectory(start string) (WorkingDirectory, error) {
	if start == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return WorkingDirectory{}, err
		}
		start = cwd
	}

	abs, err := filepath.Abs(start)
	if err != nil {
		return WorkingDirectory{}, err
	}

	return WorkingDirectory{path: filepath.Clean(abs)}, nil
}

// Path returns the absolute, cleaned directory path.
func (w WorkingDirectory) Path() string {
	return w.path
}

// Root is the canonical project identity input used for state-path
// calculation. In Git repositories this is the main checkout root; outside Git
// it is the resolved working directory.
type Root struct {
	path string
}

// ResolveRoot returns the project root for state identity. Linked Git
// worktrees resolve to the main worktree root by comparing git-dir with
// git-common-dir, matching the existing TypeScript resolver's worktree model.
func ResolveRoot(start string) (Root, error) {
	workingDir, err := ResolveWorkingDirectory(start)
	if err != nil {
		return Root{}, err
	}

	if gitRoot, ok := resolveGitRoot(workingDir.Path()); ok {
		return Root{path: filepath.Clean(gitRoot)}, nil
	}
	return Root{path: workingDir.Path()}, nil
}

// Path returns the canonical project root path.
func (r Root) Path() string {
	return r.path
}

func resolveGitRoot(start string) (string, bool) {
	gitDir, ok := gitOutput(start, "rev-parse", "--path-format=absolute", "--git-dir")
	if !ok {
		return "", false
	}
	commonDir, ok := gitOutput(start, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if !ok {
		return "", false
	}

	gitDirAbs := normalizePathForComparison(resolveAgainst(start, gitDir))
	commonDirAbs := normalizePathForComparison(resolveAgainst(start, commonDir))

	if gitDirAbs != commonDirAbs {
		commonCanonical := realpathOrSelf(resolveAgainst(start, commonDir))
		if strings.HasSuffix(commonCanonical, string(filepath.Separator)+".git") {
			return filepath.Dir(commonCanonical), true
		}
		return "", false
	}

	root, ok := gitOutput(start, "rev-parse", "--show-toplevel")
	if !ok {
		return "", false
	}
	return realpathOrSelf(resolveAgainst(start, root)), true
}

func resolveAgainst(base string, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}

func gitOutput(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	value := strings.TrimSpace(string(out))
	if value == "" {
		return "", false
	}
	return value, true
}

func normalizePathForComparison(path string) string {
	normalized := realpathOrSelf(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(normalized)
	}
	return normalized
}

func realpathOrSelf(path string) string {
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(realpath)
}
