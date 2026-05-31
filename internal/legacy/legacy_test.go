package legacy

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunnerPreservesArgvCwdEnvStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	nodePath := writeFakeNode(t)
	scriptPath := writeLegacyScript(t)
	cwd := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	stdin := strings.NewReader("from-stdin")

	err := Runner{
		ScriptPath: scriptPath,
		NodePath:   nodePath,
		Stdin:      stdin,
		Stdout:     &stdout,
		Stderr:     &stderr,
		Cwd:        cwd,
		Env:        append(os.Environ(), "LOAF_TEST_ENV=preserved"),
	}.Run([]string{"task", "list", "--json"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"script=" + scriptPath,
		"cwd=" + cwd,
		"args=task list --json",
		"input=from-stdin",
		"env=preserved",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want %q", out, want)
		}
	}
	if !strings.Contains(stderr.String(), "legacy stderr") {
		t.Fatalf("stderr = %q, want legacy stderr", stderr.String())
	}
}

func TestRunnerPropagatesExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	err := Runner{
		ScriptPath: writeLegacyScript(t),
		NodePath:   writeFakeNode(t),
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
		Cwd:        t.TempDir(),
	}.Run([]string{"fail"})
	if err == nil {
		t.Fatal("Run() error = nil, want exit error")
	}

	var exitErr ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Run() error = %T, want ExitError", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("ExitCode() = %d, want 7", exitErr.ExitCode())
	}
	if !exitErr.Silent() {
		t.Fatal("Silent() = false, want true")
	}
}

func TestRunnerMissingFallbackIsActionable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.js")

	err := Runner{ScriptPath: missing}.Run([]string{"--help"})
	if err == nil {
		t.Fatal("Run() error = nil, want missing fallback error")
	}
	if !strings.Contains(err.Error(), "npm run build:cli") {
		t.Fatalf("Run() error = %q, want actionable build guidance", err.Error())
	}
}

func TestRunnerFindsFallbackByWalkingUpFromCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	root := t.TempDir()
	scriptDir := filepath.Join(root, "dist-cli")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "index.js")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf 'walked-up %s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(index.js) error = %v", err)
	}
	cwd := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll(cwd) error = %v", err)
	}
	var stdout bytes.Buffer

	err := Runner{
		NodePath: writeFakeNode(t),
		Stdout:   &stdout,
		Stderr:   &bytes.Buffer{},
		Cwd:      cwd,
	}.Run([]string{"--help"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "walked-up --help") {
		t.Fatalf("stdout = %q, want walked-up fallback", stdout.String())
	}
}

func TestRunnerFindsFallbackInInstalledShareLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-only")
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("Executable() error = %v", err)
	}
	executableDir := filepath.Dir(executable)
	scriptPath := filepath.Clean(filepath.Join(executableDir, "..", "share", "loaf", legacyScriptRelative))
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(installed fallback) error = %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf 'installed-share %s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(installed fallback) error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(scriptPath)
	})

	var stdout bytes.Buffer
	err = Runner{
		NodePath: writeFakeNode(t),
		Stdout:   &stdout,
		Stderr:   &bytes.Buffer{},
		Cwd:      realpath(t, t.TempDir()),
	}.Run([]string{"task", "list"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "installed-share task list") {
		t.Fatalf("stdout = %q, want installed share fallback", stdout.String())
	}
}

func TestWalkUpScriptCandidatesIncludesAncestorsBeforeProjectRootFallback(t *testing.T) {
	got := walkUpScriptCandidates(filepath.Join("repo", "nested"))
	wantFirst := filepath.Join("repo", "nested", legacyScriptRelative)
	wantSecond := filepath.Join("repo", legacyScriptRelative)

	if len(got) < 2 {
		t.Fatalf("walkUpScriptCandidates() length = %d, want at least 2", len(got))
	}
	if got[0] != wantFirst {
		t.Fatalf("first candidate = %q, want %q", got[0], wantFirst)
	}
	if got[1] != wantSecond {
		t.Fatalf("second candidate = %q, want %q", got[1], wantSecond)
	}
}

func writeFakeNode(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "node")
	body := "#!/bin/sh\nexec \"$@\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(node) error = %v", err)
	}
	return path
}

func realpath(t *testing.T, path string) string {
	t.Helper()
	realpath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	return realpath
}

func writeLegacyScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy-cli")
	body := `#!/bin/sh
script="$0"
input="$(cat)"
printf 'script=%s\n' "$script"
printf 'cwd=%s\n' "$PWD"
printf 'args=%s\n' "$*"
printf 'input=%s\n' "$input"
printf 'env=%s\n' "$LOAF_TEST_ENV"
printf 'legacy stderr\n' >&2
if [ "$1" = "fail" ]; then
  exit 7
fi
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(legacy-cli) error = %v", err)
	}
	return path
}
