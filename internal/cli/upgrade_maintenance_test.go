package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// upgrade_maintenance_test.go holds the maintenance half of `loaf upgrade`:
// harness content sync for already-installed targets, deprecation-manifest
// cleanup, path relocations, and the legacy project-instruction migration.
// These cases arrived with the maintenance flag install used to carry; they
// moved here with the command that owns the work.

func TestRunnerUpgradeOnlyRefreshesDetectedLoafTargets(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	mkdirAll(t, filepath.Join(home, ".config", "opencode"))
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Upgrading:") || !strings.Contains(stdout.String(), "cursor") || !strings.Contains(stdout.String(), "Cursor refreshed") {
		t.Fatalf("stdout = %q, want cursor-only upgrade", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", loafInstallMarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("opencode marker stat = %v, want not installed during upgrade", err)
	}
}

func TestRunnerUpgradeMigratesLegacyProjectInstructionLayout(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(root, "bin", "claude"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(root, "bin", "claude"), 0o755); err != nil {
		t.Fatalf("Chmod(fake claude) error = %v", err)
	}
	// The legacy layout carries no managed marker, so the project config is what
	// tells the detector this is a Loaf repo and opens upgrade's project part.
	writeInstallFile(t, filepath.Join(root, ".agents", "loaf.json"), "{\"integrations\":{}}\n")
	writeInstallFile(t, filepath.Join(root, ".agents", "AGENTS.md"), "# Legacy Instructions\n")
	if err := os.Symlink(".agents/AGENTS.md", filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatalf("Symlink legacy root AGENTS.md error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.claude) error = %v", err)
	}
	if err := os.Symlink("../.agents/AGENTS.md", filepath.Join(root, ".claude", "CLAUDE.md")); err != nil {
		t.Fatalf("Symlink legacy CLAUDE.md error = %v", err)
	}

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	canonical := filepath.Join(root, "AGENTS.md")
	if installIsSymlink(canonical) {
		t.Fatal("root AGENTS.md remains a symlink after upgrade")
	}
	body := string(readFileBytes(t, canonical))
	if !strings.Contains(body, "# Legacy Instructions") || !strings.Contains(body, "<!-- loaf:managed:start sha256=") || strings.Contains(body, "<!-- loaf:managed:start v") {
		t.Fatalf("root AGENTS.md = %q, want preserved legacy content and sha256-only managed fence", body)
	}
	if _, err := os.Lstat(filepath.Join(root, ".agents", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy .agents/AGENTS.md stat = %v, want absent", err)
	}
	assertInstallSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), canonical)
}

func TestRunnerUpgradeDetectsLegacyAmpWithoutMutatingLegacyPath(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	legacyAmp := filepath.Join(home, ".amp")
	legacyPlugin := filepath.Join(legacyAmp, "plugins", "loaf.ts")
	writeInstallFile(t, filepath.Join(legacyAmp, loafInstallMarkerFile), "legacy\n")
	writeInstallFile(t, legacyPlugin, "legacy plugin\n")
	writeInstallFile(t, filepath.Join(root, "dist", "amp", ".amp", "plugins", "loaf.ts"), "current plugin\n")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Upgrading:") || !strings.Contains(stdout.String(), "amp") || !strings.Contains(stdout.String(), "Amp refreshed") {
		t.Fatalf("stdout = %q, want legacy Amp upgrade selection", stdout.String())
	}
	currentConfig := filepath.Join(home, ".config", "amp")
	assertInstallFile(t, filepath.Join(currentConfig, loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(currentConfig, "plugins", "loaf.ts"), "current plugin\n")
	assertInstallFile(t, filepath.Join(legacyAmp, loafInstallMarkerFile), "legacy\n")
	assertInstallFile(t, legacyPlugin, "legacy plugin\n")
	record := readInstallCommandJSON(t, installRecordPath(home, "amp"))
	if record["config_dir"] != currentConfig {
		t.Fatalf("amp install record = %#v, want current config dir %q", record, currentConfig)
	}
}

