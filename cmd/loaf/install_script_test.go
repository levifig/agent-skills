package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The installer is exercised for real: a fake GitHub Releases server serves a
// fixture archive whose bin/loaf is a stub that records its arguments, and the
// script downloads, verifies, unpacks, links, and hands off to `loaf install`.

const installScriptVersion = "9.9.9"

func installScriptTarget(t *testing.T) string {
	t.Helper()
	var os, arch string
	switch runtime.GOOS {
	case "darwin", "linux":
		os = runtime.GOOS
	default:
		t.Skipf("install.sh targets darwin and linux; running on %s", runtime.GOOS)
	}
	switch runtime.GOARCH {
	case "arm64":
		arch = "arm64"
	case "amd64":
		arch = "x64"
	default:
		t.Skipf("install.sh has no archive for %s", runtime.GOARCH)
	}
	return os + "-" + arch
}

func requireInstallScriptTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"bash", "curl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required for the installer test: %v", tool, err)
		}
	}
	if _, err := exec.LookPath("shasum"); err != nil {
		if _, err := exec.LookPath("sha256sum"); err != nil {
			t.Skip("neither shasum nor sha256sum is available")
		}
	}
}

type fakeRelease struct {
	archiveName string
	archive     []byte
	checksums   string
}

func buildFakeRelease(t *testing.T, target string, corruptChecksum bool) fakeRelease {
	t.Helper()
	stub := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then echo \"loaf " + installScriptVersion + "\"; exit 0; fi\n" +
		"printf '%s\\n' \"$*\" >> \"${LOAF_STUB_LOG:?}\"\n"
	root := "loaf_" + installScriptVersion + "_" + target
	var buffer strings.Builder
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	add := func(name string, body string, mode int64) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	add(root+"/bin/loaf", stub, 0o755)
	add(root+"/package.json", `{"name":"loaf","version":"`+installScriptVersion+`"}`+"\n", 0o644)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	archive := []byte(buffer.String())
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	if corruptChecksum {
		digest = strings.Repeat("0", 64)
	}
	name := root + ".tar.gz"
	return fakeRelease{archiveName: name, archive: archive, checksums: digest + "  " + name + "\n"}
}

func serveFakeRelease(t *testing.T, release fakeRelease) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	checksums := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(release.checksums)) }
	mux.HandleFunc("/latest/download/checksums.txt", checksums)
	mux.HandleFunc("/download/v"+installScriptVersion+"/checksums.txt", checksums)
	mux.HandleFunc("/download/v"+installScriptVersion+"/"+release.archiveName, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(release.archive) })
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

type installScriptRun struct {
	home     string
	loafHome string
	binDir   string
	stubLog  string
	env      []string
}

func newInstallScriptRun(t *testing.T, server *httptest.Server) installScriptRun {
	t.Helper()
	home := realpath(t, t.TempDir())
	run := installScriptRun{
		home:     home,
		loafHome: filepath.Join(home, ".local", "share", "loaf"),
		binDir:   filepath.Join(home, ".local", "bin"),
		stubLog:  filepath.Join(home, "stub.log"),
	}
	run.env = envWith(
		"HOME="+home,
		"XDG_DATA_HOME=",
		"LOAF_HOME="+run.loafHome,
		"LOAF_BIN_DIR="+run.binDir,
		"LOAF_RELEASE_BASE_URL="+server.URL,
		"LOAF_STUB_LOG="+run.stubLog,
		"LOAF_VERSION=",
		"TMPDIR="+realpath(t, t.TempDir()),
	)
	return run
}

func (run installScriptRun) exec(t *testing.T, repo string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join(repo, "install.sh")}, args...)...)
	cmd.Dir = run.home
	cmd.Env = run.env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (run installScriptRun) stubCalls(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(run.stubLog)
	if err != nil {
		return ""
	}
	return string(body)
}

