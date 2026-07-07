package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func setupShimTestHome(t *testing.T) string {
	t.Helper()
	home := realpath(t, t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	return home
}

func writeExecutableFixture(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// --- planGHShimExec: the failure matrix ---

func TestPlanGHShimExecFallsThroughOutsideLoafProject(t *testing.T) {
	dir := realpath(t, t.TempDir()) // no .git, no .agents/loaf.json anywhere above it in the sandbox
	argv := []string{"gh", "pr", "list", "--limit", "5"}
	environ := []string{"PATH=/usr/bin", "HOME=/x"}
	tokenCalled := false

	plan, err := planGHShimExec(argv, environ, dir, "/opt/loaf-binary",
		func() (string, error) { return "/opt/real/gh", nil },
		func(string, string) (string, error) { tokenCalled = true; return "unused", nil },
	)
	if err != nil {
		t.Fatalf("planGHShimExec() error = %v", err)
	}
	if plan.Path != "/opt/real/gh" {
		t.Fatalf("plan.Path = %q, want /opt/real/gh", plan.Path)
	}
	wantArgs := []string{"/opt/real/gh", "pr", "list", "--limit", "5"}
	if !reflect.DeepEqual(plan.Args, wantArgs) {
		t.Fatalf("plan.Args = %v, want %v", plan.Args, wantArgs)
	}
	if !reflect.DeepEqual(plan.Env, environ) {
		t.Fatalf("plan.Env = %v, want untouched %v", plan.Env, environ)
	}
	if plan.StderrNote != "" {
		t.Fatalf("plan.StderrNote = %q, want empty for silent fall-through", plan.StderrNote)
	}
	if tokenCalled {
		t.Fatalf("token resolver was called outside a configured project")
	}
}

func TestPlanGHShimExecFallsThroughWhenNoAccountConfigured(t *testing.T) {
	dir := realpath(t, t.TempDir())
	writeCheckFile(t, dir, ".git", "gitdir: fake\n")
	// no .agents/loaf.json at all
	tokenCalled := false

	plan, err := planGHShimExec([]string{"gh", "repo", "view"}, []string{"HOME=/x"}, dir, "/opt/loaf-binary",
		func() (string, error) { return "/opt/real/gh", nil },
		func(string, string) (string, error) { tokenCalled = true; return "unused", nil },
	)
	if err != nil {
		t.Fatalf("planGHShimExec() error = %v", err)
	}
	if plan.StderrNote != "" {
		t.Fatalf("plan.StderrNote = %q, want empty", plan.StderrNote)
	}
	if tokenCalled {
		t.Fatalf("token resolver was called with no configured account")
	}
}

func TestPlanGHShimExecInjectsTokenForConfiguredAccount(t *testing.T) {
	dir := realpath(t, t.TempDir())
	writeCheckFile(t, dir, ".git", "gitdir: fake\n")
	writeCheckFile(t, dir, ".agents/loaf.json", `{"integrations":{"github":{"account":"levifig"}}}`+"\n")

	environ := []string{"PATH=/usr/bin", "GH_TOKEN=stale-value"}
	plan, err := planGHShimExec([]string{"gh", "pr", "list"}, environ, dir, "/opt/loaf-binary",
		func() (string, error) { return "/opt/real/gh", nil },
		func(realGH string, account string) (string, error) {
			if realGH != "/opt/real/gh" {
				t.Fatalf("resolveToken realGH = %q, want /opt/real/gh", realGH)
			}
			if account != "levifig" {
				t.Fatalf("resolveToken account = %q, want levifig", account)
			}
			return "tok123", nil
		},
	)
	if err != nil {
		t.Fatalf("planGHShimExec() error = %v", err)
	}
	if plan.StderrNote != "" {
		t.Fatalf("plan.StderrNote = %q, want empty on success", plan.StderrNote)
	}
	wantEnv := []string{"PATH=/usr/bin", "GH_TOKEN=tok123"}
	if !reflect.DeepEqual(plan.Env, wantEnv) {
		t.Fatalf("plan.Env = %v, want %v (stale GH_TOKEN replaced, no other surgery)", plan.Env, wantEnv)
	}
}

func TestPlanGHShimExecFallsThroughWithStderrNoteOnTokenFailure(t *testing.T) {
	dir := realpath(t, t.TempDir())
	writeCheckFile(t, dir, ".git", "gitdir: fake\n")
	writeCheckFile(t, dir, ".agents/loaf.json", `{"integrations":{"github":{"account":"levifig"}}}`+"\n")

	environ := []string{"PATH=/usr/bin"}
	plan, err := planGHShimExec([]string{"gh", "pr", "list"}, environ, dir, "/opt/loaf-binary",
		func() (string, error) { return "/opt/real/gh", nil },
		func(string, string) (string, error) { return "", errors.New("no oauth token found") },
	)
	if err != nil {
		t.Fatalf("planGHShimExec() error = %v", err)
	}
	if !strings.Contains(plan.StderrNote, `"levifig"`) {
		t.Fatalf("plan.StderrNote = %q, want it to name the account", plan.StderrNote)
	}
	if !reflect.DeepEqual(plan.Env, environ) {
		t.Fatalf("plan.Env = %v, want untouched %v on resolution failure", plan.Env, environ)
	}
	if plan.Path != "/opt/real/gh" {
		t.Fatalf("plan.Path = %q, want the real gh even on token failure", plan.Path)
	}
}

func TestPlanGHShimExecRefusesWhenRealGHIsSelf(t *testing.T) {
	self := filepath.Join(realpath(t, t.TempDir()), "loaf")
	writeExecutableFixture(t, self, "#!/bin/sh\n")

	_, err := planGHShimExec([]string{"gh"}, nil, "", self,
		func() (string, error) { return self, nil },
		nil,
	)
	if err == nil {
		t.Fatalf("planGHShimExec() error = nil, want recursion guard error")
	}
	if !strings.Contains(err.Error(), "recurse") {
		t.Fatalf("error = %v, want it to mention recursion", err)
	}
}

func TestPlanGHShimExecFallsThroughWhenCWDUnavailable(t *testing.T) {
	tokenCalled := false
	plan, err := planGHShimExec([]string{"gh", "pr", "list"}, []string{"HOME=/x"}, "", "/opt/loaf-binary",
		func() (string, error) { return "/opt/real/gh", nil },
		func(string, string) (string, error) { tokenCalled = true; return "unused", nil },
	)
	if err != nil {
		t.Fatalf("planGHShimExec() error = %v", err)
	}
	if tokenCalled {
		t.Fatalf("token resolver was called with an empty cwd")
	}
	if plan.Path != "/opt/real/gh" {
		t.Fatalf("plan.Path = %q, want /opt/real/gh", plan.Path)
	}
}

func TestPlanGHShimExecPropagatesRealGHResolutionFailure(t *testing.T) {
	_, err := planGHShimExec([]string{"gh"}, nil, "", "/opt/loaf-binary",
		func() (string, error) { return "", errors.New("no gh found") },
		nil,
	)
	if err == nil {
		t.Fatalf("planGHShimExec() error = nil, want the resolveRealGH failure surfaced")
	}
}

// --- setGHTokenEnv ---

func TestSetGHTokenEnvReplacesExistingAndPreservesOthers(t *testing.T) {
	in := []string{"PATH=/x", "GH_TOKEN=old", "HOME=/y"}
	out := setGHTokenEnv(in, "new")

	found := 0
	for _, kv := range out {
		if kv == "GH_TOKEN=new" {
			found++
		}
		if strings.HasPrefix(kv, "GH_TOKEN=") && kv != "GH_TOKEN=new" {
			t.Fatalf("stale GH_TOKEN entry survived: %v", out)
		}
	}
	if found != 1 {
		t.Fatalf("GH_TOKEN=new count = %d, want exactly 1 in %v", found, out)
	}
	for _, want := range []string{"PATH=/x", "HOME=/y"} {
		if !containsString(out, want) {
			t.Fatalf("out = %v, missing %q", out, want)
		}
	}
}

// --- findConfiguredGitHubAccountUpward ---

func TestFindConfiguredGitHubAccountUpwardWalksToGitRoot(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeCheckFile(t, root, ".git", "gitdir: fake\n")
	writeCheckFile(t, root, ".agents/loaf.json", `{"integrations":{"github":{"account":"levifig"}}}`+"\n")
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}

	account, err := findConfiguredGitHubAccountUpward(nested)
	if err != nil {
		t.Fatalf("findConfiguredGitHubAccountUpward() error = %v", err)
	}
	if account != "levifig" {
		t.Fatalf("account = %q, want levifig", account)
	}
}

func TestFindConfiguredGitHubAccountUpwardStopsAtGitBoundary(t *testing.T) {
	outer := realpath(t, t.TempDir())
	writeCheckFile(t, outer, ".agents/loaf.json", `{"integrations":{"github":{"account":"outer-account"}}}`+"\n")
	inner := filepath.Join(outer, "nested-repo")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	writeCheckFile(t, inner, ".git", "gitdir: fake\n")

	account, err := findConfiguredGitHubAccountUpward(inner)
	if err != nil {
		t.Fatalf("findConfiguredGitHubAccountUpward() error = %v", err)
	}
	if account != "" {
		t.Fatalf("account = %q, want empty (must not leak across a git repo boundary)", account)
	}
}

// --- gh version gate ---

func TestGhVersionAtLeast(t *testing.T) {
	cases := []struct {
		version string
		min     string
		want    bool
	}{
		{"2.96.0", "2.40.0", true},
		{"2.40.0", "2.40.0", true},
		{"2.39.9", "2.40.0", false},
		{"1.0.0", "2.40.0", false},
		{"garbage", "2.40.0", false},
	}
	for _, c := range cases {
		if got := ghVersionAtLeast(c.version, c.min); got != c.want {
			t.Errorf("ghVersionAtLeast(%q, %q) = %v, want %v", c.version, c.min, got, c.want)
		}
	}
}

func TestGhVersionParsesVersionOutput(t *testing.T) {
	stub := filepath.Join(realpath(t, t.TempDir()), "gh")
	writeExecutableFixture(t, stub, "#!/bin/sh\necho 'gh version 2.96.0 (2026-07-02)'\n")

	v, err := ghVersion(stub)
	if err != nil {
		t.Fatalf("ghVersion() error = %v", err)
	}
	if v != "2.96.0" {
		t.Fatalf("ghVersion() = %q, want 2.96.0", v)
	}
}

// --- user config read/write ---

func TestShimUserConfigRoundTrip(t *testing.T) {
	setupShimTestHome(t)
	path, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}

	if err := writeShimUserConfig(path, "/opt/real/gh", "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}
	snap, ok, err := readShimUserConfig()
	if err != nil {
		t.Fatalf("readShimUserConfig() error = %v", err)
	}
	if !ok || snap.Shims.GH == nil {
		t.Fatalf("readShimUserConfig() = %+v, ok=%v, want a gh entry", snap, ok)
	}
	if snap.Shims.GH.RealPath != "/opt/real/gh" || snap.Shims.GH.EnabledAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("gh entry = %+v, want real_path/enabled_at to round-trip", snap.Shims.GH)
	}

	removed, err := removeShimUserConfigEntry(path)
	if err != nil {
		t.Fatalf("removeShimUserConfigEntry() error = %v", err)
	}
	if !removed {
		t.Fatalf("removeShimUserConfigEntry() = false, want true")
	}
	snap2, _, err := readShimUserConfig()
	if err != nil {
		t.Fatalf("readShimUserConfig() after removal error = %v", err)
	}
	if snap2.Shims.GH != nil {
		t.Fatalf("gh entry still present after removal: %+v", snap2.Shims.GH)
	}

	removedAgain, err := removeShimUserConfigEntry(path)
	if err != nil {
		t.Fatalf("removeShimUserConfigEntry() second call error = %v", err)
	}
	if removedAgain {
		t.Fatalf("removeShimUserConfigEntry() second call = true, want false (idempotent)")
	}
}