func TestRunnerUpgradeRelocatesOpenCodeAndAmpSkillHomes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		target      string
		oldSkills   func(home string) string
		ownerMarker func(home string) string
	}{
		{
			name:        "opencode",
			target:      "opencode",
			oldSkills:   func(home string) string { return filepath.Join(home, ".config", "opencode", "skills") },
			ownerMarker: func(home string) string { return filepath.Join(home, ".config", "opencode", loafInstallMarkerFile) },
		},
		{
			name:        "amp",
			target:      "amp",
			oldSkills:   func(home string) string { return filepath.Join(home, ".config", "agents", "skills") },
			ownerMarker: func(home string) string { return filepath.Join(home, ".config", "amp", loafInstallMarkerFile) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			oldSkills := tc.oldSkills(home)
			ownerMarker := tc.ownerMarker(home)
			writeInstallFile(t, ownerMarker, "old\n")
			seedOwnedManagedSkill(t, oldSkills, "foundations", "# Old foundations\n")
			writeInstallFile(t, filepath.Join(root, "dist", tc.target, "skills", "go-development", "SKILL.md"), "# Go\n")
			writeInstallDeprecationManifest(t, root, fmt.Sprintf(`{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "relocations": [
    {
      "id": "%s-skills-to-agents-home",
      "from": %q,
      "to": "${HOME}/.agents/skills",
      "owner_marker": %q,
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "%s skills moved to ~/.agents/skills"
    }
  ],
  "aliases": []
}`, tc.target, oldSkills, ownerMarker, tc.target))

			var stdout bytes.Buffer
			err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
			if err != nil {
				t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
			}
			assertInstallPathMissing(t, oldSkills)
			// Claim transfer lets canonical sync own the destination. foundations is
			// not in dist, so sync retires it as a stale managed skill; go-development
			// is installed fresh.
			assertInstallPathMissing(t, filepath.Join(home, ".agents", "skills", "foundations"))
			assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
			if !strings.Contains(stdout.String(), "relocated path "+tc.target+"-skills-to-agents-home") {
				t.Fatalf("stdout = %q, want relocation report", stdout.String())
			}
		})
	}
}

func TestRunnerUpgradeCleansRetiredTargetFromManifest(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	retiredTarget := filepath.Join(home, ".retired-tool")
	writeInstallFile(t, filepath.Join(retiredTarget, loafInstallMarkerFile), "old\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "retired-tool",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "retired by test manifest",
      "paths": ["${HOME}/.retired-tool"]
    }
  ],
  "retired_skills": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	if _, err := os.Stat(retiredTarget); !os.IsNotExist(err) {
		t.Fatalf("retired target stat = %v, want removed", err)
	}
	for _, want := range []string{"install deprecation cleanup", "removed retired target retired-tool", "retired by test manifest", "since v9.9.0, window one-release"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

// TestRunnerUpgradePreservesForeignRetiredTargetHomeWithoutReintroducingIt proves
// a marked retired target that still holds foreign content is not RemoveAll'd,
// and that the retired target name is not reintroduced as an install target.
// Named for what it asserts: preservation of foreign content, not "cleaning".
func TestRunnerUpgradePreservesForeignRetiredTargetHomeWithoutReintroducingIt(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	geminiHome := filepath.Join(home, ".gemini")
	writeInstallFile(t, filepath.Join(geminiHome, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(geminiHome, "user-settings.json"), "{\"theme\":\"dark\"}\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "gemini",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "gemini retired",
      "paths": ["${HOME}/.gemini"]
    }
  ],
  "retired_skills": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(geminiHome, "user-settings.json"), "{\"theme\":\"dark\"}\n")
	if isValidInstallTarget("gemini") {
		t.Fatal("gemini target was reintroduced")
	}
	if strings.Contains(stdout.String(), "removed retired target gemini") {
		t.Fatalf("stdout = %q, must not RemoveAll gemini home with foreign content", stdout.String())
	}
	if !strings.Contains(stdout.String(), "un-managed") {
		t.Fatalf("stdout = %q, want un-managed gemini report", stdout.String())
	}
}

