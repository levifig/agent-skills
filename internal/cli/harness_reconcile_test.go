package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHarnessReconcileUpgradesProjectBoundAmpContentFromRunningDistribution(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	t.Setenv(projectEnvironmentEnv, "1")
	writeInstallFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"0.5.0"}`+"\n")
	configDir := filepath.Join(root, ".amp")
	distDir := filepath.Join(root, "dist", "amp")
	sharedSkills := filepath.Join(root, ".agents", "skills")

	writeInstallFile(t, filepath.Join(distDir, "skills", "linear", "SKILL.md"), "linear 0.5.0\n")
	writeInstallFile(t, filepath.Join(distDir, ".amp", "plugins", "loaf.ts"), "plugin 0.5.0\n")
	writeTestTargetAdapterManifest(t, distDir, "amp", []map[string]string{{
		"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("plugin 0.5.0\n"),
	}})

	writeInstallFile(t, filepath.Join(sharedSkills, "linear", "SKILL.md"), "linear 0.3.1\n")
	oldSkillDigest, err := hashInstallSkillTree(filepath.Join(sharedSkills, "linear"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManagedSkillsManifest(sharedSkills, managedSkillsManifestV2{Version: 2, Skills: []managedSkillDigest{{Name: "linear", SHA256: oldSkillDigest}}}); err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(configDir, "plugins", "loaf.ts"), "plugin 0.3.1\n")
	writeInstallFile(t, filepath.Join(configDir, "plugins", "user.ts"), "user-owned plugin\n")
	oldMode := uint32(0o644)
	oldManifest := targetAdapterManifest{
		Version: 1, Target: "amp", PackageVersion: "0.3.1", CapabilityContractVersion: 3,
		Adapters: []string{"amp-plugin-v1"},
		Artifacts: []targetAdapterArtifact{{
			ID: "plugin:.amp/plugins/loaf.ts", Kind: "plugin", SourcePath: ".amp/plugins/loaf.ts", Destination: "plugins/loaf.ts", SHA256: sha256Hex("plugin 0.3.1\n"), Mode: &oldMode,
		}},
	}
	if err := writeTargetAdapterManifest(filepath.Join(configDir, targetInstallManifestFile), oldManifest); err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), "0.3.1\n")
	writeInstallFile(t, installRecordPath(root, "amp"), `{"version":"0.3.1","target":"amp","config_dir":"`+configDir+`","skills_dir":"`+sharedSkills+`"}`+"\n")
	writeInstallFile(t, filepath.Join(root, "AGENTS.md"), "user-owned project instructions\n")

	var stdout bytes.Buffer
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"harness", "reconcile", "--target", "amp", "--json"})
	if err != nil {
		t.Fatalf("harness reconcile error = %v\n%s", err, stdout.String())
	}
	var receipt harnessReconcileReceipt
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatalf("decode receipt: %v\n%s", err, stdout.String())
	}
	if receipt.Outcome != "updated" || receipt.FromVersion != "0.3.1" || receipt.ToVersion != "0.5.0" || receipt.Target != "amp" {
		t.Fatalf("receipt = %#v", receipt)
	}
	if !receipt.RestartRequired || !strings.Contains(receipt.Message, "new session") {
		t.Fatalf("receipt = %#v, want truthful loaded-byte restart diagnostic", receipt)
	}
	assertInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), "0.5.0\n")
	assertInstallFile(t, filepath.Join(configDir, "plugins", "loaf.ts"), "plugin 0.5.0\n")
	assertInstallFile(t, filepath.Join(configDir, "plugins", "user.ts"), "user-owned plugin\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "linear", "SKILL.md"), "linear 0.5.0\n")
	assertInstallFile(t, filepath.Join(root, "AGENTS.md"), "user-owned project instructions\n")
	if !installFileExists(filepath.Join(root, ".agents", "skills", "linear", "SKILL.md")) {
		t.Fatal("automatic reconcile did not update project-local managed skills")
	}
	if installFileExists(filepath.Join(home, ".agents", "skills", "linear", "SKILL.md")) {
		t.Fatal("project-bound reconcile wrote user-global skills")
	}
	if !installFileExists(filepath.Join(configDir, harnessReconcileReceiptFile)) {
		t.Fatal("durable reconcile receipt missing")
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"harness", "reconcile", "--target", "amp", "--json"})
	if err != nil {
		t.Fatalf("second reconcile error = %v\n%s", err, stdout.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil || receipt.Outcome != "current" {
		t.Fatalf("second receipt = %#v, err=%v", receipt, err)
	}
	assertInstallFile(t, filepath.Join(configDir, "plugins", "user.ts"), "user-owned plugin\n")
}

func TestHarnessReconcileNeverDowngradesOrClaimsUnknownContent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		marker string
		want   string
	}{
		{name: "newer", marker: "99.0.0\n", want: "binary-stale"},
		{name: "unreadable", marker: "mystery\n", want: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			configDir := filepath.Join(home, ".config", "amp")
			writeInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), tc.marker)
			writeInstallFile(t, filepath.Join(configDir, "plugins", "loaf.ts"), "user bytes\n")
			before := string(readFileBytes(t, filepath.Join(configDir, "plugins", "loaf.ts")))
			var stdout bytes.Buffer
			err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"harness", "reconcile", "--target", "amp", "--json"})
			if err != nil {
				t.Fatalf("reconcile error = %v\n%s", err, stdout.String())
			}
			var receipt harnessReconcileReceipt
			if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil || receipt.Outcome != tc.want {
				t.Fatalf("receipt = %#v err=%v", receipt, err)
			}
			if after := string(readFileBytes(t, filepath.Join(configDir, "plugins", "loaf.ts"))); after != before {
				t.Fatalf("plugin changed from %q to %q", before, after)
			}
		})
	}
}

func TestHarnessReconcileConflictLeavesAdapterAndMarkerRetryable(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"0.5.0"}`+"\n")
	configDir := filepath.Join(home, ".config", "amp")
	distDir := filepath.Join(root, "dist", "amp")
	sharedSkills := filepath.Join(home, ".agents", "skills")

	writeInstallFile(t, filepath.Join(distDir, "skills", "linear", "SKILL.md"), "linear 0.5.0\n")
	writeInstallFile(t, filepath.Join(distDir, ".amp", "plugins", "loaf.ts"), "plugin 0.5.0\n")
	writeTestTargetAdapterManifest(t, distDir, "amp", []map[string]string{{
		"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("plugin 0.5.0\n"),
	}})
	writeInstallFile(t, filepath.Join(sharedSkills, "linear", "SKILL.md"), "linear 0.3.1\n")
	oldDigest, err := hashInstallSkillTree(filepath.Join(sharedSkills, "linear"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManagedSkillsManifest(sharedSkills, managedSkillsManifestV2{Version: 2, Skills: []managedSkillDigest{{Name: "linear", SHA256: oldDigest}}}); err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(sharedSkills, "linear", "SKILL.md"), "user-modified\n")
	writeInstallFile(t, filepath.Join(configDir, "plugins", "loaf.ts"), "plugin 0.3.1\n")
	mode := uint32(0o644)
	if err := writeTargetAdapterManifest(filepath.Join(configDir, targetInstallManifestFile), targetAdapterManifest{
		Version: 1, Target: "amp", PackageVersion: "0.3.1", CapabilityContractVersion: 3, Adapters: []string{"amp-plugin-v1"},
		Artifacts: []targetAdapterArtifact{{ID: "plugin:.amp/plugins/loaf.ts", Kind: "plugin", SourcePath: ".amp/plugins/loaf.ts", Destination: "plugins/loaf.ts", SHA256: sha256Hex("plugin 0.3.1\n"), Mode: &mode}},
	}); err != nil {
		t.Fatal(err)
	}
	writeInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), "0.3.1\n")
	writeInstallFile(t, installRecordPath(home, "amp"), `{"version":"0.3.1","target":"amp","config_dir":"`+configDir+`","skills_dir":"`+sharedSkills+`"}`+"\n")

	var conflictOutput bytes.Buffer
	err = Runner{Stdout: &conflictOutput, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"harness", "reconcile", "--target", "amp", "--json"})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 || !strings.Contains(conflictOutput.String(), "managed skills sync conflicts") {
		t.Fatalf("conflict error = %v output=%q, want JSON detail and exit 1", err, conflictOutput.String())
	}
	assertInstallFile(t, filepath.Join(configDir, "plugins", "loaf.ts"), "plugin 0.3.1\n")
	assertInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), "0.3.1\n")

	writeInstallFile(t, filepath.Join(sharedSkills, "linear", "SKILL.md"), "linear 0.3.1\n")
	err = Runner{Stdout: &bytes.Buffer{}, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"harness", "reconcile", "--target", "amp", "--json"})
	if err != nil {
		t.Fatalf("retry reconcile error = %v", err)
	}
	assertInstallFile(t, filepath.Join(configDir, "plugins", "loaf.ts"), "plugin 0.5.0\n")
	assertInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), "0.5.0\n")
}

