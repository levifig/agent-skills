package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These end-to-end tests pin installed-distribution authority: a current
// installed Loaf executable invoked from inside an older source checkout must
// report and install its own shipped distribution, never the checkout's. They
// run the real compiled binary with fully isolated HOME/XDG/LOAF_DB/git
// configuration so no global state is read or mutated.

const (
	installedTestVersion = "9.9.9-current"
	staleTestVersion     = "1.1.1-stale"
)

func TestInstalledDistributionVersionAuthorityFromStaleCheckout(t *testing.T) {
	repo := repoRoot(t)
	installedRoot := writeInstalledDistributionFixture(t, repo, installedTestVersion)
	staleCheckout := writeStaleCheckoutFixture(t, staleTestVersion)
	env := isolatedInstallEnv(t)

	output, err := runInstalledLoaf(installedRoot, staleCheckout, env, "version")
	if err != nil {
		t.Fatalf("installed loaf version from stale checkout error = %v\n%s", err, output)
	}
	if !strings.Contains(output, installedTestVersion) {
		t.Fatalf("version output = %q, want installed distribution version %q", output, installedTestVersion)
	}
	// The fixture binary is compiled here and carries no release metadata, yet
	// the distribution around it is a release-archive layout — the same shape
	// the plugin marketplace and npm serve from committed binaries. Missing
	// metadata alone must never dress one of those as a dev build.
	if strings.Contains(output, "(dev build)") {
		t.Fatalf("version output = %q, must not report a shipped distribution as a dev build", output)
	}
	if strings.Contains(output, staleTestVersion) {
		t.Fatalf("version output = %q, must not report the stale checkout version %q", output, staleTestVersion)
	}
}

func TestInstalledDistributionUpgradeAuthorityFromStaleCheckout(t *testing.T) {
	repo := repoRoot(t)
	installedRoot := writeInstalledDistributionFixture(t, repo, installedTestVersion)
	staleCheckout := writeStaleCheckoutFixture(t, staleTestVersion)
	env := isolatedInstallEnv(t)
	home := envValue(t, env, "HOME")

	// A pre-existing Cursor install so the upgrade selects it, and a project
	// config so the checkout reads as the Loaf repo it is — that is what opens
	// the project half of the upgrade.
	writeFixtureFile(t, filepath.Join(home, ".cursor", ".loaf-version"), staleTestVersion+"\n")
	writeFixtureFile(t, filepath.Join(home, ".cursor", "skills", "foundations", "SKILL.md"), "# Previously Installed Foundations\n")
	writeFixtureFile(t, filepath.Join(staleCheckout, ".agents", "loaf.json"), "{\"integrations\":{}}\n")

	output, err := runInstalledLoaf(installedRoot, staleCheckout, env, "upgrade", "--yes")
	if err != nil {
		t.Fatalf("installed loaf upgrade from stale checkout error = %v\n%s", err, output)
	}

	marker := readFixtureFile(t, filepath.Join(home, ".cursor", ".loaf-version"))
	if strings.TrimSpace(marker) != installedTestVersion {
		t.Fatalf(".cursor/.loaf-version = %q, want installed distribution version %q", marker, installedTestVersion)
	}
	skill := readFixtureFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"))
	if !strings.Contains(skill, "# Current Foundations") {
		t.Fatalf("installed skill = %q, want packaged content from the installed distribution", skill)
	}
	if strings.Contains(skill, "Stale") {
		t.Fatalf("installed skill = %q, must not come from the stale checkout dist/", skill)
	}
	fenced := readFixtureFile(t, filepath.Join(staleCheckout, "AGENTS.md"))
	if !strings.Contains(fenced, "<!-- loaf:managed:start sha256=") || !strings.Contains(fenced, "## Loaf Framework") {
		t.Fatalf("project AGENTS.md fenced section = %q, want sha256-only managed section from installed binary", fenced)
	}
	if strings.Contains(fenced, "<!-- loaf:managed:start v") {
		t.Fatalf("project AGENTS.md fenced section = %q, must not embed a version stamp", fenced)
	}
	if strings.Contains(fenced, staleTestVersion) {
		t.Fatalf("project AGENTS.md = %q, must not reference the stale checkout version %q", fenced, staleTestVersion)
	}
}

