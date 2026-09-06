package devtool

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type recordingRunner struct {
	goos, goarch string
	calls        []string
}

func (r *recordingRunner) Run(dir string, env Env, name string, args ...string) error {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	return nil
}

func (r *recordingRunner) Output(dir string, env Env, name string, args ...string) (string, error) {
	if name == "go" && len(args) == 3 && args[0] == "env" {
		return r.goos + "\n" + r.goarch + "\n", nil
	}
	return "", nil
}

func fixtureRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/loaf\n\ngo 1.25\n\ntoolchain go1.27.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(body)
}

func TestGoBuildArgsCarryReleaseMetadataOnlyWhenSet(t *testing.T) {
	dev := GoBuildArgs("out", Env{})
	if strings.Join(dev, " ") != "build -trimpath -buildvcs=true -ldflags -buildid= -o out ./cmd/loaf" {
		t.Fatalf("dev args = %q", dev)
	}
	release := GoLdflags(Env{"LOAF_BUILD_COMMIT": "abc1234", "LOAF_BUILD_DATE": "2026-09-06T00:00:00Z"})
	if release != "-buildid= -X main.buildCommit=abc1234 -X main.buildDate=2026-09-06T00:00:00Z" {
		t.Fatalf("release ldflags = %q", release)
	}
	if !IsReleaseBuild(Env{"LOAF_BUILD_COMMIT": "abc1234"}) || IsReleaseBuild(Env{}) {
		t.Fatal("IsReleaseBuild misclassified")
	}
}

