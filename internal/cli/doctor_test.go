package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var regexpANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestTargetAdapterVersionFromExecutable(t *testing.T) {
	distributionRoot := filepath.Join(t.TempDir(), "distribution")
	writeDoctorFile(t, filepath.Join(distributionRoot, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`+"\n")
	executable := filepath.Join(distributionRoot, "bin", "native", "loaf")
	if got := targetAdapterVersionFromExecutable(executable); got != "9.8.7-test.1" {
		t.Fatalf("targetAdapterVersionFromExecutable() = %q, want %q", got, "9.8.7-test.1")
	}
}

func TestTargetAdapterVersionFromExecutableIgnoresWorkingAndRuntimeCheckouts(t *testing.T) {
	workingRoot := filepath.Join(t.TempDir(), "working")
	runtimeRoot := filepath.Join(t.TempDir(), "runtime")
	writeDoctorFile(t, filepath.Join(workingRoot, "package.json"), `{"name":"loaf","version":"1.0.0-working"}`+"\n")
	writeDoctorFile(t, filepath.Join(runtimeRoot, "package.json"), `{"name":"loaf","version":"2.0.0-runtime"}`+"\n")
	executable := filepath.Join(t.TempDir(), "outside-distribution", "loaf")
	if got := targetAdapterVersionFromExecutable(executable); got != "" {
		t.Fatalf("targetAdapterVersionFromExecutable() = %q, want empty outside a Loaf distribution", got)
	}
}

func TestRunnerDoctorHealthyProjectRunsNatively(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	symlinkFile(t, "../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor error = %v", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "loaf doctor") || strings.Contains(output, "failed") {
		t.Fatalf("doctor output = %q, want native healthy report", output)
	}
	if strings.Contains(output, "args=doctor") {
		t.Fatalf("doctor output = %q, want no legacy bridge output", output)
	}
}

func TestRunnerDoctorReturnsExitOneForFailures(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorFile(t, filepath.Join(root, ".cursor", "rules", "loaf.mdc"), "legacy\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor error = %v, want exit code 1", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "stale-cursor-mdc") || !strings.Contains(output, "failed") {
		t.Fatalf("doctor failure output = %q, want failing stale-file check", output)
	}
}

func TestRunnerDoctorReportsDriftedTargetAdapterArtifact(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("PATH", t.TempDir())
	config := filepath.Join(home, "config", "opencode")
	mode := uint32(0o644)
	artifact := targetAdapterArtifact{ID: "plugin:plugins/hooks.ts", Kind: "plugin", SourcePath: "plugins/hooks.ts", Destination: "plugins/hooks.ts", SHA256: sha256Bytes([]byte("expected\n")), Mode: &mode}
	writeDoctorFile(t, filepath.Join(config, "plugins", "hooks.ts"), "drifted\n")
	if err := writeTargetAdapterManifest(filepath.Join(config, targetInstallManifestFile), targetAdapterManifest{Version: 1, Target: "opencode", PackageVersion: "9.8.7-test.1", CapabilityContractVersion: TargetCapabilityEvidenceContractVersion, Adapters: []string{"opencode-context-v1"}, Artifacts: []targetAdapterArtifact{artifact}}); err != nil {
		t.Fatal(err)
	}
	writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "opencode", ConfigDir: config})
	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"doctor"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 || !strings.Contains(stripANSI(stdout.String()), "target-adapter-ownership") {
		t.Fatalf("doctor drift = %v\n%s", err, stdout.String())
	}
}

func TestRunnerDoctorTargetAdapterOwnershipHealthyMissingAndMalformed(t *testing.T) {
	for _, tc := range []struct {
		name, installed, manifest string
		omitManifest              bool
		wantError                 bool
	}{
		{name: "healthy", installed: "expected\n"},
		{name: "missing artifact", wantError: true},
		{name: "missing manifest", installed: "expected\n", omitManifest: true, wantError: true},
		{name: "malformed manifest", installed: "expected\n", manifest: "{", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeDoctorFixture(t, "9.8.7-test.1")
			home := os.Getenv("HOME")
			config := filepath.Join(home, "config", "opencode")
			mode := uint32(0o644)
			artifact := targetAdapterArtifact{ID: "plugin:plugins/hooks.ts", Kind: "plugin", SourcePath: "plugins/hooks.ts", Destination: "plugins/hooks.ts", SHA256: sha256Bytes([]byte("expected\n")), Mode: &mode}
			if tc.installed != "" {
				writeDoctorFile(t, filepath.Join(config, "plugins", "hooks.ts"), tc.installed)
			}
			if !tc.omitManifest {
				if tc.manifest != "" {
					writeDoctorFile(t, filepath.Join(config, targetInstallManifestFile), tc.manifest)
				} else if err := writeTargetAdapterManifest(filepath.Join(config, targetInstallManifestFile), targetAdapterManifest{Version: 1, Target: "opencode", PackageVersion: "9.8.7-test.1", CapabilityContractVersion: TargetCapabilityEvidenceContractVersion, Adapters: []string{"opencode-context-v1"}, Artifacts: []targetAdapterArtifact{artifact}}); err != nil {
					t.Fatal(err)
				}
			}
			writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "opencode", ConfigDir: config})
			before := ""
			if tc.installed != "" {
				before = string(readFileBytes(t, filepath.Join(config, "plugins", "hooks.ts")))
			}
			var stdout bytes.Buffer
			err := (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"doctor", "--fix"})
			if tc.wantError && err == nil {
				t.Fatalf("doctor output = %s, want failure", stdout.String())
			}
			if !tc.wantError && err != nil {
				t.Fatal(err)
			}
			if tc.installed != "" && string(readFileBytes(t, filepath.Join(config, "plugins", "hooks.ts"))) != before {
				t.Fatal("doctor --fix mutated adapter artifact")
			}
		})
	}
}

func TestTargetAdapterOwnershipRejectsWrongTargetSymlinkModeAndUsesRecordedAmpDir(t *testing.T) {
	for _, tc := range []struct {
		name       string
		target     string
		manifestAs string
		symlink    bool
		mode       os.FileMode
		want       doctorStatus
	}{
		{name: "wrong target", target: "opencode", manifestAs: "cursor", mode: 0o644, want: doctorFail},
		{name: "symlink manifest", target: "opencode", manifestAs: "opencode", symlink: true, mode: 0o644, want: doctorFail},
		{name: "mode drift", target: "opencode", manifestAs: "opencode", mode: 0o755, want: doctorFail},
		{name: "recorded amp directory", target: "amp", manifestAs: "amp", mode: 0o644, want: doctorPass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeDoctorFixture(t, "9.8.7-test.1")
			home := os.Getenv("HOME")
			config := filepath.Join(home, "custom", tc.target)
			mode := uint32(0o644)
			artifact := targetAdapterArtifact{ID: "plugin:plugins/hooks.ts", Kind: "plugin", SourcePath: "plugins/hooks.ts", Destination: "plugins/hooks.ts", SHA256: sha256Bytes([]byte("expected\n")), Mode: &mode}
			path := filepath.Join(config, "plugins", "hooks.ts")
			if tc.target == "amp" {
				artifact = targetAdapterArtifact{ID: "plugin:.amp/plugins/loaf.ts", Kind: "plugin", SourcePath: ".amp/plugins/loaf.ts", Destination: "plugins/loaf.ts", SHA256: sha256Bytes([]byte("expected\n")), Mode: &mode}
				path = filepath.Join(config, "plugins", "loaf.ts")
			}
			writeDoctorFile(t, path, "expected\n")
			if err := os.Chmod(path, tc.mode); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(config, targetInstallManifestFile)
			if err := writeTargetAdapterManifest(manifestPath, targetAdapterManifest{Version: 1, Target: tc.manifestAs, PackageVersion: "9.8.7-test.1", CapabilityContractVersion: TargetCapabilityEvidenceContractVersion, Adapters: []string{"adapter"}, Artifacts: []targetAdapterArtifact{artifact}}); err != nil {
				t.Fatal(err)
			}
			if tc.symlink {
				if err := os.Rename(manifestPath, manifestPath+".real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(manifestPath+".real", manifestPath); err != nil {
					t.Fatal(err)
				}
			}
			writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: tc.target, ConfigDir: config})
			result := checkTargetAdapterOwnership("9.8.7-test.1").Run(doctorContext{projectRoot: root})
			if result.Status != tc.want {
				t.Fatalf("status = %s, detail = %s, want %s", result.Status, result.Detail, tc.want)
			}
		})
	}
}

func TestTargetAdapterOwnershipFailsClosedForDetectedAndMalformedRecords(t *testing.T) {
	for _, tc := range []struct {
		name   string
		record string
	}{
		{name: "detected without record"},
		{name: "malformed record", record: "{"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeDoctorFixture(t, "9.8.7-test.1")
			home := os.Getenv("HOME")
			config := filepath.Join(home, "config", "opencode")
			writeDoctorFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
			if tc.record != "" {
				writeDoctorFile(t, installRecordPath(home, "opencode"), tc.record)
			}
			result := checkTargetAdapterOwnership("9.8.7-test.1").Run(doctorContext{projectRoot: root})
			if result.Status != doctorFail {
				t.Fatalf("status = %s, want fail", result.Status)
			}
		})
	}
}

func TestTargetAdapterOwnershipUsesRecordedCodexAndAmpDirectories(t *testing.T) {
	for _, target := range []string{"codex", "amp"} {
		t.Run(target, func(t *testing.T) {
			root := writeDoctorFixture(t, "9.8.7-test.1")
			home := os.Getenv("HOME")
			config := filepath.Join(root, "recorded", target)
			var artifacts []targetAdapterArtifact
			if target == "codex" {
				body := `{"hooks":{"SessionStart":[{"matcher":"startup|resume|clear|compact","hooks":[{"type":"command","command":"'/opt/loaf' journal context --from-hook --codex-hook"}]}]}}`
				writeDoctorFile(t, filepath.Join(config, "hooks.json"), body)
				digest, err := targetHookProjectionDigest("codex", []byte(body), true)
				if err != nil {
					t.Fatal(err)
				}
				artifacts = []targetAdapterArtifact{{ID: "hook-projection:.codex/hooks.json", Kind: "hook-projection", SourcePath: ".codex/hooks.json", Destination: "hooks.json", SHA256: digest}}
			} else {
				artifacts = []targetAdapterArtifact{writeDoctorConcreteAdapterArtifact(t, config, "plugin:.amp/plugins/loaf.ts", "plugin", ".amp/plugins/loaf.ts", "plugins/loaf.ts", "amp plugin\n", 0o644)}
			}
			writeDoctorOwnershipManifest(t, config, target, "9.8.7-test.1", artifacts)
			writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: target, ConfigDir: config})
			result := checkTargetAdapterOwnership("9.8.7-test.1").Run(doctorContext{projectRoot: root})
			if result.Status != doctorPass {
				t.Fatalf("status = %s, detail = %s", result.Status, result.Detail)
			}
		})
	}
}

func TestTargetAdapterOwnershipDetailsAreDeterministicAcrossTargetsAndArtifacts(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	home := os.Getenv("HOME")
	opencodeConfig := filepath.Join(root, "recorded", "opencode")
	ampConfig := filepath.Join(root, "recorded", "amp")
	opencodeArtifacts := []targetAdapterArtifact{
		writeDoctorConcreteAdapterArtifact(t, opencodeConfig, "plugin:z", "plugin", "plugins/z.ts", "plugins/z.ts", "z\n", 0o644),
		writeDoctorConcreteAdapterArtifact(t, opencodeConfig, "hook-file:a", "hook-file", "hooks/a.sh", "hooks/a.sh", "#!/bin/sh\n", 0o755),
	}
	ampArtifacts := []targetAdapterArtifact{writeDoctorConcreteAdapterArtifact(t, ampConfig, "plugin:amp", "plugin", ".amp/plugins/loaf.ts", "plugins/loaf.ts", "amp\n", 0o644)}
	writeDoctorOwnershipManifest(t, opencodeConfig, "opencode", "9.8.7-test.1", opencodeArtifacts)
	writeDoctorOwnershipManifest(t, ampConfig, "amp", "9.8.7-test.1", ampArtifacts)
	writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "opencode", ConfigDir: opencodeConfig})
	writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "amp", ConfigDir: ampConfig})
	writeDoctorFile(t, filepath.Join(ampConfig, "plugins", "loaf.ts"), "drifted amp\n")
	writeDoctorFile(t, filepath.Join(opencodeConfig, "plugins", "z.ts"), "drifted z\n")

	result := checkTargetAdapterOwnership("9.8.7-test.1").Run(doctorContext{projectRoot: root})
	if result.Status != doctorFail {
		t.Fatalf("status = %s, detail = %s", result.Status, result.Detail)
	}
	want := strings.Join([]string{
		"amp plugin:amp: drifted " + filepath.Join(ampConfig, "plugins", "loaf.ts"),
		"opencode hook-file:a: matches",
		"opencode plugin:z: drifted " + filepath.Join(opencodeConfig, "plugins", "z.ts"),
	}, "\n")
	if result.Detail != want {
		t.Fatalf("detail = %q, want %q", result.Detail, want)
	}
}

func TestTargetAdapterOwnershipRejectsArtifactSymlink(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	home := os.Getenv("HOME")
	config := filepath.Join(root, "recorded", "opencode")
	artifact := writeDoctorConcreteAdapterArtifact(t, config, "plugin:hooks", "plugin", "plugins/hooks.ts", "plugins/hooks.ts", "plugin\n", 0o644)
	path := filepath.Join(config, "plugins", "hooks.ts")
	if err := os.Rename(path, path+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path+".real", path); err != nil {
		t.Fatal(err)
	}
	writeDoctorOwnershipManifest(t, config, "opencode", "9.8.7-test.1", []targetAdapterArtifact{artifact})
	writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "opencode", ConfigDir: config})
	result := checkTargetAdapterOwnership("9.8.7-test.1").Run(doctorContext{projectRoot: root})
	if result.Status != doctorFail || !strings.Contains(result.Detail, "must be a regular file") {
		t.Fatalf("status = %s, detail = %s", result.Status, result.Detail)
	}
}

func TestTargetAdapterOwnershipMatchesCursorProjectionAndPreservesForeignHooks(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	home := os.Getenv("HOME")
	config := filepath.Join(root, "recorded", "cursor")
	installed := `{"version":1,"hooks":{"PostToolUse":[{"command":"foreign one"},{"command":"loaf task refresh","matcher":"Edit|Write","loaf-managed":true}]}}`
	writeDoctorFile(t, filepath.Join(config, "hooks.json"), installed)
	digest, err := targetHookProjectionDigest("cursor", []byte(installed), true)
	if err != nil {
		t.Fatal(err)
	}
	artifact := targetAdapterArtifact{ID: "hook-projection:hooks.json", Kind: "hook-projection", SourcePath: "hooks.json", Destination: "hooks.json", SHA256: digest}
	writeDoctorOwnershipManifest(t, config, "cursor", "9.8.7-test.1", []targetAdapterArtifact{artifact})
	writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "cursor", ConfigDir: config})
	for _, body := range []string{
		installed,
		`{"version":1,"hooks":{"PostToolUse":[{"command":"foreign changed without touching Loaf"},{"command":"loaf task refresh","matcher":"Edit|Write","loaf-managed":true}]}}`,
	} {
		writeDoctorFile(t, filepath.Join(config, "hooks.json"), body)
		result := checkTargetAdapterOwnership("9.8.7-test.1").Run(doctorContext{projectRoot: root})
		if result.Status != doctorPass {
			t.Fatalf("status = %s, detail = %s", result.Status, result.Detail)
		}
	}
}

func TestTargetAdapterOwnershipReportsVersionMismatchWithoutUntrustedCLIComparison(t *testing.T) {
	for _, tc := range []struct {
		name, recordVersion, manifestVersion, cliVersion string
		want                                             doctorStatus
	}{
		{name: "record mismatch", recordVersion: "1.0.0", manifestVersion: "2.0.0", cliVersion: "2.0.0", want: doctorFail},
		{name: "manifest mismatch", recordVersion: "2.0.0", manifestVersion: "1.0.0", cliVersion: "2.0.0", want: doctorFail},
		{name: "current CLI mismatch", recordVersion: "2.0.0", manifestVersion: "2.0.0", cliVersion: "3.0.0", want: doctorFail},
		{name: "untrusted CLI fallback omitted", recordVersion: "2.0.0", manifestVersion: "2.0.0", cliVersion: "", want: doctorPass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := writeDoctorFixture(t, "unrelated-project-version")
			home := os.Getenv("HOME")
			config := filepath.Join(root, "recorded", "opencode")
			artifact := writeDoctorConcreteAdapterArtifact(t, config, "plugin:hooks", "plugin", "plugins/hooks.ts", "plugins/hooks.ts", "plugin\n", 0o644)
			writeDoctorOwnershipManifest(t, config, "opencode", tc.manifestVersion, []targetAdapterArtifact{artifact})
			writeDoctorInstallRecord(t, home, installTargetRecord{Version: tc.recordVersion, Target: "opencode", ConfigDir: config})
			result := checkTargetAdapterOwnership(tc.cliVersion).Run(doctorContext{projectRoot: root})
			if result.Status != tc.want {
				t.Fatalf("status = %s, detail = %s, want %s", result.Status, result.Detail, tc.want)
			}
			if tc.cliVersion == "" && strings.Contains(result.Detail, "doctor=") {
				t.Fatalf("untrusted CLI version appeared in detail: %s", result.Detail)
			}
		})
	}
}

func TestRunnerDoctorFixDoesNotMutateTargetAdapterBytesOrMode(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	home := os.Getenv("HOME")
	config := filepath.Join(root, "recorded", "opencode")
	artifact := writeDoctorConcreteAdapterArtifact(t, config, "plugin:hooks", "plugin", "plugins/hooks.ts", "plugins/hooks.ts", "locally modified\n", 0o755)
	artifact.SHA256 = sha256Bytes([]byte("expected\n"))
	manifestPath := filepath.Join(config, targetInstallManifestFile)
	writeDoctorOwnershipManifest(t, config, "opencode", "9.8.7-test.1", []targetAdapterArtifact{artifact})
	writeDoctorInstallRecord(t, home, installTargetRecord{Version: "9.8.7-test.1", Target: "opencode", ConfigDir: config})
	artifactPath := filepath.Join(config, "plugins", "hooks.ts")
	beforeArtifact := append([]byte(nil), readFileBytes(t, artifactPath)...)
	beforeManifest := append([]byte(nil), readFileBytes(t, manifestPath)...)
	beforeInfo, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err = (Runner{Stdout: &stdout, WorkingDir: root}).Run([]string{"doctor", "--fix"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("doctor --fix error = %v, output = %s", err, stdout.String())
	}
	afterInfo, err := os.Stat(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(readFileBytes(t, artifactPath), beforeArtifact) || !bytes.Equal(readFileBytes(t, manifestPath), beforeManifest) || afterInfo.Mode().Perm() != beforeInfo.Mode().Perm() {
		t.Fatal("doctor --fix mutated target adapter bytes or permission mode")
	}
	check := checkTargetAdapterOwnership("9.8.7-test.1")
	if check.Fix != nil || check.Run(doctorContext{projectRoot: root}).Fixable {
		t.Fatal("target adapter ownership check unexpectedly exposes a fix path")
	}
}

func TestRunnerDoctorWarningsDoNotFail(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("0.0.1-test"))
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	symlinkFile(t, "../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor"})
	if err != nil {
		t.Fatalf("doctor warning-only error = %v, want nil", err)
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "Fenced section version drift") || strings.Contains(output, "failed") {
		t.Fatalf("doctor warning output = %q, want warning-only success", output)
	}
}

func TestRunnerDoctorFixMigratesLegacyFiles(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorFile(t, filepath.Join(root, "AGENTS.md"), "# Root Notes\n\nroot context\n")
	writeDoctorFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# Claude Notes\n\nclaude context\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor", "--fix"})
	if err != nil {
		t.Fatalf("doctor --fix legacy migration error = %v", err)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	for _, want := range []string{"root context", "claude context", "## Migrated from AGENTS.md", "## Migrated from .claude/CLAUDE.md"} {
		if !strings.Contains(canonical, want) {
			t.Fatalf("canonical = %q, want %q", canonical, want)
		}
	}
	assertSymlinkTarget(t, filepath.Join(root, "AGENTS.md"), ".agents/AGENTS.md")
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../.agents/AGENTS.md")
	for _, path := range []string{"AGENTS.md.bak", ".claude/CLAUDE.md.bak"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("doctor backup %s missing: %v", path, err)
		}
	}
	output := stripANSI(stdout.String())
	if !strings.Contains(output, "fixed") || strings.Contains(output, "failed") {
		t.Fatalf("doctor --fix output = %q, want fixes and clean report", output)
	}
}

func TestRunnerDoctorFixStripsDuplicateFencedSection(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	writeDoctorFile(t, filepath.Join(root, "AGENTS.md"), "# root\n")
	writeDoctorFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# claude\n\n<!-- loaf:managed:start v0.1.0 -->\nold fence\n<!-- loaf:managed:end -->\n\ntrailing text\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"doctor", "--fix"})
	if err != nil {
		t.Fatalf("doctor --fix duplicate fence error = %v", err)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	if strings.Contains(canonical, "old fence") {
		t.Fatalf("canonical = %q, want old managed fence stripped", canonical)
	}
	if !strings.Contains(canonical, "trailing text") {
		t.Fatalf("canonical = %q, want user text preserved", canonical)
	}
	assertSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), "../.agents/AGENTS.md")
}

func TestRunnerDoctorVerboseAndHelpAreNative(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	writeDoctorAgents(t, root, doctorFence("9.8.7-test.1"))
	symlinkFile(t, ".agents/AGENTS.md", filepath.Join(root, "AGENTS.md"))
	symlinkFile(t, "../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md"))

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "verbose", args: []string{"doctor", "--verbose"}, want: "agents-symlink"},
		{name: "help", args: []string{"doctor", "--help"}, want: "Usage: loaf doctor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout bytes.Buffer
			err := Runner{
				Stdout:     &stdout,
				WorkingDir: root,
			}.Run(tc.args)
			if err != nil {
				t.Fatalf("%v error = %v", tc.args, err)
			}
			if output := stripANSI(stdout.String()); !strings.Contains(output, tc.want) {
				t.Fatalf("%v output = %q, want %q", tc.args, output, tc.want)
			}
		})
	}
}

func TestRunnerDoctorRejectsUnknownOptionsNatively(t *testing.T) {
	root := writeDoctorFixture(t, "9.8.7-test.1")
	err := Runner{
		WorkingDir: root,
	}.Run([]string{"doctor", "--wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown doctor option") {
		t.Fatalf("doctor unknown option error = %v, want native option error", err)
	}
}

func writeDoctorFixture(t *testing.T, version string) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("PATH", t.TempDir())
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"`+version+`"}`+"\n")
	return root
}

func writeDoctorAgents(t *testing.T, root string, body string) {
	t.Helper()
	writeDoctorFile(t, filepath.Join(root, ".agents", "AGENTS.md"), body)
}

func doctorFence(version string) string {
	return strings.Join([]string{
		"<!-- loaf:managed:start v" + version + " -->",
		"<!-- Maintained by loaf install/upgrade - do not edit manually -->",
		"## Loaf Framework",
		"",
		"Sample fenced content.",
		"<!-- loaf:managed:end -->",
		"",
	}, "\n")
}

func symlinkFile(t *testing.T, target string, linkPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("Symlink(%s, %s) error = %v", target, linkPath, err)
	}
}

func writeDoctorFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, body)
}

func writeDoctorInstallRecord(t *testing.T, home string, record installTargetRecord) {
	t.Helper()
	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal install record error = %v", err)
	}
	writeDoctorFile(t, installRecordPath(home, record.Target), string(body)+"\n")
}

func writeDoctorConcreteAdapterArtifact(t *testing.T, config string, id string, kind string, sourcePath string, destination string, body string, mode os.FileMode) targetAdapterArtifact {
	t.Helper()
	path := filepath.Join(config, filepath.FromSlash(destination))
	writeDoctorFile(t, path, body)
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("Chmod(%s) error = %v", path, err)
	}
	manifestMode := uint32(mode.Perm())
	return targetAdapterArtifact{ID: id, Kind: kind, SourcePath: sourcePath, Destination: destination, SHA256: sha256Bytes([]byte(body)), Mode: &manifestMode}
}

func writeDoctorOwnershipManifest(t *testing.T, config string, target string, version string, artifacts []targetAdapterArtifact) {
	t.Helper()
	instruction := targetAdapterArtifact{ID: "managed-instructions", Kind: "instruction", Destination: "project-instructions", SHA256: sha256Bytes([]byte("instruction"))}
	manifest := targetAdapterManifest{Version: 1, Target: target, PackageVersion: version, CapabilityContractVersion: TargetCapabilityEvidenceContractVersion, Adapters: []string{target + "-doctor-v1"}, Artifacts: append([]targetAdapterArtifact{instruction}, artifacts...)}
	if err := writeTargetAdapterManifest(filepath.Join(config, targetInstallManifestFile), manifest); err != nil {
		t.Fatalf("writeTargetAdapterManifest error = %v", err)
	}
}

func assertSymlinkTarget(t *testing.T, linkPath string, want string) {
	t.Helper()
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("Lstat(%s) error = %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", linkPath)
	}
	got, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s) error = %v", linkPath, err)
	}
	if got != want {
		t.Fatalf("Readlink(%s) = %q, want %q", linkPath, got, want)
	}
}

func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return body
}

func stripANSI(value string) string {
	return regexpANSI.ReplaceAllString(value, "")
}
