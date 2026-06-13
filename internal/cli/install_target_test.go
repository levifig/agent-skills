package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallTargetOpencodeSyncsBuiltOutputAndMarker(t *testing.T) {
	root := realpath(t, t.TempDir())
	dist := filepath.Join(root, "dist", "opencode")
	config := filepath.Join(root, "config", "opencode")
	writeInstallFile(t, filepath.Join(dist, "skills", "go-development", "SKILL.md"), "# Go\n")
	writeInstallFile(t, filepath.Join(dist, "agents", "implementer.md"), "# Implementer\n")
	writeInstallFile(t, filepath.Join(config, "skills", "stale", "SKILL.md"), "stale\n")

	err := installTargetDistribution(targetInstallOptions{
		Target:    "opencode",
		DistDir:   dist,
		ConfigDir: config,
		Version:   "9.8.7-test.1",
	})
	if err != nil {
		t.Fatalf("install opencode error = %v", err)
	}
	assertInstallFile(t, filepath.Join(config, "skills", "go-development", "SKILL.md"), "# Go\n")
	if _, err := os.Stat(filepath.Join(config, "skills", "stale")); !os.IsNotExist(err) {
		t.Fatalf("stale opencode skill stat = %v, want removed by sync", err)
	}
	assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetCursorMergesHooksAndRemovesObsoleteHooksOnUpgrade(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "cursor")
	config := filepath.Join(root, ".cursor")
	checkHook := "loaf check --hook check-" + "se" + "crets"
	writeInstallFile(t, filepath.Join(dist, "skills", "foundations", "SKILL.md"), "# Foundations\n")
	writeInstallFile(t, filepath.Join(dist, "agents", "reviewer.md"), "# Reviewer\n")
	writeInstallFile(t, filepath.Join(dist, "templates", "session.md"), "session\n")
	writeInstallFile(t, filepath.Join(dist, "hooks", "post-tool", "check.sh"), "#!/bin/sh\n")
	writeInstallFile(t, filepath.Join(dist, "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"`+checkHook+`","matcher":"Edit|Write|Bash","loaf-managed":true}]}}`)
	writeInstallFile(t, filepath.Join(config, "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"user hook"},{"command":"`+checkHook+`","matcher":"Edit|Write|Bash","loaf-managed":true}],"PreToolUse":[{"prompt":"STOP. Before running gh pr merge anything"}]}}`)
	writeInstallFile(t, filepath.Join(config, "commands", "stale.md"), "stale\n")
	writeInstallFile(t, filepath.Join(config, "hooks", "session", "session-start.sh"), "obsolete\n")

	err := installTargetDistribution(targetInstallOptions{
		Target:    "cursor",
		DistDir:   dist,
		ConfigDir: config,
		Upgrade:   true,
		Version:   "9.8.7-test.1",
		HomeDir:   home,
	})
	if err != nil {
		t.Fatalf("install cursor error = %v", err)
	}
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	assertInstallFile(t, filepath.Join(config, "agents", "reviewer.md"), "# Reviewer\n")
	assertInstallFile(t, filepath.Join(config, "templates", "session.md"), "session\n")
	assertInstallFile(t, filepath.Join(config, "hooks", "post-tool", "check.sh"), "#!/bin/sh\n")
	if _, err := os.Stat(filepath.Join(config, "commands")); !os.IsNotExist(err) {
		t.Fatalf("cursor stale commands stat = %v, want removed", err)
	}
	if _, err := os.Stat(filepath.Join(config, "hooks", "session", "session-start.sh")); !os.IsNotExist(err) {
		t.Fatalf("obsolete cursor hook stat = %v, want removed", err)
	}
	hooks := readInstallHooks(t, filepath.Join(config, "hooks.json"))
	postTool := hooks.Hooks["PostToolUse"]
	if len(postTool) != 2 {
		t.Fatalf("PostToolUse hooks = %#v, want user hook plus new loaf hook", postTool)
	}
	if postTool[0]["command"] != "user hook" || postTool[1]["command"] != checkHook {
		t.Fatalf("PostToolUse hooks = %#v, want user hook preserved and loaf hook replaced", postTool)
	}
	if _, ok := hooks.Hooks["PreToolUse"]; ok {
		t.Fatalf("PreToolUse hooks = %#v, want legacy prompt removed", hooks.Hooks["PreToolUse"])
	}
	assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetCodexUsesCodexHomeForHooksAndSharedSkillsHome(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex-home")
	dist := filepath.Join(root, "dist", "codex")
	config := filepath.Join(root, "reported-config")
	writeInstallFile(t, filepath.Join(dist, "skills", "go-development", "SKILL.md"), "# Go\n")
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"loaf session log --from-hook","matcher":"Bash","if":"Bash(git commit:*)","loaf-managed":true}]}}`)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"version":1,"hooks":{"PostToolUse":[{"command":"user codex hook"},{"command":"loaf session log --from-hook","matcher":"Bash","if":"Bash(git commit:*)"}]}}`)

	err := installTargetDistribution(targetInstallOptions{
		Target:    "codex",
		DistDir:   dist,
		ConfigDir: config,
		Version:   "9.8.7-test.1",
		HomeDir:   home,
		CodexHome: codexHome,
	})
	if err != nil {
		t.Fatalf("install codex error = %v", err)
	}
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
	hooks := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	postTool := hooks.Hooks["PostToolUse"]
	if len(postTool) != 2 || postTool[0]["command"] != "user codex hook" || postTool[1]["command"] != "loaf session log --from-hook" {
		t.Fatalf("codex hooks = %#v, want user hook preserved and loaf hook replaced", postTool)
	}
	assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetGeminiAndAmpUseSharedAndCustomHomes(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	geminiDist := filepath.Join(root, "dist", "gemini")
	geminiConfig := filepath.Join(root, ".gemini")
	writeInstallFile(t, filepath.Join(geminiDist, "skills", "knowledge-base", "SKILL.md"), "# KB\n")

	if err := installTargetDistribution(targetInstallOptions{
		Target:    "gemini",
		DistDir:   geminiDist,
		ConfigDir: geminiConfig,
		Version:   "9.8.7-test.1",
		HomeDir:   home,
	}); err != nil {
		t.Fatalf("install gemini error = %v", err)
	}
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "knowledge-base", "SKILL.md"), "# KB\n")
	assertInstallFile(t, filepath.Join(geminiConfig, loafInstallMarkerFile), "9.8.7-test.1\n")

	ampDist := filepath.Join(root, "dist", "amp")
	ampConfig := filepath.Join(root, ".amp")
	ampSkills := filepath.Join(root, "amp-skills")
	ampPlugins := filepath.Join(root, "amp-plugins")
	writeInstallFile(t, filepath.Join(ampDist, "skills", "implement", "SKILL.md"), "# Implement\n")
	writeInstallFile(t, filepath.Join(ampDist, "plugins", "loaf.js"), "export default {}\n")

	if err := installTargetDistribution(targetInstallOptions{
		Target:        "amp",
		DistDir:       ampDist,
		ConfigDir:     ampConfig,
		Version:       "9.8.7-test.1",
		HomeDir:       home,
		AmpSkillsDir:  ampSkills,
		AmpPluginsDir: ampPlugins,
	}); err != nil {
		t.Fatalf("install amp error = %v", err)
	}
	assertInstallFile(t, filepath.Join(ampSkills, "implement", "SKILL.md"), "# Implement\n")
	assertInstallFile(t, filepath.Join(ampPlugins, "loaf.js"), "export default {}\n")
	assertInstallFile(t, filepath.Join(ampConfig, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetRejectsUnknownTarget(t *testing.T) {
	err := installTargetDistribution(targetInstallOptions{
		Target:    "unknown",
		DistDir:   t.TempDir(),
		ConfigDir: t.TempDir(),
		Version:   "9.8.7-test.1",
	})
	if err == nil || !strings.Contains(err.Error(), "no installer available") {
		t.Fatalf("unknown install target error = %v, want unavailable target error", err)
	}
}

func writeInstallFile(t *testing.T, path string, body string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	writeFile(t, path, body)
}

func assertInstallFile(t *testing.T, path string, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(body) != want {
		t.Fatalf("ReadFile(%s) = %q, want %q", path, body, want)
	}
}

func assertInstallPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) error = %v, want missing path", path, err)
	}
}

func readInstallHooks(t *testing.T, path string) codexHooksFile {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var hooks codexHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v\n%s", path, err, body)
	}
	return hooks
}
