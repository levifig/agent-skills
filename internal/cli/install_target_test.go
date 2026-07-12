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
	home := filepath.Join(root, "home")
	dist := filepath.Join(root, "dist", "opencode")
	config := filepath.Join(root, "config", "opencode")
	writeInstallFile(t, filepath.Join(dist, "skills", "go-development", "SKILL.md"), "# Go\n")
	writeInstallFile(t, filepath.Join(dist, "agents", "implementer.md"), "# Implementer\n")

	err := installTargetDistribution(targetInstallOptions{
		Target:    "opencode",
		DistDir:   dist,
		ConfigDir: config,
		Version:   "9.8.7-test.1",
		HomeDir:   home,
	})
	if err != nil {
		t.Fatalf("install opencode error = %v", err)
	}
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
	assertInstallPathMissing(t, filepath.Join(config, "skills", "go-development"))
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
	writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"hooks":{}}`)
	writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), `{"version":1,"description":"user hooks","hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"user codex hook"}]}],"Stop":[],"PostToolUse":[{"command":"loaf journal log --from-hook","matcher":"Bash","if":"Bash(git commit:*)"}]}}`)

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
	if hooks.Version != 0 {
		t.Fatalf("codex hooks version = %d, want omitted current schema", hooks.Version)
	}
	if hooks.Description != "user hooks" {
		t.Fatalf("codex hooks description = %q, want preserved user description", hooks.Description)
	}
	if len(hooks.Hooks["SessionStart"]) != 1 || hooks.Hooks["SessionStart"][0]["matcher"] != "startup" {
		t.Fatalf("codex hooks = %#v, want nested user matcher group preserved", hooks.Hooks)
	}
	if stop, ok := hooks.Hooks["Stop"]; !ok || len(stop) != 0 {
		t.Fatalf("codex hooks = %#v, want explicitly empty Stop event preserved", hooks.Hooks)
	}
	if _, ok := hooks.Hooks["PostToolUse"]; ok {
		t.Fatalf("codex hooks = %#v, want legacy flat Loaf hook retired", hooks.Hooks)
	}
	assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetCodexRejectsMalformedOrUnsupportedHooks(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "malformed", body: "{", want: "parse Codex hooks file"},
		{name: "unsupported-flat-user-entry", body: `{"hooks":{"PreToolUse":[{"command":"user hook"}]}}`, want: "unsupported matcher entry"},
		{name: "user-loaf-journal-command", body: `{"hooks":{"PreToolUse":[{"command":"loaf journal recent --json"}]}}`, want: "unsupported matcher entry"},
		{name: "user-custom-loaf-check", body: `{"hooks":{"PreToolUse":[{"command":"loaf check --hook my-company-gate"}]}}`, want: "unsupported matcher entry"},
		{name: "empty-handler", body: `{"hooks":{"SessionStart":[{"hooks":[{}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-handler-type", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"prompt","command":"echo user"}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-handler-field", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","bogus":true}]}]}}`, want: "unsupported matcher entry"},
		{name: "wrong-handler-field-type", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","async":"true"}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-group-field", body: `{"hooks":{"SessionStart":[{"event":"startup","hooks":[{"type":"command","command":"echo user"}]}]}}`, want: "unsupported matcher entry"},
		{name: "unknown-event", body: `{"hooks":{"BogusEvent":[{"hooks":[{"type":"command","command":"echo user"}]}]}}`, want: "unsupported hook event"},
		{name: "null-event-array", body: `{"hooks":{"Stop":null}}`, want: "must be an array"},
		{name: "timeout-zero", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":0}]}]}}`, want: "unsupported matcher entry"},
		{name: "timeout-negative", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":-1}]}]}}`, want: "unsupported matcher entry"},
		{name: "timeout-fractional", body: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo user","timeout":1.5}]}]}}`, want: "unsupported matcher entry"},
		{name: "versioned-nested-user-only", body: `{"version":1,"description":"user hooks","hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"echo user"}]}]}}`, want: "legacy version metadata"},
		{name: "version-string", body: `{"version":"1","hooks":{}}`, want: "legacy version must be numeric 1"},
		{name: "version-two", body: `{"version":2,"hooks":{}}`, want: "legacy version must be numeric 1"},
		{name: "unsupported-top-level-field", body: `{"hooks":{},"unknown":true}`, want: "unsupported top-level field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := realpath(t, t.TempDir())
			home := filepath.Join(root, "home")
			codexHome := filepath.Join(root, "codex-home")
			dist := filepath.Join(root, "dist", "codex")
			config := filepath.Join(root, "reported-config")
			writeInstallFile(t, filepath.Join(dist, ".codex", "hooks.json"), `{"hooks":{}}`)
			writeInstallFile(t, filepath.Join(codexHome, "hooks.json"), tc.body)

			err := installTargetDistribution(targetInstallOptions{
				Target:    "codex",
				DistDir:   dist,
				ConfigDir: config,
				Version:   "9.8.7-test.1",
				HomeDir:   home,
				CodexHome: codexHome,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("install codex error = %v, want %q", err, tc.want)
			}
			assertInstallFile(t, filepath.Join(codexHome, "hooks.json"), tc.body)
		})
	}
}

func TestInstallTargetAmpUsesSharedAndCustomHomes(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	ampDist := filepath.Join(root, "dist", "amp")
	ampConfig := filepath.Join(root, ".amp")
	ampSkills := filepath.Join(root, "amp-skills")
	ampPlugins := filepath.Join(root, "amp-plugins")
	writeInstallFile(t, filepath.Join(ampDist, "skills", "implement", "SKILL.md"), "# Implement\n")
	writeInstallFile(t, filepath.Join(ampDist, ".amp", "plugins", "loaf.ts"), "export default function () {}\n")

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
	assertInstallFile(t, filepath.Join(ampPlugins, "loaf.ts"), "export default function () {}\n")
	assertInstallFile(t, filepath.Join(ampConfig, loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestInstallTargetRejectsUnknownTarget(t *testing.T) {
	err := installTargetDistribution(targetInstallOptions{
		Target:    "claude-code",
		DistDir:   t.TempDir(),
		ConfigDir: t.TempDir(),
		Version:   "9.8.7-test.1",
	})
	if err == nil || !strings.Contains(err.Error(), "no installer available") {
		t.Fatalf("claude-code install target error = %v, want plugin exception unavailable target error", err)
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
