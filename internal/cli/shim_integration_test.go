package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests exercise the real argv[0] dispatch path end to end: they build
// the actual loaf binary, symlink it as "gh", and run it as a subprocess
// against a stub "real gh". Unlike shim_test.go's planGHShimExec unit tests
// (which inject fake resolvers), these prove the wiring in cmd/loaf/main.go
// and execRealGH's syscall.Exec actually behave — byte-identical fall-
// through, GH_TOKEN injection, concurrent isolation, and the recursion
// guard, all via real OS processes.

// ghRecordingStubScript stands in for the real gh. It logs every invocation
// to $INVOCATION_LOG (when set, letting a test assert an `auth status
// --active` call never happened), answers `auth token` calls with a
// deterministic per-account token, and otherwise records its final argv and
// GH_TOKEN to $RECORD_FILE (when set). When $AUTH_TOKEN_FAILS is set, the
// `auth token` branch instead writes to its own stderr and exits non-zero, so
// a test can exercise the token-unavailable fall-through.
const ghRecordingStubScript = `#!/bin/sh
if [ -n "$INVOCATION_LOG" ]; then
  { printf 'CALL:'; for a in "$@"; do printf ' %s' "$a"; done; printf '\n'; } >> "$INVOCATION_LOG"
fi
if [ "$1" = "--version" ]; then
  echo "gh version 2.96.0 (2026-07-02)"
  exit 0
fi
if [ "$1" = "auth" ] && [ "$2" = "token" ]; then
  if [ -n "$AUTH_TOKEN_FAILS" ]; then
    echo "gh: no token found for the requested account" >&2
    exit 1
  fi
  shift 2
  user=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --user) user="$2"; shift 2 ;;
      *) shift ;;
    esac
  done
  echo "token-for-${user}"
  exit 0
fi
if [ -n "$RECORD_FILE" ]; then
  {
    printf 'ARGV:'
    for a in "$@"; do printf ' %s' "$a"; done
    printf '\n'
    printf 'GH_TOKEN:%s\n' "$GH_TOKEN"
  } > "$RECORD_FILE"
fi
exit 0
`

var (
	shimTestBinaryOnce sync.Once
	shimTestBinaryPath string
	shimTestBinaryErr  error
)

// buildShimTestBinary compiles the real loaf binary once per test run and
// reuses it across every integration test in this file.
func buildShimTestBinary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("gh shim dispatch is POSIX-only")
	}
	shimTestBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "loaf-shim-binary-")
		if err != nil {
			shimTestBinaryErr = err
			return
		}
		out := filepath.Join(dir, "loaf")
		cmd := exec.Command("go", "build", "-o", out, "github.com/levifig/loaf/cmd/loaf")
		if output, err := cmd.CombinedOutput(); err != nil {
			shimTestBinaryErr = fmt.Errorf("go build cmd/loaf: %w: %s", err, output)
			return
		}
		shimTestBinaryPath = out
	})
	if shimTestBinaryErr != nil {
		t.Fatalf("buildShimTestBinary() error = %v", shimTestBinaryErr)
	}
	return shimTestBinaryPath
}

func writeStubGH(t *testing.T, dir string, script string) string {
	t.Helper()
	path := filepath.Join(dir, "gh")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", dir, err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	return path
}

// shimSymlinkPathForHome mirrors shimSymlinkPath()'s default formula
// ($HOME/.local/share when XDG_DATA_HOME isn't set) so integration tests can
// place the "gh" symlink exactly where a real `loaf shim enable gh` would,
// without needing to mutate the test process's own HOME just to ask the
// production resolver. Every test in this file leaves XDG_DATA_HOME unset in
// the child's env, so this stays in lockstep with shimSymlinkPath().
func shimSymlinkPathForHome(home string) string {
	return filepath.Join(home, ".local", "share", "loaf", "shims", "gh")
}

// symlinkShimGH creates the "gh" symlink at the real location the shim
// resolver expects for the given HOME — critical for the config-missing
// fallback test, which relies on findRealGHOnPATH excluding exactly this
// directory.
func symlinkShimGH(t *testing.T, home string, loafBinary string) string {
	t.Helper()
	path := shimSymlinkPathForHome(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.Symlink(loafBinary, path); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	return path
}

func writeConfiguredProjectFixture(t *testing.T, dir string, account string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agents"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.agents) error = %v", err)
	}
	body := fmt.Sprintf(`{"integrations":{"github":{"account":%q}}}`+"\n", account)
	if err := os.WriteFile(filepath.Join(dir, ".agents", "loaf.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(.agents/loaf.json) error = %v", err)
	}
}