func TestShimUserConfigPreservesUnrelatedKeys(t *testing.T) {
	setupShimTestHome(t)
	path, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	writeCheckFile(t, filepath.Dir(path), filepath.Base(path),
		`{"future_setting":{"x":1},"shims":{"other":"keep-me"}}`+"\n")

	if err := writeShimUserConfig(path, "/opt/real/gh", "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(body, []byte("future_setting")) {
		t.Fatalf("config = %s, want unrelated top-level key preserved", body)
	}
	if !bytes.Contains(body, []byte(`"other": "keep-me"`)) {
		t.Fatalf("config = %s, want unrelated shims key preserved", body)
	}
}

// --- diagnoseGHShim states ---

func TestDiagnoseGHShimAbsent(t *testing.T) {
	setupShimTestHome(t)
	diag, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag.State != ghShimAbsent {
		t.Fatalf("state = %q, want absent", diag.State)
	}
}

func TestDiagnoseGHShimBrokenSymlinkWhenConfigExistsButSymlinkMissing(t *testing.T) {
	setupShimTestHome(t)
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, "/opt/real/gh", "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	diag, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag.State != ghShimBrokenSymlink {
		t.Fatalf("state = %q, want broken-symlink", diag.State)
	}
}

func TestDiagnoseGHShimBrokenSymlinkWhenTargetMissing(t *testing.T) {
	home := setupShimTestHome(t)
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, "/opt/real/gh", "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}
	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		t.Fatalf("shimSymlinkPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.Symlink(filepath.Join(home, "nonexistent-loaf-binary"), symlinkPath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	diag, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag.State != ghShimBrokenSymlink {
		t.Fatalf("state = %q, want broken-symlink", diag.State)
	}
}

