package legacy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/levifig/loaf/internal/project"
)

const (
	defaultNodeExecutable = "node"
	legacyScriptRelative  = "dist-cli/index.js"
)

// Runner delegates unmigrated commands to the bundled TypeScript CLI.
type Runner struct {
	ScriptPath string
	NodePath   string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Cwd        string
	Env        []string
}

// ExitError reports a child-process exit code that the front controller should
// propagate without printing an additional wrapper error.
type ExitError struct {
	Code int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("legacy command exited with code %d", e.Code)
}

// ExitCode returns the child-process exit code.
func (e ExitError) ExitCode() int {
	return e.Code
}

// Silent tells the top-level CLI that the child already owned stderr.
func (e ExitError) Silent() bool {
	return true
}

// Run executes the legacy TypeScript CLI with argv, cwd, env, and stdio
// preserved.
func (r Runner) Run(args []string) error {
	scriptPath, err := r.resolveScriptPath()
	if err != nil {
		return err
	}

	nodePath := r.NodePath
	if nodePath == "" {
		nodePath = defaultNodeExecutable
	}

	cmd := exec.Command(nodePath, append([]string{scriptPath}, args...)...)
	cmd.Dir = r.cwd()
	cmd.Env = r.env()
	cmd.Stdin = r.stdin()
	cmd.Stdout = r.stdout()
	cmd.Stderr = r.stderr()

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return ExitError{Code: exitErr.ExitCode()}
		}
		return fmt.Errorf("run TypeScript fallback: %w", err)
	}
	return nil
}

func (r Runner) resolveScriptPath() (string, error) {
	if r.ScriptPath != "" {
		if exists(r.ScriptPath) {
			return r.ScriptPath, nil
		}
		return "", fmt.Errorf("TypeScript fallback not found at %s; run `npm run build:cli` or set LOAF_LEGACY_CLI", r.ScriptPath)
	}

	if value := os.Getenv("LOAF_LEGACY_CLI"); value != "" {
		if exists(value) {
			return value, nil
		}
		return "", fmt.Errorf("TypeScript fallback from LOAF_LEGACY_CLI not found at %s", value)
	}

	for _, candidate := range r.candidateScriptPaths() {
		if exists(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("TypeScript fallback not found; run `npm run build:cli` to create %s", legacyScriptRelative)
}

func (r Runner) candidateScriptPaths() []string {
	var candidates []string

	if cwd := r.cwd(); cwd != "" {
		candidates = append(candidates, walkUpScriptCandidates(cwd)...)
		if root, err := project.ResolveRoot(cwd); err == nil {
			candidates = append(candidates, filepath.Join(root.Path(), legacyScriptRelative))
		}
	}

	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, legacyScriptRelative),
			filepath.Join(executableDir, "..", legacyScriptRelative),
			filepath.Join(executableDir, "..", "share", "loaf", legacyScriptRelative),
		)
	}

	return candidates
}

func walkUpScriptCandidates(start string) []string {
	var candidates []string
	current := filepath.Clean(start)
	for {
		candidates = append(candidates, filepath.Join(current, legacyScriptRelative))
		parent := filepath.Dir(current)
		if parent == current {
			return candidates
		}
		current = parent
	}
}

func (r Runner) cwd() string {
	if r.Cwd != "" {
		return r.Cwd
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func (r Runner) env() []string {
	if r.Env != nil {
		return r.Env
	}
	return os.Environ()
}

func (r Runner) stdin() io.Reader {
	if r.Stdin != nil {
		return r.Stdin
	}
	return os.Stdin
}

func (r Runner) stdout() io.Writer {
	if r.Stdout != nil {
		return r.Stdout
	}
	return os.Stdout
}

func (r Runner) stderr() io.Writer {
	if r.Stderr != nil {
		return r.Stderr
	}
	return os.Stderr
}

func exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