func TestInstallScriptInstallsLinksAndHandsOffToLoafInstall(t *testing.T) {
	requireInstallScriptTools(t)
	target := installScriptTarget(t)
	repo := repoRoot(t)
	server := serveFakeRelease(t, buildFakeRelease(t, target, false))
	run := newInstallScriptRun(t, server)

	output, err := run.exec(t, repo)
	if err != nil {
		t.Fatalf("install.sh error = %v\n%s", err, output)
	}
	releaseDir := filepath.Join(run.loafHome, "releases", installScriptVersion)
	if info, err := os.Stat(filepath.Join(releaseDir, "bin", "loaf")); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("unpacked binary missing or not executable: info=%v err=%v\n%s", info, err, output)
	}
	if got, err := os.Readlink(filepath.Join(run.loafHome, "current")); err != nil || got != releaseDir {
		t.Fatalf("current -> %q (err %v), want %q", got, err, releaseDir)
	}
	if got, err := os.Readlink(filepath.Join(run.binDir, "loaf")); err != nil || got != filepath.Join(run.loafHome, "current", "bin", "loaf") {
		t.Fatalf("bin link -> %q (err %v), want the current binary", got, err)
	}
	if calls := run.stubCalls(t); strings.TrimSpace(calls) != "install" {
		t.Fatalf("loaf was invoked with %q, want a single `install`", calls)
	}
	for _, want := range []string{"verified loaf_" + installScriptVersion + "_" + target + ".tar.gz", "installed loaf " + installScriptVersion, "linked " + filepath.Join(run.binDir, "loaf")} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want %q", output, want)
		}
	}

	// A second run with the version pinned re-links and upgrades instead of
	// onboarding again.
	output, err = run.exec(t, repo, "--version", "v"+installScriptVersion, "--", "--to", "cursor")
	if err != nil {
		t.Fatalf("second install.sh error = %v\n%s", err, output)
	}
	if calls := run.stubCalls(t); !strings.HasSuffix(strings.TrimSpace(calls), "upgrade --to cursor") {
		t.Fatalf("loaf was invoked with %q, want an upgrade with the passthrough args on the second run", calls)
	}
	if !strings.Contains(output, "updated "+filepath.Join(run.binDir, "loaf")) {
		t.Fatalf("second output = %q, want the link updated in place", output)
	}
}

func TestInstallScriptRefusesACorruptArchiveAndLeavesNothingBehind(t *testing.T) {
	requireInstallScriptTools(t)
	target := installScriptTarget(t)
	repo := repoRoot(t)
	server := serveFakeRelease(t, buildFakeRelease(t, target, true))
	run := newInstallScriptRun(t, server)

	output, err := run.exec(t, repo)
	if err == nil || !strings.Contains(output, "checksum mismatch") {
		t.Fatalf("install.sh err=%v output=%q, want a checksum refusal", err, output)
	}
	if _, err := os.Stat(filepath.Join(run.loafHome, "releases")); !os.IsNotExist(err) {
		t.Fatalf("releases directory exists after a refused download: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(run.binDir, "loaf")); !os.IsNotExist(err) {
		t.Fatalf("bin link exists after a refused download: %v", err)
	}
	if run.stubCalls(t) != "" {
		t.Fatal("loaf install ran despite the refused download")
	}
}

func TestInstallScriptNeverReplacesAForeignLoafOnPath(t *testing.T) {
	requireInstallScriptTools(t)
	target := installScriptTarget(t)
	repo := repoRoot(t)
	server := serveFakeRelease(t, buildFakeRelease(t, target, false))
	run := newInstallScriptRun(t, server)
	if err := os.MkdirAll(run.binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(run.binDir, "loaf")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\necho someone else\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	output, err := run.exec(t, repo, "--no-install")
	if err != nil {
		t.Fatalf("install.sh error = %v\n%s", err, output)
	}
	if body, _ := os.ReadFile(foreign); string(body) != "#!/bin/sh\necho someone else\n" {
		t.Fatalf("foreign bin/loaf was replaced: %q", body)
	}
	if !strings.Contains(output, "not managed by this installer") || !strings.Contains(output, "skipped harness install") {
		t.Fatalf("output = %q, want the foreign-link warning and the skipped install note", output)
	}
	if run.stubCalls(t) != "" {
		t.Fatal("loaf install ran despite --no-install")
	}

	output, err = run.exec(t, repo, "--uninstall")
	if err != nil {
		t.Fatalf("install.sh --uninstall error = %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(run.loafHome, "releases")); !os.IsNotExist(err) {
		t.Fatalf("releases directory survived --uninstall: %v", err)
	}
	if body, _ := os.ReadFile(foreign); string(body) != "#!/bin/sh\necho someone else\n" {
		t.Fatalf("--uninstall removed a foreign bin/loaf: %q", body)
	}
}
