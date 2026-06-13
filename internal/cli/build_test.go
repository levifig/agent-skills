package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerBuildHelpIsNative(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: workingDir,
	}.Run([]string{"build", "--help"})
	if err != nil {
		t.Fatalf("build --help error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Usage: loaf build [options]") || !strings.Contains(output, "--target") {
		t.Fatalf("output = %q, want native build help", output)
	}
}

func TestRunnerBuildRunsContentBuilderNatively(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	seedNativeOpenCodeBuildFixture(t, root)
	seedNativeClaudeCodeBuildFixture(t, root)
	for _, staleFile := range []string{
		filepath.Join(root, "plugins", "loaf", "stale.txt"),
		filepath.Join(root, "dist", "opencode", "stale.txt"),
		filepath.Join(root, "dist", "cursor", "stale.txt"),
		filepath.Join(root, "dist", "codex", "stale.txt"),
		filepath.Join(root, "dist", "gemini", "stale.txt"),
		filepath.Join(root, "dist", "amp", "stale.txt"),
	} {
		mkdirAll(t, filepath.Dir(staleFile))
		writeFile(t, staleFile, "old target output\n")
	}
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build"})
	if err != nil {
		t.Fatalf("build error = %v", err)
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native all-target build without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "claude-code", "opencode", "cursor", "codex", "gemini", "amp", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	for _, staleFile := range []string{
		filepath.Join(root, "plugins", "loaf", "stale.txt"),
		filepath.Join(root, "dist", "opencode", "stale.txt"),
		filepath.Join(root, "dist", "cursor", "stale.txt"),
		filepath.Join(root, "dist", "codex", "stale.txt"),
		filepath.Join(root, "dist", "gemini", "stale.txt"),
		filepath.Join(root, "dist", "amp", "stale.txt"),
	} {
		if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
			t.Fatalf("stale output stat for %s = %v, want target output reset", staleFile, err)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "plugins", "loaf", ".claude-plugin", "plugin.json"),
		filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"),
		filepath.Join(root, "dist", "cursor", "hooks.json"),
		filepath.Join(root, "dist", "codex", ".codex", "hooks.json"),
		filepath.Join(root, "dist", "gemini", "skills", "demo", "SKILL.md"),
		filepath.Join(root, "dist", "amp", "plugins", "loaf.js"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
	}
}

func TestRunnerBuildTargetCodexRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "codex"})
	if err != nil {
		t.Fatalf("build --target codex error = %v\n%s", err, stdout.String())
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native codex target without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "codex", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}

	sharedSkill := readBuildFileString(t, filepath.Join(root, "dist", "skills", "demo", "SKILL.md"))
	wantSharedFrontmatter := strings.Join([]string{
		"---",
		"name: demo",
		"description: >-",
		"  Demo skill that has enough words to require folded YAML output from gray",
		"  matter when the native builder writes frontmatter for generated skills.",
		"---",
		"",
	}, "\n")
	if !strings.HasPrefix(sharedSkill, wantSharedFrontmatter) {
		t.Fatalf("shared skill frontmatter = %q, want prefix %q", sharedSkill, wantSharedFrontmatter)
	}
	if strings.Contains(sharedSkill, "{{IMPLEMENT_CMD}}") || !strings.Contains(sharedSkill, "/implement") {
		t.Fatalf("shared skill = %q, want command substitution", sharedSkill)
	}
	if strings.Contains(sharedSkill, "version: 9.8.7-test.1") {
		t.Fatalf("shared skill = %q, should not inject version into shared intermediate", sharedSkill)
	}

	codexSkill := readBuildFileString(t, filepath.Join(root, "dist", "codex", "skills", "demo", "SKILL.md"))
	wantCodexFrontmatter := strings.Join([]string{
		"---",
		"name: demo",
		"description: >-",
		"  Demo skill that has enough words to require folded YAML output from gray",
		"  matter when the native builder writes frontmatter for generated skills.",
		"version: 9.8.7-test.1",
		"---",
		"",
	}, "\n")
	if !strings.HasPrefix(codexSkill, wantCodexFrontmatter) {
		t.Fatalf("codex skill frontmatter = %q, want prefix %q", codexSkill, wantCodexFrontmatter)
	}
	for _, want := range []string{"/implement"} {
		if !strings.Contains(codexSkill, want) {
			t.Fatalf("codex skill = %q, want %q", codexSkill, want)
		}
	}
	if !strings.Contains(readBuildFileString(t, filepath.Join(root, "dist", "codex", "skills", "demo", "templates", "session.md")), "/implement") {
		t.Fatalf("shared template was not copied with command substitution")
	}
	scriptPath := filepath.Join(root, "dist", "codex", "skills", "demo", "scripts", "demo.sh")
	if readBuildFileString(t, scriptPath) != "#!/bin/sh\necho demo\n" {
		t.Fatalf("script copy mismatch")
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", scriptPath, err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("script mode = %v, want executable source mode preserved", info.Mode().Perm())
	}
	hooksJSON := readBuildFileString(t, filepath.Join(root, "dist", "codex", ".codex", "hooks.json"))
	if !strings.HasPrefix(hooksJSON, "{\n  \"version\": 1,\n  \"hooks\": {") {
		t.Fatalf("hooks.json = %q, want TypeScript-compatible top-level field order", hooksJSON)
	}
	if !strings.Contains(hooksJSON, "{\n        \"loaf-managed\": true,\n        \"matcher\": \"Bash\",\n        \"command\": \"loaf check --hook check-secrets\",\n        \"timeout\": 30,\n        \"failClosed\": true,\n        \"description\": \"Check for hardcoded secrets before writing\"\n      }") {
		t.Fatalf("hooks.json = %q, want TypeScript-compatible hook field order", hooksJSON)
	}
	for _, want := range []string{`"matcher": "Bash"`, `"command": "loaf check --hook check-secrets"`, `"timeout": 30`, `"failClosed": true`} {
		if !strings.Contains(hooksJSON, want) {
			t.Fatalf("hooks.json = %q, want %q", hooksJSON, want)
		}
	}
	if strings.Contains(hooksJSON, "workflow-pre-merge") || strings.Contains(hooksJSON, "detect-linear-magic") {
		t.Fatalf("hooks.json = %q, want only Bash enforcement hooks", hooksJSON)
	}
}

