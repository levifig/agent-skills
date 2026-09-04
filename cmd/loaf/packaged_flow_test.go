package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real packagers and extracted CLI, not a hand-maintained model
// of their file lists. Packaged rebuilds must retain the source build's Flow.
func TestPackagedRebuildPreservesTrackerNativeFlow(t *testing.T) {
	repo := repoRoot(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	npmName := "npm"
	if runtime.GOOS == "windows" {
		npmName = "npm.cmd"
	}
	npm, err := exec.LookPath(npmName)
	if err != nil {
		t.Fatal(err)
	}
	binary := sharedTestLoafBinary(t, repo)
	platform := runtime.GOOS
	if platform == "windows" {
		platform = "win32"
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	target := platform + "-" + arch
	nativeName := "loaf"
	if runtime.GOOS == "windows" {
		nativeName += ".exe"
	}

	for _, channel := range []string{"npm", "release"} {
		t.Run(channel, func(t *testing.T) {
			source := t.TempDir()
			for _, dir := range []string{"content", "config", "vnext/content"} {
				if err := os.CopyFS(filepath.Join(source, dir), os.DirFS(filepath.Join(repo, dir))); err != nil {
					t.Fatal(err)
				}
			}
			for _, file := range []string{"package.json", "bin/package.json", "cli/scripts/package-release.mjs"} {
				writeFixtureFile(t, filepath.Join(source, file), readFixtureFile(t, filepath.Join(repo, file)))
			}
			copyFixtureBinary(t, filepath.Join(repo, "bin/loaf"), filepath.Join(source, "bin/loaf"))
			copyFixtureBinary(t, binary, filepath.Join(source, "bin/native", target, nativeName))
			writeFixtureFile(t, filepath.Join(source, "vnext/continuity/unshipped.go"), "package continuity\n")
			env := isolatedInstallEnv(t)
			env = append(env, "LOAF_DEV_LINK=0", "LOAF_RELEASE_TARGETS="+target)
			// npm's executable uses env node; preserve only the resolved Node
			// directory alongside the existing fixture's system-tool PATH.
			for i, value := range env {
				if path, ok := strings.CutPrefix(value, "PATH="); ok {
					env[i] = "PATH=" + filepath.Dir(node) + string(os.PathListSeparator) + path
				}
			}
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
			// npm may emit update notices on stderr even with --json.
			if got := run(source, node, "-e", `process.stdout.write("[]"); process.stderr.write("npm notice fixture\n")`); string(got) != "[]" {
				t.Fatalf("command diagnostics contaminated stdout: %q", got)
			}
			run(source, filepath.Join(source, "bin/native", target, nativeName), "build", "--target", "codex")
			want := packagedTreeDigests(t, filepath.Join(source, "dist/codex/skills"))
			if _, ok := want["project-management/contract.json"]; !ok {
				t.Fatal("source build did not produce tracker-native Flow")
			}

			var archive, packageName string
			if channel == "npm" {
				output := run(source, npm, "pack", "--ignore-scripts", "--offline", "--json",
					"--cache", filepath.Join(t.TempDir(), "npm-cache"), "--userconfig", os.DevNull)
				var packs []struct{ Filename string }
				if err := json.Unmarshal(output, &packs); err != nil || len(packs) != 1 {
					t.Fatalf("decode npm pack: %v\n%s", err, output)
				}
				archive, packageName = filepath.Join(source, packs[0].Filename), "package"
			} else {
				run(source, node, filepath.Join(source, "cli/scripts/package-release.mjs"))
				var pkg struct{ Version string }
				if err := json.Unmarshal([]byte(readFixtureFile(t, filepath.Join(source, "package.json"))), &pkg); err != nil {
					t.Fatal(err)
				}
				packageName = "loaf_" + pkg.Version + "_" + target
				archive = filepath.Join(source, "dist/release", packageName+".tar.gz")
			}
			extracted := t.TempDir()
			run(source, "tar", "-xzf", archive, "-C", extracted)
			packaged := filepath.Join(extracted, packageName)
			if _, err := os.Stat(filepath.Join(packaged, "vnext/continuity")); !os.IsNotExist(err) {
				t.Fatalf("package must not ship dormant continuity sources: %v", err)
			}
			if channel == "npm" {
				run(packaged, node, filepath.Join(packaged, "bin/loaf"), "build", "--target", "codex")
			} else {
				run(packaged, filepath.Join(packaged, "bin", nativeName), "build", "--target", "codex")
			}
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
		})
	}
}

func TestReleasePackageRejectsMissingFlowSources(t *testing.T) {
	repo := repoRoot(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	writeFixtureFile(t, filepath.Join(source, "package.json"), readFixtureFile(t, filepath.Join(repo, "package.json")))
	writeFixtureFile(t, filepath.Join(source, "bin/native/linux-x64/loaf"), "unused native fixture\n")
	cmd := exec.Command(node, filepath.Join(repo, "cli/scripts/package-release.mjs"))
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