// A source checkout's own binary keeps the checkout as its distribution root —
// the explicit local-development opt-in is running that binary, regardless of
// the invoking working directory.
func TestInstalledDistributionCheckoutOwnBinaryReportsCheckoutVersion(t *testing.T) {
	repo := repoRoot(t)
	checkout := writeStaleCheckoutFixture(t, "3.3.3-dev")
	nativeDir := filepath.Join(checkout, "bin", "native", "test-target")
	copyFixtureBinary(t, sharedTestLoafBinary(t, repo), filepath.Join(nativeDir, "loaf"))
	writeFixtureFile(t, filepath.Join(checkout, "bin", "package.json"), "{\n  \"type\": \"commonjs\"\n}\n")
	writeFixtureFile(t, filepath.Join(checkout, "bin", ".loaf-dev-commit"), "abc1234\n")
	elsewhere := realpath(t, t.TempDir())
	env := isolatedInstallEnv(t)

	cmd := exec.Command(filepath.Join(nativeDir, "loaf"), "version")
	cmd.Dir = elsewhere
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("checkout-owned loaf version error = %v\n%s", err, output)
	}
	// A checkout's own binary is a dev build by definition, so it reports the
	// dev identity built from its owning checkout's package version and recorded
	// commit rather than the package version alone.
	if !strings.Contains(string(output), "loaf\x1b[0m 3.3.3-dev+gabc1234 (dev build)") {
		t.Fatalf("version output = %q, want the owning checkout's dev identity", output)
	}
}

// A bare binary with no adjacent distribution invoked from inside a stale
// checkout must degrade to identity-only output: version reports 0.0.0 with
// reinstall guidance, and no Targets list or Content counts may be read from
// the checkout the caller is standing in.
func TestInstalledDistributionBareBinaryVersionOmitsCheckoutTargetsAndContent(t *testing.T) {
	repo := repoRoot(t)
	bareBinary := filepath.Join(realpath(t, t.TempDir()), "loaf")
	copyFixtureBinary(t, sharedTestLoafBinary(t, repo), bareBinary)
	staleCheckout := writeStaleCheckoutFixture(t, staleTestVersion)
	env := isolatedInstallEnv(t)

	cmd := exec.Command(bareBinary, "version")
	cmd.Dir = staleCheckout
	cmd.Env = env
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	if err != nil {
		t.Fatalf("bare loaf version error = %v\n%s", err, output)
	}
	if !strings.Contains(output, "0.0.0") {
		t.Fatalf("version output = %q, want honest 0.0.0 fallback", output)
	}
	if !strings.Contains(output, "reinstall Loaf") {
		t.Fatalf("version output = %q, want the resolver's reinstall guidance", output)
	}
	if strings.Contains(output, staleTestVersion) {
		t.Fatalf("version output = %q, must not report the stale checkout version %q", output, staleTestVersion)
	}
	for _, forbidden := range []string{"Targets:", "Content:", "Skills:", "Agents:", "Hooks:", "dist/cursor/"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("version output = %q, must not surface the stale checkout's %q", output, forbidden)
		}
	}
}

// writeInstalledDistributionFixture lays out a release-archive style install:
// the native binary at bin/loaf with package.json, content, config, and built
// dist/ targets adjacent — the shape shipped by package-release.mjs.
func writeInstalledDistributionFixture(t *testing.T, repo string, version string) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	copyFixtureBinary(t, sharedTestLoafBinary(t, repo), filepath.Join(root, "bin", "loaf"))
	writeFixtureFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"`+version+`"}`)
	writeFixtureFile(t, filepath.Join(root, "content", "skills", "foundations", "SKILL.md"), "# Current Foundations\n")
	writeFixtureFile(t, filepath.Join(root, "config", "hooks.yaml"), "hooks:\n  pre-tool:\n    - id: check-secrets\n")
	writeFixtureFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Current Foundations\n")
	writeFixtureFile(t, filepath.Join(root, "dist", "cursor", "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","loaf-managed":true}]}}`)
	writeFixtureHookDistribution(t, filepath.Join(root, "dist", "cursor"), version)
	return root
}

