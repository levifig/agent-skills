package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerInstallExplicitCursorTargetRunsNatively(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","loaf-managed":true}]}}`)

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor error = %v\n%s", err, stdout.String())
	}

	if strings.Contains(stdout.String(), "args=install") {
		t.Fatalf("stdout = %q, want native install without legacy delegation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Cursor installed") {
		t.Fatalf("stdout = %q, want Cursor install summary", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	assertInstallSymlinkTarget(t, filepath.Join(root, "AGENTS.md"), filepath.Join(root, ".agents", "AGENTS.md"))
	canonical := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	if !strings.Contains(canonical, "## Loaf Framework") || !strings.Contains(canonical, "v9.8.7-test.1") {
		t.Fatalf("canonical AGENTS.md = %q, want native fenced section with package version", canonical)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want non-interactive MCP defaults disabled", integrations)
	}
}

func TestRunnerInstallUsesAgentsHomeSkillDestinations(t *testing.T) {
	for _, target := range []string{"opencode", "cursor", "codex", "amp"} {
		t.Run(target, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), "# Foundations\n")
			if target == "codex" {
				writeInstallFile(t, filepath.Join(root, "dist", target, ".codex", "hooks.json"), `{"version":1,"hooks":{}}`)
			}

			var stdout bytes.Buffer
			err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--to", target, "--yes"})
			if err != nil {
				t.Fatalf("install --to %s error = %v\n%s", target, err, stdout.String())
			}

			assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Foundations\n")
			record := readInstallCommandJSON(t, installRecordPath(home, target))
			if record["target"] != target || record["skills_dir"] != filepath.Join(home, ".agents", "skills") {
				t.Fatalf("record = %#v, want target and shared skills dir", record)
			}
			switch target {
			case "opencode":
				assertInstallPathMissing(t, filepath.Join(home, ".config", "opencode", "skills", "foundations"))
			case "amp":
				assertInstallPathMissing(t, filepath.Join(home, ".config", "agents", "skills", "foundations"))
			}
		})
	}
}