func TestDiagnoseGHShimRealGHMissing(t *testing.T) {
	home := setupShimTestHome(t)
	loafBin := filepath.Join(home, "loaf-bin")
	writeExecutableFixture(t, loafBin, "#!/bin/sh\n")
	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		t.Fatalf("shimSymlinkPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.Symlink(loafBin, symlinkPath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, filepath.Join(home, "nonexistent-real-gh"), "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	diag, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag.State != ghShimRealGHMissing {
		t.Fatalf("state = %q, want real-gh-missing", diag.State)
	}
}

func TestDiagnoseGHShimPathShadowedThenHealthy(t *testing.T) {
	home := setupShimTestHome(t)
	loafBin := filepath.Join(home, "loaf-bin")
	writeExecutableFixture(t, loafBin, "#!/bin/sh\n")
	realGH := filepath.Join(home, "real-gh")
	writeExecutableFixture(t, realGH, "#!/bin/sh\n")
	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		t.Fatalf("shimSymlinkPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.Symlink(loafBin, symlinkPath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, realGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}

	t.Setenv("PATH", "")
	diag, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag.State != ghShimPathShadowed {
		t.Fatalf("state = %q, want path-shadowed", diag.State)
	}

	t.Setenv("PATH", filepath.Dir(symlinkPath))
	diag2, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag2.State != ghShimHealthy {
		t.Fatalf("state = %q, want healthy", diag2.State)
	}
}

// --- doctor integration ---

func TestDoctorGHShimCheckAbsentSkips(t *testing.T) {
	setupShimTestHome(t)
	result := checkGHShim().Run(doctorContext{})
	if result.Status != doctorSkip {
		t.Fatalf("status = %v, message = %q, want skip", result.Status, result.Message)
	}
}

func TestDoctorGHShimCheckHealthyPasses(t *testing.T) {
	home := setupShimTestHome(t)
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	if resolved, everr := filepath.EvalSymlinks(self); everr == nil {
		self = resolved
	}
	realGH := filepath.Join(home, "real-gh")
	writeExecutableFixture(t, realGH, "#!/bin/sh\n")
	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		t.Fatalf("shimSymlinkPath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.Symlink(self, symlinkPath); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, realGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}
	t.Setenv("PATH", filepath.Dir(symlinkPath))

	result := checkGHShim().Run(doctorContext{})
	if result.Status != doctorPass {
		t.Fatalf("status = %v, message = %q, want pass", result.Status, result.Message)
	}
}

func TestDoctorGHShimCheckFixesBrokenSymlink(t *testing.T) {
	home := setupShimTestHome(t)
	realGH := filepath.Join(home, "real-gh")
	writeExecutableFixture(t, realGH, "#!/bin/sh\n")
	configPath, err := shimUserConfigPath()
	if err != nil {
		t.Fatalf("shimUserConfigPath() error = %v", err)
	}
	if err := writeShimUserConfig(configPath, realGH, "now"); err != nil {
		t.Fatalf("writeShimUserConfig() error = %v", err)
	}
	// no symlink created yet -> broken-symlink (config exists, symlink missing)

	check := checkGHShim()
	result := check.Run(doctorContext{})
	if result.Status != doctorFail || !result.Fixable {
		t.Fatalf("status = %v, fixable = %v, want a fixable failure", result.Status, result.Fixable)
	}

	fix := check.Fix(doctorContext{}, result)
	if !fix.Fixed {
		t.Fatalf("fix.Fixed = false, message = %q", fix.Message)
	}

	diag, err := diagnoseGHShim()
	if err != nil {
		t.Fatalf("diagnoseGHShim() error = %v", err)
	}
	if diag.State == ghShimBrokenSymlink {
		t.Fatalf("state still broken-symlink after fix")
	}
}

// --- CLI surface: loaf shim enable/disable/status ---

func TestRunnerShimStatusReportsAbsent(t *testing.T) {
	setupShimTestHome(t)
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "status"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "absent") {
		t.Fatalf("output = %q, want it to report absent", stdout.String())
	}
}

func TestRunnerShimEnableRefusesUnsupportedTarget(t *testing.T) {
	setupShimTestHome(t)
	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "enable", "npm"})
	if err == nil || !strings.Contains(err.Error(), `only "gh" is supported`) {
		t.Fatalf("error = %v, want an unsupported-shim refusal", err)
	}
}

