// Package devtool is Loaf's build, release, and packaging tooling. It replaces
// the npm scripts that used to live under cli/scripts: everything a checkout
// needs to produce the native binaries, the distributed content, the release
// archives, and the Homebrew formula is Go, invoked through `go run
// ./cmd/loafdev` or the Makefile. Nothing here ships in a distribution.
package devtool

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Target is one native build target in Node's platform-arch vocabulary, which
// the distribution layout (bin/native/<runtime-id>/) has always used.
type Target struct {
	RuntimeID string
	GOOS      string
	GOARCH    string
}

// BinaryName is the native binary's file name for this target.
func (t Target) BinaryName() string {
	if t.GOOS == "windows" {
		return "loaf.exe"
	}
	return "loaf"
}

var supportedTargets = []Target{
	{"darwin-arm64", "darwin", "arm64"},
	{"darwin-x64", "darwin", "amd64"},
	{"linux-arm64", "linux", "arm64"},
	{"linux-x64", "linux", "amd64"},
	{"win32-arm64", "windows", "arm64"},
	{"win32-x64", "windows", "amd64"},
}

// ReleaseTargets are the platforms every release publishes.
func ReleaseTargets() []Target {
	return append([]Target(nil), supportedTargets...)
}

// SupportedRuntimeIDs lists the accepted target names for error messages.
func SupportedRuntimeIDs() []string {
	ids := make([]string, 0, len(supportedTargets))
	for _, target := range supportedTargets {
		ids = append(ids, target.RuntimeID)
	}
	return ids
}

// TargetFromRuntimeID resolves a name such as "linux-x64".
func TargetFromRuntimeID(runtimeID string) (Target, error) {
	for _, target := range supportedTargets {
		if target.RuntimeID == runtimeID {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("unsupported LOAF_BUILD_TARGETS entry %q. Supported targets: %s", runtimeID, strings.Join(SupportedRuntimeIDs(), ", "))
}

// TargetForGo maps a GOOS/GOARCH pair to its runtime id.
func TargetForGo(goos, goarch string) (Target, error) {
	for _, target := range supportedTargets {
		if target.GOOS == goos && target.GOARCH == goarch {
			return target, nil
		}
	}
	return Target{}, fmt.Errorf("no native target for %s/%s", goos, goarch)
}

// ParseTargetList splits a comma-separated, deduplicated target list.
func ParseTargetList(value string) ([]Target, error) {
	var targets []Target
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		target, err := TargetFromRuntimeID(part)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// RuntimeIDs renders targets back to their comma-separated list form.
func RuntimeIDs(targets []Target) string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.RuntimeID)
	}
	return strings.Join(ids, ",")
}

// Env is the process environment as a map, so commands can be tested without
// touching the real environment.
type Env map[string]string

// EnvFromOS snapshots os.Environ.
func EnvFromOS() Env {
	env := Env{}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	return env
}

// Slice renders the environment for exec.Cmd.
func (env Env) Slice() []string {
	entries := make([]string, 0, len(env))
	for key, value := range env {
		entries = append(entries, key+"="+value)
	}
	return entries
}

// With returns a copy with overrides applied.
func (env Env) With(overrides map[string]string) Env {
	next := Env{}
	for key, value := range env {
		next[key] = value
	}
	for key, value := range overrides {
		next[key] = value
	}
	return next
}

// requestedTargets reads the first non-empty variable among names as a target
// list, or falls back to the current Go toolchain target.
func requestedTargets(env Env, rootDir string, run Runner, names ...string) ([]Target, error) {
	for _, name := range names {
		if value := strings.TrimSpace(env[name]); value != "" {
			return ParseTargetList(value)
		}
	}
	current, err := currentGoTarget(env, rootDir, run)
	if err != nil {
		return nil, err
	}
	return []Target{current}, nil
}

var goEnvLine = regexp.MustCompile(`^\S+$`)

func currentGoTarget(env Env, rootDir string, run Runner) (Target, error) {
	output, err := run.Output(rootDir, env, "go", "env", "GOOS", "GOARCH")
	if err != nil {
		return Target{}, fmt.Errorf("could not determine Go target from `go env GOOS GOARCH`: %w", err)
	}
	fields := strings.Fields(output)
	if len(fields) != 2 || !goEnvLine.MatchString(fields[0]) || !goEnvLine.MatchString(fields[1]) {
		return Target{}, fmt.Errorf("could not determine Go target from `go env GOOS GOARCH`: %q", output)
	}
	return TargetForGo(fields[0], fields[1])
}

// PinnedToolchainEnv reads the `toolchain` directive from go.mod so every Go
// invocation uses the pinned version, matching what CI installs.
func PinnedToolchainEnv(rootDir string) map[string]string {
	body, err := os.ReadFile(filepath.Join(rootDir, "go.mod"))
	if err != nil {
		return nil
	}
	match := regexp.MustCompile(`(?m)^toolchain\s+(\S+)\s*$`).FindStringSubmatch(string(body))
	if match == nil {
		return nil
	}
	return map[string]string{"GOTOOLCHAIN": match[1]}
}

// Runner executes external commands. Tests substitute a recorder.
type Runner interface {
	// Run executes and streams output to the caller's stdio.
	Run(dir string, env Env, name string, args ...string) error
	// Output executes and returns stdout.
	Output(dir string, env Env, name string, args ...string) (string, error)
}

// ExecRunner runs real processes.
type ExecRunner struct {
	Stdout *os.File
	Stderr *os.File
}

func (r ExecRunner) command(dir string, env Env, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env.Slice()
	return cmd
}

func (r ExecRunner) Run(dir string, env Env, name string, args ...string) error {
	cmd := r.command(dir, env, name, args...)
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func (r ExecRunner) Output(dir string, env Env, name string, args ...string) (string, error) {
	cmd := r.command(dir, env, name, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("%w: %s", err, detail)
		}
		return "", err
	}
	return string(output), nil
}