func TestGHShimIntegrationFallThroughOutsideLoafProject(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	realGHDir := filepath.Join(tempRoot, "real-gh-dir")
	projectDir := filepath.Join(tempRoot, "plain-project")
	for _, d := range []string{home, projectDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", d, err)
		}
	}

	recordFile := filepath.Join(tempRoot, "record.txt")
	stubGH := writeStubGH(t, realGHDir, ghRecordingStubScript)

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, stubGH, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	shimGH := symlinkShimGH(t, home, loafBinary)

	cmd := exec.Command(shimGH, "pr", "list", "--limit", "5")
	cmd.Dir = projectDir
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + realGHDir,
		"RECORD_FILE=" + recordFile,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shim exec error = %v, output=%s", err, output)
	}

	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("record file not written: %v", err)
	}
	want := "ARGV: pr list --limit 5\nGH_TOKEN:\n"
	if string(recorded) != want {
		t.Fatalf("recorded invocation = %q, want %q (byte-identical fall-through)", recorded, want)
	}
}

func TestGHShimIntegrationFallsBackToPATHWhenConfigMissing(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	realGHDir := filepath.Join(tempRoot, "real-gh-dir")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	recordFile := filepath.Join(tempRoot, "record.txt")
	writeStubGH(t, realGHDir, ghRecordingStubScript)
	// deliberately no shims.gh config entry recorded

	shimGH := symlinkShimGH(t, home, loafBinary)

	cmd := exec.Command(shimGH, "pr", "list")
	cmd.Dir = tempRoot
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + filepath.Dir(shimGH) + string(os.PathListSeparator) + realGHDir,
		"RECORD_FILE=" + recordFile,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shim exec error = %v, output=%s", err, output)
	}

	if _, err := os.Stat(recordFile); err != nil {
		t.Fatalf("expected PATH-minus-shim-dir fallback to reach the real gh, record file missing: %v", err)
	}
}

func TestGHShimIntegrationInjectsTokenForConfiguredAccount(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	realGHDir := filepath.Join(tempRoot, "real-gh-dir")
	projectDir := filepath.Join(tempRoot, "configured-project")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	writeConfiguredProjectFixture(t, projectDir, "levifig")

	recordFile := filepath.Join(tempRoot, "record.txt")
	invocationLogFile := filepath.Join(tempRoot, "invocations.log")
	stubGH := writeStubGH(t, realGHDir, ghRecordingStubScript)

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, stubGH, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	shimGH := symlinkShimGH(t, home, loafBinary)

	cmd := exec.Command(shimGH, "pr", "list")
	cmd.Dir = projectDir
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + realGHDir,
		"RECORD_FILE=" + recordFile,
		"INVOCATION_LOG=" + invocationLogFile,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shim exec error = %v, output=%s", err, output)
	}

	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("record file missing: %v", err)
	}
	if !strings.Contains(string(recorded), "GH_TOKEN:token-for-levifig") {
		t.Fatalf("recorded invocation = %q, want GH_TOKEN for the named account levifig", recorded)
	}
	if !strings.Contains(string(recorded), "ARGV: pr list") {
		t.Fatalf("recorded invocation = %q, want the caller's argv untouched", recorded)
	}

	invocationLog, err := os.ReadFile(invocationLogFile)
	if err != nil {
		t.Fatalf("invocation log missing: %v", err)
	}
	if strings.Contains(string(invocationLog), "auth status") {
		t.Fatalf("stub recorded an `auth status` call; the shim must only ever call `auth token`: %s", invocationLog)
	}
}

