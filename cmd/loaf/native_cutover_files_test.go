package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestNativeCutoverPackageAndSourceGuards(t *testing.T) {
	root := repoRoot(t)
	manifest := readPackageManifest(t, root)

	// package.json is the distribution manifest and nothing more: no npm
	// scripts, bin, files, engines, or dependencies. The build, release, and
	// packaging tooling is Go under internal/devtool, driven by the Makefile.
	if manifest.Name != "loaf" || manifest.Version == "" {
		t.Fatalf("package.json = %#v, want name loaf with a version", manifest)
	}
	for field, present := range map[string]bool{
		"scripts":         len(manifest.Scripts) > 0,
		"bin":             manifest.Bin != nil,
		"files":           len(manifest.Files) > 0,
		"engines":         manifest.Engines != nil,
		"dependencies":    len(manifest.Dependencies) > 0,
		"devDependencies": len(manifest.DevDependencies) > 0,
		"exports":         len(manifest.Exports) > 0,
	} {
		if present {
			t.Fatalf("package.json still carries the npm field %q; Loaf has no npm surface", field)
		}
	}
	for _, rel := range []string{"Makefile", "cmd/loafdev/main.go", "internal/devtool/buildgo.go", "internal/devtool/pack.go"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("%s missing: the Go build tooling replaced the npm scripts", rel)
		}
	}
	if tsFiles := findTypeScriptFiles(t, filepath.Join(root, "cli")); len(tsFiles) > 0 {
		t.Fatalf("cli/ still contains TypeScript sources after native cutover: %v", tsFiles)
	}

	missing := []string{
		"cli/runtime/loaf-launcher.cjs",
		"cli/scripts/build-go.mjs",
		"cli/scripts/build-release.mjs",
		"cli/scripts/package-release.mjs",
		"cli/scripts/verify-go-artifacts.mjs",
		"cli/scripts/classify-release-tag.mjs",
		"cli/scripts/dev-build-link.mjs",
		"cli/scripts/go-build-flags.mjs",
		"cli/scripts/update-homebrew-formula.mjs",
		"package-lock.json",
		"tsconfig.json",
		"tsup.config.ts",
		"vitest.config.ts",
		"cli/build-content.ts",
		"cli/index.ts",
		"cli/scripts/generate-cli-ref.ts",
		"cli/lib/cli-reference-generator.ts",
		"cli/lib/prompts.ts",
		"cli/lib/prompts.test.ts",
		"cli/lib/version.ts",
		"cli/types/gray-matter.d.ts",
		"cli/commands/build.ts",
		"cli/commands/check.ts",
		"cli/commands/doctor.ts",
		"cli/commands/housekeeping.ts",
		"cli/commands/init.ts",
		"cli/commands/install.ts",
		"cli/commands/install.test.ts",
		"cli/commands/kb.ts",
		"cli/commands/kb-glossary.ts",
		"cli/commands/kb-glossary.test.ts",
		"cli/commands/migrate.e2e.test.ts",
		"cli/commands/migrate.ts",
		"cli/commands/release.ts",
		"cli/commands/report.ts",
		"cli/commands/session.ts",
		"cli/commands/task.ts",
		"cli/commands/task.test.ts",
		"cli/commands/version.ts",
	}
	for _, rel := range missing {
		assertPathMissing(t, root, rel)
	}

	emptyDirs := []string{
		"cli/lib/build",
		"cli/lib/config",
		"cli/lib/detect",
		"cli/lib/housekeeping",
		"cli/lib/install",
		"cli/lib/journal",
		"cli/lib/kb",
		"cli/lib/linear",
		"cli/lib/locks",
		"cli/lib/migrate",
		"cli/lib/release",
		"cli/lib/session",
		"cli/lib/tasks",
	}
	for _, rel := range emptyDirs {
		if files := filesUnder(t, filepath.Join(root, rel)); len(files) > 0 {
			t.Fatalf("%s contains files after native cutover: %v", rel, files)
		}
	}

	// Root bin/ is an ignored build output: a checkout carries no launcher or
	// native binary there until `npm run build:go` runs. The committed Claude
	// Code plugin ships a shim at bin/loaf that resolves an installed loaf and
	// no native runtime of its own.
	assertExecutableFile(t, filepath.Join(root, "plugins", "loaf", "bin", "loaf"))
	for _, rel := range []string{"plugins/loaf/bin/native", "plugins/loaf/bin/package.json"} {
		assertPathMissing(t, root, rel)
	}
	for _, rel := range []string{"dist-cli", "bin/dist-cli", "plugins/loaf/dist-cli"} {
		assertPathMissing(t, root, rel)
	}
}