// writeFixtureHookDistribution adds the two files a current build emits beside
// a Cursor distribution: the adapter manifest the installer reads, and the hook
// catalog reconciliation resolves entry identity through. A distribution
// carrying neither is stale, and upgrade refuses it rather than falling back to
// the whole-file merge it replaced.
func writeFixtureHookDistribution(t *testing.T, dist string, version string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(dist, ".loaf-target-manifest.json"), `{
  "version": 1,
  "target": "cursor",
  "package_version": "`+version+`",
  "capability_contract_version": 3,
  "adapters": ["cursor-test-adapter-v1"],
  "artifacts": [
    {
      "id": "managed-instructions",
      "kind": "instruction",
      "destination": "project-instructions",
      "sha256": "`+strings.Repeat("0", 64)+`"
    }
  ]
}
`)
	writeFixtureFile(t, filepath.Join(dist, ".loaf-hook-catalog.json"), `{
  "version": 1,
  "target": "cursor",
  "package_version": "`+version+`",
  "entries": [
    {
      "event": "beforeShellExecution",
      "hook_id": "validate-commit",
      "type": "command",
      "template": {"command": "loaf check --hook validate-commit", "matcher": "Bash", "loaf-managed": true},
      "signatures": [["loaf", "check", "--hook", "validate-commit"]],
      "stems": [["--hook", "validate-commit"]]
    }
  ],
  "cohorts": []
}
`)
}

// writeStaleCheckoutFixture lays out an older Loaf source checkout: package
// metadata, a .git marker, the Go module that makes it a checkout rather than
// an unpacked distribution, content, and stale built dist/ output.
func writeStaleCheckoutFixture(t *testing.T, version string) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	writeFixtureFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"`+version+`"}`)
	writeFixtureFile(t, filepath.Join(root, "go.mod"), "module github.com/levifig/loaf\n\ngo 1.25.0\n")
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll .git error = %v", err)
	}
	writeFixtureFile(t, filepath.Join(root, "content", "skills", "foundations", "SKILL.md"), "# Stale Foundations\n")
	writeFixtureFile(t, filepath.Join(root, "config", "hooks.yaml"), "hooks:\n  pre-tool:\n    - id: check-secrets\n")
	writeFixtureFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Stale Foundations\n")
	writeFixtureFile(t, filepath.Join(root, "dist", "cursor", "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","loaf-managed":true}]}}`)
	return root
}

func runInstalledLoaf(installedRoot string, workingDir string, env []string, args ...string) (string, error) {
	cmd := exec.Command(filepath.Join(installedRoot, "bin", "loaf"), args...)
	cmd.Dir = workingDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// isolatedInstallEnv builds a process environment whose HOME, XDG paths, Loaf
// database, git configuration, and PATH all point at throwaway locations so
// the tests can never read or mutate real installations.
func isolatedInstallEnv(t *testing.T) []string {
	t.Helper()
	home := realpath(t, t.TempDir())
	return envWith(
		"HOME="+home,
		"USERPROFILE="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_DATA_HOME="+filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME="+filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"LOAF_DB="+filepath.Join(home, "loaf-test.sqlite"),
		"CODEX_HOME="+filepath.Join(home, ".codex"),
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig-isolated"),
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"PATH=/usr/bin:/bin",
	)
}

func envValue(t *testing.T, env []string, key string) string {
	t.Helper()
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, key+"="); ok {
			return value
		}
	}
	t.Fatalf("environment fixture missing %s", key)
	return ""
}

var (
	sharedTestBinaryDir  string
	sharedTestBinaryPath string
)

// TestMain removes the shared compiled binary's temp directory after every
// test in the package has finished. Cleanup cannot hang off the first
// requesting test (later tests still copy from the artifact), so it runs
// once, after m.Run returns, when no test can race the deletion.
func TestMain(m *testing.M) {
	code := m.Run()
	if sharedTestBinaryDir != "" {
		os.RemoveAll(sharedTestBinaryDir)
	}
	os.Exit(code)
}

// sharedTestLoafBinary compiles cmd/loaf once per test process; fixtures copy
// the artifact into their own layouts. TestMain deletes the directory after
// the package's tests complete.
func sharedTestLoafBinary(t *testing.T, repo string) string {
	t.Helper()
	if sharedTestBinaryPath != "" {
		return sharedTestBinaryPath
	}
	dir, err := os.MkdirTemp("", "loaf-installed-dist-")
	if err != nil {
		t.Fatalf("MkdirTemp error = %v", err)
	}
	binary := filepath.Join(dir, "loaf")
	if output, err := runCommand(repo, "go", "build", "-o", binary, "./cmd/loaf"); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("go build ./cmd/loaf error = %v\n%s", err, output)
	}
	sharedTestBinaryDir = dir
	sharedTestBinaryPath = binary
	return binary
}

func copyFixtureBinary(t *testing.T, source string, dest string) {
	t.Helper()
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(dest, body, 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", dest, err)
	}
}

func writeFixtureFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}