// TestGHShimIntegrationNeverTouchesHostsYML is V3's zero-mutation guarantee:
// a hosts.yml-equivalent fixture, pointed at via gh's own GH_CONFIG_DIR
// mechanism, must be byte-for-byte and mtime-identical after a full
// shimmed, token-injected invocation.
func TestGHShimIntegrationNeverTouchesHostsYML(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	realGHDir := filepath.Join(tempRoot, "real-gh-dir")
	projectDir := filepath.Join(tempRoot, "configured-project")
	ghConfigDir := filepath.Join(tempRoot, "gh-config")
	for _, d := range []string{home, ghConfigDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", d, err)
		}
	}
	writeConfiguredProjectFixture(t, projectDir, "levifig")

	hostsPath := filepath.Join(ghConfigDir, "hosts.yml")
	fixtureContent := "github.com:\n    user: someone\n    oauth_token: not-a-real-token\n"
	if err := os.WriteFile(hostsPath, []byte(fixtureContent), 0o600); err != nil {
		t.Fatalf("WriteFile(hosts.yml) error = %v", err)
	}
	before, err := os.Stat(hostsPath)
	if err != nil {
		t.Fatalf("Stat(hosts.yml) error = %v", err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	stubGH := writeStubGH(t, realGHDir, ghRecordingStubScript)
	if err := writeShimUserConfig(configPath, stubGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	shimGH := symlinkShimGH(t, home, loafBinary)

	cmd := exec.Command(shimGH, "pr", "list")
	cmd.Dir = projectDir
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + realGHDir,
		"GH_CONFIG_DIR=" + ghConfigDir,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shim exec error = %v, output=%s", err, output)
	}

	after, err := os.Stat(hostsPath)
	if err != nil {
		t.Fatalf("Stat(hosts.yml) after error = %v", err)
	}
	afterContent, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts.yml) after error = %v", err)
	}
	if string(afterContent) != fixtureContent {
		t.Fatalf("hosts.yml content changed:\n got:  %q\n want: %q", afterContent, fixtureContent)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("hosts.yml mtime changed: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestGHShimIntegrationTokenUnavailableFallsThroughWithNote closes the other
// half of V3: when the named-account token read fails (the stub's `auth token`
// exits non-zero), the shim must fall through to the real gh with the caller's
// argv and env untouched — no GH_TOKEN — emit exactly one stderr note naming
// the account (never the token or the underlying exec error), and still leave
// gh's hosts.yml byte-for-byte and mtime-identical.
func TestGHShimIntegrationTokenUnavailableFallsThroughWithNote(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	realGHDir := filepath.Join(tempRoot, "real-gh-dir")
	projectDir := filepath.Join(tempRoot, "configured-project")
	ghConfigDir := filepath.Join(tempRoot, "gh-config")
	for _, d := range []string{home, ghConfigDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", d, err)
		}
	}
	writeConfiguredProjectFixture(t, projectDir, "levifig")

	hostsPath := filepath.Join(ghConfigDir, "hosts.yml")
	fixtureContent := "github.com:\n    user: someone\n    oauth_token: not-a-real-token\n"
	if err := os.WriteFile(hostsPath, []byte(fixtureContent), 0o600); err != nil {
		t.Fatalf("WriteFile(hosts.yml) error = %v", err)
	}
	before, err := os.Stat(hostsPath)
	if err != nil {
		t.Fatalf("Stat(hosts.yml) error = %v", err)
	}

	recordFile := filepath.Join(tempRoot, "record.txt")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	stubGH := writeStubGH(t, realGHDir, ghRecordingStubScript)
	if err := writeShimUserConfig(configPath, stubGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	shimGH := symlinkShimGH(t, home, loafBinary)

	cmd := exec.Command(shimGH, "pr", "list")
	cmd.Dir = projectDir
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + realGHDir,
		"RECORD_FILE=" + recordFile,
		"GH_CONFIG_DIR=" + ghConfigDir,
		"AUTH_TOKEN_FAILS=1",
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("shim exec error = %v, stderr=%s", err, stderr.String())
	}

	// Fell through to the real gh with the caller's argv and no GH_TOKEN.
	recorded, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("record file missing: %v", err)
	}
	want := "ARGV: pr list\nGH_TOKEN:\n"
	if string(recorded) != want {
		t.Fatalf("recorded invocation = %q, want %q (untouched fall-through, no token)", recorded, want)
	}

	// Exactly the one account-naming note on stderr — no token value, no exec
	// error detail from the failed `auth token` sub-invocation.
	wantNote := "loaf gh-shim: token for \"levifig\" unavailable; running unshimmed\n"
	if stderr.String() != wantNote {
		t.Fatalf("stderr = %q, want exactly %q", stderr.String(), wantNote)
	}

	// gh's own hosts.yml is untouched, exactly as on the happy path.
	after, err := os.Stat(hostsPath)
	if err != nil {
		t.Fatalf("Stat(hosts.yml) after error = %v", err)
	}
	afterContent, err := os.ReadFile(hostsPath)
	if err != nil {
		t.Fatalf("ReadFile(hosts.yml) after error = %v", err)
	}
	if string(afterContent) != fixtureContent {
		t.Fatalf("hosts.yml content changed:\n got:  %q\n want: %q", afterContent, fixtureContent)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("hosts.yml mtime changed: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestGHShimIntegrationConcurrentInvocationsGetOwnTokens proves no shared
// mutable state survives between simultaneous invocations: two different
// projects, two different accounts, run at the same instant, each receiving
// only its own token.
func TestGHShimIntegrationConcurrentInvocationsGetOwnTokens(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	realGHDir := filepath.Join(tempRoot, "real-gh-dir")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	stubGH := writeStubGH(t, realGHDir, ghRecordingStubScript)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, stubGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	shimGH := symlinkShimGH(t, home, loafBinary)

	type invocation struct {
		project string
		account string
	}
	invocations := []invocation{
		{"project-a", "levifig"},
		{"project-b", "levifigueira"},
	}

	type result struct {
		recordFile string
		err        error
		output     []byte
	}
	results := make([]result, len(invocations))
	var wg sync.WaitGroup
	for i, inv := range invocations {
		i, inv := i, inv
		projectDir := filepath.Join(tempRoot, inv.project)
		writeConfiguredProjectFixture(t, projectDir, inv.account)
		recordFile := filepath.Join(tempRoot, inv.project+"-record.txt")

		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(shimGH, "pr", "list")
			cmd.Dir = projectDir
			cmd.Env = []string{
				"HOME=" + home,
				"PATH=" + realGHDir,
				"RECORD_FILE=" + recordFile,
			}
			output, err := cmd.CombinedOutput()
			results[i] = result{recordFile: recordFile, err: err, output: output}
		}()
	}
	wg.Wait()

	for i, res := range results {
		if res.err != nil {
			t.Fatalf("invocation %d (%s) error = %v, output=%s", i, invocations[i].project, res.err, res.output)
		}
		recorded, err := os.ReadFile(res.recordFile)
		if err != nil {
			t.Fatalf("invocation %d record file missing: %v", i, err)
		}
		want := fmt.Sprintf("GH_TOKEN:token-for-%s", invocations[i].account)
		if !strings.Contains(string(recorded), want) {
			t.Fatalf("invocation %d (%s) recorded = %q, want it to contain %q", i, invocations[i].project, recorded, want)
		}
	}
}

// TestGHShimIntegrationRecursionGuardRefusesSelfReferentialConfig covers a
// misconfigured shims.gh.real_path that points back at the shim itself (or,
// as here, directly at the loaf binary it symlinks to) — the guard must
// refuse rather than exec into an infinite loop.
func TestGHShimIntegrationRecursionGuardRefusesSelfReferentialConfig(t *testing.T) {
	loafBinary := buildShimTestBinary(t)
	tempRoot := realpath(t, t.TempDir())
	home := filepath.Join(tempRoot, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	shimGH := symlinkShimGH(t, home, loafBinary)

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	// Misconfigure: real_path points at the shim symlink itself.
	if err := writeShimUserConfig(configPath, shimGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	cmd := exec.Command(shimGH, "pr", "list")
	cmd.Dir = tempRoot
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + filepath.Dir(shimGH),
	}
	var combined strings.Builder
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() error = %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case runErr := <-done:
		if runErr == nil {
			t.Fatalf("want a non-zero exit refusing to recurse, got success; output=%s", combined.String())
		}
		if !strings.Contains(combined.String(), "recurse") {
			t.Fatalf("output = %q, want it to fail specifically on the recursion guard (not some other resolution error)", combined.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("gh shim did not terminate within 10s; recursion guard likely failed to trigger")
	}
}