func TestReleaseWorkflowVerifiesEvidenceBeforeStampedBuild(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	workflow := string(body)
	verifyTests := strings.Index(workflow, "      - name: Verify tests\n")
	testCommand := strings.Index(workflow, "        run: go test ./...\n")
	buildRelease := strings.Index(workflow, "      - name: Build release targets\n")
	stampCommit := strings.Index(workflow, "          export LOAF_BUILD_COMMIT=\"$(git rev-parse --short=7 HEAD)\"\n")
	buildCommand := strings.Index(workflow, "          go run ./cmd/loafdev release\n")
	packageRelease := strings.Index(workflow, "      - name: Package release archives\n")
	if verifyTests < 0 || testCommand < 0 || buildRelease < 0 || stampCommit < 0 || buildCommand < 0 || packageRelease < 0 {
		t.Fatalf("release workflow is missing its evidence verification or checked-out-tree release build contract")
	}
	if !(verifyTests < testCommand && testCommand < buildRelease && buildRelease < stampCommit && stampCommit < buildCommand && buildCommand < packageRelease) {
		t.Fatalf("release workflow must verify evidence before stamping and packaging the checked-out tree")
	}
	if strings.Count(workflow, "LOAF_BUILD_COMMIT") != 1 || strings.Count(workflow, "LOAF_BUILD_DATE") != 1 {
		t.Fatalf("release workflow must confine build metadata to the release build step")
	}
	if !strings.Contains(workflow, "go run ./cmd/loafdev classify-tag") {
		t.Fatalf("release workflow must classify tags with the tested SemVer-first devtool command")
	}
	if strings.Contains(workflow, "npm ") || strings.Contains(workflow, "npm\n") {
		t.Fatalf("release workflow must not use npm")
	}
	if strings.Contains(workflow, `g[0-9a-f]{7,40}`) || strings.Contains(workflow, ">= 1000000000") {
		t.Fatalf("release workflow must not inline tag classification")
	}
}

type packageManifest struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	Scripts         map[string]string      `json:"scripts"`
	Bin             interface{}            `json:"bin"`
	Engines         interface{}            `json:"engines"`
	Exports         map[string]interface{} `json:"exports"`
	Files           []string               `json:"files"`
	Dependencies    map[string]string      `json:"dependencies"`
	DevDependencies map[string]string      `json:"devDependencies"`
}

func readPackageManifest(t *testing.T, root string) packageManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatalf("ReadFile(package.json) error = %v", err)
	}
	var manifest packageManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal(package.json) error = %v", err)
	}
	return manifest
}

func assertPathMissing(t *testing.T, root string, rel string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s still exists", rel)
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v", rel, err)
	}
}

func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files
	} else if err != nil {
		t.Fatalf("Stat(%s) error = %v", root, err)
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", root, err)
	}
	sort.Strings(files)
	return files
}

func assertExecutableFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("%s is a directory, want executable file", path)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s mode = %s, want executable bit set", path, info.Mode())
	}
}

func findTypeScriptFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && (strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%s) error = %v", root, err)
	}
	sort.Strings(files)
	return files
}

type releaseNativeArtifact struct {
	runtimeID  string
	binaryName string
}

func releaseNativeArtifacts() []releaseNativeArtifact {
	return []releaseNativeArtifact{
		{runtimeID: "darwin-arm64", binaryName: "loaf"},
		{runtimeID: "darwin-x64", binaryName: "loaf"},
		{runtimeID: "linux-arm64", binaryName: "loaf"},
		{runtimeID: "linux-x64", binaryName: "loaf"},
		{runtimeID: "win32-arm64", binaryName: "loaf.exe"},
		{runtimeID: "win32-x64", binaryName: "loaf.exe"},
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameSortedStrings(got []string, want []string) bool {
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func splitScriptSteps(script string) []string {
	var steps []string
	for _, step := range strings.Split(script, "&&") {
		steps = append(steps, strings.TrimSpace(step))
	}
	return steps
}