func TestRunnerBuildTargetGeminiRunsNativeSkillOnlyTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	staleGeminiFile := filepath.Join(root, "dist", "gemini", "stale.txt")
	mkdirAll(t, filepath.Dir(staleGeminiFile))
	writeFile(t, staleGeminiFile, "old target output\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "gemini"})
	if err != nil {
		t.Fatalf("build --target gemini error = %v\n%s", err, stdout.String())
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native gemini target without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "gemini", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(staleGeminiFile); !os.IsNotExist(err) {
		t.Fatalf("stale gemini file stat = %v, want target output reset", err)
	}

	geminiSkill := readBuildFileString(t, filepath.Join(root, "dist", "gemini", "skills", "demo", "SKILL.md"))
	wantGeminiFrontmatter := strings.Join([]string{
		"---",
		"name: demo",
		"description: >-",
		"  Demo skill that has enough words to require folded YAML output from gray",
		"  matter when the native builder writes frontmatter for generated skills.",
		"version: 9.8.7-test.1",
		"---",
		"",
	}, "\n")
	if !strings.HasPrefix(geminiSkill, wantGeminiFrontmatter) {
		t.Fatalf("gemini skill frontmatter = %q, want prefix %q", geminiSkill, wantGeminiFrontmatter)
	}
	if strings.Contains(geminiSkill, "{{IMPLEMENT_CMD}}") || !strings.Contains(geminiSkill, "/implement") {
		t.Fatalf("gemini skill = %q, want shared command substitution", geminiSkill)
	}
	if !strings.Contains(readBuildFileString(t, filepath.Join(root, "dist", "gemini", "skills", "demo", "references", "guide.md")), "/implement") {
		t.Fatalf("gemini reference was not copied from substituted shared intermediate")
	}
	if !strings.Contains(readBuildFileString(t, filepath.Join(root, "dist", "gemini", "skills", "demo", "templates", "session.md")), "/implement") {
		t.Fatalf("gemini shared template was not copied from substituted shared intermediate")
	}
	scriptPath := filepath.Join(root, "dist", "gemini", "skills", "demo", "scripts", "demo.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", scriptPath, err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("script mode = %v, want executable source mode preserved", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "gemini", ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("gemini hooks stat = %v, want skill-only target without Codex hooks", err)
	}
}

func TestRunnerBuildTargetAmpRunsNativePluginTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	staleAmpFile := filepath.Join(root, "dist", "amp", "stale.txt")
	mkdirAll(t, filepath.Dir(staleAmpFile))
	writeFile(t, staleAmpFile, "old target output\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "amp"})
	if err != nil {
		t.Fatalf("build --target amp error = %v\n%s", err, stdout.String())
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native amp target without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "amp", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(staleAmpFile); !os.IsNotExist(err) {
		t.Fatalf("stale amp file stat = %v, want target output reset", err)
	}

	ampSkill := readBuildFileString(t, filepath.Join(root, "dist", "amp", "skills", "demo", "SKILL.md"))
	if !strings.Contains(ampSkill, "version: 9.8.7-test.1") || strings.Contains(ampSkill, "{{IMPLEMENT_CMD}}") || !strings.Contains(ampSkill, "/implement") {
		t.Fatalf("amp skill = %q, want version injection and shared command substitution", ampSkill)
	}
	plugin := readBuildFileString(t, filepath.Join(root, "dist", "amp", "plugins", "loaf.js"))
	for _, want := range []string{
		"@version 9.8.7-test.1",
		"'tool.call': async (input) =>",
		`"command": "loaf check --hook check-secrets"`,
		`"command": "cat \"$LOAF_PLUGIN_DIR/hooks/instructions/pre-merge.md\""`,
		`const postToolHooks: Record<string, HookEntry[]> = {`,
		`"script": "post-tool/kb-staleness-nudge.sh"`,
		`const sessionHooks: Record<string, HookEntry[]> = {`,
		`"command": "loaf session start"`,
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("amp plugin = %q, want %q", plugin, want)
		}
	}
	if !strings.Contains(readBuildFileString(t, filepath.Join(root, "dist", "amp", "skills", "demo", "templates", "session.md")), "/implement") {
		t.Fatalf("amp shared template was not copied from substituted shared intermediate")
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "amp", ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("amp hooks stat = %v, want Amp plugin target without Codex hooks", err)
	}
}

func TestRunnerBuildTargetCursorRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	staleCursorFile := filepath.Join(root, "dist", "cursor", "stale.txt")
	mkdirAll(t, filepath.Dir(staleCursorFile))
	writeFile(t, staleCursorFile, "old target output\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "cursor"})
	if err != nil {
		t.Fatalf("build --target cursor error = %v\n%s", err, stdout.String())
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native cursor target without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "cursor", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(staleCursorFile); !os.IsNotExist(err) {
		t.Fatalf("stale cursor file stat = %v, want target output reset", err)
	}

	cursorSkill := readBuildFileString(t, filepath.Join(root, "dist", "cursor", "skills", "demo", "SKILL.md"))
	if !strings.Contains(cursorSkill, "version: 9.8.7-test.1") || strings.Contains(cursorSkill, "{{IMPLEMENT_CMD}}") || !strings.Contains(cursorSkill, "/implement") {
		t.Fatalf("cursor skill = %q, want version injection and shared command substitution", cursorSkill)
	}
	agent := readBuildFileString(t, filepath.Join(root, "dist", "cursor", "agents", "implementer.md"))
	for _, want := range []string{
		"model: inherit",
		"is_background: true",
		"name: implementer",
		"tools:",
		"  Bash: true",
		"version: 9.8.7-test.1",
	} {
		if !strings.Contains(agent, want) {
			t.Fatalf("cursor agent = %q, want %q", agent, want)
		}
	}
	if readBuildFileString(t, filepath.Join(root, "dist", "cursor", "hooks", "post-tool", "kb-staleness-nudge.sh")) != "#!/bin/sh\necho cursor override\n" {
		t.Fatalf("cursor hook override was not applied")
	}
	hooksJSON := readBuildFileString(t, filepath.Join(root, "dist", "cursor", "hooks.json"))
	for _, want := range []string{
		`"preToolUse": [`,
		`"loaf-managed": true`,
		`"command": "loaf check --hook check-secrets"`,
		`"command": "cat \"$HOME/.cursor/hooks/instructions/pre-merge.md\""`,
		`"postToolUse": [`,
		`"command": "bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh"`,
		`"sessionStart": [`,
		`"command": "loaf session start"`,
	} {
		if !strings.Contains(hooksJSON, want) {
			t.Fatalf("cursor hooks.json = %q, want %q", hooksJSON, want)
		}
	}
}

func TestRunnerBuildTargetOpenCodeRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	seedNativeOpenCodeBuildFixture(t, root)
	staleOpenCodeFile := filepath.Join(root, "dist", "opencode", "stale.txt")
	mkdirAll(t, filepath.Dir(staleOpenCodeFile))
	writeFile(t, staleOpenCodeFile, "old target output\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "opencode"})
	if err != nil {
		t.Fatalf("build --target opencode error = %v\n%s", err, stdout.String())
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native opencode target without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "opencode", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(staleOpenCodeFile); !os.IsNotExist(err) {
		t.Fatalf("stale opencode file stat = %v, want target output reset", err)
	}

	skill := readBuildFileString(t, filepath.Join(root, "dist", "opencode", "skills", "demo", "SKILL.md"))
	if !strings.Contains(skill, "subtask: false") || !strings.Contains(skill, "version: 9.8.7-test.1") {
		t.Fatalf("opencode skill = %q, want sidecar merge and version", skill)
	}
	command := readBuildFileString(t, filepath.Join(root, "dist", "opencode", "commands", "demo.md"))
	for _, want := range []string{
		"description: >-",
		"subtask: false",
		"version: 9.8.7-test.1",
		"/implement",
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("opencode command = %q, want %q", command, want)
		}
	}
	agent := readBuildFileString(t, filepath.Join(root, "dist", "opencode", "agents", "background-runner.md"))
	for _, want := range []string{
		"mode: subagent",
		"skills:",
		"  - foundations",
		"tools:",
		"  Read: true",
		"version: 9.8.7-test.1",
	} {
		if !strings.Contains(agent, want) {
			t.Fatalf("opencode agent = %q, want %q", agent, want)
		}
	}
	plugin := readBuildFileString(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks.ts"))
	for _, want := range []string{
		"@version 9.8.7-test.1",
		"'tool.execute.before': async (input) =>",
		`"command": "loaf check --hook check-secrets"`,
		`"command": "cat \"$LOAF_PLUGIN_DIR/hooks/instructions/pre-merge.md\""`,
		`"script": "post-tool/kb-staleness-nudge.sh"`,
		`event.type === 'context.compacting'`,
	} {
		if !strings.Contains(plugin, want) {
			t.Fatalf("opencode plugin = %q, want %q", plugin, want)
		}
	}
	if readBuildFileString(t, filepath.Join(root, "dist", "opencode", "plugins", "hooks", "post-tool", "kb-staleness-nudge.sh")) != "#!/bin/sh\necho opencode hook\n" {
		t.Fatalf("opencode hook copy mismatch")
	}
}

func TestRunnerBuildTargetClaudeCodeRunsNativeTarget(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	seedNativeClaudeCodeBuildFixture(t, root)
	stalePluginFile := filepath.Join(root, "plugins", "loaf", "stale.txt")
	mkdirAll(t, filepath.Dir(stalePluginFile))
	writeFile(t, stalePluginFile, "old target output\n")
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build", "--target", "claude-code"})
	if err != nil {
		t.Fatalf("build --target claude-code error = %v\n%s", err, stdout.String())
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native claude-code target without content-builder invocation", statErr)
	}
	for _, want := range []string{"loaf build", "shared skills intermediate", "claude-code", "Build complete"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
	if _, err := os.Stat(stalePluginFile); !os.IsNotExist(err) {
		t.Fatalf("stale plugin file stat = %v, want plugin output reset", err)
	}

	marketplace := readBuildFileString(t, filepath.Join(root, ".claude-plugin", "marketplace.json"))
	for _, want := range []string{`"name": "levifig-loaf"`, `"version": "9.8.7-test.1"`, `"source": "./plugins/loaf"`} {
		if !strings.Contains(marketplace, want) {
			t.Fatalf("marketplace.json = %q, want %q", marketplace, want)
		}
	}
	pluginJSON := readBuildFileString(t, filepath.Join(root, "plugins", "loaf", ".claude-plugin", "plugin.json"))
	for _, want := range []string{`"name": "loaf"`, `"version": "9.8.7-test.1"`, `"repository": "https://github.com/levifig/loaf"`} {
		if !strings.Contains(pluginJSON, want) {
			t.Fatalf("plugin.json = %q, want %q", pluginJSON, want)
		}
	}
	skill := readBuildFileString(t, filepath.Join(root, "plugins", "loaf", "skills", "demo", "SKILL.md"))
	for _, want := range []string{
		"allowed-tools: Bash",
		"version: 9.8.7-test.1",
		"/loaf:implement",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("claude skill = %q, want %q", skill, want)
		}
	}
	if strings.Contains(skill, "{{IMPLEMENT_CMD}}") || strings.Contains(skill, "/loaf:loaf:implement") {
		t.Fatalf("claude skill = %q, want scoped command substitution exactly once", skill)
	}
	agent := readBuildFileString(t, filepath.Join(root, "plugins", "loaf", "agents", "implementer.md"))
	for _, want := range []string{
		"name: implementer",
		"tools:",
		"  - Read",
		"  - Edit",
		"version: 9.8.7-test.1",
	} {
		if !strings.Contains(agent, want) {
			t.Fatalf("claude agent = %q, want %q", agent, want)
		}
	}
	hooksJSON := readBuildFileString(t, filepath.Join(root, "plugins", "loaf", "hooks", "hooks.json"))
	for _, want := range []string{
		`"PreToolUse": [`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook check-secrets"`,
		`"command": "cat \"${CLAUDE_PLUGIN_ROOT}/hooks/instructions/pre-merge.md\""`,
		`"PostToolUse": [`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" task refresh"`,
		`"command": "bash ${CLAUDE_PLUGIN_ROOT}/hooks/kb-staleness-nudge.sh"`,
		`"SessionStart": [`,
		`"command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" session start"`,
	} {
		if !strings.Contains(hooksJSON, want) {
			t.Fatalf("claude hooks.json = %q, want %q", hooksJSON, want)
		}
	}
	if readBuildFileString(t, filepath.Join(root, "plugins", "loaf", "hooks", "subagent-notify.sh")) != "#!/bin/sh\necho subagent\n" {
		t.Fatalf("subagent hook copy mismatch")
	}
	if readBuildFileString(t, filepath.Join(root, "plugins", "loaf", "SETUP.md")) != "# Setup\n" {
		t.Fatalf("SETUP.md copy mismatch")
	}
	for _, path := range []string{
		filepath.Join(root, "plugins", "loaf", "bin", "loaf"),
		filepath.Join(root, "plugins", "loaf", "bin", "package.json"),
		filepath.Join(root, "plugins", "loaf", "bin", "native", "darwin-arm64", "loaf"),
		filepath.Join(root, "plugins", "loaf", "package.json"),
		filepath.Join(root, "plugins", "loaf", ".lsp.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "plugins", "loaf", "dist-cli", "index.js")); !os.IsNotExist(err) {
		t.Fatalf("Stat(plugins/loaf/dist-cli/index.js) error = %v, want no TypeScript fallback in plugin", err)
	}
}

func TestRunnerBuildRejectsUnknownTargetBeforeContentBuilder(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)

	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: root,
	}.Run([]string{"build", "--target", "bogus"})
	if err == nil {
		t.Fatal("build --target bogus error = nil, want native target validation")
	}
	for _, want := range []string{"Unknown target bogus", "Valid targets: claude-code, opencode, cursor, codex, gemini, amp"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want %q", err, want)
		}
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want no content-builder invocation", statErr)
	}
}

func TestNativeBuildTargetNamesReadsTargetsYaml(t *testing.T) {
	root := realpath(t, t.TempDir())
	mkdirAll(t, filepath.Join(root, "config"))
	writeFile(t, filepath.Join(root, "config", "targets.yaml"), strings.Join([]string{
		"# Target Definitions",
		"shared-templates:",
		"  session.md: [implement]",
		"",
		"targets:",
		"  alpha:",
		"    output: dist/alpha/",
		"  beta:",
		"    output: dist/beta/",
		"",
	}, "\n"))

	targets, err := nativeBuildTargetNames(root)
	if err != nil {
		t.Fatalf("nativeBuildTargetNames error = %v", err)
	}
	if strings.Join(targets, ",") != "alpha,beta" {
		t.Fatalf("targets = %#v, want alpha,beta from targets.yaml", targets)
	}
}

func TestRunnerBuildRejectsMissingTargetValue(t *testing.T) {
	workingDir := realpath(t, t.TempDir())
	err := Runner{
		Stdout:     &bytes.Buffer{},
		WorkingDir: workingDir,
	}.Run([]string{"build", "--target"})
	if err == nil {
		t.Fatal("build --target error = nil, want missing value error")
	}
	if !strings.Contains(err.Error(), "--target requires a value") {
		t.Fatalf("error = %v, want missing target value", err)
	}
}

func TestRunnerBuildReportsNativeAllTargetFailure(t *testing.T) {
	root := setupBuildCommandLoafRoot(t)
	nodeLog := setupFakeNodeForBuild(t, 0)
	seedNativeCodexBuildFixture(t, root)
	seedNativeCursorBuildFixture(t, root)
	seedNativeOpenCodeBuildFixture(t, root)
	seedNativeClaudeCodeBuildFixture(t, root)
	if err := os.Remove(filepath.Join(root, "bin", "loaf")); err != nil {
		t.Fatalf("Remove(bin/loaf) error = %v", err)
	}
	var stdout bytes.Buffer

	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
	}.Run([]string{"build"})
	if err == nil {
		t.Fatal("build error = nil, want native target failure")
	}
	if _, statErr := os.Stat(nodeLog); !os.IsNotExist(statErr) {
		t.Fatalf("node log stat = %v, want native all-target failure without content-builder invocation", statErr)
	}
	if !strings.Contains(err.Error(), "Build failed") {
		t.Fatalf("error = %v, want native build failure", err)
	}
	if !strings.Contains(stdout.String(), "Loaf launcher not found at bin/loaf") {
		t.Fatalf("stdout = %q, want target failure detail", stdout.String())
	}
}

func setupBuildCommandLoafRoot(t *testing.T) string {
	t.Helper()
	root := realpath(t, t.TempDir())
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("MkdirAll(config) error = %v", err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	t.Setenv("PATH", bin)
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"loaf","version":"9.8.7-test.1"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(package.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "targets.yaml"), []byte(strings.Join([]string{
		"targets:",
		"  claude-code:",
		"    output: plugins/",
		"  opencode:",
		"    output: dist/opencode/",
		"  cursor:",
		"    output: dist/cursor/",
		"  codex:",
		"    output: dist/codex/",
		"  gemini:",
		"    output: dist/gemini/",
		"  amp:",
		"    output: dist/amp/",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(targets.yaml) error = %v", err)
	}
	return root
}

func seedNativeCodexBuildFixture(t *testing.T, root string) {
	t.Helper()
	mkdirAll(t, filepath.Join(root, "content", "skills", "demo", "references"))
	mkdirAll(t, filepath.Join(root, "content", "skills", "demo", "scripts"))
	mkdirAll(t, filepath.Join(root, "content", "templates"))
	writeFile(t, filepath.Join(root, "config", "targets.yaml"), strings.Join([]string{
		"shared-templates:",
		"  session.md: [demo]",
		"targets:",
		"  claude-code:",
		"    output: plugins/",
		"  opencode:",
		"    output: dist/opencode/",
		"  cursor:",
		"    output: dist/cursor/",
		"  codex:",
		"    output: dist/codex/",
		"  gemini:",
		"    output: dist/gemini/",
		"  amp:",
		"    output: dist/amp/",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "config", "hooks.yaml"), strings.Join([]string{
		"hooks:",
		"  pre-tool:",
		"    - id: check-secrets",
		"      matcher: \"Edit|Write|Bash\"",
		"      timeout: 30000",
		"      failClosed: true",
		"      description: Check for hardcoded secrets before writing",
		"    - id: workflow-pre-merge",
		"      type: command",
		"      instruction: instructions/pre-merge.md",
		"      matcher: \"Bash\"",
		"      if: \"Bash(gh pr merge:*)\"",
		"      timeout: 5000",
		"      description: Advisory only",
		"    - id: detect-linear-magic",
		"      matcher: \"Bash\"",
		"      timeout: 30000",
		"      description: Not a Codex enforcement hook",
		"  post-tool:",
		"    - id: generate-task-board",
		"      type: command",
		"      command: 'loaf task refresh'",
		"      matcher: \"Edit|Write\"",
		"      timeout: 30000",
		"      description: Regenerate TASKS.md when task files change",
		"    - id: kb-staleness-nudge",
		"      script: hooks/post-tool/kb-staleness-nudge.sh",
		"      matcher: \"Edit|Write\"",
		"      timeout: 5000",
		"      description: Track covered edits",
		"  session:",
		"    - id: session-start-loaf",
		"      type: command",
		"      command: 'loaf session start'",
		"      event: SessionStart",
		"      description: Start session journal",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.md"), strings.Join([]string{
		"---",
		"name: demo",
		"description: Demo skill that has enough words to require folded YAML output from gray matter when the native builder writes frontmatter for generated skills.",
		"---",
		"",
		"# Demo",
		"",
		"Run {{IMPLEMENT_CMD}} now.",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "references", "guide.md"), "Use {{IMPLEMENT_CMD}} from references.\n")
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "scripts", "demo.sh"), "#!/bin/sh\necho demo\n")
	if err := os.Chmod(filepath.Join(root, "content", "skills", "demo", "scripts", "demo.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(demo.sh) error = %v", err)
	}
	writeFile(t, filepath.Join(root, "content", "templates", "session.md"), "Resume with {{RESUME_CMD}}.\n")
}

func seedNativeCursorBuildFixture(t *testing.T, root string) {
	t.Helper()
	mkdirAll(t, filepath.Join(root, "content", "agents"))
	mkdirAll(t, filepath.Join(root, "content", "hooks", "instructions"))
	mkdirAll(t, filepath.Join(root, "content", "hooks", "post-tool"))
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.md"), strings.Join([]string{
		"# Implementer",
		"",
		"Implement code, tests, configuration, and docs.",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.cursor.yaml"), strings.Join([]string{
		"name: implementer",
		"description: >-",
		"  Implementer writes and modifies code, tests, configuration, and documentation",
		"  while following the selected skills.",
		"is_background: true",
		"tools:",
		"  Read: true",
		"  Write: true",
		"  Edit: true",
		"  Bash: true",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "hooks", "instructions", "pre-merge.md"), "Before merge.\n")
	writeFile(t, filepath.Join(root, "content", "hooks", "post-tool", "kb-staleness-nudge.sh"), "#!/bin/sh\necho default\n")
	writeFile(t, filepath.Join(root, "content", "hooks", "post-tool", "kb-staleness-nudge.cursor.sh"), "#!/bin/sh\necho cursor override\n")
	if err := os.Chmod(filepath.Join(root, "content", "hooks", "post-tool", "kb-staleness-nudge.cursor.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(kb-staleness-nudge.cursor.sh) error = %v", err)
	}
}

func seedNativeOpenCodeBuildFixture(t *testing.T, root string) {
	t.Helper()
	mkdirAll(t, filepath.Join(root, "content", "agents"))
	mkdirAll(t, filepath.Join(root, "content", "hooks", "instructions"))
	mkdirAll(t, filepath.Join(root, "content", "hooks", "post-tool"))
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.opencode.yaml"), strings.Join([]string{
		"subtask: false",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "agents", "background-runner.md"), strings.Join([]string{
		"# Background Runner",
		"",
		"Run in the background.",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "agents", "background-runner.opencode.yaml"), strings.Join([]string{
		"name: background-runner",
		"description: >-",
		"  Lightweight background agent for non-interactive tasks that can run",
		"  independently.",
		"mode: subagent",
		"skills:",
		"  - foundations",
		"tools:",
		"  Read: true",
		"  Edit: true",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "hooks", "instructions", "pre-merge.md"), "Before merge.\n")
	writeFile(t, filepath.Join(root, "content", "hooks", "post-tool", "kb-staleness-nudge.sh"), "#!/bin/sh\necho opencode hook\n")
	if err := os.Chmod(filepath.Join(root, "content", "hooks", "post-tool", "kb-staleness-nudge.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(kb-staleness-nudge.sh) error = %v", err)
	}
}

func seedNativeClaudeCodeBuildFixture(t *testing.T, root string) {
	t.Helper()
	mkdirAll(t, filepath.Join(root, "content", "skills", "implement"))
	mkdirAll(t, filepath.Join(root, "content", "hooks", "subagent"))
	mkdirAll(t, filepath.Join(root, "bin", "native", "darwin-arm64"))
	writeFile(t, filepath.Join(root, "content", "skills", "implement", "SKILL.md"), strings.Join([]string{
		"---",
		"name: implement",
		"description: Implement work.",
		"---",
		"",
		"# Implement",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "skills", "demo", "SKILL.claude-code.yaml"), strings.Join([]string{
		"allowed-tools: Bash",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "agents", "implementer.claude-code.yaml"), strings.Join([]string{
		"name: implementer",
		"description: Claude implementer agent",
		"tools:",
		"  - Read",
		"  - Edit",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "agents", "background-runner.claude-code.yaml"), strings.Join([]string{
		"name: background-runner",
		"description: Claude background runner agent",
		"tools:",
		"  - Read",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, "content", "hooks", "subagent", "subagent-notify.sh"), "#!/bin/sh\necho subagent\n")
	writeFile(t, filepath.Join(root, "content", "SETUP.md"), "# Setup\n")
	writeFile(t, filepath.Join(root, "bin", "loaf"), "#!/bin/sh\necho loaf\n")
	writeFile(t, filepath.Join(root, "bin", "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`+"\n")
	writeFile(t, filepath.Join(root, "bin", "native", "darwin-arm64", "loaf"), "native loaf\n")
	if err := os.Chmod(filepath.Join(root, "bin", "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(bin/loaf) error = %v", err)
	}
	if err := os.Chmod(filepath.Join(root, "content", "hooks", "subagent", "subagent-notify.sh"), 0o755); err != nil {
		t.Fatalf("Chmod(subagent-notify.sh) error = %v", err)
	}
}

func readBuildFileString(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(body)
}

func setupFakeNodeForBuild(t *testing.T, exitCode int) string {
	t.Helper()
	bin := strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0]
	log := filepath.Join(t.TempDir(), "node.log")
	t.Setenv("LOAF_TEST_NODE_LOG", log)
	script := strings.Join([]string{
		"#!/bin/sh",
		`printf 'cwd=%s\n' "$PWD" >> "$LOAF_TEST_NODE_LOG"`,
		`printf 'args=%s\n' "$*" >> "$LOAF_TEST_NODE_LOG"`,
		"exit " + fmt.Sprint(exitCode),
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(node) error = %v", err)
	}
	return log
}