func TestRunnerShimEnableRefusesNonInteractiveWithoutYes(t *testing.T) {
	home := setupShimTestHome(t)
	fakeGH := filepath.Join(home, "bin", "gh")
	writeExecutableFixture(t, fakeGH, "#!/bin/sh\necho 'gh version 2.96.0 (2026-07-02)'\n")
	t.Setenv("PATH", filepath.Dir(fakeGH))

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, Stdin: strings.NewReader(""), WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "enable", "gh"})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %v, want a refusal naming --yes", err)
	}
}

func TestRunnerShimEnableRefusesOldGHVersion(t *testing.T) {
	home := setupShimTestHome(t)
	fakeGH := filepath.Join(home, "bin", "gh")
	writeExecutableFixture(t, fakeGH, "#!/bin/sh\necho 'gh version 2.10.0 (2023-01-01)'\n")
	t.Setenv("PATH", filepath.Dir(fakeGH))

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "enable", "gh", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "2.40.0") {
		t.Fatalf("error = %v, want a refusal naming the minimum version", err)
	}
}

func TestRunnerShimEnableAndDisableRoundTrip(t *testing.T) {
	home := setupShimTestHome(t)
	fakeGH := filepath.Join(home, "bin", "gh")
	writeExecutableFixture(t, fakeGH, "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'gh version 2.96.0 (2026-07-02)'; fi\n")
	t.Setenv("PATH", filepath.Dir(fakeGH))
	t.Setenv("SHELL", "/bin/zsh")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "enable", "gh", "--yes"})
	if err != nil {
		t.Fatalf("enable error = %v, output=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Symlink created") {
		t.Fatalf("enable output = %q, want a symlink confirmation", stdout.String())
	}

	symlinkPath, err := shimSymlinkPath()
	if err != nil {
		t.Fatalf("shimSymlinkPath() error = %v", err)
	}
	if !isSymlinkForDoctor(symlinkPath) {
		t.Fatalf("symlink not created at %s", symlinkPath)
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "enable", "gh", "--yes"})
	if err != nil {
		t.Fatalf("second enable error = %v, want idempotent success, output=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "already enabled") {
		t.Fatalf("second enable output = %q, want an already-enabled message", stdout.String())
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "disable", "gh"})
	if err != nil {
		t.Fatalf("disable error = %v", err)
	}
	if pathExistsForDoctor(symlinkPath) {
		t.Fatalf("symlink still exists after disable")
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: realpath(t, t.TempDir())}.Run([]string{"shim", "disable", "gh"})
	if err != nil {
		t.Fatalf("second disable error = %v, want idempotent success", err)
	}
	if !strings.Contains(stdout.String(), "not enabled") {
		t.Fatalf("second disable output = %q, want a not-enabled message", stdout.String())
	}
}
