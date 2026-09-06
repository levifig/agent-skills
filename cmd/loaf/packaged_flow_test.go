package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Exercise the real packager and extracted CLI, not a hand-maintained model
// of the file list. A packaged rebuild must retain the source build's Flow.
func TestPackagedRebuildPreservesTrackerNativeFlow(t *testing.T) {
	repo := repoRoot(t)
	binary := sharedTestLoafBinary(t, repo)
	loafdev := buildLoafdev(t, repo)
	target, nativeName := hostReleaseTarget()

	source := t.TempDir()
	for _, dir := range []string{"content", "config", "vnext/content"} {
		if err := os.CopyFS(filepath.Join(source, dir), os.DirFS(filepath.Join(repo, dir))); err != nil {
			t.Fatal(err)
		}
	}
	writeFixtureFile(t, filepath.Join(source, "package.json"), readFixtureFile(t, filepath.Join(repo, "package.json")))
	// The fixture supplies an already-built binary at both the native path the
	// packager reads and bin/loaf, the entry point a distribution exposes.
	copyFixtureBinary(t, binary, filepath.Join(source, "bin/native", target, nativeName))
	copyFixtureBinary(t, binary, filepath.Join(source, "bin", "loaf"))
	writeFixtureFile(t, filepath.Join(source, "vnext/continuity/unshipped.go"), "package continuity\n")
	env := append(isolatedInstallEnv(t), "LOAF_DEV_LINK=0", "LOAF_RELEASE_TARGETS="+target)
	run := func(dir, command string, args ...string) []byte {
		t.Helper()
		cmd := exec.Command(command, args...)
		cmd.Dir, cmd.Env = dir, env
		var diagnostics bytes.Buffer
		cmd.Stderr = &diagnostics
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s %v: %v\nstdout:\n%s\nstderr:\n%s", command, args, err, output, diagnostics.String())
		}
		return output
	}
	run(source, filepath.Join(source, "bin/native", target, nativeName), "build", "--target", "codex")
	want := packagedTreeDigests(t, filepath.Join(source, "dist/codex/skills"))
	if _, ok := want["project-management/contract.json"]; !ok {
		t.Fatal("source build did not produce tracker-native Flow")
	}

	run(source, loafdev, "package")
	var pkg struct{ Version string }
	if err := json.Unmarshal([]byte(readFixtureFile(t, filepath.Join(source, "package.json"))), &pkg); err != nil {
		t.Fatal(err)
	}
	packageName := "loaf_" + pkg.Version + "_" + target
	archive := filepath.Join(source, "dist/release", packageName+".tar.gz")
	extracted := t.TempDir()
	run(source, "tar", "-xzf", archive, "-C", extracted)
	packaged := filepath.Join(extracted, packageName)
	if _, err := os.Stat(filepath.Join(packaged, "vnext/continuity")); !os.IsNotExist(err) {
		t.Fatalf("package must not ship dormant continuity sources: %v", err)
	}
	if _, err := os.Stat(filepath.Join(packaged, "bin", nativeName)); err != nil {
		t.Fatalf("package must ship the native binary at bin/%s: %v", nativeName, err)
	}
	run(packaged, filepath.Join(packaged, "bin", nativeName), "build", "--target", "codex")
	got := packagedTreeDigests(t, filepath.Join(packaged, "dist/codex/skills"))
	if !reflect.DeepEqual(got, want) {
		t.Fatal("packaged rebuild changed skill bytes or lost tracker-native Flow/provider/report templates")
	}
	if !reflect.DeepEqual(packagedTreeDigests(t, filepath.Join(packaged, "vnext/content")), packagedTreeDigests(t, filepath.Join(source, "vnext/content"))) {
		t.Fatal("package changed or omitted authored Flow content, agents, or templates")
	}
	if _, err := os.Stat(envValue(t, env, "LOAF_DB")); !os.IsNotExist(err) {
		t.Fatalf("packaging/build must not create a continuity database: %v", err)
	}
}

func TestReleasePackageRejectsMissingFlowSources(t *testing.T) {
	repo := repoRoot(t)
	loafdev := buildLoafdev(t, repo)
	source := t.TempDir()
	writeFixtureFile(t, filepath.Join(source, "package.json"), readFixtureFile(t, filepath.Join(repo, "package.json")))
	writeFixtureFile(t, filepath.Join(source, "bin/native/linux-x64/loaf"), "unused native fixture\n")
	cmd := exec.Command(loafdev, "package")
	cmd.Dir = source
	cmd.Env = append(isolatedInstallEnv(t), "LOAF_DEV_LINK=0", "LOAF_RELEASE_TARGETS=linux-x64")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "missing tracker-native Flow content") {
		t.Fatalf("incomplete source must be rejected before packaging: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(source, "dist/release")); !os.IsNotExist(err) {
		t.Fatalf("missing sources must not create release output: %v", err)
	}
}

// hostReleaseTarget names this machine's release target and binary name.
func hostReleaseTarget() (string, string) {
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	nativeName := "loaf"
	if runtime.GOOS == "windows" {
		nativeName += ".exe"
	}
	return platform + "-" + arch, nativeName
}

// buildLoafdev compiles the dev tool once for the test process so fixtures
// outside the module can run it as a plain executable.
func buildLoafdev(t *testing.T, repo string) string {
	t.Helper()
	loafdevOnce.Do(func() {
		dir, err := os.MkdirTemp("", "loaf-loafdev-")
		if err != nil {
			loafdevErr = err
			return
		}
		loafdevPath = filepath.Join(dir, "loafdev")
		if output, err := runCommand(repo, "go", "build", "-o", loafdevPath, "./cmd/loafdev"); err != nil {
			loafdevErr = fmt.Errorf("go build ./cmd/loafdev: %v\n%s", err, output)
		}
	})
	if loafdevErr != nil {
		t.Fatal(loafdevErr)
	}
	return loafdevPath
}

var (
	loafdevOnce sync.Once
	loafdevPath string
	loafdevErr  error
)

func packagedTreeDigests(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	digests := make(map[string][32]byte)
	err := fs.WalkDir(os.DirFS(root), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return err
		}
		digests[path] = sha256.Sum256(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return digests
}