func TestHarnessReconcileUpdatesInstalledAmpOpenCodeCohortFromCanonicalSkills(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"0.5.0"}`+"\n")
	configs := defaultInstallConfigDirsForHome(home)
	sharedSkills := filepath.Join(home, ".agents", "skills")
	ampDist := filepath.Join(root, "dist", "amp")
	openCodeDist := filepath.Join(root, "dist", "opencode")

	writeInstallFile(t, filepath.Join(ampDist, "skills", "foundations", "SKILL.md"), canonicalSkillFixtureBody(false))
	writeInstallFile(t, filepath.Join(openCodeDist, "skills", "foundations", "SKILL.md"), canonicalSkillFixtureBody(true))
	writeInstallFile(t, filepath.Join(ampDist, ".amp", "plugins", "loaf.ts"), "plugin 0.5.0\n")
	writeTestTargetAdapterManifest(t, ampDist, "amp", []map[string]string{{
		"id": "plugin:.amp/plugins/loaf.ts", "kind": "plugin", "source_path": ".amp/plugins/loaf.ts", "destination": "plugins/loaf.ts", "sha256": sha256Hex("plugin 0.5.0\n"),
	}})
	writeTestTargetAdapterManifest(t, openCodeDist, "opencode", nil)

	writeInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), canonicalSkillFixtureBody(false))
	oldDigest, err := hashInstallSkillTree(filepath.Join(sharedSkills, "foundations"))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManagedSkillsManifest(sharedSkills, managedSkillsManifestV2{Version: 2, Skills: []managedSkillDigest{{Name: "foundations", SHA256: oldDigest}}}); err != nil {
		t.Fatal(err)
	}
	mode := uint32(0o644)
	writeInstallFile(t, filepath.Join(configs["amp"], "plugins", "loaf.ts"), "plugin 0.3.1\n")
	if err := writeTargetAdapterManifest(filepath.Join(configs["amp"], targetInstallManifestFile), targetAdapterManifest{
		Version: 1, Target: "amp", PackageVersion: "0.3.1", CapabilityContractVersion: 3, Adapters: []string{"amp-plugin-v1"},
		Artifacts: []targetAdapterArtifact{{ID: "plugin:.amp/plugins/loaf.ts", Kind: "plugin", SourcePath: ".amp/plugins/loaf.ts", Destination: "plugins/loaf.ts", SHA256: sha256Hex("plugin 0.3.1\n"), Mode: &mode}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeTargetAdapterManifest(filepath.Join(configs["opencode"], targetInstallManifestFile), targetAdapterManifest{
		Version: 1, Target: "opencode", PackageVersion: "0.3.1", CapabilityContractVersion: 3, Adapters: []string{"opencode-plugin-v1"},
		Artifacts: []targetAdapterArtifact{{ID: "managed-instructions", Kind: "instruction", Destination: "project-instructions", SHA256: fencedContentFingerprint(generateFencedContent())}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"amp", "opencode"} {
		writeInstallFile(t, filepath.Join(configs[target], loafInstallMarkerFile), "0.3.1\n")
		writeInstallFile(t, installRecordPath(home, target), `{"version":"0.3.1","target":"`+target+`","config_dir":"`+configs[target]+`","skills_dir":"`+sharedSkills+`"}`+"\n")
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"harness", "reconcile", "--target", "amp", "--json"}); err != nil {
		t.Fatalf("cohort reconcile error = %v\n%s", err, stdout.String())
	}
	var receipt harnessReconcileReceipt
	if err := json.Unmarshal(stdout.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if strings.Join(receipt.UpdatedTargets, ",") != "amp,opencode" && strings.Join(receipt.UpdatedTargets, ",") != "opencode,amp" {
		t.Fatalf("updated targets = %#v, want amp and opencode", receipt.UpdatedTargets)
	}
	assertInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), canonicalSkillFixtureBody(true))
	assertInstallFile(t, filepath.Join(configs["amp"], loafInstallMarkerFile), "0.5.0\n")
	assertInstallFile(t, filepath.Join(configs["opencode"], loafInstallMarkerFile), "0.5.0\n")
}

func TestHarnessReconcileLockContentionIsVisibleAndNonMutating(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	configDir := filepath.Join(home, ".config", "amp")
	writeInstallFile(t, filepath.Join(configDir, loafInstallMarkerFile), "0.3.1\n")
	held, err := acquireHarnessReconcileLock(managedContentLockDir(home), 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer held.release()
	var stdout bytes.Buffer
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"harness", "reconcile", "--target", "amp", "--json"})
	if err == nil || !strings.Contains(stdout.String(), "another Loaf process") {
		t.Fatalf("error = %v output=%q, want visible contention", err, stdout.String())
	}
	if _, statErr := os.Stat(filepath.Join(managedContentLockDir(home), harnessReconcileLockFile)); statErr != nil {
		t.Fatalf("foreign lock changed: %v", statErr)
	}
}

func TestHarnessReconcileIgnoresUnlockedHistoricalLockMetadata(t *testing.T) {
	_, home := setupInstallCommandFixture(t)
	configDir := filepath.Join(home, ".config", "amp")
	lockPath := filepath.Join(configDir, harnessReconcileLockFile)
	writeInstallFile(t, lockPath, "opaque abandoned holder\n")
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireHarnessReconcileLock(configDir, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire after expiration: %v", err)
	}
	if err := lock.release(); err != nil {
		t.Fatal(err)
	}
	if !installFileExists(lockPath) {
		t.Fatal("advisory lock metadata file should remain reusable")
	}
}

func TestTwoStaleReclaimersCannotReplaceAnActiveOSLock(t *testing.T) {
	_, home := setupInstallCommandFixture(t)
	configDir := filepath.Join(home, ".config", "amp")
	lockPath := filepath.Join(configDir, harnessReconcileLockFile)
	first, err := acquireHarnessReconcileLock(configDir, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireHarnessReconcileLock(configDir, 40*time.Millisecond); err == nil || !strings.Contains(err.Error(), "another Loaf process") {
		t.Fatalf("second reclaimer error = %v, want active OS lock contention", err)
	}
	if _, err := acquireHarnessReconcileLock(configDir, 40*time.Millisecond); err == nil || !strings.Contains(err.Error(), "another Loaf process") {
		t.Fatalf("third reclaimer error = %v, want active OS lock contention", err)
	}
	if err := first.release(); err != nil {
		t.Fatal(err)
	}
	successor, err := acquireHarnessReconcileLock(configDir, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("acquire after OS lock release: %v", err)
	}
	if err := successor.release(); err != nil {
		t.Fatal(err)
	}
}

func TestHarnessDriftReportsDurableReconcileReceipt(t *testing.T) {
	configDir := t.TempDir()
	receipt := harnessReconcileReceipt{ContractVersion: 1, Target: "amp", FromVersion: "0.3.1", ToVersion: "0.5.0", Outcome: "updated", RecordedAt: "2026-08-31T00:00:00Z", RestartRequired: true}
	if err := writeHarnessReconcileReceipt(configDir, receipt); err != nil {
		t.Fatal(err)
	}
	reading := harnessDriftReading{target: "amp", name: "Amp", configDir: configDir}
	detail := reading.reconcileReceiptDetailLine()
	for _, want := range []string{"0.3.1 → 0.5.0", "outcome updated", "restart_required=true"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail = %q, want %q", detail, want)
		}
	}
}
