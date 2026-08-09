package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This file covers the states a `.agents/loaf.json` path can be in that are
// neither absent nor readable: a file whose bytes never come back. They belong
// with the malformed case rather than with absence — a writer that read nothing
// has learned nothing about what it would be destroying — so every surface that
// touches the file refuses, preserves it, and says which path it left alone.

const preservedLoafConfigBody = "{\"integrations\":{\"linear\":{\"enabled\":true}}}\n"

// unreadableLoafConfigCase is one such state, with the assertion that it came
// through the command exactly as it went in.
type unreadableLoafConfigCase struct {
	name   string
	setup  func(t *testing.T, root string)
	verify func(t *testing.T, root string)
}

func unreadableLoafConfigCases() []unreadableLoafConfigCase {
	configPath := func(root string) string { return filepath.Join(root, ".agents", "loaf.json") }
	return []unreadableLoafConfigCase{
		{
			name: "write-only file",
			setup: func(t *testing.T, root string) {
				skipWithoutEnforcedPermissions(t)
				writeInstallFile(t, configPath(root), preservedLoafConfigBody)
				chmodForTest(t, configPath(root), 0o200)
			},
			verify: func(t *testing.T, root string) {
				chmodForTest(t, configPath(root), 0o600)
				if got := string(readFileBytes(t, configPath(root))); got != preservedLoafConfigBody {
					t.Fatalf("loaf.json = %q, want it preserved byte-for-byte as %q", got, preservedLoafConfigBody)
				}
			},
		},
		{
			name: "directory at the path",
			setup: func(t *testing.T, root string) {
				mkdirAll(t, filepath.Join(configPath(root), "not-a-config"))
			},
			verify: func(t *testing.T, root string) {
				info, err := os.Stat(configPath(root))
				if err != nil || !info.IsDir() {
					t.Fatalf("stat(loaf.json) = %v, %v, want the directory left in place", info, err)
				}
				if _, err := os.Stat(filepath.Join(configPath(root), "not-a-config")); err != nil {
					t.Fatalf("stat(loaf.json/not-a-config) error = %v, want its contents untouched", err)
				}
			},
		},
		{
			name: "unreadable parent directory",
			setup: func(t *testing.T, root string) {
				skipWithoutEnforcedPermissions(t)
				writeInstallFile(t, configPath(root), preservedLoafConfigBody)
				chmodForTest(t, filepath.Join(root, ".agents"), 0o000)
			},
			verify: func(t *testing.T, root string) {
				chmodForTest(t, filepath.Join(root, ".agents"), 0o755)
				if got := string(readFileBytes(t, configPath(root))); got != preservedLoafConfigBody {
					t.Fatalf("loaf.json = %q, want it preserved byte-for-byte as %q", got, preservedLoafConfigBody)
				}
			},
		},
	}
}

// TestUpgradePreservesAProjectConfigItCannotRead is the apply-side gate. With
// --yes and nothing to stop it, the record refresh used to read the file as an
// empty document and write Loaf's defaults over it, which on a write-only file
// meant truncating something it had not read one byte of.
func TestUpgradePreservesAProjectConfigItCannotRead(t *testing.T) {
	for _, testCase := range unreadableLoafConfigCases() {
		t.Run(testCase.name, func(t *testing.T) {
			root, home := setupUpgradeFixture(t)
			installUpgradeFixtureTarget(t, root, home, "cursor")
			writeManagedAgentsFileForTest(t, root)
			testCase.setup(t, root)

			output := runInstallCapture(t, root, "upgrade", "--yes")

			assertPreservedConfigReported(t, output)
			testCase.verify(t, root)
		})
	}
}

// TestUpgradeDryRunReportsAProjectConfigItCannotRead is the plan-side twin: the
// plan may never promise a write the apply path refuses to make, so an
// unreadable config reports the same refusal there.
func TestUpgradeDryRunReportsAProjectConfigItCannotRead(t *testing.T) {
	for _, testCase := range unreadableLoafConfigCases() {
		t.Run(testCase.name, func(t *testing.T) {
			root, home := setupUpgradeFixture(t)
			installUpgradeFixtureTarget(t, root, home, "cursor")
			writeManagedAgentsFileForTest(t, root)
			testCase.setup(t, root)

			plan := parseInstallPlanJSON(t, runInstallCapture(t, root, "upgrade", "--dry-run", "--json"))

			entry, found := findInstallPlanEntry(plan, ".agents/loaf.json")
			if !found {
				t.Fatalf("project_files = %#v, want an entry for the config it cannot read", plan.ProjectFiles)
			}
			if entry.Action != "skipped" {
				t.Fatalf("plan entry = %#v, want it skipped rather than promised", entry)
			}
			if !strings.Contains(entry.Detail, "could not be read") || !strings.Contains(entry.Detail, filepath.Join(".agents", "loaf.json")) {
				t.Fatalf("plan entry detail = %q, want the read failure and the path", entry.Detail)
			}
			testCase.verify(t, root)
		})
	}
}

// TestInstallPreservesAProjectConfigItCannotRead is install's half. Consent to
// deploy Loaf into a folder authorizes writing Loaf's own files; it does not
// authorize replacing a project config Loaf could not read.
func TestInstallPreservesAProjectConfigItCannotRead(t *testing.T) {
	for _, testCase := range unreadableLoafConfigCases() {
		t.Run(testCase.name, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
			writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
			installTestHookDistribution(t, root, "cursor")
			mkdirAll(t, filepath.Join(home, ".cursor"))
			testCase.setup(t, root)

			var stdout strings.Builder
			options := installOptions{target: "cursor", projectDeployGranted: true}
			runner := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}
			if err := runner.runInstallWithOptions(options, &stdout, root); err != nil {
				t.Fatalf("install error = %v\n%s", err, stdout.String())
			}

			assertPreservedConfigReported(t, stdout.String())
			testCase.verify(t, root)
		})
	}
}

func assertPreservedConfigReported(t *testing.T, output string) {
	t.Helper()
	if !strings.Contains(output, "could not be read") {
		t.Fatalf("output = %q, want the read failure reported", output)
	}
	if !strings.Contains(output, filepath.Join(".agents", "loaf.json")) {
		t.Fatalf("output = %q, want the preserved path named", output)
	}
	if !strings.Contains(output, "preserving it as written") {
		t.Fatalf("output = %q, want the preservation stated", output)
	}
}

// writeManagedAgentsFileForTest gives the detector a reason to run the project
// part that is not the config file itself — which, in these fixtures, is the
// one thing that cannot be read.
func writeManagedAgentsFileForTest(t *testing.T, root string) {
	t.Helper()
	writeInstallFile(t, filepath.Join(root, "AGENTS.md"), "# Project\n\n"+generateFencedContent()+"\n")
}

func findInstallPlanEntry(plan installDryRunPlan, path string) (projectFilePlanEntry, bool) {
	for _, entry := range plan.ProjectFiles {
		if entry.Path == path {
			return entry, true
		}
	}
	return projectFilePlanEntry{}, false
}

// skipWithoutEnforcedPermissions skips the cases that need a read to actually
// be denied: Windows does not deny reads through a mode-0200 file, and root is
// not stopped by any mode at all.
func skipWithoutEnforcedPermissions(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits deny nothing")
	}
}

// chmodForTest changes a mode and restores something removable at cleanup, so a
// mode-0000 directory cannot outlive the test and strand t.TempDir.
func chmodForTest(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod(%s, %v) error = %v", path, mode, err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o755) })
}