func TestBuildGoPublishesEveryTargetOnlyWhenAllCompile(t *testing.T) {
	root := fixtureRoot(t)
	linux := filepath.Join(root, "bin", "native", "linux-x64", "loaf")
	windows := filepath.Join(root, "bin", "native", "win32-x64", "loaf.exe")
	writeFixture(t, linux, "old-linux")
	writeFixture(t, windows, "old-win")
	host := Target{"linux-x64", "linux", "amd64"}
	env := Env{"LOAF_BUILD_TARGETS": "linux-x64,win32-x64", "LOAF_DEV_LINK": "0", "HOME": root}

	count := 0
	err := BuildGo(BuildGoOptions{
		RootDir: root, Env: env, Runner: &recordingRunner{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HostTarget: &host,
		Builder: func(dest string, target Target, env Env) error {
			count++
			writeFixture(t, dest, "new-"+target.RuntimeID)
			if count == 2 {
				return os.ErrPermission
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("second target failed but BuildGo returned nil")
	}
	if readFixture(t, linux) != "old-linux" || readFixture(t, windows) != "old-win" {
		t.Fatal("a failed target let a partial set through")
	}
	if _, err := os.Stat(filepath.Join(root, "bin", "native", ".staging")); !os.IsNotExist(err) {
		t.Fatal("staging directory was left behind")
	}

	var stdout bytes.Buffer
	linked := 0
	err = BuildGo(BuildGoOptions{
		RootDir: root, Env: Env{"LOAF_BUILD_TARGETS": "linux-x64,win32-x64", "HOME": root}, Runner: &recordingRunner{}, Stdout: &stdout, Stderr: &bytes.Buffer{}, HostTarget: &host,
		Builder: func(dest string, target Target, env Env) error {
			writeFixture(t, dest, "new-"+target.RuntimeID)
			return nil
		},
		Linker: func(launcher string) DevLinkResult {
			linked++
			return DevLinkResult{Status: DevLinkLinked, Link: "/tmp/loaf"}
		},
	})
	if err != nil {
		t.Fatalf("BuildGo error = %v", err)
	}
	if readFixture(t, linux) != "new-linux-x64" || readFixture(t, windows) != "new-win32-x64" {
		t.Fatal("published binaries do not match the build")
	}
	if got := readFixture(t, filepath.Join(root, "bin", "loaf")); got != "new-linux-x64" {
		t.Fatalf("bin/loaf = %q, want the host platform binary", got)
	}
	if runtime.GOOS != "windows" && linked != 1 {
		t.Fatalf("dev link refreshed %d times, want 1", linked)
	}
	if !strings.Contains(stdout.String(), "Published Loaf entry point") {
		t.Fatalf("stdout = %q, want the entry point line", stdout.String())
	}
}

func TestBuildGoSkipsTheDevLinkForReleaseBuildsAndOptOuts(t *testing.T) {
	root := fixtureRoot(t)
	host := Target{"linux-x64", "linux", "amd64"}
	for _, env := range []Env{
		{"LOAF_BUILD_TARGETS": "linux-x64", "LOAF_BUILD_COMMIT": "abc1234", "HOME": root},
		{"LOAF_BUILD_TARGETS": "linux-x64", "LOAF_DEV_LINK": "0", "HOME": root},
	} {
		linked := 0
		err := BuildGo(BuildGoOptions{
			RootDir: root, Env: env, Runner: &recordingRunner{}, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, HostTarget: &host,
			Builder: func(dest string, target Target, env Env) error { writeFixture(t, dest, "bin"); return nil },
			Linker:  func(string) DevLinkResult { linked++; return DevLinkResult{Status: DevLinkLinked} },
		})
		if err != nil || linked != 0 {
			t.Fatalf("env %v: err=%v linked=%d, want no link", env, err, linked)
		}
	}
}

func TestBuildGoDryRunTouchesNothing(t *testing.T) {
	root := fixtureRoot(t)
	var stdout bytes.Buffer
	err := BuildGo(BuildGoOptions{
		RootDir: root, Env: Env{"LOAF_BUILD_TARGETS": "linux-x64,win32-x64,linux-x64", "LOAF_NATIVE_ARTIFACT_DRY_RUN": "1"}, Runner: &recordingRunner{}, Stdout: &stdout, Stderr: &bytes.Buffer{},
		Builder: func(string, Target, Env) error { t.Fatal("builder ran during dry run"); return nil },
		Linker:  func(string) DevLinkResult { t.Fatal("linker ran during dry run"); return DevLinkResult{} },
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(stdout.String(), "DRY RUN: would build linux-x64") != 1 || !strings.Contains(stdout.String(), "win32-x64/loaf.exe") {
		t.Fatalf("dry run output = %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(root, "bin")); !os.IsNotExist(err) {
		t.Fatal("dry run created bin/")
	}
	if _, err := ParseTargetList("solaris-x64"); err == nil || !strings.Contains(err.Error(), "unsupported LOAF_BUILD_TARGETS entry") || !strings.Contains(err.Error(), "linux-x64") {
		t.Fatalf("unsupported target error = %v", err)
	}
}

func TestRefreshDevBuildLinkOwnsOnlyItsOwnPointer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("dev links are skipped on Windows")
	}
	home := t.TempDir()
	launcher := func(name string) string {
		path := filepath.Join(home, name, "bin", "loaf")
		writeFixture(t, filepath.Join(home, name, "package.json"), `{"name":"loaf"}`)
		writeFixture(t, path, "#!/bin/sh\n")
		return path
	}
	link := filepath.Join(home, ".local", "bin", "loaf")
	pointer := filepath.Join(home, ".local", "share", "loaf", "current-dev-launcher")

	first, second := launcher("first"), launcher("second")
	if got := RefreshDevBuildLink(first, DevLinkOptions{Home: home}); got.Status != DevLinkLinked {
		t.Fatalf("first link = %#v", got)
	}
	if got := RefreshDevBuildLink(second, DevLinkOptions{Home: home}); got.Status != DevLinkLinked {
		t.Fatalf("second link = %#v", got)
	}
	if target, _ := os.Readlink(link); target != pointer {
		t.Fatalf("public link -> %q, want the pointer", target)
	}
	if target, _ := os.Readlink(pointer); target != second {
		t.Fatalf("pointer -> %q, want the last build", target)
	}

	// A real file, an unrelated symlink, and a legacy checkout link are never displaced.
	for name, setup := range map[string]func(){
		"real file":         func() { os.Remove(link); os.WriteFile(link, []byte("operator-owned\n"), 0o755) },
		"unrelated symlink": func() { os.Remove(link); os.Symlink(filepath.Join(home, "other", "loaf"), link) },
		"legacy checkout":   func() { os.Remove(link); os.Symlink(first, link) },
		"racing real file":  func() { os.Remove(link) },
	} {
		setup()
		var warnings []string
		options := DevLinkOptions{Home: home, Warn: func(m string) { warnings = append(warnings, m) }}
		if name == "racing real file" {
			options.BeforeClaimPublic = func() { os.WriteFile(link, []byte("operator-owned\n"), 0o755) }
		}
		got := RefreshDevBuildLink(second, options)
		if got.Status != DevLinkConflict || len(warnings) != 1 || !strings.HasPrefix(warnings[0], "WARN:") {
			t.Fatalf("%s: result=%#v warnings=%q, want one conflict warning", name, got, warnings)
		}
		if name == "legacy checkout" && !strings.Contains(warnings[0], "already points at a Loaf checkout") {
			t.Fatalf("legacy warning = %q", warnings[0])
		}
	}
	os.Remove(link)
	got := RefreshDevBuildLink(second, DevLinkOptions{Home: home, Warn: func(string) {}, Symlink: func(target, dest string) error {
		if dest == link {
			return os.ErrPermission
		}
		return os.Symlink(target, dest)
	}})
	if got.Status != DevLinkFailed {
		t.Fatalf("permission failure result = %#v, want failed without panic", got)
	}
}

func TestClassifyReleaseTag(t *testing.T) {
	for _, tc := range []struct{ tag, status string }{
		{"v0.3+gabcdef0", "invalid"}, {"v0.3.1", "release"}, {"v0.3.1+gabcdef0", "dev"}, {"v0.3.1-alpha.1", "release"},
		{"v1.0.0", "release"}, {"v0.3.1+gabc", "release"}, {"v0.3.1+foo", "release"}, {"v0.2.1786022455", "dev"},
		{"v0.2.20+gabcdef0", "dev"}, {"v0.2.20+build.9.gabc1234", "dev"}, {"v0.2.1000000000", "dev"}, {"v0.2.999999999", "release"},
		{"v0.3.1+gABCDEF0", "release"}, {"v0.3.1+", "invalid"}, {"v02.1.0", "invalid"}, {"v0.3", "invalid"}, {"v", "invalid"},
		{"", "invalid"}, {"0.3.1", "invalid"}, {"v0.3.1+gabcdef0.extra", "dev"},
	} {
		if got := ClassifyReleaseTag(tc.tag); got.Status != tc.status {
			t.Fatalf("ClassifyReleaseTag(%q) = %q, want %q", tc.tag, got.Status, tc.status)
		}
	}
	var stdout, stderr bytes.Buffer
	if err := WriteTagClassification(&stdout, &stderr, "v0.3+gabcdef0"); err == nil || strings.Contains(stdout.String(), "dev=true") {
		t.Fatalf("malformed tag err=%v stdout=%q, want failure without a dev skip", err, stdout.String())
	}
	stdout.Reset()
	if err := WriteTagClassification(&stdout, &stderr, "v0.3.1+gabcdef0"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"tag=v0.3.1+gabcdef0\n", "ref=refs/tags/v0.3.1+gabcdef0\n", "dev=true\n"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if !strings.Contains(stderr.String(), "carries a dev build identity") {
		t.Fatalf("stderr = %q, want the guardrail notice", stderr.String())
	}
}

func TestUpdateHomebrewFormulaGeneratesAuditSafeURLs(t *testing.T) {
	dir := t.TempDir()
	version := "0.2.4"
	sums := map[string]string{"darwin-arm64": strings.Repeat("a", 64), "darwin-x64": strings.Repeat("b", 64), "linux-arm64": strings.Repeat("c", 64), "linux-x64": strings.Repeat("d", 64)}
	var lines []string
	for _, target := range []string{"darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64"} {
		lines = append(lines, sums[target]+"  loaf_"+version+"_"+target+".tar.gz")
	}
	checksums := filepath.Join(dir, "checksums.txt")
	writeFixture(t, checksums, strings.Join(lines, "\n")+"\n")
	formula := filepath.Join(dir, "loaf.rb")
	if err := UpdateHomebrewFormula(HomebrewOptions{FormulaPath: formula, ChecksumsPath: checksums, Version: version}); err != nil {
		t.Fatal(err)
	}
	body := readFixture(t, formula)
	for _, want := range []string{
		`version "0.2.4"`,
		`url "https://github.com/levifig/loaf/releases/download/v0.2.4/loaf_0.2.4_darwin-arm64.tar.gz"`,
		`sha256 "` + sums["linux-x64"] + `"`,
		`libexec.install "bin", "package.json", "config", "content", "vnext", "dist", "plugins", ".claude-plugin"`,
		`bin.write_exec_script libexec/"bin/loaf"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("formula missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "${") || strings.Contains(body, "#{version}/") {
		t.Fatalf("formula must interpolate nothing at install time:\n%s", body)
	}
	writeFixture(t, checksums, lines[0]+"\n")
	if err := UpdateHomebrewFormula(HomebrewOptions{FormulaPath: formula, ChecksumsPath: checksums, Version: version}); err == nil || !strings.Contains(err.Error(), "missing darwin-x64") {
		t.Fatalf("incomplete checksums error = %v", err)
	}
	writeFixture(t, checksums, sums["linux-x64"]+"  loaf_9.9.9_linux-x64.tar.gz\n")
	if err := UpdateHomebrewFormula(HomebrewOptions{FormulaPath: formula, ChecksumsPath: checksums, Version: version}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("version mismatch error = %v", err)
	}
}

func TestReleaseDryRunReportsTargets(t *testing.T) {
	var stdout bytes.Buffer
	if err := Release(BuildOptions{RootDir: t.TempDir(), Env: Env{"LOAF_RELEASE_DRY_RUN": "1"}, Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"DRY RUN: would run loafdev build for native release targets: darwin-arm64,darwin-x64,linux-arm64,linux-x64,win32-arm64,win32-x64",
		"DRY RUN: LOAF_VERIFY_TARGETS=darwin-arm64,darwin-x64,linux-arm64,linux-x64,win32-arm64,win32-x64",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("dry run = %q, want %q", stdout.String(), want)
		}
	}
	stdout.Reset()
	if err := Release(BuildOptions{RootDir: t.TempDir(), Env: Env{"LOAF_RELEASE_DRY_RUN": "1", "LOAF_RELEASE_TARGETS": "linux-x64,win32-x64"}, Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "release targets: linux-x64,win32-x64") || !strings.Contains(stdout.String(), "LOAF_VERIFY_TARGETS=linux-x64,win32-x64") {
		t.Fatalf("override dry run = %q", stdout.String())
	}
}

func TestPackageWritesArchivesAndChecksums(t *testing.T) {
	root := fixtureRoot(t)
	writeFixture(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"1.2.3"}`)
	writeFixture(t, filepath.Join(root, "vnext", "content", "skills", "linear", "SKILL.md"), "# Linear\n")
	writeFixture(t, filepath.Join(root, "bin", "native", "linux-x64", "loaf"), "binary")
	writeFixture(t, filepath.Join(root, "dist", "codex", "skills", "a.md"), "a")
	writeFixture(t, filepath.Join(root, "dist", "release", "stale.txt"), "never packaged")
	writeFixture(t, filepath.Join(root, ".claude-plugin", "marketplace.json"), `{"name":"levifig-loaf"}`)
	writeFixture(t, filepath.Join(root, "content", ".DS_Store"), "junk")
	writeFixture(t, filepath.Join(root, "content", "SETUP.md"), "# Setup\n")

	var stdout bytes.Buffer
	if err := Package(PackageOptions{RootDir: root, Env: Env{"LOAF_RELEASE_TARGETS": "linux-x64"}, Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "dist", "release", "loaf_1.2.3_linux-x64.tar.gz")
	entries := tarEntries(t, archive)
	for _, want := range []string{"loaf_1.2.3_linux-x64/bin/loaf", "loaf_1.2.3_linux-x64/package.json", "loaf_1.2.3_linux-x64/vnext/content/skills/linear/SKILL.md", "loaf_1.2.3_linux-x64/dist/codex/skills/a.md", "loaf_1.2.3_linux-x64/.claude-plugin/marketplace.json", "loaf_1.2.3_linux-x64/content/SETUP.md"} {
		if !entries[want] {
			t.Fatalf("archive missing %s; entries=%v", want, entries)
		}
	}
	for _, unwanted := range []string{"loaf_1.2.3_linux-x64/dist/release/stale.txt", "loaf_1.2.3_linux-x64/content/.DS_Store"} {
		if entries[unwanted] {
			t.Fatalf("archive must not contain %s", unwanted)
		}
	}
	checksums := readFixture(t, filepath.Join(root, "dist", "release", "checksums.txt"))
	if !strings.HasSuffix(strings.TrimSpace(checksums), "  loaf_1.2.3_linux-x64.tar.gz") || len(strings.Fields(checksums)[0]) != 64 {
		t.Fatalf("checksums = %q", checksums)
	}
	if err := Package(PackageOptions{RootDir: root, Env: Env{"LOAF_RELEASE_TARGETS": "win32-x64"}, Stdout: &stdout}); err == nil || !strings.Contains(err.Error(), "missing native binary for win32-x64") {
		t.Fatalf("missing binary error = %v", err)
	}
	os.RemoveAll(filepath.Join(root, "vnext"))
	if err := Package(PackageOptions{RootDir: root, Env: Env{"LOAF_RELEASE_TARGETS": "linux-x64"}, Stdout: &stdout}); err == nil || !strings.Contains(err.Error(), "missing tracker-native Flow content") {
		t.Fatalf("missing Flow content error = %v", err)
	}
}

func TestVerifyArtifactsRequiresTheShimAndForbidsAPluginRuntime(t *testing.T) {
	root := fixtureRoot(t)
	shim := "#!/bin/sh\nexec loaf \"$@\"\n"
	writeFixture(t, filepath.Join(root, "internal", "cli", "claude_plugin_shim.sh"), shim)
	writeFixture(t, filepath.Join(root, "bin", "loaf"), "binary")
	writeFixture(t, filepath.Join(root, "bin", "native", "linux-x64", "loaf"), "binary")
	writeFixture(t, filepath.Join(root, "plugins", "loaf", "bin", "loaf"), shim)
	env := Env{"LOAF_VERIFY_TARGETS": "linux-x64"}
	var stdout bytes.Buffer
	if err := VerifyArtifacts(VerifyOptions{RootDir: root, Env: env, Runner: &recordingRunner{}, Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, "plugins", "loaf", "bin", "native", "linux-x64", "loaf"), "runtime")
	if err := VerifyArtifacts(VerifyOptions{RootDir: root, Env: env, Runner: &recordingRunner{}, Stdout: &stdout}); err == nil || !strings.Contains(err.Error(), "must not exist") {
		t.Fatalf("plugin runtime error = %v", err)
	}
	os.RemoveAll(filepath.Join(root, "plugins", "loaf", "bin", "native"))
	writeFixture(t, filepath.Join(root, "plugins", "loaf", "bin", "loaf"), shim+"# edited\n")
	if err := VerifyArtifacts(VerifyOptions{RootDir: root, Env: env, Runner: &recordingRunner{}, Stdout: &stdout}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale shim error = %v", err)
	}
	stdout.Reset()
	if err := VerifyArtifacts(VerifyOptions{RootDir: root, Env: Env{"LOAF_VERIFY_TARGETS": "darwin-arm64,win32-x64", "LOAF_NATIVE_ARTIFACT_DRY_RUN": "1"}, Runner: &recordingRunner{}, Stdout: &stdout}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bin/native/darwin-arm64/loaf", "bin/native/win32-x64/loaf.exe", "plugins/loaf/bin/loaf"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("dry run = %q, want %q", stdout.String(), want)
		}
	}
}
