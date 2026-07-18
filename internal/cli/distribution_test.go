package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// distributionFixtureExecutable returns an Executable override pointing inside
// the fixture tree, mirroring the bin/native/<target>/loaf layout, so
// installed-distribution resolution selects the fixture as the running
// binary's own distribution.
func distributionFixtureExecutable(root string) func() (string, error) {
	return func() (string, error) {
		return filepath.Join(root, "bin", "native", "test-target", "loaf"), nil
	}
}

func TestResolveInstalledDistributionRootUsesExecutableProvenanceNotWorkingDirectory(t *testing.T) {
	installed := writeVersionFixture(t)
	stale := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(stale, "package.json"), `{"name":"loaf","version":"1.1.1-stale"}`)

	runner := Runner{WorkingDir: stale, Executable: distributionFixtureExecutable(installed)}
	root, err := runner.resolveInstalledDistributionRoot()
	if err != nil {
		t.Fatalf("resolveInstalledDistributionRoot error = %v", err)
	}
	if root != installed {
		t.Fatalf("root = %q, want executable-adjacent distribution %q (never the working directory %q)", root, installed, stale)
	}
}

func TestResolveInstalledDistributionRootFailsClosedWithoutAdjacentDistribution(t *testing.T) {
	bare := realpath(t, t.TempDir())
	runner := Runner{
		WorkingDir: writeVersionFixture(t),
		Executable: func() (string, error) { return filepath.Join(bare, "loaf"), nil },
	}
	if root, err := runner.resolveInstalledDistributionRoot(); err == nil {
		t.Fatalf("resolveInstalledDistributionRoot = %q, want failure for a bare executable with no adjacent distribution", root)
	} else if !strings.Contains(err.Error(), "reinstall Loaf") {
		t.Fatalf("error = %v, want reinstall guidance", err)
	}
}

func TestResolveInstalledDistributionRootFollowsExecutableSymlink(t *testing.T) {
	installed := writeVersionFixture(t)
	nativeBinary := filepath.Join(installed, "bin", "native", "test-target", "loaf")
	mkdirAll(t, filepath.Dir(nativeBinary))
	writeFile(t, nativeBinary, "#!/bin/sh\n")
	shimDir := realpath(t, t.TempDir())
	shim := filepath.Join(shimDir, "loaf")
	if err := os.Symlink(nativeBinary, shim); err != nil {
		t.Fatalf("Symlink error = %v", err)
	}

	runner := Runner{Executable: func() (string, error) { return shim, nil }}
	root, err := runner.resolveInstalledDistributionRoot()
	if err != nil {
		t.Fatalf("resolveInstalledDistributionRoot error = %v", err)
	}
	if root != installed {
		t.Fatalf("root = %q, want symlink-evaluated distribution %q", root, installed)
	}
}

func TestRunnerVersionReportsInstalledDistributionFromStaleCheckout(t *testing.T) {
	installed := writeVersionFixture(t)
	stale := realpath(t, t.TempDir())
	writeFile(t, filepath.Join(stale, "package.json"), `{"name":"loaf","version":"1.1.1-stale"}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: stale,
		Executable: distributionFixtureExecutable(installed),
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}
	if !strings.Contains(stdout.String(), "9.8.7-test.1") {
		t.Fatalf("version output = %q, want installed distribution version", stdout.String())
	}
	if strings.Contains(stdout.String(), "1.1.1-stale") {
		t.Fatalf("version output = %q, must not report the stale checkout version", stdout.String())
	}
}

func TestRunnerVersionFallsBackToZeroWithoutDistribution(t *testing.T) {
	bare := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: writeVersionFixture(t),
		Executable: func() (string, error) { return filepath.Join(bare, "loaf"), nil },
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}
	if !strings.Contains(stdout.String(), "0.0.0") {
		t.Fatalf("version output = %q, want honest 0.0.0 fallback instead of adopting the working directory", stdout.String())
	}
}

// The Content/Targets helpers join paths onto the resolved root; with no
// installed distribution an empty root would degrade those joins to
// working-directory-relative lookups. Standing inside a fully populated stale
// checkout, a bare binary must not surface that checkout's targets or counts.
func TestRunnerVersionOmitsTargetsAndContentWithoutDistribution(t *testing.T) {
	stale := writeVersionFixture(t)
	t.Chdir(stale)
	bare := realpath(t, t.TempDir())
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: stale,
		Executable: func() (string, error) { return filepath.Join(bare, "loaf"), nil },
	}.Run([]string{"version"})
	if err != nil {
		t.Fatalf("version error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "0.0.0") {
		t.Fatalf("version output = %q, want honest 0.0.0 fallback", output)
	}
	if !strings.Contains(output, "reinstall Loaf") {
		t.Fatalf("version output = %q, want the resolver's reinstall guidance", output)
	}
	for _, forbidden := range []string{"Targets:", "Content:", "Skills:", "Agents:", "Hooks:", "plugins/loaf/", "dist/cursor/"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("version output = %q, must not read %q from the working directory without an installed distribution", output, forbidden)
		}
	}
}

func TestRunnerInstallFailsClosedWithoutDistribution(t *testing.T) {
	bare := realpath(t, t.TempDir())
	root, _ := setupInstallCommandFixture(t)
	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
		Executable: func() (string, error) { return filepath.Join(bare, "loaf"), nil },
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "no Loaf distribution found alongside") {
		t.Fatalf("install error = %v, want fail-closed distribution resolution instead of installing from the working directory", err)
	}
}
