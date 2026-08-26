package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudEnvironmentBootstrapArtifactsInstallCLI(t *testing.T) {
	root, err := loafRepositoryRoot()
	if err != nil {
		t.Fatalf("loafRepositoryRoot() error = %v", err)
	}
	if err := validateCloudEnvironmentBootstrap(root); err != nil {
		t.Fatalf("validateCloudEnvironmentBootstrap() error = %v", err)
	}
	for _, rel := range []string{cursorCloudInstallScript, ampOrbSetupScript} {
		body, err := readCloudBootstrapFile(root, rel)
		if err != nil {
			t.Fatalf("readCloudBootstrapFile(%q) error = %v", rel, err)
		}
		executable := stripShellComments(body)
		loafIdx := strings.Index(executable, "loaf ")
		buildIdx := strings.Index(executable, "npm run build:go")
		if buildIdx < 0 {
			t.Fatalf("%s missing npm run build:go", rel)
		}
		if loafIdx >= 0 && buildIdx > loafIdx {
			t.Fatalf("%s runs loaf before CLI install", rel)
		}
	}
}

func TestCloudEnvironmentBootstrapStartUsesProjectEnvironmentInstall(t *testing.T) {
	root, err := loafRepositoryRoot()
	if err != nil {
		t.Fatalf("loafRepositoryRoot() error = %v", err)
	}
	for _, rel := range []string{cursorCloudStartScript, ampOrbSetupScript, ampOrbResumeScript} {
		body, err := readCloudBootstrapFile(root, rel)
		if err != nil {
			t.Fatalf("readCloudBootstrapFile(%q) error = %v", rel, err)
		}
		if !strings.Contains(body, projectEnvironmentEnv+"=1") {
			t.Fatalf("%s missing %s=1", rel, projectEnvironmentEnv)
		}
		if !strings.Contains(body, "loaf install --to ") {
			t.Fatalf("%s missing project-environment loaf install", rel)
		}
	}
}

func TestInstallUsesProjectEnvironmentPathsNotUserHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	projectDir := filepath.Join(root, "project")
	dist := filepath.Join(projectDir, "dist")
	userCursor := filepath.Join(home, ".cursor")
	projectCursor := filepath.Join(projectDir, ".cursor")
	mkdirAll(t, userCursor)
	mkdirAll(t, projectDir)
	writeInstallFile(t, filepath.Join(projectDir, "AGENTS.md"), "# Project\n")
	writeInstallFile(t, filepath.Join(projectDir, ".agents", "loaf.json"), `{"issue":{"prefix":"LOAF"}}`+"\n")
	seedCanonicalSkillsFixture(t, projectDir, home, []string{"cursor"}, "# Foundations\n")
	installTestHookDistribution(t, projectDir, "cursor")

	t.Setenv("HOME", home)
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	t.Setenv(projectEnvironmentEnv, "1")

	options := targetInstallOptions{
		Target:      "cursor",
		DistDir:     filepath.Join(dist, "cursor"),
		ConfigDir:   installLayoutConfigDirs(projectDir)["cursor"],
		Version:     "9.8.7-test.1",
		HomeDir:     installLayoutHome(projectDir),
		ProjectRoot: projectDir,
		HookState:   installTestHookState(t),
	}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("installTargetDistribution() error = %v", err)
	}

	if fileExistsForInstall(filepath.Join(userCursor, loafInstallMarkerFile)) {
		t.Fatalf("wrote harness surfaces to user-level %s", userCursor)
	}
	if !fileExistsForInstall(filepath.Join(projectCursor, loafInstallMarkerFile)) {
		t.Fatalf("missing project-environment install marker at %s", projectCursor)
	}
	if !fileExistsForInstall(filepath.Join(projectCursor, "hooks.json")) {
		t.Fatalf("missing project-environment hooks at %s", projectCursor)
	}
	skillsDest := filepath.Join(projectDir, ".agents", "skills", "foundations", "SKILL.md")
	if !fileExistsForInstall(skillsDest) {
		t.Fatalf("skills not installed to project environment: %s", skillsDest)
	}
	userSkills := filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md")
	if fileExistsForInstall(userSkills) {
		t.Fatalf("skills incorrectly installed to user home %s", userSkills)
	}
}

func TestResolveInstallLayoutFallsBackToUserHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	os.Unsetenv(projectEnvironmentEnv)

	dirs, recordHome := resolveInstallLayout(root)
	if recordHome != home {
		t.Fatalf("recordHome = %q, want %q", recordHome, home)
	}
	if dirs["cursor"] != filepath.Join(home, ".cursor") {
		t.Fatalf("cursor config = %q, want %q", dirs["cursor"], filepath.Join(home, ".cursor"))
	}
}

func TestResolveInstallLayoutUsesProjectRootWhenActive(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	projectDir := filepath.Join(root, "project")
	t.Setenv("HOME", home)
	t.Setenv(projectEnvironmentEnv, "1")

	dirs, recordHome := resolveInstallLayout(projectDir)
	if recordHome != projectDir {
		t.Fatalf("recordHome = %q, want %q", recordHome, projectDir)
	}
	if dirs["cursor"] != filepath.Join(projectDir, ".cursor") {
		t.Fatalf("cursor config = %q, want %q", dirs["cursor"], filepath.Join(projectDir, ".cursor"))
	}
	if dirs["amp"] != filepath.Join(projectDir, ".amp") {
		t.Fatalf("amp config = %q, want %q", dirs["amp"], filepath.Join(projectDir, ".amp"))
	}
}

func TestIsLoafInstalledForTargetInstallUsesLayoutHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	projectDir := filepath.Join(root, "project")
	recordDir := filepath.Join(projectDir, ".agents", "loaf", "install-targets")
	mkdirAll(t, recordDir)
	writeInstallFile(t, filepath.Join(recordDir, "cursor.json"), "{}"+"\n")

	t.Setenv("HOME", home)
	t.Setenv(projectEnvironmentEnv, "1")

	if isLoafInstalledForTargetInstall("cursor", filepath.Join(home, ".cursor"), installLayoutHome(projectDir)) {
		// record lives under project root, not user home
	} else {
		t.Fatal("expected install record under project layout home to count as installed")
	}
	if isLoafInstalledForTargetInstall("cursor", filepath.Join(home, ".cursor"), home) {
		t.Fatal("user home must not satisfy project-environment install record")
	}
}

func TestCloudEnvironmentBootstrapDocumentsClientTokenSecret(t *testing.T) {
	if projectEnvironmentClientTokenEnv != "LOAF_CLIENT_TOKEN" {
		t.Fatalf("client token env = %q", projectEnvironmentClientTokenEnv)
	}
}