func TestRunnerUpgradeCleansRetiredSkillFromManifest(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	retiredSkill := filepath.Join(skillHome, "old-skill")
	seedOwnedManagedSkill(t, skillHome, "old-skill", "# Old skill\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "old-skill",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "old-skill was retired",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	if _, err := os.Stat(retiredSkill); !os.IsNotExist(err) {
		t.Fatalf("retired skill stat = %v, want removed", err)
	}
	for _, want := range []string{"removed retired skill old-skill", "old-skill was retired"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerUpgradeSkipsDestructiveDeprecationWithoutExplicitYes(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	retiredSkill := filepath.Join(home, ".agents", "skills", "old-skill")
	writeInstallFile(t, filepath.Join(retiredSkill, "SKILL.md"), "# Old skill\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "old-skill",
      "since": "v9.9.0",
      "reason": "old-skill was retired",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(retiredSkill, "SKILL.md"), "# Old skill\n")
	for _, want := range []string{"skipped skill old-skill", "rerun with --yes to apply destructive deprecation cleanup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerUpgradeCleansRetiredAgentFromManifest(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	agentHome := filepath.Join(home, ".cursor", "agents")
	retiredAgent := filepath.Join(agentHome, "old-agent.md")
	// The marker makes Cursor look installed, so the sync runs and must have
	// something to sync; without it the run ends in a target failure this test
	// is not about.
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, retiredAgent, "# Old Agent\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [
    {
      "agent": "old-agent",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "old-agent was retired",
      "agent_homes": ["${HOME}/.cursor/agents"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, retiredAgent, "# Old Agent\n")
	if strings.Contains(stdout.String(), "removed retired agent old-agent") {
		t.Fatalf("stdout = %q, must not delete agent on marker alone", stdout.String())
	}
	if !strings.Contains(stdout.String(), "un-managed") || !strings.Contains(stdout.String(), "old-agent") {
		t.Fatalf("stdout = %q, want un-managed agent report", stdout.String())
	}
}

func TestRunnerUpgradeSkipsUnmarkedRetiredAgent(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	agentHome := filepath.Join(home, ".cursor", "agents")
	retiredAgent := filepath.Join(agentHome, "old-agent.md")
	writeInstallFile(t, retiredAgent, "# User-owned Agent\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [
    {
      "agent": "old-agent",
      "since": "v9.9.0",
      "reason": "old-agent was retired",
      "agent_homes": ["${HOME}/.cursor/agents"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, retiredAgent, "# User-owned Agent\n")
	out := stdout.String()
	if strings.Contains(out, "path is not marked as Loaf-owned") {
		t.Fatalf("stdout = %q, unowned agent must not be reported", out)
	}
	if strings.Contains(out, "install deprecation cleanup") && strings.Contains(out, "old-agent") {
		t.Fatalf("stdout = %q, unowned agent must not appear in deprecation report", out)
	}
}

func TestRunnerUpgradeReportsDefaultDeprecationWindow(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	seedOwnedManagedSkill(t, skillHome, "old-skill", "# Old skill\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "old-skill",
      "since": "v9.9.0",
      "reason": "old-skill was retired",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "since v9.9.0, window one-release") {
		t.Fatalf("stdout = %q, want default deprecation window", stdout.String())
	}
}

func TestRunnerUpgradeReportsDeprecationSignoff(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	seedOwnedManagedSkill(t, skillHome, "old-skill", "# Old skill\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "old-skill",
      "since": "v9.9.0",
      "reason": "old-skill was retired",
      "signoff": "report-spec-053-taxonomy-signoff",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"removed retired skill old-skill", "[signoff: report-spec-053-taxonomy-signoff]"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerUpgradeReportsAliasTombstoneFromManifest(t *testing.T) {
	root, _ := setupInstallCommandFixture(t)
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [],
  "aliases": [
    {
      "from": "old-skill",
      "to": "new-skill",
      "since": "v9.9.0",
      "reason": "old-skill now routes to new-skill",
      "signoff": "report-spec-053-taxonomy-signoff"
    }
  ]
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		"install deprecation cleanup",
		"alias old-skill -> new-skill",
		"old-skill now routes to new-skill",
		"since v9.9.0, window one-release",
		"[signoff: report-spec-053-taxonomy-signoff]",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerUpgradeReportsExternalizedSkillWithoutRemoving(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	externalizedSkill := filepath.Join(home, ".agents", "skills", "vendor-skill")
	writeInstallFile(t, filepath.Join(externalizedSkill, "SKILL.md"), "# Vendor skill\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "externalized_skills": [
    {
      "skill": "vendor-skill",
      "since": "v9.9.0",
      "reason": "vendor-skill moved out of Loaf core",
      "signoff": "report-spec-053-taxonomy-signoff",
      "source": "https://github.com/example/skills/tree/main/skills/vendor-skill",
      "install_command": "loaf skill add https://github.com/example/skills/tree/main/skills/vendor-skill",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(externalizedSkill, "SKILL.md"), "# Vendor skill\n")
	for _, want := range []string{
		"install deprecation cleanup",
		"externalized skill vendor-skill",
		"vendor-skill moved out of Loaf core",
		"source: https://github.com/example/skills/tree/main/skills/vendor-skill",
		"command: loaf skill add https://github.com/example/skills/tree/main/skills/vendor-skill",
		"[signoff: report-spec-053-taxonomy-signoff]",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerUpgradeSkipsUnmarkedRetiredTarget(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	retiredTarget := filepath.Join(home, ".unmarked-tool")
	writeInstallFile(t, filepath.Join(retiredTarget, "user-file.txt"), "keep me\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "unmarked-tool",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "retired by test manifest",
      "paths": ["${HOME}/.unmarked-tool"]
    }
  ],
  "retired_skills": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(retiredTarget, "user-file.txt"), "keep me\n")
	out := stdout.String()
	if strings.Contains(out, "path is not marked as Loaf-owned") {
		t.Fatalf("stdout = %q, unowned target must not be reported", out)
	}
	if strings.Contains(out, "install deprecation cleanup") && strings.Contains(out, "unmarked-tool") {
		t.Fatalf("stdout = %q, unowned target must not appear in deprecation report", out)
	}
}

func TestRunnerUpgradeRelocatesManifestPathExactlyOnce(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "skills moved to ~/.agents/skills"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, oldPath)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	if !strings.Contains(stdout.String(), "relocated path old-agents-skills") {
		t.Fatalf("stdout = %q, want relocation report", stdout.String())
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("second upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, oldPath)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
}

// TestRunnerUpgradeRelocatesOwnedSkillWhenDestinationHomeExistsButSkillAbsent
// covers the common case: ~/.agents/skills already exists (prior install) but
// does not hold the skill being relocated. Existence of the destination home
// is not proof the skill is already there — the source must be moved.

func TestRunnerUpgradeRemovesStaleSourceWhenDestinationHoldsEquivalent(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	body := "# Shared foundations\n"
	seedOwnedManagedSkill(t, oldPath, "foundations", body)
	seedOwnedManagedSkill(t, newPath, "foundations", body)
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "skills moved to ~/.agents/skills"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, filepath.Join(oldPath, "foundations"))
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), body)
	if !strings.Contains(stdout.String(), "removed stale relocated path old-agents-skills") {
		t.Fatalf("stdout = %q, want removed-stale when destination already holds equivalent", stdout.String())
	}
}

func TestRunnerUpgradeRelocatesOwnedSkillWhenDestinationHomeExistsButSkillAbsent(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "stale", "# Stale\n")
	writeInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "skills moved to ~/.agents/skills"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--yes"})
	if err != nil {
		t.Fatalf("upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, oldPath)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	assertInstallFile(t, filepath.Join(newPath, "stale", "SKILL.md"), "# Stale\n")
	if !strings.Contains(stdout.String(), "relocated path old-agents-skills") {
		t.Fatalf("stdout = %q, want relocation report (dest home exists but skill was absent)", stdout.String())
	}
}
