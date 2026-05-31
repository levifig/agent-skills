package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicBinaryDispatchesStateAndDelegatesFallback(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is required to exercise the TypeScript fallback bridge")
	}

	repoRoot := repoRoot(t)
	binary := filepath.Join(t.TempDir(), "loaf")
	if output, err := runCommand(repoRoot, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}

	workingDir := realpath(t, t.TempDir())
	stateHome := t.TempDir()
	output, err := runBinary(binary, workingDir, envWith(
		"XDG_STATE_HOME="+stateHome,
		"LOAF_LEGACY_CLI="+filepath.Join(t.TempDir(), "missing-legacy.js"),
	), "state", "path")
	if err != nil {
		t.Fatalf("loaf state path error = %v\n%s", err, output)
	}
	statePath := strings.TrimSpace(output)
	if !strings.HasPrefix(statePath, filepath.Join(stateHome, "loaf", "projects")+string(filepath.Separator)) {
		t.Fatalf("state path = %q, want under state home %q", statePath, stateHome)
	}
	if strings.HasPrefix(statePath, workingDir+string(filepath.Separator)) {
		t.Fatalf("state path = %q, want outside working dir %q", statePath, workingDir)
	}

	fallback := writeNodeFallback(t)
	output, err = runBinary(binary, workingDir, envWith("LOAF_LEGACY_CLI="+fallback), "install", "--to", "codex")
	if err != nil {
		t.Fatalf("loaf install fallback error = %v\n%s", err, output)
	}
	if !strings.Contains(output, "args=install --to codex") {
		t.Fatalf("fallback output = %q, want delegated argv", output)
	}
	if !containsCwd(output, workingDir) {
		t.Fatalf("fallback output = %q, want cwd %q", output, workingDir)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %q missing go.mod: %v", root, err)
	}
	return root
}

func runCommand(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runBinary(binary string, dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func envWith(overrides ...string) []string {
	blocked := make(map[string]bool, len(overrides))
	for _, override := range overrides {
		if key, _, ok := strings.Cut(override, "="); ok {
			blocked[key] = true
		}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, value := range os.Environ() {
		key, _, ok := strings.Cut(value, "=")
		if ok && blocked[key] {
			continue
		}
		env = append(env, value)
	}
	return append(env, overrides...)
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return realpath
}

func containsCwd(output string, cwd string) bool {
	if strings.Contains(output, "cwd="+cwd) {
		return true
	}
	if strings.HasPrefix(cwd, "/var/") {
		return strings.Contains(output, "cwd=/private"+cwd)
	}
	return false
}

func writeNodeFallback(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy-cli.js")
	body := `process.stdout.write("cwd=" + process.cwd() + "\n");
process.stdout.write("args=" + process.argv.slice(2).join(" ") + "\n");
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(legacy-cli.js) error = %v", err)
	}
	return path
}
