package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestNativeCutoverPackageAndSourceGuards(t *testing.T) {
	root := repoRoot(t)
	manifest := readPackageManifest(t, root)

	if got := manifest.Scripts["build:cli-ref"]; got != "bin/loaf __generate-cli-ref" {
		t.Fatalf("build:cli-ref = %q, want native generator", got)
	}
	if got := manifest.Scripts["build:content"]; got != "bin/loaf build" {
		t.Fatalf("build:content = %q, want native launcher", got)
	}
	if got := manifest.Scripts["build:release"]; got != "node cli/scripts/build-release.mjs" {
		t.Fatalf("build:release = %q, want native release artifact builder", got)
	}
	if got := manifest.Scripts["prepublishOnly"]; got != "npm run build:release" {
		t.Fatalf("prepublishOnly = %q, want release artifact builder", got)
	}
	wantFiles := []string{"bin/", "config/", "content/"}
	if got := append([]string(nil), manifest.Files...); !sameSortedStrings(got, wantFiles) {
		t.Fatalf("package.json files = %#v, want exactly %#v", manifest.Files, wantFiles)
	}
	if containsString(splitScriptSteps(manifest.Scripts["build"]), "npm run build:cli") {
		t.Fatalf("build script = %q, want no TypeScript CLI build step", manifest.Scripts["build"])
	}
	if containsString(splitScriptSteps(manifest.Scripts["prepare"]), "npm run build:cli") {
		t.Fatalf("prepare script = %q, want no TypeScript CLI build step", manifest.Scripts["prepare"])
	}
	if _, ok := manifest.Scripts["build:cli"]; ok {
		t.Fatalf("package.json still defines build:cli")
	}
	if _, ok := manifest.Scripts["pretest"]; ok {
		t.Fatalf("package.json still defines pretest")
	}
	if _, ok := manifest.Exports["."]; ok {
		t.Fatalf("package.json still exports TypeScript runtime bundle")
	}
	if containsString(manifest.Files, "dist-cli/") {
		t.Fatalf("package.json files includes dist-cli/: %#v", manifest.Files)
	}
	if _, ok := manifest.Dependencies["commander"]; ok {
		t.Fatalf("package.json still depends on commander")
	}
	if _, ok := manifest.Dependencies["gray-matter"]; ok {
		t.Fatalf("package.json still depends on gray-matter")
	}
	if _, ok := manifest.DevDependencies["tsup"]; ok {
		t.Fatalf("package.json still depends on tsup")
	}
	for _, dep := range []string{"picomatch", "yaml"} {
		if _, ok := manifest.Dependencies[dep]; ok {
			t.Fatalf("package.json still depends on obsolete TypeScript build dependency %s", dep)
		}
	}
	for _, dep := range []string{"@types/node", "@types/picomatch", "typescript", "vitest"} {
		if _, ok := manifest.DevDependencies[dep]; ok {
			t.Fatalf("package.json still depends on obsolete TypeScript test dependency %s", dep)
		}
	}
	if tsFiles := findTypeScriptFiles(t, filepath.Join(root, "cli")); len(tsFiles) > 0 {
		t.Fatalf("cli/ still contains TypeScript sources after native cutover: %v", tsFiles)
	}

	missing := []string{
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

	for _, rel := range []string{
		"bin/loaf",
		"plugins/loaf/bin/loaf",
	} {
		assertExecutableFile(t, filepath.Join(root, filepath.FromSlash(rel)))
	}
	for _, artifact := range releaseNativeArtifacts() {
		for _, binRoot := range []string{"bin", filepath.ToSlash(filepath.Join("plugins", "loaf", "bin"))} {
			rel := filepath.ToSlash(filepath.Join(binRoot, "native", artifact.runtimeID, artifact.binaryName))
			assertExecutableFile(t, filepath.Join(root, filepath.FromSlash(rel)))
		}
	}
	for _, rel := range []string{"dist-cli", "bin/dist-cli", "plugins/loaf/dist-cli"} {
		assertPathMissing(t, root, rel)
	}
}

func TestNativeGoArtifactScriptsSupportExplicitReleaseTargets(t *testing.T) {
	node := requireNode(t)
	root := repoRoot(t)

	buildOutput, err := runNodeScript(t, node, root, []string{
		"LOAF_NATIVE_ARTIFACT_DRY_RUN=1",
		"LOAF_BUILD_TARGETS=linux-x64,win32-x64,linux-x64",
	}, "cli/scripts/build-go.mjs")
	if err != nil {
		t.Fatalf("build-go dry run error = %v\n%s", err, buildOutput)
	}
	for _, want := range []string{
		"DRY RUN: would build linux-x64",
		filepath.ToSlash(filepath.Join("bin", "native", "linux-x64", "loaf")),
		"DRY RUN: would build win32-x64",
		filepath.ToSlash(filepath.Join("bin", "native", "win32-x64", "loaf.exe")),
	} {
		if !strings.Contains(filepath.ToSlash(buildOutput), want) {
			t.Fatalf("build-go dry run output = %q, want %q", buildOutput, want)
		}
	}
	if got := strings.Count(buildOutput, "DRY RUN: would build linux-x64"); got != 1 {
		t.Fatalf("build-go dry run built linux-x64 %d times, want deduped once\n%s", got, buildOutput)
	}

	verifyOutput, err := runNodeScript(t, node, root, []string{
		"LOAF_NATIVE_ARTIFACT_DRY_RUN=1",
		"LOAF_BUILD_TARGETS=darwin-arm64,win32-x64",
	}, "cli/scripts/verify-go-artifacts.mjs")
	if err != nil {
		t.Fatalf("verify-go-artifacts dry run error = %v\n%s", err, verifyOutput)
	}
	for _, want := range []string{
		filepath.ToSlash(filepath.Join("bin", "native", "darwin-arm64", "loaf")),
		filepath.ToSlash(filepath.Join("plugins", "loaf", "bin", "native", "darwin-arm64", "loaf")),
		filepath.ToSlash(filepath.Join("bin", "native", "win32-x64", "loaf.exe")),
		filepath.ToSlash(filepath.Join("plugins", "loaf", "bin", "native", "win32-x64", "loaf.exe")),
	} {
		if !strings.Contains(filepath.ToSlash(verifyOutput), want) {
			t.Fatalf("verify-go-artifacts dry run output = %q, want %q", verifyOutput, want)
		}
	}

	failOutput, err := runNodeScript(t, node, root, []string{
		"LOAF_NATIVE_ARTIFACT_DRY_RUN=1",
		"LOAF_BUILD_TARGETS=solaris-x64",
	}, "cli/scripts/build-go.mjs")
	if err == nil {
		t.Fatalf("build-go dry run accepted unsupported target\n%s", failOutput)
	}
	if !strings.Contains(failOutput, "unsupported LOAF_BUILD_TARGETS entry") || !strings.Contains(failOutput, "linux-x64") {
		t.Fatalf("unsupported target output = %q, want helpful supported-target error", failOutput)
	}
}

func TestNativeGoReleaseBuildUsesPublishTargetPolicy(t *testing.T) {
	node := requireNode(t)
	root := repoRoot(t)

	defaultOutput, err := runNodeScript(t, node, root, []string{
		"LOAF_RELEASE_DRY_RUN=1",
	}, "cli/scripts/build-release.mjs")
	if err != nil {
		t.Fatalf("build-release dry run error = %v\n%s", err, defaultOutput)
	}
	for _, want := range []string{
		"DRY RUN: would run npm run build for native release targets:",
		"darwin-arm64",
		"darwin-x64",
		"linux-arm64",
		"linux-x64",
		"win32-arm64",
		"win32-x64",
		"DRY RUN: LOAF_VERIFY_TARGETS=darwin-arm64,darwin-x64,linux-arm64,linux-x64,win32-arm64,win32-x64",
	} {
		if !strings.Contains(defaultOutput, want) {
			t.Fatalf("build-release default dry run output = %q, want %q", defaultOutput, want)
		}
	}

	overrideOutput, err := runNodeScript(t, node, root, []string{
		"LOAF_RELEASE_DRY_RUN=1",
		"LOAF_RELEASE_TARGETS=linux-x64,win32-x64",
	}, "cli/scripts/build-release.mjs")
	if err != nil {
		t.Fatalf("build-release override dry run error = %v\n%s", err, overrideOutput)
	}
	for _, want := range []string{
		"DRY RUN: would run npm run build for native release targets: linux-x64,win32-x64",
		"DRY RUN: LOAF_VERIFY_TARGETS=linux-x64,win32-x64",
	} {
		if !strings.Contains(overrideOutput, want) {
			t.Fatalf("build-release override dry run output = %q, want %q", overrideOutput, want)
		}
	}
}

type packageManifest struct {
	Scripts         map[string]string      `json:"scripts"`
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

func runNodeScript(t *testing.T, node string, root string, env []string, script string) (string, error) {
	t.Helper()
	cmd := exec.Command(node, script)
	cmd.Dir = root
	cmd.Env = envWith(env...)
	output, err := cmd.CombinedOutput()
	return string(output), err
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