func TestRunnerInstallSharedSkillsPreservesForeignEntries(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	sharedSkills := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(sharedSkills, "foreign-skill", "SKILL.md"), "# Mine\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(sharedSkills, "foreign-skill", "SKILL.md"), "# Mine\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), "# Foundations\n")

	if err := os.RemoveAll(filepath.Join(root, "dist", "cursor", "skills", "foundations")); err != nil {
		t.Fatalf("RemoveAll(foundations) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "go-development", "SKILL.md"), "# Go\n")
	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("second install --to cursor error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, filepath.Join(sharedSkills, "foundations"))
	assertInstallFile(t, filepath.Join(sharedSkills, "foreign-skill", "SKILL.md"), "# Mine\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "go-development", "SKILL.md"), "# Go\n")
}

func TestRunnerInstallRecordKeepsRelocatedTargetDetectable(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor error = %v\n%s", err, stdout.String())
	}
	if err := os.Remove(filepath.Join(home, ".cursor", loafInstallMarkerFile)); err != nil {
		t.Fatalf("Remove(marker) error = %v", err)
	}
	if !isLoafInstalledForTargetInstall("cursor", filepath.Join(home, ".cursor")) {
		t.Fatal("cursor install not detected from shared install record")
	}
}

func TestRunnerInstallUpgradeOnlyInstallsDetectedLoafTargets(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	mkdirAll(t, filepath.Join(home, ".config", "opencode"))
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Upgrading:") || !strings.Contains(stdout.String(), "cursor") || !strings.Contains(stdout.String(), "Cursor installed") {
		t.Fatalf("stdout = %q, want cursor-only upgrade", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", loafInstallMarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("opencode marker stat = %v, want not installed during upgrade", err)
	}
}

func TestRunnerInstallUpgradeRelocatesOpenCodeAndAmpSkillHomes(t *testing.T) {
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
			ownerMarker: func(home string) string { return filepath.Join(home, ".amp", loafInstallMarkerFile) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			oldSkills := tc.oldSkills(home)
			ownerMarker := tc.ownerMarker(home)
			writeInstallFile(t, ownerMarker, "old\n")
			writeInstallFile(t, filepath.Join(oldSkills, "foundations", "SKILL.md"), "# Old foundations\n")
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
			err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
			if err != nil {
				t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
			}
			assertInstallPathMissing(t, oldSkills)
			assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Old foundations\n")
			assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
			if !strings.Contains(stdout.String(), "relocated path "+tc.target+"-skills-to-agents-home") {
				t.Fatalf("stdout = %q, want relocation report", stdout.String())
			}
		})
	}
}

func TestRunnerInstallUpgradeCleansRetiredTargetFromManifest(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	retiredTarget := filepath.Join(home, ".retired-tool")
	writeInstallFile(t, filepath.Join(retiredTarget, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(retiredTarget, "skills", "stale", "SKILL.md"), "stale\n")
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
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

func TestRunnerInstallUpgradeCleansRetiredGeminiTargetWithoutReintroducingIt(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	geminiHome := filepath.Join(home, ".gemini")
	writeInstallFile(t, filepath.Join(geminiHome, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(geminiHome, "skills", "stale", "SKILL.md"), "# Stale\n")
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, geminiHome)
	if isValidInstallTarget("gemini") {
		t.Fatal("gemini target was reintroduced")
	}
	if !strings.Contains(stdout.String(), "removed retired target gemini") {
		t.Fatalf("stdout = %q, want gemini cleanup report", stdout.String())
	}
}

func TestRunnerInstallUpgradeCleansRetiredSkillFromManifest(t *testing.T) {
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
      "window": "one-release",
      "reason": "old-skill was retired",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
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

func TestRunnerInstallUpgradeSkipsDestructiveDeprecationWithoutExplicitYes(t *testing.T) {
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(retiredSkill, "SKILL.md"), "# Old skill\n")
	for _, want := range []string{"skipped skill old-skill", "rerun with --yes to apply destructive deprecation cleanup"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerInstallUpgradeCleansRetiredAgentFromManifest(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	agentHome := filepath.Join(home, ".cursor", "agents")
	retiredAgent := filepath.Join(agentHome, "old-agent.md")
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	if _, err := os.Stat(retiredAgent); !os.IsNotExist(err) {
		t.Fatalf("retired agent stat = %v, want removed", err)
	}
	for _, want := range []string{"removed retired agent old-agent", "old-agent was retired"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerInstallUpgradeSkipsUnmarkedRetiredAgent(t *testing.T) {
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, retiredAgent, "# User-owned Agent\n")
	if !strings.Contains(stdout.String(), "path is not marked as Loaf-owned") {
		t.Fatalf("stdout = %q, want unmarked skip", stdout.String())
	}
}

func TestRunnerInstallUpgradeReportsDefaultDeprecationWindow(t *testing.T) {
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "since v9.9.0, window one-release") {
		t.Fatalf("stdout = %q, want default deprecation window", stdout.String())
	}
}

func TestRunnerInstallUpgradeReportsDeprecationSignoff(t *testing.T) {
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
      "signoff": "report-spec-053-taxonomy-signoff",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"removed retired skill old-skill", "[signoff: report-spec-053-taxonomy-signoff]"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunnerInstallUpgradeReportsAliasTombstoneFromManifest(t *testing.T) {
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
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

func TestRunnerInstallUpgradeReportsExternalizedSkillWithoutRemoving(t *testing.T) {
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
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

func TestRunnerInstallUpgradeSkipsUnmarkedRetiredTarget(t *testing.T) {
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(retiredTarget, "user-file.txt"), "keep me\n")
	if !strings.Contains(stdout.String(), "path is not marked as Loaf-owned") {
		t.Fatalf("stdout = %q, want unmarked skip", stdout.String())
	}
}

func TestRunnerInstallUpgradeRelocatesManifestPathExactlyOnce(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(oldPath, "foundations", "SKILL.md"), "# Foundations\n")
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, oldPath)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	if !strings.Contains(stdout.String(), "relocated path old-agents-skills") {
		t.Fatalf("stdout = %q, want relocation report", stdout.String())
	}

	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("second install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, oldPath)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
}

func TestRunnerInstallUpgradeRemovesStaleRelocatedPathWhenDestinationExists(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(oldPath, "stale", "SKILL.md"), "# Stale\n")
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
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--upgrade", "--yes"})
	if err != nil {
		t.Fatalf("install --upgrade error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, oldPath)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	if !strings.Contains(stdout.String(), "removed stale relocated path old-agents-skills") {
		t.Fatalf("stdout = %q, want stale relocation removal report", stdout.String())
	}
}

func TestRunnerInstallCodexUsesCodeXHomeNatively(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	codexHome := filepath.Join(home, "custom-codex")
	t.Setenv("CODEX_HOME", codexHome)
	writeInstallFile(t, filepath.Join(root, "dist", "codex", "skills", "go-development", "SKILL.md"), "# Go\n")
	writeInstallFile(t, filepath.Join(root, "dist", "codex", ".codex", "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","if":"Bash(git commit:*)","loaf-managed":true}]}}`)

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root}.Run([]string{"install", "--to", "codex", "--yes"})
	if err != nil {
		t.Fatalf("install --to codex error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(codexHome, loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
	hooks := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	if len(hooks.Hooks["PostToolUse"]) != 1 || hooks.Hooks["PostToolUse"][0]["command"] != "loaf journal log --from-hook" {
		t.Fatalf("codex hooks = %#v, want native hook merge in CODEX_HOME", hooks.Hooks)
	}
}

func TestRunnerInstallOffersBinarySelfInstall(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	pathBin := filepath.Join(root, "path-bin")
	mkdirAll(t, pathBin)
	t.Setenv("PATH", pathBin)
	writeInstallFile(t, filepath.Join(root, "bin", "loaf"), "#!/bin/sh\nprintf 'loaf %s\\n' \"$*\"\n")
	if err := os.Chmod(filepath.Join(root, "bin", "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(source loaf) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "bin", "native", "darwin-arm64", "loaf"), "native\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("y\n\n\n"),
		WorkingDir: root,
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor with binary prompt error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Install 'loaf' binary to ~/.local/bin?") || !strings.Contains(stdout.String(), "Installed loaf binary") {
		t.Fatalf("stdout = %q, want binary install prompt and success", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".local", "bin", "loaf"), "#!/bin/sh\nprintf 'loaf %s\\n' \"$*\"\n")
	assertInstallFile(t, filepath.Join(home, ".local", "bin", "native", "darwin-arm64", "loaf"), "native\n")
	assertInstallPathMissing(t, filepath.Join(home, ".local", "share", "loaf", "dist-cli", "index.js"))
}

func TestRunnerInstallInteractiveSelectionRunsNatively(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("y\n"),
		WorkingDir: root,
	}.Run([]string{"install"})
	if err != nil {
		t.Fatalf("interactive install error = %v\n%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "args=install") {
		t.Fatalf("stdout = %q, want native interactive install without legacy delegation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Install to") || !strings.Contains(stdout.String(), "Cursor installed") {
		t.Fatalf("stdout = %q, want prompts and cursor install", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestRunnerInstallInteractiveNoTargetsStillUpdatesClaudeProjectFile(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	mkdirAll(t, filepath.Join(home, ".cursor"))
	writeInstallFile(t, filepath.Join(root, "bin", "claude"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(root, "bin", "claude"), 0o755); err != nil {
		t.Fatalf("Chmod(fake claude) error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("n\n"),
		WorkingDir: root,
	}.Run([]string{"install"})
	if err != nil {
		t.Fatalf("interactive no-target install error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "No targets selected") {
		t.Fatalf("stdout = %q, want no targets selected", stdout.String())
	}
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	assertInstallSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), canonical)
	body := string(readFileBytes(t, canonical))
	if !strings.Contains(body, "## Loaf Framework") {
		t.Fatalf("canonical body = %q, want Claude project fenced section", body)
	}
}

func TestRunnerInstallMcpRecommendationWritesCursorProjectConfig(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "bin", "loaf"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(root, "bin", "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(root, ".cursor", "mcp.json"), `{"mcpServers":{"existing":{"command":"keep","args":[]}},"theme":"dark"}`+"\n")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("p\nn\n"),
		WorkingDir: root,
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor with MCP prompt error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Recommended MCP servers") || !strings.Contains(stdout.String(), "cursor: merged into .cursor/mcp.json") {
		t.Fatalf("stdout = %q, want MCP recommendation output", stdout.String())
	}
	mcp := readInstallCommandJSON(t, filepath.Join(root, ".cursor", "mcp.json"))
	if mcp["theme"] != "dark" {
		t.Fatalf("mcp config = %#v, want existing sibling key preserved", mcp)
	}
	servers := mcp["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Fatalf("mcp servers = %#v, want existing server preserved", servers)
	}
	linear := servers["linear"].(map[string]any)
	if linear["command"] != "npx" {
		t.Fatalf("linear server = %#v, want command npx", linear)
	}
	args := linear["args"].([]any)
	if len(args) != 3 || args[2] != "https://mcp.linear.app/mcp" {
		t.Fatalf("linear args = %#v, want mcp-remote URL", args)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != true || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want linear enabled and serena disabled", integrations)
	}
}

func TestRunnerInstallMcpRecommendationOffersSerenaNativeInstall(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	bin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "serena-install.log")
	writeInstallFile(t, filepath.Join(bin, "loaf"), "#!/bin/sh\nexit 0\n")
	writeInstallFile(t, filepath.Join(bin, "uv"), fmt.Sprintf(`#!/bin/sh
echo "uv $*" >> %q
/bin/cat > %q <<'EOS'
#!/bin/sh
echo "serena $*" >> %q
exit 0
EOS
/bin/chmod +x %q
`, logPath, filepath.Join(bin, "serena"), logPath, filepath.Join(bin, "serena")))
	if err := os.Chmod(filepath.Join(bin, "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}
	if err := os.Chmod(filepath.Join(bin, "uv"), 0o755); err != nil {
		t.Fatalf("Chmod(fake uv) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("n\np\ny\n"),
		WorkingDir: root,
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor with Serena prompt error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Serena must be installed natively") || !strings.Contains(stdout.String(), "Serena native CLI installed") {
		t.Fatalf("stdout = %q, want Serena prerequisite install output", stdout.String())
	}
	log := string(readFileBytes(t, logPath))
	if !strings.Contains(log, "uv tool install -p 3.13 serena-agent@latest --prerelease=allow") || !strings.Contains(log, "serena init") {
		t.Fatalf("serena install log = %q, want uv install and serena init", log)
	}
	mcp := readInstallCommandJSON(t, filepath.Join(root, ".cursor", "mcp.json"))
	servers := mcp["mcpServers"].(map[string]any)
	serena := servers["serena"].(map[string]any)
	if serena["command"] != "serena" {
		t.Fatalf("serena server = %#v, want serena command", serena)
	}
	args := serena["args"].([]any)
	if len(args) != 4 || args[1] != "--context" || args[2] != "ide" {
		t.Fatalf("serena args = %#v, want Cursor Serena context", args)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != true {
		t.Fatalf("integrations = %#v, want linear disabled and serena enabled", integrations)
	}
}

func TestInstallMcpConfigWritersHandleOpenCodeAndNestedAmp(t *testing.T) {
	root := realpath(t, t.TempDir())
	opencodePath := filepath.Join(root, "opencode.json")
	if err := mergeOpenCodeMcpConfig(opencodePath, "linear", []string{"npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"}); err != nil {
		t.Fatalf("mergeOpenCodeMcpConfig error = %v", err)
	}
	opencode := readInstallCommandJSON(t, opencodePath)
	openServers := opencode["mcp"].(map[string]any)
	openLinear := openServers["linear"].(map[string]any)
	if openLinear["type"] != "local" || openLinear["enabled"] != true {
		t.Fatalf("opencode linear = %#v, want local enabled server", openLinear)
	}
	command := openLinear["command"].([]any)
	if len(command) != 4 || command[0] != "npx" {
		t.Fatalf("opencode command = %#v, want command array", command)
	}

	ampPath := filepath.Join(root, ".amp", "settings.json")
	writeInstallFile(t, ampPath, `{"amp":{"theme":"quiet"}}`+"\n")
	if err := mergeJSONMcpConfig(ampPath, "amp.mcpServers", "linear", []string{"npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"}); err != nil {
		t.Fatalf("mergeJSONMcpConfig(amp) error = %v", err)
	}
	amp := readInstallCommandJSON(t, ampPath)
	ampSection := amp["amp"].(map[string]any)
	if ampSection["theme"] != "quiet" {
		t.Fatalf("amp section = %#v, want existing nested key preserved", ampSection)
	}
	ampServers := ampSection["mcpServers"].(map[string]any)
	if ampServers["linear"].(map[string]any)["command"] != "npx" {
		t.Fatalf("amp servers = %#v, want nested linear server", ampServers)
	}
}

func TestRunnerInstallFromLinkedWorktreeWritesMainLoafConfig(t *testing.T) {
	requireCLIGit(t)
	main := initCLIGitRepo(t)
	home := filepath.Join(main, "home")
	bin := filepath.Join(main, "bin")
	originalPath := os.Getenv("PATH")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+originalPath)
	mkdirAll(t, bin)
	mkdirAll(t, filepath.Join(home, ".cursor"))
	writeInstallFile(t, filepath.Join(main, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`+"\n")
	writeInstallFile(t, filepath.Join(main, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	gitCLI(t, main, "add", "package.json", "dist/cursor/skills/foundations/SKILL.md")
	gitCLI(t, main, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "add install fixture")
	linked := addCLILinkedWorktree(t, main, "install-config")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: linked,
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install from linked worktree error = %v\n%s", err, stdout.String())
	}

	config := readInstallCommandJSON(t, filepath.Join(main, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want defaults recorded in main worktree", integrations)
	}
	if _, err := os.Stat(filepath.Join(linked, ".agents", "loaf.json")); !os.IsNotExist(err) {
		t.Fatalf("linked loaf.json stat = %v, want no shadow config in linked worktree", err)
	}
}

func TestRunnerInstallHelpAndInvalidTargetAreNative(t *testing.T) {
	var helpOut bytes.Buffer
	if err := (Runner{Stdout: &helpOut, WorkingDir: t.TempDir()}).Run([]string{"install", "--help"}); err != nil {
		t.Fatalf("install --help error = %v", err)
	}
	if !strings.Contains(helpOut.String(), "Usage: loaf install") {
		t.Fatalf("help output = %q, want native install help", helpOut.String())
	}

	err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: t.TempDir()}).Run([]string{"install", "--to", "wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown install target") {
		t.Fatalf("invalid target error = %v, want native unknown target error", err)
	}
}

func setupInstallCommandFixture(t *testing.T) (string, string) {
	t.Helper()
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	mkdirAll(t, home)
	mkdirAll(t, bin)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PATH", bin)
	writeInstallFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`+"\n")
	return root, home
}

func writeInstallDeprecationManifest(t *testing.T, root string, body string) {
	t.Helper()
	writeInstallFile(t, filepath.Join(root, "config", "deprecations.json"), body+"\n")
}

func readInstallCommandJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v\n%s", path, err, body)
	}
	return data
}
